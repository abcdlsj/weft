# weft 设计稿

> 状态：草案。本文件是实现的唯一依据，改动先改这里。

## 1. 目标

把散落在多台机器上的代理节点收进一处，对外只发**一条**订阅 URL，配一个移动端优先的 Web 面板。

非目标：不做协议实现（节点连接交给客户端内核）；**不做流量中转**——订阅里的流量仍是设备直连节点，weft 不在数据通路上；不做机场转售；不做多租户。

## 2. 拓扑

```
 跑 mihomo 的机器   ─┐
 只跑 hy2 / xray 的机器 ─┼─ weft agent ──出站 HTTPS──▶ weft server ──▶ /sub/{token} ──▶ 客户端
 面板里手填的节点     ─┘                                │
                                                          └──▶ 内嵌 SPA 面板
```

一台机器上的节点有三种来源，可以共存：

| 来源 | 从哪来 | 生命周期 |
|---|---|---|
| `agent` | 本机 mihomo 的 `external-controller` | 每次上报全量覆盖 |
| `config` | 本机 `hysteria` / `xray` 的服务端配置 | 每次上报全量覆盖 |
| `manual` | 面板表单或粘贴分享链接 | 只能手工增删改 |

选 agent 上报（而不是 server 去 SSH 读配置）的理由：

- 无需在主机上开入站端口或端口映射，NAT 后可用。
- server 不持有任何 SSH 凭据。
- 上报是出站请求，失败可重试，网络抖动容忍度高。
- 凭据读取集中在一个有权限的进程里，配置路径由部署者显式指定。

## 3. 领域模型

| 实体 | 字段要点 | 说明 |
|---|---|---|
| `Host` | id, name, token, **kind**(agent/static), last_seen, agent_version, controller_url, status | 一台机器。`kind=agent` 跑 mihomo，`kind=static` 只跑服务端（hy2/xray）。`status` 由 `last_seen` 推导：online < 11m, stale < 30m, offline（上报周期 5 分钟，容忍一次丢失）；static 恒为 `manual` |
| `Node` | host_id, name(原名), key(去重键: type+server+port+uuid/password hash), type, **source**(agent/config/manual), delay_ms, alive, raw, updated_at | `raw` 是完整的 mihomo proxy map，订阅直接用它，不再做协议解析 |
| `Bundle` | id, name, slug, rule_template_id, member_keys, group_by | 跨 host 的节点选集 + 分组策略 |
| `Subscription` | id, bundle_id, token, target_format, enabled, last_used_at, ua | 一条对外 URL |
| `RuleTemplate` | id, name, yaml_body | 通用段（proxy-groups / rules / dns 等） |
| `Report` | host_id, at, node_count, alive_count, avg_delay | 上报历史，面板延迟曲线 |
| `SubAccess` | subscription_id, at, device, ua, ip | 拉取日志，7 天滚动；按设备折叠成「哪台设备在订阅」 |

端点命名：`hostname/节点名`（如 `hk-01/HK-A`），冲突时追加 `#2`。订阅里保证全局唯一。

节点写入端点（只动 `source=manual` 的行，agent / config 的快照不可改）：

- `POST /api/hosts/{id}/nodes` — body 为 `{"links": ["hy2://…"]}` 或 `{"proxy": {…}}`；链接导入会逐条返回失败原因
- `PUT /api/hosts/{id}/nodes/{key}`、`DELETE /api/hosts/{id}/nodes/{key}`
- `POST /api/nodes/parse` — 只解析不落库，面板用来预览

链接解析支持 `hy2://` / `hysteria2://`、`vless://`（含 Reality）、`trojan://`、`ss://`、`vmess://`；key 由凭据推导，重复导入同一条只会更新。

## 4. Agent 协议

采集源由 flag 决定，三者可任意组合：

```bash
# 跑 mihomo 的机器
weft agent --server ... --token ... --controller http://127.0.0.1:9090 --secret <secret>

# 只跑 hy2 / xray 的机器
weft agent --server ... --token ... \
  --hysteria-config /etc/hysteria/config.yaml \
  --xray-config /usr/local/etc/xray/config.json \
  --public-host 203.0.113.24
```

服务端配置反向推导客户端节点，是这套设计的关键一步：

