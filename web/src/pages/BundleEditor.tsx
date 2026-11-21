import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, type Bundle } from '../api'
import { Icon } from '../icons'
import { delayTone } from '../nodes'
import { Copy, notify } from '../ui'
import type { PageProps } from '../App'

type Props = PageProps & { id?: string }

const groupOptions = [
  { value: 'host', label: '按机器' },
  { value: 'type', label: '按协议' },
  { value: 'region', label: '按地区' },
  { value: 'flat', label: '不分组' },
] as const

const behaviorOptions = [
  { value: 'url-test', label: 'url-test 自动测速' },
  { value: 'select', label: 'select 手动选择' },
  { value: 'fallback', label: 'fallback 故障转移' },
  { value: 'load-balance', label: 'load-balance 负载均衡' },
] as const

function TriCheckbox({
  checked,
  indeterminate,
  onChange,
}: {
  checked: boolean
  indeterminate: boolean
  onChange: () => void
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate
  }, [indeterminate])
  return <input ref={ref} type="checkbox" checked={checked} onChange={onChange} />
}

export function BundleEditor({ state, refresh, navigate, id }: Props) {
  const { t } = useTranslation()
  const existing = id ? state.bundles.find((b) => b.id === id) : undefined
  const [name, setName] = useState(existing?.name ?? '')
  const [slug, setSlug] = useState(existing?.slug ?? '')
  const [groupBy, setGroupBy] = useState<Bundle['groupBy']>(existing?.groupBy ?? 'host')
  const [groupType, setGroupType] = useState<Bundle['groupType']>(existing?.groupType ?? 'url-test')
  const [template, setTemplate] = useState(existing?.template ?? state.defaultTemplate ?? '')
  const [ruleSetID, setRuleSetID] = useState(existing?.ruleSetId ?? '')
  const [showTemplate, setShowTemplate] = useState(false)
  const [picked, setPicked] = useState<Set<string>>(
    () => new Set((existing?.members ?? []).map((m) => `${m.hostId}|${m.key}`)),
  )
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())
  const [query, setQuery] = useState('')
  const [aliveOnly, setAliveOnly] = useState(false)
  const [preview, setPreview] = useState<{ body: string; nodes: number } | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [tab, setTab] = useState<'yaml' | 'links'>('yaml')
  const [busy, setBusy] = useState(false)
  const [savedID, setSavedID] = useState(existing?.id ?? '')
  const [baseline, setBaseline] = useState(() =>
    JSON.stringify({
      name: existing?.name ?? '',
      slug: existing?.slug ?? '',
      groupBy: existing?.groupBy ?? 'host',
      groupType: existing?.groupType ?? 'url-test',
      template: existing?.template ?? state.defaultTemplate ?? '',
      ruleSetID: existing?.ruleSetId ?? '',
      members: (existing?.members ?? []).map((m) => `${m.hostId}|${m.key}`).sort(),
    }),
  )

  const currentSnapshot = useMemo(
    () =>
      JSON.stringify({
        name,
        slug,
        groupBy,
        groupType,
        template,
        ruleSetID,
        members: [...picked].sort(),
      }),
    [name, slug, groupBy, groupType, template, ruleSetID, picked],
  )
  const isDirty = currentSnapshot !== baseline

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
    let cancelled = false
    setPreviewError(null)
    const timer = window.setTimeout(() => {
      const members = [...picked].map((k) => {
        const [hostId, ...rest] = k.split('|')
        return { hostId, key: rest.join('|') }
      })
      api
        .previewDraft(
          { groupBy, groupType, template, ruleSetId: ruleSetID, members },
          tab === 'yaml' ? 'mihomo' : 'text',
        )
        .then((res) => {
          if (!cancelled) {
            setPreview(res)
            setPreviewError(null)
          }
        })
        .catch((e) => {
          if (!cancelled) {
            setPreview(null)
            setPreviewError((e as Error).message)
          }
        })
    }, 320)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [tab, picked, groupBy, groupType, template, ruleSetID])

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

  function toggleCollapse(hostID: string) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(hostID)) next.delete(hostID)
      else next.add(hostID)
      return next
    })
  }

  function selectVisible(onlyAlive = false) {
    setPicked((prev) => {
      const next = new Set(prev)
      for (const n of filtered) {
        if (!onlyAlive || n.alive) next.add(`${n.hostId}|${n.key}`)
      }
      return next
    })
  }

  const save = useCallback(async () => {
    if (!name.trim()) {
      notify(t('先给编排起个名字'), 'bad')
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
        ruleSetId: ruleSetID,
        members: [...picked].map((k) => {
          const [hostId, ...rest] = k.split('|')
          return { hostId, key: rest.join('|') }
        }),
      }
      const saved = savedID ? await api.updateBundle(savedID, body) : await api.createBundle(body)
      setSavedID(saved.id)
      setSlug(saved.slug)
      setBaseline(
        JSON.stringify({
          name: body.name,
          slug: saved.slug,
          groupBy,
          groupType,
          template,
          ruleSetID,
          members: [...picked].sort(),
        }),
      )
      await refresh()
      notify(t('已保存'), 'ok')
      if (!id) navigate(`/bundles/${saved.id}`)
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }, [name, slug, groupBy, groupType, template, ruleSetID, picked, savedID, refresh, id, navigate, t])

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
        e.preventDefault()
        if (!busy) void save()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [save, busy])

  return (
    <>
      <div className="page-head">
        <div>
          <div className="title-inline">
            <h1>{existing ? existing.name : t('新建编排')}</h1>
            {isDirty && <span className="badge badge-warn">{t('有未保存的更改')}</span>}
          </div>
          <p className="sub">
            {t('{{n}} 个节点', { n: selectedCount })}
            {savedID && <span className="mono"> · /{slug || existing?.slug}</span>}
          </p>
        </div>
        <div className="actions">
          <button className="quiet" onClick={() => navigate('/bundles')}>
            {t('返回')}
          </button>
          {savedID && (
            <button
              className="quiet"
              onClick={() => navigate(`/subscriptions?create=${encodeURIComponent(savedID)}`)}
            >
              <Icon name="link" size={14} />
              <span>{t('创建订阅')}</span>
            </button>
          )}
          <button className="primary" onClick={() => void save()} disabled={busy}>
            <Icon name="check" size={15} />
            <span>{busy ? t('保存中…') : t('保存')}</span>
          </button>
        </div>
      </div>

      <section className="panel">
        <div className="section-head">
          <h2>{t('基本设置')}</h2>
          <button className="text small" onClick={() => setShowTemplate((v) => !v)}>
            {showTemplate ? t('收起配置模板') : t('编辑配置模板')}
          </button>
        </div>
        <div className="form-grid">
          <label className="field">
            <span>{t('名称')}</span>
            <input value={name} placeholder={t('全部线路')} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="field">
            <span>{t('路径')}</span>
            <input className="mono" value={slug} placeholder="all" onChange={(e) => setSlug(e.target.value)} />
            <span className="hint">{t('用于订阅地址的分组标识，保存后尽量别改。')}</span>
          </label>
          <label className="field">
            <span>{t('分组方式')}</span>
            <select value={groupBy} onChange={(e) => setGroupBy(e.target.value as Bundle['groupBy'])}>
              {groupOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.label)}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>{t('组行为')}</span>
            <select value={groupType} onChange={(e) => setGroupType(e.target.value as Bundle['groupType'])}>
              {behaviorOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.label)}
                </option>
              ))}
            </select>
          </label>
          <label className="field full">
            <span>{t('规则集')}</span>
            <select value={ruleSetID} onChange={(e) => setRuleSetID(e.target.value)}>
              <option value="">{t('不套用规则集')}</option>
              {state.ruleSets.map((rs) => (
                <option key={rs.id} value={rs.id}>
                  {rs.name}（
                  {t('{{rules}} 条规则 · {{providers}} 个提供者', {
                    rules: rs.ruleCount ?? 0,
                    providers: rs.providerCount ?? 0,
                  })}
                  ）
                </option>
              ))}
            </select>
            <span className="hint">{t('套用后覆盖模板里的 rules，并合并 rule-providers。')}</span>
          </label>
        </div>
        {showTemplate && (
          <label className="field">
            <span>{t('配置模板（mihomo YAML，PROXY 与 AUTO 组会自动填充）')}</span>
            <textarea
              className="code-input"
              rows={12}
              value={template}
              placeholder={t('留空则使用默认模板')}
              onChange={(e) => setTemplate(e.target.value)}
            />
          </label>
        )}
      </section>

      <div className="grid split editor-split">
        <section className="panel panel-flush">
          <div className="panel-head">
            <div className="title-inline">
              <h2>{t('节点')}</h2>
              <span className="badge num">
                {t('已选 {{n}}', { n: selectedCount })} / {state.nodes.length}
              </span>
            </div>
            <div className="picker-bulk">
              <button type="button" className="text small" onClick={() => selectVisible(false)}>
                {t('全选当前')}
              </button>
              <button type="button" className="text small" onClick={() => selectVisible(true)}>
                {t('仅选可用')}
              </button>
              {selectedCount > 0 && (
                <button type="button" className="text small muted" onClick={() => setPicked(new Set())}>
                  {t('清空')}
                </button>
              )}
            </div>
          </div>

          <div className="picker-tools">
            <div className="search-box">
              <Icon name="search" size={14} className="search-icon" />
              <input
                type="search"
                value={query}
                placeholder={t('搜索节点、机器或协议')}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            <label className="check">
              <input type="checkbox" checked={aliveOnly} onChange={(e) => setAliveOnly(e.target.checked)} />
              {t('只看可用')}
            </label>
          </div>

          <div className="picker">
            {grouped.length === 0 && <p className="small muted picker-empty">{t('没有匹配的节点。')}</p>}
            {grouped.map(([hostID, nodes]) => {
              const host = hostsByID.get(hostID)
              const keys = nodes.map((n) => n.key)
              const pickedInHost = keys.filter((k) => picked.has(`${hostID}|${k}`)).length
              const all = pickedInHost === keys.length && keys.length > 0
              const some = pickedInHost > 0 && !all
              const isCollapsed = collapsed.has(hostID)
              return (
                <div className="picker-group" key={hostID}>
                  <div className="picker-head">
                    <label className="picker-head-check">
                      <TriCheckbox
                        checked={all}
                        indeterminate={some}
                        onChange={() => toggleHost(hostID, keys)}
                      />
                      <span className="picker-host-name">{host?.name ?? hostID}</span>
                    </label>
                    <span className="badge num">
                      {pickedInHost}/{nodes.length}
                    </span>
                    <button
                      type="button"
                      className="icon small picker-collapse"
                      aria-label={isCollapsed ? t('展开') : t('收起')}
                      onClick={() => toggleCollapse(hostID)}
                    >
                      <Icon
                        name="chevronDown"
                        size={14}
                        className={isCollapsed ? 'rot-closed' : undefined}
                      />
                    </button>
                  </div>
                  {!isCollapsed &&
                    nodes.map((n) => {
                      const key = `${hostID}|${n.key}`
                      const isPicked = picked.has(key)
                      const tone = delayTone(n.alive, n.delayMs)
                      return (
                        <label className="picker-row" data-selected={isPicked} key={key}>
                          <input type="checkbox" checked={isPicked} onChange={() => toggle(key)} />
                          <span className="node-name" title={n.name}>
                            {n.name}
                          </span>
                          <span className="proto-pill mono">{n.type}</span>
                          <span
                            className={`delay num${tone === 'ok' ? ' is-good' : tone === 'warn' ? ' is-warn' : tone === 'bad' ? ' is-bad' : ''}`}
                          >
                            {n.alive ? (n.delayMs ? `${n.delayMs} ms` : '—') : t('不可用')}
                          </span>
                        </label>
                      )
                    })}
                </div>
              )
            })}
          </div>
        </section>

        <section className="panel panel-flush editor-preview">
          <div className="panel-head">
            <div className="title-inline">
              <h2>{t('预览')}</h2>
              {preview && <span className="badge num">{t('{{n}} 个节点', { n: preview.nodes })}</span>}
            </div>
            <div className="preview-controls">
              <div className="tabs">
                <button aria-pressed={tab === 'yaml'} onClick={() => setTab('yaml')}>
                  YAML
                </button>
                <button aria-pressed={tab === 'links'} onClick={() => setTab('links')}>
                  {t('链接')}
                </button>
              </div>
              {preview && <Copy value={preview.body} />}
            </div>
          </div>
          <div className="editor-preview-body">
            {previewError ? (
              <p className="error panel-pad">{previewError}</p>
            ) : !preview ? (
              <p className="small muted panel-pad">{t('正在生成…')}</p>
            ) : (
              <pre>
                <code>{preview.body}</code>
              </pre>
            )}
          </div>
        </section>
      </div>
    </>
  )
}
