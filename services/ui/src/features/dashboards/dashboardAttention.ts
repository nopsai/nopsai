import {
  formatDateTime,
  refreshStatusLabel,
  type DashboardPublication,
  type DashboardRefresh,
  type DashboardRefreshSchedule,
  type DashboardRefreshSource,
  type DashboardSection,
  type DashboardSource,
} from './model.js';

export type DashboardAttentionTone = 'warning' | 'danger' | 'neutral';

export type DashboardAttentionSignal = {
  id: string;
  title: string;
  detail: string;
  action: string;
  tone: DashboardAttentionTone;
};

export function dashboardAttentionSignals({
  sections,
  sources,
  publications,
  latestRefresh,
  refreshSchedules,
}: {
  sections: DashboardSection[];
  sources: DashboardSource[];
  publications: DashboardPublication[];
  latestRefresh: DashboardRefresh | null;
  refreshSchedules: DashboardRefreshSchedule[];
}): DashboardAttentionSignal[] {
  const signals: DashboardAttentionSignal[] = [];
  const refreshSources = latestRefresh?.sources || [];
  const hasRefreshSourceIssue = refreshSources.some(source => {
    const status = `${source.status} ${source.pipeline_status || ''} ${source.output_status || ''}`.toLowerCase();
    return Boolean(source.error || /(failed|failure|timed_out|cancelled)/.test(status));
  });

  if (latestRefresh?.error) {
    signals.push({
      id: `refresh-error-${latestRefresh.id}`,
      title: refreshAttentionTitle(latestRefresh),
      detail: refreshAttentionDetail(latestRefresh, latestRefresh.error),
      action: refreshAttentionAction(latestRefresh),
      tone: attentionTone(latestRefresh.status),
    });
  } else if (latestRefresh && refreshNeedsReview(latestRefresh.status) && !hasRefreshSourceIssue) {
    signals.push({
      id: `refresh-status-${latestRefresh.id}`,
      title: refreshAttentionTitle(latestRefresh),
      detail: refreshAttentionDetail(latestRefresh),
      action: refreshAttentionAction(latestRefresh),
      tone: attentionTone(latestRefresh.status),
    });
  }

  for (const source of refreshSources) {
    const status = `${source.status} ${source.pipeline_status || ''} ${source.output_status || ''}`.toLowerCase();
    if (!source.error && !/(failed|failure|timed_out|cancelled)/.test(status)) continue;
    signals.push({
      id: `refresh-source-${source.id}`,
      title: `${source.pipeline_id} / ${source.output_name || source.entry_key || 'dashboard output'}`,
      detail: refreshSourceIssueDetail(source),
      action: 'Open dashboard details, inspect the latest run or refresh source, fix the pipeline output, then retry failed sources.',
      tone: attentionTone(source.output_status || source.pipeline_status || source.status),
    });
  }

  for (const publication of publications) {
    if (!publication.stale) continue;
    signals.push({
      id: `stale-${publication.id}`,
      title: `${publication.entry_key} is stale`,
      detail: `${publication.pipeline_id || publication.section_key} last published ${formatDateTime(publication.published_at) || 'earlier'}.`,
      action: 'Refresh the dashboard or rerun the pipeline that publishes this dashboard output.',
      tone: 'warning',
    });
  }

  for (const source of sources) {
    if (source.enabled || !source.required_for_refresh) continue;
    signals.push({
      id: `disabled-required-${source.id}`,
      title: `${source.pipeline_id} is disabled`,
      detail: `Required source ${source.output_name || source.entry_key || source.id} will not refresh until it is enabled.`,
      action: 'Open dashboard details, review the source binding, then enable or edit it.',
      tone: 'warning',
    });
  }

  const sectionsWithoutSources = sections.filter(section => !sources.some(source => source.section_key === section.section_key));
  for (const section of sectionsWithoutSources) {
    signals.push({
      id: `empty-section-${section.section_key}`,
      title: `${section.title} has no sources`,
      detail: `Section ${section.section_key} is configured but has no dashboard output bindings.`,
      action: 'Attach a dashboard-output pipeline source to this section or remove the empty section from dashboard edit.',
      tone: 'neutral',
    });
  }

  const schedule = refreshSchedules.find(item => item.enabled && item.last_status && attentionTone(item.last_status) !== 'neutral');
  if (schedule) {
    signals.push({
      id: `schedule-${schedule.id}`,
      title: `${schedule.name} last run ${refreshStatusLabel(schedule.last_status || '')}`,
      detail: `${schedule.cron_expression || schedule.cron} / ${schedule.timezone} / ${schedule.scope_type}`,
      action: 'Open dashboard details, inspect the schedule, then run it manually or update its cadence and target.',
      tone: attentionTone(schedule.last_status || ''),
    });
  }

  return signals.sort((left, right) => toneRank(right.tone) - toneRank(left.tone));
}

