import { FolderTree, GitBranch, Globe2, KeyRound, ListFilter, Search, Users } from 'lucide-react';
import type { ReactNode } from 'react';
import type { CredentialCatalogCategory, CredentialCatalogGroup, CredentialRecord } from './model';
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
  namespaces: string[];
  recentCredentials: CredentialRecord[];
  scopeTabs: CredentialScopeTab[];
  selectedID?: string;
  query: string;
  status: string;
  scope: string;
  compact: boolean;
  activeGroupKey: string | null;
  loading: boolean;
  teamPaths: string[];
  onQueryChange: (value: string) => void;
  onStatusChange: (value: string) => void;
  onScopeChange: (value: string) => void;
  onCompactChange: (value: boolean) => void;
  onGroupChange: (value: string | null) => void;
  onSelect: (credential: CredentialRecord) => void;
};

export function CredentialCatalog({
  groups,
  namespaces,
  recentCredentials,
  scopeTabs,
  selectedID,
  query,
  status,
  scope,
  compact,
  activeGroupKey,
  loading,
  teamPaths,
  onQueryChange,
  onStatusChange,
  onScopeChange,
  onCompactChange,
  onGroupChange,
  onSelect,
}: CredentialCatalogProps) {
  const resultCount = groups.reduce((count, group) => count + group.credentials.length, 0);
  const visibleGroups = activeGroupKey ? groups.filter(group => group.key === activeGroupKey) : groups;
  const visibleCount = visibleGroups.reduce((count, group) => count + group.credentials.length, 0);
  const teamGroups = visibleGroups.filter(group => group.scopeKind === 'team');
  const sharedGroups = visibleGroups.filter(group => group.scopeKind === 'shared');
  const activeGroup = groups.find(group => group.key === activeGroupKey);
  const scopeOptions = [...new Set([...namespaces, scope])]
    .filter(value => value && !['all', 'team', 'shared'].includes(value))
    .sort((left, right) => left.localeCompare(right));

  return (
    <section className="space-y-4" aria-labelledby="credential-catalog-heading">
      <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 shadow-sm">
        <div className="grid gap-2 lg:grid-cols-[minmax(280px,1fr)_180px_160px_auto]">
          <label className="relative">
            <span className="sr-only">Search credentials</span>
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-secondary)]" aria-hidden="true" />
            <input
              className="pipelines-input !rounded-lg w-full !pl-9"
              value={query}
              onChange={event => onQueryChange(event.target.value)}
              placeholder="Search credentials by name, type, or description..."
            />
          </label>
          <label>
            <span className="sr-only">Filter by scope</span>
            <select className="pipelines-input !rounded-lg w-full" value={scope} onChange={event => onScopeChange(event.target.value)}>
              <option value="all">All scopes</option>
              <option value="team">Teams</option>
              <option value="shared">Shared scopes</option>
              {scopeOptions.map(value => <option key={value} value={value}>{formatCredentialLabel(value)}</option>)}
            </select>
          </label>
          <label>
            <span className="sr-only">Filter by status</span>
            <select className="pipelines-input !rounded-lg w-full" value={status} onChange={event => onStatusChange(event.target.value)}>
              <option value="all">All statuses</option>
              <option value="active">Active</option>
              <option value="pending">Pending</option>
              <option value="disabled">Disabled</option>
            </select>
          </label>
          <button
            type="button"
            className="glass-button-ghost !rounded-lg justify-center"
            aria-pressed={compact}
            onClick={() => onCompactChange(!compact)}
          >
            <ListFilter className="h-4 w-4" aria-hidden="true" />
            {compact ? 'Comfortable view' : 'Compact view'}
          </button>
        </div>
        <h3 id="credential-catalog-heading" className="sr-only">Credential catalog</h3>
        <p className="mt-2 px-1 text-xs text-[var(--text-secondary)]">
          {loading
            ? 'Loading credentials...'
            : `${visibleCount} credential${visibleCount === 1 ? '' : 's'} shown${activeGroup ? ` in ${formatCredentialScopeLabel(activeGroup.scopeKind, activeGroup.scopePath, activeGroup.namespace)}` : ''}`}
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2" aria-label="Credential scopes">
        {scopeTabs.map(tab => {
          const active = tab.value === scope;
          return (
            <button
              key={tab.value}
              type="button"
              className={`rounded-lg border px-3 py-2 text-sm transition-colors ${
                active
                  ? 'border-[var(--border-accent)] bg-[var(--bg-active)] text-[var(--text-primary)]'
                  : 'border-transparent bg-transparent text-[var(--text-secondary)] hover:border-[var(--border-primary)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]'
              }`}
              aria-pressed={active}
              onClick={() => onScopeChange(tab.value)}
            >
              <span>{tab.label}</span>
              <span className="ml-2 rounded-full bg-[var(--bg-secondary)] px-2 py-0.5 text-xs">{tab.count}</span>
            </button>
          );
        })}
      </div>

      <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)]">
        <CredentialScopeSidebar
          activeGroupKey={activeGroupKey}
          groups={groups}
          onGroupChange={onGroupChange}
        />

        <div className="min-w-0 space-y-5">
          {teamGroups.length > 0 && (
            <CredentialScopeSection
              title="Teams"
              badge={`${teamGroups.length} team${teamGroups.length === 1 ? '' : 's'} · ${countCredentials(teamGroups)} credentials`}
              groups={teamGroups}
              compact={compact}
              selectedID={selectedID}
              teamPaths={teamPaths}
              onSelect={onSelect}
            />
          )}

          {sharedGroups.length > 0 && (
            <CredentialScopeSection
              title="Shared scopes"
              badge="Global and system-owned"
              groups={sharedGroups}
              compact={compact}
              selectedID={selectedID}
              teamPaths={teamPaths}
              onSelect={onSelect}
            />
          )}

          {!loading && resultCount === 0 && (
            <div className="rounded-xl border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] py-10 text-center">
              <KeyRound className="mx-auto h-8 w-8 text-[var(--text-secondary)]" aria-hidden="true" />
              <p className="mt-3 font-medium text-[var(--text-primary)]">No matching credentials</p>
              <p className="text-sm text-[var(--text-secondary)]">Adjust the search or filters to see more results.</p>
            </div>
          )}
        </div>
      </div>

      <CredentialRecentTable credentials={recentCredentials} teamPaths={teamPaths} />
    </section>
  );
}

