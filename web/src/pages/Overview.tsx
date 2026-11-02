import { useEffect, useMemo, useRef, useState } from 'react'
import { ago, clock, type ProxyNode, type SeriesPoint } from '../api'
import { NodeThreads } from '../weave'
import { Icon } from '../icons'
import { Copy, Empty, StatusPill } from '../ui'
import type { PageProps } from '../App'

export function Overview({ state, refresh, navigate }: PageProps) {
  const { stats, hosts, subscriptions, series } = state
  const lastReport = hosts.length ? Math.max(...hosts.map((h) => h.lastSeen)) : 0
  const deviceCount = new Set(subscriptions.flatMap((s) => (s.devices ?? []).map((d) => d.device))).size
  const sortedHosts = useMemo(
    () => [...hosts].sort((a, b) => b.aliveCount / (b.nodeCount || 1) - a.aliveCount / (a.nodeCount || 1)),
    [hosts],
  )

  const nodesByHost = useMemo(() => {
    const m = new Map<string, ProxyNode[]>()
    for (const n of state.nodes) m.set(n.hostId, [...(m.get(n.hostId) ?? []), n])
    return m
  }, [state.nodes])
  const agentCmd = `weft agent --server ${state.publicBase} --token <机器 token> --controller http://127.0.0.1:9090 --secret <mihomo secret>`

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
        <TrendPanel series={series} />

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
                      <span className="hostlist__threads">
                        <NodeThreads nodes={nodesByHost.get(h.id) ?? []} />
                      </span>
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

        <DelayPanel nodes={state.nodes} />
      </div>

      <section className="panel connect-strip">
        <h2>接入下一台机器</h2>
        <code className="mono connect-strip__cmd">{agentCmd}</code>
        <div className="connect-strip__actions">
          <Copy value={agentCmd} label="复制模板" />
          <button className="btn btn--quiet btn--sm" onClick={() => navigate('/hosts')}>
            <Icon name="plus" />
            生成 token
          </button>
        </div>
      </section>
    </>
  )
}

type Metric = 'delay' | 'alive'

function niceStep(span: number, ticks: number) {
  const raw = span / ticks
  const mag = 10 ** Math.floor(Math.log10(raw))
  const n = raw / mag
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * mag
}

function TrendPanel({ series: all }: { series: SeriesPoint[] }) {
  const [metric, setMetric] = useState<Metric>('delay')
  // a report with no live node has no delay to average; drop it rather than draw a zero
  const series = metric === 'delay' ? all.filter((p) => p.delay > 0) : all
  const last = series[series.length - 1]
  const first = series[0]
  const value = (p: SeriesPoint) => (metric === 'delay' ? p.delay : p.alive)
  const unit = metric === 'delay' ? 'ms' : '个'
  const change = last && first ? value(last) - value(first) : 0
  // delay is better when it falls, alive nodes when they rise
  const good = metric === 'delay' ? change < 0 : change > 0

  return (
    <section className="panel span-8 trend">
      <header>
        <div className="trend__head">
          <h2>{metric === 'delay' ? '平均延迟' : '可用节点'}</h2>
          {last && (
            <p className="trend__now">
              <span className="trend__value num">{value(last)}</span>
              <span className="trend__unit">{unit}</span>
              {change !== 0 && (
                <span className={`trend__delta num ${good ? 'is-good' : 'is-bad'}`}>
                  {change > 0 ? '+' : '−'}
                  {Math.abs(change)} {unit}
                </span>
              )}
              <span className="trend__window">较 6 小时前</span>
            </p>
          )}
        </div>
        <div className="segmented segmented--sm" role="group" aria-label="指标">
          <button aria-pressed={metric === 'delay'} onClick={() => setMetric('delay')}>
            延迟
          </button>
          <button aria-pressed={metric === 'alive'} onClick={() => setMetric('alive')}>
            可用节点
          </button>
        </div>
      </header>
      <LineChart series={series} value={value} unit={unit} metric={metric} />
    </section>
  )
}

