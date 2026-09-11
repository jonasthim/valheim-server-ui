import {
  ActionIcon,
  Alert,
  Button,
  CopyButton,
  Group,
  Pill,
  Skeleton,
  SimpleGrid,
  Stack,
  Switch,
  Table,
  Text,
  Tooltip,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { useNavigate } from 'react-router-dom'
import {
  IconAlertTriangle,
  IconBox,
  IconCpu,
  IconDeviceSdCard,
  IconCheck,
  IconCopy,
  IconDownload,
  IconKey,
  IconPlayerPlay,
  IconPlayerStop,
  IconPlugConnected,
  IconRefresh,
  IconServer2,
  IconUsers,
} from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo, fmtBytes, fmtPercent, fmtTime } from '../../lib/format'
import { useJobDrawer, useJobs, jobStatusColor, jobTypeLabel } from '../jobs'
import { useSystemInfo } from '../system'
import { SectionCard, StatTile, StatusDot, StatusPill } from '../../ui'
import { useModsOverview } from '../mods/useMods'
import { WorldCard } from '../agent'
import { CheckModUpdatesButton } from '../mods/CheckModUpdatesButton'
import { useInstance } from './useInstance'
import {
  useCheckForUpdate,
  useInstallInstance,
  useInstanceStatus,
  useRestartInstance,
  useSetAutostart,
  useStartInstance,
  useStopInstance,
  useUpdateInstance,
} from './instanceActions'
import { canInstall, canRestart, canStart, canStop, stateColor, stateLabel } from './instanceHelpers'

// Owned by WP-11. Props: the instance id.
export function OverviewTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const navigate = useNavigate()
  const inst = useInstance(id)
  const statusQuery = useInstanceStatus(id)
  const modsOverview = useModsOverview(id)
  const { openJob } = useJobDrawer()

  const start = useStartInstance(id)
  const stop = useStopInstance(id)
  const restart = useRestartInstance(id)
  const install = useInstallInstance(id)
  const checkUpdate = useCheckForUpdate(id)
  const updateNow = useUpdateInstance(id)
  const setAutostart = useSetAutostart(id)

  const jobsQuery = useJobs({ instance: id, limit: 5 })
  const systemInfo = useSystemInfo()

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

  function confirmRestart() {
    if (status.players_online > 0) {
      modals.openConfirmModal({
        title: 'Restart instance',
        children: (
          <Text size="sm">
            {status.players_online} player{status.players_online === 1 ? ' is' : 's are'} currently online. Restart
            anyway?
          </Text>
        ),
        labels: { confirm: 'Restart', cancel: 'Cancel' },
        confirmProps: { color: 'orange' },
        onConfirm: () => restart.mutate(),
      })
    } else {
      restart.mutate()
    }
  }

  function confirmUpdate() {
    const running = status.state === 'running'
    if (running) {
      modals.openConfirmModal({
        title: 'Update game files',
        children: (
          <Text size="sm">
            The instance is running and will be stopped, updated, and started again. Continue?
          </Text>
        ),
        labels: { confirm: 'Stop and update', cancel: 'Cancel' },
        confirmProps: { color: 'orange' },
        onConfirm: () => updateNow.mutate(true, { onSuccess: (res) => openJob(res.job.id) }),
      })
    } else {
      updateNow.mutate(false, { onSuccess: (res) => openJob(res.job.id) })
    }
  }

  const jobs = jobsQuery.data ?? []
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
      <SimpleGrid cols={{ base: 2, md: 3, xl: 7 }}>
        <StatTile
          label="State"
          value={stateLabel(status.state)}
          hint={status.state === 'running' && status.since ? `since ${fmtAgo(status.since)}` : undefined}
          icon={<IconServer2 size={16} />}
          accent={`var(--mantine-color-${stateColor(status.state)}-5)`}
        />
        <StatTile
          label="Players"
          value={`${status.players_online} / ${status.max_players ?? '?'}`}
          hint="online now"
          icon={<IconUsers size={16} />}
          accent={status.players_online > 0 ? 'var(--vh-moss)' : undefined}
        />
        {/* Join code is issued only by Valheim's crossplay networking. */}
        {instance.config.crossplay && (
          <StatTile
            label="Join code"
            value={status.join_code ? <Text ff="monospace" fw={650} size="lg">{status.join_code}</Text> : '—'}
            hint="share with friends"
            icon={
              status.join_code ? (
                <CopyButton value={status.join_code}>
                  {({ copied, copy }) => (
                    <Tooltip label={copied ? 'Copied' : 'Copy'}>
                      <ActionIcon size="sm" variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} aria-label="Copy join code">
                        {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                      </ActionIcon>
                    </Tooltip>
                  )}
                </CopyButton>
              ) : (
                <IconKey size={16} />
              )
            }
          />
        )}
        <StatTile
          label="CPU"
          value={fmtPercent(status.cpu_percent)}
          hint={status.state === 'running' ? 'of one core' : 'not running'}
          icon={<IconCpu size={16} />}
          accent={status.cpu_percent !== undefined && status.cpu_percent >= 90 ? 'var(--vh-blood)' : undefined}
        />
        <StatTile
          label="Memory"
          value={status.memory_bytes !== undefined ? fmtBytes(status.memory_bytes) : '—'}
          hint={status.state === 'running' ? 'resident' : 'not running'}
          icon={<IconDeviceSdCard size={16} />}
        />
        <StatTile
          label="Build"
          value={status.installed_buildid ?? 'unknown'}
          hint={gameUpdate ? 'update available' : 'up to date'}
          icon={<IconBox size={16} />}
          accent={gameUpdate ? 'var(--vh-ember)' : undefined}
        />
        <StatTile
          label="Updates"
          value={updatesValue}
          hint={anyUpdate ? 'available' : 'all current'}
          icon={<IconDownload size={16} />}
          accent={anyUpdate ? 'var(--vh-ember)' : undefined}
        />
      </SimpleGrid>

      <WorldCard id={id} />

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
                {gameUpdate && (
                  <Button size="xs" color="orange" loading={updateNow.isPending} onClick={confirmUpdate}>
                    Update now
                  </Button>
                )}
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
            {canOperate && (
              <Group>
                <Button
                  size="xs"
                  leftSection={<IconPlayerPlay size={14} />}
                  disabled={!canStart(status.state) || start.isPending}
                  loading={start.isPending}
                  onClick={() => start.mutate()}
                >
                  Start
                </Button>
                <Button
                  size="xs"
                  color="red"
                  variant="outline"
                  leftSection={<IconPlayerStop size={14} />}
                  disabled={!canStop(status.state) || stop.isPending}
                  loading={stop.isPending}
                  onClick={() => stop.mutate()}
                >
                  Stop
                </Button>
                <Button
                  size="xs"
                  variant="outline"
                  leftSection={<IconRefresh size={14} />}
                  disabled={!canRestart(status.state) || restart.isPending}
                  loading={restart.isPending}
                  onClick={confirmRestart}
                >
                  Restart
                </Button>
                {canInstall(status.state) && (
                  <Button size="xs" variant="light" loading={install.isPending} onClick={() => install.mutate(undefined, { onSuccess: (res) => openJob(res.job.id) })}>
                    Install
                  </Button>
                )}
              </Group>
            )}
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
        {!jobsQuery.isLoading && jobs.length === 0 && (
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
    </Stack>
  )
}
