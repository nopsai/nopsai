import { useRef, useState, type ReactNode } from 'react';
import { ChevronLeft, Plus, Search, X } from 'lucide-react';

type ResourceCollectionToolbarProps = {
  resourceLabel: string;
  activeTeam?: string;
  searchTerm: string;
  canCreate: boolean;
  onBack?: () => void;
  onSearchTermChange: (term: string) => void;
  onCreate: () => void;
  createLabel?: string;
  createDisabledReason?: string;
  showCreateWhenDisabled?: boolean;
  summary?: ReactNode;
  filters?: ReactNode;
};

export function ResourceCollectionToolbar({
  resourceLabel,
  activeTeam,
  searchTerm,
  canCreate,
  onBack,
  onSearchTermChange,
  onCreate,
  createLabel,
  createDisabledReason,
  showCreateWhenDisabled = false,
  summary,
  filters,
}: ResourceCollectionToolbarProps) {
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const plural = `${resourceLabel}s`;
  const title = resourceLabel.charAt(0).toUpperCase() + resourceLabel.slice(1);
  const createButtonLabel = createLabel || `Create new ${resourceLabel}`;
  const searchActive = searchOpen || Boolean(searchTerm.trim());

  return (
    <div className="px-4 pt-4 flex-shrink-0 resource-collection-toolbar">
      <div className="resource-collection-toolbar-row pipeline-runs-filterbar">
        <div className="resource-collection-toolbar-leading">
          {onBack ? (
            <button
              type="button"
              className="resource-collection-icon-button"
              aria-label="Back"
              title="Back"
              onClick={onBack}
              disabled={!activeTeam}
            >
              <ChevronLeft className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          {activeTeam ? (
            <span className="resource-collection-path" title={activeTeam}>
              {activeTeam}
            </span>
          ) : null}
          {filters}
          {summary ? <div className="text-xs text-[var(--text-secondary)]">{summary}</div> : null}
        </div>
        <div className="resource-collection-toolbar-actions">
          <div className={`pipelines-search-shell resource-collection-search-shell ${searchActive ? 'open' : ''}`}>
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label={`Search ${plural}`}
              title={`Search ${plural}`}
              onClick={() => {
                setSearchOpen(true);
                requestAnimationFrame(() => searchInputRef.current?.focus());
              }}
            >
              <Search className="h-4 w-4" aria-hidden="true" />
            </button>
            <input
              ref={searchInputRef}
              id={`${plural.replace(/\s+/g, '-')}-search`}
              className="pipelines-search-input"
              type="search"
              aria-label={`Search ${plural}`}
              placeholder={`Search ${plural}`}
              value={searchTerm}
              onFocus={() => setSearchOpen(true)}
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
                onMouseDown={event => event.preventDefault()}
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
          {canCreate || showCreateWhenDisabled ? (
            <button
              id={`${plural.replace(/\s+/g, '-')}-new-btn`}
              type="button"
              className="resource-collection-icon-button resource-collection-create-button"
              aria-label={createButtonLabel}
              title={canCreate ? createLabel || `New ${title}` : createDisabledReason}
              onClick={onCreate}
              disabled={!canCreate}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
