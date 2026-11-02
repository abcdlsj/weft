import { useEffect, useMemo, useState } from 'react'
import { api, type Bundle } from '../api'
import { Icon } from '../icons'
import { Copy, Empty, notify } from '../ui'
import type { PageProps } from '../App'

type Props = PageProps & { id?: string }

const groupOptions = [
  { value: 'host', label: '按机器' },
  { value: 'type', label: '按协议' },
  { value: 'flat', label: '不分组' },
] as const

const behaviorOptions = [
  { value: 'url-test', label: 'url-test 自动测速' },
  { value: 'select', label: 'select 手动选择' },
  { value: 'fallback', label: 'fallback 故障转移' },
  { value: 'load-balance', label: 'load-balance 负载均衡' },
] as const

export function BundleEditor({ state, refresh, navigate, id }: Props) {
  const existing = id ? state.bundles.find((b) => b.id === id) : undefined
  const [name, setName] = useState(existing?.name ?? '')
  const [slug, setSlug] = useState(existing?.slug ?? '')
  const [groupBy, setGroupBy] = useState<Bundle['groupBy']>(existing?.groupBy ?? 'host')
  const [groupType, setGroupType] = useState<Bundle['groupType']>(existing?.groupType ?? 'url-test')
  const [template, setTemplate] = useState(existing?.template ?? state.defaultTemplate ?? '')
  const [showTemplate, setShowTemplate] = useState(false)
  const [picked, setPicked] = useState<Set<string>>(
    () => new Set((existing?.members ?? []).map((m) => `${m.hostId}|${m.key}`)),
  )
  const [query, setQuery] = useState('')
  const [aliveOnly, setAliveOnly] = useState(false)
  const [preview, setPreview] = useState<{ body: string; nodes: number } | null>(null)
  const [tab, setTab] = useState<'yaml' | 'links'>('yaml')
  const [busy, setBusy] = useState(false)
  const [savedID, setSavedID] = useState(existing?.id ?? '')

  const hostsByID = useMemo(() => new Map(state.hosts.map((h) => [h.id, h])), [state.hosts])
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return state.nodes.filter((n) => {
      if (aliveOnly && !n.alive) return false
      if (!q) return true
      return (
        n.name.toLowerCase().includes(q) ||
        n.hostName.toLowerCase().includes(q) ||
        n.type.toLowerCase().includes(q)
      )
    })
  }, [state.nodes, query, aliveOnly])

  const grouped = useMemo(() => {
    const map = new Map<string, typeof filtered>()
    for (const n of filtered) {
      const list = map.get(n.hostId) ?? []
      list.push(n)
      map.set(n.hostId, list)
    }
    return [...map.entries()].sort((a, b) =>
      (hostsByID.get(a[0])?.name ?? '').localeCompare(hostsByID.get(b[0])?.name ?? ''),
    )
  }, [filtered, hostsByID])

  const selectedCount = picked.size

  useEffect(() => {
    if (!savedID) return
    let cancelled = false
    api
      .preview(savedID, tab === 'yaml' ? 'mihomo' : 'text')
      .then((res) => !cancelled && setPreview(res))
      .catch(() => !cancelled && setPreview(null))
    return () => {
      cancelled = true
    }
  }, [savedID, tab, picked, groupBy, groupType])

  function toggle(key: string) {
    setPicked((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  function toggleHost(hostID: string, keys: string[]) {
    setPicked((prev) => {
      const next = new Set(prev)
      const all = keys.every((k) => next.has(`${hostID}|${k}`))
      for (const k of keys) {
        if (all) next.delete(`${hostID}|${k}`)
        else next.add(`${hostID}|${k}`)
      }
      return next
    })
  }

  async function save() {
    if (!name.trim()) {
      notify('先给编排起个名字')
      return
    }
    setBusy(true)
    try {
      const body = {
        name: name.trim(),
        slug: slug.trim(),
        groupBy,
        groupType,
        template,
        members: [...picked].map((k) => {
          const [hostId, ...rest] = k.split('|')
          return { hostId, key: rest.join('|') }
        }),
      }
      const saved = savedID ? await api.updateBundle(savedID, body) : await api.createBundle(body)
      setSavedID(saved.id)
      setSlug(saved.slug)
      await refresh()
      notify('已保存')
      if (!id) navigate(`/bundles/${saved.id}`)
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>{existing ? existing.name : '新建编排'}</h1>
          <p className="sub">
            {selectedCount} 个节点
            {savedID && <span className="mono"> · /{slug || existing?.slug}</span>}
          </p>
        </div>
        <div className="title-actions">
          <button className="btn btn--quiet" onClick={() => navigate('/bundles')}>
            返回
          </button>
          <button className="btn btn--primary" onClick={() => void save()} disabled={busy}>
            保存
          </button>
        </div>
      </div>

      <section className="panel">
        <header>
          <h2>基本设置</h2>
          <button className="card-link" onClick={() => setShowTemplate((v) => !v)}>
            {showTemplate ? '收起配置模板' : '编辑配置模板'}
          </button>
        </header>
        <div className="form-grid">
          <label className="field">
            <span>名称</span>
            <input value={name} placeholder="全部线路" onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="field">
            <span>路径</span>
            <input
              className="mono"
              value={slug}
              placeholder="all"
              onChange={(e) => setSlug(e.target.value)}
            />
            <span className="field__hint">用于订阅地址的分组标识，保存后尽量别改。</span>
          </label>
          <div className="field">
            <span>分组方式</span>
            <div className="segmented">
              {groupOptions.map((o) => (
                <button
                  key={o.value}
                  type="button"
                  aria-pressed={groupBy === o.value}
                  onClick={() => setGroupBy(o.value)}
                >
                  {o.label}
                </button>
              ))}
            </div>
          </div>
          <label className="field">
            <span>组行为</span>
            <select value={groupType} onChange={(e) => setGroupType(e.target.value as Bundle['groupType'])}>
              {behaviorOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
        </div>
        {showTemplate && (
          <label className="field">
            <span>配置模板（mihomo YAML，PROXY 与 AUTO 组会自动填充）</span>
            <textarea
              className="code-input"
              rows={12}
              value={template}
              placeholder="留空则使用默认模板"
              onChange={(e) => setTemplate(e.target.value)}
            />
          </label>
        )}
      </section>

      <div className="grid grid--editor">
        <section className="panel span-5">
          <header>
            <h2>节点</h2>
            <span className="meta">已选 {selectedCount}</span>
          </header>
          <div className="picker-tools">
            <label className="search">
              <Icon name="search" />
              <input
                value={query}
                placeholder="搜索节点、机器或协议"
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <label className="check">
              <input type="checkbox" checked={aliveOnly} onChange={(e) => setAliveOnly(e.target.checked)} />
              只看可用
            </label>
          </div>
          <div className="picker">
            {grouped.length === 0 && <p className="prose muted">没有匹配的节点。</p>}
            {grouped.map(([hostID, nodes]) => {
              const host = hostsByID.get(hostID)
              const keys = nodes.map((n) => n.key)
              const all = keys.every((k) => picked.has(`${hostID}|${k}`))
              return (
                <div className="picker__group" key={hostID}>
                  <div className="picker__head">
                    <button className="check" onClick={() => toggleHost(hostID, keys)}>
                      <input type="checkbox" checked={all} readOnly tabIndex={-1} />
                      <span>{host?.name ?? hostID}</span>
                      <span className="muted">{nodes.length}</span>
                    </button>
                  </div>
                  {nodes.map((n) => {
                    const key = `${hostID}|${n.key}`
                    return (
                      <button
                        key={key}
                        className="picker__row"
                        aria-pressed={picked.has(key)}
                        onClick={() => toggle(key)}
                      >
                        <input type="checkbox" checked={picked.has(key)} readOnly tabIndex={-1} />
                        <span className="node-name">{n.name}</span>
                        <span className="mono muted">{n.type}</span>
                        <span className="picker__delay">
                          {n.alive ? `${n.delayMs} ms` : '—'}
                        </span>
                      </button>
                    )
                  })}
                </div>
              )
            })}
          </div>
        </section>

        <section className="panel span-7">
          <header>
            <h2>预览</h2>
            <div className="segmented segmented--sm">
              <button aria-pressed={tab === 'yaml'} onClick={() => setTab('yaml')}>
                YAML
              </button>
              <button aria-pressed={tab === 'links'} onClick={() => setTab('links')}>
                链接
              </button>
            </div>
          </header>
          {!savedID ? (
            <Empty title="保存后可预览" body="预览读取的是服务端最新的节点上报，先保存再生成。" />
          ) : !preview ? (
            <p className="prose muted">正在生成…</p>
          ) : (
            <>
              <div className="preview-meta">
                <span className="meta">{preview.nodes} 个节点</span>
                <Copy value={preview.body} />
              </div>
              <pre className="code code--preview">{preview.body}</pre>
            </>
          )}
        </section>
      </div>
    </>
  )
}
