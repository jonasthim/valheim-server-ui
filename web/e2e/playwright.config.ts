import { defineConfig } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// End-to-end suite: boots the real binary (direct supervisor + fake game
// server) on a scratch data dir and drives the built SPA. Run with `make e2e`.
const root = path.resolve(__dirname, '..', '..') // repo root
const port = 18099
const dataDir = path.join(root, 'web', 'e2e', '.data')

export default defineConfig({
  testDir: path.join(root, 'web', 'e2e', 'tests'),
  outputDir: path.join(root, 'web', 'e2e', 'test-results'),
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never', outputFolder: path.join(root, 'web', 'e2e', 'report') }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // Local sandboxes may provide a system Chromium; CI installs Playwright's own.
    launchOptions: process.env.PW_CHROMIUM ? { executablePath: process.env.PW_CHROMIUM } : {},
  },
  webServer: {
    command: `rm -rf "${dataDir}" && mkdir -p "${dataDir}" && "${root}/bin/valheim-ui" serve`,
    url: `http://127.0.0.1:${port}/api/v1/auth/status`,
    reuseExistingServer: false,
    timeout: 30_000,
    env: {
      VALHEIM_UI_CONFIG: path.join(root, 'web', 'e2e', 'config.yaml'),
      VALHEIM_UI_DATA_DIR: dataDir,
      VALHEIM_UI_LISTEN: `127.0.0.1:${port}`,
      VALHEIM_UI_BASE_URL: `http://127.0.0.1:${port}`,
      VALHEIM_UI_FAKE_SERVER_PATH: path.join(root, 'testdata', 'fake-server.sh'),
    },
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})
