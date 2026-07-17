import {
  DEFAULT_CRON,
  DEFAULT_TIMEZONE,
  buildCronExpression,
  cronFormFromExpression,
  type CronFormFields,
  type ScheduleFormState,
} from '../schedules/model.js';

export type DashboardSummary = {
  id: string;
  team_id?: number;
  team_path: string;
  ref: string;
  slug: string;
  title: string;
  description?: string;
  visibility: string;
  refresh_policy?: Record<string, unknown>;
  current_publication_count?: number;
  last_published_at?: string | null;
  source?: string;
  config_source_path?: string;
  config_source_commit_sha?: string;
  managed_by_config_repo?: boolean;
  created_at?: string;
  updated_at?: string;
};

export type DashboardSection = {
  id: string;
  section_key: string;
  title: string;
  description?: string;
  layout?: Record<string, unknown>;
  display_order: number;
  created_at?: string;
  updated_at?: string;
};

export type DashboardSource = {
  id: string;
  section_key: string;
  pipeline_id: string;
  output_name: string;
  entry_key?: string;
  run_scope?: string;
  enabled: boolean;
  required_for_refresh: boolean;
  refresh_order: number;
  created_at?: string;
  updated_at?: string;
};

export type DashboardPublication = {
  id: string;
  section_key: string;
  entry_key: string;
  mode: 'replace' | 'append' | string;
  content: DashboardSpec;
  revision: number;
  run_id?: string;
  run_output_id?: string;
  pipeline_id?: string;
  output_name?: string;
  run_scope?: string;
  refresh_id?: string;
  source_finished_at?: string | null;
  published_at: string;
  expires_at?: string | null;
  status: string;
  stale: boolean;
};

export type DashboardEvent = {
  id: string;
  section_key: string;
  entry_key: string;
  publication_id?: string;
  revision: number;
  event_type: string;
  content?: unknown;
  run_id?: string;
  refresh_id?: string;
  created_at: string;
};

export type DashboardRefreshStatus = 'running' | 'complete' | 'partial' | 'failed' | 'cancelled' | 'timed_out' | string;

export type DashboardRefresh = {
  id: string;
  dashboard_id: string;
  dashboard_ref: string;
  trigger_type: string;
  scope_type: 'dashboard' | 'section' | 'source' | string;
  scope?: Record<string, unknown>;
  mode: 'strict' | 'best_effort' | string;
  status: DashboardRefreshStatus;
  total_sources: number;
  required_sources: number;
  queued_sources: number;
  running_sources: number;
  successful_sources: number;
  failed_sources: number;
  skipped_sources: number;
  max_concurrency: number;
  timeout_seconds: number;
  error?: string;
  started_at: string;
  finished_at?: string | null;
  timeout_at?: string | null;
  created_at: string;
  updated_at: string;
  sources?: DashboardRefreshSource[];
};

export type DashboardRefreshSchedule = {
  id: string;
  dashboard_id: string;
  dashboard_ref: string;
  name: string;
  description?: string;
  cron: string;
  cron_expression: string;
  timezone: string;
  enabled: boolean;
  scope_type: 'dashboard' | 'section' | 'source' | string;
  scope?: Record<string, unknown>;
  mode: 'strict' | 'best_effort' | string;
  run_scope?: string;
  variables?: Record<string, string>;
  max_concurrency: number;
  timeout_seconds: number;
  next_run_at?: string | null;
  last_refresh_id?: string;
  last_status?: string;
  service_account_id: string;
  source: string;
  config_source_path?: string;
  config_source_commit_sha?: string;
  managed_by_config_repo: boolean;
  created_at: string;
  updated_at: string;
};

export type DashboardRefreshSource = {
  id: string;
  refresh_id: string;
  source_binding_id?: string;
  pipeline_id: string;
  output_name: string;
  section_key: string;
  entry_key?: string;
  run_scope?: string;
  run_id?: string;
  required: boolean;
  status: string;
  error?: string;
  started_at?: string | null;
  finished_at?: string | null;
  created_at: string;
  updated_at: string;
};

