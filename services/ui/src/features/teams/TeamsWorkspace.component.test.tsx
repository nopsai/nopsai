import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { Team } from '../../lib/teamModels';
import { TeamsWorkspace } from './TeamsWorkspace';

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
    ...overrides,
  };

  render(<TeamsWorkspace {...props} />);
  return props;
}

describe('TeamsWorkspace', () => {
  it('switches detail tabs and keeps operational actions wired', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace();

    expect(screen.getByRole('tabpanel', { name: 'Overview' })).toBeVisible();

    await user.click(screen.getByRole('tab', { name: 'Applications' }));
    expect(screen.getByRole('tab', { name: 'Applications' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('heading', { name: 'Application Scope' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Scoped Applications' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'acme/checkout-api' })).toHaveAttribute('href', 'https://github.com/acme/checkout-api');

    await user.click(screen.getByRole('tab', { name: 'GitOps' }));
    expect(screen.getByRole('heading', { name: 'platform GitOps' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Configure' }));
    expect(props.onOpenConfig).toHaveBeenCalledWith(teams[0]);

    await user.click(screen.getByRole('tab', { name: 'Access' }));
    expect(screen.getByRole('heading', { name: 'platform Access' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open Access' })).toHaveAttribute('href', '/system/access');
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
