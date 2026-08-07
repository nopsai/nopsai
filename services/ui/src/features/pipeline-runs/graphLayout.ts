import type {
  GraphLayout,
  GraphLayoutEdge,
  GraphLayoutNode,
  GraphPoint,
  GraphSize,
  GraphStatus,
} from './contracts.js';

const DEFAULT_PADDING = 32;
const KNOWN_RUN_STATUSES = new Set([
  'success',
  'warning',
  'failure',
  'failure (ignored)',
  'rejected',
  'timed_out',
  'cancelled',
  'waiting_approval',
  'running',
  'pending',
  'skipped',
]);

export type TaskStatusInput = {
  status: string;
  exit_code?: number | null;
  started_at?: string;
  finished_at?: string;
};

export function getGraphStatusColor(status: GraphStatus): string {
  if (status === 'success') return '#10b981';
  if (status === 'warning') return '#f59e0b';
  if (status === 'failed') return '#ef4444';
  if (status === 'cancelled') return '#f97316';
  if (status === 'running') return '#3b82f6';
  return '#94a3b8';
}

export function getGraphStatusLabel(status: GraphStatus): string {
  if (status === 'success') return 'Success';
  if (status === 'warning') return 'Warning';
  if (status === 'failed') return 'Failed';
  if (status === 'cancelled') return 'Cancelled';
  if (status === 'running') return 'Running';
  if (status === 'pending') return 'Pending';
  if (status === 'skipped') return 'Skipped';
  return status;
}

export function normalizeGraphStatus(status: string | undefined, complete?: boolean): GraphStatus {
  const raw = (status || '').toLowerCase();
  const normalized = KNOWN_RUN_STATUSES.has(raw) ? raw : !complete ? raw || 'pending' : 'pending';
  if (normalized === 'success') return 'success';
  if (normalized === 'warning' || normalized === 'failure (ignored)') return 'warning';
  if (normalized === 'cancelled') return 'cancelled';
  if (normalized === 'running' || normalized === 'waiting_approval') return 'running';
  if (normalized === 'skipped') return 'skipped';
  if (normalized === 'pending') return 'pending';
  return 'failed';
}

export function deriveTaskGraphStatus(task: TaskStatusInput, stepStatus?: string): GraphStatus {
  const base = normalizeGraphStatus(task.status, task.status === 'success');
  const stepBase = stepStatus ? normalizeGraphStatus(stepStatus, stepStatus === 'success') : null;
  const started = Boolean(task.started_at);
  const finished = Boolean(task.finished_at);
  const hasExitCode = typeof task.exit_code === 'number';

  if (base === 'skipped' || base === 'warning' || base === 'failed' || base === 'cancelled') return base;
  if (finished && hasExitCode) return task.exit_code === 0 ? 'success' : 'failed';
  if (!finished && started && stepBase && stepBase !== 'pending' && stepBase !== 'running') return stepBase;
  if (base === 'running' || (started && !finished)) return 'running';
  if (!started && !finished && base === 'success') return stepBase && stepBase !== 'pending' ? stepBase : 'pending';
  if (base === 'pending' && !started && !finished) {
    return stepBase && stepBase !== 'pending' && stepBase !== 'running' ? 'skipped' : (stepBase || 'pending');
  }
  return base;
}

export function deriveGraphEdgeStatus(source: GraphStatus, target: GraphStatus): GraphStatus {
  if (source === 'failed' || target === 'failed') return 'failed';
  if (source === 'cancelled' || target === 'cancelled') return 'cancelled';
  if (source === 'running' || target === 'running') return 'running';
  if (source === 'pending' || target === 'pending') return 'pending';
  if (source === 'warning' || target === 'warning') return 'warning';
  if (source === 'skipped' || target === 'skipped') return 'skipped';
  return 'success';
}

