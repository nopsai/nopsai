import { ChevronDown, Globe2, KeyRound, ListFilter, MoreVertical, Search, Users } from 'lucide-react';
import type { ReactNode } from 'react';
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
  isNopsAIAdmin: boolean;
  namespaces: string[];
  scopeTabs: CredentialScopeTab[];
  selectedID?: string;
  query: string;
  status: string;
  scope: string;
  grouped: boolean;
  loading: boolean;
  teamPaths: string[];
  onQueryChange: (value: string) => void;
  onStatusChange: (value: string) => void;
  onScopeChange: (value: string) => void;
  onGroupedChange: (value: boolean) => void;
  onSelect: (credential: CredentialRecord) => void;
};

export function CredentialCatalog({
  groups,
  isNopsAIAdmin,
  namespaces,
  scopeTabs,
  selectedID,
  query,
  status,
  scope,
  grouped,
  loading,
  teamPaths,
  onQueryChange,
  onStatusChange,
  onScopeChange,
  onGroupedChange,
  onSelect,
}: CredentialCatalogProps) {
  const visibleCount = groups.reduce((count, group) => count + group.credentials.length, 0);
  const visibleScopeCount = groups.length;
  const scopeOptions = [...new Set([...namespaces, scope])]
    .filter(value => value && !['all', 'team', 'system'].includes(value))
    .filter(value => isNopsAIAdmin || value === 'team')
    .sort((left, right) => left.localeCompare(right));
  const allRows = groups.flatMap(group => group.credentials.map(credential => ({ credential, group })));

  return (
    <section className="credential-registry__catalog" aria-labelledby="credential-catalog-heading">
      <div>
        <div className="credential-registry__toolbar">
          <label className="credential-registry__search">
            <span className="sr-only">Search credentials</span>
            <Search className="credential-registry__search-icon h-4 w-4" aria-hidden="true" />
            <input
              className="credential-registry__field"
              value={query}
              onChange={event => onQueryChange(event.target.value)}
              placeholder="Search credentials by name, type, or description..."
            />
          </label>
          <label>
            <span className="sr-only">Filter by scope</span>
            <select className="credential-registry__field" value={scope} onChange={event => onScopeChange(event.target.value)}>
              <option value="all">All scopes</option>
              <option value="team">Teams</option>
              {isNopsAIAdmin ? <option value="system">System</option> : null}
              {scopeOptions.map(value => <option key={value} value={value}>{formatCredentialLabel(value)}</option>)}
            </select>
          </label>
          <label>
            <span className="sr-only">Filter by status</span>
            <select className="credential-registry__field" value={status} onChange={event => onStatusChange(event.target.value)}>
              <option value="all">All statuses</option>
              <option value="active">Active</option>
              <option value="pending">Pending</option>
              <option value="disabled">Disabled</option>
            </select>
          </label>
          <button
            type="button"
            className="credential-registry__button credential-registry__button--ghost"
            aria-pressed={grouped}
            onClick={() => onGroupedChange(!grouped)}
          >
            <ListFilter className="h-4 w-4" aria-hidden="true" />
            {grouped ? 'Flat list' : 'Group by scope'}
          </button>
        </div>
        <h3 id="credential-catalog-heading" className="sr-only">Credential catalog</h3>
        <p className="credential-registry__result-count">
          {loading ? 'Loading credentials...' : `${visibleCount} credential${visibleCount === 1 ? '' : 's'} shown`}
        </p>
      </div>

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
        <button
          type="button"
          className="credential-registry__button credential-registry__button--ghost credential-registry__button--small"
          aria-pressed={grouped}
          onClick={() => onGroupedChange(!grouped)}
        >
          <ListFilter className="h-4 w-4" aria-hidden="true" />
          {grouped ? 'Group by scope' : 'Flat list'}
        </button>
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

        <div className="credential-registry__footer-note">
          Showing {visibleCount} credential{visibleCount === 1 ? '' : 's'} across {visibleScopeCount} scope{visibleScopeCount === 1 ? '' : 's'}
        </div>
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
