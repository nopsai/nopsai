import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Bell, CheckCircle2, Download, Play, RefreshCw, Save, ShieldCheck, Trash2 } from 'lucide-react';
import { MonitoringDashboard } from '../features/monitoring/MonitoringDashboard';
import {
  buildGroupContext,
  emptyRunnerSummary,
  formatShortDateTime,
  normalizeMonitoringRunner,
  normalizeRunnerSummary,
  normalizeServiceStatus,
  readOptionalString,
  type DispatcherStatusPayload,
  type Group,
  type MonitoringAlertEvent,
  type MonitoringAlertRule,
  type MonitoringAIUsage,
  type MonitoringExternalTriggerAnalytics,
  type MonitoringEfficiency,
  type MonitoringPerformanceResponse,
  type MonitoringRecommendation,
  type MonitoringReliability,
  type MonitoringRunAnalytics,
  type MonitoringRunner,
  type MonitoringRunnerHistory,
  type MonitoringSavedView,
  type MonitoringSecurity,
  type MonitoringSummary,
  type MonitoringTab,
  type MonitoringTriggerAnalytics,
  type RunnerSummary,
  type ServiceStatus,
} from '../features/monitoring/model';
import { requestMonitoringJson, sendMonitoringJson } from '../features/monitoring/api';

const WINDOW_OPTIONS = [
  { label: '7 days', days: 7 },
  { label: '30 days', days: 30 },
  { label: '90 days', days: 90 },
  { label: 'All time', days: 0 },
];

const STATUS_OPTIONS = ['all', 'success', 'failure', 'running', 'cancelled', 'pending', 'waiting_approval'];
const VIEW_STORAGE_KEY = 'nopsai.monitoring.view.v1';
const MONITORING_TABS: readonly MonitoringTab[] = [
  'overview',
  'runs',
  'pipelines',
  'steps-tasks',
  'triggers',
  'external-triggers',
  'runners',
  'ai-usage',
  'reliability',
  'efficiency',
  'security',
];

type MonitoringData = {
  summary: MonitoringSummary | null;
  runAnalytics: MonitoringRunAnalytics | null;
  pipelinePerformance: MonitoringPerformanceResponse | null;
  stepPerformance: MonitoringPerformanceResponse | null;
  taskPerformance: MonitoringPerformanceResponse | null;
  triggerAnalytics: MonitoringTriggerAnalytics | null;
  externalTriggerAnalytics: MonitoringExternalTriggerAnalytics | null;
  runnerHistory: MonitoringRunnerHistory | null;
  aiUsage: MonitoringAIUsage | null;
  reliability: MonitoringReliability | null;
  efficiency: MonitoringEfficiency | null;
  security: MonitoringSecurity | null;
};

type SavedMonitoringView = {
  selectedGroupId?: number | 'all';
  windowDays?: number;
  pipelineFilter?: string;
  repoFilter?: string;
  triggerSourceFilter?: string;
  statusFilter?: string;
  comparePrevious?: boolean;
  activeTab?: MonitoringTab;
};

type MonitoringAlertDraft = {
  name: string;
  metric: string;
  comparator: string;
  threshold: string;
  windowSeconds: string;
  severity: string;
};

const emptyMonitoringData: MonitoringData = {
  summary: null,
  runAnalytics: null,
  pipelinePerformance: null,
  stepPerformance: null,
  taskPerformance: null,
  triggerAnalytics: null,
  externalTriggerAnalytics: null,
  runnerHistory: null,
  aiUsage: null,
  reliability: null,
  efficiency: null,
  security: null,
};

