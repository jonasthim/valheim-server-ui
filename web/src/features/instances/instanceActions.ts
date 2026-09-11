// Mutation/query hooks for instance lifecycle actions, shared by the
// dashboard cards and the overview/config tabs. Every mutation notifies and
// invalidates the `['instances']` prefix (list + per-instance detail/status),
// matching what useEvents() already patches on `instance.status`/`job.updated`.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Instance, InstanceStatus, Job, UpdateInfo } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

function invalidateInstance(qc: QueryClient) {
  qc.invalidateQueries({ queryKey: ['instances'] })
}

/** GET /instances/{id}/status, polled every 10s as a fallback to the SSE `instance.status` event. */
export function useInstanceStatus(id: string) {
  return useQuery({
    queryKey: ['instances', id, 'status'],
    queryFn: () => api.get<{ status: InstanceStatus }>(`/instances/${id}/status`),
    enabled: !!id,
    refetchInterval: 10_000,
  })
}

export function useStartInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<{ status: InstanceStatus }>(`/instances/${id}/start`),
    onSuccess: (res) => {
      qc.setQueryData(['instances', id, 'status'], res)
      invalidateInstance(qc)
      notifySuccess('Instance starting')
    },
    onError: (err) => notifyError(err, 'Could not start instance'),
  })
}

export function useStopInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<{ status: InstanceStatus }>(`/instances/${id}/stop`),
    onSuccess: (res) => {
      qc.setQueryData(['instances', id, 'status'], res)
      invalidateInstance(qc)
      notifySuccess('Instance stopping')
    },
    onError: (err) => notifyError(err, 'Could not stop instance'),
  })
}

/**
 * Restart the instance. delaySeconds 0 (default) restarts immediately and
 * returns the new status; a positive delay enqueues a graceful restart job
 * that warns online players over a countdown, and returns { job }.
 */
export function useRestartInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (delaySeconds?: number) =>
      api.post<{ status?: InstanceStatus; job?: Job }>(
        `/instances/${id}/restart`,
        delaySeconds && delaySeconds > 0 ? { delay_seconds: delaySeconds } : undefined,
      ),
    onSuccess: (res) => {
      if (res.status) qc.setQueryData(['instances', id, 'status'], { status: res.status })
      invalidateInstance(qc)
      qc.invalidateQueries({ queryKey: ['jobs'] })
      notifySuccess(res.job ? 'Restart scheduled; players warned' : 'Instance restarting')
    },
    onError: (err) => notifyError(err, 'Could not restart instance'),
  })
}

/** POST /instances/{id}/install → 202 Job. Callers may pass their own onSuccess (e.g. to open the job drawer). */
export function useInstallInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<{ job: Job }>(`/instances/${id}/install`),
    onSuccess: () => {
      invalidateInstance(qc)
      notifySuccess('Install queued')
    },
    onError: (err) => notifyError(err, 'Could not queue install'),
  })
}

/** POST /instances/{id}/update → 202 Job. */
export function useUpdateInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (stopIfRunning: boolean) =>
      api.post<{ job: Job }>(`/instances/${id}/update`, { stop_if_running: stopIfRunning }),
    onSuccess: () => {
      invalidateInstance(qc)
      notifySuccess('Update queued')
    },
    onError: (err) => notifyError(err, 'Could not queue update'),
  })
}

/** POST /instances/{id}/update-check → 200 UpdateInfo (synchronous, not a job). */
export function useCheckForUpdate(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<UpdateInfo>(`/instances/${id}/update-check`),
    onSuccess: (info) => {
      invalidateInstance(qc)
      notifySuccess(
        info.update_available
          ? `Build ${info.latest_buildid} available`
          : `Up to date, build ${info.installed_buildid ?? info.latest_buildid}`,
      )
    },
    onError: (err) => notifyError(err, 'Could not check for updates'),
  })
}

/** PATCH /instances/{id} with only `autostart` (overview toggle). */
export function useSetAutostart(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (autostart: boolean) => api.patch<{ instance: Instance }>(`/instances/${id}`, { autostart }),
    onSuccess: () => {
      invalidateInstance(qc)
      notifySuccess('Autostart updated')
    },
    onError: (err) => notifyError(err, 'Could not update autostart'),
  })
}

/** DELETE /instances/{id}?delete_files=. Danger zone (admin only). */
export function useDeleteInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (deleteFiles: boolean) => api.del<void>(`/instances/${id}`, { delete_files: deleteFiles }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['instances', 'list'] })
      notifySuccess('Instance deleted')
    },
    onError: (err) => notifyError(err, 'Could not delete instance'),
  })
}
