import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { Icon, Mark, type IconName } from '../icons'
import { setLocale } from '../i18n'
import type { Theme } from '../App'

export function Login({
  needsSetup,
  demo,
  theme,
  onCycleTheme,
  onDone,
}: {
  needsSetup: boolean
  demo: boolean
  theme: Theme
  onCycleTheme: () => void
  onDone: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language === 'en' ? 'en' : 'zh'
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    document.body.classList.add('auth')
    return () => document.body.classList.remove('auth')
  }, [])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (needsSetup) {
      if (password.length < 6) {
        setError(t('口令至少 6 位'))
        return
      }
      if (password !== confirm) {
        setError(t('两次输入不一致'))
        return
      }
    }
    setBusy(true)
    try {
      if (needsSetup) await api.setup(password)
      else await api.login(password)
      onDone()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const themeIcon: IconName = theme === 'dark' ? 'moon' : theme === 'light' ? 'sun' : 'monitor'

  return (
    <div className="login-page">
      <div className="login-tools">
        <button
          type="button"
          className="quiet small"
          onClick={() => setLocale(locale === 'zh' ? 'en' : 'zh')}
        >
          <Icon name="globe" size={14} />
          <span>{locale === 'zh' ? 'EN' : '中文'}</span>
        </button>
        <button
          type="button"
          className="quiet small icon-btn"
          onClick={onCycleTheme}
          aria-label={t('切换主题')}
          title={t('切换主题')}
        >
          <Icon name={themeIcon} size={14} />
        </button>
      </div>

      <div className="login-shell">
        <section className="login-intro">
          <p className="brand login-brand">
            <Mark size={26} />
            <span>Weft</span>
          </p>
          <h1>{needsSetup ? t('先给面板设一个口令。') : t('把多台 mihomo 织成一条订阅。')}</h1>
          <p className="login-desc">
            {needsSetup
              ? t('口令保存在这台服务器上，之后用它进入面板。')
              : t('机器只上报，节点直连；Weft 只负责把它们织在一起。')}
          </p>
          <ul className="login-points">
            <li>
              <Icon name="hosts" size={15} />
              <span>{t('机器只读出站上报，不开放入站端口')}</span>
            </li>
            <li>
              <Icon name="layers" size={15} />
              <span>{t('订阅直连节点，流量不经过 Weft')}</span>
            </li>
            <li>
              <Icon name="link" size={15} />
              <span>{t('一条链接，随时吊销')}</span>
            </li>
          </ul>
        </section>

        <form className="login-card panel" onSubmit={submit}>
          <h2>{needsSetup ? t('初始化') : t('登录')}</h2>
          <p className="login-lede">
            {needsSetup
              ? t('口令至少 6 位，保存在这台服务器上。')
              : t('输入口令后继续，登录状态保存在这台设备上。')}
          </p>

          <label className="field">
            <span>{needsSetup ? t('设置口令') : t('面板口令')}</span>
            <div className="input-wrap">
              <input
                type={showPass ? 'text' : 'password'}
                autoFocus
                autoComplete={needsSetup ? 'new-password' : 'current-password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <button
                type="button"
                className="icon input-eye"
                tabIndex={-1}
                aria-label={showPass ? t('隐藏口令') : t('显示口令')}
                onClick={() => setShowPass((v) => !v)}
              >
                <Icon name={showPass ? 'eyeOff' : 'eye'} size={15} />
              </button>
            </div>
          </label>
          {needsSetup && (
            <label className="field">
              <span>{t('再次输入')}</span>
              <input
                type={showPass ? 'text' : 'password'}
                autoComplete="new-password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
              />
            </label>
          )}
          {error && <p className="error">{error}</p>}
          <button className="primary wide login-submit" disabled={busy || !password}>
            {needsSetup ? t('完成初始化') : t('进入面板')}
          </button>
          {demo && (
            <div className="login-demo">
              <span className="muted small">{t('演示实例口令：weft-demo')}</span>
              <button
                type="button"
                className="text small"
                onClick={() => setPassword('weft-demo')}
              >
                {t('填入演示口令')}
              </button>
            </div>
          )}
        </form>
      </div>
    </div>
  )
}
