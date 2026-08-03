import { GLOBAL_RESOURCE_TEAM_LABEL, GLOBAL_RESOURCE_TEAM_PATH, isGlobalResourceTeamPath } from '../../lib/resourceTeams.js';

export type AllowedCaller = {
  type: 'user' | 'service_account' | 'auth_team';
  id: string;
};

export type ExternalTrigger = {
  id: string;
  name: string;
  description?: string;
  enabled: boolean;
  pipeline: string;
  scope?: string;
  run_team_path?: string;
  allowed_callers?: AllowedCaller[];
  variable_mapping?: Record<string, string>;
  payload_schema?: Record<string, unknown>;
  rate_limit?: Record<string, unknown>;
  created_by?: string;
  created_at?: string;
  updated_at?: string;
  last_used_at?: string;
  source?: string;
  managed_by_config_repo?: boolean;
  config_source_path?: string;
};

export type ExternalTriggerInvocation = {
  id: string;
  trigger_id: string;
  caller_type: string;
  caller_id: string;
  status: string;
  run_id?: string;
  idempotency_key?: string;
  event_type?: string;
  source_ip?: string;
  created_at?: string;
  error?: string;
};

export type ExternalTriggerForm = {
  id: string;
  name: string;
  description: string;
  pipeline: string;
  scope: string;
  runTeamPath: string;
  enabled: boolean;
  allowedCallers: AllowedCaller[];
  variableMappingText: string;
  payloadSchemaText: string;
  rateLimitPerMinute: string;
};

export type ExternalTriggerModalState = {
  mode: 'create' | 'edit';
  trigger?: ExternalTrigger;
};

export type SelectOption = {
  value: string;
  label: string;
};

export type ExternalTriggerCollectionMetrics = {
  total: number;
  enabled: number;
  gitManaged: number;
  callerPolicies: number;
};

export type ExternalTriggerTreeItem = {
  id: string;
  label: string;
  path: string;
  source?: string;
};

export function externalTriggerSourceLabel(trigger: ExternalTrigger) {
  return trigger.managed_by_config_repo ? 'GitOps' : trigger.source || 'Database';
}

export function buildExternalTriggerCollectionMetrics(
  triggers: readonly ExternalTrigger[]
): ExternalTriggerCollectionMetrics {
  return triggers.reduce<ExternalTriggerCollectionMetrics>(
    (metrics, trigger) => ({
      total: metrics.total + 1,
      enabled: metrics.enabled + (trigger.enabled ? 1 : 0),
      gitManaged: metrics.gitManaged + (trigger.managed_by_config_repo ? 1 : 0),
      callerPolicies: metrics.callerPolicies + ((trigger.allowed_callers || []).length ? 1 : 0),
    }),
    { total: 0, enabled: 0, gitManaged: 0, callerPolicies: 0 }
  );
}

export function filterExternalTriggers(
  triggers: readonly ExternalTrigger[],
  query: string
): ExternalTrigger[] {
  const term = query.trim().toLowerCase();
  if (!term) return [...triggers];
  return triggers.filter(trigger => [
    trigger.id,
    trigger.name,
    trigger.description,
    trigger.pipeline,
    trigger.scope,
    trigger.run_team_path,
    externalTriggerSourceLabel(trigger),
    ...(trigger.allowed_callers || []).flatMap(caller => [caller.type, caller.id]),
  ].join(' ').toLowerCase().includes(term));
}

export function externalTriggerScopeLabel(scope?: string) {
  const normalized = normalizeExternalTriggerIdentifier(scope);
  return normalized.toLowerCase() === 'default' || !normalized ? 'default' : normalized;
}

export function externalTriggerTeamLabel(path?: string) {
  const normalized = externalTriggerTeamPath(path);
  return isGlobalResourceTeamPath(normalized) ? GLOBAL_RESOURCE_TEAM_LABEL : normalized;
}

export function externalTriggerTeamPath(path?: string) {
  const normalized = normalizeExternalTriggerIdentifier(path);
  return normalized || GLOBAL_RESOURCE_TEAM_PATH;
}

export function externalTriggerBelongsToTeam(trigger: ExternalTrigger, activeTeamPath: string) {
  const active = normalizeExternalTriggerIdentifier(activeTeamPath);
  if (!active) return true;
  const teamPath = externalTriggerTeamPath(trigger.run_team_path);
  return teamPath === active || teamPath.startsWith(`${active}/`);
}

export function buildExternalTriggerTreeItems(
  triggers: readonly ExternalTrigger[]
): ExternalTriggerTreeItem[] {
  return triggers
    .map(trigger => ({
      id: trigger.id,
      label: trigger.name || trigger.id,
      path: externalTriggerTeamPath(trigger.run_team_path),
      source: trigger.source,
    }))
    .sort((left, right) => left.label.localeCompare(right.label, undefined, { sensitivity: 'base' }));
}

export function externalTriggerRelativeLabel(value?: string, now = Date.now()) {
  if (!value) return 'Never';
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return 'Never';
  const delta = Math.max(0, Math.floor((now - timestamp) / 1000));
  if (delta < 60) return 'just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
  return `${Math.floor(delta / 86400)}d ago`;
}

function normalizeExternalTriggerIdentifier(value?: string) {
  return String(value || '')
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^(pipelines|external-triggers)\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\/+|\/+$/g, '');
}
