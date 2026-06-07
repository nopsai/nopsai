import { useCallback, useEffect, useState, type FormEvent } from 'react';
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

export function useMCPRegistry({ canManage }: { canManage: boolean }) {
  const [innerTab, setInnerTab] = useState<'servers' | 'profiles'>('servers');
  const [servers, setServers] = useState<MCPServerRecord[]>([]);
  const [profiles, setProfiles] = useState<MCPProfileRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [serverForm, setServerForm] = useState<MCPServerFormState>(emptyMCPServerForm);
  const [profileForm, setProfileForm] = useState<MCPProfileFormState>(emptyMCPProfileForm);
  const [editingServer, setEditingServer] = useState<string | null>(null);
  const [editingProfile, setEditingProfile] = useState<string | null>(null);
  const [panelMode, setPanelMode] = useState<MCPPanelMode | null>(null);

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
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        setServers(await saveMCPServer(payload, editingServer));
        setEditingServer(payload.name);
        setPanelMode('server-edit');
        setMessage(`Saved MCP server ${payload.name}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save MCP server');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingServer, serverForm]
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
    setProfileForm(emptyMCPProfileForm);
    setInnerTab('profiles');
    setPanelMode('profile-create');
  }, []);

  const startProfileEdit = useCallback((profile: MCPProfileRecord) => {
    setEditingProfile(profile.name);
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
      if (!canManage) return;
      const payload = mcpProfilePayloadFromForm(profileForm);
      if (!payload.name) {
        setError('MCP profile name is required.');
        return;
      }
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        setProfiles(await saveMCPProfile(payload, editingProfile));
        setEditingProfile(payload.name);
        setPanelMode('profile-edit');
        setMessage(`Saved MCP profile ${payload.name}.`);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to save MCP profile');
      } finally {
        setSaving(false);
      }
    },
    [canManage, editingProfile, profileForm]
  );

  const deleteProfile = useCallback(
    async (name: string) => {
      if (!canManage) return;
      setSaving(true);
      setError(null);
      setMessage(null);
      try {
        await deleteMCPProfile(name);
        if (editingProfile === name) {
          setEditingProfile(null);
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
    [canManage, editingProfile, loadMCP]
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
    loading,
    saving,
    testing,
    error,
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
