import { useCallback, useState } from 'react';
import {
  checkTriggerPermission,
  deleteTrigger,
  saveTrigger,
} from './api';
import {
  applyTriggerDetailsToYaml,
  buildNewTriggerYaml,
  buildTriggerSummary,
  deriveDefaultPipelinePath,
  normalizeSource,
  normalizeTriggerProvider,
  normalizeTriggerTeamPath,
  parseTriggerYaml,
  splitTriggerSlug,
  triggerDetailsFormFromYaml,
  triggerDetailsWithProvider,
  type PipelineRef,
  type TriggerDetail,
  type TriggerDetailsFormState,
  type TriggerListItem,
} from './model';

export type TriggerCreateModalState = {
  repository: string;
  details: TriggerDetailsFormState;
  yamlPreview: string;
  pending: boolean;
  error?: string;
};

export type TriggerCloneModalState = {
  repository: string;
  details: TriggerDetailsFormState;
  yamlPreview: string;
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
  permissionOwner: string;
  detail: TriggerDetail | null;
  editorValue: string;
  validationErrorCount: number;
  serverTriggers: TriggerListItem[];
  defaultTeamPath?: string;
  addToast: (message: string, tone?: ToastTone) => void;
  loadTriggers: () => Promise<void>;
  loadRecentRuns: (slug: string, pipelines: PipelineRef[]) => Promise<void>;
  onSelectSlug: (slug: string) => void;
  onSaved: (detail: TriggerDetail) => void;
  onEditingFinished: () => void;
  onDeleted: () => void;
};

