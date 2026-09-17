// Confirm dialog for installing or updating the Valheim UI Agent. Split out
// of AgentCard so AgentSetupNotice can open the same dialog from any mount
// point (see docs/WORKPLAN.md U-A2).
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
    title: `${verb} Valheim UI Agent`,
    children: (
      <Stack gap="xs">
        <span>
          The agent is installed while the instance is <strong>stopped</strong>; the game keeps plugin files open
          while it runs.
        </span>
        {isRunning && (
          <span style={{ color: 'var(--vh-text-soft)' }}>
            This instance is running. Confirming will stop it, {verb.toLowerCase()} the agent, and start it again.
          </span>
        )}
      </Stack>
    ),
    labels: { confirm: isRunning ? `Stop, ${verb.toLowerCase()} and start again` : verb, cancel: 'Cancel' },
    onConfirm: () => onConfirm(isRunning),
  })
}
