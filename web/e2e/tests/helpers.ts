import { expect, type Page } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export const ADMIN = { username: 'admin', password: 'correct-horse-battery', display: 'Jonas' }
export const dataDir = path.resolve(__dirname, '..', '.data')

/** Marks an instance as installed by creating a fake server binary. */
export function fakeInstall(id: string) {
  const dir = path.join(dataDir, 'instances', id, 'server')
  fs.mkdirSync(dir, { recursive: true })
  fs.writeFileSync(path.join(dir, 'valheim_server.x86_64'), '#!/bin/sh\n', { mode: 0o755 })
}

export async function login(page: Page) {
  await page.goto('/login')
  await page.getByLabel(/username/i).fill(ADMIN.username)
  await page.getByRole('textbox', { name: 'Password' }).fill(ADMIN.password)
  await page.getByRole('button', { name: /log in|sign in/i }).click()
  await expect(page).toHaveURL(/\/$/)
}

export async function createInstance(page: Page, opts: { id: string; name: string; serverName: string; world: string; port: number }) {
  await page.goto('/instances/new')
  await page.getByLabel(/display name/i).fill(opts.name)
  await page.getByLabel(/instance id/i).fill(opts.id)
  await page.getByLabel(/server name/i).fill(opts.serverName)
  await page.getByRole('textbox', { name: 'World name' }).fill(opts.world)
  await page.getByRole('textbox', { name: 'Password' }).fill('secret123')
  await page.getByRole('textbox', { name: 'Port' }).fill(String(opts.port))
  await page.getByRole('button', { name: /advanced/i }).first().click()
  const installBox = page.getByLabel(/download game files now/i)
  await expect(installBox).toBeVisible()
  await page.waitForTimeout(500) // accordion open animation
  await installBox.uncheck()
  await page.getByRole('button', { name: /create instance/i }).click()
  await expect(page).toHaveURL(new RegExp(`/instances/${opts.id}/overview`))
}
