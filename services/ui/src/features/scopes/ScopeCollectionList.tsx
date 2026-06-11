import {
  countScopesRecursive,
  type ScopeData,
  type ScopeEntry,
  type ScopeTreeNode,
} from './model';

type ScopeCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  searchTerm: string;
  activeFolderNode: ScopeTreeNode;
  filteredScopes: ScopeEntry[];
  scopesByLabel: Map<string, ScopeEntry>;
  scopeDataByScope: Record<string, ScopeData>;
  canCreateScopeHere: boolean;
  onOpenFolder: (path: string) => void;
  onSelectScope: (scopeLabel: string) => void;
};

export function ScopeCollectionList({
  listLoading,
  listError,
  searchTerm,
  activeFolderNode,
  filteredScopes,
  scopesByLabel,
  scopeDataByScope,
  canCreateScopeHere,
  onOpenFolder,
  onSelectScope,
}: ScopeCollectionListProps) {
  const hasSearch = Boolean(searchTerm.trim());
  const folders = hasSearch ? [] : activeFolderNode.children;
  const scopeLabels = hasSearch ? [] : activeFolderNode.scopes;
  const scopeEntries = hasSearch
    ? filteredScopes
    : scopeLabels.map(label => scopesByLabel.get(label)).filter((item): item is ScopeEntry => Boolean(item));

  return (
    <div id="scopes-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading scopes…</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load scopes: {listError}</div>
        ) : (
          <>
            {scopeEntries.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">
                {scopeEntries.map(scope => (
                  <ScopeCard
                    key={scope.scope || '__default__'}
                    scope={scope}
                    data={scopeDataByScope[scope.scope]}
                    onSelectScope={onSelectScope}
                  />
                ))}
              </div>
            ) : null}

            {!hasSearch && folders.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {folders.map(child => (
                  <ScopeFolderCard key={`folder-${child.id}`} node={child} onOpenFolder={onOpenFolder} />
                ))}
              </div>
            ) : null}

            {!scopeEntries.length && !folders.length && (
              <div id="scopes-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No scopes found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {hasSearch
                    ? `No scope groups matched “${searchTerm.trim()}”.`
                    : canCreateScopeHere
                      ? 'Create a new scope or adjust your filters.'
                      : 'Adjust your filters or browse another group.'}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function ScopeFolderCard({ node, onOpenFolder }: { node: ScopeTreeNode; onOpenFolder: (path: string) => void }) {
  const totalScopes = countScopesRecursive(node);
  return (
    <article
      className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
      onClick={() => onOpenFolder(node.fullPath)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
              <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
              <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
            </svg>
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{node.name}</h3>
            <p className="pipeline-card-path">{node.fullPath ? `/${node.fullPath}` : '/'}</p>
          </div>
        </div>
        <span className="pipeline-folder-chevron">›</span>
      </div>
      <div className="pipeline-folder-meta">
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Scopes:</span>
          <span className="pipeline-card-meta-value">{totalScopes}</span>
        </div>
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Sub groups:</span>
          <span className="pipeline-card-meta-value">{node.children.length}</span>
        </div>
      </div>
    </article>
  );
}

function ScopeCard({
  scope,
  data,
  onSelectScope,
}: {
  scope: ScopeEntry;
  data?: ScopeData;
  onSelectScope: (scopeLabel: string) => void;
}) {
  const scopeLabel = scope.scope ? `/${scope.scope}` : '/';
  const variableCount = data?.variablesLoaded ? data.variables.length : 0;
  const secretCount = data?.secretsLoaded ? data.secrets.length : scope.secretCountHint;
  const variableLabel = `${variableCount} variable${variableCount === 1 ? '' : 's'}`;
  const secretLabel = `${secretCount} secret${secretCount === 1 ? '' : 's'}`;

  return (
    <article
      className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
      onClick={() => onSelectScope(scope.scope)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="7.5" />
              <circle cx="12" cy="12" r="2.5" />
              <path d="M12 3v3m0 12v3m9-9h-3M6 12H3" />
              <path d="M16.5 7.5l-1.75 1.75m-5.5 5.5L7.5 16.5" />
              <path d="M7.5 7.5l1.75 1.75m5.5 5.5l1.75 1.75" />
            </svg>
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{scope.label}</h3>
            <p className="pipeline-card-path">{scopeLabel}</p>
            <p className="pipeline-card-description">Configuration &amp; secrets manager.</p>
          </div>
        </div>
      </div>
      <div className="pipeline-card-meta">
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Variables</span>
          <span className="pipeline-card-meta-value">{variableLabel}</span>
        </div>
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Secrets</span>
          <span className="pipeline-card-meta-value">{secretLabel}</span>
        </div>
      </div>
    </article>
  );
}
