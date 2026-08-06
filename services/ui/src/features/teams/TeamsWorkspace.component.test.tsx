import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { Team } from '../../lib/teamModels';
import { TeamsWorkspace } from './TeamsWorkspace';
import { defaultNotificationRouteDefinition } from './notificationRoutes';
import type { TeamOperationsSummaryState } from './hooks/useTeamOperationsSummary';
import type { TeamResourceCatalogState } from './resourceCatalogModel';

const teams: Team[] = [
  {
    id: 1,
    name: 'platform',
    kind: 'team',
    path: 'platform',
    description: 'Platform engineering',
    parent_id: null,
  },
  {
    id: 2,
    name: 'payments',
    kind: 'team',
    path: 'platform/payments',
    description: 'Payment systems',
    parent_id: 1,
  },
  {
    id: 3,
    name: 'checkout-api',
    kind: 'app',
    path: 'platform/payments/checkout-api',
    team_path: 'platform/payments',
    parent_id: 2,
    repository_full_name: 'acme/checkout-api',
    repo_url: 'https://github.com/acme/checkout-api',
    last_run_at: '2026-07-10T10:00:00Z',
  },
  {
    id: 4,
    name: 'docs',
    kind: 'app',
    path: 'platform/docs',
    team_path: 'platform',
    parent_id: 1,
    repository_full_name: 'acme/docs',
  },
];

const operationsSummary: TeamOperationsSummaryState = {
  teamPath: 'platform',
  loading: false,
  configRepo: {
    id: 9,
    scope_type: 'team',
    scope_id: 'platform',
    provider: 'github',
    repo_url: 'https://github.com/acme/platform-config',
    branch: 'main',
    base_path: 'teams/platform',
    credential_ref: '',
    enabled: true,
    write_enabled: true,
    write_branch: 'nopsai/team-updates',
    last_sync_status: 'success',
    last_sync_completed_at: '2026-07-10T10:01:00Z',
    last_sync_commit_sha: 'abc123',
  },
  configRepoError: null,
  notificationRoute: {
    id: 22,
    team_path: 'platform',
    definition: defaultNotificationRouteDefinition(),
    source: 'database',
    managed_by_config_repo: false,
  },
  notificationError: null,
  llmProfiles: {
    team_id: 1,
    team_path: 'platform',
    default_profile: 'fast',
    profiles: [{
      name: 'fast',
      provider: 'openai',
      model: 'gpt-4.1-mini',
      credential_ref: 'credential://openai/default',
      allowed_scopes: ['assistant'],
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    }],
  },
  agentProfiles: {
    team_id: 1,
    team_path: 'platform',
    default_profile: 'reviewer',
    profiles: [
      {
        id: 'devops-engineer',
        display_name: 'DevOps Engineer',
        role: 'operations',
        instructions: 'Operate deployments',
        enabled: true,
        source: 'built-in',
        scope: 'global',
        team_id: 0,
        team_path: '',
      },
      {
        id: 'reviewer',
        display_name: 'Reviewer',
        role: 'review',
        instructions: 'Review changes',
        enabled: true,
        scope: 'team',
        team_id: 1,
        team_path: 'platform',
      },
    ],
  },
  mcpProfiles: {
    team_id: 1,
    team_path: 'platform',
    profiles: [{
      name: 'engineering-tools',
      enabled: true,
      servers: [{ server: 'github', tools: ['issues'] }],
      allowed_scopes: ['assistant'],
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    }],
  },
  aiProfilesError: null,
  accessGrants: [{
    id: 'grant-1',
    subjectType: 'user',
    subjectID: 'alice',
    subjectDisplay: 'Alice Admin',
    role: 'owner',
    resourceType: 'team',
    resourceID: 'platform',
    inherit: true,
  }],
  accessGrantsError: null,
  permissions: [
    { action: 'team.read', label: 'View team', allowed: true },
    { action: 'team.manage_acl', label: 'Manage access', allowed: false },
  ],
  permissionsError: null,
};

