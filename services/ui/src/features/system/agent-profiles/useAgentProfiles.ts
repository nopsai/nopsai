import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { createAgentProfile, deleteAgentProfile, fetchAgentProfiles, saveAgentProfile, setDefaultAgentProfile } from './api';
import {
  agentProfileFormFromRecord,
  agentProfilePayloadFromForm,
  duplicateAgentProfileForm,
  emptyAgentProfileForm,
  emptyAgentProfilesPayload,
  type AgentProfileDeleteBlocker,
  type AgentProfileFormState,
  type AgentProfilePanelMode,
  type AgentProfileRecord,
} from './model';

export function useAgentProfiles({ canManage }: { canManage: boolean }) {
  const [payload, setPayload] = useState(emptyAgentProfilesPayload);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [selectedProfile, setSelectedProfile] = useState<AgentProfileRecord | null>(null);
  const [form, setForm] = useState<AgentProfileFormState>(emptyAgentProfileForm);
  const [deleteBlocker, setDeleteBlocker] = useState<AgentProfileDeleteBlocker | null>(null);
  const [panelMode, setPanelMode] = useState<AgentProfilePanelMode | null>(null);

  const loadProfiles = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setPayload(await fetchAgentProfiles());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load agent profiles');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadProfiles();
  }, [loadProfiles]);

  const openView = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(profile);
    setEditingID(null);
    setDeleteBlocker(null);
    setForm(agentProfileFormFromRecord(profile));
    setPanelMode('view');
  }, []);

  const openUsage = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(profile);
    setEditingID(null);
    setDeleteBlocker(null);
    setPanelMode('usage');
  }, []);

  const openSource = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(profile);
    setEditingID(null);
    setDeleteBlocker(null);
    setPanelMode('source');
  }, []);

  const startCreate = useCallback(() => {
    setSelectedProfile(null);
    setEditingID(null);
    setDeleteBlocker(null);
    setForm(emptyAgentProfileForm);
    setPanelMode('create');
  }, []);

  const startDuplicate = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(null);
    setEditingID(null);
    setDeleteBlocker(null);
    setForm(duplicateAgentProfileForm(profile));
    setPanelMode('create');
  }, []);

  const startEdit = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(profile);
    setEditingID(profile.id);
    setDeleteBlocker(null);
    setForm(agentProfileFormFromRecord(profile));
    setPanelMode('edit');
  }, []);

  const saveProfile = useCallback(
    async (event: FormEvent) => {
      event.preventDefault();
      if (!canManage) return;
      const next = agentProfilePayloadFromForm(form);
      if (!next.id) {
        setError('Agent profile id is required.');
        return;
      }
      setSaving(true);
      setError(null);
      try {
        const nextPayload = editingID ? await saveAgentProfile(form) : await createAgentProfile(form);
        setPayload(nextPayload);
        const saved = nextPayload.profiles.find(profile => profile.id === next.id) || null;
        setSelectedProfile(saved);
        setEditingID(next.id);
        setPanelMode('edit');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save agent profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingID, form]
  );

  const deleteProfile = useCallback(
    async (id: string, opts?: { force?: boolean }) => {
      if (!canManage) return;
      setSaving(true);
      setError(null);
      try {
        const result = await deleteAgentProfile(id, opts);
        if (result.status === 'conflict') {
          setDeleteBlocker({ id, references: result.references });
          setPanelMode('delete');
          return;
        }
        setDeleteBlocker(null);
        if (editingID === id) {
          setEditingID(null);
          setSelectedProfile(null);
          setForm(emptyAgentProfileForm);
        }
        setPanelMode(prev => (prev === 'delete' || editingID === id ? null : prev));
        await loadProfiles();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to delete agent profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingID, loadProfiles]
  );

  const toggleProfileEnabled = useCallback(
    async (profile: AgentProfileRecord) => {
      if (!canManage || profile.read_only) return;
      setSaving(true);
      setError(null);
      try {
        const nextPayload = await saveAgentProfile({ ...agentProfileFormFromRecord(profile), enabled: !profile.enabled });
        setPayload(nextPayload);
        const updated = nextPayload.profiles.find(item => item.id === profile.id) || null;
        if (selectedProfile?.id === profile.id) setSelectedProfile(updated);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to update agent profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, selectedProfile?.id]
  );

  const setDefaultProfile = useCallback(
    async (profileID: string) => {
      if (!canManage) return;
      setSaving(true);
      setError(null);
      try {
        setPayload(await setDefaultAgentProfile(profileID));
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to set default agent profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage]
  );

  return {
    payload,
    loading,
    error,
    saving,
    editingID,
    selectedProfile,
    form,
    setForm,
    deleteBlocker,
    setDeleteBlocker,
    panelMode,
    setPanelMode,
    loadProfiles,
    openView,
    openUsage,
    openSource,
    startCreate,
    startDuplicate,
    startEdit,
    saveProfile,
    deleteProfile,
    toggleProfileEnabled,
    setDefaultProfile,
  };
}
