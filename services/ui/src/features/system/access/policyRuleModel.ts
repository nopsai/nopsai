import type { RolePermission } from './model.js';
import type { AccessResourceCatalog, AccessResourceOption } from './resourceCatalog.js';

export const formatAccessResourceSummary = (value: string) => {
  const { resourceType, resourceID } = parseAAAResourceSelector(value);
  if (!resourceType || resourceType === '*') return 'all resources';
  const config = getAAAResourceTypeConfig(resourceType);
  const label = (config?.label || resourceType).toLowerCase();
  if (!resourceID || resourceID === '*') return `all ${label}`;
  return `${label} ${resourceID}`;
};

export const formatAccessActionSummary = (value: string) => {
  const parsed = parseAAAActionValue(value);
  const label = actionLabelFromAAAValue(parsed.action || value) || parsed.action || value || 'action';
  return parsed.effect === 'deny' ? `deny ${label}` : label;
};

export const summarizeRoleCoverage = (policies: RolePermission[]) => {
  const labels = Array.from(
    new Set(
      policies.map(policy => {
        const { resourceType } = parseAAAResourceSelector(policy.obj);
        if (!resourceType || resourceType === '*') return 'All resources';
        return getAAAResourceTypeConfig(resourceType)?.label || resourceType;
      })
    )
  );
  return labels.slice(0, 4);
};

export type AAAEffect = 'allow' | 'deny';

type AAAOption = AccessResourceOption;

type AAAOptionTeam = {
  label: string;
  options: AAAOption[];
};

type AAAResourceCatalog = AccessResourceCatalog;

type AAAResourceTypeConfig = {
  value: string;
  label: string;
  targetLabel: string;
  allowAll?: boolean;
  allLabel?: string;
  presets?: AAAOption[];
  dynamicSource?: keyof AAAResourceCatalog;
  customPlaceholder?: string;
};

export type AAANamedResourceDraft = {
  repoName: string;
  scope: string;
  name: string;
  hasScope: boolean;
};

export const AAA_CUSTOM_VALUE = '__custom__';
export const AAA_ANY_SCOPE_VALUE = '__any_scope__';
const AAA_DEFAULT_SCOPE_VALUE = '__default_scope__';

