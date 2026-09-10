// Map tab: the seed-rendered world map with live players from the agent and
// the objects the server knows about (portals, ships, carts, tombstones, beds,
// boss locations), each as a toggleable layer.
import { useMemo, useState } from 'react'
import { Alert, Badge, Button, Chip, Group, Loader, Progress, Skeleton, Stack, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconAlertTriangle, IconMapOff, IconRefresh } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import type { ExploredInfo } from '../../api/types'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo } from '../../lib/format'
import { SectionCard, StatusPill } from '../../ui'
import { useAgent } from '../agent'
import type { MapIconName } from './MapIcons'
import { MapView, type Marker, type TileSource } from './MapView'
import { mapImageUrl, tileUrl, useInstanceMap, useRenderMap, worldToFraction } from './useMap'

type Layer = 'players' | 'portals' | 'ships' | 'carts' | 'tombstones' | 'beds' | 'locations' | 'pins'
const LAYERS: { id: Layer; label: string }[] = [
  { id: 'players', label: 'Players' },
  { id: 'pins', label: 'Pins' },
  { id: 'portals', label: 'Portals' },
  { id: 'locations', label: 'Bosses & places' },
  { id: 'ships', label: 'Ships' },
  { id: 'carts', label: 'Carts' },
  { id: 'tombstones', label: 'Tombstones' },
  { id: 'beds', label: 'Beds' },
]
const DEFAULT_LAYERS: Layer[] = ['players', 'pins', 'portals', 'locations', 'ships', 'tombstones']

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

function newerExplored(a: ExploredInfo | undefined, b: ExploredInfo | undefined): ExploredInfo | undefined {
  if (!a) return b
  if (!b) return a
  return b.version > a.version ? b : a
}

const ICON_NAMES: ReadonlySet<string> = new Set<MapIconName>(['player', 'portal', 'ship', 'cart', 'tombstone', 'bed', 'temple', 'boss', 'trader', 'fire', 'house', 'mine', 'cave', 'death', 'hildir', 'other'])

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

