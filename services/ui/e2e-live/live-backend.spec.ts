import { expect, test, type Page } from '@playwright/test';

const username = process.env.NOPS_UI_LIVE_USERNAME || '';
const password = process.env.NOPS_UI_LIVE_PASSWORD || '';
const pipelineID = process.env.NOPS_UI_LIVE_PIPELINE_ID || '';
const mutationEnabled = process.env.NOPS_UI_LIVE_MUTATION === 'true';

async function login(page: Page) {
  await page.goto('/#/login');
  await page.getByLabel('Email or username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).not.toHaveURL(/#\/login/);
  await expect(page.getByRole('link', { name: 'Pipeline runs' })).toBeVisible();
}

test.describe('live backend smoke', () => {
  test.skip(!username || !password, 'Set NOPS_UI_LIVE_USERNAME and NOPS_UI_LIVE_PASSWORD to run live smoke tests.');

  test('authenticates and loads authorization-controlled navigation', async ({ page }) => {
    await login(page);
    await expect(page.getByRole('link', { name: 'Pipeline runs' })).toBeVisible();
    const setupStatus = await page.request.get('/v1/setup/status', {
      headers: {
        Authorization: `Bearer ${await page.evaluate(() => localStorage.getItem('nopsai.auth.token') || '')}`,
      },
    });
    expect(setupStatus.ok()).toBeTruthy();
  });

  test('round-trips a dedicated pipeline and starts a smoke run', async ({ page }) => {
    test.skip(!mutationEnabled || !pipelineID, 'Set NOPS_UI_LIVE_MUTATION=true and NOPS_UI_LIVE_PIPELINE_ID to enable mutation smoke.');
    await login(page);
    await page.goto(`/#/pipelines/${pipelineID.split('/').map(encodeURIComponent).join('/')}`);
    await page.getByRole('button', { name: 'Edit' }).click();
    const editor = page.locator('#pipeline-yaml-editor');
    const definition = await editor.inputValue();
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Pipeline saved.')).toBeVisible();

    const token = await page.evaluate(() => localStorage.getItem('nopsai.auth.token') || '');
    const response = await page.request.post('/v1/run', {
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      data: {
        pipeline: pipelineID,
        definition,
      },
    });
    expect(response.ok()).toBeTruthy();
    const payload = await response.json();
    expect(payload.run_id || payload.id).toBeTruthy();
  });
});
