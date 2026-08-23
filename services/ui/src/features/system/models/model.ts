import { getLLMProvider } from '../llmProviders.js';
import { asRecord, normalizeNumber, normalizeStringArray, normalizeStringMap, readOptionalString, readString } from '../data.js';

export type LLMProfileRecord = {
  name: string;
  scope?: 'global' | 'team';
  team_path?: string;
  team_local_name?: string;
  provider: string;
  model: string;
  base_url: string;
  credential_ref: string;
  allowed_scopes: string[];
  reasoning: string;
  thinking?: boolean;
  timeout_seconds: number;
  max_tokens: number;
  temperature?: number;
  prompt_cache?: LLMFeatureConfig;
  provider_state?: LLMFeatureConfig;
  pricing?: LLMPricing;
  extra: Record<string, string>;
  status: string;
  validation?: string;
  references?: string[];
  allowed_in_scope?: boolean;
  disabled_reason?: string;
};

export type LLMFeatureConfig = {
  mode?: string;
  scope?: string;
  retention?: string;
};

/**
 * Rate card in USD per million tokens. It is optional: a model without one still
 * runs, and its usage is reported as unpriced rather than as free, so leaving it
 * blank never understates spend.
 */
export type LLMPricing = {
  input_per_million_usd: number;
  output_per_million_usd: number;
  cached_input_per_million_usd?: number;
  cache_write_per_million_usd?: number;
};

export type LLMFeatureMode = 'auto' | 'required' | 'disabled';

export const LLM_FEATURE_MODE_OPTIONS: Array<{ value: LLMFeatureMode; label: string }> = [
  { value: 'auto', label: 'Auto' },
  { value: 'required', label: 'Required' },
  { value: 'disabled', label: 'Disabled' },
];

export type LLMProfilesPayload = {
  default_profile: string;
  profiles: LLMProfileRecord[];
};

export type LLMProfileFormState = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  credential_ref: string;
  allowed_scopes: string;
  reasoning: string;
  thinking: 'default' | 'true' | 'false';
  timeout_seconds: string;
  max_tokens: string;
  temperature: string;
  prompt_cache: string;
  provider_state: string;
  pricing_input: string;
  pricing_output: string;
  pricing_cached_input: string;
  pricing_cache_write: string;
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
  credential_ref: 'credential://system/llm/standard',
  allowed_scopes: '',
  reasoning: '',
  thinking: 'default',
  timeout_seconds: '',
  max_tokens: '',
  temperature: '',
  prompt_cache: '',
  provider_state: '',
  pricing_input: '',
  pricing_output: '',
  pricing_cached_input: '',
  pricing_cache_write: '',
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
        credential_ref: readString(profile.credential_ref).trim(),
        allowed_scopes: normalizeStringArray(profile.allowed_scopes),
        reasoning: readString(profile.reasoning).trim(),
        thinking: typeof profile.thinking === 'boolean' ? profile.thinking : undefined,
        timeout_seconds: normalizeNumber(profile.timeout_seconds),
        max_tokens: normalizeNumber(profile.max_tokens),
        temperature: typeof profile.temperature === 'number' ? profile.temperature : undefined,
        prompt_cache: normalizeLLMFeatureConfig(profile.prompt_cache),
        provider_state: normalizeLLMFeatureConfig(profile.provider_state),
        pricing: normalizeLLMPricing(profile.pricing),
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
  const hasDefaultProfile = Boolean(record && Object.prototype.hasOwnProperty.call(record, 'default_profile'));
  return {
    default_profile: hasDefaultProfile ? readString(record?.default_profile).trim() : profiles[0]?.name || 'standard',
    profiles,
  };
}

