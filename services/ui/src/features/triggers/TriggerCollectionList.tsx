import { Trash2 } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
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
  activeTeamNode: TriggerTreeNode;
  searchTerm: string;
  selectedSlug: string | null;
  canCreateTriggerHere: boolean;
  canDeleteTriggers: boolean;
  onSelectTrigger: (slug: string) => void;
  onOpenTeam: (path: string) => void;
  onDeleteTrigger: (slug: string) => void;
};

export function TriggerCollectionList({
  listLoading,
  listError,
  visibleTriggers,
  activeTeamNode,
  searchTerm,
  selectedSlug,
  canCreateTriggerHere,
  canDeleteTriggers,
  onSelectTrigger,
  onOpenTeam,
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

            {searchTerm.trim() ? null : activeTeamNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeTeamNode.children.map(child => (
                  <TriggerTeamCard key={`team-${child.id}`} node={child} onOpenTeam={onOpenTeam} />
                ))}
              </div>
            ) : null}

            {!visibleTriggers.length && !activeTeamNode.children.length && (
              <div id="triggers-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No triggers found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreateTriggerHere ? 'Create a new trigger or adjust your filters.' : 'Adjust your filters or browse another team.'}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function TriggerTeamCard({ node, onOpenTeam }: { node: TriggerTreeNode; onOpenTeam: (path: string) => void }) {
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
          </div>
        </div>
        <span className="pipeline-team-chevron">›</span>
      </div>
      <div className="pipeline-team-meta">
        <div className="pipeline-team-meta-row">
          <span className="pipeline-card-meta-label">Triggers:</span>
          <span className="pipeline-card-meta-value">{countTriggersRecursive(node)}</span>
        </div>
        <div className="pipeline-team-meta-row">
          <span className="pipeline-card-meta-label">Sub teams:</span>
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
          <span className="pipeline-card-icon" aria-hidden="true">
            <ObjectIcon type="trigger" />
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
              title={sourceKey === 'git' ? 'Delete database row; GitOps can recreate it on the next sync' : 'Delete trigger'}
              onClick={event => {
                event.stopPropagation();
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
