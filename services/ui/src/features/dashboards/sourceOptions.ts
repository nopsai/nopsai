import * as yaml from 'js-yaml';

export type DashboardPipelineOutputOption = {
  name: string;
  type: string;
  when: string;
  dashboardRef: string;
  sectionKey: string;
  entryKey: string;
  mode: string;
  preset: string;
  ttl: string;
};

export type DashboardPipelineCatalogItem = {
  id: string;
  outputs: DashboardPipelineOutputOption[];
};

export type DashboardEntryOption = {
  value: string;
  label: string;
};

type EntryOptionInput = {
  output?: DashboardPipelineOutputOption;
  outputName?: string;
  currentEntryKey?: string;
  existingEntryKeys?: string[];
};

export function parseDashboardPipelineOutputOptions(rawYaml: string): DashboardPipelineOutputOption[] {
  let parsed: unknown;
  try {
    parsed = yaml.load(rawYaml);
  } catch {
    return [];
  }

  const pipeline = isRecord(parsed) ? parsed : {};
  const output = isRecord(pipeline.output) ? pipeline.output : {};
  const items = Array.isArray(output.items) ? output.items : [];

  return items
    .map(item => {
      if (!isRecord(item)) return null;
      const name = stringValue(item.name).trim();
      const type = stringValue(item.type).trim().toLowerCase();
      if (!name || type !== 'dashboard') return null;
      const dashboard = isRecord(item.dashboard) ? item.dashboard : {};
      return {
        name,
        type,
        when: stringValue(item.when).trim().toLowerCase(),
        dashboardRef: stringValue(dashboard.ref).trim().replace(/^\/+|\/+$/g, ''),
        sectionKey: stringValue(dashboard.section).trim(),
        entryKey: stringValue(dashboard.entry_key).trim(),
        mode: stringValue(dashboard.mode).trim().toLowerCase(),
        preset: stringValue(dashboard.preset).trim().toLowerCase(),
        ttl: stringValue(dashboard.ttl).trim(),
      };
    })
    .filter((item): item is DashboardPipelineOutputOption => Boolean(item));
}

export function buildDashboardEntryOptions({
  output,
  outputName,
  currentEntryKey,
  existingEntryKeys = [],
}: EntryOptionInput): DashboardEntryOption[] {
  const options: DashboardEntryOption[] = [];
  const seen = new Set<string>();
  const fallbackName = (output?.name || outputName || '').trim();

  const add = (value: string, label: string) => {
    const key = value.trim();
    if (seen.has(key)) return;
    seen.add(key);
    options.push({ value, label });
  };

  add('', fallbackName ? `Use output name (${fallbackName})` : 'Use output name');
  if (output?.entryKey) add(output.entryKey, output.entryKey);
  if (currentEntryKey?.trim()) add(currentEntryKey.trim(), currentEntryKey.trim());
  existingEntryKeys
    .map(entry => entry.trim())
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b))
    .forEach(entry => add(entry, entry));

  return options;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}