export type DashboardRefreshFormState = {
  scopeType: 'dashboard' | 'section' | 'source';
  sectionKey: string;
  sourceID: string;
  mode: 'strict' | 'best_effort';
  timeout: string;
  maxConcurrency: string;
};

export type DashboardRefreshScheduleFormState = DashboardRefreshFormState & CronFormFields & {
  name: string;
  description: string;
  timezone: string;
  enabled: boolean;
};

export type DashboardView = {
  dashboard: DashboardSummary;
  sections: DashboardSection[];
  publications: DashboardPublication[];
  sources: DashboardSource[];
};

export type DashboardSpec = {
  version?: string;
  title?: string;
  blocks?: DashboardBlock[];
};

export type DashboardBlock = {
  type?: string;
  title?: string;
  text?: string;
  tone?: string;
  status?: string;
  label?: string;
  value?: string;
  href?: string;
  items?: DashboardBlockItem[];
  columns?: DashboardTableColumn[];
  rows?: Array<Record<string, unknown>>;
  progress?: DashboardProgress;
  chart?: DashboardChart;
};

export type DashboardBlockItem = {
  label?: string;
  value?: string;
  text?: string;
  status?: string;
  tone?: string;
  href?: string;
};

export type DashboardTableColumn = {
  key: string;
  label: string;
};

export type DashboardProgress = {
  value: number;
  max?: number;
  unit?: string;
};

export type DashboardChart = {
  type: 'line' | 'bar' | 'area' | 'pie' | 'donut' | string;
  unit?: string;
  time_window?: { from?: string; to?: string };
  aggregation_interval?: string;
  missing_values?: 'gap' | 'zero' | 'null' | 'previous' | string;
  dimensions?: Record<string, string>;
  series?: DashboardChartSeries[];
};

export type DashboardChartSeries = {
  key: string;
  label?: string;
  team?: string;
  environment?: string;
  unit?: string;
  color?: string;
  points?: DashboardSeriesPoint[];
};

export type DashboardSeriesPoint = {
  timestamp?: string;
  label?: string;
  value?: number | null;
};

export type DashboardFormState = {
  teamPath: string;
  slug: string;
  title: string;
  description: string;
  visibility: string;
  pipelineIDs: string[];
  pipelineScopes: Record<string, string>;
  sectionKey: string;
  sectionTitle: string;
  sectionDescription: string;
};

export type DashboardSectionSeed = {
  sectionKey: string;
  title?: string;
  description?: string;
  displayOrder?: number;
};

export type DashboardSectionFormState = {
  sectionKey: string;
  title: string;
  description: string;
  displayOrder: string;
};

export type DashboardSourceFormState = {
  sectionKey: string;
  pipelineID: string;
  outputName: string;
  entryKey: string;
  runScope: string;
  enabled: boolean;
  requiredForRefresh: boolean;
  refreshOrder: string;
};

export function createDashboardForm(teamPath = ''): DashboardFormState {
  return {
    teamPath,
    slug: '',
    title: '',
    description: '',
    visibility: 'team',
    pipelineIDs: [],
    pipelineScopes: {},
    sectionKey: 'overview',
    sectionTitle: 'Overview',
    sectionDescription: '',
  };
}

export function formFromDashboard(dashboard: DashboardSummary): DashboardFormState {
  return {
    teamPath: dashboard.team_path || '',
    slug: dashboard.slug || '',
    title: dashboard.title || '',
    description: dashboard.description || '',
    visibility: dashboard.visibility || 'team',
    pipelineIDs: [],
    pipelineScopes: {},
    sectionKey: 'overview',
    sectionTitle: 'Overview',
    sectionDescription: '',
  };
}

