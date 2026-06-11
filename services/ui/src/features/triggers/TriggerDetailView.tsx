import type { KeyboardEvent, RefObject, UIEvent } from 'react';
import { ArrowLeft } from 'lucide-react';
import { ResourceYamlDetailPanel } from '../editor/ResourceYamlDetailPanel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import { TriggerRecentRuns } from './TriggerRecentRuns';
import {
  normalizeSource,
  sourceLabel,
  type PipelineMeta,
  type PipelineRef,
  type TriggerDetail,
  type TriggerRun,
} from './model';

type TriggerDetailViewProps = {
  detail: TriggerDetail | null;
  isEditing: boolean;
  editorValue: string;
  validationErrors: YamlValidationError[];
  validationErrorLines: Set<number>;
  editorSuggestion: EditorAutocompleteSuggestion | null;
  autocompleteLoading: boolean;
  editorRef: RefObject<HTMLTextAreaElement | null>;
  highlightContentRef: RefObject<HTMLPreElement | null>;
  lineNumbersRef: RefObject<HTMLDivElement | null>;
  canUpdateSelectedTrigger: boolean;
  canCreateTriggerHere: boolean;
  saving: boolean;
  linkedPipelines: PipelineRef[];
  pipelineMetadata: Map<string, PipelineMeta>;
  pipelineSourceIndex: Map<string, string> | null;
  recentRuns: TriggerRun[];
  runsLoading: boolean;
  runsError: string | null;
  runsScrollable: boolean;
  recentRunsListRef: RefObject<HTMLUListElement | null>;
  onBack: () => void;
  onOpenScope: (scope: string) => void;
  onOpenPipeline: (identifier: string) => void;
  onOpenRun: (runId: string) => void;
  onRecentRunsScroll: (event: UIEvent<HTMLUListElement>) => void;
  onCopy: () => void;
  onDownload: () => void;
  onEdit: () => void;
  onClone: () => void;
  onDiscard: () => void;
  onSave: () => void;
  onEditorTextChange: (nextValue: string, cursor: number) => void;
  onOpenSuggestion: (cursor: number, opts?: { text?: string; force?: boolean }) => void;
  onMoveSuggestion: (direction: 1 | -1) => void;
  onDismissSuggestion: () => void;
  onSelectSuggestion: (value: string) => void;
  onEditorScroll: (event: UIEvent<HTMLTextAreaElement>) => void;
  onIndentTab: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
};

export function TriggerDetailView({
  detail,
  isEditing,
  editorValue,
  validationErrors,
  validationErrorLines,
  editorSuggestion,
  autocompleteLoading,
  editorRef,
  highlightContentRef,
  lineNumbersRef,
  canUpdateSelectedTrigger,
  canCreateTriggerHere,
  saving,
  linkedPipelines,
  pipelineMetadata,
  pipelineSourceIndex,
  recentRuns,
  runsLoading,
  runsError,
  runsScrollable,
  recentRunsListRef,
  onBack,
  onOpenScope,
  onOpenPipeline,
  onOpenRun,
  onRecentRunsScroll,
  onCopy,
  onDownload,
  onEdit,
  onClone,
  onDiscard,
  onSave,
  onEditorTextChange,
  onOpenSuggestion,
  onMoveSuggestion,
  onDismissSuggestion,
  onSelectSuggestion,
  onEditorScroll,
  onIndentTab,
  onAutoIndentEnter,
}: TriggerDetailViewProps) {
  if (!detail) {
    return (
      <div id="triggers-detail-view" className="pipelines-view">
        <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a trigger to see details.</div>
      </div>
    );
  }

  const sourceKey = normalizeSource(detail.source);
  const isGitSource = sourceKey === 'git';

  return (
    <div id="triggers-detail-view" className="pipelines-view">
      <div className="min-w-0 space-y-6">
        <div className="glass-card p-6">
          <div className="flex items-start justify-between gap-4 w-full mb-4">
            <div className="min-w-0">
              <div className="triggers-detail-heading">
                <span className="triggers-detail-icon" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
                  </svg>
                </span>
                <div className="min-w-0">
                  <h2 id="triggers-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                    {detail.slug}
                  </h2>
                  <div className="triggers-detail-meta">
                    <dl className="triggers-detail-grid">
                      <dt className="triggers-detail-label">Source:</dt>
                      <dd className="triggers-detail-value">{sourceLabel(sourceKey)}</dd>
                      <dt className="triggers-detail-label">Rules:</dt>
                      <dd className="triggers-detail-value">{detail.summary.triggerCount}</dd>
                      <dt className="triggers-detail-label">Events:</dt>
                      <dd className="triggers-detail-value">
                        {detail.summary.events.length ? detail.summary.events.join(', ') : 'N/A'}
                      </dd>
                      <dt className="triggers-detail-label" style={{ alignSelf: 'flex-start', marginTop: 4 }}>
                        Scopes:
                      </dt>
                      <dd
                        className="triggers-detail-value flex flex-wrap gap-1.5"
                        style={{ whiteSpace: 'normal', overflow: 'visible', textOverflow: 'clip' }}
                      >
                        {detail.summary.scopes.length ? (
                          detail.summary.scopes.map(scope => {
                            const label = scope ? `/${scope}` : 'Default Scope';
                            return (
                              <button
                                key={`scope-${scope || 'default'}`}
                                type="button"
                                className="pipelines-tag font-semibold transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-accent)]"
                                onClick={() => onOpenScope(scope)}
                              >
                                {label}
                              </button>
                            );
                          })
                        ) : (
                          <button
                            type="button"
                            className="pipelines-tag font-semibold transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-accent)]"
                            onClick={() => onOpenScope('')}
                          >
                            Default Scope
                          </button>
                        )}
                      </dd>
                    </dl>
                  </div>
                </div>
              </div>
            </div>
            <button id="triggers-back-btn" type="button" className="glass-button-ghost" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>Back to list</span>
            </button>
          </div>
        </div>

        <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
          <div className="min-w-0 space-y-6">
            <ResourceYamlDetailPanel
              title="Trigger Definition (YAML)"
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
                content: 'triggers-yaml-content',
                editorContainer: 'editor-container',
                lineNumbers: 'line-numbers',
                stage: 'triggers-yaml-stage',
                highlight: 'triggers-yaml-highlight',
                editor: 'triggers-yaml-editor',
                validation: 'trigger-validation-status',
                autocomplete: 'trigger-editor-autocomplete',
              }}
              editorLabel="Trigger YAML editor"
              access={null}
              canUpdate={canUpdateSelectedTrigger}
              canCreate={canCreateTriggerHere}
              isGitSource={isGitSource}
              saving={saving}
              autocompleteWidth={340}
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
              onIndentTab={onIndentTab}
              onAutoIndentEnter={onAutoIndentEnter}
            />
          </div>

          <div className="min-w-0 space-y-6">
            <LinkedPipelinesPanel
              linkedPipelines={linkedPipelines}
              pipelineMetadata={pipelineMetadata}
              pipelineSourceIndex={pipelineSourceIndex}
              onOpenPipeline={onOpenPipeline}
            />

            <TriggerRecentRuns
              runs={recentRuns}
              loading={runsLoading}
              error={runsError}
              scrollable={runsScrollable}
              listRef={recentRunsListRef}
              onScroll={onRecentRunsScroll}
              onOpenRun={onOpenRun}
            />
          </div>
        </div>
      </div>
    </div>
  );
}

