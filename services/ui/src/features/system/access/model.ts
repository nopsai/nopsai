import { asRecord, readOptionalString, readString } from '../data.js';

export const POLICY_TEMPLATE_ROLE = '__policy_template__';
export const DEFAULT_ADMIN_ROLE = 'nopsai-admin';
const DEFAULT_ADMIN_POLICY_OBJ = '*:*';
const DEFAULT_ADMIN_POLICY_ACT = '*';
export const ROOT_ACCESS_SCOPE = 'root';
export const BASIC_ROLE_VIEWER = 'viewer';
export const BASIC_ROLE_DEVELOPER = 'developer';
export const BASIC_ROLE_OWNER = 'owner';
export const BASIC_ROLE_ADMIN = 'admin';
export const PROTECTED_ACCESS_ROLES = new Set([
  DEFAULT_ADMIN_ROLE,
  BASIC_ROLE_VIEWER,
  BASIC_ROLE_DEVELOPER,
  BASIC_ROLE_OWNER,
  BASIC_ROLE_ADMIN,
]);
export const ACCESS_UI_BUILD_ID = 'access-protected-default-roles-2026-05-11';

export type UserRole = {
  role: string;
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
  provider?: string;
  status: string;
  last_login?: string;
  roles?: UserRole[];
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
  `${(input.role || '').trim()}::${(input.obj || '').trim()}::${(input.act || '').trim()}`;

export const assignmentKey = (role: string) => (role || '').trim();

export const policyName = (obj: string, act: string) => {
  const trimmed = (obj || '').replace(/^\/+|\/+$/g, '').trim();
  const leaf = trimmed.split('/').filter(Boolean).pop();
  const base = leaf || trimmed || obj || 'policy';
  const action = (act || '').trim() || 'ANY';
  return `${base} • ${action}`;
};

export const policyLabel = (input: { name?: string; obj: string; act: string }) =>
  (input.name && input.name.trim()) || policyName(input.obj, input.act);

export const isDefaultAdmin = (roleName: string) => roleName === DEFAULT_ADMIN_ROLE;

export const isProtectedAccessRole = (roleName: string) => PROTECTED_ACCESS_ROLES.has((roleName || '').trim().toLowerCase());

export const isDefaultAdminUser = (user?: Pick<UserSummary, 'sub'> | null) => (user?.sub || '').trim().toLowerCase() === 'admin';

export const isRootAccessScopeID = (value?: string) => {
  const normalized = String(value || '').trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  return normalized === 'root';
};

export const normalizeBasicGrantResourceLabel = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  const resourceType = (grant.resourceType || '').trim();
  const resourceID = (grant.resourceID || '').trim().replace(/^\/+|\/+$/g, '');
  if (resourceType === 'platform') return 'Platform';
  if (isRootAccessScopeID(resourceID)) return 'Root';
  return `/${resourceID}`;
};

export const basicAccessGrantLabel = (grant: Pick<AccessGrantRecord, 'role' | 'resourceType' | 'resourceID'>) =>
  `${grant.role} • ${normalizeBasicGrantResourceLabel(grant)}`;

export const accessGrantResourceSummary = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  if ((grant.resourceType || '').trim() === 'platform') return 'Platform wide';
  return normalizeBasicGrantResourceLabel(grant);
};

export const basicAccessGrantDescription = (grant: Pick<AccessGrantRecord, 'role' | 'resourceType' | 'resourceID' | 'grantedBy'>) => {
  const label = accessGrantResourceSummary(grant);
  if ((grant.resourceType || '').trim() === 'platform') {
    return 'This basic role gives platform-wide administrator access.';
  }
  if (label === 'Root') {
    return `This ${grant.role} basic role applies to items that are not inside any group.`;
  }
  return `This ${grant.role} basic role applies to ${label} and anything nested below it.`;
};

export const accessGrantSortLabel = (grant: AccessGrantRecord) => `${normalizeBasicGrantResourceLabel(grant)}::${grant.role}`;

export const normalizedAccessGrantResourceKey = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  const resourceType = (grant.resourceType || '').trim().toLowerCase();
  const resourceID = (grant.resourceID || '').trim();
  if (resourceType === 'folder') {
    const folderID = resourceID.replace(/^\/+|\/+$/g, '');
    if (isRootAccessScopeID(folderID)) {
      return { resourceType, resourceID: ROOT_ACCESS_SCOPE };
    }
    return { resourceType, resourceID: folderID };
  }
  if (resourceType === 'platform') {
    return { resourceType, resourceID: 'platform' };
  }
  return { resourceType, resourceID };
};

