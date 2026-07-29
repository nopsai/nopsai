import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  aiResourceMatchesTeamFilter,
} from '../system/aiResourceTeams';
import {
  effectiveScheduleRunTeamPath,
  normalizeIdentifier,
  normalizeScheduleKind,
  splitIdentifier,
  type PipelineSchedule,
} from './model';
import {
  formatTeamPath,
  sourceLabel,
  statusClass,
  statusLabel,
} from './presentation';

export type ScheduleStateFilter = 'all' | 'enabled' | 'disabled' | 'gitops' | 'once';

export const SCHEDULE_STATE_FILTERS: Array<{ value: ScheduleStateFilter; label: string }> = [
  { value: 'all', label: 'All' },
  { value: 'enabled', label: 'Enabled' },
  { value: 'disabled', label: 'Disabled' },
  { value: 'gitops', label: 'GitOps' },
  { value: 'once', label: 'One-time' },
];

export type ScheduleWorkspaceSummary = {
  total: number;
  visible: number;
  enabled: number;
  disabled: number;
  gitops: number;
  recurring: number;
  oneTime: number;
  withNextRun: number;
  pipelines: number;
};

export function scheduleDisplayName(schedule: PipelineSchedule) {
  return schedule.name || scheduleLocalName(schedule) || schedule.identifier || schedule.id;
}

export function scheduleLocalName(schedule: PipelineSchedule) {
  const fromIdentifier = splitIdentifier(schedule.identifier || '').name;
  return fromIdentifier || normalizeIdentifier(schedule.name) || normalizeIdentifier(schedule.id);
}

export function scheduleResourcePath(schedule: PipelineSchedule) {
  const runTeamPath = normalizeIdentifier(schedule.run_team_path);
  if (runTeamPath) return effectiveScheduleRunTeamPath(schedule);
  const explicitPath = normalizeIdentifier(schedule.path);
  if (explicitPath) return explicitPath.toLowerCase() === 'root' ? 'root' : explicitPath;
  const identifierPath = splitIdentifier(schedule.identifier || '').path;
  return identifierPath || 'root';
}

export function scheduleResourceID(schedule: PipelineSchedule) {
  const path = scheduleResourcePath(schedule);
  const localName = scheduleLocalName(schedule) || normalizeIdentifier(schedule.id);
  return `${path}/${localName || schedule.id}`;
}

export function scheduleResourcePathLabel(schedule: PipelineSchedule) {
  return formatTeamPath(scheduleResourcePath(schedule));
}

export function isGitOpsSchedule(schedule: PipelineSchedule) {
  return Boolean(schedule.managed_by_config_repo || sourceLabel(schedule.source) === 'GitOps');
}

export function latestScheduleRunID(schedule: PipelineSchedule) {
  return schedule.latest_run?.run_id || schedule.last_run_id || '';
}

export function latestScheduleStatus(schedule: PipelineSchedule) {
  return schedule.latest_run?.status || schedule.last_status || '';
}

export function scheduleKindLabel(schedule: PipelineSchedule) {
  return normalizeScheduleKind(schedule.schedule_kind) === 'once' ? 'One-time' : 'Recurring';
}

export function scheduleSourceDetail(schedule: PipelineSchedule) {
  if (isGitOpsSchedule(schedule)) {
    return schedule.config_source_path ? `GitOps: ${schedule.config_source_path}` : 'GitOps managed';
  }
  return sourceLabel(schedule.source);
}

export function scheduleStatusHealthClass(schedule: PipelineSchedule) {
  const classes = ['ai-resource-health'];
  const pillClass = statusClass(latestScheduleStatus(schedule));
  if (pillClass === 'runner-pill--ok') classes.push('ai-resource-health--ok');
  else if (pillClass === 'runner-pill--error') classes.push('ai-resource-health--error');
  else if (pillClass === 'runner-pill--warning') classes.push('ai-resource-health--warning');
  else classes.push('ai-resource-health--muted');
  return classes.join(' ');
}

export function scheduleStatusText(schedule: PipelineSchedule) {
  return statusLabel(latestScheduleStatus(schedule));
}

