import { test, expect } from '@playwright/test'
import { login } from './helpers'

test.describe.serial('toolbar filters', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('jobs toolbar clears filters', async ({ page }) => {
    await page.goto('/jobs')
    const instanceInput = page.getByRole('textbox', { name: 'Instance' })
    await instanceInput.fill('zzz')
    const clearButton = page.getByRole('button', { name: 'Clear filters' })
    await expect(clearButton).toBeVisible()
    await clearButton.click()
    await expect(instanceInput).toHaveValue('')
    await expect(clearButton).toBeHidden()
  })

  test('audit toolbar clears filters', async ({ page }) => {
    await page.goto('/audit')
    const usernameInput = page.getByRole('textbox', { name: 'Filter by username' })
    await usernameInput.fill('zzz')
    const clearButton = page.getByRole('button', { name: 'Clear filters' })
    await expect(clearButton).toBeVisible()
    await clearButton.click()
    await expect(usernameInput).toHaveValue('')
    await expect(clearButton).toBeHidden()
  })
})
