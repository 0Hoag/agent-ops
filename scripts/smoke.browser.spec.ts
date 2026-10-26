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
  if (process.env.SMOKE_CONTROL_NO_SESSION !== '1') {
    await page.addInitScript((token) => sessionStorage.setItem('agent-ops-token', token), process.env.SESSION_TOKEN);
  }
  await page.goto('/board');
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

test('renders both provider settings from the real API', async ({ page }) => {
  const calls: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/v1/repositories')) calls.push(`${request.method()} ${new URL(request.url()).pathname}`);
  });
  if (process.env.SMOKE_CONTROL_NO_SESSION !== '1') await page.addInitScript((token) => sessionStorage.setItem('agent-ops-token', token), process.env.SESSION_TOKEN);
  await page.goto('/repositories');
  await expect(page.getByRole('heading', { name: 'Repository settings' })).toBeVisible({ timeout: 15000 });
  await page.getByLabel('Organization').selectOption(process.env.SMOKE_ORGANIZATION_ID);
  await expect(page.getByLabel('Full name')).toBeVisible({ timeout: 15000 });
  const create = async (provider: string, name: string, mappings: string, gitlabURL?: string) => {
    await page.getByLabel('Provider').selectOption(provider);
    await page.getByLabel('Full name').fill(name);
    if (gitlabURL) await page.getByLabel('Self-hosted base URL').fill(gitlabURL);
    await page.getByLabel('Status mappings (JSON)').fill(mappings);
    await page.getByLabel('Webhook credential').fill(`disposable-${provider}-webhook`);
    await page.getByLabel('Backfill credential').fill(`disposable-${provider}-backfill`);
    await page.getByRole('button', { name: 'Create repository' }).click();
    await expect(page.getByRole('heading', { name: new RegExp(`${name} \\(${provider}\\)`) })).toBeVisible({ timeout: 15000 });
  };
  await create('github', 'fixture/settings-github', '{"status:review":"blocked"}');
  await create('gitlab', 'fixture/settings-gitlab', '{"Status::Review":"done"}', 'http://127.0.0.1:9/gitlab');
  await expect(page.getByText('GitLab URL: http://127.0.0.1:9/gitlab')).toBeVisible();
  await expect(page.getByText('Status mappings: {"Status::Review":"done"}')).toBeVisible();
  await expect(page.getByText('GitLab URL: http://127.0.0.1:9/gitlab')).toBeVisible();
  await expect(page.getByText('Status mappings: {"Status::Review":"done"}')).toBeVisible();
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR || '.agent-runs/smoke'}/repository-settings-dual-provider.png`, fullPage: true });

  const gitlab = page.locator('article').filter({ hasText: 'fixture/settings-gitlab' });
  await expect(gitlab.getByRole('button', { name: 'Edit settings' })).toBeVisible();
  expect(calls.some((call) => call === 'GET /api/v1/repositories'), `missing repository settings list request; observed ${calls.join(' | ')}`).toBeTruthy();
  const githubCard = page.locator('article').filter({ hasText: 'fixture/settings-github' });
  await githubCard.getByRole('button', { name: 'Edit settings' }).click();
  await page.getByLabel('Status mappings (JSON)').fill('{"status:review":"review"}');
  await page.getByRole('button', { name: 'Save settings' }).click();
  await expect(githubCard.getByText('Status mappings: {"status:review":"review"}')).toBeVisible();
  await githubCard.getByRole('button', { name: 'Delete repository' }).click();
  await page.getByRole('button', { name: 'Confirm delete' }).click();
  await expect(githubCard).toHaveCount(0);
  await gitlab.getByRole('button', { name: 'Delete repository' }).click();
  await page.getByRole('button', { name: 'Confirm delete' }).click();
  await expect(gitlab).toHaveCount(0);
  console.log(`Repository settings API calls: ${calls.join(' | ')}`);
});

test('creates lists edits and deletes both settings through real organization HTTP', async ({ page }) => {
  if (process.env.SMOKE_CONTROL_NO_SESSION === '1') test.skip();
  const token = process.env.SESSION_TOKEN;
  const apiBase = process.env.APP_API_URL || process.env.SMOKE_API_URL;
  if (!token || !apiBase) throw new Error('disposable session and API URL are required');
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
  const ids: Record<string, string> = {};
  const calls: string[] = [];
  const request = async (method: string, path: string, body?: object) => {
    calls.push(`${method} ${path}`);
    return page.request.fetch(`${apiBase}${path}`, { method, headers, data: body });
  };
  const github = await request('POST', '/api/v1/repositories', { provider: 'github', full_name: 'fixture/settings-github', webhook_secret: 'smoke-github-webhook', backfill_token: 'smoke-github-backfill', status_mappings: { 'status:review': 'blocked' } });
  expect(github.status(), await github.text()).toBe(201);
  const githubBody = await github.json();
  ids.github = githubBody.data.id;
  expect(githubBody.data.webhook_secret).toBe('smoke-github-webhook');
  expect(githubBody.data.backfill_token).toBe('smoke-github-backfill');
  const gitlab = await request('POST', '/api/v1/repositories', { provider: 'gitlab', full_name: 'fixture/settings-gitlab', gitlab_base_url: 'http://127.0.0.1:9/gitlab', webhook_secret: 'smoke-gitlab-webhook', backfill_token: 'smoke-gitlab-backfill', status_mappings: { 'Status::Review': 'done' } });
  expect(gitlab.status(), await gitlab.text()).toBe(201);
  const gitlabBody = await gitlab.json();
  ids.gitlab = gitlabBody.data.id;
  expect(gitlabBody.data.webhook_secret).toBe('smoke-gitlab-webhook');
  expect(gitlabBody.data.backfill_token).toBe('smoke-gitlab-backfill');
  const listed = await request('GET', '/api/v1/repositories');
  expect(listed.status(), await listed.text()).toBe(200);
  const items = (await listed.json()).data;
  expect(items.map((item: { full_name: string }) => item.full_name).sort()).toEqual(['fixture/settings-gitlab', 'fixture/settings-github'].sort());
  expect(items.find((item: { provider: string }) => item.provider === 'gitlab').gitlab_base_url).toBe('http://127.0.0.1:9/gitlab');
  const update = await request('PATCH', `/api/v1/repositories/${ids.gitlab}`, { gitlab_base_url: 'http://127.0.0.1:9/gitlab-v2', status_mappings: { 'Status::Review': 'review' } });
  expect(update.status(), await update.text()).toBe(200);
  const edited = await request('GET', `/api/v1/repositories/${ids.gitlab}`);
  expect((await edited.json()).data.gitlab_base_url).toBe('http://127.0.0.1:9/gitlab-v2');
  expect((await edited.json()).data.status_mappings).toEqual({ 'Status::Review': 'review' });
  for (const id of Object.values(ids)) expect((await request('DELETE', `/api/v1/repositories/${id}`)).status()).toBe(204);
  console.log(`Repository settings real API calls: ${calls.join(' | ')}`);
  await page.goto('/repositories');
  await expect(page.getByRole('heading', { name: 'Repository settings' })).toBeVisible();
});
