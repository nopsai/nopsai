import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent, type Dispatch, type FormEvent, type ReactNode, type SetStateAction } from 'react';
import { Copy, Edit3, Plus, RefreshCw, Search, Trash2, X } from 'lucide-react';
import { buildApiUrl } from '../lib/api';
import SetupWizard from './Setup';

type ConfigFormState = {
  agent_image: string;
  docker_network_name: string;
  default_pipeline_timeout: string;
  llm_agent_timeout: string;
  auto_removal_agent_container: boolean;
  agent_nopsai_api_url: string;
  git_bot_nopsai_api_url: string;
  nopsai_git_bot_api_url: string;
};

type ConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  managed_by_config_repo?: boolean;
  config_source_path?: string;
  last_sync_status: string;
  last_sync_message?: string;
  last_sync_started_at?: string;
  last_sync_completed_at?: string;
  last_sync_commit_sha?: string;
};

type ConfigRepositoryFormState = {
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
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

type RunnerComposeTemplate = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  dispatcherAddress: string;
  compose: string;
  command: string;
  bootstrapCommand: string;
  expiresAt: string;
  warnings: string[];
};

const initialConfig: ConfigFormState = {
  agent_image: '',
  docker_network_name: '',
  default_pipeline_timeout: '',
  llm_agent_timeout: '',
  auto_removal_agent_container: true,
  agent_nopsai_api_url: '',
  git_bot_nopsai_api_url: '',
  nopsai_git_bot_api_url: '',
};

const emptyConfigRepositoryForm: ConfigRepositoryFormState = {
  repo_url: '',
  branch: 'main',
  base_path: '',
  enabled: true,
};

const POLL_INTERVAL_MS = 5000;
const STALE_THRESHOLD_MS = 30_000;
const MAX_VISIBLE_ACTIVE_RUNS = 3;
const RUNNER_DEPLOYMENT_GUIDE_QUERY = 'guide';
const RUNNER_DEPLOYMENT_GUIDE_VALUE = 'runner';
const RUNNER_DEPLOYMENT_GUIDE_ID = 'dispatcher-runner-deployment-guide';
const POLICY_TEMPLATE_ROLE = '__policy_template__';
const DEFAULT_ADMIN_ROLE = 'nopsai-admin';
const DEFAULT_ADMIN_POLICY_OBJ = '*:*';
const DEFAULT_ADMIN_POLICY_ACT = '*';
const GENERAL_ACCESS_SCOPE = '__general__';
const BASIC_ROLE_VIEWER = 'viewer';
const BASIC_ROLE_DEVELOPER = 'developer';
const BASIC_ROLE_OWNER = 'owner';
const BASIC_ROLE_ADMIN = 'admin';
const PROTECTED_ACCESS_ROLES = new Set([DEFAULT_ADMIN_ROLE, BASIC_ROLE_VIEWER, BASIC_ROLE_DEVELOPER, BASIC_ROLE_OWNER, BASIC_ROLE_ADMIN]);
const ACCESS_UI_BUILD_ID = 'access-protected-default-roles-2026-05-11';

