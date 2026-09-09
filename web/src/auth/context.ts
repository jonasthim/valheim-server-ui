import { createContext } from 'react'
import type { AuthStatus, Role, User } from '../api/types'

export interface AuthState {
  user: User | null
  status: AuthStatus | null
  loading: boolean
  hasRole: (min: Role) => boolean
  refresh: () => Promise<void>
  logout: () => Promise<void>
}

export const AuthCtx = createContext<AuthState | null>(null)

