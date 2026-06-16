import type { KeyboardEvent, RefObject, UIEvent } from 'react';
import { Copy, Download } from 'lucide-react';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import { renderYamlHighlight, renderYamlLines } from '../../lib/yamlRenderer';
import { EditorAutocompleteMenu, type EditorAutocompleteSuggestion } from './EditorAutocompleteMenu';
import { YamlValidationPanel, type YamlValidationError } from './YamlValidationPanel';

type ResourceYamlAccess = {
  resourceType: 'pipeline' | 'scope' | 'step' | 'runner' | 'config_repo' | 'knowledge_context';
  resourceID: string;
  label: string;
} | null;

type ResourceYamlDetailPanelProps = {
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
  autocompleteWidth?: number;
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
  onIndentTab?: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  onAutoIndentEnter: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
};

export function ResourceYamlDetailPanel({
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
  autocompleteWidth,
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

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.ctrlKey && event.code === 'Space') {
      event.preventDefault();
      const cursor = event.currentTarget.selectionStart || 0;
      if (editorSuggestion) {
        onDismissSuggestion();
      } else {
        onOpenSuggestion(cursor, { force: true });
      }
      return;
    }

    if (editorSuggestion && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      event.preventDefault();
      onMoveSuggestion(event.key === 'ArrowDown' ? 1 : -1);
      return;
    }

    if (editorSuggestion && event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
      event.preventDefault();
      const selectedSuggestion = editorSuggestion.items[editorSuggestion.activeIndex];
      if (selectedSuggestion) onSelectSuggestion(selectedSuggestion);
      return;
    }

    if (editorSuggestion && event.key === 'Escape') {
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

  return (
    <div className="glass-card overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
        <h3 className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
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
                    <button className="glass-button-primary" onClick={onEdit}>
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
              <button className="glass-button-ghost" onClick={onDiscard}>
                Discard
              </button>
              <button className="glass-button-primary" onClick={onSave} disabled={saving || validationErrors.length > 0}>
                {saving ? 'Saving...' : 'Save'}
              </button>
            </>
          ) : null}
        </div>
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
                aria-describedby={ids.validation}
                aria-invalid={validationErrors.length > 0}
                aria-autocomplete="list"
                aria-controls={editorSuggestion ? ids.autocomplete : undefined}
                aria-activedescendant={editorSuggestion ? `${ids.autocomplete}-option-${editorSuggestion.activeIndex}` : undefined}
                value={editorValue}
                onChange={event => onEditorTextChange(event.target.value, event.target.selectionStart || 0)}
                onClick={event => onOpenSuggestion(event.currentTarget.selectionStart || 0)}
                onScroll={onEditorScroll}
                onKeyDown={handleKeyDown}
                spellCheck={false}
              />
            </div>
            <YamlValidationPanel id={ids.validation} errors={validationErrors} />
            {editorSuggestion ? (
              <EditorAutocompleteMenu
                id={ids.autocomplete}
                suggestion={editorSuggestion}
                loading={autocompleteLoading}
                width={autocompleteWidth}
                onSelect={onSelectSuggestion}
              />
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}
