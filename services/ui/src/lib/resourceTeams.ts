import { apiClient } from './api.js';

export type ResourceTeam = {
  id: number;
  name: string;
  kind?: 'team' | 'app' | string;
  parent_id?: number | null;
  description?: string;
  repo_url?: string;
  repository_full_name?: string;
};

type TeamListResponse = {
  teams?: Array<{
    id: number;
    name?: string;
    slug?: string;
    display_name?: string;
    parent_team_id?: number | null;
    parent_id?: number | null;
    description?: string;
  }>;
  applications?: Array<{
    id: number;
    name?: string;
    slug?: string;
    display_name?: string;
    team_id?: number | null;
    parent_id?: number | null;
    description?: string;
    repo_url?: string;
    repository_full_name?: string;
  }>;
};

export type TreeNodeLike<T> = {
  name: string;
  fullPath: string;
  children: T[];
};

export const GLOBAL_RESOURCE_TEAM_PATH = 'global';
export const GLOBAL_RESOURCE_TEAM_LABEL = 'Global';

export async function fetchResourceTeamPaths(): Promise<string[]> {
  const response = await apiClient.fetch('/v1/access/teams');
  if (!response.ok) return [];
  const payload = await response.json();
  if (!Array.isArray(payload)) return [];
  return resourceTeamPathsWithGlobal(payload.map(normalizeAccessTeamPathRecord));
}

export async function fetchPipelineRunTeamPaths(): Promise<string[]> {
  const response = await apiClient.fetch('/v1/teams');
  if (!response.ok) return [];
  const payload = await response.json();
  return buildPipelineRunTeamPaths(resourceTeamsFromTeamPayload(payload));
}

function resourceTeamsFromTeamPayload(payload: unknown): ResourceTeam[] {
  if (Array.isArray(payload)) return payload as ResourceTeam[];
  const response = payload as TeamListResponse | null | undefined;
  const teams = Array.isArray(response?.teams) ? response.teams : [];
  const applications = Array.isArray(response?.applications) ? response.applications : [];
  return [
    ...teams.map(team => ({
      id: team.id,
      name: team.slug || team.name || team.display_name || String(team.id),
      kind: 'team',
      parent_id: team.parent_team_id ?? team.parent_id ?? null,
      description: team.description,
    })),
    ...applications.map(application => ({
      id: application.id,
      name: application.repository_full_name || application.slug || application.name || application.display_name || String(application.id),
      kind: 'app',
      parent_id: application.team_id ?? application.parent_id ?? null,
      description: application.description,
      repo_url: application.repo_url,
      repository_full_name: application.repository_full_name,
    })),
  ];
}

export function buildResourceTeamPaths(teams: ResourceTeam[]): string[] {
  return buildTeamPaths(teams, team => !isApplicationTeamLike(team));
}

export function buildPipelineRunTeamPaths(teams: ResourceTeam[]): string[] {
  return [GLOBAL_RESOURCE_TEAM_PATH, ...buildResourceTeamPaths(teams).filter(path => !isGlobalResourceTeamPath(path))];
}

function buildTeamPaths(teams: ResourceTeam[], includeTeam: (team: ResourceTeam) => boolean = () => true): string[] {
  const byId = new Map<number, ResourceTeam>();
  teams.forEach(team => byId.set(team.id, team));

  const pathCache = new Map<number, string | null>();
  const resolvePath = (team: ResourceTeam, visiting = new Set<number>()): string | null => {
    if (pathCache.has(team.id)) return pathCache.get(team.id) ?? null;
    const name = String(team.name || '').trim();
    if (!name || name.includes('/')) {
      pathCache.set(team.id, null);
      return null;
    }
    if (visiting.has(team.id)) {
      pathCache.set(team.id, null);
      return null;
    }
    visiting.add(team.id);
    const parentId = team.parent_id ?? null;
    const parent = parentId !== null ? byId.get(parentId) : null;
    const parentPath = parent ? resolvePath(parent, visiting) : '';
    visiting.delete(team.id);
    if (parent && parentPath === null) {
      pathCache.set(team.id, null);
      return null;
    }
    const path = parentPath ? `${parentPath}/${name}` : name;
    pathCache.set(team.id, path);
    return path;
  };

  return Array.from(
    new Set(
      teams
        .filter(includeTeam)
        .map(team => resolvePath(team))
        .filter((path): path is string => Boolean(path))
    )
  ).sort((a, b) => a.localeCompare(b));
}

