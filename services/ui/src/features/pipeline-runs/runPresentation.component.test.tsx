import { describe, expect, it } from 'vitest';
import type { RunListItem } from './contracts';
import {
  buildGroupPath,
  buildPipelineLink,
  buildRunMonitoringLink,
  buildRunSourceGroups,
  buildStatusTimeline,
  extractLatestRunSummary,
  formatAIUsageBreakdown,
  formatBranch,
  formatBranchDisplay,
  formatConfigRepoTimestamp,
  formatRepoLabel,
  formatTokenCount,
  formatTriggerId,
  getBranchStatusTone,
  getPipelineIdentifier,
  getRunSourceKind,
  getStatusDotClass,
  groupDisplayName,
  groupRepositoryLabel,
  groupRepositoryURL,
  hasRepositoryContext,
  isAppGroup,
  repositoryBrowserURL,
  runMatchesSearch,
  runSourceLabel,
  runTimestamp,
  summarizeStatus,
  timeAgo,
} from './runPresentation';

function run(overrides: Partial<RunListItem> = {}): RunListItem {
  return {
    run_id: 'run-1',
    pipeline_name: 'release',
    status: 'success',
    is_complete: true,
    ...overrides,
  };
}

describe('Pipeline Runs presentation', () => {
  it('normalizes repository groups and browser links', () => {
    expect(isAppGroup({ name: 'platform', kind: 'group' })).toBe(false);
    expect(groupDisplayName({ name: 'platform/api', kind: 'app' })).toBe('api');
    expect(groupDisplayName({ name: 'api', kind: 'app' })).toBe('api');
    expect(groupRepositoryLabel({ name: ' platform/api ' })).toBe('platform/api');
    expect(groupRepositoryURL({ name: 'platform/api', repo_url: 'git@github.com:acme/api.git' })).toBe(
      'https://github.com/acme/api'
    );
    expect(repositoryBrowserURL('github.com/acme/web.git', '')).toBe('https://github.com/acme/web');
    expect(repositoryBrowserURL('https://gitlab.example.test/acme/web.git', '')).toBe(
      'https://gitlab.example.test/acme/web'
    );
    expect(repositoryBrowserURL('', 'acme/fallback')).toBe('https://github.com/acme/fallback');
  });

  it('groups sources in stable product order', () => {
    const repository = run({ run_id: 'repo', git_repo_owner: 'acme', git_repo_name: 'api' });
    const scheduled = run({ run_id: 'scheduled', trigger_source: 'schedule', schedule_id: 'nightly' });
    const external = run({ run_id: 'external', external_trigger_id: 'hook-1' });
    const manual = run({ run_id: 'manual' });

    expect(getRunSourceKind(repository)).toBe('repository');
    expect(hasRepositoryContext(repository)).toBe(true);
    expect(buildRunSourceGroups({ main: [manual, repository], nightly: [scheduled, external] }).map(group => group.kind)).toEqual([
      'repository',
      'schedule',
      'external',
      'manual',
    ]);
    expect(buildRunSourceGroups({ main: [repository] })[0].branches?.main).toEqual([repository]);
    expect(runSourceLabel('repository')).toBe('Git repositories');
    expect(runSourceLabel('schedule')).toBe('Scheduled runs');
    expect(runSourceLabel('external')).toBe('External triggers');
    expect(runSourceLabel('manual')).toBe('Manual / Ungrouped');
  });

  it('builds bounded group paths and status presentation', () => {
    const groups = [
      { id: 1, name: 'one', parent_id: 2 },
      { id: 2, name: 'two', parent_id: 1 },
    ];
    expect(buildGroupPath(1, groups).map(group => group.id)).toEqual([2, 1]);
    expect(buildGroupPath(null, groups)).toEqual([]);
    expect(getStatusDotClass('success', true)).toBe('bg-emerald-400');
    expect(getStatusDotClass('failure', true)).toBe('bg-red-500');
    expect(getStatusDotClass('running', false)).toBe('bg-blue-400');
    expect(getBranchStatusTone('waiting_approval')).toBe('text-cyan-400');
    expect(runTimestamp(run({ started_at: 'invalid' }))).toBe(0);
    expect(formatConfigRepoTimestamp()).toBe('-');
    expect(formatConfigRepoTimestamp('invalid')).toBe('invalid');
  });

  it('formats, searches, and links run metadata', () => {
    const item = run({
      pipeline_path: 'platform/core',
      git_repo_owner: 'acme',
      git_repo_name: 'api',
      git_ref: 'refs/heads/feature',
      git_target_ref: 'refs/heads/main',
      external_trigger_caller_id: 'service-42',
    });

    expect(runMatchesSearch(item, 'service-42')).toBe(true);
    expect(runMatchesSearch(item, 'missing')).toBe(false);
    expect(runMatchesSearch(item, '')).toBe(true);
    expect(formatRepoLabel(item)).toBe('acme/api');
    expect(formatRepoLabel(run({ schedule_name: 'Nightly' }))).toBe('Nightly');
    expect(formatRepoLabel(run({ trigger_source: '' }))).toBe('Manual');
    expect(formatBranch()).toBe('—');
    expect(formatBranchDisplay(item.git_ref, item.git_target_ref)).toBe('feature -> main');
    expect(getPipelineIdentifier(item)).toBe('platform/core/release');
    expect(buildPipelineLink(item)).toBe('/pipelines/platform/core/release');
    expect(buildPipelineLink(null)).toBe('');
    expect(buildRunMonitoringLink(item)).toBe('/monitoring?tab=ai-usage&runId=run-1');
    expect(formatTriggerId('1234567890123456')).toEqual({ display: '12345678', full: '1234567890123456' });
    expect(formatTriggerId()).toEqual({ display: 'N/A', full: 'N/A' });
  });

  it('formats LLM token usage for run summaries', () => {
    expect(formatTokenCount(1)).toBe('1 token');
    expect(formatTokenCount(4200)).toBe('4.2k tokens');
    expect(formatAIUsageBreakdown({ prompt_tokens: 300, completion_tokens: 120 })).toBe('300 tokens prompt / 120 tokens completion');
    expect(formatAIUsageBreakdown()).toBe('No prompt/completion split recorded');
  });

  it('summarizes status and latest activity deterministically', () => {
    const successful = run({ run_id: 'success', started_at: '2026-06-08T10:00:00Z' });
    const failed = run({
      run_id: 'failure',
      status: 'failure',
      started_at: '2026-06-08T11:00:00Z',
      git_commit_sha: '1234567890',
      git_pusher_name: 'Ada',
    });

    expect(summarizeStatus([successful, failed])).toBe('failure');
    expect(summarizeStatus([])).toBe('pending');
    expect(buildStatusTimeline([successful, failed], 1)).toEqual([{ key: 'failure', status: 'failure' }]);
    expect(extractLatestRunSummary({ main: [successful], release: [failed] })).toEqual({
      status: 'failure',
      branch: 'release',
      commit: '12345678',
      pusher: 'Ada',
      started_at: '2026-06-08T11:00:00Z',
    });
    expect(extractLatestRunSummary(null)).toBeNull();
    expect(timeAgo('2026-06-08T10:00:00Z', Date.parse('2026-06-08T12:00:00Z'))).toBe('2h ago');
    expect(timeAgo('2026-06-08T11:59:30Z', Date.parse('2026-06-08T12:00:00Z'))).toBe('30s ago');
    expect(timeAgo('invalid')).toBe('—');
  });
});
