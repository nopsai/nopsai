import { apiClient } from '../../lib/api.js';
import type { ExternalTrigger } from './model.js';

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

export async function fetchExternalTriggers(): Promise<ExternalTrigger[]> {
  const response = await apiClient.fetch('/v1/external-triggers', { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await readError(response, `Unable to load external triggers (${response.status})`));
  }
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}
