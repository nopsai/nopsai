import type { ReactNode } from 'react';
import { Boxes, GitBranch, Server, Activity } from 'lucide-react';
import {
  buildLiveRunnerRoutingRows,
  formatDispatcherRouteScope,
  getRunnerMeta,
  type DispatcherStatusState,
  type Runner,
} from './model';
import { clampPercent, formatSince, isRunnerHeartbeatStale, truncateId } from './presentation';

type DispatcherOverviewProps = {
  loading: boolean;
  status: DispatcherStatusState | null;
  nowMs: number;
  onOpenRunners: () => void;
  onOpenRouting: () => void;
};

type AttentionItem = {
  key: string;
  tone: 'error' | 'warning';
  title: string;
  detail: string;
};

export function DispatcherOverview({ loading, status, nowMs, onOpenRunners, onOpenRouting }: DispatcherOverviewProps) {
  const runners = status?.runners ?? [];
  const queuedJobs = status?.queuedJobs ?? 0;
  const activeRuns = runners.reduce((sum, runner) => sum + runner.activeJobs, 0);
  const totalCapacity = runners.reduce((sum, runner) => sum + runner.capacity, 0);
  const onlineRunners = runners.filter(runner => runnerAvailable(runner, nowMs)).length;
  const kubernetesRunners = runners.filter(runner => getRunnerMeta(runner).runtime === 'kubernetes').length;
  const dockerRunners = runners.length - kubernetesRunners;
  const capacityRows = buildScopeCapacityRows(runners);
  const attentionItems = buildAttentionItems(runners, nowMs);
  const activityItems = buildActivityItems(runners);
  const utilization = totalCapacity > 0 ? clampPercent((activeRuns / totalCapacity) * 100) : 0;

  return (
    <div className="dispatcher-overview">
      <div className="dispatcher-summary-grid dispatcher-summary-grid--wide">
        <OverviewMetric label="Online" value={onlineRunners} detail={`of ${runners.length} registered`} tone="ok" icon={<Server className="h-4 w-4" />} />
        <OverviewMetric label="Queued" value={queuedJobs} detail={queuedJobs === 1 ? 'job waiting' : 'jobs waiting'} tone="warning" icon={<GitBranch className="h-4 w-4" />} />
        <OverviewMetric label="Active Runs" value={activeRuns} detail={`across ${activeRunnerCount(runners)} runners`} tone="info" icon={<Activity className="h-4 w-4" />} />
        <OverviewMetric label="Capacity" value={totalCapacity} detail={`${Math.max(0, totalCapacity - activeRuns)} slots available`} tone="accent" icon={<Boxes className="h-4 w-4" />} />
        <OverviewMetric label="Utilization" value={`${utilization}%`} detail="current fleet load" tone="info" icon={<Activity className="h-4 w-4" />} />
      </div>

      <div className={`dispatcher-health-strip ${attentionItems.length > 0 ? 'dispatcher-health-strip--warning' : ''}`}>
        <span className="runner-dot runner-dot--ok" aria-hidden="true"></span>
        <div>
          <strong>{attentionItems.length > 0 ? `${attentionItems.length} item${attentionItems.length === 1 ? '' : 's'} need attention` : 'Dispatcher fleet is ready'}</strong>
          <span>{loading ? 'Refreshing dispatcher state...' : 'Live status is derived from registered runners and configured routes.'}</span>
        </div>
        <div className="dispatcher-health-strip__meta">
          <span><b>{dockerRunners}</b> Docker</span>
          <span><b>{kubernetesRunners}</b> Kubernetes</span>
        </div>
      </div>

      <div className="dispatcher-overview-grid">
        <div className="dispatcher-overview-stack">
          <section className="dispatcher-section-card">
            <div className="dispatcher-section-header">
              <div>
                <h3>Capacity by scope</h3>
                <p>Active workload compared with registered runner capacity</p>
              </div>
              <button className="glass-button-subtle text-xs" type="button" onClick={onOpenRouting}>
                Manage routing
              </button>
            </div>
            {capacityRows.length > 0 ? (
              <div className="dispatcher-scope-load">
                {capacityRows.map(row => (
                  <div className="dispatcher-scope-load__row" key={row.scope}>
                    <span>{formatDispatcherRouteScope(row.scope)}</span>
                    <div className="dispatcher-capacity-bar" aria-label={`${formatDispatcherRouteScope(row.scope)} capacity`}>
                      <span style={{ width: `${row.percent}%` }} />
                    </div>
                    <strong>{row.active} / {row.capacity}</strong>
                  </div>
                ))}
              </div>
            ) : (
              <div className="dispatcher-empty">No runner scopes registered.</div>
            )}
          </section>

          <section className="dispatcher-section-card">
            <div className="dispatcher-section-header">
              <div>
                <h3>Recent activity</h3>
                <p>Active dispatcher work visible in runner metadata</p>
              </div>
            </div>
            {activityItems.length > 0 ? (
              <div className="dispatcher-activity-list">
                {activityItems.map(item => (
                  <div className="dispatcher-activity" key={`${item.runnerId}-${item.runId}`}>
                    <span className="runner-dot runner-dot--ok" aria-hidden="true"></span>
                    <span>
                      <b>{item.runnerId}</b> is running <b>{item.pipeline}</b> ({truncateId(item.runId, 8)})
                    </span>
                    <small>{item.triggerId || 'manual'}</small>
                  </div>
                ))}
              </div>
            ) : (
              <div className="dispatcher-empty">No active run metadata is currently advertised.</div>
            )}
          </section>
        </div>

        <div className="dispatcher-overview-stack">
          <section className="dispatcher-section-card">
            <div className="dispatcher-section-header">
              <div>
                <h3>Needs attention</h3>
                <p>Issues that can reduce dispatch availability</p>
              </div>
            </div>
            {attentionItems.length > 0 ? (
              <div className="dispatcher-attention-list">
                {attentionItems.map(item => (
                  <div className={`dispatcher-attention dispatcher-attention--${item.tone}`} key={item.key}>
                    <span>{item.tone === 'error' ? '!' : '?'}</span>
                    <div>
                      <strong>{item.title}</strong>
                      <p>{item.detail}</p>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="dispatcher-empty">No runner availability issues detected.</div>
            )}
          </section>

          <section className="dispatcher-section-card">
            <div className="dispatcher-section-header">
              <div>
                <h3>Fleet composition</h3>
                <p>Runtime distribution and availability</p>
              </div>
            </div>
            <div className="dispatcher-summary-lines">
              <SummaryLine label="Docker runners" value={dockerRunners} />
              <SummaryLine label="Kubernetes runners" value={kubernetesRunners} />
            </div>
            <button className="glass-button-subtle mt-3 w-full justify-center" type="button" onClick={onOpenRunners}>
              Open runner fleet
            </button>
          </section>
        </div>
      </div>
    </div>
  );
}

function OverviewMetric({
  label,
  value,
  detail,
  tone,
  icon,
}: {
  label: string;
  value: number | string;
  detail: string;
  tone: 'ok' | 'warning' | 'info' | 'accent';
  icon: ReactNode;
}) {
  return (
    <article className={`dispatcher-stat-card dispatcher-stat-card--${tone}`}>
      <div className="dispatcher-stat-card__top">
        <span>{icon}</span>
        <p>{label}</p>
        <strong>{value}</strong>
      </div>
      <div className="dispatcher-stat-card__hint">{detail}</div>
    </article>
  );
}

function SummaryLine({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="dispatcher-summary-line">
      <span>{label}</span>
      <b>{value}</b>
    </div>
  );
}

function buildScopeCapacityRows(runners: Runner[]) {
  const scopeMap = new Map<string, { active: number; capacity: number }>();
  buildLiveRunnerRoutingRows(runners, { includeUnreachable: true }).forEach(row => {
    row.runners.forEach(runnerId => {
      const runner = runners.find(item => item.runnerId === runnerId);
      if (!runner) return;
      const current = scopeMap.get(row.scope) || { active: 0, capacity: 0 };
      current.active += runner.activeJobs;
      current.capacity += runner.capacity;
      scopeMap.set(row.scope, current);
    });
  });
  return Array.from(scopeMap.entries())
    .map(([scope, values]) => ({
      scope,
      ...values,
      percent: values.capacity > 0 ? clampPercent((values.active / values.capacity) * 100) : 0,
    }))
    .sort((left, right) => left.scope.localeCompare(right.scope));
}

function buildAttentionItems(runners: Runner[], nowMs: number): AttentionItem[] {
  return runners.flatMap<AttentionItem>(runner => {
    const meta = getRunnerMeta(runner);
    if (!meta.reachable) {
      return [{
        key: `${runner.runnerId}-offline`,
        tone: 'error' as const,
        title: `${runner.runnerId} is offline`,
        detail: `Last heartbeat was ${formatSince(nowMs, runner.lastHeartbeatUnix)}. It remains visible for routing and recovery.`,
      }];
    }
    if (!runner.allowDispatch) {
      return [{
        key: `${runner.runnerId}-paused`,
        tone: 'warning' as const,
        title: `${runner.runnerId} is paused`,
        detail: `${runner.capacity} capacity slot${runner.capacity === 1 ? '' : 's'} are unavailable until dispatch is resumed.`,
      }];
    }
    if (isRunnerHeartbeatStale(nowMs, runner.lastHeartbeatUnix)) {
      return [{
        key: `${runner.runnerId}-stale`,
        tone: 'error' as const,
        title: `${runner.runnerId} heartbeat is stale`,
        detail: `Last heartbeat was ${formatSince(nowMs, runner.lastHeartbeatUnix)}.`,
      }];
    }
    return [];
  }).slice(0, 4);
}

function buildActivityItems(runners: Runner[]) {
  return runners
    .flatMap(runner => getRunnerMeta(runner).activeRuns.map(run => ({
      runnerId: runner.runnerId,
      runId: run.runId,
      pipeline: run.pipeline || 'Pipeline',
      triggerId: run.triggerId,
    })))
    .slice(0, 4);
}

function runnerAvailable(runner: Runner, nowMs: number) {
  const meta = getRunnerMeta(runner);
  return meta.reachable && runner.allowDispatch && !isRunnerHeartbeatStale(nowMs, runner.lastHeartbeatUnix);
}

function activeRunnerCount(runners: Runner[]) {
  return runners.filter(runner => runner.activeJobs > 0).length;
}
