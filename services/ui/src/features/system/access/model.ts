import { asRecord, readOptionalString, readString } from "../data.js";

export const POLICY_TEMPLATE_ROLE = "__policy_template__";
export const DEFAULT_ADMIN_ROLE = "nopsai-admin";
const DEFAULT_ADMIN_POLICY_OBJ = "*:*";
const DEFAULT_ADMIN_POLICY_ACT = "*";
export const ROOT_ACCESS_SCOPE = "root";
export const BASIC_ROLE_VIEWER = "viewer";
export const BASIC_ROLE_DEVELOPER = "developer";
export const BASIC_ROLE_OWNER = "owner";
export const BASIC_ROLE_ADMIN = "admin";
export const PROTECTED_ACCESS_ROLES = new Set([
  DEFAULT_ADMIN_ROLE,
  BASIC_ROLE_VIEWER,
  BASIC_ROLE_DEVELOPER,
  BASIC_ROLE_OWNER,
  BASIC_ROLE_ADMIN,
]);
export const ACCESS_UI_BUILD_ID = "access-protected-default-roles-2026-05-11";

export type UserRole = {
  role: string;
};

export type UserAuthGroupSummary = {
  id: string;
  name: string;
};

export type RolePermission = {
  role: string;
  name?: string;
  obj: string;
  act: string;
};

export type UserSummary = {
  id: string;
  sub: string;
  email: string;
  display_name?: string;
  provider?: string;
  status: string;
  last_login?: string;
  roles?: UserRole[];
  external_managed?: boolean;
  external_provider_id?: string;
  external_provider_name?: string;
  external_subject?: string;
  external_groups?: string[];
  external_auth_groups?: UserAuthGroupSummary[];
  external_roles?: string[];
};

export type ServiceAccountToken = {
  id: string;
  name: string;
  token?: string;
  token_suffix: string;
  created_at: string;
  expires_at?: string;
  last_used_at?: string;
};

export type ServiceAccountSummary = {
  id: string;
  sub: string;
  email: string;
  provider?: string;
  status: string;
  token_count: number;
  last_used_at?: string;
  roles?: UserRole[];
};

export type IdentityProviderSettings = {
  local_enabled: boolean;
  oidc_enabled: boolean;
  auto_create_users: boolean;
  default_role: string;
  allow_email_linking: boolean;
};

export type IdentityProviderRecord = {
  id: string;
  type: string;
  display_name: string;
  issuer: string;
  authorization_endpoint?: string;
  token_endpoint?: string;
  jwks_uri?: string;
  userinfo_endpoint?: string;
  client_id: string;
  client_credential_ref?: string;
  scopes: string[];
  allowed_email_domains: string[];
  group_claim?: string;
  role_mapping: Record<string, string>;
  group_mapping: Record<string, string>;
  basic_role_mapping: Record<string, IdentityProviderBasicRoleMapping>;
  auto_create_users?: boolean;
  default_role?: string;
  allow_email_linking?: boolean;
  enabled: boolean;
  config_source?: string;
  has_client_credential?: boolean;
};

export type IdentityProviderBasicRoleMapping = {
  role: string;
  resource?: string;
  resource_type?: string;
  resource_id?: string;
};

export type IdentityProvidersState = {
  settings: IdentityProviderSettings;
  providers: IdentityProviderRecord[];
  domain_mappings: Record<string, string>;
};

export type IdentityProviderFormState = {
  id: string;
  type: string;
  display_name: string;
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  jwks_uri: string;
  userinfo_endpoint: string;
  client_id: string;
  client_credential_ref: string;
  scopes: string;
  allowed_email_domains: string;
  group_claim: string;
  role_mapping: string;
  group_mapping: string;
  basic_role_mapping: string;
  auto_create_users: string;
  default_role: string;
  allow_email_linking: string;
  enabled: boolean;
};

export type AccessGrantRecord = {
  id: string;
  subjectType: string;
  subjectID: string;
  subjectDisplay?: string;
  role: string;
  resourceType: string;
  resourceID: string;
  inherit: boolean;
  grantedBy?: string;
  createdAt?: string;
  managedByConfigRepo?: boolean;
  managedByIdentityProvider?: boolean;
  identityProviderID?: string;
  externalGroupName?: string;
  source?: string;
};