function LinkedPipelinesPanel({
  linkedPipelines,
  pipelineMetadata,
  pipelineSourceIndex,
  onOpenPipeline,
}: {
  linkedPipelines: PipelineRef[];
  pipelineMetadata: Map<string, PipelineMeta>;
  pipelineSourceIndex: Map<string, string> | null;
  onOpenPipeline: (identifier: string) => void;
}) {
  return (
    <div className="glass-card overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]" style={{ marginTop: '9px' }}>
        <h3 className="text-lg font-semibold text-[var(--text-primary)]">Linked Pipelines</h3>
      </div>
      <div className="p-4">
        {linkedPipelines.length ? (
          <ul className={`triggers-pipeline-list ${linkedPipelines.length > 5 ? 'triggers-pipelines-scroll' : ''}`}>
            {linkedPipelines.map(pipeline => {
              const meta = pipelineMetadata.get(pipeline.identifier);
              const sourceKeyLocal = meta?.sourceKey || pipelineSourceIndex?.get(pipeline.identifier) || 'local';
              const canNavigate = sourceKeyLocal !== 'local';
              return (
                <li key={`pipe-${pipeline.identifier}`} className={`triggers-pipeline-item ${canNavigate ? '' : 'triggers-pipeline-item--local'}`}>
                  <button
                    type="button"
                    className={`triggers-pipeline-link ${canNavigate ? '' : 'triggers-pipeline-link--local'}`}
                    onClick={canNavigate ? () => onOpenPipeline(pipeline.identifier) : undefined}
                    disabled={!canNavigate}
                    title={canNavigate ? `Open ${pipeline.identifier}` : 'Pipeline not available in the pipeline catalog yet.'}
                  >
                    <span className="triggers-pipeline-name">{pipeline.display}</span>
                    <dl className="triggers-detail-grid triggers-pipeline-details">
                      <dt className="triggers-detail-label">Path:</dt>
                      <dd className="triggers-detail-value">{pipeline.pathLabel === 'root' ? '/' : `/${pipeline.pathLabel}`}</dd>
                      <dt className="triggers-detail-label">Version:</dt>
                      <dd className="triggers-detail-value">{meta?.version || 'latest'}</dd>
                      <dt className="triggers-detail-label">Source:</dt>
                      <dd className="triggers-detail-value">{meta?.sourceLabel || sourceLabel(sourceKeyLocal)}</dd>
                    </dl>
                  </button>
                </li>
              );
            })}
          </ul>
        ) : (
          <p className="text-sm text-[var(--text-secondary)]">No pipelines referenced yet.</p>
        )}
      </div>
    </div>
  );
}
