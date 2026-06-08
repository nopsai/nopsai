import { useCallback, useEffect, useMemo, useState } from 'react';
import { RefreshCw, ShieldCheck } from 'lucide-react';
import { MonitoringDashboard } from '../features/monitoring/MonitoringDashboard';
import {
  allDirectRuns,
  buildDailyBuckets,
  buildGroupContext,
  buildGroupMetric,
  buildPipelineMetrics,
  emptyRunnerSummary,
  filterRunsByWindow,
  flattenBranchRuns,
  formatShortDateTime,
  getRunTime,
  normalizeMonitoringRunner,
  normalizeRunnerSummary,
  normalizeServiceStatus,
  readOptionalString,
  runsForGroupAndDescendants,
  statusCountsFromSummary,
  summarizeRuns,
  type DispatcherStatusPayload,
  type Group,
  type MonitoringRunner,
  type ResourceCounts,
  type RunBranchMap,
  type RunListItem,
  type RunnerSummary,
  type ServiceStatus,
} from '../features/monitoring/model';
import { apiClient } from '../lib/api';

const WINDOW_OPTIONS = [
  { label: '7 days', days: 7 },
  { label: '30 days', days: 30 },
  { label: '90 days', days: 90 },
  { label: 'All time', days: 0 },
];

const RUN_FETCH_LIMIT = 1000;
const MAX_GROUP_RUN_PAGES = 10;
const GROUP_BATCH_SIZE = 6;
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

        <MonitoringDashboard
          groups={groups}
          resourceCounts={resourceCounts}
          services={services}
          runners={runners}
          runnerSummary={runnerSummary}
          runtimeUnavailable={runtimeUnavailable}
          loading={loading}
          summary={summary}
          dailyBuckets={dailyBuckets}
          statusCounts={statusCounts}
          groupMetrics={visibleGroupMetrics}
          pipelineMetrics={pipelineMetrics}
          onSelectGroup={setSelectedGroupId}
        />
      </div>
    </div>
  );
}

export default MonitoringPage;
