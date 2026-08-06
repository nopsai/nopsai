import { useCallback, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertTriangle, ArrowRight, BarChart3, BrainCircuit, FileText, GitCompare, Info, RefreshCw, Square, Trash2, Workflow, X } from 'lucide-react';
import { AnalysisModal } from '../analysis/AnalysisModal';
import { buildRunAnalysis } from '../analysis/model';
import type { PipelineDefinition, PipelineRunFinalOutput, RunListItem, StepDetail } from './contracts';
import { BranchIcon, CommitIcon, RunIdIcon, StatusBadge, ZapIcon } from './PipelineRunCards';
import { RunDetailWorkspaceTabs } from './RunDetailWorkspaceTabs';
import { buildRunAnalysisPromptContext } from './runAnalysisEvidence';
import {
  buildPipelineLink,
  buildRunMonitoringLink,
  formatAIUsageBreakdown,
  formatBranchDisplay,
  formatRepoLabel,
  formatRunTimestamp,
  formatTokenCount,
  formatTriggerId,
  runActivityTimestamp,
  runStartedTimestamp,
  timeAgo,
  type ParentRunInfo,
} from './runPresentation';
import { buildIgnoredFailureWarning } from './runWarnings';
import { getStatusMeta, normalizeStatus } from './statusPresentation';

type RunDetail = {
  run_info: RunListItem;
  steps: StepDetail[];
  pipeline_definition?: PipelineDefinition;
  pipeline_definition_yaml?: string;
  final_outputs?: PipelineRunFinalOutput[];
  child_runs: RunListItem[];
  parent_run_info?: ParentRunInfo | null;
  approvals?: PipelineApproval[];
};

type PipelineApproval = {
  id: string;
  run_id: string;
  step_name: string;
  task_name: string;
  approval_type: string;
  assigned_teams: string[];
  allow_self_approval: boolean;
  status: string;
  requested_at: string;
  requested_by_type?: string;
  requested_by_id?: string;
  decided_by_email?: string;
  decided_at?: string;
  decision_comment?: string;
};

