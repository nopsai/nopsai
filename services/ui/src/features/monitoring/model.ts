export type Team = {
  id: number;
  name: string;
  kind?: 'team' | 'app' | string;
  parent_id?: number | null;
  repo_url?: string;
  repository_full_name?: string;
};

export type RunListItem = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  status: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
  is_complete?: boolean;
};

export type RunBranchMap = Record<string, RunListItem[]>;

export type ResourceCounts = {
  pipelines?: number;
  steps?: number;
  triggers?: number;
};

export type ServiceStatusValue = 'ok' | 'warning' | 'error' | 'unknown';

export type ServiceStatus = {
  id: string;
  label: string;
  status: ServiceStatusValue;
  message: string;
  checkedAt?: string;
};

export type DispatcherStatusPayload = {
  services?: unknown[];
  runners?: unknown[];
  runner_summary?: unknown;
  runnerSummary?: unknown;
  dispatcher_error?: string;
  dispatcherError?: string;
};

export type RunnerStatusValue = 'online' | 'stale' | 'unreachable' | 'disabled' | 'unknown';

export type MonitoringActiveRun = {
  runId: string;
  pipeline: string;
  parentStep?: string;
  triggerId?: string;
};

export type MonitoringRunner = {
  runnerId: string;
  label: string;
  status: RunnerStatusValue;
  runtime: string;
  namespace: string;
  node: string;
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  lastHeartbeatUnix?: number;
  allowDispatch: boolean;
  activeRuns: MonitoringActiveRun[];
};

export type RunnerSummary = {
  total: number;
  online: number;
  stale: number;
  unreachable: number;
  disabled: number;
  unknown: number;
  docker: number;
  kubernetes: number;
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  queuedJobs: number;
};

export type TeamMetric = {
  team: Team;
  label: string;
  depth: number;
  totalRuns: number;
  successRate: number;
  totalDurationMs: number;
  averageDurationMs: number;
};

export type SummaryMetric = {
  totalRuns: number;
  successRuns: number;
  failedRuns: number;
  runningRuns: number;
  cancelledRuns: number;
  pendingRuns: number;
  skippedRuns: number;
  successRate: number;
  totalDurationMs: number;
  averageDurationMs: number;
};

export type PipelineMetric = {
  id: string;
  pipelineName: string;
  teamLabel: string;
  totalRuns: number;
  failedRuns: number;
  successRate: number;
  averageDurationMs: number;
  totalDurationMs: number;
};

export type RunStatus = 'success' | 'failure' | 'running' | 'cancelled' | 'pending' | 'skipped';

export type DailyBucket = {
  label: string;
  runs: number;
  failures: number;
  averageDurationMs: number;
};

export type MonitoringTab =
  | 'overview'
  | 'runs'
  | 'pipelines'
  | 'steps-tasks'
  | 'triggers'
  | 'external-triggers'
  | 'runners'
  | 'ai-usage'
  | 'reliability'
  | 'efficiency'
  | 'security';

export type MonitoringWindow = {
  from: string;
  to: string;
};

export type MonitoringDurationStats = {
  average_seconds?: number;
  median_seconds?: number;
  p95_seconds?: number;
  p99_seconds?: number;
  max_seconds?: number;
  total_seconds?: number;
};

export type MonitoringRunRef = {
  run_id: string;
  pipeline_path?: string;
  pipeline_name?: string;
  status?: string;
  duration_seconds?: number;
};

export type MonitoringRunRow = MonitoringRunRef & {
  team_name?: string;
  repo?: string;
  ref?: string;
  commit_sha?: string;
  trigger_source?: string;
  external_trigger_id?: string;
  schedule_id?: string;
  failure_reason?: string;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
  queue_seconds?: number;
  end_to_end_seconds?: number;
};

export type MonitoringNamedCount = {
  key: string;
  label: string;
  count: number;
  failed?: number;
  tokens?: number;
  rate?: number;
  seconds?: number;
};

export type MonitoringTimeBucket = {
  key: string;
  label: string;
  runs: number;
  failures?: number;
  average_duration_seconds?: number;
  total_duration_seconds?: number;
};

