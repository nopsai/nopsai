import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import AgentProfilesPanel from './AgentProfilesPanel';

const apiMocks = vi.hoisted(() => {
  const profiles = [
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
  ];
  return {
    createAgentProfile: vi.fn(),
    deleteAgentProfile: vi.fn(async () => ({ status: 'deleted' as const })),
    fetchAgentProfiles: vi.fn(async () => ({
      default_profile: 'devops-engineer',
      profiles,
    })),
    saveAgentProfile: vi.fn(),
    setDefaultAgentProfile: vi.fn(async () => ({
      default_profile: 'release-manager',
      profiles,
    })),
  };
});
const teamMocks = vi.hoisted(() => ({
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
}));
const teamProfileMocks = vi.hoisted(() => {
  const teamProfiles = (teamPath = 'platform/ml', defaultProfile = 'security-reviewer') => ({
    team_id: 7,
    team_path: teamPath,
    default_profile: defaultProfile,
    profiles: [
      {
        id: 'security-reviewer',
        display_name: 'Security Reviewer',
        role: 'Senior Security Reviewer',
        description: 'Reviews security posture.',
        instructions: 'Focus on practical risk reduction.',
        enabled: true,
        source: 'team',
      },
      {
        id: 'release-manager',
        display_name: 'Release Manager',
        role: 'Senior Release Manager',
        description: 'Coordinates releases.',
        instructions: 'Check rollout evidence.',
        enabled: true,
        source: 'team',
      },
    ],
  });
  return {
    deleteTeamAgentProfile: vi.fn(),
    fetchTeamAgentProfiles: vi.fn(async (teamPath: string) => teamProfiles(teamPath)),
    requestTeamsJson: vi.fn(async () => ({ allowed: true })),
    setTeamDefaultAgentProfile: vi.fn(async (teamPath: string, defaultProfile: string) => teamProfiles(teamPath, defaultProfile)),
    upsertTeamAgentProfile: vi.fn(async (teamPath: string, profileID: string, payload: Record<string, unknown>) => ({
      team_id: 7,
      team_path: teamPath,
      default_profile: profileID,
      profiles: [{ ...payload, id: profileID, source: 'team' }],
    })),
  };
});

vi.mock('./agent-profiles/api', () => apiMocks);
vi.mock('./teamProfileApi', () => teamProfileMocks);
vi.mock('../../lib/resourceTeams', () => teamMocks);

beforeEach(() => {
  vi.clearAllMocks();
});

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
  expect(screen.queryByLabelText('Resource summary')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Search agent profiles' }).closest('.ai-resource-table-controls')).toBe(
    screen.getByRole('button', { name: /new profile/i }).closest('.ai-resource-table-controls')
  );
  expect(screen.queryByText(/^Profiles$/)).not.toBeInTheDocument();
  expect(screen.queryByText('Pipeline refs')).not.toBeInTheDocument();
  expect(screen.queryByText('Operates deployments and CI/CD.')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();

  const globalDefaultSelect = screen.getByLabelText('Default agent profile');
  await user.selectOptions(globalDefaultSelect, 'release-manager');
  await waitFor(() => expect(apiMocks.setDefaultAgentProfile).toHaveBeenCalledWith('release-manager'));

  await user.click(within(screen.getByLabelText('Agent profiles')).getByRole('button', { name: /security reviewer/i }));
  expect(screen.getByLabelText('Agent profile detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByText('Reviews security posture.')).toBeVisible();
  expect(screen.getByText('Focus on practical risk reduction.')).toBeVisible();
  expect(screen.getAllByText('/platform/ml')[0]).toBeVisible();
  const detailPanel = screen.getByLabelText('Agent profile detail');
  expect(within(detailPanel).getByRole('button', { name: 'Delete profile' }).closest('.ai-resource-detail__actions')).toBeTruthy();
  expect(detailPanel.querySelector('.ai-resource-detail__footer button')).toBeNull();

  await user.click(screen.getByRole('button', { name: /^duplicate$/i }));
  expect(screen.getByLabelText('ID')).toHaveValue('security-reviewer-custom');
  expect(screen.getByText('platform/ml/security-reviewer-custom')).toBeVisible();
  expect(screen.getByLabelText('Name')).toHaveValue('Security Reviewer Custom');
});

test('shows scoped catalog agent profiles for the selected team and saves them as team defaults', async () => {
  apiMocks.fetchAgentProfiles.mockResolvedValueOnce({
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
        id: 'platform/ml/catalog-reviewer',
        display_name: 'Catalog Reviewer',
        role: 'Senior Security Reviewer',
        description: 'Reviews catalog-scoped changes.',
        instructions: 'Focus on scoped team risk.',
        enabled: true,
        source: 'ui',
        usage_count: 0,
        references: [],
      },
    ],
  });
  teamProfileMocks.fetchTeamAgentProfiles.mockResolvedValueOnce({
    team_id: 7,
    team_path: 'platform/ml',
    default_profile: '',
    profiles: [],
  });

  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/agent-profiles?team=platform%2Fml']}>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  const profileTable = await screen.findByLabelText('Agent profiles');
  expect(within(profileTable).getByRole('button', { name: /catalog reviewer/i })).toBeVisible();
  expect(within(profileTable).queryByRole('button', { name: /devops engineer/i })).not.toBeInTheDocument();

  const defaultSelect = screen.getByLabelText('Default agent profile');
  await waitFor(() => expect(defaultSelect).toHaveValue(''));
  await user.selectOptions(defaultSelect, 'platform/ml/catalog-reviewer');

  await waitFor(() => expect(teamProfileMocks.setTeamDefaultAgentProfile).toHaveBeenCalledWith('platform/ml', 'platform/ml/catalog-reviewer'));
  expect(apiMocks.setDefaultAgentProfile).not.toHaveBeenCalledWith('platform/ml/catalog-reviewer');
});