export function MapTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const map = useInstanceMap(id)
  const agent = useAgent(id)
  const render = useRenderMap(id)
  const [layers, setLayers] = useState<string[]>(DEFAULT_LAYERS)
  const [fog, setFog] = useState(true)
  const [imageBroken, setImageBroken] = useState(false)

  const data = map.data
  const info = data?.info
  const radius = info?.world_radius ?? 10500
  const rendering = info?.state === 'rendering' || info?.state === 'encoding'
  const showImage = !!data?.image_ready && !imageBroken
  // Live positions come from the agent stream; the map query is the fallback.
  const livePlayers = agent.data?.status?.players
  const players = useMemo(() => livePlayers ?? data?.players ?? [], [livePlayers, data?.players])
  const hidden = players.filter((p) => !p.position).length
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
        ? { tileSize: tilesInfo.tile_size, maxZoom: tilesInfo.max_zoom, version: tilesInfo.version, url: (z, x, y) => tileUrl(id, z, x, y, fogOn) }
        : null,
    [tilesInfo, data?.image_ready, id, fogOn],
  )

  const markers = useMemo<Marker[]>(() => {
    const out: Marker[] = []
    const on = new Set(layers)
    if (on.has('players')) {
      for (const p of players) {
        if (!p.position) continue
        const { u, v } = worldToFraction(p.position.x, p.position.z, radius)
        out.push({ key: `p-${p.uid}`, u, v, kind: 'player', icon: 'player', label: p.name, detail: p.visible ? undefined : 'hidden from other players', caption: p.name, labelStyle: 'name' })
      }
    }
    const typeToLayer: Record<string, Layer> = { portal: 'portals', ship: 'ships', cart: 'carts', tombstone: 'tombstones', bed: 'beds' }
    ;(data?.objects ?? []).forEach((o, i) => {
      const layer = typeToLayer[o.type]
      if (!layer || !on.has(layer)) return
      if (fogOn && o.explored === false) return
      const { u, v } = worldToFraction(o.x, o.z, radius)
      out.push({ key: `o-${o.type}-${i}`, u, v, kind: o.type, icon: iconFor(o.type), label: o.label, detail: o.text || undefined })
    })
    const locations = data?.locations ?? []
    if (on.has('locations')) {
      locations.forEach((l, i) => {
        // A location a player pinned from a Vegvisir is known even under the fog.
        if (fogOn && l.explored === false && !l.discovered) return
        const { u, v } = worldToFraction(l.x, l.z, radius)
        const label = LOCATION_LABELS[l.name] ?? l.name
        out.push({ key: `l-${i}`, u, v, kind: 'location', icon: locationIcon(l.name), label, caption: label, labelStyle: 'location', detail: l.discovered ? 'discovered by a player' : undefined })
      })
    }
    if (on.has('pins')) {
      ;(data?.pins ?? []).forEach((p, i) => {
        // Pins are knowledge players wrote to a table, so they are never hidden by the fog.
        if (p.type === 'boss' && on.has('locations') && locations.some((l) => Math.hypot(l.x - p.x, l.z - p.z) <= BOSS_PIN_MERGE_M)) return
        const { u, v } = worldToFraction(p.x, p.z, radius)
        const parts = [p.source === 'vegvisir' ? 'found at a Vegvisir' : '', p.author ? `by ${p.author}` : '', p.checked ? 'checked off' : ''].filter(Boolean)
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
          detail: parts.length ? parts.join(' · ') : undefined,
        })
      })
    }
    return out
  }, [layers, players, data?.objects, data?.pins, data?.locations, radius, fogOn])

  function confirmRerender() {
    modals.openConfirmModal({
      title: 'Re-render the map',
      children: (
        <Text size="sm">
          The server samples the whole world again in the background (a minute or two at the default resolution).
          Players are not affected.
        </Text>
      ),
      labels: { confirm: 'Re-render', cancel: 'Cancel' },
      onConfirm: () => render.mutate({ force: true }),
    })
  }

  if (map.isLoading) {
    return <Skeleton height={480} />
  }
  if (!data) return <Text c="dimmed">Map unavailable.</Text>

  let overlay: React.ReactNode = null
  if (!showImage) {
    overlay = (
      <Stack align="center" gap="xs" p="lg" style={{ background: 'rgba(10,12,18,0.7)', borderRadius: 12, maxWidth: 360 }}>
        {rendering ? (
          <>
            <Loader size="sm" />
            <Text size="sm" c="white">
              Rendering the world map on the server… {Math.round((info?.progress ?? 0) * 100)}%
            </Text>
            <Progress value={(info?.progress ?? 0) * 100} w="100%" size="sm" />
          </>
        ) : (
          <>
            <IconMapOff size={28} color="var(--vh-text-soft)" />
            <Text size="sm" c="white" ta="center">
              {!agent.data?.installed
                ? 'Install the Valheim UI Agent (Mods tab) to get a map of this world.'
                : !data.connected
                  ? 'No map yet. Start the server with the agent; it renders the map a few seconds after the world loads.'
                  : !data.map_supported
                    ? `The agent running in this server (v${data.agent_version || '?'}) predates the map. Update it on the Mods tab; the server restarts and the map renders a minute or two later.`
                    : info?.state === 'failed'
                      ? `The render failed: ${info.error || 'unknown error'}`
                      : 'Waiting for the server to start the render…'}
            </Text>
            {data.connected && !data.map_supported && (
              <Button size="xs" variant="light" component={Link} to={`/instances/${id}/mods`}>
                Open the Mods tab
              </Button>
            )}
          </>
        )}
      </Stack>
    )
  }

  return (
    <Stack gap="md">
      <SectionCard
        title="World map"
        description="The world drawn like the in-game map from the server's own sampling, players live. Scroll to zoom, drag to pan."
        actions={
          <Group gap="sm" wrap="wrap" justify="flex-end">
            <StatusPill color={data.connected ? 'moss' : 'gray'}>{data.connected ? 'live' : 'agent offline'}</StatusPill>
            {data.stale && (
              <Badge color="orange" variant="light" leftSection={<IconAlertTriangle size={12} />}>
                cached image
              </Badge>
            )}
            {info && info.state === 'ready' && (
              <Badge variant="outline" color="gray">
                {info.size} px · seed {info.seed}
              </Badge>
            )}
            {hasRole('operator') && data.connected && data.map_supported && (
              <Button size="xs" variant="light" leftSection={<IconRefresh size={14} />} loading={render.isPending} disabled={rendering} onClick={confirmRerender}>
                Re-render
              </Button>
            )}
          </Group>
        }
      >
        <Stack gap="sm">
          <Group gap={6} justify="space-between" wrap="wrap">
            <Chip.Group multiple value={layers} onChange={setLayers}>
              <Group gap={6}>
                {LAYERS.map((l) => (
                  <Chip key={l.id} value={l.id} size="xs" variant="light">
                    {l.label}
                  </Chip>
                ))}
              </Group>
            </Chip.Group>
            <Tooltip
              label={
                !fogSupported
                  ? 'The running agent has no exploration tracking; update it from the Mods tab'
                  : canLiftFog
                    ? 'Only terrain players have explored (or shared on a cartography table) is shown. Operators can lift the fog.'
                    : 'Only terrain players have explored (or shared on a cartography table) is shown'
              }
            >
              <div>
                <Chip checked={fogOn} onChange={setFog} size="xs" variant="filled" color="iron" disabled={!fogSupported || !canLiftFog}>
                  Fog of war{explored ? ` · ${explored.percent.toFixed(1)}% explored` : ''}
                </Chip>
              </div>
            </Tooltip>
          </Group>
          <MapView
            imageUrl={data.image_ready ? mapImageUrl(id, data.image_version, fogOn) : null}
            tiles={tiles}
            markers={markers}
            overlay={overlay}
            onImageError={() => setImageBroken(true)}
            onImageLoad={() => setImageBroken(false)}
          />
          <Group justify="space-between" gap="xs" wrap="wrap">
            <Text size="xs" c="dimmed">
              {players.length} player{players.length === 1 ? '' : 's'} online
              {hidden > 0 ? `, ${hidden} hiding their position` : ''} · {data.objects.length} objects
              {data.objects_updated_at ? ` (scanned ${fmtAgo(data.objects_updated_at)})` : ''}
            </Text>
            <Text size="xs" c="dimmed">
              Beyond the 10 km circle lies the edge of the world.
            </Text>
          </Group>
          {data.stale && !data.connected && (
            <Alert color="orange" variant="light" icon={<IconAlertTriangle size={16} />}>
              This image was rendered during an earlier run. Start the server to confirm it still matches the world.
            </Alert>
          )}
          {data.connected && data.map_supported && !data.layers_supported && (
            <Alert color="yellow" variant="light" icon={<IconAlertTriangle size={16} />}>
              The agent in this server (v{data.agent_version || '?'}) draws the map in the older flat style. Update it from the{' '}
              <Link to={`/instances/${id}/mods`}>Mods tab</Link> (the server restarts) for the in-game look and deep zoom.
            </Alert>
          )}
        </Stack>
      </SectionCard>
    </Stack>
  )
}
