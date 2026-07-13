import { useRef, useState, type ReactNode } from 'react';
import { Plus, RefreshCw, Search, X } from 'lucide-react';
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
  onRefresh?: () => void;
  refreshLabel?: string;
  refreshDisabled?: boolean;
  filters?: ReactNode;
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
  onRefresh,
  refreshLabel,
  refreshDisabled = false,
  filters,
}: EventAutomationToolbarProps) {
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const hasTitle = Boolean(title || description);
  const resolvedRefreshLabel = refreshLabel || (title ? `Refresh ${title}` : 'Refresh');

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
        <div className="triggers-page-toolbar-actions">
          <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
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
          {onRefresh ? (
            <button
              type="button"
              className="triggers-icon-button"
              aria-label={resolvedRefreshLabel}
              title={resolvedRefreshLabel}
              onClick={onRefresh}
              disabled={refreshDisabled}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          {canCreate || showCreateWhenDisabled ? (
            <button
              type="button"
              className={canCreate ? 'glass-button-primary' : 'triggers-icon-button'}
              aria-label={createLabel}
              title={canCreate ? createLabel : createDisabledReason}
              onClick={onCreate}
              disabled={!canCreate}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
              {canCreate ? <span>{createLabel}</span> : null}
            </button>
          ) : null}
        </div>
      </div>
      {hasTitle ? <EventAutomationSwitch active={active} /> : null}
    </div>
  );
}
