// Pure route helpers (no React) shared by the sidebar nav and the top-bar
// breadcrumbs, so both read the same page list and the same instance-route
// parsing instead of duplicating path logic.
import type { Role } from '../api/types'
import { INSTANCE_TAB_LABELS, isInstanceTab } from '../features/instances/tabs'

export interface PageDef {
  to: string
  label: string
  min: Role
  group: 'Servers' | 'Administration'
}

// Same labels/order/roles as the pre-B-1 NAV_GROUPS in Shell.tsx.
export const PAGES: PageDef[] = [
  { to: '/', label: 'Dashboard', min: 'viewer', group: 'Servers' },
  { to: '/jobs', label: 'Jobs', min: 'viewer', group: 'Servers' },
  { to: '/users', label: 'Users', min: 'admin', group: 'Administration' },
  { to: '/settings', label: 'Settings', min: 'admin', group: 'Administration' },
  { to: '/audit', label: 'Audit log', min: 'admin', group: 'Administration' },
  { to: '/account', label: 'Account', min: 'viewer', group: 'Administration' },
]

/** Parses `/instances/:id/:tab`; `null` for anything else, including the
 * `/instances/new` create-instance route (id `new` is not a real instance). */
export function parseInstanceRoute(pathname: string): { id: string; tab: string } | null {
  const match = /^\/instances\/([^/]+)\/([^/]+)\/?$/.exec(pathname)
  if (!match) return null
  const id = decodeURIComponent(match[1])
  if (id === 'new') return null
  return { id, tab: match[2] }
}

export interface Crumb {
  label: string
  to?: string
}

export function deriveBreadcrumbs(pathname: string, instanceName: (id: string) => string | undefined): Crumb[] {
  const exact = PAGES.find((p) => p.to === pathname)
  if (exact) return [{ label: exact.label }]

  if (pathname === '/instances/new') {
    return [{ label: 'Dashboard', to: '/' }, { label: 'New instance' }]
  }

  const inst = parseInstanceRoute(pathname)
  if (inst) {
    return [
      { label: 'Dashboard', to: '/' },
      { label: instanceName(inst.id) ?? inst.id, to: `/instances/${inst.id}/overview` },
      { label: isInstanceTab(inst.tab) ? INSTANCE_TAB_LABELS[inst.tab] : inst.tab },
    ]
  }

  return []
}
