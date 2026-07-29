import { useMemo, useState, type ChangeEvent, type Dispatch, type FormEvent, type SetStateAction } from 'react';
import { Link } from 'react-router-dom';
import {
  GitBranch,
  Mail,
  RefreshCw,
  Route,
  Save,
  Search,
  Send,
} from 'lucide-react';
import { CredentialReferenceLink } from '../credentials/CredentialReferenceLink';
import { CONFIG_REPOSITORY_PROVIDER_OPTIONS } from '../../../lib/configRepositoryProviders.js';
import { ApplyBadge } from './ConfigApplyBadge';
import { RuntimePoolsEditor } from './RuntimePoolsEditor';
import {
  ErrorBox,
  RepoFact,
  SectionIcon,
  SettingField,
  SettingsPanel,
  SettingsSection,
  SettingsToggle,
  SummaryCard,
} from './SystemConfigWorkspacePrimitives';
import {
  formatTimestamp,
  sectionByID,
  systemSettingsSectionDomID,
} from './settingsWorkspaceHelpers';
import {
  SYSTEM_SETTINGS_SECTIONS,
  buildSystemSettingsSummary,
  filterSystemSettingsSections,
  type SystemSettingsSectionId,
} from './settingsPresentation';
import type {
  ConfigFieldMetadata,
  ConfigFormState,
  ConfigRepository,
  ConfigRepositoryFormState,
  NotificationMailSettingsFormState,
  NotificationMailSettingsRecord,
} from './model';

export type SystemConfigWorkspaceProps = {
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
  canViewDispatcher?: boolean;
};

