// Query and mutation hooks for the Valheim UI Agent (the manager's own
// server plugin): its status per instance, admin commands and the
// install/update job. The `agent.status` SSE event patches the same query
// key (see events/useEvents.ts).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { AgentCatalog, AgentChat, AgentCommandRequest, AgentCommandResult, AgentInfo, Job } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

export function agentKey(id: string) {
  return ['instances', id, 'agent'] as const
}

/** GET /instances/{id}/agent, polled every 10 s as a fallback to SSE. */
export function useAgent(id: string) {
  return useQuery({
    queryKey: agentKey(id),
    queryFn: () => api.get<AgentInfo>(`/instances/${id}/agent`),
    enabled: !!id,
    refetchInterval: 10_000,
  })
}

const COMMAND_LABELS: Record<NonNullable<AgentCommandRequest['command']>, string> = {
  save: 'World save requested',
  kick: 'Player kicked',
  ban: 'Player banned',
  unban: 'Player unbanned',
  broadcast: 'Message sent to all players',
  time: 'World time changed',
  say: 'Message sent to chat',
  setkey: 'Global key set',
  removekey: 'Global key removed',
  event: 'Event started',
  eventstop: 'Event stopped',
}

/** POST /instances/{id}/agent/commands. */
export function useAgentCommand(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: AgentCommandRequest) => api.post<AgentCommandResult>(`/instances/${id}/agent/commands`, req),
    onSuccess: (res, req) => {
      if (res.ok) notifySuccess(res.message || COMMAND_LABELS[req.command])
      else notifyError(new Error(res.message), 'The server refused the command')
      void qc.invalidateQueries({ queryKey: agentKey(id) })
      if (req.command === 'ban' || req.command === 'unban') {
        void qc.invalidateQueries({ queryKey: ['instances', id, 'lists'] })
      }
    },
    onError: (err) => notifyError(err, 'Command failed'),
  })
}

/** GET /instances/{id}/agent/catalog: the key and event pickers. Cached; the
 * catalog changes rarely, so refetch only on demand. */
export function useAgentCatalog(id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['instances', id, 'agent', 'catalog'],
    queryFn: () => api.get<AgentCatalog>(`/instances/${id}/agent/catalog`),
    enabled: !!id && enabled,
    staleTime: 60_000,
  })
}

/** GET /instances/{id}/agent/chat, polled while the panel is open. */
export function useAgentChat(id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['instances', id, 'agent', 'chat'],
    queryFn: () => api.get<AgentChat>(`/instances/${id}/agent/chat?limit=100`),
    enabled: !!id && enabled,
    refetchInterval: enabled ? 5_000 : false,
  })
}

/** POST /instances/{id}/agent/install (a job). */
export function useInstallAgent(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { stop_if_running: boolean }) => api.post<{ job: Job }>(`/instances/${id}/agent/install`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: agentKey(id) })
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      void qc.invalidateQueries({ queryKey: ['jobs'] })
      notifySuccess('Agent install queued')
    },
    onError: (err) => notifyError(err, 'Could not install the agent'),
  })
}

/** Formats the world clock (day fraction 0..1) as HH:MM. */
export function fmtWorldTime(fraction: number): string {
  const minutes = Math.round(((fraction % 1) + 1) % 1 * 24 * 60)
  const h = Math.floor(minutes / 60) % 24
  const m = minutes % 60
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
}
