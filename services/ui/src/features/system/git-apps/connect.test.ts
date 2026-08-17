import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../../lib/api.js';
import { startGitHubAppInstall, startGitHubAppRegistration } from './api.js';
import {
  gitHubAppConnectPayload,
  normalizeGitHubAppPayload,
  readGitHubAppCallbackResult,
} from './model.js';

test('normalizes the connect capability reported by the API', () => {
  const blocked = normalizeGitHubAppPayload({
    app_id: '123456',
    app_slug: 'nopsai-example',
    connect_supported: false,
    connect_blocked_by: 'public_url is not configured',
  });
  assert.equal(blocked.app_slug, 'nopsai-example');
  assert.equal(blocked.connect_supported, false);
  assert.equal(blocked.connect_blocked_by, 'public_url is not configured');

  const ready = normalizeGitHubAppPayload({ connect_supported: true });
  assert.equal(ready.connect_supported, true);
  assert.equal(ready.connect_blocked_by, '');
});

test('requires a GitHub organization name when registering for an organization', () => {
  assert.throws(
    () => gitHubAppConnectPayload({ target: 'organization', organization: '', appName: '' }),
    /Organization/
  );
  assert.throws(
    () => gitHubAppConnectPayload({ target: 'organization', organization: 'bad org', appName: '' }),
    /Organization/
  );
  assert.deepEqual(
    gitHubAppConnectPayload({ target: 'organization', organization: ' acme ', appName: ' NopsAI ' }),
    { target: 'organization', organization: 'acme', app_name: 'NopsAI' }
  );
  assert.deepEqual(
    gitHubAppConnectPayload({ target: 'personal', organization: 'ignored', appName: '' }),
    { target: 'personal', organization: '', app_name: '' }
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
    });
    const install = await startGitHubAppInstall();

    assert.deepEqual(calls.map(call => ({ path: call.path, method: call.method })), [
      { path: '/v1/git-apps/github/register/start', method: 'POST' },
      { path: '/v1/git-apps/github/install/start', method: 'POST' },
    ]);
    assert.equal(JSON.parse(String(calls[0]?.body)).organization, 'acme');
    assert.equal(registration.manifest, '{"name":"NopsAI"}');
    assert.equal(registration.post_url.startsWith('https://github.com/organizations/acme/'), true);
    assert.equal(install.install_url.includes('/apps/nopsai-example/installations/new'), true);
  } finally {
    apiClient.fetch = originalFetch;
  }
});
