import type { Role } from '../../api/types'

export const ROLE_OPTIONS: { value: Role; label: string }[] = [
  { value: 'viewer', label: 'Viewer' },
  { value: 'operator', label: 'Operator' },
  { value: 'admin', label: 'Admin' },
]

/** Tint per role for the Users table badge: admin ember, operator frost, viewer gray. */
export const ROLE_COLORS: Record<Role, string> = {
  admin: 'ember',
  operator: 'frost',
  viewer: 'gray',
}
