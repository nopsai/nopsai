import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  ArrowLeft,
  Copy,
  Download,
  Plus,
  Search,
  Trash2,
} from 'lucide-react';

import ResourceAccessCard from '../components/ResourceAccessCard';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import {
  deleteKnowledgeContext,
  fetchKnowledgeContext,
  fetchKnowledgeContexts,
  saveKnowledgeContext,
} from '../features/knowledge-context/api';
import {
  KNOWLEDGE_CONTEXTS_CHANGED_EVENT,
  buildKnowledgeID,
  buildKnowledgeTree,
  clearKnowledgeDraft,
  decodeKnowledgeRouteID,
  deriveIdentityFromFolder,
  emptyKnowledgeDraft,
  encodeKnowledgeID,
  findKnowledgeFolder,
  isGitManagedDocument,
  loadKnowledgeDraft,
  normalizeFolderPath,
  normalizeKnowledgeSource,
  parentFolder,
  saveKnowledgeDraft,
  splitKnowledgeContentForPreview,
  splitKnowledgePath,
  validateKnowledgeIdentity,
  type KnowledgeContextDetail,
  type KnowledgeContextListItem,
} from '../features/knowledge-context/model';
import { KnowledgeContextCollectionList } from '../features/knowledge-context/KnowledgeContextCollectionList';
import {
  KnowledgeContextModals,
  type KnowledgeDeleteModalState,
  type KnowledgeFormModalState,
} from '../features/knowledge-context/KnowledgeContextModals';
import { formatKnowledgeDate, kindIcon } from '../features/knowledge-context/presentation';
import { fetchResourceGroupPaths } from '../lib/resourceGroups';

type KnowledgeContextPageProps = {
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
};