export type EditableAccessGrant = {
  localID: string;
  id?: string;
  role: string;
  resourceType: string;
  resourceID: string;
  inherit: boolean;
  grantedBy?: string;
};

export type BasicGrantInput = {
  role: string;
  resourceType: string;
  resourceID: string;
  inherit?: boolean;
};

export type RolePolicyDraft = {
  name: string;
  obj: string;
  act: string;
};

export type RoleDefinition = {
  id: string;
  role: string;
  policies: RolePermission[];
};

export const policyKey = (input: { role: string; obj: string; act: string }) =>
  `${(input.role || "").trim()}::${(input.obj || "").trim()}::${(input.act || "").trim()}`;

export const assignmentKey = (role: string) => (role || "").trim();

export const policyName = (obj: string, act: string) => {
  const trimmed = (obj || "").replace(/^\/+|\/+$/g, "").trim();
  const leaf = trimmed.split("/").filter(Boolean).pop();
  const base = leaf || trimmed || obj || "policy";
  const action = (act || "").trim() || "ANY";
  return `${base} • ${action}`;
};

export const policyLabel = (input: {
  name?: string;
  obj: string;
  act: string;
}) => (input.name && input.name.trim()) || policyName(input.obj, input.act);

export const isDefaultAdmin = (roleName: string) =>
  roleName === DEFAULT_ADMIN_ROLE;

export const isProtectedAccessRole = (roleName: string) =>
  PROTECTED_ACCESS_ROLES.has((roleName || "").trim().toLowerCase());

export const isDefaultAdminUser = (user?: Pick<UserSummary, "sub"> | null) =>
  (user?.sub || "").trim().toLowerCase() === "admin";

export const isExternallyManagedUser = (
  user?: Pick<
    UserSummary,
    "provider" | "external_managed" | "external_provider_id"
  > | null,
) =>
  Boolean(
    user?.external_managed ||
    (user?.external_provider_id || "").trim() ||
    (user?.provider || "").trim().toLowerCase().startsWith("oidc:"),
  );

export const isUserRoleManagementLocked = (user?: UserSummary | null) =>
  isDefaultAdminUser(user) || isExternallyManagedUser(user);

export const userDisplayName = (
  user?: Pick<
    UserSummary,
    "display_name" | "email" | "sub" | "id" | "external_managed" | "provider"
  > | null,
) => {
  if (!user) return "User";
  if (isExternallyManagedUser(user)) {
    return (
      user.display_name ||
      user.email ||
      user.sub ||
      user.id ||
      "User"
    ).trim();
  }
  return (
    user.display_name ||
    user.sub ||
    user.email ||
    user.id ||
    "User"
  ).trim();
};

export const userProviderLabel = (
  user?: Pick<
    UserSummary,
    "provider" | "external_provider_id" | "external_provider_name"
  > | null,
) => {
  if (!user || !isExternallyManagedUser(user)) return "";
  return (
    user.external_provider_name ||
    user.external_provider_id ||
    (user.provider || "").replace(/^oidc:/i, "") ||
    "identity provider"
  ).trim();
};

export const userSubjectLabel = (
  user?: Pick<
    UserSummary,
    "sub" | "external_subject" | "external_provider_id"
  > | null,
) => {
  if (!user) return "";
  const externalSubject = (user.external_subject || "").trim();
  if (externalSubject) return externalSubject;
  const providerID = (user.external_provider_id || "").trim();
  const sub = (user.sub || "").trim();
  const prefix = providerID ? `oidc:${providerID}:` : "";
  if (prefix && sub.startsWith(prefix)) return sub.slice(prefix.length);
  return sub;
};

export const isRootAccessScopeID = (value?: string) => {
  const normalized = String(value || "")
    .trim()
    .replace(/^\/+|\/+$/g, "")
    .toLowerCase();
  return normalized === "root";
};

