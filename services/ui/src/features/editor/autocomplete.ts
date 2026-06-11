import { apiClient } from '../../lib/api.js';

export type ScopedEditorItems = {
  scope: string;
  items: string[];
};

export type EditorAutocompleteMetadata = {
  secrets: string[];
  variables: string[];
  reusableSteps: string[];
  secretScopes: ScopedEditorItems[];
  variableScopes: ScopedEditorItems[];
  agentProfiles: string[];
  llmProfiles: string[];
  mcpProfiles: string[];
  fetchedAt: number;
  loading: boolean;
};

export type EditorAutocompleteOptions = {
  includeAgentProfiles?: boolean;
  includeLLMProfiles?: boolean;
  includeMCPProfiles?: boolean;
};

export function normalizeAutocompleteList(payload: unknown): string[] {
  if (!Array.isArray(payload)) return [];
  return payload
    .map(item => {
      if (typeof item === 'string') return item.trim();
      if (item && typeof item === 'object') {
        const record = item as Record<string, unknown>;
        const name = typeof record.name === 'string' ? record.name : typeof record.id === 'string' ? record.id : '';
        return name.trim();
      }
      return '';
    })
    .filter(Boolean);
}

export function normalizeProfilePayload(payload: unknown): string[] {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : null;
  const profiles = record && Array.isArray(record.profiles) ? record.profiles : payload;
  if (!Array.isArray(profiles)) return [];
  return profiles
    .map(item => {
      if (typeof item === 'string') return item.trim();
      if (item && typeof item === 'object') {
        const profile = item as Record<string, unknown>;
        if (profile.enabled === false || profile.allowed_in_scope === false) return '';
        const name = typeof profile.name === 'string' ? profile.name : typeof profile.id === 'string' ? profile.id : '';
        return name.trim();
      }
      return '';
    })
    .filter(Boolean);
}

export function normalizeScopeLabel(entry: unknown): string {
  const normalizeRawScope = (raw: string) => {
    const normalized = raw.trim().replace(/^\/+|\/+$/g, '');
    return normalized.toLowerCase() === 'default' ? '' : normalized;
  };
  if (entry == null) return '';
  if (typeof entry === 'string') return normalizeRawScope(entry);
  if (typeof entry === 'object') {
    const record = entry as Record<string, unknown>;
    const raw = record.scope ?? record.name ?? record.value;
    if (typeof raw === 'string') return normalizeRawScope(raw);
  }
  return '';
}

export function buildEditorScopeList(secretsPayload: unknown, variablesPayload: unknown): string[] {
  const scopes = new Set<string>();
  scopes.add('');
  if (Array.isArray(secretsPayload)) {
    secretsPayload.forEach(entry => {
      scopes.add(normalizeScopeLabel(entry));
    });
  }
  if (Array.isArray(variablesPayload)) {
    variablesPayload.forEach(entry => {
      scopes.add(normalizeScopeLabel(entry));
    });
  }
  return Array.from(scopes)
    .map(scope => scope.replace(/^\/+|\/+$/g, ''))
    .sort((a, b) => a.localeCompare(b));
}

async function fetchOptionalJson(path: string, fallback: unknown = []) {
  const response = await apiClient.fetch(path);
  if (!response.ok) return fallback;
  return response.json();
}

async function fetchScopedList(path: string, scope: string): Promise<string[]> {
  const suffix = scope ? `?scope=${encodeURIComponent(scope)}` : '';
  return normalizeAutocompleteList(await fetchOptionalJson(`${path}${suffix}`));
}

export async function fetchEditorAutocompleteMetadata(
  options: EditorAutocompleteOptions = {}
): Promise<EditorAutocompleteMetadata> {
  const [secretsResp, variablesResp, stepsResp, secretScopesResp, variableScopesResp, agentProfilesResp, llmProfilesResp, mcpProfilesResp] = await Promise.all([
    fetchOptionalJson('/v1/secrets'),
    fetchOptionalJson('/v1/variables'),
    fetchOptionalJson('/v1/steps'),
    fetchOptionalJson('/v1/secrets/scopes'),
    fetchOptionalJson('/v1/variables/scopes'),
    options.includeAgentProfiles ? fetchOptionalJson('/v1/system/agent-profiles', null) : Promise.resolve(null),
    options.includeLLMProfiles ? fetchOptionalJson('/v1/system/llm-profiles', null) : Promise.resolve(null),
    options.includeMCPProfiles ? fetchOptionalJson('/v1/system/mcp/profiles', null) : Promise.resolve(null),
  ]);

  const scopeList = buildEditorScopeList(secretScopesResp, variableScopesResp);
  const [secretScopes, variableScopes] = await Promise.all([
    Promise.all(scopeList.map(async scope => ({ scope, items: await fetchScopedList('/v1/secrets', scope) }))),
    Promise.all(scopeList.map(async scope => ({ scope, items: await fetchScopedList('/v1/variables', scope) }))),
  ]);

  return {
    secrets: normalizeAutocompleteList(secretsResp),
    variables: normalizeAutocompleteList(variablesResp),
    reusableSteps: normalizeAutocompleteList(stepsResp),
    secretScopes,
    variableScopes,
    agentProfiles: normalizeProfilePayload(agentProfilesResp),
    llmProfiles: normalizeProfilePayload(llmProfilesResp),
    mcpProfiles: normalizeProfilePayload(mcpProfilesResp),
    fetchedAt: Date.now(),
    loading: false,
  };
}
