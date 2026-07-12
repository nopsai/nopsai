import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { Team } from '../../lib/teamModels';
import { TeamsWorkspace } from './TeamsWorkspace';
import { defaultNotificationRouteDefinition } from './notificationRoutes';
import type { TeamOperationsSummaryState } from './hooks/useTeamOperationsSummary';

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
    repo_url: 'https://github.com/acme/platform-config',
    branch: 'main',
    base_path: 'teams/platform',
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
    profiles: [{
      id: 'reviewer',
      display_name: 'Reviewer',
      role: 'review',
      instructions: 'Review changes',
      enabled: true,
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    }],
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

function renderWorkspace(overrides: Partial<Parameters<typeof TeamsWorkspace>[0]> = {}) {
  const props = {
    teams,
    activeTeam: teams[0],
    activeTeamPath: [teams[0]],
    searchTerm: '',
    teamsLoading: false,
    onSearchTermChange: vi.fn(),
    onSelectTeam: vi.fn(),
    onRefresh: vi.fn(),
    onCreate: vi.fn(),
    onDeleteTeam: vi.fn(),
    onOpenConfig: vi.fn(),
    operationsSummary,
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

    expect(screen.getAllByRole('button', { name: 'New' })).toHaveLength(1);
    expect(screen.getByRole('tabpanel', { name: 'Overview' })).toBeVisible();

    await user.click(screen.getByRole('tab', { name: 'Applications' }));
    expect(screen.getByRole('tab', { name: 'Applications' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('heading', { name: 'Application Scope' })).toBeVisible();
    expect(screen.queryByRole('heading', { name: 'Scoped Applications' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'acme/checkout-api' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'GitOps' }));
    expect(screen.getByRole('heading', { name: 'platform GitOps' })).toBeVisible();
    expect(screen.getByText('https://github.com/acme/platform-config')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Configure' }));
    expect(props.onOpenConfig).toHaveBeenCalledWith(teams[0], 'sync');

    await user.click(screen.getByRole('tab', { name: 'Notifications' }));
    expect(screen.getByRole('heading', { name: 'platform Notifications' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Configure' })).toBeVisible();
    expect(screen.getByText(/GitOps target:/)).toBeVisible();

    await user.click(screen.getByRole('tab', { name: 'AI Profiles' }));
    expect(screen.getByRole('heading', { name: 'Team AI Profiles' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'LLM Profiles' })).toHaveAttribute('href', '/llm-profiles?team=platform');
    expect(screen.getByRole('link', { name: 'Agent Profiles' })).toHaveAttribute('href', '/agent-profiles?team=platform');
    expect(screen.getByRole('link', { name: 'MCP' })).toHaveAttribute('href', '/mcp?team=platform&view=profiles');
    expect(screen.getByText('engineering-tools')).toBeVisible();

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

  it('links root GitOps to the global system config instead of opening team configuration', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({
      activeTeam: null,
      activeTeamPath: [],
      operationsSummary: rootOperationsSummary,
    });

    await user.click(screen.getByRole('tab', { name: 'GitOps' }));

    expect(screen.getByRole('heading', { name: 'Global GitOps' })).toBeVisible();
    expect(screen.getByText(/Root uses the global system config repository/)).toBeVisible();
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
    });

    expect(screen.getByText('Organization resources and global automation configuration.')).toBeVisible();
    expect(screen.getByText('Agent Profiles')).toBeVisible();
    expect(screen.queryByText('View scope')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open LLM Profiles' })).toHaveAttribute('href', '/llm-profiles?team=global');
    expect(screen.getByRole('link', { name: 'Open Agent Profiles' })).toHaveAttribute('href', '/agent-profiles?team=global');
    expect(screen.getByRole('link', { name: 'Open MCP Profiles' })).toHaveAttribute('href', '/mcp?team=global&view=profiles');

    await user.click(screen.getByRole('button', { name: 'Open Applications' }));
    expect(screen.getByRole('tab', { name: 'Applications' })).toHaveAttribute('aria-selected', 'true');

    await user.click(screen.getByRole('tab', { name: 'AI Profiles' }));
    expect(screen.getByRole('heading', { name: 'Global AI Profiles' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'LLM Profiles' })).toHaveAttribute('href', '/llm-profiles?team=global');
    expect(screen.getByRole('link', { name: 'Agent Profiles' })).toHaveAttribute('href', '/agent-profiles?team=global');
    expect(screen.getByRole('link', { name: 'MCP' })).toHaveAttribute('href', '/mcp?team=global&view=profiles');
    expect(screen.getByText('hosted')).toBeVisible();
    expect(screen.getByText('DevOps Engineer')).toBeVisible();
    expect(screen.getByText('github-global')).toBeVisible();

    await user.click(screen.getByRole('tab', { name: 'Access' }));
    expect(screen.getByRole('heading', { name: 'Global Access' })).toBeVisible();
    expect(screen.getByText('Platform Admin')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open Access' })).toHaveAttribute('href', '/system/access?resource_type=platform&resource_id=platform');
  });

  it('renders search results and routes table actions to the page controller', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({ searchTerm: 'checkout' });

    expect(screen.getByRole('heading', { name: 'Matching Resources' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'acme/checkout-api' })).toBeVisible();
    expect(screen.queryByRole('link', { name: 'acme/docs' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Open checkout-api' }));
    expect(props.onSelectTeam).toHaveBeenCalledWith(3);

    await user.click(screen.getByRole('button', { name: 'Delete checkout-api' }));
    expect(props.onDeleteTeam).toHaveBeenCalledWith(teams[2]);
  });

  it('shows the selected leaf state and returns to root without a blank panel', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({
      activeTeam: teams[2],
      activeTeamPath: [teams[0], teams[1], teams[2]],
    });

    expect(screen.getByRole('heading', { name: 'No child items' })).toBeVisible();
    expect(screen.getByText('checkout-api has no child teams or applications.')).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Back to root' }));
    expect(props.onSelectTeam).toHaveBeenCalledWith(null);
  });
});
