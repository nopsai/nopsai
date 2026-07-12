import { KeyRound, X } from 'lucide-react';
import { useEffect, useMemo } from 'react';
import type { Dispatch, FormEvent, SetStateAction } from 'react';
import {
  buildCredentialReference,
  CREDENTIAL_KINDS,
  normalizeCredentialTeamPath,
  type CredentialFormState,
} from './model';
import { formatCredentialLabel } from './presentation';

type CredentialCreateFormProps = {
  allowSystemScope: boolean;
  form: CredentialFormState;
  saving: boolean;
  setForm: Dispatch<SetStateAction<CredentialFormState>>;
  teamPaths: string[];
  teamPathsLoading?: boolean;
  onClose: () => void;
  onSubmit: (event: FormEvent) => void;
};

export function CredentialCreateForm({
  allowSystemScope,
  form,
  saving,
  setForm,
  teamPaths,
  teamPathsLoading = false,
  onClose,
  onSubmit,
}: CredentialCreateFormProps) {
  const scopeOptions = useMemo(
    () => Array.from(
      new Set(
        teamPaths
          .map(path => normalizeCredentialTeamPath(path))
          .filter(path => path && path.toLowerCase() !== 'root')
      )
    ).sort((left, right) => left.localeCompare(right)),
    [teamPaths]
  );
  const effectiveTeamPath = allowSystemScope ? form.team_path : normalizeCredentialTeamPath(form.team_path) || scopeOptions[0] || '';
  const referencePreview = buildCredentialReference(
    effectiveTeamPath ? 'team' : form.namespace,
    form.name || 'name',
    effectiveTeamPath
  );
  const teamScopeUnavailable = !allowSystemScope && !teamPathsLoading && scopeOptions.length === 0;
  const submitDisabled = saving || teamScopeUnavailable || (!allowSystemScope && !effectiveTeamPath);

  useEffect(() => {
    if (allowSystemScope || scopeOptions.length === 0) return;
    const currentTeamPath = normalizeCredentialTeamPath(form.team_path);
    if (currentTeamPath && scopeOptions.includes(currentTeamPath)) return;
    setForm(current => ({ ...current, namespace: 'team', team_path: scopeOptions[0] || '' }));
  }, [allowSystemScope, form.team_path, scopeOptions, setForm]);

  const updateTeamPath = (teamPath: string) => {
    setForm(current => ({
      ...current,
      team_path: teamPath,
      namespace: teamPath ? 'team' : allowSystemScope ? 'system' : 'team',
    }));
  };

  return (
    <div
      className="credential-create__overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="new-credential-heading"
      onMouseDown={event => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside className="credential-create__modal">
        <div className="credential-create__head">
          <div>
            <p className="credential-create__kicker">Credential registry</p>
            <h3 id="new-credential-heading" className="credential-create__title">New credential</h3>
          </div>
          <button type="button" aria-label="Close credential form" className="credential-create__close" onClick={onClose}>
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>

        <form onSubmit={onSubmit}>
          <div className="credential-create__body">
            <div className="credential-create__grid">
              <label className="credential-create__group">
                <span>Team</span>
                <select
                  aria-label="Team"
                  className="credential-registry__field"
                  value={form.team_path}
                  onChange={event => updateTeamPath(event.target.value)}
                  disabled={!allowSystemScope && (teamPathsLoading || scopeOptions.length === 0)}
                >
                  {allowSystemScope ? <option value="">System / global</option> : null}
                  {scopeOptions.map(path => <option key={path} value={path}>/{path}</option>)}
                </select>
                {teamPathsLoading ? <span className="credential-create__help">Loading teams...</span> : null}
                {teamScopeUnavailable ? <span className="credential-create__help">Team access is required to create credentials.</span> : null}
              </label>
              <label className="credential-create__group">
                <span>Name / path</span>
                <input
                  aria-label="Name / path"
                  className="credential-registry__field"
                  autoFocus
                  value={form.name}
                  onChange={event => setForm(current => ({ ...current, name: event.target.value }))}
                  placeholder="llm/openai-primary"
                />
                <span className="credential-create__help">Use a clear, unique path to identify this credential.</span>
              </label>
              <div className="credential-create__group credential-create__group--full">
                <span>Reference preview</span>
                <div className="credential-create__preview">{referencePreview}</div>
                <span className="credential-create__help">This reference stays stable for pipelines, profiles, MCP, and GitOps config.</span>
              </div>
              <label className="credential-create__group credential-create__group--full">
                <span>Kind</span>
                <select
                  aria-label="Kind"
                  className="credential-registry__field"
                  value={form.kind}
                  onChange={event => setForm(current => ({ ...current, kind: event.target.value as typeof current.kind }))}
                >
                  {CREDENTIAL_KINDS.map(kind => <option key={kind} value={kind}>{formatCredentialLabel(kind)}</option>)}
                </select>
              </label>
              <label className="credential-create__group credential-create__group--full">
                <span>Description</span>
                <input
                  aria-label="Description"
                  className="credential-registry__field"
                  value={form.description}
                  onChange={event => setForm(current => ({ ...current, description: event.target.value }))}
                  placeholder="What uses this credential?"
                />
              </label>
              <label className="credential-create__group credential-create__group--full">
                <span>Initial value <span className="credential-create__help">(optional)</span></span>
                <textarea
                  aria-label="Initial value"
                  className="credential-registry__field"
                  value={form.value}
                  onChange={event => setForm(current => ({ ...current, value: event.target.value }))}
                  autoComplete="new-password"
                />
                <span className="credential-create__help">The value is encrypted before it is stored.</span>
              </label>
              <label className="credential-create__group credential-create__group--full">
                <span>Expires at <span className="credential-create__help">(optional)</span></span>
                <input
                  aria-label="Expires at"
                  type="datetime-local"
                  className="credential-registry__field"
                  value={form.expires_at}
                  onChange={event => setForm(current => ({ ...current, expires_at: event.target.value }))}
                />
              </label>
            </div>
          </div>
          <div className="credential-create__foot">
            <button type="button" className="credential-registry__button credential-registry__button--ghost" onClick={onClose} disabled={saving}>
              Cancel
            </button>
            <button type="submit" className="credential-registry__button credential-registry__button--primary" disabled={submitDisabled}>
              <KeyRound className="h-4 w-4" aria-hidden="true" />
              {saving ? 'Creating...' : 'Create credential'}
            </button>
          </div>
        </form>
      </aside>
    </div>
  );
}
