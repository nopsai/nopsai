import { apiClient } from '../../lib/api.js';

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return text ? `${text} (${response.status})` : fallback;
}

export async function requestMonitoringJson<T>(path: string): Promise<T> {
  const response = await apiClient.fetch(path, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await responseError(response, `Request failed (${response.status})`));
  }
  return (await response.json()) as T;
}

export async function sendMonitoringJson<T>(path: string, method: string, body?: unknown): Promise<T> {
  const response = await apiClient.fetch(path, {
    method,
    cache: 'no-store',
    headers: body == null ? undefined : { 'Content-Type': 'application/json' },
    body: body == null ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(await responseError(response, `Request failed (${response.status})`));
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}