export function schedulePathOptions(schedules: PipelineSchedule[], knownTeamPaths: string[] = []) {
  const paths = new Set<string>();
  schedules.forEach(schedule => paths.add(scheduleResourcePath(schedule)));
  knownTeamPaths
    .map(normalizeIdentifier)
    .filter(path => path && path !== 'root')
    .forEach(path => paths.add(path));
  return Array.from(paths).sort((a, b) => {
    if (a === 'root') return -1;
    if (b === 'root') return 1;
    return a.localeCompare(b, undefined, { sensitivity: 'base' });
  });
}

export function scheduleMatchesSearch(schedule: PipelineSchedule, rawTerm: string) {
  const term = rawTerm.trim().toLowerCase();
  if (!term) return true;
  const variables = schedule.variables
    ? Object.entries(schedule.variables).map(([key, value]) => `${key} ${value}`).join(' ')
    : '';
  const haystack = [
    scheduleDisplayName(schedule),
    schedule.identifier,
    scheduleResourcePathLabel(schedule),
    schedule.path,
    schedule.pipeline,
    schedule.pipeline_path,
    schedule.pipeline_name,
    schedule.pipeline_version,
    schedule.description,
    schedule.cron_expression,
    schedule.cron,
    schedule.schedule_kind,
    scheduleKindLabel(schedule),
    schedule.run_at,
    schedule.timezone,
    schedule.scope,
    schedule.run_team_path,
    latestScheduleStatus(schedule),
    sourceLabel(schedule.source),
    schedule.config_source_path,
    variables,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  return haystack.includes(term);
}

export function scheduleMatchesState(schedule: PipelineSchedule, filter: ScheduleStateFilter) {
  switch (filter) {
    case 'enabled':
      return schedule.enabled;
    case 'disabled':
      return !schedule.enabled;
    case 'gitops':
      return isGitOpsSchedule(schedule);
    case 'once':
      return normalizeScheduleKind(schedule.schedule_kind) === 'once';
    case 'all':
    default:
      return true;
  }
}

export function scheduleMatchesPath(schedule: PipelineSchedule, pathFilter: string) {
  if (!pathFilter || pathFilter === AI_RESOURCE_TEAM_FILTER_ALL) return true;
  return aiResourceMatchesTeamFilter(scheduleResourceID(schedule), pathFilter);
}

export function filterSchedules({
  schedules,
  searchTerm,
  pathFilter,
  stateFilter,
}: {
  schedules: PipelineSchedule[];
  searchTerm: string;
  pathFilter: string;
  stateFilter: ScheduleStateFilter;
}) {
  return schedules
    .filter(schedule => scheduleMatchesSearch(schedule, searchTerm))
    .filter(schedule => scheduleMatchesPath(schedule, pathFilter))
    .filter(schedule => scheduleMatchesState(schedule, stateFilter))
    .sort((left, right) => scheduleResourceID(left).localeCompare(scheduleResourceID(right), undefined, { sensitivity: 'base' }));
}

export function summarizeSchedules(schedules: PipelineSchedule[], visibleSchedules = schedules): ScheduleWorkspaceSummary {
  const pipelines = new Set(visibleSchedules.map(schedule => normalizeIdentifier(schedule.pipeline)).filter(Boolean));
  return {
    total: schedules.length,
    visible: visibleSchedules.length,
    enabled: visibleSchedules.filter(schedule => schedule.enabled).length,
    disabled: visibleSchedules.filter(schedule => !schedule.enabled).length,
    gitops: visibleSchedules.filter(isGitOpsSchedule).length,
    recurring: visibleSchedules.filter(schedule => normalizeScheduleKind(schedule.schedule_kind) !== 'once').length,
    oneTime: visibleSchedules.filter(schedule => normalizeScheduleKind(schedule.schedule_kind) === 'once').length,
    withNextRun: visibleSchedules.filter(schedule => Boolean(schedule.next_run_at)).length,
    pipelines: pipelines.size,
  };
}

export function formatScheduleRatio(count: number, total: number) {
  if (total <= 0) return '0/0';
  return `${count}/${total}`;
}
