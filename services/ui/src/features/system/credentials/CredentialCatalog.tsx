import { ChevronDown, Globe2, KeyRound, ListFilter, MoreVertical, Search, Users, X } from 'lucide-react';
import { useRef, useState, type ReactNode } from 'react';
import type { CredentialCatalogGroup, CredentialRecord } from './model';
import { credentialReferenceDisplay } from './model';
import { formatCredentialDate, formatCredentialLabel, formatCredentialScopeLabel } from './presentation';
import { CredentialStatusBadge } from './CredentialStatusBadge';

export type CredentialScopeTab = {
  value: string;
  label: string;
  count: number;
};

type CredentialCatalogProps = {
  groups: CredentialCatalogGroup[];
  scopeTabs: CredentialScopeTab[];
  selectedID?: string;
  query: string;
  scope: string;
  grouped: boolean;
  loading: boolean;
  teamPaths: string[];
  onQueryChange: (value: string) => void;
  onScopeChange: (value: string) => void;
  onGroupedChange: (value: boolean) => void;
  onSelect: (credential: CredentialRecord) => void;
};

export function CredentialCatalog({
  groups,
  scopeTabs,
  selectedID,
  query,
  scope,
  grouped,
  loading,
  teamPaths,
  onQueryChange,
  onScopeChange,
  onGroupedChange,
  onSelect,
}: CredentialCatalogProps) {
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const searchActive = searchOpen || Boolean(query.trim());
  const visibleCount = groups.reduce((count, group) => count + group.credentials.length, 0);
  const allRows = groups.flatMap(group => group.credentials.map(credential => ({ credential, group })));

  return (
    <section className="credential-registry__catalog" aria-labelledby="credential-catalog-heading">
      <h3 id="credential-catalog-heading" className="sr-only">Credential catalog</h3>

      <div className="credential-registry__catalog-top">
        <div className="credential-registry__tabs" aria-label="Credential scopes">
          {scopeTabs.map(tab => {
            const active = tab.value === scope;
            return (
              <button
                key={tab.value}
                type="button"
                className={`credential-registry__tab ${active ? 'credential-registry__tab--active' : ''}`}
                aria-label={`${tab.label === 'All' ? 'All credentials' : tab.label} (${tab.count})`}
                aria-pressed={active}
                onClick={() => onScopeChange(tab.value)}
              >
                <span>{tab.label}</span>
                <span className="credential-registry__badge">{tab.count}</span>
              </button>
            );
          })}
        </div>
        <div className="credential-registry__catalog-actions">
          <div className={`pipelines-search-shell credential-registry__search-shell ${searchActive ? 'open' : ''}`}>
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label="Search credentials"
              title="Search credentials"
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
              className="pipelines-search-input"
              aria-label="Search credentials query"
              placeholder="Search credentials"
              value={query}
              onFocus={() => setSearchOpen(true)}
              onChange={event => {
                onQueryChange(event.target.value);
                if (event.target.value && !searchOpen) setSearchOpen(true);
              }}
              onBlur={() => {
                if (!query.trim()) setSearchOpen(false);
              }}
            />
            {query || searchOpen ? (
              <button
                type="button"
                className="pipelines-search-clear"
                aria-label="Clear search"
                onMouseDown={event => event.preventDefault()}
                onClick={() => {
                  onQueryChange('');
                  setSearchOpen(false);
                  searchInputRef.current?.blur();
                }}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
          </div>
          <button
            type="button"
            className="credential-registry__button credential-registry__button--ghost credential-registry__button--small"
            aria-pressed={grouped}
            onClick={() => onGroupedChange(!grouped)}
          >
            <ListFilter className="h-4 w-4" aria-hidden="true" />
            {grouped ? 'Flat list' : 'Group by scope'}
          </button>
        </div>
      </div>

      <div className="credential-registry__table-card">
        <div className="credential-registry__table-head" aria-hidden="true">
          <div>Credential</div>
          <div>Type</div>
          <div>Scope</div>
          <div>Status</div>
          <div>Updated</div>
          <div>Version</div>
          <div></div>
        </div>

        {loading ? (
          <CredentialCatalogEmpty icon={<KeyRound className="h-8 w-8" aria-hidden="true" />} title="Loading credentials..." />
        ) : visibleCount === 0 ? (
          <CredentialCatalogEmpty icon={<KeyRound className="h-8 w-8" aria-hidden="true" />} title="No matching credentials" detail="Adjust the search or filters to see more results." />
        ) : grouped ? (
          groups.map(group => (
            <CredentialTableGroup
              key={group.key}
              group={group}
              selectedID={selectedID}
              teamPaths={teamPaths}
              onSelect={onSelect}
            />
          ))
        ) : (
          allRows.map(({ credential, group }) => (
            <CredentialTableRow
              key={credential.id}
              credential={credential}
              group={group}
              selected={credential.id === selectedID}
              teamPaths={teamPaths}
              onSelect={onSelect}
            />
          ))
        )}

      </div>
    </section>
  );
}

type CredentialCatalogEmptyProps = {
  detail?: string;
  icon: ReactNode;
  title: string;
};

function CredentialCatalogEmpty({ detail, icon, title }: CredentialCatalogEmptyProps) {
  return (
    <div className="credential-registry__empty">
      {icon}
      <div>
        <p className="font-semibold text-[var(--credential-text)]">{title}</p>
        {detail ? <p className="mt-1">{detail}</p> : null}
      </div>
    </div>
  );
}

type CredentialTableGroupProps = {
  group: CredentialCatalogGroup;
  selectedID?: string;
  teamPaths: string[];
  onSelect: (credential: CredentialRecord) => void;
};

function CredentialTableGroup({ group, selectedID, teamPaths, onSelect }: CredentialTableGroupProps) {
  const label = formatCredentialScopeLabel(group.scopeKind, group.scopePath, group.namespace);
  const namespaceLabel = group.scopeKind === 'team' ? 'team' : group.namespace || 'system';
  const Icon = group.scopeKind === 'team' ? Users : Globe2;

  return (
    <div>
      <div className="credential-registry__group">
        <div className="credential-registry__group-left">
          <ChevronDown className="h-4 w-4 text-[var(--credential-muted)]" aria-hidden="true" />
          <Icon className="h-4 w-4 text-[var(--credential-muted)]" aria-hidden="true" />
          <span className="credential-registry__group-title">{label}</span>
          <span className="credential-registry__group-note">
            {group.credentials.length} credential{group.credentials.length === 1 ? '' : 's'}
          </span>
        </div>
        <span className="credential-registry__pill">{namespaceLabel}</span>
      </div>
      {group.credentials.map(credential => (
        <CredentialTableRow
          key={credential.id}
          credential={credential}
          group={group}
          selected={credential.id === selectedID}
          teamPaths={teamPaths}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

type CredentialTableRowProps = {
  credential: CredentialRecord;
  group: CredentialCatalogGroup;
  selected: boolean;
  teamPaths: string[];
  onSelect: (credential: CredentialRecord) => void;
};

function CredentialTableRow({ credential, group, selected, teamPaths, onSelect }: CredentialTableRowProps) {
  const reference = credentialReferenceDisplay(credential.reference, teamPaths);
  const displayName = formatCredentialLabel(reference.displayName);
  const scopeLabel = formatCredentialScopeLabel(group.scopeKind, group.scopePath, group.namespace);
  const versionLabel = credential.active_version ? String(credential.active_version) : '-';

  return (
    <button
      type="button"
      className={`credential-registry__row ${selected ? 'credential-registry__row--selected' : ''}`}
      onClick={() => onSelect(credential)}
    >
      <span className="credential-registry__credential-cell">
        <span className="credential-registry__row-icon" aria-hidden="true">
          <KeyRound className="h-4 w-4" />
        </span>
        <span className="credential-registry__credential-copy">
          <span className="credential-registry__credential-name">{displayName}</span>
        </span>
      </span>
      <span className="credential-registry__cell">{formatCredentialLabel(credential.kind || reference.category)}</span>
      <span className="credential-registry__cell">{scopeLabel}</span>
      <span className="credential-registry__cell"><CredentialStatusBadge status={credential.status} /></span>
      <span className="credential-registry__cell">{formatCredentialDate(credential.updated_at)}</span>
      <span className="credential-registry__cell">{versionLabel}</span>
      <span className="credential-registry__more" aria-hidden="true">
        <MoreVertical className="h-4 w-4" />
      </span>
    </button>
  );
}
