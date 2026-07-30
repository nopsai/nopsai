import { useState, type FormEvent } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { IdentityProvidersWorkspace } from './IdentityProvidersWorkspace';
import {
  emptyIdentityProviderForm,
  identityProviderFormFromRecord,
  type IdentityProviderFormState,
  type IdentityProviderRecord,
  type IdentityProviderSettings,
} from './model';

const provider: IdentityProviderRecord = {
  id: 'corporate',
  type: 'oidc',
  display_name: 'Company SSO',
  issuer: 'https://idp.company.com',
  client_id: 'nopsai-console',
  client_credential_ref: 'credential://system/oidc/corporate/client-secret',
  scopes: ['openid', 'email', 'profile'],
  allowed_email_domains: ['company.com'],
  team_claim: 'groups',
  role_mapping: { 'nopsai-admins': 'admin' },
  team_mapping: { engineering: 'Engineering' },
  basic_role_mapping: {
    'team-1-owner': { role: 'owner', resource: 'team:team-1' },
  },
  enabled: true,
  config_source: 'database',
  has_client_credential: true,
};

function IdentityProvidersHarness({
  initialEditorOpen = false,
  onSaveProvider = vi.fn(),
}: {
  initialEditorOpen?: boolean;
  onSaveProvider?: (form: IdentityProviderFormState) => void;
}) {
  const [settings] = useState<IdentityProviderSettings>({
    local_enabled: true,
    oidc_enabled: true,
    auto_create_users: false,
    default_role: 'viewer',
    allow_email_linking: false,
  });
  const [form, setForm] = useState<IdentityProviderFormState>(
    emptyIdentityProviderForm(),
  );
  const [selectedProvider, setSelectedProvider] =
    useState<IdentityProviderRecord | null>(null);
  const [editorOpen, setEditorOpen] = useState(initialEditorOpen);

  return (
    <MemoryRouter>
      <IdentityProvidersWorkspace
        providers={[provider]}
        filteredProviders={[provider]}
        settings={settings}
        form={form}
        selectedProvider={selectedProvider}
        editorOpen={editorOpen}
        loading={false}
        error={null}
        savingProvider={false}
        onFormChange={setForm}
        onEdit={nextProvider => {
          setSelectedProvider(nextProvider);
          setForm(identityProviderFormFromRecord(nextProvider));
          setEditorOpen(true);
        }}
        onCreate={() => {
          setSelectedProvider(null);
          setForm(emptyIdentityProviderForm());
          setEditorOpen(true);
        }}
        onCloseEditor={() => {
          setSelectedProvider(null);
          setForm(emptyIdentityProviderForm());
          setEditorOpen(false);
        }}
        onDelete={vi.fn()}
        onSubmitProvider={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault();
          onSaveProvider(form);
        }}
      />
    </MemoryRouter>
  );
}

test('renders a complete sectioned identity-provider editor', async () => {
  const user = userEvent.setup();

  render(<IdentityProvidersHarness />);

  expect(screen.queryByLabelText('Provider ID')).not.toBeInTheDocument();
  expect(screen.queryByRole('heading', { name: 'Login policy' })).not.toBeInTheDocument();
  expect(screen.getByLabelText('Status: Enabled')).toBeInTheDocument();

  await user.click(screen.getByText('Company SSO'));
  expect(screen.getByLabelText('Provider ID')).toHaveValue('corporate');
  expect(screen.getByLabelText('Provider ID')).toBeDisabled();

  await user.click(screen.getByRole('button', { name: /Connection/ }));
  expect(screen.getByLabelText('Issuer')).toHaveValue(
    'https://idp.company.com',
  );
  expect(screen.getByLabelText(/Client credential ref/)).toHaveValue(
    'credential://system/oidc/corporate/client-secret',
  );

  await user.click(screen.getByRole('button', { name: /Mappings/ }));
  expect(screen.getByLabelText('Role mappings')).toHaveValue(
    'nopsai-admins: admin',
  );
  expect(screen.getByLabelText('Auth team mappings')).toHaveValue(
    'engineering: Engineering',
  );
  expect(screen.getByLabelText('Basic role mappings')).toHaveValue(
    'team-1-owner: owner team:team-1',
  );

  await user.click(screen.getByRole('button', { name: /Review/ }));
  expect(screen.getByText('Credential')).toBeVisible();
  expect(screen.getAllByText('Credential reference').length).toBeGreaterThan(0);

  await user.click(screen.getByRole('button', { name: 'New' }));
  expect(screen.getByLabelText('Provider ID')).toBeEnabled();
  expect(screen.getByLabelText('Provider ID')).toHaveValue('');
});

test('submits the new identity-provider form through the existing save path', async () => {
  const user = userEvent.setup();
  const onSaveProvider = vi.fn();

  render(
    <IdentityProvidersHarness
      initialEditorOpen
      onSaveProvider={onSaveProvider}
    />,
  );

  await user.type(screen.getByLabelText('Provider ID'), 'entra');
  await user.selectOptions(screen.getByLabelText('Type'), 'microsoft');
  await user.type(screen.getByLabelText('Display name'), 'Entra ID');
  await user.type(screen.getByLabelText('Default role'), 'viewer');
  await user.click(screen.getByRole('button', { name: 'Save provider' }));

  expect(onSaveProvider).toHaveBeenCalledWith(
    expect.objectContaining({
      id: 'entra',
      type: 'microsoft',
      display_name: 'Entra ID',
      default_role: 'viewer',
      enabled: true,
    }),
  );
});
