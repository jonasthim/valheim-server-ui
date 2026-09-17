// Pure helpers shared by the jobs feature and reused by other WPs (backups,
// mods, schedules tabs) to render job status/type consistently.
import type { Job, JobStatus, JobType } from '../../api/types'
import type { InstanceTab } from '../instances/tabs'

const JOB_STATUS_COLORS: Record<JobStatus, string> = {
  queued: 'gray',
  running: 'blue',
  succeeded: 'green',
  failed: 'red',
  cancelled: 'yellow',
}

export function jobStatusColor(status: JobStatus): string {
  return JOB_STATUS_COLORS[status]
}

const JOB_TYPE_LABELS: Record<JobType, string> = {
  install: 'Install game files',
  update: 'Update game files',
  backup: 'Backup',
  restore: 'Restore',
  world_import: 'Import world',
  world_regenerate: 'Regenerate world',
  mod_install: 'Install mod',
  mod_update: 'Update mod',
  mod_uninstall: 'Uninstall mod',
  bepinex_install: 'Install BepInEx',
  agent_install: 'Install Valheim UI Agent',
  scheduled_restart: 'Scheduled restart',
  restart: 'Restart',
  thunderstore_refresh: 'Refresh Thunderstore index',
  self_upgrade: 'Upgrade Valheim Server UI',
}

export function jobTypeLabel(type: JobType): string {
  return JOB_TYPE_LABELS[type]
}

/** Which instance tab a job of this type was produced from/for. */
export function jobTabFor(type: JobType): InstanceTab {
  switch (type) {
    case 'backup':
    case 'restore':
      return 'backups'
    case 'world_import':
    case 'world_regenerate':
      return 'worlds'
    case 'mod_install':
    case 'mod_update':
    case 'mod_uninstall':
    case 'bepinex_install':
    case 'agent_install':
      return 'mods'
    default:
      return 'overview'
  }
}

/** Path to the instance tab that produced this job, or null for global jobs (no instance_id). */
export function jobInstancePath(job: Job): string | null {
  return job.instance_id ? `/instances/${job.instance_id}/${jobTabFor(job.type)}` : null
}

/** True once a job has reached a terminal status (no further job.log lines expected). */
export function isJobTerminal(status: JobStatus): boolean {
  return status === 'succeeded' || status === 'failed' || status === 'cancelled'
}

/** True while a job can still be cancelled. */
export function isJobCancellable(status: JobStatus): boolean {
  return status === 'queued' || status === 'running'
}

/**
 * Human duration: finished-started when done, started-now while running, else "-".
 * `now` defaults to Date.now() but can be pinned for tests/deterministic ticks.
 */
export function jobDuration(job: Pick<Job, 'started_at' | 'finished_at'>, now: number = Date.now()): string {
  if (!job.started_at) return '-'
  const start = new Date(job.started_at).getTime()
  const end = job.finished_at ? new Date(job.finished_at).getTime() : now
  const totalSec = Math.max(0, Math.round((end - start) / 1000))
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}
