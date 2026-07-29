import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import MCPPanel from './MCPPanel';

const apiMocks = vi.hoisted(() => {
  const createRegistry = () => ({
    servers: [
      {
        name: 'platform/ml/github',
        display_name: 'GitHub MCP',
        enabled: true,
        provider: 'github',
        transport: 'streamable_http',
        url: 'https://api.githubcopilot.com/mcp/x/all/readonly',
        auth_type: 'bearer_token',
        credential_ref: 'credential://system/mcp/github',
        headers: { 'X-MCP-Readonly': 'true' },
        timeout: '30s',
        allowed_scopes: ['prod'],
        last_test_status: 'connected',
        last_test_message: 'ready',
        last_tested_at: '2026-07-11T12:00:00Z',
        last_discovered_at: '2026-07-11T12:01:00Z',
        discovered_server_name: 'github',
        discovered_version: '1.0.0',
        discovered_protocol: '2025-06-18',
        tools: [
          {
            server_name: 'platform/ml/github',
            name: 'issues_list',
            description: 'List issues',
          },
        ],
      },
    ],
    profiles: [
      {
        name: 'platform/ml/pr-review',
        description: 'Review pull requests.',
        enabled: true,
        servers: [{ server: 'platform/ml/github', tools: ['issues_list'] }],
        allowed_scopes: ['prod'],
      },
    ],
  });
  return {
    createRegistry,
    deleteMCPProfile: vi.fn(),
    deleteMCPServer: vi.fn(),
    discoverMCPServer: vi.fn(async () => undefined),
    fetchMCPRegistry: vi.fn(async () => createRegistry()),
    saveMCPProfile: vi.fn(),
    saveMCPServer: vi.fn(),
    testMCPProfile: vi.fn(async () => 'ok'),
  };
});
const teamMocks = vi.hoisted(() => ({
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
}));
const teamProfileMocks = vi.hoisted(() => ({
  deleteTeamMCPProfile: vi.fn(),
  fetchTeamMCPProfiles: vi.fn(async (teamPath: string) => ({
    team_id: 7,
    team_path: teamPath,
    profiles: [],
  })),
  requestTeamsJson: vi.fn(async () => ({ allowed: true })),
  upsertTeamMCPProfile: vi.fn(async (teamPath: string, profileName: string, payload: Record<string, unknown>) => ({
    team_id: 7,
    team_path: teamPath,
    profiles: [{ ...payload, name: profileName }],
  })),
}));

vi.mock('./mcp/api', () => apiMocks);
vi.mock('./teamProfileApi', () => teamProfileMocks);
vi.mock('../../lib/resourceTeams', () => teamMocks);

beforeEach(() => {
  vi.clearAllMocks();
  apiMocks.deleteMCPProfile.mockReset();
  apiMocks.deleteMCPProfile.mockResolvedValue(undefined);
  apiMocks.deleteMCPServer.mockReset();
  apiMocks.deleteMCPServer.mockResolvedValue(undefined);
  apiMocks.discoverMCPServer.mockReset();
  apiMocks.discoverMCPServer.mockResolvedValue(undefined);
  apiMocks.fetchMCPRegistry.mockReset();
  apiMocks.fetchMCPRegistry.mockImplementation(async () => apiMocks.createRegistry());
  apiMocks.saveMCPProfile.mockReset();
  apiMocks.saveMCPServer.mockReset();
  apiMocks.testMCPProfile.mockReset();
  apiMocks.testMCPProfile.mockResolvedValue('ok');
  teamMocks.fetchResourceTeamPaths.mockReset();
  teamMocks.fetchResourceTeamPaths.mockResolvedValue(['platform/ml']);
  teamProfileMocks.deleteTeamMCPProfile.mockReset();
  teamProfileMocks.deleteTeamMCPProfile.mockResolvedValue(undefined);
  teamProfileMocks.fetchTeamMCPProfiles.mockReset();
  teamProfileMocks.fetchTeamMCPProfiles.mockImplementation(async (teamPath: string) => ({
    team_id: 7,
    team_path: teamPath,
    profiles: [],
  }));
  teamProfileMocks.requestTeamsJson.mockReset();
  teamProfileMocks.requestTeamsJson.mockResolvedValue({ allowed: true });
  teamProfileMocks.upsertTeamMCPProfile.mockReset();
  teamProfileMocks.upsertTeamMCPProfile.mockImplementation(async (teamPath: string, profileName: string, payload: Record<string, unknown>) => ({
    team_id: 7,
    team_path: teamPath,
    profiles: [{ ...payload, name: profileName }],
  }));
});

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname}{location.search}</span>;
}

