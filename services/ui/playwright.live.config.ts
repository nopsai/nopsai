import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.NOPS_UI_LIVE_BASE_URL || 'http://127.0.0.1:8080';

export default defineConfig({
  testDir: './e2e-live',
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['html', { open: 'never' }], ['list']] : 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium-live',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
