import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { ArrowLeft, Copy, Download, Play, Trash2 } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  PIPELINE_DRAFTS_CHANGED_EVENT,
  deletePipelineDraft,
  getPipelineDraftStorageKey,
  loadPipelineDrafts,
  upsertPipelineDraft,
} from '../lib/pipelineDrafts';
import { fetchResourceGroupPaths, insertGroupPath } from '../lib/resourceGroups';
import { applyEnterIndent, findParentBlock } from '../lib/lab';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';
import ResourceAccessCard from '../components/ResourceAccessCard';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { fetchEditorAutocompleteMetadata } from '../features/editor/autocomplete';
import { EditorAutocompleteMenu } from '../features/editor/EditorAutocompleteMenu';
import { ResourceCollectionToolbar } from '../features/editor/ResourceCollectionToolbar';
import { ResourceWorkflowModals } from '../features/editor/ResourceWorkflowModals';
import { YamlValidationPanel } from '../features/editor/YamlValidationPanel';
import { useDraftCollection } from '../features/editor/useDraftCollection';
import { useYamlResourceMutations } from '../features/editor/useYamlResourceMutations';
import { StepsGraph } from '../features/pipeline-runs/RunGraph';
import { PipelineActivityPanels } from '../features/pipelines/PipelineActivityPanels';
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
  PIPELINE_DIRECTIVES,
  STEP_DIRECTIVES,
  TASK_DIRECTIVES,
  buildPipelineGraphData,
  encodeId,
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

const MAX_RECENT_RUNS = 5;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

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

