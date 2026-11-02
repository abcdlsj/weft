# weft 设计稿

> 状态：草案。本文件是实现的唯一依据，改动先改这里。

## 1. 目标

管理多台机器上的 mihomo，把它们的节点聚合成**一条**对外订阅 URL，配一个精致、移动端优先的 Web 面板。

非目标：不做协议实现（节点连接全部交给 mihomo）；不做机场转售；不做多租户。

## 2. 拓扑

```
 mihomo (host)  ← 127.0.0.1:9090 (external-controller)
      ↑
 weft agent  ──出站 HTTPS──▶  weft server  ──▶ /sub/{token} ──▶ 客户端
                                   │
                                   └──▶ 内嵌 SPA 面板
```

选择 A 方案（agent 上报）的理由：

- 无需在主机上开入站端口或端口映射，NAT 后可用。
- server 不持有任何 SSH 凭据。
- agent 与 mihomo 同机，`external-controller` 保持 `127.0.0.1` 绑定，攻击面最小。
- 上报走 pull 语义（agent 主动 POST），失败可重试，网络抖动容忍度高。

## 3. 领域模型

| 实体 | 字段要点 | 说明 |
|---|---|---|
| `Host` | id, name, token, last_seen, agent_version, controller_url, status | 一台机器。`status` 由 `last_seen` 推导：online < 45s, stale < 10m, offline |
| `Node` | host_id, name(原名), key(去重键: type+server+port+uuid/password hash), type, meta, delay_ms, alive, updated_at | agent 上报的 proxies 快照，每次上报整体覆盖该 host 的集合 |
| `Bundle` | id, name, slug, rule_template_id, member_keys, group_by | 跨 host 的节点选集 + 分组策略 |
| `Subscription` | id, bundle_id, token, target_format, enabled, last_used_at, ua | 一条对外 URL |
| `RuleTemplate` | id, name, yaml_body | 通用段（proxy-groups / rules / dns 等） |
| `Report` | host_id, at, node_count, alive_count | 上报历史，用于面板折线 |

端点命名：`hostname/节点名`（如 `hk-01/HK-A`），冲突时追加 `#2`。订阅里保证全局唯一。

## 4. Agent 协议

单次上报，`POST /api/agent/report`，`Authorization: Bearer <agent-token>`：

```json
{
  "agent_version": "0.1.0",
  "mihomo": { "version": "v1.19.0", "controller": "http://127.0.0.1:9090" },
  "at": "2026-11-02T12:00:00Z",
  "nodes": [
    { "name": "HK-A", "type": "ss", "server": "1.2.3.4", "port": 8388,
      "key": "ss:1.2.3.4:8388:<credhash>", "delay_ms": 42, "alive": true,
      "meta": { "udp": true, "cipher": "aes-128-gcm" } }
  ]
}
```

- 默认 15s 上报一次；上报失败指数退避至 60s。
- agent 侧调用 mihomo `GET /proxies`、`GET /providers/proxies`，合并出站（排除 `DIRECT`/`REJECT`/内置组），对 `proxy-providers` 里的节点也一并展开。
- 首次注册：`weft host add` 在 server 侧签发 token，或面板生成一次性注册码，agent 用注册码换 token 后存本地。

## 5. 订阅出口

`GET /sub/{token}` → 依据 `Subscription.target_format` 与 `?target=` 覆盖：

- `mihomo`（默认）：完整 YAML = RuleTemplate 的 proxy-groups/rules/dns + 聚合后的 `proxies`。
- `base64`：节点链接列表 base64（v2rayN 生态）。
- `text`：明文节点链接，一行一条。

细节：

- `Subscription` 可绑定 UA 规则；可设置 `hidden` 节点（不进订阅、仅在面板可见）。
- 每次命中写 `last_used_at` 与 UA，面板展示「最近访问」。超出 30 条只保留聚合统计。
- token 为 32 字节随机，可吊销、可轮换。
- 分组策略 `group_by`: `host`（按机器分组）、`type`（按协议）、`flat`（全部一个组），配 `auto`/`select` 两种组行为。
- 健康检查：`proxy-groups` 里注入 `url-test` 组按 `delay` 排序，数据来自 agent 最近一次延迟测试。

## 6. 面板

技术：Go `embed` 静态资源 + Vite/React/TS + Tailwind；`vite-plugin-pwa` 出 manifest 与 service worker；Safari 支持靠 `apple-mobile-web-app-capable`、`apple-touch-icon`、`theme-color`、`safe-area-inset` 与 `viewport-fit=cover`。

信息架构（4 个目的地，移动端底部 tab / 桌面侧栏）：

1. **概览** — 在线机器数、节点总数/存活比、订阅最近访问；机器卡片带延迟脉冲。
2. **机器** — 列表 + 详情（agent 版本、最后上报、节点表、注册命令一键复制）。
3. **编排** — Bundle 列表 → 编辑器（左侧节点选择、右侧分组预览、顶部实时 YAML 预览）。
4. **订阅** — 链接列表、QR、复制、吊销、访问记录。

视觉方向由 `taste-skill` 的 app-screen 路径确定（`draw.mjs --app`），并按其 house rules 用 `inspect.mjs --app` 在真实浏览器中校验：无渐变、无发光、无彩色图表、重复面板用安静边框；数字必须真实可加总。

## 7. 技术选型

| 层 | 选择 | 理由 |
|---|---|---|
| 服务端 | Go 单二进制 | 与 mihomo 生态一致，交叉编译简单，静态资源可 embed |
| 存储 | SQLite（`modernc.org/sqlite`，纯 Go） | 单机零依赖；节点表整体覆盖写，量小 |
| 前端 | React + TS + Tailwind + vite-plugin-pwa | 生态成熟，PWA/Safari 支持好 |
| 认证 | 面板口令 + HttpOnly session cookie；agent 独立 Bearer token | 简单、够用 |

## 8. 里程碑

1. **M1（当前）** — server + agent 骨架、上报与订阅出口、面板三页、demo 模式。
2. M2 — 规则模板编辑、节点批量过滤与搜索、延迟测试触发。
3. M3 — 订阅访问统计图表、多用户、通知 webhook。

## 9. 开放问题

- 端口/服务发现：是否需要 agent 反向隧道以支持「面板直连 mihomo API」（如切换策略组）。
- 订阅按流量/设备限流是否需要。
