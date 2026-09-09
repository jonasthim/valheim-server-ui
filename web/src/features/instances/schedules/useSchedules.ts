// Query hooks for the schedules tab.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../../api/client'
import type { Job, Schedule, ScheduleInput } from '../../../api/types'
import { useJobDrawer } from '../../jobs'
import { notifyError, notifySuccess } from '../../../lib/notify'

function schedulesKey(id: string) {
  return ['instances', id, 'schedules'] as const
}

/** GET /instances/{id}/schedules. */
export function useSchedules(id: string) {
  return useQuery({
    queryKey: schedulesKey(id),
    queryFn: () => api.get<{ schedules: Schedule[] }>(`/instances/${id}/schedules`).then((r) => r.schedules),
    enabled: !!id,
  })
}

/** POST /instances/{id}/schedules. */
export function useCreateSchedule(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ScheduleInput) => api.post<{ schedule: Schedule }>(`/instances/${id}/schedules`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: schedulesKey(id) })
      notifySuccess('Schedule created')
    },
    onError: (err) => notifyError(err, 'Could not create schedule'),
  })
}

/** PATCH /instances/{id}/schedules/{scheduleId} with the full ScheduleInput. */
export function useUpdateSchedule(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ scheduleId, input }: { scheduleId: number; input: ScheduleInput }) =>
      api.patch<{ schedule: Schedule }>(`/instances/${id}/schedules/${scheduleId}`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: schedulesKey(id) })
      notifySuccess('Schedule updated')
    },
    onError: (err) => notifyError(err, 'Could not update schedule'),
  })
}

/** DELETE /instances/{id}/schedules/{scheduleId}. */
export function useDeleteSchedule(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (scheduleId: number) => api.del<void>(`/instances/${id}/schedules/${scheduleId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: schedulesKey(id) })
      notifySuccess('Schedule deleted')
    },
    onError: (err) => notifyError(err, 'Could not delete schedule'),
  })
}

/** POST /instances/{id}/schedules/{scheduleId}/run → 202 Job, opened in the job drawer. */
export function useRunSchedule(id: string) {
  const qc = useQueryClient()
  const { openJob } = useJobDrawer()
  return useMutation({
    mutationFn: (scheduleId: number) => api.post<{ job: Job }>(`/instances/${id}/schedules/${scheduleId}/run`),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: schedulesKey(id) })
      notifySuccess('Schedule run started')
      openJob(res.job.id)
    },
    onError: (err) => notifyError(err, 'Could not run schedule'),
  })
}
