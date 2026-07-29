import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { fetchSystemJson } from '../api';
import {
  deleteIdentityProvider as deleteIdentityProviderAPI,
  fetchAccessResourceCatalog,
  fetchIdentityProvidersState,
  saveIdentityProvider as saveIdentityProviderAPI,
  saveIdentityProviderSettings as saveIdentityProviderSettingsAPI,
} from './api';
import {
  POLICY_TEMPLATE_ROLE,
  isProtectedAccessRole,
  normalizeAccessGrantRecord,
  normalizeAdminPolicies,
  normalizeBasicGrantInputs,
  policyKey,
  policyLabel,
  policyName,
} from './model';
import type {
  AccessGrantRecord,
  BasicGrantInput,
  IdentityProviderFormState,
  IdentityProviderSettings,
  IdentityProvidersState,
  RoleDefinition,
  RolePermission,
  RolePolicyDraft,
  ServiceAccountSummary,
  ServiceAccountToken,
  UserSummary,
} from './model';
import type { AccessPanelProps } from '../AccessPanel';
import { asRecord, normalizeListPayload, readString } from '../data';
import { createEmptyAccessResourceCatalog } from './resourceCatalog';

type ToastTone = 'success' | 'error' | 'info';

type UseSystemAccessOptions = {
  enabled: boolean;
  addToast: (message: string, tone?: ToastTone) => void;
};

const createEmptyUserForm = () => ({ sub: '', email: '', password: '', roles: [] as string[] });
const createEmptyServiceAccountForm = () => ({ sub: '', email: '', tokenName: 'default', roles: [] as string[] });
const createEmptyPermissionForm = () => ({ name: '', obj: 'pipeline:*', act: 'pipeline.read' });
const createEmptyIdentityProvidersState = (): IdentityProvidersState => ({
  settings: {
    local_enabled: true,
    oidc_enabled: false,
    auto_create_users: false,
    default_role: '',
    allow_email_linking: false,
  },
  providers: [],
  domain_mappings: {},
});

