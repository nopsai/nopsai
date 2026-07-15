import { WorkflowDialogFrame, WorkflowInlineAlert } from '../../components/WorkflowPrimitives';
import {
  kindOrder,
  knowledgeConnectionDisplayName,
  knowledgeConnectionIdentifier,
  knowledgeConnectionProviders,
  knowledgeFailureModeOptions,
  knowledgeSyncModeOptions,
  normalizeTeamPath,
  type KnowledgeExternalPagePreview,
  type KnowledgeExternalPageSummary,
  type KnowledgeConnectionDraft,
  type KnowledgeConnectionListItem,
  type KnowledgeContentSource,
  type KnowledgeFailureMode,
  type KnowledgeSyncMode,
} from './model';

export type KnowledgeFormModalState = {
  mode: 'create' | 'clone';
  contentSource: KnowledgeContentSource;
  kind: string;
  team: string;
  name: string;
  description?: string;
  connection_id?: string;
  external_page_id?: string;
  external_page_url?: string;
  sync_mode?: KnowledgeSyncMode;
  failure_mode?: KnowledgeFailureMode;
  content: string;
  page_search_query?: string;
  page_search_results?: KnowledgeExternalPageSummary[];
  page_search_cursor?: string;
  page_search_loading?: boolean;
  page_resolving?: boolean;
  page_preview?: KnowledgeExternalPagePreview | null;
  page_error?: string;
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

export type KnowledgeConnectionModalState = KnowledgeConnectionDraft & {
  mode?: 'create' | 'edit';
  id?: string;
  pending: boolean;
  error?: string;
};

type KnowledgeContextModalsProps = {
  formModal: KnowledgeFormModalState | null;
  deleteModal: KnowledgeDeleteModalState | null;
  connectionModal: KnowledgeConnectionModalState | null;
  connections?: KnowledgeConnectionListItem[];
  onCloseForm: () => void;
  onUpdateForm: (patch: Partial<KnowledgeFormModalState>) => void;
  onSubmitForm: () => void;
  onSearchPages?: () => void;
  onLoadMorePages?: () => void;
  onResolvePage?: () => void;
  onSelectPage?: (page: KnowledgeExternalPageSummary) => void;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
  onCloseConnection: () => void;
  onUpdateConnection: (patch: Partial<KnowledgeConnectionModalState>) => void;
  onSubmitConnection: () => void;
  onAddConnectionFromForm?: (teamPath: string) => void;
};

export function KnowledgeContextModals({
  formModal,
  deleteModal,
  connectionModal,
  connections = [],
  onCloseForm,
  onUpdateForm,
  onSubmitForm,
  onSearchPages,
  onLoadMorePages,
  onResolvePage,
  onSelectPage,
  onCloseDelete,
  onConfirmDelete,
  onCloseConnection,
  onUpdateConnection,
  onSubmitConnection,
  onAddConnectionFromForm,
}: KnowledgeContextModalsProps) {
  const formModalId = formModal?.mode === 'clone' ? 'knowledge-context-clone-modal' : 'knowledge-context-new-modal';
  const formTitleId = `${formModalId}-title`;
  const formErrorId = `${formModalId}-error`;
  const deleteModalId = 'knowledge-context-delete-modal';
  const deleteTitleId = `${deleteModalId}-title`;
  const deleteDescriptionId = `${deleteModalId}-description`;
  const deleteErrorId = `${deleteModalId}-error`;
  const connectionModalId = 'knowledge-connection-modal';
  const connectionTitleId = `${connectionModalId}-title`;
  const connectionErrorId = `${connectionModalId}-error`;
  const teamConnections = formModal
    ? connections.filter(connection => normalizeTeamPath(connection.team) === normalizeTeamPath(formModal.team))
    : [];
  const isEditingConnection = connectionModal?.mode === 'edit';

  return (
    <>
      {formModal ? (
        <WorkflowDialogFrame
          id={formModalId}
          titleId={formTitleId}
          descriptionId={formModal.error ? formErrorId : undefined}
          onClose={onCloseForm}
          className="pipelines-modal-card kc-document-modal w-full"
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
            {formModal.mode === 'create' ? (
              <fieldset className="kc-source-choice">
                <legend>Content source</legend>
                <label>
                  <input
                    type="radio"
                    name="knowledge-content-source"
                    value="inline"
                    checked={formModal.contentSource === 'inline'}
                    onChange={() => onUpdateForm({ contentSource: 'inline' })}
                  />
                  <span>Inline content</span>
                </label>
                <label>
                  <input
                    type="radio"
                    name="knowledge-content-source"
                    value="external"
                    checked={formModal.contentSource === 'external'}
                    onChange={() => onUpdateForm({ contentSource: 'external' })}
                  />
                  <span>External page</span>
                </label>
              </fieldset>
            ) : null}
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
                Team
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="team-1"
                  value={formModal.team}
                  onChange={event => onUpdateForm({ team: event.target.value })}
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
            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Description
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="Optional summary"
                  value={formModal.description || ''}
                  onChange={event => onUpdateForm({ description: event.target.value })}
                />
              </label>
            </div>
            {formModal.contentSource === 'external' ? (
              <div className="kc-external-fields">
                <section className="kc-external-panel kc-external-panel--source" aria-label="Provider page">
                  <div className="kc-external-panel-head">
                    <strong>Provider page</strong>
                    <span>Search, paste a URL, and preview before saving.</span>
                  </div>
                  {teamConnections.length ? (
                    <label className="block text-sm font-medium text-[var(--text-secondary)]">
                      Connection
                      <select
                        className="pipelines-input w-full mt-1"
                        value={formModal.connection_id || ''}
                        onChange={event => onUpdateForm({ connection_id: event.target.value })}
                      >
                        <option value="">Choose a connection</option>
                        {teamConnections.map(connection => (
                          <option key={knowledgeConnectionIdentifier(connection)} value={connection.id}>
                            {knowledgeConnectionDisplayName(connection)} ({connection.provider})
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : (
                    <div className="kc-demo-alert">
                      <span>No connection is available for this team.</span>
                      {onAddConnectionFromForm ? (
                        <button type="button" className="kc-demo-table-link" onClick={() => onAddConnectionFromForm(formModal.team)}>
                          Add connection
                        </button>
                      ) : null}
                    </div>
                  )}
                  <div className="kc-page-id-grid">
                    <label className="block text-sm font-medium text-[var(--text-secondary)]">
                      Page URL
                      <input
                        className="pipelines-input w-full mt-1"
                        placeholder="https://workspace.example/wiki/page"
                        value={formModal.external_page_url || ''}
                        onChange={event => onUpdateForm({ external_page_url: event.target.value })}
                      />
                    </label>
                    <label className="block text-sm font-medium text-[var(--text-secondary)]">
                      Page ID
                      <input
                        className="pipelines-input w-full mt-1"
                        placeholder="Optional provider ID"
                        value={formModal.external_page_id || ''}
                        onChange={event => onUpdateForm({ external_page_id: event.target.value })}
                      />
                    </label>
                  </div>
                  <div className="kc-provider-picker">
                    <div className="kc-provider-picker-row">
                      <label className="block text-sm font-medium text-[var(--text-secondary)]">
                        Search provider pages
                        <input
                          className="pipelines-input w-full mt-1"
                          placeholder="Repository guardrails"
                          value={formModal.page_search_query || ''}
                          onChange={event => onUpdateForm({ page_search_query: event.target.value })}
                        />
                      </label>
                      <button
                        type="button"
                        className="kc-demo-outline-btn"
                        onClick={onSearchPages}
                        disabled={formModal.page_search_loading || !formModal.connection_id}
                      >
                        {formModal.page_search_loading ? 'Searching...' : 'Search'}
                      </button>
                      <button
                        type="button"
                        className="kc-demo-outline-btn"
                        onClick={onResolvePage}
                        disabled={formModal.page_resolving || !formModal.connection_id || (!formModal.external_page_url && !formModal.external_page_id)}
                      >
                        {formModal.page_resolving ? 'Previewing...' : 'Preview'}
                      </button>
                    </div>
                    {formModal.page_error ? <WorkflowInlineAlert id={`${formModalId}-page-error`}>{formModal.page_error}</WorkflowInlineAlert> : null}
                    {formModal.page_search_results?.length ? (
                      <div className="kc-provider-results" role="listbox" aria-label="Provider page results">
                        {formModal.page_search_results.map(page => (
                          <button
                            key={page.id}
                            type="button"
                            className="kc-provider-result"
                            onClick={() => onSelectPage?.(page)}
                          >
                            <strong>{page.title || page.id}</strong>
                            <span>{page.url || page.id}</span>
                          </button>
                        ))}
                        {formModal.page_search_cursor ? (
                          <button type="button" className="kc-demo-table-link" onClick={onLoadMorePages} disabled={formModal.page_search_loading}>
                            Load more
                          </button>
                        ) : null}
                      </div>
                    ) : formModal.page_search_query && !formModal.page_search_loading ? (
                      <div className="kc-demo-alert kc-demo-alert--subtle">No provider pages selected yet.</div>
                    ) : null}
                  </div>
                </section>
                <section className="kc-external-panel kc-external-panel--sync" aria-label="Sync settings">
                  <div className="kc-external-panel-head">
                    <strong>Sync settings</strong>
                    <span>Provider content is stored as the runtime snapshot.</span>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <label className="block text-sm font-medium text-[var(--text-secondary)]">
                      Sync mode
                      <select
                        className="pipelines-input w-full mt-1"
                        value={formModal.sync_mode || 'manual'}
                        onChange={event => onUpdateForm({ sync_mode: event.target.value as KnowledgeSyncMode })}
                      >
                        {knowledgeSyncModeOptions.map(option => (
                          <option key={option.value} value={option.value}>{option.label}</option>
                        ))}
                      </select>
                    </label>
                    <label className="block text-sm font-medium text-[var(--text-secondary)]">
                      Failure behavior
                      <select
                        className="pipelines-input w-full mt-1"
                        value={formModal.failure_mode || 'fail'}
                        onChange={event => onUpdateForm({ failure_mode: event.target.value as KnowledgeFailureMode })}
                      >
                        {knowledgeFailureModeOptions.map(option => (
                          <option key={option.value} value={option.value}>{option.label}</option>
                        ))}
                      </select>
                    </label>
                  </div>
                  {formModal.page_preview ? (
                    <div className="kc-provider-preview">
                      <div>
                        <strong>{formModal.page_preview.title || formModal.page_preview.id}</strong>
                        <span>{formModal.page_preview.url || formModal.page_preview.id}</span>
                      </div>
                      <pre><code>{formModal.page_preview.text || 'No preview text available.'}</code></pre>
                    </div>
                  ) : null}
                  <label className="block text-sm font-medium text-[var(--text-secondary)]">
                    Cached content preview
                    <textarea
                      className="pipelines-input w-full mt-1"
                      placeholder="Paste the provider page text to seed the first runtime snapshot."
                      value={formModal.content}
                      onChange={event => onUpdateForm({ content: event.target.value })}
                      rows={6}
                    />
                  </label>
                </section>
              </div>
            ) : null}
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

      {connectionModal ? (
        <WorkflowDialogFrame
          id={connectionModalId}
          titleId={connectionTitleId}
          descriptionId={connectionModal.error ? connectionErrorId : undefined}
          onClose={onCloseConnection}
          className="pipelines-modal-card max-w-lg w-full"
        >
          <header className="pipelines-modal-header">
            <div>
              <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">
                {isEditingConnection ? 'Update knowledge connection' : 'New knowledge connection'}
              </p>
              <h3 id={connectionTitleId} className="text-lg font-semibold text-[var(--text-primary)]">
                {isEditingConnection ? 'Reconnect external page provider' : 'Add external page connection'}
              </h3>
            </div>
            <button type="button" className="glass-button-ghost" onClick={onCloseConnection} disabled={connectionModal.pending}>
              Close
            </button>
          </header>
          <div className="pipelines-modal-body space-y-4">
            <div className="grid gap-3 sm:grid-cols-[180px_1fr]">
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Provider
                <select
                  className="pipelines-input w-full mt-1"
                  value={connectionModal.provider}
                  onChange={event => onUpdateConnection({ provider: event.target.value as KnowledgeConnectionModalState['provider'] })}
                >
                  {knowledgeConnectionProviders.map(provider => (
                    <option key={provider.value} value={provider.value}>{provider.label}</option>
                  ))}
                </select>
              </label>
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Team
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="team/path"
                  value={connectionModal.team}
                  onChange={event => onUpdateConnection({ team: event.target.value })}
                  disabled={isEditingConnection}
                />
              </label>
            </div>
            <div className="grid gap-3 sm:grid-cols-[1fr_180px]">
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                Display name
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="Team Notion"
                  value={connectionModal.display_name}
                  onChange={event => onUpdateConnection({ display_name: event.target.value })}
                  data-dialog-initial-focus
                />
              </label>
              <label className="block text-sm font-medium text-[var(--text-secondary)]">
                ID
                <input
                  className="pipelines-input w-full mt-1"
                  placeholder="team-notion"
                  value={connectionModal.name}
                  onChange={event => onUpdateConnection({ name: event.target.value })}
                  disabled={isEditingConnection}
                />
              </label>
            </div>
            <label className="block text-sm font-medium text-[var(--text-secondary)]">
              Base URL
              <input
                className="pipelines-input w-full mt-1"
                placeholder="https://www.notion.so/acme"
                value={connectionModal.base_url}
                onChange={event => onUpdateConnection({ base_url: event.target.value })}
              />
            </label>
            <label className="block text-sm font-medium text-[var(--text-secondary)]">
              Credential reference
              <input
                className="pipelines-input w-full mt-1"
                placeholder={isEditingConnection ? 'Leave empty to keep the stored credential' : 'credential namespace/name'}
                value={connectionModal.credential_ref}
                onChange={event => onUpdateConnection({ credential_ref: event.target.value })}
              />
            </label>
            {connectionModal.error ? <WorkflowInlineAlert id={connectionErrorId}>{connectionModal.error}</WorkflowInlineAlert> : null}
          </div>
          <div className="pipelines-modal-footer">
            <div className="pipelines-modal-actions">
              <button type="button" className="glass-button-ghost" onClick={onCloseConnection} disabled={connectionModal.pending}>
                Cancel
              </button>
              <button type="button" className="glass-button-primary" onClick={onSubmitConnection} disabled={connectionModal.pending}>
                {connectionModal.pending ? (isEditingConnection ? 'Saving...' : 'Creating...') : (isEditingConnection ? 'Save connection' : 'Create connection')}
              </button>
            </div>
          </div>
        </WorkflowDialogFrame>
      ) : null}
    </>
  );
}
