import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import MCPPanel from './MCPPanel';

const apiMocks = vi.hoisted(() => {
  const registry = {
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
  };
  return {
    deleteMCPProfile: vi.fn(),
    deleteMCPServer: vi.fn(),
    discoverMCPServer: vi.fn(async () => undefined),
    fetchMCPRegistry: vi.fn(async () => registry),
    saveMCPProfile: vi.fn(),
    saveMCPServer: vi.fn(),
    testMCPProfile: vi.fn(async () => 'ok'),
  };
});
const teamMocks = vi.hoisted(() => ({
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
}));

vi.mock('./mcp/api', () => apiMocks);
vi.mock('../../lib/resourceTeams', () => teamMocks);

test('renders MCP servers and profiles in the split detail workspace', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();

  render(
    <MemoryRouter>
      <MCPPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByRole('heading', { name: 'MCP' })).toHaveClass('sr-only');
  expect(document.getElementById('system-mcp-section')).toHaveClass('ai-resource-page');
  expect(screen.getByLabelText('MCP server workspace')).toHaveClass('ai-resource-workspace-card');
  expect(screen.getByLabelText('MCP server tree')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Select MCP server GitHub MCP' })).toBeVisible();
  expect(screen.queryByLabelText('MCP server detail')).not.toBeInTheDocument();
  expect(screen.getAllByText('Servers')[0]).toBeVisible();
  expect(screen.getByText('Discovered tools')).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Scopes' })).toBeVisible();
  expect(screen.queryByRole('columnheader', { name: 'Transport' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Select MCP server GitHub MCP' }));
  expect(screen.getByLabelText('MCP server detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  expect(screen.getByText('https://api.githubcopilot.com/mcp/x/all/readonly')).toBeVisible();
  expect(screen.getByRole('link', { name: 'credential://system/mcp/github' })).toHaveAttribute(
    'href',
    '/credentials?credential=credential%3A%2F%2Fsystem%2Fmcp%2Fgithub'
  );
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /edit server/i })).toHaveClass('ai-resource-icon-action');
  expect(screen.getAllByText('/platform/ml')[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: /edit server/i }));
  expect(screen.getByText('Expected type: bearer_token')).toBeVisible();
  await user.click(screen.getByRole('button', { name: /close server form/i }));

  await user.click(screen.getByRole('button', { name: /discover tools/i }));
  await waitFor(() => expect(apiMocks.discoverMCPServer).toHaveBeenCalledWith('platform/ml/github'));

  await user.click(screen.getByRole('tab', { name: 'Profiles' }));
  expect(screen.getByLabelText('MCP profile workspace')).toHaveClass('ai-resource-workspace-card');
  expect(screen.getByLabelText('MCP profile tree')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Select MCP profile pr-review' })).toBeVisible();
  expect(screen.queryByLabelText('MCP profile detail')).not.toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Profiles' })).toHaveClass('ai-resource-view-switch__item');
  expect(screen.getByText('Approved tools')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Select MCP profile pr-review' }));
  expect(screen.getByLabelText('MCP profile detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(await screen.findByText('Review pull requests.')).toBeVisible();
  expect(screen.getByText('issues_list')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');

  await user.click(screen.getByRole('button', { name: /test profile/i }));
  await waitFor(() => expect(apiMocks.testMCPProfile).toHaveBeenCalledWith('platform/ml/pr-review'));

  await user.click(screen.getByRole('button', { name: /edit profile/i }));
  expect(screen.getByLabelText('Name')).toHaveValue('platform/ml/pr-review');
  expect(screen.getByLabelText('Allowed scopes')).toHaveValue('prod');
});

test('applies the team filter and profiles view from the route query', async () => {
  render(
    <MemoryRouter initialEntries={['/mcp?team=platform%2Fml&view=profiles']}>
      <MCPPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Filter by team')).toHaveValue('platform/ml');
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Profiles' })).toHaveAttribute('aria-selected', 'true'));
  expect(screen.getByRole('button', { name: 'Select MCP profile pr-review' })).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Scopes' })).toBeVisible();
  expect(screen.queryByText('Review pull requests.')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('MCP profile detail')).not.toBeInTheDocument();
  expect(screen.queryByText('https://api.githubcopilot.com/mcp/x/all/readonly')).not.toBeInTheDocument();
});
