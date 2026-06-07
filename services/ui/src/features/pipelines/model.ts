import yaml from 'js-yaml';
import { validatePipelineYamlStrict } from '../../lib/lab.js';
import { findLineNumberForKey, normalizeLineNumber, parseYamlWithLocation } from '../../lib/yamlValidation.js';

export const PIPELINE_DIRECTIVES = [
  'name',
  'version',
  'description',
  'container_image',
  'working_directory',
  'variables',
  'steps',
  'timeout',
  'llm_enabled',
  'llm_profile',
  'mcp_profiles',
  'runtime_pool',
  'affinity_enabled',
  'knowledge_context',
  'llm_output_sharing',
  'llm_content_sharing',
  'llm_content_include',
  'llm_content_ignore',
  'display_options',
];

export const STEP_DIRECTIVES = [
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
  'ignore_failure',
  'llm_profile',
  'mcp_profiles',
  'runtime_pool',
  'knowledge_context',
  'llm_output_sharing',
];

export const TASK_DIRECTIVES = [
  'name',
  'goal',
  'script',
  'depends_on',
  'ignore_failure',
  'llm_profile',
  'mcp_profiles',
  'knowledge_context',
  'llm_output_sharing',
  'variables',
];

export type PipelineListItem = { id: string; source?: string };

export type PipelineDetail = {
  id: string;
  name: string;
  description: string;
  version: string;
  path: string;
  rawYaml: string;
  stepNames: string[];
  variables: string[];
  includedDependencies: string[];
  dependencyEdges: { from: string; to: string }[];
  containerImage?: string;
  source?: string;
};

export type PipelineGraphTaskDefinition = {
  name: string;
  goal?: string;
  script?: string;
  depends_on?: string[];
  ignore_failure?: boolean;
  variables?: Record<string, string>;
};

export type PipelineGraphApprovalDefinition = {
  type?: string;
  groups?: string[];
  allow_self_approval?: boolean;
};

export type PipelineGraphStepConfiguration = {
  include?: string;
  sync?: boolean;
  approval?: PipelineGraphApprovalDefinition;
  image?: string;
  secrets?: string[];
  volumes?: string[];
  variables?: Record<string, string>;
  ignore_failure?: boolean;
  llm_output_sharing?: boolean;
  goal?: string;
  script?: string;
  tasks?: PipelineGraphTaskDefinition[];
};

export type PipelineGraphTaskDetail = {
  task_id: string;
  step_name: string;
  task_name: string;
  status: string;
  exit_code?: number | null;
  started_at?: string;
  finished_at?: string;
  task_index: number;
};

export type PipelineGraphStepDetail = {
  name: string;
  status: string;
  depends_on: string[];
  tasks: PipelineGraphTaskDetail[];
  configuration?: PipelineGraphStepConfiguration;
};

export type PipelineGraphDefinition = {
  name?: string;
  description?: string;
  version?: string;
  steps?: {
    name: string;
    description?: string;
    depends_on?: string[];
    approval?: PipelineGraphApprovalDefinition;
    tasks?: PipelineGraphTaskDefinition[];
    goal?: string;
    script?: string;
  }[];
};

export type PipelineGraphData = {
  steps: PipelineGraphStepDetail[];
  definition?: PipelineGraphDefinition;
  error: string | null;
};

export type ValidationError = {
  message: string;
  line?: number;
  column?: number;
};

export type ValidationResult = {
  errors: ValidationError[];
};

export function normalizeRootPath(path: string) {
  const parts = path.trim().replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
  if (parts[0]?.toLowerCase() === 'root') parts.shift();
  return parts.join('/');
}

export function encodeId(id: string) {
  return id.split('/').map(encodeURIComponent).join('/');
}

export function splitIdentifier(id: string) {
  const parts = id.split('/').filter(Boolean);
  const name = decodeURIComponent(parts.pop() || '');
  const path = parts.map(decodeURIComponent).join('/');
  return { name, path };
}

export function normalizePipelineSource(source?: string) {
  const key = (source || '').trim().toLowerCase();
  if (!key) return 'database';
  if (key.includes('git')) return 'git';
  if (key.includes('draft')) return 'draft';
  if (key.includes('db')) return 'database';
  return key;
}

function normalizeStringArray(value: unknown) {
  return Array.isArray(value) ? value.map(v => (typeof v === 'string' ? v.trim() : '')).filter(Boolean) : [];
}

