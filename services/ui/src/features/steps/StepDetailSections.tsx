import type { KeyboardEvent, ReactElement, ReactNode, RefObject, UIEvent } from 'react';
import { Copy } from 'lucide-react';
import { ResourceYamlDetailPanel } from '../editor/ResourceYamlDetailPanel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlEditorTextChangeOptions } from '../editor/ResourceYamlDetailPanel';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import type { StepUsageItem } from './api';
import type { StepDetail } from './model';
import {
  formatStepDetailPath,
  summarizeStepUsageSources,
  type StepDetailSourceState,
} from './stepDetailPresentation';

export type StepDetailTabID = 'definition' | 'usage';

export function StepMetaItem({ id, icon, label, value }: { id: string; icon: ReactElement; label: string; value: string }) {
  return (
    <span className="pipeline-detail-meta-item">
      {icon}
      <span>{label}</span>
      <b id={id}>{value}</b>
    </span>
  );
}

export function StepDetailTabPanel({
  id,
  title,
  tabLabel,
  children,
}: {
  id: StepDetailTabID;
  title: string;
  tabLabel: string;
  children: ReactNode;
}) {
  return (
    <section
      id={`step-detail-panel-${id}`}
      role="tabpanel"
      aria-labelledby={`step-detail-tab-${id}`}
      className="pipeline-detail-panel"
    >
      <div className="pipeline-detail-section-intro">
        <div>
          <h3>{title}</h3>
        </div>
      </div>
      <div aria-label={tabLabel}>{children}</div>
    </section>
  );
}

export function StepDefinitionPanel({
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
  canUpdateSelectedStep,
  canCreateStepHere,
  saving,
  saveBlocked,
  editableStepName,
  editableStepTeam,
  usage,
  usageLoading,
  usageError,
  onCopy,
  onDownload,
  onEdit,
  onClone,
  onDiscard,
  onSave,
  onCopyIdentifier,
  onEditableStepNameChange,
  onEditableStepTeamChange,
  onEditorTextChange,
  onOpenSuggestion,
  onMoveSuggestion,
  onDismissSuggestion,
  onSelectSuggestion,
  onEditorScroll,
  onAutoIndentEnter,
}: {
  detail: StepDetail;
  source: string;
  sourceState: StepDetailSourceState;
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
  canUpdateSelectedStep: boolean;
  canCreateStepHere: boolean;
  saving: boolean;
  saveBlocked?: boolean;
  editableStepName: string;
  editableStepTeam: string;
  usage: StepUsageItem[];
  usageLoading: boolean;
  usageError: string | null;
  onCopy: () => void;
  onDownload: () => void;
  onEdit: () => void;
  onClone: () => void;
  onDiscard: () => void;
  onSave: () => void;
  onCopyIdentifier: (identifier: string) => void | Promise<void>;
  onEditableStepNameChange: (value: string) => void;
  onEditableStepTeamChange: (value: string) => void;
  onEditorTextChange: (nextValue: string, cursor: number, options?: YamlEditorTextChangeOptions) => void;
  onOpenSuggestion: (cursor: number, opts?: { text?: string; force?: boolean }) => void;
  onMoveSuggestion: (direction: 1 | -1) => void;
  onDismissSuggestion: () => void;
  onSelectSuggestion: (value: string) => void;
  onEditorScroll: (event: UIEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
}) {
  return (
    <section
      id="step-detail-panel-definition"
      role="tabpanel"
      aria-labelledby="step-detail-tab-definition"
      className="pipeline-detail-panel"
    >
      <div className="pipeline-detail-definition-layout">
        <div className="pipeline-detail-definition-main">
          <ResourceYamlDetailPanel
            resourceKind="step"
            title="Step Definition (YAML)"
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
              content: 'step-yaml-content',
              editorContainer: 'step-editor-container',
              lineNumbers: 'step-line-numbers',
              stage: 'step-yaml-stage',
              highlight: 'step-yaml-highlight',
              editor: 'step-yaml-editor',
              validation: 'step-validation-status',
              autocomplete: 'step-editor-autocomplete',
            }}
            editorLabel="Step YAML editor"
            access={source !== 'draft' ? { resourceType: 'step', resourceID: detail.id, label: 'step' } : null}
            canUpdate={canUpdateSelectedStep}
            canCreate={canCreateStepHere}
            isGitSource={isGitSource}
            saving={saving}
            saveBlocked={saveBlocked}
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
        <StepDefinitionSidePanel
          detail={detail}
          sourceState={sourceState}
          updatedLabel={updatedLabel}
          isEditing={isEditing}
          editableStepName={editableStepName}
          editableStepTeam={editableStepTeam}
          usage={usage}
          usageLoading={usageLoading}
          usageError={usageError}
          onCopyIdentifier={onCopyIdentifier}
          onEditableStepNameChange={onEditableStepNameChange}
          onEditableStepTeamChange={onEditableStepTeamChange}
        />
      </div>
    </section>
  );
}

