import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import {
  createKnowledgeConnection,
  deleteKnowledgeConnection,
  deleteKnowledgeContext,
  fetchKnowledgeContext,
  fetchKnowledgeConnections,
  fetchKnowledgeContexts,
  resolveKnowledgeConnectionPage,
  saveKnowledgeContext,
  searchKnowledgeConnectionPages,
  syncKnowledgeContext,
  testKnowledgeConnection,
  updateKnowledgeConnection,
} from '../features/knowledge-context/api';
import { copyTextToClipboard } from '../lib/clipboard';
import {
  KNOWLEDGE_CONTEXTS_CHANGED_EVENT,
  buildKnowledgeConnectionTeamSummaries,
  buildKnowledgeID,
  buildKnowledgeTeamOptions,
  buildKnowledgeTree,
  clearKnowledgeDraft,
  collectKnowledgeTeamDocs,
  decodeKnowledgeRouteID,
  deriveKnowledgeConnectionName,
  deriveIdentityFromTeam,
  emptyKnowledgeDraft,
  encodeKnowledgeID,
  findKnowledgeTeam,
  isExternalKnowledgeDocument,
  isGitManagedDocument,
  knowledgeDocumentTreePathFromID,
  knowledgeConnectionIdentifier,
  knowledgeConnectionMatchesIdentifier,
  knowledgeTreePathToTeam,
  loadKnowledgeDraft,
  normalizeKnowledgeConnectionProvider,
  normalizeTeamPath,
  normalizeKnowledgeWorkspaceTab,
  saveKnowledgeDraft,
  splitKnowledgeContentForPreview,
  splitKnowledgePath,
  summarizeKnowledgeWorkspace,
  validateKnowledgeExternalDraft,
  validateKnowledgeConnectionDraft,
  validateKnowledgeIdentity,
  type KnowledgeConnectionListItem,
  type KnowledgeConnectionProvider,
  type KnowledgeContextDetail,
  type KnowledgeContextListItem,
} from '../features/knowledge-context/model';
import {
  KnowledgeContextModals,
  type KnowledgeConnectionModalState,
  type KnowledgeDeleteModalState,
  type KnowledgeFormModalState,
} from '../features/knowledge-context/KnowledgeContextModals';
import { KnowledgeContextWorkspace } from '../features/knowledge-context/KnowledgeContextWorkspace';
import { fetchResourceTeamPaths, isGlobalResourceTeamPath } from '../lib/resourceTeams';
import { TEAM_ROUTE_SEGMENT, decodeTeamRouteSegments, teamScopedRoute } from '../lib/teamRoutes';

type KnowledgeContextPageProps = {
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  canWriteKnowledgeConnections?: boolean;
  canDeleteKnowledgeConnections?: boolean;
};

