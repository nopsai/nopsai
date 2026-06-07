import { apiClient } from '../../../lib/api';
import type {
  ConfigFormState,
  ConfigRepositoryFormState,
  NotificationMailSettingsFormState,
} from './model';
import {
  configRepositoryPayloadFromForm,
  mailSettingsPayloadFromForm,
  normalizeConfigRepository,
  normalizeNotificationMailSettings,
  normalizeSystemConfigPayload,
  systemConfigPayloadFromForm,
} from './model';
import type {
  ConfigRepositoryCommitResponse,
  ConfigRepositoryDriftResponse,
  ConfigRepositoryWriteFile,
} from '../../../lib/configRepositoryDrift';
import { fetchSystemJson } from '../api';

export async function fetchRuntimeConfig() {
  return normalizeSystemConfigPayload(await fetchSystemJson('/v1/system/config'));
}

export async function saveRuntimeConfig(config: ConfigFormState) {
  return normalizeSystemConfigPayload(
    await fetchSystemJson('/v1/system/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(systemConfigPayloadFromForm(config)),
    })
  );
}

export async function fetchMailSettings() {
  return normalizeNotificationMailSettings(await fetchSystemJson('/v1/system/notifications/mail'));
}

export async function saveMailSettings(form: NotificationMailSettingsFormState) {
  return normalizeNotificationMailSettings(
    await fetchSystemJson('/v1/system/notifications/mail', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(mailSettingsPayloadFromForm(form)),
    })
  );
}

export async function sendMailSettingsTest(to: string) {
  await fetchSystemJson('/v1/system/notifications/mail/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ to }),
  });
}

export async function fetchGlobalConfigRepository() {
  const response = await apiClient.fetch('/v1/system/config-repo', { cache: 'no-store' });
  if (response.status === 404) return null;
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Unable to load global config repository (${response.status})`);
  }
  return normalizeConfigRepository(await response.json());
}

export async function saveGlobalConfigRepository(form: ConfigRepositoryFormState) {
  return normalizeConfigRepository(
    await fetchSystemJson('/v1/system/config-repo', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(configRepositoryPayloadFromForm(form)),
    })
  );
}

export async function deleteGlobalConfigRepository() {
  await fetchSystemJson('/v1/system/config-repo', { method: 'DELETE' });
}

export async function syncGlobalConfigRepository() {
  await fetchSystemJson('/v1/system/config-repo/sync', { method: 'POST' });
}

export async function fetchGlobalConfigRepositoryDrift() {
  return fetchSystemJson('/v1/system/config-repo/drift') as Promise<ConfigRepositoryDriftResponse>;
}

export async function pushGlobalConfigRepositoryDrift(message: string, files: ConfigRepositoryWriteFile[]) {
  return fetchSystemJson('/v1/system/config-repo/write', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, files }),
  }) as Promise<ConfigRepositoryCommitResponse>;
}
