import { expect, test } from 'playwright/test';

test('renders seeded issue and linked PR/CI from the real API', async ({ page }) => {
  const apiResponses: string[] = [];
  page.on('response', (response) => {
    if (response.url().includes('/api/v1/')) apiResponses.push(`${response.status()} ${response.url()}`);
  });
  page.on('requestfailed', (request) => {
    if (request.url().includes('/api/v1/')) console.error(`API request failed: ${request.method()} ${request.url()} ${request.failure()?.errorText}`);
  });
  page.on('response', (response) => {
    if (response.url().includes('/api/v1/') && response.status() >= 400) console.error(`API response failed: ${response.status()} ${response.url()}`);
  });
  await page.goto('/board');
  const repositories = page.getByRole('combobox', { name: 'Repository' });
  await expect(repositories).toBeEnabled({ timeout: 15000 });
  await repositories.selectOption({ label: 'smoke/agent-ops' });
  await expect(page.getByText('Smoke fixture: board serves real API data')).toBeVisible({ timeout: 15000 });
  await expect(page.getByText('Smoke fixture: linked pull request')).toBeVisible();
  await expect(page.getByText('CI smoke-ci-99002: Passed')).toBeVisible();
  expect(apiResponses.some((item) => item.startsWith('200 ') && item.includes('/api/v1/repositories'))).toBeTruthy();
  expect(apiResponses.some((item) => item.startsWith('200 ') && item.includes('/api/v1/board?') && item.includes('repository_id='))).toBeTruthy();
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR || '.agent-runs/smoke'}/board.png`, fullPage: true });
  console.log(`API responses: ${apiResponses.join(' | ')}`);
});
