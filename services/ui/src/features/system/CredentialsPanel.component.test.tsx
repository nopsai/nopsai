import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { act } from 'react';
import { MemoryRouter } from 'react-router-dom';
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

test('uses compact references and supports enable plus old-version deletion', async () => {
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
      <CredentialsPanel canManage />
    </MemoryRouter>
  );

  expect((await screen.findAllByText('OpenAI Primary'))[0]).toBeVisible();
  expect(screen.queryByText('credential://system/llm/openai-primary')).not.toBeInTheDocument();
  expect(screen.getByText('LLM')).toBeVisible();
  expect(screen.getAllByText('system')).not.toHaveLength(0);

  await user.click(screen.getByRole('button', { name: /openai primary/i }));
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
      <CredentialsPanel canManage />
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

test('filters by team scope and toggles compact catalog view', async () => {
  const user = userEvent.setup();
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential, teamCredential]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByText('2 credentials shown')).toBeVisible();
  expect(screen.getByRole('button', { name: /all credentials/i })).toBeVisible();
  await user.selectOptions(screen.getByLabelText('Filter by scope'), 'team');

  expect(screen.getByText('1 credential shown')).toBeVisible();
  expect(screen.getByRole('button', { name: /platform \/ ml/i })).toBeVisible();
  expect((await screen.findAllByText('Platform / ML'))[0]).toBeVisible();
  expect(screen.getByText('Mail')).toBeVisible();
  expect((await screen.findAllByText('SMTP Primary'))[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Compact view' }));
  expect(screen.getByRole('button', { name: 'Comfortable view' })).toBeVisible();

  await user.type(screen.getByLabelText('Search credentials'), 'does-not-exist');
  expect(screen.getByText('0 credentials shown')).toBeVisible();
  expect(screen.getByText('No matching credentials')).toBeVisible();
});

test('renders repeated team path credentials under the team once', async () => {
  apiMocks.fetchCredentials.mockResolvedValue([duplicatedTeamPathCredential]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage />
    </MemoryRouter>
  );

  expect((await screen.findAllByText('Team 1'))[0]).toBeVisible();
  expect(screen.getByRole('button', { name: /test/i })).toBeVisible();
  expect(screen.queryByText('Team 1 / Team 1')).not.toBeInTheDocument();
});

test('opens a credential detail from the credential query parameter', async () => {
  apiMocks.fetchCredentials.mockResolvedValue([disabledCredential]);
  apiMocks.fetchCredential.mockResolvedValue(disabledCredential);

  render(
    <MemoryRouter initialEntries={['/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai-primary']}>
      <CredentialsPanel canManage />
    </MemoryRouter>
  );

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
      <CredentialsPanel canManage />
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
