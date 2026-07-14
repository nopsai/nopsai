import type { KeyboardEvent, ReactNode, RefObject, UIEvent } from 'react';
import { ArrowLeft, FileCode2, GitBranch, Trash2 } from 'lucide-react';
import { ResourceYamlDetailPanel } from '../editor/ResourceYamlDetailPanel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import { TriggerRecentRuns } from './TriggerRecentRuns';
import {
  TRIGGER_PROVIDERS,
  normalizeSource,
  sourceLabel,
  triggerDetailsWithProvider,
  triggerAllowlistStatusLabel,
  triggerIngressLabel,
  triggerTeamLabel,
  triggerWebhookSourceOptionLabel,
  type TriggerDetailsFormState,
  type TriggerProvider,
  type TriggerWebhookSourceOption,
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
  canDeleteSelectedTrigger: boolean;
  saving: boolean;
  triggerDetails: TriggerDetailsFormState;
  teamPaths: string[];
  webhookSources: TriggerWebhookSourceOption[];
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
  onDelete: () => void;
  onDiscard: () => void;
  onSave: () => void;
  onTriggerDetailsChange: (details: TriggerDetailsFormState) => void;
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
  canDeleteSelectedTrigger,
  saving,
  triggerDetails,
  teamPaths,
  webhookSources,
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
  onDelete,
  onDiscard,
  onSave,
  onTriggerDetailsChange,
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
      <div id="triggers-detail-view" className="triggers-detail-pane-empty">
        <span className="triggers-empty-icon" aria-hidden="true">
          <FileCode2 className="h-5 w-5" />
        </span>
        <strong>Select a trigger</strong>
        <span>No trigger selected.</span>
      </div>
    );
  }

  const sourceKey = normalizeSource(detail.source);
  const isGitSource = sourceKey === 'git';
  const detailName = detail.slug.split('/').filter(Boolean).pop() || detail.slug;
  const detailOwner = detail.slug.split('/').filter(Boolean).slice(0, -1).join('/') || 'root';
  const events = detail.summary.events.length ? detail.summary.events : ['N/A'];
  const branches = detail.summary.branches.length ? detail.summary.branches.join(', ') : 'Any branch';
  const scopes = detail.summary.scopes.length ? detail.summary.scopes : [''];
  const provider = (detail.provider || 'github').trim().toLowerCase();
  const ingress = triggerIngressLabel(detail);
  const allowlistStatus = triggerAllowlistStatusLabel(detail.allowlistStatus);
  const showIngressWarning = provider !== 'github' && detail.allowlistStatus !== 'allowed';
  const teamOptions = uniqueTeamOptions([...teamPaths, triggerDetails.teamPath, detail.teamPath || 'root']);
  const compatibleWebhookSources = webhookSources.filter(source => source.provider === triggerDetails.provider);

  return (
    <div id="triggers-detail-view" className="triggers-detail-pane">
      <div className="triggers-detail-scroll">
        <div className="triggers-detail-head">
          <span className="triggers-detail-icon" aria-hidden="true">
            <GitBranch className="h-5 w-5" />
          </span>
          <div className="triggers-detail-title">
            <h2 id="triggers-detail-name">{detailName}</h2>
            <p>{detail.slug}</p>
          </div>
          <span className={`triggers-badge triggers-badge--${sourceKey === 'git' ? 'blue' : 'neutral'}`}>
            <span className="triggers-badge-dot" aria-hidden="true"></span>
            {sourceLabel(sourceKey)}
          </span>
          <div className="triggers-detail-actions">
            <button id="triggers-back-btn" type="button" className="triggers-mini-button" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>List</span>
            </button>
            {canDeleteSelectedTrigger ? (
              <button
                type="button"
                className="triggers-mini-button triggers-mini-button--danger"
                onClick={onDelete}
                title={isGitSource ? 'Delete database row; GitOps can recreate it on the next sync' : 'Delete trigger'}
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" />
                <span>Delete</span>
              </button>
            ) : null}
          </div>
        </div>

        <div className="triggers-detail-page-grid">
          <div className="triggers-detail-column">
            <section className="triggers-detail-panel" aria-labelledby="triggers-overview-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="triggers-overview-heading">Overview</h3>
              </div>
              <div className="triggers-detail-panel-body">
                <div className="triggers-facts-grid">
                  <TriggerFact label="Owner" value={detailOwner} />
                  {isEditing ? (
                    <TriggerFactField label="Provider">
                      <select
                        className="pipelines-input w-full"
                        value={triggerDetails.provider}
                        onChange={event =>
                          onTriggerDetailsChange(triggerDetailsWithProvider(triggerDetails, event.target.value as TriggerProvider))
                        }
                        disabled={saving}
                      >
                        {TRIGGER_PROVIDERS.map(option => (
                          <option key={option} value={option}>{option}</option>
                        ))}
                      </select>
                    </TriggerFactField>
                  ) : (
                    <TriggerFact label="Provider" value={provider} />
                  )}
                  {isEditing ? (
                    <TriggerFactField label="Team">
                      <select
                        className="pipelines-input w-full"
                        value={triggerDetails.teamPath}
                        onChange={event => onTriggerDetailsChange({ ...triggerDetails, teamPath: event.target.value })}
                        disabled={saving}
                      >
                        {teamOptions.map(path => (
                          <option key={path} value={path}>{path === 'root' ? 'Workspace' : path}</option>
                        ))}
                      </select>
                    </TriggerFactField>
                  ) : (
                    <TriggerFact label="Team" value={triggerTeamLabel(detail.teamPath)} />
                  )}
                  <TriggerFact label="Ingress" value={ingress} />
                  <TriggerFact label="Allowlist" value={allowlistStatus} />
                  {isEditing ? (
                    <TriggerFactField label="Webhook source">
                      <select
                        className="pipelines-input w-full font-mono"
                        value={triggerDetails.webhookSourceID}
                        onChange={event => onTriggerDetailsChange({ ...triggerDetails, webhookSourceID: event.target.value })}
                        disabled={saving || triggerDetails.provider === 'github'}
                        required={triggerDetails.provider !== 'github'}
                      >
                        {triggerDetails.provider === 'github' ? (
                          <option value="">GitHub App automatic</option>
                        ) : (
                          <>
                            <option value="">Select webhook source</option>
                            {compatibleWebhookSources.map(source => (
                              <option key={source.id} value={source.id}>{triggerWebhookSourceOptionLabel(source)}</option>
                            ))}
                            {triggerDetails.webhookSourceID && !compatibleWebhookSources.some(source => source.id === triggerDetails.webhookSourceID) ? (
                              <option value={triggerDetails.webhookSourceID}>{triggerDetails.webhookSourceID}</option>
                            ) : null}
                          </>
                        )}
                      </select>
                    </TriggerFactField>
                  ) : null}
                  <TriggerFact label="Rules" value={String(detail.summary.triggerCount)} />
                  <TriggerFact label="Pipelines" value={String(linkedPipelines.length)} />
                  <TriggerFact label="Branches" value={branches} />
                </div>
                {showIngressWarning ? (
                  <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
                    Repository webhook events will not start pipelines until the trigger is assigned to a compatible webhook source whose allowlist includes this repository.
                  </div>
                ) : null}

                <div className="triggers-detail-section">
                  <div className="triggers-detail-section-title">
                    <h3>Event binding</h3>
                  </div>
                  <div className="triggers-chip-row">
                    {events.map(event => (
                      <span key={`event-${event}`} className="triggers-chip">{event}</span>
                    ))}
                  </div>
                </div>

                <div className="triggers-detail-section">
                  <div className="triggers-detail-section-title">
                    <h3>Scopes</h3>
                  </div>
                  <div className="triggers-chip-row">
                    {scopes.map(scope => (
                      <button key={`scope-${scope || 'default'}`} type="button" className="triggers-chip triggers-chip--button" onClick={() => onOpenScope(scope)}>
                        {scope ? `/${scope}` : 'Default'}
                      </button>
                    ))}
                  </div>
                </div>

                <div className="triggers-detail-section">
                  <div className="triggers-detail-section-title">
                    <h3>Authorization</h3>
                  </div>
                  <div className="triggers-facts-grid">
                    <TriggerFact label="Runtime caller" value={detail.repositoryForWebhook || detail.slug} />
                    <TriggerFact label="Same-team resources" value="Available" />
                    <TriggerFact label="Cross-team resources" value="Explicit sharing required" />
                  </div>
                </div>
              </div>
            </section>

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

          <div className="triggers-detail-column triggers-detail-column--definition">
            <ResourceYamlDetailPanel
              title="Trigger definition"
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
              canUpdate={isEditing ? canUpdateSelectedTrigger : true}
              canCreate={isEditing ? canCreateTriggerHere : true}
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
        </div>
      </div>
    </div>
  );
}

function TriggerFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="triggers-fact">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function TriggerFactField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="triggers-fact triggers-fact--field">
      <span>{label}</span>
      <strong>{children}</strong>
    </label>
  );
}

function uniqueTeamOptions(paths: string[]): string[] {
  const normalized = paths
    .map(path => String(path || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/'))
    .map(path => path && path.toLowerCase() !== 'root' ? path : 'root');
  return Array.from(new Set(['root', ...normalized])).sort((left, right) => {
    if (left === 'root') return -1;
    if (right === 'root') return 1;
    return left.localeCompare(right);
  });
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
      <div className="triggers-linked-pipelines-header flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
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
