import assert from 'node:assert/strict';
import test from 'node:test';
import {
  AI_RESOURCE_TREE_ROOT_ID,
  aiResourceTreeAncestorIDs,
  aiResourceTreeFilterForResource,
  buildAIResourceTree,
  countAIResourceTreeResources,
} from './aiResourceTree.js';
import { AI_RESOURCE_TEAM_FILTER_GLOBAL } from './aiResourceTeams.js';

test('builds AI resource trees from team paths and resource ids', () => {
  const tree = buildAIResourceTree(
    ['hosted', 'platform/ml/reasoning', 'platform/security/reviewer', 'global/default'],
    ['platform/ml', 'ops', 'data']
  );

  assert.equal(tree.id, AI_RESOURCE_TREE_ROOT_ID);
  assert.deepEqual(tree.resourceIDs, ['hosted']);
  assert.equal(countAIResourceTreeResources(tree), 4);
  assert.deepEqual(tree.children.map(child => child.fullPath), ['global', 'data', 'ops', 'platform']);
  assert.deepEqual(tree.children[3]?.children.map(child => child.fullPath), ['platform/ml', 'platform/security']);
});

test('derives open ancestors and team filters for selected AI resources', () => {
  assert.deepEqual(aiResourceTreeAncestorIDs('platform/ml'), ['__root__', 'team:platform', 'team:platform/ml']);
  assert.equal(aiResourceTreeFilterForResource('hosted'), AI_RESOURCE_TEAM_FILTER_GLOBAL);
  assert.equal(aiResourceTreeFilterForResource('platform/ml/reasoning'), 'platform/ml');
});
