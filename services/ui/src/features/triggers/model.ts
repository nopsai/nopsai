import yaml from 'js-yaml';
import {
  escapeRegExp,
  findLineNumberByRegex,
  findLineNumberForKey,
  parseYamlWithLocation,
} from '../../lib/yamlValidation.js';

export const TRIGGER_ROOT_KEYS = ['triggers'];
export const TRIGGER_KEYS = ['on', 'branches', 'skip_branches', 'tags', 'pipelines', 'scope'];
export const TRIGGER_EVENT_OPTIONS = ['push', 'pull_request', 'schedule'];

export type TriggerListItem = { slug: string; source?: string };

export type PipelineRef = {
  identifier: string;
  display: string;
  pathLabel: string;
};

export type TriggerSummary = {
  triggerCount: number;
  pipelines: PipelineRef[];
  events: string[];
  branches: string[];
  skipBranches: string[];
  tags: string[];
  scopes: string[];
};

export type TriggerDetail = {
  slug: string;
  source?: string;
  rawYaml: string;
  summary: TriggerSummary;
};

export type PipelineMeta = {
  version: string;
  sourceKey: string;
  sourceLabel: string;
};

export type TriggerRun = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  status?: string;
  git_repo_owner?: string;
  git_repo_name?: string;
  git_ref?: string;
  started_at?: string;
  trigger_event_id?: string;
};

export type TriggerValidationError = {
  message: string;
  line?: number;
  column?: number;
};

export type TriggerValidationResult = {
  errors: TriggerValidationError[];
};

export function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

export function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value).trim().replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

export function normalizeSource(source?: string): string {
  const key = (source || '').trim().toLowerCase();
  if (!key) return 'database';
  if (key.includes('git')) return 'git';
  if (key.includes('draft')) return 'draft';
  if (key.includes('db') || key.includes('database')) return 'database';
  if (key.includes('local')) return 'local';
  return key;
}

export function sourceLabel(sourceKey: string): string {
  switch (normalizeSource(sourceKey)) {
    case 'git':
      return 'Git';
    case 'draft':
      return 'Draft';
    case 'local':
      return 'Local';
    default:
      return 'Database';
  }
}

export function normalizePipelineIdentifier(value: unknown): string {
  if (!value) return '';
  return String(value)
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^pipelines\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\//, '');
}

export function describePipeline(identifier: string): PipelineRef {
  const segments = identifier.split('/').filter(Boolean);
  const name = segments.pop() || identifier;
  return { identifier, display: name, pathLabel: segments.join('/') || 'root' };
}

export function parseTriggerYaml(raw: string): Record<string, unknown> {
  const parsed = yaml.load(raw) as Record<string, unknown> | undefined;
  if (!parsed || typeof parsed !== 'object') throw new Error('Manifest must be a YAML object.');
  const triggers = parsed.triggers;
  if (!Array.isArray(triggers) || triggers.length === 0) {
    throw new Error('Manifest must contain a non-empty "triggers" array.');
  }
  triggers.forEach((trigger, index) => {
    if (!asRecord(trigger)) throw new Error(`Trigger #${index + 1} must be an object.`);
  });
  return parsed;
}