const rootOperationsSummary: TeamOperationsSummaryState = {
  ...operationsSummary,
  teamPath: '',
  configRepo: null,
  configRepoError: null,
  notificationRoute: null,
  notificationError: null,
  llmProfiles: {
    team_id: 0,
    team_path: '',
    default_profile: 'hosted',
    profiles: [{
      name: 'hosted',
      provider: 'openai',
      model: 'gpt-4.1-mini',
      credential_ref: 'credential://system/llm/openai',
      allowed_scopes: ['assistant'],
      scope: 'global',
      team_id: 0,
      team_path: '',
    }],
  },
  agentProfiles: {
    team_id: 0,
    team_path: '',
    default_profile: 'devops-engineer',
    profiles: [{
      id: 'devops-engineer',
      display_name: 'DevOps Engineer',
      role: 'operations',
      instructions: 'Operate deployments',
      enabled: true,
      scope: 'global',
      team_id: 0,
      team_path: '',
    }],
  },
  mcpProfiles: {
    team_id: 0,
    team_path: '',
    profiles: [{
      name: 'github-global',
      enabled: true,
      servers: [{ server: 'github', tools: ['issues'] }],
      allowed_scopes: ['assistant'],
      scope: 'global',
      team_id: 0,
      team_path: '',
    }],
  },
  aiProfilesError: null,
  accessGrants: [{
    id: 'grant-platform-admin',
    subjectType: 'user',
    subjectID: 'platform-admin',
    subjectDisplay: 'Platform Admin',
    role: 'admin',
    resourceType: 'platform',
    resourceID: 'platform',
    inherit: false,
  }],
  accessGrantsError: null,
  permissions: [
    { action: 'system.read', label: 'View system', allowed: true },
    { action: 'system.update', label: 'Update system', allowed: true },
  ],
  permissionsError: null,
};

const resourceCatalog: TeamResourceCatalogState = {
  teamPath: 'platform',
  loading: false,
  error: null,
  resources: [
    {
      id: 'pipeline:platform/deploy-api',
      kind: 'pipeline',
      label: 'deploy-api',
      description: 'Pipeline in platform',
      href: '/pipelines/platform/deploy-api',
      teamPath: 'platform',
      source: 'git',
    },
    {
      id: 'step:platform/build',
      kind: 'step',
      label: 'build',
      description: 'Step in platform',
      href: '/steps/platform/build',
      teamPath: 'platform',
      source: 'git',
    },
    {
      id: 'trigger:platform/acme/checkout-api',
      kind: 'trigger',
      label: 'checkout-api',
      description: 'Trigger in platform/payments',
      href: '/triggers/platform/acme/checkout-api',
      teamPath: 'platform/payments',
      source: 'database',
    },
    {
      id: 'external_trigger:platform/release',
      kind: 'external_trigger',
      label: 'release',
      description: 'Enabled external trigger / platform/deploy / runs in platform',
      href: '/external-triggers/platform/release',
      teamPath: 'platform',
      source: 'database',
    },
    {
      id: 'git_webhook_source:gitlab-platform',
      kind: 'git_webhook_source',
      label: 'GitLab Platform',
      description: 'Enabled webhook source / gitlab / 1 allowed repos',
      href: '/git-webhook-sources/gitlab-platform',
      teamPath: '',
      source: 'database',
    },
    {
      id: 'schedule:nightly',
      kind: 'schedule',
      label: 'Nightly deploy',
      description: 'Enabled schedule / cron / pipeline platform/deploy / runs in platform',
      href: '/schedules?pipeline=platform%2Fdeploy',
      teamPath: 'platform',
      source: 'git',
    },
    {
      id: 'knowledge_context:runbook/platform/restart',
      kind: 'knowledge_context',
      label: 'restart',
      description: 'runbook / public',
      href: '/knowledge-context/runbook/platform/restart',
      teamPath: '',
      source: 'git',
    },
    {
      id: 'scope:platform/payments',
      kind: 'scope',
      label: 'payments',
      description: '2 secrets in platform/payments',
      href: '/scopes/platform/payments',
      teamPath: 'platform/payments',
    },
    {
      id: 'credential:credential://team/platform/openai',
      kind: 'credential',
      label: 'openai',
      description: 'api_key in platform',
      href: '/credentials/team/platform/openai',
      teamPath: 'platform',
      source: 'database',
    },
  ],
};

