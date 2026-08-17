export type GitHubAccountType = 'organization' | 'user' | '';

export type GitHubAppInstallationRepository = {
  id: number;
  full_name: string;
  owner: string;
  name: string;
  private: boolean;
  default_branch?: string;
  access?: string;
  used_by_nopsai: boolean;
};

export type GitHubAppInstallation = {
  installation_id: string;
  account_login: string;
  account_type: GitHubAccountType;
  enabled: boolean;
  repository_selection?: string;
  accessible_repositories: number;
  connected_triggers: number;
  last_verified_at?: string;
  last_repository_refresh_at?: string;
  last_error?: string;
  status: string;
  repositories?: GitHubAppInstallationRepository[];
};

export type GitHubAppResource = {
  provider: 'github';
  app_id: string;
  app_slug: string;
  private_key_credential_ref: string;
  webhook_credential_ref: string;
  /** Stored delivery address; empty while none is configured. */
  webhook_url: string;
  /** Effective delivery address: the stored one, or one derived from the public URL. */
  webhook_endpoint: string;
  installations: GitHubAppInstallation[];
};

export type GitHubAppConnectTarget = 'organization' | 'personal';

export type GitHubAppConnectFormState = {
  target: GitHubAppConnectTarget;
  organization: string;
  appName: string;
};

/**
 * GitHub creates an App from a manifest only when the browser submits it as a
 * form, so the API hands back the target URL and the manifest to post there.
 */
export type GitHubAppRegistrationStart = {
  state: string;
  post_url: string;
  manifest: string;
  app_name: string;
  webhook_endpoint: string;
  expires_at: string;
};

export type GitHubAppInstallStart = {
  state: string;
  install_url: string;
  expires_at: string;
};

export type GitHubAppFormState = {
  appID: string;
  privateKeyCredentialRef: string;
  webhookCredentialRef: string;
  /**
   * Where GitHub delivers webhooks. This is the only address GitHub's servers
   * fetch, so it points at whatever tunnel or proxy fronts git-bot, while the
   * redirect back into NopsAI uses the operator's own browser address.
   */
  webhookURL: string;
};

export type GitHubAppInstallationFormState = {
  installationID: string;
  accountLogin: string;
  accountType: GitHubAccountType;
  enabled: boolean;
};

export type GitHubAppMetrics = {
  installations: number;
  enabled: number;
  disabled: number;
  repositories: number;
  connectedTriggers: number;
};

export const emptyGitHubApp: GitHubAppResource = {
  provider: 'github',
  app_id: '',
  app_slug: '',
  private_key_credential_ref: '',
  webhook_credential_ref: '',
  webhook_url: '',
  webhook_endpoint: '',
  installations: [],
};

export const emptyGitHubAppConnectForm: GitHubAppConnectFormState = {
  target: 'organization',
  organization: '',
  appName: '',
};

export const emptyGitHubAppForm: GitHubAppFormState = {
  appID: '',
  privateKeyCredentialRef: '',
  webhookCredentialRef: '',
  webhookURL: '',
};

export const emptyGitHubAppInstallationForm: GitHubAppInstallationFormState = {
  installationID: '',
  accountLogin: '',
  accountType: 'organization',
  enabled: true,
};

export function normalizeGitHubAppPayload(value: unknown): GitHubAppResource {
  const record = readObject(value);
  if (!record) return { ...emptyGitHubApp };
  return {
    provider: 'github',
    app_id: readString(record.app_id),
    app_slug: readString(record.app_slug),
    private_key_credential_ref: readString(record.private_key_credential_ref),
    webhook_credential_ref: readString(record.webhook_credential_ref),
    webhook_url: readString(record.webhook_url),
    webhook_endpoint: readString(record.webhook_endpoint),
    installations: readArray(record.installations)
      .map(normalizeGitHubAppInstallation)
      .filter(installation => installation.installation_id),
  };
}

export function normalizeGitHubAppInstallation(value: unknown): GitHubAppInstallation {
  const record = readObject(value) || {};
  const enabled = readBool(record.enabled, true);
  return {
    installation_id: readString(record.installation_id),
    account_login: readString(record.account_login),
    account_type: normalizeGitHubAccountType(readString(record.account_type)),
    enabled,
    repository_selection: readString(record.repository_selection) || undefined,
    accessible_repositories: readInt(record.accessible_repositories),
    connected_triggers: readInt(record.connected_triggers),
    last_verified_at: readString(record.last_verified_at) || undefined,
    last_repository_refresh_at: readString(record.last_repository_refresh_at) || undefined,
    last_error: readString(record.last_error) || undefined,
    status: readString(record.status) || (enabled ? 'connected' : 'disabled'),
    repositories: readArray(record.repositories).map(normalizeGitHubAppInstallationRepository),
  };
}

