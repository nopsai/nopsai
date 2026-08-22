import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  ClipboardCopy,
  ExternalLink,
  Info,
  Loader2,
  MessageSquare,
  ShieldCheck,
  Sparkles,
  X,
} from 'lucide-react';
import { copyTextToClipboard } from '../../lib/clipboard.js';
import {
  analysisCategoryLabel,
  filterFindingsForTab,
  type AnalysisFinding,
  type AnalysisResult,
  type AnalysisScore,
  type AnalysisSeverity,
} from './model.js';
import {
  analysisAssistantChatPrompt,
  analysisAssistantPageContext,
  type AnalysisAiPromptContext,
} from './ai.js';
import { useAnalysisAiEvaluation, type AnalysisAiEvaluationState } from './useAnalysisAiEvaluation.js';
import {
  buildAnalysisScoreView,
  formatAnalysisReportWithScoreView,
  type AnalysisScoreView,
} from './reviewedScore.js';

export type AnalysisAction = {
  id: string;
  label: string;
  icon?: ReactNode;
  onSelect?: () => void;
  to?: string;
};

type AnalysisWorkspaceProps = {
  result: AnalysisResult;
  controls?: ReactNode;
  actions?: AnalysisAction[];
  loadAiPromptContext?: () => Promise<AnalysisAiPromptContext | null | undefined>;
  autoRequestKey?: number | string;
};

type AnalysisWorkspaceState = ReturnType<typeof useAnalysisWorkspaceState>;

export function AnalysisWorkspace({
  result,
  controls,
  actions = [],
  loadAiPromptContext,
  autoRequestKey,
}: AnalysisWorkspaceProps) {
  const workspace = useAnalysisWorkspaceState({ result, loadAiPromptContext, autoRequestKey });

  return (
    <section className="space-y-5" aria-labelledby="analysis-workspace-title">
      <header className="flex flex-wrap items-start justify-between gap-4 rounded-lg border border-[var(--border-primary)] bg-white p-4 shadow-sm dark:border-white/10 dark:bg-slate-900">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2 text-xs font-semibold uppercase text-[var(--text-secondary)]">
            <ShieldCheck className="h-4 w-4" aria-hidden="true" />
            <span>Read-only analysis</span>
            <span className="font-mono normal-case">{result.snapshotRevision}</span>
          </div>
          <h3 id="analysis-workspace-title" className="mt-2 text-xl font-bold text-[var(--text-primary)]">
            {result.title}
          </h3>
          <p className="mt-1 max-w-3xl text-sm text-[var(--text-secondary)]">
            {result.subjectLabel} · {result.summary}
          </p>
        </div>
        <button type="button" className="glass-button-ghost" onClick={() => void workspace.copyReport()}>
          <ClipboardCopy className="h-4 w-4" aria-hidden="true" />
          {workspace.copyState === 'copied' ? 'Copied' : workspace.copyState === 'error' ? 'Copy failed' : 'Copy'}
        </button>
      </header>
      <AnalysisWorkspaceContent
        result={result}
        controls={controls}
        actions={actions}
        workspace={workspace}
      />
    </section>
  );
}

export function AnalysisModal({
  result,
  controls,
  actions = [],
  loadAiPromptContext,
  onClose,
}: {
  result: AnalysisResult;
  controls?: ReactNode;
  actions?: AnalysisAction[];
  loadAiPromptContext?: () => Promise<AnalysisAiPromptContext | null | undefined>;
  onClose: () => void;
}) {
  const workspace = useAnalysisWorkspaceState({ result, loadAiPromptContext });
  const navigate = useNavigate();
  return (
    <div className="fixed inset-0 z-[90] bg-slate-950/55 p-4 backdrop-blur-sm" role="presentation">
      <section
        className="mx-auto flex h-full max-h-[calc(100vh-2rem)] w-full max-w-5xl flex-col overflow-hidden rounded-lg border border-[var(--border-primary)] bg-white text-[var(--text-primary)] shadow-xl dark:border-white/10 dark:bg-slate-950"
        role="dialog"
        aria-modal="true"
        aria-labelledby="analysis-modal-title"
      >
        <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3 dark:border-white/10">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-xs font-semibold uppercase text-[var(--text-secondary)]">
              <ShieldCheck className="h-4 w-4" aria-hidden="true" />
              <span>Read-only analysis</span>
              <span className="font-mono normal-case">{result.snapshotRevision}</span>
            </div>
            <h2 id="analysis-modal-title" className="mt-2 text-xl font-bold text-[var(--text-primary)]">
              {result.title}
            </h2>
            <p className="mt-1 max-w-3xl text-sm text-[var(--text-secondary)]">
              {result.subjectLabel} · {result.summary}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <button
              type="button"
              className="glass-button-ghost"
              onClick={() => {
                onClose();
                navigate('/assistant', {
                  state: {
                    assistantPageContext: analysisAssistantPageContext(result),
                    assistantStartFresh: true,
                    assistantDraft: analysisAssistantChatPrompt(result),
                  },
                });
              }}
            >
              <MessageSquare className="h-4 w-4" aria-hidden="true" />
              Ask NopsAI
            </button>
            <button type="button" className="glass-button-ghost" onClick={() => void workspace.copyReport()}>
              <ClipboardCopy className="h-4 w-4" aria-hidden="true" />
              {workspace.copyState === 'copied' ? 'Copied' : workspace.copyState === 'error' ? 'Copy failed' : 'Copy'}
            </button>
            <button type="button" className="pipelines-icon-only" onClick={onClose} aria-label="Close analysis">
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto p-5">
          <AnalysisWorkspaceContent
            result={result}
            controls={controls}
            actions={actions}
            workspace={workspace}
          />
        </div>
      </section>
    </div>
  );
}