export type MonitoringRunnerHistoryBucket = {
  key: string;
  label: string;
  capacity?: number;
  active_jobs?: number;
  inflight_jobs?: number;
  queued_jobs?: number;
  utilization?: number;
};

export type MonitoringRunnerHistory = {
  window?: MonitoringWindow;
  buckets?: MonitoringRunnerHistoryBucket[];
};

export type MonitoringHeatmapCell = {
  day_of_week: number;
  hour: number;
  runs: number;
  failures?: number;
};

export type MonitoringSummary = {
  window?: MonitoringWindow;
  total_runs?: number;
  successful_runs?: number;
  failed_runs?: number;
  cancelled_runs?: number;
  running_runs?: number;
  pending_runs?: number;
  waiting_approval_runs?: number;
  skipped_runs?: number;
  success_rate?: number;
  failure_rate?: number;
  average_duration_seconds?: number;
  median_duration_seconds?: number;
  p95_duration_seconds?: number;
  p99_duration_seconds?: number;
  longest_run?: MonitoringRunRef;
  total_runtime_seconds?: number;
  total_steps_executed?: number;
  total_tasks_executed?: number;
  active_runners?: number;
  queued_jobs?: number;
  runner_utilization?: number;
  external_trigger_invocations?: number;
  notification_failures?: number;
  estimated_ai_tokens?: number;
  runner_summary?: RunnerSummary;
  dispatcher_error?: string;
  compare_previous_period_enabled?: boolean;
};

export type MonitoringRunAnalytics = {
  window?: MonitoringWindow;
  runs_over_time?: MonitoringTimeBucket[];
  status_split?: MonitoringNamedCount[];
  trigger_source_split?: MonitoringNamedCount[];
  failure_reasons?: MonitoringNamedCount[];
  duration?: MonitoringDurationStats;
  queue_time?: MonitoringDurationStats;
  end_to_end_time?: MonitoringDurationStats;
  longest_runs?: MonitoringRunRow[];
  run_heatmap?: MonitoringHeatmapCell[];
  rerun_count?: number;
  timeout_count?: number;
  recent_runs?: MonitoringRunRow[];
};

export type MonitoringPerformanceRow = {
  key: string;
  pipeline_path?: string;
  pipeline_name?: string;
  step_name?: string;
  task_name?: string;
  total_runs?: number;
  successful_runs?: number;
  failed_runs?: number;
  cancelled_runs?: number;
  timeout_runs?: number;
  success_rate?: number;
  failure_rate?: number;
  average_duration_seconds?: number;
  median_duration_seconds?: number;
  p95_duration_seconds?: number;
  p99_duration_seconds?: number;
  max_duration_seconds?: number;
  total_duration_seconds?: number;
  average_queue_seconds?: number;
};

export type MonitoringPerformanceResponse = {
  window?: MonitoringWindow;
  items?: MonitoringPerformanceRow[];
};

export type MonitoringTriggerAnalytics = {
  window?: MonitoringWindow;
  trigger_sources?: MonitoringNamedCount[];
  trigger_source_trend?: MonitoringTimeBucket[];
  failures_by_trigger_source?: MonitoringNamedCount[];
  duration_by_trigger_source?: MonitoringNamedCount[];
  token_by_trigger_source?: MonitoringNamedCount[];
  trigger_source_reliability?: MonitoringNamedCount[];
};

export type MonitoringExternalTriggerLastFired = {
  id: string;
  name: string;
  enabled?: boolean;
  last_used_at?: string;
  rate_limit?: string;
};

export type MonitoringExternalTriggerAnalytics = {
  window?: MonitoringWindow;
  total_external_triggers?: number;
  enabled_external_triggers?: number;
  disabled_external_triggers?: number;
  invocation_count?: number;
  successful_invocations?: number;
  failed_invocations?: number;
  pending_invocations?: number;
  invocation_to_run_rate?: number;
  most_fired_triggers?: MonitoringNamedCount[];
  top_callers?: MonitoringNamedCount[];
  error_reasons?: MonitoringNamedCount[];
  idempotency_conflicts?: number;
  last_fired_triggers?: MonitoringExternalTriggerLastFired[];
  rate_limit_violations?: number;
  rate_limit_violation_triggers?: MonitoringNamedCount[];
};