export function dashboardRequestFromForm(
  form: DashboardFormState,
  options: { includeSections?: boolean; sections?: DashboardSectionSeed[] } = {}
) {
  const request: {
    team_path: string;
    slug: string;
    title: string;
    description: string;
    visibility: string;
    sections?: Array<{
      section_key: string;
      title: string;
      description?: string;
      display_order: number;
    }>;
  } = {
    team_path: form.teamPath.trim(),
    slug: form.slug.trim(),
    title: form.title.trim(),
    description: form.description.trim(),
    visibility: form.visibility || 'team',
  };
  if (options.includeSections ?? true) {
    const sectionSeeds = options.sections && options.sections.length > 0
      ? options.sections
      : [
          {
            sectionKey: form.sectionKey.trim() || 'overview',
            title: form.sectionTitle.trim() || titleFromKey(form.sectionKey || 'overview'),
            description: form.sectionDescription.trim(),
            displayOrder: 0,
          },
        ];
    request.sections = sectionSeeds.map((section, index) => {
      const sectionKey = section.sectionKey.trim() || 'overview';
      return {
        section_key: sectionKey,
        title: section.title?.trim() || titleFromKey(sectionKey),
        description: section.description?.trim() || '',
        display_order: section.displayOrder ?? index * 10,
      };
    });
  }
  return request;
}

export function createSectionForm(displayOrder = 0): DashboardSectionFormState {
  return {
    sectionKey: '',
    title: '',
    description: '',
    displayOrder: String(displayOrder),
  };
}

export function sectionFormFromSection(section: DashboardSection): DashboardSectionFormState {
  return {
    sectionKey: section.section_key,
    title: section.title,
    description: section.description || '',
    displayOrder: String(section.display_order ?? 0),
  };
}

export function sectionRequestFromForm(form: DashboardSectionFormState) {
  const sectionKey = form.sectionKey.trim();
  return {
    section_key: sectionKey,
    title: form.title.trim() || titleFromKey(sectionKey),
    description: form.description.trim(),
    display_order: Number.parseInt(form.displayOrder || '0', 10) || 0,
  };
}

export function createSourceForm(sectionKey = 'overview'): DashboardSourceFormState {
  return {
    sectionKey,
    pipelineID: '',
    outputName: '',
    entryKey: '',
    runScope: '',
    enabled: true,
    requiredForRefresh: true,
    refreshOrder: '0',
  };
}

export function createRefreshForm(sectionKey = 'overview', sourceID = ''): DashboardRefreshFormState {
  return {
    scopeType: sourceID ? 'source' : sectionKey ? 'section' : 'dashboard',
    sectionKey,
    sourceID,
    mode: 'strict',
    timeout: '45m',
    maxConcurrency: '4',
  };
}

export function createRefreshScheduleForm(
  scope: { scopeType?: DashboardRefreshScheduleFormState['scopeType']; sectionKey?: string; sourceID?: string } = {}
): DashboardRefreshScheduleFormState {
  const sourceID = scope.sourceID || '';
  const sectionKey = scope.sectionKey || '';
  const scopeType = scope.scopeType || (sourceID ? 'source' : sectionKey ? 'section' : 'dashboard');
  return {
    ...createRefreshForm(scopeType === 'section' ? sectionKey : '', scopeType === 'source' ? sourceID : ''),
    ...cronFormFromExpression(DEFAULT_CRON),
    scopeType,
    name: '',
    description: '',
    timezone: DEFAULT_TIMEZONE,
    enabled: true,
  };
}

export function sourceFormFromSource(source: DashboardSource): DashboardSourceFormState {
  return {
    sectionKey: source.section_key,
    pipelineID: source.pipeline_id,
    outputName: source.output_name,
    entryKey: source.entry_key || '',
    runScope: source.run_scope || '',
    enabled: source.enabled,
    requiredForRefresh: source.required_for_refresh,
    refreshOrder: String(source.refresh_order ?? 0),
  };
}

