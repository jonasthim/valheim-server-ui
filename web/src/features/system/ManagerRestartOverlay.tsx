// Full-screen "restarting" cover, shown while `useManagerRestartWatch` waits
// for the manager process to come back up after a self-upgrade.
import { Loader, Overlay, Stack, Text } from '@mantine/core'

export function ManagerRestartOverlay({ visible }: { visible: boolean }) {
  if (!visible) return null
  return (
    <Overlay fixed center blur={2} backgroundOpacity={0.85} color="#000" zIndex={1000}>
      <Stack align="center" gap="sm">
        <Loader color="white" size="lg" />
        <Text c="white" fw={600}>
          Restarting Valheim Server UI…
        </Text>
      </Stack>
    </Overlay>
  )
}
