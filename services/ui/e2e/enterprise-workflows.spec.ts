import { expect, test, type Page, type Route } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const fullCapabilities = {
  pipelines: { write: true, delete: true },
  schedules: { read: true, write: true, delete: true },
  steps: { write: true, delete: true },
  triggers: { read: true, write: true, delete: true },
  external_triggers: { read: true, write: true, delete: true },
  scopes: { read: true, write: true, delete: true },
  knowledge_contexts: { read: true, write: true, delete: true },
  system: {
    config_read: true,
    config_write: true,
    llm_profiles_read: true,
    llm_profiles_write: true,
    mcp_read: true,
    mcp_write: true,
    config_repos_read: true,
    config_repos_write: true,
    dispatcher_read: true,
    dispatcher_write: true,
    access: true,
  },
};

async function fulfillJson(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

async function installApiMocks(
  page: Page,
  options: {
    capabilities?: typeof fullCapabilities | Record<string, unknown>;
    mustChangePassword?: boolean;
    setupCompleted?: boolean;
    onPipelineSave?: (body: string) => void;
  } = {}
) {
  const capabilities = options.capabilities ?? fullCapabilities;
  await page.route('**/v1/**', async route => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;

    if (path === '/v1/setup/preflight') {
      return fulfillJson(route, { ready: true, can_login: true, mode: 'team', checks: [] });
    }
    if (path === '/v1/auth/login') {
      return fulfillJson(route, {
        access_token: 'access-token',
        refresh_token: 'refresh-token',
        roles: ['nopsai-admin'],
        sub: 'admin',
        must_change_password: Boolean(options.mustChangePassword),
      });
    }
    if (path === '/v1/auth/me') {
      return fulfillJson(route, {
        sub: 'admin',
        email: 'admin@example.test',
        roles: ['nopsai-admin'],
        capabilities,
        must_change_password: Boolean(options.mustChangePassword),
      });
    }
    if (path === '/v1/setup/status') {
      return fulfillJson(route, {
        completed: options.setupCompleted ?? true,
        counts: {
          users: 1,
          pipelines: 1,
          steps: 0,
          triggers: 0,
          groups: 1,
          access_grants: 0,
          llm_profiles: 0,
          mcp_servers: 0,
          mcp_profiles: 0,
          knowledge_contexts: 0,
          config_repositories: 0,
        },
        checks: [{ id: 'runtime', label: 'Runtime', status: 'success', blocking: false }],
        github: {},
      });
    }
    if (path === '/v1/pipelines' && url.searchParams.has('include_source')) {
      return fulfillJson(route, [{ id: 'platform/deploy', source: 'database' }]);
    }
    if (path === '/v1/pipelines') {
      return fulfillJson(route, [{ id: 'platform/deploy', source: 'database' }]);
    }
    if (path === '/v1/pipelines/platform/deploy') {
      if (request.method() === 'PUT') {
        options.onPipelineSave?.(request.postData() || '');
      }
      return route.fulfill({
        status: 200,
        contentType: 'application/x-yaml',
        body: 'name: deploy\nversion: v1\ncontainer_image: alpine:3.20\nsteps:\n  - name: build\n    script: echo build\n',
      });
    }
    if (path === '/v1/access/effective-permissions') {
      return fulfillJson(route, { allowed: true });
    }
    if (path === '/v1/groups') return fulfillJson(route, []);
    if (path === '/v1/runs') return fulfillJson(route, url.searchParams.has('groupId') ? {} : []);
    if (path === '/v1/overrides') return fulfillJson(route, []);
    if (path === '/v1/secrets/scopes' || path === '/v1/variables/scopes') return fulfillJson(route, []);
    if (path === '/v1/steps' || path === '/v1/knowledge-contexts') return fulfillJson(route, []);
    if (path.endsWith('/runs') || path.endsWith('/triggers')) return fulfillJson(route, []);
    if (path === '/v1/auth/tokens') return fulfillJson(route, []);
    return fulfillJson(route, []);
  });
}

async function installStoredSession(page: Page, mustChangePassword = false) {
  await page.addInitScript(required => {
    localStorage.setItem('nopsai.auth.token', 'access-token');
    localStorage.setItem('nopsai.auth.refresh', 'refresh-token');
    localStorage.setItem('nopsai.auth.roles', JSON.stringify(['nopsai-admin']));
    localStorage.setItem('nopsai.auth.sub', 'admin');
    if (required) localStorage.setItem('nopsai.auth.mustChangePassword', 'true');
  }, mustChangePassword);
}

test('logs in and enters the authenticated workspace', async ({ page }) => {
  await installApiMocks(page);
  await page.goto('/#/login');
  await page.getByLabel('Email or username').fill('admin');
  await page.getByLabel('Password').fill('secret');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page).toHaveURL(/#\/pipelineruns\/main/);
  await expect(page.getByRole('link', { name: 'Pipeline runs' })).toBeVisible();
});

test('has no serious automatically detectable accessibility violations on login', async ({ page }) => {
  await installApiMocks(page);
  await page.goto('/#/login');
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
  const blocking = results.violations.filter(violation => violation.impact === 'critical' || violation.impact === 'serious');
  expect(blocking).toEqual([]);
});

test('redirects first-login users to the required password change', async ({ page }) => {
  await installStoredSession(page, true);
  await installApiMocks(page, { mustChangePassword: true });
  await page.goto('/#/pipelineruns/main');

  await expect(page).toHaveURL(/#\/profile/);
  await expect(page.getByRole('heading', { name: 'Change password required' })).toBeVisible();
});

test('saves an edited pipeline through the API contract', async ({ page }) => {
  let savedYaml = '';
  await installStoredSession(page);
  await installApiMocks(page, { onPipelineSave: body => (savedYaml = body) });
  await page.goto('/#/pipelines/platform/deploy');

  await page.getByRole('button', { name: 'Edit' }).click();
  const editor = page.locator('#pipeline-yaml-editor');
  await editor.fill('name: deploy\nversion: v2\ncontainer_image: alpine:3.20\nsteps:\n  - name: build\n    script: echo updated\n');
  await page.getByRole('button', { name: 'Save' }).click();

  await expect.poll(() => savedYaml).toContain('version: v2');
  await expect(page.getByText('Pipeline saved.')).toBeVisible();
});

test('renders the first-install setup workflow for system administrators', async ({ page }) => {
  await installStoredSession(page);
  await installApiMocks(page, { setupCompleted: false });
  await page.goto('/#/system/setup');

  await expect(page.getByText('First-install setup')).toBeVisible();
  await expect(page.getByText('Not completed')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Readiness' })).toBeVisible();
});

test('blocks direct navigation and hides links without capability access', async ({ page }) => {
  await installStoredSession(page);
  await installApiMocks(page, {
    capabilities: {
      pipelines: { write: false, delete: false },
      schedules: { read: false, write: false, delete: false },
    },
  });
  await page.goto('/#/schedules');

  await expect(page).toHaveURL(/#\/pipelineruns\/main/);
  await expect(page.getByRole('link', { name: 'Pipeline runs' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Schedules' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'System' })).toHaveCount(0);
});
