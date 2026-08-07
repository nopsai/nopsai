import type { RunListItem } from './contracts.js';
import {
  formatBranchDisplay,
  formatRepoLabel,
  getRunSourceKind,
  isAppTeam,
  parseRunTimestamp,
  runMatchesSearch,
  runStartedTimestamp,
  runTimestamp,
  teamDisplayName,
  teamPathForURL,
  timeAgo,
  type RunSourceKind,
  type Team,
} from './runPresentation.js';
import { normalizeStatus } from './statusPresentation.js';

export type PipelineRunSourceFilter = 'all' | RunSourceKind;
export type PipelineRunStatusFilter = 'all' | 'attention' | 'running' | 'failure' | 'waiting_approval' | 'success' | 'pending';

export type PipelineRunFilterOptions = {
  searchTerm?: string;
  sourceFilter?: PipelineRunSourceFilter;
  statusFilter?: PipelineRunStatusFilter;
};

export type PipelineRunOverviewMetric = {
  id: 'running' | 'attention' | 'success-rate' | 'median-duration';
  label: string;
  value: string;
  note: string;
  tone: 'blue' | 'red' | 'green' | 'amber';
};

export type PipelineRunNavigationItem = {
  id: number;
  label: string;
  path: string;
  kind: 'team' | 'application';
  active: boolean;
  expanded: boolean;
  level: number;
  recentRuns: number;
  childCount: number;
  lastRunAt?: string;
};

export type PipelineRunTableRow = {
  run: RunListItem;
  pipelineName: string;
  repoName: string;
  branchLabel: string;
  runID: string;
  status: string;
  statusLabel: string;
  startedLabel: string;
  durationLabel: string;
};

export type PipelineRunBranchOption = {
  key: string;
  label: string;
  runCount: number;
  latestRunAt: number;
};

export const ALL_PIPELINE_RUN_BRANCHES = 'all';

const sourceFilters: PipelineRunSourceFilter[] = ['all', 'repository', 'schedule', 'external', 'manual'];
const statusFilters: PipelineRunStatusFilter[] = ['all', 'attention', 'running', 'failure', 'waiting_approval', 'success', 'pending'];

export function normalizeRunSourceFilter(value?: string | null): PipelineRunSourceFilter {
  const normalized = (value || '').trim().toLowerCase() as PipelineRunSourceFilter;
  return sourceFilters.includes(normalized) ? normalized : 'all';
}

export function normalizeRunStatusFilter(value?: string | null): PipelineRunStatusFilter {
  const normalized = (value || '').trim().toLowerCase() as PipelineRunStatusFilter;
  return statusFilters.includes(normalized) ? normalized : 'all';
}

export function flattenRunsByBranch(runsByBranch: Record<string, RunListItem[]>): RunListItem[] {
  const byId = new Map<string, RunListItem>();
  Object.values(runsByBranch).forEach(runs => {
    runs.forEach(run => {
      byId.set(run.run_id, run);
    });
  });
  return Array.from(byId.values()).sort((left, right) => runTimestamp(right) - runTimestamp(left));
}

export function filterPipelineRuns(runs: RunListItem[], filters: PipelineRunFilterOptions): RunListItem[] {
  const term = (filters.searchTerm || '').trim().toLowerCase();
  const sourceFilter = filters.sourceFilter || 'all';
  const statusFilter = filters.statusFilter || 'all';
  return runs
    .filter(run => !term || runMatchesSearch(run, term))
    .filter(run => sourceFilter === 'all' || getRunSourceKind(run) === sourceFilter)
    .filter(run => runMatchesStatusFilter(run, statusFilter))
    .sort((left, right) => runTimestamp(right) - runTimestamp(left));
}

export function buildPipelineRunBranchOptions(runs: RunListItem[]): PipelineRunBranchOption[] {
  const optionsByKey = new Map<string, PipelineRunBranchOption>();
  runs.forEach(run => {
    const key = pipelineRunBranchKey(run);
    const label = pipelineRunBranchLabel(run);
    const existing = optionsByKey.get(key);
    const timestamp = runTimestamp(run);
    if (existing) {
      existing.runCount += 1;
      existing.latestRunAt = Math.max(existing.latestRunAt, timestamp);
    } else {
      optionsByKey.set(key, {
        key,
        label,
        runCount: 1,
        latestRunAt: timestamp,
      });
    }
  });
  return Array.from(optionsByKey.values()).sort((left, right) => {
    const byLatestRun = right.latestRunAt - left.latestRunAt;
    if (byLatestRun !== 0) return byLatestRun;
    return left.label.localeCompare(right.label, undefined, { sensitivity: 'base' });
  });
}

