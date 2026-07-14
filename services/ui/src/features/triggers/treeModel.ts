import type { TriggerListItem } from './model.js';

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

  items.forEach(item => {
    const parts = item.slug.split('/').filter(Boolean);
    const triggerName = parts.pop();
    if (!triggerName) return;

    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(candidate => candidate.kind === 'owner' && candidate.name === segment);
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

    const teamPath = normalizeTeamPath(item.teamPath);
    const teamNodeID = `${current.fullPath || 'root'}::team::${teamPath}`;
    let teamNode = current.children.find(candidate => candidate.kind === 'team' && candidate.teamPath === teamPath);
    if (!teamNode) {
      teamNode = {
        id: teamNodeID,
        name: teamPath === 'root' ? 'Workspace' : teamPath,
        fullPath: teamNodeID,
        kind: 'team',
        ownerPath: current.fullPath,
        teamPath,
        children: [],
        triggerSlugs: [],
      };
      current.children.push(teamNode);
      current.children.sort(sortNodesByName);
    }

    teamNode.triggerSlugs.push(item.slug);
    teamNode.triggerSlugs.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  });

  return root;
}

export function findTriggerTreeNode(root: TriggerTreeNode, ownerPath: string): TriggerTreeNode {
  const segments = ownerPath.split('/').filter(Boolean);
  if (!segments.length) return root;

  let current: TriggerTreeNode | undefined = root;
  for (const segment of segments) {
    current = current.children.find(child => child.kind === 'owner' && child.name === segment);
    if (!current) return root;
  }
  return current;
}

export function countTriggerTreeSlugs(node: TriggerTreeNode): number {
  return node.triggerSlugs.length + node.children.reduce((sum, child) => sum + countTriggerTreeSlugs(child), 0);
}

export function triggerTreeAncestorIDs(path: string): string[] {
  const segments = path.split('/').filter(Boolean);
  const ancestors: string[] = [];
  let current = '';
  segments.forEach(segment => {
    current = current ? `${current}/${segment}` : segment;
    ancestors.push(current);
  });
  return ancestors;
}

function sortNodesByName(a: TriggerTreeNode, b: TriggerTreeNode) {
  if (a.kind !== b.kind) {
    if (a.kind === 'owner') return -1;
    if (b.kind === 'owner') return 1;
  }
  return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
}

function normalizeTeamPath(value?: string) {
  const normalized = String(value || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
  return normalized && normalized.toLowerCase() !== 'root' ? normalized : 'root';
}
