export type AllowedCaller = {
  type: 'user' | 'service_account' | 'auth_group';
  id: string;
};

export type ExternalTrigger = {
  id: string;
  name: string;
  description?: string;
  enabled: boolean;
  pipeline: string;
  scope?: string;
  run_group_path?: string;
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

export type ExternalTriggerForm = {
  id: string;
  name: string;
  description: string;
  pipeline: string;
  scope: string;
  runGroupPath: string;
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

export function externalTriggerScopeLabel(scope?: string) {
  const normalized = normalizeExternalTriggerIdentifier(scope);
  return normalized.toLowerCase() === 'default' || !normalized ? 'default' : normalized;
}

export function externalTriggerGroupLabel(path?: string) {
  const normalized = normalizeExternalTriggerIdentifier(path);
  return normalized === 'root' || !normalized ? 'Root' : normalized;
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
