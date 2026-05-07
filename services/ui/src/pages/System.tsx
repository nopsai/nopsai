import { Link, useNavigate, useParams } from 'react-router-dom';
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
const DEFAULT_ADMIN_ROLE = 'nopsai-admin';
const DEFAULT_ADMIN_POLICY_OBJ = '*:*';
const DEFAULT_ADMIN_POLICY_ACT = '*';

type UserRole = {
  role: string;
};

type RolePermission = {
  role: string;
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

type RolePolicyDraft = {
  name: string;
  obj: string;
  act: string;
};

type RoleDefinition = {
  id: string;
  role: string;
  policies: RolePermission[];
};

type ResourceGroup = {
  id: number;
  name: string;
  parent_id?: number | null;
};

type SystemPagePermissions = {
  canViewConfig: boolean;
  canManageConfig: boolean;
  canViewDispatcher: boolean;
  canManageDispatcher: boolean;
  canViewAccess: boolean;
};

function SystemPage({ permissions }: { permissions: SystemPagePermissions }) {
  const params = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const activeTab = params.tab === 'dispatcher' ? 'dispatcher' : params.tab === 'access' ? 'access' : 'config';
  const allowedTabs = useMemo(() => {
    const tabs: Array<'config' | 'dispatcher' | 'access'> = [];
    if (permissions.canViewConfig) tabs.push('config');
    if (permissions.canViewDispatcher) tabs.push('dispatcher');
    if (permissions.canViewAccess) tabs.push('access');
    return tabs;
  }, [permissions.canViewAccess, permissions.canViewConfig, permissions.canViewDispatcher]);
  const visibleTab = allowedTabs.includes(activeTab) ? activeTab : allowedTabs[0] ?? activeTab;

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
  const [newUser, setNewUser] = useState({ sub: '', email: '', password: '', roles: [] as string[] });
  const [newRole, setNewRole] = useState({ userId: '', role: '' });
  const [newPermission, setNewPermission] = useState({ name: '', obj: 'pipeline:*', act: 'pipeline.read' });
  const [ensuringDefaultAdmin, setEnsuringDefaultAdmin] = useState(false);

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (allowedTabs.includes(activeTab)) return;
    const nextTab = allowedTabs[0];
    if (!nextTab) return;
    navigate(`/system/${nextTab}`, { replace: true });
  }, [activeTab, allowedTabs, navigate]);

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
      const roleAssignments = (newUser.roles || [])
        .map(role => role.trim())
        .filter(Boolean)
        .filter((role, index, roles) => roles.indexOf(role) === index);
      if (roleAssignments.length === 0) {
        addToast('Add at least one role before creating a user.', 'error');
        return;
      }
      try {
        const primaryRole = roleAssignments[0];
        const created = (await fetchJson('/v1/admin/users', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sub: newUser.sub.trim(),
            email: newUser.email.trim(),
            password: newUser.password,
            role: primaryRole,
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

        for (const role of roleAssignments.slice(1)) {
          try {
            await fetchJson('/v1/admin/user-roles', {
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

        addToast(`User ${newUser.sub} saved with ${roleAssignments.length} role(s)`, 'success');
        setNewUser({ sub: '', email: '', password: '', roles: [] });
        await loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to create user', 'error');
      }
    },
    [addToast, fetchJson, loadUsers, newUser.email, newUser.password, newUser.roles, newUser.sub]
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
            role: newRole.role.trim(),
          }),
        });
        addToast('Role added', 'success');
        setNewRole({ userId: '', role: '' });
        await loadUsers();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to add role', 'error');
      }
    },
    [addToast, fetchJson, loadUsers, newRole.role, newRole.userId]
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
        await fetchJson('/v1/admin/roles', {
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
        setNewPermission({ name: '', obj: 'pipeline:*', act: 'pipeline.read' });
        await loadPolicies();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to add policy', 'error');
      }
    },
    [addToast, fetchJson, loadPolicies, newPermission.act, newPermission.name, newPermission.obj]
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
      if (isDefaultAdmin(policy.role)) {
        addToast('Default admin policy cannot be deleted.', 'error');
        return;
      }
      try {
        await fetchJson('/v1/admin/roles', {
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
    [addToast, fetchJson, loadPolicies]
  );

  const deleteRoleDefinition = useCallback(
    async (role: RoleDefinition) => {
      if (isDefaultAdmin(role.role)) {
        addToast('Default admin role cannot be deleted.', 'error');
        return;
      }
      const inUse = users.some(user => (user.roles || []).some(r => r.role === role.role));
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
    [addToast, fetchJson, loadPolicies, users]
  );

  const saveRoleDefinition = useCallback(
    async ({ role, policies: drafts, original }: { role: string; policies: RolePolicyDraft[]; original?: RolePermission[] }) => {
      const roleName = role.trim();
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
          await fetchJson('/v1/admin/roles', {
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
          await fetchJson('/v1/admin/roles', {
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
          await fetchJson('/v1/admin/roles', {
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
          await fetchJson('/v1/admin/roles', {
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
    [addToast, fetchJson, loadPolicies]
  );

  const editPolicy = useCallback(
    async (current: RolePermission, next: { role: string; name: string; obj: string; act: string }) => {
      const nextRole = next.role.trim();
      const nextObj = next.obj.trim();
      const nextAct = next.act.trim();
      const nextName = next.name.trim() || policyName(nextObj, nextAct);
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
      try {
        await fetchJson('/v1/admin/roles', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: current.role,
            obj: current.obj,
            act: current.act,
          }),
        });
        await fetchJson('/v1/admin/roles', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: nextRole,
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
          await fetchJson('/v1/admin/user-roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              user_id: userId,
              role,
            }),
          });
        }
        for (const role of toAdd) {
          await fetchJson('/v1/admin/user-roles', {
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
    if (!permissions.canViewConfig || visibleTab !== 'config') return;
    void loadSystemConfig();
  }, [loadSystemConfig, permissions.canViewConfig, visibleTab]);

  useEffect(() => {
    if (permissions.canViewDispatcher && visibleTab === 'dispatcher') {
      void loadDispatcherStatus();
    }
  }, [loadDispatcherStatus, permissions.canViewDispatcher, visibleTab]);

  useEffect(() => {
    if (permissions.canViewAccess && visibleTab === 'access') {
      void loadUsers();
      void loadPolicies();
    }
  }, [loadPolicies, loadUsers, permissions.canViewAccess, visibleTab]);

  const defaultAdminPolicyExists = useMemo(
    () =>
      policies.some(p => p.role === DEFAULT_ADMIN_ROLE && isDefaultAdmin(p.role) && p.obj === DEFAULT_ADMIN_POLICY_OBJ && p.act === DEFAULT_ADMIN_POLICY_ACT),
    [policies]
  );

  useEffect(() => {
    if (!permissions.canViewAccess || visibleTab !== 'access') return;
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
  }, [defaultAdminPolicyExists, ensuringDefaultAdmin, fetchJson, loadPolicies, permissions.canViewAccess, policiesLoading, visibleTab]);

  useEffect(() => {
    const handle = window.setInterval(() => {
      if (permissions.canViewConfig && visibleTab === 'config') {
        void loadSyncStatus({ quiet: true });
      }
      if (permissions.canViewDispatcher && visibleTab === 'dispatcher') {
        void loadDispatcherStatus({ quiet: true });
      }
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [loadDispatcherStatus, loadSyncStatus, permissions.canViewConfig, permissions.canViewDispatcher, visibleTab]);

  return (
    <div data-page="system" className="active p-6 space-y-6">
      {visibleTab === 'config' && (
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
          canManageConfig={permissions.canManageConfig}
        />
      )}
      {visibleTab === 'dispatcher' && (
        <DispatcherPanel
          loading={dispatcherLoading}
          error={dispatcherError}
          status={dispatcherStatus}
          pendingActions={runnerActionPending}
          onRefresh={() => loadDispatcherStatus()}
          onToggleRunnerDispatch={toggleRunnerDispatch}
          canManageDispatcher={permissions.canManageDispatcher}
        />
      )}
      {visibleTab === 'access' && (
        <AccessPanel
          users={users}
          loading={usersLoading}
          error={usersError}
          policies={policies}
          policiesLoading={policiesLoading}
          policiesError={policiesError}
          newUser={newUser}
          newRole={newRole}
          policyTemplates={policyTemplates}
          onChangeUser={setNewUser}
          onChangeRole={setNewRole}
          onCreateUser={createUser}
          onAssignRole={assignRole}
          onReloadUsers={loadUsers}
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

const policyKey = (input: { role: string; obj: string; act: string }) =>
  `${(input.role || '').trim()}::${(input.obj || '').trim()}::${(input.act || '').trim()}`;

const assignmentKey = (role: string) => (role || '').trim();

const policyName = (obj: string, act: string) => {
  const trimmed = (obj || '').replace(/^\/+|\/+$/g, '').trim();
  const leaf = trimmed.split('/').filter(Boolean).pop();
  const base = leaf || trimmed || obj || 'policy';
  const action = (act || '').trim() || 'ANY';
  return `${base} • ${action}`;
};

const policyLabel = (input: { name?: string; obj: string; act: string }) =>
  (input.name && input.name.trim()) || policyName(input.obj, input.act);
const isDefaultAdmin = (roleName: string) => roleName === DEFAULT_ADMIN_ROLE;

const normalizeAdminPolicies = (records: RolePermission[]): RolePermission[] => {
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

type AAAEffect = 'allow' | 'deny';

type AAAOption = {
  value: string;
  label: string;
};

type AAAOptionGroup = {
  label: string;
  options: AAAOption[];
};

type AAAResourceCatalog = {
  folderOptions: AAAOption[];
  pipelineOptions: AAAOption[];
  triggerOptions: AAAOption[];
  repositoryOptions: AAAOption[];
  secretScopeOptions: AAAOption[];
  variableScopeOptions: AAAOption[];
};

type AAAResourceTypeConfig = {
  value: string;
  label: string;
  targetLabel: string;
  allowAll?: boolean;
  allLabel?: string;
  presets?: AAAOption[];
  dynamicSource?: keyof AAAResourceCatalog;
  customPlaceholder?: string;
};

type AAANamedResourceDraft = {
  repoName: string;
  scope: string;
  name: string;
  hasScope: boolean;
};

type AAAScopeResponse = {
  scope?: string | null;
};

type AAASecretScopeSummary = {
  scope?: string | null;
  secret_count?: number | null;
};

const AAA_CUSTOM_VALUE = '__custom__';
const AAA_ANY_SCOPE_VALUE = '__any_scope__';
const AAA_DEFAULT_SCOPE_VALUE = '__default_scope__';

const AAA_RESOURCE_TYPE_CONFIGS: AAAResourceTypeConfig[] = [
  {
    value: '*',
    label: 'All resources',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All resources',
  },
  {
    value: 'iam',
    label: 'IAM',
    targetLabel: 'Area',
    presets: [{ value: 'admin', label: 'Admin' }],
    customPlaceholder: 'admin',
  },
  {
    value: 'audit',
    label: 'Audit',
    targetLabel: 'Log',
    presets: [{ value: 'authz', label: 'Authorization log' }],
    customPlaceholder: 'authz',
  },
  {
    value: 'system',
    label: 'System',
    targetLabel: 'Area',
    presets: [
      { value: 'config', label: 'Config' },
      { value: 'config-sync', label: 'Config sync' },
      { value: 'steps', label: 'Step catalog' },
    ],
    customPlaceholder: 'config',
  },
  {
    value: 'dispatcher',
    label: 'Dispatcher',
    targetLabel: 'Area',
    presets: [
      { value: 'status', label: 'Status' },
      { value: 'runners', label: 'Runners' },
    ],
    customPlaceholder: 'status',
  },
  {
    value: 'folder',
    label: 'Folder',
    targetLabel: 'Folder',
    allowAll: true,
    allLabel: 'All folders',
    dynamicSource: 'folderOptions',
    customPlaceholder: 'team/platform',
  },
  {
    value: 'pipeline',
    label: 'Pipeline',
    targetLabel: 'Pipeline',
    allowAll: true,
    allLabel: 'All pipelines',
    dynamicSource: 'pipelineOptions',
    customPlaceholder: 'team/build',
  },
  {
    value: 'pipeline_run',
    label: 'Pipeline run',
    targetLabel: 'Run',
    allowAll: true,
    allLabel: 'All runs',
    customPlaceholder: 'run-123',
  },
  {
    value: 'trigger',
    label: 'Trigger',
    targetLabel: 'Repository',
    allowAll: true,
    allLabel: 'All triggers',
    dynamicSource: 'triggerOptions',
    customPlaceholder: 'owner/repo',
  },
  {
    value: 'repository',
    label: 'Repository',
    targetLabel: 'Repository',
    allowAll: true,
    allLabel: 'All repositories',
    dynamicSource: 'repositoryOptions',
    customPlaceholder: 'owner/repo',
  },
  {
    value: 'secret',
    label: 'Secret',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All secrets',
    customPlaceholder: 'repo=owner/repo&scope=prod&name=TOKEN',
  },
  {
    value: 'variable',
    label: 'Variable',
    targetLabel: 'Scope',
    allowAll: true,
    allLabel: 'All variables',
    customPlaceholder: 'repo=owner/repo&scope=prod&name=TIMEOUT',
  },
];

const AAA_ALL_ACTION_OPTION_GROUPS: AAAOptionGroup[] = [
  {
    label: 'Global',
    options: [{ value: '*', label: 'All actions (*)' }],
  },
  {
    label: 'Administration',
    options: [
      { value: 'iam.admin', label: 'admin' },
      { value: 'audit.read', label: 'read' },
    ],
  },
  {
    label: 'System',
    options: [
      { value: 'system.read', label: 'read' },
      { value: 'system.update', label: 'update' },
    ],
  },
  {
    label: 'Folders',
    options: [
      { value: 'folder.list', label: 'list' },
      { value: 'folder.create', label: 'create' },
      { value: 'folder.move', label: 'move' },
      { value: 'folder.update', label: 'update' },
      { value: 'folder.delete', label: 'delete' },
    ],
  },
  {
    label: 'Pipelines',
    options: [
      { value: 'pipeline.list', label: 'list' },
      { value: 'pipeline.read', label: 'read' },
      { value: 'pipeline.create', label: 'create' },
      { value: 'pipeline.update', label: 'update' },
      { value: 'pipeline.delete', label: 'delete' },
      { value: 'pipeline.execute', label: 'execute' },
    ],
  },
  {
    label: 'Pipeline Runs',
    options: [
      { value: 'pipeline_run.list', label: 'list' },
      { value: 'pipeline_run.read', label: 'read' },
      { value: 'pipeline_run.rerun', label: 'rerun' },
      { value: 'pipeline_run.cancel', label: 'cancel' },
      { value: 'pipeline_run.finalize', label: 'finalize' },
      { value: 'pipeline_run.write_logs', label: 'write logs' },
      { value: 'pipeline_run.task_update', label: 'update task' },
      { value: 'pipeline_run.delete', label: 'delete' },
    ],
  },
  {
    label: 'Triggers',
    options: [
      { value: 'trigger.read', label: 'read' },
      { value: 'trigger.update', label: 'update' },
      { value: 'trigger.delete', label: 'delete' },
    ],
  },
  {
    label: 'Secrets',
    options: [
      { value: 'secret.list_metadata', label: 'list metadata' },
      { value: 'secret.read_value', label: 'read value' },
      { value: 'secret.write_value', label: 'write value' },
      { value: 'secret.delete', label: 'delete' },
    ],
  },
  {
    label: 'Variables',
    options: [
      { value: 'variable.list_metadata', label: 'list metadata' },
      { value: 'variable.read_value', label: 'read value' },
      { value: 'variable.write_value', label: 'write value' },
      { value: 'variable.delete', label: 'delete' },
    ],
  },
];

const AAA_ACTION_OPTION_GROUPS_BY_SELECTOR: Record<string, AAAOptionGroup[]> = {
  '*:*': AAA_ALL_ACTION_OPTION_GROUPS,
  'iam:admin': [{ label: 'IAM actions', options: [{ value: 'iam.admin', label: 'admin' }] }],
  'audit:authz': [{ label: 'Audit actions', options: [{ value: 'audit.read', label: 'read' }] }],
  'system:config': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:config-sync': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:steps': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'dispatcher:status': [{ label: 'Dispatcher actions', options: [{ value: 'system.read', label: 'read' }] }],
  'dispatcher:runners': [{ label: 'Dispatcher actions', options: [{ value: 'system.update', label: 'update' }] }],
  'repository:*': [{ label: 'Repository actions', options: [{ value: 'system.read', label: 'read' }] }],
};

const AAA_ACTION_OPTION_GROUPS_BY_RESOURCE_TYPE: Record<string, AAAOptionGroup[]> = {
  '*': AAA_ALL_ACTION_OPTION_GROUPS,
  folder: [{ label: 'Folder actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Folders')?.options || [] }],
  pipeline: [{ label: 'Pipeline actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Pipelines')?.options || [] }],
  pipeline_run: [{ label: 'Pipeline run actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Pipeline Runs')?.options || [] }],
  trigger: [{ label: 'Trigger actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Triggers')?.options || [] }],
  secret: [{ label: 'Secret actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Secrets')?.options || [] }],
  variable: [{ label: 'Variable actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Variables')?.options || [] }],
  system: [{ label: 'System actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'System')?.options || [] }],
  repository: [{ label: 'Repository actions', options: [{ value: 'system.read', label: 'read' }] }],
  audit: [{ label: 'Audit actions', options: [{ value: 'audit.read', label: 'read' }] }],
  iam: [{ label: 'IAM actions', options: [{ value: 'iam.admin', label: 'admin' }] }],
};

const getAAAResourceTypeConfig = (resourceType: string) =>
  AAA_RESOURCE_TYPE_CONFIGS.find(config => config.value === resourceType);

const dedupeAAAOptions = (options: AAAOption[]) => {
  const seen = new Set<string>();
  return options.filter(option => {
    const value = option.value.trim();
    if (!value || seen.has(value)) return false;
    seen.add(value);
    return true;
  });
};

const hasAAAOptionValue = (groups: AAAOptionGroup[], value: string) => {
  const normalized = (value || '').trim();
  return groups.some(group => group.options.some(option => option.value === normalized));
};

const parseAAAResourceSelector = (value: string): { resourceType: string; resourceID: string } => {
  const normalized = (value || '').trim();
  if (!normalized) return { resourceType: '', resourceID: '' };
  if (normalized === '*:*' || normalized === '*') return { resourceType: '*', resourceID: '*' };
  const separatorIndex = normalized.indexOf(':');
  const resourceType = separatorIndex >= 0 ? normalized.slice(0, separatorIndex) : normalized;
  const resourceID = separatorIndex >= 0 ? normalized.slice(separatorIndex + 1) : '*';
  return {
    resourceType: (resourceType || '').trim(),
    resourceID: separatorIndex >= 0 ? resourceID.trim() : '*',
  };
};

const buildAAAResourceSelector = (resourceType: string, resourceID: string, opts?: { preserveEmpty?: boolean }) => {
  const normalizedType = (resourceType || '').trim();
  const normalizedID = (resourceID || '').trim();
  if (!normalizedType) {
    return normalizedID;
  }
  if (normalizedType === '*') {
    return '*:*';
  }
  if (!normalizedID) {
    return opts?.preserveEmpty ? `${normalizedType}:` : `${normalizedType}:*`;
  }
  if (normalizedID === '*') {
    return `${normalizedType}:*`;
  }
  return `${normalizedType}:${normalizedID}`;
};

const flattenAAAOptionGroups = (groups: AAAOptionGroup[]) => groups.flatMap(group => group.options);

const buildAAAGroupOptions = (groups: ResourceGroup[]): AAAOption[] => {
  const byID = new Map(groups.map(group => [group.id, group]));

  const buildPath = (group: ResourceGroup, trail = new Set<number>()): string => {
    const name = (group.name || '').trim();
    if (!name) return '';
    if (trail.has(group.id)) return name;
    const nextTrail = new Set(trail);
    nextTrail.add(group.id);
    const parentID = group.parent_id ?? null;
    if (parentID == null) return name;
    const parent = byID.get(parentID);
    if (!parent) return name;
    const parentPath = buildPath(parent, nextTrail);
    return parentPath ? `${parentPath}/${name}` : name;
  };

  return dedupeAAAOptions(
    groups
      .map(group => {
        const path = buildPath(group);
        return path ? { value: path, label: `/${path}` } : null;
      })
      .filter(Boolean) as AAAOption[]
  ).sort((a, b) => a.value.localeCompare(b.value));
};

const buildAAAStringOptions = (values: string[]) =>
  dedupeAAAOptions(
    values
      .map(value => value.trim())
      .filter(Boolean)
      .map(value => ({ value, label: value }))
  ).sort((a, b) => a.value.localeCompare(b.value));

const normalizeAAAScopeOptionValue = (scope: string) => {
  const normalized = (scope || '').trim();
  return normalized || AAA_DEFAULT_SCOPE_VALUE;
};

const denormalizeAAAScopeOptionValue = (value: string) => (value === AAA_DEFAULT_SCOPE_VALUE ? '' : (value || '').trim());

const buildAAAScopeOptions = (values: string[]) =>
  dedupeAAAOptions(
    ['', ...values].map(value => {
      const normalized = (value || '').trim();
      return {
        value: normalizeAAAScopeOptionValue(normalized),
        label: normalized || 'Default scope',
      };
    })
  ).sort((a, b) => {
    if (a.value === AAA_DEFAULT_SCOPE_VALUE) return -1;
    if (b.value === AAA_DEFAULT_SCOPE_VALUE) return 1;
    return a.label.localeCompare(b.label, undefined, { sensitivity: 'base' });
  });

const parseAAANamedResourceID = (value: string): AAANamedResourceDraft => {
  const normalized = (value || '').trim();
  if (!normalized || normalized === '*') {
    return {
      repoName: '',
      scope: '',
      name: '',
      hasScope: false,
    };
  }

  const params = new URLSearchParams(normalized);
  return {
    repoName: (params.get('repo') || '').trim(),
    scope: (params.get('scope') || '').trim(),
    name: (params.get('name') || '').trim(),
    hasScope: params.has('scope'),
  };
};

const buildAAANamedResourceID = ({ repoName, scope, name, hasScope }: AAANamedResourceDraft) => {
  const params = new URLSearchParams();
  const normalizedRepoName = (repoName || '').trim();
  const normalizedScope = (scope || '').trim();
  if (normalizedRepoName) {
    params.set('repo', normalizedRepoName);
  }
  if (hasScope) {
    params.set('scope', normalizedScope);
  }
  const normalizedName = (name || '').trim();
  if (normalizedName) {
    params.set('name', normalizedName);
  }
  return params.toString() || '*';
};

const buildAAANamedResourceSelector = (resourceType: string, parts: AAANamedResourceDraft) =>
  buildAAAResourceSelector(resourceType, buildAAANamedResourceID(parts));

const buildAAAParentPathOptionGroups = (options: AAAOption[], labels: { root: string; parentPrefix: string }) => {
  const groups = new Map<string, { sortKey: string; label: string; options: AAAOption[] }>();

  options.forEach(option => {
    const normalizedValue = option.value.trim();
    if (!normalizedValue) return;

    const lastSlash = normalizedValue.lastIndexOf('/');
    const parentPath = lastSlash >= 0 ? normalizedValue.slice(0, lastSlash) : '';
    const itemLabel = lastSlash >= 0 ? normalizedValue.slice(lastSlash + 1) : normalizedValue;
    const groupKey = parentPath || '';
    const groupLabel = parentPath ? `${labels.parentPrefix}${parentPath}` : labels.root;
    const existing = groups.get(groupKey);

    if (existing) {
      existing.options.push({ value: normalizedValue, label: itemLabel || option.label || normalizedValue });
      return;
    }

    groups.set(groupKey, {
      sortKey: groupKey,
      label: groupLabel,
      options: [{ value: normalizedValue, label: itemLabel || option.label || normalizedValue }],
    });
  });

  return Array.from(groups.values())
    .sort((a, b) => {
      if (!a.sortKey) return -1;
      if (!b.sortKey) return 1;
      return a.sortKey.localeCompare(b.sortKey);
    })
    .map(group => ({
      label: group.label,
      options: group.options.sort((a, b) => a.label.localeCompare(b.label)),
    }));
};

const buildAAARepositoryOptionGroups = (options: AAAOption[], labels: { root: string; ownerPrefix: string }) => {
  const groups = new Map<string, { sortKey: string; label: string; options: AAAOption[] }>();

  options.forEach(option => {
    const normalizedValue = option.value.trim();
    if (!normalizedValue) return;

    const separatorIndex = normalizedValue.indexOf('/');
    const owner = separatorIndex >= 0 ? normalizedValue.slice(0, separatorIndex) : '';
    const repoName = separatorIndex >= 0 ? normalizedValue.slice(separatorIndex + 1) : normalizedValue;
    const groupKey = owner || '';
    const groupLabel = owner ? `${labels.ownerPrefix}${owner}` : labels.root;
    const existing = groups.get(groupKey);

    if (existing) {
      existing.options.push({ value: normalizedValue, label: repoName || option.label || normalizedValue });
      return;
    }

    groups.set(groupKey, {
      sortKey: groupKey,
      label: groupLabel,
      options: [{ value: normalizedValue, label: repoName || option.label || normalizedValue }],
    });
  });

  return Array.from(groups.values())
    .sort((a, b) => {
      if (!a.sortKey) return -1;
      if (!b.sortKey) return 1;
      return a.sortKey.localeCompare(b.sortKey);
    })
    .map(group => ({
      label: group.label,
      options: group.options.sort((a, b) => a.label.localeCompare(b.label)),
    }));
};

const buildAAAResourceTargetOptionGroups = (config: AAAResourceTypeConfig, catalog: AAAResourceCatalog) => {
  const groups: AAAOptionGroup[] = [];
  const scopeOptions: AAAOption[] = [];
  if (config.allowAll) {
    scopeOptions.push({ value: '*', label: config.allLabel || 'All' });
  }
  if (config.presets) {
    scopeOptions.push(...config.presets);
  }

  const normalizedScopeOptions = dedupeAAAOptions(scopeOptions);
  if (normalizedScopeOptions.length > 0) {
    groups.push({
      label: config.dynamicSource ? 'Scope' : 'Available targets',
      options: normalizedScopeOptions,
    });
  }

  if (config.dynamicSource) {
    const dynamicOptions = dedupeAAAOptions(catalog[config.dynamicSource]);
    switch (config.dynamicSource) {
      case 'folderOptions':
        groups.push(...buildAAAParentPathOptionGroups(dynamicOptions, { root: 'Top-level folders', parentPrefix: 'Inside /' }));
        break;
      case 'pipelineOptions':
        groups.push(...buildAAAParentPathOptionGroups(dynamicOptions, { root: 'Top-level pipelines', parentPrefix: 'Folder /' }));
        break;
      case 'triggerOptions':
        groups.push(...buildAAARepositoryOptionGroups(dynamicOptions, { root: 'Ungrouped triggers', ownerPrefix: 'Owner ' }));
        break;
      case 'repositoryOptions':
        groups.push(...buildAAARepositoryOptionGroups(dynamicOptions, { root: 'Ungrouped repositories', ownerPrefix: 'Owner ' }));
        break;
      default:
        if (dynamicOptions.length > 0) {
          groups.push({ label: 'Known targets', options: dynamicOptions });
        }
        break;
    }
  }

  return groups;
};

const getAAAActionOptionGroups = (resourceSelector: string): AAAOptionGroup[] => {
  const normalized = (resourceSelector || '').trim();
  if (!normalized) return [];
  if (AAA_ACTION_OPTION_GROUPS_BY_SELECTOR[normalized]) {
    return AAA_ACTION_OPTION_GROUPS_BY_SELECTOR[normalized];
  }
  const { resourceType } = parseAAAResourceSelector(normalized);
  return AAA_ACTION_OPTION_GROUPS_BY_RESOURCE_TYPE[resourceType] || [];
};

const actionLabelFromAAAValue = (value: string) => {
  const trimmed = (value || '').trim();
  if (!trimmed || trimmed === '*') return trimmed;
  const actionPart = trimmed.includes('.') ? trimmed.slice(trimmed.lastIndexOf('.') + 1) : trimmed;
  switch (actionPart) {
    case 'list_metadata':
      return 'list metadata';
    case 'read_value':
      return 'read value';
    case 'write_value':
      return 'write value';
    case 'write_logs':
      return 'write logs';
    case 'task_update':
      return 'update task';
    default:
      return actionPart.replace(/_/g, ' ');
  }
};

const normalizeAAAActionForResource = (resourceSelector: string, actionValue: string, effect: AAAEffect) => {
  const options = flattenAAAOptionGroups(getAAAActionOptionGroups(resourceSelector));
  if (options.length === 0) return formatAAAActionValue(effect, actionValue);
  const trimmed = (actionValue || '').trim();
  if (trimmed && options.some(option => option.value === trimmed)) {
    return formatAAAActionValue(effect, trimmed);
  }
  const currentLabel = actionLabelFromAAAValue(trimmed);
  if (currentLabel) {
    const matchingVerb = options.find(option => option.label === currentLabel);
    if (matchingVerb) {
      return formatAAAActionValue(effect, matchingVerb.value);
    }
  }
  return formatAAAActionValue(effect, options[0].value);
};

const customAAAActionPlaceholder = (resourceSelector: string) => {
  const options = flattenAAAOptionGroups(getAAAActionOptionGroups(resourceSelector));
  if (options.length > 0) return options[0].value;
  const { resourceType } = parseAAAResourceSelector(resourceSelector);
  if (!resourceType || resourceType === '*') return 'pipeline.read';
  return `${resourceType}.read`;
};

const parseAAAActionValue = (value: string): { effect: AAAEffect; action: string } => {
  const trimmed = (value || '').trim();
  if (!trimmed) return { effect: 'allow', action: '' };
  if (trimmed.startsWith('deny ')) {
    return {
      effect: 'deny',
      action: trimmed.slice('deny '.length).trim(),
    };
  }
  return { effect: 'allow', action: trimmed };
};

const formatAAAActionValue = (effect: AAAEffect, action: string) => {
  const trimmed = (action || '').trim();
  if (!trimmed) return '';
  return effect === 'deny' ? `deny ${trimmed}` : trimmed;
};

const selectValueForAAAOptions = (groups: AAAOptionGroup[], value: string) =>
  hasAAAOptionValue(groups, value) ? (value || '').trim() : AAA_CUSTOM_VALUE;

function AAAPolicyRuleFields({
  policy,
  onChange,
  resourceCatalog,
}: {
  policy: { name: string; obj: string; act: string };
  onChange: (next: { name: string; obj: string; act: string }) => void;
  resourceCatalog: AAAResourceCatalog;
}) {
  const normalizedResource = (policy.obj || '').trim();
  const parsedResource = parseAAAResourceSelector(normalizedResource);
  const parsedAction = parseAAAActionValue(policy.act);
  const resourceTypeConfig = getAAAResourceTypeConfig(parsedResource.resourceType);
  const selectedResourceType = resourceTypeConfig ? resourceTypeConfig.value : AAA_CUSTOM_VALUE;
  const isNamedScopedResourceType = resourceTypeConfig?.value === 'secret' || resourceTypeConfig?.value === 'variable';
  const [forceCustomNamedScope, setForceCustomNamedScope] = useState(false);
  const namedResourceParts = isNamedScopedResourceType ? parseAAANamedResourceID(parsedResource.resourceID) : { repoName: '', scope: '', name: '', hasScope: false };
  const namedScopeOptions =
    resourceTypeConfig?.value === 'secret'
      ? resourceCatalog.secretScopeOptions
      : resourceTypeConfig?.value === 'variable'
        ? resourceCatalog.variableScopeOptions
        : [];
  const resourceTargetOptionGroups =
    resourceTypeConfig && !isNamedScopedResourceType ? buildAAAResourceTargetOptionGroups(resourceTypeConfig, resourceCatalog) : [];
  const resourceTargetOptions = flattenAAAOptionGroups(resourceTargetOptionGroups);
  const selectedResourceTarget =
    isNamedScopedResourceType
      ? ''
      : selectedResourceType === '*'
      ? '*'
      : selectedResourceType !== AAA_CUSTOM_VALUE && resourceTargetOptions.some(option => option.value === parsedResource.resourceID)
      ? parsedResource.resourceID
      : AAA_CUSTOM_VALUE;
  const normalizedNamedScope = normalizeAAAScopeOptionValue(namedResourceParts.scope);
  const derivedSelectedNamedScope =
    !isNamedScopedResourceType
      ? ''
      : !namedResourceParts.hasScope
        ? AAA_ANY_SCOPE_VALUE
        : namedScopeOptions.some(option => option.value === normalizedNamedScope)
          ? normalizedNamedScope
          : AAA_CUSTOM_VALUE;
  const selectedNamedScope = forceCustomNamedScope && isNamedScopedResourceType ? AAA_CUSTOM_VALUE : derivedSelectedNamedScope;
  const allowCustomTarget = resourceTypeConfig?.value !== '*';
  const actionOptions = getAAAActionOptionGroups(normalizedResource);
  const selectedAction = selectValueForAAAOptions(actionOptions, parsedAction.action);
  const customResourceDraft =
    selectedResourceType === AAA_CUSTOM_VALUE
      ? normalizedResource
      : normalizedResource.endsWith(':')
        ? ''
        : parsedResource.resourceID;
  const customNamedScopeDraft = selectedNamedScope === AAA_CUSTOM_VALUE ? namedResourceParts.scope : '';
  const namedResourceLabel = resourceTypeConfig?.value === 'secret' ? 'Secret name' : 'Variable name';
  const buildNamedResourceSelector = (next: Partial<AAANamedResourceDraft>) =>
    buildAAANamedResourceSelector(resourceTypeConfig?.value || '', {
      repoName: 'repoName' in next ? next.repoName ?? '' : namedResourceParts.repoName,
      scope: 'scope' in next ? next.scope ?? '' : namedResourceParts.scope,
      name: 'name' in next ? next.name ?? '' : namedResourceParts.name,
      hasScope: 'hasScope' in next ? Boolean(next.hasScope) : namedResourceParts.hasScope,
    });

  useEffect(() => {
    if (!isNamedScopedResourceType) {
      setForceCustomNamedScope(false);
    }
  }, [isNamedScopedResourceType]);

  return (
    <>
      <label className="flex flex-col gap-1 text-sm">
        <span>Policy label</span>
        <input
          className="pipelines-input"
          value={policy.name}
          onChange={e => onChange({ name: e.target.value, obj: policy.obj, act: policy.act })}
          placeholder="Pipeline reader"
          required
        />
      </label>
      <div className="grid gap-3 md:grid-cols-[0.56fr_1fr]">
        <label className="flex flex-col gap-1 text-sm">
          <span>Resource type</span>
          <select
            className="pipelines-input"
            value={selectedResourceType}
            onChange={e => {
              const nextType = e.target.value;
              if (nextType === AAA_CUSTOM_VALUE) {
                onChange({ name: policy.name, obj: normalizedResource, act: policy.act });
                return;
              }
              const nextConfig = getAAAResourceTypeConfig(nextType);
              const nextTarget = nextConfig?.allowAll ? '*' : nextConfig?.presets?.[0]?.value || '';
              const nextObj = buildAAAResourceSelector(nextType, nextTarget, { preserveEmpty: nextTarget === '' });
              onChange({
                name: policy.name,
                obj: nextObj,
                act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
              });
            }}
          >
            {AAA_RESOURCE_TYPE_CONFIGS.map(option => (
              <option key={`resource-type-${option.value}`} value={option.value}>
                {option.label}
              </option>
            ))}
            <option value={AAA_CUSTOM_VALUE}>Custom selector…</option>
          </select>
        </label>
        {selectedResourceType === AAA_CUSTOM_VALUE ? (
          <label className="flex flex-col gap-1 text-sm">
            <span>Resource selector</span>
            <input
              className="pipelines-input"
              value={normalizedResource}
              onChange={e => onChange({ name: policy.name, obj: e.target.value, act: policy.act })}
              placeholder="pipeline:team/build"
              required
            />
          </label>
        ) : isNamedScopedResourceType ? (
          <div className="space-y-3">
            <label className="flex flex-col gap-1 text-sm">
              <span>{resourceTypeConfig?.targetLabel || 'Scope'}</span>
              <select
                className="pipelines-input"
                value={selectedNamedScope}
                onChange={e => {
                  const value = e.target.value;
                  if (!resourceTypeConfig) return;
                  if (value === AAA_ANY_SCOPE_VALUE) {
                    setForceCustomNamedScope(false);
                    const nextObj = buildNamedResourceSelector({ hasScope: false, scope: '' });
                    onChange({
                      name: policy.name,
                      obj: nextObj,
                      act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
                    });
                    return;
                  }
                  if (value === AAA_CUSTOM_VALUE) {
                    setForceCustomNamedScope(true);
                    return;
                  }
                  setForceCustomNamedScope(false);
                  const nextObj = buildNamedResourceSelector({
                    hasScope: true,
                    scope: denormalizeAAAScopeOptionValue(value),
                  });
                  onChange({
                    name: policy.name,
                    obj: nextObj,
                    act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
                  });
                }}
              >
                <option value={AAA_ANY_SCOPE_VALUE}>Any scope</option>
                {namedScopeOptions.map(option => (
                  <option key={`resource-scope-${resourceTypeConfig?.value}-${option.value}`} value={option.value}>
                    {option.label}
                  </option>
                ))}
                <option value={AAA_CUSTOM_VALUE}>Custom scope…</option>
              </select>
            </label>
            {selectedNamedScope === AAA_CUSTOM_VALUE && (
              <label className="flex flex-col gap-1 text-sm">
                <span>Custom scope</span>
                <input
                  className="pipelines-input"
                  value={customNamedScopeDraft}
                  onChange={e =>
                    onChange({
                      name: policy.name,
                      obj: buildNamedResourceSelector({
                        hasScope: true,
                        scope: e.target.value,
                      }),
                      act: policy.act,
                    })
                  }
                  placeholder="prod"
                />
              </label>
            )}
            <div className="grid gap-3 md:grid-cols-2">
              <label className="flex flex-col gap-1 text-sm">
                <span>{namedResourceLabel} (optional)</span>
                <input
                  className="pipelines-input"
                  value={namedResourceParts.name}
                  onChange={e =>
                    onChange({
                      name: policy.name,
                      obj: buildNamedResourceSelector({
                        name: e.target.value,
                      }),
                      act: policy.act,
                    })
                  }
                  placeholder={resourceTypeConfig?.value === 'secret' ? 'TOKEN' : 'TIMEOUT'}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>Repository (optional)</span>
                <input
                  className="pipelines-input"
                  value={namedResourceParts.repoName}
                  onChange={e =>
                    onChange({
                      name: policy.name,
                      obj: buildNamedResourceSelector({
                        repoName: e.target.value,
                      }),
                      act: policy.act,
                    })
                  }
                  placeholder="owner/repo"
                />
              </label>
            </div>
          </div>
        ) : (
          <label className="flex flex-col gap-1 text-sm">
            <span>{resourceTypeConfig?.targetLabel || 'Target'}</span>
            <select
              className="pipelines-input"
              value={selectedResourceTarget}
              onChange={e => {
                const value = e.target.value;
                if (!resourceTypeConfig) return;
                if (value === AAA_CUSTOM_VALUE) {
                  const nextObj = buildAAAResourceSelector(resourceTypeConfig.value, parsedResource.resourceID === '*' ? '' : parsedResource.resourceID, {
                    preserveEmpty: true,
                  });
                  onChange({ name: policy.name, obj: nextObj, act: policy.act });
                  return;
                }
                const nextObj = buildAAAResourceSelector(resourceTypeConfig.value, value);
                onChange({
                  name: policy.name,
                  obj: nextObj,
                  act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
                });
              }}
            >
              {resourceTargetOptionGroups.map(group => (
                <optgroup key={`resource-target-group-${resourceTypeConfig?.value}-${group.label}`} label={group.label}>
                  {group.options.map(option => (
                    <option key={`resource-target-${resourceTypeConfig?.value}-${option.value}`} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </optgroup>
              ))}
              {allowCustomTarget && <option value={AAA_CUSTOM_VALUE}>Custom…</option>}
            </select>
            {allowCustomTarget && selectedResourceTarget === AAA_CUSTOM_VALUE && (
              <input
                className="pipelines-input"
                value={customResourceDraft}
                onChange={e =>
                  onChange({
                    name: policy.name,
                    obj: buildAAAResourceSelector(resourceTypeConfig?.value || '', e.target.value, { preserveEmpty: true }),
                    act: policy.act,
                  })
                }
                placeholder={resourceTypeConfig?.customPlaceholder || 'team/build'}
                required
              />
            )}
          </label>
        )}
      </div>
      <p className="text-xs text-[var(--text-secondary)]">
        Pick a resource type first, then narrow it with a second selector for scope, existing items, or a custom identifier.
      </p>
      {isNamedScopedResourceType && selectedResourceType !== AAA_CUSTOM_VALUE && (
        <p className="text-xs text-[var(--text-secondary)]">
          Leave the name blank to target all secrets or variables that match the selected scope and optional repository filter.
        </p>
      )}
      <div className="grid gap-3 md:grid-cols-[0.42fr_1fr]">
        <label className="flex flex-col gap-1 text-sm">
          <span>Effect</span>
          <select
            className="pipelines-input"
            value={parsedAction.effect}
            onChange={e => onChange({ name: policy.name, obj: policy.obj, act: formatAAAActionValue(e.target.value as AAAEffect, parsedAction.action) })}
          >
            <option value="allow">Allow</option>
            <option value="deny">Deny</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Action</span>
          <select
            className="pipelines-input"
            value={selectedAction}
            onChange={e => {
              const value = e.target.value;
              if (value === AAA_CUSTOM_VALUE) {
                onChange({
                  name: policy.name,
                  obj: policy.obj,
                  act: formatAAAActionValue(parsedAction.effect, selectedAction === AAA_CUSTOM_VALUE ? parsedAction.action : ''),
                });
                return;
              }
              onChange({ name: policy.name, obj: policy.obj, act: formatAAAActionValue(parsedAction.effect, value) });
            }}
          >
            {actionOptions.map(group => (
              <optgroup key={`action-${group.label}`} label={group.label}>
                {group.options.map(option => (
                  <option key={`action-${group.label}-${option.value}`} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </optgroup>
            ))}
            <option value={AAA_CUSTOM_VALUE}>Custom action…</option>
          </select>
          {selectedAction === AAA_CUSTOM_VALUE && (
            <input
              className="pipelines-input"
              value={parsedAction.action}
              onChange={e => onChange({ name: policy.name, obj: policy.obj, act: formatAAAActionValue(parsedAction.effect, e.target.value) })}
              placeholder={customAAAActionPlaceholder(normalizedResource)}
              required
            />
          )}
          <p className="text-xs text-[var(--text-secondary)]">
            Actions are filtered by the selected resource. The dropdown shows simple verbs, while custom mode accepts the full AAA action value.
          </p>
        </label>
      </div>
      <p className="text-xs text-[var(--text-secondary)]">
        Rule preview: <code>{policy.obj || 'resource:*'}</code> → <code>{policy.act || 'action'}</code>
      </p>
    </>
  );
}

function AccessPanel({
  users,
  loading,
  error,
  policies,
  policiesLoading,
  policiesError,
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
  loading: boolean;
  error: string | null;
  policies: RolePermission[];
  policiesLoading: boolean;
  policiesError: string | null;
  newUser: { sub: string; email: string; password: string; roles: string[] };
  newRole: { userId: string; role: string };
  policyTemplates: RolePermission[];
  onChangeUser: (next: { sub: string; email: string; password: string; roles: string[] }) => void;
  onChangeRole: (next: { userId: string; role: string }) => void;
  onCreateUser: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  onAssignRole: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  onReloadUsers: () => void;
  onCreatePermission: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  newPermission: { name: string; obj: string; act: string };
  onChangePermission: (next: { name: string; obj: string; act: string }) => void;
  onDeleteUser: (userId: string) => Promise<void>;
  onDeletePolicy: (policy: RolePermission) => Promise<void>;
  onDeleteRoleDefinition: (role: RoleDefinition) => Promise<void>;
  onSaveRoleDefinition: (input: { role: string; policies: RolePolicyDraft[]; original?: RolePermission[] }) => Promise<void>;
  onEditPolicy: (current: RolePermission, next: { role: string; name: string; obj: string; act: string }) => Promise<void>;
  onUpdateUserRoles: (userId: string, nextRoles: string[], previousRoles: string[]) => Promise<void>;
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
    policies: RolePolicyDraft[];
    original?: RolePermission[];
  } | null>(null);
  const [policyEditor, setPolicyEditor] = useState<{
    original: RolePermission;
    role: string;
    name: string;
    obj: string;
    act: string;
  } | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{ message: string; onConfirm: () => Promise<void> | void } | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [userAccessEditor, setUserAccessEditor] = useState<{
    user: UserSummary;
    entries: string[];
    original: string[];
    email: string;
    status: string;
    password: string;
  } | null>(null);
  const [savingRoleEditor, setSavingRoleEditor] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [savingUserAccess, setSavingUserAccess] = useState(false);
  const [resourceCatalog, setResourceCatalog] = useState<AAAResourceCatalog>({
    folderOptions: [],
    pipelineOptions: [],
    triggerOptions: [],
    repositoryOptions: [],
    secretScopeOptions: [],
    variableScopeOptions: [],
  });
  const policyOptions = useMemo(() => {
    const seen = new Set<string>();
    return policyTemplates
      .filter(policy => {
        const key = `${policy.obj}::${policy.act}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .map(policy => ({ key: `${policy.obj}::${policy.act}`, obj: policy.obj, act: policy.act, name: policy.name, label: policyLabel(policy) }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [policyTemplates]);

  const roleAssignments = useMemo(
    () =>
      users
        .flatMap(user =>
          (user.roles || []).map(role => ({
            id: `${user.id}-${role.role}`,
            role: role.role,
            user: user.sub,
            userId: user.id,
            email: user.email,
            status: user.status,
          }))
        )
        .sort((a, b) => a.role.localeCompare(b.role) || a.user.localeCompare(b.user)),
    [users]
  );

  const roleDefinitions = useMemo(() => {
    const map = new Map<string, RoleDefinition>();
    policies.forEach(policy => {
      if (!map.has(policy.role)) {
        map.set(policy.role, { id: policy.role, role: policy.role, policies: [] });
      }
      map.get(policy.role)?.policies.push(policy);
    });
    return Array.from(map.values()).sort((a, b) => a.role.localeCompare(b.role));
  }, [policies]);

  const allRoleOptions = useMemo(() => {
    const set = new Set<string>();
    roleDefinitions.forEach(role => set.add(role.role));
    set.add(DEFAULT_ADMIN_ROLE);
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [roleDefinitions]);

  const roleUserMap = useMemo(() => {
    const map = new Map<string, { user: string; userId: string; email: string }[]>();
    roleAssignments.forEach(item => {
      const existing = map.get(item.role) || [];
      existing.push({ user: item.user, userId: item.userId, email: item.email });
      map.set(item.role, existing);
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
    if (isDefaultAdmin(role.role)) return;
    setRoleEditor({
      mode: 'edit',
      role: role.role,
      policies: role.policies.map(p => ({ name: p.name || policyLabel(p), obj: p.obj, act: p.act })),
      original: role.policies,
    });
  };

  const openPolicyEditModal = (policy: RolePermission) => {
    if (isDefaultAdmin(policy.role)) return;
    setPolicyEditor({
      original: policy,
      role: policy.role,
      name: policy.name || policyLabel(policy),
      obj: policy.obj,
      act: policy.act,
    });
  };

  const openUserAccessModal = (user: UserSummary) => {
    const entries = (user.roles || []).map(role => role.role);
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
      const match = policyOptions.find(p => p.key === key);
      if (!match) return prev;
      const already = prev.policies.some(p => p.obj === match.obj && p.act === match.act);
      if (already) return prev;
      return { ...prev, policies: [...prev.policies, { name: match.name || match.label, obj: match.obj, act: match.act }] };
    });
  };

  const handleSaveRoleEditor = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!roleEditor) return;
    if (isDefaultAdmin(roleEditor.role)) return;
    setSavingRoleEditor(true);
    try {
      await onSaveRoleDefinition({
        role: roleEditor.role,
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
    const deduped = userAccessEditor.entries
      .map(role => role.trim())
      .filter(Boolean)
      .filter((role, index, roles) => roles.indexOf(role) === index);
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
      if (prev.entries.some(entry => assignmentKey(entry) === assignmentKey(roleName))) return prev;
      return { ...prev, entries: [...prev.entries, roleName] };
    });
    setNextAccessRole('');
  };

  const removeUserAccessEntry = (index: number) => {
    setUserAccessEditor(prev => {
      if (!prev) return prev;
      const entry = prev.entries[index];
      const protectedRole = entry && prev.user && isDefaultAdmin(entry) && prev.user.sub === 'admin';
      if (protectedRole) return prev;
      const next = prev.entries.filter((_, i) => i !== index);
      return { ...prev, entries: next };
    });
  };

  const updateNewUserRoleEntry = (index: number, value: string) => {
    const next = [...(newUser.roles || [])];
    next[index] = value;
    onChangeUser({ ...newUser, roles: next });
  };

  const removeNewUserRoleEntry = (index: number) => {
    const next = (newUser.roles || []).filter((_, i) => i !== index);
    onChangeUser({ ...newUser, roles: next });
  };

  const appendUserRoleFromPicker = () => {
    const roleName = nextUserRole.trim();
    if (!roleName) return;
    const existing = (newUser.roles || []).some(entry => entry.trim().toLowerCase() === roleName.toLowerCase());
    if (existing) {
      setNextUserRole('');
      return;
    }
    onChangeUser({ ...newUser, roles: [...(newUser.roles || []), roleName] });
    setNextUserRole('');
  };

  const statusKey = (value: string) => {
    const key = (value || '').toLowerCase();
    if (key.includes('active')) return 'ok';
    if (key.includes('pending')) return 'warn';
    if (key.includes('blocked') || key.includes('disabled')) return 'danger';
    return 'muted';
  };

  const policyCount = policyTemplates.length;
  const [nextPolicyKey, setNextPolicyKey] = useState('');
  const [nextUserRole, setNextUserRole] = useState('');
  const [nextAccessRole, setNextAccessRole] = useState('');

  useEffect(() => {
    let cancelled = false;

    const readJson = async <T,>(path: string): Promise<T> => {
      const response = await fetch(buildApiUrl(path));
      if (!response.ok) {
        throw new Error(`Request failed (${response.status})`);
      }
      return response.json() as Promise<T>;
    };

    void (async () => {
      const [groupsResult, pipelinesResult, triggersResult, secretScopesResult, variableScopesResult] = await Promise.allSettled([
        readJson<ResourceGroup[]>('/v1/groups'),
        readJson<string[]>('/v1/pipelines'),
        readJson<string[]>('/v1/overrides'),
        readJson<AAASecretScopeSummary[]>('/v1/secrets/scopes'),
        readJson<AAAScopeResponse[]>('/v1/variables/scopes'),
      ]);

      if (cancelled) return;

      const groups = groupsResult.status === 'fulfilled' && Array.isArray(groupsResult.value) ? groupsResult.value : [];
      const pipelines = pipelinesResult.status === 'fulfilled' && Array.isArray(pipelinesResult.value) ? pipelinesResult.value : [];
      const triggers = triggersResult.status === 'fulfilled' && Array.isArray(triggersResult.value) ? triggersResult.value : [];
      const secretScopes =
        secretScopesResult.status === 'fulfilled' && Array.isArray(secretScopesResult.value)
          ? secretScopesResult.value.map(entry => (typeof entry?.scope === 'string' ? entry.scope : ''))
          : [];
      const variableScopes =
        variableScopesResult.status === 'fulfilled' && Array.isArray(variableScopesResult.value)
          ? variableScopesResult.value.map(entry => (typeof entry?.scope === 'string' ? entry.scope : ''))
          : [];

      if (groupsResult.status === 'rejected') {
        console.error('Failed to load AAA folders', groupsResult.reason);
      }
      if (pipelinesResult.status === 'rejected') {
        console.error('Failed to load AAA pipelines', pipelinesResult.reason);
      }
      if (triggersResult.status === 'rejected') {
        console.error('Failed to load AAA triggers', triggersResult.reason);
      }
      if (secretScopesResult.status === 'rejected') {
        console.error('Failed to load AAA secret scopes', secretScopesResult.reason);
      }
      if (variableScopesResult.status === 'rejected') {
        console.error('Failed to load AAA variable scopes', variableScopesResult.reason);
      }

      const triggerOptions = buildAAAStringOptions(triggers);
      setResourceCatalog({
        folderOptions: buildAAAGroupOptions(groups),
        pipelineOptions: buildAAAStringOptions(pipelines),
        triggerOptions,
        repositoryOptions: triggerOptions,
        secretScopeOptions: buildAAAScopeOptions(secretScopes),
        variableScopeOptions: buildAAAScopeOptions(variableScopes),
      });
    })();

    return () => {
      cancelled = true;
    };
  }, []);

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

  const availablePoliciesForRoleEditor = roleEditor ? policyOptions : [];

  return (
    <div className="access-layout space-y-5 pb-24">
      <div className="glass-card p-5 border border-[var(--border-primary)] rounded-2xl space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <p className="text-xs uppercase tracking-[0.12em] text-[var(--text-secondary)]">Access control</p>
            <h3 className="text-xl font-semibold text-[var(--text-primary)]">Users, roles, policies</h3>
            <p className="text-sm text-[var(--text-secondary)]">Define AAA resource rules, bundle them into roles, and grant users access.</p>
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
                            userRoles.slice(0, 3).map(role => (
                              <span key={`${user.id}-${role.role}`} className="access-chip access-chip--muted">
                                {role.role}
                              </span>
                            ))
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
                <div className="grid grid-cols-[1.5fr,1.4fr,1fr,auto] text-[11px] uppercase tracking-[0.08em] text-[var(--text-tertiary)] px-4 py-3 bg-[var(--bg-tertiary)] border-b border-[var(--border-primary)]">
                  <span>Role</span>
                  <span>Policies</span>
                  <span>Users</span>
                  <span className="text-right">Manage</span>
                </div>
                <div className="divide-y divide-[var(--border-primary)]">
                  {roleDefinitions.map(role => {
                    const assignedUsers = roleUserMap.get(role.id) || [];
                    return (
                      <div
                        key={role.id}
                        className="grid grid-cols-[1.5fr,1.4fr,1fr,auto] items-center px-4 py-3 gap-3 text-sm text-[var(--text-primary)] access-row"
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
                        <div className="flex justify-end items-center gap-3">
                          <button
                            type="button"
                            className="access-inline-btn"
                            title={isDefaultAdmin(role.role) ? 'Protected role' : 'Edit role policies'}
                            onClick={() => openEditRoleEditor(role)}
                            disabled={isDefaultAdmin(role.role)}
                          >
                            <EditIcon />
                          </button>
                          <button
                            type="button"
                            className="access-inline-btn access-inline-btn--danger"
                            title={isDefaultAdmin(role.role) ? 'Protected role' : 'Delete role'}
                            onClick={() => confirmDeleteRoleDefinition(role)}
                            disabled={isDefaultAdmin(role.role)}
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
                <div className="grid grid-cols-[1.4fr,1fr,1.1fr,1fr,auto] text-[11px] uppercase tracking-[0.08em] text-[var(--text-tertiary)] px-4 py-3 bg-[var(--bg-tertiary)] border-b border-[var(--border-primary)]">
                  <span>Policy</span>
                  <span>Role</span>
                  <span>Resource</span>
                  <span>Action</span>
                  <span className="text-right">Manage</span>
                </div>
                <div className="divide-y divide-[var(--border-primary)]">
                  {policyTemplates.map(policy => (
                    <div
                      key={`${policy.role}-${policy.obj}-${policy.act}`}
                      className="grid grid-cols-[1.4fr,1fr,1.1fr,1fr,auto] items-center px-4 py-3 gap-3 text-sm text-[var(--text-primary)] access-policy-row"
                    >
                      <div className="min-w-0 font-semibold truncate">{policyLabel(policy)}</div>
                      <span className="access-policy-text access-policy-text--muted truncate" title={policy.role}>
                        {policy.role}
                      </span>
                      <span className="access-policy-text truncate" title={policy.obj}>
                        {policy.obj}
                      </span>
                      <span className="access-policy-text access-policy-text--muted truncate" title={policy.act}>
                        {policy.act}
                      </span>
                      <div className="flex justify-end items-center gap-3">
                        <button
                          type="button"
                          className="access-inline-btn"
                          title={isDefaultAdmin(policy.role) ? 'Protected policy' : 'Edit policy'}
                          onClick={() => openPolicyEditModal(policy)}
                          disabled={isDefaultAdmin(policy.role)}
                        >
                          <EditIcon />
                        </button>
                        <button
                          type="button"
                          className="access-inline-btn access-inline-btn--danger"
                          title={isDefaultAdmin(policy.role) ? 'Protected policy' : 'Delete policy'}
                          onClick={() => confirmDeletePolicy(policy)}
                          disabled={isDefaultAdmin(policy.role)}
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
            <div className="access-minimal-section">
              <div className="access-minimal-section__header">
                <p className="text-sm font-medium text-[var(--text-primary)]">Policies</p>
                <div className="flex gap-2 flex-wrap items-center">
                  <span className="text-[11px] text-[var(--text-secondary)]">Current rules</span>
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
          subtitle="Update the AAA rule for this role"
          onClose={() => setPolicyEditor(null)}
          icon={<SparkIcon />}
        >
          <form className="space-y-3" onSubmit={handleSavePolicyEdit}>
            <AAAPolicyRuleFields
              policy={policyEditor}
              onChange={next => setPolicyEditor(prev => (prev ? { ...prev, ...next } : prev))}
              resourceCatalog={resourceCatalog}
            />
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
          subtitle="Add or remove roles"
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
                const protectedAdmin = isDefaultAdmin(entry) && userAccessEditor.user.sub === 'admin';
                const label = protectedAdmin ? 'Protected admin role' : 'Remove assignment';
                return (
                  <div key={`user-role-${idx}`} className="access-minimal-row justify-between">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-[var(--text-primary)]">{entry || 'Role'}</span>
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
                <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Select a role'}</option>
                {allRoleOptions.map(role => (
                  <option key={`access-role-${role}`} value={role}>
                    {role}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="glass-button-subtle flex items-center justify-center gap-1"
                onClick={addUserAccessEntry}
                disabled={!nextAccessRole || allRoleOptions.length === 0}
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
            </div>
            <div className="access-minimal-section">
              <div className="access-minimal-section__header">
                <p className="text-sm font-medium text-[var(--text-primary)]">Roles</p>
              </div>
              {allRoleOptions.length === 0 && (
                <p className="text-[11px] text-[var(--text-secondary)]">No roles available yet.</p>
              )}
              {newUser.roles.length === 0 && (
                <p className="text-[11px] text-[var(--text-secondary)]">Pick at least one role to create this user.</p>
              )}
              <div className="space-y-2">
                {newUser.roles.map((entry, idx) => (
                  <div key={`new-user-role-${idx}`} className="access-minimal-row">
                    <select
                      className="pipelines-input flex-1"
                      value={entry}
                      onChange={e => updateNewUserRoleEntry(idx, e.target.value)}
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
                    disabled={allRoleOptions.length === 0}
                  >
                    <option value="">
                      {allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}
                    </option>
                    {allRoleOptions.map(role => (
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
                    disabled={allRoleOptions.length === 0}
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
          subtitle="Map a user to a role"
          onClose={() => setShowRoleModal(false)}
          icon={<ShieldIcon />}
        >
          <form className="space-y-3" onSubmit={onAssignRole}>
            <label className="flex flex-col gap-1 text-sm">
              <span>User</span>
              <select
                className="pipelines-input"
                value={newRole.userId}
                onChange={e => onChangeRole({ ...newRole, userId: e.target.value })}
                required
                disabled={users.length === 0}
              >
                <option value="">{users.length === 0 ? 'No users available' : 'Select a user'}</option>
                {users.map(user => (
                  <option key={`assign-user-${user.id}`} value={user.id}>
                    {user.sub}
                    {user.email ? ` • ${user.email}` : ''}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span>Role</span>
              <select
                className="pipelines-input"
                value={newRole.role}
                onChange={e => onChangeRole({ ...newRole, role: e.target.value })}
                required
                disabled={allRoleOptions.length === 0}
              >
                <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Select a role'}</option>
                {allRoleOptions.map(role => (
                  <option key={`assign-role-${role}`} value={role}>
                    {role}
                  </option>
                ))}
              </select>
            </label>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button type="button" className="glass-button-ghost" onClick={() => setShowRoleModal(false)}>
                  Cancel
                </button>
                <button type="submit" className="glass-button-primary" disabled={users.length === 0 || allRoleOptions.length === 0}>
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
          subtitle="Add a reusable AAA rule"
          onClose={() => setShowPolicyModal(false)}
          icon={<SparkIcon />}
        >
          <form className="space-y-3" onSubmit={onCreatePermission}>
            <AAAPolicyRuleFields
              policy={newPermission}
              onChange={onChangePermission}
              resourceCatalog={resourceCatalog}
            />
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
  canManageConfig,
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
  canManageConfig: boolean;
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
              <button className="glass-button-subtle" type="button" onClick={() => void onTriggerSync()} disabled={!canManageConfig}>
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
                  disabled={!canManageConfig || configLoading || saving}
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
                disabled={!canManageConfig || configLoading || saving}
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
                disabled={!canManageConfig || configLoading || saving}
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
                  disabled={!canManageConfig || configLoading || saving}
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
                  disabled={!canManageConfig || configLoading || saving}
                />
              </label>
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                id="system-auto-remove"
                type="checkbox"
                checked={config.auto_removal_agent_container}
                onChange={handleChange('auto_removal_agent_container')}
                disabled={!canManageConfig || configLoading || saving}
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
                disabled={!canManageConfig || configLoading || saving}
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
                disabled={!canManageConfig || configLoading || saving}
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
                disabled={!canManageConfig || configLoading || saving}
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
        <button className="glass-button-primary" type="button" onClick={() => void onSave()} disabled={!canManageConfig || configLoading || saving}>
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
  canManageDispatcher,
}: {
  loading: boolean;
  error: string | null;
  status: { queuedJobs: number; runners: Runner[]; routing: Record<string, string[]>; fetchedAt: number } | null;
  pendingActions: Set<string>;
  onRefresh: () => void;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
  canManageDispatcher: boolean;
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
              canManageDispatcher={canManageDispatcher}
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
  canManageDispatcher,
}: {
  nowMs: number;
  runner: Runner;
  pendingActions: Set<string>;
  onToggleDispatch: (runner: Runner) => Promise<void>;
  canManageDispatcher: boolean;
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
            disabled={!canManageDispatcher || pending}
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
