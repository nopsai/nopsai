import { useState } from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Check, Plus } from 'lucide-react';
import { expect, test, vi } from 'vitest';
import {
  WorkflowDialogCloseButton,
  WorkflowDialogFrame,
  WorkflowEmptyState,
  WorkflowIconButton,
  WorkflowInlineAlert,
  WorkflowPropertyRow,
  WorkflowSegmentedControl,
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

test('closes a dialog from the title-pill icon button and can take opening focus', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(<WorkflowDialogCloseButton onClose={onClose} initialFocus />);

  const close = screen.getByRole('button', { name: 'Close' });
  expect(close).toHaveClass('workflow-dialog-close');
  expect(close).toHaveAttribute('data-dialog-initial-focus');

  await user.click(close);
  expect(onClose).toHaveBeenCalledOnce();
});

test('disables the close button while a dialog is saving', () => {
  render(<WorkflowDialogCloseButton onClose={vi.fn()} disabled />);

  expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
});

test('lays out a property row with its label, hint, and control', () => {
  render(
    <WorkflowPropertyRow label="Kind" hint="Spec classification" htmlFor="property-row-kind" span="full">
      <select id="property-row-kind" aria-label="Kind">
        <option value="runbook">runbook</option>
      </select>
    </WorkflowPropertyRow>
  );

  const control = screen.getByRole('combobox', { name: 'Kind' });
  const row = control.closest('.modal-property-row');
  expect(row).toHaveClass('modal-property-row--full');
  expect(within(row as HTMLElement).getByText('Spec classification')).toHaveClass('modal-property-hint');
  expect(control.parentElement).toHaveClass('modal-property-control');
});

test('reports the chosen option from the segmented control', async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <WorkflowSegmentedControl
      name="content-source"
      legend="Content source"
      value="inline"
      options={[
        { value: 'inline', label: 'Inline content' },
        { value: 'external', label: 'External page' },
      ]}
      onChange={onChange}
      stretch
    />
  );

  const group = screen.getByRole('group', { name: 'Content source' });
  expect(group).toHaveClass('modal-segmented', 'modal-segmented--stretch');
  expect(screen.getByRole('radio', { name: 'Inline content' })).toBeChecked();

  await user.click(screen.getByRole('radio', { name: 'External page' }));
  expect(onChange).toHaveBeenCalledWith('external');
});

test('names a control that brought no id of its own, without folding in the hint', () => {
  render(
    <WorkflowPropertyRow label="Run team" hint="Owning team">
      <select>
        <option value="platform">platform</option>
      </select>
    </WorkflowPropertyRow>
  );

  // The accessible name is the label alone: a wrapping label would have made it
  // "Run team Owning team" and broken every getByLabelText in the product.
  const control = screen.getByLabelText('Run team');
  expect(control.tagName).toBe('SELECT');
  expect(control.id).not.toBe('');
});

test('leaves an id the caller already set alone', () => {
  render(
    <WorkflowPropertyRow label="Scope" hint="Variables and secrets" htmlFor="scope-field">
      <select id="scope-field">
        <option value="default">default</option>
      </select>
    </WorkflowPropertyRow>
  );

  expect(screen.getByLabelText('Scope')).toHaveAttribute('id', 'scope-field');
});
