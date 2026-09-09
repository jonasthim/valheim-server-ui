import type { ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../api/client'
import type { AuthStatus, User } from '../api/types'
import { AuthCtx, type AuthState } from './context'
import { roleAtLeast } from '../api/types'

export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const statusQ = useQuery({
    queryKey: ['auth', 'status'],
    queryFn: () => api.get<AuthStatus>('/auth/status'),
    staleTime: 60_000,
  })
  const meQ = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: async () => {
      try {
        const r = await api.get<{ user: User }>('/auth/me')
        return r.user
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
    retry: false,
    staleTime: 30_000,
  })

  const value: AuthState = {
    user: meQ.data ?? null,
    status: statusQ.data ?? null,
    loading: statusQ.isLoading || meQ.isLoading,
    hasRole: (min) => roleAtLeast(meQ.data?.role, min),
    refresh: async () => {
      await qc.invalidateQueries({ queryKey: ['auth'] })
    },
    logout: async () => {
      await api.post('/auth/logout')
      qc.clear()
      await qc.invalidateQueries({ queryKey: ['auth'] })
    },
  }
  return <AuthCtx.Provider value={value}>{children}</AuthCtx.Provider>
}

