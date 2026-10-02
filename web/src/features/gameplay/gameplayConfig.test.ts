// Covers gameplayConfig.ts's pure functions, which the e2e suite can't
// reach (spec 09's only reachable state is "not installed"; see
// /home/thim/.claude/plans/gameplay-plugin-2026-10.md TASK U-1). Run with:
//   node --experimental-strip-types --test web/src/features/gameplay/gameplayConfig.test.ts
//
// Not part of the Vite app bundle (tsconfig.app.json's "types" excludes
// "node"); the reference below gives tsc the node:test/node:assert ambient
// types for this file only, without touching the shared tsconfig.
/// <reference types="node" />
import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { ConfigEntry } from '../../api/types.ts'
import { fromEntries, toUpdates, mergeRaidNames, type GameplayValues } from './gameplayConfig.ts'

function entry(section: string, key: string, value: string): ConfigEntry {
  return { section, key, value }
}

test('fromEntries falls back to the plan defaults when entries are missing', () => {
  const values = fromEntries([])
  assert.deepEqual(values, {
    autofeedEnabled: false,
    signPrefix: 'feed',
    signRadius: 2,
    range: 20,
    intervalSeconds: 30,
    raidsEnabled: true,
    disabledRaids: [],
  })
})

test('fromEntries reads all seven keys, trims and dedupes Disabled', () => {
  const values = fromEntries([
    entry('Autofeed', 'Enabled', 'true'),
    entry('Autofeed', 'SignPrefix', 'storage'),
    entry('Autofeed', 'SignRadius', '3.5'),
    entry('Autofeed', 'Range', '35'),
    entry('Autofeed', 'IntervalSeconds', '45'),
    entry('Raids', 'Enabled', 'false'),
    entry('Raids', 'Disabled', ' army_eikthyr, wolves ,army_eikthyr,'),
  ])
  assert.deepEqual(values, {
    autofeedEnabled: true,
    signPrefix: 'storage',
    signRadius: 3.5,
    range: 35,
    intervalSeconds: 45,
    raidsEnabled: false,
    disabledRaids: ['army_eikthyr', 'wolves'],
  })
})

test('toUpdates formats all seven values as plain strings', () => {
  const values: GameplayValues = {
    autofeedEnabled: true,
    signPrefix: 'feed',
    signRadius: 2.5,
    range: 20,
    intervalSeconds: 30,
    raidsEnabled: false,
    disabledRaids: ['wolves', 'bats'],
  }
  assert.deepEqual(toUpdates(values), {
    values: [
      { section: 'Autofeed', key: 'Enabled', value: 'true' },
      { section: 'Autofeed', key: 'SignPrefix', value: 'feed' },
      { section: 'Autofeed', key: 'SignRadius', value: '2.5' },
      { section: 'Autofeed', key: 'Range', value: '20' },
      { section: 'Autofeed', key: 'IntervalSeconds', value: '30' },
      { section: 'Raids', key: 'Enabled', value: 'false' },
      { section: 'Raids', key: 'Disabled', value: 'wolves,bats' },
    ],
  })
})

test('toUpdates joins an empty Disabled list as an empty string', () => {
  const values: GameplayValues = {
    autofeedEnabled: false,
    signPrefix: '',
    signRadius: 2,
    range: 20,
    intervalSeconds: 30,
    raidsEnabled: true,
    disabledRaids: [],
  }
  assert.equal(toUpdates(values).values?.find((v) => v.key === 'Disabled')?.value, '')
})

test('mergeRaidNames keeps the catalog order and appends unknown disabled names sorted', () => {
  const known = ['wolves', 'army_eikthyr', 'skeletons']
  const disabled = ['skeletons', 'zzz_modded_raid', 'army_custom']
  assert.deepEqual(mergeRaidNames(known, disabled), ['wolves', 'army_eikthyr', 'skeletons', 'army_custom', 'zzz_modded_raid'])
})

test('mergeRaidNames returns just the known names when nothing extra is disabled', () => {
  assert.deepEqual(mergeRaidNames(['wolves', 'bats'], ['wolves']), ['wolves', 'bats'])
})

test('mergeRaidNames returns an empty list when both inputs are empty', () => {
  assert.deepEqual(mergeRaidNames([], []), [])
})
