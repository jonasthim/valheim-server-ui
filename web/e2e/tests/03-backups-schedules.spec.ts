import { test, expect } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { dataDir, login } from './helpers'

// Relies on the "main" instance created by 02-instance-lifecycle (serial suites
// share the server). Seeds world files so a backup has something to zip.
function seedWorld(id: string, world: string) {
  const dir = path.join(dataDir, 'instances', id, 'save', 'worlds_local')
  fs.mkdirSync(dir, { recursive: true })
  fs.writeFileSync(path.join(dir, `${world}.fwl`), 'fake-fwl')
  fs.writeFileSync(path.join(dir, `${world}.db`), Buffer.alloc(4096, 1))
}

test.describe.serial('backups, worlds and schedules', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('manual backup appears in the list and downloads', async ({ page }) => {
    seedWorld('main', 'Midgard')
    await page.goto('/instances/main/backups')
    await page.getByRole('button', { name: /back up now/i }).first().click()
    // The job drawer opens with the live log; wait for success, then close it.
    const drawer = page.getByRole('dialog')
    await expect(drawer.getByText(/succeeded/i).first()).toBeVisible({ timeout: 15_000 })
    await page.keyboard.press('Escape')
    await expect(page.getByText(/manual/i).first()).toBeVisible({ timeout: 15_000 })
    const res = await page.request.get('/api/v1/instances/main/backups')
    const body = (await res.json()) as { backups: { id: number; kind: string; filename: string; size_bytes: number }[] }
    expect(body.backups.length).toBeGreaterThan(0)
    expect(body.backups[0].kind).toBe('manual')
    const dl = await page.request.get(`/api/v1/instances/main/backups/${body.backups[0].id}/download`)
    expect(dl.ok()).toBeTruthy()
    expect(dl.headers()['content-type']).toContain('zip')
  })

  test('worlds tab lists the seeded world as active', async ({ page }) => {
    await page.goto('/instances/main/worlds')
    await expect(page.getByText('Midgard').first()).toBeVisible()
    await expect(page.getByText(/active/i).first()).toBeVisible()
  })

  test('restore creates a pre_restore backup first', async ({ page }) => {
    const list = await page.request.get('/api/v1/instances/main/backups')
    const { backups } = (await list.json()) as { backups: { id: number }[] }
    const res = await page.request.post(`/api/v1/instances/main/backups/${backups[0].id}/restore`, {
      headers: { 'X-Requested-With': 'valheim-ui' },
      data: {},
    })
    expect(res.status()).toBe(202)
    const { job } = (await res.json()) as { job: { id: string } }
    await expect
      .poll(async () => ((await (await page.request.get(`/api/v1/jobs/${job.id}`)).json()) as { job: { status: string } }).job.status, {
        timeout: 20_000,
      })
      .toBe('succeeded')
    const after = (await (await page.request.get('/api/v1/instances/main/backups')).json()) as { backups: { kind: string }[] }
    expect(after.backups.some((b) => b.kind === 'pre_restore')).toBeTruthy()
    await page.goto('/jobs')
    await expect(page.getByText(/restore/i).first()).toBeVisible()
  })

  test('schedule can be created, shows next run, and runs now', async ({ page }) => {
    await page.goto('/instances/main/schedules')
    await page.getByRole('button', { name: /add schedule|new schedule|create schedule/i }).first().click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    // kind defaults are fine; pick the daily preset when a preset select exists
    const preset = dialog.getByLabel(/preset/i)
    if (await preset.count()) {
      await preset.click()
      await page.getByRole('option', { name: /daily/i }).first().click()
    }
    await dialog.getByRole('button', { name: /create|save/i }).first().click()
    await expect(page.getByText(/0 4 \* \* \*|04:00|daily/i).first()).toBeVisible({ timeout: 10_000 })
    const res = await page.request.get('/api/v1/instances/main/schedules')
    const { schedules } = (await res.json()) as { schedules: { id: number; next_run_at?: string; kind: string }[] }
    expect(schedules.length).toBe(1)
    expect(schedules[0].next_run_at).toBeTruthy()
    const run = await page.request.post(`/api/v1/instances/main/schedules/${schedules[0].id}/run`, {
      headers: { 'X-Requested-With': 'valheim-ui' },
    })
    // restart on a stopped instance is a no-op job; backup/update produce jobs; skipped → 409
    expect([202, 409]).toContain(run.status())
  })
})