export const AAA_RESOURCE_TYPE_CONFIGS: AAAResourceTypeConfig[] = [
  {
    value: '*',
    label: 'All resources',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All resources',
  },
  {
    value: 'iam',
    label: 'IAM',
    targetLabel: 'Area',
    presets: [{ value: 'admin', label: 'Admin' }],
    customPlaceholder: 'admin',
  },
  {
    value: 'audit',
    label: 'Audit',
    targetLabel: 'Log',
    presets: [{ value: 'authz', label: 'Authorization log' }],
    customPlaceholder: 'authz',
  },
  {
    value: 'system',
    label: 'System',
    targetLabel: 'Area',
    presets: [
      { value: 'config', label: 'Config' },
      { value: 'config-sync', label: 'Config sync' },
      { value: 'llm-profiles', label: 'LLM profiles' },
      { value: 'agent-profiles', label: 'Agent profiles' },
      { value: 'mcp', label: 'MCP' },
      { value: 'config-repos', label: 'Config repositories' },
      { value: 'steps', label: 'Step catalog' },
    ],
    customPlaceholder: 'config',
  },
  {
    value: 'dispatcher',
    label: 'Dispatcher',
    targetLabel: 'Area',
    presets: [
      { value: 'status', label: 'Status' },
      { value: 'runners', label: 'Runners' },
    ],
    customPlaceholder: 'status',
  },
  {
    value: 'system_log',
    label: 'System log',
    targetLabel: 'Source',
    allowAll: true,
    allLabel: 'All system log sources',
    presets: [
      { value: 'nopsai', label: 'NopsAI API' },
      { value: 'aaa', label: 'AAA' },
      { value: 'dispatcher', label: 'Dispatcher' },
      { value: 'git-bot', label: 'Git bot' },
      { value: 'ui', label: 'UI' },
      { value: 'docker-runner', label: 'Docker runner' },
    ],
    customPlaceholder: 'dispatcher',
  },
  {
    value: 'team',
    label: 'Team',
    targetLabel: 'Team',
    allowAll: true,
    allLabel: 'All teams',
    dynamicSource: 'teamOptions',
    customPlaceholder: 'team/platform',
  },
  {
    value: 'pipeline',
    label: 'Pipeline',
    targetLabel: 'Pipeline',
    allowAll: true,
    allLabel: 'All pipelines',
    dynamicSource: 'pipelineOptions',
    customPlaceholder: 'team/build',
  },
  {
    value: 'pipeline_run',
    label: 'Pipeline run',
    targetLabel: 'Run',
    allowAll: true,
    allLabel: 'All runs',
    customPlaceholder: 'run-123',
  },
  {
    value: 'scope',
    label: 'Scope',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All scopes',
    dynamicSource: 'scopeOptions',
    customPlaceholder: 'prod',
  },
  {
    value: 'trigger',
    label: 'Trigger',
    targetLabel: 'Repository',
    allowAll: true,
    allLabel: 'All triggers',
    dynamicSource: 'triggerOptions',
    customPlaceholder: 'owner/repo',
  },
  {
    value: 'external_trigger',
    label: 'External trigger',
    targetLabel: 'External trigger',
    allowAll: true,
    allLabel: 'All external triggers',
    dynamicSource: 'externalTriggerOptions',
    customPlaceholder: 'deploy-prod',
  },
  {
    value: 'git_webhook_source',
    label: 'Git webhook source',
    targetLabel: 'Git webhook source',
    allowAll: true,
    allLabel: 'All Git webhook sources',
    dynamicSource: 'gitWebhookSourceOptions',
    customPlaceholder: 'gitlab-platform',
  },
  {
    value: 'repository',
    label: 'Repository',
    targetLabel: 'Repository',
    allowAll: true,
    allLabel: 'All repositories',
    dynamicSource: 'repositoryOptions',
    customPlaceholder: 'owner/repo',
  },
  {
    value: 'secret',
    label: 'Secret',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All secrets',
    customPlaceholder: 'repo=owner/repo&scope=prod&name=TOKEN',
  },
  {
    value: 'variable',
    label: 'Variable',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All variables',
    customPlaceholder: 'repo=owner/repo&scope=prod&name=TIMEOUT',
  },
  {
    value: 'llm_profile',
    label: 'LLM profile',
    targetLabel: 'Profile',
    allowAll: true,
    allLabel: 'All LLM profiles',
    customPlaceholder: 'hosted',
  },
  {
    value: 'agent_profile',
    label: 'Agent profile',
    targetLabel: 'Profile',
    allowAll: true,
    allLabel: 'All agent profiles',
    customPlaceholder: 'sre',
  },
  {
    value: 'mcp_server',
    label: 'MCP server',
    targetLabel: 'Server',
    allowAll: true,
    allLabel: 'All MCP servers',
    customPlaceholder: 'github',
  },
  {
    value: 'mcp_profile',
    label: 'MCP profile',
    targetLabel: 'Profile',
    allowAll: true,
    allLabel: 'All MCP profiles',
    customPlaceholder: 'github-pr-review',
  },
  {
    value: 'credential',
    label: 'Credential',
    targetLabel: 'Credential',
    allowAll: true,
    allLabel: 'All credentials',
    dynamicSource: 'credentialOptions',
    customPlaceholder: 'system/llm/openai',
  },
];