test('updates the selected team agent default through the team API', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/agent-profiles?team=platform%2Fml']}>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  const defaultSelect = await screen.findByLabelText('Default agent profile');
  await waitFor(() => expect(defaultSelect).toHaveValue('platform/ml/security-reviewer'));
  await user.selectOptions(defaultSelect, 'platform/ml/release-manager');

  await waitFor(() => expect(teamProfileMocks.setTeamDefaultAgentProfile).toHaveBeenCalledWith('platform/ml', 'release-manager'));
  expect(apiMocks.setDefaultAgentProfile).not.toHaveBeenCalledWith('platform/ml/release-manager');
});

test('moves an edited team agent profile to the global catalog', async () => {
  apiMocks.saveAgentProfile.mockResolvedValueOnce({
    default_profile: 'devops-engineer',
    profiles: [
      {
        id: 'security-reviewer',
        display_name: 'Security Reviewer',
        role: 'Senior Security Reviewer',
        description: 'Reviews security posture.',
        instructions: 'Focus on practical risk reduction.',
        enabled: true,
        source: 'ui',
        usage_count: 0,
        references: [],
      },
    ],
  });
  const user = userEvent.setup();

  render(
    <MemoryRouter initialEntries={['/agent-profiles?team=platform%2Fml']}>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  const profileTable = await screen.findByLabelText('Agent profiles');
  await user.click(within(profileTable).getByRole('button', { name: /security reviewer/i }));
  await user.click(screen.getByRole('button', { name: /edit profile/i }));

  expect(screen.getByLabelText('Team placement')).toHaveValue('platform/ml');
  expect(screen.getByLabelText('ID')).toHaveValue('security-reviewer');
  await user.selectOptions(screen.getByLabelText('Team placement'), '');
  expect(screen.getByLabelText('ID')).toHaveValue('security-reviewer');
  await user.click(screen.getByRole('button', { name: 'Save profile' }));

  await waitFor(() => expect(apiMocks.saveAgentProfile).toHaveBeenCalledWith(expect.objectContaining({ id: 'security-reviewer' })));
  expect(teamProfileMocks.deleteTeamAgentProfile).toHaveBeenCalledWith('platform/ml', 'security-reviewer');
});

test('applies the team filter from the route query', async () => {
  render(
    <MemoryRouter initialEntries={['/agent-profiles?team=platform%2Fml']}>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByRole('button', { name: 'Open team platform/ml' })).toHaveClass('active');
  await waitFor(() => expect(teamProfileMocks.fetchTeamAgentProfiles).toHaveBeenCalledWith('platform/ml'));
  const profileTable = await screen.findByLabelText('Agent profiles');
  expect(within(profileTable).getByRole('button', { name: /security reviewer/i })).toBeVisible();
  expect(within(profileTable).queryByRole('button', { name: /devops engineer/i })).not.toBeInTheDocument();
});

test('counts cached team-owned agent profiles in the tree', async () => {
  apiMocks.fetchAgentProfiles.mockResolvedValueOnce({
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
    ],
  });

  render(
    <MemoryRouter>
      <AgentProfilesPanel canManage />
    </MemoryRouter>
  );

  const teamButton = await screen.findByRole('button', { name: 'Open team platform/ml' });
  await waitFor(() => expect(teamProfileMocks.fetchTeamAgentProfiles).toHaveBeenCalledWith('platform/ml'));
  await waitFor(() => expect(within(teamButton).getByText('2')).toBeVisible());
});
