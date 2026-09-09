import {
  ActionIcon,
  Badge,
  Button,
  Card,
  CopyButton,
  Group,
  Loader,
  Stack,
  Text,
  Tooltip,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { Link } from 'react-router-dom'
import { IconCheck, IconCopy, IconDownload, IconPlayerPlay, IconPlayerStop, IconRefresh } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import type { Instance } from '../../api/types'
import { useJobDrawer, jobTypeLabel } from '../jobs'
import {
  useInstallInstance,
  useRestartInstance,
  useStartInstance,
  useStopInstance,
  canInstall,
  canRestart,
  canStart,
  canStop,
  isTransitioning,
  stateColor,
  stateLabel,
} from '../instances'

export function InstanceCard({ instance }: { instance: Instance }) {
  const { hasRole } = useAuth()
  const { openJob } = useJobDrawer()
  const { status, config } = instance

  const start = useStartInstance(instance.id)
  const stop = useStopInstance(instance.id)
  const restart = useRestartInstance(instance.id)
  const install = useInstallInstance(instance.id)

  const canOperate = hasRole('operator')

  function confirmRestart() {
    if (status.players_online > 0) {
      modals.openConfirmModal({
        title: 'Restart instance',
        children: (
          <Text size="sm">
            {status.players_online} player{status.players_online === 1 ? ' is' : 's are'} currently online on{' '}
            <strong>{instance.name}</strong>. Restart anyway?
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

  return (
    <Card withBorder padding="md" radius="md">
      <Stack gap="xs">
        <Group justify="space-between" wrap="nowrap">
          <Text component={Link} to={`/instances/${instance.id}/overview`} fw={600} truncate>
            {instance.name}
          </Text>
          <Badge color={stateColor(status.state)} variant="light" leftSection={isTransitioning(status.state) ? <Loader size={10} /> : null}>
            {stateLabel(status.state)}
          </Badge>
        </Group>
        <Group gap={6}>
          <Badge size="xs" color={status.ready ? 'green' : 'gray'} variant="dot">
            {status.ready ? 'Ready' : 'Not ready'}
          </Badge>
          {status.update_available && (
            <Badge size="xs" color="blue" variant="light">
              Update available
            </Badge>
          )}
          {status.pending_restart && (
            <Badge size="xs" color="yellow" variant="light">
              Restart pending
            </Badge>
          )}
        </Group>

        <Text size="sm" c="dimmed" truncate>
          {config.name} · {config.world}
        </Text>
        <Group gap="md">
          <Text size="sm">
            {status.players_online} / {status.max_players ?? '?'} players
          </Text>
          <Text size="sm">Port {config.port}</Text>
        </Group>
        {status.join_code && (
          <Group gap={4}>
            <Text size="sm" c="dimmed">
              Join code
            </Text>
            <Text size="sm" ff="monospace">
              {status.join_code}
            </Text>
            <CopyButton value={status.join_code}>
              {({ copied, copy }) => (
                <Tooltip label={copied ? 'Copied' : 'Copy'}>
                  <ActionIcon size="sm" variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} aria-label="Copy join code">
                    {copied ? <IconCheck size={12} /> : <IconCopy size={12} />}
                  </ActionIcon>
                </Tooltip>
              )}
            </CopyButton>
          </Group>
        )}

        {status.active_job && (
          <Text
            size="sm"
            c="blue"
            style={{ cursor: 'pointer' }}
            onClick={() => status.active_job && openJob(status.active_job.id)}
          >
            {jobTypeLabel(status.active_job.type)} - {status.active_job.status}
          </Text>
        )}

        {canOperate && (
          <Group gap="xs" mt="xs">
            <Button
              size="xs"
              leftSection={<IconPlayerPlay size={14} />}
              disabled={!canStart(status.state)}
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
              disabled={!canStop(status.state)}
              loading={stop.isPending}
              onClick={() => stop.mutate()}
            >
              Stop
            </Button>
            <Button
              size="xs"
              variant="outline"
              leftSection={<IconRefresh size={14} />}
              disabled={!canRestart(status.state)}
              loading={restart.isPending}
              onClick={confirmRestart}
            >
              Restart
            </Button>
            {canInstall(status.state) && (
              <Button
                size="xs"
                variant="light"
                leftSection={<IconDownload size={14} />}
                loading={install.isPending}
                onClick={() => install.mutate(undefined, { onSuccess: (res) => openJob(res.job.id) })}
              >
                Install
              </Button>
            )}
          </Group>
        )}
      </Stack>
    </Card>
  )
}
