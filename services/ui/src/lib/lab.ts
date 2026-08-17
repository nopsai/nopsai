import * as yaml from 'js-yaml';
import {
  RUNTIME_OUTPUT_REFERENCE_PREFIX,
  type RuntimeOutputRef,
  parseRuntimeOutputRefCandidates,
  parseScopedRuntimeRef,
  parseTaskOutputDeclarations,
  validateRuntimeVariableMap,
} from './yamlValidation.js';

export type LabValidationError = {
  message: string;
  line?: number | null;
};

export type LabValidationResult = {
  errors: LabValidationError[];
};

export type LabDirective = {
  key: string;
  hint: string;
};

export const PIPELINE_DIRECTIVES: LabDirective[] = [
  { key: 'name', hint: 'Pipeline display name' },
  { key: 'version', hint: 'Pipeline schema version' },
  { key: 'description', hint: 'Human readable summary' },
  { key: 'container_image', hint: 'Default container image' },
  { key: 'working_directory', hint: 'Default working directory' },
  { key: 'variables', hint: 'Global variables' },
  { key: 'steps', hint: 'List pipeline steps' },
  { key: 'timeout', hint: 'Pipeline timeout' },
  { key: 'llm_enabled', hint: 'Enable or disable LLM for this pipeline' },
  { key: 'agent_role', hint: 'Select AI role/persona' },
  { key: 'model', hint: 'Select LLM profile' },
  { key: 'mcp_profiles', hint: 'Select MCP profiles for goal tasks' },
  { key: 'governance_level', hint: 'AI governance enforcement level' },
  { key: 'runtime_pool', hint: 'Kubernetes runtime pool for steps' },
  { key: 'affinity_enabled', hint: 'Keep Kubernetes step pods on the agent node' },
  { key: 'knowledge_context', hint: 'Knowledge documents for goals' },
  { key: 'output', hint: 'Pipeline final outputs' },
  { key: 'llm_content_preload', hint: 'Share workspace files with LLM goals' },
  { key: 'llm_content_include', hint: 'Only share matching paths with LLM' },
  { key: 'llm_content_ignore', hint: 'Paths excluded from LLM context' },
  { key: 'display_option', hint: 'Run view: list or graph' },
];

export const STEP_DIRECTIVES: LabDirective[] = [
  { key: 'name', hint: 'Step name' },
  { key: 'include', hint: 'Include reusable step' },
  { key: 'sync', hint: 'Run step synchronously' },
  { key: 'approval', hint: 'Human approval checkpoint' },
  { key: 'image', hint: 'Override container image' },
  { key: 'secrets', hint: 'Step secrets' },
  { key: 'volumes', hint: 'Step volumes' },
  { key: 'variables', hint: 'Step variables' },
  { key: 'tasks', hint: 'Nested task list' },
  { key: 'condition', hint: 'Conditional execution' },
  { key: 'goal', hint: 'LLM goal prompt' },
  { key: 'script', hint: 'Shell script body' },
  { key: 'depends_on', hint: 'Upstream steps' },
  { key: 'outputs', hint: 'Runtime task outputs' },
  { key: 'ignore_failure', hint: 'Ignore failures' },
  { key: 'agent_role', hint: 'Select AI role/persona' },
  { key: 'model', hint: 'Select LLM profile' },
  { key: 'mcp_profiles', hint: 'MCP profiles for goal tasks' },
  { key: 'governance_level', hint: 'AI governance enforcement level' },
  { key: 'runtime_pool', hint: 'Kubernetes runtime pool override' },
  { key: 'knowledge_context', hint: 'Knowledge documents for this step' },
];

export const TASK_DIRECTIVES: LabDirective[] = [
  { key: 'name', hint: 'Task name' },
  { key: 'goal', hint: 'Task goal prompt' },
  { key: 'script', hint: 'Task script body' },
  { key: 'depends_on', hint: 'Dependent tasks' },
  { key: 'outputs', hint: 'Runtime task outputs' },
  { key: 'ignore_failure', hint: 'Ignore task errors' },
  { key: 'model', hint: 'Select LLM profile' },
  { key: 'mcp_profiles', hint: 'MCP profiles for this goal task' },
  { key: 'governance_level', hint: 'AI governance enforcement level' },
  { key: 'variables', hint: 'Task variable overrides' },
  { key: 'knowledge_context', hint: 'Knowledge documents for this task' },
];

export const DIRECTIVE_VALUE_METADATA: Record<string, { values: string[]; title: string }> = {
  llm_content_preload: { values: ['true', 'false'], title: 'Boolean value' },
  llm_enabled: { values: ['true', 'false'], title: 'Boolean value' },
  affinity_enabled: { values: ['true', 'false'], title: 'Boolean value' },
  ignore_failure: { values: ['true', 'false'], title: 'Boolean value' },
  sync: { values: ['true', 'false'], title: 'Boolean value' },
  governance_level: { values: ['advisory', 'strict'], title: 'Governance level' },
  display_option: { values: ['list', 'graph'], title: 'Run display' },
};

export const LIST_KEYS_WITH_NAME_TEMPLATE = new Set(['steps', 'tasks']);
export const LIST_KEYS_SIMPLE = new Set(['secrets', 'volumes', 'depends_on', 'artifacts', 'variables', 'mcp_profiles', 'llm_content_include', 'llm_content_ignore']);
export const ARRAY_KEYS = new Set(['steps', 'tasks', 'items', 'variables', 'secrets', 'volumes', 'depends_on', 'outputs', 'artifacts', 'mcp_profiles', 'knowledge_context', 'llm_content_include', 'llm_content_ignore']);

const OVERRIDE_KEY_PATTERN = /^[A-Za-z0-9_.-]+$/;
const GO_DURATION_PATTERN = /^(?=.*[1-9])(?:\d+(?:\.\d+)?|\.\d+)(?:ns|us|\u00b5s|\u03bcs|ms|s|m|h)(?:(?:\d+(?:\.\d+)?|\.\d+)(?:ns|us|\u00b5s|\u03bcs|ms|s|m|h))*$/;
const BLOCKING_KNOWLEDGE_KINDS = new Set(['guardrail', 'policy']);
export const DEFAULT_PIPELINE_NAME = 'ad-hoc-pipeline';

function isPositiveGoDuration(value: string) {
  return GO_DURATION_PATTERN.test(value.trim());
}

function knowledgeContextRefsContainBlocking(value: unknown): boolean {
  if (!Array.isArray(value)) return false;
  return value.some(ref => isPlainObject(ref) && BLOCKING_KNOWLEDGE_KINDS.has(safeString(ref.kind).toLowerCase()));
}

const VALIDATION_EXAMPLES: Array<{ pattern: RegExp; example: string }> = [
  {
    pattern: /Unknown field '.*'/i,
    example: `name: demo-pipeline\nversion: latest\nsteps:\n  - name: build\n    script: echo "hello"`,
  },
  { pattern: /At least one step is required/i, example: `steps:\n  - name: build\n    script: echo "hello"` },
  {
    pattern: /Duplicate step name/i,
    example: `steps:\n  - name: build\n    script: echo "first"\n  - name: build\n    script: echo "second"`,
  },
  {
    pattern: /Duplicate task name/i,
    example: `steps:\n  - name: build\n    tasks:\n      - name: compile\n        script: make\n      - name: compile\n        script: make test`,
  },
  {
    pattern: /has an empty 'goal'/i,
    example: `steps:\n  - name: summarize\n    goal: "Describe the changes for release notes"`,
  },
  { pattern: /has an empty 'script'/i, example: `steps:\n  - name: build\n    script: |\n      npm run build` },
  { pattern: /empty 'include'/i, example: `steps:\n  - name: reuse\n    include: "step:path/to/reusable"` },
  {
    pattern: /must define either 'goal' or 'script'/i,
    example: `steps:\n  - name: lint\n    tasks:\n      - name: run-lint\n        script: |\n          npm run lint`,
  },
];

export function buildValidationExample(message: string): string {
  if (!message) return '';
  for (const entry of VALIDATION_EXAMPLES) {
    if (entry.pattern.test(message)) return entry.example;
  }
  return '';
}

export function buildYamlPathIndex(yamlString: string): Map<string, number> {
  const index = new Map<string, number>();
  if (typeof yamlString !== 'string' || !yamlString.length) {
    return index;
  }

  const lines = yamlString.split('\n');
  const stack: Array<{ indent: number; path: string; type: 'object' | 'array'; nextIndex: number }> = [];

  const pushContext = (indent: number, path: string, type: 'object' | 'array') => {
    stack.push({ indent, path, type, nextIndex: 0 });
  };

  const popToIndent = (indent: number) => {
    while (stack.length && indent < stack[stack.length - 1].indent) {
      stack.pop();
    }
  };

  const setPathIndex = (path: string, lineNumber: number) => {
    if (path) {
      index.set(path, lineNumber);
    }
  };

  lines.forEach((line, idx) => {
    const lineNumber = idx + 1;
    const indent = line.match(/^\s*/)?.[0].length ?? 0;
    const trimmed = line.trim();

    if (!trimmed || trimmed.startsWith('#')) return;

    popToIndent(indent);

    if (trimmed.startsWith('-')) {
      const parent = stack[stack.length - 1];
      if (!parent || parent.type !== 'array') {
        return;
      }
      const itemIndex = parent.nextIndex++;
      const itemPath = `${parent.path}[${itemIndex}]`;
      setPathIndex(itemPath, lineNumber);

      const rest = trimmed.slice(1).trim();
      const keyMatch = rest.match(/^([A-Za-z0-9_]+)\s*:/);
      if (keyMatch) {
        const key = keyMatch[1];
        setPathIndex(`${itemPath}.${key}`, lineNumber);
        if (rest.endsWith(':')) {
          const isArrayKey = ARRAY_KEYS.has(key);
          pushContext(indent + 2, `${itemPath}.${key}`, isArrayKey ? 'array' : 'object');
        }
      } else {
        pushContext(indent + 2, itemPath, 'object');
      }
      return;
    }

    const keyMatch = trimmed.match(/^([A-Za-z0-9_]+)\s*:/);
    if (!keyMatch) return;

    const key = keyMatch[1];
    const parentPath = stack.length ? stack[stack.length - 1].path : '';
    const currentPath = parentPath ? `${parentPath}.${key}` : key;
    setPathIndex(currentPath, lineNumber);

    if (trimmed.endsWith(':')) {
      const isArrayKey = ARRAY_KEYS.has(key);
      pushContext(indent + 2, currentPath, isArrayKey ? 'array' : 'object');
    }
  });

  return index;
}

