import { useRef, useState } from 'react';
import { ChevronLeft, Plus, Search, X } from 'lucide-react';

type ResourceCollectionToolbarProps = {
  resourceLabel: 'pipeline' | 'step';
  activeFolder: string;
  searchTerm: string;
  canCreate: boolean;
  onBack: () => void;
  onSearchTermChange: (term: string) => void;
  onCreate: () => void;
};

export function ResourceCollectionToolbar({
  resourceLabel,
  activeFolder,
  searchTerm,
  canCreate,
  onBack,
  onSearchTermChange,
  onCreate,
}: ResourceCollectionToolbarProps) {
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const plural = `${resourceLabel}s`;
  const title = resourceLabel.charAt(0).toUpperCase() + resourceLabel.slice(1);

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
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
        </button>
        <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
          <button
            type="button"
            className="pipelines-search-toggle"
            aria-label={`Search ${plural}`}
            onClick={() => {
              setSearchOpen(true);
              requestAnimationFrame(() => searchInputRef.current?.focus());
            }}
          >
            <Search className="h-4 w-4" aria-hidden="true" />
          </button>
          <input
            ref={searchInputRef}
            id={`${plural}-search`}
            type="search"
            placeholder={`Search ${plural}`}
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
              onClick={() => {
                onSearchTermChange('');
                setSearchOpen(false);
                searchInputRef.current?.blur();
              }}
              aria-label="Clear search"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
        {canCreate ? (
          <button
            id={`${plural}-new-btn`}
            type="button"
            className="pipelines-icon-only"
            aria-label={`Create new ${resourceLabel}`}
            title={`New ${title}`}
            onClick={onCreate}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : null}
      </div>
    </div>
  );
}
