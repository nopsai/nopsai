export type EditorAutocompleteSuggestion = {
  title: string;
  items: string[];
  activeIndex: number;
  teamedSections?: Array<{ label: string; items: string[]; totalCount: number }>;
};

export function EditorAutocompleteMenu({
  id = 'editor-autocomplete-listbox',
  suggestion,
  loading,
  width = 320,
  onSelect,
}: {
  id?: string;
  suggestion: EditorAutocompleteSuggestion;
  loading: boolean;
  width?: number;
  onSelect: (item: string) => void;
}) {
  let runningIndex = 0;
  return (
    <div
      className="pipeline-suggestion-overlay"
      id={id}
      style={{ width, maxWidth: 'calc(100% - 32px)', right: 16, bottom: 16, top: 'auto', left: 'auto' }}
      role="listbox"
      aria-label={`${suggestion.title} autocomplete`}
    >
      <div className="scope-suggestion-panel">
        <div className="scope-suggestion-heading">
          <p className="scope-suggestion-kicker">Autocomplete</p>
          <p className="scope-suggestion-title">{suggestion.title}</p>
          <p className="scope-suggestion-subtitle">
            Ctrl+Space - Enter to insert - Esc to close
            {loading ? ' - Loading...' : ''}
          </p>
        </div>
        <div className="scope-suggestion-body">
          {suggestion.items.length ? (
            <div className="scope-suggestion-list">
              {suggestion.teamedSections?.length
                ? suggestion.teamedSections.map(section => {
                    const startIndex = runningIndex;
                    runningIndex += section.items.length;
                    const remaining = Math.max(0, section.totalCount - section.items.length);
                    const hasActive =
                      suggestion.activeIndex >= startIndex && suggestion.activeIndex < startIndex + section.items.length;
                    return (
                      <article
                        key={section.label}
                        className={`scope-suggestion-item ${hasActive ? 'scope-suggestion-item--active' : ''}`}
                      >
                        <div className="scope-suggestion-scope">
                          <span className="scope-suggestion-scope-label">{section.label}</span>
                          <span className="scope-suggestion-scope-count">
                            {section.totalCount} {section.totalCount === 1 ? 'item' : 'items'}
                          </span>
                        </div>
                        <div className="scope-suggestion-variables">
                          {section.items.map((item, index) => {
                            const globalIndex = startIndex + index;
                            return (
                              <button
                                key={`${section.label}-${item}-${index}`}
                                type="button"
                                role="option"
                                id={`${id}-option-${globalIndex}`}
                                aria-selected={suggestion.activeIndex === globalIndex}
                                className={`scope-suggestion-pill scope-suggestion-pill--action ${
                                  suggestion.activeIndex === globalIndex ? 'scope-suggestion-pill--active' : ''
                                }`}
                                onClick={() => onSelect(item)}
                              >
                                {item}
                              </button>
                            );
                          })}
                          {remaining > 0 ? (
                            <span className="scope-suggestion-pill scope-suggestion-pill--more">+{remaining} more</span>
                          ) : null}
                        </div>
                      </article>
                    );
                  })
                : suggestion.items.map((item, index) => (
                    <div
                      key={`${item}-${index}`}
                      className={`scope-suggestion-item ${index === suggestion.activeIndex ? 'scope-suggestion-item--active' : ''}`}
                    >
                      <button
                        type="button"
                        role="option"
                        id={`${id}-option-${index}`}
                        aria-selected={index === suggestion.activeIndex}
                        className="scope-suggestion-pill scope-suggestion-pill--action"
                        onClick={() => onSelect(item)}
                      >
                        {item}
                      </button>
                    </div>
                  ))}
            </div>
          ) : (
            <p className="scope-suggestion-empty">No suggestions available.</p>
          )}
        </div>
      </div>
    </div>
  );
}
