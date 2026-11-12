import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, setCSRF, type FleetState, type Me } from './api'
import { Mark } from './icons'
import { Toaster, notify } from './ui'
import { Login } from './pages/Login'
import { Overview } from './pages/Overview'
import { Hosts } from './pages/Hosts'
import { Bundles } from './pages/Bundles'
import { BundleEditor } from './pages/BundleEditor'
import { Subscriptions } from './pages/Subscriptions'
import { Settings } from './pages/Settings'
const Rules = lazy(() => import('./pages/Rules').then((m) => ({ default: m.Rules })))

export interface PageProps {
  state: FleetState
  refresh: () => Promise<void>
  navigate: (path: string) => void
  theme: Theme
  setTheme: (t: Theme) => void
}

type Theme = 'light' | 'dark' | 'auto'

const nav = [
  { path: '/', label: '概览' },
  { path: '/hosts', label: '机器' },
  { path: '/bundles', label: '编排' },
  { path: '/subscriptions', label: '订阅' },
  { path: '/rules', label: '规则' },
  { path: '/settings', label: '设置' },
]

function usePath() {
  const [path, setPath] = useState(window.location.pathname)
  useEffect(() => {
    const onPop = () => setPath(window.location.pathname)
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])
  const navigate = useCallback((next: string) => {
    if (next !== window.location.pathname) {
      window.history.pushState(null, '', next)
      setPath(next)
      window.scrollTo({ top: 0 })
    }
  }, [])
  return [path, navigate] as const
}

export default function App() {
  const { t } = useTranslation()
  const [me, setMe] = useState<Me | null>(null)
  const [state, setState] = useState<FleetState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [path, navigate] = usePath()
  const [theme, setTheme] = useTheme()
  const [updateReady, setUpdateReady] = useState(false)
  const poll = useRef<number | null>(null)
  const baselineVersion = useRef<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const next = await api.state()
      setState(next)
      setError(null)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  useEffect(() => {
    api
      .me()
      .then((m) => {
        setCSRF(m.csrf ?? '')
        setMe(m)
        if (m.authenticated) void refresh()
      })
      .catch(() => setMe({ authenticated: false, needsSetup: false, demo: false }))
  }, [refresh])

  useEffect(() => {
    const onUpdate = () => setUpdateReady(true)
    window.addEventListener('weft:update', onUpdate)
    // The service worker only changes when its own file changes. Comparing the
    // server version catches an app-only deploy while a tab stays open.
    const checkVersion = () => {
      api
        .version()
        .then(({ version }) => {
          if (baselineVersion.current && baselineVersion.current !== version) setUpdateReady(true)
          else baselineVersion.current = version
        })
        .catch(() => {})
    }
    checkVersion()
    const versionPoll = window.setInterval(checkVersion, 5 * 60 * 1000)
    const onVisible = () => {
      if (document.visibilityState === 'visible') checkVersion()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      window.removeEventListener('weft:update', onUpdate)
      window.clearInterval(versionPoll)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [])

  useEffect(() => {
    if (!me?.authenticated) return
    poll.current = window.setInterval(refresh, 15000)
    return () => {
      if (poll.current) window.clearInterval(poll.current)
    }
  }, [me?.authenticated, refresh])

  if (!me) {
    return (
      <div className="boot">
        <Mark size={22} />
      </div>
    )
  }

  if (!me.authenticated) {
    return (
      <>
        <Login
          needsSetup={me.needsSetup}
          demo={me.demo}
          onDone={() => {
            api.me().then((m) => {
              setCSRF(m.csrf ?? '')
              setMe(m)
              void refresh()
            })
          }}
        />
        <Toaster />
      </>
    )
  }

  if (!state) {
    return (
      <div className="boot">
        <Mark size={22} />
        <p>{error ?? t('正在读取集群状态')}</p>
        {error && (
          <button className="quiet" onClick={() => void refresh()}>
            {t('重试')}
          </button>
        )}
      </div>
    )
  }

  const page = renderPage(path, { state, refresh, navigate, theme, setTheme })
  const online = state.stats.hostsOnline

  return (
    <div className="app">
      <header className="top">
        <a className="brand" href="/" onClick={(e) => (e.preventDefault(), navigate('/'))}>
          <Mark />
          <span>weft</span>
        </a>

        <nav className="main-nav">
          {nav.map((item) => {
            const active =
              item.path === '/' ? path === '/' : path === item.path || path.startsWith(item.path + '/')
            return (
              <a
                key={item.path}
                href={item.path}
                aria-current={active ? 'page' : undefined}
                onClick={(e) => {
                  e.preventDefault()
                  navigate(item.path)
                }}
              >
                {t(item.label)}
                {item.path === '/hosts' && state.stats.hostsOnline < state.stats.hostsTotal && (
                  <span className="muted"> {state.stats.hostsTotal - state.stats.hostsOnline}</span>
                )}
              </a>
            )
          })}
        </nav>

        <span className="top-aside">
          <span>
            {state.demo ? t('演示实例') : t('{{n}} 台在线', { n: online })}
            {' · '}
            {t('最近上报')}{' '}
            {state.hosts.length ? agoShort(Math.max(...state.hosts.map((h) => h.lastSeen))) : '—'}
          </span>
          <button
            className="quiet small"
            onClick={() => {
              setCSRF('')
              api.logout().catch(() => {}).finally(() => window.location.reload())
            }}
          >
            {t('结束会话')}
          </button>
        </span>
      </header>

      <main>
        <Suspense fallback={<div className="boot"><Mark size={22} /></div>}>{page}</Suspense>
      </main>

      {updateReady && (
        <div className="update" role="status">
          <span>{t('新版本已就绪')}</span>
          <button
            className="primary small"
            onClick={() => window.dispatchEvent(new CustomEvent('weft:apply-update'))}
          >
            {t('刷新面板')}
          </button>
        </div>
      )}
      <Toaster />
    </div>
  )
}

function renderPage(path: string, props: PageProps) {
  if (path === '/') return <Overview {...props} />
  if (path === '/hosts') return <Hosts {...props} />
  if (path === '/bundles/new') return <BundleEditor {...props} />
  if (path.startsWith('/bundles/')) return <BundleEditor {...props} id={path.slice('/bundles/'.length)} />
  if (path === '/bundles') return <Bundles {...props} />
  if (path === '/subscriptions') return <Subscriptions {...props} />
  if (path === '/rules') return <Rules {...props} />
  if (path === '/settings') return <Settings {...props} />
  return <Overview {...props} />
}

function agoShort(ts: number) {
  if (!ts) return '—'
  const s = Math.max(0, Math.floor(Date.now() / 1000 - ts))
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  return `${Math.floor(s / 3600)}h`
}

function useTheme() {
  const stored = (localStorage.getItem('weft-theme') as Theme) || 'auto'
  const [theme, setTheme] = useState<Theme>(stored)
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () => {
      const resolved = theme === 'auto' ? (media.matches ? 'dark' : 'light') : theme
      document.documentElement.dataset.theme = resolved
    }
    apply()
    media.addEventListener('change', apply)
    localStorage.setItem('weft-theme', theme)
    return () => media.removeEventListener('change', apply)
  }, [theme])
  return [theme, setTheme] as const
}

export { notify }
