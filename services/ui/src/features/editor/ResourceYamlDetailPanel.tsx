import { useCallback, useEffect, useState, type KeyboardEvent, type RefObject, type UIEvent } from 'react';
import { AlertTriangle, CheckCircle2, Copy, Download, Maximize2, Minimize2 } from 'lucide-react';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import type { ResourceAccessResourceType } from '../../components/ResourceAccessCard';
import { renderYamlHighlight, renderYamlLines } from '../../lib/yamlRenderer';
import { EditorAutocompleteMenu, type EditorAutocompleteSuggestion } from './EditorAutocompleteMenu';
import type { YamlValidationError } from './YamlValidationPanel';
import { YamlEditorFullscreenDialog } from './YamlEditorFullscreenDialog';
import { YamlEditorToolbox } from './YamlEditorToolbox';
import { insertYamlSnippetAtCursor, type YamlEditorResourceKind } from './yamlToolboxModel';

type ResourceYamlAccess = {
  resourceType: ResourceAccessResourceType;
  resourceID: string;
  label: string;
} | null;

export type YamlEditorTextChangeOptions = {
  openSuggestion?: boolean;
};

type ResourceYamlDetailPanelProps = {
  resourceKind?: YamlEditorResourceKind;
  title: string;
  rawYaml: string;
  isEditing: boolean;
  editorValue: string;
  validationErrors: YamlValidationError[];
  validationErrorLines: Set<number>;
  editorSuggestion: EditorAutocompleteSuggestion | null;
  autocompleteLoading: boolean;
  editorRef: RefObject<HTMLTextAreaElement | null>;
  highlightContentRef: RefObject<HTMLPreElement | null>;
  lineNumbersRef: RefObject<HTMLDivElement | null>;
  ids: {
    content: string;
    editorContainer: string;
    lineNumbers: string;
    stage: string;
    highlight: string;
    editor: string;
    validation: string;
    autocomplete: string;
  };
  editorLabel: string;
  access: ResourceYamlAccess;
  canUpdate: boolean;
  canCreate: boolean;
  isGitSource: boolean;
  saving: boolean;
  saveBlocked?: boolean;
  showActions?: boolean;
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
  onIndentTab?: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
};

