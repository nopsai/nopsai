import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Bot,
  CheckCircle2,
  Clock3,
  Gauge,
  GitBranch,
  Layers,
  Server,
  ShieldCheck,
  Timer,
  Workflow,
  XCircle,
  Zap,
} from 'lucide-react';
import type {
  MonitoringAIUsage,
  MonitoringExternalTriggerAnalytics,
  MonitoringExternalTriggerLastFired,
  MonitoringEfficiency,
  MonitoringNamedCount,
  MonitoringPerformanceResponse,
  MonitoringPerformanceRow,
  MonitoringReliability,
  MonitoringRunAnalytics,
  MonitoringRunRow,
  MonitoringRunner,
  MonitoringRunnerHistory,
  MonitoringRunnerHistoryBucket,
  MonitoringSecurity,
  MonitoringSummary,
  MonitoringTab,
  MonitoringTimeBucket,
  MonitoringTriggerAnalytics,
  RunnerStatusValue,
  RunnerSummary,
  ServiceStatus,
  ServiceStatusValue,
} from './model';

const TABS: Array<{ id: MonitoringTab; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'runs', label: 'Runs' },
  { id: 'pipelines', label: 'Pipelines' },
  { id: 'steps-tasks', label: 'Steps & Tasks' },
  { id: 'triggers', label: 'Triggers' },
  { id: 'external-triggers', label: 'External Triggers' },
  { id: 'runners', label: 'Runners' },
  { id: 'ai-usage', label: 'LLM Usage' },
  { id: 'reliability', label: 'Reliability' },
  { id: 'efficiency', label: 'Efficiency' },
  { id: 'security', label: 'Security' },
];

const MAX_VISIBLE_RUNNER_RUNS = 3;

type MonitoringDashboardProps = {
  activeTab: MonitoringTab;
  onTabChange: (tab: MonitoringTab) => void;
  loading: boolean;
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
  previousSummary?: MonitoringSummary | null;
  previousRunAnalytics?: MonitoringRunAnalytics | null;
  previousPipelinePerformance?: MonitoringPerformanceResponse | null;
  previousStepPerformance?: MonitoringPerformanceResponse | null;
  previousTaskPerformance?: MonitoringPerformanceResponse | null;
  previousTriggerAnalytics?: MonitoringTriggerAnalytics | null;
  previousExternalTriggerAnalytics?: MonitoringExternalTriggerAnalytics | null;
  previousAIUsage?: MonitoringAIUsage | null;
  previousReliability?: MonitoringReliability | null;
  previousEfficiency?: MonitoringEfficiency | null;
  previousSecurity?: MonitoringSecurity | null;
  services: ServiceStatus[];
  runners: MonitoringRunner[];
  runnerSummary: RunnerSummary;
  runtimeUnavailable: string | null;
};

