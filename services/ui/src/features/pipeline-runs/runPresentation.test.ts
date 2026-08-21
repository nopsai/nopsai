import assert from 'node:assert/strict';
import test from 'node:test';
import type { RunListItem } from './contracts.js';
import {
  buildTeamPath,
  buildPipelineLink,
  buildRunMonitoringLink,
  buildRunSourceTeams,
  buildStatusTimeline,
  extractLatestRunSummary,
  findTeamByURLValue,
  formatRunTimestamp,
  formatAIUsageCompleteness,
  formatBranchDisplay,
  formatRepoLabel,
  formatSpendUSD,
  formatTriggerId,
  getRunSourceKind,
  getStatusDotClass,
  runActivityTimestamp,
  teamDisplayName,
  teamRepositoryURL,
  repositoryBrowserURL,
  runStartedTimestamp,
  runTimestamp,
  runMatchesSearch,
  summarizeStatus,
  teamPathForURL,
  timeAgo,
} from './runPresentation.js';

function run(overrides: Partial<RunListItem> = {}): RunListItem {
  return {
    run_id: 'run-1',
    pipeline_name: 'release',
    status: 'success',
    ...overrides,
  };
}

test('normalizes repository teams and browser links', () => {
  assert.equal(teamDisplayName({ name: 'platform/api', kind: 'app' }), 'api');
  assert.equal(
    teamRepositoryURL({ name: 'platform/api', repo_url: 'git@github.com:acme/api.git' }),
    'https://github.com/acme/api'
  );
  assert.equal(repositoryBrowserURL('github.com/acme/web.git', ''), 'https://github.com/acme/web');
});

test('teams run sources in stable product order and retains repository branches', () => {
  const repository = run({ run_id: 'repo', git_repo_owner: 'acme', git_repo_name: 'api' });
  const scheduled = run({ run_id: 'scheduled', trigger_source: 'schedule', schedule_id: 'nightly' });
  const external = run({ run_id: 'external', external_trigger_id: 'hook-1' });
  const manual = run({ run_id: 'manual' });

  assert.equal(getRunSourceKind(repository), 'repository');
  assert.deepEqual(
    buildRunSourceTeams({ main: [manual, repository], nightly: [scheduled, external] }).map(team => team.kind),
    ['repository', 'schedule', 'external', 'manual']
  );
  assert.deepEqual(buildRunSourceTeams({ main: [repository] })[0].branches?.main, [repository]);
});

test('builds bounded team paths even when malformed data contains a cycle', () => {
  const teams = [
    { id: 1, name: 'one', parent_id: 2 },
    { id: 2, name: 'two', parent_id: 1 },
  ];
  assert.deepEqual(buildTeamPath(1, teams).map(team => team.id), [2, 1]);
});

test('uses stable team paths for URL selection', () => {
  const teams = [
    { id: 1, name: 'finance', kind: 'team', path: 'finance' },
    { id: 8, name: 'accountant', kind: 'team', parent_id: 1, path: 'finance/accountant' },
  ];

  assert.equal(teamPathForURL(teams[1], teams), 'finance/accountant');
  assert.equal(findTeamByURLValue('finance/accountant', teams)?.id, 8);
  assert.equal(findTeamByURLValue('8', teams)?.id, 8);
});

test('formats, searches, and links run metadata', () => {
  const item = run({
    pipeline_path: 'platform/core',
    git_repo_owner: 'acme',
    git_repo_name: 'api',
    git_ref: 'refs/heads/feature',
    git_target_ref: 'refs/heads/main',
    external_trigger_caller_id: 'service-42',
  });

  assert.equal(runMatchesSearch(item, 'service-42'), true);
  assert.equal(runMatchesSearch(item, 'missing'), false);
  assert.equal(formatRepoLabel(item), 'acme/api');
  assert.equal(formatBranchDisplay(item.git_ref, item.git_target_ref), 'feature -> main');
  assert.equal(buildPipelineLink(item), '/pipelines/platform/core/release');
  assert.equal(buildRunMonitoringLink(item), '/monitoring/ai-usage?runId=run-1');
  assert.deepEqual(formatTriggerId('1234567890123456'), { display: '12345678', full: '1234567890123456' });
});

test('formats AI spend for run summaries', () => {
  assert.equal(formatSpendUSD(0), '$0.00');
  assert.equal(formatSpendUSD(4.2), '$4.20');
  // A run of many cheap calls must not round away to nothing and read as free.
  assert.equal(formatSpendUSD(0.0004), '$0.0004');
});

test('states when a run spend figure is missing unpriced calls', () => {
  assert.equal(formatAIUsageCompleteness({ spend_usd: 1.2, unpriced_calls: 3 }), '3 calls not priced');
  assert.equal(formatAIUsageCompleteness({ spend_usd: 1.2, unpriced_calls: 1 }), '1 call not priced');
  assert.equal(formatAIUsageCompleteness({ spend_usd: 1.2 }), '');
  assert.equal(formatAIUsageCompleteness(null), '');
});

test('summarizes status and latest activity deterministically', () => {
  const successful = run({ run_id: 'success', started_at: '2026-06-08T10:00:00Z' });
  const failed = run({
    run_id: 'failure',
    status: 'failure',
    started_at: '2026-06-08T11:00:00Z',
    git_commit_sha: '1234567890',
    git_pusher_name: 'Ada',
  });

  assert.equal(summarizeStatus([successful, failed]), 'failure');
  assert.deepEqual(buildStatusTimeline([successful, failed], 1), [{ key: 'failure', status: 'failure' }]);
  assert.deepEqual(extractLatestRunSummary({ main: [successful], release: [failed] }), {
    status: 'failure',
    branch: 'release',
    commit: '12345678',
    pusher: 'Ada',
    started_at: '2026-06-08T11:00:00Z',
  });
  assert.equal(timeAgo('2026-06-08T10:00:00Z', Date.parse('2026-06-08T12:00:00Z')), '2h ago');
});

test('summarizes warning runs below failures but above successes', () => {
  const successful = run({ run_id: 'success', status: 'success', started_at: '2026-06-08T10:00:00Z' });
  const warning = run({ run_id: 'warning', status: 'warning', started_at: '2026-06-08T11:00:00Z' });

  assert.equal(summarizeStatus([successful, warning]), 'warning');
  assert.equal(getStatusDotClass('warning', true), 'bg-amber-500');
});

test('ignores Go zero timestamps from runs that failed before start', () => {
  const zeroStarted = run({
    run_id: 'failed-before-start',
    status: 'failure',
    is_complete: true,
    started_at: '0001-01-01T00:00:00Z',
    finished_at: '2026-06-08T12:00:00Z',
  });

  assert.equal(runStartedTimestamp(zeroStarted), undefined);
  assert.equal(runActivityTimestamp(zeroStarted), '2026-06-08T12:00:00Z');
  assert.equal(runTimestamp(zeroStarted), Date.parse('2026-06-08T12:00:00Z'));
  assert.equal(timeAgo(zeroStarted.started_at, Date.parse('2026-06-08T12:01:00Z')), '—');
  assert.equal(formatRunTimestamp(zeroStarted.started_at), '—');
});
