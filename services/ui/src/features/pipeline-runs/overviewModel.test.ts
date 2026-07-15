import assert from 'node:assert/strict';
import test from 'node:test';
import type { RunListItem } from './contracts.js';
import {
  ALL_PIPELINE_RUN_BRANCHES,
  buildPipelineRunBranchOptions,
  buildPipelineRunNavigationItems,
  buildPipelineRunOverviewMetrics,
  buildPipelineRunTableRows,
  filterPipelineRuns,
  filterPipelineRunsByBranch,
  flattenRunsByBranch,
  normalizeRunSourceFilter,
  normalizeRunStatusFilter,
} from './overviewModel.js';

function run(overrides: Partial<RunListItem> = {}): RunListItem {
  return {
    run_id: 'run-1',
    pipeline_name: 'release',
    status: 'success',
    is_complete: true,
    ...overrides,
  };
}

test('normalizes filters and applies search, source, and status predicates', () => {
  const repository = run({
    run_id: 'repo',
    git_repo_owner: 'acme',
    git_repo_name: 'api',
    git_commit_sha: 'abcdef123456',
  });
  const scheduled = run({
    run_id: 'scheduled',
    status: 'failure',
    trigger_source: 'schedule',
    schedule_id: 'nightly',
    failure_reason: 'ledger mismatch',
  });
  const waiting = run({
    run_id: 'waiting',
    status: 'waiting_approval',
    is_complete: false,
    external_trigger_id: 'promote',
  });

  assert.equal(normalizeRunSourceFilter('repository'), 'repository');
  assert.equal(normalizeRunSourceFilter('unknown'), 'all');
  assert.equal(normalizeRunStatusFilter('attention'), 'attention');
  assert.equal(normalizeRunStatusFilter('waiting_approval'), 'waiting_approval');
  assert.equal(normalizeRunStatusFilter('bad'), 'all');
  assert.deepEqual(filterPipelineRuns([repository, scheduled, waiting], { sourceFilter: 'schedule' }).map(item => item.run_id), ['scheduled']);
  assert.deepEqual(filterPipelineRuns([repository, scheduled, waiting], { statusFilter: 'failure' }).map(item => item.run_id), ['scheduled']);
  assert.deepEqual(filterPipelineRuns([repository, scheduled, waiting], { statusFilter: 'attention' }).map(item => item.run_id), ['scheduled', 'waiting']);
  assert.deepEqual(filterPipelineRuns([repository, scheduled, waiting], { searchTerm: 'abcdef' }).map(item => item.run_id), ['repo']);
});

test('builds overview metrics from real run states', () => {
  const now = Date.parse('2026-07-12T12:00:00Z');
  const runs = [
    run({
      run_id: 'success',
      started_at: '2026-07-12T10:00:00Z',
      finished_at: '2026-07-12T10:04:00Z',
    }),
    run({
      run_id: 'failure',
      status: 'failure',
      started_at: '2026-07-12T11:00:00Z',
      finished_at: '2026-07-12T11:08:00Z',
      failure_reason: 'Deploy failed\nWhy: rollout timed out',
    }),
    run({
      run_id: 'running',
      status: 'running',
      is_complete: false,
      started_at: '2026-07-12T11:58:00Z',
      git_repo_owner: 'acme',
      git_repo_name: 'api',
    }),
    run({
      run_id: 'approval',
      status: 'waiting_approval',
      is_complete: false,
      started_at: '2026-07-12T11:50:00Z',
      external_trigger_name: 'release-bot',
    }),
  ];

  const metrics = buildPipelineRunOverviewMetrics(runs, now);
  assert.equal(metrics.find(metric => metric.id === 'running')?.value, '1');
  assert.equal(metrics.find(metric => metric.id === 'attention')?.note, '1 failed, 1 waiting approval');
  assert.equal(metrics.find(metric => metric.id === 'success-rate')?.value, '50%');
  assert.equal(metrics.find(metric => metric.id === 'median-duration')?.value, '4m 0s');
});

