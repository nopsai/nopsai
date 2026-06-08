import type { ReactNode } from 'react';
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
  Server,
  Workflow,
  XCircle,
  Zap,
} from 'lucide-react';
import type {
  DailyBucket,
  Group,
  GroupMetric,
  MonitoringActiveRun,
  MonitoringRunner,
  PipelineMetric,
  ResourceCounts,
  RunnerStatusValue,
  RunnerSummary,
  RunStatus,
  ServiceStatus,
  ServiceStatusValue,
  SummaryMetric,
} from './model';

const STATUS_ORDER: RunStatus[] = ['success', 'failure', 'running', 'cancelled', 'pending', 'skipped'];
const STATUS_LABELS: Record<RunStatus, string> = {
  success: 'Success',
  failure: 'Failed',
  running: 'Running',
  cancelled: 'Cancelled',
  pending: 'Pending',
  skipped: 'Skipped',
};
const STATUS_BAR_CLASS: Record<RunStatus, string> = {
  success: 'bg-emerald-500',
  failure: 'bg-red-500',
  running: 'bg-blue-500',
  cancelled: 'bg-orange-500',
  pending: 'bg-slate-400',
  skipped: 'bg-zinc-400',
};
const MAX_VISIBLE_RUNNER_RUNS = 3;

type MonitoringDashboardProps = {
  groups: Group[];
  resourceCounts: ResourceCounts;
  services: ServiceStatus[];
  runners: MonitoringRunner[];
  runnerSummary: RunnerSummary;
  runtimeUnavailable: string | null;
  loading: boolean;
  summary: SummaryMetric;
  dailyBuckets: DailyBucket[];
  statusCounts: Record<RunStatus, number>;
  groupMetrics: GroupMetric[];
  pipelineMetrics: PipelineMetric[];
  onSelectGroup: (groupId: number) => void;
};

export function MonitoringDashboard({
  groups,
  resourceCounts,
  services,
  runners,
  runnerSummary,
  runtimeUnavailable,
  loading,
  summary,
  dailyBuckets,
  statusCounts,
  groupMetrics,
  pipelineMetrics,
  onSelectGroup,
}: MonitoringDashboardProps) {
  return (
    <>
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
            metrics={groupMetrics}
            maxRuns={Math.max(1, ...groupMetrics.map(metric => metric.totalRuns))}
            loading={loading}
            onSelectGroup={onSelectGroup}
          />
        </Panel>
        <Panel title="Pipeline Performance" icon={<Clock3 className="h-4 w-4" />}>
          <PipelinePerformance metrics={pipelineMetrics} loading={loading} />
        </Panel>
      </section>
    </>
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

function StatusSplit({ counts, total, loading }: { counts: Record<RunStatus, number>; total: number; loading: boolean }) {
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
  return `Heartbeat ${Math.floor(hours / 24)}d ago`;
}

function formatActiveRunLabel(activeRun: MonitoringActiveRun): string {
  return `${activeRun.pipeline || 'Run'} ${truncateId(activeRun.runId, 6)}`;
}

function formatActiveRunTitle(activeRun: MonitoringActiveRun): string {
  const parts = [activeRun.pipeline || 'Run', `Run ${activeRun.runId}`];
  if (activeRun.parentStep) parts.push(`Step ${activeRun.parentStep}`);
  if (activeRun.triggerId) parts.push(`Trigger ${activeRun.triggerId}`);
  return parts.join(' - ');
}

function truncateId(value: string, length: number): string {
  const trimmed = value.trim();
  return trimmed.length <= length ? trimmed : trimmed.slice(0, length);
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value);
}

function formatPercent(value: number): string {
  return `${Math.round(value * 100)}%`;
}
