import { useState } from 'react'
import { api } from '../api'
import { Icon } from '../icons'
import { notify } from '../ui'
import type { PageProps } from '../App'

type Props = PageProps & {
  theme: 'light' | 'dark' | 'auto'
  setTheme: (t: 'light' | 'dark' | 'auto') => void
}

export function Settings({ state, refresh, theme, setTheme }: Props) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirmNext, setConfirmNext] = useState('')
  const [template, setTemplate] = useState(state.defaultTemplate || state.builtinTemplate)
  const [busy, setBusy] = useState(false)

  const usingBuiltin = !state.defaultTemplate

  async function changePassword() {
    if (next !== confirmNext) {
      notify('两次输入的新口令不一致')
      return
    }
    setBusy(true)
    try {
      await api.changePassword(current, next)
      setCurrent('')
      setNext('')
      setConfirmNext('')
      notify('口令已更新')
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function saveTemplate() {
    setBusy(true)
    try {
      await api.saveSettings({ defaultTemplate: template })
      await refresh()
      notify('默认模板已保存')
    } catch (e) {
      notify((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="title-row">
        <div>
          <h1>设置</h1>
          <p className="sub">面板外观、口令与新建编排的默认模板。</p>
        </div>
      </div>

      <div className="grid">
        <section className="panel span-12">
          <header>
            <h2>外观与实例</h2>
            <span className="meta">{state.demo ? '演示数据（内存库）' : '自托管'}</span>
          </header>
          <div className="panel-split">
            <div>
              <p className="label">主题</p>
              <div className="segmented" style={{ marginTop: 10 }}>
                {(
                  [
                    ['auto', '跟随系统'],
                    ['light', '浅色'],
                    ['dark', '深色'],
                  ] as const
                ).map(([value, label]) => (
                  <button key={value} aria-pressed={theme === value} onClick={() => setTheme(value)}>
                    {label}
                  </button>
                ))}
              </div>
              <p className="prose muted" style={{ marginTop: 12 }}>
                可安装到主屏，深色模式跟随系统。
              </p>
            </div>
            <ul className="facts">
              <li>
                <span>访问地址</span>
                <span className="mono">{state.publicBase}</span>
              </li>
              <li>
                <span>机器 / 节点</span>
                <span>
                  {state.stats.hostsTotal} 台 / {state.stats.nodesTotal} 个
                </span>
              </li>
              <li>
                <span>订阅链接</span>
                <span>{state.stats.subHits} 次拉取 / {state.subscriptions.length} 条</span>
              </li>
            </ul>
          </div>
        </section>

        <section className="panel span-12">
          <header>
            <h2>面板口令</h2>
            <span className="meta">保存在 weft.db，走 PBKDF2</span>
          </header>
          <div className="form-grid">
            <label className="field">
              <span>当前口令</span>
              <input
                type="password"
                autoComplete="current-password"
                value={current}
                onChange={(e) => setCurrent(e.target.value)}
              />
            </label>
            <label className="field">
              <span>新口令</span>
              <input
                type="password"
                autoComplete="new-password"
                value={next}
                onChange={(e) => setNext(e.target.value)}
              />
            </label>
            <label className="field">
              <span>再次输入</span>
              <input
                type="password"
                autoComplete="new-password"
                value={confirmNext}
                onChange={(e) => setConfirmNext(e.target.value)}
              />
            </label>
          </div>
          <div className="row-actions">
            <button
              className="btn btn--primary"
              disabled={busy || !current || next.length < 6}
              onClick={() => void changePassword()}
            >
              更新口令
            </button>
            <button
              className="btn btn--quiet"
              onClick={() => {
                api.logout().then(() => window.location.reload())
              }}
            >
              <Icon name="power" />
              结束会话
            </button>
          </div>
        </section>

        <section className="panel span-12">
          <header>
            <h2>默认配置模板</h2>
            <span className="meta">{usingBuiltin ? '当前使用内置模板' : '已自定义'}</span>
          </header>
          <p className="prose">
            节点填入 <code className="mono">PROXY</code> 与 <code className="mono">AUTO</code>
            ，其余段落原样保留；清空保存即恢复内置模板。
          </p>
          <textarea
            className="code-input"
            rows={14}
            value={template}
            onChange={(e) => setTemplate(e.target.value)}
          />
          <div className="row-actions">
            <button className="btn btn--primary" disabled={busy} onClick={() => void saveTemplate()}>
              保存模板
            </button>
            <button
              className="btn btn--quiet"
              onClick={() => {
                setTemplate(state.builtinTemplate)
                notify('已载入内置模板，保存后生效')
              }}
            >
              恢复内置模板
            </button>
          </div>
        </section>
      </div>
    </>
  )
}
