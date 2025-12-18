import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';
import {
  STEP_DRAFTS_CHANGED_EVENT,
  STEP_DRAFTS_STORAGE_KEY,
  deleteStepDraft,
  loadStepDrafts,
  type StepDraft,
  upsertStepDraft,
} from '../lib/stepDrafts';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';

const STEP_NAME_PATTERN = /^[a-zA-Z0-9_.-]+$/;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

const STEP_DIRECTIVES = [
  'name',
  'description',
  'include',
  'sync',
  'image',
  'secrets',
  'volumes',
  'variables',
  'tasks',
  'condition',
  'goal',
  'script',
  'depends_on',
  'ignore_failure',
  'llm_output_sharing',
  'artifacts',
];

const TASK_DIRECTIVES = [
  'name',
  'goal',
  'script',
  'depends_on',
  'ignore_failure',
  'llm_output_sharing',
  'variables',
];

const STEP_ALLOWED_KEYS = new Set(STEP_DIRECTIVES);
const TASK_ALLOWED_KEYS = new Set(TASK_DIRECTIVES);

type StepListItem = {
  id: string;
  source?: string;
  updatedAt?: string;
};

type StepDetail = {
  id: string;
  name: string;
  path: string;
  description: string;
  rawYaml: string;
  source?: string;
  updatedAt?: string;
};

type StepUsageItem = {
  identifier: string;
  name: string;
  path: string;
  source: string;
  description?: string;
};

type FormModalState = {
  mode: 'create' | 'clone';
  path: string;
  name: string;
  pending: boolean;
  error?: string;
  baseYaml?: string;
};

type DeleteModalState = {
  stepId: string;
  stepName: string;
  pending: boolean;
  error?: string;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

type TreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TreeNode[];
  stepIds: string[];
};

type ValidationError = {
  message: string;
  line?: number;
  column?: number;
};

type ValidationResult = {
  errors: ValidationError[];
};

function encodeId(id: string): string {
  return id.split('/').map(encodeURIComponent).join('/');
}

function splitIdentifier(id: string): { name: string; path: string } {
  const parts = id.split('/').filter(Boolean);
  const name = decodeURIComponent(parts.pop() || '');
  const path = parts.map(decodeURIComponent).join('/');
  return { name, path };
}

function normalizeSource(raw: unknown): 'git' | 'database' | 'draft' {
  const value = typeof raw === 'string' ? raw.trim().toLowerCase() : '';
  if (!value) return 'database';
  if (value.includes('git')) return 'git';
  if (value.includes('draft')) return 'draft';
  if (value.includes('db') || value.includes('database')) return 'database';
  return 'database';
}

function formatUpdatedAt(value?: string): string {
  const raw = (value || '').trim();
  if (!raw) return '—';
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function normalizeLineNumber(value: unknown): number | undefined {
  if (typeof value !== 'number' || Number.isNaN(value)) return undefined;
  const normalized = Math.max(1, Math.floor(value));
  return Number.isFinite(normalized) ? normalized : undefined;
}

function findLineNumberByRegex(yamlString: string, regex: RegExp): number | undefined {
  const lines = yamlString.split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    if (regex.test(lines[i])) return i + 1;
  }
  return undefined;
}

function findLineNumberForKey(yamlString: string, key: string): number | undefined {
  if (!key) return undefined;
  const pattern = new RegExp(`^\\s*(?:-\\s*)?${escapeRegExp(key)}\\s*:`, 'i');
  return findLineNumberByRegex(yamlString, pattern);
}

function findLineNumberForTaskName(yamlString: string, taskName: string): number | undefined {
  if (!taskName) return undefined;
  const pattern = new RegExp(`^\\s*(?:-\\s*)?name:\\s*${escapeRegExp(taskName)}\\b`, 'i');
  return findLineNumberByRegex(yamlString, pattern);
}

