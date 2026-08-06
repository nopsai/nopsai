import type { RunListItem, StepDetail, TaskDetail } from './contracts.js';

export type IgnoredFailureWarning = {
  count: number;
  items: string[];
  message: string;
};

type IgnoredFailureWarningInput = {
  steps: StepDetail[];
  childRuns?: RunListItem[];
};

export function buildIgnoredFailureWarning({
  steps,
  childRuns = [],
}: IgnoredFailureWarningInput): IgnoredFailureWarning | null {
  const items: string[] = [];
  let count = 0;

  for (const step of steps || []) {
    const ignoredTasks = (step.tasks || []).filter(task => isIgnoredFailureStatus(task.status));
    for (const task of ignoredTasks) {
      items.push(formatTaskWarningLabel(task));
      count += 1;
    }
    if (ignoredTasks.length === 0 && isIgnoredFailureStatus(step.status)) {
      items.push(`Step ${step.name}`);
      count += 1;
    }
  }

  for (const childRun of childRuns || []) {
    if (!isIgnoredFailureStatus(childRun.status)) {
      continue;
    }
    const parentStep = childRun.parent_step_name ? ` from ${childRun.parent_step_name}` : '';
    items.push(`Included pipeline ${childRun.pipeline_name}${parentStep}`);
    count += 1;
  }

  if (count === 0) {
    return null;
  }

  const message = count === 1
    ? '1 ignored failure was marked ignore_failure, so the run continued. Review it before treating this run as clean.'
    : `${count} ignored failures were marked ignore_failure, so the run continued. Review them before treating this run as clean.`;
  return {
    count,
    items,
    message,
  };
}

export function isIgnoredFailureStatus(status: string | undefined | null): boolean {
  const normalized = (status || '').trim().toLowerCase().replace(/_/g, ' ');
  return normalized.includes('ignored') && normalized.includes('failure');
}

function formatTaskWarningLabel(task: TaskDetail): string {
  return `Task ${task.step_name} / ${task.task_name}`;
}
