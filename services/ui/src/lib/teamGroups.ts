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