function safeString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object') return false;
  return !Array.isArray(value);
}

function hasOwn(obj: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(obj, key);
}

export function validateOverrideKey(key: string): boolean {
  return OVERRIDE_KEY_PATTERN.test(key);
}

type RuntimeOutputConsumer = {
  stepName: string;
  taskName: string;
  stepLevel: boolean;
};

type RuntimeVariableConsumer = {
  variables: Record<string, unknown>;
  consumer: RuntimeOutputConsumer;
  path: string;
};

function producerTaskKey(stepName: string, taskName: string): string {
  return `${stepName.trim()}/${taskName.trim()}`;
}

const STEP_VARIABLE_CONSUMER_TASK_NAME = '__step_variables__';

function stepVariableConsumerKey(stepName: string): string {
  return producerTaskKey(stepName, STEP_VARIABLE_CONSUMER_TASK_NAME);
}

function runtimeOutputConsumerKey(consumer: RuntimeOutputConsumer): string {
  return consumer.stepLevel ? stepVariableConsumerKey(consumer.stepName) : producerTaskKey(consumer.stepName, consumer.taskName);
}

function resolvePipelineTaskDependency(
  depNameRaw: string,
  currentStep: string,
  stepNames: Set<string>,
  stepToTaskNames: Map<string, Set<string>>
): { stepName: string; taskName: string; ok: boolean } {
  const depName = depNameRaw.trim();
  const localTasks = stepToTaskNames.get(currentStep);
  if (localTasks?.has(depName)) {
    return { stepName: currentStep, taskName: depName, ok: true };
  }

  let bestStep = '';
  let bestTask = '';
  stepToTaskNames.forEach((taskNames, stepName) => {
    const prefix = `${stepName}.`;
    if (!depName.startsWith(prefix)) return;
    const taskName = depName.slice(prefix.length).trim();
    if (!taskName || !taskNames.has(taskName)) return;
    if (stepName.length > bestStep.length) {
      bestStep = stepName;
      bestTask = taskName;
    }
  });
  if (bestStep) {
    return { stepName: bestStep, taskName: bestTask, ok: true };
  }
  if (stepNames.has(depName)) {
    return { stepName: depName, taskName: '', ok: true };
  }
  return { stepName: '', taskName: '', ok: false };
}

function buildPipelineDependencyIndex(
  steps: unknown[],
  stepNames: Set<string>,
  stepToTaskNames: Map<string, Set<string>>
): Map<string, string[]> {
  const dependencies = new Map<string, string[]>();
  const ensureNode = (key: string) => {
    if (!dependencies.has(key)) dependencies.set(key, []);
  };
  const appendDependency = (consumerKey: string, producerKey: string) => {
    ensureNode(consumerKey);
    ensureNode(producerKey);
    dependencies.get(consumerKey)?.push(producerKey);
  };

  stepToTaskNames.forEach((taskNames, stepName) => {
    ensureNode(stepVariableConsumerKey(stepName));
    [...taskNames].sort().forEach(taskName => ensureNode(producerTaskKey(stepName, taskName)));
  });

  for (const stepRaw of steps) {
    if (!isPlainObject(stepRaw)) continue;
    const stepName = safeString(stepRaw.name);
    if (!stepName) continue;
    const taskNames = stepToTaskNames.get(stepName) ?? new Set<string>();
    const consumerKeys = [stepVariableConsumerKey(stepName), ...[...taskNames].sort().map(taskName => producerTaskKey(stepName, taskName))];

    if (Array.isArray(stepRaw.depends_on)) {
      for (const dep of stepRaw.depends_on) {
        const resolved = resolvePipelineTaskDependency(safeString(dep), stepName, stepNames, stepToTaskNames);
        if (!resolved.ok) continue;
        let producerKeys = resolved.taskName
          ? [producerTaskKey(resolved.stepName, resolved.taskName)]
          : [...(stepToTaskNames.get(resolved.stepName) ?? [])].sort().map(taskName => producerTaskKey(resolved.stepName, taskName));
        if (!resolved.taskName && producerKeys.length === 0) {
          producerKeys = [stepVariableConsumerKey(resolved.stepName)];
        }
        for (const consumerKey of consumerKeys) {
          for (const producerKey of producerKeys) {
            appendDependency(consumerKey, producerKey);
          }
        }
      }
    }

    const tasks = Array.isArray(stepRaw.tasks) ? stepRaw.tasks : [];
    for (const taskRaw of tasks) {
      if (!isPlainObject(taskRaw)) continue;
      const taskName = safeString(taskRaw.name);
      if (!taskName || !Array.isArray(taskRaw.depends_on)) continue;
      const consumerKey = producerTaskKey(stepName, taskName);
      for (const dep of taskRaw.depends_on) {
        const resolved = resolvePipelineTaskDependency(safeString(dep), stepName, stepNames, stepToTaskNames);
        if (!resolved.ok || !resolved.taskName) continue;
        appendDependency(consumerKey, producerTaskKey(resolved.stepName, resolved.taskName));
      }
    }
  }

  return dependencies;
}

function dependencyIndexHasPath(dependencies: Map<string, string[]>, from: string, to: string): boolean {
  if (!from.trim() || !to.trim() || from === to) return false;
  const visited = new Set<string>();
  const visit = (node: string): boolean => {
    if (visited.has(node)) return false;
    visited.add(node);
    return (dependencies.get(node) ?? []).some(dep => dep === to || visit(dep));
  };
  return visit(from);
}

function consumerDependsOnOutputProducer(
  consumer: RuntimeOutputConsumer,
  ref: { stepName: string; taskName: string },
  dependencyIndex: Map<string, string[]>
): boolean {
  if (consumer.stepLevel && consumer.stepName === ref.stepName) return false;
  if (!consumer.stepLevel && consumer.stepName === ref.stepName && consumer.taskName === ref.taskName) return false;
  return dependencyIndexHasPath(dependencyIndex, runtimeOutputConsumerKey(consumer), producerTaskKey(ref.stepName, ref.taskName));
}

function validateRuntimeOutputRefsInVariables(
  variables: Record<string, unknown>,
  consumer: RuntimeOutputConsumer,
  stepToTaskNames: Map<string, Set<string>>,
  outputDeclarations: Map<string, Set<string>>,
  dependencyIndex: Map<string, string[]>
): string | null {
  for (const [name, rawValue] of Object.entries(variables)) {
    if (typeof rawValue !== 'string') continue;
    const parsed = parseRuntimeOutputRefCandidates(rawValue);
    if (parsed.error) {
      return `Validation Error: Variable '${name}' in step '${consumer.stepName}' references an invalid runtime output: ${parsed.error}.`;
    }
    if (!parsed.found) {
      if (rawValue.includes(RUNTIME_OUTPUT_REFERENCE_PREFIX)) {
        return `Validation Error: Variable '${name}' in step '${consumer.stepName}' uses a runtime output in an unsupported expression; use the full value $steps.<step>.outputs.<name> or $steps.<step>.<task>.outputs.<name>.`;
      }
      continue;
    }
    let firstRef: RuntimeOutputRef | null = null;
    let matched = false;
    for (const ref of parsed.refs ?? []) {
      if (!firstRef) firstRef = ref;
      if (!stepToTaskNames.get(ref.stepName)?.has(ref.taskName)) continue;
      if (!outputDeclarations.get(producerTaskKey(ref.stepName, ref.taskName))?.has(ref.outputName)) continue;
      if (!consumerDependsOnOutputProducer(consumer, ref, dependencyIndex)) {
        return `Validation Error: Variable '${name}' consumes output ${ref.stepName}.${ref.taskName}.outputs.${ref.outputName} without a valid dependency.`;
      }
      matched = true;
      break;
    }
    if (matched) continue;

    const ref = firstRef;
    if (!ref) continue;
    if (!stepToTaskNames.get(ref.stepName)?.has(ref.taskName)) {
      return `Validation Error: Variable '${name}' references missing output producer task ${ref.stepName}.${ref.taskName}.`;
    }
    if (!outputDeclarations.get(producerTaskKey(ref.stepName, ref.taskName))?.has(ref.outputName)) {
      return `Validation Error: Variable '${name}' references undeclared output ${ref.stepName}.${ref.taskName}.outputs.${ref.outputName}.`;
    }
  }
  return null;
}

