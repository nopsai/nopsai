import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../lib/api.js';
import {
  deleteGitWebhookSource,
  fetchGitWebhookDeliveries,
  fetchGitWebhookSource,
  fetchGitWebhookSources,
  fetchGitWebhookSourceTeamPaths,
  saveGitWebhookSource,
} from './api.js';
import type { GitWebhookSourceRequest } from './model.js';

const request: GitWebhookSourceRequest = {
  id: 'gitlab/platform',
  name: 'GitLab Platform',
  description: '',
  provider: 'gitlab',
  enabled: true,
  team_path: 'platform',
  auth_mode: 'static_token',
  credential_ref: 'credential://system/webhooks/gitlab-platform',
  repository_allowlist: ['platform/*'],
  rate_limit: { per_minute: 120 },
};

test('loads Git webhook source collections and details with encoded identifiers', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; cache?: RequestCache }> = [];
  apiClient.fetch = async (input, init) => {
    const path = String(input);
    calls.push({ path, cache: init?.cache });
    if (path.endsWith('/deliveries')) {
      return Response.json([{ id: 'delivery-1' }]);
    }
    if (path === '/v1/git-webhook-sources') {
      return Response.json([{ id: 'source-1' }]);
    }
    return Response.json({ id: 'gitlab/platform' });
  };

  try {
    assert.deepEqual(await fetchGitWebhookSources(), [{ id: 'source-1' }]);
    assert.deepEqual(await fetchGitWebhookSource('gitlab/platform'), { id: 'gitlab/platform' });
    assert.deepEqual(await fetchGitWebhookDeliveries('gitlab/platform'), [{ id: 'delivery-1' }]);
    assert.deepEqual(calls, [
      { path: '/v1/git-webhook-sources', cache: 'no-store' },
      { path: '/v1/git-webhook-sources/gitlab%2Fplatform', cache: 'no-store' },
      {
        path: '/v1/git-webhook-sources/gitlab%2Fplatform/deliveries',
        cache: 'no-store',
      },
    ]);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('normalizes malformed Git webhook source collection responses', async () => {
  const originalFetch = apiClient.fetch;
  apiClient.fetch = async () => Response.json({ unexpected: true });

  try {
    assert.deepEqual(await fetchGitWebhookSources(), []);
    assert.deepEqual(await fetchGitWebhookDeliveries('source-1'), []);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('loads selectable team paths for Git webhook source ownership', async () => {
  const originalFetch = apiClient.fetch;
  apiClient.fetch = async input => {
    assert.equal(String(input), '/v1/access/teams');
    return Response.json([{ id: 'platform' }, { name: '/platform/prod' }]);
  };

  try {
    assert.deepEqual(await fetchGitWebhookSourceTeamPaths(), ['platform', 'platform/prod']);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('creates and updates Git webhook sources with JSON requests', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string; body?: BodyInit | null }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), method: init?.method, body: init?.body });
    return Response.json(request);
  };

  try {
    await saveGitWebhookSource(request);
    await saveGitWebhookSource(request, 'gitlab/platform');
    assert.deepEqual(calls, [
      {
        path: '/v1/git-webhook-sources',
        method: 'POST',
        body: JSON.stringify(request),
      },
      {
        path: '/v1/git-webhook-sources/gitlab%2Fplatform',
        method: 'PUT',
        body: JSON.stringify(request),
      },
    ]);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('deletes Git webhook sources and surfaces API response bodies', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), method: init?.method });
    return new Response(null, { status: 204 });
  };

  try {
    await deleteGitWebhookSource('gitlab/platform');
    assert.deepEqual(calls, [{
      path: '/v1/git-webhook-sources/gitlab%2Fplatform',
      method: 'DELETE',
    }]);

    apiClient.fetch = async () => new Response('source is GitOps managed', { status: 409 });
    await assert.rejects(
      () => deleteGitWebhookSource('source-1'),
      /source is GitOps managed/
    );

    apiClient.fetch = async () => new Response('', { status: 503 });
    await assert.rejects(
      () => fetchGitWebhookSources(),
      /Request failed \(503\)/
    );
  } finally {
    apiClient.fetch = originalFetch;
  }
});
