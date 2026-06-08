import { useCallback, useState } from 'react';
import yaml from 'js-yaml';

type ResourceSource = 'git' | 'database' | 'draft' | string;

type ResourceRecord = {
  id: string;
  source?: string;
};

type ResourceDetail = ResourceRecord & {
  rawYaml: string;
};

export type YamlResourceFormModal = {
  mode: 'create' | 'clone';
  path: string;
  name: string;
  pending: boolean;
  error?: string;
  baseYaml?: string;
};

export type YamlResourceDeleteModal = {
  resourceId: string;
  resourceName: string;
  pending: boolean;
  error?: string;
};

type ToastTone = 'success' | 'error' | 'info';

type YamlResourceMutationOptions<TDetail extends ResourceDetail> = {
  resourceLabel: 'pipeline' | 'step';
  resources: ResourceRecord[];
  detail: TDetail | null;
  editorValue: string;
  validationErrorCount: number;
  validationMessage: string;
  permissionFolder: string;
  draftScope: string;
  canCreate: boolean;
  canUpdate: boolean;
  canDelete: boolean;
  canUseDrafts: boolean;
  namePattern: RegExp;
  normalizePath: (path: string) => string;
  normalizeSource: (source?: string) => ResourceSource;
  checkCreatePermission: (action: string, resourceID: string) => Promise<boolean>;
  persistYaml: (resourceID: string, rawYaml: string) => Promise<void>;
  deleteResource: (resourceID: string) => Promise<void>;
  upsertDraft: (draft: { id: string; yaml: string }) => unknown;
  removeDraft: (resourceID: string) => unknown;
  parseSaved: (rawYaml: string, resourceID: string, source?: string) => TDetail;
  reloadResources: (options?: { quiet?: boolean }) => Promise<void>;
  addToast: (message: string, tone?: ToastTone) => void;
  onSelect: (resourceID: string) => void;
  onSaved: (detail: TDetail) => void;
  onDeleted: () => void;
  buildTemplate: (name: string) => string;
};

function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function updateYamlResourceName(rawYaml: string, nextName: string) {
  try {
    const parsed = yaml.load(rawYaml) as Record<string, unknown> | undefined;
    return yaml.dump({ ...(parsed || {}), name: nextName }, { lineWidth: 120 });
  } catch {
    const replaced = rawYaml.replace(/^name:\s*.+$/m, `name: ${nextName}`);
    return replaced !== rawYaml ? replaced : `name: ${nextName}\n${rawYaml}`;
  }
}

