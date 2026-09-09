export const INSTANCE_TABS = [
  'overview',
  'console',
  'config',
  'players',
  'worlds',
  'backups',
  'mods',
  'schedules',
] as const
export type InstanceTab = (typeof INSTANCE_TABS)[number]