function scrollRunnerDeploymentGuide() {
  window.setTimeout(() => {
    document.getElementById(RUNNER_DEPLOYMENT_GUIDE_ID)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, 0);
}

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

type AccessGrantRecord = {
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

type EditableAccessGrant = {
  localID: string;
  id?: string;
  role: string;
  resourceType: string;
  resourceID: string;
  inherit: boolean;
  grantedBy?: string;
};

type BasicGrantInput = {
  role: string;
  resourceType: string;
  resourceID: string;
  inherit?: boolean;
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
  canViewSetup: boolean;
  canManageSetup: boolean;
  canViewRuntimeConfig: boolean;
  canManageRuntimeConfig: boolean;
  canViewLLMProfiles: boolean;
  canManageLLMProfiles: boolean;
  canViewMCP: boolean;
  canManageMCP: boolean;
  canViewGlobalConfigRepo: boolean;
  canManageGlobalConfigRepo: boolean;
  canViewDispatcher: boolean;
  canManageDispatcher: boolean;
  canViewAccess: boolean;
};

function SystemPage({ permissions }: { permissions: SystemPagePermissions }) {
  const params = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab =
    params.tab === 'setup'
      ? 'setup'
      : params.tab === 'dispatcher'
      ? 'dispatcher'
      : params.tab === 'access'
        ? 'access'
        : params.tab === 'llm-profiles'
          ? 'llm-profiles'
          : params.tab === 'mcp'
            ? 'mcp'
          : 'config';
  const allowedTabs = useMemo(() => {
    const tabs: Array<'config' | 'setup' | 'llm-profiles' | 'mcp' | 'dispatcher' | 'access'> = [];
    if (permissions.canViewConfig) tabs.push('config');
    if (permissions.canViewSetup) tabs.push('setup');
    if (permissions.canViewLLMProfiles) tabs.push('llm-profiles');
    if (permissions.canViewMCP) tabs.push('mcp');
    if (permissions.canViewDispatcher) tabs.push('dispatcher');
    if (permissions.canViewAccess) tabs.push('access');
    return tabs;
  }, [permissions.canViewAccess, permissions.canViewConfig, permissions.canViewDispatcher, permissions.canViewLLMProfiles, permissions.canViewMCP, permissions.canViewSetup]);
  const visibleTab = allowedTabs.includes(activeTab) ? activeTab : allowedTabs[0] ?? activeTab;

  const isMountedRef = useRef(true);

  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const [config, setConfig] = useState<ConfigFormState>(initialConfig);
  const [envFilePath, setEnvFilePath] = useState('');
  const [configLoading, setConfigLoading] = useState(true);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [globalConfigRepo, setGlobalConfigRepo] = useState<ConfigRepository | null>(null);
  const [globalConfigRepoForm, setGlobalConfigRepoForm] = useState<ConfigRepositoryFormState>(emptyConfigRepositoryForm);
  const [globalConfigRepoLoading, setGlobalConfigRepoLoading] = useState(false);
  const [globalConfigRepoSaving, setGlobalConfigRepoSaving] = useState(false);
  const [globalConfigRepoSyncing, setGlobalConfigRepoSyncing] = useState(false);
  const [globalConfigRepoError, setGlobalConfigRepoError] = useState<string | null>(null);

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
  const [accessGrants, setAccessGrants] = useState<AccessGrantRecord[]>([]);
  const [accessGrantsLoading, setAccessGrantsLoading] = useState(false);
  const [accessGrantsError, setAccessGrantsError] = useState<string | null>(null);
  const [policies, setPolicies] = useState<RolePermission[]>([]);
  const [policyTemplates, setPolicyTemplates] = useState<RolePermission[]>([]);
  const [policiesLoading, setPoliciesLoading] = useState(false);
  const [policiesError, setPoliciesError] = useState<string | null>(null);
  const [newUser, setNewUser] = useState({ sub: '', email: '', password: '', roles: [] as string[] });
  const [newPermission, setNewPermission] = useState({ name: '', obj: 'pipeline:*', act: 'pipeline.read' });

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
  }, []);

  const normalizeConfigRepository = useCallback((payload: unknown): ConfigRepository | null => {
    const record = asRecord(payload);
    if (!record) return null;
    const id = normalizeNumber(record.id);
    return {
      id,
      scope_type: readString(record.scope_type),
      scope_id: readString(record.scope_id),
      repo_url: readString(record.repo_url),
      branch: readString(record.branch).trim() || 'main',
      base_path: readString(record.base_path),
      enabled: Boolean(record.enabled),
      managed_by_config_repo: Boolean(record.managed_by_config_repo),
      config_source_path: readOptionalString(record.config_source_path),
      last_sync_status: readString(record.last_sync_status),
      last_sync_message: readOptionalString(record.last_sync_message),
      last_sync_started_at: readOptionalString(record.last_sync_started_at),
      last_sync_completed_at: readOptionalString(record.last_sync_completed_at),
      last_sync_commit_sha: readOptionalString(record.last_sync_commit_sha),
    };
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

  const loadGlobalConfigRepository = useCallback(
    async (opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setGlobalConfigRepoLoading(true);
        setGlobalConfigRepoError(null);
      }
      try {
        const response = await fetch(buildApiUrl('/v1/system/config-repo'), { cache: 'no-store' });
        if (response.status === 404) {
          setGlobalConfigRepo(null);
          setGlobalConfigRepoForm(emptyConfigRepositoryForm);
          return;
        }
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Unable to load global config repository (${response.status})`);
        }
        const repo = normalizeConfigRepository(await response.json());
        setGlobalConfigRepo(repo);
        setGlobalConfigRepoForm(repo ? {
          repo_url: repo.repo_url,
          branch: repo.branch || 'main',
          base_path: repo.base_path || '',
          enabled: repo.enabled,
        } : emptyConfigRepositoryForm);
      } catch (error) {
        console.error('Failed to load global config repository', error);
        if (!isMountedRef.current) return;
        setGlobalConfigRepoError(error instanceof Error ? error.message : 'Unable to load global config repository');
      } finally {
        if (isMountedRef.current && !opts?.quiet) {
          setGlobalConfigRepoLoading(false);
        }
      }
    },
    [normalizeConfigRepository]
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
  }, [fetchJson]);

  const loadAccessGrants = useCallback(async () => {
    setAccessGrantsLoading(true);
    setAccessGrantsError(null);
    try {
      const payload = await fetchJson('/v1/access/grants');
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
  }, [fetchJson]);

  const loadPolicies = useCallback(async () => {
    setPoliciesLoading(true);
    setPoliciesError(null);
    try {
      const payload = await fetchJson('/v1/admin/roles');
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
    }
    setPoliciesLoading(false);
  }, [fetchJson]);

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
        const created = (await fetchJson('/v1/admin/users', {
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
            const list = await fetchJson('/v1/admin/users');
            const records = normalizeListPayload(list, ['users', 'items', 'data', 'records', 'results']);
            if (records) {
              const match = (records as UserSummary[]).find(u => u.sub === sub || u.email === email);
              userId = match?.id;
            }
          } catch {
            // ignore lookup failure, will fail on assignment below if missing
          }
        }

        if (!userId) {
          addToast('User created but ID not found; roles not assigned.', 'error');
          await loadUsers();
          return false;
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

        for (const grant of basicGrants) {
          try {
            await fetchJson('/v1/access/grants', {
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
        setNewUser({ sub: '', email: '', password: '', roles: [] });
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
    [addToast, fetchJson, loadAccessGrants, loadUsers, newUser.email, newUser.password, newUser.roles, newUser.sub]
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

  const createAccessGrant = useCallback(
    async (input: { userID: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => {
      const role = input.role.trim().toLowerCase();
      const resourceType = input.resourceType.trim();
      const resourceID = input.resourceID.trim();
      await fetchJson('/v1/access/grants', {
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
    [addToast, fetchJson, loadAccessGrants]
  );

  const deleteAccessGrant = useCallback(
    async (grantID: string) => {
      await fetchJson(`/v1/access/grants/${encodeURIComponent(grantID)}`, { method: 'DELETE' });
      addToast('Basic role removed', 'success');
      await loadAccessGrants();
    },
    [addToast, fetchJson, loadAccessGrants]
  );

  const deletePolicy = useCallback(
    async (policy: RolePermission) => {
      if (isProtectedAccessRole(policy.role)) {
        addToast('Default role policies cannot be deleted.', 'error');
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
      if (isProtectedAccessRole(role.role)) {
        addToast('Default roles cannot be deleted.', 'error');
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
    [addToast, fetchJson, loadPolicies, policyTemplates]
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
        await fetchJson('/v1/admin/roles', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            role: current.role,
            obj: current.obj,
            act: current.act,
          }),
        });
        for (const policy of linkedRolePolicies) {
          await fetchJson('/v1/admin/roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              role: policy.role,
              obj: policy.obj,
              act: policy.act,
            }),
          });
        }
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
        for (const policy of linkedRolePolicies) {
          if (nextAlreadyExistsInRole(policy)) continue;
          await fetchJson('/v1/admin/roles', {
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
    [addToast, fetchJson, loadPolicies, policies]
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

  const saveGlobalConfigRepository = useCallback(async () => {
    if (globalConfigRepoSaving || !permissions.canManageGlobalConfigRepo) return;
    const repoURL = globalConfigRepoForm.repo_url.trim();
    if (!repoURL) {
      setGlobalConfigRepoError('Repository URL is required.');
      return;
    }
    setGlobalConfigRepoSaving(true);
    setGlobalConfigRepoError(null);
    try {
      const payload = await fetchJson('/v1/system/config-repo', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          repo_url: repoURL,
          branch: globalConfigRepoForm.branch.trim() || 'main',
          base_path: globalConfigRepoForm.base_path.trim(),
          enabled: Boolean(globalConfigRepoForm.enabled),
        }),
      });
      const repo = normalizeConfigRepository(payload);
      setGlobalConfigRepo(repo);
      setGlobalConfigRepoForm(repo ? {
        repo_url: repo.repo_url,
        branch: repo.branch || 'main',
        base_path: repo.base_path || '',
        enabled: repo.enabled,
      } : emptyConfigRepositoryForm);
      addToast('Global config repository saved.', 'success');
    } catch (error) {
      console.error('Failed to save global config repository', error);
      const message = error instanceof Error ? error.message : 'Unable to save global config repository';
      setGlobalConfigRepoError(message);
      addToast('Failed to save global config repository.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoSaving(false);
      }
    }
  }, [addToast, fetchJson, globalConfigRepoForm, globalConfigRepoSaving, normalizeConfigRepository, permissions.canManageGlobalConfigRepo]);

  const deleteGlobalConfigRepository = useCallback(async () => {
    if (globalConfigRepoSaving || !permissions.canManageGlobalConfigRepo || !globalConfigRepo) return;
    if (!window.confirm('Remove the global config repository? Synced resources will remain available.')) return;
    setGlobalConfigRepoSaving(true);
    setGlobalConfigRepoError(null);
    try {
      await fetchJson('/v1/system/config-repo', { method: 'DELETE' });
      setGlobalConfigRepo(null);
      setGlobalConfigRepoForm(emptyConfigRepositoryForm);
      addToast('Global config repository removed.', 'success');
    } catch (error) {
      console.error('Failed to remove global config repository', error);
      const message = error instanceof Error ? error.message : 'Unable to remove global config repository';
      setGlobalConfigRepoError(message);
      addToast('Failed to remove global config repository.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoSaving(false);
      }
    }
  }, [addToast, fetchJson, globalConfigRepo, globalConfigRepoSaving, permissions.canManageGlobalConfigRepo]);

  const syncGlobalConfigRepository = useCallback(async () => {
    if (!permissions.canManageGlobalConfigRepo || globalConfigRepoSyncing || globalConfigRepo?.last_sync_status === 'running') return;
    setGlobalConfigRepoSyncing(true);
    setGlobalConfigRepoError(null);
    try {
      await fetchJson('/v1/system/config-repo/sync', { method: 'POST' });
      setGlobalConfigRepo(prev => prev ? {
        ...prev,
        last_sync_status: 'running',
        last_sync_message: 'Configuration synchronization started.',
        last_sync_started_at: new Date().toISOString(),
        last_sync_completed_at: undefined,
      } : prev);
      window.setTimeout(() => {
        void loadGlobalConfigRepository({ quiet: true });
      }, 1000);
      addToast('Global config repository sync started.', 'success');
    } catch (error) {
      console.error('Failed to start global config repository sync', error);
      const message = error instanceof Error ? error.message : 'Unable to start global config repository sync';
      setGlobalConfigRepoError(message);
      addToast('Failed to start global config repository sync.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoSyncing(false);
      }
    }
  }, [addToast, fetchJson, globalConfigRepo?.last_sync_status, globalConfigRepoSyncing, loadGlobalConfigRepository, permissions.canManageGlobalConfigRepo]);

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
    if (permissions.canViewRuntimeConfig) {
      void loadSystemConfig();
    } else {
      setConfigLoading(false);
    }
    if (permissions.canViewGlobalConfigRepo) {
      void loadGlobalConfigRepository();
    }
  }, [loadGlobalConfigRepository, loadSystemConfig, permissions.canViewConfig, permissions.canViewGlobalConfigRepo, permissions.canViewRuntimeConfig, visibleTab]);

  useEffect(() => {
    if (permissions.canViewDispatcher && visibleTab === 'dispatcher') {
      void loadDispatcherStatus();
    }
  }, [loadDispatcherStatus, permissions.canViewDispatcher, visibleTab]);

  useEffect(() => {
    if (visibleTab !== 'dispatcher') return;
    const search = new URLSearchParams(location.search);
    if (search.get(RUNNER_DEPLOYMENT_GUIDE_QUERY) !== RUNNER_DEPLOYMENT_GUIDE_VALUE) return;
    scrollRunnerDeploymentGuide();
  }, [location.search, visibleTab]);

  useEffect(() => {
    if (permissions.canViewAccess && visibleTab === 'access') {
      void loadUsers();
      void loadAccessGrants();
      void loadPolicies();
    }
  }, [loadAccessGrants, loadPolicies, loadUsers, permissions.canViewAccess, visibleTab]);

  useEffect(() => {
    const handle = window.setInterval(() => {
      if (permissions.canViewDispatcher && visibleTab === 'dispatcher') {
        void loadDispatcherStatus({ quiet: true });
      }
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [loadDispatcherStatus, permissions.canViewDispatcher, visibleTab]);

  useEffect(() => {
    if (!permissions.canViewGlobalConfigRepo || visibleTab !== 'config' || globalConfigRepo?.last_sync_status !== 'running') return undefined;
    const handle = window.setInterval(() => {
      void loadGlobalConfigRepository({ quiet: true });
    }, 3000);
    return () => window.clearInterval(handle);
  }, [globalConfigRepo?.last_sync_status, loadGlobalConfigRepository, permissions.canViewGlobalConfigRepo, visibleTab]);

  return (
    <div data-page="system" className="active p-6 space-y-6">
      {visibleTab === 'config' && (
        <SystemConfig
          config={config}
          envFilePath={envFilePath}
          configError={configError}
          configLoading={configLoading}
          saving={saving}
          globalConfigRepo={globalConfigRepo}
          globalConfigRepoForm={globalConfigRepoForm}
          globalConfigRepoLoading={globalConfigRepoLoading}
          globalConfigRepoSaving={globalConfigRepoSaving}
          globalConfigRepoSyncing={globalConfigRepoSyncing}
          globalConfigRepoError={globalConfigRepoError}
          onChange={setConfig}
          onReload={loadSystemConfig}
          onSave={saveConfig}
          onGlobalConfigRepoChange={setGlobalConfigRepoForm}
          onSaveGlobalConfigRepo={saveGlobalConfigRepository}
          onDeleteGlobalConfigRepo={deleteGlobalConfigRepository}
          onSyncGlobalConfigRepo={syncGlobalConfigRepository}
          canViewRuntimeConfig={permissions.canViewRuntimeConfig}
          canManageRuntimeConfig={permissions.canManageRuntimeConfig}
          canViewGlobalConfigRepo={permissions.canViewGlobalConfigRepo}
          canManageGlobalConfigRepo={permissions.canManageGlobalConfigRepo}
        />
      )}
      {visibleTab === 'llm-profiles' && (
        <LLMProfilesPanel canManage={permissions.canManageLLMProfiles} />
      )}
      {visibleTab === 'setup' && (
        <SetupWizard canManage={permissions.canManageSetup} />
      )}
      {visibleTab === 'mcp' && (
        <MCPPanel canManage={permissions.canManageMCP} />
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
          accessGrants={accessGrants}
          accessGrantsLoading={accessGrantsLoading}
          accessGrantsError={accessGrantsError}
          policies={policies}
          policiesLoading={policiesLoading}
          policiesError={policiesError}
          newUser={newUser}
          policyTemplates={policyTemplates}
          onChangeUser={setNewUser}
          onCreateUser={createUser}
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
          onCreateAccessGrant={createAccessGrant}
          onDeleteAccessGrant={deleteAccessGrant}
          onReloadAccessGrants={loadAccessGrants}
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
const isProtectedAccessRole = (roleName: string) => PROTECTED_ACCESS_ROLES.has((roleName || '').trim().toLowerCase());
const isDefaultAdminUser = (user?: Pick<UserSummary, 'sub'> | null) => (user?.sub || '').trim().toLowerCase() === 'admin';

type AccessPresetID = 'viewer' | 'developer' | 'owner' | 'admin';

const ACCESS_ROLE_PRESETS: Array<{
  id: AccessPresetID;
  label: string;
  description: string;
}> = [
  {
    id: 'viewer',
    label: 'Viewer',
    description: 'Read-only access to groups, pipelines, runs, logs, triggers, and metadata.',
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

const ACCESS_SECTION_CONTENT: Record<
  'users' | 'roles' | 'policies',
  { title: string; description: string; searchPlaceholder: string; resultsLabel: string }
> = {
  users: {
    title: 'People and accounts',
    description: 'See who can sign in, what they can do, and which accounts still need access assigned.',
    searchPlaceholder: 'Search by username, email, or role',
    resultsLabel: 'people',
  },
  roles: {
    title: 'Reusable role bundles',
    description: 'Shape access around simple roles like viewer and developer, then map those bundles to people.',
    searchPlaceholder: 'Search roles, included policies, or assigned users',
    resultsLabel: 'roles',
  },
  policies: {
    title: 'Underlying AAA rules',
    description: 'Low-level resource and action rules that power your friendlier product roles.',
    searchPlaceholder: 'Search policies, resources, actions, or role names',
    resultsLabel: 'policies',
  },
};

const accessPresetIDForRole = (roleName: string): AccessPresetID | null => {
  const normalized = (roleName || '').trim().toLowerCase();
  if (!normalized) return null;
  if (normalized === DEFAULT_ADMIN_ROLE || normalized === 'admin' || normalized.endsWith('-admin')) return 'admin';
  if (normalized === 'owner' || normalized.endsWith('-owner')) return 'owner';
  if (normalized === 'developer' || normalized.endsWith('-developer')) return 'developer';
  if (normalized === 'viewer' || normalized.endsWith('-viewer')) return 'viewer';
  return null;
};

const accessPresetForRole = (roleName: string) => {
  const presetID = accessPresetIDForRole(roleName);
  return presetID ? ACCESS_ROLE_PRESETS.find(preset => preset.id === presetID) ?? null : null;
};

const accessPresetToneClass = (roleName: string) => {
  const presetID = accessPresetIDForRole(roleName);
  return presetID ? `access-chip--tone-${presetID}` : 'access-chip--muted';
};

const normalizeBasicGrantResourceLabel = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  const resourceType = (grant.resourceType || '').trim();
  const resourceID = (grant.resourceID || '').trim().replace(/^\/+|\/+$/g, '');
  if (resourceType === 'platform') return 'Platform';
  if (!resourceID || resourceID === 'general' || resourceID === GENERAL_ACCESS_SCOPE) return 'General';
  return `/${resourceID}`;
};

const basicAccessGrantLabel = (grant: Pick<AccessGrantRecord, 'role' | 'resourceType' | 'resourceID'>) =>
  `${grant.role} • ${normalizeBasicGrantResourceLabel(grant)}`;

const accessGrantResourceSummary = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  if ((grant.resourceType || '').trim() === 'platform') return 'Platform wide';
  const label = normalizeBasicGrantResourceLabel(grant);
  return label === 'General' ? 'General (without group)' : label;
};

const basicAccessGrantDescription = (grant: Pick<AccessGrantRecord, 'role' | 'resourceType' | 'resourceID' | 'grantedBy'>) => {
  const label = accessGrantResourceSummary(grant);
  if ((grant.resourceType || '').trim() === 'platform') {
    return 'This basic role gives platform-wide administrator access.';
  }
  if (label === 'General (without group)') {
    return `This ${grant.role} basic role applies to items that are not inside any group.`;
  }
  return `This ${grant.role} basic role applies to ${label} and anything nested below it.`;
};

const accessGrantSortLabel = (grant: AccessGrantRecord) => `${normalizeBasicGrantResourceLabel(grant)}::${grant.role}`;

const normalizedAccessGrantResourceKey = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  const resourceType = (grant.resourceType || '').trim().toLowerCase();
  const resourceID = (grant.resourceID || '').trim();
  if (resourceType === 'folder') {
    const folderID = resourceID.replace(/^\/+|\/+$/g, '');
    if (!folderID || folderID === 'general' || folderID === GENERAL_ACCESS_SCOPE) {
      return { resourceType, resourceID: GENERAL_ACCESS_SCOPE };
    }
    return { resourceType, resourceID: folderID };
  }
  if (resourceType === 'platform') {
    return { resourceType, resourceID: 'platform' };
  }
  return { resourceType, resourceID };
};

const accessGrantEditKey = (grant: Pick<AccessGrantRecord, 'role' | 'resourceType' | 'resourceID'>) =>
  `${(grant.role || '').trim().toLowerCase()}::${accessGrantTargetKey(grant)}`;

const accessGrantTargetKey = (grant: Pick<AccessGrantRecord, 'resourceType' | 'resourceID'>) => {
  const { resourceType, resourceID } = normalizedAccessGrantResourceKey(grant);
  return `${resourceType}::${resourceID}`;
};

const editableAccessGrantFromRecord = (grant: AccessGrantRecord): EditableAccessGrant => ({
  localID: grant.id,
  id: grant.id,
  role: grant.role,
  resourceType: grant.resourceType,
  resourceID: grant.resourceID,
  inherit: grant.inherit,
  grantedBy: grant.grantedBy,
});

const normalizeBasicGrantInputs = (entries: BasicGrantInput[]): BasicGrantInput[] =>
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

const normalizeEditableBasicGrants = (entries: EditableAccessGrant[]): BasicGrantInput[] =>
  normalizeBasicGrantInputs(
    entries.map(entry => ({
      role: entry.role,
      resourceType: entry.resourceType,
      resourceID: entry.resourceID,
      inherit: entry.inherit,
    }))
  );

const isBasicAccessGrant = (grant: AccessGrantRecord) => {
  const role = (grant.role || '').trim().toLowerCase();
  const resourceType = (grant.resourceType || '').trim();
  return (
    (resourceType === 'folder' || resourceType === 'platform') &&
    (role === BASIC_ROLE_VIEWER || role === BASIC_ROLE_DEVELOPER || role === BASIC_ROLE_OWNER || role === BASIC_ROLE_ADMIN)
  );
};

const accessGrantMatchesUser = (grant: AccessGrantRecord, user: UserSummary) => {
  const subjectType = (grant.subjectType || '').trim();
  const subjectID = (grant.subjectID || '').trim();
  if (subjectType !== 'user' || !subjectID) return false;
  return subjectID === user.id || subjectID === user.sub || subjectID === user.email;
};

const matchesAccessSearch = (query: string, ...values: Array<string | undefined>) => {
  if (!query) return true;
  return values.some(value => (value || '').toLowerCase().includes(query));
};

const formatAccessCount = (count: number, singular: string, plural = `${singular}s`) => `${count} ${count === 1 ? singular : plural}`;

const formatAccessResourceSummary = (value: string) => {
  const { resourceType, resourceID } = parseAAAResourceSelector(value);
  if (!resourceType || resourceType === '*') return 'all resources';
  const config = getAAAResourceTypeConfig(resourceType);
  const label = (config?.label || resourceType).toLowerCase();
  if (!resourceID || resourceID === '*') return `all ${label}`;
  return `${label} ${resourceID}`;
};

const formatAccessActionSummary = (value: string) => {
  const parsed = parseAAAActionValue(value);
  const label = actionLabelFromAAAValue(parsed.action || value) || parsed.action || value || 'action';
  return parsed.effect === 'deny' ? `deny ${label}` : label;
};

const summarizeRoleCoverage = (policies: RolePermission[]) => {
  const labels = Array.from(
    new Set(
      policies.map(policy => {
        const { resourceType } = parseAAAResourceSelector(policy.obj);
        if (!resourceType || resourceType === '*') return 'All resources';
        return getAAAResourceTypeConfig(resourceType)?.label || resourceType;
      })
    )
  );
  return labels.slice(0, 4);
};

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
      { value: 'llm-profiles', label: 'LLM profiles' },
      { value: 'mcp', label: 'MCP' },
      { value: 'config-repos', label: 'Config repositories' },
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
    label: 'Group',
    targetLabel: 'Group',
    allowAll: true,
    allLabel: 'All groups',
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
    label: 'Groups',
    options: [
      { value: 'folder.list', label: 'list' },
      { value: 'folder.create', label: 'create' },
      { value: 'folder.move', label: 'move' },
      { value: 'folder.update', label: 'update' },
      { value: 'folder.delete', label: 'delete' },
      { value: 'config_repo.read', label: 'read config repo' },
      { value: 'config_repo.manage', label: 'manage config repo' },
      { value: 'config_repo.sync', label: 'sync config repo' },
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
  'system:llm-profiles': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'system:steps': [{ label: 'System actions', options: [{ value: 'system.read', label: 'read' }, { value: 'system.update', label: 'update' }] }],
  'dispatcher:status': [{ label: 'Dispatcher actions', options: [{ value: 'system.read', label: 'read' }] }],
  'dispatcher:runners': [{ label: 'Dispatcher actions', options: [{ value: 'system.update', label: 'update' }] }],
  'repository:*': [{ label: 'Repository actions', options: [{ value: 'system.read', label: 'read' }] }],
};

const AAA_ACTION_OPTION_GROUPS_BY_RESOURCE_TYPE: Record<string, AAAOptionGroup[]> = {
  '*': AAA_ALL_ACTION_OPTION_GROUPS,
  folder: [{ label: 'Group actions', options: AAA_ALL_ACTION_OPTION_GROUPS.find(group => group.label === 'Groups')?.options || [] }],
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
  const normalized = (scope || '').trim().replace(/^\/+|\/+$/g, '');
  return !normalized || normalized.toLowerCase() === 'default' ? AAA_DEFAULT_SCOPE_VALUE : normalized;
};

const denormalizeAAAScopeOptionValue = (value: string) => (value === AAA_DEFAULT_SCOPE_VALUE ? '' : (value || '').trim());

const buildAAAScopeOptions = (values: string[]) =>
  dedupeAAAOptions(
    ['', ...values].map(value => {
      const normalized = (value || '').trim().replace(/^\/+|\/+$/g, '');
      const isDefault = !normalized || normalized.toLowerCase() === 'default';
      return {
        value: normalizeAAAScopeOptionValue(normalized),
        label: isDefault ? 'Default scope' : normalized,
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
        groups.push(...buildAAAParentPathOptionGroups(dynamicOptions, { root: 'Top-level groups', parentPrefix: 'Inside /' }));
        break;
      case 'pipelineOptions':
        groups.push(...buildAAAParentPathOptionGroups(dynamicOptions, { root: 'Top-level pipelines', parentPrefix: 'Group /' }));
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
  const buildNamedResourceSelector = (next: Partial<AAANamedResourceDraft>) =>
    buildAAANamedResourceSelector(resourceTypeConfig?.value || '', {
      repoName: '',
      scope: 'scope' in next ? next.scope ?? '' : namedResourceParts.scope,
      name: '',
      hasScope: 'hasScope' in next ? Boolean(next.hasScope) : namedResourceParts.hasScope,
    });
  const hasNamedResourceItemFilter = isNamedScopedResourceType && Boolean(namedResourceParts.repoName || namedResourceParts.name);

  useEffect(() => {
    if (!isNamedScopedResourceType) {
      setForceCustomNamedScope(false);
    }
  }, [isNamedScopedResourceType]);

  useEffect(() => {
    if (!hasNamedResourceItemFilter || !resourceTypeConfig) return;
    const nextObj = buildNamedResourceSelector({});
    if (nextObj === normalizedResource) return;
    onChange({
      name: policy.name,
      obj: nextObj,
      act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
    });
  }, [hasNamedResourceItemFilter, normalizedResource, onChange, parsedAction.action, parsedAction.effect, policy.name, resourceTypeConfig]);

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
        </label>
      </div>
    </>
  );
}

function AccessPanel({
  users,
  loading,
  error,
  accessGrants,
  accessGrantsLoading,
  accessGrantsError,
  policies,
  policiesLoading,
  policiesError,
  newUser,
  policyTemplates,
  onChangeUser,
  onCreateUser,
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
  onCreateAccessGrant,
  onDeleteAccessGrant,
  onReloadAccessGrants,
  onReloadPolicies,
  onUpdateUser,
}: {
  users: UserSummary[];
  loading: boolean;
  error: string | null;
  accessGrants: AccessGrantRecord[];
  accessGrantsLoading: boolean;
  accessGrantsError: string | null;
  policies: RolePermission[];
  policiesLoading: boolean;
  policiesError: string | null;
  newUser: { sub: string; email: string; password: string; roles: string[] };
  policyTemplates: RolePermission[];
  onChangeUser: (next: { sub: string; email: string; password: string; roles: string[] }) => void;
  onCreateUser: (e: FormEvent<HTMLFormElement>, options?: { basicGrants?: BasicGrantInput[] }) => Promise<boolean>;
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
  onCreateAccessGrant: (input: { userID: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => Promise<void>;
  onDeleteAccessGrant: (grantID: string) => Promise<void>;
  onReloadAccessGrants: () => void;
  onReloadPolicies: () => void;
  onUpdateUser: (userId: string, input: { email?: string; status?: string; password?: string }) => Promise<void>;
}) {
  const [accessMode, setAccessMode] = useState<'basic' | 'advanced'>('basic');
  const [activeSection, setActiveSection] = useState<'users' | 'roles' | 'policies'>('users');
  const [showUserModal, setShowUserModal] = useState(false);
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
  const [creatingUserInline, setCreatingUserInline] = useState(false);
  const [creatingPolicyInline, setCreatingPolicyInline] = useState(false);
  const [awaitingUserCreateReset, setAwaitingUserCreateReset] = useState(false);
  const [awaitingPolicyCreateReset, setAwaitingPolicyCreateReset] = useState(false);
  const [basicGrantDraft, setBasicGrantDraft] = useState({ role: '', scope: GENERAL_ACCESS_SCOPE });
  const [basicGrantEntries, setBasicGrantEntries] = useState<EditableAccessGrant[]>([]);
  const [basicGrantSaving, setBasicGrantSaving] = useState(false);
  const [basicGrantError, setBasicGrantError] = useState<string | null>(null);
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

  const openCreateRoleEditor = useCallback(() => {
    setNextPolicyKey('');
    setRoleEditor({
      mode: 'create',
      role: '',
      policies: [],
    });
  }, []);

  const openCreateUserEditor = useCallback(() => {
    setNextUserRole('');
    setAwaitingUserCreateReset(false);
    setUserAccessEditor(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: GENERAL_ACCESS_SCOPE });
    setBasicGrantEntries([]);
    setShowUserModal(true);
  }, []);

  const openCreatePolicyEditor = useCallback(() => {
    setAwaitingPolicyCreateReset(false);
    setPolicyEditor(null);
    setShowPolicyModal(true);
  }, []);

  const openEditRoleEditor = (role: RoleDefinition) => {
    if (isProtectedAccessRole(role.role)) return;
    setShowPolicyModal(false);
    setRoleEditor({
      mode: 'edit',
      role: role.role,
      policies: role.policies.map(p => ({ name: p.name || policyLabel(p), obj: p.obj, act: p.act })),
      original: role.policies,
    });
  };

  const openPolicyEditModal = (policy: RolePermission) => {
    if (isProtectedAccessRole(policy.role)) return;
    setAwaitingPolicyCreateReset(false);
    setShowPolicyModal(false);
    setPolicyEditor({
      original: policy,
      role: policy.role,
      name: policy.name || policyLabel(policy),
      obj: policy.obj,
      act: policy.act,
    });
  };

  const openUserAccessModal = (user: UserSummary) => {
    setAwaitingUserCreateReset(false);
    setShowUserModal(false);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: GENERAL_ACCESS_SCOPE });
    setBasicGrantEntries((basicUserGrantMap.get(user.id) || []).map(editableAccessGrantFromRecord));
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
      if (!isDefaultAdminUser(userAccessEditor.user)) {
        await onUpdateUserRoles(userAccessEditor.user.id, deduped, userAccessEditor.original);
      }
      if (!isDefaultAdminUser(userAccessEditor.user) && basicGrantDirty) {
        await saveBasicGrantsForUser(userAccessEditor.user);
      }
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
      if (isDefaultAdminUser(prev.user)) return prev;
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
      if (isDefaultAdminUser(prev.user)) return prev;
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

  const visiblePolicies = useMemo(() => {
    const combined = [
      ...policyTemplates,
      ...policies.filter(policy => policy.role !== POLICY_TEMPLATE_ROLE && !isProtectedAccessRole(policy.role)),
    ];
    const seen = new Set<string>();
    return combined
      .filter(policy => {
        const key = policyKey(policy);
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .sort((a, b) => a.role.localeCompare(b.role) || policyLabel(a).localeCompare(policyLabel(b)));
  }, [policies, policyTemplates]);
  const policyCount = visiblePolicies.length;
  const [nextPolicyKey, setNextPolicyKey] = useState('');
  const [nextUserRole, setNextUserRole] = useState('');
  const [nextAccessRole, setNextAccessRole] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const searchQuery = searchTerm.trim().toLowerCase();
  const basicAccessGrants = useMemo(() => accessGrants.filter(isBasicAccessGrant), [accessGrants]);
  const basicGrantOptions = useMemo(
    () => [{ value: GENERAL_ACCESS_SCOPE, label: 'General (without group)' }, ...resourceCatalog.folderOptions],
    [resourceCatalog.folderOptions]
  );
  const basicUserGrantMap = useMemo(() => {
    const map = new Map<string, AccessGrantRecord[]>();
    basicAccessGrants.forEach(grant => {
      const user = users.find(entry => accessGrantMatchesUser(grant, entry));
      const key = user?.id || grant.subjectID;
      const entries = map.get(key) || [];
      entries.push(grant);
      map.set(key, entries);
    });
    map.forEach(entries => {
      entries.sort(
        (a, b) =>
          accessGrantSortLabel(a).localeCompare(accessGrantSortLabel(b), undefined, { sensitivity: 'base' }) ||
          a.role.localeCompare(b.role, undefined, { sensitivity: 'base' })
      );
    });
    return map;
  }, [basicAccessGrants, users]);

  const sectionContent = ACCESS_SECTION_CONTENT[activeSection];
  const filteredUsers = useMemo(() => {
    if (!searchQuery) return users;
    return users.filter(user => {
      const grants = basicUserGrantMap.get(user.id) || [];
      return matchesAccessSearch(
        searchQuery,
        user.sub,
        user.email,
        user.status,
        (user.roles || []).map(role => role.role).join(' '),
        grants.map(grant => basicAccessGrantLabel(grant)).join(' ')
      );
    });
  }, [basicUserGrantMap, searchQuery, users]);

  const filteredRoleDefinitions = useMemo(() => {
    if (!searchQuery) return roleDefinitions;
    return roleDefinitions.filter(role => {
      const assignedUsers = roleUserMap.get(role.id) || [];
      const preset = accessPresetForRole(role.role);
      return matchesAccessSearch(
        searchQuery,
        role.role,
        preset?.label,
        preset?.description,
        role.policies.map(policy => `${policyLabel(policy)} ${policy.obj} ${policy.act}`).join(' '),
        assignedUsers.map(item => `${item.user} ${item.email}`).join(' ')
      );
    });
  }, [roleDefinitions, roleUserMap, searchQuery]);

  const filteredPolicies = useMemo(() => {
    if (!searchQuery) return visiblePolicies;
    return visiblePolicies.filter(policy =>
      matchesAccessSearch(
        searchQuery,
        policy.role,
        policy.name,
        policy.obj,
        policy.act,
        policyLabel(policy),
        formatAccessResourceSummary(policy.obj),
        formatAccessActionSummary(policy.act)
      )
    );
  }, [searchQuery, visiblePolicies]);

  const isNewUserPristine =
    !newUser.sub.trim() &&
    !newUser.email.trim() &&
    !newUser.password &&
    (newUser.roles || []).length === 0 &&
    basicGrantEntries.length === 0;
  const isNewPolicyPristine =
    !newPermission.name.trim() && newPermission.obj.trim() === 'pipeline:*' && newPermission.act.trim() === 'pipeline.read';
  const selectedBasicUserID = userAccessEditor?.user.id || '';
  const selectedBasicUser = userAccessEditor?.user ?? null;
  const userRoleAssignmentsLocked = isDefaultAdminUser(userAccessEditor?.user);
  const selectedBasicUserGrants = useMemo(
    () => (selectedBasicUserID ? basicUserGrantMap.get(selectedBasicUserID) || [] : []),
    [basicUserGrantMap, selectedBasicUserID]
  );
  const basicGrantDirty = useMemo(() => {
    const originalKeys = new Set(selectedBasicUserGrants.map(grant => accessGrantEditKey(grant)));
    const draftKeys = new Set(basicGrantEntries.map(grant => accessGrantEditKey(grant)));
    if (originalKeys.size !== draftKeys.size) return true;
    return Array.from(originalKeys).some(key => !draftKeys.has(key));
  }, [basicGrantEntries, selectedBasicUserGrants]);
  const basicGrantDraftDuplicate = useMemo(() => {
    const role = basicGrantDraft.role.trim().toLowerCase();
    if (!role) return false;
    const draftGrant = {
      role,
      resourceType: role === BASIC_ROLE_ADMIN ? 'platform' : 'folder',
      resourceID: role === BASIC_ROLE_ADMIN ? 'platform' : basicGrantDraft.scope || GENERAL_ACCESS_SCOPE,
    };
    const draftKey = accessGrantEditKey(draftGrant);
    return basicGrantEntries.some(grant => accessGrantEditKey(grant) === draftKey);
  }, [basicGrantDraft, basicGrantEntries]);

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
        console.error('Failed to load AAA groups', groupsResult.reason);
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

      const folderOptions = buildAAAGroupOptions(groups);
      const triggerOptions = buildAAAStringOptions(triggers);
      const scopeOptions = buildAAAScopeOptions([...secretScopes, ...variableScopes]);
      setResourceCatalog({
        folderOptions,
        pipelineOptions: buildAAAStringOptions(pipelines),
        triggerOptions,
        repositoryOptions: triggerOptions,
        secretScopeOptions: scopeOptions,
        variableScopeOptions: scopeOptions,
      });
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    setNextAccessRole('');
  }, [userAccessEditor]);

  useEffect(() => {
    setBasicGrantEntries(selectedBasicUserGrants.map(editableAccessGrantFromRecord));
  }, [selectedBasicUserGrants]);

  useEffect(() => {
    setSearchTerm('');
    setSearchOpen(false);
    setBasicGrantError(null);
    setUserAccessEditor(null);
    setBasicGrantEntries([]);
  }, [accessMode]);

  useEffect(() => {
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: GENERAL_ACCESS_SCOPE });
  }, [userAccessEditor?.user.id]);

  useEffect(() => {
    setSearchTerm('');
    setSearchOpen(false);
    setShowUserModal(false);
    setShowPolicyModal(false);
    setRoleEditor(null);
    setPolicyEditor(null);
    setUserAccessEditor(null);
    setAwaitingUserCreateReset(false);
    setAwaitingPolicyCreateReset(false);
  }, [activeSection]);

  useEffect(() => {
    if (!showUserModal || !awaitingUserCreateReset || !isNewUserPristine) return;
    setShowUserModal(false);
    setAwaitingUserCreateReset(false);
  }, [awaitingUserCreateReset, isNewUserPristine, showUserModal]);

  useEffect(() => {
    if (!showPolicyModal || !awaitingPolicyCreateReset || !isNewPolicyPristine) return;
    setShowPolicyModal(false);
    setAwaitingPolicyCreateReset(false);
  }, [awaitingPolicyCreateReset, isNewPolicyPristine, showPolicyModal]);

  const handleCreateUserInline = async (e: FormEvent<HTMLFormElement>) => {
    setCreatingUserInline(true);
    setAwaitingUserCreateReset(true);
    try {
      const created = await onCreateUser(e, { basicGrants: normalizeEditableBasicGrants(basicGrantEntries) });
      if (created) {
        setBasicGrantError(null);
        setBasicGrantDraft({ role: '', scope: GENERAL_ACCESS_SCOPE });
        setBasicGrantEntries([]);
      }
    } finally {
      setCreatingUserInline(false);
    }
  };

  const handleCreatePolicyInline = async (e: FormEvent<HTMLFormElement>) => {
    setCreatingPolicyInline(true);
    setAwaitingPolicyCreateReset(true);
    try {
      await onCreatePermission(e);
    } finally {
      setCreatingPolicyInline(false);
    }
  };

  const openConfirmDialog = (message: string, onConfirm: () => Promise<void> | void) => {
    setConfirmDialog({ message, onConfirm });
  };

  const confirmDeleteUser = (userId: string) => {
    openConfirmDialog('Delete this user? This cannot be undone.', () => onDeleteUser(userId));
  };

  const confirmDeleteRoleDefinition = (role: RoleDefinition) => {
    if (isProtectedAccessRole(role.role)) return;
    openConfirmDialog('Delete this role and its policies? This cannot be undone.', () => onDeleteRoleDefinition(role));
  };

  const confirmDeletePolicy = (policy: RolePermission) => {
    if (isProtectedAccessRole(policy.role)) return;
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
    if (accessMode === 'basic') {
      void onReloadUsers();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === 'users') {
      void onReloadUsers();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === 'policies') {
      void onReloadPolicies();
      return;
    }
    void onReloadUsers();
    if (activeSection === 'roles') {
      void onReloadPolicies();
    }
  }, [accessMode, activeSection, onReloadAccessGrants, onReloadPolicies, onReloadUsers]);

  const handleStageBasicGrant = (e?: FormEvent<HTMLFormElement>) => {
    e?.preventDefault();
    const creatingUser = showUserModal && !userAccessEditor;
    if (!selectedBasicUser && !creatingUser) {
      setBasicGrantError('Select a user first.');
      return;
    }
    if (selectedBasicUser && isDefaultAdminUser(selectedBasicUser)) {
      setBasicGrantError('Default admin role assignments are locked.');
      return;
    }
    if (creatingUser && newUser.sub.trim().toLowerCase() === 'admin') {
      setBasicGrantError('Default admin role assignments are locked.');
      return;
    }

    const normalizedRole = basicGrantDraft.role.trim().toLowerCase();
    if (!normalizedRole) {
      setBasicGrantError('Choose an access level.');
      return;
    }

    setBasicGrantError(null);
    const nextGrant: EditableAccessGrant = {
      localID: `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`,
      role: normalizedRole,
      resourceType: normalizedRole === BASIC_ROLE_ADMIN ? 'platform' : 'folder',
      resourceID: normalizedRole === BASIC_ROLE_ADMIN ? 'platform' : basicGrantDraft.scope || GENERAL_ACCESS_SCOPE,
      inherit: normalizedRole !== BASIC_ROLE_ADMIN,
    };
    const nextKey = accessGrantEditKey(nextGrant);
    if (basicGrantEntries.some(grant => accessGrantEditKey(grant) === nextKey)) {
      setBasicGrantError('This basic role is already listed.');
      return;
    }
    const nextTargetKey = accessGrantTargetKey(nextGrant);
    setBasicGrantEntries(prev => {
      let replaced = false;
      const nextEntries: EditableAccessGrant[] = [];
      prev.forEach(grant => {
        if (accessGrantTargetKey(grant) !== nextTargetKey) {
          nextEntries.push(grant);
          return;
        }
        if (replaced) return;
        nextEntries.push({
          ...nextGrant,
          localID: grant.localID,
          id: grant.id,
          grantedBy: grant.grantedBy,
        });
        replaced = true;
      });
      return replaced ? nextEntries : [...prev, nextGrant];
    });
    setBasicGrantDraft(prev => ({ ...prev, role: '' }));
  };

  const removeBasicGrantDraft = (localID: string) => {
    if (isDefaultAdminUser(selectedBasicUser)) return;
    setBasicGrantEntries(prev => prev.filter(grant => grant.localID !== localID));
    setBasicGrantError(null);
  };

  const resetBasicGrantDrafts = () => {
    if (isDefaultAdminUser(selectedBasicUser)) return;
    setBasicGrantEntries(selectedBasicUserGrants.map(editableAccessGrantFromRecord));
    setBasicGrantError(null);
  };

  const saveBasicGrantsForUser = async (user: UserSummary) => {
    if (!selectedBasicUser) {
      setBasicGrantError('Select a user first.');
      return;
    }
    if (isDefaultAdminUser(user)) {
      setBasicGrantError('Default admin role assignments are locked.');
      return;
    }
    if (!basicGrantDirty) return;
    const normalizedDraftEntries = Array.from(
      basicGrantEntries.reduce((entries, grant) => entries.set(accessGrantTargetKey(grant), grant), new Map<string, EditableAccessGrant>()).values()
    );
    const draftKeys = new Set(normalizedDraftEntries.map(grant => accessGrantEditKey(grant)));
    const originalByKey = new Map(selectedBasicUserGrants.map(grant => [accessGrantEditKey(grant), grant]));
    const grantsToDelete = selectedBasicUserGrants.filter(grant => !draftKeys.has(accessGrantEditKey(grant)));
    const grantsToAdd = normalizedDraftEntries.filter(grant => !originalByKey.has(accessGrantEditKey(grant)));

    setBasicGrantSaving(true);
    setBasicGrantError(null);
    try {
      for (const grant of grantsToDelete) {
        await onDeleteAccessGrant(grant.id);
      }
      for (const grant of grantsToAdd) {
        await onCreateAccessGrant({
          userID: user.id,
          role: grant.role,
          resourceType: grant.resourceType,
          resourceID: grant.resourceID,
          inherit: grant.inherit,
        });
      }
    } catch (error) {
      setBasicGrantError(error instanceof Error ? error.message : 'Failed to save basic roles');
      throw error;
    } finally {
      setBasicGrantSaving(false);
    }
  };

  const availablePoliciesForRoleEditor = roleEditor ? policyOptions : [];

  const createUserEditor = (
    <div className="access-editor-surface">
      <div className="access-editor-header">
        <div>
          <p className="access-editor-kicker">Create user</p>
          <h5 className="access-editor-title">New local account</h5>
          <p className="access-editor-text">Create a local account.</p>
        </div>
        <button
          type="button"
          className="access-inline-btn access-inline-btn--pill"
          onClick={() => {
            setAwaitingUserCreateReset(false);
            setShowUserModal(false);
            setBasicGrantError(null);
            setBasicGrantDraft({ role: '', scope: GENERAL_ACCESS_SCOPE });
            setBasicGrantEntries([]);
          }}
        >
          Close
        </button>
      </div>
      <form className="access-editor-form" onSubmit={handleCreateUserInline}>
        <div className="access-editor-grid">
          <label className="access-minimal-label">
            <span>Username (sub)</span>
            <input
              className="pipelines-input"
              value={newUser.sub}
              onChange={e => onChangeUser({ ...newUser, sub: e.target.value })}
              placeholder="alice"
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
        <div className="access-editor-section">
          <div className="access-minimal-section__header">
            <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
            <span className="text-[11px] text-[var(--text-secondary)]">Optional with basic roles</span>
          </div>
          <div className="space-y-2">
            {newUser.roles.length === 0 && <p className="text-[11px] text-[var(--text-secondary)]">Add access roles here or use basic roles below.</p>}
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
                <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => removeNewUserRoleEntry(idx)} title="Remove role">
                  <TrashIcon />
                </button>
              </div>
            ))}
            <div className="access-editor-inline-add">
              <select
                className="pipelines-input flex-1"
                value={nextUserRole}
                onChange={e => setNextUserRole(e.target.value)}
                disabled={allRoleOptions.length === 0}
              >
                <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}</option>
                {allRoleOptions.map(role => (
                  <option key={`new-role-opt-${role}`} value={role}>
                    {role}
                  </option>
                ))}
              </select>
              <button type="button" className="glass-button-subtle" onClick={appendUserRoleFromPicker} disabled={allRoleOptions.length === 0}>
                Add access role
              </button>
            </div>
          </div>
        </div>
        <div className="access-editor-section">
          <div className="access-minimal-section__header">
            <p className="text-sm font-medium text-[var(--text-primary)]">Basic roles</p>
            <span className="text-[11px] text-[var(--text-secondary)]">{basicGrantEntries.length} listed</span>
          </div>
          <div className="access-editor-grid">
            <label className="access-minimal-label">
              <span>Access level</span>
              <select
                className="pipelines-input"
                value={basicGrantDraft.role}
                onChange={e => {
                  const role = e.target.value;
                  setBasicGrantDraft(prev => ({
                    ...prev,
                    role,
                    scope: role === BASIC_ROLE_ADMIN ? prev.scope : prev.scope || GENERAL_ACCESS_SCOPE,
                  }));
                }}
              >
                <option value="">Select role</option>
                <option value={BASIC_ROLE_VIEWER}>Viewer</option>
                <option value={BASIC_ROLE_DEVELOPER}>Developer</option>
                <option value={BASIC_ROLE_OWNER}>Owner</option>
                <option value={BASIC_ROLE_ADMIN}>Admin</option>
              </select>
            </label>
            <label className="access-minimal-label">
              <span>Group target</span>
              <select
                className="pipelines-input"
                value={basicGrantDraft.role === BASIC_ROLE_ADMIN ? 'platform' : basicGrantDraft.scope}
                onChange={e => setBasicGrantDraft(prev => ({ ...prev, scope: e.target.value }))}
                disabled={basicGrantDraft.role === BASIC_ROLE_ADMIN}
              >
                {basicGrantDraft.role === BASIC_ROLE_ADMIN ? (
                  <option value="platform">Platform wide</option>
                ) : (
                  basicGrantOptions.map(option => (
                    <option key={`new-user-basic-${option.value}`} value={option.value}>
                      {option.label}
                    </option>
                  ))
                )}
              </select>
            </label>
          </div>
          <div className="access-editor-footer access-editor-footer--inline">
            <button type="button" className="glass-button-subtle" onClick={() => handleStageBasicGrant()} disabled={creatingUserInline || !basicGrantDraft.role || basicGrantDraftDuplicate}>
              Add basic role
            </button>
          </div>
          {basicGrantError && <div className="access-error-banner">{basicGrantError}</div>}
          <div className="space-y-2">
            {basicGrantEntries.length === 0 ? (
              <p className="text-[12px] text-[var(--text-secondary)]">No basic roles listed.</p>
            ) : (
              basicGrantEntries.map(grant => (
                <div key={grant.localID} className="access-minimal-row access-minimal-row--stack">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className={`access-chip ${accessPresetToneClass(grant.role)}`}>{grant.role}</span>
                      <span className="access-chip access-chip--muted">{accessGrantResourceSummary(grant)}</span>
                      {grant.inherit && grant.resourceType === 'folder' && grant.resourceID !== 'general' && (
                        <span className="access-chip access-chip--muted">Includes children</span>
                      )}
                    </div>
                    <p className="text-[11px] text-[var(--text-secondary)] mt-2">{basicAccessGrantDescription(grant)}</p>
                  </div>
                  <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => removeBasicGrantDraft(grant.localID)} disabled={creatingUserInline}>
                    <TrashIcon />
                    <span>Remove</span>
                  </button>
                </div>
              ))
            )}
          </div>
        </div>
        <div className="access-editor-footer">
          <button type="submit" className="glass-button-primary" disabled={creatingUserInline}>
            {creatingUserInline ? 'Saving…' : 'Save user'}
          </button>
        </div>
      </form>
    </div>
  );
  const accessSearchPlaceholder = accessMode === 'basic' ? 'Search by username, email, role, or group' : sectionContent.searchPlaceholder;
  const accessSearchControl = (
    <div className={`pipelines-search-shell access-search-shell ${searchOpen ? 'open' : ''}`}>
      <button
        type="button"
        className="pipelines-search-toggle"
        aria-label={accessSearchPlaceholder}
        onClick={() => {
          setSearchOpen(true);
          requestAnimationFrame(() => searchInputRef.current?.focus());
        }}
      >
        <SearchIcon />
      </button>
      <input
        ref={searchInputRef}
        id={`access-${accessMode}-${activeSection}-search`}
        type="text"
        placeholder={accessSearchPlaceholder}
        className="pipelines-search-input"
        value={searchTerm}
        onChange={event => {
          setSearchTerm(event.target.value);
          if (event.target.value && !searchOpen) setSearchOpen(true);
        }}
        onBlur={() => {
          if (!searchTerm.trim()) setSearchOpen(false);
        }}
      />
      {(searchTerm || searchOpen) && (
        <button
          type="button"
          className="pipelines-search-clear"
          onClick={() => {
            setSearchTerm('');
            setSearchOpen(false);
            searchInputRef.current?.blur();
          }}
          aria-label="Clear search"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
  const usersWorkspace = (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        {(error || accessGrantsError) && (
          <div className="access-error-banner">
            {error ? `Failed to load users: ${error}` : `Failed to load basic roles: ${accessGrantsError}`}
          </div>
        )}
        {loading || accessGrantsLoading ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">Loading people…</p>
            <p className="text-sm text-[var(--text-secondary)]">Fetching accounts and current role assignments.</p>
          </div>
        ) : users.length === 0 ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">No users yet</p>
            <p className="text-sm text-[var(--text-secondary)]">Create a local account, then assign access and basic roles.</p>
          </div>
        ) : filteredUsers.length === 0 ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">No people match this search</p>
            <p className="text-sm text-[var(--text-secondary)]">Try a username, email address, role, or group path.</p>
          </div>
        ) : (
          <div className="access-entity-grid access-entity-grid--users">
            {filteredUsers.map(user => {
              const userRoles = user.roles || [];
              const grants = basicUserGrantMap.get(user.id) || [];
              const isSelected = userAccessEditor?.user.id === user.id;
              return (
                <article key={user.id} className={`access-card access-card--user ${isSelected ? 'access-card--selected' : ''}`}>
                  <div className="access-card__header">
                    <div className="min-w-0 flex items-center gap-3">
                      <div className="access-avatar">{(user.sub || user.email || 'U').charAt(0).toUpperCase()}</div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <p className="access-card__title">{user.sub}</p>
                          <span className={`access-status access-status--${statusKey(user.status)}`}>{user.status || 'unknown'}</span>
                        </div>
                        <p className="access-card__subtitle">{user.email || 'No email address'}</p>
                        <p className="access-card__meta-line">
                          {user.last_login ? `Last sign-in ${formatTimestamp(user.last_login)}` : 'Never signed in'}
                        </p>
                      </div>
                    </div>
                    <div className="access-card__actions">
                      <button
                        type="button"
                        className="access-card-action"
                        title="Edit user"
                        aria-label={`Edit ${user.sub || user.email || 'user'}`}
                        onClick={() => openUserAccessModal(user)}
                      >
                        <EditIcon />
                      </button>
                      <button
                        type="button"
                        className="access-card-action access-card-action--danger"
                        title="Delete user"
                        aria-label={`Delete ${user.sub || user.email || 'user'}`}
                        onClick={() => confirmDeleteUser(user.id)}
                        disabled={loading}
                      >
                        <TrashIcon />
                      </button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <p className="access-card__label">Access roles</p>
                    <div className="flex flex-wrap gap-2">
                      {userRoles.length > 0 ? (
                        userRoles.map(role => (
                          <span key={`${user.id}-${role.role}`} className={`access-chip ${accessPresetToneClass(role.role)}`}>
                            {role.role}
                          </span>
                        ))
                      ) : (
                        <span className="text-sm text-[var(--text-secondary)]">No roles assigned yet</span>
                      )}
                    </div>
                  </div>
                  <div className="space-y-2">
                    <p className="access-card__label">Basic roles</p>
                    <div className="flex flex-wrap gap-2">
                      {grants.length > 0 ? (
                        grants.slice(0, 4).map(grant => (
                          <span key={grant.id} className={`access-chip ${accessPresetToneClass(grant.role)}`}>
                            {basicAccessGrantLabel(grant)}
                          </span>
                        ))
                      ) : (
                        <span className="text-sm text-[var(--text-secondary)]">No basic roles yet</span>
                      )}
                      {grants.length > 4 && <span className="access-chip access-chip--muted">+ {grants.length - 4} more</span>}
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </div>
      <aside className="access-editor-pane">
        {userAccessEditor ? (
          <div className="access-editor-surface access-editor-surface--minimal">
            <div className="access-editor-header">
              <div>
                <p className="access-editor-kicker">Edit user</p>
                <h5 className="access-editor-title">{userAccessEditor.user.sub}</h5>
                <p className="access-editor-text">Manage account details, access roles, and group-scoped basic roles.</p>
              </div>
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setUserAccessEditor(null)}>
                Close
              </button>
            </div>
            <form className="access-editor-form access-editor-form--compact" onSubmit={handleSaveUserAccess}>
              <div className="access-editor-grid">
                <label className="access-minimal-label">
                  <span>Email</span>
                  <input
                    className="pipelines-input"
                    type="email"
                    value={userAccessEditor.email}
                    onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, email: e.target.value } : prev))}
                    placeholder="name@example.com"
                  />
                </label>
                <label className="access-minimal-label">
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
              <label className="access-minimal-label">
                <span>New password</span>
                <input
                  className="pipelines-input"
                  type="password"
                  value={userAccessEditor.password}
                  onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, password: e.target.value } : prev))}
                  placeholder="Leave blank to keep current password"
                />
              </label>
              <div className="access-editor-section access-editor-section--plain">
                <div className="access-minimal-section__header">
                  <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
                  <span className="text-[11px] text-[var(--text-secondary)]">
                    {userRoleAssignmentsLocked ? 'Locked' : `${userAccessEditor.entries.length} assigned`}
                  </span>
                </div>
                <div className="space-y-2">
                  {userAccessEditor.entries.length === 0 && <p className="text-[12px] text-[var(--text-secondary)]">No roles assigned yet.</p>}
                  {userAccessEditor.entries.map((entry, idx) => {
                    const label = userRoleAssignmentsLocked ? 'Protected admin role assignment' : 'Remove assignment';
                    return (
                      <div key={`user-role-${idx}`} className="access-minimal-row justify-between">
                        <span className={`access-chip ${accessPresetToneClass(entry)}`}>{entry || 'Role'}</span>
                        <button
                          type="button"
                          className={`access-inline-btn access-inline-btn--danger access-role-remove ${userRoleAssignmentsLocked ? 'opacity-60 cursor-not-allowed' : ''}`}
                          onClick={() => removeUserAccessEntry(idx)}
                          title={label}
                          aria-label={label}
                          disabled={userRoleAssignmentsLocked}
                        >
                          <TrashIcon />
                        </button>
                      </div>
                    );
                  })}
                  <div className="access-editor-inline-add">
                    <select className="pipelines-input w-full" value={nextAccessRole} onChange={e => setNextAccessRole(e.target.value)} disabled={userRoleAssignmentsLocked}>
                      <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Select a role'}</option>
                      {allRoleOptions.map(role => (
                        <option key={`access-role-${role}`} value={role}>
                          {role}
                        </option>
                      ))}
                    </select>
                    <button type="button" className="glass-button-subtle" onClick={addUserAccessEntry} disabled={userRoleAssignmentsLocked || !nextAccessRole || allRoleOptions.length === 0}>
                      Add
                    </button>
                  </div>
                </div>
              </div>
              <div className="access-editor-section access-editor-section--plain">
                <div className="access-minimal-section__header">
                  <p className="text-sm font-medium text-[var(--text-primary)]">Basic roles</p>
                  <span className="text-[11px] text-[var(--text-secondary)]">
                    {userRoleAssignmentsLocked ? 'Locked' : `${basicGrantEntries.length} listed`}
                  </span>
                </div>
                <div className="access-editor-grid">
                  <label className="access-minimal-label">
                    <span>Access level</span>
                    <select
                      className="pipelines-input"
                      value={basicGrantDraft.role}
                      onChange={e => {
                        const role = e.target.value;
                        setBasicGrantDraft(prev => ({
                          ...prev,
                          role,
                          scope: role === BASIC_ROLE_ADMIN ? prev.scope : prev.scope || GENERAL_ACCESS_SCOPE,
                        }));
                      }}
                      disabled={userRoleAssignmentsLocked}
                    >
                      <option value="">Select role</option>
                      <option value={BASIC_ROLE_VIEWER}>Viewer</option>
                      <option value={BASIC_ROLE_DEVELOPER}>Developer</option>
                      <option value={BASIC_ROLE_OWNER}>Owner</option>
                      <option value={BASIC_ROLE_ADMIN}>Admin</option>
                    </select>
                  </label>
                  <label className="access-minimal-label">
                    <span>Group target</span>
                    <select
                      className="pipelines-input"
                      value={basicGrantDraft.role === BASIC_ROLE_ADMIN ? 'platform' : basicGrantDraft.scope}
                      onChange={e => setBasicGrantDraft(prev => ({ ...prev, scope: e.target.value }))}
                      disabled={userRoleAssignmentsLocked || basicGrantDraft.role === BASIC_ROLE_ADMIN}
                    >
                      {basicGrantDraft.role === BASIC_ROLE_ADMIN ? (
                        <option value="platform">Platform wide</option>
                      ) : (
                        basicGrantOptions.map(option => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))
                      )}
                    </select>
                  </label>
                </div>
                <div className="access-editor-footer access-editor-footer--inline">
                  <button type="button" className="glass-button-subtle" onClick={() => handleStageBasicGrant()} disabled={userRoleAssignmentsLocked || basicGrantSaving || !basicGrantDraft.role || basicGrantDraftDuplicate}>
                    Add
                  </button>
                </div>
                {basicGrantError && <div className="access-error-banner">{basicGrantError}</div>}
                <div className="space-y-2">
                  {basicGrantEntries.length === 0 ? (
                    <p className="text-[12px] text-[var(--text-secondary)]">No basic roles listed.</p>
                  ) : (
                    basicGrantEntries.map(grant => (
                      <div key={grant.localID} className="access-minimal-row access-minimal-row--stack">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className={`access-chip ${accessPresetToneClass(grant.role)}`}>{grant.role}</span>
                            <span className="access-chip access-chip--muted">{accessGrantResourceSummary(grant)}</span>
                            {grant.inherit && grant.resourceType === 'folder' && grant.resourceID !== 'general' && (
                              <span className="access-chip access-chip--muted">Includes children</span>
                            )}
                          </div>
                          <p className="text-[11px] text-[var(--text-secondary)] mt-2">
                            {basicAccessGrantDescription(grant)}
                            {grant.grantedBy ? ` Granted by ${grant.grantedBy}.` : ''}
                          </p>
                        </div>
                        <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => removeBasicGrantDraft(grant.localID)} disabled={userRoleAssignmentsLocked || basicGrantSaving}>
                          <TrashIcon />
                          <span>Remove</span>
                        </button>
                      </div>
                    ))
                  )}
                </div>
              </div>
              <div className="access-editor-footer gap-2">
                {basicGrantDirty && (
                  <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={resetBasicGrantDrafts} disabled={userRoleAssignmentsLocked || basicGrantSaving || savingUserAccess}>
                    Reset basic roles
                  </button>
                )}
                <button type="submit" className="glass-button-primary" disabled={savingUserAccess || basicGrantSaving}>
                  {savingUserAccess || basicGrantSaving ? 'Saving…' : 'Save changes'}
                </button>
              </div>
            </form>
          </div>
        ) : showUserModal ? (
          createUserEditor
        ) : (
          <AccessEditorEmptyState sectionLabel="User details" hint="Select a user to edit access." />
        )}
      </aside>
    </div>
  );

  return (
    <div className="access-layout pb-24" data-access-build={ACCESS_UI_BUILD_ID}>
      <div className="access-shell">
        <div className="access-header">
          <div className="access-title-group">
            <h3 className="access-header__title">Access</h3>
          </div>
          <div className="access-header__actions">
            <div className="access-mode-switch" role="tablist" aria-label="Access mode">
              <button
                type="button"
                role="tab"
                aria-selected={accessMode === 'basic'}
                className={`access-mode-switch__option ${accessMode === 'basic' ? 'access-mode-switch__option--active' : ''}`}
                onClick={() => setAccessMode('basic')}
              >
                <span className="access-mode-switch__title">Basic</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={accessMode === 'advanced'}
                className={`access-mode-switch__option ${accessMode === 'advanced' ? 'access-mode-switch__option--active' : ''}`}
                onClick={() => setAccessMode('advanced')}
              >
                <span className="access-mode-switch__title">Advanced</span>
              </button>
            </div>
            <button
              className="glass-button-ghost access-toolbar-btn"
              type="button"
              onClick={handleRefresh}
              disabled={loading || accessGrantsLoading || policiesLoading}
            >
              <RefreshIcon />
              <span>Refresh</span>
            </button>
          </div>
        </div>

        {accessMode === 'advanced' && (
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
                  <span className="access-tab__badge">{tab.id === 'policies' ? policyCount : tab.count}</span>
                </button>
              ))}
            </div>
          </div>
        )}

        {accessMode === 'basic' ? (
          <div className="access-panel-card">
            <div className="access-section-header">
              <div className="space-y-1">
                <h4 className="access-section-title">People and basic roles</h4>
              </div>
              <div className="access-section-tools">
                {accessSearchControl}
                <button type="button" className="glass-button-primary access-section-action" onClick={openCreateUserEditor}>
                  <PlusIcon />
                  <span>Add user</span>
                </button>
              </div>
            </div>
            {usersWorkspace}
          </div>
        ) : (
          <div className="access-panel-card">
            <div className="access-section-header">
              <div className="space-y-1">
                <h4 className="access-section-title">{sectionContent.title}</h4>
              </div>
              <div className="access-section-tools">
                {accessSearchControl}
                {activeSection === 'users' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreateUserEditor}>
                    <PlusIcon />
                    <span>Add user</span>
                  </button>
                )}
                {activeSection === 'roles' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreateRoleEditor}>
                    <PlusIcon />
                    <span>Add role</span>
                  </button>
                )}
                {activeSection === 'policies' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreatePolicyEditor}>
                    <PlusIcon />
                    <span>Add policy</span>
                  </button>
                )}
              </div>
            </div>

          {activeSection === 'users' && usersWorkspace}

          {activeSection === 'roles' && (
            <div className="access-workspace">
              <div className="space-y-4 access-workspace__list">
                {(policiesError || error) && <div className="access-error-banner">Failed to load roles: {policiesError || error}</div>}
                {loading || policiesLoading ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">Loading roles…</p>
                    <p className="text-sm text-[var(--text-secondary)]">Collecting reusable bundles and their current assignees.</p>
                  </div>
                ) : roleDefinitions.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No roles yet</p>
                    <p className="text-sm text-[var(--text-secondary)]">Create a role and attach policies that match the language your operators already use.</p>
                  </div>
                ) : filteredRoleDefinitions.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No roles match this search</p>
                    <p className="text-sm text-[var(--text-secondary)]">Try a role name, policy label, or one of the assigned people.</p>
                  </div>
                ) : (
                  <div className="access-entity-grid access-entity-grid--roles">
                    {filteredRoleDefinitions.map(role => {
                      const assignedUsers = roleUserMap.get(role.id) || [];
                      const preset = accessPresetForRole(role.role);
                      const coverage = summarizeRoleCoverage(role.policies);
                      const protectedRole = isProtectedAccessRole(role.role);
                      const isSelected = roleEditor?.role === role.role;
                      return (
                        <article key={role.id} className={`access-card access-card--role ${isSelected ? 'access-card--selected' : ''}`}>
                          <div className="access-card__header">
                            <div className="space-y-2 min-w-0">
                              <div className="flex items-center gap-2 flex-wrap">
                                <p className="access-card__title">{role.role}</p>
                                {preset && <span className={`access-chip ${accessPresetToneClass(role.role)}`}>{preset.label}</span>}
                                {protectedRole && <span className="access-chip access-chip--muted">Protected</span>}
                              </div>
                              <p className="access-card__subtitle">
                                {preset?.description || 'Reusable role bundle for assigning multiple low-level AAA policies together.'}
                              </p>
                              <p className="access-card__meta-line">
                                {formatAccessCount(role.policies.length, 'policy', 'policies')} · {formatAccessCount(assignedUsers.length, 'person', 'people')}
                              </p>
                            </div>
                            <div className="access-card__actions">
                              {protectedRole ? (
                                <span className="access-chip access-chip--muted">Protected</span>
                              ) : (
                                <>
                                  <button
                                    type="button"
                                    className="access-card-action"
                                    title="Edit role"
                                    aria-label={`Edit ${role.role}`}
                                    onClick={() => openEditRoleEditor(role)}
                                  >
                                    <EditIcon />
                                  </button>
                                  <button
                                    type="button"
                                    className="access-card-action access-card-action--danger"
                                    title="Delete role"
                                    aria-label={`Delete ${role.role}`}
                                    onClick={() => confirmDeleteRoleDefinition(role)}
                                  >
                                    <TrashIcon />
                                  </button>
                                </>
                              )}
                            </div>
                          </div>
                          <div className="space-y-2">
                            <p className="access-card__label">Coverage</p>
                            <div className="flex flex-wrap gap-2">
                              {coverage.map(label => (
                                <span key={`${role.id}-coverage-${label}`} className="access-chip access-chip--muted">
                                  {label}
                                </span>
                              ))}
                              {coverage.length === 0 && <span className="text-sm text-[var(--text-secondary)]">No coverage yet</span>}
                            </div>
                          </div>
                          <div className="space-y-2">
                            <p className="access-card__label">Includes</p>
                            <div className="flex flex-wrap gap-2">
                              {role.policies.slice(0, 4).map(policy => (
                                <span key={policyKey(policy)} className="access-chip access-chip--muted">
                                  {policyLabel(policy)}
                                </span>
                              ))}
                              {role.policies.length > 4 && <span className="access-chip access-chip--muted">+ {role.policies.length - 4} more</span>}
                            </div>
                          </div>
                        </article>
                      );
                    })}
                  </div>
                )}
              </div>
              <aside className="access-editor-pane">
                {roleEditor ? (
                  <div className="access-editor-surface access-editor-surface--minimal">
                    <div className="access-editor-header">
                      <div>
                        <p className="access-editor-kicker">{roleEditor.mode === 'create' ? 'Create role' : 'Edit role'}</p>
                        <h5 className="access-editor-title">{roleEditor.mode === 'create' ? 'Role definition' : roleEditor.role}</h5>
                        <p className="access-editor-text">Assign reusable policies.</p>
                      </div>
                      <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setRoleEditor(null)}>
                        Close
                      </button>
                    </div>
                    <form className="access-editor-form access-editor-form--compact" onSubmit={handleSaveRoleEditor}>
                      <label className="access-minimal-label">
                        <span>Role name</span>
                        <input
                          className="pipelines-input"
                          value={roleEditor.role}
                          onChange={e => setRoleEditor(prev => (prev ? { ...prev, role: e.target.value } : prev))}
                          placeholder="developer"
                          required
                          disabled={roleEditor.mode === 'edit'}
                        />
                      </label>
                      <div className="access-editor-section access-editor-section--plain">
                        <div className="access-minimal-section__header">
                          <p className="text-sm font-medium text-[var(--text-primary)]">Policies</p>
                          <span className="text-[11px] text-[var(--text-secondary)]">{formatAccessCount(roleEditor.policies.length, 'policy', 'policies')}</span>
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
                          <div className="access-editor-inline-add">
                            {availablePoliciesForRoleEditor.length > 0 ? (
                              <>
                                <select className="pipelines-input w-full" value={nextPolicyKey} onChange={e => setNextPolicyKey(e.target.value)}>
                                  <option value="" disabled>
                                    Add policy…
                                  </option>
                                  {availablePoliciesForRoleEditor.map(item => (
                                    <option key={item.key} value={item.key}>
                                      {item.label}
                                    </option>
                                  ))}
                                </select>
                                <button
                                  type="button"
                                  className="glass-button-subtle"
                                  onClick={() => {
                                    if (nextPolicyKey) {
                                      addExistingPolicyDraft(nextPolicyKey);
                                      setNextPolicyKey('');
                                    }
                                  }}
                                  disabled={!nextPolicyKey}
                                >
                                  Add
                                </button>
                              </>
                            ) : (
                              <p className="text-sm text-[var(--text-secondary)]">No reusable policies available</p>
                            )}
                          </div>
                        </div>
                      </div>
                      <div className="access-editor-footer access-editor-footer--inline">
                        <button type="submit" className="glass-button-primary" disabled={savingRoleEditor}>
                          {savingRoleEditor ? 'Saving…' : 'Save role'}
                        </button>
                      </div>
                    </form>
                  </div>
                ) : (
                  <AccessEditorEmptyState sectionLabel="Role details" hint="Select a role to edit policies." />
                )}
              </aside>
            </div>
          )}

          {activeSection === 'policies' && (
            <div className="access-workspace">
              <div className="space-y-4 access-workspace__list">
                {policiesError && <div className="access-error-banner">Failed to load policies: {policiesError}</div>}
                {policiesLoading ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">Loading policies…</p>
                    <p className="text-sm text-[var(--text-secondary)]">Fetching the low-level rules behind each role bundle.</p>
                  </div>
                ) : visiblePolicies.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No policies yet</p>
                    <p className="text-sm text-[var(--text-secondary)]">Create a rule, then attach it to roles like viewer or developer.</p>
                  </div>
                ) : filteredPolicies.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No policies match this search</p>
                    <p className="text-sm text-[var(--text-secondary)]">Search by role, resource selector, action, or policy label.</p>
                  </div>
                ) : (
                  <div className="access-policy-stack">
                    {filteredPolicies.map(policy => {
                      const protectedPolicy = isProtectedAccessRole(policy.role);
                      const parsedAction = parseAAAActionValue(policy.act);
                      const preset = accessPresetForRole(policy.role);
                      const isSelected =
                        policyEditor?.original.role === policy.role &&
                        policyEditor?.original.obj === policy.obj &&
                        policyEditor?.original.act === policy.act;
                      return (
                        <article key={`${policy.role}-${policy.obj}-${policy.act}`} className={`access-card access-card--policy ${isSelected ? 'access-card--selected' : ''}`}>
                          <div className="access-card__header">
                            <div className="space-y-2 min-w-0">
                              <div className="flex items-center gap-2 flex-wrap">
                                <p className="access-card__title">{policyLabel(policy)}</p>
                                <span className={`access-chip ${accessPresetToneClass(policy.role)}`}>{policy.role}</span>
                                {parsedAction.effect === 'deny' && <span className="access-chip access-chip--danger">Deny</span>}
                                {protectedPolicy && <span className="access-chip access-chip--muted">Protected</span>}
                              </div>
                              <p className="access-card__subtitle">
                                {preset ? `${preset.label} role` : 'Role'} can {formatAccessActionSummary(policy.act)} on {formatAccessResourceSummary(policy.obj)}.
                              </p>
                            </div>
                            <div className="access-card__actions">
                              {protectedPolicy ? (
                                <span className="access-chip access-chip--muted">Protected</span>
                              ) : (
                                <>
                                  <button
                                    type="button"
                                    className="access-card-action"
                                    title="Edit policy"
                                    aria-label={`Edit ${policyLabel(policy)}`}
                                    onClick={() => openPolicyEditModal(policy)}
                                  >
                                    <EditIcon />
                                  </button>
                                  <button
                                    type="button"
                                    className="access-card-action access-card-action--danger"
                                    title="Delete policy"
                                    aria-label={`Delete ${policyLabel(policy)}`}
                                    onClick={() => confirmDeletePolicy(policy)}
                                  >
                                    <TrashIcon />
                                  </button>
                                </>
                              )}
                            </div>
                          </div>
                          <div className="access-policy-preview access-policy-preview--minimal">
                            <span className="access-policy-chip access-policy-chip--path">{policy.obj}</span>
                            <span className="access-policy-arrow">-&gt;</span>
                            <span className="access-policy-chip access-policy-chip--act">{policy.act}</span>
                          </div>
                        </article>
                      );
                    })}
                  </div>
                )}
              </div>
              <aside className="access-editor-pane">
                {policyEditor ? (
                  <div className="access-editor-surface access-editor-surface--minimal">
                    <div className="access-editor-header">
                      <div>
                        <p className="access-editor-kicker">Edit policy</p>
                        <h5 className="access-editor-title">{policyEditor.name || policyLabel(policyEditor)}</h5>
                        <p className="access-editor-text">Update this reusable rule.</p>
                      </div>
                      <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setPolicyEditor(null)}>
                        Close
                      </button>
                    </div>
                    <form className="access-editor-form access-editor-form--compact" onSubmit={handleSavePolicyEdit}>
                      <AAAPolicyRuleFields
                        policy={policyEditor}
                        onChange={next => setPolicyEditor(prev => (prev ? { ...prev, ...next } : prev))}
                        resourceCatalog={resourceCatalog}
                      />
                      <div className="access-editor-footer access-editor-footer--inline">
                        <button type="submit" className="glass-button-primary" disabled={savingPolicy}>
                          {savingPolicy ? 'Saving…' : 'Save changes'}
                        </button>
                      </div>
                    </form>
                  </div>
                ) : showPolicyModal ? (
                  <div className="access-editor-surface access-editor-surface--minimal">
                    <div className="access-editor-header">
                      <div>
                        <p className="access-editor-kicker">Create policy</p>
                        <h5 className="access-editor-title">Reusable AAA rule</h5>
                        <p className="access-editor-text">Define a reusable rule.</p>
                      </div>
                      <button
                        type="button"
                        className="access-inline-btn access-inline-btn--pill"
                        onClick={() => {
                          setAwaitingPolicyCreateReset(false);
                          setShowPolicyModal(false);
                        }}
                      >
                        Close
                      </button>
                    </div>
                    <form className="access-editor-form access-editor-form--compact" onSubmit={handleCreatePolicyInline}>
                      <AAAPolicyRuleFields policy={newPermission} onChange={onChangePermission} resourceCatalog={resourceCatalog} />
                      <div className="access-editor-footer access-editor-footer--inline">
                        <button type="submit" className="glass-button-primary" disabled={creatingPolicyInline}>
                          {creatingPolicyInline ? 'Adding…' : 'Add policy'}
                        </button>
                      </div>
                    </form>
                  </div>
                ) : (
                  <AccessEditorEmptyState sectionLabel="Policy details" hint="Select a policy to edit rules." />
                )}
              </aside>
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
    </div>
  );
}

function AccessEditorEmptyState({
  sectionLabel,
  hint,
  actionLabel,
  onAction,
}: {
  sectionLabel: string;
  hint: string;
  actionLabel?: string;
  onAction?: () => void;
}) {
  return (
    <div className="access-editor-empty">
      <div className="access-editor-empty__meta">
        <span className="access-editor-empty__badge">{sectionLabel}</span>
        <p className="access-editor-empty__hint">{hint}</p>
      </div>
      {actionLabel && onAction && (
        <div className="access-editor-empty__footer">
          <button type="button" className="glass-button-primary access-editor-empty__button" onClick={onAction}>
            <PlusIcon />
            <span>{actionLabel}</span>
          </button>
        </div>
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
          <button className={minimal ? 'access-inline-btn' : 'glass-button-ghost'} onClick={onClose} aria-label="Close">
            <X className="h-4 w-4" />
            {!minimal && <span>Close</span>}
          </button>
        </header>
        <div className={`pipelines-modal-body access-modal-body ${minimal ? 'access-modal-body--minimal' : ''}`}>{children}</div>
      </div>
    </div>
  );
}

function PlusIcon() {
  return <Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />;
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}

function EditIcon() {
  return <Edit3 className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function RefreshIcon() {
  return <RefreshCw className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function SearchIcon() {
  return <Search className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

type LLMProfileRecord = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  api_key_secret: string;
  allowed_scopes: string[];
  reasoning: string;
  thinking?: boolean;
  status: string;
  validation?: string;
  references?: string[];
  allowed_in_scope?: boolean;
  disabled_reason?: string;
};

type LLMProfilesPayload = {
  default_profile: string;
  profiles: LLMProfileRecord[];
};

type LLMProfileFormState = {
  name: string;
  provider: string;
  model: string;
  base_url: string;
  api_key_secret: string;
  allowed_scopes: string;
  reasoning: string;
  thinking: 'default' | 'true' | 'false';
};

type LLMProfilePanelMode = 'create' | 'edit' | 'delete';

const emptyLLMProfileForm: LLMProfileFormState = {
  name: '',
  provider: 'gemini',
  model: '',
  base_url: '',
  api_key_secret: 'GEMINI_API_KEY',
  allowed_scopes: '',
  reasoning: '',
  thinking: 'default',
};

function LLMProfilesPanel({ canManage }: { canManage: boolean }) {
  const [payload, setPayload] = useState<LLMProfilesPayload>({ default_profile: 'standard', profiles: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [form, setForm] = useState<LLMProfileFormState>(emptyLLMProfileForm);
  const [deleteBlocker, setDeleteBlocker] = useState<{ name: string; references: string[]; migrateTo: string } | null>(null);
  const [panelMode, setPanelMode] = useState<LLMProfilePanelMode | null>(null);
  const profilePanelRef = useRef<HTMLElement | null>(null);

  const loadProfiles = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/system/llm-profiles'), { cache: 'no-store' });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load LLM profiles (${response.status})`);
      }
      setPayload(normalizeLLMProfilesPayload(await response.json()));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load LLM profiles');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadProfiles();
  }, [loadProfiles]);

  useEffect(() => {
    if (!panelMode) return;
    window.requestAnimationFrame(() => {
      if (window.innerWidth < 1280) {
        profilePanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
      const focusTarget =
        profilePanelRef.current?.querySelector<HTMLElement>('[data-profile-autofocus]:not(:disabled)') ??
        profilePanelRef.current?.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), button:not(:disabled)');
      focusTarget?.focus({ preventScroll: true });
    });
  }, [editingName, panelMode]);

  const startCreate = () => {
    setEditingName(null);
    setForm(emptyLLMProfileForm);
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('create');
  };

  const startEdit = (profile: LLMProfileRecord) => {
    setEditingName(profile.name);
    setForm({
      name: profile.name,
      provider: profile.provider || 'gemini',
      model: profile.model || '',
      base_url: profile.base_url || '',
      api_key_secret: profile.api_key_secret || '',
      allowed_scopes: (profile.allowed_scopes || []).join(', '),
      reasoning: profile.reasoning || '',
      thinking: profile.thinking === undefined ? 'default' : profile.thinking ? 'true' : 'false',
    });
    setDeleteBlocker(null);
    setTestResult(null);
    setPanelMode('edit');
  };

  const formToPayload = () => ({
    name: form.name.trim(),
    provider: form.provider.trim(),
    model: form.model.trim(),
    base_url: form.base_url.trim(),
    api_key_secret: form.api_key_secret.trim(),
    allowed_scopes: form.allowed_scopes.split(',').map(item => item.trim()).filter(Boolean),
    reasoning: form.reasoning.trim(),
    thinking: form.provider.trim() === 'lmstudio' && form.thinking !== 'default' ? form.thinking === 'true' : undefined,
  });

  const saveProfile = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    const next = formToPayload();
    if (!next.name) {
      setError('Profile name is required.');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/llm-profiles/${encodeURIComponent(next.name)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(next),
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to save LLM profile (${response.status})`);
      }
      setPayload(normalizeLLMProfilesPayload(await response.json()));
      setEditingName(next.name);
      setPanelMode('edit');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save LLM profile');
    } finally {
      setSaving(false);
    }
  };

  const saveDefaultProfile = async (nextDefault: string) => {
    if (!canManage || !nextDefault) return;
    setSaving(true);
    setError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/system/llm-profiles'), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          default_profile: nextDefault,
          profiles: payload.profiles,
        }),
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to update default profile (${response.status})`);
      }
      setPayload(normalizeLLMProfilesPayload(await response.json()));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to update default profile');
    } finally {
      setSaving(false);
    }
  };

  const deleteProfile = async (name: string, opts?: { force?: boolean; migrateTo?: string }) => {
    if (!canManage) return;
    setSaving(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (opts?.force) params.set('force', 'true');
      if (opts?.migrateTo) params.set('migrate_to', opts.migrateTo);
      const suffix = params.toString() ? `?${params.toString()}` : '';
      const response = await fetch(buildApiUrl(`/v1/system/llm-profiles/${encodeURIComponent(name)}${suffix}`), { method: 'DELETE' });
      if (response.status === 409) {
        const conflict = await response.json().catch(() => null);
        const references = Array.isArray(conflict?.references) ? conflict.references.map((item: unknown) => String(item)) : [];
        const fallback = payload.profiles.find(profile => profile.name !== name)?.name || '';
        setDeleteBlocker({ name, references, migrateTo: fallback });
        setPanelMode('delete');
        return;
      }
      if (!response.ok && response.status !== 204) {
        const text = await response.text();
        throw new Error(text || `Failed to delete LLM profile (${response.status})`);
      }
      setDeleteBlocker(null);
      if (editingName === name) {
        setEditingName(null);
        setForm(emptyLLMProfileForm);
      }
      setPanelMode(prev => (prev === 'delete' || editingName === name ? null : prev));
      await loadProfiles();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete LLM profile');
    } finally {
      setSaving(false);
    }
  };

  const testProfile = async (name: string) => {
    setTesting(name);
    setTestResult(null);
    setError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/llm-profiles/${encodeURIComponent(name)}/test`), { method: 'POST' });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Profile test failed (${response.status})`);
      }
      const result = await response.json();
      setTestResult(`${name}: ${readString(result?.reply) || 'ok'}`);
    } catch (err) {
      setTestResult(`${name}: ${err instanceof Error ? err.message : 'test failed'}`);
    } finally {
      setTesting(null);
    }
  };

  const providerOptions = ['gemini', 'lmstudio'];
  const canDelete = (profile: LLMProfileRecord) => canManage && profile.name !== payload.default_profile;
  const migrationTargets = payload.profiles.filter(profile => profile.name !== deleteBlocker?.name).map(profile => profile.name);
  const showProfilePanel = panelMode !== null || deleteBlocker !== null;
  const showProfileForm = panelMode === 'create' || panelMode === 'edit';

  return (
    <div id="system-llm-profiles-section" className="space-y-6 pb-24">
      <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <label className="flex flex-col gap-1 text-sm min-w-[220px]">
            <span>Default profile</span>
            <select
              className="pipelines-input"
              value={payload.default_profile}
              onChange={event => void saveDefaultProfile(event.target.value)}
              disabled={!canManage || loading || saving}
            >
              {payload.profiles.map(profile => (
                <option key={profile.name} value={profile.name}>
                  {profile.name}
                </option>
              ))}
            </select>
          </label>
          <div className="flex items-center gap-2">
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="glass-button-ghost" onClick={() => void loadProfiles()} disabled={loading || saving}>
              <RefreshIcon />
              Reload
            </button>
            {canManage && (
              <button type="button" className="glass-button-primary" onClick={startCreate} disabled={saving}>
                <PlusIcon />
                New profile
              </button>
            )}
          </div>
        </div>
        {error && <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">{error}</div>}
        {testResult && <div className="rounded-lg border border-[var(--border-primary)] px-4 py-3 text-sm text-[var(--text-secondary)]">{testResult}</div>}
      </section>

      <div className={`grid gap-6 ${showProfilePanel ? 'xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]' : ''}`}>
        <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
          <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between gap-3">
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Profiles</h3>
            {loading && <span className="text-sm text-[var(--text-secondary)]">Loading…</span>}
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
                <tr>
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Provider</th>
                  <th className="px-4 py-3">Model</th>
                  <th className="px-4 py-3">Base URL</th>
                  <th className="px-4 py-3">API key secret</th>
                  <th className="px-4 py-3">Allowed scopes</th>
                  <th className="px-4 py-3">Thinking</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {payload.profiles.map(profile => (
                  <tr key={profile.name} className="border-b border-[var(--border-primary)] last:border-b-0">
                    <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">
                      {profile.name}
                      {profile.name === payload.default_profile && <span className="ml-2 runner-pill runner-pill--ok">Default</span>}
                    </td>
                    <td className="px-4 py-3">{profile.provider || '-'}</td>
                    <td className="px-4 py-3 max-w-[220px] truncate" title={profile.model}>{profile.model || '-'}</td>
                    <td className="px-4 py-3 max-w-[220px] truncate" title={profile.base_url}>{profile.base_url || '-'}</td>
                    <td className="px-4 py-3">{profile.api_key_secret || '-'}</td>
                    <td className="px-4 py-3">{profile.allowed_scopes.length ? profile.allowed_scopes.join(', ') : 'All'}</td>
                    <td className="px-4 py-3">
                      {profile.reasoning || (() => {
                        if (profile.thinking === undefined) return 'Default';
                        return profile.thinking ? 'On' : 'Off';
                      })()}
                    </td>
                    <td className="px-4 py-3">
                      <div className="space-y-1">
                        <span className={`runner-pill ${profile.status === 'valid' ? 'runner-pill--ok' : 'runner-pill--error'}`} title={profile.validation || profile.disabled_reason || ''}>
                          {profile.status || 'unknown'}
                        </span>
                        {(profile.validation || profile.disabled_reason) && (
                          <p className="text-xs text-[var(--text-secondary)] max-w-[220px]">{profile.validation || profile.disabled_reason}</p>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <button type="button" className="glass-button-ghost" onClick={() => void testProfile(profile.name)} disabled={Boolean(testing)}>
                          {testing === profile.name ? 'Testing…' : 'Test'}
                        </button>
                        <button type="button" className="glass-button-subtle" onClick={() => startEdit(profile)}>
                          <EditIcon />
                          Edit
                        </button>
                        <button type="button" className="glass-button-danger" onClick={() => void deleteProfile(profile.name)} disabled={!canDelete(profile) || saving}>
                          <TrashIcon />
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
                {!loading && payload.profiles.length === 0 && (
                  <tr>
                    <td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={9}>
                      No LLM profiles configured.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        {showProfilePanel && (
          <aside ref={profilePanelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
            {showProfileForm && (
              <>
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-xs text-[var(--text-secondary)]">{panelMode === 'edit' ? 'Edit profile' : 'Create profile'}</p>
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">{panelMode === 'edit' ? editingName : 'New LLM profile'}</h3>
                  </div>
                  <button type="button" className="glass-button-ghost !px-2" aria-label="Close profile form" onClick={() => setPanelMode(null)}>
                    <X className="h-4 w-4" aria-hidden="true" />
                  </button>
                </div>
                <form className="space-y-4" onSubmit={saveProfile}>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Name</span>
                    <input data-profile-autofocus className="pipelines-input" value={form.name} onChange={event => setForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingName)} placeholder="reasoning" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Provider</span>
                    <select
                      className="pipelines-input"
                      value={form.provider}
                      onChange={event => setForm(prev => ({
                        ...prev,
                        provider: event.target.value,
                        api_key_secret: event.target.value === 'gemini' && !prev.api_key_secret ? 'GEMINI_API_KEY' : prev.api_key_secret,
                        thinking: event.target.value === 'lmstudio' ? prev.thinking : 'default',
                      }))}
                      disabled={!canManage}
                    >
                      {providerOptions.map(provider => <option key={provider} value={provider}>{provider}</option>)}
                    </select>
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Model</span>
                    <input className="pipelines-input" value={form.model} onChange={event => setForm(prev => ({ ...prev, model: event.target.value }))} disabled={!canManage} placeholder={form.provider === 'gemini' ? 'gemini-2.5-pro' : 'qwen3-coder'} />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Base URL</span>
                    <input className="pipelines-input" value={form.base_url} onChange={event => setForm(prev => ({ ...prev, base_url: event.target.value }))} disabled={!canManage} placeholder="http://lmstudio:1234" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>API key secret</span>
                    <input className="pipelines-input" value={form.api_key_secret} onChange={event => setForm(prev => ({ ...prev, api_key_secret: event.target.value }))} disabled={!canManage} placeholder="GEMINI_API_KEY" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Allowed scopes</span>
                    <input className="pipelines-input" value={form.allowed_scopes} onChange={event => setForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, internal" />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Reasoning</span>
                    <select className="pipelines-input" value={form.reasoning} onChange={event => setForm(prev => ({ ...prev, reasoning: event.target.value }))} disabled={!canManage}>
                      <option value="">Provider default</option>
                      <option value="off">Off</option>
                      <option value="low">Low</option>
                      <option value="medium">Medium</option>
                      <option value="high">High</option>
                      <option value="on">On</option>
                    </select>
                  </label>
                  {form.provider === 'lmstudio' && (
                    <label className="flex flex-col gap-1 text-sm">
                      <span>Thinking</span>
                      <select className="pipelines-input" value={form.thinking} onChange={event => setForm(prev => ({ ...prev, thinking: event.target.value as LLMProfileFormState['thinking'] }))} disabled={!canManage}>
                        <option value="default">Provider default</option>
                        <option value="true">True</option>
                        <option value="false">False</option>
                      </select>
                    </label>
                  )}
                  <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>
                    {saving ? 'Saving…' : 'Save profile'}
                  </button>
                </form>
              </>
            )}

            {deleteBlocker && panelMode === 'delete' && (
              <div className="space-y-3 text-sm">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-xs text-[var(--text-secondary)]">Delete profile</p>
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">{deleteBlocker.name}</h3>
                  </div>
                  <button type="button" className="glass-button-ghost !px-2" aria-label="Close delete details" onClick={() => { setDeleteBlocker(null); setPanelMode(null); }}>
                    <X className="h-4 w-4" aria-hidden="true" />
                  </button>
                </div>
                <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 space-y-3">
                  <p className="font-semibold text-[var(--text-primary)]">Profile is still referenced.</p>
                  <ul className="list-disc pl-5 text-[var(--text-secondary)] max-h-32 overflow-auto">
                    {deleteBlocker.references.map(ref => <li key={ref}>{ref}</li>)}
                  </ul>
                  <label className="flex flex-col gap-1">
                    <span>Migrate references to</span>
                    <select data-profile-autofocus className="pipelines-input" value={deleteBlocker.migrateTo} onChange={event => setDeleteBlocker(prev => prev ? { ...prev, migrateTo: event.target.value } : prev)}>
                      {migrationTargets.map(name => <option key={name} value={name}>{name}</option>)}
                    </select>
                  </label>
                  <button type="button" className="glass-button-danger" disabled={!deleteBlocker.migrateTo || saving} onClick={() => void deleteProfile(deleteBlocker.name, { force: true, migrateTo: deleteBlocker.migrateTo })}>
                    <TrashIcon />
                    Force delete with migration
                  </button>
                </div>
              </div>
            )}
          </aside>
        )}
      </div>
    </div>
  );
}

type MCPToolRecord = {
  server_name: string;
  name: string;
  description?: string;
  input_schema?: string;
  schema_hash?: string;
  last_seen_at?: string;
};

type MCPServerRecord = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  auth_secret: string;
  headers: Record<string, string>;
  timeout: string;
  allowed_scopes: string[];
  last_test_status?: string;
  last_test_message?: string;
  last_tested_at?: string;
  last_discovered_at?: string;
  discovered_server_name?: string;
  discovered_version?: string;
  discovered_protocol?: string;
  tools: MCPToolRecord[];
};

type MCPProfileServerRef = {
  server: string;
  tools: string[];
};

type MCPProfileRecord = {
  name: string;
  description: string;
  enabled: boolean;
  servers: MCPProfileServerRef[];
  allowed_scopes: string[];
};

type MCPServerFormState = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  auth_secret: string;
  headers_json: string;
  timeout: string;
  allowed_scopes: string;
};

type MCPProfileFormState = {
  name: string;
  description: string;
  enabled: boolean;
  selected_tools: Record<string, string[]>;
  tool_text: Record<string, string>;
  allowed_scopes: string;
};

type MCPPanelMode = 'server-create' | 'server-edit' | 'profile-create' | 'profile-edit';

const emptyMCPServerForm: MCPServerFormState = {
  name: '',
  display_name: '',
  enabled: true,
  provider: '',
  transport: 'streamable_http',
  url: '',
  auth_type: 'none',
  auth_secret: '',
  headers_json: '',
  timeout: '30s',
  allowed_scopes: '',
};

const emptyMCPProfileForm: MCPProfileFormState = {
  name: '',
  description: '',
  enabled: true,
  selected_tools: {},
  tool_text: {},
  allowed_scopes: '',
};

function MCPPanel({ canManage }: { canManage: boolean }) {
  const [innerTab, setInnerTab] = useState<'servers' | 'profiles'>('servers');
  const [servers, setServers] = useState<MCPServerRecord[]>([]);
  const [profiles, setProfiles] = useState<MCPProfileRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [serverForm, setServerForm] = useState<MCPServerFormState>(emptyMCPServerForm);
  const [profileForm, setProfileForm] = useState<MCPProfileFormState>(emptyMCPProfileForm);
  const [editingServer, setEditingServer] = useState<string | null>(null);
  const [editingProfile, setEditingProfile] = useState<string | null>(null);
  const [panelMode, setPanelMode] = useState<MCPPanelMode | null>(null);
  const mcpPanelRef = useRef<HTMLElement | null>(null);

  const loadMCP = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [serversResp, profilesResp] = await Promise.all([
        fetch(buildApiUrl('/v1/system/mcp/servers'), { cache: 'no-store' }),
        fetch(buildApiUrl('/v1/system/mcp/profiles'), { cache: 'no-store' }),
      ]);
      if (!serversResp.ok) throw new Error(await serversResp.text() || `Failed to load MCP servers (${serversResp.status})`);
      if (!profilesResp.ok) throw new Error(await profilesResp.text() || `Failed to load MCP profiles (${profilesResp.status})`);
      setServers(normalizeMCPServersPayload(await serversResp.json()));
      setProfiles(normalizeMCPProfilesPayload(await profilesResp.json()));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load MCP registry');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadMCP();
  }, [loadMCP]);

  useEffect(() => {
    if (!panelMode) return;
    window.requestAnimationFrame(() => {
      if (window.innerWidth < 1280) {
        mcpPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
      const focusTarget =
        mcpPanelRef.current?.querySelector<HTMLElement>('[data-mcp-autofocus]:not(:disabled)') ??
        mcpPanelRef.current?.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), textarea:not(:disabled), button:not(:disabled)');
      focusTarget?.focus({ preventScroll: true });
    });
  }, [editingProfile, editingServer, innerTab, panelMode]);

  const startServerCreate = () => {
    setEditingServer(null);
    setServerForm(emptyMCPServerForm);
    setInnerTab('servers');
    setPanelMode('server-create');
  };

  const startServerEdit = (server: MCPServerRecord) => {
    setEditingServer(server.name);
    setServerForm({
      name: server.name,
      display_name: server.display_name || '',
      enabled: server.enabled,
      provider: server.provider || '',
      transport: server.transport || 'streamable_http',
      url: server.url || '',
      auth_type: server.auth_type || 'none',
      auth_secret: server.auth_secret || '',
      headers_json: formatHeadersJSON(server.headers),
      timeout: server.timeout || '30s',
      allowed_scopes: server.allowed_scopes.join(', '),
    });
    setInnerTab('servers');
    setPanelMode('server-edit');
  };

  const saveServer = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    const headers = parseHeadersJSON(serverForm.headers_json);
    if (headers == null) {
      setError('MCP server headers must be a JSON object with string keys and values.');
      return;
    }
    const payload = {
      name: serverForm.name.trim(),
      display_name: serverForm.display_name.trim(),
      enabled: serverForm.enabled,
      provider: serverForm.provider.trim(),
      transport: serverForm.transport.trim(),
      url: serverForm.url.trim(),
      auth_type: serverForm.auth_type.trim(),
      auth_secret: serverForm.auth_secret.trim(),
      headers,
      timeout: serverForm.timeout.trim() || '30s',
      allowed_scopes: splitCSV(serverForm.allowed_scopes),
    };
    if (!payload.name) {
      setError('MCP server name is required.');
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const path = editingServer ? `/v1/system/mcp/servers/${encodeURIComponent(payload.name)}` : '/v1/system/mcp/servers';
      const response = await fetch(buildApiUrl(path), {
        method: editingServer ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!response.ok) throw new Error(await response.text() || `Failed to save MCP server (${response.status})`);
      setServers(normalizeMCPServersPayload(await response.json()));
      setEditingServer(payload.name);
      setPanelMode('server-edit');
      setMessage(`Saved MCP server ${payload.name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save MCP server');
    } finally {
      setSaving(false);
    }
  };

  const deleteServer = async (name: string) => {
    if (!canManage) return;
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/servers/${encodeURIComponent(name)}`), { method: 'DELETE' });
      if (!response.ok && response.status !== 204) throw new Error(await response.text() || `Failed to delete MCP server (${response.status})`);
      if (editingServer === name) {
        setEditingServer(null);
        setServerForm(emptyMCPServerForm);
        setPanelMode(prev => prev === 'server-edit' ? null : prev);
      }
      await loadMCP();
      setMessage(`Deleted MCP server ${name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete MCP server');
    } finally {
      setSaving(false);
    }
  };

  const discoverServer = async (name: string) => {
    setTesting(name);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/servers/${encodeURIComponent(name)}/discover-tools`), { method: 'POST' });
      if (!response.ok) throw new Error(await response.text() || `MCP discovery failed (${response.status})`);
      await loadMCP();
      setMessage(`Discovered tools for ${name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to discover MCP tools');
    } finally {
      setTesting(null);
    }
  };

  const startProfileCreate = () => {
    setEditingProfile(null);
    setProfileForm(emptyMCPProfileForm);
    setInnerTab('profiles');
    setPanelMode('profile-create');
  };

  const startProfileEdit = (profile: MCPProfileRecord) => {
    const selectedTools: Record<string, string[]> = {};
    const toolText: Record<string, string> = {};
    profile.servers.forEach(ref => {
      selectedTools[ref.server] = [...ref.tools];
      toolText[ref.server] = ref.tools.join('\n');
    });
    setEditingProfile(profile.name);
    setProfileForm({
      name: profile.name,
      description: profile.description || '',
      enabled: profile.enabled,
      selected_tools: selectedTools,
      tool_text: toolText,
      allowed_scopes: profile.allowed_scopes.join(', '),
    });
    setInnerTab('profiles');
    setPanelMode('profile-edit');
  };

  const toggleProfileTool = (serverName: string, toolName: string) => {
    setProfileForm(prev => {
      const current = new Set(prev.selected_tools[serverName] || []);
      if (current.has(toolName)) current.delete(toolName);
      else current.add(toolName);
      const next = { ...prev.selected_tools, [serverName]: Array.from(current).sort((a, b) => a.localeCompare(b)) };
      if (next[serverName].length === 0) delete next[serverName];
      const toolText = { ...prev.tool_text, [serverName]: (next[serverName] || []).join('\n') };
      if (!next[serverName]) delete toolText[serverName];
      return { ...prev, selected_tools: next, tool_text: toolText };
    });
  };

  const setProfileServerTools = (serverName: string, value: string) => {
    setProfileForm(prev => {
      const tools = splitToolNames(value);
      const next = { ...prev.selected_tools };
      if (tools.length > 0) next[serverName] = tools;
      else delete next[serverName];
      return { ...prev, selected_tools: next, tool_text: { ...prev.tool_text, [serverName]: value } };
    });
  };

  const saveProfile = async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    const refs = Object.entries(profileForm.selected_tools)
      .filter(([, tools]) => tools.length > 0)
      .map(([server, tools]) => ({ server, tools }));
    const payload = {
      name: profileForm.name.trim(),
      description: profileForm.description.trim(),
      enabled: profileForm.enabled,
      servers: refs,
      allowed_scopes: splitCSV(profileForm.allowed_scopes),
    };
    if (!payload.name) {
      setError('MCP profile name is required.');
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const path = editingProfile ? `/v1/system/mcp/profiles/${encodeURIComponent(payload.name)}` : '/v1/system/mcp/profiles';
      const response = await fetch(buildApiUrl(path), {
        method: editingProfile ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!response.ok) throw new Error(await response.text() || `Failed to save MCP profile (${response.status})`);
      setProfiles(normalizeMCPProfilesPayload(await response.json()));
      setEditingProfile(payload.name);
      setPanelMode('profile-edit');
      setMessage(`Saved MCP profile ${payload.name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to save MCP profile');
    } finally {
      setSaving(false);
    }
  };

  const deleteProfile = async (name: string) => {
    if (!canManage) return;
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/profiles/${encodeURIComponent(name)}`), { method: 'DELETE' });
      if (!response.ok && response.status !== 204) throw new Error(await response.text() || `Failed to delete MCP profile (${response.status})`);
      if (editingProfile === name) {
        setEditingProfile(null);
        setProfileForm(emptyMCPProfileForm);
        setPanelMode(prev => prev === 'profile-edit' ? null : prev);
      }
      await loadMCP();
      setMessage(`Deleted MCP profile ${name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete MCP profile');
    } finally {
      setSaving(false);
    }
  };

  const testProfile = async (name: string) => {
    setTesting(name);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/mcp/profiles/${encodeURIComponent(name)}/test`), { method: 'POST' });
      if (!response.ok) throw new Error(await response.text() || `MCP profile test failed (${response.status})`);
      const result = await response.json();
      const warnings = normalizeStringArray(asRecord(result)?.warnings);
      setMessage(warnings.length ? `${name}: ${warnings.join('; ')}` : `${name}: ${readString(asRecord(result)?.message) || 'ok'}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to test MCP profile');
    } finally {
      setTesting(null);
    }
  };

  const showServerPanel = panelMode === 'server-create' || panelMode === 'server-edit';
  const showProfilePanel = panelMode === 'profile-create' || panelMode === 'profile-edit';

  return (
    <div id="system-mcp-section" className="space-y-6 pb-24">
      <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex rounded-lg border border-[var(--border-primary)] overflow-hidden w-fit">
            <button type="button" className={`px-4 py-2 text-sm ${innerTab === 'servers' ? 'bg-[var(--surface-elevated)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'}`} onClick={() => setInnerTab('servers')}>
              MCP Servers
            </button>
            <button type="button" className={`px-4 py-2 text-sm border-l border-[var(--border-primary)] ${innerTab === 'profiles' ? 'bg-[var(--surface-elevated)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'}`} onClick={() => setInnerTab('profiles')}>
              MCP Profiles
            </button>
          </div>
          <div className="flex items-center gap-2">
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="glass-button-ghost" onClick={() => void loadMCP()} disabled={loading || saving}>
              <RefreshIcon />
              Reload
            </button>
            {canManage && innerTab === 'servers' && (
              <button type="button" className="glass-button-primary" onClick={startServerCreate} disabled={saving}>
                <PlusIcon />
                New server
              </button>
            )}
            {canManage && innerTab === 'profiles' && (
              <button type="button" className="glass-button-primary" onClick={startProfileCreate} disabled={saving}>
                <PlusIcon />
                New profile
              </button>
            )}
          </div>
        </div>
        {error && <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500 whitespace-pre-wrap">{error}</div>}
        {message && <div className="rounded-lg border border-[var(--border-primary)] px-4 py-3 text-sm text-[var(--text-secondary)]">{message}</div>}
      </section>

      {innerTab === 'servers' ? (
        <div className={`grid gap-6 ${showServerPanel ? 'xl:grid-cols-[minmax(0,1.3fr)_minmax(360px,0.7fr)]' : ''}`}>
          <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
            <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Servers</h3>
              {loading && <span className="text-sm text-[var(--text-secondary)]">Loading…</span>}
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
                  <tr>
                    <th className="px-4 py-3">Name</th>
                    <th className="px-4 py-3">Provider</th>
                    <th className="px-4 py-3">URL</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3">Tools</th>
                    <th className="px-4 py-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {servers.map(server => (
                    <tr key={server.name} className="border-b border-[var(--border-primary)] last:border-b-0">
                      <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">
                        {server.display_name || server.name}
                        <div className="text-xs text-[var(--text-secondary)]">{server.name}</div>
                      </td>
                      <td className="px-4 py-3">{server.provider || '-'}</td>
                      <td className="px-4 py-3 max-w-[260px] truncate" title={server.url}>{server.url || '-'}</td>
                      <td className="px-4 py-3">
                        <span className={`runner-pill ${server.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>{server.enabled ? 'Enabled' : 'Disabled'}</span>
                        {server.last_test_status && <div className="text-xs text-[var(--text-secondary)] mt-1">{server.last_test_status}</div>}
                      </td>
                      <td className="px-4 py-3">{server.tools.length}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <button type="button" className="glass-button-ghost" onClick={() => void discoverServer(server.name)} disabled={Boolean(testing)}>
                            {testing === server.name ? 'Testing…' : 'Discover'}
                          </button>
                          <button type="button" className="glass-button-subtle" onClick={() => startServerEdit(server)}>
                            <EditIcon />
                            Edit
                          </button>
                          <button type="button" className="glass-button-danger" onClick={() => void deleteServer(server.name)} disabled={!canManage || saving}>
                            <TrashIcon />
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!loading && servers.length === 0 && (
                    <tr><td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={6}>No MCP servers configured.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>

          {showServerPanel && (
            <aside ref={mcpPanelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs text-[var(--text-secondary)]">{panelMode === 'server-edit' ? 'Edit server' : 'Create server'}</p>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">{panelMode === 'server-edit' ? editingServer : 'New MCP server'}</h3>
                </div>
                <button type="button" className="glass-button-ghost !px-2" aria-label="Close server form" onClick={() => setPanelMode(null)}>
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              <form className="space-y-4" onSubmit={saveServer}>
                <label className="flex flex-col gap-1 text-sm"><span>Name</span><input data-mcp-autofocus className="pipelines-input" value={serverForm.name} onChange={event => setServerForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingServer)} placeholder="github" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Display name</span><input className="pipelines-input" value={serverForm.display_name} onChange={event => setServerForm(prev => ({ ...prev, display_name: event.target.value }))} disabled={!canManage} placeholder="GitHub MCP" /></label>
                <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={serverForm.enabled} onChange={event => setServerForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} /> Enabled</label>
                <label className="flex flex-col gap-1 text-sm"><span>Provider</span><input className="pipelines-input" value={serverForm.provider} onChange={event => setServerForm(prev => ({ ...prev, provider: event.target.value }))} disabled={!canManage} placeholder="github" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Transport</span><select className="pipelines-input" value={serverForm.transport} onChange={event => setServerForm(prev => ({ ...prev, transport: event.target.value }))} disabled={!canManage}><option value="streamable_http">streamable_http</option><option value="http">http</option></select></label>
                <label className="flex flex-col gap-1 text-sm"><span>URL</span><input className="pipelines-input" value={serverForm.url} onChange={event => setServerForm(prev => ({ ...prev, url: event.target.value }))} disabled={!canManage} placeholder="https://api.githubcopilot.com/mcp/x/all/readonly" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Auth type</span><select className="pipelines-input" value={serverForm.auth_type} onChange={event => setServerForm(prev => ({ ...prev, auth_type: event.target.value }))} disabled={!canManage}><option value="none">none</option><option value="bearer_token">bearer_token</option></select></label>
                <label className="flex flex-col gap-1 text-sm"><span>Secret reference</span><input className="pipelines-input" value={serverForm.auth_secret} onChange={event => setServerForm(prev => ({ ...prev, auth_secret: event.target.value }))} disabled={!canManage} placeholder="GITHUB_MCP_TOKEN" /></label>
                <div className="rounded-lg border border-[var(--border-primary)] p-3 space-y-3">
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Extra configuration</p>
                  <label className="flex flex-col gap-1 text-sm">
                    <span>Headers JSON</span>
                    <textarea
                      className="pipelines-input min-h-[112px] font-mono text-xs"
                      value={serverForm.headers_json}
                      onChange={event => setServerForm(prev => ({ ...prev, headers_json: event.target.value }))}
                      disabled={!canManage}
                      placeholder={'{"X-MCP-Toolsets":"repos,issues","X-MCP-Readonly":"true"}'}
                      spellCheck={false}
                    />
                  </label>
                </div>
                <label className="flex flex-col gap-1 text-sm"><span>Timeout</span><input className="pipelines-input" value={serverForm.timeout} onChange={event => setServerForm(prev => ({ ...prev, timeout: event.target.value }))} disabled={!canManage} placeholder="30s" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Allowed scopes</span><input className="pipelines-input" value={serverForm.allowed_scopes} onChange={event => setServerForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, prod" /></label>
                <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>{saving ? 'Saving…' : 'Save server'}</button>
              </form>
            </aside>
          )}
        </div>
      ) : (
        <div className={`grid gap-6 ${showProfilePanel ? 'xl:grid-cols-[minmax(0,1.2fr)_minmax(420px,0.8fr)]' : ''}`}>
          <section className="glass-card border border-[var(--border-primary)] rounded-xl overflow-hidden">
            <div className="p-4 border-b border-[var(--border-primary)] flex items-center justify-between">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Profiles</h3>
              {loading && <span className="text-sm text-[var(--text-secondary)]">Loading…</span>}
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="text-left text-xs uppercase text-[var(--text-secondary)] border-b border-[var(--border-primary)]">
                  <tr><th className="px-4 py-3">Name</th><th className="px-4 py-3">Servers</th><th className="px-4 py-3">Tools</th><th className="px-4 py-3">Status</th><th className="px-4 py-3 text-right">Actions</th></tr>
                </thead>
                <tbody>
                  {profiles.map(profile => (
                    <tr key={profile.name} className="border-b border-[var(--border-primary)] last:border-b-0">
                      <td className="px-4 py-3 font-semibold text-[var(--text-primary)]">{profile.name}<div className="text-xs text-[var(--text-secondary)]">{profile.description || '-'}</div></td>
                      <td className="px-4 py-3">{profile.servers.map(ref => ref.server).join(', ') || '-'}</td>
                      <td className="px-4 py-3">{profile.servers.reduce((total, ref) => total + ref.tools.length, 0)}</td>
                      <td className="px-4 py-3"><span className={`runner-pill ${profile.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>{profile.enabled ? 'Enabled' : 'Disabled'}</span></td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <button type="button" className="glass-button-ghost" onClick={() => void testProfile(profile.name)} disabled={Boolean(testing)}>{testing === profile.name ? 'Testing…' : 'Test'}</button>
                          <button type="button" className="glass-button-subtle" onClick={() => startProfileEdit(profile)}><EditIcon />Edit</button>
                          <button type="button" className="glass-button-danger" onClick={() => void deleteProfile(profile.name)} disabled={!canManage || saving}><TrashIcon />Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {!loading && profiles.length === 0 && (
                    <tr><td className="px-4 py-6 text-[var(--text-secondary)]" colSpan={5}>No MCP profiles configured.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>

          {showProfilePanel && (
            <aside ref={mcpPanelRef} className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs text-[var(--text-secondary)]">{panelMode === 'profile-edit' ? 'Edit profile' : 'Create profile'}</p>
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">{panelMode === 'profile-edit' ? editingProfile : 'New MCP profile'}</h3>
                </div>
                <button type="button" className="glass-button-ghost !px-2" aria-label="Close profile form" onClick={() => setPanelMode(null)}>
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              <form className="space-y-4" onSubmit={saveProfile}>
                <label className="flex flex-col gap-1 text-sm"><span>Name</span><input data-mcp-autofocus className="pipelines-input" value={profileForm.name} onChange={event => setProfileForm(prev => ({ ...prev, name: event.target.value }))} disabled={!canManage || Boolean(editingProfile)} placeholder="github-pr-review" /></label>
                <label className="flex flex-col gap-1 text-sm"><span>Description</span><input className="pipelines-input" value={profileForm.description} onChange={event => setProfileForm(prev => ({ ...prev, description: event.target.value }))} disabled={!canManage} placeholder="Read-only GitHub PR review tools" /></label>
                <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={profileForm.enabled} onChange={event => setProfileForm(prev => ({ ...prev, enabled: event.target.checked }))} disabled={!canManage} /> Enabled</label>
                <div className="space-y-3">
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Tools</p>
                  {servers.map(server => (
                    <div key={server.name} className="rounded-lg border border-[var(--border-primary)] p-3 space-y-2">
                      <div className="font-semibold text-sm text-[var(--text-primary)]">{server.display_name || server.name}</div>
                      <label className="flex flex-col gap-1 text-sm">
                        <span>Selected tools</span>
                        <textarea
                          className="pipelines-input min-h-[84px] font-mono text-xs"
                          value={profileForm.tool_text[server.name] ?? (profileForm.selected_tools[server.name] || []).join('\n')}
                          onChange={event => setProfileServerTools(server.name, event.target.value)}
                          disabled={!canManage}
                          placeholder={'*\nissues_list\nrepos_get'}
                          spellCheck={false}
                        />
                      </label>
                      {server.tools.length === 0 ? (
                        <p className="text-xs text-[var(--text-secondary)]">No discovered tools cached for this server.</p>
                      ) : (
                        <div className="grid gap-2">
                          {server.tools.map(tool => (
                            <label key={`${server.name}-${tool.name}`} className="flex items-start gap-2 text-sm">
                              <input type="checkbox" checked={(profileForm.selected_tools[server.name] || []).includes(tool.name)} onChange={() => toggleProfileTool(server.name, tool.name)} disabled={!canManage} />
                              <span><span className="font-mono">{tool.name}</span>{tool.description ? <span className="block text-xs text-[var(--text-secondary)]">{tool.description}</span> : null}</span>
                            </label>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
                <label className="flex flex-col gap-1 text-sm"><span>Allowed scopes</span><input className="pipelines-input" value={profileForm.allowed_scopes} onChange={event => setProfileForm(prev => ({ ...prev, allowed_scopes: event.target.value }))} disabled={!canManage} placeholder="dev, prod" /></label>
                <button type="submit" className="glass-button-primary w-full justify-center" disabled={!canManage || saving}>{saving ? 'Saving…' : 'Save profile'}</button>
              </form>
            </aside>
          )}
        </div>
      )}
    </div>
  );
}

function SystemConfig({
  config,
  envFilePath,
  configError,
  configLoading,
  saving,
  globalConfigRepo,
  globalConfigRepoForm,
  globalConfigRepoLoading,
  globalConfigRepoSaving,
  globalConfigRepoSyncing,
  globalConfigRepoError,
  onChange,
  onReload,
  onSave,
  onGlobalConfigRepoChange,
  onSaveGlobalConfigRepo,
  onDeleteGlobalConfigRepo,
  onSyncGlobalConfigRepo,
  canViewRuntimeConfig,
  canManageRuntimeConfig,
  canViewGlobalConfigRepo,
  canManageGlobalConfigRepo,
}: {
  config: ConfigFormState;
  envFilePath: string;
  configError: string | null;
  configLoading: boolean;
  saving: boolean;
  globalConfigRepo: ConfigRepository | null;
  globalConfigRepoForm: ConfigRepositoryFormState;
  globalConfigRepoLoading: boolean;
  globalConfigRepoSaving: boolean;
  globalConfigRepoSyncing: boolean;
  globalConfigRepoError: string | null;
  onChange: (next: ConfigFormState) => void;
  onReload: () => Promise<void>;
  onSave: () => Promise<void>;
  onGlobalConfigRepoChange: Dispatch<SetStateAction<ConfigRepositoryFormState>>;
  onSaveGlobalConfigRepo: () => Promise<void>;
  onDeleteGlobalConfigRepo: () => Promise<void>;
  onSyncGlobalConfigRepo: () => Promise<void>;
  canViewRuntimeConfig: boolean;
  canManageRuntimeConfig: boolean;
  canViewGlobalConfigRepo: boolean;
  canManageGlobalConfigRepo: boolean;
}) {
  const envPath = (envFilePath || '').trim();
  const globalRepoRunning = globalConfigRepo?.last_sync_status === 'running';
  const globalRepoCanEdit = canManageGlobalConfigRepo && !globalConfigRepoLoading && !globalConfigRepoSaving;
  const globalRepoSyncDisabled = !globalConfigRepo || !canManageGlobalConfigRepo || globalConfigRepoSyncing || globalConfigRepoSaving || globalRepoRunning;

  const handleChange = (key: keyof ConfigFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onChange({ ...config, [key]: value } as ConfigFormState);
  };

  const handleGlobalRepoChange = (key: keyof ConfigRepositoryFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onGlobalConfigRepoChange(prev => ({ ...prev, [key]: value } as ConfigRepositoryFormState));
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    void onSave();
  };

  return (
    <div id="system-config-section" className="grid gap-6 lg:grid-cols-2 pb-24">
      {canViewRuntimeConfig && (
      <form id="system-config-form" className="space-y-4 lg:col-span-2" onSubmit={onSubmit}>
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
                disabled={!canManageRuntimeConfig || configLoading || saving}
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
                disabled={!canManageRuntimeConfig || configLoading || saving}
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
                  disabled={!canManageRuntimeConfig || configLoading || saving}
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
                  disabled={!canManageRuntimeConfig || configLoading || saving}
                />
              </label>
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                id="system-auto-remove"
                type="checkbox"
                checked={config.auto_removal_agent_container}
                onChange={handleChange('auto_removal_agent_container')}
                disabled={!canManageRuntimeConfig || configLoading || saving}
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
                disabled={!canManageRuntimeConfig || configLoading || saving}
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
                disabled={!canManageRuntimeConfig || configLoading || saving}
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
                disabled={!canManageRuntimeConfig || configLoading || saving}
              />
            </label>
          </div>
        </div>

        {envPath && <p className="text-xs text-[var(--text-secondary)]">Env file: {envPath}</p>}

        {configError && (
          <div className="glass-card p-4 border border-red-500/30 rounded-xl text-sm text-red-500">
            Failed to load or save config: {configError}
          </div>
        )}
        {configLoading && <p className="text-sm text-[var(--text-secondary)]">Loading settings…</p>}
      </form>
      )}

      {canViewGlobalConfigRepo && (
        <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4 lg:col-span-2">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">GitOps source</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Global config repository</h3>
            </div>
            {!canManageGlobalConfigRepo && <span className="runner-pill runner-pill--muted self-start">Read-only</span>}
          </div>

          {globalConfigRepoLoading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading global config repository…</p>
          ) : (
            <>
              {!globalConfigRepo && (
                <div className="rounded-lg border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
                  No global config repository connected.
                </div>
              )}

              {globalConfigRepo && (
                <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 text-sm">
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Status</p>
                      <p className="font-semibold text-[var(--text-primary)]">{globalConfigRepo.last_sync_status || 'Not synced'}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Completed</p>
                      <p className="font-semibold text-[var(--text-primary)]">{formatTimestamp(globalConfigRepo.last_sync_completed_at)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Started</p>
                      <p className="font-semibold text-[var(--text-primary)]">{formatTimestamp(globalConfigRepo.last_sync_started_at)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Commit</p>
                      <p className="font-semibold text-[var(--text-primary)] truncate" title={globalConfigRepo.last_sync_commit_sha || ''}>
                        {globalConfigRepo.last_sync_commit_sha || '-'}
                      </p>
                    </div>
                  </div>
                  {globalConfigRepo.last_sync_message && (
                    <p className="mt-3 text-xs text-[var(--text-secondary)] break-words">{globalConfigRepo.last_sync_message}</p>
                  )}
                </div>
              )}

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <label className="flex flex-col gap-1 text-sm md:col-span-2">
                  <span>Repository URL</span>
                  <input
                    id="system-global-config-repo-url"
                    type="url"
                    required={canManageGlobalConfigRepo}
                    className="pipelines-input"
                    value={globalConfigRepoForm.repo_url}
                    onChange={handleGlobalRepoChange('repo_url')}
                    placeholder="https://github.com/org/nopsai-config"
                    disabled={!globalRepoCanEdit}
                  />
                </label>
                <label className="flex flex-col gap-1 text-sm">
                  <span>Branch</span>
                  <input
                    id="system-global-config-repo-branch"
                    type="text"
                    className="pipelines-input"
                    value={globalConfigRepoForm.branch}
                    onChange={handleGlobalRepoChange('branch')}
                    placeholder="main"
                    disabled={!globalRepoCanEdit}
                  />
                </label>
                <label className="flex flex-col gap-1 text-sm">
                  <span>Base path</span>
                  <input
                    id="system-global-config-repo-base-path"
                    type="text"
                    className="pipelines-input"
                    value={globalConfigRepoForm.base_path}
                    onChange={handleGlobalRepoChange('base_path')}
                    placeholder="nopsai"
                    disabled={!globalRepoCanEdit}
                  />
                </label>
                <label className="flex items-center gap-2 text-sm md:col-span-2">
                  <input
                    id="system-global-config-repo-enabled"
                    type="checkbox"
                    checked={globalConfigRepoForm.enabled}
                    onChange={handleGlobalRepoChange('enabled')}
                    disabled={!globalRepoCanEdit}
                  />
                  <span>Enabled</span>
                </label>
              </div>

              {globalConfigRepo?.managed_by_config_repo && globalConfigRepo.config_source_path && (
                <p className="text-xs text-[var(--text-secondary)]">Managed by Git: {globalConfigRepo.config_source_path}</p>
              )}

              {globalConfigRepoError && (
                <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
                  {globalConfigRepoError}
                </div>
              )}

              <div className="flex flex-wrap items-center justify-end gap-2">
                {globalConfigRepo && canManageGlobalConfigRepo && (
                  <button type="button" className="glass-button-danger mr-auto" onClick={() => void onDeleteGlobalConfigRepo()} disabled={globalConfigRepoSaving || globalConfigRepoSyncing}>
                    Remove
                  </button>
                )}
                <button type="button" className="glass-button-subtle" onClick={() => void onSyncGlobalConfigRepo()} disabled={globalRepoSyncDisabled}>
                  {globalConfigRepoSyncing || globalRepoRunning ? 'Syncing…' : 'Sync'}
                </button>
                {canManageGlobalConfigRepo && (
                  <button type="button" className="glass-button-primary" onClick={() => void onSaveGlobalConfigRepo()} disabled={!globalRepoCanEdit}>
                    {globalConfigRepoSaving ? 'Saving…' : 'Save repository'}
                  </button>
                )}
              </div>
            </>
          )}
        </section>
      )}

      {canViewRuntimeConfig && (
      <div className="fixed bottom-6 right-6 z-40 flex items-center gap-2">
        <button className="glass-button-ghost" type="button" onClick={() => void onReload()} disabled={configLoading || saving}>
          Reload
        </button>
        <button className="glass-button-primary" type="button" onClick={() => void onSave()} disabled={!canManageRuntimeConfig || configLoading || saving}>
          {saving ? 'Saving…' : 'Save settings'}
        </button>
      </div>
      )}
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

      <div className="flex flex-col gap-3 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm font-semibold text-[var(--text-primary)]">Need another runner?</p>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">Open the deployment guide for Docker runners and the Kubernetes status.</p>
        </div>
        <Link
          to={`/system/dispatcher?${RUNNER_DEPLOYMENT_GUIDE_QUERY}=${RUNNER_DEPLOYMENT_GUIDE_VALUE}`}
          className="glass-button-primary self-start whitespace-nowrap sm:self-center"
          onClick={scrollRunnerDeploymentGuide}
        >
          <Plus className="h-4 w-4" />
          New runner guide
        </Link>
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

      <RunnerDeploymentGuide canManageDispatcher={canManageDispatcher} />
    </div>
  );
}

function RunnerDeploymentGuide({ canManageDispatcher }: { canManageDispatcher: boolean }) {
  const [runnerId, setRunnerId] = useState('runner-prod-1');
  const [runnerScopes, setRunnerScopes] = useState('prod');
  const [runnerCapacity, setRunnerCapacity] = useState('2');
  const [template, setTemplate] = useState<RunnerComposeTemplate | null>(null);
  const [loadingTemplate, setLoadingTemplate] = useState(false);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const loadTemplate = useCallback(async () => {
    if (!canManageDispatcher) return;
    const capacity = Number.parseInt(runnerCapacity, 10);
    if (!Number.isFinite(capacity) || capacity <= 0) {
      setTemplateError('Capacity must be a positive number.');
      return;
    }
    const params = new URLSearchParams({
      runner_id: runnerId.trim() || 'runner-prod-1',
      runner_scopes: runnerScopes.trim(),
      runner_capacity: String(capacity),
    });
    setLoadingTemplate(true);
    setTemplateError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/dispatcher/runner-bootstrap-command?${params.toString()}`), { cache: 'no-store' });
      if (!response.ok) throw new Error(await response.text() || `Unable to generate runner install command (${response.status})`);
      setTemplate(normalizeRunnerComposeTemplate(await response.json()));
    } catch (error) {
      setTemplate(null);
      setTemplateError(error instanceof Error ? error.message : 'Unable to generate runner install command.');
    } finally {
      setLoadingTemplate(false);
    }
  }, [canManageDispatcher, runnerCapacity, runnerId, runnerScopes]);

  const handleCopyTemplate = async () => {
    if (!template?.bootstrapCommand) return;
    try {
      await navigator.clipboard.writeText(template.bootstrapCommand);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch (error) {
      console.error('Failed to copy runner install command', error);
      setTemplateError('Unable to copy runner install command.');
    }
  };

  return (
    <section id={RUNNER_DEPLOYMENT_GUIDE_ID} className="scroll-mt-6 space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="text-lg font-semibold">Runner Deployment Guide</h3>
          <p className="mt-1 max-w-3xl text-sm leading-6 text-[var(--text-secondary)]">
            Use this when the dispatcher needs more capacity, a scoped runner, or a runner on a host with specific tools.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <span className="runner-pill runner-pill--ok">Docker ready</span>
          <span className="runner-pill runner-pill--muted">K8s under construction</span>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-tertiary)] p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h4 className="text-sm font-semibold">Docker runner</h4>
            <span className="runner-pill runner-pill--ok">Available</span>
          </div>
          <ol className="mt-3 list-decimal space-y-2 pl-5 text-sm leading-6 text-[var(--text-secondary)]">
            <li>Add a runner service with a unique `RUNNER_ID`.</li>
            <li>Set `RUNNER_SCOPES` for the work it may receive, or leave it empty for all scopes.</li>
            <li>Set `RUNNER_CAPACITY` to the number of concurrent jobs this host can run.</li>
            <li>Generate a one-time install command so dispatcher address, service JWT, and TLS secrets match this live setup.</li>
            <li>Mount `/var/run/docker.sock` so the runner can start agent and step containers.</li>
            <li>Start the service, then refresh this dispatcher page to confirm the runner registered.</li>
          </ol>
          {canManageDispatcher ? (
            <div className="mt-4 space-y-3">
              <div className="grid gap-3 md:grid-cols-3">
                <label className="space-y-1 text-sm">
                  <span className="text-xs text-[var(--text-secondary)]">Runner name</span>
                  <input className="pipelines-input" value={runnerId} onChange={event => setRunnerId(event.target.value)} />
                </label>
                <label className="space-y-1 text-sm">
                  <span className="text-xs text-[var(--text-secondary)]">Scopes</span>
                  <input className="pipelines-input" value={runnerScopes} onChange={event => setRunnerScopes(event.target.value)} placeholder="empty for all scopes" />
                </label>
                <label className="space-y-1 text-sm">
                  <span className="text-xs text-[var(--text-secondary)]">Capacity</span>
                  <input className="pipelines-input" type="number" min="1" value={runnerCapacity} onChange={event => setRunnerCapacity(event.target.value)} />
                </label>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <button type="button" className="glass-button-subtle" onClick={() => void loadTemplate()} disabled={loadingTemplate}>
                  <RefreshCw className={`h-4 w-4 ${loadingTemplate ? 'animate-spin' : ''}`} />
                  {loadingTemplate ? 'Generating…' : template ? 'Regenerate one-time command' : 'Generate one-time command'}
                </button>
                <button type="button" className="glass-button-primary" onClick={() => void handleCopyTemplate()} disabled={!template?.bootstrapCommand || loadingTemplate}>
                  <Copy className="h-4 w-4" />
                  {copied ? 'Copied' : 'Copy install command'}
                </button>
              </div>
              {templateError && <p className="text-sm text-red-500">{templateError}</p>}
              {template?.dispatcherAddress && (
                <p className="text-xs leading-5 text-[var(--text-secondary)]">
                  Dispatcher address: <span className="font-mono text-[var(--text-primary)]">{template.dispatcherAddress}</span>
                </p>
              )}
              {template?.expiresAt && (
                <p className="text-xs leading-5 text-[var(--text-secondary)]">
                  One-time token expires: <span className="font-mono text-[var(--text-primary)]">{formatTimestamp(template.expiresAt)}</span>
                </p>
              )}
              <pre className="max-h-96 overflow-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs leading-5 text-[var(--text-primary)]">
                <code>{template?.bootstrapCommand || 'Generate a one-time install command, then run it on the Docker host.'}</code>
              </pre>
              {template?.warnings.map(warning => (
                <div key={warning} className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs leading-5 text-amber-700 dark:text-amber-300">
                  {warning}
                </div>
              ))}
            </div>
          ) : (
            <div className="mt-4 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-sm leading-6 text-[var(--text-secondary)]">
              Dispatcher management access is required to generate a one-time runner install command.
            </div>
          )}
        </div>

        <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-tertiary)] p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h4 className="text-sm font-semibold">Kubernetes runner</h4>
            <span className="runner-pill runner-pill--muted">Under construction</span>
          </div>
          <p className="mt-3 text-sm leading-6 text-[var(--text-secondary)]">
            Kubernetes support is not ready to deploy from this UI yet. The expected path is a runner Deployment with NopsAI service secrets, `DISPATCHER_ADDRESS`, `RUNNER_ID`, `RUNNER_SCOPES`, and `RUNNER_CAPACITY`, plus a container runtime strategy for agent and step execution.
          </p>
          <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm leading-6 text-amber-700 dark:text-amber-300">
            Keep using Docker runners for active workloads until the Kubernetes manifests and runtime wiring are completed.
          </div>
        </div>
      </div>
    </section>
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

function normalizeLLMProfilesPayload(value: unknown): LLMProfilesPayload {
  const record = asRecord(value);
  const profilesRaw = record && Array.isArray(record.profiles) ? record.profiles : [];
  const profiles = profilesRaw
    .map(item => {
      const profile = asRecord(item);
      if (!profile) return null;
      const name = readString(profile.name).trim();
      if (!name) return null;
      return {
        name,
        provider: readString(profile.provider).trim(),
        model: readString(profile.model).trim(),
        base_url: readString(profile.base_url).trim(),
        api_key_secret: readString(profile.api_key_secret).trim(),
        allowed_scopes: normalizeStringArray(profile.allowed_scopes),
        reasoning: readString(profile.reasoning).trim(),
        thinking: typeof profile.thinking === 'boolean' ? profile.thinking : undefined,
        status: readString(profile.status).trim() || 'unknown',
        validation: readOptionalString(profile.validation),
        references: normalizeStringArray(profile.references),
        allowed_in_scope: typeof profile.allowed_in_scope === 'boolean' ? profile.allowed_in_scope : undefined,
        disabled_reason: readOptionalString(profile.disabled_reason),
      } satisfies LLMProfileRecord;
    })
    .filter(Boolean) as LLMProfileRecord[];

  profiles.sort((a, b) => a.name.localeCompare(b.name));
  return {
    default_profile: readString(record?.default_profile).trim() || profiles[0]?.name || 'standard',
    profiles,
  };
}

function normalizeMCPServersPayload(value: unknown): MCPServerRecord[] {
  const record = asRecord(value);
  const serversRaw = record && Array.isArray(record.servers) ? record.servers : [];
  const servers = serversRaw
    .map(item => {
      const server = asRecord(item);
      if (!server) return null;
      const name = readString(server.name).trim();
      if (!name) return null;
      return {
        name,
        display_name: readString(server.display_name).trim(),
        enabled: typeof server.enabled === 'boolean' ? server.enabled : true,
        provider: readString(server.provider).trim(),
        transport: readString(server.transport).trim() || 'streamable_http',
        url: readString(server.url).trim(),
        auth_type: readString(server.auth_type).trim() || 'none',
        auth_secret: readString(server.auth_secret).trim(),
        headers: normalizeStringMap(server.headers),
        timeout: readString(server.timeout).trim() || '30s',
        allowed_scopes: normalizeStringArray(server.allowed_scopes),
        last_test_status: readOptionalString(server.last_test_status),
        last_test_message: readOptionalString(server.last_test_message),
        last_tested_at: readOptionalString(server.last_tested_at),
        last_discovered_at: readOptionalString(server.last_discovered_at),
        discovered_server_name: readOptionalString(server.discovered_server_name),
        discovered_version: readOptionalString(server.discovered_version),
        discovered_protocol: readOptionalString(server.discovered_protocol),
        tools: Array.isArray(server.tools) ? server.tools.map(normalizeMCPTool).filter((tool): tool is MCPToolRecord => Boolean(tool)) : [],
      } satisfies MCPServerRecord;
    })
    .filter(Boolean) as MCPServerRecord[];
  return servers.sort((a, b) => a.name.localeCompare(b.name));
}

function normalizeMCPTool(value: unknown): MCPToolRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const name = readString(record.name).trim();
  if (!name) return null;
  return {
    server_name: readString(record.server_name).trim(),
    name,
    description: readOptionalString(record.description),
    input_schema: readOptionalString(record.input_schema),
    schema_hash: readOptionalString(record.schema_hash),
    last_seen_at: readOptionalString(record.last_seen_at),
  };
}

function normalizeMCPProfilesPayload(value: unknown): MCPProfileRecord[] {
  const record = asRecord(value);
  const profilesRaw = record && Array.isArray(record.profiles) ? record.profiles : [];
  const profiles = profilesRaw
    .map(item => {
      const profile = asRecord(item);
      if (!profile) return null;
      const name = readString(profile.name).trim();
      if (!name) return null;
      const refsRaw = Array.isArray(profile.servers) ? profile.servers : [];
      const refs = refsRaw
        .map(refItem => {
          const ref = asRecord(refItem);
          if (!ref) return null;
          const server = readString(ref.server).trim();
          if (!server) return null;
          return { server, tools: normalizeStringArray(ref.tools) } satisfies MCPProfileServerRef;
        })
        .filter(Boolean) as MCPProfileServerRef[];
      return {
        name,
        description: readString(profile.description).trim(),
        enabled: typeof profile.enabled === 'boolean' ? profile.enabled : true,
        servers: refs,
        allowed_scopes: normalizeStringArray(profile.allowed_scopes),
      } satisfies MCPProfileRecord;
    })
    .filter(Boolean) as MCPProfileRecord[];
  return profiles.sort((a, b) => a.name.localeCompare(b.name));
}

function splitCSV(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

function splitToolNames(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/[\n,]/)
    .map(item => item.trim())
    .filter(item => {
      if (!item || seen.has(item)) return false;
      seen.add(item);
      return true;
    })
    .sort((a, b) => a.localeCompare(b));
}

function formatHeadersJSON(headers: Record<string, string>): string {
  if (Object.keys(headers || {}).length === 0) return '';
  return JSON.stringify(headers, null, 2);
}

function parseHeadersJSON(value: string): Record<string, string> | null {
  const trimmed = value.trim();
  if (!trimmed) return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return null;
  }
  const record = asRecord(parsed);
  if (!record || Array.isArray(parsed)) return null;
  const headers: Record<string, string> = {};
  for (const [key, headerValue] of Object.entries(record)) {
    const headerName = key.trim();
    if (!headerName) continue;
    if (typeof headerValue !== 'string') return null;
    headers[headerName] = headerValue.trim();
  }
  return headers;
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

function normalizeRunnerComposeTemplate(value: unknown): RunnerComposeTemplate {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    runnerScopes: readString(record.runner_scopes ?? record.runnerScopes),
    runnerCapacity: normalizeNumber(record.runner_capacity ?? record.runnerCapacity),
    dispatcherAddress: readString(record.dispatcher_address ?? record.dispatcherAddress),
    compose: readString(record.compose),
    command: readString(record.command),
    bootstrapCommand: readString(record.bootstrap_command ?? record.bootstrapCommand),
    expiresAt: readString(record.expires_at ?? record.expiresAt),
    warnings: normalizeStringArray(record.warnings),
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

function normalizeListPayload(payload: unknown, keys: string[] = []): unknown[] | null {
  let value = payload;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed || trimmed === 'null') return [];
    if (trimmed.startsWith('[') || trimmed.startsWith('{')) {
      try {
        value = JSON.parse(trimmed);
      } catch {
        return null;
      }
    }
  }
  if (value == null) return [];
  if (Array.isArray(value)) return value;

  const record = asRecord(value);
  if (!record) return null;
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) continue;
    const candidate = record[key];
    if (candidate == null) return [];
    if (Array.isArray(candidate)) return candidate;
  }
  return null;
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

function normalizeAccessGrantRecord(value: unknown): AccessGrantRecord | null {
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

export default SystemPage;
