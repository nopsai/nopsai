import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { createAgentProfile, deleteAgentProfile, fetchAgentProfiles, saveAgentProfile, setDefaultAgentProfile } from './api';
import {
  deleteTeamAgentProfile,
  fetchTeamAgentProfiles,
  setTeamDefaultAgentProfile,
  upsertTeamAgentProfile,
  type TeamAgentProfilesResponse,
} from '../teamProfileApi';
import {
  aiResourceLocalName,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  normalizeAIResourceTeamPath,
} from '../aiResourceTeams';
import { teamAgentProfileRecords } from '../teamProfileAdapters';
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

export function useAgentProfiles({
  canManage,
  canManageTeamProfiles = false,
}: {
  canManage: boolean;
  canManageTeamProfiles?: boolean;
}) {
  const [payload, setPayload] = useState(emptyAgentProfilesPayload);
  const [teamProfilesPayload, setTeamProfilesPayload] = useState<TeamAgentProfilesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [teamProfilesLoading, setTeamProfilesLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [teamProfilesError, setTeamProfilesError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [editingTeamPath, setEditingTeamPath] = useState('');
  const [selectedProfile, setSelectedProfile] = useState<AgentProfileRecord | null>(null);
  const [form, setForm] = useState<AgentProfileFormState>(emptyAgentProfileForm);
  const [deleteBlocker, setDeleteBlocker] = useState<AgentProfileDeleteBlocker | null>(null);
  const [panelMode, setPanelMode] = useState<AgentProfilePanelMode | null>(null);
  const teamProfilesRequestRef = useRef(0);

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

  const loadTeamProfiles = useCallback(async (teamPath: string) => {
    const normalizedTeamPath = normalizeAIResourceTeamPath(teamPath);
    const requestID = ++teamProfilesRequestRef.current;
    if (!normalizedTeamPath) {
      setTeamProfilesPayload(null);
      setTeamProfilesError(null);
      setTeamProfilesLoading(false);
      return;
    }

    setTeamProfilesLoading(true);
    setTeamProfilesError(null);
    try {
      const result = await fetchTeamAgentProfiles(normalizedTeamPath);
      if (teamProfilesRequestRef.current !== requestID) return;
      setTeamProfilesPayload(result);
    } catch (err) {
      if (teamProfilesRequestRef.current !== requestID) return;
      setTeamProfilesPayload(null);
      setTeamProfilesError(err instanceof Error ? err.message : 'Unable to load team agent profiles');
    } finally {
      if (teamProfilesRequestRef.current === requestID) setTeamProfilesLoading(false);
    }
  }, []);

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
    setEditingTeamPath('');
    setDeleteBlocker(null);
    setForm(emptyAgentProfileForm);
    setPanelMode('create');
  }, []);

  const startDuplicate = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(null);
    setEditingID(null);
    setEditingTeamPath('');
    setDeleteBlocker(null);
    setForm(duplicateAgentProfileForm(profile));
    setPanelMode('create');
  }, []);

  const startEdit = useCallback((profile: AgentProfileRecord) => {
    setSelectedProfile(profile);
    setEditingID(profile.id);
    setEditingTeamPath(profile.scope === 'team' ? normalizeAIResourceTeamPath(profile.team_path || aiResourceTeamScope(profile.id).teamPath) : '');
    setDeleteBlocker(null);
    setForm(agentProfileFormFromRecord(profile));
    setPanelMode('edit');
  }, []);

  const saveProfile = useCallback(
    async (event: FormEvent) => {
      event.preventDefault();
      const next = agentProfilePayloadFromForm(form);
      if (!next.id) {
        setError('Agent profile id is required.');
        return;
      }
      const inferredTeamPath = editingID ? '' : aiResourceTeamScope(next.id).teamPath;
      const teamPath = normalizeAIResourceTeamPath(editingTeamPath || inferredTeamPath);
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) {
          setError('You need team update permission to save team agent profiles.');
          return;
        }
        const localID = aiResourceLocalName(next.id);
        if (!localID) {
          setError('Team agent profile id is required.');
          return;
        }
        setSaving(true);
        setError(null);
        try {
          const result = await upsertTeamAgentProfile(teamPath, localID, { ...next, id: localID });
          setTeamProfilesPayload(result);
          const scopedID = buildAIResourceScopedID(teamPath, localID);
          const saved = teamAgentProfileRecords(result).find(profile => profile.id === scopedID) || null;
          setSelectedProfile(saved);
          setEditingID(scopedID);
          setEditingTeamPath(teamPath);
          setForm(prev => ({ ...prev, id: scopedID }));
          setPanelMode('edit');
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to save team agent profile');
        } finally {
          setSaving(false);
        }
        return;
      }
      if (!canManage) return;
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
    [canManage, canManageTeamProfiles, editingID, editingTeamPath, form]
  );

  const deleteProfile = useCallback(
    async (id: string, opts?: { force?: boolean; teamPath?: string }) => {
      const teamPath = normalizeAIResourceTeamPath(opts?.teamPath || '');
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) return;
        const localID = aiResourceLocalName(id);
        if (!localID) return;
        setSaving(true);
        setError(null);
        try {
          await deleteTeamAgentProfile(teamPath, localID);
          setDeleteBlocker(null);
          if (editingID === id) {
            setEditingID(null);
            setEditingTeamPath('');
            setSelectedProfile(null);
            setForm(emptyAgentProfileForm);
          }
          setPanelMode(prev => (prev === 'delete' || editingID === id ? null : prev));
          await loadTeamProfiles(teamPath);
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to delete team agent profile');
        } finally {
          setSaving(false);
        }
        return;
      }
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
          setEditingTeamPath('');
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
    [canManage, canManageTeamProfiles, editingID, loadProfiles, loadTeamProfiles]
  );

  const toggleProfileEnabled = useCallback(
    async (profile: AgentProfileRecord) => {
      const teamPath = normalizeAIResourceTeamPath(profile.scope === 'team' ? profile.team_path || aiResourceTeamScope(profile.id).teamPath : '');
      if (teamPath) {
        if ((!canManageTeamProfiles && !canManage) || profile.read_only) return;
        const localID = aiResourceLocalName(profile.id);
        if (!localID) return;
        setSaving(true);
        setError(null);
        try {
          const nextPayload = await upsertTeamAgentProfile(teamPath, localID, {
            ...agentProfilePayloadFromForm(agentProfileFormFromRecord(profile)),
            id: localID,
            enabled: !profile.enabled,
          });
          setTeamProfilesPayload(nextPayload);
          const scopedID = buildAIResourceScopedID(teamPath, localID);
          const updated = teamAgentProfileRecords(nextPayload).find(item => item.id === scopedID) || null;
          if (selectedProfile?.id === profile.id) setSelectedProfile(updated);
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to update team agent profile');
        } finally {
          setSaving(false);
        }
        return;
      }
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
    [canManage, canManageTeamProfiles, selectedProfile?.id]
  );

  const setDefaultProfile = useCallback(
    async (profileID: string, opts?: { teamPath?: string }) => {
      const teamPath = normalizeAIResourceTeamPath(opts?.teamPath || aiResourceTeamScope(profileID).teamPath);
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) return;
        const scopedDefault = aiResourceTeamScope(profileID).teamPath;
        const localDefault = profileID ? (scopedDefault ? profileID : aiResourceLocalName(profileID)) : '';
        setSaving(true);
        setError(null);
        try {
          setTeamProfilesPayload(await setTeamDefaultAgentProfile(teamPath, localDefault));
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to set team default agent profile');
        } finally {
          setSaving(false);
        }
        return;
      }
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
    [canManage, canManageTeamProfiles]
  );

  return {
    payload,
    teamProfilesPayload,
    loading,
    teamProfilesLoading,
    error,
    teamProfilesError,
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
    loadTeamProfiles,
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