export function buildTriggerSummary(manifest: Record<string, unknown>): TriggerSummary {
  const triggers = Array.isArray(manifest.triggers) ? manifest.triggers : [];
  const pipelineIdentifiers: PipelineRef[] = [];
  const events = new Set<string>();
  const branches = new Set<string>();
  const skipBranches = new Set<string>();
  const tags = new Set<string>();
  const scopes = new Set<string>();
  let hasDefaultScope = false;

  triggers.forEach(value => {
    const trigger = asRecord(value);
    if (!trigger) return;
    if (trigger.on != null) events.add(String(trigger.on).trim());
    (Array.isArray(trigger.branches) ? trigger.branches : []).forEach(branch => {
      const normalized = String(branch || '').trim();
      if (normalized) branches.add(normalized);
    });
    const skipValue = trigger.skip_branches ?? trigger.skipBranches;
    (Array.isArray(skipValue) ? skipValue : []).forEach(branch => {
      const normalized = String(branch || '').trim();
      if (normalized) skipBranches.add(normalized);
    });
    (Array.isArray(trigger.tags) ? trigger.tags : []).forEach(tag => {
      const normalized = String(tag || '').trim();
      if (normalized) tags.add(normalized);
    });
    const scope = normalizeScopeLabel(trigger.scope);
    if (scope) scopes.add(scope);
    else hasDefaultScope = true;
    (Array.isArray(trigger.pipelines) ? trigger.pipelines : []).forEach(entry => {
      const entryRecord = asRecord(entry);
      const normalized = normalizePipelineIdentifier(
        typeof entry === 'string' ? entry : typeof entryRecord?.path === 'string' ? entryRecord.path : ''
      );
      if (normalized) pipelineIdentifiers.push(describePipeline(normalized));
    });
  });
  if (hasDefaultScope) scopes.add('');

  return {
    triggerCount: triggers.length,
    pipelines: Array.from(new Map(pipelineIdentifiers.map(item => [item.identifier, item])).values()).sort((a, b) =>
      a.identifier.localeCompare(b.identifier)
    ),
    events: Array.from(events).sort(),
    branches: Array.from(branches).sort(),
    skipBranches: Array.from(skipBranches).sort(),
    tags: Array.from(tags).sort(),
    scopes: Array.from(scopes).sort(),
  };
}

export function parseTriggerOverrideList(payload: unknown): TriggerListItem[] {
  if (!Array.isArray(payload)) return [];
  const items: TriggerListItem[] = [];
  payload.forEach(entry => {
    if (typeof entry === 'string') {
      if (entry.trim()) items.push({ slug: entry.trim(), source: 'database' });
      return;
    }
    const record = asRecord(entry);
    if (!record) return;
    const slug = String(record.name || record.repository_name || record.slug || record.repo || '').trim();
    if (slug) items.push({ slug, source: typeof record.source === 'string' ? normalizeSource(record.source) : '' });
  });
  return items.sort((a, b) => a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' }));
}

export function buildPipelineIdentifierFromRun(run: TriggerRun): string {
  return normalizePipelineIdentifier(run.pipeline_path ? `${run.pipeline_path}/${run.pipeline_name}` : run.pipeline_name);
}

export function deriveDefaultPipelinePath(repoSlug: string): string {
  const candidate = String(repoSlug || '').split('/').filter(Boolean).pop() || '';
  const sanitized = candidate.trim().replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, '') || 'sample-pipeline';
  return `pipelines/${sanitized}.yaml`;
}

export function buildNewTriggerYaml(pipelinePath: string): string {
  return `triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - ${pipelinePath || 'pipelines/sample-pipeline.yaml'}\n`;
}

export function splitTriggerSlug(slug: string) {
  const parts = slug.split('/').filter(Boolean);
  if (parts.length < 2) throw new Error('Repository must be in owner/name format.');
  const repo = parts.pop() || '';
  const owner = parts.join('/');
  if (!owner || !repo) throw new Error('Repository must be in owner/name format.');
  return { owner, repo };
}

export function encodeTriggerSlug(slug: string): string {
  return slug.split('/').map(encodeURIComponent).join('/');
}

export function triggerSlugLabel(slug: string): { name: string; path: string } {
  const parts = slug.split('/').filter(Boolean);
  const name = parts.pop() || slug;
  return { name, path: parts.join('/') || 'root' };
}

