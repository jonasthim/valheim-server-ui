import {
  IconAdjustments,
  IconArchive,
  IconCalendar,
  IconLayoutDashboard,
  IconMap2,
  IconPuzzle,
  IconTerminal2,
  IconUsers,
  IconWorld,
} from '@tabler/icons-react'

export const INSTANCE_TABS = [
  'overview',
  'console',
  'map',
  'config',
  'players',
  'worlds',
  'backups',
  'mods',
  'schedules',
] as const
export type InstanceTab = (typeof INSTANCE_TABS)[number]

export const INSTANCE_TAB_LABELS: Record<InstanceTab, string> = {
  overview: 'Overview',
  console: 'Console',
  map: 'Map',
  config: 'Config',
  players: 'Players',
  worlds: 'Worlds',
  backups: 'Backups',
  mods: 'Mods',
  schedules: 'Schedules',
}

export const INSTANCE_TAB_ICONS: Record<InstanceTab, typeof IconLayoutDashboard> = {
  overview: IconLayoutDashboard,
  console: IconTerminal2,
  map: IconMap2,
  config: IconAdjustments,
  players: IconUsers,
  worlds: IconWorld,
  backups: IconArchive,
  mods: IconPuzzle,
  schedules: IconCalendar,
}
