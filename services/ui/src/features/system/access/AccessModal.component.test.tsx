import { useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { AccessEditorEmptyState, AccessModal } from './AccessModal';

function AccessModalHarness({ onConfirm }: { onConfirm: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Open confirmation</button>
      {open ? (
        <AccessModal
          kicker="Confirm"
          title="Delete role"
          subtitle="This action cannot be undone."
          variant="minimal"
          onClose={() => setOpen(false)}
        >
          <button data-dialog-initial-focus type="button" onClick={() => setOpen(false)}>Cancel</button>
          <button type="button" onClick={onConfirm}>Delete</button>
        </AccessModal>
      ) : null}
    </>
  );
}

test('labels the access alert dialog, traps focus, closes on Escape, and restores its opener', async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn();
  render(<AccessModalHarness onConfirm={onConfirm} />);
  const opener = screen.getByRole('button', { name: 'Open confirmation' });

  await user.click(opener);
  const dialog = screen.getByRole('alertdialog', { name: 'Delete role' });
  expect(within(dialog).getByText('This action cannot be undone.')).toBeVisible();
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();

  within(dialog).getByRole('button', { name: 'Delete' }).focus();
  await user.tab();
  expect(within(dialog).getByRole('button', { name: 'Close' })).toHaveFocus();
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
  expect(onConfirm).not.toHaveBeenCalled();
});

test('renders an actionable access editor empty state', async () => {
  const user = userEvent.setup();
  const onAction = vi.fn();
  render(
    <AccessEditorEmptyState
      sectionLabel="Role details"
      hint="Select a role."
      actionLabel="Create role"
      onAction={onAction}
    />
  );
  await user.click(screen.getByRole('button', { name: 'Create role' }));
  expect(onAction).toHaveBeenCalledOnce();
});
