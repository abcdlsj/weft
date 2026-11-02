import { useMemo } from 'react'
import { ago, clock, type SeriesPoint } from '../api'
import { Icon } from '../icons'
import { Empty, StatusPill } from '../ui'
import type { PageProps } from '../App'

export function Overview({ state, refresh, navigate }: PageProps) {
  const { stats, hosts, subscriptions, series } = state
  const lastReport = hosts.length ? Math.max(...hosts.map((h) => h.lastSeen)) : 0
  const deviceCount = new Set(subscriptions.flatMap((s) => (s.devices ?? []).map((d) => d.device))).size
  const sortedHosts = useMemo(
    () => [...hosts].sort((a, b) => b.aliveCount / (b.nodeCount || 1) - a.aliveCount / (a.nodeCount || 1)),
    [hosts],
  )

  return (
    <>
      <div className="title-row">
        <div>
          <h1>概览</h1>
          <p className="sub">
            {stats.hostsOnline} 台在线 · {stats.nodesAlive} 个节点可用
            {state.demo && <span className="tag">演示数据</span>}
          </p>
        </div>
        <div className="title-actions">
          <button className="btn btn--quiet" onClick={() => void refresh()}>
            <Icon name="refresh" />
            刷新
          </button>
          <button className="btn btn--primary" onClick={() => navigate('/bundles/new')}>
            <Icon name="plus" />
            新建编排
          </button>
        </div>
      </div>

      <section className="panel kpi">
        <div className="kpi__cell">
          <p className="label">在线机器</p>
          <p className="kpi__value">
            {stats.hostsOnline}
            <span className="kpi__unit">/ {stats.hostsTotal}</span>
          </p>
          <p className="kpi__meta">{stats.hostsOnline === stats.hostsTotal ? '全部已上报' : '有机器未上报'}</p>
        </div>
        <div className="kpi__cell">
          <p className="label">可用节点</p>
          <p className="kpi__value">
            {stats.nodesAlive}
            <span className="kpi__unit">/ {stats.nodesTotal}</span>
          </p>
          <p className="kpi__meta">
            存活率 {stats.nodesTotal ? Math.round((stats.nodesAlive / stats.nodesTotal) * 100) : 0}%
          </p>
        </div>
        <div className="kpi__cell">
          <p className="label">订阅拉取</p>
          <p className="kpi__value">{stats.subHits}</p>
          <p className="kpi__meta">
            {subscriptions.length} 条链接 · {deviceCount} 台设备
          </p>
        </div>
        <div className="kpi__cell">
          <p className="label">最近上报</p>
          <p className="kpi__value kpi__value--sm">{ago(lastReport)}</p>
          <p className="kpi__meta">agent 每 15 秒同步一次</p>
        </div>
      </section>

      <div className="grid grid--overview">
        <section className="panel span-8">
          <header>
            <h2>平均延迟</h2>
            <span className="meta">
              <b className="num">{series.length ? series[series.length - 1].delay : 0} ms</b> · 近 6
              小时
            </span>
          </header>
          <AliveChart series={series} />
        </section>

        <section className="panel span-4">
          <header>
            <h2>机器</h2>
            <button className="card-link" onClick={() => navigate('/hosts')}>
              全部
            </button>
          </header>
          {sortedHosts.length === 0 ? (
            <Empty title="还没有机器" body="接入第一台运行 mihomo 的主机后，这里会显示它的状态。" />
          ) : (
            <>
              <ul className="hostlist">
                {sortedHosts.slice(0, 4).map((h) => (
                  <li key={h.id}>
                    <button className="hostlist__row" onClick={() => navigate('/hosts')}>
                      <span className="hostlist__name">{h.name}</span>
                      <span className="hostlist__meta">
                        {h.aliveCount}/{h.nodeCount} 节点 · {ago(h.lastSeen)}
                      </span>
                      <StatusPill status={h.status} />
                    </button>
                  </li>
                ))}
              </ul>
              <div className="panel__foot">
                <button className="card-link" onClick={() => navigate('/hosts')}>
                  管理机器与接入命令
                </button>
              </div>
            </>
          )}
        </section>

        <section className="panel span-7">
          <header>
            <h2>订阅</h2>
            <button className="card-link" onClick={() => navigate('/subscriptions')}>
              管理
            </button>
          </header>
          {subscriptions.length === 0 ? (
            <Empty title="没有订阅链接" body="从一条编排生成链接，客户端就能拉取聚合后的配置。" />
          ) : (
            <>
            <table className="table table--subs">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>设备</th>
                  <th className="num">拉取</th>
                  <th>最近</th>
                </tr>
              </thead>
              <tbody>
                {subscriptions.slice(0, 4).map((s) => {
                  const top = (s.devices ?? [])[0]
                  return (
                    <tr key={s.id}>
                      <td data-label="名称">
                        <span className={s.enabled ? '' : 'muted'}>{s.label || '未命名'}</span>
                        <span className="cell-sub">{s.bundleName}</span>
                      </td>
                      <td data-label="设备" className="muted">
                        {top ? (
                          <span className="device-inline">
                            <Icon name="device" />
                            {top.device}
                          </span>
                        ) : (
                          '—'
                        )}
                      </td>
                      <td className="num" data-label="拉取">
                        {s.hits}
                      </td>
                      <td data-label="最近" className="muted">
                        {s.lastUsedAt ? ago(s.lastUsedAt) : '—'}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
            <div className="panel__foot">
              <button className="card-link" onClick={() => navigate('/subscriptions')}>
                管理订阅与设备
              </button>
            </div>
            </>
          )}
        </section>

        <section className="panel span-5">
          <header>
            <h2>接入下一台机器</h2>
          </header>
          <p className="prose">
            机器上跑 agent，读出本机 mihomo 的节点并主动上报；不需要开端口，也不需要 SSH 凭据。
          </p>
          <pre className="code">{`weft agent \\
  --server ${state.publicBase} \\
  --token <机器 token> \\
  --controller http://127.0.0.1:9090 \\
  --secret <mihomo secret>`}</pre>
          <div className="panel__foot">
            <button className="btn btn--quiet" onClick={() => navigate('/hosts')}>
              <Icon name="plus" />
              生成 token 并复制完整命令
            </button>
          </div>
        </section>
      </div>
    </>
  )
}

function AliveChart({ series }: { series: SeriesPoint[] }) {
  if (series.length < 2) {
    return (
      <div className="chart chart--empty">
        <p className="empty__body">还没有足够的上报数据</p>
      </div>
    )
  }
  const W = 720
  const H = 176
  const padX = 8
  const padY = 12
  const peak = Math.max(...series.map((p) => p.delay), 1)
  const max = Math.ceil(peak / 25) * 25
  const stepX = (W - padX * 2) / (series.length - 1)
  const y = (v: number) => H - padY - (Math.min(v, max) / max) * (H - padY * 2)
  const line = series.map((p, i) => `${i === 0 ? 'M' : 'L'}${padX + i * stepX},${y(p.delay)}`).join(' ')
  const last = series[series.length - 1]
  const first = series[0]

  return (
    <figure className="chart">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label="平均延迟曲线">
        {[0, 0.5, 1].map((f) => (
          <line
            key={f}
            x1={padX}
            x2={W - padX}
            y1={y(max * f)}
            y2={y(max * f)}
            className={f === 0 ? 'chart__base' : 'chart__grid'}
          />
        ))}
        <path d={line} className="chart__line" vectorEffect="non-scaling-stroke" />
        <circle cx={padX + (series.length - 1) * stepX} cy={y(last.delay)} r="3" className="chart__dot" />
      </svg>
      <div className="chart__axis">
        <span>{clock(first.at)}</span>
        <span className="chart__scale">0 – {max} ms</span>
        <span>{clock(last.at)}</span>
      </div>
    </figure>
  )
}