function PipelinesPage({ draftScope, canDeletePipelines }: PipelinesPageProps) {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverPipelines, setServerPipelines] = useState<PipelineListItem[]>([]);
  const [listLoading, setListLoading] = useState<boolean>(true);
  const [listError, setListError] = useState<string | null>(null);
  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);

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
  const {
    permissionFolder,
    canCreatePipelineHere,
    canUpdateSelectedPipeline,
    canExecuteSelectedPipeline,
  } = usePipelinePermissions(selectedId, activeFolder);
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
  const parsePipelineYamlRef = useRef(parsePipelineYaml);

  const [autocompleteMeta, setAutocompleteMeta] = useState<{
    secrets: string[];
    variables: string[];
    llmProfiles: string[];
    mcpProfiles: string[];
    reusableSteps: string[];
    secretScopes: Array<{ scope: string; items: string[] }>;
    variableScopes: Array<{ scope: string; items: string[] }>;
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], llmProfiles: [], mcpProfiles: [], reusableSteps: [], secretScopes: [], variableScopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<null | {
    title: string;
    items: string[];
    activeIndex: number;
    replaceStart: number;
    replaceEnd: number;
    appendColon: boolean;
    groupedSections?: Array<{ label: string; items: string[]; totalCount: number }>;
  }>(null);

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
      const ancestorKey = findParentBlock(beforeLineText, ['secrets', 'variables', 'depends_on', 'mcp_profiles', 'tasks', 'steps'], currentIndent) || '';
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
          return parsePipelineYamlRef.current(text, detail.id, detail.source).stepNames;
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
      } else if (ancestorKey === 'mcp_profiles') {
        title = 'MCP profiles';
        pool = autocompleteMeta.mcpProfiles;
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
        includeValueContext || llmProfileValueContext || ancestorKey === 'mcp_profiles' || ancestorKey === 'secrets' || ancestorKey === 'variables' || ancestorKey === 'depends_on';
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
        const promise = (async () => {
          const metadata = await fetchEditorAutocompleteMetadata({
            includeLLMProfiles: true,
            includeMCPProfiles: true,
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

        const rawYaml = await fetchPipelineYaml(pipelineId);
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
  }, [activeFolder, filteredPipelines, searchTerm]);

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

  const handleSelect = useCallback((id: string) => {
    selectedIdRef.current = id;
    setSelectedId(id);
    navigate(`/pipelines/${id.split('/').map(encodeURIComponent).join('/')}`);
  }, [navigate]);

  const handlePipelineSaved = useCallback((updated: PipelineDetail) => {
    setDetail(updated);
    setEditorValue(updated.rawYaml);
    setIsEditing(false);
  }, []);

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
    save: handleSave,
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
    permissionFolder,
    draftScope,
    canCreate: canCreatePipelineHere,
    canUpdate: canUpdateSelectedPipeline,
    canDelete: canDeletePipelines,
    canUseDrafts: canUsePipelineDrafts,
    namePattern: /^[a-zA-Z0-9_.-]+$/,
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
                  openDeleteModal(pipeline.id, name || pipeline.id);
                }}
                aria-label={source === 'draft' ? 'Discard draft pipeline' : 'Delete pipeline'}
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" />
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
    const executeDisabled = isEditing || source === 'draft' || !canExecuteSelectedPipeline;
    const executeTitle = source === 'draft'
      ? 'Save the draft before executing'
      : isEditing
        ? 'Save or discard edits before executing'
        : canExecuteSelectedPipeline
          ? 'Execute in Lab'
          : 'You do not have permission to execute this pipeline';
    const editorLines = editorValue.split('\n');
    return (
      <div id="pipelines-detail-view" className="pipelines-view">
        <div className="min-w-0 space-y-6">
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
              <div className="flex flex-wrap items-center justify-end gap-2">
                <button
                  id="pipelines-execute-btn"
                  type="button"
                  className="glass-button-primary"
                  onClick={handleExecute}
                  disabled={executeDisabled}
                  title={executeTitle}
                >
                  <Play className="h-4 w-4" aria-hidden="true" />
                  <span>Execute</span>
                </button>
                <button id="pipelines-back-btn" type="button" className="glass-button-ghost" onClick={handleBackToList}>
                  <ArrowLeft className="h-4 w-4" aria-hidden="true" />
                  <span>Back to list</span>
                </button>
              </div>
            </div>
          </div>

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
            <div className="min-w-0 space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Pipeline Definition (YAML)</h3>
                  <div className="flex items-center gap-2 flex-wrap">
                    {!isEditing ? (
                      <>
                        <button className="glass-button-ghost" onClick={handleCopy} title="Copy YAML">
                          <Copy className="h-4 w-4" aria-hidden="true" />
                        </button>
                        <button className="glass-button-ghost" onClick={handleDownload} title="Download YAML">
                          <Download className="h-4 w-4" aria-hidden="true" />
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
                              upsertPipelineDraftState({ id: detail.id, yaml: resetYaml });
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
                          aria-label="Pipeline YAML editor"
                          aria-describedby="pipeline-validation-status"
                          aria-invalid={validation.errors.length > 0}
                          aria-autocomplete="list"
                          aria-controls={editorSuggestion ? 'pipeline-editor-autocomplete' : undefined}
                          aria-activedescendant={
                            editorSuggestion ? `pipeline-editor-autocomplete-option-${editorSuggestion.activeIndex}` : undefined
                          }
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

                            if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
                              event.preventDefault();
                              handleAutoIndentEnter();
                            }
                          }}
                          spellCheck={false}
                        ></textarea>
                      </div>
                      <YamlValidationPanel id="pipeline-validation-status" errors={validation.errors} />
                      {editorSuggestion ? (
                        <EditorAutocompleteMenu
                          id="pipeline-editor-autocomplete"
                          suggestion={editorSuggestion}
                          loading={autocompleteMeta.loading}
                          onSelect={applyEditorSuggestion}
                        />
                      ) : null}
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
            <PipelineActivityPanels
              pipelineLabel={detail.name || detail.id}
              triggers={triggers}
              triggersLoading={triggersLoading}
              triggersError={triggersError}
              dependencies={detail.includedDependencies}
              runs={recentRuns}
              runsLoading={runsLoading}
              runsError={runsError}
              onOpenTrigger={repoSlug => navigate(`/triggers/${encodeId(repoSlug)}`)}
              onOpenDependency={handleSelect}
              onCopyDependency={async identifier => {
                try {
                  await navigator.clipboard.writeText(identifier);
                  addToast('Copied dependency reference.', 'success');
                } catch (error) {
                  console.error('Failed to copy dependency reference', error);
                  addToast('Unable to copy dependency reference.', 'error');
                }
              }}
              onOpenRun={runID =>
                navigate(runID ? `/pipelineruns/recent?run=${encodeURIComponent(runID)}` : '/pipelineruns/recent')
              }
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <div data-page="pipelines" className="active h-full flex flex-col">
      {!selectedId && (
        <ResourceCollectionToolbar
          resourceLabel="pipeline"
          activeFolder={activeFolder}
          searchTerm={searchTerm}
          canCreate={canCreatePipelineHere}
          onBack={() => openFolder(parentFolder(activeFolder))}
          onSearchTermChange={setSearchTerm}
          onCreate={openCreateModal}
        />
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