export function refreshScheduleFormFromSchedule(schedule: DashboardRefreshSchedule): DashboardRefreshScheduleFormState {
  const scopeType = schedule.scope_type === 'section' || schedule.scope_type === 'source' ? schedule.scope_type : 'dashboard';
  const cronExpression = schedule.cron_expression || schedule.cron || DEFAULT_CRON;
  return {
    ...createRefreshForm(
      scopeType === 'section' ? firstStringFromScope(schedule.scope, 'section_key', 'section_keys') : '',
      scopeType === 'source' ? firstStringFromScope(schedule.scope, 'source_id', 'source_ids') : ''
    ),
    ...cronFormFromExpression(cronExpression),
    scopeType,
    mode: schedule.mode === 'best_effort' ? 'best_effort' : 'strict',
    timeout: durationFromSeconds(schedule.timeout_seconds || 2700),
    maxConcurrency: String(schedule.max_concurrency || 4),
    name: schedule.name,
    description: schedule.description || '',
    cron_expression: cronExpression,
    timezone: schedule.timezone || DEFAULT_TIMEZONE,
    enabled: schedule.enabled,
  };
}

export function sourceRequestFromForm(form: DashboardSourceFormState) {
  return {
    section_key: form.sectionKey.trim(),
    pipeline_id: form.pipelineID.trim(),
    output_name: form.outputName.trim(),
    entry_key: form.entryKey.trim(),
    run_scope: normalizeRunScope(form.runScope),
    enabled: form.enabled,
    required_for_refresh: form.requiredForRefresh,
    refresh_order: Number.parseInt(form.refreshOrder || '0', 10) || 0,
  };
}

export function refreshRequestFromForm(form: DashboardRefreshFormState) {
  return {
    scope: {
      type: form.scopeType,
      section_key: form.scopeType === 'section' ? form.sectionKey.trim() : undefined,
      source_id: form.scopeType === 'source' ? form.sourceID.trim() : undefined,
    },
    mode: form.mode,
    timeout: form.timeout.trim(),
    max_concurrency: Number.parseInt(form.maxConcurrency || '4', 10) || 4,
  };
}

export function refreshScheduleRequestFromForm(form: DashboardRefreshScheduleFormState) {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    cron_expression: refreshScheduleCronExpressionFromForm(form),
    timezone: form.timezone.trim() || 'UTC',
    enabled: form.enabled,
    scope: {
      type: form.scopeType,
      section_key: form.scopeType === 'section' ? form.sectionKey.trim() : undefined,
      source_id: form.scopeType === 'source' ? form.sourceID.trim() : undefined,
    },
    mode: form.mode,
    timeout: form.timeout.trim(),
    max_concurrency: Number.parseInt(form.maxConcurrency || '4', 10) || 4,
  };
}

export function normalizeDashboardPublication(raw: unknown): DashboardPublication {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    section_key: stringValue(record.section_key),
    entry_key: stringValue(record.entry_key),
    mode: stringValue(record.mode) || 'replace',
    content: normalizeDashboardSpec(record.content),
    revision: numberValue(record.revision, 1),
    run_id: optionalString(record.run_id),
    run_output_id: optionalString(record.run_output_id),
    pipeline_id: optionalString(record.pipeline_id),
    output_name: optionalString(record.output_name),
    run_scope: optionalString(record.run_scope),
    refresh_id: optionalString(record.refresh_id),
    source_finished_at: optionalString(record.source_finished_at),
    published_at: stringValue(record.published_at),
    expires_at: optionalString(record.expires_at),
    status: stringValue(record.status) || 'current',
    stale: Boolean(record.stale),
  };
}

export function normalizeDashboardView(raw: unknown): DashboardView {
  const record = isRecord(raw) ? raw : {};
  return {
    dashboard: normalizeDashboardSummary(record.dashboard),
    sections: Array.isArray(record.sections) ? record.sections.map(normalizeDashboardSection) : [],
    publications: Array.isArray(record.publications) ? record.publications.map(normalizeDashboardPublication) : [],
    sources: Array.isArray(record.sources) ? record.sources.map(normalizeDashboardSource) : [],
  };
}

