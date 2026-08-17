import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Bot,
  GitBranch,
  KeyRound,
  PlayCircle,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Users,
} from 'lucide-react';
import { bootstrapSetup, downloadSetupTemplatesZip, fetchSetupStatus, fetchSetupTemplates } from './setup/api';
import { LLM_PROVIDERS, getLLMProvider, replaceProviderDefault } from './llmProviders';
import { SetupReviewOutput } from './setup/SetupReviewOutput';
import { SetupBootstrapResult, SetupStarterFilesPreview, SetupStatusOverview } from './setup/SetupStatusPanels';
import SetupGitHubStep from './setup/SetupGitHubStep';
import { SetupStatusIcon, SetupStepNavigation, StepIntro, WarningCallout } from './setup/SetupWizardPrimitives';
import {
  LLM_SKIP_WARNING,
  REVIEW_STEP_INDEX,
  WIZARD_STEPS,
  buildSetupGitOpsFileList,
  buildSetupGitOpsStructurePreview,
  defaultCredentialRef,
  deriveGitBotBaseURL,
  initialRepositoryTeams,
  isLikelyPublicURL,
  makeID,
  normalizeTeamName,
  parseRepositories,
  runtimeDefaults,
  statusClasses,
  type BootstrapResponse,
  type RepositoryTeamDraft,
  type RuntimeEnvSection,
  type RuntimeImplementation,
  type SetupStatus,
  type SetupTemplates,
  type UserDraft,
} from './setup/model';