export const normalizeBasicGrantResourceLabel = (
  grant: Pick<AccessGrantRecord, "resourceType" | "resourceID">,
) => {
  const resourceType = (grant.resourceType || "").trim();
  const resourceID = (grant.resourceID || "").trim().replace(/^\/+|\/+$/g, "");
  if (resourceType === "platform") return "Platform";
  if (isRootAccessScopeID(resourceID)) return "Root";
  return `/${resourceID}`;
};

export const basicAccessGrantLabel = (
  grant: Pick<AccessGrantRecord, "role" | "resourceType" | "resourceID">,
) => `${grant.role} • ${normalizeBasicGrantResourceLabel(grant)}`;

export const accessGrantResourceSummary = (
  grant: Pick<AccessGrantRecord, "resourceType" | "resourceID">,
) => {
  if ((grant.resourceType || "").trim() === "platform") return "Platform wide";
  return normalizeBasicGrantResourceLabel(grant);
};

export const basicAccessGrantDescription = (
  grant: Pick<
    AccessGrantRecord,
    "role" | "resourceType" | "resourceID" | "grantedBy"
  >,
) => {
  const label = accessGrantResourceSummary(grant);
  if ((grant.resourceType || "").trim() === "platform") {
    return "This basic role gives platform-wide administrator access.";
  }
  if (label === "Root") {
    return `This ${grant.role} basic role applies to items that are not inside any group.`;
  }
  return `This ${grant.role} basic role applies to ${label} and anything nested below it.`;
};

export const accessGrantSortLabel = (grant: AccessGrantRecord) =>
  `${normalizeBasicGrantResourceLabel(grant)}::${grant.role}`;

export const normalizedAccessGrantResourceKey = (
  grant: Pick<AccessGrantRecord, "resourceType" | "resourceID">,
) => {
  const resourceType = (grant.resourceType || "").trim().toLowerCase();
  const resourceID = (grant.resourceID || "").trim();
  if (resourceType === "folder") {
    const folderID = resourceID.replace(/^\/+|\/+$/g, "");
    if (isRootAccessScopeID(folderID)) {
      return { resourceType, resourceID: ROOT_ACCESS_SCOPE };
    }
    return { resourceType, resourceID: folderID };
  }
  if (resourceType === "platform") {
    return { resourceType, resourceID: "platform" };
  }
  return { resourceType, resourceID };
};

export const accessGrantTargetKey = (
  grant: Pick<AccessGrantRecord, "resourceType" | "resourceID">,
) => {
  const { resourceType, resourceID } = normalizedAccessGrantResourceKey(grant);
  return `${resourceType}::${resourceID}`;
};

export const accessGrantEditKey = (
  grant: Pick<AccessGrantRecord, "role" | "resourceType" | "resourceID">,
) =>
  `${(grant.role || "").trim().toLowerCase()}::${accessGrantTargetKey(grant)}`;

export const editableAccessGrantFromRecord = (
  grant: AccessGrantRecord,
): EditableAccessGrant => ({
  localID: grant.id,
  id: grant.id,
  role: grant.role,
  resourceType: grant.resourceType,
  resourceID: grant.resourceID,
  inherit: grant.inherit,
  grantedBy: grant.grantedBy,
});

export const normalizeBasicGrantInputs = (
  entries: BasicGrantInput[],
): BasicGrantInput[] =>
  Array.from(
    entries.reduce((map, entry) => {
      const role = (entry.role || "").trim().toLowerCase();
      const resourceType = (entry.resourceType || "").trim().toLowerCase();
      const resourceID = (entry.resourceID || "").trim();
      if (!role || !resourceType || !resourceID) return map;
      const normalized = {
        role,
        resourceType,
        resourceID:
          resourceType === "folder"
            ? normalizedAccessGrantResourceKey({ resourceType, resourceID })
                .resourceID
            : resourceID,
        inherit: entry.inherit,
      };
      map.set(accessGrantEditKey(normalized), normalized);
      return map;
    }, new Map<string, BasicGrantInput>()),
  ).map(([, entry]) => entry);

export const normalizeEditableBasicGrants = (
  entries: EditableAccessGrant[],
): BasicGrantInput[] =>
  normalizeBasicGrantInputs(
    entries.map((entry) => ({
      role: entry.role,
      resourceType: entry.resourceType,
      resourceID: entry.resourceID,
      inherit: entry.inherit,
    })),
  );

