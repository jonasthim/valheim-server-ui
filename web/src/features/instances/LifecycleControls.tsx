// Unified Start/Stop/Restart/Install/Update row shared by the instance page
// header and the dashboard instance card, so lifecycle actions look and behave
// identically everywhere (confirms, warnings, job-drawer hand-off).
import { Button, Group } from '@mantine/core'
import { IconDownload, IconPlayerPlay, IconPlayerStop } from '@tabler/icons-react'
import type { InstanceStatus } from '../../api/types'
import { useAuth } from '../../auth/useAuth'
import { useJobDrawer } from '../jobs'
import { useInstallInstance, useStartInstance, useStopInstance, useUpdateInstance } from './instanceActions'
import { canInstall, canRestart, canStart, canStop } from './instanceHelpers'
import { openConfirmStop, openConfirmUpdateGame } from './openConfirmLifecycle'
import { RestartControl } from './RestartControl'

export function LifecycleControls({
  id,
  name,
  status,
  size = 'xs',
}: {
  id: string
  name: string
  status: InstanceStatus
  size?: string
}) {
  const { hasRole } = useAuth()
  const { openJob } = useJobDrawer()
  const start = useStartInstance(id)
  const stop = useStopInstance(id)
  const install = useInstallInstance(id)
  const updateNow = useUpdateInstance(id)

  if (!hasRole('operator')) return null

  return (
    <Group gap="xs" wrap="wrap">
      <Button
        size={size}
        leftSection={<IconPlayerPlay size={14} />}
        disabled={!canStart(status.state)}
        loading={start.isPending}
        onClick={() => start.mutate()}
      >
        Start
      </Button>
      <Button
        size={size}
        color="red"
        variant="light"
        leftSection={<IconPlayerStop size={14} />}
        disabled={!canStop(status.state)}
        loading={stop.isPending}
        onClick={() =>
          openConfirmStop({
            instanceName: name,
            playersOnline: status.players_online,
            onConfirm: () => stop.mutate(),
          })
        }
      >
        Stop
      </Button>
      <RestartControl
        id={id}
        instanceName={name}
        playersOnline={status.players_online}
        disabled={!canRestart(status.state)}
        size={size}
      />
      {canInstall(status.state) && (
        <Button
          size={size}
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
          size={size}
          color="orange"
          variant="light"
          leftSection={<IconDownload size={14} />}
          loading={updateNow.isPending}
          onClick={() =>
            openConfirmUpdateGame({
              instanceName: name,
              running: status.state === 'running',
              onConfirm: () =>
                updateNow.mutate(status.state === 'running', { onSuccess: (res) => openJob(res.job.id) }),
            })
          }
        >
          Update
        </Button>
      )}
    </Group>
  )
}
