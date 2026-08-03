import type { TriggerListItem } from './model.js';
import {
  GLOBAL_RESOURCE_TEAM_LABEL,
  GLOBAL_RESOURCE_TEAM_PATH,
  compareResourceTreeNodes,
  isGlobalResourceTeamPath,
} from '../../lib/resourceTeams.js';

export type TriggerTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  kind: 'root' | 'owner' | 'team';
  ownerPath: string;
  teamPath: string;
  children: TriggerTreeNode[];
  triggerSlugs: string[];
};

export function buildTriggerTree(items: readonly TriggerListItem[]): TriggerTreeNode {
  const root: TriggerTreeNode = {
    id: '__root__',
    name: '',
    fullPath: '',
    kind: 'root',
    ownerPath: '',
    teamPath: '',
    children: [],
    triggerSlugs: [],
  };

  const ensureOwnerNode = (rawOwnerPath: string) => {
    const ownerPath = normalizeOwnerPath(rawOwnerPath);
    if (!ownerPath) return root;

    let current = root;
    let pathSoFar = '';
    ownerPath.split('/').filter(Boolean).forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(candidate => candidate.kind === 'owner' && candidate.fullPath === pathSoFar);
      if (!child) {
        child = {
          id: pathSoFar,
          name: segment,
          fullPath: pathSoFar,
          kind: 'owner',
          ownerPath: pathSoFar,
          teamPath: '',
          children: [],
          triggerSlugs: [],
        };
        current.children.push(child);
        current.children.sort(sortNodesByName);
      }
      current = child;
    });
    return current;
  };

  const ensureTeamNode = (ownerNode: TriggerTreeNode, ownerPath: string, rawTeamPath: string) => {
    const teamPath = normalizeTeamPath(rawTeamPath);
    const id = teamNodeID(ownerPath, teamPath);
    let child = ownerNode.children.find(candidate => candidate.kind === 'team' && candidate.teamPath === teamPath);
    if (!child) {
      child = {
        id,
        name: isGlobalResourceTeamPath(teamPath) ? GLOBAL_RESOURCE_TEAM_LABEL : teamPath,
        fullPath: id,
        kind: 'team',
        ownerPath,
        teamPath,
        children: [],
        triggerSlugs: [],
      };
      ownerNode.children.push(child);
      ownerNode.children.sort(sortNodesByName);
    }
    return child;
  };

  items.forEach(item => {
    const ownerPath = triggerOwnerPath(item.slug);
    const ownerNode = ensureOwnerNode(ownerPath);

    // Trigger slugs are repository identifiers (`owner/repo`), while NopsAI
    // ownership and inherited AAA grants come from the manifest `team_path`.
    // Keep both dimensions in the explorer so selecting a team bucket under an
    // owner filters by the owner/team intersection instead of conflating them.
    const teamNode = ensureTeamNode(ownerNode, ownerPath, item.teamPath || GLOBAL_RESOURCE_TEAM_PATH);
    teamNode.triggerSlugs.push(item.slug);
    teamNode.triggerSlugs.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  });

  return root;
}

export function findTriggerTreeNode(root: TriggerTreeNode, rawOwnerPath: string, rawTeamPath = ''): TriggerTreeNode {
  const ownerPath = normalizeOwnerPath(rawOwnerPath);
  if (!ownerPath && !rawTeamPath.trim()) return root;
  const ownerNode = findOwnerTreeNode(root, ownerPath);
  if (!rawTeamPath.trim()) return ownerNode;
  const teamPath = normalizeTeamPath(rawTeamPath);
  return ownerNode.children.find(child => child.kind === 'team' && child.teamPath === teamPath) || ownerNode;
}

function findOwnerTreeNode(root: TriggerTreeNode, ownerPath: string): TriggerTreeNode {
  if (!ownerPath) return root;
  const segments = ownerPath.split('/').filter(Boolean);

  let current: TriggerTreeNode | undefined = root;
  let pathSoFar = '';
  for (const segment of segments) {
    pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
    current = current.children.find(child => child.kind === 'owner' && child.fullPath === pathSoFar);
    if (!current) return root;
  }
  return current;
}

export function countTriggerTreeSlugs(node: TriggerTreeNode): number {
  return node.triggerSlugs.length + node.children.reduce((sum, child) => sum + countTriggerTreeSlugs(child), 0);
}

export function triggerTreeAncestorIDs(rawOwnerPath: string, rawTeamPath = ''): string[] {
  const ownerPath = normalizeOwnerPath(rawOwnerPath);
  const ancestors: string[] = [];
  let current = '';
  ownerPath.split('/').filter(Boolean).forEach(segment => {
    current = current ? `${current}/${segment}` : segment;
    ancestors.push(current);
  });
  if (rawTeamPath.trim()) {
    ancestors.push(teamNodeID(ownerPath, normalizeTeamPath(rawTeamPath)));
  }
  return ancestors;
}

function sortNodesByName(a: TriggerTreeNode, b: TriggerTreeNode) {
  if (a.kind !== b.kind) {
    if (a.kind === 'owner') return -1;
    if (b.kind === 'owner') return 1;
  }
  if (a.kind === 'team' && b.kind === 'team' && (isGlobalResourceTeamPath(a.teamPath) || isGlobalResourceTeamPath(b.teamPath))) {
    if (a.teamPath === b.teamPath) return 0;
    return isGlobalResourceTeamPath(a.teamPath) ? -1 : 1;
  }
  return compareResourceTreeNodes(a, b);
}

function triggerOwnerPath(slug: string) {
  const parts = slug.split('/').filter(Boolean);
  parts.pop();
  return normalizeOwnerPath(parts.join('/'));
}

function teamNodeID(ownerPath: string, teamPath: string) {
  return `${ownerPath || 'root'}::team::${teamPath}`;
}

function normalizeOwnerPath(value?: string) {
  return String(value || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

function normalizeTeamPath(value?: string) {
  const normalized = String(value || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
  if (!normalized || normalized.toLowerCase() === 'root' || isGlobalResourceTeamPath(normalized)) {
    return GLOBAL_RESOURCE_TEAM_PATH;
  }
  return normalized;
}
