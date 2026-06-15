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
