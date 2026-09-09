import { ActionIcon, Badge, Button, Card, CopyButton, Group, Pill, Stack, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { Link } from 'react-router-dom'
import {
  IconCheck,
  IconCopy,
  IconDownload,
  IconPlayerPlay,
  IconPlayerStop,
  IconPlug,
  IconRefresh,
  IconUsers,
} from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import type { Instance } from '../../api/types'
import { StatusDot, StatusPill } from '../../ui'
import { useJobDrawer, jobTypeLabel } from '../jobs'
import {
  useInstallInstance,
  useRestartInstance,
  useStartInstance,
  useStopInstance,
  useUpdateInstance,
  canInstall,
  canRestart,
  canStart,
  canStop,
  stateColor,
  stateLabel,
} from '../instances'
import classes from './InstanceCard.module.css'

export function InstanceCard({ instance }: { instance: Instance }) {
  const { hasRole } = useAuth()
  const { openJob } = useJobDrawer()
  const { status, config } = instance

  const start = useStartInstance(instance.id)
  const stop = useStopInstance(instance.id)
  const restart = useRestartInstance(instance.id)
  const install = useInstallInstance(instance.id)
  const updateNow = useUpdateInstance(instance.id)

  const canOperate = hasRole('operator')

  function confirmUpdate() {
    const running = status.state === 'running'
    if (running) {
      modals.openConfirmModal({
        title: 'Update game files',
        children: (
          <Text size="sm">
            <strong>{instance.name}</strong> is running and will be stopped, updated, and started again. Continue?
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
    <Card withBorder padding="lg" radius="lg" className={classes.card}>
      <Stack gap="sm">
        <Group justify="space-between" wrap="nowrap" align="flex-start">
          <Text component={Link} to={`/instances/${instance.id}/overview`} fw={650} truncate>
            {instance.name}
          </Text>
          <StatusPill color={stateColor(status.state)} pulse={status.state === 'running' || status.state === 'starting'}>
            {stateLabel(status.state)}
          </StatusPill>
        </Group>

        <Text size="sm" c="dimmed" truncate>
          {config.name} · {config.world}
        </Text>

        <Group gap="md" wrap="wrap">
          <Group gap={6} wrap="nowrap">
            <Text c="dimmed" component="span" style={{ display: 'inline-flex' }}>
              <IconUsers size={14} />
            </Text>
            <Text size="sm">
              {status.players_online} / {status.max_players ?? '?'}
            </Text>
          </Group>
          <Group gap={6} wrap="nowrap">
            <Text c="dimmed" component="span" style={{ display: 'inline-flex' }}>
              <IconPlug size={14} />
            </Text>
            <Text size="sm">{config.port}</Text>
          </Group>
          {status.join_code && (
            <Group gap={4} wrap="nowrap">
              <Pill size="sm" className={classes.joinCode}>
                {status.join_code}
              </Pill>
              <CopyButton value={status.join_code}>
                {({ copied, copy }) => (
                  <Tooltip label={copied ? 'Copied' : 'Copy'}>
                    <ActionIcon size="sm" variant="subtle" color={copied ? 'moss' : 'gray'} onClick={copy} aria-label="Copy join code">
                      {copied ? <IconCheck size={12} /> : <IconCopy size={12} />}
                    </ActionIcon>
                  </Tooltip>
                )}
              </CopyButton>
            </Group>
          )}
        </Group>

        <Group gap="xs" wrap="wrap">
          <Group gap={4}>
            <StatusDot color={status.ready ? 'moss' : 'gray'} />
            <Text size="xs" c="dimmed">
              {status.ready ? 'Ready' : 'Not ready'}
            </Text>
          </Group>
          {status.update_available && (
            <Badge size="xs" color="orange" variant="light">
              Game update available
            </Badge>
          )}
          {status.pending_restart && (
            <Badge size="xs" color="yellow" variant="light">
              Restart pending
            </Badge>
          )}
        </Group>

        {status.active_job && (
          <Text size="sm" c="blue" style={{ cursor: 'pointer' }} onClick={() => status.active_job && openJob(status.active_job.id)}>
            {jobTypeLabel(status.active_job.type)} - {status.active_job.status}
          </Text>
        )}

        {canOperate && (
          <Group gap="xs" mt={4}>
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
              color="red"
              variant="light"
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
            {status.update_available && (
              <Button
                size="xs"
                color="orange"
                variant="light"
                leftSection={<IconDownload size={14} />}
                loading={updateNow.isPending}
                onClick={confirmUpdate}
              >
                Update
              </Button>
            )}
          </Group>
        )}
      </Stack>
    </Card>
  )
}
