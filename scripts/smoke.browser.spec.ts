import { expect, test } from 'playwright/test';

test('renders seeded issue and linked PR/CI from the real API', async ({ page }) => {
  if (process.env.SMOKE_CONTROL_NO_SESSION === '1') {
    await page.goto('/board');
    await page.waitForTimeout(500);
    expect(await page.evaluate(() => sessionStorage.getItem('agent-ops-token'))).toBeNull();
    return;
  }
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
  if (process.env.SMOKE_CONTROL_NO_SESSION === '1') {
    test.skip();
    return;
  }
  const apiBase = process.env.SMOKE_API_URL;
  const token = process.env.SESSION_TOKEN;
  if (!apiBase || !token) throw new Error('disposable session and API URL are required');
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
  const invocation = process.env.SMOKE_INVOCATION_ID || `default-${process.pid}`;
  const suffix = invocation.replace(/[^a-zA-Z0-9-]/g, '-');
  const githubName = `fixture/settings-api-github-${suffix}`;
  const gitlabName = `fixture/settings-api-gitlab-${suffix}`;
  const create = async (provider: string, name: string, mappings: object, gitlabURL?: string) => {
    const response = await page.request.post(`${apiBase}/api/v1/repositories`, { headers, data: { provider, full_name: name, status_mappings: mappings, webhook_secret: `disposable-${provider}-webhook`, backfill_token: `disposable-${provider}-backfill`, ...(gitlabURL ? { gitlab_base_url: gitlabURL } : {}) } });
    expect(response.status(), await response.text()).toBe(201);
  };
  await create('github', githubName, { 'status:review': 'blocked' });
  await create('gitlab', gitlabName, { 'Status::Review': 'done' }, 'http://127.0.0.1:9/gitlab');
  await page.addInitScript((sessionToken) => sessionStorage.setItem('agent-ops-token', sessionToken), token);
  await page.goto('/repositories');
  await expect(page.getByRole('heading', { name: 'Repository settings' })).toBeVisible({ timeout: 15000 });
  const githubCard = page.locator('article').filter({ hasText: githubName });
  const gitlab = page.locator('article').filter({ hasText: gitlabName });
  await expect(githubCard.getByRole('heading', { name: `${githubName} (github)` })).toBeVisible({ timeout: 15000 });
  await expect(gitlab.getByRole('heading', { name: `${gitlabName} (gitlab)` })).toBeVisible({ timeout: 15000 });
  await expect(gitlab.getByText('GitLab URL: http://127.0.0.1:9/gitlab', { exact: true })).toBeVisible();
  await expect(gitlab.getByText('Status mappings: {"Status::Review":"done"}')).toBeVisible();
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR || '.agent-runs/smoke'}/repository-settings-dual-provider.png`, fullPage: true });

  expect(calls.some((call) => call === 'GET /api/v1/repositories'), `missing repository settings list request; observed ${calls.join(' | ')}`).toBeTruthy();
  const editValues = ['', '{"status:review":"review"}', 'http://127.0.0.1:9/gitlab-v2', '{"Status::Review":"review"}'];
  page.on('dialog', (dialog) => void dialog.accept(editValues.shift()));
  await githubCard.getByRole('button', { name: 'Edit settings' }).click();
  await expect(githubCard.getByText('Status mappings: {"status:review":"review"}')).toBeVisible();
  await gitlab.getByRole('button', { name: 'Edit settings' }).click();
  await expect(gitlab.getByText('GitLab URL: http://127.0.0.1:9/gitlab-v2')).toBeVisible();
  await expect(gitlab.getByText('Status mappings: {"Status::Review":"review"}')).toBeVisible();
  await githubCard.getByRole('button', { name: 'Delete' }).click();
  await page.reload();
  await expect(page.getByRole('heading', { name: `${githubName} (github)` })).toHaveCount(0);
  const refreshedGitlab = page.locator('article').filter({ hasText: gitlabName });
  await refreshedGitlab.getByRole('button', { name: 'Delete' }).click();
  await page.reload();
  await expect(page.getByRole('heading', { name: `${gitlabName} (gitlab)` })).toHaveCount(0);
  console.log(`Repository settings API calls: ${calls.join(' | ')}`);
});

test('creates lists edits and deletes both settings through real organization HTTP', async ({ page }) => {
  if (process.env.SMOKE_CONTROL_NO_SESSION === '1') {
    test.skip();
    return;
  }
  const token = process.env.SESSION_TOKEN;
  const apiBase = process.env.APP_API_URL || process.env.SMOKE_API_URL;
  if (!token || !apiBase) throw new Error('disposable session and API URL are required');
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
  const invocation = process.env.SMOKE_INVOCATION_ID || `default-${process.pid}`;
  const suffix = invocation.replace(/[^a-zA-Z0-9-]/g, '-');
  const githubName = `fixture/settings-github-${suffix}`;
  const gitlabName = `fixture/settings-gitlab-${suffix}`;
  const ids: Record<string, string> = {};
  const calls: string[] = [];
  const request = async (method: string, path: string, body?: object) => {
    calls.push(`${method} ${path}`);
    return page.request.fetch(`${apiBase}${path}`, { method, headers, data: body });
  };
  const github = await request('POST', '/api/v1/repositories', { provider: 'github', full_name: githubName, webhook_secret: 'smoke-github-webhook', backfill_token: 'smoke-github-backfill', status_mappings: { 'status:review': 'blocked' } });
  expect(github.status(), await github.text()).toBe(201);
  const githubBody = await github.json();
  ids.github = githubBody.data.id;
  expect(githubBody.data.webhook_secret).toBe('smoke-github-webhook');
  expect(githubBody.data.backfill_token).toBe('smoke-github-backfill');
  const gitlab = await request('POST', '/api/v1/repositories', { provider: 'gitlab', full_name: gitlabName, gitlab_base_url: 'http://127.0.0.1:9/gitlab', webhook_secret: 'smoke-gitlab-webhook', backfill_token: 'smoke-gitlab-backfill', status_mappings: { 'Status::Review': 'done' } });
  expect(gitlab.status(), await gitlab.text()).toBe(201);
  const gitlabBody = await gitlab.json();
  ids.gitlab = gitlabBody.data.id;
  expect(gitlabBody.data.webhook_secret).toBe('smoke-gitlab-webhook');
  expect(gitlabBody.data.backfill_token).toBe('smoke-gitlab-backfill');
  const listed = await request('GET', '/api/v1/repositories');
  expect(listed.status(), await listed.text()).toBe(200);
  const items = (await listed.json()).data;
  expect(items.map((item: { full_name: string }) => item.full_name)).toEqual(expect.arrayContaining([gitlabName, githubName]));
  expect(items.find((item: { id: string }) => item.id === ids.gitlab).gitlab_base_url).toBe('http://127.0.0.1:9/gitlab');
  const update = await request('PATCH', `/api/v1/repositories/${ids.gitlab}`, { gitlab_base_url: 'http://127.0.0.1:9/gitlab-v2', status_mappings: { 'Status::Review': 'review' } });
  expect(update.status(), await update.text()).toBe(200);
  const githubUpdate = await request('PATCH', `/api/v1/repositories/${ids.github}`, { status_mappings: { 'status:review': 'review' } });
  expect(githubUpdate.status(), await githubUpdate.text()).toBe(200);
  const editedGithub = await request('GET', `/api/v1/repositories/${ids.github}`);
  expect((await editedGithub.json()).data.status_mappings).toEqual({ 'status:review': 'review' });
  const edited = await request('GET', `/api/v1/repositories/${ids.gitlab}`);
  expect((await edited.json()).data.gitlab_base_url).toBe('http://127.0.0.1:9/gitlab-v2');
  expect((await edited.json()).data.status_mappings).toEqual({ 'Status::Review': 'review' });
  for (const id of Object.values(ids)) expect((await request('DELETE', `/api/v1/repositories/${id}`)).status()).toBe(204);
  console.log(`Repository settings real API calls: ${calls.join(' | ')}`);
});
