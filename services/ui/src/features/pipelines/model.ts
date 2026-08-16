import * as yaml from 'js-yaml';
import { validatePipelineYamlStrict } from '../../lib/lab.js';
import { isGlobalResourceTeamPath } from '../../lib/resourceTeams.js';
import {
  findLineNumberForKey,
  normalizeLineNumber,
  parseTaskOutputDeclarations,
  parseYamlWithLocation,
  type TaskOutputDeclaration,
} from '../../lib/yamlValidation.js';

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
  'outputs',
  'ignore_failure',
  'agent_role',
  'model',
  'mcp_profiles',
  'governance_level',
  'runtime_pool',
  'knowledge_context',
];

export const TASK_DIRECTIVES = [
  'name',
  'goal',
  'script',
  'depends_on',
  'outputs',
  'ignore_failure',
  'model',
  'mcp_profiles',
  'governance_level',
  'knowledge_context',
  'variables',
];

export type PipelineListItem = {
  id: string;
  source?: string;
  version?: string;
  updatedAt?: string;
};

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
  updatedAt?: string;
};

export type PipelineGraphTaskDefinition = {
  name: string;
  goal?: string;
  script?: string;
  depends_on?: string[];
  ignore_failure?: boolean;
  variables?: Record<string, string>;
  outputs?: TaskOutputDeclaration[];
};

export type PipelineGraphApprovalDefinition = {
  type?: string;
  teams?: string[];
  allow_self_approval?: boolean;
  timeout?: string;
};

