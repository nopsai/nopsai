import { apiClient } from '../lib/api';
import type { RunDetail, RunGroup, RunListItem } from './types';

async function fetchJson<T>(path: string): Promise<T | null> {
  try {
    const response = await apiClient.fetch(path, { cache: 'no-store' });
    if (!response.ok) return null;
    return (await response.json()) as T;
  } catch {
    return null;
  }
}

export async function fetchRunSidebarGroups(): Promise<RunGroup[]> {
  const payload = await fetchJson<RunGroup[]>('/v1/groups');
  return Array.isArray(payload) ? payload : [];
}

export async function fetchRunSidebarRecentRuns(offset: number, limit: number): Promise<RunListItem[]> {
  const payload = await fetchJson<RunListItem[]>(`/v1/runs?offset=${offset}&limit=${limit}`);
  return Array.isArray(payload) ? payload : [];
}

export async function fetchRunSidebarRepositoryRuns(groupId: number): Promise<Record<string, RunListItem[]> | null> {
  return fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${groupId}`);
}

export async function fetchRunSidebarDetail(runId: string): Promise<RunDetail | null> {
  return fetchJson<RunDetail>(`/v1/runs/${encodeURIComponent(runId)}`);
}
