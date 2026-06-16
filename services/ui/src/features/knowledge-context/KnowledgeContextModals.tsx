import { WorkflowDialogFrame, WorkflowInlineAlert } from '../../components/WorkflowPrimitives';
import { kindOrder } from './model';

export type KnowledgeFormModalState = {
  mode: 'create' | 'clone';
  kind: string;
  group: string;
  name: string;
  content: string;
  pending: boolean;
  error?: string;
};

export type KnowledgeDeleteModalState = {
  id: string;
  name: string;
  gitOpsManaged?: boolean;
  pending: boolean;
  error?: string;
};

type KnowledgeContextModalsProps = {
  formModal: KnowledgeFormModalState | null;
  deleteModal: KnowledgeDeleteModalState | null;
  onCloseForm: () => void;
  onUpdateForm: (patch: Partial<KnowledgeFormModalState>) => void;
  onSubmitForm: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
};

export function KnowledgeContextModals({
  formModal,
  deleteModal,
  onCloseForm,
  onUpdateForm,
  onSubmitForm,
  onCloseDelete,
  onConfirmDelete,
}: KnowledgeContextModalsProps) {
  const formModalId = formModal?.mode === 'clone' ? 'knowledge-context-clone-modal' : 'knowledge-context-new-modal';
  const formTitleId = `${formModalId}-title`;
  const formErrorId = `${formModalId}-error`;
  const deleteModalId = 'knowledge-context-delete-modal';
  const deleteTitleId = `${deleteModalId}-title`;
  const deleteDescriptionId = `${deleteModalId}-description`;
  const deleteErrorId = `${deleteModalId}-error`;

  return (
    <>
      {formModal ? (
        <WorkflowDialogFrame
          id={formModalId}
          titleId={formTitleId}
          descriptionId={formModal.error ? formErrorId : undefined}
          onClose={onCloseForm}
          className="pipelines-modal-card max-w-lg w-full"
        >
          <header className="pipelines-modal-header">
            <div>
              <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">
                {formModal.mode === 'create' ? 'New knowledge context' : 'Clone knowledge context'}
              </p>
              <h3 id={formTitleId} className="text-lg font-semibold text-[var(--text-primary)]">
                {formModal.mode === 'create' ? 'Create document' : 'Clone document'}
              </h3>
            </div>
            <button type="button" className="glass-button-ghost" onClick={onCloseForm} disabled={formModal.pending}>
              Close
            </button>
          </header>
          <div className="pipelines-modal-body space-y-4">
            <div className="grid gap-3 sm:grid-cols-[180px_1fr]">
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Kind
                <select className="pipelines-input w-full mt-1" value={formModal.kind} onChange={event => onUpdateForm({ kind: event.target.value })}>
                  {kindOrder.map(kind => (
                    <option key={kind} value={kind}>{kind}</option>
                  ))}
                </select>
              </label>
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Group
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="team-1"
                  value={formModal.group}
                  onChange={event => onUpdateForm({ group: event.target.value })}
                />
              </label>
            </div>
            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Name
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="repo-check"
                  value={formModal.name}
                  onChange={event => onUpdateForm({ name: event.target.value })}
                  data-dialog-initial-focus
                />
              </label>
            </div>
            {formModal.error ? <WorkflowInlineAlert id={formErrorId}>{formModal.error}</WorkflowInlineAlert> : null}
          </div>
          <div className="pipelines-modal-footer">
            <div className="pipelines-modal-actions">
              <button type="button" className="glass-button-ghost" onClick={onCloseForm} disabled={formModal.pending}>
                Cancel
              </button>
              <button type="button" className="glass-button-primary" onClick={onSubmitForm} disabled={formModal.pending}>
                {formModal.mode === 'clone' ? 'Clone' : 'Create'}
              </button>
            </div>
          </div>
        </WorkflowDialogFrame>
      ) : null}

      {deleteModal ? (
        <WorkflowDialogFrame
          id={deleteModalId}
          role="alertdialog"
          titleId={deleteTitleId}
          descriptionId={`${deleteDescriptionId}${deleteModal.error ? ` ${deleteErrorId}` : ''}`}
          onClose={onCloseDelete}
          className="pipelines-modal-card max-w-md w-full"
        >
          <header className="pipelines-modal-header">
            <div>
              <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete knowledge context</p>
              <h3 id={deleteTitleId} className="text-lg font-semibold text-[var(--text-primary)]">Remove {deleteModal.name}?</h3>
            </div>
            <button type="button" className="glass-button-ghost" onClick={onCloseDelete} disabled={deleteModal.pending}>
              Close
            </button>
          </header>
          <div className="pipelines-modal-body space-y-3">
            <p id={deleteDescriptionId} className="text-sm text-[var(--text-secondary)]">
              {deleteModal.gitOpsManaged
                ? 'This removes the database row. A future GitOps sync can recreate it from the repository.'
                : 'This action cannot be undone.'}
            </p>
            {deleteModal.error ? <WorkflowInlineAlert id={deleteErrorId}>{deleteModal.error}</WorkflowInlineAlert> : null}
          </div>
          <div className="pipelines-modal-footer">
            <div className="pipelines-modal-actions">
              <button
                type="button"
                className="glass-button-ghost"
                onClick={onCloseDelete}
                disabled={deleteModal.pending}
                data-dialog-initial-focus
              >
                Cancel
              </button>
              <button type="button" className="glass-button-danger" onClick={onConfirmDelete} disabled={deleteModal.pending}>
                {deleteModal.pending ? 'Deleting...' : 'Delete'}
              </button>
            </div>
          </div>
        </WorkflowDialogFrame>
      ) : null}
    </>
  );
}
