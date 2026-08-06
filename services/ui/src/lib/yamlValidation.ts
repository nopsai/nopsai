import * as yaml from 'js-yaml';

export type YamlParseError = {
  message: string;
  line?: number;
  column?: number;
};

export type ScopedRuntimeRef = {
  raw: string;
  scope: string;
  name: string;
  explicitScope: boolean;
};

export type RuntimeOutputRef = {
  stepName: string;
  taskName: string;
  outputName: string;
};

export type TaskOutputDeclaration = {
  name: string;
  sensitive?: boolean;
};

export const RUNTIME_OUTPUT_REFERENCE_PREFIX = '$steps.';
const RUNTIME_REFERENCE_NAME_PATTERN = /^[A-Za-z0-9_.-]+$/;
const TASK_OUTPUT_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function normalizeLineNumber(value: unknown): number | undefined {
  if (typeof value !== 'number' || Number.isNaN(value)) return undefined;
  const normalized = Math.max(1, Math.floor(value));
  return Number.isFinite(normalized) ? normalized : undefined;
}

export function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

export function findLineNumberByRegex(yamlString: string, regex: RegExp): number | undefined {
  const lines = yamlString.split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    if (regex.test(lines[i])) return i + 1;
  }
  return undefined;
}

export function findLineNumberForKey(yamlString: string, key: string): number | undefined {
  if (!key) return undefined;
  const pattern = new RegExp(`^\\s*(?:-\\s*)?${escapeRegExp(key)}\\s*:`, 'i');
  return findLineNumberByRegex(yamlString, pattern);
}

export function findLineNumberForTaskName(yamlString: string, taskName: string): number | undefined {
  if (!taskName) return undefined;
  const pattern = new RegExp(`^\\s*(?:-\\s*)?name:\\s*${escapeRegExp(taskName)}\\b`, 'i');
  return findLineNumberByRegex(yamlString, pattern);
}

export function parseYamlWithLocation(rawYaml: string): { parsed: unknown | null; error?: YamlParseError } {
  try {
    return { parsed: yaml.load(rawYaml) as unknown, error: undefined };
  } catch (error: unknown) {
    const record = error && typeof error === 'object' ? (error as Record<string, unknown>) : null;
    const mark = record?.mark && typeof record.mark === 'object' ? (record.mark as Record<string, unknown>) : null;
    const line = normalizeLineNumber(typeof mark?.line === 'number' ? mark.line + 1 : undefined);
    const column = normalizeLineNumber(typeof mark?.column === 'number' ? mark.column + 1 : undefined);
    const message =
      error instanceof Error ? error.message : typeof record?.message === 'string' ? record.message : 'Unable to parse YAML.';
    return { parsed: null, error: { message, line, column } };
  }
}

export function isPlainYamlObject(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object') return false;
  return !Array.isArray(value);
}

export function isValidRuntimeReferenceName(name: string): boolean {
  return RUNTIME_REFERENCE_NAME_PATTERN.test(name.trim());
}

export function isValidTaskOutputName(name: string): boolean {
  return TASK_OUTPUT_NAME_PATTERN.test(name.trim());
}

export function parseScopedRuntimeRef(raw: string): { ref?: ScopedRuntimeRef; error?: string } {
  const trimmed = raw.trim();
  if (!trimmed) return { error: 'runtime reference is empty' };

  const separator = trimmed.indexOf(':');
  if (separator >= 0) {
    const scopePart = trimmed.slice(0, separator).trim().replace(/^\/+|\/+$/g, '');
    const namePart = trimmed.slice(separator + 1).trim();
    if (!scopePart) return { error: `scope is empty in runtime reference '${trimmed}'` };
    if (!namePart) return { error: `name is empty in runtime reference '${trimmed}'` };
    if (namePart.includes(':')) return { error: `name contains ':' in runtime reference '${trimmed}'` };
    const normalizedScope = scopePart.toLowerCase() === 'default' ? '' : scopePart;
    const scopeError = validateRelativeScopePath(normalizedScope);
    if (scopeError) return { error: scopeError };
    if (!isValidRuntimeReferenceName(namePart)) {
      return { error: `name '${namePart}' must match ^[A-Za-z0-9_.-]+$` };
    }
    return { ref: { raw: trimmed, scope: normalizedScope, name: namePart, explicitScope: true } };
  }

  if (!isValidRuntimeReferenceName(trimmed)) {
    return { error: `name '${trimmed}' must match ^[A-Za-z0-9_.-]+$` };
  }
  return { ref: { raw: trimmed, scope: '', name: trimmed, explicitScope: false } };
}

export function validateRuntimeVariableMap(value: unknown, label: string): string | null {
  if (value == null) return null;
  if (!isPlainYamlObject(value)) return `${label} must be a map of variable names to string values.`;
  for (const [rawName, rawValue] of Object.entries(value)) {
    const name = rawName.trim();
    if (!name) return `${label} contains an empty variable name.`;
    if (!isValidRuntimeReferenceName(name)) return `${label} variable '${rawName}' must match ^[A-Za-z0-9_.-]+$.`;
    if (typeof rawValue !== 'string') return `${label} variable '${rawName}' must be a string.`;
  }
  return null;
}

