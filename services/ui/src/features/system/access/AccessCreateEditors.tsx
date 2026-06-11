import type { FormEvent } from 'react';
import { Trash2 } from 'lucide-react';
import { BasicAccessGrantEditor } from './BasicAccessGrantEditor';
import { ServiceAccountTokenReveal } from './ServiceAccountTokenPanel';
import type { EditableAccessGrant, ServiceAccountToken } from './model';
import type { BasicGrantDraft, NewServiceAccountFormState, NewUserFormState } from './panelTypes';

type BasicGrantOption = {
  value: string;
  label: string;
};

type CommonCreateEditorProps = {
  allRoleOptions: string[];
  nextRole: string;
  onNextRoleChange: (value: string) => void;
  basicGrantEntries: EditableAccessGrant[];
  basicGrantDraft: BasicGrantDraft;
  basicGrantOptions: BasicGrantOption[];
  basicGrantError: string | null;
  toneClassForRole: (role: string) => string;
  onBasicGrantDraftChange: (draft: BasicGrantDraft) => void;
  onAddBasicGrant: () => void;
  onRemoveBasicGrant: (localID: string) => void;
};

export type CreateUserEditorProps = CommonCreateEditorProps & {
  newUser: NewUserFormState;
  creating: boolean;
  onChangeUser: (next: NewUserFormState) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
  onUpdateRoleEntry: (index: number, value: string) => void;
  onRemoveRoleEntry: (index: number) => void;
  onAppendRole: () => void;
};

export function CreateUserEditor({
  newUser,
  creating,
  allRoleOptions,
  nextRole,
  basicGrantEntries,
  basicGrantDraft,
  basicGrantOptions,
  basicGrantError,
  toneClassForRole,
  onChangeUser,
  onSubmit,
  onClose,
  onUpdateRoleEntry,
  onRemoveRoleEntry,
  onNextRoleChange,
  onAppendRole,
  onBasicGrantDraftChange,
  onAddBasicGrant,
  onRemoveBasicGrant,
}: CreateUserEditorProps) {
  return (
    <div className="access-editor-surface">
      <div className="access-editor-header">
        <div>
          <p className="access-editor-kicker">Create user</p>
          <h5 className="access-editor-title">New local account</h5>
          <p className="access-editor-text">Create a local account.</p>
        </div>
        <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onClose}>
          Close
        </button>
      </div>
      <form className="access-editor-form" onSubmit={onSubmit}>
        <div className="access-editor-grid">
          <label className="access-minimal-label">
            <span>Username (sub)</span>
            <input
              className="pipelines-input"
              value={newUser.sub}
              onChange={event => onChangeUser({ ...newUser, sub: event.target.value })}
              placeholder="alice"
              required
            />
          </label>
          <label className="access-minimal-label">
            <span>Email</span>
            <input
              className="pipelines-input"
              type="email"
              value={newUser.email}
              onChange={event => onChangeUser({ ...newUser, email: event.target.value })}
              placeholder="name@example.com"
            />
          </label>
        </div>
        <label className="access-minimal-label">
          <span>Password</span>
          <input
            className="pipelines-input"
            type="password"
            value={newUser.password}
            onChange={event => onChangeUser({ ...newUser, password: event.target.value })}
            placeholder="••••••••"
            required
          />
        </label>
        <CreateAccessRolePicker
          entries={newUser.roles}
          emptyLabel="Add access roles here or use basic roles below."
          allRoleOptions={allRoleOptions}
          nextRole={nextRole}
          onNextRoleChange={onNextRoleChange}
          onUpdateRoleEntry={onUpdateRoleEntry}
          onRemoveRoleEntry={onRemoveRoleEntry}
          onAppendRole={onAppendRole}
        />
        <BasicAccessGrantEditor
          entries={basicGrantEntries}
          draft={basicGrantDraft}
          options={basicGrantOptions}
          error={basicGrantError}
          saving={creating}
          addLabel="Add basic role"
          toneClassForRole={toneClassForRole}
          onDraftChange={onBasicGrantDraftChange}
          onAdd={onAddBasicGrant}
          onRemove={onRemoveBasicGrant}
        />
        <div className="access-editor-footer">
          <button type="submit" className="glass-button-primary" disabled={creating}>
            {creating ? 'Saving…' : 'Save user'}
          </button>
        </div>
      </form>
    </div>
  );
}

export type CreateServiceAccountEditorProps = CommonCreateEditorProps & {
  newServiceAccount: NewServiceAccountFormState;
  createdToken: ServiceAccountToken | null;
  copyTokenLabel: string;
  creating: boolean;
  onChangeServiceAccount: (next: NewServiceAccountFormState) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
  onCopyToken: () => void;
  onUpdateRoleEntry: (index: number, value: string) => void;
  onRemoveRoleEntry: (index: number) => void;
  onAppendRole: () => void;
};