export function validatePipelineYamlStrict(yamlString: string): LabValidationResult {
  const pathIndex = buildYamlPathIndex(yamlString);

  const knownPipelineKeys = new Set([
    'name',
    'version',
    'description',
    'container_image',
    'display_option',
    'working_directory',
    'variables',
    'steps',
    'timeout',
    'llm_enabled',
    'agent_role',
    'model',
    'mcp_profiles',
    'governance_level',
    'runtime_pool',
    'affinity_enabled',
    'knowledge_context',
    'output',
    'llm_content_preload',
    'llm_content_include',
    'llm_content_ignore',
    'access',
  ]);
  const knownStepKeys = new Set([
    'name',
    'include',
    'sync',
    'approval',
    'image',
    'secrets',
    'volumes',
    'variables',
    'tasks',
    'condition',
    'goal',
    'script',
    'depends_on',
    'outputs',
    'ignore_failure',
    'agent_role',
    'model',
    'mcp_profiles',
    'governance_level',
    'runtime_pool',
    'knowledge_context',
  ]);
  const knownTaskKeys = new Set([
    'name',
    'goal',
    'script',
    'depends_on',
    'outputs',
    'ignore_failure',
    'model',
    'mcp_profiles',
    'governance_level',
    'variables',
    'knowledge_context',
  ]);
  const knownOutputKeys = new Set(['model', 'items']);
  const knownOutputItemKeys = new Set(['name', 'type', 'when', 'prompt', 'model', 'dashboard']);
  const knownDashboardTargetKeys = new Set(['ref', 'section', 'entry_key', 'mode', 'preset', 'ttl']);

  const createError = (message: string, pathHints: string[] = []): LabValidationError => {
    let line: number | null = null;
    for (const hint of pathHints) {
      if (!hint) continue;
      if (hint.startsWith('line:')) {
        const direct = Number(hint.slice(5));
        if (!Number.isNaN(direct) && direct > 0) {
          line = direct;
          break;
        }
        continue;
      }
      const candidate = pathIndex.get(hint);
      if (typeof candidate === 'number') {
        line = candidate;
        break;
      }
    }
    return { message, line };
  };

  const findUnknownKeys = (obj: unknown, knownKeys: Set<string>, path = '') => {
    if (!isPlainObject(obj)) return [];
    const unknown: Array<{ path: string; key: string }> = [];
    Object.keys(obj).forEach(key => {
      if (!knownKeys.has(key)) {
        unknown.push({ path: path ? `${path}.${key}` : key, key });
      }
    });
    return unknown;
  };

  const checkAllKeys = (pipeline: Record<string, unknown>) => {
    let allUnknown = findUnknownKeys(pipeline, knownPipelineKeys);
    if (pipeline.output) {
      allUnknown = allUnknown.concat(findUnknownKeys(pipeline.output, knownOutputKeys, 'output'));
      const outputItems = isPlainObject(pipeline.output) && Array.isArray(pipeline.output.items) ? pipeline.output.items : [];
      outputItems.forEach((item: unknown, index: number) => {
        const itemPath = `output.items[${index}]`;
        allUnknown = allUnknown.concat(findUnknownKeys(item, knownOutputItemKeys, itemPath));
        if (isPlainObject(item) && hasOwn(item, 'dashboard')) {
          allUnknown = allUnknown.concat(findUnknownKeys(item.dashboard, knownDashboardTargetKeys, `${itemPath}.dashboard`));
        }
      });
    }

    const steps = Array.isArray(pipeline.steps) ? pipeline.steps : [];
    steps.forEach((step, index) => {
      const stepPath = `steps[${index}]`;
      allUnknown = allUnknown.concat(findUnknownKeys(step, knownStepKeys, stepPath));
      let tasks: unknown[] = [];
      if (isPlainObject(step) && Array.isArray(step.tasks)) {
        tasks = step.tasks;
      }
      tasks.forEach((task: unknown, taskIndex: number) => {
        const taskPath = `${stepPath}.tasks[${taskIndex}]`;
        allUnknown = allUnknown.concat(findUnknownKeys(task, knownTaskKeys, taskPath));
      });
    });
    return allUnknown;
  };

  try {
    const parsed = yaml.load(yamlString) as unknown;
    if (!parsed) return { errors: [createError('YAML is empty or invalid.', [''])] };
    if (!isPlainObject(parsed)) return { errors: [createError('YAML root must be an object.', [''])] };

    const pipeline = parsed;

    const unknownKeys = checkAllKeys(pipeline);
    if (unknownKeys.length > 0) {
      return { errors: unknownKeys.map(item => createError(`Validation Error: Unknown field '${item.key}'.`, [item.path])) };
    }

    // Governance level is validated by value, not just by key, so a removed
    // level is caught in the editor instead of only when the run is submitted.
    const allowedGovernanceLevels = new Set(['advisory', 'strict']);
    const governanceValues: Array<{ value: unknown; path: string }> = [];
    if (hasOwn(pipeline, 'governance_level')) {
      governanceValues.push({ value: pipeline.governance_level, path: 'governance_level' });
    }
    (Array.isArray(pipeline.steps) ? pipeline.steps : []).forEach((step: unknown, index: number) => {
      if (!isPlainObject(step)) return;
      if (hasOwn(step, 'governance_level')) {
        governanceValues.push({ value: step.governance_level, path: `steps[${index}].governance_level` });
      }
      (Array.isArray(step.tasks) ? step.tasks : []).forEach((task: unknown, taskIndex: number) => {
        if (!isPlainObject(task)) return;
        if (hasOwn(task, 'governance_level')) {
          governanceValues.push({ value: task.governance_level, path: `steps[${index}].tasks[${taskIndex}].governance_level` });
        }
      });
    });
    for (const entry of governanceValues) {
      // An absent or empty value falls back to the strict default, matching the backend.
      if (entry.value === null || entry.value === undefined) continue;
      if (typeof entry.value === 'string' && entry.value.trim() === '') continue;
      const normalized = typeof entry.value === 'string' ? entry.value.trim().toLowerCase() : '';
      if (!allowedGovernanceLevels.has(normalized)) {
        return {
          errors: [createError("Validation Error: 'governance_level' must be 'advisory' or 'strict'.", [entry.path])],
        };
      }
    }

    // display_option is validated by value so the editor rejects a removed view
    // (mermaid, tree, flat) instead of silently falling back at render time.
    if (hasOwn(pipeline, 'display_option')) {
      const displayOption = pipeline.display_option;
      // An absent or empty value falls back to the graph default, matching the backend.
      const isEmpty =
        displayOption === null ||
        displayOption === undefined ||
        (typeof displayOption === 'string' && displayOption.trim() === '');
      // Case-sensitive, matching validatePipelineDisplayOption in the backend.
      const normalized = typeof displayOption === 'string' ? displayOption.trim() : '';
      if (!isEmpty && normalized !== 'list' && normalized !== 'graph') {
        return {
          errors: [createError("Validation Error: 'display_option' must be 'list' or 'graph'.", ['display_option'])],
        };
      }
    }

    const llmEnabledValue = hasOwn(pipeline, 'llm_enabled') ? pipeline.llm_enabled : undefined;
    if (llmEnabledValue !== undefined && typeof llmEnabledValue !== 'boolean') {
      return { errors: [createError("Validation Error: 'llm_enabled' must be true or false.", ['llm_enabled'])] };
    }
    const llmEnabled = typeof llmEnabledValue === 'boolean' ? llmEnabledValue : true;

    if (hasOwn(pipeline, 'output')) {
      if (!isPlainObject(pipeline.output)) {
        return { errors: [createError("Validation Error: 'output' must be an object.", ['output'])] };
      }
      const output = pipeline.output as Record<string, unknown>;
      if (hasOwn(output, 'model') && typeof output.model !== 'string') {
        return { errors: [createError("Validation Error: 'output.model' must be a string.", ['output.model', 'output'])] };
      }
      const outputItems = Array.isArray(output.items) ? output.items : [];
      if (!hasOwn(output, 'items') || outputItems.length === 0) {
        return { errors: [createError("Validation Error: 'output.items' must contain at least one final output.", ['output.items', 'output'])] };
      }
      const outputNames = new Set<string>();
      const allowedOutputTypes = new Set(['markdown', 'pdf', 'excel', 'json', 'html', 'dashboard']);
      const allowedOutputWhen = new Set(['always', 'success', 'failure']);
      const allowedDashboardModes = new Set(['', 'replace', 'append', 'snapshot', 'series']);
      const allowedDashboardPresets = new Set(['', 'auto', 'report', 'table', 'status', 'timeline', 'comparison', 'metrics', 'mixed']);
      for (let index = 0; index < outputItems.length; index += 1) {
        const itemPath = `output.items[${index}]`;
        const item = outputItems[index] as unknown;
        if (!isPlainObject(item)) {
          return { errors: [createError('Validation Error: A final output item is not a valid object.', [itemPath])] };
        }
        const outputName = safeString(item.name).trim();
        if (!outputName) {
          return { errors: [createError('Validation Error: Final output item requires a name.', [`${itemPath}.name`, itemPath])] };
        }
        const outputNameKey = outputName.toLowerCase();
        if (outputNames.has(outputNameKey)) {
          return { errors: [createError(`Validation Error: Final output '${outputName}' is defined more than once.`, [`${itemPath}.name`, itemPath])] };
        }
        outputNames.add(outputNameKey);
        const outputType = safeString(item.type).trim().toLowerCase();
        if (!outputType) {
          return { errors: [createError(`Validation Error: Final output '${outputName}' requires a type.`, [`${itemPath}.type`, itemPath])] };
        }
        if (!allowedOutputTypes.has(outputType)) {
          return { errors: [createError(`Validation Error: Final output '${outputName}' has unsupported type '${safeString(item.type)}'.`, [`${itemPath}.type`, itemPath])] };
        }
        const outputWhen = safeString(item.when).trim().toLowerCase();
        if (outputWhen && !allowedOutputWhen.has(outputWhen)) {
          return { errors: [createError(`Validation Error: Final output '${outputName}' has unsupported when '${safeString(item.when)}'.`, [`${itemPath}.when`, itemPath])] };
        }
        if (!safeString(item.prompt).trim()) {
          return { errors: [createError(`Validation Error: Final output '${outputName}' requires a prompt.`, [`${itemPath}.prompt`, itemPath])] };
        }
        if (hasOwn(item, 'model') && typeof item.model !== 'string') {
          return { errors: [createError(`Validation Error: Final output '${outputName}' model must be a string.`, [`${itemPath}.model`, itemPath])] };
        }
        const hasDashboardTarget = hasOwn(item, 'dashboard');
        const dashboardTarget = hasDashboardTarget ? item.dashboard : undefined;
        if (outputType === 'dashboard') {
          if (!isPlainObject(dashboardTarget)) {
            return { errors: [createError(`Validation Error: Final output '${outputName}' dashboard target must be an object.`, [`${itemPath}.dashboard`, itemPath])] };
          }
          const dashboardStringFields = ['ref', 'section', 'entry_key', 'mode', 'preset', 'ttl'];
          for (const key of dashboardStringFields) {
            if (hasOwn(dashboardTarget, key) && typeof dashboardTarget[key] !== 'string') {
              return {
                errors: [
                  createError(`Validation Error: Final output '${outputName}' dashboard.${key} must be a string.`, [
                    `${itemPath}.dashboard.${key}`,
                    `${itemPath}.dashboard`,
                    itemPath,
                  ]),
                ],
              };
            }
          }
          const dashboardRef = safeString(dashboardTarget.ref).replace(/^\/+|\/+$/g, '');
          if (!dashboardRef) {
            return { errors: [createError(`Validation Error: Final output '${outputName}' dashboard.ref is required.`, [`${itemPath}.dashboard.ref`, `${itemPath}.dashboard`, itemPath])] };
          }
          const refSegments = dashboardRef.split('/');
          if (dashboardRef.startsWith('~') || refSegments.length < 2 || refSegments.some(segment => !segment || segment === '.' || segment === '..')) {
            return {
              errors: [
                createError(`Validation Error: Final output '${outputName}' dashboard.ref must use team/dashboard format.`, [
                  `${itemPath}.dashboard.ref`,
                  `${itemPath}.dashboard`,
                  itemPath,
                ]),
              ],
            };
          }
          const dashboardSection = safeString(dashboardTarget.section);
          if (!dashboardSection) {
            return { errors: [createError(`Validation Error: Final output '${outputName}' dashboard.section is required.`, [`${itemPath}.dashboard.section`, `${itemPath}.dashboard`, itemPath])] };
          }
          if (!/^[a-zA-Z0-9_.-]+$/.test(dashboardSection)) {
            return {
              errors: [
                createError(`Validation Error: Final output '${outputName}' dashboard.section can only contain letters, numbers, underscores, dots, and hyphens.`, [
                  `${itemPath}.dashboard.section`,
                  `${itemPath}.dashboard`,
                  itemPath,
                ]),
              ],
            };
          }
          const entryKey = safeString(dashboardTarget.entry_key);
          if (entryKey && !/^[a-zA-Z0-9_.:/-]+$/.test(entryKey)) {
            return {
              errors: [
                createError(`Validation Error: Final output '${outputName}' dashboard.entry_key can only contain letters, numbers, underscores, dots, colons, slashes, and hyphens.`, [
                  `${itemPath}.dashboard.entry_key`,
                  `${itemPath}.dashboard`,
                  itemPath,
                ]),
              ],
            };
          }
          const dashboardMode = safeString(dashboardTarget.mode).toLowerCase();
          if (!allowedDashboardModes.has(dashboardMode)) {
            return {
              errors: [
                createError(`Validation Error: Final output '${outputName}' dashboard.mode '${safeString(dashboardTarget.mode)}' is not supported.`, [
                  `${itemPath}.dashboard.mode`,
                  `${itemPath}.dashboard`,
                  itemPath,
                ]),
              ],
            };
          }
          const dashboardPreset = safeString(dashboardTarget.preset).toLowerCase();
          if (!allowedDashboardPresets.has(dashboardPreset)) {
            return {
              errors: [
                createError(`Validation Error: Final output '${outputName}' dashboard.preset '${safeString(dashboardTarget.preset)}' is not supported.`, [
                  `${itemPath}.dashboard.preset`,
                  `${itemPath}.dashboard`,
                  itemPath,
                ]),
              ],
            };
          }
        } else if (hasDashboardTarget) {
          return {
            errors: [
              createError(`Validation Error: Final output '${outputName}' dashboard configuration requires type 'dashboard'.`, [
                `${itemPath}.dashboard`,
                itemPath,
              ]),
            ],
          };
        }
      }
      if (!llmEnabled) {
        return { errors: [createError('Validation Error: Pipeline has LLM disabled but defines final outputs.', ['output'])] };
      }
    }

    const pipelineName = safeString(pipeline.name);
    if (!pipelineName) return { errors: [createError("Validation Error: 'name' is a required field.", ['name'])] };

    const allowed = /^[a-zA-Z0-9_.-]+$/;
    if (!allowed.test(pipelineName)) {
      return { errors: [createError(`Validation Error: Pipeline name '${pipelineName}' contains invalid characters.`, ['name'])] };
    }

    const pipelineVersion = safeString(pipeline.version);
    if (pipelineVersion && !allowed.test(pipelineVersion)) {
      return { errors: [createError(`Validation Error: Pipeline version '${pipelineVersion}' contains invalid characters.`, ['version'])] };
    }

    if (hasOwn(pipeline, 'variables')) {
      if (!Array.isArray(pipeline.variables)) {
        return { errors: [createError("Validation Error: 'variables' must be a list of runtime variable references.", ['variables'])] };
      }
      const runtimeNames = new Map<string, string>();
      for (let variableIndex = 0; variableIndex < pipeline.variables.length; variableIndex += 1) {
        const rawVariable = pipeline.variables[variableIndex];
        const variablePath = `variables[${variableIndex}]`;
        if (typeof rawVariable !== 'string' || !rawVariable.trim()) {
          return { errors: [createError(`Validation Error: Pipeline variable declaration #${variableIndex + 1} must be a non-empty string.`, [variablePath, 'variables'])] };
        }
        const parsedRef = parseScopedRuntimeRef(rawVariable);
        if (parsedRef.error || !parsedRef.ref) {
          return { errors: [createError(`Validation Error: Pipeline variable '${rawVariable}' is invalid: ${parsedRef.error}.`, [variablePath, 'variables'])] };
        }
        const previous = runtimeNames.get(parsedRef.ref.name);
        if (previous) {
          return {
            errors: [
              createError(`Validation Error: Pipeline variable '${rawVariable}' declares runtime variable '${parsedRef.ref.name}' more than once; previous declaration was '${previous}'.`, [
                variablePath,
                'variables',
              ]),
            ],
          };
        }
        runtimeNames.set(parsedRef.ref.name, rawVariable.trim());
      }
    }

    const steps = Array.isArray(pipeline.steps) ? pipeline.steps : [];
    if (steps.length === 0) {
      return { errors: [createError("Validation Error: At least one step is required in 'steps'.", ['steps'])] };
    }

    const stepNames = new Set<string>();
    const stepToTaskNames = new Map<string, Set<string>>();
    const outputDeclarations = new Map<string, Set<string>>();
    const runtimeVariableConsumers: RuntimeVariableConsumer[] = [];
    const pipelineHasBlockingKnowledge = knowledgeContextRefsContainBlocking(pipeline.knowledge_context);
    for (let index = 0; index < steps.length; index += 1) {
      const step = steps[index] as unknown;
      const stepPath = `steps[${index}]`;
      if (!isPlainObject(step)) {
        return { errors: [createError('Validation Error: A step is not a valid object.', [stepPath])] };
      }

      const stepName = safeString(step.name);
      if (!stepName) {
        return { errors: [createError('Validation Error: All steps require a name.', [`${stepPath}.name`, stepPath])] };
      }
      if (stepNames.has(stepName)) {
        return { errors: [createError(`Validation Error: Duplicate step name '${stepName}' found.`, [`${stepPath}.name`, stepPath])] };
      }
      stepNames.add(stepName);
      stepToTaskNames.set(stepName, new Set<string>());
      const stepHasBlockingKnowledge = pipelineHasBlockingKnowledge || knowledgeContextRefsContainBlocking(step.knowledge_context);

      const stepVariables = step.variables;
      const stepVariablesError = validateRuntimeVariableMap(stepVariables, `Step '${stepName}' variables`);
      if (stepVariablesError) {
        return { errors: [createError(`Validation Error: ${stepVariablesError}`, [`${stepPath}.variables`, stepPath])] };
      }
      if (isPlainObject(stepVariables)) {
        runtimeVariableConsumers.push({
          variables: stepVariables,
          consumer: {
            stepName,
            taskName: stepName,
            stepLevel: true,
          },
          path: `${stepPath}.variables`,
        });
      }

      if (hasOwn(step, 'secrets')) {
        if (!Array.isArray(step.secrets)) {
          return { errors: [createError(`Validation Error: Step '${stepName}' secrets must be a list.`, [`${stepPath}.secrets`, stepPath])] };
        }
        for (let secretIndex = 0; secretIndex < step.secrets.length; secretIndex += 1) {
          const rawSecret = step.secrets[secretIndex];
          if (typeof rawSecret !== 'string' || !rawSecret.trim()) {
            return { errors: [createError(`Validation Error: Step '${stepName}' secret #${secretIndex + 1} must be a non-empty string.`, [`${stepPath}.secrets`, stepPath])] };
          }
          const parsedSecret = parseScopedRuntimeRef(rawSecret);
          if (parsedSecret.error) {
            return { errors: [createError(`Validation Error: Step '${stepName}' secret '${rawSecret}' is invalid: ${parsedSecret.error}.`, [`${stepPath}.secrets`, stepPath])] };
          }
        }
      }

      const hasIncludeKey = hasOwn(step, 'include');
      const includeValue = hasIncludeKey ? step.include : null;
      const includeValid = typeof includeValue === 'string' && includeValue.trim().length > 0;
      if (hasIncludeKey && !includeValid) {
        return { errors: [createError(`Validation Error: Step '${stepName}' has an empty 'include' value.`, [`${stepPath}.include`, stepPath])] };
      }
      const isInclude = includeValid;
      if (isInclude) {
        stepToTaskNames.get(stepName)?.add(stepName);
      }

      const hasTasksKey = hasOwn(step, 'tasks');
      if (hasTasksKey && !Array.isArray(step.tasks)) {
        return { errors: [createError(`Validation Error: Step '${stepName}' has 'tasks' but the value is not an array.`, [`${stepPath}.tasks`, stepPath])] };
      }
      const tasks = Array.isArray(step.tasks) ? step.tasks : [];
      const hasTasks = tasks.length > 0;
      if (hasTasksKey && !hasTasks) {
        return { errors: [createError(`Validation Error: Step '${stepName}' must define at least one task when using 'tasks'.`, [`${stepPath}.tasks`, stepPath])] };
      }

      const hasGoalKey = hasOwn(step, 'goal');
      const goalValue = hasGoalKey ? step.goal : null;
      const hasGoalContent = typeof goalValue === 'string' && goalValue.trim().length > 0;

      const hasScriptKey = hasOwn(step, 'script');
      const scriptValue = hasScriptKey ? step.script : null;
      const hasScriptContent = typeof scriptValue === 'string' && scriptValue.trim().length > 0;

      const hasApprovalKey = hasOwn(step, 'approval');
      const approvalValue = hasApprovalKey ? step.approval : null;

      if (hasGoalKey && !hasGoalContent) {
        return { errors: [createError(`Validation Error: Step '${stepName}' has an empty 'goal'.`, [`${stepPath}.goal`, stepPath])] };
      }
      if (hasScriptKey && !hasScriptContent) {
        return { errors: [createError(`Validation Error: Step '${stepName}' has an empty 'script'.`, [`${stepPath}.script`, stepPath])] };
      }
      const stepCondition = safeString(step.condition);
      if (!llmEnabled && stepCondition) {
        return { errors: [createError(`Validation Error: Pipeline has LLM disabled but step '${stepName}' defines condition.`, [`${stepPath}.condition`, stepPath])] };
      }
      if (!llmEnabled && hasGoalContent) {
        return { errors: [createError(`Validation Error: Pipeline has LLM disabled but step '${stepName}' defines goal.`, [`${stepPath}.goal`, stepPath])] };
      }
      if (!llmEnabled && hasScriptContent && stepHasBlockingKnowledge) {
        return {
          errors: [
            createError(`Validation Error: Pipeline has LLM disabled but script step '${stepName}' uses blocking knowledge context.`, [
              `${stepPath}.script`,
              `${stepPath}.knowledge_context`,
              'knowledge_context',
              stepPath,
            ]),
          ],
        };
      }

      if (hasGoalKey && hasScriptKey) {
        return {
          errors: [
            createError(`Validation Error: Step '${stepName}' cannot define both 'goal' and 'script'.`, [
              `${stepPath}.goal`,
              `${stepPath}.script`,
              stepPath,
            ]),
          ],
        };
      }

      const hasLegacyContent = hasGoalContent || hasScriptContent;
      if (!isInclude && !hasTasks && hasLegacyContent) {
        stepToTaskNames.get(stepName)?.add(stepName);
      }
      const stepOutputs = parseTaskOutputDeclarations(step.outputs, `Step '${stepName}' outputs`);
      if (stepOutputs.error) {
        return { errors: [createError(`Validation Error: ${stepOutputs.error}`, [`${stepPath}.outputs`, stepPath])] };
      }
      if (stepOutputs.outputs.length > 0) {
        if (isInclude) {
          const normalizedInclude = safeString(includeValue).toLowerCase();
          if (!normalizedInclude.startsWith('pipeline:')) {
            return {
              errors: [
                createError(`Validation Error: Include step '${stepName}' can declare outputs only for pipeline includes.`, [
                  `${stepPath}.outputs`,
                  `${stepPath}.include`,
                  stepPath,
                ]),
              ],
            };
          }
          if (step.sync !== true) {
            return {
              errors: [
                createError(`Validation Error: Pipeline include step '${stepName}' must use sync: true to declare parent-visible outputs.`, [
                  `${stepPath}.sync`,
                  `${stepPath}.outputs`,
                  stepPath,
                ]),
              ],
            };
          }
          outputDeclarations.set(producerTaskKey(stepName, stepName), new Set(stepOutputs.outputs.map(output => output.name)));
        }
        if (!isInclude && hasApprovalKey) {
          return { errors: [createError(`Validation Error: Approval step '${stepName}' cannot declare task outputs.`, [`${stepPath}.outputs`, stepPath])] };
        }
        if (!isInclude && hasTasks) {
          return { errors: [createError(`Validation Error: Step '${stepName}' has tasks, so outputs must be declared on individual tasks.`, [`${stepPath}.outputs`, stepPath])] };
        }
        if (!isInclude) {
          stepToTaskNames.get(stepName)?.add(stepName);
          outputDeclarations.set(producerTaskKey(stepName, stepName), new Set(stepOutputs.outputs.map(output => output.name)));
        }
      }

      if (hasApprovalKey) {
        if (!isPlainObject(approvalValue)) {
          return { errors: [createError(`Validation Error: Approval step '${stepName}' must define approval as an object.`, [`${stepPath}.approval`, stepPath])] };
        }

        const approval = approvalValue as Record<string, unknown>;
        const approvalType = safeString(approval.type).trim();
        if (!approvalType) {
          return { errors: [createError(`Validation Error: Approval step '${stepName}' must define approval.type.`, [`${stepPath}.approval.type`, `${stepPath}.approval`, stepPath])] };
        }
        if (!allowed.test(approvalType)) {
          return {
            errors: [
              createError(`Validation Error: Approval step '${stepName}' approval.type can only contain letters, numbers, underscores, dots, and hyphens.`, [
                `${stepPath}.approval.type`,
                `${stepPath}.approval`,
                stepPath,
              ]),
            ],
          };
        }

        const teams = Array.isArray(approval.teams) ? approval.teams : [];
        if (teams.length === 0) {
          return { errors: [createError(`Validation Error: Approval step '${stepName}' must assign at least one approval team.`, [`${stepPath}.approval.teams`, `${stepPath}.approval`, stepPath])] };
        }
        const seenTeams = new Set<string>();
        for (const rawTeam of teams) {
          const team = typeof rawTeam === 'string' ? rawTeam.trim() : '';
          const normalizedTeam = team.replace(/\\/g, '/').replace(/^\/+|\/+$/g, '');
          const segments = normalizedTeam.split('/');
          if (!team || team.startsWith('/') || team.startsWith('~') || segments.some(segment => !segment || segment === '.' || segment === '..')) {
            return {
              errors: [
                createError(`Validation Error: Approval step '${stepName}' approval team '${team || '<empty>'}' must be a relative team path.`, [
                  `${stepPath}.approval.teams`,
                  `${stepPath}.approval`,
                  stepPath,
                ]),
              ],
            };
          }
          const teamKey = normalizedTeam.toLowerCase();
          if (seenTeams.has(teamKey)) {
            return {
              errors: [
                createError(`Validation Error: Approval step '${stepName}' repeats approval team '${team}'.`, [
                  `${stepPath}.approval.teams`,
                  `${stepPath}.approval`,
                  stepPath,
                ]),
              ],
            };
          }
          seenTeams.add(teamKey);
        }
        if (hasOwn(approval, 'allow_self_approval') && typeof approval.allow_self_approval !== 'boolean') {
          return {
            errors: [
              createError(`Validation Error: Approval step '${stepName}' approval.allow_self_approval must be true or false.`, [
                `${stepPath}.approval.allow_self_approval`,
                `${stepPath}.approval`,
                stepPath,
              ]),
            ],
          };
        }
        if (hasOwn(approval, 'timeout')) {
          const timeout = typeof approval.timeout === 'string' ? approval.timeout.trim() : '';
          if (!timeout || !isPositiveGoDuration(timeout)) {
            return {
              errors: [
                createError(`Validation Error: Approval step '${stepName}' approval.timeout must be a positive duration.`, [
                  `${stepPath}.approval.timeout`,
                  `${stepPath}.approval`,
                  stepPath,
                ]),
              ],
            };
          }
        }
      }

      if (!isInclude && !hasTasks && !hasLegacyContent && !hasApprovalKey) {
        return {
          errors: [
            createError(`Validation Error: Step '${stepName}' must contain 'include', 'tasks', 'goal', 'script', or 'approval'.`, [
              `${stepPath}.include`,
              `${stepPath}.tasks`,
              `${stepPath}.goal`,
              `${stepPath}.script`,
              `${stepPath}.approval`,
              stepPath,
            ]),
          ],
        };
      }
      if (isInclude && (hasTasksKey || hasGoalKey || hasScriptKey || hasApprovalKey)) {
        return {
          errors: [
            createError(`Validation Error: Step '${stepName}' is an include step and cannot also contain tasks, goal, script, or approval.`, [
              `${stepPath}.include`,
              `${stepPath}.tasks`,
              `${stepPath}.goal`,
              `${stepPath}.script`,
              `${stepPath}.approval`,
              stepPath,
            ]),
          ],
        };
      }
      if (hasApprovalKey && (hasTasksKey || hasGoalKey || hasScriptKey)) {
        return {
          errors: [
            createError(`Validation Error: Approval step '${stepName}' cannot also contain tasks, goal, or script.`, [
              `${stepPath}.approval`,
              `${stepPath}.tasks`,
              `${stepPath}.goal`,
              `${stepPath}.script`,
              stepPath,
            ]),
          ],
        };
      }
      if (llmEnabled && isInclude && hasOwn(step, 'mcp_profiles')) {
        return {
          errors: [createError(`Validation Error: Include step '${stepName}' cannot define mcp_profiles.`, [`${stepPath}.mcp_profiles`, stepPath])],
        };
      }
      if (llmEnabled && hasScriptContent && hasOwn(step, 'mcp_profiles')) {
        return {
          errors: [createError(`Validation Error: Script step '${stepName}' cannot define mcp_profiles.`, [`${stepPath}.mcp_profiles`, stepPath])],
        };
      }
      if (hasTasks && (hasGoalKey || hasScriptKey)) {
        return {
          errors: [
            createError(`Validation Error: Step '${stepName}' mixes tasks with goal/script.`, [
              `${stepPath}.tasks`,
              `${stepPath}.goal`,
              `${stepPath}.script`,
              stepPath,
            ]),
          ],
        };
      }

      if (hasTasks) {
        const taskNames = new Set<string>();
        for (let taskIndex = 0; taskIndex < tasks.length; taskIndex += 1) {
          const task = tasks[taskIndex] as unknown;
          const taskPath = `${stepPath}.tasks[${taskIndex}]`;
          if (!isPlainObject(task) || !safeString(task.name)) {
            return {
              errors: [createError(`Validation Error: A task in step '${stepName}' is missing its name.`, [`${taskPath}.name`, taskPath])],
            };
          }

          const taskName = safeString(task.name);
          if (taskNames.has(taskName)) {
            return { errors: [createError(`Validation Error: Duplicate task name '${taskName}' in step '${stepName}'.`, [`${taskPath}.name`, taskPath])] };
          }
          taskNames.add(taskName);
          stepToTaskNames.get(stepName)?.add(taskName);

          const taskVariables = task.variables;
          const taskVariablesError = validateRuntimeVariableMap(taskVariables, `Task '${taskName}' in step '${stepName}' variables`);
          if (taskVariablesError) {
            return { errors: [createError(`Validation Error: ${taskVariablesError}`, [`${taskPath}.variables`, taskPath])] };
          }
          if (isPlainObject(taskVariables)) {
            runtimeVariableConsumers.push({
              variables: taskVariables,
              consumer: {
                stepName,
                taskName,
                stepLevel: false,
              },
              path: `${taskPath}.variables`,
            });
          }

          const taskOutputs = parseTaskOutputDeclarations(task.outputs, `Task '${taskName}' in step '${stepName}' outputs`);
          if (taskOutputs.error) {
            return { errors: [createError(`Validation Error: ${taskOutputs.error}`, [`${taskPath}.outputs`, taskPath])] };
          }
          if (taskOutputs.outputs.length > 0) {
            outputDeclarations.set(producerTaskKey(stepName, taskName), new Set(taskOutputs.outputs.map(output => output.name)));
          }

          const taskHasGoalKey = hasOwn(task, 'goal');
          const taskGoalValue = taskHasGoalKey ? task.goal : null;
          const taskHasGoalContent = typeof taskGoalValue === 'string' && taskGoalValue.trim().length > 0;

          const taskHasScriptKey = hasOwn(task, 'script');
          const taskScriptValue = taskHasScriptKey ? task.script : null;
          const taskHasScriptContent = typeof taskScriptValue === 'string' && taskScriptValue.trim().length > 0;

          if (taskHasGoalKey && taskHasScriptKey) {
            return {
              errors: [
                createError(`Validation Error: Task '${taskName}' in step '${stepName}' cannot define both 'goal' and 'script'.`, [
                  `${taskPath}.goal`,
                  `${taskPath}.script`,
                  taskPath,
                ]),
              ],
            };
          }
          if (taskHasGoalKey && !taskHasGoalContent) {
            return {
              errors: [createError(`Validation Error: Task '${taskName}' in step '${stepName}' has an empty 'goal'.`, [`${taskPath}.goal`, taskPath])],
            };
          }
          if (taskHasScriptKey && !taskHasScriptContent) {
            return {
              errors: [createError(`Validation Error: Task '${taskName}' in step '${stepName}' has an empty 'script'.`, [`${taskPath}.script`, taskPath])],
            };
          }
          if (!llmEnabled && taskHasGoalContent) {
            return {
              errors: [createError(`Validation Error: Pipeline has LLM disabled but task '${taskName}' in step '${stepName}' defines goal.`, [`${taskPath}.goal`, taskPath])],
            };
          }
          if (!llmEnabled && taskHasScriptContent && (stepHasBlockingKnowledge || knowledgeContextRefsContainBlocking(task.knowledge_context))) {
            return {
              errors: [
                createError(`Validation Error: Pipeline has LLM disabled but script task '${taskName}' in step '${stepName}' uses blocking knowledge context.`, [
                  `${taskPath}.script`,
                  `${taskPath}.knowledge_context`,
                  `${stepPath}.knowledge_context`,
                  'knowledge_context',
                  taskPath,
                ]),
              ],
            };
          }
          if (llmEnabled && taskHasScriptContent && hasOwn(task, 'mcp_profiles')) {
            return {
              errors: [createError(`Validation Error: Script task '${taskName}' in step '${stepName}' cannot define mcp_profiles.`, [`${taskPath}.mcp_profiles`, taskPath])],
            };
          }
          if (!taskHasGoalContent && !taskHasScriptContent) {
            return {
              errors: [
                createError(`Validation Error: Task '${taskName}' in step '${stepName}' must define either 'goal' or 'script'.`, [
                  `${taskPath}.goal`,
                  `${taskPath}.script`,
                  taskPath,
                ]),
              ],
            };
          }
        }

        for (let taskIndex = 0; taskIndex < tasks.length; taskIndex += 1) {
          const task = tasks[taskIndex] as unknown;
          if (!isPlainObject(task)) continue;
          const taskName = safeString(task.name);
          if (!taskName) continue;
          if (Array.isArray(task.depends_on)) {
            for (const dep of task.depends_on) {
              const depName = safeString(dep);
              if (depName && !taskNames.has(depName) && !depName.includes('.')) {
                const taskPath = `${stepPath}.tasks[${taskIndex}].depends_on`;
                return {
                  errors: [createError(`Validation Error: Task '${taskName}' in step '${stepName}' depends on unknown task '${depName}'.`, [taskPath])],
                };
              }
            }
          }
        }
      }
    }

    for (let index = 0; index < steps.length; index += 1) {
      const step = steps[index] as unknown;
      if (!isPlainObject(step)) continue;
      const stepName = safeString(step.name);
      if (!stepName) continue;
      if (Array.isArray(step.depends_on)) {
        for (const dep of step.depends_on) {
          const depName = safeString(dep);
          if (depName.includes(RUNTIME_OUTPUT_REFERENCE_PREFIX)) {
            return {
              errors: [
                createError(`Validation Error: Step '${stepName}' dependency '${depName}' cannot use a runtime output reference.`, [
                  `steps[${index}].depends_on`,
                  `steps[${index}]`,
                ]),
              ],
            };
          }
          const resolved = resolvePipelineTaskDependency(depName, stepName, stepNames, stepToTaskNames);
          if (depName && !resolved.ok) {
            return {
              errors: [
                createError(`Validation Error: Step '${stepName}' depends on unknown step '${depName}'.`, [
                  `steps[${index}].depends_on`,
                  `steps[${index}]`,
                ]),
              ],
            };
          }
        }
      }
      const tasks = Array.isArray(step.tasks) ? step.tasks : [];
      for (let taskIndex = 0; taskIndex < tasks.length; taskIndex += 1) {
        const task = tasks[taskIndex] as unknown;
        if (!isPlainObject(task)) continue;
        const taskName = safeString(task.name);
        if (!taskName || !Array.isArray(task.depends_on)) continue;
        for (const dep of task.depends_on) {
          const depName = safeString(dep);
          if (depName.includes(RUNTIME_OUTPUT_REFERENCE_PREFIX)) {
            return {
              errors: [
                createError(`Validation Error: Task '${taskName}' in step '${stepName}' dependency '${depName}' cannot use a runtime output reference.`, [
                  `steps[${index}].tasks[${taskIndex}].depends_on`,
                  `steps[${index}].tasks[${taskIndex}]`,
                ]),
              ],
            };
          }
          const resolved = resolvePipelineTaskDependency(depName, stepName, stepNames, stepToTaskNames);
          if (!depName || !resolved.ok || !resolved.taskName) {
            return {
              errors: [
                createError(`Validation Error: Task '${taskName}' in step '${stepName}' has invalid dependency '${depName}'. Tasks can depend on another task in the same step or use a qualified step.task dependency.`, [
                  `steps[${index}].tasks[${taskIndex}].depends_on`,
                  `steps[${index}].tasks[${taskIndex}]`,
                ]),
              ],
            };
          }
        }
      }
    }

    const dependencyIndex = buildPipelineDependencyIndex(steps, stepNames, stepToTaskNames);
    for (const entry of runtimeVariableConsumers) {
      const error = validateRuntimeOutputRefsInVariables(entry.variables, entry.consumer, stepToTaskNames, outputDeclarations, dependencyIndex);
      if (error) {
        return { errors: [createError(error, [entry.path])] };
      }
    }

    return { errors: [] };
  } catch (error: unknown) {
    const record = isPlainObject(error) ? error : null;
    const markRecord = record && isPlainObject(record.mark) ? record.mark : null;
    const markLine = markRecord && typeof markRecord.line === 'number' ? markRecord.line : null;
    const message = error instanceof Error ? error.message : typeof record?.message === 'string' ? record.message : String(error);

    if (typeof markLine === 'number') {
      return { errors: [createError(`YAML Parsing Error: ${message}`, [`line:${markLine + 1}`])] };
    }
    return { errors: [createError(`YAML Parsing Error: ${message}`)] };
  }
}

