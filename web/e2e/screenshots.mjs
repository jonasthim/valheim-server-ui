#!/usr/bin/env node
// Boots the built binary with the fake game server on a scratch data dir,
// seeds a realistic state (admin, two instances, one running with a player,
// a backup, a schedule, an operator user) and screenshots every page.
//
//   node web/e2e/screenshots.mjs [--out docs/screenshots] [--scheme dark|light] [--width 1360] [--axe]
//
// Requires `make build` first (the binary embeds web/dist). With --axe, also
// runs axe-core (WCAG 2.x/2.2 AA tags) against every page after its
// screenshot, prints a summary and writes <out>/axe-report.json; exits 1 if
// any violation was found. Requires axe-core installed (not in the lockfile):
//   cd web && npm install --no-save --no-audit --no-fund axe-core@4.10.3
import { chromium } from '@playwright/test'
import { spawn } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(__dirname, '..', '..')
const args = Object.fromEntries(
  process.argv.slice(2).map((a, i, all) => (a.startsWith('--') ? [a.slice(2), all[i + 1] ?? ''] : [])).filter((x) => x.length),
)
const out = path.resolve(root, args.out || 'docs/screenshots')
const scheme = args.scheme || 'dark'
const width = Number(args.width || 1360)
const axeMode = process.argv.includes('--axe')
const port = 18098
const base = `http://127.0.0.1:${port}`
const dataDir = path.join(root, 'web', 'e2e', '.data-screens')
const ADMIN = { username: 'jonas', password: 'correct-horse-battery', display_name: 'Jonas' }

fs.rmSync(dataDir, { recursive: true, force: true })
fs.mkdirSync(dataDir, { recursive: true })
fs.mkdirSync(out, { recursive: true })

const server = spawn(path.join(root, 'bin', 'valheim-ui'), ['serve'], {
  env: {
    ...process.env,
    VALHEIM_UI_CONFIG: path.join(root, 'web', 'e2e', 'config.yaml'),
    VALHEIM_UI_DATA_DIR: dataDir,
    VALHEIM_UI_LISTEN: `127.0.0.1:${port}`,
    VALHEIM_UI_BASE_URL: base,
    VALHEIM_UI_FAKE_SERVER_PATH: path.join(root, 'testdata', 'fake-server.sh'),
  },
  stdio: ['ignore', 'ignore', 'inherit'],
})
const stop = () => {
  try {
    server.kill('SIGTERM')
  } catch {}
}
process.on('exit', stop)
process.on('SIGINT', () => process.exit(130))

async function waitFor(url, ms = 30_000) {
  const t0 = Date.now()
  while (Date.now() - t0 < ms) {
    try {
      const r = await fetch(url)
      if (r.ok) return
    } catch {}
    await new Promise((r) => setTimeout(r, 300))
  }
  throw new Error(`timeout waiting for ${url}`)
}

