import { expect, test } from 'playwright/test';

type AggregateRow = {
  wave: string;
  worker: string;
  runs: number;
  cost_usd?: number;
  cost_vnd?: number;
  billed_cost_usd?: number;
  equivalent_cost_usd?: number;
};

test('pricing versions, permissions, tenant isolation and run costs use the real API', async ({ page }) => {
  if (process.env.SMOKE_CONTROL_NO_SESSION === '1') test.skip();
  const api = process.env.SMOKE_API_URL!;
  const token = process.env.SESSION_TOKEN!;
  const org = process.env.SMOKE_ORGANIZATION_ID!;
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
  const calls: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/v1/')) calls.push(`${request.method()} ${new URL(request.url()).pathname}`);
  });
  const request = async (url: string, init: { method?: string; data?: unknown; token?: string } = {}) => {
    const response = await page.request.fetch(`${api}${url}`, {
      method: init.method || 'GET',
      headers: { ...headers, Authorization: `Bearer ${init.token || token}` },
      data: init.data,
    });
    return response;
  };
  const priceResponse = await request('/api/v1/agent-run-prices');
  expect(priceResponse.status()).toBe(200);
  const responseLog: Array<{ endpoint: string; status: number; body: unknown }> = [
    { endpoint: '/api/v1/agent-run-prices', status: priceResponse.status(), body: await priceResponse.json() },
  ];
  const aggregateResponse = await request('/api/v1/agent-runs/aggregate');
  expect(aggregateResponse.status(), `aggregate HTTP ${aggregateResponse.status()}`).toBe(200);
  const aggregateBody = await aggregateResponse.json();
  responseLog.push({ endpoint: '/api/v1/agent-runs/aggregate', status: aggregateResponse.status(), body: aggregateBody });
  const rows = aggregateBody.data as AggregateRow[];
  expect(rows).toHaveLength(2);
  const apiRun = rows.find((row) => row.wave === 'W10' && row.worker === 'smoke-api' && row.cost_usd === 1.25 && row.equivalent_cost_usd === 1.25 && row.billed_cost_usd === 1.25);
  const subscriptionRun = rows.find((row) => row.wave === 'W10' && row.worker === 'smoke-sub' && row.cost_usd === 2.5 && row.equivalent_cost_usd === 2.5 && row.billed_cost_usd === 0);
  expect(apiRun, JSON.stringify(rows)).toBeTruthy();
  expect(subscriptionRun, JSON.stringify(rows)).toBeTruthy();
  responseLog.push({ endpoint: '/api/v1/agent-runs/aggregate asserted totals', status: 200, body: rows });
  const exchangeRate = Number(process.env.EXCHANGE_RATE_VND_PER_USD || 0);
  if (exchangeRate > 0) {
    expect(apiRun!.cost_vnd).toBeCloseTo(apiRun!.cost_usd! * exchangeRate, 6);
    expect(subscriptionRun!.cost_vnd).toBeCloseTo(subscriptionRun!.cost_usd! * exchangeRate, 6);
    expect(apiRun!.cost_vnd).toBeGreaterThan(apiRun!.cost_usd!);
    expect(subscriptionRun!.cost_vnd).toBeGreaterThan(subscriptionRun!.cost_usd!);
  } else {
    expect(apiRun!.cost_vnd).toBeUndefined();
    expect(subscriptionRun!.cost_vnd).toBeUndefined();
  }

  await page.addInitScript((sessionToken) => sessionStorage.setItem('agent-ops-token', sessionToken), token);
  await page.goto('/prices');
  await expect(page.getByRole('heading', { name: 'Model prices' })).toBeVisible();
  const suffix = (process.env.SMOKE_INVOCATION_ID || `${Date.now()}`).replace(/[^a-zA-Z0-9-]/g, '-');
  const model = `smoke-${suffix}`;
  const occupiedDate = '2027-01-01T00:00:00Z';
  const newDate = '2027-01-02';
  const secondDate = '2027-01-03';
  const draft = { model, effective_date: occupiedDate, input_per_million: 1, cached_input_per_million: 0.2, cache_write_per_million: 0.3, output_per_million: 2, reasoning_per_million: 0.4 };
  await page.getByLabel('Model').fill(model);
  await page.getByLabel('Effective date').fill(occupiedDate.slice(0, 10));
  await page.getByRole('spinbutton', { name: 'input per million', exact: true }).fill('1');
  await page.getByRole('spinbutton', { name: 'cached input per million', exact: true }).fill('0.2');
  await page.getByRole('spinbutton', { name: 'cache write per million', exact: true }).fill('0.3');
  await page.getByRole('spinbutton', { name: 'output per million', exact: true }).fill('2');
  await page.getByRole('spinbutton', { name: 'reasoning per million', exact: true }).fill('0.4');
  const createResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().includes('/api/v1/agent-run-prices'));
  await page.getByRole('button', { name: 'Create version' }).click();
  const createResponse = await createResponsePromise;
  const createBody = await createResponse.json();
  responseLog.push({ endpoint: '/api/v1/agent-run-prices (browser create)', status: createResponse.status(), body: createBody, request: createResponse.request().postDataJSON() });
  expect(createResponse.status(), JSON.stringify({ request: createResponse.request().postDataJSON(), response: createBody })).toBe(201);
  await expect(page.getByRole('alert')).toHaveCount(0);
  const initialPriceResponse = await request('/api/v1/agent-run-prices');
  const initialPriceBody = await initialPriceResponse.json();
  expect(initialPriceResponse.status(), JSON.stringify(initialPriceBody)).toBe(200);
  expect(initialPriceBody.data.some((item: { model: string }) => item.model === model), JSON.stringify(initialPriceBody)).toBeTruthy();
  await expect(page.getByText(model, { exact: true })).toBeVisible();
  const existingPrices = await request('/api/v1/agent-run-prices').then((response) => response.json());
  const newPrice = existingPrices.data.find((item: { model: string }) => item.model === model);
  expect(newPrice).toBeTruthy();
  const ownPriceRow = page.locator('tr').filter({ hasText: model }).filter({ hasText: occupiedDate.slice(0, 10) });
  await expect(ownPriceRow).toHaveCount(1);
  const occupiedDraft = { ...draft, effective_date: `${newDate}T00:00:00Z` };
  const occupiedCreate = await request('/api/v1/agent-run-prices', { method: 'POST', data: occupiedDraft });
  expect(occupiedCreate.status()).toBe(201);
  responseLog.push({ endpoint: '/api/v1/agent-run-prices (occupied model/date)', status: occupiedCreate.status(), body: await occupiedCreate.json() });
  await ownPriceRow.getByRole('button', { name: 'Edit' }).click();
  await expect(page.getByLabel('Model')).toHaveValue(model);
  await page.getByLabel('Effective date').fill(occupiedDate);
  await page.getByRole('spinbutton', { name: 'output per million', exact: true }).fill('3');
  const rejectedPatch = page.waitForResponse((response) => response.request().method() === 'PATCH' && response.url().includes(`/api/v1/agent-run-prices/${newPrice.id}`));
  await page.getByRole('button', { name: /save|update/i }).click();
  const rejectedResponse = await rejectedPatch;
  expect([400, 409]).toContain(rejectedResponse.status());
  responseLog.push({ endpoint: '/api/v1/agent-run-prices (occupied browser PATCH)', status: rejectedResponse.status(), body: await rejectedResponse.json() });
  await expect(page.getByRole('alert')).toContainText('Unable to save price version. Check for a duplicate or invalid version.');
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR}/pricing-occupied-date.png`, fullPage: true });
  await page.getByLabel('Effective date').fill(newDate);
  const recoveredPatch = page.waitForResponse((response) => response.request().method() === 'PATCH' && response.url().includes(`/api/v1/agent-run-prices/${newPrice.id}`));
  await page.getByRole('button', { name: /save|update/i }).click();
  const recoveredResponse = await recoveredPatch;
  expect(recoveredResponse.status()).toBe(200);
  responseLog.push({ endpoint: '/api/v1/agent-run-prices (browser PATCH recovery)', status: recoveredResponse.status(), body: await recoveredResponse.json() });
  await expect(page.getByText(model, { exact: true })).toBeVisible();
  const recoveredPriceRow = page.locator('tr').filter({ hasText: model }).filter({ hasText: newDate });
  await expect(recoveredPriceRow).toHaveCount(1);
  await recoveredPriceRow.getByRole('button', { name: 'Edit' }).click();
  await expect(page.getByLabel('Model')).toHaveValue(model);
  await page.getByLabel('Effective date').fill(secondDate);
  await page.getByRole('spinbutton', { name: 'output per million', exact: true }).fill('3');
  const updateResponsePromise = page.waitForResponse((response) => response.request().method() === 'PATCH' && response.url().includes(`/api/v1/agent-run-prices/${newPrice.id}`));
  await page.getByRole('button', { name: /save|update/i }).click();
  const updateResponse = await updateResponsePromise;
  expect(updateResponse.status()).toBe(200);
  responseLog.push({ endpoint: '/api/v1/agent-run-prices (browser edit)', status: updateResponse.status(), body: await updateResponse.json() });
  const listed = await request('/api/v1/agent-run-prices');
  const listedBody = await listed.json();
  const saved = listedBody.data.find((item: { model: string }) => item.model === model);
  expect(saved?.effective_date).toContain(secondDate);
  responseLog.push({ endpoint: '/api/v1/agent-run-prices (recovered persisted)', status: listed.status(), body: listedBody });
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR}/pricing-owner.png`, fullPage: true });

  const inviteeLogin = await page.request.post(`${api}/api/v1/auth/login`, { data: { email: 'invitee@example.invalid', password: 'smoke-password-not-a-secret' } });
  const viewerToken = (await inviteeLogin.json()).token as string;
  const memberLogin = await page.request.post(`${api}/api/v1/auth/login`, { data: { email: 'member@example.invalid', password: 'smoke-password-not-a-secret' } });
  const memberAccountToken = (await memberLogin.json()).token as string;
  const memberSelect = await page.request.post(`${api}/api/v1/auth/select-organization`, { headers: { Authorization: `Bearer ${memberAccountToken}` }, data: { organization_id: org } });
  const memberToken = (await memberSelect.json()).token as string;
  const viewerPrices = await request('/api/v1/agent-run-prices', { token: viewerToken });
  expect(viewerPrices.status()).toBe(200);
  const memberPrices = await request('/api/v1/agent-run-prices', { token: memberToken });
  expect(memberPrices.status()).toBe(200);
  const memberWrite = await request('/api/v1/agent-run-prices', { method: 'POST', token: memberToken, data: { ...draft, model: `${model}-member` } });
  expect(memberWrite.status()).toBe(403);
  await page.addInitScript((value) => sessionStorage.setItem('agent-ops-token', value), viewerToken);
  await page.goto('/prices');
  await expect(page.getByText('Read-only: only organization owners and admins can edit prices.')).toBeVisible();
  expect(await page.getByRole('button', { name: 'Create version' }).count()).toBe(0);
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR}/pricing-viewer.png`, fullPage: true });
  await page.addInitScript((value) => sessionStorage.setItem('agent-ops-token', value), memberToken);
  await page.reload();
  await expect(page.getByText('Read-only: only organization owners and admins can edit prices.')).toBeVisible();
  expect(await page.getByRole('button', { name: 'Create version' }).count()).toBe(0);

  const otherLogin = await page.request.post(`${api}/api/v1/auth/login`, { data: { email: 'other@example.invalid', password: 'smoke-password-not-a-secret' } });
  const otherAccountToken = (await otherLogin.json()).token as string;
  const otherSelect = await page.request.post(`${api}/api/v1/auth/select-organization`, { headers: { Authorization: `Bearer ${otherAccountToken}` }, data: { organization_id: process.env.SMOKE_OTHER_ORGANIZATION_ID } });
  const otherToken = (await otherSelect.json()).token as string;
  const otherPrices = await request('/api/v1/agent-run-prices', { token: otherToken });
  expect((await otherPrices.json()).data).toEqual([]);
  const crossTenantCreate = await request('/api/v1/agent-run-prices', { method: 'POST', token: otherToken, data: { ...draft, model: `${model}-other` } });
  expect(crossTenantCreate.status()).toBe(201);
  const ownerPricesAfter = await request('/api/v1/agent-run-prices');
  expect((await ownerPricesAfter.json()).data.some((item: { model: string }) => item.model === `${model}-other`)).toBe(false);

  await page.addInitScript((value) => sessionStorage.setItem('agent-ops-token', value), token);
  await page.goto('/runs');
  await expect(page.getByText('$1.2500 (equivalent API USD $1.2500)')).toBeVisible();
  await expect(page.getByText('$0.0000 (equivalent API USD $2.5000)')).toBeVisible();
  await page.getByLabel(/currency/i).selectOption('VND');
  if (apiRun!.cost_vnd !== undefined) {
    await expect(page.getByText(`VND ${new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 }).format(apiRun!.cost_vnd)}`)).toBeVisible();
    await expect(page.getByText(`VND ${new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 }).format(subscriptionRun!.cost_vnd!)}`)).toBeVisible();
  } else {
    await expect(page.getByText(/unknown/i)).toHaveCount(2);
  }
  await page.screenshot({ path: `${process.env.SMOKE_ARTIFACT_DIR}/runs-vnd.png`, fullPage: true });
  console.log(`Pricing live API responses: ${JSON.stringify(responseLog)}; aggregate rows: ${JSON.stringify(rows)}; browser requests: ${calls.join(' | ')}`);
  expect(calls.some((call) => call.includes('/agent-run-prices'))).toBeTruthy();
});