export type MonitoringAIUsage = {
  window?: MonitoringWindow;
  total_prompt_tokens?: number;
  total_completion_tokens?: number;
  total_tokens?: number;
  exact_tokens?: number;
  estimated_tokens?: number;
  exact_token_events?: number;
  estimated_token_events?: number;
  assistant_chat_tokens?: number;
  assistant_chat_messages?: number;
  by_pipeline?: MonitoringNamedCount[];
  by_step?: MonitoringNamedCount[];
  by_task?: MonitoringNamedCount[];
  by_feature?: MonitoringNamedCount[];
  by_provider?: MonitoringNamedCount[];
  by_profile?: MonitoringNamedCount[];
  by_model?: MonitoringNamedCount[];
  by_subject?: MonitoringNamedCount[];
  trend?: MonitoringTimeBucket[];
  top_token_runs?: MonitoringNamedCount[];
};

export type MonitoringReliability = {
  window?: MonitoringWindow;
  recent_failures?: MonitoringRunRow[];
  failure_reasons?: MonitoringNamedCount[];
  repeated_failure_pipelines?: MonitoringPerformanceRow[];
  flaky_pipelines?: MonitoringPerformanceRow[];
  stuck_runs?: MonitoringRunRow[];
  approvals_waiting_too_long?: MonitoringNamedCount[];
  notification_failures?: MonitoringNamedCount[];
  failed_external_invocations?: MonitoringNamedCount[];
};

export type MonitoringEfficiency = {
  window?: MonitoringWindow;
  total_runtime_seconds?: number;
  total_runner_minutes?: number;
  total_ai_tokens?: number;
  token_by_pipeline?: MonitoringNamedCount[];
  token_by_team?: MonitoringNamedCount[];
  token_by_step?: MonitoringNamedCount[];
  token_heavy_low_success_pipelines?: MonitoringPerformanceRow[];
  frequent_reruns?: MonitoringPerformanceRow[];
  high_queue_teams?: MonitoringNamedCount[];
  recommendations?: string[];
};

export type MonitoringSecurity = {
  window?: MonitoringWindow;
  runs_by_requester?: MonitoringNamedCount[];
  runs_by_effective_subject?: MonitoringNamedCount[];
  external_trigger_callers?: MonitoringNamedCount[];
  service_account_runs?: MonitoringNamedCount[];
  high_risk_failed_pipelines?: MonitoringPerformanceRow[];
};

export type MonitoringSavedView = {
  id: string;
  name: string;
  owner_subject_type?: string;
  owner_subject_id?: string;
  visibility?: 'private' | 'team' | 'workspace';
  team_id?: number;
  filters?: Record<string, unknown>;
  columns?: string[];
  source?: string;
  managed_by_config_repo?: boolean;
  created_at?: string;
  updated_at?: string;
};

export type MonitoringAlertEvent = {
  id: string;
  rule_id?: string;
  status: string;
  value?: number;
  message?: string;
  started_at?: string;
  resolved_at?: string;
  created_at?: string;
};

export type MonitoringAlertRule = {
  id: string;
  name: string;
  description?: string;
  enabled?: boolean;
  visibility?: 'private' | 'team' | 'workspace';
  severity?: 'info' | 'warning' | 'critical';
  metric?: string;
  comparator?: string;
  threshold?: number;
  window_seconds?: number;
  filters?: Record<string, unknown>;
  source?: string;
  managed_by_config_repo?: boolean;
  created_at?: string;
  updated_at?: string;
  last_event?: MonitoringAlertEvent;
};

export type MonitoringRecommendation = {
  id: string;
  fingerprint?: string;
  category?: string;
  severity?: 'info' | 'warning' | 'critical';
  status?: 'open' | 'acknowledged' | 'resolved';
  message: string;
  metadata?: Record<string, unknown>;
  first_seen_at?: string;
  last_seen_at?: string;
  resolved_at?: string;
};

