import { apiClient } from '../../../lib/api';
import {
  llmProfilePayloadFromForm,
  normalizeLLMProfilesPayload,
  type LLMProfileFormState,
  type LLMProfileRecord,
  type LLMProfilesPayload,
} from './model';

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function fetchLLMProfiles(): Promise<LLMProfilesPayload> {
  const response = await apiClient.fetch('/v1/system/llm-profiles', { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to load LLM profiles (${response.status})`));
  }
  return normalizeLLMProfilesPayload(await response.json());
}

export async function saveLLMProfile(form: LLMProfileFormState): Promise<{ name: string; payload: LLMProfilesPayload }> {
  const next = llmProfilePayloadFromForm(form);
  const response = await apiClient.fetch(`/v1/system/llm-profiles/${encodeURIComponent(next.name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(next),
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to save LLM profile (${response.status})`));
  }
  return {
    name: next.name,
    payload: normalizeLLMProfilesPayload(await response.json()),
  };
}

export async function saveDefaultLLMProfile(nextDefault: string, profiles: LLMProfileRecord[]): Promise<LLMProfilesPayload> {
  const response = await apiClient.fetch('/v1/system/llm-profiles', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      default_profile: nextDefault,
      profiles,
    }),
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to update default profile (${response.status})`));
  }
  return normalizeLLMProfilesPayload(await response.json());
}

export async function deleteLLMProfile(
  name: string,
  opts?: { force?: boolean; migrateTo?: string }
): Promise<{ status: 'deleted' } | { status: 'conflict'; references: string[] }> {
  const params = new URLSearchParams();
  if (opts?.force) params.set('force', 'true');
  if (opts?.migrateTo) params.set('migrate_to', opts.migrateTo);
  const suffix = params.toString() ? `?${params.toString()}` : '';
  const response = await apiClient.fetch(`/v1/system/llm-profiles/${encodeURIComponent(name)}${suffix}`, { method: 'DELETE' });
  if (response.status === 409) {
    const conflict = await response.json().catch(() => null);
    const references = Array.isArray(conflict?.references) ? conflict.references.map((item: unknown) => String(item)) : [];
    return { status: 'conflict', references };
  }
  if (!response.ok && response.status !== 204) {
    throw new Error(await readError(response, `Failed to delete LLM profile (${response.status})`));
  }
  return { status: 'deleted' };
}

export async function testLLMProfile(name: string): Promise<string> {
  const response = await apiClient.fetch(`/v1/system/llm-profiles/${encodeURIComponent(name)}/test`, { method: 'POST' });
  if (!response.ok) {
    throw new Error(await readError(response, `Profile test failed (${response.status})`));
  }
  const result = await response.json();
  return typeof result?.reply === 'string' && result.reply.trim() ? result.reply : 'ok';
}
