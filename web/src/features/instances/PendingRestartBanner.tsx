// "Restart required" banner for an instance whose running server has not yet
// picked up saved configuration changes. Rendered at the instance-page level
// so it is visible on every tab, with the warned-restart control in it.
import { Alert, Group, Text } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import type { InstanceStatus } from '../../api/types'
import { RestartControl } from './RestartControl'

export function PendingRestartBanner({ id, name, status }: { id: string; name: string; status: InstanceStatus }) {
  if (!status.pending_restart) return null
  return (
    <Alert color="yellow" icon={<IconAlertTriangle size={16} />} title="Restart required to apply">
      <Group justify="space-between" wrap="wrap" gap="sm">
        <Text size="sm">Configuration changes are saved but will only take effect after a restart.</Text>
        <RestartControl id={id} instanceName={name} playersOnline={status.players_online} color="yellow" />
      </Group>
    </Alert>
  )
}
