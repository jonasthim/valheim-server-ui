// The pannable, zoomable map surface. The terrain is a tile pyramid rendered
// on the server (crisp at any zoom); markers are positioned in world
// fractions so they stay put, and counter-scaled so they keep their screen
// size. Without tiles (older agents) a single image is shown instead.
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent, type ReactNode, type WheelEvent } from 'react'
import { ActionIcon, Group, Tooltip } from '@mantine/core'
import { IconFocusCentered, IconMinus, IconPlus } from '@tabler/icons-react'
import { MapIcon, type MapIconName } from './MapIcons'
import classes from './map.module.css'

export type Marker = {
  key: string
  u: number
  v: number
  kind: 'player' | 'portal' | 'ship' | 'cart' | 'tombstone' | 'bed' | 'location' | 'pin'
  icon: MapIconName
  /** For kind 'pin': the game's pin type (fire, house, mine, cave, death, bed, portal, boss, hildir, other). */
  pin?: string
  /** Drawn crossed out (a pin the player checked off). */
  checked?: boolean
  label: string
  detail?: string
  /** Rendered with the marker: 'location' puts an uppercase caption below (the game's boss labels), 'name' a name beside it (players). */
  caption?: string
  labelStyle?: 'location' | 'name'
}

/** The tile pyramid to draw, from InstanceMap.tiles. */
export type TileSource = {
  tileSize: number
  maxZoom: number
  version: string
  url: (z: number, x: number, y: number) => string
}

/** Animated overlays drawn on top of the tiles (all masked so nothing hidden shows). */
export type Overlays = {
  /** Seamless cloud texture URL. */
  clouds: string
  /** Fog mask URL (alpha 255 = unexplored); clouds drift only there. */
  fogMask: string | null
  /** Water mask URL (luminance 255 = explored water); the shimmer stays there. */
  water: string | null
}

/** A ping to animate at a world fraction. */
export type Ping = { key: string; u: number; v: number; name: string }

type Transform = { k: number; tx: number; ty: number }

const MIN_ZOOM = 1
/** Without tiles a single image is shown; beyond this it is only pixels. */
const IMAGE_MAX_ZOOM = 24

const ICON_SIZE: Record<Marker['kind'], number> = {
  player: 22,
  portal: 18,
  ship: 20,
  cart: 18,
  tombstone: 18,
  bed: 16,
  location: 26,
  pin: 20,
}

