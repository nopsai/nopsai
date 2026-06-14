import type { AIUsageSummary, RunListItem } from './contracts.js';
import { normalizeStatus } from './statusPresentation.js';

export type Group = {
  id: number;
  name: string;
  kind?: 'group' | 'app' | string;
  parent_id?: number | null;
  description?: string;
  repo_url?: string;
  repository_full_name?: string;
  last_run_at?: string;
  navigation_only?: boolean;
};

export type ParentRunInfo = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
};

export type RunSourceKind = 'repository' | 'external' | 'schedule' | 'manual';

export type RunSourceGroup = {
  kind: RunSourceKind;
  label: string;
  runs: RunListItem[];
  branches?: Record<string, RunListItem[]>;
};

export type RepoSummary = {
  status: string;
  branch: string;
  commit: string;
  pusher: string;
  started_at?: string;
};

const STATUS_PRIORITY = [
  'failure',
  'rejected',
  'failure (ignored)',
  'cancelled',
  'waiting_approval',
  'running',
  'pending',
  'skipped',
  'success',
];

export function formatConfigRepoTimestamp(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function isAppGroup(group: Pick<Group, 'kind' | 'name' | 'repo_url' | 'repository_full_name'>) {
  return group.kind === 'app' || Boolean(group.repo_url || group.repository_full_name) || group.name.includes('/');
}

export function groupDisplayName(group: Pick<Group, 'kind' | 'name' | 'repo_url' | 'repository_full_name'>) {
  if (!isAppGroup(group)) return group.name;
  if (group.kind === 'app' && group.name && !group.name.includes('/')) return group.name;
  const fullName = group.repository_full_name || group.name;
  return fullName.split('/').filter(Boolean).pop() || group.name;
}

export function groupRepositoryURL(group: Pick<Group, 'name' | 'repo_url' | 'repository_full_name'>) {
  const fullName = (group.repository_full_name || group.name).trim().replace(/^\/+|\/+$/g, '');
  if (group.repo_url) return repositoryBrowserURL(group.repo_url, fullName);
  return fullName.includes('/') ? `https://github.com/${fullName}` : '';
}

export function groupRepositoryLabel(group: Pick<Group, 'name' | 'repo_url' | 'repository_full_name'>) {
  return (group.repository_full_name || group.name).trim().replace(/^\/+|\/+$/g, '');
}

export function repositoryBrowserURL(rawURL: string, fallbackFullName: string) {
  const trimmed = rawURL.trim();
  if (!trimmed) return fallbackFullName.includes('/') ? `https://github.com/${fallbackFullName}` : '';
  if (trimmed.startsWith('git@github.com:')) {
    const path = trimmed.slice('git@github.com:'.length).replace(/\.git$/, '').replace(/^\/+|\/+$/g, '');
    return path ? `https://github.com/${path}` : '';
  }
  if (trimmed.startsWith('github.com/')) return `https://${trimmed.replace(/\.git$/, '')}`;
  if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\.git$/, '');
  return fallbackFullName.includes('/') ? `https://github.com/${fallbackFullName}` : trimmed;
}

export function buildRunSourceGroups(runsByBranch: Record<string, RunListItem[]>): RunSourceGroup[] {
  const buckets = new Map<RunSourceKind, RunListItem[]>();
  const repositoryBranches: Record<string, RunListItem[]> = {};

  Object.entries(runsByBranch).forEach(([branch, runs]) => {
    runs.forEach(run => {
      const kind = getRunSourceKind(run);
      const list = buckets.get(kind) || [];
      list.push(run);
      buckets.set(kind, list);

      if (kind === 'repository') {
        const branchRuns = repositoryBranches[branch] || [];
        branchRuns.push(run);
        repositoryBranches[branch] = branchRuns;
      }
    });
  });

  const order: RunSourceKind[] = ['repository', 'schedule', 'external', 'manual'];
  return order.flatMap(kind => {
    const runs = buckets.get(kind) || [];
    if (!runs.length) return [];
    return [{
      kind,
      label: runSourceLabel(kind),
      runs,
      branches: kind === 'repository' ? repositoryBranches : undefined,
    }];
  });
}

