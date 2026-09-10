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
