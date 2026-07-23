import { useEffect, useMemo, useRef, useState, type Dispatch, type FormEvent, type SetStateAction } from 'react';
import { Boxes, Cable, CheckCircle2, Edit3, ExternalLink, KeyRound, Plus, RefreshCw, Trash2, Wrench, X } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { useMCPRegistry } from './mcp/useMCPRegistry';
import { CredentialReferenceLink } from './credentials/CredentialReferenceLink';
import {
  AIResourceEmptyState,
  AIResourceIconAction,
  AIResourceTeamBadge,
  AIResourceTeamFilter,
  AIResourceTeamPlacementField,
  AIResourceTableHeader,
} from './AIResourcePanel';
import { AIResourceMetricGrid, AIResourceWorkspace, type AIResourceWorkspaceItem } from './AIResourceWorkspace';
import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  aiResourceLocalName,
  aiResourceMatchesTeamFilter,
  aiResourceTeamFilterFromSearch,
  buildAIResourceScopedID,
  collectAIResourceTeamPaths,
  formatAIResourceTeamLabel,
  selectableAIResourceTeamPath,
} from './aiResourceTeams';
import {
  formatAIResourceRatio,
  formatFilteredCount,
  matchesAIResourceSearch,
} from './aiResourcePresentation';
import { aiResourceTreeFilterForResource } from './aiResourceTree';
import { useAIResourceTeamPaths } from './useAIResourceTeamPaths';
import { MCPDetailSection } from './MCPDetailSection';
import { MCPProfileTable, MCPServerTable } from './MCPResourceTables';
import { MCPViewSwitch } from './MCPViewSwitch';
import {
  countMCPProfileTools,
  formatMCPScopes,
  type MCPProfileFormState,
  type MCPProfileRecord,
  type MCPServerFormState,
  type MCPServerRecord,
} from './mcp/model';
import ResourceAccessCard from '../../components/ResourceAccessCard';

function mcpViewFromSearch(search: string): 'servers' | 'profiles' | null {
  const params = new URLSearchParams(search);
  const view = (params.get('view') || params.get('tab') || '').trim().toLowerCase();
  if (view === 'profile' || view === 'profiles') return 'profiles';
  if (view === 'server' || view === 'servers') return 'servers';
  return null;
}

