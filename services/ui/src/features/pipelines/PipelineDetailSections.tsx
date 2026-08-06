import type { KeyboardEvent, ReactElement, ReactNode, RefObject, UIEvent } from 'react';
import { ResourceYamlDetailPanel } from '../editor/ResourceYamlDetailPanel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import type { PipelineAnalysisScope } from '../analysis/model';
import { analysisCategoryLabel, type AnalysisFinding } from '../analysis/model';
import { formatPipelineDetailPath, formatPipelineDetailSource } from './pipelineDetailPresentation';
import { GLOBAL_RESOURCE_TEAM_PATH } from '../../lib/resourceTeams';
import type { PipelineDependencyReference, PipelineDetail } from './model';

export type DetailTabID = 'flow' | 'definition' | 'triggers' | 'runs' | 'health' | 'dependencies';

export function MetaItem({ id, icon, label, value }: { id: string; icon: ReactElement; label: string; value: string }) {
  return (
    <span className="pipeline-detail-meta-item">
      {icon}
      <span>{label}</span>
      <b id={id}>{value}</b>
    </span>
  );
}

export function ActivityTabPanel({
  id,
  title,
  tabLabel,
  children,
}: {
  id: DetailTabID;
  title: string;
  tabLabel: string;
  children: ReactNode;
}) {
  return (
    <section
      id={`pipeline-detail-panel-${id}`}
      role="tabpanel"
      aria-labelledby={`pipeline-detail-tab-${id}`}
      className="pipeline-detail-panel"
    >
      <div className="pipeline-detail-section-intro">
        <div>
          <h3>{title}</h3>
        </div>
      </div>
      <div aria-label={tabLabel}>
        {children}
      </div>
    </section>
  );
}

export function PipelineDefinitionPanel({
  detail,
  source,
  sourceState,
  updatedLabel,
  isGitSource,
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
  saving,
  editablePipelineName,
  editablePipelineTeam,
  dependencies,
  onCopy,
  onDownload,
  onEdit,
  onClone,
  onDiscard,
  onSave,
  onOpenDependency,
  onCopyDependency,
  onEditorTextChange,
  onOpenSuggestion,
  onMoveSuggestion,
  onDismissSuggestion,
  onSelectSuggestion,
  onEditorScroll,
  onAutoIndentEnter,
  onEditablePipelineNameChange,
  onEditablePipelineTeamChange,
}: {
  detail: PipelineDetail;
  source: string;
  sourceState: ReturnType<typeof formatPipelineDetailSource>;
  updatedLabel: string;
  isGitSource: boolean;
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
  saving: boolean;
  editablePipelineName: string;
  editablePipelineTeam: string;
  dependencies: PipelineDependencyReference[];
  onCopy: () => void;
  onDownload: () => void;
  onEdit: () => void;
  onClone: () => void;
  onDiscard: () => void;
  onSave: () => void;
  onOpenDependency: (dependency: PipelineDependencyReference) => void;
  onCopyDependency: (identifier: string) => void | Promise<void>;
  onEditorTextChange: (nextValue: string, cursor: number) => void;
  onOpenSuggestion: (cursor: number, opts?: { text?: string; force?: boolean }) => void;
  onMoveSuggestion: (direction: 1 | -1) => void;
  onDismissSuggestion: () => void;
  onSelectSuggestion: (value: string) => void;
  onEditorScroll: (event: UIEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  onEditablePipelineNameChange: (value: string) => void;
  onEditablePipelineTeamChange: (value: string) => void;
}) {
  return (
    <section
      id="pipeline-detail-panel-definition"
      role="tabpanel"
      aria-labelledby="pipeline-detail-tab-definition"
      className="pipeline-detail-panel"
    >
      <div className="pipeline-detail-definition-layout">
        <div className="pipeline-detail-definition-main">
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
            showActions={false}
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
        </div>
        <DefinitionSidePanel
          detail={detail}
          sourceState={sourceState}
          updatedLabel={updatedLabel}
          isEditing={isEditing}
          editablePipelineName={editablePipelineName}
          editablePipelineTeam={editablePipelineTeam}
          validationErrorCount={validationErrors.length}
          dependencies={dependencies}
          onOpenDependency={onOpenDependency}
          onCopyDependency={onCopyDependency}
          onEditablePipelineNameChange={onEditablePipelineNameChange}
          onEditablePipelineTeamChange={onEditablePipelineTeamChange}
        />
      </div>
    </section>
  );
}

function DefinitionSidePanel({
  detail,
  sourceState,
  updatedLabel,
  isEditing,
  editablePipelineName,
  editablePipelineTeam,
  validationErrorCount,
  dependencies,
  onOpenDependency,
  onCopyDependency,
  onEditablePipelineNameChange,
  onEditablePipelineTeamChange,
}: {
  detail: PipelineDetail;
  sourceState: ReturnType<typeof formatPipelineDetailSource>;
  updatedLabel: string;
  isEditing: boolean;
  editablePipelineName: string;
  editablePipelineTeam: string;
  validationErrorCount: number;
  dependencies: PipelineDependencyReference[];
  onOpenDependency: (dependency: PipelineDependencyReference) => void;
  onCopyDependency: (identifier: string) => void | Promise<void>;
  onEditablePipelineNameChange: (value: string) => void;
  onEditablePipelineTeamChange: (value: string) => void;
}) {
  return (
    <aside className="pipeline-detail-definition-side" aria-label="Definition summary">
      <section className="pipeline-detail-side-panel">
        <h3>Identity</h3>
        {isEditing ? (
          <div className="pipeline-detail-identity-fields">
            <label htmlFor="pipeline-edit-team">
              <span>Team</span>
              <input
                id="pipeline-edit-team"
                type="text"
                className="pipelines-input"
                value={editablePipelineTeam}
                onChange={event => onEditablePipelineTeamChange(event.target.value)}
                placeholder="team/service"
              />
            </label>
            <label htmlFor="pipeline-edit-name">
              <span>Name</span>
              <input
                id="pipeline-edit-name"
                type="text"
                className="pipelines-input"
                value={editablePipelineName}
                onChange={event => onEditablePipelineNameChange(event.target.value)}
                placeholder="build-and-test"
              />
            </label>
          </div>
        ) : (
          <dl className="pipeline-detail-key-values">
            <KeyValue label="Team" value={detail.path || GLOBAL_RESOURCE_TEAM_PATH} />
            <KeyValue label="Name" value={detail.name || detail.id} />
          </dl>
        )}
      </section>
      <section className="pipeline-detail-side-panel">
        <h3>Source and sync</h3>
        <dl className="pipeline-detail-key-values">
          <KeyValue label="Source" value={sourceState.label} />
          <KeyValue label="State" value={sourceState.description} />
          <KeyValue label="Path" value={formatPipelineDetailPath(detail)} />
          <KeyValue label="Updated" value={updatedLabel} />
        </dl>
      </section>
      <section className="pipeline-detail-side-panel">
        <h3>Validation</h3>
        <dl className="pipeline-detail-key-values">
          <KeyValue label="YAML syntax" value={validationErrorCount ? 'Needs review' : 'Valid'} tone={validationErrorCount ? 'danger' : 'success'} />
          <KeyValue label="Errors" value={String(validationErrorCount)} />
          <KeyValue label="Version" value={detail.version || 'latest'} />
        </dl>
      </section>
      <section className="pipeline-detail-side-panel">
        <h3>Dependencies</h3>
        <DependencyLinks dependencies={dependencies} onOpenDependency={onOpenDependency} onCopyDependency={onCopyDependency} stacked />
      </section>
    </aside>
  );
}

function KeyValue({ label, value, tone }: { label: string; value: string; tone?: 'success' | 'danger' }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={tone ? `pipeline-detail-key-value--${tone}` : ''}>{value}</dd>
    </div>
  );
}

