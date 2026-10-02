import { test, expect } from '@playwright/test'
import { login } from './helpers'

// Relies on the "main" instance created by 02-instance-lifecycle (serial
// suites share one worker/data dir, per playwright.config.ts).
test.describe.serial('gameplay card', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('gameplay card shows the install prompt without the plugin', async ({ page }) => {
    await page.goto('/instances/main/mods')
    await expect(page.getByRole('heading', { name: 'Gameplay' })).toBeVisible()
    await expect(page.getByText(/Install the Gameplay plugin/)).toBeVisible()
    // The e2e fixture server never installs BepInEx, so the installed /
    // update-available states (and the Autofeed/Raids form) are not
    // reachable here. They are covered by gameplayConfig.test.ts (the pure
    // mapping and merge logic) and verified manually against a
    // hand-written cfg file during development (TASK U-1 report).
  })
})
