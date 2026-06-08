import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildGroupContext,
  normalizeMonitoringRunner,
  runsForGroupAndDescendants,
  summarizeRuns,
  type Group,
  type RunListItem,
} from './model.js';

test('aggregates runs across a selected group hierarchy', () => {
  const groups: Group[] = [
    { id: 1, name: 'Platform' },
    { id: 2, name: 'Release', parent_id: 1 },
  ];
  const runsByGroup: Record<number, RunListItem[]> = {
    1: [
      {
        run_id: 'run-parent',
        pipeline_name: 'build',
        status: 'success',
        started_at: '2026-06-08T10:00:00Z',
        finished_at: '2026-06-08T10:01:00Z',
        is_complete: true,
      },
    ],
    2: [
      {
        run_id: 'run-child',
        pipeline_name: 'release',
        status: 'failure',
        started_at: '2026-06-08T11:00:00Z',
        finished_at: '2026-06-08T11:02:00Z',
        is_complete: true,
      },
    ],
  };

  const context = buildGroupContext(groups);
  const runs = runsForGroupAndDescendants(1, runsByGroup, context.childrenByParent);
  const summary = summarizeRuns(runs);

  assert.deepEqual(context.labels.get(2), 'Platform/Release');
  assert.equal(summary.totalRuns, 2);
  assert.equal(summary.successRuns, 1);
  assert.equal(summary.failedRuns, 1);
  assert.equal(summary.totalDurationMs, 180000);
});

test('normalizes runner runtime and active run metadata', () => {
  const runner = normalizeMonitoringRunner({
    runner_id: 'runner-1',
    label: 'Enterprise runner',
    status: 'healthy',
    runtime: 'k8s',
    capacity: '4',
    active_jobs: 2,
    active_runs: JSON.stringify([
      {
        run_id: 'run-1',
        pipeline: 'platform/release',
        parent_step: 'deploy',
      },
    ]),
  });

  assert.equal(runner.status, 'online');
  assert.equal(runner.runtime, 'kubernetes');
  assert.equal(runner.capacity, 4);
  assert.deepEqual(runner.activeRuns, [
    {
      runId: 'run-1',
      pipeline: 'platform/release',
      parentStep: 'deploy',
      triggerId: undefined,
    },
  ]);
});
