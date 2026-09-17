// Provides `useUpgradeFlow()` to its subtree and renders the single shared
// `ManagerRestartOverlay`. Mount this once, high in the tree (Shell.tsx,
// inside JobDrawerHost since starting a watch also opens the job drawer) so
// both AppUpdateBanner and SettingsPage drive the same upgrade-watch state
// instead of each keeping (and rendering) their own.
import { useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { ManagerRestartOverlay } from './ManagerRestartOverlay'
import { UpgradeFlowCtx, type UpgradeFlowState } from './upgradeFlowContext'
import { useManagerRestartWatch, useUpgradeInFlight } from './useAppUpdate'

export function UpgradeFlowHost({ children }: { children: ReactNode }) {
  const [jobId, setJobId] = useState<string | undefined>()
  const { inFlight, target } = useUpgradeInFlight()
  const { restarting } = useManagerRestartWatch({ jobId, active: inFlight })

  const startWatching = useCallback((id: string) => setJobId(id), [])
  const value = useMemo<UpgradeFlowState>(
    () => ({ startWatching, restarting, inFlight, target }),
    [startWatching, restarting, inFlight, target],
  )

  return (
    <UpgradeFlowCtx.Provider value={value}>
      {children}
      <ManagerRestartOverlay visible={restarting || inFlight} />
    </UpgradeFlowCtx.Provider>
  )
}
