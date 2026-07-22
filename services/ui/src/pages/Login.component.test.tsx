import { render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  buildOIDCStartUrl: vi.fn(),
  consumeNextSSOLoginPrompt: vi.fn(),
  discoverAuthProvider: vi.fn(),
  exchangeSessionCode: vi.fn(),
  fetchAuthProviders: vi.fn(),
  loginLocal: vi.fn(),
  persistSession: vi.fn(),
}));

vi.mock('../lib/api', () => ({
  apiClient: { fetch: apiMocks.apiFetch },
  buildOIDCStartUrl: apiMocks.buildOIDCStartUrl,
  consumeNextSSOLoginPrompt: apiMocks.consumeNextSSOLoginPrompt,
  discoverAuthProvider: apiMocks.discoverAuthProvider,
  exchangeSessionCode: apiMocks.exchangeSessionCode,
  fetchAuthProviders: apiMocks.fetchAuthProviders,
  loginLocal: apiMocks.loginLocal,
  persistSession: apiMocks.persistSession,
}));

import LoginPage from './Login';

const preflightResponse = {
  ready: false,
  can_login: false,
  mode: 'fresh',
  checks: [
    {
      id: 'database',
      label: 'Database connection',
      status: 'success',
      message: 'Database is reachable.',
      required: true,
    },
    {
      id: 'admin-password',
      label: 'Default admin password',
      status: 'error',
      message: 'Default administrator still uses the built-in password.',
      required: true,
    },
  ],
};

describe('LoginPage setup readiness', () => {
  beforeEach(() => {
    apiMocks.apiFetch.mockResolvedValue({
      status: 200,
      json: async () => preflightResponse,
    });
    apiMocks.fetchAuthProviders.mockResolvedValue({
      local_enabled: true,
      oidc_enabled: false,
      providers: [],
    });
    apiMocks.consumeNextSSOLoginPrompt.mockReturnValue(false);
  });

  it('uses one shared brand and marks configured required checks with a green tick', async () => {
    render(
      <MemoryRouter>
        <LoginPage onLogin={vi.fn()} />
      </MemoryRouter>
    );

    expect(await screen.findByRole('heading', { name: 'Installation readiness' })).toBeTruthy();
    expect(screen.getAllByRole('img', { name: 'NopsAI' })).toHaveLength(1);

    expect(screen.getByLabelText('Database connection configured').textContent).toContain('✓');
    const failedCheckHeader = screen.getByText('Default admin password').parentElement;
    expect(failedCheckHeader).not.toBeNull();
    expect(within(failedCheckHeader as HTMLElement).getByText('Required')).toBeTruthy();

    await waitFor(() => expect(apiMocks.fetchAuthProviders).toHaveBeenCalledOnce());
  });
});
