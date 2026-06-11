import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../lib/api.js';
import { fetchRunLogs, requestPipelineRunsJson } from './api.js';

test('normalizes empty and text pipeline-run API responses', async () => {
  const originalFetch = apiClient.fetch;
  const calls: string[] = [];
  apiClient.fetch = async input => {
    calls.push(String(input));
    if (String(input).includes('/logs')) {
      return new Response(JSON.stringify({ unexpected: true }), {
        headers: { 'Content-Type': 'application/json' },
        status: 200,
      });
    }
    return new Response('plain text', { status: 200 });
  };

  try {
    assert.equal(await requestPipelineRunsJson<string>('/v1/runs/plain'), 'plain text');
    assert.deepEqual(await fetchRunLogs('run/1', 7), []);
    assert.deepEqual(calls, [
      '/v1/runs/plain',
      '/v1/runs/run%2F1/logs?since_line=7',
    ]);
  } finally {
    apiClient.fetch = originalFetch;
  }
});

test('surfaces failed pipeline-run API response bodies before fallbacks', async () => {
  const originalFetch = apiClient.fetch;
  apiClient.fetch = async () => new Response('log store unavailable', { status: 503 });

  try {
    await assert.rejects(
      () => fetchRunLogs('run-1', 0),
      /log store unavailable/
    );
  } finally {
    apiClient.fetch = originalFetch;
  }
});
