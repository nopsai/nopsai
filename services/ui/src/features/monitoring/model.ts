export type Group = {
  id: number;
  name: string;
  parent_id?: number | null;
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

export type RunnerStatusValue = 'online' | 'stale' | 'disabled' | 'unknown';

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
  disabled: number;
  unknown: number;
  docker: number;
  kubernetes: number;
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  queuedJobs: number;
};

export type GroupMetric = {
  group: Group;
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
  groupLabel: string;
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

export const emptyRunnerSummary: RunnerSummary = {
  total: 0,
  online: 0,
  stale: 0,
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

export function buildGroupContext(groups: Group[]) {
  const groupById = new Map(groups.map(group => [group.id, group]));
  const childrenByParent = new Map<number | null, Group[]>();
  groups.forEach(group => {
    const parentId = group.parent_id ?? null;
    const children = childrenByParent.get(parentId) || [];
    children.push(group);
    childrenByParent.set(parentId, children);
  });

  const labels = new Map<number, string>();
  const depths = new Map<number, number>();
  groups.forEach(group => {
    const path: string[] = [];
    const visited = new Set<number>();
    let current: Group | undefined = group;
    while (current && !visited.has(current.id)) {
      visited.add(current.id);
      path.unshift(current.name);
      const parentId: number | null = current.parent_id ?? null;
      current = parentId == null ? undefined : groupById.get(parentId);
    }
    labels.set(group.id, path.join('/'));
    depths.set(group.id, Math.max(0, path.length - 1));
  });

  return { childrenByParent, labels, depths };
}

export function allDirectRuns(runsByGroup: Record<number, RunListItem[]>): RunListItem[] {
  const seen = new Set<string>();
  const runs: RunListItem[] = [];
  Object.values(runsByGroup).forEach(groupRuns => {
    groupRuns.forEach(run => {
      if (!run.run_id || seen.has(run.run_id)) return;
      seen.add(run.run_id);
      runs.push(run);
    });
  });
  return runs;
}

export function runsForGroupAndDescendants(groupId: number, runsByGroup: Record<number, RunListItem[]>, childrenByParent: Map<number | null, Group[]>): RunListItem[] {
  const groupIds = new Set<number>([groupId]);
  const queue = [groupId];
  while (queue.length) {
    const current = queue.shift();
    if (current == null) continue;
    (childrenByParent.get(current) || []).forEach(child => {
      if (groupIds.has(child.id)) return;
      groupIds.add(child.id);
      queue.push(child.id);
    });
  }

  const seen = new Set<string>();
  const runs: RunListItem[] = [];
  groupIds.forEach(id => {
    (runsByGroup[id] || []).forEach(run => {
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

export function buildGroupMetric(group: Group, label: string, depth: number, runs: RunListItem[]): GroupMetric {
  const summary = summarizeRuns(runs);
  return {
    group,
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
  groups: Group[],
  runsByGroup: Record<number, RunListItem[]>,
  groupLabels: Map<number, string>
): PipelineMetric[] {
  const groupForRun = new Map<string, string>();
  groups.forEach(group => {
    (runsByGroup[group.id] || []).forEach(run => {
      groupForRun.set(run.run_id, groupLabels.get(group.id) || group.name);
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
        groupLabel: latestRun ? groupForRun.get(latestRun.run_id) || 'Unassigned' : 'Unassigned',
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
