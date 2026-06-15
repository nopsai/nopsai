import { apiClient } from '../../lib/api.js';
import type {
  GitWebhookDelivery,
  GitWebhookSource,
  GitWebhookSourceRequest,
} from './model.js';

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await apiClient.fetch(path, { cache: 'no-store', ...init });
  if (!response.ok) {
    throw new Error(await readError(response, `Request failed (${response.status})`));
  }
  return (await response.json()) as T;
}

export async function fetchGitWebhookSources(): Promise<GitWebhookSource[]> {
  const payload = await requestJson<GitWebhookSource[]>('/v1/git-webhook-sources');
  return Array.isArray(payload) ? payload : [];
}

export function fetchGitWebhookSource(sourceID: string): Promise<GitWebhookSource> {
  return requestJson<GitWebhookSource>(`/v1/git-webhook-sources/${encodeURIComponent(sourceID)}`);
}

export async function fetchGitWebhookDeliveries(sourceID: string): Promise<GitWebhookDelivery[]> {
  const payload = await requestJson<GitWebhookDelivery[]>(
    `/v1/git-webhook-sources/${encodeURIComponent(sourceID)}/deliveries`
  );
  return Array.isArray(payload) ? payload : [];
}

export function saveGitWebhookSource(
  request: GitWebhookSourceRequest,
  existingID?: string
): Promise<GitWebhookSource> {
  const path = existingID
    ? `/v1/git-webhook-sources/${encodeURIComponent(existingID)}`
    : '/v1/git-webhook-sources';
  return requestJson<GitWebhookSource>(path, {
    method: existingID ? 'PUT' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  });
}

export async function deleteGitWebhookSource(sourceID: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/git-webhook-sources/${encodeURIComponent(sourceID)}`, {
    method: 'DELETE',
  });
  if (!response.ok && response.status !== 204) {
    throw new Error(await readError(response, `Unable to delete Git webhook source (${response.status})`));
  }
}
