import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { AccessConfirmationDialog } from './AccessConfirmationDialog';

test('delegates destructive confirmation and cancellation', async () => {
  const onCancel = vi.fn();
  const onConfirm = vi.fn();
  const user = userEvent.setup();
  render(
    <AccessConfirmationDialog
      message="Delete this service account?"
      pending={false}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );

  expect(screen.getByRole('alertdialog', { name: 'Please confirm' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onConfirm).toHaveBeenCalledOnce();
  await user.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onCancel).toHaveBeenCalledOnce();
});
