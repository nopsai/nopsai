import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { act } from 'react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import CredentialsPanel from './CredentialsPanel';

const disabledCredential = {
  id: 'credential-1',
  reference: 'credential://system/llm/openai-primary',
  kind: 'api_key',
  description: 'Primary OpenAI key',
  status: 'disabled',
  has_value: false,
  active_version: 2,
  managed_by_config_repo: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
  versions: [
    { version: 2, created_at: '2026-01-02T00:00:00Z', created_by: 'admin' },
    { version: 1, created_at: '2026-01-01T00:00:00Z', created_by: 'admin' },
  ],
};

const teamCredential = {
  id: 'credential-2',
  reference: 'credential://team/platform/ml/mail/smtp-primary',
  kind: 'password',
  description: 'SMTP password for ML notifications',
  status: 'active',
  has_value: true,
  active_version: 1,
  managed_by_config_repo: true,
  created_at: '2026-01-03T00:00:00Z',
  updated_at: '2026-01-04T00:00:00Z',
  versions: [
    { version: 1, created_at: '2026-01-03T00:00:00Z', created_by: 'admin' },
  ],
};

const duplicatedTeamPathCredential = {
  id: 'credential-3',
  reference: 'credential://team/team-1/team-1/test',
  kind: 'api_key',
  description: 'Team credential with a repeated team path',
  status: 'active',
  has_value: true,
  active_version: 1,
  managed_by_config_repo: false,
  created_at: '2026-01-05T00:00:00Z',
  updated_at: '2026-01-05T00:00:00Z',
  versions: [
    { version: 1, created_at: '2026-01-05T00:00:00Z', created_by: 'admin' },
  ],
};

const apiMocks = vi.hoisted(() => ({
  activateCredentialVersion: vi.fn(),
  createCredential: vi.fn(),
  deleteCredential: vi.fn(),
  deleteCredentialVersion: vi.fn(),
  disableCredential: vi.fn(),
  enableCredential: vi.fn(),
  fetchCredential: vi.fn(),
  fetchCredentials: vi.fn(),
  rotateCredential: vi.fn(),
}));

vi.mock('./credentials/api', () => apiMocks);
vi.mock('../../lib/resourceTeams', () => ({
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
}));

beforeEach(() => {
  vi.restoreAllMocks();
  Object.values(apiMocks).forEach(mock => mock.mockReset());
});

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname}{location.search}</span>;
}

test('uses registry table references and supports enable plus old-version deletion', async () => {
  const user = userEvent.setup();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential]);
  apiMocks.fetchCredential.mockResolvedValue(disabledCredential);
  apiMocks.enableCredential.mockResolvedValue({ ...disabledCredential, status: 'active', has_value: true });
  apiMocks.deleteCredentialVersion.mockResolvedValue({
    ...disabledCredential,
    status: 'active',
    has_value: true,
    versions: [disabledCredential.versions[0]],
  });

  render(
    <MemoryRouter>
      <CredentialsPanel canManage isNopsAIAdmin />
      <LocationProbe />
    </MemoryRouter>
  );

  expect((await screen.findAllByText('OpenAI Primary'))[0]).toBeVisible();
  expect(screen.queryByText('credential://system/llm/openai-primary')).not.toBeInTheDocument();
  expect(screen.getByText('API Key')).toBeVisible();
  expect(screen.getAllByText('System')).not.toHaveLength(0);

  await user.click(screen.getByRole('button', { name: /openai primary/i }));
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/credentials/system/llm/openai-primary'));
  expect(await screen.findByText('credential://system/llm/openai-primary')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Enable' }));
  await waitFor(() => expect(apiMocks.enableCredential).toHaveBeenCalledWith('credential-1'));

  await user.click(screen.getByRole('button', { name: 'Delete version 1' }));
  await waitFor(() => expect(apiMocks.deleteCredentialVersion).toHaveBeenCalledWith('credential-1', 1));
});

test('creates credentials using global and team scope fields', async () => {
  const user = userEvent.setup();
  apiMocks.fetchCredentials.mockResolvedValue([]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage isNopsAIAdmin />
    </MemoryRouter>
  );
  await screen.findByText('No matching credentials');
  await user.click(screen.getByRole('button', { name: 'New credential' }));

  expect(screen.getByLabelText('Team')).toHaveValue('');
  await user.type(screen.getByLabelText('Name / path'), 'mail/smtp-primary');
  expect(screen.getByText('credential://system/mail/smtp-primary')).toBeVisible();

  expect(await screen.findByRole('option', { name: '/platform/ml' })).toBeVisible();
  await user.selectOptions(screen.getByLabelText('Team'), 'platform/ml');
  expect(screen.getByText('credential://team/platform/ml/mail/smtp-primary')).toBeVisible();
});

