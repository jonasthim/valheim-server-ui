import { useMemo, useState } from 'react'
import {
  ActionIcon,
  Alert,
  Anchor,
  Button,
  Code,
  CopyButton,
  Grid,
  Group,
  SegmentedControl,
  Skeleton,
  Stack,
  Switch,
  Text,
  Title,
  Tooltip,
} from '@mantine/core'
import { Link, useNavigate } from 'react-router-dom'
import {
  IconAlertTriangle,
  IconBox,
  IconCpu,
  IconDeviceSdCard,
  IconCheck,
  IconCopy,
  IconDownload,
  IconHistory,
  IconPlugConnected,
  IconRefresh,
} from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo, fmtBytes, fmtPercent, fmtTime } from '../../lib/format'
import { useJobDrawer, useJobs, jobStatusColor, jobTypeLabel } from '../jobs'
import { useSystemInfo } from '../system'
import { DataTable, EmptyState, LoadError, SectionCard, Sparkline, StatStrip, type StatStripItem, StatusDot, StatusPill } from '../../ui'
import { useModsOverview } from '../mods/useMods'
import { AgentSetupNotice, useAgent, WorldCard } from '../agent'
import { useAgentSetup } from '../agent/useAgentSetup'
import { DEFAULT_LAYERS, MapView, useLiveMap } from '../map'
import { frameFor, useExploredBounds } from '../map/useExploredBounds'
import { useInstance } from './useInstance'
import { useCheckForUpdate, useInstanceEvents, useInstanceStatus, useSetAutostart } from './instanceActions'
import { useInstanceMetrics, type MetricRange } from './useMetrics'
import { stateColor, stateLabel } from './instanceHelpers'
import { StatusRow } from './StatusRow'
import classes from './OverviewTab.module.css'

// One row in the "Recent activity" table, merging jobs and instance events
// into a single feed sorted by time (newest first, capped to 8).
type ActivityRow =
  | { kind: 'job'; id: string; label: string; status: string; color: string; at: string }
  | { kind: 'event'; id: number; label: string; status: string; color: string; at: string }

