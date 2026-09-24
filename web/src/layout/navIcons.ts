// Icons for the sidebar's PAGES entries (web/src/lib/routes.ts), keyed by
// route path. Same Tabler icons as the pre-B-1 NAV_GROUPS in Shell.tsx.
import { IconHistory, IconLayoutDashboard, IconListCheck, IconSettings, IconUserCircle, IconUsers } from '@tabler/icons-react'

export const NAV_ICONS: Record<string, typeof IconLayoutDashboard> = {
  '/': IconLayoutDashboard,
  '/jobs': IconListCheck,
  '/users': IconUsers,
  '/settings': IconSettings,
  '/audit': IconHistory,
  '/account': IconUserCircle,
}
