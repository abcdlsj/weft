import { useState } from 'react'
import { ago, api, type Subscription } from '../api'
import { Icon } from '../icons'
import { Copy, Empty, Modal, Switch, notify } from '../ui'
import type { PageProps } from '../App'

const formatLabel: Record<string, string> = {
  mihomo: 'mihomo YAML',
  base64: 'base64 链接',
  text: '明文链接',
}

export function Subscriptions({ state, refresh }: PageProps) {
  const [creating, setCreating] = useState(false)
  const [bundleID, setBundleID] = useState(state.bundles[0]?.id ?? '')
  const [label, setLabel] = useState('')
  const [format, setFormat] = useState('mihomo')
  const [busy, setBusy] = useState(false)
  const [qr, setQr] = useState<Subscription | null>(null)
  const [fresh, setFresh] = useState<{ label: string; url: string } | null>(null)

  const url = (s: Subscription) => `${state.publicBase}/sub/${s.token}`

  async function create() {
    if (!bundleID) return
    setBusy(true)
    try {
      const res = await api.createSubscription({ bundleId: bundleID, label: label || '未命名', format })
      setCreating(false)
      setLabel('')
      setFresh({ label: res.subscription.label, url: res.url })
      await refresh()
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function patch(s: Subscription, body: { enabled?: boolean; format?: string; rotate?: boolean }) {
    const res = await api.patchSubscription(s.id, body)
    if (res.url) setFresh({ label: s.label || '未命名', url: res.url })
    notify('已更新')
    await refresh()
  }

  async function remove(s: Subscription) {
    if (!confirm(`删除订阅「${s.label || '未命名'}」？客户端将无法继续拉取。`)) return
    await api.deleteSubscription(s.id)
    notify('订阅已删除')
    await refresh()
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>订阅</h1>
          <p className="sub">一条链接对应一条编排。换设备、换客户端都用同一个地址。</p>
        </div>
        <div className="title-actions">
          <button
            className="btn btn--primary"
            onClick={() => setCreating(true)}
            disabled={state.bundles.length === 0}
          >
            <Icon name="plus" />
            新建订阅
          </button>
        </div>
      </div>

      <section className="panel">
        {state.subscriptions.length === 0 ? (
          <Empty
            title="还没有订阅链接"
            body={
              state.bundles.length
                ? '从一条编排生成链接，粘进客户端即可。'
                : '先创建一条编排，再生成订阅链接。'
            }
          />
        ) : (
          <ul className="subs">
            {state.subscriptions.map((s) => (
              <li key={s.id} className={s.enabled ? 'subs__row' : 'subs__row subs__row--off'}>
                <div className="subs__main">
                  <div className="subs__title">
                    <span className="cell-main">{s.label || '未命名'}</span>
                    <span className="pill pill--neutral">{formatLabel[s.format] ?? s.format}</span>
                    {!s.enabled && <span className="pill pill--warn">已停用</span>}
                  </div>
                  <div className="subs__url">
                    <code className="mono">{url(s)}</code>
                    <Copy value={url(s)} />
                  </div>
                  <div className="subs__meta">
                    <span>编排 {s.bundleName}</span>
                    <span>拉取 {s.hits} 次</span>
                    <span>{s.lastUsedAt ? `最近 ${ago(s.lastUsedAt)}` : '还没被拉取过'}</span>
                  </div>
                  <div className="devices">
                    {(s.devices ?? []).length === 0 ? (
                      <span className="devices__empty">还没有设备拉取过这条链接</span>
                    ) : (
                      (s.devices ?? []).map((d) => (
                        <span
                          key={d.device}
                          className="device"
                          title={`${d.ua || d.device}
IP ${d.ip} · 最近 ${ago(d.lastAt)} · 共 ${d.hits} 次`}
                        >
                          <Icon name="device" />
                          <span className="device__name">{d.device}</span>
                          <span className="device__meta">
                            {d.hits} 次 · {ago(d.lastAt)}
                          </span>
                        </span>
                      ))
                    )}
                  </div>
                </div>
                <div className="subs__actions">
                  <Switch
                    checked={s.enabled}
                    label={s.enabled ? '停用订阅' : '启用订阅'}
                    onChange={() => void patch(s, { enabled: !s.enabled })}
                  />
                  <select
                    value={s.format}
                    aria-label="输出格式"
                    onChange={(e) => void patch(s, { format: e.target.value })}
                  >
                    <option value="mihomo">mihomo</option>
                    <option value="base64">base64</option>
                    <option value="text">明文</option>
                  </select>
                  <button className="icon-btn" aria-label="二维码" onClick={() => setQr(s)}>
                    <Icon name="qr" />
                  </button>
                  <button
                    className="icon-btn"
                    aria-label="轮换地址"
                    onClick={() => {
                      if (confirm('轮换地址？旧链接会立刻失效。')) void patch(s, { rotate: true })
                    }}
                  >
                    <Icon name="key" />
                  </button>
                  <button className="icon-btn" aria-label="删除" onClick={() => void remove(s)}>
                    <Icon name="trash" />
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="panel panel--note">
        <header>
          <h2>客户端怎么填</h2>
          <span className="meta">三种输出格式的区别</span>
        </header>
        <ul className="facts facts--defs">
          <li>
            <span>mihomo YAML</span>
            <span>整份配置：节点、分组、规则、DNS 都由 weft 生成，直接作为 profile 地址。</span>
          </li>
          <li>
            <span>base64 链接</span>
            <span>节点列表的 base64，给 v2rayN、Shadowrocket 这类只吃节点的客户端。</span>
          </li>
          <li>
            <span>明文链接</span>
            <span>一行一个 <span className="mono">ss://</span> 或 <span className="mono">vmess://</span>，便于脚本处理。</span>
          </li>
        </ul>
        <p className="prose muted">
          轮换地址会让旧链接立即失效；停用只是让链接返回 404，随时可以恢复。
        </p>
      </section>

      {creating && (
        <Modal
          title="新建订阅"
          onClose={() => setCreating(false)}
          footer={
            <>
              <button className="btn btn--quiet" onClick={() => setCreating(false)}>
                取消
              </button>
              <button className="btn btn--primary" onClick={() => void create()} disabled={busy || !bundleID}>
                生成链接
              </button>
            </>
          }
        >
          <label className="field">
            <span>编排</span>
            <select value={bundleID} onChange={(e) => setBundleID(e.target.value)}>
              {state.bundles.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}（{b.members.length} 节点）
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>名称</span>
            <input
              value={label}
              placeholder="主力订阅"
              onChange={(e) => setLabel(e.target.value)}
            />
            <span className="field__hint">方便区分设备或客户端，可以不填。</span>
          </label>
          <label className="field">
            <span>输出格式</span>
            <select value={format} onChange={(e) => setFormat(e.target.value)}>
              <option value="mihomo">mihomo YAML（完整配置）</option>
              <option value="base64">base64 链接列表</option>
              <option value="text">明文链接列表</option>
            </select>
          </label>
        </Modal>
      )}

      {fresh && (
        <Modal
          title="订阅链接"
          onClose={() => setFresh(null)}
          wide
          footer={
            <button className="btn btn--primary" onClick={() => setFresh(null)}>
              完成
            </button>
          }
        >
          <p className="prose">把它粘进 mihomo、Clash Verge 或 sing-box 的订阅地址里。</p>
          <pre className="code">{fresh.url}</pre>
          <div className="row-actions">
            <Copy value={fresh.url} label="复制链接" />
          </div>
        </Modal>
      )}

      {qr && (
        <Modal title={`${qr.label || '未命名'} · 二维码`} onClose={() => setQr(null)}>
          <div className="qr">
            <img src={`/api/subscriptions/${qr.id}/qr.png?size=360`} alt="订阅二维码" width={280} height={280} />
            <code className="mono">{url(qr)}</code>
          </div>
        </Modal>
      )}
    </>
  )
}