export const emptyRunnerSummary: RunnerSummary = {
  total: 0,
  online: 0,
  stale: 0,
  unreachable: 0,
  disabled: 0,
  unknown: 0,
  docker: 0,
  kubernetes: 0,
  capacity: 0,
  activeJobs: 0,
  inflightJobs: 0,
  queuedJobs: 0,
};

export function flattenBranchRuns(branchMap: RunBranchMap): RunListItem[] {
  const seen = new Set<string>();
  const runs: RunListItem[] = [];
  Object.values(branchMap || {}).forEach(branchRuns => {
    if (!Array.isArray(branchRuns)) return;
    branchRuns.forEach(run => {
      if (!run?.run_id || seen.has(run.run_id)) return;
      seen.add(run.run_id);
      runs.push(run);
    });
  });
  return runs.sort((a, b) => getRunTime(b) - getRunTime(a));
}

export function buildTeamContext(teams: Team[]) {
  const teamById = new Map(teams.map(team => [team.id, team]));
  const childrenByParent = new Map<number | null, Team[]>();
  teams.forEach(team => {
    const parentId = team.parent_id ?? null;
    const children = childrenByParent.get(parentId) || [];
    children.push(team);
    childrenByParent.set(parentId, children);
  });

  const labels = new Map<number, string>();
  const depths = new Map<number, number>();
  teams.forEach(team => {
    const path: string[] = [];
    const visited = new Set<number>();
    let current: Team | undefined = team;
    while (current && !visited.has(current.id)) {
      visited.add(current.id);
      path.unshift(current.name);
      const parentId: number | null = current.parent_id ?? null;
      current = parentId == null ? undefined : teamById.get(parentId);
    }
    labels.set(team.id, path.join('/'));
    depths.set(team.id, Math.max(0, path.length - 1));
  });

  return { childrenByParent, labels, depths };
}

export function selectableMonitoringTeams(teams: Team[]): Team[] {
  return teams.filter(team => !isMonitoringApplicationTeam(team));
}

function isMonitoringApplicationTeam(team: Team): boolean {
  return team.kind === 'app' || Boolean(team.repo_url || team.repository_full_name) || team.name.includes('/');
}

export function allDirectRuns(runsByTeam: Record<number, RunListItem[]>): RunListItem[] {
  const seen = new Set<string>();
  const runs: RunListItem[] = [];
  Object.values(runsByTeam).forEach(teamRuns => {
    teamRuns.forEach(run => {
      if (!run.run_id || seen.has(run.run_id)) return;
      seen.add(run.run_id);
      runs.push(run);
    });
  });
  return runs;
}

export function runsForTeamAndDescendants(teamId: number, runsByTeam: Record<number, RunListItem[]>, childrenByParent: Map<number | null, Team[]>): RunListItem[] {
  const teamIds = new Set<number>([teamId]);
  const queue = [teamId];
  while (queue.length) {
    const current = queue.shift();
    if (current == null) continue;
    (childrenByParent.get(current) || []).forEach(child => {
      if (teamIds.has(child.id)) return;
      teamIds.add(child.id);
      queue.push(child.id);
    });
  }

  const seen = new Set<string>();
  const runs: RunListItem[] = [];
  teamIds.forEach(id => {
    (runsByTeam[id] || []).forEach(run => {
      if (!run.run_id || seen.has(run.run_id)) return;
      seen.add(run.run_id);
      runs.push(run);
    });
  });
  return runs;
}

export function filterRunsByWindow(runs: RunListItem[], days: number): RunListItem[] {
  if (!days) return runs;
  const cutoff = Date.now() - days * 24 * 60 * 60 * 1000;
  return runs.filter(run => getRunTime(run) >= cutoff);
}

export function buildTeamMetric(team: Team, label: string, depth: number, runs: RunListItem[]): TeamMetric {
  const summary = summarizeRuns(runs);
  return {
    team,
    label,
    depth,
    totalRuns: summary.totalRuns,
    successRate: summary.successRate,
    totalDurationMs: summary.totalDurationMs,
    averageDurationMs: summary.averageDurationMs,
  };
}

