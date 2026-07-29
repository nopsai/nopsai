import { useCallback, useMemo, useState, type ReactNode } from 'react';
import { Boxes, ChevronRight, Search, UsersRound } from 'lucide-react';
import { TreeColumnResizeHandle } from '../../components/resizableTreeColumn';
import { useResizableTreeColumn } from '../../components/resizableTreeColumnState';

export type ResourceCollectionTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: ResourceCollectionTreeNode[];
  resourceIds: string[];
};

type ResourceCollectionNavigationItem = {
  id: string;
  label: string;
  path: string;
  active: boolean;
  expanded: boolean;
  level: number;
  resourceCount: number;
  childCount: number;
};

type ResourceCollectionWorkspaceProps = {
  treeTitle: string;
  rootLabel: string;
  searchLabel: string;
  searchPlaceholder: string;
  emptyLabel: string;
  activePath: string;
  rootNode: ResourceCollectionTreeNode;
  resizeStorageKey: string;
  resizeLabel: string;
  onOpenPath: (path: string) => void;
  children: ReactNode;
};

export function ResourceCollectionWorkspace({
  treeTitle,
  rootLabel,
  searchLabel,
  searchPlaceholder,
  emptyLabel,
  activePath,
  rootNode,
  resizeStorageKey,
  resizeLabel,
  onOpenPath,
  children,
}: ResourceCollectionWorkspaceProps) {
  const [treeSearch, setTreeSearch] = useState('');
  const [expandedNodeIds, setExpandedNodeIds] = useState<Set<string>>(new Set());
  const [collapsedNodeIds, setCollapsedNodeIds] = useState<Set<string>>(new Set());
  const navigationItems = useMemo(
    () => buildResourceCollectionNavigationItems(rootNode, activePath, treeSearch, expandedNodeIds, collapsedNodeIds),
    [activePath, collapsedNodeIds, expandedNodeIds, rootNode, treeSearch]
  );
  const treeResize = useResizableTreeColumn({
    storageKey: resizeStorageKey,
    defaultWidth: 248,
    minWidth: 216,
    maxWidth: 480,
  });
  const handleToggleNode = useCallback((item: ResourceCollectionNavigationItem) => {
    if (!item.childCount) return;
    setExpandedNodeIds(prev => {
      const next = new Set(prev);
      if (item.expanded) {
        next.delete(item.id);
      } else {
        next.add(item.id);
      }
      return next;
    });
    setCollapsedNodeIds(prev => {
      const next = new Set(prev);
      if (item.expanded) {
        next.add(item.id);
      } else {
        next.delete(item.id);
      }
      return next;
    });
  }, []);

  return (
    <div className="pipeline-runs-workspace resource-collection-workspace" style={treeResize.gridStyle}>
      <ResourceCollectionTreeRail
        title={treeTitle}
        rootLabel={rootLabel}
        searchLabel={searchLabel}
        searchPlaceholder={searchPlaceholder}
        emptyLabel={emptyLabel}
        rootCount={countResourceCollectionItems(rootNode)}
        activePath={normalizeResourceTreePath(activePath)}
        navigationItems={navigationItems}
        treeSearch={treeSearch}
        onTreeSearchChange={setTreeSearch}
        onOpenPath={onOpenPath}
        onToggleNode={handleToggleNode}
      />
      <TreeColumnResizeHandle {...treeResize} label={resizeLabel} />
      <div className="pipeline-runs-overview-main">
        {children}
      </div>
    </div>
  );
}

