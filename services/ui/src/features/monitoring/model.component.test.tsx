import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  allDirectRuns,
  asRecord,
  buildDailyBuckets,
  buildTeamContext,
  buildTeamMetric,
  buildPipelineMetrics,
  filterRunsByWindow,
  flattenBranchRuns,
  formatDateKey,
  getRunDurationMs,
  normalizeMonitoringActiveRuns,
  normalizeMonitoringRunner,
  normalizeNumber,
  normalizeOptionalNumber,
  normalizeRunnerStatusValue,
  normalizeRunnerSummary,
  normalizeServiceStatus,
  normalizeServiceStatusValue,
  normalizeStatus,
  parseDateMs,
  parseGoDurationMs,
  readOptionalString,
  runsForTeamAndDescendants,
  statusCountsFromSummary,
  summarizeRuns,
  type Team,
  type RunListItem,
} from './model';

afterEach(() => {
  vi.useRealTimers();
});

const teams: Team[] = [
  { id: 1, name: 'Platform' },
  { id: 2, name: 'Release', parent_id: 1 },
];

const successful: RunListItem = {
  run_id: 'run-parent',
  pipeline_name: 'build',
  status: 'success',
  started_at: '2026-06-08T10:00:00Z',
  finished_at: '2026-06-08T10:01:00Z',
  is_complete: true,
};

const failed: RunListItem = {
  run_id: 'run-child',
  pipeline_name: 'release',
  pipeline_path: 'platform',
  status: 'failure',
  started_at: '2026-06-08T11:00:00Z',
  finished_at: '2026-06-08T11:02:00Z',
  is_complete: true,
};

describe('Monitoring model', () => {
  it('aggregates runs across team and branch boundaries', () => {
    const runsByTeam = { 1: [successful], 2: [failed, successful] };
    const context = buildTeamContext(teams);
    const runs = runsForTeamAndDescendants(1, runsByTeam, context.childrenByParent);
    const summary = summarizeRuns(runs);

    expect(context.labels.get(2)).toBe('Platform/Release');
    expect(context.depths.get(2)).toBe(1);
    expect(runs).toHaveLength(2);
    expect(summary).toMatchObject({ totalRuns: 2, successRuns: 1, failedRuns: 1, totalDurationMs: 180000 });
    expect(statusCountsFromSummary(summary)).toMatchObject({ success: 1, failure: 1 });
    expect(buildTeamMetric(teams[0], 'Platform', 0, runs).totalRuns).toBe(2);
    expect(allDirectRuns(runsByTeam)).toHaveLength(2);
    expect(flattenBranchRuns({ main: [successful], release: [failed, successful] }).map(run => run.run_id)).toEqual([
      'run-child',
      'run-parent',
    ]);
  });

  it('builds pipeline and daily metrics', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-06-09T12:00:00Z'));
    const runsByTeam = { 1: [successful], 2: [failed] };
    const context = buildTeamContext(teams);
    const metrics = buildPipelineMetrics([successful, failed], teams, runsByTeam, context.labels);

    expect(metrics[0]).toMatchObject({ id: 'platform/release', failedRuns: 1, teamLabel: 'Platform/Release' });
    expect(buildDailyBuckets([successful, failed], 2).reduce((total, bucket) => total + bucket.runs, 0)).toBe(2);
    expect(filterRunsByWindow([successful, failed], 0)).toHaveLength(2);
    expect(filterRunsByWindow([successful, failed], 1)).toHaveLength(0);
    expect(filterRunsByWindow([successful, failed], 2)).toHaveLength(2);
  });

  it('normalizes service and runner payloads', () => {
    expect(normalizeServiceStatus({ id: 'api', status: 'healthy' })).toMatchObject({
      id: 'api',
      label: 'api',
      status: 'ok',
    });
    expect(normalizeServiceStatusValue('degraded')).toBe('warning');
    expect(normalizeServiceStatusValue('failed')).toBe('error');
    expect(normalizeServiceStatusValue('other')).toBe('unknown');

    const runner = normalizeMonitoringRunner({
      runner_id: 'runner-1',
      label: 'Enterprise runner',
      status: 'healthy',
      runtime: 'k8s',
      capacity: '4',
      active_jobs: 2,
      active_runs: JSON.stringify([{ run_id: 'run-1', pipeline: 'platform/release', parent_step: 'deploy' }]),
    });
    expect(runner).toMatchObject({ runnerId: 'runner-1', status: 'online', runtime: 'kubernetes', capacity: 4 });
    expect(runner.activeRuns[0]).toMatchObject({ runId: 'run-1', pipeline: 'platform/release', parentStep: 'deploy' });
    expect(normalizeMonitoringActiveRuns('invalid')).toEqual([]);
    expect(normalizeMonitoringActiveRuns([{ pipeline: 'missing-id' }, null])).toEqual([]);
    expect(normalizeRunnerStatusValue('paused')).toBe('disabled');
    expect(normalizeRunnerStatusValue('disconnected')).toBe('unreachable');
    expect(normalizeRunnerSummary(null, [runner])).toMatchObject({ total: 1, online: 1, kubernetes: 1, capacity: 4 });
    expect(normalizeRunnerSummary({ total: '2', unreachable: 1, queued_jobs: 3 }, [])).toMatchObject({ total: 2, unreachable: 1, queuedJobs: 3 });
  });

  it('parses statuses, dates, durations, and primitive values', () => {
    expect(normalizeStatus('failure (ignored)', true)).toBe('failure');
    expect(normalizeStatus('pending', false)).toBe('running');
    expect(normalizeStatus('unknown', true)).toBe('pending');
    expect(parseDateMs('invalid')).toBe(0);
    expect(parseGoDurationMs('1h2m3.5s4ms500us')).toBeCloseTo(3723504.5);
    expect(getRunDurationMs({ ...successful, finished_at: undefined, duration: '2m' })).toBe(120000);
    expect(asRecord([])).toBeNull();
    expect(readOptionalString('  value ')).toBe('value');
    expect(normalizeNumber('4')).toBe(4);
    expect(normalizeNumber('invalid')).toBe(0);
    expect(normalizeOptionalNumber(0)).toBeUndefined();
    expect(formatDateKey(new Date(2026, 5, 9))).toBe('2026-06-09');
  });
});
