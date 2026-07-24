import type { AnalysisAiPromptContext } from '../analysis/ai.js';
import { compactPromptList, formatPromptTimestamp, redactAnalysisPromptText } from '../analysis/promptContext.js';
import type { YamlValidationError } from '../editor/YamlValidationPanel.js';
import type { PipelineRun, PipelineTrigger } from './api.js';
import type { PipelineDetail, PipelineGraphData, PipelineGraphStepDetail, PipelineGraphTaskDefinition } from './model.js';

const MAX_YAML_LINES = 60;
const MAX_GRAPH_LINES = 50;
const MAX_TRIGGER_LINES = 30;
const MAX_RUN_LINES = 30;
const MAX_LINE_LENGTH = 700;

export type PipelineAnalysisPromptContextInput = {
  detail: PipelineDetail;
  graphData: PipelineGraphData;
  triggers: PipelineTrigger[];
  recentRuns: PipelineRun[];
  includeRunHistory: boolean;
  validationErrors?: YamlValidationError[];
  triggersLoading?: boolean;
  triggersError?: string | null;
  runsLoading?: boolean;
  runsError?: string | null;
};

export function buildPipelineAnalysisPromptContext(input: PipelineAnalysisPromptContextInput): AnalysisAiPromptContext {
  const sections: AnalysisAiPromptContext['sections'] = [
    buildPipelineSnapshotSection(input),
    buildPipelineYamlSection(input.detail.rawYaml),
    buildPipelineGraphSection(input.graphData),
    buildPipelineTriggerSection(input),
    buildPipelineRunHistorySection(input),
  ];
  const validationSection = buildPipelineValidationSection(input.validationErrors || []);
  if (validationSection) sections.splice(1, 0, validationSection);
  return { sections };
}

function buildPipelineSnapshotSection(input: PipelineAnalysisPromptContextInput): AnalysisAiPromptContext['sections'][number] {
  const graphStepCount = input.graphData.steps.length;
  const taskCount = input.graphData.steps.reduce((count, step) => count + (step.configuration?.tasks?.length || step.tasks?.length || 0), 0);
  return {
    title: 'Pipeline page snapshot',
    summary: 'Visible pipeline metadata loaded by the pipeline detail page. Use this with YAML, graph, trigger, and recent-run sections before recommending changes.',
    items: [
      { label: 'Pipeline id', value: input.detail.id, kind: 'fact' },
      { label: 'Name', value: input.detail.name || '-', kind: 'fact' },
      { label: 'Path', value: input.detail.path || 'Global', kind: 'fact' },
      { label: 'Version', value: input.detail.version || 'latest', kind: 'fact' },
      { label: 'Source', value: input.detail.source || 'database', kind: 'fact' },
      { label: 'Description', value: redactAnalysisPromptText(input.detail.description || '-'), kind: 'redacted' },
      { label: 'Container image', value: input.detail.containerImage || '-', kind: 'fact' },
      { label: 'Variables', value: compactPromptList(input.detail.variables || []), kind: 'fact' },
      { label: 'Included dependencies', value: compactPromptList(input.detail.includedDependencies || []), kind: 'fact' },
      { label: 'Parsed graph steps', value: String(graphStepCount), kind: 'metric' },
      { label: 'Parsed configured tasks', value: String(taskCount), kind: 'metric' },
      { label: 'Validation errors', value: String(input.validationErrors?.length || 0), kind: 'metric' },
      { label: 'Triggers visible', value: String(input.triggers.length), kind: 'metric' },
      { label: 'Recent runs visible', value: input.includeRunHistory ? String(input.recentRuns.length) : 'Excluded by reviewer option', kind: 'metric' },
    ],
  };
}

function buildPipelineValidationSection(errors: YamlValidationError[]): AnalysisAiPromptContext['sections'][number] | null {
  if (errors.length === 0) return null;
  return {
    title: 'Pipeline YAML validation errors',
    summary: 'Current editor validation results for the saved pipeline snapshot.',
    lines: errors.slice(0, 20).map(error => [
      typeof error.line === 'number' ? `line=${error.line}` : '',
      typeof error.column === 'number' ? `column=${error.column}` : '',
      redactAnalysisPromptText(error.message, 400),
    ].filter(Boolean).join(' ')),
    limitations: errors.length > 20 ? [`${errors.length - 20} additional validation error${errors.length - 20 === 1 ? '' : 's'} omitted.`] : [],
  };
}