test('filters by team scope and toggles registry grouping', async () => {
  const user = userEvent.setup();
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential, teamCredential]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage isNopsAIAdmin />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Credential summary')).toBeVisible();
  expect(screen.getByRole('button', { name: /all credentials/i })).toBeVisible();
  expect(await screen.findByRole('button', { name: /system \(1\)/i })).toBeVisible();
  expect(screen.queryByLabelText('Filter by scope')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Filter by status')).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: /teams \(1\)/i }));

  expect((await screen.findAllByText('Platform / ML'))[0]).toBeVisible();
  expect(screen.getAllByText('Password')).not.toHaveLength(0);
  expect((await screen.findAllByText('SMTP Primary'))[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Flat list' }));
  expect(screen.getByRole('button', { name: 'Group by scope' })).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Search credentials' }));
  await user.type(screen.getByLabelText('Search credentials query'), 'does-not-exist');
  expect(screen.getByText('No matching credentials')).toBeVisible();
});

test('limits non-admin credentials to team scopes and team creation', async () => {
  const user = userEvent.setup();
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential, teamCredential]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Credential summary')).toBeVisible();
  expect(await screen.findByRole('button', { name: /teams \(1\)/i })).toBeVisible();
  expect(screen.queryByText('OpenAI Primary')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /system/i })).not.toBeInTheDocument();

  const createButton = screen.getByRole('button', { name: 'New credential' });
  await waitFor(() => expect(createButton).toBeEnabled());
  await user.click(createButton);

  expect(screen.queryByRole('option', { name: 'System / global' })).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText('Team')).toHaveValue('platform/ml'));
  await user.type(screen.getByLabelText('Name / path'), 'mail/smtp-primary');
  expect(screen.getByText('credential://team/platform/ml/mail/smtp-primary')).toBeVisible();
});

test('renders repeated team path credentials under the team once', async () => {
  apiMocks.fetchCredentials.mockResolvedValue([duplicatedTeamPathCredential]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage isNopsAIAdmin />
    </MemoryRouter>
  );

  expect((await screen.findAllByText('Team 1'))[0]).toBeVisible();
  expect(screen.getByRole('button', { name: /test/i })).toBeVisible();
  expect(screen.queryByText('Team 1 / Team 1')).not.toBeInTheDocument();
});

test('shows credential rotation failures inside the open drawer', async () => {
  const user = userEvent.setup();
  apiMocks.fetchCredentials.mockResolvedValue([teamCredential]);
  apiMocks.fetchCredential.mockResolvedValue(teamCredential);
  apiMocks.rotateCredential.mockRejectedValue(new Error("parse docker config json: invalid character 'g' looking for beginning of value"));

  render(
    <MemoryRouter>
      <CredentialsPanel canManage isNopsAIAdmin />
    </MemoryRouter>
  );

  await user.click(await screen.findByRole('button', { name: /smtp primary/i }));
  const dialog = await screen.findByRole('dialog', { name: /smtp primary/i });

  await user.type(within(dialog).getByLabelText('New credential value'), 'garbage');
  await user.click(within(dialog).getByRole('button', { name: 'Rotate credential' }));

  await waitFor(() => expect(apiMocks.rotateCredential).toHaveBeenCalledWith('credential-2', 'garbage'));
  expect(screen.getAllByRole('alert')).toHaveLength(1);
  expect(within(dialog).getByRole('alert')).toHaveTextContent("parse docker config json: invalid character 'g' looking for beginning of value");
});

test('opens a credential detail from the credential query parameter', async () => {
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential]);
  apiMocks.fetchCredential.mockResolvedValue(disabledCredential);

  render(
    <MemoryRouter initialEntries={['/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai-primary']}>
      <CredentialsPanel canManage isNopsAIAdmin />
      <LocationProbe />
    </MemoryRouter>
  );

  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/credentials/system/llm/openai-primary'));
  expect(await screen.findByText('credential://system/llm/openai-primary')).toBeVisible();
  expect(apiMocks.fetchCredential).toHaveBeenCalledWith('credential-1');
});

test('does not reopen credential details after closing while detail loading is in flight', async () => {
  const user = userEvent.setup();
  let resolveDetail: ((value: typeof disabledCredential) => void) | null = null;
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential]);
  apiMocks.fetchCredential.mockImplementation(() => new Promise<typeof disabledCredential>(resolve => {
    resolveDetail = resolve;
  }));

  render(
    <MemoryRouter>
      <CredentialsPanel canManage isNopsAIAdmin />
    </MemoryRouter>
  );

  await user.click(await screen.findByRole('button', { name: /openai primary/i }));
  expect(await screen.findByRole('dialog', { name: /openai primary/i })).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Close credential details' }));
  await waitFor(() => expect(screen.queryByRole('dialog', { name: /openai primary/i })).not.toBeInTheDocument());

  await act(async () => {
    resolveDetail?.({ ...disabledCredential, description: 'Fetched detail should stay closed' });
  });

  expect(screen.queryByText('Fetched detail should stay closed')).not.toBeInTheDocument();
});
