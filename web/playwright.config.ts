import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  // Both modes share the servers the setup starts, so one worker avoids port churn.
  workers: 1,
  reporter: 'list',
  globalSetup: './e2e/global-setup.ts',
})
