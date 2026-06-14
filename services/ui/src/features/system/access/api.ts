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
  { key: 'groups', path: '/v1/groups' },
  { key: 'pipelines', path: '/v1/pipelines' },
  { key: 'triggers', path: '/v1/overrides' },
  { key: 'externalTriggers', path: '/v1/external-triggers' },
  { key: 'secretScopes', path: '/v1/secrets/scopes' },
  { key: 'variableScopes', path: '/v1/variables/scopes' },
];

export async function fetchAccessResourceCatalog(): Promise<AccessResourceCatalog> {
  const results = await Promise.allSettled(CATALOG_REQUESTS.map(request => fetchSystemJson(request.path)));
  const sources: AccessResourceCatalogSources = {
    groups: [],
    pipelines: [],
    triggers: [],
    externalTriggers: [],
    secretScopes: [],
    variableScopes: [],
  };

  results.forEach((result, index) => {
    const request = CATALOG_REQUESTS[index];
    if (result.status === 'rejected') {
      console.error(`Failed to load Access resource catalog source ${request.path}`, result.reason);
      return;
    }
    sources[request.key] = Array.isArray(result.value) ? result.value : [];
  });

  return buildAccessResourceCatalog(sources);
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
