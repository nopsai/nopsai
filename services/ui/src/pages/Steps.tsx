import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import * as yaml from 'js-yaml';
import {
  STEP_DRAFTS_CHANGED_EVENT,
  deleteStepDraft,
  getStepDraftStorageKey,
  loadStepDrafts,
  upsertStepDraft,
} from '../lib/stepDrafts';
import { applyEnterIndent, findParentBlock } from '../lib/lab';
import { copyTextToClipboard } from '../lib/clipboard';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { fetchEditorAutocompleteMetadata } from '../features/editor/autocomplete';
import { ResourceCollectionToolbar } from '../features/editor/ResourceCollectionToolbar';
import { ResourceWorkflowModals } from '../features/editor/ResourceWorkflowModals';
import { useDraftCollection } from '../features/editor/useDraftCollection';
import { useYamlResourceMutations } from '../features/editor/useYamlResourceMutations';
import {
  checkStepPermission,
  deleteStep,
  fetchStepList,
  fetchStepUsage,
  fetchStepYaml,
  saveStepYaml,
  type StepListItem,
  type StepUsageItem,
} from '../features/steps/api';
import {
  STEP_DIRECTIVES,
  STEP_NAME_PATTERN,
  TASK_DIRECTIVES,
  filterVisibleStepList,
  normalizeRootPath,
  normalizeSource,
  parseStepYaml,
  splitIdentifier,
  validateStepYaml,
  type StepDetail,
} from '../features/steps/model';
import { useStepPermissions } from '../features/steps/useStepPermissions';
import { StepCollectionList } from '../features/steps/StepCollectionList';
import { StepDetailView } from '../features/steps/StepDetailView';
import {
  GLOBAL_RESOURCE_TEAM_PATH,
  compareResourceTreeNodes,
  fetchResourceTeamPaths,
  insertTeamPath,
} from '../lib/resourceTeams';
import { TEAM_ROUTE_SEGMENT, decodeTeamRouteSegments, teamScopedRoute } from '../lib/teamRoutes';

const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

type TreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TreeNode[];
  stepIds: string[];
};

type StepsPageProps = {
  draftScope: string;
  canDeleteSteps: boolean;
};

function buildStepTemplateYaml(name: string) {
  return [
    `name: ${name}`,
    'description: Describe what this step does.',
    'script: |',
    `  echo "Implement ${name}"`,
    '',
  ].join('\n');
}

