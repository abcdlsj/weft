import { useMemo, useState } from 'react'
import { ago, api, type Host, type HostKind, type ProxyNode } from '../api'
import { Icon } from '../icons'
import { Copy, Drawer, Empty, Modal, StatusPill, notify } from '../ui'
import { LinkImport, NodeForm, NodeTable, protocolOf } from '../nodes'
import { NodeThreads } from '../weave'
import type { PageProps } from '../App'

function agentCommand(base: string, token: string, kind: HostKind) {
  if (kind === 'static') {
    return `weft agent --server ${base} \\
  --token ${token} \\
  --hysteria-config /etc/hysteria/config.yaml \\
  --xray-config /usr/local/etc/xray/config.json \\
  --public-host <这台机器的公网 IP>`
  }
  return `weft agent --server ${base} \\
  --token ${token} \\
  --controller http://127.0.0.1:9090 \\
  --secret <mihomo-secret>`
}

const KIND_COPY: Record<HostKind, { label: string; blurb: string }> = {
  agent: {
    label: '跑 mihomo 的机器',
    blurb: 'agent 读本机 mihomo，自动上报节点与延迟',
  },
  static: {
    label: '只跑 hy2 / v2ray 的机器',
    blurb: 'agent 读机器上的 hysteria / xray 配置，反推出客户端节点',
  },
}