function SetupWizard({
  canManage,
  onStatusChange,
}: {
  canManage: boolean;
  onStatusChange?: (status: SetupStatus) => void;
}) {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [generateServiceSecrets, setGenerateServiceSecrets] = useState(true);
  const [runtimeImplementation, setRuntimeImplementation] = useState<RuntimeImplementation>('docker');
  const [nopsaiAPIURL, setNopsaiAPIURL] = useState(runtimeDefaults('docker').nopsaiAPIURL);
  const [gitBotServiceURL, setGitBotServiceURL] = useState(runtimeDefaults('docker').gitBotServiceURL);
  const [repoURL, setRepoURL] = useState('');
  const [branch, setBranch] = useState('main');
  const [basePath, setBasePath] = useState('');
  const [syncConfigRepository, setSyncConfigRepository] = useState(false);
  const [repositoryEnabled, setRepositoryEnabled] = useState(true);
  const [repositoryTeams, setRepositoryTeams] = useState<RepositoryTeamDraft[]>(() => initialRepositoryTeams());
  const [aiEnabled, setAIEnabled] = useState(true);
  const [llmProvider, setLLMProvider] = useState('lmstudio');
  const [llmModel, setLLMModel] = useState('qwen3-coder');
  const [llmBaseURL, setLLMBaseURL] = useState('http://lmstudio:1234');
  const [llmCredentialRef, setLLMCredentialRef] = useState(defaultCredentialRef('lmstudio'));
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
  const normalizedRepositoryTeams = useMemo(
    () =>
      repositoryEnabled
        ? repositoryTeams
            .map(team => ({
              name: normalizeTeamName(team.name),
              repositories: parseRepositories(team.repositoriesText),
            }))
            .filter(team => team.name)
        : [],
    [repositoryEnabled, repositoryTeams]
  );
  const repositories = useMemo(
    () => Array.from(new Set(normalizedRepositoryTeams.flatMap(team => team.repositories))).sort(),
    [normalizedRepositoryTeams]
  );
  const teamOptions = useMemo(() => normalizedRepositoryTeams.map(team => team.name), [normalizedRepositoryTeams]);
  const userTeamOptions = useMemo(() => (teamOptions.length > 0 ? teamOptions : ['']), [teamOptions]);
  const templatePaths = useMemo(() => (templates ? Object.keys(templates.files).sort() : []), [templates]);
  const selectedTemplate = selectedTemplatePath && templates ? templates.files[selectedTemplatePath] : '';
  const requiredHealthErrors = (status?.checks || []).filter(check => check.blocking && check.status === 'error');
  const llmReference = llmCredentialRef.trim() || defaultCredentialRef(llmProvider);
  const llmProviderDefinition = getLLMProvider(llmProvider);
  const currentRuntimeDefaults = runtimeDefaults(runtimeImplementation);

  useEffect(() => {
    if (!templates || selectedTemplatePath) return;
    setSelectedTemplatePath(templatePaths[0] || '');
  }, [selectedTemplatePath, templatePaths, templates]);

  const updateRuntimeImplementation = (nextRuntime: RuntimeImplementation) => {
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
      const payload = await fetchSetupStatus();
      setStatus(payload);
      onStatusChange?.(payload);
      setNopsaiAPIURL(current => {
        const configured = (payload.github.nopsai_api_url || '').trim();
        return configured && (!current.trim() || current === runtimeDefaults('docker').nopsaiAPIURL) ? configured : current;
      });
      setGitBotServiceURL(current => {
        const configured = (payload.github.git_bot_service_url || (payload.github.webhook_url ? deriveGitBotBaseURL(payload.github.webhook_url) : '')).trim();
        if (isLikelyPublicURL(configured)) return current;
        return configured && (!current.trim() || current === runtimeDefaults('docker').gitBotServiceURL) ? configured : current;
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
  }, [onStatusChange]);

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  useEffect(() => {
    if (status?.completed) {
      setWizardStepIndex(REVIEW_STEP_INDEX >= 0 ? REVIEW_STEP_INDEX : WIZARD_STEPS.length - 1);
    }
  }, [status?.completed]);

  const addRepositoryTeam = () => {
    if (repositoryTeams.length >= 2) return;
    const next = { id: makeID('team'), name: 'services', repositoriesText: '' };
    setRepositoryTeams(current => [...current, next]);
  };

  const updateRepositoryTeam = (id: string, updates: Partial<RepositoryTeamDraft>) => {
    setRepositoryEnabled(true);
    setRepositoryTeams(current => current.map(team => (team.id === id ? { ...team, ...updates } : team)));
    setTemplates(null);
  };

  const removeRepositoryTeam = (id: string) => {
    setRepositoryTeams(current => {
      const next = current.filter(team => team.id !== id);
      return next.length > 0 ? next : initialRepositoryTeams().slice(0, 1);
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
        team: teamOptions[0] || '',
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
    normalizedRepositoryTeams.forEach(team => {
      params.append('repository_team', `${team.name}:${team.repositories.join(',')}`);
    });
    if (usersEnabled) {
      users
        .map(user => ({
          sub: user.email.trim(),
          email: user.email.trim(),
          role: user.role,
          team: user.team,
        }))
        .filter(user => user.sub)
        .forEach(user => params.append('setup_user', JSON.stringify(user)));
    }
    params.set('include_llm', aiEnabled ? 'true' : 'false');
    params.set('mcp_examples', aiEnabled && mcpExamples ? 'true' : 'false');
    if (aiEnabled) {
      params.set('llm_provider', llmProvider.trim());
      params.set('llm_model', llmModel.trim());
      if (llmProviderDefinition.apiKeyMode !== 'none') params.set('llm_credential_ref', llmReference);
      if (llmProviderDefinition.baseURLMode !== 'hidden' && llmBaseURL.trim()) params.set('llm_base_url', llmBaseURL.trim());
    }
    return params;
  }, [aiEnabled, llmBaseURL, llmModel, llmProvider, llmProviderDefinition.apiKeyMode, llmProviderDefinition.baseURLMode, llmReference, mcpExamples, normalizedRepositoryTeams, repositories, users, usersEnabled]);

  const loadTemplates = useCallback(async () => {
    setTemplateLoading(true);
    setError(null);
    try {
      const params = buildTemplateParams();
      const payload = await fetchSetupTemplates(params);
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
      const blob = await downloadSetupTemplatesZip(params);
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
          team: user.team,
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
      `GIT_BOT_API_URL=${gitBotServiceURL.trim() || currentRuntimeDefaults.gitBotServiceURL}`,
      runtimeImplementation === 'docker' ? 'DOCKER_NETWORK_NAME=nopsai-net' : 'RUNTIME=kubernetes',
    ];
    const gitBotLines = [
      `NOPSAI_API_URL=${nopsaiAPIURL.trim() || currentRuntimeDefaults.nopsaiAPIURL}`,
      'GIT_BOT_SERVICE_ID=git-bot',
    ];
    return [
      { title: 'Shared by nopsai, git-bot, dispatcher, runner, and aaa', fileName: 'shared.env', lines: sharedLines },
      { title: 'nopsai container', fileName: 'nopsai.env', lines: nopsaiLines },
      { title: 'git-bot container', fileName: 'git-bot.env', lines: gitBotLines },
    ];
  }, [
    currentRuntimeDefaults.gitBotServiceURL,
    currentRuntimeDefaults.nopsaiAPIURL,
    gitBotServiceURL,
    nopsaiAPIURL,
    runtimeImplementation,
  ]);

  const environmentSnippet = useMemo(
    () => runtimeEnvSections.map(section => [`# ${section.title}`, ...section.lines].join('\n')).join('\n\n'),
    [runtimeEnvSections]
  );

  const gitOpsStructureSnippet = useMemo(
    () => buildSetupGitOpsStructurePreview(normalizedRepositoryTeams),
    [normalizedRepositoryTeams]
  );

  const gitOpsFiles = useMemo(
    () =>
      buildSetupGitOpsFileList(normalizedRepositoryTeams, repositories, {
        includeLLM: aiEnabled,
        includeMCP: aiEnabled && mcpExamples,
      }),
    [aiEnabled, mcpExamples, normalizedRepositoryTeams, repositories]
  );

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
      const payload = await bootstrapSetup({
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
        repository_teams: normalizedRepositoryTeams,
        repositories,
        model: {
          name: 'standard',
          provider: llmProvider.trim(),
          model: llmModel.trim(),
          base_url: llmProviderDefinition.baseURLMode === 'hidden' ? '' : llmBaseURL.trim(),
          credential_ref: aiEnabled && llmProviderDefinition.apiKeyMode !== 'none' && (llmProviderDefinition.apiKeyMode === 'required' || llmAPIKey.trim()) ? llmReference : '',
          api_key_value: aiEnabled ? llmAPIKey.trim() : '',
          allowed_scopes: ['dev', 'prod'],
        },
        users: userPayload,
      });
      setBootstrapResult(payload);
      setStatus(payload.status);
      onStatusChange?.(payload.status);
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
    llmBaseURL,
    llmModel,
    llmProvider,
    llmProviderDefinition.apiKeyMode,
    llmProviderDefinition.baseURLMode,
    llmReference,
    mcpExamples,
    normalizedRepositoryTeams,
    repositories,
    repoURL,
    saving,
    onStatusChange,
    syncConfigRepository,
    userPayload,
  ]);

  const renderWizardStep = () => {
    switch (currentWizardStep.id) {
      case 'readiness':
        return (
          <div className="space-y-4">
            <StepIntro title="Confirm the control plane is ready" icon={<ShieldCheck className="h-4 w-4" />}>
              These checks make sure the database, admin account, signing keys, runner path, and starter resources are visible before real automation starts. Blocking errors must be resolved before setup can continue.
            </StepIntro>
            <div className="grid gap-3 md:grid-cols-2">
              {(status?.checks || []).map(check => (
                <div key={check.id} className={`rounded-lg border p-3 ${statusClasses(check.status)}`}>
                  <div className="flex items-center gap-2 text-sm font-semibold">
                    <SetupStatusIcon status={check.status} />
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
              NopsAI needs shared signing, dispatcher, webhook, and service URLs so the UI, API, git-bot, dispatcher, runners, and agents can trust each other. Choose the runtime address family that matches this installation.
            </StepIntro>
            <div className="grid gap-3 md:grid-cols-2">
              <button type="button" onClick={() => updateRuntimeImplementation('docker')} className={`rounded-lg border p-3 text-left ${runtimeImplementation === 'docker' ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}>
                <div className="text-sm font-semibold">Docker Compose</div>
                <div className="mt-1 text-xs leading-5 text-[var(--text-secondary)]">Use container DNS names like `nopsai` and `git-bot` on the `nopsai-net` bridge network.</div>
              </button>
              <button type="button" onClick={() => updateRuntimeImplementation('kubernetes')} className={`rounded-lg border p-3 text-left ${runtimeImplementation === 'kubernetes' ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}>
                <div className="flex items-center justify-between gap-2 text-sm font-semibold">
                  <span>Kubernetes</span>
                  <span className="rounded border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] uppercase text-emerald-700 dark:text-emerald-300">Available</span>
                </div>
                <div className="mt-1 text-xs leading-5 text-[var(--text-secondary)]">Use cluster DNS names like `nopsai.nopsai.svc.cluster.local` and `nopsai-git-bot.nopsai.svc.cluster.local`.</div>
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
                <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">For Docker Compose this should stay `http://git-bot:8081`; configure public GitHub webhook access on the git-bot deployment.</span>
              </label>
            </div>
          </div>
        );
      case 'github':
        return <SetupGitHubStep canManage={canManage} />;
      case 'gitops':
        return (
          <div className="space-y-4">
            <StepIntro title="Connect a GitOps source of truth" icon={<GitBranch className="h-4 w-4" />}>
              The global config repository stores reviewable workspace definitions: teams, starter pipeline, reusable step, repository triggers, scopes, access grants, knowledge docs, LLM profile, and MCP examples. You can skip this for a quick introduction and connect GitOps later.
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
      case 'repositories':
        return (
          <div className="space-y-4">
            <StepIntro title="Create one or two repository teams" icon={<Users className="h-4 w-4" />}>
              This is an introduction, not a full migration. Create one or two top-level teams and add app repositories as GitHub `owner/repo` names, for example `acme/service-api`. NopsAI uses the repository URL identity for starter triggers, pipeline-run navigation, and user access assignments.
            </StepIntro>
            <label className="flex items-center gap-2 rounded-md border border-[var(--border-primary)] p-3 text-sm">
              <input type="checkbox" checked={repositoryEnabled} onChange={event => setRepositoryEnabled(event.target.checked)} disabled={!canManage} />
              Create starter repository teams
            </label>
            <div className="grid gap-3 lg:grid-cols-2">
              {repositoryTeams.map(team => (
                <div key={team.id} className="rounded-lg border border-[var(--border-primary)] p-3">
                  <div className="mb-3 flex items-center justify-between gap-2">
                    <label className="min-w-0 flex-1 space-y-1">
                      <span className="text-xs text-[var(--text-secondary)]">Team name</span>
                      <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={team.name} onChange={event => updateRepositoryTeam(team.id, { name: event.target.value })} disabled={!canManage || !repositoryEnabled} />
                    </label>
                    <button className="rounded-md border border-[var(--border-primary)] p-2 disabled:opacity-50" onClick={() => removeRepositoryTeam(team.id)} disabled={!canManage || repositoryTeams.length <= 1} title="Remove team">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                  <label className="space-y-1">
                    <span className="text-xs text-[var(--text-secondary)]">Repositories, one GitHub `owner/repo` per line</span>
                    <textarea className="min-h-32 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 font-mono text-xs" value={team.repositoriesText} onChange={event => updateRepositoryTeam(team.id, { repositoriesText: event.target.value })} placeholder="acme/service-api&#10;acme/web-app" disabled={!canManage || !repositoryEnabled} />
                    <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">HTTPS and SSH GitHub URLs are accepted; generated structure stores apps with a repository URL.</span>
                  </label>
                </div>
              ))}
            </div>
            <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm disabled:opacity-50" onClick={addRepositoryTeam} disabled={!canManage || repositoryTeams.length >= 2}>
              <Plus className="h-4 w-4" />
              Add team
            </button>
          </div>
        );
      case 'ai':
        return (
          <div className="space-y-4">
            <StepIntro title="Configure the default AI profile" icon={<Bot className="h-4 w-4" />}>
              This creates one default LLM profile named `standard` so goal-based steps can work immediately. Hosted providers use secret references; local providers can run without credentials when their endpoint allows it.
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
                  const previousDefinition = getLLMProvider(llmProvider);
                  const nextDefinition = getLLMProvider(nextProvider);
                  setLLMProvider(nextProvider);
                  setLLMModel(current => replaceProviderDefault(current, previousDefinition.defaultModel, nextDefinition.defaultModel));
                  setLLMBaseURL(current => replaceProviderDefault(current, previousDefinition.defaultBaseURL, nextDefinition.defaultBaseURL));
                  setLLMCredentialRef(current => replaceProviderDefault(current, previousDefinition.defaultCredentialRef, nextDefinition.defaultCredentialRef));
                }} disabled={!canManage || !aiEnabled}>
                  {LLM_PROVIDERS.map(provider => <option key={provider.id} value={provider.id}>{provider.label}</option>)}
                </select>
              </label>
              <label className="space-y-1 text-sm">
                <span className="text-xs text-[var(--text-secondary)]">Model</span>
                <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmModel} onChange={event => setLLMModel(event.target.value)} disabled={!canManage || !aiEnabled} />
              </label>
              {llmProviderDefinition.baseURLMode !== 'hidden' && (
                <label className="space-y-1 text-sm">
                  <span className="text-xs text-[var(--text-secondary)]">Base URL{llmProviderDefinition.baseURLMode === 'required' ? ' *' : ''}</span>
                  <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmBaseURL} onChange={event => setLLMBaseURL(event.target.value)} placeholder={llmProviderDefinition.defaultBaseURL || 'https://resource.openai.azure.com'} disabled={!canManage || !aiEnabled} />
                  <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Use a URL the agent container can reach.</span>
                </label>
              )}
              {llmProviderDefinition.apiKeyMode !== 'none' && (
                <>
                  <label className="space-y-1 text-sm">
                    <span className="text-xs text-[var(--text-secondary)]">Credential reference{llmProviderDefinition.apiKeyMode === 'required' ? ' *' : ''}</span>
                    <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmCredentialRef} onChange={event => setLLMCredentialRef(event.target.value)} placeholder={defaultCredentialRef(llmProvider)} disabled={!canManage || !aiEnabled} />
                    <span className="block text-[11px] leading-5 text-[var(--text-secondary)]">Expected type: api_key</span>
                  </label>
                  <label className="space-y-1 text-sm">
                    <span className="text-xs text-[var(--text-secondary)]">{llmProviderDefinition.apiKeyMode === 'required' ? `${llmProviderDefinition.label} API key value` : 'Optional API key value'}</span>
                    <input className="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2" value={llmAPIKey} onChange={event => setLLMAPIKey(event.target.value)} type="password" placeholder={llmProviderDefinition.apiKeyMode === 'optional' ? `Optional for ${llmProviderDefinition.label}` : 'Paste key to seed the secret'} disabled={!canManage || !aiEnabled} />
                  </label>
                </>
              )}
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
            <StepIntro title="Create starter users and assign team roles" icon={<Users className="h-4 w-4" />}>
              Add a small set of users for the teams above. Each user gets a product role on the selected team. Passwords are temporary and users must change them on first login; leave a password blank when you want NopsAI to generate one.
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
                  <select className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={user.team} onChange={event => updateUser(user.id, { team: event.target.value })} disabled={!canManage || !usersEnabled}>
                    {userTeamOptions.map(team => <option key={team || 'no-team'} value={team}>{team || 'No team selected'}</option>)}
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
          <SetupReviewOutput
            aiEnabled={aiEnabled}
            normalizedRepositoryTeams={normalizedRepositoryTeams}
            repositories={repositories}
            userCount={userPayload.length}
            runtimeEnvSections={runtimeEnvSections}
            environmentSnippet={environmentSnippet}
            gitOpsStructureSnippet={gitOpsStructureSnippet}
            gitOpsFiles={gitOpsFiles}
            templateLoading={templateLoading}
            templatesLoaded={Boolean(templates)}
            downloadingGitOpsZip={downloadingGitOpsZip}
            onLoadTemplates={() => void loadTemplates()}
            onDownloadGitOpsZip={() => void downloadGitOpsZip()}
          />
        );
    }
  };

  const wizardModal = status && !status.completed ? (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="flex max-h-[92vh] w-full max-w-6xl flex-col overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-xl">
        <div className="border-b border-[var(--border-primary)] p-4">
          <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <div>
              <div className="text-xs uppercase tracking-wide text-[var(--text-secondary)]">First-install wizard</div>
              <h2 className="text-xl font-semibold">{currentWizardStep.label}</h2>
            </div>
            <div className="text-sm text-[var(--text-secondary)]">Step {wizardStepIndex + 1} of {WIZARD_STEPS.length}</div>
          </div>
          <div className="mt-4">
            <SetupStepNavigation wizardStepIndex={wizardStepIndex} onSelectStep={setWizardStepIndex} />
          </div>
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

      <SetupStatusOverview status={status} />
      <SetupBootstrapResult bootstrapResult={bootstrapResult} />

      <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
        <div className="mb-4 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="text-sm font-semibold">Setup details</div>
            <div className="mt-1 text-xs text-[var(--text-secondary)]">Review every setup step, generated env block, and GitOps file preview anytime.</div>
          </div>
          <div className="text-sm text-[var(--text-secondary)]">Step {wizardStepIndex + 1} of {WIZARD_STEPS.length}</div>
        </div>
        <SetupStepNavigation wizardStepIndex={wizardStepIndex} onSelectStep={setWizardStepIndex} />
        <div className="mt-5">{renderWizardStep()}</div>
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <button className="rounded-md border border-[var(--border-primary)] px-4 py-2 text-sm" onClick={() => setWizardStepIndex(index => Math.max(0, index - 1))} disabled={wizardStepIndex === 0}>Back</button>
          <button className="glass-button-primary" onClick={() => setWizardStepIndex(index => Math.min(WIZARD_STEPS.length - 1, index + 1))} disabled={wizardStepIndex >= WIZARD_STEPS.length - 1}>Continue</button>
        </div>
      </section>

      <SetupStarterFilesPreview
        templates={templates}
        templatePaths={templatePaths}
        selectedTemplatePath={selectedTemplatePath}
        selectedTemplate={selectedTemplate}
        onSelectedTemplatePathChange={setSelectedTemplatePath}
      />
    </div>
  );
}

export default SetupWizard;
