import type { ChangeEvent, Dispatch, FormEvent, SetStateAction } from 'react';
import { Mail, Save, Send } from 'lucide-react';
import type {
  ConfigFieldMetadata,
  ConfigFormState,
  ConfigRepository,
  ConfigRepositoryFormState,
  NotificationMailSettingsFormState,
  NotificationMailSettingsRecord,
} from './config/model';
import { ApplyBadge } from './config/ConfigApplyBadge';
import GitHubAppSettingsCard from './config/GitHubAppSettingsCard';
import { RuntimePoolsEditor } from './config/RuntimePoolsEditor';
import { CredentialReferenceLink } from './credentials/CredentialReferenceLink';
import { CONFIG_REPOSITORY_PROVIDER_OPTIONS } from '../../lib/configRepositoryProviders.js';

function SystemConfig({
  config,
  envFilePath,
  fieldMetadata,
  configError,
  configLoading,
  saving,
  globalConfigRepo,
  globalConfigRepoForm,
  globalConfigRepoLoading,
  globalConfigRepoSaving,
  globalConfigRepoSyncing,
  globalConfigRepoError,
  mailSettings,
  mailSettingsForm,
  mailSettingsLoading,
  mailSettingsSaving,
  mailSettingsTesting,
  mailSettingsError,
  onChange,
  onReload,
  onSave,
  onMailSettingsChange,
  onSaveMailSettings,
  onTestMailSettings,
  onGlobalConfigRepoChange,
  onSaveGlobalConfigRepo,
  onDeleteGlobalConfigRepo,
  onSyncGlobalConfigRepo,
  onCheckGlobalConfigRepoDrift,
  globalConfigRepoDriftLoading,
  globalConfigRepoPushing,
  canViewRuntimeConfig,
  canManageRuntimeConfig,
  canViewGlobalConfigRepo,
  canManageGlobalConfigRepo,
}: {
  config: ConfigFormState;
  envFilePath: string;
  fieldMetadata: Record<string, ConfigFieldMetadata>;
  configError: string | null;
  configLoading: boolean;
  saving: boolean;
  globalConfigRepo: ConfigRepository | null;
  globalConfigRepoForm: ConfigRepositoryFormState;
  globalConfigRepoLoading: boolean;
  globalConfigRepoSaving: boolean;
  globalConfigRepoSyncing: boolean;
  globalConfigRepoError: string | null;
  mailSettings: NotificationMailSettingsRecord | null;
  mailSettingsForm: NotificationMailSettingsFormState;
  mailSettingsLoading: boolean;
  mailSettingsSaving: boolean;
  mailSettingsTesting: boolean;
  mailSettingsError: string | null;
  onChange: (next: ConfigFormState) => void;
  onReload: () => Promise<void>;
  onSave: () => Promise<void>;
  onMailSettingsChange: Dispatch<SetStateAction<NotificationMailSettingsFormState>>;
  onSaveMailSettings: () => Promise<void>;
  onTestMailSettings: () => Promise<void>;
  onGlobalConfigRepoChange: Dispatch<SetStateAction<ConfigRepositoryFormState>>;
  onSaveGlobalConfigRepo: () => Promise<void>;
  onDeleteGlobalConfigRepo: () => Promise<void>;
  onSyncGlobalConfigRepo: () => Promise<void>;
  onCheckGlobalConfigRepoDrift: () => Promise<void>;
  globalConfigRepoDriftLoading: boolean;
  globalConfigRepoPushing: boolean;
  canViewRuntimeConfig: boolean;
  canManageRuntimeConfig: boolean;
  canViewGlobalConfigRepo: boolean;
  canManageGlobalConfigRepo: boolean;
}) {
  const envPath = (envFilePath || '').trim();
  const globalRepoRunning = globalConfigRepo?.last_sync_status === 'running';
  const globalRepoCanEdit = canManageGlobalConfigRepo && !globalConfigRepoLoading && !globalConfigRepoSaving;
  const globalRepoSyncDisabled = !globalConfigRepo || !canManageGlobalConfigRepo || globalConfigRepoSyncing || globalConfigRepoSaving || globalRepoRunning;
  const globalRepoDriftDisabled = !globalConfigRepo || globalConfigRepoDriftLoading || globalConfigRepoSaving || globalConfigRepoSyncing || globalRepoRunning;
  const globalRepoPushDisabled = globalRepoDriftDisabled || globalConfigRepoPushing || !canManageGlobalConfigRepo || !globalConfigRepo?.write_enabled || !globalConfigRepo?.write_branch;
  const mailManaged = Boolean(mailSettings?.managed_by_config_repo);
  const mailCanEdit = canManageRuntimeConfig && !mailSettingsLoading && !mailSettingsSaving && !mailManaged;
  const mailSourceLabel = mailManaged ? 'GitOps' : mailSettings ? 'Database' : 'Default';
  const mailSectionClass = 'rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4';
  const mailFieldClass = 'flex flex-col gap-1 text-sm text-[var(--text-primary)]';
  const mailToggleClass = 'flex min-h-[46px] items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]';
  const mailInputClass = 'pipelines-input w-full';
  const mailSectionTitleClass = 'text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]';
  const handleChange = (key: keyof ConfigFormState) => (event: ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onChange({ ...config, [key]: value } as ConfigFormState);
  };

  const labelWithApply = (label: string, key: keyof ConfigFormState) => (
    <span className="flex flex-wrap items-center gap-2">
      <span>{label}</span>
      <ApplyBadge metadata={fieldMetadata[key]} />
    </span>
  );

  const handleMailChange = (key: keyof NotificationMailSettingsFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onMailSettingsChange(prev => ({ ...prev, [key]: value } as NotificationMailSettingsFormState));
  };

  const handleGlobalRepoChange = (key: keyof ConfigRepositoryFormState) => (event: ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onGlobalConfigRepoChange(prev => ({ ...prev, [key]: value } as ConfigRepositoryFormState));
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    void onSave();
  };

  return (
    <div id="system-config-section" className="grid gap-6 lg:grid-cols-2 pb-24">
      {canViewRuntimeConfig && (
      <form id="system-config-form" className="space-y-4 lg:col-span-2" onSubmit={onSubmit}>
        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">General</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Control plane defaults</h3>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Log level', 'log_level')}
              <select
                id="system-log-level"
                className="pipelines-input"
                value={config.log_level}
                onChange={handleChange('log_level')}
                disabled={!canManageRuntimeConfig || configLoading || saving}
              >
                <option value="">Default</option>
                <option value="debug">debug</option>
                <option value="info">info</option>
                <option value="warn">warn</option>
                <option value="error">error</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Log format', 'log_format')}
              <select
                id="system-log-format"
                className="pipelines-input"
                value={config.log_format}
                onChange={handleChange('log_format')}
                disabled={!canManageRuntimeConfig || configLoading || saving}
              >
                <option value="">Default</option>
                <option value="json">json</option>
                <option value="console">console</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Environment', 'environment')}
              <select
                id="system-environment"
                className="pipelines-input"
                value={config.environment}
                onChange={handleChange('environment')}
                disabled={!canManageRuntimeConfig || configLoading || saving}
              >
                <option value="">Default</option>
                <option value="development">development</option>
                <option value="staging">staging</option>
                <option value="production">production</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Public URL', 'public_url')}
              <input
                id="system-public-url"
                type="url"
                className="pipelines-input"
                value={config.public_url}
                onChange={handleChange('public_url')}
                placeholder="https://nopsai.example.com"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Mail logo URL', 'notification_mail_logo_url')}
              <input
                id="system-mail-logo-url"
                type="url"
                className="pipelines-input"
                value={config.notification_mail_logo_url}
                onChange={handleChange('notification_mail_logo_url')}
                placeholder="https://cdn.example.com/logo.png"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Mail website URL', 'notification_mail_website_url')}
              <input
                id="system-mail-website-url"
                type="url"
                className="pipelines-input"
                value={config.notification_mail_website_url}
                onChange={handleChange('notification_mail_website_url')}
                placeholder="https://example.com"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Mail support URL', 'notification_mail_support_url')}
              <input
                id="system-mail-support-url"
                type="url"
                className="pipelines-input"
                value={config.notification_mail_support_url}
                onChange={handleChange('notification_mail_support_url')}
                placeholder="https://support.example.com"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Mail footer address', 'notification_mail_footer_address')}
              <input
                id="system-mail-footer-address"
                type="text"
                className="pipelines-input"
                value={config.notification_mail_footer_address}
                onChange={handleChange('notification_mail_footer_address')}
                placeholder="Example Corp"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                id="system-require-production-gates"
                type="checkbox"
                checked={config.require_production_gates}
                onChange={handleChange('require_production_gates')}
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
              {labelWithApply('Require production gates', 'require_production_gates')}
            </label>
          </div>
        </div>

        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">Runtime tuning</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Runners & timeouts</h3>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Agent image', 'agent_image')}
              <input
                id="system-agent-image"
                type="text"
                className="pipelines-input"
                value={config.agent_image}
                onChange={handleChange('agent_image')}
                placeholder="nopsai-agent:latest"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('Docker network name', 'docker_network_name')}
              <input
                id="system-docker-network"
                type="text"
                className="pipelines-input"
                value={config.docker_network_name}
                onChange={handleChange('docker_network_name')}
                placeholder="nopsai-net"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                {labelWithApply('Default pipeline timeout', 'default_pipeline_timeout')}
                <input
                  id="system-default-timeout"
                  type="text"
                  className="pipelines-input"
                  value={config.default_pipeline_timeout}
                  onChange={handleChange('default_pipeline_timeout')}
                  placeholder="30m"
                  disabled={!canManageRuntimeConfig || configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                {labelWithApply('LLM agent timeout', 'llm_agent_timeout')}
                <input
                  id="system-llm-timeout"
                  type="text"
                  className="pipelines-input"
                  value={config.llm_agent_timeout}
                  onChange={handleChange('llm_agent_timeout')}
                  placeholder="2m"
                  disabled={!canManageRuntimeConfig || configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                {labelWithApply('Default runner ID', 'runner_id')}
                <input
                  id="system-runner-id"
                  type="text"
                  className="pipelines-input"
                  value={config.runner_id}
                  onChange={handleChange('runner_id')}
                  placeholder="runner-general"
                  disabled={!canManageRuntimeConfig || configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                {labelWithApply('Default runner scopes', 'runner_scopes')}
                <input
                  id="system-runner-scopes"
                  type="text"
                  className="pipelines-input"
                  value={config.runner_scopes}
                  onChange={handleChange('runner_scopes')}
                  placeholder="dev,prod"
                  disabled={!canManageRuntimeConfig || configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                {labelWithApply('Default runner capacity', 'runner_capacity')}
                <input
                  id="system-runner-capacity"
                  type="number"
                  min="1"
                  className="pipelines-input"
                  value={config.runner_capacity}
                  onChange={handleChange('runner_capacity')}
                  placeholder="2"
                  disabled={!canManageRuntimeConfig || configLoading || saving}
                />
              </label>
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                id="system-auto-remove"
                type="checkbox"
                checked={config.auto_removal_agent_container}
                onChange={handleChange('auto_removal_agent_container')}
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
              {labelWithApply('Auto-remove agent containers', 'auto_removal_agent_container')}
            </label>
          </div>
        </div>

        <RuntimePoolsEditor
          value={config.runtime_pools}
          metadata={fieldMetadata.runtime_pools}
          disabled={!canManageRuntimeConfig || configLoading || saving}
          onChange={runtime_pools => onChange({ ...config, runtime_pools })}
        />

        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">Networking</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Service discovery</h3>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('NopsAI API URL', 'nopsai_api_url')}
              <input
                id="system-nopsai-api"
                type="text"
                className="pipelines-input"
                value={config.nopsai_api_url}
                onChange={handleChange('nopsai_api_url')}
                placeholder="http://nopsai:8080"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              {labelWithApply('GitBot API URL', 'git_bot_api_url')}
              <input
                id="system-gitbot-api"
                type="text"
                className="pipelines-input"
                value={config.git_bot_api_url}
                onChange={handleChange('git_bot_api_url')}
                placeholder="http://git-bot:8081"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm md:col-span-2">
              {labelWithApply('Dispatcher gRPC address', 'dispatcher_grpc_address')}
              <input
                id="system-dispatcher-address"
                type="text"
                className="pipelines-input"
                value={config.dispatcher_grpc_address}
                onChange={handleChange('dispatcher_grpc_address')}
                placeholder="dispatcher:9090"
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
          </div>
        </div>

        <GitHubAppSettingsCard
          config={config}
          fieldMetadata={fieldMetadata}
          disabled={!canManageRuntimeConfig || configLoading || saving}
          onChange={onChange}
        />

        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">Notifications</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Mail server</h3>
              {mailSettings?.updated_at && <p className="text-xs text-[var(--text-secondary)]">Updated {formatTimestamp(mailSettings.updated_at)}</p>}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <span className="runner-pill runner-pill--muted">{mailSourceLabel}</span>
              {mailManaged && mailSettings?.config_source_path && (
                <span className="runner-pill runner-pill--link" title={mailSettings.config_source_path}>
                  {mailSettings.config_source_path}
                </span>
              )}
            </div>
          </div>

          {mailSettingsLoading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading mail settings…</p>
          ) : (
            <>
              {mailManaged && (
                <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
                  Managed by GitOps.
                </div>
              )}

              <div className={`${mailSectionClass} space-y-4`}>
                <p className={mailSectionTitleClass}>Delivery identity</p>
                <div className="grid grid-cols-1 md:grid-cols-[minmax(0,220px)_1fr] gap-4 items-end">
                  <label className={mailToggleClass}>
                    <input
                      id="system-mail-enabled"
                      type="checkbox"
                      checked={mailSettingsForm.enabled}
                      onChange={handleMailChange('enabled')}
                      disabled={!mailCanEdit}
                    />
                    <span>Enabled</span>
                  </label>
                  <label className={mailFieldClass}>
                    <span>From address</span>
                    <input
                      id="system-mail-from"
                      type="email"
                      className={mailInputClass}
                      value={mailSettingsForm.from}
                      onChange={handleMailChange('from')}
                      placeholder="nopsai@example.com"
                      disabled={!mailCanEdit}
                    />
                  </label>
                </div>
              </div>

              <div className={`${mailSectionClass} space-y-4`}>
                <p className={mailSectionTitleClass}>SMTP connection</p>
                <div className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_minmax(120px,160px)] gap-4">
                  <label className={mailFieldClass}>
                    <span>SMTP host</span>
                    <input
                      id="system-mail-smtp-host"
                      type="text"
                      className={mailInputClass}
                      value={mailSettingsForm.smtp_host}
                      onChange={handleMailChange('smtp_host')}
                      placeholder="smtp.example.com"
                      disabled={!mailCanEdit}
                    />
                  </label>
                  <label className={mailFieldClass}>
                    <span>SMTP port</span>
                    <input
                      id="system-mail-smtp-port"
                      type="number"
                      min="1"
                      max="65535"
                      className={mailInputClass}
                      value={mailSettingsForm.smtp_port}
                      onChange={handleMailChange('smtp_port')}
                      placeholder="587"
                      disabled={!mailCanEdit}
                    />
                  </label>
                  <label className={mailFieldClass}>
                    <span>SMTP username</span>
                    <input
                      id="system-mail-smtp-username"
                      type="text"
                      className={mailInputClass}
                      value={mailSettingsForm.smtp_username}
                      onChange={handleMailChange('smtp_username')}
                      placeholder="nopsai@example.com"
                      disabled={!mailCanEdit}
                    />
                  </label>
                  <label className={mailFieldClass}>
                    <span className="flex flex-wrap items-center gap-2">
                      <span>Password credential ref</span>
                      <CredentialReferenceLink reference={mailSettingsForm.smtp_password_credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
                        Open credential
                      </CredentialReferenceLink>
                    </span>
                    <input
                      id="system-mail-smtp-secret"
                      type="text"
                      className={mailInputClass}
                      value={mailSettingsForm.smtp_password_credential_ref}
                      onChange={handleMailChange('smtp_password_credential_ref')}
                      placeholder="credential://system/mail/smtp-password"
                      disabled={!mailCanEdit}
                    />
                  </label>
                  <label className={`${mailToggleClass} md:col-span-2`}>
                    <input
                      id="system-mail-smtp-start-tls"
                      type="checkbox"
                      checked={mailSettingsForm.smtp_start_tls}
                      onChange={handleMailChange('smtp_start_tls')}
                      disabled={!mailCanEdit}
                    />
                    <span>StartTLS</span>
                  </label>
                </div>
              </div>

              <div className={`${mailSectionClass} space-y-4`}>
                <p className={mailSectionTitleClass}>Test delivery</p>
                <div className="grid grid-cols-1 md:grid-cols-[1fr_auto_auto] gap-3 items-end">
                  <label className={mailFieldClass}>
                    <span>Test recipient</span>
                    <input
                      id="system-mail-test-to"
                      type="email"
                      className={mailInputClass}
                      value={mailSettingsForm.test_to}
                      onChange={handleMailChange('test_to')}
                      placeholder="operator@example.com"
                      disabled={!canManageRuntimeConfig || mailSettingsLoading || mailSettingsTesting}
                    />
                  </label>
                  <button
                    className="glass-button-subtle justify-center"
                    type="button"
                    onClick={() => void onTestMailSettings()}
                    disabled={!canManageRuntimeConfig || mailSettingsTesting || mailSettingsLoading || !mailSettings?.enabled}
                  >
                    <Send className="h-4 w-4" />
                    {mailSettingsTesting ? 'Sending…' : 'Send test'}
                  </button>
                  {canManageRuntimeConfig && (
                    <button className="glass-button-primary justify-center" type="button" onClick={() => void onSaveMailSettings()} disabled={!mailCanEdit}>
                      {mailSettingsSaving ? <Mail className="h-4 w-4" /> : <Save className="h-4 w-4" />}
                      {mailSettingsSaving ? 'Saving…' : 'Save mail'}
                    </button>
                  )}
                </div>
              </div>

              {mailSettingsError && (
                <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
                  {mailSettingsError}
                </div>
              )}
            </>
          )}
        </div>

        {envPath && <p className="text-xs text-[var(--text-secondary)]">Env file: {envPath}</p>}

        {configError && (
          <div className="glass-card p-4 border border-red-500/30 rounded-xl text-sm text-red-500">
            Failed to load or save config: {configError}
          </div>
        )}
        {configLoading && <p className="text-sm text-[var(--text-secondary)]">Loading settings…</p>}
      </form>
      )}

      {canViewGlobalConfigRepo && (
        <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4 lg:col-span-2">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">GitOps source</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Global config repository</h3>
            </div>
            {!canManageGlobalConfigRepo && <span className="runner-pill runner-pill--muted self-start">Read-only</span>}
          </div>

          {globalConfigRepoLoading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading global config repository…</p>
          ) : (
            <>
              {!globalConfigRepo && (
                <div className="rounded-lg border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
                  No global config repository connected.
                </div>
              )}

              {globalConfigRepo && (
                <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3 text-sm">
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Provider</p>
                      <p className="font-semibold text-[var(--text-primary)]">{globalConfigRepo.provider}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Status</p>
                      <p className="font-semibold text-[var(--text-primary)]">{globalConfigRepo.last_sync_status || 'Not synced'}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Completed</p>
                      <p className="font-semibold text-[var(--text-primary)]">{formatTimestamp(globalConfigRepo.last_sync_completed_at)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Started</p>
                      <p className="font-semibold text-[var(--text-primary)]">{formatTimestamp(globalConfigRepo.last_sync_started_at)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Commit</p>
                      <p className="font-semibold text-[var(--text-primary)] truncate" title={globalConfigRepo.last_sync_commit_sha || ''}>
                        {globalConfigRepo.last_sync_commit_sha || '-'}
                      </p>
                    </div>
                  </div>
                  {globalConfigRepo.last_sync_message && (
                    <p className="mt-3 text-xs text-[var(--text-secondary)] break-words">{globalConfigRepo.last_sync_message}</p>
                  )}
                </div>
              )}

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <label className="flex flex-col gap-1 text-sm">
                  <span>Provider</span>
                  <select
                    id="system-global-config-repo-provider"
                    className="pipelines-input"
                    value={globalConfigRepoForm.provider}
                    onChange={handleGlobalRepoChange('provider')}
                    disabled={!globalRepoCanEdit}
                  >
                    {CONFIG_REPOSITORY_PROVIDER_OPTIONS.map(option => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </select>
                </label>
                <label className="flex flex-col gap-1 text-sm">
                  <span>Repository URL</span>
                  <input
                    id="system-global-config-repo-url"
                    type="url"
                    required={canManageGlobalConfigRepo}
                    className="pipelines-input"
                    value={globalConfigRepoForm.repo_url}
                    onChange={handleGlobalRepoChange('repo_url')}
                    placeholder="https://github.com/org/nopsai-config"
                    disabled={!globalRepoCanEdit}
                  />
                </label>
                <label className="flex flex-col gap-1 text-sm md:col-span-2">
                  <span>Credential reference</span>
                  <input
                    id="system-global-config-repo-credential-ref"
                    type="text"
                    className="pipelines-input"
                    value={globalConfigRepoForm.credential_ref}
                    onChange={handleGlobalRepoChange('credential_ref')}
                    placeholder="credential://system/gitops/gitlab-token"
                    required={canManageGlobalConfigRepo && globalConfigRepoForm.provider !== 'github'}
                    disabled={!globalRepoCanEdit}
                  />
                  <CredentialReferenceLink reference={globalConfigRepoForm.credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
                    Open credential
                  </CredentialReferenceLink>
                </label>
                <label className="flex flex-col gap-1 text-sm">
                  <span>Branch</span>
                  <input
                    id="system-global-config-repo-branch"
                    type="text"
                    className="pipelines-input"
                    value={globalConfigRepoForm.branch}
                    onChange={handleGlobalRepoChange('branch')}
                    placeholder="main"
                    disabled={!globalRepoCanEdit}
                  />
                </label>
                <label className="flex flex-col gap-1 text-sm">
                  <span>Base path</span>
                  <input
                    id="system-global-config-repo-base-path"
                    type="text"
                    className="pipelines-input"
                    value={globalConfigRepoForm.base_path}
                    onChange={handleGlobalRepoChange('base_path')}
                    placeholder="nopsai"
                    disabled={!globalRepoCanEdit}
                  />
                </label>
                <label className="flex items-center gap-2 text-sm md:col-span-2">
                  <input
                    id="system-global-config-repo-enabled"
                    type="checkbox"
                    checked={globalConfigRepoForm.enabled}
                    onChange={handleGlobalRepoChange('enabled')}
                    disabled={!globalRepoCanEdit}
                  />
                  <span>Enabled</span>
                </label>
                <div className="md:col-span-2 border-t border-[var(--border-primary)] pt-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        id="system-global-config-repo-write-enabled"
                        type="checkbox"
                        checked={globalConfigRepoForm.write_enabled}
                        onChange={handleGlobalRepoChange('write_enabled')}
                        disabled={!globalRepoCanEdit}
                      />
                      <span>Enable Git push</span>
                    </label>
                    <label className="flex flex-col gap-1 text-sm">
                      <span>Push branch</span>
                      <input
                        id="system-global-config-repo-write-branch"
                        type="text"
                        className="pipelines-input"
                        value={globalConfigRepoForm.write_branch}
                        onChange={handleGlobalRepoChange('write_branch')}
                        placeholder="nopsai/ui-changes"
                        disabled={!globalRepoCanEdit || !globalConfigRepoForm.write_enabled}
                      />
                    </label>
                  </div>
                </div>
              </div>

              {globalConfigRepo?.managed_by_config_repo && globalConfigRepo.config_source_path && (
                <p className="text-xs text-[var(--text-secondary)]">Managed by Git: {globalConfigRepo.config_source_path}</p>
              )}

              {globalConfigRepoError && (
                <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
                  {globalConfigRepoError}
                </div>
              )}

              <div className="flex flex-wrap items-center justify-end gap-2">
                {globalConfigRepo && canManageGlobalConfigRepo && (
                  <button type="button" className="glass-button-danger mr-auto" onClick={() => void onDeleteGlobalConfigRepo()} disabled={globalConfigRepoSaving || globalConfigRepoSyncing}>
                    Remove
                  </button>
                )}
                <button type="button" className="glass-button-subtle" onClick={() => void onCheckGlobalConfigRepoDrift()} disabled={globalRepoDriftDisabled}>
                  {globalConfigRepoDriftLoading ? 'Checking...' : 'Check drift'}
                </button>
                <button type="button" className="glass-button-subtle" onClick={() => void onCheckGlobalConfigRepoDrift()} disabled={globalRepoPushDisabled}>
                  {globalConfigRepoPushing ? 'Pushing...' : 'Push to Git'}
                </button>
                <button type="button" className="glass-button-subtle" onClick={() => void onSyncGlobalConfigRepo()} disabled={globalRepoSyncDisabled}>
                  {globalConfigRepoSyncing || globalRepoRunning ? 'Syncing…' : 'Sync'}
                </button>
                {canManageGlobalConfigRepo && (
                  <button type="button" className="glass-button-primary" onClick={() => void onSaveGlobalConfigRepo()} disabled={!globalRepoCanEdit}>
                    {globalConfigRepoSaving ? 'Saving…' : 'Save repository'}
                  </button>
                )}
              </div>
            </>
          )}
        </section>
      )}

      {canViewRuntimeConfig && (
      <div className="fixed bottom-6 right-6 z-40 flex items-center gap-2">
        <button className="glass-button-ghost" type="button" onClick={() => void onReload()} disabled={configLoading || saving}>
          Reload
        </button>
        <button className="glass-button-primary" type="button" onClick={() => void onSave()} disabled={!canManageRuntimeConfig || configLoading || saving}>
          {saving ? 'Saving…' : 'Save settings'}
        </button>
      </div>
      )}
    </div>
  );
}

function formatTimestamp(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}


export default SystemConfig;
