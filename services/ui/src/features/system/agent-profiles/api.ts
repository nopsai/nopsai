import { apiClient } from '../../../lib/api';
import {
  agentProfilePayloadFromForm,
  normalizeAgentProfilesPayload,
  type AgentProfileFormState,
  type AgentProfilesPayload,
} from './model';

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function fetchAgentProfiles(): Promise<AgentProfilesPayload> {
  const response = await apiClient.fetch('/v1/system/agent-profiles', { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to load agent profiles (${response.status})`));
  }
  return normalizeAgentProfilesPayload(await response.json());
}

export async function createAgentProfile(form: AgentProfileFormState): Promise<AgentProfilesPayload> {
  const payload = agentProfilePayloadFromForm(form);
  const response = await apiClient.fetch('/v1/system/agent-profiles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to create agent profile (${response.status})`));
  }
  return normalizeAgentProfilesPayload(await response.json());
}

export async function saveAgentProfile(form: AgentProfileFormState): Promise<AgentProfilesPayload> {
  const payload = agentProfilePayloadFromForm(form);
  const response = await apiClient.fetch(`/v1/system/agent-profiles/${encodeURIComponent(payload.id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to save agent profile (${response.status})`));
  }
  return normalizeAgentProfilesPayload(await response.json());
}

export async function setDefaultAgentProfile(defaultProfile: string): Promise<AgentProfilesPayload> {
  const response = await apiClient.fetch('/v1/system/agent-profiles/default', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ default_profile: defaultProfile.trim() }),
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to set default agent profile (${response.status})`));
  }
  return normalizeAgentProfilesPayload(await response.json());
}

export async function deleteAgentProfile(
  id: string,
  opts?: { force?: boolean }
): Promise<{ status: 'deleted' } | { status: 'conflict'; references: string[] }> {
  const params = new URLSearchParams();
  if (opts?.force) params.set('force', 'true');
  const suffix = params.toString() ? `?${params.toString()}` : '';
  const response = await apiClient.fetch(`/v1/system/agent-profiles/${encodeURIComponent(id)}${suffix}`, { method: 'DELETE' });
  if (response.status === 409) {
    const conflict = await response.json().catch(() => null);
    const references = Array.isArray(conflict?.references) ? conflict.references.map((item: unknown) => String(item)) : [];
    return { status: 'conflict', references };
  }
  if (!response.ok && response.status !== 204) {
    throw new Error(await readError(response, `Failed to delete agent profile (${response.status})`));
  }
  return { status: 'deleted' };
}
