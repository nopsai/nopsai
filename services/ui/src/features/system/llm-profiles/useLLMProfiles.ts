import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  deleteLLMProfile,
  fetchLLMProfiles,
  saveDefaultLLMProfile,
  saveLLMProfile,
  testLLMProfile,
} from './api';
import {
  deleteTeamLLMProfile,
  fetchTeamLLMProfiles,
  setTeamDefaultLLMProfile,
  upsertTeamLLMProfile,
  type TeamLLMProfilesResponse,
} from '../teamProfileApi';
import {
  aiResourceLocalName,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  normalizeAIResourceTeamPath,
} from '../aiResourceTeams';
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

export function useLLMProfiles({
  canManage,
  canManageTeamProfiles = false,
}: {
  canManage: boolean;
  canManageTeamProfiles?: boolean;
}) {
  const [payload, setPayload] = useState(emptyLLMProfilesPayload);
  const [teamProfilesPayload, setTeamProfilesPayload] = useState<TeamLLMProfilesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [teamProfilesLoading, setTeamProfilesLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [teamProfilesError, setTeamProfilesError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [editingTeamPath, setEditingTeamPath] = useState('');
  const [form, setForm] = useState<LLMProfileFormState>(emptyLLMProfileForm);
  const [deleteBlocker, setDeleteBlocker] = useState<LLMProfileDeleteBlocker | null>(null);
  const [panelMode, setPanelMode] = useState<LLMProfilePanelMode | null>(null);
  const teamProfilesRequestRef = useRef(0);

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
      const result = await fetchTeamLLMProfiles(normalizedTeamPath);
      if (teamProfilesRequestRef.current !== requestID) return;
      setTeamProfilesPayload(result);
    } catch (err) {
      if (teamProfilesRequestRef.current !== requestID) return;
      setTeamProfilesPayload(null);
      setTeamProfilesError(err instanceof Error ? err.message : 'Unable to load team LLM profiles');
    } finally {
      if (teamProfilesRequestRef.current === requestID) setTeamProfilesLoading(false);
    }
  }, []);

  const startCreate = useCallback(() => {
    setEditingName(null);
    setEditingTeamPath('');
    setForm(emptyLLMProfileForm);
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('create');
  }, []);

  const startEdit = useCallback((profile: LLMProfileRecord) => {
    setEditingName(profile.name);
    setEditingTeamPath(profile.scope === 'team' ? normalizeAIResourceTeamPath(profile.team_path || aiResourceTeamScope(profile.name).teamPath) : '');
    setForm(llmProfileFormFromRecord(profile));
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('edit');
  }, []);

  const saveProfile = useCallback(
    async (event: FormEvent) => {
      event.preventDefault();
      const next = llmProfilePayloadFromForm(form);
      if (!next.name) {
        setError('Profile name is required.');
        return;
      }
      const inferredTeamPath = editingName ? '' : aiResourceTeamScope(next.name).teamPath;
      const teamPath = normalizeAIResourceTeamPath(editingTeamPath || inferredTeamPath);
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) {
          setError('You need team update permission to save team LLM profiles.');
          return;
        }
        const localName = aiResourceLocalName(next.name);
        if (!localName) {
          setError('Team profile name is required.');
          return;
        }
        setSaving(true);
        setError(null);
        try {
          const result = await upsertTeamLLMProfile(teamPath, localName, { ...next, name: localName });
          setTeamProfilesPayload(result);
          setEditingName(buildAIResourceScopedID(teamPath, localName));
          setEditingTeamPath(teamPath);
          setForm(prev => ({ ...prev, name: buildAIResourceScopedID(teamPath, localName) }));
          setPanelMode('edit');
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to save team LLM profile');
        } finally {
          setSaving(false);
        }
        return;
      }
      if (!canManage) return;
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
    [canManage, canManageTeamProfiles, editingName, editingTeamPath, form]
  );

  const saveDefaultProfile = useCallback(
    async (nextDefault: string, opts?: { teamPath?: string }) => {
      const teamPath = normalizeAIResourceTeamPath(opts?.teamPath || aiResourceTeamScope(nextDefault).teamPath);
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) return;
        const scopedDefault = aiResourceTeamScope(nextDefault).teamPath;
        const localDefault = nextDefault ? (scopedDefault ? nextDefault : aiResourceLocalName(nextDefault)) : '';
        setSaving(true);
        setError(null);
        try {
          setTeamProfilesPayload(await setTeamDefaultLLMProfile(teamPath, localDefault));
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to update team default profile');
        } finally {
          setSaving(false);
        }
        return;
      }
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
    [canManage, canManageTeamProfiles, payload.profiles]
  );

  const deleteProfile = useCallback(
    async (name: string, opts?: { force?: boolean; migrateTo?: string; teamPath?: string }) => {
      const teamPath = normalizeAIResourceTeamPath(opts?.teamPath || '');
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) return;
        const localName = aiResourceLocalName(name);
        if (!localName) return;
        setSaving(true);
        setError(null);
        try {
          await deleteTeamLLMProfile(teamPath, localName);
          setDeleteBlocker(null);
          if (editingName === name) {
            setEditingName(null);
            setEditingTeamPath('');
            setForm(emptyLLMProfileForm);
          }
          setPanelMode(prev => (prev === 'delete' || editingName === name ? null : prev));
          await loadTeamProfiles(teamPath);
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to delete team LLM profile');
        } finally {
          setSaving(false);
        }
        return;
      }
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
          setEditingTeamPath('');
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
    [canManage, canManageTeamProfiles, editingName, loadProfiles, loadTeamProfiles, payload.profiles]
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
    teamProfilesPayload,
    loading,
    teamProfilesLoading,
    error,
    teamProfilesError,
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
    loadTeamProfiles,
    startCreate,
    startEdit,
    saveProfile,
    saveDefaultProfile,
    deleteProfile,
    testProfile,
  };
}
