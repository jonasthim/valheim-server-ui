import { useState } from 'react'
import {
  ActionIcon,
  Alert,
  Anchor,
  Button,
  CopyButton,
  Grid,
  Group,
  Paper,
  Pill,
  Skeleton,
  SimpleGrid,
  Stack,
  Switch,
  Table,
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
  IconPlugConnected,
  IconRefresh,
} from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { API_BASE } from '../../api/client'
import { fmtAgo, fmtBytes, fmtPercent, fmtTime } from '../../lib/format'
import { useJobDrawer, useJobs, jobStatusColor, jobTypeLabel } from '../jobs'
import { useSystemInfo } from '../system'
import { LoadError, SectionCard, StatTile, StatusDot, StatusPill } from '../../ui'
import { useModsOverview } from '../mods/useMods'
import { useAgent, WorldCard } from '../agent'
import { CheckModUpdatesButton } from '../mods/CheckModUpdatesButton'
import { useInstance } from './useInstance'
import { useCheckForUpdate, useInstanceEvents, useInstanceStatus, useSetAutostart } from './instanceActions'
import { stateColor, stateLabel } from './instanceHelpers'
import { LifecycleControls } from './LifecycleControls'

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

  const jobsQuery = useJobs({ instance: id, limit: 5 })
  const eventsQuery = useInstanceEvents(id)
  const systemInfo = useSystemInfo()
  const agentQuery = useAgent(id)
  // Hides the hero map <img> on load failure so the parchment Paper shows
  // through instead of a broken-image glyph (see the Hero section below).
  const [mapImgOk, setMapImgOk] = useState(true)

  if (inst.isLoading) {
    return (
      <Stack>
        <SimpleGrid cols={{ base: 2, md: 3, xl: 6 }}>
          <Skeleton height={92} />
          <Skeleton height={92} />
          <Skeleton height={92} />
          <Skeleton height={92} />
        </SimpleGrid>
        <Skeleton height={220} />
        <Skeleton height={220} />
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
  const worldSummary = world
    ? `world ${world.name ?? instance.config.world}, day ${world.day ?? '?'}, ${
        world.weather ? world.weather.toLowerCase() : 'unknown weather'
      }, ${world.is_night ? 'night' : 'day'}`
    : `world ${instance.config.world}`

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

  return (
    <Stack>
      <Stack gap={2}>
        <Group gap="sm" align="center">
          <StatusDot color={stateColor(status.state)} pulse={status.state === 'running' || status.state === 'starting'} />
          <Title order={2}>
            {stateLabel(status.state)}
            {status.state === 'running' ? ` · ${status.players_online} online` : ''}
          </Title>
        </Group>
        <Text size="sm" c="dimmed">
          {worldSummary}
        </Text>
      </Stack>

      <Grid gap="md">
        <Grid.Col span={{ base: 12, md: 7 }}>
          <Paper p="sm" style={{ background: 'var(--vh-parchment)' }}>
            <div style={{ width: 'min(100%, 60vh)', aspectRatio: '1', margin: '0 auto' }}>
              {mapImgOk && (
                <img
                  src={`${API_BASE}/instances/${id}/map.png`}
                  alt="World map"
                  style={{ width: '100%', aspectRatio: '1', objectFit: 'cover', display: 'block' }}
                  onError={() => setMapImgOk(false)}
                />
              )}
            </div>
            <Anchor component={Link} to={`/instances/${id}/map`} size="sm" mt="xs" ta="center" style={{ display: 'block' }}>
              Open the map
            </Anchor>
          </Paper>
        </Grid.Col>
        <Grid.Col span={{ base: 12, md: 5 }}>
          <WorldCard id={id} variant="parchment" />
        </Grid.Col>
      </Grid>

      <SimpleGrid cols={{ base: 2, md: 4 }}>
        <StatTile
          compact
          label="CPU"
          value={fmtPercent(status.cpu_percent)}
          hint={status.state === 'running' ? 'of one core' : 'not running'}
          icon={<IconCpu size={16} />}
          accent={status.cpu_percent !== undefined && status.cpu_percent >= 90 ? 'var(--vh-blood)' : undefined}
        />
        <StatTile
          compact
          label="Memory"
          value={status.memory_bytes !== undefined ? fmtBytes(status.memory_bytes) : '—'}
          hint={status.state === 'running' ? 'resident' : 'not running'}
          icon={<IconDeviceSdCard size={16} />}
        />
        <StatTile
          compact
          label="Build"
          value={status.installed_buildid ?? 'unknown'}
          hint={gameUpdate ? 'update available' : 'up to date'}
          icon={<IconBox size={16} />}
          accent={gameUpdate ? 'var(--vh-ember)' : undefined}
        />
        <StatTile
          compact
          label="Updates"
          value={updatesValue}
          hint={anyUpdate ? 'available' : 'all current'}
          icon={<IconDownload size={16} />}
          accent={anyUpdate ? 'var(--vh-ember)' : undefined}
        />
      </SimpleGrid>

      <SectionCard title="Updates" description="Game server files and installed mods.">
        <Stack gap="md">
          <Group justify="space-between" wrap="wrap" gap="sm" align="flex-start">
            <div style={{ minWidth: 0 }}>
              <Group gap="xs">
                <Text size="sm" fw={600}>
                  Game server
                </Text>
                <StatusPill color={gameUpdate ? 'orange' : 'moss'}>
                  {gameUpdate ? 'update available' : 'up to date'}
                </StatusPill>
              </Group>
              <Text size="xs" c="dimmed">
                Installed build {status.installed_buildid ?? 'unknown'} · latest {latestBuildId ?? 'unknown'}
                {latestBuildCheckedAt ? ` (checked ${fmtAgo(latestBuildCheckedAt)})` : ''}
              </Text>
              {gameUpdate && (
                <Text size="xs" c="dimmed">
                  {instance.config.backup_before_update
                    ? 'A backup will be taken automatically before updating.'
                    : 'Enable "Backup before update" in Config to snapshot the world first.'}
                </Text>
              )}
            </div>
            {canOperate && (
              <Group gap="xs">
                <Button
                  size="xs"
                  variant="default"
                  leftSection={<IconRefresh size={14} />}
                  loading={checkUpdate.isPending}
                  onClick={() => checkUpdate.mutate()}
                >
                  Check for updates
                </Button>
              </Group>
            )}
          </Group>

          <div style={{ borderTop: '1px solid var(--vh-border)' }} />

          <Group justify="space-between" wrap="wrap" gap="sm" align="flex-start">
            <div style={{ minWidth: 0 }}>
              <Group gap="xs">
                <Text size="sm" fw={600}>
                  Mods
                </Text>
                {bepinex?.installed ? (
                  <StatusPill color={modsPending > 0 ? 'orange' : 'moss'}>
                    {modsPending > 0 ? `${modsPending} update${modsPending === 1 ? '' : 's'}` : 'up to date'}
                  </StatusPill>
                ) : (
                  <StatusPill color="gray">BepInEx not installed</StatusPill>
                )}
              </Group>
              <Text size="xs" c="dimmed">
                {bepinex?.installed
                  ? `BepInEx ${bepinex.version ?? ''}${bepinexUpdate ? ` → ${bepinex.latest_version}` : ''} · ${modUpdateCount} mod${modUpdateCount === 1 ? '' : 's'} with an update`
                  : 'Install BepInEx from the Mods tab to run mods.'}
              </Text>
            </div>
            <Group gap="xs">
              {canOperate && bepinex?.installed && <CheckModUpdatesButton id={id} />}
              <Button size="xs" variant="light" onClick={() => navigate(`/instances/${id}/mods`)}>
                Open Mods
              </Button>
            </Group>
          </Group>
        </Stack>
      </SectionCard>

      <SimpleGrid cols={{ base: 1, md: 2 }} spacing="md">
        <SectionCard title="Controls" description="Manage the running process.">
          <Stack gap="sm">
            <Group gap="xs">
              <StatusDot color={status.ready ? 'moss' : 'gray'} />
              <Text size="sm" c="dimmed">
                Ready
              </Text>
              <Text size="sm" fw={600}>
                {status.ready ? 'Yes' : 'No'}
              </Text>
            </Group>
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
            <Switch
              label="Autostart"
              description="Start automatically when the manager starts"
              checked={status.autostart}
              disabled={!canOperate || setAutostart.isPending}
              onChange={(e) => setAutostart.mutate(e.currentTarget.checked)}
            />
            {status.pending_restart && (
              <Alert color="yellow" icon={<IconAlertTriangle size={16} />}>
                Pending restart to apply the latest configuration.
              </Alert>
            )}
            {status.state === 'failed' && status.detail && (
              <Alert color="red" icon={<IconAlertTriangle size={16} />} title="Failed">
                {status.detail}
              </Alert>
            )}
            {status.crash_count_24h > 0 && (
              <Alert color="red" icon={<IconAlertTriangle size={16} />} title="Crashes detected">
                Crashed {status.crash_count_24h}× in the last 24 h — last exit: {status.last_exit_detail || 'unknown'},{' '}
                {fmtAgo(status.last_crash_at)}
              </Alert>
            )}
            <LifecycleControls id={id} name={instance.name} status={status} />
          </Stack>
        </SectionCard>

        <SectionCard title="Connect" description="Share these details with players.">
          <Stack gap="sm">
            <Group gap="xs">
              <Pill size="lg" ff="monospace">
                {host}:{instance.config.port}
              </Pill>
              <CopyButton value={`${host}:${instance.config.port}`}>
                {({ copied, copy }) => (
                  <Tooltip label={copied ? 'Copied' : 'Copy'}>
                    <ActionIcon variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} aria-label="Copy address">
                      {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                    </ActionIcon>
                  </Tooltip>
                )}
              </CopyButton>
            </Group>
            <Text size="xs" c="dimmed">
              The query port (server browser / A2S) is port+1 ({instance.config.port + 1}).
            </Text>
            {instance.config.crossplay && (
              <Group gap="xs" align="center">
                <IconPlugConnected size={16} style={{ opacity: 0.7 }} />
                <Text size="sm" c="dimmed">
                  Join code
                </Text>
                <Text size="sm" ff="monospace" fw={600}>
                  {status.join_code ?? '—'}
                </Text>
              </Group>
            )}
          </Stack>
        </SectionCard>
      </SimpleGrid>

      <SectionCard title="Recent jobs" flush>
        {jobsQuery.isLoading && (
          <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
            <Skeleton height={80} />
          </div>
        )}
        {jobsQuery.isError && (
          <LoadError error={jobsQuery.error} title="Could not load recent jobs" onRetry={() => jobsQuery.refetch()} />
        )}
        {!jobsQuery.isLoading && !jobsQuery.isError && jobs.length === 0 && (
          <Text c="dimmed" size="sm" p="lg">
            No jobs yet for this instance.
          </Text>
        )}
        {jobs.length > 0 && (
          <Table.ScrollContainer minWidth={480}>
            <Table verticalSpacing="xs" highlightOnHover>
              <Table.Tbody>
                {jobs.map((job) => (
                  <Table.Tr key={job.id} style={{ cursor: 'pointer' }} onClick={() => openJob(job.id)}>
                    <Table.Td>
                      <StatusPill color={jobStatusColor(job.status)}>{job.status}</StatusPill>
                    </Table.Td>
                    <Table.Td>{job.title || jobTypeLabel(job.type)}</Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {fmtAgo(job.created_at)}
                      </Text>
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </SectionCard>

      <SectionCard title="Recent events" flush>
        {eventsQuery.isLoading && (
          <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
            <Skeleton height={80} />
          </div>
        )}
        {!eventsQuery.isLoading && events.length === 0 && (
          <Text c="dimmed" size="sm" p="lg">
            No events yet.
          </Text>
        )}
        {events.length > 0 && (
          <Table.ScrollContainer minWidth={480}>
            <Table verticalSpacing="xs" highlightOnHover>
              <Table.Tbody>
                {events.map((ev) => (
                  <Table.Tr key={ev.id}>
                    <Table.Td>
                      <StatusPill color={eventKindColor(ev.kind)}>{ev.kind}</StatusPill>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" c={ev.detail ? undefined : 'dimmed'}>
                        {ev.detail || '—'}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {fmtAgo(ev.at)}
                      </Text>
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </SectionCard>
    </Stack>
  )
}

// eventKindColor maps an InstanceEvent.kind to the StatusPill color used for
// its badge in "Recent events".
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