function isApplicationTeamLike(team: ResourceTeam) {
  return team.kind === 'app' || Boolean(team.repo_url || team.repository_full_name) || String(team.name || '').includes('/');
}

function normalizeAccessTeamPathRecord(record: unknown): string {
  if (!record || typeof record !== 'object') return '';
  const entry = record as {
    id?: unknown;
    name?: unknown;
    kind?: unknown;
    repo_url?: unknown;
    repository_full_name?: unknown;
  };
  if (
    String(entry.kind || '').trim().toLowerCase() === 'app' ||
    Boolean(entry.repo_url || entry.repository_full_name)
  ) {
    return '';
  }
  const raw = typeof entry.id === 'string'
    ? entry.id
    : typeof entry.name === 'string'
      ? entry.name
      : '';
  return normalizeResourceTeamPath(raw);
}

export function normalizeResourceTeamPath(value: unknown): string {
  return String(value ?? '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

export function isGlobalResourceTeamPath(value?: string | null): boolean {
  const normalized = normalizeResourceTeamPath(value).toLowerCase();
  return normalized === GLOBAL_RESOURCE_TEAM_PATH;
}

export function resourceTeamPathsWithGlobal(paths: string[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  const append = (path: unknown) => {
    const normalized = normalizeResourceTeamPath(path);
    if (!normalized) return;
    const canonical = isGlobalResourceTeamPath(normalized) ? GLOBAL_RESOURCE_TEAM_PATH : normalized;
    const key = canonical.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    result.push(canonical);
  };

  append(GLOBAL_RESOURCE_TEAM_PATH);
  paths.forEach(append);

  return result.sort(compareResourceTeamPathsWithGlobalFirst);
}

export function compareResourceTeamPathsWithGlobalFirst(left: string, right: string): number {
  const leftGlobal = isGlobalResourceTeamPath(left);
  const rightGlobal = isGlobalResourceTeamPath(right);
  if (leftGlobal || rightGlobal) {
    if (leftGlobal === rightGlobal) return 0;
    return leftGlobal ? -1 : 1;
  }
  return normalizeResourceTeamPath(left).localeCompare(normalizeResourceTeamPath(right), undefined, { sensitivity: 'base' });
}

export function compareResourceTreeNodes<T extends { name: string; fullPath?: string | null }>(
  left: T,
  right: T
): number {
  const leftGlobal = isGlobalResourceTreeNode(left);
  const rightGlobal = isGlobalResourceTreeNode(right);
  if (leftGlobal || rightGlobal) {
    if (leftGlobal === rightGlobal) return 0;
    return leftGlobal ? -1 : 1;
  }
  const nameCompare = left.name.localeCompare(right.name, undefined, { sensitivity: 'base' });
  if (nameCompare !== 0) return nameCompare;
  return normalizeResourceTeamPath(left.fullPath).localeCompare(
    normalizeResourceTeamPath(right.fullPath),
    undefined,
    { sensitivity: 'base' }
  );
}

function isGlobalResourceTreeNode(node: { name: string; fullPath?: string | null }): boolean {
  return isGlobalResourceTeamPath(node.fullPath) || isGlobalResourceTeamPath(node.name);
}

export function insertTeamPath<T extends TreeNodeLike<T>>(
  root: T,
  rawPath: string,
  createNode: (id: string, name: string, fullPath: string) => T
): void {
  const parts = rawPath.split('/').map(part => part.trim()).filter(Boolean);
  let current = root;
  let pathSoFar = '';
  parts.forEach(segment => {
    pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
    let child = current.children.find(node => node.name === segment);
    if (!child) {
      child = createNode(pathSoFar, segment, pathSoFar);
      current.children.push(child);
      current.children.sort(compareResourceTreeNodes);
    }
    current = child;
  });
}
