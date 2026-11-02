/** A bundle's weave: one segment per host, width proportional to its node count.
 *  It is the product's own shape — several threads, one cloth. */
export function WeaveStrip({
  segments,
  width = 132,
}: {
  segments: { label: string; count: number }[]
  width?: number
}) {
  if (segments.length === 0) return <span className="muted">—</span>
  const total = segments.reduce((a, s) => a + s.count, 0)
  const title = segments.map((s) => `${s.label} · ${s.count}`).join('\n')
  return (
    <div className="weave-cell">
      <div className="weave" style={{ width }} role="img" aria-label={title} title={title}>
        {segments.map((s) => (
          <span key={s.label} style={{ flexGrow: Math.max(s.count, 0.6) / total }} />
        ))}
      </div>
    </div>
  )
}
