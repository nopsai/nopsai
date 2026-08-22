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

const localKeycloakProvider = {
  id: 'keycloak',
  type: 'oidc',
  display_name: 'Local Keycloak',
  scopes: ['openid', 'email', 'profile', 'teams'],
  allowed_email_domains: ['example.com'],
};

const localAuthProviders = {
  local_enabled: true,
  oidc_enabled: false,
  providers: [],
};

const populatedRunDetail = {
  run_info: {
    run_id: 'run-1',
    pipeline_name: 'Enterprise pipeline',
    pipeline_path: 'platform',
    pipeline_version: 'v1',
    pipeline_source: 'database',
    status: 'success',
    is_complete: true,
    started_at: '2026-06-08T10:00:00Z',
    finished_at: '2026-06-08T10:02:00Z',
    duration: '2m',
    git_repo_owner: 'example',
    git_repo_name: 'platform',
    git_ref: 'refs/heads/main',
    git_commit_sha: 'abc123def456',
    git_pusher_name: 'Release Bot',
  },
  steps: [
    {
      name: 'build',
      status: 'success',
      depends_on: [],
      started_at: '2026-06-08T10:00:00Z',
      finished_at: '2026-06-08T10:01:00Z',
      tasks: [
        {
          task_id: 'task-1',
          step_name: 'build',
          task_name: 'compile',
          status: 'success',
          task_index: 0,
          started_at: '2026-06-08T10:00:00Z',
          finished_at: '2026-06-08T10:01:00Z',
        },
      ],
    },
    {
      name: 'deploy',
      status: 'failure',
      depends_on: ['build'],
      started_at: '2026-06-08T10:01:00Z',
      finished_at: '2026-06-08T10:02:00Z',
      tasks: [
        {
          task_id: 'task-2',
          step_name: 'deploy',
          task_name: 'publish',
          status: 'failure',
          exit_code: 1,
          task_index: 0,
          started_at: '2026-06-08T10:01:00Z',
          finished_at: '2026-06-08T10:02:00Z',
        },
      ],
    },
  ],
  pipeline_definition: {
    name: 'Enterprise pipeline',
    version: 'v1',
    steps: [
      { name: 'build', tasks: [{ name: 'compile' }] },
      { name: 'deploy', depends_on: ['build'], tasks: [{ name: 'publish', depends_on: ['compile'] }] },
    ],
  },
  pipeline_definition_yaml: 'name: Enterprise pipeline\nversion: v1\nsteps:\n  - name: build\n  - name: deploy\n',
  child_runs: [],
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
    authProviders?: {
      local_enabled: boolean;
      oidc_enabled: boolean;
      providers: Array<typeof localKeycloakProvider>;
    };
    discoverProvider?: typeof localKeycloakProvider | null;
    sessionExchangeResponse?: Record<string, unknown>;
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
    if (path === '/v1/auth/providers') {
      return fulfillJson(route, options.authProviders ?? localAuthProviders);
    }
    if (path === '/v1/auth/discover') {
      const provider = options.discoverProvider;
      return fulfillJson(route, provider ? { found: true, provider } : { found: false });
    }
    if (path === '/v1/auth/session/exchange') {
      return fulfillJson(route, options.sessionExchangeResponse ?? {
        access_token: 'sso-access-token',
        refresh_token: 'sso-refresh-token',
        roles: ['viewer'],
        sub: 'sso-user',
        must_change_password: false,
      });
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
          teams: 1,
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
    if (path === '/v1/teams') return fulfillJson(route, []);
    if (path === '/v1/runs/run-1/approvals') return fulfillJson(route, []);
    if (path === '/v1/runs/run-1/logs') {
      return fulfillJson(route, [
        {
          id: 1,
          timestamp: '2026-06-08T10:00:10Z',
          line: '{"level":"info","step":"build","message":"compiled enterprise workspace"}',
        },
        {
          id: 2,
          timestamp: '2026-06-08T10:01:10Z',
          line: '{"level":"error","step":"deploy","message":"deployment smoke failure"}',
        },
      ]);
    }
    if (path === '/v1/runs/run-1') return fulfillJson(route, populatedRunDetail);
    if (path === '/v1/runs') return fulfillJson(route, url.searchParams.has('teamId') ? {} : []);
    if (path === '/v1/overrides') return fulfillJson(route, []);
    if (path === '/v1/secrets/scopes' || path === '/v1/variables/scopes') return fulfillJson(route, []);
    if (path === '/v1/steps' || path === '/v1/knowledge-contexts') return fulfillJson(route, []);
    if (path.endsWith('/runs') || path.endsWith('/triggers')) return fulfillJson(route, []);
    if (path === '/v1/auth/tokens') return fulfillJson(route, []);
    return fulfillJson(route, []);
  });
}

