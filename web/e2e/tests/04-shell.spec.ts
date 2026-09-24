import { test, expect } from '@playwright/test'
import { login } from './helpers'

// Relies on the "main" instance created by 02-instance-lifecycle (serial
// suites share one worker/data dir, per playwright.config.ts).
test.describe.serial('shell top bar', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('account menu logs out from the top bar', async ({ page }) => {
    await page.getByRole('button', { name: 'Account menu' }).click()
    await page.getByRole('menuitem', { name: 'Log out' }).click()
    await expect(page).toHaveURL(/\/login/)
  })

  test('breadcrumbs follow the route', async ({ page }) => {
    await page.goto('/instances/main/config')
    const nav = page.getByRole('navigation', { name: 'Breadcrumb' })
    await expect(nav.getByRole('link', { name: 'Dashboard' })).toBeVisible()
    await expect(nav.getByRole('link', { name: 'Main' })).toBeVisible()
    await expect(nav.locator('[aria-current="page"]:visible')).toHaveText('Config')
  })

  test('document titles follow the page', async ({ page }) => {
    await page.goto('/')
    await expect(page).toHaveTitle('Dashboard · Valheim Server UI')

    await page.goto('/jobs')
    await expect(page).toHaveTitle('Jobs · Valheim Server UI')

    await page.goto('/instances/main/config')
    await expect(page).toHaveTitle('Main · Config · Valheim Server UI')
  })

  test('theme menu switches scheme', async ({ page }) => {
    await page.getByRole('button', { name: 'Account menu' }).click()
    await page.getByRole('menuitem', { name: 'Light' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'light')
    await page.getByRole('button', { name: 'Account menu' }).click()
    await page.getByRole('menuitem', { name: 'Dark' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'dark')
  })

  test('sidebar collapses to an icon rail and remembers it', async ({ page }) => {
    await page.getByRole('button', { name: 'Collapse sidebar' }).click()
    await expect(page.getByRole('button', { name: 'Expand sidebar' })).toBeVisible()
    expect(await page.evaluate(() => localStorage.getItem('vh-sidebar-collapsed'))).toBe('true')

    await page.reload()
    await expect(page.getByRole('button', { name: 'Expand sidebar' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Jobs' })).toBeVisible()

    await page.getByRole('button', { name: 'Expand sidebar' }).click()
    await expect(page.getByRole('button', { name: 'Collapse sidebar' })).toBeVisible()
  })
})