export function calculateGraphLayout<T extends { id: string; dependsOn?: string[]; status: GraphStatus }>(
  items: T[],
  getSize: (item: T) => GraphSize,
  horizontalGap: number,
  verticalGap: number,
  orientation: 'horizontal' | 'vertical' = 'horizontal',
  padding = DEFAULT_PADDING
): GraphLayout<T> {
  if (!items.length) {
    return { nodes: [], edges: [], width: padding * 2, height: padding * 2 };
  }

  const ranks = getRanks(items);
  const levels: T[][] = [];
  items.forEach(item => {
    const rank = ranks[item.id] || 0;
    if (!levels[rank]) levels[rank] = [];
    levels[rank].push(item);
  });
  levels.forEach(levelItems => levelItems?.sort((a, b) => a.id.localeCompare(b.id)));

  const nodes: GraphLayoutNode<T>[] = [];
  const edges: GraphLayoutEdge[] = [];
  let totalWidth = padding * 2;
  let totalHeight = padding * 2;

  if (orientation === 'horizontal') {
    let currentX = padding;
    const levelXs: number[] = [];
    const levelMaxWidths: number[] = [];
    levels.forEach((levelItems, levelIndex) => {
      levelXs[levelIndex] = currentX;
      const maxWidth = Math.max(...levelItems.map(item => getSize(item).width), 0);
      levelMaxWidths[levelIndex] = maxWidth;
      currentX += maxWidth + horizontalGap;
    });
    totalWidth = Math.max(padding * 2, currentX - horizontalGap + padding);
    const levelHeights = levels.map(levelItems =>
      levelItems.reduce((height, item) => height + getSize(item).height + verticalGap, 0) - verticalGap
    );
    const maxLevelHeight = Math.max(...levelHeights, 0);
    totalHeight = Math.max(padding * 2, maxLevelHeight + padding * 2);

    levels.forEach((levelItems, levelIndex) => {
      const x = levelXs[levelIndex];
      const maxWidth = levelMaxWidths[levelIndex] || 0;
      let currentY = padding + (maxLevelHeight - levelHeights[levelIndex]) / 2;
      levelItems.forEach(item => {
        const size = getSize(item);
        nodes.push({
          data: item,
          level: levelIndex,
          x: x + (maxWidth - size.width) / 2,
          y: currentY,
          width: size.width,
          height: size.height,
        });
        currentY += size.height + verticalGap;
      });
    });
  } else {
    let currentY = padding;
    const levelYs: number[] = [];
    levels.forEach((levelItems, levelIndex) => {
      levelYs[levelIndex] = currentY;
      currentY += Math.max(...levelItems.map(item => getSize(item).height), 0) + verticalGap;
    });
    totalHeight = Math.max(padding * 2, currentY - verticalGap + padding);
    const levelWidths = levels.map(levelItems =>
      levelItems.reduce((width, item) => width + getSize(item).width + horizontalGap, 0) - horizontalGap
    );
    const maxLevelWidth = Math.max(...levelWidths, 0);
    totalWidth = Math.max(padding * 2, maxLevelWidth + padding * 2);

    levels.forEach((levelItems, levelIndex) => {
      const y = levelYs[levelIndex];
      let currentX = padding + (maxLevelWidth - levelWidths[levelIndex]) / 2;
      levelItems.forEach(item => {
        const size = getSize(item);
        nodes.push({
          data: item,
          level: levelIndex,
          x: currentX,
          y,
          width: size.width,
          height: size.height,
        });
        currentX += size.width + horizontalGap;
      });
    });
  }

  items.forEach(item => {
    const targetNode = nodes.find(node => node.data.id === item.id);
    if (!targetNode) return;
    item.dependsOn?.forEach(parentId => {
      const sourceNode = nodes.find(node => node.data.id === parentId);
      if (!sourceNode) return;
      const start = orientation === 'horizontal'
        ? { x: sourceNode.x + sourceNode.width - 35, y: sourceNode.y + sourceNode.height / 2 }
        : { x: sourceNode.x + sourceNode.width / 2, y: sourceNode.y + sourceNode.height - 2 };
      const end = orientation === 'horizontal'
        ? { x: targetNode.x - 2, y: targetNode.y + targetNode.height / 2 }
        : { x: targetNode.x + targetNode.width / 2, y: targetNode.y + 2 };
      const controlDistance = orientation === 'horizontal'
        ? Math.max(20, (end.x - start.x) * 0.38)
        : Math.max(18, (end.y - start.y) * 0.45);
      const points = orientation === 'horizontal'
        ? [
            start,
            { x: start.x + controlDistance, y: start.y },
            { x: end.x - controlDistance, y: end.y },
            end,
          ]
        : [
            start,
            { x: start.x, y: start.y + controlDistance },
            { x: end.x, y: end.y - controlDistance },
            end,
          ];
      edges.push({
        id: `${parentId}-${item.id}`,
        from: parentId,
        to: item.id,
        status: deriveGraphEdgeStatus(sourceNode.data.status, targetNode.data.status),
        points,
      });
    });
  });

  return { nodes, edges, width: totalWidth, height: totalHeight };
}