export const isBasicAccessGrant = (grant: AccessGrantRecord) => {
  const role = (grant.role || "").trim().toLowerCase();
  const resourceType = (grant.resourceType || "").trim();
  return (
    (resourceType === "folder" || resourceType === "platform") &&
    (role === BASIC_ROLE_VIEWER ||
      role === BASIC_ROLE_DEVELOPER ||
      role === BASIC_ROLE_OWNER ||
      role === BASIC_ROLE_ADMIN)
  );
};

export const accessGrantMatchesUser = (
  grant: AccessGrantRecord,
  user: UserSummary,
) => {
  const subjectType = (grant.subjectType || "").trim();
  const subjectID = (grant.subjectID || "").trim();
  if (!subjectID) return false;
  if (subjectType === "auth_group" || subjectType === "group") {
    return (user.external_auth_groups || []).some(
      (group) => subjectID === group.id || subjectID === group.name,
    );
  }
  if (subjectType !== "user") return false;
  return subjectID === user.id || subjectID === user.sub || subjectID === user.email;
};

export const accessGrantMatchesServiceAccount = (
  grant: AccessGrantRecord,
  account: ServiceAccountSummary,
) => {
  const subjectType = (grant.subjectType || "").trim();
  const subjectID = (grant.subjectID || "").trim();
  if (subjectType !== "service_account" || !subjectID) return false;
  return subjectID === account.sub || subjectID === account.id;
};

export const normalizeAdminPolicies = (
  records: RolePermission[],
): RolePermission[] => {
  const deduped = records.filter(
    (entry, idx, arr) =>
      idx === arr.findIndex((other) => policyKey(other) === policyKey(entry)),
  );
  const filtered = deduped.filter(
    (entry) =>
      !isDefaultAdmin(entry.role) ||
      (entry.obj === DEFAULT_ADMIN_POLICY_OBJ &&
        entry.act === DEFAULT_ADMIN_POLICY_ACT),
  );
  const hasCanonicalAdmin = filtered.some(
    (entry) =>
      isDefaultAdmin(entry.role) &&
      entry.obj === DEFAULT_ADMIN_POLICY_OBJ &&
      entry.act === DEFAULT_ADMIN_POLICY_ACT,
  );
  if (!hasCanonicalAdmin) {
    filtered.push({
      role: DEFAULT_ADMIN_ROLE,
      name: "Admin all access",
      obj: DEFAULT_ADMIN_POLICY_OBJ,
      act: DEFAULT_ADMIN_POLICY_ACT,
    });
  }
  return filtered;
};

export const emptyIdentityProviderForm = (): IdentityProviderFormState => ({
  id: "",
  type: "oidc",
  display_name: "",
  issuer: "",
  authorization_endpoint: "",
  token_endpoint: "",
  jwks_uri: "",
  userinfo_endpoint: "",
  client_id: "",
  client_credential_ref: "",
  scopes: "openid, email, profile",
  allowed_email_domains: "",
  group_claim: "groups",
  role_mapping: "",
  group_mapping: "",
  basic_role_mapping: "",
  auto_create_users: "inherit",
  default_role: "",
  allow_email_linking: "inherit",
  enabled: true,
});

export function identityProviderFormFromRecord(
  record: IdentityProviderRecord,
): IdentityProviderFormState {
  return {
    id: record.id,
    type: record.type || "oidc",
    display_name: record.display_name || record.id,
    issuer: record.issuer || "",
    authorization_endpoint: record.authorization_endpoint || "",
    token_endpoint: record.token_endpoint || "",
    jwks_uri: record.jwks_uri || "",
    userinfo_endpoint: record.userinfo_endpoint || "",
    client_id: record.client_id || "",
    client_credential_ref: record.client_credential_ref || "",
    scopes: (record.scopes || []).join(", "),
    allowed_email_domains: (record.allowed_email_domains || []).join(", "),
    group_claim: record.group_claim || "groups",
    role_mapping: Object.entries(record.role_mapping || {})
      .map(([group, role]) => `${group}: ${role}`)
      .join("\n"),
    group_mapping: Object.entries(record.group_mapping || {})
      .map(([group, authGroup]) => `${group}: ${authGroup}`)
      .join("\n"),
    basic_role_mapping: Object.entries(record.basic_role_mapping || {})
      .map(([group, grant]) => `${group}: ${grant.role} ${grant.resource || `${grant.resource_type || ""}:${grant.resource_id || ""}`}`)
      .join("\n"),
    auto_create_users:
      typeof record.auto_create_users === "boolean"
        ? String(record.auto_create_users)
        : "inherit",
    default_role: record.default_role || "",
    allow_email_linking:
      typeof record.allow_email_linking === "boolean"
        ? String(record.allow_email_linking)
        : "inherit",
    enabled: Boolean(record.enabled),
  };
}

