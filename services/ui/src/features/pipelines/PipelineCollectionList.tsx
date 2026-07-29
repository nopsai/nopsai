import { useMemo, type ReactNode } from 'react';
import { ArrowRight, Trash2 } from 'lucide-react';
import { ResourceCollectionWorkspace, type ResourceCollectionTreeNode } from '../editor/ResourceCollectionWorkspace';
import { formatResourceListUpdatedAt } from '../editor/resourceCollectionModel';
import { normalizePipelineSource, splitIdentifier, type PipelineListItem } from './model';

type PipelineTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: PipelineTreeNode[];
  pipelineIds: string[];
};

type PipelineCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  visiblePipelines: PipelineListItem[];
  treeRoot: PipelineTreeNode;
  activeTeam: string;
  canCreatePipelineHere: boolean;
  canUsePipelineDrafts: boolean;
  canDeletePipelines: boolean;
  onSelectPipeline: (id: string) => void;
  onOpenTeam: (path: string) => void;
  onDeletePipeline: (id: string, name: string) => void;
};

export function PipelineCollectionList({
  listLoading,
  listError,
  visiblePipelines,
  treeRoot,
  activeTeam,
  canCreatePipelineHere,
  canUsePipelineDrafts,
  canDeletePipelines,
  onSelectPipeline,
  onOpenTeam,
  onDeletePipeline,
}: PipelineCollectionListProps) {
  const resourceTreeRoot = useMemo(() => toResourceCollectionTreeNode(treeRoot), [treeRoot]);
  const emptyMessage = canCreatePipelineHere ? 'Create a new pipeline or adjust your filters.' : 'Adjust your filters or check your access.';

  if (listLoading) {
    return (
      <PipelineWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
        <ResourcePanel title="Pipeline definitions" countLabel="Loading" showHeader={false}><div className="pipeline-runs-empty-state">Loading pipelines...</div></ResourcePanel>
      </PipelineWorkspace>
    );
  }

  if (listError) {
    return (
      <PipelineWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
        <ResourcePanel title="Pipeline definitions" countLabel="Error" showHeader={false}><div className="pipeline-runs-empty-state text-red-500">Failed to load pipelines: {listError}</div></ResourcePanel>
      </PipelineWorkspace>
    );
  }

  return (
    <PipelineWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
      <div id="pipelines-list-view" className="pipelines-view pipeline-runs-content-grid">
        <ResourcePanel title="Pipeline definitions" countLabel={`${visiblePipelines.length} visible`} showHeader={false}>
          {visiblePipelines.length ? (
            <PipelineTable
              pipelines={visiblePipelines}
              canUsePipelineDrafts={canUsePipelineDrafts}
              canDeletePipelines={canDeletePipelines}
              onSelectPipeline={onSelectPipeline}
              onDeletePipeline={onDeletePipeline}
            />
          ) : (
            <div id="pipelines-empty" className="pipeline-runs-empty-state">
              <h3 className="text-base font-semibold text-[var(--text-primary)]">No pipelines found</h3>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">{emptyMessage}</p>
            </div>
          )}
        </ResourcePanel>
      </div>
    </PipelineWorkspace>
  );
}

function PipelineWorkspace({
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
      searchLabel="Search pipeline teams"
      searchPlaceholder=""
      showTreeSearch={false}
      emptyLabel="No pipeline teams found."
      activePath={activeTeam}
      rootNode={treeRoot}
      resizeStorageKey="pipelines"
      resizeLabel="Resize pipeline team tree"
      onOpenPath={onOpenTeam}
    >
      {children}
    </ResourceCollectionWorkspace>
  );
}

function ResourcePanel({
  title,
  countLabel,
  showHeader = true,
  children,
}: {
  title: string;
  countLabel: string;
  showHeader?: boolean;
  children: ReactNode;
}) {
  const titleID = `${title.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-title`;
  return (
    <section
      className="pipeline-runs-panel resource-collection-panel"
      aria-labelledby={showHeader ? titleID : undefined}
      aria-label={showHeader ? undefined : title}
    >
      {showHeader ? (
        <header className="pipeline-runs-panel-head">
          <div className="pipeline-runs-panel-title">
            <h2 id={titleID}>{title}</h2>
            <span>{countLabel}</span>
          </div>
        </header>
      ) : null}
      {children}
    </section>
  );
}

