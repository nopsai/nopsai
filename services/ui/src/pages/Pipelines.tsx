import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  PIPELINE_DRAFTS_CHANGED_EVENT,
  deletePipelineDraft,
  getPipelineDraftStorageKey,
  loadPipelineDrafts,
  upsertPipelineDraft,
} from '../lib/pipelineDrafts';
import { applyEnterIndent } from '../lib/lab';
import { copyTextToClipboard } from '../lib/clipboard';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { fetchEditorAutocompleteMetadata } from '../features/editor/autocomplete';
import { ResourceCollectionToolbar } from '../features/editor/ResourceCollectionToolbar';
import { ResourceWorkflowModals } from '../features/editor/ResourceWorkflowModals';
import { useDraftCollection } from '../features/editor/useDraftCollection';
import { updateYamlResourceName, useYamlResourceMutations } from '../features/editor/useYamlResourceMutations';
import { PipelineCollectionList } from '../features/pipelines/PipelineCollectionList';
import { PipelineDetailView } from '../features/pipelines/PipelineDetailView';
import {
  buildPipelineEditorSuggestion,
  type PipelineEditorSuggestion,
} from '../features/pipelines/editorAutocomplete';
import {
  fetchPipelineList,
  fetchPipelineTriggers as fetchPipelineTriggersRequest,
  fetchPipelineYaml,
  fetchRecentPipelineRuns,
  checkPipelinePermission,
  deletePipeline,
  savePipelineYaml,
  type PipelineRun,
  type PipelineTrigger,
} from '../features/pipelines/api';
import {
  buildPipelineGraphData,
  encodeId,
  filterVisiblePipelineList,
  normalizePipelineSource as normalizeSource,
  normalizeRootPath,
  parsePipelineYaml,
  splitIdentifier,
  validatePipelineYaml,
  type PipelineDetail,
  type PipelineGraphData,
  type PipelineListItem,
} from '../features/pipelines/model';
import { usePipelinePermissions } from '../features/pipelines/usePipelinePermissions';
import {
  GLOBAL_RESOURCE_TEAM_PATH,
  compareResourceTreeNodes,
  fetchResourceTeamPaths,
  insertTeamPath,
} from '../lib/resourceTeams';
import { TEAM_ROUTE_SEGMENT, decodeTeamRouteSegments, teamScopedRoute } from '../lib/teamRoutes';

const MAX_RECENT_RUNS = 5;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;
const PIPELINE_NAME_PATTERN = /^[a-zA-Z0-9_.-]+$/;

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

function buildPipelineTemplateYaml(name: string) {
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
}

function pipelineIdentityFromIdentifier(id: string) {
  return splitIdentifier(id);
}

function buildPipelineIdentifier(path: string, name: string) {
  const normalizedPath = normalizeRootPath(path);
  const trimmedName = name.trim();
  return trimmedName ? (normalizedPath ? `${normalizedPath}/${trimmedName}` : trimmedName) : '';
}

