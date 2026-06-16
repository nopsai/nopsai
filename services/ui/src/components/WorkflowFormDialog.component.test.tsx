import { fireEvent, render, screen, within } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { WorkflowFormDialog } from './WorkflowFormDialog';

test('renders the Pipeline form-dialog structure and submits its form', () => {
  const onClose = vi.fn();
  const onSubmit = vi.fn((event: React.FormEvent<HTMLFormElement>) => event.preventDefault());

  render(
    <WorkflowFormDialog
      id="shared-form-dialog"
      titleId="shared-form-dialog-title"
      descriptionId="shared-form-dialog-error"
      kicker="New workflow"
      title="Create workflow"
      subtitle="Configure the workflow."
      headerLeading={<span data-testid="dialog-icon">+</span>}
      actions={<button type="submit">Create</button>}
      onClose={onClose}
      onSubmit={onSubmit}
      size="wide"
      cardClassName="feature-dialog"
      bodyClassName="feature-dialog-body"
    >
      <input aria-label="Name" data-dialog-initial-focus />
      <p id="shared-form-dialog-error">Name is required.</p>
    </WorkflowFormDialog>
  );

  const dialog = screen.getByRole('dialog', { name: 'Create workflow' });
  expect(dialog).toHaveClass(
    'pipelines-modal-card',
    'workflow-form-dialog',
    'workflow-form-dialog--wide',
    'feature-dialog'
  );
  expect(dialog).toHaveAccessibleDescription('Name is required.');
  expect(screen.getByTestId('dialog-icon')).toBeVisible();
  expect(within(dialog).getByText('Configure the workflow.')).toBeVisible();
  expect(within(dialog).getByLabelText('Name')).toHaveFocus();
  expect(within(dialog).getByText('Name is required.').parentElement).toHaveClass(
    'pipelines-modal-body',
    'feature-dialog-body'
  );

  fireEvent.click(within(dialog).getByRole('button', { name: 'Create' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }));
  expect(onSubmit).toHaveBeenCalledOnce();
  expect(onClose).toHaveBeenCalledOnce();
});

test('supports non-form content, default sizing, and a disabled close action', () => {
  const onClose = vi.fn();

  render(
    <WorkflowFormDialog
      id="read-only-dialog"
      titleId="read-only-dialog-title"
      kicker="Workflow"
      title="Read-only workflow"
      actions={<button type="button">Done</button>}
      onClose={onClose}
      closeDisabled
    >
      <p>Review the workflow.</p>
    </WorkflowFormDialog>
  );

  const dialog = screen.getByRole('dialog', { name: 'Read-only workflow' });
  expect(dialog).not.toHaveClass('workflow-form-dialog--wide');
  expect(within(dialog).getByText('Review the workflow.').parentElement).toHaveClass('space-y-4');
  expect(within(dialog).getByRole('button', { name: 'Close' })).toBeDisabled();
  expect(onClose).not.toHaveBeenCalled();
});
