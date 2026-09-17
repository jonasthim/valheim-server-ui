// The shared "upgrade the manager" action: confirm → POST upgrade → hand the
// job to the shared watcher (UpgradeFlowHost) and the job drawer. Lives in
// its own .tsx file because it renders JSX for the confirm body — oxlint's
// react/only-export-components forbids mixing that with a component export,
// so this file exports only the hook (precedent: openConfirmLifecycle.tsx).
import { Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { useJobDrawer } from '../jobs'
import { useUpgradeFlow } from './upgradeFlowContext'
import { UPGRADE_EXPLANATION, useUpgradeApp } from './useAppUpdate'

export function useUpgradeAppAction() {
  const { openJob } = useJobDrawer()
  const { startWatching, restarting, inFlight, target } = useUpgradeFlow()
  const upgradeApp = useUpgradeApp()

  function upgrade() {
    modals.openConfirmModal({
      title: 'Upgrade Valheim Server UI',
      children: <Text size="sm">{UPGRADE_EXPLANATION}</Text>,
      labels: { confirm: 'Upgrade now', cancel: 'Cancel' },
      onConfirm: () =>
        upgradeApp.mutate(undefined, {
          onSuccess: (res) => {
            startWatching(res.job.id)
            openJob(res.job.id)
          },
        }),
    })
  }

  return { upgrade, isPending: upgradeApp.isPending, inFlight, target, restarting }
}
