import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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

  expect(await screen.findByText('OpenAI Primary')).toBeVisible();
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

test('creates credentials using namespace and name fields', async () => {
  const user = userEvent.setup();
  apiMocks.fetchCredentials.mockResolvedValue([]);

  render(
    <MemoryRouter>
      <CredentialsPanel canManage />
    </MemoryRouter>
  );
  await screen.findByText('No matching credentials');
  await user.click(screen.getByRole('button', { name: 'New credential' }));

  expect(screen.getByLabelText('Namespace')).toHaveValue('system');
  await user.type(screen.getByLabelText('Name / path'), 'mail/smtp-primary');
  expect(screen.getByText('credential://system/mail/smtp-primary')).toBeVisible();
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
