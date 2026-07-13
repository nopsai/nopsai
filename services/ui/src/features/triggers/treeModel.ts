import type { TriggerListItem } from './model.js';

export type TriggerTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TriggerTreeNode[];
  triggerSlugs: string[];
};

export function buildTriggerTree(items: readonly TriggerListItem[]): TriggerTreeNode {
  const root: TriggerTreeNode = { id: '__root__', name: '', fullPath: '', children: [], triggerSlugs: [] };

  items.forEach(item => {
    const parts = item.slug.split('/').filter(Boolean);
    const triggerName = parts.pop();
    if (!triggerName) return;

    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(candidate => candidate.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], triggerSlugs: [] };
        current.children.push(child);
        current.children.sort(sortNodesByName);
      }
      current = child;
    });

    current.triggerSlugs.push(item.slug);
    current.triggerSlugs.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  });

  return root;
}

export function findTriggerTreeNode(root: TriggerTreeNode, ownerPath: string): TriggerTreeNode {
  const segments = ownerPath.split('/').filter(Boolean);
  if (!segments.length) return root;

  let current: TriggerTreeNode | undefined = root;
  for (const segment of segments) {
    current = current.children.find(child => child.name === segment);
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
  return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
}
