// Query hooks for jobs. Kept separate from components so the barrel export
// doesn't trip oxlint's react/only-export-components rule.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Job, JobStatus } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

export interface JobFilters {
  instance?: string
  status?: JobStatus
  limit?: number
}

/**
 * GET /jobs?instance=&status=&limit=, polled every `refetchIntervalMs` (default
 * 10s, matching JobsPage) as a fallback to the SSE `job.updated` invalidation
 * already wired in useEvents (it invalidates the `['jobs','list']` prefix, so
 * every useJobs call benefits regardless of its own filters).
 */
export function useJobs(filters: JobFilters = {}, refetchIntervalMs = 10_000) {
  return useQuery({
    queryKey: ['jobs', 'list', filters],
    queryFn: () =>
      api
        .get<{ jobs: Job[] }>('/jobs', { instance: filters.instance, status: filters.status, limit: filters.limit ?? 100 })
        .then((r) => r.jobs),
    refetchInterval: refetchIntervalMs,
  })
}

/**
 * GET /jobs/{id}. Query key and response shape (`{ job }`) match what
 * useEvents' `job.updated` handler patches into the cache, so live updates
 * apply without a refetch.
 */
export function useJob(id: string | undefined) {
  return useQuery({
    queryKey: ['jobs', id],
    queryFn: () => api.get<{ job: Job }>(`/jobs/${id}`),
    enabled: !!id,
  })
}

/** POST /jobs/{id}/cancel, patching the same cache entries `job.updated` events patch. */
export function useCancelJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (jobId: string) => api.post<{ job: Job }>(`/jobs/${jobId}/cancel`),
    onSuccess: (res, jobId) => {
      qc.setQueryData(['jobs', jobId], res)
      qc.invalidateQueries({ queryKey: ['jobs', 'list'] })
      notifySuccess('Cancellation requested')
    },
    onError: (err) => notifyError(err, 'Could not cancel job'),
  })
}