export function DependencyLinks({
  dependencies,
  onOpenDependency,
  onCopyDependency,
  stacked = false,
}: {
  dependencies: PipelineDependencyReference[];
  onOpenDependency: (dependency: PipelineDependencyReference) => void;
  onCopyDependency: (identifier: string) => void | Promise<void>;
  stacked?: boolean;
}) {
  if (!dependencies.length) {
    return <span className="pipeline-detail-muted">No dependencies detected</span>;
  }
  return (
    <div className={stacked ? 'pipeline-detail-dependency-list' : 'pipeline-detail-dependency-links'}>
      {!stacked ? <span>Dependencies</span> : null}
      {dependencies.map(dependency => (
        <button
          key={dependency.raw}
          type="button"
          className="pipeline-detail-resource-link"
          title={`${dependency.actionLabel} ${dependency.identifier || dependency.raw}`}
          onClick={() =>
            dependency.navigable
              ? onOpenDependency(dependency)
              : void onCopyDependency(dependency.identifier || dependency.raw)
          }
        >
          <span>{dependency.identifier || dependency.raw}</span>
          <small>{dependency.typeLabel}</small>
        </button>
      ))}
    </div>
  );
}

export function SummaryBlock({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="pipeline-detail-summary-block">
      <span>{label}</span>
      <b className={mono ? 'font-mono' : ''}>{value}</b>
    </div>
  );
}

export function FindingRow({ finding, onOpenDefinition }: { finding: AnalysisFinding; onOpenDefinition: () => void }) {
  const evidence = finding.evidence[0];
  return (
    <div className="pipeline-detail-finding-row">
      <span className={`pipeline-detail-severity pipeline-detail-severity--${finding.severity}`}>
        {finding.severity}
      </span>
      <div>
        <div className="pipeline-detail-finding-title">{finding.title}</div>
        <p>{finding.summary}</p>
      </div>
      <div className="pipeline-detail-finding-location">
        {evidence ? `${evidence.label}: ${evidence.value}` : analysisCategoryLabel(finding.category)}
      </div>
      <button type="button" className="glass-button-ghost pipeline-detail-small-action" onClick={onOpenDefinition}>
        Definition
      </button>
    </div>
  );
}

export function PipelineAnalysisControls({
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
