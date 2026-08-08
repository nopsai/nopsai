import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Activity, Bot, Copy, Edit3, FileText, Plus, Power, Trash2, X } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  agentProfileSourceLabel,
  type AgentProfileRecord,
} from './agent-profiles/model';
import { useAgentProfiles } from './agent-profiles/useAgentProfiles';
import {
  teamAgentProfileRecords,
  teamScopedDefaultID,
} from './teamProfileAdapters';
import { useTeamProfileWriteAccess } from './useTeamProfileWriteAccess';
import {
  AIResourceEmptyState,
  AIResourceIconAction,
  AIResourceTeamBadge,
  AIResourceTeamPlacementField,
  AIResourceTableHeader,
} from './AIResourcePanel';
import { AIResourceWorkspace, type AIResourceWorkspaceItem } from './AIResourceWorkspace';
import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  aiResourceLocalName,
  aiResourceMatchesTeamFilter,
  aiResourceRoute,
  aiResourceSearchParamsForTeamFilter,
  aiResourceTeamFilterFromSearch,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  collectAIResourceTeamPaths,
  formatAIResourceTeamLabel,
  normalizeAIResourceTeamPath,
} from './aiResourceTeams';
import { matchesAIResourceSearch } from './aiResourcePresentation';
import { aiResourceTreeFilterForResource } from './aiResourceTree';
import { useAIResourceTeamPaths } from './useAIResourceTeamPaths';
import { ObjectIcon } from '../../components/ObjectIcon';
import ResourceAccessCard from '../../components/ResourceAccessCard';

type AgentProfilesPanelProps = {
  canManage: boolean;
  selectedProfileID?: string;
  onSelectedProfileIDChange?: (profileID: string) => void;
};