const rootResourceCatalog: TeamResourceCatalogState = {
  teamPath: '',
  loading: false,
  error: null,
  resources: [
    {
      id: 'scope:default',
      kind: 'scope',
      label: 'Default Scope',
      description: '0 global secrets',
      href: '/scopes/default',
      teamPath: '',
    },
  ],
};

function renderWorkspace(overrides: Partial<Parameters<typeof TeamsWorkspace>[0]> = {}) {
  const props = {
    teams,
    activeTeam: teams[0],
    activeTeamPath: [teams[0]],
    searchTerm: '',
    onSearchTermChange: vi.fn(),
    onSelectTeam: vi.fn(),
    onCreate: vi.fn(),
    onEditTeam: vi.fn(),
    onDeleteTeam: vi.fn(),
    onOpenConfig: vi.fn(),
    operationsSummary,
    resourceCatalog,
    currentUser: {
      sub: 'alice',
      email: 'alice@example.com',
      displayName: 'Alice Operator',
      roles: ['nopsai-admin', 'platform-owner'],
    },
    ...overrides,
  };

  render(
    <MemoryRouter>
      <TeamsWorkspace {...props} />
    </MemoryRouter>
  );
  return props;
}

describe('TeamsWorkspace', () => {
  it('switches detail tabs and keeps operational actions wired', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace();

    const createButton = screen.getByRole('button', { name: 'New team' });
    await user.click(createButton);
    expect(props.onCreate).toHaveBeenCalledWith('team');
    expect(screen.getByRole('button', { name: 'Search teams' }).closest('.teams-search')).toHaveClass('ai-resource-search');
    expect(screen.queryByRole('button', { name: 'Refresh teams' })).not.toBeInTheDocument();
    expect(screen.getByRole('separator', { name: 'Resize team tree' })).toBeVisible();
    expect(screen.getByRole('tabpanel', { name: 'Overview' })).toBeVisible();
    const overviewCard = screen.getByRole('heading', { name: 'Team Overview' }).closest('article');
    expect(overviewCard).not.toBeNull();
    expect(within(overviewCard as HTMLElement).getByText('Applications')).toBeVisible();
    expect(within(overviewCard as HTMLElement).getAllByText('2').length).toBeGreaterThan(0);
    expect(within(overviewCard as HTMLElement).queryByText('Repositories')).not.toBeInTheDocument();
    expect(within(overviewCard as HTMLElement).queryByText('Scoped teams')).not.toBeInTheDocument();
    expect(within(overviewCard as HTMLElement).queryByText('Recent run signals')).not.toBeInTheDocument();
    expect(within(overviewCard as HTMLElement).getByText('Owners')).toBeVisible();
    expect(within(overviewCard as HTMLElement).getByText('Alice Admin')).toBeVisible();
    expect(within(overviewCard as HTMLElement).getByText('Default LLM profile')).toBeVisible();
    expect(within(overviewCard as HTMLElement).getByRole('link', { name: 'fast' })).toHaveAttribute('href', '/llm-profiles?team=platform');
    expect(within(overviewCard as HTMLElement).getByText('Default agent profile')).toBeVisible();
    expect(within(overviewCard as HTMLElement).getByRole('link', { name: 'reviewer' })).toHaveAttribute('href', '/agent-profiles?team=platform');
    expect(within(overviewCard as HTMLElement).getByText('Latest run app')).toBeVisible();
    expect(within(overviewCard as HTMLElement).getByText(/checkout-api/)).toBeVisible();
    expect(screen.queryByRole('heading', { name: 'Team Activity' })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Activity range' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Applications' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'AI Profiles' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'GitOps' }));
    expect(screen.getByRole('heading', { name: 'platform GitOps' })).toBeVisible();
    expect(screen.getByText('https://github.com/acme/platform-config')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Configure' }));
    expect(props.onOpenConfig).toHaveBeenCalledWith(teams[0], 'sync');

    await user.click(screen.getByRole('tab', { name: 'Notifications' }));
    expect(screen.getByRole('heading', { name: 'platform Notifications' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Configure' })).toBeVisible();
    expect(screen.getByText(/GitOps target:/)).toBeVisible();

    await user.click(screen.getByRole('tab', { name: 'Access' }));
    expect(screen.getByRole('heading', { name: 'platform Access' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Current User Access' })).toBeVisible();
    expect(screen.getByText('nopsai-admin')).toBeVisible();
    expect(screen.getByText('platform-owner')).toBeVisible();
    expect(screen.getByText('Scoped Basic Roles')).toBeVisible();
    expect(screen.getByText('owner + child scopes')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Effective Checks' })).toBeVisible();
    expect(screen.getByText('1/2')).toBeVisible();
    expect(screen.getByText('Alice Admin')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open Access' })).toHaveAttribute('href', '/system/access?resource_type=team&resource_id=platform');
  });

  it('keeps GitOps controls available for navigation-only team scopes', async () => {
    const user = userEvent.setup();
    const navigationTeam = { ...teams[0], navigation_only: true };
    const navTeams = [navigationTeam, ...teams.slice(1)];
    const props = renderWorkspace({
      teams: navTeams,
      activeTeam: navigationTeam,
      activeTeamPath: [navigationTeam],
    });

    await user.click(screen.getByRole('tab', { name: 'GitOps' }));

    expect(screen.getByRole('heading', { name: 'platform GitOps' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Configure' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Configure' }));
    expect(props.onOpenConfig).toHaveBeenCalledWith(navigationTeam, 'sync');
  });

  it('links global GitOps to the system config instead of opening team configuration', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({
      activeTeam: null,
      activeTeamPath: [],
      operationsSummary: rootOperationsSummary,
    });

    await user.click(screen.getByRole('tab', { name: 'GitOps' }));

    expect(screen.getByRole('heading', { name: 'Global GitOps' })).toBeVisible();
    expect(screen.getByText(/Global uses the system config repository/)).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open Global Config' })).toHaveAttribute('href', '/system/config');
    expect(screen.getByRole('link', { name: 'System Config' })).toHaveAttribute('href', '/system/config');
    expect(screen.queryByRole('tab', { name: 'Notifications' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Configure' })).not.toBeInTheDocument();
    expect(props.onOpenConfig).not.toHaveBeenCalled();
  });

  it('summarizes global AI profiles and platform admins at root', async () => {
    const user = userEvent.setup();
    renderWorkspace({
      activeTeam: null,
      activeTeamPath: [],
      operationsSummary: rootOperationsSummary,
      resourceCatalog: rootResourceCatalog,
    });

    expect(screen.getByText('Organization resources and global automation configuration.')).toBeVisible();
    expect(screen.getByText('Agent Profiles')).toBeVisible();
    expect(screen.queryByText('View scope')).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Applications' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'AI Profiles' })).not.toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Applications resources' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Open LLM Profiles' })).toHaveTextContent('1');
    await user.click(screen.getByRole('button', { name: 'Open LLM Profiles' }));
    expect(screen.getByRole('region', { name: 'LLM Profiles resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open hosted' })).toHaveAttribute('href', '/llm-profiles?team=global');

    await user.click(screen.getByRole('button', { name: 'Open Agent Profiles' }));
    expect(screen.getByRole('region', { name: 'Agent Profiles resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open DevOps Engineer' })).toHaveAttribute('href', '/agent-profiles?team=global');

    await user.click(screen.getByRole('button', { name: 'Open MCP Profiles' }));
    expect(screen.getByRole('region', { name: 'MCP Profiles resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open github-global' })).toHaveAttribute('href', '/mcp/profiles?team=global');

    await user.click(screen.getByRole('tab', { name: 'Access' }));
    expect(screen.getByRole('heading', { name: 'Global Access' })).toBeVisible();
    expect(screen.getByText('Platform Admin')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open Access' })).toHaveAttribute('href', '/system/access?resource_type=platform&resource_id=platform');
  });

  it('summarizes linked resource categories and lists the selected resource tab', async () => {
    const user = userEvent.setup();
    renderWorkspace();

    expect(screen.queryByRole('button', { name: 'Open Repositories' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Linked Resources' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tablist', { name: 'Linked resource type' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open Applications' })).toHaveTextContent('2');
    expect(screen.getByRole('button', { name: 'Open LLM Profiles' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Agent Profiles' })).toHaveTextContent('2');
    expect(screen.getByRole('button', { name: 'Open MCP Profiles' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Notifications' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Pipelines' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Steps' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Triggers' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open External Triggers' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Git Webhook Sources' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Schedules' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Knowledge Context' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Scopes' })).toHaveTextContent('1');
    expect(screen.getByRole('button', { name: 'Open Credentials' })).toHaveTextContent('1');
    const applicationsRegion = screen.getByRole('region', { name: 'Applications resources' });
    expect(within(applicationsRegion).getByRole('link', { name: 'Open checkout-api' })).toHaveAttribute(
      'href',
      '/teams/team/platform/payments/checkout-api'
    );

    await user.click(screen.getByRole('button', { name: 'Open LLM Profiles' }));
    expect(screen.getByRole('region', { name: 'LLM Profiles resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open fast' })).toHaveAttribute('href', '/llm-profiles?team=platform');

    await user.click(screen.getByRole('button', { name: 'Open Agent Profiles' }));
    expect(screen.getByRole('region', { name: 'Agent Profiles resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open DevOps Engineer' })).toHaveAttribute('href', '/agent-profiles?team=global');
    expect(screen.getByRole('link', { name: 'Open Reviewer' })).toHaveAttribute('href', '/agent-profiles?team=platform');

    await user.click(screen.getByRole('button', { name: 'Open MCP Profiles' }));
    expect(screen.getByRole('region', { name: 'MCP Profiles resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open engineering-tools' })).toHaveAttribute('href', '/mcp/profiles?team=platform');

    await user.click(screen.getByRole('button', { name: 'Open Pipelines' }));
    expect(screen.getByRole('region', { name: 'Pipelines resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open deploy-api' })).toHaveAttribute('href', '/pipelines/platform/deploy-api');

    await user.click(screen.getByRole('button', { name: 'Open Steps' }));
    expect(screen.getByRole('region', { name: 'Steps resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open build' })).toHaveAttribute('href', '/steps/platform/build');

    await user.click(screen.getByRole('button', { name: 'Open Triggers' }));
    expect(screen.getByRole('region', { name: 'Triggers resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open checkout-api' })).toHaveAttribute('href', '/triggers/platform/acme/checkout-api');

    await user.click(screen.getByRole('button', { name: 'Open External Triggers' }));
    expect(screen.getByRole('region', { name: 'External Triggers resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open release' })).toHaveAttribute('href', '/external-triggers/platform/release');

    await user.click(screen.getByRole('button', { name: 'Open Git Webhook Sources' }));
    expect(screen.getByRole('region', { name: 'Git Webhook Sources resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open GitLab Platform' })).toHaveAttribute('href', '/git-webhook-sources/gitlab-platform');

    await user.click(screen.getByRole('button', { name: 'Open Schedules' }));
    expect(screen.getByRole('region', { name: 'Schedules resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open Nightly deploy' })).toHaveAttribute('href', '/schedules?pipeline=platform%2Fdeploy');

    await user.click(screen.getByRole('button', { name: 'Open Knowledge Context' }));
    expect(screen.getByRole('region', { name: 'Knowledge Context resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open restart' })).toHaveAttribute('href', '/knowledge-context/runbook/platform/restart');

    await user.click(screen.getByRole('button', { name: 'Open Scopes' }));
    expect(screen.getByRole('region', { name: 'Scopes resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open payments' })).toHaveAttribute('href', '/scopes/platform/payments');

    await user.click(screen.getByRole('button', { name: 'Open Credentials' }));
    expect(screen.getByRole('region', { name: 'Credentials resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open openai' })).toHaveAttribute(
      'href',
      '/credentials/team/platform/openai'
    );
  });

  it('renders search results and routes table actions to the page controller', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({ searchTerm: 'checkout' });

    expect(screen.getByRole('heading', { name: 'Matching Resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'acme/checkout-api' })).toBeVisible();
    expect(screen.queryByRole('link', { name: 'acme/docs' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Open checkout-api' }));
    expect(props.onSelectTeam).toHaveBeenCalledWith(3);

    await user.click(screen.getByRole('button', { name: 'Edit checkout-api' }));
    expect(props.onEditTeam).toHaveBeenCalledWith(teams[2]);

    await user.click(screen.getByRole('button', { name: 'Delete checkout-api' }));
    expect(props.onDeleteTeam).toHaveBeenCalledWith(teams[2]);
  });

  it('collapses team and application branches independently', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({
      activeTeam: null,
      activeTeamPath: [],
      operationsSummary: rootOperationsSummary,
    });

    expect(screen.queryByRole('button', { name: 'Expand payments' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'checkout-api' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Expand platform' }));
    expect(screen.getByRole('button', { name: 'Expand payments' })).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Collapse platform' }));
    expect(screen.queryByRole('button', { name: 'Expand payments' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open platform' })).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Expand platform' }));
    await user.click(screen.getByRole('button', { name: 'Expand payments' }));
    expect(screen.getByRole('button', { name: 'checkout-api' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Collapse payments' }));
    expect(screen.queryByRole('button', { name: 'checkout-api' })).not.toBeInTheDocument();
    expect(props.onSelectTeam).not.toHaveBeenCalled();
  });

  it('shows application overview with only app-related resources', () => {
    renderWorkspace({
      activeTeam: teams[2],
      activeTeamPath: [teams[0], teams[1], teams[2]],
    });

    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveClass('active');
    expect(screen.queryByRole('tab', { name: 'Details' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'checkout-api Overview' })).not.toBeInTheDocument();
    expect(screen.getByText('checkout-api application details: owner team, repository, app path, and latest run context.')).toBeVisible();
    expect(screen.queryByText('Application ownership, repository identity, and run metadata.')).not.toBeInTheDocument();
    expect(screen.getByText('acme/checkout-api')).toBeVisible();
    expect(screen.queryByText('Parent')).not.toBeInTheDocument();
    expect(screen.getByText('Owner team')).toBeVisible();
    expect(screen.getByText('Last run')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Related Runs' })).toHaveAttribute('href', '/pipelineruns/main/team/platform/payments/checkout-api');
    expect(screen.queryByRole('link', { name: 'Recent Runs' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Repository' })).toHaveAttribute('href', 'https://github.com/acme/checkout-api');
    expect(screen.queryByRole('heading', { name: 'Application Activity' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'No child items' })).not.toBeInTheDocument();
    expect(screen.getByText('Application-specific automation and configuration linked by app path or repository identity.')).toBeVisible();
    expect(screen.getByRole('region', { name: 'Triggers resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open checkout-api' })).toHaveAttribute('href', '/triggers/platform/acme/checkout-api');
    expect(screen.queryByRole('button', { name: 'Open Applications' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Open LLM Profiles' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Open Pipelines' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'GitOps' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Notifications' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Access' })).not.toBeInTheDocument();
  });
});
