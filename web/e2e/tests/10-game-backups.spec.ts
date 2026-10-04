import { test, expect } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { dataDir, GAME_BACKUP_MARKER, login, seedGameBackup } from './helpers'

// Relies on the "main" instance created by 02-instance-lifecycle (world "Midgard").
const WORLD = 'Midgard'
const STAMP = '20260101120000'
const worldsLocal = path.join(dataDir, 'instances', 'main', 'save', 'worlds_local')

test.describe.serial('game backups', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test("lists the game's own copies", async ({ page }) => {
    const name = seedGameBackup('main', WORLD, STAMP)
    await page.goto('/instances/main/backups')
    const table = page.getByRole('table', { name: 'Game backups' })
    await expect(table).toContainText(name)
  })

  test('restores a copy as a job', async ({ page }) => {
    const name = `${WORLD}_backup_auto-${STAMP}`
    await page.goto('/instances/main/backups')
    const table = page.getByRole('table', { name: 'Game backups' })
    const row = table.getByRole('row', { name: new RegExp(name) })
    await row.hover()
    await row.getByRole('button', { name: /^Restore .*_backup_auto-/ }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog.getByText('Restore game backup')).toBeVisible()
    const stop = dialog.getByLabel(/stop, restore and start again/i)
    if (await stop.count()) await stop.check()
    await dialog.getByRole('button', { name: 'Restore', exact: true }).click()
    // The job drawer opens with the live log; wait for success.
    await expect(page.getByRole('dialog').getByText(/succeeded/i).first()).toBeVisible({ timeout: 30_000 })
    await page.keyboard.press('Escape')
    const db2 = path.join(worldsLocal, WORLD, '_main.1.db2')
    expect(fs.existsSync(db2)).toBeTruthy()
    expect(fs.readFileSync(db2, 'utf8')).toBe(GAME_BACKUP_MARKER)
    // The copy itself stays in place.
    expect(fs.existsSync(path.join(worldsLocal, name))).toBeTruthy()
  })
})