function useAnalysisWorkspaceState({
  result,
  loadAiPromptContext,
  autoRequestKey,
}: Pick<AnalysisWorkspaceProps, 'result' | 'loadAiPromptContext' | 'autoRequestKey'>) {
  const defaultTab = result.tabs[0]?.id || 'overview';
  const [tabState, setTabState] = useState({ snapshotRevision: result.snapshotRevision, activeTab: defaultTab });
  const [dismissedState, setDismissedState] = useState<{ snapshotRevision: string; ids: Set<string> }>({
    snapshotRevision: result.snapshotRevision,
    ids: new Set(),
  });
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'error'>('idle');
  const autoRequestRef = useRef<number | string | null>(null);
  const aiEvaluation = useAnalysisAiEvaluation(result, { loadPromptContext: loadAiPromptContext });
  const requestAiEvaluation = aiEvaluation.requestEvaluation;
  const aiEvaluationStatus = aiEvaluation.state.status;
  const readyAiEvaluation = aiEvaluation.state.status === 'ready' ? aiEvaluation.state.evaluation : null;
  const scoreView = useMemo(
    () => buildAnalysisScoreView(result, readyAiEvaluation),
    [readyAiEvaluation, result]
  );
  const activeTab = tabState.snapshotRevision === result.snapshotRevision &&
    result.tabs.some(tab => tab.id === tabState.activeTab)
    ? tabState.activeTab
    : defaultTab;
  const dismissedFindings = useMemo(
    () => dismissedState.snapshotRevision === result.snapshotRevision ? dismissedState.ids : new Set<string>(),
    [dismissedState.ids, dismissedState.snapshotRevision, result.snapshotRevision]
  );
  const visibleFindings = useMemo(
    () => filterFindingsForTab(result.findings, activeTab).filter(finding => !dismissedFindings.has(finding.id)),
    [activeTab, dismissedFindings, result.findings]
  );
  const activeTabLabel = result.tabs.find(tab => tab.id === activeTab)?.label || 'Overview';
  const primaryFinding = result.findings[0] || null;

  const copyReport = async () => {
    try {
      await copyTextToClipboard(formatAnalysisReportWithScoreView(result, scoreView, readyAiEvaluation));
      setCopyState('copied');
      window.setTimeout(() => setCopyState('idle'), 1600);
    } catch {
      setCopyState('error');
    }
  };

  useEffect(() => {
    if (autoRequestKey == null) return;
    if (autoRequestRef.current === autoRequestKey) return;
    autoRequestRef.current = autoRequestKey;
    if (aiEvaluationStatus === 'loading') return;
    void requestAiEvaluation();
  }, [aiEvaluationStatus, autoRequestKey, requestAiEvaluation]);

  return {
    activeTab,
    activeTabLabel,
    aiEvaluation,
    copyReport,
    copyState,
    dismissedFindings,
    primaryFinding,
    scoreView,
    setDismissedState,
    setTabState,
    visibleFindings,
  };
}