export function filterPipelineRunsByBranch(runs: RunListItem[], branchKey: string): RunListItem[] {
  if (!branchKey || branchKey === ALL_PIPELINE_RUN_BRANCHES) return runs;
  return runs.filter(run => pipelineRunBranchKey(run) === branchKey);
}

export function buildPipelineRunOverviewMetrics(runs: RunListItem[], now = Date.now()): PipelineRunOverviewMetric[] {
  const activeRuns = runs.filter(run => normalizedRunStatus(run) === 'running');
  const failedRuns = runs.filter(isFailureRun);
  const waitingRuns = runs.filter(run => normalizedRunStatus(run) === 'waiting_approval');
  const recentWindowStart = now - 24 * 60 * 60 * 1000;
  const recentRuns = runs.filter(run => {
    const timestamp = runTimestamp(run);
    return timestamp > 0 && timestamp >= recentWindowStart;
  });
  const terminalRuns = (recentRuns.length ? recentRuns : runs).filter(isTerminalRun);
  const successfulRuns = terminalRuns.filter(run => normalizedRunStatus(run) === 'success');
  const successRate = terminalRuns.length ? `${Math.round((successfulRuns.length / terminalRuns.length) * 1000) / 10}%` : '-';
  const durations = terminalRuns
    .map(runDurationSeconds)
    .filter((duration): duration is number => duration !== null && duration >= 0)
    .sort((left, right) => left - right);
  const medianDuration = durations.length ? formatDurationSeconds(durations[Math.floor((durations.length - 1) / 2)] || 0) : '-';
  const activeScopes = new Set(activeRuns.map(run => runScopeName(run)).filter(Boolean));

  return [
    {
      id: 'running',
      label: 'Running now',
      value: String(activeRuns.length),
      note: activeRuns.length ? `Across ${activeScopes.size || 1} ${activeScopes.size === 1 ? 'target' : 'targets'}` : 'No active runs',
      tone: 'blue',
    },
    {
      id: 'attention',
      label: 'Needs attention',
      value: String(failedRuns.length + waitingRuns.length),
      note: `${failedRuns.length} failed, ${waitingRuns.length} waiting approval`,
      tone: 'red',
    },
    {
      id: 'success-rate',
      label: 'Success rate',
      value: successRate,
      note: terminalRuns.length ? 'Last 24 hours' : 'No terminal runs',
      tone: 'green',
    },
    {
      id: 'median-duration',
      label: 'Median duration',
      value: medianDuration,
      note: durations.length ? 'Completed runs' : 'No durations recorded',
      tone: 'amber',
    },
  ];
}

export function buildPipelineRunNavigationItems(
  teams: Team[],
  activeTeamId: number | null,
  teamSearchTerm = '',
  expandedTeamIds: ReadonlySet<number> = new Set(),
  collapsedTeamIds: ReadonlySet<number> = new Set()
): PipelineRunNavigationItem[] {
  const term = teamSearchTerm.trim().toLowerCase();
  const childrenByParent = buildChildrenByParent(teams);
  const activePathIds = new Set(teamLineage(teams, activeTeamId).map(team => team.id));
  const visibleIds = term ? buildSearchVisibleTeamIds(teams, term) : null;
  const items: PipelineRunNavigationItem[] = [];

  const visit = (parentId: number | null, level: number) => {
    (childrenByParent.get(parentId) || []).forEach(team => {
      if (visibleIds && !visibleIds.has(team.id)) return;
      const children = childrenByParent.get(team.id) || [];
      const expanded = visibleIds
        ? children.some(child => visibleIds.has(child.id))
        : !collapsedTeamIds.has(team.id) && (activePathIds.has(team.id) || expandedTeamIds.has(team.id));
      const subtree = teamSubtree(teams, team.id);
      const recentRuns = subtree.filter(item => Boolean(item.last_run_at)).length;

      items.push({
        id: team.id,
        label: teamDisplayName(team),
        path: teamPathForURL(team, teams),
        kind: isAppTeam(team) ? 'application' : 'team',
        active: activeTeamId === team.id,
        expanded,
        level,
        recentRuns,
        childCount: children.length,
        lastRunAt: team.last_run_at,
      });

      if (expanded) {
        visit(team.id, level + 1);
      }
    });
  };

  visit(null, 0);
  return items;
}