export type LineInfo = {
  line: string;
  start: number;
  end: number;
  column: number;
  indent: number;
};

export function getCurrentLineInfo(text: string, pos: number): LineInfo {
  const start = text.lastIndexOf('\n', pos - 1) + 1;
  let end = text.indexOf('\n', pos);
  if (end === -1) end = text.length;
  const line = text.slice(start, end);
  const indent = line.match(/^\s*/)?.[0].length ?? 0;
  return { line, start, end, column: pos - start, indent };
}

export type LabSuggestionType =
  | 'include'
  | 'depends_on'
  | 'secrets'
  | 'variables'
  | 'agent_role'
  | 'model'
  | 'mcp_profile'
  | 'runtime_pool'
  | 'directive-value'
  | 'pipeline-key'
  | 'step-key'
  | 'task-key';

export type LabSuggestionContext = {
  type: LabSuggestionType;
  title: string;
  prefix: string;
  rangeStart: number;
  rangeEnd: number;
  insertSuffix?: string;
  insertPrefix?: string;
  key?: string;
  existingKeys?: Set<string>;
};

export type LabSuggestionItem = {
  value: string;
  label?: string;
  hint?: string;
  snippet?: string;
  overrideSuffix?: string;
};

export function findParentBlock(beforeText: string, targetKeys: string[], currentIndent: number, stopKeys: string[] = []): string | null {
  const lines = beforeText.split('\n');
  let indentCursor = currentIndent;
  for (let i = lines.length - 1; i >= 0; i -= 1) {
    const line = lines[i];
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;

    const indent = line.match(/^\s*/)?.[0].length ?? 0;
    if (indent < indentCursor) {
      const colonIdx = trimmed.indexOf(':');
      if (colonIdx !== -1) {
        const key = trimmed.slice(0, colonIdx).trim();
        if (stopKeys.includes(key)) return null;
        if (targetKeys.includes(key)) return key;
        if (indent === 0) return null;
      } else if (indent === 0) {
        return null;
      }
      indentCursor = indent;
    }
  }
  return null;
}

