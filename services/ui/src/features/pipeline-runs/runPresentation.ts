import type { AIUsageSummary, RunListItem } from './contracts.js';
import { monitoringTabRoute } from '../monitoring/routes.js';
import { normalizeStatus } from './statusPresentation.js';

export type { Team } from '../../lib/teamModels.js';
export {
  buildTeamPath,
  findTeamByURLValue,
  formatConfigRepoTimestamp,
  normalizeTeamURLValue,
  teamDisplayName,
  teamPathForURL,
  teamRepositoryLabel,
  teamRepositoryURL,
  isAppTeam,
  repositoryBrowserURL,
} from '../../lib/teamModels.js';

export type ParentRunInfo = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
};

export type RunSourceKind = 'repository' | 'external' | 'schedule' | 'manual';

export type RunSourceTeam = {
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
  'timed_out',
  'cancelled',
  'waiting_approval',
  'running',
  'pending',
  'warning',
  'failure (ignored)',
  'skipped',
  'success',
];

const MIN_RUN_TIMESTAMP_MS = Date.UTC(2000, 0, 1);
const GO_ZERO_TIME_PATTERN = /^0001-01-01T00:00:00(?:\.0+)?(?:Z|[+-]00:?00)?$/i;

export function buildRunSourceTeams(runsByBranch: Record<string, RunListItem[]>): RunSourceTeam[] {
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
      return 'Applications';
    case 'schedule':
      return 'Scheduled runs';
    case 'external':
      return 'External triggers';
    default:
      return 'Manual / Unteamed';
  }
}

export function getStatusDotClass(status: string | undefined, complete?: boolean) {
  const normalized = normalizeStatus(status, complete);
  if (normalized === 'success') return 'bg-emerald-400';
  if (normalized === 'warning') return 'bg-amber-500';
  if (normalized === 'failure') return 'bg-red-500';
  if (normalized === 'failure (ignored)') return 'bg-amber-500';
  if (normalized === 'running') return 'bg-blue-400';
  if (normalized === 'cancelled') return 'bg-orange-400';
  if (normalized === 'timed_out') return 'bg-orange-500';
  if (normalized === 'skipped') return 'bg-slate-400';
  return 'bg-gray-500';
}

export function runTimestamp(run?: RunListItem) {
  if (!run) return 0;
  return parseRunTimestamp(runActivityTimestamp(run)) ?? 0;
}

export function parseRunTimestamp(value?: string | null): number | null {
  const text = (value || '').trim();
  if (!text || GO_ZERO_TIME_PATTERN.test(text)) return null;
  const timestamp = Date.parse(text);
  if (!Number.isFinite(timestamp) || timestamp < MIN_RUN_TIMESTAMP_MS) return null;
  return timestamp;
}

export function runStartedTimestamp(run?: Pick<RunListItem, 'started_at'> | null): string | undefined {
  return parseRunTimestamp(run?.started_at) === null ? undefined : run?.started_at;
}

export function runActivityTimestamp(run?: Pick<RunListItem, 'started_at' | 'finished_at' | 'is_complete'> | null): string | undefined {
  if (!run) return undefined;
  const primary = run.is_complete ? run.finished_at : run.started_at;
  const fallback = run.is_complete ? run.started_at : run.finished_at;
  if (parseRunTimestamp(primary) !== null) return primary;
  if (parseRunTimestamp(fallback) !== null) return fallback;
  return undefined;
}

export function formatRunTimestamp(value?: string | null): string {
  return parseRunTimestamp(value) === null ? '—' : (value || '').trim();
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
  if (normalized === 'warning' || normalized === 'failure (ignored)') return 'text-amber-400';
  if (normalized === 'failure') return 'text-red-400';
  if (normalized === 'rejected') return 'text-rose-400';
  if (normalized === 'timed_out') return 'text-orange-400';
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

export function aiUsageSpendUSD(usage?: AIUsageSummary | null) {
  const spend = Number(usage?.spend_usd || 0);
  return Number.isFinite(spend) && spend > 0 ? spend : 0;
}

/**
 * Formats AI spend. Sub-cent amounts keep four decimals so that a run made of
 * many cheap calls does not round to $0.00 and read as free.
 */
export function formatSpendUSD(value?: number | null) {
  const amount = Number(value || 0);
  if (!Number.isFinite(amount) || amount <= 0) return '$0.00';
  const fractionDigits = amount < 0.01 ? 4 : 2;
  return amount.toLocaleString(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
}

/**
 * Describes how complete a spend figure is. An unpriced call contributes nothing
 * to the total, so its existence has to be stated rather than left to inference.
 */
export function formatAIUsageCompleteness(usage?: AIUsageSummary | null) {
  const unpriced = Number(usage?.unpriced_calls || 0);
  if (!Number.isFinite(unpriced) || unpriced <= 0) return '';
  return `${unpriced.toLocaleString()} call${unpriced === 1 ? '' : 's'} not priced`;
}

export function buildRunMonitoringLink(run: Pick<RunListItem, 'run_id'> | null | undefined) {
  const params = new URLSearchParams();
  const runID = (run?.run_id || '').trim();
  if (runID) params.set('runId', runID);
  return monitoringTabRoute('ai-usage', params);
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
  const timestamp = parseRunTimestamp(dateInput);
  if (timestamp === null) return '—';
  const seconds = Math.max(0, Math.floor((now - timestamp) / 1000));
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
    started_at: runStartedTimestamp(resolved),
  };
}