export function useTriggerManifestMutations({
  canDeleteTriggers,
  permissionOwner,
  detail,
  editorValue,
  validationErrorCount,
  serverTriggers,
  defaultTeamPath = '',
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
            yamlPreview: buildNewTriggerYaml(pipelinePath, current.details),
            error: undefined,
          }
        : current
    );
  }, []);

  const updateCreateDetails = useCallback((details: TriggerDetailsFormState) => {
    setCreateModal(current => {
      if (!current) return current;
      const nextDetails = triggerDetailsWithProvider(details, normalizeTriggerProvider(details.provider));
      return {
        ...current,
        details: nextDetails,
        yamlPreview: applyTriggerDetailsToYaml(current.yamlPreview, nextDetails),
        error: undefined,
      };
    });
  }, []);

  const updateCreateYamlPreview = useCallback((yamlPreview: string) => {
    setCreateModal(current =>
      current
        ? {
            ...current,
            details: triggerDetailsFormFromYaml(yamlPreview, {
              provider: current.details.provider,
              teamPath: current.details.teamPath,
              management: current.details.management,
              webhookSourceID: current.details.webhookSourceID,
            }),
            yamlPreview,
            error: undefined,
          }
        : current
    );
  }, []);

  const updateCloneRepository = useCallback((repository: string) => {
    setCloneModal(current => (current ? { ...current, repository, error: undefined } : current));
  }, []);

  const updateCloneDetails = useCallback((details: TriggerDetailsFormState) => {
    setCloneModal(current => {
      if (!current) return current;
      const nextDetails = triggerDetailsWithProvider(details, normalizeTriggerProvider(details.provider));
      return {
        ...current,
        details: nextDetails,
        yamlPreview: applyTriggerDetailsToYaml(current.yamlPreview, nextDetails),
        error: undefined,
      };
    });
  }, []);

  const updateCloneYamlPreview = useCallback((yamlPreview: string) => {
    setCloneModal(current =>
      current
        ? {
            ...current,
            details: triggerDetailsFormFromYaml(yamlPreview, {
              provider: current.details.provider,
              teamPath: current.details.teamPath,
              management: current.details.management,
              webhookSourceID: current.details.webhookSourceID,
            }),
            yamlPreview,
            error: undefined,
          }
        : current
    );
  }, []);

  const openCreateModal = useCallback(() => {
    const repository = permissionOwner ? `${permissionOwner}/new-repository` : '';
    const details: TriggerDetailsFormState = {
      provider: 'github',
      teamPath: normalizeTriggerTeamPath(defaultTeamPath),
      management: 'nopsai',
      webhookSourceID: '',
    };
    setCreateModal({
      repository,
      details,
      yamlPreview: buildNewTriggerYaml(deriveDefaultPipelinePath(repository), details),
      pending: false,
    });
  }, [defaultTeamPath, permissionOwner]);

  const openCloneModal = useCallback(() => {
    if (!detail) {
      addToast('Select a trigger to clone.', 'info');
      return;
    }
    const details = {
      ...triggerDetailsFormFromYaml(detail.rawYaml, detail),
      management: 'nopsai' as const,
    };
    setCloneModal({
      repository: detail.slug,
      details,
      yamlPreview: applyTriggerDetailsToYaml(detail.rawYaml, details),
      pending: false,
    });
  }, [addToast, detail]);

  const openDeleteModal = useCallback(
    (slug: string) => {
      if (!canDeleteTriggers) return;
      const source = normalizeSource(serverTriggers.find(item => item.slug === slug)?.source);
      setDeleteModal({ slug, gitOpsManaged: source === 'git', pending: false });
    },
    [canDeleteTriggers, serverTriggers]
  );

  const save = useCallback(async () => {
    if (!detail) return false;
    const allowed = await checkTriggerPermission('trigger.update', detail.slug);
    if (!allowed) {
      addToast('You do not have permission to update triggers.', 'error');
      return false;
    }
    const isGitSource = normalizeSource(detail.source) === 'git';
    if (validationErrorCount > 0) {
      addToast('Resolve validation errors before saving.', 'error');
      return false;
    }
    const savedDetails = triggerDetailsFormFromYaml(editorValue, detail);
    if (savedDetails.provider !== 'github' && !savedDetails.webhookSourceID.trim()) {
      addToast('Select a webhook source before saving a non-GitHub trigger.', 'error');
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
      const updated = {
        ...detail,
        rawYaml: editorValue,
        summary,
        source: isGitSource ? 'database' : detail.source,
        provider: savedDetails.provider,
        teamPath: savedDetails.teamPath,
        management: savedDetails.management,
        webhookSourceID: savedDetails.webhookSourceID,
        ingress: savedDetails.provider === 'github' ? 'GitHub App - automatic' : savedDetails.webhookSourceID,
      };
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
    detail,
    editorValue,
    loadRecentRuns,
    loadTriggers,
    onEditingFinished,
    onSaved,
    validationErrorCount,
  ]);

  const submitCreateModal = useCallback(async () => {
    if (!createModal) return false;
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
    if (createModal.details.provider !== 'github' && !createModal.details.webhookSourceID.trim()) {
      setCreateModal(current =>
        current ? { ...current, error: 'Webhook source is required for non-GitHub triggers.' } : current
      );
      return false;
    }
    if (createModal.details.management === 'repository') {
      setCreateModal(current =>
        current ? { ...current, error: 'Repository-managed GitHub triggers are read-only; create NopsAI-managed overrides here.' } : current
      );
      return false;
    }

    const normalizedSlug = `${owner}/${repo}`;
    const allowed = await checkTriggerPermission('trigger.update', normalizedSlug, createModal.details.teamPath);
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
      await saveTrigger(normalizedSlug, applyTriggerDetailsToYaml(createModal.yamlPreview, createModal.details));
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
  }, [addToast, createModal, loadTriggers, onSelectSlug]);

  const submitCloneModal = useCallback(async () => {
    if (!cloneModal || !detail) return false;
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
    const allowed = await checkTriggerPermission('trigger.update', normalizedSlug, cloneModal.details.teamPath);
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
      await saveTrigger(normalizedSlug, applyTriggerDetailsToYaml(cloneModal.yamlPreview, cloneModal.details));
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
  }, [addToast, cloneModal, detail, loadTriggers, onSelectSlug]);

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
    updateCloneDetails,
    updateCloneRepository,
    updateCloneYamlPreview,
    updateCreateDetails,
    updateCreateRepository,
    updateCreateYamlPreview,
  };
}