function collectExistingKeysForContext(text: string, lineInfo: LineInfo, type: LabSuggestionType): Set<string> {
  const keys = new Set<string>();
  if (type === 'pipeline-key') return keys;

  const lines = text.split('\n');
  let currentLineIdx = 0;
  let count = 0;
  for (let i = 0; i < lines.length; i += 1) {
    if (count + lines[i].length >= lineInfo.start) {
      currentLineIdx = i;
      break;
    }
    count += lines[i].length + 1;
  }

  const targetIndent = lineInfo.indent;
  let start = currentLineIdx;
  while (start >= 0) {
    const line = lines[start];
    const indent = line.match(/^\s*/)?.[0].length ?? 0;
    if (indent < targetIndent) break;
    if (indent === targetIndent && line.trim().startsWith('-')) break;
    start -= 1;
  }
  if (start < 0) start = 0;

  for (let i = start; i < lines.length; i += 1) {
    const line = lines[i];
    const trimmed = line.trim();
    if (!trimmed) continue;
    const indent = line.match(/^\s*/)?.[0].length ?? 0;
    if (i > start && indent <= targetIndent && trimmed.startsWith('-')) break;
    if (indent < targetIndent) break;

    const match = trimmed.match(/^(-\s+)?([a-zA-Z0-9_]+):/);
    if (match) keys.add(match[2]);
  }
  return keys;
}

