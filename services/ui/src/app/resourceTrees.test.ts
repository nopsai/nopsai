import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildKnowledgeContextTree,
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

test('builds pipeline tree from explicit groups and pipeline ids', () => {
  const tree = buildPipelineTree(['platform/payments/deploy', 'platform/search/index'], ['platform/security']);

  assert.deepEqual(tree.children.map(child => child.name), ['platform']);
  const platform = tree.children[0];
  assert.deepEqual(platform.children.map(child => child.name), ['payments', 'search', 'security']);
  assert.deepEqual(platform.children[0].pipelineIds, ['platform/payments/deploy']);
});

test('builds scope and knowledge context trees with enterprise folder ordering', () => {
  const scopeTree = buildScopeTree(['', 'platform/payments'], ['platform/security']);
  assert.deepEqual(scopeTree.scopes, ['']);
  assert.deepEqual(scopeTree.children[0].children.map(child => child.name), ['payments', 'security']);

  const knowledgeTree = buildKnowledgeContextTree(
    ['runbook/platform/restart', 'architecture/platform/topology'],
    ['platform/security']
  );
  assert.deepEqual(knowledgeTree.children.slice(0, 2).map(child => child.name), ['architecture', 'guardrail']);
  const architecture = knowledgeTree.children.find(child => child.name === 'architecture');
  assert.equal(architecture?.children.find(child => child.name === 'platform')?.knowledgeContextIds[0], 'architecture/platform/topology');
});
