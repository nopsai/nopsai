import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  CalendarClock,
  Database,
  Download,
  Edit3,
  Eye,
  FileArchive,
  PauseCircle,
  PlayCircle,
  Plus,
  RefreshCw,
  Save,
  Trash2,
  X,
} from 'lucide-react';
import { buildApiUrl } from '../lib/api';

type CleanupTarget = 'runs' | 'logs';
type CleanupMode = 'keep_last' | 'older_than_days' | 'all_terminal_runs' | 'all_logs';
type BackupType = 'full' | 'runs' | 'logs';

type DataBackup = {
  id: string;
  backup_type: BackupType | string;
  status: string;
  file_name?: string;
  file_path?: string;
  content_type?: string;
  size_bytes?: number;
  checksum_sha256?: string;
  requested_by?: string;
  error?: string;
  created_at: string;
  completed_at?: string;
};

type CleanupCounts = Record<string, number>;

type CleanupPreview = {
  plan: CleanupPlan;
  counts: CleanupCounts;
  total_rows: number;
};

type CleanupPlan = {
  target: CleanupTarget;
  mode: CleanupMode;
  keep_last?: number;
  older_than_days?: number;
  backup_before_cleanup?: boolean;
};

type CleanupJob = {
  id: string;
  schedule_id?: string;
  trigger_type: string;
  status: string;
  target: CleanupTarget | string;
  mode: CleanupMode | string;
  keep_last?: number;
  older_than_days?: number;
  backup_before_cleanup?: boolean;
  backup_id?: string;
  requested_by?: string;
  preview_counts?: CleanupCounts;
  deleted_counts?: CleanupCounts;
  error?: string;
  started_at: string;
  completed_at?: string;
  created_at: string;
};

type CleanupSchedule = {
  id: string;
  name: string;
  description?: string;
  enabled: boolean;
  target: CleanupTarget | string;
  mode: CleanupMode | string;
  keep_last?: number;
  older_than_days?: number;
  backup_before_cleanup?: boolean;
  cron_expression: string;
  timezone: string;
  next_run_at?: string;
  last_run_at?: string;
  last_job_id?: string;
  last_status?: string;
  last_deleted_counts?: CleanupCounts;
  last_error?: string;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
};

type ManualCleanupForm = {
  target: CleanupTarget;
  mode: CleanupMode;
  keepLast: string;
  olderThanDays: string;
  backupBeforeCleanup: boolean;
};

type ScheduleForm = {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  target: CleanupTarget;
  mode: CleanupMode;
  keepLast: string;
  olderThanDays: string;
  backupBeforeCleanup: boolean;
  cronExpression: string;
  timezone: string;
};

type Toast = {
  message: string;
  tone: 'success' | 'error' | 'info';
};

const defaultManualForm: ManualCleanupForm = {
  target: 'runs',
  mode: 'keep_last',
  keepLast: '30',
  olderThanDays: '30',
  backupBeforeCleanup: false,
};

const defaultScheduleForm: ScheduleForm = {
  id: '',
  name: 'Weekly cleanup',
  description: '',
  enabled: true,
  target: 'runs',
  mode: 'keep_last',
  keepLast: '30',
  olderThanDays: '30',
  backupBeforeCleanup: true,
  cronExpression: '0 2 * * 0',
  timezone: 'UTC',
};

const countLabels: Record<string, string> = {
  pipeline_runs: 'Pipeline runs',
  task_runs: 'Tasks',
  step_runs: 'Steps',
  pipeline_run_logs: 'Logs',
  pipeline_run_checkpoints: 'Checkpoints',
  pipeline_approvals: 'Approvals',
  pipeline_run_knowledge_contexts: 'Knowledge snapshots',
};

