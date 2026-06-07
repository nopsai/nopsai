import { apiClient } from '../../../lib/api';
import {
  cleanupRequestFromManualForm,
  scheduleRequestFromForm,
  type BackupType,
  type CleanupJob,
  type CleanupPreview,
  type CleanupSchedule,
  type DataBackup,
  type ManualCleanupForm,
  type ScheduleForm,
} from './model';

export type DataManagementState = {
  backups: DataBackup[];
  jobs: CleanupJob[];
  schedules: CleanupSchedule[];
};

async function fetchJson(path: string, init?: RequestInit): Promise<unknown> {
  const response = await apiClient.fetch(path, init);
  if (response.status === 204) return null;
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed (${response.status})`);
  }
  return response.json();
}

export async function fetchDataManagementState(): Promise<DataManagementState> {
  const [backupPayload, jobPayload, schedulePayload] = await Promise.all([
    fetchJson('/v1/system/data/backups'),
    fetchJson('/v1/system/data/cleanup/jobs'),
    fetchJson('/v1/system/data/cleanup/schedules'),
  ]);
  return {
    backups: Array.isArray(backupPayload) ? (backupPayload as DataBackup[]) : [],
    jobs: Array.isArray(jobPayload) ? (jobPayload as CleanupJob[]) : [],
    schedules: Array.isArray(schedulePayload) ? (schedulePayload as CleanupSchedule[]) : [],
  };
}

export async function createDataBackup(backupType: BackupType): Promise<void> {
  await fetchJson('/v1/system/data/backups', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ backup_type: backupType }),
  });
}

export async function downloadDataBackup(backupID: string): Promise<Blob> {
  const response = await apiClient.fetch(`/v1/system/data/backups/${encodeURIComponent(backupID)}/download`);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Download failed (${response.status})`);
  }
  return response.blob();
}

export async function deleteDataBackup(backupID: string): Promise<void> {
  await fetchJson(`/v1/system/data/backups/${encodeURIComponent(backupID)}`, { method: 'DELETE' });
}

export async function previewDataCleanup(form: ManualCleanupForm): Promise<CleanupPreview> {
  return (await fetchJson('/v1/system/data/cleanup/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cleanupRequestFromManualForm(form)),
  })) as CleanupPreview;
}

export async function runDataCleanup(form: ManualCleanupForm): Promise<void> {
  await fetchJson('/v1/system/data/cleanup/run', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cleanupRequestFromManualForm(form)),
  });
}

export async function saveCleanupSchedule(form: ScheduleForm): Promise<void> {
  const editing = Boolean(form.id);
  await fetchJson(
    editing
      ? `/v1/system/data/cleanup/schedules/${encodeURIComponent(form.id)}`
      : '/v1/system/data/cleanup/schedules',
    {
      method: editing ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(scheduleRequestFromForm(form)),
    }
  );
}

export async function deleteCleanupSchedule(scheduleID: string): Promise<void> {
  await fetchJson(`/v1/system/data/cleanup/schedules/${encodeURIComponent(scheduleID)}`, { method: 'DELETE' });
}

export async function setCleanupScheduleEnabled(scheduleID: string, enabled: boolean): Promise<void> {
  const action = enabled ? 'enable' : 'disable';
  await fetchJson(`/v1/system/data/cleanup/schedules/${encodeURIComponent(scheduleID)}/${action}`, { method: 'POST' });
}

export async function runCleanupSchedule(scheduleID: string): Promise<void> {
  await fetchJson(`/v1/system/data/cleanup/schedules/${encodeURIComponent(scheduleID)}/run`, { method: 'POST' });
}
