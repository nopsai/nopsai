import type { ReactNode } from 'react';
import { Database, Folder, GitBranch, Trash2, Zap } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { TreeColumnResizeHandle, useResizableTreeColumn } from '../../components/resizableTreeColumn';
import { TriggerExplorerTree } from './TriggerExplorerTree';
import {
  buildTriggerCollectionMetrics,
  normalizeSource,
  sourceLabel,
  triggerAllowlistStatusLabel,
  triggerIngressLabel,
  triggerSlugLabel,
  type TriggerListItem,
} from './model';
import type { TriggerTreeNode } from './treeModel';

type TriggerCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  allTriggers: TriggerListItem[];
  visibleTriggers: TriggerListItem[];
  treeRoot: TriggerTreeNode;
  activeOwnerNode: TriggerTreeNode;
  activeOwner: string;
  searchTerm: string;
  selectedSlug: string | null;
  canCreateTriggerHere: boolean;
  canDeleteTriggers: boolean;
  onSelectTrigger: (slug: string) => void;
  onOpenOwner: (path: string) => void;
  onDeleteTrigger: (slug: string) => void;
};

export function TriggerCollectionList({
  listLoading,
  listError,
  allTriggers,
  visibleTriggers,
  treeRoot,
  activeOwnerNode,
  activeOwner,
  searchTerm,
  selectedSlug,
  canCreateTriggerHere,
  canDeleteTriggers,
  onSelectTrigger,
  onOpenOwner,
  onDeleteTrigger,
}: TriggerCollectionListProps) {
  const metrics = buildTriggerCollectionMetrics(allTriggers, activeOwner);
  const activeOwnerLabel = activeOwner ? activeOwner : 'All owners';
  const treeResize = useResizableTreeColumn({
    storageKey: 'triggers',
    defaultWidth: 280,
    minWidth: 240,
    maxWidth: 520,
  });

  return (
    <div id="triggers-list-view" className="triggers-workspace-list triggers-browser" style={treeResize.gridStyle}>
      <TriggerExplorerTree
        rootNode={treeRoot}
        allTriggers={allTriggers}
        activeOwner={activeOwner}
        selectedSlug={selectedSlug}
        onOpenOwner={onOpenOwner}
        onSelectTrigger={onSelectTrigger}
      />
      <TreeColumnResizeHandle {...treeResize} label="Resize trigger tree" />
      <section className="triggers-browser-main" aria-label="Trigger collection">
        <TriggerMetricGrid metrics={metrics} />
        <div className="triggers-list-container">
          {listLoading ? (
            <div className="triggers-workspace-empty">Loading triggers...</div>
          ) : listError ? (
            <div className="triggers-workspace-empty triggers-workspace-empty--error">Failed to load triggers: {listError}</div>
          ) : (
            <>
              <div className="triggers-collection-head">
                <div>
                  <h3>{searchTerm.trim() ? 'Search results' : activeOwnerLabel}</h3>
                  <p>
                    {visibleTriggers.length} trigger{visibleTriggers.length === 1 ? '' : 's'}
                    {searchTerm.trim() ? ` matching "${searchTerm.trim()}"` : ''}
                  </p>
                </div>
                {!searchTerm.trim() && activeOwnerNode.children.length ? (
                  <span className="triggers-badge triggers-badge--neutral">
                    {activeOwnerNode.children.length} nested owner{activeOwnerNode.children.length === 1 ? '' : 's'}
                  </span>
                ) : null}
              </div>

              <div className="triggers-resource-table-shell">
                {visibleTriggers.length ? (
                  <table className="triggers-resource-table triggers-resource-table--triggers">
                    <thead>
                      <tr>
                        <th scope="col">Trigger</th>
                        <th scope="col">Provider</th>
                        <th scope="col">Ingress</th>
                        <th scope="col">Allowlist</th>
                        <th scope="col">Source</th>
                        <th scope="col" aria-label="Actions"></th>
                      </tr>
                    </thead>
                    <tbody>
                      {visibleTriggers.map(item => (
                        <TriggerRow
                          key={item.slug}
                          item={item}
                          selectedSlug={selectedSlug}
                          canDeleteTriggers={canDeleteTriggers}
                          onSelectTrigger={onSelectTrigger}
                          onDeleteTrigger={onDeleteTrigger}
                        />
                      ))}
                    </tbody>
                  </table>
                ) : (
                  <div className="triggers-workspace-empty">
                    <span className="triggers-empty-icon" aria-hidden="true">
                      <ObjectIcon type="trigger" />
                    </span>
                    <strong>{searchTerm.trim() ? 'No matching triggers' : 'No triggers for this owner'}</strong>
                    <span>{canCreateTriggerHere ? 'Create a trigger or adjust your filters.' : 'Adjust your filters or browse another owner.'}</span>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </section>
    </div>
  );
}

function TriggerMetricGrid({ metrics }: { metrics: ReturnType<typeof buildTriggerCollectionMetrics> }) {
  return (
    <div className="triggers-metrics-grid" aria-label="Trigger summary">
      <TriggerMetric icon={<Zap className="h-4 w-4" aria-hidden="true" />} label="Triggers" value={metrics.total} />
      <TriggerMetric icon={<GitBranch className="h-4 w-4" aria-hidden="true" />} label="GitOps" value={metrics.gitManaged} />
      <TriggerMetric icon={<Database className="h-4 w-4" aria-hidden="true" />} label="Overrides" value={metrics.databaseManaged} />
      <TriggerMetric icon={<Folder className="h-4 w-4" aria-hidden="true" />} label="Owners" value={metrics.ownerCount} />
    </div>
  );
}

function TriggerMetric({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
  return (
    <div className="triggers-metric">
      <span className="triggers-metric-icon">{icon}</span>
      <span className="triggers-metric-label">{label}</span>
      <strong className="triggers-metric-value">{value}</strong>
    </div>
  );
}

function TriggerRow({
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
    <tr className={isActive ? 'selected' : ''} onClick={() => onSelectTrigger(item.slug)}>
      <td>
        <button
          type="button"
          className="triggers-resource-cell"
          onClick={event => {
            event.stopPropagation();
            onSelectTrigger(item.slug);
          }}
        >
          <span className="triggers-resource-icon" aria-hidden="true">
            <ObjectIcon type="trigger" />
          </span>
          <span className="triggers-resource-name">
            <strong>{item.slug}</strong>
          </span>
        </button>
      </td>
      <td>
        <span className="triggers-mono">{item.provider || 'github'}</span>
      </td>
      <td>
        <span className="triggers-mono">{triggerIngressLabel(item)}</span>
      </td>
      <td>
        <span className="triggers-mono">{triggerAllowlistStatusLabel(item.allowlistStatus)}</span>
      </td>
      <td>
        <span className={`triggers-badge triggers-badge--${sourceKey === 'git' ? 'blue' : 'neutral'}`}>
          <span className="triggers-badge-dot" aria-hidden="true"></span>
          {sourceLabel(sourceKey)}
        </span>
      </td>
      <td>
        <div className="triggers-row-actions">
          {canDeleteTriggers && (
            <button
              type="button"
              className="triggers-icon-button triggers-icon-button--danger"
              title={sourceKey === 'git' ? 'Delete database row; GitOps can recreate it on the next sync' : 'Delete trigger'}
              onClick={event => {
                event.stopPropagation();
                onDeleteTrigger(item.slug);
              }}
              aria-label={`Delete ${name || item.slug}`}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
        </div>
      </td>
    </tr>
  );
}
