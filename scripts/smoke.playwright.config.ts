import { defineConfig } from 'playwright/test';

export default defineConfig({
  testDir: '.',
  testMatch: 'smoke.browser.spec.ts',
  reporter: 'line',
  outputDir: `${process.env.SMOKE_ARTIFACT_DIR || '.agent-runs/smoke'}/test-results`,
  use: { baseURL: process.env.SMOKE_CLIENT_URL || 'http://127.0.0.1:4173', browserName: 'chromium' },
});
