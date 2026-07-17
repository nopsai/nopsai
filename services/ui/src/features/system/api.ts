import { apiClient } from '../../lib/api.js';

export async function fetchSystemJson(path: string, init?: RequestInit): Promise<unknown> {
  const response = await apiClient.fetch(path, init);
  if (response.status === 204) return null;
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed (${response.status})`);
  }
  const contentType = response.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    return response.json();
  }
  return response.text();
}
