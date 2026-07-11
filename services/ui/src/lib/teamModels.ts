import { normalizeTeamRoutePath } from './teamRoutes.js';

export type Team = {
  id: number;
  name: string;
  kind?: 'team' | 'app' | string;
  parent_id?: number | null;
  path?: string;
  team_path?: string;
  description?: string;
  repo_url?: string;
  repository_full_name?: string;
  last_run_at?: string;
  navigation_only?: boolean;
};

export function formatConfigRepoTimestamp(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function isAppTeam(team: Pick<Team, 'kind' | 'name' | 'repo_url' | 'repository_full_name'>) {
  return team.kind === 'app' || Boolean(team.repo_url || team.repository_full_name) || team.name.includes('/');
}

export function teamDisplayName(team: Pick<Team, 'kind' | 'name' | 'repo_url' | 'repository_full_name'>) {
  if (!isAppTeam(team)) return team.name;
  if (team.kind === 'app' && team.name && !team.name.includes('/')) return team.name;
  const fullName = team.repository_full_name || team.name;
  return fullName.split('/').filter(Boolean).pop() || team.name;
}

export function teamRepositoryURL(team: Pick<Team, 'name' | 'repo_url' | 'repository_full_name'>) {
  const fullName = (team.repository_full_name || team.name).trim().replace(/^\/+|\/+$/g, '');
  if (team.repo_url) return repositoryBrowserURL(team.repo_url, fullName);
  return fullName.includes('/') ? `https://github.com/${fullName}` : '';
}

export function teamRepositoryLabel(team: Pick<Team, 'name' | 'repo_url' | 'repository_full_name'>) {
  return (team.repository_full_name || team.name).trim().replace(/^\/+|\/+$/g, '');
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

export function buildTeamPath(teamId: number | null, teams: Team[]): Team[] {
  if (!teamId) return [];
  const map = new Map(teams.map(team => [team.id, team]));
  const path: Team[] = [];
  let current = map.get(teamId) || null;
  const visited = new Set<number>();
  while (current && !visited.has(current.id)) {
    visited.add(current.id);
    path.unshift(current);
    const parentId = current.parent_id ?? null;
    current = parentId ? map.get(parentId) || null : null;
  }
  return path;
}

export function normalizeTeamURLValue(value?: string | null) {
  return normalizeTeamRoutePath(value);
}

export function teamPathForURL(team: Team | null | undefined, teams: Team[]) {
  if (!team) return '';
  const directPath = normalizeTeamURLValue(team.path || '');
  if (directPath) return directPath;
  return buildTeamPath(team.id, teams)
    .map(item => normalizeTeamURLValue(item.name))
    .filter(Boolean)
    .join('/');
}

export function findTeamByURLValue(value: string | null | undefined, teams: Team[]) {
  const normalized = normalizeTeamURLValue(value);
  if (!normalized) return null;

  const maybeID = Number(normalized);
  if (Number.isInteger(maybeID) && maybeID > 0) {
    return teams.find(team => team.id === maybeID) || null;
  }

  const target = normalized.toLowerCase();
  return teams.find(team => teamPathForURL(team, teams).toLowerCase() === target) || null;
}
