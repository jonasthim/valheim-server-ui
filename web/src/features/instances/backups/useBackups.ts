// Query hooks for the backups tab.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../../api/client'
import type { Backup, Job } from '../../../api/types'
import { useJobDrawer } from '../../jobs'
import { notifyError, notifySuccess } from '../../../lib/notify'

function backupsKey(id: string) {
  return ['instances', id, 'backups'] as const
}

/** GET /instances/{id}/backups. */
export function useBackups(id: string) {
  return useQuery({
    queryKey: backupsKey(id),
    queryFn: () => api.get<{ backups: Backup[] }>(`/instances/${id}/backups`).then((r) => r.backups),
    enabled: !!id,
  })
}

/** POST /instances/{id}/backups → 202 Job, opened in the job drawer. */
export function useCreateBackup(id: string) {
  const qc = useQueryClient()
  const { openJob } = useJobDrawer()
  return useMutation({
    mutationFn: (note: string) => api.post<{ job: Job }>(`/instances/${id}/backups`, note ? { note } : {}),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: backupsKey(id) })
      notifySuccess('Backup started')
      openJob(res.job.id)
    },
    onError: (err) => notifyError(err, 'Could not start backup'),
  })
}

/** POST /instances/{id}/backups/upload (multipart) → 201 Backup, no job. */
export function useUploadBackup(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData()
      form.append('file', file)
      return api.upload<{ backup: Backup }>(`/instances/${id}/backups/upload`, form)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: backupsKey(id) })
      notifySuccess('Backup uploaded')
    },
    onError: (err) => notifyError(err, 'Could not upload backup'),
  })
}

/** DELETE /instances/{id}/backups/{backupId}. */
export function useDeleteBackup(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (backupId: number) => api.del<void>(`/instances/${id}/backups/${backupId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: backupsKey(id) })
      notifySuccess('Backup deleted')
    },
    onError: (err) => notifyError(err, 'Could not delete backup'),
  })
}

/** POST /instances/{id}/backups/{backupId}/restore → 202 Job, opened in the job drawer. */
export function useRestoreBackup(id: string) {
  const qc = useQueryClient()
  const { openJob } = useJobDrawer()
  return useMutation({
    mutationFn: ({ backupId, stopIfRunning }: { backupId: number; stopIfRunning: boolean }) =>
      api.post<{ job: Job }>(`/instances/${id}/backups/${backupId}/restore`, { stop_if_running: stopIfRunning }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: backupsKey(id) })
      notifySuccess('Restore started')
      openJob(res.job.id)
    },
    onError: (err) => notifyError(err, 'Could not start restore'),
  })
}
