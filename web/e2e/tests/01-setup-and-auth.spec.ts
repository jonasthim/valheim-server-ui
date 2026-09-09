import { test, expect } from '@playwright/test'
import { ADMIN, login } from './helpers'

test.describe.serial('first run and authentication', () => {
  test('empty database redirects to the setup wizard and creates the admin', async ({ page }) => {
    await page.goto('/')
    await expect(page).toHaveURL(/\/setup$/)
    await page.getByLabel(/^username/i).fill(ADMIN.username)
    await page.getByLabel(/display name/i).fill(ADMIN.display)
    await page.getByLabel(/^password/i).fill(ADMIN.password)
    await page.getByLabel(/confirm/i).fill(ADMIN.password)
    await page.getByRole('button', { name: /create account/i }).click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByText(ADMIN.display)).toBeVisible()
  })

  test('setup is no longer reachable', async ({ page, request }) => {
    await page.goto('/setup')
    await expect(page).toHaveURL(/\/login$/)
    const res = await request.post('/api/v1/auth/setup', {
      headers: { 'X-Requested-With': 'valheim-ui' },
      data: { username: 'second', password: 'another-long-password' },
    })
    expect(res.status()).toBe(404)
  })

  test('wrong password is rejected, correct one logs in, logout returns to login', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel(/username/i).fill(ADMIN.username)
    await page.getByRole('textbox', { name: 'Password' }).fill('definitely-wrong-pw')
    await page.getByRole('button', { name: /log in|sign in/i }).click()
    await expect(page.getByText(/invalid|incorrect/i).first()).toBeVisible()
    await page.getByRole('textbox', { name: 'Password' }).fill(ADMIN.password)
    await page.getByRole('button', { name: /log in|sign in/i }).click()
    await expect(page).toHaveURL(/\/$/)
    await page.getByText(ADMIN.display).first().click()
    await page.getByText(/log out/i).click()
    await expect(page).toHaveURL(/\/login/)
    await page.goto('/')
    await expect(page).toHaveURL(/\/login/)
  })

  test('users page can create an operator and the audit log records it', async ({ page }) => {
    await login(page)
    await page.goto('/users')
    await page.getByRole('button', { name: /create user|new user|add user/i }).first().click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel(/username/i).fill('ops')
    await dialog.getByRole('textbox', { name: /^password/i }).fill('operator-password-1')
    await dialog.getByRole('button', { name: /create|save/i }).click()
    await expect(page.getByRole('cell', { name: 'ops' }).first()).toBeVisible()
    await page.goto('/audit')
    await expect(page.getByText('user.create').first()).toBeVisible()
  })
})