export function RunDetailView({
  detail,
  loading,
  error,
  onClose,
  onCancel,
  onCancelOutput,
  onRetryOutput,
  onRerun,
  onDelete,
  selectedStep,
  onSelectStep,
  onOpenLogs,
  onOpenStepLogs,
  onOpenTaskLogs,
  onOpenStepDetail,
  onOpenRun,
  onShowDefinition,
  onApprovalDecision,
  approvalDecisionPending,
  comparisonRuns,
}: {
  detail: RunDetail;
  loading: boolean;
  error: string | null;
  onClose: () => void;
  onCancel: () => void;
  onCancelOutput: (outputId: string) => void;
  onRetryOutput: (outputId: string) => void;
  onRerun: () => void;
  onDelete: () => void;
  selectedStep: string | null;
  onSelectStep: (step: string | null) => void;
  onOpenLogs: () => void;
  onOpenStepLogs: (stepName: string) => void;
  onOpenTaskLogs: (stepName: string, taskName: string) => void;
  onOpenStepDetail: (stepName: string, taskName?: string) => void;
  onOpenRun: (id: string) => void;
  onShowDefinition: () => void;
  onApprovalDecision: (approval: PipelineApproval, decision: 'approve' | 'reject') => void;
  approvalDecisionPending: string | null;
  comparisonRuns: RunListItem[];
}) {
  const run = detail.run_info;
  const [metadataOpen, setMetadataOpen] = useState(false);
  const [analysisOpen, setAnalysisOpen] = useState(false);
  const normalizedStatus = normalizeStatus(run.status, run.is_complete);
  const isActiveRun = normalizedStatus === 'running' || normalizedStatus === 'pending' || normalizedStatus === 'waiting_approval';
  const approvals = detail.approvals || [];
  const pipelineLink = buildPipelineLink(run);
  const monitoringLink = buildRunMonitoringLink(run);
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const parentRun = detail.parent_run_info;
  const ignoredFailureWarning = useMemo(
    () => buildIgnoredFailureWarning({ steps: detail.steps, childRuns: detail.child_runs }),
    [detail.child_runs, detail.steps]
  );

  const actionBase =
    'inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-semibold transition duration-150 focus:outline-none';
  const ghostAction = `${actionBase} border border-[var(--border-primary)]/80 bg-[var(--bg-secondary)] text-[var(--text-primary)] shadow-[0_8px_22px_rgba(0,0,0,0.07)] hover:border-indigo-300/60 hover:text-indigo-600 dark:border-white/10 dark:bg-white/5 dark:text-[var(--text-primary)] dark:shadow-[0_8px_22px_rgba(0,0,0,0.22)] dark:hover:border-indigo-300/50 dark:hover:bg-white/10`;
  const primaryAction = `${actionBase} bg-gradient-to-r from-indigo-500 to-purple-500 text-[var(--text-button)] shadow-[0_10px_28px_rgba(79,70,229,0.22)] hover:shadow-[0_14px_34px_rgba(79,70,229,0.3)] focus:ring-2 focus:ring-offset-2 focus:ring-indigo-400`;
  const dangerAction = `${actionBase} border border-red-500/40 text-red-600 bg-red-50 hover:bg-red-100 dark:text-red-100 dark:bg-red-500/10 dark:hover:bg-red-500/20`;
  const iconDanger = 'inline-flex items-center justify-center h-9 w-9 rounded-lg p-0 text-red-600 hover:text-red-700 dark:text-red-200 dark:hover:text-red-100 bg-transparent border-none shadow-none';

  const startedAt = runStartedTimestamp(run);
  const startedLabel = timeAgo(startedAt);
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const repoLabel = formatRepoLabel(run);
  const isExternalTriggerRun = run.trigger_source === 'external_trigger' || Boolean(run.external_trigger_id);
  const analysisResult = useMemo(
    () => buildRunAnalysis({ detail, comparisonRuns }),
    [comparisonRuns, detail]
  );
  const loadAnalysisPromptContext = useCallback(
    () => buildRunAnalysisPromptContext(detail),
    [detail]
  );
  const externalCaller =
    run.external_trigger_caller_type && run.external_trigger_caller_id
      ? `${run.external_trigger_caller_type}:${run.external_trigger_caller_id}`
      : '—';

  const detailLines = [
    {
      label: 'Run ID',
      value: run.run_id || '—',
      subtext: run.duration ? `${run.duration} elapsed` : 'Elapsed: —',
      icon: <RunIdIcon className="h-4 w-4 text-slate-500" />,
    },
    {
      label: 'Commit',
      value: run.git_commit_sha || '—',
      subtext: run.git_pusher_name ? `Committer: ${run.git_pusher_name}` : 'Committer: —',
      icon: <CommitIcon className="h-4 w-4 text-slate-500" />,
    },
    {
      label: 'Trigger Event ID',
      value: triggerLabel.full || '—',
      subtext: startedAt ? `Started ${startedLabel}` : 'Started: —',
      icon: <ZapIcon className="h-4 w-4 text-slate-500" />,
    },
    {
      label: 'LLM tokens',
      value: formatTokenCount(run.ai_usage?.total_tokens),
      subtext: formatAIUsageBreakdown(run.ai_usage),
      icon: <BarChart3 className="h-4 w-4 text-slate-500" />,
    },
  ];


  const renderHeroStatus = () => {
    const pulseClasses = 'relative flex h-2.5 w-2.5';
    const baseCircle = 'absolute inline-flex h-full w-full rounded-full opacity-60';
    if (normalizedStatus === 'success') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-green-500/10 border border-green-500/30 text-green-700 dark:text-green-200 text-xs font-semibold">
          <span className={pulseClasses}>
            <span className={`${baseCircle} animate-ping bg-green-400`} />
            <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-green-500" />
          </span>
          Success
        </span>
      );
    }
    if (normalizedStatus === 'running') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-blue-500/10 border border-blue-500/30 text-blue-700 dark:text-blue-200 text-xs font-semibold">
          <svg className="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="10" className="opacity-30" />
            <path d="M12 2a10 10 0 0110 10" />
          </svg>
          Running
        </span>
      );
    }
    if (normalizedStatus === 'waiting_approval') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-cyan-500/10 border border-cyan-500/30 text-cyan-800 dark:text-cyan-100 text-xs font-semibold">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M9 11l3 3L22 4" />
            <path d="M21 12v7a2 2 0 01-2 2H5a2 2 0 01-2-2V5a2 2 0 012-2h11" />
          </svg>
          Waiting approval
        </span>
      );
    }
    if (normalizedStatus === 'rejected') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-rose-500/10 border border-rose-500/30 text-rose-700 dark:text-rose-100 text-xs font-semibold">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M18 6L6 18" />
            <path d="M6 6l12 12" />
          </svg>
          Rejected
        </span>
      );
    }
    if (normalizedStatus === 'failure') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-red-500/10 border border-red-500/30 text-red-700 dark:text-red-200 text-xs font-semibold">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M6 18L18 6M6 6l12 12" />
          </svg>
          Failed
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-slate-500/10 border border-slate-300 text-slate-700 dark:text-[var(--text-primary)] text-xs font-semibold">
        <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 8v4l3 3" />
          <circle cx="12" cy="12" r="10" />
        </svg>
        {getStatusMeta(normalizedStatus, run.is_complete).text}
      </span>
    );
  };

  return (
    <>
      <div className="space-y-4">
      <div className="rounded-lg border border-[var(--border-primary)] bg-white text-[var(--text-primary)] shadow-[0_10px_28px_rgba(8,10,24,0.08)] dark:border-white/10 dark:bg-gradient-to-br from-[#0b0c15] via-[#0c0f1f] to-[#0b0c15] dark:text-[var(--text-primary)] dark:shadow-[0_12px_32px_rgba(8,10,24,0.38)] overflow-hidden">
        <div className="p-3 flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-xl lg:text-[1.35rem] font-black tracking-tight text-[var(--text-primary)] dark:text-[var(--text-primary)]">{run.pipeline_name}</span>
                {parentRun && (
                  <button type="button" className={`${ghostAction} px-3 py-1.5 text-xs`} onClick={() => onOpenRun(parentRun.run_id)}>
                    <ArrowRight className="h-4 w-4" aria-hidden="true" />
                    Parent: {parentRun.pipeline_name}
                  </button>
                )}
                {renderHeroStatus()}
                {run.pipeline_source && (
                  <span className="runner-pill runner-pill--muted capitalize bg-[var(--bg-secondary)] text-[var(--text-primary)] border-[var(--border-primary)] dark:bg-white/10 dark:text-[var(--text-primary)] dark:border-white/20">
                    {run.pipeline_source}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-3 text-sm text-[var(--text-secondary)]">
                <span className="inline-flex items-center gap-2 min-w-0">
                  <svg className="h-4 w-4 text-[var(--text-secondary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <circle cx="8" cy="7" r="2" />
                    <circle cx="8" cy="17" r="2" />
                    <circle cx="16" cy="7" r="2" />
                    <path d="M10 7h4" />
                    <path d="M8 9v6a4 4 0 004 4h4" />
                  </svg>
                  <span className="font-medium text-[var(--text-primary)] dark:text-[var(--text-primary)] truncate max-w-xs" title={repoLabel}>
                    {repoLabel}
                  </span>
                </span>
                <span className="text-[var(--border-primary)]">/</span>
                <span className="inline-flex items-center gap-2 min-w-0">
                  <BranchIcon className="h-4 w-4 text-[var(--text-secondary)]" />
                  <span className="font-mono text-[var(--text-primary)] dark:text-[var(--text-primary)] break-words" title={branchLabel || undefined}>
                    {branchLabel || '—'}
                  </span>
                </span>
              </div>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <div className="flex flex-wrap items-center justify-end gap-1.5">
                <button className={ghostAction} type="button" onClick={() => setAnalysisOpen(true)}>
                  <BrainCircuit className="h-4 w-4 text-current" aria-hidden="true" />
                  Analyse Run
                </button>
                <button className={ghostAction} type="button" onClick={onOpenLogs}>
                  <FileText className="h-4 w-4 text-current" aria-hidden="true" />
                  Logs
                </button>
                <Link className={ghostAction} to={monitoringLink}>
                  <BarChart3 className="h-4 w-4 text-current" aria-hidden="true" />
                  Usage
                </Link>
                <button
                  className={ghostAction}
                  type="button"
                  onClick={() => setMetadataOpen(open => !open)}
                  aria-expanded={metadataOpen}
                  aria-controls="pipeline-run-metadata"
                >
                  <Info className="h-4 w-4 text-current" aria-hidden="true" />
                  Info
                </button>
                {pipelineLink ? (
                  <Link className={ghostAction} to={pipelineLink}>
                    <Workflow className="h-4 w-4 text-current" aria-hidden="true" />
                    Pipeline
                  </Link>
                ) : (
                  <button className={ghostAction} type="button" onClick={onShowDefinition}>
                    <Workflow className="h-4 w-4 text-current" aria-hidden="true" />
                    Pipeline
                  </button>
                )}
              </div>
              <div className="hidden h-5 w-px bg-[var(--border-primary)] dark:bg-white/10 sm:block" />
              <button className={isActiveRun ? dangerAction : primaryAction} type="button" onClick={isActiveRun ? onCancel : onRerun} disabled={loading}>
                {isActiveRun ? (
                  <Square className="h-4 w-4 text-current" aria-hidden="true" />
                ) : (
                  <RefreshCw className="h-4 w-4 text-current" aria-hidden="true" />
                )}
                {isActiveRun ? 'Cancel' : 'Re-run'}
              </button>
              <button className={iconDanger} type="button" onClick={onDelete} aria-label="Delete run">
                <Trash2 className="h-4 w-4 text-current" aria-hidden="true" />
              </button>
              <button
                className="pipelines-icon-only"
                type="button"
                onClick={onClose}
                aria-label="Close details"
                title="Close"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </div>

          <div className="flex items-start gap-4 flex-wrap justify-between">
            <div className="flex-1 min-w-[320px] space-y-4">
              <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-4 text-sm text-[var(--text-primary)] mt-1">
                {detailLines.map(item => (
                  <div
                    key={item.label}
                    className="flex min-h-[76px] flex-col gap-1 rounded-xl border border-[var(--border-primary)] bg-white text-[var(--text-primary)] px-3 py-2 shadow-[0_8px_24px_rgba(0,0,0,0.07)] dark:bg-white/5 dark:border-white/10 dark:text-[var(--text-primary)] dark:shadow-[0_8px_24px_rgba(0,0,0,0.28)] h-full"
                  >
                    <div className="flex items-center justify-between text-[10px] uppercase tracking-wide text-[var(--text-secondary)]">
                      <span className="inline-flex items-center gap-2 font-semibold">
                        {item.icon}
                        {item.label}
                      </span>
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="font-mono text-[12px] leading-5 text-[var(--text-primary)] dark:text-[var(--text-primary)] break-words whitespace-pre-wrap">{item.value}</div>
                      {item.subtext && (
                        <div className="text-[11px] leading-4 text-[var(--text-secondary)] dark:text-slate-400 break-words whitespace-pre-wrap">{item.subtext}</div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {isExternalTriggerRun && (
                <div className="grid gap-3 md:grid-cols-2 text-sm text-[var(--text-primary)]">
                  <div className="rounded-xl border border-[var(--border-primary)] bg-white px-3 py-2 dark:bg-white/5 dark:border-white/10">
                    <div className="text-[11px] uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Triggered by</div>
                    <div className="mt-2 font-semibold text-[var(--text-primary)] dark:text-[var(--text-primary)]">External trigger</div>
                    <div className="mt-1 font-mono text-xs text-[var(--text-secondary)] break-words">{run.external_trigger_name || run.external_trigger_id || '—'}</div>
                  </div>
                  <div className="rounded-xl border border-[var(--border-primary)] bg-white px-3 py-2 dark:bg-white/5 dark:border-white/10">
                    <div className="text-[11px] uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Caller</div>
                    <div className="mt-2 font-mono text-sm text-[var(--text-primary)] dark:text-[var(--text-primary)] break-words">{externalCaller}</div>
                    <div className="mt-1 text-xs text-[var(--text-secondary)] break-words">
                      {run.external_trigger_event_type ? `Event: ${run.external_trigger_event_type}` : 'Event: —'}
                    </div>
                  </div>
                  <div className="rounded-xl border border-[var(--border-primary)] bg-white px-3 py-2 dark:bg-white/5 dark:border-white/10 md:col-span-2">
                    <div className="text-[11px] uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Idempotency key</div>
                    <div className="mt-2 font-mono text-sm text-[var(--text-primary)] dark:text-[var(--text-primary)] break-words">
                      {run.external_trigger_idempotency_key || '—'}
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {metadataOpen ? <RunMetadataPanel run={run} /> : null}

      {error && <div className="text-red-500 text-sm">{error}</div>}

      {run.failure_reason && (
        <div className="bg-red-50 dark:bg-red-900/40 border border-red-200 dark:border-red-700 text-red-700 dark:text-red-200 px-4 py-3 rounded-lg text-sm">
          <div className="font-semibold">Failed to start</div>
          <div className="mt-2 font-mono text-xs whitespace-pre-wrap break-words">{run.failure_reason}</div>
        </div>
      )}

      {ignoredFailureWarning && (
        <div
          role="status"
          aria-label="Ignored failures detected"
          className="rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 shadow-sm dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100"
        >
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-200" aria-hidden="true" />
            <div className="min-w-0">
              <div className="font-semibold">Ignored failures detected</div>
              <p className="mt-1 text-xs leading-5 text-amber-800 dark:text-amber-100/90">{ignoredFailureWarning.message}</p>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {ignoredFailureWarning.items.slice(0, 3).map((item, index) => (
                  <span key={`${item}-${index}`} className="runner-pill border-amber-300 bg-white/70 text-amber-900 dark:border-amber-400/40 dark:bg-amber-300/10 dark:text-amber-100">
                    {item}
                  </span>
                ))}
                {ignoredFailureWarning.items.length > 3 ? (
                  <span className="runner-pill border-amber-300 bg-white/70 text-amber-900 dark:border-amber-400/40 dark:bg-amber-300/10 dark:text-amber-100">
                    +{ignoredFailureWarning.items.length - 3} more
                  </span>
                ) : null}
              </div>
            </div>
          </div>
        </div>
      )}

      <RunDetailWorkspaceTabs
        runID={run.run_id}
        steps={detail.steps}
        selectedStep={selectedStep}
        onSelectStep={onSelectStep}
        onOpenStepLogs={onOpenStepLogs}
        onOpenTaskLogs={onOpenTaskLogs}
        onOpenStepDetail={onOpenStepDetail}
        childRuns={detail.child_runs}
        pipelineDefinition={detail.pipeline_definition}
        outputs={detail.final_outputs}
        onCancelOutput={onCancelOutput}
        onRetryOutput={onRetryOutput}
      />

      {approvals.length > 0 && (
        <div className="border border-[var(--border-primary)] rounded-lg bg-white dark:bg-slate-950 p-3 space-y-3 shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="font-semibold text-[var(--text-primary)]">Approvals</h3>
            <span className="text-xs text-[var(--text-secondary)]">
              {approvals.filter(approval => approval.status === 'pending').length} pending
            </span>
          </div>
          <div className="space-y-3">
            {approvals.map(approval => {
              const pending = approval.status === 'pending';
              const approveKey = `${approval.id}:approve`;
              const rejectKey = `${approval.id}:reject`;
              return (
                <div key={approval.id} className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 space-y-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <StatusBadge status={approval.status === 'rejected' ? 'rejected' : approval.status === 'approved' ? 'success' : 'waiting_approval'} complete={approval.status !== 'pending'} />
                        <span className="font-semibold text-[var(--text-primary)] break-words">{approval.step_name}</span>
                        <span className="runner-pill runner-pill--muted">{approval.approval_type}</span>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {approval.assigned_teams.map(team => (
                          <span key={team} className="runner-pill runner-pill--muted">
                            {team}
                          </span>
                        ))}
                      </div>
                      <div className="text-xs text-[var(--text-secondary)]">
                        Requested {timeAgo(approval.requested_at)}
                        {approval.decided_by_email ? ` · Decided by ${approval.decided_by_email}` : ''}
                      </div>
                    </div>
                    {pending && (
                      <div className="flex items-center gap-2">
                        <button
                          className={primaryAction}
                          type="button"
                          disabled={Boolean(approvalDecisionPending)}
                          onClick={() => onApprovalDecision(approval, 'approve')}
                        >
                          {approvalDecisionPending === approveKey ? 'Approving' : 'Approve'}
                        </button>
                        <button
                          className={dangerAction}
                          type="button"
                          disabled={Boolean(approvalDecisionPending)}
                          onClick={() => onApprovalDecision(approval, 'reject')}
                        >
                          {approvalDecisionPending === rejectKey ? 'Rejecting' : 'Reject'}
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {detail.child_runs?.length > 0 && (
        <div className="border border-[var(--border-primary)] rounded-lg bg-white dark:bg-slate-950 p-3 space-y-2 shadow-sm">
          <h3 className="font-semibold text-[var(--text-primary)]">Child runs</h3>
          <div className="space-y-3">
            {detail.child_runs.map(child => (
              <div key={child.run_id} className="flex items-center justify-between text-sm p-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
                <div className="flex items-center gap-2">
                  <StatusBadge status={child.status} complete={child.is_complete} />
                  <span className="font-medium text-[var(--text-primary)]">{child.pipeline_name}</span>
                  {child.parent_step_name && <span className="runner-pill runner-pill--muted">Step {child.parent_step_name}</span>}
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-[var(--text-secondary)]">{timeAgo(runActivityTimestamp(child))}</span>
                  <button className={ghostAction} type="button" onClick={() => onOpenRun(child.run_id)}>
                    Open
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      </div>
      {analysisOpen ? (
        <AnalysisModal
          result={analysisResult}
          loadAiPromptContext={loadAnalysisPromptContext}
          actions={[
            {
              id: 'logs',
              label: 'Open logs',
              icon: <FileText className="h-4 w-4" aria-hidden="true" />,
              onSelect: () => {
                setAnalysisOpen(false);
                onOpenLogs();
              },
            },
            pipelineLink ? {
              id: 'pipeline',
              label: 'Open pipeline',
              icon: <Workflow className="h-4 w-4" aria-hidden="true" />,
              to: pipelineLink,
            } : {
              id: 'pipeline-definition',
              label: 'Pipeline definition',
              icon: <Workflow className="h-4 w-4" aria-hidden="true" />,
              onSelect: () => {
                setAnalysisOpen(false);
                onShowDefinition();
              },
            },
            ...(analysisResult.comparison?.length ? [{
              id: 'compare',
              label: 'Compare with last success',
              icon: <GitCompare className="h-4 w-4" aria-hidden="true" />,
              onSelect: () => document.getElementById('analysis-comparison')?.scrollIntoView({ block: 'start', behavior: 'smooth' }),
            }] : []),
          ]}
          onClose={() => setAnalysisOpen(false)}
        />
      ) : null}
    </>
  );
}

function RunMetadataPanel({ run }: { run: RunListItem }) {
  const runtimeOverrides = Object.entries(run.runtime_variable_overrides || {}).sort(([a], [b]) => a.localeCompare(b));
  const runItems = [
    { label: 'Pipeline ID', value: pipelineID(run) },
    { label: 'Pipeline path', value: run.pipeline_path },
    { label: 'Pipeline name', value: run.pipeline_name },
    { label: 'Version', value: run.pipeline_version },
    { label: 'Pipeline source', value: run.pipeline_source },
    { label: 'Status', value: run.status },
    { label: 'Scope', value: run.scope || 'Default scope' },
    { label: 'Team ID', value: run.team_id ? String(run.team_id) : '' },
    { label: 'Run ID', value: run.run_id },
    { label: 'Parent run ID', value: run.parent_run_id || '' },
    { label: 'Created', value: formatRunTimestamp(run.created_at) },
    { label: 'Started', value: formatRunTimestamp(run.started_at) },
    { label: 'Finished', value: formatRunTimestamp(run.finished_at) },
    { label: 'Timeout', value: formatRunTimestamp(run.timeout_at) },
    { label: 'Duration', value: run.duration },
    { label: 'Failure reason', value: run.failure_reason, wide: true },
  ];
  const triggerItems = [
    { label: 'Trigger source', value: run.trigger_source },
    { label: 'Trigger event ID', value: run.trigger_event_id },
    { label: 'Requested by', value: subjectLabel(run.requested_by_type, run.requested_by_id) },
    { label: 'Effective subject', value: subjectLabel(run.effective_subject_type, run.effective_subject_id) },
    { label: 'Schedule', value: scheduleLabel(run) },
    { label: 'External trigger', value: externalTriggerLabel(run) },
    { label: 'External caller', value: subjectLabel(run.external_trigger_caller_type, run.external_trigger_caller_id) },
    { label: 'External event type', value: run.external_trigger_event_type },
    { label: 'Idempotency key', value: run.external_trigger_idempotency_key, wide: true },
  ];
  const gitItems = [
    { label: 'Repository', value: formatRepoLabel(run) },
    { label: 'Clone URL', value: run.git_clone_url, wide: true },
    { label: 'SSH URL', value: run.git_ssh_url, wide: true },
    { label: 'Ref', value: run.git_ref },
    { label: 'Target ref', value: run.git_target_ref },
    { label: 'Commit SHA', value: run.git_commit_sha },
    { label: 'Commit URL', value: run.git_commit_url, wide: true },
    { label: 'Commit author', value: subjectLabel(run.git_commit_author_name, run.git_commit_author_email) },
    { label: 'Commit author username', value: run.git_commit_author_username },
    { label: 'Pusher', value: subjectLabel(run.git_pusher_name, run.git_pusher_email) },
    { label: 'Check run ID', value: run.git_check_run_id ? String(run.git_check_run_id) : '' },
    { label: 'Commit message', value: run.git_commit_message, wide: true },
  ];

  return (
    <section
      id="pipeline-run-metadata"
      className="rounded-lg border border-[var(--border-primary)] bg-white p-3 shadow-sm dark:border-white/10 dark:bg-slate-950"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="font-semibold text-[var(--text-primary)]">Run Metadata</h3>
        <span className="text-xs text-[var(--text-secondary)]">{run.run_id}</span>
      </div>
      <div className="mt-4 grid gap-4 xl:grid-cols-3">
        <MetadataGroup title="Run" items={runItems} />
        <MetadataGroup title="Trigger" items={triggerItems} />
        <MetadataGroup title="Git" items={gitItems} />
      </div>
      <div className="mt-4 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 dark:border-white/10">
        <div className="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Runtime variable overrides</div>
        {runtimeOverrides.length > 0 ? (
          <div className="mt-2 grid gap-2 md:grid-cols-2">
            {runtimeOverrides.map(([key, value]) => (
              <MetadataValue key={key} label={key} value={metadataValue(value)} />
            ))}
          </div>
        ) : (
          <div className="mt-2 text-xs text-[var(--text-secondary)]">No overrides recorded</div>
        )}
      </div>
    </section>
  );
}

function MetadataGroup({ title, items }: { title: string; items: Array<{ label: string; value?: string | null; wide?: boolean }> }) {
  return (
    <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 dark:border-white/10">
      <div className="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{title}</div>
      <div className="mt-3 grid gap-2">
        {items.map(item => (
          <MetadataValue key={item.label} label={item.label} value={item.value} wide={item.wide} />
        ))}
      </div>
    </div>
  );
}

function MetadataValue({ label, value, wide }: { label: string; value?: string | null; wide?: boolean }) {
  return (
    <div className={`min-w-0 rounded-lg bg-white px-3 py-2 dark:bg-black/20 ${wide ? 'md:col-span-2' : ''}`}>
      <div className="text-[10px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{label}</div>
      <div className="mt-1 break-words font-mono text-xs text-[var(--text-primary)]">{metadataValue(value)}</div>
    </div>
  );
}

function metadataValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function pipelineID(run: RunListItem) {
  const path = (run.pipeline_path || '').replace(/^\/+|\/+$/g, '');
  const name = (run.pipeline_name || '').trim();
  if (path && name) return `${path}/${name}`;
  return name || path;
}

function subjectLabel(type?: string, id?: string) {
  const normalizedType = (type || '').trim();
  const normalizedID = (id || '').trim();
  if (normalizedType && normalizedID) return `${normalizedType}:${normalizedID}`;
  return normalizedID || normalizedType;
}

function scheduleLabel(run: RunListItem) {
  return [run.schedule_name || run.schedule_path || '', run.schedule_id || ''].filter(Boolean).join(' / ');
}

function externalTriggerLabel(run: RunListItem) {
  return [run.external_trigger_name || '', run.external_trigger_id || ''].filter(Boolean).join(' / ');
}
