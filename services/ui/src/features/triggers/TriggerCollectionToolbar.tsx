import type { RefObject } from 'react';
import { Plus, Search, X } from 'lucide-react';
import { EventAutomationSwitch } from '../event-automation/EventAutomationSwitch';
import type { TriggerSourceFilter } from './model';

type TriggerCollectionToolbarProps = {
  searchTerm: string;
  sourceFilter: TriggerSourceFilter;
  searchOpen: boolean;
  searchInputRef: RefObject<HTMLInputElement | null>;
  canCreateTriggerHere: boolean;
  onSearchTermChange: (value: string) => void;
  onSourceFilterChange: (value: TriggerSourceFilter) => void;
  onSearchOpenChange: (open: boolean) => void;
  onCreate: () => void;
};

export function TriggerCollectionToolbar({
  searchTerm,
  sourceFilter,
  searchOpen,
  searchInputRef,
  canCreateTriggerHere,
  onSearchTermChange,
  onSourceFilterChange,
  onSearchOpenChange,
  onCreate,
}: TriggerCollectionToolbarProps) {
  const searchActive = searchOpen || Boolean(searchTerm.trim());

  return (
    <div className="triggers-page-toolbar triggers-page-toolbar--compact">
      <div className="triggers-page-toolbar-head triggers-page-toolbar-head--compact">
        <EventAutomationSwitch active="triggers" />
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
          <select
            className="triggers-source-filter"
            aria-label="Filter triggers by source"
            value={sourceFilter}
            onChange={event => onSourceFilterChange(event.target.value as TriggerSourceFilter)}
          >
            <option value="all">All sources</option>
            <option value="git">GitOps</option>
            <option value="database">Database</option>
          </select>
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
