import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import { fetchResourceGroupPaths, insertGroupPath } from '../lib/resourceGroups';
import { escapeRegExp, findLineNumberByRegex, findLineNumberForKey, parseYamlWithLocation } from '../lib/yamlValidation';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';
import { EditorAutocompleteMenu } from '../features/editor/EditorAutocompleteMenu';
import { TriggerRecentRuns } from '../features/triggers/TriggerRecentRuns';
import {
  checkTriggerPermission,
  deleteTrigger,
  fetchTriggerAutocompleteResources,
  fetchTriggerDetail,
  fetchTriggerPipelineYaml,
  fetchTriggerRuns,
  fetchTriggers,
  saveTrigger,
} from '../features/triggers/api';
import { useTriggerPermissions } from '../features/triggers/useTriggerPermissions';

const INITIAL_RECENT_RUNS = 5;
const RUNS_PAGE_SIZE = 10;
const RUNS_CACHE_TTL = 60 * 1000;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;
const TRIGGER_ROOT_KEYS = ['triggers'];
const TRIGGER_KEYS = ['on', 'branches', 'skip_branches', 'tags', 'pipelines', 'scope'];
const TRIGGER_EVENT_OPTIONS = ['push', 'pull_request', 'schedule'];

function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value)
    .trim()
    .replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

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

    const scope = normalizeScopeLabel(trigger.scope);
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

