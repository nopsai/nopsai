import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { buildApiUrl } from '../lib/api';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import {
  buildConfigRepositoryWriteFiles,
  type ConfigRepositoryCommitResponse,
  type ConfigRepositoryDriftResponse,
} from '../lib/configRepositoryDrift';
import DataManagementPage from './DataManagement';
import SetupWizard from './Setup';
import LLMProfilesPanel from '../features/system/LLMProfilesPanel';
import MCPPanel from '../features/system/MCPPanel';
import DispatcherPanel, { getRunnerMeta, normalizeDispatcherStatus, runnerActionKey } from '../features/system/DispatcherPanel';
import SystemConfig from '../features/system/SystemConfig';
import AccessPanel, {
  POLICY_TEMPLATE_ROLE,
  isProtectedAccessRole,
  normalizeAccessGrantRecord,
  normalizeAdminPolicies,
  normalizeBasicGrantInputs,
  policyKey,
  policyLabel,
  policyName,
  type AccessGrantRecord,
  type BasicGrantInput,
  type RoleDefinition,
  type RolePermission,
  type RolePolicyDraft,
  type ServiceAccountSummary,
  type ServiceAccountToken,
  type UserSummary,
} from '../features/system/AccessPanel';

type ConfigFormState = {
  agent_image: string;
  docker_network_name: string;
  default_pipeline_timeout: string;
  llm_agent_timeout: string;
  auto_removal_agent_container: boolean;
  agent_nopsai_api_url: string;
  git_bot_nopsai_api_url: string;
  nopsai_git_bot_api_url: string;
  dispatcher_address: string;
  dispatcher_routing: Record<string, string[]>;
  runner_id: string;
  runner_scopes: string;
  runner_capacity: string;
};

type ConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
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
  write_enabled: boolean;
  write_branch: string;
};

type NotificationMailSMTPSettings = {
  host: string;
  port: number;
  start_tls: boolean;
  username: string;
  password_secret_ref: string;
};

type NotificationMailSettingsRecord = {
  enabled: boolean;
  from: string;
  smtp: NotificationMailSMTPSettings;
  source?: string;
  config_source_path?: string;
  managed_by_config_repo?: boolean;
  updated_at?: string;
};