function MCPPanel({ canManage }: { canManage: boolean }) {
  const mcpPanelRef = useRef<HTMLElement | null>(null);
  const location = useLocation();
  const requestedTeamFilter = useMemo(() => aiResourceTeamFilterFromSearch(location.search), [location.search]);
  const requestedView = useMemo(() => mcpViewFromSearch(location.search), [location.search]);
  const [serverSearchTerm, setServerSearchTerm] = useState('');
  const [profileSearchTerm, setProfileSearchTerm] = useState('');
  const [teamFilter, setTeamFilter] = useState(requestedTeamFilter);
  const [createServerTeamPath, setCreateServerTeamPath] = useState('');
  const [createProfileTeamPath, setCreateProfileTeamPath] = useState('');
  const [selectedServerName, setSelectedServerName] = useState<string | null>(null);
  const [selectedProfileName, setSelectedProfileName] = useState<string | null>(null);
  const { teamPaths, teamPathsLoading } = useAIResourceTeamPaths();
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
    setTeamFilter(requestedTeamFilter);
  }, [requestedTeamFilter]);

  useEffect(() => {
    if (!requestedView) return;
    setInnerTab(requestedView);
    setPanelMode(null);
  }, [requestedView, setInnerTab, setPanelMode]);

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

  const hasConnectionStatus = servers.some(server => Boolean(server.last_test_status));
  const visibleServers = useMemo(
    () => servers.filter(server => matchesAIResourceSearch(
      serverSearchTerm,
      server.name,
      aiResourceLocalName(server.name),
      formatAIResourceTeamLabel(server.name),
      server.display_name,
      server.provider,
      server.transport,
      server.url,
      server.auth_type,
      server.credential_ref,
      server.allowed_scopes.join(', '),
      server.enabled ? 'enabled' : 'disabled',
      server.last_test_status,
      server.last_test_message,
      server.tools.length
    ) && aiResourceMatchesTeamFilter(server.name, teamFilter)),
    [serverSearchTerm, servers, teamFilter]
  );
  const visibleProfiles = useMemo(
    () => profiles.filter(profile => matchesAIResourceSearch(
      profileSearchTerm,
      profile.name,
      aiResourceLocalName(profile.name),
      formatAIResourceTeamLabel(profile.name),
      profile.description,
      profile.enabled ? 'enabled' : 'disabled',
      profile.allowed_scopes.join(', '),
      profile.servers.map(ref => ref.server).join(', '),
      profile.servers.flatMap(ref => ref.tools).join(', '),
      profile.servers.reduce((total, ref) => total + ref.tools.length, 0)
    ) && aiResourceMatchesTeamFilter(profile.name, teamFilter)),
    [profileSearchTerm, profiles, teamFilter]
  );
  const selectedServer = useMemo(
    () => selectedServerName ? visibleServers.find(server => server.name === selectedServerName) ?? null : null,
    [selectedServerName, visibleServers]
  );
  const selectedProfile = useMemo(
    () => selectedProfileName ? visibleProfiles.find(profile => profile.name === selectedProfileName) ?? null : null,
    [selectedProfileName, visibleProfiles]
  );
  const showServerForm = panelMode === 'server-create' || panelMode === 'server-edit';
  const showProfileForm = panelMode === 'profile-create' || panelMode === 'profile-edit';
  const teamFilterOptions = useMemo(
    () => collectAIResourceTeamPaths(
      innerTab === 'servers' ? servers.map(server => server.name) : profiles.map(profile => profile.name),
      teamPaths
    ),
    [innerTab, profiles, servers, teamPaths]
  );
  const filteredCountToken = (innerTab === 'servers' ? serverSearchTerm : profileSearchTerm) ||
    (teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL ? teamFilter : '');
  const visibleServerTools = visibleServers.reduce((total, server) => total + server.tools.length, 0);
  const visibleConnectedServers = visibleServers.filter(isServerConnected).length;
  const visibleEnabledServers = visibleServers.filter(server => server.enabled).length;
  const visibleCredentialServers = visibleServers.filter(server => Boolean(server.credential_ref)).length;
  const visibleProfileTools = visibleProfiles.reduce((total, profile) => total + countMCPProfileTools(profile), 0);
  const visibleEnabledProfiles = visibleProfiles.filter(profile => profile.enabled).length;
  const visibleProfileServerRefs = visibleProfiles.reduce((total, profile) => total + profile.servers.length, 0);
  const serverWorkspaceResources = useMemo<AIResourceWorkspaceItem[]>(
    () => servers.map(server => ({
      id: server.name,
      label: server.display_name || aiResourceLocalName(server.name) || server.name,
      description: server.provider || server.transport || 'MCP server',
    })),
    [servers]
  );
  const profileWorkspaceResources = useMemo<AIResourceWorkspaceItem[]>(
    () => profiles.map(profile => ({
      id: profile.name,
      label: aiResourceLocalName(profile.name) || profile.name,
      description: profile.description || `${profile.servers.length} servers`,
    })),
    [profiles]
  );
  const selectedWorkspaceServerName = showServerForm ? null : selectedServer?.name ?? null;
  const selectedWorkspaceProfileName = showProfileForm ? null : selectedProfile?.name ?? null;
  const serverDetailOpen = showServerForm || Boolean(selectedWorkspaceServerName);
  const profileDetailOpen = showProfileForm || Boolean(selectedWorkspaceProfileName);
  const openMCPView = (view: 'servers' | 'profiles') => {
    setInnerTab(view);
    setSelectedServerName(null);
    setSelectedProfileName(null);
    setPanelMode(null);
  };
  const openTeamFilter = (value: string) => {
    setTeamFilter(value);
    setSelectedServerName(null);
    setSelectedProfileName(null);
    setPanelMode(null);
  };
  const selectServer = (name: string) => {
    setSelectedServerName(name);
    setTeamFilter(aiResourceTreeFilterForResource(name));
    setPanelMode(null);
  };
  const selectProfile = (name: string) => {
    setSelectedProfileName(name);
    setTeamFilter(aiResourceTreeFilterForResource(name));
    setPanelMode(null);
  };
  const closeDetail = () => {
    setSelectedServerName(null);
    setSelectedProfileName(null);
    setPanelMode(null);
  };

  const openServerCreate = () => {
    const initialTeamPath = teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL && teamFilter !== AI_RESOURCE_TEAM_FILTER_GLOBAL
      ? selectableAIResourceTeamPath(teamFilter, teamPaths)
      : '';
    setSelectedServerName(null);
    setCreateServerTeamPath(initialTeamPath);
    startServerCreate();
    setServerForm(prev => ({ ...prev, name: buildAIResourceScopedID(initialTeamPath, aiResourceLocalName(prev.name)) }));
  };
  const openProfileCreate = () => {
    const initialTeamPath = teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL && teamFilter !== AI_RESOURCE_TEAM_FILTER_GLOBAL
      ? selectableAIResourceTeamPath(teamFilter, teamPaths)
      : '';
    setSelectedProfileName(null);
    setCreateProfileTeamPath(initialTeamPath);
    startProfileCreate();
    setProfileForm(prev => ({ ...prev, name: buildAIResourceScopedID(initialTeamPath, aiResourceLocalName(prev.name)) }));
  };
  const setCreateServerTeam = (teamPath: string) => {
    setCreateServerTeamPath(teamPath);
    setServerForm(prev => ({ ...prev, name: buildAIResourceScopedID(teamPath, aiResourceLocalName(prev.name)) }));
  };
  const setCreateServerName = (localName: string) => {
    setServerForm(prev => ({ ...prev, name: buildAIResourceScopedID(createServerTeamPath, localName) }));
  };
  const setCreateProfileTeam = (teamPath: string) => {
    setCreateProfileTeamPath(teamPath);
    setProfileForm(prev => ({ ...prev, name: buildAIResourceScopedID(teamPath, aiResourceLocalName(prev.name)) }));
  };
  const setCreateProfileName = (localName: string) => {
    setProfileForm(prev => ({ ...prev, name: buildAIResourceScopedID(createProfileTeamPath, localName) }));
  };

  return (
    <div id="system-mcp-section" className="ai-resource-panel ai-resource-page space-y-5 pb-24">
      <div className="ai-resource-page-header ai-resource-page-header--toolbar">
        <div>
          <h2 className="sr-only">MCP</h2>
          <MCPViewSwitch activeView={innerTab} onChange={openMCPView} />
        </div>
        <div className="ai-resource-page-actions">
          {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
          <button type="button" className="ai-resource-icon-button" onClick={() => void loadMCP()} disabled={loading || saving} aria-label="Reload">
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
          </button>
          {canManage && innerTab === 'servers' && (
            <button type="button" className="ai-resource-primary-button" onClick={openServerCreate} disabled={saving}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New server
            </button>
          )}
          {canManage && innerTab === 'profiles' && (
            <button type="button" className="ai-resource-primary-button" onClick={openProfileCreate} disabled={saving}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New profile
            </button>
          )}
        </div>
      </div>

      {error && <div className="ai-resource-alert ai-resource-alert--error whitespace-pre-wrap">{error}</div>}
      {message && <div className="ai-resource-alert">{message}</div>}

      {innerTab === 'servers' ? (
        <>
          {!serverDetailOpen && (
            <AIResourceMetricGrid
              metrics={[
                { label: 'Servers', value: visibleServers.length, icon: <Cable className="h-4 w-4" /> },
                { label: hasConnectionStatus ? 'Connected' : 'Enabled', value: formatAIResourceRatio(hasConnectionStatus ? visibleConnectedServers : visibleEnabledServers, visibleServers.length), icon: <CheckCircle2 className="h-4 w-4" />, tone: visibleServers.length === 0 || (hasConnectionStatus ? visibleConnectedServers : visibleEnabledServers) === visibleServers.length ? 'ok' : 'warning' },
                { label: 'Discovered tools', value: visibleServerTools, icon: <Wrench className="h-4 w-4" />, tone: 'info' },
                { label: 'Credential-backed', value: visibleCredentialServers, icon: <KeyRound className="h-4 w-4" />, tone: visibleCredentialServers > 0 ? 'muted' : 'warning' },
              ]}
            />
          )}
          <AIResourceWorkspace
            storageKey="mcp-servers"
            workspaceLabel="MCP server workspace"
            treeTitle="MCP server tree"
            resourceType="mcp-server"
            resourceLabel="MCP server"
            resources={serverWorkspaceResources}
            teamPaths={teamFilterOptions}
            teamFilter={teamFilter}
            selectedResourceID={selectedWorkspaceServerName}
            onTeamFilterChange={openTeamFilter}
            onResourceSelect={selectServer}
            onDetailClose={closeDetail}
            detailOpen={serverDetailOpen}
            detailRef={mcpPanelRef}
            detailLabel="MCP server detail"
            listHeader={(
              <AIResourceTableHeader
                title="Servers"
                count={formatFilteredCount(visibleServers.length, servers.length, filteredCountToken)}
                loading={loading}
                searchLabel="Search MCP servers"
                searchPlaceholder="Search servers..."
                searchValue={serverSearchTerm}
                onSearchChange={setServerSearchTerm}
                filters={(
                  <AIResourceTeamFilter
                    value={teamFilter}
                    onChange={openTeamFilter}
                    teamPaths={teamFilterOptions}
                    disabled={teamPathsLoading && teamFilterOptions.length === 0}
                  />
                )}
              />
            )}
            list={(
              <MCPServerTable
                servers={visibleServers}
                selectedServerName={selectedWorkspaceServerName}
                loading={loading}
                emptyMessage={servers.length === 0 ? 'No MCP servers configured.' : 'No MCP servers match your filters.'}
                onSelectServer={selectServer}
              />
            )}
            detail={(
              <>
                {showServerForm ? (
                  <MCPServerForm
                    canManage={canManage}
                    saving={saving}
                    editingServer={editingServer}
                    serverForm={serverForm}
                    setServerForm={setServerForm}
                    createTeamPath={createServerTeamPath}
                    teamPaths={teamPaths}
                    teamPathsLoading={teamPathsLoading}
                    onCreateTeamPathChange={setCreateServerTeam}
                    onCreateLocalNameChange={setCreateServerName}
                    onSubmit={saveServer}
                    onClose={() => setPanelMode(null)}
                  />
                ) : selectedServer ? (
                  <MCPServerDetail
                    server={selectedServer}
                    canManage={canManage}
                    saving={saving}
                    testing={testing}
                    onDiscover={discoverServer}
                    onEdit={startServerEdit}
                    onDelete={deleteServer}
                  />
                ) : (
                  <AIResourceEmptyState>Select an MCP server to inspect.</AIResourceEmptyState>
                )}
              </>
            )}
          />
        </>
      ) : (
        <>
          {!profileDetailOpen && (
            <AIResourceMetricGrid
              metrics={[
                { label: 'Profiles', value: visibleProfiles.length, icon: <Boxes className="h-4 w-4" /> },
                { label: 'Enabled', value: formatAIResourceRatio(visibleEnabledProfiles, visibleProfiles.length), icon: <CheckCircle2 className="h-4 w-4" />, tone: visibleProfiles.length === 0 || visibleEnabledProfiles === visibleProfiles.length ? 'ok' : 'warning' },
                { label: 'Approved tools', value: visibleProfileTools, icon: <Wrench className="h-4 w-4" />, tone: 'info' },
                { label: 'Server refs', value: visibleProfileServerRefs, icon: <Cable className="h-4 w-4" />, tone: 'muted' },
              ]}
            />
          )}
          <AIResourceWorkspace
            storageKey="mcp-profiles"
            workspaceLabel="MCP profile workspace"
            treeTitle="MCP profile tree"
            resourceType="mcp-profile"
            resourceLabel="MCP profile"
            resources={profileWorkspaceResources}
            teamPaths={teamFilterOptions}
            teamFilter={teamFilter}
            selectedResourceID={selectedWorkspaceProfileName}
            onTeamFilterChange={openTeamFilter}
            onResourceSelect={selectProfile}
            onDetailClose={closeDetail}
            detailOpen={profileDetailOpen}
            detailRef={mcpPanelRef}
            detailLabel="MCP profile detail"
            listHeader={(
              <AIResourceTableHeader
                title="Profiles"
                count={formatFilteredCount(visibleProfiles.length, profiles.length, filteredCountToken)}
                loading={loading}
                searchLabel="Search MCP profiles"
                searchPlaceholder="Search MCP profiles..."
                searchValue={profileSearchTerm}
                onSearchChange={setProfileSearchTerm}
                filters={(
                  <AIResourceTeamFilter
                    value={teamFilter}
                    onChange={openTeamFilter}
                    teamPaths={teamFilterOptions}
                    disabled={teamPathsLoading && teamFilterOptions.length === 0}
                  />
                )}
              />
            )}
            list={(
              <MCPProfileTable
                profiles={visibleProfiles}
                selectedProfileName={selectedWorkspaceProfileName}
                loading={loading}
                emptyMessage={profiles.length === 0 ? 'No MCP profiles configured.' : 'No MCP profiles match your filters.'}
                onSelectProfile={selectProfile}
              />
            )}
            detail={(
              <>
                {showProfileForm ? (
                  <MCPProfileForm
                    canManage={canManage}
                    saving={saving}
                    editingProfile={editingProfile}
                    profileForm={profileForm}
                    setProfileForm={setProfileForm}
                    createTeamPath={createProfileTeamPath}
                    teamPaths={teamPaths}
                    teamPathsLoading={teamPathsLoading}
                    onCreateTeamPathChange={setCreateProfileTeam}
                    onCreateLocalNameChange={setCreateProfileName}
                    servers={servers}
                    toggleProfileTool={toggleProfileTool}
                    setProfileServerTools={setProfileServerTools}
                    onSubmit={saveProfile}
                    onClose={() => setPanelMode(null)}
                  />
                ) : selectedProfile ? (
                  <MCPProfileDetail
                    profile={selectedProfile}
                    canManage={canManage}
                    saving={saving}
                    testing={testing}
                    onTest={testProfile}
                    onEdit={startProfileEdit}
                    onDelete={deleteProfile}
                  />
                ) : (
                  <AIResourceEmptyState>Select an MCP profile to inspect.</AIResourceEmptyState>
                )}
              </>
            )}
          />
        </>
      )}
    </div>
  );
}