const AAA_ALL_ACTION_OPTION_TEAMS: AAAOptionTeam[] = [
  {
    label: 'Global',
    options: [{ value: '*', label: 'All actions (*)' }],
  },
  {
    label: 'Administration',
    options: [
      { value: 'iam.admin', label: 'admin' },
      { value: 'audit.read', label: 'read' },
    ],
  },
  {
    label: 'System',
    options: [
      { value: 'system.read', label: 'read' },
      { value: 'system.update', label: 'update' },
      { value: 'system_log.read', label: 'read system logs' },
    ],
  },
  {
    label: 'Teams',
    options: [
      { value: 'team.list', label: 'list' },
      { value: 'team.create', label: 'create' },
      { value: 'team.move', label: 'move' },
      { value: 'team.update', label: 'update' },
      { value: 'team.delete', label: 'delete' },
      { value: 'config_repo.read', label: 'read config repo' },
      { value: 'config_repo.manage', label: 'manage config repo' },
      { value: 'config_repo.sync', label: 'sync config repo' },
    ],
  },
  {
    label: 'Pipelines',
    options: [
      { value: 'pipeline.list', label: 'list' },
      { value: 'pipeline.read', label: 'read' },
      { value: 'pipeline.create', label: 'create' },
      { value: 'pipeline.update', label: 'update' },
      { value: 'pipeline.delete', label: 'delete' },
      { value: 'pipeline.execute', label: 'execute' },
      { value: 'pipeline.use', label: 'use' },
    ],
  },
  {
    label: 'Pipeline Runs',
    options: [
      { value: 'pipeline_run.list', label: 'list' },
      { value: 'pipeline_run.read', label: 'read' },
      { value: 'pipeline_run.rerun', label: 'rerun' },
      { value: 'pipeline_run.cancel', label: 'cancel' },
      { value: 'pipeline_run.finalize', label: 'finalize' },
      { value: 'pipeline_run.write_logs', label: 'write logs' },
      { value: 'pipeline_run.task_update', label: 'update task' },
      { value: 'pipeline_run.delete', label: 'delete' },
    ],
  },
  {
    label: 'Scopes',
    options: [
      { value: 'scope.read', label: 'read' },
      { value: 'scope.use', label: 'use' },
      { value: 'scope.update', label: 'update' },
      { value: 'scope.delete', label: 'delete' },
      { value: 'scope.manage_acl', label: 'manage ACL' },
    ],
  },
  {
    label: 'Triggers',
    options: [
      { value: 'trigger.read', label: 'read' },
      { value: 'trigger.update', label: 'update' },
      { value: 'trigger.delete', label: 'delete' },
    ],
  },
  {
    label: 'External Triggers',
    options: [
      { value: 'external_trigger.read', label: 'read' },
      { value: 'external_trigger.create', label: 'create' },
      { value: 'external_trigger.update', label: 'update' },
      { value: 'external_trigger.invoke', label: 'invoke' },
      { value: 'external_trigger.delete', label: 'delete' },
      { value: 'external_trigger.manage_acl', label: 'manage ACL' },
    ],
  },
  {
    label: 'Git Webhook Sources',
    options: [
      { value: 'git_webhook_source.read', label: 'read' },
      { value: 'git_webhook_source.create', label: 'create' },
      { value: 'git_webhook_source.update', label: 'update' },
      { value: 'git_webhook_source.delete', label: 'delete' },
      { value: 'git_webhook_source.manage_acl', label: 'manage ACL' },
    ],
  },
  {
    label: 'Credentials',
    options: [
      { value: 'credential.list_metadata', label: 'list metadata' },
      { value: 'credential.create', label: 'create' },
      { value: 'credential.write_value', label: 'write value' },
      { value: 'credential.rotate', label: 'rotate' },
      { value: 'credential.disable', label: 'disable' },
      { value: 'credential.enable', label: 'enable' },
      { value: 'credential.delete_version', label: 'delete version' },
      { value: 'credential.delete', label: 'delete' },
      { value: 'credential.use', label: 'use' },
      { value: 'credential.manage_acl', label: 'manage access' },
    ],
  },
  {
    label: 'Secrets',
    options: [
      { value: 'secret.list_metadata', label: 'list metadata' },
      { value: 'secret.read_value', label: 'read value' },
      { value: 'secret.write_value', label: 'write value' },
      { value: 'secret.delete', label: 'delete' },
    ],
  },
  {
    label: 'Variables',
    options: [
      { value: 'variable.list_metadata', label: 'list metadata' },
      { value: 'variable.read_value', label: 'read value' },
      { value: 'variable.write_value', label: 'write value' },
      { value: 'variable.delete', label: 'delete' },
    ],
  },
  {
    label: 'AI Profiles',
    options: [
      { value: 'llm_profile.read', label: 'read LLM profile' },
      { value: 'llm_profile.use', label: 'use LLM profile' },
      { value: 'llm_profile.manage_acl', label: 'manage LLM access' },
      { value: 'agent_profile.read', label: 'read agent profile' },
      { value: 'agent_profile.use', label: 'use agent profile' },
      { value: 'agent_profile.manage_acl', label: 'manage agent access' },
      { value: 'mcp_server.read', label: 'read MCP server' },
      { value: 'mcp_server.use', label: 'use MCP server' },
      { value: 'mcp_server.manage_acl', label: 'manage MCP server access' },
      { value: 'mcp_profile.read', label: 'read MCP profile' },
      { value: 'mcp_profile.use', label: 'use MCP profile' },
      { value: 'mcp_profile.manage_acl', label: 'manage MCP profile access' },
    ],
  },
];

