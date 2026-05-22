import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  Download,
  FileText,
  FolderTree,
  GitBranch,
  Github,
  Info,
  KeyRound,
  PlayCircle,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Users,
} from 'lucide-react';
import { buildApiUrl } from '../lib/api';

type SetupCheck = {
  id: string;
  label: string;
  status: 'success' | 'warning' | 'error' | 'info' | string;
  message?: string;
  blocking: boolean;
};

type SetupCounts = {
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

type SetupStatus = {
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

type SetupTemplates = {
  profile: string;
  files: Record<string, string>;
};

type TemporaryCredential = {
  sub: string;
  email?: string;
  temporary_password?: string;
  role?: string;
};

type BootstrapResponse = {
  status: SetupStatus;
  details?: Record<string, number>;
  generated_secrets?: string[];
  requires_restart?: boolean;
  temporary_credentials?: TemporaryCredential[];
  messages?: string[];
  warnings?: string[];
};

type RepositoryGroupDraft = {
  id: string;
  name: string;
  repositoriesText: string;
};

type UserDraft = {
  id: string;
  email: string;
  password: string;
  role: 'owner' | 'developer' | 'viewer';
  group: string;
};

type SetupStepID = 'readiness' | 'runtime' | 'gitops' | 'github' | 'repositories' | 'ai' | 'users' | 'review';
type RuntimeImplementation = 'docker' | 'kubernetes';
type GitHubPrivateKeyMode = 'path' | 'inline';

type RuntimeDefaults = {
  nopsaiAPIURL: string;
  gitBotServiceURL: string;
};

type RuntimeEnvSection = {
  title: string;
  fileName: string;
  lines: string[];
};

const WIZARD_STEPS: Array<{ id: SetupStepID; label: string; required: boolean }> = [
  { id: 'readiness', label: 'Readiness', required: true },
  { id: 'runtime', label: 'Runtime', required: true },
  { id: 'gitops', label: 'GitOps', required: false },
  { id: 'github', label: 'GitHub App', required: false },
  { id: 'repositories', label: 'Groups', required: false },
  { id: 'ai', label: 'AI', required: false },
  { id: 'users', label: 'Users', required: false },
  { id: 'review', label: 'Output', required: true },
];

const REVIEW_STEP_INDEX = WIZARD_STEPS.findIndex(step => step.id === 'review');
const LLM_SKIP_WARNING = 'LLM profile setup was skipped. Pipelines with AI-enabled goal tasks may not work until an LLM profile is configured.';

async function fetchJson(path: string, init?: RequestInit): Promise<unknown> {
  const response = await fetch(buildApiUrl(path), init);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed (${response.status})`);
  }
  return response.json();
}

function makeID(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function initialRepositoryGroups(): RepositoryGroupDraft[] {
  return [
    { id: makeID('group'), name: 'platform', repositoriesText: '' },
    { id: makeID('group'), name: 'applications', repositoriesText: '' },
  ];
}

function parseRepositories(value: string): string[] {
  return Array.from(
    new Set(
      value
        .split(/[\n,]+/)
        .map(item => item.trim())
        .filter(Boolean)
    )
  ).sort();
}

function normalizeGroupName(value: string): string {
  return value.trim().replace(/^\/+|\/+$/g, '').replace(/[\\/\s]+/g, '-');
}

function statusClasses(status: string): string {
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

function statusIcon(status: string) {
  if (status === 'success') return <CheckCircle2 className="h-4 w-4" />;
  if (status === 'error') return <AlertTriangle className="h-4 w-4" />;
  if (status === 'warning') return <AlertTriangle className="h-4 w-4" />;
  return <Info className="h-4 w-4" />;
}

function deriveGitBotBaseURL(webhookURL?: string): string {
  const value = (webhookURL || '').trim();
  if (!value) return 'https://nopsai.example.com/git-bot';
  return value.replace(/\/webhook\/?$/, '');
}

function defaultSecretName(provider: string): string {
  return provider === 'gemini' ? 'GEMINI_API_KEY' : 'LLM_API_KEY';
}

function runtimeDefaults(runtime: RuntimeImplementation): RuntimeDefaults {
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

function isLikelyPublicURL(value?: string): boolean {
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

function generateBrowserSecret(bytes = 32): string {
  const buffer = new Uint8Array(bytes);
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(buffer);
  } else {
    for (let index = 0; index < buffer.length; index += 1) {
      buffer[index] = Math.floor(Math.random() * 256);
    }
  }
  const binary = Array.from(buffer, byte => String.fromCharCode(byte)).join('');
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '');
}

function downloadTextFile(fileName: string, content: string, mimeType = 'text/plain;charset=utf-8') {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function secretPlaceholder(provided: boolean, fallback: string): string {
  return provided ? '<provided in wizard; store as a secret value>' : fallback;
}

function StepIntro({ title, children, icon }: { title: string; children: ReactNode; icon: ReactNode }) {
  return (
    <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
      <div className="flex items-center gap-2 text-sm font-semibold">
        {icon}
        {title}
      </div>
      <div className="mt-2 text-sm leading-6 text-[var(--text-secondary)]">{children}</div>
    </div>
  );
}

function WarningCallout({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm leading-6 text-amber-700 dark:text-amber-300">
      <div className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
        <div>{children}</div>
      </div>
    </div>
  );
}

function SetupWizard({ canManage }: { canManage: boolean }) {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [generateServiceSecrets, setGenerateServiceSecrets] = useState(true);
  const [runtimeImplementation, setRuntimeImplementation] = useState<RuntimeImplementation>('docker');
  const [nopsaiAPIURL, setNopsaiAPIURL] = useState(runtimeDefaults('docker').nopsaiAPIURL);
  const [gitBotServiceURL, setGitBotServiceURL] = useState(runtimeDefaults('docker').gitBotServiceURL);
  const [gitBotPublicURL, setGitBotPublicURL] = useState('');
  const [repoURL, setRepoURL] = useState('');
  const [branch, setBranch] = useState('main');
  const [basePath, setBasePath] = useState('');
  const [syncConfigRepository, setSyncConfigRepository] = useState(false);
  const [githubEnabled, setGithubEnabled] = useState(true);
  const [githubAppID, setGithubAppID] = useState('');
  const [githubInstallationID, setGithubInstallationID] = useState('');
  const [githubPrivateKeyMode, setGithubPrivateKeyMode] = useState<GitHubPrivateKeyMode>('path');
  const [githubPrivateKeyPath, setGithubPrivateKeyPath] = useState('/run/secrets/nopsai-github-app.pem');
  const [githubPrivateKey, setGithubPrivateKey] = useState('');
  const [githubWebhookSecret, setGithubWebhookSecret] = useState('');
  const [repositoryEnabled, setRepositoryEnabled] = useState(true);
  const [repositoryGroups, setRepositoryGroups] = useState<RepositoryGroupDraft[]>(() => initialRepositoryGroups());
  const [aiEnabled, setAIEnabled] = useState(true);
  const [llmProvider, setLLMProvider] = useState('lmstudio');
  const [llmModel, setLLMModel] = useState('qwen3-coder');
  const [llmBaseURL, setLLMBaseURL] = useState('http://lmstudio:1234');
  const [llmAPIKeySecretName, setLLMAPIKeySecretName] = useState(defaultSecretName('lmstudio'));
  const [llmAPIKey, setLLMAPIKey] = useState('');
  const [mcpExamples, setMCPExamples] = useState(false);
  const [usersEnabled, setUsersEnabled] = useState(true);
  const [users, setUsers] = useState<UserDraft[]>([]);
  const [templates, setTemplates] = useState<SetupTemplates | null>(null);
  const [templateLoading, setTemplateLoading] = useState(false);
  const [selectedTemplatePath, setSelectedTemplatePath] = useState('');
  const [bootstrapResult, setBootstrapResult] = useState<BootstrapResponse | null>(null);
  const [downloadingGitOpsZip, setDownloadingGitOpsZip] = useState(false);
  const [wizardStepIndex, setWizardStepIndex] = useState(0);

  const currentWizardStep = WIZARD_STEPS[Math.min(wizardStepIndex, WIZARD_STEPS.length - 1)];
  const normalizedRepositoryGroups = useMemo(
    () =>
      repositoryEnabled
        ? repositoryGroups
            .map(group => ({
              name: normalizeGroupName(group.name),
              repositories: parseRepositories(group.repositoriesText),
            }))
            .filter(group => group.name)
        : [],
    [repositoryEnabled, repositoryGroups]
  );
  const repositories = useMemo(
    () => Array.from(new Set(normalizedRepositoryGroups.flatMap(group => group.repositories))).sort(),
    [normalizedRepositoryGroups]
  );
  const groupOptions = useMemo(() => normalizedRepositoryGroups.map(group => group.name), [normalizedRepositoryGroups]);
  const userGroupOptions = useMemo(() => (groupOptions.length > 0 ? groupOptions : ['']), [groupOptions]);
  const templatePaths = useMemo(() => (templates ? Object.keys(templates.files).sort() : []), [templates]);
  const selectedTemplate = selectedTemplatePath && templates ? templates.files[selectedTemplatePath] : '';
  const requiredHealthErrors = (status?.checks || []).filter(check => check.blocking && check.status === 'error');
  const llmSecretName = llmAPIKeySecretName.trim() || defaultSecretName(llmProvider);
  const currentRuntimeDefaults = runtimeDefaults(runtimeImplementation);
  const gitBotPublicBaseURL = gitBotPublicURL.trim().replace(/\/+$/, '') || 'https://<your-ngrok-or-git-bot-domain>';
  const gitBotWebhookURL = `${gitBotPublicBaseURL}/webhook`;

  useEffect(() => {
    if (!templates || selectedTemplatePath) return;
    setSelectedTemplatePath(templatePaths[0] || '');
  }, [selectedTemplatePath, templatePaths, templates]);

  useEffect(() => {
    setLLMAPIKeySecretName(current => current || defaultSecretName(llmProvider));
    if (llmProvider === 'gemini') {
      setLLMModel(current => (current === 'qwen3-coder' || !current.trim() ? 'gemini-2.5-flash' : current));
      setLLMBaseURL('');
    } else {
      setLLMModel(current => (current.startsWith('gemini-') || !current.trim() ? 'qwen3-coder' : current));
      setLLMBaseURL(current => current || 'http://lmstudio:1234');
    }
  }, [llmProvider]);

  const updateRuntimeImplementation = (nextRuntime: RuntimeImplementation) => {
    if (nextRuntime === 'kubernetes') return;
    const previousDefaults = runtimeDefaults(runtimeImplementation);
    const nextDefaults = runtimeDefaults(nextRuntime);
    setRuntimeImplementation(nextRuntime);
    setNopsaiAPIURL(current => (!current.trim() || current === previousDefaults.nopsaiAPIURL ? nextDefaults.nopsaiAPIURL : current));
    setGitBotServiceURL(current => (!current.trim() || current === previousDefaults.gitBotServiceURL ? nextDefaults.gitBotServiceURL : current));
  };

  const loadStatus = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const payload = (await fetchJson('/v1/setup/status')) as SetupStatus;
      setStatus(payload);
      setNopsaiAPIURL(current => {
        const configured = (payload.github.nopsai_api_url || '').trim();
        return configured && (!current.trim() || current === runtimeDefaults('docker').nopsaiAPIURL) ? configured : current;
      });
      setGitBotServiceURL(current => {
        const configured = (payload.github.git_bot_service_url || (payload.github.webhook_url ? deriveGitBotBaseURL(payload.github.webhook_url) : '')).trim();
        if (isLikelyPublicURL(configured)) return current;
        return configured && (!current.trim() || current === runtimeDefaults('docker').gitBotServiceURL) ? configured : current;
      });
      setGitBotPublicURL(current => {
        if (current.trim() || !isLikelyPublicURL(payload.github.webhook_url)) return current;
        return deriveGitBotBaseURL(payload.github.webhook_url);
      });
      if (payload.global_config_repo) {
        setRepoURL(payload.global_config_repo.repo_url || '');
        setBranch(payload.global_config_repo.branch || 'main');
        setBasePath(payload.global_config_repo.base_path || '');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load setup status');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  useEffect(() => {
    if (status?.completed) {
      setWizardStepIndex(REVIEW_STEP_INDEX >= 0 ? REVIEW_STEP_INDEX : WIZARD_STEPS.length - 1);
    }
  }, [status?.completed]);

  const addRepositoryGroup = () => {
    if (repositoryGroups.length >= 2) return;
    const next = { id: makeID('group'), name: 'services', repositoriesText: '' };
    setRepositoryGroups(current => [...current, next]);
  };

  const updateRepositoryGroup = (id: string, updates: Partial<RepositoryGroupDraft>) => {
    setRepositoryEnabled(true);
    setRepositoryGroups(current => current.map(group => (group.id === id ? { ...group, ...updates } : group)));
    setTemplates(null);
  };

  const removeRepositoryGroup = (id: string) => {
    setRepositoryGroups(current => {
      const next = current.filter(group => group.id !== id);
      return next.length > 0 ? next : initialRepositoryGroups().slice(0, 1);
    });
    setTemplates(null);
  };

  const addUser = () => {
    setUsersEnabled(true);
    setUsers(current => [
      ...current,
      {
        id: makeID('user'),
        email: '',
        password: '',
        role: 'developer',
        group: groupOptions[0] || '',
      },
    ]);
  };

  const updateUser = (id: string, updates: Partial<UserDraft>) => {
    setUsersEnabled(true);
    setUsers(current => current.map(user => (user.id === id ? { ...user, ...updates } : user)));
  };

  const removeUser = (id: string) => {
    setUsers(current => current.filter(user => user.id !== id));
  };

  const buildTemplateParams = useCallback(() => {
    const params = new URLSearchParams({ profile: 'team' });
    if (repositories.length > 0) params.set('repositories', repositories.join(','));
    normalizedRepositoryGroups.forEach(group => {
      params.append('repository_group', `${group.name}:${group.repositories.join(',')}`);
    });
    if (usersEnabled) {
      users
        .map(user => ({
          sub: user.email.trim(),
          email: user.email.trim(),
          role: user.role,
          group: user.group,
        }))
        .filter(user => user.sub)
        .forEach(user => params.append('setup_user', JSON.stringify(user)));
    }
    params.set('include_llm', aiEnabled ? 'true' : 'false');
    params.set('mcp_examples', aiEnabled && mcpExamples ? 'true' : 'false');
    if (aiEnabled) {
      params.set('llm_provider', llmProvider.trim());
      params.set('llm_model', llmModel.trim());
      params.set('llm_api_key_secret', llmSecretName);
      if (llmProvider === 'lmstudio') params.set('llm_base_url', llmBaseURL.trim());
    }
    return params;
  }, [aiEnabled, llmBaseURL, llmModel, llmProvider, llmSecretName, mcpExamples, normalizedRepositoryGroups, repositories, users, usersEnabled]);

  const loadTemplates = useCallback(async () => {
    setTemplateLoading(true);
    setError(null);
    try {
      const params = buildTemplateParams();
      const payload = (await fetchJson(`/v1/setup/templates?${params.toString()}`)) as SetupTemplates;
      setTemplates(payload);
      const paths = Object.keys(payload.files).sort();
      setSelectedTemplatePath(paths[0] || '');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load templates');
    } finally {
      setTemplateLoading(false);
    }
  }, [buildTemplateParams]);

  const downloadGitOpsZip = useCallback(async () => {
    setDownloadingGitOpsZip(true);
    setError(null);
    try {
      const params = buildTemplateParams();
      const response = await fetch(buildApiUrl(`/v1/setup/templates.zip?${params.toString()}`));
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Download failed (${response.status})`);
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = 'nopsai-gitops-starter.zip';
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to download GitOps starter files');
    } finally {
      setDownloadingGitOpsZip(false);
    }
  }, [buildTemplateParams]);

  const userPayload = useMemo(
    () =>
      usersEnabled
        ? users
            .map(user => ({
              sub: user.email.trim(),
              email: user.email.trim(),
              role: user.role,
              password: user.password,
              group: user.group,
            }))
            .filter(user => user.email)
        : [],
    [users, usersEnabled]
  );

  const runtimeEnvSections = useMemo<RuntimeEnvSection[]>(() => {
    const sharedLines = [
      'NOPSAI_MASTER_KEY=<generate-strong-value>',
      'JWT_SIGNING_KEY=<generate-strong-value>',
      'SERVICE_JWT_SIGNING_KEY=<generate-strong-value>',
      'AAA_SHARED_INTERNAL_TOKEN=<generate-strong-value>',
      'DISPATCHER_TLS_SECRET=<generate-strong-value>',
    ];
    const nopsaiLines = [
      `NOPSAI_GIT_BOT_API_URL=${gitBotServiceURL.trim() || currentRuntimeDefaults.gitBotServiceURL}`,
      runtimeImplementation === 'docker' ? 'DOCKER_NETWORK_NAME=nopsai-net' : '# Kubernetes runtime is under construction.',
    ];
    if (aiEnabled && llmProvider === 'gemini') {
      nopsaiLines.push(`${llmSecretName}=${secretPlaceholder(Boolean(llmAPIKey.trim()), '<paste Gemini API key or mount from secret manager>')}`);
    } else if (aiEnabled && llmProvider === 'lmstudio' && llmAPIKey.trim()) {
      nopsaiLines.push(`${llmSecretName}=<optional LM Studio API key provided in wizard>`);
    }

    const gitBotLines = [
      `GIT_BOT_NOPSAI_API_URL=${nopsaiAPIURL.trim() || currentRuntimeDefaults.nopsaiAPIURL}`,
    ];
    if (githubEnabled) {
      gitBotLines.push(`GITHUB_APP_ID=${githubAppID.trim() || '<github-app-id>'}`);
      gitBotLines.push(`GITHUB_INSTALLATION_ID=${githubInstallationID.trim() || '<github-installation-id>'}`);
      gitBotLines.push(`GITHUB_WEBHOOK_SECRET=${githubWebhookSecret.trim() || '<generate-or-paste-webhook-secret>'}`);
      gitBotLines.push(`GITHUB_PRIVATE_KEY_PATH=${githubPrivateKeyPath.trim() || '/run/secrets/nopsai-github-app.pem'}`);
      if (githubPrivateKeyMode === 'inline') {
        gitBotLines.push(`GITHUB_PRIVATE_KEY=${secretPlaceholder(Boolean(githubPrivateKey.trim()), '<paste-private-key-pem>')}`);
      } else {
        gitBotLines.push('# Mount the GitHub App private key file at GITHUB_PRIVATE_KEY_PATH.');
      }
    }

    return [
      { title: 'Shared by nopsai, git-bot, dispatcher, runner, and aaa', fileName: 'shared.env', lines: sharedLines },
      { title: 'nopsai container', fileName: 'nopsai.env', lines: nopsaiLines },
      { title: 'git-bot container', fileName: 'git-bot.env', lines: gitBotLines },
    ];
  }, [
    aiEnabled,
    currentRuntimeDefaults.gitBotServiceURL,
    currentRuntimeDefaults.nopsaiAPIURL,
    gitBotServiceURL,
    githubAppID,
    githubEnabled,
    githubInstallationID,
    githubPrivateKey,
    githubPrivateKeyMode,
    githubPrivateKeyPath,
    githubWebhookSecret,
    llmAPIKey,
    llmProvider,
    llmSecretName,
    nopsaiAPIURL,
    runtimeImplementation,
  ]);

  const environmentSnippet = useMemo(
    () => runtimeEnvSections.map(section => [`# ${section.title}`, ...section.lines].join('\n')).join('\n\n'),
    [runtimeEnvSections]
  );

  const gitOpsStructureSnippet = useMemo(() => {
    const groups = normalizedRepositoryGroups;
    if (groups.length === 0) {
      return '{}';
    }
    const lines: string[] = [];
    groups.forEach(group => {
      lines.push(`${group.name}:`);
      lines.push('  description: Repository group');
      if (group.repositories.length === 0) {
        lines.push('  repos: []');
      } else {
        lines.push('  repos:');
        group.repositories.forEach(repo => lines.push(`    - ${repo}`));
      }
    });
    return lines.join('\n');
  }, [normalizedRepositoryGroups]);

  const gitOpsFiles = useMemo(() => {
    const files = [
      'config-repositories/groups/structure.yaml',
      'pipelines/setup/first-run.yaml',
      'steps/setup/announce.yaml',
      'scopes/dev/scope.yaml',
      'scopes/prod/scope.yaml',
      'knowledge/guideline/platform/setup-run.md',
      'access/bootstrap.yaml',
    ];
    if (aiEnabled) files.push('setting/system/llm_profile.yaml');
    if (mcpExamples) files.push('setting/system/mcp.yaml');
    repositories.forEach(repo => files.push(`triggers/${repo}.yaml`));
    return files;
  }, [aiEnabled, mcpExamples, repositories]);

  const canContinueWizard = (() => {
    switch (currentWizardStep.id) {
      case 'readiness':
        return !loading && requiredHealthErrors.length === 0;
      default:
        return true;
    }
  })();

  const skipCurrentStep = () => {
    switch (currentWizardStep.id) {
      case 'gitops':
        setRepoURL('');
        setSyncConfigRepository(false);
        break;
      case 'github':
        setGithubEnabled(false);
        break;
      case 'repositories':
        setRepositoryEnabled(false);
        setTemplates(null);
        break;
      case 'ai':
        setAIEnabled(false);
        setLLMAPIKey('');
        setMCPExamples(false);
        break;
      case 'users':
        setUsersEnabled(false);
        setUsers([]);
        break;
    }
    setWizardStepIndex(index => Math.min(WIZARD_STEPS.length - 1, index + 1));
  };

  const applySetup = useCallback(async () => {
    if (!canManage || saving) return;
    setSaving(true);
    setError(null);
    setBootstrapResult(null);
    try {
      const payload = (await fetchJson('/v1/setup/bootstrap', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          profile: 'team',
          generate_secrets: generateServiceSecrets,
          seed_starter_database: true,
          seed_llm_profile: aiEnabled,
          mcp_examples: aiEnabled && mcpExamples,
          production_acknowledged: false,
          sync_config_repository: Boolean(syncConfigRepository && repoURL.trim()),
          config_repository: {
            repo_url: repoURL.trim(),
            branch: branch.trim() || 'main',
            base_path: basePath.trim(),
            enabled: true,
          },
          repository_groups: normalizedRepositoryGroups,
          repositories,
          llm_profile: {
            name: 'standard',
            provider: llmProvider.trim(),
            model: llmModel.trim(),
            base_url: llmProvider === 'gemini' ? '' : llmBaseURL.trim(),
            api_key_secret: aiEnabled && (llmProvider === 'gemini' || llmAPIKey.trim()) ? llmSecretName : '',
            api_key_value: aiEnabled ? llmAPIKey.trim() : '',
            allowed_scopes: ['dev', 'prod'],
          },
          users: userPayload,
        }),
      })) as BootstrapResponse;
      setBootstrapResult(payload);
      setStatus(payload.status);
      setLLMAPIKey('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed');
    } finally {
      setSaving(false);
    }
  }, [
    aiEnabled,
    basePath,
    branch,
    canManage,
    generateServiceSecrets,
    llmAPIKey,
    llmAPIKeySecretName,
    llmBaseURL,
    llmModel,
    llmProvider,
    llmSecretName,
    mcpExamples,
    normalizedRepositoryGroups,
    repositories,
    repoURL,
    saving,
    syncConfigRepository,
    userPayload,
  ]);

  const renderWizardStep = () => {
    switch (currentWizardStep.id) {
      case 'readiness':
        return (
          <div className="space-y-4">
            <StepIntro title="Confirm the control plane is ready" icon={<ShieldCheck className="h-4 w-4" />}>
              These checks make sure the database, admin account, signing keys, GitHub App wiring, runner path, and starter resources are visible before real automation starts. Blocking errors must be resolved before setup can continue.
            </StepIntro>
            <div className="grid gap-3 md:grid-cols-2">
              {(status?.checks || []).map(check => (
                <div key={check.id} className={`rounded-lg border p-3 ${statusClasses(check.status)}`}>
                  <div className="flex items-center gap-2 text-sm font-semibold">
                    {statusIcon(check.status)}
                    {check.label}
                  </div>
                  {check.message && <div className="mt-2 text-xs leading-5">{check.message}</div>}
                </div>
              ))}
            </div>
            {requiredHealthErrors.length > 0 && (
              <div className="rounded-md border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300">
                Resolve required readiness errors before continuing. If this is the first admin login, finish the password change first and refresh checks.
              </div>
            )}
            <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={() => void loadStatus()} disabled={loading}>
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh checks
            </button>
          </div>
        );
      case 'runtime':
        return (
          <div className="space-y-4">
            <StepIntro title="Collect service-level runtime values" icon={<KeyRound className="h-4 w-4" />}>
              NopsAI needs shared signing, dispatcher, webhook, and service URLs so the UI, API, git-bot, dispatcher, runners, and agents can trust each other. Docker Compose uses service-network addresses; Kubernetes support is visible here but still under construction.
            </StepIntro>
            <div className="grid gap-3 md:grid-cols-2">
              <button type="button" onClick={() => updateRuntimeImplementation('docker')} className={`rounded-lg border p-3 text-left ${runtimeImplementation === 'docker' ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}>
                <div className="text-sm font-semibold">Docker Compose</div>
                <div className="mt-1 text-xs leading-5 text-[var(--text-secondary)]">Use container DNS names like `nopsai` and `git-bot` on the `nopsai-net` bridge network.</div>
              </button>
              <button type="button" disabled className="rounded-lg border border-[var(--border-primary)] p-3 text-left opacity-60">
                <div className="flex items-center justify-between gap-2 text-sm font-semibold">
                  <span>Kubernetes</span>
                  <span className="rounded border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] uppercase text-amber-700 dark:text-amber-300">Under construction</span>
                </div>
                <div className="mt-1 text-xs leading-5 text-[var(--text-secondary)]">Service DNS defaults are planned, but this wizard will not apply K8s manifests yet.</div>
              </button>
            </div>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={generateServiceSecrets} onChange={event => setGenerateServiceSecrets(event.target.checked)} disabled={!canManage} />
              Generate missing service secrets during setup
            </label>
            <div className="grid gap-3 md:grid-cols-2">
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">git-bot to NopsAI API URL</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={nopsaiAPIURL} onChange={event => setNopsaiAPIURL(event.target.value)} disabled={!canManage} />
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">For Docker Compose this should stay `http://nopsai:8080`, not localhost.</span>
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">NopsAI to git-bot service URL</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={gitBotServiceURL} onChange={event => setGitBotServiceURL(event.target.value)} disabled={!canManage} />
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">For Docker Compose this should stay `http://git-bot:8081`; the public webhook tunnel is configured in the GitHub step.</span>
              </label>
            </div>
          </div>
        );
      case 'gitops':
        return (
          <div className="space-y-4">
            <StepIntro title="Connect a GitOps source of truth" icon={<GitBranch className="h-4 w-4" />}>
              The global config repository stores reviewable workspace definitions: folders, starter pipeline, reusable step, repository triggers, scopes, access grants, knowledge docs, LLM profile, and MCP examples. You can skip this for a quick introduction and connect GitOps later.
            </StepIntro>
            <div className="grid gap-3 lg:grid-cols-3">
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Global config repo</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={repoURL} onChange={event => setRepoURL(event.target.value)} placeholder="https://github.com/acme/nopsai-config.git" disabled={!canManage} />
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Branch</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={branch} onChange={event => setBranch(event.target.value)} disabled={!canManage} />
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Base path</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={basePath} onChange={event => setBasePath(event.target.value)} disabled={!canManage} />
              </label>
            </div>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={syncConfigRepository} onChange={event => setSyncConfigRepository(event.target.checked)} disabled={!canManage || !repoURL.trim()} />
              Start config sync after saving the repository
            </label>
          </div>
        );
      case 'github':
        return (
          <div className="space-y-4">
            <StepIntro title="Prepare the GitHub App integration" icon={<Github className="h-4 w-4" />}>
              GitHub automation needs an App ID, installation ID, webhook secret, private key, and a webhook URL that GitHub can reach. For local Docker you can expose only git-bot through ngrok or another tunnel; NopsAI itself can remain private on the Docker network.
            </StepIntro>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={githubEnabled} onChange={event => setGithubEnabled(event.target.checked)} disabled={!canManage} />
              Include GitHub App configuration in the generated output
            </label>
            <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <div className="font-semibold">git-bot install checklist</div>
              <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs leading-5 text-[var(--text-secondary)]">
                <li>Start the `git-bot` service with Docker Compose or your runtime, and set `GIT_BOT_NOPSAI_API_URL` to `{nopsaiAPIURL || currentRuntimeDefaults.nopsaiAPIURL}`.</li>
                <li>Create or open a GitHub App, set its webhook URL to the value shown below, and paste the same webhook secret into GitHub and `GITHUB_WEBHOOK_SECRET`.</li>
                <li>Set repository permissions to Contents read and write, Metadata read, Pull requests read, and Checks read and write.</li>
                <li>Generate a private key in the GitHub App settings, configure the App ID and private key for git-bot, then install the App on the selected repositories.</li>
                <li>Copy the installation ID from the GitHub installation URL after installing the App.</li>
              </ol>
            </div>
            <label className="space-y-1 text-sm">
              <span className="text-xs text-[var(--text-secondary)]">Public git-bot webhook base URL</span>
              <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={gitBotPublicURL} onChange={event => setGitBotPublicURL(event.target.value)} placeholder="https://your-subdomain.ngrok-free.app" disabled={!canManage || !githubEnabled} />
              <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Point ngrok at the host or container port that reaches git-bot, then paste the tunnel base URL here. GitHub only needs `/webhook`; the API URL used by git-bot stays `{nopsaiAPIURL || currentRuntimeDefaults.nopsaiAPIURL}`.</span>
            </label>
            <div className="grid gap-3 md:grid-cols-2">
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">GitHub App ID</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={githubAppID} onChange={event => setGithubAppID(event.target.value)} disabled={!canManage || !githubEnabled} />
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">GitHub App settings, General tab, App ID.</span>
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Installation ID</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={githubInstallationID} onChange={event => setGithubInstallationID(event.target.value)} disabled={!canManage || !githubEnabled} />
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Install App page, then use the number in the installation URL, such as `/settings/installations/12345678`.</span>
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Private key source</span>
                <select className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={githubPrivateKeyMode} onChange={event => setGithubPrivateKeyMode(event.target.value as GitHubPrivateKeyMode)} disabled={!canManage || !githubEnabled}>
                  <option value="path">Mount downloaded key file</option>
                  <option value="inline">Paste PEM into secret value</option>
                </select>
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Generate a new private key from the GitHub App settings Private keys section, then mount the downloaded PEM or paste it into your secret manager.</span>
              </label>
              {githubPrivateKeyMode === 'path' ? (
                <label className="space-y-1 text-sm">
                  <span className="text-xs text-[var(--text-secondary)]">Private key mount path</span>
                  <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={githubPrivateKeyPath} onChange={event => setGithubPrivateKeyPath(event.target.value)} disabled={!canManage || !githubEnabled} />
                </label>
              ) : (
                <label className="space-y-1 text-sm md:col-span-2">
                  <span className="text-xs text-[var(--text-secondary)]">Pasted private key</span>
                  <textarea className="min-h-32 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 font-mono text-xs" value={githubPrivateKey} onChange={event => setGithubPrivateKey(event.target.value)} placeholder="-----BEGIN RSA PRIVATE KEY-----" disabled={!canManage || !githubEnabled} />
                  <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">The review output will not echo the PEM; it will show where to store it.</span>
                </label>
              )}
            </div>
            <div className="grid gap-3 md:grid-cols-[1fr_auto]">
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Webhook secret</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={githubWebhookSecret} onChange={event => setGithubWebhookSecret(event.target.value)} placeholder="Paste or generate a shared secret" disabled={!canManage || !githubEnabled} />
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Use the same value in GitHub App webhook settings and `GITHUB_WEBHOOK_SECRET`.</span>
              </label>
              <button type="button" className="self-end rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm disabled:opacity-50" onClick={() => setGithubWebhookSecret(generateBrowserSecret(32))} disabled={!canManage || !githubEnabled}>Generate secret</button>
            </div>
            <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <div className="text-xs text-[var(--text-secondary)]">Webhook URL to configure in GitHub</div>
              <div className="mt-2 break-all font-mono text-xs">{gitBotWebhookURL}</div>
              <div className="mt-3 text-xs text-[var(--text-secondary)]">Events: {(status?.github.required_events || []).join(', ') || 'push, pull_request, check_run, check_suite, ping'}</div>
            </div>
          </div>
        );
      case 'repositories':
        return (
          <div className="space-y-4">
            <StepIntro title="Create one or two repository groups" icon={<FolderTree className="h-4 w-4" />}>
              This is an introduction, not a full migration. Create one or two folder groups and add repositories as GitHub `owner/repo` names, for example `acme/service-api`. NopsAI uses these groups for starter triggers, pipeline-run navigation, and user access assignments.
            </StepIntro>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={repositoryEnabled} onChange={event => setRepositoryEnabled(event.target.checked)} disabled={!canManage} />
              Create starter repository groups
            </label>
            <div className="grid gap-3 lg:grid-cols-2">
              {repositoryGroups.map(group => (
                <div key={group.id} className="rounded-lg border border-[var(--border-primary)] p-3">
                  <div className="mb-3 flex items-center justify-between gap-2">
                    <label className="min-w-0 flex-1 space-y-1">
                      <span className="text-xs text-[var(--text-secondary)]">Group folder name</span>
                      <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={group.name} onChange={event => updateRepositoryGroup(group.id, { name: event.target.value })} disabled={!canManage || !repositoryEnabled} />
                    </label>
                    <button className="rounded-md border border-[var(--border-primary)] p-2 disabled:opacity-50" onClick={() => removeRepositoryGroup(group.id)} disabled={!canManage || repositoryGroups.length <= 1} title="Remove group">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                  <label className="space-y-1">
                    <span className="text-xs text-[var(--text-secondary)]">Repositories, one GitHub `owner/repo` per line</span>
                    <textarea className="min-h-32 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 font-mono text-xs" value={group.repositoriesText} onChange={event => updateRepositoryGroup(group.id, { repositoriesText: event.target.value })} placeholder="acme/service-api&#10;acme/web-app" disabled={!canManage || !repositoryEnabled} />
                    <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">HTTPS and SSH GitHub URLs are accepted, but the generated structure stores them as `owner/repo`.</span>
                  </label>
                </div>
              ))}
            </div>
            <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm disabled:opacity-50" onClick={addRepositoryGroup} disabled={!canManage || repositoryGroups.length >= 2}>
              <Plus className="h-4 w-4" />
              Add group
            </button>
          </div>
        );
      case 'ai':
        return (
          <div className="space-y-4">
            <StepIntro title="Configure the default AI profile" icon={<Bot className="h-4 w-4" />}>
              This creates one default LLM profile named `standard` so goal-based steps can work immediately. Gemini uses a model and API key secret; LM Studio uses an OpenAI-compatible base URL and only needs a key when your local server requires one.
            </StepIntro>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={aiEnabled} onChange={event => setAIEnabled(event.target.checked)} disabled={!canManage} />
              Configure default AI profile
            </label>
            {!aiEnabled && <WarningCallout>{LLM_SKIP_WARNING}</WarningCallout>}
            <div className="grid gap-3 md:grid-cols-2">
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Provider</span>
                <select className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmProvider} onChange={event => {
                  const nextProvider = event.target.value;
                  setLLMProvider(nextProvider);
                  setLLMAPIKeySecretName(current => (!current.trim() || current === defaultSecretName(llmProvider) ? defaultSecretName(nextProvider) : current));
                }} disabled={!canManage || !aiEnabled}>
                  <option value="lmstudio">LM Studio</option>
                  <option value="gemini">Gemini</option>
                </select>
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Model</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmModel} onChange={event => setLLMModel(event.target.value)} disabled={!canManage || !aiEnabled} />
              </label>
              {llmProvider === 'lmstudio' && (
                <label className="space-y-1 text-sm">
                  <span className="text-xs text-[var(--text-secondary)]">Base URL</span>
                  <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmBaseURL} onChange={event => setLLMBaseURL(event.target.value)} placeholder="http://lmstudio:1234" disabled={!canManage || !aiEnabled} />
                  <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Use a URL the agent container can reach.</span>
                </label>
              )}
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">API key secret name</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmAPIKeySecretName} onChange={event => setLLMAPIKeySecretName(event.target.value)} placeholder={defaultSecretName(llmProvider)} disabled={!canManage || !aiEnabled} />
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">{llmProvider === 'gemini' ? 'Gemini API key value' : 'Optional API key value'}</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmAPIKey} onChange={event => setLLMAPIKey(event.target.value)} type="password" placeholder={llmProvider === 'lmstudio' ? 'Optional for local LM Studio' : 'Paste key to seed the secret'} disabled={!canManage || !aiEnabled} />
              </label>
            </div>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={mcpExamples} onChange={event => setMCPExamples(event.target.checked)} disabled={!canManage || !aiEnabled} />
              Include disabled MCP examples for later review
            </label>
          </div>
        );
      case 'users':
        return (
          <div className="space-y-4">
            <StepIntro title="Create starter users and assign group roles" icon={<Users className="h-4 w-4" />}>
              Add a small set of users for the groups above. Each user gets a product role on the selected group. Passwords are temporary and users must change them on first login; leave a password blank when you want NopsAI to generate one.
            </StepIntro>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={usersEnabled} onChange={event => setUsersEnabled(event.target.checked)} disabled={!canManage} />
              Create starter users
            </label>
            <div className="space-y-3">
              {users.map(user => (
                <div key={user.id} className="grid gap-2 rounded-lg border border-[var(--border-primary)] p-3 md:grid-cols-[1.5fr_1fr_1fr_1fr_auto]">
                  <input className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={user.email} onChange={event => updateUser(user.id, { email: event.target.value })} placeholder="alice@example.com" disabled={!canManage || !usersEnabled} />
                  <input className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={user.password} onChange={event => updateUser(user.id, { password: event.target.value })} type="password" placeholder="Temporary password" disabled={!canManage || !usersEnabled} />
                  <select className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={user.role} onChange={event => updateUser(user.id, { role: event.target.value as UserDraft['role'] })} disabled={!canManage || !usersEnabled}>
                    <option value="owner">Owner</option>
                    <option value="developer">Developer</option>
                    <option value="viewer">Viewer</option>
                  </select>
                  <select className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={user.group} onChange={event => updateUser(user.id, { group: event.target.value })} disabled={!canManage || !usersEnabled}>
                    {userGroupOptions.map(group => <option key={group || 'workspace'} value={group}>{group || 'workspace'}</option>)}
                  </select>
                  <button className="rounded-md border border-[var(--border-primary)] p-2 disabled:opacity-50" onClick={() => removeUser(user.id)} disabled={!canManage} title="Remove user">
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ))}
            </div>
            <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={addUser} disabled={!canManage || !usersEnabled}>
              <Plus className="h-4 w-4" />
              Add user
            </button>
          </div>
        );
      default:
        return (
          <div className="space-y-4">
            <StepIntro title="Generated setup output" icon={<FileText className="h-4 w-4" />}>
              Review what will be created now and what should be applied outside the database. Runtime variables are grouped by target container; GitOps starter files can be previewed in the page and downloaded as one zip for the global config repository.
            </StepIntro>
            {!aiEnabled && <WarningCallout>{LLM_SKIP_WARNING}</WarningCallout>}
            <div className="grid gap-3 md:grid-cols-3">
              <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
                <div className="text-xs text-[var(--text-secondary)]">Repository groups</div>
                <div className="mt-1">{normalizedRepositoryGroups.length || 'Skipped'}</div>
              </div>
              <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
                <div className="text-xs text-[var(--text-secondary)]">Repositories</div>
                <div className="mt-1">{repositories.length || 'Skipped'}</div>
              </div>
              <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
                <div className="text-xs text-[var(--text-secondary)]">Users</div>
                <div className="mt-1">{userPayload.length || 'Skipped'}</div>
              </div>
            </div>
            <div className="space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-sm font-semibold">Runtime variables by container</div>
                <button type="button" className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={() => downloadTextFile('nopsai-runtime-env.txt', environmentSnippet)}>
                  <Download className="h-4 w-4" />
                  Download all env
                </button>
              </div>
              <div className="grid gap-3 xl:grid-cols-3">
                {runtimeEnvSections.map(section => (
                  <div key={section.fileName} className="rounded-md border border-[var(--border-primary)] p-3">
                    <div className="mb-2 flex items-center justify-between gap-2">
                      <div className="text-sm font-semibold">{section.title}</div>
                      <button type="button" className="rounded-md border border-[var(--border-primary)] p-2" onClick={() => downloadTextFile(section.fileName, section.lines.join('\n'))} title={`Download ${section.fileName}`}>
                        <Download className="h-4 w-4" />
                      </button>
                    </div>
                    <pre className="max-h-72 overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs leading-5">{section.lines.join('\n')}</pre>
                  </div>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-2 text-sm font-semibold">GitOps group file</div>
              <pre className="max-h-80 overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs leading-5">{gitOpsStructureSnippet}</pre>
            </div>
            <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="font-semibold">Files to commit when using GitOps</div>
                  <div className="mt-1 text-xs text-[var(--text-secondary)]">The zip preserves folder paths such as `pipelines/`, `steps/`, `triggers/`, and `setting/system/`.</div>
                </div>
                <button type="button" className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={() => void downloadGitOpsZip()} disabled={downloadingGitOpsZip}>
                  <Download className="h-4 w-4" />
                  {downloadingGitOpsZip ? 'Downloading...' : 'Download GitOps zip'}
                </button>
              </div>
              <div className="mt-3 grid gap-2 md:grid-cols-2">
                {gitOpsFiles.map(file => <div key={file} className="rounded border border-[var(--border-primary)] px-2 py-1 font-mono text-xs">{file}</div>)}
              </div>
            </div>
            <div className="rounded-md border border-sky-500/30 bg-sky-500/10 p-3 text-sm leading-6 text-sky-700 dark:text-sky-300">
              Configure the GitHub App webhook URL as `{gitBotWebhookURL}`, apply each env group to the matching container or secret manager, mount or store the GitHub private key for git-bot, commit the GitOps zip if you are using GitOps, restart services that received new runtime values, and run `setup/first-run` to prove runner, agent, logs, and UI are working{aiEnabled ? ', including AI.' : '.'}
            </div>
            <div className="flex flex-wrap gap-2">
              <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-4 py-2 text-sm" onClick={() => void loadTemplates()} disabled={templateLoading}>
                <FileText className="h-4 w-4" />
                {templateLoading ? 'Loading...' : templates ? 'Refresh file preview' : 'Preview GitOps files'}
              </button>
            </div>
          </div>
        );
    }
  };

  const renderStepNavigation = () => (
    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8">
      {WIZARD_STEPS.map((step, index) => (
        <button
          key={step.id}
          type="button"
          onClick={() => setWizardStepIndex(index)}
          aria-current={index === wizardStepIndex ? 'step' : undefined}
          className={`rounded-md border px-2 py-2 text-left text-xs ${index === wizardStepIndex ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}
        >
          <span className="block font-semibold">{step.label}</span>
          <span className="text-[var(--text-secondary)]">{step.required ? 'Required' : 'Optional'}</span>
        </button>
      ))}
    </div>
  );

  const wizardModal = status && !status.completed ? (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="flex max-h-[92vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-2xl">
        <div className="border-b border-[var(--border-primary)] p-5">
          <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <div>
              <div className="text-xs uppercase tracking-wide text-[var(--text-secondary)]">First-install wizard</div>
              <h2 className="text-xl font-semibold">{currentWizardStep.label}</h2>
            </div>
            <div className="text-sm text-[var(--text-secondary)]">Step {wizardStepIndex + 1} of {WIZARD_STEPS.length}</div>
          </div>
          <div className="mt-4">{renderStepNavigation()}</div>
        </div>
        <div className="flex-1 overflow-auto p-5">{renderWizardStep()}</div>
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-primary)] p-5">
          <div className="text-xs text-[var(--text-secondary)]">{currentWizardStep.required ? 'Complete this step to continue.' : 'Optional step. Skip it now and configure it later.'}</div>
          <div className="flex flex-wrap gap-2">
            <button className="rounded-md border border-[var(--border-primary)] px-4 py-2 text-sm" onClick={() => setWizardStepIndex(index => Math.max(0, index - 1))} disabled={wizardStepIndex === 0}>Back</button>
            {!currentWizardStep.required && <button className="rounded-md border border-[var(--border-primary)] px-4 py-2 text-sm" onClick={skipCurrentStep}>Skip</button>}
            {wizardStepIndex >= WIZARD_STEPS.length - 1 ? (
              <button className="glass-button-primary inline-flex items-center gap-2" onClick={() => void applySetup()} disabled={!canManage || saving || !canContinueWizard}>
                <PlayCircle className="h-4 w-4" />
                {saving ? 'Applying...' : 'Apply setup'}
              </button>
            ) : (
              <button className="glass-button-primary" onClick={() => setWizardStepIndex(index => Math.min(WIZARD_STEPS.length - 1, index + 1))} disabled={!canContinueWizard}>Continue</button>
            )}
          </div>
        </div>
      </div>
    </div>
  ) : null;

  return (
    <div className="space-y-6">
      {wizardModal}
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 className="text-xl font-semibold">First-install setup</h2>
          <div className="mt-1 text-sm text-[var(--text-secondary)]">{status?.completed ? 'Completed' : 'Not completed'}</div>
        </div>
        <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={() => void loadStatus()} disabled={loading}>
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      {error && <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300">{error}</div>}

      <section className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
        <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
          <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
            <CheckCircle2 className="h-4 w-4" />
            Health checks
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            {(status?.checks || []).map(check => (
              <div key={check.id} className={`rounded-lg border p-3 ${statusClasses(check.status)}`}>
                <div className="flex items-center gap-2 text-sm font-semibold">
                  {statusIcon(check.status)}
                  {check.label}
                </div>
                {check.message && <div className="mt-2 text-xs leading-5">{check.message}</div>}
              </div>
            ))}
          </div>
        </div>

        <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
          <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
            <Info className="h-4 w-4" />
            Resource counts
          </div>
          <div className="grid grid-cols-2 gap-2 text-sm">
            {status && Object.entries(status.counts).map(([key, value]) => (
              <div key={key} className="rounded-md border border-[var(--border-primary)] p-2">
                <div className="text-xs capitalize text-[var(--text-secondary)]">{key.replaceAll('_', ' ')}</div>
                <div className="text-lg font-semibold">{value}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {bootstrapResult && (
        <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
          <div className="mb-3 text-sm font-semibold">Bootstrap result</div>
          <div className="grid gap-3 lg:grid-cols-3">
            {(bootstrapResult.warnings || []).map(warning => (
              <div key={warning} className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300">
                <div className="flex items-start gap-2">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                  <span>{warning}</span>
                </div>
              </div>
            ))}
            {(bootstrapResult.messages || []).map(message => (
              <div key={message} className="rounded-md border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300">{message}</div>
            ))}
            {(bootstrapResult.generated_secrets || []).map(secret => (
              <div key={secret} className="rounded-md border border-[var(--border-primary)] p-3 font-mono text-xs">{secret}</div>
            ))}
          </div>
          {(bootstrapResult.temporary_credentials || []).length > 0 && (
            <pre className="mt-3 overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs">{JSON.stringify(bootstrapResult.temporary_credentials, null, 2)}</pre>
          )}
        </section>
      )}

      <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
        <div className="mb-4 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="text-sm font-semibold">Setup details</div>
            <div className="mt-1 text-xs text-[var(--text-secondary)]">Review every setup step, generated env group, and GitOps file preview anytime.</div>
          </div>
          <div className="text-sm text-[var(--text-secondary)]">Step {wizardStepIndex + 1} of {WIZARD_STEPS.length}</div>
        </div>
        {renderStepNavigation()}
        <div className="mt-5">{renderWizardStep()}</div>
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <button className="rounded-md border border-[var(--border-primary)] px-4 py-2 text-sm" onClick={() => setWizardStepIndex(index => Math.max(0, index - 1))} disabled={wizardStepIndex === 0}>Back</button>
          <button className="glass-button-primary" onClick={() => setWizardStepIndex(index => Math.min(WIZARD_STEPS.length - 1, index + 1))} disabled={wizardStepIndex >= WIZARD_STEPS.length - 1}>Continue</button>
        </div>
      </section>

      {templates && (
        <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div className="text-sm font-semibold">Starter files</div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <select className="max-w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={selectedTemplatePath} onChange={event => setSelectedTemplatePath(event.target.value)}>
                {templatePaths.map(path => <option key={path} value={path}>{path}</option>)}
              </select>
            </div>
          </div>
          <pre className="max-h-[520px] overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 text-xs leading-5">{selectedTemplate}</pre>
        </section>
      )}
    </div>
  );
}

export default SetupWizard;
