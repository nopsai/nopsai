import { Edit3, Server, Trash2 } from "lucide-react";
import {
  basicAccessGrantLabel,
  isExternallyManagedUser,
  isProtectedAccessRole,
  policyKey,
  policyLabel,
  userDisplayName,
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

function statusKey(value: string) {
  const key = (value || "").toLowerCase();
  if (key.includes("active")) return "ok";
  if (key.includes("pending")) return "warn";
  if (key.includes("blocked") || key.includes("disabled")) return "danger";
  return "muted";
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
        detail="Try a username, email address, role, or group path."
      />
    );
  }

  return (
    <div className="access-entity-grid access-entity-grid--users">
      {filteredUsers.map((user) => {
        const grants = grantMap.get(user.id) || [];
        const displayName = userDisplayName(user);
        const providerLabel = userProviderLabel(user);
        const subjectLabel = userSubjectLabel(user);
        const externalManaged = isExternallyManagedUser(user);
        return (
          <article
            key={user.id}
            className={`access-card access-card--user ${selectedUserID === user.id ? "access-card--selected" : ""}`}
          >
            <div className="access-card__header">
              <div className="min-w-0 flex items-center gap-3">
                <div className="access-avatar">
                  {(displayName || user.email || "U").charAt(0).toUpperCase()}
                </div>
                <div className="min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <p className="access-card__title">{displayName}</p>
                    <span
                      className={`access-status access-status--${statusKey(user.status)}`}
                    >
                      {user.status || "unknown"}
                    </span>
                    {externalManaged && (
                      <span className="access-chip access-chip--muted">
                        Managed by {providerLabel}
                      </span>
                    )}
                  </div>
                  <p className="access-card__subtitle">
                    {externalManaged
                      ? `External subject ${subjectLabel || user.sub}`
                      : user.email || "No email address"}
                  </p>
                  <p className="access-card__meta-line">
                    {user.last_login
                      ? `Last sign-in ${formatAccessTimestamp(user.last_login)}`
                      : "Never signed in"}
                  </p>
                </div>
              </div>
              <div className="access-card__actions">
                <button
                  type="button"
                  className="access-card-action"
                  title="Edit user"
                  aria-label={`Edit ${displayName || user.email || "user"}`}
                  onClick={() => onEdit(user)}
                >
                  <Edit3
                    className="h-4 w-4"
                    strokeWidth={1.8}
                    aria-hidden="true"
                  />
                </button>
                <button
                  type="button"
                  className="access-card-action access-card-action--danger"
                  title="Delete user"
                  aria-label={`Delete ${displayName || user.email || "user"}`}
                  onClick={() => onDelete(user.id)}
                  disabled={loading}
                >
                  <Trash2
                    className="h-4 w-4"
                    strokeWidth={1.9}
                    aria-hidden="true"
                  />
                </button>
              </div>
            </div>
            <div className="space-y-2">
              <p className="access-card__label">Access roles</p>
              <div className="flex flex-wrap gap-2">
                {(user.roles || []).length ? (
                  (user.roles || []).map((role) => (
                    <span
                      key={`${user.id}-${role.role}`}
                      className={`access-chip ${accessPresetToneClass(role.role)}`}
                    >
                      {role.role}
                    </span>
                  ))
                ) : (
                  <span className="text-sm text-[var(--text-secondary)]">
                    No roles assigned yet
                  </span>
                )}
              </div>
            </div>
            {externalManaged &&
              ((user.external_groups || []).length ||
                (user.external_auth_groups || []).length) && (
                <div className="space-y-2">
                  <p className="access-card__label">Identity groups</p>
                  <div className="flex flex-wrap gap-2">
                    {(user.external_groups || []).slice(0, 3).map((group) => (
                      <span
                        key={`${user.id}-external-${group}`}
                        className="access-chip access-chip--muted"
                      >
                        Keycloak: {group}
                      </span>
                    ))}
                    {(user.external_auth_groups || [])
                      .slice(0, 3)
                      .map((group) => (
                        <span
                          key={`${user.id}-auth-${group.id || group.name}`}
                          className="access-chip access-chip--accent"
                        >
                          NopsAI: {group.name}
                        </span>
                      ))}
                  </div>
                </div>
              )}
            <div className="space-y-2">
              <p className="access-card__label">Basic roles</p>
              <div className="flex flex-wrap gap-2">
                {grants.length ? (
                  grants.slice(0, 4).map((grant) => (
                    <span
                      key={grant.id}
                      className={`access-chip ${accessPresetToneClass(grant.role)}`}
                    >
                      {basicAccessGrantLabel(grant)}
                    </span>
                  ))
                ) : (
                  <span className="text-sm text-[var(--text-secondary)]">
                    No basic roles yet
                  </span>
                )}
                {grants.length > 4 && (
                  <span className="access-chip access-chip--muted">
                    + {grants.length - 4} more
                  </span>
                )}
              </div>
            </div>
          </article>
        );
      })}
    </div>
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
        detail="Try a service account ID, contact email, role, or group path."
      />
    );
  }

  return (
    <div className="access-entity-grid access-entity-grid--users">
      {filteredAccounts.map((account) => {
        const grants = grantMap.get(account.sub) || [];
        return (
          <article
            key={account.id}
            className={`access-card access-card--user ${selectedAccountID === account.id ? "access-card--selected" : ""}`}
          >
            <div className="access-card__header">
              <div className="min-w-0 flex items-center gap-3">
                <div className="access-avatar">
                  <Server className="h-4 w-4" />
                </div>
                <div className="min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <p className="access-card__title">{account.sub}</p>
                    <span
                      className={`access-status access-status--${statusKey(account.status)}`}
                    >
                      {account.status || "unknown"}
                    </span>
                  </div>
                  <p className="access-card__subtitle">
                    {account.email || "No contact email"}
                  </p>
                  <p className="access-card__meta-line">
                    {account.last_used_at
                      ? `Last token use ${formatAccessTimestamp(account.last_used_at)}`
                      : "No token activity yet"}{" "}
                    · {formatAccessCount(account.token_count || 0, "token")}
                  </p>
                </div>
              </div>
              <div className="access-card__actions">
                <button
                  type="button"
                  className="access-card-action"
                  title="Edit service account"
                  aria-label={`Edit ${account.sub || "service account"}`}
                  onClick={() => onEdit(account)}
                >
                  <Edit3
                    className="h-4 w-4"
                    strokeWidth={1.8}
                    aria-hidden="true"
                  />
                </button>
                <button
                  type="button"
                  className="access-card-action access-card-action--danger"
                  title="Delete service account"
                  aria-label={`Delete ${account.sub || "service account"}`}
                  onClick={() => onDelete(account.id)}
                  disabled={loading}
                >
                  <Trash2
                    className="h-4 w-4"
                    strokeWidth={1.9}
                    aria-hidden="true"
                  />
                </button>
              </div>
            </div>
            <div className="space-y-2">
              <p className="access-card__label">Access roles</p>
              <div className="flex flex-wrap gap-2">
                {(account.roles || []).length ? (
                  (account.roles || []).map((role) => (
                    <span
                      key={`${account.id}-${role.role}`}
                      className={`access-chip ${accessPresetToneClass(role.role)}`}
                    >
                      {role.role}
                    </span>
                  ))
                ) : (
                  <span className="text-sm text-[var(--text-secondary)]">
                    No roles assigned yet
                  </span>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <p className="access-card__label">Basic roles</p>
              <div className="flex flex-wrap gap-2">
                {grants.length ? (
                  grants.slice(0, 4).map((grant) => (
                    <span
                      key={grant.id}
                      className={`access-chip ${accessPresetToneClass(grant.role)}`}
                    >
                      {basicAccessGrantLabel(grant)}
                    </span>
                  ))
                ) : (
                  <span className="text-sm text-[var(--text-secondary)]">
                    No basic roles yet
                  </span>
                )}
                {grants.length > 4 && (
                  <span className="access-chip access-chip--muted">
                    + {grants.length - 4} more
                  </span>
                )}
              </div>
            </div>
          </article>
        );
      })}
    </div>
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
    <div className="access-entity-grid access-entity-grid--roles">
      {filteredRoles.map((role) => {
        const assignedUsers = roleUserMap.get(role.id) || [];
        const preset = accessPresetForRole(role.role);
        const coverage = summarizeRoleCoverage(role.policies);
        const protectedRole = isProtectedAccessRole(role.role);
        return (
          <article
            key={role.id}
            className={`access-card access-card--role ${selectedRole === role.role ? "access-card--selected" : ""}`}
          >
            <div className="access-card__header">
              <div className="space-y-2 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="access-card__title">{role.role}</p>
                  {preset && (
                    <span
                      className={`access-chip ${accessPresetToneClass(role.role)}`}
                    >
                      {preset.label}
                    </span>
                  )}
                  {protectedRole && (
                    <span className="access-chip access-chip--muted">
                      Protected
                    </span>
                  )}
                </div>
                <p className="access-card__subtitle">
                  {preset?.description ||
                    "Reusable role bundle for assigning multiple low-level AAA policies together."}
                </p>
                <p className="access-card__meta-line">
                  {formatAccessCount(
                    role.policies.length,
                    "policy",
                    "policies",
                  )}{" "}
                  · {formatAccessCount(assignedUsers.length, "assignee")}
                </p>
              </div>
              <div className="access-card__actions">
                {protectedRole ? (
                  <span className="access-chip access-chip--muted">
                    Protected
                  </span>
                ) : (
                  <>
                    <button
                      type="button"
                      className="access-card-action"
                      title="Edit role"
                      aria-label={`Edit ${role.role}`}
                      onClick={() => onEdit(role)}
                    >
                      <Edit3
                        className="h-4 w-4"
                        strokeWidth={1.8}
                        aria-hidden="true"
                      />
                    </button>
                    <button
                      type="button"
                      className="access-card-action access-card-action--danger"
                      title="Delete role"
                      aria-label={`Delete ${role.role}`}
                      onClick={() => onDelete(role)}
                    >
                      <Trash2
                        className="h-4 w-4"
                        strokeWidth={1.9}
                        aria-hidden="true"
                      />
                    </button>
                  </>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <p className="access-card__label">Coverage</p>
              <div className="flex flex-wrap gap-2">
                {coverage.map((label) => (
                  <span
                    key={`${role.id}-coverage-${label}`}
                    className="access-chip access-chip--muted"
                  >
                    {label}
                  </span>
                ))}
                {!coverage.length && (
                  <span className="text-sm text-[var(--text-secondary)]">
                    No coverage yet
                  </span>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <p className="access-card__label">Includes</p>
              <div className="flex flex-wrap gap-2">
                {role.policies.slice(0, 4).map((policy) => (
                  <span
                    key={policyKey(policy)}
                    className="access-chip access-chip--muted"
                  >
                    {policyLabel(policy)}
                  </span>
                ))}
                {role.policies.length > 4 && (
                  <span className="access-chip access-chip--muted">
                    + {role.policies.length - 4} more
                  </span>
                )}
              </div>
            </div>
          </article>
        );
      })}
    </div>
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
    <div className="access-policy-stack">
      {filteredPolicies.map((policy) => {
        const protectedPolicy = isProtectedAccessRole(policy.role);
        const parsedAction = parseAAAActionValue(policy.act);
        const preset = accessPresetForRole(policy.role);
        const isSelected =
          selectedPolicy?.role === policy.role &&
          selectedPolicy.obj === policy.obj &&
          selectedPolicy.act === policy.act;
        return (
          <article
            key={`${policy.role}-${policy.obj}-${policy.act}`}
            className={`access-card access-card--policy ${isSelected ? "access-card--selected" : ""}`}
          >
            <div className="access-card__header">
              <div className="space-y-2 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="access-card__title">{policyLabel(policy)}</p>
                  <span
                    className={`access-chip ${accessPresetToneClass(policy.role)}`}
                  >
                    {policy.role}
                  </span>
                  {parsedAction.effect === "deny" && (
                    <span className="access-chip access-chip--danger">
                      Deny
                    </span>
                  )}
                  {protectedPolicy && (
                    <span className="access-chip access-chip--muted">
                      Protected
                    </span>
                  )}
                </div>
                <p className="access-card__subtitle">
                  {preset ? `${preset.label} role` : "Role"} can{" "}
                  {formatAccessActionSummary(policy.act)} on{" "}
                  {formatAccessResourceSummary(policy.obj)}.
                </p>
              </div>
              <div className="access-card__actions">
                {protectedPolicy ? (
                  <span className="access-chip access-chip--muted">
                    Protected
                  </span>
                ) : (
                  <>
                    <button
                      type="button"
                      className="access-card-action"
                      title="Edit policy"
                      aria-label={`Edit ${policyLabel(policy)}`}
                      onClick={() => onEdit(policy)}
                    >
                      <Edit3
                        className="h-4 w-4"
                        strokeWidth={1.8}
                        aria-hidden="true"
                      />
                    </button>
                    <button
                      type="button"
                      className="access-card-action access-card-action--danger"
                      title="Delete policy"
                      aria-label={`Delete ${policyLabel(policy)}`}
                      onClick={() => onDelete(policy)}
                    >
                      <Trash2
                        className="h-4 w-4"
                        strokeWidth={1.9}
                        aria-hidden="true"
                      />
                    </button>
                  </>
                )}
              </div>
            </div>
            <div className="access-policy-preview access-policy-preview--minimal">
              <span className="access-policy-chip access-policy-chip--path">
                {policy.obj}
              </span>
              <span className="access-policy-arrow">-&gt;</span>
              <span className="access-policy-chip access-policy-chip--act">
                {policy.act}
              </span>
            </div>
          </article>
        );
      })}
    </div>
  );
}
