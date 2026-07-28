import * as yaml from 'js-yaml';
import {
  escapeRegExp,
  findLineNumberByRegex,
  findLineNumberForKey,
  parseYamlWithLocation,
} from '../../lib/yamlValidation.js';

export const TRIGGER_ROOT_KEYS = ['provider', 'team', 'team_path', 'webhook_source', 'management', 'triggers'];
export const TRIGGER_KEYS = ['on', 'branches', 'skip_branches', 'tags', 'include_paths', 'exclude_paths', 'pipelines', 'scope'];
export const TRIGGER_EVENT_OPTIONS = ['push', 'pull_request', 'schedule'];
export const TRIGGER_PROVIDERS = ['github', 'gitlab', 'bitbucket', 'gitea', 'generic'] as const;
export const TRIGGER_MANAGEMENT_MODES = ['nopsai', 'repository'] as const;

export type TriggerProvider = (typeof TRIGGER_PROVIDERS)[number];
export type TriggerManagementMode = (typeof TRIGGER_MANAGEMENT_MODES)[number];

export type TriggerListItem = {
  slug: string;
  source?: string;
  provider?: string;
  teamPath?: string;
  management?: string;
  webhookSourceID?: string;
  webhookSourceName?: string;
  ingress?: string;
  allowlistStatus?: string;
  repositoryForWebhook?: string;
  scopes?: string[];
};

export type TriggerSourceFilter = 'all' | 'git' | 'database';

export type TriggerCollectionMetrics = {
  total: number;
  gitManaged: number;
  databaseManaged: number;
  ownerCount: number;
  teamCount: number;
};

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
  provider?: string;
  teamPath?: string;
  management?: string;
  webhookSourceID?: string;
  webhookSourceName?: string;
  ingress?: string;
  allowlistStatus?: string;
  repositoryForWebhook?: string;
  rawYaml: string;
  summary: TriggerSummary;
};

export type TriggerDetailsFormState = {
  provider: TriggerProvider;
  teamPath: string;
  management: TriggerManagementMode;
  webhookSourceID: string;
};

