import { Power, ShieldAlert, Trash2, X } from 'lucide-react';
import type { FormEvent } from 'react';
import type { CredentialRecord } from './model';
import { parseCredentialReference } from './model';
import { formatCredentialDate, formatCredentialLabel } from './presentation';
import { CredentialStatusBadge } from './CredentialStatusBadge';

type CredentialDetailProps = {
  credential: CredentialRecord;
  canManage: boolean;
  saving: boolean;
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
  const reference = parseCredentialReference(credential.reference);
  const canDeleteHistory = credential.versions.length >= 2;

  return (
    <aside className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-[var(--text-secondary)]">
              {formatCredentialLabel(reference.category)}
            </span>
            <CredentialStatusBadge status={credential.status} />
          </div>
          <h3 className="mt-1 text-lg font-semibold text-[var(--text-primary)] break-words">
            {formatCredentialLabel(reference.displayName)}
          </h3>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            {credential.description || 'No description'}
          </p>
        </div>
        <button type="button" aria-label="Close credential details" className="glass-button-ghost !px-2" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2">
        <p className="text-xs text-[var(--text-secondary)]">Reference</p>
        <code className="text-xs break-all text-[var(--text-primary)]">{credential.reference}</code>
      </div>

      <dl className="grid grid-cols-2 gap-3 text-sm">
        <div><dt className="text-[var(--text-secondary)]">Namespace</dt><dd>{reference.namespace}</dd></div>
        <div><dt className="text-[var(--text-secondary)]">Kind</dt><dd>{formatCredentialLabel(credential.kind)}</dd></div>
        <div><dt className="text-[var(--text-secondary)]">Active version</dt><dd>{credential.active_version || '-'}</dd></div>
        <div><dt className="text-[var(--text-secondary)]">Expires</dt><dd>{formatCredentialDate(credential.expires_at)}</dd></div>
        <div><dt className="text-[var(--text-secondary)]">Source</dt><dd>{credential.managed_by_config_repo ? 'GitOps' : 'System'}</dd></div>
        <div><dt className="text-[var(--text-secondary)]">Updated by</dt><dd>{credential.updated_by || '-'}</dd></div>
      </dl>

      {canManage && (
        <form className="space-y-3 border-t border-[var(--border-primary)] pt-4" onSubmit={onSubmitRotation}>
          <label className="flex flex-col gap-1 text-sm">
            <span>Rotate value</span>
            <textarea
              className="pipelines-input min-h-24"
              value={rotationValue}
              onChange={event => onRotationValueChange(event.target.value)}
              autoComplete="new-password"
              placeholder="Enter a new write-only value"
            />
          </label>
          <button type="submit" className="glass-button-primary w-full justify-center" disabled={saving}>
            Rotate credential
          </button>
        </form>
      )}

      <div className="space-y-2 border-t border-[var(--border-primary)] pt-4">
        <div className="flex items-center justify-between gap-3">
          <h4 className="font-semibold text-sm">Version history</h4>
          <span className="text-xs text-[var(--text-secondary)]">{credential.versions.length} stored</span>
        </div>
        {credential.versions.map(version => {
          const isActive = version.version === credential.active_version;
          return (
            <div key={version.version} className="rounded-lg border border-[var(--border-primary)] p-3 flex items-center justify-between gap-3 text-sm">
              <div>
                <p className="font-semibold">
                  Version {version.version}
                  {isActive && <span className="ml-2 text-xs text-[var(--text-secondary)]">Active</span>}
                </p>
                <p className="text-xs text-[var(--text-secondary)]">
                  {formatCredentialDate(version.created_at)} by {version.created_by || 'system'}
                </p>
              </div>
              {canManage && !isActive && (
                <div className="flex items-center gap-2">
                  <button type="button" className="glass-button-subtle" onClick={() => onActivateVersion(version.version)} disabled={saving}>
                    Activate
                  </button>
                  <button
                    type="button"
                    className="glass-button-danger !px-2"
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
          );
        })}
        {credential.versions.length === 0 && (
          <p className="text-sm text-[var(--text-secondary)]">No value has been stored yet.</p>
        )}
      </div>

      {canManage && (
        <div className="flex flex-wrap gap-2 border-t border-[var(--border-primary)] pt-4">
          {credential.status === 'disabled' ? (
            <button type="button" className="glass-button-subtle" onClick={onEnable} disabled={saving}>
              <Power className="h-4 w-4" aria-hidden="true" />
              Enable
            </button>
          ) : (
            <button type="button" className="glass-button-subtle" onClick={onDisable} disabled={saving}>
              <ShieldAlert className="h-4 w-4" aria-hidden="true" />
              Disable
            </button>
          )}
          <button type="button" className="glass-button-danger" onClick={onDelete} disabled={saving}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            Delete credential
          </button>
        </div>
      )}
    </aside>
  );
}
