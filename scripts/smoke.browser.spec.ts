import { expect, test } from 'playwright/test';

test('renders seeded issue and linked PR/CI from the real API', async ({ page }) => {
  const apiResponses: string[] = [];
  const apiErrors: string[] = [];
  page.on('response', (response) => {
    if (response.url().includes('/api/v1/')) apiResponses.push(`${response.status()} ${response.url()}`);
  });
  page.on('requestfailed', (request) => {
    if (request.url().includes('/api/v1/')) console.error(`API request failed: ${request.method()} ${request.url()} ${request.failure()?.errorText}`);
  });
  page.on('response', (response) => {
    if (response.url().includes('/api/v1/') && response.status() >= 400) {
      const diagnostic = `${response.status()} ${response.url()}`;
      apiErrors.push(diagnostic);
      console.error(`API response failed: ${diagnostic}`);
    }
  });
  await page.addInitScript((token) => sessionStorage.setItem('agent-ops-token', token), process.env.SESSION_TOKEN);
  await page.goto('/board');
  if (process.env.SMOKE_CONTROL_NO_SESSION === '1') {
    const responseStatus = await page.evaluate(async (apiURL) => {
      const credentials = await fetch(`${apiURL}/api/v1/auth/login`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: 'smoke@example.invalid', password: 'smoke-password-not-a-secret' }),
      });
      if (!credentials.ok) throw new Error(`real account login failed: ${credentials.status}`);
      const { token } = await credentials.json();
      sessionStorage.setItem('agent-ops-token', token);
      const response = await fetch(`${apiURL}/api/v1/repositories?page=1&per_page=100`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      return response.status;
    }, process.env.SMOKE_API_URL);
    expect(responseStatus, `expected account-only session to be rejected for tenant data; got ${responseStatus}`).toBe(401);
    expect(apiErrors.some((item) => item.startsWith('401 ') && item.includes('/api/v1/repositories'))).toBeTruthy();
    console.log(`EXPECTED TENANT CONTROL: account login issued a real token; tenant repository request returned ${responseStatus}`);
    return;
  }
  await expect(page.getByRole('heading', { name: 'Issue board' })).toBeVisible({ timeout: 15000 });
  const repositories = page.getByRole('combobox', { name: 'Repository' });
  await expect(repositories).toBeEnabled({ timeout: 15000 });
  await repositories.selectOption({ label: 'smoke/agent-ops' });
  await expect(page.getByText('Smoke fixture: board serves real API data')).toBeVisible({ timeout: 15000 });
  await expect(page.getByText('Smoke fixture: linked pull request')).toBeVisible();
  await expect(page.getByText('CI smoke-ci-99002: Passed')).toBeVisible();
  expect(apiResponses.some((item) => item.startsWith('200 ') && item.includes('/api/v1/repositories'))).toBeTruthy();
  expect(apiResponses.some((item) => item.startsWith('200 ') && item.includes('/api/v1/board?') && item.includes('repository_id='))).toBeTruthy();
  expect(apiResponses.some((item) => item.startsWith('200 ') && item.includes('/api/v1/auth/session'))).toBeTruthy();
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR || '.agent-runs/smoke'}/board.png`, fullPage: true });
  console.log(`API responses: ${apiResponses.join(' | ')}`);
});
