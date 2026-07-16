import { useState } from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Check, Plus } from 'lucide-react';
import { expect, test, vi } from 'vitest';
import {
  WorkflowDialogFrame,
  WorkflowEmptyState,
  WorkflowIconButton,
  WorkflowInlineAlert,
} from './WorkflowPrimitives';

function DialogHarness({ onClose }: { onClose: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Open dialog</button>
      {open ? (
        <WorkflowDialogFrame
          id="workflow-primitive-dialog"
          titleId="workflow-primitive-dialog-title"
          descriptionId="workflow-primitive-dialog-description"
          onClose={() => {
            setOpen(false);
            onClose();
          }}
          className="pipelines-modal-card max-w-md w-full"
        >
          <h3 id="workflow-primitive-dialog-title">Shared primitive</h3>
          <p id="workflow-primitive-dialog-description">Keyboard focus stays inside this dialog.</p>
          <button type="button" data-dialog-initial-focus>First action</button>
          <button type="button">Second action</button>
        </WorkflowDialogFrame>
      ) : null}
    </>
  );
}

test('frames dialogs with labels, modal semantics, focus trapping, and Escape close', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(<DialogHarness onClose={onClose} />);

  const opener = screen.getByRole('button', { name: 'Open dialog' });
  await user.click(opener);
  const dialog = screen.getByRole('dialog', { name: 'Shared primitive' });
  expect(dialog).toHaveAttribute('aria-modal', 'true');
  expect(dialog).toHaveAccessibleDescription('Keyboard focus stays inside this dialog.');
  expect(within(dialog).getByRole('button', { name: 'First action' })).toHaveFocus();

  within(dialog).getByRole('button', { name: 'Second action' }).focus();
  await user.tab();
  expect(within(dialog).getByRole('button', { name: 'First action' })).toHaveFocus();

  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
  expect(onClose).toHaveBeenCalledOnce();
});

test('closes shared dialogs when the backdrop is clicked', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(<DialogHarness onClose={onClose} />);

  await user.click(screen.getByRole('button', { name: 'Open dialog' }));
  const dialog = screen.getByRole('dialog', { name: 'Shared primitive' });
  const backdrop = dialog.parentElement;
  expect(backdrop).not.toBeNull();

  fireEvent.pointerDown(backdrop!);

  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(onClose).toHaveBeenCalledOnce();
});

test('renders shared alerts, empty states, and icon buttons with accessible names', async () => {
  const user = userEvent.setup();
  const onAction = vi.fn();
  const onIconClick = vi.fn();

  render(
    <>
      <WorkflowInlineAlert id="primitive-alert">Fix the workflow name.</WorkflowInlineAlert>
      <WorkflowEmptyState
        badge="Role details"
        hint="Select a role to continue."
        actionLabel="Create role"
        actionIcon={<Plus className="h-4 w-4" aria-hidden="true" />}
        onAction={onAction}
      />
      <WorkflowIconButton
        label="Approve"
        icon={<Check className="h-4 w-4" aria-hidden="true" />}
        onClick={onIconClick}
      />
    </>
  );

  expect(screen.getByRole('alert')).toHaveTextContent('Fix the workflow name.');
  await user.click(screen.getByRole('button', { name: 'Create role' }));
  await user.click(screen.getByRole('button', { name: 'Approve' }));
  expect(onAction).toHaveBeenCalledOnce();
  expect(onIconClick).toHaveBeenCalledOnce();
});