function SystemConfigWorkspace({
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
  canViewDispatcher = false,
}: SystemConfigWorkspaceProps) {
  const [query, setQuery] = useState('');
  const [activeSection, setActiveSection] = useState<SystemSettingsSectionId>('platform');
  const availableSections = useMemo(
    () => SYSTEM_SETTINGS_SECTIONS.filter(section => (section.id === 'source' ? canViewGlobalConfigRepo : canViewRuntimeConfig)),
    [canViewGlobalConfigRepo, canViewRuntimeConfig]
  );
  const visibleSections = useMemo(() => filterSystemSettingsSections(query, availableSections), [availableSections, query]);
  const visibleSectionIDs = useMemo(() => new Set(visibleSections.map(section => section.id)), [visibleSections]);
  const summaryCards = useMemo(
    () => buildSystemSettingsSummary({ config, envFilePath, globalConfigRepo, mailSettings, canViewGlobalConfigRepo }),
    [canViewGlobalConfigRepo, config, envFilePath, globalConfigRepo, mailSettings]
  );
  const visibleActiveSection = visibleSectionIDs.has(activeSection) ? activeSection : visibleSections[0]?.id;

  const envPath = envFilePath.trim();
  const runtimeDisabled = !canManageRuntimeConfig || configLoading || saving;
  const globalRepoRunning = globalConfigRepo?.last_sync_status === 'running';
  const globalRepoCanEdit = canManageGlobalConfigRepo && !globalConfigRepoLoading && !globalConfigRepoSaving;
  const globalRepoSyncDisabled = !globalConfigRepo || !canManageGlobalConfigRepo || globalConfigRepoSyncing || globalConfigRepoSaving || globalRepoRunning;
  const globalRepoDriftDisabled = !globalConfigRepo || globalConfigRepoDriftLoading || globalConfigRepoSaving || globalConfigRepoSyncing || globalRepoRunning || globalConfigRepoPushing;
  const globalRepoPushDisabled = globalRepoDriftDisabled || !canManageGlobalConfigRepo || !globalConfigRepo?.write_enabled || !globalConfigRepo?.write_branch;
  const mailManaged = Boolean(mailSettings?.managed_by_config_repo);
  const mailCanEdit = canManageRuntimeConfig && !mailSettingsLoading && !mailSettingsSaving && !mailManaged;
  const mailSourceLabel = mailManaged ? 'GitOps' : mailSettings ? 'Database' : 'Default';

  const handleConfigChange = (key: keyof ConfigFormState) => (event: ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onChange({ ...config, [key]: value } as ConfigFormState);
  };

  const handleMailChange = (key: keyof NotificationMailSettingsFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onMailSettingsChange(prev => ({ ...prev, [key]: value } as NotificationMailSettingsFormState));
  };

  const handleGlobalRepoChange = (key: keyof ConfigRepositoryFormState) => (event: ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onGlobalConfigRepoChange(prev => ({ ...prev, [key]: value } as ConfigRepositoryFormState));
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    void onSave();
  };

  const labelWithApply = (label: string, key: keyof ConfigFormState) => (
    <span className="system-settings-label-with-badge">
      <span>{label}</span>
      <ApplyBadge metadata={fieldMetadata[key]} />
    </span>
  );

  const selectSection = (sectionID: SystemSettingsSectionId) => {
    setActiveSection(sectionID);
  };

  const showSection = (sectionID: SystemSettingsSectionId) => visibleActiveSection === sectionID;

  return (
    <div id="system-config-section" className="system-settings-workspace">
      <h2 className="sr-only">System settings</h2>

      <div className="system-settings-summary-grid" aria-label="Settings summary">
        {summaryCards.map(card => (
          <SummaryCard key={card.id} card={card} />
        ))}
      </div>

      <div className="system-settings-control-bar">
        <div className="system-settings-tabs-row">
          <div className="system-settings-tablist" role="tablist" aria-label="Settings sections">
            {visibleSections.map(section => (
              <button
                key={section.id}
                id={`system-settings-tab-${section.id}`}
                type="button"
                role="tab"
                aria-selected={visibleActiveSection === section.id}
                aria-controls={systemSettingsSectionDomID(section.id)}
                className={`system-settings-tab ${visibleActiveSection === section.id ? 'is-active' : ''}`}
                onClick={() => selectSection(section.id)}
              >
                <span className="system-settings-tab__icon"><SectionIcon sectionID={section.id} /></span>
                <span className="system-settings-tab__label">{section.label}</span>
              </button>
            ))}
          </div>
          <label className="system-settings-search">
            <Search className="h-4 w-4" />
            <input
              type="search"
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder="Search settings"
              aria-label="Search settings"
            />
          </label>
        </div>
        {envPath && (
          <div className="system-settings-context-strip">
            <div className="system-settings-context-item">
              <span>Env file</span>
              <code>{envPath}</code>
            </div>
          </div>
        )}
      </div>

      <main className="system-settings-content">
          {!visibleSections.length && (
            <div className="system-settings-empty">
              <strong>No matching settings</strong>
              <span>Try a broader search term.</span>
            </div>
          )}

          {canViewRuntimeConfig && visibleActiveSection && visibleActiveSection !== 'source' && (
            <form id="system-config-form" className="system-settings-form" onSubmit={handleSubmit}>
              {showSection('platform') && (
                <SettingsSection section={sectionByID('platform')}>
                  <div className="system-settings-panel-grid">
                    <SettingsPanel title="Runtime Posture" description="Environment and logging defaults used by the control plane.">
                      <div className="system-settings-field-grid system-settings-field-grid--compact">
                      <SettingField label={labelWithApply('Log level', 'log_level')}>
                        <select
                          id="system-log-level"
                          className="pipelines-input"
                          value={config.log_level}
                          onChange={handleConfigChange('log_level')}
                          disabled={runtimeDisabled}
                        >
                          <option value="">Default</option>
                          <option value="debug">debug</option>
                          <option value="info">info</option>
                          <option value="warn">warn</option>
                          <option value="error">error</option>
                        </select>
                      </SettingField>
                      <SettingField label={labelWithApply('Log format', 'log_format')}>
                        <select
                          id="system-log-format"
                          className="pipelines-input"
                          value={config.log_format}
                          onChange={handleConfigChange('log_format')}
                          disabled={runtimeDisabled}
                        >
                          <option value="">Default</option>
                          <option value="json">json</option>
                          <option value="console">console</option>
                        </select>
                      </SettingField>
                      <SettingField label={labelWithApply('Environment', 'environment')}>
                        <select
                          id="system-environment"
                          className="pipelines-input"
                          value={config.environment}
                          onChange={handleConfigChange('environment')}
                          disabled={runtimeDisabled}
                        >
                          <option value="">Default</option>
                          <option value="development">development</option>
                          <option value="staging">staging</option>
                          <option value="production">production</option>
                        </select>
                      </SettingField>
                        <SettingsToggle
                          label={labelWithApply('Require production gates', 'require_production_gates')}
                          checked={config.require_production_gates}
                          onChange={handleConfigChange('require_production_gates')}
                          disabled={runtimeDisabled}
                          wide
                        />
                      </div>
                    </SettingsPanel>

                    <SettingsPanel title="Public Identity" description="URLs and mail branding shown outside the control plane.">
                      <div className="system-settings-field-grid">
                      <SettingField label={labelWithApply('Public URL', 'public_url')}>
                        <input
                          id="system-public-url"
                          type="url"
                          className="pipelines-input"
                          value={config.public_url}
                          onChange={handleConfigChange('public_url')}
                          placeholder="https://nopsai.example.com"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Mail logo URL', 'notification_mail_logo_url')}>
                        <input
                          id="system-mail-logo-url"
                          type="url"
                          className="pipelines-input"
                          value={config.notification_mail_logo_url}
                          onChange={handleConfigChange('notification_mail_logo_url')}
                          placeholder="https://cdn.example.com/logo.png"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Mail website URL', 'notification_mail_website_url')}>
                        <input
                          id="system-mail-website-url"
                          type="url"
                          className="pipelines-input"
                          value={config.notification_mail_website_url}
                          onChange={handleConfigChange('notification_mail_website_url')}
                          placeholder="https://example.com"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Mail support URL', 'notification_mail_support_url')}>
                        <input
                          id="system-mail-support-url"
                          type="url"
                          className="pipelines-input"
                          value={config.notification_mail_support_url}
                          onChange={handleConfigChange('notification_mail_support_url')}
                          placeholder="https://support.example.com"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Mail footer address', 'notification_mail_footer_address')}>
                        <input
                          id="system-mail-footer-address"
                          type="text"
                          className="pipelines-input"
                          value={config.notification_mail_footer_address}
                          onChange={handleConfigChange('notification_mail_footer_address')}
                          placeholder="Example Corp"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      </div>
                    </SettingsPanel>
                  </div>
                </SettingsSection>
              )}

              {showSection('execution') && (
                <SettingsSection section={sectionByID('execution')}>
                  <div className="system-settings-panel-grid">
                    <SettingsPanel
                      title="Runner Template"
                      description="Image, network, identity, scopes, and capacity used when runtime objects do not override them."
                      action={canViewDispatcher && (
                        <Link to="/system/dispatcher?guide=runner" className="glass-button-subtle">
                          <Route className="h-4 w-4" />
                          Dispatcher
                        </Link>
                      )}
                    >
                      <div className="system-settings-field-grid">
                      <SettingField label={labelWithApply('Agent image', 'agent_image')}>
                        <input
                          id="system-agent-image"
                          type="text"
                          className="pipelines-input"
                          value={config.agent_image}
                          onChange={handleConfigChange('agent_image')}
                          placeholder="nopsai-agent:latest"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Docker network name', 'docker_network_name')}>
                        <input
                          id="system-docker-network"
                          type="text"
                          className="pipelines-input"
                          value={config.docker_network_name}
                          onChange={handleConfigChange('docker_network_name')}
                          placeholder="nopsai-net"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                        <SettingField label={labelWithApply('Default runner ID', 'runner_id')}>
                          <input
                            id="system-runner-id"
                            type="text"
                            className="pipelines-input"
                            value={config.runner_id}
                            onChange={handleConfigChange('runner_id')}
                            placeholder="runner-general"
                            disabled={runtimeDisabled}
                          />
                        </SettingField>
                        <SettingField label={labelWithApply('Default runner scopes', 'runner_scopes')}>
                          <input
                            id="system-runner-scopes"
                            type="text"
                            className="pipelines-input"
                            value={config.runner_scopes}
                            onChange={handleConfigChange('runner_scopes')}
                            placeholder="dev,prod"
                            disabled={runtimeDisabled}
                          />
                        </SettingField>
                      </div>
                    </SettingsPanel>

                    <SettingsPanel title="Run Limits" description="Timeout, agent lifecycle, and default runner capacity.">
                      <div className="system-settings-field-grid system-settings-field-grid--compact">
                      <SettingField label={labelWithApply('Default pipeline timeout', 'default_pipeline_timeout')}>
                        <input
                          id="system-default-timeout"
                          type="text"
                          className="pipelines-input"
                          value={config.default_pipeline_timeout}
                          onChange={handleConfigChange('default_pipeline_timeout')}
                          placeholder="30m"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('LLM agent timeout', 'llm_agent_timeout')}>
                        <input
                          id="system-llm-timeout"
                          type="text"
                          className="pipelines-input"
                          value={config.llm_agent_timeout}
                          onChange={handleConfigChange('llm_agent_timeout')}
                          placeholder="2m"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Default runner capacity', 'runner_capacity')}>
                        <input
                          id="system-runner-capacity"
                          type="number"
                          min="1"
                          className="pipelines-input"
                          value={config.runner_capacity}
                          onChange={handleConfigChange('runner_capacity')}
                          placeholder="2"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingsToggle
                        label={labelWithApply('Auto-remove agent containers', 'auto_removal_agent_container')}
                        checked={config.auto_removal_agent_container}
                        onChange={handleConfigChange('auto_removal_agent_container')}
                        disabled={runtimeDisabled}
                      />
                      </div>
                    </SettingsPanel>
                  </div>

                  <RuntimePoolsEditor
                    value={config.runtime_pools}
                    metadata={fieldMetadata.runtime_pools}
                    disabled={runtimeDisabled}
                    onChange={runtime_pools => onChange({ ...config, runtime_pools })}
                  />
                </SettingsSection>
              )}

              {showSection('networking') && (
                <SettingsSection section={sectionByID('networking')}>
                  <div className="system-settings-card">
                    <div className="system-settings-field-grid">
                      <SettingField label={labelWithApply('NopsAI API URL', 'nopsai_api_url')}>
                        <input
                          id="system-nopsai-api"
                          type="text"
                          className="pipelines-input"
                          value={config.nopsai_api_url}
                          onChange={handleConfigChange('nopsai_api_url')}
                          placeholder="http://nopsai:8080"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('GitBot API URL', 'git_bot_api_url')}>
                        <input
                          id="system-gitbot-api"
                          type="text"
                          className="pipelines-input"
                          value={config.git_bot_api_url}
                          onChange={handleConfigChange('git_bot_api_url')}
                          placeholder="http://git-bot:8081"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                      <SettingField label={labelWithApply('Dispatcher gRPC address', 'dispatcher_grpc_address')} wide>
                        <input
                          id="system-dispatcher-address"
                          type="text"
                          className="pipelines-input"
                          value={config.dispatcher_grpc_address}
                          onChange={handleConfigChange('dispatcher_grpc_address')}
                          placeholder="dispatcher:9090"
                          disabled={runtimeDisabled}
                        />
                      </SettingField>
                    </div>
                  </div>
                </SettingsSection>
              )}

              {showSection('notifications') && (
                <SettingsSection section={sectionByID('notifications')}>
                  <div className="system-settings-card">
                    <div className="system-settings-card__toolbar">
                      <div>
                        <h3>Mail Server</h3>
                        {mailSettings?.updated_at && <p>Updated {formatTimestamp(mailSettings.updated_at)}</p>}
                      </div>
                      <div className="system-settings-badge-row">
                        <span className="runner-pill runner-pill--muted">{mailSourceLabel}</span>
                        {mailManaged && mailSettings?.config_source_path && (
                          <span className="runner-pill runner-pill--link" title={mailSettings.config_source_path}>
                            {mailSettings.config_source_path}
                          </span>
                        )}
                      </div>
                    </div>

                    {mailSettingsLoading ? (
                      <p className="system-settings-muted">Loading mail settings...</p>
                    ) : (
                      <>
                        {mailManaged && (
                          <div className="system-settings-inline-alert">
                            Managed by GitOps.
                          </div>
                        )}
                        <div className="system-settings-field-grid">
                          <SettingsToggle
                            label="Enabled"
                            checked={mailSettingsForm.enabled}
                            onChange={handleMailChange('enabled')}
                            disabled={!mailCanEdit}
                          />
                          <SettingField label="From address">
                            <input
                              id="system-mail-from"
                              type="email"
                              className="pipelines-input"
                              value={mailSettingsForm.from}
                              onChange={handleMailChange('from')}
                              placeholder="nopsai@example.com"
                              disabled={!mailCanEdit}
                            />
                          </SettingField>
                          <SettingField label="SMTP host">
                            <input
                              id="system-mail-smtp-host"
                              type="text"
                              className="pipelines-input"
                              value={mailSettingsForm.smtp_host}
                              onChange={handleMailChange('smtp_host')}
                              placeholder="smtp.example.com"
                              disabled={!mailCanEdit}
                            />
                          </SettingField>
                          <SettingField label="SMTP port">
                            <input
                              id="system-mail-smtp-port"
                              type="number"
                              min="1"
                              max="65535"
                              className="pipelines-input"
                              value={mailSettingsForm.smtp_port}
                              onChange={handleMailChange('smtp_port')}
                              placeholder="587"
                              disabled={!mailCanEdit}
                            />
                          </SettingField>
                          <SettingField label="SMTP username">
                            <input
                              id="system-mail-smtp-username"
                              type="text"
                              className="pipelines-input"
                              value={mailSettingsForm.smtp_username}
                              onChange={handleMailChange('smtp_username')}
                              placeholder="nopsai@example.com"
                              disabled={!mailCanEdit}
                            />
                          </SettingField>
                          <SettingField
                            label={
                              <span className="system-settings-label-with-badge">
                                <span>Password credential ref</span>
                                <CredentialReferenceLink reference={mailSettingsForm.smtp_password_credential_ref} className="system-settings-inline-link">
                                  Open credential
                                </CredentialReferenceLink>
                              </span>
                            }
                            helper="Expected type: password"
                          >
                            <input
                              id="system-mail-smtp-secret"
                              type="text"
                              className="pipelines-input"
                              value={mailSettingsForm.smtp_password_credential_ref}
                              onChange={handleMailChange('smtp_password_credential_ref')}
                              placeholder="credential://system/mail/smtp-password"
                              disabled={!mailCanEdit}
                            />
                          </SettingField>
                          <SettingsToggle
                            label="StartTLS"
                            checked={mailSettingsForm.smtp_start_tls}
                            onChange={handleMailChange('smtp_start_tls')}
                            disabled={!mailCanEdit}
                          />
                          <SettingField label="Test recipient">
                            <input
                              id="system-mail-test-to"
                              type="email"
                              className="pipelines-input"
                              value={mailSettingsForm.test_to}
                              onChange={handleMailChange('test_to')}
                              placeholder="operator@example.com"
                              disabled={!canManageRuntimeConfig || mailSettingsLoading || mailSettingsTesting}
                            />
                          </SettingField>
                        </div>

                        <div className="system-settings-actions">
                          <button
                            className="glass-button-subtle"
                            type="button"
                            onClick={() => void onTestMailSettings()}
                            disabled={!canManageRuntimeConfig || mailSettingsTesting || mailSettingsLoading || !mailSettings?.enabled}
                          >
                            <Send className="h-4 w-4" />
                            {mailSettingsTesting ? 'Sending...' : 'Send test'}
                          </button>
                          {canManageRuntimeConfig && (
                            <button className="glass-button-primary" type="button" onClick={() => void onSaveMailSettings()} disabled={!mailCanEdit}>
                              {mailSettingsSaving ? <Mail className="h-4 w-4" /> : <Save className="h-4 w-4" />}
                              {mailSettingsSaving ? 'Saving...' : 'Save mail'}
                            </button>
                          )}
                        </div>
                      </>
                    )}

                    {mailSettingsError && <ErrorBox>{mailSettingsError}</ErrorBox>}
                  </div>
                </SettingsSection>
              )}

              {(configError || configLoading) && (
                <div className="system-settings-status-stack">
                  {configError && <ErrorBox>Failed to load or save config: {configError}</ErrorBox>}
                  {configLoading && <p className="system-settings-muted">Loading settings...</p>}
                </div>
              )}

              {canViewRuntimeConfig && (
                <div className="system-settings-action-bar">
                  <button className="glass-button-primary" type="submit" disabled={!canManageRuntimeConfig || configLoading || saving}>
                    <Save className="h-4 w-4" />
                    {saving ? 'Saving...' : 'Save settings'}
                  </button>
                </div>
              )}
            </form>
          )}

          {canViewGlobalConfigRepo && showSection('source') && (
            <SettingsSection section={sectionByID('source')}>
              <div className="system-settings-card">
                <div className="system-settings-card__toolbar">
                  <div>
                    <h3>Repository Connection</h3>
                    <p>Provider, source branch, sync state, and write-back branch.</p>
                  </div>
                  {!canManageGlobalConfigRepo && <span className="runner-pill runner-pill--muted">Read-only</span>}
                </div>

                {globalConfigRepoLoading ? (
                  <p className="system-settings-muted">Loading global config repository...</p>
                ) : (
                  <>
                    {!globalConfigRepo && (
                      <div className="system-settings-inline-alert system-settings-inline-alert--empty">
                        No global config repository connected.
                      </div>
                    )}

                    {globalConfigRepo && (
                      <div className="system-settings-repo-strip">
                        <RepoFact label="Provider" value={globalConfigRepo.provider} />
                        <RepoFact label="Status" value={globalConfigRepo.last_sync_status || 'Not synced'} />
                        <RepoFact label="Completed" value={formatTimestamp(globalConfigRepo.last_sync_completed_at)} />
                        <RepoFact label="Started" value={formatTimestamp(globalConfigRepo.last_sync_started_at)} />
                        <RepoFact label="Commit" value={globalConfigRepo.last_sync_commit_sha || '-'} truncate />
                        {globalConfigRepo.last_sync_message && (
                          <p className="system-settings-repo-strip__message">{globalConfigRepo.last_sync_message}</p>
                        )}
                      </div>
                    )}

                    <div className="system-settings-field-grid">
                      <SettingField label="Provider">
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
                      </SettingField>
                      <SettingField label="Repository URL">
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
                      </SettingField>
                      <SettingField
                        label={
                          <span className="system-settings-label-with-badge">
                            <span>Credential reference</span>
                            <CredentialReferenceLink reference={globalConfigRepoForm.credential_ref} className="system-settings-inline-link">
                              Open credential
                            </CredentialReferenceLink>
                          </span>
                        }
                        helper="Expected type: bearer_token"
                        wide
                      >
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
                      </SettingField>
                      <SettingField label="Branch">
                        <input
                          id="system-global-config-repo-branch"
                          type="text"
                          className="pipelines-input"
                          value={globalConfigRepoForm.branch}
                          onChange={handleGlobalRepoChange('branch')}
                          placeholder="main"
                          disabled={!globalRepoCanEdit}
                        />
                      </SettingField>
                      <SettingField label="Base path">
                        <input
                          id="system-global-config-repo-base-path"
                          type="text"
                          className="pipelines-input"
                          value={globalConfigRepoForm.base_path}
                          onChange={handleGlobalRepoChange('base_path')}
                          placeholder="nopsai"
                          disabled={!globalRepoCanEdit}
                        />
                      </SettingField>
                      <SettingsToggle
                        label="Enabled"
                        checked={globalConfigRepoForm.enabled}
                        onChange={handleGlobalRepoChange('enabled')}
                        disabled={!globalRepoCanEdit}
                      />
                      <SettingsToggle
                        label="Enable Git push"
                        checked={globalConfigRepoForm.write_enabled}
                        onChange={handleGlobalRepoChange('write_enabled')}
                        disabled={!globalRepoCanEdit}
                      />
                      <SettingField label="Push branch" wide>
                        <input
                          id="system-global-config-repo-write-branch"
                          type="text"
                          className="pipelines-input"
                          value={globalConfigRepoForm.write_branch}
                          onChange={handleGlobalRepoChange('write_branch')}
                          placeholder="nopsai/ui-changes"
                          disabled={!globalRepoCanEdit || !globalConfigRepoForm.write_enabled}
                        />
                      </SettingField>
                    </div>

                    {globalConfigRepo?.managed_by_config_repo && globalConfigRepo.config_source_path && (
                      <p className="system-settings-muted">Managed by Git: {globalConfigRepo.config_source_path}</p>
                    )}

                    {globalConfigRepoError && <ErrorBox>{globalConfigRepoError}</ErrorBox>}

                    <div className="system-settings-actions system-settings-actions--repo">
                      {globalConfigRepo && canManageGlobalConfigRepo && (
                        <button
                          type="button"
                          className="glass-button-danger system-settings-actions__start"
                          onClick={() => void onDeleteGlobalConfigRepo()}
                          disabled={globalConfigRepoSaving || globalConfigRepoSyncing}
                        >
                          Remove
                        </button>
                      )}
                      <button type="button" className="glass-button-subtle" onClick={() => void onCheckGlobalConfigRepoDrift()} disabled={globalRepoDriftDisabled}>
                        <GitBranch className="h-4 w-4" />
                        {globalConfigRepoDriftLoading ? 'Checking...' : 'Check drift'}
                      </button>
                      <button type="button" className="glass-button-subtle" onClick={() => void onCheckGlobalConfigRepoDrift()} disabled={globalRepoPushDisabled}>
                        <GitBranch className="h-4 w-4" />
                        {globalConfigRepoPushing ? 'Pushing...' : 'Review & push'}
                      </button>
                      <button type="button" className="glass-button-subtle" onClick={() => void onSyncGlobalConfigRepo()} disabled={globalRepoSyncDisabled}>
                        <RefreshCw className={`h-4 w-4 ${globalConfigRepoSyncing || globalRepoRunning ? 'animate-spin' : ''}`} />
                        {globalConfigRepoSyncing || globalRepoRunning ? 'Syncing...' : 'Sync'}
                      </button>
                      {canManageGlobalConfigRepo && (
                        <button type="button" className="glass-button-primary" onClick={() => void onSaveGlobalConfigRepo()} disabled={!globalRepoCanEdit}>
                          <Save className="h-4 w-4" />
                          {globalConfigRepoSaving ? 'Saving...' : 'Save repository'}
                        </button>
                      )}
                    </div>
                  </>
                )}
              </div>
            </SettingsSection>
          )}
      </main>
    </div>
  );
}

export default SystemConfigWorkspace;
