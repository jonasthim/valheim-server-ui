import { useContext } from 'react'
import { AuthCtx, type AuthState } from './context'

export function useAuth(): AuthState {
  const v = useContext(AuthCtx)
  if (!v) throw new Error('useAuth outside AuthProvider')
  return v
}
