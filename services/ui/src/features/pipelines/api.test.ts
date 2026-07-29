import assert from 'node:assert/strict';
import { test } from 'node:test';
import { normalizePipelineListPayload } from './api.js';

test('normalizes pipeline list API payloads', () => {
  assert.deepEqual(
    normalizePipelineListPayload([
      'platform/build',
      { id: 'app/deploy', source: 'git', version: 'v2', updated_at: '2026-07-24T09:30:00Z' },
      { identifier: 'draft/test' },
      {},
    ]),
    [
      { id: 'app/deploy', source: 'git', version: 'v2', updatedAt: '2026-07-24T09:30:00Z' },
      { id: 'draft/test', source: undefined, version: undefined, updatedAt: undefined },
      { id: 'platform/build' },
    ]
  );
  assert.deepEqual(normalizePipelineListPayload(null), []);
});
