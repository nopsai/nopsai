import yaml from 'js-yaml';
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

export type ScopeData = {
  variables: string[];
  variableMeta: Record<string, ItemMeta>;
  variablesLoaded: boolean;
  variablesLoading: boolean;
  secrets: string[];
  secretMeta: Record<string, ItemMeta>;
  secretsLoaded: boolean;
  secretsLoading: boolean;
  error?: string;
};

export type ScopePipelineMeta = {
  identifier: string;
  name: string;
  description: string;
  path: string;
  version: string;
  source: string;
};

export type ScopeTriggerDescriptor = {
  slug: string;
  scope: string;
  pipelines: string[];
  event: string;
  branches: string[];
  tags: string[];
};

export type GroupedScopedItem = { full: string; display: string };
export type GroupedScopedList = {
  global: GroupedScopedItem[];
  repositories: { repo: string; items: GroupedScopedItem[] }[];
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

export function scopeSourceLabel(source: SourceKey): string {
  switch (normalizeSourceKey(source)) {
    case 'git':
      return 'GitOps';
    case 'draft':
      return 'Draft';
    case 'local':
      return 'Local';
    default:
      return 'Database';
  }
}

export function scopeSourcePillClass(source: SourceKey): string {
  const normalized = normalizeSourceKey(source);
  if (normalized === 'git') return 'scope-variable-source-pill--git';
  if (normalized === 'draft') return 'scope-variable-source-pill--draft';
  if (normalized === 'local') return 'scope-variable-source-pill--local';
  return 'scope-variable-source-pill--database';
}

export function formatScopeDisplay(scopeLabel: string): string {
  const normalized = normalizeScopeLabel(scopeLabel);
  return normalized ? `/${normalized}` : '/';
}

export function createInitialScopeData(): ScopeData {
  return {
    variables: [],
    variableMeta: {},
    variablesLoaded: false,
    variablesLoading: false,
    secrets: [],
    secretMeta: {},
    secretsLoaded: false,
    secretsLoading: false,
  };
}

export function parentScopeFolder(path: string): string {
  const cleaned = normalizeScopeLabel(path);
  if (!cleaned) return '';
  const parts = cleaned.split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

export function getScopeTreeNode(root: ScopeTreeNode, path: string): ScopeTreeNode | null {
  const normalized = normalizeScopeLabel(path);
  if (!normalized) return root;
  const parts = normalized.split('/').filter(Boolean);
  let node = root;
  for (const part of parts) {
    const next = node.children.find(child => child.name === part);
    if (!next) return null;
    node = next;
  }
  return node;
}

export function countScopesRecursive(node: ScopeTreeNode): number {
  return node.scopes.length + node.children.reduce((total, child) => total + countScopesRecursive(child), 0);
}

export function isEditableScopeSource(source: SourceKey): boolean {
  return ['database', 'git'].includes(normalizeSourceKey(source));
}

export function isGitOpsScopeSource(source: SourceKey | undefined): boolean {
  return normalizeSourceKey(source) === 'git';
}

export function groupScopedItems(items: string[]): GroupedScopedList {
  const global: GroupedScopedItem[] = [];
  const repoMap = new Map<string, GroupedScopedItem[]>();

  items.forEach(entry => {
    const trimmed = String(entry || '').trim();
    if (!trimmed) return;
    const identity = parseScopedIdentity(trimmed);
    if (identity.repoSlug) {
      const list = repoMap.get(identity.repoSlug) || [];
      list.push({ full: identity.fullName, display: identity.name });
      repoMap.set(identity.repoSlug, list);
      return;
    }
    global.push({ full: trimmed, display: trimmed });
  });

  global.sort((a, b) => a.display.localeCompare(b.display, undefined, { sensitivity: 'base' }));
  const repositories = Array.from(repoMap.entries())
    .map(([repo, entries]) => ({
      repo,
      items: entries.sort((a, b) => a.display.localeCompare(b.display, undefined, { sensitivity: 'base' })),
    }))
    .sort((a, b) => a.repo.localeCompare(b.repo, undefined, { sensitivity: 'base' }));

  return { global, repositories };
}

export async function runWithConcurrencyLimit(tasks: Array<() => Promise<void>>, limit = 4): Promise<void> {
  const queue = tasks.slice();
  const workerCount = Math.max(1, Math.min(limit, queue.length));
  const workers = Array.from({ length: workerCount }, async () => {
    while (queue.length) {
      const task = queue.shift();
      if (!task) return;
      try {
        await task();
      } catch (error) {
        console.warn('Scope preload task failed', error);
      }
    }
  });
  await Promise.all(workers);
}

export function asScopeRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

export function parseScopeYamlSafe(raw: string): Record<string, unknown> {
  try {
    return asScopeRecord(yaml.load(raw) as unknown) || {};
  } catch (error) {
    console.warn('Failed to parse YAML', error);
    return {};
  }
}

export function normalizeScopePipelineIdentifier(value: string): string {
  return String(value || '')
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^pipelines\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\//, '');
}

export function parseScopePipelineIdentifier(identifier: string): { path: string; name: string } {
  const trimmed = normalizeScopePipelineIdentifier(identifier);
  if (!trimmed) return { path: '', name: '' };
  const parts = trimmed.split('/').filter(Boolean);
  return { name: parts.pop() || '', path: parts.join('/') };
}

export function buildScopePipelineMeta(
  identifier: string,
  manifest: Record<string, unknown>,
  seed?: { path?: string; version?: string; source?: string }
): ScopePipelineMeta {
  const normalizedID = normalizeScopePipelineIdentifier(identifier);
  const fallback = parseScopePipelineIdentifier(normalizedID);
  const name = typeof manifest.name === 'string' && manifest.name.trim() ? manifest.name.trim() : fallback.name || normalizedID;
  const description = typeof manifest.description === 'string' ? manifest.description : '';
  const seedPath = typeof seed?.path === 'string' ? seed.path.trim() : '';
  const detailPath = typeof manifest.path === 'string' ? manifest.path.trim() : '';
  const path = normalizeScopePipelineIdentifier(detailPath || seedPath || fallback.path);
  const version =
    typeof manifest.version === 'string' && manifest.version.trim()
      ? manifest.version.trim()
      : typeof seed?.version === 'string' && seed.version.trim()
        ? seed.version.trim()
        : 'latest';
  const sourceRaw = typeof manifest.source === 'string' ? manifest.source : seed?.source;

  return {
    identifier: normalizedID,
    name,
    description,
    path,
    version,
    source: scopeSourceLabel(normalizeSourceKey(sourceRaw)),
  };
}

export function normalizeScopePipelineList(payload: unknown): {
  identifiers: string[];
  seeds: Map<string, { path?: string; version?: string; source?: string }>;
} {
  const seeds = new Map<string, { path?: string; version?: string; source?: string }>();
  if (!Array.isArray(payload)) return { identifiers: [], seeds };
  const identifiers: string[] = [];

  payload.forEach(item => {
    if (!item) return;
    let identifier = '';
    if (typeof item === 'string') {
      identifier = normalizeScopePipelineIdentifier(item);
      if (identifier && !seeds.has(identifier)) seeds.set(identifier, { path: item.replace(/^\/+/, '') });
    } else if (typeof item === 'object') {
      const record = item as Record<string, unknown>;
      const rawIdentifier = typeof record.id === 'string' ? record.id : typeof record.identifier === 'string' ? record.identifier : '';
      identifier = normalizeScopePipelineIdentifier(rawIdentifier);
      if (identifier) {
        seeds.set(identifier, {
          path: typeof record.path === 'string' ? record.path : typeof record.file === 'string' ? record.file : '',
          version: typeof record.version === 'string' ? record.version : '',
          source: typeof record.source === 'string' ? record.source : '',
        });
      }
    }
    if (identifier) identifiers.push(identifier);
  });

  return { identifiers: Array.from(new Set(identifiers)).sort((a, b) => a.localeCompare(b)), seeds };
}

export function extractPipelineSecrets(manifest: unknown): string[] {
  const record = asScopeRecord(manifest);
  if (!record || !Array.isArray(record.steps)) return [];
  const secrets = new Set<string>();
  record.steps.forEach(stepValue => {
    const step = asScopeRecord(stepValue);
    if (!step || !Array.isArray(step.secrets)) return;
    step.secrets.forEach(secret => {
      if (typeof secret === 'string' && secret.trim()) secrets.add(secret.trim());
    });
  });
  return Array.from(secrets);
}

export function extractScopeVariables(manifest: unknown): string[] {
  const variables = new Set<string>();
  const record = asScopeRecord(manifest);
  if (!record) return [];

  const collect = (value: unknown) => {
    if (Array.isArray(value)) {
      value.forEach(entry => {
        if (typeof entry === 'string' && entry.trim()) variables.add(entry.trim());
      });
      return;
    }
    const valueRecord = asScopeRecord(value);
    if (!valueRecord) return;
    Object.entries(valueRecord).forEach(([key, entry]) => {
      if (key.trim()) variables.add(key.trim());
      if (typeof entry === 'string' && entry.trim()) variables.add(entry.trim());
    });
  };

  collect(record.variables);
  if (Array.isArray(record.steps)) {
    record.steps.forEach(stepValue => {
      const step = asScopeRecord(stepValue);
      if (!step) return;
      collect(step.variables);
      if (Array.isArray(step.tasks)) {
        step.tasks.forEach(taskValue => collect(asScopeRecord(taskValue)?.variables));
      }
    });
  }

  return Array.from(variables);
}

export function canonicalizeTriggerEvent(value: unknown): string {
  if (!value) return 'custom';
  const normalized = String(value).trim().toLowerCase();
  return normalized === 'pull-request' ? 'pull_request' : normalized;
}

export function extractTriggerPipelines(entries: unknown): string[] {
  if (!Array.isArray(entries)) return [];
  const identifiers = new Set<string>();
  entries.forEach(entry => {
    const record = asScopeRecord(entry);
    const raw =
      typeof entry === 'string'
        ? entry
        : typeof record?.path === 'string'
          ? record.path
          : typeof record?.pipeline === 'string'
            ? record.pipeline
            : '';
    const normalized = normalizeScopePipelineIdentifier(raw);
    if (normalized) identifiers.add(normalized);
  });
  return Array.from(identifiers);
}

export function normalizeTriggerOverrideSlugs(payload: unknown): string[] {
  if (!Array.isArray(payload)) return [];
  const slugs: string[] = [];
  payload.forEach(item => {
    if (typeof item === 'string') {
      if (item.trim()) slugs.push(item.trim());
      return;
    }
    const record = asScopeRecord(item);
    if (!record) return;
    const owner =
      typeof record.owner === 'string'
        ? record.owner
        : typeof record.repo_owner === 'string'
          ? record.repo_owner
          : typeof record.repoOwner === 'string'
            ? record.repoOwner
            : '';
    const name =
      typeof record.name === 'string'
        ? record.name
        : typeof record.repo === 'string'
          ? record.repo
          : typeof record.repository === 'string'
            ? record.repository
            : '';
    const slug = [owner, name].filter(Boolean).join('/');
    if (slug) slugs.push(slug);
  });
  return Array.from(new Set(slugs)).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
}
