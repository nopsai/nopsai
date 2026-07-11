import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import MCPPanel from './MCPPanel';

const apiMocks = vi.hoisted(() => {
  const registry = {
    servers: [
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
        last_test_status: 'connected',
        last_test_message: 'ready',
        last_tested_at: '2026-07-11T12:00:00Z',
        last_discovered_at: '2026-07-11T12:01:00Z',
        discovered_server_name: 'github',
        discovered_version: '1.0.0',
        discovered_protocol: '2025-06-18',
        tools: [
          {
            server_name: 'github',
            name: 'issues_list',
            description: 'List issues',
          },
        ],
      },
    ],
    profiles: [
      {
        name: 'pr-review',
        description: 'Review pull requests.',
        enabled: true,
        servers: [{ server: 'github', tools: ['issues_list'] }],
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

vi.mock('./mcp/api', () => apiMocks);

test('renders MCP servers and profiles in the split detail workspace', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();

  render(
    <MemoryRouter>
      <MCPPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByRole('heading', { name: 'MCP' })).toBeVisible();
  expect(screen.getByText('https://api.githubcopilot.com/mcp/x/all/readonly')).toBeVisible();
  expect(screen.getByRole('link', { name: 'credential://system/mcp/github' })).toHaveAttribute(
    'href',
    '/system/credentials?credential=credential%3A%2F%2Fsystem%2Fmcp%2Fgithub'
  );
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /edit server/i })).toHaveClass('ai-resource-icon-action');

  await user.click(screen.getByRole('button', { name: /discover tools/i }));
  await waitFor(() => expect(apiMocks.discoverMCPServer).toHaveBeenCalledWith('github'));

  await user.click(screen.getByRole('tab', { name: 'Profiles' }));
  expect(await screen.findByText('Review pull requests.')).toBeVisible();
  expect(screen.getByText('issues_list')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');

  await user.click(screen.getByRole('button', { name: /test profile/i }));
  await waitFor(() => expect(apiMocks.testMCPProfile).toHaveBeenCalledWith('pr-review'));

  await user.click(screen.getByRole('button', { name: /edit profile/i }));
  expect(screen.getByLabelText('Name')).toHaveValue('pr-review');
  expect(screen.getByLabelText('Allowed scopes')).toHaveValue('prod');
});
