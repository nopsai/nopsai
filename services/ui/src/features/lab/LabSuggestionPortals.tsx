import type { RefObject } from 'react';
import { createPortal } from 'react-dom';
import type { LabSuggestionContext, LabSuggestionItem } from '../../lib/lab';

type LabSuggestionPortalsProps = {
  portalBody: HTMLElement | null;
  overlayHost: HTMLElement | null;
  inlineOverlayRef: RefObject<HTMLDivElement | null>;
  suggestionPanelRef: RefObject<HTMLElement | null>;
  inlineSuggestion: string;
  suggestionContext: LabSuggestionContext | null;
  suggestionCopy: { title: string; subtitle: string };
  suggestionItems: LabSuggestionItem[];
  autocompleteLoading: boolean;
  showFloatingPanel?: boolean;
  onApplySuggestion: (item: LabSuggestionItem) => void;
};

export function LabSuggestionPortals({
  portalBody,
  overlayHost,
  inlineOverlayRef,
  suggestionPanelRef,
  inlineSuggestion,
  suggestionContext,
  suggestionCopy,
  suggestionItems,
  autocompleteLoading,
  showFloatingPanel = true,
  onApplySuggestion,
}: LabSuggestionPortalsProps) {
  return (
    <>
      {portalBody
        ? createPortal(
            <div
              id="lab-editor-autocomplete"
              ref={inlineOverlayRef}
              className={`pipeline-editor-autocomplete ${inlineSuggestion ? '' : 'hidden'}`}
              aria-hidden="true"
            >
              <span id="lab-editor-autocomplete-ghost" className="pipeline-editor-autocomplete__ghost">
                {inlineSuggestion}
              </span>
            </div>,
            portalBody
          )
        : null}

      {overlayHost && showFloatingPanel
        ? createPortal(
            <section
              id="lab-suggestion-panel"
              ref={suggestionPanelRef}
              className="scope-suggestion-panel pipeline-suggestion-panel pipeline-suggestion-overlay"
              aria-live="polite"
              data-base-width="260"
            >
              <div className="scope-suggestion-heading">
                <div>
                  <h3 id="lab-suggestion-title" className="scope-suggestion-title">
                    {suggestionContext?.title || suggestionCopy.title}
                  </h3>
                </div>
                <p id="lab-suggestion-subtitle" className="scope-suggestion-subtitle">
                  {suggestionCopy.subtitle}
                </p>
              </div>

              <div className="scope-suggestion-body">
                <p id="lab-suggestion-empty" className={`scope-suggestion-empty ${suggestionItems.length ? 'hidden' : ''}`}>
                  {autocompleteLoading ? 'Loading suggestions…' : 'No suggestions available yet.'}
                </p>
                <div id="lab-suggestion-list" className={`scope-suggestion-list ${suggestionItems.length ? '' : 'hidden'}`}>
                  {suggestionItems.length > 0 && (
                    <article className="scope-suggestion-item">
                      <div className="scope-suggestion-scope">
                        <span className="scope-suggestion-scope-label">{suggestionContext?.title || suggestionCopy.title}</span>
                        <span className="scope-suggestion-scope-count">{suggestionItems.length} items</span>
                      </div>
                      <div className="scope-suggestion-variables">
                        {suggestionItems.map(item => (
                          <button
                            key={`${item.value}`}
                            type="button"
                            className="scope-suggestion-pill scope-suggestion-pill--action"
                            onClick={() => onApplySuggestion(item)}
                          >
                            <span>{item.label ?? item.value}</span>
                            {item.hint && <span className="scope-suggestion-hint">{item.hint}</span>}
                          </button>
                        ))}
                      </div>
                    </article>
                  )}
                </div>
              </div>
            </section>,
            overlayHost
          )
        : null}
    </>
  );
}
