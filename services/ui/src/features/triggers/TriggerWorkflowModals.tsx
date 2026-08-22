import type { ReactNode } from 'react';
import { Edit3, ListPlus, Plus } from 'lucide-react';
import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import { WorkflowDialogCloseButton, WorkflowDialogFrame, WorkflowInlineAlert } from '../../components/WorkflowPrimitives';
import { YamlValidationPanel, type YamlValidationError } from '../editor/YamlValidationPanel';
import {
  GLOBAL_RESOURCE_TEAM_PATH,
  compareResourceTeamPathsWithGlobalFirst,
} from '../../lib/resourceTeams';
import {
  TRIGGER_PROVIDERS,
  normalizeTriggerTeamPath,
  triggerDetailsWithProvider,
  triggerTeamLabel,
  triggerWebhookSourceOptionLabel,
  type TriggerDetailsFormState,
  type TriggerProvider,
  type TriggerWebhookSourceOption,
} from './model';
import type {
  TriggerCloneModalState,
  TriggerCreateModalState,
  TriggerDeleteModalState,
} from './useTriggerManifestMutations';

type TriggerWorkflowModalsProps = {
  createModal: TriggerCreateModalState | null;
  editModal: TriggerEditModalState | null;
  cloneModal: TriggerCloneModalState | null;
  deleteModal: TriggerDeleteModalState | null;
  canDeleteTriggers: boolean;
  selectedSlug?: string;
  teamPaths: string[];
  webhookSources: TriggerWebhookSourceOption[];
  onCloseCreate: () => void;
  onUpdateCreateRepository: (repository: string) => void;
  onUpdateCreateDetails: (details: TriggerDetailsFormState) => void;
  onUpdateCreateYamlPreview: (yamlPreview: string) => void;
  onSubmitCreate: () => void;
  onCloseEdit: () => void;
  onUpdateEditDetails: (details: TriggerDetailsFormState) => void;
  onUpdateEditYamlPreview: (yamlPreview: string) => void;
  onSubmitEdit: () => void;
  onCloseClone: () => void;
  onUpdateCloneRepository: (repository: string) => void;
  onUpdateCloneDetails: (details: TriggerDetailsFormState) => void;
  onUpdateCloneYamlPreview: (yamlPreview: string) => void;
  onSubmitClone: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
};

export type TriggerEditModalState = {
  slug: string;
  details: TriggerDetailsFormState;
  yamlPreview: string;
  validationErrors: YamlValidationError[];
  saveBlocked?: boolean;
  pending: boolean;
  gitOpsManaged?: boolean;
};

function TriggerRepositoryDialog({
  mode,
  modal,
  selectedSlug,
  onClose,
  onUpdateRepository,
  onUpdateDetails,
  onUpdateYamlPreview,
  teamPaths,
  webhookSources,
  onSubmit,
}: {
  mode: 'create' | 'clone';
  modal: TriggerCreateModalState | TriggerCloneModalState;
  selectedSlug?: string;
  onClose: () => void;
  onUpdateRepository: (repository: string) => void;
  onUpdateDetails?: (details: TriggerDetailsFormState) => void;
  onUpdateYamlPreview?: (yamlPreview: string) => void;
  teamPaths: string[];
  webhookSources: TriggerWebhookSourceOption[];
  onSubmit: () => void;
}) {
  const isCreate = mode === 'create';
  const modalId = isCreate ? 'triggers-new-modal' : 'triggers-clone-modal';
  const titleId = `${modalId}-title`;
  const descriptionId = `${modalId}-description`;
  const errorId = `${modalId}-error`;
  const inputId = `${modalId}-repository`;
  const title = isCreate ? 'Create trigger override' : `Clone ${selectedSlug || 'trigger'}`;
  const modalDetails = 'details' in modal ? modal.details : null;
  const teamOptions = uniqueTeamOptions(teamPaths || []);

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
      <div className="trigger-modal-field-repository">
        <label htmlFor={inputId} className="modal-property-label">
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
            : 'Copies the current trigger into an editable target override.'}
        </p>
      </div>
      {modalDetails && onUpdateDetails ? (
        <TriggerMetadataFields
          details={modalDetails}
          pending={modal.pending}
          teamPaths={teamOptions}
          webhookSources={webhookSources}
          onUpdate={onUpdateDetails}
        />
      ) : null}
      {'yamlPreview' in modal ? (
        <div className="trigger-modal-field-repository">
          <label className="modal-property-label">
            {isCreate ? 'Template' : 'Definition'}
            <textarea
              className="pipelines-input min-h-52 w-full font-mono text-xs"
              value={modal.yamlPreview}
              onChange={event => onUpdateYamlPreview?.(event.target.value)}
              disabled={modal.pending}
              spellCheck={false}
            />
          </label>
        </div>
      ) : null}
      {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
    </WorkflowFormDialog>
  );
}

