import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from '../lib/api';
import TeamsPage from './Teams';

const teams = [
  {
    id: 1,
    name: 'platform',
    kind: 'team',
    description: 'Platform engineering',
    parent_id: null,
  },
  {
    id: 2,
    name: 'service-api',
    kind: 'app',
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

    renderTeams('/teams?team=1');

    expect(await screen.findByText('service-api')).toBeVisible();
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

  it('falls back to root teams when the selected team id is stale', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(Response.json(teamsPayload));

    renderTeams('/teams?team=999');

    await screen.findByText('platform');
    expect(screen.getAllByText('platform').length).toBeGreaterThan(0);
    expect(screen.queryByRole('heading', { name: 'No visible teams' })).not.toBeInTheDocument();
  });

  it('shows a selected leaf state instead of looking blank', async () => {
    vi.spyOn(apiClient, 'fetch').mockResolvedValue(Response.json(teamsPayload));

    renderTeams('/teams?team=2');

    expect(await screen.findByRole('heading', { name: 'No child items' })).toBeVisible();
    expect(screen.getByText('service-api has no child teams or applications.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Back to root' })).toBeVisible();
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
    renderTeams('/teams?team=1');

    await screen.findByText('service-api');
    await user.click(screen.getByRole('button', { name: 'New' }));
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