export const accessGrantTargetKey = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  const { resourceType, resourceID } = normalizedAccessGrantResourceKey(grant);
  return `${resourceType}::${resourceID}`;
};

export const accessGrantEditKey = (grant: Pick<AccessGrantRecord, 'role' | 'resourceType' | 'resourceID'>) =>
  `${(grant.role || '').trim().toLowerCase()}::${accessGrantTargetKey(grant)}`;

export const editableAccessGrantFromRecord = (grant: AccessGrantRecord): EditableAccessGrant => ({
  localID: grant.id,
  id: grant.id,
  role: grant.role,
  resourceType: grant.resourceType,
  resourceID: grant.resourceID,
  inherit: grant.inherit,
  grantedBy: grant.grantedBy,
});

export const normalizeBasicGrantInputs = (entries: BasicGrantInput[]): BasicGrantInput[] =>
  Array.from(
    entries.reduce((map, entry) => {
      const role = (entry.role || '').trim().toLowerCase();
      const resourceType = (entry.resourceType || '').trim().toLowerCase();
      const resourceID = (entry.resourceID || '').trim();
      if (!role || !resourceType || !resourceID) return map;
      const normalized = {
        role,
        resourceType,
        resourceID: resourceType === 'folder' ? normalizedAccessGrantResourceKey({ resourceType, resourceID }).resourceID : resourceID,
        inherit: entry.inherit,
      };
      map.set(accessGrantEditKey(normalized), normalized);
      return map;
    }, new Map<string, BasicGrantInput>())
  ).map(([, entry]) => entry);

export const normalizeEditableBasicGrants = (entries: EditableAccessGrant[]): BasicGrantInput[] =>
  normalizeBasicGrantInputs(
    entries.map(entry => ({
      role: entry.role,
      resourceType: entry.resourceType,
      resourceID: entry.resourceID,
      inherit: entry.inherit,
    }))
  );

export const isBasicAccessGrant = (grant: AccessGrantRecord) => {
  const role = (grant.role || '').trim().toLowerCase();
  const resourceType = (grant.resourceType || '').trim();
  return (
    (resourceType === 'folder' || resourceType === 'platform') &&
    (role === BASIC_ROLE_VIEWER || role === BASIC_ROLE_DEVELOPER || role === BASIC_ROLE_OWNER || role === BASIC_ROLE_ADMIN)
  );
};

export const accessGrantMatchesUser = (grant: AccessGrantRecord, user: UserSummary) => {
  const subjectType = (grant.subjectType || '').trim();
  const subjectID = (grant.subjectID || '').trim();
  if (subjectType !== 'user' || !subjectID) return false;
  return subjectID === user.id || subjectID === user.sub || subjectID === user.email;
};

export const accessGrantMatchesServiceAccount = (grant: AccessGrantRecord, account: ServiceAccountSummary) => {
  const subjectType = (grant.subjectType || '').trim();
  const subjectID = (grant.subjectID || '').trim();
  if (subjectType !== 'service_account' || !subjectID) return false;
  return subjectID === account.sub || subjectID === account.id;
};

export const normalizeAdminPolicies = (records: RolePermission[]): RolePermission[] => {
  const deduped = records.filter((entry, idx, arr) => idx === arr.findIndex(other => policyKey(other) === policyKey(entry)));
  const filtered = deduped.filter(
    entry => !isDefaultAdmin(entry.role) || (entry.obj === DEFAULT_ADMIN_POLICY_OBJ && entry.act === DEFAULT_ADMIN_POLICY_ACT)
  );
  const hasCanonicalAdmin = filtered.some(
    entry => isDefaultAdmin(entry.role) && entry.obj === DEFAULT_ADMIN_POLICY_OBJ && entry.act === DEFAULT_ADMIN_POLICY_ACT
  );
  if (!hasCanonicalAdmin) {
    filtered.push({
      role: DEFAULT_ADMIN_ROLE,
      name: 'Admin all access',
      obj: DEFAULT_ADMIN_POLICY_OBJ,
      act: DEFAULT_ADMIN_POLICY_ACT,
    });
  }
  return filtered;
};

export function normalizeAccessGrantRecord(value: unknown): AccessGrantRecord | null {
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
  };
}
