import {
  effectiveScheduleRunTeamPath,
  friendlyCronLabel,
  normalizeIdentifier,
  normalizeScheduleKind,
  normalizeScopeOption,
  type PipelineSchedule,
} from './model';

export function sourceLabel(source?: string) {
  const normalized = (source || '').trim().toLowerCase();
  if (normalized.includes('git')) return 'GitOps';
  if (normalized.includes('db') || normalized.includes('database')) return 'Database';
  return normalized || 'Database';
}

export function statusLabel(status?: string) {
  const normalized = (status || '').trim();
  if (!normalized) return 'No runs';
  return normalized
    .split(/[\s_-]+/)
    .filter(Boolean)
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

export function statusClass(status?: string) {
  const normalized = (status || '').toLowerCase();
  if (normalized.includes('success')) return 'runner-pill--ok';
  if (normalized.includes('fail') || normalized.includes('cancel')) return 'runner-pill--error';
  if (normalized.includes('running') || normalized.includes('pending')) return 'runner-pill--warning';
  return 'runner-pill--muted';
}

export function formatDateTime(value?: string, timeZone?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  const options: Intl.DateTimeFormatOptions = {
    weekday: 'short',
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'short',
  };
  const normalizedZone = (timeZone || '').trim();
  try {
    return new Intl.DateTimeFormat(undefined, normalizedZone ? { ...options, timeZone: normalizedZone } : options).format(date);
  } catch {
    return new Intl.DateTimeFormat(undefined, options).format(date);
  }
}

export function formatScope(scope?: string) {
  const normalized = normalizeScopeOption(scope);
  return normalized || 'default';
}

export function formatTeamPath(path?: string) {
  const normalized = normalizeIdentifier(path);
  return normalized === 'root' || !normalized ? 'Root' : normalized;
}

export function friendlyScheduleLabel(schedule: PipelineSchedule) {
  if (normalizeScheduleKind(schedule.schedule_kind) === 'once') {
    return `Once at ${formatDateTime(schedule.run_at || schedule.next_run_at, schedule.timezone)}`;
  }
  return friendlyCronLabel(schedule.cron_expression || schedule.cron);
}

export function scheduleRunTeamLabel(schedule: PipelineSchedule) {
  return formatTeamPath(effectiveScheduleRunTeamPath(schedule));
}
