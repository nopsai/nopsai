import { apiClient } from '../../lib/api.js';
import { encodeId } from './model.js';

export type StepListItem = {
  id: string;
  source?: string;
  updatedAt?: string;
};

export type StepUsageItem = {
  identifier: string;
  name: string;
  path: string;
  source: string;
  description?: string;
};

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export function normalizeStepListPayload(payload: unknown): StepListItem[] {
  const normalized: StepListItem[] = Array.isArray(payload)
    ? payload
        .map((item: unknown): StepListItem | null => {
          if (typeof item === 'string') return { id: item.trim() };
          if (item && typeof item === 'object') {
            const record = item as Record<string, unknown>;
            const id = typeof record.identifier === 'string' ? record.identifier : typeof record.id === 'string' ? record.id : '';
            const source = typeof record.source === 'string' ? record.source : undefined;
            const updatedAt = typeof record.updated_at === 'string' ? record.updated_at : undefined;
            return id ? { id, source, updatedAt } : null;
          }
          return null;
        })
        .filter((item): item is StepListItem => Boolean(item))
    : [];
  return normalized.sort((a, b) => a.id.localeCompare(b.id));
}

export function normalizeStepUsagePayload(payload: unknown): StepUsageItem[] {
  const list: StepUsageItem[] = Array.isArray(payload)
    ? payload
        .map((item: unknown): StepUsageItem | null => {
          if (!item || typeof item !== 'object') return null;
          const record = item as Record<string, unknown>;
          const identifier = typeof record.identifier === 'string' ? record.identifier : '';
          if (!identifier) return null;
          const name = typeof record.name === 'string' ? record.name : '';
          const path = typeof record.path === 'string' ? record.path : '';
          const source = typeof record.source === 'string' ? record.source : 'database';
          const description = typeof record.description === 'string' ? record.description : undefined;
          return { identifier, name, path, source, description };
        })
        .filter((item): item is StepUsageItem => Boolean(item))
    : [];
  return list.sort((a, b) => a.identifier.localeCompare(b.identifier));
}

export async function checkStepPermission(action: string, resourceID: string): Promise<boolean> {
  const params = new URLSearchParams({
    action,
    resource_type: 'step',
    resource_id: resourceID,
  });
  const response = await apiClient.fetch(`/v1/access/effective-permissions?${params.toString()}`);
  if (!response.ok) return false;
  const payload = await response.json();
  return Boolean(payload?.allowed);
}

export async function fetchStepList(): Promise<StepListItem[]> {
  const response = await apiClient.fetch('/v1/steps?include_source=true');
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to load steps (${response.status})`));
  }
  return normalizeStepListPayload(await response.json());
}

export async function fetchStepYaml(stepID: string): Promise<string> {
  const response = await apiClient.fetch(`/v1/steps/${encodeId(stepID)}`);
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to fetch step (${response.status})`));
  }
  return response.text();
}

export async function fetchStepUsage(stepID: string): Promise<StepUsageItem[]> {
  const response = await apiClient.fetch(`/v1/steps/${encodeId(stepID)}/usage`);
  if (!response.ok) {
    if (response.status === 404) return [];
    throw new Error(await readError(response, `Failed to load usage (${response.status})`));
  }
  return normalizeStepUsagePayload(await response.json());
}

export async function saveStepYaml(stepID: string, rawYaml: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/steps/${encodeId(stepID)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/x-yaml' },
    body: rawYaml,
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to save step (${response.status})`));
  }
}

export async function deleteStep(stepID: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/steps/${encodeId(stepID)}`, { method: 'DELETE' });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to delete step (${response.status})`));
  }
}
