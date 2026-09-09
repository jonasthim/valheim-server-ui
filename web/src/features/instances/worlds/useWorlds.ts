// Query hooks for the worlds tab.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../../api/client'
import type { Instance, InstanceConfig, Job, World } from '../../../api/types'
import { useJobDrawer } from '../../jobs'
import { notifyError, notifySuccess } from '../../../lib/notify'

function worldsKey(id: string) {
  return ['instances', id, 'worlds'] as const
}

/** GET /instances/{id}/worlds. */
export function useWorlds(id: string) {
  return useQuery({
    queryKey: worldsKey(id),
    queryFn: () => api.get<{ worlds: World[] }>(`/instances/${id}/worlds`).then((r) => r.worlds),
    enabled: !!id,
  })
}

/**
 * PATCH /instances/{id} with the full config, `world` replaced by the
 * selected one. Callers build `config` from `useInstance(id)`'s current
 * value; the API sets `pending_restart` since the running process keeps the
 * old world loaded until restarted.
 */
export function useSetActiveWorld(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (config: InstanceConfig) => api.patch<{ instance: Instance }>(`/instances/${id}`, { config }),
    onSuccess: (res) => {
      qc.setQueryData(['instances', id, 'detail'], res.instance)
      qc.invalidateQueries({ queryKey: ['instances'] })
      notifySuccess('Active world updated — restart the instance to apply it')
    },
    onError: (err) => notifyError(err, 'Could not change the active world'),
  })
}

/** DELETE /instances/{id}/worlds/{name} (inactive worlds only). */
export function useDeleteWorld(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.del<void>(`/instances/${id}/worlds/${encodeURIComponent(name)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: worldsKey(id) })
      notifySuccess('World deleted')
    },
    onError: (err) => notifyError(err, 'Could not delete world'),
  })
}

/**
 * POST /instances/{id}/worlds/{name}/regenerate → 202 Job (opened in the job
 * drawer). Backs the active world up, deletes its files and lets Valheim create
 * a fresh world with a new seed on the next start.
 */
export function useRegenerateWorld(id: string) {
  const qc = useQueryClient()
  const { openJob } = useJobDrawer()
  return useMutation({
    mutationFn: ({ name, stopIfRunning }: { name: string; stopIfRunning: boolean }) =>
      api.post<{ job: Job }>(`/instances/${id}/worlds/${encodeURIComponent(name)}/regenerate`, {
        stop_if_running: stopIfRunning,
      }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: worldsKey(id) })
      qc.invalidateQueries({ queryKey: ['instances', id, 'backups'] })
      notifySuccess('World regeneration started')
      openJob(res.job.id)
    },
    onError: (err) => notifyError(err, 'Could not regenerate world'),
  })
}

/** POST /instances/{id}/worlds (multipart) → 202 Job, opened in the job drawer. */
export function useUploadWorlds(id: string) {
  const qc = useQueryClient()
  const { openJob } = useJobDrawer()
  return useMutation({
    mutationFn: ({ files, overwrite }: { files: File[]; overwrite: boolean }) => {
      const form = new FormData()
      files.forEach((f) => form.append('files', f))
      form.append('overwrite', String(overwrite))
      return api.upload<{ job: Job }>(`/instances/${id}/worlds`, form)
    },
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: worldsKey(id) })
      notifySuccess('World upload started')
      openJob(res.job.id)
    },
    onError: (err) => notifyError(err, 'Could not upload world'),
  })
}
