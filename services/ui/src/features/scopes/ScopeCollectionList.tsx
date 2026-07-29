import { useMemo, type ReactNode } from 'react';
import { ArrowRight } from 'lucide-react';
import { ResourceCollectionWorkspace, type ResourceCollectionTreeNode } from '../editor/ResourceCollectionWorkspace';
import {
  formatScopeDisplay,
  type ScopeData,
  type ScopeEntry,
  type ScopeTreeNode,
} from './model';

type ScopeCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  visibleScopes: ScopeEntry[];
  treeRoot: ScopeTreeNode;
  activeTeam: string;
  canCreateScopeHere: boolean;
  onOpenTeam: (path: string) => void;
  onSelectScope: (scopeLabel: string) => void;
  scopeDataByScope: Record<string, ScopeData>;
};

export function ScopeCollectionList({
  listLoading,
  listError,
  visibleScopes,
  treeRoot,
  activeTeam,
  canCreateScopeHere,
  onOpenTeam,
  onSelectScope,
  scopeDataByScope,
}: ScopeCollectionListProps) {
  const resourceTreeRoot = useMemo(() => toResourceCollectionTreeNode(treeRoot), [treeRoot]);
  const emptyMessage = canCreateScopeHere ? 'Create a new scope or adjust your filters.' : 'Adjust your filters or check your access.';

  if (listLoading) {
    return (
      <ScopeWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
        <ResourcePanel title="Scopes" countLabel="Loading">
          <div className="pipeline-runs-empty-state">Loading scopes...</div>
        </ResourcePanel>
      </ScopeWorkspace>
    );
  }

  if (listError) {
    return (
      <ScopeWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
        <ResourcePanel title="Scopes" countLabel="Error">
          <div className="pipeline-runs-empty-state text-red-500">Failed to load scopes: {listError}</div>
        </ResourcePanel>
      </ScopeWorkspace>
    );
  }

  return (
    <ScopeWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
      <div id="scopes-list-view" className="pipelines-view pipeline-runs-content-grid">
        <ResourcePanel title="Scopes" countLabel={`${visibleScopes.length} visible`}>
          {visibleScopes.length ? (
            <ScopeTable
              scopes={visibleScopes}
              scopeDataByScope={scopeDataByScope}
              onSelectScope={onSelectScope}
            />
          ) : (
            <div id="scopes-empty" className="pipeline-runs-empty-state">
              <h3 className="text-base font-semibold text-[var(--text-primary)]">No scopes found</h3>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">{emptyMessage}</p>
            </div>
          )}
        </ResourcePanel>
      </div>
    </ScopeWorkspace>
  );
}

function ScopeWorkspace({
  treeRoot,
  activeTeam,
  onOpenTeam,
  children,
}: {
  treeRoot: ResourceCollectionTreeNode;
  activeTeam: string;
  onOpenTeam: (path: string) => void;
  children: ReactNode;
}) {
  return (
    <ResourceCollectionWorkspace
      treeTitle="Teams"
      rootLabel="All teams"
      searchLabel="Search scope teams"
      searchPlaceholder=""
      showTreeSearch={false}
      emptyLabel="No scope teams found."
      activePath={activeTeam}
      rootNode={treeRoot}
      resizeStorageKey="scopes"
      resizeLabel="Resize scope team tree"
      onOpenPath={onOpenTeam}
    >
      {children}
    </ResourceCollectionWorkspace>
  );
}

function ResourcePanel({
  title,
  countLabel,
  children,
}: {
  title: string;
  countLabel: string;
  children: ReactNode;
}) {
  const titleID = `${title.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-title`;
  return (
    <section className="pipeline-runs-panel resource-collection-panel" aria-labelledby={titleID}>
      <header className="pipeline-runs-panel-head">
        <div className="pipeline-runs-panel-title">
          <h2 id={titleID}>{title}</h2>
          <span>{countLabel}</span>
        </div>
      </header>
      {children}
    </section>
  );
}

function ScopeTable({
  scopes,
  scopeDataByScope,
  onSelectScope,
}: {
  scopes: ScopeEntry[];
  scopeDataByScope: Record<string, ScopeData>;
  onSelectScope: (scopeLabel: string) => void;
}) {
  return (
    <div className="pipeline-runs-table-wrap">
      <table className="pipeline-runs-table resource-collection-table resource-collection-table--scopes" data-testid="scopes-resource-table">
        <thead>
          <tr>
            <th>Scope</th>
            <th>Team</th>
            <th>Variables</th>
            <th>Secrets</th>
            <th>Identifier</th>
            <th>
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {scopes.map(scope => (
            <ScopeRow
              key={scope.scope || '__default__'}
              scope={scope}
              data={scopeDataByScope[scope.scope]}
              onSelectScope={onSelectScope}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ScopeRow({
  scope,
  data,
  onSelectScope,
}: {
  scope: ScopeEntry;
  data?: ScopeData;
  onSelectScope: (scopeLabel: string) => void;
}) {
  const displayName = scope.label || formatScopeDisplay(scope.scope);
  const scopeLabel = formatScopeDisplay(scope.scope);
  const variableCount = data?.variablesLoaded ? data.variables.length : 0;
  const secretCount = data?.secretsLoaded ? data.secrets.length : scope.secretCountHint;
  const variableLabel = `${variableCount} variable${variableCount === 1 ? '' : 's'}`;
  const secretLabel = `${secretCount} secret${secretCount === 1 ? '' : 's'}`;
  const identifier = scope.scope || 'default';

  return (
    <tr>
      <td>
        <button type="button" className="pipeline-runs-table-title" onClick={() => onSelectScope(scope.scope)}>
          <span title={displayName}>{displayName}</span>
          <small>{scopeLabel}</small>
        </button>
      </td>
      <td>
        <span className="pipeline-runs-mono">{scope.teamPath || 'root'}</span>
      </td>
      <td className="pipeline-runs-mono">{variableLabel}</td>
      <td className="pipeline-runs-mono">{secretLabel}</td>
      <td className="pipeline-runs-mono">{identifier}</td>
      <td>
        <div className="pipeline-runs-row-actions">
          <button
            type="button"
            className="pipeline-runs-icon-button"
            onClick={() => onSelectScope(scope.scope)}
            aria-label={`Open scope ${displayName}`}
          >
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </td>
    </tr>
  );
}

function toResourceCollectionTreeNode(node: ScopeTreeNode): ResourceCollectionTreeNode {
  return {
    id: node.id,
    name: node.name,
    fullPath: node.fullPath,
    resourceIds: [...node.scopes],
    children: node.children.map(toResourceCollectionTreeNode),
  };
}
