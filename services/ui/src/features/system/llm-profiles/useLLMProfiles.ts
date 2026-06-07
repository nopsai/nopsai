import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  deleteLLMProfile,
  fetchLLMProfiles,
  saveDefaultLLMProfile,
  saveLLMProfile,
  testLLMProfile,
} from './api';
import {
  emptyLLMProfileForm,
  emptyLLMProfilesPayload,
  llmProfileFormFromRecord,
  llmProfilePayloadFromForm,
  type LLMProfileDeleteBlocker,
  type LLMProfileFormState,
  type LLMProfilePanelMode,
  type LLMProfileRecord,
} from './model';

export function useLLMProfiles({ canManage }: { canManage: boolean }) {
  const [payload, setPayload] = useState(emptyLLMProfilesPayload);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [form, setForm] = useState<LLMProfileFormState>(emptyLLMProfileForm);
  const [deleteBlocker, setDeleteBlocker] = useState<LLMProfileDeleteBlocker | null>(null);
  const [panelMode, setPanelMode] = useState<LLMProfilePanelMode | null>(null);

  const loadProfiles = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setPayload(await fetchLLMProfiles());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load LLM profiles');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadProfiles();
  }, [loadProfiles]);

  const startCreate = useCallback(() => {
    setEditingName(null);
    setForm(emptyLLMProfileForm);
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('create');
  }, []);

  const startEdit = useCallback((profile: LLMProfileRecord) => {
    setEditingName(profile.name);
    setForm(llmProfileFormFromRecord(profile));
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('edit');
  }, []);

  const saveProfile = useCallback(
    async (event: FormEvent) => {
      event.preventDefault();
      if (!canManage) return;
      const next = llmProfilePayloadFromForm(form);
      if (!next.name) {
        setError('Profile name is required.');
        return;
      }
      setSaving(true);
      setError(null);
      try {
        const result = await saveLLMProfile(form);
        setPayload(result.payload);
        setEditingName(result.name);
        setPanelMode('edit');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save LLM profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, form]
  );

  const saveDefaultProfile = useCallback(
    async (nextDefault: string) => {
      if (!canManage || !nextDefault) return;
      setSaving(true);
      setError(null);
      try {
        setPayload(await saveDefaultLLMProfile(nextDefault, payload.profiles));
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to update default profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, payload.profiles]
  );

  const deleteProfile = useCallback(
    async (name: string, opts?: { force?: boolean; migrateTo?: string }) => {
      if (!canManage) return;
      setSaving(true);
      setError(null);
      try {
        const result = await deleteLLMProfile(name, opts);
        if (result.status === 'conflict') {
          const fallback = payload.profiles.find(profile => profile.name !== name)?.name || '';
          setDeleteBlocker({ name, references: result.references, migrateTo: fallback });
          setPanelMode('delete');
          return;
        }
        setDeleteBlocker(null);
        if (editingName === name) {
          setEditingName(null);
          setForm(emptyLLMProfileForm);
        }
        setPanelMode(prev => (prev === 'delete' || editingName === name ? null : prev));
        await loadProfiles();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to delete LLM profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingName, loadProfiles, payload.profiles]
  );

  const testProfile = useCallback(async (name: string) => {
    setTesting(name);
    setTestResult(null);
    setError(null);
    try {
      const reply = await testLLMProfile(name);
      setTestResult(`${name}: ${reply}`);
    } catch (err) {
      setTestResult(`${name}: ${err instanceof Error ? err.message : 'test failed'}`);
    } finally {
      setTesting(null);
    }
  }, []);

  return {
    payload,
    loading,
    error,
    saving,
    testing,
    testResult,
    editingName,
    form,
    setForm,
    deleteBlocker,
    setDeleteBlocker,
    panelMode,
    setPanelMode,
    loadProfiles,
    startCreate,
    startEdit,
    saveProfile,
    saveDefaultProfile,
    deleteProfile,
    testProfile,
  };
}
