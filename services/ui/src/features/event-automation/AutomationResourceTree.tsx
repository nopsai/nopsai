import { useMemo, useState } from 'react';
import { ChevronRight, FolderTree } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import type { ObjectIconType } from '../../components/objectIconRegistry';
import {
  automationResourceTreeAncestorIDs,
  countAutomationResourceTreeItems,
  normalizeAutomationTreePath,
  type AutomationResourceTreeItem,
  type AutomationResourceTreeNode,
} from './resourceTreeModel';
import { eventAutomationSourceKey, eventAutomationSourceLabel } from './sourcePresentation';

type AutomationResourceTreeProps = {
  title: string;
  rootLabel: string;
  rootNode: AutomationResourceTreeNode;
  items: AutomationResourceTreeItem[];
  activePath: string;
  selectedID?: string | null;
  nodeIconType?: ObjectIconType;
  leafIconType: ObjectIconType;
  leafAriaLabel: string;
  onOpenPath: (path: string) => void;
  onSelectItem: (id: string) => void;
};

export function AutomationResourceTree({
  title,
  rootLabel,
  rootNode,
  items,
  activePath,
  selectedID,
  nodeIconType = 'team',
  leafIconType,
  leafAriaLabel,
  onOpenPath,
  onSelectItem,
}: AutomationResourceTreeProps) {
  const itemByID = useMemo(() => new Map(items.map(item => [item.id, item])), [items]);
  const normalizedActivePath = normalizeAutomationTreePath(activePath);
  const rootCount = countAutomationResourceTreeItems(rootNode);
  const [nodeOpenOverrides, setNodeOpenOverrides] = useState<Map<string, boolean>>(() => new Map());

  const forcedOpenNodeIDs = useMemo(() => {
    const ids = new Set<string>(['__root__']);
    automationResourceTreeAncestorIDs(normalizedActivePath).forEach(id => ids.add(id));
    const selectedItem = selectedID ? itemByID.get(selectedID) : null;
    if (selectedItem) {
      automationResourceTreeAncestorIDs(selectedItem.path).forEach(id => ids.add(id));
    }
    return ids;
  }, [itemByID, normalizedActivePath, selectedID]);

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

  return (
    <aside className="triggers-explorer" aria-label={title}>
      <div className="triggers-explorer-head">
        <span className="triggers-explorer-head-icon" aria-hidden="true">
          <FolderTree className="h-4 w-4" />
        </span>
        <div>
          <h3>{title}</h3>
          <p>{countAutomationResourceTreeItems(rootNode)} total</p>
        </div>
      </div>

      <button
        type="button"
        aria-label={`${rootLabel} (${rootCount})`}
        className={`triggers-explorer-root ${!normalizedActivePath && !selectedID ? 'active' : ''}`}
        onClick={() => onOpenPath('')}
      >
        <span className="triggers-explorer-folder" aria-hidden="true">
          <ObjectIcon type={nodeIconType} />
        </span>
        <span>{rootLabel}</span>
        <strong>{rootCount}</strong>
      </button>

      <ul className="triggers-explorer-tree">
        {rootNode.children.map(child => (
          <AutomationResourceTreeNodeRow
            key={child.id}
            node={child}
            itemByID={itemByID}
            openNodeIDs={openNodeIDs}
            activePath={normalizedActivePath}
            selectedID={selectedID || null}
            nodeIconType={nodeIconType}
            leafIconType={leafIconType}
            leafAriaLabel={leafAriaLabel}
            onToggleNode={toggleNode}
            onOpenPath={onOpenPath}
            onSelectItem={onSelectItem}
          />
        ))}
        {rootNode.itemIDs.map(id => (
          <AutomationResourceTreeLeaf
            key={id}
            item={itemByID.get(id)}
            selected={id === selectedID}
            leafIconType={leafIconType}
            leafAriaLabel={leafAriaLabel}
            onSelectItem={onSelectItem}
          />
        ))}
      </ul>
    </aside>
  );
}

function AutomationResourceTreeNodeRow({
  node,
  itemByID,
  openNodeIDs,
  activePath,
  selectedID,
  nodeIconType,
  leafIconType,
  leafAriaLabel,
  onToggleNode,
  onOpenPath,
  onSelectItem,
}: {
  node: AutomationResourceTreeNode;
  itemByID: Map<string, AutomationResourceTreeItem>;
  openNodeIDs: Set<string>;
  activePath: string;
  selectedID: string | null;
  nodeIconType: ObjectIconType;
  leafIconType: ObjectIconType;
  leafAriaLabel: string;
  onToggleNode: (id: string) => void;
  onOpenPath: (path: string) => void;
  onSelectItem: (id: string) => void;
}) {
  const open = openNodeIDs.has(node.id);
  const active = activePath === node.fullPath && !selectedID;
  const total = countAutomationResourceTreeItems(node);

  return (
    <li className="triggers-explorer-node">
      <div className="triggers-explorer-node-row">
        <button
          type="button"
          className="triggers-explorer-toggle"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${node.fullPath}`}
          aria-expanded={open}
          onClick={() => onToggleNode(node.id)}
        >
          <ChevronRight className={`h-3.5 w-3.5 ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={`triggers-explorer-owner ${active ? 'active' : ''}`}
          aria-label={`Open team ${node.fullPath}`}
          onClick={() => onOpenPath(node.fullPath)}
        >
          <span className="triggers-explorer-folder" aria-hidden="true">
            <ObjectIcon type={nodeIconType} />
          </span>
          <span className="truncate">{node.name}</span>
          <strong>{total}</strong>
        </button>
      </div>

      {open ? (
        <ul className="triggers-explorer-children">
          {node.children.map(child => (
            <AutomationResourceTreeNodeRow
              key={child.id}
              node={child}
              itemByID={itemByID}
              openNodeIDs={openNodeIDs}
              activePath={activePath}
              selectedID={selectedID}
              nodeIconType={nodeIconType}
              leafIconType={leafIconType}
              leafAriaLabel={leafAriaLabel}
              onToggleNode={onToggleNode}
              onOpenPath={onOpenPath}
              onSelectItem={onSelectItem}
            />
          ))}
          {node.itemIDs.map(id => (
            <AutomationResourceTreeLeaf
              key={id}
              item={itemByID.get(id)}
              selected={id === selectedID}
              leafIconType={leafIconType}
              leafAriaLabel={leafAriaLabel}
              onSelectItem={onSelectItem}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

function AutomationResourceTreeLeaf({
  item,
  selected,
  leafIconType,
  leafAriaLabel,
  onSelectItem,
}: {
  item?: AutomationResourceTreeItem;
  selected: boolean;
  leafIconType: ObjectIconType;
  leafAriaLabel: string;
  onSelectItem: (id: string) => void;
}) {
  if (!item) return null;
  const sourceKey = eventAutomationSourceKey(item.source);

  return (
    <li className="triggers-explorer-leaf">
      <button
        type="button"
        className={`triggers-explorer-trigger ${selected ? 'active' : ''}`}
        aria-label={`${leafAriaLabel} ${item.label}`}
        onClick={() => onSelectItem(item.id)}
      >
        <span className="triggers-explorer-trigger-icon" aria-hidden="true">
          <ObjectIcon type={leafIconType} />
        </span>
        <span className="truncate">{item.label}</span>
        <span
          className={`triggers-explorer-source triggers-explorer-source--${sourceKey === 'git' ? 'git' : 'database'}`}
          title={eventAutomationSourceLabel(item.source)}
          aria-label={eventAutomationSourceLabel(item.source)}
        />
      </button>
    </li>
  );
}
