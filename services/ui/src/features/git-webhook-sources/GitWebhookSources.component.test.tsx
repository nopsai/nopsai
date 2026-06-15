import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import GitWebhookSourcesPage from '../../pages/GitWebhookSources';
import { GitWebhookSourceForm } from './GitWebhookSourceForm';
import { gitWebhookSourceForm } from './model';

const source = {
  id: 'gitlab-platform',
  name: 'GitLab Platform',
  description: 'Primary GitLab source',
  provider: 'gitlab' as const,
  enabled: true,
  auth_mode: 'static_token' as const,
  credential_ref: 'credential://system/webhooks/gitlab-platform',
  repository_allowlist: ['platform/*'],
  rate_limit: { per_minute: 120 },
  source: 'database',
  managed_by_config_repo: false,
};

const apiMocks = vi.hoisted(() => ({
  deleteGitWebhookSource: vi.fn(),
  fetchGitWebhookDeliveries: vi.fn(),
  fetchGitWebhookSource: vi.fn(),
  fetchGitWebhookSources: vi.fn(),
  saveGitWebhookSource: vi.fn(),
}));

vi.mock('./api', () => apiMocks);

test('renders source details and audited deliveries', async () => {
  apiMocks.fetchGitWebhookSources.mockResolvedValue([source]);
  apiMocks.fetchGitWebhookSource.mockResolvedValue(source);
  apiMocks.fetchGitWebhookDeliveries.mockResolvedValue([{
    id: 'delivery-1',
    source_id: source.id,
    delivery_id: 'provider-delivery-1',
    provider: 'gitlab',
    event_type: 'push',
    repository_full_name: 'platform/api',
    status: 'processed',
    run_ids: ['run-1'],
    received_at: '2026-06-15T10:00:00Z',
  }]);

  render(
    <MemoryRouter initialEntries={['/git-webhook-sources/gitlab-platform']}>
      <GitWebhookSourcesPage canWriteGitWebhookSources canDeleteGitWebhookSources />
    </MemoryRouter>
  );

  expect(await screen.findByRole('heading', { name: 'GitLab Platform' })).toBeVisible();
  expect(await screen.findByText('platform/api')).toBeVisible();
  expect(screen.getByDisplayValue(/\/v1\/git\/webhooks\/gitlab-platform$/)).toBeVisible();
  expect(screen.getByText('processed')).toBeVisible();
});

test('creates a source through the feature-owned form and API', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitWebhookSources.mockResolvedValue([]);
  apiMocks.saveGitWebhookSource.mockImplementation(async request => ({ ...source, ...request }));

  render(
    <MemoryRouter initialEntries={['/git-webhook-sources']}>
      <GitWebhookSourcesPage canWriteGitWebhookSources canDeleteGitWebhookSources />
    </MemoryRouter>
  );

  await screen.findByText('No Git webhook sources configured.');
  await user.click(screen.getByRole('button', { name: 'New source' }));
  const dialog = screen.getByRole('dialog', { name: 'New Git webhook source' });
  expect(dialog).toBeVisible();
  expect(dialog).not.toHaveClass('glass-card');
  expect(screen.getByLabelText('Source ID')).toHaveFocus();
  await user.type(screen.getByLabelText('Source ID'), 'gitlab-platform');
  await user.selectOptions(screen.getByLabelText('Provider'), 'gitlab');
  await user.selectOptions(screen.getByLabelText('Authentication'), 'static_token');
  await user.type(
    screen.getByLabelText(/^Credential reference/),
    'credential://system/webhooks/gitlab-platform'
  );
  await user.type(screen.getByLabelText(/^Repository allowlist/), 'platform/*');
  await user.click(screen.getByRole('button', { name: 'Create source' }));

  await waitFor(() => expect(apiMocks.saveGitWebhookSource).toHaveBeenCalledTimes(1));
  expect(apiMocks.saveGitWebhookSource.mock.calls[0][0]).toMatchObject({
    id: 'gitlab-platform',
    provider: 'gitlab',
    auth_mode: 'static_token',
    repository_allowlist: ['platform/*'],
  });
});

test('renders solid edit, validation, and saving states for source forms', () => {
  const onChange = vi.fn();
  const onClose = vi.fn();
  const onSubmit = vi.fn((event: React.FormEvent<HTMLFormElement>) => event.preventDefault());
  const form = { ...gitWebhookSourceForm(source), authMode: 'none' as const };
  const { rerender } = render(
    <GitWebhookSourceForm
      source={source}
      form={form}
      saving={false}
      error="Repository allowlist is required."
      onChange={onChange}
      onClose={onClose}
      onSubmit={onSubmit}
    />
  );

  const dialog = screen.getByRole('dialog', { name: 'Edit Git webhook source' });
  expect(dialog).not.toHaveClass('glass-card');
  expect(dialog).toHaveAccessibleDescription('Repository allowlist is required.');
  expect(screen.getByLabelText('Source ID')).toBeDisabled();
  expect(screen.getByText(/network-isolated ingress/)).toBeVisible();

  fireEvent.change(screen.getByLabelText('Display name'), { target: { value: 'GitLab' } });
  fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Primary source' } });
  fireEvent.change(screen.getByLabelText(/^Rate limit per minute/), { target: { value: '60' } });
  fireEvent.click(screen.getByLabelText('Accept webhook deliveries'));
  fireEvent.click(screen.getByRole('button', { name: 'Save source' }));
  fireEvent.click(screen.getByRole('button', { name: 'Close' }));

  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ name: 'GitLab' }));
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ description: 'Primary source' }));
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ rateLimitPerMinute: '60' }));
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
  expect(onSubmit).toHaveBeenCalledOnce();
  expect(onClose).toHaveBeenCalledOnce();

  rerender(
    <GitWebhookSourceForm
      source={source}
      form={form}
      saving
      error={null}
      onChange={onChange}
      onClose={onClose}
      onSubmit={onSubmit}
    />
  );

  expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
});
