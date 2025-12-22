import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';

const INITIAL_RECENT_RUNS = 5;
const RUNS_PAGE_SIZE = 10;
const RUNS_CACHE_TTL = 60 * 1000;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

const TRIGGER_ROOT_KEYS = ['triggers'];
const TRIGGER_KEYS = ['on', 'branches', 'skip_branches', 'tags', 'pipelines', 'scope'];
const TRIGGER_EVENT_OPTIONS = ['push', 'pull_request', 'schedule'];

type TriggerListItem = { slug: string; source?: string };

type PipelineRef = {
  identifier: string;
  display: string;
  pathLabel: string;
};

type TriggerSummary = {
  triggerCount: number;
  pipelines: PipelineRef[];
  events: string[];
  branches: string[];
  skipBranches: string[];
  tags: string[];
  scopes: string[];
};

type TriggerDetail = {
  slug: string;
  source?: string;
  rawYaml: string;
  summary: TriggerSummary;
};

type PipelineMeta = {
  version: string;
  sourceKey: string;
  sourceLabel: string;
};

type TriggerRun = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  status?: string;
  git_repo_owner?: string;
  git_repo_name?: string;
  git_ref?: string;
  started_at?: string;
  trigger_event_id?: string;
};

type ValidationError = {
  message: string;
  line?: number;
  column?: number;
};

type ValidationResult = {
  errors: ValidationError[];
};

type TreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TreeNode[];
  triggerSlugs: string[];
};

type CreateModalState = {
  repository: string;
  yamlPreview: string;
  pending: boolean;
  error?: string;
};

type CloneModalState = {
  repository: string;
  pending: boolean;
  error?: string;
};

type DeleteModalState = {
  slug: string;
  pending: boolean;
  error?: string;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object') return null;
  return value as Record<string, unknown>;
}

function normalizeSource(source?: string): string {
  const key = (source || '').trim().toLowerCase();
  if (!key) return 'database';
  if (key.includes('git')) return 'git';
  if (key.includes('draft')) return 'draft';
  if (key.includes('db') || key.includes('database')) return 'database';
  if (key.includes('local')) return 'local';
  return key;
}

