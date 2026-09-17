import { useState } from 'react'
import { ActionIcon, Badge, Card, CopyButton, Group, Pill, Stack, Text, Tooltip } from '@mantine/core'
import { Link } from 'react-router-dom'
import { IconCheck, IconCpu, IconCopy, IconPlug, IconUsers } from '@tabler/icons-react'
import { fmtBytes, fmtPercent } from '../../lib/format'
import type { Instance } from '../../api/types'
import { API_BASE } from '../../api/client'
import { StatusDot, StatusPill } from '../../ui'
import { useJobDrawer, jobTypeLabel } from '../jobs'
import { LifecycleControls, stateColor, stateLabel } from '../instances'
import classes from './InstanceCard.module.css'

export function InstanceCard({ instance }: { instance: Instance }) {
  const { openJob } = useJobDrawer()
  const { status, config } = instance
  const [showMap, setShowMap] = useState(true)

  return (
    <Card withBorder padding={0} radius="lg" className={classes.card}>
      <div className={classes.mapTop}>
        {showMap && (
          <img
            className={classes.mapImg}
            src={`${API_BASE}/instances/${instance.id}/map/tiles/0/0/0.png`}
            alt=""
            loading="lazy"
            onError={() => setShowMap(false)}
          />
        )}
        <div className={classes.mapStrip}>
          <StatusDot color={stateColor(instance.status.state)} pulse={instance.status.state === 'running'} />
          <Text component={Link} to={`/instances/${instance.id}/overview`} className={classes.mapName} truncate>
            {instance.name}
          </Text>
          {instance.status.players_online > 0 && (
            <Text size="xs" c="dimmed" ml="auto">
              {instance.status.players_online} online
            </Text>
          )}
        </div>
      </div>
      <div className={classes.body}>
        <Stack gap="sm">
          <Group justify="space-between" wrap="nowrap" align="flex-start">
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
            {status.memory_bytes !== undefined && (
              <Tooltip label="Game process CPU (percent of one core) and resident memory">
                <Group gap={6} wrap="nowrap">
                  <Text c="dimmed" component="span" style={{ display: 'inline-flex' }}>
                    <IconCpu size={14} />
                  </Text>
                  <Text size="sm">
                    {fmtPercent(status.cpu_percent)} · {fmtBytes(status.memory_bytes)}
                  </Text>
                </Group>
              </Tooltip>
            )}
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

          <LifecycleControls id={instance.id} name={instance.name} status={instance.status} />
        </Stack>
      </div>
    </Card>
  )
}
