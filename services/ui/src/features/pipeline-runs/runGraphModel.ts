import type { StepDetail, TaskDetail } from './contracts.js';

const MAX_ELAPSED_MS = 1000 * 60 * 60 * 24 * 30;

function parseTimestamp(value?: string | null): number | null {
  if (!value) return null;
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp) ? null : timestamp;
}

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
  const startTimestamp = parseTimestamp(start);
  if (!startTimestamp) return fallback;
  const parsedEnd = parseTimestamp(end);
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

function isTerminalStatus(status?: string): boolean {
  const normalized = (status || '').toLowerCase().trim();
  return normalized === 'success'
    || normalized === 'failure'
    || normalized === 'failure (ignored)'
    || normalized === 'failed'
    || normalized === 'cancelled'
    || normalized === 'skipped'
    || normalized === 'rejected';
}

function calculateStepDurationFromTasks(tasks: TaskDetail[]): string | null {
  if (!tasks.length) return null;
  let earliestStart: number | null = null;
  let latestEnd: number | null = null;

  tasks.forEach(task => {
    const start = parseTimestamp(task.started_at);
    if (start === null) return;
    earliestStart = earliestStart === null ? start : Math.min(earliestStart, start);
    const end = parseTimestamp(task.finished_at) ?? Date.now();
    latestEnd = latestEnd === null ? end : Math.max(latestEnd, end);
  });

  if (earliestStart === null || latestEnd === null) return null;
  const duration = latestEnd - earliestStart;
  if (duration <= 0 || duration > MAX_ELAPSED_MS) return null;
  return humanizeDurationMs(duration);
}
