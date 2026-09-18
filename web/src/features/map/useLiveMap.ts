// The live map composition MapTab and the Overview hero both draw from: the
// instance map query, live player positions from the agent stream, fog of
// war, and the tile/overlay/marker data to draw. `fog` and `layers` mirror
// the Map tab's own controls; a caller with no controls of its own (the
// Overview hero) just passes fixed values.
import { useMemo } from 'react'
import type { ExploredInfo } from '../../api/types'
import { useAuth } from '../../auth/useAuth'
import { useAgent } from '../agent'
import type { MapIconName } from './MapIcons'
import type { Marker, Overlays, Ping, TileSource } from './MapView'
import {
  cloudsUrl,
  fogMaskUrl,
  mapImageUrl,
  tileUrl,
  useInstanceMap,
  waterMaskUrl,
  worldToFraction,
} from './useMap'

export type Layer =
  | 'players'
  | 'portals'
  | 'ships'
  | 'carts'
  | 'tombstones'
  | 'beds'
  | 'locations'
  | 'pins'

export const DEFAULT_LAYERS: Layer[] = [
  'players',
  'pins',
  'portals',
  'locations',
  'ships',
  'tombstones',
]

const PIN_LABELS: Record<string, string> = {
  fire: 'Fire',
  house: 'House',
  mine: 'Mine',
  cave: 'Cave',
  death: 'Death',
  bed: 'Bed',
  portal: 'Portal',
  boss: 'Boss',
  hildir: 'Hildir',
  other: 'Pin',
}

function newerExplored(
  a: ExploredInfo | undefined,
  b: ExploredInfo | undefined,
): ExploredInfo | undefined {
  if (!a) return b
  if (!b) return a
  return b.version > a.version ? b : a
}

const ICON_NAMES: ReadonlySet<string> = new Set<MapIconName>([
  'player',
  'portal',
  'ship',
  'cart',
  'tombstone',
  'bed',
  'temple',
  'boss',
  'trader',
  'fire',
  'house',
  'mine',
  'cave',
  'death',
  'hildir',
  'other',
])

/** Maps an object type or pin type to a glyph. */
function iconFor(kind: string): MapIconName {
  return ICON_NAMES.has(kind) ? (kind as MapIconName) : 'other'
}

/** The glyph for one of the game's location icons. */
function locationIcon(name: string): MapIconName {
  if (name === 'StartTemple') return 'temple'
  if (/Vendor|Hildir_camp|BogWitch/.test(name)) return 'trader'
  return 'boss'
}

/** A boss pin this close to a boss location is the location itself (a Vegvisir marks the altar). */
const BOSS_PIN_MERGE_M = 80

const LOCATION_LABELS: Record<string, string> = {
  StartTemple: 'Sacrificial Stones',
  Eikthyrnir: 'Eikthyr',
  GDKing: 'The Elder',
  Bonemass: 'Bonemass',
  Dragonqueen: 'Moder',
  GoblinKing: 'Yagluth',
  Mistlands_DvergrBossEntrance1: 'The Queen',
  FaderLocation: 'Fader',
  Vendor_BlackForest: 'Haldor',
  Hildir_camp: 'Hildir',
  BogWitch_Camp: 'Bog Witch',
}

/**
 * The live map composition for one instance: the map query, live players,
 * fog of war, and the tile/overlay/marker data to draw. `fog` and `layers`
 * drive the same filtering the Map tab's controls do; `animate` defaults on
 * (callers with no toggle of their own, like the Overview hero, get the
 * drifting fog/water and pings without needing to say so).
 */
