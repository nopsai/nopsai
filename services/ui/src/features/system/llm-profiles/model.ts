import { asRecord, normalizeStringArray, readOptionalString, readString } from '../data.js';

export type LLMProfileRecord = {
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

export type LLMProfilesPayload = {
  default_profile: string;
  profiles: LLMProfileRecord[];
};

export type LLMProfileFormState = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  api_key_secret: string;
  allowed_scopes: string;
  reasoning: string;
  thinking: 'default' | 'true' | 'false';
};

export type LLMProfilePanelMode = 'create' | 'edit' | 'delete';

export type LLMProfileDeleteBlocker = {
  name: string;
  references: string[];
  migrateTo: string;
};

export const providerOptions = ['gemini', 'lmstudio'];

export const emptyLLMProfileForm: LLMProfileFormState = {
  name: '',
  provider: 'gemini',
  model: '',
  base_url: '',
  api_key_secret: 'GEMINI_API_KEY',
  allowed_scopes: '',
  reasoning: '',
  thinking: 'default',
};

export const emptyLLMProfilesPayload: LLMProfilesPayload = {
  default_profile: 'standard',
  profiles: [],
};

export function normalizeLLMProfilesPayload(value: unknown): LLMProfilesPayload {
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

export function llmProfileFormFromRecord(profile: LLMProfileRecord): LLMProfileFormState {
  return {
    name: profile.name,
    provider: profile.provider || 'gemini',
    model: profile.model || '',
    base_url: profile.base_url || '',
    api_key_secret: profile.api_key_secret || '',
    allowed_scopes: (profile.allowed_scopes || []).join(', '),
    reasoning: profile.reasoning || '',
    thinking: profile.thinking === undefined ? 'default' : profile.thinking ? 'true' : 'false',
  };
}

export function llmProfilePayloadFromForm(form: LLMProfileFormState) {
  const provider = form.provider.trim();
  return {
    name: form.name.trim(),
    provider,
    model: form.model.trim(),
    base_url: form.base_url.trim(),
    api_key_secret: form.api_key_secret.trim(),
    allowed_scopes: form.allowed_scopes.split(',').map(item => item.trim()).filter(Boolean),
    reasoning: form.reasoning.trim(),
    thinking: provider === 'lmstudio' && form.thinking !== 'default' ? form.thinking === 'true' : undefined,
  };
}
