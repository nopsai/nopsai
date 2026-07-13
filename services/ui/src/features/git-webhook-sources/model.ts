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
  team_path?: string;
  run_team_path?: string;
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
  teamPath: string;
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
  team_path: string;
  auth_mode: GitWebhookAuthMode;
  credential_ref?: string;
  repository_allowlist: string[];
  rate_limit: Record<string, number>;
};

export type GitWebhookSourceMetrics = {
  total: number;
  enabled: number;
  gitManaged: number;
  secured: number;
};

export type GitWebhookSourceTreeItem = {
  id: string;
  label: string;
  path: string;
  source?: string;
};

export const emptyGitWebhookSourceForm: GitWebhookSourceFormState = {
  id: '',
  name: '',
  description: '',
  provider: 'generic',
  enabled: true,
  authMode: 'hmac',
  teamPath: 'root',
  credentialRef: '',
  repositoryAllowlistText: '',
  rateLimitPerMinute: '',
};

export function buildGitWebhookSourceMetrics(
  sources: readonly GitWebhookSource[]
): GitWebhookSourceMetrics {
  return sources.reduce<GitWebhookSourceMetrics>(
    (metrics, source) => ({
      total: metrics.total + 1,
      enabled: metrics.enabled + (source.enabled ? 1 : 0),
      gitManaged: metrics.gitManaged + (source.managed_by_config_repo ? 1 : 0),
      secured: metrics.secured + (source.auth_mode !== 'none' && source.credential_ref ? 1 : 0),
    }),
    { total: 0, enabled: 0, gitManaged: 0, secured: 0 }
  );
}

export function filterGitWebhookSources(
  sources: readonly GitWebhookSource[],
  query: string
): GitWebhookSource[] {
  const term = query.trim().toLowerCase();
  if (!term) return [...sources];
  return sources.filter(source => [
    source.id,
    source.name,
    source.description,
    source.provider,
    source.auth_mode,
    gitWebhookSourceTeamLabel(source),
    gitWebhookSourceTeamPath(source),
    source.credential_ref,
    source.source,
    sourceStatusLabel(source),
    ...source.repository_allowlist,
  ].join(' ').toLowerCase().includes(term));
}

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
    teamPath: gitWebhookSourceTeamPath(source) || 'root',
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
    team_path: normalizeGitWebhookSourceTeamPath(form.teamPath) || 'root',
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

export function gitWebhookSourceTeamPath(source: GitWebhookSource): string {
  return normalizeGitWebhookSourceTeamPath(source.team_path || source.run_team_path);
}

export function gitWebhookSourceTeamLabel(source: GitWebhookSource): string {
  return gitWebhookSourceTeamPath(source) || 'Global';
}

export function gitWebhookSourceBelongsToTeam(
  source: GitWebhookSource,
  activeTeamPath: string
): boolean {
  const active = normalizeGitWebhookSourceTeamPath(activeTeamPath);
  if (!active) return true;
  const teamPath = gitWebhookSourceTeamPath(source);
  return teamPath === active || teamPath.startsWith(`${active}/`);
}

export function buildGitWebhookSourceTreeItems(
  sources: readonly GitWebhookSource[]
): GitWebhookSourceTreeItem[] {
  return sources
    .map(source => ({
      id: source.id,
      label: source.name || source.id,
      path: gitWebhookSourceTeamPath(source),
      source: source.source,
    }))
    .sort((left, right) => left.label.localeCompare(right.label, undefined, { sensitivity: 'base' }));
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

function normalizeGitWebhookSourceTeamPath(value?: string): string {
  return String(value || '')
    .trim()
    .replace(/\/+/g, '/')
    .replace(/^\/+|\/+$/g, '')
    .replace(/^root$/i, '');
}
