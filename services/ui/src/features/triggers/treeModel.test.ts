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
  ]);

  assert.deepEqual(root.children.map(child => child.fullPath), ['owner-a', 'owner-b']);
  assert.equal(countTriggerTreeSlugs(root), 3);
  assert.equal(countTriggerTreeSlugs(findTriggerTreeNode(root, 'owner-a')), 2);
  assert.deepEqual(
    findTriggerTreeNode(root, 'owner-a/app').children.find(child => child.kind === 'team' && child.teamPath === 'team-1')?.triggerSlugs,
    ['owner-a/app/build']
  );
  assert.equal(findTriggerTreeNode(root, 'missing'), root);
  assert.deepEqual(triggerTreeAncestorIDs('owner-a/app'), ['owner-a', 'owner-a/app']);
});
