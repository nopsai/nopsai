import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { Edit3, Plus, RefreshCw, Trash2, X } from 'lucide-react';
import { buildApiUrl } from '../../lib/api';
import { asRecord, normalizeStringArray, readOptionalString, readString } from './data';

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

type LLMProfileRecord = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  api_key_secret: string;
  allowed_scopes: string[];
  reasoning: string;
  thinking?: boolean;
  status: string;
  validation?: string;
  references?: string[];
  allowed_in_scope?: boolean;
  disabled_reason?: string;
};

type LLMProfilesPayload = {
  default_profile: string;
  profiles: LLMProfileRecord[];
};

type LLMProfileFormState = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  api_key_secret: string;
  allowed_scopes: string;
  reasoning: string;
  thinking: 'default' | 'true' | 'false';
};

type LLMProfilePanelMode = 'create' | 'edit' | 'delete';

const emptyLLMProfileForm: LLMProfileFormState = {
  name: '',
  provider: 'gemini',
  model: '',
  base_url: '',
  api_key_secret: 'GEMINI_API_KEY',
  allowed_scopes: '',
  reasoning: '',
  thinking: 'default',
};

function LLMProfilesPanel({ canManage }: { canManage: boolean }) {
  const [payload, setPayload] = useState<LLMProfilesPayload>({ default_profile: 'standard', profiles: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [form, setForm] = useState<LLMProfileFormState>(emptyLLMProfileForm);
  const [deleteBlocker, setDeleteBlocker] = useState<{ name: string; references: string[]; migrateTo: string } | null>(null);
  const [panelMode, setPanelMode] = useState<LLMProfilePanelMode | null>(null);
  const profilePanelRef = useRef<HTMLElement | null>(null);

  const loadProfiles = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/system/llm-profiles'), { cache: 'no-store' });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load LLM profiles (${response.status})`);
      }
      setPayload(normalizeLLMProfilesPayload(await response.json()));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load LLM profiles');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadProfiles();
  }, [loadProfiles]);

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

  const startCreate = () => {
    setEditingName(null);
    setForm(emptyLLMProfileForm);
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('create');
  };

  const startEdit = (profile: LLMProfileRecord) => {
    setEditingName(profile.name);
    setForm({
      name: profile.name,
      provider: profile.provider || 'gemini',
      model: profile.model || '',
      base_url: profile.base_url || '',
      api_key_secret: profile.api_key_secret || '',
      allowed_scopes: (profile.allowed_scopes || []).join(', '),
      reasoning: profile.reasoning || '',
      thinking: profile.thinking === undefined ? 'default' : profile.thinking ? 'true' : 'false',
    });
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('edit');
  };

  const formToPayload = () => ({
    name: form.name.trim(),
    provider: form.provider.trim(),
    model: form.model.trim(),
    base_url: form.base_url.trim(),
    api_key_secret: form.api_key_secret.trim(),
    allowed_scopes: form.allowed_scopes.split(',').map(item => item.trim()).filter(Boolean),
    reasoning: form.reasoning.trim(),
    thinking: form.provider.trim() === 'lmstudio' && form.thinking !== 'default' ? form.thinking === 'true' : undefined,
  });

  const saveProfile = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    const next = formToPayload();
    if (!next.name) {
      setError('Profile name is required.');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/llm-profiles/${encodeURIComponent(next.name)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(next),
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to save LLM profile (${response.status})`);
      }
      setPayload(normalizeLLMProfilesPayload(await response.json()));
      setEditingName(next.name);
      setPanelMode('edit');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save LLM profile');
    } finally {
      setSaving(false);
    }
  };

  const saveDefaultProfile = async (nextDefault: string) => {
    if (!canManage || !nextDefault) return;
    setSaving(true);
    setError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/system/llm-profiles'), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          default_profile: nextDefault,
          profiles: payload.profiles,
        }),
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to update default profile (${response.status})`);
      }
      setPayload(normalizeLLMProfilesPayload(await response.json()));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to update default profile');
    } finally {
      setSaving(false);
    }
  };

  const deleteProfile = async (name: string, opts?: { force?: boolean; migrateTo?: string }) => {
    if (!canManage) return;
    setSaving(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (opts?.force) params.set('force', 'true');
      if (opts?.migrateTo) params.set('migrate_to', opts.migrateTo);
      const suffix = params.toString() ? `?${params.toString()}` : '';
      const response = await fetch(buildApiUrl(`/v1/system/llm-profiles/${encodeURIComponent(name)}${suffix}`), { method: 'DELETE' });
      if (response.status === 409) {
        const conflict = await response.json().catch(() => null);
        const references = Array.isArray(conflict?.references) ? conflict.references.map((item: unknown) => String(item)) : [];
        const fallback = payload.profiles.find(profile => profile.name !== name)?.name || '';
        setDeleteBlocker({ name, references, migrateTo: fallback });
        setPanelMode('delete');
        return;
      }
      if (!response.ok && response.status !== 204) {
        const text = await response.text();
        throw new Error(text || `Failed to delete LLM profile (${response.status})`);
      }
      setDeleteBlocker(null);
      if (editingName === name) {
        setEditingName(null);
        setForm(emptyLLMProfileForm);
      }
      setPanelMode(prev => (prev === 'delete' || editingName === name ? null : prev));
      await loadProfiles();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete LLM profile');
    } finally {
      setSaving(false);
    }
  };

  const testProfile = async (name: string) => {
    setTesting(name);
    setTestResult(null);
    setError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/llm-profiles/${encodeURIComponent(name)}/test`), { method: 'POST' });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Profile test failed (${response.status})`);
      }
      const result = await response.json();
      setTestResult(`${name}: ${readString(result?.reply) || 'ok'}`);
    } catch (err) {
      setTestResult(`${name}: ${err instanceof Error ? err.message : 'test failed'}`);
    } finally {
      setTesting(null);
    }
  };

  const providerOptions = ['gemini', 'lmstudio'];
  const canDelete = (profile: LLMProfileRecord) => canManage && profile.name !== payload.default_profile;
  const migrationTargets = payload.profiles.filter(profile => profile.name !== deleteBlocker?.name).map(profile => profile.name);
  const showProfilePanel = panelMode !== null || deleteBlocker !== null;
  const showProfileForm = panelMode === 'create' || panelMode === 'edit';

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
                    <td className="px-4 py-3">{profile.provider || '-'}</td>
                    <td className="px-4 py-3 max-w-[220px] truncate" title={profile.model}>{profile.model || '-'}</td>
                    <td className="px-4 py-3 max-w-[220px] truncate" title={profile.base_url}>{profile.base_url || '-'}</td>
                    <td className="px-4 py-3">{profile.api_key_secret || '-'}</td>
                    <td className="px-4 py-3">{profile.allowed_scopes.length ? profile.allowed_scopes.join(', ') : 'All'}</td>
                    <td className="px-4 py-3">
                      {profile.reasoning || (() => {
                        if (profile.thinking === undefined) return 'Default';
                        return profile.thinking ? 'On' : 'Off';
                      })()}
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
                    <td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={9}>
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
                      onChange={event => setForm(prev => ({
                        ...prev,
                        provider: event.target.value,
                        api_key_secret: event.target.value === 'gemini' && !prev.api_key_secret ? 'GEMINI_API_KEY' : prev.api_key_secret,
                        thinking: event.target.value === 'lmstudio' ? prev.thinking : 'default',
                      }))}
                      disabled={!canManage}
                    >
                      {providerOptions.map(provider => <option key={provider} value={provider}>{provider}</option>)}
                    </select>
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Model</span>
                    <input className="pipelines-input" value={form.model} onChange={event => setForm(prev => ({ ...prev, model: event.target.value }))} disabled={!canManage} placeholder={form.provider === 'gemini' ? 'gemini-2.5-pro' : 'qwen3-coder'} />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Base URL</span>
                    <input className="pipelines-input" value={form.base_url} onChange={event => setForm(prev => ({ ...prev, base_url: event.target.value }))} disabled={!canManage} placeholder="http://lmstudio:1234" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>API key secret</span>
                    <input className="pipelines-input" value={form.api_key_secret} onChange={event => setForm(prev => ({ ...prev, api_key_secret: event.target.value }))} disabled={!canManage} placeholder="GEMINI_API_KEY" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Allowed scopes</span>
                    <input className="pipelines-input" value={form.allowed_scopes} onChange={event => setForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, internal" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Reasoning</span>
                    <select className="pipelines-input" value={form.reasoning} onChange={event => setForm(prev => ({ ...prev, reasoning: event.target.value }))} disabled={!canManage}>
                      <option value="">Provider default</option>
                      <option value="off">Off</option>
                      <option value="low">Low</option>
                      <option value="medium">Medium</option>
                      <option value="high">High</option>
                      <option value="on">On</option>
                    </select>
                  </label>
                  {form.provider === 'lmstudio' && (
                    <label className="flex flex-col gap-1 text-sm">
                      <span>Thinking</span>
                      <select className="pipelines-input" value={form.thinking} onChange={event => setForm(prev => ({ ...prev, thinking: event.target.value as LLMProfileFormState['thinking'] }))} disabled={!canManage}>
                        <option value="default">Provider default</option>
                        <option value="true">True</option>
                        <option value="false">False</option>
                      </select>
                    </label>
                  )}
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


function normalizeLLMProfilesPayload(value: unknown): LLMProfilesPayload {
  const record = asRecord(value);
  const profilesRaw = record && Array.isArray(record.profiles) ? record.profiles : [];
  const profiles = profilesRaw
    .map(item => {
      const profile = asRecord(item);
      if (!profile) return null;
      const name = readString(profile.name).trim();
      if (!name) return null;
      return {
        name,
        provider: readString(profile.provider).trim(),
        model: readString(profile.model).trim(),
        base_url: readString(profile.base_url).trim(),
        api_key_secret: readString(profile.api_key_secret).trim(),
        allowed_scopes: normalizeStringArray(profile.allowed_scopes),
        reasoning: readString(profile.reasoning).trim(),
        thinking: typeof profile.thinking === 'boolean' ? profile.thinking : undefined,
        status: readString(profile.status).trim() || 'unknown',
        validation: readOptionalString(profile.validation),
        references: normalizeStringArray(profile.references),
        allowed_in_scope: typeof profile.allowed_in_scope === 'boolean' ? profile.allowed_in_scope : undefined,
        disabled_reason: readOptionalString(profile.disabled_reason),
      } satisfies LLMProfileRecord;
    })
    .filter(Boolean) as LLMProfileRecord[];

  profiles.sort((a, b) => a.name.localeCompare(b.name));
  return {
    default_profile: readString(record?.default_profile).trim() || profiles[0]?.name || 'standard',
    profiles,
  };
}


export default LLMProfilesPanel;
