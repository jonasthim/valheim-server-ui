// Confirm dialog for installing or updating the Valheim UI Agent and the
// Gameplay plugin, bundled together. Split out of the mod loader card so
// AgentSetupNotice and the Gameplay card can open the same dialog from any
// mount point (see docs/WORKPLAN.md U-A2).
import { Stack } from '@mantine/core'
import { modals } from '@mantine/modals'

/** Opens the confirm dialog for installing or updating the agent; calls
 * `onConfirm(stopIfRunning)` once the user confirms. */
export function openConfirmInstallAgent({
  update,
  isRunning,
  onConfirm,
}: {
  update: boolean
  isRunning: boolean
  onConfirm: (stopIfRunning: boolean) => void
}) {
  const verb = update ? 'Update' : 'Install'
  modals.openConfirmModal({
    title: `${verb} Valheim UI plugins`,
    children: (
      <Stack gap="xs">
        <span>
          This installs the agent and the gameplay plugin together, while the instance is <strong>stopped</strong>;
          the game keeps plugin files open while it runs.
        </span>
        {isRunning && (
          <span style={{ color: 'var(--vh-text-soft)' }}>
            This instance is running. Confirming will stop it, {verb.toLowerCase()} the plugins, and start it again.
          </span>
        )}
      </Stack>
    ),
    labels: { confirm: isRunning ? `Stop, ${verb.toLowerCase()} and start again` : verb, cancel: 'Cancel' },
    onConfirm: () => onConfirm(isRunning),
  })
}
