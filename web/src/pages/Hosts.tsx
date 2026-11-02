import { useMemo, useState } from 'react'
import { ago, api, type Host } from '../api'
import { Icon } from '../icons'
import { Copy, Drawer, Empty, Modal, StatusPill, notify } from '../ui'
import type { PageProps } from '../App'

function agentCommand(base: string, token: string) {
  return `weft agent --server ${base} \\
  --token ${token} \\
  --controller http://127.0.0.1:9090 \\
  --secret <mihomo-secret>`
}

export function Hosts({ state, refresh }: PageProps) {
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [created, setCreated] = useState<Host | null>(null)
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [freshToken, setFreshToken] = useState<string | null>(null)

  const selected = state.hosts.find((h) => h.id === selectedID) ?? null
  const selectedNodes = useMemo(
    () => state.nodes.filter((n) => n.hostId === selectedID),
    [state.nodes, selectedID],
  )

  async function create() {
    const value = name.trim()
    if (!value) return
    setBusy(true)
    try {
      const host = await api.createHost(value)
      setCreated(host)
      setName('')
      setAdding(false)
      await refresh()
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function rotate(host: Host) {
    if (!confirm(`轮换 ${host.name} 的 token？旧命令会立即失效。`)) return
    const res = await api.patchHost(host.id, { rotate: true })
    if (res.token) setFreshToken(res.token)
    notify('已生成新 token')
    await refresh()
  }

  async function remove(host: Host) {
    if (!confirm(`删除 ${host.name}？它的节点会同时从所有编排中移除。`)) return
    await api.deleteHost(host.id)
    setSelectedID(null)
    notify('机器已删除')
    await refresh()
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>机器</h1>
          <p className="sub">每台运行 mihomo 的主机跑一个 agent，向 weft 上报节点。</p>
        </div>
        <div className="title-actions">
          <button className="btn btn--primary" onClick={() => setAdding(true)}>
            <Icon name="plus" />
            添加机器
          </button>
        </div>
      </div>

      <section className="panel">
        {state.hosts.length === 0 ? (
          <Empty
            title="还没有机器"
            body="添加一台机器，拿到它的接入命令，在主机上跑起来即可。"
            action={
              <button className="btn btn--primary" onClick={() => setAdding(true)}>
                添加机器
              </button>
            }
          />
        ) : (
          <table className="table table--click">
            <thead>
              <tr>
                <th>机器</th>
                <th>状态</th>
                <th className="num">节点</th>
                <th className="num">平均延迟</th>
                <th>最后上报</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {state.hosts.map((h) => {
                const nodes = state.nodes.filter((n) => n.hostId === h.id && n.alive)
                const avg = nodes.length
                  ? Math.round(nodes.reduce((a, n) => a + n.delayMs, 0) / nodes.length)
                  : 0
                return (
                  <tr
                    key={h.id}
                    tabIndex={0}
                    role="button"
                    aria-label={`查看 ${h.name} 的详情`}
                    onClick={() => {
                      setSelectedID(h.id)
                      setFreshToken(null)
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        setSelectedID(h.id)
                        setFreshToken(null)
                      }
                    }}
                  >
                    <td data-label="机器">
                      <span className="cell-main">{h.name}</span>
                      <span className="cell-sub mono">{h.controller || '—'}</span>
                    </td>
                    <td data-label="状态">
                      <StatusPill status={h.status} />
                    </td>
                    <td className="num" data-label="节点">
                      <span>
                        {h.aliveCount}
                        <span className="muted"> / {h.nodeCount}</span>
                      </span>
                    </td>
                    <td className="num" data-label="平均延迟">
                      {avg ? `${avg} ms` : '—'}
                    </td>
                    <td data-label="最后上报" className="muted">
                      {h.lastSeen ? ago(h.lastSeen) : '从未'}
                    </td>
                    <td className="cell-action">
                      <Icon name="chevron" />
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </section>

      <section className="panel panel--note">
        <header>
          <h2>接入命令</h2>
          <span className="meta">token 在添加机器时生成一次</span>
        </header>
        <div className="panel-split">
          <div>
            <p className="prose">
              agent 只读本机 mihomo，节点与凭据不出机器；上报是出站请求，NAT 后也能用。
            </p>
            <ul className="facts">
              <li>
                <span>上报周期</span>
                <span>15 秒，失败退避至 60 秒</span>
              </li>
              <li>
                <span>待复核</span>
                <span>超过 45 秒没有上报</span>
              </li>
              <li>
                <span>离线</span>
                <span>超过 10 分钟没有上报</span>
              </li>
            </ul>
          </div>
          <div>
            <pre className="code">{`weft agent --server ${state.publicBase} \\
  --token <机器 token> \\
  --controller http://127.0.0.1:9090 \\
  --secret <mihomo secret>`}</pre>
            <div className="row-actions">
              <Copy
                label="复制模板"
                value={`weft agent --server ${state.publicBase} --token <机器 token> --controller http://127.0.0.1:9090 --secret <mihomo secret>`}
              />
            </div>
          </div>
        </div>
      </section>

      {adding && (
        <Modal
          title="添加机器"
          onClose={() => setAdding(false)}
          footer={
            <>
              <button className="btn btn--quiet" onClick={() => setAdding(false)}>
                取消
              </button>
              <button className="btn btn--primary" onClick={() => void create()} disabled={busy || !name.trim()}>
                生成 token
              </button>
            </>
          }
        >
          <label className="field">
            <span>机器名</span>
            <input
              autoFocus
              value={name}
              placeholder="hk-01"
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && void create()}
            />
            <span className="field__hint">用作订阅里的节点前缀，不要含空格或斜杠。</span>
          </label>
        </Modal>
      )}

      {created && (
        <Modal
          title={`${created.name} · 接入命令`}
          onClose={() => setCreated(null)}
          wide
          footer={
            <button className="btn btn--primary" onClick={() => setCreated(null)}>
              完成
            </button>
          }
        >
          <p className="prose">
            token 只显示这一次。把它和 mihomo 的 secret 一起写进主机上的 systemd 或 launchd 里。
          </p>
          <pre className="code">{agentCommand(state.publicBase, created.token ?? '')}</pre>
          <div className="row-actions">
            <Copy value={agentCommand(state.publicBase, created.token ?? '')} label="复制命令" />
          </div>
        </Modal>
      )}

      {selected && (
        <Drawer
          title={selected.name}
          onClose={() => {
            setSelectedID(null)
            setFreshToken(null)
          }}
        >
          <dl className="meta-list">
            <div>
              <dt>状态</dt>
              <dd>
                <StatusPill status={selected.status} />
              </dd>
            </div>
            <div>
              <dt>最后上报</dt>
              <dd>{selected.lastSeen ? ago(selected.lastSeen) : '从未'}</dd>
            </div>
            <div>
              <dt>mihomo</dt>
              <dd className="mono">{selected.mihomoVersion || '—'}</dd>
            </div>
            <div>
              <dt>agent</dt>
              <dd className="mono">{selected.agentVersion || '—'}</dd>
            </div>
            <div>
              <dt>controller</dt>
              <dd className="mono">{selected.controller || '—'}</dd>
            </div>
            <div>
              <dt>节点</dt>
              <dd>
                {selected.aliveCount} 可用 / {selected.nodeCount} 总计
              </dd>
            </div>
          </dl>

          <h3 className="section-title">接入命令</h3>
          {freshToken ? (
            <pre className="code">{agentCommand(state.publicBase, freshToken)}</pre>
          ) : (
            <p className="prose muted">token 只在生成时显示。轮换后会得到一条新命令。</p>
          )}
          <div className="row-actions">
            {freshToken && <Copy value={agentCommand(state.publicBase, freshToken)} label="复制命令" />}
            <button className="btn btn--quiet btn--sm" onClick={() => void rotate(selected)}>
              <Icon name="key" />
              轮换 token
            </button>
            <button className="btn btn--danger btn--sm" onClick={() => void remove(selected)}>
              <Icon name="trash" />
              删除机器
            </button>
          </div>

          <h3 className="section-title">节点（{selectedNodes.length}）</h3>
          <table className="table table--compact">
            <thead>
              <tr>
                <th>节点</th>
                <th>协议</th>
                <th className="num">延迟</th>
              </tr>
            </thead>
            <tbody>
              {selectedNodes.map((n) => (
                <tr key={n.key}>
                  <td>
                    <span className="node-name">{n.name}</span>
                  </td>
                  <td className="mono muted">{n.type}</td>
                  <td className="num">
                    {n.alive ? `${n.delayMs} ms` : <span className="muted">不可用</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Drawer>
      )}
    </>
  )
}
