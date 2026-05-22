import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { HashRouter, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import AppHelp from './components/AppHelp';
import { buildApiUrl, clearSession, getStoredSession, setPasswordChangeRequired, type StoredSession } from './lib/api';
import { PIPELINE_DRAFTS_CHANGED_EVENT, getPipelineDraftStorageKey, loadPipelineDrafts } from './lib/pipelineDrafts';
import { fetchResourceGroupPaths, insertGroupPath } from './lib/resourceGroups';
import { STEP_DRAFTS_CHANGED_EVENT, getStepDraftStorageKey, loadStepDrafts } from './lib/stepDrafts';

const PipelineRunsPage = lazy(() => import('./pages/PipelineRuns'));
const PipelinesPage = lazy(() => import('./pages/Pipelines'));
const TriggersPage = lazy(() => import('./pages/Triggers'));
const ScopesPage = lazy(() => import('./pages/Scopes'));
const LabPage = lazy(() => import('./pages/Lab'));
const StepsPage = lazy(() => import('./pages/Steps'));
const KnowledgeContextPage = lazy(() => import('./pages/KnowledgeContext'));
const MonitoringPage = lazy(() => import('./pages/Monitoring'));
const SystemPage = lazy(() => import('./pages/System'));
const LoginPage = lazy(() => import('./pages/Login'));
const ProfilePage = lazy(() => import('./pages/Profile'));

type Theme = 'light' | 'dark';

type NavItem = {
  label: string;
  path: string;
  icon: ReactNode;
};

type PipelineTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: PipelineTreeNode[];
  pipelineIds: string[];
};

type TriggerTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TriggerTreeNode[];
  triggerSlugs: string[];
};

type StepTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: StepTreeNode[];
  stepIds: string[];
};

type ScopeTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: ScopeTreeNode[];
  scopes: string[];
};

type KnowledgeContextTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: KnowledgeContextTreeNode[];
  knowledgeContextIds: string[];
};

type ResourceCapabilities = {
  write?: boolean;
  delete?: boolean;
};

type ReadCapabilities = {
  read?: boolean;
  write?: boolean;
  delete?: boolean;
};

type SystemCapabilities = {
  configRead?: boolean;
  configWrite?: boolean;
  llmProfilesRead?: boolean;
  llmProfilesWrite?: boolean;
  mcpRead?: boolean;
  mcpWrite?: boolean;
  configReposRead?: boolean;
  configReposWrite?: boolean;
  dispatcherRead?: boolean;
  dispatcherWrite?: boolean;
  access?: boolean;
};

type SetupStatusSummary = {
  completed?: boolean;
};

type CurrentUser = {
  sub: string;
  email?: string;
  roles?: string[];
  capabilities?: {
    pipelines?: ResourceCapabilities;
    steps?: ResourceCapabilities;
    triggers?: ReadCapabilities;
    scopes?: ReadCapabilities;
    knowledge_contexts?: ReadCapabilities;
    system?: SystemCapabilities;
  };
};

function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value)
    .trim()
    .replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

type AuthSession = StoredSession;

type RunGroup = {
  id: number;
  name: string;
  parent_id?: number | null;
  description?: string;
};

type RunListItem = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
  pipeline_source?: string;
  status: string;
  git_commit_sha?: string;
  git_repo_name?: string;
  git_repo_owner?: string;
  git_ref?: string;
  git_target_ref?: string;
  git_pusher_name?: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
  is_complete?: boolean;
  parent_run_id?: string | null;
  trigger_event_id?: string;
};

type RunDetail = {
  run_info?: RunListItem;
};

type RunTabKey = 'main' | 'recent' | 'events';

const baseNavItems: NavItem[] = [
  {
    label: 'Pipeline runs',
    path: '/pipelineruns/main',
    icon: <IconPlay />, 
  },
  {
    label: 'Monitoring',
    path: '/monitoring',
    icon: <IconMonitoring />,
  },
  {
    label: 'Pipelines',
    path: '/pipelines',
    icon: <IconFlow />, 
  },
  {
    label: 'Triggers',
    path: '/triggers',
    icon: <IconBell />, 
  },
  {
    label: 'Scopes',
    path: '/scopes',
    icon: <IconScope />, 
  },
  {
    label: 'Lab',
    path: '/lab',
    icon: <IconFlask />, 
  },
  {
    label: 'Steps',
    path: '/steps',
    icon: <IconSteps />,
  },
  {
    label: 'Knowledge Context',
    path: '/knowledge-context',
    icon: <IconKnowledge />,
  },
  {
    label: 'System',
    path: '/system/config',
    icon: <IconCog />,
  },
];

const baseSystemSubNav: NavItem[] = [
  { label: 'Config', path: '/system/config', icon: <IconCog /> },
  { label: 'Setup', path: '/system/setup', icon: <IconShield /> },
  { label: 'LLM Profiles', path: '/system/llm-profiles', icon: <IconFlask /> },
  { label: 'MCP', path: '/system/mcp', icon: <IconFlask /> },
  { label: 'Dispatcher', path: '/system/dispatcher', icon: <IconDispatch /> },
  { label: 'Access', path: '/system/access', icon: <IconShield /> },
];

const titleMap: Record<string, string> = {
  pipelineruns: 'Pipeline runs',
  monitoring: 'Monitoring',
  pipelines: 'Pipelines',
  triggers: 'Triggers',
  scopes: 'Scopes',
  lab: 'Lab',
  steps: 'Steps',
  'knowledge-context': 'Knowledge Context',
  system: 'System',
  profile: 'Profile',
};

const STATUS_PRIORITY = ['failure', 'failure (ignored)', 'cancelled', 'running', 'pending', 'skipped', 'success'];
const SIDEBAR_MIN_WIDTH = 260;
const SIDEBAR_MAX_WIDTH = 520;
const SIDEBAR_DEFAULT_WIDTH = 320;
const SIDEBAR_RECENT_PAGE_SIZE = 200;
const SIDEBAR_SCROLL_BUFFER = 200;
const KNOWLEDGE_CONTEXT_KIND_ORDER = ['architecture', 'guardrail', 'policy', 'adr', 'guideline', 'runbook', 'reference', 'example'];
const KNOWLEDGE_CONTEXTS_CHANGED_EVENT = 'nopsai-knowledge-contexts-changed';
const SETUP_REDIRECTED_KEY = 'nopsai.setup.redirected';

const getInitialTheme = (): Theme => {
  if (typeof window === 'undefined') return 'light';
  const stored = localStorage.getItem('theme');
  if (stored === 'dark' || stored === 'light') return stored;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
};

function App() {
  return (
    <HashRouter>
      <AppShell />
    </HashRouter>
  );
}

function PageLoading() {
  return <div className="p-6 text-sm text-[var(--text-secondary)]">Loading...</div>;
}

