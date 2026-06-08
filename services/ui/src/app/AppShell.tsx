import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { BranchIcon, IconMenu, IconX, RunIdIcon } from './icons';
import {
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  SIDEBAR_SCROLL_BUFFER,
} from './constants';
import { baseNavItems, baseSystemSubNav, titleMap } from './navigation';
import {
  formatBranch,
  formatBranchDisplay,
  formatRepoLabel,
  formatTriggerLabel,
  getSidebarStatusTone,
  getStatusDotClass,
  isRunAppGroup,
  normalizeRunStatus,
  runGroupDisplayName,
  runGroupRepositoryURL,
  runMatchesSearch,
  summarizeStatus,
  timeAgoShort,
} from './runSidebarUtils';
import { normalizeScopeLabel } from './resourceTrees';
import type {
  CurrentUser,
  KnowledgeContextTreeNode,
  NavItem,
  PipelineTreeNode,
  RunGroup,
  RunListItem,
  RunTabKey,
  ScopeTreeNode,
  StepTreeNode,
  Theme,
  TriggerTreeNode,
} from './types';
import { useSidebarState } from './useSidebarState';
import { useResourceTrees } from './useResourceTrees';
import { BaseSidebarNavigation } from './BaseSidebarNavigation';
import { useInitialSetupRedirect } from './useInitialSetupRedirect';
import { usePipelineRunsSidebar } from './usePipelineRunsSidebar';
import { getAppAccess } from '../auth/capabilities';
import { PermissionGuard } from '../auth/permissionGuards';
import { useAuth } from '../auth/AuthContext';
import { buildLoginRedirectState, resolvePostLoginPath } from '../auth/authRedirect';
import AppHelp from '../components/AppHelp';

const PipelineRunsPage = lazy(() => import('../pages/PipelineRuns'));
const PipelinesPage = lazy(() => import('../pages/Pipelines'));
const SchedulesPage = lazy(() => import('../pages/Schedules'));
const TriggersPage = lazy(() => import('../pages/Triggers'));
const ExternalTriggersPage = lazy(() => import('../pages/ExternalTriggers'));
const ScopesPage = lazy(() => import('../pages/Scopes'));
const LabPage = lazy(() => import('../pages/Lab'));
const StepsPage = lazy(() => import('../pages/Steps'));
const KnowledgeContextPage = lazy(() => import('../pages/KnowledgeContext'));
const MonitoringPage = lazy(() => import('../pages/Monitoring'));
const SystemPage = lazy(() => import('../pages/System'));
const LoginPage = lazy(() => import('../pages/Login'));
const ProfilePage = lazy(() => import('../pages/Profile'));

const getInitialTheme = (): Theme => {
  if (typeof window === 'undefined') return 'light';
  const stored = localStorage.getItem('theme');
  if (stored === 'dark' || stored === 'light') return stored;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
};

function PageLoading() {
  return <div className="p-6 text-sm text-[var(--text-secondary)]">Loading...</div>;
}