function PipelinesPage({ draftScope, canDeletePipelines }: PipelinesPageProps) {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverPipelines, setServerPipelines] = useState<PipelineListItem[]>([]);
  const [resourceTeamPaths, setResourceTeamPaths] = useState<string[]>([]);
  const [listLoading, setListLoading] = useState<boolean>(true);
  const [listError, setListError] = useState<string | null>(null);
  const [activeTeam, setActiveTeam] = useState('');
  const [searchTerm, setSearchTerm] = useState('');

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
  const [selectedGraphStep, setSelectedGraphStep] = useState<string | null>(null);

  const [isEditing, setIsEditing] = useState(false);
  const [editorValue, setEditorValue] = useState('');
  const [editableIdentity, setEditableIdentity] = useState({ path: '', name: '' });
  const {
    permissionTeam,
    canCreatePipelineHere,
    canUpdateSelectedPipeline,
    canExecuteSelectedPipeline,
  } = usePipelinePermissions(selectedId, activeTeam);
  const canUsePipelineDrafts = canCreatePipelineHere || canUpdateSelectedPipeline;
  const {
    drafts: draftPipelines,
    draftsByID: draftsById,
    removeDraft: removePipelineDraft,
    upsertDraft: upsertPipelineDraftState,
  } = useDraftCollection({
    enabled: canUsePipelineDrafts,
    scope: draftScope,
    changedEvent: PIPELINE_DRAFTS_CHANGED_EVENT,
    getStorageKey: getPipelineDraftStorageKey,
    load: loadPipelineDrafts,
    upsert: upsertPipelineDraft,
    remove: deletePipelineDraft,
    autosave: {
      active: Boolean(detail && isEditing && normalizeSource(detail.source) === 'draft'),
      id: detail?.id || '',
      yaml: editorValue,
    },
  });

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
    agentProfiles: string[];
    llmProfiles: string[];
    mcpProfiles: string[];
    runtimePools: string[];
    reusableSteps: string[];
    secretScopes: Array<{ scope: string; items: string[] }>;
    variableScopes: Array<{ scope: string; items: string[] }>;
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], agentProfiles: [], llmProfiles: [], mcpProfiles: [], runtimePools: [], reusableSteps: [], secretScopes: [], variableScopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<PipelineEditorSuggestion | null>(null);

  const validation = useMemo(() => {
    if (!isEditing) return { errors: [] };
    return validatePipelineYaml(editorValue);
  }, [editorValue, isEditing]);

  const validationErrorLines = useMemo(() => {
    const lines = new Set<number>();
    validation.errors.forEach(err => {
      if (typeof err.line === 'number') lines.add(err.line);
    });
    return lines;
  }, [validation.errors]);

  const graphData = useMemo<PipelineGraphData>(() => {
    const source = isEditing ? editorValue : detail?.rawYaml;
    return buildPipelineGraphData(source);
  }, [detail?.rawYaml, editorValue, isEditing]);

  useEffect(() => {
    if (selectedGraphStep && !graphData.steps.some(step => step.name === selectedGraphStep)) {
      setSelectedGraphStep(null);
    }
  }, [graphData.steps, selectedGraphStep]);

  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

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
      setEditorSuggestion(buildPipelineEditorSuggestion({
        text,
        cursor,
        force: opts?.force,
        metadata: autocompleteMeta,
        detail,
      }));
    },
    [autocompleteMeta, detail, editorValue]
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
    setEditableIdentity(pipelineIdentityFromIdentifier(detail.id));
    if (normalizeSource(detail.source) === 'draft' && draftScope) {
      upsertPipelineDraftState({ id: detail.id, yaml: resetYaml });
    }
    setIsEditing(false);
  }, [detail, draftScope, upsertPipelineDraftState]);

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
          const metadata = await fetchEditorAutocompleteMetadata({
            includeAgentProfiles: true,
            includeLLMProfiles: true,
            includeMCPProfiles: true,
            includeRuntimePools: true,
          });
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

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

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
    navigate(teamScopedRoute('/pipelines', cleaned));
  };

  const loadRecentRuns = useCallback(
    async (pipelineId: string) => {
      const targetId = pipelineId;
      setRunsLoading(true);
      setRunsError(null);
      try {
        const filtered = await fetchRecentPipelineRuns(pipelineId, MAX_RECENT_RUNS);
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
      setTriggersLoading(true);
      setTriggersError(null);
      try {
        const results = await fetchPipelineTriggersRequest(pipelineId);
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
      setServerPipelines(await fetchPipelineList());
    } catch (error) {
      console.error('Failed to load pipelines', error);
      setListError(error instanceof Error ? error.message : 'Unable to load pipelines');
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadPipelineDetail = useCallback(
    async (pipelineId: string, source?: string, updatedAt?: string) => {
      const normalizedSource = normalizeSource(source);
      setDetailLoading(true);
      setDetailError(null);
      try {
        if (normalizedSource === 'draft') {
          const draft = draftsById.get(pipelineId);
          if (!draft) throw new Error('Draft pipeline not found');
          const parsed = parsePipelineYaml(draft.yaml, pipelineId, 'draft', draft.updatedAt);
          setDetail(parsed);
          setEditorValue(draft.yaml);
          setEditableIdentity(pipelineIdentityFromIdentifier(pipelineId));
          setIsEditing(true);
          return;
        }

        const rawYaml = await fetchPipelineYaml(pipelineId);
        const parsed = parsePipelineYaml(rawYaml, pipelineId, normalizedSource, updatedAt);
        setDetail(parsed);
        setEditorValue(rawYaml);
        setEditableIdentity(pipelineIdentityFromIdentifier(pipelineId));
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

  const pipelines = useMemo(() => {
    const merged = new Map<string, PipelineListItem>();
    serverPipelines.forEach(item => merged.set(item.id, item));
    draftPipelines.forEach(draft => merged.set(draft.id, { id: draft.id, source: 'draft', updatedAt: draft.updatedAt }));
    return Array.from(merged.values()).sort((a, b) => a.id.localeCompare(b.id));
  }, [serverPipelines, draftPipelines]);

  useEffect(() => {
    void loadPipelines();
  }, [loadPipelines]);

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
    if (serverPipelines.some(item => item.id === activeId)) return;
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/pipelines', { replace: true });
  }, [listLoading, listError, draftsById, serverPipelines, navigate]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'pipelines') return;
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
      navigate(teamScopedRoute('/pipelines', team), { replace: true });
    }
  }, [location.pathname, location.search, navigate]);

  useEffect(() => {
    if (!selectedId) {
      setDetail(null);
      setEditorValue('');
      setEditableIdentity({ path: '', name: '' });
      setIsEditing(false);
      return;
    }
    const selectedItem = pipelines.find(item => item.id === selectedId);
    void loadPipelineDetail(selectedId, selectedItem?.source, selectedItem?.updatedAt);
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

  const visiblePipelines = useMemo(
    () => filterVisiblePipelineList(pipelines, searchTerm, activeTeam),
    [activeTeam, pipelines, searchTerm]
  );

  const buildTree = useMemo(() => {
    const root: TreeNode = { id: '__root__', name: '', fullPath: '', children: [], pipelineIds: [] };
    const createNode = (id: string, name: string, fullPath: string): TreeNode => ({ id, name, fullPath, children: [], pipelineIds: [] });
    const ensureGlobalNode = () => {
      insertTeamPath(root, GLOBAL_RESOURCE_TEAM_PATH, createNode);
      return root.children.find(child => child.fullPath === GLOBAL_RESOURCE_TEAM_PATH) || root;
    };
    ensureGlobalNode();
    resourceTeamPaths.forEach(path => {
      insertTeamPath(root, path, createNode);
    });
    pipelines.forEach(item => {
      const parts = item.id.split('/').filter(Boolean);
      const pipelineName = parts.pop();
      if (!pipelineName) return;
      let current = parts.length ? root : ensureGlobalNode();
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], pipelineIds: [] };
          current.children.push(child);
          current.children.sort(compareResourceTreeNodes);
        }
        current = child;
      });
      current.pipelineIds.push(item.id);
      current.pipelineIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [pipelines, resourceTeamPaths]);

  const handleSelect = useCallback((id: string) => {
    selectedIdRef.current = id;
    setSelectedId(id);
    navigate(`/pipelines/${id.split('/').map(encodeURIComponent).join('/')}`);
  }, [navigate]);

  const handlePipelineSaved = useCallback((updated: PipelineDetail) => {
    setDetail(updated);
    setEditorValue(updated.rawYaml);
    setEditableIdentity(pipelineIdentityFromIdentifier(updated.id));
    setIsEditing(false);
  }, []);

  const handleStartEdit = useCallback(() => {
    if (!detail) return;
    setEditableIdentity(pipelineIdentityFromIdentifier(detail.id));
    setIsEditing(true);
  }, [detail]);

  const handlePipelineDeleted = useCallback(() => {
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/pipelines');
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
    save: savePipelineResource,
    saving,
    submitFormModal,
    updateFormModal,
  } = useYamlResourceMutations({
    resourceLabel: 'pipeline',
    resources: pipelines,
    detail,
    editorValue,
    validationErrorCount: validation.errors.length,
    validationMessage: 'Resolve validation errors before saving.',
    permissionTeam,
    draftScope,
    canCreate: canCreatePipelineHere,
    canUpdate: canUpdateSelectedPipeline,
    canDelete: canDeletePipelines,
    canUseDrafts: canUsePipelineDrafts,
    namePattern: PIPELINE_NAME_PATTERN,
    normalizePath: normalizeRootPath,
    normalizeSource,
    checkCreatePermission: checkPipelinePermission,
    persistYaml: savePipelineYaml,
    deleteResource: deletePipeline,
    upsertDraft: upsertPipelineDraftState,
    removeDraft: removePipelineDraft,
    parseSaved: parsePipelineYaml,
    reloadResources: loadPipelines,
    addToast,
    onSelect: handleSelect,
    onSaved: handlePipelineSaved,
    onDeleted: handlePipelineDeleted,
    buildTemplate: buildPipelineTemplateYaml,
  });

  const handleSavePipeline = useCallback(async () => {
    if (!detail) return;
    const name = editableIdentity.name.trim();
    const path = normalizeRootPath(editableIdentity.path);
    const nextID = buildPipelineIdentifier(path, name);
    if (!name) {
      addToast('Pipeline name is required.', 'error');
      return;
    }
    if (!PIPELINE_NAME_PATTERN.test(name)) {
      addToast('Pipeline name can only contain letters, numbers, dots, underscores, and hyphens.', 'error');
      return;
    }
    const nextYaml = updateYamlResourceName(editorValue, name);
    const saved = await savePipelineResource({ resourceID: nextID, rawYaml: nextYaml });
    if (saved) {
      setEditorValue(nextYaml);
      setEditableIdentity({ path, name });
    }
  }, [addToast, detail, editableIdentity.name, editableIdentity.path, editorValue, savePipelineResource]);

  const handleBackToList = () => {
    setSelectedId(null);
    selectedIdRef.current = null;
    navigate('/pipelines');
  };

  const handleCopy = async () => {
    if (!detail?.rawYaml) return;
    try {
      await copyTextToClipboard(detail.rawYaml);
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

  const handleExecute = () => {
    if (!detail) return;
    const source = normalizeSource(detail.source);
    if (source === 'draft') {
      addToast('Save this draft before executing it in Lab.', 'info');
      return;
    }
    if (isEditing) {
      addToast('Save or discard edits before executing this pipeline.', 'info');
      return;
    }
    if (!canExecuteSelectedPipeline) {
      addToast('You do not have permission to execute this pipeline.', 'info');
      return;
    }
    navigate(`/lab?pipeline=${encodeURIComponent(detail.id)}`);
  };

  return (
    <div data-page="pipelines" className="active h-full min-h-0 flex flex-col overflow-hidden">
      {!selectedId && (
        <ResourceCollectionToolbar
          resourceLabel="pipeline"
          activeTeam={activeTeam}
          searchTerm={searchTerm}
          canCreate={canCreatePipelineHere}
          onBack={() => openTeam(parentTeam(activeTeam))}
          onSearchTermChange={setSearchTerm}
          onCreate={openCreateModal}
        />
      )}
      <div className="flex-1 min-h-0 overflow-hidden">
        <main id="main-content-pipelines" className="pipeline-runs-main-scroll h-full min-h-0 overflow-y-auto p-4 space-y-3">
          {!selectedId ? (
            <PipelineCollectionList
              listLoading={listLoading}
              listError={listError}
              visiblePipelines={visiblePipelines}
              treeRoot={buildTree}
              activeTeam={activeTeam}
              canCreatePipelineHere={canCreatePipelineHere}
              canUsePipelineDrafts={canUsePipelineDrafts}
              canDeletePipelines={canDeletePipelines}
              onSelectPipeline={handleSelect}
              onOpenTeam={openTeam}
              onDeletePipeline={openDeleteModal}
            />
          ) : detailLoading ? (
            <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading pipeline…</div>
          ) : detailError ? (
            <div className="glass-card p-5 text-sm text-red-500">Failed to load pipeline: {detailError}</div>
          ) : (
            <PipelineDetailView
              detail={detail}
              graphData={graphData}
              selectedGraphStep={selectedGraphStep}
              isEditing={isEditing}
              editorValue={editorValue}
              validationErrors={validation.errors}
              validationErrorLines={validationErrorLines}
              editorSuggestion={editorSuggestion}
              autocompleteLoading={autocompleteMeta.loading}
              editorRef={editorRef}
              highlightContentRef={highlightContentRef}
              lineNumbersRef={lineNumbersRef}
              canUpdateSelectedPipeline={canUpdateSelectedPipeline}
              canCreatePipelineHere={canCreatePipelineHere}
              canExecuteSelectedPipeline={canExecuteSelectedPipeline}
              saving={saving}
              editablePipelineName={editableIdentity.name}
              editablePipelineTeam={editableIdentity.path}
              triggers={triggers}
              triggersLoading={triggersLoading}
              triggersError={triggersError}
              recentRuns={recentRuns}
              runsLoading={runsLoading}
              runsError={runsError}
              onBack={handleBackToList}
              onExecute={handleExecute}
              onCopy={() => void handleCopy()}
              onDownload={handleDownload}
              onEdit={handleStartEdit}
              onClone={openCloneModal}
              onDiscard={discardEditorChanges}
              onSave={() => void handleSavePipeline()}
              onEditablePipelineNameChange={value => setEditableIdentity(current => ({ ...current, name: value }))}
              onEditablePipelineTeamChange={value => setEditableIdentity(current => ({ ...current, path: value }))}
              onSelectGraphStep={setSelectedGraphStep}
              onOpenTrigger={repoSlug => navigate(`/triggers/${encodeId(repoSlug)}`)}
              onOpenDependency={handleSelect}
              onCopyDependency={async identifier => {
                try {
                  await copyTextToClipboard(identifier);
                  addToast('Copied dependency reference.', 'success');
                } catch (error) {
                  console.error('Failed to copy dependency reference', error);
                  addToast('Unable to copy dependency reference.', 'error');
                }
              }}
              onOpenRun={runID =>
                navigate(runID ? `/pipelineruns/recent?run=${encodeURIComponent(runID)}` : '/pipelineruns/recent')
              }
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
        resourceLabel="pipeline"
        formModal={formModal}
        formModalId={mode => (mode === 'create' ? 'pipelines-new-modal' : 'pipelines-clone-modal')}
        pathPlaceholder="team/service"
        namePlaceholder="build-and-test"
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
        deleteModalId="pipelines-delete-modal"
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

export default PipelinesPage;
