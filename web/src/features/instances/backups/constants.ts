import type { BackupKind } from '../../../api/types'

export const BACKUP_KIND_COLORS: Record<BackupKind, string> = {
  manual: 'frost',
  scheduled: 'moss',
  pre_update: 'straw',
  pre_restore: 'spirit',
  uploaded: 'gray',
}

export const BACKUP_KIND_LABELS: Record<BackupKind, string> = {
  manual: 'Manual',
  scheduled: 'Scheduled',
  pre_update: 'Pre-update',
  pre_restore: 'Pre-restore',
  uploaded: 'Uploaded',
}
