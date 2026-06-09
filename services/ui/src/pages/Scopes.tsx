import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import yaml from 'js-yaml';
import { ArrowLeft, Copy, Eye, EyeOff, KeyRound, Pencil, Plus, Search, Trash2, X } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import { fetchResourceGroupPaths } from '../lib/resourceGroups';
import ResourceAccessCard from '../components/ResourceAccessCard';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { ScopeWorkflowModals } from '../features/scopes/ScopeWorkflowModals';
import { ScopeUsagePanel } from '../features/scopes/ScopeUsagePanel';
import {
  fetchScopeCatalogs,
  fetchScopedItems,
  fetchScopeUsageCatalogs,
  fetchScopeUsagePipelineYaml,
  fetchScopeUsageTriggerYaml,
  fetchVariableValue as fetchVariableValueRequest,
  scopedResourcePath,
} from '../features/scopes/api';
import { useScopeModalMutations } from '../features/scopes/useScopeModalMutations';
import { useScopePermissions } from '../features/scopes/useScopePermissions';
import {
  buildScopeTree,
  decodeScopeFromRoute,
  encodeScopeForRoute,
  normalizeItemListPayload,
  normalizeScopeLabel,
  normalizeSourceKey,
  parseScopedIdentity,
  type ItemMeta,
  type ScopeEntry,
  type ScopeTreeNode,
  type SourceKey,
} from '../features/scopes/model';