test('renders MCP servers and profiles in the split detail workspace', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();

  render(
    <MemoryRouter>
      <MCPPanel canManage />
      <LocationProbe />
    </MemoryRouter>
  );

  expect(await screen.findByRole('heading', { name: 'MCP' })).toHaveClass('sr-only');
  expect(document.getElementById('system-mcp-section')).toHaveClass('ai-resource-page');
  expect(screen.getByLabelText('MCP server workspace')).toHaveClass('ai-resource-workspace-card');
  expect(screen.getByLabelText('MCP server tree')).toBeVisible();
  expect(await screen.findByRole('button', { name: 'Select MCP server GitHub MCP' })).toBeVisible();
  expect(screen.queryByLabelText('MCP server detail')).not.toBeInTheDocument();
  expect(screen.getAllByText('Servers')[0]).toBeVisible();
  expect(screen.getByText('Discovered tools')).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Scopes' })).toBeVisible();
  expect(screen.queryByRole('columnheader', { name: 'Transport' })).not.toBeInTheDocument();

  await user.click(await screen.findByRole('button', { name: 'Select MCP server GitHub MCP' }));
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/mcp/servers/platform/ml/github'));
  expect(screen.getByLabelText('MCP server detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  expect(screen.getByText('https://api.githubcopilot.com/mcp/x/all/readonly')).toBeVisible();
  expect(screen.getByRole('link', { name: 'credential://system/mcp/github' })).toHaveAttribute(
    'href',
    '/credentials/system/mcp/github'
  );
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /edit server/i })).toHaveClass('ai-resource-icon-action');
  expect(screen.getAllByText('/platform/ml')[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: /edit server/i }));
  expect(screen.getByLabelText('Team placement')).toHaveValue('platform/ml');
  expect(screen.getByLabelText('Name')).toHaveValue('github');
  expect(screen.getByText('Expected type: bearer_token')).toBeVisible();
  await user.click(screen.getByRole('button', { name: /close server form/i }));

  await user.click(screen.getByRole('button', { name: /discover tools/i }));
  await waitFor(() => expect(apiMocks.discoverMCPServer).toHaveBeenCalledWith('platform/ml/github'));

  await user.click(screen.getByRole('tab', { name: 'Profiles' }));
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/mcp/profiles'));
  expect(screen.getByLabelText('MCP profile workspace')).toHaveClass('ai-resource-workspace-card');
  expect(screen.getByLabelText('MCP profile tree')).toBeVisible();
  const profileTable = await screen.findByLabelText('MCP profiles');
  expect(await within(profileTable).findByRole('button', { name: 'Select MCP profile pr-review' })).toBeVisible();
  expect(screen.queryByLabelText('MCP profile detail')).not.toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Profiles' })).toHaveClass('ai-resource-view-switch__item');
  expect(screen.getByText('Approved tools')).toBeVisible();

  await user.click(await within(profileTable).findByRole('button', { name: 'Select MCP profile pr-review' }));
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/mcp/profiles/platform/ml/pr-review'));
  expect(screen.getByLabelText('MCP profile detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(await screen.findByText('Review pull requests.')).toBeVisible();
  expect(screen.getByText('issues_list')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');

  await user.click(screen.getByRole('button', { name: /test profile/i }));
  await waitFor(() => expect(apiMocks.testMCPProfile).toHaveBeenCalledWith('platform/ml/pr-review'));

  await user.click(screen.getByRole('button', { name: /edit profile/i }));
  expect(screen.getByLabelText('Team placement')).toHaveValue('platform/ml');
  expect(screen.getByLabelText('Name')).toHaveValue('pr-review');
  expect(screen.getByLabelText('Allowed scopes')).toHaveValue('prod');
});

test('moves an edited MCP server to the global catalog when no profiles reference it', async () => {
  apiMocks.fetchMCPRegistry.mockResolvedValueOnce({
    servers: [
      {
        name: 'platform/ml/github',
        display_name: 'GitHub MCP',
        enabled: true,
        provider: 'github',
        transport: 'streamable_http',
        url: 'https://api.githubcopilot.com/mcp/x/all/readonly',
        auth_type: 'bearer_token',
        credential_ref: 'credential://system/mcp/github',
        headers: { 'X-MCP-Readonly': 'true' },
        timeout: '30s',
        allowed_scopes: ['prod'],
        tools: [],
      },
    ],
    profiles: [],
  });
  apiMocks.saveMCPServer.mockResolvedValueOnce([
    {
      name: 'github',
      display_name: 'GitHub MCP',
      enabled: true,
      provider: 'github',
      transport: 'streamable_http',
      url: 'https://api.githubcopilot.com/mcp/x/all/readonly',
      auth_type: 'bearer_token',
      credential_ref: 'credential://system/mcp/github',
      headers: { 'X-MCP-Readonly': 'true' },
      timeout: '30s',
      allowed_scopes: ['prod'],
      tools: [],
    },
  ]);
  const user = userEvent.setup();

  render(
    <MemoryRouter initialEntries={['/mcp/servers?team=platform%2Fml']}>
      <MCPPanel canManage />
    </MemoryRouter>
  );

  const serverTable = await screen.findByLabelText('MCP servers');
  await user.click(await within(serverTable).findByRole('button', { name: 'Select MCP server GitHub MCP' }));
  await user.click(screen.getByRole('button', { name: /edit server/i }));

  expect(screen.getByLabelText('Team placement')).toHaveValue('platform/ml');
  expect(screen.getByLabelText('Name')).toHaveValue('github');
  await user.selectOptions(screen.getByLabelText('Team placement'), '');
  expect(screen.getByText('github')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Save server' }));

  await waitFor(() => expect(apiMocks.saveMCPServer).toHaveBeenCalledWith(
    expect.objectContaining({ name: 'github' }),
    'platform/ml/github'
  ));
  expect(apiMocks.deleteMCPServer).toHaveBeenCalledWith('platform/ml/github');
});

test('moves an edited team MCP profile to the global catalog', async () => {
  teamProfileMocks.fetchTeamMCPProfiles.mockResolvedValueOnce({
    team_id: 7,
    team_path: 'platform/ml',
    profiles: [
      {
        name: 'pr-review',
        description: 'Team-owned PR review tools.',
        enabled: true,
        servers: [{ server: 'platform/ml/github', tools: ['issues_list'] }],
        allowed_scopes: ['prod'],
      },
    ],
  });
  apiMocks.saveMCPProfile.mockResolvedValueOnce([
    {
      name: 'pr-review',
      description: 'Team-owned PR review tools.',
      enabled: true,
      servers: [{ server: 'platform/ml/github', tools: ['issues_list'] }],
      allowed_scopes: ['prod'],
    },
  ]);
  const user = userEvent.setup();

  render(
    <MemoryRouter initialEntries={['/mcp/profiles?team=platform%2Fml']}>
      <MCPPanel canManage />
    </MemoryRouter>
  );

  const profileTable = await screen.findByLabelText('MCP profiles');
  await user.click(await within(profileTable).findByRole('button', { name: 'Select MCP profile pr-review' }));
  await user.click(screen.getByRole('button', { name: /edit profile/i }));

  expect(screen.getByLabelText('Team placement')).toHaveValue('platform/ml');
  expect(screen.getByLabelText('Name')).toHaveValue('pr-review');
  await user.selectOptions(screen.getByLabelText('Team placement'), '');
  expect(screen.getByLabelText('Name')).toHaveValue('pr-review');
  await user.click(screen.getByRole('button', { name: 'Save profile' }));

  await waitFor(() => expect(apiMocks.saveMCPProfile).toHaveBeenCalledWith(
    expect.objectContaining({ name: 'pr-review' }),
    'platform/ml/pr-review'
  ));
  expect(teamProfileMocks.deleteTeamMCPProfile).toHaveBeenCalledWith('platform/ml', 'pr-review');
});

test('applies the team filter and profiles view from the route query', async () => {
  render(
    <MemoryRouter initialEntries={['/mcp/profiles?team=platform%2Fml']}>
      <MCPPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Filter by team')).toHaveValue('platform/ml');
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Profiles' })).toHaveAttribute('aria-selected', 'true'));
  const profileTable = await screen.findByLabelText('MCP profiles');
  expect(await within(profileTable).findByRole('button', { name: 'Select MCP profile pr-review' })).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Scopes' })).toBeVisible();
  expect(screen.queryByText('Review pull requests.')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('MCP profile detail')).not.toBeInTheDocument();
  expect(screen.queryByText('https://api.githubcopilot.com/mcp/x/all/readonly')).not.toBeInTheDocument();
});