function buildPipelineYamlSection(rawYaml: string): AnalysisAiPromptContext['sections'][number] {
  const lines = rawYaml.split(/\r?\n/);
  return {
    title: 'Pipeline YAML snapshot',
    summary: `Bounded redacted YAML with line numbers. Full YAML has ${lines.length} line${lines.length === 1 ? '' : 's'}.`,
    lines: selectHeadTailLines(lines, MAX_YAML_LINES).map(({ line, lineNumber }) =>
      `${lineNumber}: ${redactAnalysisPromptText(line, MAX_LINE_LENGTH)}`
    ),
    limitations: lines.length > MAX_YAML_LINES ? ['YAML was bounded with head and tail lines; inspect the full pipeline definition before editing.'] : [],
  };
}

function buildPipelineGraphSection(graphData: PipelineGraphData): AnalysisAiPromptContext['sections'][number] {
  if (graphData.error) {
    return {
      title: 'Parsed pipeline graph',
      summary: 'The pipeline graph could not be parsed.',
      limitations: [redactAnalysisPromptText(graphData.error, 500)],
    };
  }
  if (graphData.steps.length === 0) {
    return {
      title: 'Parsed pipeline graph',
      summary: 'No parsed steps were available in the graph snapshot.',
      limitations: ['Precise dependency and task analysis requires parsed step metadata.'],
    };
  }
  return {
    title: 'Parsed pipeline graph',
    summary: 'Step, dependency, approval, runtime, and task configuration visible on the pipeline detail graph.',
    lines: graphData.steps.slice(0, MAX_GRAPH_LINES).map(formatGraphStepLine),
    limitations: graphData.steps.length > MAX_GRAPH_LINES ? [`${graphData.steps.length - MAX_GRAPH_LINES} additional step${graphData.steps.length - MAX_GRAPH_LINES === 1 ? '' : 's'} omitted.`] : [],
  };
}

function buildPipelineTriggerSection(input: PipelineAnalysisPromptContextInput): AnalysisAiPromptContext['sections'][number] {
  const limitations = [
    input.triggersLoading ? 'Trigger metadata was still loading when AI context was built.' : '',
    input.triggersError ? `Trigger metadata error: ${redactAnalysisPromptText(input.triggersError, 400)}` : '',
    input.triggers.length > MAX_TRIGGER_LINES ? `${input.triggers.length - MAX_TRIGGER_LINES} additional trigger${input.triggers.length - MAX_TRIGGER_LINES === 1 ? '' : 's'} omitted.` : '',
  ].filter(Boolean);
  return {
    title: 'Pipeline trigger bindings',
    summary: input.triggers.length > 0
      ? 'Visible trigger definitions that can start this pipeline.'
      : 'No trigger bindings were visible on the pipeline detail page.',
    lines: input.triggers.slice(0, MAX_TRIGGER_LINES).map(trigger =>
      [
        `repo=${trigger.repoSlug || '-'}`,
        `source=${trigger.source || '-'}`,
        `trigger=${redactAnalysisPromptText(safeJSONString(trigger.trigger), MAX_LINE_LENGTH)}`,
      ].join(' ')
    ),
    limitations,
  };
}

function buildPipelineRunHistorySection(input: PipelineAnalysisPromptContextInput): AnalysisAiPromptContext['sections'][number] {
  if (!input.includeRunHistory) {
    return {
      title: 'Recent run history',
      summary: 'Run history was excluded by the reviewer option.',
      limitations: ['Reliability and regression conclusions should not rely on hidden run history.'],
    };
  }
  const limitations = [
    input.runsLoading ? 'Recent runs were still loading when AI context was built.' : '',
    input.runsError ? `Recent run loading error: ${redactAnalysisPromptText(input.runsError, 400)}` : '',
    input.recentRuns.length > MAX_RUN_LINES ? `${input.recentRuns.length - MAX_RUN_LINES} additional run${input.recentRuns.length - MAX_RUN_LINES === 1 ? '' : 's'} omitted.` : '',
  ].filter(Boolean);
  return {
    title: 'Recent run history',
    summary: input.recentRuns.length > 0
      ? 'Most recent visible runs for this pipeline, newest first.'
      : 'No recent runs were visible for this pipeline.',
    lines: input.recentRuns.slice(0, MAX_RUN_LINES).map(run =>
      [
        `run=${run.run_id || '-'}`,
        `status=${run.status || '-'}`,
        `started=${formatPromptTimestamp(run.started_at) || '-'}`,
        `duration=${run.duration || '-'}`,
        `ref=${run.git_ref || '-'}`,
        `repo=${compactPromptList([run.git_repo_owner, run.git_repo_name], '-')}`,
        `final_output=${formatFinalOutputStatus(run.final_output_status)}`,
      ].join(' ')
    ),
    limitations,
  };
}