function MCPServerForm({
  canManage,
  saving,
  editingServer,
  serverForm,
  setServerForm,
  createTeamPath,
  teamPaths,
  teamPathsLoading,
  onCreateTeamPathChange,
  onCreateLocalNameChange,
  onSubmit,
  onClose,
}: {
  canManage: boolean;
  saving: boolean;
  editingServer: string | null;
  serverForm: MCPServerFormState;
  setServerForm: Dispatch<SetStateAction<MCPServerFormState>>;
  createTeamPath: string;
  teamPaths: string[];
  teamPathsLoading: boolean;
  onCreateTeamPathChange: (value: string) => void;
  onCreateLocalNameChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
  onClose: () => void;
}) {
  const isCreate = !editingServer;

  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">{editingServer ? 'Edit server' : 'Create server'}</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{editingServer || 'New MCP server'}</h3>
        </div>
        <button type="button" className="glass-button-ghost !px-2" aria-label="Close server form" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <form className="space-y-4" onSubmit={onSubmit}>
        {isCreate && (
          <AIResourceTeamPlacementField
            teamPath={createTeamPath}
            onTeamPathChange={onCreateTeamPathChange}
            teamPaths={teamPaths}
            teamPathsLoading={teamPathsLoading}
            localName={aiResourceLocalName(serverForm.name)}
            resourceLabel="Server"
            disabled={!canManage}
          />
        )}
        <label className="flex flex-col gap-1 text-sm"><span>Name</span><input data-mcp-autofocus className="pipelines-input" value={isCreate ? aiResourceLocalName(serverForm.name) : serverForm.name} onChange={event => isCreate ? onCreateLocalNameChange(event.target.value) : setServerForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingServer)} placeholder="github" /></label>
        <label className="flex flex-col gap-1 text-sm"><span>Display name</span><input className="pipelines-input" value={serverForm.display_name} onChange={event => setServerForm(prev => ({ ...prev, display_name: event.target.value }))} disabled={!canManage} placeholder="GitHub MCP" /></label>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={serverForm.enabled} onChange={event => setServerForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} /> Enabled</label>
        <label className="flex flex-col gap-1 text-sm"><span>Provider</span><input className="pipelines-input" value={serverForm.provider} onChange={event => setServerForm(prev => ({ ...prev, provider: event.target.value }))} disabled={!canManage} placeholder="github" /></label>
        <label className="flex flex-col gap-1 text-sm"><span>Transport</span><select className="pipelines-input" value={serverForm.transport} onChange={event => setServerForm(prev => ({ ...prev, transport: event.target.value }))} disabled={!canManage}><option value="streamable_http">streamable_http</option><option value="http">http</option></select></label>
        <label className="flex flex-col gap-1 text-sm"><span>URL</span><input className="pipelines-input" value={serverForm.url} onChange={event => setServerForm(prev => ({ ...prev, url: event.target.value }))} disabled={!canManage} placeholder="https://api.githubcopilot.com/mcp/x/all/readonly" /></label>
        <label className="flex flex-col gap-1 text-sm"><span>Auth type</span><select className="pipelines-input" value={serverForm.auth_type} onChange={event => setServerForm(prev => ({ ...prev, auth_type: event.target.value }))} disabled={!canManage}><option value="none">none</option><option value="bearer_token">bearer_token</option></select></label>
        <label className="flex flex-col gap-1 text-sm">
          <span className="flex flex-wrap items-center gap-2">
            <span>Credential reference</span>
            <CredentialReferenceLink reference={serverForm.credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
              Open credential
            </CredentialReferenceLink>
          </span>
          <input className="pipelines-input" value={serverForm.credential_ref} onChange={event => setServerForm(prev => ({ ...prev, credential_ref: event.target.value }))} disabled={!canManage} placeholder="credential://system/mcp/github-readonly" />
          <span className="text-xs text-[var(--text-secondary)]">Expected type: bearer_token</span>
        </label>
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
        <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>{saving ? 'Saving...' : 'Save server'}</button>
      </form>
    </div>
  );
}

