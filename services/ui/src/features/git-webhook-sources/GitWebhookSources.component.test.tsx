import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import GitWebhookSourcesPage from '../../pages/GitWebhookSources';
import { GitWebhookSourceForm } from './GitWebhookSourceForm';
import { GitWebhookSourceCards } from './GitWebhookSourceCards';
import { gitWebhookSourceForm } from './model';

const source = {
  id: 'gitlab-platform',
  name: 'GitLab Platform',
  description: 'Primary GitLab source',
  provider: 'gitlab' as const,
  enabled: true,
  visibility: 'team' as const,
  auth_mode: 'static_token' as const,
  credential_ref: 'credential://system/webhooks/gitlab-platform',
  repository_allowlist: ['platform/*'],
  rate_limit: { per_minute: 120 },
  last_used_at: '2026-06-15T10:00:00Z',
  source: 'database',
  managed_by_config_repo: false,
};

const apiMocks = vi.hoisted(() => ({
  deleteGitWebhookSource: vi.fn(),
  fetchGitWebhookDeliveries: vi.fn(),
  fetchGitWebhookSource: vi.fn(),
  fetchGitWebhookSources: vi.fn(),
  fetchGitWebhookSourceTeamPaths: vi.fn(),
  saveGitWebhookSource: vi.fn(),
}));

vi.mock('./api', () => apiMocks);

beforeEach(() => {
  apiMocks.deleteGitWebhookSource.mockReset();
  apiMocks.fetchGitWebhookDeliveries.mockReset();
  apiMocks.fetchGitWebhookSource.mockReset();
  apiMocks.fetchGitWebhookSources.mockReset();
  apiMocks.fetchGitWebhookSourceTeamPaths.mockReset();
  apiMocks.saveGitWebhookSource.mockReset();
  apiMocks.fetchGitWebhookDeliveries.mockResolvedValue([]);
  apiMocks.fetchGitWebhookSource.mockResolvedValue(source);
  apiMocks.fetchGitWebhookSources.mockResolvedValue([]);
  apiMocks.fetchGitWebhookSourceTeamPaths.mockResolvedValue(['platform', 'platform/prod']);
});

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
  expect(screen.getByRole('complementary', { name: 'Team tree' })).toBeVisible();
  expect(screen.getByRole('separator', { name: 'Resize team tree' })).toBeVisible();
  expect(screen.getByRole('link', { name: 'External API' })).toHaveAttribute('href', '/external-triggers');
  expect(await screen.findByText('platform/api')).toBeVisible();
  expect(screen.getByText('Triggers connected')).toBeVisible();
  expect(screen.getByText(/\/v1\/git\/webhooks\/gitlab-platform$/)).toBeVisible();
  expect(screen.getByText('processed')).toBeVisible();
});

test('shows source details after selecting a row from the list route', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitWebhookSources.mockResolvedValue([source]);
  apiMocks.fetchGitWebhookSource.mockResolvedValue(source);
  apiMocks.fetchGitWebhookDeliveries.mockResolvedValue([]);

  render(
    <MemoryRouter initialEntries={['/git-webhook-sources']}>
      <GitWebhookSourcesPage canWriteGitWebhookSources canDeleteGitWebhookSources />
    </MemoryRouter>
  );

  expect(await screen.findByRole('button', { name: 'GitLab Platform' })).toBeVisible();
  expect(screen.getByRole('complementary', { name: 'Team tree' })).toBeVisible();
  expect(screen.getByRole('separator', { name: 'Resize team tree' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'All teams (1)' })).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Last used' })).toBeVisible();
  expect(screen.queryByText('Select a webhook source')).not.toBeInTheDocument();
  expect(screen.queryByText(/\/v1\/git\/webhooks\/gitlab-platform$/)).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'GitLab Platform' }));

  expect(await screen.findByText(/\/v1\/git\/webhooks\/gitlab-platform$/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
});

test('renders and selects managed webhook sources as compact GitOps cards', async () => {
  const user = userEvent.setup();
  const onSelect = vi.fn();
  render(
    <GitWebhookSourceCards
      sources={[{
        ...source,
        name: '',
        description: '',
        enabled: false,
        managed_by_config_repo: true,
      }]}
      selectedID=""
      onSelect={onSelect}
    />
  );

  expect(screen.getByText('Disabled')).toBeVisible();
  expect(screen.getByText('GitOps')).toBeVisible();
  expect(screen.getByRole('article')).toHaveClass('compact-resource-card--bordered', 'git-webhook-source-card');
  expect(screen.getByText('1')).toBeVisible();
  const selector = screen.getByRole('button', { name: 'Select Git webhook source gitlab-platform' });
  expect(selector).toHaveAttribute('aria-pressed', 'false');
  await user.click(selector);
  expect(onSelect).toHaveBeenCalledWith('gitlab-platform');
});

