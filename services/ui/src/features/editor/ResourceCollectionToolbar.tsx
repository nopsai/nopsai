import { useRef, useState, type ReactNode } from 'react';
import { ChevronLeft, Plus, RefreshCw, Search, X } from 'lucide-react';

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
  onRefresh?: () => void;
  refreshDisabled?: boolean;
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
  onRefresh,
  refreshDisabled = false,
  summary,
  filters,
}: ResourceCollectionToolbarProps) {
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const plural = `${resourceLabel}s`;
  const title = resourceLabel.charAt(0).toUpperCase() + resourceLabel.slice(1);
  const createButtonLabel = createLabel || `Create new ${resourceLabel}`;

  return (
    <div className="px-6 pt-6 flex-shrink-0 resource-collection-toolbar">
      <div className="resource-collection-toolbar-row pipeline-runs-filterbar">
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
        <label className={`pipeline-runs-search-field ${searchOpen ? 'pipeline-runs-search-field--active' : ''}`}>
          <Search className="h-4 w-4" aria-hidden="true" />
          <span className="sr-only">{`Search ${plural}`}</span>
          <input
            ref={searchInputRef}
            id={`${plural.replace(/\s+/g, '-')}-search`}
            type="search"
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
        </label>
        {filters}
        {onRefresh ? (
          <button
            type="button"
            className="resource-collection-icon-button"
            aria-label={`Refresh ${plural}`}
            title={`Refresh ${plural}`}
            onClick={onRefresh}
            disabled={refreshDisabled}
          >
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : null}
        {canCreate || showCreateWhenDisabled ? (
          <button
            id={`${plural.replace(/\s+/g, '-')}-new-btn`}
            type="button"
            className="resource-collection-icon-button"
            aria-label={createButtonLabel}
            title={canCreate ? createLabel || `New ${title}` : createDisabledReason}
            onClick={onCreate}
            disabled={!canCreate}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : null}
        {summary ? <div className="text-xs text-[var(--text-secondary)]">{summary}</div> : null}
      </div>
    </div>
  );
}
