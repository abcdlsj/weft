import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon } from './icons'

export function notify(message: string) {
  window.dispatchEvent(new CustomEvent('weft:toast', { detail: message }))
}

export function Toaster() {
  const [msg, setMsg] = useState<string | null>(null)
  const timer = useRef<number | null>(null)
  useEffect(() => {
    function onToast(e: Event) {
      setMsg((e as CustomEvent<string>).detail)
      if (timer.current) window.clearTimeout(timer.current)
      timer.current = window.setTimeout(() => setMsg(null), 2600)
    }
    window.addEventListener('weft:toast', onToast)
    return () => window.removeEventListener('weft:toast', onToast)
  }, [])
  if (!msg) return null
  return (
    <div className="toaster" role="status">
      {msg}
    </div>
  )
}

export function Copy({ value, label = '复制' }: { value: string; label?: string }) {
  const [done, setDone] = useState(false)
  return (
    <button
      type="button"
      className="btn btn--quiet btn--sm"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value)
        } catch {
          const el = document.createElement('textarea')
          el.value = value
          document.body.appendChild(el)
          el.select()
          document.execCommand('copy')
          el.remove()
        }
        setDone(true)
        window.setTimeout(() => setDone(false), 1400)
      }}
    >
      <Icon name={done ? 'check' : 'copy'} />
      {done ? '已复制' : label}
    </button>
  )
}

export function StatusPill({ status }: { status: string }) {
  const label =
    status === 'online'
      ? '在线'
      : status === 'stale'
        ? '待复核'
        : status === 'offline'
          ? '离线'
          : status === 'manual'
            ? '手工'
            : '未接入'
  const tone =
    status === 'online' ? 'ok' : status === 'stale' ? 'warn' : status === 'manual' ? 'quiet' : 'neutral'
  return <span className={`pill pill--${tone}`}>{label}</span>
}

export function Modal({
  title,
  onClose,
  children,
  footer,
  wide,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    <div className="scrim" onClick={onClose}>
      <div
        className={`modal${wide ? ' modal--wide' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="modal__head">
          <h2>{title}</h2>
          <button className="icon-btn" onClick={onClose} aria-label="关闭">
            <Icon name="close" />
          </button>
        </header>
        <div className="modal__body">{children}</div>
        {footer && <footer className="modal__foot">{footer}</footer>}
      </div>
    </div>
  )
}

export function Drawer({
  title,
  onClose,
  children,
  wide,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  wide?: boolean
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    <div className="scrim scrim--right" onClick={onClose}>
      <aside className={`drawer${wide ? ' drawer--wide' : ''}`} onClick={(e) => e.stopPropagation()} aria-label={title}>
        <header className="drawer__head">
          <h2>{title}</h2>
          <button className="icon-btn" onClick={onClose} aria-label="关闭">
            <Icon name="close" />
          </button>
        </header>
        <div className="drawer__body">{children}</div>
      </aside>
    </div>
  )
}

export function Switch({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: () => void
  label: string
}) {
  return (
    <button
      type="button"
      className="switch"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={onChange}
    >
      <span />
    </button>
  )
}

export function Empty({
  title,
  body,
  action,
}: {
  title: string
  body: string
  action?: ReactNode
}) {
  return (
    <div className="empty">
      <p className="empty__title">{title}</p>
      <p className="empty__body">{body}</p>
      {action}
    </div>
  )
}
