import type { FormEvent, ReactNode } from "react";
import { Trash2 } from "lucide-react";
import { AccessEditorDrawer } from "./AccessEditorDrawer";
import {
  AccessServiceAccountsCatalog,
  AccessUsersCatalog,
} from "./AccessEntityCatalogs";
import {
  AccessFormCard,
  AccessReviewRow,
  AccessReviewStat,
  AccessSectionedEditor,
  type AccessEditorSection,
} from "./AccessSectionedEditor";
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

function titleCase(value: string) {
  return (value || "unknown")
    .replace(/[-_]/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

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
  onCloseCreate: () => void;
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
  onCloseCreate,
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
  const editorContent = userAccessEditor
    ? (() => {
        const externalManaged = isExternallyManagedUser(userAccessEditor.user);
        const providerLabel = userProviderLabel(userAccessEditor.user);
        const displayName = userDisplayName(userAccessEditor.user);
        const identityTeamCount =
          (userAccessEditor.user.external_teams || []).length +
          (userAccessEditor.user.external_auth_teams || []).length;
        const resetBasicRolesAction = basicGrantDirty ? (
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
        ) : null;
        const sections: AccessEditorSection[] = [
          {
            id: "details",
            label: "Profile",
            description: "Identity and state",
            children: (
              <>
                <AccessFormCard
                  title="Identity"
                  description="The fields administrators use to recognize and contact this person."
                  badge={externalManaged ? "External" : "Local"}
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
                </AccessFormCard>
                <AccessFormCard
                  title="Authentication"
                  description="Authentication source stays explicit so operators know who owns lifecycle and profile claims."
                  badge={providerLabel}
                >
                  <dl className="access-review-list">
                    <AccessReviewRow
                      label="Subject"
                      value={userAccessEditor.user.sub || displayName}
                    />
                    <AccessReviewRow
                      label="Source"
                      value={
                        externalManaged
                          ? `Authenticated by ${providerLabel}`
                          : "Platform-managed local account"
                      }
                    />
                    <AccessReviewRow
                      label="Identity teams"
                      value={String(identityTeamCount)}
                    />
                  </dl>
                </AccessFormCard>
              </>
            ),
          },
          {
            id: "access",
            label: "Access",
            description: "Roles and scopes",
            children: (
              <>
                <AccessFormCard
                  title="Access roles"
                  description="Reusable bundles for low-level permissions."
                  badge={
                    userRoleAssignmentsLocked
                      ? "Locked"
                      : `${userAccessEditor.entries.length} assigned`
                  }
                >
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
                </AccessFormCard>
                <AccessFormCard
                  title="Basic roles"
                  description="Simple product roles with an explicit team target."
                  badge={userRoleAssignmentsLocked ? "Locked" : `${entries.length} listed`}
                >
                  <BasicAccessGrantEditor
                    entries={entries}
                    draft={draft}
                    options={options}
                    error={basicGrantError}
                    disabled={userRoleAssignmentsLocked}
                    saving={basicGrantSaving}
                    plain
                    countLabel={userRoleAssignmentsLocked ? "Locked" : undefined}
                    showGrantedBy
                    toneClassForRole={toneClassForRole}
                    onDraftChange={onDraftChange}
                    onAdd={onAdd}
                    onRemove={onRemove}
                  />
                </AccessFormCard>
              </>
            ),
          },
          {
            id: "review",
            label: "Review",
            description: "Save impact",
            children: (
              <>
                <div className="access-review-grid">
                  <AccessReviewStat label="Basic roles" value={entries.length} />
                  <AccessReviewStat
                    label="Access roles"
                    value={userAccessEditor.entries.length}
                  />
                  <AccessReviewStat
                    label="Identity teams"
                    value={identityTeamCount}
                  />
                </div>
                <AccessFormCard
                  title="Summary"
                  description="The account state that will be written through the access API."
                  badge={titleCase(userAccessEditor.status)}
                >
                  <dl className="access-review-list">
                    <AccessReviewRow label="User" value={displayName} />
                    <AccessReviewRow
                      label="Email"
                      value={userAccessEditor.email || "No email address"}
                    />
                    <AccessReviewRow
                      label="Authentication"
                      value={externalManaged ? providerLabel : "Local account"}
                    />
                    <AccessReviewRow
                      label="Status"
                      value={titleCase(userAccessEditor.status)}
                    />
                  </dl>
                </AccessFormCard>
              </>
            ),
          },
        ];
        return (
          <AccessSectionedEditor
            modeLabel="Edit"
            entityLabel={externalManaged ? "External user" : "User"}
            title={displayName}
            subtitle={
              externalManaged
                ? `Authenticated by ${providerLabel}. Local access can still be managed here.`
                : "Manage account details, access roles, and team-scoped basic roles."
            }
            icon={(displayName || userAccessEditor.email || "U").charAt(0).toUpperCase()}
            sections={sections}
            resetKey={`user-${userAccessEditor.user.id}`}
            saveLabel="Save changes"
            savingLabel="Saving..."
            saving={savingUserAccess || basicGrantSaving}
            deleteLabel="Delete user"
            deleteDisabled={userAccessEditor.user.sub === "admin"}
            secondaryFooterAction={resetBasicRolesAction}
            onClose={onCloseEditor}
            onDelete={() => {
              onCloseEditor();
              onDelete(userAccessEditor.user.id);
            }}
            onSubmit={onSubmit}
          />
        );
      })()
    : showUserModal
      ? createUserEditor
      : null;
  const closeDrawer = userAccessEditor ? onCloseEditor : onCloseCreate;

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
      <AccessEditorDrawer
        open={Boolean(editorContent)}
        label="User editor"
        onClose={closeDrawer}
      >
        {editorContent}
      </AccessEditorDrawer>
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
  onCloseCreate: () => void;
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
  onCloseCreate,
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
  const editorContent = serviceAccountEditor ? (
    (() => {
      const resetBasicRolesAction = basicGrantDirty ? (
        <button
          type="button"
          className="access-inline-btn access-inline-btn--pill"
          onClick={onReset}
          disabled={basicGrantSaving || savingServiceAccountAccess}
        >
          Reset basic roles
        </button>
      ) : null;
      const sections: AccessEditorSection[] = [
        {
          id: "details",
          label: "Account",
          description: "Owner and state",
          children: (
            <>
              <ServiceAccountTokenReveal
                token={createdToken}
                copyLabel={copyTokenLabel}
                onCopy={onCopyToken}
              />
              <AccessFormCard
                title="Account details"
                description="Contact, provider, and lifecycle state for this token-only identity."
                badge={titleCase(serviceAccountEditor.status)}
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
                  <label className="access-minimal-label">
                    <span>Provider</span>
                    <input
                      className="pipelines-input"
                      value={serviceAccountEditor.account.provider || "service-account"}
                      readOnly
                    />
                  </label>
                </div>
              </AccessFormCard>
            </>
          ),
        },
        {
          id: "credentials",
          label: "Credentials",
          description: "Tokens",
          children: (
            <AccessFormCard
              title="Tokens"
              description="Issue or revoke service-account tokens. Generated tokens are shown only once."
              badge={`${serviceAccountEditor.tokens.length} active`}
            >
              <ServiceAccountTokenPanel
                tokens={serviceAccountEditor.tokens}
                loading={serviceAccountEditor.tokensLoading}
                error={serviceAccountEditor.tokensError}
                tokenName={serviceAccountEditor.tokenName}
                onTokenNameChange={onChangeTokenName}
                onCreate={onCreateToken}
                onRevoke={onRevokeToken}
              />
            </AccessFormCard>
          ),
        },
        {
          id: "access",
          label: "Access",
          description: "Roles and scopes",
          children: (
            <>
              <AccessFormCard
                title="Access roles"
                description="Reusable permissions assigned to this service identity."
                badge={`${serviceAccountEditor.entries.length} assigned`}
              >
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
              </AccessFormCard>
              <AccessFormCard
                title="Basic roles"
                description="Team-scoped product roles granted to the service identity."
                badge={`${entries.length} listed`}
              >
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
              </AccessFormCard>
            </>
          ),
        },
        {
          id: "review",
          label: "Review",
          description: "Save impact",
          children: (
            <>
              <div className="access-review-grid">
                <AccessReviewStat label="Basic roles" value={entries.length} />
                <AccessReviewStat
                  label="Access roles"
                  value={serviceAccountEditor.entries.length}
                />
                <AccessReviewStat
                  label="Tokens"
                  value={serviceAccountEditor.tokens.length}
                />
              </div>
              <AccessFormCard
                title="Summary"
                description="Machine identity configuration."
                badge={titleCase(serviceAccountEditor.status)}
              >
                <dl className="access-review-list">
                  <AccessReviewRow
                    label="Name"
                    value={serviceAccountEditor.account.sub}
                  />
                  <AccessReviewRow
                    label="Contact"
                    value={serviceAccountEditor.email || "No contact email"}
                  />
                  <AccessReviewRow
                    label="Status"
                    value={titleCase(serviceAccountEditor.status)}
                  />
                  <AccessReviewRow
                    label="Token activity"
                    value={
                      serviceAccountEditor.account.last_used_at
                        ? serviceAccountEditor.account.last_used_at
                        : "No token activity"
                    }
                  />
                </dl>
              </AccessFormCard>
            </>
          ),
        },
      ];

      return (
        <AccessSectionedEditor
          modeLabel="Edit"
          entityLabel="Service account"
          title={serviceAccountEditor.account.sub}
          subtitle="Manage token-only integration access and scoped basic roles."
          icon="SA"
          sections={sections}
          resetKey={`service-account-${serviceAccountEditor.account.id}`}
          saveLabel="Save changes"
          savingLabel="Saving..."
          saving={savingServiceAccountAccess || basicGrantSaving}
          deleteLabel="Delete service account"
          secondaryFooterAction={resetBasicRolesAction}
          onClose={onCloseEditor}
          onDelete={() => {
            onCloseEditor();
            onDelete(serviceAccountEditor.account.id);
          }}
          onSubmit={onSubmit}
        />
      );
    })()
  ) : showServiceAccountModal ? (
    createServiceAccountEditor
  ) : null;
  const closeDrawer = serviceAccountEditor ? onCloseEditor : onCloseCreate;

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
      <AccessEditorDrawer
        open={Boolean(editorContent)}
        label="Service account editor"
        onClose={closeDrawer}
      >
        {editorContent}
      </AccessEditorDrawer>
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