export function getRunSourceKind(run: RunListItem): RunSourceKind {
  if (run.trigger_source === 'external_trigger' || Boolean(run.external_trigger_id)) return 'external';
  if (run.trigger_source === 'schedule' || Boolean(run.schedule_id)) return 'schedule';
  if (hasRepositoryContext(run) || (run.trigger_source || '').startsWith('github_')) return 'repository';
  return 'manual';
}

export function hasRepositoryContext(run: Pick<RunListItem, 'git_repo_owner' | 'git_repo_name'>) {
  return Boolean((run.git_repo_owner || '').trim() || (run.git_repo_name || '').trim());
}

export function runSourceLabel(kind: RunSourceKind) {
  switch (kind) {
    case 'repository':
      return 'Git repositories';
    case 'schedule':
      return 'Scheduled runs';
    case 'external':
      return 'External triggers';
    default:
      return 'Manual / Ungrouped';
  }
}

export function buildGroupPath(groupId: number | null, groups: Group[]): Group[] {
  if (!groupId) return [];
  const map = new Map(groups.map(group => [group.id, group]));
  const path: Group[] = [];
  let current = map.get(groupId) || null;
  const visited = new Set<number>();
  while (current && !visited.has(current.id)) {
    visited.add(current.id);
    path.unshift(current);
    const parentId = current.parent_id ?? null;
    current = parentId ? map.get(parentId) || null : null;
  }
  return path;
}

export function getStatusDotClass(status: string | undefined, complete?: boolean) {
  const normalized = normalizeStatus(status, complete);
  if (normalized === 'success') return 'bg-emerald-400';
  if (normalized === 'failure') return 'bg-red-500';
  if (normalized === 'failure (ignored)') return 'bg-amber-500';
  if (normalized === 'running') return 'bg-blue-400';
  if (normalized === 'cancelled') return 'bg-orange-400';
  if (normalized === 'skipped') return 'bg-slate-400';
  return 'bg-gray-500';
}

export function runTimestamp(run?: RunListItem) {
  if (!run) return 0;
  const value = run.started_at || run.finished_at;
  if (!value) return 0;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? 0 : date.getTime();
}

export function buildStatusTimeline(runs: RunListItem[], limit = 36) {
  return [...runs]
    .sort((a, b) => runTimestamp(b) - runTimestamp(a))
    .slice(0, limit)
    .map((run, index) => ({
      key: run.run_id || `${run.trigger_event_id || 'run'}-${index}`,
      status: normalizeStatus(run.status, run.is_complete),
    }));
}

export function getBranchStatusTone(status: string) {
  const normalized = normalizeStatus(status, true);
  if (normalized === 'success') return 'text-green-400';
  if (normalized === 'failure' || normalized === 'failure (ignored)') return 'text-red-400';
  if (normalized === 'rejected') return 'text-rose-400';
  if (normalized === 'waiting_approval') return 'text-cyan-400';
  if (normalized === 'running') return 'text-blue-400';
  return 'text-slate-300';
}