export type GraphRegion = GraphPoint & GraphSize;

export type FittedGraphLayout<T> = GraphLayout<T> & {
  scale: number;
  offset: GraphPoint;
};

export function fitGraphLayoutToRegion<T>(
  layout: GraphLayout<T>,
  region: GraphRegion,
  maxScale = 1
): FittedGraphLayout<T> {
  if (!layout.nodes.length) {
    return {
      nodes: [],
      edges: [],
      width: region.width,
      height: region.height,
      scale: 1,
      offset: { x: region.x, y: region.y },
    };
  }

  const bounds = getGraphLayoutBounds(layout);
  const scale = Math.min(
    maxScale,
    Math.max(0.1, Math.min(region.width / bounds.width, region.height / bounds.height))
  );
  const scaledWidth = bounds.width * scale;
  const scaledHeight = bounds.height * scale;
  const offset = {
    x: region.x + (region.width - scaledWidth) / 2 - bounds.x * scale,
    y: region.y + (region.height - scaledHeight) / 2 - bounds.y * scale,
  };

  return {
    nodes: layout.nodes.map(node => ({
      ...node,
      x: node.x * scale + offset.x,
      y: node.y * scale + offset.y,
      width: node.width * scale,
      height: node.height * scale,
    })),
    edges: layout.edges.map(edge => ({
      ...edge,
      points: edge.points.map(point => ({
        x: point.x * scale + offset.x,
        y: point.y * scale + offset.y,
      })),
    })),
    width: region.width,
    height: region.height,
    scale,
    offset,
  };
}

export function getGraphLayoutBounds<T>(layout: GraphLayout<T>): GraphRegion {
  if (!layout.nodes.length) {
    return { x: 0, y: 0, width: Math.max(1, layout.width), height: Math.max(1, layout.height) };
  }
  const edgePoints = layout.edges.flatMap(edge => edge.points);
  const xs = layout.nodes.flatMap(node => [node.x, node.x + node.width]).concat(edgePoints.map(point => point.x));
  const ys = layout.nodes.flatMap(node => [node.y, node.y + node.height]).concat(edgePoints.map(point => point.y));
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  return {
    x: minX,
    y: minY,
    width: Math.max(1, maxX - minX),
    height: Math.max(1, maxY - minY),
  };
}

function getRanks(items: Array<{ id: string; dependsOn?: string[] }>): Record<string, number> {
  const ranks: Record<string, number> = {};
  const visiting = new Set<string>();
  items.forEach(item => {
    if (!item.dependsOn?.length) ranks[item.id] = 0;
  });

  const getRank = (id: string): number => {
    if (ranks[id] !== undefined) return ranks[id];
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const item = items.find(candidate => candidate.id === id);
    if (!item?.dependsOn?.length) {
      ranks[id] = 0;
      visiting.delete(id);
      return 0;
    }
    const rank = Math.max(-1, ...item.dependsOn.map(getRank)) + 1;
    ranks[id] = rank;
    visiting.delete(id);
    return rank;
  };

  items.forEach(item => getRank(item.id));
  return ranks;
}
