import * as yaml from 'js-yaml';
import { isGlobalResourceTeamPath } from '../../lib/resourceTeams.js';
import {
  findLineNumberForKey,
  findLineNumberForTaskName,
  parseScopedRuntimeRef,
  parseTaskOutputDeclarations,
  parseYamlWithLocation,
  validateRuntimeVariableMap,
} from '../../lib/yamlValidation.js';

export const STEP_NAME_PATTERN = /^[a-zA-Z0-9_.-]+$/;

export const STEP_DIRECTIVES = [
  'name',
  'description',
  'include',
  'sync',
  'image',
  'secrets',
  'volumes',
  'variables',
  'knowledge_context',
  'tasks',
  'condition',
  'goal',
  'script',
  'depends_on',
  'outputs',
  'ignore_failure',
  'agent_profile',
  'policy_merge_mode',
  'runtime_pool',
  'llm_output_sharing',
  'artifacts',
  'access',
];

export const TASK_DIRECTIVES = [
  'name',
  'goal',
  'script',
  'depends_on',
  'outputs',
  'ignore_failure',
  'llm_output_sharing',
  'policy_merge_mode',
  'variables',
  'knowledge_context',
];

const STEP_ALLOWED_KEYS = new Set(STEP_DIRECTIVES);
const TASK_ALLOWED_KEYS = new Set(TASK_DIRECTIVES);

export type ValidationError = {
  message: string;
  line?: number;
  column?: number;
};

export type ValidationResult = {
  errors: ValidationError[];
};

export type StepDetail = {
  id: string;
  name: string;
  path: string;
  description: string;
  rawYaml: string;
  source?: string;
  updatedAt?: string;
};

export function normalizeRootPath(path: string) {
  const parts = path.trim().replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
  if (isGlobalResourceTeamPath(parts[0])) parts.shift();
  return parts.join('/');
}

export function encodeId(id: string): string {
  return id.split('/').map(encodeURIComponent).join('/');
}

export function splitIdentifier(id: string): { name: string; path: string } {
  const parts = id.split('/').filter(Boolean);
  const name = decodeURIComponent(parts.pop() || '');
  const path = parts.map(decodeURIComponent).join('/');
  return { name, path };
}

export function normalizeSource(raw: unknown): 'git' | 'database' | 'draft' {
  const value = typeof raw === 'string' ? raw.trim().toLowerCase() : '';
  if (!value) return 'database';
  if (value.includes('git')) return 'git';
  if (value.includes('draft')) return 'draft';
  if (value.includes('db') || value.includes('database')) return 'database';
  return 'database';
}

export function filterVisibleStepList<T extends { id: string }>(items: T[], searchTerm: string, activeTeam: string): T[] {
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
        : filtered.filter(item => stepListItemBelongsToTeam(item, normalizedTeam));
  return [...scoped].sort((a, b) => a.id.localeCompare(b.id));
}

function stepListItemBelongsToTeam(item: { id: string }, normalizedTeam: string): boolean {
  const resourcePath = normalizeRootPath(splitIdentifier(item.id).path);
  return resourcePath === normalizedTeam || resourcePath.startsWith(`${normalizedTeam}/`);
}

export function parseStepYaml(
  rawYaml: string,
  id: string,
  source?: string,
  updatedAt?: string
): StepDetail {
  const safe = (value: unknown) => (typeof value === 'string' ? value : '');
  let parsed: Record<string, unknown> | null = null;
  try {
    const loaded = yaml.load(rawYaml) as unknown;
    if (loaded && typeof loaded === 'object' && !Array.isArray(loaded)) {
      parsed = loaded as Record<string, unknown>;
    }
  } catch (error) {
    console.warn('Failed to parse step YAML for metadata', error);
  }
  const { name: fallbackName, path } = splitIdentifier(id);
  return {
    id,
    name: safe(parsed?.name).trim() || fallbackName,
    description: safe(parsed?.description) || 'No description provided.',
    path,
    rawYaml,
    source,
    updatedAt,
  };
}