function validateStepYaml(rawYaml: string, opts?: { expectedName?: string }): ValidationResult {
  const trimmed = rawYaml.trim();
  if (!trimmed) {
    return { errors: [{ message: 'Step definition cannot be empty.', line: 1 }] };
  }

  let parsed: unknown = null;
  try {
    parsed = yaml.load(rawYaml) as unknown;
  } catch (error: unknown) {
    const record = error && typeof error === 'object' ? (error as Record<string, unknown>) : null;
    const mark = record?.mark && typeof record.mark === 'object' ? (record.mark as Record<string, unknown>) : null;
    const line = typeof mark?.line === 'number' ? mark.line + 1 : undefined;
    const column = typeof mark?.column === 'number' ? mark.column + 1 : undefined;
    return {
      errors: [
        {
          message: error instanceof Error ? error.message : typeof record?.message === 'string' ? record.message : 'Unable to parse YAML.',
          line: normalizeLineNumber(line),
          column: normalizeLineNumber(column),
        },
      ],
    };
  }

  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { errors: [{ message: 'Step YAML must define an object.', line: 1 }] };
  }

  const record = parsed as Record<string, unknown>;
  const unknownKey = Object.keys(record).find(key => !STEP_ALLOWED_KEYS.has(key));
  if (unknownKey) {
    return {
      errors: [
        {
          message: `Unknown step directive '${unknownKey}'.`,
          line: findLineNumberForKey(rawYaml, unknownKey) ?? 1,
        },
      ],
    };
  }

  const name = typeof record.name === 'string' ? record.name.trim() : '';
  if (!name) {
    return {
      errors: [
        {
          message: "Step YAML must include a 'name' field.",
          line: findLineNumberForKey(rawYaml, 'name') ?? 1,
        },
      ],
    };
  }

  if (!STEP_NAME_PATTERN.test(name)) {
    return {
      errors: [
        {
          message: 'Step name can only contain letters, numbers, dots, underscores, and hyphens.',
          line: findLineNumberForKey(rawYaml, 'name') ?? 1,
        },
      ],
    };
  }

  const expectedName = (opts?.expectedName || '').trim();
  if (expectedName && expectedName !== name) {
    return {
      errors: [
        {
          message: `Step name in YAML ('${name}') must match the identifier name ('${expectedName}').`,
          line: findLineNumberForKey(rawYaml, 'name') ?? 1,
        },
      ],
    };
  }

  const hasInclude = record.include != null;
  const hasTasks = record.tasks != null;
  const hasGoal = record.goal != null;
  const hasScript = record.script != null;

  const modeCount = [hasInclude, hasTasks, hasGoal, hasScript].filter(Boolean).length;
  const lineForMode =
    findLineNumberForKey(rawYaml, 'include') ??
    findLineNumberForKey(rawYaml, 'tasks') ??
    findLineNumberForKey(rawYaml, 'goal') ??
    findLineNumberForKey(rawYaml, 'script') ??
    1;

  if (modeCount === 0) {
    return {
      errors: [{ message: "Step must define one of 'include', 'tasks', 'goal', or 'script'.", line: lineForMode }],
    };
  }
  if (modeCount > 1) {
    return {
      errors: [{ message: "Step may only define one of 'include', 'tasks', 'goal', or 'script'.", line: lineForMode }],
    };
  }

  if (hasInclude) {
    const includeValue = typeof record.include === 'string' ? record.include.trim() : '';
    if (!includeValue) {
      return {
        errors: [
          {
            message: "Include steps must provide a non-empty 'include' value.",
            line: findLineNumberForKey(rawYaml, 'include') ?? 1,
          },
        ],
      };
    }
    if (!includeValue.startsWith('step:')) {
      return {
        errors: [
          {
            message: "Include steps must reference a reusable step using the 'step:' prefix.",
            line: findLineNumberForKey(rawYaml, 'include') ?? 1,
          },
        ],
      };
    }
    return { errors: [] };
  }

  if (hasTasks) {
    const tasks = Array.isArray(record.tasks) ? record.tasks : null;
    const tasksLine = findLineNumberForKey(rawYaml, 'tasks') ?? 1;
    if (!tasks || tasks.length === 0) {
      return { errors: [{ message: "Step 'tasks' must be a non-empty list.", line: tasksLine }] };
    }

    const taskNames = new Map<string, string>();

    for (let idx = 0; idx < tasks.length; idx += 1) {
      const taskValue = tasks[idx];
      if (!taskValue || typeof taskValue !== 'object' || Array.isArray(taskValue)) {
        return { errors: [{ message: `Task #${idx + 1} must be an object.`, line: tasksLine }] };
      }
      const taskObj = taskValue as Record<string, unknown>;
      const taskName = typeof taskObj.name === 'string' ? taskObj.name.trim() : '';
      if (!taskName) {
        return { errors: [{ message: `Task #${idx + 1} is missing the required 'name' field.`, line: tasksLine }] };
      }
      const nameKey = taskName.toLowerCase();
      if (taskNames.has(nameKey)) {
        return {
          errors: [
            {
              message: `Duplicate task name '${taskName}' found. Task names must be unique within a step.`,
              line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
            },
          ],
        };
      }
      taskNames.set(nameKey, taskName);

      const invalidTaskKey = Object.keys(taskObj).find(key => !TASK_ALLOWED_KEYS.has(key));
      if (invalidTaskKey) {
        return {
          errors: [
            {
              message: `Task '${taskName}' contains unknown directive '${invalidTaskKey}'.`,
              line: findLineNumberForKey(rawYaml, invalidTaskKey) ?? findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
            },
          ],
        };
      }

      const taskGoal = typeof taskObj.goal === 'string' ? taskObj.goal.trim() : '';
      const taskScript = typeof taskObj.script === 'string' ? taskObj.script.trim() : '';

      if (taskGoal && taskScript) {
        return { errors: [{ message: `Task '${taskName}' cannot define both 'goal' and 'script'.`, line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine }] };
      }
      if (!taskGoal && !taskScript) {
        return { errors: [{ message: `Task '${taskName}' must define either 'goal' or 'script'.`, line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine }] };
      }
    }

    for (const taskValue of tasks) {
      const taskObj = taskValue as Record<string, unknown>;
      const taskName = typeof taskObj.name === 'string' ? taskObj.name.trim() : '';
      const deps = Array.isArray(taskObj.depends_on) ? taskObj.depends_on : [];
      for (const dep of deps) {
        const depKey = typeof dep === 'string' ? dep.trim().toLowerCase() : '';
        if (!depKey) {
          return {
            errors: [
              {
                message: 'Task dependency names must be non-empty strings.',
                line: findLineNumberForKey(rawYaml, 'depends_on') ?? findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
              },
            ],
          };
        }
        if (!taskNames.has(depKey)) {
          return {
            errors: [
              {
                message: `Task '${taskName || 'unknown'}' depends on undefined task '${String(dep)}'.`,
                line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
              },
            ],
          };
        }
      }
    }

    return { errors: [] };
  }

  if (hasGoal) {
    const goalValue = typeof record.goal === 'string' ? record.goal.trim() : '';
    if (!goalValue) {
      return { errors: [{ message: "Step 'goal' must be a non-empty string.", line: findLineNumberForKey(rawYaml, 'goal') ?? 1 }] };
    }
    return { errors: [] };
  }

  if (hasScript) {
    const scriptValue = typeof record.script === 'string' ? record.script.trim() : '';
    if (!scriptValue) {
      return { errors: [{ message: "Step 'script' must be a non-empty string.", line: findLineNumberForKey(rawYaml, 'script') ?? 1 }] };
    }
    return { errors: [] };
  }

  return { errors: [] };
}

