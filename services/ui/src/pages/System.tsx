import { Link, useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent, type ReactNode } from 'react';
import { buildApiUrl } from '../lib/api';

type ConfigFormState = {
  config_repo_url: string;
  agent_image: string;
  docker_network_name: string;
  default_pipeline_timeout: string;
  llm_agent_timeout: string;
  auto_removal_agent_container: boolean;
  agent_nopsai_api_url: string;
  git_bot_nopsai_api_url: string;
  nopsai_git_bot_api_url: string;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

type ConfigSyncStatus = {
  status: string;
  message?: string;
  details?: Record<string, number>;
  started_at?: string;
  completed_at?: string;
};

type Runner = {
  runnerId: string;
  scopes: string[];
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  lastHeartbeatUnix: number;
  allowDispatch: boolean;
  metadata: Record<string, string>;
};

type RunnerActiveRun = {
  runId: string;
  pipeline: string;
  parentStep?: string;
  triggerId?: string;
};

type RunnerMeta = {
  connectionId: string;
  hostname: string;
  network: string;
  activeRuns: RunnerActiveRun[];
};

const initialConfig: ConfigFormState = {
  config_repo_url: '',
  agent_image: '',
  docker_network_name: '',
  default_pipeline_timeout: '',
  llm_agent_timeout: '',
  auto_removal_agent_container: true,
  agent_nopsai_api_url: '',
  git_bot_nopsai_api_url: '',
  nopsai_git_bot_api_url: '',
};

const POLL_INTERVAL_MS = 5000;
const STALE_THRESHOLD_MS = 30_000;
const MAX_VISIBLE_ACTIVE_RUNS = 3;
const POLICY_TEMPLATE_ROLE = '__policy_template__';
const DEFAULT_TENANT_ID = '00000000-0000-0000-0000-000000000001';
const DEFAULT_ADMIN_ROLE = 'nopsai-admin';
const DEFAULT_ADMIN_POLICY_OBJ = '/*';
const DEFAULT_ADMIN_POLICY_ACT = '.*';

type UserRole = {
  tenant_id: string;
  role: string;
};

type RolePermission = {
  role: string;
  tenant_id: string;
  name?: string;
  obj: string;
  act: string;
};

type UserSummary = {
  id: string;
  sub: string;
  email: string;
  status: string;
  last_login?: string;
  roles?: UserRole[];
};

type TenantRecord = {
  id: string;
  name: string;
};

type RolePolicyDraft = {
  name: string;
  obj: string;
  act: string;
};

type RoleDefinition = {
  id: string;
  role: string;
  tenant: string;
  policies: RolePermission[];
};

function SystemPage() {
  const params = useParams<{ tab?: string }>();
  const activeTab = params.tab === 'dispatcher' ? 'dispatcher' : params.tab === 'access' ? 'access' : 'config';

  const isMountedRef = useRef(true);

  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const [config, setConfig] = useState<ConfigFormState>(initialConfig);
  const [envFilePath, setEnvFilePath] = useState('');
  const [configLoading, setConfigLoading] = useState(true);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [syncStatus, setSyncStatus] = useState<ConfigSyncStatus | null>(null);
  const [syncBusy, setSyncBusy] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);

  const [dispatcherLoading, setDispatcherLoading] = useState(false);
  const [dispatcherError, setDispatcherError] = useState<string | null>(null);
  const [dispatcherStatus, setDispatcherStatus] = useState<{
    queuedJobs: number;
    runners: Runner[];
    routing: Record<string, string[]>;
    fetchedAt: number;
  } | null>(null);

  const [runnerActionPending, setRunnerActionPending] = useState<Set<string>>(new Set());

  const [users, setUsers] = useState<UserSummary[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [usersError, setUsersError] = useState<string | null>(null);
  const [policies, setPolicies] = useState<RolePermission[]>([]);
  const [policyTemplates, setPolicyTemplates] = useState<RolePermission[]>([]);
  const [policiesLoading, setPoliciesLoading] = useState(false);
  const [policiesError, setPoliciesError] = useState<string | null>(null);
  const [tenants, setTenants] = useState<TenantRecord[]>([]);
  const [tenantError, setTenantError] = useState<string | null>(null);
  const [newUser, setNewUser] = useState({ sub: '', email: '', password: '', tenant: 'default', roles: [] as { role: string; tenant: string }[] });
  const [newRole, setNewRole] = useState({ userId: '', tenant: '', role: '' });
  const [newPermission, setNewPermission] = useState({ tenant: '', name: '', obj: '/v1/*', act: 'GET|POST|PUT|DELETE' });
  const [ensuringDefaultAdmin, setEnsuringDefaultAdmin] = useState(false);

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const addToast = useCallback((message: string, tone: ToastMessage['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const fetchJson = useCallback(async (path: string, init?: RequestInit): Promise<unknown> => {
    const response = await fetch(buildApiUrl(path), init);
    if (response.status === 204) return null;
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed (${response.status})`);
    }
    const contentType = response.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
      return response.json();
    }
    return response.text();
  }, []);

  const applySystemConfigResponse = useCallback((payload: unknown) => {
    const record = asRecord(payload);
    if (!record) throw new Error('Unexpected system config response');

    const nextConfig: ConfigFormState = {
      config_repo_url: readString(record.config_repo_url),
      agent_image: readString(record.agent_image),
      docker_network_name: readString(record.docker_network_name),
      default_pipeline_timeout: readString(record.default_pipeline_timeout),
      llm_agent_timeout: readString(record.llm_agent_timeout),
      auto_removal_agent_container: Boolean(record.auto_removal_agent_container),
      agent_nopsai_api_url: readString(record.agent_nopsai_api_url),
      git_bot_nopsai_api_url: readString(record.git_bot_nopsai_api_url),
      nopsai_git_bot_api_url: readString(record.nopsai_git_bot_api_url),
    };
    setConfig(nextConfig);
    setEnvFilePath(readString(record.env_file_path));

    const nextSyncStatus = normalizeConfigSyncStatus(record.config_sync_status);
    setSyncStatus(nextSyncStatus);
  }, []);

  const loadSystemConfig = useCallback(async () => {
    setConfigError(null);
    setConfigLoading(true);
    try {
      const payload = await fetchJson('/v1/system/config');
      applySystemConfigResponse(payload);
    } catch (error) {
      console.error('Failed to load system config', error);
      if (!isMountedRef.current) return;
      setConfigError(error instanceof Error ? error.message : 'Unable to load system config');
    } finally {
      if (isMountedRef.current) {
        setConfigLoading(false);
      }
    }
  }, [applySystemConfigResponse, fetchJson]);

  const loadSyncStatus = useCallback(
    async (opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setSyncError(null);
      }
      try {
        const payload = await fetchJson('/v1/system/config/sync');
        const nextStatus = normalizeConfigSyncStatus(payload);
        if (isMountedRef.current) {
          setSyncStatus(nextStatus);
          setSyncError(null);
        }
      } catch (error) {
        console.error('Failed to load sync status', error);
        if (!isMountedRef.current) return;
        setSyncError(error instanceof Error ? error.message : 'Unable to load sync status');
      }
    },
    [fetchJson]
  );

  const loadDispatcherStatus = useCallback(
    async (opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setDispatcherError(null);
        setDispatcherLoading(true);
      }
      try {
        const payload = await fetchJson('/v1/system/dispatcher');
        const normalized = normalizeDispatcherStatus(payload);
        if (isMountedRef.current) {
          setDispatcherStatus({ ...normalized, fetchedAt: Date.now() });
          setDispatcherError(null);
        }
      } catch (error) {
        console.error('Failed to load dispatcher status', error);
        if (!isMountedRef.current) return;
        setDispatcherError(error instanceof Error ? error.message : 'Unable to load dispatcher status');
      } finally {
        if (isMountedRef.current && !opts?.quiet) {
          setDispatcherLoading(false);
        }
      }
    },
    [fetchJson]
  );

  const loadTenants = useCallback(async () => {
    try {
      setTenantError(null);
      const payload = await fetchJson('/v1/tenants');
      if (Array.isArray(payload)) {
        const normalized = payload
          .map(item => (item && typeof item === 'object' ? (item as TenantRecord) : null))
          .filter(Boolean) as TenantRecord[];
        setTenants(normalized);
      }
    } catch (error) {
      setTenantError(error instanceof Error ? error.message : 'Unable to load tenants');
    }
  }, [fetchJson]);

  const loadUsers = useCallback(async () => {
    setUsersLoading(true);
    setUsersError(null);
    try {
      const payload = await fetchJson('/v1/admin/users');
      if (Array.isArray(payload)) {
        setUsers(payload as UserSummary[]);
      } else {
        setUsersError('Unexpected response');
      }
    } catch (error) {
      setUsersError(error instanceof Error ? error.message : 'Unable to load users');
    } finally {
      setUsersLoading(false);
    }
  }, [fetchJson]);

  const loadPolicies = useCallback(async () => {
    setPoliciesLoading(true);
    setPoliciesError(null);
    try {
      const payload = await fetchJson('/v1/admin/roles');
      if (!Array.isArray(payload)) {
        setPoliciesError('Unexpected response');
        setPoliciesLoading(false);
        return;
      }
      const records = payload as RolePermission[];
      const templates = records.filter(p => p.role === POLICY_TEMPLATE_ROLE);
      const rolePolicies = normalizeAdminPolicies(records.filter(p => p.role !== POLICY_TEMPLATE_ROLE));
      setPolicyTemplates(templates);
      setPolicies(rolePolicies);
    } catch (error) {
      setPoliciesError(error instanceof Error ? error.message : 'Unable to load policies');
    }
    setPoliciesLoading(false);
  }, [fetchJson]);

  const createUser = useCallback(
    async (e: FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      const tenantName = normalizeTenant(newUser.tenant);
      const roleAssignmentsInput = (newUser.roles || []).map(entry => ({ role: entry.role.trim(), tenant: tenantName }));
      const roleAssignments = roleAssignmentsInput.filter(entry => entry.role);
      const seenRoles = new Set<string>();
      const dedupedAssignments = roleAssignments.filter(entry => {
        const key = `${entry.role}::${entry.tenant}`;
        if (seenRoles.has(key)) return false;
        seenRoles.add(key);
        return true;
      });
      if (dedupedAssignments.length === 0) {
        addToast('Add at least one role before creating a user.', 'error');
        return;
      }
      try {
        const primaryAssignment = dedupedAssignments[0];
        const created = (await fetchJson('/v1/admin/users', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sub: newUser.sub.trim(),
            email: newUser.email.trim(),
            password: newUser.password,
            tenant_name: tenantName,
            role: primaryAssignment?.role,
          }),
        })) as Partial<UserSummary> & { user_id?: string; userId?: string };

        let userId: string | undefined =
          (created && (created.id || created.user_id || created.userId)) || undefined;

        if (!userId) {
          try {
            const list = await fetchJson('/v1/admin/users');
            if (Array.isArray(list)) {
              const match = (list as UserSummary[]).find(u => u.sub === newUser.sub || u.email === newUser.email);
              userId = match?.id;
            }
          } catch {
            // ignore lookup failure, will fail on assignment below if missing
          }
        }

        if (!userId) {
          addToast('User created but ID not found; roles not assigned.', 'error');
          await loadUsers();
          return;
        }

        for (const entry of dedupedAssignments) {
          try {
            await fetchJson('/v1/admin/user-roles', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                user_id: userId,
                tenant_name: entry.tenant,
                role: entry.role,
              }),
            });
          } catch (err) {
            console.error('Failed to assign role', entry, err);
          }
        }

        addToast(`User ${newUser.sub} saved with ${dedupedAssignments.length} role(s)`, 'success');
        setNewUser({ sub: '', email: '', password: '', tenant: tenantName, roles: [] });
        loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to create user', 'error');
      }
    },
    [addToast, fetchJson, loadUsers, newUser.email, newUser.password, newUser.roles, newUser.sub, newUser.tenant]
  );

  const assignRole = useCallback(
    async (e: FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      try {
        await fetchJson('/v1/admin/user-roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            user_id: newRole.userId.trim(),
            tenant_name: newRole.tenant.trim(),
            role: newRole.role.trim(),
          }),
        });
        addToast('Role added', 'success');
        setNewRole({ userId: '', tenant: '', role: '' });
        loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to add role', 'error');
      }
    },
    [addToast, fetchJson, loadUsers, newRole.role, newRole.tenant, newRole.userId]
  );

  const createPermission = useCallback(
    async (e: FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      const tenant = newPermission.tenant.trim();
      const obj = newPermission.obj.trim();
      const act = newPermission.act.trim();
      const name = newPermission.name.trim();
      if (!name || !obj || !act) {
        addToast('Policy name, object, and action are required.', 'error');
        return;
      }
      try {
        await fetchJson('/v1/admin/roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: POLICY_TEMPLATE_ROLE,
            tenant_name: tenant,
            name,
            obj,
            act,
          }),
        });
        addToast('Policy added', 'success');
        setNewPermission({ tenant: '', name: '', obj: '/v1/*', act: 'GET|POST|PUT|DELETE' });
        loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to add policy', 'error');
      }
    },
    [addToast, fetchJson, loadPolicies, newPermission.act, newPermission.name, newPermission.obj, newPermission.tenant]
  );

  const deleteUser = useCallback(
    async (userId: string) => {
      try {
        await fetchJson(`/v1/admin/users/${encodeURIComponent(userId)}`, { method: 'DELETE' });
        addToast('User deleted', 'success');
        loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to delete user', 'error');
      }
    },
    [addToast, fetchJson, loadUsers]
  );

  const deletePolicy = useCallback(
    async (policy: RolePermission) => {
      if (isDefaultAdmin(policy.role, policy.tenant_id)) {
        addToast('Default admin policy cannot be deleted.', 'error');
        return;
      }
      try {
        await fetchJson('/v1/admin/roles', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: policy.role,
            tenant_id: policy.tenant_id,
            obj: policy.obj,
            act: policy.act,
          }),
        });
        addToast('Policy removed', 'success');
        loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to remove policy', 'error');
      }
    },
    [addToast, fetchJson, loadPolicies, policyTemplates]
  );

  const deleteRoleDefinition = useCallback(
    async (role: RoleDefinition) => {
      if (isDefaultAdmin(role.role, role.tenant)) {
        addToast('Default admin role cannot be deleted.', 'error');
        return;
      }
      const inUse = users.some(user =>
        (user.roles || []).some(
          r => r.role === role.role && normalizeTenant(r.tenant_id ?? (r as { tenant?: string }).tenant) === normalizeTenant(role.tenant)
        )
      );
      if (inUse) {
        addToast('Cannot delete a role that is still assigned to users. Remove assignments first.', 'error');
        return;
      }
      try {
        for (const entry of role.policies) {
          await fetchJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.role,
              tenant_id: entry.tenant_id || role.tenant,
              tenant_name: role.tenant,
              obj: entry.obj,
              act: entry.act,
            }),
          });
        }
        addToast('Role removed', 'success');
        loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to remove role', 'error');
      }
    },
    [addToast, fetchJson, loadPolicies, users]
  );

  const saveRoleDefinition = useCallback(
    async ({ role, tenant, policies: drafts, original }: { role: string; tenant: string; policies: RolePolicyDraft[]; original?: RolePermission[] }) => {
      const roleName = role.trim();
      const tenantName = tenant.trim() || 'default';
      const tenantKey = normalizeTenant(tenantName);
      const templateByName = new Map<string, RolePermission>();
      policyTemplates
        .filter(template => normalizeTenant(template.tenant_id) === tenantKey)
        .forEach(template => {
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
      const nextPolicies = resolved.map(item => ({ role: roleName, tenant: tenantName, name: item.name, obj: item.obj, act: item.act }));
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
          await fetchJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.role,
              tenant_id: entry.tenant_id,
              tenant_name: tenantName || entry.tenant_id,
              obj: entry.obj,
              act: entry.act,
            }),
          });
        }
        for (const entry of renamed) {
          await fetchJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.previous.role,
              tenant_id: entry.previous.tenant_id,
              tenant_name: tenantName || entry.previous.tenant_id,
              obj: entry.previous.obj,
              act: entry.previous.act,
            }),
          });
        }
        for (const entry of toAdd) {
          await fetchJson('/v1/admin/roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.role,
              tenant_name: tenantName,
              name: entry.name,
              obj: entry.obj,
              act: entry.act,
            }),
          });
        }
        for (const entry of renamed) {
          await fetchJson('/v1/admin/roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: entry.next.role,
              tenant_name: tenantName,
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
    [addToast, fetchJson, loadPolicies]
  );

  const editPolicy = useCallback(
    async (current: RolePermission, next: { role: string; tenant: string; name: string; obj: string; act: string }) => {
      const nextRole = next.role.trim();
      const nextTenant = next.tenant.trim();
      const nextObj = next.obj.trim();
      const nextAct = next.act.trim();
      const nextName = next.name.trim() || policyName(nextObj, nextAct);
      if (!nextRole || !nextObj || !nextAct) {
        addToast('Role, object, and action are required.', 'error');
        throw new Error('Policy validation failed');
      }
      const sameKey = policyKey({
        role: current.role,
        tenant_id: current.tenant_id,
        obj: current.obj,
        act: current.act,
      }) ===
        policyKey({
          role: nextRole,
          tenant: nextTenant,
          obj: nextObj,
          act: nextAct,
        });
      const currentName = (current.name || '').trim() || policyName(current.obj, current.act);
      if (sameKey && currentName === nextName) {
        addToast('No changes to save for this policy.', 'info');
        return;
      }
      try {
        await fetchJson('/v1/admin/roles', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: current.role,
            tenant_id: current.tenant_id,
            obj: current.obj,
            act: current.act,
          }),
        });
        await fetchJson('/v1/admin/roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: nextRole,
            tenant_name: nextTenant,
            name: nextName,
            obj: nextObj,
            act: nextAct,
          }),
        });
        addToast('Policy updated', 'success');
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to update policy', 'error');
        throw error;
      }
    },
    [addToast, fetchJson, loadPolicies]
  );

  const updateUserRoles = useCallback(
    async (
      userId: string,
      nextRoles: { role: string; tenant: string }[],
      previousRoles: { role: string; tenant: string }[]
    ) => {
      const normalize = (entry: { role: string; tenant: string }) => ({
        role: entry.role.trim(),
        tenant: entry.tenant.trim(),
      });
      const cleanedPrev = previousRoles.map(normalize).filter(item => item.role);
      const cleanedNext = nextRoles
        .map(normalize)
        .filter(item => item.role)
        .filter((item, index, arr) => index === arr.findIndex(other => assignmentKey(other) === assignmentKey(item)));
      const prevKeys = new Set(cleanedPrev.map(assignmentKey));
      const nextKeys = new Set(cleanedNext.map(assignmentKey));
      const toRemove = cleanedPrev.filter(item => !nextKeys.has(assignmentKey(item)));
      const toAdd = cleanedNext.filter(item => !prevKeys.has(assignmentKey(item)));
      try {
        for (const entry of toRemove) {
          await fetchJson('/v1/admin/user-roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              user_id: userId,
              tenant_name: entry.tenant,
              role: entry.role,
            }),
          });
        }
        for (const entry of toAdd) {
          await fetchJson('/v1/admin/user-roles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              user_id: userId,
              tenant_name: entry.tenant,
              role: entry.role,
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
    [addToast, fetchJson, loadUsers]
  );

  const updateUser = useCallback(
    async (userId: string, input: { email?: string; status?: string; password?: string }) => {
      const payload: Record<string, string> = {};
      if (input.email) payload.email = input.email.trim();
      if (input.status) payload.status = input.status.trim();
      if (input.password) payload.password = input.password;
      if (Object.keys(payload).length === 0) return;
      try {
        await fetchJson(`/v1/admin/users/${encodeURIComponent(userId)}`, {
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
    [addToast, fetchJson, loadUsers]
  );

  const saveConfig = useCallback(async () => {
    if (saving) return;
    setSaving(true);
    setConfigError(null);
    try {
      const payload = await fetchJson('/v1/system/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          config_repo_url: config.config_repo_url.trim(),
          agent_image: config.agent_image.trim(),
          docker_network_name: config.docker_network_name.trim(),
          default_pipeline_timeout: config.default_pipeline_timeout.trim(),
          llm_agent_timeout: config.llm_agent_timeout.trim(),
          auto_removal_agent_container: Boolean(config.auto_removal_agent_container),
          agent_nopsai_api_url: config.agent_nopsai_api_url.trim(),
          git_bot_nopsai_api_url: config.git_bot_nopsai_api_url.trim(),
          nopsai_git_bot_api_url: config.nopsai_git_bot_api_url.trim(),
        }),
      });
      applySystemConfigResponse(payload);
      addToast('System settings saved.', 'success');
    } catch (error) {
      console.error('Failed to save system config', error);
      addToast('Failed to save settings.', 'error');
      if (isMountedRef.current) {
        setConfigError(error instanceof Error ? error.message : 'Unable to save system config');
      }
    } finally {
      if (isMountedRef.current) {
        setSaving(false);
      }
    }
  }, [addToast, applySystemConfigResponse, config, fetchJson, saving]);

  const triggerSync = useCallback(async () => {
    if (syncBusy) return;
    setSyncBusy(true);
    setSyncError(null);
    try {
      const payload = await fetchJson('/v1/system/config/sync', { method: 'POST' });
      const status = normalizeConfigSyncStatus(payload);
      if (isMountedRef.current) {
        setSyncStatus(status);
      }
      addToast('Config sync requested.', 'info');
    } catch (error) {
      console.error('Failed to trigger config sync', error);
      addToast('Unable to start config sync.', 'error');
      if (isMountedRef.current) {
        setSyncError(error instanceof Error ? error.message : 'Unable to start config sync');
      }
    } finally {
      if (isMountedRef.current) {
        setSyncBusy(false);
      }
    }
  }, [addToast, fetchJson, syncBusy]);

  const setRunnerPending = useCallback((runnerId: string, connectionId: string, pending: boolean) => {
    const key = runnerActionKey(runnerId, connectionId);
    if (!key) return;
    setRunnerActionPending(prev => {
      const next = new Set(prev);
      if (pending) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);

  const toggleRunnerDispatch = useCallback(
    async (runner: Runner) => {
      const meta = getRunnerMeta(runner);
      const key = runnerActionKey(runner.runnerId, meta.connectionId);
      if (!key || runnerActionPending.has(key)) return;

      const nextAllow = !runner.allowDispatch;
      setRunnerPending(runner.runnerId, meta.connectionId, true);
      try {
        await fetchJson(`/v1/system/dispatcher/runners/${encodeURIComponent(runner.runnerId)}/dispatch`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            allow_dispatch: nextAllow,
            ...(meta.connectionId ? { connection_id: meta.connectionId } : {}),
          }),
        });
        await loadDispatcherStatus({ quiet: true });
      } catch (error) {
        console.error('Failed to toggle runner dispatch', error);
        addToast('Failed to update runner dispatch.', 'error');
      } finally {
        setRunnerPending(runner.runnerId, meta.connectionId, false);
      }
    },
    [addToast, fetchJson, loadDispatcherStatus, runnerActionPending, setRunnerPending]
  );

  useEffect(() => {
    void loadSystemConfig();
  }, [loadSystemConfig]);

  useEffect(() => {
    void loadTenants();
  }, [loadTenants]);

  useEffect(() => {
    if (activeTab === 'dispatcher') {
      void loadDispatcherStatus();
    }
  }, [activeTab, loadDispatcherStatus]);

  useEffect(() => {
    if (activeTab === 'access') {
      void loadUsers();
      void loadPolicies();
    }
  }, [activeTab, loadPolicies, loadUsers]);

  const defaultAdminPolicyExists = useMemo(
    () =>
      policies.some(
        p =>
          p.role === DEFAULT_ADMIN_ROLE &&
          isDefaultAdmin(p.role, p.tenant_id ?? (p as { tenant?: string }).tenant) &&
          p.obj === DEFAULT_ADMIN_POLICY_OBJ &&
          p.act === DEFAULT_ADMIN_POLICY_ACT
      ),
    [policies]
  );

  useEffect(() => {
    if (policiesLoading || ensuringDefaultAdmin) return;
    if (defaultAdminPolicyExists) return;
    setEnsuringDefaultAdmin(true);
    void (async () => {
      try {
        await fetchJson('/v1/admin/roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: DEFAULT_ADMIN_ROLE,
            tenant_name: 'default',
            name: 'Admin all access',
            obj: DEFAULT_ADMIN_POLICY_OBJ,
            act: DEFAULT_ADMIN_POLICY_ACT,
          }),
        });
        await loadPolicies();
      } catch (error) {
        console.error('Failed to ensure default admin policy', error);
      } finally {
        setEnsuringDefaultAdmin(false);
      }
    })();
  }, [defaultAdminPolicyExists, ensuringDefaultAdmin, fetchJson, loadPolicies, policiesLoading]);

  useEffect(() => {
    const handle = window.setInterval(() => {
      void loadSyncStatus({ quiet: true });
      if (activeTab === 'dispatcher') {
        void loadDispatcherStatus({ quiet: true });
      }
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [activeTab, loadDispatcherStatus, loadSyncStatus]);

  return (
    <div data-page="system" className="active p-6 space-y-6">
      {activeTab === 'config' && (
        <SystemConfig
          config={config}
          envFilePath={envFilePath}
          syncStatus={syncStatus}
          syncError={syncError}
          configError={configError}
          configLoading={configLoading}
          saving={saving}
          onChange={setConfig}
          onReload={loadSystemConfig}
          onRefreshSyncStatus={loadSyncStatus}
          onSave={saveConfig}
          onTriggerSync={triggerSync}
        />
      )}
      {activeTab === 'dispatcher' && (
        <DispatcherPanel
          loading={dispatcherLoading}
          error={dispatcherError}
          status={dispatcherStatus}
          pendingActions={runnerActionPending}
          onRefresh={() => loadDispatcherStatus()}
          onToggleRunnerDispatch={toggleRunnerDispatch}
        />
      )}
      {activeTab === 'access' && (
        <AccessPanel
          users={users}
          tenants={tenants}
          loading={usersLoading}
          error={usersError}
          policies={policies}
          policiesLoading={policiesLoading}
          policiesError={policiesError}
          tenantError={tenantError}
          newUser={newUser}
          newRole={newRole}
          policyTemplates={policyTemplates}
          onChangeUser={setNewUser}
          onChangeRole={setNewRole}
          onCreateUser={createUser}
          onAssignRole={assignRole}
          onReloadUsers={loadUsers}
          onReloadTenants={loadTenants}
          onReloadPolicies={loadPolicies}
          onCreatePermission={createPermission}
          newPermission={newPermission}
          onChangePermission={setNewPermission}
          onDeleteUser={deleteUser}
          onDeletePolicy={deletePolicy}
          onDeleteRoleDefinition={deleteRoleDefinition}
          onSaveRoleDefinition={saveRoleDefinition}
          onEditPolicy={editPolicy}
          onUpdateUserRoles={updateUserRoles}
          onUpdateUser={updateUser}
        />
      )}

      {toasts.length > 0 && (
        <div className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3">
          {toasts.map(toast => (
            <div key={toast.id} className={`pipelines-toast pipelines-toast--${toast.tone} show`}>
              <div className="pipelines-toast__content">{toast.message}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

const policyKey = (input: { role: string; tenant?: string; tenant_id?: string; obj: string; act: string }) =>
  `${(input.role || '').trim()}::${(input.tenant_id || input.tenant || '').trim()}::${(input.obj || '').trim()}::${(input.act || '').trim()}`;

const assignmentKey = (input: { role: string; tenant?: string }) =>
  `${(input.role || '').trim()}::${(input.tenant || '').trim()}`;

const normalizeTenant = (value?: string) => {
  const trimmed = (value || '').trim();
  if (!trimmed || trimmed === DEFAULT_TENANT_ID) return 'default';
  return trimmed;
};

const policyName = (obj: string, act: string) => {
  const trimmed = (obj || '').replace(/^\/+|\/+$/g, '').trim();
  const leaf = trimmed.split('/').filter(Boolean).pop();
  const base = leaf || trimmed || obj || 'policy';
  const action = (act || '').trim() || 'ANY';
  return `${base} • ${action}`;
};

const policyLabel = (input: { name?: string; obj: string; act: string }) =>
  (input.name && input.name.trim()) || policyName(input.obj, input.act);
const isDefaultAdmin = (roleName: string, tenant?: string) =>
  roleName === DEFAULT_ADMIN_ROLE && normalizeTenant(tenant) === 'default';

const normalizeAdminPolicies = (records: RolePermission[]): RolePermission[] => {
  const deduped = records.filter((entry, idx, arr) => idx === arr.findIndex(other => policyKey(other) === policyKey(entry)));
  const filtered = deduped.filter(
    entry =>
      !isDefaultAdmin(entry.role, entry.tenant_id ?? (entry as { tenant?: string }).tenant) ||
      (entry.obj === DEFAULT_ADMIN_POLICY_OBJ && entry.act === DEFAULT_ADMIN_POLICY_ACT)
  );
  const hasCanonicalAdmin = filtered.some(
    entry =>
      isDefaultAdmin(entry.role, entry.tenant_id ?? (entry as { tenant?: string }).tenant) &&
      entry.obj === DEFAULT_ADMIN_POLICY_OBJ &&
      entry.act === DEFAULT_ADMIN_POLICY_ACT
  );
  if (!hasCanonicalAdmin) {
    filtered.push({
      role: DEFAULT_ADMIN_ROLE,
      tenant_id: 'default',
      name: 'Admin all access',
      obj: DEFAULT_ADMIN_POLICY_OBJ,
      act: DEFAULT_ADMIN_POLICY_ACT,
    });
  }
  return filtered;
};

function AccessPanel({
  users,
  tenants,
  loading,
  error,
  policies,
  policiesLoading,
  policiesError,
  tenantError,
  newUser,
  newRole,
  policyTemplates,
  onChangeUser,
  onChangeRole,
  onCreateUser,
  onAssignRole,
  onReloadUsers,
  onCreatePermission,
  newPermission,
  onChangePermission,
  onDeleteUser,
  onDeletePolicy,
  onDeleteRoleDefinition,
  onSaveRoleDefinition,
  onEditPolicy,
  onUpdateUserRoles,
  onReloadPolicies,
  onUpdateUser,
}: {
  users: UserSummary[];
  tenants: TenantRecord[];
  loading: boolean;
  error: string | null;
  policies: RolePermission[];
  policiesLoading: boolean;
  policiesError: string | null;
  tenantError: string | null;
  newUser: { sub: string; email: string; password: string; tenant: string; roles: { role: string; tenant: string }[] };
  newRole: { userId: string; tenant: string; role: string };
  policyTemplates: RolePermission[];
  onChangeUser: (next: { sub: string; email: string; password: string; tenant: string; roles: { role: string; tenant: string }[] }) => void;
  onChangeRole: (next: { userId: string; tenant: string; role: string }) => void;
  onCreateUser: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  onAssignRole: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  onReloadUsers: () => void;
  onReloadTenants: () => void;
  onCreatePermission: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  newPermission: { tenant: string; name: string; obj: string; act: string };
  onChangePermission: (next: { tenant: string; name: string; obj: string; act: string }) => void;
  onDeleteUser: (userId: string) => Promise<void>;
  onDeletePolicy: (policy: RolePermission) => Promise<void>;
  onDeleteRoleDefinition: (role: RoleDefinition) => Promise<void>;
  onSaveRoleDefinition: (input: { role: string; tenant: string; policies: RolePolicyDraft[]; original?: RolePermission[] }) => Promise<void>;
  onEditPolicy: (current: RolePermission, next: { role: string; tenant: string; name: string; obj: string; act: string }) => Promise<void>;
  onUpdateUserRoles: (
    userId: string,
    nextRoles: { role: string; tenant: string }[],
    previousRoles: { role: string; tenant: string }[]
  ) => Promise<void>;
  onReloadPolicies: () => void;
  onUpdateUser: (userId: string, input: { email?: string; status?: string; password?: string }) => Promise<void>;
}) {
  const [activeSection, setActiveSection] = useState<'users' | 'roles' | 'policies'>('users');
  const [showUserModal, setShowUserModal] = useState(false);
  const [showRoleModal, setShowRoleModal] = useState(false);
  const [showPolicyModal, setShowPolicyModal] = useState(false);
  const [roleEditor, setRoleEditor] = useState<{
    mode: 'create' | 'edit';
    role: string;
    tenant: string;
    policies: RolePolicyDraft[];
    original?: RolePermission[];
  } | null>(null);
  const [policyEditor, setPolicyEditor] = useState<{
    original: RolePermission;
    role: string;
    tenant: string;
    name: string;
    obj: string;
    act: string;
  } | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{ message: string; onConfirm: () => Promise<void> | void } | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [userAccessEditor, setUserAccessEditor] = useState<{
    user: UserSummary;
    entries: { role: string; tenant: string }[];
    original: { role: string; tenant: string }[];
    email: string;
    status: string;
    password: string;
  } | null>(null);
  const [savingRoleEditor, setSavingRoleEditor] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [savingUserAccess, setSavingUserAccess] = useState(false);
  const tenantLabel = useMemo(() => {
    const map = new Map<string, string>();
    tenants.forEach(t => {
      const key = (t.id || '').trim();
      const value = (t.name || t.id || '').trim();
      if (key) map.set(key, value || key);
    });
    map.set('default', 'default');
    map.set(DEFAULT_TENANT_ID, 'default');
    return (tenantId?: string) => {
      const key = (tenantId || '').trim();
      if (!key || key === DEFAULT_TENANT_ID) return 'default';
      return map.get(key) || key;
    };
  }, [tenants]);
  const tenantKeyFromInput = useCallback(
    (value?: string) => {
      const raw = (value || '').trim();
      if (!raw || raw === DEFAULT_TENANT_ID) return 'default';
      const match = tenants.find(t => t.id === raw || t.name === raw);
      return normalizeTenant(match?.id || raw);
    },
    [tenants]
  );
  const policyOptionsByTenant = useMemo(() => {
    const map = new Map<string, { key: string; obj: string; act: string; name?: string; label: string }[]>();
    policyTemplates.forEach(p => {
      const tenantKey = normalizeTenant(p.tenant_id || (p as { tenant?: string }).tenant);
      const entryKey = `${p.obj}::${p.act}`;
      const existing = map.get(tenantKey) || [];
      const alreadyIncluded = existing.some(item => item.key === entryKey);
      if (!alreadyIncluded) {
        existing.push({ key: entryKey, obj: p.obj, act: p.act, name: p.name, label: policyLabel(p) });
      }
      map.set(tenantKey, existing);
    });
    map.forEach(list => list.sort((a, b) => a.label.localeCompare(b.label)));
    return map;
  }, [policyTemplates]);

  const roleAssignments = useMemo(
    () =>
      users
        .flatMap(user =>
          (user.roles || []).map(role => {
            const tenantValue = normalizeTenant(role.tenant_id ?? (role as { tenant?: string }).tenant);
            return {
              id: `${user.id}-${role.role}-${tenantValue || 'default'}`,
              role: role.role,
              tenant: tenantValue,
              user: user.sub,
              userId: user.id,
              email: user.email,
              status: user.status,
            };
          })
        )
        .sort((a, b) => a.role.localeCompare(b.role) || a.user.localeCompare(b.user)),
    [users]
  );

  const roleDefinitions = useMemo(() => {
    const map = new Map<string, RoleDefinition>();
    policies.forEach(policy => {
      const tenant = normalizeTenant(policy.tenant_id ?? (policy as { tenant?: string }).tenant);
      const key = `${policy.role}::${tenant}`;
      if (!map.has(key)) {
        map.set(key, { id: key, role: policy.role, tenant, policies: [] });
      }
      map.get(key)?.policies.push(policy);
    });
    return Array.from(map.values()).sort((a, b) => a.role.localeCompare(b.role) || a.tenant.localeCompare(b.tenant));
  }, [policies]);

  const rolesByTenant = useMemo(() => {
    const map = new Map<string, Set<string>>();
    const addRole = (tenant: string, role: string) => {
      const trimmedRole = (role || '').trim();
      if (!trimmedRole) return;
      const key = normalizeTenant(tenant);
      if (!map.has(key)) {
        map.set(key, new Set());
      }
      map.get(key)?.add(trimmedRole);
    };
    roleDefinitions.forEach(role => addRole(role.tenant, role.role));
    addRole('default', DEFAULT_ADMIN_ROLE);
    return new Map(
      Array.from(map.entries()).map(([tenant, set]) => [tenant, Array.from(set).sort((a, b) => a.localeCompare(b))])
    );
  }, [roleDefinitions]);

  const safeHtmlId = (value: string) => value.replace(/[^a-zA-Z0-9_-]/g, '-');
  const roleDatalistBaseId = 'access-role-names';
  const roleListIdForTenant = (tenant?: string) => `${roleDatalistBaseId}-${safeHtmlId(normalizeTenant(tenant) || 'default')}`;
  const roleOptionsForTenant = (tenant?: string) => {
    const key = tenantKeyFromInput(tenant);
    return rolesByTenant.get(key) || rolesByTenant.get('default') || [];
  };
  const roleDatalistId = roleDatalistBaseId;
  const roleListFor = (tenant?: string) => {
    const key = tenantKeyFromInput(tenant);
    const options = roleOptionsForTenant(key);
    return options.length > 0 ? roleListIdForTenant(key) : roleDatalistId;
  };

  const allRoleOptions = useMemo(() => {
    const set = new Set<string>();
    rolesByTenant.forEach(list => list.forEach(role => set.add(role)));
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [rolesByTenant]);

  const roleUserMap = useMemo(() => {
    const map = new Map<string, { user: string; userId: string; email: string }[]>();
    roleAssignments.forEach(item => {
      const key = `${item.role}::${item.tenant}`;
      const existing = map.get(key) || [];
      existing.push({ user: item.user, userId: item.userId, email: item.email });
      map.set(key, existing);
    });
    return map;
  }, [roleAssignments]);

  const tabItems = [
    { id: 'users', label: 'Users', count: users.length },
    { id: 'roles', label: 'Roles', count: roleDefinitions.length },
    { id: 'policies', label: 'Policies', count: policyTemplates.length },
  ] as const;

  const actionLabel = activeSection === 'users' ? 'New user' : activeSection === 'roles' ? 'New role' : 'New policy';

  const openCreateRoleEditor = useCallback(
    () =>
      setRoleEditor({
        mode: 'create',
        role: '',
        tenant: '',
        policies: [],
      }),
    []
  );

  const openActiveModal = () => {
    if (activeSection === 'users') setShowUserModal(true);
    else if (activeSection === 'roles') openCreateRoleEditor();
    else setShowPolicyModal(true);
  };

  const openEditRoleEditor = (role: RoleDefinition) => {
    if (isDefaultAdmin(role.role, role.tenant)) return;
    setRoleEditor({
      mode: 'edit',
      role: role.role,
      tenant: role.tenant,
      policies: role.policies.map(p => ({ name: p.name || policyLabel(p), obj: p.obj, act: p.act })),
      original: role.policies,
    });
  };

  const openPolicyEditModal = (policy: RolePermission) => {
    if (isDefaultAdmin(policy.role, policy.tenant_id ?? (policy as { tenant?: string }).tenant)) return;
    setPolicyEditor({
      original: policy,
      role: policy.role,
      tenant: normalizeTenant(policy.tenant_id),
      name: policy.name || policyLabel(policy),
      obj: policy.obj,
      act: policy.act,
    });
  };

  const openUserAccessModal = (user: UserSummary) => {
    const entries = (user.roles || []).map(role => ({
      role: role.role,
      tenant: normalizeTenant(role.tenant_id ?? (role as { tenant?: string }).tenant),
    }));
    const nextEntries = entries.length > 0 ? entries : [];
    setUserAccessEditor({
      user,
      entries: nextEntries,
      original: entries,
      email: user.email || '',
      status: user.status || 'active',
      password: '',
    });
  };

  const removeRolePolicyDraft = (index: number) => {
    setRoleEditor(prev => {
      if (!prev) return prev;
      const nextPolicies = prev.policies.filter((_, i) => i !== index);
      return {
        ...prev,
        policies: nextPolicies,
      };
    });
  };

  const addExistingPolicyDraft = (key: string) => {
    setRoleEditor(prev => {
      if (!prev) return prev;
      const tenantKey = normalizeTenant(prev.tenant);
      const available = policyOptionsByTenant.get(tenantKey) || policyOptionsByTenant.get('default') || [];
      const match = available.find(p => p.key === key);
      if (!match) return prev;
      const already = prev.policies.some(p => p.obj === match.obj && p.act === match.act);
      if (already) return prev;
      return { ...prev, policies: [...prev.policies, { name: match.name || match.label, obj: match.obj, act: match.act }] };
    });
  };

  const handleSaveRoleEditor = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!roleEditor) return;
    if (isDefaultAdmin(roleEditor.role, roleEditor.tenant)) return;
    setSavingRoleEditor(true);
    try {
      await onSaveRoleDefinition({
        role: roleEditor.role,
        tenant: roleEditor.tenant,
        policies: roleEditor.policies,
        original: roleEditor.original,
      });
      setRoleEditor(null);
    } catch (error) {
      console.error('Failed to save role', error);
    } finally {
      setSavingRoleEditor(false);
    }
  };

  const handleSavePolicyEdit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!policyEditor) return;
    setSavingPolicy(true);
    try {
      await onEditPolicy(policyEditor.original, {
        role: policyEditor.role,
        tenant: policyEditor.tenant,
        name: policyEditor.name,
        obj: policyEditor.obj,
        act: policyEditor.act,
      });
      setPolicyEditor(null);
    } catch (error) {
      console.error('Failed to update policy', error);
    } finally {
      setSavingPolicy(false);
    }
  };

  const handleSaveUserAccess = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!userAccessEditor) return;
    const normalized = userAccessEditor.entries
      .map(entry => ({ role: entry.role.trim(), tenant: entry.tenant.trim() }))
      .filter(entry => entry.role);
    const deduped = normalized.filter(
      (entry, idx) => idx === normalized.findIndex(other => assignmentKey(other) === assignmentKey(entry))
    );
    setSavingUserAccess(true);
    try {
      const updatePayload: { email?: string; status?: string; password?: string } = {};
      const emailTrimmed = userAccessEditor.email.trim();
      if (emailTrimmed && emailTrimmed !== userAccessEditor.user.email) {
        updatePayload.email = emailTrimmed;
      }
      const statusTrimmed = userAccessEditor.status.trim();
      if (statusTrimmed && statusTrimmed !== userAccessEditor.user.status) {
        updatePayload.status = statusTrimmed;
      }
      const passwordTrimmed = userAccessEditor.password.trim();
      if (passwordTrimmed) {
        updatePayload.password = passwordTrimmed;
      }
      if (Object.keys(updatePayload).length > 0) {
        await onUpdateUser(userAccessEditor.user.id, updatePayload);
      }
      await onUpdateUserRoles(userAccessEditor.user.id, deduped, userAccessEditor.original);
      setUserAccessEditor(null);
    } catch (error) {
      console.error('Failed to update user access', error);
    } finally {
      setSavingUserAccess(false);
    }
  };

  const addUserAccessEntry = () => {
    setUserAccessEditor(prev => {
      if (!prev) return prev;
      const roleName = nextAccessRole.trim();
      if (!roleName) return prev;
      const tenant = normalizeTenant(prev.entries[0]?.tenant || 'default');
      const alreadyExists = prev.entries.some(entry => assignmentKey({ role: entry.role, tenant: normalizeTenant(entry.tenant || tenant) }) === assignmentKey({ role: roleName, tenant }));
      if (alreadyExists) return prev;
      return { ...prev, entries: [...prev.entries, { role: roleName, tenant }] };
    });
    setNextAccessRole('');
  };

  const removeUserAccessEntry = (index: number) => {
    setUserAccessEditor(prev => {
      if (!prev) return prev;
       const entry = prev.entries[index];
       const protectedRole =
         entry && prev.user && isDefaultAdmin(entry.role, entry.tenant) && prev.user.sub === 'admin';
       if (protectedRole) return prev;
      const next = prev.entries.filter((_, i) => i !== index);
      return { ...prev, entries: next };
    });
  };

  const updateNewUserRoleEntry = (index: number, value: string) => {
    const next = [...(newUser.roles || [])];
    next[index] = { ...next[index], role: value, tenant: newUser.tenant };
    onChangeUser({ ...newUser, roles: next });
  };

  const removeNewUserRoleEntry = (index: number) => {
    const next = (newUser.roles || []).filter((_, i) => i !== index);
    onChangeUser({ ...newUser, roles: next });
  };

  const appendUserRoleFromPicker = () => {
    const roleName = nextUserRole.trim();
    if (!roleName) return;
    const existing = (newUser.roles || []).some(entry => entry.role.trim().toLowerCase() === roleName.toLowerCase());
    if (existing) {
      setNextUserRole('');
      return;
    }
    onChangeUser({ ...newUser, roles: [...(newUser.roles || []), { role: roleName, tenant: newUser.tenant }] });
    setNextUserRole('');
  };

  const statusKey = (value: string) => {
    const key = (value || '').toLowerCase();
    if (key.includes('active')) return 'ok';
    if (key.includes('pending')) return 'warn';
    if (key.includes('blocked') || key.includes('disabled')) return 'danger';
    return 'muted';
  };

  const tenantDatalistId = 'tenant-names';
  const userDatalistId = 'access-user-ids';
  const policyCount = policyTemplates.length;
  const [nextPolicyKey, setNextPolicyKey] = useState('');
  const [nextUserRole, setNextUserRole] = useState('');
  const [nextAccessRole, setNextAccessRole] = useState('');

  useEffect(() => {
    setNextPolicyKey('');
  }, [roleEditor?.tenant]);

  useEffect(() => {
    setNextUserRole('');
  }, [newUser.tenant]);

  useEffect(() => {
    setNextAccessRole('');
  }, [userAccessEditor]);

  const openConfirmDialog = (message: string, onConfirm: () => Promise<void> | void) => {
    setConfirmDialog({ message, onConfirm });
  };

  const confirmDeleteUser = (userId: string) => {
    openConfirmDialog('Delete this user? This cannot be undone.', () => onDeleteUser(userId));
  };

  const confirmDeleteRoleDefinition = (role: RoleDefinition) => {
    openConfirmDialog('Delete this role and its policies? This cannot be undone.', () => onDeleteRoleDefinition(role));
  };

  const confirmDeletePolicy = (policy: RolePermission) => {
    openConfirmDialog('Delete this policy? This cannot be undone.', () => onDeletePolicy(policy));
  };

  const handleConfirmDialog = async () => {
    if (!confirmDialog) return;
    setConfirming(true);
    try {
      await confirmDialog.onConfirm();
    } finally {
      setConfirming(false);
      setConfirmDialog(null);
    }
  };

  const handleRefresh = useCallback(() => {
    if (activeSection === 'policies') {
      void onReloadPolicies();
      return;
    }
    void onReloadUsers();
    if (activeSection === 'roles') {
      void onReloadPolicies();
    }
  }, [activeSection, onReloadPolicies, onReloadUsers]);

  const availablePoliciesForRoleEditor =
    roleEditor && policyOptionsByTenant.size > 0
      ? policyOptionsByTenant.get(normalizeTenant(roleEditor.tenant)) || policyOptionsByTenant.get('default') || []
      : [];

  const tenantRoleOptions = useMemo(() => roleOptionsForTenant(newUser.tenant), [newUser.tenant, rolesByTenant]);
  const accessRoleOptionsForTenant = userAccessEditor ? roleOptionsForTenant(userAccessEditor.entries[0]?.tenant || 'default') : [];

  return (
    <div className="access-layout space-y-5 pb-24">
      <div className="glass-card p-5 border border-[var(--border-primary)] rounded-2xl space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <p className="text-xs uppercase tracking-[0.12em] text-[var(--text-secondary)]">Access control</p>
            <h3 className="text-xl font-semibold text-[var(--text-primary)]">Users, roles, policies</h3>
            <p className="text-sm text-[var(--text-secondary)]">Define policies, bundle them into roles, and grant users access.</p>
          </div>
        </div>
        <div className="access-nav">
          <div className="access-tabs">
            {tabItems.map(tab => (
              <button
                key={tab.id}
                type="button"
                className={`access-tab ${activeSection === tab.id ? 'access-tab--active' : ''}`}
                onClick={() => setActiveSection(tab.id)}
              >
                <span className="access-tab__label">{tab.label}</span>
                <span className="access-tab__badge">
                  {tab.id === 'policies' ? policyCount : tab.count}
                </span>
              </button>
            ))}
          </div>
          <div className="access-tab-actions">
            <button
              className="access-icon-btn access-icon-btn--plain"
              type="button"
              onClick={openActiveModal}
              title={actionLabel}
              aria-label={actionLabel}
            >
              <PlusIcon />
            </button>
            <button
              className="access-icon-btn access-icon-btn--plain"
              type="button"
              onClick={handleRefresh}
              disabled={loading}
              title="Refresh"
              aria-label="Refresh"
            >
              <RefreshIcon />
            </button>
          </div>
        </div>

        {activeSection === 'users' && (
          <div className="space-y-4">
            {error && <div className="text-sm text-red-500">Failed to load users: {error}</div>}
            {tenantError && <div className="text-sm text-yellow-500">Tenant lookup: {tenantError}</div>}
            {loading ? (
              <div className="access-empty-card">
                <p className="font-medium text-[var(--text-primary)]">Loading users…</p>
                <p className="text-sm text-[var(--text-secondary)]">Fetching directory entries.</p>
              </div>
            ) : users.length === 0 ? (
              <div className="access-empty-card">
                <p className="font-medium text-[var(--text-primary)]">No users yet</p>
                <p className="text-sm text-[var(--text-secondary)]">Start by creating a local account.</p>
              </div>
            ) : (
              <div className="glass-card p-0 border border-[var(--border-primary)] rounded-xl overflow-hidden">
                <div className="grid grid-cols-[1.4fr,1.2fr,1.1fr,0.8fr,auto] text-[11px] uppercase tracking-[0.08em] text-[var(--text-tertiary)] px-4 py-3 bg-[var(--bg-tertiary)] border-b border-[var(--border-primary)]">
                  <span>User</span>
                  <span>Email</span>
                  <span>Roles</span>
                  <span className="text-center">Status</span>
                  <span className="text-right">Manage</span>
                </div>
                <div className="divide-y divide-[var(--border-primary)]">
                  {users.map(user => {
                    const userRoles = user.roles || [];
                    return (
                      <div
                        key={user.id}
                        className="grid grid-cols-[1.4fr,1.2fr,1.1fr,0.8fr,auto] items-center px-4 py-3 gap-3 text-sm text-[var(--text-primary)] access-row"
                      >
                        <div className="min-w-0 flex items-center gap-3">
                          <div className="access-avatar access-avatar--sm">{(user.sub || user.email || 'U').charAt(0).toUpperCase()}</div>
                          <div className="min-w-0">
                            <p className="font-semibold truncate">{user.sub}</p>
                            <p className="text-[11px] text-[var(--text-secondary)] truncate">
                              {user.last_login ? new Date(user.last_login).toLocaleString() : 'Never signed in'}
                            </p>
                          </div>
                        </div>
                        <span className="text-[var(--text-secondary)] truncate">{user.email || 'No email'}</span>
                        <div className="flex flex-wrap gap-1.5">
                          {userRoles.length ? (
                            userRoles.slice(0, 3).map(role => {
                              const tenant = tenantLabel(role.tenant_id);
                              return (
                                <span key={`${user.id}-${role.role}-${role.tenant_id || 'default'}`} className="access-chip access-chip--muted">
                                  {role.role}
                                  {tenant ? ` @ ${tenant}` : ''}
                                </span>
                              );
                            })
                          ) : (
                            <span className="text-xs text-[var(--text-secondary)]">No roles</span>
                          )}
                          {userRoles.length > 3 && (
                            <span className="access-chip access-chip--muted">+ {userRoles.length - 3} more</span>
                          )}
                        </div>
                        <div className="flex justify-center">
                          <span className={`access-status access-status--${statusKey(user.status)}`}>{user.status}</span>
                        </div>
                        <div className="flex justify-end items-center gap-3">
                          <button
                            type="button"
                            className="access-inline-btn"
                            title="Manage access"
                            onClick={() => openUserAccessModal(user)}
                          >
                            <EditIcon />
                          </button>
                          <button
                            type="button"
                            className="access-inline-btn access-inline-btn--danger"
                            title="Delete user"
                            onClick={() => confirmDeleteUser(user.id)}
                            disabled={loading}
                          >
                            <TrashIcon />
                          </button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}

        {activeSection === 'roles' && (
          <div className="space-y-4">
            {roleDefinitions.length === 0 ? (
              <div className="access-empty-card">
                <p className="font-medium text-[var(--text-primary)]">No roles yet</p>
                <p className="text-sm text-[var(--text-secondary)]">Create a role and attach policies.</p>
              </div>
            ) : (
              <div className="glass-card p-0 border border-[var(--border-primary)] rounded-xl overflow-hidden">
                <div className="grid grid-cols-[1.3fr,1.3fr,1fr,0.8fr,auto] text-[11px] uppercase tracking-[0.08em] text-[var(--text-tertiary)] px-4 py-3 bg-[var(--bg-tertiary)] border-b border-[var(--border-primary)]">
                  <span>Role</span>
                  <span>Policies</span>
                  <span>Users</span>
                  <span className="text-center">Tenant</span>
                  <span className="text-right">Manage</span>
                </div>
                <div className="divide-y divide-[var(--border-primary)]">
                  {roleDefinitions.map(role => {
                    const assignedUsers = roleUserMap.get(role.id) || [];
                    return (
                      <div
                        key={role.id}
                        className="grid grid-cols-[1.3fr,1.3fr,1fr,0.8fr,auto] items-center px-4 py-3 gap-3 text-sm text-[var(--text-primary)] access-row"
                      >
                        <div className="min-w-0">
                          <p className="font-semibold truncate">{role.role}</p>
                          <p className="text-[11px] text-[var(--text-secondary)] truncate">{role.policies.length} policy(ies)</p>
                        </div>
                        <div className="flex flex-wrap gap-1.5">
                          {role.policies.slice(0, 3).map(policy => (
                            <span key={policyKey(policy)} className="access-chip access-chip--muted">
                              {policyLabel(policy)}
                            </span>
                          ))}
                          {role.policies.length > 3 && (
                            <span className="access-chip access-chip--muted">+ {role.policies.length - 3} more</span>
                          )}
                        </div>
                        <div className="flex flex-wrap gap-1.5">
                          {assignedUsers.slice(0, 3).map(item => (
                            <span key={`${role.id}-${item.userId}`} className="access-chip access-chip--muted">
                              {item.user}
                            </span>
                          ))}
                          {assignedUsers.length > 3 && (
                            <span className="access-chip access-chip--muted">+ {assignedUsers.length - 3} more</span>
                          )}
                          {assignedUsers.length === 0 && <span className="text-xs text-[var(--text-secondary)]">No users</span>}
                        </div>
                        <span className="text-[var(--text-secondary)] text-center truncate">{tenantLabel(role.tenant)}</span>
                        <div className="flex justify-end items-center gap-3">
                          <button
                            type="button"
                            className="access-inline-btn"
                            title={isDefaultAdmin(role.role, role.tenant) ? 'Protected role' : 'Edit role policies'}
                            onClick={() => openEditRoleEditor(role)}
                            disabled={isDefaultAdmin(role.role, role.tenant)}
                          >
                            <EditIcon />
                          </button>
                          <button
                            type="button"
                            className="access-inline-btn access-inline-btn--danger"
                            title={isDefaultAdmin(role.role, role.tenant) ? 'Protected role' : 'Delete role'}
                            onClick={() => confirmDeleteRoleDefinition(role)}
                            disabled={isDefaultAdmin(role.role, role.tenant)}
                          >
                            <TrashIcon />
                          </button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}

        {activeSection === 'policies' && (
          <div className="space-y-4">
            {policiesError && <div className="text-sm text-red-500">Failed to load policies: {policiesError}</div>}
            {policiesLoading ? (
              <div className="access-empty-card">
                <p className="font-medium text-[var(--text-primary)]">Loading policies…</p>
                <p className="text-sm text-[var(--text-secondary)]">Fetching policies.</p>
              </div>
            ) : policyTemplates.length === 0 ? (
              <div className="access-empty-card">
                <p className="font-medium text-[var(--text-primary)]">No policies yet</p>
                <p className="text-sm text-[var(--text-secondary)]">Use the plus button to define a rule.</p>
              </div>
            ) : (
              <div className="glass-card p-0 border border-[var(--border-primary)] rounded-xl overflow-hidden">
                <div className="grid grid-cols-[1.5fr,1.1fr,1fr,0.9fr,auto] text-[11px] uppercase tracking-[0.08em] text-[var(--text-tertiary)] px-4 py-3 bg-[var(--bg-tertiary)] border-b border-[var(--border-primary)]">
                  <span>Policy</span>
                  <span>Object</span>
                  <span>Action</span>
                  <span className="text-center">Tenant</span>
                  <span className="text-right">Manage</span>
                </div>
                <div className="divide-y divide-[var(--border-primary)]">
                  {policyTemplates.map(policy => (
                    <div
                      key={`${policy.role}-${policy.tenant_id}-${policy.obj}-${policy.act}`}
                      className="grid grid-cols-[1.5fr,1.1fr,1fr,0.9fr,auto] items-center px-4 py-3 gap-3 text-sm text-[var(--text-primary)] access-policy-row"
                    >
                      <div className="min-w-0 font-semibold truncate">{policyLabel(policy)}</div>
                      <span className="access-policy-text truncate" title={policy.obj}>
                        {policy.obj}
                      </span>
                      <span className="access-policy-text access-policy-text--muted truncate" title={policy.act}>
                        {policy.act}
                      </span>
                      <span className="text-[var(--text-secondary)] text-center truncate">{tenantLabel(policy.tenant_id)}</span>
                      <div className="flex justify-end items-center gap-3">
                        <button
                          type="button"
                          className="access-inline-btn"
                          title={isDefaultAdmin(policy.role, policy.tenant_id) ? 'Protected policy' : 'Edit policy'}
                          onClick={() => openPolicyEditModal(policy)}
                          disabled={isDefaultAdmin(policy.role, policy.tenant_id)}
                        >
                          <EditIcon />
                        </button>
                        <button
                          type="button"
                          className="access-inline-btn access-inline-btn--danger"
                          title={isDefaultAdmin(policy.role, policy.tenant_id) ? 'Protected policy' : 'Delete policy'}
                          onClick={() => confirmDeletePolicy(policy)}
                          disabled={isDefaultAdmin(policy.role, policy.tenant_id)}
                        >
                          <TrashIcon />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      <datalist id={tenantDatalistId}>
        {tenants.map(t => (
          <option key={t.id} value={t.name || t.id} />
        ))}
      </datalist>
      <datalist id={userDatalistId}>
        {users.map(u => (
          <option key={u.id} value={u.id} label={`${u.sub} (${u.email || 'No email'})`} />
        ))}
      </datalist>
      {Array.from(rolesByTenant.entries()).map(([tenantKey, roles]) => (
        <datalist key={tenantKey} id={roleListIdForTenant(tenantKey)}>
          {roles.map(role => (
            <option key={`${tenantKey}-${role}`} value={role} />
          ))}
        </datalist>
      ))}
      <datalist id={roleDatalistId}>
        {allRoleOptions.map(role => (
          <option key={`all-${role}`} value={role} />
        ))}
      </datalist>

      {confirmDialog && (
        <AccessModal
          kicker="Confirm"
          title="Please confirm"
          subtitle="This action cannot be undone."
          onClose={() => setConfirmDialog(null)}
          icon={<TrashIcon />}
          variant="minimal"
        >
          <div className="space-y-4">
            <p className="text-sm text-[var(--text-primary)]">{confirmDialog.message}</p>
            <div className="flex items-center justify-end gap-2">
              <button type="button" className="access-inline-btn" onClick={() => setConfirmDialog(null)} disabled={confirming}>
                Cancel
              </button>
              <button type="button" className="glass-button-danger" onClick={() => void handleConfirmDialog()} disabled={confirming}>
                {confirming ? 'Working…' : 'Delete'}
              </button>
            </div>
          </div>
        </AccessModal>
      )}

      {roleEditor && (
        <AccessModal
          kicker={roleEditor.mode === 'create' ? 'Role' : 'Role policies'}
          title={roleEditor.mode === 'create' ? 'Create role' : `Edit ${roleEditor.role}`}
          subtitle="Bundle policies and reuse them across users"
          onClose={() => setRoleEditor(null)}
          icon={<ShieldIcon />}
          variant="minimal"
        >
          <form className="access-minimal-form" onSubmit={handleSaveRoleEditor}>
            <div className="access-minimal-grid">
              <label className="access-minimal-label">
                <span>Role name</span>
                <input
                  className="pipelines-input"
                  value={roleEditor.role}
                  onChange={e => setRoleEditor(prev => (prev ? { ...prev, role: e.target.value } : prev))}
                  placeholder="nopsai-editor"
                  required
                  disabled={roleEditor.mode === 'edit'}
                />
              </label>
              <label className="access-minimal-label">
                <span>Tenant</span>
                <input
                  className="pipelines-input"
                  list={tenantDatalistId}
                  value={roleEditor.tenant}
                  onChange={e => setRoleEditor(prev => (prev ? { ...prev, tenant: e.target.value } : prev))}
                  placeholder="default"
                />
              </label>
            </div>
            <div className="access-minimal-section">
              <div className="access-minimal-section__header">
                <p className="text-sm font-medium text-[var(--text-primary)]">Policies</p>
                <div className="flex gap-2 flex-wrap items-center">
                  <span className="text-[11px] text-[var(--text-secondary)]">Current policies</span>
                </div>
              </div>
              <div className="space-y-2">
                {roleEditor.policies.map((policy, idx) => (
                  <div key={`policy-${idx}`} className="access-minimal-row">
                    <div className="flex-1 space-y-1">
                      <p className="font-semibold truncate">{policyLabel(policy)}</p>
                      <p className="text-[11px] text-[var(--text-secondary)] truncate">{policy.obj || 'Select an object'}</p>
                      <p className="text-[11px] text-[var(--text-secondary)] truncate">{policy.act || 'Select an action'}</p>
                    </div>
                    <button
                      type="button"
                      className="access-inline-btn access-inline-btn--danger"
                      onClick={() => removeRolePolicyDraft(idx)}
                      title="Remove policy"
                    >
                      <TrashIcon />
                    </button>
                  </div>
                ))}
                <div className="access-minimal-row access-minimal-row--add">
                  {availablePoliciesForRoleEditor.length > 0 ? (
                    <>
                      <select
                        className="pipelines-input w-full"
                        value={nextPolicyKey}
                        onChange={e => setNextPolicyKey(e.target.value)}
                      >
                        <option value="" disabled>
                          Add new policy…
                        </option>
                        {availablePoliciesForRoleEditor.map(item => (
                          <option key={item.key} value={item.key}>
                            {item.label}
                          </option>
                        ))}
                      </select>
                      <button
                        type="button"
                        className="access-inline-btn access-inline-btn--pill"
                        onClick={() => {
                          if (nextPolicyKey) {
                            addExistingPolicyDraft(nextPolicyKey);
                            setNextPolicyKey('');
                          }
                        }}
                        disabled={!nextPolicyKey}
                        title="Add policy"
                      >
                        <PlusIcon />
                      </button>
                    </>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No reusable policies available</p>
                  )}
                </div>
              </div>
            </div>
            <div className="access-modal-footer access-modal-footer--minimal">
              <button type="button" className="access-inline-btn" onClick={() => setRoleEditor(null)}>
                Cancel
              </button>
              <button type="submit" className="glass-button-primary" disabled={savingRoleEditor}>
                {savingRoleEditor ? 'Saving…' : 'Save role'}
              </button>
            </div>
          </form>
        </AccessModal>
      )}

      {policyEditor && (
        <AccessModal
          kicker="Policy"
          title="Edit policy"
          subtitle="Update role binding for this rule"
          onClose={() => setPolicyEditor(null)}
          icon={<SparkIcon />}
        >
          <form className="space-y-3" onSubmit={handleSavePolicyEdit}>
            <label className="flex flex-col gap-1 text-sm">
              <span>Tenant</span>
              <input
                className="pipelines-input"
                list={tenantDatalistId}
                value={policyEditor.tenant}
                onChange={e => setPolicyEditor(prev => (prev ? { ...prev, tenant: e.target.value } : prev))}
                placeholder="default"
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Policy name</span>
              <input
                className="pipelines-input"
                value={policyEditor.name}
                onChange={e => setPolicyEditor(prev => (prev ? { ...prev, name: e.target.value } : prev))}
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Object pattern</span>
              <input
                className="pipelines-input"
                value={policyEditor.obj}
                onChange={e => setPolicyEditor(prev => (prev ? { ...prev, obj: e.target.value } : prev))}
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Action regex</span>
              <input
                className="pipelines-input"
                value={policyEditor.act}
                onChange={e => setPolicyEditor(prev => (prev ? { ...prev, act: e.target.value } : prev))}
                required
              />
            </label>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button type="button" className="glass-button-ghost" onClick={() => setPolicyEditor(null)}>
                  Cancel
                </button>
                <button type="submit" className="glass-button-primary" disabled={savingPolicy}>
                  {savingPolicy ? 'Saving…' : 'Save changes'}
                </button>
              </div>
            </div>
          </form>
        </AccessModal>
      )}

      {userAccessEditor && (
        <AccessModal
          kicker="User access"
          title={`Manage ${userAccessEditor.user.sub}`}
          subtitle="Add or remove tenant-scoped roles"
          onClose={() => setUserAccessEditor(null)}
          icon={<ShieldIcon />}
        >
          <form className="space-y-3" onSubmit={handleSaveUserAccess}>
            <div className="grid gap-3 md:grid-cols-2">
              <label className="flex flex-col gap-1 text-sm">
                <span>Email</span>
                <input
                  className="pipelines-input"
                  type="email"
                  value={userAccessEditor.email}
                  onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, email: e.target.value } : prev))}
                  placeholder="name@example.com"
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>Status</span>
                <select
                  className="pipelines-input"
                  value={userAccessEditor.status}
                  onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, status: e.target.value } : prev))}
                  disabled={userAccessEditor.user.sub === 'admin'}
                >
                  <option value="active">Active</option>
                  <option value="disabled">Disabled</option>
                </select>
              </label>
            </div>
            <label className="flex flex-col gap-1 text-sm">
              <span>New password</span>
              <input
                className="pipelines-input"
                type="password"
                value={userAccessEditor.password}
                onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, password: e.target.value } : prev))}
                placeholder="Leave blank to keep current password"
              />
              <span className="text-[11px] text-[var(--text-secondary)]">Resets the user's password.</span>
            </label>
            <div className="space-y-2">
              {userAccessEditor.entries.length === 0 && (
                <p className="text-[12px] text-[var(--text-secondary)]">No roles assigned yet.</p>
              )}
              {userAccessEditor.entries.map((entry, idx) => {
                const protectedAdmin = isDefaultAdmin(entry.role, entry.tenant) && userAccessEditor.user.sub === 'admin';
                const label = protectedAdmin ? 'Protected admin role' : 'Remove assignment';
                return (
                  <div key={`user-role-${idx}`} className="access-minimal-row justify-between">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-[var(--text-primary)]">{entry.role || 'Role'}</span>
                    </div>
                    <button
                      type="button"
                      className={`access-inline-btn access-inline-btn--danger access-role-remove flex items-center gap-1 ${
                        protectedAdmin ? 'opacity-60 cursor-not-allowed' : ''
                      }`}
                      onClick={() => removeUserAccessEntry(idx)}
                      title={label}
                      aria-label={label}
                      disabled={protectedAdmin}
                    >
                      <TrashIcon />
                    </button>
                  </div>
                );
              })}
            </div>
            <div className="grid gap-2 sm:grid-cols-[1fr_auto] items-center">
              <select
                className="pipelines-input w-full"
                value={nextAccessRole}
                onChange={e => setNextAccessRole(e.target.value)}
              >
                <option value="">{accessRoleOptionsForTenant.length === 0 ? 'No roles available' : 'Select a role'}</option>
                {accessRoleOptionsForTenant.map(role => (
                  <option key={`access-role-${role}`} value={role}>
                    {role}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="glass-button-subtle flex items-center justify-center gap-1"
                onClick={addUserAccessEntry}
                disabled={!nextAccessRole || accessRoleOptionsForTenant.length === 0}
              >
                <PlusIcon />
                <span>Add role</span>
              </button>
            </div>
            <div className="flex justify-end">
              <button type="submit" className="glass-button-primary" disabled={savingUserAccess}>
                {savingUserAccess ? 'Saving…' : 'Save access'}
              </button>
            </div>
          </form>
        </AccessModal>
      )}

      {showUserModal && (
        <AccessModal
          kicker="Directory"
          title="Create user"
          subtitle="Provision a local account for Nopsai"
          onClose={() => setShowUserModal(false)}
          icon={<PlusIcon />}
          variant="minimal"
        >
          <form className="access-minimal-form" onSubmit={onCreateUser}>
            <div className="access-minimal-grid">
              <label className="access-minimal-label">
                <span>Username (sub)</span>
                <input
                  className="pipelines-input"
                  value={newUser.sub}
                  onChange={e => onChangeUser({ ...newUser, sub: e.target.value })}
                  placeholder="admin"
                  required
                />
              </label>
              <label className="access-minimal-label">
                <span>Email</span>
                <input
                  className="pipelines-input"
                  type="email"
                  value={newUser.email}
                  onChange={e => onChangeUser({ ...newUser, email: e.target.value })}
                  placeholder="name@example.com"
                />
              </label>
            </div>
            <div className="access-minimal-grid">
              <label className="access-minimal-label">
                <span>Password</span>
                <input
                  className="pipelines-input"
                  type="password"
                  value={newUser.password}
                  onChange={e => onChangeUser({ ...newUser, password: e.target.value })}
                  placeholder="••••••••"
                  required
                />
              </label>
              <label className="access-minimal-label">
                <span>Tenant</span>
                <input
                  className="pipelines-input"
                  list={tenantDatalistId}
                  value={newUser.tenant}
                  onChange={e => {
                    const nextTenant = tenantKeyFromInput(e.target.value);
                    onChangeUser({
                      ...newUser,
                      tenant: nextTenant,
                      roles: [],
                    });
                    setNextUserRole('');
                  }}
                  placeholder="default"
                />
              </label>
            </div>
            <div className="access-minimal-section">
              <div className="access-minimal-section__header">
                <div className="space-y-1">
                  <p className="text-sm font-medium text-[var(--text-primary)]">Roles</p>
                  <p className="text-[11px] text-[var(--text-secondary)]">
                    Scoped to tenant {tenantLabel(newUser.tenant || 'default')}
                  </p>
                </div>
              </div>
              {tenantRoleOptions.length === 0 && (
                <p className="text-[11px] text-[var(--text-secondary)]">No roles available for this tenant.</p>
              )}
              {newUser.roles.length === 0 && (
                <p className="text-[11px] text-[var(--text-secondary)]">Pick at least one role to create this user.</p>
              )}
              <div className="space-y-2">
                {newUser.roles.map((entry, idx) => (
                  <div key={`new-user-role-${idx}`} className="access-minimal-row">
                    <select
                      className="pipelines-input flex-1"
                      value={entry.role}
                      onChange={e => updateNewUserRoleEntry(idx, e.target.value)}
                      required
                      disabled={tenantRoleOptions.length === 0}
                    >
                      <option value="" disabled>
                        {tenantRoleOptions.length === 0 ? 'No roles in tenant' : 'Pick a role'}
                      </option>
                      {tenantRoleOptions.map(role => (
                        <option key={`role-opt-${role}`} value={role}>
                          {role}
                        </option>
                      ))}
                    </select>
                    <span className="text-[11px] text-[var(--text-secondary)] min-w-[88px] text-right">
                      {tenantLabel(newUser.tenant || 'default')}
                    </span>
                    <button
                      type="button"
                      className="access-inline-btn access-inline-btn--danger"
                      onClick={() => removeNewUserRoleEntry(idx)}
                      title="Remove role"
                    >
                      <TrashIcon />
                    </button>
                  </div>
                ))}
                <div className="access-minimal-row access-minimal-row--add">
                  <select
                    className="pipelines-input flex-1"
                    value={nextUserRole}
                    onChange={e => setNextUserRole(e.target.value)}
                    disabled={tenantRoleOptions.length === 0}
                  >
                    <option value="">
                      {tenantRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}
                    </option>
                    {tenantRoleOptions.map(role => (
                      <option key={`new-role-opt-${role}`} value={role}>
                        {role}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    className="access-inline-btn access-inline-btn--pill"
                    onClick={appendUserRoleFromPicker}
                    title="Add role"
                    disabled={tenantRoleOptions.length === 0}
                  >
                    <PlusIcon />
                  </button>
                </div>
              </div>
            </div>
            <div className="access-modal-footer access-modal-footer--minimal">
              <button type="button" className="access-inline-btn" onClick={() => setShowUserModal(false)}>
                Cancel
              </button>
              <button type="submit" className="glass-button-primary">
                Save user
              </button>
            </div>
          </form>
        </AccessModal>
      )}

      {showRoleModal && (
        <AccessModal
          kicker="Assignment"
          title="Assign role"
          subtitle="Map a user to a tenant-scoped role"
          onClose={() => setShowRoleModal(false)}
          icon={<ShieldIcon />}
        >
          <form className="space-y-3" onSubmit={onAssignRole}>
            <label className="flex flex-col gap-1 text-sm">
              <span>User ID</span>
              <input
                className="pipelines-input"
                list={userDatalistId}
                value={newRole.userId}
                onChange={e => onChangeRole({ ...newRole, userId: e.target.value })}
                placeholder="user UUID"
                required
              />
            </label>
            <div className="grid gap-3 md:grid-cols-2">
              <label className="flex flex-col gap-1 text-sm">
                <span>Tenant name</span>
                <input
                  className="pipelines-input"
                  list={tenantDatalistId}
                  value={newRole.tenant}
                    onChange={e => onChangeRole({ ...newRole, tenant: tenantKeyFromInput(e.target.value) })}
                    placeholder="default"
                    required
                  />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>Role</span>
                <input
                  className="pipelines-input"
                  list={roleListFor(newRole.tenant)}
                  value={newRole.role}
                  onChange={e => onChangeRole({ ...newRole, role: e.target.value })}
                  placeholder="nopsai-admin"
                  required
                />
              </label>
            </div>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button type="button" className="glass-button-ghost" onClick={() => setShowRoleModal(false)}>
                  Cancel
                </button>
                <button type="submit" className="glass-button-primary">
                  Add role
                </button>
              </div>
            </div>
          </form>
        </AccessModal>
      )}

      {showPolicyModal && (
        <AccessModal
          kicker="Policy"
          title="Create role policy"
          subtitle="Add a minimal rule for this role"
          onClose={() => setShowPolicyModal(false)}
          icon={<SparkIcon />}
        >
          <form className="space-y-3" onSubmit={onCreatePermission}>
            <label className="flex flex-col gap-1 text-sm">
              <span>Tenant name</span>
              <input
                className="pipelines-input"
                list={tenantDatalistId}
                value={newPermission.tenant}
                onChange={e => onChangePermission({ ...newPermission, tenant: e.target.value })}
                placeholder="default"
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Policy name</span>
              <input
                className="pipelines-input"
                value={newPermission.name}
                onChange={e => onChangePermission({ ...newPermission, name: e.target.value })}
                placeholder="Editor API access"
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Object pattern</span>
              <input
                className="pipelines-input"
                value={newPermission.obj}
                onChange={e => onChangePermission({ ...newPermission, obj: e.target.value })}
                placeholder="/v1/pipelines/*"
                required
              />
              <p className="text-xs text-[var(--text-secondary)]">Supports path wildcards via keyMatch2 (e.g., /v1/runs/*).</p>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Action regex</span>
              <input
                className="pipelines-input"
                value={newPermission.act}
                onChange={e => onChangePermission({ ...newPermission, act: e.target.value })}
                placeholder="GET|POST"
                required
              />
            </label>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button type="button" className="glass-button-ghost" onClick={() => setShowPolicyModal(false)}>
                  Cancel
                </button>
                <button type="submit" className="glass-button-primary">
                  Add policy
                </button>
              </div>
            </div>
          </form>
        </AccessModal>
      )}
    </div>
  );
}

function AccessModal({
  kicker,
  title,
  subtitle,
  icon,
  onClose,
  children,
  variant = 'default',
}: {
  kicker: string;
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  variant?: 'default' | 'minimal';
}) {
  const minimal = variant === 'minimal';
  return (
    <div className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
      <div className={`pipelines-modal-card access-modal-card max-w-xl w-full ${minimal ? 'access-modal-card--minimal' : ''}`}>
        <header className={`pipelines-modal-header access-modal-header ${minimal ? 'access-modal-header--minimal' : ''}`}>
          <div className="access-modal-heading">
            {!minimal && (
              <span className="access-modal-icon" aria-hidden="true">
                {icon ?? <PlusIcon />}
              </span>
            )}
            <div className="min-w-0">
              {kicker && (
                <p
                  className={`pipelines-modal-kicker ${minimal ? 'text-[11px] tracking-[0.12em] uppercase text-[var(--text-tertiary)]' : 'text-xs text-[var(--text-secondary)]'}`}
                >
                  {kicker}
                </p>
              )}
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
              {subtitle && (
                <p className={`text-xs mt-1 ${minimal ? 'text-[var(--text-secondary)]' : 'text-[var(--text-secondary)]'}`}>
                  {subtitle}
                </p>
              )}
            </div>
          </div>
          <button className={minimal ? 'access-inline-btn' : 'glass-button-ghost'} onClick={onClose}>
            Close
          </button>
        </header>
        <div className={`pipelines-modal-body access-modal-body ${minimal ? 'access-modal-body--minimal' : ''}`}>{children}</div>
      </div>
    </div>
  );
}

function PlusIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 5v14M5 12h14" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 6h18" />
      <path d="M8 6v-2a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
      <path d="M10 11v6" />
      <path d="M14 11v6" />
    </svg>
  );
}

function EditIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L8 18l-4 1 1-4Z" />
    </svg>
  );
}

function RefreshIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 12a9 9 0 0 1 9-9 9 9 0 0 1 8 5" />
      <path d="M21 3v5h-5" />
      <path d="M21 12a9 9 0 0 1-9 9 9 9 0 0 1-8-5" />
      <path d="M3 21v-5h5" />
    </svg>
  );
}

function ShieldIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3l8 4v5c0 4.5-3.2 8.3-8 9-4.8-.7-8-4.5-8-9V7z" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  );
}

function SparkIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3v4" />
      <path d="M12 17v4" />
      <path d="M3 12h4" />
      <path d="M17 12h4" />
      <path d="m18.36 5.64-2.83 2.83" />
      <path d="m8.47 15.53-2.83 2.83" />
      <path d="m5.64 5.64 2.83 2.83" />
      <path d="m15.53 15.53 2.83 2.83" />
    </svg>
  );
}

function SystemConfig({
  config,
  envFilePath,
  syncStatus,
  syncError,
  configError,
  configLoading,
  saving,
  onChange,
  onReload,
  onRefreshSyncStatus,
  onSave,
  onTriggerSync,
}: {
  config: ConfigFormState;
  envFilePath: string;
  syncStatus: ConfigSyncStatus | null;
  syncError: string | null;
  configError: string | null;
  configLoading: boolean;
  saving: boolean;
  onChange: (next: ConfigFormState) => void;
  onReload: () => Promise<void>;
  onRefreshSyncStatus: (opts?: { quiet?: boolean }) => Promise<void>;
  onSave: () => Promise<void>;
  onTriggerSync: () => Promise<void>;
}) {
  const repoUrl = config.config_repo_url.trim();
  const statusKey = normalizeStatus(syncStatus?.status, repoUrl);
  const envPath = (envFilePath || '').trim();

  const handleChange = (key: keyof ConfigFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onChange({ ...config, [key]: value } as ConfigFormState);
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    void onSave();
  };

  return (
    <div id="system-config-section" className="grid gap-6 lg:grid-cols-2 pb-24">
      <form id="system-config-form" className="space-y-4 lg:col-span-2" onSubmit={onSubmit}>
        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">Git sync</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Setup & status</h3>
              <p className="text-xs text-[var(--text-secondary)]">Connect your repo and monitor sync health in one place.</p>
            </div>
            <div className="flex gap-2 flex-wrap">
              <button className="glass-button-ghost" type="button" onClick={() => void onRefreshSyncStatus()}>
                Refresh status
              </button>
              <button className="glass-button-subtle" type="button" onClick={() => void onTriggerSync()}>
                Sync config
              </button>
            </div>
          </div>

          <div className="grid gap-4 lg:grid-cols-[2fr_1fr] items-start">
            <div className="space-y-2">
              <label className="flex flex-col gap-1 text-sm">
                <span>Config repo URL</span>
                <input
                  id="system-config-repo"
                  type="text"
                  className="pipelines-input"
                  value={config.config_repo_url}
                  onChange={handleChange('config_repo_url')}
                  placeholder="https://github.com/org/repo"
                  disabled={configLoading || saving}
                />
              </label>
              <p className="text-xs text-[var(--text-secondary)]">Source of truth for pipelines, triggers, and steps.</p>
            </div>

            <div className="space-y-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-tertiary)] p-3">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-medium text-[var(--text-primary)] truncate">{repoUrl || 'Not configured'}</span>
                <span className={`system-chip ${repoUrl ? (statusKey === 'success' ? 'system-chip--success' : statusKey === 'error' ? 'system-chip--error' : statusKey === 'running' ? 'system-chip--warning' : 'system-chip--muted') : 'system-chip--muted'}`}>
                  {repoUrl ? (statusKey === 'success' ? 'Synced' : statusKey === 'error' ? 'Sync failed' : statusKey === 'running' ? 'Syncing' : 'Ready') : 'Not configured'}
                </span>
              </div>
              <p className="text-xs text-[var(--text-secondary)]">
                {syncStatus?.completed_at
                  ? `Last sync completed ${formatTimestamp(syncStatus.completed_at)}`
                  : syncStatus?.started_at
                    ? `Sync started ${formatTimestamp(syncStatus.started_at)}`
                    : repoUrl
                      ? 'Awaiting the first sync.'
                      : 'Set the Git URL to enable sync from source control.'}
              </p>
              {syncError && <p className="text-xs text-red-500">Sync status error: {syncError}</p>}
              {syncStatus?.details && Object.keys(syncStatus.details).length > 0 && (
                <ul className="sync-detail-list mt-1">
                  {Object.entries(syncStatus.details).map(([key, value]) => (
                    <li key={key}>
                      <strong>{key}:</strong> <span>{String(value)}</span>
                    </li>
                  ))}
                </ul>
              )}
              {envPath && <p className="text-xs text-[var(--text-secondary)]">Env file: {envPath}</p>}
            </div>
          </div>
        </div>

        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">Runtime tuning</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Runners & timeouts</h3>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="flex flex-col gap-1 text-sm">
              <span>Agent image</span>
              <input
                id="system-agent-image"
                type="text"
                className="pipelines-input"
                value={config.agent_image}
                onChange={handleChange('agent_image')}
                placeholder="nopsai-agent:latest"
                disabled={configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Docker network name</span>
              <input
                id="system-docker-network"
                type="text"
                className="pipelines-input"
                value={config.docker_network_name}
                onChange={handleChange('docker_network_name')}
                placeholder="nopsai-net"
                disabled={configLoading || saving}
              />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>Default pipeline timeout</span>
                <input
                  id="system-default-timeout"
                  type="text"
                  className="pipelines-input"
                  value={config.default_pipeline_timeout}
                  onChange={handleChange('default_pipeline_timeout')}
                  placeholder="30m"
                  disabled={configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>LLM agent timeout</span>
                <input
                  id="system-llm-timeout"
                  type="text"
                  className="pipelines-input"
                  value={config.llm_agent_timeout}
                  onChange={handleChange('llm_agent_timeout')}
                  placeholder="2m"
                  disabled={configLoading || saving}
                />
              </label>
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                id="system-auto-remove"
                type="checkbox"
                checked={config.auto_removal_agent_container}
                onChange={handleChange('auto_removal_agent_container')}
                disabled={configLoading || saving}
              />
              <span>Auto-remove agent containers</span>
            </label>
          </div>
        </div>

        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">Networking</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Service discovery</h3>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="flex flex-col gap-1 text-sm">
              <span>Agent ↔ Server API URL</span>
              <input
                id="system-agent-api"
                type="text"
                className="pipelines-input"
                value={config.agent_nopsai_api_url}
                onChange={handleChange('agent_nopsai_api_url')}
                placeholder="http://agent:8080"
                disabled={configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>GitBot ↔ Server API URL</span>
              <input
                id="system-gitbot-api"
                type="text"
                className="pipelines-input"
                value={config.git_bot_nopsai_api_url}
                onChange={handleChange('git_bot_nopsai_api_url')}
                placeholder="http://gitbot:8080"
                disabled={configLoading || saving}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm md:col-span-2">
              <span>NopsAI ↔ GitBot API URL</span>
              <input
                id="system-nopsai-gitbot-api"
                type="text"
                className="pipelines-input"
                value={config.nopsai_git_bot_api_url}
                onChange={handleChange('nopsai_git_bot_api_url')}
                placeholder="http://nopsai-gitbot:8080"
                disabled={configLoading || saving}
              />
            </label>
          </div>
        </div>

        {configError && (
          <div className="glass-card p-4 border border-red-500/30 rounded-xl text-sm text-red-500">
            Failed to load or save config: {configError}
          </div>
        )}
        {configLoading && <p className="text-sm text-[var(--text-secondary)]">Loading settings…</p>}
      </form>

      <div className="fixed bottom-6 right-6 z-40 flex items-center gap-2">
        <button className="glass-button-ghost" type="button" onClick={() => void onReload()} disabled={configLoading || saving}>
          Reload
        </button>
        <button className="glass-button-primary" type="button" onClick={() => void onSave()} disabled={configLoading || saving}>
          {saving ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </div>
  );
}

function DispatcherPanel({
  loading,
  error,
  status,
  pendingActions,
  onRefresh,
  onToggleRunnerDispatch,
}: {
  loading: boolean;
  error: string | null;
  status: { queuedJobs: number; runners: Runner[]; routing: Record<string, string[]>; fetchedAt: number } | null;
  pendingActions: Set<string>;
  onRefresh: () => void;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
}) {
  const runners = status?.runners ?? [];
  const runnerCount = runners.length;
  const queuedJobs = status?.queuedJobs ?? 0;
  const activeSum = runners.reduce((sum, r) => sum + (r.activeJobs || 0), 0);
  const updatedLabel = status?.fetchedAt ? `Updated ${new Date(status.fetchedAt).toLocaleTimeString()}` : 'Not loaded yet';
  const nowMs = status?.fetchedAt ?? 0;

  return (
    <div id="system-dispatcher-section" className="space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard label="Queued" value={queuedJobs} id="dispatcher-queue-count" />
        <StatCard label="Runners" value={runnerCount} id="dispatcher-runner-count" />
        <StatCard label="Active" value={activeSum} id="dispatcher-active-count" />
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Runners</h3>
          <div className="flex items-center gap-2">
            <span id="dispatcher-updated" className="text-sm text-[var(--text-secondary)]">
              {updatedLabel}
            </span>
            <button className="glass-button-ghost" type="button" onClick={onRefresh} disabled={loading}>
              Refresh
            </button>
          </div>
        </div>
        {error && <p className="text-sm text-red-500">Failed to load dispatcher status: {error}</p>}
        {loading && <p className="text-sm text-[var(--text-secondary)]">Loading runner status…</p>}
        <div id="dispatcher-runner-list" className="grid gap-4 md:grid-cols-2">
          {runners.map(runner => (
            <RunnerCard
              key={runnerActionKey(runner.runnerId, getRunnerMeta(runner).connectionId) || runner.runnerId}
              nowMs={nowMs}
              runner={runner}
              pendingActions={pendingActions}
              onToggleDispatch={onToggleRunnerDispatch}
            />
          ))}
        </div>
        {runners.length === 0 && (
          <div id="dispatcher-empty" className="text-sm text-[var(--text-secondary)]">
            No runners registered.
          </div>
        )}
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Routing</h3>
          <span className="text-sm text-[var(--text-secondary)]">Scope to runner mapping</span>
        </div>
        <RoutingMap routing={status?.routing ?? {}} />
      </div>
    </div>
  );
}

function RunnerCard({
  nowMs,
  runner,
  pendingActions,
  onToggleDispatch,
}: {
  nowMs: number;
  runner: Runner;
  pendingActions: Set<string>;
  onToggleDispatch: (runner: Runner) => Promise<void>;
}) {
  const stale = isStale(nowMs, runner.lastHeartbeatUnix);
  const paused = !runner.allowDispatch;
  const statusClass = stale ? 'runner-dot--error' : 'runner-dot--ok';
  const badgeLabel = stale ? 'Stale' : 'Healthy';
  const badgeClass = stale ? 'runner-pill--error' : 'runner-pill--ok';

  const meta = getRunnerMeta(runner);
  const connectionLabel = formatConnection(meta.connectionId);
  const pendingKey = runnerActionKey(runner.runnerId, meta.connectionId);
  const pending = Boolean(pendingKey && pendingActions.has(pendingKey));

  const toggleLabel = paused ? 'Resume' : 'Pause';
  const toggleTone = paused ? 'glass-button-primary' : 'glass-button-danger';
  const actionLabel = pending ? (paused ? 'Enabling…' : 'Pausing…') : toggleLabel;

  return (
    <div className={`runner-card glass-card p-5 space-y-4 ${paused ? 'runner-card--paused' : ''}`}>
      <div className="runner-card__header">
        <div className="runner-card__title">
          <span className={`runner-dot ${statusClass}`}></span>
          <div className="runner-card__title-stack">
            <div className={`runner-name ${paused ? 'runner-name--paused' : ''}`}>
              {runner.runnerId}
              {paused && <span className="runner-paused-label">Paused</span>}
            </div>
            <div className="runner-card__health-row">
              <span className={`runner-pill ${badgeClass}`}>{badgeLabel}</span>
            </div>
          </div>
        </div>
        <div className="runner-card__actions">
          <button
            type="button"
            className={`${toggleTone} text-xs ${pending ? 'opacity-60 cursor-wait' : ''}`}
            disabled={pending}
            onClick={() => void onToggleDispatch(runner)}
          >
            {actionLabel}
          </button>
        </div>
      </div>
      <div className="grid grid-cols-3 gap-2 runner-card__stat-grid text-xs">
        <div className="runner-stat">
          <span className="runner-stat__label">Active</span>
          <span className="runner-stat__value">{runner.activeJobs}</span>
        </div>
        <div className="runner-stat">
          <span className="runner-stat__label">Inflight</span>
          <span className="runner-stat__value">{runner.inflightJobs}</span>
        </div>
        <div className="runner-stat">
          <span className="runner-stat__label">Load</span>
          <span className="runner-stat__value">{runner.activeJobs}/{runner.capacity}</span>
        </div>
      </div>
      <div className="runner-card__meta-row text-xs text-[var(--text-secondary)]">
        <div className="flex flex-wrap gap-2">
          <span className="runner-pill runner-pill--muted">{runner.scopes.length ? runner.scopes.join(', ') : 'All scopes'}</span>
          {meta.hostname && <span className="runner-pill runner-pill--muted">{meta.hostname}</span>}
          {meta.network && <span className="runner-pill runner-pill--muted">{meta.network}</span>}
          <span className="runner-pill runner-pill--muted">Cap {runner.capacity}</span>
          {connectionLabel && <span className="runner-pill runner-pill--muted">{connectionLabel}</span>}
          <span className="runner-pill runner-pill--muted">Seen {formatSince(nowMs, runner.lastHeartbeatUnix)}</span>
        </div>
      </div>
      {meta.activeRuns.length > 0 ? (
        <div className="runner-run-list">
          {meta.activeRuns.slice(0, MAX_VISIBLE_ACTIVE_RUNS).map(run => {
            const runIdLabel = truncateId(run.runId, 6);
            const triggerLabel = truncateId(run.triggerId || 'manual', 6);
            const display = `${run.pipeline || 'Run'}-${triggerLabel}-${runIdLabel}`;
            const title = `${run.pipeline || 'Run'} • Trigger ${run.triggerId || 'manual'} • Run ${run.runId}`;
            const to = `/pipelineruns/recent?run_id=${encodeURIComponent(run.runId)}`;
            return (
              <Link key={run.runId} to={to} className="runner-pill runner-pill--link" title={title}>
                {display}
              </Link>
            );
          })}
          {meta.activeRuns.length > MAX_VISIBLE_ACTIVE_RUNS && (
            <span className="runner-pill runner-pill--muted">+{meta.activeRuns.length - MAX_VISIBLE_ACTIVE_RUNS}</span>
          )}
        </div>
      ) : (
        <p className="text-xs text-[var(--text-secondary)]">No active runs</p>
      )}
    </div>
  );
}

function StatCard({ label, value, id }: { label: string; value: number; id?: string }) {
  return (
    <div className="glass-card p-4 border border-[var(--border-primary)] rounded-xl" id={id}>
      <p className="text-xs text-[var(--text-secondary)]">{label}</p>
      <p className="text-2xl font-semibold">{value}</p>
    </div>
  );
}

function RoutingMap({ routing }: { routing: Record<string, string[]> }) {
  const rows = Object.entries(routing || {})
    .map(([scope, runners]) => ({
      scope: (scope || '*').trim() || '*',
      runners: Array.isArray(runners) && runners.length ? runners.map(r => (r || '*').trim() || '*') : ['*'],
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));

  if (rows.length === 0) {
    return (
      <div id="dispatcher-routing-empty" className="text-sm text-[var(--text-secondary)]">
        No routing rules configured.
      </div>
    );
  }

  return (
    <div id="dispatcher-routing" className="space-y-2">
      {rows.map(row => (
        <div
          key={row.scope}
          className="flex items-center justify-between gap-3 bg-[var(--bg-tertiary)] px-3 py-2 rounded-md border border-[var(--border-primary)]"
        >
          <span className="runner-pill runner-pill--ok">{row.scope}</span>
          <div className="flex flex-wrap gap-2 justify-end text-sm">
            {row.runners.map(r => (
              <span key={`${row.scope}-${r}`} className="runner-pill runner-pill--muted">
                {r === '*' ? 'Any' : r}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function formatConnection(connection: string) {
  const trimmed = connection.trim();
  if (!trimmed) return '';
  if (trimmed.length <= 14) return trimmed;
  return `${trimmed.slice(0, 6)}...${trimmed.slice(-4)}`;
}

function truncateId(value: string, length = 8) {
  const trimmed = (value || '').trim();
  if (!trimmed) return '';
  return trimmed.slice(0, length);
}

function formatTimestamp(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

function formatSince(nowMs: number, unixSeconds: number) {
  if (!unixSeconds) return 'never';
  const diff = nowMs - unixSeconds * 1000;
  if (diff < 0) return 'now';
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

function normalizeStatus(value: unknown, repoUrl = '') {
  const key = String(value ?? '').toLowerCase().trim();
  if (!key && !repoUrl.trim()) return 'missing';
  if (['running', 'loading', 'in_progress'].includes(key)) return 'running';
  if (['success', 'completed', 'complete', 'done'].includes(key)) return 'success';
  if (['error', 'failed', 'failure'].includes(key)) return 'error';
  return key || 'idle';
}

function normalizeConfigSyncStatus(value: unknown): ConfigSyncStatus | null {
  const record = asRecord(value);
  if (!record) return null;
  const status = readString(record.status);
  if (!status) return null;

  const detailsRaw = asRecord(record.details);
  const details: Record<string, number> = {};
  if (detailsRaw) {
    Object.entries(detailsRaw).forEach(([key, val]) => {
      const num = typeof val === 'number' ? val : Number(val);
      if (Number.isFinite(num)) {
        details[key] = num;
      }
    });
  }

  const normalized: ConfigSyncStatus = {
    status,
    message: readOptionalString(record.message),
    started_at: readOptionalString(record.started_at),
    completed_at: readOptionalString(record.completed_at),
  };
  if (Object.keys(details).length > 0) normalized.details = details;
  return normalized;
}

function normalizeDispatcherStatus(value: unknown): { queuedJobs: number; runners: Runner[]; routing: Record<string, string[]> } {
  const record = asRecord(value);
  const runnersRaw = record && Array.isArray(record.runners) ? record.runners : [];
  const routingRaw = record ? (record.routing ?? record.routing_map) : null;

  return {
    queuedJobs: record ? normalizeNumber(record.queued_jobs ?? record.queuedJobs) : 0,
    runners: runnersRaw.map(normalizeRunner).filter(runner => runner.runnerId),
    routing: normalizeRouting(routingRaw),
  };
}

function normalizeRunner(value: unknown): Runner {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    scopes: normalizeStringArray(record.scopes),
    capacity: normalizeNumber(record.capacity),
    activeJobs: normalizeNumber(record.active_jobs ?? record.activeJobs),
    inflightJobs: normalizeNumber(record.inflight_jobs ?? record.inflightJobs),
    lastHeartbeatUnix: normalizeNumber(record.last_heartbeat_unix ?? record.lastHeartbeatUnix),
    metadata: normalizeStringMap(record.metadata),
    allowDispatch: Boolean(record.allow_dispatch ?? record.allowDispatch),
  };
}

function getRunnerMeta(runner: Runner): RunnerMeta {
  const meta = runner.metadata || {};
  return {
    connectionId: readString(meta.connection_id || meta.instance_id),
    hostname: readString(meta.hostname || meta.host || meta.runner_host),
    network: readString(meta.docker_network || meta.docker_network_name || meta.docker_networkname),
    activeRuns: parseActiveRuns(meta),
  };
}

function parseActiveRuns(meta: Record<string, string>): RunnerActiveRun[] {
  const raw = (meta && meta.active_runs) || '';
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map(item => {
        const record = asRecord(item);
        if (!record) return null;
        const runId = readString(record.run_id);
        if (!runId) return null;
        return {
          runId,
          pipeline: readString(record.pipeline),
          parentStep: readOptionalString(record.parent_step),
          triggerId: readOptionalString(record.trigger_event_id),
        } satisfies RunnerActiveRun;
      })
      .filter(Boolean) as RunnerActiveRun[];
  } catch (error) {
    console.warn('Failed to parse active_runs metadata', error);
    return [];
  }
}

function runnerActionKey(runnerId: string, connectionId = '') {
  const rid = (runnerId || '').trim();
  const cid = (connectionId || '').trim();
  if (!rid) return '';
  return cid ? `${rid}::${cid}` : rid;
}

function isStale(nowMs: number, lastHeartbeatUnix: number) {
  if (!lastHeartbeatUnix) return true;
  return nowMs - lastHeartbeatUnix * 1000 > STALE_THRESHOLD_MS;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object') return null;
  return value as Record<string, unknown>;
}

function readString(value: unknown): string {
  if (typeof value !== 'string') return '';
  return value;
}

function readOptionalString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  return value;
}

function normalizeNumber(value: unknown): number {
  const num = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(num) ? num : 0;
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map(item => String(item || '').trim()).filter(Boolean);
}

function normalizeStringMap(value: unknown): Record<string, string> {
  const record = asRecord(value);
  if (!record) return {};
  const normalized: Record<string, string> = {};
  Object.entries(record).forEach(([key, val]) => {
    if (!key) return;
    normalized[key] = typeof val === 'string' ? val : String(val ?? '');
  });
  return normalized;
}

function normalizeRouting(value: unknown): Record<string, string[]> {
  const record = asRecord(value);
  if (!record) return {};
  const normalized: Record<string, string[]> = {};
  Object.entries(record).forEach(([scope, runners]) => {
    if (!scope) return;
    if (Array.isArray(runners)) {
      normalized[scope] = runners.map(item => String(item || '').trim()).filter(Boolean);
    } else if (typeof runners === 'string') {
      normalized[scope] = [runners.trim()].filter(Boolean);
    }
  });
  return normalized;
}

export default SystemPage;
