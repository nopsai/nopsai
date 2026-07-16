import { apiClient } from '../../lib/api';
import { fetchPipelineRunTeamPaths } from '../../lib/resourceTeams';
import {
  dashboardRequestFromForm,
  normalizeDashboardRefresh,
  normalizeDashboardRefreshSchedule,
  normalizeDashboardPublication,
  normalizeDashboardSource,
  normalizeDashboardSummary,
  normalizeDashboardView,
  refreshRequestFromForm,
  sourceRequestFromForm,
  type DashboardEvent,
  type DashboardFormState,
  type DashboardPublication,
  type DashboardRefresh,
  type DashboardRefreshSchedule,
  type DashboardRefreshFormState,
  type DashboardSource,
  type DashboardSourceFormState,
  type DashboardSummary,
  type DashboardView,
} from './model';

async function readResponseError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await apiClient.fetch(path, { cache: 'no-store', ...init });
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Request failed (${response.status})`));
  }
  return (await response.json()) as T;
}

export async function fetchDashboards(filters: { team?: string; query?: string } = {}): Promise<DashboardSummary[]> {
  const params = new URLSearchParams();
  if (filters.team) params.set('team', filters.team);
  if (filters.query) params.set('q', filters.query);
  const suffix = params.toString() ? `?${params.toString()}` : '';
  const payload = await requestJson<unknown[]>(`/v1/dashboards${suffix}`);
  return Array.isArray(payload) ? payload.map(normalizeDashboardSummary) : [];
}

export function fetchDashboardView(dashboardID: string): Promise<DashboardView> {
  return requestJson<unknown>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/view`).then(normalizeDashboardView);
}

export async function fetchDashboardHistory(dashboardID: string): Promise<DashboardEvent[]> {
  const payload = await requestJson<unknown[]>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/history`);
  return Array.isArray(payload) ? payload as DashboardEvent[] : [];
}

export async function fetchDashboardRefreshes(dashboardID: string): Promise<DashboardRefresh[]> {
  const payload = await requestJson<unknown[]>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/refreshes?limit=20`);
  return Array.isArray(payload) ? payload.map(normalizeDashboardRefresh) : [];
}

export async function fetchDashboardRefreshSchedules(dashboardID: string): Promise<DashboardRefreshSchedule[]> {
  const payload = await requestJson<unknown[]>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh-schedules`);
  return Array.isArray(payload) ? payload.map(normalizeDashboardRefreshSchedule) : [];
}

export function runDashboardRefreshSchedule(dashboardID: string, scheduleID: string): Promise<DashboardRefresh> {
  return requestJson<unknown>(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh-schedules/${encodeURIComponent(scheduleID)}/run`,
    { method: 'POST' }
  ).then(normalizeDashboardRefresh);
}

export function startDashboardRefresh(dashboardID: string, form: DashboardRefreshFormState): Promise<DashboardRefresh> {
  return requestJson<unknown>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': `dashboard-refresh-${dashboardID}-${Date.now()}`,
    },
    body: JSON.stringify(refreshRequestFromForm(form)),
  }).then(normalizeDashboardRefresh);
}

export function cancelDashboardRefresh(dashboardID: string, refreshID: string): Promise<DashboardRefresh> {
  return requestJson<unknown>(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/refreshes/${encodeURIComponent(refreshID)}/cancel`,
    { method: 'POST' }
  ).then(normalizeDashboardRefresh);
}

export function retryDashboardRefreshFailed(dashboardID: string, refreshID: string): Promise<DashboardRefresh> {
  return requestJson<unknown>(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/refreshes/${encodeURIComponent(refreshID)}/retry-failed`,
    { method: 'POST' }
  ).then(normalizeDashboardRefresh);
}

export function saveDashboard(form: DashboardFormState, dashboard?: DashboardSummary): Promise<DashboardSummary> {
  return requestJson<unknown>(dashboard ? `/v1/dashboards/${encodeURIComponent(dashboard.id)}` : '/v1/dashboards', {
    method: dashboard ? 'PUT' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(dashboardRequestFromForm(form)),
  }).then(normalizeDashboardSummary);
}

export async function deleteDashboard(dashboardID: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/dashboards/${encodeURIComponent(dashboardID)}`, { method: 'DELETE' });
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete dashboard (${response.status})`));
  }
}

export async function fetchDashboardSources(dashboardID: string): Promise<DashboardSource[]> {
  const payload = await requestJson<unknown[]>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/sources`);
  return Array.isArray(payload) ? payload.map(normalizeDashboardSource) : [];
}

export function saveDashboardSource(
  dashboardID: string,
  form: DashboardSourceFormState,
  source?: DashboardSource
): Promise<DashboardSource> {
  return requestJson<unknown>(
    source
      ? `/v1/dashboards/${encodeURIComponent(dashboardID)}/sources/${encodeURIComponent(source.id)}`
      : `/v1/dashboards/${encodeURIComponent(dashboardID)}/sources`,
    {
      method: source ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(sourceRequestFromForm(form)),
    }
  ).then(normalizeDashboardSource);
}

export async function deleteDashboardSource(dashboardID: string, sourceID: string): Promise<void> {
  const response = await apiClient.fetch(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/sources/${encodeURIComponent(sourceID)}`,
    { method: 'DELETE' }
  );
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete source (${response.status})`));
  }
}

export async function fetchDashboardMetadata(): Promise<{ teams: string[]; pipelines: string[] }> {
  const [teams, pipelines] = await Promise.all([
    fetchPipelineRunTeamPaths().catch(() => []),
    requestJson<Array<string | { id?: string }>>('/v1/pipelines').catch(() => []),
  ]);
  return {
    teams,
    pipelines: pipelines
      .map(item => (typeof item === 'string' ? item : item.id || ''))
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b)),
  };
}

export function normalizePublication(payload: unknown): DashboardPublication {
  return normalizeDashboardPublication(payload);
}
