export const GIT_WEBHOOK_PROVIDERS = ['generic', 'gitlab', 'bitbucket', 'gitea'] as const;
export const GIT_WEBHOOK_AUTH_MODES = ['hmac', 'static_token', 'none'] as const;

export type GitWebhookProvider = (typeof GIT_WEBHOOK_PROVIDERS)[number];
export type GitWebhookAuthMode = (typeof GIT_WEBHOOK_AUTH_MODES)[number];

export type GitWebhookSource = {
  id: string;
  name: string;
  description: string;
  provider: GitWebhookProvider;
  enabled: boolean;
  auth_mode: GitWebhookAuthMode;
  credential_ref?: string;
  repository_allowlist: string[];
  rate_limit: Record<string, unknown>;
  created_by?: string;
  created_at?: string;
  updated_at?: string;
  last_used_at?: string;
  source?: string;
  config_source_path?: string;
  managed_by_config_repo?: boolean;
};

export type GitWebhookDelivery = {
  id: string;
  source_id: string;
  delivery_id: string;
  provider: string;
  event_type: string;
  repository_full_name: string;
  status: string;
  run_ids: string[];
  error?: string;
  source_ip?: string;
  received_at: string;
  completed_at?: string;
};

export type GitWebhookSourceFormState = {
  id: string;
  name: string;
  description: string;
  provider: GitWebhookProvider;
  enabled: boolean;
  authMode: GitWebhookAuthMode;
  credentialRef: string;
  repositoryAllowlistText: string;
  rateLimitPerMinute: string;
};

export type GitWebhookSourceRequest = {
  id: string;
  name: string;
  description: string;
  provider: GitWebhookProvider;
  enabled: boolean;
  auth_mode: GitWebhookAuthMode;
  credential_ref?: string;
  repository_allowlist: string[];
  rate_limit: Record<string, number>;
};

export const emptyGitWebhookSourceForm: GitWebhookSourceFormState = {
  id: '',
  name: '',
  description: '',
  provider: 'generic',
  enabled: true,
  authMode: 'hmac',
  credentialRef: '',
  repositoryAllowlistText: '',
  rateLimitPerMinute: '',
};

export function gitWebhookSourceForm(source?: GitWebhookSource): GitWebhookSourceFormState {
  if (!source) return { ...emptyGitWebhookSourceForm };
  const perMinute = readPositiveInteger(source.rate_limit.per_minute);
  return {
    id: source.id,
    name: source.name,
    description: source.description,
    provider: source.provider,
    enabled: source.enabled,
    authMode: source.auth_mode,
    credentialRef: source.credential_ref || '',
    repositoryAllowlistText: source.repository_allowlist.join('\n'),
    rateLimitPerMinute: perMinute ? String(perMinute) : '',
  };
}

export function gitWebhookSourceRequest(form: GitWebhookSourceFormState): GitWebhookSourceRequest {
  const id = form.id.trim();
  const name = form.name.trim() || id;
  if (!id) throw new Error('Source ID is required.');
  if (!/^[A-Za-z0-9_.-]{1,160}$/.test(id)) {
    throw new Error('Source ID may only use letters, numbers, dots, underscores, and hyphens.');
  }

  const repositoryAllowlist = [...new Set(
    uniqueLines(form.repositoryAllowlistText).map(value =>
      value.toLowerCase().replace(/^\/+|\/+$/g, '')
    )
  )];
  if (!repositoryAllowlist.length || repositoryAllowlist.some(value => !value.includes('/'))) {
    throw new Error('Add at least one owner/repository allowlist pattern.');
  }

  const credentialRef = form.credentialRef.trim();
  if (form.authMode !== 'none' && !isCredentialReference(credentialRef)) {
    throw new Error('Credential reference must use credential://namespace/name.');
  }

  const rateLimit = Number(form.rateLimitPerMinute);
  if (form.rateLimitPerMinute.trim() && (!Number.isInteger(rateLimit) || rateLimit <= 0)) {
    throw new Error('Rate limit must be a positive whole number.');
  }

  return {
    id,
    name,
    description: form.description.trim(),
    provider: form.provider,
    enabled: form.enabled,
    auth_mode: form.authMode,
    credential_ref: form.authMode === 'none' ? undefined : credentialRef,
    repository_allowlist: repositoryAllowlist,
    rate_limit: rateLimit > 0 ? { per_minute: rateLimit } : {},
  };
}

export function sourceStatusLabel(source: GitWebhookSource): string {
  if (!source.enabled) return 'Disabled';
  if (source.auth_mode !== 'none' && !source.credential_ref) return 'Credential required';
  return 'Enabled';
}

export function deliveryStatusClass(status: string): string {
  switch (status.trim().toLowerCase()) {
    case 'processed':
      return 'runner-pill--ok';
    case 'partial':
    case 'pending':
      return 'runner-pill--warning';
    case 'failed':
      return 'runner-pill--error';
    default:
      return 'runner-pill--muted';
  }
}

export function formatGitWebhookDate(value?: string): string {
  if (!value) return 'Never';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

function uniqueLines(value: string): string[] {
  return [...new Set(value.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean))];
}

function readPositiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : 0;
}

function isCredentialReference(value: string): boolean {
  const match = /^credential:\/\/([^/]+)\/(.+)$/.exec(value);
  if (!match) return false;
  const segmentPattern = /^[a-z0-9][a-z0-9._-]*$/;
  return segmentPattern.test(match[1]) &&
    match[2].split('/').every(segment => segmentPattern.test(segment));
}
