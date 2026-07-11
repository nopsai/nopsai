import type { FormEvent, ReactNode } from "react";
import { Trash2 } from "lucide-react";
import { AccessEditorEmptyState } from "./AccessModal";
import {
  AccessServiceAccountsCatalog,
  AccessUsersCatalog,
} from "./AccessEntityCatalogs";
import { BasicAccessGrantEditor } from "./BasicAccessGrantEditor";
import {
  ServiceAccountTokenPanel,
  ServiceAccountTokenReveal,
} from "./ServiceAccountTokenPanel";
import {
  isExternallyManagedUser,
  userDisplayName,
  userProviderLabel,
} from "./model";
import type {
  AccessGrantRecord,
  EditableAccessGrant,
  ServiceAccountSummary,
  ServiceAccountToken,
  UserSummary,
} from "./model";
import type {
  BasicGrantDraft,
  ServiceAccountEditorState,
  UserAccessEditorState,
} from "./panelTypes";

type BasicGrantOption = {
  value: string;
  label: string;
};

type SharedBasicGrantProps = {
  entries: EditableAccessGrant[];
  draft: BasicGrantDraft;
  options: BasicGrantOption[];
  basicGrantError: string | null;
  basicGrantSaving: boolean;
  basicGrantDirty: boolean;
  toneClassForRole: (role: string) => string;
  onDraftChange: (draft: BasicGrantDraft) => void;
  onAdd: () => void;
  onRemove: (localID: string) => void;
  onReset: () => void;
};

export type UsersWorkspaceProps = SharedBasicGrantProps & {
  users: UserSummary[];
  filteredUsers: UserSummary[];
  grantMap: Map<string, AccessGrantRecord[]>;
  selectedUserID?: string;
  loading: boolean;
  error: string | null;
  grantsLoading: boolean;
  grantsError: string | null;
  userAccessEditor: UserAccessEditorState | null;
  showUserModal: boolean;
  createUserEditor: ReactNode;
  allRoleOptions: string[];
  nextAccessRole: string;
  userRoleAssignmentsLocked: boolean;
  userRoleAssignmentsLockLabel: string;
  savingUserAccess: boolean;
  onEdit: (user: UserSummary) => void;
  onDelete: (userID: string) => void;
  onCloseEditor: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onChangeEmail: (value: string) => void;
  onChangeStatus: (value: string) => void;
  onChangePassword: (value: string) => void;
  onNextAccessRoleChange: (value: string) => void;
  onAddAccessEntry: () => void;
  onRemoveAccessEntry: (index: number) => void;
};

