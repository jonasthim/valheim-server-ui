// Pure cfg <-> form value mapping for the Valheim UI Gameplay plugin's
// BepInEx config file, plus the raid-name merge for the Raids table. Kept
// free of React so it can be covered by `node --test`
// (gameplayConfig.test.ts) without a DOM. See
// /home/thim/.claude/plans/gameplay-plugin-2026-10.md Design for the cfg
// shape this mirrors.
import type { ConfigEntry, ConfigFileUpdate } from '../../api/types'

export const GAMEPLAY_CFG = 'se.jonasthim.valheimui.gameplay.cfg'
export const GAMEPLAY_MOD = { owner: 'jonasthim', name: 'valheimui_gameplay' } as const

export interface GameplayValues {
  autofeedEnabled: boolean
  signPrefix: string
  signRadius: number
  range: number
  intervalSeconds: number
  raidsEnabled: boolean
  disabledRaids: string[]
}

// The plan's own defaults (Design section), used whenever a key is missing
// from the cfg file (a version mismatch, or — in dev — a hand-written file
// that skipped a key).
const DEFAULTS: GameplayValues = {
  autofeedEnabled: false,
  signPrefix: 'feed',
  signRadius: 2,
  range: 20,
  intervalSeconds: 30,
  raidsEnabled: true,
  disabledRaids: [],
}

function findValue(entries: ConfigEntry[], section: string, key: string): string | undefined {
  return entries.find((e) => e.section === section && e.key === key)?.value
}

function parseDisabled(value: string | undefined): string[] {
  if (!value) return []
  const seen = new Set<string>()
  for (const raw of value.split(',')) {
    const name = raw.trim()
    if (name) seen.add(name)
  }
  return [...seen]
}

/** Maps `ConfigFile.entries` to typed form values. A missing key falls back
 * to the plan's default for that key, never to blank/zero. */
export function fromEntries(entries: ConfigEntry[]): GameplayValues {
  const autofeed = findValue(entries, 'Autofeed', 'Enabled')
  const raids = findValue(entries, 'Raids', 'Enabled')
  return {
    autofeedEnabled: autofeed === undefined ? DEFAULTS.autofeedEnabled : autofeed === 'true',
    signPrefix: findValue(entries, 'Autofeed', 'SignPrefix') ?? DEFAULTS.signPrefix,
    signRadius: Number(findValue(entries, 'Autofeed', 'SignRadius') ?? DEFAULTS.signRadius),
    range: Number(findValue(entries, 'Autofeed', 'Range') ?? DEFAULTS.range),
    intervalSeconds: Number(findValue(entries, 'Autofeed', 'IntervalSeconds') ?? DEFAULTS.intervalSeconds),
    raidsEnabled: raids === undefined ? DEFAULTS.raidsEnabled : raids === 'true',
    disabledRaids: parseDisabled(findValue(entries, 'Raids', 'Disabled')),
  }
}

/** Maps form values back to the seven cfg values the PUT endpoint writes
 * (`ConfigFileUpdate.values`); the server preserves comments and ordering. */
export function toUpdates(v: GameplayValues): ConfigFileUpdate {
  return {
    values: [
      { section: 'Autofeed', key: 'Enabled', value: String(v.autofeedEnabled) },
      { section: 'Autofeed', key: 'SignPrefix', value: v.signPrefix },
      { section: 'Autofeed', key: 'SignRadius', value: String(v.signRadius) },
      { section: 'Autofeed', key: 'Range', value: String(v.range) },
      { section: 'Autofeed', key: 'IntervalSeconds', value: String(v.intervalSeconds) },
      { section: 'Raids', key: 'Enabled', value: String(v.raidsEnabled) },
      { section: 'Raids', key: 'Disabled', value: v.disabledRaids.join(',') },
    ],
  }
}

/** Raid rows for the DataTable: catalog names (the live world's order) first,
 * then any disabled name the catalog doesn't know about — so a name saved
 * against an older/modded/offline world is never silently dropped — sorted. */
export function mergeRaidNames(known: string[], disabled: string[]): string[] {
  const extra = disabled.filter((name) => !known.includes(name)).sort((a, b) => a.localeCompare(b))
  return [...known, ...extra]
}
