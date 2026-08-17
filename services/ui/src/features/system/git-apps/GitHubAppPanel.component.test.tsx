import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import GitHubAppPanel from './GitHubAppPanel';
import { useGitHubApp } from './useGitHubApp';

const app = {
  provider: 'github' as const,
  app_id: '123456',
  private_key_credential_ref: 'credential://system/github/app-private-key',
  webhook_credential_ref: 'credential://system/github/webhook-secret',
  webhook_url: 'https://live-gecko-national.ngrok-free.app/webhook',
  webhook_endpoint: 'https://live-gecko-national.ngrok-free.app/webhook',
  app_slug: 'nopsai-example',
  installations: [{
    installation_id: '987654',
    account_login: 'nopsai',
    account_type: 'organization' as const,
    enabled: true,
    accessible_repositories: 3,
    connected_triggers: 2,
    status: 'connected',
  }],
};

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

vi.mock('./api', () => apiMocks);
vi.mock('./manifestForm', () => manifestMocks);

beforeEach(() => {
  apiMocks.deleteGitHubAppInstallation.mockReset();
  apiMocks.fetchGitHubApp.mockReset();
  apiMocks.fetchGitHubAppInstallationRepositories.mockReset();
  apiMocks.refreshGitHubAppInstallation.mockReset();
  apiMocks.saveGitHubApp.mockReset();
  apiMocks.saveGitHubAppInstallation.mockReset();
  apiMocks.verifyGitHubAppInstallation.mockReset();
  apiMocks.startGitHubAppInstall.mockReset();
  apiMocks.startGitHubAppRegistration.mockReset();
  manifestMocks.clearGitHubAppCallbackParams.mockReset();
  manifestMocks.submitGitHubAppManifest.mockReset();
  apiMocks.fetchGitHubApp.mockResolvedValue(app);
  apiMocks.startGitHubAppRegistration.mockResolvedValue({
    state: 'state-value',
    post_url: 'https://github.com/organizations/acme/settings/apps/new?state=state-value',
    manifest: '{"name":"NopsAI"}',
    app_name: 'NopsAI',
    webhook_endpoint: 'https://live-gecko-national.ngrok-free.app/webhook',
    expires_at: '2026-06-15T10:00:00Z',
  });
  apiMocks.startGitHubAppInstall.mockResolvedValue({
    state: 'install-state',
    install_url: 'https://github.com/apps/nopsai-example/installations/new?state=install-state',
    expires_at: '2026-06-15T10:00:00Z',
  });
  apiMocks.saveGitHubApp.mockImplementation(async (form, installations) => ({
    ...app,
    app_id: form.appID,
    private_key_credential_ref: form.privateKeyCredentialRef,
    webhook_credential_ref: form.webhookCredentialRef,
    installations,
  }));
  apiMocks.saveGitHubAppInstallation.mockResolvedValue({
    installation_id: '456789',
    account_login: 'acme',
    account_type: 'organization',
    enabled: true,
    accessible_repositories: 0,
    connected_triggers: 0,
    status: 'connected',
  });
  apiMocks.refreshGitHubAppInstallation.mockResolvedValue({
    ...app.installations[0],
    repositories: [{ id: 1, full_name: 'nopsai/api', owner: 'nopsai', name: 'api', private: true, used_by_nopsai: true }],
  });
});

function Harness({ canManage = true }: { canManage?: boolean }) {
  const controller = useGitHubApp({ enabled: true, canManage });
  return <GitHubAppPanel controller={controller} canManage={canManage} />;
}

test('renders GitHub App settings and saves app-scoped fields', async () => {
  const user = userEvent.setup();
  render(<Harness />);

  expect(await screen.findByRole('heading', { name: 'GitHub App' })).toBeVisible();
  expect(screen.getByText('setting/git-apps/github.yaml')).toBeVisible();
  expect(screen.queryByText('GitHub installation ID')).not.toBeInTheDocument();
  expect(await screen.findByRole('button', { name: 'nopsai' })).toBeVisible();
  expect(apiMocks.fetchGitHubAppInstallationRepositories).not.toHaveBeenCalled();
  expect(apiMocks.refreshGitHubAppInstallation).not.toHaveBeenCalled();
  expect(screen.getByText('Connected')).toBeVisible();
  const verifyAction = screen.getByRole('button', { name: 'Verify nopsai' });
  expect(verifyAction).toHaveClass('github-app-action--verify');
  expect(verifyAction).not.toHaveClass('glass-button-ghost');
  expect(verifyAction.querySelector('svg')).not.toBeNull();
  expect(screen.getByRole('button', { name: 'Refresh repositories for nopsai' })).toHaveClass('github-app-action--sync');

  await user.clear(screen.getByLabelText('App ID'));
  await user.type(screen.getByLabelText('App ID'), '777777');
  await user.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(apiMocks.saveGitHubApp).toHaveBeenCalledTimes(1));
  expect(apiMocks.saveGitHubApp.mock.calls[0][0]).toMatchObject({
    appID: '777777',
    privateKeyCredentialRef: 'credential://system/github/app-private-key',
    webhookCredentialRef: 'credential://system/github/webhook-secret',
    webhookURL: 'https://live-gecko-national.ngrok-free.app/webhook',
  });
  expect(apiMocks.saveGitHubApp.mock.calls[0][1][0]).toMatchObject({
    installation_id: '987654',
    account_login: 'nopsai',
  });
});

