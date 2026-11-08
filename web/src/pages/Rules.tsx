import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ago, api, type RuleSet } from '../api'
import { Icon } from '../icons'
import { Copy, Drawer, Empty, Modal, notify } from '../ui'
import type { PageProps } from '../App'

type Mode = 'blank' | 'preset' | 'import'

const blankContent = `rules:
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

export function Rules({ state, refresh }: PageProps) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<RuleSet | null>(null)
  const [creating, setCreating] = useState(false)
  const [mode, setMode] = useState<Mode>('preset')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [content, setContent] = useState(blankContent)
  const [presetKey, setPresetKey] = useState(state.rulePresets[0]?.key ?? '')
  const [sourceURL, setSourceURL] = useState(state.gfwListUrl)
  const [busy, setBusy] = useState(false)

  const preset = useMemo(
    () => state.rulePresets.find((p) => p.key === presetKey) ?? state.rulePresets[0],
    [state.rulePresets, presetKey],
  )

  function openNew(next: Mode = 'preset') {
    const first = state.rulePresets[0]
    setMode(next)
    setCreating(true)
    setName(first?.name ?? '')
    setDescription('')
    setContent(blankContent)
    setPresetKey(first?.key ?? '')
    setSourceURL(state.gfwListUrl)
  }

  async function create() {
    setBusy(true)
    try {
      let created: RuleSet
      if (mode === 'import') {
        created = await api.importRuleSet({
          name: name.trim() || t('导入'),
          url: sourceURL.trim(),
          description: description.trim(),
        })
      } else {
        if (!name.trim()) {
          notify(t('名称不能为空'))
          return
        }
        created = await api.createRuleSet({
          name: name.trim(),
          description: description.trim(),
          content: mode === 'preset' && preset ? preset.content : content,
          source: mode === 'preset' && preset ? `preset:${preset.key}` : '',
        })
      }
      setCreating(false)
      await refresh()
      setEditing(created)
      notify(t('规则集已创建'))
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function openEdit(rs: RuleSet) {
    try {
      setEditing(await api.ruleSet(rs.id))
    } catch (e) {
      notify((e as Error).message)
    }
  }

  async function save() {
    if (!editing) return
    setBusy(true)
    try {
      const updated = await api.updateRuleSet(editing.id, {
        name: editing.name,
        description: editing.description,
        content: editing.content ?? '',
        source: editing.source,
      })
      setEditing(updated)
      await refresh()
      notify(t('规则集已保存'))
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(rs: RuleSet) {
    if (!confirm(t('删除规则集「{{name}}」？引用它的编排会恢复模板规则。', { name: rs.name }))) return
    await api.deleteRuleSet(rs.id)
    setEditing(null)
    await refresh()
    notify(t('规则集已删除'))
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>{t('规则')}</h1>
          <p className="sub">{t('一套可复用的分流方案：rules、rule-providers 与额外代理组。')}</p>
        </div>
        <div className="title-actions">
          <button className="btn btn--primary" onClick={() => openNew()}>
            <Icon name="plus" />
            {t('新建规则集')}
          </button>
        </div>
      </div>

      <section className="panel">
        {state.ruleSets.length === 0 ? (
          <Empty
            title={t('还没有规则集')}
            body={t('从 GFW 列表导入，或从预设开始，再按自己的习惯改。')}
            action={
              <button className="btn btn--primary" onClick={() => openNew()}>
                {t('新建规则集')}
              </button>
            }
          />
        ) : (
          <table className="table table--click">
            <thead>
              <tr>
                <th>{t('名称')}</th>
                <th>{t('规则数')}</th>
                <th>{t('规则提供者')}</th>
                <th>{t('被引用')}</th>
                <th>{t('最近更新')}</th>
                <th aria-label={t('操作')} />
              </tr>
            </thead>
            <tbody>
              {state.ruleSets.map((rs) => (
                <tr
                  key={rs.id}
                  tabIndex={0}
                  role="button"
                  aria-label={t('编辑 {{name}}', { name: rs.name })}
                  onClick={() => void openEdit(rs)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      void openEdit(rs)
                    }
                  }}
                >
                  <td data-label={t('名称')}>
                    <span className="cell-main">{rs.name}</span>
                    <span className="cell-sub">{rs.description || rs.source || '—'}</span>
                  </td>
                  <td className="num" data-label={t('规则数')}>
                    {rs.ruleCount ?? 0}
                  </td>
                  <td className="num" data-label={t('规则提供者')}>
                    {rs.providerCount ?? 0}
                  </td>
                  <td className="num" data-label={t('被引用')}>
                    {rs.usedBy ? t('{{n}} 个编排', { n: rs.usedBy }) : <span className="muted">{t('未使用')}</span>}
                  </td>
                  <td className="num muted" data-label={t('最近更新')}>
                    {ago(rs.updatedAt)}
                  </td>
                  <td className="cell-action">
                    <Icon name="chevron" />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="panel panel--note">
        <header>
          <h2>{t('规则')}</h2>
          <span className="meta">{t('一条编排可以被多条订阅引用')}</span>
        </header>
        <p className="prose">{t('规则集是编排的一部分；一条编排可以选择一套规则，生成配置时覆盖模板里的 rules。')}</p>
        <ul className="facts facts--defs">
          {state.rulePresets.map((p) => (
            <li key={p.key}>
              <span>{t(p.name)}</span>
              <span>{t(p.description)}</span>
            </li>
          ))}
        </ul>
      </section>

      {creating && (
        <Modal
          title={t('新建规则集')}
          onClose={() => setCreating(false)}
          wide
          footer={
            <>
              <button className="btn btn--quiet" onClick={() => setCreating(false)}>
                {t('取消')}
              </button>
              <button className="btn btn--primary" onClick={() => void create()} disabled={busy}>
                {mode === 'import' ? t('拉取并创建') : t('创建规则集')}
              </button>
            </>
          }
        >
          <div className="segmented" style={{ marginBottom: 18 }}>
            {(
              [
                ['preset', '从预设开始'],
                ['import', '从 GFW 列表生成'],
                ['blank', '从空白开始'],
              ] as const
            ).map(([value, label]) => (
              <button key={value} aria-pressed={mode === value} onClick={() => setMode(value)}>
                {t(label)}
              </button>
            ))}
          </div>

          <label className="field">
            <span>{t('名称')}</span>
            <input
              value={name}
              placeholder={mode === 'import' ? t('导入的规则') : t('我的分流规则')}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          <label className="field">
            <span>{t('描述')}</span>
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>

          {mode === 'preset' && (
            <>
              <label className="field">
                <span>{t('预设')}</span>
                <select
                  value={presetKey}
                  onChange={(e) => {
                    setPresetKey(e.target.value)
                    const next = state.rulePresets.find((p) => p.key === e.target.value)
                    if (next) setName(next.name)
                  }}
                >
                  {state.rulePresets.map((p) => (
                    <option key={p.key} value={p.key}>
                      {t(p.name)}
                    </option>
                  ))}
                </select>
                <span className="field__hint">{preset ? t(preset.description) : ''}</span>
              </label>
              {preset && <pre className="code code--preview">{preset.content}</pre>}
              <p className="prose muted">{t('这些预设在创建后会复制成你自己的规则集，随时可改。')}</p>
            </>
          )}

          {mode === 'import' && (
            <label className="field">
              <span>{t('GFW 列表地址')}</span>
              <input className="mono" value={sourceURL} onChange={(e) => setSourceURL(e.target.value)} />
              <span className="field__hint">{t('支持 gfwlist、Clash rule-provider 文本或含 rules 的 YAML。')}</span>
            </label>
          )}

          {mode === 'blank' && (
            <label className="field">
              <span>{t('规则内容（YAML）')}</span>
              <textarea
                className="code-input"
                rows={14}
                value={content}
                onChange={(e) => setContent(e.target.value)}
              />
              <span className="field__hint">{t('只允许 rules、rule-providers、proxy-groups 三个键。')}</span>
            </label>
          )}
        </Modal>
      )}

      {editing && (
        <Drawer title={editing.name} wide onClose={() => setEditing(null)}>
          <div className="form-grid">
            <label className="field">
              <span>{t('名称')}</span>
              <input value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
            </label>
            <label className="field">
              <span>{t('描述')}</span>
              <input
                value={editing.description}
                onChange={(e) => setEditing({ ...editing, description: e.target.value })}
              />
            </label>
          </div>
          <p className="prose muted">
            {t('{{rules}} 条规则 · {{providers}} 个提供者', {
              rules: editing.ruleCount ?? 0,
              providers: editing.providerCount ?? 0,
            })}
            {editing.usedBy ? ` · ${t('{{n}} 个编排', { n: editing.usedBy })}` : ''}
          </p>
          <label className="field">
            <span>{t('规则内容（YAML）')}</span>
            <textarea
              className="code-input"
              rows={22}
              spellCheck={false}
              value={editing.content ?? ''}
              onChange={(e) => setEditing({ ...editing, content: e.target.value })}
            />
            <span className="field__hint">{t('只允许 rules、rule-providers、proxy-groups 三个键。')}</span>
          </label>
          {editing.source && (
            <p className="prose muted">
              {t('规则集来源')}：<span className="mono">{editing.source}</span>
            </p>
          )}
          <div className="row-actions">
            <button className="btn btn--primary" disabled={busy} onClick={() => void save()}>
              {t('保存')}
            </button>
            <Copy value={editing.content ?? ''} label={t('复制')} />
            <button className="btn btn--danger btn--sm" onClick={() => void remove(editing)}>
              <Icon name="trash" />
              {t('删除')}
            </button>
          </div>
        </Drawer>
      )}
    </>
  )
}