function ResourceCollectionTreeRail({
  title,
  rootLabel,
  searchLabel,
  searchPlaceholder,
  emptyLabel,
  rootCount,
  activePath,
  navigationItems,
  treeSearch,
  onTreeSearchChange,
  onOpenPath,
  onToggleNode,
}: {
  title: string;
  rootLabel: string;
  searchLabel: string;
  searchPlaceholder: string;
  emptyLabel: string;
  rootCount: number;
  activePath: string;
  navigationItems: ResourceCollectionNavigationItem[];
  treeSearch: string;
  onTreeSearchChange: (value: string) => void;
  onOpenPath: (path: string) => void;
  onToggleNode: (item: ResourceCollectionNavigationItem) => void;
}) {
  return (
    <aside className="pipeline-runs-scope-rail" aria-label={title}>
      <div className="pipeline-runs-scope-head">
        <h2>{title}</h2>
      </div>
      <label className="pipeline-runs-scope-search">
        <Search className="h-4 w-4" aria-hidden="true" />
        <span className="sr-only">{searchLabel}</span>
        <input
          value={treeSearch}
          onChange={event => onTreeSearchChange(event.target.value)}
          placeholder={searchPlaceholder}
        />
      </label>
      <div className="pipeline-runs-scope-list">
        <button
          type="button"
          className={`pipeline-runs-scope-item ${!activePath ? 'pipeline-runs-scope-item--active' : ''}`}
          onClick={() => onOpenPath('')}
          aria-pressed={!activePath}
        >
          <Boxes className="h-4 w-4" aria-hidden="true" />
          <span className="pipeline-runs-scope-label">{rootLabel}</span>
          <span className="resource-collection-tree-count">{rootCount}</span>
        </button>
        {navigationItems.length === 0 ? (
          <div className="pipeline-runs-scope-empty">{emptyLabel}</div>
        ) : (
          navigationItems.map(item => (
            <div
              key={item.id}
              className="pipeline-runs-scope-row"
              style={{ paddingLeft: `${0.55 + item.level * 0.9}rem` }}
            >
              {item.childCount > 0 ? (
                <button
                  type="button"
                  className="pipeline-runs-scope-toggle"
                  onClick={() => onToggleNode(item)}
                  aria-label={`${item.expanded ? 'Collapse' : 'Expand'} team ${item.label}`}
                  aria-expanded={item.expanded}
                >
                  <ChevronRight className={`h-3.5 w-3.5 ${item.expanded ? 'rotate-90' : ''}`} aria-hidden="true" />
                </button>
              ) : (
                <span className="pipeline-runs-scope-toggle-spacer" aria-hidden="true" />
              )}
              <button
                type="button"
                className={`pipeline-runs-scope-select ${item.active ? 'pipeline-runs-scope-select--active' : ''}`}
                onClick={() => onOpenPath(item.path)}
                aria-label={`Open team ${item.path || item.label}`}
                aria-pressed={item.active}
              >
                <UsersRound className="h-4 w-4" aria-hidden="true" />
                <span className="pipeline-runs-scope-label" title={item.path || item.label}>{item.label}</span>
                <span className="resource-collection-tree-count">{item.resourceCount}</span>
              </button>
            </div>
          ))
        )}
      </div>
    </aside>
  );
}

function buildResourceCollectionNavigationItems(
  rootNode: ResourceCollectionTreeNode,
  activePath: string,
  treeSearchTerm = '',
  expandedNodeIds: ReadonlySet<string> = new Set(),
  collapsedNodeIds: ReadonlySet<string> = new Set()
): ResourceCollectionNavigationItem[] {
  const term = treeSearchTerm.trim().toLowerCase();
  const normalizedActivePath = normalizeResourceTreePath(activePath);
  const activePathIds = new Set(resourceTreeAncestorIds(normalizedActivePath));
  const visibleIds = term ? buildSearchVisibleResourceTreeIds(rootNode, term) : null;
  const items: ResourceCollectionNavigationItem[] = [];

  const visit = (node: ResourceCollectionTreeNode, level: number) => {
    node.children.forEach(child => {
      if (visibleIds && !visibleIds.has(child.id)) return;
      const expanded = visibleIds
        ? child.children.some(grandchild => visibleIds.has(grandchild.id))
        : !collapsedNodeIds.has(child.id) && (activePathIds.has(child.id) || expandedNodeIds.has(child.id));
      items.push({
        id: child.id,
        label: child.name,
        path: child.fullPath,
        active: normalizedActivePath === child.fullPath,
        expanded,
        level,
        resourceCount: countResourceCollectionItems(child),
        childCount: child.children.length,
      });
      if (expanded) visit(child, level + 1);
    });
  };

  visit(rootNode, 0);
  return items;
}

function countResourceCollectionItems(node: ResourceCollectionTreeNode): number {
  return node.resourceIds.length + node.children.reduce((sum, child) => sum + countResourceCollectionItems(child), 0);
}

function normalizeResourceTreePath(value?: string | null): string {
  return String(value || '')
    .trim()
    .replace(/\/+/g, '/')
    .replace(/^\/+|\/+$/g, '')
    .replace(/^root$/i, '');
}

function buildSearchVisibleResourceTreeIds(rootNode: ResourceCollectionTreeNode, term: string): Set<string> {
  const visibleIds = new Set<string>();
  const visit = (node: ResourceCollectionTreeNode, lineage: string[]) => {
    node.children.forEach(child => {
      const childLineage = [...lineage, child.id];
      if (resourceTreeSearchText(child).includes(term)) {
        childLineage.forEach(id => visibleIds.add(id));
      }
      visit(child, childLineage);
    });
  };
  visit(rootNode, []);
  return visibleIds;
}

function resourceTreeAncestorIds(path: string): string[] {
  const segments = normalizeResourceTreePath(path).split('/').filter(Boolean);
  const ancestors: string[] = [];
  let current = '';
  segments.forEach(segment => {
    current = current ? `${current}/${segment}` : segment;
    ancestors.push(current);
  });
  return ancestors;
}

function resourceTreeSearchText(node: ResourceCollectionTreeNode): string {
  return [node.name, node.fullPath].filter(Boolean).join(' ').toLowerCase();
}
