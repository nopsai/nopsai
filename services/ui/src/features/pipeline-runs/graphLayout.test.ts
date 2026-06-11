import assert from 'node:assert/strict';
import test from 'node:test';
import {
  calculateGraphLayout,
  deriveGraphEdgeStatus,
  deriveTaskGraphStatus,
  normalizeGraphStatus,
} from './graphLayout.js';

test('derives graph statuses from run and task execution details', () => {
  assert.equal(normalizeGraphStatus('waiting_approval'), 'running');
  assert.equal(normalizeGraphStatus('failure'), 'failed');
  assert.equal(deriveTaskGraphStatus({ status: 'success' }, 'pending'), 'pending');
  assert.equal(deriveTaskGraphStatus({ status: 'pending' }, 'failure'), 'skipped');
  assert.equal(
    deriveTaskGraphStatus({ status: 'running', started_at: '2026-01-01', finished_at: '2026-01-01', exit_code: 1 }),
    'failed'
  );
  assert.equal(deriveGraphEdgeStatus('success', 'running'), 'running');
});

test('lays out dependency graphs deterministically in both orientations', () => {
  const items = [
    { id: 'build', status: 'success' as const },
    { id: 'test', dependsOn: ['build'], status: 'running' as const },
    { id: 'deploy', dependsOn: ['test'], status: 'pending' as const },
  ];
  const size = () => ({ width: 100, height: 40 });
  const horizontal = calculateGraphLayout(items, size, 20, 10);
  const vertical = calculateGraphLayout(items, size, 20, 10, 'vertical');

  assert.deepEqual(horizontal.nodes.map(node => node.level), [0, 1, 2]);
  assert.deepEqual(horizontal.edges.map(edge => edge.status), ['running', 'running']);
  assert.ok(horizontal.nodes[1].x > horizontal.nodes[0].x);
  assert.ok(vertical.nodes[1].y > vertical.nodes[0].y);
});

test('handles cyclic dependencies without unbounded recursion', () => {
  const layout = calculateGraphLayout(
    [
      { id: 'one', dependsOn: ['two'], status: 'pending' as const },
      { id: 'two', dependsOn: ['one'], status: 'pending' as const },
    ],
    () => ({ width: 80, height: 30 }),
    20,
    10
  );
  assert.equal(layout.nodes.length, 2);
  assert.equal(layout.edges.length, 2);
});
