import type { GraphStatus, GraphStep, GraphTask } from './contracts.js';

export type RunExecutionListEntityKind = 'step' | 'task';

export type RunExecutionListRow<T extends RunExecutionListEntity> = {
  id: string;
  entity: T;
  level: number;
  position: number;
  dependsOn: string[];
  missingDependsOn: string[];
  dependencyLabel: string;
};

export type RunExecutionListGroup<T extends RunExecutionListEntity> = {
  id: string;
  level: number;
  kind: RunExecutionListEntityKind;
  label: string;
  dependencyLabel: string;
  parallel: boolean;
  rows: Array<RunExecutionListRow<T>>;
};

export type RunExecutionLine = {
  id: string;
  stepName: string;
  unitName: string;
  status: GraphStatus;
  duration?: string;
  taskName?: string;
};

export type RunExecutionListEntity = {
  id: string;
  name: string;
  status: GraphStatus;
  duration?: string;
  dependsOn?: string[];
};

export function buildStepExecutionGroups(steps: GraphStep[]): Array<RunExecutionListGroup<GraphStep>> {
  return buildExecutionGroups(steps, 'step');
}

export function buildTaskExecutionGroups(tasks: GraphTask[]): Array<RunExecutionListGroup<GraphTask>> {
  return buildExecutionGroups(tasks, 'task');
}

export function buildRunExecutionLines(stepGroups: Array<RunExecutionListGroup<GraphStep>>): RunExecutionLine[] {
  return stepGroups.flatMap(group => group.rows.flatMap(row => linesForStep(row.entity)));
}

export function dependencyListLabel(ids: string[], fallback = 'No prerequisites'): string {
  const normalized = ids.map(id => id.trim()).filter(Boolean);
  if (!normalized.length) return fallback;
  return `After ${normalized.join(', ')}`;
}

function linesForStep(step: GraphStep): RunExecutionLine[] {
  if (!step.tasks.length) {
    return [{
      id: `step-${step.id}`,
      stepName: step.name,
      unitName: step.name,
      status: step.status,
      duration: step.duration,
    }];
  }

  return buildTaskExecutionGroups(step.tasks).flatMap(group => group.rows.map(row => {
    const task = row.entity;
    return {
      id: `task-${step.id}-${task.id}`,
      stepName: step.name,
      unitName: task.name,
      status: task.status,
      duration: task.duration,
      taskName: task.name,
    } satisfies RunExecutionLine;
  }));
}

function buildExecutionGroups<T extends RunExecutionListEntity>(
  entities: T[],
  kind: RunExecutionListEntityKind
): Array<RunExecutionListGroup<T>> {
  if (!entities.length) return [];
  const entityByID = new Map(entities.map(entity => [entity.id, entity]));
  const rankMemo = new Map<string, number>();
  const visiting = new Set<string>();

  const rankFor = (entity: T): number => {
    const cached = rankMemo.get(entity.id);
    if (cached !== undefined) return cached;
    if (visiting.has(entity.id)) return 0;
    visiting.add(entity.id);
    const knownParentRanks = (entity.dependsOn || [])
      .map(parentID => entityByID.get(parentID))
      .filter((parent): parent is T => Boolean(parent))
      .map(parent => rankFor(parent));
    visiting.delete(entity.id);
    const rank = knownParentRanks.length ? Math.max(...knownParentRanks) + 1 : 0;
    rankMemo.set(entity.id, rank);
    return rank;
  };

  const rows = entities.map((entity, position) => {
    const dependsOn = (entity.dependsOn || []).map(id => id.trim()).filter(Boolean);
    const missingDependsOn = dependsOn.filter(parentID => !entityByID.has(parentID));
    return {
      id: entity.id,
      entity,
      level: rankFor(entity),
      position,
      dependsOn,
      missingDependsOn,
      dependencyLabel: dependencyListLabel(dependsOn),
    } satisfies RunExecutionListRow<T>;
  });

  const groups = new Map<number, Array<RunExecutionListRow<T>>>();
  rows.forEach(row => {
    const levelRows = groups.get(row.level) || [];
    levelRows.push(row);
    groups.set(row.level, levelRows);
  });

  return Array.from(groups.entries())
    .sort(([left], [right]) => left - right)
    .map(([level, levelRows]) => {
      const sortedRows = [...levelRows].sort((left, right) => left.position - right.position);
      const dependencies = Array.from(new Set(sortedRows.flatMap(row => row.dependsOn))).filter(Boolean);
      const parallel = sortedRows.length > 1;
      return {
        id: `${kind}-level-${level}`,
        level,
        kind,
        label: groupLabel(kind, level, parallel),
        dependencyLabel: dependencyListLabel(dependencies, level === 0 ? 'Starts first' : 'No explicit prerequisites'),
        parallel,
        rows: sortedRows,
      } satisfies RunExecutionListGroup<T>;
    });
}

function groupLabel(kind: RunExecutionListEntityKind, level: number, parallel: boolean): string {
  const entity = kind === 'step' ? 'step' : 'task';
  if (!parallel) return `${capitalize(entity)} ${level + 1}`;
  return `Parallel ${entity} group ${level + 1}`;
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
