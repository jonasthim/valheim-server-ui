import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  CopyButton,
  Group,
  Loader,
  Paper,
  Skeleton,
  SimpleGrid,
  Stack,
  Switch,
  Table,
  Text,
  Title,
  Tooltip,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconAlertTriangle, IconCheck, IconCopy, IconPlayerPlay, IconPlayerStop, IconRefresh } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo, fmtTime } from '../../lib/format'
import { useJobDrawer, useJobs, jobStatusColor, jobTypeLabel } from '../jobs'
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
import { canInstall, canRestart, canStart, canStop, isTransitioning, stateColor, stateLabel } from './instanceHelpers'

// Owned by WP-11. Props: the instance id.
export function OverviewTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const inst = useInstance(id)
  const statusQuery = useInstanceStatus(id)
  const { openJob } = useJobDrawer()

  const start = useStartInstance(id)
  const stop = useStopInstance(id)
  const restart = useRestartInstance(id)
  const install = useInstallInstance(id)
  const checkUpdate = useCheckForUpdate(id)
  const updateNow = useUpdateInstance(id)
  const setAutostart = useSetAutostart(id)

  const jobsQuery = useJobs({ instance: id, limit: 5 })

  if (inst.isLoading) {
    return (
      <Stack>
        <Skeleton height={160} />
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

  return (
    <Stack>
      <SimpleGrid cols={{ base: 1, md: 2 }}>
        <Paper withBorder p="md">
          <Stack gap="sm">
            <Group justify="space-between">
              <Title order={4}>Status</Title>
              <Badge color={stateColor(status.state)} variant="light" leftSection={isTransitioning(status.state) ? <Loader size={10} /> : null}>
                {stateLabel(status.state)}
              </Badge>
            </Group>
            <Group gap="xs">
              <Text size="sm" c="dimmed">
                Ready
              </Text>
              <Badge color={status.ready ? 'green' : 'gray'} variant="dot">
                {status.ready ? 'Yes' : 'No'}
              </Badge>
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
                  color="orange"
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
        </Paper>

        <Paper withBorder p="md">
          <Stack gap="sm">
            <Title order={4}>Players</Title>
            <Text size="sm">
              {status.players_online} / {status.max_players ?? '?'} online
            </Text>
            {status.join_code && (
              <Group gap="xs">
                <Text size="sm" c="dimmed">
                  Join code
                </Text>
                <Text ff="monospace">{status.join_code}</Text>
                <CopyButton value={status.join_code}>
                  {({ copied, copy }) => (
                    <Tooltip label={copied ? 'Copied' : 'Copy'}>
                      <ActionIcon variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} aria-label="Copy join code">
                        {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                      </ActionIcon>
                    </Tooltip>
                  )}
                </CopyButton>
              </Group>
            )}
          </Stack>
        </Paper>

        <Paper withBorder p="md">
          <Stack gap="sm">
            <Group justify="space-between">
              <Title order={4}>Build</Title>
              {status.update_available && (
                <Badge color="blue" variant="light">
                  Update available
                </Badge>
              )}
            </Group>
            <Text size="sm" c="dimmed">
              Installed build {status.installed_buildid ?? 'unknown'}
            </Text>
            <Group>
              <Badge color={status.bepinex_installed ? 'green' : 'gray'} variant="light">
                BepInEx {status.bepinex_installed ? 'installed' : 'not installed'}
              </Badge>
              <Badge color={status.bepinex_enabled ? 'green' : 'gray'} variant="light">
                BepInEx {status.bepinex_enabled ? 'enabled' : 'disabled'}
              </Badge>
            </Group>
            {canOperate && (
              <Group>
                <Button size="xs" variant="outline" loading={checkUpdate.isPending} onClick={() => checkUpdate.mutate()}>
                  Check for updates
                </Button>
                <Button size="xs" variant="light" disabled={!status.update_available} loading={updateNow.isPending} onClick={confirmUpdate}>
                  Update now
                </Button>
              </Group>
            )}
            {checkUpdate.data && (
              <Text size="xs" c="dimmed">
                Latest known build: {checkUpdate.data.latest_buildid ?? 'unknown'} (checked {fmtAgo(checkUpdate.data.checked_at)})
              </Text>
            )}
          </Stack>
        </Paper>

        <Paper withBorder p="md">
          <Stack gap="sm">
            <Title order={4}>Connect</Title>
            <Text size="sm" ff="monospace">
              {host}:{instance.config.port}
            </Text>
            <Text size="xs" c="dimmed">
              The query port (server browser / A2S) is port+1 ({instance.config.port + 1}).
            </Text>
          </Stack>
        </Paper>
      </SimpleGrid>

      <Paper withBorder p="md">
        <Stack gap="sm">
          <Title order={4}>Recent jobs</Title>
          {jobsQuery.isLoading && <Skeleton height={80} />}
          {!jobsQuery.isLoading && jobs.length === 0 && (
            <Text c="dimmed" size="sm">
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
                        <Badge size="sm" color={jobStatusColor(job.status)} variant="light">
                          {job.status}
                        </Badge>
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
        </Stack>
      </Paper>
    </Stack>
  )
}
