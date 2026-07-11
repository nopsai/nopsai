export type PipelineListItem = {
  id: string;
  identifier?: string;
  source?: string;
};

export type ScheduleRunSummary = {
  run_id: string;
  status: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
};

export type PipelineSchedule = {
  id: string;
  path: string;
  name: string;
  identifier: string;
  description?: string;
  pipeline: string;
  pipeline_path?: string;
  pipeline_name?: string;
  pipeline_version?: string;
  schedule_kind?: string;
  cron?: string;
  cron_expression?: string;
  run_at?: string;
  timezone: string;
  enabled: boolean;
  scope?: string;
  run_team_path?: string;
  variables?: Record<string, string>;
  next_run_at?: string;
  last_run_at?: string;
  last_run_id?: string;
  last_status?: string;
  latest_run?: ScheduleRunSummary;
  source?: string;
  visibility?: string;
  managed_by_config_repo?: boolean;
  config_source_path?: string;
  created_at?: string;
  updated_at?: string;
};

export type CronMode = 'once' | 'minutes' | 'hourly' | 'daily' | 'weekdays' | 'weekly' | 'monthly' | 'yearly' | 'custom';

export type ScheduleFormState = {
  name: string;
  description: string;
  pipeline: string;
  cronMode: CronMode;
  cronTime: string;
  cronWeekday: string;
  cronMonthday: string;
  cronMonth: string;
  cronMinute: string;
  intervalValue: string;
  cron_expression: string;
  runAtDate: string;
  runAtTime: string;
  timezone: string;
  enabled: boolean;
  scope: string;
  runTeamPath: string;
  variablesText: string;
};

export type CronFormFields = Pick<
  ScheduleFormState,
  'cronMode' | 'cronTime' | 'cronWeekday' | 'cronMonthday' | 'cronMonth' | 'cronMinute' | 'intervalValue' | 'cron_expression' | 'runAtDate' | 'runAtTime'
>;

export type ScheduleModalState = {
  mode: 'create' | 'edit';
  schedule?: PipelineSchedule;
};

export type ScheduleRequest = {
  path: string;
  name: string;
  description: string;
  pipeline: string;
  schedule_kind: 'once' | 'cron';
  cron_expression: string;
  run_at?: string;
  timezone: string;
  enabled: boolean;
  scope: string;
  run_team_path: string;
  variables: Record<string, string>;
};

export type ScheduleMetadata = {
  pipelines: string[];
  teams: string[];
  scopes: string[];
};

export const DEFAULT_CRON = '0 2 * * *';
export const DEFAULT_CRON_TIME = '02:00';
export const DEFAULT_TIMEZONE = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
export const WEEKDAY_OPTIONS = [
  { value: '1', label: 'Monday', short: 'Mon' },
  { value: '2', label: 'Tuesday', short: 'Tue' },
  { value: '3', label: 'Wednesday', short: 'Wed' },
  { value: '4', label: 'Thursday', short: 'Thu' },
  { value: '5', label: 'Friday', short: 'Fri' },
  { value: '6', label: 'Saturday', short: 'Sat' },
  { value: '0', label: 'Sunday', short: 'Sun' },
];
export const MONTH_OPTIONS = [
  { value: '1', label: 'January' },
  { value: '2', label: 'February' },
  { value: '3', label: 'March' },
  { value: '4', label: 'April' },
  { value: '5', label: 'May' },
  { value: '6', label: 'June' },
  { value: '7', label: 'July' },
  { value: '8', label: 'August' },
  { value: '9', label: 'September' },
  { value: '10', label: 'October' },
  { value: '11', label: 'November' },
  { value: '12', label: 'December' },
];

export function getTimezoneOptions(): string[] {
  const intlWithSupportedValues = Intl as typeof Intl & { supportedValuesOf?: (key: 'timeZone') => string[] };
  return typeof intlWithSupportedValues.supportedValuesOf === 'function'
    ? intlWithSupportedValues.supportedValuesOf('timeZone')
    : ['UTC', 'Europe/Amsterdam', 'Europe/London', 'America/New_York', 'America/Los_Angeles', 'Asia/Tokyo'];
}

export const WEEKDAY_VALUES = WEEKDAY_OPTIONS.map(option => option.value);
export const MONTHDAY_VALUES = Array.from({ length: 31 }, (_, index) => String(index + 1));

