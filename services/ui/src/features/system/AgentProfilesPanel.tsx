import { useEffect, useMemo, useRef } from 'react';
import { Copy, Edit3, Eye, FileText, Plus, RefreshCw, Trash2, X } from 'lucide-react';
import {
  agentProfileSection,
  agentProfileSourceLabel,
  type AgentProfileRecord,
} from './agent-profiles/model';
import { useAgentProfiles } from './agent-profiles/useAgentProfiles';

function AgentProfilesPanel({ canManage }: { canManage: boolean }) {
  const panelRef = useRef<HTMLElement | null>(null);
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
  } = useAgentProfiles({ canManage });

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

  const sections = useMemo(() => {
    const teamed = {
      builtIn: [] as AgentProfileRecord[],
      custom: [] as AgentProfileRecord[],
      gitops: [] as AgentProfileRecord[],
    };
    payload.profiles.forEach(profile => {
      const section = agentProfileSection(profile);
      if (section === 'built-in') teamed.builtIn.push(profile);
      else if (section === 'gitops') teamed.gitops.push(profile);
      else teamed.custom.push(profile);
    });
    return teamed;
  }, [payload.profiles]);

  const showSidePanel = panelMode !== null;
  const showForm = panelMode === 'create' || panelMode === 'edit';
  const selectedSource = selectedProfile ? agentProfileSourceLabel(selectedProfile.source) : '';
  const defaultProfileRecord = payload.profiles.find(profile => profile.id === payload.default_profile) || null;
  const defaultProfileOptions = useMemo(() => {
    const options = payload.profiles.filter(profile => profile.enabled);
    if (defaultProfileRecord && !options.some(profile => profile.id === defaultProfileRecord.id)) {
      options.push(defaultProfileRecord);
    }
    return options;
  }, [defaultProfileRecord, payload.profiles]);

  return (
    <div id="system-agent-profiles-section" className="space-y-6 pb-24">
      <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)]">Default resolution</p>
            <h2 className="text-xl font-semibold text-[var(--text-primary)]">{defaultProfileRecord?.display_name || payload.default_profile}</h2>
            <p className="text-sm text-[var(--text-secondary)]">{payload.default_profile}</p>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex flex-col gap-1 text-sm min-w-64">
              <span className="text-xs text-[var(--text-secondary)]">Default profile</span>
              <select
                className="pipelines-input"
                value={payload.default_profile}
                onChange={event => void setDefaultProfile(event.target.value)}
                disabled={!canManage || loading || saving || defaultProfileOptions.length === 0}
              >
                {defaultProfileOptions.map(profile => (
                  <option key={profile.id} value={profile.id}>
                    {profile.display_name} ({profile.id})
                  </option>
                ))}
              </select>
            </label>
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="glass-button-ghost" onClick={() => void loadProfiles()} disabled={loading || saving}>
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              Reload
            </button>
            {canManage && (
              <button type="button" className="glass-button-primary" onClick={startCreate} disabled={saving}>
                <Plus className="h-4 w-4" aria-hidden="true" />
                New profile
              </button>
            )}
          </div>
        </div>
        {error && <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">{error}</div>}
      </section>

      <div className={`grid gap-6 ${showSidePanel ? 'xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]' : ''}`}>
        <div className="space-y-6">
          <ProfileTable
            title="Built-in Profiles"
            profiles={sections.builtIn}
            loading={loading}
            canManage={canManage}
            saving={saving}
            onView={openView}
            onDuplicate={startDuplicate}
            onEdit={startEdit}
            onToggleEnabled={toggleProfileEnabled}
            onDelete={deleteProfile}
            onUsage={openUsage}
            onSource={openSource}
            defaultProfile={payload.default_profile}
          />
          <ProfileTable
            title="Custom Profiles"
            profiles={sections.custom}
            loading={loading}
            canManage={canManage}
            saving={saving}
            onView={openView}
            onDuplicate={startDuplicate}
            onEdit={startEdit}
            onToggleEnabled={toggleProfileEnabled}
            onDelete={deleteProfile}
            onUsage={openUsage}
            onSource={openSource}
            defaultProfile={payload.default_profile}
          />
          <ProfileTable
            title="GitOps Managed Profiles"
            profiles={sections.gitops}
            loading={loading}
            canManage={canManage}
            saving={saving}
            onView={openView}
            onDuplicate={startDuplicate}
            onEdit={startEdit}
            onToggleEnabled={toggleProfileEnabled}
            onDelete={deleteProfile}
            onUsage={openUsage}
            onSource={openSource}
            defaultProfile={payload.default_profile}
          />
        </div>

        {showSidePanel && (
          <aside ref={panelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-xs text-[var(--text-secondary)]">
                  {showForm ? (panelMode === 'edit' ? 'Edit profile' : 'Create profile') : selectedSource || 'Agent profile'}
                </p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">
                  {showForm ? (editingID || 'New agent profile') : selectedProfile?.display_name || deleteBlocker?.id}
                </h3>
              </div>
              <button type="button" className="glass-button-ghost !px-2" aria-label="Close agent profile panel" onClick={() => { setDeleteBlocker(null); setPanelMode(null); }}>
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>

            {showForm && (
              <form className="space-y-4" onSubmit={saveProfile}>
                <label className="flex flex-col gap-1 text-sm">
                  <span>ID</span>
                  <input data-agent-profile-autofocus className="pipelines-input" value={form.id} onChange={event => setForm(prev => ({ ...prev, id: event.target.value }))} disabled={!canManage || Boolean(editingID)} placeholder="security-reviewer" />
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
            )}

            {panelMode === 'view' && selectedProfile && (
              <ProfileReadout profile={selectedProfile} />
            )}

            {panelMode === 'usage' && selectedProfile && (
              <UsageReadout profile={selectedProfile} />
            )}

            {panelMode === 'source' && selectedProfile && (
              <SourceReadout profile={selectedProfile} />
            )}

            {panelMode === 'delete' && deleteBlocker && (
              <div className="space-y-3 text-sm">
                <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 space-y-3">
                  <p className="font-semibold text-[var(--text-primary)]">Profile is still referenced.</p>
                  <ul className="list-disc pl-5 text-[var(--text-secondary)] max-h-36 overflow-auto">
                    {deleteBlocker.references.map(ref => <li key={ref}>{ref}</li>)}
                  </ul>
                  <button type="button" className="glass-button-danger" disabled={saving} onClick={() => void deleteProfile(deleteBlocker.id, { force: true })}>
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    Force delete
                  </button>
                </div>
              </div>
            )}
          </aside>
        )}
      </div>
    </div>
  );
}

