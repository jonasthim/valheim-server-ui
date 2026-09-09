// Hooks for manager self-upgrade (WP-32). Kept free of JSX so this stays a
// plain hooks module (oxlint react/only-export-components) — confirm dialogs
// that need JSX live in the components that call these hooks.
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { AppUpdateInfo, Job, SystemInfo } from '../../api/types'
import { useJob } from '../jobs'
import { notifyError, notifySuccess } from '../../lib/notify'

/** Shown in the "confirm upgrade" dialog wherever it appears (banner + settings). */
export const UPGRADE_EXPLANATION =
  'The manager downloads and verifies the release, then replaces itself and restarts within a few seconds. ' +
  'Game servers keep running throughout; this page will reconnect automatically once the manager is back.'

/** GET /system, polled every 60s as a fallback to the SSE `app.update_available` invalidation. */
export function useSystemInfo() {
  return useQuery({
    queryKey: ['system'],
    queryFn: () => api.get<SystemInfo>('/system'),
    refetchInterval: 60_000,
  })
}

/** POST /system/update-check → 200 AppUpdateInfo (synchronous, not a job). */
export function useCheckAppUpdate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<AppUpdateInfo>('/system/update-check'),
    onSuccess: (info) => {
      qc.setQueryData<SystemInfo | undefined>(['system'], (prev) => (prev ? { ...prev, app_update: info } : prev))
      qc.invalidateQueries({ queryKey: ['system'] })
      notifySuccess(
        info.update_available ? `Version ${info.latest_version} available` : `Up to date (v${info.current_version})`,
      )
    },
    onError: (err) => notifyError(err, 'Could not check for updates'),
  })
}

/** POST /system/upgrade → 202 Job. Callers open the job drawer and start `useManagerRestartWatch` themselves. */
export function useUpgradeApp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (version?: string) => api.post<{ job: Job }>('/system/upgrade', version ? { version } : undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['system'] })
      notifySuccess('Upgrade queued')
    },
    onError: (err) => notifyError(err, 'Could not queue upgrade'),
  })
}

/**
 * After the given `self_upgrade` job reaches `succeeded`, polls GET /system
 * every 2s until it responds with a different `version` than the manager was
 * running before the upgrade (or 60s pass), then notifies and reloads the
 * page so the new frontend bundle loads. Pass `undefined` to stay idle.
 */
export function useManagerRestartWatch(jobId: string | undefined): { restarting: boolean } {
  const jobQuery = useJob(jobId)
  const qc = useQueryClient()
  const [restarting, setRestarting] = useState(false)
  const watchedJobId = useRef<string | undefined>(undefined)

  useEffect(() => {
    const job = jobQuery.data?.job
    if (!job || job.type !== 'self_upgrade' || job.status !== 'succeeded') return
    if (watchedJobId.current === job.id) return
    watchedJobId.current = job.id

    const preVersion = qc.getQueryData<SystemInfo>(['system'])?.version
    let cancelled = false
    setRestarting(true)

    function finish(version?: string) {
      if (cancelled) return
      cancelled = true
      setRestarting(false)
      notifySuccess(version ? `Upgraded to v${version}` : 'Manager restarted', 'Upgrade complete')
      window.location.reload()
    }

    const deadline = Date.now() + 60_000

    async function poll() {
      while (!cancelled) {
        try {
          const info = await api.get<SystemInfo>('/system')
          if (!preVersion || info.version !== preVersion) {
            qc.setQueryData(['system'], info)
            finish(info.version)
            return
          }
        } catch {
          // Manager is mid-restart; keep polling until it answers again.
        }
        if (Date.now() >= deadline) {
          finish()
          return
        }
        await new Promise((resolve) => setTimeout(resolve, 2000))
      }
    }
    void poll()

    return () => {
      cancelled = true
    }
  }, [jobQuery.data, qc])

  return { restarting }
}
