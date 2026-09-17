// Hooks for the account page's personal API tokens (F-2.5): list, create and
// revoke. Kept free of JSX so this stays a plain hooks module (oxlint
// react/only-export-components) — the create-token modal lives in
// AccountPage.tsx.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { APIToken } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

const TOKENS_KEY = ['auth', 'tokens']

/** GET /auth/tokens. */
export function useTokens() {
  return useQuery({
    queryKey: TOKENS_KEY,
    queryFn: () => api.get<{ tokens: APIToken[] }>('/auth/tokens'),
  })
}

/**
 * POST /auth/tokens. The mutation's `data` on success is the create response
 * (`{ token, secret }`) so the page can show the secret exactly once.
 */
export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; expires_in_days: number }) =>
      api.post<{ token: APIToken; secret: string }>('/auth/tokens', input),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: TOKENS_KEY })
      notifySuccess('Token created')
    },
    onError: (err) => notifyError(err, 'Could not create token'),
  })
}

/** DELETE /auth/tokens/{id}: revoke one of the caller's own API tokens. */
export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>(`/auth/tokens/${id}`),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: TOKENS_KEY })
      notifySuccess('Token revoked')
    },
    onError: (err) => notifyError(err, 'Could not revoke token'),
  })
}
