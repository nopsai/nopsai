import { asRecord, readString } from '../data.js';

export type AccessResourceOption = {
  value: string;
  label: string;
};

export type AccessResourceCatalog = {
  teamOptions: AccessResourceOption[];
  pipelineOptions: AccessResourceOption[];
  scopeOptions: AccessResourceOption[];
  triggerOptions: AccessResourceOption[];
  externalTriggerOptions: AccessResourceOption[];
  gitWebhookSourceOptions: AccessResourceOption[];
  repositoryOptions: AccessResourceOption[];
  secretScopeOptions: AccessResourceOption[];
  variableScopeOptions: AccessResourceOption[];
};

export type AccessResourceCatalogSources = {
  teams: unknown[];
  pipelines: unknown[];
  triggers: unknown[];
  externalTriggers: unknown[];
  gitWebhookSources: unknown[];
  secretScopes: unknown[];
  variableScopes: unknown[];
};

type ResourceTeam = {
  id: number;
  name: string;
  parent_id?: number | null;
};

const DEFAULT_SCOPE_VALUE = '__default_scope__';

export function createEmptyAccessResourceCatalog(): AccessResourceCatalog {
  return {
    teamOptions: [],
    pipelineOptions: [],
    scopeOptions: [],
    triggerOptions: [],
    externalTriggerOptions: [],
    gitWebhookSourceOptions: [],
    repositoryOptions: [],
    secretScopeOptions: [],
    variableScopeOptions: [],
  };
}

export function buildAccessResourceCatalog(sources: AccessResourceCatalogSources): AccessResourceCatalog {
  const teams = sources.teams.map(normalizeResourceTeam).filter(Boolean) as ResourceTeam[];
  const pipelines = normalizeStringValues(sources.pipelines);
  const triggers = normalizeStringValues(sources.triggers);
  const externalTriggers = sources.externalTriggers
    .map(entry => {
      const record = asRecord(entry);
      return record ? readString(record.id || record.name).trim() : '';
    })
    .filter(Boolean);
  const gitWebhookSources = sources.gitWebhookSources
    .map(entry => {
      const record = asRecord(entry);
      return record ? readString(record.id || record.name).trim() : '';
    })
    .filter(Boolean);
  const secretScopes = normalizeScopeValues(sources.secretScopes);
  const variableScopes = normalizeScopeValues(sources.variableScopes);
  const triggerOptions = buildStringOptions(triggers);
  const namedScopeOptions = buildNamedScopeOptions([...secretScopes, ...variableScopes]);

  return {
    teamOptions: buildTeamOptions(teams),
    pipelineOptions: buildStringOptions(pipelines),
    scopeOptions: buildStringOptions([...secretScopes, ...variableScopes]),
    triggerOptions,
    externalTriggerOptions: buildStringOptions(externalTriggers),
    gitWebhookSourceOptions: buildStringOptions(gitWebhookSources),
    repositoryOptions: triggerOptions,
    secretScopeOptions: namedScopeOptions,
    variableScopeOptions: namedScopeOptions,
  };
}

function normalizeResourceTeam(value: unknown): ResourceTeam | null {
  const record = asRecord(value);
  if (!record) return null;
  const id = Number(record.id);
  const name = readString(record.name).trim();
  if (!Number.isFinite(id) || !name) return null;
  const parentID = record.parent_id == null ? null : Number(record.parent_id);
  return {
    id,
    name,
    parent_id: Number.isFinite(parentID) ? parentID : null,
  };
}

function normalizeStringValues(values: unknown[]): string[] {
  return values.map(value => readString(value).trim()).filter(Boolean);
}

function normalizeScopeValues(values: unknown[]): string[] {
  return values
    .map(value => {
      const record = asRecord(value);
      return record ? readString(record.scope).trim() : '';
    })
    .filter(Boolean);
}

function dedupeOptions(options: AccessResourceOption[]): AccessResourceOption[] {
  const seen = new Set<string>();
  return options.filter(option => {
    const value = option.value.trim();
    if (!value || seen.has(value)) return false;
    seen.add(value);
    return true;
  });
}

function buildTeamOptions(teams: ResourceTeam[]): AccessResourceOption[] {
  const byID = new Map(teams.map(team => [team.id, team]));

  const buildPath = (team: ResourceTeam, trail = new Set<number>()): string => {
    if (trail.has(team.id)) return "";
    const nextTrail = new Set(trail);
    nextTrail.add(team.id);
    const parentID = team.parent_id ?? null;
    if (parentID == null) return team.name;
    const parent = byID.get(parentID);
    if (!parent) return team.name;
    const parentPath = buildPath(parent, nextTrail);
    return parentPath ? `${parentPath}/${team.name}` : team.name;
  };

  return dedupeOptions(teams.map(team => {
    const path = buildPath(team);
    return { value: path, label: `/${path}` };
  })).sort((a, b) => a.value.localeCompare(b.value));
}

function buildStringOptions(values: string[]): AccessResourceOption[] {
  return dedupeOptions(
    values
      .map(value => value.trim())
      .filter(Boolean)
      .map(value => ({ value, label: value }))
  ).sort((a, b) => a.value.localeCompare(b.value));
}

function normalizeScopeOptionValue(scope: string): string {
  const normalized = scope.trim().replace(/^\/+|\/+$/g, '');
  return !normalized || normalized.toLowerCase() === 'default' ? DEFAULT_SCOPE_VALUE : normalized;
}

function buildNamedScopeOptions(values: string[]): AccessResourceOption[] {
  return dedupeOptions(
    ['', ...values].map(value => {
      const normalized = value.trim().replace(/^\/+|\/+$/g, '');
      const isDefault = !normalized || normalized.toLowerCase() === 'default';
      return {
        value: normalizeScopeOptionValue(normalized),
        label: isDefault ? 'Default scope' : normalized,
      };
    })
  ).sort((a, b) => {
    if (a.value === DEFAULT_SCOPE_VALUE) return -1;
    if (b.value === DEFAULT_SCOPE_VALUE) return 1;
    return a.label.localeCompare(b.label, undefined, { sensitivity: 'base' });
  });
}
