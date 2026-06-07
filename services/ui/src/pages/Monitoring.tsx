import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Box,
  CheckCircle2,
  Clock3,
  CircleHelp,
  Layers,
  RefreshCw,
  Server,
  ShieldCheck,
  Workflow,
  XCircle,
  Zap,
} from 'lucide-react';
import { apiClient } from '../lib/api';

type Group = {
  id: number;
  name: string;
  parent_id?: number | null;
};

type RunListItem = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  status: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
  is_complete?: boolean;
};

type RunBranchMap = Record<string, RunListItem[]>;

type ResourceCounts = {
  pipelines?: number;
  steps?: number;
  triggers?: number;
};

type ServiceStatusValue = 'ok' | 'warning' | 'error' | 'unknown';

type ServiceStatus = {
  id: string;
  label: string;
  status: ServiceStatusValue;
  message: string;
  checkedAt?: string;
};

type DispatcherStatusPayload = {
  services?: unknown[];
  runners?: unknown[];
  runner_summary?: unknown;
  runnerSummary?: unknown;
  dispatcher_error?: string;
  dispatcherError?: string;
};

type RunnerStatusValue = 'online' | 'stale' | 'disabled' | 'unknown';

type MonitoringActiveRun = {
  runId: string;
  pipeline: string;
  parentStep?: string;
  triggerId?: string;
};

