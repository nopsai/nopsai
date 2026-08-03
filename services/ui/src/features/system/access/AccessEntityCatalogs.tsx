import type { KeyboardEvent, MouseEvent, ReactNode } from "react";
import { Edit3, Server, Trash2 } from "lucide-react";
import {
  basicAccessGrantLabel,
  isExternallyManagedUser,
  isProtectedAccessRole,
  policyLabel,
  userDisplayName,
  userEmailVerificationLabel,
  userProviderLabel,
  userSubjectLabel,
  type AccessGrantRecord,
  type RoleDefinition,
  type RolePermission,
  type ServiceAccountSummary,
  type UserSummary,
} from "./model";
import {
  formatAccessActionSummary,
  formatAccessResourceSummary,
  parseAAAActionValue,
  summarizeRoleCoverage,
} from "./policyRuleModel";
import {
  accessPresetForRole,
  accessPresetToneClass,
  formatAccessCount,
  formatAccessTimestamp,
} from "./presentation";

type AccessChipItem = {
  id: string;
  label: string;
  className?: string;
  title?: string;
};

function statusKey(value: string) {
  const key = (value || "").toLowerCase();
  if (key.includes("active") || key.includes("enabled")) return "ok";
  if (key.includes("pending") || key.includes("invited")) return "warn";
  if (
    key.includes("blocked") ||
    key.includes("disabled") ||
    key.includes("suspended")
  ) {
    return "danger";
  }
  return "muted";
}