export function MonitoringDashboard({
  activeTab,
  onTabChange,
  loading,
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
  previousSummary,
  previousRunAnalytics,
  previousPipelinePerformance,
  previousStepPerformance,
  previousTaskPerformance,
  previousTriggerAnalytics,
  previousExternalTriggerAnalytics,
  previousAIUsage,
  previousReliability,
  previousEfficiency,
  previousSecurity,
  services,
  runners,
  runnerSummary,
  runtimeUnavailable,
}: MonitoringDashboardProps) {
  return (
    <div className="space-y-5">
      <div className="overflow-x-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-1">
        <div className="flex min-w-max gap-1">
          {TABS.map(tab => (
            <button
              key={tab.id}
              type="button"
              onClick={() => onTabChange(tab.id)}
              className={`h-9 rounded-md px-3 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'bg-[var(--bg-active)] text-[var(--text-primary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {activeTab === 'overview' ? (
        <OverviewTab summary={summary} previousSummary={previousSummary || null} runAnalytics={runAnalytics} loading={loading} />
      ) : activeTab === 'runs' ? (
        <RunsTab runAnalytics={runAnalytics} previousRunAnalytics={previousRunAnalytics || null} loading={loading} />
      ) : activeTab === 'pipelines' ? (
        <PipelinesTab performance={pipelinePerformance} previousPerformance={previousPipelinePerformance || null} loading={loading} />
      ) : activeTab === 'steps-tasks' ? (
        <StepsTasksTab steps={stepPerformance} tasks={taskPerformance} previousSteps={previousStepPerformance || null} previousTasks={previousTaskPerformance || null} loading={loading} />
      ) : activeTab === 'triggers' ? (
        <TriggersTab analytics={triggerAnalytics} previousAnalytics={previousTriggerAnalytics || null} loading={loading} />
      ) : activeTab === 'external-triggers' ? (
        <ExternalTriggersTab analytics={externalTriggerAnalytics} previousAnalytics={previousExternalTriggerAnalytics || null} loading={loading} />
      ) : activeTab === 'runners' ? (
        <RunnersTab services={services} runners={runners} summary={runnerSummary} history={runnerHistory} unavailable={runtimeUnavailable} loading={loading} />
      ) : activeTab === 'ai-usage' ? (
        <AIUsageTab usage={aiUsage} previousUsage={previousAIUsage || null} loading={loading} />
      ) : activeTab === 'reliability' ? (
        <ReliabilityTab reliability={reliability} previousReliability={previousReliability || null} loading={loading} />
      ) : activeTab === 'efficiency' ? (
        <EfficiencyTab efficiency={efficiency} previousEfficiency={previousEfficiency || null} loading={loading} />
      ) : (
        <SecurityTab security={security} previousSecurity={previousSecurity || null} loading={loading} />
      )}
    </div>
  );
}

function OverviewTab({ summary, previousSummary, runAnalytics, loading }: { summary: MonitoringSummary | null; previousSummary: MonitoringSummary | null; runAnalytics: MonitoringRunAnalytics | null; loading: boolean }) {
  const statusCounts = statusCountsFromSplit(runAnalytics?.status_split || []);
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Activity />} label="Pipeline runs" value={formatNumber(summary?.total_runs)} detail={`${formatNumber(summary?.running_runs)} running`} delta={deltaValue(summary?.total_runs, previousSummary?.total_runs)} tone="blue" loading={loading} />
        <MetricCard icon={<CheckCircle2 />} label="Success rate" value={formatPercent(summary?.success_rate)} detail={`${formatNumber(summary?.successful_runs)} successful`} delta={deltaValue(summary?.success_rate, previousSummary?.success_rate)} deltaFormat="percent" positiveIsGood tone="green" loading={loading} />
        <MetricCard icon={<XCircle />} label="Failure rate" value={formatPercent(summary?.failure_rate)} detail={`${formatNumber(summary?.failed_runs)} failed`} delta={deltaValue(summary?.failure_rate, previousSummary?.failure_rate)} deltaFormat="percent" tone="red" loading={loading} />
        <MetricCard icon={<Clock3 />} label="Average duration" value={formatDurationSeconds(summary?.average_duration_seconds)} detail={`p95 ${formatDurationSeconds(summary?.p95_duration_seconds)}`} delta={deltaValue(summary?.average_duration_seconds, previousSummary?.average_duration_seconds)} deltaFormat="duration" tone="amber" loading={loading} />
      </section>
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Timer />} label="Median duration" value={formatDurationSeconds(summary?.median_duration_seconds)} detail={`p99 ${formatDurationSeconds(summary?.p99_duration_seconds)}`} delta={deltaValue(summary?.median_duration_seconds, previousSummary?.median_duration_seconds)} deltaFormat="duration" tone="blue" loading={loading} />
        <MetricCard icon={<Gauge />} label="Runner utilization" value={formatPercent(summary?.runner_utilization)} detail={`${formatNumber(summary?.queued_jobs)} queued jobs`} delta={deltaValue(summary?.runner_utilization, previousSummary?.runner_utilization)} deltaFormat="percent" tone="green" loading={loading} />
        <MetricCard icon={<Workflow />} label="Steps executed" value={formatNumber(summary?.total_steps_executed)} detail={`${formatNumber(summary?.total_tasks_executed)} tasks`} delta={deltaValue(summary?.total_steps_executed, previousSummary?.total_steps_executed)} tone="amber" loading={loading} />
        <MetricCard icon={<Bot />} label="LLM tokens" value={formatNumber(summary?.estimated_ai_tokens)} detail="recorded usage" delta={deltaValue(summary?.estimated_ai_tokens, previousSummary?.estimated_ai_tokens)} tone="red" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.8fr)]">
        <Panel title="Run Trend" icon={<BarChart3 className="h-4 w-4" />}>
          <TimeBucketChart buckets={runAnalytics?.runs_over_time || []} loading={loading} />
        </Panel>
        <Panel title="Status Split" icon={<Activity className="h-4 w-4" />}>
          <StatusSplit counts={statusCounts} total={summary?.total_runs || 0} loading={loading} />
        </Panel>
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="Longest Run" icon={<Clock3 className="h-4 w-4" />}>
          {summary?.longest_run ? <RunRefBlock run={summary.longest_run} /> : <EmptyBlock label={loading ? 'Loading run' : 'No completed runs'} />}
        </Panel>
        <Panel title="Trigger Sources" icon={<Zap className="h-4 w-4" />}>
          <NamedCountList items={runAnalytics?.trigger_source_split || []} loading={loading} value="count" />
        </Panel>
        <Panel title="Failures" icon={<AlertTriangle className="h-4 w-4" />}>
          <NamedCountList items={runAnalytics?.failure_reasons || []} loading={loading} value="count" />
        </Panel>
      </section>
    </div>
  );
}

function RunsTab({ runAnalytics, previousRunAnalytics, loading }: { runAnalytics: MonitoringRunAnalytics | null; previousRunAnalytics: MonitoringRunAnalytics | null; loading: boolean }) {
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Clock3 />} label="Average" value={formatDurationSeconds(runAnalytics?.duration?.average_seconds)} detail={`median ${formatDurationSeconds(runAnalytics?.duration?.median_seconds)}`} delta={deltaValue(runAnalytics?.duration?.average_seconds, previousRunAnalytics?.duration?.average_seconds)} deltaFormat="duration" tone="blue" loading={loading} />
        <MetricCard icon={<Timer />} label="p95 duration" value={formatDurationSeconds(runAnalytics?.duration?.p95_seconds)} detail={`p99 ${formatDurationSeconds(runAnalytics?.duration?.p99_seconds)}`} delta={deltaValue(runAnalytics?.duration?.p95_seconds, previousRunAnalytics?.duration?.p95_seconds)} deltaFormat="duration" tone="amber" loading={loading} />
        <MetricCard icon={<Gauge />} label="Queue time" value={formatDurationSeconds(runAnalytics?.queue_time?.average_seconds)} detail={`max ${formatDurationSeconds(runAnalytics?.queue_time?.max_seconds)}`} delta={deltaValue(runAnalytics?.queue_time?.average_seconds, previousRunAnalytics?.queue_time?.average_seconds)} deltaFormat="duration" tone="green" loading={loading} />
        <MetricCard icon={<GitBranch />} label="Reruns" value={formatNumber(runAnalytics?.rerun_count)} detail={`${formatNumber(runAnalytics?.timeout_count)} timeouts`} delta={deltaValue(runAnalytics?.rerun_count, previousRunAnalytics?.rerun_count)} tone="red" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.8fr)]">
        <Panel title="Runs Over Time" icon={<BarChart3 className="h-4 w-4" />}>
          <TimeBucketChart buckets={runAnalytics?.runs_over_time || []} loading={loading} />
        </Panel>
        <Panel title="Run Heatmap" icon={<Activity className="h-4 w-4" />}>
          <Heatmap cells={runAnalytics?.run_heatmap || []} loading={loading} />
        </Panel>
      </section>
      <section className="grid gap-4 xl:grid-cols-2">
        <Panel title="Longest Runs" icon={<Clock3 className="h-4 w-4" />}>
          <RunTable rows={runAnalytics?.longest_runs || []} loading={loading} />
        </Panel>
        <Panel title="Recent Runs" icon={<Activity className="h-4 w-4" />}>
          <RunTable rows={runAnalytics?.recent_runs || []} loading={loading} />
        </Panel>
      </section>
    </div>
  );
}

function PipelinesTab({ performance, previousPerformance, loading }: { performance: MonitoringPerformanceResponse | null; previousPerformance: MonitoringPerformanceResponse | null; loading: boolean }) {
  return (
    <div className="space-y-4">
      <PerformanceSummaryCards rows={performance?.items || []} previousRows={previousPerformance?.items || []} loading={loading} noun="Pipelines" />
      <Panel title="Pipeline Performance" icon={<Workflow className="h-4 w-4" />}>
        <PerformanceTable rows={performance?.items || []} loading={loading} />
      </Panel>
    </div>
  );
}

function StepsTasksTab({ steps, tasks, previousSteps, previousTasks, loading }: { steps: MonitoringPerformanceResponse | null; tasks: MonitoringPerformanceResponse | null; previousSteps: MonitoringPerformanceResponse | null; previousTasks: MonitoringPerformanceResponse | null; loading: boolean }) {
  const taskRows = tasks?.items || [];
  const stepRows = (steps?.items || []).length ? steps?.items || [] : deriveStepRowsFromTaskRows(taskRows);
  const previousTaskRows = previousTasks?.items || [];
  const previousStepRows = (previousSteps?.items || []).length ? previousSteps?.items || [] : deriveStepRowsFromTaskRows(previousTaskRows);
  const rows = [...stepRows, ...taskRows];
  const previousRows = [...previousStepRows, ...previousTaskRows];
  return (
    <div className="space-y-4">
      <PerformanceSummaryCards rows={rows} previousRows={previousRows} loading={loading} noun="Steps/tasks" />
      <section className="grid gap-4 xl:grid-cols-2">
        <Panel title="Slowest Steps" icon={<Layers className="h-4 w-4" />}>
          <PerformanceTable rows={stepRows} loading={loading} compact />
        </Panel>
        <Panel title="Slowest Tasks" icon={<Workflow className="h-4 w-4" />}>
          <PerformanceTable rows={taskRows} loading={loading} compact />
        </Panel>
      </section>
    </div>
  );
}

function TriggersTab({ analytics, previousAnalytics, loading }: { analytics: MonitoringTriggerAnalytics | null; previousAnalytics: MonitoringTriggerAnalytics | null; loading: boolean }) {
  const currentInvocations = sumNamedCounts(analytics?.trigger_sources || [], 'count');
  const previousInvocations = sumNamedCounts(previousAnalytics?.trigger_sources || [], 'count');
  const currentFailures = sumNamedCounts(analytics?.failures_by_trigger_source || [], 'count');
  const previousFailures = sumNamedCounts(previousAnalytics?.failures_by_trigger_source || [], 'count');
  const currentTokens = sumNamedCounts(analytics?.token_by_trigger_source || [], 'tokens');
  const previousTokens = sumNamedCounts(previousAnalytics?.token_by_trigger_source || [], 'tokens');
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Zap />} label="Trigger runs" value={formatNumber(currentInvocations)} detail={`${formatNumber((analytics?.trigger_sources || []).length)} sources`} delta={deltaValue(currentInvocations, previousInvocations)} tone="blue" loading={loading} />
        <MetricCard icon={<XCircle />} label="Trigger failures" value={formatNumber(currentFailures)} detail="failed runs" delta={deltaValue(currentFailures, previousFailures)} tone="red" loading={loading} />
        <MetricCard icon={<Bot />} label="Trigger tokens" value={formatNumber(currentTokens)} detail="LLM token usage" delta={deltaValue(currentTokens, previousTokens)} tone="amber" loading={loading} />
        <MetricCard icon={<CheckCircle2 />} label="Reliability teams" value={formatNumber((analytics?.trigger_source_reliability || []).length)} detail="tracked sources" delta={deltaValue((analytics?.trigger_source_reliability || []).length, (previousAnalytics?.trigger_source_reliability || []).length)} positiveIsGood tone="green" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="Source Split" icon={<Zap className="h-4 w-4" />}>
          <NamedCountList items={analytics?.trigger_sources || []} loading={loading} value="count" />
        </Panel>
        <Panel title="Reliability" icon={<CheckCircle2 className="h-4 w-4" />}>
          <NamedCountList items={analytics?.trigger_source_reliability || []} loading={loading} value="rate" />
        </Panel>
        <Panel title="AI Tokens By Source" icon={<Bot className="h-4 w-4" />}>
          <NamedCountList items={analytics?.token_by_trigger_source || []} loading={loading} value="tokens" />
        </Panel>
      </section>
      <Panel title="Trigger Source Trend" icon={<BarChart3 className="h-4 w-4" />}>
        <TimeBucketChart buckets={analytics?.trigger_source_trend || []} loading={loading} />
      </Panel>
    </div>
  );
}

function ExternalTriggersTab({ analytics, previousAnalytics, loading }: { analytics: MonitoringExternalTriggerAnalytics | null; previousAnalytics: MonitoringExternalTriggerAnalytics | null; loading: boolean }) {
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Zap />} label="External triggers" value={formatNumber(analytics?.total_external_triggers)} detail={`${formatNumber(analytics?.enabled_external_triggers)} enabled`} delta={deltaValue(analytics?.total_external_triggers, previousAnalytics?.total_external_triggers)} tone="blue" loading={loading} />
        <MetricCard icon={<Activity />} label="Invocations" value={formatNumber(analytics?.invocation_count)} detail={`${formatNumber(analytics?.pending_invocations)} pending`} delta={deltaValue(analytics?.invocation_count, previousAnalytics?.invocation_count)} tone="green" loading={loading} />
        <MetricCard icon={<XCircle />} label="Failures" value={formatNumber(analytics?.failed_invocations)} detail={`${formatNumber(analytics?.idempotency_conflicts)} idempotency conflicts`} delta={deltaValue(analytics?.failed_invocations, previousAnalytics?.failed_invocations)} tone="red" loading={loading} />
        <MetricCard icon={<AlertTriangle />} label="Rate limits" value={formatNumber(analytics?.rate_limit_violations)} detail="limit violations" delta={deltaValue(analytics?.rate_limit_violations, previousAnalytics?.rate_limit_violations)} tone="amber" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-4">
        <Panel title="Most Fired" icon={<Zap className="h-4 w-4" />}>
          <NamedCountList items={analytics?.most_fired_triggers || []} loading={loading} value="count" linkForItem={item => externalTriggerHref(item.key)} />
        </Panel>
        <Panel title="Top Callers" icon={<ShieldCheck className="h-4 w-4" />}>
          <NamedCountList items={analytics?.top_callers || []} loading={loading} value="count" />
        </Panel>
        <Panel title="Error Reasons" icon={<AlertTriangle className="h-4 w-4" />}>
          <NamedCountList items={analytics?.error_reasons || []} loading={loading} value="count" />
        </Panel>
        <Panel title="Rate Limit Violations" icon={<AlertTriangle className="h-4 w-4" />}>
          <NamedCountList items={analytics?.rate_limit_violation_triggers || []} loading={loading} value="count" linkForItem={item => externalTriggerHref(item.key)} />
        </Panel>
      </section>
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.75fr)]">
        <Panel title="Last Fired" icon={<Clock3 className="h-4 w-4" />}>
          <ExternalTriggerLastFiredList items={analytics?.last_fired_triggers || []} loading={loading} />
        </Panel>
        <Panel title="Run Creation" icon={<CheckCircle2 className="h-4 w-4" />}>
          <MetricInline label="Accepted" value={formatNumber(analytics?.successful_invocations)} />
          <MetricInline label="Run rate" value={formatPercent(analytics?.invocation_to_run_rate)} />
          <MetricInline label="Failed" value={formatNumber(analytics?.failed_invocations)} />
        </Panel>
      </section>
    </div>
  );
}

function RunnersTab({ services, runners, summary, history, unavailable, loading }: { services: ServiceStatus[]; runners: MonitoringRunner[]; summary: RunnerSummary; history: MonitoringRunnerHistory | null; unavailable: string | null; loading: boolean }) {
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Server />} label="Online runners" value={formatNumber(summary.online)} detail={summary.unreachable ? `${formatNumber(summary.unreachable)} unreachable` : `${formatNumber(summary.total)} total`} positiveIsGood tone="green" loading={loading} />
        <MetricCard icon={<Gauge />} label="Capacity" value={formatNumber(summary.capacity)} detail={`${formatNumber(summary.activeJobs)} active`} positiveIsGood tone="blue" loading={loading} />
        <MetricCard icon={<Activity />} label="Inflight jobs" value={formatNumber(summary.inflightJobs)} detail={`${formatNumber(summary.queuedJobs)} queued`} tone="amber" loading={loading} />
        <MetricCard icon={<Layers />} label="Kubernetes" value={formatNumber(summary.kubernetes)} detail={`${formatNumber(summary.docker)} docker`} tone="red" loading={loading} />
      </section>
      <Panel title="Capacity Trend" icon={<BarChart3 className="h-4 w-4" />}>
        <RunnerHistoryChart buckets={history?.buckets || []} loading={loading} />
      </Panel>
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(400px,0.9fr)]">
        <Panel title="Services" icon={<Server className="h-4 w-4" />}>
          <ServiceStatusGrid services={services} unavailable={unavailable} loading={loading} />
        </Panel>
        <Panel title="Runners" icon={<Server className="h-4 w-4" />}>
          <RunnerStatusGrid runners={runners} summary={summary} unavailable={unavailable} loading={loading} />
        </Panel>
      </section>
    </div>
  );
}

function AIUsageTab({ usage, previousUsage, loading }: { usage: MonitoringAIUsage | null; previousUsage: MonitoringAIUsage | null; loading: boolean }) {
  const assistantChatTokens = safeNumber(usage?.assistant_chat_tokens);
  const assistantChatDetail = assistantChatTokens > 0
    ? ` · ${formatNumber(assistantChatTokens)} assistant chat`
    : '';
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Bot />} label="Total tokens" value={formatNumber(usage?.total_tokens)} detail={`${formatNumber(usage?.total_prompt_tokens)} prompt / ${formatNumber(usage?.total_completion_tokens)} completion${assistantChatDetail}`} delta={deltaValue(usage?.total_tokens, previousUsage?.total_tokens)} tone="blue" loading={loading} />
        <MetricCard icon={<Activity />} label="Exact tokens" value={formatNumber(usage?.exact_tokens)} detail={`${formatNumber(usage?.exact_token_events)} provider events`} delta={deltaValue(usage?.exact_tokens, previousUsage?.exact_tokens)} positiveIsGood tone="green" loading={loading} />
        <MetricCard icon={<Gauge />} label="Estimated tokens" value={formatNumber(usage?.estimated_tokens)} detail={`${formatNumber(usage?.estimated_token_events)} estimated events`} delta={deltaValue(usage?.estimated_tokens, previousUsage?.estimated_tokens)} tone="amber" loading={loading} />
        <MetricCard icon={<Clock3 />} label="Token trend" value={formatNumber((usage?.trend || []).reduce((sum, item) => sum + safeNumber(item.runs), 0))} detail={`${(usage?.trend || []).length} buckets`} delta={deltaValue((usage?.trend || []).reduce((sum, item) => sum + safeNumber(item.runs), 0), (previousUsage?.trend || []).reduce((sum, item) => sum + safeNumber(item.runs), 0))} tone="red" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="By Pipeline" icon={<Workflow className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_pipeline || []} loading={loading} value="tokens" linkForItem={pipelineNamedCountHref} />
        </Panel>
        <Panel title="By Step" icon={<Layers className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_step || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="By Task" icon={<Bot className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_task || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="By Feature" icon={<Bot className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_feature || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="By Provider" icon={<Bot className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_provider || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="By LLM Profile" icon={<ShieldCheck className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_profile || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="By Model" icon={<Gauge className="h-4 w-4" />}>
          <NamedCountList items={usage?.by_model || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="Top Token Runs" icon={<AlertTriangle className="h-4 w-4" />}>
          <NamedCountList items={usage?.top_token_runs || []} loading={loading} value="tokens" linkForItem={item => runHref(item.key)} />
        </Panel>
      </section>
    </div>
  );
}

function ReliabilityTab({ reliability, previousReliability, loading }: { reliability: MonitoringReliability | null; previousReliability: MonitoringReliability | null; loading: boolean }) {
  const notificationFailures = sumNamedCounts(reliability?.notification_failures || [], 'count');
  const previousNotificationFailures = sumNamedCounts(previousReliability?.notification_failures || [], 'count');
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<XCircle />} label="Recent failures" value={formatNumber(reliability?.recent_failures?.length)} detail="failed runs" delta={deltaValue(reliability?.recent_failures?.length, previousReliability?.recent_failures?.length)} tone="red" loading={loading} />
        <MetricCard icon={<Clock3 />} label="Stuck runs" value={formatNumber(reliability?.stuck_runs?.length)} detail="pending or running" delta={deltaValue(reliability?.stuck_runs?.length, previousReliability?.stuck_runs?.length)} tone="amber" loading={loading} />
        <MetricCard icon={<AlertTriangle />} label="Notifications" value={formatNumber(notificationFailures)} detail="failed deliveries" delta={deltaValue(notificationFailures, previousNotificationFailures)} tone="blue" loading={loading} />
        <MetricCard icon={<Activity />} label="Flaky pipelines" value={formatNumber(reliability?.flaky_pipelines?.length)} detail="mixed outcomes" delta={deltaValue(reliability?.flaky_pipelines?.length, previousReliability?.flaky_pipelines?.length)} tone="green" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="Failure Reasons" icon={<AlertTriangle className="h-4 w-4" />}>
          <NamedCountList items={reliability?.failure_reasons || []} loading={loading} value="count" />
        </Panel>
        <Panel title="Waiting Approvals" icon={<Clock3 className="h-4 w-4" />}>
          <NamedCountList items={reliability?.approvals_waiting_too_long || []} loading={loading} value="seconds" />
        </Panel>
        <Panel title="Notification Failures" icon={<XCircle className="h-4 w-4" />}>
          <NamedCountList items={reliability?.notification_failures || []} loading={loading} value="count" />
        </Panel>
      </section>
      <section className="grid gap-4 xl:grid-cols-2">
        <Panel title="Recent Failures" icon={<XCircle className="h-4 w-4" />}>
          <RunTable rows={reliability?.recent_failures || []} loading={loading} />
        </Panel>
        <Panel title="Stuck Runs" icon={<Clock3 className="h-4 w-4" />}>
          <RunTable rows={reliability?.stuck_runs || []} loading={loading} />
        </Panel>
        <Panel title="Repeated Failures" icon={<AlertTriangle className="h-4 w-4" />}>
          <PerformanceTable rows={reliability?.repeated_failure_pipelines || []} loading={loading} compact />
        </Panel>
        <Panel title="Flaky Pipelines" icon={<Activity className="h-4 w-4" />}>
          <PerformanceTable rows={reliability?.flaky_pipelines || []} loading={loading} compact />
        </Panel>
      </section>
    </div>
  );
}

function EfficiencyTab({ efficiency, previousEfficiency, loading }: { efficiency: MonitoringEfficiency | null; previousEfficiency: MonitoringEfficiency | null; loading: boolean }) {
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Clock3 />} label="Runtime" value={formatDurationSeconds(efficiency?.total_runtime_seconds)} detail={`${formatNumber(efficiency?.total_runner_minutes)} runner minutes`} delta={deltaValue(efficiency?.total_runtime_seconds, previousEfficiency?.total_runtime_seconds)} deltaFormat="duration" tone="blue" loading={loading} />
        <MetricCard icon={<Bot />} label="LLM tokens" value={formatNumber(efficiency?.total_ai_tokens)} detail="recorded usage" delta={deltaValue(efficiency?.total_ai_tokens, previousEfficiency?.total_ai_tokens)} tone="amber" loading={loading} />
        <MetricCard icon={<GitBranch />} label="Rerun teams" value={formatNumber(efficiency?.frequent_reruns?.length)} detail="pipelines with reruns" delta={deltaValue(efficiency?.frequent_reruns?.length, previousEfficiency?.frequent_reruns?.length)} tone="green" loading={loading} />
        <MetricCard icon={<Gauge />} label="High queue teams" value={formatNumber(efficiency?.high_queue_teams?.length)} detail="capacity pressure" delta={deltaValue(efficiency?.high_queue_teams?.length, previousEfficiency?.high_queue_teams?.length)} tone="red" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="Tokens By Pipeline" icon={<Workflow className="h-4 w-4" />}>
          <NamedCountList items={efficiency?.token_by_pipeline || []} loading={loading} value="tokens" linkForItem={pipelineNamedCountHref} />
        </Panel>
        <Panel title="Tokens By Team" icon={<Layers className="h-4 w-4" />}>
          <NamedCountList items={efficiency?.token_by_team || []} loading={loading} value="tokens" />
        </Panel>
        <Panel title="Tokens By Step" icon={<Bot className="h-4 w-4" />}>
          <NamedCountList items={efficiency?.token_by_step || []} loading={loading} value="tokens" />
        </Panel>
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="Queue Pressure" icon={<Gauge className="h-4 w-4" />}>
          <NamedCountList items={efficiency?.high_queue_teams || []} loading={loading} value="seconds" />
        </Panel>
        <Panel title="Token Heavy Low Success" icon={<AlertTriangle className="h-4 w-4" />}>
          <PerformanceTable rows={efficiency?.token_heavy_low_success_pipelines || []} loading={loading} compact />
        </Panel>
        <Panel title="Recommendations" icon={<CheckCircle2 className="h-4 w-4" />}>
          <RecommendationList items={efficiency?.recommendations || []} loading={loading} />
        </Panel>
      </section>
    </div>
  );
}

function SecurityTab({ security, previousSecurity, loading }: { security: MonitoringSecurity | null; previousSecurity: MonitoringSecurity | null; loading: boolean }) {
  const effectiveRuns = sumNamedCounts(security?.runs_by_effective_subject || [], 'count');
  const previousEffectiveRuns = sumNamedCounts(previousSecurity?.runs_by_effective_subject || [], 'count');
  const serviceAccountRuns = sumNamedCounts(security?.service_account_runs || [], 'count');
  const previousServiceAccountRuns = sumNamedCounts(previousSecurity?.service_account_runs || [], 'count');
  return (
    <div className="space-y-4">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<ShieldCheck />} label="Effective runs" value={formatNumber(effectiveRuns)} detail={`${formatNumber((security?.runs_by_effective_subject || []).length)} subjects`} delta={deltaValue(effectiveRuns, previousEffectiveRuns)} tone="blue" loading={loading} />
        <MetricCard icon={<ShieldCheck />} label="Service account runs" value={formatNumber(serviceAccountRuns)} detail="automation activity" delta={deltaValue(serviceAccountRuns, previousServiceAccountRuns)} tone="green" loading={loading} />
        <MetricCard icon={<Zap />} label="External callers" value={formatNumber((security?.external_trigger_callers || []).length)} detail="caller identities" delta={deltaValue((security?.external_trigger_callers || []).length, (previousSecurity?.external_trigger_callers || []).length)} tone="amber" loading={loading} />
        <MetricCard icon={<AlertTriangle />} label="High-risk failures" value={formatNumber(security?.high_risk_failed_pipelines?.length)} detail="failed privileged runs" delta={deltaValue(security?.high_risk_failed_pipelines?.length, previousSecurity?.high_risk_failed_pipelines?.length)} tone="red" loading={loading} />
      </section>
      <section className="grid gap-4 xl:grid-cols-3">
        <Panel title="Requesters" icon={<ShieldCheck className="h-4 w-4" />}>
          <NamedCountList items={security?.runs_by_requester || []} loading={loading} value="count" />
        </Panel>
        <Panel title="Effective Subjects" icon={<ShieldCheck className="h-4 w-4" />}>
          <NamedCountList items={security?.runs_by_effective_subject || []} loading={loading} value="count" />
        </Panel>
        <Panel title="External Callers" icon={<Zap className="h-4 w-4" />}>
          <NamedCountList items={security?.external_trigger_callers || []} loading={loading} value="count" />
        </Panel>
      </section>
      <section className="grid gap-4 xl:grid-cols-2">
        <Panel title="Service Account Runs" icon={<ShieldCheck className="h-4 w-4" />}>
          <NamedCountList items={security?.service_account_runs || []} loading={loading} value="count" />
        </Panel>
        <Panel title="High-Risk Failed Pipelines" icon={<AlertTriangle className="h-4 w-4" />}>
          <PerformanceTable rows={security?.high_risk_failed_pipelines || []} loading={loading} compact />
        </Panel>
      </section>
    </div>
  );
}

function MetricCard({
  icon,
  label,
  value,
  detail,
  delta,
  deltaFormat = 'number',
  positiveIsGood = false,
  tone,
  loading,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  detail: string;
  delta?: number;
  deltaFormat?: 'number' | 'percent' | 'duration';
  positiveIsGood?: boolean;
  tone: 'blue' | 'green' | 'amber' | 'red';
  loading: boolean;
}) {
  const toneClass = {
    blue: 'text-blue-500 bg-blue-500/10 border-blue-500/20',
    green: 'text-emerald-500 bg-emerald-500/10 border-emerald-500/20',
    amber: 'text-amber-500 bg-amber-500/10 border-amber-500/20',
    red: 'text-red-500 bg-red-500/10 border-red-500/20',
  }[tone];
  const deltaLabel = delta == null || loading ? '' : formatDelta(delta, deltaFormat);
  const deltaClass = delta == null ? '' : deltaPillClass(delta, positiveIsGood);
  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-[var(--text-secondary)]">{label}</p>
          <p className="mt-1 truncate text-xl font-semibold text-[var(--text-primary)]">{loading ? '...' : value}</p>
        </div>
        <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-md border ${toneClass}`}>
          <span className="[&>svg]:h-5 [&>svg]:w-5">{icon}</span>
        </div>
      </div>
      <div className="mt-3 flex min-w-0 items-center gap-2">
        <p className="min-w-0 flex-1 truncate text-sm text-[var(--text-secondary)]">{loading ? 'Loading' : detail}</p>
        {deltaLabel ? <span className={`shrink-0 rounded-md border px-1.5 py-0.5 text-[11px] font-semibold ${deltaClass}`}>{deltaLabel}</span> : null}
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

function PerformanceSummaryCards({ rows, previousRows, loading, noun }: { rows: MonitoringPerformanceRow[]; previousRows: MonitoringPerformanceRow[]; loading: boolean; noun: string }) {
  const currentRuns = sumPerformanceRows(rows, 'total_runs');
  const previousRuns = sumPerformanceRows(previousRows, 'total_runs');
  const currentFailures = sumPerformanceRows(rows, 'failed_runs');
  const previousFailures = sumPerformanceRows(previousRows, 'failed_runs');
  const currentP95 = averagePerformanceRows(rows, 'p95_duration_seconds');
  const previousP95 = averagePerformanceRows(previousRows, 'p95_duration_seconds');
  return (
    <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <MetricCard icon={<Workflow />} label={noun} value={formatNumber(rows.length)} detail="tracked rows" delta={deltaValue(rows.length, previousRows.length)} positiveIsGood tone="blue" loading={loading} />
      <MetricCard icon={<Activity />} label="Runs" value={formatNumber(currentRuns)} detail="selected window" delta={deltaValue(currentRuns, previousRuns)} tone="green" loading={loading} />
      <MetricCard icon={<XCircle />} label="Failures" value={formatNumber(currentFailures)} detail="failed runs" delta={deltaValue(currentFailures, previousFailures)} tone="red" loading={loading} />
      <MetricCard icon={<Timer />} label="Average p95" value={formatDurationSeconds(currentP95)} detail="row average" delta={deltaValue(currentP95, previousP95)} deltaFormat="duration" tone="amber" loading={loading} />
    </section>
  );
}

function TimeBucketChart({ buckets, loading }: { buckets: MonitoringTimeBucket[]; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading trend" />;
  if (!buckets.length || buckets.every(bucket => safeNumber(bucket.runs) === 0)) return <EmptyBlock label="No trend data" />;

  const visibleBuckets = buckets.slice(-14);
  const maxRuns = Math.max(1, ...visibleBuckets.map(bucket => safeNumber(bucket.runs)));
  const firstBucket = visibleBuckets[0];
  const lastBucket = visibleBuckets[visibleBuckets.length - 1];
  return (
    <div className="h-64 space-y-2">
      <div className="flex min-w-0 items-center justify-between gap-3 text-xs text-[var(--text-secondary)]">
        <span className="truncate" title={bucketFullLabel(firstBucket)}>{bucketFullLabel(firstBucket)}</span>
        <span className="shrink-0 text-[11px] uppercase">to</span>
        <span className="truncate text-right" title={bucketFullLabel(lastBucket)}>{bucketFullLabel(lastBucket)}</span>
      </div>
      <div className="flex h-52 items-end gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 pb-8 pt-4">
        {visibleBuckets.map(bucket => (
          <div key={bucket.key} className="flex min-w-0 flex-1 flex-col items-center justify-end gap-2" title={`${bucketFullLabel(bucket)}: ${formatNumber(bucket.runs)} run(s)`} aria-label={`${bucketFullLabel(bucket)} ${formatNumber(bucket.runs)} runs`}>
            <div className="flex h-36 w-full items-end rounded-sm bg-[var(--bg-tertiary)]/70">
              <div
                className="w-full rounded-sm bg-blue-500/80 transition-all"
                style={{ height: `${Math.max(4, (safeNumber(bucket.runs) / maxRuns) * 100)}%` }}
              />
            </div>
            <span className="w-full whitespace-nowrap text-center text-[10px] text-[var(--text-secondary)]">{bucketShortLabel(bucket)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function RunnerHistoryChart({ buckets, loading }: { buckets: MonitoringRunnerHistoryBucket[]; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading runner history" />;
  if (!buckets.length) return <EmptyBlock label="No runner history" />;
  const visibleBuckets = buckets.slice(-14);
  const maxCapacity = Math.max(1, ...visibleBuckets.map(bucket => safeNumber(bucket.capacity)));
  const maxQueued = Math.max(1, ...visibleBuckets.map(bucket => safeNumber(bucket.queued_jobs)));
  const firstBucket = visibleBuckets[0];
  const lastBucket = visibleBuckets[visibleBuckets.length - 1];
  return (
    <div className="space-y-4">
      <div className="flex min-w-0 items-center justify-between gap-3 text-xs text-[var(--text-secondary)]">
        <span className="truncate" title={bucketFullLabel(firstBucket)}>{bucketFullLabel(firstBucket)}</span>
        <span className="shrink-0 text-[11px] uppercase">to</span>
        <span className="truncate text-right" title={bucketFullLabel(lastBucket)}>{bucketFullLabel(lastBucket)}</span>
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(220px,0.45fr)]">
        <div className="flex h-56 items-end gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 pb-8 pt-4">
          {visibleBuckets.map(bucket => {
            const capacityHeight = Math.max(4, (safeNumber(bucket.capacity) / maxCapacity) * 100);
            const activeHeight = Math.max(0, Math.min(capacityHeight, (safeNumber(bucket.active_jobs) / maxCapacity) * 100));
            return (
              <div key={bucket.key} className="flex min-w-0 flex-1 flex-col items-center justify-end gap-2" title={`${bucketFullLabel(bucket)}: ${formatNumber(bucket.active_jobs)} active / ${formatNumber(bucket.capacity)} capacity, ${formatNumber(bucket.queued_jobs)} queued`}>
                <div className="relative flex h-36 w-full items-end rounded-sm bg-[var(--bg-tertiary)]/70">
                  <div className="absolute bottom-0 left-0 right-0 rounded-sm bg-blue-500/30" style={{ height: `${capacityHeight}%` }} />
                  <div className="relative z-10 w-full rounded-sm bg-emerald-500/80" style={{ height: `${Math.max(3, activeHeight)}%` }} />
                </div>
                <span className="w-full whitespace-nowrap text-center text-[10px] text-[var(--text-secondary)]">{bucketShortLabel(bucket)}</span>
              </div>
            );
          })}
        </div>
        <div className="space-y-3">
          {visibleBuckets.slice(-6).map(bucket => (
            <div key={bucket.key} className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="truncate text-sm font-medium text-[var(--text-primary)]" title={bucketFullLabel(bucket)}>{bucketShortLabel(bucket)}</span>
                <span className="text-xs font-semibold text-[var(--text-secondary)]">{formatPercent(bucket.utilization)}</span>
              </div>
              <div className="mt-2 h-2 overflow-hidden rounded-full bg-[var(--bg-tertiary)]">
                <div className="h-full rounded-full bg-amber-500" style={{ width: `${Math.max(4, (safeNumber(bucket.queued_jobs) / maxQueued) * 100)}%` }} />
              </div>
              <p className="mt-1 text-xs text-[var(--text-secondary)]">{formatNumber(bucket.queued_jobs)} queued</p>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function StatusSplit({ counts, total, loading }: { counts: Record<string, number>; total: number; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading statuses" />;
  if (!total) return <EmptyBlock label="No status data" />;
  const statuses = Object.entries(counts).filter(([, count]) => count > 0);
  return (
    <div className="space-y-5">
      <div className="flex h-5 overflow-hidden rounded-full bg-[var(--bg-tertiary)]">
        {statuses.map(([status, count]) => (
          <div key={status} className={statusColor(status)} style={{ width: `${Math.max(1, (count / total) * 100)}%` }} title={`${status}: ${count}`} />
        ))}
      </div>
      <div className="grid gap-x-5 gap-y-2 sm:grid-cols-2">
        {statuses.map(([status, count]) => (
          <div key={status} className="flex items-center justify-between gap-3 py-1.5">
            <span className="inline-flex min-w-0 items-center gap-2 text-sm text-[var(--text-secondary)]">
              <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${statusColor(status)}`} />
              <span className="truncate capitalize">{status}</span>
            </span>
            <span className="text-sm font-semibold text-[var(--text-primary)]">{formatNumber(count)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

type NamedCountValue = 'count' | 'rate' | 'seconds' | 'tokens';

function NamedCountList({ items, loading, value, linkForItem }: { items: MonitoringNamedCount[]; loading: boolean; value: NamedCountValue; linkForItem?: (item: MonitoringNamedCount) => string }) {
  if (loading) return <EmptyBlock label="Loading data" />;
  if (!items.length) return <EmptyBlock label="No data" />;
  const max = Math.max(1, ...items.map(item => namedCountValue(item, value)));
  return (
    <div className="space-y-3">
      {items.slice(0, 10).map(item => {
        const current = namedCountValue(item, value);
        const label = item.label || item.key || 'Unknown';
        const href = linkForItem?.(item) || '';
        return (
          <div key={`${item.key}-${item.label}`} className="min-w-0">
            <div className="flex items-center justify-between gap-3 text-sm">
              {href ? (
                <Link to={href} className="truncate font-medium text-[var(--text-link)] hover:underline" title={label}>{label}</Link>
              ) : (
                <span className="truncate font-medium text-[var(--text-primary)]" title={label}>{label}</span>
              )}
              <span className="shrink-0 text-[var(--text-secondary)]">{formatNamedCount(item, value)}</span>
            </div>
            <div className="mt-2 h-2 overflow-hidden rounded-full bg-[var(--bg-tertiary)]">
              <div className="h-full rounded-full bg-blue-500" style={{ width: `${Math.max(4, (current / max) * 100)}%` }} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

function ExternalTriggerLastFiredList({ items, loading }: { items: MonitoringExternalTriggerLastFired[]; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading triggers" />;
  if (!items.length) return <EmptyBlock label="No trigger activity" />;
  return (
    <div className="divide-y divide-[var(--border-primary)]">
      {items.slice(0, 12).map(item => {
        const href = externalTriggerHref(item.id);
        return (
          <div key={item.id} className="flex items-start justify-between gap-3 py-3 first:pt-0 last:pb-0">
            <div className="min-w-0">
              {href ? (
                <Link to={href} className="block truncate text-sm font-semibold text-[var(--text-link)] hover:underline" title={item.id}>{item.name || item.id}</Link>
              ) : (
                <p className="truncate text-sm font-semibold text-[var(--text-primary)]">{item.name || item.id || 'Unknown trigger'}</p>
              )}
              <p className="mt-1 truncate text-xs text-[var(--text-secondary)]">{item.last_used_at ? formatDateTime(item.last_used_at) : 'Never fired'}</p>
            </div>
            <div className="flex shrink-0 flex-col items-end gap-1">
              <span className={`rounded-md border px-2 py-1 text-xs font-semibold ${item.enabled ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300' : 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300'}`}>
                {item.enabled ? 'Enabled' : 'Disabled'}
              </span>
              {item.rate_limit ? <span className="text-xs text-[var(--text-secondary)]">{item.rate_limit}</span> : null}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function MetricInline({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-[var(--border-primary)] py-3 first:pt-0 last:border-b-0 last:pb-0">
      <span className="text-sm text-[var(--text-secondary)]">{label}</span>
      <span className="text-sm font-semibold text-[var(--text-primary)]">{value}</span>
    </div>
  );
}

function PerformanceTable({ rows, loading, compact = false }: { rows: MonitoringPerformanceRow[]; loading: boolean; compact?: boolean }) {
  if (loading) return <EmptyBlock label="Loading performance" />;
  if (!rows.length) return <EmptyBlock label="No performance data" />;
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-left text-sm">
        <thead className="text-xs uppercase text-[var(--text-secondary)]">
          <tr className="border-b border-[var(--border-primary)]">
            <th className="py-2 pr-3 font-semibold">Name</th>
            <th className="px-3 py-2 font-semibold">Runs</th>
            <th className="px-3 py-2 font-semibold">Success</th>
            <th className="px-3 py-2 font-semibold">p95</th>
            {!compact ? <th className="px-3 py-2 font-semibold">Avg queue</th> : null}
            <th className="px-3 py-2 font-semibold">Total</th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, compact ? 12 : 30).map(row => {
            const pipelineLink = pipelineHrefFromParts(row.pipeline_path, row.pipeline_name);
            const pipelineLabel = pipelineIdentifierFromParts(row.pipeline_path, row.pipeline_name);
            const name = displayPerformanceName(row);
            return (
              <tr key={row.key} className="border-b border-[var(--border-primary)] last:border-b-0">
                <td className="max-w-[24rem] py-3 pr-3">
                  {pipelineLink ? (
                    <Link to={pipelineLink} className="block truncate font-medium text-[var(--text-link)] hover:underline" title={name}>{name}</Link>
                  ) : (
                    <p className="truncate font-medium text-[var(--text-primary)]" title={name}>{name}</p>
                  )}
                  <p className="mt-0.5 truncate text-xs text-[var(--text-secondary)]" title={pipelineLabel}>{pipelineLabel}</p>
                </td>
                <td className="whitespace-nowrap px-3 py-3 text-[var(--text-primary)]">{formatNumber(row.total_runs)}</td>
                <td className="whitespace-nowrap px-3 py-3 text-[var(--text-primary)]">{formatPercent(row.success_rate)}</td>
                <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDurationSeconds(row.p95_duration_seconds)}</td>
                {!compact ? <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDurationSeconds(row.average_queue_seconds)}</td> : null}
                <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDurationSeconds(row.total_duration_seconds)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function RunTable({ rows, loading }: { rows: MonitoringRunRow[]; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading runs" />;
  if (!rows.length) return <EmptyBlock label="No runs" />;
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-left text-sm">
        <thead className="text-xs uppercase text-[var(--text-secondary)]">
          <tr className="border-b border-[var(--border-primary)]">
            <th className="py-2 pr-3 font-semibold">Run</th>
            <th className="px-3 py-2 font-semibold">Status</th>
            <th className="px-3 py-2 font-semibold">Duration</th>
            <th className="px-3 py-2 font-semibold">Queue</th>
            <th className="px-3 py-2 font-semibold">Trigger</th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, 20).map(row => {
            const pipelineLink = pipelineHrefFromParts(row.pipeline_path, row.pipeline_name);
            const pipelineLabel = pipelineIdentifierFromParts(row.pipeline_path, row.pipeline_name) || row.repo || truncateId(row.run_id, 8);
            return (
              <tr key={row.run_id} className="border-b border-[var(--border-primary)] last:border-b-0">
                <td className="max-w-[20rem] py-3 pr-3">
                  <Link to={runHref(row.run_id)} className="block truncate font-medium text-[var(--text-link)] hover:underline" title={row.run_id}>
                    {row.pipeline_name || row.run_id}
                  </Link>
                  {pipelineLink ? (
                    <Link to={pipelineLink} className="mt-0.5 block truncate text-xs text-[var(--text-secondary)] hover:text-[var(--text-link)]" title={pipelineLabel}>{pipelineLabel}</Link>
                  ) : (
                    <p className="mt-0.5 truncate text-xs text-[var(--text-secondary)]" title={pipelineLabel}>{pipelineLabel}</p>
                  )}
                </td>
                <td className="whitespace-nowrap px-3 py-3"><StatusPill status={row.status || 'unknown'} /></td>
                <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDurationSeconds(row.duration_seconds)}</td>
                <td className="whitespace-nowrap px-3 py-3 text-[var(--text-secondary)]">{formatDurationSeconds(row.queue_seconds)}</td>
                <td className="max-w-[12rem] truncate px-3 py-3 text-[var(--text-secondary)]" title={row.trigger_source}>{row.trigger_source || 'unknown'}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function Heatmap({ cells, loading }: { cells: Array<{ day_of_week: number; hour: number; runs: number; failures?: number }>; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading heatmap" />;
  if (!cells.length) return <EmptyBlock label="No heatmap data" />;
  const cellMap = new Map(cells.map(cell => [`${cell.day_of_week}-${cell.hour}`, cell]));
  const max = Math.max(1, ...cells.map(cell => safeNumber(cell.runs)));
  return (
    <div className="space-y-2">
      {[1, 2, 3, 4, 5, 6, 7].map(day => (
        <div key={day} className="grid grid-cols-[2rem_repeat(24,minmax(0,1fr))] gap-1">
          <span className="text-[10px] text-[var(--text-secondary)]">{dayLabel(day)}</span>
          {Array.from({ length: 24 }, (_, hour) => {
            const cell = cellMap.get(`${day}-${hour}`);
            const intensity = cell ? Math.max(0.12, safeNumber(cell.runs) / max) : 0.04;
            return (
              <div
                key={hour}
                className="aspect-square rounded-sm bg-blue-500"
                style={{ opacity: intensity }}
                title={`${dayLabel(day)} ${hour}:00 - ${formatNumber(cell?.runs)} runs`}
              />
            );
          })}
        </div>
      ))}
    </div>
  );
}

function ServiceStatusGrid({ services, unavailable, loading }: { services: ServiceStatus[]; unavailable: string | null; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading services" />;
  if (!services.length) return <EmptyBlock label={unavailable || 'No service status available'} />;
  return (
    <div className="space-y-4">
      {unavailable ? <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800/70 dark:bg-amber-950/30 dark:text-amber-100">{unavailable}</div> : null}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {services.map(service => (
          <div key={service.id} className="min-w-0 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold text-[var(--text-primary)]">{service.label}</p>
                <p className="mt-1 line-clamp-2 text-xs leading-5 text-[var(--text-secondary)]">{service.message || 'No status message.'}</p>
              </div>
              <span className={`shrink-0 rounded-md border px-2 py-1 text-xs font-semibold ${serviceStatusPillClass(service.status)}`}>{formatServiceStatusLabel(service.status)}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function RunnerStatusGrid({ runners, summary, unavailable, loading }: { runners: MonitoringRunner[]; summary: RunnerSummary; unavailable: string | null; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading runners" />;
  if (!runners.length) return <EmptyBlock label={unavailable || 'No runners registered'} />;
  return (
    <div className="space-y-4">
      {unavailable ? <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800/70 dark:bg-amber-950/30 dark:text-amber-100">{unavailable}</div> : null}
      <div className="grid grid-cols-2 gap-3">
        <RuntimeMini label="Total" value={formatNumber(summary.total)} />
        <RuntimeMini label="Online" value={formatNumber(summary.online)} />
        <RuntimeMini label="Unreachable" value={formatNumber(summary.unreachable)} />
        <RuntimeMini label="K8s" value={formatNumber(summary.kubernetes)} />
        <RuntimeMini label="Docker" value={formatNumber(summary.docker)} />
        <RuntimeMini label="Capacity" value={formatNumber(summary.capacity)} />
        <RuntimeMini label="Active" value={formatNumber(summary.activeJobs)} />
      </div>
      <div className="divide-y divide-[var(--border-primary)]">
        {runners.map(runner => (
          <RunnerStatusItem key={runner.runnerId || runner.label} runner={runner} />
        ))}
      </div>
    </div>
  );
}

function RunnerStatusItem({ runner }: { runner: MonitoringRunner }) {
  return (
    <div className="py-3 first:pt-0 last:pb-0">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Link to="/system/dispatcher" className="block truncate text-sm font-semibold text-[var(--text-link)] hover:underline" title={runner.runnerId || runner.label}>
            {runner.label || runner.runnerId}
          </Link>
          <p className="mt-1 text-xs text-[var(--text-secondary)]">{formatRunnerHeartbeat(runner.lastHeartbeatUnix)}</p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            <span className={`rounded-md border px-2 py-0.5 text-[11px] font-semibold ${runner.runtime === 'kubernetes' ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300' : 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300'}`}>
              {runner.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker'}
            </span>
            {runner.namespace ? <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]">ns {runner.namespace}</span> : null}
            {runner.node ? <span className="rounded-md border border-[var(--border-primary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]">node {runner.node}</span> : null}
          </div>
        </div>
        <span className={`shrink-0 rounded-md border px-2 py-1 text-xs font-semibold ${runnerStatusPillClass(runner.status)}`}>{formatRunnerStatusLabel(runner.status)}</span>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-4 text-xs">
        <MetricMini label="Capacity" value={formatNumber(runner.capacity)} />
        <MetricMini label="Active" value={formatNumber(runner.activeJobs)} />
        <MetricMini label="Inflight" value={formatNumber(runner.inflightJobs)} />
      </div>
      {runner.activeRuns.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {runner.activeRuns.slice(0, MAX_VISIBLE_RUNNER_RUNS).map(activeRun => (
            <Link key={activeRun.runId} to={runHref(activeRun.runId)} className="inline-flex max-w-full items-center rounded-md border border-blue-500/30 bg-blue-500/10 px-2 py-1 text-xs font-medium text-[var(--text-primary)] hover:border-[var(--border-accent)]">
              <span className="truncate">{`${activeRun.pipeline || 'Run'} ${truncateId(activeRun.runId, 6)}`}</span>
            </Link>
          ))}
        </div>
      ) : runner.activeJobs > 0 ? (
        <p className="mt-3 text-xs text-[var(--text-secondary)]">No visible active runs</p>
      ) : null}
    </div>
  );
}

function RunRefBlock({ run }: { run: { run_id: string; pipeline_name?: string; pipeline_path?: string; status?: string; duration_seconds?: number } }) {
  const pipelineLink = pipelineHrefFromParts(run.pipeline_path, run.pipeline_name);
  const pipelineLabel = pipelineIdentifierFromParts(run.pipeline_path, run.pipeline_name);
  return (
    <div className="space-y-3">
      <Link to={runHref(run.run_id)} className="block truncate text-sm font-semibold text-[var(--text-link)] hover:underline" title={run.run_id}>
        {run.pipeline_name || run.run_id}
      </Link>
      {pipelineLink ? (
        <Link to={pipelineLink} className="block truncate text-xs text-[var(--text-secondary)] hover:text-[var(--text-link)]" title={pipelineLabel}>{pipelineLabel}</Link>
      ) : null}
      <div className="grid grid-cols-2 gap-3 text-xs">
        <MetricMini label="Duration" value={formatDurationSeconds(run.duration_seconds)} />
        <MetricMini label="Status" value={run.status || 'unknown'} />
      </div>
    </div>
  );
}

function RecommendationList({ items, loading }: { items: string[]; loading: boolean }) {
  if (loading) return <EmptyBlock label="Loading recommendations" />;
  if (!items.length) return <EmptyBlock label="No recommendations" />;
  return (
    <div className="space-y-3">
      {items.map(item => (
        <div key={item} className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]">
          {item}
        </div>
      ))}
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

function StatusPill({ status }: { status: string }) {
  return <span className={`rounded-md border px-2 py-1 text-xs font-semibold ${statusPillClass(status)}`}>{status || 'unknown'}</span>;
}

function statusCountsFromSplit(items: MonitoringNamedCount[]) {
  return items.reduce<Record<string, number>>((acc, item) => {
    acc[item.label || item.key || 'unknown'] = safeNumber(item.count);
    return acc;
  }, {});
}

function displayPerformanceName(row: MonitoringPerformanceRow) {
  return [row.pipeline_name, row.step_name, row.task_name].filter(Boolean).join(' / ') || row.key || 'Unnamed';
}

function deriveStepRowsFromTaskRows(rows: MonitoringPerformanceRow[]): MonitoringPerformanceRow[] {
  const buckets = new Map<string, MonitoringPerformanceRow>();
  rows.forEach(row => {
    const stepName = row.step_name || 'Tasks';
    const key = [row.pipeline_path, row.pipeline_name, stepName].filter(Boolean).join('/') || stepName;
    const current = buckets.get(key) || {
      key,
      pipeline_path: row.pipeline_path,
      pipeline_name: row.pipeline_name,
      step_name: stepName,
      total_runs: 0,
      successful_runs: 0,
      failed_runs: 0,
      cancelled_runs: 0,
      timeout_runs: 0,
      success_rate: 0,
      failure_rate: 0,
      average_duration_seconds: 0,
      median_duration_seconds: 0,
      p95_duration_seconds: 0,
      p99_duration_seconds: 0,
      max_duration_seconds: 0,
      total_duration_seconds: 0,
      average_queue_seconds: 0,
    };
    current.total_runs = safeNumber(current.total_runs) + safeNumber(row.total_runs);
    current.successful_runs = safeNumber(current.successful_runs) + safeNumber(row.successful_runs);
    current.failed_runs = safeNumber(current.failed_runs) + safeNumber(row.failed_runs);
    current.cancelled_runs = safeNumber(current.cancelled_runs) + safeNumber(row.cancelled_runs);
    current.timeout_runs = safeNumber(current.timeout_runs) + safeNumber(row.timeout_runs);
    current.total_duration_seconds = safeNumber(current.total_duration_seconds) + safeNumber(row.total_duration_seconds);
    current.median_duration_seconds = Math.max(safeNumber(current.median_duration_seconds), safeNumber(row.median_duration_seconds));
    current.p95_duration_seconds = Math.max(safeNumber(current.p95_duration_seconds), safeNumber(row.p95_duration_seconds));
    current.p99_duration_seconds = Math.max(safeNumber(current.p99_duration_seconds), safeNumber(row.p99_duration_seconds));
    current.max_duration_seconds = Math.max(safeNumber(current.max_duration_seconds), safeNumber(row.max_duration_seconds));
    buckets.set(key, current);
  });
  return Array.from(buckets.values())
    .map(row => {
      const totalRuns = safeNumber(row.total_runs);
      const completedRuns = safeNumber(row.successful_runs) + safeNumber(row.failed_runs) + safeNumber(row.cancelled_runs);
      return {
        ...row,
        average_duration_seconds: totalRuns > 0 ? safeNumber(row.total_duration_seconds) / totalRuns : 0,
        success_rate: completedRuns > 0 ? safeNumber(row.successful_runs) / completedRuns : 0,
        failure_rate: completedRuns > 0 ? safeNumber(row.failed_runs) / completedRuns : 0,
      };
    })
    .sort((a, b) => safeNumber(b.p95_duration_seconds) - safeNumber(a.p95_duration_seconds) || safeNumber(b.total_runs) - safeNumber(a.total_runs));
}

function runHref(runID?: string) {
  const id = String(runID || '').trim();
  return id ? `/pipelineruns/recent?run=${encodeURIComponent(id)}` : '/pipelineruns/recent';
}

function pipelineIdentifierFromParts(path?: string, name?: string) {
  return [path, name].map(value => String(value || '').trim().replace(/^\/+|\/+$/g, '')).filter(Boolean).join('/');
}

function pipelineHrefFromParts(path?: string, name?: string) {
  return pipelineHrefFromIdentifier(pipelineIdentifierFromParts(path, name));
}

function pipelineHrefFromIdentifier(identifier?: string) {
  const normalized = String(identifier || '').trim().replace(/^\/+|\/+$/g, '');
  if (!normalized || normalized.toLowerCase() === 'unknown') return '';
  return `/pipelines/${encodeRouteIdentifier(normalized)}`;
}

function pipelineNamedCountHref(item: MonitoringNamedCount) {
  return pipelineHrefFromIdentifier(item.key || item.label);
}

function externalTriggerHref(id?: string) {
  const normalized = String(id || '').trim().replace(/^\/+|\/+$/g, '');
  if (!normalized || normalized.toLowerCase() === 'unknown') return '';
  return `/external-triggers/${encodeRouteIdentifier(normalized)}`;
}

function encodeRouteIdentifier(identifier: string) {
  return identifier.split('/').filter(Boolean).map(segment => encodeURIComponent(segment)).join('/');
}

function namedCountValue(item: MonitoringNamedCount, value: NamedCountValue) {
  if (value === 'rate') return safeNumber(item.rate);
  if (value === 'seconds') return safeNumber(item.seconds);
  if (value === 'tokens') return safeNumber(item.tokens);
  return safeNumber(item.count);
}

function formatNamedCount(item: MonitoringNamedCount, value: NamedCountValue) {
  if (value === 'rate') return formatPercent(item.rate);
  if (value === 'seconds') return formatDurationSeconds(item.seconds);
  if (value === 'tokens') return formatNumber(item.tokens);
  return formatNumber(item.count);
}

function statusColor(status: string) {
  const normalized = status.toLowerCase();
  if (normalized.includes('success')) return 'bg-emerald-500';
  if (normalized.includes('fail')) return 'bg-red-500';
  if (normalized.includes('running')) return 'bg-blue-500';
  if (normalized.includes('cancel')) return 'bg-orange-500';
  if (normalized.includes('approval')) return 'bg-amber-500';
  return 'bg-slate-400';
}

function statusPillClass(status: string) {
  const normalized = status.toLowerCase();
  if (normalized.includes('success')) return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300';
  if (normalized.includes('fail')) return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300';
  if (normalized.includes('running')) return 'border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-300';
  if (normalized.includes('cancel')) return 'border-orange-500/30 bg-orange-500/10 text-orange-700 dark:text-orange-300';
  return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300';
}

function serviceStatusPillClass(status: ServiceStatusValue) {
  if (status === 'ok') return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300';
  if (status === 'warning') return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300';
  if (status === 'error') return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300';
  return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300';
}

function formatServiceStatusLabel(status: ServiceStatusValue) {
  if (status === 'ok') return 'OK';
  if (status === 'warning') return 'Warning';
  if (status === 'error') return 'Error';
  return 'Unknown';
}

function runnerStatusPillClass(status: RunnerStatusValue) {
  if (status === 'online') return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300';
  if (status === 'unreachable') return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300';
  if (status === 'stale') return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300';
  if (status === 'disabled') return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300';
  return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300';
}

function formatRunnerStatusLabel(status: RunnerStatusValue) {
  if (status === 'online') return 'Online';
  if (status === 'unreachable') return 'Unreachable';
  if (status === 'stale') return 'Stale';
  if (status === 'disabled') return 'Disabled';
  return 'Unknown';
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

function dayLabel(day: number) {
  return ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'][Math.max(0, Math.min(6, day - 1))] || String(day);
}

function truncateId(value: string, length: number): string {
  const trimmed = value.trim();
  return trimmed.length <= length ? trimmed : trimmed.slice(0, length);
}

function deltaValue(current?: number | null, previous?: number | null): number | undefined {
  if (current == null || previous == null) return undefined;
  const safeCurrent = safeNumber(current);
  const safePrevious = safeNumber(previous);
  if (!Number.isFinite(safeCurrent) || !Number.isFinite(safePrevious)) return undefined;
  return safeCurrent - safePrevious;
}

function formatDelta(delta: number, format: 'number' | 'percent' | 'duration'): string {
  if (Math.abs(delta) < 0.000001) return '0';
  const sign = delta > 0 ? '+' : '-';
  const absolute = Math.abs(delta);
  if (format === 'percent') return `${sign}${Math.round(absolute * 100)}pp`;
  if (format === 'duration') return `${sign}${formatDurationSeconds(absolute)}`;
  return `${sign}${formatNumber(absolute)}`;
}

function deltaPillClass(delta: number, positiveIsGood: boolean): string {
  if (Math.abs(delta) < 0.000001) return 'border-slate-500/30 bg-slate-500/10 text-slate-600 dark:text-slate-300';
  const good = positiveIsGood ? delta > 0 : delta < 0;
  if (good) return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300';
  return 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-300';
}

function sumNamedCounts(items: MonitoringNamedCount[], field: NamedCountValue): number {
  return items.reduce((sum, item) => sum + namedCountValue(item, field), 0);
}

function sumPerformanceRows(rows: MonitoringPerformanceRow[], field: keyof MonitoringPerformanceRow): number {
  return rows.reduce((sum, row) => sum + safeNumber(row[field] as number | undefined), 0);
}

function averagePerformanceRows(rows: MonitoringPerformanceRow[], field: keyof MonitoringPerformanceRow): number {
  const values = rows.map(row => safeNumber(row[field] as number | undefined)).filter(value => value > 0);
  if (!values.length) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function safeNumber(value?: number | null): number {
  return Number.isFinite(value) ? Number(value) : 0;
}

function formatNumber(value?: number | null): string {
  return new Intl.NumberFormat().format(safeNumber(value));
}

function formatPercent(value?: number | null): string {
  return `${Math.round(safeNumber(value) * 100)}%`;
}

function formatDurationSeconds(seconds?: number | null): string {
  const safeSeconds = Math.round(safeNumber(seconds));
  if (safeSeconds <= 0) return '0s';
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  const remainingSeconds = safeSeconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${remainingSeconds}s`;
  return `${remainingSeconds}s`;
}

function formatDateTime(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function bucketShortLabel(bucket?: { key?: string; label?: string }): string {
  const raw = bucket?.key || bucket?.label || '';
  const date = parseBucketDate(raw);
  if (date) {
    const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
    if (bucketHasTime(raw)) {
      options.hour = '2-digit';
      options.minute = '2-digit';
    }
    return new Intl.DateTimeFormat(undefined, options).format(date);
  }
  return shortBucketFallback(bucket?.label || raw);
}

function bucketFullLabel(bucket?: { key?: string; label?: string }): string {
  const raw = bucket?.key || bucket?.label || '';
  const date = parseBucketDate(raw);
  if (date) {
    const options: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'short', day: 'numeric' };
    if (bucketHasTime(raw)) {
      options.hour = '2-digit';
      options.minute = '2-digit';
    }
    return new Intl.DateTimeFormat(undefined, options).format(date);
  }
  return bucket?.label || raw || 'Unknown';
}

function parseBucketDate(raw: string): Date | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  let normalized = trimmed;
  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
    normalized = `${trimmed}T00:00:00Z`;
  } else if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(trimmed)) {
    normalized = `${trimmed.replace(' ', 'T')}:00Z`;
  } else if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(trimmed)) {
    normalized = `${trimmed.replace(' ', 'T')}Z`;
  }
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? null : date;
}

function bucketHasTime(raw: string): boolean {
  return /\d{1,2}:\d{2}/.test(raw);
}

function shortBucketFallback(label: string): string {
  const normalized = label.trim();
  if (normalized.length <= 14) return normalized;
  return normalized.replace(/^(\d{4})-/, '').slice(0, 14);
}