export function runMatchesSearch(run: RunListItem, term: string): boolean {
  if (!term) return true;
  const haystack = [
    run.run_id,
    run.parent_run_id,
    run.pipeline_name,
    run.pipeline_path,
    run.pipeline_version,
    run.pipeline_source,
    run.trigger_source,
    run.schedule_id,
    run.schedule_name,
    run.schedule_path,
    run.git_repo_name,
    run.git_repo_owner,
    run.git_ref,
    run.git_target_ref,
    run.git_commit_sha,
    run.git_pusher_name,
    run.status,
    run.trigger_event_id,
    run.external_trigger_id,
    run.external_trigger_name,
    run.external_trigger_event_type,
    run.external_trigger_caller_type,
    run.external_trigger_caller_id,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  return haystack.includes(term);
}

export function formatBranch(ref?: string) {
  if (!ref) return '—';
  return ref.replace(/^refs\/heads\//, '');
}

export function formatBranchDisplay(source?: string, target?: string) {
  const sourceBranch = formatBranch(source);
  const targetBranch = formatBranch(target);
  return targetBranch && targetBranch !== '—' ? `${sourceBranch} -> ${targetBranch}` : sourceBranch;
}

export function formatRepoLabel(run: RunListItem) {
  const owner = (run.git_repo_owner || '').trim();
  const name = (run.git_repo_name || '').trim();
  if (owner && name) return `${owner}/${name}`;
  if (name) return name;
  if (owner) return owner;
  if (run.external_trigger_name || run.external_trigger_id) {
    return run.external_trigger_name || run.external_trigger_id || 'External trigger';
  }
  if (run.schedule_name || run.schedule_path || run.schedule_id) {
    return run.schedule_name || run.schedule_path || run.schedule_id || 'Scheduled run';
  }
  const path = (run.pipeline_path || '').trim().replace(/^\/+|\/+$/g, '');
  if (path) return path;
  if ((run.trigger_source || '').trim()) return runSourceLabel(getRunSourceKind(run));
  return 'Manual';
}

export function aiUsageTotalTokens(usage?: AIUsageSummary | null) {
  const total = Number(usage?.total_tokens || 0);
  return Number.isFinite(total) && total > 0 ? total : 0;
}

export function formatTokenCount(value?: number | null) {
  const count = Number(value || 0);
  if (!Number.isFinite(count) || count <= 0) return '0 tokens';
  if (count < 1000) return `${count.toLocaleString()} ${count === 1 ? 'token' : 'tokens'}`;
  if (count < 1_000_000) return `${(count / 1000).toFixed(count < 10_000 ? 1 : 0)}k tokens`;
  return `${(count / 1_000_000).toFixed(count < 10_000_000 ? 1 : 0)}M tokens`;
}

export function formatAIUsageBreakdown(usage?: AIUsageSummary | null) {
  const prompt = Number(usage?.prompt_tokens || 0);
  const completion = Number(usage?.completion_tokens || 0);
  if (prompt <= 0 && completion <= 0) return 'No prompt/completion split recorded';
  return `${formatTokenCount(prompt)} prompt / ${formatTokenCount(completion)} completion`;
}

export function buildRunMonitoringLink(run: Pick<RunListItem, 'run_id'> | null | undefined) {
  const params = new URLSearchParams({ tab: 'ai-usage' });
  const runID = (run?.run_id || '').trim();
  if (runID) params.set('runId', runID);
  return `/monitoring?${params.toString()}`;
}

export function getPipelineIdentifier(
  run?: Pick<RunListItem, 'pipeline_name' | 'pipeline_path'> | ParentRunInfo | null
) {
  if (!run) return '';
  const name = (run.pipeline_name || '').trim();
  const path = (run.pipeline_path || '').trim().replace(/^\/+|\/+$/g, '');
  if (!name) return '';
  return path ? `${path}/${name}` : name;
}

export function buildPipelineLink(
  run?: Pick<RunListItem, 'pipeline_name' | 'pipeline_path'> | ParentRunInfo | null
) {
  const identifier = getPipelineIdentifier(run);
  if (!identifier) return '';
  const encoded = identifier.split('/').map((segment: string) => encodeURIComponent(segment)).join('/');
  return `/pipelines/${encoded}`;
}

export function timeAgo(dateInput?: string, now = Date.now()) {
  if (!dateInput) return '—';
  const date = new Date(dateInput);
  if (Number.isNaN(date.getTime())) return '—';
  const seconds = Math.floor((now - date.getTime()) / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function formatTriggerId(id?: string) {
  if (!id) return { display: 'N/A', full: 'N/A' };
  const full = String(id);
  return { display: full.length > 12 ? full.slice(0, 8) : full, full };
}

export function summarizeStatus(runs: RunListItem[]): string {
  if (!runs.length) return 'pending';
  const ranked = runs
    .map(run => normalizeStatus(run.status, run.is_complete))
    .sort((a, b) => STATUS_PRIORITY.indexOf(a) - STATUS_PRIORITY.indexOf(b));
  return ranked[0] || 'pending';
}

export function extractLatestRunSummary(runsByBranch: Record<string, RunListItem[]> | null): RepoSummary | null {
  if (!runsByBranch) return null;
  let latest: RunListItem | null = null;
  let branchName = '';
  Object.entries(runsByBranch).forEach(([branch, runs]) => {
    runs.forEach(run => {
      if (!latest || runTimestamp(run) > runTimestamp(latest)) {
        latest = run;
        branchName = branch;
      }
    });
  });
  if (!latest) return null;
  const resolved = latest as RunListItem;
  return {
    status: normalizeStatus(resolved.status, resolved.is_complete),
    branch: branchName,
    commit: (resolved.git_commit_sha || '').slice(0, 8),
    pusher: resolved.git_pusher_name || '',
    started_at: resolved.started_at,
  };
}
