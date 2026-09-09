import { modals } from '@mantine/modals'
import { ConfirmWorldActionModal } from './ConfirmWorldActionModal'

const MODAL_ID = 'confirm-world-action'

/** Opens the type-to-confirm dialog for deleting or regenerating a world. */
export function openConfirmWorldAction(opts: {
  worldName: string
  action: 'delete' | 'regenerate'
  instanceRunning: boolean
  onConfirm: (o: { stopIfRunning: boolean }) => void
}) {
  modals.open({
    modalId: MODAL_ID,
    title: opts.action === 'regenerate' ? `Regenerate world "${opts.worldName}"` : `Delete world "${opts.worldName}"`,
    children: (
      <ConfirmWorldActionModal
        worldName={opts.worldName}
        action={opts.action}
        instanceRunning={opts.instanceRunning}
        onCancel={() => modals.close(MODAL_ID)}
        onConfirm={(o) => {
          modals.close(MODAL_ID)
          opts.onConfirm(o)
        }}
      />
    ),
  })
}
