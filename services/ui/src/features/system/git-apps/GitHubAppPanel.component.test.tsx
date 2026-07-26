import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import GitHubAppPanel from './GitHubAppPanel';
import { useGitHubApp } from './useGitHubApp';

const app = {
  provider: 'github' as const,
  app_id: '123456',
  private_key_credential_ref: 'credential://system/github/app-private-key',
  webhook_credential_ref: 'credential://system/github/webhook-secret',
  webhook_endpoint: 'https://nopsai.example.com/webhook',
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
  verifyGitHubAppInstallation: vi.fn(),
}));

vi.mock('./api', () => apiMocks);

beforeEach(() => {
  apiMocks.deleteGitHubAppInstallation.mockReset();
  apiMocks.fetchGitHubApp.mockReset();
  apiMocks.fetchGitHubAppInstallationRepositories.mockReset();
  apiMocks.refreshGitHubAppInstallation.mockReset();
  apiMocks.saveGitHubApp.mockReset();
  apiMocks.saveGitHubAppInstallation.mockReset();
  apiMocks.verifyGitHubAppInstallation.mockReset();
  apiMocks.fetchGitHubApp.mockResolvedValue(app);
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
  await user.click(screen.getByRole('button', { name: 'Add installation' }));
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
