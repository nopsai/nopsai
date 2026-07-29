import { useRef, useState, type ReactNode } from 'react';
import { Plus, Search, X } from 'lucide-react';
import { EventAutomationSwitch, type EventAutomationResource } from './EventAutomationSwitch';

type EventAutomationToolbarProps = {
  active: EventAutomationResource;
  title?: string;
  description?: string;
  searchLabel: string;
  searchTerm: string;
  canCreate: boolean;
  createLabel: string;
  onSearchTermChange: (term: string) => void;
  onCreate: () => void;
  createDisabledReason?: string;
  showCreateWhenDisabled?: boolean;
  filters?: ReactNode;
  summary?: ReactNode;
};

export function EventAutomationToolbar({
  active,
  title,
  description,
  searchLabel,
  searchTerm,
  canCreate,
  createLabel,
  createDisabledReason,
  showCreateWhenDisabled = false,
  onSearchTermChange,
  onCreate,
  filters,
  summary,
}: EventAutomationToolbarProps) {
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const hasTitle = Boolean(title || description);
  const searchActive = searchOpen || Boolean(searchTerm.trim());

  return (
    <div className={`triggers-page-toolbar ${hasTitle ? '' : 'triggers-page-toolbar--compact'}`}>
      <div className={`triggers-page-toolbar-head ${hasTitle ? '' : 'triggers-page-toolbar-head--compact'}`}>
        <div className="triggers-page-toolbar-main">
          {hasTitle ? (
            <div className="triggers-page-title">
              <span className="triggers-page-eyebrow">Event automation</span>
              {title ? <h2>{title}</h2> : null}
              {description ? <p>{description}</p> : null}
            </div>
          ) : (
            <EventAutomationSwitch active={active} />
          )}
        </div>
        {summary ? <div className="triggers-page-toolbar-summary">{summary}</div> : null}
        <div className="triggers-page-toolbar-actions">
          <div className={`pipelines-search-shell ${searchActive ? 'open' : ''}`}>
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label={searchLabel}
              onClick={() => {
                setSearchOpen(true);
                requestAnimationFrame(() => searchInputRef.current?.focus());
              }}
            >
              <Search className="h-4 w-4" aria-hidden="true" />
            </button>
            <input
              ref={searchInputRef}
              type="search"
              placeholder={searchLabel}
              className="pipelines-search-input"
              value={searchTerm}
              onChange={event => {
                onSearchTermChange(event.target.value);
                if (event.target.value && !searchOpen) setSearchOpen(true);
              }}
              onBlur={() => {
                if (!searchTerm.trim()) setSearchOpen(false);
              }}
            />
            {searchTerm || searchOpen ? (
              <button
                type="button"
                className="pipelines-search-clear"
                aria-label="Clear search"
                onMouseDown={event => event.preventDefault()}
                onClick={() => {
                  onSearchTermChange('');
                  setSearchOpen(false);
                  searchInputRef.current?.blur();
                }}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
          </div>
          {filters}
          {canCreate || showCreateWhenDisabled ? (
            <button
              type="button"
              className="glass-button-primary triggers-create-button"
              aria-label={createLabel}
              title={canCreate ? createLabel : createDisabledReason}
              onClick={onCreate}
              disabled={!canCreate}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>
      {hasTitle ? <EventAutomationSwitch active={active} /> : null}
    </div>
  );
}