function ProfileTable({
  title,
  profiles,
  loading,
  canManage,
  saving,
  onView,
  onDuplicate,
  onEdit,
  onToggleEnabled,
  onDelete,
  onUsage,
  onSource,
  defaultProfile,
}: {
  title: string;
  profiles: AgentProfileRecord[];
  loading: boolean;
  canManage: boolean;
  saving: boolean;
  onView: (profile: AgentProfileRecord) => void;
  onDuplicate: (profile: AgentProfileRecord) => void;
  onEdit: (profile: AgentProfileRecord) => void;
  onToggleEnabled: (profile: AgentProfileRecord) => void | Promise<void>;
  onDelete: (id: string) => void | Promise<void>;
  onUsage: (profile: AgentProfileRecord) => void;
  onSource: (profile: AgentProfileRecord) => void;
  defaultProfile: string;
}) {
  return (
    <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
      <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between gap-3">
        <h3 className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
        {loading && <span className="text-sm text-[var(--text-secondary)]">Loading...</span>}
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
            <tr>
              <th className="px-4 py-3">Name</th>
              <th className="px-4 py-3">Prompt role</th>
              <th className="px-4 py-3">Description</th>
              <th className="px-4 py-3">Source</th>
              <th className="px-4 py-3">Enabled</th>
              <th className="px-4 py-3">Used by</th>
              <th className="px-4 py-3">Last updated</th>
              <th className="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {profiles.map(profile => {
              const readOnly = Boolean(profile.read_only || profile.built_in || profile.source === 'gitops');
              const canEdit = canManage && !readOnly;
              const isDefault = profile.id === defaultProfile;
              return (
                <tr key={profile.id} className="border-b border-[var(--border-primary)] last:border-b-0">
                  <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">
                    {profile.display_name}
                    <span className="ml-2 text-xs text-[var(--text-secondary)]">{profile.id}</span>
                    {isDefault && <span className="ml-2 runner-pill runner-pill--ok">Default</span>}
                  </td>
                  <td className="px-4 py-3">{profile.role || <span className="text-[var(--text-secondary)]">Uses name</span>}</td>
                  <td className="px-4 py-3 max-w-[280px] truncate" title={profile.description}>{profile.description || '-'}</td>
                  <td className="px-4 py-3">{agentProfileSourceLabel(profile.source)}</td>
                  <td className="px-4 py-3"><span className={`runner-pill ${profile.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>{profile.enabled ? 'Enabled' : 'Disabled'}</span></td>
                  <td className="px-4 py-3">{profile.usage_count}</td>
                  <td className="px-4 py-3">{profile.last_updated ? new Date(profile.last_updated).toLocaleString() : '-'}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-2">
                      <button type="button" className="glass-button-ghost" onClick={() => onView(profile)}>
                        <Eye className="h-4 w-4" aria-hidden="true" />
                        View
                      </button>
                      <button type="button" className="glass-button-subtle" onClick={() => onDuplicate(profile)} disabled={!canManage || saving}>
                        <Copy className="h-4 w-4" aria-hidden="true" />
                        Duplicate
                      </button>
                      {profile.source === 'gitops' ? (
                        <button type="button" className="glass-button-subtle" onClick={() => onSource(profile)}>
                          <FileText className="h-4 w-4" aria-hidden="true" />
                          Source
                        </button>
                      ) : canEdit ? (
                        <>
                          <button type="button" className="glass-button-subtle" onClick={() => onEdit(profile)}>
                            <Edit3 className="h-4 w-4" aria-hidden="true" />
                            Edit
                          </button>
                          <button type="button" className="glass-button-ghost" onClick={() => void onToggleEnabled(profile)} disabled={saving || isDefault}>
                            {profile.enabled ? 'Disable' : 'Enable'}
                          </button>
                          <button type="button" className="glass-button-danger" onClick={() => void onDelete(profile.id)} disabled={saving || isDefault}>
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                            Delete
                          </button>
                        </>
                      ) : null}
                      {profile.usage_count > 0 && (
                        <button type="button" className="glass-button-ghost" onClick={() => onUsage(profile)}>
                          Usage
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
            {!loading && profiles.length === 0 && (
              <tr>
                <td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={8}>No profiles in this section.</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ProfileReadout({ profile }: { profile: AgentProfileRecord }) {
  return (
    <div className="space-y-3 text-sm">
      <dl className="grid grid-cols-[7rem_1fr] gap-x-3 gap-y-2">
        <dt className="text-[var(--text-secondary)]">ID</dt>
        <dd>{profile.id}</dd>
        <dt className="text-[var(--text-secondary)]">Prompt role</dt>
        <dd>{profile.role || 'Uses profile name'}</dd>
        <dt className="text-[var(--text-secondary)]">Source</dt>
        <dd>{agentProfileSourceLabel(profile.source)}</dd>
        <dt className="text-[var(--text-secondary)]">Enabled</dt>
        <dd>{profile.enabled ? 'Yes' : 'No'}</dd>
      </dl>
      <p className="text-[var(--text-secondary)]">{profile.description || 'No description provided.'}</p>
      <pre className="max-h-80 overflow-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-xs whitespace-pre-wrap">{profile.instructions}</pre>
    </div>
  );
}

function UsageReadout({ profile }: { profile: AgentProfileRecord }) {
  return (
    <div className="space-y-3 text-sm">
      <p className="text-[var(--text-secondary)]">{profile.usage_count} explicit pipeline or step reference{profile.usage_count === 1 ? '' : 's'}.</p>
      <ul className="list-disc pl-5 text-[var(--text-secondary)] max-h-64 overflow-auto">
        {profile.references.map(ref => <li key={ref}>{ref}</li>)}
      </ul>
    </div>
  );
}

function SourceReadout({ profile }: { profile: AgentProfileRecord }) {
  const roleLine = profile.role ? `    role: ${profile.role}\n` : '';
  const source = `agent_profiles:\n  - id: ${profile.id}\n    display_name: ${profile.display_name}\n${roleLine}    enabled: ${profile.enabled}\n    description: ${profile.description || ''}\n    instructions: |\n${profile.instructions.split('\n').map(line => `      ${line}`).join('\n')}`;
  return (
    <div className="space-y-3 text-sm">
      <p className="text-[var(--text-secondary)]">{profile.source_path || 'GitOps source path unavailable.'}</p>
      <pre className="max-h-96 overflow-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-xs whitespace-pre-wrap">{source}</pre>
    </div>
  );
}

export default AgentProfilesPanel;