test('creates installations and refreshes repositories from the panel', async () => {
  const user = userEvent.setup();
  render(<Harness />);

  await screen.findByRole('button', { name: 'nopsai' });
  await user.click(screen.getByRole('button', { name: 'Add manually' }));
  expect(screen.getByRole('dialog', { name: 'Add GitHub installation' })).toBeVisible();
  await user.type(screen.getByLabelText('Installation ID'), '456789');
  await user.type(screen.getByLabelText('Account login'), 'acme');
  await user.click(screen.getByRole('button', { name: 'Save installation' }));

  await waitFor(() => expect(apiMocks.saveGitHubAppInstallation).toHaveBeenCalledTimes(1));
  expect(apiMocks.saveGitHubAppInstallation.mock.calls[0][0]).toMatchObject({
    installationID: '456789',
    accountLogin: 'acme',
    accountType: 'organization',
    enabled: true,
  });
  expect(await screen.findByRole('button', { name: 'acme' })).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'nopsai' }));
  await user.click(screen.getByRole('button', { name: 'Refresh repositories for nopsai' }));
  await waitFor(() => expect(apiMocks.refreshGitHubAppInstallation).toHaveBeenCalledWith('987654'));
  expect(await screen.findByText('nopsai/api')).toBeVisible();
});

test('registers a GitHub App from the connect dialog by posting the manifest to GitHub', async () => {
  const user = userEvent.setup();
  render(<Harness />);

  await screen.findByRole('button', { name: 'nopsai' });
  await user.click(screen.getByRole('button', { name: 'Replace App' }));

  const dialog = await screen.findByRole('dialog', { name: 'Replace GitHub App' });
  await user.clear(within(dialog).getByLabelText('Organization'));
  await user.type(within(dialog).getByLabelText('Organization'), 'acme');
  await user.click(within(dialog).getByRole('button', { name: 'Continue on GitHub' }));

  await waitFor(() => expect(apiMocks.startGitHubAppRegistration).toHaveBeenCalledTimes(1));
  expect(apiMocks.startGitHubAppRegistration.mock.calls[0][0]).toMatchObject({
    target: 'organization',
    organization: 'acme',
  });
  // The delivery address is the tunnel in front of git-bot, not the NopsAI address.
  expect(apiMocks.startGitHubAppRegistration.mock.calls[0][1])
    .toBe('https://live-gecko-national.ngrok-free.app/webhook');
  await waitFor(() => expect(manifestMocks.submitGitHubAppManifest).toHaveBeenCalledTimes(1));
  expect(manifestMocks.submitGitHubAppManifest.mock.calls[0][0]).toMatchObject({
    manifest: '{"name":"NopsAI"}',
  });
});

test('sends the operator to GitHub to choose repositories for an installation', async () => {
  const user = userEvent.setup();
  const assign = vi.fn();
  const originalLocation = window.location;
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...originalLocation, assign, search: '' },
  });

  try {
    render(<Harness />);
    await screen.findByRole('button', { name: 'nopsai' });
    await user.click(screen.getByRole('button', { name: 'Install on an account' }));

    await waitFor(() => expect(apiMocks.startGitHubAppInstall).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(
      'https://github.com/apps/nopsai-example/installations/new?state=install-state'
    ));
  } finally {
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation });
  }
});

// NopsAI running on localhost behind a tunnel that only fronts git-bot is a
// supported setup, so the connect action must stay usable there.
test('connects with only a webhook URL, without a public NopsAI address', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitHubApp.mockResolvedValue({
    ...app,
    app_id: '',
    app_slug: '',
    webhook_url: '',
    webhook_endpoint: '',
    installations: [],
  });
  render(<Harness />);

  await user.click(await screen.findByRole('button', { name: 'Connect GitHub' }));
  const dialog = await screen.findByRole('dialog', { name: 'Connect GitHub App' });
  const submit = within(dialog).getByRole('button', { name: 'Continue on GitHub' });
  expect(submit).toBeDisabled();

  await user.type(
    within(dialog).getByLabelText('Webhook URL'),
    'https://live-gecko-national.ngrok-free.app/webhook'
  );
  expect(submit).toBeEnabled();

  await user.click(submit);
  await waitFor(() => expect(apiMocks.startGitHubAppRegistration).toHaveBeenCalledTimes(1));
  expect(apiMocks.startGitHubAppRegistration.mock.calls[0][1])
    .toBe('https://live-gecko-national.ngrok-free.app/webhook');
});

test('warns on the Git App card when the webhook URL is an address GitHub cannot reach', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitHubApp.mockResolvedValue({
    ...app,
    webhook_url: '',
    webhook_endpoint: '',
  });
  render(<Harness />);

  await screen.findByRole('button', { name: 'nopsai' });
  await user.type(screen.getByLabelText('Webhook URL'), 'http://localhost:8081/webhook');

  expect(await screen.findByText(/GitHub cannot reach localhost/)).toBeVisible();
});

// One App, many installations: the page must present a single Git App record
// rather than repeating its fields in a second card.
test('shows exactly one GitHub App record above the installation list', async () => {
  render(<Harness />);

  await screen.findByRole('button', { name: 'nopsai' });
  expect(screen.getAllByLabelText('App ID')).toHaveLength(1);
  expect(screen.getAllByLabelText('Webhook URL')).toHaveLength(1);
  expect(screen.getByRole('heading', { name: 'GitHub App' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'GitHub accounts' })).toBeVisible();
});