type CredentialScopeSidebarProps = {
  activeGroupKey: string | null;
  groups: CredentialCatalogGroup[];
  onGroupChange: (value: string | null) => void;
};

function CredentialScopeSidebar({ activeGroupKey, groups, onGroupChange }: CredentialScopeSidebarProps) {
  const teamGroups = groups.filter(group => group.scopeKind === 'team');
  const sharedGroups = groups.filter(group => group.scopeKind === 'shared');
  const totalCount = countCredentials(groups);

  return (
    <aside className="h-fit rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 shadow-sm xl:sticky xl:top-4">
      <div className="mb-3 flex items-center gap-2 px-1">
        <FolderTree className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
        <p className="text-sm font-semibold text-[var(--text-primary)]">Credential scopes</p>
      </div>
      <div className="space-y-1">
        <CredentialScopeNavButton
          active={!activeGroupKey}
          count={totalCount}
          icon={<KeyRound className="h-4 w-4" aria-hidden="true" />}
          label="All credentials"
          onClick={() => onGroupChange(null)}
        />
      </div>

      {teamGroups.length > 0 && (
        <CredentialScopeNavSection title="Teams">
          {teamGroups.map(group => (
            <CredentialScopeNavButton
              key={group.key}
              active={activeGroupKey === group.key}
              count={group.credentials.length}
              icon={<Users className="h-4 w-4" aria-hidden="true" />}
              label={formatCredentialScopeLabel(group.scopeKind, group.scopePath, group.namespace)}
              onClick={() => onGroupChange(activeGroupKey === group.key ? null : group.key)}
            />
          ))}
        </CredentialScopeNavSection>
      )}

      {sharedGroups.length > 0 && (
        <CredentialScopeNavSection title="Shared">
          {sharedGroups.map(group => (
            <CredentialScopeNavButton
              key={group.key}
              active={activeGroupKey === group.key}
              count={group.credentials.length}
              icon={<Globe2 className="h-4 w-4" aria-hidden="true" />}
              label={formatCredentialScopeLabel(group.scopeKind, group.scopePath, group.namespace)}
              onClick={() => onGroupChange(activeGroupKey === group.key ? null : group.key)}
            />
          ))}
        </CredentialScopeNavSection>
      )}
    </aside>
  );
}