const RUNTIME_OUTPUT_REFERENCE_SYNTAX = '$steps.<step>.outputs.<name> or $steps.<step>.<task>.outputs.<name>';

export function parseRuntimeOutputRef(raw: string): { found: boolean; ref?: RuntimeOutputRef; error?: string } {
  const value = raw.trim();
  if (!value.startsWith(RUNTIME_OUTPUT_REFERENCE_PREFIX)) return { found: false };

  const body = value.slice(RUNTIME_OUTPUT_REFERENCE_PREFIX.length);
  const outputMarker = '.outputs.';
  const outputIndex = body.lastIndexOf(outputMarker);
  if (outputIndex <= 0 || outputIndex + outputMarker.length >= body.length) {
    return { found: true, error: `runtime output reference must use ${RUNTIME_OUTPUT_REFERENCE_SYNTAX}` };
  }

  const producer = body.slice(0, outputIndex).trim();
  const outputName = body.slice(outputIndex + outputMarker.length).trim();
  const taskIndex = producer.lastIndexOf('.');
  if (taskIndex === producer.length - 1) {
    return { found: true, error: 'runtime output reference must include a producing task' };
  }

  if (taskIndex <= 0) {
    if (!producer) {
      return { found: true, error: 'runtime output reference must include a producing step' };
    }
    if (!outputName) {
      return { found: true, error: 'runtime output reference must include non-empty step and output names' };
    }
    if (!isValidTaskOutputName(outputName)) {
      return { found: true, error: `runtime output name '${outputName}' is invalid` };
    }
    return { found: true, ref: { stepName: producer, taskName: producer, outputName } };
  }

  const stepName = producer.slice(0, taskIndex).trim();
  const taskName = producer.slice(taskIndex + 1).trim();
  if (!stepName || !taskName || !outputName) {
    return { found: true, error: 'runtime output reference must include non-empty step, task and output names' };
  }
  if (!isValidTaskOutputName(outputName)) {
    return { found: true, error: `runtime output name '${outputName}' is invalid` };
  }
  return { found: true, ref: { stepName, taskName, outputName } };
}

export function parseRuntimeOutputRefCandidates(raw: string): { found: boolean; refs?: RuntimeOutputRef[]; error?: string } {
  const parsed = parseRuntimeOutputRef(raw);
  if (parsed.error || !parsed.found || !parsed.ref) return { found: parsed.found, error: parsed.error };

  const refs = [parsed.ref];
  const value = raw.trim();
  const body = value.slice(RUNTIME_OUTPUT_REFERENCE_PREFIX.length);
  const outputMarker = '.outputs.';
  const outputIndex = body.lastIndexOf(outputMarker);
  if (outputIndex <= 0 || outputIndex + outputMarker.length >= body.length) return { found: true, refs };

  const producer = body.slice(0, outputIndex).trim();
  const outputName = body.slice(outputIndex + outputMarker.length).trim();
  if (!producer || !outputName || !isValidTaskOutputName(outputName)) return { found: true, refs };

  const stepLevel = { stepName: producer, taskName: producer, outputName };
  if (producerTaskOutputKey(stepLevel) !== producerTaskOutputKey(parsed.ref)) {
    refs.push(stepLevel);
  }
  return { found: true, refs };
}

function producerTaskOutputKey(ref: RuntimeOutputRef): string {
  return `${ref.stepName}/${ref.taskName}/${ref.outputName}`;
}

export function parseTaskOutputDeclarations(value: unknown, label: string): { outputs: TaskOutputDeclaration[]; error?: string } {
  if (value == null) return { outputs: [] };
  if (!Array.isArray(value)) return { outputs: [], error: `${label} must be a list.` };

  const outputs: TaskOutputDeclaration[] = [];
  const seen = new Set<string>();
  for (let index = 0; index < value.length; index += 1) {
    const item = value[index];
    let name = '';
    let sensitive: boolean | undefined;
    if (typeof item === 'string') {
      name = item.trim();
    } else if (isPlainYamlObject(item)) {
      name = typeof item.name === 'string' ? item.name.trim() : '';
      if (item.sensitive !== undefined) {
        if (typeof item.sensitive !== 'boolean') return { outputs, error: `${label}[${index}].sensitive must be true or false.` };
        sensitive = item.sensitive;
      }
    } else {
      return { outputs, error: `${label}[${index}] must be a string or an object with name.` };
    }
    if (!name) return { outputs, error: `${label}[${index}] is missing its required name.` };
    if (!isValidTaskOutputName(name)) return { outputs, error: `${label}[${index}] name '${name}' must match ^[A-Za-z_][A-Za-z0-9_]*$.` };
    if (seen.has(name)) return { outputs, error: `${label} output '${name}' is declared more than once.` };
    seen.add(name);
    outputs.push({ name, sensitive });
  }
  return { outputs };
}

function validateRelativeScopePath(scope: string): string | null {
  const normalized = scope.trim().replace(/\\/g, '/').replace(/^\/+|\/+$/g, '');
  if (!normalized) return null;
  if (scope.trim().startsWith('/') || scope.trim().startsWith('~')) return `scope '${scope}' must be relative`;
  const segments = normalized.split('/');
  if (segments.some(segment => !segment || segment === '.' || segment === '..')) {
    return `scope '${scope}' contains invalid path segments`;
  }
  return null;
}