test('creates a source through the feature-owned form and API', async () => {
  const user = userEvent.setup();
  apiMocks.fetchGitWebhookSources.mockResolvedValue([]);
  apiMocks.saveGitWebhookSource.mockImplementation(async request => ({
    ...source,
    ...request,
    credential_ref: request.credential_ref || 'credential://team/platform/prod/webhooks/gitlab-platform',
    generated_credential: {
      reference: request.credential_ref || 'credential://team/platform/prod/webhooks/gitlab-platform',
      value: 'generated-secret',
      auth_mode: request.auth_mode,
    },
  }));

  render(
    <MemoryRouter initialEntries={['/git-webhook-sources']}>
      <GitWebhookSourcesPage canWriteGitWebhookSources canDeleteGitWebhookSources />
    </MemoryRouter>
  );

  await screen.findByText('No webhook sources found');
  expect(screen.getByRole('button', { name: 'Search webhook sources' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Refresh Git webhook sources' })).toBeVisible();
  expect(screen.queryByText('Event automation')).not.toBeInTheDocument();
  expect(screen.queryByText('Git webhook sources')).not.toBeInTheDocument();
  expect(screen.queryByText('Connect trusted Git providers, restrict repositories, and monitor webhook deliveries.')).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'New source' }));
  const dialog = screen.getByRole('dialog', { name: 'New Git webhook source' });
  expect(dialog).toBeVisible();
  expect(dialog).toHaveClass(
    'pipelines-modal-card',
    'workflow-form-dialog',
    'workflow-form-dialog--wide'
  );
  expect(dialog).not.toHaveClass('glass-card');
  expect(dialog.querySelector('.pipelines-modal-header')).not.toBeNull();
  expect(dialog.querySelector('.pipelines-modal-body')).not.toBeNull();
  expect(dialog.querySelector('.pipelines-modal-footer')).not.toBeNull();
  expect(screen.getByLabelText('Source ID')).toHaveFocus();
  await user.type(screen.getByLabelText('Source ID'), 'gitlab-platform');
  await user.selectOptions(screen.getByLabelText('Provider'), 'gitlab');
  await user.selectOptions(screen.getByRole('combobox', { name: /^Team/ }), 'platform/prod');
  await user.selectOptions(screen.getByLabelText('Authentication'), 'static_token');
  await user.type(screen.getByLabelText(/^Repository allowlist/), 'platform/*');
  await user.click(screen.getByRole('button', { name: 'Create source' }));

  await waitFor(() => expect(apiMocks.saveGitWebhookSource).toHaveBeenCalledTimes(1));
  expect(apiMocks.saveGitWebhookSource.mock.calls[0][0]).toMatchObject({
    id: 'gitlab-platform',
    provider: 'gitlab',
    team_path: 'platform/prod',
    visibility: 'team',
    auth_mode: 'static_token',
    repository_allowlist: ['platform/*'],
  });
  expect(apiMocks.saveGitWebhookSource.mock.calls[0][0]).not.toHaveProperty('credential_ref');
  expect(await screen.findByText('generated-secret')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Copy generated webhook secret' })).toBeVisible();
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
      teamPaths={['platform', 'platform/prod']}
      onChange={onChange}
      onClose={onClose}
      onSubmit={onSubmit}
    />
  );

  const dialog = screen.getByRole('dialog', { name: 'Edit Git webhook source' });
  expect(dialog).toHaveClass(
    'pipelines-modal-card',
    'workflow-form-dialog',
    'workflow-form-dialog--wide'
  );
  expect(dialog).not.toHaveClass('glass-card');
  expect(dialog).toHaveAccessibleDescription('Repository allowlist is required.');
  expect(screen.getByLabelText('Source ID')).toBeDisabled();
  expect(screen.getByRole('combobox', { name: /^Team/ })).toHaveValue('root');
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
      teamPaths={['platform', 'platform/prod']}
      onChange={onChange}
      onClose={onClose}
      onSubmit={onSubmit}
    />
  );

  expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
});
