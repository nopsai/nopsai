import { ListPlus, Plus } from 'lucide-react';
import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import { WorkflowDialogFrame, WorkflowInlineAlert } from '../../components/WorkflowPrimitives';
import type {
  TriggerCloneModalState,
  TriggerCreateModalState,
  TriggerDeleteModalState,
} from './useTriggerManifestMutations';

type TriggerWorkflowModalsProps = {
  createModal: TriggerCreateModalState | null;
  cloneModal: TriggerCloneModalState | null;
  deleteModal: TriggerDeleteModalState | null;
  canDeleteTriggers: boolean;
  selectedSlug?: string;
  onCloseCreate: () => void;
  onUpdateCreateRepository: (repository: string) => void;
  onSubmitCreate: () => void;
  onCloseClone: () => void;
  onUpdateCloneRepository: (repository: string) => void;
  onSubmitClone: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
};

function TriggerRepositoryDialog({
  mode,
  modal,
  selectedSlug,
  onClose,
  onUpdateRepository,
  onSubmit,
}: {
  mode: 'create' | 'clone';
  modal: TriggerCreateModalState | TriggerCloneModalState;
  selectedSlug?: string;
  onClose: () => void;
  onUpdateRepository: (repository: string) => void;
  onSubmit: () => void;
}) {
  const isCreate = mode === 'create';
  const modalId = isCreate ? 'triggers-new-modal' : 'triggers-clone-modal';
  const titleId = `${modalId}-title`;
  const descriptionId = `${modalId}-description`;
  const errorId = `${modalId}-error`;
  const inputId = `${modalId}-repository`;
  const title = isCreate ? 'Create trigger override' : `Clone ${selectedSlug || 'trigger'}`;

  return (
    <WorkflowFormDialog
      id={modalId}
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      onSubmit={event => {
        event.preventDefault();
        onSubmit();
      }}
      closeDisabled={modal.pending}
      kicker={isCreate ? 'New trigger' : 'Clone trigger'}
      title={title}
      headerLeading={(
        <span className="trigger-modal-icon" aria-hidden="true">
          {isCreate ? <Plus className="h-5 w-5" /> : <ListPlus className="h-5 w-5" />}
        </span>
      )}
      cardClassName="trigger-modal-card"
      bodyClassName="trigger-modal-body"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={modal.pending}>
            {modal.pending ? (isCreate ? 'Creating…' : 'Cloning…') : isCreate ? 'Create' : 'Clone'}
          </button>
        </>
      )}
    >
      <div className="trigger-modal-field-team">
        <label htmlFor={inputId} className="block text-sm font-medium text-[var(--text-secondary)]">
          {isCreate ? 'Repository' : 'Target repository'}
        </label>
        <input
          id={inputId}
          type="text"
          className="pipelines-input w-full mt-1"
          placeholder="owner/repo"
          value={modal.repository}
          onChange={event => onUpdateRepository(event.target.value)}
          disabled={modal.pending}
          data-dialog-initial-focus
        />
        <p id={descriptionId} className="trigger-modal-hint">
          {isCreate
            ? 'Creates or replaces a trigger override stored in the database.'
            : 'Copies the YAML from the current trigger into the target override.'}
        </p>
      </div>
      {isCreate && 'yamlPreview' in modal ? (
        <div className="trigger-modal-field-team">
          <p className="block text-sm font-medium text-[var(--text-secondary)]">Template</p>
          <div className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
            <pre className="p-3 text-xs overflow-auto max-h-52">{modal.yamlPreview}</pre>
          </div>
        </div>
      ) : null}
      {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
    </WorkflowFormDialog>
  );
}

function TriggerDeleteDialog({
  modal,
  onClose,
  onConfirm,
}: {
  modal: TriggerDeleteModalState;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const titleId = 'triggers-delete-modal-title';
  const descriptionId = 'triggers-delete-modal-description';
  const errorId = 'triggers-delete-modal-error';

  return (
    <WorkflowDialogFrame
      id="triggers-delete-modal"
      role="alertdialog"
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      className="pipelines-modal-card max-w-md w-full"
    >
        <header className="pipelines-modal-header">
          <div>
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete trigger</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">Remove {modal.slug}?</h3>
          </div>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Close
          </button>
        </header>
        <div className="pipelines-modal-body space-y-3">
          <p id={descriptionId} className="text-sm text-[var(--text-secondary)]">
            {modal.gitOpsManaged
              ? 'This removes the database row. A future GitOps sync can recreate it from the repository.'
              : 'This action cannot be undone.'}
          </p>
          {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
        </div>
        <div className="pipelines-modal-footer">
          <div className="pipelines-modal-actions">
            <button
              type="button"
              className="glass-button-ghost"
              onClick={onClose}
              disabled={modal.pending}
              data-dialog-initial-focus
            >
              Cancel
            </button>
            <button type="button" className="glass-button-danger" onClick={onConfirm} disabled={modal.pending}>
              {modal.pending ? 'Deleting…' : 'Delete'}
            </button>
          </div>
        </div>
    </WorkflowDialogFrame>
  );
}

export function TriggerWorkflowModals({
  createModal,
  cloneModal,
  deleteModal,
  canDeleteTriggers,
  selectedSlug,
  onCloseCreate,
  onUpdateCreateRepository,
  onSubmitCreate,
  onCloseClone,
  onUpdateCloneRepository,
  onSubmitClone,
  onCloseDelete,
  onConfirmDelete,
}: TriggerWorkflowModalsProps) {
  return (
    <>
      {createModal ? (
        <TriggerRepositoryDialog
          mode="create"
          modal={createModal}
          onClose={onCloseCreate}
          onUpdateRepository={onUpdateCreateRepository}
          onSubmit={onSubmitCreate}
        />
      ) : null}
      {cloneModal ? (
        <TriggerRepositoryDialog
          mode="clone"
          modal={cloneModal}
          selectedSlug={selectedSlug}
          onClose={onCloseClone}
          onUpdateRepository={onUpdateCloneRepository}
          onSubmit={onSubmitClone}
        />
      ) : null}
      {canDeleteTriggers && deleteModal ? (
        <TriggerDeleteDialog modal={deleteModal} onClose={onCloseDelete} onConfirm={onConfirmDelete} />
      ) : null}
    </>
  );
}
