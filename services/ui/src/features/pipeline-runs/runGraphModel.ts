import type {
  GraphStatus,
  GraphStep,
  GraphTask,
  PipelineDefinition,
  RunListItem,
  StepConfiguration,
  StepDetail,
  TaskDefinition,
  TaskDetail,
} from './contracts.js';
import { deriveTaskGraphStatus, normalizeGraphStatus } from './graphLayout.js';
import { parseRunTimestamp } from './runPresentation.js';

const MAX_ELAPSED_MS = 1000 * 60 * 60 * 24 * 30;
const EMPTY_TASK_DEFINITIONS: TaskDefinition[] = [];

export type RunGraphStatusFilter = GraphStatus | 'all';

export type RunGraphEntityFilter = {
  searchQuery?: string;
  statusFilter?: RunGraphStatusFilter;
};

export type BuildRunGraphStepsInput = {
  steps: StepDetail[];
  pipelineDefinition?: PipelineDefinition;
  childRuns?: RunListItem[];
};

function humanizeDurationMs(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1000));
  const seconds = totalSeconds % 60;
  const minutes = Math.floor(totalSeconds / 60) % 60;
  const hours = Math.floor(totalSeconds / 3600);
  if (hours) return `${hours}h ${minutes}m`;
  if (minutes) return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
  return `${seconds}s`;
}

export function formatElapsedLabel(
  start?: string | null,
  end?: string | null,
  fallback = '0s',
  openEnded = true
): string {
  const startTimestamp = parseRunTimestamp(start);
  if (startTimestamp === null) return fallback;
  const parsedEnd = parseRunTimestamp(end);
  if (parsedEnd === null && !openEnded) return fallback;
  const endTimestamp = parsedEnd ?? Date.now();
  const duration = endTimestamp - startTimestamp;
  if (duration <= 0 || duration > MAX_ELAPSED_MS) return fallback;
  return humanizeDurationMs(duration);
}

export function formatStepDuration(step: StepDetail): string {
  const provided = (step.duration || '').trim();
  if (provided && /[a-zA-Z]/.test(provided) && isTerminalStatus(step.status)) return provided;
  const taskRange = calculateStepDurationFromTasks(step.tasks);
  if (taskRange) return taskRange;
  if (provided && /[a-zA-Z]/.test(provided)) return provided;
  return formatElapsedLabel(step.started_at, step.finished_at, '') || '0s';
}

export function formatTaskDuration(task: TaskDetail, graphStatus?: string): string {
  const openEnded = graphStatus === 'running';
  return formatElapsedLabel(task.started_at, task.finished_at, '0s', openEnded);
}

export function buildRunGraphSteps({
  steps,
  pipelineDefinition,
  childRuns = [],
}: BuildRunGraphStepsInput): GraphStep[] {
  const stepDefMap = new Map<string, NonNullable<PipelineDefinition['steps']>[number]>();
  (pipelineDefinition?.steps || []).forEach(step => stepDefMap.set(step.name, step));
  const childRunMap = new Map<string, RunListItem>();
  childRuns.forEach(run => {
    if (run.parent_step_name) childRunMap.set(run.parent_step_name, run);
  });

  return steps.map(step => {
    const stepDef = stepDefMap.get(step.name);
    const taskDefinitions = mergeTaskDefinitions(stepDef?.tasks, step.configuration?.tasks);
    const includeLabel = step.configuration?.include
      ? `Included ${step.configuration.include.toLowerCase().includes('pipeline') ? 'Pipeline' : 'Step'}`
      : '';
    const status = normalizeGraphStatus(step.status, step.status === 'success');
    const tasks: GraphTask[] = (step.tasks || [])
      .filter(task => isDisplayableGraphTask(task, step.name, taskDefinitions, step.tasks.length))
      .map(task => {
        const taskName = task.task_name.trim();
        const def = taskDefinitions.find(candidate => candidate.name === taskName);
        const taskStatus = deriveTaskGraphStatus(task, step.status);
        return {
          id: taskName,
          name: taskName,
          status: taskStatus,
          duration: formatTaskDuration(task, taskStatus),
          dependsOn: def?.depends_on || [],
        };
      });

    return {
      id: step.name,
      name: step.name,
      status,
      duration: formatStepDuration(step),
      dependsOn: step.depends_on || [],
      tasks,
      includeLabel,
      childRun: childRunMap.get(step.name) || null,
    };
  });
}

