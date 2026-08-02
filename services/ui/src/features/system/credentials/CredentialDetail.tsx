import { Copy, GitBranch, Power, RotateCcw, ShieldAlert, Trash2, X } from 'lucide-react';
import type { FormEvent, ReactNode } from 'react';
import type { CredentialRecord } from './model';
import { credentialReferenceDisplay } from './model';
import { formatCredentialDate, formatCredentialLabel, formatCredentialScopeLabel } from './presentation';
import { CredentialStatusBadge } from './CredentialStatusBadge';

type CredentialDetailProps = {
  credential: CredentialRecord;
  canManage: boolean;
  saving: boolean;
  operationError?: string | null;
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
  operationError,
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
  const sourceLabel = credential.managed_by_config_repo ? 'GitOps' : 'System';
  const canDeleteHistory = credential.versions.length >= 2;
  const detailFields: Array<{ label: string; value: ReactNode }> = [
    { label: 'Scope', value: scopeLabel },
    { label: 'Kind', value: formatCredentialLabel(credential.kind) },
    { label: 'Active version', value: credential.active_version || '-' },
    { label: 'Source', value: sourceLabel },
    { label: 'Expires', value: formatCredentialDate(credential.expires_at) },
    { label: 'Last rotated', value: formatCredentialDate(credential.last_rotated_at) },
    { label: 'Updated by', value: credential.updated_by || '-' },
    { label: 'Namespace', value: reference.namespace },
  ];
  const copyReference = () => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) return;
    void navigator.clipboard.writeText(credential.reference).catch(() => undefined);
  };

  return (
    <div
      className="credential-detail__overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="credential-detail-heading"
      onMouseDown={event => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside className="credential-detail__drawer">
        <header className="credential-detail__header">
          <div className="credential-detail__headline">
            <div className="min-w-0">
              <div className="credential-detail__crumbs">
                <span className="credential-registry__pill">{scopeLabel}</span>
                <span className="credential-registry__pill">{formatCredentialLabel(reference.category)}</span>
                <CredentialStatusBadge status={credential.status} />
                <span className={`credential-registry__pill ${credential.managed_by_config_repo ? 'credential-registry__pill--good' : ''}`}>
                  {credential.managed_by_config_repo ? <GitBranch className="h-3 w-3" aria-hidden="true" /> : null}
                  {sourceLabel}
                </span>
              </div>
              <h3 id="credential-detail-heading" className="credential-detail__title">
                {formatCredentialLabel(reference.displayName)}
              </h3>
              <p className="credential-detail__subtitle">{credential.description || 'No description'}</p>
            </div>
            <button type="button" aria-label="Close credential details" className="credential-detail__close" onClick={onClose}>
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>

        <div className="credential-detail__body">
          <section>
            <p className="credential-detail__label">Reference</p>
            <div className="credential-detail__copybox">
              <code>{credential.reference}</code>
              <button type="button" className="credential-detail__copy" aria-label="Copy credential reference" onClick={copyReference}>
                <Copy className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </section>

          <dl className="credential-detail__meta-grid">
            {detailFields.map(field => (
              <div key={field.label} className="credential-detail__meta">
                <dt>{field.label}</dt>
                <dd>{field.value}</dd>
              </div>
            ))}
          </dl>

          {canManage && (
            <form onSubmit={onSubmitRotation}>
              <div className="credential-detail__section-title">
                <RotateCcw className="h-4 w-4 text-[var(--credential-muted)]" aria-hidden="true" />
                <h4>Rotate value</h4>
              </div>
              {operationError ? (
                <div className="credential-detail__error" role="alert">
                  {operationError}
                </div>
              ) : null}
              <label className="block">
                <span className="sr-only">New credential value</span>
                <textarea
                  className="credential-registry__field"
                  value={rotationValue}
                  onChange={event => onRotationValueChange(event.target.value)}
                  autoComplete="new-password"
                  placeholder="Enter a new write-only value"
                />
              </label>
              <button type="submit" className="credential-registry__button credential-registry__button--primary mt-3 w-full" disabled={saving}>
                Rotate credential
              </button>
            </form>
          )}

          <section>
            <div className="credential-detail__section-title">
              <h4>Version history</h4>
              <span className="credential-registry__pill">{credential.versions.length} stored</span>
            </div>
            {credential.versions.length > 0 ? (
              <div>
                {credential.versions.map(version => {
                  const isActive = version.version === credential.active_version;
                  return (
                    <div key={version.version} className="credential-detail__version">
                      <div className="credential-detail__version-top">
                        <div>
                          <p className="credential-detail__version-title">
                            Version {version.version}
                            {isActive ? <span className="ml-2 text-xs text-[var(--credential-muted)]">Active</span> : null}
                          </p>
                          <p className="credential-detail__version-meta">
                            {formatCredentialDate(version.created_at)} by {version.created_by || 'system'}
                          </p>
                        </div>
                        {canManage && !isActive ? (
                          <div className="credential-detail__version-actions">
                            <button type="button" className="credential-registry__button credential-registry__button--ghost credential-registry__button--small" onClick={() => onActivateVersion(version.version)} disabled={saving}>
                              Activate
                            </button>
                            <button
                              type="button"
                              className="credential-registry__button credential-registry__button--danger credential-registry__button--small"
                              aria-label={`Delete version ${version.version}`}
                              title={canDeleteHistory ? `Delete version ${version.version}` : 'At least two versions are required'}
                              onClick={() => onDeleteVersion(version.version)}
                              disabled={saving || !canDeleteHistory}
                            >
                              <Trash2 className="h-4 w-4" aria-hidden="true" />
                            </button>
                          </div>
                        ) : null}
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="credential-detail__empty">No value has been stored yet.</p>
            )}
          </section>
        </div>

        {canManage && (
          <footer className="credential-detail__footer">
            {credential.status === 'disabled' ? (
              <button type="button" className="credential-registry__button credential-registry__button--ghost" onClick={onEnable} disabled={saving}>
                <Power className="h-4 w-4" aria-hidden="true" />
                Enable
              </button>
            ) : (
              <button type="button" className="credential-registry__button credential-registry__button--ghost" onClick={onDisable} disabled={saving}>
                <ShieldAlert className="h-4 w-4" aria-hidden="true" />
                Disable
              </button>
            )}
            <button type="button" className="credential-registry__button credential-registry__button--danger" onClick={onDelete} disabled={saving}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
              Delete credential
            </button>
          </footer>
        )}
      </aside>
    </div>
  );
}
