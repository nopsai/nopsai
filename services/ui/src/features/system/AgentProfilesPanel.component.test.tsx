import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import AgentProfilesPanel from './AgentProfilesPanel';

const apiMocks = vi.hoisted(() => ({
  createAgentProfile: vi.fn(),
  deleteAgentProfile: vi.fn(async () => ({ status: 'deleted' as const })),
  fetchAgentProfiles: vi.fn(async () => ({
    default_profile: 'devops-engineer',
    profiles: [
      {
        id: 'devops-engineer',
        display_name: 'DevOps Engineer',
        role: 'Senior DevOps Engineer',
        description: 'Operates deployments and CI/CD.',
        instructions: 'Keep releases boring and reversible.',
        enabled: true,
        built_in: true,
        source: 'built-in',
        usage_count: 2,
        references: ['pipelines/deploy.yaml'],
      },
      {
        id: 'platform/ml/security-reviewer',
        display_name: 'Security Reviewer',
        role: 'Senior Security Reviewer',
        description: 'Reviews security posture.',
        instructions: 'Focus on practical risk reduction.',
        enabled: true,
        source: 'ui',
        usage_count: 0,
        references: [],
        last_updated: '2026-07-11T12:00:00Z',
      },
      {
        id: 'release-manager',
        display_name: 'Release Manager',
        role: 'Senior Release Manager',
        description: 'Coordinates releases.',
        instructions: 'Check rollout evidence.',
        enabled: true,
        source: 'ui',
        usage_count: 0,
        references: [],
      },
    ],
  })),
  saveAgentProfile: vi.fn(),
  setDefaultAgentProfile: vi.fn(async () => ({
    default_profile: 'release-manager',
    profiles: [],
  })),
}));
const teamMocks = vi.hoisted(() => ({
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
}));

vi.mock('./agent-profiles/api', () => apiMocks);
vi.mock('../../lib/resourceTeams', () => teamMocks);

test('renders agent profiles as a split detail workspace and keeps actions wired', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();

  render(
    <MemoryRouter>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByRole('heading', { name: 'Agent Profiles' })).toHaveClass('sr-only');
  expect(document.getElementById('system-agent-profiles-section')).toHaveClass('ai-resource-page');
  expect(screen.getByLabelText('Agent profile workspace')).toHaveClass('ai-resource-workspace-card');
  expect(screen.getByLabelText('Agent profile tree')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Select agent profile DevOps Engineer' })).toBeVisible();
  expect(screen.queryByLabelText('Agent profile detail')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Default agent profile').closest('.ai-resource-overview-bar')).toBe(
    screen.getByLabelText('Resource summary').closest('.ai-resource-overview-bar')
  );
  expect(screen.getAllByText('Profiles')[0]).toBeVisible();
  expect(screen.getByText('Pipeline refs')).toBeVisible();
  expect(screen.queryByText('Operates deployments and CI/CD.')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();

  await user.click(within(screen.getByLabelText('Agent profiles')).getByRole('button', { name: /security reviewer/i }));
  expect(screen.getByLabelText('Agent profile detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByText('Reviews security posture.')).toBeVisible();
  expect(screen.getByText('Focus on practical risk reduction.')).toBeVisible();
  expect(screen.getAllByText('/platform/ml')[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: /^duplicate$/i }));
  expect(screen.getByLabelText('ID')).toHaveValue('security-reviewer-custom');
  expect(screen.getByText('platform/ml/security-reviewer-custom')).toBeVisible();
  expect(screen.getByLabelText('Name')).toHaveValue('Security Reviewer Custom');

  const defaultSelect = screen.getByLabelText('Default agent profile');
  expect(defaultSelect).not.toHaveTextContent('Security Reviewer');
  await user.selectOptions(defaultSelect, 'release-manager');
  await waitFor(() => expect(apiMocks.setDefaultAgentProfile).toHaveBeenCalledWith('release-manager'));
});

test('applies the team filter from the route query', async () => {
  render(
    <MemoryRouter initialEntries={['/agent-profiles?team=platform%2Fml']}>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Filter by team')).toHaveValue('platform/ml');
  const profileTable = await screen.findByLabelText('Agent profiles');
  expect(within(profileTable).getByRole('button', { name: /security reviewer/i })).toBeVisible();
  expect(within(profileTable).queryByRole('button', { name: /devops engineer/i })).not.toBeInTheDocument();
});
