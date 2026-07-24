import { useCallback, useMemo, useState, type KeyboardEvent, type RefObject, type UIEvent } from 'react';
import { Activity, ArrowLeft, BrainCircuit, Play } from 'lucide-react';
import { ResourceYamlDetailPanel } from '../editor/ResourceYamlDetailPanel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import { StepsGraph } from '../pipeline-runs/RunGraph';
import { AnalysisModal } from '../analysis/AnalysisModal';
import { buildPipelineAnalysis, type PipelineAnalysisScope } from '../analysis/model';
import type { PipelineRun, PipelineTrigger } from './api';
import { buildPipelineAnalysisPromptContext } from './pipelineAnalysisEvidence';
import { PipelineActivityPanels } from './PipelineActivityPanels';
import { normalizePipelineSource as normalizeSource, type PipelineDetail, type PipelineGraphData } from './model';

type PipelineDetailViewProps = {
  detail: PipelineDetail | null;
  graphData: PipelineGraphData;
  selectedGraphStep: string | null;
  isEditing: boolean;
  editorValue: string;
  validationErrors: YamlValidationError[];
  validationErrorLines: Set<number>;
  editorSuggestion: EditorAutocompleteSuggestion | null;
  autocompleteLoading: boolean;
  editorRef: RefObject<HTMLTextAreaElement | null>;
  highlightContentRef: RefObject<HTMLPreElement | null>;
  lineNumbersRef: RefObject<HTMLDivElement | null>;
  canUpdateSelectedPipeline: boolean;
  canCreatePipelineHere: boolean;
  canExecuteSelectedPipeline: boolean;
  saving: boolean;
  triggers: PipelineTrigger[];
  triggersLoading: boolean;
  triggersError: string | null;
  recentRuns: PipelineRun[];
  runsLoading: boolean;
  runsError: string | null;
  onBack: () => void;
  onExecute: () => void;
  onCopy: () => void;
  onDownload: () => void;
  onEdit: () => void;
  onClone: () => void;
  onDiscard: () => void;
  onSave: () => void;
  onSelectGraphStep: (step: string | null) => void;
  onOpenTrigger: (repoSlug: string) => void;
  onOpenDependency: (identifier: string) => void;
  onCopyDependency: (identifier: string) => void | Promise<void>;
  onOpenRun: (runID: string) => void;
  onEditorTextChange: (nextValue: string, cursor: number) => void;
  onOpenSuggestion: (cursor: number, opts?: { text?: string; force?: boolean }) => void;
  onMoveSuggestion: (direction: 1 | -1) => void;
  onDismissSuggestion: () => void;
  onSelectSuggestion: (value: string) => void;
  onEditorScroll: (event: UIEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
};

export function PipelineDetailView({
  detail,
  graphData,
  selectedGraphStep,
  isEditing,
  editorValue,
  validationErrors,
  validationErrorLines,
  editorSuggestion,
  autocompleteLoading,
  editorRef,
  highlightContentRef,
  lineNumbersRef,
  canUpdateSelectedPipeline,
  canCreatePipelineHere,
  canExecuteSelectedPipeline,
  saving,
  triggers,
  triggersLoading,
  triggersError,
  recentRuns,
  runsLoading,
  runsError,
  onBack,
  onExecute,
  onCopy,
  onDownload,
  onEdit,
  onClone,
  onDiscard,
  onSave,
  onSelectGraphStep,
  onOpenTrigger,
  onOpenDependency,
  onCopyDependency,
  onOpenRun,
  onEditorTextChange,
  onOpenSuggestion,
  onMoveSuggestion,
  onDismissSuggestion,
  onSelectSuggestion,
  onEditorScroll,
  onAutoIndentEnter,
}: PipelineDetailViewProps) {
  const [analysisOpen, setAnalysisOpen] = useState(false);
  const [analysisScope, setAnalysisScope] = useState<PipelineAnalysisScope>('complete');
  const [includeRunHistory, setIncludeRunHistory] = useState(true);
  const analysisResult = useMemo(
    () => detail
      ? buildPipelineAnalysis({
          detail,
          graphData,
          triggers,
          recentRuns,
          scope: analysisScope,
          includeRunHistory,
        })
      : null,
    [analysisScope, detail, graphData, includeRunHistory, recentRuns, triggers]
  );
  const loadAnalysisPromptContext = useCallback(
    async () => detail
      ? buildPipelineAnalysisPromptContext({
          detail,
          graphData,
          triggers,
          recentRuns,
          includeRunHistory,
          validationErrors,
          triggersLoading,
          triggersError,
          runsLoading,
          runsError,
        })
      : null,
    [
      detail,
      graphData,
      includeRunHistory,
      recentRuns,
      runsError,
      runsLoading,
      triggers,
      triggersError,
      triggersLoading,
      validationErrors,
    ]
  );

  if (!detail) {
    return (
      <div id="pipelines-detail-view" className="pipelines-view">
        <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a pipeline to see details.</div>
      </div>
    );
  }

  const source = normalizeSource(detail.source);
  const isGitSource = source === 'git';
  const executeDisabled = isEditing || source === 'draft' || !canExecuteSelectedPipeline;
  const analysisDisabled = isEditing || source === 'draft';
  const executeTitle = source === 'draft'
    ? 'Save the draft before executing'
    : isEditing
      ? 'Save or discard edits before executing'
      : canExecuteSelectedPipeline
        ? 'Execute in Lab'
        : 'You do not have permission to execute this pipeline';
  return (
    <>
      <div id="pipelines-detail-view" className="pipelines-view">
      <div className="min-w-0 space-y-6">
        <div className="glass-card p-6">
          <div className="flex items-start justify-between gap-4 w-full mb-4">
            <div>
              <h2 id="pipeline-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                {detail.name || detail.id}
              </h2>
              <p id="pipeline-detail-description" className="text-sm text-[var(--text-secondary)] mt-1">
                {detail.description || 'No description provided.'}
              </p>
              <div className="flex flex-wrap gap-3 mt-3 text-xs uppercase tracking-wide text-[var(--text-secondary)]">
                <span>Path: <span className="text-[var(--text-primary)]" id="pipeline-detail-path">{detail.path || 'Root'}</span></span>
                <span>Version: <span className="text-[var(--text-primary)]" id="pipeline-detail-version">{detail.version || 'latest'}</span></span>
                <span>Source: <span className="text-[var(--text-primary)]" id="pipeline-detail-source">{source}</span></span>
              </div>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <button
                id="pipelines-analyse-btn"
                type="button"
                className="glass-button-ghost"
                onClick={() => setAnalysisOpen(true)}
                disabled={analysisDisabled}
                title={analysisDisabled ? 'Save the pipeline before analysing this snapshot' : 'Analyse pipeline'}
              >
                <BrainCircuit className="h-4 w-4" aria-hidden="true" />
                <span>Analyse Pipeline</span>
              </button>
              <button
                id="pipelines-execute-btn"
                type="button"
                className="glass-button-primary"
                onClick={onExecute}
                disabled={executeDisabled}
                title={executeTitle}
              >
                <Play className="h-4 w-4" aria-hidden="true" />
                <span>Execute</span>
              </button>
              <button id="pipelines-back-btn" type="button" className="glass-button-ghost" onClick={onBack}>
                <ArrowLeft className="h-4 w-4" aria-hidden="true" />
                <span>Back to list</span>
              </button>
            </div>
          </div>
        </div>

        <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
          <div className="min-w-0 space-y-6">
            <ResourceYamlDetailPanel
              title="Pipeline Definition (YAML)"
              rawYaml={detail.rawYaml}
              isEditing={isEditing}
              editorValue={editorValue}
              validationErrors={validationErrors}
              validationErrorLines={validationErrorLines}
              editorSuggestion={editorSuggestion}
              autocompleteLoading={autocompleteLoading}
              editorRef={editorRef}
              highlightContentRef={highlightContentRef}
              lineNumbersRef={lineNumbersRef}
              ids={{
                content: 'pipeline-yaml-content',
                editorContainer: 'editor-container',
                lineNumbers: 'line-numbers',
                stage: 'pipeline-yaml-stage',
                highlight: 'pipeline-yaml-highlight',
                editor: 'pipeline-yaml-editor',
                validation: 'pipeline-validation-status',
                autocomplete: 'pipeline-editor-autocomplete',
              }}
              editorLabel="Pipeline YAML editor"
              access={source !== 'draft' ? { resourceType: 'pipeline', resourceID: detail.id, label: 'pipeline' } : null}
              canUpdate={canUpdateSelectedPipeline}
              canCreate={canCreatePipelineHere}
              isGitSource={isGitSource}
              saving={saving}
              onCopy={onCopy}
              onDownload={onDownload}
              onEdit={onEdit}
              onClone={onClone}
              onDiscard={onDiscard}
              onSave={onSave}
              onEditorTextChange={onEditorTextChange}
              onOpenSuggestion={onOpenSuggestion}
              onMoveSuggestion={onMoveSuggestion}
              onDismissSuggestion={onDismissSuggestion}
              onSelectSuggestion={onSelectSuggestion}
              onEditorScroll={onEditorScroll}
              onAutoIndentEnter={onAutoIndentEnter}
            />

            <div className="glass-card overflow-hidden">
              <div className="p-4">
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step Dependency Graph</h3>
                <p className="text-xs text-[var(--text-secondary)] mt-1">Based on `depends_on` relationships.</p>
              </div>
              <div className="pipelines-graph">
                {graphData.error ? (
                  <p className="text-sm text-red-500">Unable to render graph: {graphData.error}</p>
                ) : !graphData.steps.length ? (
                  <p className="text-sm text-[var(--text-secondary)]">No steps defined in this pipeline.</p>
                ) : (
                  <div className="rounded-2xl border border-[var(--border-primary)] bg-white dark:bg-slate-950 shadow-[0_16px_44px_rgba(15,23,42,0.07)] p-2">
                    <StepsGraph
                      steps={graphData.steps}
                      selectedStep={selectedGraphStep}
                      onSelectStep={onSelectGraphStep}
                      childRuns={[]}
                      pipelineDefinition={graphData.definition}
                      statusVariant="dot"
                      stepStatusColorOverride="#10b981"
                      taskStatusColorOverride="#60a5fa"
                      hideStatusLegend
                    />
                  </div>
                )}
              </div>
            </div>
          </div>

          <PipelineActivityPanels
            pipelineLabel={detail.name || detail.id}
            triggers={triggers}
            triggersLoading={triggersLoading}
            triggersError={triggersError}
            dependencies={detail.includedDependencies}
            runs={recentRuns}
            runsLoading={runsLoading}
            runsError={runsError}
            onOpenTrigger={onOpenTrigger}
            onOpenDependency={onOpenDependency}
            onCopyDependency={onCopyDependency}
            onOpenRun={onOpenRun}
          />
        </div>
      </div>
      </div>
      {analysisOpen && analysisResult ? (
        <AnalysisModal
          result={analysisResult}
          loadAiPromptContext={loadAnalysisPromptContext}
          controls={(
            <PipelineAnalysisControls
              scope={analysisScope}
              includeRunHistory={includeRunHistory}
              onScopeChange={setAnalysisScope}
              onIncludeRunHistoryChange={setIncludeRunHistory}
            />
          )}
          actions={recentRuns[0] ? [{
            id: 'latest-run',
            label: 'Open latest run',
            icon: <Activity className="h-4 w-4" aria-hidden="true" />,
            onSelect: () => onOpenRun(recentRuns[0].run_id),
          }] : []}
          onClose={() => setAnalysisOpen(false)}
        />
      ) : null}
    </>
  );
}

function PipelineAnalysisControls({
  scope,
  includeRunHistory,
  onScopeChange,
  onIncludeRunHistoryChange,
}: {
  scope: PipelineAnalysisScope;
  includeRunHistory: boolean;
  onScopeChange: (scope: PipelineAnalysisScope) => void;
  onIncludeRunHistoryChange: (value: boolean) => void;
}) {
  const scopeOptions: Array<{ value: PipelineAnalysisScope; label: string }> = [
    { value: 'complete', label: 'Complete analysis' },
    { value: 'security', label: 'Security' },
    { value: 'reliability', label: 'Reliability' },
    { value: 'monitoring', label: 'Monitoring' },
    { value: 'performance', label: 'Performance' },
    { value: 'maintainability', label: 'Maintainability' },
    { value: 'pre-execution', label: 'Pre-execution check' },
  ];

  return (
    <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_16rem]">
      <div>
        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Scope</div>
        <div className="mt-2 flex flex-wrap gap-2" role="radiogroup" aria-label="Pipeline analysis scope">
          {scopeOptions.map(option => (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={scope === option.value}
              className={`pipeline-runs-segment ${scope === option.value ? 'pipeline-runs-segment--active' : ''}`}
              onClick={() => onScopeChange(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>
      <div>
        <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Run history</div>
        <label className="mt-2 flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-white px-3 py-2 text-sm font-semibold text-[var(--text-primary)] dark:border-white/10 dark:bg-black/20">
          <input
            type="checkbox"
            checked={includeRunHistory}
            onChange={event => onIncludeRunHistoryChange(event.target.checked)}
          />
          Include last 30 runs
        </label>
      </div>
    </div>
  );
}