function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const [theme, setTheme] = useState<Theme>(getInitialTheme);
  const {
    authSession,
    currentUser,
    currentUserLoading,
    isAuthenticated,
    refreshAuthSession,
    clearAuthSession,
    updateCurrentUser,
    markPasswordChanged,
  } = useAuth();
  const access = useMemo(() => getAppAccess(currentUser, authSession), [authSession, currentUser]);
  const {
    draftScope,
    canWritePipelines,
    canDeletePipelines,
    canViewSchedules,
    canWriteSchedules,
    canDeleteSchedules,
    canWriteSteps,
    canDeleteSteps,
    canViewTriggers,
    canDeleteTriggers,
    canViewExternalTriggers,
    canWriteExternalTriggers,
    canDeleteExternalTriggers,
    canViewScopes,
    canDeleteScopes,
    canViewKnowledge,
    canWriteKnowledge,
    canDeleteKnowledge,
    canViewSystemRuntimeConfig,
    canViewSystemConfig,
    canViewSystemSetup,
    canViewSystemLLMProfiles,
    canViewSystemMCP,
    canViewSystemDispatcher,
    canViewSystemAccess,
    canViewAnySystem,
    preferredSystemPath,
    isInitialAdminUser,
    systemPermissions,
  } = access;
  const sidebar = useSidebarState(location.pathname);

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
    const isAuthenticated = Boolean(authSession.accessToken);
    if (!isAuthenticated && location.pathname !== '/login') {
      navigate('/login', {
        replace: true,
        state: buildLoginRedirectState(location.pathname, location.search),
      });
    }
    if (isAuthenticated && authSession.mustChangePassword && location.pathname !== '/profile') {
      navigate('/profile', { replace: true });
    }
    if (isAuthenticated && location.pathname === '/login') {
      navigate(
        authSession.mustChangePassword ? '/profile' : resolvePostLoginPath(location.state),
        { replace: true }
      );
    }
  }, [authSession.accessToken, authSession.mustChangePassword, location.pathname, location.search, location.state, navigate]);

  const handleLoginSuccess = useCallback(() => {
    refreshAuthSession();
  }, [refreshAuthSession]);

  const handleOpenProfile = useCallback(() => {
    navigate('/profile');
  }, [navigate]);

  const handleLogout = useCallback(() => {
    clearAuthSession();
    navigate('/login', { replace: true });
  }, [clearAuthSession, navigate]);

  const handleUserUpdated = useCallback((updates: Partial<CurrentUser>) => {
    updateCurrentUser(updates);
  }, [updateCurrentUser]);
  const navItems = useMemo(() => {
    return baseNavItems
      .map(item => (item.path.startsWith('/system') ? { ...item, path: preferredSystemPath } : item))
      .filter(item => {
        if (item.path.startsWith('/system')) return canViewAnySystem;
        if (item.path === '/schedules') return canViewSchedules;
        if (item.path === '/triggers') return canViewTriggers;
        if (item.path === '/external-triggers') return canViewExternalTriggers;
        if (item.path === '/scopes') return canViewScopes;
        if (item.path === '/knowledge-context') return canViewKnowledge;
        return true;
      });
  }, [canViewAnySystem, canViewExternalTriggers, canViewKnowledge, canViewSchedules, canViewScopes, canViewTriggers, preferredSystemPath]);
  const systemSubNav = useMemo(
    () =>
      baseSystemSubNav.filter(item => {
        if (item.path === '/system/config') return canViewSystemConfig;
        if (item.path === '/system/setup') return canViewSystemSetup;
        if (item.path === '/system/llm-profiles') return canViewSystemLLMProfiles;
        if (item.path === '/system/mcp') return canViewSystemMCP;
        if (item.path === '/system/data-management') return canViewSystemRuntimeConfig;
        if (item.path === '/system/dispatcher') return canViewSystemDispatcher;
        if (item.path === '/system/access') return canViewSystemAccess;
        return false;
      }),
    [canViewSystemAccess, canViewSystemConfig, canViewSystemDispatcher, canViewSystemLLMProfiles, canViewSystemMCP, canViewSystemRuntimeConfig, canViewSystemSetup]
  );

  useInitialSetupRedirect({
    accessToken: authSession.accessToken || '',
    authSubject: authSession.sub,
    canViewSystemSetup,
    currentSubject: currentUser?.sub,
    currentUserLoading,
    isInitialAdminUser,
    isAuthenticated,
    mustChangePassword: Boolean(authSession.mustChangePassword),
    pathname: location.pathname,
    navigate,
  });

  const resourceTrees = useResourceTrees({
    canViewKnowledge,
    canWritePipelines,
    canWriteSteps,
    draftScope,
    isAuthenticated,
    pathname: location.pathname,
  });

  const title = useMemo(() => {
    const key = location.pathname.split('/').filter(Boolean)[0] || 'pipelineruns';
    return titleMap[key] || 'Dashboard';
  }, [location.pathname]);
  const isLoginRoute = location.pathname === '/login';

  const renderAccessControlledPage = useCallback(
    (allowed: boolean, element: ReactNode) => {
      return (
        <PermissionGuard allowed={allowed} loading={currentUserLoading}>
          {element}
        </PermissionGuard>
      );
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
              open={sidebar.open}
              onClose={sidebar.close}
              width={sidebar.width}
              pipelineTree={resourceTrees.pipelineTree}
              pipelineTreeOpen={resourceTrees.pipelineTreeOpen}
              onTogglePipelineNode={resourceTrees.onTogglePipelineNode}
              triggerTree={resourceTrees.triggerTree}
              triggerTreeOpen={resourceTrees.triggerTreeOpen}
              onToggleTriggerNode={resourceTrees.onToggleTriggerNode}
              stepTree={resourceTrees.stepTree}
              stepTreeOpen={resourceTrees.stepTreeOpen}
              onToggleStepNode={resourceTrees.onToggleStepNode}
              scopeTree={resourceTrees.scopeTree}
              scopeTreeOpen={resourceTrees.scopeTreeOpen}
              onToggleScopeNode={resourceTrees.onToggleScopeNode}
              knowledgeContextTree={resourceTrees.knowledgeContextTree}
              knowledgeContextTreeOpen={resourceTrees.knowledgeContextTreeOpen}
              onToggleKnowledgeContextNode={resourceTrees.onToggleKnowledgeContextNode}
              splitIdentifier={resourceTrees.splitIdentifier}
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
              className={`hidden sm:block w-1.5 cursor-col-resize flex-shrink-0 transition-colors duration-200 ${sidebar.isResizing ? 'bg-[var(--border-accent)]' : 'bg-[var(--bg-tertiary)] hover:bg-[var(--border-accent)]'}`}
              onMouseDown={sidebar.startResize}
              onTouchStart={sidebar.startResize}
              aria-label="Resize sidebar"
            ></div>
            <main className="flex-1 flex flex-col overflow-hidden">
              <Header
                title={title}
                theme={theme}
                onToggleTheme={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                onOpenSidebar={sidebar.openSidebar}
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
                      path="/schedules/*"
                      element={renderAccessControlledPage(
                        canViewSchedules,
                        <SchedulesPage canWriteSchedules={canWriteSchedules} canDeleteSchedules={canDeleteSchedules} />
                      )}
                    />
                    <Route
                      path="/triggers/*"
                      element={renderAccessControlledPage(
                        canViewTriggers,
                        <TriggersPage canDeleteTriggers={canDeleteTriggers} />
                      )}
                    />
                    <Route
                      path="/external-triggers/*"
                      element={renderAccessControlledPage(
                        canViewExternalTriggers,
                        <ExternalTriggersPage
                          canWriteExternalTriggers={canWriteExternalTriggers}
                          canDeleteExternalTriggers={canDeleteExternalTriggers}
                        />
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
                        <SystemPage permissions={systemPermissions} />
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
                          onPasswordChanged={markPasswordChanged}
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
        <BaseSidebarNavigation navItems={navItems} systemSubNav={systemSubNav} locationPathname={locationPathname} />
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
  const autoScrolledRunIdRef = useRef<string | null>(null);
  const searchTerm = (searchParams.get('q') || '').trim().toLowerCase();
  const activeRunId = searchParams.get('run');
  const activeGroupId = useMemo(() => {
    const raw = Number(searchParams.get('group'));
    return Number.isFinite(raw) ? raw : null;
  }, [searchParams]);
  const {
    groups,
    groupsLoading,
    recentRuns,
    runsLoading,
    recentHasMore,
    recentLoadingMore,
    expandedGroups,
    expandedBranches,
    repoRunsCache,
    loadingRepos,
    loadMoreRecentRuns,
    toggleGroup,
    toggleBranch,
  } = usePipelineRunsSidebar({ activeGroupId, activeRunId, tab });

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
    const isRepo = isRunAppGroup(group);
    const expanded = expandedGroups.has(group.id);
    const children = groups.filter(child => (child.parent_id ?? null) === group.id);
    const label = runGroupDisplayName(group);
    const repoURL = runGroupRepositoryURL(group);
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
          {isRepo && repoURL && (
            <a
              href={repoURL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
              title={`Open ${label} repository`}
              aria-label={`Open ${label} repository`}
            >
              <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M7 17L17 7" />
                <path d="M8 7h9v9" />
              </svg>
            </a>
          )}
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
  const isFailure = normalized === 'failure' || normalized === 'failure (ignored)' || normalized === 'rejected';
  const isCancelled = normalized === 'cancelled';
  const isRunning = normalized === 'running' || normalized === 'waiting_approval';
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

function StatusDot({ status, complete }: { status: string; complete?: boolean }) {
  return <span className={`inline-block h-2.5 w-2.5 rounded-full ${getStatusDotClass(status, complete)}`} aria-hidden="true" />;
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

export default AppShell;