export function normalizeGitHubAppInstallationRepository(value: unknown): GitHubAppInstallationRepository {
  const record = readObject(value) || {};
  return {
    id: readInt(record.id),
    full_name: readString(record.full_name),
    owner: readString(record.owner),
    name: readString(record.name),
    private: readBool(record.private, false),
    default_branch: readString(record.default_branch) || undefined,
    access: readString(record.access) || undefined,
    used_by_nopsai: readBool(record.used_by_nopsai, false),
  };
}

export function gitHubAppForm(app: GitHubAppResource): GitHubAppFormState {
  return {
    appID: app.app_id,
    privateKeyCredentialRef: app.private_key_credential_ref,
    webhookCredentialRef: app.webhook_credential_ref,
    webhookURL: app.webhook_url,
  };
}

export function gitHubAppInstallationForm(installation?: GitHubAppInstallation): GitHubAppInstallationFormState {
  if (!installation) return { ...emptyGitHubAppInstallationForm };
  return {
    installationID: installation.installation_id,
    accountLogin: installation.account_login,
    accountType: installation.account_type || 'organization',
    enabled: installation.enabled,
  };
}

export function gitHubAppPayloadFromForm(
  form: GitHubAppFormState,
  installations: readonly GitHubAppInstallation[]
): GitHubAppResource {
  validateGitHubAppForm(form);
  return {
    provider: 'github',
    app_id: form.appID.trim(),
    app_slug: '',
    private_key_credential_ref: form.privateKeyCredentialRef.trim(),
    webhook_credential_ref: form.webhookCredentialRef.trim(),
    webhook_url: form.webhookURL.trim(),
    webhook_endpoint: '',
    installations: installations.map(installation => ({
      ...installation,
      repositories: undefined,
    })),
  };
}

export function gitHubAppInstallationPayloadFromForm(
  form: GitHubAppInstallationFormState
): GitHubAppInstallation {
  const installationID = form.installationID.trim();
  const accountLogin = form.accountLogin.trim();
  if (!/^\d+$/.test(installationID)) {
    throw new Error('Installation ID must be a positive number.');
  }
  if (!/^[A-Za-z0-9-]{1,100}$/.test(accountLogin)) {
    throw new Error('Account login must use a valid GitHub owner name.');
  }
  return {
    installation_id: installationID,
    account_login: accountLogin,
    account_type: normalizeGitHubAccountType(form.accountType) || 'organization',
    enabled: form.enabled,
    accessible_repositories: 0,
    connected_triggers: 0,
    status: form.enabled ? 'connected' : 'disabled',
  };
}

export function gitHubAppConnectPayload(
  form: GitHubAppConnectFormState,
  rawWebhookURL: string
): {
  target: GitHubAppConnectTarget;
  organization: string;
  app_name: string;
  webhook_url: string;
} {
  const organization = form.organization.trim();
  if (form.target === 'organization' && !/^[A-Za-z0-9-]{1,100}$/.test(organization)) {
    throw new Error('Organization must use a valid GitHub organization name.');
  }
  const webhookURL = rawWebhookURL.trim();
  if (!webhookURL) {
    throw new Error('Webhook URL is required so GitHub can deliver events to git-bot.');
  }
  if (!isAbsoluteHTTPURL(webhookURL)) {
    throw new Error('Webhook URL must be an absolute http or https URL.');
  }
  return {
    target: form.target,
    organization: form.target === 'organization' ? organization : '',
    app_name: form.appName.trim(),
    webhook_url: webhookURL,
  };
}

/**
 * GitHub's servers have to fetch the webhook URL, so an address that only
 * resolves inside the deployment silently drops every delivery.
 */
export function gitHubWebhookURLWarning(rawURL: string): string {
  const value = rawURL.trim();
  if (!value || !isAbsoluteHTTPURL(value)) return '';
  const host = readURLHost(value);
  const unreachable = host === 'localhost' ||
    host === '127.0.0.1' ||
    host === '::1' ||
    host === 'git-bot' ||
    host.endsWith('.svc.cluster.local') ||
    host.endsWith('.internal');
  return unreachable
    ? `GitHub cannot reach ${host}. Use the public address of the tunnel or proxy in front of git-bot.`
    : '';
}

