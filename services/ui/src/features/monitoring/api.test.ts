import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../lib/api.js';
import { requestMonitoringJson, sendMonitoringJson } from './api.js';

test('requests monitoring JSON without caching', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; cache?: RequestCache }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), cache: init?.cache });
    return new Response(JSON.stringify({ ok: true }), { status: 200 });
  };

  try {
    assert.deepEqual(await requestMonitoringJson('/v1/monitoring/summary'), { ok: true });
    assert.deepEqual(calls, [{ path: '/v1/monitoring/summary', cache: 'no-store' }]);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('sends monitoring JSON and handles empty responses', async () => {
  const originalFetch = apiClient.fetch;
  const calls: Array<{ path: string; method?: string; body?: BodyInit | null }> = [];
  apiClient.fetch = async (input, init) => {
    calls.push({ path: String(input), method: init?.method, body: init?.body });
    return new Response(null, { status: 204 });
  };

  try {
    assert.equal(await sendMonitoringJson('/v1/monitoring/views/view-1', 'DELETE', { id: 'view-1' }), undefined);
    assert.deepEqual(calls, [{ path: '/v1/monitoring/views/view-1', method: 'DELETE', body: '{"id":"view-1"}' }]);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('surfaces monitoring API response bodies before fallbacks', async () => {
  const originalFetch = apiClient.fetch;
  apiClient.fetch = async () => new Response('monitoring unavailable', { status: 503 });

  try {
    await assert.rejects(() => requestMonitoringJson('/v1/monitoring/summary'), /monitoring unavailable \(503\)/);
  } finally {
    apiClient.fetch = originalFetch;
  }
});
