import { useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerWorkflowModals } from './TriggerWorkflowModals';
import type {
  TriggerCloneModalState,
  TriggerCreateModalState,
  TriggerDeleteModalState,
} from './useTriggerManifestMutations';

type OpenDialog = 'create' | 'edit' | 'clone' | 'delete' | null;

function TriggerModalHarness({
  onCreate,
  onEdit,
  onClone,
  onDelete,
}: {
  onCreate: () => void;
  onEdit: () => void;
  onClone: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState<OpenDialog>(null);
  const [createModal, setCreateModal] = useState<TriggerCreateModalState>({
    repository: '',
    details: {
      provider: 'github',
      teamPath: 'root',
      management: 'nopsai',
      webhookSourceID: '',
    },
    yamlPreview: 'triggers:\n  - on: push\n',
    pending: false,
    error: 'Repository is required.',
  });
  const [cloneModal, setCloneModal] = useState<TriggerCloneModalState>({
    repository: 'owner/repo-copy',
    pending: false,
  });
  const deleteModal: TriggerDeleteModalState = {
    slug: 'owner/repo',
    pending: false,
  };

  return (
    <>
      <button type="button" onClick={() => setOpen('create')}>Open create</button>
      <button type="button" onClick={() => setOpen('edit')}>Open edit</button>
      <button type="button" onClick={() => setOpen('clone')}>Open clone</button>
      <button type="button" onClick={() => setOpen('delete')}>Open delete</button>
      <TriggerWorkflowModals
        createModal={open === 'create' ? createModal : null}
        editModal={open === 'edit' ? {
          slug: 'owner/repo',
          details: {
            provider: 'gitlab',
            teamPath: 'team-1',
            management: 'nopsai',
            webhookSourceID: 'corporate-gitlab',
          },
          yamlPreview: 'provider: gitlab\nteam: team-1\nwebhook_source: corporate-gitlab\ntriggers:\n  - on: push\n',
          validationErrors: [],
          pending: false,
        } : null}
        cloneModal={open === 'clone' ? cloneModal : null}
        deleteModal={open === 'delete' ? deleteModal : null}
        canDeleteTriggers
        selectedSlug="owner/repo"
        teamPaths={['root', 'team-1']}
        webhookSources={[
          { id: 'corporate-gitlab', name: 'Corporate GitLab', provider: 'gitlab', teamPath: 'team-1', visibility: 'workspace' },
        ]}
        onCloseCreate={() => setOpen(null)}
        onUpdateCreateRepository={repository =>
          setCreateModal(current => ({ ...current, repository, error: undefined }))
        }
        onUpdateCreateDetails={details =>
          setCreateModal(current => ({ ...current, details, error: undefined }))
        }
        onUpdateCreateYamlPreview={yamlPreview =>
          setCreateModal(current => ({ ...current, yamlPreview, error: undefined }))
        }
        onSubmitCreate={onCreate}
        onCloseEdit={() => setOpen(null)}
        onUpdateEditDetails={details =>
          setCreateModal(current => ({ ...current, details, error: undefined }))
        }
        onUpdateEditYamlPreview={yamlPreview =>
          setCreateModal(current => ({ ...current, yamlPreview, error: undefined }))
        }
        onSubmitEdit={onEdit}
        onCloseClone={() => setOpen(null)}
        onUpdateCloneRepository={repository =>
          setCloneModal(current => ({ ...current, repository, error: undefined }))
        }
        onSubmitClone={onClone}
        onCloseDelete={() => setOpen(null)}
        onConfirmDelete={onDelete}
      />
    </>
  );
}

function renderHarness() {
  const callbacks = {
    onCreate: vi.fn(),
    onEdit: vi.fn(),
    onClone: vi.fn(),
    onDelete: vi.fn(),
  };
  render(<TriggerModalHarness {...callbacks} />);
  return callbacks;
}

test('creates trigger overrides with labelled fields, validation announcements, and keyboard close', async () => {
  const user = userEvent.setup();
  const { onCreate } = renderHarness();
  const opener = screen.getByRole('button', { name: 'Open create' });

  await user.click(opener);
  const dialog = screen.getByRole('dialog', { name: 'Create trigger override' });
  expect(dialog).toHaveClass('pipelines-modal-card', 'workflow-form-dialog');
  expect(dialog.querySelector('.pipelines-modal-header')).not.toBeNull();
  expect(dialog.querySelector('.pipelines-modal-body')).not.toBeNull();
  expect(dialog.querySelector('.pipelines-modal-footer')).not.toBeNull();
  expect(screen.getByLabelText('Repository')).toHaveFocus();
  expect(screen.getByLabelText('Provider')).toBeVisible();
  expect(screen.getByLabelText('Team')).toBeVisible();
  expect(screen.getByLabelText('Webhook source')).toBeDisabled();
  expect(within(dialog).getByRole('alert')).toHaveTextContent('Repository is required.');

  await user.type(screen.getByLabelText('Repository'), 'owner/repo');
  await user.click(within(dialog).getByRole('button', { name: 'Create' }));
  expect(onCreate).toHaveBeenCalledOnce();

  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog', { name: 'Create trigger override' })).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
});

test('edits trigger overrides in a modal form', async () => {
  const user = userEvent.setup();
  const { onEdit } = renderHarness();

  await user.click(screen.getByRole('button', { name: 'Open edit' }));
  const dialog = screen.getByRole('dialog', { name: 'owner/repo' });
  expect(dialog).toHaveClass('workflow-form-dialog');
  expect(screen.getByLabelText('Provider')).toHaveValue('gitlab');
  expect(screen.getByLabelText('Team')).toHaveValue('team-1');
  expect(screen.getByLabelText('Webhook source')).toHaveValue('corporate-gitlab');
  expect((screen.getByLabelText('Definition') as HTMLTextAreaElement).value).toContain('webhook_source: corporate-gitlab');

  await user.click(within(dialog).getByRole('button', { name: 'Save trigger' }));
  expect(onEdit).toHaveBeenCalledOnce();
});

test('supports clone form submission and traps focus inside the dialog', async () => {
  const user = userEvent.setup();
  const { onClone } = renderHarness();

  await user.click(screen.getByRole('button', { name: 'Open clone' }));
  const dialog = screen.getByRole('dialog', { name: 'Clone owner/repo' });
  const repository = screen.getByLabelText('Target repository');
  expect(repository).toHaveFocus();

  await user.clear(repository);
  await user.type(repository, 'owner/repo-copy');
  await user.click(within(dialog).getByRole('button', { name: 'Clone' }));
  expect(onClone).toHaveBeenCalledOnce();

  within(dialog).getByRole('button', { name: 'Clone' }).focus();
  await user.tab();
  expect(within(dialog).getByRole('button', { name: 'Close' })).toHaveFocus();
});

test('uses alert-dialog semantics for trigger deletion', async () => {
  const user = userEvent.setup();
  const { onDelete } = renderHarness();
  const opener = screen.getByRole('button', { name: 'Open delete' });

  await user.click(opener);
  const dialog = screen.getByRole('alertdialog', { name: 'Remove owner/repo?' });
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
  await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
  expect(onDelete).toHaveBeenCalledOnce();
});
