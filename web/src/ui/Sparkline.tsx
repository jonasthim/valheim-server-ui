import { Tooltip } from '@mantine/core'

// Internal viewBox width; horizontal scaling is stretch-to-fill
// (preserveAspectRatio="none"), so the actual unit doesn't matter.
const VIEWBOX_WIDTH = 100

/**
 * Zero-dependency inline-SVG sparkline: a thin trend line with a faint area
 * fill beneath it, scaled to the series' min/max (a flat series draws a
 * mid-line rather than dividing by zero). Wrapped in a Tooltip showing the
 * last value, but the tooltip is never the only carrier of that value
 * (WCAG 1.4.13 / 2.1.1): pass `label` (the trend's name, e.g. "CPU") and the
 * SVG gets an accessible name of `"<label>: <last value>"` instead of being
 * `aria-hidden`. Without `label` the SVG stays decorative/`aria-hidden`, so
 * callers must otherwise show the value themselves (StatTile and the
 * Overview history rows both pass `label`).
 */
export function Sparkline({
  values,
  width = 120,
  height = 28,
  color = 'var(--vh-text-soft)',
  format,
  label,
}: {
  values: number[]
  width?: number | string
  height?: number
  color?: string
  format?: (v: number) => string
  label?: string
}) {
  if (values.length < 2) return null

  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min
  const points = values.map((v, i) => {
    const x = (i / (values.length - 1)) * VIEWBOX_WIDTH
    const y = span === 0 ? height / 2 : height - ((v - min) / span) * height
    return { x, y }
  })

  const polyline = points.map((p) => `${p.x},${p.y}`).join(' ')
  const first = points[0]
  const last = points[points.length - 1]
  const area = [
    `M ${first.x},${height}`,
    ...points.map((p) => `L ${p.x},${p.y}`),
    `L ${last.x},${height}`,
    'Z',
  ].join(' ')

  const lastValue = values[values.length - 1]
  const formatted = format ? format(lastValue) : String(lastValue)

  const svg = (
    <svg
      viewBox={`0 0 ${VIEWBOX_WIDTH} ${height}`}
      preserveAspectRatio="none"
      style={{ width, height, display: 'block' }}
      aria-hidden={label ? undefined : true}
    >
      <path d={area} fill={color} opacity={0.12} stroke="none" />
      <polyline points={polyline} fill="none" stroke={color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
    </svg>
  )

  return (
    <Tooltip label={formatted}>
      {label ? (
        <span role="img" aria-label={`${label}: ${formatted}`} style={{ display: 'inline-block', width, height }}>
          {svg}
        </span>
      ) : (
        svg
      )}
    </Tooltip>
  )
}
