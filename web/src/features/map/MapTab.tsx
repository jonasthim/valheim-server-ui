// Map tab: the seed-rendered world map with live players from the agent and
// the objects the server knows about (portals, ships, carts, tombstones, beds,
// boss locations), each as a toggleable layer.
import { useMemo, useState } from 'react'
import { Alert, Badge, Button, Chip, Group, Loader, Progress, Skeleton, Stack, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconAlertTriangle, IconMapOff, IconRefresh } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo } from '../../lib/format'
import { SectionCard, StatusPill } from '../../ui'
import { useAgent } from '../agent'
import { ImportExploredButton } from './ImportExploredButton'
import { MapView, type Marker } from './MapView'
import { fogImageUrl, mapImageUrl, useInstanceMap, useRenderMap, worldToFraction } from './useMap'

type Layer = 'players' | 'portals' | 'ships' | 'carts' | 'tombstones' | 'beds' | 'locations'
const LAYERS: { id: Layer; label: string }[] = [
  { id: 'players', label: 'Players' },
  { id: 'portals', label: 'Portals' },
  { id: 'locations', label: 'Bosses & places' },
  { id: 'ships', label: 'Ships' },
  { id: 'carts', label: 'Carts' },
  { id: 'tombstones', label: 'Tombstones' },
  { id: 'beds', label: 'Beds' },
]
const DEFAULT_LAYERS: Layer[] = ['players', 'portals', 'locations', 'ships', 'tombstones']

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
  const fogOn = fog && fogSupported
  // The agent stream carries the fog version every 2 s; the map poll is the fallback.
  const explored = agent.data?.explored ?? data?.explored

  const markers = useMemo<Marker[]>(() => {
    const out: Marker[] = []
    const on = new Set(layers)
    if (on.has('players')) {
      for (const p of players) {
        if (!p.position) continue
        const { u, v } = worldToFraction(p.position.x, p.position.z, radius)
        out.push({ key: `p-${p.uid}`, u, v, kind: 'player', label: p.name, detail: p.visible ? undefined : 'hidden from other players', caption: p.name })
      }
    }
    const typeToLayer: Record<string, Layer> = { portal: 'portals', ship: 'ships', cart: 'carts', tombstone: 'tombstones', bed: 'beds' }
    ;(data?.objects ?? []).forEach((o, i) => {
      const layer = typeToLayer[o.type]
      if (!layer || !on.has(layer)) return
      if (fogOn && o.explored === false) return
      const { u, v } = worldToFraction(o.x, o.z, radius)
      out.push({ key: `o-${o.type}-${i}`, u, v, kind: o.type, label: o.label, detail: o.text || undefined })
    })
    if (on.has('locations')) {
      ;(data?.locations ?? []).forEach((l, i) => {
        if (fogOn && l.explored === false) return
        const { u, v } = worldToFraction(l.x, l.z, radius)
        out.push({ key: `l-${i}`, u, v, kind: 'location', label: LOCATION_LABELS[l.name] ?? l.name })
      })
    }
    return out
  }, [layers, players, data?.objects, data?.locations, radius, fogOn])

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
        description="Terrain from the world seed, players live from the server. Scroll to zoom, drag to pan."
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
            {hasRole('operator') && data.connected && fogSupported && (
              <ImportExploredButton id={id} />
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
                fogSupported
                  ? 'Only terrain players have explored (or shared on a cartography table) is shown'
                  : 'The running agent has no exploration tracking; update it from the Mods tab'
              }
            >
              <div>
                <Chip checked={fogOn} onChange={setFog} size="xs" variant="filled" color="iron" disabled={!fogSupported}>
                  Fog of war{explored ? ` · ${explored.percent.toFixed(1)}% explored` : ''}
                </Chip>
              </div>
            </Tooltip>
          </Group>
          <MapView
            imageUrl={data.image_ready ? mapImageUrl(id, info) : null}
            fogUrl={fogOn ? fogImageUrl(id, explored) : null}
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
        </Stack>
      </SectionCard>
    </Stack>
  )
}
