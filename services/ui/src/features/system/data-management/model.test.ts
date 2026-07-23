import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  cleanupRequestFromManualForm,
  cleanupRuleLabel,
  cleanupScheduleSourceLabel,
  cleanupSignature,
  formatBytes,
  modeOptions,
  normalizeModeForTarget,
  scheduleFormFromRecord,
  scheduleRequestFromForm,
  sumCounts,
  type CleanupSchedule,
} from './model.js';

test('builds manual cleanup requests and signatures', () => {
  const form = {
    target: 'runs' as const,
    mode: 'keep_last' as const,
    keepLast: '25',
    olderThanDays: '',
    backupBeforeCleanup: true,
  };

  assert.deepEqual(cleanupRequestFromManualForm(form), {
    target: 'runs',
    mode: 'keep_last',
    keep_last: 25,
    older_than_days: 0,
    backup_before_cleanup: true,
  });
  assert.equal(cleanupSignature(form), JSON.stringify(cleanupRequestFromManualForm(form)));
});

test('normalizes cleanup modes for the selected target', () => {
  assert.deepEqual(modeOptions('logs').map(option => option.value), ['older_than_days', 'all_logs']);
  assert.deepEqual(modeOptions('runs').map(option => option.value), ['keep_last', 'older_than_days', 'all_terminal_runs']);
  assert.equal(normalizeModeForTarget('all_logs', 'logs'), 'all_logs');
  assert.equal(normalizeModeForTarget('keep_last', 'logs'), 'older_than_days');
  assert.equal(normalizeModeForTarget('all_logs', 'runs'), 'keep_last');
});

test('maps cleanup schedules to form state and API payloads', () => {
  const schedule: CleanupSchedule = {
    id: 'sched-1',
    name: 'Monthly logs',
    enabled: false,
    target: 'logs',
    mode: 'all_logs',
    keep_last: 10,
    older_than_days: 90,
    backup_before_cleanup: true,
    cron_expression: '0 3 1 * *',
    timezone: 'Europe/Amsterdam',
    source: 'git',
    managed_by_config_repo: true,
    config_source_path: 'setting/system/data-management.yaml',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };

  assert.equal(cleanupScheduleSourceLabel(schedule), 'GitOps');

  const form = scheduleFormFromRecord(schedule);
  assert.deepEqual(form, {
    id: 'sched-1',
    name: 'Monthly logs',
    description: '',
    enabled: false,
    target: 'logs',
    mode: 'all_logs',
    keepLast: '10',
    olderThanDays: '90',
    backupBeforeCleanup: true,
    cronExpression: '0 3 1 * *',
    timezone: 'Europe/Amsterdam',
  });

  assert.deepEqual(scheduleRequestFromForm({ ...form, name: ' Monthly logs ', timezone: '' }), {
    name: 'Monthly logs',
    description: '',
    enabled: false,
    target: 'logs',
    mode: 'all_logs',
    keep_last: 10,
    older_than_days: 90,
    backup_before_cleanup: true,
    cron_expression: '0 3 1 * *',
    timezone: 'UTC',
  });
});

test('formats cleanup labels, counts, and backup sizes', () => {
  assert.equal(cleanupRuleLabel({ target: 'runs', mode: 'keep_last', keep_last: 30 }), 'Runs: keep last 30');
  assert.equal(cleanupRuleLabel({ target: 'logs', mode: 'older_than_days', older_than_days: 14 }), 'Logs: older than 14 day(s)');
  assert.equal(sumCounts({ pipeline_runs: 3, pipeline_run_logs: 7, ignored: Number.NaN }), 10);
  assert.equal(formatBytes(0), '-');
  assert.equal(formatBytes(1024), '1.0 KB');
  assert.equal(formatBytes(10 * 1024), '10 KB');
  assert.equal(cleanupScheduleSourceLabel({ source: 'database', managed_by_config_repo: false }), 'Database');
});