function AnalysisWorkspaceContent({
  result,
  controls,
  actions,
  workspace,
}: {
  result: AnalysisResult;
  controls?: ReactNode;
  actions: AnalysisAction[];
  workspace: AnalysisWorkspaceState;
}) {
  return (
          <div className="grid gap-5 lg:grid-cols-[18rem_minmax(0,1fr)]">
            <aside className="space-y-4 lg:sticky lg:top-0 lg:self-start">
              <HealthScorePanel result={result} scoreView={workspace.scoreView} />
              <MetricScoresPanel scores={workspace.scoreView.scores} source={workspace.scoreView.source} />
              <FindingSummaryPanel counts={workspace.scoreView.counts} source={workspace.scoreView.source} />
              <SafeguardsPanel safeguards={result.safeguards} />
            </aside>

            <div className="min-w-0 space-y-5">
              <PrimaryAssessment result={result} finding={workspace.primaryFinding} scoreView={workspace.scoreView} actions={actions} />

              {controls ? (
                <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 dark:border-white/10 dark:bg-white/5">
                  {controls}
                </section>
              ) : null}

              <AiEvaluationPanel
                state={workspace.aiEvaluation.state}
                autoEvaluates={workspace.aiEvaluation.autoEvaluates}
                historyCount={workspace.aiEvaluation.history.length}
                onRequest={() => void workspace.aiEvaluation.requestEvaluation()}
              />

              {result.comparison?.length ? (
                <section
                  id="analysis-comparison"
                  className="rounded-lg border border-[var(--border-primary)] bg-white p-4 shadow-sm dark:border-white/10 dark:bg-slate-900"
                >
                  <h3 className="text-sm font-semibold text-[var(--text-primary)]">Changed Since Last Success</h3>
                  <div className="mt-3 grid gap-2 md:grid-cols-2">
                    {result.comparison.map(item => (
                      <div key={item.label} className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm dark:border-white/10 dark:bg-white/5">
                        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">{item.label}</div>
                        <div className="mt-1 break-words font-mono text-xs text-[var(--text-primary)]">
                          {item.before} {'->'} {item.after}
                        </div>
                        <div className={`mt-2 text-xs font-semibold ${item.changed ? 'text-amber-700 dark:text-amber-200' : 'text-emerald-700 dark:text-emerald-200'}`}>
                          {item.changed ? 'Changed' : 'Unchanged'}
                        </div>
                      </div>
                    ))}
                  </div>
                </section>
              ) : null}

              <section className="space-y-3">
                <nav className="pipeline-runs-segmented" aria-label="Analysis categories">
                  {result.tabs.map(tab => {
                    const count = filterFindingsForTab(result.findings, tab.id).length;
                    return (
                      <button
                        key={tab.id}
                        type="button"
                        className={`pipeline-runs-segment ${workspace.activeTab === tab.id ? 'pipeline-runs-segment--active' : ''}`}
                        aria-pressed={workspace.activeTab === tab.id}
                        onClick={() => workspace.setTabState({ snapshotRevision: result.snapshotRevision, activeTab: tab.id })}
                      >
                        {tab.label}
                        <span className="ml-1 font-mono text-[10px]">{count}</span>
                      </button>
                    );
                  })}
                </nav>

                <div className="space-y-3" aria-label={`${workspace.activeTabLabel} findings`}>
                  {workspace.visibleFindings.length === 0 ? (
                    <div className="rounded-lg border border-[var(--border-primary)] bg-white p-5 text-sm text-[var(--text-secondary)] shadow-sm dark:border-white/10 dark:bg-slate-900">
                      No findings in {workspace.activeTabLabel.toLowerCase()} for this snapshot.
                    </div>
                  ) : (
                    workspace.visibleFindings.map(finding => (
                      <FindingCard
                        key={finding.id}
                        finding={finding}
                        onDismiss={() => {
                          workspace.setDismissedState(current => {
                            const currentIds = current.snapshotRevision === result.snapshotRevision ? current.ids : new Set<string>();
                            const next = new Set(currentIds);
                            next.add(finding.id);
                            return { snapshotRevision: result.snapshotRevision, ids: next };
                          });
                        }}
                      />
                    ))
                  )}
                </div>
              </section>
            </div>
          </div>
  );
}

