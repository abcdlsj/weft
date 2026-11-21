import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ago, api, type Host, type HostKind, type ProxyNode } from '../api'
import { Icon, type IconName } from '../icons'
import { ConfirmModal, Copy, Drawer, Empty, Modal, StatusPill, notify } from '../ui'
import { LinkImport, NodeForm, NodeTable, delayTone, protocolOf } from '../nodes'
import { NodeThreads } from '../weave'
import type { PageProps } from '../App'

function agentCommand(base: string, token: string, kind: HostKind) {
  if (kind === 'static') {
    return `weft agent --server ${base} \\
  --token ${token} \\
  --hysteria-config /etc/hysteria/config.yaml \\
  --xray-config /usr/local/etc/xray/config.json \\
  --public-host <public IP>`
  }
  return `weft agent --server ${base} \\
  --token ${token} \\
  --controller http://127.0.0.1:9090 \\
  --secret <mihomo-secret>`
}

const KIND_COPY: Record<HostKind, { label: string; short: string; blurb: string; icon: IconName }> = {
  agent: {
    label: '跑 mihomo 的机器',
    short: 'mihomo',
    blurb: 'agent 读本机 mihomo，自动上报节点与延迟',
    icon: 'hosts',
  },
  static: {
    label: '只跑 hy2 / v2ray 的机器',
    short: 'hy2 / v2ray',
    blurb: 'agent 读机器上的 hysteria / xray 配置，反推出客户端节点',
    icon: 'key',
  },
  sub: {
    label: '订阅地址（机场）',
    short: '订阅源',
    blurb: 'Weft 自己拉订阅，清掉广告节点，按地区拆组',
    icon: 'globe',
  },
}

type ConfirmAction =
  | { type: 'rotate'; host: Host }
  | { type: 'delete-host'; host: Host }
  | { type: 'delete-node'; node: ProxyNode }