function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const [theme, setTheme] = useState<Theme>(getInitialTheme);
  const [authSession, setAuthSession] = useState<AuthSession>(() => getStoredSession());
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);
  const [currentUserLoading, setCurrentUserLoading] = useState(() => Boolean(getStoredSession().accessToken));
  const isAuthenticated = useMemo(() => Boolean(authSession.accessToken), [authSession.accessToken]);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState<number>(() => {
    if (typeof window === 'undefined') return SIDEBAR_DEFAULT_WIDTH;
    const stored = Number(localStorage.getItem('sidebarWidth'));
    if (Number.isFinite(stored)) return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, stored));
    return SIDEBAR_DEFAULT_WIDTH;
  });
  const [isResizingSidebar, setIsResizingSidebar] = useState(false);
  const resizeStartXRef = useRef(0);
  const resizeStartWidthRef = useRef(SIDEBAR_DEFAULT_WIDTH);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const serverPipelinesRef = useRef<string[]>([]);
  const [pipelineTreeOpen, setPipelineTreeOpen] = useState<Set<string>>(new Set());

  const [triggers, setTriggers] = useState<string[]>([]);
  const [triggerTreeOpen, setTriggerTreeOpen] = useState<Set<string>>(new Set());

  const [steps, setSteps] = useState<string[]>([]);
  const serverStepsRef = useRef<string[]>([]);
  const [stepTreeOpen, setStepTreeOpen] = useState<Set<string>>(new Set());

  const [scopes, setScopes] = useState<string[]>([]);
  const serverScopesRef = useRef<string[]>([]);
  const [scopeTreeOpen, setScopeTreeOpen] = useState<Set<string>>(new Set());

  const [knowledgeContexts, setKnowledgeContexts] = useState<string[]>([]);
  const [knowledgeContextTreeOpen, setKnowledgeContextTreeOpen] = useState<Set<string>>(new Set());
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);
  const setupRedirectCheckedRef = useRef('');

  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'dark') {
      root.classList.add('dark');
      localStorage.setItem('theme', 'dark');
    } else {
      root.classList.remove('dark');
      localStorage.setItem('theme', 'light');
    }
  }, [theme]);

  useEffect(() => {
    const handleAuthChange = () => {
      setAuthSession(getStoredSession());
    };
    window.addEventListener('storage', handleAuthChange);
    window.addEventListener('nopsai-auth-changed', handleAuthChange as EventListener);
    return () => {
      window.removeEventListener('storage', handleAuthChange);
      window.removeEventListener('nopsai-auth-changed', handleAuthChange as EventListener);
    };
  }, []);

  useEffect(() => {
    const isAuthenticated = Boolean(authSession.accessToken);
    if (!isAuthenticated && location.pathname !== '/login') {
      navigate('/login', { replace: true });
    }
    if (isAuthenticated && authSession.mustChangePassword && location.pathname !== '/profile') {
      navigate('/profile', { replace: true });
    }
    if (isAuthenticated && location.pathname === '/login') {
      navigate(authSession.mustChangePassword ? '/profile' : '/pipelineruns/main', { replace: true });
    }
  }, [authSession.accessToken, authSession.mustChangePassword, location.pathname, navigate]);

  useEffect(() => {
    if (!authSession.accessToken) {
      const handle = window.setTimeout(() => {
        setCurrentUser(null);
        setCurrentUserLoading(false);
      }, 0);
      return () => window.clearTimeout(handle);
    }
    let cancelled = false;
    const loadingHandle = window.setTimeout(() => {
      if (!cancelled) setCurrentUserLoading(true);
    }, 0);
    fetch(buildApiUrl('/v1/auth/me'))
      .then(resp => {
        if (resp.status === 401 || resp.status === 403 || resp.status === 404) throw new Error('session-invalid');
        if (!resp.ok) throw new Error(`Failed to load user (${resp.status})`);
        return resp.json();
      })
      .then(data => {
        if (cancelled) return;
        const capabilities =
          data?.capabilities && typeof data.capabilities === 'object'
            ? {
                pipelines:
                  data.capabilities.pipelines && typeof data.capabilities.pipelines === 'object'
                    ? {
                        write: Boolean(data.capabilities.pipelines.write),
                        delete: Boolean(data.capabilities.pipelines.delete),
                      }
                    : undefined,
                steps:
                  data.capabilities.steps && typeof data.capabilities.steps === 'object'
                    ? {
                        write: Boolean(data.capabilities.steps.write),
                        delete: Boolean(data.capabilities.steps.delete),
                      }
                    : undefined,
                triggers:
                  data.capabilities.triggers && typeof data.capabilities.triggers === 'object'
                    ? {
                        read: Boolean(data.capabilities.triggers.read),
                        write: Boolean(data.capabilities.triggers.write),
                        delete: Boolean(data.capabilities.triggers.delete),
                      }
                    : undefined,
                scopes:
                  data.capabilities.scopes && typeof data.capabilities.scopes === 'object'
                    ? {
                        read: Boolean(data.capabilities.scopes.read),
                        write: Boolean(data.capabilities.scopes.write),
                        delete: Boolean(data.capabilities.scopes.delete),
                      }
                    : undefined,
                knowledge_contexts:
                  data.capabilities.knowledge_contexts && typeof data.capabilities.knowledge_contexts === 'object'
                    ? {
                        read: Boolean(data.capabilities.knowledge_contexts.read),
                        write: Boolean(data.capabilities.knowledge_contexts.write),
                        delete: Boolean(data.capabilities.knowledge_contexts.delete),
                      }
                    : undefined,
                system:
                  data.capabilities.system && typeof data.capabilities.system === 'object'
                    ? {
                        configRead: Boolean(data.capabilities.system.config_read),
                        configWrite: Boolean(data.capabilities.system.config_write),
                        llmProfilesRead: Boolean(data.capabilities.system.llm_profiles_read),
                        llmProfilesWrite: Boolean(data.capabilities.system.llm_profiles_write),
                        mcpRead: Boolean(data.capabilities.system.mcp_read),
                        mcpWrite: Boolean(data.capabilities.system.mcp_write),
                        configReposRead: Boolean(data.capabilities.system.config_repos_read),
                        configReposWrite: Boolean(data.capabilities.system.config_repos_write),
                        dispatcherRead: Boolean(data.capabilities.system.dispatcher_read),
                        dispatcherWrite: Boolean(data.capabilities.system.dispatcher_write),
                        access: Boolean(data.capabilities.system.access),
                      }
                    : undefined,
              }
            : undefined;
        setCurrentUser({
          sub: data?.sub || '',
          email: data?.email || '',
          roles: Array.isArray(data?.roles) ? data.roles : undefined,
          capabilities,
        });
        setPasswordChangeRequired(Boolean(data?.must_change_password));
      })
      .catch(err => {
        if (cancelled) return;
        setCurrentUser(null);
        if (err instanceof Error && err.message === 'session-invalid') {
          clearSession();
          setAuthSession(getStoredSession());
        }
      })
      .finally(() => {
        if (!cancelled) setCurrentUserLoading(false);
      });
    return () => {
      cancelled = true;
      window.clearTimeout(loadingHandle);
    };
  }, [authSession.accessToken]);

  const handleLoginSuccess = useCallback(() => {
    setAuthSession(getStoredSession());
  }, []);

  const handleOpenProfile = useCallback(() => {
    navigate('/profile');
  }, [navigate]);

  const handleLogout = useCallback(() => {
    clearSession();
    setAuthSession({});
    navigate('/login', { replace: true });
  }, [navigate]);

  const handleUserUpdated = useCallback((updates: Partial<CurrentUser>) => {
    setCurrentUser(prev => (prev ? { ...prev, ...updates } : prev));
  }, []);

  const draftScope = useMemo(() => {
    const sub = (authSession.sub || currentUser?.sub || '').trim();
    return sub;
  }, [authSession.sub, currentUser?.sub]);

  const canWritePipelines = Boolean(currentUser?.capabilities?.pipelines?.write);
  const canDeletePipelines = Boolean(currentUser?.capabilities?.pipelines?.delete);
  const canWriteSteps = Boolean(currentUser?.capabilities?.steps?.write);
  const canDeleteSteps = Boolean(currentUser?.capabilities?.steps?.delete);
  const canViewTriggers = Boolean(currentUser?.capabilities?.triggers?.read);
  const canDeleteTriggers = Boolean(currentUser?.capabilities?.triggers?.delete);
  const canViewScopes = Boolean(currentUser?.capabilities?.scopes?.read);
  const canDeleteScopes = Boolean(currentUser?.capabilities?.scopes?.delete);
  const canViewKnowledge = Boolean(currentUser?.capabilities?.knowledge_contexts?.read);
  const canWriteKnowledge = Boolean(currentUser?.capabilities?.knowledge_contexts?.write);
  const canDeleteKnowledge = Boolean(currentUser?.capabilities?.knowledge_contexts?.delete);
  const canViewSystemRuntimeConfig = Boolean(currentUser?.capabilities?.system?.configRead);
  const canManageSystemRuntimeConfig = Boolean(currentUser?.capabilities?.system?.configWrite);
  const canViewSystemLLMProfiles = Boolean(currentUser?.capabilities?.system?.llmProfilesRead) || canViewSystemRuntimeConfig;
  const canManageSystemLLMProfiles = Boolean(currentUser?.capabilities?.system?.llmProfilesWrite) || canManageSystemRuntimeConfig;
  const canViewSystemMCP = Boolean(currentUser?.capabilities?.system?.mcpRead) || canViewSystemRuntimeConfig;
  const canManageSystemMCP = Boolean(currentUser?.capabilities?.system?.mcpWrite) || canManageSystemRuntimeConfig;
  const canViewSystemConfigRepo = Boolean(currentUser?.capabilities?.system?.configReposRead);
  const canManageSystemConfigRepo = Boolean(currentUser?.capabilities?.system?.configReposWrite);
  const canViewSystemConfig = canViewSystemRuntimeConfig || canViewSystemConfigRepo;
  const canViewSystemSetup = canViewSystemRuntimeConfig;
  const canManageSystemSetup = canManageSystemRuntimeConfig;
  const canViewSystemDispatcher = Boolean(currentUser?.capabilities?.system?.dispatcherRead);
  const canManageSystemDispatcher = Boolean(currentUser?.capabilities?.system?.dispatcherWrite);
  const canViewSystemAccess = Boolean(currentUser?.capabilities?.system?.access);
  const isInitialAdminUser = useMemo(() => {
    const sub = (currentUser?.sub || authSession.sub || '').trim().toLowerCase();
    const roles = currentUser?.roles || authSession.roles || [];
    return sub === 'admin' || roles.some(role => role === 'nopsai-admin');
  }, [authSession.roles, authSession.sub, currentUser?.roles, currentUser?.sub]);
  const canViewAnySystem = canViewSystemConfig || canViewSystemSetup || canViewSystemLLMProfiles || canViewSystemMCP || canViewSystemDispatcher || canViewSystemAccess;
  const preferredSystemPath = canViewSystemConfig
    ? '/system/config'
    : canViewSystemSetup
      ? '/system/setup'
    : canViewSystemLLMProfiles
      ? '/system/llm-profiles'
      : canViewSystemMCP
        ? '/system/mcp'
        : canViewSystemDispatcher
          ? '/system/dispatcher'
          : canViewSystemAccess
            ? '/system/access'
            : '/system/config';
  const navItems = useMemo(() => {
    return baseNavItems
      .map(item => (item.path.startsWith('/system') ? { ...item, path: preferredSystemPath } : item))
      .filter(item => {
        if (item.path.startsWith('/system')) return canViewAnySystem;
        if (item.path === '/triggers') return canViewTriggers;
        if (item.path === '/scopes') return canViewScopes;
        if (item.path === '/knowledge-context') return canViewKnowledge;
        return true;
      });
  }, [canViewAnySystem, canViewKnowledge, canViewScopes, canViewTriggers, preferredSystemPath]);
  const systemSubNav = useMemo(
    () =>
      baseSystemSubNav.filter(item => {
        if (item.path === '/system/config') return canViewSystemConfig;
        if (item.path === '/system/setup') return canViewSystemSetup;
        if (item.path === '/system/llm-profiles') return canViewSystemLLMProfiles;
        if (item.path === '/system/mcp') return canViewSystemMCP;
        if (item.path === '/system/dispatcher') return canViewSystemDispatcher;
        if (item.path === '/system/access') return canViewSystemAccess;
        return false;
      }),
    [canViewSystemAccess, canViewSystemConfig, canViewSystemDispatcher, canViewSystemLLMProfiles, canViewSystemMCP, canViewSystemSetup]
  );

  useEffect(() => {
    if (!isAuthenticated || authSession.mustChangePassword || currentUserLoading || !canViewSystemSetup) return;
    if (!isInitialAdminUser) return;
    if (location.pathname === '/system/setup') return;

    const subject = (currentUser?.sub || authSession.sub || 'current').trim() || 'current';
    const checkKey = `${subject}:${authSession.accessToken || ''}`;
    if (setupRedirectCheckedRef.current === checkKey) return;
    setupRedirectCheckedRef.current = checkKey;

    let cancelled = false;
    void fetch(buildApiUrl('/v1/setup/status'))
      .then(response => {
        if (!response.ok) throw new Error(`setup status failed (${response.status})`);
        return response.json() as Promise<SetupStatusSummary>;
      })
      .then(status => {
        if (cancelled || status.completed) return;
        const redirectKey = `${SETUP_REDIRECTED_KEY}.${subject}`;
        if (sessionStorage.getItem(redirectKey) === 'true') return;
        sessionStorage.setItem(redirectKey, 'true');
        navigate('/system/setup', { replace: true });
      })
      .catch(error => {
        console.warn('Failed to check setup status', error);
      });

    return () => {
      cancelled = true;
    };
  }, [
    authSession.accessToken,
    authSession.mustChangePassword,
    authSession.sub,
    canViewSystemSetup,
    currentUser?.sub,
    currentUserLoading,
    isInitialAdminUser,
    isAuthenticated,
    location.pathname,
    navigate,
  ]);

  useEffect(() => {
    if (!sidebarOpen) return;
    const handle = window.setTimeout(() => setSidebarOpen(false), 0);
    return () => window.clearTimeout(handle);
  }, [location.pathname, sidebarOpen]);

  useEffect(() => {
    if (!isAuthenticated) {
      const handle = window.setTimeout(() => setResourceGroupPaths([]), 0);
      return () => window.clearTimeout(handle);
    }
    let cancelled = false;
    const loadResourceGroups = () => {
      void fetchResourceGroupPaths()
        .then(paths => {
          if (!cancelled) setResourceGroupPaths(paths);
        })
        .catch(error => {
          console.warn('Failed to load groups for resource trees', error);
          if (!cancelled) setResourceGroupPaths([]);
        });
    };
    loadResourceGroups();
    window.addEventListener('nopsai-resource-groups-changed', loadResourceGroups);
    return () => {
      cancelled = true;
      window.removeEventListener('nopsai-resource-groups-changed', loadResourceGroups);
    };
  }, [isAuthenticated]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const [secretResp, variableResp] = await Promise.all([
          fetch(buildApiUrl('/v1/secrets/scopes')),
          fetch(buildApiUrl('/v1/variables/scopes')),
        ]);
        const secretJson = secretResp.ok ? await secretResp.json() : [];
        const variableJson = variableResp.ok ? await variableResp.json() : [];
        const scopeSet = new Set<string>();
        scopeSet.add('');
        if (Array.isArray(secretJson)) {
          secretJson.forEach((entry: unknown) => {
            if (!entry || typeof entry !== 'object') return;
            const record = entry as Record<string, unknown>;
            const scopeLabel = typeof record.scope === 'string' ? record.scope : '';
            scopeSet.add(normalizeScopeLabel(scopeLabel));
          });
        }
        if (Array.isArray(variableJson)) {
          variableJson.forEach((entry: unknown) => {
            if (typeof entry === 'string') {
              scopeSet.add(normalizeScopeLabel(entry));
              return;
            }
            if (!entry || typeof entry !== 'object') return;
            const record = entry as Record<string, unknown>;
            const scopeLabel = typeof record.scope === 'string'
              ? record.scope
              : typeof record.name === 'string'
                ? record.name
                : '';
            scopeSet.add(normalizeScopeLabel(scopeLabel));
          });
        }
        const list = Array.from(scopeSet).map(normalizeScopeLabel).sort((a, b) => a.localeCompare(b));
        serverScopesRef.current = list;
        setScopes(list);
      } catch (error) {
        console.warn('Failed to load scopes for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/scopes')) {
      void load();
    }
  }, [isAuthenticated, location.pathname]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/pipelines'));
        if (!response.ok) return;
        const payload = await response.json();
        const asRecord = (value: unknown): Record<string, unknown> | null => {
          if (!value || typeof value !== 'object') return null;
          return value as Record<string, unknown>;
        };
        const ids = Array.isArray(payload)
          ? payload
              .map((item: unknown) => {
                if (typeof item === 'string') return item;
                const record = asRecord(item);
                if (!record) return '';
                if (typeof record.id === 'string') return record.id;
                if (typeof record.identifier === 'string') return record.identifier;
                return '';
              })
              .filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        serverPipelinesRef.current = ids;
        const draftIds = canWritePipelines && draftScope ? loadPipelineDrafts(draftScope).map(draft => draft.id) : [];
        const merged = Array.from(new Set([...ids, ...draftIds])).sort((a, b) => a.localeCompare(b));
        setPipelines(merged);
      } catch (error) {
        console.warn('Failed to load pipelines for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/pipelines')) {
      void load();
    }
  }, [canWritePipelines, draftScope, isAuthenticated, location.pathname]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/overrides'));
        if (!response.ok) return;
        const payload = await response.json();
        const slugs = Array.isArray(payload)
          ? payload.map((item: unknown) => (typeof item === 'string' ? item.trim() : '')).filter(Boolean)
          : [];
        slugs.sort((a: string, b: string) => a.localeCompare(b));
        setTriggers(slugs);
      } catch (error) {
        console.warn('Failed to load triggers for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/triggers')) {
      void load();
    }
  }, [isAuthenticated, location.pathname]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/steps'));
        if (!response.ok) return;
        const payload = await response.json();
        const ids = Array.isArray(payload)
          ? payload.map((item: unknown) => (typeof item === 'string' ? item.trim() : '')).filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        serverStepsRef.current = ids;
        const draftIds = canWriteSteps && draftScope ? loadStepDrafts(draftScope).map(draft => draft.id) : [];
        const merged = Array.from(new Set([...ids, ...draftIds])).sort((a, b) => a.localeCompare(b));
        setSteps(merged);
      } catch (error) {
        console.warn('Failed to load steps for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/steps')) {
      void load();
    }
  }, [canWriteSteps, draftScope, isAuthenticated, location.pathname]);

  useEffect(() => {
    if (!isAuthenticated || !canViewKnowledge) {
      const handle = window.setTimeout(() => setKnowledgeContexts([]), 0);
      return () => window.clearTimeout(handle);
    }
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/knowledge-contexts'));
        if (!response.ok) return;
        const payload = await response.json();
        const ids = Array.isArray(payload)
          ? payload
              .map((item: unknown) => {
                if (typeof item === 'string') return item.trim();
                if (!item || typeof item !== 'object') return '';
                const record = item as Record<string, unknown>;
                return typeof record.id === 'string' ? record.id.trim() : '';
              })
              .filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        setKnowledgeContexts(ids);
      } catch (error) {
        console.warn('Failed to load knowledge contexts for sidebar', error);
      }
    };
    const handleKnowledgeContextsChanged = () => {
      if (location.pathname.startsWith('/knowledge-context')) {
        void load();
      }
    };
    window.addEventListener(KNOWLEDGE_CONTEXTS_CHANGED_EVENT, handleKnowledgeContextsChanged);
    if (location.pathname.startsWith('/knowledge-context')) {
      void load();
    }
    return () => {
      window.removeEventListener(KNOWLEDGE_CONTEXTS_CHANGED_EVENT, handleKnowledgeContextsChanged);
    };
  }, [canViewKnowledge, isAuthenticated, location.pathname]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (!canWritePipelines || !draftScope) return;
    const storageKey = getPipelineDraftStorageKey(draftScope);
    const handleDraftsChanged = () => {
      if (!location.pathname.startsWith('/pipelines')) return;
      const draftIds = loadPipelineDrafts(draftScope).map(draft => draft.id);
      const merged = Array.from(new Set([...serverPipelinesRef.current, ...draftIds])).sort((a, b) => a.localeCompare(b));
      setPipelines(merged);
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== storageKey) return;
      handleDraftsChanged();
    };
    window.addEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
      window.removeEventListener('storage', handleStorage);
    };
  }, [canWritePipelines, draftScope, location.pathname]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (!canWriteSteps || !draftScope) return;
    const storageKey = getStepDraftStorageKey(draftScope);
    const handleDraftsChanged = () => {
      if (!location.pathname.startsWith('/steps')) return;
      const draftIds = loadStepDrafts(draftScope).map(draft => draft.id);
      const merged = Array.from(new Set([...serverStepsRef.current, ...draftIds])).sort((a, b) => a.localeCompare(b));
      setSteps(merged);
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== storageKey) return;
      handleDraftsChanged();
    };
    window.addEventListener(STEP_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(STEP_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
      window.removeEventListener('storage', handleStorage);
    };
  }, [canWriteSteps, draftScope, location.pathname]);

  const title = useMemo(() => {
    const key = location.pathname.split('/').filter(Boolean)[0] || 'pipelineruns';
    return titleMap[key] || 'Dashboard';
  }, [location.pathname]);
  const isLoginRoute = location.pathname === '/login';

  const splitIdentifier = (id: string) => {
    const parts = id.split('/').filter(Boolean);
    const name = decodeURIComponent(parts.pop() || '');
    const path = parts.map(decodeURIComponent).join('/');
    return { name, path };
  };

  const buildTree = useMemo(() => {
    const root: PipelineTreeNode = { id: '__root__', name: 'All pipelines', fullPath: '', children: [], pipelineIds: [] };
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], pipelineIds: [] }));
    });
    pipelines.forEach(id => {
      const parts = id.split('/').filter(Boolean);
      const pipelineName = parts.pop();
      if (!pipelineName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], pipelineIds: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.pipelineIds.push(id);
      current.pipelineIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [pipelines, resourceGroupPaths]);

  const buildTriggerTree = useMemo(() => {
    const root: TriggerTreeNode = { id: '__root__', name: 'All triggers', fullPath: '', children: [], triggerSlugs: [] };
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], triggerSlugs: [] }));
    });
    triggers.forEach(slug => {
      const parts = slug.split('/').filter(Boolean);
      const repoName = parts.pop();
      if (!repoName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], triggerSlugs: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.triggerSlugs.push(slug);
      current.triggerSlugs.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [resourceGroupPaths, triggers]);

  const buildStepTree = useMemo(() => {
    const root: StepTreeNode = { id: '__root__', name: 'All steps', fullPath: '', children: [], stepIds: [] };
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], stepIds: [] }));
    });
    steps.forEach(id => {
      const parts = id.split('/').filter(Boolean);
      const stepName = parts.pop();
      if (!stepName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], stepIds: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.stepIds.push(id);
      current.stepIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [resourceGroupPaths, steps]);

  const buildScopeTree = useMemo(() => {
    const root: ScopeTreeNode = { id: '__root__', name: 'All scopes', fullPath: '', children: [], scopes: [] };
    resourceGroupPaths.forEach(path => {
      insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], scopes: [] }));
    });
    scopes.forEach(scope => {
      const normalized = normalizeScopeLabel(scope);
      const parts = normalized.split('/').filter(Boolean);
      if (!parts.length) {
        root.scopes.push('');
        return;
      }
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], scopes: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.scopes.push(normalized);
      current.scopes.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [resourceGroupPaths, scopes]);

  const buildKnowledgeContextTree = useMemo(() => {
    const root: KnowledgeContextTreeNode = { id: '__root__', name: 'knowledge contexts', fullPath: '', children: [], knowledgeContextIds: [] };
    const folderRank = (name: string) => {
      const index = KNOWLEDGE_CONTEXT_KIND_ORDER.indexOf(name);
      return index < 0 ? KNOWLEDGE_CONTEXT_KIND_ORDER.length : index;
    };
    const sortChildren = (node: KnowledgeContextTreeNode) => {
      node.children.sort((a, b) => folderRank(a.name) - folderRank(b.name) || a.name.localeCompare(b.name));
      node.knowledgeContextIds.sort((a, b) => a.localeCompare(b));
      node.children.forEach(sortChildren);
    };
    const ensureChild = (parent: KnowledgeContextTreeNode, segment: string, fullPath: string) => {
      let child = parent.children.find(c => c.name === segment);
      if (!child) {
        child = { id: fullPath, name: segment, fullPath, children: [], knowledgeContextIds: [] };
        parent.children.push(child);
      }
      return child;
    };

    KNOWLEDGE_CONTEXT_KIND_ORDER.forEach(kind => {
      ensureChild(root, kind, kind);
    });
    KNOWLEDGE_CONTEXT_KIND_ORDER.forEach(kind => {
      resourceGroupPaths.forEach(groupPath => {
        const normalizedGroup = groupPath.split('/').map(part => part.trim()).filter(Boolean).join('/');
        if (!normalizedGroup) return;
        insertGroupPath(root, `${kind}/${normalizedGroup}`, (id, name, fullPath) => ({ id, name, fullPath, children: [], knowledgeContextIds: [] }));
      });
    });

    knowledgeContexts.forEach(id => {
      const parts = id.split('/').filter(Boolean);
      const documentName = parts.pop();
      if (!documentName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        current = ensureChild(current, segment, pathSoFar);
      });
      current.knowledgeContextIds.push(id);
    });

    sortChildren(root);
    return root;
  }, [knowledgeContexts, resourceGroupPaths]);

  const handleTogglePipelineNode = (id: string) => {
    setPipelineTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleToggleTriggerNode = (id: string) => {
    setTriggerTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleToggleStepNode = (id: string) => {
    setStepTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleToggleScopeNode = (id: string) => {
    setScopeTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleToggleKnowledgeContextNode = (id: string) => {
    setKnowledgeContextTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const clampSidebarWidth = useCallback(
    (value: number) => Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, value)),
    []
  );

  const startSidebarResize = (event: React.MouseEvent | React.TouchEvent) => {
    if (typeof window !== 'undefined' && window.innerWidth < 640) return;
    const clientX = 'touches' in event ? event.touches[0]?.clientX : event.clientX;
    if (typeof clientX !== 'number') return;
    resizeStartXRef.current = clientX;
    resizeStartWidthRef.current = sidebarWidth;
    setIsResizingSidebar(true);
    event.stopPropagation();
    event.preventDefault();
  };

  useEffect(() => {
    if (typeof window === 'undefined') return;
    localStorage.setItem('sidebarWidth', String(sidebarWidth));
  }, [sidebarWidth]);

  useEffect(() => {
    if (!isResizingSidebar) return undefined;
    const handleMove = (event: MouseEvent | TouchEvent) => {
      const clientX = 'touches' in event ? event.touches[0]?.clientX : event.clientX;
      if (typeof clientX !== 'number') return;
      const delta = clientX - resizeStartXRef.current;
      setSidebarWidth(clampSidebarWidth(resizeStartWidthRef.current + delta));
    };
    const handleUp = () => setIsResizingSidebar(false);
    window.addEventListener('mousemove', handleMove);
    window.addEventListener('touchmove', handleMove);
    window.addEventListener('mouseup', handleUp);
    window.addEventListener('touchend', handleUp);
    return () => {
      window.removeEventListener('mousemove', handleMove);
      window.removeEventListener('touchmove', handleMove);
      window.removeEventListener('mouseup', handleUp);
      window.removeEventListener('touchend', handleUp);
    };
  }, [clampSidebarWidth, isResizingSidebar]);

  useEffect(() => {
    if (typeof document === 'undefined') return undefined;
    if (!isResizingSidebar) return undefined;
    const prevCursor = document.body.style.cursor;
    const prevUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    return () => {
      document.body.style.cursor = prevCursor;
      document.body.style.userSelect = prevUserSelect;
    };
  }, [isResizingSidebar]);

  const renderAccessControlledPage = useCallback(
    (allowed: boolean, element: ReactNode) => {
      if (currentUserLoading) {
        return <div className="p-6 text-sm text-[var(--text-secondary)]">Loading access…</div>;
      }
      return allowed ? element : <Navigate to="/pipelineruns/main" replace />;
    },
    [currentUserLoading]
  );

  return (
    <div className="min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)]">
      {isLoginRoute ? (
        <Suspense fallback={<PageLoading />}>
          <LoginPage onLogin={handleLoginSuccess} />
        </Suspense>
      ) : !isAuthenticated ? (
        <Navigate to="/login" replace />
      ) : (
        <>
          <div id="hover-hint" aria-hidden="true"></div>
          <div className="flex h-screen overflow-hidden">
            <Sidebar
              navItems={navItems}
              systemSubNav={systemSubNav}
              open={sidebarOpen}
              onClose={() => setSidebarOpen(false)}
              width={sidebarWidth}
              pipelineTree={buildTree}
              pipelineTreeOpen={pipelineTreeOpen}
              onTogglePipelineNode={handleTogglePipelineNode}
              triggerTree={buildTriggerTree}
              triggerTreeOpen={triggerTreeOpen}
              onToggleTriggerNode={handleToggleTriggerNode}
              stepTree={buildStepTree}
              stepTreeOpen={stepTreeOpen}
              onToggleStepNode={handleToggleStepNode}
              scopeTree={buildScopeTree}
              scopeTreeOpen={scopeTreeOpen}
              onToggleScopeNode={handleToggleScopeNode}
              knowledgeContextTree={buildKnowledgeContextTree}
              knowledgeContextTreeOpen={knowledgeContextTreeOpen}
              onToggleKnowledgeContextNode={handleToggleKnowledgeContextNode}
              splitIdentifier={splitIdentifier}
              locationPathname={location.pathname}
              locationSearch={location.search}
              navigateTo={navigate}
              onSelectPipelineFolder={path => navigate(path ? `/pipelines?folder=${encodeURIComponent(path)}` : '/pipelines')}
              onSelectTriggerFolder={path => navigate(path ? `/triggers?folder=${encodeURIComponent(path)}` : '/triggers')}
              onSelectStepFolder={path => navigate(path ? `/steps?folder=${encodeURIComponent(path)}` : '/steps')}
              onSelectScopeFolder={path => navigate(path ? `/scopes?folder=${encodeURIComponent(path)}` : '/scopes')}
              onSelectKnowledgeContextFolder={path => navigate(path ? `/knowledge-context?folder=${encodeURIComponent(path)}` : '/knowledge-context')}
            />
            <div
              id="sidebar-resizer"
              className={`hidden sm:block w-1.5 cursor-col-resize flex-shrink-0 transition-colors duration-200 ${isResizingSidebar ? 'bg-[var(--border-accent)]' : 'bg-[var(--bg-tertiary)] hover:bg-[var(--border-accent)]'}`}
              onMouseDown={startSidebarResize}
              onTouchStart={startSidebarResize}
              aria-label="Resize sidebar"
            ></div>
            <main className="flex-1 flex flex-col overflow-hidden">
              <Header
                title={title}
                theme={theme}
                onToggleTheme={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                onOpenSidebar={() => setSidebarOpen(true)}
                onLogout={handleLogout}
                currentUser={currentUser}
                userLoading={currentUserLoading}
                onOpenProfile={handleOpenProfile}
              />
              <div id="page-content-wrapper" className="flex-1 overflow-auto">
                <Suspense fallback={<PageLoading />}>
                  <Routes>
                    <Route path="/" element={<Navigate to="/pipelineruns/main" replace />} />
                    <Route path="/pipelineruns/:tab?" element={<PipelineRunsPage />} />
                    <Route path="/monitoring" element={<MonitoringPage />} />
                    <Route
                      path="/pipelines/*"
                      element={<PipelinesPage draftScope={draftScope} canDeletePipelines={canDeletePipelines} />}
                    />
                    <Route
                      path="/triggers/*"
                      element={renderAccessControlledPage(
                        canViewTriggers,
                        <TriggersPage canDeleteTriggers={canDeleteTriggers} />
                      )}
                    />
                    <Route
                      path="/scopes/*"
                      element={renderAccessControlledPage(
                        canViewScopes,
                        <ScopesPage canDeleteScopes={canDeleteScopes} />
                      )}
                    />
                    <Route path="/lab/*" element={<LabPage />} />
                    <Route
                      path="/steps/*"
                      element={<StepsPage draftScope={draftScope} canDeleteSteps={canDeleteSteps} />}
                    />
                    <Route
                      path="/knowledge-context/*"
                      element={renderAccessControlledPage(
                        canViewKnowledge,
                        <KnowledgeContextPage canWriteKnowledge={canWriteKnowledge} canDeleteKnowledge={canDeleteKnowledge} />
                      )}
                    />
                    <Route
                      path="/system/:tab?"
                      element={renderAccessControlledPage(
                        canViewAnySystem,
                        <SystemPage
                          permissions={{
                            canViewConfig: canViewSystemConfig,
                            canViewSetup: canViewSystemSetup,
                            canManageSetup: canManageSystemSetup,
                            canViewRuntimeConfig: canViewSystemRuntimeConfig,
                            canManageRuntimeConfig: canManageSystemRuntimeConfig,
                            canViewLLMProfiles: canViewSystemLLMProfiles,
                            canManageLLMProfiles: canManageSystemLLMProfiles,
                            canViewMCP: canViewSystemMCP,
                            canManageMCP: canManageSystemMCP,
                            canViewGlobalConfigRepo: canViewSystemConfigRepo,
                            canManageGlobalConfigRepo: canManageSystemConfigRepo,
                            canViewDispatcher: canViewSystemDispatcher,
                            canManageDispatcher: canManageSystemDispatcher,
                            canViewAccess: canViewSystemAccess,
                          }}
                        />
                      )}
                    />
                    <Route
                      path="/profile"
                      element={
                        <ProfilePage
                          user={currentUser}
                          loading={currentUserLoading}
                          onLogout={handleLogout}
                          onUserUpdated={handleUserUpdated}
                          mustChangePassword={Boolean(authSession.mustChangePassword)}
                          onPasswordChanged={() => setPasswordChangeRequired(false)}
                          canAccessSystem={canViewAnySystem}
                          systemPath={preferredSystemPath}
                        />
                      }
                    />
                    <Route path="*" element={<Navigate to="/pipelineruns/main" replace />} />
                  </Routes>
                </Suspense>
              </div>
            </main>
          </div>
          <div id="toast-container" className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3"></div>
        </>
      )}
    </div>
  );
}

function Sidebar({
  navItems,
  systemSubNav,
  open,
  onClose,
  width,
  pipelineTree,
  pipelineTreeOpen,
  onTogglePipelineNode,
  triggerTree,
  triggerTreeOpen,
  onToggleTriggerNode,
  stepTree,
  stepTreeOpen,
  onToggleStepNode,
  scopeTree,
  scopeTreeOpen,
  onToggleScopeNode,
  knowledgeContextTree,
  knowledgeContextTreeOpen,
  onToggleKnowledgeContextNode,
  splitIdentifier,
  locationPathname,
  locationSearch,
  navigateTo,
  onSelectPipelineFolder,
  onSelectTriggerFolder,
  onSelectStepFolder,
  onSelectScopeFolder,
  onSelectKnowledgeContextFolder,
}: {
  navItems: NavItem[];
  systemSubNav: NavItem[];
  open: boolean;
  onClose: () => void;
  width: number;
  pipelineTree: PipelineTreeNode;
  pipelineTreeOpen: Set<string>;
  onTogglePipelineNode: (id: string) => void;
  triggerTree: TriggerTreeNode;
  triggerTreeOpen: Set<string>;
  onToggleTriggerNode: (id: string) => void;
  stepTree: StepTreeNode;
  stepTreeOpen: Set<string>;
  onToggleStepNode: (id: string) => void;
  scopeTree: ScopeTreeNode;
  scopeTreeOpen: Set<string>;
  onToggleScopeNode: (id: string) => void;
  knowledgeContextTree: KnowledgeContextTreeNode;
  knowledgeContextTreeOpen: Set<string>;
  onToggleKnowledgeContextNode: (id: string) => void;
  splitIdentifier: (id: string) => { name: string; path: string };
  locationPathname: string;
  locationSearch: string;
  navigateTo: (path: string) => void;
  onSelectPipelineFolder: (path: string) => void;
  onSelectTriggerFolder: (path: string) => void;
  onSelectStepFolder: (path: string) => void;
  onSelectScopeFolder: (path: string) => void;
  onSelectKnowledgeContextFolder: (path: string) => void;
}) {
  const isPipelinesRoute = locationPathname.startsWith('/pipelines');
  const isTriggersRoute = locationPathname.startsWith('/triggers');
  const isStepsRoute = locationPathname.startsWith('/steps');
  const isScopesRoute = locationPathname.startsWith('/scopes');
  const isKnowledgeContextRoute = locationPathname.startsWith('/knowledge-context');
  const isPipelineRunsRoute = locationPathname.startsWith('/pipelineruns');
  const isSystemRoute = locationPathname.startsWith('/system');
  const searchParams = useMemo(() => new URLSearchParams(locationSearch), [locationSearch]);
  const pipelineRunsTab: RunTabKey =
    locationPathname.startsWith('/pipelineruns/recent') ? 'recent' : locationPathname.startsWith('/pipelineruns/events') ? 'events' : 'main';
  const activeFolder = searchParams.get('folder') || '';
  const encodeKnowledgeContextRoute = (id: string) => `/knowledge-context/${id.split('/').filter(Boolean).map(encodeURIComponent).join('/')}`;
  const activeKnowledgeContextID = (() => {
    const prefix = '/knowledge-context/';
    if (!locationPathname.startsWith(prefix)) return '';
    return locationPathname
      .slice(prefix.length)
      .split('/')
      .filter(Boolean)
      .map(part => {
        try {
          return decodeURIComponent(part);
        } catch {
          return part;
        }
      })
      .join('/');
  })();

  const renderPipelineTreeNode = (node: PipelineTreeNode) => {
    const isOpen = pipelineTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={node.id} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onTogglePipelineNode(node.id)}
              aria-label="Toggle group"
            >
              <svg
                className={`h-3.5 w-3.5 transition-transform ${isOpen ? 'rotate-90' : ''}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M9 5l7 7-7 7" />
              </svg>
            </button>
            <button
              className={`pipeline-tree-folder flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onTogglePipelineNode(node.id);
                onSelectPipelineFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderPipelineTreeNode(child))}
            {node.pipelineIds.map(pid => {
              const { name } = splitIdentifier(pid);
              const active = locationPathname.includes(`/pipelines/${pid}`);
              return (
                <li key={pid} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={`/pipelines/${pid.split('/').map(encodeURIComponent).join('/')}`}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="2.5" />
                        <path d="M4 12h3m10 0h3M12 4v3m0 10v3" />
                      </svg>
                    </span>
                    <span className="truncate">{name || pid}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  const renderTriggerTreeNode = (node: TriggerTreeNode) => {
    const isOpen = triggerTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={`tr-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onToggleTriggerNode(node.id)}
              aria-label="Toggle group"
            >
              <svg
                className={`h-3.5 w-3.5 transition-transform ${isOpen ? 'rotate-90' : ''}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M9 5l7 7-7 7" />
              </svg>
            </button>
            <button
              className={`pipeline-tree-folder flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleTriggerNode(node.id);
                onSelectTriggerFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderTriggerTreeNode(child))}
            {node.triggerSlugs.map(slug => {
              const { name } = splitIdentifier(slug);
              const active = locationPathname.includes(`/triggers/${slug}`);
              return (
                <li key={`slug-${slug}`} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={`/triggers/${slug.split('/').map(encodeURIComponent).join('/')}`}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
                      </svg>
                    </span>
                    <span className="truncate">{name || slug}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  const renderStepTreeNode = (node: StepTreeNode) => {
    const isOpen = stepTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={`step-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onToggleStepNode(node.id)}
              aria-label="Toggle group"
            >
              <svg
                className={`h-3.5 w-3.5 transition-transform ${isOpen ? 'rotate-90' : ''}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M9 5l7 7-7 7" />
              </svg>
            </button>
            <button
              className={`pipeline-tree-folder flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleStepNode(node.id);
                onSelectStepFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderStepTreeNode(child))}
            {node.stepIds.map(stepId => {
              const { name } = splitIdentifier(stepId);
              const active = locationPathname.includes(`/steps/${stepId}`);
              return (
                <li key={`step-id-${stepId}`} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={`/steps/${stepId.split('/').map(encodeURIComponent).join('/')}`}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M12 2l8 4.5v11L12 22 4 17.5v-11L12 2z" />
                        <path d="M12 22v-7.5" />
                        <path d="M20 6.5l-8 4.5-8-4.5" />
                      </svg>
                    </span>
                    <span className="truncate">{name || stepId}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  const encodeScopeForRoute = (scope: string) => {
    const normalized = normalizeScopeLabel(scope);
    if (!normalized) return 'default';
    return normalized
      .split('/')
      .filter(Boolean)
      .map(encodeURIComponent)
      .join('/');
  };

  const renderScopeTreeNode = (node: ScopeTreeNode) => {
    const isOpen = scopeTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={`scope-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onToggleScopeNode(node.id)}
              aria-label="Toggle group"
            >
              <svg
                className={`h-3.5 w-3.5 transition-transform ${isOpen ? 'rotate-90' : ''}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M9 5l7 7-7 7" />
              </svg>
            </button>
            <button
              className={`pipeline-tree-folder flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleScopeNode(node.id);
                onSelectScopeFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderScopeTreeNode(child))}
            {node.scopes.map(scopeLabel => {
              const active = locationPathname.includes(`/scopes/${encodeScopeForRoute(scopeLabel)}`);
              const name = scopeLabel.split('/').filter(Boolean).pop() || 'Default';
              return (
                <li key={`scope-leaf-${scopeLabel || 'default'}`} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink
                    className="pipeline-tree-leaf-btn"
                    to={`/scopes/${encodeScopeForRoute(scopeLabel)}`}
                    onClick={() => onSelectScopeFolder(node.fullPath)}
                  >
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="7" />
                        <circle cx="12" cy="12" r="2.2" />
                        <path d="M12 4v2.4m0 11.2V20m8-8h-2.4M6.4 12H4" />
                        <path d="M16.4 7.6l-1.4 1.4m-6 6-1.4 1.4" />
                        <path d="M7.6 7.6l1.4 1.4m6 6 1.4 1.4" />
                      </svg>
                    </span>
                    <span className="truncate">{name}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  const renderKnowledgeContextTreeNode = (node: KnowledgeContextTreeNode) => {
    const isOpen = knowledgeContextTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={`knowledge-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onToggleKnowledgeContextNode(node.id)}
              aria-label="Toggle group"
            >
              <svg
                className={`h-3.5 w-3.5 transition-transform ${isOpen ? 'rotate-90' : ''}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M9 5l7 7-7 7" />
              </svg>
            </button>
            <button
              className={`pipeline-tree-folder flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleKnowledgeContextNode(node.id);
                onSelectKnowledgeContextFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderKnowledgeContextTreeNode(child))}
            {node.knowledgeContextIds.map(contextId => {
              const { name } = splitIdentifier(contextId);
              const active = activeKnowledgeContextID === contextId;
              return (
                <li key={`knowledge-id-${contextId}`} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={encodeKnowledgeContextRoute(contextId)}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                        <path d="M14 2v6h6" />
                        <path d="M8 13h8M8 17h5" />
                      </svg>
                    </span>
                    <span className="truncate">{name || contextId}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  return (
    <>
      <div
        className={`fixed inset-0 bg-[var(--bg-overlay)] sm:hidden transition-opacity duration-200 ${open ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}
        onClick={onClose}
      ></div>
      <aside
        id="sidebar"
        className={`bg-[var(--bg-secondary)] border-r border-[var(--border-primary)] flex-shrink-0 flex flex-col transition-transform duration-300 ease-in-out h-full z-20 w-80 sidebar-scrollbar overflow-hidden
          ${open ? 'translate-x-0' : '-translate-x-full'} sm:translate-x-0 fixed sm:static`}
        style={{ width, minWidth: SIDEBAR_MIN_WIDTH, maxWidth: SIDEBAR_MAX_WIDTH }}
      >
        <div className="flex items-center justify-between px-6 h-16 border-b border-[var(--border-primary)] flex-shrink-0">
          <div className="sidebar-brand" aria-label="NopsAI">
            <span className="sr-only">NopsAI</span>
            <img className="brand-mark brand-mark--light" src="/brand/nopsai-mark-light.png" alt="" aria-hidden="true" />
            <img className="brand-mark brand-mark--dark" src="/brand/nopsai-mark-dark.png" alt="" aria-hidden="true" />
            <span className="sidebar-brand__name" aria-hidden="true">NopsAI</span>
          </div>
          <button
            id="close-sidebar-btn"
            className="sm:hidden text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
            onClick={onClose}
            aria-label="Close sidebar"
          >
            <IconX />
          </button>
        </div>
        <nav id="sidebar-base-nav" className="px-4 py-4 flex-shrink-0 space-y-1">
          {navItems.map(item => {
            const isSystemItem = item.path.startsWith('/system');
            const isActive = locationPathname.startsWith(item.path);
            return (
              <div key={item.path} className="space-y-1">
                <NavLink
                  to={item.path}
                  className={({ isActive: linkActive }) =>
                    `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors sidebar-link ${
                      linkActive || (isSystemItem && isSystemRoute)
                        ? 'active text-[var(--text-primary)] bg-[var(--bg-tertiary)]'
                        : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]'
                    }`
                  }
                >
                  <span className="text-[var(--text-secondary)]">{item.icon}</span>
                  <span className="truncate">{item.label}</span>
                </NavLink>
                {isSystemItem && (isSystemRoute || isActive) && (
                  <div className="pl-9 space-y-1">
                    {systemSubNav.map(sub => (
                      <NavLink
                        key={sub.path}
                        to={sub.path}
                        className={({ isActive: subActive }) =>
                          `flex items-center gap-2 px-3 py-1.5 rounded-md text-sm transition-colors ${
                            subActive
                              ? 'text-[var(--text-primary)] bg-[var(--bg-tertiary)]'
                              : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]'
                          }`
                        }
                      >
                        <span className="text-[var(--text-secondary)]">{sub.icon}</span>
                        <span className="truncate">{sub.label}</span>
                      </NavLink>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </nav>
        <div className="flex-1 overflow-y-auto sidebar-scrollbar border-t border-[var(--border-primary)]">
          <nav id="sidebar-details-nav" className="px-4 py-4 space-y-2">
            {isPipelineRunsRoute ? (
              <PipelineRunsSidebarContent
                tab={pipelineRunsTab}
                searchParams={searchParams}
                navigateTo={navigateTo}
                onClose={onClose}
              />
            ) : isPipelinesRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All pipelines</p>
                </div>
                <ul className="pipeline-tree-list">
                  {renderPipelineTreeNode(pipelineTree)}
                </ul>
              </div>
            ) : isTriggersRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All triggers</p>
                </div>
                <ul className="pipeline-tree-list">
                  {renderTriggerTreeNode(triggerTree)}
                </ul>
              </div>
            ) : isStepsRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All steps</p>
                </div>
                <ul className="pipeline-tree-list">{renderStepTreeNode(stepTree)}</ul>
              </div>
            ) : isScopesRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All scopes</p>
                </div>
                <ul className="pipeline-tree-list">{renderScopeTreeNode(scopeTree)}</ul>
              </div>
            ) : isKnowledgeContextRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All knowledge contexts</p>
                </div>
                <ul className="pipeline-tree-list">{renderKnowledgeContextTreeNode(knowledgeContextTree)}</ul>
              </div>
            ) : (
              <p className="text-xs text-[var(--text-secondary)]">Contextual navigation will appear here as features are migrated.</p>
            )}
          </nav>
        </div>
      </aside>
    </>
  );
}

function PipelineRunsSidebarContent({
  tab,
  searchParams,
  navigateTo,
  onClose,
}: {
  tab: RunTabKey;
  searchParams: URLSearchParams;
  navigateTo: (path: string) => void;
  onClose?: () => void;
}) {
  const [groups, setGroups] = useState<RunGroup[]>([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [recentRuns, setRecentRuns] = useState<RunListItem[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [recentHasMore, setRecentHasMore] = useState(true);
  const [recentLoadingMore, setRecentLoadingMore] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Set<number>>(new Set());
  const [expandedBranches, setExpandedBranches] = useState<Set<string>>(new Set());
  const [repoRunsCache, setRepoRunsCache] = useState<Map<number, Record<string, RunListItem[]>>>(new Map());
  const [loadingRepos, setLoadingRepos] = useState<Set<number>>(new Set());
  const groupsRef = useRef(groups);
  const recentRunsRef = useRef(recentRuns);
  const expandedGroupsRef = useRef(expandedGroups);
  const activeGroupIdRef = useRef<number | null>(null);
  const repoRunsCacheRef = useRef(repoRunsCache);
  const loadingReposRef = useRef(loadingRepos);
  const autoScrolledRunIdRef = useRef<string | null>(null);
  const pollRef = useRef<number | null>(null);

  const searchTerm = (searchParams.get('q') || '').trim().toLowerCase();
  const activeRunId = searchParams.get('run');
  const activeGroupId = useMemo(() => {
    const raw = Number(searchParams.get('group'));
    return Number.isFinite(raw) ? raw : null;
  }, [searchParams]);

  useEffect(() => {
    groupsRef.current = groups;
    recentRunsRef.current = recentRuns;
    expandedGroupsRef.current = expandedGroups;
    activeGroupIdRef.current = activeGroupId;
  }, [activeGroupId, expandedGroups, groups, recentRuns]);

  const fetchJson = useCallback(async <T,>(path: string): Promise<T | null> => {
    try {
      const resp = await fetch(buildApiUrl(path), { cache: 'no-store' });
      if (!resp.ok) return null;
      return (await resp.json()) as T;
    } catch {
      return null;
    }
  }, []);

  const ensureRepoRuns = useCallback(
    async (groupId: number, options?: { force?: boolean }) => {
      const force = options?.force ?? false;
      if ((!force && repoRunsCacheRef.current.has(groupId)) || loadingReposRef.current.has(groupId)) return;
      setLoadingRepos(prev => {
        const next = new Set(prev);
        next.add(groupId);
        loadingReposRef.current = next;
        return next;
      });
      const data = await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${groupId}`);
      setRepoRunsCache(prev => {
        const next = new Map(prev);
        next.set(groupId, data || {});
        repoRunsCacheRef.current = next;
        return next;
      });
      setLoadingRepos(prev => {
        const next = new Set(prev);
        next.delete(groupId);
        loadingReposRef.current = next;
        return next;
      });
    },
    [fetchJson]
  );

  const handleSelectGroup = useCallback(
    (groupId: number | null) => {
      const params = new URLSearchParams(searchParams);
      if (groupId === null) params.delete('group');
      else params.set('group', String(groupId));
      params.delete('run');
      navigateTo(`/pipelineruns/main${params.toString() ? `?${params.toString()}` : ''}`);
      onClose?.();
    },
    [navigateTo, onClose, searchParams]
  );

  const handleOpenRun = useCallback(
    (runId: string, groupId?: number | null) => {
      const params = new URLSearchParams(searchParams);
      if (groupId) params.set('group', String(groupId));
      params.set('run', runId);
      const base = tab === 'recent' ? '/pipelineruns/recent' : tab === 'events' ? '/pipelineruns/events' : '/pipelineruns/main';
      navigateTo(`${base}?${params.toString()}`);
      onClose?.();
    },
    [navigateTo, onClose, searchParams, tab]
  );

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setGroupsLoading(true);
      const data = await fetchJson<RunGroup[]>('/v1/groups');
      if (cancelled) return;
      setGroups(Array.isArray(data) ? data : []);
      setGroupsLoading(false);
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [fetchJson, tab]);

  useEffect(() => {
    if (tab !== 'recent') return;
    let cancelled = false;
    const loadRuns = async () => {
      setRunsLoading(true);
      setRecentHasMore(true);
      setRecentLoadingMore(false);
      const data = await fetchJson<RunListItem[]>(`/v1/runs?offset=0&limit=${SIDEBAR_RECENT_PAGE_SIZE}`);
      if (cancelled) return;
      const list = Array.isArray(data) ? data : [];
      setRecentRuns(list);
      setRecentHasMore(list.length === SIDEBAR_RECENT_PAGE_SIZE);
      setRunsLoading(false);
    };
    void loadRuns();
    return () => {
      cancelled = true;
    };
  }, [fetchJson, tab]);

  const loadMoreRecentRuns = useCallback(async () => {
    if (tab !== 'recent') return;
    if (!recentHasMore || recentLoadingMore || runsLoading) return;
    setRecentLoadingMore(true);
    const data = await fetchJson<RunListItem[]>(`/v1/runs?offset=${recentRuns.length}&limit=${SIDEBAR_RECENT_PAGE_SIZE}`);
    const list = Array.isArray(data) ? data : [];
    setRecentHasMore(list.length === SIDEBAR_RECENT_PAGE_SIZE);
    setRecentRuns(prev => {
      const existing = new Set(prev.map(run => run.run_id));
      const appended = list.filter(run => !existing.has(run.run_id));
      return [...prev, ...appended];
    });
    setRecentLoadingMore(false);
  }, [fetchJson, recentHasMore, recentLoadingMore, recentRuns.length, runsLoading, tab]);

  const refreshRecentRuns = useCallback(async () => {
    const limit = Math.max(SIDEBAR_RECENT_PAGE_SIZE, recentRunsRef.current.length || 0);
    const data = await fetchJson<RunListItem[]>(`/v1/runs?offset=0&limit=${limit}`);
    if (!Array.isArray(data)) return;
    setRecentRuns(data);
    setRecentHasMore(data.length === limit);
  }, [fetchJson]);

  const refreshVisibleRepoRuns = useCallback(async () => {
    const groupsById = new Map(groupsRef.current.map(group => [group.id, group]));
    const targetGroupIds = new Set<number>();

    const activeGroup = activeGroupIdRef.current !== null ? groupsById.get(activeGroupIdRef.current) : null;
    if (activeGroup?.name.includes('/')) {
      targetGroupIds.add(activeGroup.id);
    }

    expandedGroupsRef.current.forEach(groupId => {
      const group = groupsById.get(groupId);
      if (group?.name.includes('/')) {
        targetGroupIds.add(groupId);
      }
    });

    const idsToRefresh = Array.from(targetGroupIds).filter(groupId => !loadingReposRef.current.has(groupId));
    if (!idsToRefresh.length) return;

    const responses = await Promise.all(
      idsToRefresh.map(async groupId => ({
        groupId,
        data: await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${groupId}`),
      }))
    );

    setRepoRunsCache(prev => {
      let next: Map<number, Record<string, RunListItem[]>> | null = null;
      responses.forEach(({ groupId, data }) => {
        if (!data) return;
        if (!next) next = new Map(prev);
        next.set(groupId, data);
      });
      if (!next) return prev;
      repoRunsCacheRef.current = next;
      return next;
    });
  }, [fetchJson]);

  useEffect(() => {
    if (tab !== 'recent') return;
    const nav = document.getElementById('sidebar-details-nav');
    const container = nav?.parentElement;
    if (!container) return;
    const handleScroll = () => {
      const remaining = container.scrollHeight - container.scrollTop - container.clientHeight;
      if (remaining <= SIDEBAR_SCROLL_BUFFER) {
        void loadMoreRecentRuns();
      }
    };
    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => container.removeEventListener('scroll', handleScroll);
  }, [loadMoreRecentRuns, tab]);

  useEffect(() => {
    if (!activeGroupId || !groups.length) return;
    const path = buildGroupPath(activeGroupId, groups);
    if (!path.length) return;
    const handle = window.setTimeout(() => {
      setExpandedGroups(prev => {
        const next = new Set(prev);
        path.forEach(g => next.add(g.id));
        return next;
      });
    }, 0);
    return () => window.clearTimeout(handle);
  }, [activeGroupId, groups]);

  useEffect(() => {
    if (tab !== 'main') return;
    if (!activeGroupId) return;
    const group = groups.find(g => g.id === activeGroupId);
    if (!group || !group.name.includes('/')) return;
    void ensureRepoRuns(group.id, { force: true });
  }, [activeGroupId, ensureRepoRuns, groups, tab]);

  useEffect(() => {
    if (!activeRunId) return;
    const expandForRun = async () => {
      const detail = await fetchJson<RunDetail>(`/v1/runs/${encodeURIComponent(activeRunId)}`);
      const info = detail?.run_info;
      if (!info) return;
      const repoName = info.git_repo_owner && info.git_repo_name ? `${info.git_repo_owner}/${info.git_repo_name}` : '';
      const repoGroup = repoName ? groups.find(g => g.name === repoName) : null;
      if (!repoGroup) return;
      const path = buildGroupPath(repoGroup.id, groups);
      setExpandedGroups(prev => {
        const next = new Set(prev);
        path.forEach(g => next.add(g.id));
        return next;
      });
      await ensureRepoRuns(repoGroup.id, { force: true });
      const branchName = formatBranch(info.git_ref);
      if (branchName) {
        const key = `${repoGroup.id}:${branchName}`;
        setExpandedBranches(prev => {
          const next = new Set(prev);
          next.add(key);
          return next;
        });
      }
    };
    void expandForRun();
  }, [activeRunId, ensureRepoRuns, fetchJson, groups]);

  useEffect(() => {
    if (pollRef.current) {
      window.clearTimeout(pollRef.current);
      pollRef.current = null;
    }
    let cancelled = false;
    const tick = async () => {
      if (cancelled) return;
      if (tab === 'recent') {
        await refreshRecentRuns();
      } else {
        await refreshVisibleRepoRuns();
      }
      if (cancelled) return;
      const interval = document.hidden ? 12000 : 6000;
      pollRef.current = window.setTimeout(tick, interval);
    };
    const interval = document.hidden ? 12000 : 6000;
    pollRef.current = window.setTimeout(tick, interval);
    return () => {
      cancelled = true;
      if (pollRef.current) {
        window.clearTimeout(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [refreshRecentRuns, refreshVisibleRepoRuns, tab]);

  const toggleGroup = (group: RunGroup) => {
    const isRepo = group.name.includes('/');
    setExpandedGroups(prev => {
      const next = new Set(prev);
      const willExpand = !next.has(group.id);
      if (willExpand) {
        next.add(group.id);
        if (isRepo) void ensureRepoRuns(group.id, { force: true });
      } else {
        next.delete(group.id);
      }
      return next;
    });
  };

  const toggleBranch = (groupId: number, branch: string) => {
    const key = `${groupId}:${branch}`;
    setExpandedBranches(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const rootGroups = useMemo(() => groups.filter(g => (g.parent_id ?? null) === null), [groups]);
  const filteredRecent = useMemo(() => {
    if (tab !== 'recent') return [];
    if (!searchTerm) return recentRuns;
    return recentRuns.filter(run => runMatchesSearch(run, searchTerm));
  }, [recentRuns, searchTerm, tab]);

  useEffect(() => {
    if (!activeRunId) {
      autoScrolledRunIdRef.current = null;
      return;
    }
    if (autoScrolledRunIdRef.current === activeRunId) return;
    const selector = (() => {
      if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') return `[data-run-id="${CSS.escape(activeRunId)}"]`;
      return `[data-run-id="${activeRunId.replace(/"/g, '\\"')}"]`;
    })();
    const scrollToActive = () => {
      const el = document.querySelector(selector);
      if (el && 'scrollIntoView' in el) {
        (el as HTMLElement).scrollIntoView({ behavior: 'smooth', block: 'center' });
        autoScrolledRunIdRef.current = activeRunId;
      }
    };
    const id = window.setTimeout(scrollToActive, 100);
    return () => window.clearTimeout(id);
  }, [activeRunId, repoRunsCache, recentRuns]);

  const renderRunRow = (run: RunListItem, groupId?: number | null) => (
    <RunSidebarRow
      key={run.run_id}
      run={run}
      active={activeRunId === run.run_id}
      onOpen={() => handleOpenRun(run.run_id, groupId)}
    />
  );

  const renderBranchRuns = (groupId: number, branch: string, runs: RunListItem[]) => {
    const key = `${groupId}:${branch}`;
    const expanded = expandedBranches.has(key);
    const filteredRuns = searchTerm ? runs.filter(run => runMatchesSearch(run, searchTerm)) : runs;
    if (searchTerm && filteredRuns.length === 0) return null;
    const branchLabel = formatBranch(branch);
    return (
      <div key={key} className="border border-[var(--border-primary)] rounded-lg overflow-hidden bg-[var(--bg-primary)]">
        <button
          type="button"
          onClick={() => toggleBranch(groupId, branch)}
          className="flex items-center justify-between w-full px-3 py-2 text-left hover:bg-[var(--bg-tertiary)] transition-colors"
        >
          <div className="flex items-center gap-2 min-w-0">
            <svg
              className={`h-4 w-4 text-[var(--text-secondary)] transition-transform ${expanded ? 'rotate-90' : ''}`}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path d="M9 5l7 7-7 7" />
            </svg>
            <span className="text-xs font-semibold text-[var(--text-primary)] truncate">{branchLabel || branch}</span>
            <span className="text-[10px] text-[var(--text-secondary)] whitespace-nowrap">({runs.length})</span>
          </div>
          <StatusDot status={summarizeStatus(runs)} />
        </button>
        {expanded && (
          <div className="border-t border-[var(--border-primary)] bg-[var(--bg-secondary)]/40">
            {filteredRuns.length === 0 ? (
              <div className="px-3 py-2 text-[var(--text-secondary)] text-xs">No matching runs.</div>
            ) : (
              <div className="divide-y divide-[var(--border-primary)]">
                {filteredRuns.map(run => (
                  <div key={run.run_id} className="px-3 py-2">
                    {renderRunRow(run, groupId)}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  const renderGroupNode = (group: RunGroup) => {
    const isRepo = group.name.includes('/');
    const expanded = expandedGroups.has(group.id);
    const children = groups.filter(child => (child.parent_id ?? null) === group.id);
    const label = isRepo ? group.name.split('/')[1] || group.name : group.name;
    const repoRuns = repoRunsCache.get(group.id);
    const isLoadingRepo = loadingRepos.has(group.id);

    return (
      <div key={group.id} className="space-y-2">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => toggleGroup(group)}
            aria-label={expanded ? 'Collapse sidebar item' : 'Expand sidebar item'}
            className="inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
          >
            <svg
              className={`h-3.5 w-3.5 transition-transform ${expanded ? 'rotate-90' : ''}`}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M9 5l7 7-7 7" />
            </svg>
          </button>
          <button
            type="button"
            className="flex items-center gap-2 flex-1 min-w-0 text-left"
            onClick={() => handleSelectGroup(group.id)}
            aria-expanded={expanded}
            title={isRepo ? 'Open in main view' : 'Open group in main view'}
          >
            <span className={`h-4 w-4 flex items-center justify-center ${isRepo ? 'text-[var(--text-accent)]' : 'text-[var(--text-secondary)]'}`}>
              {isRepo ? (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4">
                  <circle cx="8" cy="7" r="2" />
                  <circle cx="8" cy="17" r="2" />
                  <circle cx="16" cy="7" r="2" />
                  <path d="M10 7h4" />
                  <path d="M8 9v6a4 4 0 004 4h4" />
                </svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4">
                  <path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z" />
                </svg>
              )}
            </span>
            <span className="text-sm text-[var(--text-primary)] truncate" title={group.name}>
              {label}
            </span>
          </button>
        </div>
        {expanded && (
          <div className="pl-4 space-y-2">
            {isRepo ? (
              <>
                {isLoadingRepo && <div className="text-[var(--text-secondary)] text-xs">Loading runs…</div>}
                {!isLoadingRepo && (!repoRuns || Object.keys(repoRuns).length === 0) && (
                  <div className="text-[var(--text-secondary)] text-xs">No runs for this repository.</div>
                )}
                {repoRuns &&
                  Object.entries(repoRuns)
                    .sort(([a], [b]) => a.localeCompare(b))
                    .map(([branch, runs]) => renderBranchRuns(group.id, branch, runs))}
              </>
            ) : (
              children
                .sort((a, b) => a.name.localeCompare(b.name))
                .map(child => renderGroupNode(child))
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="space-y-3">
      {tab === 'recent' ? (
        <>
          <div className="flex items-center justify-between gap-2">
            <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">Recent runs</p>
            <span className="text-[10px] text-[var(--text-secondary)]">{filteredRecent.length} items</span>
          </div>
          {runsLoading && <div className="text-xs text-[var(--text-secondary)]">Loading runs…</div>}
          {!runsLoading && filteredRecent.length === 0 && <div className="text-xs text-[var(--text-secondary)]">No runs to show.</div>}
          {!runsLoading && filteredRecent.map(run => renderRunRow(run))}
          {recentLoadingMore && <div className="text-xs text-[var(--text-secondary)]">Loading more runs…</div>}
          {!runsLoading && !recentLoadingMore && recentHasMore && (
            <button
              type="button"
              className="text-xs text-[var(--text-link)] hover:underline"
              onClick={() => void loadMoreRecentRuns()}
            >
              Load more runs
            </button>
          )}
        </>
      ) : (
        <>
          <div className="flex items-center justify-between gap-2">
            <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">Main</p>
            <button
              type="button"
              className="text-xs text-[var(--text-link)] hover:underline"
              onClick={() => handleSelectGroup(null)}
          >
            Root
          </button>
        </div>
          {groupsLoading && <div className="text-xs text-[var(--text-secondary)]">Loading groups…</div>}
          {!groupsLoading && rootGroups.length === 0 && <div className="text-xs text-[var(--text-secondary)]">No groups defined yet.</div>}
          {!groupsLoading && rootGroups.map(group => renderGroupNode(group))}
        </>
      )}
    </div>
  );
}

function RunSidebarBadges({ run }: { run: RunListItem }) {
  const badges: ReactNode[] = [];
  if (run.pipeline_source === 'database override') {
    badges.push(
      <span key="override" className="text-[10px] font-semibold text-[var(--text-link)] inline-flex items-center gap-1 whitespace-nowrap">
        <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M13 16h-1v-4h-1m1-4h.01" />
          <path d="M12 2a10 10 0 100 20 10 10 0 000-20z" />
        </svg>
        Override
      </span>
    );
  }
  if (run.parent_run_id) {
    badges.push(
      <span key="included" className="text-[10px] font-semibold text-[var(--text-link)] whitespace-nowrap">
        Included
      </span>
    );
  }
  if (!badges.length) return null;
  return <div className="flex flex-col items-end gap-1 text-right flex-shrink-0">{badges}</div>;
}

function RunSidebarRow({ run, active, onOpen }: { run: RunListItem; active: boolean; onOpen: () => void }) {
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const repoLabel = formatRepoLabel(run);
  const trigger = formatTriggerLabel(run.trigger_event_id);
  const shortCommit = (run.git_commit_sha || 'N/A').slice(0, 8);
  const shortRunId = (run.run_id || 'N/A').slice(0, 8);
  return (
    <button
      type="button"
      onClick={onOpen}
      data-trigger-id={run.trigger_event_id || ''}
      data-run-id={run.run_id}
      className={`w-full text-left rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] hover:border-[var(--border-accent)] transition shadow-sm px-3 py-2 ${
        active ? 'run-link-highlight' : ''
      }`}
    >
      <div className="flex items-start gap-2">
        <SidebarStatusIcon status={run.status} complete={run.is_complete} />
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="text-sm text-[var(--text-primary)] truncate">{run.pipeline_name || 'Pipeline Run'}</p>
              <p className="text-[11px] text-[var(--text-secondary)] font-mono flex items-center gap-1 truncate">
                <RunIdIcon className="h-3.5 w-3.5 flex-shrink-0" />
                <span>{shortRunId}</span>
              </p>
            </div>
            <RunSidebarBadges run={run} />
          </div>
        </div>
        <span className="text-[10px] text-[var(--text-secondary)] whitespace-nowrap flex-shrink-0">
          {timeAgoShort(run.started_at || run.finished_at)}
        </span>
      </div>
      <div className="mt-2 text-[11px] text-[var(--text-secondary)] font-mono space-y-1">
        <div className="flex items-center gap-2 truncate" title="Repository">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="8" cy="7" r="2" />
            <circle cx="8" cy="17" r="2" />
            <circle cx="16" cy="7" r="2" />
            <path d="M10 7h4" />
            <path d="M8 9v6a4 4 0 004 4h4" />
          </svg>
          <span className="truncate">{repoLabel}</span>
        </div>
        <div className="flex items-center gap-2 truncate" title="Branch">
          <BranchIcon className="h-3.5 w-3.5" />
          <span className="truncate">{branchLabel || 'N/A'}</span>
        </div>
        <div className="flex items-center gap-2 truncate" title="Commit">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3" />
            <path d="M3 12h6" />
            <path d="M15 12h6" />
          </svg>
          <span className="truncate">{shortCommit}</span>
        </div>
        <div className="flex items-center gap-2 truncate" title="Trigger ID">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M7 7a1 1 0 011-1h3.586a1 1 0 01.707.293l6.414 6.414a1 1 0 010 1.414l-4.586 4.586a1 1 0 01-1.414 0L7.293 13.707A1 1 0 017 13V9a1 1 0 011-1z" />
          </svg>
          <span className="truncate">{trigger.display.slice(0, 8)}</span>
        </div>
      </div>
    </button>
  );
}

function SidebarStatusIcon({ status, complete }: { status: string; complete?: boolean }) {
  const normalized = normalizeRunStatus(status, complete);
  const tone = getSidebarStatusTone(normalized);
  const isFailure = normalized === 'failure' || normalized === 'failure (ignored)';
  const isCancelled = normalized === 'cancelled';
  const isRunning = normalized === 'running';
  const isSkipped = normalized === 'skipped';
  const isPending = normalized === 'pending';
  return (
    <span
      className={`inline-flex h-7 w-7 items-center justify-center rounded-full border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${tone}`}
      aria-label={normalized}
    >
      {isRunning ? (
        <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12a9 9 0 11-6.219-8.56" />
        </svg>
      ) : isFailure || isCancelled ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M18 6L6 18" />
          <path d="M6 6l12 12" />
        </svg>
      ) : isSkipped ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M6 12h12" />
          <path d="M6 16h12" />
        </svg>
      ) : isPending ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 8v4l3 3" />
          <path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      ) : (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 13l4 4L19 7" />
        </svg>
      )}
    </span>
  );
}

function getSidebarStatusTone(status: string) {
  const normalized = normalizeRunStatus(status, true);
  if (normalized === 'success') return 'text-green-400';
  if (normalized === 'failure' || normalized === 'failure (ignored)') return 'text-red-400';
  if (normalized === 'running') return 'text-blue-400';
  return 'text-slate-300';
}

function StatusDot({ status, complete }: { status: string; complete?: boolean }) {
  return <span className={`inline-block h-2.5 w-2.5 rounded-full ${getStatusDotClass(status, complete)}`} aria-hidden="true" />;
}

function getStatusDotClass(status: string | undefined, complete?: boolean) {
  const normalized = normalizeRunStatus(status, complete);
  if (normalized === 'success') return 'bg-emerald-400';
  if (normalized === 'failure') return 'bg-red-500';
  if (normalized === 'failure (ignored)') return 'bg-amber-500';
  if (normalized === 'running') return 'bg-blue-400';
  if (normalized === 'cancelled') return 'bg-orange-400';
  if (normalized === 'skipped') return 'bg-slate-400';
  return 'bg-gray-500';
}

function normalizeRunStatus(status: string | undefined, complete?: boolean): string {
  const raw = (status || '').toLowerCase();
  if (!complete && raw !== 'success' && raw !== 'failure' && raw !== 'cancelled' && raw !== 'skipped') return 'running';
  if (STATUS_PRIORITY.includes(raw)) return raw;
  return 'pending';
}

function runMatchesSearch(run: RunListItem, term: string): boolean {
  if (!term) return true;
  const haystack = [
    run.pipeline_name,
    run.pipeline_path,
    run.git_repo_name,
    run.git_repo_owner,
    run.git_ref,
    run.git_target_ref,
    run.git_commit_sha,
    run.git_pusher_name,
    run.status,
    run.trigger_event_id,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  return haystack.includes(term);
}

function formatBranch(ref?: string) {
  if (!ref) return '';
  return ref.replace(/^refs\/heads\//, '');
}

function formatBranchDisplay(source?: string, target?: string) {
  const sourceBranch = formatBranch(source);
  const targetBranch = formatBranch(target);
  if (targetBranch && targetBranch !== '—') {
    return `${sourceBranch} -> ${targetBranch}`;
  }
  return sourceBranch;
}

function formatRepoLabel(run: RunListItem) {
  const owner = run.git_repo_owner || '';
  const name = run.git_repo_name || '';
  if (owner && name) return `${owner}/${name}`;
  return name || owner || 'Repository';
}

function formatTriggerLabel(id?: string) {
  if (!id) return { display: 'N/A', full: 'N/A' };
  const full = String(id);
  return { display: full, full };
}

function timeAgoShort(dateInput?: string) {
  if (!dateInput) return '—';
  const date = new Date(dateInput);
  if (Number.isNaN(date.getTime())) return '—';
  const diff = Date.now() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function buildGroupPath(groupId: number | null, groups: RunGroup[]): RunGroup[] {
  if (!groupId) return [];
  const map = new Map<number, RunGroup>();
  groups.forEach(group => map.set(group.id, group));
  const path: RunGroup[] = [];
  let current = map.get(groupId) || null;
  const visited = new Set<number>();
  while (current && !visited.has(current.id)) {
    visited.add(current.id);
    path.unshift(current);
    const parentId = current.parent_id ?? null;
    current = parentId ? map.get(parentId) || null : null;
  }
  return path;
}

function summarizeStatus(runs: RunListItem[]): string {
  if (!runs.length) return 'pending';
  const ranked = runs
    .map(run => normalizeRunStatus(run.status, run.is_complete))
    .sort((a, b) => STATUS_PRIORITY.indexOf(a) - STATUS_PRIORITY.indexOf(b));
  return ranked[0] || 'pending';
}

function Header({
  title,
  onOpenSidebar,
  theme,
  onToggleTheme,
  onLogout,
  currentUser,
  userLoading,
  onOpenProfile,
}: {
  title: string;
  onOpenSidebar: () => void;
  theme: Theme;
  onToggleTheme: () => void;
  onLogout?: () => void;
  currentUser?: CurrentUser | null;
  userLoading?: boolean;
  onOpenProfile?: () => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const initials = (() => {
    const base = (currentUser?.sub || currentUser?.email || 'U').trim();
    const cleaned = base.replace(/[^A-Za-z0-9]/g, '');
    return (cleaned[0] || base[0] || 'U').toUpperCase();
  })();
  const displayName = (() => {
    const preferred = (currentUser?.sub || '').trim();
    if (preferred && !preferred.includes('@')) return preferred;
    return initials;
  })();

  useEffect(() => {
    if (!menuOpen) return;
    const handleClick = (event: MouseEvent) => {
      if (!menuRef.current) return;
      if (!menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKey);
    };
  }, [menuOpen]);

  const closeMenu = () => setMenuOpen(false);

  return (
    <header
      className="relative flex items-center justify-between px-6 py-4 themed-bg-blur backdrop-blur-sm shadow-sm z-40 border-b border-[var(--border-primary)] flex-shrink-0"
      style={{ paddingTop: '11px' }}
    >
      <button
        id="open-sidebar-btn"
        className="sm:hidden text-[var(--text-secondary)] hover:text-[var(--text-primary)] mr-4"
        onClick={onOpenSidebar}
        aria-label="Open sidebar"
      >
        <IconMenu />
      </button>
      <div id="main-header" className="flex-1 text-xl font-semibold min-w-0 truncate">{title}</div>
      <div className="flex items-center gap-3">
        <AppHelp />
        <div className="relative" ref={menuRef}>
          <button
            type="button"
            onClick={() => setMenuOpen(open => !open)}
            className={`flex items-center gap-3 px-3 h-11 rounded-full border bg-[var(--bg-secondary)] text-[var(--text-primary)] shadow-sm transition-all ${
              menuOpen ? 'border-[var(--border-accent)]' : 'border-[var(--border-primary)] hover:border-[var(--border-accent)]'
            } focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]`}
            aria-haspopup="true"
            aria-expanded={menuOpen}
          >
            <span className="text-sm font-semibold text-[var(--text-primary)] max-w-[160px] truncate">{displayName}</span>
          </button>
          {menuOpen && (
            <div className="absolute right-0 mt-2 w-72 rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-2xl overflow-hidden z-[500]">
              <div className="p-4 border-b border-[var(--border-primary)] bg-[var(--bg-tertiary)]/70 backdrop-blur-sm">
                <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] mb-1">Signed in as</p>
                <p className="text-sm font-semibold text-[var(--text-primary)] truncate">{userLoading ? 'Loading…' : displayName}</p>
                <p className="text-xs text-[var(--text-secondary)] mt-2">Global access model</p>
              </div>
              <div className="p-2 space-y-1">
                <button
                  className="w-full text-left px-3 py-2 rounded-lg hover:bg-[var(--bg-tertiary)] text-[var(--text-primary)] text-sm"
                  onClick={() => {
                    closeMenu();
                    onOpenProfile?.();
                  }}
                >
                  View profile
                </button>
                <button
                  className="w-full text-left px-3 py-2 rounded-lg hover:bg-[var(--bg-tertiary)] text-[var(--text-primary)] text-sm"
                  onClick={() => {
                    closeMenu();
                    onToggleTheme();
                  }}
                >
                  {theme === 'dark' ? 'Use light mode' : 'Use dark mode'}
                </button>
                {onLogout && (
                  <button
                    className="w-full text-left px-3 py-2 rounded-lg hover:bg-[var(--bg-tertiary)] text-[var(--text-primary)] text-sm"
                    onClick={() => {
                      closeMenu();
                      onLogout();
                    }}
                  >
                    Logout
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}

function IconX() {
  return (
    <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
    </svg>
  );
}

function IconMenu() {
  return (
    <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16m-7 6h7" />
    </svg>
  );
}

function IconPlay() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M14.752 11.168l-4.197-2.42A1 1 0 009 9.58v4.84a1 1 0 001.555.832l4.197-2.42a1 1 0 000-1.664z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function IconFlow() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
    </svg>
  );
}

function IconMonitoring() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M4 19V5m0 14h16" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M7 15l3-4 3 2 4-7" />
      <circle cx="7" cy="15" r="1.25" strokeWidth="1.8" />
      <circle cx="10" cy="11" r="1.25" strokeWidth="1.8" />
      <circle cx="13" cy="13" r="1.25" strokeWidth="1.8" />
      <circle cx="17" cy="6" r="1.25" strokeWidth="1.8" />
    </svg>
  );
}

function RunIdIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M4 7h4v10H4z" />
      <path d="M12 7h8" />
      <path d="M12 12h8" />
      <path d="M12 17h8" />
    </svg>
  );
}

function BranchIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="6" y1="3" x2="6" y2="15" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 01-9 9" />
    </svg>
  );
}

function IconBell() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
    </svg>
  );
}

function IconScope() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <circle cx="12" cy="12" r="8" strokeWidth="1.8" />
      <circle cx="12" cy="12" r="2" strokeWidth="1.8" />
      <path strokeWidth="1.8" strokeLinecap="round" d="M12 3v3m0 12v3m9-9h-3M6 12H3" />
      <path strokeWidth="1.8" strokeLinecap="round" d="M16.95 7.05l-2.12 2.12m-5.66 5.66-2.12 2.12" />
      <path strokeWidth="1.8" strokeLinecap="round" d="M7.05 7.05l2.12 2.12m5.66 5.66 2.12 2.12" />
    </svg>
  );
}

function IconFlask() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M10 2h4m-2 0v8m0 0H8m4 0h4m-6 4h4m-6 4h8M6 10h12" />
    </svg>
  );
}

function IconCog() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0a1.724 1.724 0 002.573 1.02c.842-.488 1.91.27 1.662 1.2a1.724 1.724 0 001.091 2.062c.9.3.9 1.603 0 1.902a1.724 1.724 0 00-1.09 2.062c.247.93-.82 1.688-1.663 1.2a1.724 1.724 0 00-2.572 1.02c-.3.921-1.603.921-1.902 0a1.724 1.724 0 00-2.573-1.02c-.842.488-1.91-.27-1.662-1.2a1.724 1.724 0 00-1.091-2.062c-.9-.3-.9-1.603 0-1.902a1.724 1.724 0 001.09-2.062c-.247-.93.82-1.688 1.663-1.2a1.724 1.724 0 002.572-1.02z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
    </svg>
  );
}

function IconDispatch() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M3 12h7l-2 3m2-3-2-3m11 0h-7l2 3m-2 0 2 3" />
      <circle cx="12" cy="12" r="9" strokeWidth="1.6" />
    </svg>
  );
}

function IconShield() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M12 3l8 4v5c0 4.5-3.2 8.3-8 9-4.8-.7-8-4.5-8-9V7z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M9 12l2 2 4-4" />
    </svg>
  );
}

function IconSteps() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 2l8 4.5v11L12 22 4 17.5v-11L12 2z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 22v-7.5" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M20 6.5l-8 4.5-8-4.5" />
    </svg>
  );
}

function IconKnowledge() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M4 5.5A2.5 2.5 0 016.5 3H20v16H6.5A2.5 2.5 0 014 16.5z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.9" d="M8 7h8M8 11h8M8 15h5" />
    </svg>
  );
}

export default App;
