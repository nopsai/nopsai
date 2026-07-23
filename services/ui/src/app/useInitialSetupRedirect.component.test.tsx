import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import { fetchSetupStatus } from '../features/system/setup/api';
import type { SetupStatus } from '../features/system/setup/model';
import { isInitialSetupAllowedRoute, useInitialSetupRedirect } from './useInitialSetupRedirect';

vi.mock('../features/system/setup/api', () => ({
  fetchSetupStatus: vi.fn(),
}));

const fetchSetupStatusMock = vi.mocked(fetchSetupStatus);

function setupStatus(completed: boolean): SetupStatus {
  return {
    completed,
    counts: {
      users: 1,
      pipelines: 0,
      steps: 0,
      triggers: 0,
      teams: 0,
      access_grants: 0,
      llm_profiles: 0,
      mcp_servers: 0,
      mcp_profiles: 0,
      knowledge_contexts: 0,
      config_repositories: 0,
    },
    checks: [],
    github: {},
  };
}

function SetupGateProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  const gate = useInitialSetupRedirect({
    accessToken: 'access-token',
    authSubject: 'admin',
    canViewSystemSetup: true,
    currentSubject: 'admin',
    currentUserLoading: false,
    isAuthenticated: true,
    isInitialAdminUser: true,
    mustChangePassword: false,
    pathname: location.pathname,
    navigate,
  });

  return (
    <div>
      <div data-testid="path">{location.pathname}</div>
      <div data-testid="setup-required">{String(gate.required)}</div>
      <button type="button" onClick={() => navigate('/teams')}>Open teams</button>
    </div>
  );
}

function renderGate(initialPath: string) {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <SetupGateProbe />
    </MemoryRouter>
  );
}

beforeEach(() => {
  fetchSetupStatusMock.mockReset();
});

test('keeps redirecting edited URLs to setup while first-install setup is incomplete', async () => {
  const user = userEvent.setup();
  fetchSetupStatusMock.mockResolvedValue(setupStatus(false));

  renderGate('/pipelineruns/main');

  await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/system/setup'));
  expect(screen.getByTestId('setup-required')).toHaveTextContent('true');

  await user.click(screen.getByRole('button', { name: /open teams/i }));

  await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/system/setup'));
  expect(fetchSetupStatusMock).toHaveBeenCalledTimes(1);
});

test('leaves requested routes alone after setup is complete', async () => {
  fetchSetupStatusMock.mockResolvedValue(setupStatus(true));

  renderGate('/teams');

  await waitFor(() => expect(fetchSetupStatusMock).toHaveBeenCalledTimes(1));
  expect(screen.getByTestId('path')).toHaveTextContent('/teams');
  expect(screen.getByTestId('setup-required')).toHaveTextContent('false');
});

test('allows profile only for the forced password-change step before setup', () => {
  expect(isInitialSetupAllowedRoute('/system/setup', false)).toBe(true);
  expect(isInitialSetupAllowedRoute('/profile', true)).toBe(true);
  expect(isInitialSetupAllowedRoute('/profile', false)).toBe(false);
  expect(isInitialSetupAllowedRoute('/teams', false)).toBe(false);
});
