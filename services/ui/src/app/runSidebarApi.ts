import { apiClient } from '../lib/api';
import { fetchTeams } from '../features/teams/api';
import type { RunDetail, RunTeam, RunListItem } from './types';

async function fetchJson<T>(path: string): Promise<T | null> {
  try {
    const response = await apiClient.fetch(path, { cache: 'no-store' });
    if (!response.ok) return null;
    return (await response.json()) as T;
  } catch {
    return null;
  }
}

export async function fetchRunSidebarTeams(): Promise<RunTeam[]> {
  try {
    const payload = await fetchTeams();
    return Array.isArray(payload) ? payload : [];
  } catch {
    return [];
  }
}

export async function fetchRunSidebarRecentRuns(offset: number, limit: number): Promise<RunListItem[]> {
  const payload = await fetchJson<RunListItem[]>(`/v1/runs?offset=${offset}&limit=${limit}`);
  return Array.isArray(payload) ? payload : [];
}

export async function fetchRunSidebarRepositoryRuns(teamId: number): Promise<Record<string, RunListItem[]> | null> {
  return fetchJson<Record<string, RunListItem[]>>(`/v1/runs?teamId=${teamId}`);
}

export async function fetchRunSidebarDetail(runId: string): Promise<RunDetail | null> {
  return fetchJson<RunDetail>(`/v1/runs/${encodeURIComponent(runId)}`);
}
