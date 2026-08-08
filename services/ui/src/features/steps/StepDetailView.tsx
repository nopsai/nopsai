import type { KeyboardEvent, RefObject, UIEvent } from 'react';
import { ArrowLeft } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { ResourceYamlDetailPanel } from '../editor/ResourceYamlDetailPanel';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlEditorTextChangeOptions } from '../editor/ResourceYamlDetailPanel';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import { GLOBAL_RESOURCE_TEAM_PATH } from '../../lib/resourceTeams';
import type { StepUsageItem } from './api';
import { formatUpdatedAt, normalizeSource, type StepDetail } from './model';
import { StepUsagePanel } from './StepUsagePanel';

type StepDetailViewProps = {
  detail: StepDetail | null;
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
  usage: StepUsageItem[];
  usageLoading: boolean;
  usageError: string | null;
  onBack: () => void;
  onCopy: () => void;
  onDownload: () => void;
  onEdit: () => void;
  onClone: () => void;
  onDiscard: () => void;
  onSave: () => void;
  onEditorTextChange: (nextValue: string, cursor: number, options?: YamlEditorTextChangeOptions) => void;
  onOpenSuggestion: (cursor: number, opts?: { text?: string; force?: boolean }) => void;
  onMoveSuggestion: (direction: 1 | -1) => void;
  onDismissSuggestion: () => void;
  onSelectSuggestion: (value: string) => void;
  onEditorScroll: (event: UIEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
};

export function StepDetailView({
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
  canUpdateSelectedStep,
  canCreateStepHere,
  saving,
  saveBlocked,
  usage,
  usageLoading,
  usageError,
  onBack,
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
  onAutoIndentEnter,
}: StepDetailViewProps) {
  if (!detail) {
    return (
      <div id="steps-detail-view" className="pipelines-view">
        <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a step to see details.</div>
      </div>
    );
  }

  const source = normalizeSource(detail.source);
  const sourceLabel = source === 'git' ? 'Git' : source === 'draft' ? 'Draft' : 'Database';
  const isGitSource = source === 'git';
  const updatedLabel = source === 'draft' ? 'Draft' : formatUpdatedAt(detail.updatedAt);
  const pathLabel = detail.path || GLOBAL_RESOURCE_TEAM_PATH;

  return (
    <div id="steps-detail-view" className="pipelines-view">
      <div className="min-w-0 space-y-4">
        <div className="glass-card p-4">
          <div className="flex items-start justify-between gap-4 w-full mb-4">
            <div className="min-w-0 flex items-start gap-3">
              <span className="pipeline-card-icon mt-1" aria-hidden="true">
                <ObjectIcon type="step" />
              </span>
              <div className="min-w-0">
                <h2 id="step-detail-name" className="text-xl font-bold text-[var(--text-primary)] truncate">
                  {detail.name || detail.id}
                </h2>
                <p id="step-detail-description" className="text-sm text-[var(--text-secondary)] mt-1">
                  {detail.description || 'No description provided.'}
                </p>
                <div className="pipeline-detail-meta">
                  <dl className="pipeline-detail-grid">
                    <dt className="pipeline-detail-label">Identifier:</dt>
                    <dd className="pipeline-detail-value" id="step-detail-identifier">{detail.id}</dd>
                    <dt className="pipeline-detail-label">Path:</dt>
                    <dd className="pipeline-detail-value" id="step-detail-path">{pathLabel}</dd>
                    <dt className="pipeline-detail-label">Source:</dt>
                    <dd className="pipeline-detail-value" id="step-detail-source">{sourceLabel}</dd>
                    <dt className="pipeline-detail-label">Last updated:</dt>
                    <dd className="pipeline-detail-value" id="step-detail-updated">{updatedLabel}</dd>
                  </dl>
                </div>
              </div>
            </div>
            <button id="steps-back-btn" type="button" className="glass-button-ghost" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>Back to list</span>
            </button>
          </div>
        </div>

        <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
          <div className="min-w-0 space-y-4">
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
          <StepUsagePanel usage={usage} loading={usageLoading} error={usageError} />
        </div>
      </div>
    </div>
  );
}