// Owned by WP-11. Props: the instance id.
export function OverviewTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const navigate = useNavigate()
  const inst = useInstance(id)
  const statusQuery = useInstanceStatus(id)
  const modsOverview = useModsOverview(id)
  const { openJob } = useJobDrawer()

  const checkUpdate = useCheckForUpdate(id)
  const setAutostart = useSetAutostart(id)

  const jobsQuery = useJobs({ instance: id, limit: 8 })
  const eventsQuery = useInstanceEvents(id)
  const systemInfo = useSystemInfo()
  const agentQuery = useAgent(id)
  const agentSetup = useAgentSetup(id)
  // Explored bounding box from the fog mask, so the hero map frames what
  // players have actually explored instead of the whole world.
  const exploredBounds = useExploredBounds(id, agentQuery.data?.explored?.mask_version)
  // The hero is the same live map as the Map tab, opened on the explored area.
  const live = useLiveMap(id, { fog: true, layers: DEFAULT_LAYERS })
  // Memoised so MapView's initial-frame effect only re-runs when the explored
  // area actually changes, not on every Overview render.
  const initialFrame = useMemo(() => (exploredBounds ? frameFor(exploredBounds) : undefined), [exploredBounds])

  // F-1.3: 24h sparklines on the compact CPU/Memory strip cells, plus a
  // ranged history section below (dedupes with the strip's query at 24h).
  const tileMetrics = useInstanceMetrics(id, '24h')
  const [historyRange, setHistoryRange] = useState<MetricRange>('24h')
  const historyMetrics = useInstanceMetrics(id, historyRange)

  if (inst.isLoading) {
    return (
      <Stack gap="md">
        <Skeleton height={56} />
        <Skeleton height={320} />
        <Skeleton height={160} />
      </Stack>
    )
  }
  if (!inst.data) return <Text c="dimmed">Instance not found.</Text>

  const instance = inst.data
  // The 10s poll and SSE both write ['instances', id, 'status']; fall back to
  // the instance detail's embedded status until the poll resolves once.
  const status = statusQuery.data?.status ?? instance.status
  const canOperate = hasRole('operator')
  const host = window.location.hostname

  // Status statement subtitle: the live world clock/weather through the
  // agent when connected, else just the configured world name.
  const world = agentQuery.data?.connected ? agentQuery.data.status?.world : undefined
  // While the server is still loading, the agent reports an empty name, day 0
  // and no weather: fall back to the configured name and say nothing more.
  const worldName = world?.name || instance.config.world
  const worldLoaded = !!world && (world.day ?? 0) > 0
  const worldSummary = worldLoaded
    ? `world ${worldName}, day ${world.day}${world.weather ? `, ${world.weather.toLowerCase()}` : ''}, ${
        world.is_night ? 'night' : 'day'
      }`
    : `world ${worldName}`

  const jobs = jobsQuery.data ?? []
  const events = eventsQuery.data?.events ?? []
  const latestBuildId = checkUpdate.data?.latest_buildid ?? systemInfo.data?.latest_buildid
  const latestBuildCheckedAt = checkUpdate.data?.checked_at ?? systemInfo.data?.buildid_checked_at

  // Updates: game server (SteamCMD buildid) + mods (Thunderstore/Hexium index).
  const bepinex = modsOverview.data?.bepinex
  const modUpdateCount = modsOverview.data?.mods.filter((m) => m.update_available).length ?? 0
  const bepinexUpdate = !!bepinex?.update_available
  const modsPending = modUpdateCount + (bepinexUpdate ? 1 : 0)
  const gameUpdate = !!status.update_available
  const anyUpdate = gameUpdate || modsPending > 0
  const updatesValue = anyUpdate
    ? [gameUpdate ? 'Game' : null, modsPending > 0 ? `${modsPending} mod${modsPending === 1 ? '' : 's'}` : null]
        .filter(Boolean)
        .join(' + ')
    : 'Up to date'

  // With an agent installed the Overview leads with the map and a margin
  // column (World card + strip); without one the strip takes a single
  // full-width row instead.
  const worldAvailable = agentSetup.stage === 'ready' || agentSetup.stage === 'offline' || agentSetup.stage === 'update'
  const miniItems: StatStripItem[] = [
    {
      label: 'CPU',
      value: fmtPercent(status.cpu_percent),
      hint: status.state === 'running' ? 'of one core' : 'not running',
      icon: <IconCpu size={14} />,
      tone: status.cpu_percent !== undefined && status.cpu_percent >= 90 ? 'danger' : 'default',
      spark: tileMetrics.data?.cpu,
      sparkFormat: fmtPercent,
    },
    {
      label: 'Memory',
      value: status.memory_bytes !== undefined ? fmtBytes(status.memory_bytes) : '—',
      hint: status.state === 'running' ? 'resident' : 'not running',
      icon: <IconDeviceSdCard size={14} />,
      spark: tileMetrics.data?.mem,
      sparkFormat: fmtBytes,
    },
    {
      label: 'Build',
      value: status.installed_buildid ?? 'unknown',
      hint: gameUpdate ? 'update available' : 'up to date',
      icon: <IconBox size={14} />,
      tone: gameUpdate ? 'accent' : 'default',
    },
    {
      label: 'Updates',
      value: updatesValue,
      hint: anyUpdate ? 'available' : 'all current',
      icon: <IconDownload size={14} />,
      tone: anyUpdate ? 'accent' : 'default',
    },
  ]

  const hasHistory = [historyMetrics.data?.cpu, historyMetrics.data?.mem, historyMetrics.data?.players].some(
    (s) => (s?.length ?? 0) >= 2,
  )

  // Recent activity: jobs and instance events merged into one feed, newest
  // first, capped to the last 8.
  const activity: ActivityRow[] = [
    ...jobs.map(
      (j): ActivityRow => ({
        kind: 'job',
        id: j.id,
        label: j.title || jobTypeLabel(j.type),
        status: j.status,
        color: jobStatusColor(j.status),
        at: j.created_at,
      }),
    ),
    ...events.map(
      (e): ActivityRow => ({
        kind: 'event',
        id: e.id,
        label: e.detail || e.kind,
        status: e.kind,
        color: eventKindColor(e.kind),
        at: e.at,
      }),
    ),
  ]
    .sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime())
    .slice(0, 8)

  return (
    <Stack gap="md">
      <Group gap="sm" align="baseline" wrap="wrap">
        <Group gap="sm" align="center" wrap="nowrap">
          <StatusDot color={stateColor(status.state)} pulse={status.state === 'running' || status.state === 'starting'} />
          <Title order={2} fz={18} fw={600} lh={1.3}>
            {stateLabel(status.state)}
            {status.state === 'running' ? ` · ${status.players_online} online` : ''}
          </Title>
        </Group>
        <Text size="sm" c="dimmed">
          {worldSummary}
        </Text>
      </Group>

      {/* Agent setup prompt (install / update / enable) as a full-width banner
          above the hero; it renders nothing once the agent is ready. */}
      <AgentSetupNotice id={id} context="overview" />

      {status.state === 'failed' && status.detail && (
        <Alert color="blood" icon={<IconAlertTriangle size={16} />} title="Failed">
          {status.detail}
        </Alert>
      )}
      {status.crash_count_24h > 0 && (
        <Alert color="blood" icon={<IconAlertTriangle size={16} />} title="Crashes detected">
          Crashed {status.crash_count_24h}× in the last 24 h — last exit: {status.last_exit_detail || 'unknown'},{' '}
          {fmtAgo(status.last_crash_at)}
        </Alert>
      )}

      {worldAvailable ? (
        <Grid gap="md">
          <Grid.Col span={{ base: 12, md: 8 }}>
            <div className={classes.mapHero}>
              <div className={classes.mapSquare}>
                <MapView
                  imageUrl={live.imageUrl}
                  tiles={live.tiles}
                  markers={live.markers}
                  overlays={live.overlays}
                  pings={live.pings}
                  initialFrame={initialFrame}
                />
              </div>
              <Anchor component={Link} to={`/instances/${id}/map`} size="sm" className={classes.mapLink}>
                Open the map
              </Anchor>
            </div>
          </Grid.Col>
          <Grid.Col span={{ base: 12, md: 4 }}>
            {/* The margin column: the World card on top, the compact strip
                pinned to the bottom, so the column fills the map's height. */}
            <Stack gap="md" h="100%" justify="space-between">
              <WorldCard id={id} playersLink />
              <StatStrip cols={2} minCellWidth={110} items={miniItems} />
            </Stack>
          </Grid.Col>
        </Grid>
      ) : (
        <StatStrip minCellWidth={150} items={miniItems} />
      )}

      <SectionCard title="Status" flush>
        <StatusRow label="Process">
          <StatusPill color={status.ready ? 'moss' : 'gray'}>{status.ready ? 'Ready' : 'Not ready'}</StatusPill>
          {status.pid !== undefined && (
            <Text size="sm" c="dimmed">
              PID {status.pid}
            </Text>
          )}
          {status.since && (
            <Text size="sm" c="dimmed">
              Since {fmtAgo(status.since)} ({fmtTime(status.since)})
            </Text>
          )}
        </StatusRow>
        <StatusRow label="Autostart" hint="Start automatically when the manager starts">
          <Switch
            aria-label="Autostart"
            checked={status.autostart}
            disabled={!canOperate || setAutostart.isPending}
            onChange={(e) => setAutostart.mutate(e.currentTarget.checked)}
          />
        </StatusRow>
        <StatusRow
          label="Game server"
          hint={
            <>
              Installed build {status.installed_buildid ?? 'unknown'}, latest {latestBuildId ?? 'unknown'}
              {latestBuildCheckedAt ? ` (checked ${fmtAgo(latestBuildCheckedAt)})` : ''}
              {gameUpdate && <br />}
              {gameUpdate &&
                (instance.config.backup_before_update
                  ? 'A backup will be taken automatically before updating.'
                  : 'Enable "Backup before update" in Config to snapshot the world first.')}
            </>
          }
        >
          <StatusPill color={gameUpdate ? 'orange' : 'moss'}>{gameUpdate ? 'update available' : 'up to date'}</StatusPill>
          {canOperate && (
            <Button
              size="xs"
              variant="default"
              leftSection={<IconRefresh size={14} />}
              loading={checkUpdate.isPending}
              onClick={() => checkUpdate.mutate()}
            >
              Check for updates
            </Button>
          )}
        </StatusRow>
        <StatusRow
          label="Mods"
          hint={
            bepinex?.installed
              ? `BepInEx ${bepinex.version ?? ''}${bepinexUpdate ? ` → ${bepinex.latest_version}` : ''}, ${modUpdateCount} mod${modUpdateCount === 1 ? '' : 's'} with an update`
              : 'Install BepInEx from the Mods tab to run mods.'
          }
        >
          {bepinex?.installed ? (
            <StatusPill color={modsPending > 0 ? 'orange' : 'moss'}>
              {modsPending > 0 ? `${modsPending} update${modsPending === 1 ? '' : 's'}` : 'up to date'}
            </StatusPill>
          ) : (
            <StatusPill color="gray">BepInEx not installed</StatusPill>
          )}
          <Button size="xs" variant="default" onClick={() => navigate(`/instances/${id}/mods`)}>
            Open Mods
          </Button>
        </StatusRow>
        <StatusRow label="Connect" hint={`The query port (server browser / A2S) is port+1 (${instance.config.port + 1}).`}>
          <Code fz="sm" className={classes.connectCode}>
            {host}:{instance.config.port}
          </Code>
          <CopyButton value={`${host}:${instance.config.port}`}>
            {({ copied, copy }) => (
              <Tooltip label={copied ? 'Copied' : 'Copy'}>
                <ActionIcon variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} aria-label="Copy address">
                  {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                </ActionIcon>
              </Tooltip>
            )}
          </CopyButton>
          {instance.config.crossplay && (
            <Group gap="xs">
              <IconPlugConnected size={16} style={{ opacity: 0.7 }} />
              <Text size="sm" c="dimmed">
                Join code
              </Text>
              <Text size="sm" ff="monospace" fw={600}>
                {status.join_code ?? '—'}
              </Text>
            </Group>
          )}
        </StatusRow>
      </SectionCard>

      <SectionCard
        title="History"
        actions={
          <SegmentedControl
            size="xs"
            value={historyRange}
            onChange={(v) => setHistoryRange(v as MetricRange)}
            data={[
              { label: '1h', value: '1h' },
              { label: '24h', value: '24h' },
              { label: '7d', value: '7d' },
              { label: '30d', value: '30d' },
            ]}
          />
        }
      >
        {historyMetrics.isLoading ? (
          <Skeleton height={56} />
        ) : hasHistory ? (
          <Stack gap="md">
            <HistoryRow label="CPU" values={historyMetrics.data?.cpu} format={fmtPercent} />
            <HistoryRow label="Memory" values={historyMetrics.data?.mem} format={fmtBytes} />
            <HistoryRow label="Players" values={historyMetrics.data?.players} format={(v) => `${Math.round(v)} online`} />
          </Stack>
        ) : (
          <Text size="sm" c="dimmed">
            No history yet
          </Text>
        )}
      </SectionCard>

      <SectionCard title="Recent activity" flush>
        <DataTable<ActivityRow>
          minWidth={480}
          columns={[
            {
              key: 'what',
              header: 'Activity',
              render: (r) => (
                <Text size="sm" truncate>
                  {r.label}
                </Text>
              ),
            },
            {
              key: 'status',
              header: 'Status',
              width: 140,
              render: (r) => <StatusPill color={r.color}>{r.status}</StatusPill>,
            },
            {
              key: 'when',
              header: 'When',
              width: 120,
              nowrap: true,
              render: (r) => (
                <Text size="xs" c="dimmed">
                  {fmtAgo(r.at)}
                </Text>
              ),
            },
          ]}
          rows={activity}
          rowKey={(r) => `${r.kind}-${r.id}`}
          loading={jobsQuery.isLoading || eventsQuery.isLoading}
          error={
            jobsQuery.isError && (
              <LoadError error={jobsQuery.error} title="Could not load recent jobs" onRetry={() => jobsQuery.refetch()} />
            )
          }
          empty={
            <EmptyState
              compact
              icon={<IconHistory size={22} />}
              title="No activity yet"
              description="Jobs and server events will show up here."
            />
          }
          clickable={(r) => r.kind === 'job'}
          onRowClick={(r) => r.kind === 'job' && openJob(r.id)}
        />
      </SectionCard>
    </Stack>
  )
}