type ScopeData = {
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

type PipelineMeta = {
  identifier: string;
  name: string;
  description: string;
  path: string;
  version: string;
  source: string;
};

type TriggerDescriptor = {
  slug: string;
  scope: string;
  pipelines: string[];
  event: string;
  branches: string[];
  tags: string[];
};

function sourceLabel(source: SourceKey): string {
  switch (normalizeSourceKey(source)) {
    case 'git':
      return 'Git';
    case 'draft':
      return 'Draft';
    case 'local':
      return 'Local';
    case 'database':
    default:
      return 'Database';
  }
}

function sourcePillClass(source: SourceKey): string {
  const normalized = normalizeSourceKey(source);
  if (normalized === 'git') return 'scope-variable-source-pill--git';
  if (normalized === 'draft') return 'scope-variable-source-pill--draft';
  if (normalized === 'local') return 'scope-variable-source-pill--local';
  return 'scope-variable-source-pill--database';
}

function formatScopeDisplay(scopeLabel: string): string {
  const normalized = normalizeScopeLabel(scopeLabel);
  return normalized ? `/${normalized}` : '/';
}

function createInitialScopeData(): ScopeData {
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

function parentFolder(path: string): string {
  const cleaned = normalizeScopeLabel(path);
  if (!cleaned) return '';
  const parts = cleaned.split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function getScopeTreeNode(root: ScopeTreeNode, path: string): ScopeTreeNode | null {
  const normalized = normalizeScopeLabel(path);
  if (!normalized) return root;
  const parts = normalized.split('/').filter(Boolean);
  let node: ScopeTreeNode = root;
  for (const part of parts) {
    const next = node.children.find(child => child.name === part);
    if (!next) return null;
    node = next;
  }
  return node;
}

function countScopesRecursive(node: ScopeTreeNode): number {
  let total = node.scopes.length;
  node.children.forEach(child => {
    total += countScopesRecursive(child);
  });
  return total;
}

function isEditableSource(source: SourceKey): boolean {
  return normalizeSourceKey(source) !== 'git';
}

type GroupedScopedItem = { full: string; display: string };
type GroupedScopedList = { global: GroupedScopedItem[]; repositories: { repo: string; items: GroupedScopedItem[] }[] };

function groupScopedItems(items: string[]): GroupedScopedList {
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
    .map(([repo, vars]) => ({
      repo,
      items: vars.sort((a, b) => a.display.localeCompare(b.display, undefined, { sensitivity: 'base' })),
    }))
    .sort((a, b) => a.repo.localeCompare(b.repo, undefined, { sensitivity: 'base' }));

  return { global, repositories };
}

async function runWithConcurrencyLimit(tasks: Array<() => Promise<void>>, limit = 4): Promise<void> {
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

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function parseYamlSafe(raw: string): Record<string, unknown> {
  try {
    const parsed = yaml.load(raw) as unknown;
    return asRecord(parsed) || {};
  } catch (error) {
    console.warn('Failed to parse YAML', error);
  }
  return {};
}

function normalizePipelineIdentifier(value: string): string {
  return String(value || '')
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^pipelines\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\//, '');
}

function parsePipelineIdentifier(identifier: string): { path: string; name: string } {
  const trimmed = normalizePipelineIdentifier(identifier);
  if (!trimmed) return { path: '', name: '' };
  const parts = trimmed.split('/').filter(Boolean);
  const name = parts.pop() || '';
  const path = parts.join('/');
  return { path, name };
}

function buildPipelineMeta(identifier: string, manifest: Record<string, unknown>, seed?: { path?: string; version?: string; source?: string }): PipelineMeta {
  const normalizedId = normalizePipelineIdentifier(identifier);
  const fallback = parsePipelineIdentifier(normalizedId);
  const name = typeof manifest?.name === 'string' && manifest.name.trim() ? manifest.name.trim() : fallback.name || normalizedId;
  const description = typeof manifest?.description === 'string' ? manifest.description : '';
  const seedPath = typeof seed?.path === 'string' ? seed.path.trim() : '';
  const detailPath = typeof manifest?.path === 'string' ? manifest.path.trim() : '';
  const path = normalizePipelineIdentifier(detailPath || seedPath || fallback.path);
  const version = typeof manifest?.version === 'string' && manifest.version.trim()
    ? manifest.version.trim()
    : typeof seed?.version === 'string' && seed.version.trim()
      ? seed.version.trim()
      : 'latest';
  const sourceRaw = typeof manifest?.source === 'string' ? manifest.source : seed?.source;
  const source = sourceLabel(normalizeSourceKey(sourceRaw));

  return {
    identifier: normalizedId,
    name,
    description,
    path,
    version,
    source,
  };
}

function normalizePipelineList(payload: unknown): { identifiers: string[]; seeds: Map<string, { path?: string; version?: string; source?: string }> } {
  const seeds = new Map<string, { path?: string; version?: string; source?: string }>();
  if (!Array.isArray(payload)) {
    return { identifiers: [], seeds };
  }
  const identifiers: string[] = [];
  payload.forEach(item => {
    if (!item) return;
    let identifier = '';
    if (typeof item === 'string') {
      identifier = normalizePipelineIdentifier(item);
      if (identifier && !seeds.has(identifier)) {
        seeds.set(identifier, { path: item.replace(/^\/+/, '') });
      }
    } else if (typeof item === 'object') {
      const record = item as Record<string, unknown>;
      const rawIdentifier = typeof record.id === 'string' ? record.id : typeof record.identifier === 'string' ? record.identifier : '';
      identifier = normalizePipelineIdentifier(rawIdentifier);
      if (identifier) {
        const rawPath = typeof record.path === 'string' ? record.path : typeof record.file === 'string' ? record.file : '';
        const version = typeof record.version === 'string' ? record.version : '';
        const source = typeof record.source === 'string' ? record.source : '';
        seeds.set(identifier, { path: rawPath, version, source });
      }
    }
    if (identifier) identifiers.push(identifier);
  });
  return { identifiers: Array.from(new Set(identifiers)).sort((a, b) => a.localeCompare(b)), seeds };
}

function extractPipelineSecrets(manifest: unknown): string[] {
  const record = asRecord(manifest);
  if (!record || !Array.isArray(record.steps)) return [];
  const secrets = new Set<string>();
  record.steps.forEach((stepValue: unknown) => {
    const step = asRecord(stepValue);
    if (step && Array.isArray(step.secrets)) {
      step.secrets.forEach((secret: unknown) => {
        if (secret && typeof secret === 'string') secrets.add(secret.trim());
      });
    }
  });
  return Array.from(secrets);
}

function extractScopeVariables(manifest: unknown): string[] {
  const variables = new Set<string>();
  const record = asRecord(manifest);
  if (!record) return [];

  const collect = (value: unknown) => {
    if (!value) return;
    if (Array.isArray(value)) {
      value.forEach(entry => {
        if (typeof entry === 'string' && entry.trim()) variables.add(entry.trim());
      });
      return;
    }
    const valueRecord = asRecord(value);
    if (valueRecord) {
      Object.entries(valueRecord).forEach(([key, val]) => {
        if (typeof key === 'string' && key.trim()) variables.add(key.trim());
        if (typeof val === 'string' && val.trim()) variables.add(val.trim());
      });
    }
  };

  collect(record.variables);
  if (Array.isArray(record.steps)) {
    record.steps.forEach((stepValue: unknown) => {
      const step = asRecord(stepValue);
      if (!step) return;
      collect(step?.variables);
      if (Array.isArray(step?.tasks)) {
        step.tasks.forEach((taskValue: unknown) => {
          const task = asRecord(taskValue);
          collect(task?.variables);
        });
      }
    });
  }

  return Array.from(variables);
}

function canonicalizeEvent(value: unknown): string {
  if (!value) return 'custom';
  const normalized = String(value).trim().toLowerCase();
  if (normalized === 'pull-request') return 'pull_request';
  return normalized;
}

function extractTriggerPipelines(entries: unknown): string[] {
  if (!Array.isArray(entries)) return [];
  const identifiers = new Set<string>();
  entries.forEach(entry => {
    let raw = '';
    if (typeof entry === 'string') {
      raw = entry;
    } else if (entry && typeof entry === 'object') {
      const record = entry as Record<string, unknown>;
      raw = typeof record.path === 'string' ? record.path : typeof record.pipeline === 'string' ? record.pipeline : '';
    }
    const normalized = normalizePipelineIdentifier(raw);
    if (normalized) identifiers.add(normalized);
  });
  return Array.from(identifiers);
}

function normalizeOverrideSlugs(payload: unknown): string[] {
  if (!Array.isArray(payload)) return [];
  const slugs: string[] = [];
  payload.forEach(item => {
    if (!item) return;
    if (typeof item === 'string') {
      const slug = item.trim();
      if (slug) slugs.push(slug);
      return;
    }
    if (typeof item === 'object') {
      const record = item as Record<string, unknown>;
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
    }
  });
  return Array.from(new Set(slugs.filter(Boolean))).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
}

function ScopesPage({
  canDeleteScopes = false,
}: {
  canDeleteScopes?: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();

  const [scopes, setScopes] = useState<ScopeEntry[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [scopeDataByScope, setScopeDataByScope] = useState<Record<string, ScopeData>>({});
  const scopeDataRef = useRef<Record<string, ScopeData>>({});
  const scopeVariablesPromiseRef = useRef<Map<string, Promise<void>>>(new Map());
  const scopeSecretsPromiseRef = useRef<Map<string, Promise<void>>>(new Map());

  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const [selectedScope, setSelectedScope] = useState<string | null>(null);
  const selectedScopeRef = useRef<string | null>(null);
  const preloadScopesRef = useRef<Set<string>>(new Set());
  const [selectedVariable, setSelectedVariable] = useState<string | null>(null);
  const [selectedSecret, setSelectedSecret] = useState<string | null>(null);
  const selectVariable = useCallback((name: string | null) => {
    setSelectedVariable(name);
    if (name) {
      setSelectedSecret(null);
    }
  }, []);
  const selectSecret = useCallback((name: string | null) => {
    setSelectedSecret(name);
    if (name) {
      setSelectedVariable(null);
    }
  }, []);

  const [expandedVariableKey, setExpandedVariableKey] = useState<string | null>(null);
  const [variableValueLoadingKey, setVariableValueLoadingKey] = useState<string | null>(null);
  const [variableValues, setVariableValues] = useState<Record<string, string>>({});
  const variableValuesRef = useRef<Record<string, string>>({});
  const variableValuePromiseRef = useRef<Map<string, Promise<string>>>(new Map());

  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

  const [pipelineVariableIndex, setPipelineVariableIndex] = useState<Map<string, Set<string>>>(new Map());
  const [pipelineSecretIndex, setPipelineSecretIndex] = useState<Map<string, Set<string>>>(new Map());
  const [pipelineMetadata, setPipelineMetadata] = useState<Map<string, PipelineMeta>>(new Map());
  const [triggersByScope, setTriggersByScope] = useState<Map<string, TriggerDescriptor[]>>(new Map());
  const [usageLoading, setUsageLoading] = useState(false);
  const [usageError, setUsageError] = useState<string | null>(null);
  const usageReadyRef = useRef(false);

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  useEffect(() => {
    scopeDataRef.current = scopeDataByScope;
  }, [scopeDataByScope]);

  useEffect(() => {
    variableValuesRef.current = variableValues;
  }, [variableValues]);

  const loadScopes = useCallback(async () => {
    setListLoading(true);
    setListError(null);
    try {
      const { secrets: secretJson, variables: variableJson } = await fetchScopeCatalogs();

      const secretCounts = new Map<string, number>();
      if (Array.isArray(secretJson)) {
        secretJson.forEach((entry: unknown) => {
          if (!entry || typeof entry !== 'object') return;
          const record = entry as Record<string, unknown>;
          const scopeLabel = normalizeScopeLabel(record.scope);
          const count = typeof record.secret_count === 'number' ? record.secret_count : 0;
          secretCounts.set(scopeLabel, count);
        });
      }

      const scopeSet = new Set<string>();
      scopeSet.add('');
      if (Array.isArray(variableJson)) {
        variableJson.forEach((entry: unknown) => {
          if (typeof entry === 'string') {
            scopeSet.add(normalizeScopeLabel(entry));
            return;
          }
          if (!entry || typeof entry !== 'object') return;
          const record = entry as Record<string, unknown>;
          scopeSet.add(normalizeScopeLabel(record.scope ?? record.name ?? record.value));
        });
      }
      secretCounts.forEach((_, scopeLabel) => scopeSet.add(scopeLabel));

      const nextScopes: ScopeEntry[] = Array.from(scopeSet)
        .map(scopeLabel => {
          const normalized = normalizeScopeLabel(scopeLabel);
          const parts = normalized.split('/').filter(Boolean);
          const label = normalized ? parts[parts.length - 1] : 'Default Scope';
          const description = normalized ? `Scope “/${normalized}”` : 'Fallback scope shared across all pipelines';
          return {
            scope: normalized,
            label,
            folderPath: normalized,
            description,
            secretCountHint: secretCounts.get(normalized) || 0,
          };
        })
        .sort((a, b) => {
          const folderCompare = (a.folderPath || '').localeCompare(b.folderPath || '', undefined, { sensitivity: 'base' });
          if (folderCompare !== 0) return folderCompare;
          return (a.label || '').localeCompare(b.label || '', undefined, { sensitivity: 'base' });
        });

      setScopes(nextScopes);
    } catch (error) {
      console.error('Failed to load scopes', error);
      setListError(error instanceof Error ? error.message : 'Unable to load scopes');
      setScopes([]);
    } finally {
      setListLoading(false);
    }
  }, []);

  const ensureScopeVariables = useCallback(async (scopeLabel: string, force = false) => {
    const scope = normalizeScopeLabel(scopeLabel);
    const existing = scopeDataRef.current[scope];
    if (!force && existing?.variablesLoaded) return;

    if (scopeVariablesPromiseRef.current.has(scope)) {
      await scopeVariablesPromiseRef.current.get(scope);
      if (!force) return;
    }

    setScopeDataByScope(prev => {
      const current = prev[scope] || createInitialScopeData();
      if (!force && current.variablesLoaded) return prev;
      return { ...prev, [scope]: { ...current, variablesLoading: true, error: undefined } };
    });

    const promise = (async () => {
      try {
        const payload = await fetchScopedItems('variable', scope);
        const { names, meta } = normalizeItemListPayload(payload);

        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              variables: names,
              variableMeta: meta,
              variablesLoaded: true,
              variablesLoading: false,
            },
          };
        });
      } catch (error) {
        console.error('Failed to load scope variables', { scope, error });
        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              variables: current.variablesLoaded ? current.variables : [],
              variableMeta: current.variablesLoaded ? current.variableMeta : {},
              variablesLoaded: current.variablesLoaded,
              variablesLoading: false,
              error: error instanceof Error ? error.message : 'Unable to load variables',
            },
          };
        });
      } finally {
        scopeVariablesPromiseRef.current.delete(scope);
      }
    })();

    scopeVariablesPromiseRef.current.set(scope, promise);
    await promise;
  }, []);

  const ensureScopeSecrets = useCallback(async (scopeLabel: string, force = false) => {
    const scope = normalizeScopeLabel(scopeLabel);
    const existing = scopeDataRef.current[scope];
    if (!force && existing?.secretsLoaded) return;

    if (scopeSecretsPromiseRef.current.has(scope)) {
      await scopeSecretsPromiseRef.current.get(scope);
      if (!force) return;
    }

    setScopeDataByScope(prev => {
      const current = prev[scope] || createInitialScopeData();
      if (!force && current.secretsLoaded) return prev;
      return { ...prev, [scope]: { ...current, secretsLoading: true, error: undefined } };
    });

    const promise = (async () => {
      try {
        const payload = await fetchScopedItems('secret', scope);
        const { names, meta } = normalizeItemListPayload(payload);

        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              secrets: names,
              secretMeta: meta,
              secretsLoaded: true,
              secretsLoading: false,
            },
          };
        });
      } catch (error) {
        console.error('Failed to load scope secrets', { scope, error });
        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              secrets: current.secretsLoaded ? current.secrets : [],
              secretMeta: current.secretsLoaded ? current.secretMeta : {},
              secretsLoaded: current.secretsLoaded,
              secretsLoading: false,
              error: error instanceof Error ? error.message : 'Unable to load secrets',
            },
          };
        });
      } finally {
        scopeSecretsPromiseRef.current.delete(scope);
      }
    })();

    scopeSecretsPromiseRef.current.set(scope, promise);
    await promise;
  }, []);

  const buildUsageIndexes = useCallback(async () => {
    if (usageReadyRef.current) return;
    setUsageLoading(true);
    setUsageError(null);
    try {
      const { pipelines: pipelinesPayload, triggers: trigPayload } = await fetchScopeUsageCatalogs();
      const { identifiers, seeds } = normalizePipelineList(pipelinesPayload);

      const variableIndex = new Map<string, Set<string>>();
      const secretIndex = new Map<string, Set<string>>();
      const metaMap = new Map<string, PipelineMeta>();

      const pipelineTasks = identifiers.map(identifier => {
        return async () => {
          try {
            const rawYaml = await fetchScopeUsagePipelineYaml(identifier);
            if (!rawYaml) return;
            const manifest = parseYamlSafe(rawYaml);
            const seed = seeds.get(identifier);
            metaMap.set(identifier, buildPipelineMeta(identifier, manifest, seed));

            extractScopeVariables(manifest).forEach(variable => {
              if (!variable) return;
              const set = variableIndex.get(variable) || new Set<string>();
              set.add(identifier);
              variableIndex.set(variable, set);
            });

            extractPipelineSecrets(manifest).forEach(secret => {
              if (!secret) return;
              const set = secretIndex.get(secret) || new Set<string>();
              set.add(identifier);
              secretIndex.set(secret, set);
            });
          } catch (error) {
            console.warn('Failed to process pipeline for usage', identifier, error);
          }
        };
      });

      await runWithConcurrencyLimit(pipelineTasks, 4);
      setPipelineVariableIndex(variableIndex);
      setPipelineSecretIndex(secretIndex);
      setPipelineMetadata(metaMap);

      const slugs = normalizeOverrideSlugs(trigPayload);
      const trigMap = new Map<string, TriggerDescriptor[]>();

      const triggerTasks = slugs.map(slug => {
        return async () => {
          const [owner, name] = slug.split('/');
          if (!owner || !name) return;
          try {
            const rawYaml = await fetchScopeUsageTriggerYaml(slug);
            if (!rawYaml) return;
            const manifest = parseYamlSafe(rawYaml);
            const triggers = Array.isArray(manifest?.triggers) ? manifest.triggers : [];
            triggers.forEach((triggerValue: unknown) => {
              const trigger = asRecord(triggerValue);
              if (!trigger) return;
              const scope = normalizeScopeLabel(trigger?.scope || '');
              const entry: TriggerDescriptor = {
                slug,
                scope,
                pipelines: extractTriggerPipelines(trigger?.pipelines),
                event: canonicalizeEvent(trigger?.on),
                branches: Array.isArray(trigger?.branches) ? trigger.branches.map((b: unknown) => String(b || '').trim()).filter(Boolean) : [],
                tags: Array.isArray(trigger?.tags) ? trigger.tags.map((t: unknown) => String(t || '').trim()).filter(Boolean) : [],
              };
              const list = trigMap.get(scope) || [];
              list.push(entry);
              trigMap.set(scope, list);
            });
          } catch (error) {
            console.warn('Failed to process trigger override', slug, error);
          }
        };
      });

      await runWithConcurrencyLimit(triggerTasks, 4);
      trigMap.forEach(list => {
        list.sort((a, b) => a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' }));
      });
      setTriggersByScope(trigMap);

      usageReadyRef.current = true;
    } catch (error) {
      console.error('Impact analysis failed', error);
      setUsageError(error instanceof Error ? error.message : 'Unable to build impact analysis.');
    } finally {
      setUsageLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadScopes();
  }, [loadScopes]);

  useEffect(() => {
    let cancelled = false;
    void fetchResourceGroupPaths()
      .then(paths => {
        if (!cancelled) setResourceGroupPaths(paths);
      })
      .catch(error => {
        console.warn('Failed to load groups for scope tree', error);
        if (!cancelled) setResourceGroupPaths([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!scopes.length) return;
    const tasks: Array<() => Promise<void>> = [];
    scopes.forEach(scope => {
      const label = normalizeScopeLabel(scope.scope);
      if (preloadScopesRef.current.has(label)) return;
      preloadScopesRef.current.add(label);
      tasks.push(async () => {
        await ensureScopeVariables(label);
        await ensureScopeSecrets(label);
      });
    });
    if (tasks.length) {
      void runWithConcurrencyLimit(tasks, 4);
    }
  }, [ensureScopeSecrets, ensureScopeVariables, scopes]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'scopes') return;
    if (segments.length > 1) {
      const scopeLabel = normalizeScopeLabel(decodeScopeFromRoute(segments.slice(1)));
      if (scopeLabel !== selectedScopeRef.current) {
        selectedScopeRef.current = scopeLabel;
        setSelectedScope(scopeLabel);
      }
    } else if (selectedScopeRef.current !== null) {
      selectedScopeRef.current = null;
      setSelectedScope(null);
    }

    const params = new URLSearchParams(location.search);
    setActiveFolder(params.get('folder') || '');
  }, [location.pathname, location.search]);

  useEffect(() => {
    if (listLoading || listError) return;
    if (selectedScope == null) return;
    const normalized = normalizeScopeLabel(selectedScope);
    if (!scopes.some(scope => scope.scope === normalized)) {
      selectedScopeRef.current = null;
      setSelectedScope(null);
      navigate('/scopes', { replace: true });
    }
  }, [listError, listLoading, navigate, scopes, selectedScope]);

  useEffect(() => {
    if (selectedScope == null) {
      selectVariable(null);
      selectSecret(null);
      setExpandedVariableKey(null);
      return;
    }
    selectVariable(null);
    selectSecret(null);
    setExpandedVariableKey(null);
    void ensureScopeVariables(selectedScope);
    void ensureScopeSecrets(selectedScope);
  }, [ensureScopeSecrets, ensureScopeVariables, selectSecret, selectVariable, selectedScope]);

  useEffect(() => {
    if (selectedScope == null) return;
    const data = scopeDataByScope[selectedScope];
    if (!data) return;

    if (data.variablesLoaded && selectedVariable && !data.variables.includes(selectedVariable)) {
      selectVariable(null);
    }

    if (data.secretsLoaded && selectedSecret && !data.secrets.includes(selectedSecret)) {
      selectSecret(null);
    }
  }, [scopeDataByScope, selectSecret, selectVariable, selectedScope, selectedSecret, selectedVariable]);

  useEffect(() => {
    if (selectedScope == null) return;
    if (usageReadyRef.current) return;
    void buildUsageIndexes();
  }, [buildUsageIndexes, selectedScope]);

  const {
    canCreateScopeHere,
    canWriteVariablesInSelectedScope,
    canWriteSecretsInSelectedScope,
  } = useScopePermissions(activeFolder, selectedScope);

  const scopesByLabel = useMemo(() => {
    const map = new Map<string, ScopeEntry>();
    scopes.forEach(scope => map.set(scope.scope, scope));
    return map;
  }, [scopes]);

  const knownRepositories = useMemo(() => {
    const repos = new Set<string>();
    Object.values(scopeDataByScope).forEach(data => {
      data.variables.forEach(name => {
        const identity = parseScopedIdentity(name);
        if (identity.repoSlug) repos.add(identity.repoSlug);
      });
      data.secrets.forEach(name => {
        const identity = parseScopedIdentity(name);
        if (identity.repoSlug) repos.add(identity.repoSlug);
      });
    });
    return Array.from(repos).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  }, [scopeDataByScope]);

  const variableSuggestionEntries = useMemo(() => {
    const entries: Array<{ scope: string; label: string; count: number; preview: string[] }> = [];
    Object.entries(scopeDataByScope).forEach(([scope, data]) => {
      if (!data.variablesLoaded || data.variables.length === 0) return;
      entries.push({
        scope,
        label: scope ? `/${scope}` : '/ (default)',
        count: data.variables.length,
        preview: data.variables.slice(0, 5),
      });
    });
    entries.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
    return entries;
  }, [scopeDataByScope]);

  const secretSuggestionEntries = useMemo(() => {
    const entries: Array<{ scope: string; label: string; count: number; preview: string[] }> = [];
    Object.entries(scopeDataByScope).forEach(([scope, data]) => {
      if (!data.secretsLoaded || data.secrets.length === 0) return;
      entries.push({
        scope,
        label: scope ? `/${scope}` : '/ (default)',
        count: data.secrets.length,
        preview: data.secrets.slice(0, 5),
      });
    });
    entries.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
    return entries;
  }, [scopeDataByScope]);

  const scopeTree = useMemo(() => buildScopeTree(scopes, resourceGroupPaths), [resourceGroupPaths, scopes]);

  const filteredScopes = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    if (!term) return scopes;
    return scopes.filter(scope => {
      if (scope.scope.toLowerCase().includes(term)) return true;
      if (scope.label.toLowerCase().includes(term)) return true;
      if (scope.description.toLowerCase().includes(term)) return true;
      return false;
    });
  }, [scopes, searchTerm]);

  const activeFolderNode = useMemo(() => {
    const node = getScopeTreeNode(scopeTree, activeFolder);
    return node || scopeTree;
  }, [activeFolder, scopeTree]);

  const openFolder = (path: string) => {
    const cleaned = normalizeScopeLabel(path);
    setActiveFolder(cleaned);
    selectedScopeRef.current = null;
    setSelectedScope(null);
    navigate(cleaned ? `/scopes?folder=${encodeURIComponent(cleaned)}` : '/scopes');
  };

  const handleSelectScope = (scopeLabel: string) => {
    const normalized = normalizeScopeLabel(scopeLabel);
    selectedScopeRef.current = normalized;
    setSelectedScope(normalized);
    navigate(`/scopes/${encodeScopeForRoute(normalized)}`);
  };

  const handleBackToList = () => {
    if (selectedScope != null) {
      navigate(parentFolder(selectedScope) ? `/scopes?folder=${encodeURIComponent(parentFolder(selectedScope))}` : '/scopes');
      return;
    }
    navigate('/scopes');
  };

  const {
    chooseSecretSuggestion,
    chooseVariableSuggestion,
    closeDeleteModal,
    closeGitOpsEncryptModal,
    closeScopeModal,
    closeSecretModal,
    closeVariableModal,
    confirmDelete,
    copyGitOpsEncryptedValue,
    deleteModal,
    encryptGitOpsSecretValue,
    gitOpsEncryptModal,
    openDeleteModal,
    openGitOpsEncryptModal,
    openNewScopeModal,
    openSecretCloneModal,
    openSecretCreateModal,
    openSecretUpdateModal,
    openVariableCloneModal,
    openVariableCreateModal,
    openVariableUpdateModal,
    scopeModal,
    secretModal,
    submitScopeModal,
    submitSecretModal,
    submitVariableModal,
    updateGitOpsEncryptValue,
    updateScopeName,
    updateSecretModal,
    updateVariableModal,
    variableModal,
  } = useScopeModalMutations({
    activeFolder,
    scopesByLabel,
    scopeDataByScope,
    canCreateScopeHere,
    canWriteVariablesInSelectedScope,
    canWriteSecretsInSelectedScope,
    canDeleteScopes,
    selectedVariable,
    selectedSecret,
    addToast,
    loadScopes,
    ensureScopeVariables,
    ensureScopeSecrets,
    selectVariable,
    selectSecret,
    clearExpandedVariable: () => setExpandedVariableKey(null),
    onScopeCreated: scope => navigate(`/scopes/${encodeScopeForRoute(scope)}`),
  });

  const fetchVariableValue = useCallback(async (scopeLabel: string, fullName: string): Promise<string> => {
    const scope = normalizeScopeLabel(scopeLabel);
    const identity = parseScopedIdentity(fullName);
    if (!identity.name) return '';

    const cacheKey = `${identity.fullName}@@${scope}`;
    if (Object.prototype.hasOwnProperty.call(variableValuesRef.current, cacheKey)) {
      return variableValuesRef.current[cacheKey] ?? '';
    }
    if (variableValuePromiseRef.current.has(cacheKey)) {
      return (await variableValuePromiseRef.current.get(cacheKey)) ?? '';
    }

    const promise = (async () => {
      try {
        return await fetchVariableValueRequest(scopedResourcePath('variable', scope, identity.name, identity.repoSlug));
      } finally {
        variableValuePromiseRef.current.delete(cacheKey);
      }
    })();

    variableValuePromiseRef.current.set(cacheKey, promise);
    const value = await promise;
    setVariableValues(prev => ({ ...prev, [cacheKey]: value }));
    return value;
  }, []);

  const toggleVariableValue = async (scopeLabel: string, fullName: string) => {
    const scope = normalizeScopeLabel(scopeLabel);
    const identity = parseScopedIdentity(fullName);
    const cacheKey = `${identity.fullName}@@${scope}`;
    if (expandedVariableKey === cacheKey) {
      setExpandedVariableKey(null);
      return;
    }

    if (!Object.prototype.hasOwnProperty.call(variableValuesRef.current, cacheKey)) {
      try {
        setVariableValueLoadingKey(cacheKey);
        await fetchVariableValue(scope, identity.fullName);
      } catch (error) {
        console.error('Failed to fetch variable value', error);
        addToast(error instanceof Error ? error.message : 'Failed to load variable value.', 'error');
      } finally {
        setVariableValueLoadingKey(null);
      }
    }

    setExpandedVariableKey(cacheKey);
  };

  const renderFolderCard = (node: ScopeTreeNode) => {
    const totalScopes = countScopesRecursive(node);
    return (
      <article
        key={`folder-${node.id}`}
        className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
        onClick={() => openFolder(node.fullPath)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="pipeline-card-icon" aria-hidden="true">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{node.name}</h3>
              <p className="pipeline-card-path">{node.fullPath ? `/${node.fullPath}` : '/'}</p>
            </div>
          </div>
          <span className="pipeline-folder-chevron">›</span>
        </div>
        <div className="pipeline-folder-meta">
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-card-meta-label">Scopes:</span>
            <span className="pipeline-card-meta-value">{totalScopes}</span>
          </div>
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-card-meta-label">Sub groups:</span>
            <span className="pipeline-card-meta-value">{node.children.length}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderScopeCard = (entry: ScopeEntry) => {
    const scopeLabel = entry.scope ? `/${entry.scope}` : '/';
    const data = scopeDataByScope[entry.scope];
    const variableCount = data?.variablesLoaded ? data.variables.length : 0;
    const secretCount = data?.secretsLoaded ? data.secrets.length : entry.secretCountHint;
    const variableLabel = `${variableCount} variable${variableCount === 1 ? '' : 's'}`;
    const secretLabel = `${secretCount} secret${secretCount === 1 ? '' : 's'}`;
    return (
      <article
        key={entry.scope || '__default__'}
        className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
        onClick={() => handleSelectScope(entry.scope)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="pipeline-card-icon" aria-hidden="true">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="7.5" />
                <circle cx="12" cy="12" r="2.5" />
                <path d="M12 3v3m0 12v3m9-9h-3M6 12H3" />
                <path d="M16.5 7.5l-1.75 1.75m-5.5 5.5L7.5 16.5" />
                <path d="M7.5 7.5l1.75 1.75m5.5 5.5l1.75 1.75" />
              </svg>
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{entry.label}</h3>
              <p className="pipeline-card-path">{scopeLabel}</p>
              <p className="pipeline-card-description">Configuration &amp; secrets manager.</p>
            </div>
          </div>
        </div>
        <div className="pipeline-card-meta">
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Variables</span>
            <span className="pipeline-card-meta-value">{variableLabel}</span>
          </div>
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Secrets</span>
            <span className="pipeline-card-meta-value">{secretLabel}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderList = () => {
    const hasSearch = Boolean(searchTerm.trim());
    const folders = hasSearch ? [] : activeFolderNode.children;
    const scopeLabels = hasSearch ? [] : activeFolderNode.scopes;
    const scopeEntries = hasSearch
      ? filteredScopes
      : scopeLabels.map(label => scopesByLabel.get(label)).filter((item): item is ScopeEntry => Boolean(item));

    return (
      <div id="scopes-list-view" className="pipelines-view">
        <div className="space-y-3">
          {listLoading ? (
            <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading scopes…</div>
          ) : listError ? (
            <div className="glass-card p-5 text-sm text-red-500">Failed to load scopes: {listError}</div>
          ) : (
            <>
              {scopeEntries.length ? (
                <div className="pipelines-card-grid pipelines-card-grid--pipelines">{scopeEntries.map(scope => renderScopeCard(scope))}</div>
              ) : null}

              {!hasSearch && folders.length ? (
                <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">{folders.map(child => renderFolderCard(child))}</div>
              ) : null}

              {!scopeEntries.length && !folders.length && (
                <div id="scopes-empty" className="pipelines-empty">
                  <h3 className="text-base font-semibold text-[var(--text-primary)]">No scopes found</h3>
                  <p className="text-sm text-[var(--text-secondary)]">
                    {hasSearch
                      ? `No scope groups matched “${searchTerm.trim()}”.`
                      : canCreateScopeHere
                        ? 'Create a new scope or adjust your filters.'
                        : 'Adjust your filters or browse another group.'}
                  </p>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    );
  };

  const renderDetail = () => {
    if (selectedScope == null) return null;
    const scopeLabel = normalizeScopeLabel(selectedScope);
    const scopeDisplay = formatScopeDisplay(scopeLabel);
    const data = scopeDataByScope[scopeLabel] || createInitialScopeData();
    const variableGroups = groupScopedItems(data.variables);
    const secretGroups = groupScopedItems(data.secrets);

    const renderVariableSection = (title: string, items: GroupedScopedItem[]) => (
      <section key={`var-section-${title || 'global'}`} className="space-y-2">
        {title ? <p className="text-xs uppercase tracking-[0.18em] text-[var(--text-secondary)]">{title}</p> : null}
        <div className="scope-variable-buttons">
          {items.map(item => {
            const isActive = item.full === selectedVariable;
            const cacheKey = `${item.full}@@${scopeLabel}`;
            const isExpanded = expandedVariableKey === cacheKey;
            const value = variableValues[cacheKey] ?? '';
            const displayValue = value ? value : '(empty)';
            const isLoading = variableValueLoadingKey === cacheKey;
            const meta = data.variableMeta[item.full];
            const editable = isEditableSource(meta?.source || 'database');
            return (
              <div
                key={`var-${item.full}`}
                className={`scope-variable-item rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${isActive ? 'scope-variable-item--active' : ''} ${isExpanded ? 'scope-variable-item--expanded' : ''}`}
              >
                <div className="scope-variable-info">
                  <button
                    type="button"
                    className={`scope-variable-btn${isActive ? ' scope-variable-btn--active' : ''}`}
                    onClick={() => selectVariable(item.full)}
                  >
                    <span className="truncate">{item.display}</span>
                  </button>
                  <span className={`scope-variable-source-pill ${sourcePillClass(meta?.source || 'database')}`}>{sourceLabel(meta?.source || 'database')}</span>
                </div>
                <div className="scope-variable-inline-actions">
                  <button
                    type="button"
                    className={`scope-inline-icon${isLoading ? ' loading' : ''}${isExpanded ? ' scope-inline-icon--active' : ''}`}
                    title={isExpanded ? 'Hide value' : 'Show value'}
                    aria-label={isExpanded ? 'Hide value' : 'Show value'}
                    disabled={isLoading}
                    onClick={async event => {
                      event.preventDefault();
                      event.stopPropagation();
                      selectVariable(item.full);
                      await toggleVariableValue(scopeLabel, item.full);
                    }}
                  >
                    {isExpanded ? (
                      <EyeOff className="h-4 w-4" aria-hidden="true" />
                    ) : (
                      <Eye className="h-4 w-4" aria-hidden="true" />
                    )}
                  </button>

                  {editable ? (
                    <>
	                      {canWriteVariablesInSelectedScope && (
                        <button
                          type="button"
                          className="scope-inline-icon"
                          title="Edit variable"
                          onClick={event => {
                            event.preventDefault();
                            event.stopPropagation();
                            selectVariable(item.full);
                            openVariableUpdateModal(scopeLabel, item.full);
                          }}
                        >
                          <Pencil className="h-4 w-4" aria-hidden="true" />
                        </button>
                      )}
                      {canDeleteScopes && (
                        <button
                          type="button"
                          className="scope-inline-icon scope-inline-icon--danger"
                          title="Delete variable"
                          onClick={event => {
                            event.preventDefault();
                            event.stopPropagation();
                            selectVariable(item.full);
                            openDeleteModal('variable', scopeLabel, item.full);
                          }}
                        >
                          <Trash2 className="h-4 w-4" aria-hidden="true" />
                        </button>
                      )}
                    </>
	                  ) : canWriteVariablesInSelectedScope ? (
                    <button
                      type="button"
                      className="scope-inline-icon"
                      title="Clone"
                      onClick={event => {
                        event.preventDefault();
                        event.stopPropagation();
                        openVariableCloneModal(scopeLabel, item.full);
                      }}
                    >
                      <Copy className="h-4 w-4" aria-hidden="true" />
                    </button>
                  ) : null}
                </div>
                <div className="scope-variable-value">{isExpanded ? displayValue : ''}</div>
              </div>
            );
          })}
        </div>
      </section>
    );

    const renderSecretSection = (title: string, items: GroupedScopedItem[]) => (
      <section key={`secret-section-${title || 'global'}`} className="space-y-2">
        {title ? <p className="text-xs uppercase tracking-[0.18em] text-[var(--text-secondary)]">{title}</p> : null}
        <div className="scope-variable-buttons">
          {items.map(item => {
            const isActive = item.full === selectedSecret;
            const meta = data.secretMeta[item.full];
            const editable = isEditableSource(meta?.source || 'database');
            return (
              <div
                key={`secret-${item.full}`}
                className={`scope-variable-item rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${isActive ? ' scope-variable-item--active' : ''}`}
              >
                <div className="scope-variable-info">
                  <button
                    type="button"
                    className={`scope-variable-btn${isActive ? ' scope-variable-btn--active' : ''}`}
                    onClick={() => selectSecret(item.full)}
                  >
                    <span className="truncate">{item.display}</span>
                  </button>
                  <span className={`scope-variable-source-pill ${sourcePillClass(meta?.source || 'database')}`}>{sourceLabel(meta?.source || 'database')}</span>
                </div>
                <div className="scope-variable-inline-actions">
                  {editable ? (
                    <>
	                      {canWriteSecretsInSelectedScope && (
                        <button
                          type="button"
                          className="scope-inline-icon"
                          title="Edit secret"
                          onClick={event => {
                            event.preventDefault();
                            event.stopPropagation();
                            selectSecret(item.full);
                            openSecretUpdateModal(scopeLabel, item.full);
                          }}
                        >
                          <Pencil className="h-4 w-4" aria-hidden="true" />
                        </button>
                      )}
                      {canDeleteScopes && (
                        <button
                          type="button"
                          className="scope-inline-icon scope-inline-icon--danger"
                          title="Delete secret"
                          onClick={event => {
                            event.preventDefault();
                            event.stopPropagation();
                            selectSecret(item.full);
                            openDeleteModal('secret', scopeLabel, item.full);
                          }}
                        >
                          <Trash2 className="h-4 w-4" aria-hidden="true" />
                        </button>
                      )}
                    </>
	                  ) : canWriteSecretsInSelectedScope ? (
                    <button
                      type="button"
                      className="scope-inline-icon"
                      title="Clone"
                      onClick={event => {
                        event.preventDefault();
                        event.stopPropagation();
                        openSecretCloneModal(scopeLabel, item.full);
                      }}
                    >
                      <Copy className="h-4 w-4" aria-hidden="true" />
                    </button>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      </section>
    );

    const variableMeta = selectedVariable ? data.variableMeta[selectedVariable] : undefined;
    const secretMeta = selectedSecret ? data.secretMeta[selectedSecret] : undefined;
    const relatedVariablePipelines = selectedVariable ? Array.from(pipelineVariableIndex.get(selectedVariable) || []) : [];
    const relatedSecretPipelines = selectedSecret ? Array.from(pipelineSecretIndex.get(selectedSecret) || []) : [];
    const scopeTriggers = triggersByScope.get(scopeLabel) || [];

    const activeSelection = selectedVariable
      ? { type: 'variable' as const, name: selectedVariable, meta: variableMeta, pipelines: relatedVariablePipelines }
      : selectedSecret
        ? { type: 'secret' as const, name: selectedSecret, meta: secretMeta, pipelines: relatedSecretPipelines }
        : null;

    return (
      <div id="scopes-detail-view" className="pipelines-view">
        <div className="glass-card p-6">
          <div className="flex items-start justify-between gap-4 w-full">
            <div className="min-w-0 space-y-2">
              <p className="text-xs uppercase tracking-[0.2em] text-[var(--text-secondary)]">Scope</p>
              <h2 className="text-3xl font-bold text-[var(--text-primary)] truncate">{scopeDisplay}</h2>
              <p className="text-sm text-[var(--text-secondary)]">Manage variables and secrets for this scope, all in one view.</p>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                className="pipelines-icon-only"
                aria-label="Encrypt secret for GitOps"
                title="Encrypt secret for GitOps"
                onClick={openGitOpsEncryptModal}
              >
                <KeyRound className="h-4 w-4" aria-hidden="true" />
              </button>
              <ResourceAccessCard resourceType="scope" resourceID={scopeLabel || 'default'} label="scope" sensitive />
              <button type="button" className="glass-button-ghost" onClick={handleBackToList}>
                <ArrowLeft className="h-4 w-4" aria-hidden="true" />
                <span>Back</span>
              </button>
            </div>
          </div>
        </div>

        <div className="grid gap-6 mt-6 lg:grid-cols-[360px_1fr]">
          <div className="space-y-4">
            <div className="glass-card p-4 rounded-2xl border border-[var(--border-primary)]">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Variables</p>
                  <p className="text-xs text-[var(--text-secondary)]">Plain text values.</p>
                </div>
                {canWriteVariablesInSelectedScope && (
                  <button className="glass-button-primary" onClick={() => openVariableCreateModal(scopeLabel)}>
                    New
                  </button>
                )}
              </div>
              {!data.variablesLoading && !data.variables.length ? <div className="scope-panel-empty">No variables configured yet.</div> : null}
              {data.variablesLoading && !data.variablesLoaded ? <div className="scope-panel-empty">Loading variables…</div> : null}
              <div className="scope-variable-list space-y-4">
                {variableGroups.global.length ? renderVariableSection('Global', variableGroups.global) : null}
                {variableGroups.repositories.map(group => renderVariableSection(group.repo, group.items))}
              </div>
            </div>

            <div className="glass-card p-4 rounded-2xl border border-[var(--border-primary)]">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Secrets</p>
                  <p className="text-xs text-[var(--text-secondary)]">Encrypted values.</p>
                </div>
	                {canWriteSecretsInSelectedScope && (
                  <button className="glass-button-primary" onClick={() => openSecretCreateModal(scopeLabel)}>
                    New
                  </button>
                )}
              </div>
              {!data.secretsLoading && !data.secrets.length ? <div className="scope-panel-empty">No secrets configured yet.</div> : null}
              {data.secretsLoading && !data.secretsLoaded ? <div className="scope-panel-empty">Loading secrets…</div> : null}
              <div className="scope-variable-list space-y-4">
                {secretGroups.global.length ? renderSecretSection('Global', secretGroups.global) : null}
                {secretGroups.repositories.map(group => renderSecretSection(group.repo, group.items))}
              </div>
            </div>
          </div>

          <div className="space-y-4">
            <ScopeUsagePanel
              selection={activeSelection}
              pipelineMetadata={pipelineMetadata}
              triggers={scopeTriggers}
              loading={usageLoading}
              error={usageError}
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <div data-page="scopes" className="active h-full flex flex-col">
      {selectedScope === null && (
        <div className="px-6 pt-6 pb-4">
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              className="glass-button-ghost"
              aria-label="Back"
              onClick={() => openFolder(parentFolder(activeFolder))}
              disabled={!activeFolder}
            >
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            </button>

            <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
              <button
                type="button"
                className="pipelines-search-toggle"
                aria-label="Search scopes"
                onClick={() => {
                  setSearchOpen(true);
                  requestAnimationFrame(() => searchInputRef.current?.focus());
                }}
              >
                <Search className="h-4 w-4" aria-hidden="true" />
              </button>
              <input
                ref={searchInputRef}
                id="scopes-search"
                type="text"
                placeholder="Search scopes"
                className="pipelines-search-input"
                value={searchTerm}
                onChange={event => {
                  setSearchTerm(event.target.value);
                  if (event.target.value && !searchOpen) setSearchOpen(true);
                }}
                onBlur={() => {
                  if (!searchTerm.trim()) setSearchOpen(false);
                }}
              />
              {(searchTerm || searchOpen) && (
                <button
                  type="button"
                  className="pipelines-search-clear"
                  onClick={() => {
                    setSearchTerm('');
                    setSearchOpen(false);
                    searchInputRef.current?.blur();
                  }}
                  aria-label="Clear search"
                >
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              )}
            </div>

            <button
              type="button"
              className="pipelines-icon-only"
              aria-label="Encrypt secret for GitOps"
              title="Encrypt secret for GitOps"
              onClick={openGitOpsEncryptModal}
            >
              <KeyRound className="h-4 w-4" aria-hidden="true" />
            </button>

	            {!searchTerm.trim() && canCreateScopeHere && (
              <button
                id="scopes-new-btn"
                type="button"
                className="pipelines-icon-only"
                aria-label="Create new scope"
                title="New Scope"
                onClick={openNewScopeModal}
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">{selectedScope === null ? renderList() : renderDetail()}</div>

      <ScopeWorkflowModals
        scopeModal={scopeModal}
        variableModal={variableModal}
        secretModal={secretModal}
        gitOpsEncryptModal={gitOpsEncryptModal}
        deleteModal={deleteModal}
        canDeleteScopes={canDeleteScopes}
        knownRepositories={knownRepositories}
        variableSuggestionEntries={variableSuggestionEntries}
        secretSuggestionEntries={secretSuggestionEntries}
        onCloseScope={closeScopeModal}
        onUpdateScopeName={updateScopeName}
        onSubmitScope={() => void submitScopeModal()}
        onCloseVariable={closeVariableModal}
        onUpdateVariable={updateVariableModal}
        onChooseVariableSuggestion={chooseVariableSuggestion}
        onSubmitVariable={() => void submitVariableModal()}
        onCloseSecret={closeSecretModal}
        onUpdateSecret={updateSecretModal}
        onChooseSecretSuggestion={chooseSecretSuggestion}
        onSubmitSecret={() => void submitSecretModal()}
        onCloseGitOpsEncrypt={closeGitOpsEncryptModal}
        onUpdateGitOpsEncryptValue={updateGitOpsEncryptValue}
        onEncryptGitOpsSecret={() => void encryptGitOpsSecretValue()}
        onCopyGitOpsEncryptedValue={() => void copyGitOpsEncryptedValue()}
        onCloseDelete={closeDeleteModal}
        onConfirmDelete={() => void confirmDelete()}
      />

      <WorkflowToastRegion toasts={toasts} />
    </div>
  );
}

export default ScopesPage;
