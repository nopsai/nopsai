import { apiClient } from '../../lib/api';
import { fetchPipelineRunTeamPaths } from '../../lib/resourceTeams';
import {
  normalizeScheduleMetadata,
  scheduleRequestFromForm,
  type PipelineListItem,
  type PipelineSchedule,
  type ScheduleFormState,
  type ScheduleMetadata,
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

export async function fetchSchedules(pipelineFilter = ''): Promise<PipelineSchedule[]> {
  const query = pipelineFilter ? `?pipeline=${encodeURIComponent(pipelineFilter)}` : '';
  const payload = await requestJson<PipelineSchedule[]>(`/v1/schedules${query}`);
  return Array.isArray(payload) ? payload : [];
}

export async function fetchScheduleMetadata(): Promise<ScheduleMetadata> {
  const [pipelinePayload, teamPayload, secretScopes, variableScopes] = await Promise.all([
    requestJson<Array<PipelineListItem | string>>('/v1/pipelines').catch(() => []),
    fetchPipelineRunTeamPaths().catch(() => []),
    requestJson<Array<string | { scope?: string; name?: string }>>('/v1/secrets/scopes').catch(() => []),
    requestJson<Array<string | { scope?: string; name?: string }>>('/v1/variables/scopes').catch(() => []),
  ]);
  return normalizeScheduleMetadata(pipelinePayload, teamPayload, secretScopes, variableScopes);
}

export function saveSchedule(form: ScheduleFormState, schedule?: PipelineSchedule): Promise<PipelineSchedule> {
  return requestJson<PipelineSchedule>(schedule ? `/v1/schedules/${encodeURIComponent(schedule.id)}` : '/v1/schedules', {
    method: schedule ? 'PUT' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(scheduleRequestFromForm(form)),
  });
}

export function setScheduleEnabled(scheduleID: string, enabled: boolean): Promise<PipelineSchedule> {
  return requestJson<PipelineSchedule>(`/v1/schedules/${encodeURIComponent(scheduleID)}/${enabled ? 'enable' : 'disable'}`, {
    method: 'POST',
  });
}

export function runSchedule(scheduleID: string): Promise<{ run_id?: string }> {
  return requestJson<{ run_id?: string }>(`/v1/schedules/${encodeURIComponent(scheduleID)}/run`, { method: 'POST' });
}

export async function deleteSchedule(scheduleID: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/schedules/${encodeURIComponent(scheduleID)}`, { method: 'DELETE' });
  if (!response.ok) {
    throw new Error(await readResponseError(response, `Unable to delete schedule (${response.status})`));
  }
}