export function llmProfileFormFromRecord(profile: LLMProfileRecord): LLMProfileFormState {
  return {
    name: profile.name,
    provider: profile.provider || 'gemini',
    model: profile.model || '',
    base_url: profile.base_url || '',
    credential_ref: profile.credential_ref || '',
    allowed_scopes: (profile.allowed_scopes || []).join(', '),
    reasoning: profile.reasoning || '',
    thinking: profile.thinking === undefined ? 'default' : profile.thinking ? 'true' : 'false',
    timeout_seconds: profile.timeout_seconds > 0 ? String(profile.timeout_seconds) : '',
    max_tokens: profile.max_tokens > 0 ? String(profile.max_tokens) : '',
    temperature: profile.temperature === undefined ? '' : String(profile.temperature),
    prompt_cache: formatLLMFeatureConfig(profile.prompt_cache),
    provider_state: formatLLMFeatureConfig(profile.provider_state),
    pricing_input: formatRate(profile.pricing?.input_per_million_usd),
    pricing_output: formatRate(profile.pricing?.output_per_million_usd),
    pricing_cached_input: formatRate(profile.pricing?.cached_input_per_million_usd),
    pricing_cache_write: formatRate(profile.pricing?.cache_write_per_million_usd),
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
    credential_ref: providerDefinition.apiKeyMode === 'none' ? '' : form.credential_ref.trim(),
    allowed_scopes: form.allowed_scopes.split(',').map(item => item.trim()).filter(Boolean),
    reasoning: providerDefinition.supportsReasoning ? form.reasoning.trim() : '',
    thinking: providerDefinition.supportsThinking && form.thinking !== 'default' ? form.thinking === 'true' : undefined,
    timeout_seconds: parseOptionalNonNegativeNumber(form.timeout_seconds),
    max_tokens: providerDefinition.supportsMaxTokens ? parseOptionalNonNegativeNumber(form.max_tokens) : undefined,
    temperature: providerDefinition.supportsTemperature ? parseOptionalNumber(form.temperature) : undefined,
    prompt_cache: parseLLMFeatureConfig(form.prompt_cache),
    provider_state: parseLLMFeatureConfig(form.provider_state),
    // Always stated, never omitted: the server keeps an existing rate card when a
    // payload stays silent, so clearing the fields has to say so explicitly.
    pricing: llmPricingFromForm(form),
    extra: parseExtraOptions(form.extra),
  };
}

export function normalizeLLMPricing(value: unknown): LLMPricing | undefined {
  const record = asRecord(value);
  if (!record) return undefined;
  const pricing: LLMPricing = {
    input_per_million_usd: normalizeNumber(record.input_per_million_usd),
    output_per_million_usd: normalizeNumber(record.output_per_million_usd),
  };
  if (typeof record.cached_input_per_million_usd === 'number') {
    pricing.cached_input_per_million_usd = record.cached_input_per_million_usd;
  }
  if (typeof record.cache_write_per_million_usd === 'number') {
    pricing.cache_write_per_million_usd = record.cache_write_per_million_usd;
  }
  return pricing;
}

/**
 * Builds the rate card a save should store, or null to state that this model has
 * none. A model that costs nothing says so with explicit zeroes, which is a
 * claim the operator makes, so a blank form clears rather than keeps.
 */
export function llmPricingFromForm(form: LLMProfileFormState): LLMPricing | null {
  const input = parseOptionalNonNegativeNumber(form.pricing_input);
  const output = parseOptionalNonNegativeNumber(form.pricing_output);
  const cachedInput = parseOptionalNonNegativeNumber(form.pricing_cached_input);
  const cacheWrite = parseOptionalNonNegativeNumber(form.pricing_cache_write);
  if (input === undefined && output === undefined && cachedInput === undefined && cacheWrite === undefined) {
    return null;
  }
  const pricing: LLMPricing = {
    input_per_million_usd: input || 0,
    output_per_million_usd: output || 0,
  };
  if (cachedInput !== undefined) pricing.cached_input_per_million_usd = cachedInput;
  if (cacheWrite !== undefined) pricing.cache_write_per_million_usd = cacheWrite;
  return pricing;
}

function formatRate(value?: number): string {
  return typeof value === 'number' && Number.isFinite(value) ? String(value) : '';
}

export function normalizeLLMFeatureConfig(value: unknown): LLMFeatureConfig | undefined {
  const record = asRecord(value);
  if (!record) return undefined;
  const feature: LLMFeatureConfig = {};
  const mode = readOptionalString(record.mode)?.trim();
  const scope = readOptionalString(record.scope)?.trim();
  const retention = readOptionalString(record.retention)?.trim();
  if (mode) feature.mode = mode;
  if (scope) feature.scope = scope;
  if (retention) feature.retention = retention;
  return Object.keys(feature).length ? feature : undefined;
}

export function formatLLMFeatureConfig(value?: LLMFeatureConfig): string {
  if (!value || Object.keys(value).length === 0) return '';
  return JSON.stringify(value, null, 2);
}

export function parseLLMFeatureConfig(value: string): LLMFeatureConfig | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  try {
    return normalizeLLMFeatureConfig(JSON.parse(trimmed));
  } catch {
    return undefined;
  }
}

export function llmFeatureModeFromFormValue(value: string): LLMFeatureMode {
  const mode = parseLLMFeatureConfig(value)?.mode?.trim().toLowerCase();
  if (mode === 'required' || mode === 'disabled') return mode;
  return 'auto';
}

export function llmFeatureFormValueWithMode(value: string, mode: LLMFeatureMode): string {
  const current = parseLLMFeatureConfig(value) || {};
  return formatLLMFeatureConfig({ ...current, mode });
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
