import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { Edit3, Plus, RefreshCw, Trash2, X } from 'lucide-react';
import { buildApiUrl } from '../../lib/api';
import { asRecord, normalizeStringArray, normalizeStringMap, readOptionalString, readString } from './data';

function PlusIcon() {
  return <Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />;
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}

function EditIcon() {
  return <Edit3 className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function RefreshIcon() {
  return <RefreshCw className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

type MCPToolRecord = {
  server_name: string;
  name: string;
  description?: string;
  input_schema?: string;
  schema_hash?: string;
  last_seen_at?: string;
};

type MCPServerRecord = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  auth_secret: string;
  headers: Record<string, string>;
  timeout: string;
  allowed_scopes: string[];
  last_test_status?: string;
  last_test_message?: string;
  last_tested_at?: string;
  last_discovered_at?: string;
  discovered_server_name?: string;
  discovered_version?: string;
  discovered_protocol?: string;
  tools: MCPToolRecord[];
};

type MCPProfileServerRef = {
  server: string;
  tools: string[];
};

type MCPProfileRecord = {
  name: string;
  description: string;
  enabled: boolean;
  servers: MCPProfileServerRef[];
  allowed_scopes: string[];
};

type MCPServerFormState = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  auth_secret: string;
  headers_json: string;
  timeout: string;
  allowed_scopes: string;
};

type MCPProfileFormState = {
  name: string;
  description: string;
  enabled: boolean;
  selected_tools: Record<string, string[]>;
  tool_text: Record<string, string>;
  allowed_scopes: string;
};

type MCPPanelMode = 'server-create' | 'server-edit' | 'profile-create' | 'profile-edit';

const emptyMCPServerForm: MCPServerFormState = {
  name: '',
  display_name: '',
  enabled: true,
  provider: '',
  transport: 'streamable_http',
  url: '',
  auth_type: 'none',
  auth_secret: '',
  headers_json: '',
  timeout: '30s',
  allowed_scopes: '',
};

const emptyMCPProfileForm: MCPProfileFormState = {
  name: '',
  description: '',
  enabled: true,
  selected_tools: {},
  tool_text: {},
  allowed_scopes: '',
};