export function countRunGraphTasks(steps: GraphStep[]): number {
  return steps.reduce((sum, step) => sum + step.tasks.length, 0);
}

export function summarizeGraphStatuses(items: Array<{ status: GraphStatus }>) {
  return items.reduce(
    (summary, item) => {
      summary[item.status] += 1;
      return summary;
    },
    {
      success: 0,
      warning: 0,
      failed: 0,
      running: 0,
      pending: 0,
      skipped: 0,
      cancelled: 0,
    } satisfies Record<GraphStatus, number>
  );
}

export function matchesRunGraphEntityFilter(
  entity: { name: string; status: GraphStatus },
  filter: RunGraphEntityFilter
): boolean {
  const normalizedQuery = (filter.searchQuery || '').trim().toLowerCase();
  const matchesSearch = !normalizedQuery || entity.name.toLowerCase().includes(normalizedQuery);
  const statusFilter = filter.statusFilter || 'all';
  const matchesStatus = statusFilter === 'all' || entity.status === statusFilter;
  return matchesSearch && matchesStatus;
}

export function isDisplayableGraphTask(
  task: TaskDetail,
  stepName: string,
  stepTaskDefs: NonNullable<StepConfiguration['tasks']>,
  rawTaskCount: number
) {
  const taskName = task.task_name?.trim();
  if (!taskName) return false;
  const hasMatchingDefinition = stepTaskDefs.some(def => def.name === taskName);
  return !(rawTaskCount === 1 && taskName === stepName && !hasMatchingDefinition);
}

function isTerminalStatus(status?: string): boolean {
  const normalized = (status || '').toLowerCase().trim();
  return normalized === 'success'
    || normalized === 'warning'
    || normalized === 'failure'
    || normalized === 'failure (ignored)'
    || normalized === 'failed'
    || normalized === 'cancelled'
    || normalized === 'skipped'
    || normalized === 'rejected'
    || normalized === 'timed_out';
}

function calculateStepDurationFromTasks(tasks: TaskDetail[]): string | null {
  if (!tasks.length) return null;
  let earliestStart: number | null = null;
  let latestEnd: number | null = null;

  tasks.forEach(task => {
    const start = parseRunTimestamp(task.started_at);
    if (start === null) return;
    earliestStart = earliestStart === null ? start : Math.min(earliestStart, start);
    const end = parseRunTimestamp(task.finished_at) ?? Date.now();
    latestEnd = latestEnd === null ? end : Math.max(latestEnd, end);
  });

  if (earliestStart === null || latestEnd === null) return null;
  const duration = latestEnd - earliestStart;
  if (duration <= 0 || duration > MAX_ELAPSED_MS) return null;
  return humanizeDurationMs(duration);
}

function mergeTaskDefinitions(
  definitionTasks: TaskDefinition[] | undefined,
  runtimeTasks: StepConfiguration['tasks'] | undefined
): TaskDefinition[] {
  const base = definitionTasks || EMPTY_TASK_DEFINITIONS;
  const runtime = runtimeTasks || EMPTY_TASK_DEFINITIONS;
  if (!base.length) return runtime;
  if (!runtime.length) return base;
  const map = new Map<string, TaskDefinition>();
  base.forEach(task => map.set(task.name, task));
  runtime.forEach(task => map.set(task.name, task));
  return Array.from(map.values());
}
