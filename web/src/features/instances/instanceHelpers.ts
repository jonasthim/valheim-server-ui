// Pure helpers shared by the dashboard cards, overview, console and config
// tabs. Kept free of hooks/components so it can be imported anywhere without
// pulling in React Query or Mantine.
import type { InstanceState } from '../../api/types'

const STATE_COLORS: Record<InstanceState, string> = {
  not_installed: 'gray',
  stopped: 'gray',
  starting: 'yellow',
  stopping: 'yellow',
  running: 'green',
  failed: 'red',
}

export function stateColor(state: InstanceState | undefined): string {
  return state ? STATE_COLORS[state] : 'gray'
}

const STATE_LABELS: Record<InstanceState, string> = {
  not_installed: 'Not installed',
  stopped: 'Stopped',
  starting: 'Starting',
  stopping: 'Stopping',
  running: 'Running',
  failed: 'Failed',
}

export function stateLabel(state: InstanceState | undefined): string {
  return state ? STATE_LABELS[state] : 'Unknown'
}

/** True while the manager is transitioning the process (show a spinner, disable actions). */
export function isTransitioning(state: InstanceState | undefined): boolean {
  return state === 'starting' || state === 'stopping'
}

export function canStart(state: InstanceState | undefined): boolean {
  return state === 'stopped' || state === 'failed'
}

export function canStop(state: InstanceState | undefined): boolean {
  return state === 'running'
}

export function canRestart(state: InstanceState | undefined): boolean {
  return state === 'running'
}

export function canInstall(state: InstanceState | undefined): boolean {
  return state === 'not_installed'
}

/** Derive a valid instance id slug from a free-text display name. */
// Instance ids are ASCII. Letters with diacritics are transliterated
// ("Fnaskhörna" -> "fnaskhorna", "Ångström" -> "angstrom") instead of being
// dropped; a few Nordic letters have no combining form and are mapped by hand.
const TRANSLITERATE: Record<string, string> = { ø: 'o', æ: 'ae', ð: 'd', þ: 'th', ß: 'ss', œ: 'oe', ł: 'l' }

export function slugify(name: string): string {
  const slug = name
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[øæðþßœł]/g, (c) => TRANSLITERATE[c] ?? c)
    .trim()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 32)
  return slug.replace(/^[-]+/, '')
}

export const INSTANCE_ID_PATTERN = /^[a-z0-9][a-z0-9-]{0,31}$/
export const WORLD_NAME_PATTERN = /^[A-Za-z0-9_\- ]{1,32}$/

/** Console lines worth calling out (join/leave/ready/save) per ARCHITECTURE §8. */
const LOG_HIGHLIGHTS: { pattern: RegExp; color: string }[] = [
  { pattern: /Game server connected/i, color: 'blue' },
  { pattern: /Got character ZDOID/i, color: 'green' },
  { pattern: /Closing socket|wrong password/i, color: 'orange' },
  { pattern: /World saved/i, color: 'teal' },
]

/** Returns a highlight color for a console line, or undefined for a plain line. */
export function highlightColor(line: string): string | undefined {
  for (const { pattern, color } of LOG_HIGHLIGHTS) {
    if (pattern.test(line)) return color
  }
  return undefined
}

const CONFIG_FIELD_KEYS = new Set([
  'name',
  'world',
  'password',
  'port',
  'public',
  'crossplay',
  'preset',
  'modifiers',
  'setkeys',
  'save_interval_sec',
  'game_backups',
  'game_backup_short_sec',
  'game_backup_long_sec',
  'extra_args',
  'bepinex_enabled',
  'backup_keep_last',
  'backup_keep_days',
  'backup_before_update',
])

/**
 * The API reports InstanceConfig validation errors relative to `config`
 * (e.g. "port", "modifiers.combat", "setkeys"); the create/config forms nest
 * those fields under `config.*`. Top-level CreateInstanceRequest/
 * UpdateInstanceRequest fields ("id", "autostart") pass through unchanged.
 */
export function mapConfigFieldErrors(fields: Record<string, string>): Record<string, string> {
  const mapped: Record<string, string> = {}
  for (const [field, message] of Object.entries(fields)) {
    // The API reports the display name as "display_name" and config fields
    // relative to config (e.g. "name" is the in-game server name).
    if (field === 'display_name') {
      mapped.name = message
      continue
    }
    const head = field.split('.')[0]
    mapped[CONFIG_FIELD_KEYS.has(head) ? `config.${field}` : field] = message
  }
  return mapped
}
