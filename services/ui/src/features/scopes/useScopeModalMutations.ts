import { useCallback, useState } from 'react';
import {
  checkScopePermission,
  deleteScopedValue,
  encryptSecretValue,
  fetchVariableValue,
  saveScopedValue,
  scopedResourcePath,
} from './api';
import {
  normalizeRepositorySlug,
  normalizeScopeLabel,
  normalizeSourceKey,
  parseScopedIdentity,
  sanitizeScopeSegments,
  suggestCloneName,
} from './model';
import { buildNamedResourceID } from './useScopePermissions';

export const SAMPLE_SCOPE_VARIABLE = 'sample_variable';
const SAMPLE_SCOPE_VALUE = 'Replace with your %SCOPE% scope value.';
const VARIABLE_NAME_PATTERN = /^[A-Za-z0-9_.-]+$/;
const SECRET_NAME_PATTERN = /^[A-Za-z0-9_.-]+$/;

type ScopedKind = 'variable' | 'secret';
type ToastTone = 'success' | 'error' | 'info';
type ScopeCollection = {
  variables: string[];
  secrets: string[];
  variableMeta?: Record<string, { source?: unknown }>;
  secretMeta?: Record<string, { source?: unknown }>;
};

export type ScopeModalState = {
  parent: string;
  name: string;
  pending: boolean;
  error?: string;
};

export type ScopedValueModalState = {
  mode: 'create' | 'update';
  scope: string;
  originalName?: string;
  name: string;
  repository: string;
  value: string;
  gitOpsManaged?: boolean;
  valueLoading?: boolean;
  pending: boolean;
  error?: string;
};

export type GitOpsSecretEncryptModalState = {
  value: string;
  encryptedValue?: string;
  pending: boolean;
  error?: string;
};

export type ScopedValueDeleteModalState = {
  kind: ScopedKind;
  scope: string;
  name: string;
  gitOpsManaged?: boolean;
  pending: boolean;
  error?: string;
};

type ScopeModalMutationOptions = {
  activeTeam: string;
  scopesByLabel: Map<string, unknown>;
  scopeDataByScope: Record<string, ScopeCollection | undefined>;
  canCreateScopeHere: boolean;
  canWriteVariablesInSelectedScope: boolean;
  canWriteSecretsInSelectedScope: boolean;
  canDeleteScopes: boolean;
  selectedVariable: string | null;
  selectedSecret: string | null;
  addToast: (message: string, tone?: ToastTone) => void;
  loadScopes: () => Promise<void>;
  ensureScopeVariables: (scope: string, force?: boolean) => Promise<void>;
  ensureScopeSecrets: (scope: string, force?: boolean) => Promise<void>;
  selectVariable: (name: string | null) => void;
  selectSecret: (name: string | null) => void;
  clearExpandedVariable: () => void;
  onScopeCreated: (scope: string) => void;
};

function applyModalPatch<T extends object>(
  current: T | null,
  patch: Partial<T>
): T | null {
  return current ? { ...current, ...patch, error: undefined } : current;
}

function isGitOpsManagedSource(source: unknown): boolean {
  return normalizeSourceKey(source) === 'git';
}

function isScopedValueGitOpsManaged(
  scopeDataByScope: Record<string, ScopeCollection | undefined>,
  kind: ScopedKind,
  scopeLabel: string,
  fullName: string
): boolean {
  const scope = normalizeScopeLabel(scopeLabel);
  const data = scopeDataByScope[scope];
  const meta = kind === 'variable' ? data?.variableMeta?.[fullName] : data?.secretMeta?.[fullName];
  return isGitOpsManagedSource(meta?.source);
}

