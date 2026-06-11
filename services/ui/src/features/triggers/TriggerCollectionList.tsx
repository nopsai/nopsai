import { Trash2 } from 'lucide-react';
import { normalizeSource, triggerSlugLabel, type TriggerListItem } from './model';

export type TriggerTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TriggerTreeNode[];
  triggerSlugs: string[];
};

type TriggerCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  visibleTriggers: TriggerListItem[];
  activeFolderNode: TriggerTreeNode;
  searchTerm: string;
  selectedSlug: string | null;
  canCreateTriggerHere: boolean;
  canDeleteTriggers: boolean;
  onSelectTrigger: (slug: string) => void;
  onOpenFolder: (path: string) => void;
  onDeleteTrigger: (slug: string) => void;
};

export function TriggerCollectionList({
  listLoading,
  listError,
  visibleTriggers,
  activeFolderNode,
  searchTerm,
  selectedSlug,
  canCreateTriggerHere,
  canDeleteTriggers,
  onSelectTrigger,
  onOpenFolder,
  onDeleteTrigger,
}: TriggerCollectionListProps) {
  return (
    <div id="triggers-list-view" className="pipelines-view">
      <div className="space-y-3 triggers-list-container">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading triggers…</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load triggers: {listError}</div>
        ) : (
          <>
            {searchTerm.trim() ? (
              <div className="triggers-search-summary">
                Showing {visibleTriggers.length} result{visibleTriggers.length === 1 ? '' : 's'} for "{searchTerm.trim()}"
              </div>
            ) : null}

            {visibleTriggers.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">
                {visibleTriggers.map(item => (
                  <TriggerCard
                    key={item.slug}
                    item={item}
                    selectedSlug={selectedSlug}
                    canDeleteTriggers={canDeleteTriggers}
                    onSelectTrigger={onSelectTrigger}
                    onDeleteTrigger={onDeleteTrigger}
                  />
                ))}
              </div>
            ) : null}

            {searchTerm.trim() ? null : activeFolderNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeFolderNode.children.map(child => (
                  <TriggerFolderCard key={`folder-${child.id}`} node={child} onOpenFolder={onOpenFolder} />
                ))}
              </div>
            ) : null}

            {!visibleTriggers.length && !activeFolderNode.children.length && (
              <div id="triggers-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No triggers found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreateTriggerHere ? 'Create a new trigger or adjust your filters.' : 'Adjust your filters or browse another group.'}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function TriggerFolderCard({ node, onOpenFolder }: { node: TriggerTreeNode; onOpenFolder: (path: string) => void }) {
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
          <span className="pipeline-card-meta-label">Triggers:</span>
          <span className="pipeline-card-meta-value">{countTriggersRecursive(node)}</span>
        </div>
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Sub groups:</span>
          <span className="pipeline-card-meta-value">{node.children.length}</span>
        </div>
      </div>
    </article>
  );
}

function TriggerCard({
  item,
  selectedSlug,
  canDeleteTriggers,
  onSelectTrigger,
  onDeleteTrigger,
}: {
  item: TriggerListItem;
  selectedSlug: string | null;
  canDeleteTriggers: boolean;
  onSelectTrigger: (slug: string) => void;
  onDeleteTrigger: (slug: string) => void;
}) {
  const { name } = triggerSlugLabel(item.slug);
  const sourceKey = normalizeSource(item.source);
  const isActive = item.slug === selectedSlug;

  return (
    <article
      className={`glass-card pipeline-card triggers-card border border-[var(--border-primary)] rounded-xl p-4 ${isActive ? 'triggers-card--active' : ''}`}
      onClick={() => onSelectTrigger(item.slug)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="triggers-card-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
              <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
            </svg>
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{name || item.slug}</h3>
          </div>
        </div>
        <div className="pipeline-card-actions">
          {canDeleteTriggers && (
            <button
              type="button"
              className="pipelines-delete-button"
              aria-disabled={sourceKey === 'git'}
              title={sourceKey === 'git' ? 'This trigger is managed via Git. Clone it to customize.' : 'Delete trigger'}
              onClick={event => {
                event.stopPropagation();
                if (sourceKey === 'git') return;
                onDeleteTrigger(item.slug);
              }}
              aria-label="Delete trigger"
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
        </div>
      </div>
      <div className="pipeline-card-meta">
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Source</span>
          <span className="pipeline-card-meta-value">{sourceKey}</span>
        </div>
      </div>
    </article>
  );
}

function countTriggersRecursive(node: TriggerTreeNode): number {
  const own = node.triggerSlugs.length;
  if (!node.children.length) return own;
  return own + node.children.reduce((sum, child) => sum + countTriggersRecursive(child), 0);
}