function StepsPage() {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverSteps, setServerSteps] = useState<StepListItem[]>([]);
  const [draftSteps, setDraftSteps] = useState<StepDraft[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selectedIdRef = useRef<string | null>(null);

  const [detail, setDetail] = useState<StepDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  const [usage, setUsage] = useState<StepUsageItem[]>([]);
  const [usageLoading, setUsageLoading] = useState(false);
  const [usageError, setUsageError] = useState<string | null>(null);

  const [isEditing, setIsEditing] = useState(false);
  const [editorValue, setEditorValue] = useState('');
  const [saving, setSaving] = useState(false);

  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const highlightContentRef = useRef<HTMLPreElement | null>(null);
  const lineNumbersRef = useRef<HTMLDivElement | null>(null);
  const editSessionOriginalYamlRef = useRef<string>('');
  const wasEditingRef = useRef(false);

  const autocompleteFetchRef = useRef<{ fetchedAt: number; loadingPromise: Promise<void> | null }>({
    fetchedAt: 0,
    loadingPromise: null,
  });

  const [autocompleteMeta, setAutocompleteMeta] = useState<{
    secrets: string[];
    variables: string[];
    reusableSteps: string[];
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], reusableSteps: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<null | {
    title: string;
    items: string[];
    activeIndex: number;
    replaceStart: number;
    replaceEnd: number;
    appendColon: boolean;
  }>(null);

  const [formModal, setFormModal] = useState<FormModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<DeleteModalState | null>(null);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = useCallback((message: string, tone: ToastMessage['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const draftsById = useMemo(() => {
    const map = new Map<string, StepDraft>();
    draftSteps.forEach(draft => map.set(draft.id, draft));
    return map;
  }, [draftSteps]);

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
    (cursor: number) => {
      const text = editorValue;
      const before = text.slice(0, cursor);
      const lineStart = before.lastIndexOf('\n') + 1;
      const lineBeforeCursor = text.slice(lineStart, cursor);
      const prefixMatch = lineBeforeCursor.match(/[A-Za-z0-9_.:/-]+$/);
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
          const match = rawLine.match(/^\s*([A-Za-z0-9_.-]+)\s*:\s*/);
          if (match) {
            parentKey = match[1];
            break;
          }
        }
      }

      const includeValueContext =
        currentKey === 'include' || /^\s*include\s*:\s*[A-Za-z0-9_.:/-]*$/.test(lineBeforeCursor.trim());
      const cursorInKeyPosition = !lineBeforeCursor.includes(':');

      const resolveTaskNames = () => {
        try {
          const parsed = yaml.load(text) as unknown;
          if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return [];
          const record = parsed as Record<string, unknown>;
          const tasksValue = record.tasks;
          const tasks = Array.isArray(tasksValue) ? tasksValue : [];
          return tasks
            .map(task => {
              if (!task || typeof task !== 'object' || Array.isArray(task)) return '';
              const taskRecord = task as Record<string, unknown>;
              const taskName = taskRecord.name;
              return typeof taskName === 'string' ? taskName.trim() : '';
            })
            .filter(Boolean);
        } catch {
          return [];
        }
      };

      let title = 'Suggestions';
      let pool: string[] = [];
      let appendColon = false;

      if (includeValueContext) {
        title = 'Reusable steps';
        pool = autocompleteMeta.reusableSteps.map(id => `step:${id}`);
      } else if (parentKey === 'secrets') {
        title = 'Secrets';
        pool = autocompleteMeta.secrets;
      } else if (parentKey === 'depends_on') {
        title = 'Task dependencies';
        pool = resolveTaskNames();
      } else if (parentKey === 'variables' && cursorInKeyPosition) {
        title = 'Variables';
        pool = autocompleteMeta.variables;
        appendColon = true;
      } else {
        appendColon = true;
        if (parentKey === 'tasks') {
          title = 'Task keys';
          pool = TASK_DIRECTIVES;
        } else {
          title = 'Step keys';
          pool = STEP_DIRECTIVES;
        }
      }

      const normalizedPrefix = prefix.toLowerCase();
      const filtered = pool.filter(item => item.toLowerCase().startsWith(normalizedPrefix)).sort((a, b) => a.localeCompare(b));
      setEditorSuggestion({
        title,
        items: filtered.slice(0, 50),
        activeIndex: 0,
        replaceStart,
        replaceEnd,
        appendColon,
      });
    },
    [autocompleteMeta.reusableSteps, autocompleteMeta.secrets, autocompleteMeta.variables, editorValue]
  );

  const loadAutocomplete = useCallback(
    async (force?: boolean) => {
      const now = Date.now();
      if (!force && autocompleteFetchRef.current.fetchedAt && now - autocompleteFetchRef.current.fetchedAt < AUTOCOMPLETE_REFRESH_INTERVAL) {
        return;
      }
      if (autocompleteFetchRef.current.loadingPromise) {
        await autocompleteFetchRef.current.loadingPromise;
        return;
      }

      setAutocompleteMeta(prev => ({ ...prev, loading: true }));

      try {
        const normalizeList = (payload: unknown) => {
          if (!Array.isArray(payload)) return [];
          return payload
            .map((item: unknown) => {
              if (typeof item === 'string') return item.trim();
              if (item && typeof item === 'object') {
                const record = item as Record<string, unknown>;
                const name = typeof record.name === 'string' ? record.name : '';
                return name.trim();
              }
              return '';
            })
            .filter(Boolean);
        };

        const promise = (async () => {
          const [secretsResp, variablesResp, stepsResp] = await Promise.all([
            fetch(buildApiUrl('/v1/secrets')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/variables')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/steps')).then(r => (r.ok ? r.json() : [])),
          ]);
          setAutocompleteMeta({
            secrets: normalizeList(secretsResp),
            variables: normalizeList(variablesResp),
            reusableSteps: normalizeList(stepsResp),
            fetchedAt: Date.now(),
            loading: false,
          });
          autocompleteFetchRef.current.fetchedAt = Date.now();
        })();

        autocompleteFetchRef.current.loadingPromise = promise;
        await promise;
      } catch (error) {
        console.warn('Failed to load editor autocomplete metadata', error);
        setAutocompleteMeta(prev => ({ ...prev, loading: false }));
      } finally {
        autocompleteFetchRef.current.loadingPromise = null;
      }
    },
    []
  );

  const parseStepYaml = useCallback((raw: string, id: string, source?: string, updatedAt?: string): StepDetail => {
    const safe = (value: unknown) => (typeof value === 'string' ? value : '');
    let parsed: Record<string, unknown> | null = null;
    try {
      const loaded = yaml.load(raw) as unknown;
      if (loaded && typeof loaded === 'object' && !Array.isArray(loaded)) {
        parsed = loaded as Record<string, unknown>;
      }
    } catch (error) {
      console.warn('Failed to parse step YAML for metadata', error);
    }
    const { name: fallbackName, path } = splitIdentifier(id);
    return {
      id,
      name: safe(parsed?.name).trim() || fallbackName,
      description: safe(parsed?.description) || 'No description provided.',
      path,
      rawYaml: raw,
      source,
      updatedAt,
    };
  }, []);

  const validation = useMemo(() => {
    if (!isEditing) return { errors: [] };
    const expectedName = detail ? splitIdentifier(detail.id).name : '';
    return validateStepYaml(editorValue, { expectedName });
  }, [detail, editorValue, isEditing]);

  const validationErrorLines = useMemo(() => {
    const lines = new Set<number>();
    validation.errors.forEach(err => {
      if (typeof err.line === 'number') lines.add(err.line);
    });
    return lines;
  }, [validation.errors]);

  const loadSteps = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) {
      setListLoading(true);
    }
    setListError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/steps?include_source=true'));
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load steps (${response.status})`);
      }
      const payload = await response.json();
      const normalized: StepListItem[] = Array.isArray(payload)
        ? payload
            .map((item: unknown): StepListItem | null => {
              if (typeof item === 'string') return { id: item.trim() };
              if (item && typeof item === 'object') {
                const record = item as Record<string, unknown>;
                const id =
                  typeof record.identifier === 'string' ? record.identifier : typeof record.id === 'string' ? record.id : '';
                const source = typeof record.source === 'string' ? record.source : undefined;
                const updatedAt = typeof record.updated_at === 'string' ? record.updated_at : undefined;
                return id ? { id, source, updatedAt } : null;
              }
              return null;
            })
            .filter((item: StepListItem | null): item is StepListItem => Boolean(item))
        : [];
      normalized.sort((a, b) => a.id.localeCompare(b.id));
      setServerSteps(normalized);
    } catch (error) {
      console.error('Failed to load steps', error);
      setListError(error instanceof Error ? error.message : 'Unable to load steps');
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadStepDetail = useCallback(
    async (stepId: string, source?: string, updatedAt?: string) => {
      const normalizedSource = normalizeSource(source);
      setDetailLoading(true);
      setDetailError(null);
      try {
        if (normalizedSource === 'draft') {
          const draft = draftsById.get(stepId);
          if (!draft) throw new Error('Draft step not found');
          const parsed = parseStepYaml(draft.yaml, stepId, 'draft', draft.updatedAt);
          setDetail(parsed);
          setEditorValue(draft.yaml);
          setIsEditing(true);
          return;
        }

        const response = await fetch(buildApiUrl(`/v1/steps/${encodeId(stepId)}`));
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to fetch step (${response.status})`);
        }
        const rawYaml = await response.text();
        const parsed = parseStepYaml(rawYaml, stepId, normalizedSource, updatedAt);
        setDetail(parsed);
        setEditorValue(rawYaml);
        setIsEditing(false);
      } catch (error) {
        console.error('Failed to fetch step', error);
        setDetailError(error instanceof Error ? error.message : 'Unable to load step details');
      } finally {
        setDetailLoading(false);
      }
    },
    [draftsById, parseStepYaml]
  );

  const loadUsage = useCallback(async (stepId: string) => {
    const targetId = stepId;
    setUsageLoading(true);
    setUsageError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/steps/${encodeId(stepId)}/usage`));
      if (!response.ok) {
        if (response.status === 404) {
          setUsage([]);
          return;
        }
        const text = await response.text();
        throw new Error(text || `Failed to load usage (${response.status})`);
      }
      const payload = await response.json();
      const list: StepUsageItem[] = Array.isArray(payload)
        ? payload
            .map((item: unknown): StepUsageItem | null => {
              if (!item || typeof item !== 'object') return null;
              const record = item as Record<string, unknown>;
              const identifier = typeof record.identifier === 'string' ? record.identifier : '';
              if (!identifier) return null;
              const name = typeof record.name === 'string' ? record.name : '';
              const path = typeof record.path === 'string' ? record.path : '';
              const source = typeof record.source === 'string' ? record.source : 'database';
              const description = typeof record.description === 'string' ? record.description : undefined;
              return { identifier, name, path, source, description };
            })
            .filter((item: StepUsageItem | null): item is StepUsageItem => Boolean(item))
        : [];
      list.sort((a, b) => a.identifier.localeCompare(b.identifier));
      if (selectedIdRef.current === targetId) {
        setUsage(list);
      }
    } catch (error) {
      console.error('Failed to load step usage', error);
      if (selectedIdRef.current === targetId) {
        setUsageError(error instanceof Error ? error.message : 'Unable to load usage');
        setUsage([]);
      }
    } finally {
      if (selectedIdRef.current === targetId) {
        setUsageLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    setDraftSteps(loadStepDrafts());
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const refreshDrafts = () => setDraftSteps(loadStepDrafts());
    const onStorage = (event: StorageEvent) => {
      if (event.key !== STEP_DRAFTS_STORAGE_KEY) return;
      refreshDrafts();
    };
    window.addEventListener(STEP_DRAFTS_CHANGED_EVENT, refreshDrafts);
    window.addEventListener('storage', onStorage);
    return () => {
      window.removeEventListener(STEP_DRAFTS_CHANGED_EVENT, refreshDrafts);
      window.removeEventListener('storage', onStorage);
    };
  }, []);

  const steps = useMemo(() => {
    const merged = new Map<string, StepListItem>();
    serverSteps.forEach(item => merged.set(item.id, item));
    draftSteps.forEach(draft => merged.set(draft.id, { id: draft.id, source: 'draft', updatedAt: draft.updatedAt }));
    return Array.from(merged.values()).sort((a, b) => a.id.localeCompare(b.id));
  }, [serverSteps, draftSteps]);

  useEffect(() => {
    void loadSteps();
  }, [loadSteps]);

  useEffect(() => {
    if (listLoading || listError) return;
    const activeId = selectedIdRef.current;
    if (!activeId) return;
    if (draftsById.has(activeId)) return;
    if (serverSteps.some(item => item.id === activeId)) return;
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/steps', { replace: true });
  }, [draftsById, listError, listLoading, navigate, serverSteps]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'steps') return;
    if (segments.length > 1) {
      const identifier = segments.slice(1).map(decodeURIComponent).join('/');
      if (identifier !== selectedIdRef.current) {
        setSelectedId(identifier);
        selectedIdRef.current = identifier;
      }
    } else if (selectedIdRef.current) {
      setSelectedId(null);
      selectedIdRef.current = null;
    }

    const params = new URLSearchParams(location.search);
    setActiveFolder(params.get('folder') || '');
  }, [location.pathname, location.search]);

  useEffect(() => {
    if (!selectedId) {
      setDetail(null);
      setEditorValue('');
      setIsEditing(false);
      setEditorSuggestion(null);
      setUsage([]);
      setUsageError(null);
      setUsageLoading(false);
      return;
    }
    const selectedItem = steps.find(item => item.id === selectedId);
    void loadStepDetail(selectedId, selectedItem?.source, selectedItem?.updatedAt);
    void loadUsage(selectedId);
  }, [loadStepDetail, loadUsage, selectedId, steps]);

  useEffect(() => {
    if (!isEditing) {
      setEditorSuggestion(null);
      return;
    }
    void loadAutocomplete();
  }, [isEditing, loadAutocomplete]);

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
    if (!detail || !isEditing) return;
    if (normalizeSource(detail.source) !== 'draft') return;
    const draftId = detail.id;
    const handle = window.setTimeout(() => {
      setDraftSteps(upsertStepDraft({ id: draftId, yaml: editorValue }));
    }, 800);
    return () => window.clearTimeout(handle);
  }, [detail, editorValue, isEditing]);

  useEffect(() => {
    if (!isEditing) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
      return '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [isEditing]);

  useEffect(() => {
    if (!isEditing) return;
    syncEditorOverlays(editorRef.current);
  }, [editorValue, isEditing, syncEditorOverlays]);

  const filteredSteps = useMemo(() => {
    const query = searchTerm.trim().toLowerCase();
    if (!query) return steps;
    return steps.filter(item => item.id.toLowerCase().includes(query));
  }, [searchTerm, steps]);

  const visibleSteps = useMemo(() => {
    const list = searchTerm.trim()
      ? filteredSteps
      : filteredSteps.filter(item => splitIdentifier(item.id).path === activeFolder);
    return [...list].sort((a, b) => a.id.localeCompare(b.id));
  }, [activeFolder, filteredSteps, searchTerm]);

  const buildTree = useMemo(() => {
    const root: TreeNode = { id: '__root__', name: '', fullPath: '', children: [], stepIds: [] };
    steps.forEach(item => {
      const parts = item.id.split('/').filter(Boolean);
      const leafName = parts.pop();
      if (!leafName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], stepIds: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.stepIds.push(item.id);
      current.stepIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [steps]);

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

  const buildIdentifier = (path: string, name: string) => {
    const cleanedName = name.trim();
    const cleanedPath = path.trim().replace(/^\/+|\/+$/g, '');
    if (!cleanedName) return '';
    return cleanedPath ? `${cleanedPath}/${cleanedName}` : cleanedName;
  };

  const parentFolder = (path: string) => {
    const parts = path.split('/').filter(Boolean);
    parts.pop();
    return parts.join('/');
  };

  const openFolder = (path: string) => {
    const cleaned = path.trim().replace(/^\/+|\/+$/g, '');
    setActiveFolder(cleaned);
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate(cleaned ? `/steps?folder=${encodeURIComponent(cleaned)}` : '/steps');
  };

  const handleSelect = (id: string) => {
    selectedIdRef.current = id;
    setSelectedId(id);
    navigate(`/steps/${id.split('/').map(encodeURIComponent).join('/')}`);
  };

  const handleBackToList = () => {
    if (detail) {
      const folder = splitIdentifier(detail.id).path;
      navigate(folder ? `/steps?folder=${encodeURIComponent(folder)}` : '/steps');
      return;
    }
    navigate('/steps');
  };

  const handleCopy = async () => {
    if (!detail?.rawYaml) return;
    try {
      await navigator.clipboard.writeText(detail.rawYaml);
      addToast('Step YAML copied to clipboard.', 'success');
    } catch (error) {
      console.error('Copy failed', error);
      addToast('Unable to copy YAML.', 'error');
    }
  };

  const handleDownload = () => {
    if (!detail?.rawYaml) return;
    const blob = new Blob([detail.rawYaml], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    const { name } = splitIdentifier(detail.id);
    link.href = url;
    link.download = `${name || 'step'}.yaml`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const buildTemplateYaml = (name: string) => {
    return [
      `name: ${name}`,
      'description: Describe what this step does.',
      'script: |',
      `  echo "Implement ${name}"`,
      '',
    ].join('\n');
  };

  const updateYamlName = (raw: string, nextName: string) => {
    try {
      const parsed = yaml.load(raw) as Record<string, unknown> | undefined;
      const updated = { ...(parsed || {}), name: nextName };
      return yaml.dump(updated, { lineWidth: 120 });
    } catch {
      const replaced = raw.replace(/^name:\s*.+$/m, `name: ${nextName}`);
      if (replaced !== raw) return replaced;
      return `name: ${nextName}\n${raw}`;
    }
  };

  const handleSave = async () => {
    if (!detail || !editorValue.trim()) return;
    if (normalizeSource(detail.source) === 'git') {
      addToast('Git-managed steps are read-only. Clone it to create an editable draft.', 'info');
      return;
    }
    if (validation.errors.length > 0) {
      addToast('Fix validation errors before saving.', 'error');
      return;
    }
    setSaving(true);
    try {
      const response = await fetch(buildApiUrl(`/v1/steps/${encodeId(detail.id)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: editorValue,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to save step (${response.status})`);
      }
      addToast('Step saved.', 'success');
      const wasDraft = normalizeSource(detail.source) === 'draft';
      if (wasDraft) {
        setDraftSteps(deleteStepDraft(detail.id));
      }
      const resolvedSource = wasDraft ? 'database' : steps.find(item => item.id === detail.id)?.source;
      const savedAt = new Date().toISOString();
      const updated = parseStepYaml(editorValue, detail.id, resolvedSource, savedAt);
      setDetail(updated);
      setEditorValue(editorValue);
      setIsEditing(false);
      await loadSteps({ quiet: true });
    } catch (error) {
      console.error('Save failed', error);
      addToast(error instanceof Error ? error.message : 'Unable to save step', 'error');
    } finally {
      setSaving(false);
    }
  };

  const openCreateModal = () => setFormModal({ mode: 'create', path: activeFolder, name: '', pending: false });
  const openCloneModal = () => {
    if (!detail) {
      addToast('Select a step to clone.', 'info');
      return;
    }
    const { path, name } = splitIdentifier(detail.id);
    setFormModal({
      mode: 'clone',
      path,
      name: `${name}-copy`,
      pending: false,
      baseYaml: detail.rawYaml,
    });
  };

  const submitFormModal = async () => {
    if (!formModal) return;
    const identifier = buildIdentifier(formModal.path, formModal.name);
    if (!identifier) {
      setFormModal(prev => (prev ? { ...prev, error: 'Step name is required.' } : prev));
      return;
    }
    if (!STEP_NAME_PATTERN.test(formModal.name.trim())) {
      setFormModal(prev =>
        prev
          ? { ...prev, error: 'Step name can only contain letters, numbers, dots, underscores, and hyphens.' }
          : prev
      );
      return;
    }
    if (steps.some(item => item.id === identifier)) {
      setFormModal(prev => (prev ? { ...prev, error: 'A step with that identifier already exists.' } : prev));
      return;
    }
    setFormModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const yamlBody =
        formModal.mode === 'clone' && formModal.baseYaml
          ? updateYamlName(formModal.baseYaml, formModal.name.trim())
          : buildTemplateYaml(formModal.name.trim());
      setDraftSteps(upsertStepDraft({ id: identifier, yaml: yamlBody }));
      addToast(`Draft step ${formModal.mode === 'create' ? 'created' : 'cloned'}.`, 'success');
      setFormModal(null);
      handleSelect(identifier);
    } catch (error) {
      console.error('Draft save failed', error);
      setFormModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to create draft' } : prev));
    } finally {
      setFormModal(prev => (prev ? { ...prev, pending: false } : prev));
    }
  };

  const confirmDelete = async () => {
    if (!deleteModal) return;
    setDeleteModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const source = steps.find(item => item.id === deleteModal.stepId)?.source;
      if (normalizeSource(source) === 'git') {
        throw new Error('This step is managed via Git. Clone it to customize instead of deleting.');
      }
      if (normalizeSource(source) === 'draft') {
        setDraftSteps(deleteStepDraft(deleteModal.stepId));
      } else {
        const response = await fetch(buildApiUrl(`/v1/steps/${encodeId(deleteModal.stepId)}`), { method: 'DELETE' });
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to delete step (${response.status})`);
        }
      }
      addToast('Step deleted.', 'success');
      setDeleteModal(null);
      setSelectedId(null);
      selectedIdRef.current = null;
      navigate('/steps');
      await loadSteps();
    } catch (error) {
      console.error('Delete failed', error);
      setDeleteModal(prev => (prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to delete step' } : prev));
    } finally {
      setDeleteModal(prev => (prev ? { ...prev, pending: false } : prev));
    }
  };

  const renderStepCard = (step: StepListItem) => {
    const { name, path } = splitIdentifier(step.id);
    const source = normalizeSource(step.source);
    return (
      <article
        key={step.id}
        className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
        onClick={() => handleSelect(step.id)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="pipeline-card-icon" aria-hidden="true">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12 2l8 4.5v11L12 22 4 17.5v-11L12 2z" />
                <path d="M12 22v-7.5" />
                <path d="M20 6.5l-8 4.5-8-4.5" />
              </svg>
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{name || step.id}</h3>
              <p className="pipeline-card-path">{path || 'root'}</p>
              <p className="pipeline-card-description">Reusable step definition.</p>
            </div>
          </div>
          <div className="pipeline-card-actions">
            <button
              type="button"
              className="pipelines-delete-button"
              aria-disabled={source === 'git'}
              title={source === 'git' ? 'This step is managed via Git. Clone it to customize.' : 'Delete step'}
              onClick={event => {
                event.stopPropagation();
                if (source === 'git') return;
                setDeleteModal({ stepId: step.id, stepName: name || step.id, pending: false });
              }}
              aria-label="Delete step"
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
            <span className="pipeline-card-meta-value">{source}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderFolderCard = (node: TreeNode) => {
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
            </div>
          </div>
          <span className="pipeline-folder-chevron">›</span>
        </div>
        <div className="pipeline-folder-meta">
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-card-meta-label">Steps:</span>
            <span className="pipeline-card-meta-value">{node.stepIds.length}</span>
          </div>
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-card-meta-label">Sub folders:</span>
            <span className="pipeline-card-meta-value">{node.children.length}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderList = () => (
    <div id="steps-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading steps…</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load steps: {listError}</div>
        ) : (
          <>
            {visibleSteps.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">{visibleSteps.map(item => renderStepCard(item))}</div>
            ) : null}

            {searchTerm.trim() ? null : activeFolderNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeFolderNode.children.map(child => renderFolderCard(child))}
              </div>
            ) : null}

            {!visibleSteps.length && !activeFolderNode.children.length && (
              <div id="steps-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No steps found</h3>
                <p className="text-sm text-[var(--text-secondary)]">Create a new step or adjust your filters.</p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );

  const renderDetail = () => {
    if (!detail) {
      return (
        <div id="steps-detail-view" className="pipelines-view">
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a step to see details.</div>
        </div>
      );
    }
    const source = normalizeSource(detail.source);
    const sourceLabel = source === 'git' ? 'Git' : source === 'draft' ? 'Draft' : 'Database';
    const isGitSource = source === 'git';
    const editorLines = editorValue.split('\n');
    const updatedLabel = source === 'draft' ? 'Draft' : formatUpdatedAt(detail.updatedAt);
    const pathLabel = detail.path || 'root';
    return (
      <div id="steps-detail-view" className="pipelines-view">
        <div className="space-y-6">
          <div className="glass-card p-6">
            <div className="flex items-start justify-between gap-4 w-full mb-4">
              <div className="min-w-0 flex items-start gap-3">
                <span className="step-logo step-logo--detail step-logo--steps mt-1" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M6 10l6-3 6 3-6 3-6-3z"></path>
                    <path d="M6 14l6 3 6-3"></path>
                    <path d="M6 18l6 3 6-3"></path>
                  </svg>
                </span>
                <div className="min-w-0">
                  <h2 id="step-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                    {detail.name || detail.id}
                  </h2>
                  <p id="step-detail-description" className="text-sm text-[var(--text-secondary)] mt-1">
                    {detail.description || 'No description provided.'}
                  </p>
                  <div className="pipeline-detail-meta">
                    <dl className="pipeline-detail-grid">
                      <dt className="pipeline-detail-label">Identifier:</dt>
                      <dd className="pipeline-detail-value" id="step-detail-identifier">{detail.id}</dd>
                      <dt className="pipeline-detail-label">Path:</dt>
                      <dd className="pipeline-detail-value" id="step-detail-path">{pathLabel}</dd>
                      <dt className="pipeline-detail-label">Source:</dt>
                      <dd className="pipeline-detail-value" id="step-detail-source">{sourceLabel}</dd>
                      <dt className="pipeline-detail-label">Last updated:</dt>
                      <dd className="pipeline-detail-value" id="step-detail-updated">{updatedLabel}</dd>
                    </dl>
                  </div>
                </div>
              </div>
              <button id="steps-back-btn" className="glass-button-ghost" onClick={handleBackToList}>
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
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step Definition (YAML)</h3>
                  <div className="flex items-center gap-2 flex-wrap">
                    {!isEditing ? (
                      <>
                        <button className="glass-button-ghost" onClick={handleCopy} title="Copy YAML">
                          <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </button>
                        <button className="glass-button-ghost" onClick={handleDownload} title="Download YAML">
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
                            if (normalizeSource(detail.source) === 'draft') {
                              setDraftSteps(upsertStepDraft({ id: detail.id, yaml: resetYaml }));
                            }
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
                    <div id="step-yaml-content" className="yaml-view">
                      {renderYamlLines(detail.rawYaml)}
                    </div>
                  ) : (
                    <div id="step-editor-container" className="editor-container">
                      <div id="step-line-numbers" ref={lineNumbersRef}>
                        <div className="line-number-track">
                          {editorLines.map((_, idx) => (
                            <div key={`ln-${idx}`} className={`line-number ${validationErrorLines.has(idx + 1) ? 'line-number--error' : ''}`}>
                              {idx + 1}
                            </div>
                          ))}
                        </div>
                      </div>
                      <div id="step-yaml-stage" className="yaml-editor-stage yaml-editor-stage--with-highlight">
                        <div id="step-yaml-highlight" className="yaml-editor-highlight" aria-hidden="true">
                          <pre ref={highlightContentRef} className="yaml-editor-highlight__content">
                            {renderYamlHighlight(editorValue)}
                          </pre>
                        </div>
                        <textarea
                          ref={editorRef}
                          id="step-yaml-editor"
                          value={editorValue}
                          onChange={event => setEditorValue(event.target.value)}
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
                          spellCheck={false}
                        ></textarea>
                        {editorSuggestion && (
                          <div className="pipeline-suggestion-overlay" style={{ top: 12, left: 12, width: 320 }}>
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
                                        <button
                                          type="button"
                                          className="env-suggestion-pill env-suggestion-pill--action"
                                          onClick={() => applyEditorSuggestion(item)}
                                        >
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
                      <div id="step-validation-status" className={`validation-box ${validation.errors.length ? '' : 'validation-box--success'}`}>
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
                <div className="p-4 border-b border-[var(--border-primary)]" style={{ paddingTop: 4 }}>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Used in Pipelines</h3>
                  <p className="text-xs text-[var(--text-secondary)] mt-1">Pipelines currently importing this step.</p>
                </div>
                <div className="p-4">
                  {usageLoading ? (
                    <p className="text-sm text-[var(--text-secondary)]">Loading usage…</p>
                  ) : usageError ? (
                    <p className="text-sm text-red-500">Failed to load usage: {usageError}</p>
                  ) : usage.length ? (
                    <ul className="space-y-2">
                      {usage.map(item => {
                        const pipelineId = item.identifier;
                        return (
                          <li key={pipelineId}>
                            <NavLink
                              className="glass-card p-3 block hover:border-[var(--border-accent)] transition-colors"
                              to={`/pipelines/${pipelineId.split('/').map(encodeURIComponent).join('/')}`}
                            >
                              <div className="flex items-center justify-between gap-2">
                                <span className="text-sm font-medium text-[var(--text-primary)] truncate">{pipelineId}</span>
                                <span className="text-xs text-[var(--text-secondary)] uppercase">{normalizeSource(item.source)}</span>
                              </div>
                              {item.description ? (
                                <p className="text-xs text-[var(--text-secondary)] mt-1 line-clamp-2">{item.description}</p>
                              ) : null}
                            </NavLink>
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No pipelines reference this step.</p>
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
    <div data-page="steps" className="active h-full flex flex-col">
      {!selectedId && (
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
                aria-label="Search steps"
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
                id="steps-search"
                type="search"
                placeholder="Search steps"
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
            <button id="steps-new-btn" type="button" className="pipelines-icon-only" aria-label="Create new step" title="New Step" onClick={openCreateModal}>
              <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 5v14M5 12h14" />
              </svg>
            </button>
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {!selectedId ? renderList() : detailLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading step…</div>
        ) : detailError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load step: {detailError}</div>
        ) : (
          renderDetail()
        )}
      </div>

      {formModal && (
        <div id={formModal.mode === 'create' ? 'steps-new-modal' : 'steps-clone-modal'} className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card max-w-md w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">
                  {formModal.mode === 'create' ? 'New step' : 'Clone step'}
                </p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">
                  {formModal.mode === 'create' ? 'Create step' : 'Clone step'}
                </h3>
              </div>
              <button className="glass-button-ghost" onClick={() => setFormModal(null)}>
                Close
              </button>
            </header>
            <div className="pipelines-modal-body space-y-4">
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Step Path</label>
                <input
                  type="text"
                  className="pipelines-input w-full mt-1"
                  placeholder="library/docker"
                  value={formModal.path}
                  onChange={event => setFormModal(prev => (prev ? { ...prev, path: event.target.value } : prev))}
                />
                <p className="text-xs text-[var(--text-secondary)] mt-1">Optional folder-style path. Leave blank for root.</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Step Name</label>
                <input
                  type="text"
                  className="pipelines-input w-full mt-1"
                  placeholder="build-image"
                  value={formModal.name}
                  onChange={event => setFormModal(prev => (prev ? { ...prev, name: event.target.value } : prev))}
                />
              </div>
              {formModal.error && <p className="text-sm text-red-500">{formModal.error}</p>}
            </div>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button className="glass-button-ghost" onClick={() => setFormModal(null)} disabled={formModal.pending}>
                  Cancel
                </button>
                <button className="glass-button-primary" onClick={submitFormModal} disabled={formModal.pending}>
                  {formModal.pending ? 'Saving…' : formModal.mode === 'create' ? 'Create' : 'Clone'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {deleteModal && (
        <div id="steps-delete-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card max-w-md w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete step</p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Remove {deleteModal.stepName}?</h3>
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
            <div key={toast.id} className={`pipelines-toast pipelines-toast--${toast.tone} show`}>
              <div className="pipelines-toast__content">{toast.message}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default StepsPage;