export function identityProviderPayloadFromForm(
  form: IdentityProviderFormState,
) {
  const optionalBool = (value: string) =>
    value === "inherit" ? undefined : value === "true";
  return {
    id: form.id.trim(),
    type: form.type.trim() || "oidc",
    display_name: form.display_name.trim(),
    issuer: form.issuer.trim(),
    authorization_endpoint: form.authorization_endpoint.trim(),
    token_endpoint: form.token_endpoint.trim(),
    jwks_uri: form.jwks_uri.trim(),
    userinfo_endpoint: form.userinfo_endpoint.trim(),
    client_id: form.client_id.trim(),
    client_credential_ref: form.client_credential_ref.trim(),
    scopes: splitCSV(form.scopes),
    allowed_email_domains: splitCSV(form.allowed_email_domains),
    group_claim: form.group_claim.trim(),
    role_mapping: parseRoleMapping(form.role_mapping),
    group_mapping: parseRoleMapping(form.group_mapping),
    basic_role_mapping: parseBasicRoleMapping(form.basic_role_mapping),
    auto_create_users: optionalBool(form.auto_create_users),
    default_role: form.default_role.trim(),
    allow_email_linking: optionalBool(form.allow_email_linking),
    enabled: Boolean(form.enabled),
  };
}

export function normalizeIdentityProvidersState(
  payload: unknown,
): IdentityProvidersState {
  const record = asRecord(payload);
  const settings = asRecord(record?.settings);
  const mappings = asRecord(record?.domain_mappings);
  return {
    settings: {
      local_enabled: settings?.local_enabled !== false,
      oidc_enabled: Boolean(settings?.oidc_enabled),
      auto_create_users: Boolean(settings?.auto_create_users),
      default_role: readString(settings?.default_role),
      allow_email_linking: Boolean(settings?.allow_email_linking),
    },
    providers: Array.isArray(record?.providers)
      ? (record.providers
          .map(normalizeIdentityProviderRecord)
          .filter(Boolean) as IdentityProviderRecord[])
      : [],
    domain_mappings: Object.fromEntries(
      Object.entries(mappings || {})
        .map(([domain, provider]) => [domain, String(provider || "")])
        .filter(([domain, provider]) => domain && provider),
    ),
  };
}