export function normalizeIdentifier(value: unknown): string {
  if (!value) return '';
  return String(value)
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^(pipelines|schedules)\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\/+|\/+$/g, '');
}

export function normalizeScopeOption(value: unknown): string {
  const normalized = normalizeIdentifier(value);
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

export function uniqueRunTeamOptions(values: string[]): string[] {
  return Array.from(new Set(['root', ...values.map(normalizeIdentifier).filter(Boolean)])).sort((a, b) => {
    if (a === 'root') return -1;
    if (b === 'root') return 1;
    return a.localeCompare(b);
  });
}

export function splitIdentifier(identifier: string) {
  const parts = normalizeIdentifier(identifier).split('/').filter(Boolean);
  const name = parts.pop() || '';
  return { path: parts.join('/'), name };
}

export function effectiveScheduleRunTeamPath(schedule: PipelineSchedule) {
  return normalizeIdentifier(schedule.run_team_path) || 'root';
}

export function normalizeScheduleKind(kind?: string) {
  const normalized = String(kind || '').trim().toLowerCase();
  return normalized === 'once' || normalized === 'one_time' || normalized === 'one-time' ? 'once' : 'cron';
}

export function stripCronTimezone(expression: string) {
  return String(expression || '').trim().replace(/^(?:CRON_TZ|TZ)=\S+\s+/i, '').trim();
}

function parseCronParts(expression: string) {
  const parts = stripCronTimezone(expression).split(/\s+/).filter(Boolean);
  return parts.length === 5 ? parts : null;
}

function parseCronNumber(value: string, min: number, max: number) {
  if (!/^\d+$/.test(value)) return null;
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= min && parsed <= max ? parsed : null;
}

function parseCronNumberList(value: string, min: number, max: number) {
  const parts = String(value || '').split(',').map(part => part.trim()).filter(Boolean);
  if (!parts.length) return null;
  const parsed = parts.map(part => parseCronNumber(part, min, max));
  return parsed.some(item => item === null) ? null : (parsed as number[]);
}

export function normalizeCronList(raw: string, allowedValues: string[], fallback: string) {
  const allowed = new Set(allowedValues);
  const selected = String(raw || '')
    .split(',')
    .map(value => value.trim())
    .filter(value => allowed.has(value));
  const unique = allowedValues.filter(value => selected.includes(value));
  return unique.length ? unique.join(',') : fallback;
}

export function toggleCronListValue(raw: string, value: string, allowedValues: string[], fallback: string) {
  const current = new Set(normalizeCronList(raw, allowedValues, fallback).split(','));
  if (current.has(value)) current.delete(value);
  else current.add(value);
  const next = allowedValues.filter(item => current.has(item));
  return next.length ? next.join(',') : fallback;
}

function padCronNumber(value: number) {
  return String(value).padStart(2, '0');
}

function cronTime(hour: number, minute: number) {
  return `${padCronNumber(hour)}:${padCronNumber(minute)}`;
}

function weekdayLabels(values: number[]) {
  return values
    .map(value => {
      const normalized = String(value === 7 ? 0 : value);
      return WEEKDAY_OPTIONS.find(option => option.value === normalized)?.short || normalized;
    })
    .join(', ');
}

function defaultRunAtFields() {
  const next = new Date(Date.now() + 60 * 60 * 1000);
  next.setMinutes(0, 0, 0);
  return {
    date: `${next.getFullYear()}-${padCronNumber(next.getMonth() + 1)}-${padCronNumber(next.getDate())}`,
    time: `${padCronNumber(next.getHours())}:00`,
  };
}

function zonedDateTimeFields(value?: string, timeZone?: string) {
  const fallback = defaultRunAtFields();
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: timeZone?.trim() || undefined,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).formatToParts(date);
    const get = (type: string) => parts.find(part => part.type === type)?.value || '';
    return { date: `${get('year')}-${get('month')}-${get('day')}`, time: `${get('hour')}:${get('minute')}` };
  } catch {
    return { date: value.slice(0, 10) || fallback.date, time: value.slice(11, 16) || fallback.time };
  }
}

export function buildRunAtValue(form: ScheduleFormState) {
  const date = form.runAtDate.trim();
  const time = form.runAtTime.trim();
  return date && time ? `${date}T${time}` : '';
}