function HealthScorePanel({ result, scoreView }: { result: AnalysisResult; scoreView: AnalysisScoreView }) {
  const reviewed = scoreView.source === 'ai-reviewed';
  const scoreText = scoreView.scoreBasis.findingCount === 0
    ? 'No weighted findings were detected.'
    : `${scoreView.scoreBasis.findingCount} weighted ${scoreView.scoreBasis.findingCount === 1 ? 'finding' : 'findings'} subtract ${scoreView.scoreBasis.totalDeduction} points.`;
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 dark:border-white/10 dark:bg-white/5">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Overall Health</div>
        {reviewed ? (
          <span className="runner-pill runner-pill--muted">
            {scoreView.snapshotMatches ? 'AI-reviewed' : 'previous review'}
          </span>
        ) : null}
      </div>
      <div className="mt-2 flex items-end gap-2">
        <span className="text-5xl font-black text-[var(--text-primary)]">{scoreView.healthScore}</span>
        <span className="pb-1 text-sm font-semibold text-[var(--text-secondary)]">/100</span>
      </div>
      {reviewed ? (
        <div className="mt-1 text-xs font-semibold text-[var(--text-secondary)]">
          Deterministic baseline: {result.healthScore}/100
        </div>
      ) : null}
      <p className="mt-3 text-sm text-[var(--text-secondary)]">{scoreText}</p>
      <details className="mt-3 rounded-md border border-[var(--border-primary)] bg-white p-3 text-xs dark:border-white/10 dark:bg-black/20">
        <summary className="cursor-pointer font-semibold text-[var(--text-primary)]">Where the score comes from</summary>
        <div className="mt-2 space-y-2 text-[var(--text-secondary)]">
          <p>{scoreView.scoreBasis.formula}</p>
          <p>Inputs: {scoreView.scoreBasis.inputs.join('; ')}.</p>
          {scoreView.reviewedAt ? <p>Reviewed: {formatTimestamp(scoreView.reviewedAt)}</p> : null}
          {!scoreView.snapshotMatches && scoreView.cachedSnapshotRevision ? (
            <p>Review snapshot: {scoreView.cachedSnapshotRevision}; current snapshot: {scoreView.currentSnapshotRevision}.</p>
          ) : null}
          {scoreView.profileName ? <p>Profile: {scoreView.profileName}{scoreView.modelLabel ? ` / ${scoreView.modelLabel}` : ''}</p> : null}
          {scoreView.scoreBasis.limitations.slice(0, 2).map(limitation => (
            <p key={limitation}>{limitation}</p>
          ))}
        </div>
      </details>
    </section>
  );
}

function FindingSummaryPanel({ counts, source }: { counts: Record<AnalysisSeverity, number>; source: AnalysisScoreView['source'] }) {
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 dark:border-white/10 dark:bg-white/5">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Findings</div>
        {source === 'ai-reviewed' ? <span className="runner-pill runner-pill--muted">reviewed</span> : null}
      </div>
      <div className="mt-3 grid grid-cols-2 gap-2">
        <SeverityCount label="Critical" severity="critical" value={counts.critical} />
        <SeverityCount label="High" severity="high" value={counts.high} />
        <SeverityCount label="Medium" severity="medium" value={counts.medium} />
        <SeverityCount label="Low" severity="low" value={counts.low} />
      </div>
      {counts.opportunity > 0 ? (
        <div className="mt-2 rounded-md border border-[var(--border-primary)] bg-white px-3 py-2 text-sm dark:border-white/10 dark:bg-black/20">
          <div className="font-bold text-emerald-700 dark:text-emerald-200">{counts.opportunity}</div>
          <div className="text-[11px] font-semibold uppercase text-[var(--text-secondary)]">Opportunities</div>
        </div>
      ) : null}
    </section>
  );
}

function MetricScoresPanel({ scores, source }: { scores: AnalysisScore[]; source: AnalysisScoreView['source'] }) {
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 dark:border-white/10 dark:bg-white/5">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Metric Scores</div>
        <Info className="h-3.5 w-3.5 text-[var(--text-secondary)]" aria-hidden="true" />
      </div>
      <p className="mt-1 text-xs text-[var(--text-secondary)]">
        Hover a metric for its {source === 'ai-reviewed' ? 'AI-reviewed' : 'deterministic'} score basis.
      </p>
      <div className="mt-3 space-y-3">
        {scores.map(score => (
          <MetricScore key={score.category} score={score} />
        ))}
      </div>
    </section>
  );
}

