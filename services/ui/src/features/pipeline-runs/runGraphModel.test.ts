import assert from 'node:assert/strict';
import test from 'node:test';
import { formatElapsedLabel, formatStepDuration, formatTaskDuration } from './runGraphModel.js';

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
