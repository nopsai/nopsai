import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { Pencil, Save, Star, Trash2 } from 'lucide-react';
import type {
  TeamAgentProfile,
  TeamAgentProfilePayload,
  TeamAgentProfilesResponse,
  TeamLLMProfile,
  TeamLLMProfilePayload,
  TeamLLMProfilesResponse,
  TeamMCPProfile,
  TeamMCPProfilePayload,
  TeamMCPProfilesResponse,
} from './api';

type LLMFormState = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  credential_ref: string;
  allowed_scopes: string;
  reasoning: string;
  timeout_seconds: string;
  max_tokens: string;
  temperature: string;
};

type AgentFormState = {
  id: string;
  display_name: string;
  role: string;
  description: string;
  instructions: string;
  enabled: boolean;
};

type MCPFormState = {
  name: string;
  description: string;
  enabled: boolean;
  servers: string;
  allowed_scopes: string;
};

const emptyLLMForm: LLMFormState = {
  name: '',
  provider: 'openai',
  model: '',
  base_url: '',
  credential_ref: '',
  allowed_scopes: '',
  reasoning: '',
  timeout_seconds: '',
  max_tokens: '',
  temperature: '',
};

const emptyAgentForm: AgentFormState = {
  id: '',
  display_name: '',
  role: '',
  description: '',
  instructions: '',
  enabled: true,
};

const emptyMCPForm: MCPFormState = {
  name: '',
  description: '',
  enabled: true,
  servers: '',
  allowed_scopes: '',
};