function DataManagementPage({ canManage }: { canManage: boolean }) {
  const [backups, setBackups] = useState<DataBackup[]>([]);
  const [jobs, setJobs] = useState<CleanupJob[]>([]);
  const [schedules, setSchedules] = useState<CleanupSchedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [backupType, setBackupType] = useState<BackupType>('full');
  const [manualForm, setManualForm] = useState<ManualCleanupForm>(defaultManualForm);
  const [scheduleForm, setScheduleForm] = useState<ScheduleForm>(defaultScheduleForm);
  const [preview, setPreview] = useState<(CleanupPreview & { signature: string }) | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);

  const manualSignature = useMemo(() => JSON.stringify(cleanupRequestFromManualForm(manualForm)), [manualForm]);
  const previewReady = Boolean(preview && preview.signature === manualSignature);

  const showToast = useCallback((message: string, tone: Toast['tone'] = 'info') => {
    setToast({ message, tone });
    window.setTimeout(() => setToast(null), 3600);
  }, []);

  const fetchJson = useCallback(async (path: string, init?: RequestInit): Promise<unknown> => {
    const response = await fetch(buildApiUrl(path), init);
    if (response.status === 204) return null;
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed (${response.status})`);
    }
    return response.json();
  }, []);

  const loadAll = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const [backupPayload, jobPayload, schedulePayload] = await Promise.all([
        fetchJson('/v1/system/data/backups'),
        fetchJson('/v1/system/data/cleanup/jobs'),
        fetchJson('/v1/system/data/cleanup/schedules'),
      ]);
      setBackups(Array.isArray(backupPayload) ? backupPayload as DataBackup[] : []);
      setJobs(Array.isArray(jobPayload) ? jobPayload as CleanupJob[] : []);
      setSchedules(Array.isArray(schedulePayload) ? schedulePayload as CleanupSchedule[] : []);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to load data management state', 'error');
    } finally {
      if (!quiet) setLoading(false);
    }
  }, [fetchJson, showToast]);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  const createBackup = useCallback(async () => {
    if (!canManage) return;
    setBusy('backup-create');
    try {
      await fetchJson('/v1/system/data/backups', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ backup_type: backupType }),
      });
      showToast('Backup created.', 'success');
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to create backup', 'error');
    } finally {
      setBusy(null);
    }
  }, [backupType, canManage, fetchJson, loadAll, showToast]);

  const downloadBackup = useCallback(async (backup: DataBackup) => {
    setBusy(`backup-download-${backup.id}`);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/data/backups/${encodeURIComponent(backup.id)}/download`));
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Download failed (${response.status})`);
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = backup.file_name || `nopsai-${backup.backup_type}-backup.jsonl.gz`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to download backup', 'error');
    } finally {
      setBusy(null);
    }
  }, [loadAll, showToast]);

  const deleteBackup = useCallback(async (backup: DataBackup) => {
    if (!canManage) return;
    if (!window.confirm(`Delete backup ${backup.file_name || backup.id}?`)) return;
    setBusy(`backup-delete-${backup.id}`);
    try {
      await fetchJson(`/v1/system/data/backups/${encodeURIComponent(backup.id)}`, { method: 'DELETE' });
      showToast('Backup deleted.', 'success');
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to delete backup', 'error');
    } finally {
      setBusy(null);
    }
  }, [canManage, fetchJson, loadAll, showToast]);

  const previewCleanup = useCallback(async () => {
    const request = cleanupRequestFromManualForm(manualForm);
    setBusy('cleanup-preview');
    try {
      const payload = await fetchJson('/v1/system/data/cleanup/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
      }) as CleanupPreview;
      setPreview({ ...payload, signature: JSON.stringify(request) });
      showToast('Cleanup preview ready.', 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to preview cleanup', 'error');
    } finally {
      setBusy(null);
    }
  }, [fetchJson, manualForm, showToast]);

  const runCleanup = useCallback(async () => {
    if (!canManage || !previewReady) return;
    const totalRows = preview?.total_rows ?? sumCounts(preview?.counts);
    if (totalRows > 0 && !window.confirm(`Run cleanup and delete ${totalRows.toLocaleString()} row(s)?`)) return;
    setBusy('cleanup-run');
    try {
      await fetchJson('/v1/system/data/cleanup/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(cleanupRequestFromManualForm(manualForm)),
      });
      showToast('Cleanup completed.', 'success');
      setPreview(null);
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to run cleanup', 'error');
      await loadAll(true);
    } finally {
      setBusy(null);
    }
  }, [canManage, fetchJson, loadAll, manualForm, preview, previewReady, showToast]);

  const saveSchedule = useCallback(async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canManage) return;
    const editing = Boolean(scheduleForm.id);
    setBusy('schedule-save');
    try {
      const request = scheduleRequestFromForm(scheduleForm);
      await fetchJson(
        editing
          ? `/v1/system/data/cleanup/schedules/${encodeURIComponent(scheduleForm.id)}`
          : '/v1/system/data/cleanup/schedules',
        {
          method: editing ? 'PUT' : 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(request),
        }
      );
      showToast(editing ? 'Schedule updated.' : 'Schedule created.', 'success');
      setScheduleForm(defaultScheduleForm);
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to save schedule', 'error');
    } finally {
      setBusy(null);
    }
  }, [canManage, fetchJson, loadAll, scheduleForm, showToast]);

  const editSchedule = useCallback((schedule: CleanupSchedule) => {
    setScheduleForm({
      id: schedule.id,
      name: schedule.name || 'Weekly cleanup',
      description: schedule.description || '',
      enabled: Boolean(schedule.enabled),
      target: schedule.target === 'logs' ? 'logs' : 'runs',
      mode: normalizeModeForTarget(schedule.mode, schedule.target === 'logs' ? 'logs' : 'runs'),
      keepLast: String(schedule.keep_last || 30),
      olderThanDays: String(schedule.older_than_days || 30),
      backupBeforeCleanup: Boolean(schedule.backup_before_cleanup),
      cronExpression: schedule.cron_expression || '0 2 * * 0',
      timezone: schedule.timezone || 'UTC',
    });
  }, []);

  const deleteSchedule = useCallback(async (schedule: CleanupSchedule) => {
    if (!canManage) return;
    if (!window.confirm(`Delete cleanup schedule ${schedule.name}?`)) return;
    setBusy(`schedule-delete-${schedule.id}`);
    try {
      await fetchJson(`/v1/system/data/cleanup/schedules/${encodeURIComponent(schedule.id)}`, { method: 'DELETE' });
      showToast('Schedule deleted.', 'success');
      if (scheduleForm.id === schedule.id) setScheduleForm(defaultScheduleForm);
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to delete schedule', 'error');
    } finally {
      setBusy(null);
    }
  }, [canManage, fetchJson, loadAll, scheduleForm.id, showToast]);

  const setScheduleEnabled = useCallback(async (schedule: CleanupSchedule, enabled: boolean) => {
    if (!canManage) return;
    setBusy(`schedule-enabled-${schedule.id}`);
    try {
      const action = enabled ? 'enable' : 'disable';
      await fetchJson(`/v1/system/data/cleanup/schedules/${encodeURIComponent(schedule.id)}/${action}`, { method: 'POST' });
      showToast(enabled ? 'Schedule enabled.' : 'Schedule disabled.', 'success');
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to update schedule', 'error');
    } finally {
      setBusy(null);
    }
  }, [canManage, fetchJson, loadAll, showToast]);

  const runScheduleNow = useCallback(async (schedule: CleanupSchedule) => {
    if (!canManage) return;
    if (!window.confirm(`Run cleanup schedule ${schedule.name} now?`)) return;
    setBusy(`schedule-run-${schedule.id}`);
    try {
      await fetchJson(`/v1/system/data/cleanup/schedules/${encodeURIComponent(schedule.id)}/run`, { method: 'POST' });
      showToast('Scheduled cleanup started.', 'success');
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to run schedule', 'error');
      await loadAll(true);
    } finally {
      setBusy(null);
    }
  }, [canManage, fetchJson, loadAll, showToast]);

  const updateManualTarget = (target: CleanupTarget) => {
    setManualForm(prev => ({
      ...prev,
      target,
      mode: target === 'logs' ? 'older_than_days' : 'keep_last',
    }));
    setPreview(null);
  };

  const updateScheduleTarget = (target: CleanupTarget) => {
    setScheduleForm(prev => ({
      ...prev,
      target,
      mode: target === 'logs' ? 'older_than_days' : 'keep_last',
    }));
  };

  return (
    <div id="system-data-management-section" className="space-y-6 pb-24">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.16em] text-[var(--text-secondary)]">System Data</p>
          <h3 className="text-xl font-semibold text-[var(--text-primary)]">Data Management</h3>
        </div>
        <button type="button" className="glass-button-ghost inline-flex items-center gap-2" onClick={() => void loadAll()} disabled={loading}>
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      {toast && (
        <div className={`rounded-lg border px-4 py-3 text-sm ${toast.tone === 'error' ? 'border-red-400/40 text-red-500' : toast.tone === 'success' ? 'border-emerald-400/40 text-emerald-600' : 'border-[var(--border-primary)] text-[var(--text-secondary)]'}`}>
          {toast.message}
        </div>
      )}

      <section className="glass-card border border-[var(--border-primary)] rounded-xl p-5 space-y-4">
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <div className="flex items-center gap-2">
              <FileArchive className="h-4 w-4 text-[var(--text-secondary)]" />
              <h4 className="text-base font-semibold text-[var(--text-primary)]">Backups</h4>
            </div>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <select className="pipelines-input min-w-36" value={backupType} onChange={event => setBackupType(event.target.value as BackupType)} disabled={!canManage || busy === 'backup-create'}>
              <option value="full">Full DB</option>
              <option value="runs">Runs</option>
              <option value="logs">Logs</option>
            </select>
            <button type="button" className="glass-button-primary inline-flex items-center gap-2" onClick={() => void createBackup()} disabled={!canManage || busy === 'backup-create'}>
              <Plus className="h-4 w-4" />
              Create
            </button>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="text-left text-xs uppercase text-[var(--text-secondary)]">
              <tr>
                <th className="px-3 py-2">Created</th>
                <th className="px-3 py-2">Type</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Size</th>
                <th className="px-3 py-2">Checksum</th>
                <th className="px-3 py-2 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border-primary)]">
              {backups.length === 0 ? (
                <tr><td className="px-3 py-6 text-[var(--text-secondary)]" colSpan={6}>No backups yet.</td></tr>
              ) : backups.map(backup => (
                <tr key={backup.id}>
                  <td className="px-3 py-3 whitespace-nowrap">{formatDate(backup.created_at)}</td>
                  <td className="px-3 py-3 capitalize">{backup.backup_type}</td>
                  <td className="px-3 py-3"><StatusPill status={backup.status} /></td>
                  <td className="px-3 py-3 whitespace-nowrap">{formatBytes(backup.size_bytes || 0)}</td>
                  <td className="px-3 py-3 font-mono text-xs text-[var(--text-secondary)]">{backup.checksum_sha256 ? backup.checksum_sha256.slice(0, 16) : backup.error || '-'}</td>
                  <td className="px-3 py-3">
                    <div className="flex justify-end gap-2">
                      <button type="button" className="glass-button-ghost inline-flex items-center justify-center" title="Download backup" disabled={backup.status !== 'success' || busy === `backup-download-${backup.id}`} onClick={() => void downloadBackup(backup)}>
                        <Download className="h-4 w-4" />
                      </button>
                      <button type="button" className="glass-button-danger inline-flex items-center justify-center" title="Delete backup" disabled={!canManage || busy === `backup-delete-${backup.id}`} onClick={() => void deleteBackup(backup)}>
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]">
        <div className="glass-card border border-[var(--border-primary)] rounded-xl p-5 space-y-4">
          <div className="flex items-center gap-2">
            <Database className="h-4 w-4 text-[var(--text-secondary)]" />
            <h4 className="text-base font-semibold text-[var(--text-primary)]">Manual Cleanup</h4>
          </div>
          <CleanupForm
            form={manualForm}
            canManage={true}
            onTargetChange={updateManualTarget}
            onChange={next => {
              setManualForm(next);
              setPreview(null);
            }}
          />
          <div className="flex flex-wrap gap-2">
            <button type="button" className="glass-button-subtle inline-flex items-center gap-2" onClick={() => void previewCleanup()} disabled={busy === 'cleanup-preview'}>
              <Eye className="h-4 w-4" />
              Preview
            </button>
            <button type="button" className="glass-button-primary inline-flex items-center gap-2" onClick={() => void runCleanup()} disabled={!canManage || !previewReady || busy === 'cleanup-run'}>
              <PlayCircle className="h-4 w-4" />
              Run
            </button>
          </div>
          {preview && (
            <div className="rounded-lg border border-[var(--border-primary)] p-4">
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm font-medium">Preview result</span>
                <span className="text-sm text-[var(--text-secondary)]">{(preview.total_rows ?? sumCounts(preview.counts)).toLocaleString()} row(s)</span>
              </div>
              <CountsList counts={preview.counts} />
              {!previewReady && <p className="mt-2 text-xs text-amber-600">Preview no longer matches the form.</p>}
            </div>
          )}
        </div>

        <div className="glass-card border border-[var(--border-primary)] rounded-xl p-5 space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <CalendarClock className="h-4 w-4 text-[var(--text-secondary)]" />
              <h4 className="text-base font-semibold text-[var(--text-primary)]">Scheduled Cleanup</h4>
            </div>
            {scheduleForm.id && (
              <button type="button" className="glass-button-ghost inline-flex items-center gap-2" onClick={() => setScheduleForm(defaultScheduleForm)}>
                <X className="h-4 w-4" />
                Cancel
              </button>
            )}
          </div>
          <form className="space-y-4" onSubmit={saveSchedule}>
            <div className="grid gap-3 md:grid-cols-2">
              <label className="space-y-1 text-sm">
                <span>Name</span>
                <input className="pipelines-input w-full" value={scheduleForm.name} onChange={event => setScheduleForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage} />
              </label>
              <label className="space-y-1 text-sm">
                <span>Cron</span>
                <input className="pipelines-input w-full font-mono" value={scheduleForm.cronExpression} onChange={event => setScheduleForm(prev => ({ ...prev, cronExpression: event.target.value }))} disabled={!canManage} />
              </label>
              <label className="space-y-1 text-sm">
                <span>Timezone</span>
                <input className="pipelines-input w-full" value={scheduleForm.timezone} onChange={event => setScheduleForm(prev => ({ ...prev, timezone: event.target.value }))} disabled={!canManage} />
              </label>
              <label className="space-y-1 text-sm">
                <span>Description</span>
                <input className="pipelines-input w-full" value={scheduleForm.description} onChange={event => setScheduleForm(prev => ({ ...prev, description: event.target.value }))} disabled={!canManage} />
              </label>
            </div>
            <ScheduleCleanupFields form={scheduleForm} canManage={canManage} onTargetChange={updateScheduleTarget} onChange={setScheduleForm} />
            <div className="flex flex-wrap items-center gap-3">
              <label className="inline-flex items-center gap-2 text-sm">
                <input type="checkbox" checked={scheduleForm.enabled} onChange={event => setScheduleForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} />
                Enabled
              </label>
              <button type="submit" className="glass-button-primary inline-flex items-center gap-2" disabled={!canManage || busy === 'schedule-save'}>
                <Save className="h-4 w-4" />
                {scheduleForm.id ? 'Save' : 'Create'}
              </button>
            </div>
          </form>
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="text-left text-xs uppercase text-[var(--text-secondary)]">
                <tr>
                  <th className="px-3 py-2">Name</th>
                  <th className="px-3 py-2">Rule</th>
                  <th className="px-3 py-2">Next</th>
                  <th className="px-3 py-2">Last</th>
                  <th className="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border-primary)]">
                {schedules.length === 0 ? (
                  <tr><td className="px-3 py-6 text-[var(--text-secondary)]" colSpan={5}>No cleanup schedules yet.</td></tr>
                ) : schedules.map(schedule => (
                  <tr key={schedule.id}>
                    <td className="px-3 py-3">
                      <div className="font-medium">{schedule.name}</div>
                      <div className="text-xs text-[var(--text-secondary)]">{schedule.enabled ? 'Enabled' : 'Disabled'}</div>
                    </td>
                    <td className="px-3 py-3">{cleanupRuleLabel(schedule)}</td>
                    <td className="px-3 py-3 whitespace-nowrap">{formatDate(schedule.next_run_at)}</td>
                    <td className="px-3 py-3">
                      <div><StatusPill status={schedule.last_status || 'idle'} /></div>
                      <div className="mt-1 text-xs text-[var(--text-secondary)]">{sumCounts(schedule.last_deleted_counts).toLocaleString()} row(s)</div>
                    </td>
                    <td className="px-3 py-3">
                      <div className="flex justify-end gap-2">
                        <button type="button" className="glass-button-ghost inline-flex items-center justify-center" title="Run now" disabled={!canManage || busy === `schedule-run-${schedule.id}`} onClick={() => void runScheduleNow(schedule)}>
                          <PlayCircle className="h-4 w-4" />
                        </button>
                        <button type="button" className="glass-button-ghost inline-flex items-center justify-center" title="Edit schedule" disabled={!canManage} onClick={() => editSchedule(schedule)}>
                          <Edit3 className="h-4 w-4" />
                        </button>
                        <button type="button" className={schedule.enabled ? 'glass-button-danger inline-flex items-center justify-center' : 'glass-button-subtle inline-flex items-center justify-center'} title={schedule.enabled ? 'Disable schedule' : 'Enable schedule'} disabled={!canManage || busy === `schedule-enabled-${schedule.id}`} onClick={() => void setScheduleEnabled(schedule, !schedule.enabled)}>
                          {schedule.enabled ? <PauseCircle className="h-4 w-4" /> : <PlayCircle className="h-4 w-4" />}
                        </button>
                        <button type="button" className="glass-button-danger inline-flex items-center justify-center" title="Delete schedule" disabled={!canManage || busy === `schedule-delete-${schedule.id}`} onClick={() => void deleteSchedule(schedule)}>
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section className="glass-card border border-[var(--border-primary)] rounded-xl p-5 space-y-4">
        <div className="flex items-center gap-2">
          <PlayCircle className="h-4 w-4 text-[var(--text-secondary)]" />
          <h4 className="text-base font-semibold text-[var(--text-primary)]">Cleanup Jobs</h4>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="text-left text-xs uppercase text-[var(--text-secondary)]">
              <tr>
                <th className="px-3 py-2">Started</th>
                <th className="px-3 py-2">Trigger</th>
                <th className="px-3 py-2">Rule</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Deleted</th>
                <th className="px-3 py-2">Backup</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border-primary)]">
              {jobs.length === 0 ? (
                <tr><td className="px-3 py-6 text-[var(--text-secondary)]" colSpan={6}>No cleanup jobs yet.</td></tr>
              ) : jobs.map(job => (
                <tr key={job.id}>
                  <td className="px-3 py-3 whitespace-nowrap">{formatDate(job.started_at)}</td>
                  <td className="px-3 py-3 capitalize">{job.trigger_type}</td>
                  <td className="px-3 py-3">{cleanupRuleLabel(job)}</td>
                  <td className="px-3 py-3">
                    <StatusPill status={job.status} />
                    {job.error && <div className="mt-1 max-w-md text-xs text-red-500">{job.error}</div>}
                  </td>
                  <td className="px-3 py-3">
                    <div>{sumCounts(job.deleted_counts).toLocaleString()} row(s)</div>
                    <CountsInline counts={job.deleted_counts} />
                  </td>
                  <td className="px-3 py-3 font-mono text-xs text-[var(--text-secondary)]">{job.backup_id ? job.backup_id.slice(0, 8) : '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function CleanupForm({
  form,
  canManage,
  onTargetChange,
  onChange,
}: {
  form: ManualCleanupForm;
  canManage: boolean;
  onTargetChange: (target: CleanupTarget) => void;
  onChange: (form: ManualCleanupForm) => void;
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <label className="space-y-1 text-sm">
        <span>Target</span>
        <select className="pipelines-input w-full" value={form.target} onChange={event => onTargetChange(event.target.value as CleanupTarget)} disabled={!canManage}>
          <option value="runs">Runs</option>
          <option value="logs">Logs</option>
        </select>
      </label>
      <label className="space-y-1 text-sm">
        <span>Mode</span>
        <select className="pipelines-input w-full" value={form.mode} onChange={event => onChange({ ...form, mode: event.target.value as CleanupMode })} disabled={!canManage}>
          {modeOptions(form.target).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
      </label>
      {form.mode === 'keep_last' && (
        <label className="space-y-1 text-sm">
          <span>Keep last</span>
          <input className="pipelines-input w-full" type="number" min="1" value={form.keepLast} onChange={event => onChange({ ...form, keepLast: event.target.value })} disabled={!canManage} />
        </label>
      )}
      {form.mode === 'older_than_days' && (
        <label className="space-y-1 text-sm">
          <span>Older than days</span>
          <input className="pipelines-input w-full" type="number" min="1" value={form.olderThanDays} onChange={event => onChange({ ...form, olderThanDays: event.target.value })} disabled={!canManage} />
        </label>
      )}
      <label className="inline-flex items-center gap-2 text-sm sm:col-span-2">
        <input type="checkbox" checked={form.backupBeforeCleanup} onChange={event => onChange({ ...form, backupBeforeCleanup: event.target.checked })} disabled={!canManage} />
        Backup before cleanup
      </label>
    </div>
  );
}

function ScheduleCleanupFields({
  form,
  canManage,
  onTargetChange,
  onChange,
}: {
  form: ScheduleForm;
  canManage: boolean;
  onTargetChange: (target: CleanupTarget) => void;
  onChange: (form: ScheduleForm) => void;
}) {
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <label className="space-y-1 text-sm">
        <span>Target</span>
        <select className="pipelines-input w-full" value={form.target} onChange={event => onTargetChange(event.target.value as CleanupTarget)} disabled={!canManage}>
          <option value="runs">Runs</option>
          <option value="logs">Logs</option>
        </select>
      </label>
      <label className="space-y-1 text-sm">
        <span>Mode</span>
        <select className="pipelines-input w-full" value={form.mode} onChange={event => onChange({ ...form, mode: event.target.value as CleanupMode })} disabled={!canManage}>
          {modeOptions(form.target).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
      </label>
      {form.mode === 'keep_last' && (
        <label className="space-y-1 text-sm">
          <span>Keep last</span>
          <input className="pipelines-input w-full" type="number" min="1" value={form.keepLast} onChange={event => onChange({ ...form, keepLast: event.target.value })} disabled={!canManage} />
        </label>
      )}
      {form.mode === 'older_than_days' && (
        <label className="space-y-1 text-sm">
          <span>Older than days</span>
          <input className="pipelines-input w-full" type="number" min="1" value={form.olderThanDays} onChange={event => onChange({ ...form, olderThanDays: event.target.value })} disabled={!canManage} />
        </label>
      )}
      <label className="inline-flex items-center gap-2 text-sm md:col-span-2">
        <input type="checkbox" checked={form.backupBeforeCleanup} onChange={event => onChange({ ...form, backupBeforeCleanup: event.target.checked })} disabled={!canManage} />
        Backup before cleanup
      </label>
    </div>
  );
}

function StatusPill({ status }: { status: string }) {
  const normalized = (status || 'idle').toLowerCase();
  const tone =
    normalized === 'success'
      ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30'
      : normalized === 'failure' || normalized === 'error'
        ? 'bg-red-500/10 text-red-500 border-red-500/30'
        : normalized === 'running'
          ? 'bg-sky-500/10 text-sky-600 border-sky-500/30'
          : 'bg-[var(--bg-tertiary)] text-[var(--text-secondary)] border-[var(--border-primary)]';
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-xs font-medium capitalize ${tone}`}>{status || 'idle'}</span>;
}

