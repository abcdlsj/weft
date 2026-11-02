import { useMemo, useState } from 'react'
import { api, type ProxyNode } from './api'
import { Icon } from './icons'
import { notify } from './ui'

/* Protocol definitions drive the form. Adding a protocol means adding one entry
   here: the field list is the whole contract between panel and server. */

export type Protocol = 'hysteria2' | 'vless' | 'trojan' | 'ss'

interface FieldSpec {
  key: string
  label: string
  hint?: string
  placeholder?: string
  kind?: 'text' | 'password' | 'number' | 'switch' | 'list'
  required?: boolean
  options?: string[]
  wide?: boolean
  group?: string
}

export const PROTOCOLS: Record<Protocol, { label: string; blurb: string; fields: FieldSpec[] }> = {
  hysteria2: {
    label: 'Hysteria2',
    blurb: 'QUIC over UDP，抗丢包好，适合长距离线路',
    fields: [
      { key: 'name', label: '节点名', placeholder: 'Berlin-Hy2', required: true },
      { key: 'server', label: '服务器', placeholder: '203.0.113.24', required: true },
      { key: 'port', label: '端口', placeholder: '36906', kind: 'number', required: true },
      { key: 'password', label: '密码', kind: 'password', required: true },
      { key: 'sni', label: 'SNI', placeholder: 'www.bing.com', hint: '证书里的域名，自签时也要填' },
      {
        key: 'obfs',
        label: '混淆',
        kind: 'text',
        placeholder: 'salamander',
        hint: '留空表示不混淆',
        options: ['', 'salamander'],
      },
      { key: 'obfs-password', label: '混淆密码', kind: 'password' },
      { key: 'alpn', label: 'ALPN', kind: 'list', placeholder: 'h3', hint: '逗号分隔' },
      { key: 'up', label: '上行 Mbps', placeholder: '200 mbps' },
      { key: 'down', label: '下行 Mbps', placeholder: '1 gbps' },
      { key: 'skip-cert-verify', label: '跳过证书校验', kind: 'switch', hint: '自签证书时打开' },
    ],
  },
  vless: {
    label: 'VLESS',
    blurb: 'TCP + Reality 伪装，UDP 被限速时的兜底',
    fields: [
      { key: 'name', label: '节点名', placeholder: 'Berlin-Reality', required: true },
      { key: 'server', label: '服务器', placeholder: '203.0.113.24', required: true },
      { key: 'port', label: '端口', placeholder: '8443', kind: 'number', required: true },
      { key: 'uuid', label: 'UUID', required: true },
      {
        key: 'flow',
        label: 'Flow',
        placeholder: 'xtls-rprx-vision',
        hint: '与 xray 服务端一致',
        options: ['', 'xtls-rprx-vision'],
      },
      { key: 'servername', label: 'SNI', placeholder: 'www.bing.com' },
      {
        key: 'client-fingerprint',
        label: '指纹',
        placeholder: 'chrome',
        options: ['chrome', 'firefox', 'safari', 'ios', 'android', 'edge', 'random'],
      },
      { key: 'reality-opts.public-key', label: 'Reality 公钥', hint: '服务端私钥推导出的 publicKey' },
      { key: 'reality-opts.short-id', label: 'Short ID', placeholder: 'f8a64b66' },
      { key: 'network', label: '传输', placeholder: 'tcp', options: ['tcp', 'ws', 'grpc'] },
    ],
  },
  trojan: {
    label: 'Trojan',
    blurb: 'TLS 明文伪装，兼容性最广',
    fields: [
      { key: 'name', label: '节点名', required: true },
      { key: 'server', label: '服务器', required: true },
      { key: 'port', label: '端口', kind: 'number', required: true },
      { key: 'password', label: '密码', kind: 'password', required: true },
      { key: 'sni', label: 'SNI' },
      { key: 'skip-cert-verify', label: '跳过证书校验', kind: 'switch' },
    ],
  },
  ss: {
    label: 'Shadowsocks',
    blurb: '最轻量，速度取决于线路',
    fields: [
      { key: 'name', label: '节点名', required: true },
      { key: 'server', label: '服务器', required: true },
      { key: 'port', label: '端口', kind: 'number', required: true },
      { key: 'cipher', label: '加密方式', placeholder: 'aes-256-gcm', required: true },
      { key: 'password', label: '密码', kind: 'password', required: true },
    ],
  },
}