function downloadKnowledgeContextFile(detail: Pick<KnowledgeContextDetail, 'name'>, content: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${detail.name || 'knowledge-context'}.txt`;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

function normalizeKnowledgeFormTeamPath(value: string) {
  const normalized = normalizeTeamPath(value);
  return isGlobalResourceTeamPath(normalized) ? '' : normalized;
}

export default function KnowledgeContextPage({
  canWriteKnowledge,
  canDeleteKnowledge,
  canWriteKnowledgeConnections,
  canDeleteKnowledgeConnections,
}: KnowledgeContextPageProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const routeSegments = useMemo(() => location.pathname.split('/').filter(Boolean), [location.pathname]);
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search]);
  const isTeamRoute = routeSegments[1] === TEAM_ROUTE_SEGMENT;
  const selectedID = isTeamRoute ? '' : decodeKnowledgeRouteID(location.pathname);
  const toastCounterRef = useRef(0);
  const editSessionOriginalRef = useRef<{ detail: KnowledgeContextDetail; content: string } | null>(null);
  const pendingDraftRef = useRef<{ detail: KnowledgeContextDetail; content: string } | null>(null);
  const pendingEditIDRef = useRef<string | null>(null);
  const [items, setItems] = useState<KnowledgeContextListItem[]>([]);
  const [connections, setConnections] = useState<KnowledgeConnectionListItem[]>([]);
  const [resourceTeamPaths, setResourceTeamPaths] = useState<string[]>([]);
  const [listLoading, setListLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [connectionsLoading, setConnectionsLoading] = useState(false);
  const [connectionsError, setConnectionsError] = useState<string | null>(null);
  const [detail, setDetail] = useState<KnowledgeContextDetail | null>(null);
  const [editorValue, setEditorValue] = useState('');
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [search, setSearch] = useState('');
  const [draftID, setDraftID] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [formModal, setFormModal] = useState<KnowledgeFormModalState | null>(null);
  const [connectionModal, setConnectionModal] = useState<KnowledgeConnectionModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<KnowledgeDeleteModalState | null>(null);
  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    toastCounterRef.current += 1;
    const id = toastCounterRef.current;
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const loadList = useCallback(async () => {
    setListLoading(true);
    setListError(null);
    try {
      setItems(await fetchKnowledgeContexts());
      window.dispatchEvent(new Event(KNOWLEDGE_CONTEXTS_CHANGED_EVENT));
    } catch (err) {
      setListError(err instanceof Error ? err.message : 'Unable to load knowledge contexts');
    } finally {
      setListLoading(false);
    }
  }, []);

  const loadConnections = useCallback(async () => {
    setConnectionsLoading(true);
    setConnectionsError(null);
    try {
      setConnections(await fetchKnowledgeConnections());
    } catch (err) {
      setConnectionsError(err instanceof Error ? err.message : 'Unable to load knowledge connections');
    } finally {
      setConnectionsLoading(false);
    }
  }, []);

  const loadResourceTeamPaths = useCallback(async () => {
    setResourceTeamPaths(await fetchResourceTeamPaths());
  }, []);

  useEffect(() => {
    void loadList();
    void loadConnections();
    void loadResourceTeamPaths();
  }, [loadConnections, loadList, loadResourceTeamPaths]);

  useEffect(() => {
    if (!selectedID) {
      setDetail(null);
      setEditorValue('');
      setDraftID(null);
      setIsEditing(false);
      editSessionOriginalRef.current = null;
      pendingDraftRef.current = null;
      return;
    }
    const pendingDraft = pendingDraftRef.current;
    const originalDraft = editSessionOriginalRef.current;
    const storedDraft = loadKnowledgeDraft(selectedID);
    const activeDraft =
      pendingDraft?.detail.id === selectedID
        ? pendingDraft
        : originalDraft?.detail.id === selectedID && !originalDraft.detail.uuid
          ? originalDraft
          : storedDraft;
    if (activeDraft) {
      setDetail(activeDraft.detail);
      setEditorValue(activeDraft.content);
      setDraftID(selectedID);
      setIsEditing(true);
      editSessionOriginalRef.current = activeDraft;
      setDetailLoading(false);
      setDetailError(null);
      return;
    }
    if (draftID === selectedID) {
      setDetailLoading(false);
      setDetailError(null);
      return;
    }
    let cancelled = false;
    setDetailLoading(true);
    setDetailError(null);
    fetchKnowledgeContext(selectedID)
      .then(payload => {
        if (cancelled) return;
        const content = typeof payload?.content === 'string' ? payload.content : '';
        const startInEditMode = pendingEditIDRef.current === payload.id;
        if (startInEditMode) pendingEditIDRef.current = null;
        setDetail(payload);
        setEditorValue(content);
        setDraftID(null);
        setIsEditing(startInEditMode);
        editSessionOriginalRef.current = startInEditMode ? { detail: { ...payload }, content } : null;
      })
      .catch(err => {
        if (!cancelled) {
          setDetail(null);
          setDetailError(err instanceof Error ? err.message : 'Unable to load document');
        }
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [draftID, selectedID]);

  useEffect(() => {
    if (!draftID || !detail || detail.uuid || detail.id !== draftID) return;
    saveKnowledgeDraft({ detail, content: editorValue });
  }, [detail, draftID, editorValue]);

  const filteredItems = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return items;
    return items.filter(item =>
      [item.kind, item.team, item.name, item.description, item.source]
        .some(value => (value || '').toLowerCase().includes(term))
    );
  }, [items, search]);

  const activeTeam = useMemo(() => {
    const routeTeam = isTeamRoute ? decodeTeamRouteSegments(routeSegments.slice(2)) : '';
    const selectedTeam = selectedID ? knowledgeDocumentTreePathFromID(selectedID) : '';
    return normalizeTeamPath(routeTeam || searchParams.get('team') || selectedTeam || '');
  }, [isTeamRoute, routeSegments, searchParams, selectedID]);
  const activeWorkspaceTab = normalizeKnowledgeWorkspaceTab(searchParams.get('tab'));
  const knowledgeTree = useMemo(() => buildKnowledgeTree(items, resourceTeamPaths), [items, resourceTeamPaths]);
  const activeTeamNode = useMemo(() => findKnowledgeTeam(knowledgeTree, activeTeam), [activeTeam, knowledgeTree]);
  const activeTeamDocuments = useMemo(() => collectKnowledgeTeamDocs(activeTeamNode), [activeTeamNode]);
  const hasDocumentFilters = Boolean(search.trim());
  const collectionDocuments = hasDocumentFilters ? filteredItems : activeTeamDocuments;
  const workspaceMetrics = useMemo(() => summarizeKnowledgeWorkspace(items), [items]);
  const activeConnectionTeam = useMemo(() => knowledgeTreePathToTeam(activeTeam), [activeTeam]);
  const connectionTreeTeams = useMemo(
    () => buildKnowledgeConnectionTeamSummaries(items, resourceTeamPaths, connections),
    [connections, items, resourceTeamPaths]
  );
  const connectionTeams = useMemo(() => {
    const summaries = connectionTreeTeams;
    if (!activeConnectionTeam) return summaries;
    return summaries.filter(summary => summary.teamPath === activeConnectionTeam || summary.teamPath.startsWith(`${activeConnectionTeam}/`));
  }, [activeConnectionTeam, connectionTreeTeams]);
  const teamOptions = useMemo(() => {
    return buildKnowledgeTeamOptions({
      activeTeam,
      activeConnectionTeam,
      resourceTeamPaths,
      items,
      connections,
    });
  }, [activeConnectionTeam, activeTeam, connections, items, resourceTeamPaths]);
  const defaultKnowledgeTeam = teamOptions[0] || '';
  const previewDocument = useMemo(() => splitKnowledgeContentForPreview(editorValue), [editorValue]);
  const previewContent = previewDocument.content;

  const contentMetrics = useMemo(() => {
    const trimmed = previewContent.trim();
    return {
      lines: previewContent ? previewContent.split('\n').length : 0,
      words: trimmed ? trimmed.split(/\s+/).filter(Boolean).length : 0,
      chars: previewContent.length,
    };
  }, [previewContent]);

  const openTeam = useCallback(
    (team: string) => {
      const normalized = normalizeTeamPath(team);
      navigate(teamScopedRoute('/knowledge-context', normalized));
    },
    [navigate]
  );

  const switchWorkspaceTab = useCallback(
    (tab: 'documents' | 'connections') => {
      const params = new URLSearchParams(location.search);
      if (tab === 'connections') {
        params.set('tab', 'connections');
      } else {
        params.delete('tab');
      }
      const query = params.toString();
      const targetPath = selectedID ? teamScopedRoute('/knowledge-context', activeTeam) : location.pathname;
      navigate(`${targetPath}${query ? `?${query}` : ''}`);
    },
    [activeTeam, location.pathname, location.search, navigate, selectedID]
  );

  const openConnectionTeam = useCallback(
    (teamPath: string) => {
      const target = teamScopedRoute('/knowledge-context', normalizeTeamPath(teamPath));
      navigate(`${target}?tab=connections`);
    },
    [navigate]
  );

  const handleConnectionSetupRequest = useCallback(
    (teamPath: string) => {
      if (!(canWriteKnowledgeConnections ?? canWriteKnowledge)) {
        addToast('You have read-only access to knowledge connections.', 'info');
        return;
      }
      const requestedTeam = normalizeTeamPath(teamPath || activeConnectionTeam);
      const team = requestedTeam
        ? normalizeKnowledgeFormTeamPath(requestedTeam)
        : normalizeKnowledgeFormTeamPath(defaultKnowledgeTeam);
      setConnectionModal({
        mode: 'create',
        team,
        provider: 'notion',
        name: '',
        display_name: '',
        base_url: '',
        credential_ref: '',
        pending: false,
      });
    },
    [activeConnectionTeam, addToast, canWriteKnowledge, canWriteKnowledgeConnections, defaultKnowledgeTeam]
  );

  const handleEditConnection = useCallback(
    (connection: KnowledgeConnectionListItem) => {
      if (!(canWriteKnowledgeConnections ?? canWriteKnowledge)) {
        addToast('You have read-only access to knowledge connections.', 'info');
        return;
      }
      const provider = normalizeKnowledgeConnectionProvider(connection.provider) as KnowledgeConnectionProvider;
      setConnectionModal({
        mode: 'edit',
        id: knowledgeConnectionIdentifier(connection),
        team: connection.team,
        provider: provider === 'notion' || provider === 'confluence' || provider === 'wiki' ? provider : 'wiki',
        name: connection.name,
        display_name: connection.display_name || connection.name,
        base_url: connection.base_url || '',
        credential_ref: '',
        disabled: connection.disabled,
        pending: false,
      });
    },
    [addToast, canWriteKnowledge, canWriteKnowledgeConnections]
  );

  const submitConnectionModal = useCallback(async () => {
    if (!connectionModal) return;
    const normalizedConnection = {
      ...connectionModal,
      team: normalizeKnowledgeFormTeamPath(connectionModal.team),
      name: connectionModal.name.trim() || deriveKnowledgeConnectionName(connectionModal.display_name || connectionModal.provider),
      display_name: connectionModal.display_name.trim(),
      base_url: connectionModal.base_url.trim(),
      credential_ref: connectionModal.credential_ref.trim(),
    };
    const normalizedID = `${normalizedConnection.team}/${normalizedConnection.name}`;
    const currentID = normalizedConnection.mode === 'edit' ? normalizedConnection.id || normalizedID : '';
    const error = validateKnowledgeConnectionDraft(normalizedConnection, connections, currentID);
    if (error) {
      setConnectionModal(prev => (prev ? { ...prev, error } : prev));
      return;
    }
    setConnectionModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      const savedConnection =
        normalizedConnection.mode === 'edit'
          ? await updateKnowledgeConnection(
              connections.find(connection => knowledgeConnectionMatchesIdentifier(connection, currentID)) || {
                id: normalizedID,
                team: normalizedConnection.team,
                name: normalizedConnection.name,
                display_name: normalizedConnection.display_name,
                provider: normalizedConnection.provider,
                status: 'authentication_required',
                credential_visibility: 'not_configured',
              },
              normalizedConnection
            )
          : await createKnowledgeConnection({
              team: normalizedConnection.team,
              provider: normalizedConnection.provider,
              name: normalizedConnection.name,
              display_name: normalizedConnection.display_name,
              base_url: normalizedConnection.base_url,
              credential_ref: normalizedConnection.credential_ref,
            });
      setConnectionModal(null);
      await loadConnections();
      setFormModal(prev =>
        prev?.contentSource === 'external' && normalizeTeamPath(prev.team) === normalizeTeamPath(savedConnection.team)
          ? { ...prev, connection_id: savedConnection.id, error: undefined }
          : prev
      );
      addToast(normalizedConnection.mode === 'edit' ? 'Knowledge connection updated.' : 'Knowledge connection created.', 'success');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to create knowledge connection';
      setConnectionModal(prev => (prev ? { ...prev, pending: false, error: message } : prev));
      addToast(message, 'error');
    }
  }, [addToast, connectionModal, connections, loadConnections]);

  const handleTestConnection = useCallback(async (connection: KnowledgeConnectionListItem) => {
    try {
      const result = await testKnowledgeConnection(connection);
      await loadConnections();
      addToast(result.message || `Connection status: ${result.status}`, result.ok ? 'success' : 'info');
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Unable to test knowledge connection', 'error');
    }
  }, [addToast, loadConnections]);

  const handleDeleteConnection = useCallback(async (connection: KnowledgeConnectionListItem) => {
    if (!(canDeleteKnowledgeConnections ?? canDeleteKnowledge)) {
      addToast('You do not have permission to delete knowledge connections.', 'info');
      return;
    }
    try {
      const affected = connection.external_document_count ?? connection.document_count ?? 0;
      const confirmed = affected > 0
        ? window.confirm(`${connection.display_name || connection.name} is used by ${affected} knowledge context${affected === 1 ? '' : 's'}. Delete it and detach those contexts?`)
        : true;
      if (!confirmed) return;
      await deleteKnowledgeConnection(connection, affected > 0);
      await loadConnections();
      addToast('Knowledge connection deleted.', 'success');
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Unable to delete knowledge connection', 'error');
    }
  }, [addToast, canDeleteKnowledge, canDeleteKnowledgeConnections, loadConnections]);

  const handleToggleConnection = useCallback(async (connection: KnowledgeConnectionListItem) => {
    if (!(canWriteKnowledgeConnections ?? canWriteKnowledge)) {
      addToast('You have read-only access to knowledge connections.', 'info');
      return;
    }
    try {
      await updateKnowledgeConnection(connection, { disabled: !connection.disabled });
      await loadConnections();
      addToast(connection.disabled ? 'Knowledge connection enabled.' : 'Knowledge connection disabled.', 'success');
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Unable to update knowledge connection', 'error');
    }
  }, [addToast, canWriteKnowledge, canWriteKnowledgeConnections, loadConnections]);

  useEffect(() => {
    if (isTeamRoute || selectedID) return;
    const legacyTeam = normalizeTeamPath(searchParams.get('team') || '');
    if (!legacyTeam) return;
    navigate(teamScopedRoute('/knowledge-context', legacyTeam), { replace: true });
  }, [isTeamRoute, navigate, searchParams, selectedID]);

  const handleSelectDocument = useCallback(
    (id: string) => {
      navigate(`/knowledge-context/${encodeKnowledgeID(id)}`);
    },
    [navigate]
  );

  const handleBackToList = useCallback(() => {
    const team = detail ? splitKnowledgePath(detail.id).team : activeTeam;
    openTeam(team);
  }, [activeTeam, detail, openTeam]);

  const openCreateModal = useCallback(() => {
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    const identity = deriveIdentityFromTeam(activeTeam, defaultKnowledgeTeam);
    setFormModal({
      mode: 'create',
      contentSource: 'inline',
      kind: identity.kind,
      team: identity.team,
      name: '',
      description: '',
      connection_id: '',
      external_page_id: '',
      external_page_url: '',
      sync_mode: 'manual',
      failure_mode: 'fail',
      content: '',
      page_search_query: '',
      page_search_results: [],
      page_search_cursor: '',
      page_search_loading: false,
      page_resolving: false,
      page_preview: null,
      pending: false,
    });
  }, [activeTeam, addToast, canWriteKnowledge, defaultKnowledgeTeam]);

  const openCloneForDetail = useCallback((sourceDetail: KnowledgeContextDetail, content: string) => {
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    let candidateName = `${sourceDetail.name || 'document'}-copy`;
    let suffix = 2;
    while (items.some(item => item.id === buildKnowledgeID(sourceDetail.kind, sourceDetail.team, candidateName))) {
      candidateName = `${sourceDetail.name || 'document'}-copy-${suffix}`;
      suffix += 1;
    }
    setFormModal({
      mode: 'clone',
      contentSource: 'inline',
      kind: sourceDetail.kind,
      team: sourceDetail.team,
      name: candidateName,
      description: sourceDetail.description || '',
      content,
      pending: false,
    });
  }, [addToast, canWriteKnowledge, items]);

  const openCloneModal = useCallback(() => {
    if (!detail) {
      addToast('Select a knowledge context to clone.', 'info');
      return;
    }
    openCloneForDetail(detail, editorValue);
  }, [addToast, detail, editorValue, openCloneForDetail]);

  const submitFormModal = useCallback(async () => {
    if (!formModal || formModal.pending) return;
    const kind = formModal.kind.trim();
    const team = normalizeKnowledgeFormTeamPath(formModal.team);
    const name = formModal.name.trim().replace(/\.(yaml|yml)$/i, '');
    const error = validateKnowledgeIdentity(kind, team, name, items);
    if (error) {
      setFormModal(prev => (prev ? { ...prev, error } : prev));
      return;
    }
    const id = buildKnowledgeID(kind, team, name);
    const content = formModal.content || '';

    if (formModal.contentSource === 'external') {
      const externalError = validateKnowledgeExternalDraft({
        connection_id: formModal.connection_id || '',
        external_page_id: formModal.external_page_id || '',
        external_page_url: formModal.external_page_url || '',
        sync_mode: formModal.sync_mode || 'manual',
        failure_mode: formModal.failure_mode || 'fail',
        content,
      }, team, connections);
      if (externalError) {
        setFormModal(prev => (prev ? { ...prev, error: externalError } : prev));
        return;
      }
      const connection = connections.find(item => knowledgeConnectionMatchesIdentifier(item, formModal.connection_id || ''));
      const draft: KnowledgeContextDetail = {
        ...emptyKnowledgeDraft,
        id,
        kind,
        team,
        name,
        description: formModal.description || '',
        visibility: 'team',
        source: connection?.provider || 'wiki',
        content,
        managed_by_config_repo: false,
        used_by: [],
        used_by_count: 0,
        connection_id: connection?.uuid || formModal.connection_id || '',
        connection_ref: connection?.id || formModal.connection_id || '',
        external_provider: connection?.provider || 'wiki',
        external_page_id: formModal.external_page_id?.trim() || '',
        external_page_url: formModal.external_page_url?.trim() || '',
        sync_mode: formModal.sync_mode || 'manual',
        failure_mode: formModal.failure_mode || 'fail',
        sync_status: content.trim() ? 'cached' : 'not_synced',
        external_page_title: formModal.page_preview?.title || '',
        content_hash: formModal.page_preview?.hash || '',
      };
      setFormModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
      try {
        const payload = await saveKnowledgeContext(draft, content);
        setDetail(payload);
        setEditorValue(payload.content || '');
        setDraftID(null);
        setIsEditing(false);
        editSessionOriginalRef.current = null;
        pendingDraftRef.current = null;
        setDetailError(null);
        setFormModal(null);
        navigate(`/knowledge-context/${encodeKnowledgeID(payload.id)}`);
        await loadList();
        addToast('External knowledge context created.', 'success');
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Unable to create external knowledge context';
        setFormModal(prev => (prev ? { ...prev, pending: false, error: message } : prev));
        addToast(message, 'error');
      }
      return;
    }

    const draft: KnowledgeContextDetail = {
      ...emptyKnowledgeDraft,
      id,
      kind,
      team,
      name,
      description: formModal.description || '',
      visibility: 'team',
      source: 'database',
      content,
      managed_by_config_repo: false,
      used_by: [],
      used_by_count: 0,
    };
    setDetail(draft);
    setEditorValue(content);
    setDraftID(id);
    setIsEditing(true);
    editSessionOriginalRef.current = { detail: draft, content };
    pendingDraftRef.current = { detail: draft, content };
    saveKnowledgeDraft({ detail: draft, content });
    setDetailError(null);
    setFormModal(null);
    navigate(`/knowledge-context/${encodeKnowledgeID(id)}`);
    addToast(formModal.mode === 'clone' ? 'Draft knowledge context cloned.' : 'Draft knowledge context created.', 'success');
  }, [addToast, connections, formModal, items, loadList, navigate]);

  const startEditing = useCallback(() => {
    if (!detail) return;
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    if (isGitManagedDocument(detail)) {
      addToast('Editing saves a database override. A future GitOps sync can replace it from the repository.', 'info');
    }
    editSessionOriginalRef.current = { detail: { ...detail }, content: editorValue };
    setIsEditing(true);
  }, [addToast, canWriteKnowledge, detail, editorValue]);

  const discardEditing = useCallback(() => {
    const original = editSessionOriginalRef.current;
    if (original) {
      setDetail(original.detail);
      setEditorValue(original.content);
    }
    setIsEditing(false);
    setDetailError(null);
    editSessionOriginalRef.current = null;
  }, []);

  const saveDetail = useCallback(async () => {
    if (!detail || !canWriteKnowledge || saving) return;
    const isGitManaged = isGitManagedDocument(detail);
    const originalID = editSessionOriginalRef.current?.detail.id || draftID || detail.id;
    const normalizedTeam = normalizeKnowledgeFormTeamPath(detail.team);
    const normalizedName = detail.name.trim().replace(/\.(yaml|yml)$/i, '');
    const saveCandidate = {
      ...detail,
      id: buildKnowledgeID(detail.kind, normalizedTeam, normalizedName),
      team: normalizedTeam,
      name: normalizedName,
    };
    const validationError = validateKnowledgeIdentity(saveCandidate.kind, saveCandidate.team, saveCandidate.name, items, originalID);
    if (validationError) {
      setDetailError(validationError);
      addToast('Resolve the document identity before saving.', 'error');
      return;
    }
    if (isExternalKnowledgeDocument(saveCandidate)) {
      const externalError = validateKnowledgeExternalDraft({
        connection_id: saveCandidate.connection_ref || saveCandidate.connection_id || '',
        external_page_id: saveCandidate.external_page_id || '',
        external_page_url: saveCandidate.external_page_url || '',
        sync_mode: saveCandidate.sync_mode === 'before_run' || saveCandidate.sync_mode === 'periodic' ? saveCandidate.sync_mode : 'manual',
        failure_mode: saveCandidate.failure_mode === 'use_cached' || saveCandidate.failure_mode === 'skip' ? saveCandidate.failure_mode : 'fail',
        content: editorValue,
      }, saveCandidate.team, connections);
      if (externalError) {
        setDetailError(externalError);
        addToast('Resolve the external page settings before saving.', 'error');
        return;
      }
    }
    setSaving(true);
    setDetailError(null);
    try {
      const payload = await saveKnowledgeContext(saveCandidate, editorValue);
      if (originalID !== payload.id && !draftID) {
        await deleteKnowledgeContext(originalID);
      }
      setDetail(payload);
      setEditorValue(payload.content || '');
      setDraftID(null);
      setIsEditing(false);
      editSessionOriginalRef.current = null;
      pendingDraftRef.current = null;
      clearKnowledgeDraft(originalID);
      if (payload.id && payload.id !== originalID) clearKnowledgeDraft(payload.id);
      navigate(`/knowledge-context/${encodeKnowledgeID(payload.id)}`, { replace: true });
      await loadList();
      addToast(
        isGitManaged
          ? 'Knowledge context saved as a database override. A future GitOps sync can replace it from the repository.'
          : 'Knowledge context saved.',
        'success'
      );
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to save document';
      setDetailError(message);
      addToast(message, 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, canWriteKnowledge, connections, detail, draftID, editorValue, items, loadList, navigate, saving]);

  const openDeleteModal = useCallback(
    (doc: Pick<KnowledgeContextListItem, 'id' | 'name' | 'source'> & { managed_by_config_repo?: boolean }) => {
      if (!canDeleteKnowledge) {
        addToast('You do not have permission to delete knowledge contexts.', 'info');
        return;
      }
      setDeleteModal({
        id: doc.id,
        name: doc.name || doc.id,
        gitOpsManaged: isGitManagedDocument(doc),
        pending: false,
      });
    },
    [addToast, canDeleteKnowledge]
  );

  const confirmDelete = useCallback(async () => {
    if (!deleteModal) return;
    setSaving(true);
    setDetailError(null);
    setDeleteModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      if (deleteModal.id === draftID && !detail?.uuid) {
        setDetail(null);
        setEditorValue('');
        setDraftID(null);
        setIsEditing(false);
        editSessionOriginalRef.current = null;
        pendingDraftRef.current = null;
        clearKnowledgeDraft(deleteModal.id);
      } else {
        await deleteKnowledgeContext(deleteModal.id);
        await loadList();
      }
      setDeleteModal(null);
      addToast(
        deleteModal.gitOpsManaged
          ? 'Knowledge context database row deleted. The next GitOps sync can recreate it from the repository.'
          : 'Knowledge context deleted.',
        'success'
      );
      if (detail?.id === deleteModal.id) {
        openTeam(splitKnowledgePath(deleteModal.id).team);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to delete document';
      setDeleteModal(prev => (prev ? { ...prev, pending: false, error: message } : prev));
      addToast(message, 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, deleteModal, detail?.id, detail?.uuid, draftID, loadList, openTeam]);

  const handleCopy = useCallback(async () => {
    if (!detail) return;
    try {
      await copyTextToClipboard(previewContent);
      setDetailError(null);
      addToast('Content copied to clipboard.', 'success');
    } catch {
      setDetailError('Unable to copy document content.');
      addToast('Unable to copy content.', 'error');
    }
  }, [addToast, detail, previewContent]);

  const handleDownload = useCallback(() => {
    if (!detail) return;
    downloadKnowledgeContextFile(detail, previewContent);
  }, [detail, previewContent]);

  const handleDownloadDocument = useCallback(async (doc: KnowledgeContextListItem) => {
    try {
      if (detail?.id === doc.id) {
        downloadKnowledgeContextFile(detail, previewContent);
        return;
      }
      const payload = await fetchKnowledgeContext(doc.id);
      downloadKnowledgeContextFile(payload, splitKnowledgeContentForPreview(payload.content || '').content);
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Unable to export knowledge context', 'error');
    }
  }, [addToast, detail, previewContent]);

  const handleAccessChange = useCallback((access: { resource_id?: string; visibility?: string }) => {
    const visibility = access.visibility;
    const resourceID = access.resource_id || detail?.id;
    if (!visibility || !resourceID) return;
    setDetail(prev => (prev && prev.id === resourceID ? { ...prev, visibility } : prev));
    setItems(prev => prev.map(item => (item.id === resourceID ? { ...item, visibility } : item)));
  }, [detail?.id]);

  const canEditSelected = Boolean(detail && canWriteKnowledge);
  const canWriteConnections = canWriteKnowledgeConnections ?? canWriteKnowledge;
  const canDeleteConnections = canDeleteKnowledgeConnections ?? canDeleteKnowledge;
  const selectedCanEdit = Boolean(canEditSelected && isEditing);
  const handleDescriptionChange = useCallback((value: string) => {
    setDetail(current => (current ? { ...current, description: value } : current));
  }, []);
  const handleDetailPatch = useCallback((patch: Partial<KnowledgeContextDetail>) => {
    setDetail(current => (current ? { ...current, ...patch } : current));
  }, []);

  const handleSyncNow = useCallback(async () => {
    if (!detail || syncing) return;
    if (!isExternalKnowledgeDocument(detail)) {
      addToast('Only external page documents can be synchronized.', 'info');
      return;
    }
    setSyncing(true);
    setDetailError(null);
    try {
      const payload = await syncKnowledgeContext(detail.id);
      setDetail(payload);
      setEditorValue(payload.content || '');
      await loadList();
      if (payload.sync_error) {
        setDetailError(payload.sync_error);
        addToast(payload.sync_error, 'error');
      } else {
        addToast('Knowledge context synchronized from provider content.', 'success');
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to sync knowledge context';
      setDetailError(message);
      addToast(message, 'error');
    } finally {
      setSyncing(false);
    }
  }, [addToast, detail, loadList, syncing]);

  const handleSyncDocument = useCallback(async (doc: KnowledgeContextListItem) => {
    if (syncing) return;
    if (!isExternalKnowledgeDocument(doc)) {
      addToast('Only external page documents can be synchronized.', 'info');
      return;
    }
    if (detail?.id === doc.id) {
      await handleSyncNow();
      return;
    }
    setSyncing(true);
    try {
      const payload = await syncKnowledgeContext(doc.id);
      await loadList();
      if (payload.sync_error) {
        addToast(payload.sync_error, 'error');
      } else {
        addToast('Knowledge context synchronized from provider content.', 'success');
      }
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Unable to sync knowledge context', 'error');
    } finally {
      setSyncing(false);
    }
  }, [addToast, detail?.id, handleSyncNow, loadList, syncing]);

  const handleEditDocument = useCallback((doc: KnowledgeContextListItem) => {
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    if (detail?.id === doc.id) {
      startEditing();
      return;
    }
    pendingEditIDRef.current = doc.id;
    navigate(`/knowledge-context/${encodeKnowledgeID(doc.id)}`);
  }, [addToast, canWriteKnowledge, detail?.id, navigate, startEditing]);

  const handleCloneDocument = useCallback(async (doc: KnowledgeContextListItem) => {
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    try {
      const payload = detail?.id === doc.id && detail ? detail : await fetchKnowledgeContext(doc.id);
      const content = detail?.id === doc.id ? editorValue : payload.content || '';
      openCloneForDetail(payload, content);
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Unable to clone knowledge context', 'error');
    }
  }, [addToast, canWriteKnowledge, detail, editorValue, openCloneForDetail]);

  const selectedFormConnection = useMemo(() => {
    if (!formModal?.connection_id) return null;
    return connections.find(item => knowledgeConnectionMatchesIdentifier(item, formModal.connection_id || '')) || null;
  }, [connections, formModal?.connection_id]);

  const handleSearchProviderPages = useCallback(async (append = false) => {
    if (!formModal || !selectedFormConnection) {
      setFormModal(prev => (prev ? { ...prev, page_error: 'Choose a connection before searching provider pages.' } : prev));
      return;
    }
    const cursor = append ? formModal.page_search_cursor || '' : '';
    setFormModal(prev => (prev ? { ...prev, page_search_loading: true, page_error: undefined } : prev));
    try {
      const result = await searchKnowledgeConnectionPages(selectedFormConnection, formModal.page_search_query || '', cursor);
      setFormModal(prev => {
        if (!prev) return prev;
        const existing = append ? prev.page_search_results || [] : [];
        return {
          ...prev,
          page_search_results: [...existing, ...result.pages],
          page_search_cursor: result.next_cursor || '',
          page_search_loading: false,
          page_error: result.pages.length || append ? undefined : 'No provider pages matched the search.',
        };
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to search provider pages';
      setFormModal(prev => (prev ? { ...prev, page_search_loading: false, page_error: message } : prev));
      addToast(message, 'error');
    }
  }, [addToast, formModal, selectedFormConnection]);

  const handleResolveProviderPage = useCallback(async () => {
    if (!formModal || !selectedFormConnection) {
      setFormModal(prev => (prev ? { ...prev, page_error: 'Choose a connection before previewing a provider page.' } : prev));
      return;
    }
    setFormModal(prev => (prev ? { ...prev, page_resolving: true, page_error: undefined } : prev));
    try {
      const page = await resolveKnowledgeConnectionPage(selectedFormConnection, {
        page_id: formModal.external_page_id || '',
        page_url: formModal.external_page_url || '',
      });
      setFormModal(prev => prev ? {
        ...prev,
        external_page_id: page.id || prev.external_page_id,
        external_page_url: page.url || prev.external_page_url,
        content: page.text || prev.content,
        page_preview: page,
        page_resolving: false,
        page_error: undefined,
      } : prev);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to preview provider page';
      setFormModal(prev => (prev ? { ...prev, page_resolving: false, page_error: message } : prev));
      addToast(message, 'error');
    }
  }, [addToast, formModal, selectedFormConnection]);

  const handleSelectProviderPage = useCallback(async (page: { id: string; title: string; url?: string }) => {
    if (!selectedFormConnection) return;
    setFormModal(prev => prev ? { ...prev, external_page_id: page.id, external_page_url: page.url || prev.external_page_url, page_resolving: true, page_error: undefined } : prev);
    try {
      const preview = await resolveKnowledgeConnectionPage(selectedFormConnection, { page_id: page.id, page_url: page.url || '' });
      setFormModal(prev => prev ? {
        ...prev,
        external_page_id: preview.id || page.id,
        external_page_url: preview.url || page.url || prev.external_page_url,
        content: preview.text || prev.content,
        page_preview: preview,
        page_resolving: false,
      } : prev);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to preview provider page';
      setFormModal(prev => (prev ? { ...prev, page_resolving: false, page_error: message } : prev));
      addToast(message, 'error');
    }
  }, [addToast, selectedFormConnection]);

  const handleOpenPipeline = useCallback(
    (pipelineID: string) => {
      navigate(`/pipelines/${pipelineID.split('/').map(encodeURIComponent).join('/')}`);
    },
    [navigate]
  );

  return (
    <div data-page="knowledge-context" className="active h-full flex flex-col">
      <div className="flex-1 overflow-auto px-4 pb-6 triggers-content">
        <KnowledgeContextWorkspace
          activeTeam={activeTeam}
          activeConnectionTeam={activeConnectionTeam}
          activeTab={activeWorkspaceTab}
          treeRoot={knowledgeTree}
          metrics={workspaceMetrics}
          connectionTeams={connectionTeams}
          connectionTreeTeams={connectionTreeTeams}
          listLoading={activeWorkspaceTab === 'connections' ? connectionsLoading : listLoading}
          listError={activeWorkspaceTab === 'connections' ? connectionsError : listError}
          search={search}
          collectionDocuments={collectionDocuments}
          selectedID={selectedID}
          detailLoading={detailLoading}
          selectedDetail={{
            detail,
            editorValue,
            previewContent,
            contentMetrics,
            detailError,
            draftID,
            isEditing,
            canEditSelected,
            selectedCanEdit,
            canWriteKnowledge,
            canDeleteKnowledge,
            saving,
            syncing,
            connections,
            teamOptions,
            onBackToList: handleBackToList,
            onCopy: handleCopy,
            onDownload: handleDownload,
            onStartEditing: startEditing,
            onClone: openCloneModal,
            onDiscardEditing: discardEditing,
            onSave: saveDetail,
            onSyncNow: handleSyncNow,
            onDelete: openDeleteModal,
            onDescriptionChange: handleDescriptionChange,
            onDetailPatch: handleDetailPatch,
            onContentChange: setEditorValue,
            onAccessChange: handleAccessChange,
            onOpenPipeline: handleOpenPipeline,
            onCreateDocument: openCreateModal,
          }}
          canWriteKnowledge={activeWorkspaceTab === 'connections' ? canWriteConnections : canWriteKnowledge}
          canDeleteKnowledge={activeWorkspaceTab === 'connections' ? canDeleteConnections : canDeleteKnowledge}
          onSearchChange={setSearch}
          onSwitchTab={switchWorkspaceTab}
          onOpenTeam={openTeam}
          onSelectConnectionTeam={openConnectionTeam}
          onSelectDocument={handleSelectDocument}
          onDownloadDocument={handleDownloadDocument}
          onSyncDocument={handleSyncDocument}
          onEditDocument={handleEditDocument}
          onCloneDocument={handleCloneDocument}
          onAccessChange={handleAccessChange}
          onDeleteDocument={openDeleteModal}
          onCreateDocument={openCreateModal}
          onAddConnection={handleConnectionSetupRequest}
          onTestConnection={handleTestConnection}
          onEditConnection={handleEditConnection}
          onToggleConnection={handleToggleConnection}
          onDeleteConnection={handleDeleteConnection}
        />
      </div>

      <KnowledgeContextModals
        formModal={formModal}
        deleteModal={deleteModal}
        connectionModal={connectionModal}
        connections={connections}
        teamOptions={teamOptions}
        onCloseForm={() => setFormModal(null)}
        onUpdateForm={patch => setFormModal(prev => (prev ? { ...prev, ...patch, error: undefined } : prev))}
        onSubmitForm={submitFormModal}
        onSearchPages={() => void handleSearchProviderPages(false)}
        onLoadMorePages={() => void handleSearchProviderPages(true)}
        onResolvePage={() => void handleResolveProviderPage()}
        onSelectPage={page => void handleSelectProviderPage(page)}
        onCloseDelete={() => setDeleteModal(null)}
        onConfirmDelete={confirmDelete}
        onCloseConnection={() => setConnectionModal(null)}
        onUpdateConnection={patch => setConnectionModal(prev => (prev ? { ...prev, ...patch, error: undefined } : prev))}
        onSubmitConnection={submitConnectionModal}
        onAddConnectionFromForm={handleConnectionSetupRequest}
      />

      <WorkflowToastRegion toasts={toasts} />
    </div>
  );
}
