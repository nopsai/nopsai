import assert from 'node:assert/strict';
import { test } from 'node:test';
import { normalizePipelineListPayload } from './api.js';

test('normalizes pipeline list API payloads', () => {
  assert.deepEqual(normalizePipelineListPayload(['platform/build', { id: 'app/deploy', source: 'git' }, { identifier: 'draft/test' }, {}]), [
    { id: 'app/deploy', source: 'git' },
    { id: 'draft/test', source: undefined },
    { id: 'platform/build' },
  ]);
  assert.deepEqual(normalizePipelineListPayload(null), []);
});