export function Hosts({ state, refresh }: PageProps) {
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<HostKind>('agent')
  const [created, setCreated] = useState<Host | null>(null)
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [freshToken, setFreshToken] = useState<string | null>(null)

  const [nodeMode, setNodeMode] = useState<'none' | 'form' | 'import'>('none')
  const [editing, setEditing] = useState<ProxyNode | null>(null)

  const selected = state.hosts.find((h) => h.id === selectedID) ?? null
  const selectedNodes = useMemo(
    () => state.nodes.filter((n) => n.hostId === selectedID),
    [state.nodes, selectedID],
  )

  function closeNodeEditor() {
    setNodeMode('none')
    setEditing(null)
  }

  async function create() {
    const value = name.trim()
    if (!value) return
    setBusy(true)
    try {
      const host = await api.createHost(value, kind)
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

  async function removeNode(node: ProxyNode) {
    if (!selected) return
    if (!confirm(`删除节点 ${node.name}？它会从所有编排里移除。`)) return
    await api.deleteNode(selected.id, node.key)
    notify('节点已删除')
    await refresh()
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>机器</h1>
          <p className="sub">跑 mihomo 的机器自动上报；只跑 hy2 / v2ray 的机器由 agent 读配置反推。</p>
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
          <table className="table table--click table--hosts">
            <thead>
              <tr>
                <th>机器</th>
                <th>状态</th>
                <th>节点</th>
                <th className="num">可用</th>
                <th className="num">平均延迟</th>
                <th>数据来源</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {state.hosts.map((h) => {
                const all = state.nodes.filter((n) => n.hostId === h.id)
                const nodes = all.filter((n) => n.alive)
                const avg = nodes.length
                  ? Math.round(nodes.reduce((a, n) => a + n.delayMs, 0) / nodes.length)
                  : 0
                return (
                  <tr
                    key={h.id}
                    tabIndex={0}
                    role="button"
                    aria-label={`查看 ${h.name} 的详情`}
                    data-summary={`${h.aliveCount}/${h.nodeCount} 可用 · ${avg ? `${avg} ms` : '—'}`}
                    onClick={() => {
                      setSelectedID(h.id)
                      setFreshToken(null)
                      closeNodeEditor()
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        setSelectedID(h.id)
                        setFreshToken(null)
                        closeNodeEditor()
                      }
                    }}
                  >
                    <td data-label="机器">
                      <span className="cell-main">{h.name}</span>
                      <span className="cell-sub mono">
                        {h.kind === 'static' ? 'hy2 / v2ray' : h.controller || '—'}
                      </span>
                    </td>
                    <td data-label="状态">
                      <StatusPill status={h.status} />
                    </td>
                    <td data-label="节点" className="cell-threads">
                      <NodeThreads nodes={all} />
                    </td>
                    <td className="num" data-label="可用">
                      <span>
                        {h.aliveCount}
                        <span className="muted"> / {h.nodeCount}</span>
                      </span>
                    </td>
                    <td className="num" data-label="平均延迟">
                      {avg ? `${avg} ms` : '—'}
                    </td>
                    <td data-label="数据来源" className="muted">
                      {h.kind === 'static'
                        ? h.lastSeen
                          ? `配置 · ${ago(h.lastSeen)}`
                          : '配置 · 未上报'
                        : h.lastSeen
                          ? `mihomo · ${ago(h.lastSeen)}`
                          : 'mihomo · 未上报'}
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
          <h2>两种接入方式</h2>
          <span className="meta">token 在添加机器时生成一次</span>
        </header>
        <div className="panel-split">
          <div>
            <p className="prose">
              agent 只读本机，节点与凭据不出机器；上报是出站请求，NAT 后也能用。
            </p>
            <ul className="facts">
              <li>
                <span>mihomo 机器</span>
                <span>读 external-controller，节点与延迟实时上报</span>
              </li>
              <li>
                <span>hy2 / v2ray 机器</span>
                <span>读 hysteria / xray 配置，反推出密码、SNI、Reality 公钥</span>
              </li>
              <li>
                <span>上报周期</span>
                <span>15 秒，失败退避至 60 秒</span>
              </li>
            </ul>
          </div>
          <div>
            <pre className="code">{`# 只跑服务的机器
weft agent --server ${state.publicBase} \\
  --token <机器 token> \\
  --hysteria-config /etc/hysteria/config.yaml \\
  --xray-config /usr/local/etc/xray/config.json \\
  --public-host <公网 IP>`}</pre>
            <div className="row-actions">
              <Copy
                label="复制模板"
                value={`weft agent --server ${state.publicBase} --token <机器 token> --hysteria-config /etc/hysteria/config.yaml --xray-config /usr/local/etc/xray/config.json --public-host <公网 IP>`}
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
          <div className="kind-choice">
            {(Object.keys(KIND_COPY) as HostKind[]).map((key) => (
              <button
                key={key}
                type="button"
                className="kind-choice__option"
                aria-pressed={kind === key}
                onClick={() => setKind(key)}
              >
                <strong>{KIND_COPY[key].label}</strong>
                <span>{KIND_COPY[key].blurb}</span>
              </button>
            ))}
          </div>
          <label className="field">
            <span>机器名</span>
            <input
              autoFocus
              value={name}
              placeholder="h2nexus"
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
            token 只显示这一次。{created.kind === 'static'
              ? '把两条配置路径换成机器上的真实路径，公网 IP 填这台机器对外可达的地址。'
              : '把它和 mihomo 的 secret 一起写进主机上的 systemd 或 launchd 里。'}
          </p>
          <pre className="code">{agentCommand(state.publicBase, created.token ?? '', created.kind)}</pre>
          <div className="row-actions">
            <Copy
              value={agentCommand(state.publicBase, created.token ?? '', created.kind)}
              label="复制命令"
            />
          </div>
        </Modal>
      )}

      {selected && (
        <Drawer
          title={selected.name}
          wide
          onClose={() => {
            setSelectedID(null)
            setFreshToken(null)
            closeNodeEditor()
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
              <dt>类型</dt>
              <dd>{KIND_COPY[selected.kind]?.label ?? selected.kind}</dd>
            </div>
            <div>
              <dt>节点</dt>
              <dd>
                {selected.aliveCount} 可用 / {selected.nodeCount} 总计
              </dd>
            </div>
            {selected.kind === 'agent' && (
              <>
                <div>
                  <dt>最后上报</dt>
                  <dd>{selected.lastSeen ? ago(selected.lastSeen) : '从未'}</dd>
                </div>
                <div>
                  <dt>mihomo</dt>
                  <dd className="mono">{selected.mihomoVersion || '—'}</dd>
                </div>
                <div>
                  <dt>controller</dt>
                  <dd className="mono">{selected.controller || '—'}</dd>
                </div>
              </>
            )}
          </dl>

          <h3 className="section-title">接入命令</h3>
          {freshToken ? (
            <pre className="code">{agentCommand(state.publicBase, freshToken, selected.kind)}</pre>
          ) : (
            <>
              <p className="prose muted">token 只在生成时显示。下面这条命令里的 token 要换成轮换后拿到的那一个。</p>
              <pre className="code">
                {agentCommand(state.publicBase, '<机器 token>', selected.kind)}
              </pre>
            </>
          )}
          <div className="row-actions">
            {freshToken && (
              <Copy value={agentCommand(state.publicBase, freshToken, selected.kind)} label="复制命令" />
            )}
            <button className="btn btn--quiet btn--sm" onClick={() => void rotate(selected)}>
              <Icon name="key" />
              轮换 token
            </button>
            <button className="btn btn--danger btn--sm" onClick={() => void remove(selected)}>
              <Icon name="trash" />
              删除机器
            </button>
          </div>

          <div className="section-head">
            <h3 className="section-title">节点（{selectedNodes.length}）</h3>
            <div className="row-actions row-actions--tight">
              <button className="btn btn--quiet btn--sm" onClick={() => setNodeMode('import')}>
                <Icon name="link" />
                粘贴导入
              </button>
              <button
                className="btn btn--secondary btn--sm"
                onClick={() => {
                  setEditing(null)
                  setNodeMode('form')
                }}
              >
                <Icon name="plus" />
                手工添加
              </button>
            </div>
          </div>
          <NodeTable
            nodes={selectedNodes}
            compact
            onEdit={(node) => {
              setEditing(node)
              setNodeMode('form')
            }}
            onDelete={(node) => void removeNode(node)}
          />
        </Drawer>
      )}

      {selected && nodeMode === 'form' && (
        <Modal
          title={editing ? `编辑 ${editing.name}` : `添加节点 · ${selected.name}`}
          wide
          onClose={closeNodeEditor}
        >
          <NodeForm
            initial={editing ? protocolOf(editing) : undefined}
            initialProxy={
              editing ? (JSON.parse(JSON.stringify(editing.raw ?? {})) as Record<string, unknown>) : undefined
            }
            busy={busy}
            onCancel={closeNodeEditor}
            onSubmit={async (proxy) => {
              setBusy(true)
              try {
                if (editing) await api.updateNode(selected.id, editing.key, proxy)
                else await api.createNode(selected.id, { proxy })
                notify(editing ? '节点已更新' : '节点已添加')
                await refresh()
                closeNodeEditor()
              } finally {
                setBusy(false)
              }
            }}
          />
        </Modal>
      )}

      {selected && nodeMode === 'import' && (
        <Modal title={`粘贴导入 · ${selected.name}`} wide onClose={closeNodeEditor}>
          <p className="prose">
            一行一条，支持 <span className="mono">hy2://</span>{' '}
            <span className="mono">vless://</span> <span className="mono">trojan://</span>{' '}
            <span className="mono">ss://</span> <span className="mono">vmess://</span>。
            重复导入同一条只会更新，不会新增。
          </p>
          <LinkImport
            hostId={selected.id}
            onDone={refresh}
            onCancel={closeNodeEditor}
          />
        </Modal>
      )}
    </>
  )
}
