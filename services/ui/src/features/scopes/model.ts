import { insertGroupPath } from '../../lib/resourceGroups.js';

export type ScopeEntry = {
  scope: string;
  label: string;
  folderPath: string;
  description: string;
  secretCountHint: number;
};

export type SourceKey = 'git' | 'database' | 'draft' | 'local';

export type ItemMeta = {
  source: SourceKey;
  createdAt?: string;
  updatedAt?: string;
};

export type ScopeTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: ScopeTreeNode[];
  scopes: string[];
};

export type ScopedIdentity = {
  repoOwner: string;
  repoName: string;
  repoSlug: string;
  name: string;
  fullName: string;
};

export function normalizeSourceKey(raw: unknown): SourceKey {
  const value = typeof raw === 'string' ? raw.trim().toLowerCase() : '';
  if (!value) return 'database';
  if (value.includes('git')) return 'git';
  if (value.includes('draft')) return 'draft';
  if (value.includes('local')) return 'local';
  return 'database';
}

export function parseScopedIdentity(fullName: string): ScopedIdentity {
  const normalized = String(fullName || '').trim().replace(/^\/+|\/+$/g, '');
  const parts = normalized.split('/').filter(Boolean);
  if (parts.length === 3) {
    const [repoOwner, repoName, name] = parts;
    return {
      repoOwner,
      repoName,
      repoSlug: `${repoOwner}/${repoName}`,
      name,
      fullName: `${repoOwner}/${repoName}/${name}`,
    };
  }
  return {
    repoOwner: '',
    repoName: '',
    repoSlug: '',
    name: normalized,
    fullName: normalized,
  };
}

export function normalizeRepositorySlug(raw: string): string {
  const trimmed = String(raw || '').trim().replace(/^\/+|\/+$/g, '');
  const parts = trimmed.split('/').filter(Boolean);
  if (parts.length !== 2) return '';
  const [owner, repository] = parts;
  if (!owner || !repository || /\s/.test(owner) || /\s/.test(repository)) return '';
  return `${owner}/${repository}`;
}

export function sanitizeScopeSegments(raw: string): string[] {
  return String(raw || '')
    .split('/')
    .map(part => part.trim().replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, ''))
    .filter(Boolean);
}

export function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value).trim().replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

export function encodeScopeForRoute(scope: string): string {
  const normalized = normalizeScopeLabel(scope);
  if (!normalized) return 'default';
  return normalized
    .split('/')
    .filter(Boolean)
    .map(encodeURIComponent)
    .join('/');
}

export function decodeScopeFromRoute(segments: string[]): string {
  const decoded = segments
    .filter(Boolean)
    .map(segment => {
      try {
        return decodeURIComponent(segment);
      } catch {
        return segment;
      }
    })
    .filter(Boolean);

  if (decoded.length === 1 && decoded[0] === 'default') return '';
  if (decoded[0] === 'default') return decoded.slice(1).join('/');
  return decoded.join('/');
}

export function buildScopeTree(scopes: ScopeEntry[], groupPaths: string[] = []): ScopeTreeNode {
  const root: ScopeTreeNode = { id: '__root__', name: 'All scopes', fullPath: '', children: [], scopes: [] };
  groupPaths.forEach(path => {
    insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], scopes: [] }));
  });
  scopes.forEach(scope => {
    const normalized = normalizeScopeLabel(scope.scope);
    const parts = normalized.split('/').filter(Boolean);
    if (!parts.length) {
      root.scopes.push('');
      return;
    }
    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(node => node.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], scopes: [] };
        current.children.push(child);
        current.children.sort((a, b) => a.name.localeCompare(b.name));
      }
      current = child;
    });
    current.scopes.push(normalized);
    current.scopes.sort((a, b) => a.localeCompare(b));
  });
  return root;
}

export function normalizeItemListPayload(payload: unknown): { names: string[]; meta: Record<string, ItemMeta> } {
  const names: string[] = [];
  const meta: Record<string, ItemMeta> = {};
  if (!Array.isArray(payload)) return { names, meta };

  payload.forEach(item => {
    if (typeof item === 'string') {
      const name = item.trim();
      if (!name || meta[name]) return;
      meta[name] = { source: 'database' };
      names.push(name);
      return;
    }
    if (!item || typeof item !== 'object') return;
    const record = item as Record<string, unknown>;
    const name = typeof record.name === 'string' ? record.name.trim() : '';
    if (!name || meta[name]) return;
    const createdAt =
      typeof record.created_at === 'string'
        ? record.created_at
        : typeof record.createdAt === 'string'
          ? record.createdAt
          : '';
    const updatedAt =
      typeof record.updated_at === 'string'
        ? record.updated_at
        : typeof record.updatedAt === 'string'
          ? record.updatedAt
          : '';
    meta[name] = {
      source: normalizeSourceKey(record.source),
      createdAt: createdAt || undefined,
      updatedAt: updatedAt || undefined,
    };
    names.push(name);
  });

  names.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  return { names, meta };
}

export function suggestCloneName(existing: string[], repoSlug: string, baseName: string): string {
  const sanitizedBase =
    String(baseName || 'item')
      .trim()
      .replace(/[^A-Za-z0-9_.-]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'item';
  const existingSet = new Set(existing.map(name => name.toLowerCase()));
  const buildFull = (candidate: string) => (repoSlug ? `${repoSlug}/${candidate}` : candidate).toLowerCase();
  let candidate = `${sanitizedBase}_copy`;
  let counter = 2;
  while (existingSet.has(buildFull(candidate))) {
    candidate = `${sanitizedBase}_copy_${counter++}`;
  }
  return candidate;
}