function fakeInstall(id, worlds = []) {
  const dir = path.join(dataDir, 'instances', id, 'server')
  fs.mkdirSync(dir, { recursive: true })
  fs.writeFileSync(path.join(dir, 'valheim_server.x86_64'), '#!/bin/sh\n', { mode: 0o755 })
  const wdir = path.join(dataDir, 'instances', id, 'save', 'worlds_local')
  fs.mkdirSync(wdir, { recursive: true })
  for (const w of worlds) {
    fs.writeFileSync(path.join(wdir, `${w}.fwl`), Buffer.alloc(2048, 1))
    fs.writeFileSync(path.join(wdir, `${w}.db`), Buffer.alloc(6 * 1024 * 1024, 2))
  }
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// Set once main() decides whether an axe-flagged violation was found, so the
// final .then()/.catch() at the bottom of the file can pick the exit code.
let hadViolations = false

async function main() {
  await waitFor(`${base}/api/v1/auth/status`)
  const browser = await chromium.launch(process.env.PW_CHROMIUM ? { executablePath: process.env.PW_CHROMIUM } : {})
  // bypassCSP lets --axe inject axe-core via addScriptTag; the app serves a
  // strict `script-src 'self'` CSP (internal/api/router.go) that would
  // otherwise block the inline script. No effect on rendering/screenshots.
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme, deviceScaleFactor: 2, bypassCSP: axeMode })
  const page = await context.newPage()
  await page.addInitScript((s) => {
    try {
      window.localStorage.setItem('mantine-color-scheme-value', s)
    } catch {}
  }, scheme)

  // axe-core is loaded once, only in --axe mode, so the script still runs
  // without it installed when the flag is absent.
  const axe = axeMode ? (await import('axe-core')).default : null
  const axeResults = []

  /** Injects axe-core into `targetPage` and runs it against the current
   *  document, scoped to the WCAG 2.x/2.2 AA tags. Collects violations into
   *  `axeResults` and prints a short summary line per page. No-op without
   *  --axe. */
  async function runAxe(name, targetPage = page) {
    if (!axe) return
    await targetPage.addScriptTag({ content: axe.source })
    const result = await targetPage.evaluate(() =>
      // `axe` here is the browser global injected by the script tag above,
      // not the Node-side import — plain .mjs, so no typing needed.
      axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'] } }),
    )
    const violations = result.violations.map((v) => ({
      page: name,
      id: v.id,
      impact: v.impact,
      help: v.help,
      helpUrl: v.helpUrl,
      nodes: v.nodes.map((n) => ({ target: n.target, summary: n.failureSummary })),
    }))
    axeResults.push(...violations)
    console.log(`axe ${name}: ${violations.length} violation(s)`)
    for (const v of violations) {
      console.log(`  ${v.id} ${v.impact} ${v.help}`)
      for (const n of v.nodes.slice(0, 2)) console.log(`    ${JSON.stringify(n.target)}`)
    }
  }
  const api = async (method, p, data) => {
    const res = await page.request.fetch(`${base}/api/v1${p}`, {
      method,
      headers: { 'X-Requested-With': 'valheim-ui', 'Content-Type': 'application/json' },
      data: data === undefined ? undefined : JSON.stringify(data),
    })
    if (!res.ok()) throw new Error(`${method} ${p}: ${res.status()} ${await res.text()}`)
    const text = await res.text()
    return text ? JSON.parse(text) : null
  }

  // --- seed -------------------------------------------------------------
  await api('POST', '/auth/setup', ADMIN)
  await api('POST', '/instances', {
    id: 'berra',
    name: 'Berra goes to Valhalla',
    install: false,
    autostart: true,
    config: {
      name: 'Berra goes to Valhalla',
      world: 'Dedotated wham',
      password: 'secret123',
      port: 2456,
      public: true,
      crossplay: false,
      preset: 'hard',
      modifiers: { resources: 'more' },
      setkeys: ['fire'],
    },
  })
  await api('POST', '/instances', {
    id: 'creative',
    name: 'Creative build night',
    install: false,
    config: {
      name: 'Creative build night',
      world: 'Asgard',
      password: 'secret123',
      port: 2466,
      public: false,
      crossplay: true,
      preset: 'hammer',
      modifiers: {},
      setkeys: ['nobuildcost', 'passivemobs'],
    },
  })
  fakeInstall('berra', ['Dedotated wham', 'Old meadows'])
  fakeInstall('creative', ['Asgard'])
  await api('POST', '/users', { username: 'ops', display_name: 'Operator Olof', password: 'operator-password-1', role: 'operator' }).catch(() => {})
  await api('POST', '/users', { username: 'viewer', display_name: 'Guest', password: 'viewer-password-1', role: 'viewer' }).catch(() => {})
  await api('POST', '/instances/berra/schedules', { kind: 'backup', cron: '0 4 * * *', enabled: true, note: 'Nightly backup' }).catch((e) =>
    console.error('schedule:', e.message),
  )
  await api('POST', '/instances/berra/schedules', { kind: 'restart', cron: '0 5 * * 1', enabled: true, only_when_empty: true, note: 'Weekly restart' }).catch((e) =>
    console.error('schedule:', e.message),
  )
  // A couple of edits so the audit log shows real field diffs.
  await api('PATCH', '/instances/berra', { config: { modifiers: { resources: 'more', portals: 'hard' } } })
  await api('PATCH', '/instances/berra', { config: { modifiers: { resources: 'more' }, setkeys: ['fire', 'nomap'] } })
  await api('PATCH', '/instances/berra', { config: { setkeys: ['fire'] }, autostart: true })
  await api('POST', '/instances/berra/start')
  // wait for running + player from the fake server
  for (let i = 0; i < 60; i++) {
    const st = await api('GET', '/instances/berra/status')
    if (st.status.state === 'running' && st.status.players_online > 0) break
    await sleep(1000)
  }
  await api('POST', '/instances/berra/backups', { note: 'Before the plains raid' }).catch((e) => console.error('backup:', e.message))
  await sleep(2500)

  // --- screenshots ------------------------------------------------------
  const shots = [
    ['dashboard', '/'],
    ['overview', '/instances/berra/overview'],
    ['console', '/instances/berra/console'],
    ['config', '/instances/berra/config'],
    ['players', '/instances/berra/players'],
    ['worlds', '/instances/berra/worlds'],
    ['backups', '/instances/berra/backups'],
    ['mods', '/instances/berra/mods'],
    ['schedules', '/instances/berra/schedules'],
    ['jobs', '/jobs'],
    ['users', '/users'],
    ['settings', '/settings'],
    ['audit', '/audit'],
    ['account', '/account'],
  ]
  async function shot(name, p) {
    await page.goto(`${base}${p}`, { waitUntil: 'networkidle' })
    await sleep(600)
    await page.screenshot({ path: path.join(out, `${name}.png`), fullPage: name !== 'console' })
    console.log('shot', name)
    await runAxe(name, page)
  }
  for (const [name, p] of shots) {
    await shot(name, p)
  }
  // Mobile dashboard + overview
  const mobile = await browser.newContext({ viewport: { width: 390, height: 844 }, colorScheme: scheme, deviceScaleFactor: 2, isMobile: true, bypassCSP: axeMode })
  const mp = await mobile.newPage()
  await mp.addInitScript((s) => {
    try {
      window.localStorage.setItem('mantine-color-scheme-value', s)
    } catch {}
  }, scheme)
  const cookies = await context.cookies()
  await mobile.addCookies(cookies)
  await mp.goto(`${base}/`, { waitUntil: 'networkidle' })
  await sleep(600)
  await mp.screenshot({ path: path.join(out, 'mobile-dashboard.png') })
  await runAxe('mobile-dashboard', mp)
  await mp.goto(`${base}/instances/berra/overview`, { waitUntil: 'networkidle' })
  await sleep(600)
  await mp.screenshot({ path: path.join(out, 'mobile-overview.png') })
  await runAxe('mobile-overview', mp)
  await mobile.close()

  // Login page in a fresh context
  const anon = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme, deviceScaleFactor: 2, bypassCSP: axeMode })
  const ap = await anon.newPage()
  await ap.addInitScript((s) => {
    try {
      window.localStorage.setItem('mantine-color-scheme-value', s)
    } catch {}
  }, scheme)
  await ap.goto(`${base}/login`, { waitUntil: 'networkidle' })
  await sleep(400)
  await ap.screenshot({ path: path.join(out, 'login.png') })
  await runAxe('login', ap)
  await anon.close()

  await browser.close()

  if (axe) {
    fs.writeFileSync(path.join(out, 'axe-report.json'), JSON.stringify(axeResults, null, 2))
    hadViolations = axeResults.length > 0
    console.log(`axe: ${axeResults.length} total violation(s) -> ${path.join(out, 'axe-report.json')}`)
  }

  console.log('done ->', out)
}

main()
  .then(() => process.exit(hadViolations ? 1 : 0))
  .catch((e) => {
    console.error(e)
    process.exit(1)
  })
