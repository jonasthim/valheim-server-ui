// Query and mutation hooks for the Valheim UI Agent (the manager's own
// server plugin): its status per instance, admin commands and the
// install/update job. The `agent.status` SSE event patches the same query
// key (see events/useEvents.ts).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { AgentCommandRequest, AgentCommandResult, AgentInfo, Job } from '../../api/types'
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

const COMMAND_LABELS: Record<AgentCommandRequest['command'], string> = {
  save: 'World save requested',
  kick: 'Player kicked',
  ban: 'Player banned',
  unban: 'Player unbanned',
  broadcast: 'Message sent to all players',
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
