// Query hooks for Valheim's own rolling world copies (Backups tab → Game backups).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../../api/client'
import type { GameBackup, Job } from '../../../api/types'
import { useJobDrawer } from '../../jobs'
import { notifyError, notifySuccess } from '../../../lib/notify'

export function gameBackupsKey(id: string) {
  return ['instances', id, 'game-backups'] as const
}

/** GET /instances/{id}/game-backups. */
export function useGameBackups(id: string) {
  return useQuery({
    queryKey: gameBackupsKey(id),
    queryFn: () => api.get<{ copies: GameBackup[] }>(`/instances/${id}/game-backups`).then((r) => r.copies),
    enabled: !!id,
    refetchInterval: 60_000,
  })
}

/** POST /instances/{id}/game-backups/{name}/restore → 202 Job, opened in the job drawer. */
export function useRestoreGameBackup(id: string) {
  const qc = useQueryClient()
  const { openJob } = useJobDrawer()
  return useMutation({
    mutationFn: ({ name, stopIfRunning }: { name: string; stopIfRunning: boolean }) =>
      api.post<{ job: Job }>(`/instances/${id}/game-backups/${encodeURIComponent(name)}/restore`, {
        stop_if_running: stopIfRunning,
      }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: gameBackupsKey(id) })
      qc.invalidateQueries({ queryKey: ['instances', id, 'backups'] })
      qc.invalidateQueries({ queryKey: ['instances', id, 'worlds'] })
      notifySuccess('Restore queued')
      openJob(res.job.id)
    },
    onError: (err) => notifyError(err, 'Could not start restore'),
  })
}