// HistoryRow is one labelled trend line in the "History" section: a
// full-width sparkline, or a dimmed placeholder while there are fewer than 2
// samples in the selected range (F-1.3).
function HistoryRow({
  label,
  values,
  format,
}: {
  label: string
  values: number[] | undefined
  format: (v: number) => string
}) {
  return (
    <Group justify="space-between" align="center" wrap="nowrap" gap="md">
      <Text size="sm" c="dimmed" style={{ minWidth: 64, flexShrink: 0 }}>
        {label}
      </Text>
      {values && values.length >= 2 ? (
        <>
          <Text size="sm" fw={600} style={{ flexShrink: 0 }}>
            {format(values[values.length - 1])}
          </Text>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Sparkline values={values} format={format} width="100%" height={56} label={label} />
          </div>
        </>
      ) : (
        <Text size="xs" c="dimmed">
          No samples yet.
        </Text>
      )}
    </Group>
  )
}

// eventKindColor maps an InstanceEvent.kind to the StatusPill color used for
// its badge in "Recent activity".
function eventKindColor(kind: string): string {
  switch (kind) {
    case 'start':
      return 'moss'
    case 'ready':
      return 'blue'
    case 'stop':
      return 'gray'
    case 'crash':
      return 'red'
    case 'update':
      return 'orange'
    default:
      return 'gray'
  }
}
