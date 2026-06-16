import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  deleteGitWebhookSource,
  fetchGitWebhookDeliveries,
  fetchGitWebhookSource,
  fetchGitWebhookSources,
  saveGitWebhookSource,
} from './api';
import {
  gitWebhookSourceForm,
  gitWebhookSourceRequest,
  type GitWebhookDelivery,
  type GitWebhookSource,
  type GitWebhookSourceFormState,
} from './model';

export function useGitWebhookSources({
  selectedID,
  canWrite,
  canDelete,
  onSelect,
}: {
  selectedID: string;
  canWrite: boolean;
  canDelete: boolean;
  onSelect: (sourceID: string) => void;
}) {
  const [sources, setSources] = useState<GitWebhookSource[]>([]);
  const [deliveries, setDeliveries] = useState<GitWebhookDelivery[]>([]);
  const [form, setForm] = useState<GitWebhookSourceFormState>(() => gitWebhookSourceForm());
  const [editing, setEditing] = useState<GitWebhookSource | null | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const selected = useMemo(
    () => sources.find(source => source.id === selectedID) || null,
    [selectedID, sources]
  );

  const upsertSource = useCallback((source: GitWebhookSource) => {
    setSources(current =>
      [...current.filter(item => item.id !== source.id), source].sort((left, right) =>
        left.name.localeCompare(right.name)
      )
    );
  }, []);

  const loadSources = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setSources(await fetchGitWebhookSources());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load Git webhook sources');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const timeout = window.setTimeout(() => void loadSources(), 0);
    return () => window.clearTimeout(timeout);
  }, [loadSources]);

  useEffect(() => {
    if (!selectedID) {
      const timeout = window.setTimeout(() => setDeliveries([]), 0);
      return () => window.clearTimeout(timeout);
    }
    let cancelled = false;
    setDetailLoading(true);
    setError(null);
    void Promise.all([
      fetchGitWebhookSource(selectedID),
      fetchGitWebhookDeliveries(selectedID),
    ])
      .then(([source, records]) => {
        if (cancelled) return;
        upsertSource(source);
        setDeliveries(records);
      })
      .catch(err => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Unable to load Git webhook source');
        }
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedID, upsertSource]);

  const startCreate = useCallback(() => {
    if (!canWrite) return;
    setEditing(null);
    setForm(gitWebhookSourceForm());
    setError(null);
  }, [canWrite]);

  const startEdit = useCallback((source: GitWebhookSource) => {
    if (!canWrite) return;
    setEditing(source);
    setForm(gitWebhookSourceForm(source));
    setError(null);
  }, [canWrite]);

  const closeEditor = useCallback(() => {
    if (saving) return;
    setEditing(undefined);
  }, [saving]);

  const submit = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    if (!canWrite || saving) return;
    setSaving(true);
    setError(null);
    try {
      const request = gitWebhookSourceRequest(form);
      const source = await saveGitWebhookSource(request, editing?.id);
      upsertSource(source);
      setEditing(undefined);
      onSelect(source.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save Git webhook source');
    } finally {
      setSaving(false);
    }
  }, [canWrite, editing, form, onSelect, saving, upsertSource]);

  const setEnabled = useCallback(async (source: GitWebhookSource, enabled: boolean) => {
    if (!canWrite || saving) return;
    if (source.managed_by_config_repo && !window.confirm(
      `This Git webhook source is managed by GitOps. ${enabled ? 'Enabling' : 'Disabling'} it saves a database override that the next GitOps sync can replace unless it is pushed to GitOps. Continue?`
    )) {
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const request = gitWebhookSourceRequest({ ...gitWebhookSourceForm(source), enabled });
      upsertSource(await saveGitWebhookSource(request, source.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to update Git webhook source');
    } finally {
      setSaving(false);
    }
  }, [canWrite, saving, upsertSource]);

  const remove = useCallback(async (source: GitWebhookSource) => {
    if (!canDelete || saving) return;
    const message = source.managed_by_config_repo
      ? `Delete Git webhook source ${source.id}? This removes the database row; the next GitOps sync can recreate it from the repository. Delivery history will also be deleted.`
      : `Delete Git webhook source ${source.id}? Delivery history will also be deleted.`;
    if (!window.confirm(message)) return;
    setSaving(true);
    setError(null);
    try {
      await deleteGitWebhookSource(source.id);
      setSources(current => current.filter(item => item.id !== source.id));
      setDeliveries([]);
      onSelect('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete Git webhook source');
    } finally {
      setSaving(false);
    }
  }, [canDelete, onSelect, saving]);

  return {
    sources,
    selected,
    deliveries,
    form,
    setForm,
    editorOpen: editing !== undefined,
    editing,
    loading,
    detailLoading,
    saving,
    error,
    loadSources,
    startCreate,
    startEdit,
    closeEditor,
    submit,
    setEnabled,
    remove,
    onSelect,
  };
}

export type GitWebhookSourcesController = ReturnType<typeof useGitWebhookSources>;
