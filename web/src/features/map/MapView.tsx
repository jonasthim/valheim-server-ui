// The pannable, zoomable map surface: the rendered terrain image with
// markers positioned in image fractions so they stay put at any zoom, and
// counter-scaled so they keep their screen size.
import { useCallback, useEffect, useRef, useState, type CSSProperties, type PointerEvent, type ReactNode, type WheelEvent } from 'react'
import { ActionIcon, Group, Tooltip } from '@mantine/core'
import { IconFocusCentered, IconMinus, IconPlus } from '@tabler/icons-react'
import classes from './map.module.css'

export type Marker = {
  key: string
  u: number
  v: number
  kind: 'player' | 'portal' | 'ship' | 'cart' | 'tombstone' | 'bed' | 'location'
  label: string
  detail?: string
  /** Rendered next to the marker (player names). */
  caption?: string
}

type Transform = { k: number; tx: number; ty: number }

const MIN_ZOOM = 1
const MAX_ZOOM = 24

export function MapView({
  imageUrl,
  fogUrl,
  markers,
  overlay,
  onImageError,
  onImageLoad,
}: {
  imageUrl: string | null
  /** Fog mask (opaque where unexplored); null disables the fog layer. */
  fogUrl?: string | null
  markers: Marker[]
  /** Rendered over the map (progress, empty states). */
  overlay?: ReactNode
  onImageError?: () => void
  onImageLoad?: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [t, setT] = useState<Transform>({ k: 1, tx: 0, ty: 0 })
  const drag = useRef<{ x: number; y: number; tx: number; ty: number } | null>(null)

  // Keep the image inside the viewport: at zoom 1 it fills the square.
  const clamp = useCallback((n: Transform): Transform => {
    const el = ref.current
    if (!el) return n
    const size = el.clientWidth
    const k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, n.k))
    const min = size - size * k
    return { k, tx: Math.min(0, Math.max(min, n.tx)), ty: Math.min(0, Math.max(min, n.ty)) }
  }, [])

  const zoomAt = useCallback(
    (factor: number, cx: number, cy: number) => {
      setT((prev) => {
        const k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, prev.k * factor))
        const ratio = k / prev.k
        return clamp({ k, tx: cx - (cx - prev.tx) * ratio, ty: cy - (cy - prev.ty) * ratio })
      })
    },
    [clamp],
  )

  function onWheel(e: WheelEvent<HTMLDivElement>) {
    e.preventDefault()
    const rect = e.currentTarget.getBoundingClientRect()
    zoomAt(e.deltaY < 0 ? 1.2 : 1 / 1.2, e.clientX - rect.left, e.clientY - rect.top)
  }

  function onPointerDown(e: PointerEvent<HTMLDivElement>) {
    if (e.button !== 0) return
    e.currentTarget.setPointerCapture(e.pointerId)
    drag.current = { x: e.clientX, y: e.clientY, tx: t.tx, ty: t.ty }
  }
  function onPointerMove(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current
    if (!d) return
    setT((prev) => clamp({ k: prev.k, tx: d.tx + (e.clientX - d.x), ty: d.ty + (e.clientY - d.y) }))
  }
  function onPointerUp(e: PointerEvent<HTMLDivElement>) {
    drag.current = null
    e.currentTarget.releasePointerCapture(e.pointerId)
  }

  useEffect(() => {
    const onResize = () => setT((prev) => clamp(prev))
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [clamp])

  const center = () => {
    const el = ref.current
    if (!el) return { x: 0, y: 0 }
    return { x: el.clientWidth / 2, y: el.clientHeight / 2 }
  }

  const surfaceStyle: CSSProperties = {
    transform: `translate(${t.tx}px, ${t.ty}px) scale(${t.k})`,
  }
  const markerScale: CSSProperties = { transform: `translate(-50%, -50%) scale(${1 / t.k})` }

  return (
    <div className={classes.viewport} ref={ref} onWheel={onWheel} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}>
      <div className={classes.surface} style={surfaceStyle}>
        {imageUrl && (
          <img
            className={classes.image}
            src={imageUrl}
            alt="World map"
            draggable={false}
            onError={onImageError}
            onLoad={onImageLoad}
          />
        )}
        {imageUrl && fogUrl && (
          <div
            className={classes.fog}
            style={{ maskImage: `url("${fogUrl}")`, WebkitMaskImage: `url("${fogUrl}")` }}
            aria-hidden
          />
        )}
        {markers.map((m) => (
          <div key={m.key} className={classes.markerAnchor} style={{ left: `${m.u * 100}%`, top: `${m.v * 100}%` }}>
            <Tooltip label={m.detail ? `${m.label} · ${m.detail}` : m.label} withArrow openDelay={150}>
              <div className={`${classes.marker} ${classes[m.kind]}`} style={markerScale} data-kind={m.kind}>
                {m.caption && <span className={classes.caption}>{m.caption}</span>}
              </div>
            </Tooltip>
          </div>
        ))}
      </div>
      {overlay && <div className={classes.overlay}>{overlay}</div>}
      <Group gap={4} className={classes.controls}>
        <ActionIcon variant="default" aria-label="Zoom in" onClick={() => zoomAt(1.5, center().x, center().y)}>
          <IconPlus size={16} />
        </ActionIcon>
        <ActionIcon variant="default" aria-label="Zoom out" onClick={() => zoomAt(1 / 1.5, center().x, center().y)}>
          <IconMinus size={16} />
        </ActionIcon>
        <ActionIcon variant="default" aria-label="Reset view" onClick={() => setT({ k: 1, tx: 0, ty: 0 })}>
          <IconFocusCentered size={16} />
        </ActionIcon>
      </Group>
    </div>
  )
}
