import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ago, api, type Subscription } from '../api'
import { Icon } from '../icons'
import { ConfirmModal, Copy, Empty, Modal, Switch, notify } from '../ui'
import type { PageProps } from '../App'

const formatLabel: Record<string, string> = {
  mihomo: 'mihomo YAML',
  base64: 'base64 链接',
  text: '明文链接',
}

function clashLink(url: string) {
  return `clash://install-config?url=${encodeURIComponent(url)}`
}

function shadowrocketLink(url: string, name: string) {
  return `shadowrocket://add/sub://${encodeURIComponent(btoa(url))}?remark=${encodeURIComponent(name)}`
}

function singboxLink(url: string, name: string) {
  return `sing-box://import-remote-profile?url=${encodeURIComponent(url)}#${encodeURIComponent(name)}`
}

function randomBearer() {
  const bytes = crypto.getRandomValues(new Uint8Array(24))
  return [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('')
}

function ClientLink({
  label,
  value,
  openable = true,
}: {
  label: string
  value: string
  openable?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className="client-link">
      <div className="client-link-head">
        <span>{label}</span>
        <div className="actions">
          {openable && (
            <a className="btn-link quiet small" href={value}>
              <Icon name="arrowUpRight" size={14} />
              <span>{t('打开客户端')}</span>
            </a>
          )}
          <Copy value={value} />
        </div>
      </div>
      <code className="mono">{value}</code>
    </div>
  )
}

type ConfirmAction =
  | { type: 'rotate'; sub: Subscription }
  | { type: 'delete'; sub: Subscription }

export function Subscriptions({ state, refresh, navigate, search }: PageProps) {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [bundleID, setBundleID] = useState(state.bundles[0]?.id ?? '')
  const [label, setLabel] = useState('')
  const [format, setFormat] = useState('mihomo')
  const [busy, setBusy] = useState(false)
  const [qr, setQr] = useState<Subscription | null>(null)
  const [fresh, setFresh] = useState<{ label: string; url: string } | null>(null)
  const [authSub, setAuthSub] = useState<Subscription | null>(null)
  const [bearerDraft, setBearerDraft] = useState('')
  const [renaming, setRenaming] = useState<Subscription | null>(null)
  const [labelDraft, setLabelDraft] = useState('')
  const [sharing, setSharing] = useState<Subscription | null>(null)
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  useEffect(() => {
    const createBundle = new URLSearchParams(search).get('create')
    if (createBundle && state.bundles.some((b) => b.id === createBundle)) {
      setBundleID(createBundle)
      setCreating(true)
      navigate('/subscriptions')
    }
  }, [search, state.bundles, navigate])

  const url = (s: Subscription) =>
    `${state.publicBase}/sub/${s.token}${s.accessKey ? `?key=${s.accessKey}` : ''}`

  async function create() {
    if (!bundleID) return
    setBusy(true)
    try {
      const res = await api.createSubscription({ bundleId: bundleID, label: label || t('未命名'), format })
      setCreating(false)
      setLabel('')
      setFresh({ label: res.subscription.label, url: res.url })
      await refresh()
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }

  async function patch(
    s: Subscription,
    body: { label?: string; enabled?: boolean; format?: string; bearer?: string; rotate?: boolean },
  ) {
    const res = await api.patchSubscription(s.id, body)
    if (res.url) setFresh({ label: s.label || t('未命名'), url: res.url })
    notify(t('已更新'), 'ok')
    await refresh()
  }

  async function runConfirmed() {
    if (!confirmAction) return
    setBusy(true)
    try {
      if (confirmAction.type === 'rotate') {
        await patch(confirmAction.sub, { rotate: true })
      } else {
        await api.deleteSubscription(confirmAction.sub.id)
        notify(t('订阅已删除'), 'ok')
        await refresh()
      }
      setConfirmAction(null)
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
          <h1>{t('{{n}} 条订阅', { n: state.subscriptions.length })}</h1>
          <p className="sub">{t('一条链接对应一条编排。')}</p>
        </div>
        <div className="actions">
          <button
            className="primary"
            onClick={() => {
              if (!bundleID && state.bundles[0]) setBundleID(state.bundles[0].id)
              setCreating(true)
            }}
            disabled={state.bundles.length === 0}
          >
            <Icon name="plus" size={15} />
            <span>{t('新建订阅')}</span>
          </button>
        </div>
      </div>

      <section>
        {state.subscriptions.length === 0 ? (
          <div className="panel">
            <Empty
              icon="link"
              title={t('还没有订阅链接')}
              body={
                state.bundles.length
                  ? t('从一条编排生成链接，粘进客户端即可。')
                  : t('先创建一条编排，再生成订阅链接。')
              }
              action={
                state.bundles.length > 0 ? (
                  <button className="primary" onClick={() => setCreating(true)}>
                    <Icon name="plus" size={15} />
                    <span>{t('新建订阅')}</span>
                  </button>
                ) : (
                  <button className="primary" onClick={() => navigate('/bundles/new')}>
                    <Icon name="plus" size={15} />
                    <span>{t('新建编排')}</span>
                  </button>
                )
              }
            />
          </div>
        ) : (
          <ul className="sub-list">
            {state.subscriptions.map((s) => {
              const link = url(s)
              return (
                <li key={s.id} className="sub-card panel" data-off={!s.enabled}>
                  <div className="sub-card-top">
                    <div className="sub-head">
                      <span className="name">{s.label || t('未命名')}</span>
                      <span className="badge badge-accent">{s.bundleName}</span>
                      <span className="badge">{t(formatLabel[s.format] ?? s.format)}</span>
                      {!s.enabled && <span className="badge badge-warn">{t('已停用')}</span>}
                    </div>
                    <div className="sub-quick">
                      <button
                        type="button"
                        className="quiet small"
                        onClick={() => setSharing(s)}
                      >
                        <Icon name="share" size={14} />
                        <span>{t('导入到客户端')}</span>
                      </button>
                      <button
                        type="button"
                        className="quiet small icon-btn"
                        aria-label={t('二维码')}
                        title={t('二维码')}
                        onClick={() => setQr(s)}
                      >
                        <Icon name="qr" size={15} />
                      </button>
                      <Switch
                        checked={s.enabled}
                        label={s.enabled ? t('停用订阅') : t('启用订阅')}
                        onChange={() => void patch(s, { enabled: !s.enabled })}
                      />
                    </div>
                  </div>

                  <div className="sub-url">
                    <code
                      title={link}
                      onClick={(e) => {
                        const range = document.createRange()
                        range.selectNodeContents(e.currentTarget)
                        const sel = window.getSelection()
                        sel?.removeAllRanges()
                        sel?.addRange(range)
                      }}
                    >
                      {link}
                    </code>
                    <Copy value={link} label={t('复制链接')} />
                  </div>

                  <div className="sub-info-row">
                    <div className="sub-meta">
                      <span className="num">{t('拉取 {{n}} 次', { n: s.hits })}</span>
                      <span>
                        {s.lastUsedAt ? t('最近 {{ago}}', { ago: ago(s.lastUsedAt) }) : t('还没被拉取过')}
                      </span>
                    </div>
                    <div className="devices">
                      {(s.devices ?? []).length === 0 ? (
                        <span className="muted">{t('还没有设备拉取过这条链接')}</span>
                      ) : (
                        (s.devices ?? []).map((d) => (
                          <span
                            key={d.device}
                            className="device-chip"
                            title={`${d.ua || d.device}\n${t('IP {{ip}} · 最近 {{ago}} · 共 {{n}} 次', {
                              ip: d.ip,
                              ago: ago(d.lastAt),
                              n: d.hits,
                            })}`}
                          >
                            <Icon name="device" size={12} />
                            <span>{d.device}</span>
                            <em className="num">
                              {t('{{n}} 次 · {{ago}}', { n: d.hits, ago: ago(d.lastAt) })}
                            </em>
                          </span>
                        ))
                      )}
                    </div>
                  </div>

                  <div className="sub-actions">
                    <select
                      value={s.format}
                      aria-label={t('输出格式')}
                      onChange={(e) => void patch(s, { format: e.target.value })}
                    >
                      <option value="mihomo">mihomo</option>
                      <option value="base64">base64</option>
                      <option value="text">{t('明文')}</option>
                    </select>
                    <button
                      className="quiet small"
                      onClick={() => {
                        setRenaming(s)
                        setLabelDraft(s.label)
                      }}
                    >
                      <Icon name="pencil" size={13} />
                      <span>{t('重命名')}</span>
                    </button>
                    <button
                      className="quiet small"
                      onClick={() => {
                        setAuthSub(s)
                        setBearerDraft(s.bearer)
                      }}
                    >
                      <Icon name="key" size={13} />
                      <span>{t('鉴权')}</span>
                    </button>
                    <button
                      className="quiet small"
                      onClick={() => setConfirmAction({ type: 'rotate', sub: s })}
                    >
                      <Icon name="refresh" size={13} />
                      <span>{t('轮换地址')}</span>
                    </button>
                    <button
                      className="danger small push"
                      onClick={() => setConfirmAction({ type: 'delete', sub: s })}
                    >
                      <Icon name="trash" size={13} />
                      <span>{t('删除')}</span>
                    </button>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      <details className="disclosure panel" open={state.subscriptions.length === 0}>
        <summary>
          <span className="disclosure-title">
            <Icon name="link" size={15} />
            <span>{t('客户端怎么填')}</span>
          </span>
          <span className="disclosure-meta small muted">{t('三种输出格式的区别')}</span>
          <Icon name="chevronDown" size={15} className="disclosure-chevron" />
        </summary>
        <div className="disclosure-body">
          <ul className="facts">
            <li>
              <span>mihomo YAML</span>
              <span>{t('整份配置，直接作为 profile 地址。')}</span>
            </li>
            <li>
              <span>{t('base64 链接')}</span>
              <span>{t('节点列表，给只吃节点的客户端。')}</span>
            </li>
            <li>
              <span>{t('明文链接')}</span>
              <span>
                {t('一行一个')} <code className="mono">ss://</code> {t('或')} <code className="mono">vmess://</code>
                {t('，便于脚本处理。')}
              </span>
            </li>
          </ul>
          <p className="small muted disclosure-note">
            {t('轮换后旧地址立即失效。同型号设备想分开统计，在链接后加')}{' '}
            <code>?tag=living-room AppleTV</code>。
          </p>
        </div>
      </details>

      {renaming && (
        <Modal
          title={t('重命名订阅')}
          onClose={() => setRenaming(null)}
          footer={
            <>
              <button className="quiet" onClick={() => setRenaming(null)}>
                {t('取消')}
              </button>
              <button
                className="primary"
                disabled={!labelDraft.trim()}
                onClick={() => {
                  void patch(renaming, { label: labelDraft.trim() })
                    .then(() => setRenaming(null))
                    .catch((e) => notify((e as Error).message, 'bad'))
                }}
              >
                {t('保存')}
              </button>
            </>
          }
        >
          <label className="field">
            <span>{t('名称')}</span>
            <input
              autoFocus
              value={labelDraft}
              placeholder={t('主力订阅')}
              onChange={(e) => setLabelDraft(e.target.value)}
            />
            <span className="hint">{t('客户端下次拉取时按这个名称显示 profile。')}</span>
          </label>
        </Modal>
      )}

      {sharing && (
        <Modal
          title={`${sharing.label || t('未命名')} · ${t('导入到客户端')}`}
          onClose={() => setSharing(null)}
          wide
        >
          <p className="small muted">{t('链接里已经包含访问凭据；不要把它发到公开的地方。')}</p>
          <ClientLink label="Clash Verge / mihomo" value={clashLink(url(sharing))} />
          <ClientLink label="Shadowrocket" value={shadowrocketLink(url(sharing), sharing.label || t('未命名'))} />
          <ClientLink label="sing-box" value={singboxLink(url(sharing), sharing.label || t('未命名'))} />
          <ClientLink label={t('订阅地址')} value={url(sharing)} openable={false} />
        </Modal>
      )}

      {authSub && (
        <Modal
          title={`${authSub.label || t('未命名')} · ${t('鉴权')}`}
          onClose={() => setAuthSub(null)}
          footer={
            <>
              <button className="quiet" onClick={() => setAuthSub(null)}>
                {t('取消')}
              </button>
              <button
                className="primary"
                onClick={() => {
                  void patch(authSub, { bearer: bearerDraft })
                    .then(() => {
                      notify(t('鉴权已更新'), 'ok')
                      setAuthSub(null)
                    })
                    .catch((e) => notify((e as Error).message, 'bad'))
                }}
              >
                {t('保存鉴权')}
              </button>
            </>
          }
        >
          <p>{t('这条链接支持两种凭据：URL 里的 key，或请求头 Authorization: Bearer。')}</p>
          <label className="field">
            <span>{t('访问密钥')}（URL）</span>
            <input className="mono" readOnly value={authSub.accessKey} />
          </label>
          <label className="field">
            <span>Bearer token</span>
            <input className="mono" value={bearerDraft} onChange={(e) => setBearerDraft(e.target.value)} />
            <span className="hint">{t('自动生成，也可以自己填。至少 8 位。')}</span>
          </label>
          <div className="actions">
            <button className="quiet small" onClick={() => setBearerDraft(randomBearer())}>
              <Icon name="refresh" size={14} />
              <span>{t('重新生成')}</span>
            </button>
            <Copy value={`Authorization: Bearer ${bearerDraft}`} label={t('复制')} />
          </div>
          <p className="small muted disclosure-note">{t('轮换会同时换掉 key 与 bearer token。')}</p>
        </Modal>
      )}

      {creating && (
        <Modal
          title={t('新建订阅')}
          onClose={() => setCreating(false)}
          footer={
            <>
              <button className="quiet" onClick={() => setCreating(false)}>
                {t('取消')}
              </button>
              <button className="primary" onClick={() => void create()} disabled={busy || !bundleID}>
                {t('生成链接')}
              </button>
            </>
          }
        >
          <label className="field">
            <span>{t('编排')}</span>
            <select value={bundleID} onChange={(e) => setBundleID(e.target.value)}>
              {state.bundles.map((b) => (
                <option key={b.id} value={b.id}>
                  {t('{{name}}（{{n}} 节点）', { name: b.name, n: b.members.length })}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>{t('名称')}</span>
            <input value={label} placeholder={t('主力订阅')} onChange={(e) => setLabel(e.target.value)} />
            <span className="hint">{t('方便区分设备或客户端，可以不填。')}</span>
          </label>
          <label className="field">
            <span>{t('输出格式')}</span>
            <select value={format} onChange={(e) => setFormat(e.target.value)}>
              <option value="mihomo">{t('mihomo YAML（完整配置）')}</option>
              <option value="base64">{t('base64 链接列表')}</option>
              <option value="text">{t('明文链接列表')}</option>
            </select>
          </label>
        </Modal>
      )}

      {fresh && (
        <Modal
          title={t('订阅链接')}
          onClose={() => setFresh(null)}
          wide
          footer={
            <button className="primary" onClick={() => setFresh(null)}>
              {t('完成')}
            </button>
          }
        >
          <p>{t('把它粘进 mihomo、Clash Verge 或 sing-box 的订阅地址里。')}</p>
          <pre>
            <code>{fresh.url}</code>
          </pre>
          <Copy value={fresh.url} label={t('复制链接')} />
        </Modal>
      )}

      {qr && (
        <Modal title={t('{{name}} · 二维码', { name: qr.label || t('未命名') })} onClose={() => setQr(null)}>
          <div className="qr">
            <img src={`/api/subscriptions/${qr.id}/qr.png?size=360`} alt={t('订阅二维码')} width={264} height={264} />
            <code className="mono">{url(qr)}</code>
            <div className="qr-actions">
              <Copy value={url(qr)} label={t('复制链接')} />
            </div>
          </div>
        </Modal>
      )}

      {confirmAction && (
        <ConfirmModal
          title={confirmAction.type === 'rotate' ? t('确认轮换') : t('确认删除')}
          message={
            confirmAction.type === 'rotate'
              ? t('轮换地址？旧链接会立刻失效。')
              : t('删除订阅「{{name}}」？客户端将无法继续拉取。', {
                  name: confirmAction.sub.label || t('未命名'),
                })
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
