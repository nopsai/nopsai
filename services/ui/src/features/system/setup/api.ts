import { apiClient } from '../../../lib/api';
import type { BootstrapResponse, SetupBootstrapRequest, SetupStatus, SetupTemplates } from './model';

async function fetchJson(path: string, init?: RequestInit): Promise<unknown> {
  const response = await apiClient.fetch(path, init);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed (${response.status})`);
  }
  return response.json();
}

export async function fetchSetupStatus(): Promise<SetupStatus> {
  return (await fetchJson('/v1/setup/status')) as SetupStatus;
}

export async function fetchSetupTemplates(params: URLSearchParams): Promise<SetupTemplates> {
  return (await fetchJson(`/v1/setup/templates?${params.toString()}`)) as SetupTemplates;
}

export async function downloadSetupTemplatesZip(params: URLSearchParams): Promise<Blob> {
  const response = await apiClient.fetch(`/v1/setup/templates.zip?${params.toString()}`);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Download failed (${response.status})`);
  }
  return response.blob();
}

export async function bootstrapSetup(payload: SetupBootstrapRequest): Promise<BootstrapResponse> {
  return (await fetchJson('/v1/setup/bootstrap', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })) as BootstrapResponse;
}
