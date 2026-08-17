import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../../lib/api.js';
import {
  deleteGitHubAppInstallation,
  fetchGitHubApp,
  fetchGitHubAppInstallationRepositories,
  refreshGitHubAppInstallation,
  saveGitHubApp,
  saveGitHubAppInstallation,
  verifyGitHubAppInstallation,
} from './api.js';
import type { GitHubAppInstallation } from './model.js';

const installation: GitHubAppInstallation = {
  installation_id: '987654',
  account_login: 'nopsai',
  account_type: 'organization',
  enabled: true,
  accessible_repositories: 0,
  connected_triggers: 0,
  status: 'connected',
};

test('loads and saves GitHub App resources with dedicated API routes', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string; body?: BodyInit | null; cache?: RequestCache }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), method: init?.method, body: init?.body, cache: init?.cache });
    return Response.json({
      provider: 'github',
      app_id: '123456',
      private_key_credential_ref: 'credential://system/github/app-private-key',
      webhook_credential_ref: 'credential://system/github/webhook-secret',
      installations: [installation],
    });
  };

  try {
    const app = await fetchGitHubApp();
    await saveGitHubApp({
      appID: '123456',
      privateKeyCredentialRef: 'credential://system/github/app-private-key',
      webhookCredentialRef: 'credential://system/github/webhook-secret',
      webhookURL: 'https://live-gecko-national.ngrok-free.app/webhook',
    }, app.installations);

    assert.deepEqual(calls.map(call => ({ path: call.path, method: call.method })), [
      { path: '/v1/git-apps/github', method: undefined },
      { path: '/v1/git-apps/github', method: 'PUT' },
    ]);
    const savedBody = JSON.parse(String(calls[1]?.body));
    assert.equal(savedBody.provider, 'github');
    assert.equal(savedBody.installations[0].installation_id, '987654');
    assert.equal(Object.hasOwn(savedBody, 'github_installation_id'), false);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('manages GitHub App installations and repository refresh routes', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string; body?: BodyInit | null }> = [];
  apiClient.fetch = async (input, init) => {
    const path = String(input);
    calls.push({ path, method: init?.method, body: init?.body });
    if (path.endsWith('/repositories')) {
      return Response.json([{ id: 1, full_name: 'nopsai/api', owner: 'nopsai', name: 'api', used_by_nopsai: true }]);
    }
    if (init?.method === 'DELETE') {
      return new Response(null, { status: 204 });
    }
    return Response.json(installation);
  };

  try {
    await saveGitHubAppInstallation({
      installationID: '987654',
      accountLogin: 'nopsai',
      accountType: 'organization',
      enabled: true,
    });
    await verifyGitHubAppInstallation('987654');
    await refreshGitHubAppInstallation('987654');
    assert.deepEqual(await fetchGitHubAppInstallationRepositories('987654'), [{
      id: 1,
      full_name: 'nopsai/api',
      owner: 'nopsai',
      name: 'api',
      private: false,
      default_branch: undefined,
      access: undefined,
      used_by_nopsai: true,
    }]);
    await deleteGitHubAppInstallation('987654');

    assert.deepEqual(calls.map(call => ({ path: call.path, method: call.method })), [
      { path: '/v1/git-apps/github/installations', method: 'POST' },
      { path: '/v1/git-apps/github/installations/987654/verify', method: 'POST' },
      { path: '/v1/git-apps/github/installations/987654/refresh', method: 'POST' },
      { path: '/v1/git-apps/github/installations/987654/repositories', method: undefined },
      { path: '/v1/git-apps/github/installations/987654', method: 'DELETE' },
    ]);
    const savedBody = JSON.parse(String(calls[0]?.body));
    assert.equal(savedBody.account_login, 'nopsai');
    assert.equal(Object.hasOwn(savedBody, 'team_path'), false);
    assert.equal(Object.hasOwn(savedBody, 'repository_allowlist'), false);
  } finally {
    apiClient.fetch = originalFetch;
  }
});
