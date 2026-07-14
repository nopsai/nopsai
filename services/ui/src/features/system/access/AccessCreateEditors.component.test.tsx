import type { FormEvent } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { CreateUserEditor } from './AccessCreateEditors';

test('delegates create-user editor role, basic grant, close, and submit actions', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  const onSubmit = vi.fn((event: FormEvent<HTMLFormElement>) => event.preventDefault());
  const onAppendRole = vi.fn();
  const onAddBasicGrant = vi.fn();

  render(
    <CreateUserEditor
      newUser={{ sub: 'alice', email: 'alice@example.com', password: 'secret', roles: [] }}
      creating={false}
      allRoleOptions={['developer']}
      nextRole=""
      basicGrantEntries={[]}
      basicGrantDraft={{ role: 'viewer', scope: 'root' }}
      basicGrantOptions={[{ value: 'root', label: 'Global' }]}
      basicGrantError={null}
      toneClassForRole={() => 'access-chip--muted'}
      onChangeUser={vi.fn()}
      onSubmit={onSubmit}
      onClose={onClose}
      onUpdateRoleEntry={vi.fn()}
      onRemoveRoleEntry={vi.fn()}
      onNextRoleChange={vi.fn()}
      onAppendRole={onAppendRole}
      onBasicGrantDraftChange={vi.fn()}
      onAddBasicGrant={onAddBasicGrant}
      onRemoveBasicGrant={vi.fn()}
    />
  );

  await user.click(screen.getByRole('button', { name: 'Close' }));
  expect(onClose).toHaveBeenCalledOnce();

  await user.click(screen.getByRole('button', { name: 'Add access role' }));
  expect(onAppendRole).toHaveBeenCalledOnce();

  await user.click(screen.getByRole('button', { name: 'Add basic role' }));
  expect(onAddBasicGrant).toHaveBeenCalledOnce();

  await user.click(screen.getByRole('button', { name: 'Save user' }));
  expect(onSubmit).toHaveBeenCalledOnce();
});