function PipelineTable({
  pipelines,
  canUsePipelineDrafts,
  canDeletePipelines,
  onSelectPipeline,
  onDeletePipeline,
}: {
  pipelines: PipelineListItem[];
  canUsePipelineDrafts: boolean;
  canDeletePipelines: boolean;
  onSelectPipeline: (id: string) => void;
  onDeletePipeline: (id: string, name: string) => void;
}) {
  return (
    <div className="pipeline-runs-table-wrap">
      <table className="pipeline-runs-table resource-collection-table resource-collection-table--pipelines" data-testid="pipelines-resource-table">
        <thead>
          <tr>
            <th>Source</th>
            <th>Pipeline</th>
            <th>Team</th>
            <th>Updated</th>
            <th>Version</th>
            <th>Identifier</th>
            <th>
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {pipelines.map(pipeline => (
            <PipelineRow
              key={pipeline.id}
              pipeline={pipeline}
              canUsePipelineDrafts={canUsePipelineDrafts}
              canDeletePipelines={canDeletePipelines}
              onSelectPipeline={onSelectPipeline}
              onDeletePipeline={onDeletePipeline}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function PipelineRow({
  pipeline,
  canUsePipelineDrafts,
  canDeletePipelines,
  onSelectPipeline,
  onDeletePipeline,
}: {
  pipeline: PipelineListItem;
  canUsePipelineDrafts: boolean;
  canDeletePipelines: boolean;
  onSelectPipeline: (id: string) => void;
  onDeletePipeline: (id: string, name: string) => void;
}) {
  const { name, path } = splitIdentifier(pipeline.id);
  const source = normalizePipelineSource(pipeline.source);
  const canDeleteThisPipeline = source === 'draft' ? canUsePipelineDrafts : canDeletePipelines && source !== 'git';
  const displayName = name || pipeline.id;
  const versionLabel = pipeline.version?.trim() || 'latest';
  const updatedLabel = formatResourceListUpdatedAt(pipeline.updatedAt);

  return (
    <tr>
      <td>
        <ResourceSourcePill source={source} />
      </td>
      <td>
        <button type="button" className="pipeline-runs-table-title" onClick={() => onSelectPipeline(pipeline.id)}>
          <span title={displayName}>{displayName}</span>
        </button>
      </td>
      <td>
        <span className="pipeline-runs-mono">{path || 'root'}</span>
      </td>
      <td className="pipeline-runs-mono">{updatedLabel}</td>
      <td className="pipeline-runs-mono">{versionLabel}</td>
      <td className="pipeline-runs-mono">{pipeline.id}</td>
      <td>
        <div className="pipeline-runs-row-actions">
          {canDeleteThisPipeline ? (
            <button
              type="button"
              className="pipeline-runs-icon-button pipeline-runs-icon-button--danger"
              title={source === 'draft' ? 'Discard draft' : 'Delete pipeline'}
              onClick={() => onDeletePipeline(pipeline.id, displayName)}
              aria-label={source === 'draft' ? `Discard draft pipeline ${displayName}` : `Delete pipeline ${displayName}`}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          <button
            type="button"
            className="pipeline-runs-icon-button"
            onClick={() => onSelectPipeline(pipeline.id)}
            aria-label={`Open pipeline ${displayName}`}
          >
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </td>
    </tr>
  );
}

function ResourceSourcePill({ source }: { source: string }) {
  const label = source === 'git' ? 'GitOps' : source === 'draft' ? 'Draft' : 'Database';
  const status = source === 'git' ? 'success' : source === 'draft' ? 'waiting' : 'pending';
  return (
    <span className={`pipeline-runs-status pipeline-runs-status-${status}`}>
      <span aria-hidden="true" />
      {label}
    </span>
  );
}

function toResourceCollectionTreeNode(node: PipelineTreeNode): ResourceCollectionTreeNode {
  return {
    id: node.id,
    name: node.name,
    fullPath: node.fullPath,
    resourceIds: node.pipelineIds,
    children: node.children.map(toResourceCollectionTreeNode),
  };
}