export function UsersWorkspace({
  users,
  filteredUsers,
  grantMap,
  selectedUserID,
  loading,
  error,
  grantsLoading,
  grantsError,
  userAccessEditor,
  showUserModal,
  createUserEditor,
  allRoleOptions,
  nextAccessRole,
  userRoleAssignmentsLocked,
  userRoleAssignmentsLockLabel,
  savingUserAccess,
  entries,
  draft,
  options,
  basicGrantError,
  basicGrantSaving,
  basicGrantDirty,
  toneClassForRole,
  onEdit,
  onDelete,
  onCloseEditor,
  onSubmit,
  onChangeEmail,
  onChangeStatus,
  onChangePassword,
  onNextAccessRoleChange,
  onAddAccessEntry,
  onRemoveAccessEntry,
  onDraftChange,
  onAdd,
  onRemove,
  onReset,
}: UsersWorkspaceProps) {
  return (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        <AccessUsersCatalog
          users={users}
          filteredUsers={filteredUsers}
          grantMap={grantMap}
          selectedUserID={selectedUserID}
          loading={loading}
          error={error}
          grantsLoading={grantsLoading}
          grantsError={grantsError}
          onEdit={onEdit}
          onDelete={onDelete}
        />
      </div>
      <aside className="access-editor-pane">
        {userAccessEditor ? (
          (() => {
            const externalManaged = isExternallyManagedUser(
              userAccessEditor.user,
            );
            const providerLabel = userProviderLabel(userAccessEditor.user);
            const displayName = userDisplayName(userAccessEditor.user);
            return (
              <div className="access-editor-surface access-editor-surface--minimal">
                <div className="access-editor-header">
                  <div>
                    <p className="access-editor-kicker">
                      {externalManaged ? "External user" : "Edit user"}
                    </p>
                    <h5 className="access-editor-title">{displayName}</h5>
                    <p className="access-editor-text">
                      {externalManaged
                        ? `This user's role assignments are managed by ${providerLabel}. Change teams in ${providerLabel} to update NopsAI access.`
                        : "Manage account details, access roles, and team-scoped basic roles."}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="access-inline-btn access-inline-btn--pill"
                    onClick={onCloseEditor}
                  >
                    Close
                  </button>
                </div>
                <form
                  className="access-editor-form access-editor-form--compact"
                  onSubmit={onSubmit}
                >
                  <div className="access-editor-grid">
                    <label className="access-minimal-label">
                      <span>Email</span>
                      <input
                        className="pipelines-input"
                        type="email"
                        value={userAccessEditor.email}
                        onChange={(event) => onChangeEmail(event.target.value)}
                        placeholder="name@example.com"
                      />
                    </label>
                    <label className="access-minimal-label">
                      <span>Status</span>
                      <select
                        className="pipelines-input"
                        value={userAccessEditor.status}
                        onChange={(event) => onChangeStatus(event.target.value)}
                        disabled={userAccessEditor.user.sub === "admin"}
                      >
                        <option value="active">Active</option>
                        <option value="disabled">Disabled</option>
                      </select>
                    </label>
                  </div>
                  <label className="access-minimal-label">
                    <span>New password</span>
                    <input
                      className="pipelines-input"
                      type="password"
                      value={userAccessEditor.password}
                      onChange={(event) => onChangePassword(event.target.value)}
                      placeholder="Leave blank to keep current password"
                    />
                  </label>
                  <AssignedRoleEditor
                    entries={userAccessEditor.entries}
                    allRoleOptions={allRoleOptions}
                    nextAccessRole={nextAccessRole}
                    locked={userRoleAssignmentsLocked}
                    lockedLabel={userRoleAssignmentsLockLabel}
                    toneClassForRole={toneClassForRole}
                    onNextAccessRoleChange={onNextAccessRoleChange}
                    onAdd={onAddAccessEntry}
                    onRemove={onRemoveAccessEntry}
                  />
                  <BasicAccessGrantEditor
                    entries={entries}
                    draft={draft}
                    options={options}
                    error={basicGrantError}
                    disabled={userRoleAssignmentsLocked}
                    saving={basicGrantSaving}
                    plain
                    countLabel={
                      userRoleAssignmentsLocked ? "Locked" : undefined
                    }
                    showGrantedBy
                    toneClassForRole={toneClassForRole}
                    onDraftChange={onDraftChange}
                    onAdd={onAdd}
                    onRemove={onRemove}
                  />
                  <div className="access-editor-footer gap-2">
                    {basicGrantDirty && (
                      <button
                        type="button"
                        className="access-inline-btn access-inline-btn--pill"
                        onClick={onReset}
                        disabled={
                          userRoleAssignmentsLocked ||
                          basicGrantSaving ||
                          savingUserAccess
                        }
                      >
                        Reset basic roles
                      </button>
                    )}
                    <button
                      type="submit"
                      className="glass-button-primary"
                      disabled={savingUserAccess || basicGrantSaving}
                    >
                      {savingUserAccess || basicGrantSaving
                        ? "Saving…"
                        : "Save changes"}
                    </button>
                  </div>
                </form>
              </div>
            );
          })()
        ) : showUserModal ? (
          createUserEditor
        ) : (
          <AccessEditorEmptyState
            sectionLabel="User details"
            hint="Select a user to edit access."
          />
        )}
      </aside>
    </div>
  );
}

