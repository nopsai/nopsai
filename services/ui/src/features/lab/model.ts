import * as yaml from 'js-yaml';

export type LabIncludedDependencies = {
  status: 'ok' | 'invalid' | 'no-steps' | 'parse-error';
  items: string[];
};

export function parseLabIncludedDependencies(rawYaml: string): LabIncludedDependencies {
  try {
    const parsed = yaml.load(rawYaml) as unknown;
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { status: 'invalid', items: [] };
    }

    const steps = Array.isArray((parsed as Record<string, unknown>).steps)
      ? ((parsed as Record<string, unknown>).steps as unknown[])
      : [];
    if (steps.length === 0) return { status: 'no-steps', items: [] };

    const includes = new Set<string>();
    steps.forEach(step => {
      if (!step || typeof step !== 'object' || Array.isArray(step)) return;
      const include = (step as Record<string, unknown>).include;
      if (typeof include === 'string' && include.trim()) includes.add(include.trim());
    });

    return { status: 'ok', items: Array.from(includes).sort((a, b) => a.localeCompare(b)) };
  } catch {
    return { status: 'parse-error', items: [] };
  }
}
