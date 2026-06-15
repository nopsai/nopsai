import { useEffect, useRef } from 'react';
import { Edit3, Plus, RefreshCw, Trash2, X } from 'lucide-react';
import { useMCPRegistry } from './mcp/useMCPRegistry';

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

function MCPPanel({ canManage }: { canManage: boolean }) {
  const mcpPanelRef = useRef<HTMLElement | null>(null);
  const {
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
  } = useMCPRegistry({ canManage });

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
                <label className="flex flex-col gap-1 text-sm"><span>Credential reference</span><input className="pipelines-input" value={serverForm.credential_ref} onChange={event => setServerForm(prev => ({ ...prev, credential_ref: event.target.value }))} disabled={!canManage} placeholder="credential://system/mcp/github-readonly" /></label>
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


export default MCPPanel;