type NotificationMailSettingsFormState = {
  enabled: boolean;
  from: string;
  smtp_host: string;
  smtp_port: string;
  smtp_start_tls: boolean;
  smtp_username: string;
  smtp_password_secret_ref: string;
  test_to: string;
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

type DispatcherStatusState = {
  queuedJobs: number;
  runners: Runner[];
  routing: Record<string, string[]>;
  dispatcherError?: string;
  fetchedAt: number;
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
  dispatcher_address: '',
  dispatcher_routing: {},
  runner_id: '',
  runner_scopes: '',
  runner_capacity: '1',
};

const emptyConfigRepositoryForm: ConfigRepositoryFormState = {
  repo_url: '',
  branch: 'main',
  base_path: '',
  enabled: true,
  write_enabled: false,
  write_branch: 'nopsai/ui-changes',
};

const emptyNotificationMailSettingsForm: NotificationMailSettingsFormState = {
  enabled: false,
  from: '',
  smtp_host: '',
  smtp_port: '587',
  smtp_start_tls: true,
  smtp_username: '',
  smtp_password_secret_ref: '',
  test_to: '',
};

const POLL_INTERVAL_MS = 5000;
const RUNNER_DEPLOYMENT_GUIDE_QUERY = 'guide';
const RUNNER_DEPLOYMENT_GUIDE_VALUE = 'runner';
const RUNNER_DEPLOYMENT_GUIDE_ID = 'dispatcher-runner-deployment-guide';

function scrollRunnerDeploymentGuide() {
  window.setTimeout(() => {
    document.getElementById(RUNNER_DEPLOYMENT_GUIDE_ID)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, 0);
}

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
  canViewDataManagement: boolean;
  canManageDataManagement: boolean;
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
            : params.tab === 'data-management'
              ? 'data-management'
              : 'config';
  const allowedTabs = useMemo(() => {
    const tabs: Array<'config' | 'setup' | 'llm-profiles' | 'mcp' | 'data-management' | 'dispatcher' | 'access'> = [];
    if (permissions.canViewConfig) tabs.push('config');
    if (permissions.canViewSetup) tabs.push('setup');
    if (permissions.canViewLLMProfiles) tabs.push('llm-profiles');
    if (permissions.canViewMCP) tabs.push('mcp');
    if (permissions.canViewDataManagement) tabs.push('data-management');
    if (permissions.canViewDispatcher) tabs.push('dispatcher');
    if (permissions.canViewAccess) tabs.push('access');
    return tabs;
  }, [permissions.canViewAccess, permissions.canViewConfig, permissions.canViewDataManagement, permissions.canViewDispatcher, permissions.canViewLLMProfiles, permissions.canViewMCP, permissions.canViewSetup]);
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
  const [globalConfigRepoDriftOpen, setGlobalConfigRepoDriftOpen] = useState(false);
  const [globalConfigRepoDrift, setGlobalConfigRepoDrift] = useState<ConfigRepositoryDriftResponse | null>(null);
  const [globalConfigRepoDriftLoading, setGlobalConfigRepoDriftLoading] = useState(false);
  const [globalConfigRepoDriftError, setGlobalConfigRepoDriftError] = useState<string | null>(null);
  const [globalConfigRepoPushing, setGlobalConfigRepoPushing] = useState(false);
  const [globalConfigRepoPushResult, setGlobalConfigRepoPushResult] = useState<ConfigRepositoryCommitResponse | null>(null);
  const [mailSettings, setMailSettings] = useState<NotificationMailSettingsRecord | null>(null);
  const [mailSettingsForm, setMailSettingsForm] = useState<NotificationMailSettingsFormState>(emptyNotificationMailSettingsForm);
  const [mailSettingsLoading, setMailSettingsLoading] = useState(false);
  const [mailSettingsSaving, setMailSettingsSaving] = useState(false);
  const [mailSettingsTesting, setMailSettingsTesting] = useState(false);
  const [mailSettingsError, setMailSettingsError] = useState<string | null>(null);

  const [dispatcherLoading, setDispatcherLoading] = useState(false);
  const [dispatcherError, setDispatcherError] = useState<string | null>(null);
  const [dispatcherStatus, setDispatcherStatus] = useState<DispatcherStatusState | null>(null);

  const [runnerActionPending, setRunnerActionPending] = useState<Set<string>>(new Set());

  const [users, setUsers] = useState<UserSummary[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [usersError, setUsersError] = useState<string | null>(null);
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccountSummary[]>([]);
  const [serviceAccountsLoading, setServiceAccountsLoading] = useState(false);
  const [serviceAccountsError, setServiceAccountsError] = useState<string | null>(null);
  const [accessGrants, setAccessGrants] = useState<AccessGrantRecord[]>([]);
  const [accessGrantsLoading, setAccessGrantsLoading] = useState(false);
  const [accessGrantsError, setAccessGrantsError] = useState<string | null>(null);
  const [policies, setPolicies] = useState<RolePermission[]>([]);
  const [policyTemplates, setPolicyTemplates] = useState<RolePermission[]>([]);
  const [policiesLoading, setPoliciesLoading] = useState(false);
  const [policiesError, setPoliciesError] = useState<string | null>(null);
  const [newUser, setNewUser] = useState({ sub: '', email: '', password: '', roles: [] as string[] });
  const [newServiceAccount, setNewServiceAccount] = useState({ sub: '', email: '', tokenName: 'default', roles: [] as string[] });
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
      dispatcher_address: readString(record.dispatcher_address),
      dispatcher_routing: normalizeRouting(record.dispatcher_routing),
      runner_id: readString(record.runner_id),
      runner_scopes: readString(record.runner_scopes),
      runner_capacity: String(record.runner_capacity ?? '1'),
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
      write_enabled: Boolean(record.write_enabled),
      write_branch: readString(record.write_branch).trim() || 'nopsai/ui-changes',
      managed_by_config_repo: Boolean(record.managed_by_config_repo),
      config_source_path: readOptionalString(record.config_source_path),
      last_sync_status: readString(record.last_sync_status),
      last_sync_message: readOptionalString(record.last_sync_message),
      last_sync_started_at: readOptionalString(record.last_sync_started_at),
      last_sync_completed_at: readOptionalString(record.last_sync_completed_at),
      last_sync_commit_sha: readOptionalString(record.last_sync_commit_sha),
    };
  }, []);

  const normalizeMailSettings = useCallback((payload: unknown): NotificationMailSettingsRecord => {
    return normalizeNotificationMailSettings(payload);
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

  const loadMailSettings = useCallback(async () => {
    setMailSettingsLoading(true);
    setMailSettingsError(null);
    try {
      const settings = normalizeMailSettings(await fetchJson('/v1/system/notifications/mail'));
      if (!isMountedRef.current) return;
      setMailSettings(settings);
      setMailSettingsForm(mailSettingsFormFromRecord(settings));
    } catch (error) {
      console.error('Failed to load mail notification settings', error);
      if (!isMountedRef.current) return;
      setMailSettingsError(error instanceof Error ? error.message : 'Unable to load mail notification settings');
    } finally {
      if (isMountedRef.current) {
        setMailSettingsLoading(false);
      }
    }
  }, [fetchJson, normalizeMailSettings]);

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
          write_enabled: repo.write_enabled,
          write_branch: repo.write_branch || 'nopsai/ui-changes',
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

  const loadServiceAccounts = useCallback(async () => {
    setServiceAccountsLoading(true);
    setServiceAccountsError(null);
    try {
      const payload = await fetchJson('/v1/admin/service-accounts');
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
        const created = await fetchJson('/v1/admin/service-accounts', {
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
            await fetchJson('/v1/admin/service-account-roles', {
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
            await fetchJson('/v1/access/grants', {
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
        setNewServiceAccount({ sub: '', email: '', tokenName: 'default', roles: [] });
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
    [addToast, fetchJson, loadAccessGrants, loadServiceAccounts, newServiceAccount.email, newServiceAccount.roles, newServiceAccount.sub, newServiceAccount.tokenName]
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

  const deleteServiceAccount = useCallback(
    async (serviceAccountId: string) => {
      try {
        await fetchJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}`, { method: 'DELETE' });
        addToast('Service account deleted', 'success');
        await loadServiceAccounts();
        await loadAccessGrants();
      } catch (error) {
        addToast(error instanceof Error ? error.message : 'Failed to delete service account', 'error');
      }
    },
    [addToast, fetchJson, loadAccessGrants, loadServiceAccounts]
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

  const createServiceAccountAccessGrant = useCallback(
    async (input: { serviceAccountSub: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => {
      const role = input.role.trim().toLowerCase();
      const resourceType = input.resourceType.trim();
      const resourceID = input.resourceID.trim();
      await fetchJson('/v1/access/grants', {
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
      const inUse =
        users.some(user => (user.roles || []).some(r => r.role === role.role)) ||
        serviceAccounts.some(account => (account.roles || []).some(r => r.role === role.role));
      if (inUse) {
        addToast('Cannot delete a role that is still assigned. Remove assignments first.', 'error');
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
    [addToast, fetchJson, loadPolicies, serviceAccounts, users]
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
          await fetchJson('/v1/admin/service-account-roles', {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              service_account_id: serviceAccountId,
              role,
            }),
          });
        }
        for (const role of toAdd) {
          await fetchJson('/v1/admin/service-account-roles', {
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
    [addToast, fetchJson, loadServiceAccounts]
  );

  const updateServiceAccount = useCallback(
    async (serviceAccountId: string, input: { email?: string; status?: string }) => {
      const payload: Record<string, string> = {};
      if (input.email) payload.email = input.email.trim();
      if (input.status) payload.status = input.status.trim();
      if (Object.keys(payload).length === 0) return;
      try {
        await fetchJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}`, {
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
    [addToast, fetchJson, loadServiceAccounts]
  );

  const loadServiceAccountTokens = useCallback(
    async (serviceAccountId: string): Promise<ServiceAccountToken[]> => {
      const payload = await fetchJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}/tokens`);
      const records = normalizeListPayload(payload, ['tokens', 'items', 'data', 'records', 'results']);
      return records ? (records as ServiceAccountToken[]) : [];
    },
    [fetchJson]
  );

  const createServiceAccountToken = useCallback(
    async (serviceAccountId: string, name: string): Promise<ServiceAccountToken> => {
      const token = (await fetchJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}/tokens`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      })) as ServiceAccountToken;
      addToast('Service account token created', 'success');
      await loadServiceAccounts();
      return token;
    },
    [addToast, fetchJson, loadServiceAccounts]
  );

  const revokeServiceAccountToken = useCallback(
    async (serviceAccountId: string, tokenId: string) => {
      await fetchJson(`/v1/admin/service-accounts/${encodeURIComponent(serviceAccountId)}/tokens/${encodeURIComponent(tokenId)}`, {
        method: 'DELETE',
      });
      addToast('Service account token revoked', 'success');
      await loadServiceAccounts();
    },
    [addToast, fetchJson, loadServiceAccounts]
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
          dispatcher_address: config.dispatcher_address.trim(),
          dispatcher_routing: config.dispatcher_routing,
          runner_id: config.runner_id.trim(),
          runner_scopes: config.runner_scopes.trim(),
          runner_capacity: Number.parseInt(config.runner_capacity, 10) || 1,
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

  const saveMailSettings = useCallback(async () => {
    if (mailSettingsSaving || !permissions.canManageRuntimeConfig || mailSettings?.managed_by_config_repo) return;
    setMailSettingsSaving(true);
    setMailSettingsError(null);
    try {
      const settings = normalizeMailSettings(await fetchJson('/v1/system/notifications/mail', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(mailSettingsPayloadFromForm(mailSettingsForm)),
      }));
      setMailSettings(settings);
      setMailSettingsForm(mailSettingsFormFromRecord(settings, mailSettingsForm.test_to));
      setGlobalConfigRepoDrift(null);
      setGlobalConfigRepoPushResult(null);
      addToast('Mail notification settings saved.', 'success');
    } catch (error) {
      console.error('Failed to save mail notification settings', error);
      const message = error instanceof Error ? error.message : 'Unable to save mail notification settings';
      setMailSettingsError(message);
      addToast('Failed to save mail notification settings.', 'error');
    } finally {
      if (isMountedRef.current) {
        setMailSettingsSaving(false);
      }
    }
  }, [addToast, fetchJson, mailSettings?.managed_by_config_repo, mailSettingsForm, mailSettingsSaving, normalizeMailSettings, permissions.canManageRuntimeConfig]);

  const testMailSettings = useCallback(async () => {
    if (mailSettingsTesting || !permissions.canManageRuntimeConfig) return;
    const to = mailSettingsForm.test_to.trim();
    if (!to) {
      setMailSettingsError('Test recipient is required.');
      return;
    }
    setMailSettingsTesting(true);
    setMailSettingsError(null);
    try {
      await fetchJson('/v1/system/notifications/mail/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ to }),
      });
      addToast('Mail test sent.', 'success');
    } catch (error) {
      console.error('Failed to send mail notification test', error);
      const message = error instanceof Error ? error.message : 'Unable to send mail test';
      setMailSettingsError(message);
      addToast('Failed to send mail test.', 'error');
    } finally {
      if (isMountedRef.current) {
        setMailSettingsTesting(false);
      }
    }
  }, [addToast, fetchJson, mailSettingsForm.test_to, mailSettingsTesting, permissions.canManageRuntimeConfig]);

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
          write_enabled: Boolean(globalConfigRepoForm.write_enabled),
          write_branch: globalConfigRepoForm.write_branch.trim(),
        }),
      });
      const repo = normalizeConfigRepository(payload);
      setGlobalConfigRepo(repo);
      setGlobalConfigRepoForm(repo ? {
        repo_url: repo.repo_url,
        branch: repo.branch || 'main',
        base_path: repo.base_path || '',
        enabled: repo.enabled,
        write_enabled: repo.write_enabled,
        write_branch: repo.write_branch || 'nopsai/ui-changes',
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

  const checkGlobalConfigRepositoryDrift = useCallback(async () => {
    if (!permissions.canViewGlobalConfigRepo || globalConfigRepoDriftLoading) return;
    setGlobalConfigRepoDriftOpen(true);
    setGlobalConfigRepoDriftLoading(true);
    setGlobalConfigRepoDriftError(null);
    setGlobalConfigRepoPushResult(null);
    try {
      const payload = await fetchJson('/v1/system/config-repo/drift');
      setGlobalConfigRepoDrift(payload as ConfigRepositoryDriftResponse);
    } catch (error) {
      console.error('Failed to check global config repository drift', error);
      const message = error instanceof Error ? error.message : 'Unable to check config repository drift';
      setGlobalConfigRepoDriftError(message);
      addToast('Failed to check config repository drift.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoDriftLoading(false);
      }
    }
  }, [addToast, fetchJson, globalConfigRepoDriftLoading, permissions.canViewGlobalConfigRepo]);

  const pushGlobalConfigRepositoryDrift = useCallback(async () => {
    if (!permissions.canManageGlobalConfigRepo || globalConfigRepoPushing) return;
    const files = buildConfigRepositoryWriteFiles(globalConfigRepoDrift);
    if (!globalConfigRepoDrift || files.length === 0) return;
    if (!globalConfigRepoDrift.can_push) {
      setGlobalConfigRepoDriftError('Enable Git push and set a push branch before committing changes.');
      return;
    }
    setGlobalConfigRepoPushing(true);
    setGlobalConfigRepoDriftError(null);
    try {
      const payload = await fetchJson('/v1/system/config-repo/write', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: globalConfigRepoDrift.push_message || 'Update Nopsai config',
          files,
        }),
      });
      const result = payload as ConfigRepositoryCommitResponse;
      setGlobalConfigRepoPushResult(result);
      const branch = result.branch || globalConfigRepoDrift.push_branch || globalConfigRepo?.write_branch || 'the push branch';
      addToast(`Pushed ${files.length} file${files.length === 1 ? '' : 's'} to ${branch}.`, 'success');
    } catch (error) {
      console.error('Failed to push global config repository drift', error);
      const message = error instanceof Error ? error.message : 'Unable to push config repository changes';
      setGlobalConfigRepoDriftError(message);
      addToast('Failed to push config repository changes.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoPushing(false);
      }
    }
  }, [addToast, fetchJson, globalConfigRepo?.write_branch, globalConfigRepoDrift, globalConfigRepoPushing, permissions.canManageGlobalConfigRepo]);

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
      void loadMailSettings();
    } else {
      setConfigLoading(false);
    }
    if (permissions.canViewGlobalConfigRepo) {
      void loadGlobalConfigRepository();
    }
  }, [loadGlobalConfigRepository, loadMailSettings, loadSystemConfig, permissions.canViewConfig, permissions.canViewGlobalConfigRepo, permissions.canViewRuntimeConfig, visibleTab]);

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
      void loadServiceAccounts();
      void loadAccessGrants();
      void loadPolicies();
    }
  }, [loadAccessGrants, loadPolicies, loadServiceAccounts, loadUsers, permissions.canViewAccess, visibleTab]);

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

  const reloadConfigTab = useCallback(async () => {
    if (!permissions.canViewRuntimeConfig) return;
    await Promise.all([loadSystemConfig(), loadMailSettings()]);
  }, [loadMailSettings, loadSystemConfig, permissions.canViewRuntimeConfig]);

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
          mailSettings={mailSettings}
          mailSettingsForm={mailSettingsForm}
          mailSettingsLoading={mailSettingsLoading}
          mailSettingsSaving={mailSettingsSaving}
          mailSettingsTesting={mailSettingsTesting}
          mailSettingsError={mailSettingsError}
          onChange={setConfig}
          onReload={reloadConfigTab}
          onSave={saveConfig}
          onMailSettingsChange={setMailSettingsForm}
          onSaveMailSettings={saveMailSettings}
          onTestMailSettings={testMailSettings}
          onGlobalConfigRepoChange={setGlobalConfigRepoForm}
          onSaveGlobalConfigRepo={saveGlobalConfigRepository}
          onDeleteGlobalConfigRepo={deleteGlobalConfigRepository}
          onSyncGlobalConfigRepo={syncGlobalConfigRepository}
          onCheckGlobalConfigRepoDrift={checkGlobalConfigRepositoryDrift}
          globalConfigRepoDriftLoading={globalConfigRepoDriftLoading}
          globalConfigRepoPushing={globalConfigRepoPushing}
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
      {visibleTab === 'data-management' && (
        <DataManagementPage canManage={permissions.canManageDataManagement} />
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
          runnerDefaults={config}
        />
      )}
      {visibleTab === 'access' && (
        <AccessPanel
          users={users}
          loading={usersLoading}
          error={usersError}
          serviceAccounts={serviceAccounts}
          serviceAccountsLoading={serviceAccountsLoading}
          serviceAccountsError={serviceAccountsError}
          accessGrants={accessGrants}
          accessGrantsLoading={accessGrantsLoading}
          accessGrantsError={accessGrantsError}
          policies={policies}
          policiesLoading={policiesLoading}
          policiesError={policiesError}
          newUser={newUser}
          newServiceAccount={newServiceAccount}
          policyTemplates={policyTemplates}
          onChangeUser={setNewUser}
          onCreateUser={createUser}
          onChangeServiceAccount={setNewServiceAccount}
          onCreateServiceAccount={createServiceAccount}
          onReloadUsers={loadUsers}
          onReloadServiceAccounts={loadServiceAccounts}
          onReloadPolicies={loadPolicies}
          onCreatePermission={createPermission}
          newPermission={newPermission}
          onChangePermission={setNewPermission}
          onDeleteUser={deleteUser}
          onDeleteServiceAccount={deleteServiceAccount}
          onDeletePolicy={deletePolicy}
          onDeleteRoleDefinition={deleteRoleDefinition}
          onSaveRoleDefinition={saveRoleDefinition}
          onEditPolicy={editPolicy}
          onUpdateUserRoles={updateUserRoles}
          onUpdateServiceAccountRoles={updateServiceAccountRoles}
          onCreateAccessGrant={createAccessGrant}
          onCreateServiceAccountAccessGrant={createServiceAccountAccessGrant}
          onDeleteAccessGrant={deleteAccessGrant}
          onReloadAccessGrants={loadAccessGrants}
          onUpdateUser={updateUser}
          onUpdateServiceAccount={updateServiceAccount}
          onLoadServiceAccountTokens={loadServiceAccountTokens}
          onCreateServiceAccountToken={createServiceAccountToken}
          onRevokeServiceAccountToken={revokeServiceAccountToken}
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

      {globalConfigRepoDriftOpen && (
        <ConfigRepositoryDriftModal
          title="Global config repository"
          drift={globalConfigRepoDrift}
          loading={globalConfigRepoDriftLoading}
          error={globalConfigRepoDriftError}
          pushing={globalConfigRepoPushing}
          pushResult={globalConfigRepoPushResult}
          canPush={permissions.canManageGlobalConfigRepo && Boolean(globalConfigRepoDrift?.can_push)}
          onClose={() => setGlobalConfigRepoDriftOpen(false)}
          onRefresh={checkGlobalConfigRepositoryDrift}
          onPush={pushGlobalConfigRepositoryDrift}
        />
      )}
    </div>
  );
}