function detectDirectiveValueContext(lineInfo: LineInfo, selectionEnd: number): LabSuggestionContext | null {
  const rawLine = lineInfo.line;
  const colonIndex = rawLine.indexOf(':');
  if (colonIndex === -1 || lineInfo.column <= colonIndex) return null;

  const key = rawLine.slice(lineInfo.indent, colonIndex).trim();
  const ws = rawLine.slice(colonIndex + 1).match(/^\s*/)?.[0] ?? '';
  const valueOffsetLocal = colonIndex + 1 + ws.length;
  const rangeStart = lineInfo.start + valueOffsetLocal;
  const currentValue = rawLine.slice(valueOffsetLocal, lineInfo.column).trim();

  const metadata = DIRECTIVE_VALUE_METADATA[key];
  if (!metadata && key === 'agent_role') {
    return {
      type: 'agent_role',
      title: 'Agent Profiles',
      key,
      prefix: currentValue,
      rangeStart,
      rangeEnd: Math.max(rangeStart, selectionEnd),
      insertSuffix: '',
    };
  }
  if (!metadata && key === 'model') {
    return {
      type: 'model',
      title: 'LLM Profiles',
      key,
      prefix: currentValue,
      rangeStart,
      rangeEnd: Math.max(rangeStart, selectionEnd),
      insertSuffix: '',
    };
  }
  if (!metadata && key === 'runtime_pool') {
    return {
      type: 'runtime_pool',
      title: 'Runtime Pools',
      key,
      prefix: currentValue,
      rangeStart,
      rangeEnd: Math.max(rangeStart, selectionEnd),
      insertSuffix: '',
    };
  }
  if (!metadata) return null;

  return {
    type: 'directive-value',
    title: metadata.title,
    key,
    prefix: currentValue,
    rangeStart,
    rangeEnd: Math.max(rangeStart, selectionEnd),
    insertSuffix: '',
  };
}

