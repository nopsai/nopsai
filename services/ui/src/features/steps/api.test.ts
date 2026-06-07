import assert from 'node:assert/strict';
import { test } from 'node:test';
import { normalizeStepListPayload, normalizeStepUsagePayload } from './api.js';

test('normalizes step list API payloads', () => {
  assert.deepEqual(
    normalizeStepListPayload([
      'shared/build',
      { identifier: 'deploy/api', source: 'git', updated_at: '2026-01-01T00:00:00Z' },
      { id: 'draft/test' },
      {},
    ]),
    [
      { id: 'deploy/api', source: 'git', updatedAt: '2026-01-01T00:00:00Z' },
      { id: 'draft/test', source: undefined, updatedAt: undefined },
      { id: 'shared/build' },
    ]
  );
  assert.deepEqual(normalizeStepListPayload(null), []);
});

test('normalizes step usage API payloads', () => {
  assert.deepEqual(
    normalizeStepUsagePayload([
      {
        identifier: 'pipelines/deploy',
        name: 'deploy',
        path: 'pipelines',
        source: 'database',
        description: 'uses step',
      },
      {},
      { identifier: 'pipelines/build' },
    ]),
    [
      {
        identifier: 'pipelines/build',
        name: '',
        path: '',
        source: 'database',
        description: undefined,
      },
      {
        identifier: 'pipelines/deploy',
        name: 'deploy',
        path: 'pipelines',
        source: 'database',
        description: 'uses step',
      },
    ]
  );
  assert.deepEqual(normalizeStepUsagePayload(null), []);
});