export function CreateServiceAccountEditor({
  newServiceAccount,
  createdToken,
  copyTokenLabel,
  creating,
  allRoleOptions,
  nextRole,
  basicGrantEntries,
  basicGrantDraft,
  basicGrantOptions,
  basicGrantError,
  toneClassForRole,
  onChangeServiceAccount,
  onSubmit,
  onClose,
  onCopyToken,
  onUpdateRoleEntry,
  onRemoveRoleEntry,
  onNextRoleChange,
  onAppendRole,
  onBasicGrantDraftChange,
  onAddBasicGrant,
  onRemoveBasicGrant,
}: CreateServiceAccountEditorProps) {
  return (
    <div className="access-editor-surface">
      <div className="access-editor-header">
        <div>
          <p className="access-editor-kicker">Create service account</p>
          <h5 className="access-editor-title">New integration identity</h5>
          <p className="access-editor-text">Create an account that authenticates only with service account tokens.</p>
        </div>
        <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onClose}>
          Close
        </button>
      </div>
      <ServiceAccountTokenReveal token={createdToken} copyLabel={copyTokenLabel} onCopy={onCopyToken} />
      {!createdToken && (
        <form className="access-editor-form" onSubmit={onSubmit}>
          <div className="access-editor-grid">
            <label className="access-minimal-label">
              <span>Service account ID</span>
              <input
                className="pipelines-input"
                value={newServiceAccount.sub}
                onChange={event => onChangeServiceAccount({ ...newServiceAccount, sub: event.target.value })}
                placeholder="deploy-bot"
                required
              />
            </label>
            <label className="access-minimal-label">
              <span>Contact email</span>
              <input
                className="pipelines-input"
                type="email"
                value={newServiceAccount.email}
                onChange={event => onChangeServiceAccount({ ...newServiceAccount, email: event.target.value })}
                placeholder="platform@example.com"
              />
            </label>
          </div>
          <label className="access-minimal-label">
            <span>Initial token name</span>
            <input
              className="pipelines-input"
              value={newServiceAccount.tokenName}
              onChange={event => onChangeServiceAccount({ ...newServiceAccount, tokenName: event.target.value })}
              placeholder="default"
              required
            />
          </label>
          <CreateAccessRolePicker
            entries={newServiceAccount.roles}
            emptyLabel="Add access roles here or use basic roles below."
            allRoleOptions={allRoleOptions}
            nextRole={nextRole}
            onNextRoleChange={onNextRoleChange}
            onUpdateRoleEntry={onUpdateRoleEntry}
            onRemoveRoleEntry={onRemoveRoleEntry}
            onAppendRole={onAppendRole}
          />
          <BasicAccessGrantEditor
            entries={basicGrantEntries}
            draft={basicGrantDraft}
            options={basicGrantOptions}
            error={basicGrantError}
            saving={creating}
            addLabel="Add basic role"
            toneClassForRole={toneClassForRole}
            onDraftChange={onBasicGrantDraftChange}
            onAdd={onAddBasicGrant}
            onRemove={onRemoveBasicGrant}
          />
          <div className="access-editor-footer">
            <button type="submit" className="glass-button-primary" disabled={creating}>
              {creating ? 'Saving…' : 'Save service account'}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

function CreateAccessRolePicker({
  entries,
  emptyLabel,
  allRoleOptions,
  nextRole,
  onNextRoleChange,
  onUpdateRoleEntry,
  onRemoveRoleEntry,
  onAppendRole,
}: {
  entries: string[];
  emptyLabel: string;
  allRoleOptions: string[];
  nextRole: string;
  onNextRoleChange: (value: string) => void;
  onUpdateRoleEntry: (index: number, value: string) => void;
  onRemoveRoleEntry: (index: number) => void;
  onAppendRole: () => void;
}) {
  return (
    <div className="access-editor-section">
      <div className="access-minimal-section__header">
        <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
        <span className="text-[11px] text-[var(--text-secondary)]">Optional with basic roles</span>
      </div>
      <div className="space-y-2">
        {entries.length === 0 && <p className="text-[11px] text-[var(--text-secondary)]">{emptyLabel}</p>}
        {entries.map((entry, index) => (
          <div key={`new-access-role-${index}`} className="access-minimal-row">
            <select
              className="pipelines-input flex-1"
              value={entry}
              onChange={event => onUpdateRoleEntry(index, event.target.value)}
              required
              disabled={allRoleOptions.length === 0}
            >
              <option value="" disabled>
                {allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}
              </option>
              {allRoleOptions.map(role => (
                <option key={`role-opt-${role}`} value={role}>
                  {role}
                </option>
              ))}
            </select>
            <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => onRemoveRoleEntry(index)} title="Remove role">
              <TrashIcon />
            </button>
          </div>
        ))}
        <div className="access-editor-inline-add">
          <select
            className="pipelines-input flex-1"
            value={nextRole}
            onChange={event => onNextRoleChange(event.target.value)}
            disabled={allRoleOptions.length === 0}
          >
            <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}</option>
            {allRoleOptions.map(role => (
              <option key={`new-role-opt-${role}`} value={role}>
                {role}
              </option>
            ))}
          </select>
          <button type="button" className="glass-button-subtle" onClick={onAppendRole} disabled={allRoleOptions.length === 0}>
            Add access role
          </button>
        </div>
      </div>
    </div>
  );
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}
