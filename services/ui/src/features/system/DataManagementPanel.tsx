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
import {
  cleanupRuleLabel,
  countLabels,
  defaultScheduleForm,
  formatBytes,
  formatDate,
  modeOptions,
  sumCounts,
  type BackupType,
  type CleanupCounts,
  type CleanupMode,
  type CleanupTarget,
  type ManualCleanupForm,
  type ScheduleForm,
} from './data-management/model';
import { useDataManagement } from './data-management/useDataManagement';

function DataManagementPanel({ canManage }: { canManage: boolean }) {
  const {
    backups,
    jobs,
    schedules,
    loading,
    backupType,
    setBackupType,
    manualForm,
    setManualForm,
    scheduleForm,
    setScheduleForm,
    preview,
    setPreview,
    previewReady,
    busy,
    toast,
    loadAll,
    createBackup,
    downloadBackup,
    deleteBackup,
    previewCleanup,
    runCleanup,
    saveSchedule,
    editSchedule,
    deleteSchedule,
    setScheduleEnabled,
    runScheduleNow,
    updateManualTarget,
    updateScheduleTarget,
  } = useDataManagement({ canManage });

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

export default DataManagementPanel;