function detectListEntryContext(lineInfo: LineInfo, selectionEnd: number, beforeLine: string, keyName: string): Omit<LabSuggestionContext, 'type' | 'title'> | null {
  const trimmed = lineInfo.line.trimStart();
  if (!trimmed.startsWith('-')) return null;
  const parent = findParentBlock(beforeLine, [keyName], lineInfo.indent);
  if (parent !== keyName) return null;
  const dashMatch = lineInfo.line.match(/^(\s*-\s*)/);
  const valueStart = dashMatch ? dashMatch[0].length : lineInfo.indent;
  const rangeStart = lineInfo.start + valueStart;
  return {
    prefix: lineInfo.line.slice(valueStart, lineInfo.column).trim(),
    rangeStart,
    rangeEnd: Math.max(rangeStart, selectionEnd),
    insertSuffix: '',
    insertPrefix: dashMatch && /\s$/.test(dashMatch[0]) ? '' : ' ',
  };
}

function detectVariableContext(lineInfo: LineInfo, selectionEnd: number, beforeLine: string): LabSuggestionContext | null {
  const parent = findParentBlock(beforeLine, ['variables'], lineInfo.indent, ['steps', 'tasks']);
  if (parent !== 'variables') return null;

  const local = lineInfo.line.slice(lineInfo.indent);
  const trimmedLocal = local.trimStart();

  if (trimmedLocal.startsWith('-')) {
    const dashMatch = local.match(/^-\s*/);
    const dashSegment = dashMatch ? dashMatch[0] : '-';
    const valueStartLocal = lineInfo.indent + dashSegment.length;
    const relativeText = lineInfo.line.slice(valueStartLocal, lineInfo.column);
    const trimmedValue = relativeText.trim();
    const relativeOffset = trimmedValue ? relativeText.indexOf(trimmedValue) : 0;
    const rangeStart = lineInfo.start + valueStartLocal + relativeOffset;
    return {
      type: 'variables',
      title: 'Variables',
      prefix: trimmedValue,
      rangeStart,
      rangeEnd: Math.max(rangeStart, selectionEnd),
      insertSuffix: '',
      insertPrefix: dashSegment.endsWith(' ') ? '' : ' ',
    };
  }

  const colonIndex = lineInfo.line.indexOf(':', lineInfo.indent);
  const hasColon = colonIndex !== -1;
  const valueEnd = hasColon ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
  const rawPrefix = lineInfo.line.slice(lineInfo.indent, valueEnd);
  const prefix = rawPrefix.trim();
  const computedRangeEnd = hasColon && colonIndex < selectionEnd ? lineInfo.start + colonIndex : selectionEnd;
  const safeRangeEnd = Math.max(lineInfo.start + lineInfo.indent, computedRangeEnd);
  return {
    type: 'variables',
    title: 'Variables',
    prefix,
    rangeStart: lineInfo.start + lineInfo.indent,
    rangeEnd: safeRangeEnd,
    insertSuffix: hasColon ? '' : ': ',
    insertPrefix: '',
  };
}

function detectDirectiveKeyContext(lineInfo: LineInfo, beforeLine: string, fullText: string): LabSuggestionContext | null {
  const rawLine = lineInfo.line;
  if (!rawLine) return null;
  const trimmed = rawLine.trim();
  if (trimmed.startsWith('#')) return null;

  const colonIndex = rawLine.indexOf(':');
  if (colonIndex !== -1 && lineInfo.column > colonIndex) return null;

  let type: LabSuggestionType = 'pipeline-key';
  let rangeStart = 0;
  let rangeEnd = 0;
  let prefix = '';
  let parent = findParentBlock(beforeLine, ['steps', 'tasks'], lineInfo.indent);

  if (trimmed.startsWith('-')) {
    const dashMatch = rawLine.match(/^(\s*-\s*)/);
    const dashSegment = dashMatch ? dashMatch[0] : '- ';
    const valueStartLocal = dashSegment.length;
    const valueSlice = rawLine.slice(valueStartLocal, Math.max(lineInfo.column, valueStartLocal));
    rangeStart = lineInfo.start + valueStartLocal;
    const endIndex = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : Math.max(lineInfo.column, valueStartLocal);
    prefix = valueSlice.slice(0, Math.max(0, endIndex - valueStartLocal)).trim();
    parent = findParentBlock(beforeLine, ['steps', 'tasks'], lineInfo.indent);
    if (parent === 'steps') type = 'step-key';
    else if (parent === 'tasks') type = 'task-key';
    else return null;
  } else {
    if (parent === 'steps') type = 'step-key';
    else if (parent === 'tasks') type = 'task-key';
    else if (lineInfo.indent !== 0) return null;

    rangeStart = lineInfo.start + lineInfo.indent;
    const endIndex = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
    prefix = rawLine.slice(lineInfo.indent, endIndex).trim();
  }

  const colonBound = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : Math.max(lineInfo.column, rangeStart - lineInfo.start);
  rangeEnd = Math.max(rangeStart, lineInfo.start + colonBound);

  const title = type === 'pipeline-key' ? 'Pipeline Directives' : type === 'step-key' ? 'Step Directives' : 'Task Directives';
  const existingKeys = collectExistingKeysForContext(fullText, lineInfo, type);

  return {
    type,
    title,
    prefix,
    rangeStart,
    rangeEnd,
    insertSuffix: ': ',
    existingKeys,
  };
}