const AAA_ACTION_OPTION_TEAMS_BY_SELECTOR: Record<string, AAAOptionTeam[]> = {
  '*:*': AAA_ALL_ACTION_OPTION_TEAMS,
  'iam:admin': [{ label: 'IAM actions', options: [{ value: 'iam.admin', label: 'admin' }] }],
  'audit:authz': [{ label: 'Audit actions', options: [{ value: 'audit.read', label: 'read' }] }],
  'system:config': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:config-sync': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:llm-profiles': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:agent-profiles': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:steps': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'dispatcher:status': [{ label: 'Dispatcher actions', options: [{ value: 'system.read', label: 'read' }] }],
  'dispatcher:runners': [{ label: 'Dispatcher actions', options: [{ value: 'system.update', label: 'update' }] }],
  'system_log:*': [{ label: 'System log actions', options: [{ value: 'system_log.read', label: 'read' }] }],
  'repository:*': [{ label: 'Repository actions', options: [{ value: 'system.read', label: 'read' }] }],
};

const aiProfileActionOptions = (prefix: string) =>
  AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'AI Profiles')?.options.filter(option => option.value.startsWith(prefix)) || [];

const AAA_ACTION_OPTION_TEAMS_BY_RESOURCE_TYPE: Record<string, AAAOptionTeam[]> = {
  '*': AAA_ALL_ACTION_OPTION_TEAMS,
  team: [{ label: 'Team actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Teams')?.options || [] }],
  pipeline: [{ label: 'Pipeline actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Pipelines')?.options || [] }],
  pipeline_run: [{ label: 'Pipeline run actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Pipeline Runs')?.options || [] }],
  scope: [{ label: 'Scope actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Scopes')?.options || [] }],
  trigger: [{ label: 'Trigger actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Triggers')?.options || [] }],
  external_trigger: [{ label: 'External trigger actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'External Triggers')?.options || [] }],
  git_webhook_source: [{ label: 'Git webhook source actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Git Webhook Sources')?.options || [] }],
  secret: [{ label: 'Secret actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Secrets')?.options || [] }],
  variable: [{ label: 'Variable actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Variables')?.options || [] }],
  llm_profile: [{ label: 'LLM profile actions', options: aiProfileActionOptions('llm_profile.') }],
  agent_profile: [{ label: 'Agent profile actions', options: aiProfileActionOptions('agent_profile.') }],
  mcp_server: [{ label: 'MCP server actions', options: aiProfileActionOptions('mcp_server.') }],
  mcp_profile: [{ label: 'MCP profile actions', options: aiProfileActionOptions('mcp_profile.') }],
  credential: [{ label: 'Credential actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'Credentials')?.options || [] }],
  system: [{ label: 'System actions', options: AAA_ALL_ACTION_OPTION_TEAMS.find(team => team.label === 'System')?.options || [] }],
  system_log: [{ label: 'System log actions', options: [{ value: 'system_log.read', label: 'read' }] }],
  repository: [{ label: 'Repository actions', options: [{ value: 'system.read', label: 'read' }] }],
  audit: [{ label: 'Audit actions', options: [{ value: 'audit.read', label: 'read' }] }],
  iam: [{ label: 'IAM actions', options: [{ value: 'iam.admin', label: 'admin' }] }],
};

export const getAAAResourceTypeConfig = (resourceType: string) =>
  AAA_RESOURCE_TYPE_CONFIGS.find(config => config.value === resourceType);

const dedupeAAAOptions = (options: AAAOption[]) => {
  const seen = new Set<string>();
  return options.filter(option => {
    const value = option.value.trim();
    if (!value || seen.has(value)) return false;
    seen.add(value);
    return true;
  });
};

const hasAAAOptionValue = (teams: AAAOptionTeam[], value: string) => {
  const normalized = (value || '').trim();
  return teams.some(team => team.options.some(option => option.value === normalized));
};

export const parseAAAResourceSelector = (value: string): { resourceType: string; resourceID: string } => {
  const normalized = (value || '').trim();
  if (!normalized) return { resourceType: '', resourceID: '' };
  if (normalized === '*:*' || normalized === '*') return { resourceType: '*', resourceID: '*' };
  const separatorIndex = normalized.indexOf(':');
  const resourceType = separatorIndex >= 0 ? normalized.slice(0, separatorIndex) : normalized;
  const resourceID = separatorIndex >= 0 ? normalized.slice(separatorIndex + 1) : '*';
  return {
    resourceType: (resourceType || '').trim(),
    resourceID: separatorIndex >= 0 ? resourceID.trim() : '*',
  };
};

export const buildAAAResourceSelector = (resourceType: string, resourceID: string, opts?: { preserveEmpty?: boolean }) => {
  const normalizedType = (resourceType || '').trim();
  const normalizedID = (resourceID || '').trim();
  if (!normalizedType) {
    return normalizedID;
  }
  if (normalizedType === '*') {
    return '*:*';
  }
  if (!normalizedID) {
    return opts?.preserveEmpty ? `${normalizedType}:` : `${normalizedType}:*`;
  }
  if (normalizedID === '*') {
    return `${normalizedType}:*`;
  }
  return `${normalizedType}:${normalizedID}`;
};

export const flattenAAAOptionTeams = (teams: AAAOptionTeam[]) => teams.flatMap(team => team.options);

export const normalizeAAAScopeOptionValue = (scope: string) => {
  const normalized = (scope || '').trim().replace(/^\/+|\/+$/g, '');
  return !normalized || normalized.toLowerCase() === 'default' ? AAA_DEFAULT_SCOPE_VALUE : normalized;
};

export const denormalizeAAAScopeOptionValue = (value: string) => (value === AAA_DEFAULT_SCOPE_VALUE ? '' : (value || '').trim());

export const parseAAANamedResourceID = (value: string): AAANamedResourceDraft => {
  const normalized = (value || '').trim();
  if (!normalized || normalized === '*') {
    return {
      repoName: '',
      scope: '',
      name: '',
      hasScope: false,
    };
  }

  const params = new URLSearchParams(normalized);
  return {
    repoName: (params.get('repo') || '').trim(),
    scope: (params.get('scope') || '').trim(),
    name: (params.get('name') || '').trim(),
    hasScope: params.has('scope'),
  };
};

const buildAAANamedResourceID = ({ repoName, scope, name, hasScope }: AAANamedResourceDraft) => {
  const params = new URLSearchParams();
  const normalizedRepoName = (repoName || '').trim();
  const normalizedScope = (scope || '').trim();
  if (normalizedRepoName) {
    params.set('repo', normalizedRepoName);
  }
  if (hasScope) {
    params.set('scope', normalizedScope);
  }
  const normalizedName = (name || '').trim();
  if (normalizedName) {
    params.set('name', normalizedName);
  }
  return params.toString() || '*';
};

export const buildAAANamedResourceSelector = (resourceType: string, parts: AAANamedResourceDraft) =>
  buildAAAResourceSelector(resourceType, buildAAANamedResourceID(parts));

const buildAAAParentPathOptionTeams = (options: AAAOption[], labels: { root: string; parentPrefix: string }) => {
  const teams = new Map<string, { sortKey: string; label: string; options: AAAOption[] }>();

  options.forEach(option => {
    const normalizedValue = option.value.trim();
    if (!normalizedValue) return;

    const lastSlash = normalizedValue.lastIndexOf('/');
    const parentPath = lastSlash >= 0 ? normalizedValue.slice(0, lastSlash) : '';
    const itemLabel = lastSlash >= 0 ? normalizedValue.slice(lastSlash + 1) : normalizedValue;
    const teamKey = parentPath || '';
    const teamLabel = parentPath ? `${labels.parentPrefix}${parentPath}` : labels.root;
    const existing = teams.get(teamKey);

    if (existing) {
      existing.options.push({ value: normalizedValue, label: itemLabel || option.label || normalizedValue });
      return;
    }

    teams.set(teamKey, {
      sortKey: teamKey,
      label: teamLabel,
      options: [{ value: normalizedValue, label: itemLabel || option.label || normalizedValue }],
    });
  });

  return Array.from(teams.values())
    .sort((a, b) => {
      if (!a.sortKey) return -1;
      if (!b.sortKey) return 1;
      return a.sortKey.localeCompare(b.sortKey);
    })
    .map(team => ({
      label: team.label,
      options: team.options.sort((a, b) => a.label.localeCompare(b.label)),
    }));
};

const buildAAARepositoryOptionTeams = (options: AAAOption[], labels: { root: string; ownerPrefix: string }) => {
  const teams = new Map<string, { sortKey: string; label: string; options: AAAOption[] }>();

  options.forEach(option => {
    const normalizedValue = option.value.trim();
    if (!normalizedValue) return;

    const separatorIndex = normalizedValue.indexOf('/');
    const owner = separatorIndex >= 0 ? normalizedValue.slice(0, separatorIndex) : '';
    const repoName = separatorIndex >= 0 ? normalizedValue.slice(separatorIndex + 1) : normalizedValue;
    const teamKey = owner || '';
    const teamLabel = owner ? `${labels.ownerPrefix}${owner}` : labels.root;
    const existing = teams.get(teamKey);

    if (existing) {
      existing.options.push({ value: normalizedValue, label: repoName || option.label || normalizedValue });
      return;
    }

    teams.set(teamKey, {
      sortKey: teamKey,
      label: teamLabel,
      options: [{ value: normalizedValue, label: repoName || option.label || normalizedValue }],
    });
  });

  return Array.from(teams.values())
    .sort((a, b) => {
      if (!a.sortKey) return -1;
      if (!b.sortKey) return 1;
      return a.sortKey.localeCompare(b.sortKey);
    })
    .map(team => ({
      label: team.label,
      options: team.options.sort((a, b) => a.label.localeCompare(b.label)),
    }));
};

export const buildAAAResourceTargetOptionTeams = (config: AAAResourceTypeConfig, catalog: AAAResourceCatalog) => {
  const teams: AAAOptionTeam[] = [];
  const scopeOptions: AAAOption[] = [];
  if (config.allowAll) {
    scopeOptions.push({ value: '*', label: config.allLabel || 'All' });
  }
  if (config.presets) {
    scopeOptions.push(...config.presets);
  }

  const normalizedScopeOptions = dedupeAAAOptions(scopeOptions);
  if (normalizedScopeOptions.length > 0) {
    teams.push({
      label: config.dynamicSource ? 'Scope' : 'Available targets',
      options: normalizedScopeOptions,
    });
  }

  if (config.dynamicSource) {
    const dynamicOptions = dedupeAAAOptions(catalog[config.dynamicSource]);
    switch (config.dynamicSource) {
      case 'teamOptions':
        teams.push(...buildAAAParentPathOptionTeams(dynamicOptions, { root: 'Top-level teams', parentPrefix: 'Inside /' }));
        break;
      case 'pipelineOptions':
        teams.push(...buildAAAParentPathOptionTeams(dynamicOptions, { root: 'Top-level pipelines', parentPrefix: 'Team /' }));
        break;
      case 'scopeOptions':
        teams.push(...buildAAAParentPathOptionTeams(dynamicOptions, { root: 'Top-level scopes', parentPrefix: 'Team /' }));
        break;
      case 'triggerOptions':
        teams.push(...buildAAARepositoryOptionTeams(dynamicOptions, { root: 'Unteamed triggers', ownerPrefix: 'Owner ' }));
        break;
      case 'externalTriggerOptions':
        teams.push(...buildAAAParentPathOptionTeams(dynamicOptions, { root: 'External triggers', parentPrefix: 'Team /' }));
        break;
      case 'repositoryOptions':
        teams.push(...buildAAARepositoryOptionTeams(dynamicOptions, { root: 'Unteamed repositories', ownerPrefix: 'Owner ' }));
        break;
      default:
        if (dynamicOptions.length > 0) {
          teams.push({ label: 'Known targets', options: dynamicOptions });
        }
        break;
    }
  }

  return teams;
};

export const getAAAActionOptionTeams = (resourceSelector: string): AAAOptionTeam[] => {
  const normalized = (resourceSelector || '').trim();
  if (!normalized) return [];
  if (AAA_ACTION_OPTION_TEAMS_BY_SELECTOR[normalized]) {
    return AAA_ACTION_OPTION_TEAMS_BY_SELECTOR[normalized];
  }
  const { resourceType } = parseAAAResourceSelector(normalized);
  return AAA_ACTION_OPTION_TEAMS_BY_RESOURCE_TYPE[resourceType] || [];
};

const actionLabelFromAAAValue = (value: string) => {
  const trimmed = (value || '').trim();
  if (!trimmed || trimmed === '*') return trimmed;
  const actionPart = trimmed.includes('.') ? trimmed.slice(trimmed.lastIndexOf('.') + 1) : trimmed;
  switch (actionPart) {
    case 'list_metadata':
      return 'list metadata';
    case 'read_value':
      return 'read value';
    case 'write_value':
      return 'write value';
    case 'write_logs':
      return 'write logs';
    case 'task_update':
      return 'update task';
    default:
      return actionPart.replace(/_/g, ' ');
  }
};

export const normalizeAAAActionForResource = (resourceSelector: string, actionValue: string, effect: AAAEffect) => {
  const options = flattenAAAOptionTeams(getAAAActionOptionTeams(resourceSelector));
  if (options.length === 0) return formatAAAActionValue(effect, actionValue);
  const trimmed = (actionValue || '').trim();
  if (trimmed && options.some(option => option.value === trimmed)) {
    return formatAAAActionValue(effect, trimmed);
  }
  const currentLabel = actionLabelFromAAAValue(trimmed);
  if (currentLabel) {
    const matchingVerb = options.find(option => option.label === currentLabel);
    if (matchingVerb) {
      return formatAAAActionValue(effect, matchingVerb.value);
    }
  }
  return formatAAAActionValue(effect, options[0].value);
};

export const customAAAActionPlaceholder = (resourceSelector: string) => {
  const options = flattenAAAOptionTeams(getAAAActionOptionTeams(resourceSelector));
  if (options.length > 0) return options[0].value;
  const { resourceType } = parseAAAResourceSelector(resourceSelector);
  if (!resourceType || resourceType === '*') return 'pipeline.read';
  return `${resourceType}.read`;
};

export const parseAAAActionValue = (value: string): { effect: AAAEffect; action: string } => {
  const trimmed = (value || '').trim();
  if (!trimmed) return { effect: 'allow', action: '' };
  if (trimmed.startsWith('deny ')) {
    return {
      effect: 'deny',
      action: trimmed.slice('deny '.length).trim(),
    };
  }
  return { effect: 'allow', action: trimmed };
};

export const formatAAAActionValue = (effect: AAAEffect, action: string) => {
  const trimmed = (action || '').trim();
  if (!trimmed) return '';
  return effect === 'deny' ? `deny ${trimmed}` : trimmed;
};

export const selectValueForAAAOptions = (teams: AAAOptionTeam[], value: string) =>
  hasAAAOptionValue(teams, value) ? (value || '').trim() : AAA_CUSTOM_VALUE;