async function expectNoBlockingAxeViolations(page: Page, include?: string) {
  const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']);
  if (include) builder.include(include);
  const results = await builder.analyze();
  const blocking = results.violations
    .filter(violation => violation.impact === 'critical' || violation.impact === 'serious')
    .map(violation => ({
      id: violation.id,
      impact: violation.impact,
      targets: violation.nodes.map(node => node.target),
    }));
  expect(blocking).toEqual([]);
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

test('starts direct enterprise provider login with the return path preserved', async ({ page }) => {
  await installApiMocks(page, {
    authProviders: {
      local_enabled: true,
      oidc_enabled: true,
      providers: [localKeycloakProvider],
    },
  });
  await page.goto('/#/login');

  const startRequest = page.waitForRequest(request => {
    const url = new URL(request.url());
    return url.pathname === '/v1/auth/oidc/keycloak/start';
  });
  await page.getByRole('button', { name: 'Continue with Local Keycloak' }).click();

  const request = await startRequest;
  const url = new URL(request.url());
  expect(url.searchParams.get('return_to')).toBe('/pipelineruns/main');
});

test('falls back to local password login when SSO email discovery has no match', async ({ page }) => {
  await installApiMocks(page, {
    authProviders: {
      local_enabled: true,
      oidc_enabled: true,
      providers: [localKeycloakProvider],
    },
    discoverProvider: null,
  });
  await page.goto('/#/login');

  await page.getByLabel('Company email').fill('teammate@unknown.example');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();

  await expect(page.getByLabel('Password')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign in' })).toBeEnabled();
});

test('exchanges an SSO callback session code and stores the Nopsai session', async ({ page }) => {
  await installApiMocks(page, {
    authProviders: {
      local_enabled: true,
      oidc_enabled: true,
      providers: [localKeycloakProvider],
    },
    sessionExchangeResponse: {
      access_token: 'sso-access-token',
      refresh_token: 'sso-refresh-token',
      roles: ['developer'],
      sub: 'sso-operator',
      must_change_password: false,
    },
  });

  await page.goto('/#/login?session_code=callback-code');

  await expect(page).toHaveURL(/#\/pipelineruns\/main/);
  await expect.poll(() => page.evaluate(() => localStorage.getItem('nopsai.auth.token'))).toBe('sso-access-token');
  await expect.poll(() => page.evaluate(() => localStorage.getItem('nopsai.auth.refresh'))).toBe('sso-refresh-token');
});

test('has no serious automatically detectable accessibility violations on login', async ({ page }) => {
  await installApiMocks(page);
  await page.goto('/#/login');
  await expectNoBlockingAxeViolations(page);
});

test('has no serious automatically detectable accessibility violations in the authenticated workspace', async ({ page }) => {
  await installStoredSession(page);
  await installApiMocks(page);
  await page.goto('/#/pipelineruns/main');
  const pipelineRunsLink = page.getByRole('link', { name: 'Pipeline runs' });
  await expect(pipelineRunsLink).toBeVisible();
  await expect(pipelineRunsLink).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true');
  const resizer = page.getByRole('separator', { name: 'Resize sidebar' });
  await resizer.focus();
  const widthBeforeKeyboardResize = Number(await resizer.getAttribute('aria-valuenow'));
  await resizer.press('ArrowRight');
  await expect(resizer).toHaveAttribute('aria-valuenow', String(Math.min(520, widthBeforeKeyboardResize + 16)));

  const userMenuButton = page.getByRole('button', { name: 'Open user menu for admin' });
  await userMenuButton.click();
  await expect(page.getByRole('menu', { name: 'User menu' })).toBeVisible();
  await expectNoBlockingAxeViolations(page, '#user-menu');
  await page.keyboard.press('Escape');
  await expect(userMenuButton).toBeFocused();

  await expectNoBlockingAxeViolations(page);
});

test('audits shared workflow dialogs and keyboard-accessible YAML editing', async ({ page }) => {
  await installStoredSession(page);
  await installApiMocks(page);
  await page.goto('/#/pipelines');

  const createButton = page.getByRole('button', { name: 'Create new pipeline' });
  await createButton.click();
  const dialog = page.getByRole('dialog', { name: 'Create pipeline' });
  await expect(dialog).toBeVisible();
  await expect(page.getByLabel('Pipeline Name')).toBeFocused();
  await expectNoBlockingAxeViolations(page, '#pipelines-new-modal');

  await dialog.getByRole('button', { name: 'Create', exact: true }).focus();
  await page.keyboard.press('Tab');
  await expect(dialog.getByRole('button', { name: 'Close' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(createButton).toBeFocused();

  await page.goto('/#/pipelines/platform/deploy');
  await page.getByRole('button', { name: 'Edit' }).click();
  const editor = page.getByRole('textbox', { name: 'Pipeline YAML editor' });
  await editor.focus();
  await editor.press('Control+Space');
  const autocomplete = page.getByRole('listbox');
  await expect(autocomplete).toBeVisible();
  await editor.press('ArrowDown');
  await expect(editor).toHaveAttribute('aria-activedescendant', 'pipeline-editor-autocomplete-option-1');
  await expectNoBlockingAxeViolations(page, '#editor-container');
  await editor.press('Escape');
  await expect(autocomplete).toHaveCount(0);
  await expect(editor).toBeFocused();
});

test('audits keyboard graph interaction and a fully populated log dialog', async ({ page }) => {
  await installStoredSession(page);
  await installApiMocks(page);
  await page.goto('/#/pipelineruns/main?run=run-1');

  await expect(page.getByText('Enterprise pipeline', { exact: true }).first()).toBeVisible();
  const graph = page.getByRole('region', { name: 'Pipeline run graph' });
  await expect(graph).toBeVisible();
  const buildStep = graph.getByRole('button', { name: /Expand build step/ });
  await buildStep.focus();
  await buildStep.press('Enter');
  await expect(graph.getByRole('button', { name: /Open logs for compile task/ })).toBeVisible();
  await expectNoBlockingAxeViolations(page, '[aria-label="Pipeline run graph"]');

  const logsButton = page.getByRole('button', { name: 'Logs', exact: true });
  await logsButton.click();
  const logsDialog = page.getByRole('dialog', { name: 'Run Logs for Enterprise pipeline' });
  await expect(logsDialog).toBeVisible();
  await expect(page.getByRole('searchbox', { name: 'Search run logs' })).toBeFocused();
  await expect(page.getByText(/compiled enterprise workspace/)).toBeVisible();
  await expect(page.getByText(/deployment smoke failure/)).toBeVisible();
  await expectNoBlockingAxeViolations(page, '[role="dialog"][aria-labelledby="run-logs-title"]');

  await logsDialog.getByRole('button', { name: 'Reset filters' }).focus();
  await page.keyboard.press('Tab');
  await expect(logsDialog.getByRole('button', { name: 'Download' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(logsDialog).toHaveCount(0);
  await expect(logsButton).toBeFocused();
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

test('TEMP schedule form shot', async ({ page }) => {
  await installStoredSession(page);
  await installApiMocks(page);
  await page.addInitScript(() => window.localStorage.setItem('theme', 'dark'));
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('/#/schedules');
  await page.waitForTimeout(900);
  const buttons = await page.getByRole('button').all();
  for (const b of buttons) {
    const label = (await b.getAttribute('aria-label')) || (await b.textContent()) || '';
    if (/new schedule|create schedule|add schedule/i.test(label)) { await b.click(); break; }
  }
  await page.waitForTimeout(800);
  await page.screenshot({ path: 'shot-schedule-form.png' });
});
