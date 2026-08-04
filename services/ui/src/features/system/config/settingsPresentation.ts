import type { ConfigFormState, ConfigRepository, NotificationMailSettingsRecord } from './model.js';

export type SystemSettingsSectionId = 'platform' | 'execution' | 'networking' | 'notifications' | 'source';

export type SystemSettingsSection = {
  id: SystemSettingsSectionId;
  label: string;
  title: string;
  eyebrow: string;
  description: string;
  fieldLabels: string[];
  keywords: string[];
};

export type SystemSettingsSummaryCard = {
  id: string;
  label: string;
  value: string;
  detail: string;
  tone: 'neutral' | 'success' | 'warning';
};

export const SYSTEM_SETTINGS_SECTIONS: SystemSettingsSection[] = [
  {
    id: 'platform',
    label: 'Platform',
    title: 'Platform Identity',
    eyebrow: 'Control Plane',
    description: 'Environment, logging, public URLs, mail branding, and production guardrails.',
    fieldLabels: [
      'Log level',
      'Log format',
      'Environment',
      'Public URL',
      'Mail logo URL',
      'Mail website URL',
      'Mail support URL',
      'Mail footer address',
      'Require production gates',
    ],
    keywords: ['general', 'runtime', 'guardrails', 'branding'],
  },
  {
    id: 'execution',
    label: 'Execution',
    title: 'Execution Defaults',
    eyebrow: 'Runtime',
    description: 'Agent image, runner defaults, container lifecycle, timeouts, and runtime pools.',
    fieldLabels: [
      'Agent image',
      'Docker network name',
      'Default pipeline timeout',
      'LLM agent timeout',
      'Default runner ID',
      'Default runner scopes',
      'Default runner capacity',
      'Revoked runner IDs',
      'Auto-remove agent containers',
      'Runtime pools',
    ],
    keywords: ['runner', 'dispatcher', 'capacity', 'kubernetes', 'scheduling', 'pool', 'revoked'],
  },
  {
    id: 'networking',
    label: 'Networking',
    title: 'Service Discovery',
    eyebrow: 'Connectivity',
    description: 'Internal service URLs and dispatcher gRPC addressing used by runtime components.',
    fieldLabels: ['NopsAI API URL', 'GitBot API URL', 'Dispatcher gRPC address'],
    keywords: ['service', 'grpc', 'dispatcher', 'api'],
  },
  {
    id: 'notifications',
    label: 'Notifications',
    title: 'Mail Notifications',
    eyebrow: 'Delivery',
    description: 'Email sender identity, SMTP transport, credential reference, and test delivery.',
    fieldLabels: [
      'Enabled',
      'From address',
      'SMTP host',
      'SMTP port',
      'SMTP username',
      'Password credential ref',
      'StartTLS',
      'Test recipient',
    ],
    keywords: ['mail', 'smtp', 'email', 'delivery', 'credential'],
  },
  {
    id: 'source',
    label: 'Config Source',
    title: 'Global Config Repository',
    eyebrow: 'GitOps',
    description: 'Repository connection, sync status, drift review, and optional write-back branch.',
    fieldLabels: [
      'Provider',
      'Repository URL',
      'Credential reference',
      'Branch',
      'Base path',
      'Enabled',
      'Enable Git push',
      'Push branch',
    ],
    keywords: ['gitops', 'repository', 'drift', 'sync', 'write back', 'push'],
  },
];

export function filterSystemSettingsSections(
  query: string,
  sections: SystemSettingsSection[] = SYSTEM_SETTINGS_SECTIONS
): SystemSettingsSection[] {
  const normalizedQuery = normalizeQuery(query);
  if (!normalizedQuery) return sections;
  return sections.filter(section => {
    const searchable = [
      section.label,
      section.title,
      section.eyebrow,
      section.description,
      ...section.fieldLabels,
      ...section.keywords,
    ]
      .join(' ')
      .toLowerCase();
    return searchable.includes(normalizedQuery);
  });
}

export function getSystemSettingsSectionCount(sectionID: SystemSettingsSectionId): number {
  return SYSTEM_SETTINGS_SECTIONS.find(section => section.id === sectionID)?.fieldLabels.length ?? 0;
}

export function buildSystemSettingsSummary({
  config,
  envFilePath,
  globalConfigRepo,
  mailSettings,
  canViewGlobalConfigRepo,
}: {
  config: ConfigFormState;
  envFilePath: string;
  globalConfigRepo: ConfigRepository | null;
  mailSettings: NotificationMailSettingsRecord | null;
  canViewGlobalConfigRepo: boolean;
}): SystemSettingsSummaryCard[] {
  const runtimePoolCount = Object.keys(config.runtime_pools || {}).length;
  const environment = config.environment.trim() || 'Default';
  const logConfig = [config.log_level.trim() || 'default', config.log_format.trim() || 'default'].join(' / ');

  return [
    {
      id: 'environment',
      label: 'Environment',
      value: environment,
      detail: `Logging ${logConfig}`,
      tone: environment === 'production' ? 'success' : 'neutral',
    },
    {
      id: 'runtime-pools',
      label: 'Runtime Pools',
      value: String(runtimePoolCount),
      detail: runtimePoolCount === 1 ? 'Scheduling profile' : 'Scheduling profiles',
      tone: runtimePoolCount > 0 ? 'success' : 'neutral',
    },
    {
      id: 'mail',
      label: 'Mail',
      value: mailSettings ? (mailSettings.enabled ? 'Enabled' : 'Disabled') : 'Default',
      detail: mailSettings?.managed_by_config_repo ? 'GitOps managed' : mailSettings ? 'Database managed' : 'No override',
      tone: mailSettings?.enabled ? 'success' : 'neutral',
    },
    {
      id: 'gitops',
      label: 'GitOps',
      value: gitOpsSummaryValue(globalConfigRepo, canViewGlobalConfigRepo),
      detail: gitOpsSummaryDetail(globalConfigRepo, canViewGlobalConfigRepo, envFilePath),
      tone: globalConfigRepo?.enabled ? 'success' : canViewGlobalConfigRepo ? 'warning' : 'neutral',
    },
  ];
}

function gitOpsSummaryValue(globalConfigRepo: ConfigRepository | null, canViewGlobalConfigRepo: boolean) {
  if (!canViewGlobalConfigRepo) return 'Restricted';
  if (!globalConfigRepo) return 'Not connected';
  return globalConfigRepo.enabled ? 'Enabled' : 'Disabled';
}

function gitOpsSummaryDetail(globalConfigRepo: ConfigRepository | null, canViewGlobalConfigRepo: boolean, envFilePath: string) {
  if (!canViewGlobalConfigRepo) return 'Permission required';
  if (globalConfigRepo) return globalConfigRepo.last_sync_status || 'Not synced';
  return envFilePath.trim() ? `Env ${envFilePath.trim()}` : 'Database only';
}

function normalizeQuery(query: string) {
  return query.trim().replace(/\s+/g, ' ').toLowerCase();
}
