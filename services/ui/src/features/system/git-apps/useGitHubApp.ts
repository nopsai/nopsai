import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  deleteGitHubAppInstallation,
  fetchGitHubApp,
  fetchGitHubAppInstallationRepositories,
  refreshGitHubAppInstallation,
  saveGitHubApp,
  saveGitHubAppInstallation,
  startGitHubAppInstall,
  startGitHubAppRegistration,
  verifyGitHubAppInstallation,
} from './api.js';
import { clearGitHubAppCallbackParams, submitGitHubAppManifest } from './manifestForm.js';
import {
  emptyGitHubApp,
  emptyGitHubAppConnectForm,
  gitHubAppForm,
  gitHubAppInstallationForm,
  readGitHubAppCallbackResult,
  type GitHubAppConnectFormState,
  type GitHubAppFormState,
  type GitHubAppInstallation,
  type GitHubAppInstallationFormState,
  type GitHubAppResource,
} from './model.js';

export function useGitHubApp({
  enabled,
  canManage,
  addToast,
}: {
  enabled: boolean;
  canManage: boolean;
  addToast?: (message: string, tone?: 'success' | 'error' | 'info') => void;
}) {
  const [app, setApp] = useState<GitHubAppResource>(emptyGitHubApp);
  const [form, setForm] = useState<GitHubAppFormState>(() => gitHubAppForm(emptyGitHubApp));
  const [installationForm, setInstallationForm] = useState<GitHubAppInstallationFormState>(() => gitHubAppInstallationForm());
  const [editingInstallation, setEditingInstallation] = useState<GitHubAppInstallation | null | undefined>(undefined);
  const [selectedInstallationID, setSelectedInstallationID] = useState('');
  const [connectForm, setConnectForm] = useState<GitHubAppConnectFormState>(() => ({ ...emptyGitHubAppConnectForm }));
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const selectedInstallation = useMemo(
    () => app.installations.find(installation => installation.installation_id === selectedInstallationID) || null,
    [app.installations, selectedInstallationID]
  );

  const updateInstallation = useCallback((installation: GitHubAppInstallation) => {
    setApp(current => ({
      ...current,
      installations: upsertInstallation(current.installations, installation),
    }));
  }, []);

  const loadApp = useCallback(async () => {
    if (!enabled) return;
    setLoading(true);
    setError(null);
    try {
      const next = await fetchGitHubApp();
      setApp(next);
      setForm(gitHubAppForm(next));
      setSelectedInstallationID(current =>
        current && next.installations.some(installation => installation.installation_id === current)
          ? current
          : next.installations[0]?.installation_id || ''
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load GitHub App');
    } finally {
      setLoading(false);
    }
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return undefined;
    const timeout = window.setTimeout(() => void loadApp(), 0);
    return () => window.clearTimeout(timeout);
  }, [enabled, loadApp]);

  // GitHub sends the operator back here after the App is created or installed.
  useEffect(() => {
    if (!enabled || typeof window === 'undefined') return;
    const result = readGitHubAppCallbackResult(window.location.search);
    if (!result) return;
    clearGitHubAppCallbackParams();
    if (result.tone === 'error') {
      setError(result.message);
    }
    addToast?.(result.message, result.tone);
  }, [addToast, enabled]);

  const submitApp = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || saving) return;
    setSaving(true);
    setError(null);
    try {
      const next = await saveGitHubApp(form, app.installations);
      setApp(next);
      setForm(gitHubAppForm(next));
      addToast?.('GitHub App saved', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save GitHub App');
      addToast?.('GitHub App save failed', 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, app.installations, canManage, form, saving]);

  // Registration hands the browser to GitHub, so the page navigates away on
  // success and only failures return here.
  const connectGitHubApp = useCallback(async () => {
    if (!canManage || connecting) return;
    setConnecting(true);
    setError(null);
    try {
      submitGitHubAppManifest(await startGitHubAppRegistration(connectForm));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to start GitHub App registration');
      addToast?.('GitHub App registration failed to start', 'error');
      setConnecting(false);
    }
  }, [addToast, canManage, connectForm, connecting]);

  const installGitHubApp = useCallback(async () => {
    if (!canManage || connecting) return;
    setConnecting(true);
    setError(null);
    try {
      const start = await startGitHubAppInstall();
      if (!start.install_url) {
        throw new Error('GitHub App install link is unavailable');
      }
      window.location.assign(start.install_url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to start the GitHub App installation');
      addToast?.('GitHub App installation failed to start', 'error');
      setConnecting(false);
    }
  }, [addToast, canManage, connecting]);

  const startCreateInstallation = useCallback(() => {
    if (!canManage) return;
    setEditingInstallation(null);
    setInstallationForm(gitHubAppInstallationForm());
    setError(null);
  }, [canManage]);

  const startEditInstallation = useCallback((installation: GitHubAppInstallation) => {
    if (!canManage) return;
    setEditingInstallation(installation);
    setInstallationForm(gitHubAppInstallationForm(installation));
    setError(null);
  }, [canManage]);

  const closeInstallationEditor = useCallback(() => {
    if (saving) return;
    setEditingInstallation(undefined);
  }, [saving]);

  const submitInstallation = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || saving) return;
    setSaving(true);
    setError(null);
    try {
      const installation = await saveGitHubAppInstallation(installationForm);
      updateInstallation(installation);
      setSelectedInstallationID(installation.installation_id);
      setEditingInstallation(undefined);
      addToast?.('GitHub App installation saved', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save GitHub App installation');
      addToast?.('GitHub App installation save failed', 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, canManage, installationForm, saving, updateInstallation]);

  const removeInstallation = useCallback(async (installation: GitHubAppInstallation) => {
    if (!canManage || saving) return;
    if (!window.confirm(`Delete GitHub App installation ${installation.installation_id}?`)) return;
    setSaving(true);
    setError(null);
    try {
      await deleteGitHubAppInstallation(installation.installation_id);
      setApp(current => ({
        ...current,
        installations: current.installations.filter(item => item.installation_id !== installation.installation_id),
      }));
      setSelectedInstallationID(current => current === installation.installation_id ? '' : current);
      addToast?.('GitHub App installation deleted', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete GitHub App installation');
      addToast?.('GitHub App installation delete failed', 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, canManage, saving]);

  const verifyInstallation = useCallback(async (installation: GitHubAppInstallation) => {
    if (!canManage || saving) return;
    setSaving(true);
    setError(null);
    try {
      updateInstallation(await verifyGitHubAppInstallation(installation.installation_id));
      addToast?.('GitHub App installation verified', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to verify GitHub App installation');
      addToast?.('GitHub App installation verify failed', 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, canManage, saving, updateInstallation]);

  const refreshInstallation = useCallback(async (installation: GitHubAppInstallation) => {
    if (saving) return;
    setSaving(true);
    setError(null);
    try {
      const refreshed = await refreshGitHubAppInstallation(installation.installation_id);
      updateInstallation(refreshed);
      setSelectedInstallationID(refreshed.installation_id);
      addToast?.('GitHub repositories refreshed', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to refresh GitHub repositories');
      addToast?.('GitHub repository refresh failed', 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, saving, updateInstallation]);

  const loadRepositories = useCallback(async (installation: GitHubAppInstallation) => {
    if (saving) return;
    setSaving(true);
    setError(null);
    try {
      const repositories = await fetchGitHubAppInstallationRepositories(installation.installation_id);
      updateInstallation({
        ...installation,
        repositories,
        accessible_repositories: repositories.length,
      });
      setSelectedInstallationID(installation.installation_id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load GitHub repositories');
    } finally {
      setSaving(false);
    }
  }, [saving, updateInstallation]);

  return {
    app,
    form,
    setForm,
    installationForm,
    setInstallationForm,
    editingInstallation,
    installationEditorOpen: editingInstallation !== undefined,
    selectedInstallation,
    selectedInstallationID,
    setSelectedInstallationID,
    connectForm,
    setConnectForm,
    connecting,
    loading,
    saving,
    error,
    loadApp,
    submitApp,
    connectGitHubApp,
    installGitHubApp,
    startCreateInstallation,
    startEditInstallation,
    closeInstallationEditor,
    submitInstallation,
    removeInstallation,
    verifyInstallation,
    refreshInstallation,
    loadRepositories,
  };
}

export type GitHubAppController = ReturnType<typeof useGitHubApp>;

function upsertInstallation(
  installations: readonly GitHubAppInstallation[],
  installation: GitHubAppInstallation
): GitHubAppInstallation[] {
  return [
    ...installations.filter(item => item.installation_id !== installation.installation_id),
    installation,
  ].sort((left, right) =>
    (left.account_login || left.installation_id).localeCompare(right.account_login || right.installation_id)
  );
}