function MonitoringPage() {
  const savedView = useMemo(readSavedMonitoringView, []);
  const linkedRunId = useMemo(readInitialMonitoringRunID, []);
  const [groups, setGroups] = useState<Group[]>([]);
  const [services, setServices] = useState<ServiceStatus[]>([]);
  const [runners, setRunners] = useState<MonitoringRunner[]>([]);
  const [runnerSummary, setRunnerSummary] = useState<RunnerSummary>(emptyRunnerSummary);
  const [runtimeUnavailable, setRuntimeUnavailable] = useState<string | null>(null);
  const [data, setData] = useState<MonitoringData>(emptyMonitoringData);
  const [previousData, setPreviousData] = useState<MonitoringData | null>(null);
  const [savedViews, setSavedViews] = useState<MonitoringSavedView[]>([]);
  const [alertRules, setAlertRules] = useState<MonitoringAlertRule[]>([]);
  const [alertEvents, setAlertEvents] = useState<MonitoringAlertEvent[]>([]);
  const [recommendations, setRecommendations] = useState<MonitoringRecommendation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [workflowError, setWorkflowError] = useState<string | null>(null);
  const [workflowBusy, setWorkflowBusy] = useState(false);
  const [selectedGroupId, setSelectedGroupId] = useState<number | 'all'>(savedView.selectedGroupId ?? 'all');
  const [windowDays, setWindowDays] = useState(linkedRunId ? 0 : savedView.windowDays ?? 30);
  const [runIdFilter] = useState(linkedRunId);
  const [pipelineFilter, setPipelineFilter] = useState(savedView.pipelineFilter ?? '');
  const [repoFilter, setRepoFilter] = useState(savedView.repoFilter ?? '');
  const [triggerSourceFilter, setTriggerSourceFilter] = useState(savedView.triggerSourceFilter ?? '');
  const [statusFilter, setStatusFilter] = useState(savedView.statusFilter ?? 'all');
  const [comparePrevious, setComparePrevious] = useState(Boolean(savedView.comparePrevious));
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [activeTab, setActiveTab] = useState<MonitoringTab>(() => readInitialMonitoringTab(savedView.activeTab));
  const [refreshedAt, setRefreshedAt] = useState<string>('');
  const [savedAt, setSavedAt] = useState<string>('');
  const [serverViewName, setServerViewName] = useState(savedView.activeTab ? `${savedView.activeTab} view` : 'Monitoring view');
  const [alertDraft, setAlertDraft] = useState<MonitoringAlertDraft>({
    name: 'Failure rate regression',
    metric: 'failure_rate',
    comparator: 'gt',
    threshold: '0.1',
    windowSeconds: '3600',
    severity: 'warning',
  });

  const fetchJson = useCallback(async <T,>(path: string): Promise<T> => {
    return requestMonitoringJson<T>(path);
  }, []);

  const sendJson = useCallback(async <T,>(path: string, method: string, body?: unknown): Promise<T> => {
    return sendMonitoringJson<T>(path, method, body);
  }, []);

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
      const runners = runnersRaw.map(normalizeMonitoringRunner).filter(runner => runner.label || runner.runnerId);
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

  const fetchMonitoringAnalyticsSet = useCallback(async (query: string): Promise<MonitoringData> => {
    const [
      summary,
      runAnalytics,
      pipelinePerformance,
      stepPerformance,
      taskPerformance,
      triggerAnalytics,
      externalTriggerAnalytics,
      runnerHistory,
      aiUsage,
      reliability,
      efficiency,
      security,
    ] = await Promise.all([
      fetchJson<MonitoringSummary>(`/v1/monitoring/summary?${query}`),
      fetchJson<MonitoringRunAnalytics>(`/v1/monitoring/runs/analytics?${query}`),
      fetchJson<MonitoringPerformanceResponse>(`/v1/monitoring/pipelines/performance?${query}`),
      fetchJson<MonitoringPerformanceResponse>(`/v1/monitoring/steps/performance?${query}`),
      fetchJson<MonitoringPerformanceResponse>(`/v1/monitoring/tasks/performance?${query}`),
      fetchJson<MonitoringTriggerAnalytics>(`/v1/monitoring/triggers/analytics?${query}`),
      fetchJson<MonitoringExternalTriggerAnalytics>(`/v1/monitoring/external-triggers/analytics?${query}`),
      fetchJson<MonitoringRunnerHistory>(`/v1/monitoring/runners/history?${query}`),
      fetchJson<MonitoringAIUsage>(`/v1/monitoring/ai-usage?${query}`),
      fetchJson<MonitoringReliability>(`/v1/monitoring/reliability?${query}`),
      fetchJson<MonitoringEfficiency>(`/v1/monitoring/efficiency?${query}`),
      fetchJson<MonitoringSecurity>(`/v1/monitoring/security?${query}`),
    ]);
    return {
      summary,
      runAnalytics,
      pipelinePerformance,
      stepPerformance,
      taskPerformance,
      triggerAnalytics,
      externalTriggerAnalytics,
      runnerHistory,
      aiUsage,
      reliability,
      efficiency,
      security,
    };
  }, [fetchJson]);

  const fetchMonitoringWorkflows = useCallback(async () => {
    const [views, rules, events, recommendationRows] = await Promise.all([
      fetchJson<MonitoringSavedView[]>('/v1/monitoring/views'),
      fetchJson<MonitoringAlertRule[]>('/v1/monitoring/alert-rules'),
      fetchJson<MonitoringAlertEvent[]>('/v1/monitoring/alert-events'),
      fetchJson<MonitoringRecommendation[]>('/v1/monitoring/recommendations?status=open'),
    ]);
    setSavedViews(Array.isArray(views) ? views : []);
    setAlertRules(Array.isArray(rules) ? rules : []);
    setAlertEvents(Array.isArray(events) ? events : []);
    setRecommendations(Array.isArray(recommendationRows) ? recommendationRows : []);
    setWorkflowError(null);
  }, [fetchJson]);

  const monitoringQuery = useMemo(
    () => buildMonitoringQuery({ selectedGroupId, windowDays, pipelineFilter, repoFilter, triggerSourceFilter, statusFilter, comparePrevious, runIdFilter }),
    [comparePrevious, pipelineFilter, repoFilter, runIdFilter, selectedGroupId, statusFilter, triggerSourceFilter, windowDays]
  );

  const previousMonitoringQuery = useMemo(
    () => (comparePrevious && windowDays > 0 && !runIdFilter
      ? buildMonitoringQuery({ selectedGroupId, windowDays, pipelineFilter, repoFilter, triggerSourceFilter, statusFilter, comparePrevious: false, runIdFilter }, 1)
      : ''),
    [comparePrevious, pipelineFilter, repoFilter, runIdFilter, selectedGroupId, statusFilter, triggerSourceFilter, windowDays]
  );

  const loadMonitoringData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [groupPayload, currentData, previousWindowData, runtime] = await Promise.all([
        fetchJson<Group[]>('/v1/groups'),
        fetchMonitoringAnalyticsSet(monitoringQuery),
        previousMonitoringQuery ? fetchMonitoringAnalyticsSet(previousMonitoringQuery) : Promise.resolve(null),
        fetchMonitoringRuntime(),
      ]);

      const accessibleGroups = Array.isArray(groupPayload) ? groupPayload : [];
      setGroups(accessibleGroups);
      setData(currentData);
      setPreviousData(previousWindowData);
      setServices(runtime.services);
      setRunners(runtime.runners);
      setRunnerSummary(currentData.summary?.runner_summary || runtime.runnerSummary);
      setRuntimeUnavailable(currentData.summary?.dispatcher_error || runtime.unavailable);
      setSelectedGroupId(current => (current !== 'all' && !accessibleGroups.some(group => group.id === current) ? 'all' : current));
      setRefreshedAt(new Date().toISOString());
      void fetchMonitoringWorkflows().catch(err => {
        setWorkflowError(err instanceof Error ? err.message : 'Failed to load monitoring workflows');
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load monitoring data');
    } finally {
      setLoading(false);
    }
  }, [fetchJson, fetchMonitoringAnalyticsSet, fetchMonitoringRuntime, fetchMonitoringWorkflows, monitoringQuery, previousMonitoringQuery]);

  useEffect(() => {
    void loadMonitoringData();
  }, [loadMonitoringData]);

  useEffect(() => {
    if (!autoRefresh) return undefined;
    const timer = window.setInterval(() => void loadMonitoringData(), 30000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, loadMonitoringData]);

  useEffect(() => {
    syncMonitoringTabToURL(activeTab);
  }, [activeTab]);

  const groupContext = useMemo(() => buildGroupContext(groups), [groups]);
  const selectedGroupLabel = selectedGroupId === 'all' ? 'All accessible groups' : groupContext.labels.get(selectedGroupId) || 'Selected group';

  const saveView = () => {
    writeSavedMonitoringView({ selectedGroupId, windowDays, pipelineFilter, repoFilter, triggerSourceFilter, statusFilter, comparePrevious, activeTab });
    setSavedAt(new Date().toISOString());
  };

  const exportCSV = () => {
    const csv = csvForMonitoringTab(activeTab, data);
    downloadText(`nopsai-monitoring-${activeTab}.csv`, csv);
  };

  const activateShortcut = (shortcut: 'failures' | 'queue' | 'ai') => {
    if (shortcut === 'failures') {
      setStatusFilter('failure');
      setActiveTab('reliability');
    } else if (shortcut === 'queue') {
      setActiveTab('efficiency');
    } else {
      setActiveTab('ai-usage');
    }
  };

  const currentSavedViewFilters = () => ({
    selectedGroupId,
    windowDays,
    pipelineFilter,
    repoFilter,
    triggerSourceFilter,
    statusFilter,
    comparePrevious,
    activeTab,
  });

  const currentAlertFilters = () => {
    const filters: Record<string, unknown> = {};
    if (selectedGroupId !== 'all') filters.groupId = selectedGroupId;
    const pipeline = pipelineFilter.trim();
    if (pipeline) {
      const parts = pipeline.split('/').filter(Boolean);
      if (parts.length > 1) {
        filters.pipelinePath = parts.slice(0, -1).join('/');
        filters.pipelineName = parts[parts.length - 1];
      } else {
        filters.pipelineName = pipeline;
      }
    }
    if (repoFilter.trim()) filters.repo = repoFilter.trim();
    if (triggerSourceFilter.trim()) filters.triggerSource = triggerSourceFilter.trim();
    if (statusFilter !== 'all') filters.status = statusFilter;
    return filters;
  };

  const saveServerView = async () => {
    const name = serverViewName.trim();
    if (!name) {
      setWorkflowError('View name is required.');
      return;
    }
    setWorkflowBusy(true);
    setWorkflowError(null);
    try {
      const view = await sendJson<MonitoringSavedView>('/v1/monitoring/views', 'POST', {
        name,
        visibility: 'private',
        filters: currentSavedViewFilters(),
        columns: [],
      });
      setSavedViews(current => [view, ...current.filter(item => item.id !== view.id)]);
      setSavedAt(new Date().toISOString());
    } catch (err) {
      setWorkflowError(err instanceof Error ? err.message : 'Failed to save monitoring view');
    } finally {
      setWorkflowBusy(false);
    }
  };

  const applyServerView = (view: MonitoringSavedView) => {
    const filters = view.filters || {};
    const groupValue = filters.selectedGroupId;
    const savedWindowDays = Number(filters.windowDays);
    setSelectedGroupId(typeof groupValue === 'number' ? groupValue : 'all');
    setWindowDays(Number.isFinite(savedWindowDays) ? savedWindowDays : 30);
    setPipelineFilter(String(filters.pipelineFilter || ''));
    setRepoFilter(String(filters.repoFilter || ''));
    setTriggerSourceFilter(String(filters.triggerSourceFilter || ''));
    setStatusFilter(String(filters.statusFilter || 'all'));
    setComparePrevious(Boolean(filters.comparePrevious));
    if (isMonitoringTab(String(filters.activeTab))) setActiveTab(String(filters.activeTab) as MonitoringTab);
  };

  const deleteServerView = async (view: MonitoringSavedView) => {
    setWorkflowBusy(true);
    setWorkflowError(null);
    try {
      await sendJson<void>(`/v1/monitoring/views/${encodeURIComponent(view.id)}`, 'DELETE');
      setSavedViews(current => current.filter(item => item.id !== view.id));
    } catch (err) {
      setWorkflowError(err instanceof Error ? err.message : 'Failed to delete monitoring view');
    } finally {
      setWorkflowBusy(false);
    }
  };

  const createAlertRule = async () => {
    const threshold = Number(alertDraft.threshold);
    const windowSeconds = Number(alertDraft.windowSeconds);
    if (!alertDraft.name.trim() || !Number.isFinite(threshold) || !Number.isFinite(windowSeconds)) {
      setWorkflowError('Alert rule name, threshold, and window are required.');
      return;
    }
    setWorkflowBusy(true);
    setWorkflowError(null);
    try {
      const rule = await sendJson<MonitoringAlertRule>('/v1/monitoring/alert-rules', 'POST', {
        name: alertDraft.name.trim(),
        enabled: true,
        visibility: 'workspace',
        severity: alertDraft.severity,
        metric: alertDraft.metric,
        comparator: alertDraft.comparator,
        threshold,
        window_seconds: windowSeconds,
        filters: currentAlertFilters(),
      });
      setAlertRules(current => [rule, ...current.filter(item => item.id !== rule.id)]);
    } catch (err) {
      setWorkflowError(err instanceof Error ? err.message : 'Failed to create alert rule');
    } finally {
      setWorkflowBusy(false);
    }
  };

  const evaluateAlertRule = async (rule: MonitoringAlertRule) => {
    setWorkflowBusy(true);
    setWorkflowError(null);
    try {
      const event = await sendJson<MonitoringAlertEvent>(`/v1/monitoring/alert-rules/${encodeURIComponent(rule.id)}/evaluate`, 'POST');
      setAlertEvents(current => [event, ...current.filter(item => item.id !== event.id)]);
      await fetchMonitoringWorkflows();
    } catch (err) {
      setWorkflowError(err instanceof Error ? err.message : 'Failed to evaluate alert rule');
    } finally {
      setWorkflowBusy(false);
    }
  };

  const deleteAlertRule = async (rule: MonitoringAlertRule) => {
    setWorkflowBusy(true);
    setWorkflowError(null);
    try {
      await sendJson<void>(`/v1/monitoring/alert-rules/${encodeURIComponent(rule.id)}`, 'DELETE');
      setAlertRules(current => current.filter(item => item.id !== rule.id));
    } catch (err) {
      setWorkflowError(err instanceof Error ? err.message : 'Failed to delete alert rule');
    } finally {
      setWorkflowBusy(false);
    }
  };

  const resolveRecommendation = async (recommendation: MonitoringRecommendation) => {
    setWorkflowBusy(true);
    setWorkflowError(null);
    try {
      await sendJson<MonitoringRecommendation>(`/v1/monitoring/recommendations/${encodeURIComponent(recommendation.id)}/resolve`, 'POST');
      setRecommendations(current => current.filter(item => item.id !== recommendation.id));
    } catch (err) {
      setWorkflowError(err instanceof Error ? err.message : 'Failed to resolve recommendation');
    } finally {
      setWorkflowBusy(false);
    }
  };

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto w-full max-w-[1500px] space-y-6 px-4 py-5 sm:px-6 lg:px-8">
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
              {savedAt ? ` - view saved ${formatShortDateTime(savedAt)}` : ''}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" className="inline-flex h-10 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-primary)] shadow-sm hover:bg-[var(--bg-tertiary)]" onClick={() => activateShortcut('failures')}>
              <Bell className="h-4 w-4" />
              Failures
            </button>
            <button type="button" className="inline-flex h-10 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-primary)] shadow-sm hover:bg-[var(--bg-tertiary)]" onClick={() => activateShortcut('queue')}>
              <Bell className="h-4 w-4" />
              Queue
            </button>
            <button type="button" className="inline-flex h-10 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-primary)] shadow-sm hover:bg-[var(--bg-tertiary)]" onClick={() => activateShortcut('ai')}>
              <Bell className="h-4 w-4" />
              LLM tokens
            </button>
          </div>
        </div>

        <section className="grid gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-2 shadow-sm md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8">
          <select value={String(selectedGroupId)} onChange={event => setSelectedGroupId(event.target.value === 'all' ? 'all' : Number(event.target.value))} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]">
            <option value="all">All accessible groups</option>
            {groups.map(group => (
              <option key={group.id} value={group.id}>{groupContext.labels.get(group.id) || group.name}</option>
            ))}
          </select>
          <div className="inline-flex h-9 overflow-hidden rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] xl:col-span-2">
            {WINDOW_OPTIONS.map(option => (
              <button key={option.label} type="button" onClick={() => setWindowDays(option.days)} className={`flex-1 px-2 text-sm font-medium transition-colors ${windowDays === option.days ? 'bg-[var(--bg-active)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}`}>
                {option.label}
              </button>
            ))}
          </div>
          <input value={pipelineFilter} onChange={event => setPipelineFilter(event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]" placeholder="Pipeline" />
          <input value={repoFilter} onChange={event => setRepoFilter(event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]" placeholder="Repo" />
          <input value={triggerSourceFilter} onChange={event => setTriggerSourceFilter(event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]" placeholder="Trigger source" />
          <select value={statusFilter} onChange={event => setStatusFilter(event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]">
            {STATUS_OPTIONS.map(status => (
              <option key={status} value={status}>{status === 'all' ? 'All statuses' : status}</option>
            ))}
          </select>
          <label className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]">
            <input type="checkbox" checked={comparePrevious} onChange={event => setComparePrevious(event.target.checked)} />
            Compare
          </label>
          <label className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2.5 text-sm text-[var(--text-primary)]">
            <input type="checkbox" checked={autoRefresh} onChange={event => setAutoRefresh(event.target.checked)} />
            Auto refresh
          </label>
          <div className="inline-flex h-9 items-center justify-end gap-1 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-1.5 xl:col-span-2">
            <button type="button" onClick={exportCSV} title="Export CSV" aria-label="Export CSV" className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]">
              <Download className="h-4 w-4" />
            </button>
            <button type="button" onClick={saveView} title="Save local view" aria-label="Save local view" className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]">
              <Save className="h-4 w-4" />
            </button>
            <button type="button" onClick={() => void loadMonitoringData()} disabled={loading} title="Refresh" aria-label="Refresh" className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] disabled:cursor-not-allowed disabled:opacity-60">
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            </button>
          </div>
        </section>

        {error ? <div className="rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">{error}</div> : null}
        {workflowError ? <div className="rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-100">{workflowError}</div> : null}

        <MonitoringWorkflowPanel
          savedViews={savedViews}
          viewName={serverViewName}
          onViewNameChange={setServerViewName}
          onSaveView={saveServerView}
          onApplyView={applyServerView}
          onDeleteView={deleteServerView}
          alertDraft={alertDraft}
          onAlertDraftChange={(field, value) => setAlertDraft(current => ({ ...current, [field]: value }))}
          alertRules={alertRules}
          alertEvents={alertEvents}
          recommendations={recommendations}
          onCreateAlertRule={createAlertRule}
          onEvaluateAlertRule={evaluateAlertRule}
          onDeleteAlertRule={deleteAlertRule}
          onResolveRecommendation={resolveRecommendation}
          busy={workflowBusy}
        />

        <MonitoringDashboard
          activeTab={activeTab}
          onTabChange={setActiveTab}
          loading={loading}
          summary={data.summary}
          runAnalytics={data.runAnalytics}
          pipelinePerformance={data.pipelinePerformance}
          stepPerformance={data.stepPerformance}
          taskPerformance={data.taskPerformance}
          triggerAnalytics={data.triggerAnalytics}
          externalTriggerAnalytics={data.externalTriggerAnalytics}
          runnerHistory={data.runnerHistory}
          aiUsage={data.aiUsage}
          reliability={data.reliability}
          efficiency={data.efficiency}
          security={data.security}
          previousSummary={previousData?.summary || null}
          previousRunAnalytics={previousData?.runAnalytics || null}
          previousPipelinePerformance={previousData?.pipelinePerformance || null}
          previousStepPerformance={previousData?.stepPerformance || null}
          previousTaskPerformance={previousData?.taskPerformance || null}
          previousTriggerAnalytics={previousData?.triggerAnalytics || null}
          previousExternalTriggerAnalytics={previousData?.externalTriggerAnalytics || null}
          previousAIUsage={previousData?.aiUsage || null}
          previousReliability={previousData?.reliability || null}
          previousEfficiency={previousData?.efficiency || null}
          previousSecurity={previousData?.security || null}
          services={services}
          runners={runners}
          runnerSummary={runnerSummary}
          runtimeUnavailable={runtimeUnavailable}
        />
      </div>
    </div>
  );
}