export function MapView({
  imageUrl,
  tiles,
  markers,
  overlays,
  pings = [],
  overlay,
  onImageError,
  onImageLoad,
}: {
  /** Whole-map image, used when no tile pyramid is available. */
  imageUrl: string | null
  tiles?: TileSource | null
  markers: Marker[]
  /** Drifting clouds and water shimmer; omit to draw a still map. */
  overlays?: Overlays | null
  pings?: Ping[]
  /** Rendered over the map (progress, empty states). */
  overlay?: ReactNode
  onImageError?: () => void
  onImageLoad?: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [t, setT] = useState<Transform>({ k: 1, tx: 0, ty: 0 })
  const [size, setSize] = useState(0)
  const drag = useRef<{ x: number; y: number; tx: number; ty: number } | null>(null)

  // The deepest useful zoom: with tiles, the level where one tile pixel is
  // one screen pixel; with a single image, a fixed cap.
  const maxZoom = useCallback(() => {
    if (tiles && size > 0) return Math.max(MIN_ZOOM, (tiles.tileSize * 2 ** tiles.maxZoom) / size)
    return IMAGE_MAX_ZOOM
  }, [tiles, size])

  // Keep the map inside the viewport: at zoom 1 it fills the square.
  const clamp = useCallback(
    (n: Transform): Transform => {
      const el = ref.current
      if (!el) return n
      const s = el.clientWidth
      const k = Math.min(maxZoom(), Math.max(MIN_ZOOM, n.k))
      const min = s - s * k
      return { k, tx: Math.min(0, Math.max(min, n.tx)), ty: Math.min(0, Math.max(min, n.ty)) }
    },
    [maxZoom],
  )

  const zoomAt = useCallback(
    (factor: number, cx: number, cy: number) => {
      setT((prev) => {
        const k = Math.min(maxZoom(), Math.max(MIN_ZOOM, prev.k * factor))
        const ratio = k / prev.k
        return clamp({ k, tx: cx - (cx - prev.tx) * ratio, ty: cy - (cy - prev.ty) * ratio })
      })
    },
    [clamp, maxZoom],
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
    const el = ref.current
    if (!el) return
    const measure = () => {
      setSize(el.clientWidth)
      setT((prev) => clamp(prev))
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
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
        {tiles ? (
          <TileLayer tiles={tiles} transform={t} viewport={size} onFirstLoad={onImageLoad} />
        ) : (
          imageUrl && (
            <img className={classes.image} src={imageUrl} alt="World map" draggable={false} onError={onImageError} onLoad={onImageLoad} />
          )
        )}
        {overlays?.fogMask && (
          <>
            <div className={classes.clouds} style={{ backgroundImage: `url("${overlays.clouds}")`, maskImage: `url("${overlays.fogMask}")`, WebkitMaskImage: `url("${overlays.fogMask}")` }} aria-hidden />
            <div className={`${classes.clouds} ${classes.cloudsFar}`} style={{ backgroundImage: `url("${overlays.clouds}")`, maskImage: `url("${overlays.fogMask}")`, WebkitMaskImage: `url("${overlays.fogMask}")` }} aria-hidden />
          </>
        )}
        {overlays?.water && (
          <div className={classes.water} style={{ backgroundImage: `url("${overlays.clouds}")`, maskImage: `url("${overlays.water}")`, WebkitMaskImage: `url("${overlays.water}")` }} aria-hidden />
        )}
        {pings.map((p) => (
          <div key={p.key} className={classes.markerAnchor} style={{ left: `${p.u * 100}%`, top: `${p.v * 100}%` }}>
            <div className={classes.pingWrap} style={markerScale}>
              <span className={classes.ping} />
              <span className={`${classes.ping} ${classes.pingLate}`} />
              {p.name && <span className={classes.pingName}>{p.name}</span>}
            </div>
          </div>
        ))}
        {markers.map((m) => (
          <div key={m.key} className={`${classes.markerAnchor} ${m.kind === 'player' ? classes.glide : ''}`} style={{ left: `${m.u * 100}%`, top: `${m.v * 100}%` }}>
            <Tooltip label={m.detail ? `${m.label} · ${m.detail}` : m.label} withArrow openDelay={150}>
              <div className={classes.marker} style={markerScale} data-kind={m.kind} data-pin={m.pin}>
                <MapIcon name={m.icon} size={ICON_SIZE[m.kind]} checked={m.checked} />
                {m.caption && m.labelStyle === 'location' && <span className={classes.locationLabel}>{m.caption}</span>}
                {m.caption && m.labelStyle !== 'location' && <span className={classes.caption}>{m.caption}</span>}
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

type TileKey = string

/**
 * Draws the tiles that intersect the viewport at the zoom level whose tile
 * pixels match screen pixels. The previous level stays mounted underneath
 * until every visible tile of the new level has loaded, so zooming never
 * flashes to the parchment background.
 */
function TileLayer({ tiles, transform, viewport, onFirstLoad }: { tiles: TileSource; transform: Transform; viewport: number; onFirstLoad?: () => void }) {
  const [loaded, setLoaded] = useState<Set<TileKey>>(() => new Set())
  const announced = useRef(false)

  const z = useMemo(() => {
    if (viewport <= 0) return 0
    const ideal = Math.log2((viewport * transform.k) / tiles.tileSize)
    return Math.min(tiles.maxZoom, Math.max(0, Math.round(ideal)))
  }, [viewport, transform.k, tiles.tileSize, tiles.maxZoom])

  // Level bookkeeping derived during render (the "previous value" pattern):
  // remember the level we are leaving until the new one is complete, and
  // forget loaded tiles when the version changes (they reload).
  const [level, setLevel] = useState<{ z: number; prevZ: number | null; version: string }>({ z, prevZ: null, version: tiles.version })
  if (level.version !== tiles.version) {
    setLevel({ z, prevZ: null, version: tiles.version })
    setLoaded(new Set())
  } else if (level.z !== z) {
    setLevel({ z, prevZ: level.z, version: tiles.version })
  }
  const prevZ = level.version === tiles.version && level.z === z ? level.prevZ : null

  const visible = useCallback(
    (level: number): { x: number; y: number }[] => {
      if (viewport <= 0) return []
      const n = 2 ** level
      const extent = viewport * transform.k
      const u0 = -transform.tx / extent
      const u1 = (viewport - transform.tx) / extent
      const v0 = -transform.ty / extent
      const v1 = (viewport - transform.ty) / extent
      const x0 = Math.max(0, Math.floor(u0 * n) - 1)
      const x1 = Math.min(n - 1, Math.floor(u1 * n) + 1)
      const y0 = Math.max(0, Math.floor(v0 * n) - 1)
      const y1 = Math.min(n - 1, Math.floor(v1 * n) + 1)
      const out: { x: number; y: number }[] = []
      for (let y = y0; y <= y1; y++) for (let x = x0; x <= x1; x++) out.push({ x, y })
      return out
    },
    [viewport, transform.k, transform.tx, transform.ty],
  )

  const current = visible(z)
  const allLoaded = current.every((c) => loaded.has(`${tiles.version}/${z}/${c.x}/${c.y}`))
  if (allLoaded && prevZ !== null) {
    setLevel({ z, prevZ: null, version: tiles.version })
  }

  const renderLevel = (level: number, keyPrefix: string) => {
    const n = 2 ** level
    const pct = 100 / n
    return visible(level).map(({ x, y }) => {
      const key = `${tiles.version}/${level}/${x}/${y}`
      return (
        <img
          key={`${keyPrefix}${key}`}
          className={classes.tile}
          style={{ left: `${x * pct}%`, top: `${y * pct}%`, width: `${pct}%`, height: `${pct}%` }}
          src={`${tiles.url(level, x, y)}?v=${encodeURIComponent(tiles.version)}`}
          alt=""
          draggable={false}
          decoding="async"
          onLoad={() => {
            setLoaded((prev) => {
              if (prev.has(key)) return prev
              const next = new Set(prev)
              next.add(key)
              return next
            })
            if (!announced.current) {
              announced.current = true
              onFirstLoad?.()
            }
          }}
        />
      )
    })
  }

  return (
    <>
      {prevZ !== null && prevZ !== z && !allLoaded && renderLevel(prevZ, 'prev-')}
      {renderLevel(z, '')}
    </>
  )
}