function MCPPanel({ canManage }: { canManage: boolean }) {
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
  const mcpPanelRef = useRef<HTMLElement | null>(null);

  const loadMCP = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [serversResp, profilesResp] = await Promise.all([
        fetch(buildApiUrl('/v1/system/mcp/servers'), { cache: 'no-store' }),
        fetch(buildApiUrl('/v1/system/mcp/profiles'), { cache: 'no-store' }),
      ]);
      if (!serversResp.ok) throw new Error(await serversResp.text() || `Failed to load MCP servers (${serversResp.status})`);
      if (!profilesResp.ok) throw new Error(await profilesResp.text() || `Failed to load MCP profiles (${profilesResp.status})`);
      setServers(normalizeMCPServersPayload(await serversResp.json()));
      setProfiles(normalizeMCPProfilesPayload(await profilesResp.json()));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load MCP registry');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadMCP();
  }, [loadMCP]);

  useEffect(() => {
    if (!panelMode) return;
    window.requestAnimationFrame(() => {
      if (window.innerWidth < 1280) {
        mcpPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
      const focusTarget =
        mcpPanelRef.current?.querySelector<HTMLElement>('[data-mcp-autofocus]:not(:disabled)') ??
        mcpPanelRef.current?.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), textarea:not(:disabled), button:not(:disabled)');
      focusTarget?.focus({ preventScroll: true });
    });
  }, [editingProfile, editingServer, innerTab, panelMode]);

  const startServerCreate = () => {
    setEditingServer(null);
    setServerForm(emptyMCPServerForm);
    setInnerTab('servers');
    setPanelMode('server-create');
  };

  const startServerEdit = (server: MCPServerRecord) => {
    setEditingServer(server.name);
    setServerForm({
      name: server.name,
      display_name: server.display_name || '',
      enabled: server.enabled,
      provider: server.provider || '',
      transport: server.transport || 'streamable_http',
      url: server.url || '',
      auth_type: server.auth_type || 'none',
      auth_secret: server.auth_secret || '',
      headers_json: formatHeadersJSON(server.headers),
      timeout: server.timeout || '30s',
      allowed_scopes: server.allowed_scopes.join(', '),
    });
    setInnerTab('servers');
    setPanelMode('server-edit');
  };

  const saveServer = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    const headers = parseHeadersJSON(serverForm.headers_json);
    if (headers == null) {
      setError('MCP server headers must be a JSON object with string keys and values.');
      return;
    }
    const payload = {
      name: serverForm.name.trim(),
      display_name: serverForm.display_name.trim(),
      enabled: serverForm.enabled,
      provider: serverForm.provider.trim(),
      transport: serverForm.transport.trim(),
      url: serverForm.url.trim(),
      auth_type: serverForm.auth_type.trim(),
      auth_secret: serverForm.auth_secret.trim(),
      headers,
      timeout: serverForm.timeout.trim() || '30s',
      allowed_scopes: splitCSV(serverForm.allowed_scopes),
    };
    if (!payload.name) {
      setError('MCP server name is required.');
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const path = editingServer ? `/v1/system/mcp/servers/${encodeURIComponent(payload.name)}` : '/v1/system/mcp/servers';
      const response = await fetch(buildApiUrl(path), {
        method: editingServer ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!response.ok) throw new Error(await response.text() || `Failed to save MCP server (${response.status})`);
      setServers(normalizeMCPServersPayload(await response.json()));
      setEditingServer(payload.name);
      setPanelMode('server-edit');
      setMessage(`Saved MCP server ${payload.name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save MCP server');
    } finally {
      setSaving(false);
    }
  };

  const deleteServer = async (name: string) => {
    if (!canManage) return;
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/servers/${encodeURIComponent(name)}`), { method: 'DELETE' });
      if (!response.ok && response.status !== 204) throw new Error(await response.text() || `Failed to delete MCP server (${response.status})`);
      if (editingServer === name) {
        setEditingServer(null);
        setServerForm(emptyMCPServerForm);
        setPanelMode(prev => prev === 'server-edit' ? null : prev);
      }
      await loadMCP();
      setMessage(`Deleted MCP server ${name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete MCP server');
    } finally {
      setSaving(false);
    }
  };

  const discoverServer = async (name: string) => {
    setTesting(name);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/servers/${encodeURIComponent(name)}/discover-tools`), { method: 'POST' });
      if (!response.ok) throw new Error(await response.text() || `MCP discovery failed (${response.status})`);
      await loadMCP();
      setMessage(`Discovered tools for ${name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to discover MCP tools');
    } finally {
      setTesting(null);
    }
  };

  const startProfileCreate = () => {
    setEditingProfile(null);
    setProfileForm(emptyMCPProfileForm);
    setInnerTab('profiles');
    setPanelMode('profile-create');
  };

  const startProfileEdit = (profile: MCPProfileRecord) => {
    const selectedTools: Record<string, string[]> = {};
    const toolText: Record<string, string> = {};
    profile.servers.forEach(ref => {
      selectedTools[ref.server] = [...ref.tools];
      toolText[ref.server] = ref.tools.join('\n');
    });
    setEditingProfile(profile.name);
    setProfileForm({
      name: profile.name,
      description: profile.description || '',
      enabled: profile.enabled,
      selected_tools: selectedTools,
      tool_text: toolText,
      allowed_scopes: profile.allowed_scopes.join(', '),
    });
    setInnerTab('profiles');
    setPanelMode('profile-edit');
  };

  const toggleProfileTool = (serverName: string, toolName: string) => {
    setProfileForm(prev => {
      const current = new Set(prev.selected_tools[serverName] || []);
      if (current.has(toolName)) current.delete(toolName);
      else current.add(toolName);
      const next = { ...prev.selected_tools, [serverName]: Array.from(current).sort((a, b) => a.localeCompare(b)) };
      if (next[serverName].length === 0) delete next[serverName];
      const toolText = { ...prev.tool_text, [serverName]: (next[serverName] || []).join('\n') };
      if (!next[serverName]) delete toolText[serverName];
      return { ...prev, selected_tools: next, tool_text: toolText };
    });
  };

  const setProfileServerTools = (serverName: string, value: string) => {
    setProfileForm(prev => {
      const tools = splitToolNames(value);
      const next = { ...prev.selected_tools };
      if (tools.length > 0) next[serverName] = tools;
      else delete next[serverName];
      return { ...prev, selected_tools: next, tool_text: { ...prev.tool_text, [serverName]: value } };
    });
  };

  const saveProfile = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    const refs = Object.entries(profileForm.selected_tools)
      .filter(([, tools]) => tools.length > 0)
      .map(([server, tools]) => ({ server, tools }));
    const payload = {
      name: profileForm.name.trim(),
      description: profileForm.description.trim(),
      enabled: profileForm.enabled,
      servers: refs,
      allowed_scopes: splitCSV(profileForm.allowed_scopes),
    };
    if (!payload.name) {
      setError('MCP profile name is required.');
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const path = editingProfile ? `/v1/system/mcp/profiles/${encodeURIComponent(payload.name)}` : '/v1/system/mcp/profiles';
      const response = await fetch(buildApiUrl(path), {
        method: editingProfile ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!response.ok) throw new Error(await response.text() || `Failed to save MCP profile (${response.status})`);
      setProfiles(normalizeMCPProfilesPayload(await response.json()));
      setEditingProfile(payload.name);
      setPanelMode('profile-edit');
      setMessage(`Saved MCP profile ${payload.name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save MCP profile');
    } finally {
      setSaving(false);
    }
  };

  const deleteProfile = async (name: string) => {
    if (!canManage) return;
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/profiles/${encodeURIComponent(name)}`), { method: 'DELETE' });
      if (!response.ok && response.status !== 204) throw new Error(await response.text() || `Failed to delete MCP profile (${response.status})`);
      if (editingProfile === name) {
        setEditingProfile(null);
        setProfileForm(emptyMCPProfileForm);
        setPanelMode(prev => prev === 'profile-edit' ? null : prev);
      }
      await loadMCP();
      setMessage(`Deleted MCP profile ${name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete MCP profile');
    } finally {
      setSaving(false);
    }
  };

  const testProfile = async (name: string) => {
    setTesting(name);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/profiles/${encodeURIComponent(name)}/test`), { method: 'POST' });
      if (!response.ok) throw new Error(await response.text() || `MCP profile test failed (${response.status})`);
      const result = await response.json();
      const warnings = normalizeStringArray(asRecord(result)?.warnings);
      setMessage(warnings.length ? `${name}: ${warnings.join('; ')}` : `${name}: ${readString(asRecord(result)?.message) || 'ok'}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to test MCP profile');
    } finally {
      setTesting(null);
    }
  };

  const showServerPanel = panelMode === 'server-create' || panelMode === 'server-edit';
  const showProfilePanel = panelMode === 'profile-create' || panelMode === 'profile-edit';

  return (
    <div id="system-mcp-section" className="space-y-6 pb-24">
      <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex rounded-lg border border-[var(--border-primary)] overflow-hidden w-fit">
            <button type="button" className={`px-4 py-2 text-sm ${innerTab === 'servers' ? 'bg-[var(--surface-elevated)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'}`} onClick={() => setInnerTab('servers')}>
              MCP Servers
            </button>
            <button type="button" className={`px-4 py-2 text-sm border-l border-[var(--border-primary)] ${innerTab === 'profiles' ? 'bg-[var(--surface-elevated)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'}`} onClick={() => setInnerTab('profiles')}>
              MCP Profiles
            </button>
          </div>
          <div className="flex items-center gap-2">
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="glass-button-ghost" onClick={() => void loadMCP()} disabled={loading || saving}>
              <RefreshIcon />
              Reload
            </button>
            {canManage && innerTab === 'servers' && (
              <button type="button" className="glass-button-primary" onClick={startServerCreate} disabled={saving}>
                <PlusIcon />
                New server
              </button>
            )}
            {canManage && innerTab === 'profiles' && (
              <button type="button" className="glass-button-primary" onClick={startProfileCreate} disabled={saving}>
                <PlusIcon />
                New profile
              </button>
            )}
          </div>
        </div>
        {error && <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500 whitespace-pre-wrap">{error}</div>}
        {message && <div className="rounded-lg border border-[var(--border-primary)] px-4 py-3 text-sm text-[var(--text-secondary)]">{message}</div>}
      </section>

      {innerTab === 'servers' ? (
        <div className={`grid gap-6 ${showServerPanel ? 'xl:grid-cols-[minmax(0,1.3fr)_minmax(360px,0.7fr)]' : ''}`}>
          <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
            <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Servers</h3>
              {loading && <span className="text-sm text-[var(--text-secondary)]">Loading…</span>}
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
                  <tr>
                    <th className="px-4 py-3">Name</th>
                    <th className="px-4 py-3">Provider</th>
                    <th className="px-4 py-3">URL</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3">Tools</th>
                    <th className="px-4 py-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {servers.map(server => (
                    <tr key={server.name} className="border-b border-[var(--border-primary)] last:border-b-0">
                      <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">
                        {server.display_name || server.name}
                        <div className="text-xs text-[var(--text-secondary)]">{server.name}</div>
                      </td>
                      <td className="px-4 py-3">{server.provider || '-'}</td>
                      <td className="px-4 py-3 max-w-[260px] truncate" title={server.url}>{server.url || '-'}</td>
                      <td className="px-4 py-3">
                        <span className={`runner-pill ${server.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>{server.enabled ? 'Enabled' : 'Disabled'}</span>
                        {server.last_test_status && <div className="text-xs text-[var(--text-secondary)] mt-1">{server.last_test_status}</div>}
                      </td>
                      <td className="px-4 py-3">{server.tools.length}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <button type="button" className="glass-button-ghost" onClick={() => void discoverServer(server.name)} disabled={Boolean(testing)}>
                            {testing === server.name ? 'Testing…' : 'Discover'}
                          </button>
                          <button type="button" className="glass-button-subtle" onClick={() => startServerEdit(server)}>
                            <EditIcon />
                            Edit
                          </button>
                          <button type="button" className="glass-button-danger" onClick={() => void deleteServer(server.name)} disabled={!canManage || saving}>
                            <TrashIcon />
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!loading && servers.length === 0 && (
                    <tr><td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={6}>No MCP servers configured.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>

          {showServerPanel && (
            <aside ref={mcpPanelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs text-[var(--text-secondary)]">{panelMode === 'server-edit' ? 'Edit server' : 'Create server'}</p>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">{panelMode === 'server-edit' ? editingServer : 'New MCP server'}</h3>
                </div>
                <button type="button" className="glass-button-ghost !px-2" aria-label="Close server form" onClick={() => setPanelMode(null)}>
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              <form className="space-y-4" onSubmit={saveServer}>
                <label className="flex flex-col gap-1 text-sm"><span>Name</span><input data-mcp-autofocus className="pipelines-input" value={serverForm.name} onChange={event => setServerForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingServer)} placeholder="github" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Display name</span><input className="pipelines-input" value={serverForm.display_name} onChange={event => setServerForm(prev => ({ ...prev, display_name: event.target.value }))} disabled={!canManage} placeholder="GitHub MCP" /></label>
                <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={serverForm.enabled} onChange={event => setServerForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} /> Enabled</label>
                <label className="flex flex-col gap-1 text-sm"><span>Provider</span><input className="pipelines-input" value={serverForm.provider} onChange={event => setServerForm(prev => ({ ...prev, provider: event.target.value }))} disabled={!canManage} placeholder="github" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Transport</span><select className="pipelines-input" value={serverForm.transport} onChange={event => setServerForm(prev => ({ ...prev, transport: event.target.value }))} disabled={!canManage}><option value="streamable_http">streamable_http</option><option value="http">http</option></select></label>
                <label className="flex flex-col gap-1 text-sm"><span>URL</span><input className="pipelines-input" value={serverForm.url} onChange={event => setServerForm(prev => ({ ...prev, url: event.target.value }))} disabled={!canManage} placeholder="https://api.githubcopilot.com/mcp/x/all/readonly" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Auth type</span><select className="pipelines-input" value={serverForm.auth_type} onChange={event => setServerForm(prev => ({ ...prev, auth_type: event.target.value }))} disabled={!canManage}><option value="none">none</option><option value="bearer_token">bearer_token</option></select></label>
                <label className="flex flex-col gap-1 text-sm"><span>Secret reference</span><input className="pipelines-input" value={serverForm.auth_secret} onChange={event => setServerForm(prev => ({ ...prev, auth_secret: event.target.value }))} disabled={!canManage} placeholder="GITHUB_MCP_TOKEN" /></label>
                <div className="rounded-lg border border-[var(--border-primary)] p-3 space-y-3">
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Extra configuration</p>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Headers JSON</span>
                    <textarea
                      className="pipelines-input min-h-[112px] font-mono text-xs"
                      value={serverForm.headers_json}
                      onChange={event => setServerForm(prev => ({ ...prev, headers_json: event.target.value }))}
                      disabled={!canManage}
                      placeholder={'{"X-MCP-Toolsets":"repos,issues","X-MCP-Readonly":"true"}'}
                      spellCheck={false}
                    />
                  </label>
                </div>
                <label className="flex flex-col gap-1 text-sm"><span>Timeout</span><input className="pipelines-input" value={serverForm.timeout} onChange={event => setServerForm(prev => ({ ...prev, timeout: event.target.value }))} disabled={!canManage} placeholder="30s" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Allowed scopes</span><input className="pipelines-input" value={serverForm.allowed_scopes} onChange={event => setServerForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, prod" /></label>
                <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>{saving ? 'Saving…' : 'Save server'}</button>
              </form>
            </aside>
          )}
        </div>
      ) : (
        <div className={`grid gap-6 ${showProfilePanel ? 'xl:grid-cols-[minmax(0,1.2fr)_minmax(420px,0.8fr)]' : ''}`}>
          <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
            <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Profiles</h3>
              {loading && <span className="text-sm text-[var(--text-secondary)]">Loading…</span>}
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
                  <tr><th className="px-4 py-3">Name</th><th className="px-4 py-3">Servers</th><th className="px-4 py-3">Tools</th><th className="px-4 py-3">Status</th><th className="px-4 py-3 text-right">Actions</th></tr>
                </thead>
                <tbody>
                  {profiles.map(profile => (
                    <tr key={profile.name} className="border-b border-[var(--border-primary)] last:border-b-0">
                      <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">{profile.name}<div className="text-xs text-[var(--text-secondary)]">{profile.description || '-'}</div></td>
                      <td className="px-4 py-3">{profile.servers.map(ref => ref.server).join(', ') || '-'}</td>
                      <td className="px-4 py-3">{profile.servers.reduce((total, ref) => total + ref.tools.length, 0)}</td>
                      <td className="px-4 py-3"><span className={`runner-pill ${profile.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>{profile.enabled ? 'Enabled' : 'Disabled'}</span></td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <button type="button" className="glass-button-ghost" onClick={() => void testProfile(profile.name)} disabled={Boolean(testing)}>{testing === profile.name ? 'Testing…' : 'Test'}</button>
                          <button type="button" className="glass-button-subtle" onClick={() => startProfileEdit(profile)}><EditIcon />Edit</button>
                          <button type="button" className="glass-button-danger" onClick={() => void deleteProfile(profile.name)} disabled={!canManage || saving}><TrashIcon />Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!loading && profiles.length === 0 && (
                    <tr><td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={5}>No MCP profiles configured.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>

          {showProfilePanel && (
            <aside ref={mcpPanelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs text-[var(--text-secondary)]">{panelMode === 'profile-edit' ? 'Edit profile' : 'Create profile'}</p>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">{panelMode === 'profile-edit' ? editingProfile : 'New MCP profile'}</h3>
                </div>
                <button type="button" className="glass-button-ghost !px-2" aria-label="Close profile form" onClick={() => setPanelMode(null)}>
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              <form className="space-y-4" onSubmit={saveProfile}>
                <label className="flex flex-col gap-1 text-sm"><span>Name</span><input data-mcp-autofocus className="pipelines-input" value={profileForm.name} onChange={event => setProfileForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingProfile)} placeholder="github-pr-review" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Description</span><input className="pipelines-input" value={profileForm.description} onChange={event => setProfileForm(prev => ({ ...prev, description: event.target.value }))} disabled={!canManage} placeholder="Read-only GitHub PR review tools" /></label>
                <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={profileForm.enabled} onChange={event => setProfileForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} /> Enabled</label>
                <div className="space-y-3">
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Tools</p>
                  {servers.map(server => (
                    <div key={server.name} className="rounded-lg border border-[var(--border-primary)] p-3 space-y-2">
                      <div className="font-semibold text-sm text-[var(--text-primary)]">{server.display_name || server.name}</div>
                      <label className="flex flex-col gap-1 text-sm">
                        <span>Selected tools</span>
                        <textarea
                          className="pipelines-input min-h-[84px] font-mono text-xs"
                          value={profileForm.tool_text[server.name] ?? (profileForm.selected_tools[server.name] || []).join('\n')}
                          onChange={event => setProfileServerTools(server.name, event.target.value)}
                          disabled={!canManage}
                          placeholder={'*\nissues_list\nrepos_get'}
                          spellCheck={false}
                        />
                      </label>
                      {server.tools.length === 0 ? (
                        <p className="text-xs text-[var(--text-secondary)]">No discovered tools cached for this server.</p>
                      ) : (
                        <div className="grid gap-2">
                          {server.tools.map(tool => (
                            <label key={`${server.name}-${tool.name}`} className="flex items-start gap-2 text-sm">
                              <input type="checkbox" checked={(profileForm.selected_tools[server.name] || []).includes(tool.name)} onChange={() => toggleProfileTool(server.name, tool.name)} disabled={!canManage} />
                              <span><span className="font-mono">{tool.name}</span>{tool.description ? <span className="block text-xs text-[var(--text-secondary)]">{tool.description}</span> : null}</span>
                            </label>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
                <label className="flex flex-col gap-1 text-sm"><span>Allowed scopes</span><input className="pipelines-input" value={profileForm.allowed_scopes} onChange={event => setProfileForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, prod" /></label>
                <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>{saving ? 'Saving…' : 'Save profile'}</button>
              </form>
            </aside>
          )}
        </div>
      )}
    </div>
  );
}


function normalizeMCPServersPayload(value: unknown): MCPServerRecord[] {
  const record = asRecord(value);
  const serversRaw = record && Array.isArray(record.servers) ? record.servers : [];
  const servers = serversRaw
    .map(item => {
      const server = asRecord(item);
      if (!server) return null;
      const name = readString(server.name).trim();
      if (!name) return null;
      return {
        name,
        display_name: readString(server.display_name).trim(),
        enabled: typeof server.enabled === 'boolean' ? server.enabled : true,
        provider: readString(server.provider).trim(),
        transport: readString(server.transport).trim() || 'streamable_http',
        url: readString(server.url).trim(),
        auth_type: readString(server.auth_type).trim() || 'none',
        auth_secret: readString(server.auth_secret).trim(),
        headers: normalizeStringMap(server.headers),
        timeout: readString(server.timeout).trim() || '30s',
        allowed_scopes: normalizeStringArray(server.allowed_scopes),
        last_test_status: readOptionalString(server.last_test_status),
        last_test_message: readOptionalString(server.last_test_message),
        last_tested_at: readOptionalString(server.last_tested_at),
        last_discovered_at: readOptionalString(server.last_discovered_at),
        discovered_server_name: readOptionalString(server.discovered_server_name),
        discovered_version: readOptionalString(server.discovered_version),
        discovered_protocol: readOptionalString(server.discovered_protocol),
        tools: Array.isArray(server.tools) ? server.tools.map(normalizeMCPTool).filter((tool): tool is MCPToolRecord => Boolean(tool)) : [],
      } satisfies MCPServerRecord;
    })
    .filter(Boolean) as MCPServerRecord[];
  return servers.sort((a, b) => a.name.localeCompare(b.name));
}

function normalizeMCPTool(value: unknown): MCPToolRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const name = readString(record.name).trim();
  if (!name) return null;
  return {
    server_name: readString(record.server_name).trim(),
    name,
    description: readOptionalString(record.description),
    input_schema: readOptionalString(record.input_schema),
    schema_hash: readOptionalString(record.schema_hash),
    last_seen_at: readOptionalString(record.last_seen_at),
  };
}

function normalizeMCPProfilesPayload(value: unknown): MCPProfileRecord[] {
  const record = asRecord(value);
  const profilesRaw = record && Array.isArray(record.profiles) ? record.profiles : [];
  const profiles = profilesRaw
    .map(item => {
      const profile = asRecord(item);
      if (!profile) return null;
      const name = readString(profile.name).trim();
      if (!name) return null;
      const refsRaw = Array.isArray(profile.servers) ? profile.servers : [];
      const refs = refsRaw
        .map(refItem => {
          const ref = asRecord(refItem);
          if (!ref) return null;
          const server = readString(ref.server).trim();
          if (!server) return null;
          return { server, tools: normalizeStringArray(ref.tools) } satisfies MCPProfileServerRef;
        })
        .filter(Boolean) as MCPProfileServerRef[];
      return {
        name,
        description: readString(profile.description).trim(),
        enabled: typeof profile.enabled === 'boolean' ? profile.enabled : true,
        servers: refs,
        allowed_scopes: normalizeStringArray(profile.allowed_scopes),
      } satisfies MCPProfileRecord;
    })
    .filter(Boolean) as MCPProfileRecord[];
  return profiles.sort((a, b) => a.name.localeCompare(b.name));
}

function splitCSV(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

function splitToolNames(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/[\n,]/)
    .map(item => item.trim())
    .filter(item => {
      if (!item || seen.has(item)) return false;
      seen.add(item);
      return true;
    })
    .sort((a, b) => a.localeCompare(b));
}

function formatHeadersJSON(headers: Record<string, string>): string {
  if (Object.keys(headers || {}).length === 0) return '';
  return JSON.stringify(headers, null, 2);
}

function parseHeadersJSON(value: string): Record<string, string> | null {
  const trimmed = value.trim();
  if (!trimmed) return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return null;
  }
  const record = asRecord(parsed);
  if (!record || Array.isArray(parsed)) return null;
  const headers: Record<string, string> = {};
  for (const [key, headerValue] of Object.entries(record)) {
    const headerName = key.trim();
    if (!headerName) continue;
    if (typeof headerValue !== 'string') return null;
    headers[headerName] = headerValue.trim();
  }
  return headers;
}


export default MCPPanel;
