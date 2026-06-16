import { Plus, X } from 'lucide-react';
import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import type {
  AllowedCaller,
  ExternalTriggerForm,
  ExternalTriggerModalState,
  SelectOption,
} from './model';

type ExternalTriggerFormModalProps = {
  modal: ExternalTriggerModalState;
  form: ExternalTriggerForm;
  formError: string;
  saving: boolean;
  pipelineOptions: string[];
  scopeOptions: string[];
  runGroupOptions: string[];
  callerDraft: AllowedCaller;
  activeCallerOptions: SelectOption[];
  onClose: () => void;
  onSubmit: React.FormEventHandler<HTMLFormElement>;
  onFormChange: (patch: Partial<ExternalTriggerForm>) => void;
  onPipelineChange: (pipeline: string) => void;
  onCallerTypeChange: (type: AllowedCaller['type']) => void;
  onCallerIDChange: (id: string) => void;
  onAddCaller: () => void;
  onRemoveCaller: (index: number) => void;
};

const titleId = 'external-trigger-form-title';
const errorId = 'external-trigger-form-error';

export function ExternalTriggerFormModal({
  modal,
  form,
  formError,
  saving,
  pipelineOptions,
  scopeOptions,
  runGroupOptions,
  callerDraft,
  activeCallerOptions,
  onClose,
  onSubmit,
  onFormChange,
  onPipelineChange,
  onCallerTypeChange,
  onCallerIDChange,
  onAddCaller,
  onRemoveCaller,
}: ExternalTriggerFormModalProps) {
  const isCreate = modal.mode === 'create';

  return (
    <WorkflowFormDialog
      id="external-triggers-edit-modal"
      titleId={titleId}
      descriptionId={formError ? errorId : undefined}
      onClose={onClose}
      onSubmit={onSubmit}
      closeDisabled={saving}
      size="wide"
      kicker={isCreate ? 'Create external trigger' : 'Edit external trigger'}
      title={isCreate ? 'New authenticated endpoint' : form.name || form.id}
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={saving}>
            {saving ? 'Saving...' : isCreate ? 'Create trigger' : 'Save trigger'}
          </button>
        </>
      )}
    >
      {formError ? (
        <div id={errorId} className="dispatcher-error" role="alert">
          {formError}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <label className="flex flex-col gap-1 text-sm">
              <span>Name</span>
              <input
                className="pipelines-input"
                value={form.name}
                onChange={event => onFormChange({ name: event.target.value })}
                required
                data-dialog-initial-focus
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>ID</span>
              <input
                className="pipelines-input"
                value={form.id}
                onChange={event => onFormChange({ id: event.target.value })}
                disabled={!isCreate}
                placeholder="deploy-prod"
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-sm md:col-span-2">
              <span>Description</span>
              <input
                className="pipelines-input"
                value={form.description}
                onChange={event => onFormChange({ description: event.target.value })}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Pipeline</span>
              <select
                className="pipelines-input"
                value={form.pipeline}
                onChange={event => onPipelineChange(event.target.value)}
                required
              >
                <option value="" disabled>Select pipeline</option>
                {pipelineOptions.map(pipeline => (
                  <option key={pipeline} value={pipeline}>{pipeline}</option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Scope</span>
              <select
                className="pipelines-input"
                value={form.scope}
                onChange={event => onFormChange({ scope: event.target.value })}
              >
                {scopeOptions.map(scope => (
                  <option key={scope || '__default__'} value={scope}>{scope || 'default'}</option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Run group</span>
              <select
                className="pipelines-input"
                value={form.runGroupPath}
                onChange={event => onFormChange({ runGroupPath: event.target.value })}
              >
                {runGroupOptions.map(group => (
                  <option key={group} value={group}>{group === 'root' ? 'Root' : group}</option>
                ))}
              </select>
            </label>
      </div>

      <section className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">Allowed callers</h3>
              <label className="dispatcher-toggle">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={event => onFormChange({ enabled: event.target.checked })}
                />
                <span className="dispatcher-toggle__control"><span /></span>
                <span className="dispatcher-toggle__label">Enabled</span>
              </label>
            </div>
            <div className="flex flex-wrap gap-2">
              <select
                className="pipelines-input max-w-[180px]"
                value={callerDraft.type}
                onChange={event => onCallerTypeChange(event.target.value as AllowedCaller['type'])}
                aria-label="Caller type"
              >
                <option value="service_account">Service account</option>
                <option value="user">User</option>
                <option value="auth_group">Group</option>
              </select>
              <select
                className="pipelines-input min-w-[220px] flex-1"
                value={callerDraft.id}
                onChange={event => onCallerIDChange(event.target.value)}
                aria-label="Caller"
              >
                <option value="" disabled>
                  {activeCallerOptions.length ? 'Select caller' : 'No callers available'}
                </option>
                {activeCallerOptions.map(option => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
              <button
                type="button"
                className="pipelines-secondary-button"
                onClick={onAddCaller}
                disabled={!callerDraft.id}
              >
                <Plus className="h-4 w-4" />
                Add
              </button>
            </div>
            <div className="flex flex-wrap gap-2">
              {form.allowedCallers.map((caller, index) => (
                <button
                  key={`${caller.type}:${caller.id}`}
                  type="button"
                  className="runner-pill runner-pill--muted"
                  onClick={() => onRemoveCaller(index)}
                >
                  {caller.type}:{caller.id}
                  <X className="h-3 w-3" />
                </button>
              ))}
            </div>
      </section>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <label className="flex flex-col gap-1 text-sm">
              <span>Variable mapping</span>
              <textarea
                className="pipelines-input min-h-[140px] font-mono"
                value={form.variableMappingText}
                onChange={event => onFormChange({ variableMappingText: event.target.value })}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Payload schema</span>
              <textarea
                className="pipelines-input min-h-[140px] font-mono"
                value={form.payloadSchemaText}
                onChange={event => onFormChange({ payloadSchemaText: event.target.value })}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Rate limit per minute</span>
              <input
                className="pipelines-input"
                type="number"
                min="0"
                value={form.rateLimitPerMinute}
                onChange={event => onFormChange({ rateLimitPerMinute: event.target.value })}
              />
            </label>
      </div>
    </WorkflowFormDialog>
  );
}