function CredentialScopeNavSection({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="mt-4 space-y-1">
      <p className="px-2 text-xs font-semibold uppercase text-[var(--text-secondary)]">{title}</p>
      {children}
    </div>
  );
}

type CredentialScopeNavButtonProps = {
  active: boolean;
  count: number;
  icon: ReactNode;
  label: string;
  onClick: () => void;
};

function CredentialScopeNavButton({ active, count, icon, label, onClick }: CredentialScopeNavButtonProps) {
  return (
    <button
      type="button"
      className={`grid w-full grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors ${
        active
          ? 'bg-[var(--bg-active)] text-[var(--text-primary)]'
          : 'text-[var(--text-secondary)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]'
      }`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--bg-primary)]">{icon}</span>
      <span className="truncate font-medium">{label}</span>
      <span className="rounded-full bg-[var(--bg-primary)] px-2 py-0.5 text-xs">{count}</span>
    </button>
  );
}

type CredentialScopeSectionProps = {
  title: string;
  badge: string;
  groups: CredentialCatalogGroup[];
  compact: boolean;
  selectedID?: string;
  teamPaths: string[];
  onSelect: (credential: CredentialRecord) => void;
};

function CredentialScopeSection({
  title,
  badge,
  groups,
  compact,
  selectedID,
  teamPaths,
  onSelect,
}: CredentialScopeSectionProps) {
  const gridClass = groups.length === 1 ? 'grid-cols-1' : 'md:grid-cols-2 2xl:grid-cols-3';

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h4 className="font-semibold text-[var(--text-primary)]">{title}</h4>
          <span className="runner-pill runner-pill--muted">{badge}</span>
        </div>
      </div>
      <div className={`grid gap-3 ${gridClass}`}>
        {groups.map(group => (
          <CredentialScopeCard
            key={group.key}
            group={group}
            compact={compact}
            selectedID={selectedID}
            teamPaths={teamPaths}
            onSelect={onSelect}
          />
        ))}
      </div>
    </section>
  );
}

type CredentialScopeCardProps = {
  group: CredentialCatalogGroup;
  compact: boolean;
  selectedID?: string;
  teamPaths: string[];
  onSelect: (credential: CredentialRecord) => void;
};

function CredentialScopeCard({ group, compact, selectedID, teamPaths, onSelect }: CredentialScopeCardProps) {
  const scopeLabel = formatCredentialScopeLabel(group.scopeKind, group.scopePath, group.namespace);
  const avatarLabel = scopeLabel.charAt(0).toUpperCase();

  return (
    <article className="overflow-hidden rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)]">
      <div className="flex items-start justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-secondary)] text-sm font-semibold text-[var(--text-accent)]">
            {avatarLabel}
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-[var(--text-primary)]">{scopeLabel}</p>
            <p className="text-xs text-[var(--text-secondary)]">
              {group.credentials.length} credential{group.credentials.length === 1 ? '' : 's'}
            </p>
          </div>
        </div>
        <span className="runner-pill runner-pill--muted">{group.namespace}</span>
      </div>

      <div className="divide-y divide-[var(--border-primary)]">
        {group.categories.map(category => (
          <CredentialCategoryRows
            key={category.key}
            category={category}
            compact={compact}
            selectedID={selectedID}
            showCategory={group.categories.length > 1 || category.category !== 'general'}
            teamPaths={teamPaths}
            onSelect={onSelect}
          />
        ))}
      </div>
    </article>
  );
}

type CredentialCategoryRowsProps = {
  category: CredentialCatalogCategory;
  compact: boolean;
  selectedID?: string;
  showCategory: boolean;
  teamPaths: string[];
  onSelect: (credential: CredentialRecord) => void;
};

