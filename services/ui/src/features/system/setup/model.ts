export type SetupCheck = {
  id: string;
  label: string;
  status: 'success' | 'warning' | 'error' | 'info' | string;
  message?: string;
  blocking: boolean;
};

export type SetupCounts = {
  users: number;
  pipelines: number;
  steps: number;
  triggers: number;
  groups: number;
  access_grants: number;
  llm_profiles: number;
  mcp_servers: number;
  mcp_profiles: number;
  knowledge_contexts: number;
  config_repositories: number;
};

export type SetupStatus = {
  completed: boolean;
  completed_at?: string;
  counts: SetupCounts;
  checks: SetupCheck[];
  github: {
    webhook_url?: string;
    git_bot_service_url?: string;
    nopsai_api_url?: string;
    required_events?: string[];
    required_permissions?: Record<string, string>;
  };
  global_config_repo?: {
    repo_url: string;
    branch: string;
    base_path?: string;
    enabled: boolean;
    last_sync_status?: string;
    last_sync_message?: string;
  };
};

export type SetupTemplates = {
  profile: string;
  files: Record<string, string>;
};

export type TemporaryCredential = {
  sub: string;
  email?: string;
  temporary_password?: string;
  role?: string;
};

export type BootstrapResponse = {
  status: SetupStatus;
  details?: Record<string, number>;
  generated_secrets?: string[];
  requires_restart?: boolean;
  temporary_credentials?: TemporaryCredential[];
  messages?: string[];
  warnings?: string[];
};

export type RepositoryGroupDraft = {
  id: string;
  name: string;
  repositoriesText: string;
};

export type UserDraft = {
  id: string;
  email: string;
  password: string;
  role: 'owner' | 'developer' | 'viewer';
  group: string;
};

export type SetupStepID = 'readiness' | 'runtime' | 'gitops' | 'github' | 'repositories' | 'ai' | 'users' | 'review';
export type RuntimeImplementation = 'docker' | 'kubernetes';
export type GitHubPrivateKeyMode = 'path' | 'inline';

export type RuntimeDefaults = {
  nopsaiAPIURL: string;
  gitBotServiceURL: string;
};

export type RuntimeEnvSection = {
  title: string;
  fileName: string;
  lines: string[];
};

export type SetupBootstrapRequest = {
  profile: 'team';
  generate_secrets: boolean;
  seed_starter_database: boolean;
  seed_llm_profile: boolean;
  mcp_examples: boolean;
  production_acknowledged: boolean;
  sync_config_repository: boolean;
  config_repository: {
    repo_url: string;
    branch: string;
    base_path: string;
    enabled: boolean;
  };
  repository_groups: Array<{ name: string; repositories: string[] }>;
  repositories: string[];
  llm_profile: {
    name: string;
    provider: string;
    model: string;
    base_url: string;
    api_key_secret: string;
    api_key_value: string;
    allowed_scopes: string[];
  };
  users: Array<{
    sub: string;
    email: string;
    role: UserDraft['role'];
    password: string;
    group: string;
  }>;
};

export const WIZARD_STEPS: Array<{ id: SetupStepID; label: string; required: boolean }> = [
  { id: 'readiness', label: 'Readiness', required: true },
  { id: 'runtime', label: 'Runtime', required: true },
  { id: 'gitops', label: 'GitOps', required: false },
  { id: 'github', label: 'GitHub App', required: false },
  { id: 'repositories', label: 'Groups', required: false },
  { id: 'ai', label: 'AI', required: false },
  { id: 'users', label: 'Users', required: false },
  { id: 'review', label: 'Output', required: true },
];

export const REVIEW_STEP_INDEX = WIZARD_STEPS.findIndex(step => step.id === 'review');
export const LLM_SKIP_WARNING = 'LLM profile setup was skipped. Pipelines with AI-enabled goal tasks may not work until an LLM profile is configured.';

export function makeID(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

export function initialRepositoryGroups(): RepositoryGroupDraft[] {
  return [
    { id: makeID('group'), name: 'platform', repositoriesText: '' },
    { id: makeID('group'), name: 'applications', repositoriesText: '' },
  ];
}

export function parseRepositories(value: string): string[] {
  return Array.from(
    new Set(
      value
        .split(/[\n,]+/)
        .map(item => item.trim())
        .filter(Boolean)
    )
  ).sort();
}

export function normalizeGroupName(value: string): string {
  return value.trim().replace(/^\/+|\/+$/g, '').replace(/[\\/\s]+/g, '-');
}

export function statusClasses(status: string): string {
  switch (status) {
    case 'success':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300';
    case 'error':
      return 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300';
    case 'warning':
      return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300';
    default:
      return 'border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300';
  }
}

export function deriveGitBotBaseURL(webhookURL?: string): string {
  const value = (webhookURL || '').trim();
  if (!value) return 'https://nopsai.example.com/git-bot';
  return value.replace(/\/webhook\/?$/, '');
}

export function defaultSecretName(provider: string): string {
  return provider === 'gemini' ? 'GEMINI_API_KEY' : 'LLM_API_KEY';
}

export function runtimeDefaults(runtime: RuntimeImplementation): RuntimeDefaults {
  if (runtime === 'kubernetes') {
    return {
      nopsaiAPIURL: 'http://nopsai.nopsai.svc.cluster.local:8080',
      gitBotServiceURL: 'http://nopsai-git-bot.nopsai.svc.cluster.local:8081',
    };
  }
  return {
    nopsaiAPIURL: 'http://nopsai:8080',
    gitBotServiceURL: 'http://git-bot:8081',
  };
}

export function isLikelyPublicURL(value?: string): boolean {
  const trimmed = (value || '').trim();
  if (!trimmed) return false;
  try {
    const parsed = new URL(trimmed);
    const host = parsed.hostname.toLowerCase();
    return host !== 'localhost' && host !== '127.0.0.1' && host !== 'git-bot' && !host.endsWith('.svc.cluster.local');
  } catch {
    return false;
  }
}

export function secretPlaceholder(provided: boolean, fallback: string): string {
  return provided ? '<provided in wizard; store as a secret value>' : fallback;
}
