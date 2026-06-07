import { apiClient } from '../../lib/api';

export type ScopedResourceKind = 'variable' | 'secret';

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function checkScopePermission(action: string, resourceType: string, resourceID: string): Promise<boolean> {
  const params = new URLSearchParams({ action, resource_type: resourceType, resource_id: resourceID });
  const response = await apiClient.fetch(`/v1/access/effective-permissions?${params.toString()}`);
  return response.ok ? Boolean((await response.json())?.allowed) : false;
}

export async function fetchScopeCatalogs(): Promise<{ secrets: unknown; variables: unknown }> {
  const [secretResponse, variableResponse] = await Promise.all([
    apiClient.fetch('/v1/secrets/scopes'),
    apiClient.fetch('/v1/variables/scopes'),
  ]);
  return {
    secrets: secretResponse.ok ? await secretResponse.json() : [],
    variables: variableResponse.ok ? await variableResponse.json() : [],
  };
}

export async function fetchScopedItems(kind: ScopedResourceKind, scope: string): Promise<unknown> {
  const plural = kind === 'variable' ? 'variables' : 'secrets';
  const query = scope ? `?scope=${encodeURIComponent(scope)}&include_source=true` : '?include_source=true';
  const response = await apiClient.fetch(`/v1/${plural}${query}`);
  if (!response.ok) throw new Error(await responseError(response, `Failed to load ${plural} (${response.status})`));
  return response.json();
}

export async function fetchScopeUsageCatalogs(): Promise<{ pipelines: unknown; triggers: unknown }> {
  const [pipelineResponse, triggerResponse] = await Promise.all([
    apiClient.fetch('/v1/pipelines?include_source=true'),
    apiClient.fetch('/v1/overrides?include_source=true'),
  ]);
  return {
    pipelines: pipelineResponse.ok ? await pipelineResponse.json() : [],
    triggers: triggerResponse.ok ? await triggerResponse.json() : [],
  };
}

export async function fetchScopeUsagePipelineYaml(identifier: string): Promise<string | null> {
  const encodedIdentifier = identifier
    .split('/')
    .filter(Boolean)
    .map(encodeURIComponent)
    .join('/');
  if (!encodedIdentifier) return null;
  const response = await apiClient.fetch(`/v1/pipelines/${encodedIdentifier}`);
  return response.ok ? response.text() : null;
}

export async function fetchScopeUsageTriggerYaml(slug: string): Promise<string | null> {
  const [owner, repository] = slug.split('/');
  if (!owner || !repository) return null;
  const response = await apiClient.fetch(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repository)}`);
  return response.ok ? response.text() : null;
}

export function scopedResourcePath(kind: ScopedResourceKind, scope: string, name: string, repositorySlug = '') {
  const plural = kind === 'variable' ? 'variables' : 'secrets';
  const [owner, repository] = repositorySlug.split('/');
  const base =
    owner && repository
      ? `/v1/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repository)}/${plural}/${encodeURIComponent(name)}`
      : `/v1/${plural}/${encodeURIComponent(name)}`;
  return scope ? `${base}?scope=${encodeURIComponent(scope)}` : base;
}

export async function fetchVariableValue(path: string): Promise<string> {
  const response = await apiClient.fetch(path);
  if (response.status === 404) return '';
  if (!response.ok) throw new Error(await responseError(response, `Failed to load variable (${response.status})`));
  const payload = await response.json();
  return payload && typeof payload === 'object' && payload.value != null ? String(payload.value) : '';
}

export async function saveScopedValue(path: string, value: string, kind: ScopedResourceKind): Promise<void> {
  const response = await apiClient.fetch(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value }),
  });
  if (!response.ok) throw new Error(await responseError(response, `Failed to save ${kind} (${response.status})`));
}

export async function deleteScopedValue(path: string): Promise<void> {
  const response = await apiClient.fetch(path, { method: 'DELETE' });
  if (!response.ok) throw new Error(await responseError(response, `Failed to delete (${response.status})`));
}

export async function encryptSecretValue(value: string): Promise<string> {
  const response = await apiClient.fetch('/v1/secrets/encrypt', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value }),
  });
  if (!response.ok) throw new Error(await responseError(response, `Failed to encrypt secret (${response.status})`));
  const payload = await response.json();
  const encryptedValue = typeof payload?.encrypted_value === 'string' ? payload.encrypted_value : '';
  if (!encryptedValue) throw new Error('Encryption response did not include a value.');
  return encryptedValue;
}
