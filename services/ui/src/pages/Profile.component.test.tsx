import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMocks = vi.hoisted(() => ({
  changePassword: vi.fn(),
  createPersonalAccessToken: vi.fn(),
  listPersonalAccessTokens: vi.fn(),
  revokePersonalAccessToken: vi.fn(),
  updateEmail: vi.fn(),
}));

const clipboardMocks = vi.hoisted(() => ({
  copyTextToClipboard: vi.fn(),
}));

vi.mock('../lib/api', () => ({
  changePassword: apiMocks.changePassword,
  createPersonalAccessToken: apiMocks.createPersonalAccessToken,
  listPersonalAccessTokens: apiMocks.listPersonalAccessTokens,
  revokePersonalAccessToken: apiMocks.revokePersonalAccessToken,
  updateEmail: apiMocks.updateEmail,
}));

vi.mock('../lib/clipboard', () => ({
  copyTextToClipboard: clipboardMocks.copyTextToClipboard,
}));

import ProfilePage from './Profile';

describe('ProfilePage personal tokens', () => {
  beforeEach(() => {
    apiMocks.listPersonalAccessTokens.mockResolvedValue([]);
    apiMocks.createPersonalAccessToken.mockResolvedValue({
      id: 'token-1',
      name: 'Deployment script',
      token: 'nopat_secret',
      token_suffix: 'cret',
      created_at: '2026-06-19T10:00:00Z',
      expires_at: '2026-09-17T23:59:59Z',
    });
    clipboardMocks.copyTextToClipboard.mockResolvedValue(undefined);
  });

  it('copies the newly-created personal token through the shared clipboard helper', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ProfilePage
          user={{ sub: 'alice', email: 'alice@example.com', roles: ['nopsai-admin'] }}
          onLogout={vi.fn()}
        />
      </MemoryRouter>
    );

    await waitFor(() => expect(apiMocks.listPersonalAccessTokens).toHaveBeenCalledOnce());

    await user.click(screen.getByRole('button', { name: /create token/i }));
    await user.type(screen.getByLabelText('Name'), 'Deployment script');
    await user.click(screen.getByRole('button', { name: /^create$/i }));

    await waitFor(() =>
      expect(apiMocks.createPersonalAccessToken).toHaveBeenCalledWith('Deployment script', {
        expiresInDays: 90,
        expiresAt: undefined,
        neverExpires: false,
      })
    );
    expect(await screen.findByDisplayValue('nopat_secret')).toBeVisible();

    await user.click(screen.getByRole('button', { name: /^copy$/i }));

    await waitFor(() => expect(clipboardMocks.copyTextToClipboard).toHaveBeenCalledWith('nopat_secret'));
    expect(await screen.findByRole('button', { name: /copied/i })).toBeVisible();
  });
});