function CountsList({ counts }: { counts?: CleanupCounts }) {
  const entries = Object.entries(counts || {}).filter(([, value]) => value > 0);
  if (entries.length === 0) return <p className="mt-2 text-sm text-[var(--text-secondary)]">No rows selected.</p>;
  return (
    <div className="mt-3 grid gap-2 sm:grid-cols-2">
      {entries.map(([key, value]) => (
        <div key={key} className="flex items-center justify-between gap-3 rounded-md bg-[var(--bg-tertiary)] px-3 py-2 text-sm">
          <span>{countLabels[key] || key}</span>
          <span className="font-mono">{value.toLocaleString()}</span>
        </div>
      ))}
    </div>
  );
}

function CountsInline({ counts }: { counts?: CleanupCounts }) {
  const entries = Object.entries(counts || {}).filter(([, value]) => value > 0).slice(0, 3);
  if (entries.length === 0) return null;
  return <div className="mt-1 text-xs text-[var(--text-secondary)]">{entries.map(([key, value]) => `${countLabels[key] || key}: ${value.toLocaleString()}`).join(', ')}</div>;
}

function modeOptions(target: CleanupTarget): Array<{ value: CleanupMode; label: string }> {
  if (target === 'logs') {
    return [
      { value: 'older_than_days', label: 'Older than days' },
      { value: 'all_logs', label: 'All logs' },
    ];
  }
  return [
    { value: 'keep_last', label: 'Keep last N' },
    { value: 'older_than_days', label: 'Older than days' },
    { value: 'all_terminal_runs', label: 'All terminal runs' },
  ];
}

