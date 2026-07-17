import { Link } from 'react-router-dom';
import { ArrowRight, BarChart3, FileText, RefreshCw, Square, Trash2, Workflow, X } from 'lucide-react';
import type { PipelineDefinition, PipelineRunFinalOutput, RunListItem, StepDetail } from './contracts';
import { BranchIcon, CommitIcon, RunIdIcon, StatusBadge, ZapIcon } from './PipelineRunCards';
import { RunFinalOutputs } from './RunFinalOutputs';
import { StepsGraph } from './RunGraph';
import {
  buildPipelineLink,
  buildRunMonitoringLink,
  formatAIUsageBreakdown,
  formatBranchDisplay,
  formatRepoLabel,
  formatTokenCount,
  formatTriggerId,
  runActivityTimestamp,
  runStartedTimestamp,
  timeAgo,
  type ParentRunInfo,
} from './runPresentation';
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
  onRerun,
  onDelete,
  selectedStep,
  onSelectStep,
  onOpenLogs,
  onOpenTaskLogs,
  onOpenStepDetail,
  onOpenRun,
  onShowDefinition,
  onApprovalDecision,
  approvalDecisionPending,
}: {
  detail: RunDetail;
  loading: boolean;
  error: string | null;
  onClose: () => void;
  onCancel: () => void;
  onCancelOutput: (outputId: string) => void;
  onRerun: () => void;
  onDelete: () => void;
  selectedStep: string | null;
  onSelectStep: (step: string | null) => void;
  onOpenLogs: () => void;
  onOpenTaskLogs: (stepName: string, taskName: string) => void;
  onOpenStepDetail: (stepName: string) => void;
  onOpenRun: (id: string) => void;
  onShowDefinition: () => void;
  onApprovalDecision: (approval: PipelineApproval, decision: 'approve' | 'reject') => void;
  approvalDecisionPending: string | null;
}) {
  const run = detail.run_info;
  const normalizedStatus = normalizeStatus(run.status, run.is_complete);
  const isActiveRun = normalizedStatus === 'running' || normalizedStatus === 'pending' || normalizedStatus === 'waiting_approval';
  const approvals = detail.approvals || [];
  const pipelineLink = buildPipelineLink(run);
  const monitoringLink = buildRunMonitoringLink(run);
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const parentRun = detail.parent_run_info;

  const actionBase =
    'inline-flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-semibold transition duration-150 focus:outline-none';
  const ghostAction = `${actionBase} border border-[var(--border-primary)]/80 bg-[var(--bg-secondary)] text-[var(--text-primary)] shadow-[0_10px_30px_rgba(0,0,0,0.08)] hover:border-indigo-300/60 hover:text-indigo-600 dark:border-white/10 dark:bg-white/5 dark:text-[var(--text-primary)] dark:shadow-[0_10px_30px_rgba(0,0,0,0.25)] dark:hover:border-indigo-300/50 dark:hover:bg-white/10`;
  const primaryAction = `${actionBase} bg-gradient-to-r from-indigo-500 to-purple-500 text-[var(--text-button)] shadow-[0_14px_34px_rgba(79,70,229,0.25)] hover:shadow-[0_18px_44px_rgba(79,70,229,0.32)] focus:ring-2 focus:ring-offset-2 focus:ring-indigo-400`;
  const dangerAction = `${actionBase} border border-red-500/40 text-red-600 bg-red-50 hover:bg-red-100 dark:text-red-100 dark:bg-red-500/10 dark:hover:bg-red-500/20`;
  const iconDanger = 'inline-flex items-center justify-center h-11 w-11 rounded-xl p-0 text-red-600 hover:text-red-700 dark:text-red-200 dark:hover:text-red-100 bg-transparent border-none shadow-none';

  const startedAt = runStartedTimestamp(run);
  const startedLabel = timeAgo(startedAt);
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const repoLabel = formatRepoLabel(run);
  const isExternalTriggerRun = run.trigger_source === 'external_trigger' || Boolean(run.external_trigger_id);
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
    <div className="space-y-6">
      <div className="rounded-3xl border border-[var(--border-primary)] bg-white text-[var(--text-primary)] shadow-[0_22px_60px_rgba(8,10,24,0.12)] dark:border-white/10 dark:bg-gradient-to-br from-[#0b0c15] via-[#0c0f1f] to-[#0b0c15] dark:text-[var(--text-primary)] dark:shadow-[0_22px_60px_rgba(8,10,24,0.5)] overflow-hidden">
        <div className="p-6 flex flex-col gap-6">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-3 flex-wrap">
                <span className="text-3xl font-black tracking-tight text-[var(--text-primary)] dark:text-[var(--text-primary)]">{run.pipeline_name}</span>
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
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-2">
                <button className={ghostAction} type="button" onClick={onOpenLogs}>
                  <FileText className="h-4 w-4 text-current" aria-hidden="true" />
                  Logs
                </button>
                <Link className={ghostAction} to={monitoringLink}>
                  <BarChart3 className="h-4 w-4 text-current" aria-hidden="true" />
                  Usage
                </Link>
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
              <div className="h-6 w-px bg-[var(--border-primary)] dark:bg-white/10" />
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

          <div className="flex items-start gap-6 flex-wrap justify-between">
            <div className="flex-1 min-w-[320px] space-y-6">
              <div className="grid gap-3 md:grid-cols-4 text-sm text-[var(--text-primary)] mt-4">
                {detailLines.map(item => (
                  <div
                    key={item.label}
                    className="flex flex-col gap-2 rounded-2xl border border-[var(--border-primary)] bg-white text-[var(--text-primary)] px-4 py-3 shadow-[0_12px_32px_rgba(0,0,0,0.08)] dark:bg-white/5 dark:border-white/10 dark:text-[var(--text-primary)] dark:shadow-[0_12px_32px_rgba(0,0,0,0.35)] h-full"
                  >
                    <div className="flex items-center justify-between text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
                      <span className="inline-flex items-center gap-2 font-semibold">
                        {item.icon}
                        {item.label}
                      </span>
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="font-mono text-sm text-[var(--text-primary)] dark:text-[var(--text-primary)] break-words whitespace-pre-wrap">{item.value}</div>
                      {item.subtext && (
                        <div className="text-xs text-[var(--text-secondary)] dark:text-slate-400 break-words whitespace-pre-wrap">{item.subtext}</div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {isExternalTriggerRun && (
                <div className="grid gap-3 md:grid-cols-2 text-sm text-[var(--text-primary)]">
                  <div className="rounded-2xl border border-[var(--border-primary)] bg-white px-4 py-3 dark:bg-white/5 dark:border-white/10">
                    <div className="text-[11px] uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Triggered by</div>
                    <div className="mt-2 font-semibold text-[var(--text-primary)] dark:text-[var(--text-primary)]">External trigger</div>
                    <div className="mt-1 font-mono text-xs text-[var(--text-secondary)] break-words">{run.external_trigger_name || run.external_trigger_id || '—'}</div>
                  </div>
                  <div className="rounded-2xl border border-[var(--border-primary)] bg-white px-4 py-3 dark:bg-white/5 dark:border-white/10">
                    <div className="text-[11px] uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Caller</div>
                    <div className="mt-2 font-mono text-sm text-[var(--text-primary)] dark:text-[var(--text-primary)] break-words">{externalCaller}</div>
                    <div className="mt-1 text-xs text-[var(--text-secondary)] break-words">
                      {run.external_trigger_event_type ? `Event: ${run.external_trigger_event_type}` : 'Event: —'}
                    </div>
                  </div>
                  <div className="rounded-2xl border border-[var(--border-primary)] bg-white px-4 py-3 dark:bg-white/5 dark:border-white/10 md:col-span-2">
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

      {error && <div className="text-red-500 text-sm">{error}</div>}

      {run.failure_reason && (
        <div className="bg-red-50 dark:bg-red-900/40 border border-red-200 dark:border-red-700 text-red-700 dark:text-red-200 px-4 py-3 rounded-lg text-sm">
          <div className="font-semibold">Failed to start</div>
          <div className="mt-2 font-mono text-xs whitespace-pre-wrap break-words">{run.failure_reason}</div>
        </div>
      )}

      <RunFinalOutputs runID={run.run_id} outputs={detail.final_outputs} onCancelOutput={onCancelOutput} />

      {approvals.length > 0 && (
        <div className="border border-[var(--border-primary)] rounded-2xl bg-white dark:bg-slate-950 p-4 space-y-3 shadow-sm">
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
                <div key={approval.id} className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm">
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

      <div className="space-y-4">
        <div className="rounded-2xl border border-[var(--border-primary)] bg-white dark:bg-slate-950 shadow-[0_16px_44px_rgba(15,23,42,0.07)] p-2">
          <StepsGraph
            steps={detail.steps}
            selectedStep={selectedStep}
            onSelectStep={onSelectStep}
            onOpenTaskLogs={onOpenTaskLogs}
            onOpenStepDetail={onOpenStepDetail}
            childRuns={detail.child_runs}
            pipelineDefinition={detail.pipeline_definition}
          />
        </div>
      </div>

      {detail.child_runs?.length > 0 && (
        <div className="border border-[var(--border-primary)] rounded-2xl bg-white dark:bg-slate-950 p-4 space-y-2 shadow-sm">
          <h3 className="font-semibold text-[var(--text-primary)]">Child runs</h3>
          <div className="space-y-3">
            {detail.child_runs.map(child => (
              <div key={child.run_id} className="flex items-center justify-between text-sm p-3 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
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
  );
}
