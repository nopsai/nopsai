import type { FormEvent } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import AccessPanel, { type AccessPanelProps } from '../AccessPanel';
import { createEmptyAccessResourceCatalog } from './resourceCatalog';

function baseProps(): AccessPanelProps {
  return {
    users: [
      {
        id: 'user-1',
        sub: 'alice',
        email: 'alice@example.com',
        status: 'active',
        roles: [{ role: 'developer' }],
      },
      {
        id: 'user-2',
        sub: 'bob',
        email: 'bob@example.com',
        status: 'disabled',
        roles: [{ role: 'viewer' }],
      },
    ],
    loading: false,
    error: null,
    serviceAccounts: [
      {
        id: 'svc-1',
        sub: 'deploy-bot',
        email: 'deploy@example.com',
        provider: 'service-account',
        status: 'active',
        token_count: 1,
        roles: [{ role: 'viewer' }],
      },
    ],
    serviceAccountsLoading: false,
    serviceAccountsError: null,
    accessGrants: [
      {
        id: 'grant-1',
        subjectType: 'user',
        subjectID: 'user-1',
        role: 'viewer',
        resourceType: 'team',
        resourceID: 'root',
        inherit: true,
      },
    ],
    accessGrantsLoading: false,
    accessGrantsError: null,
    identityProviders: [],
    identityProviderSettings: {
      local_enabled: true,
      oidc_enabled: false,
      auto_create_users: false,
      default_role: '',
      allow_email_linking: false,
    },
    identityProviderDomainMappings: {},
    identityProvidersLoading: false,
    identityProvidersError: null,
    policies: [
      {
        role: 'developer',
        name: 'Execute pipelines',
        obj: 'pipeline:*',
        act: 'pipeline.execute',
      },
      {
        role: 'viewer',
        name: 'Read pipelines',
        obj: 'pipeline:*',
        act: 'pipeline.read',
      },
    ],
    policiesLoading: false,
    policiesError: null,
    resourceCatalog: createEmptyAccessResourceCatalog(),
    newUser: { sub: '', email: '', password: '', roles: [] },
    newServiceAccount: { sub: '', email: '', tokenName: 'default', roles: [] },
    policyTemplates: [
      {
        role: '__policy_template__',
        name: 'Read pipelines',
        obj: 'pipeline:*',
        act: 'pipeline.read',
      },
    ],
    onChangeUser: vi.fn(),
    onCreateUser: vi.fn(async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      return true;
    }),
    onChangeServiceAccount: vi.fn(),
    onCreateServiceAccount: vi.fn(async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      return null;
    }),
    onCreatePermission: vi.fn(async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
    }),
    newPermission: { name: '', obj: 'pipeline:*', act: 'pipeline.read' },
    onChangePermission: vi.fn(),
    onDeleteUser: vi.fn(async () => undefined),
    onDeleteServiceAccount: vi.fn(async () => undefined),
    onDeletePolicy: vi.fn(async () => undefined),
    onDeleteRoleDefinition: vi.fn(async () => undefined),
    onSaveRoleDefinition: vi.fn(async () => undefined),
    onEditPolicy: vi.fn(async () => undefined),
    onUpdateUserRoles: vi.fn(async () => undefined),
    onUpdateServiceAccountRoles: vi.fn(async () => undefined),
    onCreateAccessGrant: vi.fn(async () => undefined),
    onCreateServiceAccountAccessGrant: vi.fn(async () => undefined),
    onDeleteAccessGrant: vi.fn(async () => undefined),
    onSaveIdentityProviderSettings: vi.fn(async () => undefined),
    onSaveIdentityProvider: vi.fn(async () => undefined),
    onDeleteIdentityProvider: vi.fn(async () => undefined),
    onUpdateUser: vi.fn(async () => undefined),
    onUpdateServiceAccount: vi.fn(async () => undefined),
    onLoadServiceAccountTokens: vi.fn(async () => []),
    onCreateServiceAccountToken: vi.fn(async () => ({
      id: 'token-1',
      name: 'default',
      token: 'nopsat_secret',
      token_suffix: 'secret',
      created_at: '2026-07-30T00:00:00Z',
    })),
    onRevokeServiceAccountToken: vi.fn(async () => undefined),
  };
}

