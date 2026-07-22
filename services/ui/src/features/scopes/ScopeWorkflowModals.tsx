import { Copy, KeyRound } from 'lucide-react';
import { WorkflowDialogFrame, WorkflowInlineAlert } from '../../components/WorkflowPrimitives';
import { normalizeScopeLabel, parseScopedIdentity } from './model';
import {
  SAMPLE_SCOPE_VARIABLE,
  type GitOpsSecretEncryptModalState,
  type ScopeModalState,
  type ScopedValueDeleteModalState,
  type ScopedValueModalState,
} from './useScopeModalMutations';

export type ScopeSuggestionEntry = {
  scope: string;
  label: string;
  count: number;
  preview: string[];
};

type ScopeWorkflowModalsProps = {
  scopeModal: ScopeModalState | null;
  variableModal: ScopedValueModalState | null;
  secretModal: ScopedValueModalState | null;
  gitOpsEncryptModal: GitOpsSecretEncryptModalState | null;
  deleteModal: ScopedValueDeleteModalState | null;
  canDeleteScopes: boolean;
  knownRepositories: string[];
  variableSuggestionEntries: ScopeSuggestionEntry[];
  secretSuggestionEntries: ScopeSuggestionEntry[];
  onCloseScope: () => void;
  onUpdateScopeName: (name: string) => void;
  onSubmitScope: () => void;
  onCloseVariable: () => void;
  onUpdateVariable: (patch: Partial<ScopedValueModalState>) => void;
  onChooseVariableSuggestion: (fullName: string) => void;
  onSubmitVariable: () => void;
  onCloseSecret: () => void;
  onUpdateSecret: (patch: Partial<ScopedValueModalState>) => void;
  onChooseSecretSuggestion: (fullName: string) => void;
  onSubmitSecret: () => void;
  onCloseGitOpsEncrypt: () => void;
  onUpdateGitOpsEncryptValue: (value: string) => void;
  onEncryptGitOpsSecret: () => void;
  onCopyGitOpsEncryptedValue: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
};

function formatScopeDisplay(scopeLabel: string): string {
  const normalized = normalizeScopeLabel(scopeLabel);
  return normalized ? `/${normalized}` : '/';
}

