import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { Mark } from '../icons'

export function Login({
  needsSetup,
  demo,
  onDone,
}: {
  needsSetup: boolean
  demo: boolean
  onDone: () => void
}) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
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

  return (
    <div className="login-shell">
      <section className="login-intro">
        <p className="brand login-brand">
          <Mark size={26} />
          <span>Weft</span>
        </p>
        <h1>{needsSetup ? t('先给面板设一个口令。') : t('把多台 mihomo 织成一条订阅。')}</h1>
        <p>
          {needsSetup
            ? t('口令保存在这台服务器上，之后用它进入面板。')
            : t('机器只上报，节点直连；Weft 只负责把它们织在一起。')}
        </p>
        <ul className="login-points">
          <li>{t('机器只读出站上报，不开放入站端口')}</li>
          <li>{t('订阅直连节点，流量不经过 Weft')}</li>
          <li>{t('一条链接，随时吊销')}</li>
        </ul>
      </section>

      <form className="login-card" onSubmit={submit}>
        <h2>{needsSetup ? t('初始化') : t('登录')}</h2>
        <p className="login-lede">
          {needsSetup ? t('口令至少 6 位，保存在这台服务器上。') : t('输入口令后继续，登录状态保存在这台设备上。')}
        </p>

        <label className="field">
          <span>{needsSetup ? t('设置口令') : t('面板口令')}</span>
          <input
            type="password"
            autoFocus
            autoComplete={needsSetup ? 'new-password' : 'current-password'}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {needsSetup && (
          <label className="field">
            <span>{t('再次输入')}</span>
            <input
              type="password"
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
        {demo && <p className="login-hint">{t('演示实例口令：weft-demo')}</p>}
      </form>
    </div>
  )
}
