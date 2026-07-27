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
import { teamAgentProfileRecords, teamDefaultProfileAPIValue } from '../teamProfileAdapters';
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
      const targetTeamPath = normalizeAIResourceTeamPath(aiResourceTeamScope(next.id).teamPath);
      const localID = aiResourceLocalName(next.id);
      const targetID = buildAIResourceScopedID(targetTeamPath, localID);
      const originalTeamPath = editingID ? normalizeAIResourceTeamPath(editingTeamPath) : '';
      const movingProfile = Boolean(editingID && (targetID !== editingID || targetTeamPath !== originalTeamPath));
      if (!localID) {
        setError(targetTeamPath ? 'Team agent profile id is required.' : 'Agent profile id is required.');
        return;
      }
      if (movingProfile && !originalTeamPath && targetTeamPath && payload.default_profile === editingID) {
        setError('Default agent profiles cannot be moved to a team. Change the global default profile first.');
        return;
      }
      if (movingProfile && !originalTeamPath && targetTeamPath && !canManage) {
        setError('You need system update permission to move a global agent profile into a team.');
        return;
      }
      const references = selectedProfile?.id === editingID
        ? selectedProfile.references
        : payload.profiles.find(profile => profile.id === editingID)?.references || [];
      if (movingProfile && !originalTeamPath && targetTeamPath && references.length > 0) {
        setError(`Agent profile ${editingID} cannot be moved because it is still referenced by ${references.join(', ')}.`);
        return;
      }
      if (targetTeamPath) {
        if (!canManageTeamProfiles && !canManage) {
          setError('You need team update permission to save team agent profiles.');
          return;
        }
        setSaving(true);
        setError(null);
        try {
          const result = await upsertTeamAgentProfile(targetTeamPath, localID, { ...next, id: localID });
          if (movingProfile && editingID) {
            if (originalTeamPath) {
              await deleteTeamAgentProfile(originalTeamPath, aiResourceLocalName(editingID));
            } else {
              const deleteResult = await deleteAgentProfile(editingID);
              if (deleteResult.status === 'conflict') {
                setDeleteBlocker({ id: editingID, references: deleteResult.references });
                setPanelMode('delete');
                setError(`Agent profile ${editingID} was saved for /${targetTeamPath}, but the original global profile is still referenced.`);
                return;
              }
            }
          }
          setTeamProfilesPayload(result);
          const scopedID = buildAIResourceScopedID(targetTeamPath, localID);
          const saved = teamAgentProfileRecords(result).find(profile => profile.id === scopedID) || null;
          setSelectedProfile(saved);
          setEditingID(scopedID);
          setEditingTeamPath(targetTeamPath);
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
        const targetForm = { ...form, id: targetID };
        const nextPayload = editingID ? await saveAgentProfile(targetForm) : await createAgentProfile(targetForm);
        if (movingProfile && editingID && originalTeamPath) {
          await deleteTeamAgentProfile(originalTeamPath, aiResourceLocalName(editingID));
          await loadTeamProfiles(originalTeamPath);
        }
        setPayload(nextPayload);
        const saved = nextPayload.profiles.find(profile => profile.id === targetID) || null;
        setSelectedProfile(saved);
        setEditingID(targetID);
        setEditingTeamPath('');
        setForm(prev => ({ ...prev, id: targetID }));
        setPanelMode('edit');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save agent profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, canManageTeamProfiles, editingID, editingTeamPath, form, loadTeamProfiles, payload.default_profile, payload.profiles, selectedProfile]
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
        const defaultForAPI = teamDefaultProfileAPIValue(
          teamPath,
          profileID,
          teamProfilesPayload?.profiles.map(profile => profile.id) || []
        );
        setSaving(true);
        setError(null);
        try {
          setTeamProfilesPayload(await setTeamDefaultAgentProfile(teamPath, defaultForAPI));
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
    [canManage, canManageTeamProfiles, teamProfilesPayload]
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
