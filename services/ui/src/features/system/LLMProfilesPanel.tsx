import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Bot, CheckCircle2, Edit3, ExternalLink, KeyRound, Plus, RefreshCw, Sparkles, Trash2, X } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { LLM_PROVIDERS, getLLMProvider, replaceProviderDefault } from './llmProviders';
import { type LLMFeatureConfig, type LLMProfileFormState, type LLMProfileRecord } from './llm-profiles/model';
import { LLMFeatureControls } from './llm-profiles/LLMFeatureControls';
import { useLLMProfiles } from './llm-profiles/useLLMProfiles';
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
  aiResourceTeamScope,
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
import { ObjectIcon } from '../../components/ObjectIcon';
import ResourceAccessCard from '../../components/ResourceAccessCard';

function PlusIcon() {
  return <Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />;
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}

function RefreshIcon() {
  return <RefreshCw className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function OptionLabel({ children, help }: { children: string; help: string }) {
  return <span title={help}>{children}</span>;
}

function ProviderGlyph({ providerID }: { providerID: string }) {
  const Icon = providerID === 'gemini' ? Sparkles : Bot;
  return (
    <span className="ai-resource-provider-glyph" aria-hidden="true">
      <Icon className="h-4 w-4" strokeWidth={2} />
    </span>
  );
}

function profileThinkingText(profile: LLMProfileRecord) {
  const provider = getLLMProvider(profile.provider);
  if (provider.supportsThinking) {
    return profile.reasoning || (profile.thinking === undefined ? 'Provider default' : profile.thinking ? 'On' : 'Off');
  }
  return profile.reasoning || '-';
}

function profileTimeoutText(profile: LLMProfileRecord) {
  return profile.timeout_seconds > 0 ? `${profile.timeout_seconds}s` : 'Provider default';
}

function profileMaxTokensText(profile: LLMProfileRecord) {
  return profile.max_tokens > 0 ? `${profile.max_tokens} tokens` : 'Provider default';
}

function profileTemperatureText(profile: LLMProfileRecord) {
  return profile.temperature === undefined ? 'Provider default' : String(profile.temperature);
}

function profileFeatureModeText(feature?: LLMFeatureConfig) {
  const mode = feature?.mode?.trim().toLowerCase();
  if (mode === 'required') return 'Required';
  if (mode === 'disabled') return 'Disabled';
  return 'Auto';
}

function isProfileHealthy(profile: LLMProfileRecord) {
  return profile.status === 'valid';
}

function LLMProfilesPanel({ canManage }: { canManage: boolean }) {
  const profilePanelRef = useRef<HTMLElement | null>(null);
  const location = useLocation();
  const requestedTeamFilter = useMemo(() => aiResourceTeamFilterFromSearch(location.search), [location.search]);
  const [searchTerm, setSearchTerm] = useState('');
  const [teamFilter, setTeamFilter] = useState(requestedTeamFilter);
  const [createTeamPath, setCreateTeamPath] = useState('');
  const [selectedProfileName, setSelectedProfileName] = useState('');
  const { teamPaths, teamPathsLoading } = useAIResourceTeamPaths();
  const {
    payload,
    loading,
    error,
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
    startCreate,
    startEdit,
    saveProfile,
    saveDefaultProfile,
    deleteProfile,
    testProfile,
  } = useLLMProfiles({ canManage });

  useEffect(() => {
    setTeamFilter(requestedTeamFilter);
  }, [requestedTeamFilter]);

  useEffect(() => {
    if (!panelMode) return;
    window.requestAnimationFrame(() => {
      if (window.innerWidth < 1280) {
        profilePanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
      const focusTarget =
        profilePanelRef.current?.querySelector<HTMLElement>('[data-profile-autofocus]:not(:disabled)') ??
        profilePanelRef.current?.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), button:not(:disabled)');
      focusTarget?.focus({ preventScroll: true });
    });
  }, [editingName, panelMode]);

  const canDelete = (profile: LLMProfileRecord) => canManage && profile.name !== payload.default_profile;
  const migrationTargets = payload.profiles.filter(profile => profile.name !== deleteBlocker?.name).map(profile => profile.name);
  const showProfilePanel = panelMode !== null || deleteBlocker !== null;
  const showProfileForm = panelMode === 'create' || panelMode === 'edit';
  const formProvider = getLLMProvider(form.provider);
  const defaultProfileOptions = useMemo(
    () => payload.profiles.filter(profile => aiResourceTeamScope(profile.name).teamPath === ''),
    [payload.profiles]
  );
  const canManageGlobalDefault = canManage && defaultProfileOptions.length > 0;
  const teamFilterOptions = useMemo(
    () => collectAIResourceTeamPaths(payload.profiles.map(profile => profile.name), teamPaths),
    [payload.profiles, teamPaths]
  );
  const visibleProfiles = useMemo(
    () => payload.profiles.filter(profile => {
      const provider = getLLMProvider(profile.provider);
      if (!aiResourceMatchesTeamFilter(profile.name, teamFilter)) return false;
      return matchesAIResourceSearch(
        searchTerm,
        profile.name,
        aiResourceLocalName(profile.name),
        formatAIResourceTeamLabel(profile.name),
        provider.label,
        profile.provider,
        profile.model,
        profile.base_url,
        profile.credential_ref,
        profile.allowed_scopes.join(', '),
        profile.status,
        profile.validation,
        profile.disabled_reason
      );
    }),
    [payload.profiles, searchTerm, teamFilter]
  );
  const selectedProfile = useMemo(() => {
    if (!selectedProfileName) return null;
    return visibleProfiles.find(profile => profile.name === selectedProfileName) ?? null;
  }, [selectedProfileName, visibleProfiles]);
  const emptyProfilesMessage = payload.profiles.length === 0 ? 'No LLM profiles configured.' : 'No LLM profiles match your filters.';
  const filteredCountToken = searchTerm || (teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL ? teamFilter : '');
  const visibleHealthyCount = visibleProfiles.filter(isProfileHealthy).length;
  const visibleProviderCount = new Set(visibleProfiles.map(profile => getLLMProvider(profile.provider).id)).size;
  const visibleCredentialCount = visibleProfiles.filter(profile => Boolean(profile.credential_ref)).length;
  const workspaceResources = useMemo<AIResourceWorkspaceItem[]>(
    () => payload.profiles.map(profile => {
      const provider = getLLMProvider(profile.provider);
      return {
        id: profile.name,
        label: aiResourceLocalName(profile.name) || profile.name,
        description: `${provider.label} ${profile.model || ''}`.trim(),
      };
    }),
    [payload.profiles]
  );
  const selectedWorkspaceProfileName = showProfileForm ? null : selectedProfile?.name ?? null;
  const detailOpen = showProfilePanel || Boolean(selectedWorkspaceProfileName);
  const selectProfile = (profileName: string) => {
    setSelectedProfileName(profileName);
    setTeamFilter(aiResourceTreeFilterForResource(profileName));
    setDeleteBlocker(null);
    setPanelMode(null);
  };
  const openTeamFilter = (value: string) => {
    setTeamFilter(value);
    setSelectedProfileName('');
    setDeleteBlocker(null);
    setPanelMode(null);
  };
  const closeDetail = () => {
    setSelectedProfileName('');
    setDeleteBlocker(null);
    setPanelMode(null);
  };
  const openCreate = () => {
    const initialTeamPath = teamFilter !== AI_RESOURCE_TEAM_FILTER_ALL && teamFilter !== AI_RESOURCE_TEAM_FILTER_GLOBAL
      ? selectableAIResourceTeamPath(teamFilter, teamPaths)
      : '';
    setSelectedProfileName('');
    setCreateTeamPath(initialTeamPath);
    startCreate();
    setForm(prev => ({ ...prev, name: buildAIResourceScopedID(initialTeamPath, aiResourceLocalName(prev.name)) }));
  };
  const setCreateScopedName = (localName: string) => {
    setForm(prev => ({ ...prev, name: buildAIResourceScopedID(createTeamPath, localName) }));
  };
  const setCreateTeam = (teamPath: string) => {
    setCreateTeamPath(teamPath);
    setForm(prev => ({ ...prev, name: buildAIResourceScopedID(teamPath, aiResourceLocalName(prev.name)) }));
  };

  return (
    <div id="system-llm-profiles-section" className="ai-resource-panel ai-resource-page space-y-5 pb-24">
      <div className="ai-resource-page-header ai-resource-page-header--toolbar ai-resource-overview-bar">
        <h2 className="sr-only">LLM Profiles</h2>
        <div className="ai-resource-default-control">
          <span>Default profile</span>
          {canManageGlobalDefault ? (
            <select
              className="ai-resource-default-select"
              value={payload.default_profile}
              onChange={event => void saveDefaultProfile(event.target.value)}
              disabled={loading || saving}
              aria-label="Default LLM profile"
            >
              {defaultProfileOptions.map(profile => (
                <option key={profile.name} value={profile.name}>
                  {profile.name}
                </option>
              ))}
            </select>
          ) : (
            <strong>{payload.default_profile || '-'}</strong>
          )}
        </div>
        {!detailOpen && (
          <AIResourceMetricGrid
            metrics={[
              { label: 'Profiles', value: visibleProfiles.length, icon: <Bot className="h-4 w-4" /> },
              { label: 'Healthy', value: formatAIResourceRatio(visibleHealthyCount, visibleProfiles.length), icon: <CheckCircle2 className="h-4 w-4" />, tone: visibleProfiles.length === 0 || visibleHealthyCount === visibleProfiles.length ? 'ok' : 'warning' },
              { label: 'Providers', value: visibleProviderCount, icon: <Sparkles className="h-4 w-4" />, tone: 'info' },
              { label: 'Credentials', value: visibleCredentialCount, icon: <KeyRound className="h-4 w-4" />, tone: visibleCredentialCount > 0 ? 'muted' : 'warning' },
            ]}
          />
        )}
        <div className="ai-resource-page-actions">
          {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
          <button type="button" className="ai-resource-icon-button" onClick={() => void loadProfiles()} disabled={loading || saving} aria-label="Reload">
            <RefreshIcon />
          </button>
          {canManage && (
            <button type="button" className="ai-resource-primary-button" onClick={openCreate} disabled={saving}>
              <PlusIcon />
              New profile
            </button>
          )}
        </div>
      </div>
      {error && <div className="ai-resource-alert ai-resource-alert--error">{error}</div>}

      <AIResourceWorkspace
        storageKey="llm-profiles"
        workspaceLabel="LLM profile workspace"
        treeTitle="LLM profile tree"
        resourceType="llm-profile"
        resourceLabel="LLM profile"
        resources={workspaceResources}
        teamPaths={teamFilterOptions}
        teamFilter={teamFilter}
        selectedResourceID={selectedWorkspaceProfileName}
        onTeamFilterChange={openTeamFilter}
        onResourceSelect={selectProfile}
        onDetailClose={closeDetail}
        detailOpen={detailOpen}
        detailRef={profilePanelRef}
        detailLabel="LLM profile detail"
        listHeader={(
          <AIResourceTableHeader
            title="Profiles"
            count={formatFilteredCount(visibleProfiles.length, payload.profiles.length, filteredCountToken)}
            loading={loading}
            searchLabel="Search LLM profiles"
            searchPlaceholder="Search profiles..."
            searchValue={searchTerm}
            onSearchChange={setSearchTerm}
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
          <LLMProfileTable
            profiles={visibleProfiles}
            selectedProfileName={selectedWorkspaceProfileName}
            loading={loading}
            emptyMessage={emptyProfilesMessage}
            onSelectProfile={selectProfile}
          />
        )}
        detail={(
          <>
            {showProfileForm && (
              <>
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-xs text-[var(--text-secondary)]">{panelMode === 'edit' ? 'Edit profile' : 'Create profile'}</p>
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">{panelMode === 'edit' ? editingName : 'New LLM profile'}</h3>
                  </div>
                  <button type="button" className="glass-button-ghost !px-2" aria-label="Close profile form" onClick={() => setPanelMode(null)}>
                    <X className="h-4 w-4" aria-hidden="true" />
                  </button>
                </div>
                <form className="space-y-4" onSubmit={saveProfile}>
                  {panelMode === 'create' && (
                    <AIResourceTeamPlacementField
                      teamPath={createTeamPath}
                      onTeamPathChange={setCreateTeam}
                      teamPaths={teamPaths}
                      teamPathsLoading={teamPathsLoading}
                      localName={aiResourceLocalName(form.name)}
                      resourceLabel="Profile"
                      disabled={!canManage}
                    />
                  )}
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Name</span>
                    <input
                      data-profile-autofocus
                      className="pipelines-input"
                      value={panelMode === 'create' ? aiResourceLocalName(form.name) : form.name}
                      onChange={event => panelMode === 'create' ? setCreateScopedName(event.target.value) : setForm(prev => ({ ...prev, name: event.target.value }))}
                      disabled={!canManage || Boolean(editingName)}
                      placeholder="reasoning"
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Provider</span>
                    <select
                      className="pipelines-input"
                      value={form.provider}
                      onChange={event => setForm(prev => {
                        const previousProvider = getLLMProvider(prev.provider);
                        const nextProvider = getLLMProvider(event.target.value);
                        return {
                          ...prev,
                          provider: nextProvider.id,
                          model: replaceProviderDefault(prev.model, previousProvider.defaultModel, nextProvider.defaultModel),
                          base_url: replaceProviderDefault(prev.base_url, previousProvider.defaultBaseURL, nextProvider.defaultBaseURL),
                          credential_ref: replaceProviderDefault(prev.credential_ref, previousProvider.defaultCredentialRef, nextProvider.defaultCredentialRef),
                          reasoning: nextProvider.supportsReasoning ? prev.reasoning : '',
                          thinking: nextProvider.supportsThinking ? prev.thinking : 'default',
                        };
                      })}
                      disabled={!canManage}
                    >
                      {LLM_PROVIDERS.map(provider => <option key={provider.id} value={provider.id}>{provider.label}</option>)}
                    </select>
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Model</span>
                    <input className="pipelines-input" value={form.model} onChange={event => setForm(prev => ({ ...prev, model: event.target.value }))} disabled={!canManage} placeholder={formProvider.defaultModel} />
                  </label>
                  {formProvider.baseURLMode !== 'hidden' && (
                    <label className="flex flex-col gap-1 text-sm">
                      <span>Base URL{formProvider.baseURLMode === 'required' ? ' *' : ''}</span>
                      <input className="pipelines-input" value={form.base_url} onChange={event => setForm(prev => ({ ...prev, base_url: event.target.value }))} disabled={!canManage} placeholder={formProvider.defaultBaseURL || 'https://resource.openai.azure.com'} />
                    </label>
                  )}
                  {formProvider.apiKeyMode !== 'none' && (
                    <label className="flex flex-col gap-1 text-sm">
                      <span className="flex flex-wrap items-center gap-2">
                        <span>Credential reference{formProvider.apiKeyMode === 'required' ? ' *' : ''}</span>
                        <CredentialReferenceLink reference={form.credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
                          Open credential
                        </CredentialReferenceLink>
                      </span>
                      <input className="pipelines-input" value={form.credential_ref} onChange={event => setForm(prev => ({ ...prev, credential_ref: event.target.value }))} disabled={!canManage} placeholder={formProvider.defaultCredentialRef} />
                      <span className="text-xs text-[var(--text-secondary)]">Expected type: api_key</span>
                    </label>
                  )}
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Allowed scopes</span>
                    <input className="pipelines-input" value={form.allowed_scopes} onChange={event => setForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, internal" />
                  </label>
                  {formProvider.supportsReasoning && (
                    <label className="flex flex-col gap-1 text-sm">
                      <OptionLabel help="Controls how much internal reasoning the model performs before answering.">Reasoning</OptionLabel>
                      <select className="pipelines-input" value={form.reasoning} onChange={event => setForm(prev => ({ ...prev, reasoning: event.target.value }))} disabled={!canManage}>
                        <option value="">Provider default</option>
                        <option value="off">Off</option>
                        <option value="low">Low</option>
                        <option value="medium">Medium</option>
                        <option value="high">High</option>
                        <option value="on">On</option>
                      </select>
                    </label>
                  )}
                  {formProvider.supportsThinking && (
                    <label className="flex flex-col gap-1 text-sm">
                      <OptionLabel help="Turns the provider's extended thinking mode on or off when no reasoning level is selected.">Thinking</OptionLabel>
                      <select className="pipelines-input" value={form.thinking} onChange={event => setForm(prev => ({ ...prev, thinking: event.target.value as LLMProfileFormState['thinking'] }))} disabled={!canManage}>
                        <option value="default">Provider default</option>
                        <option value="true">True</option>
                        <option value="false">False</option>
                      </select>
                    </label>
                  )}
                  <div className="grid items-end gap-3 sm:grid-cols-3">
                    <label className="flex flex-col gap-1 text-sm">
                      <OptionLabel help="Maximum time to wait for the provider before the request is cancelled.">Timeout seconds</OptionLabel>
                      <input className="pipelines-input" type="number" min="0" value={form.timeout_seconds} onChange={event => setForm(prev => ({ ...prev, timeout_seconds: event.target.value }))} disabled={!canManage} placeholder="60" />
                    </label>
                    {formProvider.supportsMaxTokens && (
                      <label className="flex flex-col gap-1 text-sm">
                        <OptionLabel help="Maximum number of tokens the model may generate in its response.">Max tokens</OptionLabel>
                        <input className="pipelines-input" type="number" min="0" value={form.max_tokens} onChange={event => setForm(prev => ({ ...prev, max_tokens: event.target.value }))} disabled={!canManage} placeholder="2048" />
                      </label>
                    )}
                    {formProvider.supportsTemperature && (
                      <label className="flex flex-col gap-1 text-sm">
                        <OptionLabel help="Controls response randomness: lower values are more predictable, higher values are more varied.">Temperature</OptionLabel>
                        <input className="pipelines-input" type="number" min="0" max={formProvider.temperatureMax} step="0.1" value={form.temperature} onChange={event => setForm(prev => ({ ...prev, temperature: event.target.value }))} disabled={!canManage} placeholder="Provider default" />
                      </label>
                    )}
                  </div>
                  <p className="text-xs text-[var(--text-secondary)]">{formProvider.generationOptionsNote}</p>
                  <LLMFeatureControls form={form} setForm={setForm} disabled={!canManage} />
                  <label className="flex flex-col gap-1 text-sm">
                    <OptionLabel help="Additional provider-specific settings entered as one key=value pair per line.">Provider options</OptionLabel>
                    <textarea
                      className="pipelines-input min-h-24 font-mono text-xs"
                      value={form.extra}
                      onChange={event => setForm(prev => ({ ...prev, extra: event.target.value }))}
                      disabled={!canManage}
                      placeholder={form.provider === 'azure-openai' ? 'deployment=my-deployment\napi_version=2024-10-21' : form.provider === 'openrouter' ? 'http_referer=https://nopsai.example.com\nx_title=NopsAI' : 'key=value'}
                    />
                    <span className="text-xs text-[var(--text-secondary)]">One `key=value` option per line.</span>
                  </label>
                  <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>
                    {saving ? 'Saving…' : 'Save profile'}
                  </button>
                </form>
              </>
            )}

            {deleteBlocker && panelMode === 'delete' && (
              <div className="space-y-3 text-sm">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-xs text-[var(--text-secondary)]">Delete profile</p>
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">{deleteBlocker.name}</h3>
                  </div>
                  <button type="button" className="glass-button-ghost !px-2" aria-label="Close delete details" onClick={() => { setDeleteBlocker(null); setPanelMode(null); }}>
                    <X className="h-4 w-4" aria-hidden="true" />
                  </button>
                </div>
                <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 space-y-3">
                  <p className="font-semibold text-[var(--text-primary)]">Profile is still referenced.</p>
                  <ul className="list-disc pl-5 text-[var(--text-secondary)] max-h-32 overflow-auto">
                    {deleteBlocker.references.map(ref => <li key={ref}>{ref}</li>)}
                  </ul>
                  <label className="flex flex-col gap-1">
                    <span>Migrate references to</span>
                    <select data-profile-autofocus className="pipelines-input" value={deleteBlocker.migrateTo} onChange={event => setDeleteBlocker(prev => prev ? { ...prev, migrateTo: event.target.value } : prev)}>
                      {migrationTargets.map(name => <option key={name} value={name}>{name}</option>)}
                    </select>
                  </label>
                  <button type="button" className="glass-button-danger" disabled={!deleteBlocker.migrateTo || saving} onClick={() => void deleteProfile(deleteBlocker.name, { force: true, migrateTo: deleteBlocker.migrateTo })}>
                    <TrashIcon />
                    Force delete with migration
                  </button>
                </div>
              </div>
            )}
            {!showProfilePanel && selectedProfile && (
              <LLMProfileDetail
                profile={selectedProfile}
                isDefault={selectedProfile.name === payload.default_profile}
                canDelete={canDelete(selectedProfile)}
                canManage={canManage}
                saving={saving}
                testing={testing}
                testResult={testResult}
                onEdit={() => startEdit(selectedProfile)}
                onDelete={() => void deleteProfile(selectedProfile.name)}
                onTest={() => void testProfile(selectedProfile.name)}
              />
            )}
            {!showProfilePanel && !selectedProfile && (
              <AIResourceEmptyState>{emptyProfilesMessage}</AIResourceEmptyState>
            )}
          </>
        )}
      />
    </div>
  );
}

function LLMProfileTable({
  profiles,
  selectedProfileName,
  loading,
  emptyMessage,
  onSelectProfile,
}: {
  profiles: LLMProfileRecord[];
  selectedProfileName: string | null;
  loading: boolean;
  emptyMessage: string;
  onSelectProfile: (profileName: string) => void;
}) {
  if (!loading && profiles.length === 0) {
    return <AIResourceEmptyState>{emptyMessage}</AIResourceEmptyState>;
  }

  return (
    <div className="ai-resource-table-shell">
      <table className="ai-resource-registry-table" aria-label="LLM profiles">
        <colgroup>
          <col style={{ width: '28%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '16%' }} />
          <col style={{ width: '22%' }} />
          <col style={{ width: '10%' }} />
          <col style={{ width: '10%' }} />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">Profile</th>
            <th scope="col">Team</th>
            <th scope="col">Provider</th>
            <th scope="col">Model</th>
            <th scope="col">Scopes</th>
            <th scope="col">Status</th>
          </tr>
        </thead>
        <tbody>
          {profiles.map(profile => {
            const provider = getLLMProvider(profile.provider);
            const selected = selectedProfileName === profile.name;
            const healthy = isProfileHealthy(profile);
            return (
              <tr key={profile.name} className={selected ? 'selected' : ''} onClick={() => onSelectProfile(profile.name)}>
                <td>
                  <button
                    type="button"
                    className="ai-resource-table-resource"
                    aria-label={`Select LLM profile ${aiResourceLocalName(profile.name) || profile.name}`}
                    onClick={event => {
                      event.stopPropagation();
                      onSelectProfile(profile.name);
                    }}
                  >
                    <span className="ai-resource-table-resource-icon" aria-hidden="true">
                      <ObjectIcon type="llm-profile" />
                    </span>
                    <span className="ai-resource-table-resource-name">
                      <strong>{profile.name}</strong>
                    </span>
                  </button>
                </td>
                <td><AIResourceTeamBadge resourceID={profile.name} /></td>
                <td>{provider.label}</td>
                <td><span className="ai-resource-table-mono">{profile.model || '-'}</span></td>
                <td>{profile.allowed_scopes.length ? profile.allowed_scopes.join(', ') : 'All scopes'}</td>
                <td>
                  <span className={`ai-resource-health ${healthy ? 'ai-resource-health--ok' : 'ai-resource-health--error'}`}>
                    <span aria-hidden="true" />
                    {healthy ? 'Healthy' : profile.status || 'Needs attention'}
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

function LLMProfileDetail({
  profile,
  isDefault,
  canDelete,
  canManage,
  saving,
  testing,
  testResult,
  onEdit,
  onDelete,
  onTest,
}: {
  profile: LLMProfileRecord;
  isDefault: boolean;
  canDelete: boolean;
  canManage: boolean;
  saving: boolean;
  testing: string | null;
  testResult: string | null;
  onEdit: () => void;
  onDelete: () => void;
  onTest: () => void;
}) {
  const provider = getLLMProvider(profile.provider);
  const healthy = isProfileHealthy(profile);
  const scopedTestResult = testResult?.startsWith(`${profile.name}:`) ? testResult : null;

  return (
    <div className="ai-resource-detail">
      <div className="ai-resource-detail__header">
        <div>
          <div className="ai-resource-detail__title">
            <h3>{profile.name}</h3>
            {isDefault && <span className="runner-pill runner-pill--ok">Default</span>}
            <span className={`ai-resource-health ${healthy ? 'ai-resource-health--ok' : 'ai-resource-health--error'}`}>
              <span aria-hidden="true" />
              {healthy ? 'Healthy' : profile.status || 'Needs attention'}
            </span>
          </div>
          <div className="ai-resource-detail__provider">
            <ProviderGlyph providerID={provider.id} />
            {provider.label}
          </div>
        </div>
        <div className="ai-resource-detail__actions">
          <AIResourceIconAction label={testing === profile.name ? 'Testing connection' : 'Test connection'} tone="primary" onClick={onTest} disabled={Boolean(testing)}>
            {testing === profile.name ? <RefreshIcon /> : <CheckCircle2 className="h-4 w-4" aria-hidden="true" />}
          </AIResourceIconAction>
          <AIResourceIconAction label="Edit profile" tone="accent" onClick={onEdit} disabled={!canManage || saving}>
            <Edit3 className="h-4 w-4" aria-hidden="true" />
          </AIResourceIconAction>
          <ResourceAccessCard
            resourceType="llm_profile"
            resourceID={profile.name}
            label="LLM profile"
            buttonClassName="ai-resource-icon-action"
            iconOnly
          />
        </div>
      </div>

      <LLMDetailSection
        title="Connection"
        rows={[
          { label: 'Model', value: profile.model || '-' },
          { label: 'Base URL', value: profile.base_url || '-', mono: true },
          {
            label: 'Credential',
            value: profile.credential_ref ? (
              <span className="ai-resource-detail-link">
                <CredentialReferenceLink reference={profile.credential_ref} className="ai-resource-ref-link underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]" />
                <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
              </span>
            ) : '-',
            mono: true,
          },
        ]}
      />
      <LLMDetailSection
        title="Runtime behavior"
        rows={[
          { label: 'Thinking', value: profileThinkingText(profile) },
          { label: 'Timeout', value: profileTimeoutText(profile) },
          { label: 'Max tokens', value: profileMaxTokensText(profile) },
          { label: 'Temperature', value: profileTemperatureText(profile) },
          { label: 'Prompt cache', value: profileFeatureModeText(profile.prompt_cache) },
          { label: 'Provider state', value: profileFeatureModeText(profile.provider_state) },
        ]}
      />
      <LLMDetailSection
        title="Access"
        rows={[
          { label: 'Team', value: <AIResourceTeamBadge resourceID={profile.name} /> },
          { label: 'Allowed scopes', value: profile.allowed_scopes.length ? profile.allowed_scopes.join(', ') : 'All scopes' },
        ]}
      />
      <LLMDetailSection
        title="Recent test"
        rows={[
          {
            label: '',
            value: (
              <span className={`ai-resource-test-result ${healthy ? 'ai-resource-test-result--ok' : 'ai-resource-test-result--error'}`}>
                <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                {scopedTestResult || profile.validation || profile.disabled_reason || (healthy ? 'Connection ready' : 'Test recommended')}
              </span>
            ),
          },
        ]}
      />

      <div className="ai-resource-detail__footer">
        <button type="button" className="ai-resource-delete-link" onClick={onDelete} disabled={!canDelete || saving}>
          <TrashIcon />
          Delete profile
        </button>
        {isDefault && <p>Default profiles cannot be deleted.</p>}
      </div>
    </div>
  );
}

function LLMDetailSection({ title, rows }: { title: string; rows: Array<{ label: string; value: ReactNode; mono?: boolean }> }) {
  return (
    <section className="ai-resource-detail-section">
      <h4>{title}</h4>
      <dl>
        {rows.map(row => (
          <div key={`${title}-${row.label || 'result'}`} className={`ai-resource-detail-row ${row.label ? '' : 'ai-resource-detail-row--full'}`}>
            {row.label && <dt>{row.label}</dt>}
            <dd className={row.mono ? 'ai-resource-detail-row__mono' : undefined}>{row.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export default LLMProfilesPanel;