- **hysteria2**：`listen` → 端口；`auth.password` / `auth.userpass` → 凭据（多用户展开成多个节点）；`tls.sni` → 其次 `acme.domains[0]` → 其次证书 SAN；读不到证书或证书自签 → `skip-cert-verify: true`；`obfs.salamander` → 混淆；`bandwidth` → 上下行。
- **xray**：`inbounds[].port` / `[].protocol` / `settings.clients[].id`；`streamSettings.security=reality` 时，客户端需要的 `publicKey` **由服务端 `privateKey` 经 X25519 推导**（`crypto/ecdh`，与 `xray x25519` 同算法），`serverNames[0]` → SNI，`shortIds[0]` → short-id。
- **公网地址无法从配置读出**（云上公网 IP 常是 NAT），必须由 `--public-host` 给出；缺省时退回 `127.0.0.1`，面板会显示为不可用。

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

- 默认 5 分钟上报一次；上报失败按 15s→30s→60s 重试，成功后回到 5 分钟。
- agent 侧调用 mihomo `GET /proxies`、`GET /providers/proxies`，合并出站（排除 `DIRECT`/`REJECT`/内置组），对 `proxy-providers` 里的节点也一并展开。
- 首次注册：`weft host add` 在 server 侧签发 token，或面板生成一次性注册码，agent 用注册码换 token 后存本地。

## 5. 订阅出口

`GET /sub/{token}` → 依据 `Subscription.target_format` 与 `?target=` 覆盖：

- `mihomo`（默认）：完整 YAML = RuleTemplate 的 proxy-groups/rules/dns + 聚合后的 `proxies`。
- `base64`：节点链接列表 base64（v2rayN 生态）。
- `text`：明文节点链接，一行一条。

细节：

- 节点名统一加机器前缀（`host/节点`），同名不同机不会冲突；前缀也是排障时的第一线索。
- 每次命中写 `last_used_at`、UA、来源 IP，并落一条 `sub_access`（保留 7 天）。UA 归一成客户端名（Clash Verge / sing-box / Shadowrocket / mihomo…），面板按设备聚合展示次数与最近时间；不采集任何设备指纹。
- token 为 32 字节随机，可吊销、可轮换。
- 分组策略 `group_by`: `host`（按机器分组）、`type`（按协议）、`flat`（不分组），组行为取 `url-test` / `select` / `fallback` / `load-balance`。模板里的 `PROXY` 组填入分组入口，`AUTO` 组填入全部节点；模板缺少的分组由 weft 追加，模板其余段落与键顺序原样保留。
- 健康检查：`proxy-groups` 里注入 `url-test` 组按 `delay` 排序，数据来自 agent 最近一次延迟测试。

## 6. 面板

技术：Go `embed` 静态资源 + Vite/React/TS + Tailwind；`vite-plugin-pwa` 出 manifest 与 service worker；Safari 支持靠 `apple-mobile-web-app-capable`、`apple-touch-icon`、`theme-color`、`safe-area-inset` 与 `viewport-fit=cover`。

信息架构（4 个目的地，移动端底部 tab / 桌面侧栏）：

1. **概览** — 在线机器数、节点总数/存活比、订阅最近访问；机器卡片带延迟脉冲。
2. **机器** — 列表 + 详情（类型、最后上报、节点表、接入命令一键复制）；抽屉里可直接粘贴导入或手工添加节点。
3. **编排** — Bundle 列表 → 编辑器（左侧节点选择、右侧分组预览、顶部实时 YAML 预览）。
4. **订阅** — 链接列表、QR、复制、吊销、访问记录。

视觉方向由 `taste-skill` 的 app-screen 路径确定（`draw.mjs --app`），并按其 house rules 用 `inspect.mjs --app` 在真实浏览器中校验：无渐变、无发光、无彩色图表、重复面板用安静边框；数字必须真实可加总。

## 7. 技术选型

| 层 | 选择 | 理由 |
|---|---|---|
| 服务端 | Go 单二进制 | 与 mihomo 生态一致，交叉编译简单，静态资源可 embed |
| 存储 | SQLite（`modernc.org/sqlite`，纯 Go） | 单机零依赖；节点表整体覆盖写，量小 |
| 前端 | React + TS + Tailwind + vite-plugin-pwa | 生态成熟，PWA/Safari 支持好 |
| 认证 | 面板口令（PBKDF2-SHA256）+ HttpOnly session cookie；agent 独立 Bearer token | 简单、够用 |

## 8. 里程碑

1. **M1（已完成）** — server + agent、上报与订阅出口、面板五页、规则模板、demo 模式、PWA。
2. M2 — 面板触发延迟测试、节点按机器批量重命名、订阅访问统计图表（当前只有延迟折线与设备聚合）。
3. M3 — 订阅访问统计图表、多用户、通知 webhook。

## 9. 开放问题

- 是否需要 agent 反向隧道，让面板直接调用各机 mihomo API（切换策略组、触发测速）。
- 订阅是否要按设备限速或限 UA。
- 多用户与只读账号是否需要（当前是单口令）。
