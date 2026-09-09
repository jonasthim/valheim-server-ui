// Shared constants for the players tab (online panel, known players, list editors).
import type { ListKind } from '../../../api/types'

/** Valheim list entries are platform ids like `Steam_76561198000000000` or a bare numeric id. */
export const PLATFORM_ID_PATTERN = /^[A-Za-z0-9_]{1,64}$/

export const LIST_KINDS: ListKind[] = ['admin', 'banned', 'permitted']

export const LIST_KIND_LABELS: Record<ListKind, string> = {
  admin: 'Admins',
  permitted: 'Permitted',
  banned: 'Banned',
}

/** Verb used on the known-players row actions, e.g. "Add to admins". */
export const LIST_KIND_ACTION_LABELS: Record<ListKind, string> = {
  admin: 'Add to admins',
  permitted: 'Permit',
  banned: 'Ban',
}

export const COUNT_SOURCE_LABELS: Record<'a2s' | 'log' | 'none', string> = {
  a2s: 'via query port',
  log: 'from log',
  none: 'unknown',
}
