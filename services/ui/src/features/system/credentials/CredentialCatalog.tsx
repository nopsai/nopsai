import { GitBranch, KeyRound, Search } from 'lucide-react';
import type { CredentialTeam, CredentialRecord } from './model';
import { parseCredentialReference } from './model';
import { formatCredentialLabel } from './presentation';
import { CredentialStatusBadge } from './CredentialStatusBadge';

type CredentialCatalogProps = {
  teams: CredentialTeam[];
  namespaces: string[];
  selectedID?: string;
  query: string;
  status: string;
  namespace: string;
  loading: boolean;
  onQueryChange: (value: string) => void;
  onStatusChange: (value: string) => void;
  onNamespaceChange: (value: string) => void;
  onSelect: (credential: CredentialRecord) => void;
};

export function CredentialCatalog({
  teams,
  namespaces,
  selectedID,
  query,
  status,
  namespace,
  loading,
  onQueryChange,
  onStatusChange,
  onNamespaceChange,
  onSelect,
}: CredentialCatalogProps) {
  const resultCount = teams.reduce((count, team) => count + team.credentials.length, 0);

  return (
    <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
      <div className="p-4 border-b border-[var(--border-primary)] space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="font-semibold text-[var(--text-primary)]">Credential catalog</h3>
            <p className="text-xs text-[var(--text-secondary)]">
              {loading ? 'Loading credentials...' : `${resultCount} credential${resultCount === 1 ? '' : 's'} shown`}
            </p>
          </div>
        </div>
        <div className="grid gap-2 lg:grid-cols-[minmax(0,1fr)_160px_160px]">
          <label className="relative">
            <span className="sr-only">Search credentials</span>
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-secondary)]" aria-hidden="true" />
            <input
              className="pipelines-input w-full !pl-9"
              value={query}
              onChange={event => onQueryChange(event.target.value)}
              placeholder="Search name, kind, or description"
            />
          </label>
          <label>
            <span className="sr-only">Filter by status</span>
            <select className="pipelines-input w-full" value={status} onChange={event => onStatusChange(event.target.value)}>
              <option value="all">All statuses</option>
              <option value="active">Active</option>
              <option value="disabled">Disabled</option>
              <option value="pending">Pending</option>
            </select>
          </label>
          <label>
            <span className="sr-only">Filter by namespace</span>
            <select className="pipelines-input w-full" value={namespace} onChange={event => onNamespaceChange(event.target.value)}>
              <option value="all">All namespaces</option>
              {namespaces.map(value => <option key={value} value={value}>{value}</option>)}
            </select>
          </label>
        </div>
      </div>

      <div className="p-4 space-y-6">
        {teams.map(team => (
          <section key={team.key} className="space-y-3">
            <div className="flex items-center gap-2">
              <h4 className="text-sm font-semibold text-[var(--text-primary)]">
                {formatCredentialLabel(team.category)}
              </h4>
              <span className="runner-pill runner-pill--muted">{team.namespace}</span>
              <span className="text-xs text-[var(--text-secondary)]">{team.credentials.length}</span>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              {team.credentials.map(credential => {
                const reference = parseCredentialReference(credential.reference);
                const selected = credential.id === selectedID;
                return (
                  <button
                    key={credential.id}
                    type="button"
                    className={`rounded-xl border p-4 text-left transition-colors ${
                      selected
                        ? 'border-[var(--accent-primary)] bg-[var(--bg-secondary)]'
                        : 'border-[var(--border-primary)] hover:bg-[var(--bg-secondary)]'
                    }`}
                    onClick={() => onSelect(credential)}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex min-w-0 items-start gap-3">
                        <span className="rounded-lg bg-[var(--bg-secondary)] p-2 text-[var(--text-secondary)]">
                          <KeyRound className="h-4 w-4" aria-hidden="true" />
                        </span>
                        <div className="min-w-0">
                          <p className="font-semibold text-[var(--text-primary)] break-words">
                            {formatCredentialLabel(reference.displayName)}
                          </p>
                          {reference.parentPath && (
                            <p className="text-xs text-[var(--text-secondary)]">{reference.parentPath}</p>
                          )}
                        </div>
                      </div>
                      <CredentialStatusBadge status={credential.status} />
                    </div>
                    <p className="mt-3 line-clamp-2 text-sm text-[var(--text-secondary)]">
                      {credential.description || 'No description'}
                    </p>
                    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--text-secondary)]">
                      <span>{formatCredentialLabel(credential.kind)}</span>
                      <span>Version {credential.active_version || '-'}</span>
                      <span className="inline-flex items-center gap-1">
                        {credential.managed_by_config_repo && <GitBranch className="h-3 w-3" aria-hidden="true" />}
                        {credential.managed_by_config_repo ? 'GitOps' : 'System'}
                      </span>
                    </div>
                  </button>
                );
              })}
            </div>
          </section>
        ))}

        {!loading && teams.length === 0 && (
          <div className="py-10 text-center">
            <KeyRound className="mx-auto h-8 w-8 text-[var(--text-secondary)]" aria-hidden="true" />
            <p className="mt-3 font-medium text-[var(--text-primary)]">No matching credentials</p>
            <p className="text-sm text-[var(--text-secondary)]">Adjust the search or filters to see more results.</p>
          </div>
        )}
      </div>
    </section>
  );
}
