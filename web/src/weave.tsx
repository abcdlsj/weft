import { useTranslation } from 'react-i18next'
import { nodeHealth, type ProxyNode } from './api'

/** A bundle's weave: one segment per host, width proportional to its node count.
 *  It is the product's own shape — several threads, one cloth. */
export function WeaveStrip({
  segments,
  width = 124,
}: {
  segments: { label: string; count: number }[]
  width?: number
}) {
  if (segments.length === 0) return <span className="muted">—</span>
  const total = segments.reduce((a, s) => a + s.count, 0)
  const title = segments.map((s) => `${s.label} · ${s.count}`).join('\n')
  return (
    <div className="weave" style={{ width }} role="img" aria-label={title} title={title}>
      {segments.map((s) => (
        <span
          key={s.label}
          style={{ flexGrow: Math.max(s.count, 0.6) / total }}
          title={`${s.label} · ${s.count}`}
        />
      ))}
    </div>
  )
}

/** A host's threads: one cell per node, colored by availability / probe state. */
export function NodeThreads({ nodes }: { nodes: ProxyNode[] }) {
  const { t } = useTranslation()
  if (nodes.length === 0) return <span className="muted">—</span>
  const ok = nodes.filter((n) => nodeHealth(n) === 'ok').length
  const unverified = nodes.filter((n) => nodeHealth(n) === 'unverified').length
  const visible = nodes.slice(0, 24)
  const overflow = nodes.length - visible.length
  const label = unverified
    ? t('{{ok}} 可用 · {{unverified}} 未验证 / {{total}} 总计', { ok, unverified, total: nodes.length })
    : t('{{alive}} / {{total}} 个节点可用', { alive: ok, total: nodes.length })
  return (
    <span className="threads" role="img" aria-label={label}>
      {visible.map((n) => {
        const health = nodeHealth(n)
        const statusText =
          health === 'ok'
            ? n.delayMs
              ? `${n.delayMs} ms`
              : t('可用')
            : health === 'unverified'
              ? t('未验证')
              : health === 'pending'
                ? t('未探测')
                : t('不可达')
        return (
          <span
            key={n.key}
            data-down={health === 'unreachable'}
            data-unverified={health === 'unverified'}
            data-pending={health === 'pending'}
            data-fast={health === 'ok' && n.delayMs > 0 && n.delayMs < 100}
            title={`${n.name} · ${statusText}`}
          />
        )
      })}
      {overflow > 0 && <em className="threads-more num">+{overflow}</em>}
    </span>
  )
}
