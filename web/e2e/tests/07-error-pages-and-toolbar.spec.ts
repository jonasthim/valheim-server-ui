import { test, expect } from '@playwright/test'
import { login, loginAs } from './helpers'

test.describe.serial('error pages', () => {
  test('unknown routes render the 404 page', async ({ page }) => {
    await login(page)
    await page.goto('/nope')
    await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible()
    await page.getByRole('link', { name: 'Go to dashboard' }).click()
    await expect(page).toHaveURL(/\/$/)
  })

  test('unknown instance tab renders 404', async ({ page }) => {
    await login(page)
    await page.goto('/instances/main/bogus')
    await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible()
  })

  test('non-admins get a 403 on admin pages', async ({ page }) => {
    await loginAs(page, 'ops', 'operator-password-1')
    await page.goto('/settings')
    await expect(page.getByRole('heading', { name: "You don't have access to this page" })).toBeVisible()
    await expect(page).toHaveURL(/\/settings$/)
  })
})