export function ResourceYamlDetailPanel({
  resourceKind = 'pipeline',
  title,
  rawYaml,
  isEditing,
  editorValue,
  validationErrors,
  validationErrorLines,
  editorSuggestion,
  autocompleteLoading,
  editorRef,
  highlightContentRef,
  lineNumbersRef,
  ids,
  editorLabel,
  access,
  canUpdate,
  canCreate,
  isGitSource,
  saving,
  saveBlocked,
  showActions = true,
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
}: ResourceYamlDetailPanelProps) {
  const editorLines = editorValue.split('\n');
  const saveDisabled = saving || (saveBlocked ?? validationErrors.length > 0);
  const [editorExpanded, setEditorExpanded] = useState(false);
  const activeEditorExpanded = isEditing && editorExpanded;
  const closeExpandedEditor = useCallback(() => setEditorExpanded(false), []);

  const handleInsertSnippet = useCallback(
    (snippet: string) => {
      const editor = editorRef.current;
      const start = editor?.selectionStart ?? editorValue.length;
      const end = editor?.selectionEnd ?? start;
      const { nextValue, nextCursor } = insertYamlSnippetAtCursor(editorValue, start, end, snippet);
      onDismissSuggestion();
      onEditorTextChange(nextValue, nextCursor);
      requestAnimationFrame(() => {
        const el = editorRef.current;
        if (!el) return;
        el.focus();
        el.selectionStart = nextCursor;
        el.selectionEnd = nextCursor;
      });
    },
    [editorRef, editorValue, onDismissSuggestion, onEditorTextChange]
  );

  const handleEdit = useCallback(() => {
    setEditorExpanded(false);
    onEdit();
  }, [onEdit]);

  const handleDiscard = useCallback(() => {
    setEditorExpanded(false);
    onDiscard();
  }, [onDiscard]);

  useEffect(() => {
    if (!activeEditorExpanded) return undefined;
    const frame = window.requestAnimationFrame(() => {
      editorRef.current?.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [activeEditorExpanded, editorRef]);

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>, suggestionsVisible: boolean) => {
    if (event.ctrlKey && event.code === 'Space') {
      event.preventDefault();
      const cursor = event.currentTarget.selectionStart || 0;
      if (!suggestionsVisible) {
        onDismissSuggestion();
      } else if (editorSuggestion) {
        onDismissSuggestion();
      } else {
        onOpenSuggestion(cursor, { force: true });
      }
      return;
    }

    if (suggestionsVisible && editorSuggestion && event.altKey && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      event.preventDefault();
      onMoveSuggestion(event.key === 'ArrowDown' ? 1 : -1);
      return;
    }

    if (suggestionsVisible && editorSuggestion && event.key === 'Tab') {
      event.preventDefault();
      const selectedSuggestion = editorSuggestion.items[editorSuggestion.activeIndex];
      if (selectedSuggestion) onSelectSuggestion(selectedSuggestion);
      return;
    }

    if (suggestionsVisible && editorSuggestion && event.key === 'Escape') {
      event.preventDefault();
      onDismissSuggestion();
      return;
    }

    if (event.key === 'Tab' && onIndentTab) {
      event.preventDefault();
      onIndentTab(event);
      return;
    }

    if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
      event.preventDefault();
      onAutoIndentEnter(event);
    }
  };

  const renderEditorWorkspace = (fullscreen: boolean) => (
    <div className={`resource-yaml-editor-shell ${fullscreen ? 'resource-yaml-editor-shell--expanded resource-yaml-editor-shell--fullscreen' : 'resource-yaml-editor-shell--normal'}`}>
      <div id={ids.editorContainer} className="editor-container">
        <div id={ids.lineNumbers} ref={lineNumbersRef}>
          <div className="line-number-track">
            {editorLines.map((_, index) => (
              <div key={`ln-${index}`} className={`line-number ${validationErrorLines.has(index + 1) ? 'line-number--error' : ''}`}>
                {index + 1}
              </div>
            ))}
          </div>
        </div>
        <div id={ids.stage} className="yaml-editor-stage yaml-editor-stage--with-highlight">
          <div id={ids.highlight} className="yaml-editor-highlight" aria-hidden="true">
            <pre ref={highlightContentRef} className="yaml-editor-highlight__content">
              {renderYamlHighlight(editorValue)}
            </pre>
          </div>
          <textarea
            ref={editorRef}
            id={ids.editor}
            aria-label={editorLabel}
            aria-describedby={fullscreen ? undefined : ids.validation}
            aria-invalid={validationErrors.length > 0}
            aria-autocomplete="list"
            aria-controls={fullscreen && editorSuggestion ? ids.autocomplete : undefined}
            aria-activedescendant={fullscreen && editorSuggestion ? `${ids.autocomplete}-option-${editorSuggestion.activeIndex}` : undefined}
            value={editorValue}
            onChange={event => onEditorTextChange(event.target.value, event.target.selectionStart || 0, { openSuggestion: fullscreen })}
            onClick={event => {
              if (fullscreen) {
                onOpenSuggestion(event.currentTarget.selectionStart || 0);
              } else {
                onDismissSuggestion();
              }
            }}
            onScroll={onEditorScroll}
            onKeyDown={event => handleKeyDown(event, fullscreen)}
            spellCheck={false}
          />
        </div>
      </div>
      {fullscreen ? (
        <YamlEditorToolbox
          resourceKind={resourceKind}
          suggestionSlot={
            editorSuggestion ? (
              <EditorAutocompleteMenu
                id={ids.autocomplete}
                suggestion={editorSuggestion}
                loading={autocompleteLoading}
                placement="inline"
                onSelect={onSelectSuggestion}
              />
            ) : (
              <p className="yaml-editor-toolbox__empty">Use Ctrl+Space while editing to show contextual completions here.</p>
            )
          }
          onInsertSnippet={handleInsertSnippet}
        />
      ) : (
        <div id={ids.validation} className="sr-only" role="status" aria-live="polite">
          {validationErrors.length ? `${validationErrors.length} YAML validation issue${validationErrors.length === 1 ? '' : 's'}` : 'YAML valid'}
        </div>
      )}
    </div>
  );

  return (
    <div className={`glass-card overflow-hidden resource-yaml-detail-card ${activeEditorExpanded ? 'resource-yaml-detail-card--expanded' : ''}`}>
      <div className="resource-yaml-detail-card__head">
        <div className="resource-yaml-detail-card__title-row">
          <h3>{title}</h3>
          <InlineYamlValidationStatus errors={validationErrors} />
        </div>
        {isEditing && !showActions ? (
          <button
            className="glass-button-ghost"
            type="button"
            onClick={() => setEditorExpanded(current => !current)}
            title={activeEditorExpanded ? 'Collapse YAML editor' : 'Expand YAML editor'}
            aria-label={activeEditorExpanded ? 'Collapse YAML editor' : 'Expand YAML editor'}
            aria-expanded={activeEditorExpanded}
          >
            {activeEditorExpanded ? <Minimize2 className="h-4 w-4" aria-hidden="true" /> : <Maximize2 className="h-4 w-4" aria-hidden="true" />}
          </button>
        ) : null}
        {showActions ? (
          <div className="flex items-center gap-2 flex-wrap">
            {!isEditing ? (
              <>
                <button className="glass-button-ghost" onClick={onCopy} title="Copy YAML" aria-label="Copy YAML">
                  <Copy className="h-4 w-4" aria-hidden="true" />
                </button>
                <button className="glass-button-ghost" onClick={onDownload} title="Download YAML" aria-label="Download YAML">
                  <Download className="h-4 w-4" aria-hidden="true" />
                </button>
                {access ? <ResourceAccessCard resourceType={access.resourceType} resourceID={access.resourceID} label={access.label} /> : null}
                {!canUpdate && !canCreate ? null : (
                  <>
                    {canUpdate ? (
                      <button className="glass-button-primary" onClick={handleEdit}>
                        Edit
                      </button>
                    ) : null}
                    {canCreate ? (
                      <button className="glass-button-subtle" onClick={onClone}>
                        Clone
                      </button>
                    ) : null}
                  </>
                )}
              </>
            ) : canUpdate ? (
              <>
                <button
                  className="glass-button-ghost"
                  type="button"
                  onClick={() => setEditorExpanded(current => !current)}
                  title={activeEditorExpanded ? 'Collapse YAML editor' : 'Expand YAML editor'}
                  aria-label={activeEditorExpanded ? 'Collapse YAML editor' : 'Expand YAML editor'}
                  aria-expanded={activeEditorExpanded}
                >
                  {activeEditorExpanded ? <Minimize2 className="h-4 w-4" aria-hidden="true" /> : <Maximize2 className="h-4 w-4" aria-hidden="true" />}
                </button>
                <button className="glass-button-ghost" onClick={handleDiscard}>
                  Discard
                </button>
                <button className="glass-button-primary" onClick={onSave} disabled={saveDisabled}>
                  {saving ? 'Saving...' : 'Save'}
                </button>
              </>
            ) : null}
          </div>
        ) : null}
      </div>
      <div className="p-4 space-y-3">
        {isGitSource ? (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
            Editing here saves a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
          </div>
        ) : null}
        {!isEditing ? (
          <div id={ids.content} className="yaml-view">
            {renderYamlLines(rawYaml)}
          </div>
        ) : (
          <>
            {activeEditorExpanded ? (
              <div className="resource-yaml-expanded-placeholder">
                Expanded YAML editor is open.
              </div>
            ) : renderEditorWorkspace(false)}
            {activeEditorExpanded ? (
              <YamlEditorFullscreenDialog
                title={title}
                subtitle="Fullscreen YAML authoring"
                validationErrors={validationErrors}
                onClose={closeExpandedEditor}
                actions={
                  canUpdate ? (
                    <>
                      <button type="button" className="glass-button-ghost" onClick={handleDiscard}>
                        Discard
                      </button>
                      <button type="button" className="glass-button-primary" onClick={onSave} disabled={saveDisabled}>
                        {saving ? 'Saving...' : 'Save'}
                      </button>
                    </>
                  ) : null
                }
              >
                {isGitSource ? (
                  <div className="yaml-editor-fullscreen-modal__notice">
                    Editing here saves a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
                  </div>
                ) : null}
                {renderEditorWorkspace(true)}
              </YamlEditorFullscreenDialog>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}

function InlineYamlValidationStatus({ errors }: { errors: YamlValidationError[] }) {
  if (!errors.length) {
    return (
      <span className="resource-yaml-validation-chip resource-yaml-validation-chip--valid" role="status" aria-live="polite">
        <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
        YAML valid
      </span>
    );
  }

  const label = `${errors.length} issue${errors.length === 1 ? '' : 's'}`;
  return (
    <span className="resource-yaml-validation-chip resource-yaml-validation-chip--invalid" role="status" aria-live="polite">
      <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
      {label}
    </span>
  );
}
