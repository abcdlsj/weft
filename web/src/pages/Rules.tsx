import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import CodeMirror from '@uiw/react-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { oneDark } from '@codemirror/theme-one-dark'
import { diffLines, type Change } from 'diff'
import { ago, api, type RuleSet } from '../api'
import { Copy, Drawer, Empty, Modal, notify } from '../ui'
import type { PageProps } from '../App'

type Mode = 'blank' | 'preset' | 'import'

const blankContent = `rules:
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

export function Rules({ state, refresh, theme }: PageProps) {
  const { t } = useTranslation()
  const dark =
    theme === 'dark' ||
    (theme === 'auto' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  const yamlExtensions = useMemo(() => [yaml()], [])
  const [editing, setEditing] = useState<RuleSet | null>(null)
  const [creating, setCreating] = useState(false)
  const [mode, setMode] = useState<Mode>('preset')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [content, setContent] = useState(blankContent)
  const [presetKey, setPresetKey] = useState(state.rulePresets[0]?.key ?? '')
  const [sourceURL, setSourceURL] = useState(state.gfwListUrl)
  const [busy, setBusy] = useState(false)
  const [diff, setDiff] = useState<{ content: string; parts: Change[] } | null>(null)
  const [refreshing, setRefreshing] = useState(false)

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

  async function save(contentOverride?: string) {
    if (!editing) return
    setBusy(true)
    try {
      const updated = await api.updateRuleSet(editing.id, {
        name: editing.name,
        description: editing.description,
        content: contentOverride ?? editing.content ?? '',
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

  async function refreshFromSource() {
    if (!editing) return
    setRefreshing(true)
    try {
      const res = await api.refreshRuleSet(editing.id)
      setDiff({ content: res.content, parts: diffLines(editing.content ?? '', res.content) })
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setRefreshing(false)
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
      <div className="page-head">
        <div>
          <h1>{t('规则')}</h1>
          <p className="sub">{t('一套可复用的分流方案：rules、rule-providers 与额外代理组。')}</p>
        </div>
        <div className="actions">
          <button className="primary" onClick={() => openNew()}>
            {t('新建规则集')}
          </button>
        </div>
      </div>

      <section>
        {state.ruleSets.length === 0 ? (
          <Empty
            title={t('还没有规则集')}
            body={t('从 GFW 列表导入，或从预设开始，再按自己的习惯改。')}
            action={
              <button className="primary" onClick={() => openNew()}>
                {t('新建规则集')}
              </button>
            }
          />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t('名称')}</th>
                  <th className="num">{t('规则数')}</th>
                  <th className="num">{t('规则提供者')}</th>
                  <th className="num">{t('被引用')}</th>
                  <th className="num">{t('最近更新')}</th>
                </tr>
              </thead>
              <tbody>
                {state.ruleSets.map((rs) => (
                  <tr
                    key={rs.id}
                    className="click"
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
                    <td>
                      <span className="cell-main">
                        {rs.name}
                        {!!rs.warningCount && (
                          <span className="badge" title={t('{{n}} 条引用警告', { n: rs.warningCount })}>
                            {' '}
                            {rs.warningCount}
                          </span>
                        )}
                      </span>
                      <span className="cell-sub">{rs.description || rs.source || '—'}</span>
                    </td>
                    <td className="num">{rs.ruleCount ?? 0}</td>
                    <td className="num">{rs.providerCount ?? 0}</td>
                    <td className="num small muted">
                      {rs.usedBy ? t('{{n}} 个编排', { n: rs.usedBy }) : t('未使用')}
                    </td>
                    <td className="num small muted">{ago(rs.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section>
        <div className="section-head">
          <h2>{t('规则')}</h2>
          <span className="small muted">{t('一条编排可以被多条订阅引用')}</span>
        </div>
        <p>{t('规则集是编排的一部分；一条编排可以选择一套规则，生成配置时覆盖模板里的 rules。')}</p>
        <ul className="facts">
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
              <button className="quiet" onClick={() => setCreating(false)}>
                {t('取消')}
              </button>
              <button className="primary" onClick={() => void create()} disabled={busy}>
                {mode === 'import' ? t('拉取并创建') : t('创建规则集')}
              </button>
            </>
          }
        >
          <div className="tabs">
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
                <span className="hint">{preset ? t(preset.description) : ''}</span>
              </label>
              {preset && (
                <pre>
                  <code>{preset.content}</code>
                </pre>
              )}
              <p className="small muted">{t('这些预设在创建后会复制成你自己的规则集，随时可改。')}</p>
            </>
          )}

          {mode === 'import' && (
            <label className="field">
              <span>{t('GFW 列表地址')}</span>
              <input className="mono" value={sourceURL} onChange={(e) => setSourceURL(e.target.value)} />
              <span className="hint">{t('支持 gfwlist、Clash rule-provider 文本或含 rules 的 YAML。')}</span>
            </label>
          )}

          {mode === 'blank' && (
            <div className="field">
              <span>{t('规则内容（YAML）')}</span>
              <CodeMirror
                value={content}
                height="320px"
                extensions={yamlExtensions}
                theme={dark ? oneDark : 'light'}
                onChange={setContent}
              />
              <span className="hint">{t('只允许 rules、rule-providers、proxy-groups 三个键。')}</span>
            </div>
          )}
        </Modal>
      )}

      {editing && (
        <Drawer title={editing.name} onClose={() => setEditing(null)}>
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
          <p className="small muted">
            {t('{{rules}} 条规则 · {{providers}} 个提供者', {
              rules: editing.ruleCount ?? 0,
              providers: editing.providerCount ?? 0,
            })}
            {editing.usedBy ? ` · ${t('{{n}} 个编排', { n: editing.usedBy })}` : ''}
          </p>
          <div className="field">
            <span>{t('规则内容（YAML）')}</span>
            <CodeMirror
              value={editing.content ?? ''}
              height="420px"
              extensions={yamlExtensions}
              theme={dark ? oneDark : 'light'}
              onChange={(value) => setEditing({ ...editing, content: value })}
            />
            <span className="hint">{t('只允许 rules、rule-providers、proxy-groups 三个键。')}</span>
          </div>
          {!!editing.warnings?.length && (
            <div className="warnings">
              <p>{t('引用警告')}</p>
              <ul className="facts">
                {editing.warnings.map((w) => (
                  <li key={w}>
                    <span>{w}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {editing.source && !editing.source.startsWith('preset:') && (
            <p className="small muted">
              {t('规则集来源')}：<span className="mono">{editing.source}</span>
            </p>
          )}
          <div className="actions">
            <button className="primary" disabled={busy} onClick={() => void save()}>
              {t('保存')}
            </button>
            {(editing.source.startsWith('http://') || editing.source.startsWith('https://')) && (
              <button className="quiet" disabled={refreshing} onClick={() => void refreshFromSource()}>
                {refreshing ? t('拉取中…') : t('从来源刷新')}
              </button>
            )}
            <Copy value={editing.content ?? ''} label={t('复制')} />
            <button className="danger small" onClick={() => void remove(editing)}>
              {t('删除')}
            </button>
          </div>
        </Drawer>
      )}

      {diff && (
        <Modal
          title={t('规则差异')}
          onClose={() => setDiff(null)}
          wide
          footer={
            <>
              <button className="quiet" onClick={() => setDiff(null)}>
                {t('取消')}
              </button>
              <button
                className="primary"
                disabled={busy}
                onClick={() => {
                  const next = diff.content
                  setDiff(null)
                  void save(next)
                }}
              >
                {t('应用并保存')}
              </button>
            </>
          }
        >
          <p className="small muted">{t('绿色为新增，红色为删除，灰色为未变。')}</p>
          <pre className="diff-view">
            {diff.parts.map((part, i) =>
              part.value
                .replace(/\n$/, '')
                .split('\n')
                .map((line, j) => (
                  <span
                    key={`${i}-${j}`}
                    className={part.added ? 'diff-added' : part.removed ? 'diff-removed' : 'diff-same'}
                  >
                    {part.added ? '+ ' : part.removed ? '- ' : '  '}
                    {line}
                    {'\n'}
                  </span>
                )),
            )}
          </pre>
        </Modal>
      )}
    </>
  )
}
