import { apiClient } from '../../../lib/api';
import type { BootstrapResponse, SetupBootstrapRequest, SetupLicenseDocument, SetupStatus } from './model';

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

export async function bootstrapSetup(payload: SetupBootstrapRequest): Promise<BootstrapResponse> {
  return (await fetchJson('/v1/setup/bootstrap', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })) as BootstrapResponse;
}

export async function fetchSetupLicense(): Promise<SetupLicenseDocument> {
  return (await fetchJson('/v1/setup/license')) as SetupLicenseDocument;
}

/**
 * The digest is echoed back so the server refuses acceptance of a stale copy
 * held by a browser tab that was open across an upgrade.
 */
export async function acceptSetupLicense(documentSha256: string): Promise<SetupLicenseDocument> {
  return (await fetchJson('/v1/setup/license/accept', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ accept: true, document_sha256: documentSha256 }),
  })) as SetupLicenseDocument;
}
