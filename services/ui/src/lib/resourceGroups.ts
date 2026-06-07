import { apiClient } from './api.js';

export type ResourceGroup = {
  id: number;
  name: string;
  kind?: 'group' | 'app' | string;
  parent_id?: number | null;
  description?: string;
  repo_url?: string;
  repository_full_name?: string;
};

export type TreeNodeLike<T> = {
  name: string;
  fullPath: string;
  children: T[];
};

export async function fetchResourceGroupPaths(): Promise<string[]> {
  const response = await apiClient.fetch('/v1/groups');
  if (!response.ok) return [];
  const payload = await response.json();
  return Array.isArray(payload) ? buildResourceGroupPaths(payload as ResourceGroup[]) : [];
}

export async function fetchPipelineRunGroupPaths(): Promise<string[]> {
  const response = await apiClient.fetch('/v1/groups');
  if (!response.ok) return [];
  const payload = await response.json();
  return Array.isArray(payload) ? buildPipelineRunGroupPaths(payload as ResourceGroup[]) : [];
}

export function buildResourceGroupPaths(groups: ResourceGroup[]): string[] {
  return buildGroupPaths(groups);
}

export function buildPipelineRunGroupPaths(groups: ResourceGroup[]): string[] {
  return ['root', ...buildGroupPaths(groups, group => !isPipelineRunAppGroup(group)).filter(path => path !== 'root')];
}

function buildGroupPaths(groups: ResourceGroup[], includeGroup: (group: ResourceGroup) => boolean = () => true): string[] {
  const byId = new Map<number, ResourceGroup>();
  groups.forEach(group => byId.set(group.id, group));

  const pathCache = new Map<number, string | null>();
  const resolvePath = (group: ResourceGroup, visiting = new Set<number>()): string | null => {
    if (pathCache.has(group.id)) return pathCache.get(group.id) ?? null;
    const name = String(group.name || '').trim();
    if (!name || name.includes('/')) {
      pathCache.set(group.id, null);
      return null;
    }
    if (visiting.has(group.id)) {
      pathCache.set(group.id, null);
      return null;
    }
    visiting.add(group.id);
    const parentId = group.parent_id ?? null;
    const parent = parentId !== null ? byId.get(parentId) : null;
    const parentPath = parent ? resolvePath(parent, visiting) : '';
    visiting.delete(group.id);
    if (parent && parentPath === null) {
      pathCache.set(group.id, null);
      return null;
    }
    const path = parentPath ? `${parentPath}/${name}` : name;
    pathCache.set(group.id, path);
    return path;
  };

  return Array.from(
    new Set(
      groups
        .filter(includeGroup)
        .map(group => resolvePath(group))
        .filter((path): path is string => Boolean(path))
    )
  ).sort((a, b) => a.localeCompare(b));
}

function isPipelineRunAppGroup(group: ResourceGroup) {
  return group.kind === 'app' || Boolean(group.repo_url || group.repository_full_name) || String(group.name || '').includes('/');
}

export function insertGroupPath<T extends TreeNodeLike<T>>(
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
      current.children.sort((a, b) => a.name.localeCompare(b.name));
    }
    current = child;
  });
}
