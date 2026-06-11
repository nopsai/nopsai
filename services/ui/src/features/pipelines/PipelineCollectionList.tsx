import { Trash2 } from 'lucide-react';
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
  activeFolderNode: PipelineTreeNode;
  searchTerm: string;
  canCreatePipelineHere: boolean;
  canUsePipelineDrafts: boolean;
  canDeletePipelines: boolean;
  onSelectPipeline: (id: string) => void;
  onOpenFolder: (path: string) => void;
  onDeletePipeline: (id: string, name: string) => void;
};

export function PipelineCollectionList({
  listLoading,
  listError,
  visiblePipelines,
  activeFolderNode,
  searchTerm,
  canCreatePipelineHere,
  canUsePipelineDrafts,
  canDeletePipelines,
  onSelectPipeline,
  onOpenFolder,
  onDeletePipeline,
}: PipelineCollectionListProps) {
  return (
    <div id="pipelines-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading pipelines...</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load pipelines: {listError}</div>
        ) : (
          <>
            {visiblePipelines.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">
                {visiblePipelines.map(pipeline => (
                  <PipelineCard
                    key={pipeline.id}
                    pipeline={pipeline}
                    canUsePipelineDrafts={canUsePipelineDrafts}
                    canDeletePipelines={canDeletePipelines}
                    onSelectPipeline={onSelectPipeline}
                    onDeletePipeline={onDeletePipeline}
                  />
                ))}
              </div>
            ) : null}

            {searchTerm.trim() ? null : activeFolderNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeFolderNode.children.map(child => (
                  <PipelineFolderCard key={`folder-${child.id}`} node={child} onOpenFolder={onOpenFolder} />
                ))}
              </div>
            ) : null}

            {!visiblePipelines.length && !activeFolderNode.children.length && (
              <div id="pipelines-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No pipelines found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreatePipelineHere ? 'Create a new pipeline or adjust your filters.' : 'Adjust your filters or check your access.'}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function PipelineCard({
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

  return (
    <article className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4" onClick={() => onSelectPipeline(pipeline.id)}>
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M6 12h2m8 0h2M12 6v2m0 8v2" />
            </svg>
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{name || pipeline.id}</h3>
            <p className="pipeline-card-path">{path || 'root'}</p>
            <p className="pipeline-card-description">A sample pipeline.</p>
          </div>
        </div>
        <div className="pipeline-card-actions">
          {canDeleteThisPipeline ? (
            <button
              type="button"
              className="pipelines-delete-button"
              title={source === 'draft' ? 'Discard draft' : 'Delete pipeline'}
              onClick={event => {
                event.stopPropagation();
                onDeletePipeline(pipeline.id, name || pipeline.id);
              }}
              aria-label={source === 'draft' ? 'Discard draft pipeline' : 'Delete pipeline'}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>
      <div className="pipeline-card-meta">
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Version</span>
          <span className="pipeline-card-meta-value">latest</span>
        </div>
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Source</span>
          <span className="pipeline-card-meta-value">{source}</span>
        </div>
      </div>
    </article>
  );
}

function PipelineFolderCard({ node, onOpenFolder }: { node: PipelineTreeNode; onOpenFolder: (path: string) => void }) {
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
          </div>
        </div>
        <span className="pipeline-folder-chevron">›</span>
      </div>
      <div className="pipeline-folder-meta">
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Pipelines:</span>
          <span className="pipeline-card-meta-value">{node.pipelineIds.length}</span>
        </div>
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Sub groups:</span>
          <span className="pipeline-card-meta-value">{node.children.length}</span>
        </div>
      </div>
    </article>
  );
}