test('renders the redesigned access shell and searches records without filter or export controls', async () => {
  const user = userEvent.setup();

  render(
    <MemoryRouter>
      <AccessPanel {...baseProps()} />
    </MemoryRouter>,
  );

  expect(screen.queryByRole('heading', { name: 'Access management' })).not.toBeInTheDocument();
  expect(screen.getByText('Active users')).toBeVisible();
  expect(screen.getByText('alice')).toBeVisible();
  expect(screen.getByText('bob')).toBeVisible();
  expect(screen.queryByLabelText('Filter by state')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Filter by scope')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Export' })).not.toBeInTheDocument();

  await user.click(
    screen.getByRole('button', { name: /Search by username, email, role, or team/ }),
  );
  await user.type(
    screen.getByPlaceholderText('Search by username, email, role, or team'),
    'alice',
  );
  expect(screen.getByText('alice')).toBeVisible();
  expect(screen.queryByText('bob')).not.toBeInTheDocument();
});

test('closes drawer editors when access mode or advanced section changes', async () => {
  const user = userEvent.setup();

  render(
    <MemoryRouter>
      <AccessPanel {...baseProps()} />
    </MemoryRouter>,
  );

  await user.click(screen.getByRole('tab', { name: 'Advanced' }));
  await user.click(screen.getByRole('tab', { name: /Service accounts/ }));
  await user.click(screen.getByText('deploy-bot'));

  const dialog = screen.getByRole('dialog', { name: 'Service account editor' });
  expect(within(dialog).getByRole('button', { name: 'Close' })).toHaveFocus();
  await user.click(within(dialog).getByRole('button', { name: /Credentials/ }));
  expect(within(dialog).getByRole('heading', { name: 'Tokens' })).toBeVisible();
  await user.click(within(dialog).getByRole('button', { name: /Review/ }));
  expect(within(dialog).getByText('Token activity')).toBeVisible();

  await user.keyboard('{Escape}');
  expect(
    screen.queryByRole('dialog', { name: 'Service account editor' }),
  ).not.toBeInTheDocument();

  await user.click(screen.getByText('deploy-bot'));
  expect(
    screen.getByRole('dialog', { name: 'Service account editor' }),
  ).toBeVisible();

  await user.click(screen.getByRole('tab', { name: 'Basic' }));
  expect(
    screen.queryByRole('dialog', { name: 'Service account editor' }),
  ).not.toBeInTheDocument();

  await user.click(screen.getByRole('tab', { name: 'Advanced' }));
  await user.click(screen.getByRole('tab', { name: /Service accounts/ }));
  expect(
    screen.queryByRole('dialog', { name: 'Service account editor' }),
  ).not.toBeInTheDocument();
});

test('renders sectioned drawers for custom roles and reusable policies', async () => {
  const user = userEvent.setup();
  const props = baseProps();
  props.policies = [
    ...props.policies,
    {
      role: 'dispatcher-internal',
      name: 'Finalize runs',
      obj: 'pipeline_run:*',
      act: 'pipeline_run.finalize',
    },
  ];

  render(
    <MemoryRouter>
      <AccessPanel {...props} />
    </MemoryRouter>,
  );

  await user.click(screen.getByRole('tab', { name: 'Advanced' }));
  await user.click(screen.getByRole('tab', { name: /^Roles/ }));
  await user.click(screen.getByText('dispatcher-internal'));

  const roleDialog = screen.getByRole('dialog', { name: 'Role editor' });
  await user.click(within(roleDialog).getByRole('button', { name: /Permissions/ }));
  expect(within(roleDialog).getByText('Included policies')).toBeVisible();
  await user.click(within(roleDialog).getByRole('button', { name: /Review/ }));
  expect(within(roleDialog).getByText('Role summary')).toBeVisible();

  await user.click(screen.getByRole('tab', { name: /^Policies/ }));
  await user.click(screen.getByText('Finalize runs'));

  const policyDialog = screen.getByRole('dialog', { name: 'Policy editor' });
  await user.click(within(policyDialog).getByRole('button', { name: /Review/ }));
  expect(within(policyDialog).getByText('Effective rule')).toBeVisible();
});