function TriggersPage({
  canDeleteTriggers = false,
}: {
  canDeleteTriggers?: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverTriggers, setServerTriggers] = useState<TriggerListItem[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);
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
    const trimmed = rawYaml.trim();
    if (!trimmed) {
      return { errors: [{ message: 'Trigger manifest cannot be empty.', line: 1 }] };
    }

    const { parsed, error } = parseYamlWithLocation(rawYaml);
    if (error) {
      return { errors: [error] };
    }

    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { errors: [{ message: 'YAML must define an object at the root.', line: 1 }] };
    }

    const root = asRecord(parsed);
    if (!root) {
      return { errors: [{ message: 'YAML must define an object at the root.', line: 1 }] };
    }

    const errors: ValidationError[] = [];

    if (root.steps) {
      errors.push({
        message:
          "Validation failed: The provided file appears to be a pipeline, not a trigger manifest. A trigger must contain 'triggers', not 'steps'.",
        line: findLineNumberForKey(rawYaml, 'steps') ?? 1,
      });
    }

    const unknownRootKey = Object.keys(root).find(key => !TRIGGER_ROOT_KEYS.includes(key));
    if (unknownRootKey) {
      errors.push({
        message: `Unknown directive '${unknownRootKey}' at root.`,
        line: findLineNumberForKey(rawYaml, unknownRootKey) ?? 1,
      });
    }

    const triggers = Array.isArray(root.triggers) ? root.triggers : [];
    if (triggers.length === 0) {
      errors.push({ message: "'triggers' must be a non-empty list.", line: findLineNumberForKey(rawYaml, 'triggers') ?? 1 });
      return { errors };
    }

    triggers.forEach((trigger, index: number) => {
      const triggerRecord = asRecord(trigger);
      const triggerLine =
        findLineNumberByRegex(
          rawYaml,
          new RegExp(
            `^\\s*-\\s*on:\\s*${escapeRegExp(typeof triggerRecord?.on === 'string' ? triggerRecord.on.trim() : '')}\\b`,
            'i'
          )
        ) ?? findLineNumberForKey(rawYaml, 'triggers') ?? 1;

      if (!triggerRecord) {
        errors.push({ message: `Trigger #${index + 1} must be an object.`, line: triggerLine });
        return;
      }
      const unknownKey = Object.keys(triggerRecord).find(key => !TRIGGER_KEYS.includes(key) && key !== 'skipBranches');
      if (unknownKey) {
        errors.push({
          message: `Trigger #${index + 1} contains unknown directive '${unknownKey}'.`,
          line: findLineNumberForKey(rawYaml, unknownKey) ?? triggerLine,
        });
      }
      const onValue = typeof triggerRecord.on === 'string' ? triggerRecord.on.trim() : '';
      if (!onValue) {
        errors.push({
          message: `Trigger #${index + 1} is missing required 'on' event.`,
          line: findLineNumberForKey(rawYaml, 'on') ?? triggerLine,
        });
      } else if (!TRIGGER_EVENT_OPTIONS.includes(onValue)) {
        errors.push({
          message: `Trigger #${index + 1} has unsupported event '${onValue}'.`,
          line:
            findLineNumberByRegex(rawYaml, new RegExp(`^\\s*(?:-\\s*)?on:\\s*${escapeRegExp(onValue)}\\b`, 'i')) ??
            triggerLine,
        });
      }

      const pipelines = Array.isArray(triggerRecord.pipelines) ? triggerRecord.pipelines : [];
      if (pipelines.length === 0) {
        errors.push({
          message: `Trigger #${index + 1} must include at least one pipeline reference.`,
          line: findLineNumberForKey(rawYaml, 'pipelines') ?? triggerLine,
        });
      } else {
        pipelines.forEach((entry, pipelineIdx) => {
          const entryRecord =
            entry && typeof entry === 'object' && !Array.isArray(entry) ? (entry as Record<string, unknown>) : null;
          const path =
            typeof entry === 'string'
              ? entry.trim()
              : typeof entryRecord?.path === 'string'
              ? entryRecord.path.trim()
              : '';
          if (!path) {
            errors.push({
              message: `Trigger #${index + 1} pipeline #${pipelineIdx + 1} is missing a path.`,
              line: findLineNumberForKey(rawYaml, 'pipelines') ?? triggerLine,
            });
          }
        });
      }

      const branches = Array.isArray(triggerRecord.branches) ? triggerRecord.branches : [];
      branches.forEach((branch, branchIdx) => {
        const value = typeof branch === 'string' ? branch.trim() : '';
        if (!value) {
          errors.push({
            message: `Trigger #${index + 1} has an empty branch entry at position ${branchIdx + 1}.`,
            line: findLineNumberForKey(rawYaml, 'branches') ?? triggerLine,
          });
        }
      });

      const rawSkip = (Array.isArray(triggerRecord.skip_branches)
        ? triggerRecord.skip_branches
        : Array.isArray((triggerRecord as Record<string, unknown>).skipBranches)
        ? (triggerRecord as Record<string, unknown>).skipBranches
        : []) as unknown[];
      rawSkip.forEach((branch, branchIdx) => {
        const value = typeof branch === 'string' ? branch.trim() : '';
        if (!value) {
          errors.push({
            message: `Trigger #${index + 1} has an empty skip_branches entry at position ${branchIdx + 1}.`,
            line: findLineNumberForKey(rawYaml, 'skip_branches') ?? triggerLine,
          });
        }
      });

      const tags = Array.isArray(triggerRecord.tags) ? triggerRecord.tags : [];
      tags.forEach((tag, tagIdx) => {
        const value = typeof tag === 'string' ? tag.trim() : '';
        if (!value) {
          errors.push({
            message: `Trigger #${index + 1} has an empty tag entry at position ${tagIdx + 1}.`,
            line: findLineNumberForKey(rawYaml, 'tags') ?? triggerLine,
          });
        }
      });

      if (triggerRecord.scope != null) {
        const scopeValue = typeof triggerRecord.scope === 'string' ? triggerRecord.scope.trim() : '';
        if (!scopeValue) {
          errors.push({
            message: `Trigger #${index + 1} has an empty 'scope'.`,
            line: findLineNumberForKey(rawYaml, 'scope') ?? triggerLine,
          });
        }
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
    (cursor: number, opts?: { text?: string; force?: boolean }) => {
      const text = typeof opts?.text === 'string' ? opts.text : editorValue;
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

      if (
        !opts?.force &&
        !lineBeforeCursor.trim() &&
        !prefix &&
        !pipelineValueContext &&
        !scopeValueContext &&
        !onValueContext
      ) {
        setEditorSuggestion(null);
        return;
      }

      const hasContext = pipelineValueContext || scopeValueContext || onValueContext || isKeyContext;
      const shouldShow = opts?.force || hasContext || filtered.length > 0;

      if (!shouldShow) {
        setEditorSuggestion(null);
        return;
      }

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
              const scope = normalizeScopeLabel(record?.scope);
              if (scope) return scope;
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
          const { pipelines: pipelineResp, scopes: scopeResp } = await fetchTriggerAutocompleteResources();

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
        const rawYaml = await fetchTriggerPipelineYaml(normalized);
        if (rawYaml) {
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
      setServerTriggers(await fetchTriggers());
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
      const loaded = await fetchTriggerDetail(slug, source);
      if (selectedSlugRef.current !== target) return;
      setDetail(loaded);
      setLinkedPipelines(loaded.summary.pipelines);
      setEditorValue(loaded.rawYaml);
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

  const loadRecentRuns = useCallback(async (slug: string, pipelines: PipelineRef[]) => {
    const target = slug;
    setRunsLoading(true);
    setRunsError(null);
    try {
      const now = Date.now();
      if (!runsCacheRef.current.runs.length || now - runsCacheRef.current.fetchedAt > RUNS_CACHE_TTL) {
        runsCacheRef.current = { runs: await fetchTriggerRuns(), fetchedAt: Date.now() };
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

  const permissionFolder = selectedSlug ? folderForSlug(selectedSlug) : activeFolder;
  const {
    canCreateTriggerHere,
    canUpdateSelectedTrigger,
  } = useTriggerPermissions(permissionFolder, selectedSlug);

  const openCreateModal = () => {
    if (!canCreateTriggerHere) return;
    const repository = permissionFolder ? `${permissionFolder}/new-repository` : '';
    const yamlPreview = buildNewTriggerYaml(deriveDefaultPipelinePath(repository));
    setCreateModal({ repository, yamlPreview, pending: false });
  };

  const openCloneModal = () => {
    if (!canCreateTriggerHere) return;
    if (!detail) {
      addToast('Select a trigger to clone.', 'info');
      return;
    }
    setCloneModal({ repository: detail.slug, pending: false });
  };

  const openDeleteModal = (slug: string) => {
    if (!canDeleteTriggers) return;
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
    if (!canUpdateSelectedTrigger) {
      addToast('You do not have permission to update triggers.', 'error');
      return;
    }
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
      await saveTrigger(detail.slug, editorValue);
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
    if (!canCreateTriggerHere) return;
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

    const allowed = await checkTriggerPermission('trigger.update', repoSlug);
    if (!allowed) {
      setCreateModal(prev => (prev ? { ...prev, error: 'You do not have permission to create triggers for this repository.' } : prev));
      return;
    }

    setCreateModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const yamlBody = createModal.yamlPreview;
      await saveTrigger(`${owner}/${repo}`, yamlBody);
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
    if (!canCreateTriggerHere) return;
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

    const allowed = await checkTriggerPermission('trigger.update', targetSlug);
    if (!allowed) {
      setCloneModal(prev => (prev ? { ...prev, error: 'You do not have permission to create triggers for this repository.' } : prev));
      return;
    }

    setCloneModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      await saveTrigger(`${owner}/${repo}`, detail.rawYaml);
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
    if (!canDeleteTriggers) return;
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
      await deleteTrigger(`${owner}/${repo}`);
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
    let cancelled = false;
    void fetchResourceGroupPaths()
      .then(paths => {
        if (!cancelled) setResourceGroupPaths(paths);
      })
      .catch(error => {
        console.warn('Failed to load groups for trigger tree', error);
        if (!cancelled) setResourceGroupPaths([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

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
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], triggerSlugs: [] }));
    });
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
  }, [resourceGroupPaths, serverTriggers]);

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
          <span className="pipeline-card-meta-label">Sub groups:</span>
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
            {canDeleteTriggers && (
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
            )}
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
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreateTriggerHere ? 'Create a new trigger or adjust your filters.' : 'Adjust your filters or browse another group.'}
                </p>
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
        <div className="min-w-0 space-y-6">
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

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
            <div className="min-w-0 space-y-6">
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
                        {(canUpdateSelectedTrigger || canCreateTriggerHere) &&
                          (isGitSource ? (
                            canCreateTriggerHere ? (
                              <button className="glass-button-primary" onClick={openCloneModal}>
                                Clone
                              </button>
                            ) : null
                          ) : (
                            <>
                              {canUpdateSelectedTrigger ? (
                                <button className="glass-button-primary" onClick={() => setIsEditing(true)}>
                                  Edit
                                </button>
                              ) : null}
                              {canCreateTriggerHere ? (
                                <button className="glass-button-subtle" onClick={openCloneModal}>
                                  Clone
                                </button>
                              ) : null}
                            </>
                          ))}
                      </>
                    ) : canUpdateSelectedTrigger ? (
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
                    ) : null}
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
                          aria-label="Trigger YAML editor"
                          aria-describedby="trigger-validation-status"
                          aria-invalid={validation.errors.length > 0}
                          aria-autocomplete="list"
                          aria-controls={editorSuggestion ? 'trigger-editor-autocomplete' : undefined}
                          aria-activedescendant={
                            editorSuggestion ? `trigger-editor-autocomplete-option-${editorSuggestion.activeIndex}` : undefined
                          }
                          value={editorValue}
                          onChange={event => {
                            const next = event.target.value;
                            setEditorValue(next);
                            const cursor = event.target.selectionStart || 0;
                            openEditorSuggestion(cursor, { text: next });
                          }}
                          onScroll={handleEditorScroll}
                          onKeyDown={event => {
                            if (event.ctrlKey && event.code === 'Space') {
                              event.preventDefault();
                              const cursor = event.currentTarget.selectionStart || 0;
                              if (editorSuggestion) {
                                setEditorSuggestion(null);
                              } else {
                                openEditorSuggestion(cursor, { force: true });
                              }
                              return;
                            }

                            if (editorSuggestion && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
                              event.preventDefault();
                              setEditorSuggestion(current => {
                                if (!current || !current.items.length) return current;
                                const direction = event.key === 'ArrowDown' ? 1 : -1;
                                return {
                                  ...current,
                                  activeIndex: (current.activeIndex + direction + current.items.length) % current.items.length,
                                };
                              });
                              return;
                            }

                            if (editorSuggestion && event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
                              event.preventDefault();
                              const selectedSuggestion = editorSuggestion.items[editorSuggestion.activeIndex];
                              if (selectedSuggestion) applyEditorSuggestion(selectedSuggestion);
                              return;
                            }

                            if (editorSuggestion && event.key === 'Escape') {
                              event.preventDefault();
                              setEditorSuggestion(null);
                              return;
                            }

                            if (event.key === 'Tab') {
                              event.preventDefault();
                              handleIndentTab(event);
                              return;
                            }

                            if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
                              event.preventDefault();
                              handleAutoIndentEnter(event);
                            }
                          }}
                          onClick={event => {
                            const cursor = event.currentTarget.selectionStart || 0;
                            openEditorSuggestion(cursor);
                          }}
                          spellCheck={false}
                        ></textarea>

                      </div>

                      <div
                        id="trigger-validation-status"
                        className={`validation-box ${validation.errors.length ? '' : 'validation-box--success'}`}
                        role="status"
                        aria-live="polite"
                      >
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
                      {editorSuggestion ? (
                        <EditorAutocompleteMenu
                          id="trigger-editor-autocomplete"
                          suggestion={editorSuggestion}
                          loading={autocompleteMeta.loading}
                          width={340}
                          onSelect={applyEditorSuggestion}
                        />
                      ) : null}
                    </div>
                  )}
                </div>
              </div>
            </div>

            <div className="min-w-0 space-y-6">
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

              <TriggerRecentRuns
                runs={recentRuns}
                loading={runsLoading}
                error={runsError}
                scrollable={recentRuns.length >= INITIAL_RECENT_RUNS}
                listRef={recentRunsListRef}
                onScroll={handleRecentRunsScroll}
                onOpenRun={runId => navigate(`/pipelineruns/recent/${encodeURIComponent(runId)}`)}
              />
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
            {canCreateTriggerHere && (
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
            )}
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

      {canDeleteTriggers && deleteModal && (
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
