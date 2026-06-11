import type { RefObject } from 'react';
import { ArrowLeft, Plus, Search, X } from 'lucide-react';

type TriggerCollectionToolbarProps = {
  activeFolder: string;
  searchTerm: string;
  searchOpen: boolean;
  searchInputRef: RefObject<HTMLInputElement | null>;
  canCreateTriggerHere: boolean;
  onBack: () => void;
  onSearchTermChange: (value: string) => void;
  onSearchOpenChange: (open: boolean) => void;
  onCreate: () => void;
};

export function TriggerCollectionToolbar({
  activeFolder,
  searchTerm,
  searchOpen,
  searchInputRef,
  canCreateTriggerHere,
  onBack,
  onSearchTermChange,
  onSearchOpenChange,
  onCreate,
}: TriggerCollectionToolbarProps) {
  return (
    <div className="px-6 pt-6 pb-4">
      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          className="glass-button-ghost"
          aria-label="Back"
          onClick={onBack}
          disabled={!activeFolder}
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        </button>
        <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
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
        {canCreateTriggerHere && (
          <button
            id="triggers-new-btn"
            type="button"
            className="pipelines-icon-only"
            aria-label="Create new trigger"
            title="New Trigger"
            onClick={onCreate}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
          </button>
        )}
      </div>
    </div>
  );
}
