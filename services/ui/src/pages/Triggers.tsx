import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type UIEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import * as yaml from 'js-yaml';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { TreeColumnResizeHandle, useResizableTreeColumn } from '../components/resizableTreeColumn';
import {
  buildTriggerEditorSuggestion,
  type TriggerEditorSuggestion,
} from '../features/triggers/editorAutocomplete';
import { TriggerCollectionList } from '../features/triggers/TriggerCollectionList';
import { TriggerCollectionToolbar } from '../features/triggers/TriggerCollectionToolbar';
import { TriggerDetailView } from '../features/triggers/TriggerDetailView';
import { TriggerWorkflowModals } from '../features/triggers/TriggerWorkflowModals';
import { fetchGitWebhookSources } from '../features/git-webhook-sources/api';
import {
  fetchTriggerAutocompleteResources,
  fetchTriggerDetail,
  fetchTriggerPipelineYaml,
  fetchTriggerRuns,
  fetchTriggers,
} from '../features/triggers/api';
import { TriggerExplorerTree } from '../features/triggers/TriggerExplorerTree';
import { useTriggerManifestMutations } from '../features/triggers/useTriggerManifestMutations';
import { useTriggerPermissions } from '../features/triggers/useTriggerPermissions';
import {
  applyTriggerDetailsToYaml,
  asRecord,
  buildPipelineIdentifierFromRun,
  encodeTriggerSlug,
  filterTriggerListItems,
  normalizeTriggerTeamPath,
  normalizePipelineIdentifier,
  normalizeScopeLabel,
  normalizeSource,
  sourceLabel,
  splitTriggerSlug,
  triggerDetailsFormFromYaml,
  triggerBelongsToOwner,
  validateTriggerYaml,
  type TriggerDetailsFormState,
  type PipelineMeta,
  type PipelineRef,
  type TriggerDetail,
  type TriggerListItem,
  type TriggerRun,
  type TriggerSourceFilter,
  type TriggerWebhookSourceOption,
} from '../features/triggers/model';
import { buildTriggerTree, findTriggerTreeNode } from '../features/triggers/treeModel';
import { fetchResourceTeamPaths } from '../lib/resourceTeams';
import { buildPipelineRunsRoute } from '../lib/teamRoutes';

const INITIAL_RECENT_RUNS = 5;
const RUNS_PAGE_SIZE = 10;
const RUNS_CACHE_TTL = 60 * 1000;
const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;
const LEGACY_TRIGGER_TEAM_ROUTE_SEGMENT = 'team';

