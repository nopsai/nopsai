import { apiClient } from '../../../lib/api';
import {
  credentialPayloadFromForm,
  normalizeCredential,
  normalizeCredentialsPayload,
  type CredentialFormState,
  type CredentialRecord,
} from './model';

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

async function readCredential(response: Response, fallback: string): Promise<CredentialRecord> {
  if (!response.ok) {
    throw new Error(await readError(response, fallback));
  }
  const credential = normalizeCredential(await response.json());
  if (!credential) throw new Error('The server returned an invalid credential record.');
  return credential;
}

export async function fetchCredentials(): Promise<CredentialRecord[]> {
  const response = await apiClient.fetch('/v1/system/credentials', { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to load credentials (${response.status})`));
  }
  return normalizeCredentialsPayload(await response.json());
}

export async function fetchCredential(id: string): Promise<CredentialRecord> {
  const response = await apiClient.fetch(`/v1/system/credentials/${encodeURIComponent(id)}`, { cache: 'no-store' });
  return readCredential(response, `Failed to load credential (${response.status})`);
}

export async function createCredential(form: CredentialFormState): Promise<CredentialRecord> {
  const response = await apiClient.fetch('/v1/system/credentials', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(credentialPayloadFromForm(form)),
  });
  return readCredential(response, `Failed to create credential (${response.status})`);
}

export async function rotateCredential(id: string, value: string): Promise<CredentialRecord> {
  const response = await apiClient.fetch(`/v1/system/credentials/${encodeURIComponent(id)}/value`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value }),
  });
  return readCredential(response, `Failed to rotate credential (${response.status})`);
}

export async function activateCredentialVersion(id: string, version: number): Promise<CredentialRecord> {
  const response = await apiClient.fetch(
    `/v1/system/credentials/${encodeURIComponent(id)}/versions/${version}/activate`,
    { method: 'POST' }
  );
  return readCredential(response, `Failed to activate credential version (${response.status})`);
}

export async function disableCredential(id: string): Promise<CredentialRecord> {
  const response = await apiClient.fetch(`/v1/system/credentials/${encodeURIComponent(id)}/disable`, { method: 'POST' });
  return readCredential(response, `Failed to disable credential (${response.status})`);
}

export async function enableCredential(id: string): Promise<CredentialRecord> {
  const response = await apiClient.fetch(`/v1/system/credentials/${encodeURIComponent(id)}/enable`, { method: 'POST' });
  return readCredential(response, `Failed to enable credential (${response.status})`);
}

export async function deleteCredentialVersion(id: string, version: number): Promise<CredentialRecord> {
  const response = await apiClient.fetch(
    `/v1/system/credentials/${encodeURIComponent(id)}/versions/${version}`,
    { method: 'DELETE' }
  );
  return readCredential(response, `Failed to delete credential version (${response.status})`);
}

export async function deleteCredential(id: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/system/credentials/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (!response.ok && response.status !== 204) {
    throw new Error(await readError(response, `Failed to delete credential (${response.status})`));
  }
}