export function detectSuggestionContext(text: string, caret: number, selectionEnd: number): LabSuggestionContext | null {
  const lineInfo = getCurrentLineInfo(text, caret);
  const beforeLine = text.slice(0, lineInfo.start);
  const lineStr = lineInfo.line;

  const incMatch = lineStr.match(/include:\s*"?([^"]*)$/);
  if (incMatch) {
    const offset = incMatch.index ?? 0;
    return {
      type: 'include',
      title: 'Include Targets',
      prefix: incMatch[1],
      rangeStart: lineInfo.start + offset + incMatch[0].length - incMatch[1].length,
      rangeEnd: selectionEnd,
      insertSuffix: '',
    };
  }

  const depMatch = lineStr.match(/depends_on:\s*\[?([^\]]*)$/);
  if (depMatch || findParentBlock(beforeLine, ['depends_on'], lineInfo.indent)) {
    const word = lineStr.slice(0, lineInfo.column).split(/[[\s,]+/).pop() ?? '';
    return {
      type: 'depends_on',
      title: 'Dependencies',
      prefix: word,
      rangeStart: caret - word.length,
      rangeEnd: selectionEnd,
      insertSuffix: '',
    };
  }

  const secList = detectListEntryContext(lineInfo, selectionEnd, beforeLine, 'secrets');
  if (secList) return { ...secList, type: 'secrets', title: 'Secrets' };

  const mcpList = detectListEntryContext(lineInfo, selectionEnd, beforeLine, 'mcp_profiles');
  if (mcpList) return { ...mcpList, type: 'mcp_profile', title: 'MCP Profiles' };

  const variableContext = detectVariableContext(lineInfo, selectionEnd, beforeLine);
  if (variableContext) return variableContext;

  const valContext = detectDirectiveValueContext(lineInfo, selectionEnd);
  if (valContext) return valContext;

  const directiveCtx = detectDirectiveKeyContext(lineInfo, beforeLine, text);
  if (directiveCtx) return directiveCtx;

  return null;
}

function collectStepNames(text: string): string[] {
  const names: string[] = [];
  const regex = /^\s*-\s*name:\s*([a-zA-Z0-9_-]+)/gm;
  let match: RegExpExecArray | null = null;
  while ((match = regex.exec(text)) !== null) {
    names.push(match[1]);
  }
  return names;
}

export function buildSuggestionItems(
  ctx: LabSuggestionContext,
  text: string,
  opts: {
    secrets: string[];
    variables: string[];
    agentProfiles?: string[];
    llmProfiles: string[];
    mcpProfiles?: string[];
    runtimePools?: string[];
    reusableSteps: string[];
    pipelineIds: string[];
  }
): LabSuggestionItem[] {
  const prefix = ctx.prefix || '';
  let pool: LabSuggestionItem[] = [];

  if (ctx.type === 'depends_on') {
    pool = collectStepNames(text).map(n => ({ value: n, label: n }));
  } else if (ctx.type === 'secrets') {
    pool = opts.secrets.map(s => ({ value: s, label: s }));
  } else if (ctx.type === 'variables') {
    pool = opts.variables.map(v => ({ value: v, label: v }));
  } else if (ctx.type === 'agent_role') {
    pool = (opts.agentProfiles || []).map(p => ({ value: p, label: p }));
  } else if (ctx.type === 'model') {
    pool = opts.llmProfiles.map(p => ({ value: p, label: p }));
  } else if (ctx.type === 'mcp_profile') {
    pool = (opts.mcpProfiles || []).map(p => ({ value: p, label: p }));
  } else if (ctx.type === 'runtime_pool') {
    pool = (opts.runtimePools || []).map(p => ({ value: p, label: p }));
  } else if (ctx.type === 'include') {
    pool = [
      ...opts.reusableSteps.map(s => ({ value: `step:${s}`, label: `step:${s}` })),
      ...opts.pipelineIds.map(p => ({ value: `pipeline:${p}`, label: `pipeline:${p}` })),
    ];
  } else if (ctx.type === 'directive-value') {
    const metadata = ctx.key ? DIRECTIVE_VALUE_METADATA[ctx.key] : undefined;
    if (metadata?.values?.length) {
      pool = metadata.values.map(v => ({ value: v, label: v }));
    }
  } else if (ctx.type === 'pipeline-key') {
    pool = PIPELINE_DIRECTIVES.map(d => ({ value: d.key, label: d.key, hint: d.hint }));
  } else if (ctx.type === 'step-key') {
    pool = STEP_DIRECTIVES.map(d => ({ value: d.key, label: d.key, hint: d.hint }));
  } else if (ctx.type === 'task-key') {
    pool = TASK_DIRECTIVES.map(d => ({ value: d.key, label: d.key, hint: d.hint }));
  }

  const existing = ctx.existingKeys ?? new Set<string>();
  pool = pool.filter(item => !existing.has(item.value));
  if (!pool.length) return [];

  const normalizedPrefix = prefix.toLowerCase();
  return pool.filter(item => item.value.toLowerCase().includes(normalizedPrefix)).slice(0, 15);
}

export function suggestionCopyForContext(contextInfo: LabSuggestionContext | null): { title: string; subtitle: string; footnote: string } {
  const type = contextInfo?.type;
  switch (type) {
    case 'variables':
      return { title: 'Variables', subtitle: 'Insert variables scoped to your scope.', footnote: 'Tab to accept inline hint.' };
    case 'secrets':
      return { title: 'Secrets', subtitle: 'Available secret names.', footnote: 'Tab to accept inline hint.' };
    case 'include':
      return { title: 'Include targets', subtitle: 'Reusable steps and pipelines.', footnote: 'Click or Tab to insert.' };
    case 'agent_role':
      return { title: 'Agent profiles', subtitle: 'AI roles/personas for this pipeline or step.', footnote: 'Click or Tab to insert.' };
    case 'model':
      return { title: 'LLM profiles', subtitle: 'Profiles allowed for the selected scope.', footnote: 'Click or Tab to insert.' };
    case 'mcp_profile':
      return { title: 'MCP profiles', subtitle: 'Approved external tool bundles for goal tasks.', footnote: 'Click or Tab to insert.' };
    case 'runtime_pool':
      return { title: 'Runtime pools', subtitle: 'Configured Kubernetes scheduling pools.', footnote: 'Click or Tab to insert.' };
    case 'pipeline-key':
      return { title: 'Pipeline directives', subtitle: 'Keys allowed at root level.', footnote: 'Tab to accept inline hint.' };
    case 'step-key':
      return { title: 'Step directives', subtitle: 'Keys allowed within steps.', footnote: 'Tab to accept inline hint.' };
    case 'task-key':
      return { title: 'Task directives', subtitle: 'Keys allowed within tasks.', footnote: 'Tab to accept inline hint.' };
    case 'depends_on':
      return { title: 'Dependencies', subtitle: 'Existing step/task names.', footnote: 'Tab to accept inline hint.' };
    case 'directive-value':
      return { title: 'Allowed values', subtitle: 'Insert permitted values.', footnote: 'Tab to accept inline hint.' };
    default:
      return { title: 'Suggestions', subtitle: 'Context-aware helpers.', footnote: 'Tab to accept inline hint.' };
  }
}

export function applyEnterIndent(
  value: string,
  selectionStart: number,
  selectionEnd: number
): { nextValue: string; nextCursor: number } {
  const start = selectionStart;
  const end = selectionEnd;
  const lineInfo = getCurrentLineInfo(value, start);
  const before = value.slice(0, start);
  const after = value.slice(end);
  const currentIndent = lineInfo.line.match(/^\s*/)?.[0] ?? '';
  const trimmed = lineInfo.line.trim();
  const parentBlock = findParentBlock(value.slice(0, lineInfo.start), ['steps', 'tasks'], lineInfo.indent, Array.from(LIST_KEYS_SIMPLE));

  let newIndent = currentIndent;
  let listPrefix = '';

  if (/^-\s*name\s*:/i.test(trimmed)) {
    newIndent = ' '.repeat(lineInfo.indent + 2);
  } else if (trimmed.startsWith('-')) {
    newIndent = currentIndent;
    const parent = findParentBlock(
      value.slice(0, lineInfo.start),
      ['steps', 'tasks'],
      lineInfo.indent,
      Array.from(LIST_KEYS_SIMPLE)
    );
    if (parent && LIST_KEYS_WITH_NAME_TEMPLATE.has(parent)) {
      listPrefix = '- name: ';
    } else {
      listPrefix = '- ';
    }
  } else if (trimmed.endsWith(':')) {
    newIndent = `${currentIndent}  `;
    const key = trimmed.slice(0, -1).trim();
    if (LIST_KEYS_WITH_NAME_TEMPLATE.has(key)) {
      listPrefix = '- name: ';
    } else if (LIST_KEYS_SIMPLE.has(key) && !parentBlock) {
      listPrefix = '- ';
    }
  } else {
    if (parentBlock && LIST_KEYS_WITH_NAME_TEMPLATE.has(parentBlock) && trimmed === '') {
      newIndent = ' '.repeat(lineInfo.indent);
      listPrefix = '- name: ';
    } else {
      newIndent = currentIndent;
    }
  }

  const insertion = `\n${newIndent}${listPrefix}`;
  const nextValue = before + insertion + after;
  const nextCursor = before.length + insertion.length;
  return { nextValue, nextCursor };
}
