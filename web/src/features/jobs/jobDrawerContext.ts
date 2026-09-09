// Context plumbing for the job drawer, split from the provider component so
// this file only exports non-component values (oxlint react/only-export-components).
import { createContext } from 'react'

export interface JobDrawerState {
  /** Open the drawer for this job id (e.g. from a link, a row click, a job reference elsewhere in the app). */
  openJob: (jobId: string) => void
  /** Close the drawer if open. */
  closeJob: () => void
}

const noop: JobDrawerState = { openJob: () => {}, closeJob: () => {} }

export const JobDrawerCtx = createContext<JobDrawerState>(noop)