function normalizeIdentityProviderRecord(
  value: unknown,
): IdentityProviderRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const id = readString(record.id);
  if (!id) return null;
  return {
    id,
    type: readString(record.type) || "oidc",
    display_name: readString(record.display_name) || id,
    issuer: readString(record.issuer),
    authorization_endpoint: readOptionalString(record.authorization_endpoint),
    token_endpoint: readOptionalString(record.token_endpoint),
    jwks_uri: readOptionalString(record.jwks_uri),
    userinfo_endpoint: readOptionalString(record.userinfo_endpoint),
    client_id: readString(record.client_id),
    client_credential_ref: readOptionalString(record.client_credential_ref),
    scopes: Array.isArray(record.scopes)
      ? record.scopes.map((item) => String(item || "").trim()).filter(Boolean)
      : [],
    allowed_email_domains: Array.isArray(record.allowed_email_domains)
      ? record.allowed_email_domains
          .map((item) => String(item || "").trim())
          .filter(Boolean)
      : [],
    group_claim: readOptionalString(record.group_claim),
    role_mapping: normalizeRoleMappingRecord(record.role_mapping),
    group_mapping: normalizeRoleMappingRecord(record.group_mapping),
    basic_role_mapping: normalizeBasicRoleMappingRecord(record.basic_role_mapping),
    auto_create_users:
      typeof record.auto_create_users === "boolean"
        ? record.auto_create_users
        : undefined,
    default_role: readOptionalString(record.default_role),
    allow_email_linking:
      typeof record.allow_email_linking === "boolean"
        ? record.allow_email_linking
        : undefined,
    enabled: record.enabled !== false,
    config_source: readOptionalString(record.config_source),
    has_client_credential: Boolean(record.has_client_credential),
  };
}

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseRoleMapping(value: string): Record<string, string> {
  const entries: Array<[string, string]> = [];
  value.split("\n").forEach((line) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const separator = trimmed.includes(":") ? ":" : "=";
    const index = trimmed.indexOf(separator);
    if (index <= 0) return;
    const group = trimmed.slice(0, index).trim();
    const role = trimmed.slice(index + 1).trim();
    if (group && role) entries.push([group, role]);
  });
  return Object.fromEntries(entries);
}

function parseBasicRoleMapping(value: string): Record<string, IdentityProviderBasicRoleMapping> {
  const entries: Array<[string, IdentityProviderBasicRoleMapping]> = [];
  value.split("\n").forEach((line) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const colonIndex = trimmed.indexOf(":");
    const equalsIndex = trimmed.indexOf("=");
    const index = equalsIndex >= 0 && (colonIndex < 0 || equalsIndex < colonIndex) ? equalsIndex : colonIndex;
    if (index <= 0) return;
    const group = trimmed.slice(0, index).trim();
    const value = trimmed.slice(index + 1).trim();
    const [role = "", resource = ""] = value.split(/\s+/, 2);
    const normalizedRole = role.trim().toLowerCase();
    const normalizedResource = resource.trim();
    if (!group || !normalizedRole || !normalizedResource) return;
    entries.push([
      group,
      {
        role: normalizedRole,
        resource: normalizedResource,
      },
    ]);
  });
  return Object.fromEntries(entries);
}

function normalizeBasicRoleMappingRecord(value: unknown): Record<string, IdentityProviderBasicRoleMapping> {
  const record = asRecord(value);
  if (!record) return {};
  return Object.fromEntries(
    Object.entries(record)
      .map(([group, rawGrant]) => {
        const grant = asRecord(rawGrant);
        return [
          group.trim(),
          {
            role: readString(grant?.role).toLowerCase(),
            resource: readOptionalString(grant?.resource),
            resource_type: readOptionalString(grant?.resource_type),
            resource_id: readOptionalString(grant?.resource_id),
          },
        ] as const;
      })
      .filter(([group, grant]) =>
        Boolean(group && grant.role && (grant.resource || (grant.resource_type && grant.resource_id))),
      ),
  );
}

function normalizeRoleMappingRecord(value: unknown): Record<string, string> {
  const record = asRecord(value);
  if (!record) return {};
  return Object.fromEntries(
    Object.entries(record)
      .map(([group, role]) => [group.trim(), String(role || "").trim()])
      .filter(([group, role]) => group && role),
  );
}

export function normalizeAccessGrantRecord(
  value: unknown,
): AccessGrantRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const id = readString(record.id);
  if (!id) return null;
  return {
    id,
    subjectType: readString(record.subject_type),
    subjectID: readString(record.subject_id),
    subjectDisplay: readOptionalString(record.subject_display),
    role: readString(record.role),
    resourceType: readString(record.resource_type),
    resourceID: readString(record.resource_id),
    inherit: Boolean(record.inherit),
    grantedBy: readOptionalString(record.granted_by),
    createdAt: readOptionalString(record.created_at),
    managedByConfigRepo: Boolean(record.managed_by_config_repo),
    managedByIdentityProvider: Boolean(record.managed_by_identity_provider),
    identityProviderID: readOptionalString(record.identity_provider_id),
    externalGroupName: readOptionalString(record.external_group_name),
    source: readOptionalString(record.source),
  };
}
