import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { AccessPoliciesCatalog, AccessUsersCatalog } from './AccessEntityCatalogs';

test('renders user assignments and delegates edit and delete actions', async () => {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const user = userEvent.setup();
  const account = {
    id: 'user-1',
    sub: 'alice',
    email: 'alice@example.com',
    status: 'active',
    roles: [{ role: 'developer' }],
  };
  const grant = {
    id: 'grant-1',
    subjectType: 'user',
    subjectID: 'user-1',
    role: 'viewer',
    resourceType: 'folder',
    resourceID: 'platform',
    inherit: true,
  };

  render(
    <AccessUsersCatalog
      users={[account]}
      filteredUsers={[account]}
      grantMap={new Map([['user-1', [grant]]])}
      selectedUserID="user-1"
      loading={false}
      error={null}
      grantsLoading={false}
      grantsError={null}
      onEdit={onEdit}
      onDelete={onDelete}
    />
  );

  expect(screen.getByText('developer')).toBeInTheDocument();
  expect(screen.getByText(/viewer/i)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Edit alice' }));
  await user.click(screen.getByRole('button', { name: 'Delete alice' }));
  expect(onEdit).toHaveBeenCalledWith(account);
  expect(onDelete).toHaveBeenCalledWith('user-1');
});

test('keeps protected AAA policies read-only', () => {
  render(
    <AccessPoliciesCatalog
      policies={[{ role: 'viewer', name: 'View pipelines', obj: 'pipeline:*', act: 'pipeline.read' }]}
      filteredPolicies={[{ role: 'viewer', name: 'View pipelines', obj: 'pipeline:*', act: 'pipeline.read' }]}
      loading={false}
      error={null}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />
  );

  expect(screen.getAllByText('Protected').length).toBeGreaterThan(0);
  expect(screen.queryByRole('button', { name: /edit view pipelines/i })).not.toBeInTheDocument();
});