export function summarizeRuns(runs: RunListItem[]): SummaryMetric {
  const summary: SummaryMetric = {
    totalRuns: runs.length,
    successRuns: 0,
    failedRuns: 0,
    runningRuns: 0,
    cancelledRuns: 0,
    pendingRuns: 0,
    skippedRuns: 0,
    successRate: 0,
    totalDurationMs: 0,
    averageDurationMs: 0,
  };

  runs.forEach(run => {
    const status = normalizeStatus(run.status, run.is_complete);
    if (status === 'success') summary.successRuns += 1;
    else if (status === 'failure') summary.failedRuns += 1;
    else if (status === 'running') summary.runningRuns += 1;
    else if (status === 'cancelled') summary.cancelledRuns += 1;
    else if (status === 'skipped') summary.skippedRuns += 1;
    else summary.pendingRuns += 1;

    summary.totalDurationMs += getRunDurationMs(run);
  });

  const completed = summary.successRuns + summary.failedRuns + summary.cancelledRuns;
  summary.successRate = completed ? summary.successRuns / completed : 0;
  summary.averageDurationMs = summary.totalRuns ? summary.totalDurationMs / summary.totalRuns : 0;
  return summary;
}

export function statusCountsFromSummary(summary: SummaryMetric): Record<RunStatus, number> {
  return {
    success: summary.successRuns,
    failure: summary.failedRuns,
    running: summary.runningRuns,
    cancelled: summary.cancelledRuns,
    pending: summary.pendingRuns,
    skipped: summary.skippedRuns,
  };
}

export function buildPipelineMetrics(
  runs: RunListItem[],
  teams: Team[],
  runsByTeam: Record<number, RunListItem[]>,
  teamLabels: Map<number, string>
): PipelineMetric[] {
  const teamForRun = new Map<string, string>();
  teams.forEach(team => {
    (runsByTeam[team.id] || []).forEach(run => {
      teamForRun.set(run.run_id, teamLabels.get(team.id) || team.name);
    });
  });

  const buckets = new Map<string, RunListItem[]>();
  runs.forEach(run => {
    const pipelineId = [run.pipeline_path, run.pipeline_name].filter(Boolean).join('/') || run.pipeline_name || 'Unnamed pipeline';
    const bucket = buckets.get(pipelineId) || [];
    bucket.push(run);
    buckets.set(pipelineId, bucket);
  });

  return Array.from(buckets.entries())
    .map(([id, bucket]) => {
      const summary = summarizeRuns(bucket);
      const latestRun = bucket.slice().sort((a, b) => getRunTime(b) - getRunTime(a))[0];
      return {
        id,
        pipelineName: latestRun?.pipeline_name || id,
        teamLabel: latestRun ? teamForRun.get(latestRun.run_id) || 'Unassigned' : 'Unassigned',
        totalRuns: summary.totalRuns,
        failedRuns: summary.failedRuns,
        successRate: summary.successRate,
        averageDurationMs: summary.averageDurationMs,
        totalDurationMs: summary.totalDurationMs,
      };
    })
    .sort((a, b) => b.failedRuns - a.failedRuns || b.totalDurationMs - a.totalDurationMs || b.totalRuns - a.totalRuns);
}

export function buildDailyBuckets(runs: RunListItem[], days: number) {
  const bucketCount = days > 0 ? Math.min(days, 14) : 14;
  const today = startOfDay(new Date());
  const buckets = Array.from({ length: bucketCount }, (_, index) => {
    const date = new Date(today);
    date.setDate(today.getDate() - (bucketCount - 1 - index));
    const key = formatDateKey(date);
    return { key, label: formatDayLabel(date), runs: 0, failures: 0, totalDurationMs: 0, averageDurationMs: 0 };
  });
  const bucketByKey = new Map(buckets.map(bucket => [bucket.key, bucket]));

  runs.forEach(run => {
    const timestamp = getRunTime(run);
    if (!timestamp) return;
    const key = formatDateKey(new Date(timestamp));
    const bucket = bucketByKey.get(key);
    if (!bucket) return;
    bucket.runs += 1;
    if (normalizeStatus(run.status, run.is_complete) === 'failure') bucket.failures += 1;
    bucket.totalDurationMs += getRunDurationMs(run);
  });

  return buckets.map(bucket => ({
    label: bucket.label,
    runs: bucket.runs,
    failures: bucket.failures,
    averageDurationMs: bucket.runs ? bucket.totalDurationMs / bucket.runs : 0,
  }));
}

