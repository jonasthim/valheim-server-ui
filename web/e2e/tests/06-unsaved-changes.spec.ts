import { test, expect } from '@playwright/test'
import { login } from './helpers'

// Relies on the "main" instance created by 02-instance-lifecycle (serial
// suites share one worker/data dir, per playwright.config.ts). Never
// page.goto while a form is dirty: Playwright auto-dismisses beforeunload
// and the navigation hangs, so leaving a dirty form goes through an in-app
// link and the "Discard changes?" confirm instead.
test.describe.serial('unsaved-changes system', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('config form shows the sticky bar and discards', async ({ page }) => {
    await page.goto('/instances/main/config')
    const bar = page.getByRole('region', { name: 'Unsaved changes' })
    await expect(bar).toBeHidden()

    const serverName = page.getByLabel(/server name/i)
    const original = await serverName.inputValue()
    await serverName.fill('Dirty name')
    await expect(bar).toBeVisible()

    await page.getByRole('button', { name: 'Discard' }).click()
    await expect(bar).toBeHidden()
    await expect(serverName).toHaveValue(original)
  })

  test('leaving a dirty form asks first', async ({ page }) => {
    await page.goto('/instances/main/config')
    await page.getByLabel(/server name/i).fill('Dirty name')

    await page.getByRole('link', { name: 'Jobs' }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText('Discard changes?')
    await dialog.getByRole('button', { name: 'Keep editing' }).click()
    await expect(page).toHaveURL(/\/config$/)
    await expect(page.getByLabel(/server name/i)).toHaveValue('Dirty name')

    await page.getByRole('link', { name: 'Jobs' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Discard' }).click()
    await expect(page).toHaveURL(/\/jobs$/)
  })

  test('saving marks the form clean', async ({ page }) => {
    await page.goto('/instances/main/config')
    await page.getByLabel(/server name/i).fill('E2E Server Saved')
    await page.getByRole('button', { name: 'Save changes' }).click()
    await expect(page.getByText(/saved/i).first()).toBeVisible()

    const bar = page.getByRole('region', { name: 'Unsaved changes' })
    await expect(bar).toBeHidden()

    await page.getByRole('link', { name: 'Jobs' }).click()
    await expect(page).toHaveURL(/\/jobs$/)
    await expect(page.getByRole('dialog')).toHaveCount(0)
  })

  test('settings bar', async ({ page }) => {
    await page.goto('/settings')
    const bar = page.getByRole('region', { name: 'Unsaved changes' })
    await expect(bar).toBeHidden()

    const autoUpgrade = page.getByLabel('Auto-upgrade')
    await autoUpgrade.click()
    await expect(bar).toBeVisible()
    await expect(autoUpgrade).toBeChecked()

    await page.getByRole('button', { name: 'Discard' }).click()
    await expect(bar).toBeHidden()
    await expect(autoUpgrade).not.toBeChecked()
  })

  test('create keeps its primary button', async ({ page }) => {
    await page.goto('/instances/new')
    await expect(page.getByRole('button', { name: /create instance/i })).toBeVisible()
  })
})
