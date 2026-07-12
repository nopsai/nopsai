import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from '../lib/api';
import TeamsPage from './Teams';

vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({
    currentUser: {
      id: 'user-1',
      sub: 'user-1',
      email: 'user@example.test',
      roles: [],
      permissions: [],
    },
  }),
}));

vi.mock('../features/teams/hooks/useTeamOperationsSummary', () => ({
  useTeamOperationsSummary: () => ({
    teamPath: '',
    loading: false,
    configRepo: null,
    configRepoError: null,
    notificationRoute: null,
    notificationError: null,
    llmProfiles: null,
    agentProfiles: null,
    mcpProfiles: null,
    aiProfilesError: null,
    accessGrants: [],
    accessGrantsError: null,
    permissions: [],
    permissionsError: null,
  }),
}));

vi.mock('../features/teams/hooks/useTeamResourceCatalog', () => ({
  useTeamResourceCatalog: () => ({
    teamPath: '',
    loading: false,
    error: null,
    resources: [],
  }),
}));

vi.mock('../features/teams/hooks/useTeamConfigRepositoryController', () => ({
  useTeamConfigRepositoryController: () => ({
    configRepoTeam: null,
    configRepo: null,
    configRepoForm: {
      repo_url: '',
      branch: 'main',
      base_path: '',
      enabled: false,
      write_enabled: false,
      write_branch: 'nopsai/ui-changes',
    },
    configRepoLoading: false,
    configRepoSaving: false,
    configRepoSyncing: false,
    configRepoError: null,
    configRepoDriftLoading: false,
    configRepoDriftOpen: false,
    configRepoDrift: null,
    configRepoDriftError: null,
    configRepoPushing: false,
    configRepoPushResult: null,
    configRepoInitialTab: 'sync',
    configRepoManageAllowed: true,
    configRepoSyncAllowed: true,
    notificationRoute: null,
    notificationRouteForm: { routes: [], selectedRouteID: null },
    notificationRouteLoading: false,
    notificationRouteSaving: false,
    notificationRouteError: null,
    setConfigRepoForm: () => undefined,
    setNotificationRouteForm: () => undefined,
    setConfigRepoDriftOpen: () => undefined,
    openTeamConfigRepository: () => undefined,
    closeTeamConfigRepository: () => undefined,
    saveTeamConfigRepository: async () => undefined,
    deleteTeamConfigRepository: async () => undefined,
    syncTeamConfigRepository: async () => undefined,
    checkTeamConfigRepositoryDrift: async () => undefined,
    pushTeamConfigRepositoryDrift: async () => undefined,
    saveTeamNotificationRoute: async () => undefined,
    deleteTeamNotificationRoute: async () => undefined,
  }),
}));

const teams = [
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
    name: 'service-api',
    kind: 'app',
    path: 'platform/service-api',
    team_path: 'platform',
    parent_id: 1,
    repo_url: 'https://github.com/acme/service-api',
    repository_full_name: 'acme/service-api',
  },
];

const teamsPayload = {
  teams: [teams[0]],
  applications: [teams[1]],
};

function renderTeams(initialEntry = '/teams') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <TeamsPage />
    </MemoryRouter>
  );
}

describe('TeamsPage', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders teams and applications from the Teams API', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(Response.json(teamsPayload));

    renderTeams('/teams/team/platform');

    expect((await screen.findAllByText('service-api'))[0]).toBeVisible();
    expect(document.querySelector('[data-page="teams"]')).toHaveClass('active');
    expect(screen.getAllByText('platform').length).toBeGreaterThan(0);
    expect(screen.getByRole('link', { name: 'acme/service-api' })).toHaveAttribute(
      'href',
      'https://github.com/acme/service-api'
    );
  });

  it('shows an actionable empty state when no teams are visible', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(Response.json({ teams: [], applications: [] }));

    renderTeams();

    expect(await screen.findByRole('heading', { name: 'No visible teams' })).toBeVisible();
    expect(screen.getByText(/Teams appear here/)).toBeVisible();
    expect(screen.getByRole('button', { name: 'Create team' })).toBeVisible();
    expect(screen.queryByText('No teams.')).not.toBeInTheDocument();
  });

  it('falls back to root teams when the selected team path is stale', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(Response.json(teamsPayload));

    renderTeams('/teams/team/missing/team');

    await screen.findAllByText('platform');
    expect(screen.getAllByText('platform').length).toBeGreaterThan(0);
    expect(screen.queryByRole('heading', { name: 'No visible teams' })).not.toBeInTheDocument();
  });

  it('shows a selected leaf state instead of looking blank', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(Response.json(teamsPayload));

    renderTeams('/teams/team/platform/service-api');

    expect(await screen.findByRole('heading', { name: 'No child items' })).toBeVisible();
    expect(screen.getByText('service-api has no child teams or applications.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Back to global' })).toBeVisible();
  });

  it('shows a retry state when the Teams API fails', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(new Response('authorization unavailable', { status: 503 }));

    renderTeams();

    expect(await screen.findByRole('heading', { name: 'Teams could not load' })).toBeVisible();
    expect(screen.getByText('authorization unavailable')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible();
  });

  it('creates an application under the selected team', async () => {
    const created: unknown[] = [];
    vi.spyOn(apiClient, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === '/v1/teams/1/applications' && init?.method === 'POST') {
        created.push(JSON.parse(String(init.body)));
        return Response.json({ id: 3 }, { status: 201 });
      }
      if (path === '/v1/teams?include=applications') return Response.json(teamsPayload);
      return Response.json({ allowed: true });
    });

    const user = userEvent.setup();
    renderTeams('/teams/team/platform');

    await screen.findAllByText('service-api');
    await user.click(screen.getAllByRole('button', { name: 'New' })[0]);
    expect(screen.getByRole('dialog', { name: 'Create Team Item' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Application' }));
    await user.type(screen.getByLabelText('Application Name'), 'worker');
    await user.type(screen.getByLabelText('Repository URL'), 'https://github.com/acme/worker');
    await user.click(screen.getByRole('button', { name: 'Create' }));

    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]).toEqual({
      name: 'worker',
      repo_url: 'https://github.com/acme/worker',
    });
  });
});