function StepsPage({ draftScope, canDeleteSteps }: StepsPageProps) {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverSteps, setServerSteps] = useState<StepListItem[]>([]);
  const [resourceTeamPaths, setResourceTeamPaths] = useState<string[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [activeTeam, setActiveTeam] = useState('');
  const [searchTerm, setSearchTerm] = useState('');

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
  const {
    permissionTeam,
    canCreateStepHere,
    canUpdateSelectedStep,
  } = useStepPermissions(selectedId, activeTeam);
  const canUseStepDrafts = canCreateStepHere || canUpdateSelectedStep;
  const {
    drafts: draftSteps,
    draftsByID: draftsById,
    removeDraft: removeStepDraft,
    upsertDraft: upsertStepDraftState,
  } = useDraftCollection({
    enabled: canUseStepDrafts,
    scope: draftScope,
    changedEvent: STEP_DRAFTS_CHANGED_EVENT,
    getStorageKey: getStepDraftStorageKey,
    load: loadStepDrafts,
    upsert: upsertStepDraft,
    remove: deleteStepDraft,
    autosave: {
      active: Boolean(detail && isEditing && normalizeSource(detail.source) === 'draft'),
      id: detail?.id || '',
      yaml: editorValue,
    },
  });

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
    agentProfiles: string[];
    runtimePools: string[];
    reusableSteps: string[];
    secretScopes: Array<{ scope: string; items: string[] }>;
    variableScopes: Array<{ scope: string; items: string[] }>;
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], agentProfiles: [], runtimePools: [], reusableSteps: [], secretScopes: [], variableScopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<null | {
    title: string;
    items: string[];
    activeIndex: number;
    replaceStart: number;
    replaceEnd: number;
    appendColon: boolean;
    teamedSections?: Array<{ label: string; items: string[]; totalCount: number }>;
  }>(null);

  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

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

      const beforeLineText = text.slice(0, lineStart);
      const ancestorKey = findParentBlock(beforeLineText, ['secrets', 'variables', 'depends_on', 'tasks'], currentIndent) || '';
      const containerBlock = findParentBlock(beforeLineText, ['tasks'], currentIndent) || '';

      const includeValueContext =
        currentKey === 'include' || /^\s*include\s*:\s*[A-Za-z0-9_.:/-]*$/.test(lineBeforeCursor.trim());
      const agentProfileValueContext =
        currentKey === 'agent_profile' || /^\s*agent_profile\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());
      const runtimePoolValueContext =
        currentKey === 'runtime_pool' || /^\s*runtime_pool\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());
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
      let teamedSections: Array<{ label: string; items: string[]; totalCount: number }> | undefined;

      if (includeValueContext) {
        title = 'Reusable steps';
        pool = autocompleteMeta.reusableSteps.map(id => `step:${id}`);
      } else if (agentProfileValueContext) {
        title = 'Agent profiles';
        pool = autocompleteMeta.agentProfiles;
      } else if (runtimePoolValueContext) {
        title = 'Runtime pools';
        pool = autocompleteMeta.runtimePools;
      } else if (ancestorKey === 'secrets') {
        title = 'Secrets';
        const base = autocompleteMeta.secretScopes.length
          ? autocompleteMeta.secretScopes
          : [{ scope: '', items: autocompleteMeta.secrets }];
        let remaining = 50;
        teamedSections = base
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
        pool = teamedSections.flatMap(section => section.items);
      } else if (ancestorKey === 'depends_on') {
        title = 'Task dependencies';
        pool = resolveTaskNames();
      } else if (ancestorKey === 'variables' && cursorInKeyPosition) {
        title = 'Variables';
        const base = autocompleteMeta.variableScopes.length
          ? autocompleteMeta.variableScopes
          : [{ scope: '', items: autocompleteMeta.variables }];
        let remaining = 50;
        teamedSections = base
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
        pool = teamedSections.flatMap(section => section.items);
        appendColon = true;
      } else {
        appendColon = true;
        if (containerBlock === 'tasks') {
          title = 'Task keys';
          pool = TASK_DIRECTIVES;
        } else {
          title = 'Step keys';
          pool = STEP_DIRECTIVES;
        }
      }

      const normalizedPrefix = prefix.toLowerCase();
      const filtered = pool.filter(item => item.toLowerCase().startsWith(normalizedPrefix)).sort((a, b) => a.localeCompare(b));
      const hasContext =
        includeValueContext ||
        agentProfileValueContext ||
        runtimePoolValueContext ||
        ancestorKey === 'secrets' ||
        ancestorKey === 'depends_on' ||
        ancestorKey === 'variables';
      const isRootLine = !containerBlock && currentIndent === 0 && !currentKey;
      const shouldShow = opts?.force || hasContext || filtered.length > 0 || containerBlock === 'tasks';

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
        teamedSections,
      });
    },
    [autocompleteMeta.agentProfiles, autocompleteMeta.reusableSteps, autocompleteMeta.runtimePools, autocompleteMeta.secrets, autocompleteMeta.secretScopes, autocompleteMeta.variableScopes, autocompleteMeta.variables, editorValue]
  );

  const handleEditorTextChange = useCallback(
    (next: string, cursor: number) => {
      setEditorValue(next);
      openEditorSuggestion(cursor, { text: next });
    },
    [openEditorSuggestion]
  );

  const moveEditorSuggestion = useCallback((direction: 1 | -1) => {
    setEditorSuggestion(current => {
      if (!current || !current.items.length) return current;
      return {
        ...current,
        activeIndex: (current.activeIndex + direction + current.items.length) % current.items.length,
      };
    });
  }, []);

  const discardEditorChanges = useCallback(() => {
    if (!detail) return;
    const resetYaml = editSessionOriginalYamlRef.current || detail.rawYaml;
    setEditorSuggestion(null);
    setEditorValue(resetYaml);
    if (normalizeSource(detail.source) === 'draft' && draftScope) {
      upsertStepDraftState({ id: detail.id, yaml: resetYaml });
    }
    setIsEditing(false);
  }, [detail, draftScope, upsertStepDraftState]);

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
        const promise = (async () => {
          const metadata = await fetchEditorAutocompleteMetadata({ includeAgentProfiles: true, includeRuntimePools: true });
          setAutocompleteMeta(metadata);
          autocompleteFetchRef.current.fetchedAt = metadata.fetchedAt;
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
      setServerSteps(await fetchStepList());
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

        const rawYaml = await fetchStepYaml(stepId);
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
    [draftsById]
  );

  const loadUsage = useCallback(async (stepId: string) => {
    const targetId = stepId;
    setUsageLoading(true);
    setUsageError(null);
    try {
      const list = await fetchStepUsage(stepId);
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
    let cancelled = false;
    void fetchResourceTeamPaths()
      .then(paths => {
        if (!cancelled) setResourceTeamPaths(paths);
      })
      .catch(() => {
        if (!cancelled) setResourceTeamPaths([]);
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
    if (serverSteps.some(item => item.id === activeId)) return;
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/steps', { replace: true });
  }, [draftsById, listError, listLoading, navigate, serverSteps]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'steps') return;
    const isTeamRoute = segments[1] === TEAM_ROUTE_SEGMENT;
    if (isTeamRoute) {
      setSelectedId(null);
      selectedIdRef.current = null;
    } else if (segments.length > 1) {
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
    const routeTeam = isTeamRoute ? decodeTeamRouteSegments(segments.slice(2)) : '';
    const team = routeTeam || params.get('team') || '';
    setActiveTeam(team);
    if (!isTeamRoute && segments.length === 1 && params.get('team')) {
      navigate(teamScopedRoute('/steps', team), { replace: true });
    }
  }, [location.pathname, location.search, navigate]);

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

  const visibleSteps = useMemo(
    () => filterVisibleStepList(steps, searchTerm, activeTeam),
    [activeTeam, searchTerm, steps]
  );

  const buildTree = useMemo(() => {
    const root: TreeNode = { id: '__root__', name: '', fullPath: '', children: [], stepIds: [] };
    const createNode = (id: string, name: string, fullPath: string): TreeNode => ({ id, name, fullPath, children: [], stepIds: [] });
    const ensureGlobalNode = () => {
      insertTeamPath(root, GLOBAL_RESOURCE_TEAM_PATH, createNode);
      return root.children.find(child => child.fullPath === GLOBAL_RESOURCE_TEAM_PATH) || root;
    };
    ensureGlobalNode();
    resourceTeamPaths.forEach(path => {
      insertTeamPath(root, path, createNode);
    });
    steps.forEach(item => {
      const parts = item.id.split('/').filter(Boolean);
      const leafName = parts.pop();
      if (!leafName) return;
      let current = parts.length ? root : ensureGlobalNode();
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], stepIds: [] };
          current.children.push(child);
          current.children.sort(compareResourceTreeNodes);
        }
        current = child;
      });
      current.stepIds.push(item.id);
      current.stepIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [resourceTeamPaths, steps]);

  const parentTeam = (path: string) => {
    const parts = path.split('/').filter(Boolean);
    parts.pop();
    return parts.join('/');
  };

  const openTeam = (path: string) => {
    const cleaned = path.trim().replace(/^\/+|\/+$/g, '');
    setActiveTeam(cleaned);
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate(teamScopedRoute('/steps', cleaned));
  };

  const handleSelect = useCallback((id: string) => {
    selectedIdRef.current = id;
    setSelectedId(id);
    navigate(`/steps/${id.split('/').map(encodeURIComponent).join('/')}`);
  }, [navigate]);

  const parseSavedStep = useCallback(
    (rawYaml: string, id: string, source?: string) =>
      parseStepYaml(rawYaml, id, source, new Date().toISOString()),
    []
  );

  const handleStepSaved = useCallback((updated: StepDetail) => {
    setDetail(updated);
    setEditorValue(updated.rawYaml);
    setIsEditing(false);
  }, []);

  const handleStepDeleted = useCallback(() => {
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/steps');
  }, [navigate]);

  const {
    closeDeleteModal,
    closeFormModal,
    confirmDelete,
    deleteModal,
    formModal,
    openCloneModal,
    openCreateModal,
    openDeleteModal,
    save: handleSave,
    saving,
    submitFormModal,
    updateFormModal,
  } = useYamlResourceMutations({
    resourceLabel: 'step',
    resources: steps,
    detail,
    editorValue,
    validationErrorCount: validation.errors.length,
    validationMessage: 'Fix validation errors before saving.',
    permissionTeam,
    draftScope,
    canCreate: canCreateStepHere,
    canUpdate: canUpdateSelectedStep,
    canDelete: canDeleteSteps,
    canUseDrafts: canUseStepDrafts,
    namePattern: STEP_NAME_PATTERN,
    normalizePath: normalizeRootPath,
    normalizeSource,
    checkCreatePermission: checkStepPermission,
    persistYaml: saveStepYaml,
    deleteResource: deleteStep,
    upsertDraft: upsertStepDraftState,
    removeDraft: removeStepDraft,
    parseSaved: parseSavedStep,
    reloadResources: loadSteps,
    addToast,
    onSelect: handleSelect,
    onSaved: handleStepSaved,
    onDeleted: handleStepDeleted,
    buildTemplate: buildStepTemplateYaml,
  });

  const handleBackToList = () => {
    if (detail) {
      const team = splitIdentifier(detail.id).path;
      navigate(teamScopedRoute('/steps', team));
      return;
    }
    navigate('/steps');
  };

  const handleCopy = async () => {
    if (!detail?.rawYaml) return;
    try {
      await copyTextToClipboard(detail.rawYaml);
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

  return (
    <div data-page="steps" className="active h-full min-h-0 flex flex-col overflow-hidden">
      {!selectedId && (
        <ResourceCollectionToolbar
          resourceLabel="step"
          activeTeam={activeTeam}
          searchTerm={searchTerm}
          canCreate={canCreateStepHere}
          onBack={() => openTeam(parentTeam(activeTeam))}
          onSearchTermChange={setSearchTerm}
          onCreate={openCreateModal}
        />
      )}

      <div className="flex-1 min-h-0 overflow-hidden">
        <main id="main-content-steps" className="pipeline-runs-main-scroll h-full min-h-0 overflow-y-auto p-4 space-y-3">
          {!selectedId ? (
            <StepCollectionList
              listLoading={listLoading}
              listError={listError}
              visibleSteps={visibleSteps}
              treeRoot={buildTree}
              activeTeam={activeTeam}
              canCreateStepHere={canCreateStepHere}
              canUseStepDrafts={canUseStepDrafts}
              canDeleteSteps={canDeleteSteps}
              onSelectStep={handleSelect}
              onOpenTeam={openTeam}
              onDeleteStep={openDeleteModal}
            />
          ) : detailLoading ? (
            <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading step…</div>
          ) : detailError ? (
            <div className="glass-card p-5 text-sm text-red-500">Failed to load step: {detailError}</div>
          ) : (
            <StepDetailView
              detail={detail}
              isEditing={isEditing}
              editorValue={editorValue}
              validationErrors={validation.errors}
              validationErrorLines={validationErrorLines}
              editorSuggestion={editorSuggestion}
              autocompleteLoading={autocompleteMeta.loading}
              editorRef={editorRef}
              highlightContentRef={highlightContentRef}
              lineNumbersRef={lineNumbersRef}
              canUpdateSelectedStep={canUpdateSelectedStep}
              canCreateStepHere={canCreateStepHere}
              saving={saving}
              usage={usage}
              usageLoading={usageLoading}
              usageError={usageError}
              onBack={handleBackToList}
              onCopy={() => void handleCopy()}
              onDownload={handleDownload}
              onEdit={() => setIsEditing(true)}
              onClone={openCloneModal}
              onDiscard={discardEditorChanges}
              onSave={() => void handleSave()}
              onEditorTextChange={handleEditorTextChange}
              onOpenSuggestion={openEditorSuggestion}
              onMoveSuggestion={moveEditorSuggestion}
              onDismissSuggestion={() => setEditorSuggestion(null)}
              onSelectSuggestion={applyEditorSuggestion}
              onEditorScroll={handleEditorScroll}
              onAutoIndentEnter={() => handleAutoIndentEnter()}
            />
          )}
        </main>
      </div>

      <ResourceWorkflowModals
        resourceLabel="step"
        formModal={formModal}
        formModalId={mode => (mode === 'create' ? 'steps-new-modal' : 'steps-clone-modal')}
        pathPlaceholder="library/docker"
        namePlaceholder="build-image"
        deleteModal={
          deleteModal
            ? {
                resourceName: deleteModal.resourceName,
                gitOpsManaged: deleteModal.gitOpsManaged,
                pending: deleteModal.pending,
                error: deleteModal.error,
              }
            : null
        }
        deleteModalId="steps-delete-modal"
        onChangeForm={updateFormModal}
        onCloseForm={closeFormModal}
        onSubmitForm={() => void submitFormModal()}
        onCloseDelete={closeDeleteModal}
        onConfirmDelete={() => void confirmDelete()}
      />

      <WorkflowToastRegion toasts={toasts} />
    </div>
  );
}

export default StepsPage;
