import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, setCSRF, type FleetState, type Me } from './api'
import { Icon, Mark, type IconName } from './icons'
import { Toaster, notify } from './ui'
import { Login } from './pages/Login'
import { Overview } from './pages/Overview'
import { Hosts } from './pages/Hosts'
import { Bundles } from './pages/Bundles'
import { BundleEditor } from './pages/BundleEditor'
import { Subscriptions } from './pages/Subscriptions'
import { Settings } from './pages/Settings'
const Rules = lazy(() => import('./pages/Rules').then((m) => ({ default: m.Rules })))

export type Theme = 'light' | 'dark' | 'auto'

export interface PageProps {
  state: FleetState
  refresh: () => Promise<void>
  navigate: (path: string) => void
  search: string
  theme: Theme
  setTheme: (t: Theme) => void
}

const nav: { path: string; label: string; icon: IconName }[] = [
  { path: '/', label: '概览', icon: 'overview' },
  { path: '/hosts', label: '机器', icon: 'hosts' },
  { path: '/bundles', label: '编排', icon: 'layers' },
  { path: '/subscriptions', label: '订阅', icon: 'link' },
  { path: '/rules', label: '规则', icon: 'rules' },
  { path: '/settings', label: '设置', icon: 'gear' },
]

function useLocation() {
  const [loc, setLoc] = useState(() => ({
    pathname: window.location.pathname,
    search: window.location.search,
  }))
  useEffect(() => {
    const onPop = () =>
      setLoc({ pathname: window.location.pathname, search: window.location.search })
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])
  const navigate = useCallback((next: string) => {
    const url = new URL(next, window.location.origin)
    const full = url.pathname + url.search
    if (full !== window.location.pathname + window.location.search) {
      window.history.pushState(null, '', full)
      setLoc({ pathname: url.pathname, search: url.search })
      if (url.pathname !== window.location.pathname) {
        window.scrollTo({ top: 0 })
      }
    }
  }, [])
  return [loc, navigate] as const
}

export default function App() {
  const { t } = useTranslation()
  const [me, setMe] = useState<Me | null>(null)
  const [state, setState] = useState<FleetState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loc, navigate] = useLocation()
  const [theme, setTheme] = useTheme()
  const [updateReady, setUpdateReady] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const poll = useRef<number | null>(null)
  const baselineVersion = useRef<string | null>(null)

  const refresh = useCallback(async () => {
    setRefreshing(true)
    try {
      const next = await api.state()
      setState(next)
      setError(null)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setRefreshing(false)
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

  const cycleTheme = () => {
    const next: Theme = theme === 'auto' ? 'light' : theme === 'light' ? 'dark' : 'auto'
    setTheme(next)
  }

  const themeIcon: IconName = theme === 'dark' ? 'moon' : theme === 'light' ? 'sun' : 'monitor'
  const themeLabel = theme === 'dark' ? t('深色') : theme === 'light' ? t('浅色') : t('跟随系统')

  if (!me) {
    return (
      <div className="boot">
        <Mark size={26} />
      </div>
    )
  }

  if (!me.authenticated) {
    return (
      <>
        <Login
          needsSetup={me.needsSetup}
          demo={me.demo}
          theme={theme}
          onCycleTheme={cycleTheme}
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
        <Mark size={26} />
        <p>{error ?? t('正在读取集群状态')}</p>
        {error && (
          <button className="quiet" onClick={() => void refresh()}>
            <Icon name="refresh" size={14} />
            <span>{t('重试')}</span>
          </button>
        )}
      </div>
    )
  }

  const path = loc.pathname
  const page = renderPage(path, { state, refresh, navigate, search: loc.search, theme, setTheme })
  const online = state.stats.hostsOnline
  const offlineCount = Math.max(0, state.stats.hostsTotal - state.stats.hostsOnline)
  const latestSeen = state.hosts.length ? Math.max(...state.hosts.map((h) => h.lastSeen)) : 0

  return (
    <div className="app">
      <header className="top-bar">
        <div className="top-inner">
          <a className="brand" href="/" onClick={(e) => (e.preventDefault(), navigate('/'))}>
            <Mark size={22} />
            <span>Weft</span>
            {state.demo && <span className="brand-tag">{t('演示数据')}</span>}
          </a>

          <nav className="main-nav" aria-label="Main">
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
                  <Icon name={item.icon} size={15} className="nav-icon" />
                  <span>{t(item.label)}</span>
                  {item.path === '/hosts' && offlineCount > 0 && (
                    <span
                      className="nav-alert num"
                      title={t('{{n}} 台离线或异常', { n: offlineCount })}
                    >
                      {offlineCount}
                    </span>
                  )}
                </a>
              )
            })}
          </nav>

          <div className="top-aside">
            <button
              type="button"
              className="live-chip"
              title={t('刷新')}
              onClick={() => void refresh()}
            >
              <span className="live-dot" data-warn={offlineCount > 0} />
              <span className="num">
                {state.demo ? t('演示实例') : t('{{n}} 台在线', { n: online })}
              </span>
              <span className="live-sep">·</span>
              <span className="num muted">{latestSeen ? agoShort(latestSeen) : '—'}</span>
              {refreshing && <Icon name="refresh" size={12} className="spin" />}
            </button>

            <button
              type="button"
              className="icon top-icon-btn"
              onClick={cycleTheme}
              aria-label={`${t('切换主题')} (${themeLabel})`}
              title={`${t('主题')}: ${themeLabel}`}
            >
              <Icon name={themeIcon} size={16} />
            </button>

            <button
              type="button"
              className="icon top-icon-btn"
              aria-label={t('结束会话')}
              title={t('结束会话')}
              onClick={() => {
                setCSRF('')
                api.logout().catch(() => {}).finally(() => window.location.reload())
              }}
            >
              <Icon name="logout" size={16} />
            </button>
          </div>
        </div>
      </header>

      <main className="page-container">
        <Suspense
          fallback={
            <div className="boot">
              <Mark size={24} />
            </div>
          }
        >
          {page}
        </Suspense>
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
