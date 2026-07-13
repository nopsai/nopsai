import assert from 'node:assert/strict';
import test from 'node:test';
import {
  automationResourceBelongsToPath,
  automationResourceTreeAncestorIDs,
  buildAutomationResourceTree,
  countAutomationResourceTreeItems,
  findAutomationResourceTreeNode,
  normalizeAutomationTreePath,
} from './resourceTreeModel.js';

test('builds reusable team trees for event automation resources', () => {
  const tree = buildAutomationResourceTree([
    { id: 'root-source', label: 'Root source', path: '' },
    { id: 'prod', label: 'Prod', path: 'platform/prod' },
    { id: 'dev', label: 'Dev', path: 'platform/dev' },
  ]);

  assert.equal(countAutomationResourceTreeItems(tree), 3);
  assert.deepEqual(tree.children.map(child => child.fullPath), ['platform']);
  assert.deepEqual(findAutomationResourceTreeNode(tree, 'platform').children.map(child => child.name), ['dev', 'prod']);
  assert.deepEqual(findAutomationResourceTreeNode(tree, 'platform/prod').itemIDs, ['prod']);
  assert.equal(findAutomationResourceTreeNode(tree, 'missing'), tree);
});

test('normalizes and matches event automation team paths', () => {
  assert.equal(normalizeAutomationTreePath('/root/'), '');
  assert.equal(normalizeAutomationTreePath('root'), '');
  assert.deepEqual(automationResourceTreeAncestorIDs('/platform/prod/'), ['platform', 'platform/prod']);
  assert.equal(automationResourceBelongsToPath('platform/prod', ''), true);
  assert.equal(automationResourceBelongsToPath('platform/prod/api', 'platform/prod'), true);
  assert.equal(automationResourceBelongsToPath('', 'platform/prod'), false);
  assert.equal(automationResourceBelongsToPath('shared', 'platform/prod'), false);
});