type MonitoringRunner = {
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

type RunnerSummary = {
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

type GroupMetric = {
  group: Group;
  label: string;
  depth: number;
  totalRuns: number;
  successRate: number;
  totalDurationMs: number;
  averageDurationMs: number;
};

type SummaryMetric = {
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

type PipelineMetric = {
  id: string;
  pipelineName: string;
  groupLabel: string;
  totalRuns: number;
  failedRuns: number;
  successRate: number;
  averageDurationMs: number;
  totalDurationMs: number;
};

const STATUS_ORDER = ['success', 'failure', 'running', 'cancelled', 'pending', 'skipped'] as const;
const STATUS_LABELS: Record<(typeof STATUS_ORDER)[number], string> = {
  success: 'Success',
  failure: 'Failed',
  running: 'Running',
  cancelled: 'Cancelled',
  pending: 'Pending',
  skipped: 'Skipped',
};

const STATUS_BAR_CLASS: Record<(typeof STATUS_ORDER)[number], string> = {
  success: 'bg-emerald-500',
  failure: 'bg-red-500',
  running: 'bg-blue-500',
  cancelled: 'bg-orange-500',
  pending: 'bg-slate-400',
  skipped: 'bg-zinc-400',
};

const WINDOW_OPTIONS = [
  { label: '7 days', days: 7 },
  { label: '30 days', days: 30 },
  { label: '90 days', days: 90 },
  { label: 'All time', days: 0 },
];

const RUN_FETCH_LIMIT = 1000;
const MAX_GROUP_RUN_PAGES = 10;
const GROUP_BATCH_SIZE = 6;
const MAX_VISIBLE_RUNNER_RUNS = 3;
const emptyRunnerSummary: RunnerSummary = {
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

function MonitoringPage() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [runsByGroup, setRunsByGroup] = useState<Record<number, RunListItem[]>>({});
  const [resourceCounts, setResourceCounts] = useState<ResourceCounts>({});
  const [services, setServices] = useState<ServiceStatus[]>([]);
  const [runners, setRunners] = useState<MonitoringRunner[]>([]);
  const [runnerSummary, setRunnerSummary] = useState<RunnerSummary>(emptyRunnerSummary);
  const [runtimeUnavailable, setRuntimeUnavailable] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedGroupId, setSelectedGroupId] = useState<number | 'all'>('all');
  const [windowDays, setWindowDays] = useState(30);
  const [refreshedAt, setRefreshedAt] = useState<string>('');

  const fetchJson = useCallback(async <T,>(path: string): Promise<T> => {
    const response = await apiClient.fetch(path, { cache: 'no-store' });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text ? `${text} (${response.status})` : `Request failed (${response.status})`);
    }
    return (await response.json()) as T;
  }, []);

  const fetchOptionalCount = useCallback(
    async (path: string): Promise<number | undefined> => {
      try {
        const payload = await fetchJson<unknown>(path);
        return Array.isArray(payload) ? payload.length : undefined;
      } catch {
        return undefined;
      }
    },
    [fetchJson]
  );

  const fetchGroupRuns = useCallback(
    async (groupId: number): Promise<RunListItem[]> => {
      const seen = new Set<string>();
      const runs: RunListItem[] = [];
      for (let page = 0; page < MAX_GROUP_RUN_PAGES; page += 1) {
        const offset = page * RUN_FETCH_LIMIT;
        const branchMap = await fetchJson<RunBranchMap>(`/v1/runs?groupId=${groupId}&limit=${RUN_FETCH_LIMIT}&offset=${offset}`);
        const pageRuns = flattenBranchRuns(branchMap);
        pageRuns.forEach(run => {
          if (!run.run_id || seen.has(run.run_id)) return;
          seen.add(run.run_id);
          runs.push(run);
        });
        if (pageRuns.length < RUN_FETCH_LIMIT) break;
      }
      return runs.sort((a, b) => getRunTime(b) - getRunTime(a));
    },
    [fetchJson]
  );

  const fetchMonitoringRuntime = useCallback(async (): Promise<{
    services: ServiceStatus[];
    runners: MonitoringRunner[];
    runnerSummary: RunnerSummary;
    unavailable: string | null;
  }> => {
    try {
      const payload = await fetchJson<DispatcherStatusPayload>('/v1/monitoring/dispatcher');
      const servicesRaw = Array.isArray(payload?.services) ? payload.services : [];
      const runnersRaw = Array.isArray(payload?.runners) ? payload.runners : [];
      const dispatcherError = readOptionalString(payload?.dispatcher_error ?? payload?.dispatcherError);
      const runners = runnersRaw.map(normalizeMonitoringRunner).filter(runner => runner.label);
      return {
        services: servicesRaw.map(normalizeServiceStatus).filter(service => service.id),
        runners,
        runnerSummary: normalizeRunnerSummary(payload?.runner_summary ?? payload?.runnerSummary, runners),
        unavailable: dispatcherError || null,
      };
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Service status is unavailable.';
      return {
        services: [],
        runners: [],
        runnerSummary: emptyRunnerSummary,
        unavailable: message.includes('403') ? 'Runtime status is unavailable for this user.' : message,
      };
    }
  }, [fetchJson]);

  const loadMonitoringData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const groupPayload = await fetchJson<Group[]>('/v1/groups');
      const accessibleGroups = Array.isArray(groupPayload) ? groupPayload : [];

      const nextRunsByGroup: Record<number, RunListItem[]> = {};
      for (let i = 0; i < accessibleGroups.length; i += GROUP_BATCH_SIZE) {
        const batch = accessibleGroups.slice(i, i + GROUP_BATCH_SIZE);
        const results = await Promise.all(
          batch.map(async group => {
            const runs = await fetchGroupRuns(group.id);
            return [group.id, runs] as const;
          })
        );
        results.forEach(([groupId, runs]) => {
          nextRunsByGroup[groupId] = runs;
        });
      }

      const [pipelines, steps, triggers, runtime] = await Promise.all([
        fetchOptionalCount('/v1/pipelines'),
        fetchOptionalCount('/v1/steps'),
        fetchOptionalCount('/v1/overrides'),
        fetchMonitoringRuntime(),
      ]);

      setGroups(accessibleGroups);
      setRunsByGroup(nextRunsByGroup);
      setResourceCounts({ pipelines, steps, triggers });
      setServices(runtime.services);
      setRunners(runtime.runners);
      setRunnerSummary(runtime.runnerSummary);
      setRuntimeUnavailable(runtime.unavailable);
      setSelectedGroupId(current =>
        current !== 'all' && !accessibleGroups.some(group => group.id === current) ? 'all' : current
      );
      setRefreshedAt(new Date().toISOString());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load monitoring data');
    } finally {
      setLoading(false);
    }
  }, [fetchGroupRuns, fetchJson, fetchMonitoringRuntime, fetchOptionalCount]);

  useEffect(() => {
    void loadMonitoringData();
  }, [loadMonitoringData]);

  const groupContext = useMemo(() => buildGroupContext(groups), [groups]);

  const selectedGroupRuns = useMemo(() => {
    if (selectedGroupId === 'all') return allDirectRuns(runsByGroup);
    return runsForGroupAndDescendants(selectedGroupId, runsByGroup, groupContext.childrenByParent);
  }, [groupContext.childrenByParent, runsByGroup, selectedGroupId]);

  const filteredRuns = useMemo(
    () => filterRunsByWindow(selectedGroupRuns, windowDays),
    [selectedGroupRuns, windowDays]
  );

  const summary = useMemo(() => summarizeRuns(filteredRuns), [filteredRuns]);

  const groupMetrics = useMemo(() => {
    return groups
      .map(group => {
        const aggregateRuns = filterRunsByWindow(
          runsForGroupAndDescendants(group.id, runsByGroup, groupContext.childrenByParent),
          windowDays
        );
        return buildGroupMetric(group, groupContext.labels.get(group.id) || group.name, groupContext.depths.get(group.id) || 0, aggregateRuns);
      })
      .sort((a, b) => b.totalRuns - a.totalRuns || a.label.localeCompare(b.label));
  }, [groupContext.childrenByParent, groupContext.depths, groupContext.labels, groups, runsByGroup, windowDays]);

  const visibleGroupMetrics = useMemo(
    () => groupMetrics.filter(metric => metric.totalRuns > 0).slice(0, 10),
    [groupMetrics]
  );

  const pipelineMetrics = useMemo(
    () => buildPipelineMetrics(filteredRuns, groups, runsByGroup, groupContext.labels).slice(0, 8),
    [filteredRuns, groupContext.labels, groups, runsByGroup]
  );

  const dailyBuckets = useMemo(() => buildDailyBuckets(filteredRuns, windowDays), [filteredRuns, windowDays]);
  const statusCounts = useMemo(() => statusCountsFromSummary(summary), [summary]);
  const selectedGroupLabel = selectedGroupId === 'all' ? 'All accessible groups' : groupContext.labels.get(selectedGroupId) || 'Selected group';

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto w-full max-w-[1500px] px-4 py-5 sm:px-6 lg:px-8 space-y-6">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div className="min-w-0">
            <div className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-1.5 text-xs font-semibold text-[var(--text-secondary)]">
              <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
              Access-filtered
            </div>
            <h1 className="mt-3 text-2xl font-semibold text-[var(--text-primary)]">Monitoring</h1>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">
              {selectedGroupLabel}
              {refreshedAt ? ` - refreshed ${formatShortDateTime(refreshedAt)}` : ''}
            </p>
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <select
              value={String(selectedGroupId)}
              onChange={event => setSelectedGroupId(event.target.value === 'all' ? 'all' : Number(event.target.value))}
              className="h-10 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-secondary)] px-3 text-sm text-[var(--text-primary)] shadow-sm focus:border-[var(--border-input-focus)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent-focus-ring)] sm:min-w-64"
            >
              <option value="all">All accessible groups</option>
              {groups.map(group => (
                <option key={group.id} value={group.id}>
                  {groupContext.labels.get(group.id) || group.name}
                </option>
              ))}
            </select>
            <div className="inline-flex h-10 overflow-hidden rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
              {WINDOW_OPTIONS.map(option => (
                <button
                  key={option.label}
                  type="button"
                  onClick={() => setWindowDays(option.days)}
                  className={`px-3 text-sm font-medium transition-colors ${
                    windowDays === option.days
                      ? 'bg-[var(--bg-active)] text-[var(--text-primary)]'
                      : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
            <button
              type="button"
              onClick={() => void loadMonitoringData()}
              disabled={loading}
              className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-primary)] shadow-sm transition hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-60"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
        </div>

        {error ? (
          <div className="rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">
            {error}
          </div>
        ) : null}

        <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <MetricCard icon={<Activity />} label="Pipeline runs" value={formatNumber(summary.totalRuns)} detail={`${formatNumber(summary.runningRuns)} running`} tone="blue" />
          <MetricCard icon={<CheckCircle2 />} label="Success rate" value={formatPercent(summary.successRate)} detail={`${formatNumber(summary.successRuns)} successful`} tone="green" />
          <MetricCard icon={<Clock3 />} label="Average execution" value={formatDuration(summary.averageDurationMs)} detail={`${formatDuration(summary.totalDurationMs)} total`} tone="amber" />
          <MetricCard icon={<XCircle />} label="Failures" value={formatNumber(summary.failedRuns)} detail={`${formatNumber(summary.cancelledRuns)} cancelled`} tone="red" />
        </section>

        <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <ResourceCountCard icon={<Layers />} label="Accessible groups" value={groups.length} loading={loading} />
          <ResourceCountCard icon={<Workflow />} label="Pipelines" value={resourceCounts.pipelines} loading={loading} />
          <ResourceCountCard icon={<Box />} label="Steps" value={resourceCounts.steps} loading={loading} />
          <ResourceCountCard icon={<Zap />} label="Triggers" value={resourceCounts.triggers} loading={loading} />
        </section>

        <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(380px,0.85fr)]">
          <Panel title="Services" icon={<Server className="h-4 w-4" />}>
            <ServiceStatusGrid services={services} unavailable={runtimeUnavailable} loading={loading} />
          </Panel>
          <Panel title="Runners" icon={<Server className="h-4 w-4" />}>
            <RunnerStatusGrid runners={runners} summary={runnerSummary} unavailable={runtimeUnavailable} loading={loading} />
          </Panel>
        </section>

        <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.8fr)]">
          <Panel title="Run Trend" icon={<BarChart3 className="h-4 w-4" />}>
            <DailyRunsChart buckets={dailyBuckets} loading={loading} />
          </Panel>
          <Panel title="Status Split" icon={<Activity className="h-4 w-4" />}>
            <StatusSplit counts={statusCounts} total={summary.totalRuns} loading={loading} />
          </Panel>
        </section>

        <section className="grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(380px,0.95fr)]">
          <Panel title="Group Comparison" icon={<Layers className="h-4 w-4" />}>
            <GroupComparison
              metrics={visibleGroupMetrics}
              maxRuns={Math.max(1, ...visibleGroupMetrics.map(metric => metric.totalRuns))}
              loading={loading}
              onSelectGroup={groupId => setSelectedGroupId(groupId)}
            />
          </Panel>
          <Panel title="Pipeline Performance" icon={<Clock3 className="h-4 w-4" />}>
            <PipelinePerformance metrics={pipelineMetrics} loading={loading} />
          </Panel>
        </section>
      </div>
    </div>
  );
}

function MetricCard({
  icon,
  label,
  value,
  detail,
  tone,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  detail: string;
  tone: 'blue' | 'green' | 'amber' | 'red';
}) {
  const toneClass = {
    blue: 'text-blue-500 bg-blue-500/10 border-blue-500/20',
    green: 'text-emerald-500 bg-emerald-500/10 border-emerald-500/20',
    amber: 'text-amber-500 bg-amber-500/10 border-amber-500/20',
    red: 'text-red-500 bg-red-500/10 border-red-500/20',
  }[tone];
  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-[var(--text-secondary)]">{label}</p>
          <p className="mt-2 truncate text-3xl font-semibold text-[var(--text-primary)]">{value}</p>
        </div>
        <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-md border ${toneClass}`}>
          <span className="[&>svg]:h-5 [&>svg]:w-5">{icon}</span>
        </div>
      </div>
      <p className="mt-3 truncate text-sm text-[var(--text-secondary)]">{detail}</p>
    </div>
  );
}

function ResourceCountCard({ icon, label, value, loading }: { icon: ReactNode; label: string; value?: number; loading: boolean }) {
  return (
    <div className="flex items-center gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 shadow-sm">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-[var(--bg-tertiary)] text-[var(--text-accent)] [&>svg]:h-4 [&>svg]:w-4">
        {icon}
      </span>
      <div className="min-w-0">
        <p className="text-sm font-medium text-[var(--text-secondary)]">{label}</p>
        <p className="text-lg font-semibold text-[var(--text-primary)]">{loading ? '...' : value == null ? 'N/A' : formatNumber(value)}</p>
      </div>
    </div>
  );
}

function Panel({ title, icon, children }: { title: string; icon: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm">
      <div className="flex h-12 items-center gap-2 border-b border-[var(--border-primary)] px-4">
        <span className="text-[var(--text-accent)]">{icon}</span>
        <h2 className="text-sm font-semibold text-[var(--text-primary)]">{title}</h2>
      </div>
      <div className="p-4">{children}</div>
    </section>
  );
}

function DailyRunsChart({ buckets, loading }: { buckets: Array<{ label: string; runs: number; failures: number; averageDurationMs: number }>; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading trend" />;
  if (!buckets.length || buckets.every(bucket => bucket.runs === 0)) return <EmptyBlock label="No runs in range" />;

  const maxRuns = Math.max(1, ...buckets.map(bucket => bucket.runs));
  const maxDuration = Math.max(1, ...buckets.map(bucket => bucket.averageDurationMs));
  const points = buckets.map((bucket, index) => {
    const x = buckets.length === 1 ? 50 : (index / (buckets.length - 1)) * 100;
    const y = 100 - (bucket.averageDurationMs / maxDuration) * 82 - 8;
    return `${x},${y}`;
  });

  return (
    <div className="h-72">
      <div className="relative h-56 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 pb-8 pt-4">
        <svg className="absolute inset-0 h-full w-full overflow-visible px-3 pb-8 pt-4" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
          <polyline fill="none" stroke="rgba(99,102,241,0.9)" strokeWidth="2.5" points={points.join(' ')} vectorEffect="non-scaling-stroke" />
        </svg>
        <div className="relative z-10 flex h-full items-end gap-2">
          {buckets.map(bucket => (
            <div key={bucket.label} className="flex min-w-0 flex-1 flex-col items-center justify-end gap-2">
              <div className="flex h-36 w-full items-end rounded-sm bg-[var(--bg-tertiary)]/70">
                <div
                  className="w-full rounded-sm bg-blue-500/80 transition-all"
                  style={{ height: `${Math.max(4, (bucket.runs / maxRuns) * 100)}%` }}
                  title={`${bucket.label}: ${bucket.runs} run(s), ${bucket.failures} failed`}
                />
              </div>
              <span className="w-full truncate text-center text-[11px] text-[var(--text-secondary)]">{bucket.label}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-[var(--text-secondary)]">
        <span className="inline-flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-sm bg-blue-500" /> Runs</span>
        <span className="inline-flex items-center gap-2"><span className="h-0.5 w-6 rounded-full bg-indigo-500" /> Avg execution</span>
      </div>
    </div>
  );
}

function StatusSplit({ counts, total, loading }: { counts: Record<(typeof STATUS_ORDER)[number], number>; total: number; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading statuses" />;
  if (total === 0) return <EmptyBlock label="No status data" />;

  return (
    <div className="space-y-5">
      <div className="flex h-5 overflow-hidden rounded-full bg-[var(--bg-tertiary)]">
        {STATUS_ORDER.map(status => {
          const width = (counts[status] / total) * 100;
          if (width <= 0) return null;
          return <div key={status} className={`${STATUS_BAR_CLASS[status]}`} style={{ width: `${width}%` }} title={`${STATUS_LABELS[status]}: ${counts[status]}`} />;
        })}
      </div>
      <div className="grid gap-x-5 gap-y-2 sm:grid-cols-2">
        {STATUS_ORDER.map(status => (
          <div key={status} className="flex items-center justify-between gap-3 py-1.5">
            <span className="inline-flex min-w-0 items-center gap-2 text-sm text-[var(--text-secondary)]">
              <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${STATUS_BAR_CLASS[status]}`} />
              <span className="truncate">{STATUS_LABELS[status]}</span>
            </span>
            <span className="text-sm font-semibold text-[var(--text-primary)]">{formatNumber(counts[status])}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function ServiceStatusGrid({
  services,
  unavailable,
  loading,
}: {
  services: ServiceStatus[];
  unavailable: string | null;
  loading: boolean;
}) {
  if (loading) return <EmptyBlock label="Loading services" />;
  if (!services.length) return <EmptyBlock label={unavailable || 'No service status available'} />;

  return (
    <div className="space-y-4">
      {unavailable ? (
        <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800/70 dark:bg-amber-950/30 dark:text-amber-100">
          {unavailable}
        </div>
      ) : null}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {services.map(service => (
          <ServiceStatusItem key={service.id} service={service} />
        ))}
      </div>
    </div>
  );
}

function ServiceStatusItem({ service }: { service: ServiceStatus }) {
  return (
    <div className="min-w-0 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className={`mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-md ${serviceStatusIconClass(service.status)}`}>
            <ServiceStatusIcon status={service.status} />
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-[var(--text-primary)]">{service.label}</p>
            <p className="mt-1 line-clamp-2 text-xs leading-5 text-[var(--text-secondary)]">{service.message || 'No status message.'}</p>
          </div>
        </div>
        <span className={`shrink-0 rounded-md border px-2 py-1 text-xs font-semibold ${serviceStatusPillClass(service.status)}`}>
          {formatServiceStatusLabel(service.status)}
        </span>
      </div>
    </div>
  );
}

function ServiceStatusIcon({ status }: { status: ServiceStatusValue }) {
  const className = 'h-4 w-4';
  if (status === 'ok') return <CheckCircle2 className={className} />;
  if (status === 'warning') return <AlertTriangle className={className} />;
  if (status === 'error') return <XCircle className={className} />;
  return <CircleHelp className={className} />;
}

function formatServiceStatusLabel(status: ServiceStatusValue) {
  if (status === 'ok') return 'OK';
  if (status === 'warning') return 'Warning';
  if (status === 'error') return 'Error';
  return 'Unknown';
}

function serviceStatusIconClass(status: ServiceStatusValue) {
  if (status === 'ok') return 'bg-emerald-500/10 text-emerald-500';
  if (status === 'warning') return 'bg-amber-500/10 text-amber-500';
  if (status === 'error') return 'bg-red-500/10 text-red-500';
  return 'bg-slate-500/10 text-slate-500';
}

function serviceStatusPillClass(status: ServiceStatusValue) {
  if (status === 'ok') return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300';
  if (status === 'warning') return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300';
  if (status === 'error') return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300';
  return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300';
}

function RunnerStatusGrid({
  runners,
  summary,
  unavailable,
  loading,
}: {
  runners: MonitoringRunner[];
  summary: RunnerSummary;
  unavailable: string | null;
  loading: boolean;
}) {
  if (loading) return <EmptyBlock label="Loading runners" />;
  if (!runners.length) return <EmptyBlock label={unavailable || 'No runners registered'} />;

  return (
    <div className="space-y-4">
      {unavailable ? (
        <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800/70 dark:bg-amber-950/30 dark:text-amber-100">
          {unavailable}
        </div>
      ) : null}
      <div className="grid grid-cols-2 gap-3">
        <RuntimeMini label="Total" value={formatNumber(summary.total)} />
        <RuntimeMini label="Online" value={formatNumber(summary.online)} />
        <RuntimeMini label="K8s" value={formatNumber(summary.kubernetes)} />
        <RuntimeMini label="Docker" value={formatNumber(summary.docker)} />
        <RuntimeMini label="Capacity" value={formatNumber(summary.capacity)} />
        <RuntimeMini label="Active" value={formatNumber(summary.activeJobs)} />
      </div>
      <div className="divide-y divide-[var(--border-primary)]">
        {runners.map(runner => (
          <RunnerStatusItem key={runner.label} runner={runner} />
        ))}
      </div>
    </div>
  );
}

function RuntimeMini({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md bg-[var(--bg-primary)] px-3 py-2">
      <p className="truncate text-xs text-[var(--text-secondary)]">{label}</p>
      <p className="mt-1 truncate text-lg font-semibold text-[var(--text-primary)]">{value}</p>
    </div>
  );
}

function RunnerStatusItem({ runner }: { runner: MonitoringRunner }) {
  return (
    <div className="py-3 first:pt-0 last:pb-0">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-[var(--text-primary)]">{runner.label}</p>
          <p className="mt-1 text-xs text-[var(--text-secondary)]">
            {formatRunnerHeartbeat(runner.lastHeartbeatUnix)}
          </p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            <span className={`rounded-md border px-2 py-0.5 text-[11px] font-semibold ${runner.runtime === 'kubernetes' ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300' : 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300'}`}>
              {runner.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker'}
            </span>
            {runner.namespace ? <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]">ns {runner.namespace}</span> : null}
            {runner.node ? <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]">node {runner.node}</span> : null}
          </div>
        </div>
        <span className={`shrink-0 rounded-md border px-2 py-1 text-xs font-semibold ${runnerStatusPillClass(runner.status)}`}>
          {formatRunnerStatusLabel(runner.status)}
        </span>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-4 text-xs">
        <MetricMini label="Capacity" value={formatNumber(runner.capacity)} />
        <MetricMini label="Active" value={formatNumber(runner.activeJobs)} />
        <MetricMini label="Inflight" value={formatNumber(runner.inflightJobs)} />
      </div>
      {runner.activeRuns.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {runner.activeRuns.slice(0, MAX_VISIBLE_RUNNER_RUNS).map(activeRun => (
            <Link
              key={activeRun.runId}
              to={`/pipelineruns/recent?run_id=${encodeURIComponent(activeRun.runId)}`}
              title={formatActiveRunTitle(activeRun)}
              className="inline-flex max-w-full items-center rounded-md border border-blue-500/30 bg-blue-500/10 px-2 py-1 text-xs font-medium text-[var(--text-primary)] hover:border-[var(--border-accent)]"
            >
              <span className="truncate">{formatActiveRunLabel(activeRun)}</span>
            </Link>
          ))}
          {runner.activeRuns.length > MAX_VISIBLE_RUNNER_RUNS ? (
            <span className="inline-flex items-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2 py-1 text-xs font-medium text-[var(--text-secondary)]">
              +{runner.activeRuns.length - MAX_VISIBLE_RUNNER_RUNS}
            </span>
          ) : null}
        </div>
      ) : runner.activeJobs > 0 ? (
        <p className="mt-3 text-xs text-[var(--text-secondary)]">No visible active runs</p>
      ) : null}
    </div>
  );
}

function formatRunnerStatusLabel(status: RunnerStatusValue) {
  if (status === 'online') return 'Online';
  if (status === 'stale') return 'Stale';
  if (status === 'disabled') return 'Disabled';
  return 'Unknown';
}

function runnerStatusPillClass(status: RunnerStatusValue) {
  if (status === 'online') return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300';
  if (status === 'stale') return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300';
  if (status === 'disabled') return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300';
  return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300';
}

function GroupComparison({
  metrics,
  maxRuns,
  loading,
  onSelectGroup,
}: {
  metrics: GroupMetric[];
  maxRuns: number;
  loading: boolean;
  onSelectGroup: (groupId: number) => void;
}) {
  if (loading) return <EmptyBlock label="Loading groups" />;
  if (!metrics.length) return <EmptyBlock label="No group runs in range" />;

  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-left text-sm">
        <thead className="text-xs uppercase text-[var(--text-secondary)]">
          <tr className="border-b border-[var(--border-primary)]">
            <th className="py-2 pr-3 font-semibold">Group</th>
            <th className="px-3 py-2 font-semibold">Runs</th>
            <th className="px-3 py-2 font-semibold">Success</th>
            <th className="px-3 py-2 font-semibold">Avg</th>
            <th className="px-3 py-2 font-semibold">Total time</th>
          </tr>
        </thead>
        <tbody>
          {metrics.map(metric => (
            <tr key={metric.group.id} className="border-b border-[var(--border-primary)] last:border-b-0">
              <td className="max-w-[20rem] py-3 pr-3">
                <button
                  type="button"
                  onClick={() => onSelectGroup(metric.group.id)}
                  className="block max-w-full truncate text-left font-medium text-[var(--text-primary)] hover:text-[var(--text-link)]"
                  style={{ paddingLeft: `${Math.min(metric.depth, 4) * 0.5}rem` }}
                  title={metric.label}
                >
                  {metric.label}
                </button>
              </td>
              <td className="min-w-36 px-3 py-3">
                <div className="flex items-center gap-3">
                  <span className="w-10 shrink-0 font-semibold text-[var(--text-primary)]">{formatNumber(metric.totalRuns)}</span>
                  <div className="h-2 min-w-24 flex-1 overflow-hidden rounded-full bg-[var(--bg-tertiary)]">
                    <div className="h-full rounded-full bg-blue-500" style={{ width: `${Math.max(4, (metric.totalRuns / maxRuns) * 100)}%` }} />
                  </div>
                </div>
              </td>
              <td className="whitespace-nowrap px-3 py-3 text-[var(--text-primary)]">{formatPercent(metric.successRate)}</td>
              <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDuration(metric.averageDurationMs)}</td>
              <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDuration(metric.totalDurationMs)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function PipelinePerformance({ metrics, loading }: { metrics: PipelineMetric[]; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading pipelines" />;
  if (!metrics.length) return <EmptyBlock label="No pipeline runs in range" />;

  return (
    <div className="divide-y divide-[var(--border-primary)]">
      {metrics.map(metric => (
        <div key={metric.id} className="py-3 first:pt-0 last:pb-0">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-[var(--text-primary)]" title={metric.pipelineName}>{metric.pipelineName}</p>
              <p className="mt-0.5 truncate text-xs text-[var(--text-secondary)]" title={metric.groupLabel}>{metric.groupLabel}</p>
            </div>
            <span className="shrink-0 rounded-md bg-[var(--bg-tertiary)] px-2 py-1 text-xs font-semibold text-[var(--text-primary)]">
              {formatNumber(metric.totalRuns)}
            </span>
          </div>
          <div className="mt-3 grid grid-cols-3 gap-4 text-xs">
            <MetricMini label="Success" value={formatPercent(metric.successRate)} />
            <MetricMini label="Avg" value={formatDuration(metric.averageDurationMs)} />
            <MetricMini label="Total" value={formatDuration(metric.totalDurationMs)} />
          </div>
        </div>
      ))}
    </div>
  );
}

function MetricMini({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-[var(--text-secondary)]">{label}</p>
      <p className="mt-1 truncate font-semibold text-[var(--text-primary)]">{value}</p>
    </div>
  );
}

function EmptyBlock({ label }: { label: string }) {
  return (
    <div className="flex h-52 items-center justify-center rounded-md border border-dashed border-[var(--border-primary)] bg-[var(--bg-primary)] text-sm text-[var(--text-secondary)]">
      {label}
    </div>
  );
}

function flattenBranchRuns(branchMap: RunBranchMap): RunListItem[] {
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

function buildGroupContext(groups: Group[]) {
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

function allDirectRuns(runsByGroup: Record<number, RunListItem[]>): RunListItem[] {
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

function runsForGroupAndDescendants(groupId: number, runsByGroup: Record<number, RunListItem[]>, childrenByParent: Map<number | null, Group[]>): RunListItem[] {
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

function filterRunsByWindow(runs: RunListItem[], days: number): RunListItem[] {
  if (!days) return runs;
  const cutoff = Date.now() - days * 24 * 60 * 60 * 1000;
  return runs.filter(run => getRunTime(run) >= cutoff);
}

function buildGroupMetric(group: Group, label: string, depth: number, runs: RunListItem[]): GroupMetric {
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

function summarizeRuns(runs: RunListItem[]): SummaryMetric {
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

function statusCountsFromSummary(summary: SummaryMetric): Record<(typeof STATUS_ORDER)[number], number> {
  return {
    success: summary.successRuns,
    failure: summary.failedRuns,
    running: summary.runningRuns,
    cancelled: summary.cancelledRuns,
    pending: summary.pendingRuns,
    skipped: summary.skippedRuns,
  };
}

function buildPipelineMetrics(
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

function buildDailyBuckets(runs: RunListItem[], days: number) {
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

function normalizeServiceStatus(value: unknown): ServiceStatus {
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

function normalizeServiceStatusValue(value: unknown): ServiceStatusValue {
  const normalized = readString(value).trim().toLowerCase();
  if (normalized === 'ok' || normalized === 'success' || normalized === 'healthy') return 'ok';
  if (normalized === 'warning' || normalized === 'warn' || normalized === 'degraded') return 'warning';
  if (normalized === 'error' || normalized === 'failed' || normalized === 'failure' || normalized === 'unhealthy') return 'error';
  return 'unknown';
}

function normalizeMonitoringRunner(value: unknown): MonitoringRunner {
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

function normalizeMonitoringActiveRuns(value: unknown): MonitoringActiveRun[] {
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

function normalizeRunnerStatusValue(value: unknown): RunnerStatusValue {
  const normalized = readString(value).trim().toLowerCase();
  if (normalized === 'online' || normalized === 'ok' || normalized === 'healthy') return 'online';
  if (normalized === 'stale' || normalized === 'warning' || normalized === 'degraded') return 'stale';
  if (normalized === 'disabled' || normalized === 'paused') return 'disabled';
  return 'unknown';
}

function normalizeRunnerSummary(value: unknown, runners: MonitoringRunner[]): RunnerSummary {
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

function normalizeStatus(status?: string, complete?: boolean): (typeof STATUS_ORDER)[number] {
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

function getRunTime(run: RunListItem): number {
  return parseDateMs(run.started_at) || parseDateMs(run.finished_at) || 0;
}

function getRunDurationMs(run: RunListItem): number {
  const started = parseDateMs(run.started_at);
  const finished = parseDateMs(run.finished_at);
  if (started && finished && finished > started) return finished - started;
  if (started && !finished && normalizeStatus(run.status, run.is_complete) === 'running') return Math.max(0, Date.now() - started);
  return parseGoDurationMs(run.duration);
}

function parseDateMs(value?: string): number {
  if (!value) return 0;
  const time = new Date(value).getTime();
  return Number.isFinite(time) ? time : 0;
}

function parseGoDurationMs(value?: string): number {
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

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value : value == null ? '' : String(value);
}

function readOptionalString(value: unknown): string | undefined {
  const trimmed = readString(value).trim();
  return trimmed || undefined;
}

function normalizeNumber(value: unknown): number {
  const numberValue = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numberValue) ? numberValue : 0;
}

function normalizeOptionalNumber(value: unknown): number | undefined {
  const numberValue = normalizeNumber(value);
  return numberValue > 0 ? numberValue : undefined;
}

function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function formatDayLabel(date: Date): string {
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date);
}

function formatDateKey(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function formatShortDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'now';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '0s';
  const totalSeconds = Math.round(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

function formatRunnerHeartbeat(lastHeartbeatUnix?: number): string {
  if (!lastHeartbeatUnix) return 'No heartbeat yet';
  const diffSeconds = Math.max(0, Math.floor((Date.now() - lastHeartbeatUnix * 1000) / 1000));
  if (diffSeconds < 60) return `Heartbeat ${diffSeconds}s ago`;
  const minutes = Math.floor(diffSeconds / 60);
  if (minutes < 60) return `Heartbeat ${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `Heartbeat ${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `Heartbeat ${days}d ago`;
}

function formatActiveRunLabel(activeRun: MonitoringActiveRun): string {
  const pipeline = activeRun.pipeline || 'Run';
  return `${pipeline} ${truncateId(activeRun.runId, 6)}`;
}

function formatActiveRunTitle(activeRun: MonitoringActiveRun): string {
  const parts = [activeRun.pipeline || 'Run', `Run ${activeRun.runId}`];
  if (activeRun.parentStep) parts.push(`Step ${activeRun.parentStep}`);
  if (activeRun.triggerId) parts.push(`Trigger ${activeRun.triggerId}`);
  return parts.join(' - ');
}

function truncateId(value: string, length: number): string {
  const trimmed = value.trim();
  if (trimmed.length <= length) return trimmed;
  return trimmed.slice(0, length);
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value);
}

function formatPercent(value: number): string {
  return `${Math.round(value * 100)}%`;
}

export default MonitoringPage;
