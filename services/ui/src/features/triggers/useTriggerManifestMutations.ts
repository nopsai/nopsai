import { useCallback, useState } from 'react';
import {
  checkTriggerPermission,
  deleteTrigger,
  saveTrigger,
} from './api';
import {
  buildNewTriggerYaml,
  buildTriggerSummary,
  deriveDefaultPipelinePath,
  normalizeSource,
  parseTriggerYaml,
  splitTriggerSlug,
  type PipelineRef,
  type TriggerDetail,
  type TriggerListItem,
} from './model';

export type TriggerCreateModalState = {
  repository: string;
  yamlPreview: string;
  pending: boolean;
  error?: string;
};

export type TriggerCloneModalState = {
  repository: string;
  pending: boolean;
  error?: string;
};

export type TriggerDeleteModalState = {
  slug: string;
  gitOpsManaged?: boolean;
  pending: boolean;
  error?: string;
};

type ToastTone = 'success' | 'error' | 'info';

type TriggerManifestMutationOptions = {
  canCreateTriggerHere: boolean;
  canUpdateSelectedTrigger: boolean;
  canDeleteTriggers: boolean;
  permissionFolder: string;
  detail: TriggerDetail | null;
  editorValue: string;
  validationErrorCount: number;
  serverTriggers: TriggerListItem[];
  addToast: (message: string, tone?: ToastTone) => void;
  loadTriggers: () => Promise<void>;
  loadRecentRuns: (slug: string, pipelines: PipelineRef[]) => Promise<void>;
  onSelectSlug: (slug: string) => void;
  onSaved: (detail: TriggerDetail) => void;
  onEditingFinished: () => void;
  onDeleted: () => void;
};