export function buildPipelineRunTableRows(runs: RunListItem[], limit?: number, now = Date.now()): PipelineRunTableRow[] {
  const sorted = [...runs].sort((left, right) => runTimestamp(right) - runTimestamp(left));
  const visible = typeof limit === 'number' && Number.isFinite(limit) && limit > 0
    ? sorted.slice(0, limit)
    : sorted;
  return visible.map(run => {
    const status = normalizedRunStatus(run);
    return {
      run,
      pipelineName: run.pipeline_name || 'Pipeline run',
      repoName: formatRepoLabel(run),
      branchLabel: formatPipelineRunBranch(run),
      runID: formatPipelineRunID(run.run_id),
      status,
      statusLabel: statusDisplayLabel(status),
      startedLabel: timeAgo(runStartedTimestamp(run), now),
      durationLabel: formatDuration(run),
    };
  });
}

function runMatchesStatusFilter(run: RunListItem, filter: PipelineRunStatusFilter): boolean {
  if (filter === 'all') return true;
  const normalized = normalizedRunStatus(run);
  if (filter === 'attention') return isFailureRun(run) || normalized === 'waiting_approval';
  if (filter === 'failure') return isFailureRun(run);
  return normalized === filter;
}

function pipelineRunBranchKey(run: RunListItem): string {
  const source = normalizeBranchPart(run.git_ref);
  const target = normalizeBranchPart(run.git_target_ref);
  if (!source && !target) return 'no-branch';
  return `${source || '-'}=>${target || '-'}`;
}

function pipelineRunBranchLabel(run: RunListItem): string {
  const label = formatBranchDisplay(run.git_ref, run.git_target_ref);
  return label && label !== '\u2014' ? label : 'No branch';
}

