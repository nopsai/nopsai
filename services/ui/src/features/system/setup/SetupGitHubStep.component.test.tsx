import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import SetupGitHubStep from './SetupGitHubStep';

const tunnelWebhook = 'https://live-gecko-national.ngrok-free.app/webhook';

const apiMocks = vi.hoisted(() => ({
  deleteGitHubAppInstallation: vi.fn(),
  fetchGitHubApp: vi.fn(),
  fetchGitHubAppInstallationRepositories: vi.fn(),
  refreshGitHubAppInstallation: vi.fn(),
  saveGitHubApp: vi.fn(),
  saveGitHubAppInstallation: vi.fn(),
  startGitHubAppInstall: vi.fn(),
  startGitHubAppRegistration: vi.fn(),
  verifyGitHubAppInstallation: vi.fn(),
}));

const manifestMocks = vi.hoisted(() => ({
  clearGitHubAppCallbackParams: vi.fn(),
  submitGitHubAppManifest: vi.fn(),
}));

vi.mock('../git-apps/api', () => apiMocks);
vi.mock('../git-apps/manifestForm', () => manifestMocks);

const unregistered = {
  provider: 'github' as const,
  app_id: '',
  app_slug: '',
  private_key_credential_ref: '',
  webhook_credential_ref: '',
  webhook_url: tunnelWebhook,
  webhook_endpoint: tunnelWebhook,
  installations: [],
};

beforeEach(() => {
  Object.values(apiMocks).forEach(mock => mock.mockReset());
  Object.values(manifestMocks).forEach(mock => mock.mockReset());
  apiMocks.fetchGitHubApp.mockResolvedValue(unregistered);
  apiMocks.startGitHubAppRegistration.mockResolvedValue({
    state: 'state-value',
    post_url: 'https://github.com/settings/apps/new?state=state-value',
    manifest: '{"name":"NopsAI"}',
    app_name: 'NopsAI',
    webhook_endpoint: tunnelWebhook,
    expires_at: '2026-06-15T10:00:00Z',
  });
  apiMocks.startGitHubAppInstall.mockResolvedValue({
    state: 'install-state',
    install_url: 'https://github.com/apps/nopsai-example/installations/new?state=install-state',
    expires_at: '2026-06-15T10:00:00Z',
  });
});

// Operators do not have App IDs, private keys, or webhook secrets before the App
// exists; GitHub issues all three, so the step must not ask for any of them.
test('asks for no GitHub App credentials when the webhook address is already known', async () => {
  render(<SetupGitHubStep canManage />);

  await waitFor(() => expect(apiMocks.fetchGitHubApp).toHaveBeenCalled());
  expect(screen.queryByLabelText('App ID')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Private key credential ref')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Webhook credential ref')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Webhook URL')).not.toBeInTheDocument();
});

test('registers the App on a personal account from one button', async () => {
  const user = userEvent.setup();
  render(<SetupGitHubStep canManage />);

  const install = await screen.findByRole('button', { name: 'Install GitHub App on GitHub' });
  await waitFor(() => expect(install).toBeEnabled());
  await user.click(install);

  await waitFor(() => expect(apiMocks.startGitHubAppRegistration).toHaveBeenCalledTimes(1));
  expect(apiMocks.startGitHubAppRegistration.mock.calls[0][0]).toMatchObject({
    target: 'personal',
    organization: '',
  });
  expect(apiMocks.startGitHubAppRegistration.mock.calls[0][1]).toBe(tunnelWebhook);
  await waitFor(() => expect(manifestMocks.submitGitHubAppManifest).toHaveBeenCalledTimes(1));
});

test('registers the App on an organization when one is named', async () => {
  const user = userEvent.setup();
  render(<SetupGitHubStep canManage />);

  await user.type(await screen.findByLabelText('GitHub organization'), 'acme');
  await user.click(screen.getByRole('button', { name: 'Install GitHub App on GitHub' }));

  await waitFor(() => expect(apiMocks.startGitHubAppRegistration).toHaveBeenCalledTimes(1));
  expect(apiMocks.startGitHubAppRegistration.mock.calls[0][0]).toMatchObject({
    target: 'organization',
    organization: 'acme',
  });
});

// A deployment that has not configured the git-bot address yet still has to be
// able to finish the step, so the one value NopsAI cannot infer is asked for.
test('asks for the webhook URL only when the deployment has none', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitHubApp.mockResolvedValue({ ...unregistered, webhook_url: '', webhook_endpoint: '' });
  render(<SetupGitHubStep canManage />);

  const install = await screen.findByRole('button', { name: 'Install GitHub App on GitHub' });
  await waitFor(() => expect(install).toBeDisabled());

  await user.type(screen.getByLabelText('Webhook URL'), tunnelWebhook);
  expect(install).toBeEnabled();

  await user.click(install);
  await waitFor(() => expect(apiMocks.startGitHubAppRegistration.mock.calls[0][1]).toBe(tunnelWebhook));
});

test('sends an already registered App straight to GitHub repository selection', async () => {
  const user = userEvent.setup();
  const assign = vi.fn();
  const originalLocation = window.location;
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...originalLocation, assign, search: '' },
  });
  apiMocks.fetchGitHubApp.mockResolvedValue({ ...unregistered, app_id: '123456', app_slug: 'nopsai-example' });

  try {
    render(<SetupGitHubStep canManage />);
    await user.click(await screen.findByRole('button', { name: 'Install on another GitHub account' }));

    await waitFor(() => expect(apiMocks.startGitHubAppInstall).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(
      'https://github.com/apps/nopsai-example/installations/new?state=install-state'
    ));
    expect(apiMocks.startGitHubAppRegistration).not.toHaveBeenCalled();
  } finally {
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation });
  }
});

// "Does it need /webhook?" has to be answerable without reading the API source,
// so the step shows the address GitHub will actually be registered with.
test('shows the resolved delivery address for a bare tunnel URL', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitHubApp.mockResolvedValue({ ...unregistered, webhook_url: '', webhook_endpoint: '' });
  render(<SetupGitHubStep canManage />);

  await user.type(await screen.findByLabelText('Webhook URL'), 'https://live-gecko-national.ngrok-free.app');

  expect(screen.getByText(tunnelWebhook)).toBeVisible();
});
