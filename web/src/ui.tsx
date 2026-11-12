import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

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
    <p className="toast" role="status">
      {msg}
    </p>
  )
}

export function Copy({ value, label = '复制' }: { value: string; label?: string }) {
  const { t } = useTranslation()
  const [done, setDone] = useState(false)
  return (
    <button
      type="button"
      className="quiet small"
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
      {done ? t('已复制') : t(label)}
    </button>
  )
}

export function StatusPill({ status }: { status: string }) {
  const { t } = useTranslation()
  const label =
    status === 'online'
      ? t('在线')
      : status === 'stale'
        ? t('待复核')
        : status === 'offline'
          ? t('离线')
          : status === 'manual'
            ? t('手工')
            : t('未接入')
  const tone = status === 'online' ? 'ok' : status === 'stale' ? 'warn' : status === 'offline' ? 'bad' : 'muted'
  return (
    <span className="status" data-tone={tone}>
      {label}
    </span>
  )
}

/** A native dialog: the browser owns the backdrop, focus and Escape. */
function Dialog({
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
  const { t } = useTranslation()
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el || el.open) return
    // Safari 15.4 added showModal; older engines still get a usable fixed box.
    const focusFirst = () => {
      const first = el.querySelector<HTMLElement>(
        'input:not([type="hidden"]):not([type="file"]), select, textarea',
      )
      ;(first ?? el).focus()
    }
    if (typeof el.showModal === 'function') {
      el.showModal()
      focusFirst()
      return
    }
    el.setAttribute('open', '')
    el.classList.add('fallback')
    focusFirst()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  return (
    <dialog
      ref={ref}
      className={wide ? 'wide' : undefined}
      aria-label={title}
      tabIndex={-1}
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
      onClick={(e) => {
        if (e.target === ref.current) onClose()
      }}
    >
      <div className="dialog-head">
        <h2>{title}</h2>
        <button type="button" className="icon" onClick={onClose} aria-label={t('关闭')}>
          ✕
        </button>
      </div>
      <div className="dialog-body">{children}</div>
      {footer && <div className="dialog-foot">{footer}</div>}
    </dialog>
  )
}

export function Modal(props: {
  title: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  return <Dialog {...props} />
}

export function Drawer(props: {
  title: string
  onClose: () => void
  children: ReactNode
  wide?: boolean
}) {
  return <Dialog {...props} wide />
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
    <p className="empty">
      <strong>{title}</strong>
      {body}
      {action}
    </p>
  )
}
