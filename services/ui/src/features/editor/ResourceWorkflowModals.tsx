import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import { WorkflowDialogCloseButton, WorkflowDialogFrame, WorkflowInlineAlert } from '../../components/WorkflowPrimitives';

export type ResourceFormModal = {
  mode: 'create' | 'clone';
  path: string;
  name: string;
  pending: boolean;
  error?: string;
};

export type ResourceDeleteModal = {
  resourceName: string;
  gitOpsManaged?: boolean;
  pending: boolean;
  error?: string;
};

type ResourceWorkflowModalsProps = {
  resourceLabel: 'pipeline' | 'step';
  formModal: ResourceFormModal | null;
  formModalId: (mode: ResourceFormModal['mode']) => string;
  pathPlaceholder: string;
  namePlaceholder: string;
  deleteModal: ResourceDeleteModal | null;
  deleteModalId: string;
  onChangeForm: (patch: Partial<Pick<ResourceFormModal, 'path' | 'name'>>) => void;
  onCloseForm: () => void;
  onSubmitForm: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
};

function ResourceFormDialog({
  resourceLabel,
  formModal,
  modalId,
  pathPlaceholder,
  namePlaceholder,
  onChangeForm,
  onClose,
  onSubmit,
}: {
  resourceLabel: ResourceWorkflowModalsProps['resourceLabel'];
  formModal: ResourceFormModal;
  modalId: string;
  pathPlaceholder: string;
  namePlaceholder: string;
  onChangeForm: ResourceWorkflowModalsProps['onChangeForm'];
  onClose: () => void;
  onSubmit: () => void;
}) {
  const resourceTitle = resourceLabel.charAt(0).toUpperCase() + resourceLabel.slice(1);
  const pathInputId = `${resourceLabel}-workflow-path`;
  const nameInputId = `${resourceLabel}-workflow-name`;
  const titleId = `${modalId}-title`;
  const errorId = `${modalId}-error`;

  return (
    <WorkflowFormDialog
      id={modalId}
      titleId={titleId}
      descriptionId={formModal.error ? errorId : undefined}
      onClose={onClose}
      closeDisabled={formModal.pending}
      kicker={formModal.mode === 'create' ? `New ${resourceLabel}` : `Clone ${resourceLabel}`}
      title={formModal.mode === 'create' ? `Create ${resourceLabel}` : `Clone ${resourceLabel}`}
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={formModal.pending}>
            Cancel
          </button>
          <button type="button" className="glass-button-primary" onClick={onSubmit} disabled={formModal.pending}>
            {formModal.pending ? 'Saving…' : formModal.mode === 'create' ? 'Create' : 'Clone'}
          </button>
        </>
      )}
    >
      <div>
        <label htmlFor={pathInputId} className="modal-property-label">
          {resourceTitle} Path
        </label>
        <input
          id={pathInputId}
          type="text"
          className="pipelines-input w-full mt-1"
          placeholder={pathPlaceholder}
          value={formModal.path}
          onChange={event => onChangeForm({ path: event.target.value })}
          data-dialog-initial-focus
        />
        <p className="text-xs text-[var(--text-secondary)] mt-1">Optional team path. Leave blank for root.</p>
      </div>
      <div>
        <label htmlFor={nameInputId} className="modal-property-label">
          {resourceTitle} Name
        </label>
        <input
          id={nameInputId}
          type="text"
          className="pipelines-input w-full mt-1"
          placeholder={namePlaceholder}
          value={formModal.name}
          onChange={event => onChangeForm({ name: event.target.value })}
        />
      </div>
      {formModal.error ? <WorkflowInlineAlert id={errorId}>{formModal.error}</WorkflowInlineAlert> : null}
    </WorkflowFormDialog>
  );
}

function ResourceDeleteDialog({
  resourceLabel,
  deleteModal,
  modalId,
  onClose,
  onConfirm,
}: {
  resourceLabel: ResourceWorkflowModalsProps['resourceLabel'];
  deleteModal: ResourceDeleteModal;
  modalId: string;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const titleId = `${modalId}-title`;
  const descriptionId = `${modalId}-description`;
  const errorId = `${modalId}-error`;

  return (
    <WorkflowDialogFrame
      id={modalId}
      role="alertdialog"
      titleId={titleId}
      descriptionId={`${descriptionId}${deleteModal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      className="pipelines-modal-card workflow-dialog--compact w-full"
    >
        <header className="pipelines-modal-header">
          <div>
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete {resourceLabel}</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">
              Remove {deleteModal.resourceName}?
            </h3>
          </div>
          <WorkflowDialogCloseButton onClose={onClose} disabled={deleteModal.pending} />
        </header>
        <div className="pipelines-modal-body space-y-3">
          <p id={descriptionId} className="text-sm text-[var(--text-secondary)]">
            {deleteModal.gitOpsManaged
              ? 'This removes the database row. A future GitOps sync can recreate it from the repository.'
              : 'This action cannot be undone.'}
          </p>
          {deleteModal.error ? <WorkflowInlineAlert id={errorId}>{deleteModal.error}</WorkflowInlineAlert> : null}
        </div>
        <div className="pipelines-modal-footer">
          <div className="pipelines-modal-actions">
            <button
              type="button"
              className="glass-button-ghost"
              onClick={onClose}
              disabled={deleteModal.pending}
              data-dialog-initial-focus
            >
              Cancel
            </button>
            <button type="button" className="glass-button-danger" onClick={onConfirm} disabled={deleteModal.pending}>
              {deleteModal.pending ? 'Deleting…' : 'Delete'}
            </button>
          </div>
        </div>
    </WorkflowDialogFrame>
  );
}

export function ResourceWorkflowModals({
  resourceLabel,
  formModal,
  formModalId,
  pathPlaceholder,
  namePlaceholder,
  deleteModal,
  deleteModalId,
  onChangeForm,
  onCloseForm,
  onSubmitForm,
  onCloseDelete,
  onConfirmDelete,
}: ResourceWorkflowModalsProps) {
  return (
    <>
      {formModal ? (
        <ResourceFormDialog
          resourceLabel={resourceLabel}
          formModal={formModal}
          modalId={formModalId(formModal.mode)}
          pathPlaceholder={pathPlaceholder}
          namePlaceholder={namePlaceholder}
          onChangeForm={onChangeForm}
          onClose={onCloseForm}
          onSubmit={onSubmitForm}
        />
      ) : null}

      {deleteModal ? (
        <ResourceDeleteDialog
          resourceLabel={resourceLabel}
          deleteModal={deleteModal}
          modalId={deleteModalId}
          onClose={onCloseDelete}
          onConfirm={onConfirmDelete}
        />
      ) : null}
    </>
  );
}
