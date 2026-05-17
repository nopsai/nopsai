import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';
import {
  PIPELINE_DRAFTS_CHANGED_EVENT,
  deletePipelineDraft,
  getPipelineDraftStorageKey,
  loadPipelineDrafts,
  type PipelineDraft,
  upsertPipelineDraft,
} from '../lib/pipelineDrafts';
import { fetchResourceGroupPaths, insertGroupPath } from '../lib/resourceGroups';
import { applyEnterIndent, findParentBlock, validatePipelineYamlStrict } from '../lib/lab';
import { findLineNumberForKey, normalizeLineNumber, parseYamlWithLocation } from '../lib/yamlValidation';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';
import ResourceAccessCard from '../components/ResourceAccessCard';
import { StepsGraph, type PipelineDefinition as RunPipelineDefinition, type StepDetail as RunStepDetail, type TaskDefinition as RunTaskDefinition, type TaskDetail as RunTaskDetail } from './PipelineRuns';

const MAX_RECENT_RUNS = 5;
const MAX_VISIBLE_TRIGGER_CARDS = 5;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

const PIPELINE_DIRECTIVES = [
  'name',
  'version',
  'description',
  'container_image',
  'working_directory',
  'variables',
  'steps',
  'timeout',
  'llm_profile',
  'llm_output_sharing',
  'llm_content_sharing',
  'llm_content_include',
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
  'llm_profile',
  'llm_output_sharing',
];

const TASK_DIRECTIVES = [
  'name',
  'goal',
  'script',
  'depends_on',
  'ignore_failure',
  'llm_profile',
  'llm_output_sharing',
  'variables',
];

const PIPELINE_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

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

type PipelinesPageProps = {
  draftScope: string;
  canDeletePipelines: boolean;
};

