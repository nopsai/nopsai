import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { PipelineSchedule } from './model.js';
import {
  filterSchedules,
  formatScheduleRatio,
  isGitOpsSchedule,
  latestScheduleRunID,
  scheduleDisplayName,
  schedulePathOptions,
  scheduleResourceID,
  scheduleResourcePathLabel,
  summarizeSchedules,
} from './workspaceModel.js';

const schedules: PipelineSchedule[] = [
  {
    id: 'schedule-1',
    path: 'platform/prod',
    name: 'Nightly deploy',
    identifier: 'platform/prod/nightly-deploy',
    pipeline: 'platform/deploy',
    schedule_kind: 'cron',
    cron_expression: '0 2 * * *',
    timezone: 'UTC',
    enabled: true,
    source: 'database',
    next_run_at: '2026-06-16T02:00:00Z',
    latest_run: { run_id: 'run-1', status: 'success' },
  },
  {
    id: 'schedule-2',
    path: '',
    name: '',
    identifier: 'release-window',
    pipeline: 'platform/release',
    schedule_kind: 'once',
    run_at: '2026-06-18T09:30:00Z',
    timezone: 'UTC',
    enabled: false,
    source: 'git',
    managed_by_config_repo: true,
    config_source_path: 'schedules/release-window.yaml',
    last_run_id: 'run-2',
    last_status: 'failed',
  },
];

test('builds stable schedule workspace identifiers and labels', () => {
  assert.equal(scheduleResourceID(schedules[0]), 'platform/prod/nightly-deploy');
  assert.equal(scheduleResourceID(schedules[1]), 'root/release-window');
  assert.equal(scheduleResourcePathLabel(schedules[1]), 'Root');
  assert.equal(scheduleDisplayName(schedules[1]), 'release-window');
  assert.equal(latestScheduleRunID(schedules[1]), 'run-2');
  assert.equal(isGitOpsSchedule(schedules[1]), true);
});

test('uses run team as the schedule browser owner when it differs from the pipeline path', () => {
  const schedule: PipelineSchedule = {
    id: 'schedule-3',
    path: 'general',
    name: 'Release from general pipeline',
    identifier: 'general/release-from-general-pipeline',
    pipeline: 'general/deploy',
    schedule_kind: 'cron',
    cron_expression: '0 1 * * *',
    timezone: 'UTC',
    enabled: true,
    run_team_path: 'prod',
    source: 'database',
  };

  assert.equal(scheduleResourceID(schedule), 'prod/release-from-general-pipeline');
  assert.equal(scheduleResourcePathLabel(schedule), 'prod');
  assert.deepEqual(
    filterSchedules({
      schedules: [schedule],
      searchTerm: '',
      pathFilter: 'prod',
      stateFilter: 'all',
    }).map(item => item.id),
    ['schedule-3']
  );
});

test('filters schedules by search, state, and path', () => {
  assert.deepEqual(
    filterSchedules({
      schedules,
      searchTerm: 'deploy',
      pathFilter: '__all__',
      stateFilter: 'all',
    }).map(schedule => schedule.id),
    ['schedule-1']
  );
  assert.deepEqual(
    filterSchedules({
      schedules,
      searchTerm: '',
      pathFilter: '__all__',
      stateFilter: 'gitops',
    }).map(schedule => schedule.id),
    ['schedule-2']
  );
  assert.deepEqual(
    filterSchedules({
      schedules,
      searchTerm: '',
      pathFilter: 'platform',
      stateFilter: 'enabled',
    }).map(schedule => schedule.id),
    ['schedule-1']
  );
});

test('summarizes visible schedule state for count tabs', () => {
  const summary = summarizeSchedules(schedules, schedules);
  assert.equal(summary.total, 2);
  assert.equal(summary.visible, 2);
  assert.equal(summary.enabled, 1);
  assert.equal(summary.disabled, 1);
  assert.equal(summary.gitops, 1);
  assert.equal(summary.recurring, 1);
  assert.equal(summary.oneTime, 1);
  assert.equal(summary.withNextRun, 1);
  assert.equal(summary.pipelines, 2);
  assert.equal(formatScheduleRatio(summary.enabled, summary.visible), '1/2');
  assert.equal(formatScheduleRatio(0, 0), '0/0');
});

test('includes schedule paths and known teams in tree filter options', () => {
  assert.deepEqual(schedulePathOptions(schedules, ['platform', 'shared/tools']), [
    'root',
    'platform',
    'platform/prod',
    'shared/tools',
  ]);
});
