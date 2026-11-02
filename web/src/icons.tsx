import type { SVGProps } from 'react'

const paths: Record<string, string> = {
  overview: 'M3 3h6v6H3zM11 3h6v6h-6zM3 11h6v6H3zM11 11h6v6h-6z',
  hosts: 'M3 4h14v5H3zM3 11h14v5H3zM5.5 6.5h.01M5.5 13.5h.01',
  layers: 'M9 2.5 2.5 6 9 9.5 15.5 6zM2.5 10 9 13.5 15.5 10M2.5 13.5 9 17l6.5-3.5',
  link: 'M7.5 10.5a3 3 0 0 0 4.2 0l2.1-2.1a3 3 0 0 0-4.2-4.2l-1 1M10.5 7.5a3 3 0 0 0-4.2 0l-2.1 2.1a3 3 0 0 0 4.2 4.2l1-1',
  gear: 'M9 11.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM14.2 11a1.2 1.2 0 0 0 .2 1.3l.1.1a1.4 1.4 0 1 1-2 2l-.1-.1a1.2 1.2 0 0 0-2 .8v.3a1.4 1.4 0 1 1-2.8 0v-.2a1.2 1.2 0 0 0-2-.8l-.1.1a1.4 1.4 0 1 1-2-2l.1-.1a1.2 1.2 0 0 0-.8-2H3.4a1.4 1.4 0 1 1 0-2.8h.2a1.2 1.2 0 0 0 .8-2l-.1-.1a1.4 1.4 0 1 1 2-2l.1.1a1.2 1.2 0 0 0 2-.8V3a1.4 1.4 0 1 1 2.8 0v.2a1.2 1.2 0 0 0 2 .8l.1-.1a1.4 1.4 0 1 1 2 2l-.1.1a1.2 1.2 0 0 0 .8 2h.2a1.4 1.4 0 1 1 0 2.8h-.2a1.2 1.2 0 0 0-1.1.7z',
  plus: 'M9 3.5v11M3.5 9h11',
  check: 'M3.5 9.5 7 13l7.5-8',
  copy: 'M6.5 6.5V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-2.5M4.5 6.5h6a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-6a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1z',
  refresh: 'M15 9a6 6 0 1 1-1.8-4.3M15 3.5V6h-2.5',
  close: 'M4 4l10 10M14 4 4 14',
  chevron: 'M7 4l4 5-4 5',
  trash: 'M3.5 5.5h11M7 5.5V4a.5.5 0 0 1 .5-.5h3A.5.5 0 0 1 11 4v1.5M5 5.5l.6 9a1 1 0 0 0 1 .9h4.8a1 1 0 0 0 1-.9l.6-9',
  key: 'M10.5 7.5a3 3 0 1 0-3.9 2.9L3 14v1.5h1.5V14H6v-1.5h1.5L9 11a3 3 0 0 0 1.5-3.5z',
  search: 'M12.5 12.5 15 15M8 12.5a4.5 4.5 0 1 0 0-9 4.5 4.5 0 0 0 0 9z',
  qr: 'M3.5 3.5h4v4h-4zM10.5 3.5h4v4h-4zM3.5 10.5h4v4h-4zM10.5 10.5h1.5v1.5h-1.5zM13 13h1.5v1.5H13z',
  power: 'M9 2.5v6M13.5 5a6 6 0 1 1-9 0',
  arrowUpRight: 'M5.5 12.5 13 5M7 5h6v6',
}

export function Icon({ name, ...rest }: { name: keyof typeof paths | string } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 18 18"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
    >
      <path d={paths[name] ?? paths.overview} />
    </svg>
  )
}

/** The mark: three warp threads crossed by one weft thread. */
export function Mark({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" className="mark">
      <path d="M6 3.5v17M12 3.5v17M18 3.5v17" stroke="currentColor" strokeWidth="1.4" opacity="0.45" />
      <path d="M3 9h18M3 15h18" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  )
}
