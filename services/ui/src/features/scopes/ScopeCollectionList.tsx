import { ObjectIcon } from '../../components/ObjectIcon';
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
  activeTeamNode: ScopeTreeNode;
  filteredScopes: ScopeEntry[];
  scopesByLabel: Map<string, ScopeEntry>;
  scopeDataByScope: Record<string, ScopeData>;
  canCreateScopeHere: boolean;
  onOpenTeam: (path: string) => void;
  onSelectScope: (scopeLabel: string) => void;
};

export function ScopeCollectionList({
  listLoading,
  listError,
  searchTerm,
  activeTeamNode,
  filteredScopes,
  scopesByLabel,
  scopeDataByScope,
  canCreateScopeHere,
  onOpenTeam,
  onSelectScope,
}: ScopeCollectionListProps) {
  const hasSearch = Boolean(searchTerm.trim());
  const teams = hasSearch ? [] : activeTeamNode.children;
  const scopeLabels = hasSearch ? [] : activeTeamNode.scopes;
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

            {!hasSearch && teams.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {teams.map(child => (
                  <ScopeTeamCard key={`team-${child.id}`} node={child} onOpenTeam={onOpenTeam} />
                ))}
              </div>
            ) : null}

            {!scopeEntries.length && !teams.length && (
              <div id="scopes-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No scopes found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {hasSearch
                    ? `No scope teams matched “${searchTerm.trim()}”.`
                    : canCreateScopeHere
                      ? 'Create a new scope or adjust your filters.'
                      : 'Adjust your filters or browse another team.'}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function ScopeTeamCard({ node, onOpenTeam }: { node: ScopeTreeNode; onOpenTeam: (path: string) => void }) {
  const totalScopes = countScopesRecursive(node);
  return (
    <article
      className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
      onClick={() => onOpenTeam(node.fullPath)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <ObjectIcon type="team" />
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{node.name}</h3>
            <p className="pipeline-card-path">{node.fullPath ? `/${node.fullPath}` : '/'}</p>
          </div>
        </div>
        <span className="pipeline-team-chevron">›</span>
      </div>
      <div className="pipeline-team-meta">
        <div className="pipeline-team-meta-row">
          <span className="pipeline-card-meta-label">Scopes:</span>
          <span className="pipeline-card-meta-value">{totalScopes}</span>
        </div>
        <div className="pipeline-team-meta-row">
          <span className="pipeline-card-meta-label">Sub teams:</span>
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
            <ObjectIcon type="scope" />
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
