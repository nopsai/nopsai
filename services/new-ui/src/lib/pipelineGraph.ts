export type GraphItem = {
  name: string;
  depends_on?: string[];
};

export type GraphNode = {
  id: string;
  name: string;
  depends_on: string[];
  level: number;
  x: number;
  y: number;
  width: number;
  height: number;
};

export type GraphEdge = { from: GraphNode; to: GraphNode };

export type GraphLayout = {
  nodes: GraphNode[];
  edges: GraphEdge[];
  width: number;
  height: number;
};

type NodeInternal = GraphNode & { parents: Set<string>; children: Set<string> };

export function calculateGraphLayout(
  items: GraphItem[],
  opts?: {
    nodeWidth?: number;
    nodeHeight?: number;
    horizontalGap?: number;
    verticalGap?: number;
    paddingX?: number;
    paddingY?: number;
    vertical?: boolean;
  }
): GraphLayout {
  if (!items.length) return { nodes: [], edges: [], width: 0, height: 0 };

  const nodeWidth = opts?.nodeWidth ?? 120;
  const nodeHeight = opts?.nodeHeight ?? 100;
  const hGap = opts?.horizontalGap ?? 90;
  const vGap = opts?.verticalGap ?? 16;
  const paddingX = opts?.paddingX ?? 80;
  const paddingY = opts?.paddingY ?? 80;
  const vertical = Boolean(opts?.vertical);

  const nodes: Record<string, NodeInternal> = {};
  const adjacency: Record<string, string[]> = {};

  items.forEach(item => {
    const id = item.name;
    if (!id) return;
    nodes[id] = {
      id,
      name: item.name,
      depends_on: Array.isArray(item.depends_on) ? item.depends_on.filter(Boolean) : [],
      level: -1,
      x: 0,
      y: 0,
      width: nodeWidth,
      height: nodeHeight,
      parents: new Set(),
      children: new Set(),
    };
    adjacency[id] = [];
  });

  Object.values(nodes).forEach(node => {
    node.depends_on.forEach(dep => {
      const depNode = nodes[dep];
      if (!depNode) return;
      adjacency[dep].push(node.id);
      node.parents.add(dep);
      depNode.children.add(node.id);
    });
  });

  let level = 0;
  let processedCount = 0;
  const queue: NodeInternal[] = Object.values(nodes).filter(node => node.parents.size === 0);

  const mutableParents = new Map<string, Set<string>>();
  Object.values(nodes).forEach(node => mutableParents.set(node.id, new Set(node.parents)));

  while (queue.length) {
    const levelSize = queue.length;
    for (let i = 0; i < levelSize; i += 1) {
      const node = queue.shift();
      if (!node) continue;
      node.level = level;
      processedCount += 1;

      (adjacency[node.id] || []).forEach(childId => {
        const set = mutableParents.get(childId);
        if (!set) return;
        set.delete(node.id);
        if (set.size === 0) {
          queue.push(nodes[childId]);
        }
      });
    }
    level += 1;
  }

  const nodeCount = Object.keys(nodes).length;
  if (processedCount < nodeCount) {
    Object.values(nodes)
      .filter(n => n.level === -1)
      .forEach(n => {
        n.level = level;
      });
    level += 1;
  }

  const levels: NodeInternal[][] = [];
  Object.values(nodes).forEach(node => {
    if (!levels[node.level]) levels[node.level] = [];
    levels[node.level].push(node);
  });

  let maxNodesInLevel = 0;
  levels.forEach(nodesAtLevel => {
    if (!nodesAtLevel) return;
    maxNodesInLevel = Math.max(maxNodesInLevel, nodesAtLevel.length);
  });

  let totalWidth = 0;
  let totalHeight = 0;

  if (vertical) {
    totalWidth = maxNodesInLevel * nodeWidth + (maxNodesInLevel > 1 ? (maxNodesInLevel - 1) * hGap : 0);
    totalHeight = levels.length * nodeHeight + (levels.length > 1 ? (levels.length - 1) * vGap : 0);

    levels.forEach((nodesAtLevel, i) => {
      if (!nodesAtLevel) return;
      const levelWidth = nodesAtLevel.length * nodeWidth + (nodesAtLevel.length > 1 ? (nodesAtLevel.length - 1) * hGap : 0);
      const xOffset = (totalWidth - levelWidth) / 2;
      nodesAtLevel.forEach((node, j) => {
        node.x = j * (nodeWidth + hGap) + xOffset + paddingX / 2;
        node.y = i * (nodeHeight + vGap) + paddingY / 2;
      });
    });
  } else {
    totalWidth = levels.length * nodeWidth + (levels.length > 1 ? (levels.length - 1) * hGap : 0);
    totalHeight = maxNodesInLevel * nodeHeight + (maxNodesInLevel > 1 ? (maxNodesInLevel - 1) * vGap : 0);

    levels.forEach((nodesAtLevel, i) => {
      if (!nodesAtLevel) return;
      const levelHeight = nodesAtLevel.length * nodeHeight + (nodesAtLevel.length > 1 ? (nodesAtLevel.length - 1) * vGap : 0);
      const yOffset = (totalHeight - levelHeight) / 2;
      nodesAtLevel.forEach((node, j) => {
        node.x = i * (nodeWidth + hGap) + paddingX / 2;
        node.y = j * (nodeHeight + vGap) + yOffset + paddingY / 2;
      });
    });
  }

  const edges: GraphEdge[] = [];
  Object.values(nodes).forEach(node => {
    node.depends_on.forEach(dep => {
      const fromNode = nodes[dep];
      if (!fromNode) return;
      edges.push({ from: fromNode, to: node });
    });
  });

  return {
    nodes: Object.values(nodes),
    edges,
    width: totalWidth + paddingX,
    height: totalHeight + paddingY,
  };
}