export type ServiceAccountsWorkspaceProps = SharedBasicGrantProps & {
  accounts: ServiceAccountSummary[];
  filteredAccounts: ServiceAccountSummary[];
  grantMap: Map<string, AccessGrantRecord[]>;
  selectedAccountID?: string;
  loading: boolean;
  error: string | null;
  grantsLoading: boolean;
  grantsError: string | null;
  serviceAccountEditor: ServiceAccountEditorState | null;
  showServiceAccountModal: boolean;
  createServiceAccountEditor: ReactNode;
  allRoleOptions: string[];
  nextAccessRole: string;
  createdToken: ServiceAccountToken | null;
  copyTokenLabel: string;
  savingServiceAccountAccess: boolean;
  onEdit: (account: ServiceAccountSummary) => void;
  onDelete: (serviceAccountID: string) => void;
  onCloseEditor: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onChangeEmail: (value: string) => void;
  onChangeStatus: (value: string) => void;
  onChangeTokenName: (value: string) => void;
  onCreateToken: () => void;
  onRevokeToken: (tokenID: string) => void;
  onCopyToken: () => void;
  onNextAccessRoleChange: (value: string) => void;
  onAddAccessEntry: () => void;
  onRemoveAccessEntry: (index: number) => void;
};

export function ServiceAccountsWorkspace({
  accounts,
  filteredAccounts,
  grantMap,
  selectedAccountID,
  loading,
  error,
  grantsLoading,
  grantsError,
  serviceAccountEditor,
  showServiceAccountModal,
  createServiceAccountEditor,
  allRoleOptions,
  nextAccessRole,
  createdToken,
  copyTokenLabel,
  savingServiceAccountAccess,
  entries,
  draft,
  options,
  basicGrantError,
  basicGrantSaving,
  basicGrantDirty,
  toneClassForRole,
  onEdit,
  onDelete,
  onCloseEditor,
  onSubmit,
  onChangeEmail,
  onChangeStatus,
  onChangeTokenName,
  onCreateToken,
  onRevokeToken,
  onCopyToken,
  onNextAccessRoleChange,
  onAddAccessEntry,
  onRemoveAccessEntry,
  onDraftChange,
  onAdd,
  onRemove,
  onReset,
}: ServiceAccountsWorkspaceProps) {
  return (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        <AccessServiceAccountsCatalog
          accounts={accounts}
          filteredAccounts={filteredAccounts}
          grantMap={grantMap}
          selectedAccountID={selectedAccountID}
          loading={loading}
          error={error}
          grantsLoading={grantsLoading}
          grantsError={grantsError}
          onEdit={onEdit}
          onDelete={onDelete}
        />
      </div>
      <aside className="access-editor-pane">
        {serviceAccountEditor ? (
          <div className="access-editor-surface access-editor-surface--minimal">
            <div className="access-editor-header">
              <div>
                <p className="access-editor-kicker">Edit service account</p>
                <h5 className="access-editor-title">
                  {serviceAccountEditor.account.sub}
                </h5>
                <p className="access-editor-text">
                  Manage token-only integration access and scoped basic roles.
                </p>
              </div>
              <button
                type="button"
                className="access-inline-btn access-inline-btn--pill"
                onClick={onCloseEditor}
              >
                Close
              </button>
            </div>
            <ServiceAccountTokenReveal
              token={createdToken}
              copyLabel={copyTokenLabel}
              onCopy={onCopyToken}
            />
            <form
              className="access-editor-form access-editor-form--compact"
              onSubmit={onSubmit}
            >
              <div className="access-editor-grid">
                <label className="access-minimal-label">
                  <span>Contact email</span>
                  <input
                    className="pipelines-input"
                    type="email"
                    value={serviceAccountEditor.email}
                    onChange={(event) => onChangeEmail(event.target.value)}
                    placeholder="platform@example.com"
                  />
                </label>
                <label className="access-minimal-label">
                  <span>Status</span>
                  <select
                    className="pipelines-input"
                    value={serviceAccountEditor.status}
                    onChange={(event) => onChangeStatus(event.target.value)}
                  >
                    <option value="active">Active</option>
                    <option value="disabled">Disabled</option>
                  </select>
                </label>
              </div>
              <ServiceAccountTokenPanel
                tokens={serviceAccountEditor.tokens}
                loading={serviceAccountEditor.tokensLoading}
                error={serviceAccountEditor.tokensError}
                tokenName={serviceAccountEditor.tokenName}
                onTokenNameChange={onChangeTokenName}
                onCreate={onCreateToken}
                onRevoke={onRevokeToken}
              />
              <AssignedRoleEditor
                entries={serviceAccountEditor.entries}
                allRoleOptions={allRoleOptions}
                nextAccessRole={nextAccessRole}
                locked={false}
                lockedLabel="Remove assignment"
                toneClassForRole={toneClassForRole}
                onNextAccessRoleChange={onNextAccessRoleChange}
                onAdd={onAddAccessEntry}
                onRemove={onRemoveAccessEntry}
              />
              <BasicAccessGrantEditor
                entries={entries}
                draft={draft}
                options={options}
                error={basicGrantError}
                saving={basicGrantSaving}
                plain
                showGrantedBy
                toneClassForRole={toneClassForRole}
                onDraftChange={onDraftChange}
                onAdd={onAdd}
                onRemove={onRemove}
              />
              <div className="access-editor-footer gap-2">
                {basicGrantDirty && (
                  <button
                    type="button"
                    className="access-inline-btn access-inline-btn--pill"
                    onClick={onReset}
                    disabled={basicGrantSaving || savingServiceAccountAccess}
                  >
                    Reset basic roles
                  </button>
                )}
                <button
                  type="submit"
                  className="glass-button-primary"
                  disabled={savingServiceAccountAccess || basicGrantSaving}
                >
                  {savingServiceAccountAccess || basicGrantSaving
                    ? "Saving…"
                    : "Save changes"}
                </button>
              </div>
            </form>
          </div>
        ) : showServiceAccountModal ? (
          createServiceAccountEditor
        ) : (
          <AccessEditorEmptyState
            sectionLabel="Service account details"
            hint="Select a service account to edit access and tokens."
          />
        )}
      </aside>
    </div>
  );
}

