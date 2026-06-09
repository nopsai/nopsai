import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { ArrowLeft, Copy, Download, Trash2 } from 'lucide-react';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import {
  STEP_DRAFTS_CHANGED_EVENT,
  deleteStepDraft,
  getStepDraftStorageKey,
  loadStepDrafts,
  upsertStepDraft,
} from '../lib/stepDrafts';
import { fetchResourceGroupPaths, insertGroupPath } from '../lib/resourceGroups';
import { applyEnterIndent, findParentBlock } from '../lib/lab';
import { renderYamlHighlight, renderYamlLines } from '../lib/yamlRenderer';
import ResourceAccessCard from '../components/ResourceAccessCard';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { fetchEditorAutocompleteMetadata } from '../features/editor/autocomplete';
import { EditorAutocompleteMenu } from '../features/editor/EditorAutocompleteMenu';
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
  formatUpdatedAt,
  normalizeRootPath,
  normalizeSource,
  parseStepYaml,
  splitIdentifier,
  validateStepYaml,
  type StepDetail,
} from '../features/steps/model';
import { useStepPermissions } from '../features/steps/useStepPermissions';

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
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [activeFolder, setActiveFolder] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);

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
    permissionFolder,
    canCreateStepHere,
    canUpdateSelectedStep,
  } = useStepPermissions(selectedId, activeFolder);
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
    reusableSteps: string[];
    secretScopes: Array<{ scope: string; items: string[] }>;
    variableScopes: Array<{ scope: string; items: string[] }>;
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], reusableSteps: [], secretScopes: [], variableScopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<null | {
    title: string;
    items: string[];
    activeIndex: number;
    replaceStart: number;
    replaceEnd: number;
    appendColon: boolean;
    groupedSections?: Array<{ label: string; items: string[]; totalCount: number }>;
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
      let groupedSections: Array<{ label: string; items: string[]; totalCount: number }> | undefined;

      if (includeValueContext) {
        title = 'Reusable steps';
        pool = autocompleteMeta.reusableSteps.map(id => `step:${id}`);
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
      } else if (ancestorKey === 'depends_on') {
        title = 'Task dependencies';
        pool = resolveTaskNames();
      } else if (ancestorKey === 'variables' && cursorInKeyPosition) {
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
        includeValueContext || ancestorKey === 'secrets' || ancestorKey === 'depends_on' || ancestorKey === 'variables';
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
        groupedSections,
      });
    },
    [autocompleteMeta.reusableSteps, autocompleteMeta.secrets, autocompleteMeta.secretScopes, autocompleteMeta.variableScopes, autocompleteMeta.variables, editorValue]
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
          const metadata = await fetchEditorAutocompleteMetadata();
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
    void fetchResourceGroupPaths()
      .then(paths => {
        if (!cancelled) setResourceGroupPaths(paths);
      })
      .catch(error => {
        console.warn('Failed to load groups for step tree', error);
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
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], stepIds: [] }));
    });
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
  }, [resourceGroupPaths, steps]);

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
    permissionFolder,
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

  const renderStepCard = (step: StepListItem) => {
    const { name, path } = splitIdentifier(step.id);
    const source = normalizeSource(step.source);
    const canDeleteThisStep = source === 'draft' ? canUseStepDrafts : canDeleteSteps && source !== 'git';
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
            {canDeleteThisStep ? (
              <button
                type="button"
                className="pipelines-delete-button"
                title={source === 'draft' ? 'Discard draft' : 'Delete step'}
                onClick={event => {
                  event.stopPropagation();
                  openDeleteModal(step.id, name || step.id);
                }}
                aria-label={source === 'draft' ? 'Discard draft step' : 'Delete step'}
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
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
            <span className="pipeline-card-meta-label">Sub groups:</span>
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
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreateStepHere ? 'Create a new step or adjust your filters.' : 'Adjust your filters or check your access.'}
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
        <div className="min-w-0 space-y-6">
          <div className="glass-card p-6">
            <div className="flex items-start justify-between gap-4 w-full mb-4">
              <div className="min-w-0 flex items-start gap-3">
                <span className="step-logo step-logo--detail step-logo--steps mt-1" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 2l8 4.5v11L12 22 4 17.5v-11L12 2z" />
                    <path d="M12 22v-7.5" />
                    <path d="M20 6.5l-8 4.5-8-4.5" />
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
              <button id="steps-back-btn" type="button" className="glass-button-ghost" onClick={handleBackToList}>
                <ArrowLeft className="h-4 w-4" aria-hidden="true" />
                <span>Back to list</span>
              </button>
            </div>
          </div>

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
            <div className="min-w-0 space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step Definition (YAML)</h3>
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
                          <ResourceAccessCard resourceType="step" resourceID={detail.id} label="step" />
                        ) : null}
                        {!canUpdateSelectedStep && !canCreateStepHere ? null : isGitSource ? (
                          canCreateStepHere ? (
                            <button className="glass-button-primary" onClick={openCloneModal}>
                              Clone
                            </button>
                          ) : null
                        ) : (
                          <>
                            {canUpdateSelectedStep ? (
                              <button className="glass-button-primary" onClick={() => setIsEditing(true)}>
                                Edit
                              </button>
                            ) : null}
                            {canCreateStepHere ? (
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
                              upsertStepDraftState({ id: detail.id, yaml: resetYaml });
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
                          aria-label="Step YAML editor"
                          aria-describedby="step-validation-status"
                          aria-invalid={validation.errors.length > 0}
                          aria-autocomplete="list"
                          aria-controls={editorSuggestion ? 'step-editor-autocomplete' : undefined}
                          aria-activedescendant={
                            editorSuggestion ? `step-editor-autocomplete-option-${editorSuggestion.activeIndex}` : undefined
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
                      <div
                        id="step-validation-status"
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
                          id="step-editor-autocomplete"
                          suggestion={editorSuggestion}
                          loading={autocompleteMeta.loading}
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
        <ResourceCollectionToolbar
          resourceLabel="step"
          activeFolder={activeFolder}
          searchTerm={searchTerm}
          canCreate={canCreateStepHere}
          onBack={() => openFolder(parentFolder(activeFolder))}
          onSearchTermChange={setSearchTerm}
          onCreate={openCreateModal}
        />
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