function TriggerEditDialog({
  modal,
  teamPaths,
  webhookSources,
  onClose,
  onUpdateDetails,
  onUpdateYamlPreview,
  onSubmit,
}: {
  modal: TriggerEditModalState;
  teamPaths: string[];
  webhookSources: TriggerWebhookSourceOption[];
  onClose: () => void;
  onUpdateDetails: (details: TriggerDetailsFormState) => void;
  onUpdateYamlPreview: (yamlPreview: string) => void;
  onSubmit: () => void;
}) {
  const titleId = 'triggers-edit-modal-title';
  const descriptionId = 'triggers-edit-modal-description';
  const validationId = 'triggers-edit-modal-validation';
  const teamOptions = uniqueTeamOptions(teamPaths || []);

  return (
    <WorkflowFormDialog
      id="triggers-edit-modal"
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.validationErrors.length ? ` ${validationId}` : ''}`}
      onClose={onClose}
      onSubmit={event => {
        event.preventDefault();
        onSubmit();
      }}
      closeDisabled={modal.pending}
      size="wide"
      kicker="Edit trigger"
      title={modal.slug}
      subtitle="Changes are saved as a NopsAI trigger override."
      headerLeading={(
        <span className="trigger-modal-icon" aria-hidden="true">
          <Edit3 className="h-5 w-5" />
        </span>
      )}
      cardClassName="trigger-modal-card"
      bodyClassName="trigger-modal-body"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={modal.pending || Boolean(modal.saveBlocked)}>
            {modal.pending ? 'Saving...' : 'Save trigger'}
          </button>
        </>
      )}
    >
      {modal.gitOpsManaged ? (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
          Saving here creates a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
        </div>
      ) : null}
      <p id={descriptionId} className="trigger-modal-hint">
        Select the NopsAI team and Git ingress, then adjust the trigger rules.
      </p>
      <TriggerMetadataFields
        details={modal.details}
        pending={modal.pending}
        teamPaths={teamOptions}
        webhookSources={webhookSources}
        onUpdate={onUpdateDetails}
      />
      <Field label="Definition">
        <textarea
          className="pipelines-input min-h-72 w-full font-mono text-xs"
          value={modal.yamlPreview}
          onChange={event => onUpdateYamlPreview(event.target.value)}
          disabled={modal.pending}
          spellCheck={false}
          data-dialog-initial-focus
        />
      </Field>
      <YamlValidationPanel id={validationId} errors={modal.validationErrors} />
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
      className="pipelines-modal-card workflow-dialog--compact w-full"
    >
        <header className="pipelines-modal-header">
          <div>
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete trigger</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">Remove {modal.slug}?</h3>
          </div>
          <WorkflowDialogCloseButton onClose={onClose} disabled={modal.pending} />
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
  editModal,
  cloneModal,
  deleteModal,
  canDeleteTriggers,
  selectedSlug,
  teamPaths,
  webhookSources,
  onCloseCreate,
  onUpdateCreateRepository,
  onUpdateCreateDetails,
  onUpdateCreateYamlPreview,
  onSubmitCreate,
  onCloseEdit,
  onUpdateEditDetails,
  onUpdateEditYamlPreview,
  onSubmitEdit,
  onCloseClone,
  onUpdateCloneRepository,
  onUpdateCloneDetails,
  onUpdateCloneYamlPreview,
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
          onUpdateDetails={onUpdateCreateDetails}
          onUpdateYamlPreview={onUpdateCreateYamlPreview}
          teamPaths={teamPaths}
          webhookSources={webhookSources}
          onSubmit={onSubmitCreate}
        />
      ) : null}
      {editModal ? (
        <TriggerEditDialog
          modal={editModal}
          teamPaths={teamPaths}
          webhookSources={webhookSources}
          onClose={onCloseEdit}
          onUpdateDetails={onUpdateEditDetails}
          onUpdateYamlPreview={onUpdateEditYamlPreview}
          onSubmit={onSubmitEdit}
        />
      ) : null}
      {cloneModal ? (
        <TriggerRepositoryDialog
          mode="clone"
          modal={cloneModal}
          selectedSlug={selectedSlug}
          onClose={onCloseClone}
          onUpdateRepository={onUpdateCloneRepository}
          onUpdateDetails={onUpdateCloneDetails}
          onUpdateYamlPreview={onUpdateCloneYamlPreview}
          teamPaths={teamPaths}
          webhookSources={webhookSources}
          onSubmit={onSubmitClone}
        />
      ) : null}
      {canDeleteTriggers && deleteModal ? (
        <TriggerDeleteDialog modal={deleteModal} onClose={onCloseDelete} onConfirm={onConfirmDelete} />
      ) : null}
    </>
  );
}

function TriggerMetadataFields({
  details,
  pending,
  teamPaths,
  webhookSources,
  onUpdate,
}: {
  details: TriggerDetailsFormState;
  pending: boolean;
  teamPaths: string[];
  webhookSources: TriggerWebhookSourceOption[];
  onUpdate: (details: TriggerDetailsFormState) => void;
}) {
  const compatibleWebhookSources = webhookSources.filter(source => source.provider === details.provider);

  return (
    <div className="trigger-modal-details-grid">
      <Field label="Provider">
        <select
          className="pipelines-input w-full"
          value={details.provider}
          onChange={event => onUpdate(triggerDetailsWithProvider(details, event.target.value as TriggerProvider))}
          disabled={pending}
        >
          {TRIGGER_PROVIDERS.map(provider => (
            <option key={provider} value={provider}>{provider}</option>
          ))}
        </select>
      </Field>
      <Field label="Team">
        <select
          className="pipelines-input w-full"
          value={details.teamPath}
          onChange={event => onUpdate({ ...details, teamPath: event.target.value })}
          disabled={pending}
        >
          {teamPaths.map(path => (
            <option key={path} value={path}>{triggerTeamLabel(path)}</option>
          ))}
        </select>
      </Field>
      <Field label="Webhook source">
        <select
          className="pipelines-input w-full font-mono"
          value={details.webhookSourceID}
          onChange={event => onUpdate({ ...details, webhookSourceID: event.target.value })}
          disabled={pending || details.provider === 'github'}
          required={details.provider !== 'github'}
        >
          {details.provider === 'github' ? (
            <option value="">GitHub App automatic</option>
          ) : (
            <>
              <option value="">Select webhook source</option>
              {compatibleWebhookSources.map(source => (
                <option key={source.id} value={source.id}>{triggerWebhookSourceOptionLabel(source)}</option>
              ))}
              {details.webhookSourceID && !compatibleWebhookSources.some(source => source.id === details.webhookSourceID) ? (
                <option value={details.webhookSourceID}>{details.webhookSourceID}</option>
              ) : null}
            </>
          )}
        </select>
      </Field>
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label className="block text-sm text-[var(--text-primary)]">
      <span className="font-medium text-[var(--text-secondary)]">{label}</span>
      <span className="mt-1 block">{children}</span>
    </label>
  );
}

function uniqueTeamOptions(paths: string[]): string[] {
  const normalized = paths
    .map(path => String(path || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/'))
    .map(normalizeTriggerTeamPath);
  return Array.from(new Set([GLOBAL_RESOURCE_TEAM_PATH, ...normalized])).sort(compareResourceTeamPathsWithGlobalFirst);
}
