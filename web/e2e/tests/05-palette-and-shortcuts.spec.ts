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

// Same shared "main" instance/single-worker suite as above.
test.describe.serial('keyboard shortcuts', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('g j goes to Jobs and ? opens help', async ({ page }) => {
    await page.goto('/')
    await page.keyboard.press('g')
    await page.keyboard.press('j')
    await expect(page).toHaveURL(/\/jobs$/)

    await page.keyboard.press('?')
    const dialog = page.getByRole('dialog', { name: 'Keyboard shortcuts' })
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })

  test('[ and ] move between instance tabs', async ({ page }) => {
    await page.goto('/instances/main/overview')
    await expect(page.getByRole('tab', { name: 'Overview' })).toBeVisible()

    await page.keyboard.press(']')
    await expect(page).toHaveURL(/\/instances\/main\/console$/)

    await page.keyboard.press('[')
    await expect(page).toHaveURL(/\/instances\/main\/overview$/)
  })

  test('shortcuts are ignored while typing', async ({ page }) => {
    await page.goto('/jobs')
    const instanceFilter = page.getByRole('textbox', { name: 'Instance' })
    await instanceFilter.click()
    await page.keyboard.type('gj')
    await expect(page).toHaveURL(/\/jobs$/)
    await expect(instanceFilter).toHaveValue('gj')
  })

  test('mod+B toggles the sidebar', async ({ page }) => {
    await page.keyboard.press('Control+b')
    await expect(page.getByRole('button', { name: 'Expand sidebar' })).toBeVisible()
    await page.keyboard.press('Control+b')
    await expect(page.getByRole('button', { name: 'Collapse sidebar' })).toBeVisible()
  })
})
