import { useState } from 'react'
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
    <div className="auth">
      <form className="auth__card" onSubmit={submit}>
        <div className="auth__brand">
          <Mark size={26} />
          <strong>weft</strong>
        </div>
        <p className="auth__lede">
          {needsSetup ? t('先给面板设一个口令。') : t('把多台 mihomo 织成一条订阅。')}
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
        {error && <p className="form-error">{error}</p>}
        <button className="btn btn--primary btn--block" disabled={busy || !password}>
          {needsSetup ? t('完成初始化') : t('进入面板')}
        </button>
        {demo && <p className="auth__hint">{t('演示实例口令：weft-demo')}</p>}
      </form>
    </div>
  )
}
