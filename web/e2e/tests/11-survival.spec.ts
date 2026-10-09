import { test, expect } from '@playwright/test'
import { login } from './helpers'

const at = '2026-10-09T10:00:00Z'
const moments = [
  { id: 1, instance_id: 'main', world_uid: 55, world_name: 'Midgard', kind: 'death', character_id: '991', player_name: 'Quin', day: 3, at, label: 'Death', biome: 'Meadows' },
  { id: 2, instance_id: 'main', world_uid: 55, world_name: 'Midgard', kind: 'death', character_id: '991', player_name: 'Quin', day: 9, at, label: 'Death', enemy: 'Deathsquito', enemy_level: 2, situation: 'EnemyHit', biome: 'Black Forest' },
  { id: 3, instance_id: 'main', world_uid: 55, world_name: 'Midgard', kind: 'progression', day: 10, at, label: 'Eikthyr progression unlocked' },
  { id: 4, instance_id: 'main', world_uid: 55, world_name: 'Midgard', kind: 'death', character_id: '992', player_name: 'Bjorn', day: 12, at, label: 'Death', biome: 'Meadows' },
]

test('survival tab shows deaths, milestones, player filter, and details', async ({ page }) => {
  await login(page)
  await page.route('**/api/v1/instances/main/survival?*', (route) => route.fulfill({ json: { worlds: [55], moments } }))
  await page.goto('/instances/main/survival')
  await expect(page.getByRole('heading', { name: 'Survival log' })).toBeVisible()
  await expect(page.getByRole('button', { name: /Progression Day 10 Eikthyr/ })).toBeVisible()
  await expect(page.getByText('Deaths recorded').locator('..')).toContainText('3')
  const plot = page.getByRole('group', { name: /Deaths by world day/ })
  await expect(plot.locator('circle')).toHaveCount(3)
  await plot.getByRole('button', { name: 'Day 10: Eikthyr progression unlocked' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toContainText('Eikthyr progression unlocked')
  await page.getByRole('button', { name: 'Close details' }).click()
  await page.getByRole('slider').first().focus()
  for (let i = 0; i < 4; i++) await page.keyboard.press('ArrowRight')
  await expect(plot.locator('circle')).toHaveCount(2)
  await page.getByRole('button', { name: 'Black Forest 1' }).click()
  await page.getByRole('switch', { name: 'Focus on matching days' }).click()
  await expect(plot.locator('circle')).toHaveCount(1)
  await expect(plot.locator('circle title')).toHaveText(/Death 2, day 9/)
  await page.getByRole('button', { name: /Quin Day 9/ }).click()
  await expect(page.getByRole('dialog')).toContainText('Black Forest')
  await expect(page.getByRole('dialog')).toContainText('Killer: Deathsquito')
  await expect(page.getByRole('dialog')).toContainText('Cause: Enemy Hit')
  await page.getByRole('button', { name: 'Close details' }).click()
  await page.getByRole('button', { name: 'Killer', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Deathsquito 1' })).toBeVisible()
  await page.getByRole('combobox', { name: 'Player' }).click()
  await page.getByRole('option', { name: 'Quin' }).click()
  await expect(page.getByText('Deaths recorded').locator('..')).toContainText('2')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Survival log' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true)
})