function AgentProfilesPanel({
  canManage,
  selectedProfileID: controlledSelectedProfileID,
  onSelectedProfileIDChange,
}: AgentProfilesPanelProps) {
  const panelRef = useRef<HTMLElement | null>(null);
  const location = useLocation();
  const navigate = useNavigate();
  const requestedTeamFilter = useMemo(() => aiResourceTeamFilterFromSearch(location.search), [location.search]);
  const [searchTerm, setSearchTerm] = useState('');
  const [teamFilter, setTeamFilter] = useState(requestedTeamFilter);
  const [createTeamPath, setCreateTeamPath] = useState('');
  const [uncontrolledSelectedProfileID, setUncontrolledSelectedProfileID] = useState('');
  const selectedProfileID = controlledSelectedProfileID ?? uncontrolledSelectedProfileID;
  const setSelectedProfileID = (profileID: string | null) => {
    const nextProfileID = profileID || '';
    if (onSelectedProfileIDChange) onSelectedProfileIDChange(nextProfileID);
    else setUncontrolledSelectedProfileID(nextProfileID);
  };
  const { teamPaths, teamPathsLoading } = useAIResourceTeamPaths();
  const selectedTeamPath = useMemo(() => {
    if (teamFilter === AI_RESOURCE_TEAM_FILTER_ALL || teamFilter === AI_RESOURCE_TEAM_FILTER_GLOBAL) return '';
    return normalizeAIResourceTeamPath(teamFilter);
  }, [teamFilter]);
  const teamWriteAccess = useTeamProfileWriteAccess(selectedTeamPath);
  const canManageTeamProfiles = canManage || teamWriteAccess.allowed;
  const {
    payload,
    teamProfilesPayload,
    teamProfilesByPath,
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
    loadTeamProfiles,
    loadTeamProfilesForTree,
    openUsage,
    openSource,
    startCreate,
    startDuplicate,
    startEdit,
    saveProfile,
    deleteProfile,
    toggleProfileEnabled,
    setDefaultProfile,
  } = useAgentProfiles({
    canManage,
    canManageTeamProfiles,
    onProfileSaved: profileID => {
      setSelectedProfileID(profileID);
      setTeamFilter(aiResourceTreeFilterForResource(profileID));
    },
  });

  useEffect(() => {
    setTeamFilter(requestedTeamFilter);
  }, [requestedTeamFilter]);

  useEffect(() => {
    if (!selectedProfileID) return;
    setTeamFilter(aiResourceTreeFilterForResource(selectedProfileID));
  }, [selectedProfileID]);

  useEffect(() => {
    void loadTeamProfiles(selectedTeamPath);
  }, [loadTeamProfiles, selectedTeamPath]);

  useEffect(() => {
    if (!panelMode) return;
    window.requestAnimationFrame(() => {
      if (window.innerWidth < 1280) {
        panelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
      const focusTarget =
        panelRef.current?.querySelector<HTMLElement>('[data-agent-profile-autofocus]:not(:disabled)') ??
        panelRef.current?.querySelector<HTMLElement>('input:not(:disabled), textarea:not(:disabled), button:not(:disabled)');
      focusTarget?.focus({ preventScroll: true });
    });
  }, [editingID, panelMode]);

  const teamFilterOptions = useMemo(
    () => collectAIResourceTeamPaths(payload.profiles.map(profile => profile.id), teamPaths),
    [payload.profiles, teamPaths]
  );
  useEffect(() => {
    loadTeamProfilesForTree(teamFilterOptions);
  }, [loadTeamProfilesForTree, teamFilterOptions]);
  const selectedTeamPayload = selectedTeamPath && normalizeAIResourceTeamPath(teamProfilesPayload?.team_path || '') === selectedTeamPath
    ? teamProfilesPayload
    : null;
  const teamScopedProfiles = useMemo(
    () => teamAgentProfileRecords(selectedTeamPayload),
    [selectedTeamPayload]
  );
  const catalogScopedProfiles = useMemo(
    () => selectedTeamPath ? payload.profiles.filter(profile => aiResourceTeamScope(profile.id).teamPath === selectedTeamPath) : [],
    [payload.profiles, selectedTeamPath]
  );
  const cachedTeamScopedProfiles = useMemo(
    () => Object.values(teamProfilesByPath).flatMap(teamAgentProfileRecords),
    [teamProfilesByPath]
  );
  const sourceProfiles = useMemo(() => {
    const byID = new Map<string, AgentProfileRecord>();
    payload.profiles.forEach(profile => byID.set(profile.id, profile));
    if (!selectedTeamPath) {
      cachedTeamScopedProfiles.forEach(profile => byID.set(profile.id, profile));
      return Array.from(byID.values()).sort((a, b) => a.display_name.localeCompare(b.display_name, undefined, { sensitivity: 'base' }));
    }
    byID.clear();
    catalogScopedProfiles.forEach(profile => byID.set(profile.id, profile));
    teamScopedProfiles.forEach(profile => byID.set(profile.id, profile));
    return Array.from(byID.values()).sort((a, b) => a.display_name.localeCompare(b.display_name, undefined, { sensitivity: 'base' }));
  }, [cachedTeamScopedProfiles, catalogScopedProfiles, payload.profiles, selectedTeamPath, teamScopedProfiles]);
  const workspaceProfiles = useMemo(() => {
    const byID = new Map<string, AgentProfileRecord>();
    payload.profiles.forEach(profile => byID.set(profile.id, profile));
    cachedTeamScopedProfiles.forEach(profile => byID.set(profile.id, profile));
    teamScopedProfiles.forEach(profile => byID.set(profile.id, profile));
    return Array.from(byID.values()).sort((a, b) => a.display_name.localeCompare(b.display_name, undefined, { sensitivity: 'base' }));
  }, [cachedTeamScopedProfiles, payload.profiles, teamScopedProfiles]);
  const activeDefaultProfile = selectedTeamPath
    ? teamScopedDefaultID(selectedTeamPath, selectedTeamPayload?.default_profile || '')
    : payload.default_profile;
  const defaultProfileRecord = sourceProfiles.find(profile => profile.id === activeDefaultProfile) || null;
  const defaultProfileOptions = useMemo(() => {
    if (selectedTeamPath) return sourceProfiles.filter(profile => profile.enabled);
    const options = payload.profiles.filter(profile => profile.enabled && aiResourceTeamScope(profile.id).teamPath === '');
    const globalDefaultRecord = payload.profiles.find(profile => profile.id === payload.default_profile) || null;
    if (globalDefaultRecord && aiResourceTeamScope(globalDefaultRecord.id).teamPath === '' && !options.some(profile => profile.id === globalDefaultRecord.id)) {
      options.push(globalDefaultRecord);
    }
    return options;
  }, [payload.default_profile, payload.profiles, selectedTeamPath, sourceProfiles]);
  const canManageCurrentScope = selectedTeamPath ? canManageTeamProfiles : canManage;
  const canManageDefault = canManageCurrentScope && defaultProfileOptions.length > 0;
  const defaultControlLoading = loading || saving || (selectedTeamPath ? teamProfilesLoading || teamWriteAccess.loading : false);
  const listLoading = loading || (selectedTeamPath ? teamProfilesLoading : false);
  const visibleProfiles = useMemo(
    () => sourceProfiles.filter(profile => matchesAIResourceSearch(
      searchTerm,
      profile.display_name,
      profile.id,
      aiResourceLocalName(profile.id),
      formatAIResourceTeamLabel(profile.id),
      profile.role,
      profile.description,
      profile.instructions,
      agentProfileSourceLabel(profile.source),
      profile.enabled ? 'enabled' : 'disabled',
      profile.usage_count,
      profile.last_updated,
      profile.source_path
    ) && aiResourceMatchesTeamFilter(profile.id, teamFilter)),
    [sourceProfiles, searchTerm, teamFilter]
  );

  const selectedListProfile = useMemo(
    () => selectedProfileID ? visibleProfiles.find(profile => profile.id === selectedProfileID) ?? null : null,
    [selectedProfileID, visibleProfiles]
  );
  const showForm = panelMode === 'create' || panelMode === 'edit';
  const detailProfile = (panelMode ? selectedProfile : null) ?? selectedListProfile;
  const workspaceResources = useMemo<AIResourceWorkspaceItem[]>(
    () => workspaceProfiles.map(profile => ({
      id: profile.id,
      label: profile.display_name || aiResourceLocalName(profile.id) || profile.id,
      description: profile.role || profile.description || profile.id,
    })),
    [workspaceProfiles]
  );
  const selectedWorkspaceProfileID = panelMode === 'create' ? null : detailProfile?.id ?? selectedProfileID;
  const detailOpen = showForm || panelMode === 'usage' || panelMode === 'source' || panelMode === 'delete' || Boolean(selectedWorkspaceProfileID);
  const selectProfile = (profileID: string) => {
    setSelectedProfileID(profileID);
    setTeamFilter(aiResourceTreeFilterForResource(profileID));
    setDeleteBlocker(null);
    setPanelMode(null);
  };
  const closeDetail = () => {
    setSelectedProfileID(null);
    setDeleteBlocker(null);
    setPanelMode(null);
  };
  const openTeamFilter = (value: string) => {
    setTeamFilter(value);
    if (!onSelectedProfileIDChange) setUncontrolledSelectedProfileID('');
    setDeleteBlocker(null);
    setPanelMode(null);
    navigate(
      aiResourceRoute('/agent-profiles', '', aiResourceSearchParamsForTeamFilter(new URLSearchParams(location.search), value)),
      { preventScrollReset: true }
    );
  };
  const openCreate = () => {
    const initialTeamPath = selectedTeamPath;
    setCreateTeamPath(initialTeamPath);
    startCreate();
    setForm(prev => ({ ...prev, id: buildAIResourceScopedID(initialTeamPath, aiResourceLocalName(prev.id)) }));
  };
  const openDuplicate = (profile: AgentProfileRecord) => {
    setCreateTeamPath(normalizeAIResourceTeamPath(aiResourceTeamScope(profile.id).teamPath));
    startDuplicate(profile);
  };
  const openEdit = (profile: AgentProfileRecord) => {
    setCreateTeamPath(normalizeAIResourceTeamPath(
      profile.scope === 'team' ? profile.team_path || aiResourceTeamScope(profile.id).teamPath : aiResourceTeamScope(profile.id).teamPath
    ));
    startEdit(profile);
  };
  const setCreateTeam = (teamPath: string) => {
    setCreateTeamPath(teamPath);
    setForm(prev => ({ ...prev, id: buildAIResourceScopedID(teamPath, aiResourceLocalName(prev.id)) }));
  };
  const setCreateScopedID = (localID: string) => {
    setForm(prev => ({ ...prev, id: buildAIResourceScopedID(createTeamPath, localID) }));
  };
  return (
    <div id="system-agent-profiles-section" className="ai-resource-panel ai-resource-page space-y-5 pb-24">
      <div className="ai-resource-page-header ai-resource-page-header--toolbar ai-resource-overview-bar">
        <h2 className="sr-only">Agent Profiles</h2>
        <div className="ai-resource-default-control">
          <span>Default profile</span>
          {canManageDefault ? (
            <select
              className="ai-resource-default-select"
              value={activeDefaultProfile}
              onChange={event => void setDefaultProfile(event.target.value, selectedTeamPath ? { teamPath: selectedTeamPath } : undefined)}
              disabled={defaultControlLoading}
              aria-label="Default agent profile"
            >
              {selectedTeamPath && <option value="">No default</option>}
              {defaultProfileOptions.map(profile => (
                <option key={profile.id} value={profile.id}>
                  {profile.display_name}
                </option>
              ))}
            </select>
          ) : (
            <strong>{defaultProfileRecord?.display_name || activeDefaultProfile || (selectedTeamPath ? 'No default' : '-')}</strong>
          )}
        </div>
      </div>

      {error && <div className="ai-resource-alert ai-resource-alert--error">{error}</div>}
      {teamProfilesError && <div className="ai-resource-alert ai-resource-alert--error">{teamProfilesError}</div>}

      <AIResourceWorkspace
        storageKey="agent-profiles"
        workspaceLabel="Agent profile workspace"
        treeTitle="Agent profile tree"
        resourceType="agent-profile"
        resourceLabel="agent profile"
        resources={workspaceResources}
        teamPaths={teamFilterOptions}
        teamFilter={teamFilter}
        selectedResourceID={selectedWorkspaceProfileID}
        onTeamFilterChange={openTeamFilter}
        onResourceSelect={selectProfile}
        onDetailClose={closeDetail}
        detailOpen={detailOpen}
        detailRef={panelRef}
        detailLabel="Agent profile detail"
        listHeader={(
          <AIResourceTableHeader
            loading={listLoading}
            searchLabel="Search agent profiles"
            searchPlaceholder="Search agent profiles..."
            searchValue={searchTerm}
            onSearchChange={setSearchTerm}
            filters={!canManageCurrentScope ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
            actions={canManageCurrentScope ? (
              <button
                type="button"
                className="ai-resource-primary-button ai-resource-create-button"
                aria-label="New profile"
                title="New profile"
                onClick={openCreate}
                disabled={saving}
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
            className="ai-resource-table-head--list-actions"
          />
        )}
        list={(
          <AgentProfileTable
            profiles={visibleProfiles}
            defaultProfile={activeDefaultProfile}
            selectedProfileID={selectedWorkspaceProfileID}
            loading={listLoading}
            emptyMessage={sourceProfiles.length === 0 ? 'No agent profiles configured.' : 'No agent profiles match your filters.'}
            onSelectProfile={selectProfile}
          />
        )}
        detail={(
          <>
            {showForm ? (
              <AgentProfileForm
                canManage={canManageCurrentScope}
                saving={saving}
                editingID={editingID}
                form={form}
                setForm={setForm}
                createTeamPath={createTeamPath}
                teamPaths={teamPaths}
                teamPathsLoading={teamPathsLoading}
                onCreateTeamPathChange={setCreateTeam}
                onCreateLocalIDChange={setCreateScopedID}
                onSubmit={saveProfile}
                onClose={() => setPanelMode(null)}
              />
            ) : panelMode === 'usage' && selectedProfile ? (
              <AgentProfileUsageDetail profile={selectedProfile} onClose={() => setPanelMode(null)} />
            ) : panelMode === 'source' && selectedProfile ? (
              <AgentProfileSourceDetail profile={selectedProfile} onClose={() => setPanelMode(null)} />
            ) : panelMode === 'delete' && deleteBlocker ? (
              <AgentProfileDeleteBlocker
                blocker={deleteBlocker}
                saving={saving}
                onForceDelete={deleteProfile}
                onClose={() => { setDeleteBlocker(null); setPanelMode(null); }}
              />
            ) : detailProfile ? (
              <AgentProfileDetail
                profile={detailProfile}
                defaultProfile={activeDefaultProfile}
                canManage={canManageCurrentScope}
                saving={saving}
                onDuplicate={openDuplicate}
                onEdit={openEdit}
                onToggleEnabled={toggleProfileEnabled}
                onDelete={(id: string) => deleteProfile(id, { teamPath: detailProfile.team_path })}
                onUsage={openUsage}
                onSource={openSource}
              />
            ) : (
              <AIResourceEmptyState>Select an agent profile to inspect.</AIResourceEmptyState>
            )}
          </>
        )}
      />
    </div>
  );
}

function AgentProfileTable({
  profiles,
  defaultProfile,
  selectedProfileID,
  loading,
  emptyMessage,
  onSelectProfile,
}: {
  profiles: AgentProfileRecord[];
  defaultProfile: string;
  selectedProfileID: string | null | undefined;
  loading: boolean;
  emptyMessage: string;
  onSelectProfile: (profileID: string) => void;
}) {
  if (!loading && profiles.length === 0) {
    return <AIResourceEmptyState>{emptyMessage}</AIResourceEmptyState>;
  }

  return (
    <div className="ai-resource-table-shell">
      <table className="ai-resource-registry-table" aria-label="Agent profiles">
        <colgroup>
          <col style={{ width: '30%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '22%' }} />
          <col style={{ width: '8%' }} />
          <col style={{ width: '12%' }} />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">Profile</th>
            <th scope="col">Team</th>
            <th scope="col">Source</th>
            <th scope="col">Role</th>
            <th scope="col">Usage</th>
            <th scope="col">Status</th>
          </tr>
        </thead>
        <tbody>
          {profiles.map(profile => {
            const selected = selectedProfileID === profile.id;
            const isDefault = profile.id === defaultProfile;
            return (
              <tr key={profile.id} className={selected ? 'selected' : ''} onClick={() => onSelectProfile(profile.id)}>
                <td>
                  <button
                    type="button"
                    className="ai-resource-table-resource"
                    aria-label={`Select agent profile ${profile.display_name || profile.id}`}
                    onClick={event => {
                      event.stopPropagation();
                      onSelectProfile(profile.id);
                    }}
                  >
                    <span className="ai-resource-table-resource-icon" aria-hidden="true">
                      <ObjectIcon type="agent-profile" />
                    </span>
                    <span className="ai-resource-table-resource-name">
                      <strong>{profile.display_name}</strong>
                    </span>
                  </button>
                </td>
                <td><AIResourceTeamBadge resourceID={profile.id} /></td>
                <td><span className="ai-resource-table-mono">{agentProfileSourceLabel(profile.source)}</span></td>
                <td>{profile.role || '-'}</td>
                <td><span className="ai-resource-table-mono ai-resource-table-number">{profile.usage_count}</span></td>
                <td>
                  <span className={`ai-resource-health ${profile.enabled ? 'ai-resource-health--ok' : 'ai-resource-health--muted'}`}>
                    <span aria-hidden="true" />
                    {profile.enabled ? (isDefault ? 'Default' : 'Enabled') : 'Disabled'}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function AgentProfileForm({
  canManage,
  saving,
  editingID,
  form,
  setForm,
  createTeamPath,
  teamPaths,
  teamPathsLoading,
  onCreateTeamPathChange,
  onCreateLocalIDChange,
  onSubmit,
  onClose,
}: {
  canManage: boolean;
  saving: boolean;
  editingID: string | null;
  form: ReturnType<typeof useAgentProfiles>['form'];
  setForm: ReturnType<typeof useAgentProfiles>['setForm'];
  createTeamPath: string;
  teamPaths: string[];
  teamPathsLoading: boolean;
  onCreateTeamPathChange: (value: string) => void;
  onCreateLocalIDChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
  onClose: () => void;
}) {
  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">{editingID ? 'Edit profile' : 'Create profile'}</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{editingID || 'New agent profile'}</h3>
        </div>
        <button type="button" className="glass-button-ghost !px-2" aria-label="Close agent profile form" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <form className="space-y-4" onSubmit={onSubmit}>
        <AIResourceTeamPlacementField
          teamPath={createTeamPath}
          onTeamPathChange={onCreateTeamPathChange}
          teamPaths={teamPaths}
          teamPathsLoading={teamPathsLoading}
          localName={aiResourceLocalName(form.id)}
          resourceLabel="Profile"
          disabled={!canManage}
        />
        <label className="flex flex-col gap-1 text-sm">
          <span>ID</span>
          <input
            data-agent-profile-autofocus
            className="pipelines-input"
            value={aiResourceLocalName(form.id)}
            onChange={event => onCreateLocalIDChange(event.target.value)}
            disabled={!canManage || Boolean(editingID)}
            placeholder="security-reviewer"
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Name</span>
          <input className="pipelines-input" value={form.display_name} onChange={event => setForm(prev => ({ ...prev, display_name: event.target.value }))} disabled={!canManage} placeholder="Security Reviewer" />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Prompt role override</span>
          <input className="pipelines-input" value={form.role} onChange={event => setForm(prev => ({ ...prev, role: event.target.value }))} disabled={!canManage} placeholder="Defaults to profile name" />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Description</span>
          <input className="pipelines-input" value={form.description} onChange={event => setForm(prev => ({ ...prev, description: event.target.value }))} disabled={!canManage} placeholder="Reviews release security risk." />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Instructions</span>
          <textarea className="pipelines-input min-h-36" value={form.instructions} onChange={event => setForm(prev => ({ ...prev, instructions: event.target.value }))} disabled={!canManage} placeholder="Focus on practical risk reduction and least privilege." />
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={form.enabled} onChange={event => setForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} />
          Enabled
        </label>
        <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>
          {saving ? 'Saving...' : 'Save profile'}
        </button>
      </form>
    </div>
  );
}

function AgentProfileDetail({
  profile,
  defaultProfile,
  canManage,
  saving,
  onDuplicate,
  onEdit,
  onToggleEnabled,
  onDelete,
  onUsage,
  onSource,
}: {
  profile: AgentProfileRecord;
  defaultProfile: string;
  canManage: boolean;
  saving: boolean;
  onDuplicate: (profile: AgentProfileRecord) => void;
  onEdit: (profile: AgentProfileRecord) => void;
  onToggleEnabled: (profile: AgentProfileRecord) => void | Promise<void>;
  onDelete: (id: string) => void | Promise<void>;
  onUsage: (profile: AgentProfileRecord) => void;
  onSource: (profile: AgentProfileRecord) => void;
}) {
  const sourceLabel = agentProfileSourceLabel(profile.source);
  const isDefault = profile.id === defaultProfile;
  const readOnly = Boolean(profile.read_only || profile.built_in || profile.source === 'gitops');
  const canEdit = canManage && !readOnly;

  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <div className="ai-resource-detail__title">
            <h3>{profile.display_name}</h3>
            {isDefault && <span className="runner-pill runner-pill--ok">Default</span>}
            <span className={`ai-resource-health ${profile.enabled ? 'ai-resource-health--ok' : 'ai-resource-health--muted'}`}>
              <span aria-hidden="true" />
              {profile.enabled ? 'Enabled' : 'Disabled'}
            </span>
          </div>
          <div className="ai-resource-detail__provider">
            <span className="ai-resource-provider-glyph" aria-hidden="true">
              <Bot className="h-3.5 w-3.5" />
            </span>
            {sourceLabel}
          </div>
        </div>
        <div className="ai-resource-detail__actions">
          <AIResourceIconAction label="Duplicate" tone="primary" onClick={() => onDuplicate(profile)} disabled={!canManage || saving}>
            <Copy className="h-4 w-4" aria-hidden="true" />
          </AIResourceIconAction>
          {canEdit && (
            <AIResourceIconAction label="Edit profile" tone="accent" onClick={() => onEdit(profile)} disabled={saving}>
              <Edit3 className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          )}
          <ResourceAccessCard
            resourceType="agent_profile"
            resourceID={profile.id}
            label="agent profile"
            buttonClassName="ai-resource-icon-action"
            iconOnly
          />
          {profile.source === 'gitops' || profile.source_path ? (
            <AIResourceIconAction label="View GitOps source" onClick={() => onSource(profile)}>
              <FileText className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          ) : null}
          {profile.usage_count > 0 && (
            <AIResourceIconAction label="View usage" onClick={() => onUsage(profile)}>
              <Activity className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          )}
          {canEdit && (
            <AIResourceIconAction
              label={profile.enabled ? 'Disable profile' : 'Enable profile'}
              tone={profile.enabled ? 'warning' : 'primary'}
              onClick={() => void onToggleEnabled(profile)}
              disabled={saving || isDefault}
            >
              <Power className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          )}
          {canEdit && (
            <AIResourceIconAction
              label="Delete profile"
              tone="danger"
              onClick={() => void onDelete(profile.id)}
              disabled={saving || isDefault}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </AIResourceIconAction>
          )}
        </div>
      </div>

      <p className="ai-resource-detail-copy">{profile.description || 'No description provided.'}</p>

      <AgentDetailSection
        title="Profile"
        rows={[
          { label: 'Team', value: <AIResourceTeamBadge resourceID={profile.id} /> },
          { label: 'ID', value: profile.id, mono: true },
          { label: 'Prompt role', value: profile.role || 'Uses profile name' },
          { label: 'Source', value: sourceLabel },
          { label: 'Last updated', value: profile.last_updated ? new Date(profile.last_updated).toLocaleString() : '-' },
        ]}
      />

      <AgentDetailSection
        title="Instructions"
        rows={[
          {
            label: 'System prompt',
            value: <pre className="ai-resource-detail-pre">{profile.instructions || 'No instructions configured.'}</pre>,
            full: true,
          },
        ]}
      />

      <AgentDetailSection
        title="Usage"
        rows={[
          { label: 'Used by', value: `${profile.usage_count} reference${profile.usage_count === 1 ? '' : 's'}` },
          { label: 'Access grants', value: 'Managed per profile' },
        ]}
      />

      {(isDefault || readOnly) && (
        <div className="ai-resource-detail__footer">
          {isDefault && <p>Default profiles cannot be deleted.</p>}
          {readOnly && !isDefault && <p>{sourceLabel} profiles are managed outside this page.</p>}
        </div>
      )}
    </div>
  );
}

function AgentProfileUsageDetail({ profile, onClose }: { profile: AgentProfileRecord; onClose: () => void }) {
  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">Usage</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{profile.display_name}</h3>
        </div>
        <button type="button" className="glass-button-ghost !px-2" aria-label="Close usage panel" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <p className="text-[var(--text-secondary)]">{profile.usage_count} explicit pipeline or step reference{profile.usage_count === 1 ? '' : 's'}.</p>
      <ul className="ai-resource-reference-list">
        {profile.references.map(ref => <li key={ref}>{ref}</li>)}
      </ul>
    </div>
  );
}

function AgentProfileSourceDetail({ profile, onClose }: { profile: AgentProfileRecord; onClose: () => void }) {
  const roleLine = profile.role ? `    role: ${profile.role}\n` : '';
  const source = `agent_profiles:\n  - id: ${profile.id}\n    display_name: ${profile.display_name}\n${roleLine}    enabled: ${profile.enabled}\n    description: ${profile.description || ''}\n    instructions: |\n${profile.instructions.split('\n').map(line => `      ${line}`).join('\n')}`;
  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">GitOps source</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{profile.display_name}</h3>
        </div>
        <button type="button" className="glass-button-ghost !px-2" aria-label="Close source panel" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <p className="text-[var(--text-secondary)]">{profile.source_path || 'GitOps source path unavailable.'}</p>
      <pre className="ai-resource-detail-pre ai-resource-detail-pre--tall">{source}</pre>
    </div>
  );
}

function AgentProfileDeleteBlocker({
  blocker,
  saving,
  onForceDelete,
  onClose,
}: {
  blocker: { id: string; references: string[] };
  saving: boolean;
  onForceDelete: (id: string, opts?: { force?: boolean }) => void | Promise<void>;
  onClose: () => void;
}) {
  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">Delete blocked</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">{blocker.id}</h3>
        </div>
        <button type="button" className="glass-button-ghost !px-2" aria-label="Close delete panel" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 space-y-3">
        <p className="font-semibold text-[var(--text-primary)]">Profile is still referenced.</p>
        <ul className="list-disc pl-5 text-[var(--text-secondary)] max-h-36 overflow-auto">
          {blocker.references.map(ref => <li key={ref}>{ref}</li>)}
        </ul>
        <button type="button" className="glass-button-danger" disabled={saving} onClick={() => void onForceDelete(blocker.id, { force: true })}>
          <Trash2 className="h-4 w-4" aria-hidden="true" />
          Force delete
        </button>
      </div>
    </div>
  );
}

function AgentDetailSection({ title, rows }: { title: string; rows: Array<{ label: string; value: ReactNode; mono?: boolean; full?: boolean }> }) {
  return (
    <section className="ai-resource-detail-section">
      <h4>{title}</h4>
      <dl>
        {rows.map(row => (
          <div key={row.label} className={`ai-resource-detail-row ${row.full ? 'ai-resource-detail-row--full' : ''}`}>
            {!row.full && <dt>{row.label}</dt>}
            <dd className={row.mono ? 'ai-resource-detail-row__mono' : undefined}>{row.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export default AgentProfilesPanel;