function parseTimeParts(value: string) {
  const match = /^(\d{1,2}):(\d{2})$/.exec(String(value || '').trim());
  if (!match) return { hour: 2, minute: 0 };
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 ? { hour, minute } : { hour: 2, minute: 0 };
}

function cronFormFields(cronMode: CronMode, expression: string, overrides: Partial<CronFormFields> = {}): CronFormFields {
  const runAt = defaultRunAtFields();
  return {
    cronMode,
    cronTime: DEFAULT_CRON_TIME,
    cronWeekday: '1',
    cronMonthday: '1',
    cronMonth: '1',
    cronMinute: '0',
    intervalValue: '15',
    cron_expression: expression || DEFAULT_CRON,
    runAtDate: runAt.date,
    runAtTime: runAt.time,
    ...overrides,
  };
}

export function cronFormFromExpression(expression: string): CronFormFields {
  const parts = parseCronParts(expression || DEFAULT_CRON);
  if (!parts) return cronFormFields('custom', expression);

  const [minutePart, hourPart, dayPart, monthPart, weekdayPart] = parts;
  const intervalMinuteMatch = /^\*\/(\d+)$/.exec(minutePart);
  const intervalHourMatch = /^\*\/(\d+)$/.exec(hourPart);
  if (intervalMinuteMatch && hourPart === '*' && dayPart === '*' && monthPart === '*' && weekdayPart === '*') {
    return cronFormFields('minutes', expression, { intervalValue: intervalMinuteMatch[1] });
  }
  const minute = parseCronNumber(minutePart, 0, 59);
  const hour = parseCronNumber(hourPart, 0, 23);
  if (minute !== null && intervalHourMatch && dayPart === '*' && monthPart === '*' && weekdayPart === '*') {
    return cronFormFields('hourly', expression, { cronMinute: String(minute), intervalValue: intervalHourMatch[1] });
  }
  if (minute !== null && hour !== null && monthPart === '*') {
    const time = cronTime(hour, minute);
    if (dayPart === '*' && weekdayPart === '*') return cronFormFields('daily', expression, { cronTime: time, cronMinute: String(minute) });
    if (dayPart === '*' && weekdayPart === '1-5') return cronFormFields('weekdays', expression, { cronTime: time, cronMinute: String(minute) });
    const weekdays = parseCronNumberList(weekdayPart, 0, 7);
    if (dayPart === '*' && weekdays !== null) {
      return cronFormFields('weekly', expression, {
        cronTime: time,
        cronWeekday: weekdays.map(weekday => String(weekday === 7 ? 0 : weekday)).join(','),
        cronMinute: String(minute),
      });
    }
    const monthdays = parseCronNumberList(dayPart, 1, 31);
    if (monthdays !== null && weekdayPart === '*') {
      return cronFormFields('monthly', expression, { cronTime: time, cronMonthday: monthdays.join(','), cronMinute: String(minute) });
    }
  }
  const month = parseCronNumber(monthPart, 1, 12);
  const monthday = parseCronNumber(dayPart, 1, 31);
  if (minute !== null && hour !== null && month !== null && monthday !== null && weekdayPart === '*') {
    return cronFormFields('yearly', expression, {
      cronTime: cronTime(hour, minute),
      cronMonth: String(month),
      cronMonthday: String(monthday),
      cronMinute: String(minute),
    });
  }
  const hourlyMinute = parseCronNumber(minutePart, 0, 59);
  if (hourPart === '*' && dayPart === '*' && monthPart === '*' && weekdayPart === '*' && hourlyMinute !== null) {
    return cronFormFields('hourly', expression, { cronMinute: String(hourlyMinute) });
  }
  return cronFormFields('custom', expression);
}