function titleCase(value: string) {
  return (value || "unknown")
    .replace(/[-_]/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function rowActivationHandler(onActivate: () => void) {
  return (event: KeyboardEvent<HTMLTableRowElement>) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    onActivate();
  };
}

function stopRowAction(
  event: MouseEvent<HTMLButtonElement>,
  action: () => void,
) {
  event.stopPropagation();
  action();
}

function CatalogEmptyState({
  title,
  detail,
}: {
  title: string;
  detail: string;
}) {
  return (
    <div className="access-empty-card">
      <p className="font-medium text-[var(--text-primary)]">{title}</p>
      <p className="text-sm text-[var(--text-secondary)]">{detail}</p>
    </div>
  );
}

function AccessCatalogTable({
  label,
  columns,
  children,
  minWidth = 900,
}: {
  label: string;
  columns: string[];
  children: ReactNode;
  minWidth?: number;
}) {
  return (
    <div className="access-table-shell" aria-label={label}>
      <div className="access-table-wrap">
        <table className="access-table" style={{ minWidth }}>
          <thead>
            <tr>
              {columns.map((column) => (
                <th
                  key={column}
                  className={column === "Actions" ? "access-table__right" : ""}
                >
                  {column}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>{children}</tbody>
        </table>
      </div>
    </div>
  );
}

function AccessEntityCell({
  avatar,
  service = false,
  title,
  status,
  meta,
  detail,
}: {
  avatar: ReactNode;
  service?: boolean;
  title: ReactNode;
  status?: string;
  meta: ReactNode;
  detail?: ReactNode;
}) {
  const titleContent = status ? (
    <span className="access-table-title-line">
      {title}
      <AccessStatusDot value={status} />
    </span>
  ) : (
    title
  );

  return (
    <div className="access-table-entity">
      <div className={`access-avatar ${service ? "access-avatar--service" : ""}`}>
        {avatar}
      </div>
      <div className="access-table-entity__copy">
        <div className="access-table-entity__title">{titleContent}</div>
        <div className="access-table-entity__meta">{meta}</div>
        {detail ? <div className="access-table-entity__detail">{detail}</div> : null}
      </div>
    </div>
  );
}

function AccessStatusDot({ value }: { value: string }) {
  const label = titleCase(value || "unknown");
  return (
    <span
      className={`access-status-dot access-status-dot--${statusKey(value)}`}
      title={label}
      aria-label={`Status: ${label}`}
    />
  );
}

function AccessChipList({
  items,
  emptyLabel = "None",
  limit = 3,
}: {
  items: AccessChipItem[];
  emptyLabel?: string;
  limit?: number;
}) {
  if (!items.length) {
    return <span className="access-cell-muted">{emptyLabel}</span>;
  }
  const visible = items.slice(0, limit);
  const remaining = items.length - visible.length;
  return (
    <div
      className="access-chip-list access-chip-list--compact"
      title={items.map((item) => item.title || item.label).join(", ")}
    >
      {visible.map((item) => (
        <span
          key={item.id}
          className={`access-chip ${item.className || "access-chip--muted"}`}
        >
          {item.label}
        </span>
      ))}
      {remaining > 0 && (
        <span className="access-chip access-chip--more">+{remaining}</span>
      )}
    </div>
  );
}

function roleChipItems(
  ownerID: string,
  roles: Array<{ role: string }> = [],
): AccessChipItem[] {
  return roles.map((role, index) => ({
    id: `${ownerID}-role-${role.role}-${index}`,
    label: role.role,
    className: accessPresetToneClass(role.role),
  }));
}

function grantChipItems(
  grants: AccessGrantRecord[],
  ownerID: string,
): AccessChipItem[] {
  return grants.map((grant) => ({
    id: `${ownerID}-grant-${grant.id}`,
    label: basicAccessGrantLabel(grant),
    className: accessPresetToneClass(grant.role),
  }));
}

function ActionButtons({
  editLabel,
  deleteLabel,
  disabled = false,
  onEdit,
  onDelete,
}: {
  editLabel: string;
  deleteLabel: string;
  disabled?: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <span className="access-row-actions">
      <button
        type="button"
        className="access-card-action"
        title="Edit"
        aria-label={editLabel}
        onClick={(event) => stopRowAction(event, onEdit)}
      >
        <Edit3 className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />
      </button>
      <button
        type="button"
        className="access-card-action access-card-action--danger"
        title="Delete"
        aria-label={deleteLabel}
        onClick={(event) => stopRowAction(event, onDelete)}
        disabled={disabled}
      >
        <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />
      </button>
    </span>
  );
}

type AccessUsersCatalogProps = {
  users: UserSummary[];
  filteredUsers: UserSummary[];
  grantMap: Map<string, AccessGrantRecord[]>;
  selectedUserID?: string;
  loading: boolean;
  error: string | null;
  grantsLoading: boolean;
  grantsError: string | null;
  onEdit: (user: UserSummary) => void;
  onDelete: (userID: string) => void;
};

export function AccessUsersCatalog({
  users,
  filteredUsers,
  grantMap,
  selectedUserID,
  loading,
  error,
  grantsLoading,
  grantsError,
  onEdit,
  onDelete,
}: AccessUsersCatalogProps) {
  if (error || grantsError) {
    return (
      <div className="access-error-banner">
        {error
          ? `Failed to load users: ${error}`
          : `Failed to load basic roles: ${grantsError}`}
      </div>
    );
  }
  if (loading || grantsLoading) {
    return (
      <CatalogEmptyState
        title="Loading people…"
        detail="Fetching accounts and current role assignments."
      />
    );
  }
  if (!users.length) {
    return (
      <CatalogEmptyState
        title="No users yet"
        detail="Create a local account, then assign access and basic roles."
      />
    );
  }
  if (!filteredUsers.length) {
    return (
      <CatalogEmptyState
        title="No people match this search"
        detail="Try a username, email address, role, or team path."
      />
    );
  }

  return (
    <AccessCatalogTable
      label="Users"
      columns={[
        "User",
        "Basic roles",
        "Access roles",
        "Identity teams",
        "Last sign-in",
        "Actions",
      ]}
      minWidth={1040}
    >
      {filteredUsers.map((user) => {
        const grants = grantMap.get(user.id) || [];
        const displayName = userDisplayName(user);
        const providerLabel = userProviderLabel(user);
        const subjectLabel = userSubjectLabel(user);
        const externalManaged = isExternallyManagedUser(user);
        const emailVerificationLabel = userEmailVerificationLabel(user);
        const identityTeams: AccessChipItem[] = [
          ...(user.external_teams || []).map((team) => ({
            id: `${user.id}-external-${team}`,
            label: `IdP: ${team}`,
            className: "access-chip--team",
          })),
          ...(user.external_auth_teams || []).map((team) => ({
            id: `${user.id}-auth-${team.id || team.name}`,
            label: `NopsAI: ${team.name}`,
            className: "access-chip--accent",
          })),
        ];
        return (
          <tr
            key={user.id}
            tabIndex={0}
            className={`access-table-row ${selectedUserID === user.id ? "access-table-row--selected" : ""}`}
            onClick={() => onEdit(user)}
            onKeyDown={rowActivationHandler(() => onEdit(user))}
          >
            <td>
              <AccessEntityCell
                avatar={(displayName || user.email || "U")
                  .charAt(0)
                  .toUpperCase()}
                title={displayName}
                status={user.status}
                meta={
                  externalManaged
                    ? `External subject ${subjectLabel || user.sub}`
                    : user.email || "No email address"
                }
                detail={
                  externalManaged ? (
                    <>
                      <span className="access-chip access-chip--muted">
                        Authenticated by {providerLabel}
                      </span>
                      {emailVerificationLabel ? (
                        <span className="access-chip access-chip--warn">
                          {emailVerificationLabel}
                        </span>
                      ) : null}
                    </>
                  ) : null
                }
              />
            </td>
            <td>
              <AccessChipList
                items={grantChipItems(grants, user.id)}
                emptyLabel="No basic roles"
                limit={2}
              />
            </td>
            <td>
              <AccessChipList
                items={roleChipItems(user.id, user.roles)}
                emptyLabel="No access roles"
                limit={2}
              />
            </td>
            <td>
              <AccessChipList
                items={identityTeams}
                emptyLabel={externalManaged ? "No mapped teams" : "Local account"}
                limit={2}
              />
            </td>
            <td className="access-cell-muted">
              {user.last_login
                ? formatAccessTimestamp(user.last_login)
                : "Never signed in"}
            </td>
            <td className="access-table__right">
              <ActionButtons
                editLabel={`Edit ${displayName || user.email || "user"}`}
                deleteLabel={`Delete ${displayName || user.email || "user"}`}
                disabled={loading}
                onEdit={() => onEdit(user)}
                onDelete={() => onDelete(user.id)}
              />
            </td>
          </tr>
        );
      })}
    </AccessCatalogTable>
  );
}

type AccessServiceAccountsCatalogProps = {
  accounts: ServiceAccountSummary[];
  filteredAccounts: ServiceAccountSummary[];
  grantMap: Map<string, AccessGrantRecord[]>;
  selectedAccountID?: string;
  loading: boolean;
  error: string | null;
  grantsLoading: boolean;
  grantsError: string | null;
  onEdit: (account: ServiceAccountSummary) => void;
  onDelete: (accountID: string) => void;
};

export function AccessServiceAccountsCatalog({
  accounts,
  filteredAccounts,
  grantMap,
  selectedAccountID,
  loading,
  error,
  grantsLoading,
  grantsError,
  onEdit,
  onDelete,
}: AccessServiceAccountsCatalogProps) {
  if (error || grantsError) {
    return (
      <div className="access-error-banner">
        {error
          ? `Failed to load service accounts: ${error}`
          : `Failed to load basic roles: ${grantsError}`}
      </div>
    );
  }
  if (loading || grantsLoading) {
    return (
      <CatalogEmptyState
        title="Loading service accounts…"
        detail="Fetching integration identities, tokens, and role assignments."
      />
    );
  }
  if (!accounts.length) {
    return (
      <CatalogEmptyState
        title="No service accounts yet"
        detail="Create a token-only account for integrations and automation."
      />
    );
  }
  if (!filteredAccounts.length) {
    return (
      <CatalogEmptyState
        title="No service accounts match this search"
        detail="Try a service account ID, contact email, role, or team path."
      />
    );
  }

  return (
    <AccessCatalogTable
      label="Service accounts"
      columns={[
        "Service account",
        "Access roles",
        "Basic roles",
        "Tokens",
        "Last used",
        "Actions",
      ]}
      minWidth={980}
    >
      {filteredAccounts.map((account) => {
        const grants = grantMap.get(account.sub) || [];
        return (
          <tr
            key={account.id}
            tabIndex={0}
            className={`access-table-row ${selectedAccountID === account.id ? "access-table-row--selected" : ""}`}
            onClick={() => onEdit(account)}
            onKeyDown={rowActivationHandler(() => onEdit(account))}
          >
            <td>
              <AccessEntityCell
                avatar={<Server className="h-4 w-4" aria-hidden="true" />}
                service
                title={account.sub}
                status={account.status}
                meta={account.email || "No contact email"}
                detail={account.provider || "Service account"}
              />
            </td>
            <td>
              <AccessChipList
                items={roleChipItems(account.id, account.roles)}
                emptyLabel="No access roles"
                limit={2}
              />
            </td>
            <td>
              <AccessChipList
                items={grantChipItems(grants, account.id)}
                emptyLabel="No basic roles"
                limit={2}
              />
            </td>
            <td className="access-cell-muted">
              {formatAccessCount(account.token_count || 0, "token")}
            </td>
            <td className="access-cell-muted">
              {account.last_used_at
                ? formatAccessTimestamp(account.last_used_at)
                : "No token activity"}
            </td>
            <td className="access-table__right">
              <ActionButtons
                editLabel={`Edit ${account.sub || "service account"}`}
                deleteLabel={`Delete ${account.sub || "service account"}`}
                disabled={loading}
                onEdit={() => onEdit(account)}
                onDelete={() => onDelete(account.id)}
              />
            </td>
          </tr>
        );
      })}
    </AccessCatalogTable>
  );
}

type RoleAssignee = {
  user: string;
  userId: string;
  email: string;
  kind: string;
};

type AccessRolesCatalogProps = {
  roles: RoleDefinition[];
  filteredRoles: RoleDefinition[];
  roleUserMap: Map<string, RoleAssignee[]>;
  selectedRole?: string;
  loading: boolean;
  error: string | null;
  onEdit: (role: RoleDefinition) => void;
  onDelete: (role: RoleDefinition) => void;
};

export function AccessRolesCatalog({
  roles,
  filteredRoles,
  roleUserMap,
  selectedRole,
  loading,
  error,
  onEdit,
  onDelete,
}: AccessRolesCatalogProps) {
  if (error)
    return (
      <div className="access-error-banner">Failed to load roles: {error}</div>
    );
  if (loading)
    return (
      <CatalogEmptyState
        title="Loading roles…"
        detail="Collecting reusable bundles and their current assignees."
      />
    );
  if (!roles.length)
    return (
      <CatalogEmptyState
        title="No roles yet"
        detail="Create a role and attach policies that match the language your operators already use."
      />
    );
  if (!filteredRoles.length)
    return (
      <CatalogEmptyState
        title="No roles match this search"
        detail="Try a role name, policy label, or one of the assigned accounts."
      />
    );

  return (
    <AccessCatalogTable
      label="Roles"
      columns={[
        "Role",
        "Type",
        "Coverage",
        "Policies",
        "Assignees",
        "Actions",
      ]}
      minWidth={920}
    >
      {filteredRoles.map((role) => {
        const assignedUsers = roleUserMap.get(role.id) || [];
        const preset = accessPresetForRole(role.role);
        const coverage = summarizeRoleCoverage(role.policies);
        const protectedRole = isProtectedAccessRole(role.role);
        return (
          <tr
            key={role.id}
            tabIndex={protectedRole ? -1 : 0}
            className={`access-table-row ${selectedRole === role.role ? "access-table-row--selected" : ""}`}
            onClick={() => {
              if (!protectedRole) onEdit(role);
            }}
            onKeyDown={rowActivationHandler(() => {
              if (!protectedRole) onEdit(role);
            })}
          >
            <td>
              <AccessEntityCell
                avatar="R"
                title={
                  <span className="access-table-title-line">
                    {role.role}
                    {preset ? (
                      <span
                        className={`access-chip ${accessPresetToneClass(role.role)}`}
                      >
                        {preset.label}
                      </span>
                    ) : null}
                  </span>
                }
                meta={
                  preset?.description ||
                  "Reusable role bundle for low-level AAA policies."
                }
              />
            </td>
            <td>
              <span
                className={`access-chip ${protectedRole ? "access-chip--brand" : "access-chip--muted"}`}
              >
                {protectedRole ? "Protected" : "Custom"}
              </span>
            </td>
            <td>
              <AccessChipList
                items={coverage.map((label) => ({
                  id: `${role.id}-coverage-${label}`,
                  label,
                  className: "access-chip--muted",
                }))}
                emptyLabel="No coverage"
                limit={2}
              />
            </td>
            <td className="access-cell-muted">
              {formatAccessCount(role.policies.length, "policy", "policies")}
            </td>
            <td className="access-cell-muted">
              {formatAccessCount(assignedUsers.length, "assignee")}
            </td>
            <td className="access-table__right">
              {protectedRole ? (
                <span className="access-chip access-chip--muted">Protected</span>
              ) : (
                <ActionButtons
                  editLabel={`Edit ${role.role}`}
                  deleteLabel={`Delete ${role.role}`}
                  onEdit={() => onEdit(role)}
                  onDelete={() => onDelete(role)}
                />
              )}
            </td>
          </tr>
        );
      })}
    </AccessCatalogTable>
  );
}

type AccessPoliciesCatalogProps = {
  policies: RolePermission[];
  filteredPolicies: RolePermission[];
  selectedPolicy?: RolePermission;
  loading: boolean;
  error: string | null;
  onEdit: (policy: RolePermission) => void;
  onDelete: (policy: RolePermission) => void;
};

export function AccessPoliciesCatalog({
  policies,
  filteredPolicies,
  selectedPolicy,
  loading,
  error,
  onEdit,
  onDelete,
}: AccessPoliciesCatalogProps) {
  if (error)
    return (
      <div className="access-error-banner">
        Failed to load policies: {error}
      </div>
    );
  if (loading)
    return (
      <CatalogEmptyState
        title="Loading policies…"
        detail="Fetching the low-level rules behind each role bundle."
      />
    );
  if (!policies.length)
    return (
      <CatalogEmptyState
        title="No policies yet"
        detail="Create a rule, then attach it to roles like viewer or developer."
      />
    );
  if (!filteredPolicies.length)
    return (
      <CatalogEmptyState
        title="No policies match this search"
        detail="Search by role, resource selector, action, or policy label."
      />
    );

  return (
    <AccessCatalogTable
      label="Policies"
      columns={[
        "Policy",
        "Resource",
        "Effect",
        "Action",
        "Assigned role",
        "Actions",
      ]}
      minWidth={980}
    >
      {filteredPolicies.map((policy) => {
        const protectedPolicy = isProtectedAccessRole(policy.role);
        const parsedAction = parseAAAActionValue(policy.act);
        const preset = accessPresetForRole(policy.role);
        const isSelected =
          selectedPolicy?.role === policy.role &&
          selectedPolicy.obj === policy.obj &&
          selectedPolicy.act === policy.act;
        return (
          <tr
            key={`${policy.role}-${policy.obj}-${policy.act}`}
            tabIndex={protectedPolicy ? -1 : 0}
            className={`access-table-row ${isSelected ? "access-table-row--selected" : ""}`}
            onClick={() => {
              if (!protectedPolicy) onEdit(policy);
            }}
            onKeyDown={rowActivationHandler(() => {
              if (!protectedPolicy) onEdit(policy);
            })}
          >
            <td>
              <AccessEntityCell
                avatar="P"
                title={policyLabel(policy)}
                meta={`${preset ? `${preset.label} role` : "Role"} can ${formatAccessActionSummary(policy.act)} on ${formatAccessResourceSummary(policy.obj)}.`}
                detail={
                  protectedPolicy ? (
                    <span className="access-chip access-chip--muted">
                      Protected
                    </span>
                  ) : null
                }
              />
            </td>
            <td>
              <span className="access-policy-chip access-policy-chip--path">
                {policy.obj}
              </span>
            </td>
            <td>
              <span
                className={`access-chip ${parsedAction.effect === "deny" ? "access-chip--danger" : "access-chip--success"}`}
              >
                {titleCase(parsedAction.effect || "allow")}
              </span>
            </td>
            <td>
              <span className="access-policy-chip access-policy-chip--act">
                {policy.act}
              </span>
            </td>
            <td>
              <span
                className={`access-chip ${accessPresetToneClass(policy.role)}`}
              >
                {policy.role}
              </span>
            </td>
            <td className="access-table__right">
              {protectedPolicy ? (
                <span className="access-chip access-chip--muted">Protected</span>
              ) : (
                <ActionButtons
                  editLabel={`Edit ${policyLabel(policy)}`}
                  deleteLabel={`Delete ${policyLabel(policy)}`}
                  onEdit={() => onEdit(policy)}
                  onDelete={() => onDelete(policy)}
                />
              )}
            </td>
          </tr>
        );
      })}
    </AccessCatalogTable>
  );
}
