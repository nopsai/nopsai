import { asRecord, normalizeNumber, readOptionalString, readString } from '../data.js';

export type ConfigFormState = {
  log_level: string;
  log_format: string;
  environment: string;
  public_url: string;
  notification_mail_logo_url: string;
  notification_mail_website_url: string;
  notification_mail_support_url: string;
  notification_mail_footer_address: string;
  require_production_gates: boolean;
  agent_image: string;
  docker_network_name: string;
  default_pipeline_timeout: string;
  llm_agent_timeout: string;
  auto_removal_agent_container: boolean;
  agent_nopsai_api_url: string;
  git_bot_nopsai_api_url: string;
  nopsai_git_bot_api_url: string;
  github_app_id: string;
  github_installation_id: string;
  github_private_key_credential_ref: string;
  github_webhook_credential_ref: string;
  dispatcher_address: string;
  dispatcher_routing: Record<string, string[]>;
  runner_id: string;
  runner_scopes: string;
  runner_capacity: string;
};

export type ConfigFieldMetadata = {
  scope: string;
  label: string;
  section: string;
  apply: string;
};

export type ConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
  managed_by_config_repo?: boolean;
  config_source_path?: string;
  last_sync_status: string;
  last_sync_message?: string;
  last_sync_started_at?: string;
  last_sync_completed_at?: string;
  last_sync_commit_sha?: string;
};

export type ConfigRepositoryFormState = {
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
};

export type NotificationMailSMTPSettings = {
  host: string;
  port: number;
  start_tls: boolean;
  username: string;
  password_credential_ref: string;
};

export type NotificationMailSettingsRecord = {
  enabled: boolean;
  from: string;
  smtp: NotificationMailSMTPSettings;
  source?: string;
  config_source_path?: string;
  managed_by_config_repo?: boolean;
  updated_at?: string;
};

export type NotificationMailSettingsFormState = {
  enabled: boolean;
  from: string;
  smtp_host: string;
  smtp_port: string;
  smtp_start_tls: boolean;
  smtp_username: string;
  smtp_password_credential_ref: string;
  test_to: string;
};

export const initialConfig: ConfigFormState = {
  log_level: '',
  log_format: '',
  environment: '',
  public_url: '',
  notification_mail_logo_url: '',
  notification_mail_website_url: '',
  notification_mail_support_url: '',
  notification_mail_footer_address: '',
  require_production_gates: false,
  agent_image: '',
  docker_network_name: '',
  default_pipeline_timeout: '',
  llm_agent_timeout: '',
  auto_removal_agent_container: true,
  agent_nopsai_api_url: '',
  git_bot_nopsai_api_url: '',
  nopsai_git_bot_api_url: '',
  github_app_id: '',
  github_installation_id: '',
  github_private_key_credential_ref: '',
  github_webhook_credential_ref: '',
  dispatcher_address: '',
  dispatcher_routing: {},
  runner_id: '',
  runner_scopes: '',
  runner_capacity: '1',
};

export const emptyConfigRepositoryForm: ConfigRepositoryFormState = {
  repo_url: '',
  branch: 'main',
  base_path: '',
  enabled: true,
  write_enabled: false,
  write_branch: 'nopsai/ui-changes',
};

export const emptyNotificationMailSettingsForm: NotificationMailSettingsFormState = {
  enabled: false,
  from: '',
  smtp_host: '',
  smtp_port: '587',
  smtp_start_tls: true,
  smtp_username: '',
  smtp_password_credential_ref: '',
  test_to: '',
};

