import { getLLMProvider } from '../llmProviders.js';
import { asRecord, normalizeNumber, normalizeStringArray, normalizeStringMap, readOptionalString, readString } from '../data.js';

export type LLMProfileRecord = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  api_key_secret: string;
  allowed_scopes: string[];
  reasoning: string;
  thinking?: boolean;
  timeout_seconds: number;
  max_tokens: number;
  temperature?: number;
  extra: Record<string, string>;
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
  timeout_seconds: string;
  max_tokens: string;
  temperature: string;
  extra: string;
};

export type LLMProfilePanelMode = 'create' | 'edit' | 'delete';

export type LLMProfileDeleteBlocker = {
  name: string;
  references: string[];
  migrateTo: string;
};

export const emptyLLMProfileForm: LLMProfileFormState = {
  name: '',
  provider: 'lmstudio',
  model: 'qwen3-coder',
  base_url: 'http://lmstudio:1234',
  api_key_secret: 'LLM_API_KEY',
  allowed_scopes: '',
  reasoning: '',
  thinking: 'default',
  timeout_seconds: '',
  max_tokens: '',
  temperature: '',
  extra: '',
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
        timeout_seconds: normalizeNumber(profile.timeout_seconds),
        max_tokens: normalizeNumber(profile.max_tokens),
        temperature: typeof profile.temperature === 'number' ? profile.temperature : undefined,
        extra: normalizeStringMap(profile.extra),
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
    timeout_seconds: profile.timeout_seconds > 0 ? String(profile.timeout_seconds) : '',
    max_tokens: profile.max_tokens > 0 ? String(profile.max_tokens) : '',
    temperature: profile.temperature === undefined ? '' : String(profile.temperature),
    extra: formatExtraOptions(profile.extra),
  };
}

export function llmProfilePayloadFromForm(form: LLMProfileFormState) {
  const provider = form.provider.trim();
  const providerDefinition = getLLMProvider(provider);
  return {
    name: form.name.trim(),
    provider,
    model: form.model.trim(),
    base_url: providerDefinition.baseURLMode === 'hidden' ? '' : form.base_url.trim(),
    api_key_secret: providerDefinition.apiKeyMode === 'none' ? '' : form.api_key_secret.trim(),
    allowed_scopes: form.allowed_scopes.split(',').map(item => item.trim()).filter(Boolean),
    reasoning: providerDefinition.supportsReasoning ? form.reasoning.trim() : '',
    thinking: providerDefinition.supportsThinking && form.thinking !== 'default' ? form.thinking === 'true' : undefined,
    timeout_seconds: parseOptionalNonNegativeNumber(form.timeout_seconds),
    max_tokens: providerDefinition.supportsMaxTokens ? parseOptionalNonNegativeNumber(form.max_tokens) : undefined,
    temperature: providerDefinition.supportsTemperature ? parseOptionalNumber(form.temperature) : undefined,
    extra: parseExtraOptions(form.extra),
  };
}

export function formatExtraOptions(extra: Record<string, string>): string {
  return Object.entries(extra)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join('\n');
}

export function parseExtraOptions(value: string): Record<string, string> {
  const extra: Record<string, string> = {};
  value.split(/\r?\n/).forEach(line => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const separator = trimmed.indexOf('=');
    if (separator < 1) return;
    const key = trimmed.slice(0, separator).trim();
    if (!key) return;
    extra[key] = trimmed.slice(separator + 1).trim();
  });
  return extra;
}

function parseOptionalNumber(value: string): number | undefined {
  if (!value.trim()) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function parseOptionalNonNegativeNumber(value: string): number | undefined {
  const parsed = parseOptionalNumber(value);
  return parsed !== undefined && parsed >= 0 ? parsed : undefined;
}
