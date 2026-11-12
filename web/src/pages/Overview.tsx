import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ago, clock, type ProxyNode, type SeriesPoint } from '../api'
import { NodeThreads } from '../weave'
import { Copy, Empty, StatusPill } from '../ui'
import type { PageProps } from '../App'

export function Overview({ state, refresh, navigate }: PageProps) {
  const { t } = useTranslation()
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
  const agentCmd = `weft agent --server ${state.publicBase} --token <host token> --controller http://127.0.0.1:9090 --secret <mihomo secret>`

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('概览')}</h1>
          <p className="sub">
            {t('{{online}} 台在线 · {{alive}} 个节点可用', { online: stats.hostsOnline, alive: stats.nodesAlive })}
            {state.demo && <span className="badge"> {t('演示数据')}</span>}
          </p>
        </div>
        <div className="actions">
          <button className="quiet" onClick={() => void refresh()}>
            {t('刷新')}
          </button>
          <button className="primary" onClick={() => navigate('/bundles/new')}>
            {t('新建编排')}
          </button>
        </div>
      </div>

      <section>
        <dl className="stats">
          <div>
            <dt>{t('在线机器')}</dt>
            <dd>
              {stats.hostsOnline}
              <small>/ {stats.hostsTotal}</small>
            </dd>
            <dd className="note">
              {stats.hostsOnline === stats.hostsTotal ? t('全部已上报') : t('有机器未上报')}
            </dd>
          </div>
          <div>
            <dt>{t('可用节点')}</dt>
            <dd>
              {stats.nodesAlive}
              <small>/ {stats.nodesTotal}</small>
            </dd>
            <dd className="note">
              {t('存活率 {{pct}}%', {
                pct: stats.nodesTotal ? Math.round((stats.nodesAlive / stats.nodesTotal) * 100) : 0,
              })}
            </dd>
          </div>
          <div>
            <dt>{t('订阅拉取')}</dt>
            <dd>{stats.subHits}</dd>
            <dd className="note">
              {t('{{links}} 条链接 · {{devices}} 台设备', { links: subscriptions.length, devices: deviceCount })}
            </dd>
          </div>
          <div>
            <dt>{t('最近上报')}</dt>
            <dd className="mono small">{ago(lastReport)}</dd>
            <dd className="note">{t('agent 变动即报，每小时心跳')}</dd>
          </div>
        </dl>
      </section>

      <TrendPanel series={series} />

      <div className="grid two">
        <section>
          <div className="section-head">
            <h2>{t('机器')}</h2>
            <button className="text small" onClick={() => navigate('/hosts')}>
              {t('全部')}
            </button>
          </div>
          {sortedHosts.length === 0 ? (
            <Empty title={t('还没有机器')} body={t('接入第一台运行 mihomo 的主机后，这里会显示它的状态。')} />
          ) : (
            <table>
              <tbody>
                {sortedHosts.slice(0, 4).map((h) => (
                  <tr key={h.id} className="click" onClick={() => navigate('/hosts')}>
                    <td>
                      <span className="cell-main">{h.name}</span>
                      <span className="cell-sub">
                        {t('{{alive}}/{{total}} 节点 · {{ago}}', {
                          alive: h.aliveCount,
                          total: h.nodeCount,
                          ago: ago(h.lastSeen),
                        })}
                      </span>
                    </td>
                    <td>
                      <NodeThreads nodes={nodesByHost.get(h.id) ?? []} />
                    </td>
                    <td>
                      <StatusPill status={h.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p className="small">
            <button className="text small" onClick={() => navigate('/hosts')}>
              {t('管理机器与接入命令')}
            </button>
          </p>
        </section>

        <section>
          <div className="section-head">
            <h2>{t('订阅')}</h2>
            <button className="text small" onClick={() => navigate('/subscriptions')}>
              {t('管理')}
            </button>
          </div>
          {subscriptions.length === 0 ? (
            <Empty title={t('没有订阅链接')} body={t('从一条编排生成链接，客户端就能拉取聚合后的配置。')} />
          ) : (
            <table>
              <tbody>
                {subscriptions.slice(0, 4).map((s) => {
                  const top = (s.devices ?? [])[0]
                  return (
                    <tr key={s.id}>
                      <td>
                        <span className={s.enabled ? 'cell-main' : 'cell-main muted'}>{s.label || t('未命名')}</span>
                        <span className="cell-sub">{s.bundleName}</span>
                      </td>
                      <td className="small muted">{top ? top.device : '—'}</td>
                      <td className="num" title={t('拉取')}>
                        {s.hits}
                      </td>
                      <td className="small muted num">{s.lastUsedAt ? ago(s.lastUsedAt) : '—'}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
          <p className="small">
            <button className="text small" onClick={() => navigate('/subscriptions')}>
              {t('管理订阅与设备')}
            </button>
          </p>
        </section>
      </div>

      <DelayPanel nodes={state.nodes} />

      <section>
        <h2>{t('接入下一台机器')}</h2>
        <pre>
          <code>{agentCmd}</code>
        </pre>
        <div className="actions">
          <Copy value={agentCmd} label={t('复制模板')} />
          <button className="text small" onClick={() => navigate('/hosts')}>
            {t('生成 token')}
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
  const { t } = useTranslation()
  const [metric, setMetric] = useState<Metric>('delay')
  // a report with no live node has no delay to average; drop it rather than draw a zero
  const series = metric === 'delay' ? all.filter((p) => p.delay > 0) : all
  const last = series[series.length - 1]
  const first = series[0]
  const value = (p: SeriesPoint) => (metric === 'delay' ? p.delay : p.alive)
  const unit = metric === 'delay' ? 'ms' : t('个')
  const change = last && first ? value(last) - value(first) : 0
  // delay is better when it falls, alive nodes when they rise
  const good = metric === 'delay' ? change < 0 : change > 0

  return (
    <section>
      <div className="section-head">
        <h2>{metric === 'delay' ? t('平均延迟') : t('可用节点')}</h2>
        <div className="tabs" role="group" aria-label={t('指标')}>
          <button aria-pressed={metric === 'delay'} onClick={() => setMetric('delay')}>
            {t('延迟')}
          </button>
          <button aria-pressed={metric === 'alive'} onClick={() => setMetric('alive')}>
            {t('可用节点')}
          </button>
        </div>
      </div>
      {last && (
        <p className="trend-value">
          <strong className="num">{value(last)}</strong>
          <span className="unit">{unit}</span>
          {change !== 0 && (
            <span className={`delta num ${good ? 'is-good' : 'is-bad'}`}>
              {change > 0 ? '+' : '−'}
              {Math.abs(change)} {unit}
            </span>
          )}
          <span className="window">{t('较 6 小时前')}</span>
        </p>
      )}
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
  const { t } = useTranslation()
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
    return <div className="chart-empty">{t('还没有足够的上报数据')}</div>
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
    <figure className="chart chart-wrap" ref={box}>
      <svg
        width={W}
        height={H}
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={t('{{metric}}曲线', { metric: metric === 'delay' ? t('平均延迟') : t('可用节点') })}
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
              {t('总计 {{n}}', { n: capacity })}
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
          className="chart-tip"
          style={{ left: `${(x(hover) / W) * 100}%`, transform: `translateX(${hover > series.length / 2 ? '-108%' : '8%'})` }}
        >
          <span>{clock(h.at)}</span> <b className="num">{value(h)} {unit}</b>
        </div>
      )}
    </figure>
  )
}

const BIN = 50

function DelayPanel({ nodes }: { nodes: ProxyNode[] }) {
  const { t } = useTranslation()
  const alive = nodes.filter((n) => n.alive).map((n) => n.delayMs).sort((a, b) => a - b)
  const down = nodes.length - alive.length
  if (nodes.length === 0) {
    return (
      <section>
        <h2>{t('延迟分布')}</h2>
        <Empty title={t('还没有节点')} body={t('机器上报后，这里按 50 ms 分档显示节点延迟。')} />
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
    <section>
      <div className="section-head">
        <h2>{t('延迟分布')}</h2>
        <span className="small muted">{t('{{n}} 个可用节点 · 每档 50 ms', { n: alive.length })}</span>
      </div>
      <dl className="dist-stats">
        <div>
          <dt>{t('中位数')}</dt>
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
          <dt>{t('低于 100 ms')}</dt>
          <dd className="num">
            {fast}
            <span>{t('个')}</span>
          </dd>
        </div>
        <div>
          <dt>{t('不可用')}</dt>
          <dd className={`num${down ? ' is-bad' : ''}`}>
            {down}
            <span>{t('个')}</span>
          </dd>
        </div>
      </dl>
      <div
        className="dist"
        role="img"
        aria-label={bins
          .map((b) => t('{{from}}–{{to}} ms：{{count}} 个', { from: b.from, to: b.from + BIN, count: b.count }))
          .join(', ')}
      >
        {bins.map((b) => (
          <div
            key={b.from}
            className="dist-col"
            title={t('{{from}}–{{to}} ms · {{count}} 个', { from: b.from, to: b.from + BIN, count: b.count })}
          >
            <span className="dist-count num">{b.count || ''}</span>
            <span
              className="dist-bar"
              data-fast={b.from < 100}
              style={{ height: `${(b.count / peak) * 100}%` }}
            />
          </div>
        ))}
      </div>
      <div className="dist-axis num">
        {bins.map((b) => (
          <span key={b.from}>{b.from}</span>
        ))}
        <span>{top}</span>
      </div>
    </section>
  )
}
