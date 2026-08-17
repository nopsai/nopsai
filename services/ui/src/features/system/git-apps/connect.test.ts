import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../../lib/api.js';
import { startGitHubAppInstall, startGitHubAppRegistration } from './api.js';
import {
  gitHubAppConnectPayload,
  gitHubWebhookURLWarning,
  normalizeGitHubAppPayload,
  readGitHubAppCallbackResult,
} from './model.js';

const tunnelWebhook = 'https://live-gecko-national.ngrok-free.app/webhook';

test('normalizes the stored and effective webhook addresses', () => {
  const tunnelled = normalizeGitHubAppPayload({
    app_id: '123456',
    app_slug: 'nopsai-example',
    webhook_url: tunnelWebhook,
    webhook_endpoint: tunnelWebhook,
  });
  assert.equal(tunnelled.app_slug, 'nopsai-example');
  assert.equal(tunnelled.webhook_url, tunnelWebhook);
  assert.equal(tunnelled.webhook_endpoint, tunnelWebhook);

  const unconfigured = normalizeGitHubAppPayload({});
  assert.equal(unconfigured.webhook_url, '');
  assert.equal(unconfigured.webhook_endpoint, '');
});

test('requires a GitHub organization name when registering for an organization', () => {
  assert.throws(
    () => gitHubAppConnectPayload({ target: 'organization', organization: '', appName: '' }, tunnelWebhook),
    /Organization/
  );
  assert.throws(
    () => gitHubAppConnectPayload({ target: 'organization', organization: 'bad org', appName: '' }, tunnelWebhook),
    /Organization/
  );
  assert.deepEqual(
    gitHubAppConnectPayload({ target: 'organization', organization: ' acme ', appName: ' NopsAI ' }, ` ${tunnelWebhook} `),
    { target: 'organization', organization: 'acme', app_name: 'NopsAI', webhook_url: tunnelWebhook }
  );
  assert.deepEqual(
    gitHubAppConnectPayload({ target: 'personal', organization: 'ignored', appName: '' }, tunnelWebhook),
    { target: 'personal', organization: '', app_name: '', webhook_url: tunnelWebhook }
  );
});

// GitHub fetches the webhook URL, so it is the one address that must be public;
// the NopsAI address only has to work in the operator's browser.
test('requires an absolute webhook URL and flags addresses GitHub cannot reach', () => {
  assert.throws(
    () => gitHubAppConnectPayload({ target: 'personal', organization: '', appName: '' }, '  '),
    /Webhook URL is required/
  );
  assert.throws(
    () => gitHubAppConnectPayload({ target: 'personal', organization: '', appName: '' }, '/webhook'),
    /absolute http/
  );

  assert.equal(gitHubWebhookURLWarning(tunnelWebhook), '');
  assert.equal(gitHubWebhookURLWarning(''), '');
  assert.match(gitHubWebhookURLWarning('http://localhost:8081/webhook'), /GitHub cannot reach localhost/);
  assert.match(gitHubWebhookURLWarning('http://git-bot:8081/webhook'), /GitHub cannot reach git-bot/);
  assert.match(
    gitHubWebhookURLWarning('http://nopsai-git-bot.nopsai.svc.cluster.local:8081/webhook'),
    /GitHub cannot reach/
  );
});

test('reads the registration outcome GitHub returns in the query string', () => {
  assert.equal(readGitHubAppCallbackResult(''), null);
  assert.equal(readGitHubAppCallbackResult('?other=1'), null);
  assert.deepEqual(readGitHubAppCallbackResult('?github_app=created'), {
    tone: 'success',
    message: 'GitHub App created. Install it on an account to finish.',
  });
  assert.deepEqual(readGitHubAppCallbackResult('?github_app=installed'), {
    tone: 'success',
    message: 'GitHub App installed and registered.',
  });
  assert.deepEqual(readGitHubAppCallbackResult('?github_app_error=public_url+is+not+configured'), {
    tone: 'error',
    message: 'public_url is not configured',
  });
});

test('starts registration and installation through the Git Apps API routes', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string; body?: BodyInit | null }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), method: init?.method, body: init?.body });
    if (String(input).includes('/register/start')) {
      return Response.json({
        state: 'state-value',
        post_url: 'https://github.com/organizations/acme/settings/apps/new?state=state-value',
        manifest: '{"name":"NopsAI"}',
        app_name: 'NopsAI',
        webhook_endpoint: 'https://nopsai.example.com/webhook',
        expires_at: '2026-06-15T10:00:00Z',
      });
    }
    return Response.json({
      state: 'install-state',
      install_url: 'https://github.com/apps/nopsai-example/installations/new?state=install-state',
      expires_at: '2026-06-15T10:00:00Z',
    });
  };

  try {
    const registration = await startGitHubAppRegistration({
      target: 'organization',
      organization: 'acme',
      appName: 'NopsAI',
    }, tunnelWebhook);
    const install = await startGitHubAppInstall();

    assert.deepEqual(calls.map(call => ({ path: call.path, method: call.method })), [
      { path: '/v1/git-apps/github/register/start', method: 'POST' },
      { path: '/v1/git-apps/github/install/start', method: 'POST' },
    ]);
    assert.equal(JSON.parse(String(calls[0]?.body)).organization, 'acme');
    assert.equal(JSON.parse(String(calls[0]?.body)).webhook_url, tunnelWebhook);
    assert.equal(registration.manifest, '{"name":"NopsAI"}');
    assert.equal(registration.post_url.startsWith('https://github.com/organizations/acme/'), true);
    assert.equal(install.install_url.includes('/apps/nopsai-example/installations/new'), true);
  } finally {
    apiClient.fetch = originalFetch;
  }
});
