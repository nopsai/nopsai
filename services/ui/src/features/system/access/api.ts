import { fetchSystemJson } from '../api';
import {
  identityProviderPayloadFromForm,
  normalizeIdentityProvidersState,
  type IdentityProviderFormState,
  type IdentityProviderSettings,
  type IdentityProvidersState,
} from './model';
import {
  buildAccessResourceCatalog,
  type AccessResourceCatalog,
  type AccessResourceCatalogSources,
} from './resourceCatalog';

type CatalogSourceKey = keyof AccessResourceCatalogSources;

const CATALOG_REQUESTS: Array<{ key: CatalogSourceKey; path: string }> = [
  { key: 'teams', path: '/v1/teams?include=applications' },
  { key: 'pipelines', path: '/v1/pipelines' },
  { key: 'triggers', path: '/v1/overrides' },
  { key: 'externalTriggers', path: '/v1/external-triggers' },
  { key: 'gitWebhookSources', path: '/v1/git-webhook-sources' },
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
    sources[request.key] = Array.isArray(result.value) ? result.value : [];
  });

  return buildAccessResourceCatalog(sources);
}

function normalizeTeamCatalogPayload(value: unknown): unknown[] {
  if (Array.isArray(value)) return value;
  const record = value && typeof value === 'object' ? value as { teams?: unknown[]; applications?: unknown[] } : null;
  const teams = Array.isArray(record?.teams) ? record.teams : [];
  const applications = Array.isArray(record?.applications) ? record.applications : [];
  return [
    ...teams.map(team => normalizeTeamCatalogEntry(team, false)),
    ...applications.map(application => normalizeTeamCatalogEntry(application, true)),
  ].filter(Boolean);
}

function normalizeTeamCatalogEntry(value: unknown, application: boolean): unknown {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  return {
    id: record.id,
    name: record.slug || record.name || record.display_name || record.repository_full_name,
    parent_id: application ? record.team_id ?? record.parent_id : record.parent_team_id ?? record.parent_id,
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
