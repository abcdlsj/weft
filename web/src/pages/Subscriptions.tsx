import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ago, api, type Subscription } from '../api'
import { Icon } from '../icons'
import { Copy, Empty, Modal, Switch, notify } from '../ui'
import type { PageProps } from '../App'

const formatLabel: Record<string, string> = {
  mihomo: 'mihomo YAML',
  base64: 'base64 链接',
  text: '明文链接',
}

function randomBearer() {
  const bytes = crypto.getRandomValues(new Uint8Array(24))
  return [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('')
}

export function Subscriptions({ state, refresh }: PageProps) {
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
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function patch(
    s: Subscription,
    body: { enabled?: boolean; format?: string; bearer?: string; rotate?: boolean },
  ) {
    const res = await api.patchSubscription(s.id, body)
    if (res.url) setFresh({ label: s.label || t('未命名'), url: res.url })
    notify(t('已更新'))
    await refresh()
  }

  async function remove(s: Subscription) {
    if (!confirm(t('删除订阅「{{name}}」？客户端将无法继续拉取。', { name: s.label || t('未命名') }))) return
    await api.deleteSubscription(s.id)
    notify(t('订阅已删除'))
    await refresh()
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>{t('订阅')}</h1>
          <p className="sub">{t('一条链接对应一条编排。')}</p>
        </div>
        <div className="title-actions">
          <button
            className="btn btn--primary"
            onClick={() => setCreating(true)}
            disabled={state.bundles.length === 0}
          >
            <Icon name="plus" />
            {t('新建订阅')}
          </button>
        </div>
      </div>

      <section className="panel">
        {state.subscriptions.length === 0 ? (
          <Empty
            title={t('还没有订阅链接')}
            body={
              state.bundles.length
                ? t('从一条编排生成链接，粘进客户端即可。')
                : t('先创建一条编排，再生成订阅链接。')
            }
          />
        ) : (
          <ul className="subs">
            {state.subscriptions.map((s) => (
              <li key={s.id} className={s.enabled ? 'subs__row' : 'subs__row subs__row--off'}>
                <div className="subs__main">
                  <div className="subs__title">
                    <span className="cell-main">{s.label || t('未命名')}</span>
                    <span className="pill pill--neutral">{t(formatLabel[s.format] ?? s.format)}</span>
                    {!s.enabled && <span className="pill pill--warn">{t('已停用')}</span>}
                  </div>
                  <div className="subs__url">
                    <code className="mono">{url(s)}</code>
                    <Copy value={url(s)} />
                  </div>
                  <div className="subs__meta">
                    <span>{t('编排 {{name}}', { name: s.bundleName })}</span>
                    <span>{t('拉取 {{n}} 次', { n: s.hits })}</span>
                    <span>{s.lastUsedAt ? t('最近 {{ago}}', { ago: ago(s.lastUsedAt) }) : t('还没被拉取过')}</span>
                  </div>
                  <div className="devices">
                    {(s.devices ?? []).length === 0 ? (
                      <span className="devices__empty">{t('还没有设备拉取过这条链接')}</span>
                    ) : (
                      (s.devices ?? []).map((d) => (
                        <span
                          key={d.device}
                          className="device"
                          title={`${d.ua || d.device}
${t('IP {{ip}} · 最近 {{ago}} · 共 {{n}} 次', { ip: d.ip, ago: ago(d.lastAt), n: d.hits })}`}
                        >
                          <Icon name="device" />
                          <span className="device__name">{d.device}</span>
                          <span className="device__meta">
                            {t('{{n}} 次 · {{ago}}', { n: d.hits, ago: ago(d.lastAt) })}
                          </span>
                        </span>
                      ))
                    )}
                  </div>
                </div>
                <div className="subs__actions">
                  <Switch
                    checked={s.enabled}
                    label={s.enabled ? t('停用订阅') : t('启用订阅')}
                    onChange={() => void patch(s, { enabled: !s.enabled })}
                  />
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
                    className="btn btn--quiet btn--sm"
                    onClick={() => {
                      setAuthSub(s)
                      setBearerDraft(s.bearer)
                    }}
                  >
                    {t('鉴权')}
                  </button>
                  <button className="icon-btn" aria-label={t('二维码')} onClick={() => setQr(s)}>
                    <Icon name="qr" />
                  </button>
                  <button
                    className="icon-btn"
                    aria-label={t('轮换地址')}
                    onClick={() => {
                      if (confirm(t('轮换地址？旧链接会立刻失效。'))) void patch(s, { rotate: true })
                    }}
                  >
                    <Icon name="key" />
                  </button>
                  <button className="icon-btn" aria-label={t('删除')} onClick={() => void remove(s)}>
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
          <h2>{t('客户端怎么填')}</h2>
          <span className="meta">{t('三种输出格式的区别')}</span>
        </header>
        <ul className="facts facts--defs">
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
            <span>{t('一行一个')} <span className="mono">ss://</span> {t('或')} <span className="mono">vmess://</span>{t('，便于脚本处理。')}</span>
          </li>
        </ul>
        <p className="prose muted">
          {t('轮换后旧地址立即失效。同型号设备想分开统计，在链接后加')}{' '}
          <code className="mono">?tag=living-room AppleTV</code>。
        </p>
      </section>

      {authSub && (
        <Modal
          title={`${authSub.label || t('未命名')} · ${t('鉴权')}`}
          onClose={() => setAuthSub(null)}
          footer={
            <>
              <button className="btn btn--quiet" onClick={() => setAuthSub(null)}>
                {t('取消')}
              </button>
              <button
                className="btn btn--primary"
                onClick={() => {
                  void patch(authSub, { bearer: bearerDraft })
                    .then(() => {
                      notify(t('鉴权已更新'))
                      setAuthSub(null)
                    })
                    .catch((e) => notify((e as Error).message))
                }}
              >
                {t('保存鉴权')}
              </button>
            </>
          }
        >
          <p className="prose">{t('这条链接支持两种凭据：URL 里的 key，或请求头 Authorization: Bearer。')}</p>
          <label className="field">
            <span>{t('访问密钥')}（URL）</span>
            <input className="mono" readOnly value={authSub.accessKey} />
          </label>
          <label className="field">
            <span>{t('Bearer token')}</span>
            <input
              className="mono"
              value={bearerDraft}
              onChange={(e) => setBearerDraft(e.target.value)}
            />
            <span className="field__hint">{t('自动生成，也可以自己填。至少 8 位。')}</span>
          </label>
          <div className="row-actions">
            <button
              className="btn btn--quiet btn--sm"
              onClick={() => setBearerDraft(randomBearer())}
            >
              {t('重新生成')}
            </button>
            <Copy value={`Authorization: Bearer ${bearerDraft}`} label={t('复制')} />
          </div>
          <p className="prose muted">{t('轮换会同时换掉 key 与 bearer token。')}</p>
        </Modal>
      )}

      {creating && (
        <Modal
          title={t('新建订阅')}
          onClose={() => setCreating(false)}
          footer={
            <>
              <button className="btn btn--quiet" onClick={() => setCreating(false)}>
                {t('取消')}
              </button>
              <button className="btn btn--primary" onClick={() => void create()} disabled={busy || !bundleID}>
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
            <input
              value={label}
              placeholder={t('主力订阅')}
              onChange={(e) => setLabel(e.target.value)}
            />
            <span className="field__hint">{t('方便区分设备或客户端，可以不填。')}</span>
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
            <button className="btn btn--primary" onClick={() => setFresh(null)}>
              {t('完成')}
            </button>
          }
        >
          <p className="prose">{t('把它粘进 mihomo、Clash Verge 或 sing-box 的订阅地址里。')}</p>
          <pre className="code">{fresh.url}</pre>
          <div className="row-actions">
            <Copy value={fresh.url} label={t('复制链接')} />
          </div>
        </Modal>
      )}

      {qr && (
        <Modal title={t('{{name}} · 二维码', { name: qr.label || t('未命名') })} onClose={() => setQr(null)}>
          <div className="qr">
            <img src={`/api/subscriptions/${qr.id}/qr.png?size=360`} alt={t('订阅二维码')} width={280} height={280} />
            <code className="mono">{url(qr)}</code>
          </div>
        </Modal>
      )}
    </>
  )
}
