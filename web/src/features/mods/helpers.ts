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

/** BepInEx ServerSync mods tag each setting's description with
 * `[Synced with Server]` or `[Not Synced with Server]`. A not-synced key is
 * client-side: on a dedicated server, setting it here is never pushed to
 * connected players and may have no server-side effect. This also covers the
 * one-shot "action" keys (e.g. AzuExtendedPlayerInventory's `Apply Preset`)
 * that only run from the in-game configuration manager and reset themselves
 * to their default on the next load. */
export function isClientSideSetting(entry: ConfigEntry): boolean {
  return /\[not synced with server\]/i.test(entry.description ?? '')
}

/** KeyboardShortcut settings store a "+"-separated key combo (e.g. "O + LeftAlt")
 * and their acceptable-values list is a huge KeyCode enum with duplicate names
 * (Unity aliases LeftMeta/LeftCommand to one value). Render them as free text,
 * not a dropdown. */
export function isKeybindEntry(entry: ConfigEntry): boolean {
  return (entry.type ?? '').toLowerCase().includes('keyboardshortcut')
}

/** Best-effort match of a mod to the config files it likely owns. Config files
 * are named by the plugin's BepInEx GUID (e.g. `Azumatt.AzuCraftyBoxes.cfg`,
 * `MidnightsFX.AchievementEnabler.cfg`), not the Thunderstore owner-name, so we
 * match a file whose stem equals `<owner>.<name>` or contains `<name>` as a
 * dot-delimited segment. The user confirms the result, so a loose match is fine. */
export function matchModConfigs(mod: { owner: string; name: string }, files: { name: string }[]): string[] {
  const name = mod.name.toLowerCase()
  const ownerDotName = `${mod.owner}.${mod.name}`.toLowerCase()
  return files
    .filter((f) => {
      const stem = f.name.replace(/\.cfg$/i, '').toLowerCase()
      return stem === ownerDotName || stem === name || stem.split('.').includes(name)
    })
    .map((f) => f.name)
}