export function TeamAIProfilesPanel({
  llmProfiles,
  agentProfiles,
  mcpProfiles,
  loading,
  saving,
  error,
  canManage,
  gitOpsTarget,
  onSaveLLM,
  onSetDefaultLLM,
  onDeleteLLM,
  onSaveAgent,
  onSetDefaultAgent,
  onDeleteAgent,
  onSaveMCP,
  onDeleteMCP,
  onCheckDrift,
}: {
  llmProfiles: TeamLLMProfilesResponse | null;
  agentProfiles: TeamAgentProfilesResponse | null;
  mcpProfiles: TeamMCPProfilesResponse | null;
  loading: boolean;
  saving: boolean;
  error: string | null;
  canManage: boolean;
  gitOpsTarget: string;
  onSaveLLM: (profileName: string, payload: TeamLLMProfilePayload) => Promise<void>;
  onSetDefaultLLM: (profileName: string) => Promise<void>;
  onDeleteLLM: (profileName: string) => Promise<void>;
  onSaveAgent: (profileID: string, payload: TeamAgentProfilePayload) => Promise<void>;
  onSetDefaultAgent: (profileID: string) => Promise<void>;
  onDeleteAgent: (profileID: string) => Promise<void>;
  onSaveMCP: (profileName: string, payload: TeamMCPProfilePayload) => Promise<void>;
  onDeleteMCP: (profileName: string) => Promise<void>;
  onCheckDrift: () => Promise<void>;
}) {
  const [activeProfileTab, setActiveProfileTab] = useState<'llm' | 'agent' | 'mcp'>('llm');
  const [llmForm, setLLMForm] = useState<LLMFormState>(emptyLLMForm);
  const [agentForm, setAgentForm] = useState<AgentFormState>(emptyAgentForm);
  const [mcpForm, setMCPForm] = useState<MCPFormState>(emptyMCPForm);
  const canEdit = canManage && !saving && !loading;
  const inputClass = 'pipelines-input w-full text-sm disabled:cursor-not-allowed disabled:opacity-70';
  const textareaClass = `${inputClass} min-h-[92px] resize-y`;
  const sectionClass = 'rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4';
  const fieldClass = 'flex flex-col gap-1 text-sm text-[var(--text-primary)]';
  const toggleClass = 'flex min-h-[42px] items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]';
  const tabClass = (tab: 'llm' | 'agent' | 'mcp') =>
    `inline-flex min-h-[36px] items-center justify-center rounded-md px-3 py-1.5 text-sm font-semibold transition ${
      activeProfileTab === tab
        ? 'bg-[var(--bg-primary)] text-[var(--text-primary)] shadow-sm border border-[var(--border-primary)]'
        : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
    }`;

  const sortedLLMProfiles = useMemo(() => [...(llmProfiles?.profiles ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [llmProfiles]);
  const sortedAgentProfiles = useMemo(() => [...(agentProfiles?.profiles ?? [])].sort((a, b) => a.id.localeCompare(b.id)), [agentProfiles]);
  const sortedMCPProfiles = useMemo(() => [...(mcpProfiles?.profiles ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [mcpProfiles]);

  useEffect(() => {
    if (!llmForm.name && sortedLLMProfiles[0]) setLLMForm(llmProfileToForm(sortedLLMProfiles[0]));
  }, [llmForm.name, sortedLLMProfiles]);

  useEffect(() => {
    if (!agentForm.id && sortedAgentProfiles[0]) setAgentForm(agentProfileToForm(sortedAgentProfiles[0]));
  }, [agentForm.id, sortedAgentProfiles]);

  useEffect(() => {
    if (!mcpForm.name && sortedMCPProfiles[0]) setMCPForm(mcpProfileToForm(sortedMCPProfiles[0]));
  }, [mcpForm.name, sortedMCPProfiles]);

  if (loading) {
    return <div className="text-sm text-[var(--text-secondary)]">Loading AI profiles...</div>;
  }

  return (
    <div className="space-y-5" role="tabpanel" aria-label="AI profiles">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h4 className="text-sm font-semibold text-[var(--text-primary)]">Team AI profiles</h4>
          <p className="text-xs text-[var(--text-secondary)]">{gitOpsTarget ? `GitOps target: ${gitOpsTarget}` : 'No config repository connected'}</p>
        </div>
        <button type="button" className="glass-button-subtle" onClick={() => void onCheckDrift()} disabled={!gitOpsTarget || saving}>
          Review GitOps drift
        </button>
      </div>

      <div className="inline-flex rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-1" role="tablist" aria-label="AI profile types">
        <button type="button" role="tab" aria-selected={activeProfileTab === 'llm'} className={tabClass('llm')} onClick={() => setActiveProfileTab('llm')}>LLM</button>
        <button type="button" role="tab" aria-selected={activeProfileTab === 'agent'} className={tabClass('agent')} onClick={() => setActiveProfileTab('agent')}>Agents</button>
        <button type="button" role="tab" aria-selected={activeProfileTab === 'mcp'} className={tabClass('mcp')} onClick={() => setActiveProfileTab('mcp')}>MCP</button>
      </div>

      {!canManage && <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">Read-only.</div>}
      {error && <div className="text-sm text-red-600 break-words">{error}</div>}

      {activeProfileTab === 'llm' && (
        <ProfileEditorLayout
          list={<ProfileList
            emptyLabel="No team LLM profiles"
            rows={sortedLLMProfiles.map(profile => ({
              key: profile.name,
              title: profile.name,
              subtitle: [profile.provider, profile.model].filter(Boolean).join(' / '),
              isDefault: profile.name === llmProfiles?.default_profile,
              onEdit: () => setLLMForm(llmProfileToForm(profile)),
              onDefault: () => void onSetDefaultLLM(profile.name),
              onDelete: () => void onDeleteLLM(profile.name),
            }))}
            canEdit={canEdit}
            saving={saving}
          />}
          editor={<form className={`${sectionClass} space-y-4`} onSubmit={event => void submitLLMForm(event, llmForm, onSaveLLM)}>
            <ProfileFormHeader title="LLM profile" onNew={() => setLLMForm(emptyLLMForm)} canEdit={canEdit} />
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field id="team-llm-name" label="Name" value={llmForm.name} onChange={value => setLLMForm(prev => ({ ...prev, name: value }))} disabled={!canEdit} inputClass={inputClass} required />
              <Field id="team-llm-provider" label="Provider" value={llmForm.provider} onChange={value => setLLMForm(prev => ({ ...prev, provider: value }))} disabled={!canEdit} inputClass={inputClass} required />
              <Field id="team-llm-model" label="Model" value={llmForm.model} onChange={value => setLLMForm(prev => ({ ...prev, model: value }))} disabled={!canEdit} inputClass={inputClass} />
              <Field id="team-llm-credential" label="Credential ref" value={llmForm.credential_ref} onChange={value => setLLMForm(prev => ({ ...prev, credential_ref: value }))} disabled={!canEdit} inputClass={inputClass} required />
              <Field id="team-llm-base-url" label="Base URL" value={llmForm.base_url} onChange={value => setLLMForm(prev => ({ ...prev, base_url: value }))} disabled={!canEdit} inputClass={inputClass} />
              <Field id="team-llm-scopes" label="Allowed scopes" value={llmForm.allowed_scopes} onChange={value => setLLMForm(prev => ({ ...prev, allowed_scopes: value }))} disabled={!canEdit} inputClass={inputClass} />
              <Field id="team-llm-timeout" label="Timeout seconds" value={llmForm.timeout_seconds} onChange={value => setLLMForm(prev => ({ ...prev, timeout_seconds: value }))} disabled={!canEdit} inputClass={inputClass} type="number" />
              <Field id="team-llm-max-tokens" label="Max tokens" value={llmForm.max_tokens} onChange={value => setLLMForm(prev => ({ ...prev, max_tokens: value }))} disabled={!canEdit} inputClass={inputClass} type="number" />
              <Field id="team-llm-temperature" label="Temperature" value={llmForm.temperature} onChange={value => setLLMForm(prev => ({ ...prev, temperature: value }))} disabled={!canEdit} inputClass={inputClass} type="number" step="0.01" />
              <Field id="team-llm-reasoning" label="Reasoning" value={llmForm.reasoning} onChange={value => setLLMForm(prev => ({ ...prev, reasoning: value }))} disabled={!canEdit} inputClass={inputClass} />
            </div>
            <SaveButton saving={saving} canEdit={canEdit} label="Save LLM profile" />
          </form>}
        />
      )}

      {activeProfileTab === 'agent' && (
        <ProfileEditorLayout
          list={<ProfileList
            emptyLabel="No team agent profiles"
            rows={sortedAgentProfiles.map(profile => ({
              key: profile.id,
              title: profile.id,
              subtitle: profile.display_name,
              isDefault: profile.id === agentProfiles?.default_profile,
              onEdit: () => setAgentForm(agentProfileToForm(profile)),
              onDefault: () => void onSetDefaultAgent(profile.id),
              onDelete: () => void onDeleteAgent(profile.id),
            }))}
            canEdit={canEdit}
            saving={saving}
          />}
          editor={<form className={`${sectionClass} space-y-4`} onSubmit={event => void submitAgentForm(event, agentForm, onSaveAgent)}>
            <ProfileFormHeader title="Agent profile" onNew={() => setAgentForm(emptyAgentForm)} canEdit={canEdit} />
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field id="team-agent-id" label="ID" value={agentForm.id} onChange={value => setAgentForm(prev => ({ ...prev, id: value }))} disabled={!canEdit} inputClass={inputClass} required />
              <Field id="team-agent-display" label="Display name" value={agentForm.display_name} onChange={value => setAgentForm(prev => ({ ...prev, display_name: value }))} disabled={!canEdit} inputClass={inputClass} required />
              <Field id="team-agent-role" label="Role" value={agentForm.role} onChange={value => setAgentForm(prev => ({ ...prev, role: value }))} disabled={!canEdit} inputClass={inputClass} />
              <Field id="team-agent-description" label="Description" value={agentForm.description} onChange={value => setAgentForm(prev => ({ ...prev, description: value }))} disabled={!canEdit} inputClass={inputClass} />
            </div>
            <label htmlFor="team-agent-instructions" className={fieldClass}>
              <span>Instructions</span>
              <textarea id="team-agent-instructions" value={agentForm.instructions} onChange={event => setAgentForm(prev => ({ ...prev, instructions: event.target.value }))} disabled={!canEdit} className={textareaClass} required />
            </label>
            <label className={toggleClass}>
              <input type="checkbox" className="h-4 w-4 rounded border-[var(--border-primary)]" checked={agentForm.enabled} onChange={event => setAgentForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canEdit} />
              Enabled
            </label>
            <SaveButton saving={saving} canEdit={canEdit} label="Save agent profile" />
          </form>}
        />
      )}

      {activeProfileTab === 'mcp' && (
        <ProfileEditorLayout
          list={<ProfileList
            emptyLabel="No team MCP profiles"
            rows={sortedMCPProfiles.map(profile => ({
              key: profile.name,
              title: profile.name,
              subtitle: profile.servers?.map(ref => ref.server).join(', ') || 'No servers',
              isDefault: false,
              onEdit: () => setMCPForm(mcpProfileToForm(profile)),
              onDelete: () => void onDeleteMCP(profile.name),
            }))}
            canEdit={canEdit}
            saving={saving}
            hideDefault
          />}
          editor={<form className={`${sectionClass} space-y-4`} onSubmit={event => void submitMCPForm(event, mcpForm, onSaveMCP)}>
            <ProfileFormHeader title="MCP profile" onNew={() => setMCPForm(emptyMCPForm)} canEdit={canEdit} />
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field id="team-mcp-name" label="Name" value={mcpForm.name} onChange={value => setMCPForm(prev => ({ ...prev, name: value }))} disabled={!canEdit} inputClass={inputClass} required />
              <Field id="team-mcp-scopes" label="Allowed scopes" value={mcpForm.allowed_scopes} onChange={value => setMCPForm(prev => ({ ...prev, allowed_scopes: value }))} disabled={!canEdit} inputClass={inputClass} />
            </div>
            <label htmlFor="team-mcp-description" className={fieldClass}>
              <span>Description</span>
              <textarea id="team-mcp-description" value={mcpForm.description} onChange={event => setMCPForm(prev => ({ ...prev, description: event.target.value }))} disabled={!canEdit} className={textareaClass} />
            </label>
            <label htmlFor="team-mcp-servers" className={fieldClass}>
              <span>Servers</span>
              <textarea id="team-mcp-servers" value={mcpForm.servers} onChange={event => setMCPForm(prev => ({ ...prev, servers: event.target.value }))} disabled={!canEdit} className={textareaClass} required placeholder="github:*" />
            </label>
            <label className={toggleClass}>
              <input type="checkbox" className="h-4 w-4 rounded border-[var(--border-primary)]" checked={mcpForm.enabled} onChange={event => setMCPForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canEdit} />
              Enabled
            </label>
            <SaveButton saving={saving} canEdit={canEdit} label="Save MCP profile" />
          </form>}
        />
      )}
    </div>
  );
}

function ProfileEditorLayout({ list, editor }: { list: ReactNode; editor: ReactNode }) {
  return <div className="grid grid-cols-1 xl:grid-cols-[minmax(260px,320px)_1fr] gap-4">{list}{editor}</div>;
}

function ProfileList({
  rows,
  emptyLabel,
  canEdit,
  saving,
  hideDefault,
}: {
  rows: Array<{ key: string; title: string; subtitle: string; isDefault: boolean; onEdit: () => void; onDefault?: () => void; onDelete: () => void }>;
  emptyLabel: string;
  canEdit: boolean;
  saving: boolean;
  hideDefault?: boolean;
}) {
  if (rows.length === 0) {
    return <div className="rounded-lg border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 text-sm text-[var(--text-secondary)]">{emptyLabel}</div>;
  }
  return (
    <div className="space-y-2">
      {rows.map(row => (
        <div key={row.key} className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <p className="truncate text-sm font-semibold text-[var(--text-primary)]">{row.title}</p>
                {row.isDefault && <span className="runner-pill runner-pill--ok">Default</span>}
              </div>
              <p className="mt-1 truncate text-xs text-[var(--text-secondary)]">{row.subtitle || '-'}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <button type="button" className="pipelines-icon-only" aria-label={`Edit ${row.title}`} onClick={row.onEdit}>
                <Pencil className="h-4 w-4" aria-hidden="true" />
              </button>
              {!hideDefault && row.onDefault && (
                <button type="button" className="pipelines-icon-only" aria-label={`Set ${row.title} as default`} onClick={row.onDefault} disabled={!canEdit || saving || row.isDefault}>
                  <Star className="h-4 w-4" aria-hidden="true" />
                </button>
              )}
              <button type="button" className="pipelines-icon-only text-red-600" aria-label={`Delete ${row.title}`} onClick={row.onDelete} disabled={!canEdit || saving}>
                <Trash2 className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function ProfileFormHeader({ title, onNew, canEdit }: { title: string; onNew: () => void; canEdit: boolean }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <h5 className="text-sm font-semibold text-[var(--text-primary)]">{title}</h5>
      <button type="button" className="glass-button-subtle" onClick={onNew} disabled={!canEdit}>
        New
      </button>
    </div>
  );
}

function Field({
  id,
  label,
  value,
  onChange,
  disabled,
  inputClass,
  required,
  type,
  step,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  inputClass: string;
  required?: boolean;
  type?: string;
  step?: string;
}) {
  return (
    <label htmlFor={id} className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
      <span>{label}</span>
      <input id={id} value={value} onChange={event => onChange(event.target.value)} disabled={disabled} className={inputClass} required={required} type={type} step={step} />
    </label>
  );
}

function SaveButton({ saving, canEdit, label }: { saving: boolean; canEdit: boolean; label: string }) {
  return (
    <div className="flex justify-end">
      <button type="submit" className="glass-button-primary" disabled={!canEdit || saving}>
        <Save className="h-4 w-4" aria-hidden="true" />
        {saving ? 'Saving...' : label}
      </button>
    </div>
  );
}

function llmProfileToForm(profile: TeamLLMProfile): LLMFormState {
  return {
    name: profile.name,
    provider: profile.provider || 'openai',
    model: profile.model || '',
    base_url: profile.base_url || '',
    credential_ref: profile.credential_ref || '',
    allowed_scopes: joinList(profile.allowed_scopes),
    reasoning: profile.reasoning || '',
    timeout_seconds: numberToString(profile.timeout_seconds),
    max_tokens: numberToString(profile.max_tokens),
    temperature: numberToString(profile.temperature),
  };
}

function agentProfileToForm(profile: TeamAgentProfile): AgentFormState {
  return {
    id: profile.id,
    display_name: profile.display_name,
    role: profile.role || '',
    description: profile.description || '',
    instructions: profile.instructions,
    enabled: profile.enabled ?? true,
  };
}

function mcpProfileToForm(profile: TeamMCPProfile): MCPFormState {
  return {
    name: profile.name,
    description: profile.description || '',
    enabled: profile.enabled ?? true,
    servers: (profile.servers || []).map(ref => `${ref.server}:${(ref.tools || []).join(',')}`).join('\n'),
    allowed_scopes: joinList(profile.allowed_scopes),
  };
}

async function submitLLMForm(event: FormEvent, form: LLMFormState, onSave: (profileName: string, payload: TeamLLMProfilePayload) => Promise<void>) {
  event.preventDefault();
  await onSave(form.name, {
    name: form.name.trim(),
    provider: form.provider.trim(),
    model: form.model.trim(),
    base_url: form.base_url.trim(),
    credential_ref: form.credential_ref.trim(),
    allowed_scopes: splitList(form.allowed_scopes),
    reasoning: form.reasoning.trim(),
    timeout_seconds: parseOptionalInt(form.timeout_seconds),
    max_tokens: parseOptionalInt(form.max_tokens),
    temperature: parseOptionalFloat(form.temperature),
  });
}

async function submitAgentForm(event: FormEvent, form: AgentFormState, onSave: (profileID: string, payload: TeamAgentProfilePayload) => Promise<void>) {
  event.preventDefault();
  await onSave(form.id, {
    id: form.id.trim(),
    display_name: form.display_name.trim(),
    role: form.role.trim(),
    description: form.description.trim(),
    instructions: form.instructions.trim(),
    enabled: form.enabled,
  });
}

async function submitMCPForm(event: FormEvent, form: MCPFormState, onSave: (profileName: string, payload: TeamMCPProfilePayload) => Promise<void>) {
  event.preventDefault();
  await onSave(form.name, {
    name: form.name.trim(),
    description: form.description.trim(),
    enabled: form.enabled,
    servers: parseServerRefs(form.servers),
    allowed_scopes: splitList(form.allowed_scopes),
  });
}

function parseServerRefs(raw: string) {
  return raw
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(Boolean)
    .map(line => {
      const [server, tools = ''] = line.split(':', 2);
      return { server: server.trim(), tools: splitList(tools) };
    });
}

function splitList(raw: string): string[] {
  return raw.split(/[\n,]/).map(value => value.trim()).filter(Boolean);
}

function joinList(values?: string[]): string {
  return (values || []).join(', ');
}

function parseOptionalInt(raw: string): number | undefined {
  const value = Number.parseInt(raw, 10);
  return Number.isFinite(value) ? value : undefined;
}

function parseOptionalFloat(raw: string): number | undefined {
  const value = Number.parseFloat(raw);
  return Number.isFinite(value) ? value : undefined;
}

function numberToString(value?: number): string {
  return typeof value === 'number' && Number.isFinite(value) ? String(value) : '';
}