export default function KnowledgeContextPage({ canWriteKnowledge, canDeleteKnowledge }: KnowledgeContextPageProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const selectedID = decodeKnowledgeRouteID(location.pathname);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const toastCounterRef = useRef(0);
  const editSessionOriginalRef = useRef<{ detail: KnowledgeContextDetail; content: string } | null>(null);
  const pendingDraftRef = useRef<{ detail: KnowledgeContextDetail; content: string } | null>(null);
  const [items, setItems] = useState<KnowledgeContextListItem[]>([]);
  const [listLoading, setListLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);
  const [detail, setDetail] = useState<KnowledgeContextDetail | null>(null);
  const [editorValue, setEditorValue] = useState('');
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [draftID, setDraftID] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [formModal, setFormModal] = useState<KnowledgeFormModalState | null>(null);
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

  useEffect(() => {
    void loadList();
  }, [loadList]);

  useEffect(() => {
    let cancelled = false;
    const loadGroups = async () => {
      const paths = await fetchResourceGroupPaths();
      if (!cancelled) setResourceGroupPaths(paths);
    };
    void loadGroups();
    const handleGroupsChanged = () => void loadGroups();
    window.addEventListener('nopsai-resource-groups-changed', handleGroupsChanged);
    return () => {
      cancelled = true;
      window.removeEventListener('nopsai-resource-groups-changed', handleGroupsChanged);
    };
  }, []);

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
        setDetail(payload);
        setEditorValue(typeof payload?.content === 'string' ? payload.content : '');
        setDraftID(null);
        setIsEditing(false);
        editSessionOriginalRef.current = null;
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
    return items.filter(item => [item.kind, item.group, item.name, item.description].some(value => (value || '').toLowerCase().includes(term)));
  }, [items, search]);

  const activeFolder = useMemo(() => normalizeFolderPath(new URLSearchParams(location.search).get('folder') || ''), [location.search]);
  const knowledgeTree = useMemo(() => buildKnowledgeTree(items, resourceGroupPaths), [items, resourceGroupPaths]);
  const activeFolderNode = useMemo(() => findKnowledgeFolder(knowledgeTree, activeFolder), [activeFolder, knowledgeTree]);
  const visibleDocuments = useMemo(() => {
    if (search.trim()) return filteredItems;
    return activeFolderNode.docs;
  }, [activeFolderNode.docs, filteredItems, search]);
  const visibleFolders = search.trim() ? [] : activeFolderNode.children;
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

  const openFolder = useCallback(
    (folder: string) => {
      const normalized = normalizeFolderPath(folder);
      navigate(normalized ? `/knowledge-context?folder=${encodeURIComponent(normalized)}` : '/knowledge-context');
    },
    [navigate]
  );

  const handleSelectDocument = useCallback(
    (id: string) => {
      navigate(`/knowledge-context/${encodeKnowledgeID(id)}`);
    },
    [navigate]
  );

  const handleBackToList = useCallback(() => {
    const folder = detail ? splitKnowledgePath(detail.id).folder : activeFolder;
    openFolder(folder);
  }, [activeFolder, detail, openFolder]);

  const openCreateModal = useCallback(() => {
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    const identity = deriveIdentityFromFolder(activeFolder);
    setFormModal({
      mode: 'create',
      kind: identity.kind,
      group: identity.group,
      name: '',
      content: '',
      pending: false,
    });
  }, [activeFolder, addToast, canWriteKnowledge]);

  const openCloneModal = useCallback(() => {
    if (!detail) {
      addToast('Select a knowledge context to clone.', 'info');
      return;
    }
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    let candidateName = `${detail.name || 'document'}-copy`;
    let suffix = 2;
    while (items.some(item => item.id === buildKnowledgeID(detail.kind, detail.group, candidateName))) {
      candidateName = `${detail.name || 'document'}-copy-${suffix}`;
      suffix += 1;
    }
    setFormModal({
      mode: 'clone',
      kind: detail.kind,
      group: detail.group,
      name: candidateName,
      content: editorValue,
      pending: false,
    });
  }, [addToast, canWriteKnowledge, detail, editorValue, items]);

  const submitFormModal = useCallback(() => {
    if (!formModal) return;
    const error = validateKnowledgeIdentity(formModal.kind, formModal.group, formModal.name, items);
    if (error) {
      setFormModal(prev => (prev ? { ...prev, error } : prev));
      return;
    }
    const kind = formModal.kind.trim();
    const group = normalizeFolderPath(formModal.group);
    const name = formModal.name.trim().replace(/\.(yaml|yml)$/i, '');
    const id = buildKnowledgeID(kind, group, name);
    const content = formModal.content || '';
    const draft: KnowledgeContextDetail = {
      ...emptyKnowledgeDraft,
      id,
      kind,
      group,
      name,
      description: '',
      visibility: 'group',
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
  }, [addToast, formModal, items, navigate]);

  const startEditing = useCallback(() => {
    if (!detail) return;
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    if (isGitManagedDocument(detail)) {
      addToast('Editing saves a database override. The next GitOps sync can replace it unless it is pushed to GitOps.', 'info');
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
    const validationError = validateKnowledgeIdentity(detail.kind, detail.group, detail.name, items, draftID || detail.id);
    if (validationError) {
      setDetailError(validationError);
      addToast('Resolve the document identity before saving.', 'error');
      return;
    }
    setSaving(true);
    setDetailError(null);
    try {
      const payload = await saveKnowledgeContext(detail, editorValue);
      setDetail(payload);
      setEditorValue(payload.content || '');
      setDraftID(null);
      setIsEditing(false);
      editSessionOriginalRef.current = null;
      pendingDraftRef.current = null;
      clearKnowledgeDraft(detail.id);
      if (payload.id && payload.id !== detail.id) clearKnowledgeDraft(payload.id);
      navigate(`/knowledge-context/${encodeKnowledgeID(payload.id)}`, { replace: true });
      await loadList();
      addToast(
        isGitManaged
          ? 'Knowledge context saved as a database override. The next GitOps sync can replace it unless it is pushed to GitOps.'
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
  }, [addToast, canWriteKnowledge, detail, draftID, editorValue, items, loadList, navigate, saving]);

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
        openFolder(splitKnowledgePath(deleteModal.id).folder);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to delete document';
      setDeleteModal(prev => (prev ? { ...prev, pending: false, error: message } : prev));
      addToast(message, 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, deleteModal, detail?.id, detail?.uuid, draftID, loadList, openFolder]);

  const handleCopy = useCallback(async () => {
    if (!detail) return;
    try {
      await navigator.clipboard.writeText(previewContent);
      setDetailError(null);
      addToast('Content copied to clipboard.', 'success');
    } catch {
      setDetailError('Unable to copy document content.');
      addToast('Unable to copy content.', 'error');
    }
  }, [addToast, detail, previewContent]);

  const handleDownload = useCallback(() => {
    if (!detail) return;
    const blob = new Blob([previewContent], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${detail.name || 'knowledge-context'}.txt`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }, [detail, previewContent]);

  const handleAccessChange = useCallback((access: { resource_id?: string; visibility?: string }) => {
    const visibility = access.visibility;
    const resourceID = access.resource_id || detail?.id;
    if (!visibility || !resourceID) return;
    setDetail(prev => (prev && prev.id === resourceID ? { ...prev, visibility } : prev));
    setItems(prev => prev.map(item => (item.id === resourceID ? { ...item, visibility } : item)));
  }, [detail?.id]);

  const canEditSelected = Boolean(detail && canWriteKnowledge);
  const selectedCanEdit = Boolean(canEditSelected && isEditing);
  const CurrentKindIcon = kindIcon(detail?.kind || '');

  const renderDetail = () => {
    if (!detail) {
      return (
        <div id="knowledge-context-detail-view" className="pipelines-view">
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a knowledge context to see details.</div>
        </div>
      );
    }

    const nameLabel = detail.name || detail.id;
    const descriptionLabel = (detail.description || '').trim();
    const source = normalizeKnowledgeSource(detail.source);
    const isDraftDocument = Boolean(draftID && !detail.uuid);
    const updatedLabel = isDraftDocument ? 'Unsaved' : formatKnowledgeDate(detail.updated_at);

    return (
      <div id="knowledge-context-detail-view" className="pipelines-view">
        <div className="min-w-0 space-y-6">
          <div className="glass-card p-6">
            <div className="flex items-start justify-between gap-4 w-full mb-4">
              <div className="min-w-0 flex items-start gap-3">
                <span className={`kc-document-header__icon kc-kind-mark kc-kind-mark--${detail.kind} mt-1`} aria-hidden="true">
                  <CurrentKindIcon className="h-5 w-5" />
                </span>
                <div className="min-w-0">
                  <h2 id="knowledge-context-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                    {nameLabel}
                  </h2>
                  {descriptionLabel ? (
                    <p id="knowledge-context-detail-description" className="text-sm text-[var(--text-secondary)] mt-1">
                      {descriptionLabel}
                    </p>
                  ) : null}
                  <dl className="kc-header-details">
                    <div>
                      <dt>Identifier</dt>
                      <dd>{detail.id}</dd>
                    </div>
                    <div>
                      <dt>Kind</dt>
                      <dd>{detail.kind}</dd>
                    </div>
                    <div>
                      <dt>Group</dt>
                      <dd>{detail.group || 'Root'}</dd>
                    </div>
                    <div>
                      <dt>Source</dt>
                      <dd>{isDraftDocument ? 'draft' : source}</dd>
                    </div>
                    <div>
                      <dt>Updated</dt>
                      <dd>{updatedLabel}</dd>
                    </div>
                  </dl>
                </div>
              </div>
              <button id="knowledge-context-back-btn" className="glass-button-ghost" onClick={handleBackToList}>
                <ArrowLeft className="h-4 w-4" />
                <span>Back to list</span>
              </button>
            </div>
          </div>

          {detailError ? <div className="kc-alert kc-alert--error">{detailError}</div> : null}
          {isGitManagedDocument(detail) ? (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
              Editing here saves a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
            </div>
          ) : null}

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
            <div className="min-w-0 space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
                  <div className="kc-editor-heading">
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">Knowledge Content</h3>
                    <span className="kc-editor-stat">{contentMetrics.words} words</span>
                  </div>
                  <div className="flex items-center gap-2 flex-wrap">
                    {!isEditing ? (
                      <>
                        <button className="glass-button-ghost" onClick={handleCopy} title="Copy content" aria-label="Copy content">
                          <Copy className="h-4 w-4" />
                        </button>
                        <button className="glass-button-ghost" onClick={handleDownload} title="Download content" aria-label="Download content">
                          <Download className="h-4 w-4" />
                        </button>
                        {detail.uuid && !draftID ? (
                          <ResourceAccessCard
                            resourceType="knowledge_context"
                            resourceID={detail.id}
                            label="knowledge context"
                            buttonClassName="glass-button-ghost"
                            onAccessChange={handleAccessChange}
                          />
                        ) : null}
                        {canEditSelected ? (
                          <button className="glass-button-primary" onClick={startEditing}>
                            Edit
                          </button>
                        ) : null}
                        {canWriteKnowledge ? (
                          <button className={isGitManagedDocument(detail) ? 'glass-button-primary' : 'glass-button-subtle'} onClick={openCloneModal}>
                            Clone
                          </button>
                        ) : null}
                      </>
                    ) : (
                      <>
                        <button className="glass-button-ghost" onClick={discardEditing}>
                          Discard
                        </button>
                        <button className="glass-button-primary" onClick={saveDetail} disabled={saving}>
                          {saving ? 'Saving...' : 'Save'}
                        </button>
                      </>
                    )}
                    {canDeleteKnowledge ? (
                      <button className="glass-button-danger" onClick={() => openDeleteModal(detail)} disabled={saving}>
                        <Trash2 className="h-4 w-4" />
                        Delete
                      </button>
                    ) : null}
                  </div>
                </div>

                {isEditing ? (
                  <div className="kc-description-editor border-b border-[var(--border-primary)] p-4">
                    <label className="block text-sm font-medium text-[var(--text-secondary)]" htmlFor="knowledge-context-description">
                      Description
                    </label>
                    <textarea
                      id="knowledge-context-description"
                      className="kc-description-input mt-2"
                      value={detail.description || ''}
                      disabled={!selectedCanEdit}
                      onChange={event => setDetail(current => (current ? { ...current, description: event.target.value } : current))}
                      placeholder="Optional description"
                    />
                  </div>
                ) : null}

                <div className="kc-editor-body">
                  {isEditing ? (
                    <textarea
                      className="kc-editor-textarea"
                      value={editorValue}
                      disabled={!selectedCanEdit}
                      onChange={event => setEditorValue(event.target.value)}
                      spellCheck={false}
                    />
                  ) : (
                    <TextPreview content={previewContent} />
                  )}
                </div>
              </div>
            </div>

            <div className="min-w-0 space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Used in Pipelines</h3>
                  <p className="text-xs text-[var(--text-secondary)] mt-1">Pipelines currently referencing this knowledge context.</p>
                </div>
                <div className="p-4">
                  {detail.used_by?.length ? (
                    <ul className={`triggers-pipeline-list ${detail.used_by.length > 5 ? 'triggers-pipelines-scroll' : ''}`}>
                      {detail.used_by.map(pipelineID => (
                        <li key={pipelineID} className="triggers-pipeline-item">
                          <button
                            type="button"
                            className="triggers-pipeline-link"
                            title={`Open ${pipelineID}`}
                            onClick={() => navigate(`/pipelines/${pipelineID.split('/').map(encodeURIComponent).join('/')}`)}
                          >
                            <span className="triggers-pipeline-name">{pipelineID}</span>
                            <dl className="triggers-detail-grid triggers-pipeline-details">
                              <dt className="triggers-detail-label">Action:</dt>
                              <dd className="triggers-detail-value">Open pipeline</dd>
                            </dl>
                          </button>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No pipelines reference this document.</p>
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
    <div data-page="knowledge-context" className="active h-full flex flex-col">
      {!selectedID && (
        <div className="px-6 pt-6 pb-4">
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              className="glass-button-ghost"
              aria-label="Back"
              onClick={() => openFolder(parentFolder(activeFolder))}
              disabled={!activeFolder}
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
              <button
                type="button"
                className="pipelines-search-toggle"
                aria-label="Search knowledge contexts"
                onClick={() => {
                  setSearchOpen(true);
                  requestAnimationFrame(() => searchInputRef.current?.focus());
                }}
              >
                <Search className="h-4 w-4" />
              </button>
              <input
                ref={searchInputRef}
                id="knowledge-context-search"
                type="text"
                placeholder="Search knowledge contexts"
                className="pipelines-search-input"
                value={search}
                onChange={event => {
                  setSearch(event.target.value);
                  if (event.target.value && !searchOpen) setSearchOpen(true);
                }}
                onBlur={() => {
                  if (!search.trim()) setSearchOpen(false);
                }}
              />
              {(search || searchOpen) && (
                <button
                  type="button"
                  className="pipelines-search-clear"
                  onClick={() => {
                    setSearch('');
                    setSearchOpen(false);
                    searchInputRef.current?.blur();
                  }}
                  aria-label="Clear search"
                >
                  x
                </button>
              )}
            </div>
            {canWriteKnowledge ? (
              <button id="knowledge-context-new-btn" type="button" className="pipelines-icon-only" aria-label="Create new knowledge context" title="New Knowledge Context" onClick={openCreateModal}>
                <Plus className="h-4 w-4" />
              </button>
            ) : null}
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {!selectedID ? (
          <KnowledgeContextCollectionList
            listLoading={listLoading}
            listError={listError}
            search={search}
            visibleDocuments={visibleDocuments}
            visibleFolders={visibleFolders}
            selectedID={selectedID}
            canWriteKnowledge={canWriteKnowledge}
            canDeleteKnowledge={canDeleteKnowledge}
            onOpenFolder={openFolder}
            onSelectDocument={handleSelectDocument}
            onDeleteDocument={openDeleteModal}
          />
        ) : detailLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading knowledge context...</div>
        ) : detailError && !detail ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load knowledge context: {detailError}</div>
        ) : (
          renderDetail()
        )}
      </div>

      <KnowledgeContextModals
        formModal={formModal}
        deleteModal={deleteModal}
        onCloseForm={() => setFormModal(null)}
        onUpdateForm={patch => setFormModal(prev => (prev ? { ...prev, ...patch, error: undefined } : prev))}
        onSubmitForm={submitFormModal}
        onCloseDelete={() => setDeleteModal(null)}
        onConfirmDelete={confirmDelete}
      />

      <WorkflowToastRegion toasts={toasts} />
    </div>
  );
}

function TextPreview({ content }: { content: string }) {
  if (!content.trim()) return <article className="kc-markdown kc-markdown--empty">No content</article>;
  return (
    <article className="kc-markdown">
      <pre>
        <code>{content}</code>
      </pre>
    </article>
  );
}