function cleanupRequestFromManualForm(form: ManualCleanupForm) {
  return {
    target: form.target,
    mode: form.mode,
    keep_last: Number.parseInt(form.keepLast, 10) || 0,
    older_than_days: Number.parseInt(form.olderThanDays, 10) || 0,
    backup_before_cleanup: Boolean(form.backupBeforeCleanup),
  };
}

function scheduleRequestFromForm(form: ScheduleForm) {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    enabled: Boolean(form.enabled),
    target: form.target,
    mode: form.mode,
    keep_last: Number.parseInt(form.keepLast, 10) || 0,
    older_than_days: Number.parseInt(form.olderThanDays, 10) || 0,
    backup_before_cleanup: Boolean(form.backupBeforeCleanup),
    cron_expression: form.cronExpression.trim(),
    timezone: form.timezone.trim() || 'UTC',
  };
}

function normalizeModeForTarget(mode: string, target: CleanupTarget): CleanupMode {
  if (target === 'logs') {
    return mode === 'all_logs' ? 'all_logs' : 'older_than_days';
  }
  if (mode === 'older_than_days' || mode === 'all_terminal_runs') return mode;
  return 'keep_last';
}

function cleanupRuleLabel(rule: { target: string; mode: string; keep_last?: number; older_than_days?: number }) {
  const target = rule.target === 'logs' ? 'Logs' : 'Runs';
  switch (rule.mode) {
    case 'keep_last':
      return `${target}: keep last ${rule.keep_last || 0}`;
    case 'older_than_days':
      return `${target}: older than ${rule.older_than_days || 0} day(s)`;
    case 'all_terminal_runs':
      return 'Runs: all terminal';
    case 'all_logs':
      return 'Logs: all';
    default:
      return `${target}: ${rule.mode}`;
  }
}

function sumCounts(counts?: CleanupCounts) {
  return Object.values(counts || {}).reduce((sum, value) => sum + (Number.isFinite(value) ? value : 0), 0);
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '-';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

export default DataManagementPage;
