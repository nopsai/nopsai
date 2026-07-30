import {
  DEFAULT_ADMIN_ROLE,
  isProtectedAccessRole,
  type IdentityProviderRecord,
  type RoleDefinition,
  type RolePermission,
  type ServiceAccountSummary,
  type UserSummary,
} from './model.js';

export type AccessPresetID = 'viewer' | 'developer' | 'owner' | 'admin';

export type AccessSummaryMetric = {
  id: string;
  label: string;
  value: string;
  hint: string;
  tone?: 'success' | 'warning' | 'neutral';
};

export const ACCESS_ROLE_PRESETS: Array<{
  id: AccessPresetID;
  label: string;
  description: string;
}> = [
  {
    id: 'viewer',
    label: 'Viewer',
    description: 'Read-only access to teams, pipelines, runs, logs, triggers, and metadata.',
  },
  {
    id: 'developer',
    label: 'Developer',
    description: 'Viewer access plus create, update, execute, and write access for day-to-day delivery work.',
  },
  {
    id: 'owner',
    label: 'Owner',
    description: 'Developer access plus deletes, secret reads, and ACL management inside an owned scope.',
  },
  {
    id: 'admin',
    label: 'Admin',
    description: 'Platform-wide access through the normal AAA path, with sensitive actions still audited.',
  },
];

export const ACCESS_SECTION_CONTENT: Record<
  'users' | 'service-accounts' | 'roles' | 'identity-providers' | 'policies',
  { title: string; description: string; searchPlaceholder: string; resultsLabel: string }
> = {
  users: {
    title: 'People and accounts',
    description: 'See who can sign in, what they can do, and which accounts still need access assigned.',
    searchPlaceholder: 'Search by username, email, or role',
    resultsLabel: 'people',
  },
  'service-accounts': {
    title: 'Service accounts',
    description: 'Manage token-only identities for integrations, automation, and service-to-service access.',
    searchPlaceholder: 'Search by service account, contact, token, or role',
    resultsLabel: 'service accounts',
  },
  roles: {
    title: 'Reusable role bundles',
    description: 'Shape access around simple roles like viewer and developer, then map those bundles to people.',
    searchPlaceholder: 'Search roles, included policies, or assigned users',
    resultsLabel: 'roles',
  },
  'identity-providers': {
    title: 'Identity providers',
    description: 'Configure Okta, Entra ID, Keycloak, Google, GitHub, and generic OIDC providers plus email-domain discovery.',
    searchPlaceholder: 'Search providers, issuers, domains, or client IDs',
    resultsLabel: 'identity providers',
  },
  policies: {
    title: 'Underlying AAA rules',
    description: 'Low-level resource and action rules that power your friendlier product roles.',
    searchPlaceholder: 'Search policies, resources, actions, or role names',
    resultsLabel: 'policies',
  },
};

export function accessPresetIDForRole(roleName: string): AccessPresetID | null {
  const normalized = (roleName || '').trim().toLowerCase();
  if (!normalized) return null;
  if (normalized === DEFAULT_ADMIN_ROLE || normalized === 'admin' || normalized.endsWith('-admin')) return 'admin';
  if (normalized === 'owner' || normalized.endsWith('-owner')) return 'owner';
  if (normalized === 'developer' || normalized.endsWith('-developer')) return 'developer';
  if (normalized === 'viewer' || normalized.endsWith('-viewer')) return 'viewer';
  return null;
}

export function accessPresetForRole(roleName: string) {
  const presetID = accessPresetIDForRole(roleName);
  return presetID ? ACCESS_ROLE_PRESETS.find(preset => preset.id === presetID) ?? null : null;
}

export function accessPresetToneClass(roleName: string) {
  const presetID = accessPresetIDForRole(roleName);
  return presetID ? `access-chip--tone-${presetID}` : 'access-chip--muted';
}

export function matchesAccessSearch(query: string, ...values: Array<string | undefined>) {
  if (!query) return true;
  return values.some(value => (value || '').toLowerCase().includes(query));
}

export function formatAccessCount(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

export function formatAccessTimestamp(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

export function buildAccessSummaryMetrics(input: {
  users: UserSummary[];
  serviceAccounts: ServiceAccountSummary[];
  roles: RoleDefinition[];
  policies: RolePermission[];
  identityProviders: IdentityProviderRecord[];
}): AccessSummaryMetric[] {
  const activeUsers = input.users.filter(
    user => (user.status || '').trim().toLowerCase() === 'active',
  ).length;
  const inactiveUsers = Math.max(input.users.length - activeUsers, 0);
  const tokenCount = input.serviceAccounts.reduce(
    (total, account) => total + (account.token_count || 0),
    0
  );
  const protectedRoles = input.roles.filter(role =>
    isProtectedAccessRole(role.role)
  ).length;
  const rolesWithPolicies = input.roles.filter(role => role.policies.length > 0).length;
  const policyCoverage = input.roles.length
    ? Math.round((rolesWithPolicies / input.roles.length) * 100)
    : 0;
  const enabledProviders = input.identityProviders.filter(provider => provider.enabled).length;

  return [
    {
      id: 'active-users',
      label: 'Active users',
      value: String(activeUsers),
      hint: inactiveUsers
        ? `${inactiveUsers} inactive or pending`
        : `${input.users.length} total local and SSO users`,
      tone: 'success',
    },
    {
      id: 'service-accounts',
      label: 'Service accounts',
      value: String(input.serviceAccounts.length),
      hint: `${formatAccessCount(tokenCount, 'registered token')}`,
      tone: 'neutral',
    },
    {
      id: 'reusable-roles',
      label: 'Reusable roles',
      value: String(input.roles.length),
      hint: `${formatAccessCount(protectedRoles, 'protected system role')}`,
      tone: protectedRoles ? 'warning' : 'neutral',
    },
    {
      id: 'policy-coverage',
      label: 'Policy coverage',
      value: `${policyCoverage}%`,
      hint: `${formatAccessCount(input.policies.length, 'visible AAA rule')}${
        enabledProviders ? `, ${formatAccessCount(enabledProviders, 'IdP')} enabled` : ''
      }`,
      tone: policyCoverage >= 90 ? 'success' : 'warning',
    },
  ];
}