function normalizeTriggerOwnerPath(value?: string | null) {
  return (value || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

function decodeTriggerOwnerSegments(segments: string[]) {
  return normalizeTriggerOwnerPath(
    segments
      .filter(Boolean)
      .map(segment => {
        try {
          return decodeURIComponent(segment);
        } catch {
          return segment;
        }
      })
      .join('/')
  );
}

function ownerScopedTriggerRoute(ownerPath?: string | null) {
  const owner = normalizeTriggerOwnerPath(ownerPath);
  if (!owner) return '/triggers';
  const params = new URLSearchParams({ owner });
  return `/triggers?${params.toString()}`;
}

function TriggersPage({
  canDeleteTriggers = false,
}: {
  canDeleteTriggers?: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();

  const [serverTriggers, setServerTriggers] = useState<TriggerListItem[]>([]);
  const [teamPathOptions, setTeamPathOptions] = useState<string[]>(['root']);
  const [webhookSourceOptions, setWebhookSourceOptions] = useState<TriggerWebhookSourceOption[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [activeOwner, setActiveOwner] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [sourceFilter, setSourceFilter] = useState<TriggerSourceFilter>('all');
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
  const treeResize = useResizableTreeColumn({
    storageKey: 'triggers',
    defaultWidth: 280,
    minWidth: 240,
    maxWidth: 520,
  });

  const [autocompleteMeta, setAutocompleteMeta] = useState<{
    pipelines: string[];
    scopes: string[];
    fetchedAt: number;
    loading: boolean;
  }>({ pipelines: [], scopes: [], fetchedAt: 0, loading: false });

  const [editorSuggestion, setEditorSuggestion] = useState<TriggerEditorSuggestion | null>(null);
  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const validation = useMemo(() => {
    if (!isEditing) return { errors: [] };
    return validateTriggerYaml(editorValue);
  }, [editorValue, isEditing]);

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
      setEditorSuggestion(buildTriggerEditorSuggestion({
        text,
        cursor,
        force: opts?.force,
        metadata: autocompleteMeta,
      }));
    },
    [autocompleteMeta, editorValue]
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
    setIsEditing(false);
  }, [detail]);

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

      const { owner, repo } = splitTriggerSlug(slug);
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

  const ownerForSlug = (slug: string) => {
    const parts = slug.split('/').filter(Boolean);
    parts.pop();
    return parts.join('/');
  };

  const openOwner = (path: string) => {
    const cleaned = normalizeTriggerOwnerPath(path);
    setActiveOwner(cleaned);
    setSelectedSlug(null);
    selectedSlugRef.current = null;
    navigate(ownerScopedTriggerRoute(cleaned));
  };

  const handleSelectSlug = useCallback((slug: string) => {
    selectedSlugRef.current = slug;
    setSelectedSlug(slug);
    navigate(`/triggers/${encodeTriggerSlug(slug)}`);
  }, [navigate]);

  const handleBackToList = () => {
    if (!detail) {
      navigate('/triggers');
      return;
    }
    openOwner(ownerForSlug(detail.slug));
  };

  const openEditModal = useCallback(() => {
    if (!detail) return;
    setEditorSuggestion(null);
    setEditorValue(detail.rawYaml);
    setIsEditing(true);
  }, [detail]);

  const permissionOwner = selectedSlug ? ownerForSlug(selectedSlug) : activeOwner;
  const selectedListItem = selectedSlug ? serverTriggers.find(item => item.slug === selectedSlug) : undefined;
  const workspaceTeamPath = selectedSlug
    ? normalizeTriggerTeamPath(selectedListItem?.teamPath || detail?.teamPath)
    : 'root';
  const {
    canCreateTriggerHere,
    canUpdateSelectedTrigger,
  } = useTriggerPermissions(permissionOwner, selectedSlug);

  const handleTriggerSaved = useCallback((updated: TriggerDetail) => {
    setDetail(updated);
    setLinkedPipelines(updated.summary.pipelines);
  }, []);

  const handleTriggerDetailsChange = useCallback((nextDetails: TriggerDetailsFormState) => {
    setEditorValue(current => applyTriggerDetailsToYaml(current || detail?.rawYaml || '', nextDetails));
  }, [detail]);

  const handleEditYamlPreviewChange = useCallback((nextValue: string) => {
    setEditorSuggestion(null);
    setEditorValue(nextValue);
  }, []);

  const handleTriggerMutationSelect = useCallback((slug: string) => {
    autoEnterEditSlugRef.current = slug;
    handleSelectSlug(slug);
  }, [handleSelectSlug]);

  const handleTriggerDeleted = useCallback(() => {
    setSelectedSlug(null);
    selectedSlugRef.current = null;
    navigate('/triggers');
  }, [navigate]);

  const {
    cloneModal,
    closeCloneModal,
    closeCreateModal,
    closeDeleteModal,
    confirmDelete,
    createModal,
    deleteModal,
    openCloneModal,
    openCreateModal,
    openDeleteModal,
    save: handleSave,
    saving,
    submitCloneModal,
    submitCreateModal,
    updateCloneDetails,
    updateCloneRepository,
    updateCloneYamlPreview,
    updateCreateDetails,
    updateCreateRepository,
    updateCreateYamlPreview,
  } = useTriggerManifestMutations({
    canCreateTriggerHere,
    canUpdateSelectedTrigger,
    canDeleteTriggers,
    permissionOwner,
    detail,
    editorValue,
    validationErrorCount: validation.errors.length,
    serverTriggers,
    defaultTeamPath: workspaceTeamPath,
    addToast,
    loadTriggers,
    loadRecentRuns,
    onSelectSlug: handleTriggerMutationSelect,
    onSaved: handleTriggerSaved,
    onEditingFinished: () => setIsEditing(false),
    onDeleted: handleTriggerDeleted,
  });

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

  useEffect(() => {
    void loadTriggers();
  }, [loadTriggers]);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([
      fetchResourceTeamPaths(),
      fetchGitWebhookSources(),
    ]).then(([teamPaths, webhookSources]) => {
      if (cancelled) return;
      setTeamPathOptions(teamPaths);
      setWebhookSourceOptions(
        webhookSources.map(source => ({
          id: source.id,
          name: source.name,
          provider: source.provider,
          teamPath: source.team_path || source.run_team_path,
          visibility: source.visibility,
        }))
      );
    }).catch(() => {
      if (cancelled) return;
      setTeamPathOptions(['root']);
      setWebhookSourceOptions([]);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'triggers') return;
    const isLegacyTeamRoute = segments[1] === LEGACY_TRIGGER_TEAM_ROUTE_SEGMENT;
    if (isLegacyTeamRoute) {
      setSelectedSlug(null);
      selectedSlugRef.current = null;
    } else if (segments.length > 1) {
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
    const routeOwner = isLegacyTeamRoute ? decodeTriggerOwnerSegments(segments.slice(2)) : '';
    const owner = normalizeTriggerOwnerPath(routeOwner || params.get('owner') || params.get('team') || '');
    setActiveOwner(owner);
    if (isLegacyTeamRoute || (segments.length === 1 && params.get('team'))) {
      navigate(ownerScopedTriggerRoute(owner), { replace: true });
    }
  }, [location.pathname, location.search, navigate]);

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
    return filterTriggerListItems(serverTriggers, { query: searchTerm, source: sourceFilter });
  }, [serverTriggers, searchTerm, sourceFilter]);

  const triggerTeamPaths = useMemo(() => {
    const paths = [...teamPathOptions, ...serverTriggers.map(item => normalizeTriggerTeamPath(item.teamPath))]
      .filter(Boolean);
    return Array.from(new Set(['root', ...paths])).sort((left, right) => {
      if (left === 'root') return -1;
      if (right === 'root') return 1;
      return left.localeCompare(right);
    });
  }, [serverTriggers, teamPathOptions]);

  const workspaceOwner = selectedSlug ? ownerForSlug(selectedSlug) : activeOwner;
  const triggerDetails = useMemo(
    () => triggerDetailsFormFromYaml(editorValue || detail?.rawYaml || '', detail),
    [detail, editorValue]
  );

  const visibleTriggers = useMemo(() => {
    const list = searchTerm.trim()
      ? filteredTriggers
      : filteredTriggers.filter(item => triggerBelongsToOwner(item.slug, workspaceOwner));
    return [...list].sort((a, b) => a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' }));
  }, [filteredTriggers, searchTerm, workspaceOwner]);

  const buildTree = useMemo(() => {
    return buildTriggerTree(serverTriggers);
  }, [serverTriggers]);

  const activeOwnerNode = useMemo(() => {
    return findTriggerTreeNode(buildTree, workspaceOwner);
  }, [workspaceOwner, buildTree]);

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

  return (
    <div data-page="triggers" className="active h-full flex flex-col">
      <TriggerCollectionToolbar
        searchTerm={searchTerm}
        sourceFilter={sourceFilter}
        searchOpen={searchOpen}
        searchInputRef={searchInputRef}
        canCreateTriggerHere={canCreateTriggerHere}
        onSearchTermChange={setSearchTerm}
        onSourceFilterChange={setSourceFilter}
        onSearchOpenChange={setSearchOpen}
        onCreate={openCreateModal}
      />

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {selectedSlug ? (
          <section className="triggers-detail-fullscreen triggers-detail-fullscreen--with-tree" style={treeResize.gridStyle} aria-label="Trigger detail">
            <TriggerExplorerTree
              rootNode={buildTree}
              allTriggers={serverTriggers}
              activeOwner={workspaceOwner}
              selectedSlug={selectedSlug}
              onOpenOwner={openOwner}
              onSelectTrigger={handleSelectSlug}
            />
            <TreeColumnResizeHandle {...treeResize} label="Resize trigger tree" />
            <div className="triggers-detail-fullscreen-main">
              {selectedSlug && detailLoading ? (
                <div className="triggers-detail-pane-empty">Loading trigger...</div>
              ) : selectedSlug && detailError ? (
                <div className="triggers-detail-pane-empty triggers-workspace-empty--error">Failed to load trigger: {detailError}</div>
              ) : (
                <TriggerDetailView
                  detail={detail}
                  isEditing={false}
                  editorValue={editorValue}
                  validationErrors={validation.errors}
                  validationErrorLines={validationErrorLines}
                  editorSuggestion={editorSuggestion}
                  autocompleteLoading={autocompleteMeta.loading}
                  editorRef={editorRef}
                  highlightContentRef={highlightContentRef}
                  lineNumbersRef={lineNumbersRef}
                  canUpdateSelectedTrigger={canUpdateSelectedTrigger}
                  canCreateTriggerHere={canCreateTriggerHere}
                  canDeleteSelectedTrigger={canDeleteTriggers}
                  saving={saving}
                  triggerDetails={triggerDetails}
                  teamPaths={triggerTeamPaths}
                  webhookSources={webhookSourceOptions}
                  linkedPipelines={linkedPipelines}
                  pipelineMetadata={pipelineMetaCacheRef.current}
                  pipelineSourceIndex={pipelineSourceIndexRef.current}
                  recentRuns={recentRuns}
                  runsLoading={runsLoading}
                  runsError={runsError}
                  runsScrollable={recentRuns.length >= INITIAL_RECENT_RUNS}
                  recentRunsListRef={recentRunsListRef}
                  onBack={handleBackToList}
                  onOpenScope={scope => navigate(`/scopes/${scope ? encodeURIComponent(scope) : 'default'}`)}
                  onOpenPipeline={identifier => navigate(`/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`)}
                  onOpenRun={runId => {
                    navigate(`${buildPipelineRunsRoute('recent')}?run=${encodeURIComponent(runId)}`);
                  }}
                  onRecentRunsScroll={handleRecentRunsScroll}
                  onCopy={() => void handleCopyYaml()}
                  onDownload={handleDownloadYaml}
                  onEdit={openEditModal}
                  onClone={openCloneModal}
                  onDelete={() => openDeleteModal(detail?.slug || selectedSlug)}
                  onDiscard={discardEditorChanges}
                  onSave={() => void handleSave()}
                  onTriggerDetailsChange={handleTriggerDetailsChange}
                  onEditorTextChange={handleEditorTextChange}
                  onOpenSuggestion={openEditorSuggestion}
                  onMoveSuggestion={moveEditorSuggestion}
                  onDismissSuggestion={() => setEditorSuggestion(null)}
                  onSelectSuggestion={applyEditorSuggestion}
                  onEditorScroll={handleEditorScroll}
                  onIndentTab={handleIndentTab}
                  onAutoIndentEnter={handleAutoIndentEnter}
                />
              )}
            </div>
          </section>
        ) : (
          <div className="triggers-workspace-panel triggers-workspace-panel--trigger-browser triggers-workspace-panel--summary">
            <TriggerCollectionList
              listLoading={listLoading}
              listError={listError}
              allTriggers={serverTriggers}
              visibleTriggers={visibleTriggers}
              treeRoot={buildTree}
              activeOwnerNode={activeOwnerNode}
              activeOwner={workspaceOwner}
              searchTerm={searchTerm}
              selectedSlug={selectedSlug}
              canCreateTriggerHere={canCreateTriggerHere}
              canDeleteTriggers={canDeleteTriggers}
              onSelectTrigger={handleSelectSlug}
              onOpenOwner={openOwner}
              onDeleteTrigger={openDeleteModal}
            />
          </div>
        )}
      </div>

      <TriggerWorkflowModals
        createModal={createModal}
        editModal={isEditing && detail ? {
          slug: detail.slug,
          details: triggerDetails,
          yamlPreview: editorValue,
          validationErrors: validation.errors,
          pending: saving,
          gitOpsManaged: normalizeSource(detail.source) === 'git',
        } : null}
        cloneModal={cloneModal}
        deleteModal={deleteModal}
        canDeleteTriggers={canDeleteTriggers}
        selectedSlug={detail?.slug}
        teamPaths={triggerTeamPaths}
        webhookSources={webhookSourceOptions}
        onCloseCreate={closeCreateModal}
        onUpdateCreateRepository={updateCreateRepository}
        onUpdateCreateDetails={updateCreateDetails}
        onUpdateCreateYamlPreview={updateCreateYamlPreview}
        onSubmitCreate={() => void submitCreateModal()}
        onCloseEdit={discardEditorChanges}
        onUpdateEditDetails={handleTriggerDetailsChange}
        onUpdateEditYamlPreview={handleEditYamlPreviewChange}
        onSubmitEdit={() => void handleSave()}
        onCloseClone={closeCloneModal}
        onUpdateCloneRepository={updateCloneRepository}
        onUpdateCloneDetails={updateCloneDetails}
        onUpdateCloneYamlPreview={updateCloneYamlPreview}
        onSubmitClone={() => void submitCloneModal()}
        onCloseDelete={closeDeleteModal}
        onConfirmDelete={() => void confirmDelete()}
      />

      <WorkflowToastRegion toasts={toasts} classPrefix="triggers" />
    </div>
  );
}

export default TriggersPage;
