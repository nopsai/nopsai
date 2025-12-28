import yaml from 'js-yaml';

export type YamlParseError = {
  message: string;
  line?: number;
  column?: number;
};

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
