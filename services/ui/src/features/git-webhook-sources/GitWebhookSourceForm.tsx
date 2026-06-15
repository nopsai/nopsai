import { X } from 'lucide-react';
import { WorkflowDialogFrame } from '../../components/WorkflowPrimitives';
import {
  GIT_WEBHOOK_AUTH_MODES,
  GIT_WEBHOOK_PROVIDERS,
  type GitWebhookSource,
  type GitWebhookSourceFormState,
} from './model';

export function GitWebhookSourceForm({
  source,
  form,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  source: GitWebhookSource | null;
  form: GitWebhookSourceFormState;
  saving: boolean;
  error: string | null;
  onChange: (form: GitWebhookSourceFormState) => void;
  onClose: () => void;
  onSubmit: React.FormEventHandler<HTMLFormElement>;
}) {
  const update = <K extends keyof GitWebhookSourceFormState>(
    key: K,
    value: GitWebhookSourceFormState[K]
  ) => onChange({ ...form, [key]: value });
  const titleId = 'git-webhook-source-form-title';
  const errorId = 'git-webhook-source-form-error';

  return (
    <WorkflowDialogFrame
      id="git-webhook-source-form-modal"
      titleId={titleId}
      descriptionId={error ? errorId : undefined}
      onClose={onClose}
      className="w-full max-w-3xl overflow-hidden rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-2xl"
      overlayClassName="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] p-4"
    >
      <form onSubmit={onSubmit}>
        <header className="flex items-start justify-between gap-4 border-b border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-5 py-4">
          <div className="min-w-0">
            <h2 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">
              {source ? 'Edit Git webhook source' : 'New Git webhook source'}
            </h2>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">
              Provider credentials are referenced from the encrypted credential registry.
            </p>
          </div>
          <button
            type="button"
            className="glass-button-ghost"
            onClick={onClose}
            aria-label="Close"
            disabled={saving}
          >
            <X className="h-4 w-4" />
          </button>
        </header>

        <div className="max-h-[calc(100vh-12rem)] overflow-y-auto p-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Source ID">
              <input
                className="pipelines-input w-full font-mono"
                value={form.id}
                onChange={event => update('id', event.target.value)}
                disabled={Boolean(source)}
                placeholder="gitlab-platform"
                required
                data-dialog-initial-focus
              />
            </Field>
            <Field label="Display name">
              <input
                className="pipelines-input w-full"
                value={form.name}
                onChange={event => update('name', event.target.value)}
                placeholder="GitLab Platform"
              />
            </Field>
            <Field label="Provider">
              <select
                className="pipelines-input w-full"
                value={form.provider}
                onChange={event => update('provider', event.target.value as GitWebhookSourceFormState['provider'])}
              >
                {GIT_WEBHOOK_PROVIDERS.map(provider => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
            </Field>
            <Field label="Authentication">
              <select
                className="pipelines-input w-full"
                value={form.authMode}
                onChange={event => update('authMode', event.target.value as GitWebhookSourceFormState['authMode'])}
              >
                {GIT_WEBHOOK_AUTH_MODES.map(mode => (
                  <option key={mode} value={mode}>{mode === 'none' ? 'none (internal only)' : mode}</option>
                ))}
              </select>
            </Field>
          </div>

          <Field label="Description">
            <textarea
              className="pipelines-input min-h-20 w-full"
              value={form.description}
              onChange={event => update('description', event.target.value)}
              placeholder="Receives repository events from the primary GitLab instance."
            />
          </Field>

          {form.authMode !== 'none' ? (
            <Field label="Credential reference" hint="Create or rotate the value in System > Credentials.">
              <input
                className="pipelines-input w-full font-mono"
                value={form.credentialRef}
                onChange={event => update('credentialRef', event.target.value)}
                placeholder="credential://system/webhooks/gitlab-platform"
                required
              />
            </Field>
          ) : (
            <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm text-[var(--text-secondary)]">
              Unauthenticated sources must only be exposed through a trusted, network-isolated ingress.
            </div>
          )}

          <Field
            label="Repository allowlist"
            hint="One owner/repository pattern per line. Recursive wildcards are supported."
          >
            <textarea
              className="pipelines-input min-h-28 w-full font-mono"
              value={form.repositoryAllowlistText}
              onChange={event => update('repositoryAllowlistText', event.target.value)}
              placeholder={'platform/api\nplatform/*'}
              required
            />
          </Field>

          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <Field label="Rate limit per minute" hint="Leave empty for no source-level limit.">
              <input
                className="pipelines-input w-full"
                type="number"
                min="1"
                step="1"
                value={form.rateLimitPerMinute}
                onChange={event => update('rateLimitPerMinute', event.target.value)}
                placeholder="120"
              />
            </Field>
            <label className="flex items-center gap-3 self-end rounded-lg border border-[var(--border-primary)] px-4 py-3 text-sm text-[var(--text-primary)]">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={event => update('enabled', event.target.checked)}
              />
              Accept webhook deliveries
            </label>
          </div>

          {error ? (
            <div
              id={errorId}
              className="mt-4 rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500"
              role="alert"
            >
              {error}
            </div>
          ) : null}
        </div>

        <footer className="flex justify-end gap-2 border-t border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-5 py-4">
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={saving}>
            {saving ? 'Saving...' : source ? 'Save source' : 'Create source'}
          </button>
        </footer>
      </form>
    </WorkflowDialogFrame>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="mt-4 block text-sm text-[var(--text-primary)]">
      <span className="font-medium">{label}</span>
      {hint ? <span className="ml-2 text-xs text-[var(--text-secondary)]">{hint}</span> : null}
      <span className="mt-1 block">{children}</span>
    </label>
  );
}
