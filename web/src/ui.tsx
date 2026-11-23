import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon, type IconName } from './icons'

export type ToastTone = 'default' | 'ok' | 'bad'

export function notify(message: string, tone: ToastTone = 'default') {
  window.dispatchEvent(new CustomEvent('weft:toast', { detail: { message, tone } }))
}

export function Toaster() {
  const [toast, setToast] = useState<{ message: string; tone: ToastTone; id: number } | null>(null)
  const timer = useRef<number | null>(null)
  useEffect(() => {
    function onToast(e: Event) {
      const detail = (e as CustomEvent<string | { message: string; tone?: ToastTone }>).detail
      const next =
        typeof detail === 'string'
          ? { message: detail, tone: 'default' as ToastTone, id: Date.now() }
          : { message: detail.message, tone: detail.tone ?? 'default', id: Date.now() }
      setToast(next)
      if (timer.current) window.clearTimeout(timer.current)
      timer.current = window.setTimeout(() => setToast(null), 2800)
    }
    window.addEventListener('weft:toast', onToast)
    return () => window.removeEventListener('weft:toast', onToast)
  }, [])
  if (!toast) return null
  return (
    <div key={toast.id} className="toast" data-tone={toast.tone} role="status" aria-live="polite">
      {toast.tone === 'ok' && <Icon name="check" size={14} />}
      {toast.tone === 'bad' && <Icon name="alert" size={14} />}
      <span>{toast.message}</span>
    </div>
  )
}

export function Copy({
  value,
  label = '复制',
  small = true,
  iconOnly = false,
}: {
  value: string
  label?: string
  small?: boolean
  iconOnly?: boolean
}) {
  const { t } = useTranslation()
  const [done, setDone] = useState(false)
  return (
    <button
      type="button"
      className={`${small ? 'quiet small' : 'quiet'}${done ? ' is-copied' : ''}${iconOnly ? ' icon-btn' : ''}`}
      aria-label={done ? t('已复制') : t(label)}
      title={done ? t('已复制') : t(label)}
      onClick={async (e) => {
        e.stopPropagation()
        try {
          await navigator.clipboard.writeText(value)
        } catch {
          const el = document.createElement('textarea')
          el.value = value
          el.setAttribute('readonly', '')
          el.style.position = 'fixed'
          el.style.top = '-9999px'
          el.style.left = '-9999px'
          el.style.opacity = '0'
          el.style.fontSize = '16px'
          document.body.appendChild(el)
          el.select()
          document.execCommand('copy')
          el.remove()
        }
        setDone(true)
        window.setTimeout(() => setDone(false), 1500)
      }}
    >
      <Icon name={done ? 'check' : 'copy'} size={14} />
      {!iconOnly && <span>{done ? t('已复制') : t(label)}</span>}
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

let openDialogCount = 0

/** A native dialog: the browser owns the backdrop, focus and Escape. */
function Dialog({
  title,
  subtitle,
  onClose,
  children,
  footer,
  wide,
  variant = 'modal',
}: {
  title: string
  subtitle?: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
  variant?: 'modal' | 'drawer'
}) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el || el.open) return
    openDialogCount += 1
    document.documentElement.classList.add('dialog-open')
    const coarse = window.matchMedia?.('(pointer: coarse)').matches
    const focusFirst = () => {
      if (coarse) {
        el.focus()
        return
      }
      const first = el.querySelector<HTMLElement>(
        'input:not([type="hidden"]):not([type="file"]):not([readonly]), select, textarea',
      )
      ;(first ?? el).focus()
    }
    const releaseScroll = () => {
      openDialogCount = Math.max(0, openDialogCount - 1)
      if (openDialogCount === 0) document.documentElement.classList.remove('dialog-open')
    }
    if (typeof el.showModal === 'function') {
      el.showModal()
      focusFirst()
      return () => {
        if (el.open) el.close()
        releaseScroll()
      }
    }
    el.setAttribute('open', '')
    el.classList.add('fallback')
    focusFirst()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      releaseScroll()
    }
  }, [])
  const classes = [variant === 'drawer' ? 'drawer' : '', wide ? 'wide' : ''].filter(Boolean).join(' ')
  return (
    <dialog
      ref={ref}
      className={classes || undefined}
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
      <div className="dialog-shell">
        <div className="dialog-head">
          <div className="dialog-title">
            <h2>{title}</h2>
            {subtitle && <p className="dialog-sub">{subtitle}</p>}
          </div>
          <button type="button" className="icon icon-close" onClick={onClose} aria-label={t('关闭')}>
            <Icon name="close" size={16} />
          </button>
        </div>
        <div className="dialog-body">{children}</div>
        {footer && <div className="dialog-foot">{footer}</div>}
      </div>
    </dialog>
  )
}

export function Modal(props: {
  title: string
  subtitle?: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  return <Dialog {...props} variant="modal" />
}

export function Drawer(props: {
  title: string
  subtitle?: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  return <Dialog {...props} variant="drawer" />
}

export function ConfirmModal({
  title,
  message,
  confirmLabel,
  danger = true,
  busy = false,
  onConfirm,
  onClose,
}: {
  title: string
  message: string
  confirmLabel?: string
  danger?: boolean
  busy?: boolean
  onConfirm: () => void | Promise<void>
  onClose: () => void
}) {
  const { t } = useTranslation()
  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="quiet" onClick={onClose} disabled={busy}>
            {t('取消')}
          </button>
          <button
            type="button"
            className={danger ? 'danger-solid' : 'primary'}
            disabled={busy}
            onClick={() => void onConfirm()}
          >
            {confirmLabel ?? t('确认')}
          </button>
        </>
      }
    >
      <p className="confirm-message">{message}</p>
    </Modal>
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
  icon,
  title,
  body,
  action,
}: {
  icon?: IconName
  title: string
  body: string
  action?: ReactNode
}) {
  return (
    <div className="empty">
      {icon && (
        <span className="empty-icon">
          <Icon name={icon} size={22} />
        </span>
      )}
      <strong>{title}</strong>
      <p>{body}</p>
      {action && <div className="empty-action">{action}</div>}
    </div>
  )
}
