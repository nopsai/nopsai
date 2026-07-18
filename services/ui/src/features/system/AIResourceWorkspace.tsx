import { useMemo, useState, type ReactNode, type RefObject } from 'react';
import { ArrowLeft, ChevronRight, FolderTree } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import type { ObjectIconType } from '../../components/objectIconRegistry';
import { TreeColumnResizeHandle } from '../../components/resizableTreeColumn';
import { useResizableTreeColumn } from '../../components/resizableTreeColumnState';
import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  normalizeAIResourceTeamPath,
} from './aiResourceTeams';
import {
  AI_RESOURCE_TREE_GLOBAL_ID,
  AI_RESOURCE_TREE_ROOT_ID,
  aiResourceTreeAncestorIDs,
  aiResourceTreeFilterIsAll,
  aiResourceTreeFilterIsGlobal,
  buildAIResourceTree,
  countAIResourceTreeResources,
  type AIResourceTreeNode,
} from './aiResourceTree';

export type AIResourceWorkspaceItem = {
  id: string;
  label: string;
  description?: string;
};

export type AIResourceWorkspaceMetric = {
  label: string;
  value: ReactNode;
  icon: ReactNode;
  tone?: 'default' | 'ok' | 'info' | 'warning' | 'muted';
};

export function AIResourceMetricGrid({ metrics }: { metrics: AIResourceWorkspaceMetric[] }) {
  return (
    <div className="ai-resource-metrics-grid" aria-label="Resource summary">
      {metrics.map(metric => (
        <div key={metric.label} className={`ai-resource-metric ai-resource-metric--${metric.tone || 'default'}`}>
          <span className="ai-resource-metric-icon" aria-hidden="true">{metric.icon}</span>
          <span className="ai-resource-metric-label">{metric.label}</span>
          <strong className="ai-resource-metric-value">{metric.value}</strong>
        </div>
      ))}
    </div>
  );
}

export function AIResourceWorkspace({
  storageKey,
  workspaceLabel,
  treeTitle,
  resourceType,
  resourceLabel,
  resources,
  teamPaths,
  teamFilter,
  selectedResourceID,
  onTeamFilterChange,
  onResourceSelect,
  onDetailClose,
  detailOpen,
  listHeader,
  list,
  detail,
  detailRef,
  detailLabel,
}: {
  storageKey: string;
  workspaceLabel: string;
  treeTitle: string;
  resourceType: ObjectIconType;
  resourceLabel: string;
  resources: AIResourceWorkspaceItem[];
  teamPaths: string[];
  teamFilter: string;
  selectedResourceID: string | null;
  onTeamFilterChange: (value: string) => void;
  onResourceSelect: (id: string) => void;
  onDetailClose: () => void;
  detailOpen?: boolean;
  listHeader: ReactNode;
  list: ReactNode;
  detail: ReactNode;
  detailRef: RefObject<HTMLElement | null>;
  detailLabel: string;
}) {
  const treeResize = useResizableTreeColumn({
    storageKey,
    defaultWidth: 280,
    minWidth: 240,
    maxWidth: 520,
  });
  const tree = useMemo(
    () => buildAIResourceTree(resources.map(resource => resource.id), teamPaths),
    [resources, teamPaths]
  );
  const resourceByID = useMemo(
    () => new Map(resources.map(resource => [resource.id, resource])),
    [resources]
  );
  const showDetail = detailOpen === true || Boolean(selectedResourceID);

  return (
    <section className="ai-resource-table-card ai-resource-workspace-card" aria-label={workspaceLabel}>
      <div className={`ai-resource-browser ${showDetail ? 'ai-resource-browser--detail' : 'ai-resource-browser--list'}`} style={treeResize.gridStyle}>
        <AIResourceExplorerTree
          tree={tree}
          resourceByID={resourceByID}
          treeTitle={treeTitle}
          resourceType={resourceType}
          resourceLabel={resourceLabel}
          teamFilter={teamFilter}
          selectedResourceID={selectedResourceID}
          onTeamFilterChange={onTeamFilterChange}
          onResourceSelect={onResourceSelect}
        />
        <TreeColumnResizeHandle {...treeResize} label={`Resize ${treeTitle.toLowerCase()}`} />
        {showDetail ? (
          <section ref={detailRef} className="ai-resource-detail-fullscreen-main" aria-label={detailLabel}>
            <div className="ai-resource-detail-scroll">
              <div className="ai-resource-detail-toolbar">
                <button type="button" className="ai-resource-mini-button" onClick={onDetailClose}>
                  <ArrowLeft className="h-4 w-4" aria-hidden="true" />
                  <span>List</span>
                </button>
              </div>
              {detail}
            </div>
          </section>
        ) : (
          <section className="ai-resource-browser-main" aria-label={`${resourceLabel} list`}>
            {listHeader}
            <div className="ai-resource-browser-list">
              {list}
            </div>
          </section>
        )}
      </div>
    </section>
  );
}

