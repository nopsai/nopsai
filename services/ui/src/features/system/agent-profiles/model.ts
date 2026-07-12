import { asRecord, normalizeStringArray, readOptionalString, readString } from '../data.js';

export type AgentProfileSource = 'built-in' | 'ui' | 'gitops' | string;

export type AgentProfileRecord = {
  id: string;
  display_name: string;
  role: string;
  description: string;
  instructions: string;
  enabled: boolean;
  built_in?: boolean;
  source: AgentProfileSource;
  usage_count: number;
  references: string[];
  last_updated?: string;
  read_only?: boolean;
  source_path?: string;
};

export type AgentProfilesPayload = {
  default_profile: string;
  profiles: AgentProfileRecord[];
};

export type AgentProfileFormState = {
  id: string;
  display_name: string;
  role: string;
  description: string;
  instructions: string;
  enabled: boolean;
};

export type AgentProfilePanelMode = 'create' | 'edit' | 'view' | 'usage' | 'source' | 'delete';

export type AgentProfileDeleteBlocker = {
  id: string;
  references: string[];
};

export const emptyAgentProfileForm: AgentProfileFormState = {
  id: '',
  display_name: '',
  role: '',
  description: '',
  instructions: '',
  enabled: true,
};

export const emptyAgentProfilesPayload: AgentProfilesPayload = {
  default_profile: 'devops-engineer',
  profiles: [],
};

export function normalizeAgentProfilesPayload(value: unknown): AgentProfilesPayload {
  const record = asRecord(value);
  const profilesRaw = record && Array.isArray(record.profiles) ? record.profiles : [];
  const profiles = profilesRaw
    .map(item => {
      const profile = asRecord(item);
      if (!profile) return null;
      const id = readString(profile.id).trim();
      if (!id) return null;
      return {
        id,
        display_name: readString(profile.display_name).trim() || id,
        role: readString(profile.role).trim(),
        description: readString(profile.description).trim(),
        instructions: readString(profile.instructions).trim(),
        enabled: typeof profile.enabled === 'boolean' ? profile.enabled : true,
        built_in: typeof profile.built_in === 'boolean' ? profile.built_in : undefined,
        source: readString(profile.source).trim() || 'ui',
        usage_count: typeof profile.usage_count === 'number' ? profile.usage_count : 0,
        references: normalizeStringArray(profile.references),
        last_updated: readOptionalString(profile.last_updated),
        read_only: typeof profile.read_only === 'boolean' ? profile.read_only : undefined,
        source_path: readOptionalString(profile.source_path),
      } satisfies AgentProfileRecord;
    })
    .filter(Boolean) as AgentProfileRecord[];

  profiles.sort((a, b) => a.display_name.localeCompare(b.display_name));
  const hasDefaultProfile = Boolean(record && Object.prototype.hasOwnProperty.call(record, 'default_profile'));
  return {
    default_profile: hasDefaultProfile ? readString(record?.default_profile).trim() : 'devops-engineer',
    profiles,
  };
}

export function agentProfileFormFromRecord(profile: AgentProfileRecord): AgentProfileFormState {
  return {
    id: profile.id,
    display_name: profile.display_name,
    role: profile.role,
    description: profile.description,
    instructions: profile.instructions,
    enabled: profile.enabled,
  };
}

export function duplicateAgentProfileForm(profile: AgentProfileRecord): AgentProfileFormState {
  return {
    id: `${profile.id}-custom`,
    display_name: `${profile.display_name} Custom`,
    role: profile.role,
    description: profile.description,
    instructions: profile.instructions,
    enabled: true,
  };
}

export function agentProfilePayloadFromForm(form: AgentProfileFormState) {
  return {
    id: form.id.trim(),
    display_name: form.display_name.trim(),
    role: form.role.trim(),
    description: form.description.trim(),
    instructions: form.instructions.trim(),
    enabled: form.enabled,
  };
}

export function agentProfileSourceLabel(source: AgentProfileSource): string {
  const normalized = (source || '').trim().toLowerCase();
  if (normalized === 'built-in') return 'Built-in';
  if (normalized === 'gitops') return 'GitOps';
  if (normalized === 'ui') return 'Custom';
  return source || 'Custom';
}

export function agentProfileSection(profile: AgentProfileRecord): 'built-in' | 'custom' | 'gitops' {
  if (profile.source === 'built-in' || profile.built_in) return 'built-in';
  if (profile.source === 'gitops') return 'gitops';
  return 'custom';
}
