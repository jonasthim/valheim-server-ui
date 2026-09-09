// Small pure helpers shared by the mods components.
import type { ConfigEntry, Mod } from '../../api/types'

/** Thunderstore dependency strings look like `Owner-Name-1.2.3`; owner and
 * name never contain hyphens (Thunderstore package name rules), so the first
 * and last segments are unambiguous. */
export function parseDependency(dep: string): { owner: string; name: string; version: string } {
  const parts = dep.split('-')
  if (parts.length < 3) return { owner: parts[0] ?? dep, name: parts[1] ?? '', version: parts[2] ?? '' }
  return { owner: parts[0], name: parts.slice(1, -1).join('-'), version: parts[parts.length - 1] }
}

/** Find an installed mod matching a Thunderstore package's owner/name. */
export function findInstalledMod(mods: Mod[], owner: string, name: string): Mod | undefined {
  return mods.find((m) => m.owner === owner && m.name === name)
}

export const SORT_OPTIONS = [
  { value: 'rating', label: 'Top rated' },
  { value: 'downloads', label: 'Most downloaded' },
  { value: 'updated', label: 'Recently updated' },
  { value: 'name', label: 'Name' },
] as const

/** Whether a config entry should render as a numeric input, from its declared
 * type or the mere presence of a min/max range. */
export function isNumericEntry(entry: ConfigEntry): boolean {
  if (entry.range) return true
  const t = entry.type?.toLowerCase() ?? ''
  return ['int32', 'int64', 'byte', 'single', 'double', 'float', 'decimal'].some((n) => t.includes(n))
}

export function isBooleanEntry(entry: ConfigEntry): boolean {
  return (entry.type?.toLowerCase() ?? '') === 'boolean'
}

/** Stable key for one config entry within a file. */
export function entryKey(section: string, key: string): string {
  return `${section}::${key}`
}