test('builds persistent team and application navigation without drilling away from root teams', () => {
  const teams = [
    { id: 1, name: 'Platform', kind: 'team' as const },
    { id: 2, name: 'platform/api', kind: 'app' as const, parent_id: 1, last_run_at: '2026-07-12T10:00:00Z' },
    { id: 3, name: 'Payments', kind: 'team' as const },
    { id: 4, name: 'Infra', kind: 'team' as const, parent_id: 1 },
    { id: 5, name: 'platform/infra/worker', kind: 'app' as const, parent_id: 4, last_run_at: '2026-07-12T11:00:00Z' },
  ];

  const rootItems = buildPipelineRunNavigationItems(teams, null);
  assert.deepEqual(rootItems.map(item => `${item.label}:${item.level}:${item.expanded ? 'expanded' : 'collapsed'}`), [
    'Payments:0:collapsed',
    'Platform:0:collapsed',
  ]);

  const expandedRootItems = buildPipelineRunNavigationItems(teams, null, '', new Set([1]));
  assert.deepEqual(expandedRootItems.map(item => `${item.label}:${item.level}:${item.expanded ? 'expanded' : 'collapsed'}`), [
    'Payments:0:collapsed',
    'Platform:0:expanded',
    'Infra:1:collapsed',
    'api:1:collapsed',
  ]);

  const platformItems = buildPipelineRunNavigationItems(teams, 1);
  assert.deepEqual(platformItems.map(item => `${item.label}:${item.level}:${item.active ? 'active' : 'idle'}`), [
    'Payments:0:idle',
    'Platform:0:active',
    'Infra:1:idle',
    'api:1:idle',
  ]);

  const infraItems = buildPipelineRunNavigationItems(teams, 4);
  assert.deepEqual(infraItems.map(item => `${item.label}:${item.level}:${item.active ? 'active' : 'idle'}`), [
    'Payments:0:idle',
    'Platform:0:idle',
    'Infra:1:active',
    'worker:2:idle',
    'api:1:idle',
  ]);

  assert.deepEqual(buildPipelineRunNavigationItems(teams, null, 'worker').map(item => item.label), ['Platform', 'Infra', 'worker']);
});

test('builds dynamic application branch options and filters runs by branch', () => {
  const main = run({
    run_id: 'main',
    git_ref: 'refs/heads/main',
    started_at: '2026-07-12T11:00:00Z',
  });
  const mainNewer = run({
    run_id: 'main-newer',
    git_ref: 'refs/heads/main',
    started_at: '2026-07-12T12:00:00Z',
  });
  const feature = run({
    run_id: 'feature',
    git_ref: 'refs/heads/feature/search',
    git_target_ref: 'refs/heads/main',
    started_at: '2026-07-12T10:00:00Z',
  });
  const manual = run({ run_id: 'manual' });

  const options = buildPipelineRunBranchOptions([main, feature, manual, mainNewer]);
  assert.deepEqual(options.map(option => `${option.label}:${option.runCount}`), [
    'main:2',
    'feature/search -> main:1',
    'No branch:1',
  ]);
  assert.deepEqual(filterPipelineRunsByBranch([main, feature, manual, mainNewer], options[0]?.key || '').map(item => item.run_id), ['main', 'main-newer']);
  assert.deepEqual(filterPipelineRunsByBranch([main, feature], ALL_PIPELINE_RUN_BRANCHES).map(item => item.run_id), ['main', 'feature']);
});

test('flattens branch buckets and shapes table rows', () => {
  const older = run({ run_id: 'older', started_at: '2026-07-12T10:00:00Z' });
  const newer = run({
    run_id: 'newer',
    pipeline_name: 'deploy-api',
    pipeline_path: 'platform/api',
    git_repo_owner: 'acme',
    git_repo_name: 'api',
    git_ref: 'refs/heads/main',
    git_commit_sha: 'abcdef123456',
    started_at: '2026-07-12T11:00:00Z',
    finished_at: '2026-07-12T11:02:11Z',
  });

  assert.deepEqual(flattenRunsByBranch({ main: [older, newer], duplicate: [newer] }).map(item => item.run_id), ['newer', 'older']);

  const row = buildPipelineRunTableRows([newer], 5, Date.parse('2026-07-12T12:00:00Z'))[0];
  assert.equal(row?.pipelineName, 'deploy-api');
  assert.equal(row?.pipelineMeta, 'main - abcdef12 - newer');
  assert.equal(row?.scopeName, 'api');
  assert.equal(row?.sourceLabel, 'Application');
  assert.equal(row?.durationLabel, '2m 11s');
  assert.equal(row?.startedLabel, '1h ago');
});