export function normalizeSystemConfigPayload(payload: unknown): { config: ConfigFormState; envFilePath: string; fieldMetadata: Record<string, ConfigFieldMetadata> } {
  const record = asRecord(payload);
  if (!record) throw new Error('Unexpected system config response');

  return {
    config: {
      log_level: readString(record.log_level),
      log_format: readString(record.log_format),
      environment: readString(record.environment),
      public_url: readString(record.public_url),
      notification_mail_logo_url: readString(record.notification_mail_logo_url),
      notification_mail_website_url: readString(record.notification_mail_website_url),
      notification_mail_support_url: readString(record.notification_mail_support_url),
      notification_mail_footer_address: readString(record.notification_mail_footer_address),
      require_production_gates: Boolean(record.require_production_gates),
      agent_image: readString(record.agent_image),
      docker_network_name: readString(record.docker_network_name),
      default_pipeline_timeout: readString(record.default_pipeline_timeout),
      llm_agent_timeout: readString(record.llm_agent_timeout),
      auto_removal_agent_container: Boolean(record.auto_removal_agent_container),
      agent_nopsai_api_url: readString(record.agent_nopsai_api_url),
      git_bot_nopsai_api_url: readString(record.git_bot_nopsai_api_url),
      nopsai_git_bot_api_url: readString(record.nopsai_git_bot_api_url),
      github_app_id: readString(record.github_app_id),
      github_installation_id: readString(record.github_installation_id),
      github_private_key_credential_ref: readString(record.github_private_key_credential_ref),
      github_webhook_credential_ref: readString(record.github_webhook_credential_ref),
      dispatcher_address: readString(record.dispatcher_address),
      dispatcher_routing: normalizeRouting(record.dispatcher_routing),
      runner_id: readString(record.runner_id),
      runner_scopes: readString(record.runner_scopes),
      runner_capacity: String(record.runner_capacity ?? '1'),
    },
    envFilePath: readString(record.env_file_path),
    fieldMetadata: normalizeFieldMetadata(record.field_metadata),
  };
}

export function systemConfigPayloadFromForm(config: ConfigFormState) {
  return {
    log_level: config.log_level.trim(),
    log_format: config.log_format.trim(),
    environment: config.environment.trim(),
    public_url: config.public_url.trim(),
    notification_mail_logo_url: config.notification_mail_logo_url.trim(),
    notification_mail_website_url: config.notification_mail_website_url.trim(),
    notification_mail_support_url: config.notification_mail_support_url.trim(),
    notification_mail_footer_address: config.notification_mail_footer_address.trim(),
    require_production_gates: Boolean(config.require_production_gates),
    agent_image: config.agent_image.trim(),
    docker_network_name: config.docker_network_name.trim(),
    default_pipeline_timeout: config.default_pipeline_timeout.trim(),
    llm_agent_timeout: config.llm_agent_timeout.trim(),
    auto_removal_agent_container: Boolean(config.auto_removal_agent_container),
    agent_nopsai_api_url: config.agent_nopsai_api_url.trim(),
    git_bot_nopsai_api_url: config.git_bot_nopsai_api_url.trim(),
    nopsai_git_bot_api_url: config.nopsai_git_bot_api_url.trim(),
    github_app_id: config.github_app_id.trim(),
    github_installation_id: config.github_installation_id.trim(),
    github_private_key_credential_ref: config.github_private_key_credential_ref.trim(),
    github_webhook_credential_ref: config.github_webhook_credential_ref.trim(),
    dispatcher_address: config.dispatcher_address.trim(),
    dispatcher_routing: config.dispatcher_routing,
    runner_id: config.runner_id.trim(),
    runner_scopes: config.runner_scopes.trim(),
    runner_capacity: Number.parseInt(config.runner_capacity, 10) || 1,
  };
}

function normalizeFieldMetadata(value: unknown): Record<string, ConfigFieldMetadata> {
  const record = asRecord(value);
  if (!record) return {};
  const result: Record<string, ConfigFieldMetadata> = {};
  Object.entries(record).forEach(([key, raw]) => {
    const item = asRecord(raw);
    if (!item) return;
    result[key] = {
      scope: readString(item.scope),
      label: readString(item.label),
      section: readString(item.section),
      apply: readString(item.apply),
    };
  });
  return result;
}