export function validateTriggerYaml(rawYaml: string): TriggerValidationResult {
  const trimmed = rawYaml.trim();
  if (!trimmed) {
    return { errors: [{ message: 'Trigger manifest cannot be empty.', line: 1 }] };
  }

  const { parsed, error } = parseYamlWithLocation(rawYaml);
  if (error) return { errors: [error] };
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { errors: [{ message: 'YAML must define an object at the root.', line: 1 }] };
  }

  const root = asRecord(parsed);
  if (!root) {
    return { errors: [{ message: 'YAML must define an object at the root.', line: 1 }] };
  }

  const errors: TriggerValidationError[] = [];
  if (root.steps) {
    errors.push({
      message:
        "Validation failed: The provided file appears to be a pipeline, not a trigger manifest. A trigger must contain 'triggers', not 'steps'.",
      line: findLineNumberForKey(rawYaml, 'steps') ?? 1,
    });
  }

  const unknownRootKey = Object.keys(root).find(key => !TRIGGER_ROOT_KEYS.includes(key));
  if (unknownRootKey) {
    errors.push({
      message: `Unknown directive '${unknownRootKey}' at root.`,
      line: findLineNumberForKey(rawYaml, unknownRootKey) ?? 1,
    });
  }

  const triggers = Array.isArray(root.triggers) ? root.triggers : [];
  if (triggers.length === 0) {
    errors.push({ message: "'triggers' must be a non-empty list.", line: findLineNumberForKey(rawYaml, 'triggers') ?? 1 });
    return { errors };
  }

  triggers.forEach((trigger, index) => {
    const triggerRecord = asRecord(trigger);
    const triggerLine =
      findLineNumberByRegex(
        rawYaml,
        new RegExp(
          `^\\s*-\\s*on:\\s*${escapeRegExp(typeof triggerRecord?.on === 'string' ? triggerRecord.on.trim() : '')}\\b`,
          'i'
        )
      ) ?? findLineNumberForKey(rawYaml, 'triggers') ?? 1;

    if (!triggerRecord) {
      errors.push({ message: `Trigger #${index + 1} must be an object.`, line: triggerLine });
      return;
    }

    const unknownKey = Object.keys(triggerRecord).find(key => !TRIGGER_KEYS.includes(key) && key !== 'skipBranches');
    if (unknownKey) {
      errors.push({
        message: `Trigger #${index + 1} contains unknown directive '${unknownKey}'.`,
        line: findLineNumberForKey(rawYaml, unknownKey) ?? triggerLine,
      });
    }

    const onValue = typeof triggerRecord.on === 'string' ? triggerRecord.on.trim() : '';
    if (!onValue) {
      errors.push({
        message: `Trigger #${index + 1} is missing required 'on' event.`,
        line: findLineNumberForKey(rawYaml, 'on') ?? triggerLine,
      });
    } else if (!TRIGGER_EVENT_OPTIONS.includes(onValue)) {
      errors.push({
        message: `Trigger #${index + 1} has unsupported event '${onValue}'.`,
        line:
          findLineNumberByRegex(rawYaml, new RegExp(`^\\s*(?:-\\s*)?on:\\s*${escapeRegExp(onValue)}\\b`, 'i')) ??
          triggerLine,
      });
    }

    const pipelines = Array.isArray(triggerRecord.pipelines) ? triggerRecord.pipelines : [];
    if (pipelines.length === 0) {
      errors.push({
        message: `Trigger #${index + 1} must include at least one pipeline reference.`,
        line: findLineNumberForKey(rawYaml, 'pipelines') ?? triggerLine,
      });
    } else {
      pipelines.forEach((entry, pipelineIndex) => {
        const entryRecord = asRecord(entry);
        const path =
          typeof entry === 'string'
            ? entry.trim()
            : typeof entryRecord?.path === 'string'
              ? entryRecord.path.trim()
              : '';
        if (!path) {
          errors.push({
            message: `Trigger #${index + 1} pipeline #${pipelineIndex + 1} is missing a path.`,
            line: findLineNumberForKey(rawYaml, 'pipelines') ?? triggerLine,
          });
        }
      });
    }

    const validateEntries = (value: unknown, key: 'branches' | 'skip_branches' | 'tags') => {
      const entries = Array.isArray(value) ? value : [];
      entries.forEach((entry, entryIndex) => {
        const normalized = typeof entry === 'string' ? entry.trim() : '';
        if (!normalized) {
          errors.push({
            message: `Trigger #${index + 1} has an empty ${key} entry at position ${entryIndex + 1}.`,
            line: findLineNumberForKey(rawYaml, key) ?? triggerLine,
          });
        }
      });
    };

    validateEntries(triggerRecord.branches, 'branches');
    validateEntries(triggerRecord.skip_branches ?? triggerRecord.skipBranches, 'skip_branches');
    validateEntries(triggerRecord.tags, 'tags');

    if (triggerRecord.scope != null) {
      const scopeValue = typeof triggerRecord.scope === 'string' ? triggerRecord.scope.trim() : '';
      if (!scopeValue) {
        errors.push({
          message: `Trigger #${index + 1} has an empty 'scope'.`,
          line: findLineNumberForKey(rawYaml, 'scope') ?? triggerLine,
        });
      }
    }
  });

  return { errors };
}
