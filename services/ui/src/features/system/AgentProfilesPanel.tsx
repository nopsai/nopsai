import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Activity, Bot, Copy, Edit3, FileText, Plus, Power, RefreshCw, Trash2, X } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import {
  agentProfileSourceLabel,
  type AgentProfileRecord,
} from './agent-profiles/model';
import { useAgentProfiles } from './agent-profiles/useAgentProfiles';
import {
  AIResourceEmptyState,
  AIResourceIconAction,
  AIResourceTeamBadge,
  AIResourceTeamFilter,
  AIResourceTeamPlacementField,
  AIResourceTableHeader,
} from './AIResourcePanel';
import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  aiResourceLocalName,
  aiResourceMatchesTeamFilter,
  aiResourceTeamFilterFromSearch,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  collectAIResourceTeamPaths,
  formatAIResourceTeamLabel,
} from './aiResourceTeams';
import {
  formatFilteredCount,
  matchesAIResourceSearch,
} from './aiResourcePresentation';
import { useAIResourceTeamPaths } from './useAIResourceTeamPaths';
import ResourceAccessCard from '../../components/ResourceAccessCard';

function AgentProfilesPanel({ canManage }: { canManage: boolean }) {
  const panelRef = useRef<HTMLElement | null>(null);
  const location = useLocation();
  const requestedTeamFilter = useMemo(() => aiResourceTeamFilterFromSearch(location.search), [location.search]);
  const [searchTerm, setSearchTerm] = useState('');
  const [teamFilter, setTeamFilter] = useState(requestedTeamFilter);
  const [createTeamPath, setCreateTeamPath] = useState('');
  const [selectedProfileID, setSelectedProfileID] = useState<string | null>(null);
  const { teamPaths, teamPathsLoading } = useAIResourceTeamPaths();
  const {
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
    openUsage,
    openSource,
    startCreate,
    startDuplicate,
    startEdit,
    saveProfile,
    deleteProfile,
    toggleProfileEnabled,
    setDefaultProfile,
  } = useAgentProfiles({ canManage });

  useEffect(() => {
    setTeamFilter(requestedTeamFilter);
  }, [requestedTeamFilter]);

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

  const visibleProfiles = useMemo(
    () => payload.profiles.filter(profile => matchesAIResourceSearch(
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
    [payload.profiles, searchTerm, teamFilter]
  );

  const selectedListProfile = useMemo(() => {
    const selected = visibleProfiles.find(profile => profile.id === selectedProfileID);
    if (selected) return selected;
    return (
      visibleProfiles.find(profile => profile.id === payload.default_profile) ??
      visibleProfiles[0] ??
      null
    );
  }, [payload.default_profile, selectedProfileID, visibleProfiles]);

  const defaultProfileRecord = payload.profiles.find(profile => profile.id === payload.default_profile) || null;
  const defaultProfileOptions = useMemo(() => {
    const options = payload.profiles.filter(profile => profile.enabled && aiResourceTeamScope(profile.id).teamPath === '');
    if (defaultProfileRecord && aiResourceTeamScope(defaultProfileRecord.id).teamPath === '' && !options.some(profile => profile.id === defaultProfileRecord.id)) {
      options.push(defaultProfileRecord);
    }
    return options;
  }, [defaultProfileRecord, payload.profiles]);
  const canManageGlobalDefault = canManage && defaultProfileOptions.length > 0;
  const enabledCount = payload.profiles.filter(profile => profile.enabled).length;
  const builtInCount = payload.profiles.filter(profile => profile.source === 'built-in' || profile.built_in).length;
  const teamFilterOptions = useMemo(
    () => collectAIResourceTeamPaths(payload.profiles.map(profile => profile.id), teamPaths),
    [payload.profiles, teamPaths]
  );
  const showForm = panelMode === 'create' || panelMode === 'edit';
  const detailProfile = selectedProfile ?? selectedListProfile;
  const filteredCountToken = searchTerm || (teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL ? teamFilter : '');
  const openCreate = () => {
    const initialTeamPath = teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL && teamFilter !== AI_RESOURCE_TEAM_FILTER_GLOBAL ? teamFilter : '';
    setCreateTeamPath(initialTeamPath);
    startCreate();
    setForm(prev => ({ ...prev, id: buildAIResourceScopedID(initialTeamPath, aiResourceLocalName(prev.id)) }));
  };
  const openDuplicate = (profile: AgentProfileRecord) => {
    setCreateTeamPath(aiResourceTeamScope(profile.id).teamPath);
    startDuplicate(profile);
  };
  const setCreateTeam = (teamPath: string) => {
    setCreateTeamPath(teamPath);
    setForm(prev => ({ ...prev, id: buildAIResourceScopedID(teamPath, aiResourceLocalName(prev.id)) }));
  };
  const setCreateScopedID = (localID: string) => {
    setForm(prev => ({ ...prev, id: buildAIResourceScopedID(createTeamPath, localID) }));
  };

  return (
    <div id="system-agent-profiles-section" className="ai-resource-panel space-y-5 pb-24">
      <div className="ai-resource-page-header">
        <div>
          <h2>Agent Profiles</h2>
          <p>Manage automation roles, instructions, and team access.</p>
        </div>
        <div className="ai-resource-page-actions">
          {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
          <button type="button" className="ai-resource-icon-button" onClick={() => void loadProfiles()} disabled={loading || saving} aria-label="Reload">
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
          </button>
          {canManage && (
            <button type="button" className="ai-resource-primary-button" onClick={openCreate} disabled={saving}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New profile
            </button>
          )}
        </div>
      </div>

      <div className="ai-resource-summary-band">
        <div className="ai-resource-summary-item">
          <span>Default resolution</span>
          {canManageGlobalDefault ? (
            <select
              className="ai-resource-summary-select"
              value={payload.default_profile}
              onChange={event => void setDefaultProfile(event.target.value)}
              disabled={loading || saving}
              aria-label="Default agent profile"
            >
              {defaultProfileOptions.map(profile => (
                <option key={profile.id} value={profile.id}>
                  {profile.display_name}
                </option>
              ))}
            </select>
          ) : (
            <strong>{defaultProfileRecord?.display_name || payload.default_profile || '-'}</strong>
          )}
        </div>
        <div className="ai-resource-summary-item">
          <span>Profiles</span>
          <strong>{payload.profiles.length}</strong>
        </div>
        <div className="ai-resource-summary-item">
          <span>Enabled</span>
          <strong className={enabledCount === payload.profiles.length ? 'text-emerald-600' : undefined}>{enabledCount} of {payload.profiles.length}</strong>
        </div>
        <div className="ai-resource-summary-item">
          <span>Built-in</span>
          <strong>{builtInCount}</strong>
        </div>
      </div>

      {error && <div className="ai-resource-alert ai-resource-alert--error">{error}</div>}

      <section className="ai-resource-table-card ai-resource-split-card">
        <div className="ai-resource-split">
          <div className="ai-resource-split__list">
            <AIResourceTableHeader
              title="Profiles"
              count={formatFilteredCount(visibleProfiles.length, payload.profiles.length, filteredCountToken)}
              loading={loading}
              searchLabel="Search agent profiles"
              searchPlaceholder="Search agent profiles..."
              searchValue={searchTerm}
              onSearchChange={setSearchTerm}
              filters={(
                <AIResourceTeamFilter
                  value={teamFilter}
                  onChange={setTeamFilter}
                  teamPaths={teamFilterOptions}
                  disabled={teamPathsLoading && teamFilterOptions.length === 0}
                />
              )}
            />
            <div className="ai-resource-profile-list">
              {visibleProfiles.map(profile => {
                const isActive = detailProfile?.id === profile.id && panelMode !== 'create';
                const isDefault = profile.id === payload.default_profile;
                return (
                  <button
                    key={profile.id}
                    type="button"
                    className={`ai-resource-profile-option ${isActive ? 'ai-resource-profile-option--active' : ''}`}
                    onClick={() => {
                      setSelectedProfileID(profile.id);
                      setDeleteBlocker(null);
                      setPanelMode(null);
                    }}
                  >
                    <span className="ai-resource-provider-glyph" aria-hidden="true">
                      <Bot className="h-4 w-4" />
                    </span>
                    <span className="ai-resource-profile-option__body">
                      <span className="ai-resource-profile-option__title">
                        {profile.display_name}
                        {isDefault && <span className="runner-pill runner-pill--ok">Default</span>}
                        <AIResourceTeamBadge resourceID={profile.id} />
                      </span>
                      <span className="ai-resource-profile-option__provider">{agentProfileSourceLabel(profile.source)}</span>
                      <span className="ai-resource-profile-option__model">{profile.role || profile.id}</span>
                    </span>
                    <span className={`ai-resource-status-dot ${profile.enabled ? 'ai-resource-status-dot--ok' : 'ai-resource-status-dot--muted'}`} aria-label={profile.enabled ? 'Enabled' : 'Disabled'} />
                  </button>
                );
              })}
              {!loading && visibleProfiles.length === 0 && (
                <AIResourceEmptyState>{payload.profiles.length === 0 ? 'No agent profiles configured.' : 'No agent profiles match your filters.'}</AIResourceEmptyState>
              )}
            </div>
          </div>

          <aside ref={panelRef} className="ai-resource-split__detail">
            {showForm ? (
              <AgentProfileForm
                canManage={canManage}
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
                defaultProfile={payload.default_profile}
                canManage={canManage}
                saving={saving}
                onDuplicate={openDuplicate}
                onEdit={startEdit}
                onToggleEnabled={toggleProfileEnabled}
                onDelete={deleteProfile}
                onUsage={openUsage}
                onSource={openSource}
              />
            ) : (
              <AIResourceEmptyState>Select an agent profile to inspect.</AIResourceEmptyState>
            )}
          </aside>
        </div>
      </section>
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
  const isCreate = !editingID;

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
        {isCreate && (
          <AIResourceTeamPlacementField
            teamPath={createTeamPath}
            onTeamPathChange={onCreateTeamPathChange}
            teamPaths={teamPaths}
            teamPathsLoading={teamPathsLoading}
            localName={aiResourceLocalName(form.id)}
            resourceLabel="Profile"
            disabled={!canManage}
          />
        )}
        <label className="flex flex-col gap-1 text-sm">
          <span>ID</span>
          <input
            data-agent-profile-autofocus
            className="pipelines-input"
            value={isCreate ? aiResourceLocalName(form.id) : form.id}
            onChange={event => isCreate ? onCreateLocalIDChange(event.target.value) : setForm(prev => ({ ...prev, id: event.target.value }))}
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

      <div className="ai-resource-detail__footer">
        {canEdit && (
          <button type="button" className="ai-resource-delete-link" onClick={() => void onDelete(profile.id)} disabled={saving || isDefault}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            Delete profile
          </button>
        )}
        {isDefault && <p>Default profiles cannot be deleted.</p>}
        {readOnly && !isDefault && <p>{sourceLabel} profiles are managed outside this page.</p>}
      </div>
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
