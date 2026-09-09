import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { Center, Loader } from '@mantine/core'
import { useAuth } from './useAuth'
import type { Role } from '../api/types'

/** Redirects to /setup while no users exist, to /login when unauthenticated. */
export function RequireAuth() {
  const { user, status, loading } = useAuth()
  const loc = useLocation()
  if (loading) {
    return (
      <Center h="100vh">
        <Loader />
      </Center>
    )
  }
  if (status?.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />
  return <Outlet />
}

export function RequireRole({ min }: { min: Role }) {
  const { hasRole } = useAuth()
  if (!hasRole(min)) return <Navigate to="/" replace />
  return <Outlet />
}
