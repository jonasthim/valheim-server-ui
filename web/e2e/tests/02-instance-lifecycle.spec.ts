import { test, expect } from '@playwright/test'
import { createInstance, fakeInstall, login } from './helpers'

test.describe.serial('instance lifecycle with the fake game server', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('create, start, observe console and players, stop', async ({ page }) => {
    await createInstance(page, { id: 'main', name: 'Main', serverName: 'E2E Server', world: 'Midgard', port: 2456, crossplay: true })
    await expect(page.getByText(/not installed/i).first()).toBeVisible()

    fakeInstall('main')
    await page.reload()
    await page.getByRole('button', { name: /^start$/i }).first().click()
    await expect(page.getByText(/^running$/i).first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('123456').first()).toBeVisible({ timeout: 20_000 })

    await page.goto('/instances/main/console')
    await expect(page.getByText(/Game server connected/).first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(/Got character ZDOID from Bjorn/).first()).toBeVisible({ timeout: 20_000 })

    await page.goto('/instances/main/players')
    await expect(page.getByText('Bjorn').first()).toBeVisible({ timeout: 15_000 })

    await page.goto('/')
    await expect(page.getByText(/1 \/ /).first()).toBeVisible({ timeout: 15_000 })
    await page.getByRole('button', { name: /^stop$/i }).first().click()
    await expect(page.getByText(/^stopped$/i).first()).toBeVisible({ timeout: 15_000 })
  })

  test('port overlap is rejected with a field error', async ({ page }) => {
    await page.goto('/instances/new')
    await page.getByLabel(/display name/i).fill('Second')
    await page.getByLabel(/instance id/i).fill('second')
    await page.getByLabel(/server name/i).fill('Second Server')
    await page.getByRole('textbox', { name: 'Password' }).fill('secret123')
    await page.getByRole('textbox', { name: 'Port' }).fill('2457')
    await page.getByRole('button', { name: /advanced/i }).first().click()
    const installBox = page.getByLabel(/download game files now/i)
    await expect(installBox).toBeVisible()
    await page.waitForTimeout(500) // accordion open animation
    await installBox.uncheck()
    await page.getByRole('button', { name: /create instance/i }).click()
    await expect(page.getByText(/port/i).and(page.getByText(/overlap|in use|conflict/i)).first()).toBeVisible()
    await expect(page).toHaveURL(/\/instances\/new$/)
  })

  test('config edit while stopped, then banned list round-trip', async ({ page }) => {
    await page.goto('/instances/main/config')
    const serverName = page.getByLabel(/server name/i)
    await serverName.fill('E2E Server Renamed')
    await page.getByRole('button', { name: /save/i }).first().click()
    await expect(page.getByText(/saved|updated/i).first()).toBeVisible()

    await page.goto('/instances/main/players')
    await page.getByRole('tab', { name: /banned/i }).click()
    await page.getByPlaceholder(/platform id/i).first().fill('76561198000000099')
    await page.getByPlaceholder(/comment/i).first().fill('griefer')
    await page.getByRole('button', { name: /^add$/i }).first().click()
    await expect(page.getByText('76561198000000099').first()).toBeVisible()
    const res = await page.request.get('/api/v1/instances/main/lists/banned')
    expect(res.ok()).toBeTruthy()
    const body = (await res.json()) as { entries: { id: string; comment?: string }[] }
    expect(body.entries).toEqual([{ id: '76561198000000099', comment: 'griefer' }])
  })
})