function MCPProfileForm({
  canManage,
  saving,
  editingProfile,
  profileForm,
  setProfileForm,
  createTeamPath,
  teamPaths,
  teamPathsLoading,
  onCreateTeamPathChange,
  onCreateLocalNameChange,
  servers,
  toggleProfileTool,
  setProfileServerTools,
  onSubmit,
  onClose,
}: {
  canManage: boolean;
  saving: boolean;
  editingProfile: string | null;
  profileForm: MCPProfileFormState;
  setProfileForm: Dispatch<SetStateAction<MCPProfileFormState>>;
  createTeamPath: string;
  teamPaths: string[];
  teamPathsLoading: boolean;
  onCreateTeamPathChange: (value: string) => void;
  onCreateLocalNameChange: (value: string) => void;
  servers: MCPServerRecord[];
  toggleProfileTool: (serverName: string, toolName: string) => void;
  setProfileServerTools: (serverName: string, value: string) => void;
  onSubmit: (event: FormEvent) => void;
  onClose: () => void;
}) {
  const isCreate = !editingProfile;

  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">{editingProfile ? 'Edit profile' : 'Create profile'}</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{editingProfile || 'New MCP profile'}</h3>
        </div>
        <button type="button" className="glass-button-ghost !px-2" aria-label="Close profile form" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <form className="space-y-4" onSubmit={onSubmit}>
        {isCreate && (
          <AIResourceTeamPlacementField
            teamPath={createTeamPath}
            onTeamPathChange={onCreateTeamPathChange}
            teamPaths={teamPaths}
            teamPathsLoading={teamPathsLoading}
            localName={aiResourceLocalName(profileForm.name)}
            resourceLabel="Profile"
            disabled={!canManage}
          />
        )}
        <label className="flex flex-col gap-1 text-sm"><span>Name</span><input data-mcp-autofocus className="pipelines-input" value={isCreate ? aiResourceLocalName(profileForm.name) : profileForm.name} onChange={event => isCreate ? onCreateLocalNameChange(event.target.value) : setProfileForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingProfile)} placeholder="github-pr-review" /></label>
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
        <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>{saving ? 'Saving...' : 'Save profile'}</button>
      </form>
    </div>
  );
}

