import type { AnalysisAiPromptContext } from '../analysis/ai.js';
import {
  compactPromptList,
  formatPromptTimestamp,
  redactAnalysisPromptText,
} from '../analysis/promptContext.js';
import { fetchRunLogs } from './api.js';
import type { PipelineDefinition, RunListItem, StepDetail } from './contracts.js';
import {
  enrichRunLogLines,
  normalizeRunLogLevel,
  type EnrichedRunLogLine,
  type RunLogLine,
} from './runLogs.js';

const MAX_LOG_LINES = 100;
const MAX_LOG_LINE_LENGTH = 700;
const LOG_TAIL_LINES = 70;

type RunAnalysisEvidenceDetail = {
  run_info: RunListItem;
  steps: StepDetail[];
  pipeline_definition?: PipelineDefinition;
  pipeline_definition_yaml?: string;
};

type RunLogFetcher = (runID: string, sinceLine: number) => Promise<RunLogLine[]>;

export async function buildRunAnalysisPromptContext(
  detail: RunAnalysisEvidenceDetail,
  logFetcher: RunLogFetcher = fetchRunLogs
): Promise<AnalysisAiPromptContext> {
  const firstFailure = findFirstFailedExecution(detail.steps);
  const sections: AnalysisAiPromptContext['sections'] = [];

  if (firstFailure) {
    sections.push(buildFailedExecutionSection(firstFailure, detail.pipeline_definition));
    const yamlExcerpt = extractPipelineYamlStepExcerpt(detail.pipeline_definition_yaml || '', firstFailure.step.name);
    if (yamlExcerpt.length > 0) {
      sections.push({
        title: 'Pipeline YAML excerpt for failed step',
        summary: 'Bounded redacted YAML around the failed step. Use it to understand what command/config was actually configured.',
        lines: yamlExcerpt,
      });
    }
  } else {
    sections.push({
      title: 'Failed execution point',
      summary: 'No failed step or task was present in the visible run detail.',
      limitations: ['A precise root cause requires task status rows or run logs.'],
    });
  }

  if (detail.run_info.run_id) {
    sections.push(await buildRunLogEvidenceSection(detail.run_info.run_id, firstFailure, logFetcher));
  }

  return { sections };
}

function buildFailedExecutionSection(
  firstFailure: NonNullable<ReturnType<typeof findFirstFailedExecution>>,
  pipelineDefinition?: PipelineDefinition
): AnalysisAiPromptContext['sections'][number] {
  const configuredStep = pipelineDefinition?.steps?.find(step => step.name === firstFailure.step.name);
  const configuredTask = firstFailure.task
    ? firstFailure.step.configuration?.tasks?.find(task => task.name === firstFailure.task?.task_name) ||
      configuredStep?.tasks?.find(task => task.name === firstFailure.task?.task_name)
    : undefined;
  const items = [
    { label: 'Failed step', value: firstFailure.step.name, kind: 'fact' as const },
    { label: 'Step status', value: firstFailure.step.status || 'failure', kind: 'fact' as const },
    ...(firstFailure.task ? [
      { label: 'Failed task', value: firstFailure.task.task_name, kind: 'fact' as const },
      { label: 'Task status', value: firstFailure.task.status || 'failure', kind: 'fact' as const },
      { label: 'Exit code', value: firstFailure.task.exit_code == null ? '-' : String(firstFailure.task.exit_code), kind: 'fact' as const },
    ] : []),
    { label: 'Step dependencies', value: compactPromptList(firstFailure.step.depends_on || configuredStep?.depends_on || []), kind: 'fact' as const },
    { label: 'Step runtime pool', value: firstFailure.step.configuration?.runtime_pool || configuredStep?.runtime_pool || '-', kind: 'fact' as const },
    { label: 'Step image', value: firstFailure.step.configuration?.image || '-', kind: 'fact' as const },
    { label: 'Step secret references', value: compactPromptList(firstFailure.step.configuration?.secrets || []), kind: 'redacted' as const },
    { label: 'Step variable names', value: compactPromptList(Object.keys(firstFailure.step.configuration?.variables || {}).sort()), kind: 'fact' as const },
    { label: 'Step script', value: redactAnalysisPromptText(firstFailure.step.configuration?.script || configuredStep?.script || ''), kind: 'redacted' as const },
    { label: 'Step goal', value: redactAnalysisPromptText(firstFailure.step.configuration?.goal || configuredStep?.goal || ''), kind: 'redacted' as const },
    ...(configuredTask ? [
      { label: 'Task script', value: redactAnalysisPromptText(configuredTask.script || ''), kind: 'redacted' as const },
      { label: 'Task goal', value: redactAnalysisPromptText(configuredTask.goal || ''), kind: 'redacted' as const },
      { label: 'Task dependencies', value: compactPromptList(configuredTask.depends_on || []), kind: 'fact' as const },
      { label: 'Task ignores failure', value: configuredTask.ignore_failure ? 'true' : 'false', kind: 'fact' as const },
    ] : []),
  ].filter(item => item.value !== '');

  return {
    title: 'Failed execution point and configured command context',
    summary: 'Use this before blaming commit, scope, or runner changes. The log excerpt must confirm the exact failure reason.',
    items,
  };
}

