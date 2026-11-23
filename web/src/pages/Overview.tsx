import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ago,
  clock,
  isNodeAvailable,
  isProbedSource,
  nodeHealth,
  type Host,
  type ProxyNode,
  type SeriesPoint,
} from '../api'
import { Icon } from '../icons'
import { NodeThreads } from '../weave'
import { Copy, Empty, StatusPill } from '../ui'
import type { PageProps } from '../App'

export function Overview({ state, refresh, navigate }: PageProps) {
  const { t } = useTranslation()
  const { stats, hosts, subscriptions, series } = state
  const [spinning, setSpinning] = useState(false)
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

  const hostPct = stats.hostsTotal ? Math.round((stats.hostsOnline / stats.hostsTotal) * 100) : 0
  const alivePct = stats.nodesTotal ? Math.round((stats.nodesAlive / stats.nodesTotal) * 100) : 0
  const hasProbedNodes = state.nodes.some((n) => isProbedSource(n.source))
  const unverifiedTotal = stats.nodesUnverified ?? 0
  const agentCmd = `weft agent --server ${state.publicBase} --token <host token> --controller http://127.0.0.1:9090 --secret <mihomo secret>`

  async function onRefresh() {
    setSpinning(true)
    try {
      await refresh()
    } finally {
      window.setTimeout(() => setSpinning(false), 300)
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{t('{{online}} 台在线', { online: stats.hostsOnline })}</h1>
          <p className="sub">
            {t('{{alive}} 个节点可用 · 共 {{hosts}} 台机器', { alive: stats.nodesAlive, hosts: state.hosts.length })}
          </p>
        </div>
        <div className="actions">
          <button className="quiet" onClick={() => void onRefresh()}>
            <Icon name="refresh" size={15} className={spinning ? 'spin' : undefined} />
            <span>{t('刷新')}</span>
          </button>
          <button className="primary" onClick={() => navigate('/bundles/new')}>
            <Icon name="plus" size={15} />
            <span>{t('新建编排')}</span>
          </button>
        </div>
      </div>

      <section className="stats-grid" aria-label={t('概览')}>
        <button type="button" className="stat-card panel" onClick={() => navigate('/hosts')}>
          <div className="stat-top">
            <span className="stat-label">{t('在线机器')}</span>
            <Icon name="hosts" size={15} className="stat-icon" />
          </div>
          <div className="stat-val num">
            {stats.hostsOnline}
            <small>/ {stats.hostsTotal}</small>
          </div>
          <div className="stat-meter">
            <span style={{ width: `${hostPct}%` }} data-tone={hostPct === 100 ? 'ok' : 'warn'} />
          </div>
          <span className="stat-note">
            {stats.hostsOnline === stats.hostsTotal ? t('全部已上报') : t('有机器未上报')}
          </span>
        </button>

        <button type="button" className="stat-card panel" onClick={() => navigate('/hosts')}>
          <div className="stat-top">
            <span className="stat-label">{t('可用节点')}</span>
            <Icon name="layers" size={15} className="stat-icon" />
          </div>
          <div className="stat-val num">
            {stats.nodesAlive}
            <small>/ {stats.nodesTotal}</small>
          </div>
          <div className="stat-meter">
            <span style={{ width: `${alivePct}%` }} data-tone={alivePct >= 80 ? 'ok' : alivePct >= 50 ? 'warn' : 'bad'} />
          </div>
          <span className="stat-note">
            {unverifiedTotal > 0
              ? t('存活率 {{pct}}% · {{unverified}} 未验证', { pct: alivePct, unverified: unverifiedTotal })
              : t('存活率 {{pct}}%', { pct: alivePct })}
          </span>
          {hasProbedNodes && (
            <span className="stat-note stat-note-sub">{t('探测来自服务器视角，不代表你的网络')}</span>
          )}
        </button>

        <button type="button" className="stat-card panel" onClick={() => navigate('/subscriptions')}>
          <div className="stat-top">
            <span className="stat-label">{t('订阅拉取')}</span>
            <Icon name="link" size={15} className="stat-icon" />
          </div>
          <div className="stat-val num">{stats.subHits}</div>
          <span className="stat-note">
            {t('{{links}} 条链接 · {{devices}} 台设备', { links: subscriptions.length, devices: deviceCount })}
          </span>
        </button>

        <div className="stat-card panel">
          <div className="stat-top">
            <span className="stat-label">{t('最近上报')}</span>
            <Icon name="clock" size={15} className="stat-icon" />
          </div>
          <div className="stat-val stat-val-sm num">{ago(lastReport)}</div>
          <span className="stat-note">{t('agent 变动即报，每小时心跳')}</span>
        </div>
      </section>

      <TrendPanel series={series} />

      <div className="grid two">
        <section className="panel panel-flush">
          <div className="panel-head">
            <h2>{t('机器')}</h2>
            <button className="text small" onClick={() => navigate('/hosts')}>
              {t('全部')} →
            </button>
          </div>
          {sortedHosts.length === 0 ? (
            <div className="panel-pad">
              <Empty
                icon="hosts"
                title={t('还没有机器')}
                body={t('接入第一台运行 mihomo 的主机后，这里会显示它的状态。')}
              />
            </div>
          ) : (
            <div className="table-wrap">
              <table className="compact-table">
                <tbody>
                  {sortedHosts.slice(0, 5).map((h) => (
                    <tr
                      key={h.id}
                      className="click"
                      tabIndex={0}
                      onClick={() => navigate(`/hosts?id=${encodeURIComponent(h.id)}`)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          navigate(`/hosts?id=${encodeURIComponent(h.id)}`)
                        }
                      }}
                    >
                      <td>
                        <span className="cell-main">{h.name}</span>
                        <span className="cell-sub">{overviewHostSub(h, t)}</span>
                      </td>
                      <td>
                        <NodeThreads nodes={nodesByHost.get(h.id) ?? []} />
                      </td>
                      <td className="num">
                        <StatusPill status={h.status} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="panel panel-flush">
          <div className="panel-head">
            <h2>{t('订阅')}</h2>
            <button className="text small" onClick={() => navigate('/subscriptions')}>
              {t('管理')} →
            </button>
          </div>
          {subscriptions.length === 0 ? (
            <div className="panel-pad">
              <Empty
                icon="link"
                title={t('没有订阅链接')}
                body={t('从一条编排生成链接，客户端就能拉取聚合后的配置。')}
              />
            </div>
          ) : (
            <div className="table-wrap">
              <table className="mini-subs compact-table">
                <tbody>
                  {subscriptions.slice(0, 5).map((s) => {
                    const top = (s.devices ?? [])[0]
                    return (
                      <tr
                        key={s.id}
                        className="click"
                        tabIndex={0}
                        onClick={() => navigate('/subscriptions')}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault()
                            navigate('/subscriptions')
                          }
                        }}
                      >
                        <td>
                          <span className={s.enabled ? 'cell-main' : 'cell-main muted'}>
                            {s.label || t('未命名')}
                          </span>
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
            </div>
          )}
        </section>
      </div>

      <DelayPanel nodes={state.nodes} />

      <details className="disclosure panel" open={hosts.length === 0}>
        <summary>
          <span className="disclosure-title">
            <Icon name="hosts" size={15} />
            <span>{t('接入下一台机器')}</span>
          </span>
          <span className="disclosure-meta small muted">{t('快速接入命令')}</span>
          <Icon name="chevronDown" size={15} className="disclosure-chevron" />
        </summary>
        <div className="disclosure-body">
          <pre>
            <code>{agentCmd}</code>
          </pre>
          <div className="actions">
            <Copy value={agentCmd} label={t('复制模板')} />
            <button className="text small" onClick={() => navigate('/hosts')}>
              {t('生成 token')} →
            </button>
          </div>
        </div>
      </details>
    </>
  )
}

function overviewHostSub(h: Host, t: (key: string, opts?: Record<string, unknown>) => string): string {
  if (h.kind === 'sub' || h.kind === 'static') {
    if (!h.lastProbedAt) {
      return t('{{total}} 节点 · 尚未探测', { total: h.nodeCount })
    }
    const counts = t('{{ok}} 可用 · {{unverified}} 未验证 · {{unreachable}} 不可达', {
      ok: h.probeOkCount ?? 0,
      unverified: h.probeUnverifiedCount ?? 0,
      unreachable: h.probeUnreachableCount ?? 0,
    })
    return `${counts} · ${t('最近探测 {{ago}}', { ago: ago(h.lastProbedAt) })}`
  }
  return t('{{alive}}/{{total}} 节点 · {{ago}}', {
    alive: h.aliveCount,
    total: h.nodeCount,
    ago: ago(h.lastSeen),
  })
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
  const series = metric === 'delay' ? all.filter((p) => p.delay > 0) : all
  const last = series[series.length - 1]
  const first = series[0]
  const value = (p: SeriesPoint) => (metric === 'delay' ? p.delay : p.alive)
  const unit = metric === 'delay' ? 'ms' : t('个')
  const change = last && first ? value(last) - value(first) : 0
  const good = metric === 'delay' ? change < 0 : change > 0

  return (
    <section className="panel">
      <div className="section-head">
        <div>
          <h2>{metric === 'delay' ? t('平均延迟') : t('可用节点')}</h2>
          {last && (
            <p className="trend-value">
              <strong className="num">{value(last)}</strong>
              <span className="unit">{unit}</span>
              {change !== 0 && (
                <span className={`delta-pill num ${good ? 'is-good' : 'is-bad'}`}>
                  {change > 0 ? '+' : '−'}
                  {Math.abs(change)} {unit}
                </span>
              )}
              <span className="window">{t('较 6 小时前')}</span>
            </p>
          )}
        </div>
        <div className="tabs" role="group" aria-label={t('指标')}>
          <button aria-pressed={metric === 'delay'} onClick={() => setMetric('delay')}>
            {t('延迟')}
          </button>
          <button aria-pressed={metric === 'alive'} onClick={() => setMetric('alive')}>
            {t('可用节点')}
          </button>
        </div>
      </div>
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
  const gradId = useId()
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
  const H = W < 600 ? 184 : 236
  const left = 36
  const right = 10
  const top = 12
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
  for (let tick = min; tick <= max + 1e-9; tick += step) ticks.push(Math.round(tick * 10) / 10)

  const x = (i: number) => left + (i / (series.length - 1)) * (W - left - right)
  const y = (v: number) => top + (1 - (v - min) / (max - min)) * (H - top - bottom)
  const line = series.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(value(p)).toFixed(1)}`).join(' ')
  const area = `${line} L${x(series.length - 1).toFixed(1)},${y(min).toFixed(1)} L${x(0).toFixed(1)},${y(min).toFixed(1)} Z`

  const hours: number[] = []
  const t0 = series[0].at
  const t1 = series[series.length - 1].at
  const every = W < 600 ? 7200 : 3600
  for (let ts = Math.ceil(t0 / every) * every; ts <= t1; ts += every) hours.push(ts)
  const xt = (ts: number) => left + ((ts - t0) / (t1 - t0)) * (W - left - right)
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
        <defs>
          <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--links)" stopOpacity="0.18" />
            <stop offset="100%" stopColor="var(--links)" stopOpacity="0.01" />
          </linearGradient>
        </defs>
        {ticks.map((tick, i) => (
          <g key={tick}>
            <line
              x1={left}
              x2={W - right}
              y1={y(tick)}
              y2={y(tick)}
              className={i === 0 ? 'chart__base' : 'chart__grid'}
            />
            <text x={left - 8} y={y(tick) + 3.5} className="chart__tick" textAnchor="end">
              {tick}
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
        <path d={area} fill={`url(#${gradId})`} />
        <path d={line} className="chart__line" />
        {hours.map((ts, i) => (
          <text
            key={ts}
            x={xt(ts)}
            y={H - 6}
            className="chart__tick"
            textAnchor={i === 0 && xt(ts) - left < 20 ? 'start' : 'middle'}
          >
            {clock(ts)}
          </text>
        ))}
        {h && hover !== null ? (
          <g>
            <line x1={x(hover)} x2={x(hover)} y1={top} y2={H - bottom} className="chart__cursor" />
            <circle cx={x(hover)} cy={y(value(h))} r="7" className="chart__halo" />
            <circle cx={x(hover)} cy={y(value(h))} r="3.8" className="chart__dot" />
          </g>
        ) : (
          <g>
            <circle cx={x(series.length - 1)} cy={y(values[values.length - 1])} r="6.5" className="chart__halo" />
            <circle cx={x(series.length - 1)} cy={y(values[values.length - 1])} r="3.5" className="chart__dot" />
          </g>
        )}
      </svg>
      {h && hover !== null && (
        <div
          className="chart-tip"
          style={{
            left: `${(x(hover) / W) * 100}%`,
            transform: `translateX(${hover > series.length * 0.65 ? '-106%' : hover < series.length * 0.15 ? '6%' : '-50%'})`,
          }}
        >
          <span className="chart-tip-time num">{clock(h.at)}</span>
          <b className="num">
            {value(h)} {unit}
          </b>
          {metric === 'delay' && h.nodes > 0 && (
            <span className="chart-tip-extra num">
              {h.alive}/{h.nodes}
            </span>
          )}
        </div>
      )}
    </figure>
  )
}

const BIN = 50

function DelayPanel({ nodes }: { nodes: ProxyNode[] }) {
  const { t } = useTranslation()
  const alive = nodes
    .filter((n) => isNodeAvailable(n) && n.delayMs > 0)
    .map((n) => n.delayMs)
    .sort((a, b) => a - b)
  const down = nodes.filter((n) => nodeHealth(n) === 'unreachable').length
  if (nodes.length === 0) {
    return (
      <section className="panel">
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
    <section className="panel">
      <div className="section-head">
        <div>
          <h2>{t('延迟分布')}</h2>
          <span className="small muted">{t('{{n}} 个可用节点 · 每档 50 ms', { n: alive.length })}</span>
        </div>
        <div className="dist-legend small muted" aria-label={t('延迟分布参考')}>
          <span>
            <i className="swatch swatch-fast" />
            {t('快（<100 ms）')}
          </span>
          <span>
            <i className="swatch" />
            {t('常规（≥100 ms）')}
          </span>
        </div>
      </div>
      <div className="dist-layout">
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
            <dd className="num is-good">
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
        <div>
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
                  data-empty={b.count === 0}
                  style={{ height: b.count ? `${Math.max(6, (b.count / peak) * 100)}%` : '2px' }}
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
        </div>
      </div>
    </section>
  )
}
