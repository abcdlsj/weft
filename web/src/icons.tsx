import type { ReactElement, SVGProps } from 'react'

/* One grid: 20×20, 1.5px stroke, round caps and joins, 2px of air inside the box.
   Geometry is drawn by hand so every icon shares weight, corner and rhythm. */

const shapes: Record<string, ReactElement> = {
  overview: (
    <>
      <rect x="3" y="3" width="6" height="6" rx="1.6" />
      <rect x="11" y="3" width="6" height="6" rx="1.6" />
      <rect x="3" y="11" width="6" height="6" rx="1.6" />
      <rect x="11" y="11" width="6" height="6" rx="1.6" />
    </>
  ),
  hosts: (
    <>
      <rect x="2.75" y="3.5" width="14.5" height="5.5" rx="1.8" />
      <rect x="2.75" y="11" width="14.5" height="5.5" rx="1.8" />
      <circle cx="5.9" cy="6.25" r="0.85" fill="currentColor" stroke="none" />
      <circle cx="5.9" cy="13.75" r="0.85" fill="currentColor" stroke="none" />
      <path d="M13.4 6.25h1.6M13.4 13.75h1.6" />
    </>
  ),
  rules: (
    <>
      <path d="M4.2 5.2h11.6M4.2 10h11.6M4.2 14.8h7.6" />
      <circle cx="14.7" cy="14.8" r="1.7" />
    </>
  ),
  layers: (
    <>
      <path d="M10 2.8 17.2 6.6 10 10.4 2.8 6.6z" />
      <path d="M3.4 10.3 10 13.8l6.6-3.5" />
      <path d="M3.4 13.9 10 17.4l6.6-3.5" />
    </>
  ),
  link: (
    <>
      <path d="M8.6 11.4a3.1 3.1 0 0 0 4.4 0l2-2a3.1 3.1 0 0 0-4.4-4.4l-1.1 1.1" />
      <path d="M11.4 8.6a3.1 3.1 0 0 0-4.4 0l-2 2a3.1 3.1 0 0 0 4.4 4.4l1.1-1.1" />
    </>
  ),
  gear: (
    <>
      <path d="M3 5.75h4.2M11.2 5.75H17" />
      <path d="M3 10h7.2M14.2 10H17" />
      <path d="M3 14.25h2.4M9.4 14.25H17" />
      <circle cx="9.2" cy="5.75" r="1.9" />
      <circle cx="12.2" cy="10" r="1.9" />
      <circle cx="7.4" cy="14.25" r="1.9" />
    </>
  ),
  plus: <path d="M10 4.5v11M4.5 10h11" />,
  check: <path d="M4 10.4 8 14.4l8-8.9" />,
  copy: (
    <>
      <rect x="7.25" y="7.25" width="10" height="10" rx="2.2" />
      <path d="M12.75 4.9v-.4A2.25 2.25 0 0 0 10.5 2.25h-6A2.25 2.25 0 0 0 2.25 4.5v6a2.25 2.25 0 0 0 2.25 2.25h.4" />
    </>
  ),
  refresh: (
    <>
      <path d="M16.6 10a6.6 6.6 0 1 1-1.95-4.67" />
      <path d="M16.9 3.6v3.9h-3.9" />
    </>
  ),
  close: <path d="M5.2 5.2l9.6 9.6M14.8 5.2l-9.6 9.6" />,
  chevron: <path d="M8.2 5.2 13 10l-4.8 4.8" />,
  trash: (
    <>
      <path d="M3.6 5.9h12.8" />
      <path d="M8.1 5.9V4.4a1.2 1.2 0 0 1 1.2-1.2h1.4a1.2 1.2 0 0 1 1.2 1.2v1.5" />
      <path d="M5.4 5.9l.7 9.9a1.3 1.3 0 0 0 1.3 1.2h5.2a1.3 1.3 0 0 0 1.3-1.2l.7-9.9" />
      <path d="M8.6 9v4.6M11.4 9v4.6" />
    </>
  ),
  key: (
    <>
      <circle cx="7.4" cy="7.4" r="3.5" />
      <path d="M9.9 9.9 16.4 16.4M14.2 16.4h2.2v-2.2" />
    </>
  ),
  pencil: (
    <>
      <path d="M4.2 15.8 3.6 17l1.2-.6 9.1-9.1-1.2-1.2z" />
      <path d="m12.1 4.3 1.2-1.2a1.4 1.4 0 0 1 2 0l1.6 1.6a1.4 1.4 0 0 1 0 2l-1.2 1.2" />
    </>
  ),
  share: (
    <>
      <circle cx="6" cy="10" r="2.1" />
      <circle cx="14.2" cy="5.4" r="2.1" />
      <circle cx="14.2" cy="14.6" r="2.1" />
      <path d="m7.9 9 4.4-2.6M7.9 11l4.4 2.6" />
    </>
  ),
  search: (
    <>
      <circle cx="8.9" cy="8.9" r="5.4" />
      <path d="M12.9 12.9 16.6 16.6" />
    </>
  ),
  qr: (
    <>
      <rect x="2.9" y="2.9" width="5.6" height="5.6" rx="1.2" />
      <rect x="11.5" y="2.9" width="5.6" height="5.6" rx="1.2" />
      <rect x="2.9" y="11.5" width="5.6" height="5.6" rx="1.2" />
      <path d="M11.5 11.5h2.2v2.2h-2.2zM14.9 14.9h2.2v2.2h-2.2z" />
    </>
  ),
  power: (
    <>
      <path d="M10 2.9v6.4" />
      <path d="M14.9 5.4a6.6 6.6 0 1 1-9.8 0" />
    </>
  ),
  device: (
    <>
      <rect x="4.2" y="2.9" width="11.6" height="14.2" rx="2.4" />
      <path d="M8.6 14.6h2.8" />
    </>
  ),
  clock: (
    <>
      <circle cx="10" cy="10" r="7" />
      <path d="M10 6.2V10l2.8 1.9" />
    </>
  ),
  arrowUpRight: <path d="M6.4 13.6 13.6 6.4M7.6 6.4h6v6" />,
}

export type IconName = keyof typeof shapes

export function Icon({
  name,
  ...rest
}: { name: IconName | string } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
    >
      {shapes[name] ?? shapes.overview}
    </svg>
  )
}

/** The mark: three warp threads crossed by two weft threads. */
export function Mark({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden="true" className="mark">
      <path d="M5.6 3.2v13.6M10 3.2v13.6M14.4 3.2v13.6" stroke="currentColor" strokeWidth="1.2" opacity="0.4" />
      <path d="M3 7.4h14M3 12.6h14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

export { shapes as iconShapes }
