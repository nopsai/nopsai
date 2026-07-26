import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  deleteMCPProfile,
  deleteMCPServer,
  discoverMCPServer,
  fetchMCPRegistry,
  saveMCPProfile,
  saveMCPServer,
  testMCPProfile,
} from './api';
import {
  deleteTeamMCPProfile,
  fetchTeamMCPProfiles,
  upsertTeamMCPProfile,
  type TeamMCPProfilesResponse,
} from '../teamProfileApi';
import {
  aiResourceLocalName,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  normalizeAIResourceTeamPath,
} from '../aiResourceTeams';
import { teamMCPProfileRecords } from '../teamProfileAdapters';
import {
  emptyMCPProfileForm,
  emptyMCPServerForm,
  mcpProfileFormFromRecord,
  mcpProfilePayloadFromForm,
  mcpServerFormFromRecord,
  mcpServerPayloadFromForm,
  setProfileServerToolText,
  toggleProfileToolSelection,
  type MCPPanelMode,
  type MCPProfileFormState,
  type MCPProfileRecord,
  type MCPServerFormState,
  type MCPServerRecord,
} from './model';

export function useMCPRegistry({
  canManage,
  canManageTeamProfiles = false,
}: {
  canManage: boolean;
  canManageTeamProfiles?: boolean;
}) {
  const [innerTab, setInnerTab] = useState<'servers' | 'profiles'>('servers');
  const [servers, setServers] = useState<MCPServerRecord[]>([]);
  const [profiles, setProfiles] = useState<MCPProfileRecord[]>([]);
  const [teamProfilesPayload, setTeamProfilesPayload] = useState<TeamMCPProfilesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [teamProfilesLoading, setTeamProfilesLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [teamProfilesError, setTeamProfilesError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [serverForm, setServerForm] = useState<MCPServerFormState>(emptyMCPServerForm);
  const [profileForm, setProfileForm] = useState<MCPProfileFormState>(emptyMCPProfileForm);
  const [editingServer, setEditingServer] = useState<string | null>(null);
  const [editingProfile, setEditingProfile] = useState<string | null>(null);
  const [editingProfileTeamPath, setEditingProfileTeamPath] = useState('');
  const [panelMode, setPanelMode] = useState<MCPPanelMode | null>(null);
  const teamProfilesRequestRef = useRef(0);

  const loadMCP = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const registry = await fetchMCPRegistry();
      setServers(registry.servers);
      setProfiles(registry.profiles);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load MCP registry');
    } finally {
      setLoading(false);
    }
  }, []);

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
      const result = await fetchTeamMCPProfiles(normalizedTeamPath);
      if (teamProfilesRequestRef.current !== requestID) return;
      setTeamProfilesPayload(result);
    } catch (err) {
      if (teamProfilesRequestRef.current !== requestID) return;
      setTeamProfilesPayload(null);
      setTeamProfilesError(err instanceof Error ? err.message : 'Unable to load team MCP profiles');
    } finally {
      if (teamProfilesRequestRef.current === requestID) setTeamProfilesLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadMCP();
  }, [loadMCP]);

  const startServerCreate = useCallback(() => {
    setEditingServer(null);
    setServerForm(emptyMCPServerForm);
    setInnerTab('servers');
    setPanelMode('server-create');
  }, []);

  const startServerEdit = useCallback((server: MCPServerRecord) => {
    setEditingServer(server.name);
    setServerForm(mcpServerFormFromRecord(server));
    setInnerTab('servers');
    setPanelMode('server-edit');
  }, []);

  const saveServer = useCallback(
    async (event: FormEvent) => {
      event.preventDefault();
      if (!canManage) return;
      const payload = mcpServerPayloadFromForm(serverForm);
      if (payload == null) {
        setError('MCP server headers must be a JSON object with string keys and values.');
        return;
      }
      if (!payload.name) {
        setError('MCP server name is required.');
        return;
      }
      const targetTeamPath = normalizeAIResourceTeamPath(aiResourceTeamScope(payload.name).teamPath);
      const localName = aiResourceLocalName(payload.name);
      const targetName = buildAIResourceScopedID(targetTeamPath, localName);
      const movingServer = Boolean(editingServer && targetName !== editingServer);
      if (!localName) {
        setError(targetTeamPath ? 'Team MCP server name is required.' : 'MCP server name is required.');
        return;
      }
      if (movingServer && editingServer) {
        const profileReferences = [...profiles, ...teamMCPProfileRecords(teamProfilesPayload)]
          .filter(profile => profile.servers.some(ref => ref.server === editingServer))
          .map(profile => profile.name)
          .sort((a, b) => a.localeCompare(b));
        if (profileReferences.length > 0) {
          setError(`MCP server ${editingServer} cannot be moved because it is still referenced by profiles: ${profileReferences.join(', ')}.`);
          return;
        }
      }
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        const nextServers = await saveMCPServer({ ...payload, name: targetName }, editingServer);
        if (movingServer && editingServer) {
          await deleteMCPServer(editingServer);
          await loadMCP();
        } else {
          setServers(nextServers);
        }
        setEditingServer(targetName);
        setServerForm(prev => ({ ...prev, name: targetName }));
        setPanelMode('server-edit');
        setMessage(`Saved MCP server ${targetName}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save MCP server');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingServer, loadMCP, profiles, serverForm, teamProfilesPayload]
  );

  const deleteServer = useCallback(
    async (name: string) => {
      if (!canManage) return;
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        await deleteMCPServer(name);
        if (editingServer === name) {
          setEditingServer(null);
          setServerForm(emptyMCPServerForm);
          setPanelMode(prev => (prev === 'server-edit' ? null : prev));
        }
        await loadMCP();
        setMessage(`Deleted MCP server ${name}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to delete MCP server');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingServer, loadMCP]
  );

  const discoverServer = useCallback(
    async (name: string) => {
      setTesting(name);
      setError(null);
      setMessage(null);
      try {
        await discoverMCPServer(name);
        await loadMCP();
        setMessage(`Discovered tools for ${name}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to discover MCP tools');
      } finally {
        setTesting(null);
      }
    },
    [loadMCP]
  );

  const startProfileCreate = useCallback(() => {
    setEditingProfile(null);
    setEditingProfileTeamPath('');
    setProfileForm(emptyMCPProfileForm);
    setInnerTab('profiles');
    setPanelMode('profile-create');
  }, []);

  const startProfileEdit = useCallback((profile: MCPProfileRecord) => {
    setEditingProfile(profile.name);
    setEditingProfileTeamPath(profile.scope === 'team'
      ? normalizeAIResourceTeamPath(profile.team_path || aiResourceTeamScope(profile.name).teamPath)
      : ''
    );
    setProfileForm(mcpProfileFormFromRecord(profile));
    setInnerTab('profiles');
    setPanelMode('profile-edit');
  }, []);

  const toggleProfileTool = useCallback((serverName: string, toolName: string) => {
    setProfileForm(prev => toggleProfileToolSelection(prev, serverName, toolName));
  }, []);

  const setProfileServerTools = useCallback((serverName: string, value: string) => {
    setProfileForm(prev => setProfileServerToolText(prev, serverName, value));
  }, []);

  const saveProfile = useCallback(
    async (event: FormEvent) => {
      event.preventDefault();
      const payload = mcpProfilePayloadFromForm(profileForm);
      if (!payload.name) {
        setError('MCP profile name is required.');
        return;
      }
      const targetTeamPath = normalizeAIResourceTeamPath(aiResourceTeamScope(payload.name).teamPath);
      const localName = aiResourceLocalName(payload.name);
      const targetName = buildAIResourceScopedID(targetTeamPath, localName);
      const originalTeamPath = editingProfile ? normalizeAIResourceTeamPath(editingProfileTeamPath) : '';
      const movingProfile = Boolean(editingProfile && (targetName !== editingProfile || targetTeamPath !== originalTeamPath));
      if (!localName) {
        setError(targetTeamPath ? 'Team MCP profile name is required.' : 'MCP profile name is required.');
        return;
      }
      if (targetTeamPath) {
        if (movingProfile && !originalTeamPath && !canManage) {
          setError('You need system update permission to move a global MCP profile into a team.');
          return;
        }
        if (!canManageTeamProfiles && !canManage) {
          setError('You need team update permission to save team MCP profiles.');
          return;
        }
        setSaving(true);
        setError(null);
        setMessage(null);
        try {
          const result = await upsertTeamMCPProfile(targetTeamPath, localName, { ...payload, name: localName });
          if (movingProfile && editingProfile) {
            if (originalTeamPath) {
              await deleteTeamMCPProfile(originalTeamPath, aiResourceLocalName(editingProfile));
            } else {
              await deleteMCPProfile(editingProfile);
            }
          }
          setTeamProfilesPayload(result);
          const saved = teamMCPProfileRecords(result).find(profile => profile.name === targetName) || null;
          setEditingProfile(targetName);
          setEditingProfileTeamPath(targetTeamPath);
          setProfileForm(prev => ({ ...prev, name: targetName }));
          setPanelMode('profile-edit');
          setMessage(`Saved MCP profile ${saved?.name || targetName}.`);
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to save team MCP profile');
        } finally {
          setSaving(false);
        }
        return;
      }
      if (!canManage) return;
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        const nextProfiles = await saveMCPProfile({ ...payload, name: targetName }, editingProfile);
        if (movingProfile && editingProfile && originalTeamPath) {
          await deleteTeamMCPProfile(originalTeamPath, aiResourceLocalName(editingProfile));
          await loadTeamProfiles(originalTeamPath);
        }
        setProfiles(nextProfiles);
        setEditingProfile(targetName);
        setEditingProfileTeamPath('');
        setProfileForm(prev => ({ ...prev, name: targetName }));
        setPanelMode('profile-edit');
        setMessage(`Saved MCP profile ${targetName}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save MCP profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, canManageTeamProfiles, editingProfile, editingProfileTeamPath, loadTeamProfiles, profileForm]
  );

  const deleteProfile = useCallback(
    async (name: string, opts?: { teamPath?: string }) => {
      const teamPath = normalizeAIResourceTeamPath(opts?.teamPath || '');
      if (teamPath) {
        if (!canManageTeamProfiles && !canManage) return;
        setSaving(true);
        setError(null);
        setMessage(null);
        try {
          await deleteTeamMCPProfile(teamPath, aiResourceLocalName(name));
          if (editingProfile === name) {
            setEditingProfile(null);
            setEditingProfileTeamPath('');
            setProfileForm(emptyMCPProfileForm);
            setPanelMode(prev => (prev === 'profile-edit' ? null : prev));
          }
          await loadTeamProfiles(teamPath);
          setMessage(`Deleted MCP profile ${name}.`);
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Unable to delete team MCP profile');
        } finally {
          setSaving(false);
        }
        return;
      }
      if (!canManage) return;
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        await deleteMCPProfile(name);
        if (editingProfile === name) {
          setEditingProfile(null);
          setEditingProfileTeamPath('');
          setProfileForm(emptyMCPProfileForm);
          setPanelMode(prev => (prev === 'profile-edit' ? null : prev));
        }
        await loadMCP();
        setMessage(`Deleted MCP profile ${name}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to delete MCP profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, canManageTeamProfiles, editingProfile, loadMCP, loadTeamProfiles]
  );

  const testProfile = useCallback(async (name: string) => {
    setTesting(name);
    setError(null);
    setMessage(null);
    try {
      setMessage(`${name}: ${await testMCPProfile(name)}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to test MCP profile');
    } finally {
      setTesting(null);
    }
  }, []);

  return {
    innerTab,
    setInnerTab,
    servers,
    profiles,
    teamProfilesPayload,
    loading,
    teamProfilesLoading,
    saving,
    testing,
    error,
    teamProfilesError,
    message,
    serverForm,
    setServerForm,
    profileForm,
    setProfileForm,
    editingServer,
    editingProfile,
    panelMode,
    setPanelMode,
    loadMCP,
    loadTeamProfiles,
    startServerCreate,
    startServerEdit,
    saveServer,
    deleteServer,
    discoverServer,
    startProfileCreate,
    startProfileEdit,
    toggleProfileTool,
    setProfileServerTools,
    saveProfile,
    deleteProfile,
    testProfile,
  };
}
