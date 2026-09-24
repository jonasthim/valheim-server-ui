// Plain confirm-dialog helpers shared by LifecycleControls (Overview tab and
// the dashboard instance card) so the update and stop confirmations behave
// identically everywhere instead of being duplicated per call site.
import { modals } from '@mantine/modals'
import { Text } from '@mantine/core'

/**
 * Confirms updating the game server files when the instance is running (it
 * will be stopped, updated, and started again). When it isn't running,
 * there's nothing to warn about, so `onConfirm` fires immediately.
 */
export function openConfirmUpdateGame(opts: { instanceName: string; running: boolean; onConfirm: () => void }) {
  if (!opts.running) {
    opts.onConfirm()
    return
  }
  modals.openConfirmModal({
    title: 'Update game files',
    children: (
      <Text size="sm">
        <strong>{opts.instanceName}</strong> is running and will be stopped, updated, and started again. Continue?
      </Text>
    ),
    labels: { confirm: 'Stop and update', cancel: 'Cancel' },
    confirmProps: { color: 'orange' },
    onConfirm: opts.onConfirm,
  })
}

/**
 * Confirms stopping the instance when players are online (they'll be
 * disconnected immediately). Skips the dialog when the server is empty.
 */
export function openConfirmStop(opts: { instanceName: string; playersOnline: number; onConfirm: () => void }) {
  if (opts.playersOnline === 0) {
    opts.onConfirm()
    return
  }
  modals.openConfirmModal({
    title: 'Stop instance',
    children: (
      <Text size="sm">
        {opts.playersOnline} player{opts.playersOnline === 1 ? ' is' : 's are'} online and will be disconnected
        immediately.
      </Text>
    ),
    labels: { confirm: 'Stop', cancel: 'Cancel' },
    confirmProps: { color: 'red' },
    onConfirm: opts.onConfirm,
  })
}

/**
 * Confirms restarting the instance when players are online (they'll be
 * disconnected immediately unless a delay is chosen instead). Skips the
 * dialog when the server is empty. Shared by RestartControl (its "Restart
 * now" menu item) and the command palette's "Restart {name}" action.
 */
export function openConfirmRestart(opts: { instanceName?: string; playersOnline: number; onConfirm: () => void }) {
  if (opts.playersOnline === 0) {
    opts.onConfirm()
    return
  }
  modals.openConfirmModal({
    title: 'Restart instance',
    children: (
      <Text size="sm">
        {opts.instanceName && (
          <>
            Restart <strong>{opts.instanceName}</strong>?{' '}
          </>
        )}
        {opts.playersOnline} player{opts.playersOnline === 1 ? ' is' : 's are'} currently online and will be
        disconnected immediately. Restart now, or pick a delay to warn them first.
      </Text>
    ),
    labels: { confirm: 'Restart now', cancel: 'Cancel' },
    confirmProps: { color: 'orange' },
    onConfirm: opts.onConfirm,
  })
}