function PipelinesPage({ draftScope, canDeletePipelines }: PipelinesPageProps) {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverPipelines, setServerPipelines] = useState<PipelineListItem[]>([]);
  const [draftPipelines, setDraftPipelines] = useState<PipelineDraft[]>([]);
  const [listLoading, setListLoading] = useState<boolean>(true);
  const [listError, setListError] = useState<string | null>(null);
  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selectedIdRef = useRef<string | null>(null);
  const [folderCreateAllowed, setFolderCreateAllowed] = useState(false);
  const [selectedUpdateAllowed, setSelectedUpdateAllowed] = useState(false);

  const [detail, setDetail] = useState<PipelineDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [recentRuns, setRecentRuns] = useState<PipelineRun[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string | null>(null);
  const [triggers, setTriggers] = useState<PipelineTrigger[]>([]);
  const [triggersLoading, setTriggersLoading] = useState(false);
  const [triggersError, setTriggersError] = useState<string | null>(null);
  const [selectedGraphStep, setSelectedGraphStep] = useState<string | null>(null);

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
    llmProfiles: string[];
    reusableSteps: string[];
    secretScopes: Array<{ scope: string; items: string[] }>;
    variableScopes: Array<{ scope: string; items: string[] }>;
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], llmProfiles: [], reusableSteps: [], secretScopes: [], variableScopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<null | {
    title: string;
    items: string[];
    activeIndex: number;
    replaceStart: number;
    replaceEnd: number;
    appendColon: boolean;
    groupedSections?: Array<{ label: string; items: string[]; totalCount: number }>;
  }>(null);

  const validatePipelineYaml = useCallback((rawYaml: string): ValidationResult => {
    const trimmed = rawYaml.trim();
    if (!trimmed) {
      return { errors: [{ message: 'Pipeline definition cannot be empty.', line: 1 }] };
    }

    const { parsed, error: parseError } = parseYamlWithLocation(rawYaml);
    if (parseError) {
      return { errors: [parseError] };
    }

    const errors: ValidationError[] = [];
    const strict = validatePipelineYamlStrict(rawYaml);
    strict.errors.forEach(err => {
      errors.push({
        message: err.message,
        line: normalizeLineNumber(err.line),
      });
    });

    const pipelineObject = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : null;
    if (pipelineObject) {
      const steps = Array.isArray(pipelineObject.steps) ? (pipelineObject.steps as Array<Record<string, unknown>>) : [];
      if (steps.length > 0) {
        const containerImage = typeof pipelineObject.container_image === 'string' ? pipelineObject.container_image.trim() : '';
        const firstStep = steps[0] as Record<string, unknown> | undefined;
        const firstStepImage = typeof firstStep?.image === 'string' ? (firstStep.image as string).trim() : '';
        if (!containerImage && !firstStepImage) {
          errors.push({
            message: "'container_image' is required when steps do not specify their own image.",
            line: findLineNumberForKey(rawYaml, 'container_image') ?? findLineNumberForKey(rawYaml, 'steps') ?? 1,
          });
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

  const graphData = useMemo<{
    steps: RunStepDetail[];
    definition?: RunPipelineDefinition;
    error: string | null;
  }>(() => {
    const source = isEditing ? editorValue : detail?.rawYaml;
    const base = { steps: [] as RunStepDetail[], definition: undefined as RunPipelineDefinition | undefined, error: null as string | null };
    if (!source) return base;

    const normalizeStringArray = (value: unknown) =>
      Array.isArray(value) ? value.map(v => (typeof v === 'string' ? v.trim() : '')).filter(Boolean) : [];
    const normalizeVariables = (value: unknown) => {
      if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
      const entries: Record<string, string> = {};
      Object.entries(value as Record<string, unknown>).forEach(([key, val]) => {
        if (typeof val === 'string') entries[key] = val;
      });
      return Object.keys(entries).length ? entries : undefined;
    };

    type NormalizedStep = {
      name: string;
      description?: string;
      depends_on: string[];
      include?: string;
      sync?: boolean;
      image?: string;
      secrets?: string[];
      volumes?: string[];
      variables?: Record<string, string>;
      ignore_failure?: boolean;
      llm_output_sharing?: boolean;
      goal?: string;
      script?: string;
      tasks: RunTaskDefinition[];
    };

    try {
      const parsed = yaml.load(source) as any;
      const rawSteps = Array.isArray(parsed?.steps) ? parsed.steps : [];
      const normalizedSteps: NormalizedStep[] = rawSteps
        .map((step: any) => {
          const name = typeof step?.name === 'string' ? step.name.trim() : '';
          if (!name) return null;

          const taskDefs: RunTaskDefinition[] = Array.isArray(step?.tasks)
            ? step.tasks
                .map((task: any) => {
                  const taskName = typeof task?.name === 'string' ? task.name.trim() : '';
                  if (!taskName) return null;
                  return {
                    name: taskName,
                    goal: typeof task?.goal === 'string' ? task.goal : undefined,
                    script: typeof task?.script === 'string' ? task.script : undefined,
                    depends_on: normalizeStringArray(task?.depends_on),
                    ignore_failure: typeof task?.ignore_failure === 'boolean' ? task.ignore_failure : undefined,
                    variables: normalizeVariables(task?.variables),
                  } as RunTaskDefinition;
                })
                .filter(Boolean)
            : [];

          return {
            name,
            description: typeof step?.description === 'string' ? step.description : undefined,
            depends_on: normalizeStringArray(step?.depends_on),
            include: typeof step?.include === 'string' ? step.include : undefined,
            sync: typeof step?.sync === 'boolean' ? step.sync : undefined,
            image: typeof step?.image === 'string' ? step.image : undefined,
            secrets: normalizeStringArray(step?.secrets),
            volumes: normalizeStringArray(step?.volumes),
            variables: normalizeVariables(step?.variables),
            ignore_failure: typeof step?.ignore_failure === 'boolean' ? step.ignore_failure : undefined,
            llm_output_sharing: typeof step?.llm_output_sharing === 'boolean' ? step.llm_output_sharing : undefined,
            goal: typeof step?.goal === 'string' ? step.goal : undefined,
            script: typeof step?.script === 'string' ? step.script : undefined,
            tasks: taskDefs,
          };
        })
        .filter(Boolean) as NormalizedStep[];

      const definition: RunPipelineDefinition | undefined =
        normalizedSteps.length > 0
          ? {
              name: typeof parsed?.name === 'string' ? parsed.name : undefined,
              description: typeof parsed?.description === 'string' ? parsed.description : undefined,
              version: typeof parsed?.version === 'string' ? parsed.version : undefined,
              steps: normalizedSteps.map(step => ({
                name: step.name,
                description: step.description,
                depends_on: step.depends_on,
                tasks: step.tasks,
                goal: step.goal,
                script: step.script,
              })),
            }
          : undefined;

      const stepDetails: RunStepDetail[] = normalizedSteps.map(step => {
        const taskDetails: RunTaskDetail[] = step.tasks.map((task, index) => ({
          task_id: `def-${step.name}-${task.name || index}`,
          step_name: step.name,
          task_name: task.name,
          status: 'pending',
          exit_code: null,
          started_at: undefined,
          finished_at: undefined,
          task_index: index,
        }));

        return {
          name: step.name,
          status: 'success',
          depends_on: step.depends_on,
          tasks: taskDetails,
          configuration: {
            include: step.include,
            sync: step.sync,
            image: step.image,
            secrets: step.secrets,
            volumes: step.volumes,
            variables: step.variables,
            ignore_failure: step.ignore_failure,
            llm_output_sharing: step.llm_output_sharing,
            goal: step.goal,
            script: step.script,
            tasks: step.tasks,
          },
        };
      });

      return { steps: stepDetails, definition, error: null };
    } catch (error: any) {
      return { steps: [], definition: undefined, error: error instanceof Error ? error.message : 'Unable to parse YAML' };
    }
  }, [detail?.rawYaml, editorValue, isEditing]);

  useEffect(() => {
    if (selectedGraphStep && !graphData.steps.some(step => step.name === selectedGraphStep)) {
      setSelectedGraphStep(null);
    }
  }, [graphData.steps, selectedGraphStep]);

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

  const handleAutoIndentEnter = useCallback(() => {
    const textarea = editorRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart ?? 0;
    const end = textarea.selectionEnd ?? start;
    const { nextValue, nextCursor } = applyEnterIndent(editorValue, start, end);
    setEditorValue(nextValue);
    requestAnimationFrame(() => {
      const el = editorRef.current;
      if (!el) return;
      el.focus();
      el.selectionStart = nextCursor;
      el.selectionEnd = nextCursor;
      syncEditorOverlays(el);
    });
  }, [editorValue, syncEditorOverlays]);

  const openEditorSuggestion = useCallback(
    (cursor: number, opts?: { text?: string; force?: boolean }) => {
      const text = typeof opts?.text === 'string' ? opts.text : editorValue;
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

      const beforeLineText = text.slice(0, lineStart);
      const ancestorKey = findParentBlock(beforeLineText, ['secrets', 'variables', 'depends_on', 'tasks', 'steps'], currentIndent) || '';
      const containerBlock = findParentBlock(beforeLineText, ['tasks', 'steps'], currentIndent) || '';

      const includeValueContext =
        currentKey === 'include' ||
        /^\s*include\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());
      const llmProfileValueContext =
        currentKey === 'llm_profile' ||
        /^\s*llm_profile\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());

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
      let groupedSections: Array<{ label: string; items: string[]; totalCount: number }> | undefined;

      if (includeValueContext) {
        title = 'Reusable steps';
        pool = autocompleteMeta.reusableSteps;
      } else if (llmProfileValueContext) {
        title = 'LLM profiles';
        pool = autocompleteMeta.llmProfiles;
      } else if (ancestorKey === 'secrets') {
        title = 'Secrets';
        const base = autocompleteMeta.secretScopes.length
          ? autocompleteMeta.secretScopes
          : [{ scope: '', items: autocompleteMeta.secrets }];
        let remaining = 50;
        groupedSections = base
          .map(entry => {
            const filteredItems = entry.items.filter(item => item.toLowerCase().startsWith(prefix.toLowerCase()));
            if (!filteredItems.length) return null;
            const slice = filteredItems.slice(0, remaining);
            remaining -= slice.length;
            return {
              label: entry.scope ? `/${entry.scope}` : 'Default scope',
              items: slice,
              totalCount: filteredItems.length,
            };
          })
          .filter(Boolean) as Array<{ label: string; items: string[]; totalCount: number }>;
        pool = groupedSections.flatMap(section => section.items);
      } else if (ancestorKey === 'variables') {
        title = 'Variables';
        const base = autocompleteMeta.variableScopes.length
          ? autocompleteMeta.variableScopes
          : [{ scope: '', items: autocompleteMeta.variables }];
        let remaining = 50;
        groupedSections = base
          .map(entry => {
            const filteredItems = entry.items.filter(item => item.toLowerCase().startsWith(prefix.toLowerCase()));
            if (!filteredItems.length) return null;
            const slice = filteredItems.slice(0, remaining);
            remaining -= slice.length;
            return {
              label: entry.scope ? `/${entry.scope}` : 'Default scope',
              items: slice,
              totalCount: filteredItems.length,
            };
          })
          .filter(Boolean) as Array<{ label: string; items: string[]; totalCount: number }>;
        pool = groupedSections.flatMap(section => section.items);
      } else if (ancestorKey === 'depends_on') {
        title = 'Step dependencies';
        pool = resolveStepNames();
      } else {
        appendColon = true;
        if (containerBlock === 'tasks') {
          title = 'Task keys';
          pool = TASK_DIRECTIVES;
        } else if (containerBlock === 'steps') {
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

      const hasContext =
        includeValueContext || llmProfileValueContext || ancestorKey === 'secrets' || ancestorKey === 'variables' || ancestorKey === 'depends_on';
      const isRootLine = !containerBlock && currentIndent === 0 && !currentKey;
      const shouldShow = opts?.force || hasContext || filtered.length > 0 || containerBlock === 'tasks' || containerBlock === 'steps';

      if (!shouldShow || (!opts?.force && isRootLine && !prefix)) {
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
        groupedSections,
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

        const normalizeLLMProfiles = (payload: unknown): string[] => {
          const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : null;
          const profiles = record && Array.isArray(record.profiles) ? record.profiles : payload;
          return normalize(profiles);
        };

        const normalizeScopeLabel = (entry: unknown) => {
          const normalizeRawScope = (raw: string) => {
            const normalized = raw.trim().replace(/^\/+|\/+$/g, '');
            return normalized.toLowerCase() === 'default' ? '' : normalized;
          };
          if (entry == null) return '';
          if (typeof entry === 'string') return normalizeRawScope(entry);
          if (typeof entry === 'object') {
            const record = entry as Record<string, unknown>;
            const raw = record.scope ?? record.name ?? record.value;
            if (typeof raw === 'string') return normalizeRawScope(raw);
          }
          return '';
        };

        const buildScopeList = (secretsPayload: unknown, variablesPayload: unknown): string[] => {
          const scopes = new Set<string>();
          scopes.add('');
          if (Array.isArray(secretsPayload)) {
            secretsPayload.forEach(entry => {
              const label = normalizeScopeLabel(entry);
              if (label !== null) scopes.add(label);
            });
          }
          if (Array.isArray(variablesPayload)) {
            variablesPayload.forEach(entry => {
              const label = normalizeScopeLabel(entry);
              if (label !== null) scopes.add(label);
            });
          }
          return Array.from(scopes)
            .map(scope => scope.replace(/^\/+|\/+$/g, ''))
            .sort((a, b) => a.localeCompare(b));
        };

        const fetchScopedList = async (path: string, scope: string) => {
          const suffix = scope ? `?scope=${encodeURIComponent(scope)}` : '';
          const response = await fetch(buildApiUrl(`${path}${suffix}`));
          if (!response.ok) return [];
          const payload = await response.json();
          return normalize(payload);
        };

        const promise = (async () => {
          const [secretsResp, varsResp, stepsResp, secretScopesResp, variableScopesResp, llmProfilesResp] = await Promise.all([
            fetch(buildApiUrl('/v1/secrets')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/variables')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/steps')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/secrets/scopes')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/variables/scopes')).then(r => (r.ok ? r.json() : [])),
            fetch(buildApiUrl('/v1/system/llm-profiles')).then(r => (r.ok ? r.json() : null)),
          ]);

          const scopeList = buildScopeList(secretScopesResp, variableScopesResp);

          const [secretScopes, variableScopes] = await Promise.all([
            Promise.all(
              scopeList.map(async scope => ({
                scope,
                items: await fetchScopedList('/v1/secrets', scope),
              }))
            ),
            Promise.all(
              scopeList.map(async scope => ({
                scope,
                items: await fetchScopedList('/v1/variables', scope),
              }))
            ),
          ]);

          setAutocompleteMeta({
            secrets: normalize(secretsResp),
            variables: normalize(varsResp),
            llmProfiles: normalizeLLMProfiles(llmProfilesResp),
            reusableSteps: normalize(stepsResp),
            secretScopes,
            variableScopes,
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

  const buildPermissionProbeIdentifier = (folder: string) => {
    const cleaned = folder.trim().replace(/^\/+|\/+$/g, '');
    return cleaned ? `${cleaned}/${PIPELINE_PERMISSION_PROBE_NAME}` : PIPELINE_PERMISSION_PROBE_NAME;
  };

  const checkPipelinePermission = useCallback(async (action: string, resourceID: string) => {
    const params = new URLSearchParams({
      action,
      resource_type: 'pipeline',
      resource_id: resourceID,
    });
    const response = await fetch(buildApiUrl(`/v1/access/effective-permissions?${params.toString()}`));
    if (!response.ok) return false;
    const payload = await response.json();
    return Boolean(payload?.allowed);
  }, []);

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

  const permissionFolder = selectedId ? splitIdentifier(selectedId).path : activeFolder;

  useEffect(() => {
    let cancelled = false;
    setFolderCreateAllowed(false);
    void checkPipelinePermission('pipeline.create', buildPermissionProbeIdentifier(permissionFolder))
      .then(allowed => {
        if (!cancelled) setFolderCreateAllowed(allowed);
      })
      .catch(() => {
        if (!cancelled) setFolderCreateAllowed(false);
      });

    return () => {
      cancelled = true;
    };
  }, [checkPipelinePermission, permissionFolder]);

  useEffect(() => {
    let cancelled = false;
    if (!selectedId) {
      setSelectedUpdateAllowed(false);
      return () => {
        cancelled = true;
      };
    }

    setSelectedUpdateAllowed(false);
    void checkPipelinePermission('pipeline.update', selectedId)
      .then(allowed => {
        if (!cancelled) setSelectedUpdateAllowed(allowed);
      })
      .catch(() => {
        if (!cancelled) setSelectedUpdateAllowed(false);
      });

    return () => {
      cancelled = true;
    };
  }, [checkPipelinePermission, selectedId]);

  const canCreatePipelineHere = folderCreateAllowed;
  const canUpdateSelectedPipeline = selectedUpdateAllowed;
  const canUsePipelineDrafts = canCreatePipelineHere || canUpdateSelectedPipeline;

  useEffect(() => {
    if (!canUsePipelineDrafts || !draftScope) {
      setDraftPipelines([]);
      return;
    }
    setDraftPipelines(loadPipelineDrafts(draftScope));
  }, [canUsePipelineDrafts, draftScope]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (!canUsePipelineDrafts || !draftScope) return;
    const storageKey = getPipelineDraftStorageKey(draftScope);
    const refreshDrafts = () => setDraftPipelines(loadPipelineDrafts(draftScope));
    const onStorage = (event: StorageEvent) => {
      if (event.key !== storageKey) return;
      refreshDrafts();
    };
    window.addEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, refreshDrafts);
    window.addEventListener('storage', onStorage);
    return () => {
      window.removeEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, refreshDrafts);
      window.removeEventListener('storage', onStorage);
    };
  }, [canUsePipelineDrafts, draftScope]);

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
    let cancelled = false;
    void fetchResourceGroupPaths()
      .then(paths => {
        if (!cancelled) setResourceGroupPaths(paths);
      })
      .catch(error => {
        console.warn('Failed to load groups for pipeline tree', error);
        if (!cancelled) setResourceGroupPaths([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

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
    if (!canUsePipelineDrafts || !draftScope) return;
    if (normalizeSource(detail.source) !== 'draft') return;
    const draftId = detail.id;
    const handle = window.setTimeout(() => {
      setDraftPipelines(upsertPipelineDraft({ id: draftId, yaml: editorValue }, draftScope));
    }, 800);
    return () => window.clearTimeout(handle);
  }, [canUsePipelineDrafts, detail, draftScope, editorValue, isEditing]);

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
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], pipelineIds: [] }));
    });
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
  }, [pipelines, resourceGroupPaths]);

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
    const detailSource = normalizeSource(detail.source);
    const canPersistPipeline = detailSource === 'draft' ? canCreatePipelineHere : canUpdateSelectedPipeline;
    if (!canPersistPipeline) {
      addToast('You have read-only access to pipelines.', 'info');
      return;
    }
    if (detailSource === 'git') {
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
        setDraftPipelines(deletePipelineDraft(detail.id, draftScope));
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

  const openCreateModal = () => {
    if (!canCreatePipelineHere) {
      addToast('You have read-only access to pipelines.', 'info');
      return;
    }
    setFormModal({ mode: 'create', path: permissionFolder, name: '', pending: false });
  };
  const openCloneModal = () => {
    if (!canCreatePipelineHere) {
      addToast('You have read-only access to pipelines.', 'info');
      return;
    }
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
    if (!canCreatePipelineHere || !draftScope) {
      setFormModal(prev => prev ? { ...prev, error: 'You have read-only access to pipelines.' } : prev);
      return;
    }
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
    const allowed = await checkPipelinePermission('pipeline.create', identifier);
    if (!allowed) {
      setFormModal(prev => prev ? { ...prev, error: 'You do not have permission to create pipelines in this path.' } : prev);
      return;
    }
    setFormModal(prev => prev ? { ...prev, pending: true, error: undefined } : prev);
    try {
      const yamlBody = formModal.mode === 'clone' && formModal.baseYaml
        ? updateYamlName(formModal.baseYaml, formModal.name.trim())
        : buildTemplateYaml(formModal.name.trim());
      setDraftPipelines(upsertPipelineDraft({ id: identifier, yaml: yamlBody }, draftScope));
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
      const normalizedSource = normalizeSource(source);
      if (normalizedSource === 'git') {
        throw new Error('This pipeline is managed via Git. Clone it to customize instead of deleting.');
      }
      if (normalizedSource === 'draft') {
        if (!canUsePipelineDrafts || !draftScope) {
          throw new Error('You have read-only access to pipelines.');
        }
        setDraftPipelines(deletePipelineDraft(deleteModal.pipelineId, draftScope));
      } else {
        if (!canDeletePipelines) {
          throw new Error('You do not have permission to delete pipelines.');
        }
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
    const canDeleteThisPipeline = source === 'draft' ? canUsePipelineDrafts : canDeletePipelines && source !== 'git';
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
            {canDeleteThisPipeline ? (
              <button
                type="button"
                className="pipelines-delete-button"
                title={source === 'draft' ? 'Discard draft' : 'Delete pipeline'}
                onClick={event => {
                  event.stopPropagation();
                  setDeleteModal({ pipelineId: pipeline.id, pipelineName: name || pipeline.id, pending: false });
                }}
                aria-label={source === 'draft' ? 'Discard draft pipeline' : 'Delete pipeline'}
              >
                <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                  <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                  <path d="M4 7h16" />
                </svg>
              </button>
            ) : null}
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
            <span className="pipeline-card-meta-label">Sub groups:</span>
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
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreatePipelineHere ? 'Create a new pipeline or adjust your filters.' : 'Adjust your filters or check your access.'}
                </p>
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
                        {source !== 'draft' ? (
                          <ResourceAccessCard resourceType="pipeline" resourceID={detail.id} label="pipeline" />
                        ) : null}
                        {!canUpdateSelectedPipeline && !canCreatePipelineHere ? null : isGitSource ? (
                          canCreatePipelineHere ? (
                            <button className="glass-button-primary" onClick={openCloneModal}>
                              Clone
                            </button>
                          ) : null
                        ) : (
                          <>
                            {canUpdateSelectedPipeline ? (
                              <button className="glass-button-primary" onClick={() => setIsEditing(true)}>
                                Edit
                              </button>
                            ) : null}
                            {canCreatePipelineHere ? (
                              <button className="glass-button-subtle" onClick={openCloneModal}>
                                Clone
                              </button>
                            ) : null}
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
                            if (normalizeSource(detail.source) === 'draft' && draftScope) {
                              setDraftPipelines(upsertPipelineDraft({ id: detail.id, yaml: resetYaml }, draftScope));
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
                          onChange={event => {
                            const next = event.target.value;
                            setEditorValue(next);
                            const cursor = event.target.selectionStart || 0;
                            openEditorSuggestion(cursor, { text: next });
                          }}
                          onClick={event => {
                            const cursor = event.currentTarget.selectionStart || 0;
                            openEditorSuggestion(cursor);
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

                            if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
                              event.preventDefault();
                              handleAutoIndentEnter();
                              return;
                            }

                            if (editorSuggestion && event.key === 'Escape') {
                              event.preventDefault();
                              setEditorSuggestion(null);
                            }
                          }}
                          spellCheck={false}
                        ></textarea>
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
                      {editorSuggestion && (
                        <div
                          className="pipeline-suggestion-overlay"
                          style={{
                            width: 320,
                            maxWidth: 'calc(100% - 32px)',
                            right: 16,
                            bottom: 16,
                            top: 'auto',
                            left: 'auto',
                          }}
                        >
                          <div className="scope-suggestion-panel">
                            <div className="scope-suggestion-heading">
                              <p className="scope-suggestion-kicker">Autocomplete</p>
                              <p className="scope-suggestion-title">{editorSuggestion.title}</p>
                              <p className="scope-suggestion-subtitle">
                                Ctrl+Space • Enter to insert • Esc to close
                                {autocompleteMeta.loading ? ' • Loading…' : ''}
                              </p>
                            </div>
                            <div className="scope-suggestion-body">
                              {editorSuggestion.items.length ? (
                                <div className="scope-suggestion-list">
                                  {editorSuggestion.groupedSections && editorSuggestion.groupedSections.length > 0 ? (
                                    (() => {
                                      let runningIndex = 0;
                                      return editorSuggestion.groupedSections.map(section => {
                                        const startIndex = runningIndex;
                                        const pills = section.items.map((item, idx) => {
                                          const globalIndex = startIndex + idx;
                                          const isActive = editorSuggestion.activeIndex === globalIndex;
                                          runningIndex += 1;
                                          return (
                                            <button
                                              key={`${section.label}-${item}-${idx}`}
                                              type="button"
                                              className={`scope-suggestion-pill scope-suggestion-pill--action ${isActive ? 'scope-suggestion-pill--active' : ''}`}
                                              onClick={() => applyEditorSuggestion(item)}
                                            >
                                              {item}
                                            </button>
                                          );
                                        });
                                        const remaining = Math.max(0, section.totalCount - section.items.length);
                                        const hasActive =
                                          editorSuggestion.activeIndex >= startIndex &&
                                          editorSuggestion.activeIndex < startIndex + section.items.length;
                                        return (
                                          <article
                                            key={`section-${section.label}`}
                                            className={`scope-suggestion-item ${hasActive ? 'scope-suggestion-item--active' : ''}`}
                                          >
                                            <div className="scope-suggestion-scope">
                                              <span className="scope-suggestion-scope-label">{section.label}</span>
                                              <span className="scope-suggestion-scope-count">
                                                {section.totalCount} {section.totalCount === 1 ? 'item' : 'items'}
                                              </span>
                                            </div>
                                            <div className="scope-suggestion-variables">
                                              {pills}
                                              {remaining > 0 && (
                                                <span className="scope-suggestion-pill scope-suggestion-pill--more">
                                                  +{remaining} more
                                                </span>
                                              )}
                                            </div>
                                          </article>
                                        );
                                      });
                                    })()
                                  ) : (
                                    editorSuggestion.items.map((item, idx) => (
                                      <div
                                        key={`sg-${item}-${idx}`}
                                        className={`scope-suggestion-item ${idx === editorSuggestion.activeIndex ? 'scope-suggestion-item--active' : ''}`}
                                      >
                                        <button
                                          type="button"
                                          className="scope-suggestion-pill scope-suggestion-pill--action"
                                          onClick={() => applyEditorSuggestion(item)}
                                        >
                                          {item}
                                        </button>
                                      </div>
                                    ))
                                  )}
                                </div>
                              ) : (
                                <p className="scope-suggestion-empty">No suggestions available.</p>
                              )}
                            </div>
                          </div>
                        </div>
                      )}
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
                  {graphData.error ? (
                    <p className="text-sm text-red-500">Unable to render graph: {graphData.error}</p>
                  ) : !graphData.steps.length ? (
                    <p className="text-sm text-[var(--text-secondary)]">No steps defined in this pipeline.</p>
                  ) : (
                    <div className="rounded-2xl border border-[var(--border-primary)] bg-white dark:bg-slate-950 shadow-[0_16px_44px_rgba(15,23,42,0.07)] p-2">
                      <StepsGraph
                        steps={graphData.steps}
                        selectedStep={selectedGraphStep}
                        onSelectStep={setSelectedGraphStep}
                        childRuns={[]}
                        pipelineDefinition={graphData.definition}
                        statusVariant="dot"
                        stepStatusColorOverride="#10b981"
                        taskStatusColorOverride="#60a5fa"
                        hideStatusLegend
                      />
                    </div>
                  )}
                </div>
              </div>
            </div>
            <div className="space-y-4">
              <div className="glass-card overflow-hidden">
                <div className="p-4 border-b border-[var(--border-primary)]" style={{ marginTop: '9px' }}>
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
                type="text"
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
            {canCreatePipelineHere ? (
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
            ) : null}
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
        <div
          id={formModal.mode === 'create' ? 'pipelines-new-modal' : 'pipelines-clone-modal'}
          className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show"
        >
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
                <p className="text-xs text-[var(--text-secondary)] mt-1">Optional group path. Leave blank for root.</p>
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
        <div id="pipelines-delete-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
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
