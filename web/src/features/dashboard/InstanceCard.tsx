import { useState } from 'react'
import { ActionIcon, Anchor, Card, Code, CopyButton, Group, Text, Tooltip } from '@mantine/core'
import { Link } from 'react-router-dom'
import { IconCheck, IconCopy, IconCpu, IconPlug, IconUsers, IconWorld } from '@tabler/icons-react'
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
  const [thumb, setThumb] = useState<'loading' | 'ready' | 'none'>('loading')

  return (
    <Card padding={0} className={classes.card}>
      <div className={thumb === 'ready' ? classes.thumb : classes.headerStrip}>
        {thumb !== 'none' && (
          <img
            className={classes.thumbImg}
            src={`${API_BASE}/instances/${instance.id}/map/tiles/0/0/0.png`}
            alt=""
            style={thumb === 'ready' ? undefined : { display: 'none' }}
            onLoad={() => setThumb('ready')}
            onError={() => setThumb('none')}
          />
        )}
      </div>
      <div className={classes.body}>
        <Group justify="space-between" wrap="nowrap" gap="sm">
          <Group gap={8} wrap="nowrap" style={{ minWidth: 0 }}>
            <StatusDot color={stateColor(status.state)} pulse={status.state === 'running'} />
            <Text component={Link} to={`/instances/${instance.id}/overview`} size="sm" fw={600} truncate className={classes.name}>
              {instance.name}
            </Text>
          </Group>
          {status.players_online > 0 && (
            <Text size="xs" c="dimmed" style={{ flex: 'none' }}>
              {status.players_online} online
            </Text>
          )}
        </Group>

        <Group gap="md" wrap="wrap" className={classes.meta}>
          <span className={classes.metaItem}>
            <IconWorld size={14} />
            World {config.world}
          </span>
          <span className={classes.metaItem}>
            <IconUsers size={14} />
            {status.players_online} / {status.max_players ?? '?'}
          </span>
          <span className={classes.metaItem}>
            <IconPlug size={14} />
            {config.port}
          </span>
          {status.memory_bytes !== undefined && (
            <Tooltip label="Game process CPU (percent of one core) and resident memory">
              <span className={classes.metaItem}>
                <IconCpu size={14} />
                {fmtPercent(status.cpu_percent)}, {fmtBytes(status.memory_bytes)}
              </span>
            </Tooltip>
          )}
          {status.join_code && (
            <Group gap={4} wrap="nowrap">
              <Code className={classes.joinCode}>{status.join_code}</Code>
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

        <Group gap={6} wrap="wrap">
          <StatusPill color={stateColor(status.state)} pulse={status.state === 'running' || status.state === 'starting'}>
            {stateLabel(status.state)}
          </StatusPill>
          <StatusPill color={status.ready ? 'moss' : 'gray'}>{status.ready ? 'Ready' : 'Not ready'}</StatusPill>
          {status.update_available && <StatusPill color="orange">Game update available</StatusPill>}
          {status.pending_restart && <StatusPill color="yellow">Restart pending</StatusPill>}
          {status.active_job && (
            <Anchor component="button" type="button" size="xs" onClick={() => openJob(status.active_job!.id)}>
              {jobTypeLabel(status.active_job.type)} - {status.active_job.status}
            </Anchor>
          )}
        </Group>

        <LifecycleControls id={instance.id} name={instance.name} status={instance.status} />
      </div>
    </Card>
  )
}