export function useLiveMap(
  id: string,
  opts: { fog: boolean; layers: string[]; animate?: boolean },
) {
  const { fog, layers } = opts
  const animate = opts.animate ?? true
  const { hasRole } = useAuth()
  const map = useInstanceMap(id)
  const agent = useAgent(id)

  const data = map.data
  const info = data?.info
  const radius = info?.world_radius ?? 10500
  // Live positions come from the agent stream; the map query is the fallback.
  const livePlayers = agent.data?.status?.players
  const players = useMemo(
    () => livePlayers ?? data?.players ?? [],
    [livePlayers, data?.players],
  )
  const fogSupported = !!data?.fog_supported
  // The fog is composited on the server; only operators may lift it.
  const canLiftFog = hasRole('operator')
  const fogOn = (fog || !canLiftFog) && fogSupported
  // Two sources carry the fog state: the agent stream (every change, ~2 s)
  // and the map poll (5 s). Show whichever reflects the newer exploration.
  const explored = newerExplored(agent.data?.explored, data?.explored)
  // The deep-zoom pyramid, when the agent exports layers.
  const tilesInfo = data?.tiles
  const tiles = useMemo<TileSource | null>(
    () =>
      tilesInfo && data?.image_ready
        ? {
            tileSize: tilesInfo.tile_size,
            maxZoom: tilesInfo.max_zoom,
            version: tilesInfo.version,
            url: (z, x, y) => tileUrl(id, z, x, y, fogOn),
          }
        : null,
    [tilesInfo, data?.image_ready, id, fogOn],
  )

  // Drifting clouds over the fog and a shimmer over explored water; both are
  // masked by images the server already fogs, so nothing hidden appears.
  const overlays = useMemo<Overlays | null>(() => {
    if (!animate || !data?.image_ready) return null
    return {
      clouds: cloudsUrl(id),
      fogMask: fogOn ? fogMaskUrl(id, explored?.mask_version) : null,
      water: tilesInfo ? waterMaskUrl(id, tilesInfo.version) : null,
    }
  }, [
    animate,
    data?.image_ready,
    id,
    fogOn,
    explored?.mask_version,
    tilesInfo,
  ])

  const pings = useMemo<Ping[]>(() => {
    if (!animate) return []
    return (agent.data?.status?.pings ?? []).map((p) => {
      const { u, v } = worldToFraction(p.position.x, p.position.z, radius)
      return { key: `${p.name}-${p.at}`, u, v, name: p.name }
    })
  }, [animate, agent.data?.status?.pings, radius])

  const markers = useMemo<Marker[]>(() => {
    const out: Marker[] = []
    const on = new Set(layers)
    if (on.has('players')) {
      for (const p of players) {
        if (!p.position) continue
        const { u, v } = worldToFraction(p.position.x, p.position.z, radius)
        out.push({
          key: `p-${p.uid}`,
          u,
          v,
          kind: 'player',
          icon: 'player',
          label: p.name,
          detail: p.visible ? undefined : 'hidden from other players',
          caption: p.name,
          labelStyle: 'name',
        })
      }
    }
    const typeToLayer: Record<string, Layer> = {
      portal: 'portals',
      ship: 'ships',
      cart: 'carts',
      tombstone: 'tombstones',
      bed: 'beds',
    }
    ;(data?.objects ?? []).forEach((o, i) => {
      const layer = typeToLayer[o.type]
      if (!layer || !on.has(layer)) return
      if (fogOn && o.explored === false) return
      const { u, v } = worldToFraction(o.x, o.z, radius)
      out.push({
        key: `o-${o.type}-${i}`,
        u,
        v,
        kind: o.type,
        icon: iconFor(o.type),
        label: o.label,
        detail: o.text || undefined,
      })
    })
    const locations = data?.locations ?? []
    if (on.has('locations')) {
      locations.forEach((l, i) => {
        // A location a player pinned from a Vegvisir is known even under the fog.
        if (fogOn && l.explored === false && !l.discovered) return
        const { u, v } = worldToFraction(l.x, l.z, radius)
        const label = LOCATION_LABELS[l.name] ?? l.name
        out.push({
          key: `l-${i}`,
          u,
          v,
          kind: 'location',
          icon: locationIcon(l.name),
          label,
          caption: label,
          labelStyle: 'location',
          detail: l.discovered ? 'discovered by a player' : undefined,
        })
      })
    }
    if (on.has('pins')) {
      (data?.pins ?? []).forEach((p, i) => {
        // Pins are knowledge players wrote to a table, so they are never hidden by the fog.
        if (
          p.type === 'boss' &&
          on.has('locations') &&
          locations.some(
            (l) => Math.hypot(l.x - p.x, l.z - p.z) <= BOSS_PIN_MERGE_M,
          )
        )
          return
        const { u, v } = worldToFraction(p.x, p.z, radius)
        const parts = [
          p.source === 'vegvisir' ? 'found at a Vegvisir' : '',
          p.author ? `by ${p.author}` : '',
          p.checked ? 'checked off' : '',
        ].filter(Boolean)
        const label = p.name || PIN_LABELS[p.type] || 'Pin'
        out.push({
          key: `pin-${i}`,
          u,
          v,
          kind: 'pin',
          icon: iconFor(p.type),
          pin: p.type,
          checked: p.checked,
          label,
          // Boss pins carry the game's uppercase caption; other pins keep their name in the tooltip.
          caption: p.type === 'boss' && p.name ? p.name : undefined,
          labelStyle: 'location',
          detail: parts.length ? parts.join(', ') : undefined,
        })
      })
    }
    return out
  }, [
    layers,
    players,
    data?.objects,
    data?.pins,
    data?.locations,
    radius,
    fogOn,
  ])

  const imageUrl = data?.image_ready
    ? mapImageUrl(id, data?.image_version, fogOn)
    : null

  return {
    map,
    data,
    agent,
    explored,
    tilesInfo,
    players,
    fogSupported,
    canLiftFog,
    fogOn,
    tiles,
    overlays,
    pings,
    markers,
    imageUrl,
  }
}