export function useScopeModalMutations({
  activeTeam,
  scopesByLabel,
  scopeDataByScope,
  canCreateScopeHere,
  canWriteVariablesInSelectedScope,
  canWriteSecretsInSelectedScope,
  canDeleteScopes,
  selectedVariable,
  selectedSecret,
  addToast,
  loadScopes,
  ensureScopeVariables,
  ensureScopeSecrets,
  selectVariable,
  selectSecret,
  clearExpandedVariable,
  onScopeCreated,
}: ScopeModalMutationOptions) {
  const [scopeModal, setScopeModal] = useState<ScopeModalState | null>(null);
  const [variableModal, setVariableModal] = useState<ScopedValueModalState | null>(null);
  const [secretModal, setSecretModal] = useState<ScopedValueModalState | null>(null);
  const [gitOpsEncryptModal, setGitOpsEncryptModal] =
    useState<GitOpsSecretEncryptModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<ScopedValueDeleteModalState | null>(null);

  const closeScopeModal = useCallback(() => setScopeModal(null), []);
  const closeVariableModal = useCallback(() => setVariableModal(null), []);
  const closeSecretModal = useCallback(() => setSecretModal(null), []);
  const closeGitOpsEncryptModal = useCallback(() => setGitOpsEncryptModal(null), []);
  const closeDeleteModal = useCallback(() => setDeleteModal(null), []);

  const openNewScopeModal = useCallback(() => {
    if (!canCreateScopeHere) return;
    setScopeModal({
      parent: normalizeScopeLabel(activeTeam),
      name: '',
      pending: false,
    });
  }, [activeTeam, canCreateScopeHere]);

  const updateScopeName = useCallback((name: string) => {
    setScopeModal(current => applyModalPatch(current, { name }));
  }, []);

  const createSampleVariableForScope = useCallback(async (scopeLabel: string) => {
    const normalized = normalizeScopeLabel(scopeLabel);
    const sampleValue = SAMPLE_SCOPE_VALUE.replace('%SCOPE%', normalized || 'default');
    await saveScopedValue(
      scopedResourcePath('variable', normalized, SAMPLE_SCOPE_VARIABLE),
      sampleValue,
      'variable'
    );
  }, []);

  const submitScopeModal = useCallback(async () => {
    if (!canCreateScopeHere || !scopeModal) return false;

    const parentLabel = normalizeScopeLabel(scopeModal.parent);
    const segments = sanitizeScopeSegments(scopeModal.name);
    if (!segments.length) {
      setScopeModal(current => (current ? { ...current, error: 'Scope name is required.' } : current));
      return false;
    }

    const parentSegments = sanitizeScopeSegments(parentLabel);
    const combinedSegments = parentSegments.concat(segments);
    const normalizedLabel = normalizeScopeLabel(combinedSegments.join('/'));
    if (!normalizedLabel) {
      setScopeModal(current => (current ? { ...current, error: 'Scope name is required.' } : current));
      return false;
    }
    if (scopesByLabel.has(normalizedLabel)) {
      setScopeModal(current =>
        current ? { ...current, error: `Scope “/${normalizedLabel}” already exists.` } : current
      );
      return false;
    }

    const [scopeAllowed, variableAllowed] = await Promise.all([
      checkScopePermission('scope.update', 'scope', normalizedLabel),
      checkScopePermission(
        'variable.write_value',
        'variable',
        buildNamedResourceID('', normalizedLabel, SAMPLE_SCOPE_VARIABLE)
      ),
    ]);
    if (!scopeAllowed || !variableAllowed) {
      setScopeModal(current =>
        current ? { ...current, error: 'You do not have permission to create scopes in this path.' } : current
      );
      return false;
    }

    setScopeModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      const scopeChain: string[] = [];
      combinedSegments.forEach((_, index) => {
        const partial = normalizeScopeLabel(combinedSegments.slice(0, index + 1).join('/'));
        if (partial) scopeChain.push(partial);
      });
      for (const scopePath of scopeChain) {
        await createSampleVariableForScope(scopePath);
      }

      await loadScopes();
      await ensureScopeVariables(normalizedLabel, true);
      addToast(`Scope “/${normalizedLabel}” created.`, 'success');
      setScopeModal(null);
      onScopeCreated(normalizedLabel);
      return true;
    } catch (error) {
      console.error('Failed to create scope', error);
      setScopeModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Failed to create scope.' } : current
      );
      return false;
    } finally {
      setScopeModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [
    addToast,
    canCreateScopeHere,
    createSampleVariableForScope,
    ensureScopeVariables,
    loadScopes,
    onScopeCreated,
    scopeModal,
    scopesByLabel,
  ]);

  const openVariableCreateModal = useCallback(
    (
      scopeLabel: string,
      options?: { repository?: string; nameSuggestion?: string; valuePreset?: string }
    ) => {
      if (!canWriteVariablesInSelectedScope) return;
      setVariableModal({
        mode: 'create',
        scope: normalizeScopeLabel(scopeLabel),
        name: options?.nameSuggestion || '',
        repository: options?.repository || '',
        value: options?.valuePreset || '',
        pending: false,
      });
    },
    [canWriteVariablesInSelectedScope]
  );

  const openVariableUpdateModal = useCallback(
    (scopeLabel: string, fullName: string) => {
      if (!canWriteVariablesInSelectedScope) return;
      const scope = normalizeScopeLabel(scopeLabel);
      const identity = parseScopedIdentity(fullName);
      const gitOpsManaged = isScopedValueGitOpsManaged(scopeDataByScope, 'variable', scope, identity.fullName);
      if (gitOpsManaged) {
        addToast('Editing saves a database override. The next GitOps sync can replace it unless it is pushed to GitOps.', 'info');
      }
      setVariableModal({
        mode: 'update',
        scope,
        originalName: identity.fullName,
        name: identity.name,
        repository: identity.repoSlug,
        value: '',
        gitOpsManaged,
        valueLoading: true,
        pending: false,
      });
      void fetchVariableValue(scopedResourcePath('variable', scope, identity.name, identity.repoSlug))
        .then(value => {
          setVariableModal(current => {
            if (!current || current.mode !== 'update') return current;
            if (current.scope !== scope || current.originalName !== identity.fullName) return current;
            return { ...current, value: current.value || value, valueLoading: false, error: undefined };
          });
        })
        .catch(error => {
          console.error('Failed to load variable value', error);
          setVariableModal(current => {
            if (!current || current.mode !== 'update') return current;
            if (current.scope !== scope || current.originalName !== identity.fullName) return current;
            return {
              ...current,
              valueLoading: false,
              error: error instanceof Error ? error.message : 'Failed to load variable value.',
            };
          });
        });
    },
    [addToast, canWriteVariablesInSelectedScope, scopeDataByScope]
  );

  const openVariableCloneModal = useCallback(
    (scopeLabel: string, fullName: string) => {
      if (!canWriteVariablesInSelectedScope) return;
      const scope = normalizeScopeLabel(scopeLabel);
      const identity = parseScopedIdentity(fullName);
      openVariableCreateModal(scope, {
        repository: identity.repoSlug,
        nameSuggestion: suggestCloneName(
          scopeDataByScope[scope]?.variables || [],
          identity.repoSlug,
          identity.name || fullName
        ),
      });
    },
    [canWriteVariablesInSelectedScope, openVariableCreateModal, scopeDataByScope]
  );

  const updateVariableModal = useCallback((patch: Partial<ScopedValueModalState>) => {
    setVariableModal(current => applyModalPatch(current, patch));
  }, []);

  const chooseVariableSuggestion = useCallback((fullName: string) => {
    setVariableModal(current => {
      if (!current || current.mode !== 'create') return current;
      const picked = parseScopedIdentity(fullName);
      return { ...current, name: picked.name, repository: picked.repoSlug, error: undefined };
    });
  }, []);

  const submitVariableModal = useCallback(async () => {
    if (!canWriteVariablesInSelectedScope || !variableModal) return false;

    const scope = normalizeScopeLabel(variableModal.scope);
    const nameInput = variableModal.name.trim();
    const repoSlug = normalizeRepositorySlug(variableModal.repository);
    const value = variableModal.value ?? '';

    if (variableModal.mode === 'create') {
      if (!nameInput) {
        setVariableModal(current => (current ? { ...current, error: 'Variable name is required.' } : current));
        return false;
      }
      if (!VARIABLE_NAME_PATTERN.test(nameInput)) {
        setVariableModal(current =>
          current ? { ...current, error: 'Variable name may contain letters, numbers, underscores, dots, and hyphens.' } : current
        );
        return false;
      }
      if (variableModal.repository.trim() && !repoSlug) {
        setVariableModal(current => (current ? { ...current, error: 'Repository must use the “owner/repository” format.' } : current));
        return false;
      }
      if (repoSlug && nameInput.includes('/')) {
        setVariableModal(current => (current ? { ...current, error: 'Variable name should not include “/” when a repository is selected.' } : current));
        return false;
      }
      if (!value) {
        setVariableModal(current => (current ? { ...current, error: 'Provide a value for the new variable.' } : current));
        return false;
      }
    } else if (!variableModal.originalName) {
      setVariableModal(current => (current ? { ...current, error: 'Missing variable identifier.' } : current));
      return false;
    }

    const identity =
      variableModal.mode === 'update' && variableModal.originalName
        ? parseScopedIdentity(variableModal.originalName)
        : { ...parseScopedIdentity(nameInput), repoSlug };
    const finalRepoSlug = variableModal.mode === 'create' ? repoSlug : identity.repoSlug;
    const finalName = variableModal.mode === 'create' ? nameInput : identity.name;
    const allowed = await checkScopePermission(
      'variable.write_value',
      'variable',
      buildNamedResourceID(finalRepoSlug, scope, finalName)
    );
    if (!allowed) {
      setVariableModal(current => (current ? { ...current, error: 'You do not have permission to save variables in this scope.' } : current));
      return false;
    }

    setVariableModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      await saveScopedValue(
        scopedResourcePath('variable', scope, finalName, finalRepoSlug),
        value,
        'variable'
      );
      const fullName = finalRepoSlug ? `${finalRepoSlug}/${finalName}` : finalName;
      addToast(
        variableModal.mode === 'update' && variableModal.gitOpsManaged
          ? 'Variable saved as a database override. GitOps can replace it on the next sync unless it is pushed.'
          : variableModal.mode === 'update'
            ? 'Variable updated.'
            : 'Variable created.',
        'success'
      );
      setVariableModal(null);
      await ensureScopeVariables(scope, true);
      selectVariable(fullName);
      clearExpandedVariable();
      return true;
    } catch (error) {
      console.error('Failed to save variable', error);
      setVariableModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Failed to save variable.' } : current
      );
      return false;
    } finally {
      setVariableModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [
    addToast,
    canWriteVariablesInSelectedScope,
    clearExpandedVariable,
    ensureScopeVariables,
    selectVariable,
    variableModal,
  ]);

  const openSecretCreateModal = useCallback(
    (
      scopeLabel: string,
      options?: { repository?: string; nameSuggestion?: string; valuePreset?: string }
    ) => {
      if (!canWriteSecretsInSelectedScope) return;
      setSecretModal({
        mode: 'create',
        scope: normalizeScopeLabel(scopeLabel),
        name: options?.nameSuggestion || '',
        repository: options?.repository || '',
        value: options?.valuePreset || '',
        pending: false,
      });
    },
    [canWriteSecretsInSelectedScope]
  );

  const openSecretUpdateModal = useCallback(
    (scopeLabel: string, fullName: string) => {
      if (!canWriteSecretsInSelectedScope) return;
      const scope = normalizeScopeLabel(scopeLabel);
      const identity = parseScopedIdentity(fullName);
      const gitOpsManaged = isScopedValueGitOpsManaged(scopeDataByScope, 'secret', scope, identity.fullName);
      if (gitOpsManaged) {
        addToast('Editing saves a database override. The next GitOps sync can replace it unless it is pushed to GitOps.', 'info');
      }
      setSecretModal({
        mode: 'update',
        scope,
        originalName: identity.fullName,
        name: identity.name,
        repository: identity.repoSlug,
        value: '',
        gitOpsManaged,
        pending: false,
      });
    },
    [addToast, canWriteSecretsInSelectedScope, scopeDataByScope]
  );

  const openSecretCloneModal = useCallback(
    (scopeLabel: string, fullName: string) => {
      if (!canWriteSecretsInSelectedScope) return;
      const scope = normalizeScopeLabel(scopeLabel);
      const identity = parseScopedIdentity(fullName);
      openSecretCreateModal(scope, {
        repository: identity.repoSlug,
        nameSuggestion: suggestCloneName(
          scopeDataByScope[scope]?.secrets || [],
          identity.repoSlug,
          identity.name || fullName
        ),
      });
    },
    [canWriteSecretsInSelectedScope, openSecretCreateModal, scopeDataByScope]
  );

  const updateSecretModal = useCallback((patch: Partial<ScopedValueModalState>) => {
    setSecretModal(current => applyModalPatch(current, patch));
  }, []);

  const chooseSecretSuggestion = useCallback((fullName: string) => {
    setSecretModal(current => {
      if (!current || current.mode !== 'create') return current;
      const picked = parseScopedIdentity(fullName);
      return { ...current, name: picked.name, repository: picked.repoSlug, error: undefined };
    });
  }, []);

  const submitSecretModal = useCallback(async () => {
    if (!canWriteSecretsInSelectedScope || !secretModal) return false;

    const scope = normalizeScopeLabel(secretModal.scope);
    const nameInput = secretModal.name.trim();
    const repoSlug = normalizeRepositorySlug(secretModal.repository);
    const value = secretModal.value ?? '';

    if (secretModal.mode === 'create') {
      if (!nameInput) {
        setSecretModal(current => (current ? { ...current, error: 'Secret name is required.' } : current));
        return false;
      }
      if (!SECRET_NAME_PATTERN.test(nameInput)) {
        setSecretModal(current =>
          current ? { ...current, error: 'Secret name may contain letters, numbers, underscores, dots, and hyphens.' } : current
        );
        return false;
      }
      if (secretModal.repository.trim() && !repoSlug) {
        setSecretModal(current => (current ? { ...current, error: 'Repository must use the “owner/repository” format.' } : current));
        return false;
      }
      if (repoSlug && nameInput.includes('/')) {
        setSecretModal(current => (current ? { ...current, error: 'Secret name should not include “/” when a repository is selected.' } : current));
        return false;
      }
      if (!value) {
        setSecretModal(current => (current ? { ...current, error: 'Provide a value for the new secret.' } : current));
        return false;
      }
    } else if (!secretModal.originalName) {
      setSecretModal(current => (current ? { ...current, error: 'Missing secret identifier.' } : current));
      return false;
    }

    if (secretModal.mode === 'update' && !value) {
      addToast('Secret value updated (unchanged).', 'info');
      setSecretModal(null);
      return true;
    }

    const identity =
      secretModal.mode === 'update' && secretModal.originalName
        ? parseScopedIdentity(secretModal.originalName)
        : { ...parseScopedIdentity(nameInput), repoSlug };
    const finalRepoSlug = secretModal.mode === 'create' ? repoSlug : identity.repoSlug;
    const finalName = secretModal.mode === 'create' ? nameInput : identity.name;
    const allowed = await checkScopePermission(
      'secret.write_value',
      'secret',
      buildNamedResourceID(finalRepoSlug, scope, finalName)
    );
    if (!allowed) {
      setSecretModal(current => (current ? { ...current, error: 'You do not have permission to save secrets in this scope.' } : current));
      return false;
    }

    setSecretModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      await saveScopedValue(
        scopedResourcePath('secret', scope, finalName, finalRepoSlug),
        value,
        'secret'
      );
      const fullName = finalRepoSlug ? `${finalRepoSlug}/${finalName}` : finalName;
      addToast(
        secretModal.mode === 'update' && secretModal.gitOpsManaged
          ? 'Secret saved as a database override. GitOps can replace it on the next sync unless it is pushed.'
          : secretModal.mode === 'update'
            ? 'Secret value updated.'
            : 'Secret created.',
        'success'
      );
      setSecretModal(null);
      await ensureScopeSecrets(scope, true);
      selectSecret(fullName);
      return true;
    } catch (error) {
      console.error('Failed to save secret', error);
      setSecretModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Failed to save secret.' } : current
      );
      return false;
    } finally {
      setSecretModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [addToast, canWriteSecretsInSelectedScope, ensureScopeSecrets, secretModal, selectSecret]);

  const openGitOpsEncryptModal = useCallback(() => {
    setGitOpsEncryptModal({ value: '', pending: false });
  }, []);

  const updateGitOpsEncryptValue = useCallback((value: string) => {
    setGitOpsEncryptModal(current =>
      current ? { ...current, value, encryptedValue: undefined, error: undefined } : current
    );
  }, []);

  const encryptGitOpsSecretValue = useCallback(async () => {
    if (!gitOpsEncryptModal) return false;
    const value = gitOpsEncryptModal.value ?? '';
    if (!value) {
      setGitOpsEncryptModal(current => (current ? { ...current, error: 'Provide a value to encrypt.' } : current));
      return false;
    }

    setGitOpsEncryptModal(current =>
      current ? { ...current, pending: true, error: undefined, encryptedValue: undefined } : current
    );
    try {
      const encryptedValue = await encryptSecretValue(value);
      setGitOpsEncryptModal(current => (current ? { ...current, encryptedValue, error: undefined } : current));
      return true;
    } catch (error) {
      console.error('Failed to encrypt secret for GitOps', error);
      setGitOpsEncryptModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Failed to encrypt secret.' } : current
      );
      return false;
    } finally {
      setGitOpsEncryptModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [gitOpsEncryptModal]);

  const copyGitOpsEncryptedValue = useCallback(async () => {
    const value = gitOpsEncryptModal?.encryptedValue;
    if (!value) return false;
    try {
      await navigator.clipboard.writeText(value);
      addToast('Encrypted value copied.', 'success');
      return true;
    } catch (error) {
      console.error('Failed to copy encrypted secret value', error);
      setGitOpsEncryptModal(current => (current ? { ...current, error: 'Unable to copy encrypted value.' } : current));
      return false;
    }
  }, [addToast, gitOpsEncryptModal?.encryptedValue]);

  const openDeleteModal = useCallback(
    (kind: ScopedKind, scope: string, name: string) => {
      if (!canDeleteScopes) return;
      setDeleteModal({
        kind,
        scope,
        name,
        gitOpsManaged: isScopedValueGitOpsManaged(scopeDataByScope, kind, scope, name),
        pending: false,
      });
    },
    [canDeleteScopes, scopeDataByScope]
  );

  const confirmDelete = useCallback(async () => {
    if (!canDeleteScopes || !deleteModal) return false;
    const scope = normalizeScopeLabel(deleteModal.scope);
    const identity = parseScopedIdentity(deleteModal.name);

    setDeleteModal(current => (current ? { ...current, pending: true, error: undefined } : current));
    try {
      await deleteScopedValue(
        scopedResourcePath(deleteModal.kind, scope, identity.name, identity.repoSlug)
      );
      if (deleteModal.kind === 'variable') {
        await ensureScopeVariables(scope, true);
        if (selectedVariable === identity.fullName) selectVariable(null);
      } else {
        await ensureScopeSecrets(scope, true);
        if (selectedSecret === identity.fullName) selectSecret(null);
      }

      await loadScopes();
      addToast(
        deleteModal.gitOpsManaged
          ? `${deleteModal.kind === 'variable' ? 'Variable' : 'Secret'} database row removed. GitOps can recreate it on the next sync unless it is removed from GitOps.`
          : deleteModal.kind === 'variable'
            ? 'Variable removed.'
            : 'Secret removed.',
        'success'
      );
      setDeleteModal(null);
      return true;
    } catch (error) {
      console.error('Delete failed', error);
      setDeleteModal(current =>
        current ? { ...current, error: error instanceof Error ? error.message : 'Delete failed.' } : current
      );
      return false;
    } finally {
      setDeleteModal(current => (current ? { ...current, pending: false } : current));
    }
  }, [
    addToast,
    canDeleteScopes,
    deleteModal,
    ensureScopeSecrets,
    ensureScopeVariables,
    loadScopes,
    selectSecret,
    selectVariable,
    selectedSecret,
    selectedVariable,
  ]);

  return {
    chooseSecretSuggestion,
    chooseVariableSuggestion,
    closeDeleteModal,
    closeGitOpsEncryptModal,
    closeScopeModal,
    closeSecretModal,
    closeVariableModal,
    confirmDelete,
    copyGitOpsEncryptedValue,
    deleteModal,
    encryptGitOpsSecretValue,
    gitOpsEncryptModal,
    openDeleteModal,
    openGitOpsEncryptModal,
    openNewScopeModal,
    openSecretCloneModal,
    openSecretCreateModal,
    openSecretUpdateModal,
    openVariableCloneModal,
    openVariableCreateModal,
    openVariableUpdateModal,
    scopeModal,
    secretModal,
    submitScopeModal,
    submitSecretModal,
    submitVariableModal,
    updateGitOpsEncryptValue,
    updateScopeName,
    updateSecretModal,
    updateVariableModal,
    variableModal,
  };
}