export function Hosts({ state, refresh, navigate, search }: PageProps) {
  const { t } = useTranslation()
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<HostKind>('agent')
  const [url, setURL] = useState('')
  const [created, setCreated] = useState<Host | null>(null)
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [freshToken, setFreshToken] = useState<string | null>(null)
  const [hostQuery, setHostQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'online' | 'issue'>('all')
  const [nodeQuery, setNodeQuery] = useState('')
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  const [nodeMode, setNodeMode] = useState<'none' | 'form' | 'import'>('none')
  const [editing, setEditing] = useState<ProxyNode | null>(null)

  // Deep-link from Overview: /hosts?id=<hostId>
  useEffect(() => {
    const id = new URLSearchParams(search).get('id')
    if (id && state.hosts.some((h) => h.id === id)) {
      setSelectedID(id)
      setFreshToken(null)
      setNodeQuery('')
      setNodeMode('none')
      setEditing(null)
    }
  }, [search, state.hosts])

  const selected = state.hosts.find((h) => h.id === selectedID) ?? null
  const selectedNodes = useMemo(
    () => state.nodes.filter((n) => n.hostId === selectedID),
    [state.nodes, selectedID],
  )
  const filteredSelectedNodes = useMemo(() => {
    const q = nodeQuery.trim().toLowerCase()
    if (!q) return selectedNodes
    return selectedNodes.filter(
      (n) =>
        n.name.toLowerCase().includes(q) ||
        n.type.toLowerCase().includes(q) ||
        n.server.toLowerCase().includes(q),
    )
  }, [selectedNodes, nodeQuery])

  const filteredHosts = useMemo(() => {
    const q = hostQuery.trim().toLowerCase()
    return state.hosts.filter((h) => {
      if (statusFilter === 'online' && h.status !== 'online' && h.status !== 'manual') return false
      if (statusFilter === 'issue' && (h.status === 'online' || h.status === 'manual') && !h.subError) return false
      if (!q) return true
      return (
        h.name.toLowerCase().includes(q) ||
        (h.controller ?? '').toLowerCase().includes(q) ||
        (h.subUrl ?? '').toLowerCase().includes(q) ||
        h.kind.toLowerCase().includes(q)
      )
    })
  }, [state.hosts, hostQuery, statusFilter])

  const issueCount = useMemo(
    () => state.hosts.filter((h) => (h.status !== 'online' && h.status !== 'manual') || !!h.subError).length,
    [state.hosts],
  )

  function openHost(id: string) {
    setSelectedID(id)
    setFreshToken(null)
    setNodeQuery('')
    closeNodeEditor()
  }

  function closeHost() {
    setSelectedID(null)
    setFreshToken(null)
    setNodeQuery('')
    closeNodeEditor()
    if (new URLSearchParams(search).has('id')) navigate('/hosts')
  }

  function closeNodeEditor() {
    setNodeMode('none')
    setEditing(null)
  }

  async function create() {
    const value = name.trim()
    if (!value) return
    if (kind === 'sub' && !url.trim()) {
      notify(t('订阅地址不能为空'), 'bad')
      return
    }
    setBusy(true)
    try {
      const res = await api.createHost(value, kind, kind === 'sub' ? url.trim() : undefined)
      const host = 'host' in res && res.host ? res.host : (res as Host)
      if (kind === 'sub') {
        const pulled = 'nodes' in res ? res.nodes ?? 0 : 0
        const failure = 'error' in res ? res.error : ''
        notify(
          failure
            ? t('订阅拉取失败：{{error}}', { error: failure })
            : t('已拉取 {{n}} 个节点', { n: pulled }),
          failure ? 'bad' : 'ok',
        )
      }
      setCreated(kind === 'sub' ? null : host)
      setName('')
      setURL('')
      setAdding(false)
      await refresh()
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }

  async function refreshSource(host: Host) {
    setBusy(true)
    try {
      const res = await api.refreshHost(host.id)
      notify(
        res.error
          ? t('订阅拉取失败：{{error}}', { error: res.error })
          : t('已拉取 {{n}} 个节点', { n: res.nodes }),
        res.error ? 'bad' : 'ok',
      )
      await refresh()
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }

  async function runConfirmed() {
    if (!confirmAction) return
    setBusy(true)
    try {
      if (confirmAction.type === 'rotate') {
        const res = await api.patchHost(confirmAction.host.id, { rotate: true })
        if (res.token) setFreshToken(res.token)
        notify(t('已生成新 token'), 'ok')
      } else if (confirmAction.type === 'delete-host') {
        await api.deleteHost(confirmAction.host.id)
        closeHost()
        notify(t('机器已删除'), 'ok')
      } else if (confirmAction.type === 'delete-node' && selected) {
        await api.deleteNode(selected.id, confirmAction.node.key)
        notify(t('节点已删除'), 'ok')
      }
      setConfirmAction(null)
      await refresh()
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('{{n}} 台机器', { n: state.hosts.length })}</h1>
          <p className="sub">
            {t('跑 mihomo 的机器自动上报；只跑 hy2 / v2ray 的机器由 agent 读配置反推；机场订阅由 Weft 自己拉。')}
          </p>
        </div>
        <div className="actions">
          <button className="primary" onClick={() => setAdding(true)}>
            <Icon name="plus" size={15} />
            <span>{t('添加机器')}</span>
          </button>
        </div>
      </div>

      <section className="panel panel-flush">
        {state.hosts.length === 0 ? (
          <div className="panel-pad">
            <Empty
              icon="hosts"
              title={t('还没有机器')}
              body={t('添加一台机器，拿到它的接入命令，在主机上跑起来即可。')}
              action={
                <button className="primary" onClick={() => setAdding(true)}>
                  <Icon name="plus" size={15} />
                  <span>{t('添加机器')}</span>
                </button>
              }
            />
          </div>
        ) : (
          <>
            <div className="toolbar">
              <div className="search-box">
                <Icon name="search" size={15} className="search-icon" />
                <input
                  type="search"
                  value={hostQuery}
                  placeholder={t('搜索机器或地址')}
                  onChange={(e) => setHostQuery(e.target.value)}
                />
              </div>
              <div className="tabs" role="group">
                <button aria-pressed={statusFilter === 'all'} onClick={() => setStatusFilter('all')}>
                  {t('全部')} <span className="tab-count num">{state.hosts.length}</span>
                </button>
                <button aria-pressed={statusFilter === 'online'} onClick={() => setStatusFilter('online')}>
                  {t('在线')}
                </button>
                {issueCount > 0 && (
                  <button aria-pressed={statusFilter === 'issue'} onClick={() => setStatusFilter('issue')}>
                    {t('异常')} <span className="tab-count tab-count-warn num">{issueCount}</span>
                  </button>
                )}
              </div>
            </div>

            {filteredHosts.length === 0 ? (
              <div className="panel-pad">
                <p className="small muted">{t('没有匹配的机器。')}</p>
              </div>
            ) : (
              <div className="table-wrap">
                <table className="hosts">
                  <thead>
                    <tr>
                      <th>{t('机器')}</th>
                      <th>{t('状态')}</th>
                      <th>{t('节点')}</th>
                      <th className="num">{t('可用')}</th>
                      <th className="num">{t('平均延迟')}</th>
                      <th>{t('数据来源')}</th>
                      <th aria-label={t('操作')} />
                    </tr>
                  </thead>
                  <tbody>
                    {filteredHosts.map((h) => {
                      const all = state.nodes.filter((n) => n.hostId === h.id)
                      const nodes = all.filter((n) => n.alive)
                      const avg = nodes.length
                        ? Math.round(nodes.reduce((a, n) => a + n.delayMs, 0) / nodes.length)
                        : 0
                      const tone = avg ? delayTone(true, avg) : 'muted'
                      return (
                        <tr
                          key={h.id}
                          className="click"
                          tabIndex={0}
                          role="button"
                          aria-label={t('查看 {{name}} 的详情', { name: h.name })}
                          onClick={() => openHost(h.id)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' || e.key === ' ') {
                              e.preventDefault()
                              openHost(h.id)
                            }
                          }}
                        >
                          <td>
                            <div className="cell-title-row">
                              <span className="cell-main">{h.name}</span>
                              <span className="badge">{t(KIND_COPY[h.kind]?.short ?? h.kind)}</span>
                            </div>
                            <span className="cell-sub mono truncate host-addr">
                              {h.kind === 'static'
                                ? 'hy2 / v2ray'
                                : h.kind === 'sub'
                                  ? h.subUrl || '—'
                                  : h.controller || '—'}
                            </span>
                            {h.subError && (
                              <span className="cell-sub is-bad">
                                {t('上次拉取失败：{{error}}', { error: h.subError })}
                              </span>
                            )}
                          </td>
                          <td>
                            <StatusPill status={h.status} />
                          </td>
                          <td>
                            <NodeThreads nodes={all} />
                          </td>
                          <td className="num">
                            {h.aliveCount}
                            <span className="muted"> / {h.nodeCount}</span>
                          </td>
                          <td className="num">
                            {avg ? (
                              <span className={tone === 'ok' ? 'is-good' : tone === 'warn' ? 'is-warn' : undefined}>
                                {avg} ms
                              </span>
                            ) : (
                              '—'
                            )}
                          </td>
                          <td className="small muted">
                            {h.kind === 'sub'
                              ? h.subCheckedAt
                                ? t('订阅 · {{ago}}', { ago: ago(h.subCheckedAt) })
                                : t('订阅 · 未拉取')
                              : h.kind === 'static'
                                ? h.lastSeen
                                  ? t('配置 · {{ago}}', { ago: ago(h.lastSeen) })
                                  : t('配置 · 未上报')
                                : h.lastSeen
                                  ? t('mihomo · {{ago}}', { ago: ago(h.lastSeen) })
                                  : t('mihomo · 未上报')}
                          </td>
                          <td className="num row-chevron">
                            <Icon name="chevron" size={15} />
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </section>

      <details className="disclosure panel" open={state.hosts.length === 0}>
        <summary>
          <span className="disclosure-title">
            <Icon name="key" size={15} />
            <span>{t('两种接入方式')}</span>
          </span>
          <span className="disclosure-meta small muted">{t('token 在添加机器时生成一次')}</span>
          <Icon name="chevronDown" size={15} className="disclosure-chevron" />
        </summary>
        <div className="disclosure-body">
          <div className="grid two">
            <div>
              <p>{t('agent 只读本机，节点与凭据不出机器；上报是出站请求，NAT 后也能用。')}</p>
              <ul className="facts">
                <li>
                  <span>{t('mihomo 机器')}</span>
                  <span>{t('读 external-controller，节点与延迟实时上报')}</span>
                </li>
                <li>
                  <span>{t('hy2 / v2ray 机器')}</span>
                  <span>{t('读 hysteria / xray 配置，反推出密码、SNI、Reality 公钥')}</span>
                </li>
                <li>
                  <span>{t('上报周期')}</span>
                  <span>{t('5 分钟读一次配置；有变化即报，无变化每小时心跳')}</span>
                </li>
              </ul>
            </div>
            <div>
              <pre>
                <code>{`# ${t('只跑服务的机器')}
weft agent --server ${state.publicBase} \\
  --token <host token> \\
  --hysteria-config /etc/hysteria/config.yaml \\
  --xray-config /usr/local/etc/xray/config.json \\
  --public-host <public IP>`}</code>
              </pre>
              <Copy
                label={t('复制模板')}
                value={`weft agent --server ${state.publicBase} --token <host token> --hysteria-config /etc/hysteria/config.yaml --xray-config /usr/local/etc/xray/config.json --public-host <public IP>`}
              />
            </div>
          </div>
        </div>
      </details>

      {adding && (
        <Modal
          title={t('添加机器')}
          wide
          onClose={() => setAdding(false)}
          footer={
            <>
              <button className="quiet" onClick={() => setAdding(false)}>
                {t('取消')}
              </button>
              <button
                className="primary"
                onClick={() => void create()}
                disabled={busy || !name.trim() || (kind === 'sub' && !url.trim())}
              >
                {kind === 'sub' ? t('拉取并创建') : t('生成 token')}
              </button>
            </>
          }
        >
          <div className="choice">
            {(Object.keys(KIND_COPY) as HostKind[]).map((key) => (
              <button
                key={key}
                type="button"
                aria-pressed={kind === key}
                onClick={() => setKind(key)}
              >
                <span className="choice-icon">
                  <Icon name={KIND_COPY[key].icon} size={16} />
                </span>
                <strong>{t(KIND_COPY[key].label)}</strong>
                <span>{t(KIND_COPY[key].blurb)}</span>
              </button>
            ))}
          </div>
          <label className="field">
            <span>{t('机器名')}</span>
            <input
              autoFocus
              value={name}
              placeholder={kind === 'sub' ? 'my-airport' : 'hk-01'}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && void create()}
            />
            <span className="hint">{t('用作订阅里的节点前缀，不要含空格或斜杠。')}</span>
          </label>
          {kind === 'sub' && (
            <label className="field">
              <span>{t('订阅地址')}</span>
              <input
                value={url}
                placeholder="https://example.com/api/v1/client/subscribe?token=…"
                onChange={(e) => setURL(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && void create()}
              />
              <span className="hint">
                {t('支持 Clash 配置、base64 链接列表和明文链接；每 6 小时自动重拉一次。')}
              </span>
            </label>
          )}
        </Modal>
      )}

      {created && (
        <Modal
          title={t('{{name}} · 接入命令', { name: created.name })}
          onClose={() => setCreated(null)}
          wide
          footer={
            <button className="primary" onClick={() => setCreated(null)}>
              {t('完成')}
            </button>
          }
        >
          <p>
            {t('token 只显示这一次。{{hint}}', {
              hint:
                created.kind === 'static'
                  ? t('把两条配置路径换成机器上的真实路径，公网 IP 填这台机器对外可达的地址。')
                  : t('把它和 mihomo 的 secret 一起写进主机上的 systemd 或 launchd 里。'),
            })}
          </p>
          <pre>
            <code>{agentCommand(state.publicBase, created.token ?? '', created.kind)}</code>
          </pre>
          <Copy value={agentCommand(state.publicBase, created.token ?? '', created.kind)} label={t('复制命令')} />
        </Modal>
      )}

      {selected && (
        <Drawer
          title={selected.name}
          subtitle={t(KIND_COPY[selected.kind]?.label ?? selected.kind)}
          onClose={closeHost}
        >
          <div className="drawer-summary">
            <div>
              <span className="small muted">{t('状态')}</span>
              <div>
                <StatusPill status={selected.status} />
              </div>
            </div>
            <div>
              <span className="small muted">{t('可用节点')}</span>
              <strong className="num">
                {selected.aliveCount} / {selected.nodeCount}
              </strong>
            </div>
            <div>
              <span className="small muted">{selected.kind === 'sub' ? t('最后拉取') : t('最后上报')}</span>
              <strong className="num">
                {selected.kind === 'sub'
                  ? selected.subCheckedAt
                    ? ago(selected.subCheckedAt)
                    : t('从未')
                  : selected.lastSeen
                    ? ago(selected.lastSeen)
                    : t('从未')}
              </strong>
            </div>
          </div>

          {(selected.kind === 'agent' || selected.kind === 'sub') && (
            <dl className="kv">
              {selected.kind === 'agent' && (
                <>
                  <dt>mihomo</dt>
                  <dd className="mono">{selected.mihomoVersion || '—'}</dd>
                  <dt>controller</dt>
                  <dd className="mono">{selected.controller || '—'}</dd>
                </>
              )}
              {selected.kind === 'sub' && (
                <>
                  <dt>{t('订阅地址')}</dt>
                  <dd className="mono small">{selected.subUrl || '—'}</dd>
                </>
              )}
            </dl>
          )}

          {selected.kind === 'sub' ? (
            <div className="drawer-section">
              {selected.subError && (
                <p className="error">{t('上次拉取失败：{{error}}', { error: selected.subError })}</p>
              )}
              <div className="actions">
                <button
                  className="quiet small"
                  disabled={busy}
                  onClick={() => void refreshSource(selected)}
                >
                  <Icon name="refresh" size={14} className={busy ? 'spin' : undefined} />
                  <span>{t('现在重新拉取')}</span>
                </button>
                <button
                  className="danger small push"
                  onClick={() => setConfirmAction({ type: 'delete-host', host: selected })}
                >
                  <Icon name="trash" size={14} />
                  <span>{t('删除订阅源')}</span>
                </button>
              </div>
            </div>
          ) : (
            <div className="drawer-section">
              <div className="section-head">
                <h3>{t('接入命令')}</h3>
                {freshToken && (
                  <Copy value={agentCommand(state.publicBase, freshToken, selected.kind)} label={t('复制命令')} />
                )}
              </div>
              {freshToken ? (
                <pre>
                  <code>{agentCommand(state.publicBase, freshToken, selected.kind)}</code>
                </pre>
              ) : (
                <>
                  <p className="small muted">
                    {t('token 只在生成时显示。下面这条命令里的 token 要换成轮换后拿到的那一个。')}
                  </p>
                  <pre>
                    <code>{agentCommand(state.publicBase, '<host token>', selected.kind)}</code>
                  </pre>
                </>
              )}
              <div className="actions">
                <button
                  className="quiet small"
                  onClick={() => setConfirmAction({ type: 'rotate', host: selected })}
                >
                  <Icon name="key" size={14} />
                  <span>{t('轮换 token')}</span>
                </button>
                <button
                  className="danger small push"
                  onClick={() => setConfirmAction({ type: 'delete-host', host: selected })}
                >
                  <Icon name="trash" size={14} />
                  <span>{t('删除机器')}</span>
                </button>
              </div>
            </div>
          )}

          <div className="drawer-section">
            <div className="section-head">
              <h3>{t('节点（{{n}}）', { n: selectedNodes.length })}</h3>
              <div className="actions">
                <button className="quiet small" onClick={() => setNodeMode('import')}>
                  {t('粘贴导入')}
                </button>
                <button
                  className="primary small"
                  onClick={() => {
                    setEditing(null)
                    setNodeMode('form')
                  }}
                >
                  <Icon name="plus" size={14} />
                  <span>{t('手工添加')}</span>
                </button>
              </div>
            </div>
            {selectedNodes.length > 5 && (
              <div className="search-box drawer-search">
                <Icon name="search" size={14} className="search-icon" />
                <input
                  type="search"
                  value={nodeQuery}
                  placeholder={t('过滤节点')}
                  onChange={(e) => setNodeQuery(e.target.value)}
                />
              </div>
            )}
            <NodeTable
              nodes={filteredSelectedNodes}
              compact
              onEdit={(node) => {
                setEditing(node)
                setNodeMode('form')
              }}
              onDelete={(node) => setConfirmAction({ type: 'delete-node', node })}
            />
          </div>
        </Drawer>
      )}

      {selected && nodeMode === 'form' && (
        <Modal
          title={editing ? t('编辑 {{name}}', { name: editing.name }) : t('添加节点 · {{name}}', { name: selected.name })}
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
                notify(editing ? t('节点已更新') : t('节点已添加'), 'ok')
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
        <Modal title={t('粘贴导入 · {{name}}', { name: selected.name })} wide onClose={closeNodeEditor}>
          <p>
            {t('一行一条，支持')} <span className="mono">hy2://</span>{' '}
            <span className="mono">vless://</span> <span className="mono">trojan://</span>{' '}
            <span className="mono">ss://</span> <span className="mono">vmess://</span>。
            {t('重复导入同一条只会更新，不会新增。')}
          </p>
          <LinkImport hostId={selected.id} onDone={refresh} onCancel={closeNodeEditor} />
        </Modal>
      )}

      {confirmAction && (
        <ConfirmModal
          title={confirmAction.type === 'rotate' ? t('确认轮换') : t('确认删除')}
          message={
            confirmAction.type === 'rotate'
              ? t('轮换 {{name}} 的 token？旧命令会立即失效。', { name: confirmAction.host.name })
              : confirmAction.type === 'delete-host'
                ? t('删除 {{name}}？它的节点会同时从所有编排中移除。', { name: confirmAction.host.name })
                : t('删除节点 {{name}}？它会从所有编排里移除。', { name: confirmAction.node.name })
          }
          confirmLabel={confirmAction.type === 'rotate' ? t('立即轮换') : t('删除')}
          busy={busy}
          onConfirm={runConfirmed}
          onClose={() => setConfirmAction(null)}
        />
      )}
    </>
  )
}