export function normalizeDashboardRefresh(raw: unknown): DashboardRefresh {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    dashboard_id: stringValue(record.dashboard_id),
    dashboard_ref: stringValue(record.dashboard_ref),
    trigger_type: stringValue(record.trigger_type) || 'manual',
    scope_type: stringValue(record.scope_type) || 'dashboard',
    scope: isRecord(record.scope) ? record.scope : undefined,
    mode: stringValue(record.mode) || 'strict',
    status: stringValue(record.status) || 'running',
    total_sources: numberValue(record.total_sources, 0),
    required_sources: numberValue(record.required_sources, 0),
    queued_sources: numberValue(record.queued_sources, 0),
    running_sources: numberValue(record.running_sources, 0),
    successful_sources: numberValue(record.successful_sources, 0),
    failed_sources: numberValue(record.failed_sources, 0),
    skipped_sources: numberValue(record.skipped_sources, 0),
    max_concurrency: numberValue(record.max_concurrency, 0),
    timeout_seconds: numberValue(record.timeout_seconds, 0),
    error: optionalString(record.error),
    started_at: stringValue(record.started_at),
    finished_at: optionalString(record.finished_at),
    timeout_at: optionalString(record.timeout_at),
    created_at: stringValue(record.created_at),
    updated_at: stringValue(record.updated_at),
    sources: Array.isArray(record.sources) ? record.sources.map(normalizeDashboardRefreshSource) : [],
  };
}

export function normalizeDashboardRefreshSchedule(raw: unknown): DashboardRefreshSchedule {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    dashboard_id: stringValue(record.dashboard_id),
    dashboard_ref: stringValue(record.dashboard_ref),
    name: stringValue(record.name),
    description: optionalString(record.description),
    cron: stringValue(record.cron),
    cron_expression: stringValue(record.cron_expression),
    timezone: stringValue(record.timezone) || 'UTC',
    enabled: Boolean(record.enabled),
    scope_type: stringValue(record.scope_type) || 'dashboard',
    scope: isRecord(record.scope) ? record.scope : undefined,
    mode: stringValue(record.mode) || 'strict',
    run_scope: optionalString(record.run_scope),
    variables: isStringRecord(record.variables) ? record.variables : undefined,
    max_concurrency: numberValue(record.max_concurrency, 0),
    timeout_seconds: numberValue(record.timeout_seconds, 0),
    next_run_at: optionalString(record.next_run_at),
    last_refresh_id: optionalString(record.last_refresh_id),
    last_status: optionalString(record.last_status),
    service_account_id: stringValue(record.service_account_id),
    source: stringValue(record.source) || 'database',
    config_source_path: optionalString(record.config_source_path),
    config_source_commit_sha: optionalString(record.config_source_commit_sha),
    managed_by_config_repo: Boolean(record.managed_by_config_repo),
    created_at: stringValue(record.created_at),
    updated_at: stringValue(record.updated_at),
  };
}

export function normalizeDashboardRefreshSource(raw: unknown): DashboardRefreshSource {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    refresh_id: stringValue(record.refresh_id),
    source_binding_id: optionalString(record.source_binding_id),
    pipeline_id: stringValue(record.pipeline_id),
    output_name: stringValue(record.output_name),
    section_key: stringValue(record.section_key),
    entry_key: optionalString(record.entry_key),
    run_scope: optionalString(record.run_scope),
    run_id: optionalString(record.run_id),
    required: Boolean(record.required),
    status: stringValue(record.status) || 'queued',
    error: optionalString(record.error),
    started_at: optionalString(record.started_at),
    finished_at: optionalString(record.finished_at),
    created_at: stringValue(record.created_at),
    updated_at: stringValue(record.updated_at),
  };
}

export function normalizeDashboardSummary(raw: unknown): DashboardSummary {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    team_id: typeof record.team_id === 'number' ? record.team_id : undefined,
    team_path: stringValue(record.team_path),
    ref: stringValue(record.ref),
    slug: stringValue(record.slug),
    title: stringValue(record.title),
    description: optionalString(record.description),
    visibility: stringValue(record.visibility) || 'team',
    refresh_policy: isRecord(record.refresh_policy) ? record.refresh_policy : undefined,
    current_publication_count: numberValue(record.current_publication_count, 0),
    last_published_at: optionalString(record.last_published_at),
    source: optionalString(record.source),
    config_source_path: optionalString(record.config_source_path),
    config_source_commit_sha: optionalString(record.config_source_commit_sha),
    managed_by_config_repo: Boolean(record.managed_by_config_repo),
    created_at: optionalString(record.created_at),
    updated_at: optionalString(record.updated_at),
  };
}

