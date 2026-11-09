import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, type Bundle } from '../api'
import { Icon } from '../icons'
import { Copy, notify } from '../ui'
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
  const [query, setQuery] = useState('')
  const [aliveOnly, setAliveOnly] = useState(false)
  const [preview, setPreview] = useState<{ body: string; nodes: number } | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
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
    }, 350)
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

  async function save() {
    if (!name.trim()) {
      notify(t('先给编排起个名字'))
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
      await refresh()
      notify(t('已保存'))
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
          <h1>{existing ? existing.name : t('新建编排')}</h1>
          <p className="sub">
            {t('{{n}} 个节点', { n: selectedCount })}
            {savedID && <span className="mono"> · /{slug || existing?.slug}</span>}
          </p>
        </div>
        <div className="title-actions">
          <button className="btn btn--quiet" onClick={() => navigate('/bundles')}>
            {t('返回')}
          </button>
          <button className="btn btn--primary" onClick={() => void save()} disabled={busy}>
            {t('保存')}
          </button>
        </div>
      </div>

      <section className="panel">
        <header>
          <h2>{t('基本设置')}</h2>
          <button className="card-link" onClick={() => setShowTemplate((v) => !v)}>
            {showTemplate ? t('收起配置模板') : t('编辑配置模板')}
          </button>
        </header>
        <div className="form-grid">
          <label className="field">
            <span>{t('名称')}</span>
            <input value={name} placeholder={t('全部线路')} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="field">
            <span>{t('路径')}</span>
            <input
              className="mono"
              value={slug}
              placeholder="all"
              onChange={(e) => setSlug(e.target.value)}
            />
            <span className="field__hint">{t('用于订阅地址的分组标识，保存后尽量别改。')}</span>
          </label>
          <div className="field">
            <span>{t('分组方式')}</span>
            <div className="segmented">
              {groupOptions.map((o) => (
                <button
                  key={o.value}
                  type="button"
                  aria-pressed={groupBy === o.value}
                  onClick={() => setGroupBy(o.value)}
                >
                  {t(o.label)}
                </button>
              ))}
            </div>
          </div>
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
          <label className="field">
            <span>{t('规则集')}</span>
            <select value={ruleSetID} onChange={(e) => setRuleSetID(e.target.value)}>
              <option value="">{t('不套用规则集')}</option>
              {state.ruleSets.map((rs) => (
                <option key={rs.id} value={rs.id}>
                  {rs.name}（{t('{{rules}} 条规则 · {{providers}} 个提供者', {
                    rules: rs.ruleCount ?? 0,
                    providers: rs.providerCount ?? 0,
                  })}）
                </option>
              ))}
            </select>
            <span className="field__hint">{t('套用后覆盖模板里的 rules，并合并 rule-providers。')}</span>
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

      <div className="grid grid--editor">
        <section className="panel span-5">
          <header>
            <h2>{t('节点')}</h2>
            <span className="meta">{t('已选 {{n}}', { n: selectedCount })}</span>
          </header>
          <div className="picker-tools">
            <label className="search">
              <Icon name="search" />
              <input
                value={query}
                placeholder={t('搜索节点、机器或协议')}
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <label className="check">
              <input type="checkbox" checked={aliveOnly} onChange={(e) => setAliveOnly(e.target.checked)} />
              {t('只看可用')}
            </label>
          </div>
          <div className="picker">
            {grouped.length === 0 && <p className="prose muted">{t('没有匹配的节点。')}</p>}
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
            <h2>{t('预览')}</h2>
            <div className="segmented segmented--sm">
              <button aria-pressed={tab === 'yaml'} onClick={() => setTab('yaml')}>
                YAML
              </button>
              <button aria-pressed={tab === 'links'} onClick={() => setTab('links')}>
                {t('链接')}
              </button>
            </div>
          </header>
          {previewError ? (
            <p className="form-error">{previewError}</p>
          ) : !preview ? (
            <p className="prose muted">{t('正在生成…')}</p>
          ) : (
            <>
              <div className="preview-meta">
                <span className="meta">{t('{{n}} 个节点', { n: preview.nodes })}</span>
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
