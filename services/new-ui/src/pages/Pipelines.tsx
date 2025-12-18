import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';
import {
  PIPELINE_DRAFTS_CHANGED_EVENT,
  PIPELINE_DRAFTS_STORAGE_KEY,
  deletePipelineDraft,
  loadPipelineDrafts,
  type PipelineDraft,
  upsertPipelineDraft,
} from '../lib/pipelineDrafts';
import { calculateGraphLayout, type GraphItem } from '../lib/pipelineGraph';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';

const MAX_RECENT_RUNS = 5;
const MAX_VISIBLE_TRIGGER_CARDS = 5;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;
const PIPELINE_NAME_PATTERN = /^[a-zA-Z0-9_.-]+$/;

const PIPELINE_DIRECTIVES = [
  'name',
  'version',
  'description',
  'container_image',
  'working_directory',
  'variables',
  'steps',
  'timeout',
  'llm_output_sharing',
  'llm_content_sharing',
  'llm_content_ignore',
  'display_options',
];

const STEP_DIRECTIVES = [
  'name',
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

type PipelineListItem = { id: string; source?: string };
type PipelineDetail = {
  id: string;
  name: string;
  description: string;
  version: string;
  path: string;
  rawYaml: string;
  stepNames: string[];
  variables: string[];
  includedDependencies: string[];
  dependencyEdges: { from: string; to: string }[];
  containerImage?: string;
  source?: string;
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
  pipelineId: string;
  pipelineName: string;
  pending: boolean;
  error?: string;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

type PipelineRun = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  status?: string;
  git_repo_owner?: string;
  git_repo_name?: string;
  git_ref?: string;
  duration?: string;
  started_at?: string;
};

type PipelineTrigger = {
  repoSlug: string;
  source: string;
  trigger: Record<string, unknown>;
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
  pipelineIds: string[];
};

function PipelinesPage() {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverPipelines, setServerPipelines] = useState<PipelineListItem[]>([]);
  const [draftPipelines, setDraftPipelines] = useState<PipelineDraft[]>([]);
  const [listLoading, setListLoading] = useState<boolean>(true);
  const [listError, setListError] = useState<string | null>(null);
  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selectedIdRef = useRef<string | null>(null);

  const [detail, setDetail] = useState<PipelineDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [recentRuns, setRecentRuns] = useState<PipelineRun[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string | null>(null);
  const [triggers, setTriggers] = useState<PipelineTrigger[]>([]);
  const [triggersLoading, setTriggersLoading] = useState(false);
  const [triggersError, setTriggersError] = useState<string | null>(null);

  const [isEditing, setIsEditing] = useState(false);
  const [editorValue, setEditorValue] = useState('');
  const [saving, setSaving] = useState(false);

  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const highlightContentRef = useRef<HTMLPreElement | null>(null);
  const lineNumbersRef = useRef<HTMLDivElement | null>(null);
  const autocompleteFetchRef = useRef<{ fetchedAt: number; loadingPromise: Promise<void> | null }>({
    fetchedAt: 0,
    loadingPromise: null,
  });
  const editSessionOriginalYamlRef = useRef<string>('');
  const wasEditingRef = useRef(false);

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

  const validatePipelineYaml = useCallback((rawYaml: string): ValidationResult => {
    const errors: ValidationError[] = [];
    let parsed: any = null;
    try {
      parsed = yaml.load(rawYaml) as any;
    } catch (error: any) {
      const mark = error?.mark;
      errors.push({
        message: error instanceof Error ? error.message : 'Invalid YAML',
        line: typeof mark?.line === 'number' ? mark.line + 1 : undefined,
        column: typeof mark?.column === 'number' ? mark.column + 1 : undefined,
      });
      return { errors };
    }

    if (!parsed || typeof parsed !== 'object') {
      return { errors: [{ message: 'YAML must define an object at the root.' }] };
    }

    const safeString = (value: unknown) => (typeof value === 'string' ? value.trim() : '');

    const name = safeString(parsed.name);
    if (!name) {
      errors.push({ message: "'name' is a required field" });
    } else if (!PIPELINE_NAME_PATTERN.test(name)) {
      errors.push({ message: 'Pipeline name can only contain alphanumeric characters, underscores, dots, and hyphens.' });
    }

    const version = safeString(parsed.version);
    if (version && !PIPELINE_NAME_PATTERN.test(version)) {
      errors.push({ message: 'Pipeline version can only contain alphanumeric characters, underscores, dots, and hyphens.' });
    }

    const steps = Array.isArray(parsed.steps) ? parsed.steps : [];
    if (steps.length === 0) {
      errors.push({ message: 'At least one step is required.' });
      return { errors };
    }

    const containerImage = safeString(parsed.container_image);
    const firstStepImage = safeString((steps[0] as any)?.image);
    if (!containerImage && steps.length > 0 && !firstStepImage) {
      errors.push({ message: "'container_image' is required when steps do not specify their own image." });
    }

    const allStepNames = new Set<string>();
    const stepToTaskNames = new Map<string, Set<string>>();
    const stepTaskMeta = new Map<string, { hasTasks: boolean; taskNames: Set<string> }>();

    for (const stepRaw of steps) {
      const step = stepRaw && typeof stepRaw === 'object' ? (stepRaw as any) : null;
      const stepName = safeString(step?.name);
      if (!stepName) {
        errors.push({ message: "A step is missing its required 'name' field." });
        continue;
      }
      if (allStepNames.has(stepName)) {
        errors.push({ message: `Duplicate step name '${stepName}' found. Step names must be unique.` });
        continue;
      }
      allStepNames.add(stepName);
      stepToTaskNames.set(stepName, new Set());

      const includeValue = safeString(step?.include);
      const tasks = Array.isArray(step?.tasks) ? step.tasks : [];
      const hasTasks = tasks.length > 0;
      const hasLegacy = Boolean(safeString(step?.goal) || safeString(step?.script));

      if (includeValue) {
        if (hasTasks || hasLegacy) {
          errors.push({
            message: `Step '${stepName}' is an 'include' step and cannot also contain 'tasks', 'goal', or 'script'.`,
          });
        }
      } else if (hasTasks) {
        if (hasLegacy) {
          errors.push({ message: `Step '${stepName}' has tasks and should not also contain 'goal' or 'script'.` });
        }
        const taskNames = stepToTaskNames.get(stepName)!;
        for (const taskRaw of tasks) {
          const task = taskRaw && typeof taskRaw === 'object' ? (taskRaw as any) : null;
          const taskName = safeString(task?.name);
          if (!taskName) {
            errors.push({ message: `A task in step '${stepName}' is missing its required 'name' field.` });
            continue;
          }
          if (taskNames.has(taskName)) {
            errors.push({
              message: `Duplicate task name '${taskName}' found within step '${stepName}'. Task names must be unique within a step.`,
            });
            continue;
          }
          taskNames.add(taskName);

          const hasGoal = Boolean(safeString(task?.goal));
          const hasScript = Boolean(safeString(task?.script));
          if (hasGoal && hasScript) {
            errors.push({ message: `Task '${taskName}' in step '${stepName}' cannot define both 'goal' and 'script'.` });
          } else if (!hasGoal && !hasScript) {
            errors.push({ message: `Task '${taskName}' in step '${stepName}' must define either 'goal' or 'script'.` });
          }
        }
      } else if (!hasLegacy) {
        errors.push({ message: `Step '${stepName}' must contain 'include', 'tasks', 'goal', or 'script'.` });
      }

      const taskNames = stepToTaskNames.get(stepName)!;
      stepTaskMeta.set(stepName, { hasTasks, taskNames });
    }

    for (const stepRaw of steps) {
      const step = stepRaw && typeof stepRaw === 'object' ? (stepRaw as any) : null;
      const stepName = safeString(step?.name);
      if (!stepName) continue;

      const dependsOn = Array.isArray(step?.depends_on) ? step.depends_on : [];
      for (const depRaw of dependsOn) {
        const depName = safeString(depRaw);
        if (depName && !allStepNames.has(depName)) {
          errors.push({ message: `Step '${stepName}' has an undefined dependency: '${depName}'.` });
        }
      }

      const taskInfo = stepTaskMeta.get(stepName);
      if (taskInfo?.hasTasks) {
        const tasks = Array.isArray(step?.tasks) ? step.tasks : [];
        for (const taskRaw of tasks) {
          const task = taskRaw && typeof taskRaw === 'object' ? (taskRaw as any) : null;
          const taskName = safeString(task?.name);
          if (!taskName) continue;
          const taskDependsOn = Array.isArray(task?.depends_on) ? task.depends_on : [];
          for (const depRaw of taskDependsOn) {
            const depName = safeString(depRaw);
            if (depName && !taskInfo.taskNames.has(depName)) {
              errors.push({
                message: `Task '${taskName}' in step '${stepName}' has an invalid dependency: '${depName}'. Tasks can only depend on other tasks within the same step.`,
              });
            }
          }
        }
      }
    }

    return { errors };
  }, []);

  const validation = useMemo(() => {
    if (!isEditing) return { errors: [] };
    return validatePipelineYaml(editorValue);
  }, [editorValue, isEditing, validatePipelineYaml]);

  const validationErrorLines = useMemo(() => {
    const lines = new Set<number>();
    validation.errors.forEach(err => {
      if (typeof err.line === 'number') lines.add(err.line);
    });
    return lines;
  }, [validation.errors]);

  const [formModal, setFormModal] = useState<FormModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<DeleteModalState | null>(null);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const draftsById = useMemo(() => {
    const map = new Map<string, PipelineDraft>();
    draftPipelines.forEach(draft => map.set(draft.id, draft));
    return map;
  }, [draftPipelines]);

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
      const prefixMatch = lineBeforeCursor.match(/[A-Za-z0-9_.-]+$/);
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

      const includeValueContext =
        currentKey === 'include' ||
        /^\s*include\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());

      const resolveStepNames = () => {
        if (!detail) return [];
        try {
          return parsePipelineYaml(text, detail.id, detail.source).stepNames;
        } catch {
          return [];
        }
      };

      let title = 'Suggestions';
      let pool: string[] = [];
      let appendColon = false;

      if (includeValueContext) {
        title = 'Reusable steps';
        pool = autocompleteMeta.reusableSteps;
      } else if (parentKey === 'secrets') {
        title = 'Secrets';
        pool = autocompleteMeta.secrets;
      } else if (parentKey === 'variables') {
        title = 'Variables';
        pool = autocompleteMeta.variables;
      } else if (parentKey === 'depends_on') {
        title = 'Step dependencies';
        pool = resolveStepNames();
      } else {
        appendColon = true;
        if (parentKey === 'tasks') {
          title = 'Task keys';
          pool = TASK_DIRECTIVES;
        } else if (parentKey === 'steps') {
          title = 'Step keys';
          pool = STEP_DIRECTIVES;
        } else {
          title = 'Pipeline keys';
          pool = PIPELINE_DIRECTIVES;
        }
      }

      const normalizedPrefix = prefix.toLowerCase();
      const filtered = pool
        .filter(item => item.toLowerCase().startsWith(normalizedPrefix))
        .sort((a, b) => a.localeCompare(b));

      if (!filtered.length) {
        setEditorSuggestion({
          title,
          items: [],
          activeIndex: 0,
          replaceStart,
          replaceEnd,
          appendColon,
        });
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
    [autocompleteMeta, detail, editorValue]
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
        const normalize = (payload: unknown): string[] => {
          if (!Array.isArray(payload)) return [];
          return payload
            .map(item => {
              if (typeof item === 'string') return item.trim();
              if (item && typeof item === 'object') {
                const name = (item as any).name;
                if (typeof name === 'string') return name.trim();
              }
              return '';
            })
            .filter(Boolean);
        };

        const promise = (async () => {
          const [secretsResp, varsResp, stepsResp] = await Promise.all([
            fetch(buildApiUrl('/v1/secrets')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/variables')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/steps')).then(r => (r.ok ? r.json() : [])),
          ]);

          setAutocompleteMeta({
            secrets: normalize(secretsResp),
            variables: normalize(varsResp),
            reusableSteps: normalize(stepsResp),
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

  const addToast = useCallback((message: string, tone: ToastMessage['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const encodeId = (id: string) => id.split('/').map(encodeURIComponent).join('/');

  const splitIdentifier = (id: string) => {
    const parts = id.split('/').filter(Boolean);
    const name = decodeURIComponent(parts.pop() || '');
    const path = parts.map(decodeURIComponent).join('/');
    return { name, path };
  };

  const normalizeSource = (source?: string) => {
    const key = (source || '').trim().toLowerCase();
    if (!key) return 'database';
    if (key.includes('git')) return 'git';
    if (key.includes('draft')) return 'draft';
    if (key.includes('db')) return 'database';
    return key;
  };

  const normalizePipelineIdentifier = (value: unknown) => {
    if (!value) return '';
    return String(value)
      .trim()
      .replace(/^\.nopsai\//, '')
      .replace(/\.ya?ml$/i, '')
      .replace(/\/+/g, '/')
      .replace(/^\//, '');
  };

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

  const formatTriggerEvent = (value: unknown) => {
    if (Array.isArray(value)) {
      return value.map(item => String(item)).join(', ');
    }
    if (!value) return 'N/A';
    const raw = String(value).toLowerCase();
    switch (raw) {
      case 'push':
        return 'Push';
      case 'pull_request':
      case 'pull-request':
        return 'Pull request';
      case 'schedule':
        return 'Schedule';
      default:
        return String(value);
    }
  };

  const formatTriggerBranchField = (trigger: Record<string, unknown>) => {
    const branches = Array.isArray(trigger.branches) ? trigger.branches.map(item => String(item)).filter(Boolean) : [];
    const skip = Array.isArray(trigger.skip_branches) ? trigger.skip_branches.map(item => String(item)).filter(Boolean) : [];
    if (branches.length) {
      return { label: 'branches:', value: branches.join(', ') };
    }
    if (skip.length) {
      return { label: 'skip_branches:', value: skip.join(', ') };
    }
    return { label: 'branches:', value: 'All branches' };
  };

  const formatTriggerScope = (trigger: Record<string, unknown>) => {
    const scope = typeof trigger.scope === 'string' ? trigger.scope.trim() : '';
    return scope || 'default';
  };

  const parsePipelineYaml = (raw: string, id: string, source?: string): PipelineDetail => {
    let parsed: Record<string, unknown> | null = null;
    try {
      parsed = yaml.load(raw) as Record<string, unknown>;
    } catch (error) {
      console.warn('Failed to parse pipeline YAML', error);
    }

    const safe = (value: unknown) => (typeof value === 'string' ? value : '');
    const includedDeps: string[] = [];
    const dependencyEdges: { from: string; to: string }[] = [];
    const stepNames = Array.isArray(parsed?.steps)
      ? (parsed?.steps as Array<Record<string, unknown>>)
          .map(step => {
            const stepName = safe(step?.name).trim();
            const includeVal = safe(step?.include).trim();
            if (includeVal) includedDeps.push(includeVal);
            const deps = Array.isArray(step?.depends_on) ? (step?.depends_on as unknown[]) : [];
            deps.forEach(dep => {
              const from = safe(dep).trim();
              if (from && stepName) {
                dependencyEdges.push({ from, to: stepName });
              }
            });
            return stepName;
          })
          .filter(Boolean)
      : [];
    const variables = Array.isArray(parsed?.variables)
      ? (parsed?.variables as unknown[])
          .map(item => (typeof item === 'string' ? item : ''))
          .filter(Boolean)
      : [];

    const { name: fallbackName, path } = splitIdentifier(id);
    return {
      id,
      name: safe(parsed?.name) || fallbackName,
      description: safe(parsed?.description) || 'No description provided.',
      version: safe(parsed?.version) || 'latest',
      path,
      rawYaml: raw,
      stepNames,
      variables,
      includedDependencies: includedDeps,
      dependencyEdges,
      containerImage: safe((parsed as Record<string, unknown> | undefined)?.container_image ?? (parsed as Record<string, unknown> | undefined)?.containerImage),
      source,
    };
  };

  const buildTemplateYaml = (name: string) => {
    return [
      `name: ${name}`,
      'version: v1',
      'description: Describe what this pipeline does.',
      'container_image: alpine:3.20',
      'variables: []',
      'steps:',
      '  - name: example',
      '    goal: Say hello from this pipeline.',
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
    navigate(cleaned ? `/pipelines?folder=${encodeURIComponent(cleaned)}` : '/pipelines');
  };

  const loadRecentRuns = useCallback(
    async (pipelineId: string) => {
      const targetId = pipelineId;
      setRunsLoading(true);
      setRunsError(null);
      try {
        const response = await fetch(buildApiUrl('/v1/runs'));
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to load runs (${response.status})`);
        }
        const payload = await response.json();
        const runsPayload: PipelineRun[] = Array.isArray(payload) ? payload : [];
        const { path, name } = splitIdentifier(pipelineId);
        const normalizedName = name.toLowerCase();
        const normalizedPath = (path || '').toLowerCase();
        const filtered = runsPayload
          .filter(run => (run?.pipeline_name || '').toLowerCase() === normalizedName && (run?.pipeline_path || '').toLowerCase() === normalizedPath)
          .sort((a, b) => {
            const aTime = new Date(a.started_at || '').getTime() || 0;
            const bTime = new Date(b.started_at || '').getTime() || 0;
            return bTime - aTime;
          })
          .slice(0, MAX_RECENT_RUNS);
        if (selectedIdRef.current === targetId) {
          setRecentRuns(filtered);
        }
      } catch (error) {
        console.error('Failed to load runs', error);
        if (selectedIdRef.current === targetId) {
          setRunsError(error instanceof Error ? error.message : 'Unable to load runs');
          setRecentRuns([]);
        }
      } finally {
        if (selectedIdRef.current === targetId) {
          setRunsLoading(false);
        }
      }
    },
    []
  );

  const loadPipelineTriggers = useCallback(
    async (pipelineId: string) => {
      const targetId = pipelineId;
      const normalizedTarget = normalizePipelineIdentifier(pipelineId);
      if (!normalizedTarget) {
        setTriggers([]);
        return;
      }

      setTriggersLoading(true);
      setTriggersError(null);
      try {
        const listResponse = await fetch(buildApiUrl('/v1/overrides?include_source=true'));
        if (!listResponse.ok) {
          const text = await listResponse.text();
          throw new Error(text || `Failed to load overrides (${listResponse.status})`);
        }
        const overridesPayload = await listResponse.json();
        const overrides: any[] = Array.isArray(overridesPayload) ? overridesPayload : [];
        const results: PipelineTrigger[] = [];

        await Promise.all(
          overrides.map(async entry => {
            try {
              const slugRaw =
                typeof entry === 'string'
                  ? entry
                  : entry?.name || entry?.repository_name || entry?.slug || entry?.repo || '';
              const repoSlug = String(slugRaw || '').trim();
              if (!repoSlug || !repoSlug.includes('/')) return;
              const [owner, repo] = repoSlug.split('/');
              const overrideResponse = await fetch(
                buildApiUrl(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`)
              );
              if (!overrideResponse.ok) return;
              const yamlText = await overrideResponse.text();
              const manifest = yaml.load(yamlText) as Record<string, unknown>;
              const triggerList = Array.isArray(manifest?.triggers) ? manifest?.triggers : [];
              triggerList.forEach(item => {
                const trigger = (item || {}) as Record<string, unknown>;
                const pipelines = Array.isArray((trigger as any).pipelines) ? (trigger as any).pipelines : [];
                const matches = pipelines.some((value: unknown) => {
                  const candidate = typeof value === 'string' ? value : (value as any)?.path;
                  return normalizePipelineIdentifier(candidate) === normalizedTarget;
                });
                if (matches) {
                  results.push({
                    repoSlug,
                    source: normalizeSource(typeof entry === 'string' ? 'database' : entry?.source),
                    trigger,
                  });
                }
              });
            } catch (innerError) {
              console.warn('Failed to parse overrides entry', innerError);
            }
          })
        );

        results.sort((a, b) => a.repoSlug.localeCompare(b.repoSlug));
        if (selectedIdRef.current === targetId) {
          setTriggers(results);
        }
      } catch (error) {
        console.error('Failed to load triggers', error);
        if (selectedIdRef.current === targetId) {
          setTriggersError(error instanceof Error ? error.message : 'Unable to load triggers');
          setTriggers([]);
        }
      } finally {
        if (selectedIdRef.current === targetId) {
          setTriggersLoading(false);
        }
      }
    },
    []
  );

  const loadPipelines = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) {
      setListLoading(true);
    }
    setListError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/pipelines?include_source=true'));
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load pipelines (${response.status})`);
      }
      const payload = await response.json();
      const normalized: PipelineListItem[] = Array.isArray(payload)
        ? payload
            .map((item: any) => {
              if (typeof item === 'string') return { id: item };
              if (item && typeof item === 'object') {
                const id = typeof item.id === 'string' ? item.id : typeof item.identifier === 'string' ? item.identifier : '';
                return id ? { id, source: item.source } : null;
              }
              return null;
            })
            .filter(Boolean) as PipelineListItem[]
        : [];
      normalized.sort((a, b) => a.id.localeCompare(b.id));
      setServerPipelines(normalized);
    } catch (error) {
      console.error('Failed to load pipelines', error);
      setListError(error instanceof Error ? error.message : 'Unable to load pipelines');
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadPipelineDetail = useCallback(
    async (pipelineId: string, source?: string) => {
      const normalizedSource = normalizeSource(source);
      setDetailLoading(true);
      setDetailError(null);
      try {
        if (normalizedSource === 'draft') {
          const draft = draftsById.get(pipelineId);
          if (!draft) throw new Error('Draft pipeline not found');
          const parsed = parsePipelineYaml(draft.yaml, pipelineId, 'draft');
          setDetail(parsed);
          setEditorValue(draft.yaml);
          setIsEditing(true);
          return;
        }

        const response = await fetch(buildApiUrl(`/v1/pipelines/${encodeId(pipelineId)}`));
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to fetch pipeline (${response.status})`);
        }
        const rawYaml = await response.text();
        const parsed = parsePipelineYaml(rawYaml, pipelineId, normalizedSource);
        setDetail(parsed);
        setEditorValue(rawYaml);
        setIsEditing(false);
      } catch (error) {
        console.error('Failed to fetch pipeline', error);
        setDetailError(error instanceof Error ? error.message : 'Unable to load pipeline details');
      } finally {
        setDetailLoading(false);
      }
    },
    [draftsById]
  );

  useEffect(() => {
    setDraftPipelines(loadPipelineDrafts());
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const refreshDrafts = () => setDraftPipelines(loadPipelineDrafts());
    const onStorage = (event: StorageEvent) => {
      if (event.key !== PIPELINE_DRAFTS_STORAGE_KEY) return;
      refreshDrafts();
    };
    window.addEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, refreshDrafts);
    window.addEventListener('storage', onStorage);
    return () => {
      window.removeEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, refreshDrafts);
      window.removeEventListener('storage', onStorage);
    };
  }, []);

  const pipelines = useMemo(() => {
    const merged = new Map<string, PipelineListItem>();
    serverPipelines.forEach(item => merged.set(item.id, item));
    draftPipelines.forEach(draft => merged.set(draft.id, { id: draft.id, source: 'draft' }));
    return Array.from(merged.values()).sort((a, b) => a.id.localeCompare(b.id));
  }, [serverPipelines, draftPipelines]);

  useEffect(() => {
    void loadPipelines();
  }, [loadPipelines]);

  useEffect(() => {
    if (listLoading || listError) return;
    const activeId = selectedIdRef.current;
    if (!activeId) return;
    if (draftsById.has(activeId)) return;
    if (serverPipelines.some(item => item.id === activeId)) return;
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/pipelines', { replace: true });
  }, [listLoading, listError, draftsById, serverPipelines, navigate]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'pipelines') return;
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
    const folder = params.get('folder') || '';
    setActiveFolder(folder);
  }, [location.pathname, location.search]);

  useEffect(() => {
    if (!selectedId) {
      setDetail(null);
      setEditorValue('');
      setIsEditing(false);
      return;
    }
    const source = pipelines.find(item => item.id === selectedId)?.source;
    void loadPipelineDetail(selectedId, source);
  }, [selectedId, pipelines, loadPipelineDetail]);

  useEffect(() => {
    if (!isEditing) return;
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
      setDraftPipelines(upsertPipelineDraft({ id: draftId, yaml: editorValue }));
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
  }, [isEditing, editorValue, syncEditorOverlays]);

  useEffect(() => {
    if (!selectedId) {
      setRecentRuns([]);
      setTriggers([]);
      setRunsError(null);
      setTriggersError(null);
      return;
    }
    void loadRecentRuns(selectedId);
    void loadPipelineTriggers(selectedId);
  }, [selectedId, loadPipelineTriggers, loadRecentRuns]);

  const filteredPipelines = useMemo(() => {
    const query = searchTerm.trim().toLowerCase();
    if (!query) return pipelines;
    return pipelines.filter(item => item.id.toLowerCase().includes(query));
  }, [pipelines, searchTerm]);

  const visiblePipelines = useMemo(() => {
    const list = searchTerm.trim()
      ? filteredPipelines
      : filteredPipelines.filter(item => splitIdentifier(item.id).path === activeFolder);
    return [...list].sort((a, b) => a.id.localeCompare(b.id));
  }, [filteredPipelines, searchTerm, activeFolder]);

  const buildTree = useMemo(() => {
    const root: TreeNode = { id: '__root__', name: '', fullPath: '', children: [], pipelineIds: [] };
    pipelines.forEach(item => {
      const parts = item.id.split('/').filter(Boolean);
      const pipelineName = parts.pop();
      if (!pipelineName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], pipelineIds: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.pipelineIds.push(item.id);
      current.pipelineIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [pipelines]);

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

  const handleSelect = (id: string) => {
    selectedIdRef.current = id;
    setSelectedId(id);
    navigate(`/pipelines/${id.split('/').map(encodeURIComponent).join('/')}`);
  };

  const handleBackToList = () => {
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/pipelines');
  };

  const handleCopy = async () => {
    if (!detail?.rawYaml) return;
    try {
      await navigator.clipboard.writeText(detail.rawYaml);
      addToast('Pipeline YAML copied to clipboard.', 'success');
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
    link.download = `${name || 'pipeline'}.yaml`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const handleSave = async () => {
    if (!detail || !editorValue.trim()) return;
    if (normalizeSource(detail.source) === 'git') {
      addToast('Git-managed pipelines are read-only. Clone it to create an editable draft.', 'info');
      return;
    }
    const validationResult = validatePipelineYaml(editorValue);
    if (validationResult.errors.length) {
      addToast('Resolve validation errors before saving.', 'error');
      return;
    }
    setSaving(true);
    try {
      const response = await fetch(buildApiUrl(`/v1/pipelines/${encodeId(detail.id)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: editorValue,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to save pipeline (${response.status})`);
      }
      addToast('Pipeline saved.', 'success');
      const wasDraft = normalizeSource(detail.source) === 'draft';
      if (wasDraft) {
        setDraftPipelines(deletePipelineDraft(detail.id));
      }
      const resolvedSource = wasDraft ? 'database' : pipelines.find(item => item.id === detail.id)?.source;
      const updated = parsePipelineYaml(editorValue, detail.id, resolvedSource);
      setDetail(updated);
      setEditorValue(editorValue);
      setIsEditing(false);
      await loadPipelines({ quiet: true });
    } catch (error) {
      console.error('Save failed', error);
      addToast(error instanceof Error ? error.message : 'Unable to save pipeline', 'error');
    } finally {
      setSaving(false);
    }
  };

  const openCreateModal = () => setFormModal({ mode: 'create', path: activeFolder, name: '', pending: false });
  const openCloneModal = () => {
    if (!detail) {
      addToast('Select a pipeline to clone.', 'info');
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
      setFormModal(prev => prev ? { ...prev, error: 'Pipeline name is required.' } : prev);
      return;
    }
    if (!/^[a-zA-Z0-9_.-]+$/.test(formModal.name.trim())) {
      setFormModal(prev => prev ? { ...prev, error: 'Pipeline name can only contain letters, numbers, dots, underscores, and hyphens.' } : prev);
      return;
    }
    if (pipelines.some(item => item.id === identifier)) {
      setFormModal(prev => prev ? { ...prev, error: 'A pipeline with that identifier already exists.' } : prev);
      return;
    }
    setFormModal(prev => prev ? { ...prev, pending: true, error: undefined } : prev);
    try {
      const yamlBody = formModal.mode === 'clone' && formModal.baseYaml
        ? updateYamlName(formModal.baseYaml, formModal.name.trim())
        : buildTemplateYaml(formModal.name.trim());
      setDraftPipelines(upsertPipelineDraft({ id: identifier, yaml: yamlBody }));
      addToast(`Draft pipeline ${formModal.mode === 'create' ? 'created' : 'cloned'}.`, 'success');
      setFormModal(null);
      handleSelect(identifier);
    } catch (error) {
      console.error('Draft save failed', error);
      setFormModal(prev => prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to create draft' } : prev);
    } finally {
      setFormModal(prev => prev ? { ...prev, pending: false } : prev);
    }
  };

  const confirmDelete = async () => {
    if (!deleteModal) return;
    setDeleteModal(prev => prev ? { ...prev, pending: true, error: undefined } : prev);
    try {
      const source = pipelines.find(item => item.id === deleteModal.pipelineId)?.source;
      if (normalizeSource(source) === 'git') {
        throw new Error('This pipeline is managed via Git. Clone it to customize instead of deleting.');
      }
      if (normalizeSource(source) === 'draft') {
        setDraftPipelines(deletePipelineDraft(deleteModal.pipelineId));
      } else {
        const response = await fetch(buildApiUrl(`/v1/pipelines/${encodeId(deleteModal.pipelineId)}`), { method: 'DELETE' });
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to delete pipeline (${response.status})`);
        }
      }
      addToast('Pipeline deleted.', 'success');
      setDeleteModal(null);
      setSelectedId(null);
      selectedIdRef.current = null;
      navigate('/pipelines');
      await loadPipelines();
    } catch (error) {
      console.error('Delete failed', error);
      setDeleteModal(prev => prev ? { ...prev, error: error instanceof Error ? error.message : 'Unable to delete pipeline' } : prev);
    } finally {
      setDeleteModal(prev => prev ? { ...prev, pending: false } : prev);
    }
  };

  const renderPipelineCard = (pipeline: PipelineListItem) => {
    const { name, path } = splitIdentifier(pipeline.id);
    const source = normalizeSource(pipeline.source);
    return (
      <article
        key={pipeline.id}
        className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
        onClick={() => handleSelect(pipeline.id)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="pipeline-card-icon" aria-hidden="true">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="3" />
                <path d="M6 12h2m8 0h2M12 6v2m0 8v2" />
              </svg>
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{name || pipeline.id}</h3>
              <p className="pipeline-card-path">{path || 'root'}</p>
              <p className="pipeline-card-description">A sample pipeline.</p>
            </div>
          </div>
          <div className="pipeline-card-actions">
            <button
              type="button"
              className="pipelines-delete-button"
              aria-disabled={source === 'git'}
              title={source === 'git' ? 'This pipeline is managed via Git. Clone it to customize.' : 'Delete pipeline'}
              onClick={event => {
                event.stopPropagation();
                if (source === 'git') return;
                setDeleteModal({ pipelineId: pipeline.id, pipelineName: name || pipeline.id, pending: false });
              }}
              aria-label="Delete pipeline"
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
            <span className="pipeline-card-meta-label">Version</span>
            <span className="pipeline-card-meta-value">latest</span>
          </div>
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
              <p className="pipeline-card-path">folder</p>
            </div>
          </div>
          <span className="pipeline-folder-chevron">›</span>
        </div>
        <div className="pipeline-folder-meta">
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-card-meta-label">Pipelines:</span>
            <span className="pipeline-card-meta-value">{node.pipelineIds.length}</span>
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
    <div id="pipelines-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading pipelines…</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load pipelines: {listError}</div>
        ) : (
          <>
            {visiblePipelines.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">
                {visiblePipelines.map(item => renderPipelineCard(item))}
              </div>
            ) : null}

            {searchTerm.trim() ? null : activeFolderNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeFolderNode.children.map(child => renderFolderCard(child))}
              </div>
            ) : null}

            {!visiblePipelines.length && !activeFolderNode.children.length && (
              <div id="pipelines-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No pipelines found</h3>
                <p className="text-sm text-[var(--text-secondary)]">Create a new pipeline or adjust your filters.</p>
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
        <div id="pipelines-detail-view" className="pipelines-view">
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a pipeline to see details.</div>
        </div>
      );
    }
    const source = normalizeSource(detail.source);
    const isGitSource = source === 'git';
    const editorLines = editorValue.split('\n');
    return (
      <div id="pipelines-detail-view" className="pipelines-view">
        <div className="space-y-6">
          <div className="glass-card p-6">
            <div className="flex items-start justify-between gap-4 w-full mb-4">
              <div>
                <h2 id="pipeline-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                  {detail.name || detail.id}
                </h2>
                <p id="pipeline-detail-description" className="text-sm text-[var(--text-secondary)] mt-1">
                  {detail.description || 'No description provided.'}
                </p>
                <div className="flex flex-wrap gap-3 mt-3 text-xs uppercase tracking-wide text-[var(--text-secondary)]">
                  <span>Path: <span className="text-[var(--text-primary)]" id="pipeline-detail-path">{detail.path || 'Root'}</span></span>
                  <span>Version: <span className="text-[var(--text-primary)]" id="pipeline-detail-version">{detail.version || 'latest'}</span></span>
                  <span>Source: <span className="text-[var(--text-primary)]" id="pipeline-detail-source">{source}</span></span>
                </div>
              </div>
              <button id="pipelines-back-btn" className="glass-button-ghost" onClick={handleBackToList}>
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
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Pipeline Definition (YAML)</h3>
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
                              setDraftPipelines(upsertPipelineDraft({ id: detail.id, yaml: resetYaml }));
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
                    <div id="pipeline-yaml-content" className="yaml-view">
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
                      <div id="pipeline-yaml-stage" className="yaml-editor-stage yaml-editor-stage--with-highlight">
                        <div id="pipeline-yaml-highlight" className="yaml-editor-highlight" aria-hidden="true">
                          <pre ref={highlightContentRef} className="yaml-editor-highlight__content">
                            {renderYamlHighlight(editorValue)}
                          </pre>
                        </div>
                        <textarea
                          ref={editorRef}
                          id="pipeline-yaml-editor"
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

              <div className="glass-card overflow-hidden">
                <div className="p-4">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step Dependency Graph</h3>
                  <p className="text-xs text-[var(--text-secondary)] mt-1">Based on `depends_on` relationships.</p>
                </div>
                <div className="pipelines-graph">
                  {(() => {
                    const graphYaml = isEditing ? editorValue : detail.rawYaml;
                    let steps: GraphItem[] = [];
                    let parseError: string | null = null;
                    try {
                      const parsed = yaml.load(graphYaml) as any;
                      const rawSteps = Array.isArray(parsed?.steps) ? parsed.steps : [];
	                      steps = rawSteps
	                        .map((step: any) => ({
	                          name: typeof step?.name === 'string' ? step.name.trim() : '',
	                          depends_on: Array.isArray(step?.depends_on)
	                            ? step.depends_on.map((dep: any) => (typeof dep === 'string' ? dep.trim() : '')).filter(Boolean)
	                            : [],
	                        }))
	                        .filter((item: GraphItem) => item.name);
	                    } catch (error) {
	                      parseError = error instanceof Error ? error.message : 'Unable to parse YAML';
	                    }

                    if (parseError) {
                      return <p className="text-sm text-red-500">Unable to render graph: {parseError}</p>;
                    }
                    if (!steps.length) {
                      return <p className="text-sm text-[var(--text-secondary)]">No steps defined in this pipeline.</p>;
                    }

                    const layout = calculateGraphLayout(steps);

                    const iconRadius = 12;
                    const arrowPad = 3;
                    const pathBetween = (fromNode: any, toNode: any) => {
                      const fromCx = fromNode.x + fromNode.width / 2;
                      const fromCy = fromNode.y + fromNode.height / 2;
                      const toCx = toNode.x + toNode.width / 2;
                      const toCy = toNode.y + toNode.height / 2;
                      const sx = fromCx + iconRadius + arrowPad;
                      const sy = fromCy;
                      const tx = toCx - iconRadius - arrowPad;
                      const ty = toCy;
                      const curveX = sx + (tx - sx) * 0.5;
                      return `M ${sx} ${sy} C ${curveX} ${sy}, ${curveX} ${ty}, ${tx} ${ty}`;
                    };

                    return (
                      <div id="pipeline-graph" style={{ overflow: 'auto' }}>
                        <svg
                          viewBox={`0 0 ${layout.width} ${layout.height}`}
                          preserveAspectRatio="xMinYMin meet"
                          xmlns="http://www.w3.org/2000/svg"
                          style={{ width: '100%', height: 'auto', display: 'block' }}
                        >
                          <defs>
                            <radialGradient id="glassyIconGradientPipelineDef" cx="40%" cy="35%" r="80%" fx="30%" fy="30%">
                              <stop offset="0%" stopColor="rgba(254, 252, 232, 0.9)" />
                              <stop offset="50%" stopColor="rgba(250, 204, 21, 0.85)" />
                              <stop offset="100%" stopColor="rgba(217, 119, 6, 0.9)" />
                            </radialGradient>
                            <filter id="softIconShadowPipelineDef" x="-40%" y="-40%" width="180%" height="180%">
                              <feDropShadow dx="1" dy="3" stdDeviation="2.5" floodColor="#a16207" floodOpacity="0.25" />
                            </filter>
                            <marker id="pipeline-def-arrowhead" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
                              <path d="M0,0 L8,4 L0,8 Q2.4,4 0,0 Z" fill="var(--border-secondary)" />
                            </marker>
                          </defs>
                          <rect x="0" y="0" width={layout.width} height={layout.height} fill="transparent" style={{ pointerEvents: 'all' }} />
                          {layout.edges.map((edge, idx) => {
                            const d = pathBetween(edge.from, edge.to);
                            return (
                              <g key={`edge-${idx}`}>
                                <path d={d} className="edge-path-halo" />
                                <path d={d} className="edge-path" markerEnd="url(#pipeline-def-arrowhead)" />
                              </g>
                            );
                          })}
                          {layout.nodes.map(node => {
                            const nodeCenterX = node.x + node.width / 2;
                            const nodeCenterY = node.y + node.height / 2;
                            return (
                              <g key={node.id} className="graph-node graph-node-pipeline-def" data-step-name={node.name}>
                                <circle
                                  cx={nodeCenterX}
                                  cy={nodeCenterY}
                                  r={iconRadius}
                                  fill="url(#glassyIconGradientPipelineDef)"
                                  stroke="rgba(202, 138, 4, 0.25)"
                                  strokeWidth="0.5"
                                  filter="url(#softIconShadowPipelineDef)"
                                  opacity="0.95"
                                />
                                <text x={nodeCenterX} y={nodeCenterY + 35} textAnchor="middle" className="pipeline-def-node-label">
                                  {node.name}
                                </text>
                                <text x={nodeCenterX} y={nodeCenterY + 48} textAnchor="middle" className="pipeline-def-node-sublabel">
                                  Defined
                                </text>
                              </g>
                            );
                          })}
                        </svg>
                      </div>
                    );
                  })()}
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <div className="glass-card overflow-hidden">
                <div className="p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Trigger Rules</h3>
                </div>
                <div className="p-4">
                  {triggersLoading ? (
                    <p className="text-sm text-[var(--text-secondary)]">Loading triggers…</p>
                  ) : triggersError ? (
                    <p className="text-sm text-red-500">Failed to load triggers: {triggersError}</p>
                  ) : triggers.length ? (
                    <ul className={`triggers-pipeline-list ${triggers.length > MAX_VISIBLE_TRIGGER_CARDS ? 'triggers-list-scroll' : ''}`}>
                      {triggers.map((item, index) => {
                        const sourceLabel = (item.source || 'database').trim() || 'database';
                        const triggerPath = `/triggers/${encodeId(item.repoSlug)}`;
                        const branchField = formatTriggerBranchField(item.trigger);
                        return (
                          <li key={`${item.repoSlug}-${index}`} className="triggers-pipeline-item">
                            <button
                              type="button"
                              className="triggers-pipeline-link"
                              title={`Open trigger ${item.repoSlug}`}
                              onClick={() => navigate(triggerPath)}
                            >
                              <span className="triggers-pipeline-name">{item.repoSlug}</span>
                              <dl className="triggers-detail-grid triggers-pipeline-details">
                                <dt className="triggers-detail-label">Event:</dt>
                                <dd className="triggers-detail-value">{formatTriggerEvent(item.trigger.on)}</dd>
                                <dt className="triggers-detail-label">{branchField.label}</dt>
                                <dd className="triggers-detail-value">{branchField.value}</dd>
                                <dt className="triggers-detail-label">Scope:</dt>
                                <dd className="triggers-detail-value">{formatTriggerScope(item.trigger)}</dd>
                                <dt className="triggers-detail-label">Source:</dt>
                                <dd className="triggers-detail-value">{sourceLabel}</dd>
                              </dl>
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No trigger manifests reference this pipeline.</p>
                  )}
                </div>
              </div>

              <div className="glass-card overflow-hidden">
                <div className="p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Included Dependencies</h3>
                </div>
                <div className="p-4">
                  {detail.includedDependencies.length ? (
                    <ul
                      className={`triggers-pipeline-list ${
                        detail.includedDependencies.length > MAX_VISIBLE_TRIGGER_CARDS ? 'triggers-list-scroll' : ''
                      }`}
                    >
                      {Array.from(new Set(detail.includedDependencies))
                        .sort((a, b) => a.localeCompare(b))
                        .map(dep => {
                          const trimmed = dep.trim();
                          const isPipeline = trimmed.startsWith('pipeline:');
                          const isStep = trimmed.startsWith('step:');
                          const identifier = isPipeline
                            ? trimmed.slice('pipeline:'.length).trim()
                            : isStep
                              ? trimmed.slice('step:'.length).trim()
                              : trimmed;

                          const typeLabel = isPipeline ? 'Pipeline' : isStep ? 'Step' : 'Include';
                          const actionLabel = isPipeline ? 'Open' : 'Copy';

                          return (
                            <li key={trimmed} className="triggers-pipeline-item">
                              <button
                                type="button"
                                className="triggers-pipeline-link"
                                title={isPipeline ? `Open ${identifier}` : `Copy ${identifier}`}
                                onClick={async () => {
                                  if (isPipeline && identifier) {
                                    handleSelect(identifier);
                                    return;
                                  }
                                  try {
                                    await navigator.clipboard.writeText(identifier || trimmed);
                                    addToast('Copied dependency reference.', 'success');
                                  } catch (error) {
                                    console.error('Failed to copy dependency reference', error);
                                    addToast('Unable to copy dependency reference.', 'error');
                                  }
                                }}
                              >
                                <span className="triggers-pipeline-name">{identifier || trimmed}</span>
                                <dl className="triggers-detail-grid triggers-pipeline-details">
                                  <dt className="triggers-detail-label">Type:</dt>
                                  <dd className="triggers-detail-value">{typeLabel}</dd>
                                  <dt className="triggers-detail-label">Action:</dt>
                                  <dd className="triggers-detail-value">{actionLabel}</dd>
                                </dl>
                              </button>
                            </li>
                          );
                        })}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No includes detected for this pipeline.</p>
                  )}
                </div>
              </div>

              <div className="glass-card overflow-hidden" id="pipeline-recent-runs">
                <div className="p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Recent Pipeline Runs</h3>
                </div>
                <div className="p-4">
                  {runsLoading ? (
                    <p className="text-sm text-[var(--text-secondary)]">Loading recent runs…</p>
                  ) : runsError ? (
                    <p className="text-sm text-red-500">Failed to load runs: {runsError}</p>
                  ) : recentRuns.length ? (
                    <ul className="triggers-pipeline-list">
                      {recentRuns.map(run => {
                        const runId = run.run_id || '';
                        const shortRunId = runId ? runId.slice(0, 8) : '—';
                        const triggerId = typeof (run as any).trigger_event_id === 'string' ? (run as any).trigger_event_id : '';
                        const shortTriggerId = triggerId ? triggerId.slice(0, 8) : '—';
                        const runPath = runId ? `/pipelineruns/recent?run_id=${encodeURIComponent(runId)}` : '/pipelineruns/recent';
                        return (
                          <li key={runId || `${run.pipeline_name}-${run.started_at}`} className="triggers-pipeline-item">
                            <button
                              type="button"
                              className="triggers-pipeline-link"
                              title={runId ? `Open run ${runId}` : 'Open pipeline runs'}
                              onClick={() => navigate(runPath)}
                            >
                              <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0">
                                  <span className="triggers-pipeline-name">{detail.name || detail.id}</span>
                                  <p className="text-xs text-[var(--text-secondary)] mt-0.5">{formatRelativeTime(run.started_at)}</p>
                                </div>
                                <span className={`runner-pill ${statusClass(run.status)}`}>{statusLabel(run.status)}</span>
                              </div>
                              <dl className="triggers-detail-grid triggers-pipeline-details">
                                <dt className="triggers-detail-label">Branch:</dt>
                                <dd className="triggers-detail-value">{formatRef(run.git_ref)}</dd>
                                <dt className="triggers-detail-label">Run ID:</dt>
                                <dd className="triggers-detail-value">{shortRunId}</dd>
                                <dt className="triggers-detail-label">Trigger:</dt>
                                <dd className="triggers-detail-value">{shortTriggerId}</dd>
                              </dl>
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No recent runs for this pipeline.</p>
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
    <div data-page="pipelines" className="active h-full flex flex-col">
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
                aria-label="Search pipelines"
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
                id="pipelines-search"
                type="search"
                placeholder="Search pipelines"
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
              id="pipelines-new-btn"
              type="button"
              className="pipelines-icon-only"
              aria-label="Create new pipeline"
              title="New Pipeline"
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
        {!selectedId ? renderList() : detailLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading pipeline…</div>
        ) : detailError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load pipeline: {detailError}</div>
        ) : (
          renderDetail()
        )}
      </div>

      {formModal && (
        <div className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50">
          <div className="pipelines-modal-card max-w-md w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">
                  {formModal.mode === 'create' ? 'New pipeline' : 'Clone pipeline'}
                </p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">
                  {formModal.mode === 'create' ? 'Create pipeline' : 'Clone pipeline'}
                </h3>
              </div>
              <button className="glass-button-ghost" onClick={() => setFormModal(null)}>Close</button>
            </header>
            <div className="pipelines-modal-body space-y-4">
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Pipeline Path</label>
                <input
                  type="text"
                  className="pipelines-input w-full mt-1"
                  placeholder="team/service"
                  value={formModal.path}
                  onChange={event => setFormModal(prev => prev ? { ...prev, path: event.target.value } : prev)}
                />
                <p className="text-xs text-[var(--text-secondary)] mt-1">Optional folder-style path. Leave blank for root.</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)]">Pipeline Name</label>
                <input
                  type="text"
                  className="pipelines-input w-full mt-1"
                  placeholder="build-and-test"
                  value={formModal.name}
                  onChange={event => setFormModal(prev => prev ? { ...prev, name: event.target.value } : prev)}
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
        <div className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50">
          <div className="pipelines-modal-card max-w-md w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete pipeline</p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Remove {deleteModal.pipelineName}?</h3>
              </div>
              <button className="glass-button-ghost" onClick={() => setDeleteModal(null)} disabled={deleteModal.pending}>Close</button>
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

export default PipelinesPage;