async function buildRunLogEvidenceSection(
  runID: string,
  firstFailure: ReturnType<typeof findFirstFailedExecution>,
  logFetcher: RunLogFetcher
): Promise<AnalysisAiPromptContext['sections'][number]> {
  try {
    const rawLines = await logFetcher(runID, 0);
    const enriched = enrichRunLogLines(rawLines);
    const excerpt = selectRunLogExcerpt(enriched, firstFailure);
    if (excerpt.length === 0) {
      return {
        title: 'Failed task log excerpt',
        summary: `Fetched ${rawLines.length} run log line${rawLines.length === 1 ? '' : 's'}, but none matched the failed task or error-signal filters.`,
        limitations: ['Do not infer an exact root cause without the failed command output.'],
      };
    }
    return {
      title: 'Failed task log excerpt',
      summary: `Fetched ${rawLines.length} run log line${rawLines.length === 1 ? '' : 's'} and included ${excerpt.length} redacted line${excerpt.length === 1 ? '' : 's'} around the failed task/error signal.`,
      lines: excerpt.map(formatLogLineForPrompt),
      lineRetention: 'tail',
      limitations: excerpt.length >= MAX_LOG_LINES ? ['Log excerpt was capped; open full logs if the exact failure is not visible here.'] : [],
    };
  } catch (error) {
    return {
      title: 'Failed task log excerpt',
      summary: 'Run logs could not be loaded for AI Evaluation.',
      limitations: [error instanceof Error ? error.message : 'Unknown log loading error'],
    };
  }
}

function selectRunLogExcerpt(
  lines: EnrichedRunLogLine[],
  firstFailure: ReturnType<typeof findFirstFailedExecution>
): EnrichedRunLogLine[] {
  if (lines.length === 0) return [];
  const failedStep = firstFailure?.step.name || '';
  const failedTask = firstFailure?.task?.task_name || '';
  const exactTaskLines = lines.filter(line =>
    matchesStep(line, failedStep) && (!failedTask || matchesTask(line, failedTask))
  );
  const stepLines = exactTaskLines.length > 0 ? exactTaskLines : lines.filter(line => matchesStep(line, failedStep));
  const scoped = stepLines.length > 0 ? stepLines : lines;
  const signalIndexes = scoped
    .map((line, index) => lineHasFailureSignal(line) ? index : -1)
    .filter(index => index >= 0);

  const selectedIndexes = new Set<number>();
  const addRange = (start: number, end: number) => {
    for (let cursor = Math.max(0, start); cursor <= Math.min(scoped.length - 1, end); cursor += 1) {
      selectedIndexes.add(cursor);
    }
  };

  for (const index of signalIndexes.slice(-10)) {
    addRange(index - 4, index + 8);
  }
  addRange(scoped.length - LOG_TAIL_LINES, scoped.length - 1);

  const selected = Array.from(selectedIndexes)
    .sort((left, right) => left - right)
    .map(index => scoped[index]);
  return selected.slice(-MAX_LOG_LINES);
}

function findFirstFailedExecution(steps: StepDetail[]) {
  for (const step of steps) {
    const failedTask = [...(step.tasks || [])]
      .sort((left, right) => left.task_index - right.task_index)
      .find(task => isFailureStatus(task.status));
    if (failedTask) return { step, task: failedTask };
    if (isFailureStatus(step.status)) return { step, task: null };
  }
  return null;
}

function matchesStep(line: EnrichedRunLogLine, stepName: string) {
  return Boolean(stepName) && (line.step || line.step_name || '').trim() === stepName;
}

function matchesTask(line: EnrichedRunLogLine, taskName: string) {
  return Boolean(taskName) && (line.task || line.task_name || '').trim() === taskName;
}

function lineHasFailureSignal(line: EnrichedRunLogLine) {
  const text = `${line.level || ''} ${line.line || ''}`.toLowerCase();
  return normalizeRunLogLevel(line.level) === 'error' ||
    /\b(fail(?:ed|ure)?|error|exception|traceback|panic|exit code|lint|coverage|assert|timeout|denied|not found|no such file|cannot|missing|required|expected|invalid|validation|threshold)\b/.test(text);
}

function formatLogLineForPrompt(line: EnrichedRunLogLine) {
  const timestamp = formatPromptTimestamp(line.timestamp);
  const parts = [
    `#${line.id || '?'}`,
    timestamp,
    line.source ? `source=${line.source}` : '',
    line.stream ? `stream=${line.stream}` : '',
    line.step || line.step_name ? `step=${line.step || line.step_name}` : '',
    line.task || line.task_name ? `task=${line.task || line.task_name}` : '',
    `level=${normalizeRunLogLevel(line.level)}`,
  ].filter(Boolean);
  return `${parts.join(' ')} :: ${redactAnalysisPromptText(line.line || '', MAX_LOG_LINE_LENGTH)}`;
}

function extractPipelineYamlStepExcerpt(yaml: string, stepName: string) {
  const lines = yaml.split(/\r?\n/);
  if (!stepName || lines.length === 0) return [];
  const namePattern = new RegExp(`name:\\s*['"]?${escapeRegExp(stepName)}['"]?\\s*$`);
  const index = lines.findIndex(line => namePattern.test(line.trim()));
  if (index < 0) return [];
  return lines
    .slice(Math.max(0, index - 3), Math.min(lines.length, index + 18))
    .map((line, offset) => `${Math.max(1, index - 2) + offset}: ${redactAnalysisPromptText(line, 500)}`);
}

function isFailureStatus(status?: string) {
  const normalized = String(status || '').trim().toLowerCase().replace(/\s+/g, '_');
  return normalized === 'failure' || normalized === 'failed' || normalized === 'error' || normalized === 'cancelled' || normalized === 'rejected';
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