function sourceLabel(sourceKey: string): string {
  switch (normalizeSource(sourceKey)) {
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

function normalizePipelineIdentifier(value: unknown): string {
  if (!value) return '';
  return String(value)
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^pipelines\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\//, '');
}

function describePipeline(identifier: string): PipelineRef {
  const segments = identifier.split('/').filter(Boolean);
  const name = segments.pop() || identifier;
  const path = segments.join('/');
  return { identifier, display: name, pathLabel: path || 'root' };
}

function parseTriggerYaml(raw: string): Record<string, unknown> {
  const parsed = yaml.load(raw) as Record<string, unknown> | undefined;
  if (!parsed || typeof parsed !== 'object') {
    throw new Error('Manifest must be a YAML object.');
  }
  const triggerValue = (parsed as { triggers?: unknown }).triggers;
  if (!Array.isArray(triggerValue) || triggerValue.length === 0) {
    throw new Error('Manifest must contain a non-empty "triggers" array.');
  }
  triggerValue.forEach((trigger: unknown, idx: number) => {
    const triggerRecord = asRecord(trigger);
    if (!triggerRecord) {
      throw new Error(`Trigger #${idx + 1} must be an object.`);
    }
    if (Object.prototype.hasOwnProperty.call(triggerRecord, 'environment')) {
      throw new Error(`Trigger #${idx + 1} uses deprecated 'environment'. Rename it to 'scope'.`);
    }
  });
  return parsed;
}

function buildTriggerSummary(manifest: Record<string, unknown>): TriggerSummary {
  const triggerValue = (manifest as { triggers?: unknown }).triggers;
  const triggers = Array.isArray(triggerValue) ? triggerValue : [];
  const pipelineIdentifiers: PipelineRef[] = [];
  const events = new Set<string>();
  const branches = new Set<string>();
  const skipBranches = new Set<string>();
  const tags = new Set<string>();
  const scopes = new Set<string>();
  let hasDefaultScope = false;

  triggers.forEach(triggerValueItem => {
    const trigger = asRecord(triggerValueItem);
    if (!trigger) return;

    if (trigger.on != null) {
      const raw = String(trigger.on).trim();
      events.add(raw);
    }

    const branchesList = Array.isArray(trigger.branches) ? trigger.branches : [];
    branchesList.forEach(branch => {
      const value = String(branch || '').trim();
      if (value) branches.add(value);
    });

    const skipValue = trigger.skip_branches ?? trigger.skipBranches;
    const skipList = Array.isArray(skipValue) ? skipValue : [];
    skipList.forEach(branch => {
      const value = String(branch || '').trim();
      if (value) skipBranches.add(value);
    });

    const tagsList = Array.isArray(trigger.tags) ? trigger.tags : [];
    tagsList.forEach(tag => {
      const value = String(tag || '').trim();
      if (value) tags.add(value);
    });

    const scope = typeof trigger.scope === 'string' ? trigger.scope.trim() : '';
    if (scope) {
      scopes.add(scope);
    } else {
      hasDefaultScope = true;
    }

    const pipelines = Array.isArray(trigger.pipelines) ? trigger.pipelines : [];
    pipelines.forEach(entry => {
      const entryRecord = asRecord(entry);
      const raw = typeof entry === 'string' ? entry : typeof entryRecord?.path === 'string' ? entryRecord.path : '';
      const normalized = normalizePipelineIdentifier(raw);
      if (!normalized) return;
      pipelineIdentifiers.push(describePipeline(normalized));
    });
  });

  if (hasDefaultScope) scopes.add('');

  const dedupedPipelines = Array.from(new Map(pipelineIdentifiers.map(item => [item.identifier, item])).values()).sort((a, b) =>
    a.identifier.localeCompare(b.identifier)
  );

  return {
    triggerCount: triggers.length,
    pipelines: dedupedPipelines,
    events: Array.from(events).sort((a, b) => a.localeCompare(b)),
    branches: Array.from(branches).sort((a, b) => a.localeCompare(b)),
    skipBranches: Array.from(skipBranches).sort((a, b) => a.localeCompare(b)),
    tags: Array.from(tags).sort((a, b) => a.localeCompare(b)),
    scopes: Array.from(scopes).sort((a, b) => a.localeCompare(b)),
  };
}

function parseTriggerOverrideList(payload: unknown): TriggerListItem[] {
  if (!Array.isArray(payload)) return [];
  const items: TriggerListItem[] = [];
  payload.forEach(entry => {
    if (entry == null) return;
    if (typeof entry === 'string') {
      const slug = entry.trim();
      if (!slug) return;
      items.push({ slug, source: 'database' });
      return;
    }
    const record = asRecord(entry);
    if (!record) return;
    const slugRaw =
      (typeof record.name === 'string' && record.name) ||
      (typeof record.repository_name === 'string' && record.repository_name) ||
      (typeof record.slug === 'string' && record.slug) ||
      (typeof record.repo === 'string' && record.repo) ||
      '';
    const slug = String(slugRaw || '').trim();
    if (!slug) return;
    const source = typeof record.source === 'string' ? normalizeSource(record.source) : '';
    items.push({ slug, source });
  });
  items.sort((a, b) => a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' }));
  return items;
}

function buildPipelineIdentifierFromRun(run: TriggerRun): string {
  const name = run.pipeline_name || '';
  const path = run.pipeline_path || '';
  const identifier = path ? `${path}/${name}` : name;
  return normalizePipelineIdentifier(identifier);
}

function sanitizePipelineFileName(value: string): string {
  const trimmed = String(value || '').trim();
  const fallback = 'sample-pipeline';
  if (!trimmed) return fallback;
  const sanitized = trimmed.replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, '');
  return sanitized || fallback;
}

function deriveDefaultPipelinePath(repoSlug: string): string {
  const parts = String(repoSlug || '').split('/').filter(Boolean);
  const candidate = parts[parts.length - 1] || '';
  const fileName = sanitizePipelineFileName(candidate);
  return `pipelines/${fileName}.yaml`;
}

function buildNewTriggerYaml(pipelinePath: string): string {
  const path = pipelinePath || 'pipelines/sample-pipeline.yaml';
  return `triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - ${path}\n`;
}

function TriggersPage() {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverTriggers, setServerTriggers] = useState<TriggerListItem[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const [selectedSlug, setSelectedSlug] = useState<string | null>(null);
  const selectedSlugRef = useRef<string | null>(null);

  const [detail, setDetail] = useState<TriggerDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  const [linkedPipelines, setLinkedPipelines] = useState<PipelineRef[]>([]);
  const [recentRuns, setRecentRuns] = useState<TriggerRun[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string | null>(null);

  const [isEditing, setIsEditing] = useState(false);
  const [editorValue, setEditorValue] = useState('');
  const [saving, setSaving] = useState(false);

  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const highlightContentRef = useRef<HTMLPreElement | null>(null);
  const lineNumbersRef = useRef<HTMLDivElement | null>(null);
  const editSessionOriginalYamlRef = useRef<string>('');
  const wasEditingRef = useRef(false);
  const autoEnterEditSlugRef = useRef<string | null>(null);

  const runsCacheRef = useRef<{ fetchedAt: number; runs: TriggerRun[] }>({ fetchedAt: 0, runs: [] });
  const recentRunsAllRef = useRef<TriggerRun[]>([]);
  const recentRunsListRef = useRef<HTMLUListElement | null>(null);

  const pipelineSourceIndexRef = useRef<Map<string, string> | null>(null);
  const pipelineMetaCacheRef = useRef<Map<string, PipelineMeta>>(new Map());
  const pipelineMetaPromiseRef = useRef<Map<string, Promise<void>>>(new Map());
  const [, bumpPipelineMetaRevision] = useState(0);

  const autocompleteFetchRef = useRef<{ fetchedAt: number; loadingPromise: Promise<void> | null }>({
    fetchedAt: 0,
    loadingPromise: null,
  });

  const [autocompleteMeta, setAutocompleteMeta] = useState<{
    pipelines: string[];
    scopes: string[];
    fetchedAt: number;
    loading: boolean;
  }>({ pipelines: [], scopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<null | {
    title: string;
    items: string[];
    activeIndex: number;
    replaceStart: number;
    replaceEnd: number;
    appendColon: boolean;
  }>(null);

  const [createModal, setCreateModal] = useState<CreateModalState | null>(null);
  const [cloneModal, setCloneModal] = useState<CloneModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<DeleteModalState | null>(null);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = useCallback((message: string, tone: ToastMessage['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const encodeSlug = (slug: string) => slug.split('/').map(encodeURIComponent).join('/');

  const splitSlug = (slug: string) => {
    const parts = slug.split('/').filter(Boolean);
    if (parts.length < 2) throw new Error('Repository must be in owner/name format.');
    const repo = parts.pop()!;
    const owner = parts.join('/');
    if (!owner || !repo) throw new Error('Repository must be in owner/name format.');
    return { owner, repo };
  };

  const slugToLabel = (slug: string) => {
    const parts = slug.split('/').filter(Boolean);
    const name = parts.pop() || slug;
    const path = parts.join('/') || 'root';
    return { name, path };
  };

  const validateTriggerYaml = useCallback((rawYaml: string): ValidationResult => {
    const errors: ValidationError[] = [];
    let parsed: unknown = null;
    try {
      parsed = yaml.load(rawYaml) as unknown;
    } catch (error: unknown) {
      const errorRecord = asRecord(error);
      const markRecord = asRecord(errorRecord?.mark);
      errors.push({
        message: error instanceof Error ? error.message : 'Invalid YAML',
        line: typeof markRecord?.line === 'number' ? markRecord.line + 1 : undefined,
        column: typeof markRecord?.column === 'number' ? markRecord.column + 1 : undefined,
      });
      return { errors };
    }

    if (!parsed || typeof parsed !== 'object') {
      return { errors: [{ message: 'YAML must define an object at the root.' }] };
    }

    const root = asRecord(parsed);
    const triggers = Array.isArray(root?.triggers) ? root?.triggers : [];
    if (triggers.length === 0) {
      errors.push({ message: "'triggers' must be a non-empty list." });
      return { errors };
    }

    triggers.forEach((trigger, index: number) => {
      const triggerRecord = asRecord(trigger);
      if (!triggerRecord) {
        errors.push({ message: `Trigger #${index + 1} must be an object.` });
        return;
      }
      if (Object.prototype.hasOwnProperty.call(triggerRecord, 'environment')) {
        errors.push({ message: `Trigger #${index + 1} uses deprecated 'environment'. Rename it to 'scope'.` });
      }
    });

    return { errors };
  }, []);

  const validation = useMemo(() => {
    if (!isEditing) return { errors: [] };
    return validateTriggerYaml(editorValue);
  }, [editorValue, isEditing, validateTriggerYaml]);

  const validationErrorLines = useMemo(() => {
    const lines = new Set<number>();
    validation.errors.forEach(err => {
      if (typeof err.line === 'number') lines.add(err.line);
    });
    return lines;
  }, [validation.errors]);

  const syncEditorOverlays = useCallback((textarea: HTMLTextAreaElement | null) => {
    if (!textarea) return;
    const x = textarea.scrollLeft || 0;
    const y = textarea.scrollTop || 0;
    if (highlightContentRef.current) {
      highlightContentRef.current.style.transform = `translate(${-x}px, ${-y}px)`;
    }
    if (lineNumbersRef.current) {
      lineNumbersRef.current.style.setProperty('--line-number-scroll', `${y}px`);
    }
  }, []);

  const handleEditorScroll = useCallback(
    (event: UIEvent<HTMLTextAreaElement>) => {
      syncEditorOverlays(event.currentTarget);
    },
    [syncEditorOverlays]
  );

  const applyEditorSuggestion = useCallback(
    (value: string) => {
      const suggestion = editorSuggestion;
      if (!suggestion) return;
      const insertText = suggestion.appendColon ? `${value}: ` : value;
      const nextValue = `${editorValue.slice(0, suggestion.replaceStart)}${insertText}${editorValue.slice(suggestion.replaceEnd)}`;
      const nextCursor = suggestion.replaceStart + insertText.length;
      setEditorSuggestion(null);
      setEditorValue(nextValue);
      requestAnimationFrame(() => {
        const el = editorRef.current;
        if (!el) return;
        el.focus();
        el.selectionStart = nextCursor;
        el.selectionEnd = nextCursor;
        syncEditorOverlays(el);
      });
    },
    [editorSuggestion, editorValue, syncEditorOverlays]
  );

  const openEditorSuggestion = useCallback(
    (cursor: number, nextText?: string) => {
      const text = nextText ?? editorValue;
      const before = text.slice(0, cursor);
      const lineStart = before.lastIndexOf('\n') + 1;
      const lineBeforeCursor = text.slice(lineStart, cursor);
      const prefixMatch = lineBeforeCursor.match(/[A-Za-z0-9_./-]+$/);
      const prefix = prefixMatch ? prefixMatch[0] : '';
      const replaceStart = cursor - prefix.length;
      const replaceEnd = cursor;

      const lines = text.split('\n');
      const lineIndex = before.split('\n').length - 1;
      const currentLine = lines[lineIndex] || '';
      const currentIndent = currentLine.match(/^\s*/)?.[0].length ?? 0;

      const currentKeyMatch = currentLine.match(/^\s*-?\s*([A-Za-z0-9_.-]+)\s*:\s*/);
      const currentKey = currentKeyMatch?.[1] || '';

      let parentKey = '';
      for (let i = lineIndex; i >= 0; i -= 1) {
        const rawLine = lines[i];
        const trimmed = rawLine.trim();
        if (!trimmed || trimmed.startsWith('#')) continue;
        const indent = rawLine.match(/^\s*/)?.[0].length ?? 0;
        if (indent < currentIndent) {
          const match = rawLine.match(/^\s*-?\s*([A-Za-z0-9_.-]+)\s*:\s*/);
          if (match) {
            parentKey = match[1];
            break;
          }
        }
      }

      const trimmedLineBefore = lineBeforeCursor.trim();
      const isKeyContext = !trimmedLineBefore.includes(':') && /^-?\s*[A-Za-z0-9_.-]*$/.test(trimmedLineBefore.replace(/^-/, '').trim());

      const pipelineValueContext = parentKey === 'pipelines';
      const scopeValueContext =
        currentKey === 'scope' || /^\s*scope\s*:\s*[A-Za-z0-9_./-]*$/.test(trimmedLineBefore);
      const onValueContext = currentKey === 'on' || /^\s*on\s*:\s*[A-Za-z0-9_.-]*$/.test(trimmedLineBefore);

      let title = 'Suggestions';
      let pool: string[] = [];
      let appendColon = false;

      if (pipelineValueContext) {
        title = 'Pipelines';
        pool = autocompleteMeta.pipelines;
      } else if (scopeValueContext) {
        title = 'Scopes';
        pool = autocompleteMeta.scopes;
      } else if (onValueContext) {
        title = 'Events';
        pool = TRIGGER_EVENT_OPTIONS;
      } else if (isKeyContext) {
        appendColon = true;
        title = currentIndent === 0 ? 'Root keys' : 'Trigger keys';
        pool = currentIndent === 0 ? TRIGGER_ROOT_KEYS : TRIGGER_KEYS;
      } else {
        title = 'Suggestions';
        pool = [];
      }

      const normalizedPrefix = prefix.toLowerCase();
      const filtered = pool
        .filter(item => item.toLowerCase().startsWith(normalizedPrefix))
        .sort((a, b) => a.localeCompare(b));

      setEditorSuggestion({
        title,
        items: filtered.slice(0, 50),
        activeIndex: 0,
        replaceStart,
        replaceEnd,
        appendColon,
      });
    },
    [autocompleteMeta.pipelines, autocompleteMeta.scopes, editorValue]
  );

  const ensureAutocompleteMeta = useCallback(
    async (force?: boolean) => {
      const now = Date.now();
      if (
        !force &&
        autocompleteFetchRef.current.fetchedAt &&
        now - autocompleteFetchRef.current.fetchedAt < AUTOCOMPLETE_REFRESH_INTERVAL
      ) {
        return;
      }
      if (autocompleteFetchRef.current.loadingPromise) {
        await autocompleteFetchRef.current.loadingPromise;
        return;
      }

      setAutocompleteMeta(prev => ({ ...prev, loading: true }));
      try {
        const normalizeScopes = (payload: unknown): string[] => {
          if (!Array.isArray(payload)) return [];
          return payload
            .map(item => {
              if (typeof item === 'string') return item.trim();
              const record = asRecord(item);
              const scope = typeof record?.scope === 'string' ? record.scope : '';
              if (scope) return scope.trim();
              return '';
            })
            .filter(Boolean);
        };

        const normalizePipelines = (payload: unknown): { list: string[]; sourceIndex: Map<string, string> } => {
          const sourceIndex = new Map<string, string>();
          if (!Array.isArray(payload)) return { list: [], sourceIndex };
          payload.forEach(item => {
            const record = asRecord(item);
            const idRaw =
              typeof item === 'string'
                ? item
                : typeof record?.id === 'string'
                  ? record.id
                  : typeof record?.ID === 'string'
                    ? record.ID
                    : typeof record?.identifier === 'string'
                      ? record.identifier
                      : '';
            const id = normalizePipelineIdentifier(idRaw);
            if (!id) return;
            const source = typeof record?.source === 'string' ? normalizeSource(record.source) : 'database';
            if (source) sourceIndex.set(id, source);
          });
          const list = Array.from(sourceIndex.keys()).sort((a, b) => a.localeCompare(b));
          return { list, sourceIndex };
        };

        const promise = (async () => {
          const [pipelineResp, scopeResp] = await Promise.all([
            fetch(buildApiUrl('/v1/pipelines?include_source=true')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/variables/scopes')).then(r => (r.ok ? r.json() : [])),
          ]);

          const { list: pipelines, sourceIndex } = normalizePipelines(pipelineResp);
          pipelineSourceIndexRef.current = sourceIndex;

          setAutocompleteMeta({
            pipelines,
            scopes: normalizeScopes(scopeResp),
            fetchedAt: Date.now(),
            loading: false,
          });

          autocompleteFetchRef.current.fetchedAt = Date.now();
        })();

        autocompleteFetchRef.current.loadingPromise = promise;
        await promise;
      } catch (error) {
        console.warn('Failed to load autocomplete metadata', error);
        setAutocompleteMeta(prev => ({ ...prev, loading: false }));
      } finally {
        autocompleteFetchRef.current.loadingPromise = null;
      }
    },
    []
  );

  const ensurePipelineMeta = useCallback(async (pipelineId: string): Promise<PipelineMeta | null> => {
    const normalized = normalizePipelineIdentifier(pipelineId);
    if (!normalized) return null;
    const cached = pipelineMetaCacheRef.current.get(normalized);
    if (cached) return cached;

    const pending = pipelineMetaPromiseRef.current.get(normalized);
    if (pending) {
      await pending;
      return pipelineMetaCacheRef.current.get(normalized) ?? null;
    }

    const promise = (async () => {
      const sourceKey = pipelineSourceIndexRef.current?.get(normalized) || 'local';
      let version = 'latest';
	      try {
	        const response = await fetch(buildApiUrl(`/v1/pipelines/${normalized.split('/').map(encodeURIComponent).join('/')}`));
	        if (response.ok) {
	          const rawYaml = await response.text();
	          const parsed = asRecord(yaml.load(rawYaml) as unknown);
	          const parsedVersion = typeof parsed?.version === 'string' ? parsed.version.trim() : '';
	          if (parsedVersion) {
	            version = parsedVersion;
	          }
	        }
	      } catch (error) {
	        console.warn('Failed to load pipeline meta', error);
      }
      pipelineMetaCacheRef.current.set(normalized, { version, sourceKey, sourceLabel: sourceLabel(sourceKey) });
      bumpPipelineMetaRevision(prev => prev + 1);
    })();

    pipelineMetaPromiseRef.current.set(normalized, promise);
    try {
      await promise;
    } finally {
      pipelineMetaPromiseRef.current.delete(normalized);
    }
    return pipelineMetaCacheRef.current.get(normalized) ?? null;
  }, []);

  const loadTriggers = useCallback(async () => {
    setListLoading(true);
    setListError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/overrides?include_source=true'));
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load triggers (${response.status})`);
      }
      const payload = await response.json();
      setServerTriggers(parseTriggerOverrideList(payload));
    } catch (error) {
      console.error('Failed to load triggers', error);
      setListError(error instanceof Error ? error.message : 'Unable to load triggers');
      setServerTriggers([]);
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadTriggerDetail = useCallback(async (slug: string, source?: string) => {
    const target = slug;
    setDetailLoading(true);
    setDetailError(null);
    try {
      const { owner, repo } = splitSlug(slug);
      const response = await fetch(buildApiUrl(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`));
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load trigger (${response.status})`);
      }
      const rawYaml = await response.text();
      const manifest = parseTriggerYaml(rawYaml);
      const summary = buildTriggerSummary(manifest);
      if (selectedSlugRef.current !== target) return;
      setDetail({ slug, source, rawYaml, summary });
      setLinkedPipelines(summary.pipelines);
      setEditorValue(rawYaml);
      setIsEditing(false);
    } catch (error) {
      console.error('Failed to load trigger', error);
      if (selectedSlugRef.current === target) {
        setDetail(null);
        setLinkedPipelines([]);
        setEditorValue('');
        setIsEditing(false);
        setDetailError(error instanceof Error ? error.message : 'Unable to load trigger');
      }
    } finally {
      if (selectedSlugRef.current === target) {
        setDetailLoading(false);
      }
    }
  }, []);

  const formatRelativeTime = (value?: string) => {
    if (!value) return 'N/A';
    const timestamp = new Date(value).getTime();
    if (Number.isNaN(timestamp)) return value;
    const delta = (Date.now() - timestamp) / 1000;
    if (delta < 60) return 'Just now';
    if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
    if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
    return `${Math.floor(delta / 86400)}d ago`;
  };

  const formatRef = (ref?: string) => {
    if (!ref) return '—';
    return ref.replace(/^refs\/heads\//i, '').replace(/^refs\/tags\//i, '');
  };

  const statusClass = (status?: string) => {
    const key = (status || '').toLowerCase();
    if (key === 'success' || key === 'succeeded') return 'runner-pill--ok';
    if (key === 'failure' || key === 'failed' || key === 'error' || key === 'cancelled') return 'runner-pill--error';
    return 'runner-pill--muted';
  };

  const statusLabel = (status?: string) => {
    const value = (status || '').replace(/_/g, ' ').trim();
    if (!value) return 'unknown';
    return value.charAt(0).toUpperCase() + value.slice(1);
  };

  const loadRecentRuns = useCallback(async (slug: string, pipelines: PipelineRef[]) => {
    const target = slug;
    setRunsLoading(true);
    setRunsError(null);
    try {
      const now = Date.now();
      if (!runsCacheRef.current.runs.length || now - runsCacheRef.current.fetchedAt > RUNS_CACHE_TTL) {
        const response = await fetch(buildApiUrl('/v1/runs'));
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to load runs (${response.status})`);
        }
        const payload = await response.json();
        runsCacheRef.current = { runs: Array.isArray(payload) ? payload : [], fetchedAt: Date.now() };
      }

      const { owner, repo } = splitSlug(slug);
      const pipelineSet = new Set(pipelines.map(item => item.identifier));
      const normalizedOwner = owner.toLowerCase();
      const normalizedRepo = repo.toLowerCase();

      const filtered = (runsCacheRef.current.runs || [])
        .filter(run => {
          const runOwner = (run.git_repo_owner || '').toLowerCase();
          const runRepo = (run.git_repo_name || '').toLowerCase();
          if (runOwner !== normalizedOwner || runRepo !== normalizedRepo) return false;
          if (!pipelineSet.size) return true;
          const pipelineIdentifier = buildPipelineIdentifierFromRun(run);
          return pipelineSet.has(pipelineIdentifier);
        })
        .sort((a, b) => {
          const aTime = new Date(a.started_at || '').getTime() || 0;
          const bTime = new Date(b.started_at || '').getTime() || 0;
          return bTime - aTime;
        });

      if (selectedSlugRef.current === target) {
        recentRunsAllRef.current = filtered;
        setRecentRuns(filtered.slice(0, INITIAL_RECENT_RUNS));
        requestAnimationFrame(() => {
          recentRunsListRef.current?.scrollTo({ top: 0 });
        });
      }
    } catch (error) {
      console.error('Failed to load runs', error);
      if (selectedSlugRef.current === target) {
        setRunsError(error instanceof Error ? error.message : 'Unable to load runs');
        recentRunsAllRef.current = [];
        setRecentRuns([]);
      }
    } finally {
      if (selectedSlugRef.current === target) {
        setRunsLoading(false);
      }
    }
  }, []);

  const loadMoreRuns = useCallback(() => {
    setRecentRuns(prev => {
      const allRuns = recentRunsAllRef.current;
      if (!allRuns.length || prev.length >= allRuns.length) return prev;
      const nextCount = Math.min(prev.length + RUNS_PAGE_SIZE, allRuns.length);
      return allRuns.slice(0, nextCount);
    });
  }, []);

  const handleRecentRunsScroll = useCallback(
    (event: UIEvent<HTMLUListElement>) => {
      const list = event.currentTarget;
      if (!list) return;
      const remaining = list.scrollHeight - list.scrollTop - list.clientHeight;
      if (remaining > 80) return;
      if (recentRuns.length >= recentRunsAllRef.current.length) return;
      loadMoreRuns();
    },
    [loadMoreRuns, recentRuns.length],
  );

  const parentFolder = (path: string) => {
    const parts = path.split('/').filter(Boolean);
    parts.pop();
    return parts.join('/');
  };

  const folderForSlug = (slug: string) => {
    const parts = slug.split('/').filter(Boolean);
    parts.pop();
    return parts.join('/');
  };

  const openFolder = (path: string) => {
    const cleaned = path.trim().replace(/^\/+|\/+$/g, '');
    setActiveFolder(cleaned);
    setSelectedSlug(null);
    selectedSlugRef.current = null;
    navigate(cleaned ? `/triggers?folder=${encodeURIComponent(cleaned)}` : '/triggers');
  };

  const handleSelectSlug = (slug: string) => {
    selectedSlugRef.current = slug;
    setSelectedSlug(slug);
    navigate(`/triggers/${encodeSlug(slug)}`);
  };

  const handleBackToList = () => {
    if (!detail) {
      navigate('/triggers');
      return;
    }
    openFolder(folderForSlug(detail.slug));
  };

  const openCreateModal = () => {
    const yamlPreview = buildNewTriggerYaml(deriveDefaultPipelinePath(''));
    setCreateModal({ repository: '', yamlPreview, pending: false });
  };

  const openCloneModal = () => {
    if (!detail) {
      addToast('Select a trigger to clone.', 'info');
      return;
    }
    setCloneModal({ repository: detail.slug, pending: false });
  };

  const openDeleteModal = (slug: string) => {
    const source = normalizeSource(serverTriggers.find(item => item.slug === slug)?.source);
    if (source === 'git') {
      addToast('This trigger is managed via Git. Clone it to customize instead of deleting.', 'info');
      return;
    }
    setDeleteModal({ slug, pending: false });
  };

  const handleCopyYaml = async () => {
    if (!detail?.rawYaml) return;
    try {
      await navigator.clipboard.writeText(detail.rawYaml);
      addToast('Trigger YAML copied to clipboard.', 'success');
    } catch (error) {
      console.error('Copy failed', error);
      addToast('Unable to copy YAML.', 'error');
    }
  };

  const handleDownloadYaml = () => {
    if (!detail?.rawYaml) return;
    const blob = new Blob([detail.rawYaml], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${detail.slug.replace(/\//g, '_') || 'trigger'}.yaml`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const handleSave = async () => {
    if (!detail) return;
    if (normalizeSource(detail.source) === 'git') {
      addToast('Git-managed triggers are read-only. Clone it to customize.', 'info');
      return;
    }
    if (validation.errors.length) {
      addToast('Resolve validation errors before saving.', 'error');
      return;
    }
    if (editorValue === detail.rawYaml) {
      setIsEditing(false);
      return;
    }
    setSaving(true);
    try {
      const { owner, repo } = splitSlug(detail.slug);
      const response = await fetch(buildApiUrl(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: editorValue,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to save trigger (${response.status})`);
      }
      const manifest = parseTriggerYaml(editorValue);
      const summary = buildTriggerSummary(manifest);
      setDetail(prev => (prev ? { ...prev, rawYaml: editorValue, summary } : prev));
      setLinkedPipelines(summary.pipelines);
      setIsEditing(false);
      addToast('Trigger saved.', 'success');
      await loadTriggers();
      void loadRecentRuns(detail.slug, summary.pipelines);
    } catch (error) {
      console.error('Save failed', error);
      addToast(error instanceof Error ? error.message : 'Unable to save trigger', 'error');
    } finally {
      setSaving(false);
    }
  };

  const submitCreateModal = async () => {
    if (!createModal) return;
    const repoSlug = createModal.repository.trim();
    if (!repoSlug) {
      setCreateModal(prev => (prev ? { ...prev, error: 'Repository is required.' } : prev));
      return;
    }
    let owner: string;
    let repo: string;
    try {
      ({ owner, repo } = splitSlug(repoSlug));
    } catch (error) {
      setCreateModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Invalid repository.' } : prev));
      return;
    }

    setCreateModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const yamlBody = createModal.yamlPreview;
      const response = await fetch(buildApiUrl(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: yamlBody,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to create trigger (${response.status})`);
      }
      setCreateModal(null);
      addToast('Trigger created.', 'success');
      await loadTriggers();
      autoEnterEditSlugRef.current = repoSlug;
      handleSelectSlug(repoSlug);
    } catch (error) {
      console.error('Create failed', error);
      setCreateModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to create trigger' } : prev));
    } finally {
      setCreateModal(prev => (prev ? { ...prev, pending: false } : prev));
    }
  };

  const submitCloneModal = async () => {
    if (!cloneModal || !detail) return;
    const targetSlug = cloneModal.repository.trim();
    if (!targetSlug) {
      setCloneModal(prev => (prev ? { ...prev, error: 'Repository is required.' } : prev));
      return;
    }

    let owner: string;
    let repo: string;
    try {
      ({ owner, repo } = splitSlug(targetSlug));
    } catch (error) {
      setCloneModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Invalid repository.' } : prev));
      return;
    }

    setCloneModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const response = await fetch(buildApiUrl(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: detail.rawYaml,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to clone trigger (${response.status})`);
      }
      setCloneModal(null);
      addToast('Trigger cloned.', 'success');
      await loadTriggers();
      autoEnterEditSlugRef.current = targetSlug;
      handleSelectSlug(targetSlug);
    } catch (error) {
      console.error('Clone failed', error);
      setCloneModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to clone trigger' } : prev));
    } finally {
      setCloneModal(prev => (prev ? { ...prev, pending: false } : prev));
    }
  };

  const confirmDelete = async () => {
    if (!deleteModal) return;
    const slug = deleteModal.slug;
    let owner: string;
    let repo: string;
    try {
      ({ owner, repo } = splitSlug(slug));
    } catch (error) {
      setDeleteModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Invalid repository.' } : prev));
      return;
    }

    setDeleteModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const response = await fetch(buildApiUrl(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`), { method: 'DELETE' });
      if (!response.ok && response.status !== 204) {
        const text = await response.text();
        throw new Error(text || `Failed to delete trigger (${response.status})`);
      }
      setDeleteModal(null);
      addToast('Trigger deleted.', 'success');
      await loadTriggers();
      setSelectedSlug(null);
      selectedSlugRef.current = null;
      navigate('/triggers');
    } catch (error) {
      console.error('Delete failed', error);
      setDeleteModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to delete trigger' } : prev));
    } finally {
      setDeleteModal(prev => (prev ? { ...prev, pending: false } : prev));
    }
  };

  useEffect(() => {
    void loadTriggers();
  }, [loadTriggers]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'triggers') return;
    if (segments.length > 1) {
      const slug = segments.slice(1).map(decodeURIComponent).join('/');
      if (slug !== selectedSlugRef.current) {
        setSelectedSlug(slug);
        selectedSlugRef.current = slug;
      }
    } else if (selectedSlugRef.current) {
      setSelectedSlug(null);
      selectedSlugRef.current = null;
    }

    const params = new URLSearchParams(location.search);
    const folder = params.get('folder') || '';
    setActiveFolder(folder);
  }, [location.pathname, location.search]);

  useEffect(() => {
    if (listLoading || listError) return;
    const active = selectedSlugRef.current;
    if (!active) return;
    if (serverTriggers.some(item => item.slug === active)) return;
    setSelectedSlug(null);
    selectedSlugRef.current = null;
    navigate('/triggers', { replace: true });
  }, [listLoading, listError, serverTriggers, navigate]);

  useEffect(() => {
    if (!selectedSlug) {
      setDetail(null);
      setLinkedPipelines([]);
      setRecentRuns([]);
      recentRunsAllRef.current = [];
      setRunsError(null);
      setEditorValue('');
      setIsEditing(false);
      return;
    }
    const source = serverTriggers.find(item => item.slug === selectedSlug)?.source;
    void loadTriggerDetail(selectedSlug, source);
  }, [selectedSlug, serverTriggers, loadTriggerDetail]);

  useEffect(() => {
    if (!detail) return;
    void (async () => {
      await ensureAutocompleteMeta();
      await Promise.all(linkedPipelines.map(p => ensurePipelineMeta(p.identifier)));
      await loadRecentRuns(detail.slug, linkedPipelines);
    })();
  }, [detail, linkedPipelines, ensureAutocompleteMeta, ensurePipelineMeta, loadRecentRuns]);

  useEffect(() => {
    if (isEditing && !wasEditingRef.current) {
      wasEditingRef.current = true;
      editSessionOriginalYamlRef.current = editorValue;
    }
    if (!isEditing && wasEditingRef.current) {
      wasEditingRef.current = false;
    }
  }, [editorValue, isEditing]);

  useEffect(() => {
    if (!isEditing) return;
    void ensureAutocompleteMeta();
  }, [isEditing, ensureAutocompleteMeta]);

  useEffect(() => {
    if (!isEditing) return;
    syncEditorOverlays(editorRef.current);
  }, [isEditing, editorValue, syncEditorOverlays]);

  useEffect(() => {
    const desired = autoEnterEditSlugRef.current;
    if (!desired || !detail) return;
    if (detail.slug !== desired) return;
    if (normalizeSource(detail.source) === 'git') return;
    autoEnterEditSlugRef.current = null;
    setIsEditing(true);
  }, [detail]);

  const filteredTriggers = useMemo(() => {
    const query = searchTerm.trim().toLowerCase();
    if (!query) return serverTriggers;
    return serverTriggers.filter(item => item.slug.toLowerCase().includes(query));
  }, [serverTriggers, searchTerm]);

  const visibleTriggers = useMemo(() => {
    const list = searchTerm.trim()
      ? filteredTriggers
      : filteredTriggers.filter(item => slugToLabel(item.slug).path === (activeFolder || 'root'));
    return [...list].sort((a, b) => a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' }));
  }, [filteredTriggers, searchTerm, activeFolder]);

  const buildTree = useMemo(() => {
    const root: TreeNode = { id: '__root__', name: '', fullPath: '', children: [], triggerSlugs: [] };
    serverTriggers.forEach(item => {
      const parts = item.slug.split('/').filter(Boolean);
      const triggerName = parts.pop();
      if (!triggerName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], triggerSlugs: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.triggerSlugs.push(item.slug);
      current.triggerSlugs.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [serverTriggers]);

  const activeFolderNode = useMemo(() => {
    if (!activeFolder) return buildTree;
    const segments = activeFolder.split('/').filter(Boolean);
    let current: TreeNode | null = buildTree;
    for (const segment of segments) {
      const nextNode: TreeNode | undefined = current?.children.find(child => child.name === segment);
      if (!nextNode) return buildTree;
      current = nextNode;
    }
    return current || buildTree;
  }, [activeFolder, buildTree]);

  const countTriggersRecursive = (node: TreeNode): number => {
    const own = node.triggerSlugs.length;
    if (!node.children.length) return own;
    return own + node.children.reduce((sum, child) => sum + countTriggersRecursive(child), 0);
  };

  const renderFolderCard = (node: TreeNode) => (
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
          </div>
        </div>
        <span className="pipeline-folder-chevron">›</span>
      </div>
      <div className="pipeline-folder-meta">
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Triggers:</span>
          <span className="pipeline-card-meta-value">{countTriggersRecursive(node)}</span>
        </div>
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Sub folders:</span>
          <span className="pipeline-card-meta-value">{node.children.length}</span>
        </div>
      </div>
    </article>
  );

  const renderTriggerCard = (item: TriggerListItem) => {
    const { name } = slugToLabel(item.slug);
    const sourceKey = normalizeSource(item.source);
    const isActive = item.slug === selectedSlug;
    return (
      <article
        key={item.slug}
        className={`glass-card pipeline-card triggers-card border border-[var(--border-primary)] rounded-xl p-4 ${isActive ? 'triggers-card--active' : ''}`}
        onClick={() => handleSelectSlug(item.slug)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="triggers-card-icon" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
              </svg>
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{name || item.slug}</h3>
            </div>
          </div>
          <div className="pipeline-card-actions">
            <button
              type="button"
              className="pipelines-delete-button"
              aria-disabled={sourceKey === 'git'}
              title={sourceKey === 'git' ? 'This trigger is managed via Git. Clone it to customize.' : 'Delete trigger'}
              onClick={event => {
                event.stopPropagation();
                if (sourceKey === 'git') return;
                openDeleteModal(item.slug);
              }}
              aria-label="Delete trigger"
            >
              <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                <path d="M4 7h16" />
              </svg>
            </button>
          </div>
        </div>
        <div className="pipeline-card-meta">
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Source</span>
            <span className="pipeline-card-meta-value">{sourceKey}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderList = () => (
    <div id="triggers-list-view" className="pipelines-view">
      <div className="space-y-3 triggers-list-container">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading triggers…</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load triggers: {listError}</div>
        ) : (
          <>
            {searchTerm.trim() ? (
              <div className="triggers-search-summary">
                Showing {visibleTriggers.length} result{visibleTriggers.length === 1 ? '' : 's'} for "{searchTerm.trim()}"
              </div>
            ) : null}

            {visibleTriggers.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">{visibleTriggers.map(item => renderTriggerCard(item))}</div>
            ) : null}

            {searchTerm.trim() ? null : activeFolderNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeFolderNode.children.map(child => renderFolderCard(child))}
              </div>
            ) : null}

            {!visibleTriggers.length && !activeFolderNode.children.length && (
              <div id="triggers-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No triggers found</h3>
                <p className="text-sm text-[var(--text-secondary)]">Create a new trigger or adjust your filters.</p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );

  const handleIndentTab = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    const el = event.currentTarget;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? start;
    const value = el.value;
    const indent = '  ';

    if (start === end) {
      const next = `${value.slice(0, start)}${indent}${value.slice(end)}`;
      setEditorValue(next);
      requestAnimationFrame(() => {
        if (!editorRef.current) return;
        editorRef.current.selectionStart = start + indent.length;
        editorRef.current.selectionEnd = start + indent.length;
      });
      return;
    }

    const before = value.slice(0, start);
    const selection = value.slice(start, end);
    const after = value.slice(end);
    const selectionLines = selection.split('\n');
    const indented = selectionLines.map(line => indent + line).join('\n');
    const next = before + indented + after;
    setEditorValue(next);
    requestAnimationFrame(() => {
      if (!editorRef.current) return;
      editorRef.current.selectionStart = start;
      editorRef.current.selectionEnd = end + indent.length * selectionLines.length;
    });
  };

  const handleAutoIndentEnter = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    const el = event.currentTarget;
    const cursor = el.selectionStart ?? 0;
    const value = el.value;
    const lineStart = value.lastIndexOf('\n', cursor - 1) + 1;
    const line = value.slice(lineStart, cursor);
    const baseIndent = line.match(/^\s*/)?.[0] ?? '';
    const trimmed = line.trimEnd();
    const extraIndent = trimmed.endsWith(':') ? '  ' : '';
    const insert = `\n${baseIndent}${extraIndent}`;
    const next = `${value.slice(0, cursor)}${insert}${value.slice(cursor)}`;
    setEditorValue(next);
    requestAnimationFrame(() => {
      if (!editorRef.current) return;
      const nextCursor = cursor + insert.length;
      editorRef.current.selectionStart = nextCursor;
      editorRef.current.selectionEnd = nextCursor;
    });
  };

  const renderDetail = () => {
    if (!detail) {
      return (
        <div id="triggers-detail-view" className="pipelines-view">
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a trigger to see details.</div>
        </div>
      );
    }

    const sourceKey = normalizeSource(detail.source);
    const isGitSource = sourceKey === 'git';
    const editorLines = editorValue.split('\n');

    return (
      <div id="triggers-detail-view" className="pipelines-view">
        <div className="space-y-6">
          <div className="glass-card p-6">
            <div className="flex items-start justify-between gap-4 w-full mb-4">
              <div className="min-w-0">
                <div className="triggers-detail-heading">
                  <span className="triggers-detail-icon" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
                    </svg>
                  </span>
                  <div className="min-w-0">
                    <h2 id="triggers-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                      {detail.slug}
                    </h2>
                    <div className="triggers-detail-meta">
                      <dl className="triggers-detail-grid">
                        <dt className="triggers-detail-label">Source:</dt>
                        <dd className="triggers-detail-value">{sourceLabel(sourceKey)}</dd>
                        <dt className="triggers-detail-label">Rules:</dt>
                        <dd className="triggers-detail-value">{detail.summary.triggerCount}</dd>
                        <dt className="triggers-detail-label">Events:</dt>
                        <dd className="triggers-detail-value">
                          {detail.summary.events.length ? detail.summary.events.join(', ') : 'N/A'}
                        </dd>
                        <dt className="triggers-detail-label" style={{ alignSelf: 'flex-start', marginTop: 4 }}>
                          Scopes:
                        </dt>
                        <dd
                          className="triggers-detail-value flex flex-wrap gap-1.5"
                          style={{ whiteSpace: 'normal', overflow: 'visible', textOverflow: 'clip' }}
                        >
                          {detail.summary.scopes.length ? (
                            detail.summary.scopes.map(scope => {
                              const label = scope ? `/${scope}` : 'Default Scope';
                              const target = scope ? encodeURIComponent(scope) : 'default';
                              return (
                                <button
                                  key={`scope-${scope || 'default'}`}
                                  type="button"
                                  className="pipelines-tag font-semibold transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-accent)]"
                                  onClick={() => navigate(`/scopes/${target}`)}
                                >
                                  {label}
                                </button>
                              );
                            })
                          ) : (
                            <button
                              type="button"
                              className="pipelines-tag font-semibold transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-accent)]"
                              onClick={() => navigate('/scopes/default')}
                            >
                              Default Scope
                            </button>
                          )}
                        </dd>
                      </dl>
                    </div>
                  </div>
                </div>
              </div>
              <button id="triggers-back-btn" className="glass-button-ghost" onClick={handleBackToList}>
                <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 19l-7-7 7-7" />
                </svg>
                <span>Back to list</span>
              </button>
            </div>

          </div>

          <div className="grid gap-6 lg:grid-cols-[2fr_1fr]">
            <div className="space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Trigger Definition (YAML)</h3>
                  <div className="flex items-center gap-2 flex-wrap">
                    {!isEditing ? (
                      <>
                        <button className="glass-button-ghost" onClick={handleCopyYaml} title="Copy YAML">
                          <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </button>
                        <button className="glass-button-ghost" onClick={handleDownloadYaml} title="Download YAML">
                          <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                          </svg>
                        </button>
                        {isGitSource ? (
                          <button className="glass-button-primary" onClick={openCloneModal}>
                            Clone
                          </button>
                        ) : (
                          <>
                            <button className="glass-button-primary" onClick={() => setIsEditing(true)}>
                              Edit
                            </button>
                            <button className="glass-button-subtle" onClick={openCloneModal}>
                              Clone
                            </button>
                            <button className="glass-button-danger" onClick={() => openDeleteModal(detail.slug)}>
                              Delete
                            </button>
                          </>
                        )}
                      </>
                    ) : (
                      <>
                        <button
                          className="glass-button-ghost"
                          onClick={() => {
                            const resetYaml = editSessionOriginalYamlRef.current || detail.rawYaml;
                            setEditorSuggestion(null);
                            setEditorValue(resetYaml);
                            setIsEditing(false);
                          }}
                        >
                          Discard
                        </button>
                        <button className="glass-button-primary" onClick={handleSave} disabled={saving || validation.errors.length > 0}>
                          {saving ? 'Saving…' : 'Save'}
                        </button>
                      </>
                    )}
                  </div>
                </div>
                <div className="p-4 space-y-3">
                  {!isEditing ? (
                    <div id="triggers-yaml-content" className="yaml-view">
                      {renderYamlLines(detail.rawYaml)}
                    </div>
                  ) : (
                    <div id="editor-container" className="editor-container">
                      <div id="line-numbers" ref={lineNumbersRef}>
                        <div className="line-number-track">
                          {editorLines.map((_, idx) => (
                            <div key={`ln-${idx}`} className={`line-number ${validationErrorLines.has(idx + 1) ? 'line-number--error' : ''}`}>
                              {idx + 1}
                            </div>
                          ))}
                        </div>
                      </div>
                      <div id="triggers-yaml-stage" className="yaml-editor-stage yaml-editor-stage--with-highlight">
                        <div id="triggers-yaml-highlight" className="yaml-editor-highlight" aria-hidden="true">
                          <pre ref={highlightContentRef} className="yaml-editor-highlight__content">
                            {renderYamlHighlight(editorValue)}
                          </pre>
                        </div>
                        <textarea
                          ref={editorRef}
                          id="triggers-yaml-editor"
                          value={editorValue}
                          onChange={event => {
                            const next = event.target.value;
                            setEditorValue(next);
                            if (editorSuggestion) {
                              const cursor = event.target.selectionStart || 0;
                              openEditorSuggestion(cursor, next);
                            }
                          }}
                          onScroll={handleEditorScroll}
                          onKeyDown={event => {
                            if (event.ctrlKey && event.code === 'Space') {
                              event.preventDefault();
                              const cursor = event.currentTarget.selectionStart || 0;
                              if (editorSuggestion) {
                                setEditorSuggestion(null);
                              } else {
                                openEditorSuggestion(cursor);
                              }
                              return;
                            }

                            if (event.key === 'Tab') {
                              event.preventDefault();
                              handleIndentTab(event);
                              return;
                            }

                            if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey && !editorSuggestion) {
                              event.preventDefault();
                              handleAutoIndentEnter(event);
                              return;
                            }

                            if (!editorSuggestion) return;

                            if (event.key === 'Escape') {
                              event.preventDefault();
                              setEditorSuggestion(null);
                              return;
                            }
                            if (event.key === 'ArrowDown') {
                              event.preventDefault();
                              setEditorSuggestion(prev => {
                                if (!prev || prev.items.length === 0) return prev;
                                return { ...prev, activeIndex: (prev.activeIndex + 1) % prev.items.length };
                              });
                              return;
                            }
                            if (event.key === 'ArrowUp') {
                              event.preventDefault();
                              setEditorSuggestion(prev => {
                                if (!prev || prev.items.length === 0) return prev;
                                return { ...prev, activeIndex: (prev.activeIndex - 1 + prev.items.length) % prev.items.length };
                              });
                              return;
                            }
                            if (event.key === 'Enter') {
                              if (editorSuggestion.items.length === 0) return;
                              event.preventDefault();
                              applyEditorSuggestion(editorSuggestion.items[editorSuggestion.activeIndex]);
                            }
                          }}
                          onClick={event => {
                            if (!editorSuggestion) return;
                            const cursor = event.currentTarget.selectionStart || 0;
                            openEditorSuggestion(cursor);
                          }}
                          spellCheck={false}
                        ></textarea>

                        {editorSuggestion && (
                          <div className="pipeline-suggestion-overlay" style={{ top: 12, left: 12, width: 340 }}>
                            <div className="env-suggestion-panel">
                              <div className="env-suggestion-heading">
                                <p className="env-suggestion-kicker">Autocomplete</p>
                                <p className="env-suggestion-title">{editorSuggestion.title}</p>
                                <p className="env-suggestion-subtitle">
                                  Ctrl+Space • Enter to insert • Esc to close
                                  {autocompleteMeta.loading ? ' • Loading…' : ''}
                                </p>
                              </div>
                              <div className="env-suggestion-body">
                                {editorSuggestion.items.length ? (
                                  <div className="env-suggestion-list">
                                    {editorSuggestion.items.map((item, idx) => (
                                      <div
                                        key={`sg-${item}-${idx}`}
                                        className={`env-suggestion-item ${idx === editorSuggestion.activeIndex ? 'env-suggestion-item--active' : ''}`}
                                      >
                                        <button type="button" className="env-suggestion-pill env-suggestion-pill--action" onClick={() => applyEditorSuggestion(item)}>
                                          {item}
                                        </button>
                                      </div>
                                    ))}
                                  </div>
                                ) : (
                                  <p className="env-suggestion-empty">No suggestions available.</p>
                                )}
                              </div>
                            </div>
                          </div>
                        )}
                      </div>

                      <div id="validation-status" className={`validation-box ${validation.errors.length ? '' : 'validation-box--success'}`}>
                        <div className="validation-box__header">{validation.errors.length ? 'Invalid' : 'Valid'}</div>
                        {validation.errors.length > 0 &&
                          validation.errors.slice(0, 3).map((err, idx) => (
                            <div key={`val-${idx}`} className="validation-box__item">
                              {typeof err.line === 'number' && <span className="validation-box__line">Line {err.line}</span>}
                              <div className="validation-box__message">{err.message}</div>
                            </div>
                          ))}
                        {validation.errors.length > 3 && (
                          <div className="validation-box__item">
                            <div className="validation-box__message">+ {validation.errors.length - 3} more…</div>
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>

            <div className="space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]" style={{ marginTop: '9px' }}>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Linked Pipelines</h3>
                </div>
                <div className="p-4">
                  {linkedPipelines.length ? (
                    <ul className={`triggers-pipeline-list ${linkedPipelines.length > 5 ? 'triggers-pipelines-scroll' : ''}`}>
                      {linkedPipelines.map(pipeline => {
                        const meta = pipelineMetaCacheRef.current.get(pipeline.identifier);
                        const sourceKeyLocal = meta?.sourceKey || pipelineSourceIndexRef.current?.get(pipeline.identifier) || 'local';
                        const canNavigate = sourceKeyLocal !== 'local';
                        const buttonProps = canNavigate
                          ? {
                              onClick: () => navigate(`/pipelines/${pipeline.identifier.split('/').map(encodeURIComponent).join('/')}`),
                              title: `Open ${pipeline.identifier}`,
                            }
                          : { disabled: true as const, title: 'Pipeline not available in the pipeline catalog yet.' };

                        return (
                          <li key={`pipe-${pipeline.identifier}`} className={`triggers-pipeline-item ${canNavigate ? '' : 'triggers-pipeline-item--local'}`}>
                            <button type="button" className={`triggers-pipeline-link ${canNavigate ? '' : 'triggers-pipeline-link--local'}`} {...buttonProps}>
                              <span className="triggers-pipeline-name">{pipeline.display}</span>
                              <dl className="triggers-detail-grid triggers-pipeline-details">
                                <dt className="triggers-detail-label">Path:</dt>
                                <dd className="triggers-detail-value">{pipeline.pathLabel === 'root' ? '/' : `/${pipeline.pathLabel}`}</dd>
                                <dt className="triggers-detail-label">Version:</dt>
                                <dd className="triggers-detail-value">{meta?.version || 'latest'}</dd>
                                <dt className="triggers-detail-label">Source:</dt>
                                <dd className="triggers-detail-value">{meta?.sourceLabel || sourceLabel(sourceKeyLocal)}</dd>
                              </dl>
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No pipelines referenced yet.</p>
                  )}
                </div>
              </div>

              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Recent PipelineRuns</h3>
                </div>
                <div className="p-4">
                  {runsLoading ? (
                    <p className="text-sm text-[var(--text-secondary)]">Loading runs…</p>
                  ) : runsError ? (
                    <p className="text-sm text-red-500">Failed to load runs: {runsError}</p>
                  ) : recentRuns.length ? (
                    <ul
                      ref={recentRunsListRef}
                      onScroll={handleRecentRunsScroll}
                      className={`triggers-runs-list ${recentRuns.length >= INITIAL_RECENT_RUNS ? 'triggers-runs-scroll' : ''}`}
                    >
                      {recentRuns.map(run => {
                        const runId = run.run_id || '';
                        const triggerId = run.trigger_event_id || '';
                        const shortRunId = runId ? String(runId).slice(0, 8) : 'unknown';
                        const shortTriggerId = triggerId ? String(triggerId).slice(0, 8) : 'unknown';
                        return (
                          <li key={`run-${runId}`} className="triggers-runs-item">
                            <button
                              type="button"
                              className="pipelines-run-row w-full text-left"
                              onClick={() => navigate(`/pipelineruns/recent/${encodeURIComponent(runId)}`)}
                              title={`Open run ${runId}`}
                            >
                              <div className="triggers-run-row w-full">
                                <div className="triggers-run-row__line triggers-run-row__line--primary">
                                  <span className="triggers-run-row__pipeline">{run.pipeline_name || 'pipeline'}</span>
                                  <span className="triggers-run-row__time">{formatRelativeTime(run.started_at)}</span>
                                </div>
                                <div className="triggers-run-row__line triggers-run-row__line--status">
                                  <span className={`runner-pill ${statusClass(run.status)}`}>{statusLabel(run.status)}</span>
                                  <span className="runner-pill runner-pill--muted">{formatRef(run.git_ref)}</span>
                                </div>
                                <dl className="triggers-detail-grid triggers-run-details">
                                  <dt className="triggers-detail-label">Run ID:</dt>
                                  <dd className="triggers-detail-value">{shortRunId}</dd>
                                  <dt className="triggers-detail-label">Trigger:</dt>
                                  <dd className="triggers-detail-value">{shortTriggerId}</dd>
                                </dl>
                              </div>
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No recent runs for this trigger.</p>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };

  return (
    <div data-page="triggers" className="active h-full flex flex-col">
      {!selectedSlug && (
        <div className="px-6 pt-6 pb-4">
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              className="glass-button-ghost"
              aria-label="Back"
              onClick={() => openFolder(parentFolder(activeFolder))}
              disabled={!activeFolder}
            >
              <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M15 18l-6-6 6-6" />
              </svg>
            </button>
            <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
              <button
                type="button"
                className="pipelines-search-toggle"
                aria-label="Search triggers"
                onClick={() => {
                  setSearchOpen(true);
                  requestAnimationFrame(() => searchInputRef.current?.focus());
                }}
              >
                <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 21l-4.35-4.35M10 18a8 8 0 110-16 8 8 0 010 16z" />
                </svg>
              </button>
              <input
                ref={searchInputRef}
                id="triggers-search"
                type="text"
                placeholder="Search triggers"
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
                  ✕
                </button>
              )}
            </div>
            <button
              id="triggers-new-btn"
              type="button"
              className="pipelines-icon-only"
              aria-label="Create new trigger"
              title="New Trigger"
              onClick={openCreateModal}
            >
              <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 5v14M5 12h14" />
              </svg>
            </button>
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {!selectedSlug ? renderList() : detailLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading trigger…</div>
        ) : detailError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load trigger: {detailError}</div>
        ) : (
          renderDetail()
        )}
      </div>

      {createModal && (
        <div id="triggers-new-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card trigger-modal-card max-w-lg w-full">
            <header className="pipelines-modal-header trigger-modal-header">
              <div className="trigger-modal-heading">
                <span className="trigger-modal-icon" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 5v14M5 12h14" />
                  </svg>
                </span>
                <div className="min-w-0">
                  <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">New trigger</p>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Create trigger override</h3>
                </div>
              </div>
              <button className="glass-button-ghost" onClick={() => setCreateModal(null)} disabled={createModal.pending}>
                Close
              </button>
            </header>
            <div className="pipelines-modal-body trigger-modal-body">
              <div className="trigger-modal-field-group">
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Repository</label>
                <input
                  type="text"
                  className="pipelines-input w-full mt-1"
                  placeholder="owner/repo"
                  value={createModal.repository}
                  onChange={event => {
                    const repoSlug = event.target.value;
                    setCreateModal(prev => {
                      if (!prev) return prev;
                      const pipelinePath = deriveDefaultPipelinePath(repoSlug);
                      return { ...prev, repository: repoSlug, yamlPreview: buildNewTriggerYaml(pipelinePath), error: undefined };
                    });
                  }}
                />
                <p className="trigger-modal-hint">Creates or replaces a trigger override stored in the database.</p>
              </div>
              <div className="trigger-modal-field-group">
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Template</label>
                <div className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
                  <pre className="p-3 text-xs overflow-auto max-h-52">{createModal.yamlPreview}</pre>
                </div>
              </div>
              {createModal.error && <p className="text-sm text-red-500">{createModal.error}</p>}
            </div>
            <div className="pipelines-modal-footer trigger-modal-footer">
              <button className="glass-button-ghost" onClick={() => setCreateModal(null)} disabled={createModal.pending}>
                Cancel
              </button>
              <button className="glass-button-primary" onClick={submitCreateModal} disabled={createModal.pending}>
                {createModal.pending ? 'Creating…' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}

      {cloneModal && (
        <div id="triggers-clone-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card trigger-modal-card max-w-md w-full">
            <header className="pipelines-modal-header trigger-modal-header">
              <div className="trigger-modal-heading">
                <span className="trigger-modal-icon" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M8 7h12M8 12h12M8 17h12" />
                    <path d="M4 7h.01M4 12h.01M4 17h.01" />
                  </svg>
                </span>
                <div className="min-w-0">
                  <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Clone trigger</p>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Clone {detail?.slug}</h3>
                </div>
              </div>
              <button className="glass-button-ghost" onClick={() => setCloneModal(null)} disabled={cloneModal.pending}>
                Close
              </button>
            </header>
            <div className="pipelines-modal-body trigger-modal-body">
              <div className="trigger-modal-field-group">
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Target repository</label>
                <input
                  type="text"
                  className="pipelines-input w-full mt-1"
                  placeholder="owner/repo"
                  value={cloneModal.repository}
                  onChange={event => setCloneModal(prev => (prev ? { ...prev, repository: event.target.value, error: undefined } : prev))}
                />
                <p className="trigger-modal-hint">Copies the YAML from the current trigger into the target override.</p>
              </div>
              {cloneModal.error && <p className="text-sm text-red-500">{cloneModal.error}</p>}
            </div>
            <div className="pipelines-modal-footer trigger-modal-footer">
              <button className="glass-button-ghost" onClick={() => setCloneModal(null)} disabled={cloneModal.pending}>
                Cancel
              </button>
              <button className="glass-button-primary" onClick={submitCloneModal} disabled={cloneModal.pending}>
                {cloneModal.pending ? 'Cloning…' : 'Clone'}
              </button>
            </div>
          </div>
        </div>
      )}

      {deleteModal && (
        <div id="triggers-delete-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card max-w-md w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete trigger</p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Remove {deleteModal.slug}?</h3>
              </div>
              <button className="glass-button-ghost" onClick={() => setDeleteModal(null)} disabled={deleteModal.pending}>
                Close
              </button>
            </header>
            <div className="pipelines-modal-body space-y-3">
              <p className="text-sm text-[var(--text-secondary)]">This action cannot be undone.</p>
              {deleteModal.error && <p className="text-sm text-red-500">{deleteModal.error}</p>}
            </div>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button className="glass-button-ghost" onClick={() => setDeleteModal(null)} disabled={deleteModal.pending}>
                  Cancel
                </button>
                <button className="glass-button-danger" onClick={confirmDelete} disabled={deleteModal.pending}>
                  {deleteModal.pending ? 'Deleting…' : 'Delete'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {toasts.length > 0 && (
        <div className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3">
          {toasts.map(toast => (
            <div key={toast.id} className={`triggers-toast triggers-toast--${toast.tone} show`}>
              <div className="triggers-toast__content">{toast.message}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default TriggersPage;