function refreshAttentionTitle(refresh: DashboardRefresh): string {
  const normalized = refresh.status.toLowerCase();
  if (normalized === 'cancelled') return 'Latest refresh was cancelled';
  if (normalized === 'timed_out') return 'Latest refresh timed out';
  if (normalized === 'partial') return 'Latest refresh completed partially';
  return `Latest refresh ${refreshStatusLabel(refresh.status)}`;
}

function refreshAttentionDetail(refresh: DashboardRefresh, error?: string): string {
  const normalized = refresh.status.toLowerCase();
  if (normalized === 'cancelled') {
    return 'A user or automation stopped this refresh before it finished. Existing dashboard cards stay unchanged until another refresh publishes new output.';
  }
  if (error) return error;
  const completed = `${refresh.successful_sources}/${refresh.total_sources} sources completed`;
  if (normalized === 'timed_out') {
    return `${completed} before the ${formatDurationSeconds(refresh.timeout_seconds)} timeout for ${refresh.scope_type} scope.`;
  }
  return `${completed} for ${refresh.scope_type} scope.`;
}

function refreshAttentionAction(refresh: DashboardRefresh): string {
  const normalized = refresh.status.toLowerCase();
  if (normalized === 'cancelled') return 'Start another refresh when you want new output published.';
  if (normalized === 'timed_out') return 'Open dashboard details, review slow sources, then rerun with a longer timeout or fix the slow pipeline.';
  if (normalized === 'partial') return 'Open dashboard details, review skipped or failed sources, then retry failed sources.';
  return 'Open dashboard details, review the refresh, then retry failed sources or fix the pipeline before rerunning.';
}

function refreshSourceIssueDetail(source: DashboardRefreshSource): string {
  const status = `${source.status} ${source.pipeline_status || ''} ${source.output_status || ''}`.toLowerCase();
  if (/cancelled/.test(status)) {
    return 'This output was not updated because the refresh was cancelled before it finished.';
  }
  return source.error || `${refreshSourceLabel(source)} needs review for ${source.section_key}.`;
}

function refreshSourceLabel(source: DashboardRefreshSource): string {
  return [
    source.status ? `refresh ${refreshStatusLabel(source.status)}` : '',
    source.pipeline_status ? `pipeline ${refreshStatusLabel(source.pipeline_status)}` : '',
    source.output_status ? `output ${refreshStatusLabel(source.output_status)}` : '',
  ].filter(Boolean).join(' / ');
}

function attentionTone(status: string): DashboardAttentionTone {
  const normalized = status.toLowerCase();
  if (normalized === 'failed' || normalized === 'failure' || normalized === 'timed_out') return 'danger';
  if (normalized === 'queued' || normalized === 'pending' || normalized === 'partial' || normalized === 'cancelled') return 'warning';
  return 'neutral';
}

function refreshNeedsReview(status: string): boolean {
  const normalized = status.toLowerCase();
  return normalized === 'failed' || normalized === 'failure' || normalized === 'timed_out' || normalized === 'cancelled' || normalized === 'partial';
}

function toneRank(tone: DashboardAttentionTone): number {
  if (tone === 'danger') return 3;
  if (tone === 'warning') return 2;
  return 1;
}

function formatDurationSeconds(rawSeconds: number): string {
  const seconds = Math.max(0, Math.round(rawSeconds));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return remainingSeconds ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}
