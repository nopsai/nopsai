import { useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ResourceWorkflowModals, type ResourceFormModal } from './ResourceWorkflowModals';

function ModalHarness({ onSubmit }: { onSubmit: () => void }) {
  const [formModal, setFormModal] = useState<ResourceFormModal | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <>
      <button
        type="button"
        onClick={() => setFormModal({ mode: 'create', path: '', name: '', pending: false })}
      >
        Open create
      </button>
      <button type="button" onClick={() => setDeleteOpen(true)}>Open delete</button>
      <ResourceWorkflowModals
        resourceLabel="pipeline"
        formModal={formModal}
        formModalId={() => 'pipelines-new-modal'}
        pathPlaceholder="team/service"
        namePlaceholder="build-and-test"
        deleteModal={deleteOpen ? { resourceName: 'platform/release', pending: false } : null}
        deleteModalId="pipelines-delete-modal"
        onChangeForm={patch => setFormModal(current => (current ? { ...current, ...patch } : current))}
        onCloseForm={() => setFormModal(null)}
        onSubmitForm={onSubmit}
        onCloseDelete={() => setDeleteOpen(false)}
        onConfirmDelete={() => undefined}
      />
    </>
  );
}

test('edits, traps focus, and restores focus for a shared resource workflow form', async () => {
  const onSubmit = vi.fn();
  const user = userEvent.setup();
  render(<ModalHarness onSubmit={onSubmit} />);

  const opener = screen.getByRole('button', { name: 'Open create' });
  await user.click(opener);
  const dialog = screen.getByRole('dialog', { name: 'Create pipeline' });
  expect(dialog).toHaveClass('pipelines-modal-card', 'workflow-form-dialog');
  expect(dialog.querySelector('.pipelines-modal-header')).not.toBeNull();
  expect(dialog.querySelector('.pipelines-modal-body')).not.toBeNull();
  expect(dialog.querySelector('.pipelines-modal-footer')).not.toBeNull();
  expect(screen.getByLabelText('Pipeline Name')).toHaveFocus();

  await user.type(screen.getByLabelText('Pipeline Path'), 'platform/build');
  await user.type(screen.getByLabelText('Pipeline Name'), 'release');
  expect(screen.getByLabelText('Pipeline Path')).toHaveValue('platform/build');
  expect(screen.getByLabelText('Pipeline Name')).toHaveValue('release');

  await user.click(screen.getByRole('button', { name: 'Create' }));
  expect(onSubmit).toHaveBeenCalledOnce();

  within(dialog).getByRole('button', { name: 'Create' }).focus();
  await user.tab();
  expect(within(dialog).getByRole('button', { name: 'Close' })).toHaveFocus();

  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog', { name: 'Create pipeline' })).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
});

test('uses alert-dialog semantics for destructive confirmation', async () => {
  const user = userEvent.setup();
  render(<ModalHarness onSubmit={() => undefined} />);

  const opener = screen.getByRole('button', { name: 'Open delete' });
  await user.click(opener);
  expect(screen.getByRole('alertdialog', { name: 'Remove platform/release?' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus();

  await user.keyboard('{Escape}');
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
});
