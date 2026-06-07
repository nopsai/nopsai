import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  createDataBackup,
  deleteCleanupSchedule,
  deleteDataBackup,
  downloadDataBackup,
  fetchDataManagementState,
  previewDataCleanup,
  runCleanupSchedule,
  runDataCleanup,
  saveCleanupSchedule,
  setCleanupScheduleEnabled,
} from './api';
import {
  cleanupSignature,
  defaultManualForm,
  defaultScheduleForm,
  scheduleFormFromRecord,
  sumCounts,
  type BackupType,
  type CleanupJob,
  type CleanupPreviewState,
  type CleanupSchedule,
  type CleanupTarget,
  type DataBackup,
  type DataManagementToast,
  type ManualCleanupForm,
  type ScheduleForm,
} from './model';

function downloadBlob(fileName: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export function useDataManagement({ canManage }: { canManage: boolean }) {
  const [backups, setBackups] = useState<DataBackup[]>([]);
  const [jobs, setJobs] = useState<CleanupJob[]>([]);
  const [schedules, setSchedules] = useState<CleanupSchedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [backupType, setBackupType] = useState<BackupType>('full');
  const [manualForm, setManualForm] = useState<ManualCleanupForm>(defaultManualForm);
  const [scheduleForm, setScheduleForm] = useState<ScheduleForm>(defaultScheduleForm);
  const [preview, setPreview] = useState<CleanupPreviewState | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [toast, setToast] = useState<DataManagementToast | null>(null);

  const manualSignature = useMemo(() => cleanupSignature(manualForm), [manualForm]);
  const previewReady = Boolean(preview && preview.signature === manualSignature);

  const showToast = useCallback((message: string, tone: DataManagementToast['tone'] = 'info') => {
    setToast({ message, tone });
    window.setTimeout(() => setToast(null), 3600);
  }, []);

  const loadAll = useCallback(
    async (quiet = false) => {
      if (!quiet) setLoading(true);
      try {
        const state = await fetchDataManagementState();
        setBackups(state.backups);
        setJobs(state.jobs);
        setSchedules(state.schedules);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to load data management state', 'error');
      } finally {
        if (!quiet) setLoading(false);
      }
    },
    [showToast]
  );

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  const createBackup = useCallback(async () => {
    if (!canManage) return;
    setBusy('backup-create');
    try {
      await createDataBackup(backupType);
      showToast('Backup created.', 'success');
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to create backup', 'error');
    } finally {
      setBusy(null);
    }
  }, [backupType, canManage, loadAll, showToast]);

  const downloadBackup = useCallback(
    async (backup: DataBackup) => {
      setBusy(`backup-download-${backup.id}`);
      try {
        const blob = await downloadDataBackup(backup.id);
        downloadBlob(backup.file_name || `nopsai-${backup.backup_type}-backup.jsonl.gz`, blob);
        await loadAll(true);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to download backup', 'error');
      } finally {
        setBusy(null);
      }
    },
    [loadAll, showToast]
  );

  const deleteBackup = useCallback(
    async (backup: DataBackup) => {
      if (!canManage) return;
      if (!window.confirm(`Delete backup ${backup.file_name || backup.id}?`)) return;
      setBusy(`backup-delete-${backup.id}`);
      try {
        await deleteDataBackup(backup.id);
        showToast('Backup deleted.', 'success');
        await loadAll(true);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to delete backup', 'error');
      } finally {
        setBusy(null);
      }
    },
    [canManage, loadAll, showToast]
  );

  const previewCleanup = useCallback(async () => {
    setBusy('cleanup-preview');
    try {
      const payload = await previewDataCleanup(manualForm);
      setPreview({ ...payload, signature: cleanupSignature(manualForm) });
      showToast('Cleanup preview ready.', 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to preview cleanup', 'error');
    } finally {
      setBusy(null);
    }
  }, [manualForm, showToast]);

  const runCleanup = useCallback(async () => {
    if (!canManage || !previewReady) return;
    const totalRows = preview?.total_rows ?? sumCounts(preview?.counts);
    if (totalRows > 0 && !window.confirm(`Run cleanup and delete ${totalRows.toLocaleString()} row(s)?`)) return;
    setBusy('cleanup-run');
    try {
      await runDataCleanup(manualForm);
      showToast('Cleanup completed.', 'success');
      setPreview(null);
      await loadAll(true);
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Failed to run cleanup', 'error');
      await loadAll(true);
    } finally {
      setBusy(null);
    }
  }, [canManage, loadAll, manualForm, preview, previewReady, showToast]);

  const saveSchedule = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (!canManage) return;
      const editing = Boolean(scheduleForm.id);
      setBusy('schedule-save');
      try {
        await saveCleanupSchedule(scheduleForm);
        showToast(editing ? 'Schedule updated.' : 'Schedule created.', 'success');
        setScheduleForm(defaultScheduleForm);
        await loadAll(true);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to save schedule', 'error');
      } finally {
        setBusy(null);
      }
    },
    [canManage, loadAll, scheduleForm, showToast]
  );

  const editSchedule = useCallback((schedule: CleanupSchedule) => {
    setScheduleForm(scheduleFormFromRecord(schedule));
  }, []);

  const deleteSchedule = useCallback(
    async (schedule: CleanupSchedule) => {
      if (!canManage) return;
      if (!window.confirm(`Delete cleanup schedule ${schedule.name}?`)) return;
      setBusy(`schedule-delete-${schedule.id}`);
      try {
        await deleteCleanupSchedule(schedule.id);
        showToast('Schedule deleted.', 'success');
        if (scheduleForm.id === schedule.id) setScheduleForm(defaultScheduleForm);
        await loadAll(true);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to delete schedule', 'error');
      } finally {
        setBusy(null);
      }
    },
    [canManage, loadAll, scheduleForm.id, showToast]
  );

  const setScheduleEnabled = useCallback(
    async (schedule: CleanupSchedule, enabled: boolean) => {
      if (!canManage) return;
      setBusy(`schedule-enabled-${schedule.id}`);
      try {
        await setCleanupScheduleEnabled(schedule.id, enabled);
        showToast(enabled ? 'Schedule enabled.' : 'Schedule disabled.', 'success');
        await loadAll(true);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to update schedule', 'error');
      } finally {
        setBusy(null);
      }
    },
    [canManage, loadAll, showToast]
  );

  const runScheduleNow = useCallback(
    async (schedule: CleanupSchedule) => {
      if (!canManage) return;
      if (!window.confirm(`Run cleanup schedule ${schedule.name} now?`)) return;
      setBusy(`schedule-run-${schedule.id}`);
      try {
        await runCleanupSchedule(schedule.id);
        showToast('Scheduled cleanup started.', 'success');
        await loadAll(true);
      } catch (error) {
        showToast(error instanceof Error ? error.message : 'Failed to run schedule', 'error');
        await loadAll(true);
      } finally {
        setBusy(null);
      }
    },
    [canManage, loadAll, showToast]
  );

  const updateManualTarget = useCallback((target: CleanupTarget) => {
    setManualForm(prev => ({
      ...prev,
      target,
      mode: target === 'logs' ? 'older_than_days' : 'keep_last',
    }));
    setPreview(null);
  }, []);

  const updateScheduleTarget = useCallback((target: CleanupTarget) => {
    setScheduleForm(prev => ({
      ...prev,
      target,
      mode: target === 'logs' ? 'older_than_days' : 'keep_last',
    }));
  }, []);

  return {
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
  };
}