export function normalizeDashboardSection(raw: unknown): DashboardSection {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    section_key: stringValue(record.section_key),
    title: stringValue(record.title),
    description: optionalString(record.description),
    layout: isRecord(record.layout) ? record.layout : undefined,
    display_order: numberValue(record.display_order, 0),
    created_at: optionalString(record.created_at),
    updated_at: optionalString(record.updated_at),
  };
}

export function normalizeDashboardSource(raw: unknown): DashboardSource {
  const record = isRecord(raw) ? raw : {};
  return {
    id: stringValue(record.id),
    section_key: stringValue(record.section_key),
    pipeline_id: stringValue(record.pipeline_id),
    output_name: stringValue(record.output_name),
    entry_key: optionalString(record.entry_key),
    run_scope: optionalString(record.run_scope),
    enabled: Boolean(record.enabled),
    required_for_refresh: Boolean(record.required_for_refresh),
    refresh_order: numberValue(record.refresh_order, 0),
    created_at: optionalString(record.created_at),
    updated_at: optionalString(record.updated_at),
  };
}

export function normalizeDashboardSpec(raw: unknown): DashboardSpec {
  if (!isRecord(raw)) return { blocks: [] };
  return {
    version: optionalString(raw.version),
    title: optionalString(raw.title),
    blocks: Array.isArray(raw.blocks) ? raw.blocks.filter(isRecord) as DashboardBlock[] : [],
  };
}

export function groupPublicationsBySection(publications: DashboardPublication[]) {
  return publications.reduce<Record<string, DashboardPublication[]>>((acc, publication) => {
    const key = publication.section_key || 'overview';
    acc[key] = acc[key] || [];
    acc[key].push(publication);
    return acc;
  }, {});
}

export function staleLabel(publication: DashboardPublication): string {
  if (publication.stale) return 'Stale';
  if (publication.expires_at) return `Expires ${formatDateTime(publication.expires_at)}`;
  return 'Current';
}

export function formatDateTime(value?: string | null): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

export function refreshStatusLabel(status: string): string {
  return status.replace(/_/g, ' ');
}

export function normalizeRunScope(value: string | undefined): string {
  const normalized = (value || '').trim().replace(/^\/+|\/+$/g, '');
  if (!normalized || normalized.toLowerCase() === 'default') return '';
  return normalized;
}

export function runScopeLabel(value: string | undefined): string {
  const normalized = normalizeRunScope(value);
  return normalized || 'Default scope';
}

export function refreshProgress(refresh: DashboardRefresh): number {
  if (refresh.total_sources <= 0) return 0;
  const done = refresh.successful_sources + refresh.failed_sources + refresh.skipped_sources;
  return Math.min(100, Math.round((done / refresh.total_sources) * 100));
}

export function titleFromKey(key: string): string {
  const words = key.replace(/[_-]+/g, ' ').trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return 'Section';
  return words.map(word => `${word.slice(0, 1).toUpperCase()}${word.slice(1)}`).join(' ');
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function isStringRecord(value: unknown): value is Record<string, string> {
  if (!isRecord(value)) return false;
  return Object.values(value).every(item => typeof item === 'string');
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

export function refreshScheduleCronExpressionFromForm(form: DashboardRefreshScheduleFormState): string {
  return buildCronExpression({
    ...form,
    pipeline: '',
    scope: '',
    runTeamPath: 'root',
    variablesText: '',
  } satisfies ScheduleFormState);
}

function optionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined;
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function durationFromSeconds(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '45m';
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

function firstStringFromScope(scope: Record<string, unknown> | undefined, scalarKey: string, arrayKey: string): string {
  const scalar = scope?.[scalarKey];
  if (typeof scalar === 'string') return scalar;
  const values = scope?.[arrayKey];
  if (Array.isArray(values)) {
    return values.find((value): value is string => typeof value === 'string') || '';
  }
  return '';
}
