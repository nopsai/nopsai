import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildPipelineTree,
  buildScopeTree,
  normalizeScopeLabel,
  splitIdentifier,
} from './resourceTrees.js';

test('normalizes scope labels consistently for default and nested scopes', () => {
  assert.equal(normalizeScopeLabel(null), '');
  assert.equal(normalizeScopeLabel('default'), '');
  assert.equal(normalizeScopeLabel('/Team/App/'), 'Team/App');
});

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

test('builds scope tree with enterprise team ordering', () => {
  const scopeTree = buildScopeTree(['', 'platform/payments'], ['platform/security']);
  assert.deepEqual(scopeTree.scopes, ['']);
  assert.deepEqual(scopeTree.children[0].children.map(child => child.name), ['payments', 'security']);
});