export function buildCronExpression(form: ScheduleFormState) {
  const { hour, minute } = parseTimeParts(form.cronTime);
  const monthday = Math.min(31, Math.max(1, Number.parseInt(form.cronMonthday, 10) || 1));
  const monthdays = normalizeCronList(form.cronMonthday, MONTHDAY_VALUES, '1');
  const month = Math.min(12, Math.max(1, Number.parseInt(form.cronMonth, 10) || 1));
  const hourlyMinute = Math.min(59, Math.max(0, Number.parseInt(form.cronMinute, 10) || 0));
  const interval = Math.min(59, Math.max(1, Number.parseInt(form.intervalValue, 10) || 1));
  const weekdays = normalizeCronList(form.cronWeekday, WEEKDAY_VALUES, '1');

  switch (form.cronMode) {
    case 'once':
      return '';
    case 'minutes':
      return `*/${interval} * * * *`;
    case 'daily':
      return `${minute} ${hour} * * *`;
    case 'weekdays':
      return `${minute} ${hour} * * 1-5`;
    case 'weekly':
      return `${minute} ${hour} * * ${weekdays}`;
    case 'monthly':
      return `${minute} ${hour} ${monthdays} * *`;
    case 'hourly':
      return interval > 1 ? `${hourlyMinute} */${Math.min(23, interval)} * * *` : `${hourlyMinute} * * * *`;
    case 'yearly':
      return `${minute} ${hour} ${monthday} ${month} *`;
    case 'custom':
    default:
      return form.cron_expression.trim();
  }
}

export function friendlyCronLabel(expression?: string) {
  const parts = parseCronParts(expression || '');
  if (!parts) return expression || '—';
  const [minutePart, hourPart, dayPart, monthPart, weekdayPart] = parts;
  const intervalMinuteMatch = /^\*\/(\d+)$/.exec(minutePart);
  if (intervalMinuteMatch && hourPart === '*' && dayPart === '*' && monthPart === '*' && weekdayPart === '*') {
    return `Every ${intervalMinuteMatch[1]} minutes`;
  }
  const minute = parseCronNumber(minutePart, 0, 59);
  const hour = parseCronNumber(hourPart, 0, 23);
  const intervalHourMatch = /^\*\/(\d+)$/.exec(hourPart);
  if (minute !== null && intervalHourMatch && dayPart === '*' && monthPart === '*' && weekdayPart === '*') {
    return `Every ${intervalHourMatch[1]} hours at :${padCronNumber(minute)}`;
  }
  if (minute !== null && hour !== null && monthPart === '*') {
    const time = cronTime(hour, minute);
    if (dayPart === '*' && weekdayPart === '*') return `Daily at ${time}`;
    if (dayPart === '*' && weekdayPart === '1-5') return `Weekdays at ${time}`;
    const weekdays = parseCronNumberList(weekdayPart, 0, 7);
    if (dayPart === '*' && weekdays !== null) {
      if (weekdays.length === 1) {
        const normalizedWeekday = String(weekdays[0] === 7 ? 0 : weekdays[0]);
        const label = WEEKDAY_OPTIONS.find(option => option.value === normalizedWeekday)?.label || 'Monday';
        return `${label}s at ${time}`;
      }
      return `Weekly on ${weekdayLabels(weekdays)} at ${time}`;
    }
    const monthdays = parseCronNumberList(dayPart, 1, 31);
    if (monthdays !== null && weekdayPart === '*') {
      return monthdays.length === 1 ? `Monthly on day ${monthdays[0]} at ${time}` : `Monthly on days ${monthdays.join(', ')} at ${time}`;
    }
  }
  const month = parseCronNumber(monthPart, 1, 12);
  const monthday = parseCronNumber(dayPart, 1, 31);
  if (minute !== null && hour !== null && month !== null && monthday !== null && weekdayPart === '*') {
    const monthLabel = MONTH_OPTIONS.find(option => option.value === String(month))?.label || `month ${month}`;
    return `Yearly on ${monthLabel} ${monthday} at ${cronTime(hour, minute)}`;
  }
  const hourlyMinute = parseCronNumber(minutePart, 0, 59);
  if (hourPart === '*' && dayPart === '*' && monthPart === '*' && weekdayPart === '*' && hourlyMinute !== null) {
    return `Hourly at :${padCronNumber(hourlyMinute)}`;
  }
  return expression || '—';
}

export function variablesToText(variables?: Record<string, string>) {
  if (!variables) return '';
  return Object.entries(variables)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join('\n');
}

export function parseVariablesText(raw: string) {
  const variables: Record<string, string> = {};
  const invalidLines: number[] = [];
  raw.split(/\r?\n/).forEach((line, index) => {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) return;
    const equalsIndex = trimmed.indexOf('=');
    const key = equalsIndex > 0 ? trimmed.slice(0, equalsIndex).trim() : '';
    if (!key || !/^[A-Za-z0-9_.-]+$/.test(key)) {
      invalidLines.push(index + 1);
      return;
    }
    variables[key] = trimmed.slice(equalsIndex + 1);
  });
  if (invalidLines.length) {
    throw new Error(`Invalid variable line${invalidLines.length === 1 ? '' : 's'}: ${invalidLines.join(', ')}`);
  }
  return variables;
}

