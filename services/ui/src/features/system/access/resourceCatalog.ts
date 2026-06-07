import { asRecord, readString } from '../data.js';

export type AccessResourceOption = {
  value: string;
  label: string;
};

export type AccessResourceCatalog = {
  folderOptions: AccessResourceOption[];
  pipelineOptions: AccessResourceOption[];
  scopeOptions: AccessResourceOption[];
  triggerOptions: AccessResourceOption[];
  externalTriggerOptions: AccessResourceOption[];
  repositoryOptions: AccessResourceOption[];
  secretScopeOptions: AccessResourceOption[];
  variableScopeOptions: AccessResourceOption[];
};

export type AccessResourceCatalogSources = {
  groups: unknown[];
  pipelines: unknown[];
  triggers: unknown[];
  externalTriggers: unknown[];
  secretScopes: unknown[];
  variableScopes: unknown[];
};

type ResourceGroup = {
  id: number;
  name: string;
  parent_id?: number | null;
};

const DEFAULT_SCOPE_VALUE = '__default_scope__';

export function createEmptyAccessResourceCatalog(): AccessResourceCatalog {
  return {
    folderOptions: [],
    pipelineOptions: [],
    scopeOptions: [],
    triggerOptions: [],
    externalTriggerOptions: [],
    repositoryOptions: [],
    secretScopeOptions: [],
    variableScopeOptions: [],
  };
}

export function buildAccessResourceCatalog(sources: AccessResourceCatalogSources): AccessResourceCatalog {
  const groups = sources.groups.map(normalizeResourceGroup).filter(Boolean) as ResourceGroup[];
  const pipelines = normalizeStringValues(sources.pipelines);
  const triggers = normalizeStringValues(sources.triggers);
  const externalTriggers = sources.externalTriggers
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
    folderOptions: buildGroupOptions(groups),
    pipelineOptions: buildStringOptions(pipelines),
    scopeOptions: buildStringOptions([...secretScopes, ...variableScopes]),
    triggerOptions,
    externalTriggerOptions: buildStringOptions(externalTriggers),
    repositoryOptions: triggerOptions,
    secretScopeOptions: namedScopeOptions,
    variableScopeOptions: namedScopeOptions,
  };
}

function normalizeResourceGroup(value: unknown): ResourceGroup | null {
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

function buildGroupOptions(groups: ResourceGroup[]): AccessResourceOption[] {
  const byID = new Map(groups.map(group => [group.id, group]));

  const buildPath = (group: ResourceGroup, trail = new Set<number>()): string => {
    if (trail.has(group.id)) return "";
    const nextTrail = new Set(trail);
    nextTrail.add(group.id);
    const parentID = group.parent_id ?? null;
    if (parentID == null) return group.name;
    const parent = byID.get(parentID);
    if (!parent) return group.name;
    const parentPath = buildPath(parent, nextTrail);
    return parentPath ? `${parentPath}/${group.name}` : group.name;
  };

  return dedupeOptions(groups.map(group => {
    const path = buildPath(group);
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
