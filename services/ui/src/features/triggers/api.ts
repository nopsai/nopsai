import { apiClient } from '../../lib/api';
import {
  buildTriggerSummary,
  parseTriggerOverrideList,
  parseTriggerYaml,
  splitTriggerSlug,
  type TriggerDetail,
  type TriggerListItem,
  type TriggerRun,
} from './model';

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function checkTriggerPermission(action: string, resourceID: string, teamPath = ''): Promise<boolean> {
  const params = new URLSearchParams({ action, resource_type: 'trigger', resource_id: resourceID });
  if (teamPath.trim()) params.set('team_path', teamPath.trim());
  const response = await apiClient.fetch(`/v1/access/effective-permissions?${params.toString()}`);
  return response.ok ? Boolean((await response.json())?.allowed) : false;
}

export async function fetchTriggers(): Promise<TriggerListItem[]> {
  const response = await apiClient.fetch('/v1/overrides?include_source=true');
  if (!response.ok) throw new Error(await responseError(response, `Failed to load triggers (${response.status})`));
  return parseTriggerOverrideList(await response.json());
}

export async function fetchTriggerDetail(slug: string, source?: string): Promise<TriggerDetail> {
  const { owner, repo } = splitTriggerSlug(slug);
  const response = await apiClient.fetch(
    `/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}?format=json`,
    { headers: { Accept: 'application/json' } }
  );
  if (!response.ok) throw new Error(await responseError(response, `Failed to load trigger (${response.status})`));
  const contentType = response.headers.get('Content-Type') || '';
  if (contentType.includes('application/json')) {
    const payload = await response.json();
    const rawYaml = typeof payload.raw_yaml === 'string' ? payload.raw_yaml : '';
    return {
      slug: typeof payload.slug === 'string' ? payload.slug : slug,
      source: typeof payload.source === 'string' ? payload.source : source,
      provider: typeof payload.provider === 'string' ? payload.provider : undefined,
      teamPath: typeof payload.team_path === 'string' ? payload.team_path : undefined,
      management: typeof payload.management === 'string' ? payload.management : undefined,
      webhookSourceID: typeof payload.webhook_source_id === 'string' ? payload.webhook_source_id : undefined,
      webhookSourceName: typeof payload.webhook_source_name === 'string' ? payload.webhook_source_name : undefined,
      ingress: typeof payload.ingress === 'string' ? payload.ingress : undefined,
      allowlistStatus: typeof payload.allowlist_status === 'string' ? payload.allowlist_status : undefined,
      repositoryForWebhook: typeof payload.repository_for_webhook === 'string' ? payload.repository_for_webhook : undefined,
      rawYaml,
      summary: buildTriggerSummary(parseTriggerYaml(rawYaml)),
    };
  }
  const rawYaml = await response.text();
  return { slug, source, rawYaml, summary: buildTriggerSummary(parseTriggerYaml(rawYaml)) };
}

export async function fetchTriggerRuns(): Promise<TriggerRun[]> {
  const response = await apiClient.fetch('/v1/runs');
  if (!response.ok) throw new Error(await responseError(response, `Failed to load runs (${response.status})`));
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}

export async function fetchTriggerAutocompleteResources(): Promise<{ pipelines: unknown; scopes: unknown }> {
  const [pipelineResponse, scopeResponse] = await Promise.all([
    apiClient.fetch('/v1/pipelines?include_source=true'),
    apiClient.fetch('/v1/variables/scopes'),
  ]);
  return {
    pipelines: pipelineResponse.ok ? await pipelineResponse.json() : [],
    scopes: scopeResponse.ok ? await scopeResponse.json() : [],
  };
}

export async function fetchTriggerPipelineYaml(identifier: string): Promise<string | null> {
  const encodedIdentifier = identifier
    .split('/')
    .filter(Boolean)
    .map(encodeURIComponent)
    .join('/');
  if (!encodedIdentifier) return null;
  const response = await apiClient.fetch(`/v1/pipelines/${encodedIdentifier}`);
  return response.ok ? response.text() : null;
}

export async function saveTrigger(slug: string, rawYaml: string): Promise<void> {
  const { owner, repo } = splitTriggerSlug(slug);
  const response = await apiClient.fetch(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/x-yaml' },
    body: rawYaml,
  });
  if (!response.ok) throw new Error(await responseError(response, `Failed to save trigger (${response.status})`));
}

export async function deleteTrigger(slug: string): Promise<void> {
  const { owner, repo } = splitTriggerSlug(slug);
  const response = await apiClient.fetch(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`, { method: 'DELETE' });
  if (!response.ok && response.status !== 204) {
    throw new Error(await responseError(response, `Failed to delete trigger (${response.status})`));
  }
}
