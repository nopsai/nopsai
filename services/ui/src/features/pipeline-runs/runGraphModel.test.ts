import assert from 'node:assert/strict';
import test from 'node:test';
import { formatElapsedLabel, formatStepDuration } from './runGraphModel.js';

test('formats bounded run graph durations', () => {
  assert.equal(
    formatElapsedLabel('2026-06-08T10:00:00Z', '2026-06-08T11:02:03Z'),
    '1h 2m'
  );
  assert.equal(formatElapsedLabel('invalid', '2026-06-08T11:02:03Z', 'unknown'), 'unknown');
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
