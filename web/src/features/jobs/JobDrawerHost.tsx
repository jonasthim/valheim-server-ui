// Provides `useJobDrawer()` to its subtree and renders the shared JobDrawer.
//
// Mount this once, as high as convenient:
//   - Locally (as JobsPage does): wrap the page's own content so the table's
//     row click and the `?job=` deep link can open the drawer.
//   - Globally: wrap <Outlet /> in web/src/layout/Shell.tsx (owned by the
//     architect, not this WP) so any feature — backups, mods, schedules —
//     can `import { useJobDrawer } from '../jobs'` and call `openJob(id)`
//     to show a job's log without navigating to /jobs. Mounting it twice
//     (globally and inside JobsPage) is harmless: the nearest host wins.
import { useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { JobDrawerCtx, type JobDrawerState } from './jobDrawerContext'
import { JobDrawer } from './JobDrawer'

export function JobDrawerHost({ children }: { children: ReactNode }) {
  const [jobId, setJobId] = useState<string | null>(null)

  const closeJob = useCallback(() => setJobId(null), [])
  const openJob = useCallback((id: string) => setJobId(id), [])
  const value = useMemo<JobDrawerState>(() => ({ openJob, closeJob }), [openJob, closeJob])

  return (
    <JobDrawerCtx.Provider value={value}>
      {children}
      <JobDrawer jobId={jobId} onClose={closeJob} />
    </JobDrawerCtx.Provider>
  )
}