function isAbsoluteHTTPURL(value: string): boolean {
  try {
    const parsed = new URL(value);
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && Boolean(parsed.host);
  } catch {
    return false;
  }
}

function readURLHost(value: string): string {
  try {
    return new URL(value).hostname.toLowerCase();
  } catch {
    return '';
  }
}

export type GitHubAppCallbackResult = {
  tone: 'success' | 'error';
  message: string;
};

/**
 * GitHub returns the operator to the Git Apps page after the App is created or
 * installed. The outcome travels in the query string because the redirect comes
 * from GitHub, not from the UI.
 */
export function readGitHubAppCallbackResult(search: string): GitHubAppCallbackResult | null {
  const params = new URLSearchParams(search);
  const error = (params.get('github_app_error') || '').trim();
  if (error) return { tone: 'error', message: error };
  switch ((params.get('github_app') || '').trim()) {
    case 'created':
      return { tone: 'success', message: 'GitHub App created. Install it on an account to finish.' };
    case 'installed':
      return { tone: 'success', message: 'GitHub App installed and registered.' };
    case 'requested':
      return {
        tone: 'success',
        message: 'Installation requested. A GitHub organization owner has to approve it.',
      };
    default:
      return null;
  }
}

export function buildGitHubAppMetrics(app: GitHubAppResource): GitHubAppMetrics {
  return app.installations.reduce<GitHubAppMetrics>(
    (metrics, installation) => ({
      installations: metrics.installations + 1,
      enabled: metrics.enabled + (installation.enabled ? 1 : 0),
      disabled: metrics.disabled + (installation.enabled ? 0 : 1),
      repositories: metrics.repositories + Math.max(0, installation.accessible_repositories),
      connectedTriggers: metrics.connectedTriggers + Math.max(0, installation.connected_triggers),
    }),
    { installations: 0, enabled: 0, disabled: 0, repositories: 0, connectedTriggers: 0 }
  );
}

export function filterGitHubAppInstallations(
  installations: readonly GitHubAppInstallation[],
  query: string
): GitHubAppInstallation[] {
  const term = query.trim().toLowerCase();
  if (!term) return [...installations];
  return installations.filter(installation => [
    installation.installation_id,
    installation.account_login,
    installation.account_type,
    installation.status,
    installation.repository_selection,
    installation.last_error,
  ].join(' ').toLowerCase().includes(term));
}

export function installationDisplayName(installation: GitHubAppInstallation): string {
  return installation.account_login || installation.installation_id;
}

export function gitHubInstallationStatusLabel(installation: GitHubAppInstallation): string {
  if (!installation.enabled) return 'Disabled';
  if (installation.last_error) return 'Error';
  if (!installation.account_login) return 'Account needed';
  return 'Connected';
}

export function gitHubInstallationStatusTone(installation: GitHubAppInstallation): 'ok' | 'warning' | 'error' | 'muted' {
  if (!installation.enabled) return 'muted';
  if (installation.last_error) return 'error';
  if (!installation.account_login) return 'warning';
  return 'ok';
}

export function formatGitHubAppDate(value?: string): string {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function normalizeGitHubAccountType(value: string): GitHubAccountType {
  const normalized = value.trim().toLowerCase();
  if (normalized === 'org' || normalized === 'organization') return 'organization';
  if (normalized === 'user') return 'user';
  return '';
}

function validateGitHubAppForm(form: GitHubAppFormState) {
  const appID = form.appID.trim();
  if (appID && !/^\d+$/.test(appID)) {
    throw new Error('App ID must be a positive number.');
  }
  const webhookURL = form.webhookURL.trim();
  if (webhookURL && !isAbsoluteHTTPURL(webhookURL)) {
    throw new Error('Webhook URL must be an absolute http or https URL.');
  }
  for (const [label, value] of [
    ['Private key credential ref', form.privateKeyCredentialRef],
    ['Webhook credential ref', form.webhookCredentialRef],
  ] as const) {
    const ref = value.trim();
    if (ref && !isCredentialReference(ref)) {
      throw new Error(`${label} must use credential://namespace/name.`);
    }
  }
}

function isCredentialReference(value: string): boolean {
  return /^credential:\/\/[a-z0-9][a-z0-9_.-]*(?:\/[A-Za-z0-9_.@:-]+)+$/.test(value);
}

function readObject(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' ? value as Record<string, unknown> : null;
}

function readArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function readBool(value: unknown, fallback: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback;
}

function readInt(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) return Math.max(0, Math.trunc(value));
  if (typeof value === 'string' && /^\d+$/.test(value.trim())) return Number(value.trim());
  return 0;
}
