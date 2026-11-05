import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './E2E',
  testMatch: '*.spec.ts',
  outputDir: './E2E/Results',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 300_000,
  expect: { timeout: 30_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never', outputFolder: './E2E/Report' }]] : 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: process.env.DHOW_E2E_URL,
    viewport: { width: 1440, height: 900 },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
});
