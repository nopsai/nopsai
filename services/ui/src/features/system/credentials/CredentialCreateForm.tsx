import { KeyRound, X } from 'lucide-react';
import type { Dispatch, FormEvent, SetStateAction } from 'react';
import {
  buildCredentialReference,
  CREDENTIAL_KINDS,
  type CredentialFormState,
} from './model';

type CredentialCreateFormProps = {
  form: CredentialFormState;
  saving: boolean;
  setForm: Dispatch<SetStateAction<CredentialFormState>>;
  onClose: () => void;
  onSubmit: (event: FormEvent) => void;
};

export function CredentialCreateForm({
  form,
  saving,
  setForm,
  onClose,
  onSubmit,
}: CredentialCreateFormProps) {
  const referencePreview = buildCredentialReference(form.namespace, form.name || 'name');

  return (
    <aside className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">Credential registry</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">New credential</h3>
        </div>
        <button type="button" aria-label="Close credential form" className="glass-button-ghost !px-2" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      <form className="space-y-4" onSubmit={onSubmit}>
        <div className="grid gap-3 sm:grid-cols-[minmax(110px,0.35fr)_minmax(0,1fr)]">
          <label className="flex flex-col gap-1 text-sm">
            <span>Namespace</span>
            <input
              className="pipelines-input"
              value={form.namespace}
              onChange={event => setForm(current => ({ ...current, namespace: event.target.value }))}
              placeholder="system"
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>Name / path</span>
            <input
              className="pipelines-input"
              autoFocus
              value={form.name}
              onChange={event => setForm(current => ({ ...current, name: event.target.value }))}
              placeholder="llm/openai-primary"
            />
          </label>
        </div>

        <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2">
          <p className="text-xs text-[var(--text-secondary)]">Reference preview</p>
          <code className="text-xs break-all text-[var(--text-primary)]">{referencePreview}</code>
        </div>

        <label className="flex flex-col gap-1 text-sm">
          <span>Kind</span>
          <select
            className="pipelines-input"
            value={form.kind}
            onChange={event => setForm(current => ({ ...current, kind: event.target.value as typeof current.kind }))}
          >
            {CREDENTIAL_KINDS.map(kind => <option key={kind} value={kind}>{kind}</option>)}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Description</span>
          <input
            className="pipelines-input"
            value={form.description}
            onChange={event => setForm(current => ({ ...current, description: event.target.value }))}
            placeholder="What uses this credential?"
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Initial value <span className="text-[var(--text-secondary)]">(optional)</span></span>
          <textarea
            className="pipelines-input min-h-24"
            value={form.value}
            onChange={event => setForm(current => ({ ...current, value: event.target.value }))}
            autoComplete="new-password"
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Expires at <span className="text-[var(--text-secondary)]">(optional)</span></span>
          <input
            type="datetime-local"
            className="pipelines-input"
            value={form.expires_at}
            onChange={event => setForm(current => ({ ...current, expires_at: event.target.value }))}
          />
        </label>
        <button type="submit" className="glass-button-primary w-full justify-center" disabled={saving}>
          <KeyRound className="h-4 w-4" aria-hidden="true" />
          {saving ? 'Creating...' : 'Create credential'}
        </button>
      </form>
    </aside>
  );
}
