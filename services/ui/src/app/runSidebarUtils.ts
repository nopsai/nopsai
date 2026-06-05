import { STATUS_PRIORITY } from './constants';
import type { RunGroup, RunListItem } from './types';

export function getSidebarStatusTone(status: string) {
  const normalized = normalizeRunStatus(status, true);
  if (normalized === 'success') return 'text-green-400';
  if (normalized === 'failure' || normalized === 'failure (ignored)') return 'text-red-400';
  if (normalized === 'rejected') return 'text-rose-400';
  if (normalized === 'waiting_approval') return 'text-cyan-400';
  if (normalized === 'running') return 'text-blue-400';
  return 'text-slate-300';
}

export function getStatusDotClass(status: string | undefined, complete?: boolean) {
  const normalized = normalizeRunStatus(status, complete);
  if (normalized === 'success') return 'bg-emerald-400';
  if (normalized === 'failure') return 'bg-red-500';
  if (normalized === 'failure (ignored)') return 'bg-amber-500';
  if (normalized === 'rejected') return 'bg-rose-500';
  if (normalized === 'waiting_approval') return 'bg-cyan-400';
  if (normalized === 'running') return 'bg-blue-400';
  if (normalized === 'cancelled') return 'bg-orange-400';
  if (normalized === 'skipped') return 'bg-slate-400';
  return 'bg-gray-500';
}

export function normalizeRunStatus(status: string | undefined, complete?: boolean): string {
  const raw = (status || '').toLowerCase();
  if (STATUS_PRIORITY.includes(raw)) return raw;
  if (!complete && raw !== 'success' && raw !== 'failure' && raw !== 'cancelled' && raw !== 'skipped') return 'running';
  return 'pending';
}

export function runMatchesSearch(run: RunListItem, term: string): boolean {
  if (!term) return true;
  const haystack = [
    run.pipeline_name,
    run.pipeline_path,
    run.git_repo_name,
    run.git_repo_owner,
    run.git_ref,
    run.git_target_ref,
    run.git_commit_sha,
    run.git_pusher_name,
    run.status,
    run.trigger_event_id,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  return haystack.includes(term);
}

export function formatBranch(ref?: string) {
  if (!ref) return '';
  return ref.replace(/^refs\/heads\//, '');
}

export function formatBranchDisplay(source?: string, target?: string) {
  const sourceBranch = formatBranch(source);
  const targetBranch = formatBranch(target);
  if (targetBranch && targetBranch !== '—') {
    return `${sourceBranch} -> ${targetBranch}`;
  }
  return sourceBranch;
}

export function formatRepoLabel(run: RunListItem) {
  const owner = run.git_repo_owner || '';
  const name = run.git_repo_name || '';
  if (owner && name) return `${owner}/${name}`;
  return name || owner || 'Repository';
}

export function formatTriggerLabel(id?: string) {
  if (!id) return { display: 'N/A', full: 'N/A' };
  const full = String(id);
  return { display: full, full };
}

export function timeAgoShort(dateInput?: string) {
  if (!dateInput) return '—';
  const date = new Date(dateInput);
  if (Number.isNaN(date.getTime())) return '—';
  const diff = Date.now() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function isRunAppGroup(group: Pick<RunGroup, 'kind' | 'name' | 'repo_url' | 'repository_full_name'>) {
  return group.kind === 'app' || Boolean(group.repo_url || group.repository_full_name) || group.name.includes('/');
}

export function runGroupDisplayName(group: Pick<RunGroup, 'kind' | 'name' | 'repo_url' | 'repository_full_name'>) {
  if (!isRunAppGroup(group)) return group.name;
  if (group.kind === 'app' && group.name && !group.name.includes('/')) return group.name;
  const fullName = group.repository_full_name || group.name;
  return fullName.split('/').filter(Boolean).pop() || group.name;
}

export function runGroupRepositoryURL(group: Pick<RunGroup, 'name' | 'repo_url' | 'repository_full_name'>) {
  const fullName = (group.repository_full_name || group.name).trim().replace(/^\/+|\/+$/g, '');
  if (group.repo_url) return repositoryBrowserURL(group.repo_url, fullName);
  return fullName.includes('/') ? `https://github.com/${fullName}` : '';
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

export function runGroupMatchesRepository(group: RunGroup, repoName: string) {
  const normalizedRepo = repoName.trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  if (!normalizedRepo) return false;
  const fullName = (group.repository_full_name || '').trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  if (fullName && fullName === normalizedRepo) return true;
  return group.name.trim().replace(/^\/+|\/+$/g, '').toLowerCase() === normalizedRepo;
}

export function buildGroupPath(groupId: number | null, groups: RunGroup[]): RunGroup[] {
  if (!groupId) return [];
  const map = new Map<number, RunGroup>();
  groups.forEach(group => map.set(group.id, group));
  const path: RunGroup[] = [];
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

export function summarizeStatus(runs: RunListItem[]): string {
  if (!runs.length) return 'pending';
  const ranked = runs
    .map(run => normalizeRunStatus(run.status, run.is_complete))
    .sort((a, b) => STATUS_PRIORITY.indexOf(a) - STATUS_PRIORITY.indexOf(b));
  return ranked[0] || 'pending';
}
