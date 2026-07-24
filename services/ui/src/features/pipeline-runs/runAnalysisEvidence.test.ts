import assert from 'node:assert/strict';
import test from 'node:test';
import { buildRunAnalysisPromptContext } from './runAnalysisEvidence.js';
import type { RunLogLine } from './runLogs.js';

test('builds precise failed-task prompt context from configuration and redacted logs', async () => {
  const logs: RunLogLine[] = [
    {
      id: 1,
      timestamp: '2026-07-24T10:00:00Z',
      line: 'starting quality gates',
      source: 'agent',
      level: 'info',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
    {
      id: 2,
      timestamp: '2026-07-24T10:00:02Z',
      line: 'eslint failed: no-unused-vars in services/ui/src/App.tsx token=never-show-this',
      source: 'agent',
      level: 'error',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
    {
      id: 3,
      timestamp: '2026-07-24T10:00:03Z',
      line: 'exit code 1',
      source: 'agent',
      level: 'error',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
  ];

  const context = await buildRunAnalysisPromptContext({
    run_info: {
      run_id: 'run-1',
      pipeline_name: 'nopsai-platform-release',
      status: 'failure',
      is_complete: true,
    },
    steps: [{
      name: 'quality-gates',
      status: 'failure',
      depends_on: ['build'],
      configuration: {
        runtime_pool: 'nopsai',
        image: 'node:22',
        script: 'npm run quality --token=never-show-this',
        tasks: [{
          name: 'quality-gates',
          script: 'npm run lint && npm test',
        }],
      },
      tasks: [{
        task_id: 'task-1',
        step_name: 'quality-gates',
        task_name: 'quality-gates',
        status: 'failure',
        exit_code: 1,
        task_index: 0,
      }],
    }],
    pipeline_definition_yaml: [
      'name: nopsai-platform-release',
      'steps:',
      '  - name: quality-gates',
      '    runtime_pool: nopsai',
      '    script: npm run quality --token=never-show-this',
    ].join('\n'),
  }, async () => logs);

  const serialized = JSON.stringify(context);
  assert.match(serialized, /Failed execution point and configured command context/);
  assert.match(serialized, /Task script/);
  assert.match(serialized, /npm run lint && npm test/);
  assert.match(serialized, /eslint failed: no-unused-vars/);
  assert.match(serialized, /exit code 1/);
  assert.doesNotMatch(serialized, /never-show-this/);
  assert.match(serialized, /token=\[redacted\]/);
});

test('uses failed step and task metadata instead of hard-coded run content', async () => {
  const logs: RunLogLine[] = [
    {
      id: 1,
      timestamp: '2026-07-24T11:00:00Z',
      line: 'unit smoke suite reported a retryable warning',
      source: 'agent',
      level: 'error',
      step_name: 'unit-smoke',
      task_name: 'retry-check',
    },
    {
      id: 2,
      timestamp: '2026-07-24T11:01:00Z',
      line: 'starting database schema apply',
      source: 'agent',
      level: 'info',
      step_name: 'database-migration',
      task_name: 'apply-schema',
    },
    {
      id: 3,
      timestamp: '2026-07-24T11:01:05Z',
      line: 'psql: migration 042_add_invoice_state.sql failed: column invoice_state already exists',
      source: 'agent',
      level: 'error',
      step_name: 'database-migration',
      task_name: 'apply-schema',
    },
    {
      id: 4,
      timestamp: '2026-07-24T11:01:06Z',
      line: 'exit code 1',
      source: 'agent',
      level: 'error',
      step_name: 'database-migration',
      task_name: 'apply-schema',
    },
  ];

  const context = await buildRunAnalysisPromptContext({
    run_info: {
      run_id: 'run-generic-1',
      pipeline_name: 'orders-deploy',
      status: 'failure',
      is_complete: true,
    },
    steps: [
      {
        name: 'unit-smoke',
        status: 'success',
        depends_on: [],
        tasks: [{
          task_id: 'task-smoke',
          step_name: 'unit-smoke',
          task_name: 'retry-check',
          status: 'success',
          exit_code: 0,
          task_index: 0,
        }],
      },
      {
        name: 'database-migration',
        status: 'failure',
        depends_on: ['unit-smoke'],
        configuration: {
          runtime_pool: 'prod-db',
          tasks: [{
            name: 'apply-schema',
            script: 'bin/apply-schema --database orders',
          }],
        },
        tasks: [{
          task_id: 'task-db',
          step_name: 'database-migration',
          task_name: 'apply-schema',
          status: 'failure',
          exit_code: 1,
          task_index: 0,
        }],
      },
    ],
  }, async () => logs);

  const serialized = JSON.stringify(context);
  assert.match(serialized, /database-migration/);
  assert.match(serialized, /apply-schema/);
  assert.match(serialized, /bin\/apply-schema --database orders/);
  assert.match(serialized, /042_add_invoice_state\.sql failed/);
  assert.doesNotMatch(serialized, /retryable warning/);
});

test('keeps the decisive failed-task tail when earlier test output is noisy', async () => {
  const logs: RunLogLine[] = [
    ...Array.from({ length: 90 }, (_, index) => ({
      id: index + 1,
      timestamp: `2026-07-24T10:${String(Math.floor(index / 60)).padStart(2, '0')}:${String(index % 60).padStart(2, '0')}Z`,
      line: `go test ./package-${index} ok`,
      source: 'agent',
      level: 'info',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    })),
    {
      id: 91,
      timestamp: '2026-07-24T10:02:00Z',
      line: 'running scripts/release-tooling-test.sh',
      source: 'agent',
      level: 'info',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
    {
      id: 92,
      timestamp: '2026-07-24T10:02:01Z',
      line: 'packaged generated Helm chart successfully',
      source: 'agent',
      level: 'info',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
    {
      id: 93,
      timestamp: '2026-07-24T10:02:02Z',
      line: 'chart-values.yaml is missing the nopsai-runner repository',
      source: 'agent',
      level: 'error',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
    {
      id: 94,
      timestamp: '2026-07-24T10:02:03Z',
      line: 'scripts/release-tooling-test.sh exited with exit code 1',
      source: 'agent',
      level: 'error',
      step_name: 'quality-gates',
      task_name: 'quality-gates',
    },
  ];

  const context = await buildRunAnalysisPromptContext({
    run_info: {
      run_id: 'run-release-1',
      pipeline_name: 'nopsai-platform-release',
      status: 'failure',
      is_complete: true,
    },
    steps: [{
      name: 'quality-gates',
      status: 'failure',
      depends_on: [],
      configuration: {
        runtime_pool: 'nopsai',
        tasks: [{
          name: 'quality-gates',
          script: 'scripts/release-tooling-test.sh',
        }],
      },
      tasks: [{
        task_id: 'task-release-1',
        step_name: 'quality-gates',
        task_name: 'quality-gates',
        status: 'failure',
        exit_code: 1,
        task_index: 0,
      }],
    }],
  }, async () => logs);

  const serialized = JSON.stringify(context);
  assert.match(serialized, /scripts\/release-tooling-test\.sh/);
  assert.match(serialized, /chart-values\.yaml is missing the nopsai-runner repository/);
  assert.match(serialized, /nopsai-runner/);
});