function normalizeBranchPart(ref?: string): string {
  return (ref || '').trim().replace(/^refs\/heads\//, '');
}

function normalizedRunStatus(run: RunListItem): string {
  return normalizeStatus(run.status, run.is_complete);
}

function isFailureRun(run: RunListItem): boolean {
  const normalized = normalizedRunStatus(run);
  return normalized === 'failure' || normalized === 'failure (ignored)' || normalized === 'rejected' || normalized === 'timed_out';
}

function isTerminalRun(run: RunListItem): boolean {
  const normalized = normalizedRunStatus(run);
  if (normalized === 'running' || normalized === 'waiting_approval' || normalized === 'pending') return false;
  return Boolean(run.is_complete) || ['success', 'failure', 'failure (ignored)', 'rejected', 'timed_out', 'cancelled', 'skipped'].includes(normalized);
}

function runDurationSeconds(run: RunListItem): number | null {
  const fromTimestamps = timestampDurationSeconds(run.started_at, run.finished_at);
  if (fromTimestamps !== null) return fromTimestamps;
  return parseDurationSeconds(run.duration);
}

function timestampDurationSeconds(start?: string, finish?: string): number | null {
  const startDate = parseRunTimestamp(start);
  const finishDate = parseRunTimestamp(finish);
  if (startDate === null || finishDate === null) return null;
  return Math.max(0, Math.round((finishDate - startDate) / 1000));
}

function parseDurationSeconds(value?: string): number | null {
  const text = (value || '').trim();
  if (!text) return null;
  const clock = text.match(/^(\d{1,2}):(\d{2})(?::(\d{2}))?$/);
  if (clock) {
    const first = Number(clock[1]);
    const second = Number(clock[2]);
    const third = clock[3] ? Number(clock[3]) : 0;
    return clock[3] ? first * 3600 + second * 60 + third : first * 60 + second;
  }
  const units = Array.from(text.matchAll(/(\d+(?:\.\d+)?)(h|m|s)/g));
  if (!units.length) return null;
  return Math.round(
    units.reduce((total, match) => {
      const amount = Number(match[1]);
      const unit = match[2];
      if (!Number.isFinite(amount)) return total;
      if (unit === 'h') return total + amount * 3600;
      if (unit === 'm') return total + amount * 60;
      return total + amount;
    }, 0)
  );
}

function formatDuration(run: RunListItem): string {
  const seconds = runDurationSeconds(run);
  return seconds === null ? '-' : formatDurationSeconds(seconds);
}

function formatDurationSeconds(seconds: number): string {
  const safeSeconds = Math.max(0, Math.round(seconds));
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  const remainingSeconds = safeSeconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${remainingSeconds}s`;
  return `${remainingSeconds}s`;
}

function formatPipelineRunBranch(run: RunListItem): string {
  const branch = formatBranchDisplay(run.git_ref, run.git_target_ref);
  return branch && branch !== '\u2014' ? branch : '-';
}

function formatPipelineRunID(runID?: string): string {
  const normalized = (runID || '').trim();
  return normalized ? normalized.slice(0, 8) : '-';
}

function runScopeName(run: RunListItem): string {
  const path = (run.pipeline_path || '').trim().replace(/^\/+|\/+$/g, '');
  if (path) return path.split('/').filter(Boolean).pop() || path;
  const repoLabel = formatRepoLabel(run);
  if (repoLabel && repoLabel !== 'Manual' && repoLabel !== 'Repository') return repoLabel;
  if (run.schedule_name || run.schedule_path || run.schedule_id) return run.schedule_name || run.schedule_path || run.schedule_id || 'Schedule';
  if (run.external_trigger_name || run.external_trigger_id) return run.external_trigger_name || run.external_trigger_id || 'External trigger';
  return 'Workspace';
}

function statusDisplayLabel(status: string): string {
  if (status === 'failure') return 'Failed';
  if (status === 'failure (ignored)') return 'Ignored failure';
  if (status === 'waiting_approval') return 'Approval';
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function teamSearchText(team: Team): string {
  return [teamDisplayName(team), team.name, team.path, team.team_path, team.description, team.repository_full_name, team.repo_url]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
}

function compareNavigationTeams(left: Team, right: Team): number {
  const leftApp = isAppTeam(left);
  const rightApp = isAppTeam(right);
  if (leftApp !== rightApp) return leftApp ? 1 : -1;
  return teamDisplayName(left).localeCompare(teamDisplayName(right), undefined, { sensitivity: 'base' });
}

function buildChildrenByParent(teams: Team[]): Map<number | null, Team[]> {
  const childrenByParent = new Map<number | null, Team[]>();
  teams.forEach(team => {
    const parentId = team.parent_id ?? null;
    const list = childrenByParent.get(parentId) || [];
    list.push(team);
    childrenByParent.set(parentId, list);
  });
  childrenByParent.forEach(children => children.sort(compareNavigationTeams));
  return childrenByParent;
}

function buildSearchVisibleTeamIds(teams: Team[], term: string): Set<number> {
  const visibleIds = new Set<number>();
  teams
    .filter(team => teamSearchText(team).includes(term))
    .forEach(team => {
      teamLineage(teams, team.id).forEach(pathTeam => visibleIds.add(pathTeam.id));
    });
  return visibleIds;
}

function teamLineage(teams: Team[], teamId: number | null): Team[] {
  if (teamId == null) return [];
  const byId = new Map(teams.map(team => [team.id, team]));
  const path: Team[] = [];
  const seen = new Set<number>();
  let current = byId.get(teamId) || null;
  while (current && !seen.has(current.id)) {
    seen.add(current.id);
    path.unshift(current);
    const parentId = current.parent_id ?? null;
    current = parentId == null ? null : byId.get(parentId) || null;
  }
  return path;
}

function teamSubtree(teams: Team[], rootId: number | null): Team[] {
  if (rootId == null) return [...teams];
  const root = teams.find(team => team.id === rootId);
  if (!root) return [];
  const result: Team[] = [];
  const seen = new Set<number>();
  const visit = (team: Team) => {
    if (seen.has(team.id)) return;
    seen.add(team.id);
    result.push(team);
    teams
      .filter(child => (child.parent_id ?? null) === team.id)
      .forEach(visit);
  };
  visit(root);
  return result;
}
