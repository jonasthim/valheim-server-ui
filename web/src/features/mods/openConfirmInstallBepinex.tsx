// Confirm dialog for installing or upgrading BepInEx. Split out of
// BepInExCard so AgentSetupNotice can open the same dialog from any mount
// point (see docs/WORKPLAN.md U-A2).
import { Stack } from '@mantine/core'
import { modals } from '@mantine/modals'

/** Opens the confirm dialog for installing or upgrading BepInEx; calls
 * `onConfirm(stopIfRunning)` once the user confirms. */
export function openConfirmInstallBepinex({
  upgrade,
  isRunning,
  onConfirm,
}: {
  upgrade: boolean
  isRunning: boolean
  onConfirm: (stopIfRunning: boolean) => void
}) {
  const verb = upgrade ? 'Upgrade' : 'Install'
  modals.openConfirmModal({
    title: `${verb} BepInEx`,
    children: (
      <Stack gap="xs">
        <span>
          BepInEx must be installed while the instance is <strong>stopped</strong>.
        </span>
        {isRunning && (
          <span style={{ color: 'var(--vh-text-soft)' }}>
            This instance is currently running. Confirming will stop it, {verb.toLowerCase()} BepInEx, and start it
            again afterwards.
          </span>
        )}
      </Stack>
    ),
    labels: {
      confirm: isRunning ? `Stop, ${verb.toLowerCase()} and start again` : verb,
      cancel: 'Cancel',
    },
    onConfirm: () => onConfirm(isRunning),
  })
}
