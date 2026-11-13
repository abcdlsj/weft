import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { notify } from '../ui'
import { setLocale, type Locale } from '../i18n'
import type { PageProps } from '../App'

type Props = PageProps & {
  theme: 'light' | 'dark' | 'auto'
  setTheme: (t: 'light' | 'dark' | 'auto') => void
}

export function Settings({ state, refresh, theme, setTheme }: Props) {
  const { t, i18n } = useTranslation()
  const locale: Locale = i18n.language === 'en' ? 'en' : 'zh'
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirmNext, setConfirmNext] = useState('')
  const [template, setTemplate] = useState(state.defaultTemplate || state.builtinTemplate)
  const [busy, setBusy] = useState(false)
  const [restoreBusy, setRestoreBusy] = useState(false)
  const [restoreWarnings, setRestoreWarnings] = useState<string[]>([])
  const fileInput = useRef<HTMLInputElement>(null)

  const usingBuiltin = !state.defaultTemplate

  async function changePassword() {
    if (next !== confirmNext) {
      notify(t('两次输入的新口令不一致'))
      return
    }
    setBusy(true)
    try {
      await api.changePassword(current, next)
      setCurrent('')
      setNext('')
      setConfirmNext('')
      notify(t('口令已更新'))
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function restore(file: File) {
    setRestoreBusy(true)
    setRestoreWarnings([])
    try {
      const data = JSON.parse(await file.text())
      const res = await api.restore(data)
      setRestoreWarnings(res.warnings)
      await refresh()
      notify(
        t('已恢复 {{rules}} 套规则、{{bundles}} 条编排', {
          rules: res.ruleSetsCreated + res.ruleSetsUpdated,
          bundles: res.bundlesCreated + res.bundlesUpdated,
        }),
      )
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setRestoreBusy(false)
    }
  }

  async function saveTemplate() {
    setBusy(true)
    try {
      await api.saveSettings({ defaultTemplate: template })
      await refresh()
      notify(t('默认模板已保存'))
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('外观与口令')}</h1>
          <p className="sub">{t('面板外观、口令与新建编排的默认模板。')}</p>
        </div>
      </div>

      <section>
        <div className="section-head">
          <h2>{t('外观与实例')}</h2>
          <span className="small muted">{state.demo ? t('演示数据（内存库）') : t('自托管')}</span>
        </div>
        <div className="grid two">
          <div>
            <p className="small muted">{t('主题')}</p>
            <div className="tabs">
              {(
                [
                  ['auto', '跟随系统'],
                  ['light', '浅色'],
                  ['dark', '深色'],
                ] as const
              ).map(([value, label]) => (
                <button key={value} aria-pressed={theme === value} onClick={() => setTheme(value)}>
                  {t(label)}
                </button>
              ))}
            </div>
            <p className="small muted">{t('可安装到主屏，深色模式跟随系统。')}</p>
            <p className="small muted">{t('面板语言')}</p>
            <div className="tabs">
              {(
                [
                  ['zh', '中文'],
                  ['en', 'English'],
                ] as const
              ).map(([value, label]) => (
                <button key={value} aria-pressed={locale === value} onClick={() => setLocale(value)}>
                  {label}
                </button>
              ))}
            </div>
            <p className="small muted">{t('界面语言保存在这台设备上。')}</p>
          </div>
          <ul className="facts">
            <li>
              <span>{t('访问地址')}</span>
              <span className="mono">{state.publicBase}</span>
            </li>
            <li>
              <span>{t('机器 / 节点')}</span>
              <span>
                {t('{{hosts}} 台 / {{nodes}} 个', { hosts: state.stats.hostsTotal, nodes: state.stats.nodesTotal })}
              </span>
            </li>
            <li>
              <span>{t('订阅链接')}</span>
              <span>
                {t('{{hits}} 次拉取 / {{n}} 条', { hits: state.stats.subHits, n: state.subscriptions.length })}
              </span>
            </li>
          </ul>
        </div>
      </section>

      <section>
        <div className="section-head">
          <h2>{t('面板口令')}</h2>
          <span className="small muted">{t('保存在 weft.db，走 Argon2id')}</span>
        </div>
        <div className="form-grid three">
          <label className="field">
            <span>{t('当前口令')}</span>
            <input
              type="password"
              autoComplete="current-password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </label>
          <label className="field">
            <span>{t('新口令')}</span>
            <input
              type="password"
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </label>
          <label className="field">
            <span>{t('再次输入')}</span>
            <input
              type="password"
              autoComplete="new-password"
              value={confirmNext}
              onChange={(e) => setConfirmNext(e.target.value)}
            />
          </label>
        </div>
        <div className="actions">
          <button
            className="primary"
            disabled={busy || !current || next.length < 6}
            onClick={() => void changePassword()}
          >
            {t('更新口令')}
          </button>
          <button
            className="quiet push"
            onClick={() => {
              api.logout().catch(() => {}).finally(() => window.location.reload())
            }}
          >
            {t('结束会话')}
          </button>
        </div>
      </section>

      <section>
        <div className="section-head">
          <h2>{t('默认配置模板')}</h2>
          <span className="small muted">{usingBuiltin ? t('当前使用内置模板') : t('已自定义')}</span>
        </div>
        <p>
          {t('节点填入')} <code className="mono">PROXY</code> {t('与')} <code className="mono">AUTO</code>
          {t('，其余段落原样保留；清空保存即恢复内置模板。')}
        </p>
        <textarea
          className="code-input"
          rows={14}
          value={template}
          onChange={(e) => setTemplate(e.target.value)}
        />
        <div className="actions">
          <button className="primary" disabled={busy} onClick={() => void saveTemplate()}>
            {t('保存模板')}
          </button>
          <button
            className="quiet"
            onClick={() => {
              setTemplate(state.builtinTemplate)
              notify(t('已载入内置模板，保存后生效'))
            }}
          >
            {t('恢复内置模板')}
          </button>
        </div>
      </section>

      <section>
        <div className="section-head">
          <h2>{t('备份与恢复')}</h2>
          <span className="small muted">{t('不含机器 token 与订阅凭据')}</span>
        </div>
        <p>{t('导出默认模板、规则集和编排；恢复时同名规则集、同路径编排会原地更新，不生成重复项。')}</p>
        <div className="actions">
          <button className="primary" onClick={() => window.location.assign('/api/backup')}>
            {t('导出备份')}
          </button>
          <button className="quiet" disabled={restoreBusy} onClick={() => fileInput.current?.click()}>
            {restoreBusy ? t('恢复中…') : t('导入备份')}
          </button>
          <input
            ref={fileInput}
            type="file"
            accept="application/json,.json"
            hidden
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) void restore(file)
              e.target.value = ''
            }}
          />
        </div>
        {restoreWarnings.length > 0 && (
          <div className="warnings">
            <p>{t('恢复时跳过的内容')}</p>
            <ul className="facts">
              {restoreWarnings.map((w) => (
                <li key={w}>
                  <span>{w}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </section>
    </>
  )
}
