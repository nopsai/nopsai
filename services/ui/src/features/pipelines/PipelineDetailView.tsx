import {
  useCallback,
  useMemo,
  useState,
  type KeyboardEvent,
  type RefObject,
  type UIEvent,
} from 'react';
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  BrainCircuit,
  Clock3,
  Copy,
  Download,
  FileCode2,
  GitBranch,
  Layers3,
  Network,
  Play,
  Save,
  SquarePen,
  X,
} from 'lucide-react';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import { formatResourceListUpdatedAt } from '../editor/resourceCollectionModel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import { StepsGraph } from '../pipeline-runs/RunGraph';
import { AnalysisWorkspace } from '../analysis/AnalysisModal';
import { buildPipelineAnalysis, type PipelineAnalysisScope } from '../analysis/model';
import type { PipelineRun, PipelineTrigger } from './api';
import { buildPipelineAnalysisPromptContext } from './pipelineAnalysisEvidence';
import { PipelineActivityPanels } from './PipelineActivityPanels';
import {
  normalizePipelineSource as normalizeSource,
  parsePipelineDependencyReference,
  type PipelineDetail,
  type PipelineGraphData,
} from './model';
import {
  ActivityTabPanel,
  DependencyLinks,
  MetaItem,
  PipelineAnalysisControls,
  PipelineDefinitionPanel,
  PipelineMetricCard,
  SummaryBlock,
  type DetailTabID,
} from './PipelineDetailSections';
import {
  buildPipelineDetailMetrics,
  countPipelineGraphTasks,
  formatPipelineDetailPath,
  formatPipelineDetailSource,
  summarizePipelineLatestRun,
} from './pipelineDetailPresentation';

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
  editablePipelineName: string;
  editablePipelineTeam: string;
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
  onEditablePipelineNameChange: (value: string) => void;
  onEditablePipelineTeamChange: (value: string) => void;
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
  editablePipelineName,
  editablePipelineTeam,
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
  onEditablePipelineNameChange,
  onEditablePipelineTeamChange,
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
  const [analysisScope, setAnalysisScope] = useState<PipelineAnalysisScope>('complete');
  const [includeRunHistory, setIncludeRunHistory] = useState(true);
  const [activeTab, setActiveTab] = useState<DetailTabID>('flow');
  const [analysisRequestKey, setAnalysisRequestKey] = useState(0);
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
      <div id="pipelines-detail-view" className="pipelines-view pipelines-detail-shell">
        <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a pipeline to see details.</div>
      </div>
    );
  }

  const source = normalizeSource(detail.source);
  const sourceState = formatPipelineDetailSource(detail.source);
  const updatedLabel = formatResourceListUpdatedAt(detail.updatedAt);
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
  const latestRun = summarizePipelineLatestRun(recentRuns);
  const dependencyRefs = Array.from(new Set(detail.includedDependencies))
    .map(parsePipelineDependencyReference)
    .sort((a, b) => a.raw.localeCompare(b.raw));
  const metrics = buildPipelineDetailMetrics({
    graphData,
    triggers,
    recentRuns,
    analysis: analysisResult,
    validationErrorCount: validationErrors.length,
  });
  const tabs: Array<{ id: DetailTabID; label: string; count?: number }> = [
    { id: 'flow', label: 'Flow' },
    { id: 'definition', label: 'Definition' },
    { id: 'triggers', label: 'Trigger rules', count: triggers.length },
    { id: 'runs', label: 'Runs', count: recentRuns.length },
    { id: 'health', label: 'Health', count: analysisResult?.findings.length || 0 },
    { id: 'dependencies', label: 'Dependencies', count: dependencyRefs.length },
  ];

  const openDefinitionForEdit = () => {
    setActiveTab('definition');
    if (!isEditing) onEdit();
  };

  const analysePipeline = () => {
    setActiveTab('health');
    setAnalysisRequestKey(current => current + 1);
  };

  return (
    <>
      <div id="pipelines-detail-view" className="pipelines-view pipelines-detail-shell">
        <section className="pipeline-detail-hero" aria-labelledby="pipeline-detail-name">
          <div className="pipeline-detail-hero__main">
            <button id="pipelines-back-btn" type="button" className="pipeline-detail-back" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>Back to list</span>
            </button>
            <div className="pipeline-detail-title-row">
              <h2 id="pipeline-detail-name">{detail.name || detail.id}</h2>
              <span className={`pipeline-detail-source pipeline-detail-source--${sourceState.tone}`}>
                <span aria-hidden="true" />
                {sourceState.label}
              </span>
            </div>
            <p id="pipeline-detail-description" className="pipeline-detail-description">
              {detail.description || 'No description provided.'}
            </p>
            <div className="pipeline-detail-meta-line" aria-label="Pipeline metadata">
              <MetaItem id="pipeline-detail-path" icon={<FileCode2 className="h-3.5 w-3.5" aria-hidden="true" />} label="Path" value={formatPipelineDetailPath(detail)} />
              <MetaItem id="pipeline-detail-version" icon={<GitBranch className="h-3.5 w-3.5" aria-hidden="true" />} label="Version" value={detail.version || 'latest'} />
              <MetaItem id="pipeline-detail-source" icon={<Layers3 className="h-3.5 w-3.5" aria-hidden="true" />} label="Source" value={source} />
              <MetaItem id="pipeline-detail-updated" icon={<Clock3 className="h-3.5 w-3.5" aria-hidden="true" />} label="Updated" value={updatedLabel} />
            </div>
          </div>

          <div className="pipeline-detail-hero__actions" aria-label="Pipeline actions">
            <button
              id="pipelines-analyse-btn"
              type="button"
              className="glass-button-ghost pipeline-detail-action"
              onClick={analysePipeline}
              disabled={analysisDisabled}
              title={analysisDisabled ? 'Save the pipeline before analysing this snapshot' : 'Analyse pipeline'}
            >
              <BrainCircuit className="h-4 w-4" aria-hidden="true" />
              <span>Analyse Pipeline</span>
            </button>
            {!isEditing ? (
              <>
                <button
                  type="button"
                  className="glass-button-ghost pipeline-detail-action"
                  onClick={onCopy}
                  title="Copy YAML"
                  aria-label="Copy YAML"
                >
                  <Copy className="h-4 w-4" aria-hidden="true" />
                  <span>Copy YAML</span>
                </button>
                <button
                  type="button"
                  className="glass-button-ghost pipeline-detail-action"
                  onClick={onDownload}
                  title="Download YAML"
                  aria-label="Download YAML"
                >
                  <Download className="h-4 w-4" aria-hidden="true" />
                  <span>Download</span>
                </button>
                {source !== 'draft' ? (
                  <ResourceAccessCard
                    resourceType="pipeline"
                    resourceID={detail.id}
                    label="pipeline"
                    buttonClassName="glass-button-ghost pipeline-detail-action"
                  />
                ) : null}
                {canCreatePipelineHere ? (
                  <button type="button" className="glass-button-ghost pipeline-detail-action" onClick={onClone}>
                    <Copy className="h-4 w-4" aria-hidden="true" />
                    <span>Clone</span>
                  </button>
                ) : null}
                {canUpdateSelectedPipeline ? (
                  <button
                    type="button"
                    className="glass-button-ghost pipeline-detail-action"
                    onClick={openDefinitionForEdit}
                    title="Edit pipeline"
                  >
                    <SquarePen className="h-4 w-4" aria-hidden="true" />
                    <span>Edit</span>
                  </button>
                ) : null}
              </>
            ) : canUpdateSelectedPipeline || source === 'draft' ? (
              <>
                <button type="button" className="glass-button-ghost pipeline-detail-action" onClick={onDiscard}>
                  <X className="h-4 w-4" aria-hidden="true" />
                  <span>Discard</span>
                </button>
                <button
                  type="button"
                  className="glass-button-primary pipeline-detail-action"
                  onClick={onSave}
                  disabled={saving || validationErrors.length > 0}
                >
                  <Save className="h-4 w-4" aria-hidden="true" />
                  <span>{saving ? 'Saving...' : 'Save'}</span>
                </button>
              </>
            ) : null}
            <button
              id="pipelines-execute-btn"
              type="button"
              className="glass-button-primary pipeline-detail-action"
              onClick={onExecute}
              disabled={executeDisabled}
              title={executeTitle}
            >
              <Play className="h-4 w-4" aria-hidden="true" />
              <span>Execute</span>
            </button>
          </div>
        </section>

        <section className="pipeline-detail-metrics" aria-label="Pipeline summary">
          {metrics.map(metric => (
            <PipelineMetricCard key={metric.id} metric={metric} />
          ))}
        </section>

        <div className="pipeline-detail-tabs-wrap">
          <div className="pipeline-detail-tabs" role="tablist" aria-label="Pipeline detail sections">
            {tabs.map(tab => (
              <button
                key={tab.id}
                type="button"
                id={`pipeline-detail-tab-${tab.id}`}
                role="tab"
                aria-selected={activeTab === tab.id}
                aria-controls={`pipeline-detail-panel-${tab.id}`}
                className={`pipeline-detail-tab ${activeTab === tab.id ? 'pipeline-detail-tab--active' : ''}`}
                onClick={() => setActiveTab(tab.id)}
              >
                <span>{tab.label}</span>
                {typeof tab.count === 'number' ? <span className="pipeline-detail-tab-count">{tab.count}</span> : null}
              </button>
            ))}
          </div>
        </div>

        <div className="pipeline-detail-panels">
          {activeTab === 'flow' ? (
            <section
              id="pipeline-detail-panel-flow"
              role="tabpanel"
              aria-labelledby="pipeline-detail-tab-flow"
              className="pipeline-detail-panel"
            >
              <article className="pipeline-detail-card pipeline-detail-flow-card">
                <header className="pipeline-detail-card-header">
                  <div>
                    <h3>Pipeline graph</h3>
                    <p>Execution order from <code>depends_on</code> relationships.</p>
                  </div>
                  <div className="pipeline-detail-inline-summary" aria-label="Graph summary">
                    <span><b>{graphData.steps.length}</b> steps</span>
                    <span aria-hidden="true">·</span>
                    <span><b>{countPipelineGraphTasks(graphData)}</b> tasks</span>
                    <span aria-hidden="true">·</span>
                    <span><b>{dependencyRefs.length}</b> includes</span>
                  </div>
                </header>
                <div className="pipeline-detail-graph-stage">
                  {graphData.error ? (
                    <div className="pipeline-detail-state pipeline-detail-state--danger">
                      <AlertTriangle className="h-5 w-5" aria-hidden="true" />
                      <span>Unable to render graph: {graphData.error}</span>
                    </div>
                  ) : !graphData.steps.length ? (
                    <div className="pipeline-detail-state">
                      <Network className="h-5 w-5" aria-hidden="true" />
                      <span>No steps defined in this pipeline.</span>
                    </div>
                  ) : (
                    <StepsGraph
                      steps={graphData.steps}
                      selectedStep={selectedGraphStep}
                      onSelectStep={onSelectGraphStep}
                      childRuns={[]}
                      pipelineDefinition={graphData.definition}
                      statusVariant="dot"
                      stepStatusColorOverride="#14b8a6"
                      taskStatusColorOverride="#38bdf8"
                      hideStatusLegend
                    />
                  )}
                </div>
                <footer className="pipeline-detail-flow-footer">
                  <span>Source <strong>{sourceState.description}</strong></span>
                  <DependencyLinks dependencies={dependencyRefs} onOpenDependency={onOpenDependency} onCopyDependency={onCopyDependency} />
                </footer>
              </article>
            </section>
          ) : null}

          {activeTab === 'definition' ? (
            <PipelineDefinitionPanel
              detail={detail}
              source={source}
              sourceState={sourceState}
              updatedLabel={updatedLabel}
              isGitSource={isGitSource}
              isEditing={isEditing}
              editorValue={editorValue}
              validationErrors={validationErrors}
              validationErrorLines={validationErrorLines}
              editorSuggestion={editorSuggestion}
              autocompleteLoading={autocompleteLoading}
              editorRef={editorRef}
              highlightContentRef={highlightContentRef}
              lineNumbersRef={lineNumbersRef}
              canUpdateSelectedPipeline={canUpdateSelectedPipeline}
              canCreatePipelineHere={canCreatePipelineHere}
              saving={saving}
              editablePipelineName={editablePipelineName}
              editablePipelineTeam={editablePipelineTeam}
              dependencies={dependencyRefs}
              onCopy={onCopy}
              onDownload={onDownload}
              onEdit={onEdit}
              onClone={onClone}
              onDiscard={onDiscard}
              onSave={onSave}
              onEditablePipelineNameChange={onEditablePipelineNameChange}
              onEditablePipelineTeamChange={onEditablePipelineTeamChange}
              onOpenDependency={onOpenDependency}
              onCopyDependency={onCopyDependency}
              onEditorTextChange={onEditorTextChange}
              onOpenSuggestion={onOpenSuggestion}
              onMoveSuggestion={onMoveSuggestion}
              onDismissSuggestion={onDismissSuggestion}
              onSelectSuggestion={onSelectSuggestion}
              onEditorScroll={onEditorScroll}
              onAutoIndentEnter={onAutoIndentEnter}
            />
          ) : null}

          {activeTab === 'triggers' ? (
            <ActivityTabPanel id="triggers" title="Trigger rules" tabLabel="Trigger rules">
              <PipelineActivityPanels
                pipelineLabel={detail.name || detail.id}
                triggers={triggers}
                triggersLoading={triggersLoading}
                triggersError={triggersError}
                dependencies={detail.includedDependencies}
                runs={recentRuns}
                runsLoading={runsLoading}
                runsError={runsError}
                sections={['triggers']}
                variant="rows"
                onOpenTrigger={onOpenTrigger}
                onOpenDependency={onOpenDependency}
                onCopyDependency={onCopyDependency}
                onOpenRun={onOpenRun}
              />
            </ActivityTabPanel>
          ) : null}

          {activeTab === 'runs' ? (
            <ActivityTabPanel id="runs" title="Pipeline runs" tabLabel="Runs">
              <div className="pipeline-detail-run-summary">
                <SummaryBlock label="Latest status" value={latestRun.statusLabel} />
                <SummaryBlock label="Branch" value={latestRun.branchLabel} />
                <SummaryBlock label="Run ID" value={latestRun.runLabel} mono />
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
                sections={['runs']}
                variant="rows"
                onOpenTrigger={onOpenTrigger}
                onOpenDependency={onOpenDependency}
                onCopyDependency={onCopyDependency}
                onOpenRun={onOpenRun}
              />
            </ActivityTabPanel>
          ) : null}

          {activeTab === 'health' ? (
            <section
              id="pipeline-detail-panel-health"
              role="tabpanel"
              aria-labelledby="pipeline-detail-tab-health"
              className="pipeline-detail-panel pipeline-detail-health-panel"
            >
              {analysisResult ? (
                <AnalysisWorkspace
                  result={analysisResult}
                  loadAiPromptContext={loadAnalysisPromptContext}
                  autoRequestKey={analysisRequestKey || undefined}
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
                />
              ) : null}
            </section>
          ) : null}

          {activeTab === 'dependencies' ? (
            <ActivityTabPanel id="dependencies" title="Included dependencies" tabLabel="Dependencies">
              <PipelineActivityPanels
                pipelineLabel={detail.name || detail.id}
                triggers={triggers}
                triggersLoading={triggersLoading}
                triggersError={triggersError}
                dependencies={detail.includedDependencies}
                runs={recentRuns}
                runsLoading={runsLoading}
                runsError={runsError}
                sections={['dependencies']}
                variant="rows"
                onOpenTrigger={onOpenTrigger}
                onOpenDependency={onOpenDependency}
                onCopyDependency={onCopyDependency}
                onOpenRun={onOpenRun}
              />
            </ActivityTabPanel>
          ) : null}
        </div>
      </div>
    </>
  );
}
