import { useMemo, useState } from 'react';
import { ChevronRight, FolderTree } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { GLOBAL_RESOURCE_TEAM_PATH } from '../../lib/resourceTeams';
import { normalizeSource, normalizeTriggerTeamPath, sourceLabel, triggerSlugLabel, type TriggerListItem } from './model';
import { countTriggerTreeSlugs, triggerTreeAncestorIDs, type TriggerTreeNode } from './treeModel';

type TriggerExplorerTreeProps = {
  rootNode: TriggerTreeNode;
  allTriggers: TriggerListItem[];
  activeOwnerPath: string;
  activeTeamPath: string;
  selectedSlug: string | null;
  onOpenOwner: (ownerPath: string) => void;
  onOpenTeam: (ownerPath: string, teamPath: string) => void;
  onSelectTrigger: (slug: string) => void;
};

export function TriggerExplorerTree({
  rootNode,
  allTriggers,
  activeOwnerPath,
  activeTeamPath,
  selectedSlug,
  onOpenOwner,
  onOpenTeam,
  onSelectTrigger,
}: TriggerExplorerTreeProps) {
  const sourceBySlug = useMemo(() => {
    return new Map(allTriggers.map(item => [item.slug, item.source]));
  }, [allTriggers]);
  const itemBySlug = useMemo(() => {
    return new Map(allTriggers.map(item => [item.slug, item]));
  }, [allTriggers]);
  const [nodeOpenOverrides, setNodeOpenOverrides] = useState<Map<string, boolean>>(() => new Map());

  const forcedOpenNodeIDs = useMemo(() => {
    const ids = new Set<string>(['__root__']);
    triggerTreeAncestorIDs(activeOwnerPath, activeTeamPath).forEach(id => ids.add(id));
    if (selectedSlug) {
      const selectedOwnerPath = triggerSlugLabel(selectedSlug).path;
      const ownerPath = selectedOwnerPath === 'root' ? '' : selectedOwnerPath;
      triggerTreeAncestorIDs(ownerPath, normalizeTreeTeamPath(itemBySlug.get(selectedSlug)?.teamPath)).forEach(id => ids.add(id));
    }
    return ids;
  }, [activeOwnerPath, activeTeamPath, itemBySlug, selectedSlug]);

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
        className={`triggers-explorer-root ${!activeOwnerPath && !activeTeamPath && !selectedSlug ? 'active' : ''}`}
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
            activeOwnerPath={activeOwnerPath}
            activeTeamPath={activeTeamPath}
            selectedSlug={selectedSlug}
            onToggleNode={toggleNode}
            onOpenOwner={onOpenOwner}
            onOpenTeam={onOpenTeam}
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
  activeOwnerPath,
  activeTeamPath,
  selectedSlug,
  onToggleNode,
  onOpenOwner,
  onOpenTeam,
  onSelectTrigger,
}: {
  node: TriggerTreeNode;
  openNodeIDs: Set<string>;
  sourceBySlug: Map<string, string | undefined>;
  activeOwnerPath: string;
  activeTeamPath: string;
  selectedSlug: string | null;
  onToggleNode: (id: string) => void;
  onOpenOwner: (ownerPath: string) => void;
  onOpenTeam: (ownerPath: string, teamPath: string) => void;
  onSelectTrigger: (slug: string) => void;
}) {
  const open = openNodeIDs.has(node.id);
  const active = !selectedSlug && (
    node.kind === 'owner'
      ? activeOwnerPath === node.fullPath && !activeTeamPath
      : activeOwnerPath === node.ownerPath && activeTeamPath === node.teamPath
  );
  const total = countTriggerTreeSlugs(node);
  const isTeam = node.kind === 'team';

  return (
    <li className="triggers-explorer-node">
      <div className="triggers-explorer-node-row">
        <button
          type="button"
          className="triggers-explorer-toggle"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${isTeam ? node.name : node.fullPath}`}
          aria-expanded={open}
          onClick={() => onToggleNode(node.id)}
        >
          <ChevronRight className={`h-3.5 w-3.5 ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={`triggers-explorer-owner ${active ? 'active' : ''}`}
          aria-label={isTeam ? `Open team ${node.name} under owner ${node.ownerPath || GLOBAL_RESOURCE_TEAM_PATH}` : `Open owner ${node.fullPath}`}
          onClick={() => {
            if (isTeam) onOpenTeam(node.ownerPath, node.teamPath);
            else onOpenOwner(node.fullPath);
          }}
        >
          <span className="triggers-explorer-folder" aria-hidden="true">
            <ObjectIcon type={isTeam ? 'team' : 'repository-owner'} />
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
              activeOwnerPath={activeOwnerPath}
              activeTeamPath={activeTeamPath}
              selectedSlug={selectedSlug}
              onToggleNode={onToggleNode}
              onOpenOwner={onOpenOwner}
              onOpenTeam={onOpenTeam}
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

function normalizeTreeTeamPath(value?: string) {
  return normalizeTriggerTeamPath(value);
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