export function useYamlResourceMutations<TDetail extends ResourceDetail>({
  resourceLabel,
  resources,
  detail,
  editorValue,
  validationErrorCount,
  validationMessage,
  permissionFolder,
  draftScope,
  canCreate,
  canUpdate,
  canDelete,
  canUseDrafts,
  namePattern,
  normalizePath,
  normalizeSource,
  checkCreatePermission,
  persistYaml,
  deleteResource,
  upsertDraft,
  removeDraft,
  parseSaved,
  reloadResources,
  addToast,
  onSelect,
  onSaved,
  onDeleted,
  buildTemplate,
}: YamlResourceMutationOptions<TDetail>) {
  const [saving, setSaving] = useState(false);
  const [formModal, setFormModal] = useState<YamlResourceFormModal | null>(null);
  const [deleteModal, setDeleteModal] = useState<YamlResourceDeleteModal | null>(null);
  const resourceTitle = capitalize(resourceLabel);
  const resourcePlural = `${resourceLabel}s`;

  const updateFormModal = useCallback(
    (patch: Partial<Pick<YamlResourceFormModal, 'path' | 'name'>>) => {
      setFormModal(current => (current ? { ...current, ...patch } : current));
    },
    []
  );

  const closeFormModal = useCallback(() => setFormModal(null), []);
  const closeDeleteModal = useCallback(() => setDeleteModal(null), []);

  const openCreateModal = useCallback(() => {
    if (!canCreate) {
      addToast(`You have read-only access to ${resourcePlural}.`, 'info');
      return;
    }
    setFormModal({
      mode: 'create',
      path: permissionFolder,
      name: '',
      pending: false,
    });
  }, [addToast, canCreate, permissionFolder, resourcePlural]);

  const openCloneModal = useCallback(() => {
    if (!canCreate) {
      addToast(`You have read-only access to ${resourcePlural}.`, 'info');
      return;
    }
    if (!detail) {
      addToast(`Select a ${resourceLabel} to clone.`, 'info');
      return;
    }
    const parts = detail.id.split('/').filter(Boolean);
    const name = decodeURIComponent(parts.pop() || '');
    const path = parts.map(decodeURIComponent).join('/');
    setFormModal({
      mode: 'clone',
      path,
      name: `${name}-copy`,
      pending: false,
      baseYaml: detail.rawYaml,
    });
  }, [addToast, canCreate, detail, resourceLabel, resourcePlural]);

  const openDeleteModal = useCallback((resourceId: string, resourceName: string) => {
    setDeleteModal({ resourceId, resourceName, pending: false });
  }, []);

  const save = useCallback(async () => {
    if (!detail || !editorValue.trim()) return false;
    const detailSource = normalizeSource(detail.source);
    const canPersist = detailSource === 'draft' ? canCreate : canUpdate;
    if (!canPersist) {
      addToast(`You have read-only access to ${resourcePlural}.`, 'info');
      return false;
    }
    if (detailSource === 'git') {
      addToast(
        `Git-managed ${resourcePlural} are read-only. Clone it to create an editable draft.`,
        'info'
      );
      return false;
    }
    if (validationErrorCount > 0) {
      addToast(validationMessage, 'error');
      return false;
    }

    setSaving(true);
    try {
      await persistYaml(detail.id, editorValue);
      addToast(`${resourceTitle} saved.`, 'success');
      const wasDraft = detailSource === 'draft';
      if (wasDraft) removeDraft(detail.id);
      const resolvedSource = wasDraft
        ? 'database'
        : resources.find(item => item.id === detail.id)?.source;
      onSaved(parseSaved(editorValue, detail.id, resolvedSource));
      await reloadResources({ quiet: true });
      return true;
    } catch (error) {
      console.error('Save failed', error);
      addToast(
        error instanceof Error ? error.message : `Unable to save ${resourceLabel}`,
        'error'
      );
      return false;
    } finally {
      setSaving(false);
    }
  }, [
    addToast,
    canCreate,
    canUpdate,
    detail,
    editorValue,
    normalizeSource,
    onSaved,
    parseSaved,
    persistYaml,
    reloadResources,
    removeDraft,
    resourceLabel,
    resourcePlural,
    resourceTitle,
    resources,
    validationErrorCount,
    validationMessage,
  ]);

  const submitFormModal = useCallback(async () => {
    if (!formModal) return false;
    const setError = (message: string) =>
      setFormModal(current => (current ? { ...current, error: message } : current));
    if (!canCreate || !draftScope) {
      setError(`You have read-only access to ${resourcePlural}.`);
      return false;
    }

    const name = formModal.name.trim();
    const path = normalizePath(formModal.path);
    const identifier = name ? (path ? `${path}/${name}` : name) : '';
    if (!identifier) {
      setError(`${resourceTitle} name is required.`);
      return false;
    }
    if (!namePattern.test(name)) {
      setError(
        `${resourceTitle} name can only contain letters, numbers, dots, underscores, and hyphens.`
      );
      return false;
    }
    if (resources.some(item => item.id === identifier)) {
      setError(`A ${resourceLabel} with that identifier already exists.`);
      return false;
    }
    if (!(await checkCreatePermission(`${resourceLabel}.create`, identifier))) {
      setError(
        `You do not have permission to create ${resourcePlural} in this path.`
      );
      return false;
    }

    setFormModal(current =>
      current ? { ...current, pending: true, error: undefined } : current
    );
    try {
      const yamlBody =
        formModal.mode === 'clone' && formModal.baseYaml
          ? updateYamlResourceName(formModal.baseYaml, name)
          : buildTemplate(name);
      upsertDraft({ id: identifier, yaml: yamlBody });
      addToast(
        `Draft ${resourceLabel} ${formModal.mode === 'create' ? 'created' : 'cloned'}.`,
        'success'
      );
      setFormModal(null);
      onSelect(identifier);
      return true;
    } catch (error) {
      console.error('Draft save failed', error);
      setError(
        error instanceof Error ? error.message : `Unable to create ${resourceLabel} draft`
      );
      return false;
    } finally {
      setFormModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [
    addToast,
    buildTemplate,
    canCreate,
    checkCreatePermission,
    draftScope,
    formModal,
    namePattern,
    normalizePath,
    onSelect,
    resourceLabel,
    resourcePlural,
    resourceTitle,
    resources,
    upsertDraft,
  ]);

  const confirmDelete = useCallback(async () => {
    if (!deleteModal) return false;
    setDeleteModal(current =>
      current ? { ...current, pending: true, error: undefined } : current
    );
    try {
      const source = resources.find(item => item.id === deleteModal.resourceId)?.source;
      const normalizedSource = normalizeSource(source);
      if (normalizedSource === 'git') {
        throw new Error(
          `This ${resourceLabel} is managed via Git. Clone it to customize instead of deleting.`
        );
      }
      if (normalizedSource === 'draft') {
        if (!canUseDrafts || !draftScope) {
          throw new Error(`You have read-only access to ${resourcePlural}.`);
        }
        removeDraft(deleteModal.resourceId);
      } else {
        if (!canDelete) {
          throw new Error(`You do not have permission to delete ${resourcePlural}.`);
        }
        await deleteResource(deleteModal.resourceId);
      }
      addToast(`${resourceTitle} deleted.`, 'success');
      setDeleteModal(null);
      onDeleted();
      await reloadResources();
      return true;
    } catch (error) {
      console.error('Delete failed', error);
      setDeleteModal(current =>
        current
          ? {
              ...current,
              error:
                error instanceof Error
                  ? error.message
                  : `Unable to delete ${resourceLabel}`,
            }
          : current
      );
      return false;
    } finally {
      setDeleteModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [
    addToast,
    canDelete,
    canUseDrafts,
    deleteModal,
    deleteResource,
    draftScope,
    normalizeSource,
    onDeleted,
    reloadResources,
    removeDraft,
    resourceLabel,
    resourcePlural,
    resourceTitle,
    resources,
  ]);

  return {
    closeDeleteModal,
    closeFormModal,
    confirmDelete,
    deleteModal,
    formModal,
    openCloneModal,
    openCreateModal,
    openDeleteModal,
    save,
    saving,
    submitFormModal,
    updateFormModal,
  };
}
