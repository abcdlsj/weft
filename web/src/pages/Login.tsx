import { useState } from 'react'
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
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (needsSetup) {
      if (password.length < 6) {
        setError('口令至少 6 位')
        return
      }
      if (password !== confirm) {
        setError('两次输入不一致')
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
          {needsSetup ? '先给面板设一个口令。' : '把多台 mihomo 织成一条订阅。'}
        </p>

        <label className="field">
          <span>{needsSetup ? '设置口令' : '面板口令'}</span>
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
            <span>再次输入</span>
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
          {needsSetup ? '完成初始化' : '进入面板'}
        </button>
        {demo && <p className="auth__hint">演示实例口令：weft-demo</p>}
      </form>
    </div>
  )
}