function StepDefinitionSidePanel({
  detail,
  sourceState,
  updatedLabel,
  isEditing,
  editableStepName,
  editableStepTeam,
  usage,
  usageLoading,
  usageError,
  onCopyIdentifier,
  onEditableStepNameChange,
  onEditableStepTeamChange,
}: {
  detail: StepDetail;
  sourceState: StepDetailSourceState;
  updatedLabel: string;
  isEditing: boolean;
  editableStepName: string;
  editableStepTeam: string;
  usage: StepUsageItem[];
  usageLoading: boolean;
  usageError: string | null;
  onCopyIdentifier: (identifier: string) => void | Promise<void>;
  onEditableStepNameChange: (value: string) => void;
  onEditableStepTeamChange: (value: string) => void;
}) {
  const usageSummary = summarizeStepUsageSources(usage);
  const usageLabel = usageLoading ? 'Loading' : usageError ? 'Unavailable' : String(usageSummary.total);

  return (
    <aside className="pipeline-detail-definition-side" aria-label="Definition summary">
      <section className="pipeline-detail-side-panel">
        <h3>Identity</h3>
        {isEditing ? (
          <div className="pipeline-detail-identity-fields">
            <label htmlFor="step-edit-team">
              <span>Team</span>
              <input
                id="step-edit-team"
                type="text"
                className="pipelines-input"
                value={editableStepTeam}
                onChange={event => onEditableStepTeamChange(event.target.value)}
                placeholder="library/docker"
              />
            </label>
            <label htmlFor="step-edit-name">
              <span>Name</span>
              <input
                id="step-edit-name"
                type="text"
                className="pipelines-input"
                value={editableStepName}
                onChange={event => onEditableStepNameChange(event.target.value)}
                placeholder="build-image"
              />
            </label>
          </div>
        ) : (
          <dl className="pipeline-detail-key-values">
            <KeyValue label="Team" value={formatStepDetailPath(detail)} />
            <KeyValue label="Name" value={detail.name || detail.id} />
            <KeyValue label="Identifier" value={detail.id} onCopy={() => void onCopyIdentifier(detail.id)} />
          </dl>
        )}
      </section>
      <section className="pipeline-detail-side-panel">
        <h3>Source and sync</h3>
        <dl className="pipeline-detail-key-values">
          <KeyValue label="Source" value={sourceState.label} />
          <KeyValue label="State" value={sourceState.description} />
          <KeyValue label="Updated" value={updatedLabel} />
        </dl>
      </section>
      <section className="pipeline-detail-side-panel">
        <h3>Usage</h3>
        <dl className="pipeline-detail-key-values">
          <KeyValue label="Pipelines" value={usageLabel} />
          <KeyValue label="GitOps" value={String(usageSummary.gitOps)} />
          <KeyValue label="Database" value={String(usageSummary.database)} />
          <KeyValue label="Drafts" value={String(usageSummary.drafts)} />
        </dl>
      </section>
    </aside>
  );
}

function KeyValue({ label, value, onCopy }: { label: string; value: string; onCopy?: () => void }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>
        <span className="pipeline-detail-key-value-content">
          <span className="pipeline-detail-key-value-text">{value}</span>
          {onCopy ? (
            <button
              type="button"
              className="pipeline-detail-key-copy"
              onClick={onCopy}
              title={`Copy ${label.toLowerCase()}`}
              aria-label={`Copy ${label.toLowerCase()} ${value}`}
            >
              <Copy className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          ) : null}
        </span>
      </dd>
    </div>
  );
}
