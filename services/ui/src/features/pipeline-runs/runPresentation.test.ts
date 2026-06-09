import assert from 'node:assert/strict';
import test from 'node:test';
import type { RunListItem } from './contracts.js';
import {
  buildGroupPath,
  buildPipelineLink,
  buildRunSourceGroups,
  buildStatusTimeline,
  extractLatestRunSummary,
  formatBranchDisplay,
  formatRepoLabel,
  formatTriggerId,
  getRunSourceKind,
  groupDisplayName,
  groupRepositoryURL,
  repositoryBrowserURL,
  runMatchesSearch,
  summarizeStatus,
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

test('normalizes repository groups and browser links', () => {
  assert.equal(groupDisplayName({ name: 'platform/api', kind: 'app' }), 'api');
  assert.equal(
    groupRepositoryURL({ name: 'platform/api', repo_url: 'git@github.com:acme/api.git' }),
    'https://github.com/acme/api'
  );
  assert.equal(repositoryBrowserURL('github.com/acme/web.git', ''), 'https://github.com/acme/web');
});

test('groups run sources in stable product order and retains repository branches', () => {
  const repository = run({ run_id: 'repo', git_repo_owner: 'acme', git_repo_name: 'api' });
  const scheduled = run({ run_id: 'scheduled', trigger_source: 'schedule', schedule_id: 'nightly' });
  const external = run({ run_id: 'external', external_trigger_id: 'hook-1' });
  const manual = run({ run_id: 'manual' });

  assert.equal(getRunSourceKind(repository), 'repository');
  assert.deepEqual(
    buildRunSourceGroups({ main: [manual, repository], nightly: [scheduled, external] }).map(group => group.kind),
    ['repository', 'schedule', 'external', 'manual']
  );
  assert.deepEqual(buildRunSourceGroups({ main: [repository] })[0].branches?.main, [repository]);
});

test('builds bounded group paths even when malformed data contains a cycle', () => {
  const groups = [
    { id: 1, name: 'one', parent_id: 2 },
    { id: 2, name: 'two', parent_id: 1 },
  ];
  assert.deepEqual(buildGroupPath(1, groups).map(group => group.id), [2, 1]);
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
  assert.deepEqual(formatTriggerId('1234567890123456'), { display: '12345678', full: '1234567890123456' });
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
