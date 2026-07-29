import type { ReactNode } from 'react';
import { Database, Folder, GitBranch, Trash2, UsersRound, Zap } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { TreeColumnResizeHandle } from '../../components/resizableTreeColumn';
import { useResizableTreeColumn } from '../../components/resizableTreeColumnState';
import { TriggerExplorerTree } from './TriggerExplorerTree';
import {
  normalizeSource,
  sourceLabel,
  triggerIngressLabel,
  triggerScopesLabel,
  triggerSlugLabel,
  type TriggerCollectionMetrics,
  type TriggerListItem,
} from './model';
import type { TriggerTreeNode } from './treeModel';

type TriggerCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  allTriggers: TriggerListItem[];
  visibleTriggers: TriggerListItem[];
  treeRoot: TriggerTreeNode;
  activeOwner: string;
  searchTerm: string;
  selectedSlug: string | null;
  canCreateTriggerHere: boolean;
  canDeleteTriggers: boolean;
  onSelectTrigger: (slug: string) => void;
  onOpenOwner: (ownerPath: string) => void;
  onOpenTeam: (ownerPath: string, teamPath: string) => void;
  onDeleteTrigger: (slug: string) => void;
};

export function TriggerCollectionList({
  listLoading,
  listError,
  allTriggers,
  visibleTriggers,
  treeRoot,
  activeOwner,
  searchTerm,
  selectedSlug,
  canCreateTriggerHere,
  canDeleteTriggers,
  onSelectTrigger,
  onOpenOwner,
  onOpenTeam,
  onDeleteTrigger,
}: TriggerCollectionListProps) {
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
        activeOwnerPath={activeOwnerPath}
        activeTeamPath={activeTeamPath}
        selectedSlug={selectedSlug}
        onOpenOwner={onOpenOwner}
        onOpenTeam={onOpenTeam}
        onSelectTrigger={onSelectTrigger}
      />
      <TreeColumnResizeHandle {...treeResize} label="Resize trigger tree" />
      <section className="triggers-browser-main" aria-label="Trigger collection">
        <div className="triggers-list-container">
          {listLoading ? (
            <div className="triggers-workspace-empty">Loading triggers...</div>
          ) : listError ? (
            <div className="triggers-workspace-empty triggers-workspace-empty--error">Failed to load triggers: {listError}</div>
          ) : (
            <>
              <div className="triggers-resource-table-shell">
                {visibleTriggers.length ? (
                  <table className="triggers-resource-table triggers-resource-table--triggers">
                    <thead>
                      <tr>
                        <th scope="col">Trigger</th>
                        <th scope="col">Provider</th>
                        <th scope="col">Ingress</th>
                        <th scope="col">Scopes</th>
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
                    <strong>{searchTerm.trim() ? 'No matching triggers' : 'No triggers for this owner/team scope'}</strong>
                    <span>{canCreateTriggerHere ? 'Create a trigger or adjust your filters.' : 'Adjust your filters or browse another owner or team.'}</span>
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

export function TriggerMetricGrid({ metrics }: { metrics: TriggerCollectionMetrics }) {
  return (
    <div className="triggers-metrics-grid" aria-label="Trigger summary">
      <TriggerMetric icon={<Zap className="h-4 w-4" aria-hidden="true" />} label="Triggers" value={metrics.total} />
      <TriggerMetric icon={<GitBranch className="h-4 w-4" aria-hidden="true" />} label="GitOps" value={metrics.gitManaged} />
      <TriggerMetric icon={<Database className="h-4 w-4" aria-hidden="true" />} label="Overrides" value={metrics.databaseManaged} />
      <TriggerMetric icon={<UsersRound className="h-4 w-4" aria-hidden="true" />} label="Owners" value={metrics.ownerCount} />
      <TriggerMetric icon={<Folder className="h-4 w-4" aria-hidden="true" />} label="Teams" value={metrics.teamCount} />
    </div>
  );
}

function triggerCollectionScopeLabel(ownerPath: string, teamPath: string) {
  const owner = ownerPath.trim();
  const team = teamPath.trim();
  if (!owner && !team) return 'All owners';
  const teamLabel = team === 'root' ? 'Workspace' : team;
  if (owner && team) return `${owner} / ${teamLabel}`;
  if (owner) return owner;
  return `All owners / ${teamLabel}`;
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
        <span className="triggers-mono">{triggerScopesLabel(item.scopes)}</span>
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
