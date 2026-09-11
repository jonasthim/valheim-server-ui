// A small header badge that appears only while the SSE live-update stream is
// disconnected, so a dropped connection (stale data until it reconnects) is
// visible rather than silent. EventSource reconnects on its own; the badge
// clears when it does.
import { Badge, Tooltip } from '@mantine/core'
import { IconWifiOff } from '@tabler/icons-react'
import { useSSEConnected } from '../../events/useEvents'

export function LiveStatusBadge() {
  const connected = useSSEConnected()
  if (connected) return null
  return (
    <Tooltip label="Live updates are offline; reconnecting. Data may be briefly stale.">
      <Badge color="gray" variant="light" leftSection={<IconWifiOff size={12} />} radius="sm">
        offline
      </Badge>
    </Tooltip>
  )
}
