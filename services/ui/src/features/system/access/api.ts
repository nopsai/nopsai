import { fetchSystemJson } from '../api.js';
import {
  identityProviderPayloadFromForm,
  normalizeIdentityProvidersState,
  type IdentityProviderFormState,
  type IdentityProviderSettings,
  type IdentityProvidersState,
} from './model.js';
import {
  buildAccessResourceCatalog,
  type AccessResourceCatalog,
  type AccessResourceCatalogSources,
} from './resourceCatalog.js';

type CatalogSourceKey = keyof AccessResourceCatalogSources;

const CATALOG_REQUESTS: Array<{ key: CatalogSourceKey; path: string }> = [
  { key: 'teams', path: '/v1/teams' },
  { key: 'pipelines', path: '/v1/pipelines' },
  { key: 'triggers', path: '/v1/overrides' },
  { key: 'externalTriggers', path: '/v1/external-triggers' },
  { key: 'gitWebhookSources', path: '/v1/git-webhook-sources' },
  { key: 'credentials', path: '/v1/system/credentials' },
  { key: 'secretScopes', path: '/v1/secrets/scopes' },
  { key: 'variableScopes', path: '/v1/variables/scopes' },
];

export async function fetchAccessResourceCatalog(): Promise<AccessResourceCatalog> {
  const results = await Promise.allSettled(CATALOG_REQUESTS.map(request => fetchSystemJson(request.path)));
  const sources: AccessResourceCatalogSources = {
    teams: [],
    pipelines: [],
    triggers: [],
    externalTriggers: [],
    gitWebhookSources: [],
    credentials: [],
    secretScopes: [],
    variableScopes: [],
  };

  results.forEach((result, index) => {
    const request = CATALOG_REQUESTS[index];
    if (result.status === 'rejected') {
      console.error(`Failed to load Access resource catalog source ${request.path}`, result.reason);
      return;
    }
    if (request.key === 'teams') {
      sources.teams = normalizeTeamCatalogPayload(result.value);
      return;
    }
    if (request.key === 'credentials') {
      const record = result.value && typeof result.value === 'object' ? result.value as { credentials?: unknown[] } : null;
      sources.credentials = Array.isArray(record?.credentials) ? record.credentials : [];
      return;
    }
    sources[request.key] = Array.isArray(result.value) ? result.value : [];
  });

  return buildAccessResourceCatalog(sources);
}

export function normalizeTeamCatalogPayload(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value
      .map(team => normalizeTeamCatalogEntry(team))
      .filter(Boolean);
  }
  const record = value && typeof value === 'object' ? value as { teams?: unknown[] } : null;
  const teams = Array.isArray(record?.teams) ? record.teams : [];
  return teams.map(team => normalizeTeamCatalogEntry(team)).filter(Boolean);
}

function normalizeTeamCatalogEntry(value: unknown): unknown {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  if (record.kind === 'app' || record.repo_url || record.repository_full_name) return null;
  const name = record.slug || record.name || record.display_name;
  if (String(name || '').includes('/')) return null;
  return {
    id: record.id,
    name,
    parent_id: record.parent_team_id ?? record.parent_id,
  };
}

export async function fetchIdentityProvidersState(): Promise<IdentityProvidersState> {
  const payload = await fetchSystemJson('/v1/admin/identity-providers');
  return normalizeIdentityProvidersState(payload);
}

export async function saveIdentityProviderSettings(
  settings: IdentityProviderSettings,
  mappings: Record<string, string>
): Promise<IdentityProvidersState> {
  const payload = await fetchSystemJson('/v1/admin/identity-providers', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      ...settings,
      domain_mappings: mappings,
    }),
  });
  return normalizeIdentityProvidersState(payload);
}

export async function saveIdentityProvider(form: IdentityProviderFormState): Promise<IdentityProvidersState> {
  const providerID = form.id.trim();
  if (!providerID) {
    throw new Error('Provider ID is required.');
  }
  const payload = await fetchSystemJson(`/v1/admin/identity-providers/${encodeURIComponent(providerID)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(identityProviderPayloadFromForm(form)),
  });
  return normalizeIdentityProvidersState(payload);
}

export async function deleteIdentityProvider(providerID: string): Promise<void> {
  await fetchSystemJson(`/v1/admin/identity-providers/${encodeURIComponent(providerID)}`, {
    method: 'DELETE',
  });
}
