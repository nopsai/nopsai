import assert from 'node:assert/strict';
import test from 'node:test';
import { buildIgnoredFailureWarning, isIgnoredFailureStatus } from './runWarnings.js';

test('detects ignored failure statuses from steps, tasks, and child runs', () => {
  const warning = buildIgnoredFailureWarning({
    steps: [
      {
        name: 'build',
        status: 'success',
        depends_on: [],
        tasks: [
          {
            task_id: 'task-1',
            step_name: 'build',
            task_name: 'lint',
            status: 'failure (ignored)',
            task_index: 0,
          },
        ],
      },
      {
        name: 'scan',
        status: 'ignored_failure',
        depends_on: [],
        tasks: [],
      },
    ],
    childRuns: [
      {
        run_id: 'child-run',
        pipeline_name: 'integration-child',
        parent_step_name: 'include-integration',
        status: 'failure (ignored)',
      },
    ],
  });

  assert.equal(warning?.count, 3);
  assert.deepEqual(warning?.items, [
    'Task build / lint',
    'Step scan',
    'Included pipeline integration-child from include-integration',
  ]);
  assert.match(warning?.message || '', /3 ignored failures were marked/);
});

test('does not warn for ordinary failed or successful work', () => {
  assert.equal(isIgnoredFailureStatus('failure'), false);
  assert.equal(isIgnoredFailureStatus('success'), false);
  assert.equal(isIgnoredFailureStatus('failure (ignored)'), true);
  assert.equal(
    buildIgnoredFailureWarning({
      steps: [{ name: 'deploy', status: 'success', depends_on: [], tasks: [] }],
    }),
    null
  );
});