function CredentialCategoryRows({
  category,
  compact,
  selectedID,
  showCategory,
  teamPaths,
  onSelect,
}: CredentialCategoryRowsProps) {
  return (
    <div>
      {showCategory && (
        <div className="flex items-center justify-between bg-[var(--bg-secondary)] px-4 py-2">
          <p className="text-xs font-semibold uppercase text-[var(--text-secondary)]">{formatCredentialLabel(category.category)}</p>
          <span className="runner-pill runner-pill--muted">{category.credentials.length}</span>
        </div>
      )}
      <div className="divide-y divide-[var(--border-primary)]">
        {category.credentials.map(credential => (
          <CredentialRow
            key={credential.id}
            credential={credential}
            compact={compact}
            selected={credential.id === selectedID}
            teamPaths={teamPaths}
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

type CredentialRowProps = {
  credential: CredentialRecord;
  compact: boolean;
  selected: boolean;
  teamPaths: string[];
  onSelect: (credential: CredentialRecord) => void;
};

function CredentialRow({ credential, compact, selected, teamPaths, onSelect }: CredentialRowProps) {
  const reference = credentialReferenceDisplay(credential.reference, teamPaths);
  const displayName = formatCredentialLabel(reference.displayName);

  return (
    <button
      type="button"
      className={`grid w-full grid-cols-[32px_minmax(0,1fr)_auto] items-center gap-3 px-4 text-left transition-colors ${
        compact ? 'py-2' : 'py-3'
      } ${
        selected
          ? 'bg-[var(--bg-active)]'
          : 'hover:bg-[var(--bg-hover)]'
      }`}
      onClick={() => onSelect(credential)}
    >
      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-[var(--bg-secondary)] text-[var(--text-secondary)]">
        <KeyRound className="h-4 w-4" aria-hidden="true" />
      </span>
      <span className="min-w-0">
        <span className="block truncate text-sm font-semibold text-[var(--text-primary)]">{displayName}</span>
        <span className="mt-1 block truncate text-xs text-[var(--text-secondary)]">
          {formatCredentialLabel(credential.kind || reference.category)}
          {credential.active_version ? ` · Version ${credential.active_version}` : ' · No value'}
          {reference.parentPath ? ` · ${reference.parentPath}` : ''}
        </span>
        {credential.description && !compact ? (
          <span className="mt-1 block line-clamp-2 text-xs text-[var(--text-secondary)]">{credential.description}</span>
        ) : null}
      </span>
      <span className="flex flex-col items-end gap-1">
        <CredentialStatusBadge status={credential.status} />
        <span className="inline-flex items-center gap-1 text-xs text-[var(--text-secondary)]">
          {credential.managed_by_config_repo && <GitBranch className="h-3 w-3" aria-hidden="true" />}
          {credential.managed_by_config_repo ? 'GitOps' : 'System'}
        </span>
      </span>
    </button>
  );
}

function CredentialRecentTable({ credentials, teamPaths }: { credentials: CredentialRecord[]; teamPaths: string[] }) {
  if (credentials.length === 0) return null;

  return (
    <section className="overflow-hidden rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm" aria-labelledby="recent-credentials-heading">
      <div className="flex items-center justify-between border-b border-[var(--border-primary)] px-4 py-3">
        <h4 id="recent-credentials-heading" className="font-semibold text-[var(--text-primary)]">
          Recently updated
        </h4>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[680px] border-collapse text-left text-sm">
          <thead className="bg-[var(--bg-primary)] text-xs text-[var(--text-secondary)]">
            <tr>
              <th className="px-4 py-3 font-medium">Credential</th>
              <th className="px-4 py-3 font-medium">Scope</th>
              <th className="px-4 py-3 font-medium">Type</th>
              <th className="px-4 py-3 font-medium">Updated</th>
              <th className="px-4 py-3 font-medium">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border-primary)]">
            {credentials.map(credential => {
              const reference = credentialReferenceDisplay(credential.reference, teamPaths);
              return (
                <tr key={credential.id}>
                  <td className="px-4 py-3 font-medium text-[var(--text-primary)]">
                    {formatCredentialLabel(reference.displayName)}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-secondary)]">
                    {formatCredentialScopeLabel(reference.scopeKind, reference.scopePath, reference.namespace)}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-secondary)]">{formatCredentialLabel(credential.kind)}</td>
                  <td className="px-4 py-3 text-[var(--text-secondary)]">{formatCredentialDate(credential.updated_at)}</td>
                  <td className="px-4 py-3"><CredentialStatusBadge status={credential.status} /></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function countCredentials(groups: CredentialCatalogGroup[]): number {
  return groups.reduce((count, group) => count + group.credentials.length, 0);
}
