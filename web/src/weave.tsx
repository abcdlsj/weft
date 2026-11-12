import { useTranslation } from 'react-i18next'

/** A bundle's weave: one segment per host, width proportional to its node count.
 *  It is the product's own shape — several threads, one cloth. */
export function WeaveStrip({
  segments,
  width = 110,
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
        <span key={s.label} style={{ flexGrow: Math.max(s.count, 0.6) / total }} />
      ))}
    </div>
  )
}

/** A host's threads: one cell per node, filled when the node answers, hollow when it does not. */
export function NodeThreads({ nodes }: { nodes: { key: string; name: string; alive: boolean; delayMs: number }[] }) {
  const { t } = useTranslation()
  if (nodes.length === 0) return <span className="muted">—</span>
  const alive = nodes.filter((n) => n.alive).length
  return (
    <span
      className="threads"
      role="img"
      aria-label={t('{{alive}} / {{total}} 个节点可用', { alive, total: nodes.length })}
    >
      {nodes.map((n) => (
        <span
          key={n.key}
          data-down={!n.alive}
          title={`${n.name} · ${n.alive ? `${n.delayMs} ms` : t('不可用')}`}
        />
      ))}
    </span>
  )
}
