// Confirm dialog for kicking a player, shared by the Players tab's online
// list and the Map tab's player list under the map (U-A6a). Precedent:
// openConfirmInstallAgent.tsx.
import { Text } from '@mantine/core'
import { modals } from '@mantine/modals'

export function openConfirmKick({ name, onConfirm }: { name: string; onConfirm: () => void }) {
  modals.openConfirmModal({
    title: `Kick ${name}`,
    children: (
      <Text size="sm">The player is disconnected immediately and can rejoin. Use the banned list to keep them out.</Text>
    ),
    labels: { confirm: 'Kick', cancel: 'Cancel' },
    confirmProps: { color: 'red' },
    onConfirm,
  })
}
