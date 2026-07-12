import { GitBranch, KeyRound, Power, RotateCcw, ShieldAlert, Trash2, X } from 'lucide-react';
import type { FormEvent, ReactNode } from 'react';
import type { CredentialRecord } from './model';
import { credentialReferenceDisplay } from './model';
import { formatCredentialDate, formatCredentialLabel, formatCredentialScopeLabel } from './presentation';
import { CredentialStatusBadge } from './CredentialStatusBadge';

type CredentialDetailProps = {
  credential: CredentialRecord;
  canManage: boolean;
  saving: boolean;
  teamPaths: string[];
  rotationValue: string;
  onRotationValueChange: (value: string) => void;
  onSubmitRotation: (event: FormEvent) => void;
  onActivateVersion: (version: number) => void;
  onDeleteVersion: (version: number) => void;
  onEnable: () => void;
  onDisable: () => void;
  onDelete: () => void;
  onClose: () => void;
};

export function CredentialDetail({
  credential,
  canManage,
  saving,
  teamPaths,
  rotationValue,
  onRotationValueChange,
  onSubmitRotation,
  onActivateVersion,
  onDeleteVersion,
  onEnable,
  onDisable,
  onDelete,
  onClose,
}: CredentialDetailProps) {
  const reference = credentialReferenceDisplay(credential.reference, teamPaths);
  const scopeLabel = formatCredentialScopeLabel(reference.scopeKind, reference.scopePath, reference.namespace);
  const canDeleteHistory = credential.versions.length >= 2;
  const sourceLabel = credential.managed_by_config_repo ? 'GitOps' : 'System';

  return (
    <div
      className="fixed inset-0 z-50 flex justify-end bg-[rgba(2,6,23,0.48)]"
      role="dialog"
      aria-modal="true"
      aria-labelledby="credential-detail-heading"
      onMouseDown={event => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside className="flex h-full w-full max-w-[560px] flex-col border-l border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-2xl">
        <header className="border-b border-[var(--border-primary)] px-5 py-4">
          <div className="flex items-start justify-between gap-4">
            <div className="flex min-w-0 items-start gap-3">
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-secondary)] text-[var(--text-accent)]">
                <KeyRound className="h-5 w-5" aria-hidden="true" />
              </span>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-secondary)]">
                  <span>{scopeLabel}</span>
                  <span>{formatCredentialLabel(reference.category)}</span>
                  <CredentialStatusBadge status={credential.status} />
                  {credential.managed_by_config_repo && (
                    <span className="runner-pill runner-pill--muted inline-flex items-center gap-1">
                      <GitBranch className="h-3 w-3" aria-hidden="true" />
                      GitOps
                    </span>
                  )}
                </div>
                <h3 id="credential-detail-heading" className="mt-2 break-words text-xl font-semibold text-[var(--text-primary)]">
                  {formatCredentialLabel(reference.displayName)}
                </h3>
                <p className="mt-1 line-clamp-2 text-sm text-[var(--text-secondary)]">
                  {credential.description || 'No description'}
                </p>
              </div>
            </div>
            <button type="button" aria-label="Close credential details" className="glass-button-ghost !rounded-lg !px-2" onClick={onClose}>
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto">
          <section className="border-b border-[var(--border-primary)] px-5 py-4">
            <p className="text-xs text-[var(--text-secondary)]">Reference</p>
            <code className="mt-2 block break-all rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-xs text-[var(--text-primary)]">
              {credential.reference}
            </code>
          </section>

          <dl className="grid grid-cols-2 border-b border-[var(--border-primary)]">
            <CredentialDetailField label="Scope" value={scopeLabel} />
            <CredentialDetailField label="Kind" value={formatCredentialLabel(credential.kind)} />
            <CredentialDetailField label="Active version" value={credential.active_version || '-'} />
            <CredentialDetailField label="Source" value={sourceLabel} />
            <CredentialDetailField label="Expires" value={formatCredentialDate(credential.expires_at)} />
            <CredentialDetailField label="Last rotated" value={formatCredentialDate(credential.last_rotated_at)} />
            <CredentialDetailField label="Updated by" value={credential.updated_by || '-'} />
            <CredentialDetailField label="Namespace" value={reference.namespace} />
          </dl>

          {canManage && (
            <form className="border-b border-[var(--border-primary)] px-5 py-4" onSubmit={onSubmitRotation}>
              <div className="mb-3 flex items-center gap-2">
                <RotateCcw className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
                <h4 className="font-semibold text-[var(--text-primary)]">Rotate value</h4>
              </div>
              <label className="flex flex-col gap-2 text-sm">
                <span className="sr-only">New credential value</span>
                <textarea
                  className="pipelines-input !rounded-lg min-h-28"
                  value={rotationValue}
                  onChange={event => onRotationValueChange(event.target.value)}
                  autoComplete="new-password"
                  placeholder="Enter a new write-only value"
                />
              </label>
              <button type="submit" className="glass-button-primary !rounded-lg mt-3 w-full justify-center" disabled={saving}>
                Rotate credential
              </button>
            </form>
          )}

          <section className="px-5 py-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <h4 className="font-semibold text-[var(--text-primary)]">Version history</h4>
              <span className="runner-pill runner-pill--muted">{credential.versions.length} stored</span>
            </div>
            <div className="space-y-2">
              {credential.versions.map(version => {
                const isActive = version.version === credential.active_version;
                return (
                  <div key={version.version} className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-3">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <p className="font-semibold text-[var(--text-primary)]">
                          Version {version.version}
                          {isActive && <span className="ml-2 text-xs text-[var(--text-secondary)]">Active</span>}
                        </p>
                        <p className="text-xs text-[var(--text-secondary)]">
                          {formatCredentialDate(version.created_at)} by {version.created_by || 'system'}
                        </p>
                      </div>
                      {canManage && !isActive && (
                        <div className="flex items-center gap-2">
                          <button type="button" className="glass-button-subtle !rounded-lg" onClick={() => onActivateVersion(version.version)} disabled={saving}>
                            Activate
                          </button>
                          <button
                            type="button"
                            className="glass-button-danger !rounded-lg !px-2"
                            aria-label={`Delete version ${version.version}`}
                            title={canDeleteHistory ? `Delete version ${version.version}` : 'At least two versions are required'}
                            onClick={() => onDeleteVersion(version.version)}
                            disabled={saving || !canDeleteHistory}
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
              {credential.versions.length === 0 && (
                <p className="rounded-lg border border-dashed border-[var(--border-primary)] px-4 py-6 text-sm text-[var(--text-secondary)]">
                  No value has been stored yet.
                </p>
              )}
            </div>
          </section>
        </div>

        {canManage && (
          <footer className="grid gap-2 border-t border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 sm:grid-cols-2">
            {credential.status === 'disabled' ? (
              <button type="button" className="glass-button-subtle !rounded-lg justify-center" onClick={onEnable} disabled={saving}>
                <Power className="h-4 w-4" aria-hidden="true" />
                Enable
              </button>
            ) : (
              <button type="button" className="glass-button-subtle !rounded-lg justify-center" onClick={onDisable} disabled={saving}>
                <ShieldAlert className="h-4 w-4" aria-hidden="true" />
                Disable
              </button>
            )}
            <button type="button" className="glass-button-danger !rounded-lg justify-center" onClick={onDelete} disabled={saving}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
              Delete credential
            </button>
          </footer>
        )}
      </aside>
    </div>
  );
}

function CredentialDetailField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0 border-r border-t border-[var(--border-primary)] px-5 py-3 even:border-r-0">
      <dt className="text-xs text-[var(--text-secondary)]">{label}</dt>
      <dd className="mt-1 break-words text-sm font-medium text-[var(--text-primary)]">{value}</dd>
    </div>
  );
}
