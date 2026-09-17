// Context plumbing for the shared upgrade flow (confirm → mutate → watch the
// job → restart overlay), split from the provider component so this file
// only exports non-component values (oxlint react/only-export-components).
// Mirrors web/src/features/jobs/jobDrawerContext.ts.
import { createContext, useContext } from 'react'

export interface UpgradeFlowState {
  /** Start watching this self_upgrade job for completion (drives the shared restart overlay). */
  startWatching: (jobId: string) => void
  /** True once the watched job has succeeded and we're waiting for the manager to come back up. */
  restarting: boolean
  /** True while an upgrade is in flight (queued, running, or exit_pending), per the polled system info. */
  inFlight: boolean
  /** Version being upgraded to, when known. */
  target?: string
}

export const UpgradeFlowCtx = createContext<UpgradeFlowState | null>(null)

/** Call from anywhere under `<UpgradeFlowHost>` to drive the shared upgrade overlay. */
export function useUpgradeFlow(): UpgradeFlowState {
  const ctx = useContext(UpgradeFlowCtx)
  if (!ctx) throw new Error('useUpgradeFlow outside UpgradeFlowHost')
  return ctx
}