function dotGet(obj: unknown, path: string): unknown {
  return path
    .split('.')
    .reduce<unknown>((acc, key) => (acc && typeof acc === 'object' ? (acc as any)[key] : undefined), obj)
}

function dotSet(obj: Record<string, unknown>, path: string, value: unknown) {
  const keys = path.split('.')
  let cur = obj
  for (const key of keys.slice(0, -1)) {
    if (typeof cur[key] !== 'object' || cur[key] === null) cur[key] = {}
    cur = cur[key] as Record<string, unknown>
  }
  cur[keys[keys.length - 1]] = value
}

function initialValues(proxy: Record<string, unknown> | undefined, proto: Protocol) {
  const values: Record<string, string> = {}
  for (const field of PROTOCOLS[proto].fields) {
    const raw = proxy ? dotGet(proxy, field.key) : undefined
    if (Array.isArray(raw)) values[field.key] = raw.join(',')
    else if (typeof raw === 'boolean') values[field.key] = raw ? 'true' : 'false'
    else values[field.key] = raw == null ? '' : String(raw)
  }
  return values
}

function buildProxy(values: Record<string, string>, proto: Protocol, base?: Record<string, unknown>) {
  const proxy: Record<string, unknown> = { ...(base ?? {}), type: proto }
  for (const field of PROTOCOLS[proto].fields) {
    const raw = (values[field.key] ?? '').trim()
    if (field.kind === 'switch') {
      if (raw === 'true') dotSet(proxy, field.key, true)
      else deleteAt(proxy, field.key)
      continue
    }
    if (raw === '') {
      deleteAt(proxy, field.key)
      continue
    }
    if (field.kind === 'number') {
      dotSet(proxy, field.key, Number(raw))
    } else if (field.kind === 'list') {
      const list = raw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      if (list.length) dotSet(proxy, field.key, list)
    } else {
      dotSet(proxy, field.key, raw)
    }
  }
  return proxy
}

function deleteAt(obj: Record<string, any>, path: string) {
  const keys = path.split('.')
  let cur: Record<string, any> | undefined = obj
  for (const key of keys.slice(0, -1)) {
    cur = cur?.[key]
    if (!cur || typeof cur !== 'object') return
  }
  delete cur[keys[keys.length - 1]]
}

export function protocolOf(node: ProxyNode): Protocol {
  return (node.type as Protocol) in PROTOCOLS ? (node.type as Protocol) : 'hysteria2'
}

