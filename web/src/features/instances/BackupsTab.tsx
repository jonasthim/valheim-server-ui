import { Stack } from '@mantine/core'
import { useAuth } from '../../auth/useAuth'
import { useInstance } from './useInstance'
import { BackupNowCard } from './backups/BackupNowCard'
import { BackupUploadCard } from './backups/BackupUploadCard'
import { BackupsTable } from './backups/BackupsTable'
import { RetentionInfo } from './backups/RetentionInfo'
import { useBackups, useDeleteBackup, useRestoreBackup } from './backups/useBackups'

// Owned by WP-12 (docs/WORKPLAN.md). Props: the instance id.
export function BackupsTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canManage = hasRole('operator')
  const instanceQ = useInstance(id)
  const backupsQ = useBackups(id)
  const restore = useRestoreBackup(id)
  const del = useDeleteBackup(id)

  return (
    <Stack gap="md">
      {canManage && <BackupNowCard id={id} />}

      <BackupsTable
        id={id}
        backups={backupsQ.data ?? []}
        isLoading={backupsQ.isLoading}
        canManage={canManage}
        instanceState={instanceQ.data?.status.state}
        retention={<RetentionInfo id={id} config={instanceQ.data?.config} />}
        onRestore={(backupId, stopIfRunning) => restore.mutate({ backupId, stopIfRunning })}
        onDelete={(backupId) => del.mutate(backupId)}
      />

      {canManage && <BackupUploadCard id={id} />}
    </Stack>
  )
}
