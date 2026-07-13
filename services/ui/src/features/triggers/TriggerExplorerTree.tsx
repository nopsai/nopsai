import { useMemo, useState } from 'react';
import { ChevronRight, FolderTree } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { normalizeSource, sourceLabel, triggerSlugLabel, type TriggerListItem } from './model';
import { countTriggerTreeSlugs, triggerTreeAncestorIDs, type TriggerTreeNode } from './treeModel';

type TriggerExplorerTreeProps = {
  rootNode: TriggerTreeNode;
  allTriggers: TriggerListItem[];
  activeOwner: string;
  selectedSlug: string | null;
  onOpenOwner: (path: string) => void;
  onSelectTrigger: (slug: string) => void;
};

export function TriggerExplorerTree({
  rootNode,
  allTriggers,
  activeOwner,
  selectedSlug,
  onOpenOwner,
  onSelectTrigger,
}: TriggerExplorerTreeProps) {
  const sourceBySlug = useMemo(() => {
    return new Map(allTriggers.map(item => [item.slug, item.source]));
  }, [allTriggers]);
  const [nodeOpenOverrides, setNodeOpenOverrides] = useState<Map<string, boolean>>(() => new Map());

  const forcedOpenNodeIDs = useMemo(() => {
    const ids = new Set<string>(['__root__']);
    triggerTreeAncestorIDs(activeOwner).forEach(id => ids.add(id));
    if (selectedSlug) {
      triggerTreeAncestorIDs(triggerSlugLabel(selectedSlug).path).forEach(id => ids.add(id));
    }
    return ids;
  }, [activeOwner, selectedSlug]);

  const defaultOpenNodeIDs = useMemo(() => {
    const ids = new Set(forcedOpenNodeIDs);
    rootNode.children.forEach(child => ids.add(child.id));
    return ids;
  }, [forcedOpenNodeIDs, rootNode]);

  const openNodeIDs = useMemo(() => {
    const ids = new Set(defaultOpenNodeIDs);
    nodeOpenOverrides.forEach((open, id) => {
      if (open) ids.add(id);
      else ids.delete(id);
    });
    forcedOpenNodeIDs.forEach(id => ids.add(id));
    return ids;
  }, [defaultOpenNodeIDs, forcedOpenNodeIDs, nodeOpenOverrides]);

  const toggleNode = (id: string) => {
    setNodeOpenOverrides(previous => {
      const next = new Map(previous);
      next.set(id, !openNodeIDs.has(id));
      return next;
    });
  };

  return (
    <aside className="triggers-explorer" aria-label="Trigger explorer">
      <div className="triggers-explorer-head">
        <span className="triggers-explorer-head-icon" aria-hidden="true">
          <FolderTree className="h-4 w-4" />
        </span>
        <div>
          <h3>Trigger tree</h3>
          <p>{countTriggerTreeSlugs(rootNode)} total</p>
        </div>
      </div>

      <button
        type="button"
        className={`triggers-explorer-root ${!activeOwner && !selectedSlug ? 'active' : ''}`}
        onClick={() => onOpenOwner('')}
      >
        <span className="triggers-explorer-folder" aria-hidden="true">
          <ObjectIcon type="repository-owner" />
        </span>
        <span>All owners</span>
        <strong>{countTriggerTreeSlugs(rootNode)}</strong>
      </button>

      <ul className="triggers-explorer-tree">
        {rootNode.children.map(child => (
          <TriggerExplorerNode
            key={child.id}
            node={child}
            openNodeIDs={openNodeIDs}
            sourceBySlug={sourceBySlug}
            activeOwner={activeOwner}
            selectedSlug={selectedSlug}
            onToggleNode={toggleNode}
            onOpenOwner={onOpenOwner}
            onSelectTrigger={onSelectTrigger}
          />
        ))}
        {rootNode.triggerSlugs.map(slug => (
          <TriggerExplorerLeaf
            key={slug}
            slug={slug}
            source={sourceBySlug.get(slug)}
            selected={slug === selectedSlug}
            onSelectTrigger={onSelectTrigger}
          />
        ))}
      </ul>
    </aside>
  );
}

function TriggerExplorerNode({
  node,
  openNodeIDs,
  sourceBySlug,
  activeOwner,
  selectedSlug,
  onToggleNode,
  onOpenOwner,
  onSelectTrigger,
}: {
  node: TriggerTreeNode;
  openNodeIDs: Set<string>;
  sourceBySlug: Map<string, string | undefined>;
  activeOwner: string;
  selectedSlug: string | null;
  onToggleNode: (id: string) => void;
  onOpenOwner: (path: string) => void;
  onSelectTrigger: (slug: string) => void;
}) {
  const open = openNodeIDs.has(node.id);
  const active = activeOwner === node.fullPath && !selectedSlug;
  const total = countTriggerTreeSlugs(node);

  return (
    <li className="triggers-explorer-node">
      <div className="triggers-explorer-node-row">
        <button
          type="button"
          className="triggers-explorer-toggle"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${node.fullPath}`}
          onClick={() => onToggleNode(node.id)}
        >
          <ChevronRight className={`h-3.5 w-3.5 ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={`triggers-explorer-owner ${active ? 'active' : ''}`}
          aria-label={`Open owner ${node.fullPath}`}
          onClick={() => onOpenOwner(node.fullPath)}
        >
          <span className="triggers-explorer-folder" aria-hidden="true">
            <ObjectIcon type="repository-owner" />
          </span>
          <span className="truncate">{node.name}</span>
          <strong>{total}</strong>
        </button>
      </div>

      {open ? (
        <ul className="triggers-explorer-children">
          {node.children.map(child => (
            <TriggerExplorerNode
              key={child.id}
              node={child}
              openNodeIDs={openNodeIDs}
              sourceBySlug={sourceBySlug}
              activeOwner={activeOwner}
              selectedSlug={selectedSlug}
              onToggleNode={onToggleNode}
              onOpenOwner={onOpenOwner}
              onSelectTrigger={onSelectTrigger}
            />
          ))}
          {node.triggerSlugs.map(slug => (
            <TriggerExplorerLeaf
              key={slug}
              slug={slug}
              source={sourceBySlug.get(slug)}
              selected={slug === selectedSlug}
              onSelectTrigger={onSelectTrigger}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

function TriggerExplorerLeaf({
  slug,
  source,
  selected,
  onSelectTrigger,
}: {
  slug: string;
  source?: string;
  selected: boolean;
  onSelectTrigger: (slug: string) => void;
}) {
  const { name } = triggerSlugLabel(slug);
  const sourceKey = normalizeSource(source);

  return (
    <li className="triggers-explorer-leaf">
      <button
        type="button"
        className={`triggers-explorer-trigger ${selected ? 'active' : ''}`}
        aria-label={`Select trigger ${slug}`}
        onClick={() => onSelectTrigger(slug)}
      >
        <span className="triggers-explorer-trigger-icon" aria-hidden="true">
          <ObjectIcon type="trigger" />
        </span>
        <span className="truncate">{name || slug}</span>
        <span
          className={`triggers-explorer-source triggers-explorer-source--${sourceKey === 'git' ? 'git' : 'database'}`}
          title={sourceLabel(sourceKey)}
          aria-label={sourceLabel(sourceKey)}
        />
      </button>
    </li>
  );
}
