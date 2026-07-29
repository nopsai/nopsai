import type { ReactNode, RefObject } from 'react';
import { Plus, Search, X } from 'lucide-react';
import { EventAutomationSwitch } from '../event-automation/EventAutomationSwitch';

type TriggerCollectionToolbarProps = {
  searchTerm: string;
  searchOpen: boolean;
  searchInputRef: RefObject<HTMLInputElement | null>;
  canCreateTriggerHere: boolean;
  summary?: ReactNode;
  onSearchTermChange: (value: string) => void;
  onSearchOpenChange: (open: boolean) => void;
  onCreate: () => void;
};

export function TriggerCollectionToolbar({
  searchTerm,
  searchOpen,
  searchInputRef,
  canCreateTriggerHere,
  summary,
  onSearchTermChange,
  onSearchOpenChange,
  onCreate,
}: TriggerCollectionToolbarProps) {
  const searchActive = searchOpen || Boolean(searchTerm.trim());

  return (
    <div className="triggers-page-toolbar triggers-page-toolbar--compact">
      <div className="triggers-page-toolbar-head triggers-page-toolbar-head--compact">
        <EventAutomationSwitch active="triggers" />
        {summary ? <div className="triggers-page-toolbar-summary">{summary}</div> : null}
        <div className="triggers-page-toolbar-actions">
          <div className={`pipelines-search-shell ${searchActive ? 'open' : ''}`}>
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label="Search triggers"
              onClick={() => {
                onSearchOpenChange(true);
                requestAnimationFrame(() => searchInputRef.current?.focus());
              }}
            >
              <Search className="h-4 w-4" aria-hidden="true" />
            </button>
            <input
              ref={searchInputRef}
              id="triggers-search"
              type="text"
              placeholder="Search triggers"
              className="pipelines-search-input"
              value={searchTerm}
              onChange={event => {
                onSearchTermChange(event.target.value);
                if (event.target.value && !searchOpen) onSearchOpenChange(true);
              }}
              onBlur={() => {
                if (!searchTerm.trim()) onSearchOpenChange(false);
              }}
            />
            {(searchTerm || searchOpen) && (
              <button
                type="button"
                className="pipelines-search-clear"
                onMouseDown={event => event.preventDefault()}
                onClick={() => {
                  onSearchTermChange('');
                  onSearchOpenChange(false);
                  searchInputRef.current?.blur();
                }}
                aria-label="Clear search"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            )}
          </div>
          <button
            id="triggers-new-btn"
            type="button"
            className="glass-button-primary"
            aria-label="Create new trigger"
            title={canCreateTriggerHere ? 'New Trigger' : 'Create trigger override'}
            onClick={onCreate}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            <span>New trigger</span>
          </button>
        </div>
      </div>
    </div>
  );
}
