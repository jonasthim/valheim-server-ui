// Hooks for the account page's session list (F-2.7): read the caller's own
// sessions, revoke one, or sign out everywhere else. Kept free of JSX so this
// stays a plain hooks module (oxlint react/only-export-components) — confirm
// dialogs that need JSX live in AccountPage.tsx.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { SessionInfo } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

const SESSIONS_KEY = ['auth', 'sessions']

/** GET /auth/sessions, polled every 30s so a session revoked elsewhere (or expiring) disappears here too. */
export function useSessions() {
  return useQuery({
    queryKey: SESSIONS_KEY,
    queryFn: () => api.get<{ sessions: SessionInfo[] }>('/auth/sessions'),
    refetchInterval: 30_000,
  })
}

/** DELETE /auth/sessions/{sessionId}: revoke one of the caller's own sessions. */
export function useRevokeSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (sessionId: string) => api.del<void>(`/auth/sessions/${sessionId}`),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SESSIONS_KEY })
      notifySuccess('Session revoked')
    },
    onError: (err) => notifyError(err, 'Could not revoke session'),
  })
}

/** POST /auth/sessions/revoke-others: "sign out everywhere else". */
export function useRevokeOtherSessions() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<void>('/auth/sessions/revoke-others'),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SESSIONS_KEY })
      notifySuccess('Other sessions signed out')
    },
    onError: (err) => notifyError(err, 'Could not sign out other sessions'),
  })
}