export type TriggerWebhookSourceOption = {
  id: string;
  name?: string;
  provider: string;
  teamPath?: string;
  visibility?: string;
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

export function triggerManagementLabel(value?: string): string {
  switch ((value || '').trim().toLowerCase()) {
    case 'repository':
      return 'Repository';
    default:
      return 'NopsAI';
  }
}

export function triggerTeamLabel(value?: string): string {
  const normalized = String(value || '').trim().replace(/^\/+|\/+$/g, '');
  if (!normalized || normalized.toLowerCase() === 'root') return 'Workspace';
  return normalized;
}

export function triggerAllowlistStatusLabel(value?: string): string {
  switch ((value || '').trim().toLowerCase()) {
    case 'allowed':
      return 'Allowed';
    case 'automatic':
      return 'Automatic';
    case 'not_assigned':
      return 'Webhook source required';
    case 'missing_source':
      return 'Webhook source missing';
    case 'denied':
      return 'Not allowed';
    case 'no_trigger':
      return 'No trigger';
    default:
      return 'Not required';
  }
}

export function triggerScopesLabel(scopes?: readonly string[]): string {
  const normalized = [...new Set((scopes || []).map(normalizeScopeLabel))]
    .map(scope => scope || 'default')
    .filter(Boolean);
  return normalized.length ? normalized.join(', ') : 'default';
}

export function triggerIngressLabel(detail: Pick<TriggerDetail, 'provider' | 'ingress' | 'webhookSourceName' | 'webhookSourceID'>): string {
  const provider = (detail.provider || 'github').trim().toLowerCase();
  if (provider === 'github') return 'GitHub App - automatic';
  return detail.ingress || detail.webhookSourceName || detail.webhookSourceID || 'Not assigned';
}

export function triggerWebhookSourceOptionLabel(option: TriggerWebhookSourceOption): string {
  const name = option.name && option.name !== option.id ? `${option.name} (${option.id})` : option.id;
  const owner = option.visibility === 'workspace' ? 'workspace-shared' : triggerTeamLabel(option.teamPath);
  return `${name} - ${owner}`;
}

export function normalizeTriggerProvider(value?: string): TriggerProvider {
  const normalized = String(value || '').trim().toLowerCase().replace(/[-\s]+/g, '_');
  return TRIGGER_PROVIDERS.includes(normalized as TriggerProvider) ? normalized as TriggerProvider : 'github';
}

export function normalizeTriggerManagement(value?: string): TriggerManagementMode {
  const normalized = String(value || '').trim().toLowerCase().replace(/[-\s]+/g, '_');
  return normalized === 'repository' ? 'repository' : 'nopsai';
}

export function normalizeTriggerTeamPath(value?: string): string {
  const normalized = String(value || '')
    .trim()
    .replace(/^\/+|\/+$/g, '')
    .replace(/\/+/g, '/');
  return normalized && normalized.toLowerCase() !== 'root' ? normalized : 'root';
}

export function triggerDetailsFormFromYaml(rawYaml: string, detail?: Partial<TriggerDetail> | null): TriggerDetailsFormState {
  let parsed: Record<string, unknown> | undefined;
  try {
    parsed = yaml.load(rawYaml || '') as Record<string, unknown> | undefined;
  } catch {
    parsed = undefined;
  }
  const root = asRecord(parsed) || {};
  const provider = normalizeTriggerProvider(
    typeof root.provider === 'string' ? root.provider : detail?.provider
  );
  return {
    provider,
    teamPath: normalizeTriggerTeamPath(
      typeof root.team_path === 'string'
        ? root.team_path
        : typeof root.team === 'string'
          ? root.team
          : detail?.teamPath
    ),
    management: normalizeTriggerManagement(
      typeof root.management === 'string' ? root.management : detail?.management
    ),
    webhookSourceID: provider === 'github'
      ? ''
      : String(
          typeof root.webhook_source === 'string'
            ? root.webhook_source
            : detail?.webhookSourceID || ''
        ).trim(),
  };
}

export function triggerDetailsWithProvider(
  details: TriggerDetailsFormState,
  provider: TriggerProvider
): TriggerDetailsFormState {
  return {
    ...details,
    provider,
    webhookSourceID: provider === details.provider && provider !== 'github' ? details.webhookSourceID : '',
  };
}

export function applyTriggerDetailsToYaml(rawYaml: string, details: TriggerDetailsFormState): string {
  let parsed: Record<string, unknown> | undefined;
  let parseFailed = false;
  try {
    parsed = yaml.load(rawYaml || '') as Record<string, unknown> | undefined;
  } catch {
    parseFailed = true;
    parsed = undefined;
  }
  if (parseFailed && rawYaml.trim()) return rawYaml;
  const root = asRecord(parsed) || {};
  const provider = normalizeTriggerProvider(details.provider);
  const teamPath = normalizeTriggerTeamPath(details.teamPath);
  const management = normalizeTriggerManagement(details.management);
  const nextRoot: Record<string, unknown> = {
    provider,
    team: teamPath,
  };
  if (management !== 'nopsai') nextRoot.management = management;
  const webhookSourceID = String(details.webhookSourceID || '').trim();
  if (provider !== 'github' && webhookSourceID) {
    nextRoot.webhook_source = webhookSourceID;
  }
  Object.entries(root).forEach(([key, value]) => {
    if (TRIGGER_ROOT_KEYS.includes(key)) return;
    nextRoot[key] = value;
  });
  nextRoot.triggers = Array.isArray(root.triggers) && root.triggers.length
    ? root.triggers
    : [{
        on: 'push',
        branches: ['main'],
        pipelines: ['pipelines/sample-pipeline.yaml'],
      }];
  return yaml.dump(nextRoot, {
    lineWidth: -1,
    noRefs: true,
    sortKeys: false,
  });
}

export function filterTriggerListItems(
  items: readonly TriggerListItem[],
  {
    query,
    source,
  }: {
    query: string;
    source: TriggerSourceFilter;
  }
): TriggerListItem[] {
  const normalizedQuery = query.trim().toLowerCase();
  return items.filter(item => {
    const sourceKey = normalizeSource(item.source);
    if (source !== 'all' && sourceKey !== source) return false;
    if (!normalizedQuery) return true;
    return [
      item.slug,
      sourceLabel(sourceKey),
      item.provider,
      item.teamPath,
      item.management,
      item.webhookSourceID,
      item.webhookSourceName,
      item.ingress,
      item.allowlistStatus,
      ...(item.scopes || []),
    ].join(' ').toLowerCase().includes(normalizedQuery);
  });
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
    if (slug) {
      const item: TriggerListItem = {
        slug,
        source: typeof record.source === 'string' ? normalizeSource(record.source) : '',
      };
      if (typeof record.provider === 'string') item.provider = record.provider;
      if (typeof record.team_path === 'string') item.teamPath = record.team_path;
      if (typeof record.management === 'string') item.management = record.management;
      if (typeof record.webhook_source_id === 'string') item.webhookSourceID = record.webhook_source_id;
      if (typeof record.webhook_source_name === 'string') item.webhookSourceName = record.webhook_source_name;
      if (typeof record.ingress === 'string') item.ingress = record.ingress;
      if (typeof record.allowlist_status === 'string') item.allowlistStatus = record.allowlist_status;
      if (typeof record.repository_for_webhook === 'string') item.repositoryForWebhook = record.repository_for_webhook;
      if (Array.isArray(record.scopes)) {
        item.scopes = [...new Set(record.scopes.map(normalizeScopeLabel))];
      }
      items.push(item);
    }
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

export function buildNewTriggerYaml(pipelinePath: string, details?: Partial<TriggerDetailsFormState>): string {
  return applyTriggerDetailsToYaml(
    `triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - ${pipelinePath || 'pipelines/sample-pipeline.yaml'}\n`,
    {
      provider: normalizeTriggerProvider(details?.provider),
      teamPath: normalizeTriggerTeamPath(details?.teamPath),
      management: normalizeTriggerManagement(details?.management),
      webhookSourceID: details?.webhookSourceID || '',
    }
  );
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

export function triggerBelongsToTeam(item: Pick<TriggerListItem, 'teamPath'>, teamPath: string): boolean {
  const rawTeamPath = String(teamPath || '').trim().replace(/^\/+|\/+$/g, '');
  if (!rawTeamPath) return true;
  const activeTeam = normalizeTriggerTeamPath(rawTeamPath);
  const itemTeam = normalizeTriggerTeamPath(item.teamPath);
  if (activeTeam === 'root') return itemTeam === 'root';
  return itemTeam === activeTeam || itemTeam.startsWith(`${activeTeam}/`);
}

export function triggerBelongsToOwner(slug: string, ownerPath: string): boolean {
  const normalizedOwner = ownerPath.trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
  if (!normalizedOwner) return true;
  const path = triggerSlugLabel(slug).path;
  return path === normalizedOwner || path.startsWith(`${normalizedOwner}/`);
}

export function triggerBelongsToOwnerTeam(
  item: Pick<TriggerListItem, 'slug' | 'teamPath'>,
  ownerPath: string,
  teamPath: string
): boolean {
  return triggerBelongsToOwner(item.slug, ownerPath) && triggerBelongsToTeam(item, teamPath);
}

export function buildTriggerCollectionMetrics(
  items: readonly TriggerListItem[],
  ownerPath: string,
  teamPath: string
): TriggerCollectionMetrics {
  const scopedItems = items.filter(item => triggerBelongsToOwnerTeam(item, ownerPath, teamPath));
  const ownerPaths = new Set<string>();
  const teamPaths = new Set<string>();
  let gitManaged = 0;
  let databaseManaged = 0;

  scopedItems.forEach(item => {
    const sourceKey = normalizeSource(item.source);
    if (sourceKey === 'git') gitManaged += 1;
    else databaseManaged += 1;

    const owner = triggerSlugLabel(item.slug).path;
    if (owner !== 'root') ownerPaths.add(owner);
    teamPaths.add(normalizeTriggerTeamPath(item.teamPath));
  });

  return {
    total: scopedItems.length,
    gitManaged,
    databaseManaged,
    ownerCount: ownerPaths.size,
    teamCount: teamPaths.size,
  };
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