export function defaultRunTeamForPipeline(pipeline: string, runTeams: string[]) {
  const parentPath = splitIdentifier(pipeline).path;
  return parentPath && runTeams.includes(parentPath) ? parentPath : 'root';
}

export function createEmptyForm(pipelineFilter: string, runTeams: string[] = []): ScheduleFormState {
  const pipeline = normalizeIdentifier(pipelineFilter);
  const runAt = defaultRunAtFields();
  return {
    name: '',
    description: '',
    pipeline,
    ...cronFormFromExpression(DEFAULT_CRON),
    cron_expression: DEFAULT_CRON,
    runAtDate: runAt.date,
    runAtTime: runAt.time,
    timezone: DEFAULT_TIMEZONE,
    enabled: true,
    scope: '',
    runTeamPath: defaultRunTeamForPipeline(pipeline, runTeams),
    variablesText: '',
  };
}

export function formFromSchedule(schedule: PipelineSchedule): ScheduleFormState {
  const cronExpression = schedule.cron_expression || schedule.cron || DEFAULT_CRON;
  const scheduleKind = normalizeScheduleKind(schedule.schedule_kind);
  const runAtFields = zonedDateTimeFields(schedule.run_at || schedule.next_run_at, schedule.timezone);
  const cronFields =
    scheduleKind === 'once'
      ? cronFormFields('once', '', { runAtDate: runAtFields.date, runAtTime: runAtFields.time })
      : cronFormFromExpression(cronExpression);
  return {
    name: schedule.name || '',
    description: schedule.description || '',
    pipeline: normalizeIdentifier(schedule.pipeline),
    ...cronFields,
    cron_expression: scheduleKind === 'once' ? '' : cronExpression,
    runAtDate: runAtFields.date,
    runAtTime: runAtFields.time,
    timezone: schedule.timezone || 'UTC',
    enabled: Boolean(schedule.enabled),
    scope: schedule.scope || '',
    runTeamPath: effectiveScheduleRunTeamPath(schedule),
    variablesText: variablesToText(schedule.variables),
  };
}

export function scheduleRequestFromForm(form: ScheduleFormState): ScheduleRequest {
  const pipeline = normalizeIdentifier(form.pipeline);
  const scheduleKind = form.cronMode === 'once' ? 'once' : 'cron';
  return {
    path: splitIdentifier(pipeline).path,
    name: form.name.trim(),
    description: form.description.trim(),
    pipeline,
    schedule_kind: scheduleKind,
    cron_expression: scheduleKind === 'cron' ? buildCronExpression(form) : '',
    run_at: scheduleKind === 'once' ? buildRunAtValue(form) : undefined,
    timezone: form.timezone.trim() || 'UTC',
    enabled: form.enabled,
    scope: normalizeScopeOption(form.scope),
    run_team_path: normalizeIdentifier(form.runTeamPath) || 'root',
    variables: parseVariablesText(form.variablesText),
  };
}

export function normalizeScheduleMetadata(
  pipelinePayload: Array<PipelineListItem | string>,
  teamPayload: string[],
  secretScopes: Array<string | { scope?: string; name?: string }>,
  variableScopes: Array<string | { scope?: string; name?: string }>
): ScheduleMetadata {
  const pipelines = pipelinePayload
    .map(item => normalizeIdentifier(typeof item === 'string' ? item : item.id || item.identifier))
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b));
  const scopeSet = new Set<string>(['']);
  const collectScope = (entry: string | { scope?: string; name?: string }) => {
    scopeSet.add(normalizeScopeOption(typeof entry === 'string' ? entry : entry.scope || entry.name || ''));
  };
  secretScopes.forEach(collectScope);
  variableScopes.forEach(collectScope);
  return {
    pipelines,
    teams: teamPayload.map(normalizeIdentifier).filter(Boolean).sort((a, b) => a.localeCompare(b)),
    scopes: Array.from(scopeSet).sort((a, b) => a.localeCompare(b)),
  };
}
