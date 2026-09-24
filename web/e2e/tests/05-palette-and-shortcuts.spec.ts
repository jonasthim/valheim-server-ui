import { test, expect } from '@playwright/test'
import { login } from './helpers'

// Relies on the "main" instance created by 02-instance-lifecycle (serial
// suites share one worker/data dir, per playwright.config.ts); it is
// stopped by the end of that suite, so "Start Main" is available here.
test.describe.serial('command palette', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('palette opens from the top bar and Escape closes it', async ({ page }) => {
    await page.getByRole('button', { name: 'Search commands' }).click()
    const search = page.getByPlaceholder(/type a command/i)
    await expect(search).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(search).toBeHidden()
  })

  test('palette navigates', async ({ page }) => {
    await page.keyboard.press('Control+k')
    const search = page.getByPlaceholder(/type a command/i)
    await search.fill('go to jobs')
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(/\/jobs$/)
  })

  test('palette lists tabs and lifecycle for the current instance', async ({ page }) => {
    await page.goto('/instances/main/overview')

    await page.keyboard.press('Control+k')
    const search = page.getByPlaceholder(/type a command/i)
    await search.fill('Main: Config')
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(/\/instances\/main\/config$/)

    await page.keyboard.press('Control+k')
    await search.fill('Start Main')
    await expect(page.getByText('Start Main')).toBeVisible()
    await page.keyboard.press('Escape')
  })
})
