import { useEffect, useRef } from 'react';
import { Edit3, Plus, RefreshCw, Trash2, X } from 'lucide-react';
import { LLM_PROVIDERS, getLLMProvider, replaceProviderDefault } from './llmProviders';
import { type LLMProfileFormState, type LLMProfileRecord } from './llm-profiles/model';
import { useLLMProfiles } from './llm-profiles/useLLMProfiles';

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

function OptionLabel({ children, help }: { children: string; help: string }) {
  return <span title={help}>{children}</span>;
}

function LLMProfilesPanel({ canManage }: { canManage: boolean }) {
  const profilePanelRef = useRef<HTMLElement | null>(null);
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

  return (
    <div id="system-llm-profiles-section" className="space-y-6 pb-24">
      <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <label className="flex flex-col gap-1 text-sm min-w-[220px]">
            <span>Default profile</span>
            <select
              className="pipelines-input"
              value={payload.default_profile}
              onChange={event => void saveDefaultProfile(event.target.value)}
              disabled={!canManage || loading || saving}
            >
              {payload.profiles.map(profile => (
                <option key={profile.name} value={profile.name}>
                  {profile.name}
                </option>
              ))}
            </select>
          </label>
          <div className="flex items-center gap-2">
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="glass-button-ghost" onClick={() => void loadProfiles()} disabled={loading || saving}>
              <RefreshIcon />
              Reload
            </button>
            {canManage && (
              <button type="button" className="glass-button-primary" onClick={startCreate} disabled={saving}>
                <PlusIcon />
                New profile
              </button>
            )}
          </div>
        </div>
        {error && <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">{error}</div>}
        {testResult && <div className="rounded-lg border border-[var(--border-primary)] px-4 py-3 text-sm text-[var(--text-secondary)]">{testResult}</div>}
      </section>

      <div className={`grid gap-6 ${showProfilePanel ? 'xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]' : ''}`}>
        <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
          <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between gap-3">
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Profiles</h3>
            {loading && <span className="text-sm text-[var(--text-secondary)]">Loading…</span>}
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
                <tr>
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Provider</th>
                  <th className="px-4 py-3">Model</th>
                  <th className="px-4 py-3">Base URL</th>
                  <th className="px-4 py-3">API key secret</th>
                  <th className="px-4 py-3">Allowed scopes</th>
                  <th className="px-4 py-3">Limits</th>
                  <th className="px-4 py-3">Thinking</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {payload.profiles.map(profile => (
                  <tr key={profile.name} className="border-b border-[var(--border-primary)] last:border-b-0">
                    <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">
                      {profile.name}
                      {profile.name === payload.default_profile && <span className="ml-2 runner-pill runner-pill--ok">Default</span>}
                    </td>
                    <td className="px-4 py-3">{getLLMProvider(profile.provider).label}</td>
                    <td className="px-4 py-3 max-w-[220px] truncate" title={profile.model}>{profile.model || '-'}</td>
                    <td className="px-4 py-3 max-w-[220px] truncate" title={profile.base_url}>{profile.base_url || '-'}</td>
                    <td className="px-4 py-3">{profile.api_key_secret || '-'}</td>
                    <td className="px-4 py-3">{profile.allowed_scopes.length ? profile.allowed_scopes.join(', ') : 'All'}</td>
                    <td className="px-4 py-3 text-xs text-[var(--text-secondary)]">
                      {profile.timeout_seconds > 0 ? `${profile.timeout_seconds}s` : 'Default'}
                      {profile.max_tokens > 0 ? ` / ${profile.max_tokens} tokens` : ''}
                    </td>
                    <td className="px-4 py-3">
                      {getLLMProvider(profile.provider).supportsThinking
                        ? profile.reasoning || (profile.thinking === undefined ? 'Default' : profile.thinking ? 'On' : 'Off')
                        : '-'}
                    </td>
                    <td className="px-4 py-3">
                      <div className="space-y-1">
                        <span className={`runner-pill ${profile.status === 'valid' ? 'runner-pill--ok' : 'runner-pill--error'}`} title={profile.validation || profile.disabled_reason || ''}>
                          {profile.status || 'unknown'}
                        </span>
                        {(profile.validation || profile.disabled_reason) && (
                          <p className="text-xs text-[var(--text-secondary)] max-w-[220px]">{profile.validation || profile.disabled_reason}</p>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <button type="button" className="glass-button-ghost" onClick={() => void testProfile(profile.name)} disabled={Boolean(testing)}>
                          {testing === profile.name ? 'Testing…' : 'Test'}
                        </button>
                        <button type="button" className="glass-button-subtle" onClick={() => startEdit(profile)}>
                          <EditIcon />
                          Edit
                        </button>
                        <button type="button" className="glass-button-danger" onClick={() => void deleteProfile(profile.name)} disabled={!canDelete(profile) || saving}>
                          <TrashIcon />
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
                {!loading && payload.profiles.length === 0 && (
                  <tr>
                    <td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={10}>
                      No LLM profiles configured.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        {showProfilePanel && (
          <aside ref={profilePanelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
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
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Name</span>
                    <input data-profile-autofocus className="pipelines-input" value={form.name} onChange={event => setForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingName)} placeholder="reasoning" />
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
                          api_key_secret: replaceProviderDefault(prev.api_key_secret, previousProvider.defaultSecretName, nextProvider.defaultSecretName),
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
                      <span>API key secret{formProvider.apiKeyMode === 'required' ? ' *' : ''}</span>
                      <input className="pipelines-input" value={form.api_key_secret} onChange={event => setForm(prev => ({ ...prev, api_key_secret: event.target.value }))} disabled={!canManage} placeholder={formProvider.defaultSecretName} />
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
          </aside>
        )}
      </div>
    </div>
  );
}


export default LLMProfilesPanel;
