import assert from 'node:assert/strict';
import test from 'node:test';

import { apiClient } from '../../lib/api.js';
import { deleteDashboardPublication } from './api.js';

test('deletes dashboard publication entries with encoded identifiers', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), method: init?.method });
    return new Response(null, { status: 204 });
  };

  try {
    await deleteDashboardPublication('team/ops', 'publication/1');
    assert.deepEqual(calls, [
      {
        path: '/v1/dashboards/team%2Fops/publications/publication%2F1',
        method: 'DELETE',
      },
    ]);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('surfaces dashboard publication delete API errors', async () => {
  const originalFetch = apiClient.fetch;
  apiClient.fetch = async () => new Response('entry is already archived', { status: 404 });

  try {
    await assert.rejects(
      () => deleteDashboardPublication('dashboard-1', 'publication-1'),
      /entry is already archived/
    );
  } finally {
    apiClient.fetch = originalFetch;
  }
});