function normalizeVariables(value: unknown) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const entries: Record<string, string> = {};
  Object.entries(value as Record<string, unknown>).forEach(([key, val]) => {
    if (typeof val === 'string') entries[key] = val;
  });
  return Object.keys(entries).length ? entries : undefined;
}

function normalizeApproval(value: unknown): PipelineGraphApprovalDefinition | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const record = value as Record<string, unknown>;
  return {
    type: typeof record.type === 'string' ? record.type : undefined,
    groups: normalizeStringArray(record.groups),
    allow_self_approval: typeof record.allow_self_approval === 'boolean' ? record.allow_self_approval : undefined,
  };
}

export function validatePipelineYaml(rawYaml: string): ValidationResult {
  const trimmed = rawYaml.trim();
  if (!trimmed) {
    return { errors: [{ message: 'Pipeline definition cannot be empty.', line: 1 }] };
  }

  const { parsed, error: parseError } = parseYamlWithLocation(rawYaml);
  if (parseError) {
    return { errors: [parseError] };
  }

  const errors: ValidationError[] = [];
  const strict = validatePipelineYamlStrict(rawYaml);
  strict.errors.forEach(err => {
    errors.push({
      message: err.message,
      line: normalizeLineNumber(err.line),
    });
  });

  const pipelineObject = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : null;
  if (pipelineObject) {
    const steps = Array.isArray(pipelineObject.steps) ? (pipelineObject.steps as Array<Record<string, unknown>>) : [];
    if (steps.length > 0) {
      const containerImage = typeof pipelineObject.container_image === 'string' ? pipelineObject.container_image.trim() : '';
      const executableStepWithoutImage = steps.some(step => {
        if (!step || typeof step !== 'object') return false;
        const stepRecord = step as Record<string, unknown>;
        if (stepRecord.approval !== undefined) return false;
        const stepImage = typeof stepRecord.image === 'string' ? stepRecord.image.trim() : '';
        return !stepImage;
      });
      if (!containerImage && executableStepWithoutImage) {
        errors.push({
          message: "'container_image' is required when steps do not specify their own image.",
          line: findLineNumberForKey(rawYaml, 'container_image') ?? findLineNumberForKey(rawYaml, 'steps') ?? 1,
        });
      }
    }
  }

  return { errors };
}