export function normalizeServiceStatus(value: unknown): ServiceStatus {
  const record = asRecord(value) || {};
  const id = readString(record.id).trim();
  return {
    id,
    label: readString(record.label).trim() || id || 'Service',
    status: normalizeServiceStatusValue(record.status),
    message: readString(record.message).trim(),
    checkedAt: readOptionalString(record.checked_at ?? record.checkedAt),
  };
}

export function normalizeServiceStatusValue(value: unknown): ServiceStatusValue {
  const normalized = readString(value).trim().toLowerCase();
  if (normalized === 'ok' || normalized === 'success' || normalized === 'healthy') return 'ok';
  if (normalized === 'warning' || normalized === 'warn' || normalized === 'degraded') return 'warning';
  if (normalized === 'error' || normalized === 'failed' || normalized === 'failure' || normalized === 'unhealthy') return 'error';
  return 'unknown';
}

export function normalizeMonitoringRunner(value: unknown): MonitoringRunner {
  const record = asRecord(value) || {};
  const runtime = readString(record.runtime).trim().toLowerCase();
  return {
    runnerId: readString(record.runner_id ?? record.runnerId).trim(),
    label: readString(record.label).trim(),
    status: normalizeRunnerStatusValue(record.status),
    runtime: runtime === 'k8s' ? 'kubernetes' : runtime || 'docker',
    namespace: readString(record.namespace).trim(),
    node: readString(record.node).trim(),
    capacity: normalizeNumber(record.capacity),
    activeJobs: normalizeNumber(record.active_jobs ?? record.activeJobs),
    inflightJobs: normalizeNumber(record.inflight_jobs ?? record.inflightJobs),
    lastHeartbeatUnix: normalizeOptionalNumber(record.last_heartbeat_unix ?? record.lastHeartbeatUnix),
    allowDispatch: Boolean(record.allow_dispatch ?? record.allowDispatch),
    activeRuns: normalizeMonitoringActiveRuns(record.active_runs ?? record.activeRuns),
  };
}

export function normalizeMonitoringActiveRuns(value: unknown): MonitoringActiveRun[] {
  let source = value;
  if (typeof source === 'string') {
    try {
      source = JSON.parse(source);
    } catch {
      return [];
    }
  }
  if (!Array.isArray(source)) return [];
  return source
    .map(item => {
      const record = asRecord(item);
      if (!record) return null;
      const runId = readString(record.run_id ?? record.runId).trim();
      if (!runId) return null;
      return {
        runId,
        pipeline: readString(record.pipeline).trim(),
        parentStep: readOptionalString(record.parent_step ?? record.parentStep),
        triggerId: readOptionalString(record.trigger_event_id ?? record.triggerEventId ?? record.trigger_id ?? record.triggerId),
      } satisfies MonitoringActiveRun;
    })
    .filter(Boolean) as MonitoringActiveRun[];
}

export function normalizeRunnerStatusValue(value: unknown): RunnerStatusValue {
  const normalized = readString(value).trim().toLowerCase();
  if (normalized === 'online' || normalized === 'ok' || normalized === 'healthy') return 'online';
  if (normalized === 'unreachable' || normalized === 'offline' || normalized === 'disconnected') return 'unreachable';
  if (normalized === 'stale' || normalized === 'warning' || normalized === 'degraded') return 'stale';
  if (normalized === 'disabled' || normalized === 'paused') return 'disabled';
  return 'unknown';
}

