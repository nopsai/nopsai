import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildCronExpression,
  cronFormFromExpression,
  normalizeScheduleMetadata,
  parseVariablesText,
  scheduleRequestFromForm,
  uniqueRunTeamOptions,
} from './model.js';

test('round-trips supported cron modes through schedule form fields', () => {
  const weekly = cronFormFromExpression('30 8 * * 1,3,5');
  assert.equal(weekly.cronMode, 'weekly');
  assert.equal(weekly.cronTime, '08:30');
  assert.equal(weekly.cronWeekday, '1,3,5');
  assert.equal(buildCronExpression({ ...baseForm, ...weekly }), '30 8 * * 1,3,5');
});

test('builds one-time and recurring schedule requests', () => {
  assert.deepEqual(
    scheduleRequestFromForm({
      ...baseForm,
      name: ' Nightly ',
      pipeline: '.nopsai/pipelines/platform/deploy.yaml',
      scope: 'default',
      runTeamPath: '',
      variablesText: 'ENV=prod\nRETRIES=3',
    }),
    {
      path: 'platform',
      name: 'Nightly',
      description: '',
      pipeline: 'platform/deploy',
      schedule_kind: 'cron',
      cron_expression: '0 2 * * *',
      run_at: undefined,
      timezone: 'UTC',
      enabled: true,
      scope: '',
      run_team_path: 'global',
      variables: { ENV: 'prod', RETRIES: '3' },
    }
  );

  const once = scheduleRequestFromForm({
    ...baseForm,
    cronMode: 'once',
    runAtDate: '2026-06-08',
    runAtTime: '09:30',
  });
  assert.equal(once.schedule_kind, 'once');
  assert.equal(once.run_at, '2026-06-08T09:30');
  assert.equal(once.cron_expression, '');
});

test('rejects malformed variable lines', () => {
  assert.throws(() => parseVariablesText('VALID=1\ninvalid line'), /Invalid variable line: 2/);
});

test('normalizes schedule metadata payloads', () => {
  assert.deepEqual(
    normalizeScheduleMetadata(
      ['pipelines/platform/deploy.yaml', { id: 'shared/build' }],
      ['platform', '/shared/'],
      ['default', { scope: 'prod' }],
      [{ name: '/platform/dev/' }]
    ),
    {
      pipelines: ['platform/deploy', 'shared/build'],
      teams: ['platform', 'shared'],
      scopes: ['', 'platform/dev', 'prod'],
    }
  );
});

test('keeps global first in schedule run team options', () => {
  assert.deepEqual(uniqueRunTeamOptions(['data', 'platform', 'global']), ['global', 'data', 'platform']);
});

const baseForm = {
  name: 'Nightly',
  description: '',
  pipeline: 'platform/deploy',
  cronMode: 'daily' as const,
  cronTime: '02:00',
  cronWeekday: '1',
  cronMonthday: '1',
  cronMonth: '1',
  cronMinute: '0',
  intervalValue: '15',
  cron_expression: '0 2 * * *',
  runAtDate: '2026-06-08',
  runAtTime: '09:30',
  timezone: 'UTC',
  enabled: true,
  scope: '',
  runTeamPath: 'global',
  variablesText: '',
};
