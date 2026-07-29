import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildRunGraphSteps,
  formatElapsedLabel,
  formatStepDuration,
  formatTaskDuration,
  matchesRunGraphEntityFilter,
  summarizeGraphStatuses,
} from './runGraphModel.js';

test('formats bounded run graph durations', () => {
  assert.equal(
    formatElapsedLabel('2026-06-08T10:00:00Z', '2026-06-08T11:02:03Z'),
    '1h 2m'
  );
  assert.equal(formatElapsedLabel('invalid', '2026-06-08T11:02:03Z', 'unknown'), 'unknown');
  assert.equal(formatElapsedLabel('0001-01-01T00:00:00Z', '2026-06-08T11:02:03Z', 'unknown'), 'unknown');
});

test('derives step duration from task execution before the supplied fallback', () => {
  assert.equal(
    formatStepDuration({
      name: 'deploy',
      status: 'success',
      depends_on: [],
      duration: '99',
      tasks: [
        {
          task_id: 'task-1',
          step_name: 'deploy',
          task_name: 'release',
          status: 'success',
          task_index: 0,
          started_at: '2026-06-08T10:00:00Z',
          finished_at: '2026-06-08T10:01:30Z',
        },
      ],
    }),
    '1m 30s'
  );
});

test('uses provided terminal step duration instead of stale open task time', () => {
  assert.equal(
    formatStepDuration({
      name: 'preparation',
      status: 'failure',
      depends_on: [],
      duration: '45s',
      tasks: [
        {
          task_id: 'task-1',
          step_name: 'preparation',
          task_name: 'branch-versioning',
          status: 'running',
          task_index: 0,
          started_at: '2026-06-08T10:00:00Z',
        },
      ],
    }),
    '45s'
  );
});

test('does not keep counting terminal task display durations without finish times', () => {
  assert.equal(
    formatTaskDuration(
      {
        task_id: 'task-1',
        step_name: 'preparation',
        task_name: 'branch-versioning',
        status: 'running',
        task_index: 0,
        started_at: '2026-06-08T10:00:00Z',
      },
      'failed'
    ),
    '0s'
  );
});

test('builds displayable graph steps from run details and pipeline task definitions', () => {
  const graph = buildRunGraphSteps({
    steps: [
      {
        name: 'build',
        status: 'success',
        depends_on: [],
        tasks: [
          {
            task_id: 'task-1',
            step_name: 'build',
            task_name: 'compile',
            status: 'success',
            task_index: 0,
            started_at: '2026-06-08T10:00:00Z',
            finished_at: '2026-06-08T10:00:10Z',
          },
          {
            task_id: 'task-2',
            step_name: 'build',
            task_name: 'package',
            status: 'pending',
            task_index: 1,
          },
        ],
      },
      {
        name: 'approve',
        status: 'success',
        depends_on: ['build'],
        tasks: [
          {
            task_id: 'task-3',
            step_name: 'approve',
            task_name: 'approve',
            status: 'success',
            task_index: 0,
          },
        ],
      },
    ],
    pipelineDefinition: {
      steps: [
        {
          name: 'build',
          tasks: [
            { name: 'compile' },
            { name: 'package', depends_on: ['compile'] },
          ],
        },
        { name: 'approve', tasks: [] },
      ],
    },
    childRuns: [{ run_id: 'child', pipeline_name: 'child', status: 'running', parent_step_name: 'build' }],
  });

  assert.equal(graph.length, 2);
  assert.deepEqual(graph[0]?.tasks.map(task => ({ id: task.id, dependsOn: task.dependsOn })), [
    { id: 'compile', dependsOn: [] },
    { id: 'package', dependsOn: ['compile'] },
  ]);
  assert.equal(graph[0]?.childRun?.run_id, 'child');
  assert.equal(graph[1]?.tasks.length, 0);
});

test('summarizes and filters graph entities for search and status controls', () => {
  const summary = summarizeGraphStatuses([
    { status: 'success' },
    { status: 'success' },
    { status: 'failed' },
    { status: 'running' },
  ]);

  assert.equal(summary.success, 2);
  assert.equal(summary.failed, 1);
  assert.equal(matchesRunGraphEntityFilter({ name: 'publish-images', status: 'success' }, { searchQuery: 'publish', statusFilter: 'success' }), true);
  assert.equal(matchesRunGraphEntityFilter({ name: 'publish-images', status: 'success' }, { searchQuery: 'deploy', statusFilter: 'all' }), false);
  assert.equal(matchesRunGraphEntityFilter({ name: 'publish-images', status: 'success' }, { searchQuery: '', statusFilter: 'failed' }), false);
});
