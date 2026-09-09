import type { Role } from '../../api/types'

export const ROLE_MAP_OPTIONS: { value: Role; label: string }[] = [
  { value: 'viewer', label: 'Viewer' },
  { value: 'operator', label: 'Operator' },
  { value: 'admin', label: 'Admin' },
]

export const DEFAULT_ROLE_OPTIONS: { value: string; label: string }[] = [
  ...ROLE_MAP_OPTIONS,
  { value: 'deny', label: 'Deny (block login)' },
]