function AssignedRoleEditor({
  entries,
  allRoleOptions,
  nextAccessRole,
  locked,
  lockedLabel,
  toneClassForRole,
  onNextAccessRoleChange,
  onAdd,
  onRemove,
}: {
  entries: string[];
  allRoleOptions: string[];
  nextAccessRole: string;
  locked: boolean;
  lockedLabel: string;
  toneClassForRole: (role: string) => string;
  onNextAccessRoleChange: (value: string) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}) {
  return (
    <div className="access-editor-section access-editor-section--plain">
      <div className="access-minimal-section__header">
        <p className="text-sm font-medium text-[var(--text-primary)]">
          Access roles
        </p>
        <span className="text-[11px] text-[var(--text-secondary)]">
          {locked ? "Locked" : `${entries.length} assigned`}
        </span>
      </div>
      <div className="space-y-2">
        {entries.length === 0 && (
          <p className="text-[12px] text-[var(--text-secondary)]">
            No roles assigned yet.
          </p>
        )}
        {entries.map((entry, index) => {
          const label = locked ? lockedLabel : "Remove assignment";
          return (
            <div
              key={`assigned-role-${index}`}
              className="access-minimal-row justify-between"
            >
              <span className={`access-chip ${toneClassForRole(entry)}`}>
                {entry || "Role"}
              </span>
              <button
                type="button"
                className={`access-inline-btn access-inline-btn--danger access-role-remove ${locked ? "opacity-60 cursor-not-allowed" : ""}`}
                onClick={() => onRemove(index)}
                title={label}
                aria-label={label}
                disabled={locked}
              >
                <TrashIcon />
              </button>
            </div>
          );
        })}
        <div className="access-editor-inline-add">
          <select
            className="pipelines-input w-full"
            value={nextAccessRole}
            onChange={(event) => onNextAccessRoleChange(event.target.value)}
            disabled={locked}
          >
            <option value="">
              {allRoleOptions.length === 0
                ? "No roles available"
                : "Select a role"}
            </option>
            {allRoleOptions.map((role) => (
              <option key={`access-role-${role}`} value={role}>
                {role}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="glass-button-subtle"
            onClick={onAdd}
            disabled={locked || !nextAccessRole || allRoleOptions.length === 0}
          >
            Add
          </button>
        </div>
      </div>
    </div>
  );
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}
