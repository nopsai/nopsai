import { apiClient } from '../../lib/api.js';
import { fetchPipelineRunTeamPaths } from '../../lib/resourceTeams.js';
import { fetchPipelineYaml } from '../pipelines/api.js';
import {
  dashboardRequestFromForm,
  normalizeRunScope,
  normalizeDashboardRefresh,
  normalizeDashboardRefreshSchedule,
  normalizeDashboardPublication,
  normalizeDashboardSection,
  normalizeDashboardSource,
  normalizeDashboardSummary,
  normalizeDashboardView,
  refreshScheduleRequestFromForm,
  refreshRequestFromForm,
  sectionRequestFromForm,
  sourceRequestFromForm,
  type DashboardEvent,
  type DashboardFormState,
  type DashboardPublication,
  type DashboardRefresh,
  type DashboardRefreshSchedule,
  type DashboardRefreshScheduleFormState,
  type DashboardRefreshFormState,
  type DashboardSection,
  type DashboardSectionSeed,
  type DashboardSectionFormState,
  type DashboardSource,
  type DashboardSourceFormState,
  type DashboardSummary,
  type DashboardView,
} from './model.js';
import {
  parseDashboardPipelineOutputOptions,
  type DashboardPipelineCatalogItem,
  type DashboardPipelineOutputOption,
} from './sourceOptions.js';

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

export function saveDashboardRefreshSchedule(
  dashboardID: string,
  form: DashboardRefreshScheduleFormState,
  schedule?: DashboardRefreshSchedule
): Promise<DashboardRefreshSchedule> {
  return requestJson<unknown>(
    schedule
      ? `/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh-schedules/${encodeURIComponent(schedule.id)}`
      : `/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh-schedules`,
    {
      method: schedule ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(refreshScheduleRequestFromForm(form)),
    }
  ).then(normalizeDashboardRefreshSchedule);
}

export async function deleteDashboardRefreshSchedule(dashboardID: string, scheduleID: string): Promise<void> {
  const response = await apiClient.fetch(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh-schedules/${encodeURIComponent(scheduleID)}`,
    { method: 'DELETE' }
  );
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete refresh schedule (${response.status})`));
  }
}

export function setDashboardRefreshScheduleEnabled(
  dashboardID: string,
  scheduleID: string,
  enabled: boolean
): Promise<DashboardRefreshSchedule> {
  return requestJson<unknown>(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/refresh-schedules/${encodeURIComponent(scheduleID)}/${enabled ? 'enable' : 'disable'}`,
    { method: 'POST' }
  ).then(normalizeDashboardRefreshSchedule);
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

export function saveDashboard(
  form: DashboardFormState,
  dashboard?: DashboardSummary,
  options: { sections?: DashboardSectionSeed[] } = {}
): Promise<DashboardSummary> {
  return requestJson<unknown>(dashboard ? `/v1/dashboards/${encodeURIComponent(dashboard.id)}` : '/v1/dashboards', {
    method: dashboard ? 'PUT' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(dashboardRequestFromForm(form, { includeSections: !dashboard, sections: options.sections })),
  }).then(normalizeDashboardSummary);
}

export async function deleteDashboard(dashboardID: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/dashboards/${encodeURIComponent(dashboardID)}`, { method: 'DELETE' });
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete dashboard (${response.status})`));
  }
}

export async function fetchDashboardSections(dashboardID: string): Promise<DashboardSection[]> {
  const payload = await requestJson<unknown[]>(`/v1/dashboards/${encodeURIComponent(dashboardID)}/sections`);
  return Array.isArray(payload) ? payload.map(normalizeDashboardSection) : [];
}

export function saveDashboardSection(
  dashboardID: string,
  form: DashboardSectionFormState,
  section?: DashboardSection
): Promise<DashboardSection> {
  return requestJson<unknown>(
    section
      ? `/v1/dashboards/${encodeURIComponent(dashboardID)}/sections/${encodeURIComponent(section.id)}`
      : `/v1/dashboards/${encodeURIComponent(dashboardID)}/sections`,
    {
      method: section ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(sectionRequestFromForm(form)),
    }
  ).then(normalizeDashboardSection);
}

export async function deleteDashboardSection(dashboardID: string, sectionID: string): Promise<void> {
  const response = await apiClient.fetch(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/sections/${encodeURIComponent(sectionID)}`,
    { method: 'DELETE' }
  );
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete section (${response.status})`));
  }
}

export async function deleteDashboardPublication(dashboardID: string, publicationID: string): Promise<void> {
  const response = await apiClient.fetch(
    `/v1/dashboards/${encodeURIComponent(dashboardID)}/publications/${encodeURIComponent(publicationID)}`,
    { method: 'DELETE' }
  );
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete dashboard entry (${response.status})`));
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

type ScopeListEntry = string | { scope?: string; name?: string; value?: string };

export async function fetchDashboardMetadata(): Promise<{ teams: string[]; pipelines: string[]; scopes: string[] }> {
  const [teams, pipelines, secretScopes, variableScopes] = await Promise.all([
    fetchPipelineRunTeamPaths().catch(() => []),
    requestJson<Array<string | { id?: string }>>('/v1/pipelines').catch(() => []),
    requestJson<ScopeListEntry[]>('/v1/secrets/scopes').catch(() => []),
    requestJson<ScopeListEntry[]>('/v1/variables/scopes').catch(() => []),
  ]);
  return {
    teams,
    pipelines: pipelines
      .map(item => (typeof item === 'string' ? item : item.id || ''))
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b)),
    scopes: normalizeScopeList([...secretScopes, ...variableScopes]),
  };
}

export async function fetchDashboardPipelineOutputs(pipelineID: string): Promise<DashboardPipelineOutputOption[]> {
  const normalized = pipelineID.trim();
  if (!normalized) return [];
  const rawYaml = await fetchPipelineYaml(normalized);
  return parseDashboardPipelineOutputOptions(rawYaml);
}

export async function fetchDashboardPipelineCatalog(pipelineIDs: string[]): Promise<DashboardPipelineCatalogItem[]> {
  const uniquePipelineIDs = Array.from(new Set(pipelineIDs.map(id => id.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
  const items = await Promise.all(
    uniquePipelineIDs.map(async id => {
      try {
        const outputs = await fetchDashboardPipelineOutputs(id);
        return outputs.length > 0 ? { id, outputs } : null;
      } catch {
        return null;
      }
    })
  );
  return items.filter((item): item is DashboardPipelineCatalogItem => Boolean(item));
}

export function normalizePublication(payload: unknown): DashboardPublication {
  return normalizeDashboardPublication(payload);
}

function normalizeScopeList(entries: ScopeListEntry[]): string[] {
  const scopes = new Set<string>(['']);
  for (const entry of entries) {
    if (typeof entry === 'string') {
      scopes.add(normalizeRunScope(entry));
      continue;
    }
    if (!entry || typeof entry !== 'object') continue;
    scopes.add(normalizeRunScope(entry.scope || entry.name || entry.value || ''));
  }
  return Array.from(scopes).sort((a, b) => a.localeCompare(b));
}
