import { useEffect, useState, type KeyboardEvent, type RefObject, type UIEvent } from 'react';
import { ArrowLeft, Clock3, Copy, Download, FileCode2, Layers3, Save, SquarePen, Workflow, X } from 'lucide-react';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import type { YamlEditorTextChangeOptions } from '../editor/ResourceYamlDetailPanel';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import type { StepUsageItem } from './api';
import { formatUpdatedAt, normalizeSource, type StepDetail } from './model';
import {
  StepDefinitionPanel,
  StepDetailTabPanel,
  StepMetaItem,
  type StepDetailTabID,
} from './StepDetailSections';
import { StepUsagePanel } from './StepUsagePanel';
import { formatStepDetailPath, formatStepDetailSource } from './stepDetailPresentation';

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
  editableStepName: string;
  editableStepTeam: string;
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
  editableStepName,
  editableStepTeam,
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
}: StepDetailViewProps) {
  const [activeTab, setActiveTab] = useState<StepDetailTabID>('definition');

  useEffect(() => {
    if (isEditing) setActiveTab('definition');
  }, [isEditing]);

  if (!detail) {
    return (
      <div id="steps-detail-view" className="pipelines-view pipelines-detail-shell">
        <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a step to see details.</div>
      </div>
    );
  }

  const source = normalizeSource(detail.source);
  const sourceState = formatStepDetailSource(detail.source);
  const isGitSource = source === 'git';
  const updatedLabel = source === 'draft' ? 'Draft' : formatUpdatedAt(detail.updatedAt);
  const saveDisabled = saving || (saveBlocked ?? validationErrors.length > 0);
  const canPersistCurrentStep = source === 'draft' ? canCreateStepHere : canUpdateSelectedStep;
  const tabs: Array<{ id: StepDetailTabID; label: string; count?: number }> = [
    { id: 'definition', label: 'Definition' },
    { id: 'usage', label: 'Used in pipelines', count: usage.length },
  ];

  const openDefinitionForEdit = () => {
    setActiveTab('definition');
    if (!isEditing) onEdit();
  };

  return (
    <div id="steps-detail-view" className="pipelines-view pipelines-detail-shell">
      <section className="pipeline-detail-hero" aria-labelledby="step-detail-name">
        <div className="pipeline-detail-hero__main">
          <div className="pipeline-detail-back-row">
            <button id="steps-back-btn" type="button" className="pipeline-detail-back" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>Back</span>
            </button>
            <span className="pipeline-detail-back-context" title={formatStepDetailPath(detail)}>
              {formatStepDetailPath(detail)}
            </span>
          </div>
          <div className="pipeline-detail-title-row">
            <h2 id="step-detail-name">{detail.name || detail.id}</h2>
            <span className={`pipeline-detail-source pipeline-detail-source--${sourceState.tone}`}>
              <span aria-hidden="true" />
              {sourceState.label}
            </span>
          </div>
          <p id="step-detail-description" className="pipeline-detail-description">
            {detail.description || 'No description provided.'}
          </p>
          <div className="pipeline-detail-meta-line" aria-label="Step metadata">
            <StepMetaItem id="step-detail-path" icon={<FileCode2 className="h-3.5 w-3.5" aria-hidden="true" />} label="Team" value={formatStepDetailPath(detail)} />
            <StepMetaItem id="step-detail-identifier" icon={<Workflow className="h-3.5 w-3.5" aria-hidden="true" />} label="Identifier" value={detail.id} />
            <StepMetaItem id="step-detail-source" icon={<Layers3 className="h-3.5 w-3.5" aria-hidden="true" />} label="Source" value={sourceState.label} />
            <StepMetaItem id="step-detail-updated" icon={<Clock3 className="h-3.5 w-3.5" aria-hidden="true" />} label="Updated" value={updatedLabel} />
          </div>
        </div>

        <div className="pipeline-detail-hero__actions" aria-label="Step actions">
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
                  resourceType="step"
                  resourceID={detail.id}
                  label="step"
                  buttonClassName="glass-button-ghost pipeline-detail-action"
                />
              ) : null}
              {canCreateStepHere ? (
                <button type="button" className="glass-button-ghost pipeline-detail-action" onClick={onClone}>
                  <Copy className="h-4 w-4" aria-hidden="true" />
                  <span>Clone</span>
                </button>
              ) : null}
              {canUpdateSelectedStep ? (
                <button
                  type="button"
                  className="glass-button-ghost pipeline-detail-action"
                  onClick={openDefinitionForEdit}
                  title="Edit step"
                >
                  <SquarePen className="h-4 w-4" aria-hidden="true" />
                  <span>Edit</span>
                </button>
              ) : null}
            </>
          ) : canPersistCurrentStep ? (
            <>
              <button type="button" className="glass-button-ghost pipeline-detail-action" onClick={onDiscard}>
                <X className="h-4 w-4" aria-hidden="true" />
                <span>Discard</span>
              </button>
              <button
                type="button"
                className="glass-button-primary pipeline-detail-action"
                onClick={onSave}
                disabled={saveDisabled}
              >
                <Save className="h-4 w-4" aria-hidden="true" />
                <span>{saving ? 'Saving...' : 'Save'}</span>
              </button>
            </>
          ) : null}
        </div>
      </section>

      <div className="pipeline-detail-tabs-wrap">
        <div className="pipeline-detail-tabs" role="tablist" aria-label="Step detail sections">
          {tabs.map(tab => (
            <button
              key={tab.id}
              type="button"
              id={`step-detail-tab-${tab.id}`}
              role="tab"
              aria-selected={activeTab === tab.id}
              aria-controls={`step-detail-panel-${tab.id}`}
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
        {activeTab === 'definition' ? (
          <StepDefinitionPanel
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
            canUpdateSelectedStep={canUpdateSelectedStep}
            canCreateStepHere={canCreateStepHere}
            saving={saving}
            saveBlocked={saveBlocked}
            editableStepName={editableStepName}
            editableStepTeam={editableStepTeam}
            usage={usage}
            usageLoading={usageLoading}
            usageError={usageError}
            onCopy={onCopy}
            onDownload={onDownload}
            onEdit={onEdit}
            onClone={onClone}
            onDiscard={onDiscard}
            onSave={onSave}
            onCopyIdentifier={onCopyIdentifier}
            onEditableStepNameChange={onEditableStepNameChange}
            onEditableStepTeamChange={onEditableStepTeamChange}
            onEditorTextChange={onEditorTextChange}
            onOpenSuggestion={onOpenSuggestion}
            onMoveSuggestion={onMoveSuggestion}
            onDismissSuggestion={onDismissSuggestion}
            onSelectSuggestion={onSelectSuggestion}
            onEditorScroll={onEditorScroll}
            onAutoIndentEnter={onAutoIndentEnter}
          />
        ) : null}

        {activeTab === 'usage' ? (
          <StepDetailTabPanel id="usage" title="Used in pipelines" tabLabel="Pipeline usage">
            <StepUsagePanel usage={usage} loading={usageLoading} error={usageError} />
          </StepDetailTabPanel>
        ) : null}
      </div>
    </div>
  );
}