export type PipelineGraphStepConfiguration = {
  include?: string;
  sync?: boolean;
  approval?: PipelineGraphApprovalDefinition;
  image?: string;
  runtime_pool?: string;
  secrets?: string[];
  volumes?: string[];
  variables?: Record<string, string>;
  outputs?: TaskOutputDeclaration[];
  ignore_failure?: boolean;
  agent_role?: string;
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

export type PipelineDependencyReference = {
  raw: string;
  identifier: string;
  typeLabel: 'Pipeline' | 'Step' | 'Include';
  actionLabel: 'Open' | 'Copy';
  navigable: boolean;
  kind: 'pipeline' | 'step' | 'include' | 'local-step';
  targetStep?: string;
  sourceStep?: string;
};

export function normalizeRootPath(path: string) {
  const parts = path.trim().replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
  if (isGlobalResourceTeamPath(parts[0])) parts.shift();
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

export function filterVisiblePipelineList(items: PipelineListItem[], searchTerm: string, activeTeam: string): PipelineListItem[] {
  const query = searchTerm.trim().toLowerCase();
  const normalizedTeam = normalizeRootPath(activeTeam);
  const rootTeamSelected = isGlobalResourceTeamPath(activeTeam);
  const filtered = query ? items.filter(item => item.id.toLowerCase().includes(query)) : items;
  const scoped = query
    ? filtered
    : rootTeamSelected
      ? filtered.filter(item => normalizeRootPath(splitIdentifier(item.id).path) === '')
      : !normalizedTeam
        ? filtered
        : filtered.filter(item => pipelineListItemBelongsToTeam(item, normalizedTeam));
  return [...scoped].sort((a, b) => a.id.localeCompare(b.id));
}

function pipelineListItemBelongsToTeam(item: PipelineListItem, normalizedTeam: string): boolean {
  const resourcePath = normalizeRootPath(splitIdentifier(item.id).path);
  return resourcePath === normalizedTeam || resourcePath.startsWith(`${normalizedTeam}/`);
}

export function formatPipelineRelativeTime(value?: string): string {
  if (!value) return 'N/A';
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return value;
  const delta = (Date.now() - timestamp) / 1000;
  if (delta < 60) return 'Just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
  return `${Math.floor(delta / 86400)}d ago`;
}

export function formatPipelineGitRef(ref?: string): string {
  const value = (ref || '').trim();
  return value.replace(/^refs\/heads\//, '').replace(/^refs\/tags\//, '') || '—';
}

export function pipelineRunStatusClass(status?: string): string {
  const normalized = (status || '').toLowerCase();
  if (normalized === 'success' || normalized === 'succeeded') return 'runner-pill--ok';
  if (normalized === 'failure' || normalized === 'failed' || normalized === 'error' || normalized === 'cancelled') {
    return 'runner-pill--error';
  }
  return 'runner-pill--muted';
}

export function pipelineRunStatusLabel(status?: string): string {
  const normalized = (status || '').replace(/_/g, ' ').trim();
  if (!normalized) return 'unknown';
  return normalized.charAt(0).toUpperCase() + normalized.slice(1);
}

export function formatPipelineTriggerEvent(value: unknown): string {
  if (Array.isArray(value)) return value.map(item => String(item)).join(', ');
  if (!value) return 'N/A';
  const raw = String(value).toLowerCase();
  if (raw === 'push') return 'Push';
  if (raw === 'pull_request' || raw === 'pull-request') return 'Pull request';
  if (raw === 'schedule') return 'Schedule';
  return String(value);
}

export function formatPipelineTriggerBranchField(trigger: Record<string, unknown>): { label: string; value: string } {
  const branches = Array.isArray(trigger.branches) ? trigger.branches.map(String).filter(Boolean) : [];
  if (branches.length) return { label: 'branches:', value: branches.join(', ') };
  const skip = Array.isArray(trigger.skip_branches)
    ? trigger.skip_branches.map(String).filter(Boolean)
    : Array.isArray(trigger.skipBranches)
      ? trigger.skipBranches.map(String).filter(Boolean)
      : [];
  if (skip.length) return { label: 'skip_branches:', value: skip.join(', ') };
  return { label: 'branches:', value: 'All branches' };
}

export function formatPipelineTriggerScope(trigger: Record<string, unknown>): string {
  const scope = typeof trigger.scope === 'string' ? trigger.scope.trim() : '';
  return scope || 'default';
}

export function parsePipelineDependencyReference(value: string): PipelineDependencyReference {
  const raw = value.trim();
  const isPipeline = raw.startsWith('pipeline:');
  const isStep = raw.startsWith('step:');
  const identifier = isPipeline
    ? raw.slice('pipeline:'.length).trim()
    : isStep
      ? raw.slice('step:'.length).trim()
      : raw;
  return {
    raw,
    identifier,
    typeLabel: isPipeline ? 'Pipeline' : isStep ? 'Step' : 'Include',
    actionLabel: isPipeline || isStep ? 'Open' : 'Copy',
    navigable: isPipeline || isStep,
    kind: isPipeline ? 'pipeline' : isStep ? 'step' : 'include',
  };
}

export function buildPipelineDependencyReferences(detail: PipelineDetail): PipelineDependencyReference[] {
  const references = new Map<string, PipelineDependencyReference>();
  detail.includedDependencies.forEach(value => {
    const dependency = parsePipelineDependencyReference(value);
    if (dependency.kind !== 'pipeline' && dependency.kind !== 'step') return;
    const key = `${dependency.kind}:${dependency.identifier || dependency.raw}`;
    if (!references.has(key)) {
      references.set(key, dependency);
    }
  });
  const order: Record<PipelineDependencyReference['kind'], number> = {
    pipeline: 0,
    step: 1,
    include: 2,
    'local-step': 3,
  };
  return Array.from(references.values()).sort((a, b) => {
    const byKind = order[a.kind] - order[b.kind];
    if (byKind !== 0) return byKind;
    const byIdentifier = a.identifier.localeCompare(b.identifier);
    if (byIdentifier !== 0) return byIdentifier;
    return a.raw.localeCompare(b.raw);
  });
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

function normalizeOutputs(value: unknown): TaskOutputDeclaration[] | undefined {
  const parsed = parseTaskOutputDeclarations(value, 'outputs');
  return parsed.outputs.length ? parsed.outputs : undefined;
}

function normalizeApproval(value: unknown): PipelineGraphApprovalDefinition | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const record = value as Record<string, unknown>;
  return {
    type: typeof record.type === 'string' ? record.type : undefined,
    teams: normalizeStringArray(record.teams),
    allow_self_approval: typeof record.allow_self_approval === 'boolean' ? record.allow_self_approval : undefined,
    timeout: typeof record.timeout === 'string' ? record.timeout : undefined,
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
    runtime_pool?: string;
    secrets?: string[];
    volumes?: string[];
    variables?: Record<string, string>;
    outputs?: TaskOutputDeclaration[];
    ignore_failure?: boolean;
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
                  outputs: normalizeOutputs(task.outputs),
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
          runtime_pool: typeof step.runtime_pool === 'string' ? step.runtime_pool : undefined,
          secrets: normalizeStringArray(step.secrets),
          volumes: normalizeStringArray(step.volumes),
          variables: normalizeVariables(step.variables),
          outputs: normalizeOutputs(step.outputs),
          ignore_failure: typeof step.ignore_failure === 'boolean' ? step.ignore_failure : undefined,
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
          runtime_pool: step.runtime_pool,
          secrets: step.secrets,
          volumes: step.volumes,
          variables: step.variables,
          outputs: step.outputs,
          ignore_failure: step.ignore_failure,
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

export function parsePipelineYaml(raw: string, id: string, source?: string, updatedAt?: string): PipelineDetail {
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
    updatedAt,
  };
}