export function formatUpdatedAt(value?: string): string {
  const raw = (value || '').trim();
  if (!raw) return '—';
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

export function validateStepYaml(rawYaml: string, opts?: { expectedName?: string }): ValidationResult {
  const trimmed = rawYaml.trim();
  if (!trimmed) {
    return { errors: [{ message: 'Step definition cannot be empty.', line: 1 }] };
  }

  const { parsed, error } = parseYamlWithLocation(rawYaml);
  if (error) {
    return { errors: [error] };
  }

  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { errors: [{ message: 'Step YAML must define an object.', line: 1 }] };
  }

  const record = parsed as Record<string, unknown>;
  const unknownKey = Object.keys(record).find(key => !STEP_ALLOWED_KEYS.has(key));
  if (unknownKey) {
    return {
      errors: [
        {
          message: `Unknown step directive '${unknownKey}'.`,
          line: findLineNumberForKey(rawYaml, unknownKey) ?? 1,
        },
      ],
    };
  }

  const name = typeof record.name === 'string' ? record.name.trim() : '';
  if (!name) {
    return {
      errors: [
        {
          message: "Step YAML must include a 'name' field.",
          line: findLineNumberForKey(rawYaml, 'name') ?? 1,
        },
      ],
    };
  }

  if (!STEP_NAME_PATTERN.test(name)) {
    return {
      errors: [
        {
          message: 'Step name can only contain letters, numbers, dots, underscores, and hyphens.',
          line: findLineNumberForKey(rawYaml, 'name') ?? 1,
        },
      ],
    };
  }

  const expectedName = (opts?.expectedName || '').trim();
  if (expectedName && expectedName !== name) {
    return {
      errors: [
        {
          message: `Step name in YAML ('${name}') must match the identifier name ('${expectedName}').`,
          line: findLineNumberForKey(rawYaml, 'name') ?? 1,
        },
      ],
    };
  }

  const variablesError = validateRuntimeVariableMap(record.variables, `Step '${name}' variables`);
  if (variablesError) {
    return {
      errors: [
        {
          message: variablesError,
          line: findLineNumberForKey(rawYaml, 'variables') ?? 1,
        },
      ],
    };
  }

  if (record.secrets != null) {
    if (!Array.isArray(record.secrets)) {
      return { errors: [{ message: `Step '${name}' secrets must be a list.`, line: findLineNumberForKey(rawYaml, 'secrets') ?? 1 }] };
    }
    for (let idx = 0; idx < record.secrets.length; idx += 1) {
      const rawSecret = record.secrets[idx];
      if (typeof rawSecret !== 'string' || !rawSecret.trim()) {
        return { errors: [{ message: `Step '${name}' secret #${idx + 1} must be a non-empty string.`, line: findLineNumberForKey(rawYaml, 'secrets') ?? 1 }] };
      }
      const parsedSecret = parseScopedRuntimeRef(rawSecret);
      if (parsedSecret.error) {
        return { errors: [{ message: `Step '${name}' secret '${rawSecret}' is invalid: ${parsedSecret.error}.`, line: findLineNumberForKey(rawYaml, 'secrets') ?? 1 }] };
      }
    }
  }

  const hasInclude = record.include != null;
  const hasTasks = record.tasks != null;
  const hasGoal = record.goal != null;
  const hasScript = record.script != null;

  const stepOutputs = parseTaskOutputDeclarations(record.outputs, `Step '${name}' outputs`);
  if (stepOutputs.error) {
    return {
      errors: [
        {
          message: stepOutputs.error,
          line: findLineNumberForKey(rawYaml, 'outputs') ?? 1,
        },
      ],
    };
  }

  const modeCount = [hasInclude, hasTasks, hasGoal, hasScript].filter(Boolean).length;
  const lineForMode =
    findLineNumberForKey(rawYaml, 'include') ??
    findLineNumberForKey(rawYaml, 'tasks') ??
    findLineNumberForKey(rawYaml, 'goal') ??
    findLineNumberForKey(rawYaml, 'script') ??
    1;

  if (modeCount === 0) {
    return {
      errors: [{ message: "Step must define one of 'include', 'tasks', 'goal', or 'script'.", line: lineForMode }],
    };
  }
  if (modeCount > 1) {
    return {
      errors: [{ message: "Step may only define one of 'include', 'tasks', 'goal', or 'script'.", line: lineForMode }],
    };
  }

  if (hasInclude) {
    const includeValue = typeof record.include === 'string' ? record.include.trim() : '';
    if (!includeValue) {
      return {
        errors: [
          {
            message: "Include steps must provide a non-empty 'include' value.",
            line: findLineNumberForKey(rawYaml, 'include') ?? 1,
          },
        ],
      };
    }
    if (!includeValue.startsWith('step:')) {
      return {
        errors: [
          {
            message: "Include steps must reference a reusable step using the 'step:' prefix.",
            line: findLineNumberForKey(rawYaml, 'include') ?? 1,
          },
        ],
      };
    }
    if (stepOutputs.outputs.length > 0) {
      return {
        errors: [
          {
            message: `Include step '${name}' cannot declare task outputs.`,
            line: findLineNumberForKey(rawYaml, 'outputs') ?? 1,
          },
        ],
      };
    }
    return { errors: [] };
  }

  if (hasTasks) {
    const tasks = Array.isArray(record.tasks) ? record.tasks : null;
    const tasksLine = findLineNumberForKey(rawYaml, 'tasks') ?? 1;
    if (!tasks || tasks.length === 0) {
      return { errors: [{ message: "Step 'tasks' must be a non-empty list.", line: tasksLine }] };
    }
    if (stepOutputs.outputs.length > 0) {
      return {
        errors: [
          {
            message: `Step '${name}' has tasks, so outputs must be declared on individual tasks.`,
            line: findLineNumberForKey(rawYaml, 'outputs') ?? tasksLine,
          },
        ],
      };
    }

    const taskNames = new Map<string, string>();

    for (let idx = 0; idx < tasks.length; idx += 1) {
      const taskValue = tasks[idx];
      if (!taskValue || typeof taskValue !== 'object' || Array.isArray(taskValue)) {
        return { errors: [{ message: `Task #${idx + 1} must be an object.`, line: tasksLine }] };
      }
      const taskObj = taskValue as Record<string, unknown>;
      const taskName = typeof taskObj.name === 'string' ? taskObj.name.trim() : '';
      if (!taskName) {
        return { errors: [{ message: `Task #${idx + 1} is missing the required 'name' field.`, line: tasksLine }] };
      }
      const nameKey = taskName.toLowerCase();
      if (taskNames.has(nameKey)) {
        return {
          errors: [
            {
              message: `Duplicate task name '${taskName}' found. Task names must be unique within a step.`,
              line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
            },
          ],
        };
      }
      taskNames.set(nameKey, taskName);

      const invalidTaskKey = Object.keys(taskObj).find(key => !TASK_ALLOWED_KEYS.has(key));
      if (invalidTaskKey) {
        return {
          errors: [
            {
              message: `Task '${taskName}' contains unknown directive '${invalidTaskKey}'.`,
              line: findLineNumberForKey(rawYaml, invalidTaskKey) ?? findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
            },
          ],
        };
      }

      const taskVariablesError = validateRuntimeVariableMap(taskObj.variables, `Task '${taskName}' variables`);
      if (taskVariablesError) {
        return {
          errors: [
            {
              message: taskVariablesError,
              line: findLineNumberForKey(rawYaml, 'variables') ?? findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
            },
          ],
        };
      }

      const taskOutputs = parseTaskOutputDeclarations(taskObj.outputs, `Task '${taskName}' outputs`);
      if (taskOutputs.error) {
        return {
          errors: [
            {
              message: taskOutputs.error,
              line: findLineNumberForKey(rawYaml, 'outputs') ?? findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
            },
          ],
        };
      }

      const taskGoal = typeof taskObj.goal === 'string' ? taskObj.goal.trim() : '';
      const taskScript = typeof taskObj.script === 'string' ? taskObj.script.trim() : '';

      if (taskGoal && taskScript) {
        return { errors: [{ message: `Task '${taskName}' cannot define both 'goal' and 'script'.`, line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine }] };
      }
      if (!taskGoal && !taskScript) {
        return { errors: [{ message: `Task '${taskName}' must define either 'goal' or 'script'.`, line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine }] };
      }
    }

    for (const taskValue of tasks) {
      const taskObj = taskValue as Record<string, unknown>;
      const taskName = typeof taskObj.name === 'string' ? taskObj.name.trim() : '';
      const deps = Array.isArray(taskObj.depends_on) ? taskObj.depends_on : [];
      for (const dep of deps) {
        const depKey = typeof dep === 'string' ? dep.trim().toLowerCase() : '';
        if (!depKey) {
          return {
            errors: [
              {
                message: 'Task dependency names must be non-empty strings.',
                line: findLineNumberForKey(rawYaml, 'depends_on') ?? findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
              },
            ],
          };
        }
        if (!taskNames.has(depKey)) {
          return {
            errors: [
              {
                message: `Task '${taskName || 'unknown'}' depends on undefined task '${String(dep)}'.`,
                line: findLineNumberForTaskName(rawYaml, taskName) ?? tasksLine,
              },
            ],
          };
        }
      }
    }

    return { errors: [] };
  }

  if (hasGoal) {
    const goalValue = typeof record.goal === 'string' ? record.goal.trim() : '';
    if (!goalValue) {
      return { errors: [{ message: "Step 'goal' must be a non-empty string.", line: findLineNumberForKey(rawYaml, 'goal') ?? 1 }] };
    }
    return { errors: [] };
  }

  if (hasScript) {
    const scriptValue = typeof record.script === 'string' ? record.script.trim() : '';
    if (!scriptValue) {
      return { errors: [{ message: "Step 'script' must be a non-empty string.", line: findLineNumberForKey(rawYaml, 'script') ?? 1 }] };
    }
    return { errors: [] };
  }

  return { errors: [] };
}