export function buildPipelineGraphData(rawYaml?: string): PipelineGraphData {
  const base: PipelineGraphData = { steps: [], definition: undefined, error: null };
  if (!rawYaml) return base;

  type NormalizedStep = {
    name: string;
    description?: string;
    depends_on: string[];
    include?: string;
    sync?: boolean;
    approval?: PipelineGraphApprovalDefinition;
    image?: string;
    secrets?: string[];
    volumes?: string[];
    variables?: Record<string, string>;
    ignore_failure?: boolean;
    llm_output_sharing?: boolean;
    goal?: string;
    script?: string;
    tasks: PipelineGraphTaskDefinition[];
  };

  try {
    const parsed = yaml.load(rawYaml) as unknown;
    const parsedRecord = parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {};
    const rawSteps = Array.isArray(parsedRecord.steps) ? parsedRecord.steps : [];
    const normalizedSteps = rawSteps
      .map<NormalizedStep | null>((stepValue: unknown) => {
        const step = stepValue && typeof stepValue === 'object' ? (stepValue as Record<string, unknown>) : null;
        if (!step) return null;
        const name = typeof step.name === 'string' ? step.name.trim() : '';
        if (!name) return null;

        const taskDefs: PipelineGraphTaskDefinition[] = Array.isArray(step.tasks)
          ? step.tasks
              .map((taskValue: unknown) => {
                const task = taskValue && typeof taskValue === 'object' ? (taskValue as Record<string, unknown>) : null;
                if (!task) return null;
                const taskName = typeof task.name === 'string' ? task.name.trim() : '';
                if (!taskName) return null;
                return {
                  name: taskName,
                  goal: typeof task.goal === 'string' ? task.goal : undefined,
                  script: typeof task.script === 'string' ? task.script : undefined,
                  depends_on: normalizeStringArray(task.depends_on),
                  ignore_failure: typeof task.ignore_failure === 'boolean' ? task.ignore_failure : undefined,
                  variables: normalizeVariables(task.variables),
                } as PipelineGraphTaskDefinition;
              })
              .filter((task): task is PipelineGraphTaskDefinition => Boolean(task))
          : [];

        return {
          name,
          description: typeof step.description === 'string' ? step.description : undefined,
          depends_on: normalizeStringArray(step.depends_on),
          include: typeof step.include === 'string' ? step.include : undefined,
          sync: typeof step.sync === 'boolean' ? step.sync : undefined,
          approval: normalizeApproval(step.approval),
          image: typeof step.image === 'string' ? step.image : undefined,
          secrets: normalizeStringArray(step.secrets),
          volumes: normalizeStringArray(step.volumes),
          variables: normalizeVariables(step.variables),
          ignore_failure: typeof step.ignore_failure === 'boolean' ? step.ignore_failure : undefined,
          llm_output_sharing: typeof step.llm_output_sharing === 'boolean' ? step.llm_output_sharing : undefined,
          goal: typeof step.goal === 'string' ? step.goal : undefined,
          script: typeof step.script === 'string' ? step.script : undefined,
          tasks: taskDefs,
        };
      })
      .filter((step): step is NormalizedStep => Boolean(step));

    const definition: PipelineGraphDefinition | undefined =
      normalizedSteps.length > 0
        ? {
            name: typeof parsedRecord.name === 'string' ? parsedRecord.name : undefined,
            description: typeof parsedRecord.description === 'string' ? parsedRecord.description : undefined,
            version: typeof parsedRecord.version === 'string' ? parsedRecord.version : undefined,
            steps: normalizedSteps.map(step => ({
              name: step.name,
              description: step.description,
              depends_on: step.depends_on,
              approval: step.approval,
              tasks: step.tasks,
              goal: step.goal,
              script: step.script,
            })),
          }
        : undefined;

    const steps: PipelineGraphStepDetail[] = normalizedSteps.map(step => {
      const tasks: PipelineGraphTaskDetail[] = step.tasks.map((task, index) => ({
        task_id: `def-${step.name}-${task.name || index}`,
        step_name: step.name,
        task_name: task.name,
        status: 'pending',
        exit_code: null,
        started_at: undefined,
        finished_at: undefined,
        task_index: index,
      }));

      return {
        name: step.name,
        status: 'success',
        depends_on: step.depends_on,
        tasks,
        configuration: {
          include: step.include,
          sync: step.sync,
          approval: step.approval,
          image: step.image,
          secrets: step.secrets,
          volumes: step.volumes,
          variables: step.variables,
          ignore_failure: step.ignore_failure,
          llm_output_sharing: step.llm_output_sharing,
          goal: step.goal,
          script: step.script,
          tasks: step.tasks,
        },
      };
    });

    return { steps, definition, error: null };
  } catch (error) {
    return { steps: [], definition: undefined, error: error instanceof Error ? error.message : 'Unable to parse YAML' };
  }
}

export function parsePipelineYaml(raw: string, id: string, source?: string): PipelineDetail {
  let parsed: Record<string, unknown> | null = null;
  try {
    parsed = yaml.load(raw) as Record<string, unknown>;
  } catch (error) {
    console.warn('Failed to parse pipeline YAML', error);
  }

  const safe = (value: unknown) => (typeof value === 'string' ? value : '');
  const includedDeps: string[] = [];
  const dependencyEdges: { from: string; to: string }[] = [];
  const stepNames = Array.isArray(parsed?.steps)
    ? (parsed?.steps as Array<Record<string, unknown>>)
        .map(step => {
          const stepName = safe(step?.name).trim();
          const includeVal = safe(step?.include).trim();
          if (includeVal) includedDeps.push(includeVal);
          const deps = Array.isArray(step?.depends_on) ? (step?.depends_on as unknown[]) : [];
          deps.forEach(dep => {
            const from = safe(dep).trim();
            if (from && stepName) {
              dependencyEdges.push({ from, to: stepName });
            }
          });
          return stepName;
        })
        .filter(Boolean)
    : [];
  const variables = Array.isArray(parsed?.variables)
    ? (parsed?.variables as unknown[])
        .map(item => (typeof item === 'string' ? item : ''))
        .filter(Boolean)
    : [];

  const { name: fallbackName, path } = splitIdentifier(id);
  return {
    id,
    name: safe(parsed?.name) || fallbackName,
    description: safe(parsed?.description) || 'No description provided.',
    version: safe(parsed?.version) || 'latest',
    path,
    rawYaml: raw,
    stepNames,
    variables,
    includedDependencies: includedDeps,
    dependencyEdges,
    containerImage: safe((parsed as Record<string, unknown> | undefined)?.container_image ?? (parsed as Record<string, unknown> | undefined)?.containerImage),
    source,
  };
}
