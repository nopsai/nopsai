import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildTriggerTree,
  countTriggerTreeSlugs,
  findTriggerTreeNode,
  triggerTreeAncestorIDs,
} from './treeModel.js';

test('builds trigger explorer trees and counts nested trigger slugs', () => {
  const root = buildTriggerTree([
    { slug: 'owner-b/deploy', source: 'git', teamPath: 'team-2' },
    { slug: 'owner-a/app/build', source: 'database', teamPath: 'team-1' },
    { slug: 'owner-a/api', source: 'gitops', teamPath: 'team-1' },
    { slug: 'external-owner/repo', source: 'database', teamPath: 'team-1/app' },
  ]);

  assert.deepEqual(root.children.map(child => child.fullPath), ['external-owner', 'owner-a', 'owner-b']);
  assert.equal(countTriggerTreeSlugs(root), 4);
  assert.equal(countTriggerTreeSlugs(findTriggerTreeNode(root, 'owner-a')), 2);
  assert.deepEqual(findTriggerTreeNode(root, 'owner-a', 'team-1').triggerSlugs, ['owner-a/api']);
  assert.deepEqual(findTriggerTreeNode(root, 'owner-a/app', 'team-1').triggerSlugs, ['owner-a/app/build']);
  assert.deepEqual(findTriggerTreeNode(root, 'external-owner', 'team-1/app').triggerSlugs, ['external-owner/repo']);
  assert.equal(findTriggerTreeNode(root, 'missing'), root);
  assert.deepEqual(triggerTreeAncestorIDs('owner-a/app', 'team-1'), [
    'owner-a',
    'owner-a/app',
    'owner-a/app::team::team-1',
  ]);
});

test('keeps repository owners distinct from NopsAI team ownership', () => {
  const root = buildTriggerTree([
    { slug: 'external/repo', source: 'database', teamPath: 'platform' },
  ]);

  assert.equal(findTriggerTreeNode(root, 'platform'), root);
  assert.deepEqual(findTriggerTreeNode(root, 'external', 'platform').triggerSlugs, ['external/repo']);
});