function MonitoringWorkflowPanel({
  savedViews,
  viewName,
  onViewNameChange,
  onSaveView,
  onApplyView,
  onDeleteView,
  alertDraft,
  onAlertDraftChange,
  alertRules,
  alertEvents,
  recommendations,
  onCreateAlertRule,
  onEvaluateAlertRule,
  onDeleteAlertRule,
  onResolveRecommendation,
  busy,
}: {
  savedViews: MonitoringSavedView[];
  viewName: string;
  onViewNameChange: (value: string) => void;
  onSaveView: () => void;
  onApplyView: (view: MonitoringSavedView) => void;
  onDeleteView: (view: MonitoringSavedView) => void;
  alertDraft: MonitoringAlertDraft;
  onAlertDraftChange: (field: keyof MonitoringAlertDraft, value: string) => void;
  alertRules: MonitoringAlertRule[];
  alertEvents: MonitoringAlertEvent[];
  recommendations: MonitoringRecommendation[];
  onCreateAlertRule: () => void;
  onEvaluateAlertRule: (rule: MonitoringAlertRule) => void;
  onDeleteAlertRule: (rule: MonitoringAlertRule) => void;
  onResolveRecommendation: (recommendation: MonitoringRecommendation) => void;
  busy: boolean;
}) {
  return (
    <details className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm">
      <summary className="flex min-h-10 cursor-pointer list-none items-center justify-between gap-3 px-3 py-2 text-sm font-semibold text-[var(--text-primary)] [&::-webkit-details-marker]:hidden">
        <span className="inline-flex min-w-0 items-center gap-2">
          <Bell className="h-4 w-4 text-[var(--text-accent)]" />
          <span className="truncate">Monitoring operations</span>
        </span>
        <span className="flex shrink-0 flex-wrap justify-end gap-1.5 text-[11px] font-medium text-[var(--text-secondary)]">
          <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5">{savedViews.length} views</span>
          <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5">{alertRules.length} rules</span>
          <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5">{alertEvents.length} events</span>
          <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5">{recommendations.length} recs</span>
          {busy ? <span className="rounded-md border border-blue-500/30 bg-blue-500/10 px-2 py-0.5 text-blue-600 dark:text-blue-300">busy</span> : null}
        </span>
      </summary>
      <div className="grid gap-4 border-t border-[var(--border-primary)] p-3 xl:grid-cols-3">
      <div className="min-w-0 space-y-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]">
          <Save className="h-4 w-4 text-[var(--text-accent)]" />
          Saved views
        </div>
        <div className="flex gap-2">
          <input value={viewName} onChange={event => onViewNameChange(event.target.value)} className="h-9 min-w-0 flex-1 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 text-sm text-[var(--text-primary)]" />
          <button type="button" onClick={onSaveView} disabled={busy} aria-label="Save monitoring view" className="inline-flex h-9 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 text-sm font-semibold text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] disabled:opacity-60">
            <Save className="h-4 w-4" />
          </button>
        </div>
        <div className="max-h-44 divide-y divide-[var(--border-primary)] overflow-y-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)]">
          {savedViews.length ? savedViews.slice(0, 8).map(view => (
            <div key={view.id} className="flex items-center justify-between gap-2 px-3 py-2">
              <button type="button" onClick={() => onApplyView(view)} className="min-w-0 flex-1 text-left text-sm font-medium text-[var(--text-primary)] hover:text-[var(--text-link)]">
                <span className="block truncate">{view.name}</span>
              </button>
              <span className="shrink-0 text-xs text-[var(--text-secondary)]">{view.visibility || 'private'}</span>
              {!view.managed_by_config_repo ? (
                <button type="button" onClick={() => onDeleteView(view)} disabled={busy} aria-label={`Delete ${view.name}`} className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-red-500 disabled:opacity-60">
                  <Trash2 className="h-4 w-4" />
                </button>
              ) : null}
            </div>
          )) : <WorkflowEmpty label="No saved views" />}
        </div>
      </div>

      <div className="min-w-0 space-y-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]">
          <AlertTriangle className="h-4 w-4 text-[var(--text-accent)]" />
          Alert rules
        </div>
        <div className="grid grid-cols-2 gap-2">
          <input value={alertDraft.name} onChange={event => onAlertDraftChange('name', event.target.value)} className="col-span-2 h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 text-sm text-[var(--text-primary)]" />
          <select value={alertDraft.metric} onChange={event => onAlertDraftChange('metric', event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2 text-sm text-[var(--text-primary)]">
            <option value="failure_rate">Failure rate</option>
            <option value="p95_duration_seconds">p95 duration</option>
            <option value="queued_jobs">Queued jobs</option>
            <option value="runner_utilization">Runner utilization</option>
            <option value="ai_tokens">LLM tokens</option>
            <option value="external_trigger_failures">External failures</option>
          </select>
          <select value={alertDraft.comparator} onChange={event => onAlertDraftChange('comparator', event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2 text-sm text-[var(--text-primary)]">
            <option value="gt">&gt;</option>
            <option value="gte">&gt;=</option>
            <option value="lt">&lt;</option>
            <option value="lte">&lt;=</option>
            <option value="eq">=</option>
          </select>
          <input value={alertDraft.threshold} onChange={event => onAlertDraftChange('threshold', event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 text-sm text-[var(--text-primary)]" />
          <select value={alertDraft.severity} onChange={event => onAlertDraftChange('severity', event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2 text-sm text-[var(--text-primary)]">
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="critical">Critical</option>
          </select>
          <select value={alertDraft.windowSeconds} onChange={event => onAlertDraftChange('windowSeconds', event.target.value)} className="h-9 min-w-0 rounded-md border border-[var(--border-input)] bg-[var(--bg-primary)] px-2 text-sm text-[var(--text-primary)]">
            <option value="3600">1 hour</option>
            <option value="21600">6 hours</option>
            <option value="86400">24 hours</option>
            <option value="604800">7 days</option>
          </select>
          <button type="button" onClick={onCreateAlertRule} disabled={busy} className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 text-sm font-semibold text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] disabled:opacity-60">
            <AlertTriangle className="h-4 w-4" />
            Create
          </button>
        </div>
        <div className="max-h-44 divide-y divide-[var(--border-primary)] overflow-y-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)]">
          {alertRules.length ? alertRules.slice(0, 6).map(rule => (
            <div key={rule.id} className="flex items-center gap-2 px-3 py-2">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-[var(--text-primary)]">{rule.name}</p>
                <p className="truncate text-xs text-[var(--text-secondary)]">{rule.metric} {rule.comparator} {rule.threshold}</p>
              </div>
              <span className={`shrink-0 rounded-md border px-2 py-1 text-xs font-semibold ${rule.last_event?.status === 'firing' ? 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300' : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'}`}>
                {rule.last_event?.status || 'new'}
              </span>
              <button type="button" onClick={() => onEvaluateAlertRule(rule)} disabled={busy} aria-label={`Evaluate ${rule.name}`} className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] disabled:opacity-60">
                <Play className="h-4 w-4" />
              </button>
              {!rule.managed_by_config_repo ? (
                <button type="button" onClick={() => onDeleteAlertRule(rule)} disabled={busy} aria-label={`Delete ${rule.name}`} className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-red-500 disabled:opacity-60">
                  <Trash2 className="h-4 w-4" />
                </button>
              ) : null}
            </div>
          )) : <WorkflowEmpty label="No alert rules" />}
        </div>
      </div>

      <div className="min-w-0 space-y-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]">
          <CheckCircle2 className="h-4 w-4 text-[var(--text-accent)]" />
          Recommendations
        </div>
        <div className="max-h-44 divide-y divide-[var(--border-primary)] overflow-y-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)]">
          {recommendations.length ? recommendations.slice(0, 8).map(recommendation => (
            <div key={recommendation.id} className="flex items-start gap-2 px-3 py-2">
              <div className="min-w-0 flex-1">
                <p className="line-clamp-2 text-sm text-[var(--text-primary)]">{recommendation.message}</p>
                <p className="mt-1 text-xs text-[var(--text-secondary)]">{recommendation.category || 'monitoring'} - {recommendation.last_seen_at ? formatShortDateTime(recommendation.last_seen_at) : 'new'}</p>
              </div>
              <button type="button" onClick={() => onResolveRecommendation(recommendation)} disabled={busy} aria-label="Resolve recommendation" className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-emerald-500 disabled:opacity-60">
                <CheckCircle2 className="h-4 w-4" />
              </button>
            </div>
          )) : <WorkflowEmpty label="No recommendations" />}
        </div>
        <div className="max-h-28 divide-y divide-[var(--border-primary)] overflow-y-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)]">
          {alertEvents.length ? alertEvents.slice(0, 4).map(event => (
            <div key={event.id} className="px-3 py-2">
              <p className="truncate text-sm text-[var(--text-primary)]">{event.message || event.status}</p>
              <p className="mt-1 text-xs text-[var(--text-secondary)]">{event.status} - {event.created_at ? formatShortDateTime(event.created_at) : ''}</p>
            </div>
          )) : <WorkflowEmpty label="No alert events" compact />}
        </div>
      </div>
      </div>
    </details>
  );
}

function WorkflowEmpty({ label, compact = false }: { label: string; compact?: boolean }) {
  return <div className={`flex items-center justify-center px-3 text-sm text-[var(--text-secondary)] ${compact ? 'h-20' : 'h-32'}`}>{label}</div>;
}

function buildMonitoringQuery(filters: {
  selectedGroupId: number | 'all';
  windowDays: number;
  pipelineFilter: string;
  repoFilter: string;
  triggerSourceFilter: string;
  statusFilter: string;
  comparePrevious: boolean;
  runIdFilter?: string;
}, periodOffset = 0) {
  const params = new URLSearchParams();
  const windowMs = filters.windowDays * 24 * 60 * 60 * 1000;
  const to = filters.windowDays > 0 ? new Date(Date.now() - periodOffset * windowMs) : new Date();
  if (filters.windowDays > 0) {
    const from = new Date(to.getTime() - windowMs);
    params.set('from', from.toISOString());
  } else {
    params.set('from', '1970-01-01T00:00:00.000Z');
  }
  params.set('to', to.toISOString());
  if (filters.selectedGroupId !== 'all') params.set('groupId', String(filters.selectedGroupId));
  const pipeline = filters.pipelineFilter.trim();
  if (pipeline) {
    const parts = pipeline.split('/').filter(Boolean);
    if (parts.length > 1) {
      params.set('pipelinePath', parts.slice(0, -1).join('/'));
      params.set('pipelineName', parts[parts.length - 1]);
    } else {
      params.set('pipelineName', pipeline);
    }
  }
  if (filters.repoFilter.trim()) params.set('repo', filters.repoFilter.trim());
  if (filters.runIdFilter?.trim()) params.set('runId', filters.runIdFilter.trim());
  if (filters.triggerSourceFilter.trim()) params.set('triggerSource', filters.triggerSourceFilter.trim());
  if (filters.statusFilter !== 'all') params.set('status', filters.statusFilter);
  if (filters.comparePrevious) params.set('compare', 'previous_period');
  return params.toString();
}

function readSavedMonitoringView(): SavedMonitoringView {
  if (typeof localStorage === 'undefined') return {};
  try {
    const raw = localStorage.getItem(VIEW_STORAGE_KEY);
    return raw ? (JSON.parse(raw) as SavedMonitoringView) : {};
  } catch {
    return {};
  }
}

function writeSavedMonitoringView(view: SavedMonitoringView) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(VIEW_STORAGE_KEY, JSON.stringify(view));
}

function readInitialMonitoringTab(savedTab?: MonitoringTab): MonitoringTab {
  const locationTab = readMonitoringTabFromLocation();
  return locationTab ?? savedTab ?? 'overview';
}

function readMonitoringTabFromLocation(): MonitoringTab | null {
  if (typeof window === 'undefined') return null;
  const tab = new URLSearchParams(window.location.search).get('tab');
  return isMonitoringTab(tab) ? tab : null;
}

function readInitialMonitoringRunID() {
  if (typeof window === 'undefined') return '';
  return (new URLSearchParams(window.location.search).get('runId') || '').trim();
}

function syncMonitoringTabToURL(tab: MonitoringTab) {
  if (typeof window === 'undefined') return;
  const url = new URL(window.location.href);
  if (url.searchParams.get('tab') === tab) return;
  url.searchParams.set('tab', tab);
  window.history.replaceState(window.history.state, '', `${url.pathname}?${url.searchParams.toString()}${url.hash}`);
}

function isMonitoringTab(value: string | null): value is MonitoringTab {
  return MONITORING_TABS.includes(value as MonitoringTab);
}

function csvForMonitoringTab(activeTab: MonitoringTab, data: MonitoringData): string {
  if (activeTab === 'pipelines') return rowsToCSV(['name', 'runs', 'success_rate', 'p95_seconds'], (data.pipelinePerformance?.items || []).map(item => [item.key, item.total_runs, item.success_rate, item.p95_duration_seconds]));
  if (activeTab === 'steps-tasks') return rowsToCSV(['name', 'runs', 'success_rate', 'p95_seconds'], [...(data.stepPerformance?.items || []), ...(data.taskPerformance?.items || [])].map(item => [item.key, item.total_runs, item.success_rate, item.p95_duration_seconds]));
  if (activeTab === 'ai-usage') return rowsToCSV(['label', 'events', 'tokens'], (data.aiUsage?.by_pipeline || []).map(item => [item.label, item.count, item.tokens]));
  if (activeTab === 'reliability') return rowsToCSV(['run_id', 'pipeline', 'status', 'reason'], (data.reliability?.recent_failures || []).map(run => [run.run_id, run.pipeline_name, run.status, run.failure_reason]));
  if (activeTab === 'efficiency') {
    const tokenRows = [
      ...(data.efficiency?.token_by_pipeline || []).map(item => ['pipeline', item.label, item.count, item.tokens]),
      ...(data.efficiency?.token_by_group || []).map(item => ['group', item.label, item.count, item.tokens]),
      ...(data.efficiency?.token_by_step || []).map(item => ['step', item.label, item.count, item.tokens]),
    ];
    return rowsToCSV(['dimension', 'label', 'count', 'tokens'], tokenRows);
  }
  if (activeTab === 'security') return rowsToCSV(['label', 'count'], (data.security?.runs_by_effective_subject || []).map(item => [item.label, item.count]));
  if (activeTab === 'external-triggers') return rowsToCSV(['label', 'count', 'failed'], (data.externalTriggerAnalytics?.most_fired_triggers || []).map(item => [item.label, item.count, item.failed]));
  if (activeTab === 'triggers') return rowsToCSV(['label', 'count', 'failed'], (data.triggerAnalytics?.trigger_sources || []).map(item => [item.label, item.count, item.failed]));
  return rowsToCSV(['run_id', 'pipeline', 'status', 'duration_seconds'], (data.runAnalytics?.recent_runs || []).map(run => [run.run_id, run.pipeline_name, run.status, run.duration_seconds]));
}

function rowsToCSV(headers: string[], rows: unknown[][]): string {
  return [headers, ...rows].map(row => row.map(csvCell).join(',')).join('\n');
}

function csvCell(value: unknown): string {
  const text = value == null ? '' : String(value);
  return `"${text.replace(/"/g, '""')}"`;
}

function downloadText(fileName: string, content: string) {
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  URL.revokeObjectURL(url);
}

export default MonitoringPage;
