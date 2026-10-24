import { defineConfig } from 'playwright/test';

export default defineConfig({
  testDir: '.',
  testMatch: 'smoke.browser.spec.ts',
  reporter: 'line',
  use: { baseURL: process.env.SMOKE_CLIENT_URL || 'http://127.0.0.1:4173', browserName: 'chromium' },
});