function MCPServerDetail({
  server,
  canManage,
  saving,
  testing,
  onDiscover,
  onEdit,
  onDelete,
}: {
  server: MCPServerRecord;
  canManage: boolean;
  saving: boolean;
  testing: string | null;
  onDiscover: (name: string) => void | Promise<void>;
  onEdit: (server: MCPServerRecord) => void;
  onDelete: (name: string) => void | Promise<void>;
}) {
  const connected = isServerConnected(server);
  const statusLabel = server.last_test_status || (server.enabled ? 'Enabled' : 'Disabled');

  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <div className="ai-resource-detail__title">
            <h3>{server.display_name || server.name}</h3>
            <span className={`runner-pill ${server.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>{server.enabled ? 'Enabled' : 'Disabled'}</span>
            <span className={`ai-resource-health ${connected ? 'ai-resource-health--ok' : 'ai-resource-health--muted'}`}>
              <span aria-hidden="true" />
              {statusLabel}
            </span>
          </div>
          <div className="ai-resource-detail__provider">
            <span className="ai-resource-provider-glyph" aria-hidden="true">
              <Cable className="h-3.5 w-3.5" />
            </span>
            {server.provider || 'MCP server'}
          </div>
        </div>
        <div className="ai-resource-detail__actions">
          <AIResourceIconAction label={testing === server.name ? 'Discovering tools' : 'Discover tools'} tone="primary" onClick={() => void onDiscover(server.name)} disabled={Boolean(testing)}>
            <Wrench className="h-4 w-4" aria-hidden="true" />
          </AIResourceIconAction>
          {canManage && (
            <AIResourceIconAction label="Edit server" tone="accent" onClick={() => onEdit(server)} disabled={saving}>
              <Edit3 className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          )}
          <ResourceAccessCard
            resourceType="mcp_server"
            resourceID={server.name}
            label="MCP server"
            buttonClassName="ai-resource-icon-action"
            iconOnly
          />
        </div>
      </div>

      <MCPDetailSection
        title="Connection"
        rows={[
          { label: 'Team', value: <AIResourceTeamBadge resourceID={server.name} /> },
          { label: 'Name', value: server.name, mono: true },
          { label: 'URL', value: server.url || '-', mono: true },
          { label: 'Transport', value: server.transport || '-' },
          { label: 'Auth type', value: server.auth_type || 'none' },
          {
            label: 'Credential',
            value: server.credential_ref ? (
              <span className="ai-resource-detail-link">
                <CredentialReferenceLink reference={server.credential_ref} className="ai-resource-ref-link underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]" />
                <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
              </span>
            ) : '-',
          },
        ]}
      />

      <MCPDetailSection
        title="Runtime"
        rows={[
          { label: 'Timeout', value: server.timeout || '30s' },
          { label: 'Allowed scopes', value: formatMCPScopes(server.allowed_scopes) },
          { label: 'Headers', value: `${Object.keys(server.headers).length} configured` },
          { label: 'Tools', value: server.tools.length },
        ]}
      />

      <MCPDetailSection
        title="Discovery"
        rows={[
          { label: 'Last status', value: server.last_test_status || '-' },
          { label: 'Message', value: server.last_test_message || '-', full: true },
          { label: 'Last tested', value: formatTimestamp(server.last_tested_at) },
          { label: 'Last discovered', value: formatTimestamp(server.last_discovered_at) },
          { label: 'Protocol', value: server.discovered_protocol || '-' },
          { label: 'Version', value: server.discovered_version || '-' },
        ]}
      />

      <section className="ai-resource-detail-section">
        <h4>Tools</h4>
        {server.tools.length > 0 ? (
          <div className="ai-resource-tool-list">
            {server.tools.map(tool => (
              <div key={`${tool.server_name}-${tool.name}`}>
                <strong>{tool.name}</strong>
                {tool.description && <span>{tool.description}</span>}
              </div>
            ))}
          </div>
        ) : (
          <p className="ai-resource-detail-copy">No tools discovered yet.</p>
        )}
      </section>

      <div className="ai-resource-detail__footer">
        <button type="button" className="ai-resource-delete-link" onClick={() => void onDelete(server.name)} disabled={!canManage || saving}>
          <Trash2 className="h-4 w-4" aria-hidden="true" />
          Delete server
        </button>
      </div>
    </div>
  );
}

function MCPProfileDetail({
  profile,
  canManage,
  saving,
  testing,
  onTest,
  onEdit,
  onDelete,
}: {
  profile: MCPProfileRecord;
  canManage: boolean;
  saving: boolean;
  testing: string | null;
  onTest: (name: string) => void | Promise<void>;
  onEdit: (profile: MCPProfileRecord) => void;
  onDelete: (name: string) => void | Promise<void>;
}) {
  const toolCount = countMCPProfileTools(profile);

  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <div className="ai-resource-detail__title">
            <h3>{profile.name}</h3>
            <span className={`ai-resource-health ${profile.enabled ? 'ai-resource-health--ok' : 'ai-resource-health--muted'}`}>
              <span aria-hidden="true" />
              {profile.enabled ? 'Enabled' : 'Disabled'}
            </span>
          </div>
          <div className="ai-resource-detail__provider">
            <span className="ai-resource-provider-glyph" aria-hidden="true">
              <Boxes className="h-3.5 w-3.5" />
            </span>
            MCP profile
          </div>
        </div>
        <div className="ai-resource-detail__actions">
          <AIResourceIconAction label={testing === profile.name ? 'Testing profile' : 'Test profile'} tone="primary" onClick={() => void onTest(profile.name)} disabled={Boolean(testing)}>
            <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
          </AIResourceIconAction>
          {canManage && (
            <AIResourceIconAction label="Edit profile" tone="accent" onClick={() => onEdit(profile)} disabled={saving}>
              <Edit3 className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          )}
          <ResourceAccessCard
            resourceType="mcp_profile"
            resourceID={profile.name}
            label="MCP profile"
            buttonClassName="ai-resource-icon-action"
            iconOnly
          />
        </div>
      </div>

      <p className="ai-resource-detail-copy">{profile.description || 'No description provided.'}</p>

      <MCPDetailSection
        title="Access"
        rows={[
          { label: 'Team', value: <AIResourceTeamBadge resourceID={profile.name} /> },
          { label: 'Allowed scopes', value: formatMCPScopes(profile.allowed_scopes) },
          { label: 'Servers', value: profile.servers.length },
          { label: 'Approved tools', value: toolCount },
        ]}
      />

      <section className="ai-resource-detail-section">
        <h4>Server tools</h4>
        {profile.servers.length > 0 ? (
          <div className="ai-resource-tool-list">
            {profile.servers.map(ref => (
              <div key={ref.server}>
                <strong>{ref.server}</strong>
                <span>{ref.tools.length ? ref.tools.join(', ') : 'No tools selected'}</span>
              </div>
            ))}
          </div>
        ) : (
          <p className="ai-resource-detail-copy">No server tools approved yet.</p>
        )}
      </section>

      <div className="ai-resource-detail__footer">
        <button type="button" className="ai-resource-delete-link" onClick={() => void onDelete(profile.name)} disabled={!canManage || saving}>
          <Trash2 className="h-4 w-4" aria-hidden="true" />
          Delete profile
        </button>
      </div>
    </div>
  );
}

function isServerConnected(server: MCPServerRecord) {
  const status = (server.last_test_status || '').toLowerCase();
  return status.includes('connected') || status.includes('success') || status === 'ok';
}

function formatTimestamp(value?: string) {
  if (!value) return '-';
  const time = new Date(value);
  if (Number.isNaN(time.getTime())) return value;
  return time.toLocaleString();
}

export default MCPPanel;
