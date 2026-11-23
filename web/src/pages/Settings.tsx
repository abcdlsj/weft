import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { Icon, type IconName } from '../icons'
import { Copy, notify } from '../ui'
import { setLocale, type Locale } from '../i18n'
import type { PageProps, Theme } from '../App'

export function Settings({ state, refresh, theme, setTheme }: PageProps) {
  const { t, i18n } = useTranslation()
  const locale: Locale = i18n.language === 'en' ? 'en' : 'zh'
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirmNext, setConfirmNext] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [template, setTemplate] = useState(state.defaultTemplate || state.builtinTemplate)
  const [busy, setBusy] = useState(false)
  const [restoreBusy, setRestoreBusy] = useState(false)
  const [restoreWarnings, setRestoreWarnings] = useState<string[]>([])
  const fileInput = useRef<HTMLInputElement>(null)

  const usingBuiltin = !state.defaultTemplate
  const mismatch = confirmNext.length > 0 && next !== confirmNext
  const isStandalone =
    typeof window !== 'undefined' &&
    (window.matchMedia('(display-mode: standalone)').matches ||
      (navigator as Navigator & { standalone?: boolean }).standalone === true)
  const isIOSSafari =
    typeof navigator !== 'undefined' &&
    /iP(hone|od|ad)/.test(navigator.userAgent) &&
    /Safari/.test(navigator.userAgent) &&
    !/CriOS|FxiOS|EdgiOS/.test(navigator.userAgent)

  async function changePassword() {
    if (next !== confirmNext) {
      notify(t('两次输入的新口令不一致'), 'bad')
      return
    }
    setBusy(true)
    try {
      await api.changePassword(current, next)
      setCurrent('')
      setNext('')
      setConfirmNext('')
      notify(t('口令已更新'), 'ok')
    } catch (e) {
      notify((e as Error).message, 'bad')
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
        'ok',
      )
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setRestoreBusy(false)
    }
  }

  async function saveTemplate() {
    setBusy(true)
    try {
      await api.saveSettings({ defaultTemplate: template })
      await refresh()
      notify(t('默认模板已保存'), 'ok')
    } catch (e) {
      notify((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }

  const themeOptions: { value: Theme; label: string; icon: IconName }[] = [
    { value: 'auto', label: '跟随系统', icon: 'monitor' },
    { value: 'light', label: '浅色', icon: 'sun' },
    { value: 'dark', label: '深色', icon: 'moon' },
  ]

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('外观与口令')}</h1>
          <p className="sub">{t('面板外观、口令与新建编排的默认模板。')}</p>
        </div>
      </div>

      <div className="grid two">
        <section className="panel">
          <div className="section-head">
            <h2>{t('外观与实例')}</h2>
            <div className="actions">
              {isStandalone && <span className="badge badge-accent">{t('主屏应用')}</span>}
              <span className="badge">{state.demo ? t('演示数据（内存库）') : t('自托管')}</span>
            </div>
          </div>

          <div className="setting-group">
            <span className="setting-label">{t('主题')}</span>
            <div className="tabs">
              {themeOptions.map((opt) => (
                <button
                  key={opt.value}
                  aria-pressed={theme === opt.value}
                  onClick={() => setTheme(opt.value)}
                >
                  <Icon name={opt.icon} size={14} />
                  <span>{t(opt.label)}</span>
                </button>
              ))}
            </div>
            <p className="small muted">
              {isStandalone
                ? t('当前正以主屏应用模式运行，状态栏与安全区已自动适配。')
                : isIOSSafari
                  ? t('在 Safari 点击底部「共享」→「添加到主屏幕」，即可全屏作为应用使用。')
                  : t('可安装到主屏，深色模式跟随系统。')}
            </p>
          </div>

          <div className="setting-group">
            <span className="setting-label">{t('面板语言')}</span>
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
        </section>

        <section className="panel">
          <div className="section-head">
            <h2>Weft</h2>
            <span className="small muted">{state.demo ? t('演示实例') : t('自托管')}</span>
          </div>
          <ul className="facts">
            <li>
              <span>{t('访问地址')}</span>
              <span className="fact-copy">
                <code className="mono truncate">{state.publicBase}</code>
                <Copy value={state.publicBase} iconOnly />
              </span>
            </li>
            <li>
              <span>{t('机器 / 节点')}</span>
              <span className="num">
                {t('{{hosts}} 台 / {{nodes}} 个', { hosts: state.stats.hostsTotal, nodes: state.stats.nodesTotal })}
              </span>
            </li>
            <li>
              <span>{t('订阅链接')}</span>
              <span className="num">
                {t('{{hits}} 次拉取 / {{n}} 条', { hits: state.stats.subHits, n: state.subscriptions.length })}
              </span>
            </li>
          </ul>
        </section>
      </div>

      <section className="panel">
        <div className="section-head">
          <h2>{t('面板口令')}</h2>
          <div className="actions">
            <span className="small muted">{t('保存在 weft.db，走 Argon2id')}</span>
            <button
              type="button"
              className="text small"
              onClick={() => setShowPass((v) => !v)}
            >
              {showPass ? t('隐藏口令') : t('显示口令')}
            </button>
          </div>
        </div>
        <div className="form-grid three">
          <label className="field">
            <span>{t('当前口令')}</span>
            <input
              type={showPass ? 'text' : 'password'}
              autoComplete="current-password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </label>
          <label className="field">
            <span>{t('新口令')}</span>
            <input
              type={showPass ? 'text' : 'password'}
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </label>
          <label className="field">
            <span>{t('再次输入')}</span>
            <input
              type={showPass ? 'text' : 'password'}
              autoComplete="new-password"
              value={confirmNext}
              onChange={(e) => setConfirmNext(e.target.value)}
            />
            {mismatch && <span className="hint is-bad">{t('两次输入的新口令不一致')}</span>}
          </label>
        </div>
        <div className="actions">
          <button
            className="primary"
            disabled={busy || !current || next.length < 6 || mismatch}
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
            <Icon name="logout" size={14} />
            <span>{t('结束会话')}</span>
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="section-head">
          <h2>{t('默认配置模板')}</h2>
          <span className={`badge${usingBuiltin ? '' : ' badge-accent'}`}>
            {usingBuiltin ? t('当前使用内置模板') : t('已自定义')}
          </span>
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
          <Copy value={template} label={t('复制')} small={false} />
        </div>
      </section>

      <section className="panel">
        <div className="section-head">
          <h2>{t('备份与恢复')}</h2>
          <span className="small muted">{t('不含机器 token 与订阅凭据')}</span>
        </div>
        <p>{t('导出默认模板、规则集和编排；恢复时同名规则集、同路径编排会原地更新，不生成重复项。')}</p>
        <div className="actions">
          <button className="primary" onClick={() => window.location.assign('/api/backup')}>
            <Icon name="download" size={15} />
            <span>{t('导出备份')}</span>
          </button>
          <button className="quiet" disabled={restoreBusy} onClick={() => fileInput.current?.click()}>
            <Icon name="upload" size={15} />
            <span>{restoreBusy ? t('恢复中…') : t('导入备份')}</span>
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
