// Single SSE connection per app; dispatches events into React Query caches and
// to local subscribers (console log lines). See openapi.yaml → /events.
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { API_BASE } from '../api/client'
import type { InstanceStatus, Job, UpdateInfo } from '../api/types'

export type LogEvent = { instance_id: string; line: string }
export type JobLogEvent = { job_id: string; line: string }
export type PlayersEvent = { instance_id: string; online: { name: string; platform_id?: string }[] }

type Listener<T> = (e: T) => void
const listeners = {
  'instance.log': new Set<Listener<LogEvent>>(),
  'job.log': new Set<Listener<JobLogEvent>>(),
  'instance.players': new Set<Listener<PlayersEvent>>(),
}

export function onEvent<K extends keyof typeof listeners>(
  name: K,
  fn: (typeof listeners)[K] extends Set<Listener<infer T>> ? Listener<T> : never,
): () => void {
  const set = listeners[name] as Set<unknown>
  set.add(fn)
  return () => {
    set.delete(fn)
  }
}

let source: EventSource | null = null
let refs = 0

/** Mount once in the authenticated app shell. */
export function useEvents(enabled: boolean) {
  const qc = useQueryClient()
  useEffect(() => {
    if (!enabled) return
    refs++
    if (!source) {
      source = new EventSource(`${API_BASE}/events`, { withCredentials: true })
      source.addEventListener('instance.status', (e) => {
        const st = JSON.parse((e as MessageEvent).data) as InstanceStatus
        qc.setQueryData(['instances', st.instance_id, 'status'], { status: st })
        qc.invalidateQueries({ queryKey: ['instances', 'list'] })
        qc.invalidateQueries({ queryKey: ['instances', st.instance_id, 'detail'] })
      })
      source.addEventListener('job.updated', (e) => {
        const job = JSON.parse((e as MessageEvent).data) as Job
        qc.setQueryData(['jobs', job.id], { job })
        qc.invalidateQueries({ queryKey: ['jobs', 'list'] })
        if (job.status === 'succeeded' || job.status === 'failed' || job.status === 'cancelled') {
          if (job.instance_id) qc.invalidateQueries({ queryKey: ['instances', job.instance_id] })
          qc.invalidateQueries({ queryKey: ['instances', 'list'] })
        }
      })
      source.addEventListener('update.available', (e) => {
        const info = JSON.parse((e as MessageEvent).data) as UpdateInfo
        qc.invalidateQueries({ queryKey: ['instances', info.instance_id] })
        qc.invalidateQueries({ queryKey: ['system'] })
      })
      source.addEventListener('instance.log', (e) => {
        const ev = JSON.parse((e as MessageEvent).data) as LogEvent
        listeners['instance.log'].forEach((fn) => fn(ev))
      })
      source.addEventListener('job.log', (e) => {
        const ev = JSON.parse((e as MessageEvent).data) as JobLogEvent
        listeners['job.log'].forEach((fn) => fn(ev))
      })
      source.addEventListener('instance.players', (e) => {
        const ev = JSON.parse((e as MessageEvent).data) as PlayersEvent
        listeners['instance.players'].forEach((fn) => fn(ev))
        qc.invalidateQueries({ queryKey: ['instances', ev.instance_id, 'players'] })
      })
      source.onerror = () => {
        // EventSource reconnects on its own; refresh state when it comes back.
        qc.invalidateQueries({ queryKey: ['instances'] })
      }
    }
    return () => {
      refs--
      if (refs === 0 && source) {
        source.close()
        source = null
      }
    }
  }, [enabled, qc])
}
