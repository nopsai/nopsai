import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildPipelineTree,
  splitIdentifier,
} from './resourceTrees.js';

test('splits resource identifiers into display name and path', () => {
  assert.deepEqual(splitIdentifier('platform/payments/deploy%20api'), {
    name: 'deploy api',
    path: 'platform/payments',
  });
});

test('builds pipeline tree from explicit teams and pipeline ids', () => {
  const tree = buildPipelineTree(['platform/payments/deploy', 'platform/search/index'], ['platform/security']);

  assert.deepEqual(tree.children.map(child => child.name), ['platform']);
  const platform = tree.children[0];
  assert.deepEqual(platform.children.map(child => child.name), ['payments', 'search', 'security']);
  assert.deepEqual(platform.children[0].pipelineIds, ['platform/payments/deploy']);
});
