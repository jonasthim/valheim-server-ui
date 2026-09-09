import type { ScheduleKind } from '../../../api/types'

export const SCHEDULE_KIND_LABELS: Record<ScheduleKind, string> = {
  restart: 'Restart',
  backup: 'Backup',
  update: 'Update',
}

export const SCHEDULE_KIND_HELP: Record<ScheduleKind, string> = {
  restart: 'Restart the server; Valheim cannot warn players, so keep only_when_empty on.',
  backup: 'Create a scheduled backup.',
  update: 'Check Steam and update when a new build exists.',
}

export const SCHEDULE_KIND_OPTIONS: { value: ScheduleKind; label: string }[] = [
  { value: 'restart', label: SCHEDULE_KIND_LABELS.restart },
  { value: 'backup', label: SCHEDULE_KIND_LABELS.backup },
  { value: 'update', label: SCHEDULE_KIND_LABELS.update },
]

export const LAST_RESULT_COLORS: Record<'ok' | 'skipped' | 'failed', string> = {
  ok: 'green',
  skipped: 'gray',
  failed: 'red',
}