export function useSystemAccess({ enabled, addToast }: UseSystemAccessOptions): AccessPanelProps {
  const [users, setUsers] = useState<UserSummary[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [usersError, setUsersError] = useState<string | null>(null);
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccountSummary[]>([]);
  const [serviceAccountsLoading, setServiceAccountsLoading] = useState(false);
  const [serviceAccountsError, setServiceAccountsError] = useState<string | null>(null);
  const [accessGrants, setAccessGrants] = useState<AccessGrantRecord[]>([]);
  const [accessGrantsLoading, setAccessGrantsLoading] = useState(false);
  const [accessGrantsError, setAccessGrantsError] = useState<string | null>(null);
  const [identityProvidersState, setIdentityProvidersState] = useState<IdentityProvidersState>(createEmptyIdentityProvidersState);
  const [identityProvidersLoading, setIdentityProvidersLoading] = useState(false);
  const [identityProvidersError, setIdentityProvidersError] = useState<string | null>(null);
  const [policies, setPolicies] = useState<RolePermission[]>([]);
  const [policyTemplates, setPolicyTemplates] = useState<RolePermission[]>([]);
  const [policiesLoading, setPoliciesLoading] = useState(false);
  const [policiesError, setPoliciesError] = useState<string | null>(null);
  const [newUser, setNewUser] = useState(createEmptyUserForm);
  const [newServiceAccount, setNewServiceAccount] = useState(createEmptyServiceAccountForm);
  const [newPermission, setNewPermission] = useState(createEmptyPermissionForm);
  const [resourceCatalog, setResourceCatalog] = useState(createEmptyAccessResourceCatalog);

  const loadUsers = useCallback(async () => {
    setUsersLoading(true);
    setUsersError(null);
    try {
      const payload = await fetchSystemJson('/v1/admin/users');
      const records = normalizeListPayload(payload, ['users', 'items', 'data', 'records', 'results']);
      if (!records) {
        setUsersError('Unexpected response');
        return;
      }
      setUsers(records as UserSummary[]);
    } catch (error) {
      setUsersError(error instanceof Error ? error.message : 'Unable to load users');
    } finally {
      setUsersLoading(false);
    }
  }, []);

  const loadServiceAccounts = useCallback(async () => {
    setServiceAccountsLoading(true);
    setServiceAccountsError(null);
    try {
      const payload = await fetchSystemJson('/v1/admin/service-accounts');
      const records = normalizeListPayload(payload, ['service_accounts', 'items', 'data', 'records', 'results']);
      if (!records) {
        setServiceAccountsError('Unexpected response');
        return;
      }
      setServiceAccounts(records as ServiceAccountSummary[]);
    } catch (error) {
      setServiceAccountsError(error instanceof Error ? error.message : 'Unable to load service accounts');
    } finally {
      setServiceAccountsLoading(false);
    }
  }, []);

  const loadAccessGrants = useCallback(async () => {
    setAccessGrantsLoading(true);
    setAccessGrantsError(null);
    try {
      const payload = await fetchSystemJson('/v1/access/grants');
      const records = normalizeListPayload(payload, ['grants', 'access_grants', 'accessGrants', 'items', 'data', 'records', 'results']);
      if (!records) {
        setAccessGrantsError('Unexpected response');
        return;
      }
      setAccessGrants(records.map(item => normalizeAccessGrantRecord(item)).filter(Boolean) as AccessGrantRecord[]);
    } catch (error) {
      setAccessGrantsError(error instanceof Error ? error.message : 'Unable to load basic roles');
    } finally {
      setAccessGrantsLoading(false);
    }
  }, []);

  const loadPolicies = useCallback(async () => {
    setPoliciesLoading(true);
    setPoliciesError(null);
    try {
      const payload = await fetchSystemJson('/v1/admin/roles');
      const records = normalizeListPayload(payload, ['roles', 'permissions', 'items', 'data', 'records', 'results']);
      if (!records) {
        setPoliciesError('Unexpected response');
        return;
      }
      const rolePermissions = records as RolePermission[];
      const templates = rolePermissions.filter(p => p.role === POLICY_TEMPLATE_ROLE);
      const rolePolicies = normalizeAdminPolicies(rolePermissions.filter(p => p.role !== POLICY_TEMPLATE_ROLE));
      setPolicyTemplates(templates);
      setPolicies(rolePolicies);
    } catch (error) {
      setPoliciesError(error instanceof Error ? error.message : 'Unable to load policies');
    } finally {
      setPoliciesLoading(false);
    }
  }, []);

  const loadResourceCatalog = useCallback(async () => {
    setResourceCatalog(await fetchAccessResourceCatalog());
  }, []);

  const loadIdentityProviders = useCallback(async () => {
    setIdentityProvidersLoading(true);
    setIdentityProvidersError(null);
    try {
      setIdentityProvidersState(await fetchIdentityProvidersState());
    } catch (error) {
      setIdentityProvidersError(error instanceof Error ? error.message : 'Unable to load identity providers');
    } finally {
      setIdentityProvidersLoading(false);
    }
  }, []);

  const createUser = useCallback(
    async (e: FormEvent<HTMLFormElement>, options?: { basicGrants?: BasicGrantInput[] }): Promise<boolean> => {
      e.preventDefault();
      const roleAssignments = (newUser.roles || [])
        .map(role => role.trim())
        .filter(Boolean)
        .filter((role, index, roles) => roles.indexOf(role) === index);
      const basicGrants = normalizeBasicGrantInputs(options?.basicGrants || []);
      if (roleAssignments.length === 0 && basicGrants.length === 0) {
        addToast('Add at least one access role or basic role before creating a user.', 'error');
        return false;
      }
      try {
        const sub = newUser.sub.trim();
        const email = newUser.email.trim();
        const primaryRole = roleAssignments[0] || '';
        const created = (await fetchSystemJson('/v1/admin/users', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sub,
            email,
            password: newUser.password,
            role: primaryRole,
          }),
        })) as Partial<UserSummary> & { user_id?: string; userId?: string };

        let userId: string | undefined =
          (created && (created.id || created.user_id || created.userId)) || undefined;

        if (!userId) {
          const createdRecords = normalizeListPayload(created, ['users', 'items', 'data', 'records', 'results']);
          const match = createdRecords?.find(item => {
            const record = asRecord(item);
            if (!record) return false;
            return readString(record.sub) === sub || readString(record.email) === email;
          });
          userId = match ? readString(asRecord(match)?.id) : undefined;
        }

        if (!userId) {
          try {
            const list = await fetchSystemJson('/v1/admin/users');
            const records = normalizeListPayload(list, ['users', 'items', 'data', 'records', 'results']);
            if (records) {
              const match = (records as UserSummary[]).find(u => u.sub === sub || u.email === email);
              userId = match?.id;
            }
          } catch {
            // Best-effort lookup. Role assignment below will fail clearly if the ID remains unknown.
          }
        }

        if (!userId) {
          addToast('User created but ID not found; roles not assigned.', 'error');
          await loadUsers();
          return false;
        }

        for (const role of roleAssignments.slice(1)) {
          try {
            await fetchSystemJson('/v1/admin/user-roles', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                user_id: userId,
                role,
              }),
            });
          } catch (error) {
            console.error('Failed to assign role', role, error);
          }
        }

        for (const grant of basicGrants) {
          try {
            await fetchSystemJson('/v1/access/grants', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                subject_type: 'user',
                subject_id: userId,
                role: grant.role,
                resource_type: grant.resourceType,
                resource_id: grant.resourceID,
                inherit: grant.inherit,
              }),
            });
          } catch (error) {
            if (error instanceof Error && error.message.toLowerCase().includes('already exists')) {
              continue;
            }
            throw error;
          }
        }

        const savedParts = [
          roleAssignments.length ? `${roleAssignments.length} access role(s)` : '',
          basicGrants.length ? `${basicGrants.length} basic role(s)` : '',
        ].filter(Boolean);
        addToast(`User ${sub} saved with ${savedParts.join(' and ')}`, 'success');
        setNewUser(createEmptyUserForm());
        await loadUsers();
        if (basicGrants.length > 0) {
          await loadAccessGrants();
        }
        return true;
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to create user', 'error');
        return false;
      }
    },
    [addToast, loadAccessGrants, loadUsers, newUser.email, newUser.password, newUser.roles, newUser.sub]
  );

  const createServiceAccount = useCallback(
    async (e: FormEvent<HTMLFormElement>, options?: { basicGrants?: BasicGrantInput[] }): Promise<ServiceAccountToken | null> => {
      e.preventDefault();
      const roleAssignments = (newServiceAccount.roles || [])
        .map(role => role.trim())
        .filter(Boolean)
        .filter((role, index, roles) => roles.indexOf(role) === index);
      const basicGrants = normalizeBasicGrantInputs(options?.basicGrants || []);
      if (roleAssignments.length === 0 && basicGrants.length === 0) {
        addToast('Add at least one access role or basic role before creating a service account.', 'error');
        return null;
      }
      try {
        const sub = newServiceAccount.sub.trim();
        const email = newServiceAccount.email.trim();
        const primaryRole = roleAssignments[0] || '';
        const created = await fetchSystemJson('/v1/admin/service-accounts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sub,
            email,
            role: primaryRole,
            token_name: newServiceAccount.tokenName.trim() || 'default',
          }),
        });
        const createdRecord = asRecord(created);
        const createdAccountRecord = asRecord(createdRecord?.service_account);
        const serviceAccountId = readString(createdAccountRecord?.id);
        const serviceAccountSub = readString(createdAccountRecord?.sub) || sub;
        const token = asRecord(createdRecord?.token) as ServiceAccountToken | null;

        if (!serviceAccountId || !serviceAccountSub) {
          addToast('Service account created but ID not found; roles not assigned.', 'error');
          await loadServiceAccounts();
          return token;
        }

        for (const role of roleAssignments.slice(1)) {
          try {
            await fetchSystemJson('/v1/admin/service-account-roles', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                service_account_id: serviceAccountId,
                role,
              }),
            });
          } catch (error) {
            console.error('Failed to assign service account role', role, error);
          }
        }

        for (const grant of basicGrants) {
          try {
            await fetchSystemJson('/v1/access/grants', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                subject_type: 'service_account',
                subject_id: serviceAccountSub,
                role: grant.role,
                resource_type: grant.resourceType,
                resource_id: grant.resourceID,
                inherit: grant.inherit,
              }),
            });
          } catch (error) {
            if (error instanceof Error && error.message.toLowerCase().includes('already exists')) {
              continue;
            }
            throw error;
          }
        }

        const savedParts = [
          roleAssignments.length ? `${roleAssignments.length} access role(s)` : '',
          basicGrants.length ? `${basicGrants.length} basic role(s)` : '',
        ].filter(Boolean);
        addToast(`Service account ${serviceAccountSub} saved with ${savedParts.join(' and ')}`, 'success');
        setNewServiceAccount(createEmptyServiceAccountForm());
        await loadServiceAccounts();
        if (basicGrants.length > 0) {
          await loadAccessGrants();
        }
        return token;
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to create service account', 'error');
        return null;
      }
    },
    [addToast, loadAccessGrants, loadServiceAccounts, newServiceAccount.email, newServiceAccount.roles, newServiceAccount.sub, newServiceAccount.tokenName]
  );

  const createPermission = useCallback(
    async (e: FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      const obj = newPermission.obj.trim();
      const act = newPermission.act.trim();
      const name = newPermission.name.trim();
      if (!name || !obj || !act) {
        addToast('Policy label, resource, and action are required.', 'error');
        return;
      }
      try {
        await fetchSystemJson('/v1/admin/roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: POLICY_TEMPLATE_ROLE,
            name,
            obj,
            act,
          }),
        });
        addToast('Policy added', 'success');
        setNewPermission(createEmptyPermissionForm());
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to add policy', 'error');
      }
    },
    [addToast, loadPolicies, newPermission.act, newPermission.name, newPermission.obj]
  );

  const deleteUser = useCallback(
    async (userId: string) => {
      try {
        await fetchSystemJson(`/v1/admin/users/${encodeURIComponent(userId)}`, { method: 'DELETE' });
        addToast('User deleted', 'success');
        void loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to delete user', 'error');
      }
    },
    [addToast, loadUsers]
  );

  const deleteServiceAccount = useCallback(
    async (serviceAccountId: string) => {
      try {
        await fetchSystemJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}`, { method: 'DELETE' });
        addToast('Service account deleted', 'success');
        await loadServiceAccounts();
        await loadAccessGrants();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to delete service account', 'error');
      }
    },
    [addToast, loadAccessGrants, loadServiceAccounts]
  );

  const createAccessGrant = useCallback(
    async (input: { userID: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => {
      const role = input.role.trim().toLowerCase();
      const resourceType = input.resourceType.trim();
      const resourceID = input.resourceID.trim();
      await fetchSystemJson('/v1/access/grants', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          subject_type: 'user',
          subject_id: input.userID,
          role,
          resource_type: resourceType,
          resource_id: resourceID,
          inherit: input.inherit,
        }),
      });
      addToast('Basic role saved', 'success');
      await loadAccessGrants();
    },
    [addToast, loadAccessGrants]
  );

  const createServiceAccountAccessGrant = useCallback(
    async (input: { serviceAccountSub: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => {
      const role = input.role.trim().toLowerCase();
      const resourceType = input.resourceType.trim();
      const resourceID = input.resourceID.trim();
      await fetchSystemJson('/v1/access/grants', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          subject_type: 'service_account',
          subject_id: input.serviceAccountSub,
          role,
          resource_type: resourceType,
          resource_id: resourceID,
          inherit: input.inherit,
        }),
      });
      addToast('Basic role saved', 'success');
      await loadAccessGrants();
    },
    [addToast, loadAccessGrants]
  );

  const deleteAccessGrant = useCallback(
    async (grantID: string) => {
      await fetchSystemJson(`/v1/access/grants/${encodeURIComponent(grantID)}`, { method: 'DELETE' });
      addToast('Basic role removed', 'success');
      await loadAccessGrants();
    },
    [addToast, loadAccessGrants]
  );

  const deletePolicy = useCallback(
    async (policy: RolePermission) => {
      if (isProtectedAccessRole(policy.role)) {
        addToast('Default role policies cannot be deleted.', 'error');
        return;
      }
      try {
        await fetchSystemJson('/v1/admin/roles', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: policy.role,
            obj: policy.obj,
            act: policy.act,
          }),
        });
        addToast('Policy removed', 'success');
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to remove policy', 'error');
      }
    },
    [addToast, loadPolicies]
  );

  const deleteRoleDefinition = useCallback(
    async (role: RoleDefinition) => {
      if (isProtectedAccessRole(role.role)) {
        addToast('Default roles cannot be deleted.', 'error');
        return;
      }
      const inUse =
        users.some(user => (user.roles || []).some(r => r.role === role.role)) ||
        serviceAccounts.some(account => (account.roles || []).some(r => r.role === role.role));
      if (inUse) {
        addToast('Cannot delete a role that is still assigned. Remove assignments first.', 'error');
        return;
      }
      try {
        for (const entry of role.policies) {
          await fetchSystemJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.role,
              obj: entry.obj,
              act: entry.act,
            }),
          });
        }
        addToast('Role removed', 'success');
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to remove role', 'error');
      }
    },
    [addToast, loadPolicies, serviceAccounts, users]
  );

  const saveRoleDefinition = useCallback(
    async ({ role, policies: drafts, original }: { role: string; policies: RolePolicyDraft[]; original?: RolePermission[] }) => {
      const roleName = role.trim();
      if (isProtectedAccessRole(roleName)) {
        addToast('Default roles cannot be modified.', 'error');
        throw new Error('Default roles are protected');
      }
      const templateByName = new Map<string, RolePermission>();
      policyTemplates.forEach(template => {
        const key = (template.name || policyLabel(template)).trim();
        if (key) templateByName.set(key, template);
      });
      const resolved = drafts
        .map(item => {
          const name = (item.name || '').trim();
          const template = name ? templateByName.get(name) : undefined;
          const obj = (template?.obj || item.obj || '').trim();
          const act = (template?.act || item.act || '').trim();
          const finalName = name || policyName(obj, act);
          return { name: finalName, obj, act };
        })
        .filter(item => item.name && item.obj && item.act);
      if (!roleName || resolved.length === 0) {
        addToast('Role name and at least one policy are required.', 'error');
        throw new Error('Role validation failed');
      }
      const unresolved = drafts.filter(draft => !(draft.name || '').trim() && (!draft.obj.trim() || !draft.act.trim()));
      if (unresolved.length > 0) {
        addToast('Please select a policy name for each entry.', 'error');
        throw new Error('Role validation failed');
      }
      const normalizeName = (input: { name?: string; obj: string; act: string }) => (input.name || '').trim() || policyName(input.obj, input.act);
      const existing = original || [];
      const existingKeys = new Set(existing.map(policyKey));
      const nextPolicies = resolved.map(item => ({ role: roleName, name: item.name, obj: item.obj, act: item.act }));
      const nextKeys = new Set(nextPolicies.map(policyKey));
      const toRemove = existing.filter(item => !nextKeys.has(policyKey(item)));
      const toAdd = nextPolicies.filter(item => !existingKeys.has(policyKey(item)));
      const renamed = existing
        .map(existingItem => {
          const match = nextPolicies.find(next => policyKey(next) === policyKey(existingItem));
          if (!match) return null;
          const currentName = normalizeName(existingItem);
          const nextName = normalizeName(match);
          if (currentName === nextName) return null;
          return { previous: existingItem, next: match };
        })
        .filter(Boolean) as { previous: RolePermission; next: typeof nextPolicies[number] }[];

      try {
        for (const entry of toRemove) {
          await fetchSystemJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.role,
              obj: entry.obj,
              act: entry.act,
            }),
          });
        }
        for (const entry of renamed) {
          await fetchSystemJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.previous.role,
              obj: entry.previous.obj,
              act: entry.previous.act,
            }),
          });
        }
        for (const entry of toAdd) {
          await fetchSystemJson('/v1/admin/roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.role,
              name: entry.name,
              obj: entry.obj,
              act: entry.act,
            }),
          });
        }
        for (const entry of renamed) {
          await fetchSystemJson('/v1/admin/roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.next.role,
              name: entry.next.name,
              obj: entry.next.obj,
              act: entry.next.act,
            }),
          });
        }
        addToast(`Role ${roleName} saved`, 'success');
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to save role policies', 'error');
        throw error;
      }
    },
    [addToast, loadPolicies, policyTemplates]
  );

  const editPolicy = useCallback(
    async (current: RolePermission, next: { role: string; name: string; obj: string; act: string }) => {
      const nextRole = next.role.trim();
      const nextObj = next.obj.trim();
      const nextAct = next.act.trim();
      const nextName = next.name.trim() || policyName(nextObj, nextAct);
      if (isProtectedAccessRole(current.role) || isProtectedAccessRole(nextRole)) {
        addToast('Default role policies cannot be modified.', 'error');
        throw new Error('Default role policies are protected');
      }
      if (!nextRole || !nextObj || !nextAct) {
        addToast('Role, resource, and action are required.', 'error');
        throw new Error('Policy validation failed');
      }
      const sameKey = policyKey({
        role: current.role,
        obj: current.obj,
        act: current.act,
      }) ===
        policyKey({
          role: nextRole,
          obj: nextObj,
          act: nextAct,
        });
      const currentName = (current.name || '').trim() || policyName(current.obj, current.act);
      if (sameKey && currentName === nextName) {
        addToast('No changes to save for this policy.', 'info');
        return;
      }
      const normalizePolicyDisplayName = (policy: RolePermission) => (policy.name || '').trim() || policyName(policy.obj, policy.act);
      const linkedRolePolicies =
        current.role === POLICY_TEMPLATE_ROLE
          ? policies.filter(
              policy =>
                policy.role !== POLICY_TEMPLATE_ROLE &&
                !isProtectedAccessRole(policy.role) &&
                policy.obj === current.obj &&
                policy.act === current.act &&
                normalizePolicyDisplayName(policy) === currentName
            )
          : [];
      const nextAlreadyExistsInRole = (linkedPolicy: RolePermission) =>
        policies.some(
          policy =>
            policy.role === linkedPolicy.role &&
            policy.obj === nextObj &&
            policy.act === nextAct &&
            !(
              policy.obj === linkedPolicy.obj &&
              policy.act === linkedPolicy.act &&
              normalizePolicyDisplayName(policy) === normalizePolicyDisplayName(linkedPolicy)
            )
        );
      try {
        await fetchSystemJson('/v1/admin/roles', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: current.role,
            obj: current.obj,
            act: current.act,
          }),
        });
        for (const policy of linkedRolePolicies) {
          await fetchSystemJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: policy.role,
              obj: policy.obj,
              act: policy.act,
            }),
          });
        }
        await fetchSystemJson('/v1/admin/roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: nextRole,
            name: nextName,
            obj: nextObj,
            act: nextAct,
          }),
        });
        for (const policy of linkedRolePolicies) {
          if (nextAlreadyExistsInRole(policy)) continue;
          await fetchSystemJson('/v1/admin/roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: policy.role,
              name: nextName,
              obj: nextObj,
              act: nextAct,
            }),
          });
        }
        addToast('Policy updated', 'success');
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to update policy', 'error');
        throw error;
      }
    },
    [addToast, loadPolicies, policies]
  );

  const updateUserRoles = useCallback(
    async (userId: string, nextRoles: string[], previousRoles: string[]) => {
      const cleanedPrev = previousRoles.map(role => role.trim()).filter(Boolean);
      const cleanedNext = nextRoles
        .map(role => role.trim())
        .filter(Boolean)
        .filter((role, index, roles) => roles.indexOf(role) === index);
      const prevKeys = new Set(cleanedPrev);
      const nextKeys = new Set(cleanedNext);
      const toRemove = cleanedPrev.filter(role => !nextKeys.has(role));
      const toAdd = cleanedNext.filter(role => !prevKeys.has(role));
      try {
        for (const role of toRemove) {
          await fetchSystemJson('/v1/admin/user-roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              user_id: userId,
              role,
            }),
          });
        }
        for (const role of toAdd) {
          await fetchSystemJson('/v1/admin/user-roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              user_id: userId,
              role,
            }),
          });
        }
        if (toAdd.length === 0 && toRemove.length === 0) {
          addToast('No changes to save for this user.', 'info');
        } else {
          addToast('User access updated', 'success');
        }
        await loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to update user roles', 'error');
        throw error;
      }
    },
    [addToast, loadUsers]
  );

  const updateUser = useCallback(
    async (userId: string, input: { email?: string; status?: string; password?: string }) => {
      const payload: Record<string, string> = {};
      if (input.email) payload.email = input.email.trim();
      if (input.status) payload.status = input.status.trim();
      if (input.password) payload.password = input.password;
      if (Object.keys(payload).length === 0) return;
      try {
        await fetchSystemJson(`/v1/admin/users/${encodeURIComponent(userId)}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        addToast('User updated', 'success');
        await loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to update user', 'error');
        throw error;
      }
    },
    [addToast, loadUsers]
  );

  const updateServiceAccountRoles = useCallback(
    async (serviceAccountId: string, nextRoles: string[], previousRoles: string[]) => {
      const cleanedPrev = previousRoles.map(role => role.trim()).filter(Boolean);
      const cleanedNext = nextRoles
        .map(role => role.trim())
        .filter(Boolean)
        .filter((role, index, roles) => roles.indexOf(role) === index);
      const prevKeys = new Set(cleanedPrev);
      const nextKeys = new Set(cleanedNext);
      const toRemove = cleanedPrev.filter(role => !nextKeys.has(role));
      const toAdd = cleanedNext.filter(role => !prevKeys.has(role));
      try {
        for (const role of toRemove) {
          await fetchSystemJson('/v1/admin/service-account-roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              service_account_id: serviceAccountId,
              role,
            }),
          });
        }
        for (const role of toAdd) {
          await fetchSystemJson('/v1/admin/service-account-roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              service_account_id: serviceAccountId,
              role,
            }),
          });
        }
        if (toAdd.length === 0 && toRemove.length === 0) {
          addToast('No changes to save for this service account.', 'info');
        } else {
          addToast('Service account access updated', 'success');
        }
        await loadServiceAccounts();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to update service account roles', 'error');
        throw error;
      }
    },
    [addToast, loadServiceAccounts]
  );

  const updateServiceAccount = useCallback(
    async (serviceAccountId: string, input: { email?: string; status?: string }) => {
      const payload: Record<string, string> = {};
      if (input.email) payload.email = input.email.trim();
      if (input.status) payload.status = input.status.trim();
      if (Object.keys(payload).length === 0) return;
      try {
        await fetchSystemJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        addToast('Service account updated', 'success');
        await loadServiceAccounts();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to update service account', 'error');
        throw error;
      }
    },
    [addToast, loadServiceAccounts]
  );

  const loadServiceAccountTokens = useCallback(
    async (serviceAccountId: string): Promise<ServiceAccountToken[]> => {
      const payload = await fetchSystemJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}/tokens`);
      const records = normalizeListPayload(payload, ['tokens', 'items', 'data', 'records', 'results']);
      return records ? (records as ServiceAccountToken[]) : [];
    },
    []
  );

  const createServiceAccountToken = useCallback(
    async (serviceAccountId: string, name: string): Promise<ServiceAccountToken> => {
      const token = (await fetchSystemJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}/tokens`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      })) as ServiceAccountToken;
      addToast('Service account token created', 'success');
      await loadServiceAccounts();
      return token;
    },
    [addToast, loadServiceAccounts]
  );

  const revokeServiceAccountToken = useCallback(
    async (serviceAccountId: string, tokenId: string) => {
      await fetchSystemJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}/tokens/${encodeURIComponent(tokenId)}`, {
        method: 'DELETE',
      });
      addToast('Service account token revoked', 'success');
      await loadServiceAccounts();
    },
    [addToast, loadServiceAccounts]
  );

  const saveIdentityProviderSettings = useCallback(
    async (settings: IdentityProviderSettings, mappings: Record<string, string>) => {
      try {
        setIdentityProvidersState(await saveIdentityProviderSettingsAPI(settings, mappings));
        addToast('Identity provider settings saved', 'success');
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to save identity provider settings', 'error');
        throw error;
      }
    },
    [addToast]
  );

  const saveIdentityProvider = useCallback(
    async (form: IdentityProviderFormState) => {
      const providerID = form.id.trim();
      if (!providerID) {
        addToast('Provider ID is required.', 'error');
        return;
      }
      try {
        setIdentityProvidersState(await saveIdentityProviderAPI(form));
        addToast('Identity provider saved', 'success');
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to save identity provider', 'error');
        throw error;
      }
    },
    [addToast]
  );

  const deleteIdentityProvider = useCallback(
    async (providerID: string) => {
      try {
        await deleteIdentityProviderAPI(providerID);
        addToast('Identity provider deleted', 'success');
        await loadIdentityProviders();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to delete identity provider', 'error');
        throw error;
      }
    },
    [addToast, loadIdentityProviders]
  );

  useEffect(() => {
    if (!enabled) return;
    void loadUsers();
    void loadServiceAccounts();
    void loadAccessGrants();
    void loadIdentityProviders();
    void loadPolicies();
    void loadResourceCatalog();
  }, [enabled, loadAccessGrants, loadIdentityProviders, loadPolicies, loadResourceCatalog, loadServiceAccounts, loadUsers]);

  return {
    users,
    loading: usersLoading,
    error: usersError,
    serviceAccounts,
    serviceAccountsLoading,
    serviceAccountsError,
    accessGrants,
    accessGrantsLoading,
    accessGrantsError,
    identityProviders: identityProvidersState.providers,
    identityProviderSettings: identityProvidersState.settings,
    identityProviderDomainMappings: identityProvidersState.domain_mappings,
    identityProvidersLoading,
    identityProvidersError,
    policies,
    policiesLoading,
    policiesError,
    resourceCatalog,
    newUser,
    newServiceAccount,
    policyTemplates,
    onChangeUser: setNewUser,
    onCreateUser: createUser,
    onChangeServiceAccount: setNewServiceAccount,
    onCreateServiceAccount: createServiceAccount,
    onCreatePermission: createPermission,
    newPermission,
    onChangePermission: setNewPermission,
    onDeleteUser: deleteUser,
    onDeleteServiceAccount: deleteServiceAccount,
    onDeletePolicy: deletePolicy,
    onDeleteRoleDefinition: deleteRoleDefinition,
    onSaveRoleDefinition: saveRoleDefinition,
    onEditPolicy: editPolicy,
    onUpdateUserRoles: updateUserRoles,
    onUpdateServiceAccountRoles: updateServiceAccountRoles,
    onCreateAccessGrant: createAccessGrant,
    onCreateServiceAccountAccessGrant: createServiceAccountAccessGrant,
    onDeleteAccessGrant: deleteAccessGrant,
    onSaveIdentityProviderSettings: saveIdentityProviderSettings,
    onSaveIdentityProvider: saveIdentityProvider,
    onDeleteIdentityProvider: deleteIdentityProvider,
    onUpdateUser: updateUser,
    onUpdateServiceAccount: updateServiceAccount,
    onLoadServiceAccountTokens: loadServiceAccountTokens,
    onCreateServiceAccountToken: createServiceAccountToken,
    onRevokeServiceAccountToken: revokeServiceAccountToken,
  };
}
