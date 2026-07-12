import { render, screen, waitFor } from '@testing-library/react';
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
    ],
  })),
  saveAgentProfile: vi.fn(),
  setDefaultAgentProfile: vi.fn(async () => ({
    default_profile: 'security-reviewer',
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

  expect(await screen.findByRole('heading', { name: 'Agent Profiles' })).toBeVisible();
  expect(screen.getByText('Operates deployments and CI/CD.')).toBeVisible();
  expect(screen.getByText('Keep releases boring and reversible.')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /view usage/i })).toHaveClass('ai-resource-icon-action');

  await user.click(screen.getByRole('button', { name: /security reviewer/i }));
  expect(screen.getByText('Reviews security posture.')).toBeVisible();
  expect(screen.getByText('Focus on practical risk reduction.')).toBeVisible();
  expect(screen.getAllByText('/platform/ml')[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: /^duplicate$/i }));
  expect(screen.getByLabelText('ID')).toHaveValue('security-reviewer-custom');
  expect(screen.getByText('platform/ml/security-reviewer-custom')).toBeVisible();
  expect(screen.getByLabelText('Name')).toHaveValue('Security Reviewer Custom');

  await user.selectOptions(screen.getByLabelText('Default agent profile'), 'platform/ml/security-reviewer');
  await waitFor(() => expect(apiMocks.setDefaultAgentProfile).toHaveBeenCalledWith('platform/ml/security-reviewer'));
});

test('applies the team filter from the route query', async () => {
  render(
    <MemoryRouter initialEntries={['/agent-profiles?team=platform%2Fml']}>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Filter by team')).toHaveValue('platform/ml');
  expect(screen.getByRole('button', { name: /security reviewer/i })).toBeVisible();
  expect(screen.queryByRole('button', { name: /devops engineer/i })).not.toBeInTheDocument();
});
