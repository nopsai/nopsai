export type CleanupTarget = 'runs' | 'logs';
export type CleanupMode = 'keep_last' | 'older_than_days' | 'all_terminal_runs' | 'all_logs';
export type BackupType = 'full' | 'runs' | 'logs';

export type DataBackup = {
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

export type CleanupCounts = Record<string, number>;

export type CleanupPlan = {
  target: CleanupTarget;
  mode: CleanupMode;
  keep_last?: number;
  older_than_days?: number;
  backup_before_cleanup?: boolean;
};

export type CleanupPreview = {
  plan: CleanupPlan;
  counts: CleanupCounts;
  total_rows: number;
};

export type CleanupPreviewState = CleanupPreview & { signature: string };

export type CleanupJob = {
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

export type CleanupSchedule = {
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
  source?: string;
  config_repo_id?: number;
  config_source_path?: string;
  config_source_commit_sha?: string;
  managed_by_config_repo?: boolean;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
};

export type ManualCleanupForm = {
  target: CleanupTarget;
  mode: CleanupMode;
  keepLast: string;
  olderThanDays: string;
  backupBeforeCleanup: boolean;
};

export type ScheduleForm = {
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

export type DataManagementToast = {
  message: string;
  tone: 'success' | 'error' | 'info';
};

export const defaultManualForm: ManualCleanupForm = {
  target: 'runs',
  mode: 'keep_last',
  keepLast: '30',
  olderThanDays: '30',
  backupBeforeCleanup: false,
};

export const defaultScheduleForm: ScheduleForm = {
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

export const countLabels: Record<string, string> = {
  pipeline_runs: 'Pipeline runs',
  task_runs: 'Tasks',
  step_runs: 'Steps',
  pipeline_run_logs: 'Logs',
  pipeline_run_checkpoints: 'Checkpoints',
  pipeline_approvals: 'Approvals',
  pipeline_run_knowledge_contexts: 'Knowledge snapshots',
};

export function modeOptions(target: CleanupTarget): Array<{ value: CleanupMode; label: string }> {
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

export function cleanupRequestFromManualForm(form: ManualCleanupForm): CleanupPlan {
  return {
    target: form.target,
    mode: form.mode,
    keep_last: Number.parseInt(form.keepLast, 10) || 0,
    older_than_days: Number.parseInt(form.olderThanDays, 10) || 0,
    backup_before_cleanup: Boolean(form.backupBeforeCleanup),
  };
}

export function cleanupSignature(form: ManualCleanupForm): string {
  return JSON.stringify(cleanupRequestFromManualForm(form));
}

export function scheduleRequestFromForm(form: ScheduleForm) {
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

export function scheduleFormFromRecord(schedule: CleanupSchedule): ScheduleForm {
  const target = schedule.target === 'logs' ? 'logs' : 'runs';
  return {
    id: schedule.id,
    name: schedule.name || 'Weekly cleanup',
    description: schedule.description || '',
    enabled: Boolean(schedule.enabled),
    target,
    mode: normalizeModeForTarget(schedule.mode, target),
    keepLast: String(schedule.keep_last || 30),
    olderThanDays: String(schedule.older_than_days || 30),
    backupBeforeCleanup: Boolean(schedule.backup_before_cleanup),
    cronExpression: schedule.cron_expression || '0 2 * * 0',
    timezone: schedule.timezone || 'UTC',
  };
}

export function normalizeModeForTarget(mode: string, target: CleanupTarget): CleanupMode {
  if (target === 'logs') {
    return mode === 'all_logs' ? 'all_logs' : 'older_than_days';
  }
  if (mode === 'older_than_days' || mode === 'all_terminal_runs') return mode;
  return 'keep_last';
}

export function cleanupRuleLabel(rule: { target: string; mode: string; keep_last?: number; older_than_days?: number }) {
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

export function cleanupScheduleSourceLabel(schedule: Pick<CleanupSchedule, 'source' | 'managed_by_config_repo'>) {
  const source = String(schedule.source || '').toLowerCase();
  return schedule.managed_by_config_repo || source.includes('git') ? 'GitOps' : 'Database';
}

export function sumCounts(counts?: CleanupCounts) {
  return Object.values(counts || {}).reduce((sum, value) => sum + (Number.isFinite(value) ? value : 0), 0);
}

export function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

export function formatBytes(bytes: number) {
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