export function normalizeRunnerSummary(value: unknown, runners: MonitoringRunner[]): RunnerSummary {
  const record = asRecord(value);
  if (record) {
    return {
      total: normalizeNumber(record.total),
      online: normalizeNumber(record.online),
      stale: normalizeNumber(record.stale),
      unreachable: normalizeNumber(record.unreachable),
      disabled: normalizeNumber(record.disabled),
      unknown: normalizeNumber(record.unknown),
      docker: normalizeNumber(record.docker),
      kubernetes: normalizeNumber(record.kubernetes),
      capacity: normalizeNumber(record.capacity),
      activeJobs: normalizeNumber(record.active_jobs ?? record.activeJobs),
      inflightJobs: normalizeNumber(record.inflight_jobs ?? record.inflightJobs),
      queuedJobs: normalizeNumber(record.queued_jobs ?? record.queuedJobs),
    };
  }

  return runners.reduce<RunnerSummary>(
    (summary, runner) => {
      summary.total += 1;
      summary.capacity += runner.capacity;
      summary.activeJobs += runner.activeJobs;
      summary.inflightJobs += runner.inflightJobs;
      if (runner.status === 'online') summary.online += 1;
      else if (runner.status === 'stale') summary.stale += 1;
      else if (runner.status === 'unreachable') summary.unreachable += 1;
      else if (runner.status === 'disabled') summary.disabled += 1;
      else summary.unknown += 1;
      if (runner.runtime === 'kubernetes') summary.kubernetes += 1;
      else summary.docker += 1;
      return summary;
    },
    { ...emptyRunnerSummary }
  );
}

export function normalizeStatus(status?: string, complete?: boolean): RunStatus {
  const raw = (status || '').toLowerCase();
  const terminal = raw === 'success' || raw === 'failure' || raw === 'failure (ignored)' || raw === 'cancelled' || raw === 'skipped';
  if (!complete && !terminal) return 'running';
  if (raw === 'failure' || raw === 'failure (ignored)') return 'failure';
  if (raw === 'running') return 'running';
  if (raw === 'cancelled') return 'cancelled';
  if (raw === 'skipped') return 'skipped';
  if (raw === 'success') return 'success';
  return 'pending';
}

export function getRunTime(run: RunListItem): number {
  return parseDateMs(run.started_at) || parseDateMs(run.finished_at) || 0;
}

export function getRunDurationMs(run: RunListItem): number {
  const started = parseDateMs(run.started_at);
  const finished = parseDateMs(run.finished_at);
  if (started && finished && finished > started) return finished - started;
  if (started && !finished && normalizeStatus(run.status, run.is_complete) === 'running') return Math.max(0, Date.now() - started);
  return parseGoDurationMs(run.duration);
}

export function parseDateMs(value?: string): number {
  if (!value) return 0;
  const time = new Date(value).getTime();
  return Number.isFinite(time) ? time : 0;
}

export function parseGoDurationMs(value?: string): number {
  if (!value) return 0;
  const normalized = value.trim();
  if (!normalized) return 0;
  let total = 0;
  const pattern = /(\d+(?:\.\d+)?)(ns|us|\u00b5s|ms|s|m|h)/g;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(normalized)) !== null) {
    const amount = Number(match[1]);
    if (!Number.isFinite(amount)) continue;
    const unit = match[2];
    if (unit === 'h') total += amount * 60 * 60 * 1000;
    else if (unit === 'm') total += amount * 60 * 1000;
    else if (unit === 's') total += amount * 1000;
    else if (unit === 'ms') total += amount;
    else if (unit === 'us' || unit === '\u00b5s') total += amount / 1000;
    else if (unit === 'ns') total += amount / 1_000_000;
  }
  return total;
}

export function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

export function readString(value: unknown): string {
  return typeof value === 'string' ? value : value == null ? '' : String(value);
}

export function readOptionalString(value: unknown): string | undefined {
  const trimmed = readString(value).trim();
  return trimmed || undefined;
}

export function normalizeNumber(value: unknown): number {
  const numberValue = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numberValue) ? numberValue : 0;
}

export function normalizeOptionalNumber(value: unknown): number | undefined {
  const numberValue = normalizeNumber(value);
  return numberValue > 0 ? numberValue : undefined;
}

export function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

export function formatDayLabel(date: Date): string {
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date);
}

export function formatDateKey(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function formatShortDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'now';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
