import type { ScheduleKind } from '../../../api/types'

export const SCHEDULE_KIND_LABELS: Record<ScheduleKind, string> = {
  restart: 'Restart',
  backup: 'Backup',
  update: 'Update',
  announce: 'Announcement',
  command: 'Agent command',
  save: 'Save world',
}

export const SCHEDULE_KIND_HELP: Record<ScheduleKind, string> = {
  restart: 'Restart the server, warning connected players first over its own lead time.',
  backup: 'Create a scheduled backup.',
  update: 'Check Steam and update when a new build exists.',
  announce: 'Broadcast a message to connected players through the agent.',
  command: 'Send an agent command (e.g. broadcast, kick, ban, save).',
  save: 'Ask the running server to save the world.',
}

export const SCHEDULE_KIND_OPTIONS: { value: ScheduleKind; label: string }[] = [
  { value: 'restart', label: SCHEDULE_KIND_LABELS.restart },
  { value: 'backup', label: SCHEDULE_KIND_LABELS.backup },
  { value: 'update', label: SCHEDULE_KIND_LABELS.update },
  { value: 'announce', label: SCHEDULE_KIND_LABELS.announce },
  { value: 'command', label: SCHEDULE_KIND_LABELS.command },
  { value: 'save', label: SCHEDULE_KIND_LABELS.save },
]

export const LAST_RESULT_COLORS: Record<'ok' | 'skipped' | 'failed', string> = {
  ok: 'moss',
  skipped: 'gray',
  failed: 'blood',
}
