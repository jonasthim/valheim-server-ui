/// <reference types="node" />
// Covers matchModConfigs (config file -> mod matching for the uninstall
// dialog). Run with:
//   node --experimental-strip-types --test web/src/features/mods/helpers.test.ts
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { matchModConfigs } from './helpers.ts'

const mod = { owner: 'Azumatt', name: 'AzuAutoStore' }

test('cfg and yml of the same mod both match', () => {
  const files = [{ name: 'Azumatt.AzuAutoStore.cfg' }, { name: 'Azumatt.AzuAutoStore.yml' }]
  assert.deepEqual(matchModConfigs(mod, files), ['Azumatt.AzuAutoStore.cfg', 'Azumatt.AzuAutoStore.yml'])
})

test('yaml and json extensions match', () => {
  const files = [{ name: 'Azumatt.AzuAutoStore.yaml' }, { name: 'AzuAutoStore.json' }]
  assert.deepEqual(matchModConfigs(mod, files), ['Azumatt.AzuAutoStore.yaml', 'AzuAutoStore.json'])
})

test('an unrelated file does not match', () => {
  assert.deepEqual(matchModConfigs(mod, [{ name: 'Other.Thing.yml' }, { name: 'Other.Thing.cfg' }]), [])
})

test('matching is case-insensitive', () => {
  assert.deepEqual(matchModConfigs(mod, [{ name: 'AZUMATT.AZUAUTOSTORE.YML' }]), ['AZUMATT.AZUAUTOSTORE.YML'])
})
