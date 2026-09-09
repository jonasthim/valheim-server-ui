import { useContext } from 'react'
import { JobDrawerCtx, type JobDrawerState } from './jobDrawerContext'

/**
 * Call from anywhere under a `<JobDrawerHost>` to open the shared job log
 * drawer, e.g. `useJobDrawer().openJob(job.id)` from a backups/mods/schedules
 * row. Safe to call even with no host mounted (no-op), so features do not
 * need to special-case it.
 */
export function useJobDrawer(): JobDrawerState {
  return useContext(JobDrawerCtx)
}
