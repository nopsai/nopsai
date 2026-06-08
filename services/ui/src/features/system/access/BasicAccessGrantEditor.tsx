import {
  BASIC_ROLE_ADMIN,
  BASIC_ROLE_DEVELOPER,
  BASIC_ROLE_OWNER,
  BASIC_ROLE_VIEWER,
  ROOT_ACCESS_SCOPE,
  accessGrantResourceSummary,
  basicAccessGrantDescription,
  isRootAccessScopeID,
} from './model';
import { isBasicGrantDraftDuplicate, type BasicGrantDraft } from './basicGrantModel';
import type { EditableAccessGrant } from './model';

type BasicGrantOption = {
  value: string;
  label: string;
};

export function BasicAccessGrantEditor({
  entries,
  draft,
  options,
  error,
  disabled = false,
  saving = false,
  plain = false,
  addLabel = 'Add',
  countLabel,
  showGrantedBy = false,
  toneClassForRole,
  onDraftChange,
  onAdd,
  onRemove,
}: {
  entries: EditableAccessGrant[];
  draft: BasicGrantDraft;
  options: BasicGrantOption[];
  error: string | null;
  disabled?: boolean;
  saving?: boolean;
  plain?: boolean;
  addLabel?: string;
  countLabel?: string;
  showGrantedBy?: boolean;
  toneClassForRole: (role: string) => string;
  onDraftChange: (draft: BasicGrantDraft) => void;
  onAdd: () => void;
  onRemove: (localID: string) => void;
}) {
  const adminSelected = draft.role === BASIC_ROLE_ADMIN;
  const duplicate = isBasicGrantDraftDuplicate(draft, entries);
  return (
    <div className={`access-editor-section ${plain ? 'access-editor-section--plain' : ''}`}>
      <div className="access-minimal-section__header">
        <p className="text-sm font-medium text-[var(--text-primary)]">Basic roles</p>
        <span className="text-[11px] text-[var(--text-secondary)]">
          {countLabel ?? `${entries.length} listed`}
        </span>
      </div>
      <div className="access-editor-grid">
        <label className="access-minimal-label">
          <span>Access level</span>
          <select
            className="pipelines-input"
            value={draft.role}
            onChange={event => {
              const role = event.target.value;
              onDraftChange({
                ...draft,
                role,
                scope: role === BASIC_ROLE_ADMIN ? draft.scope : draft.scope || ROOT_ACCESS_SCOPE,
              });
            }}
            disabled={disabled}
          >
            <option value="">Select role</option>
            <option value={BASIC_ROLE_VIEWER}>Viewer</option>
            <option value={BASIC_ROLE_DEVELOPER}>Developer</option>
            <option value={BASIC_ROLE_OWNER}>Owner</option>
            <option value={BASIC_ROLE_ADMIN}>Admin</option>
          </select>
        </label>
        <label className="access-minimal-label">
          <span>Group target</span>
          <select
            className="pipelines-input"
            value={adminSelected ? 'platform' : draft.scope}
            onChange={event => onDraftChange({ ...draft, scope: event.target.value })}
            disabled={disabled || adminSelected}
          >
            {adminSelected ? (
              <option value="platform">Platform wide</option>
            ) : (
              options.map(option => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))
            )}
          </select>
        </label>
      </div>
      <div className="access-editor-footer access-editor-footer--inline">
        <button
          type="button"
          className="glass-button-subtle"
          onClick={onAdd}
          disabled={disabled || saving || !draft.role || duplicate}
        >
          {addLabel}
        </button>
      </div>
      {error && <div className="access-error-banner">{error}</div>}
      <div className="space-y-2">
        {entries.length === 0 ? (
          <p className="text-[12px] text-[var(--text-secondary)]">No basic roles listed.</p>
        ) : (
          entries.map(grant => (
            <div key={grant.localID} className="access-minimal-row access-minimal-row--stack">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className={`access-chip ${toneClassForRole(grant.role)}`}>{grant.role}</span>
                  <span className="access-chip access-chip--muted">{accessGrantResourceSummary(grant)}</span>
                  {grant.inherit && grant.resourceType === 'folder' && !isRootAccessScopeID(grant.resourceID) && (
                    <span className="access-chip access-chip--muted">Includes children</span>
                  )}
                </div>
                <p className="text-[11px] text-[var(--text-secondary)] mt-2">
                  {basicAccessGrantDescription(grant)}
                  {showGrantedBy && grant.grantedBy ? ` Granted by ${grant.grantedBy}.` : ''}
                </p>
              </div>
              <button
                type="button"
                className="access-inline-btn access-inline-btn--danger"
                onClick={() => onRemove(grant.localID)}
                disabled={disabled || saving}
              >
                <TrashIcon />
                <span>Remove</span>
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4" aria-hidden="true">
      <path d="M4 7h16" />
      <path d="M9 7V4h6v3" />
      <path d="m6 7 1 13h10l1-13" />
      <path d="M10 11v5M14 11v5" />
    </svg>
  );
}