export function normalizeConfigRepository(payload: unknown): ConfigRepository | null {
  const record = asRecord(payload);
  if (!record) return null;
  const id = normalizeNumber(record.id);
  return {
    id,
    scope_type: readString(record.scope_type),
    scope_id: readString(record.scope_id),
    repo_url: readString(record.repo_url),
    branch: readString(record.branch).trim() || 'main',
    base_path: readString(record.base_path),
    enabled: Boolean(record.enabled),
    write_enabled: Boolean(record.write_enabled),
    write_branch: readString(record.write_branch).trim() || 'nopsai/ui-changes',
    managed_by_config_repo: Boolean(record.managed_by_config_repo),
    config_source_path: readOptionalString(record.config_source_path),
    last_sync_status: readString(record.last_sync_status),
    last_sync_message: readOptionalString(record.last_sync_message),
    last_sync_started_at: readOptionalString(record.last_sync_started_at),
    last_sync_completed_at: readOptionalString(record.last_sync_completed_at),
    last_sync_commit_sha: readOptionalString(record.last_sync_commit_sha),
  };
}

export function configRepositoryFormFromRecord(repo: ConfigRepository | null): ConfigRepositoryFormState {
  if (!repo) return emptyConfigRepositoryForm;
  return {
    repo_url: repo.repo_url,
    branch: repo.branch || 'main',
    base_path: repo.base_path || '',
    enabled: repo.enabled,
    write_enabled: repo.write_enabled,
    write_branch: repo.write_branch || 'nopsai/ui-changes',
  };
}

export function configRepositoryPayloadFromForm(form: ConfigRepositoryFormState) {
  return {
    repo_url: form.repo_url.trim(),
    branch: form.branch.trim() || 'main',
    base_path: form.base_path.trim(),
    enabled: Boolean(form.enabled),
    write_enabled: Boolean(form.write_enabled),
    write_branch: form.write_branch.trim(),
  };
}

export function normalizeNotificationMailSettings(value: unknown): NotificationMailSettingsRecord {
  const record = asRecord(value);
  const smtp = asRecord(record?.smtp);
  const port = normalizeNumber(smtp?.port);
  return {
    enabled: Boolean(record?.enabled),
    from: readString(record?.from),
    smtp: {
      host: readString(smtp?.host),
      port: port > 0 ? port : 587,
      start_tls: typeof smtp?.start_tls === 'boolean' ? smtp.start_tls : true,
      username: readString(smtp?.username),
      password_credential_ref: readString(smtp?.password_credential_ref),
    },
    source: readOptionalString(record?.source),
    config_source_path: readOptionalString(record?.config_source_path),
    managed_by_config_repo: Boolean(record?.managed_by_config_repo),
    updated_at: readOptionalString(record?.updated_at),
  };
}

export function mailSettingsFormFromRecord(
  record: NotificationMailSettingsRecord,
  testTo = ''
): NotificationMailSettingsFormState {
  return {
    enabled: record.enabled,
    from: record.from,
    smtp_host: record.smtp.host,
    smtp_port: String(record.smtp.port || 587),
    smtp_start_tls: record.smtp.start_tls,
    smtp_username: record.smtp.username,
    smtp_password_credential_ref: record.smtp.password_credential_ref,
    test_to: testTo,
  };
}

export function mailSettingsPayloadFromForm(form: NotificationMailSettingsFormState) {
  const port = Number.parseInt(form.smtp_port, 10);
  return {
    enabled: form.enabled,
    from: form.from.trim(),
    smtp: {
      host: form.smtp_host.trim(),
      port: Number.isFinite(port) && port > 0 ? port : 587,
      start_tls: Boolean(form.smtp_start_tls),
      username: form.smtp_username.trim(),
      password_credential_ref: form.smtp_password_credential_ref.trim(),
    },
  };
}

export function normalizeRouting(value: unknown): Record<string, string[]> {
  const record = asRecord(value);
  if (!record) return {};
  const normalized: Record<string, string[]> = {};
  Object.entries(record).forEach(([scope, runners]) => {
    if (!scope) return;
    if (Array.isArray(runners)) {
      normalized[scope] = runners.map(item => String(item || '').trim()).filter(Boolean);
    } else if (typeof runners === 'string') {
      normalized[scope] = [runners.trim()].filter(Boolean);
    }
  });
  return normalized;
}