function LineChart({
  series,
  value,
  unit,
  metric,
}: {
  series: SeriesPoint[]
  value: (p: SeriesPoint) => number
  unit: string
  metric: Metric
}) {
  const [hover, setHover] = useState<number | null>(null)
  const box = useRef<HTMLElement>(null)
  const [W, setW] = useState(760)
  useEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setW(Math.max(240, Math.round(e.contentRect.width))))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  if (series.length < 2) {
    return (
      <div className="chart chart--empty">
        <p className="empty__body">还没有足够的上报数据</p>
      </div>
    )
  }
  const H = W < 600 ? 180 : 248
  const left = 34
  const right = 8
  const top = 10
  const bottom = 26
  const values = series.map(value)
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  const ceilTo = metric === 'alive' ? Math.max(...series.map((p) => p.nodes)) : hi
  const step = niceStep(Math.max(ceilTo - lo, metric === 'alive' ? 4 : 10), 4)
  const min = Math.max(0, Math.floor((lo - step / 2) / step) * step)
  let max = Math.ceil(ceilTo / step) * step
  if (max <= hi) max += step
  const ticks: number[] = []
  for (let t = min; t <= max + 1e-9; t += step) ticks.push(Math.round(t * 10) / 10)

  const x = (i: number) => left + (i / (series.length - 1)) * (W - left - right)
  const y = (v: number) => top + (1 - (v - min) / (max - min)) * (H - top - bottom)
  const line = series.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(value(p)).toFixed(1)}`).join(' ')
  const area = `${line} L${x(series.length - 1)},${y(min)} L${x(0)},${y(min)} Z`

  // x labels on whole hours, every hour that falls inside the window
  const hours: number[] = []
  const t0 = series[0].at
  const t1 = series[series.length - 1].at
  const every = W < 600 ? 7200 : 3600
  for (let t = Math.ceil(t0 / every) * every; t <= t1; t += every) hours.push(t)
  const xt = (t: number) => left + ((t - t0) / (t1 - t0)) * (W - left - right)
  const capacity = metric === 'alive' ? series[series.length - 1].nodes : null
  const h = hover === null ? null : series[hover]

  return (
    <figure className="chart" ref={box}>
      <svg
        width={W}
        height={H}
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={`${metric === 'delay' ? '平均延迟' : '可用节点'}曲线`}
        onMouseLeave={() => setHover(null)}
        onMouseMove={(e) => {
          const r = e.currentTarget.getBoundingClientRect()
          const px = ((e.clientX - r.left) / r.width) * W
          const i = Math.round(((px - left) / (W - left - right)) * (series.length - 1))
          setHover(Math.min(series.length - 1, Math.max(0, i)))
        }}
      >
        {ticks.map((t, i) => (
          <g key={t}>
            <line x1={left} x2={W - right} y1={y(t)} y2={y(t)} className={i === 0 ? 'chart__base' : 'chart__grid'} />
            <text x={left - 8} y={y(t) + 4} className="chart__tick" textAnchor="end">
              {t}
            </text>
          </g>
        ))}
        {capacity !== null && (
          <g>
            <line x1={left} x2={W - right} y1={y(capacity)} y2={y(capacity)} className="chart__ref" />
            <text x={W - right} y={y(capacity) - 6} className="chart__tick" textAnchor="end">
              总计 {capacity}
            </text>
          </g>
        )}
        <path d={area} className="chart__area" />
        <path d={line} className="chart__line" />
        {hours.map((t, i) => (
          <text
            key={t}
            x={xt(t)}
            y={H - 6}
            className="chart__tick"
            textAnchor={i === 0 && xt(t) - left < 20 ? 'start' : 'middle'}
          >
            {clock(t)}
          </text>
        ))}
        {h && hover !== null ? (
          <g>
            <line x1={x(hover)} x2={x(hover)} y1={top} y2={H - bottom} className="chart__cursor" />
            <circle cx={x(hover)} cy={y(value(h))} r="4" className="chart__dot" />
          </g>
        ) : (
          <circle cx={x(series.length - 1)} cy={y(values[values.length - 1])} r="3.5" className="chart__dot" />
        )}
      </svg>
      {h && hover !== null && (
        <div
          className="chart__tip"
          style={{ left: `${(x(hover) / W) * 100}%`, transform: `translateX(${hover > series.length / 2 ? '-108%' : '8%'})` }}
        >
          <span>{clock(h.at)}</span>
          <b className="num">
            {value(h)} {unit}
          </b>
        </div>
      )}
    </figure>
  )
}

const BIN = 50

function DelayPanel({ nodes }: { nodes: ProxyNode[] }) {
  const alive = nodes.filter((n) => n.alive).map((n) => n.delayMs).sort((a, b) => a - b)
  const down = nodes.length - alive.length
  if (nodes.length === 0) {
    return (
      <section className="panel span-5">
        <header>
          <h2>延迟分布</h2>
        </header>
        <Empty title="还没有节点" body="机器上报后，这里按 50 ms 分档显示节点延迟。" />
      </section>
    )
  }
  const top = Math.max(BIN * 4, Math.ceil(((alive[alive.length - 1] ?? 0) + 1) / BIN) * BIN)
  const bins = Array.from({ length: top / BIN }, (_, i) => ({
    from: i * BIN,
    count: alive.filter((d) => d >= i * BIN && d < (i + 1) * BIN).length,
  }))
  const peak = Math.max(...bins.map((b) => b.count), 1)
  const pct = (q: number) => (alive.length ? alive[Math.min(alive.length - 1, Math.floor(q * alive.length))] : 0)
  const fast = alive.filter((d) => d < 100).length

  return (
    <section className="panel span-5">
      <header>
        <h2>延迟分布</h2>
        <span className="meta">{alive.length} 个可用节点 · 每档 50 ms</span>
      </header>
      <dl className="dist__stats">
        <div>
          <dt>中位数</dt>
          <dd className="num">
            {pct(0.5)}
            <span>ms</span>
          </dd>
        </div>
        <div>
          <dt>P90</dt>
          <dd className="num">
            {pct(0.9)}
            <span>ms</span>
          </dd>
        </div>
        <div>
          <dt>低于 100 ms</dt>
          <dd className="num">
            {fast}
            <span>个</span>
          </dd>
        </div>
        <div>
          <dt>不可用</dt>
          <dd className={`num${down ? ' is-bad' : ''}`}>
            {down}
            <span>个</span>
          </dd>
        </div>
      </dl>
      <div className="dist" role="img" aria-label={bins.map((b) => `${b.from}–${b.from + BIN} ms：${b.count} 个`).join('，')}>
        {bins.map((b) => (
          <div key={b.from} className="dist__col" title={`${b.from}–${b.from + BIN} ms · ${b.count} 个`}>
            <span className="dist__count num">{b.count || ''}</span>
            <span
              className={b.from < 100 ? 'dist__bar dist__bar--fast' : 'dist__bar'}
              style={{ height: `${(b.count / peak) * 100}%` }}
            />
          </div>
        ))}
      </div>
      <div className="dist__axis num">
        {bins.map((b) => (
          <span key={b.from}>{b.from}</span>
        ))}
        <span>{top}</span>
      </div>
    </section>
  )
}