export function useTriggerManifestMutations({
  canCreateTriggerHere,
  canUpdateSelectedTrigger,
  canDeleteTriggers,
  permissionFolder,
  detail,
  editorValue,
  validationErrorCount,
  serverTriggers,
  addToast,
  loadTriggers,
  loadRecentRuns,
  onSelectSlug,
  onSaved,
  onEditingFinished,
  onDeleted,
}: TriggerManifestMutationOptions) {
  const [saving, setSaving] = useState(false);
  const [createModal, setCreateModal] = useState<TriggerCreateModalState | null>(null);
  const [cloneModal, setCloneModal] = useState<TriggerCloneModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<TriggerDeleteModalState | null>(null);

  const closeCreateModal = useCallback(() => setCreateModal(null), []);
  const closeCloneModal = useCallback(() => setCloneModal(null), []);
  const closeDeleteModal = useCallback(() => setDeleteModal(null), []);

  const updateCreateRepository = useCallback((repository: string) => {
    const pipelinePath = deriveDefaultPipelinePath(repository);
    setCreateModal(current =>
      current
        ? {
            ...current,
            repository,
            yamlPreview: buildNewTriggerYaml(pipelinePath),
            error: undefined,
          }
        : current
    );
  }, []);

  const updateCloneRepository = useCallback((repository: string) => {
    setCloneModal(current => (current ? { ...current, repository, error: undefined } : current));
  }, []);

  const openCreateModal = useCallback(() => {
    if (!canCreateTriggerHere) return;
    const repository = permissionFolder ? `${permissionFolder}/new-repository` : '';
    setCreateModal({
      repository,
      yamlPreview: buildNewTriggerYaml(deriveDefaultPipelinePath(repository)),
      pending: false,
    });
  }, [canCreateTriggerHere, permissionFolder]);

  const openCloneModal = useCallback(() => {
    if (!canCreateTriggerHere) return;
    if (!detail) {
      addToast('Select a trigger to clone.', 'info');
      return;
    }
    setCloneModal({ repository: detail.slug, pending: false });
  }, [addToast, canCreateTriggerHere, detail]);

  const openDeleteModal = useCallback(
    (slug: string) => {
      if (!canDeleteTriggers) return;
      const source = normalizeSource(serverTriggers.find(item => item.slug === slug)?.source);
      setDeleteModal({ slug, gitOpsManaged: source === 'git', pending: false });
    },
    [canDeleteTriggers, serverTriggers]
  );

  const save = useCallback(async () => {
    if (!canUpdateSelectedTrigger) {
      addToast('You do not have permission to update triggers.', 'error');
      return false;
    }
    if (!detail) return false;
    const isGitSource = normalizeSource(detail.source) === 'git';
    if (validationErrorCount > 0) {
      addToast('Resolve validation errors before saving.', 'error');
      return false;
    }
    if (editorValue === detail.rawYaml) {
      onEditingFinished();
      return true;
    }

    setSaving(true);
    try {
      await saveTrigger(detail.slug, editorValue);
      const summary = buildTriggerSummary(parseTriggerYaml(editorValue));
      const updated = { ...detail, rawYaml: editorValue, summary, source: isGitSource ? 'database' : detail.source };
      onSaved(updated);
      onEditingFinished();
      addToast(
        isGitSource
          ? 'Trigger saved as a database override. The next GitOps sync can replace it unless it is pushed to GitOps.'
          : 'Trigger saved.',
        'success'
      );
      await loadTriggers();
      void loadRecentRuns(detail.slug, summary.pipelines);
      return true;
    } catch (error) {
      console.error('Save failed', error);
      addToast(error instanceof Error ? error.message : 'Unable to save trigger', 'error');
      return false;
    } finally {
      setSaving(false);
    }
  }, [
    addToast,
    canUpdateSelectedTrigger,
    detail,
    editorValue,
    loadRecentRuns,
    loadTriggers,
    onEditingFinished,
    onSaved,
    validationErrorCount,
  ]);

  const submitCreateModal = useCallback(async () => {
    if (!canCreateTriggerHere || !createModal) return false;
    const repoSlug = createModal.repository.trim();
    if (!repoSlug) {
      setCreateModal(current => (current ? { ...current, error: 'Repository is required.' } : current));
      return false;
    }

    let owner: string;
    let repo: string;
    try {
      ({ owner, repo } = splitTriggerSlug(repoSlug));
    } catch (error) {
      setCreateModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Invalid repository.' } : current
      );
      return false;
    }

    const normalizedSlug = `${owner}/${repo}`;
    const allowed = await checkTriggerPermission('trigger.update', normalizedSlug);
    if (!allowed) {
      setCreateModal(current =>
        current
          ? { ...current, error: 'You do not have permission to create triggers for this repository.' }
          : current
      );
      return false;
    }

    setCreateModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      await saveTrigger(normalizedSlug, createModal.yamlPreview);
      setCreateModal(null);
      addToast('Trigger created.', 'success');
      await loadTriggers();
      onSelectSlug(normalizedSlug);
      return true;
    } catch (error) {
      console.error('Create failed', error);
      setCreateModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Unable to create trigger' } : current
      );
      return false;
    } finally {
      setCreateModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [addToast, canCreateTriggerHere, createModal, loadTriggers, onSelectSlug]);

  const submitCloneModal = useCallback(async () => {
    if (!canCreateTriggerHere || !cloneModal || !detail) return false;
    const targetSlug = cloneModal.repository.trim();
    if (!targetSlug) {
      setCloneModal(current => (current ? { ...current, error: 'Repository is required.' } : current));
      return false;
    }

    let owner: string;
    let repo: string;
    try {
      ({ owner, repo } = splitTriggerSlug(targetSlug));
    } catch (error) {
      setCloneModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Invalid repository.' } : current
      );
      return false;
    }

    const normalizedSlug = `${owner}/${repo}`;
    const allowed = await checkTriggerPermission('trigger.update', normalizedSlug);
    if (!allowed) {
      setCloneModal(current =>
        current
          ? { ...current, error: 'You do not have permission to create triggers for this repository.' }
          : current
      );
      return false;
    }

    setCloneModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      await saveTrigger(normalizedSlug, detail.rawYaml);
      setCloneModal(null);
      addToast('Trigger cloned.', 'success');
      await loadTriggers();
      onSelectSlug(normalizedSlug);
      return true;
    } catch (error) {
      console.error('Clone failed', error);
      setCloneModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Unable to clone trigger' } : current
      );
      return false;
    } finally {
      setCloneModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [addToast, canCreateTriggerHere, cloneModal, detail, loadTriggers, onSelectSlug]);

  const confirmDelete = useCallback(async () => {
    if (!canDeleteTriggers || !deleteModal) return false;
    let owner: string;
    let repo: string;
    try {
      ({ owner, repo } = splitTriggerSlug(deleteModal.slug));
    } catch (error) {
      setDeleteModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Invalid repository.' } : current
      );
      return false;
    }

    const normalizedSlug = `${owner}/${repo}`;
    setDeleteModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      await deleteTrigger(normalizedSlug);
      setDeleteModal(null);
      addToast(
        deleteModal.gitOpsManaged
          ? 'Trigger database row deleted. The next GitOps sync can recreate it from the repository.'
          : 'Trigger deleted.',
        'success'
      );
      await loadTriggers();
      onDeleted();
      return true;
    } catch (error) {
      console.error('Delete failed', error);
      setDeleteModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Unable to delete trigger' } : current
      );
      return false;
    } finally {
      setDeleteModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [addToast, canDeleteTriggers, deleteModal, loadTriggers, onDeleted]);

  return {
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
    save,
    saving,
    submitCloneModal,
    submitCreateModal,
    updateCloneRepository,
    updateCreateRepository,
  };
}