function SafeguardsPanel({ safeguards }: { safeguards: string[] }) {
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 dark:border-white/10 dark:bg-white/5">
      <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Safeguards</div>
      <ul className="mt-3 space-y-2 text-xs text-[var(--text-secondary)]">
        {safeguards.map(safeguard => (
          <li key={safeguard} className="flex gap-2">
            <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-600" aria-hidden="true" />
            <span>{safeguard}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function PrimaryAssessment({
  result,
  finding,
  scoreView,
  actions,
}: {
  result: AnalysisResult;
  finding: AnalysisFinding | null;
  scoreView: AnalysisScoreView;
  actions: AnalysisAction[];
}) {
  const reviewedFinding = scoreView.reviewedFindings[0] || null;
  const displayedSeverity = reviewedFinding?.severity || finding?.severity || null;
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-white p-4 shadow-sm dark:border-white/10 dark:bg-slate-900">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-[var(--text-primary)]">Primary Assessment</h3>
          {reviewedFinding ? (
            <p className="mt-2 text-lg font-bold text-[var(--text-primary)]">
              {reviewedFinding.title}
              <span className="ml-2 align-middle text-xs font-semibold text-[var(--text-secondary)]">
                {reviewedFinding.confidence}% AI confidence
              </span>
            </p>
          ) : result.primaryDiagnosis ? (
            <p className="mt-2 text-lg font-bold text-[var(--text-primary)]">
              {result.primaryDiagnosis.domain}
              <span className="ml-2 align-middle text-xs font-semibold text-[var(--text-secondary)]">
                {result.primaryDiagnosis.confidence}% confidence
              </span>
            </p>
          ) : finding ? (
            <p className="mt-2 text-lg font-bold text-[var(--text-primary)]">{finding.title}</p>
          ) : (
            <p className="mt-2 text-lg font-bold text-[var(--text-primary)]">No blocking finding detected</p>
          )}
          <p className="mt-2 max-w-2xl text-sm text-[var(--text-secondary)]">
            {reviewedFinding?.basis || finding?.summary || 'The visible snapshot does not contain weighted risks beyond the current safeguards.'}
          </p>
        </div>
        {displayedSeverity ? (
          <span className={`inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-bold uppercase ${severityPillClass(displayedSeverity)}`}>
            {displayedSeverity === 'opportunity' ? <Sparkles className="h-3.5 w-3.5" aria-hidden="true" /> : <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />}
            {displayedSeverity}
          </span>
        ) : (
          <CheckCircle2 className="h-5 w-5 text-emerald-600" aria-hidden="true" />
        )}
      </div>
      {finding?.recommendations[0] ? (
        <div className="mt-4 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 dark:border-white/10 dark:bg-white/5">
          <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">First suggested fix</div>
          <div className="mt-1 text-sm font-semibold text-[var(--text-primary)]">{finding.recommendations[0].title}</div>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">{finding.recommendations[0].detail}</p>
        </div>
      ) : null}
      {actions.length > 0 ? (
        <div className="mt-4 flex flex-wrap items-center gap-2">
          {actions.map(action => (
            <AnalysisActionButton key={action.id} action={action} />
          ))}
        </div>
      ) : null}
    </section>
  );
}

function AiEvaluationPanel({
  state,
  autoEvaluates,
  historyCount,
  onRequest,
}: {
  state: AnalysisAiEvaluationState;
  autoEvaluates: boolean;
  historyCount: number;
  onRequest: () => void;
}) {
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-white p-4 shadow-sm dark:border-white/10 dark:bg-slate-900">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Bot className="h-4 w-4 text-indigo-600 dark:text-indigo-300" aria-hidden="true" />
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">AI Evaluation</h3>
          </div>
          <p className="mt-1 text-xs text-[var(--text-secondary)]">
            Uses redacted evidence to refine the health score, explain scored findings, and produce safe suggestions.
          </p>
        </div>
        {state.status !== 'loading' ? (
          <button type="button" className="glass-button-ghost" onClick={onRequest}>
            <Sparkles className="h-4 w-4" aria-hidden="true" />
            {state.status === 'ready' ? 'Regenerate' : autoEvaluates ? 'Retry AI' : 'Generate AI'}
          </button>
        ) : null}
      </div>

      {state.status === 'loading' ? (
        <div className="mt-4 flex items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm text-[var(--text-secondary)] dark:border-white/10 dark:bg-white/5">
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          AI is reviewing the redacted evidence.
        </div>
      ) : state.status === 'ready' ? (
        <div className="mt-4 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 dark:border-white/10 dark:bg-white/5">
          <StructuredAiEvaluation evaluation={state.evaluation.evaluation} />
          <div className="mt-3 text-xs text-[var(--text-secondary)]">
            {state.evaluation.profileName} · {state.evaluation.modelLabel} · {state.evaluation.usage.totalTokens.toLocaleString()} tokens
          </div>
          <div className="mt-1 text-xs text-[var(--text-secondary)]">
            {state.evaluation.serverGrounded
              ? `Grounded in NopsAI server analysis${state.evaluation.dataSources.length > 0 ? ` (${state.evaluation.dataSources.length} evidence source${state.evaluation.dataSources.length === 1 ? '' : 's'})` : ''}.`
              : 'Reviewed from this page snapshot only; no server evidence was available for this subject.'}
          </div>
          <div className="mt-1 text-xs text-[var(--text-secondary)]">
            {state.source === 'cache'
              ? `Loaded cached review${state.cachedAt ? ` from ${formatTimestamp(state.cachedAt)}` : ''}.`
              : state.source === 'cache-previous-snapshot'
                ? `Loaded latest cached review${state.cachedAt ? ` from ${formatTimestamp(state.cachedAt)}` : ''}; regenerate to rescore the current snapshot.`
              : `Review saved for this snapshot${historyCount > 1 ? `; ${historyCount} cached reviews exist for this subject.` : '.'}`}
          </div>
        </div>
      ) : state.status === 'error' ? (
        <div className="mt-4 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
          <div className="font-semibold">{aiEvaluationErrorTitle(state.error)}</div>
          <p className="mt-1">{aiEvaluationErrorDetail(state.error)}</p>
          {aiEvaluationErrorNeedsProfileLink(state.error) ? (
            <Link className="mt-2 inline-flex text-xs font-semibold underline underline-offset-4" to="/models">
              Open LLM Profiles
            </Link>
          ) : null}
        </div>
      ) : (
        <div className="mt-4 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm text-[var(--text-secondary)] dark:border-white/10 dark:bg-white/5">
          {historyCount > 0
            ? `${historyCount} cached review${historyCount === 1 ? '' : 's'} exist for this subject. Generate AI to review the current snapshot.`
            : autoEvaluates
              ? 'AI evaluation starts automatically for run analysis.'
              : 'Generate an AI evaluation when you want a second pass over the deterministic findings.'}
        </div>
      )}
    </section>
  );
}

type ReadyAnalysisAiEvaluationState = Extract<AnalysisAiEvaluationState, { status: 'ready' }>;

function StructuredAiEvaluation({ evaluation }: { evaluation: ReadyAnalysisAiEvaluationState['evaluation']['evaluation'] }) {
  return (
    <div className="space-y-3 text-sm">
      <p className="font-semibold text-[var(--text-primary)]">{evaluation.summary}</p>
      {!evaluation.structured ? (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
          The response was normalized into reviewer sections because the model did not return the expected JSON shape.
        </div>
      ) : null}
      <AiSection title="Problem">
        <div className="font-semibold text-[var(--text-primary)]">{evaluation.problem.title}</div>
        <p className="mt-1 text-[var(--text-secondary)]">{evaluation.problem.detail}</p>
      </AiSection>
      <AiSection title="Why This Score">
        {evaluation.score.health != null ? (
          <div className="mb-2 font-mono text-sm font-semibold text-[var(--text-primary)]">
            Reviewed health: {evaluation.score.health}/100
          </div>
        ) : null}
        <p className="text-[var(--text-secondary)]">{evaluation.score.detail}</p>
        {evaluation.score.drivers.length > 0 ? (
          <ul className="mt-2 space-y-1 text-xs text-[var(--text-secondary)]">
            {evaluation.score.drivers.map(driver => (
              <li key={driver} className="flex gap-2">
                <span aria-hidden="true">-</span>
                <span>{driver}</span>
              </li>
            ))}
          </ul>
        ) : null}
        {evaluation.score.findings.length > 0 ? (
          <div className="mt-3 space-y-2">
            {evaluation.score.findings.map(finding => (
              <div key={`${finding.severity}:${finding.category}:${finding.title}`} className="rounded-md border border-[var(--border-primary)] bg-white p-2 dark:border-white/10 dark:bg-black/20">
                <div className="flex flex-wrap items-center gap-2">
                  <span className={`inline-flex items-center rounded-md px-2 py-0.5 text-[10px] font-bold uppercase ${severityPillClass(finding.severity)}`}>
                    {finding.severity}
                  </span>
                  <span className="runner-pill runner-pill--muted">{analysisCategoryLabel(finding.category)}</span>
                  {finding.deduction != null ? <span className="runner-pill runner-pill--muted">-{finding.deduction}</span> : null}
                  <span className="runner-pill runner-pill--muted">{finding.confidence}%</span>
                </div>
                <div className="mt-1 text-sm font-semibold text-[var(--text-primary)]">{finding.title}</div>
                <p className="mt-1 text-xs text-[var(--text-secondary)]">{finding.basis}</p>
              </div>
            ))}
          </div>
        ) : null}
      </AiSection>
      {evaluation.fixes.length > 0 ? (
        <AiSection title="Suggested Fixes">
          <div className="space-y-2">
            {evaluation.fixes.map(fix => (
              <div key={`${fix.priority}:${fix.title}`} className="rounded-md border border-[var(--border-primary)] bg-white p-2 dark:border-white/10 dark:bg-black/20">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="runner-pill runner-pill--muted">{fix.priority || 'next'}</span>
                  <span className="font-semibold text-[var(--text-primary)]">{fix.title}</span>
                </div>
                <p className="mt-1 text-[var(--text-secondary)]">{fix.detail}</p>
                {fix.safeAction ? <p className="mt-1 text-xs text-[var(--text-secondary)]">Safe action: {fix.safeAction}</p> : null}
              </div>
            ))}
          </div>
        </AiSection>
      ) : null}
      {evaluation.evidenceNeeded.length > 0 ? (
        <AiSection title="More Evidence Needed">
          <ul className="space-y-1 text-[var(--text-secondary)]">
            {evaluation.evidenceNeeded.map(item => (
              <li key={item} className="flex gap-2">
                <span aria-hidden="true">-</span>
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </AiSection>
      ) : null}
      {evaluation.confidence > 0 ? (
        <div className="text-xs font-semibold text-[var(--text-secondary)]">AI confidence: {evaluation.confidence}%</div>
      ) : null}
    </div>
  );
}

function AiSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-md border border-[var(--border-primary)] bg-white p-3 dark:border-white/10 dark:bg-black/20">
      <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">{title}</div>
      <div className="mt-2">{children}</div>
    </section>
  );
}

function aiEvaluationErrorTitle(error: string) {
  if (/LLM profile|profile/i.test(error)) return 'AI Evaluation needs a usable LLM profile';
  if (/credential/i.test(error)) return 'AI Evaluation needs a usable credential';
  if (/disabled/i.test(error)) return 'AI Evaluation is disabled';
  return 'AI Evaluation is paused';
}

function aiEvaluationErrorDetail(error: string) {
  if (/LLM profile|profile/i.test(error)) {
    return 'The deterministic health score and findings are still available. Configure or fix an LLM profile with provider, model, credential reference when required, and scope access.';
  }
  if (/credential/i.test(error)) {
    return 'The deterministic health score and findings are still available. Fix the credential reference on the selected LLM profile to enable model-backed diagnosis and fix suggestions.';
  }
  if (/disabled/i.test(error)) {
    return 'The deterministic health score and findings are still available. Enable the relevant Assistant capability to add model-backed evaluation.';
  }
  return error || 'The deterministic health score and findings are still available.';
}

function aiEvaluationErrorNeedsProfileLink(error: string) {
  return /LLM profile|profile|credential/i.test(error);
}

function formatTimestamp(value: string) {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) return value;
  return timestamp.toLocaleString();
}

function SeverityCount({
  label,
  severity,
  value,
}: {
  label: string;
  severity: AnalysisSeverity;
  value: number;
}) {
  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2.5 py-1.5 text-right dark:border-white/10 dark:bg-white/5">
      <div className={`text-sm font-bold ${severityTextClass(severity)}`}>{value}</div>
      <div className="text-[10px] font-semibold uppercase text-[var(--text-secondary)]">{label}</div>
    </div>
  );
}

function MetricScore({ score }: { score: AnalysisScore }) {
  return (
    <div
      className="rounded-md border border-[var(--border-primary)] bg-white p-3 dark:border-white/10 dark:bg-black/20"
      title={score.basis}
    >
      <div className="flex items-center justify-between gap-3 text-sm">
        <span className="flex min-w-0 items-center gap-1 font-semibold text-[var(--text-primary)]">
          <span className="truncate">{score.label}</span>
          <Info className="h-3 w-3 shrink-0 text-[var(--text-secondary)]" aria-hidden="true" />
        </span>
        <span className="font-mono text-[var(--text-secondary)]">{score.score}/100</span>
      </div>
      <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-200 dark:bg-white/10">
        <div className={scoreBarClass(score.score)} style={{ width: `${score.score}%` }} />
      </div>
    </div>
  );
}

function FindingCard({
  finding,
  onDismiss,
}: {
  finding: AnalysisFinding;
  onDismiss: () => void;
}) {
  const firstHref = finding.affectedResources.find(resource => resource.href)?.href;
  return (
    <article className="rounded-lg border border-[var(--border-primary)] bg-white p-4 shadow-sm dark:border-white/10 dark:bg-slate-900">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-bold uppercase ${severityPillClass(finding.severity)}`}>
              {finding.severity === 'opportunity' ? <Sparkles className="h-3.5 w-3.5" aria-hidden="true" /> : <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />}
              {finding.severity}
            </span>
            <span className="runner-pill runner-pill--muted">{analysisCategoryLabel(finding.category)}</span>
            <span className="runner-pill runner-pill--muted">{finding.confidence}% confidence</span>
          </div>
          <h3 className="mt-3 text-base font-semibold text-[var(--text-primary)]">{finding.title}</h3>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">{finding.summary}</p>
        </div>
        <button type="button" className="glass-button-ghost px-3 py-1.5 text-xs" onClick={onDismiss}>
          Dismiss
        </button>
      </div>

      <div className="mt-4 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 dark:border-white/10 dark:bg-white/5">
        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Suggested Fix</div>
        <div className="mt-2 space-y-3">
          {finding.recommendations.map(recommendation => (
            <div key={recommendation.title} className="text-sm">
              <div className="font-semibold text-[var(--text-primary)]">{recommendation.title}</div>
              <p className="mt-1 text-[var(--text-secondary)]">{recommendation.detail}</p>
              {recommendation.suggestedYamlChange ? (
                <pre className="mt-2 overflow-x-auto rounded-md border border-[var(--border-primary)] bg-white p-2 text-xs text-[var(--text-primary)] dark:border-white/10 dark:bg-black/30">
                  <code>{recommendation.suggestedYamlChange}</code>
                </pre>
              ) : null}
            </div>
          ))}
        </div>
      </div>

      <details className="mt-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 dark:border-white/10 dark:bg-white/5">
        <summary className="cursor-pointer text-xs font-semibold uppercase text-[var(--text-secondary)]">
          Evidence and affected resources
        </summary>
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          <div>
            <div className="flex items-center gap-2 text-xs font-semibold uppercase text-[var(--text-secondary)]">
              <Info className="h-3.5 w-3.5" aria-hidden="true" />
              Evidence
            </div>
            <div className="mt-2 space-y-2">
              {finding.evidence.map(item => (
                <div key={`${item.label}:${item.value}`} className="text-sm">
                  <div className="text-xs font-semibold text-[var(--text-secondary)]">{item.label}{item.kind ? ` · ${item.kind}` : ''}</div>
                  <div className="mt-0.5 break-words font-mono text-xs text-[var(--text-primary)]">{item.value}</div>
                </div>
              ))}
            </div>
          </div>

          <div>
            <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Affected resources</div>
            <div className="mt-2 flex flex-wrap gap-2">
              {finding.affectedResources.length > 0 ? finding.affectedResources.map(resource => (
                resource.href ? (
                  <Link
                    key={`${resource.type}:${resource.id}`}
                    className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--border-primary)] bg-white px-2.5 py-1 text-xs font-semibold text-[var(--text-primary)] hover:border-indigo-300 hover:text-indigo-600 dark:border-white/10 dark:bg-black/20"
                    to={resource.href}
                  >
                    <span className="truncate">{resource.label}</span>
                    <ExternalLink className="h-3 w-3 shrink-0" aria-hidden="true" />
                  </Link>
                ) : (
                  <span
                    key={`${resource.type}:${resource.id}`}
                    className="inline-flex max-w-full items-center rounded-md border border-[var(--border-primary)] bg-white px-2.5 py-1 text-xs font-semibold text-[var(--text-primary)] dark:border-white/10 dark:bg-black/20"
                  >
                    <span className="truncate">{resource.label}</span>
                  </span>
                )
              )) : (
                <span className="text-sm text-[var(--text-secondary)]">No specific resource reference.</span>
              )}
            </div>
          </div>
        </div>
      </details>

      {firstHref ? (
        <div className="mt-3 flex flex-wrap gap-2">
          <Link className="glass-button-ghost px-3 py-1.5 text-xs" to={firstHref}>
            <ExternalLink className="h-4 w-4" aria-hidden="true" />
            View resource
          </Link>
        </div>
      ) : null}
    </article>
  );
}

function AnalysisActionButton({ action }: { action: AnalysisAction }) {
  const content = (
    <>
      {action.icon}
      <span>{action.label}</span>
    </>
  );

  if (action.to) {
    return (
      <Link className="glass-button-ghost" to={action.to}>
        {content}
      </Link>
    );
  }

  return (
    <button type="button" className="glass-button-ghost" onClick={action.onSelect}>
      {content}
    </button>
  );
}

function severityTextClass(severity: AnalysisSeverity) {
  if (severity === 'critical') return 'text-red-700 dark:text-red-200';
  if (severity === 'high') return 'text-orange-700 dark:text-orange-200';
  if (severity === 'medium') return 'text-amber-700 dark:text-amber-200';
  if (severity === 'low') return 'text-sky-700 dark:text-sky-200';
  return 'text-emerald-700 dark:text-emerald-200';
}

function severityPillClass(severity: AnalysisSeverity) {
  if (severity === 'critical') return 'bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-100';
  if (severity === 'high') return 'bg-orange-100 text-orange-800 dark:bg-orange-500/20 dark:text-orange-100';
  if (severity === 'medium') return 'bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-100';
  if (severity === 'low') return 'bg-sky-100 text-sky-800 dark:bg-sky-500/20 dark:text-sky-100';
  return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-100';
}

function scoreBarClass(score: number) {
  const color = score >= 80 ? 'bg-emerald-500' : score >= 60 ? 'bg-amber-500' : 'bg-red-500';
  return `h-full rounded-full ${color}`;
}