function ScopeCreateDialog({
  modal,
  onClose,
  onUpdateName,
  onSubmit,
}: {
  modal: ScopeModalState;
  onClose: () => void;
  onUpdateName: (name: string) => void;
  onSubmit: () => void;
}) {
  const titleId = 'scope-new-modal-title';
  const descriptionId = 'scope-new-modal-description';
  const errorId = 'scope-new-modal-error';

  return (
    <WorkflowDialogFrame
      id="scope-new-modal"
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      className="pipelines-modal-card max-w-md w-full"
    >
        <header className="pipelines-modal-header">
          <div>
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Create scope</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">New Scope</h3>
            <p id={descriptionId} className="text-sm text-[var(--text-secondary)] mt-1">
              Parent: {formatScopeDisplay(modal.parent)}
            </p>
          </div>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Close
          </button>
        </header>
        <form
          className="pipelines-modal-body space-y-4"
          onSubmit={event => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <div>
            <label htmlFor="scope-new-name" className="block text-sm font-medium text-[var(--text-secondary)]">
              Scope Name
            </label>
            <input
              id="scope-new-name"
              type="text"
              className="pipelines-input w-full mt-1"
              placeholder="e.g. dev"
              value={modal.name}
              onChange={event => onUpdateName(event.target.value)}
              disabled={modal.pending}
              data-dialog-initial-focus
            />
            <p className="text-xs text-[var(--text-secondary)] mt-1">
              Only letters, numbers, dots, underscores, and hyphens are allowed. Use slashes for nested teams.
            </p>
          </div>
          <div className="space-y-2 bg-[var(--bg-tertiary)] rounded-md p-3 text-xs text-[var(--text-secondary)]">
            <p className="font-medium text-[var(--text-primary)]">Sample Variable</p>
            <p>
              Each new scope is seeded with <code>{SAMPLE_SCOPE_VARIABLE}</code>. Update or remove it after creating the scope.
            </p>
          </div>
          {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
          <div className="flex items-center justify-end gap-2 pt-2">
            <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
              Cancel
            </button>
            <button type="submit" className="glass-button-primary" disabled={modal.pending}>
              {modal.pending ? 'Creating…' : 'Create Scope'}
            </button>
          </div>
        </form>
    </WorkflowDialogFrame>
  );
}

function ScopeSuggestions({
  kind,
  modal,
  entries,
  onChoose,
}: {
  kind: 'variable' | 'secret';
  modal: ScopedValueModalState;
  entries: ScopeSuggestionEntry[];
  onChoose: (fullName: string) => void;
}) {
  const plural = kind === 'variable' ? 'variables' : 'secrets';
  return (
    <section
      className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 space-y-3"
      aria-label={`Existing ${plural}`}
    >
      <div className="flex items-center justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.18em] text-[var(--text-secondary)]">Suggestions</p>
          <p className="text-sm font-medium text-[var(--text-primary)]">Existing {plural}</p>
        </div>
        <span className="text-xs text-[var(--text-secondary)]">{entries.length} scopes</span>
      </div>
      <div className="scope-suggestion-body">
        {entries.length ? (
          <div className="scope-suggestion-list">
            {entries.map(entry => {
              const remaining = entry.count - entry.preview.length;
              return (
                <article
                  key={`${kind}-suggestion-${entry.scope}`}
                  className={`scope-suggestion-item${entry.scope === modal.scope ? ' scope-suggestion-item--active' : ''}`}
                >
                  <div className="scope-suggestion-scope">
                    <span className="scope-suggestion-scope-label">{entry.label}</span>
                    <span className="scope-suggestion-scope-count">
                      {entry.count} {entry.count === 1 ? kind : plural}
                    </span>
                  </div>
                  <div className="scope-suggestion-variables">
                    {entry.preview.map(name => {
                      const identity = parseScopedIdentity(name);
                      const display = identity.repoSlug ? `${identity.repoSlug}/${identity.name}` : identity.name;
                      return (
                        <button
                          key={`${kind}-suggestion-pill-${entry.scope}-${name}`}
                          type="button"
                          className="scope-suggestion-pill scope-suggestion-pill--action"
                          onClick={() => onChoose(name)}
                          disabled={modal.mode !== 'create' || modal.pending}
                        >
                          {display}
                        </button>
                      );
                    })}
                    {remaining > 0 ? (
                      <span className="scope-suggestion-pill scope-suggestion-pill--more">+{remaining} more</span>
                    ) : null}
                  </div>
                </article>
              );
            })}
          </div>
        ) : (
          <p className="scope-suggestion-empty">No {plural} have been defined yet.</p>
        )}
      </div>
    </section>
  );
}

function ScopedValueDialog({
  kind,
  modal,
  knownRepositories,
  suggestions,
  onClose,
  onUpdate,
  onChooseSuggestion,
  onSubmit,
}: {
  kind: 'variable' | 'secret';
  modal: ScopedValueModalState;
  knownRepositories: string[];
  suggestions: ScopeSuggestionEntry[];
  onClose: () => void;
  onUpdate: (patch: Partial<ScopedValueModalState>) => void;
  onChooseSuggestion: (fullName: string) => void;
  onSubmit: () => void;
}) {
  const isVariable = kind === 'variable';
  const title = modal.mode === 'update' ? (isVariable ? 'Variable' : 'Secret') : isVariable ? 'New Variable' : 'New Secret';
  const modalId = isVariable ? 'variable-edit-modal' : 'secret-edit-modal';
  const titleId = `${modalId}-title`;
  const descriptionId = `${modalId}-description`;
  const errorId = `${modalId}-error`;
  const nameId = `${modalId}-name`;
  const repositoryId = `${modalId}-repository`;
  const valueId = `${modalId}-value`;
  const repositoryListId = `${modalId}-repository-options`;
  const valueLoading = Boolean(modal.valueLoading);
  const valuePlaceholder = valueLoading
    ? 'Loading current value...'
    : modal.mode === 'update'
      ? isVariable
        ? 'Enter new value'
        : 'Enter new value (leave blank to keep unchanged)'
      : isVariable
        ? 'Provide the value stored for this scope'
        : 'Provide the secret value';
  const valueHint = valueLoading
    ? 'Loading the current value for this variable.'
    : isVariable
      ? 'Overwrites any existing value for this scope.'
      : 'Encrypted at rest; never shown in plain text.';
  const submitLabel = modal.pending
    ? 'Saving...'
    : valueLoading
      ? 'Loading...'
      : modal.mode === 'update'
        ? 'Save Value'
        : `Create ${isVariable ? 'Variable' : 'Secret'}`;

  return (
    <WorkflowDialogFrame
      id={modalId}
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      className={`pipelines-modal-card ${isVariable ? 'max-w-10xl rounded-2xl' : 'max-w-6xl rounded-xl'} w-full overflow-hidden border border-[var(--border-primary)] shadow-2xl`}
    >
        <header className="flex items-start justify-between gap-3 px-6 py-4 border-b border-[var(--border-primary)] bg-[var(--bg-secondary)]">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="px-2 py-0.5 rounded-full text-[11px] uppercase tracking-[0.18em] bg-[var(--bg-tertiary)] text-[var(--text-secondary)]">
                {modal.mode === 'update' ? 'Update' : 'Create'}
              </span>
              <span className="text-xs text-[var(--text-secondary)]">{formatScopeDisplay(modal.scope)}</span>
            </div>
            <h3 id={titleId} className="text-xl font-semibold text-[var(--text-primary)]">{title}</h3>
            <p id={descriptionId} className="text-sm text-[var(--text-secondary)]">
              {isVariable ? 'Plain text value; best for non-sensitive config.' : 'Encrypted value; use for sensitive credentials.'}
            </p>
            {modal.mode === 'update' && modal.gitOpsManaged ? (
              <p className="mt-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">
                Saving here creates a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
              </p>
            ) : null}
          </div>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Close
          </button>
        </header>
        <div className="grid gap-4 md:grid-cols-[1.6fr_1fr] p-6 bg-[var(--bg-primary)]">
          <form
            className="space-y-4"
            onSubmit={event => {
              event.preventDefault();
              onSubmit();
            }}
          >
            <div className="space-y-1">
              <label htmlFor={nameId} className="block text-sm font-medium text-[var(--text-secondary)]">
                {isVariable ? 'Variable' : 'Secret'} Name
              </label>
              <input
                id={nameId}
                type="text"
                className="pipelines-input w-full"
                placeholder={isVariable ? 'DATABASE_URL' : 'API_KEY'}
                value={modal.name}
                onChange={event => onUpdate({ name: event.target.value })}
                readOnly={modal.mode === 'update'}
                aria-readonly={modal.mode === 'update'}
                disabled={modal.pending}
                data-dialog-initial-focus={modal.mode === 'create' ? true : undefined}
              />
              {!isVariable ? (
                <p className="text-xs text-[var(--text-secondary)]">Name the secret; include repo prefix if scoped.</p>
              ) : null}
            </div>
            <div className="space-y-1">
              <label htmlFor={repositoryId} className="block text-sm font-medium text-[var(--text-secondary)]">
                Repository (optional)
              </label>
              <input
                id={repositoryId}
                type="text"
                className="pipelines-input w-full"
                placeholder="owner/repository"
                list={repositoryListId}
                value={modal.repository}
                onChange={event => onUpdate({ repository: event.target.value })}
                disabled={modal.pending || modal.mode === 'update'}
                aria-disabled={modal.mode === 'update'}
              />
              <datalist id={repositoryListId}>
                {knownRepositories.map(repository => (
                  <option key={`${kind}-repo-${repository}`} value={repository} />
                ))}
              </datalist>
              <p className="text-xs text-[var(--text-secondary)]">
                {isVariable ? 'Link a repo to scope the variable.' : 'Leave blank for global; add repo for scoped secret.'}
              </p>
            </div>
            <div className="space-y-1">
              <label htmlFor={valueId} className="block text-sm font-medium text-[var(--text-secondary)]">Value</label>
              <textarea
                id={valueId}
                rows={4}
                className="pipelines-input w-full"
                placeholder={valuePlaceholder}
                value={modal.value}
                onChange={event => onUpdate({ value: event.target.value })}
                disabled={modal.pending || valueLoading}
                data-dialog-initial-focus={modal.mode === 'update' ? true : undefined}
              />
              <p className="text-xs text-[var(--text-secondary)]">{valueHint}</p>
            </div>
            {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
            <div className="flex items-center justify-end gap-2 pt-1">
              <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
                Cancel
              </button>
              <button type="submit" className="glass-button-primary" disabled={modal.pending || valueLoading}>
                {submitLabel}
              </button>
            </div>
          </form>
          <ScopeSuggestions
            kind={kind}
            modal={modal}
            entries={suggestions}
            onChoose={onChooseSuggestion}
          />
        </div>
    </WorkflowDialogFrame>
  );
}

function GitOpsEncryptDialog({
  modal,
  onClose,
  onUpdateValue,
  onEncrypt,
  onCopy,
}: {
  modal: GitOpsSecretEncryptModalState;
  onClose: () => void;
  onUpdateValue: (value: string) => void;
  onEncrypt: () => void;
  onCopy: () => void;
}) {
  const titleId = 'gitops-secret-encrypt-modal-title';
  const descriptionId = 'gitops-secret-encrypt-modal-description';
  const errorId = 'gitops-secret-encrypt-modal-error';

  return (
    <WorkflowDialogFrame
      id="gitops-secret-encrypt-modal"
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      className="pipelines-modal-card max-w-3xl w-full overflow-hidden rounded-xl border border-[var(--border-primary)] shadow-2xl"
    >
        <header className="flex items-start justify-between gap-3 px-6 py-4 border-b border-[var(--border-primary)] bg-[var(--bg-secondary)]">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <KeyRound className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
              <span className="text-xs uppercase tracking-[0.18em] text-[var(--text-secondary)]">GitOps</span>
            </div>
            <h3 id={titleId} className="text-xl font-semibold text-[var(--text-primary)]">Secret Encryption</h3>
            <p id={descriptionId} className="text-sm text-[var(--text-secondary)]">
              Encrypt a secret value for a Git-managed scope file.
            </p>
          </div>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Close
          </button>
        </header>
        <form
          className="space-y-4 p-6 bg-[var(--bg-primary)]"
          onSubmit={event => {
            event.preventDefault();
            onEncrypt();
          }}
        >
          <div className="space-y-1">
            <label htmlFor="gitops-secret-value" className="block text-sm font-medium text-[var(--text-secondary)]">
              Value
            </label>
            <textarea
              id="gitops-secret-value"
              rows={4}
              className="pipelines-input w-full"
              value={modal.value}
              onChange={event => onUpdateValue(event.target.value)}
              disabled={modal.pending}
              data-dialog-initial-focus
            />
          </div>
          {modal.encryptedValue ? (
            <div className="space-y-1">
              <label htmlFor="gitops-secret-encrypted-value" className="block text-sm font-medium text-[var(--text-secondary)]">
                Encrypted Value
              </label>
              <textarea
                id="gitops-secret-encrypted-value"
                rows={4}
                className="pipelines-input w-full font-mono text-xs"
                value={modal.encryptedValue}
                readOnly
              />
            </div>
          ) : null}
          {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
          <div className="flex items-center justify-end gap-2 pt-1">
            {modal.encryptedValue ? (
              <button type="button" className="glass-button-ghost inline-flex items-center gap-2" onClick={onCopy} disabled={modal.pending}>
                <Copy className="h-4 w-4" aria-hidden="true" />
                Copy
              </button>
            ) : null}
            <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
              Cancel
            </button>
            <button type="submit" className="glass-button-primary" disabled={modal.pending || !modal.value}>
              {modal.pending ? 'Encrypting...' : 'Encrypt'}
            </button>
          </div>
        </form>
    </WorkflowDialogFrame>
  );
}

function ScopedValueDeleteDialog({
  modal,
  onClose,
  onConfirm,
}: {
  modal: ScopedValueDeleteModalState;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const modalId = modal.kind === 'variable' ? 'variable-delete-modal' : 'secret-delete-modal';
  const titleId = `${modalId}-title`;
  const descriptionId = `${modalId}-description`;
  const errorId = `${modalId}-error`;

  return (
    <WorkflowDialogFrame
      id={modalId}
      role="alertdialog"
      titleId={titleId}
      descriptionId={`${descriptionId}${modal.error ? ` ${errorId}` : ''}`}
      onClose={onClose}
      className="pipelines-modal-card max-w-md w-full"
    >
        <header className="pipelines-modal-header">
          <div>
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete {modal.kind}</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">Confirm removal</h3>
          </div>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={modal.pending}>
            Close
          </button>
        </header>
        <div className="pipelines-modal-body space-y-4">
          <p id={descriptionId} className="text-sm text-[var(--text-secondary)]">
            Remove <strong>{modal.name}</strong> from <strong>{formatScopeDisplay(modal.scope)}</strong>?
          </p>
          {modal.gitOpsManaged ? (
            <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">
              This removes the database row. The next GitOps sync can recreate it unless it is removed from GitOps.
            </p>
          ) : null}
          {modal.error ? <WorkflowInlineAlert id={errorId}>{modal.error}</WorkflowInlineAlert> : null}
          <div className="flex items-center justify-end gap-2 pt-2">
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

export function ScopeWorkflowModals({
  scopeModal,
  variableModal,
  secretModal,
  gitOpsEncryptModal,
  deleteModal,
  canDeleteScopes,
  knownRepositories,
  variableSuggestionEntries,
  secretSuggestionEntries,
  onCloseScope,
  onUpdateScopeName,
  onSubmitScope,
  onCloseVariable,
  onUpdateVariable,
  onChooseVariableSuggestion,
  onSubmitVariable,
  onCloseSecret,
  onUpdateSecret,
  onChooseSecretSuggestion,
  onSubmitSecret,
  onCloseGitOpsEncrypt,
  onUpdateGitOpsEncryptValue,
  onEncryptGitOpsSecret,
  onCopyGitOpsEncryptedValue,
  onCloseDelete,
  onConfirmDelete,
}: ScopeWorkflowModalsProps) {
  return (
    <>
      {scopeModal ? (
        <ScopeCreateDialog
          modal={scopeModal}
          onClose={onCloseScope}
          onUpdateName={onUpdateScopeName}
          onSubmit={onSubmitScope}
        />
      ) : null}
      {variableModal ? (
        <ScopedValueDialog
          kind="variable"
          modal={variableModal}
          knownRepositories={knownRepositories}
          suggestions={variableSuggestionEntries}
          onClose={onCloseVariable}
          onUpdate={onUpdateVariable}
          onChooseSuggestion={onChooseVariableSuggestion}
          onSubmit={onSubmitVariable}
        />
      ) : null}
      {gitOpsEncryptModal ? (
        <GitOpsEncryptDialog
          modal={gitOpsEncryptModal}
          onClose={onCloseGitOpsEncrypt}
          onUpdateValue={onUpdateGitOpsEncryptValue}
          onEncrypt={onEncryptGitOpsSecret}
          onCopy={onCopyGitOpsEncryptedValue}
        />
      ) : null}
      {secretModal ? (
        <ScopedValueDialog
          kind="secret"
          modal={secretModal}
          knownRepositories={knownRepositories}
          suggestions={secretSuggestionEntries}
          onClose={onCloseSecret}
          onUpdate={onUpdateSecret}
          onChooseSuggestion={onChooseSecretSuggestion}
          onSubmit={onSubmitSecret}
        />
      ) : null}
      {canDeleteScopes && deleteModal ? (
        <ScopedValueDeleteDialog modal={deleteModal} onClose={onCloseDelete} onConfirm={onConfirmDelete} />
      ) : null}
    </>
  );
}
