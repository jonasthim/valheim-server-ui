// Single SSE connection per app; dispatches events into React Query caches and
// to local subscribers (console log lines). See openapi.yaml → /events.
import { useEffect, useSyncExternalStore } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { API_BASE } from '../api/client'
import type { AgentInfo, InstanceStatus, Job, UpdateInfo } from '../api/types'

export type LogEvent = { instance_id: string; line: string }
export type JobLogEvent = { job_id: string; line: string }
export type PlayersEvent = { instance_id: string; online: { name: string; platform_id?: string }[] }
export type AgentStatusEvent = { instance_id: string; agent: AgentInfo }

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

// safeParse decodes one SSE frame, logging and skipping a malformed one rather
// than throwing inside the event handler (an uncaught throw there would break
// live updates silently).
function safeParse<T>(e: Event): T | null {
  try {
    return JSON.parse((e as MessageEvent).data) as T
  } catch (err) {
    console.warn('SSE: ignoring malformed event frame', err)
    return null
  }
}

// --- connection status store (so the shell can show "live updates offline") ---
let connected = false
const connListeners = new Set<() => void>()
function setConnected(v: boolean) {
  if (connected === v) return
  connected = v
  connListeners.forEach((fn) => fn())
}

/** Subscribe to whether the SSE stream is currently connected. */
export function useSSEConnected(): boolean {
  return useSyncExternalStore(
    (fn) => {
      connListeners.add(fn)
      return () => connListeners.delete(fn)
    },
    () => connected,
    () => false,
  )
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
      source.onopen = () => setConnected(true)
      source.addEventListener('instance.status', (e) => {
        const st = safeParse<InstanceStatus>(e)
        if (!st) return
        qc.setQueryData(['instances', st.instance_id, 'status'], { status: st })
        qc.invalidateQueries({ queryKey: ['instances', 'list'] })
        qc.invalidateQueries({ queryKey: ['instances', st.instance_id, 'detail'] })
      })
      source.addEventListener('job.updated', (e) => {
        const job = safeParse<Job>(e)
        if (!job) return
        qc.setQueryData(['jobs', job.id], { job })
        qc.invalidateQueries({ queryKey: ['jobs', 'list'] })
        if (job.status === 'succeeded' || job.status === 'failed' || job.status === 'cancelled') {
          if (job.instance_id) qc.invalidateQueries({ queryKey: ['instances', job.instance_id] })
          qc.invalidateQueries({ queryKey: ['instances', 'list'] })
        }
      })
      source.addEventListener('update.available', (e) => {
        const info = safeParse<UpdateInfo>(e)
        if (!info) return
        qc.invalidateQueries({ queryKey: ['instances', info.instance_id] })
        qc.invalidateQueries({ queryKey: ['system'] })
      })
      source.addEventListener('app.update_available', () => {
        // Payload is AppUpdateInfo, but SystemInfo (which embeds it) is the
        // cached shape ['system'] holds — just refetch it.
        qc.invalidateQueries({ queryKey: ['system'] })
      })
      source.addEventListener('instance.log', (e) => {
        const ev = safeParse<LogEvent>(e)
        if (!ev) return
        listeners['instance.log'].forEach((fn) => fn(ev))
      })
      source.addEventListener('job.log', (e) => {
        const ev = safeParse<JobLogEvent>(e)
        if (!ev) return
        listeners['job.log'].forEach((fn) => fn(ev))
      })
      source.addEventListener('agent.status', (e) => {
        const ev = safeParse<AgentStatusEvent>(e)
        if (!ev) return
        // The stream omits hidden players' positions; operators get them from
        // the next GET, which useAgent() also polls.
        qc.setQueryData<AgentInfo | undefined>(['instances', ev.instance_id, 'agent'], (prev) =>
          prev ? { ...prev, ...ev.agent, enabled: prev.enabled } : ev.agent,
        )
      })
      source.addEventListener('instance.players', (e) => {
        const ev = safeParse<PlayersEvent>(e)
        if (!ev) return
        listeners['instance.players'].forEach((fn) => fn(ev))
        qc.invalidateQueries({ queryKey: ['instances', ev.instance_id, 'players'] })
      })
      source.onerror = () => {
        // EventSource reconnects on its own; mark offline and refresh state
        // when it comes back (onopen flips it back to connected).
        setConnected(false)
        qc.invalidateQueries({ queryKey: ['instances'] })
      }
    }
    return () => {
      refs--
      if (refs === 0 && source) {
        source.close()
        source = null
        setConnected(false)
      }
    }
  }, [enabled, qc])
}