export function NodeForm({
  initial,
  initialProxy,
  onSubmit,
  onCancel,
  busy,
}: {
  initial?: Protocol
  initialProxy?: Record<string, unknown>
  onSubmit: (proxy: Record<string, unknown>) => Promise<void>
  onCancel: () => void
  busy?: boolean
}) {
  const [proto, setProto] = useState<Protocol>(initial ?? 'hysteria2')
  const [values, setValues] = useState<Record<string, string>>(() =>
    initialValues(initialProxy, initial ?? 'hysteria2'),
  )
  const [error, setError] = useState<string | null>(null)

  const spec = PROTOCOLS[proto]
  const missing = spec.fields.filter((f) => f.required && !(values[f.key] ?? '').trim())

  function switchProtocol(next: Protocol) {
    if (next === proto) return
    setProto(next)
    setValues(initialValues(undefined, next))
    setError(null)
  }

  async function submit() {
    if (missing.length) {
      setError(`还需要填写：${missing.map((f) => f.label).join('、')}`)
      return
    }
    setError(null)
    try {
      await onSubmit(buildProxy(values, proto, initialProxy))
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const groups = useMemo(() => {
    const out: { name: string; fields: FieldSpec[] }[] = []
    for (const field of spec.fields) {
      const name = field.group ?? ''
      const last = out[out.length - 1]
      if (last && last.name === name) last.fields.push(field)
      else out.push({ name, fields: [field] })
    }
    return out
  }, [spec])

  return (
    <div className="node-form">
      <div className="node-form__proto">
        <div className="segmented">
          {(Object.keys(PROTOCOLS) as Protocol[]).map((key) => (
            <button
              key={key}
              type="button"
              aria-pressed={key === proto}
              onClick={() => switchProtocol(key)}
            >
              {PROTOCOLS[key].label}
            </button>
          ))}
        </div>
        <p className="node-form__blurb">{spec.blurb}</p>
      </div>

      {groups.map((group, gi) => (
        <div className="node-form__group" key={group.name || gi}>
          {group.name && <h3 className="form-section">{group.name}</h3>}
          <div className="node-form__grid">
            {group.fields.map((field) => (
              <label
                key={field.key}
                className={`field${field.wide ? ' field--wide' : ''}${field.kind === 'switch' ? ' field--switch' : ''}`}
              >
                <span>
                  {field.label}
                  {field.required && <i className="req">必填</i>}
                </span>
                {field.kind === 'switch' ? (
                  <button
                    type="button"
                    className="switch"
                    role="switch"
                    aria-checked={values[field.key] === 'true'}
                    aria-label={field.label}
                    onClick={() =>
                      setValues((v) => ({ ...v, [field.key]: v[field.key] === 'true' ? 'false' : 'true' }))
                    }
                  >
                    <span />
                  </button>
                ) : field.options ? (
                  <select
                    value={values[field.key] ?? ''}
                    onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                  >
                    {field.options.map((opt) => (
                      <option key={opt} value={opt}>
                        {opt || '不设置'}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    type={field.kind === 'password' ? 'password' : 'text'}
                    inputMode={field.kind === 'number' ? 'numeric' : undefined}
                    value={values[field.key] ?? ''}
                    placeholder={field.placeholder}
                    onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                  />
                )}
                {field.hint && <span className="field__hint">{field.hint}</span>}
              </label>
            ))}
          </div>
        </div>
      ))}

      {error && <p className="form-error">{error}</p>}

      <div className="row-actions">
        <button className="btn btn--primary" onClick={() => void submit()} disabled={busy}>
          {busy ? '保存中…' : '保存节点'}
        </button>
        <button className="btn btn--quiet" onClick={onCancel}>
          取消
        </button>
      </div>
    </div>
  )
}

/* Links are what people actually have. The preview is deliberately shallow: the
   server does the real parsing and tells us which lines it could not read. */
function peekLink(link: string) {
  const trimmed = link.trim()
  const match = /^([a-z0-9]+):\/\/(.+)$/i.exec(trimmed)
  if (!match) return { ok: false, proto: '?', name: '', where: trimmed.slice(0, 32) }
  const proto = match[1].toLowerCase()
  const rest = match[2]
  const hashAt = rest.indexOf('#')
  const name = hashAt >= 0 ? safeDecode(rest.slice(hashAt + 1)) : ''
  const noHash = hashAt >= 0 ? rest.slice(0, hashAt) : rest
  const at = noHash.lastIndexOf('@')
  const where = (at >= 0 ? noHash.slice(at + 1) : noHash).split('?')[0]
  const known = ['hy2', 'hysteria2', 'vless', 'trojan', 'ss', 'vmess'].includes(proto)
  return { ok: known, proto: proto === 'hy2' ? 'hysteria2' : proto, name, where }
}

function safeDecode(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

export function LinkImport({
  hostId,
  onDone,
  onCancel,
}: {
  hostId: string
  onDone: () => Promise<void> | void
  onCancel: () => void
}) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const links = useMemo(
    () =>
      text
        .split(/[\s]+/)
        .map((s) => s.trim())
        .filter(Boolean),
    [text],
  )
  const preview = useMemo(() => links.map((link) => ({ link, ...peekLink(link) })), [links])
  const usable = preview.filter((p) => p.ok).length

  async function submit() {
    if (!links.length) return
    setBusy(true)
    setError(null)
    try {
      const res = await api.createNode(hostId, { links })
      if (res.failed?.length) {
        notify(`已导入 ${res.saved.length} 条，${res.failed.length} 条失败`)
        setError(res.failed.join('；'))
        await onDone()
      } else {
        notify(`已导入 ${res.saved.length} 条节点`)
        await onDone()
        onCancel()
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="link-import">
      <textarea
        className="code-input"
        rows={5}
        autoFocus
        spellCheck={false}
        value={text}
        placeholder={'hy2://密码@1.2.3.4:36906?sni=www.bing.com&insecure=1#Berlin\nvless://uuid@1.2.3.4:8443?security=reality&...#Reality'}
        onChange={(e) => setText(e.target.value)}
      />
      {preview.length > 0 && (
        <ul className="link-import__list">
          {preview.map((row, i) => (
            <li key={`${row.link}-${i}`} data-ok={row.ok}>
              <span className="link-import__proto">{row.proto}</span>
              <span className="link-import__name truncate">{row.name || '未命名'}</span>
              <span className="link-import__where mono truncate">{row.where}</span>
            </li>
          ))}
        </ul>
      )}
      {error && <p className="form-error">{error}</p>}
      <div className="row-actions">
        <button className="btn btn--primary" onClick={() => void submit()} disabled={busy || !usable}>
          {busy ? '导入中…' : usable ? `导入 ${usable} 条` : '导入'}
        </button>
        <button className="btn btn--quiet" onClick={onCancel}>
          取消
        </button>
      </div>
    </div>
  )
}

const SOURCE_LABEL: Record<string, string> = {
  agent: 'mihomo',
  config: '配置',
  manual: '手工',
}

export function SourceTag({ source }: { source: string }) {
  return (
    <span className={`source source--${source}`} title={sourceTitle(source)}>
      {SOURCE_LABEL[source] ?? source}
    </span>
  )
}

function sourceTitle(source: string) {
  switch (source) {
    case 'agent':
      return '来自本机 mihomo 的实时上报'
    case 'config':
      return '从机器上的 hysteria / xray 配置读出'
    default:
      return '在面板里手工填写'
  }
}

export function NodeTable({
  nodes,
  onEdit,
  onDelete,
  compact,
}: {
  nodes: ProxyNode[]
  onEdit?: (node: ProxyNode) => void
  onDelete?: (node: ProxyNode) => void
  compact?: boolean
}) {
  if (nodes.length === 0) {
    return <p className="node-empty">还没有节点。粘贴链接，或手工填一条。</p>
  }
  return (
    <table className={`table table--nodes${compact ? ' table--compact' : ''}`}>
      <thead>
        <tr>
          <th>节点</th>
          <th>协议</th>
          <th>地址</th>
          <th>来源</th>
          <th className="num">延迟</th>
          {(onEdit || onDelete) && <th aria-label="操作" />}
        </tr>
      </thead>
      <tbody>
        {nodes.map((node) => (
          <tr key={node.key}>
            <td data-label="节点">
              <span className="node-name truncate">{node.name}</span>
            </td>
            <td data-label="协议" className="mono muted">
              {node.type}
            </td>
            <td data-label="地址" className="mono muted truncate">
              {node.server}:{node.port}
            </td>
            <td data-label="来源">
              <SourceTag source={node.source} />
            </td>
            <td className="num" data-label="延迟">
              {node.alive ? (node.delayMs ? `${node.delayMs} ms` : '—') : <span className="muted">不可用</span>}
            </td>
            {(onEdit || onDelete) && (
              <td className="cell-action">
                {node.source === 'manual' ? (
                  <span className="node-row-actions">
                    {onEdit && (
                      <button
                        className="icon-btn icon-btn--sm"
                        aria-label={`编辑 ${node.name}`}
                        onClick={() => onEdit(node)}
                      >
                        <Icon name="gear" />
                      </button>
                    )}
                    {onDelete && (
                      <button
                        className="icon-btn icon-btn--sm"
                        aria-label={`删除 ${node.name}`}
                        onClick={() => onDelete(node)}
                      >
                        <Icon name="trash" />
                      </button>
                    )}
                  </span>
                ) : (
                  <span className="muted node-readonly" title="自动采集，不能在这里改">
                    自动
                  </span>
                )}
              </td>
            )}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