function AIResourceExplorerTree({
  tree,
  resourceByID,
  treeTitle,
  resourceType,
  resourceLabel,
  teamFilter,
  selectedResourceID,
  onTeamFilterChange,
  onResourceSelect,
}: {
  tree: AIResourceTreeNode;
  resourceByID: Map<string, AIResourceWorkspaceItem>;
  treeTitle: string;
  resourceType: ObjectIconType;
  resourceLabel: string;
  teamFilter: string;
  selectedResourceID: string | null;
  onTeamFilterChange: (value: string) => void;
  onResourceSelect: (id: string) => void;
}) {
  const [nodeOpenOverrides, setNodeOpenOverrides] = useState<Map<string, boolean>>(() => new Map());
  const normalizedTeamFilter = normalizeAIResourceTeamPath(teamFilter);

  const forcedOpenNodeIDs = useMemo(() => {
    const ids = new Set<string>([AI_RESOURCE_TREE_ROOT_ID]);
    if (selectedResourceID) {
      const selectedResource = resourceByID.get(selectedResourceID);
      if (selectedResource) {
        const teamPath = selectedResource.id.split('/').slice(0, -1).join('/');
        aiResourceTreeAncestorIDs(teamPath).forEach(id => ids.add(id));
        if (!teamPath) ids.add(AI_RESOURCE_TREE_GLOBAL_ID);
      }
    }
    if (!aiResourceTreeFilterIsAll(teamFilter) && !aiResourceTreeFilterIsGlobal(teamFilter)) {
      aiResourceTreeAncestorIDs(teamFilter).forEach(id => ids.add(id));
    }
    if (aiResourceTreeFilterIsGlobal(teamFilter)) ids.add(AI_RESOURCE_TREE_GLOBAL_ID);
    return ids;
  }, [resourceByID, selectedResourceID, teamFilter]);

  const openNodeIDs = useMemo(() => {
    const ids = new Set(forcedOpenNodeIDs);
    nodeOpenOverrides.forEach((open, id) => {
      if (open) ids.add(id);
      else ids.delete(id);
    });
    forcedOpenNodeIDs.forEach(id => ids.add(id));
    return ids;
  }, [forcedOpenNodeIDs, nodeOpenOverrides]);

  const toggleNode = (id: string) => {
    setNodeOpenOverrides(previous => {
      const next = new Map(previous);
      next.set(id, !openNodeIDs.has(id));
      return next;
    });
  };

  const total = countAIResourceTreeResources(tree);
  const globalOpen = openNodeIDs.has(AI_RESOURCE_TREE_GLOBAL_ID);

  return (
    <aside className="ai-resource-explorer" aria-label={treeTitle}>
      <div className="ai-resource-explorer-head">
        <span className="ai-resource-explorer-head-icon" aria-hidden="true">
          <FolderTree className="h-4 w-4" />
        </span>
        <div>
          <h3>{treeTitle}</h3>
          <p>{total} total</p>
        </div>
      </div>

      <button
        type="button"
        className={`ai-resource-explorer-root ${aiResourceTreeFilterIsAll(teamFilter) && !selectedResourceID ? 'active' : ''}`}
        onClick={() => onTeamFilterChange(AI_RESOURCE_TEAM_FILTER_ALL)}
      >
        <span className="ai-resource-explorer-folder" aria-hidden="true">
          <ObjectIcon type="team" />
        </span>
        <span>All teams</span>
        <strong>{total}</strong>
      </button>

      {tree.resourceIDs.length ? (
        <div className="ai-resource-explorer-node-row">
          <button
            type="button"
            className="ai-resource-explorer-toggle"
            aria-label={`${globalOpen ? 'Collapse' : 'Expand'} Global`}
            aria-expanded={globalOpen}
            onClick={() => toggleNode(AI_RESOURCE_TREE_GLOBAL_ID)}
          >
            <ChevronRight className={`h-3.5 w-3.5 ${globalOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
          </button>
          <button
            type="button"
            className={`ai-resource-explorer-owner ${aiResourceTreeFilterIsGlobal(teamFilter) && !selectedResourceID ? 'active' : ''}`}
            onClick={() => onTeamFilterChange(AI_RESOURCE_TEAM_FILTER_GLOBAL)}
          >
            <span className="ai-resource-explorer-folder" aria-hidden="true">
              <ObjectIcon type="team" />
            </span>
            <span className="truncate">Global</span>
            <strong>{tree.resourceIDs.length}</strong>
          </button>
        </div>
      ) : null}
      {globalOpen ? (
        <ul className="ai-resource-explorer-children">
          {tree.resourceIDs.map(id => (
            <AIResourceExplorerLeaf
              key={id}
              resource={resourceByID.get(id)}
              resourceID={id}
              resourceType={resourceType}
              resourceLabel={resourceLabel}
              selected={selectedResourceID === id}
              onResourceSelect={onResourceSelect}
            />
          ))}
        </ul>
      ) : null}

      <ul className="ai-resource-explorer-tree">
        {tree.children.map(child => (
          <AIResourceExplorerNode
            key={child.id}
            node={child}
            openNodeIDs={openNodeIDs}
            resourceByID={resourceByID}
            resourceType={resourceType}
            resourceLabel={resourceLabel}
            teamFilter={normalizedTeamFilter}
            selectedResourceID={selectedResourceID}
            onToggleNode={toggleNode}
            onTeamFilterChange={onTeamFilterChange}
            onResourceSelect={onResourceSelect}
          />
        ))}
      </ul>
    </aside>
  );
}

function AIResourceExplorerNode({
  node,
  openNodeIDs,
  resourceByID,
  resourceType,
  resourceLabel,
  teamFilter,
  selectedResourceID,
  onToggleNode,
  onTeamFilterChange,
  onResourceSelect,
}: {
  node: AIResourceTreeNode;
  openNodeIDs: Set<string>;
  resourceByID: Map<string, AIResourceWorkspaceItem>;
  resourceType: ObjectIconType;
  resourceLabel: string;
  teamFilter: string;
  selectedResourceID: string | null;
  onToggleNode: (id: string) => void;
  onTeamFilterChange: (value: string) => void;
  onResourceSelect: (id: string) => void;
}) {
  const open = openNodeIDs.has(node.id);
  const active = teamFilter === node.fullPath && !selectedResourceID;
  const total = countAIResourceTreeResources(node);

  return (
    <li className="ai-resource-explorer-node">
      <div className="ai-resource-explorer-node-row">
        <button
          type="button"
          className="ai-resource-explorer-toggle"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${node.fullPath}`}
          aria-expanded={open}
          onClick={() => onToggleNode(node.id)}
        >
          <ChevronRight className={`h-3.5 w-3.5 ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={`ai-resource-explorer-owner ${active ? 'active' : ''}`}
          aria-label={`Open team ${node.fullPath}`}
          onClick={() => onTeamFilterChange(node.fullPath)}
        >
          <span className="ai-resource-explorer-folder" aria-hidden="true">
            <ObjectIcon type="team" />
          </span>
          <span className="truncate">{node.name}</span>
          <strong>{total}</strong>
        </button>
      </div>

      {open ? (
        <ul className="ai-resource-explorer-children">
          {node.children.map(child => (
            <AIResourceExplorerNode
              key={child.id}
              node={child}
              openNodeIDs={openNodeIDs}
              resourceByID={resourceByID}
              resourceType={resourceType}
              resourceLabel={resourceLabel}
              teamFilter={teamFilter}
              selectedResourceID={selectedResourceID}
              onToggleNode={onToggleNode}
              onTeamFilterChange={onTeamFilterChange}
              onResourceSelect={onResourceSelect}
            />
          ))}
          {node.resourceIDs.map(id => (
            <AIResourceExplorerLeaf
              key={id}
              resource={resourceByID.get(id)}
              resourceID={id}
              resourceType={resourceType}
              resourceLabel={resourceLabel}
              selected={selectedResourceID === id}
              onResourceSelect={onResourceSelect}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

function AIResourceExplorerLeaf({
  resource,
  resourceID,
  resourceType,
  resourceLabel,
  selected,
  onResourceSelect,
}: {
  resource?: AIResourceWorkspaceItem;
  resourceID: string;
  resourceType: ObjectIconType;
  resourceLabel: string;
  selected: boolean;
  onResourceSelect: (id: string) => void;
}) {
  const label = resource?.label || resourceID.split('/').filter(Boolean).pop() || resourceID;

  return (
    <li className="ai-resource-explorer-leaf">
      <button
        type="button"
        className={`ai-resource-explorer-resource ${selected ? 'active' : ''}`}
        aria-label={`Select ${resourceLabel} ${label}`}
        onClick={() => onResourceSelect(resourceID)}
      >
        <span className="ai-resource-explorer-resource-icon" aria-hidden="true">
          <ObjectIcon type={resourceType} />
        </span>
        <span className="truncate">{label}</span>
      </button>
    </li>
  );
}
