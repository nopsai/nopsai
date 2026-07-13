export type AutomationResourceTreeItem = {
  id: string;
  label: string;
  path: string;
  source?: string;
};

export type AutomationResourceTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: AutomationResourceTreeNode[];
  itemIDs: string[];
};

export function normalizeAutomationTreePath(value?: string | null): string {
  return String(value || '')
    .trim()
    .replace(/\/+/g, '/')
    .replace(/^\/+|\/+$/g, '')
    .replace(/^root$/i, '');
}

export function buildAutomationResourceTree(
  items: readonly AutomationResourceTreeItem[]
): AutomationResourceTreeNode {
  const root: AutomationResourceTreeNode = {
    id: '__root__',
    name: '',
    fullPath: '',
    children: [],
    itemIDs: [],
  };

  items.forEach(item => {
    const path = normalizeAutomationTreePath(item.path);
    const parts = path.split('/').filter(Boolean);
    let current = root;
    let pathSoFar = '';

    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(candidate => candidate.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], itemIDs: [] };
        current.children.push(child);
        current.children.sort(sortAutomationNodesByName);
      }
      current = child;
    });

    current.itemIDs.push(item.id);
    current.itemIDs.sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
  });

  return root;
}

export function findAutomationResourceTreeNode(
  root: AutomationResourceTreeNode,
  path: string
): AutomationResourceTreeNode {
  const segments = normalizeAutomationTreePath(path).split('/').filter(Boolean);
  if (!segments.length) return root;

  let current: AutomationResourceTreeNode | undefined = root;
  for (const segment of segments) {
    current = current.children.find(child => child.name === segment);
    if (!current) return root;
  }
  return current;
}

export function countAutomationResourceTreeItems(node: AutomationResourceTreeNode): number {
  return node.itemIDs.length + node.children.reduce((sum, child) => sum + countAutomationResourceTreeItems(child), 0);
}

export function automationResourceTreeAncestorIDs(path: string): string[] {
  const segments = normalizeAutomationTreePath(path).split('/').filter(Boolean);
  const ancestors: string[] = [];
  let current = '';
  segments.forEach(segment => {
    current = current ? `${current}/${segment}` : segment;
    ancestors.push(current);
  });
  return ancestors;
}

export function automationResourceBelongsToPath(resourcePath: string, activePath: string): boolean {
  const resource = normalizeAutomationTreePath(resourcePath);
  const active = normalizeAutomationTreePath(activePath);
  if (!active) return true;
  if (!resource) return false;
  return resource === active || resource.startsWith(`${active}/`);
}

function sortAutomationNodesByName(
  left: AutomationResourceTreeNode,
  right: AutomationResourceTreeNode
): number {
  return left.name.localeCompare(right.name, undefined, { sensitivity: 'base' });
}