function normalizeNotificationMailSettings(value: unknown): NotificationMailSettingsRecord {
  const record = asRecord(value);
  const smtp = asRecord(record?.smtp);
  const port = normalizeNumber(smtp?.port);
  return {
    enabled: Boolean(record?.enabled),
    from: readString(record?.from),
    smtp: {
      host: readString(smtp?.host),
      port: port > 0 ? port : 587,
      start_tls: typeof smtp?.start_tls === 'boolean' ? smtp.start_tls : true,
      username: readString(smtp?.username),
      password_secret_ref: readString(smtp?.password_secret_ref),
    },
    source: readOptionalString(record?.source),
    config_source_path: readOptionalString(record?.config_source_path),
    managed_by_config_repo: Boolean(record?.managed_by_config_repo),
    updated_at: readOptionalString(record?.updated_at),
  };
}

function mailSettingsFormFromRecord(record: NotificationMailSettingsRecord, testTo = ''): NotificationMailSettingsFormState {
  return {
    enabled: record.enabled,
    from: record.from,
    smtp_host: record.smtp.host,
    smtp_port: String(record.smtp.port || 587),
    smtp_start_tls: record.smtp.start_tls,
    smtp_username: record.smtp.username,
    smtp_password_secret_ref: record.smtp.password_secret_ref,
    test_to: testTo,
  };
}

function mailSettingsPayloadFromForm(form: NotificationMailSettingsFormState) {
  const port = Number.parseInt(form.smtp_port, 10);
  return {
    enabled: form.enabled,
    from: form.from.trim(),
    smtp: {
      host: form.smtp_host.trim(),
      port: Number.isFinite(port) && port > 0 ? port : 587,
      start_tls: Boolean(form.smtp_start_tls),
      username: form.smtp_username.trim(),
      password_secret_ref: form.smtp_password_secret_ref.trim(),
    },
  };
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