function formatGraphStepLine(step: PipelineGraphStepDetail) {
  const configuration = step.configuration || {};
  const configuredTasks = configuration.tasks || [];
  const taskLines = configuredTasks.length > 0
    ? configuredTasks.map(formatTaskDefinition).join('; ')
    : step.tasks?.length
      ? step.tasks.map(task => `${task.task_name}: status=${task.status || '-'} exit=${task.exit_code ?? '-'}`).join('; ')
      : '-';
  const approval = configuration.approval
    ? `approval=${configuration.approval.type || 'approval'} teams=${compactPromptList(configuration.approval.teams || [])} self_approval=${configuration.approval.allow_self_approval === true ? 'true' : 'false'}`
    : '';
  return redactAnalysisPromptText([
    `step=${step.name}`,
    `status=${step.status || '-'}`,
    `depends_on=${compactPromptList(step.depends_on || [])}`,
    configuration.include ? `include=${configuration.include}` : '',
    configuration.image ? `image=${configuration.image}` : '',
    configuration.runtime_pool ? `runtime_pool=${configuration.runtime_pool}` : '',
    configuration.ignore_failure ? 'ignore_failure=true' : '',
    approval,
    configuration.script ? `script=${configuration.script}` : '',
    configuration.goal ? `goal=${configuration.goal}` : '',
    `tasks=${taskLines}`,
  ].filter(Boolean).join(' '), MAX_LINE_LENGTH);
}

function formatTaskDefinition(task: PipelineGraphTaskDefinition) {
  return [
    task.name,
    task.depends_on?.length ? `depends_on=${compactPromptList(task.depends_on)}` : '',
    task.ignore_failure ? 'ignore_failure=true' : '',
    task.script ? `script=${redactAnalysisPromptText(task.script, 180)}` : '',
    task.goal ? `goal=${redactAnalysisPromptText(task.goal, 180)}` : '',
    Object.keys(task.variables || {}).length ? `variables=${compactPromptList(Object.keys(task.variables || {}).sort())}` : '',
  ].filter(Boolean).join(' ');
}

function formatFinalOutputStatus(status?: PipelineRun['final_output_status']) {
  if (!status) return '-';
  return [
    status.status || '-',
    typeof status.generated === 'number' ? `generated=${status.generated}` : '',
    typeof status.failed === 'number' ? `failed=${status.failed}` : '',
    typeof status.total === 'number' ? `total=${status.total}` : '',
  ].filter(Boolean).join('/');
}

function selectHeadTailLines(lines: string[], maxLines: number): Array<{ line: string; lineNumber: number }> {
  if (lines.length <= maxLines) {
    return lines.map((line, index) => ({ line, lineNumber: index + 1 }));
  }
  const markerSlots = 1;
  const headCount = Math.max(1, Math.ceil((maxLines - markerSlots) * 0.65));
  const tailCount = Math.max(1, maxLines - markerSlots - headCount);
  return [
    ...lines.slice(0, headCount).map((line, index) => ({ line, lineNumber: index + 1 })),
    { line: `... ${lines.length - headCount - tailCount} middle line${lines.length - headCount - tailCount === 1 ? '' : 's'} omitted ...`, lineNumber: headCount + 1 },
    ...lines.slice(lines.length - tailCount).map((line, index) => ({ line, lineNumber: lines.length - tailCount + index + 1 })),
  ];
}

function safeJSONString(value: unknown) {
  try {
    return JSON.stringify(value ?? {});
  } catch {
    return String(value ?? '');
  }
}
