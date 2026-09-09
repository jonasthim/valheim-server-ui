import type { BackupKind } from '../../../api/types'

export const BACKUP_KIND_COLORS: Record<BackupKind, string> = {
  manual: 'blue',
  scheduled: 'teal',
  pre_update: 'orange',
  pre_restore: 'grape',
  uploaded: 'gray',
}

export const BACKUP_KIND_LABELS: Record<BackupKind, string> = {
  manual: 'Manual',
  scheduled: 'Scheduled',
  pre_update: 'Pre-update',
  pre_restore: 'Pre-restore',
  uploaded: 'Uploaded',
}
