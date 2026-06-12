import { useMemo } from 'react';
import type { ReactNode } from 'react';
import { CalendarClock, ChevronRight, Trash2, Webhook } from 'lucide-react';
import type { RunListItem } from './contracts';
import {
  buildStatusTimeline,
  aiUsageTotalTokens,
  formatBranch,
  formatBranchDisplay,
  formatTokenCount,
  formatRepoLabel,
  formatTriggerId,
  getBranchStatusTone,
  getStatusDotClass,
  runTimestamp,
  summarizeStatus,
  timeAgo,
} from './runPresentation';
import { STATUS_META, getStatusMeta, normalizeStatus } from './statusPresentation';

type BranchEventGroup = {
  id: string;
  runs: RunListItem[];
  status: string;
  startedAt?: string;
  actor?: string;
  branchLabel?: string;
  commitLabel?: string;
};

export function BranchIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="6" y1="3" x2="6" y2="15" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 01-9 9" />
    </svg>
  );
}

export function CommitIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="3" />
      <path d="M3 12h6" />
      <path d="M15 12h6" />
    </svg>
  );
}

export function ZapIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
    </svg>
  );
}

export function BranchRunsSection({
  branch,
  runs,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsed,
  onToggleBranch,
  onDeleteBranch,
}: {
  branch: string;
  runs: RunListItem[];
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsed: boolean;
  onToggleBranch: () => void;
  onDeleteBranch: () => void;
}) {
  const branchLabel = formatBranch(branch);
  const sortedRuns = useMemo(() => [...runs].sort((a, b) => runTimestamp(b) - runTimestamp(a)), [runs]);
  const latestRun = sortedRuns[0];
  const latestStatus = normalizeStatus(latestRun?.status, latestRun?.is_complete);
  const latestTime = latestRun ? timeAgo(latestRun.started_at || latestRun.finished_at) : '—';

  const events = useMemo<BranchEventGroup[]>(() => {
    const bucket = new Map<string, RunListItem[]>();
    sortedRuns.forEach(run => {
      const key = run.trigger_event_id || run.run_id || 'unknown';
      const list = bucket.get(key) || [];
      list.push(run);
      bucket.set(key, list);
    });
    return Array.from(bucket.entries())
      .map(([id, items]) => {
        const ordered = [...items].sort((a, b) => runTimestamp(b) - runTimestamp(a));
        const newest = ordered[0];
        return {
          id,
          runs: ordered,
          status: summarizeStatus(ordered),
          startedAt: newest?.started_at || newest?.finished_at,
          actor: newest?.git_pusher_name,
          branchLabel: formatBranchDisplay(newest?.git_ref, newest?.git_target_ref),
          commitLabel: newest?.git_commit_sha ? newest.git_commit_sha.slice(0, 8) : undefined,
        };
      })
      .sort((a, b) => runTimestamp(b.runs[0]) - runTimestamp(a.runs[0]));
  }, [sortedRuns]);

  const timeline = useMemo(() => buildStatusTimeline(sortedRuns, 40), [sortedRuns]);

  return (
    <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-[0_10px_24px_rgba(0,0,0,0.12)] overflow-hidden" data-branch-row={branch}>
      <button
        type="button"
        className="w-full flex flex-col gap-3 px-4 sm:px-5 py-3 text-left hover:bg-[var(--bg-tertiary)] transition-colors sm:flex-row sm:items-center sm:gap-4 sm:flex-nowrap sm:justify-between"
        onClick={onToggleBranch}
        aria-expanded={!collapsed}
        aria-label={`${collapsed ? 'Expand' : 'Collapse'} branch ${branchLabel || branch}`}
      >
        <div className="flex items-center gap-3 min-w-[180px] sm:min-w-[240px] flex-1">
          <ChevronRight
            className={`h-5 w-5 text-[var(--text-secondary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}
            aria-hidden="true"
          />
          <span className="h-5 w-5 flex items-center justify-center text-[var(--text-link)]">
            <BranchIcon className="h-4 w-4" />
          </span>
          <span className="text-base font-semibold text-[var(--text-primary)] break-words" title={branchLabel || branch}>
            {branchLabel || branch}
          </span>
        </div>
        <div className="flex items-center gap-3 sm:gap-4 text-xs text-[var(--text-secondary)] sm:flex-1 sm:flex-nowrap flex-wrap justify-end">
          <div className="flex items-center gap-2 flex-nowrap overflow-hidden pr-1 sm:pr-0 sm:ml-auto">
            <StatusTimeline items={timeline} />
          </div>
          <span className="whitespace-nowrap">({sortedRuns.length} {sortedRuns.length === 1 ? 'run' : 'runs'})</span>
          <span className="hidden sm:inline-block h-4 border-l border-[var(--border-primary)]" aria-hidden="true" />
          <span className="whitespace-nowrap">Latest: {latestTime}</span>
          <span className="hidden sm:inline-block h-4 border-l border-[var(--border-primary)]" aria-hidden="true" />
          <BranchStatusIcon status={latestStatus} />
          <button
            type="button"
            className="ml-2 h-8 w-8 flex items-center justify-center rounded-full text-[var(--text-secondary)] hover:text-red-400 hover:bg-[var(--bg-tertiary)] border border-transparent hover:border-[var(--border-primary)]"
            aria-label={`Delete branch ${branchLabel || branch}`}
            onClick={event => {
              event.stopPropagation();
              onDeleteBranch();
            }}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </button>
      {!collapsed && (
        <div className="border-t border-[var(--border-primary)] p-4 sm:p-5 space-y-4 bg-[var(--bg-primary)]">
          {events.map(event => (
            <BranchEventCard
              key={event.id}
              event={event}
              onOpenRun={onOpenRun}
              onSelectRun={onSelectRun}
              selectedRunIds={selectedRunIds}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function RunCollection({
  runs,
  viewMode,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
}: {
  runs: RunListItem[];
  viewMode: 'grid' | 'list';
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
}) {
  if (!runs.length) {
    return <div className="text-sm text-[var(--text-secondary)]">No runs to display.</div>;
  }

  if (viewMode === 'list') {
    return (
      <div className="flex flex-col gap-3">
        {runs.map(run => (
          <ListRunRow
            key={run.run_id}
            run={run}
            selected={selectedRunIds.has(run.run_id)}
            onSelect={() => onSelectRun(run.run_id)}
            onOpen={() => onOpenRun(run.run_id)}
          />
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-4 lg:grid-cols-4 gap-4">
      {runs.map(run => (
        <RunCard
          key={run.run_id}
          run={run}
          selected={selectedRunIds.has(run.run_id)}
          onSelect={() => onSelectRun(run.run_id)}
          onOpen={() => onOpenRun(run.run_id)}
        />
      ))}
    </div>
  );
}

function StatusTimeline({ items }: { items: { key: string; status: string }[] }) {
  if (!items.length) {
    return <span className="text-xs text-[var(--text-secondary)]">No runs yet</span>;
  }
  return (
    <div className="flex items-center gap-1.5 flex-nowrap overflow-hidden" aria-hidden="true">
      {items.map(item => (
        <span key={item.key} className={`h-2 w-2 rounded-full ${getStatusDotClass(item.status)}`} title={item.status} />
      ))}
    </div>
  );
}

function BranchEventCard({
  event,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
}: {
  event: BranchEventGroup;
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
}) {
  if (!event.runs.length) return null;
  const meta = getStatusMeta(event.status, event.status === 'success');
  const triggerLabel = formatTriggerId(event.id);
  return (
    <div className="border border-[var(--border-primary)] rounded-xl bg-[var(--bg-secondary)] shadow-[0_10px_28px_rgba(0,0,0,0.12)]">
      <div className="flex items-center justify-between gap-3 px-4 py-3 border-b border-[var(--border-primary)] text-xs text-[var(--text-secondary)]">
        <div className="flex items-center gap-3 min-w-0 flex-1 flex-nowrap overflow-hidden">
          <span className={`runner-pill ${meta.pillClass} flex-shrink-0`}>{meta.text}</span>
          <div className="flex items-center gap-2 min-w-0 flex-nowrap overflow-hidden text-xs text-[var(--text-secondary)]">
            <span className="text-sm font-semibold text-[var(--text-primary)] truncate" title={triggerLabel.full}>
              Event: {triggerLabel.display}
            </span>
            {event.startedAt && (
              <span className="inline-flex items-center gap-1 whitespace-nowrap">
                <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {timeAgo(event.startedAt)}
              </span>
            )}
            {event.actor && (
              <span className="inline-flex items-center gap-1 whitespace-nowrap">
                <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                </svg>
                {event.actor}
              </span>
            )}
            {event.commitLabel && (
              <span className="inline-flex items-center gap-1 font-mono whitespace-nowrap">
                <CommitIcon className="h-3.5 w-3.5" />
                {event.commitLabel}
              </span>
            )}
          </div>
        </div>
        <span className="px-3 py-1 rounded-full text-[11px] bg-[var(--bg-primary)] border border-[var(--border-primary)] text-[var(--text-secondary)] whitespace-nowrap">
          {event.runs.length} {event.runs.length === 1 ? 'run' : 'runs'}
        </span>
      </div>
      <div className="p-4 grid gap-3 sm:grid-cols-4 xl:grid-cols-4">
        {event.runs.map(run => (
          <RunCard
            key={run.run_id}
            run={run}
            selected={selectedRunIds.has(run.run_id)}
            onSelect={() => onSelectRun(run.run_id)}
            onOpen={() => onOpenRun(run.run_id)}
            variant="event"
          />
        ))}
      </div>
    </div>
  );
}

function getFailurePreview(reason?: string): { title: string; detail?: string } | null {
  const lines = (reason || '')
    .split('\n')
    .map(line => line.trim())
    .filter(Boolean);
  if (!lines.length) return null;
  const whyLine = lines.find(line => line.startsWith('Why: '));
  const decisionLine = lines.find(line => line.startsWith('Decision reason: '));
  return {
    title: lines[0],
    detail: whyLine || decisionLine,
  };
}

export function RunCard({
  run,
  selected,
  onSelect,
  onOpen,
  variant = 'default',
  showSelect = true,
}: {
  run: RunListItem;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
  variant?: 'default' | 'event';
  showSelect?: boolean;
}) {
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
  const repoLabel = formatRepoLabel(run);
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const failurePreview = getFailurePreview(run.failure_reason);
  const aiTokens = aiUsageTotalTokens(run.ai_usage);
  const cardTone =
    variant === 'event'
      ? 'border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-[0_6px_18px_rgba(0,0,0,0.12)]'
      : 'border-[var(--border-primary)] bg-transparent shadow-sm';
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={event => {
        if (event.key === 'Enter') onOpen();
      }}
      className={`run-card run-card--grid p-4 flex flex-col justify-between ${cardTone} hover:border-[var(--border-accent)] rounded-2xl ${selected ? 'run-link-highlight' : ''}`}
      data-trigger-id={run.trigger_event_id || ''}
      data-run-id={run.run_id}
    >
      <div className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div className="flex-1 min-w-0 pr-2">
            <div className="flex items-center gap-2 min-w-0">
              <RunStatusIcon status={run.status} complete={run.is_complete} />
              <div className="min-w-0">
                <p className="text-sm font-semibold text-[var(--text-primary)] truncate">{run.pipeline_name}</p>
                <p className="text-[11px] font-mono text-[var(--text-secondary)] truncate flex items-center gap-1">
                  <RunIdIcon className="h-3.5 w-3.5 flex-shrink-0" />
                  <span>{(run.run_id || 'N/A').slice(0, 8)}</span>
                </p>
                <div className="flex items-center gap-3 text-xs text-[var(--text-secondary)] mt-1 flex-wrap">
                </div>
              </div>
            </div>
          </div>
          <PipelineBadges run={run} />
        </div>
        <div className="text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="8" cy="7" r="2" />
              <circle cx="8" cy="17" r="2" />
              <circle cx="16" cy="7" r="2" />
              <path d="M10 7h4" />
              <path d="M8 9v6a4 4 0 004 4h4" />
            </svg>
            <span className="truncate" title="Source">{repoLabel}</span>
          </div>
          <div className="flex items-center">
            <BranchIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
            <span className="truncate" title="Branch">{branchLabel || 'N/A'}</span>
          </div>
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
            <span className="truncate">{run.git_pusher_name || 'N/A'}</span>
          </div>
          <div className="flex items-center">
            <CommitIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
            <span className="truncate" title="Commit Hash">{(run.git_commit_sha || 'N/A').slice(0, 8)}</span>
          </div>
          {aiTokens > 0 && (
            <div className="flex items-center">
              <ZapIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
              <span className="truncate" title="LLM tokens">{formatTokenCount(aiTokens)}</span>
            </div>
          )}
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M7 7a1 1 0 011-1h3.586a1 1 0 01.707.293l6.414 6.414a1 1 0 010 1.414l-4.586 4.586a1 1 0 01-1.414 0L7.293 13.707A1 1 0 017 13V9a1 1 0 011-1z" />
            </svg>
            <span className="truncate" title="Trigger Event ID">{triggerLabel.display}</span>
          </div>
        </div>
        {failurePreview && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-700/70 dark:bg-red-950/40 dark:text-red-200">
            <div className="font-semibold truncate" title={failurePreview.title}>{failurePreview.title}</div>
            {failurePreview.detail && (
              <div className="mt-1 truncate opacity-90" title={failurePreview.detail}>{failurePreview.detail}</div>
            )}
          </div>
        )}
      </div>
      <div className="mt-4 pt-3 border-t border-[var(--border-primary)] flex items-center justify-between text-xs text-[var(--text-secondary)]">
        <div className="flex items-center gap-2">
          <svg className="h-3.5 w-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="truncate">{timeAgo(timeToDisplay)}</span>
        </div>
        {showSelect && <RunSelectToggle selected={selected} onToggle={onSelect} />}
      </div>
    </div>
  );
}

function ListRunRow({ run, selected, onSelect, onOpen }: { run: RunListItem; selected: boolean; onSelect: () => void; onOpen: () => void }) {
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
  const repoLabel = formatRepoLabel(run);
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const commitLabel = (run.git_commit_sha || 'N/A').slice(0, 8);
  const runIdLabel = (run.run_id || 'N/A').slice(0, 8);
  const failurePreview = getFailurePreview(run.failure_reason);
  const aiTokens = aiUsageTotalTokens(run.ai_usage);
  return (
    <div
      className={`run-card run-card--list border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm rounded-2xl hover:border-[var(--border-accent)] ${selected ? 'run-link-highlight' : ''}`}
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={event => {
        if (event.key === 'Enter') onOpen();
      }}
      data-trigger-id={run.trigger_event_id || ''}
      data-run-id={run.run_id}
    >
      <div className="run-list-cell run-list-cell--main">
        <span className="run-list-icon">
          <RunStatusIcon status={run.status} complete={run.is_complete} />
        </span>
        <div className="run-list-main">
          <div className="run-list-title-row">
            <div className="run-list-title truncate" title={run.pipeline_name}>
              {run.pipeline_name}
            </div>
            <PipelineBadges run={run} />
          </div>
          <div className="run-list-chips">
            <span className="run-list-chip" title={repoLabel}>
              <svg className="h-3.5 w-3.5 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="8" cy="7" r="2" />
                <circle cx="8" cy="17" r="2" />
                <circle cx="16" cy="7" r="2" />
                <path d="M10 7h4" />
                <path d="M8 9v6a4 4 0 004 4h4" />
              </svg>
              <span className="truncate">{repoLabel}</span>
            </span>
            <span className="run-list-chip" title={branchLabel || 'N/A'}>
              <BranchIcon className="h-3.5 w-3.5 flex-shrink-0" />
              <span className="truncate">{branchLabel || 'N/A'}</span>
            </span>
            <span className="run-list-chip font-mono" title={`Run ${run.run_id || 'N/A'}`}>
              <RunIdIcon className="h-3.5 w-3.5 flex-shrink-0" />
              {runIdLabel}
            </span>
            {aiTokens > 0 && (
              <span className="run-list-chip font-mono" title="LLM tokens">
                <ZapIcon className="h-3.5 w-3.5 flex-shrink-0" />
                {formatTokenCount(aiTokens)}
              </span>
            )}
          </div>
          {failurePreview && (
            <div className="mt-2 max-w-full rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-700/70 dark:bg-red-950/40 dark:text-red-200">
              <div className="font-semibold truncate" title={failurePreview.title}>{failurePreview.title}</div>
              {failurePreview.detail && (
                <div className="mt-1 truncate opacity-90" title={failurePreview.detail}>{failurePreview.detail}</div>
              )}
            </div>
          )}
        </div>
      </div>
      <div className="run-list-cell">
        <span className="run-list-meta-label">Commit</span>
        <span className="run-list-meta-value font-mono">{commitLabel}</span>
      </div>
      <div className="run-list-cell">
        <span className="run-list-meta-label">Trigger</span>
        <span className="run-list-meta-value truncate" title={triggerLabel.full}>
          {triggerLabel.display}
        </span>
      </div>
      <div className="run-list-cell">
        <span className="run-list-meta-label">Updated</span>
        <span className="run-list-meta-value">{timeAgo(timeToDisplay)}</span>
      </div>
      <div className="run-list-cell run-list-cell--actions">
        <RunSelectToggle selected={selected} onToggle={onSelect} />
      </div>
    </div>
  );
}

export function RunStatusIcon({ status, complete }: { status: string; complete?: boolean }) {
  return <BranchStatusIcon status={status} complete={complete} className="run-status-icon" />;
}

export function RunIdIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M4 7h4v10H4z" />
      <path d="M12 7h8" />
      <path d="M12 12h8" />
      <path d="M12 17h8" />
    </svg>
  );
}

function BranchStatusIcon({ status, complete, className }: { status: string; complete?: boolean; className?: string }) {
  const rawStatus = (status || '').toLowerCase();
  const normalized = normalizeStatus(rawStatus, complete ?? Boolean(STATUS_META[rawStatus]));
  const tone = getBranchStatusTone(normalized);
  const isFailure = normalized === 'failure' || normalized === 'failure (ignored)' || normalized === 'rejected';
  const isCancelled = normalized === 'cancelled';
  const isRunning = normalized === 'running' || normalized === 'waiting_approval';
  const isSkipped = normalized === 'skipped';
  const isPending = normalized === 'pending';
  return (
    <span
      className={`inline-flex h-7 w-7 items-center justify-center rounded-full border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${tone} ${className || ''}`}
      aria-label={normalized}
    >
      {isRunning ? (
        <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12a9 9 0 11-6.219-8.56" />
        </svg>
      ) : isFailure || isCancelled ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M18 6L6 18" />
          <path d="M6 6l12 12" />
        </svg>
      ) : isSkipped ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10" />
          <path d="M6 12h12" />
        </svg>
      ) : isPending ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 8v4l3 3" />
          <path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      ) : (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 13l4 4L19 7" />
        </svg>
      )}
    </span>
  );
}

function RunSelectToggle({ selected, onToggle }: { selected: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      className={`run-select-toggle inline-flex items-center justify-center h-8 w-8 rounded-full border border-[var(--border-primary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--border-accent)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)] transition-colors duration-150 ${selected ? 'bg-[var(--bg-tertiary)]' : ''}`}
      aria-pressed={selected}
      onClick={event => {
        event.stopPropagation();
        onToggle();
      }}
      title={selected ? 'Deselect run' : 'Select run'}
    >
      <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M5 13l4 4L19 7" />
      </svg>
    </button>
  );
}

function PipelineBadges({ run }: { run: RunListItem }) {
  const badges: ReactNode[] = [];
  const external = run.trigger_source === 'external_trigger' || Boolean(run.external_trigger_id);
  if (external) {
    const label = run.external_trigger_name ? `External trigger: ${run.external_trigger_name}` : 'External trigger';
    badges.push(
      <span key="external" className="text-xs font-semibold text-[var(--text-link)] inline-flex items-center gap-1" title={label}>
        <Webhook className="h-4 w-4" />
        External
      </span>
    );
  }
  const scheduled = run.trigger_source === 'schedule' || Boolean(run.schedule_id);
  if (scheduled) {
    const label = run.schedule_name ? `Scheduled: ${run.schedule_name}` : 'Scheduled';
    badges.push(
      <span key="scheduled" className="text-xs font-semibold text-[var(--text-link)] inline-flex items-center gap-1" title={label}>
        <CalendarClock className="h-4 w-4" />
        Scheduled
      </span>
    );
  }
  if (run.pipeline_source === 'database override') {
    badges.push(
      <span key="override" className="text-xs font-semibold text-[var(--text-link)] inline-flex items-center gap-1">
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M13 16h-1v-4h-1m1-4h.01" />
          <path d="M12 2a10 10 0 100 20 10 10 0 000-20z" />
        </svg>
        Overridden
      </span>
    );
  }
  if (run.parent_run_id) {
    badges.push(
      <span key="included" className="text-xs font-semibold text-[var(--text-link)]">
        Included
      </span>
    );
  }
  if (!badges.length) return null;
  return <div className="flex flex-col items-end gap-1 text-right">{badges}</div>;
}

export function StatusBadge({ status, complete }: { status: string; complete?: boolean }) {
  const meta = getStatusMeta(status, complete);
  return (
    <span className={`runner-pill border ${meta.pillClass}`} title={meta.text}>
      <svg className={`h-4 w-4 ${meta.strokeClass}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d={meta.icon} />
      </svg>
      {meta.text}
    </span>
  );
}
