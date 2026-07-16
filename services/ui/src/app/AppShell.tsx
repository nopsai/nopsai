import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { NavLink, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { BranchIcon, IconMenu, IconX, RunIdIcon } from './icons';
import {
  SIDEBAR_COLLAPSED_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  SIDEBAR_SCROLL_BUFFER,
} from './constants';
import { baseNavItems, baseSystemSubNav, eventAutomationNavPath, pipelineRunsNavPath, titleMap } from './navigation';
import {
  formatBranch,
  formatBranchDisplay,
  formatRepoLabel,
  formatTriggerLabel,
  getSidebarStatusTone,
  getStatusDotClass,
  isRunAppTeam,
  normalizeRunTeamURLValue,
  normalizeRunStatus,
  runSidebarActivityTimestamp,
  runTeamDisplayName,
  runTeamPathForURL,
  runTeamRepositoryURL,
  runMatchesSearch,
  summarizeStatus,
  timeAgoShort,
} from './runSidebarUtils';
import { normalizeScopeLabel } from './resourceTrees';
import type {
  CurrentUser,
  NavItem,
  PipelineTreeNode,
  RunTeam,
  RunListItem,
  RunTabKey,
  ScopeTreeNode,
  StepTreeNode,
  Theme,
} from './types';
import { useSidebarState } from './useSidebarState';
import { useResourceTrees } from './useResourceTrees';
import { BaseSidebarNavigation } from './BaseSidebarNavigation';
import { useInitialSetupRedirect } from './useInitialSetupRedirect';
import { usePipelineRunsSidebar } from './usePipelineRunsSidebar';
import { shouldShowSidebarContextNav } from './sidebarContextVisibility';
import { AppRoutes, PageLoading } from './AppRoutes';
import { getAppAccess } from '../auth/capabilities';
import { useAuth } from '../auth/AuthContext';
import { buildLoginRedirectState, resolvePostLoginPath } from '../auth/authRedirect';
import AppHelp from '../components/AppHelp';
import AssistantDock from '../components/AssistantDock';
import BrandIdentity from '../components/BrandIdentity';
import { logoutCurrentSession } from '../lib/api';
import { buildPipelineRunsRoute, extractTeamPathFromRoute, teamScopedRoute } from '../lib/teamRoutes';
import { currentUserDisplayName } from './userIdentity';

const LoginPage = lazy(() => import('../pages/Login'));

const getInitialTheme = (): Theme => {
  if (typeof window === 'undefined') return 'light';
  const stored = localStorage.getItem('theme');
  if (stored === 'dark' || stored === 'light') return stored;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
};

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
    canViewSchedules,
    canViewDashboards,
    canWriteSteps,
    canViewTriggers,
    canViewExternalTriggers,
    canViewGitWebhookSources,
    canViewScopes,
    canViewKnowledge,
    canViewSystemRuntimeConfig,
    canViewSystemConfig,
    canViewSystemSetup,
    canViewSystemLLMProfiles,
    canViewSystemAgentProfiles,
    canViewSystemMCP,
    canViewSystemCredentials,
    canViewSystemDispatcher,
    canViewSystemLogs,
    canViewSystemAccess,
    isInitialAdminUser,
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
    void (async () => {
      const result = await logoutCurrentSession().catch((): { logoutURL?: string } => ({}));
      clearAuthSession();
      if (result.logoutURL) {
        window.location.assign(result.logoutURL);
        return;
      }
      navigate('/login', { replace: true });
    })();
  }, [clearAuthSession, navigate]);

  const handleUserUpdated = useCallback((updates: Partial<CurrentUser>) => {
    updateCurrentUser(updates);
  }, [updateCurrentUser]);
  const navItems = useMemo(() => {
    return baseNavItems
      .map(item => {
        if (item.path.startsWith('/pipelineruns')) return { ...item, path: pipelineRunsNavPath(location.pathname) };
        if (item.path === '/triggers') {
          return {
            ...item,
            path: eventAutomationNavPath({
              canViewTriggers,
              canViewExternalTriggers,
              canViewGitWebhookSources,
            }),
          };
        }
        return item;
      })
      .filter(item => {
        if (item.path === '/llm-profiles') return canViewSystemLLMProfiles;
        if (item.path === '/agent-profiles') return canViewSystemAgentProfiles;
        if (item.path === '/mcp') return canViewSystemMCP;
        if (item.path === '/credentials') return canViewSystemCredentials;
        if (item.path === '/schedules') return canViewSchedules;
        if (item.path === '/dashboards') return canViewDashboards;
        if (item.label === 'Triggers') return canViewTriggers || canViewExternalTriggers || canViewGitWebhookSources;
        if (item.path === '/external-triggers') return canViewExternalTriggers;
        if (item.path === '/git-webhook-sources') return canViewGitWebhookSources;
        if (item.path === '/scopes') return canViewScopes;
        if (item.path === '/knowledge-context') return canViewKnowledge;
        return true;
      });
  }, [canViewDashboards, canViewExternalTriggers, canViewGitWebhookSources, canViewKnowledge, canViewSchedules, canViewScopes, canViewSystemAgentProfiles, canViewSystemCredentials, canViewSystemLLMProfiles, canViewSystemMCP, canViewTriggers, location.pathname]);
  const systemSubNav = useMemo(
    () =>
      baseSystemSubNav.filter(item => {
        if (item.path === '/system/config') return canViewSystemConfig;
        if (item.path === '/system/setup') return canViewSystemSetup;
        if (item.path === '/system/data-management') return canViewSystemRuntimeConfig;
        if (item.path === '/system/dispatcher') return canViewSystemDispatcher;
        if (item.path === '/system/logs') return canViewSystemLogs;
        if (item.path === '/system/access') return canViewSystemAccess;
        return false;
      }),
    [canViewSystemAccess, canViewSystemConfig, canViewSystemDispatcher, canViewSystemLogs, canViewSystemRuntimeConfig, canViewSystemSetup]
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

  return (
    <div className="app-root-shell min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)]">
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
              collapsed={sidebar.collapsed}
              onToggleCollapsed={sidebar.toggleCollapsed}
              width={sidebar.width}
              pipelineTree={resourceTrees.pipelineTree}
              pipelineTreeOpen={resourceTrees.pipelineTreeOpen}
              onTogglePipelineNode={resourceTrees.onTogglePipelineNode}
              stepTree={resourceTrees.stepTree}
              stepTreeOpen={resourceTrees.stepTreeOpen}
              onToggleStepNode={resourceTrees.onToggleStepNode}
              scopeTree={resourceTrees.scopeTree}
              scopeTreeOpen={resourceTrees.scopeTreeOpen}
              onToggleScopeNode={resourceTrees.onToggleScopeNode}
              splitIdentifier={resourceTrees.splitIdentifier}
              locationPathname={location.pathname}
              locationSearch={location.search}
              navigateTo={navigate}
              onSelectPipelineTeam={path => navigate(teamScopedRoute('/pipelines', path))}
              onSelectStepTeam={path => navigate(teamScopedRoute('/steps', path))}
              onSelectScopeTeam={path => navigate(teamScopedRoute('/scopes', path))}
            />
            {!sidebar.collapsed ? (
              <div
                id="sidebar-resizer"
                className={`app-sidebar-resizer hidden sm:block w-2 cursor-col-resize flex-shrink-0 transition-colors duration-200 ${sidebar.isResizing ? 'app-sidebar-resizer--active' : ''}`}
                onMouseDown={sidebar.startResize}
                onTouchStart={sidebar.startResize}
                onKeyDown={sidebar.resizeWithKeyboard}
                role="separator"
                tabIndex={0}
                aria-label="Resize sidebar"
                aria-orientation="vertical"
                aria-valuemin={SIDEBAR_MIN_WIDTH}
                aria-valuemax={SIDEBAR_MAX_WIDTH}
                aria-valuenow={sidebar.width}
              ></div>
            ) : null}
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
                <AppRoutes
                  access={access}
                  currentUser={currentUser}
                  currentUserLoading={currentUserLoading}
                  mustChangePassword={Boolean(authSession.mustChangePassword)}
                  onLogout={handleLogout}
                  onPasswordChanged={markPasswordChanged}
                  onUserUpdated={handleUserUpdated}
                />
              </div>
            </main>
          </div>
          <div id="toast-container" className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3"></div>
          <AssistantDock />
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
  collapsed,
  onToggleCollapsed,
  width,
  pipelineTree,
  pipelineTreeOpen,
  onTogglePipelineNode,
  stepTree,
  stepTreeOpen,
  onToggleStepNode,
  scopeTree,
  scopeTreeOpen,
  onToggleScopeNode,
  splitIdentifier,
  locationPathname,
  locationSearch,
  navigateTo,
  onSelectPipelineTeam,
  onSelectStepTeam,
  onSelectScopeTeam,
}: {
  navItems: NavItem[];
  systemSubNav: NavItem[];
  open: boolean;
  onClose: () => void;
  collapsed: boolean;
  onToggleCollapsed: () => void;
  width: number;
  pipelineTree: PipelineTreeNode;
  pipelineTreeOpen: Set<string>;
  onTogglePipelineNode: (id: string) => void;
  stepTree: StepTreeNode;
  stepTreeOpen: Set<string>;
  onToggleStepNode: (id: string) => void;
  scopeTree: ScopeTreeNode;
  scopeTreeOpen: Set<string>;
  onToggleScopeNode: (id: string) => void;
  splitIdentifier: (id: string) => { name: string; path: string };
  locationPathname: string;
  locationSearch: string;
  navigateTo: (path: string) => void;
  onSelectPipelineTeam: (path: string) => void;
  onSelectStepTeam: (path: string) => void;
  onSelectScopeTeam: (path: string) => void;
}) {
  const sidebarWidth = collapsed ? SIDEBAR_COLLAPSED_WIDTH : width;
  const isPipelinesRoute = locationPathname.startsWith('/pipelines');
  const isTriggersRoute = locationPathname.startsWith('/triggers');
  const isStepsRoute = locationPathname.startsWith('/steps');
  const isScopesRoute = locationPathname.startsWith('/scopes');
  const isPipelineRunsRoute = locationPathname.startsWith('/pipelineruns');
  const searchParams = useMemo(() => new URLSearchParams(locationSearch), [locationSearch]);
  const pipelineRunsTab: RunTabKey =
    locationPathname.startsWith('/pipelineruns/recent') ? 'recent' : locationPathname.startsWith('/pipelineruns/events') ? 'events' : 'main';
  const showSidebarContextNav = shouldShowSidebarContextNav(locationPathname, pipelineRunsTab);
  const activeTeam = useMemo(() => {
    const root = isPipelinesRoute
      ? 'pipelines'
      : isTriggersRoute
        ? 'triggers'
        : isStepsRoute
          ? 'steps'
          : isScopesRoute
            ? 'scopes'
            : isPipelineRunsRoute
              ? 'pipelineruns'
              : '';
    return (root ? extractTeamPathFromRoute(locationPathname, root) : '') || searchParams.get('team') || '';
  }, [isPipelineRunsRoute, isPipelinesRoute, isScopesRoute, isStepsRoute, isTriggersRoute, locationPathname, searchParams]);

  const renderPipelineTreeNode = (node: PipelineTreeNode) => {
    const isOpen = pipelineTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveTeam = activeTeam === node.fullPath;
    return (
      <li key={node.id} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onTogglePipelineNode(node.id)}
              aria-label="Toggle team"
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
              className={`pipeline-tree-team flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveTeam ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onTogglePipelineNode(node.id);
                onSelectPipelineTeam(node.fullPath);
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

  const renderStepTreeNode = (node: StepTreeNode) => {
    const isOpen = stepTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveTeam = activeTeam === node.fullPath;
    return (
      <li key={`step-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onToggleStepNode(node.id)}
              aria-label="Toggle team"
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
              className={`pipeline-tree-team flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveTeam ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleStepNode(node.id);
                onSelectStepTeam(node.fullPath);
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
    const isActiveTeam = activeTeam === node.fullPath;
    return (
      <li key={`scope-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item flex items-center gap-2 rounded-md hover:bg-[var(--bg-tertiary)] px-1">
            <button
              className="pipeline-tree-toggle inline-flex items-center justify-center text-[var(--text-secondary)] hover:text-[var(--text-primary)] p-1"
              onClick={() => onToggleScopeNode(node.id)}
              aria-label="Toggle team"
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
              className={`pipeline-tree-team flex items-center gap-2 flex-1 min-w-0 text-left text-[var(--text-primary)] hover:text-[var(--text-primary)] px-2 py-1 rounded-md hover:bg-[var(--bg-tertiary)] ${isActiveTeam ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleScopeNode(node.id);
                onSelectScopeTeam(node.fullPath);
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
                    onClick={() => onSelectScopeTeam(node.fullPath)}
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

  return (
    <>
      <div
        className={`fixed inset-0 bg-[var(--bg-overlay)] sm:hidden transition-opacity duration-200 ${open ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}
        onClick={onClose}
      ></div>
      <aside
        id="sidebar"
        className={`app-sidebar-shell ${collapsed ? 'app-sidebar-shell--collapsed' : ''} bg-[var(--bg-secondary)] flex-shrink-0 flex flex-col transition-transform duration-300 ease-in-out h-full z-20 w-80 sidebar-scrollbar overflow-hidden
          ${open ? 'translate-x-0' : '-translate-x-full'} sm:translate-x-0 fixed sm:static`}
        style={{
          width: sidebarWidth,
          minWidth: collapsed ? SIDEBAR_COLLAPSED_WIDTH : SIDEBAR_MIN_WIDTH,
          maxWidth: collapsed ? SIDEBAR_COLLAPSED_WIDTH : SIDEBAR_MAX_WIDTH,
        }}
      >
        <div className="app-sidebar-brand-row flex items-center justify-between px-6 h-16 flex-shrink-0">
          <BrandIdentity className="sidebar-brand" variant={collapsed ? 'mark' : 'wordmark'} />
          <div className="app-sidebar-brand-actions">
            <button
              id="collapse-sidebar-btn"
              type="button"
              className="app-sidebar-icon-button hidden sm:inline-flex"
              onClick={onToggleCollapsed}
              aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              aria-pressed={collapsed}
              title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              <IconMenu />
            </button>
            <button
              id="close-sidebar-btn"
              className="sm:hidden text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
              onClick={onClose}
              aria-label="Close sidebar"
            >
              <IconX />
            </button>
          </div>
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto sidebar-scrollbar">
          <BaseSidebarNavigation navItems={navItems} systemSubNav={systemSubNav} locationPathname={locationPathname} />
          {showSidebarContextNav && !collapsed && (
            <nav id="sidebar-details-nav" className="sidebar-context-nav px-4 py-4 space-y-2" aria-label="Contextual">
              {isPipelineRunsRoute ? (
                <PipelineRunsSidebarContent
                  tab={pipelineRunsTab}
                  searchParams={searchParams}
                  locationPathname={locationPathname}
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
              ) : (
                <p className="text-xs text-[var(--text-secondary)]">Contextual navigation will appear here as features are migrated.</p>
              )}
            </nav>
          )}
        </div>
      </aside>
    </>
  );
}

function PipelineRunsSidebarContent({
  tab,
  searchParams,
  locationPathname,
  navigateTo,
  onClose,
}: {
  tab: RunTabKey;
  searchParams: URLSearchParams;
  locationPathname: string;
  navigateTo: (path: string) => void;
  onClose?: () => void;
}) {
  const autoScrolledRunIdRef = useRef<string | null>(null);
  const searchTerm = (searchParams.get('q') || '').trim().toLowerCase();
  const activeRunId = searchParams.get('run');
  const activeTeamValue = useMemo(
    () => normalizeRunTeamURLValue(extractTeamPathFromRoute(locationPathname, 'pipelineruns') || searchParams.get('team')),
    [locationPathname, searchParams]
  );
  const {
    teams,
    teamsLoading,
    recentRuns,
    runsLoading,
    recentHasMore,
    recentLoadingMore,
    expandedTeams,
    expandedBranches,
    repoRunsCache,
    loadingRepos,
    loadMoreRecentRuns,
    toggleTeam,
    toggleBranch,
  } = usePipelineRunsSidebar({ activeTeamValue, activeRunId, tab });

  const handleSelectTeam = useCallback(
    (teamId: number | null) => {
      const params = new URLSearchParams(searchParams);
      const team = teamId === null ? null : teams.find(item => item.id === teamId) || null;
      const teamPath = team ? runTeamPathForURL(team, teams) : '';
      params.delete('team');
      params.delete('run');
      const base = buildPipelineRunsRoute(tab, teamPath);
      navigateTo(`${base}${params.toString() ? `?${params.toString()}` : ''}`);
      onClose?.();
    },
    [navigateTo, onClose, searchParams, tab, teams]
  );

  const handleOpenRun = useCallback(
    (runId: string, teamId?: number | null) => {
      const params = new URLSearchParams(searchParams);
      let teamPath = activeTeamValue;
      if (teamId) {
        const team = teams.find(item => item.id === teamId) || null;
        if (team) teamPath = runTeamPathForURL(team, teams);
      }
      params.delete('team');
      params.set('run', runId);
      const base = buildPipelineRunsRoute(tab, teamPath);
      navigateTo(`${base}?${params.toString()}`);
      onClose?.();
    },
    [activeTeamValue, navigateTo, onClose, searchParams, tab, teams]
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

  const rootTeams = useMemo(() => teams.filter(g => (g.parent_id ?? null) === null), [teams]);
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

  const renderRunRow = (run: RunListItem, teamId?: number | null) => (
    <RunSidebarRow
      key={run.run_id}
      run={run}
      active={activeRunId === run.run_id}
      onOpen={() => handleOpenRun(run.run_id, teamId)}
    />
  );

  const renderBranchRuns = (teamId: number, branch: string, runs: RunListItem[]) => {
    const key = `${teamId}:${branch}`;
    const expanded = expandedBranches.has(key);
    const filteredRuns = searchTerm ? runs.filter(run => runMatchesSearch(run, searchTerm)) : runs;
    if (searchTerm && filteredRuns.length === 0) return null;
    const branchLabel = formatBranch(branch);
    return (
      <div key={key} className="pipeline-runs-sidebar-branch border border-[var(--border-primary)] rounded-lg overflow-hidden bg-[var(--bg-primary)]">
        <button
          type="button"
          onClick={() => toggleBranch(teamId, branch)}
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
                    {renderRunRow(run, teamId)}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  const renderTeamNode = (team: RunTeam) => {
    const isRepo = isRunAppTeam(team);
    const expanded = expandedTeams.has(team.id);
    const children = teams.filter(child => (child.parent_id ?? null) === team.id);
    const label = runTeamDisplayName(team);
    const repoURL = runTeamRepositoryURL(team);
    const repoRuns = repoRunsCache.get(team.id);
    const isLoadingRepo = loadingRepos.has(team.id);

    return (
      <div key={team.id} className="pipeline-runs-sidebar-team space-y-2">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => toggleTeam(team)}
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
            onClick={() => handleSelectTeam(team.id)}
            aria-expanded={expanded}
            title={isRepo ? 'Open in main view' : 'Open team in main view'}
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
            <span className="text-sm text-[var(--text-primary)] truncate" title={team.name}>
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
                    .map(([branch, runs]) => renderBranchRuns(team.id, branch, runs))}
              </>
            ) : (
              children
                .sort((a, b) => a.name.localeCompare(b.name))
                .map(child => renderTeamNode(child))
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="pipeline-runs-sidebar-context space-y-3">
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
              onClick={() => handleSelectTeam(null)}
          >
            Root
          </button>
        </div>
          {teamsLoading && <div className="text-xs text-[var(--text-secondary)]">Loading teams...</div>}
          {!teamsLoading && rootTeams.length === 0 && <div className="text-xs text-[var(--text-secondary)]">No teams defined yet.</div>}
          {!teamsLoading && rootTeams.map(team => renderTeamNode(team))}
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
      className={`sidebar-run-link w-full text-left rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] hover:border-[var(--border-accent)] transition shadow-sm px-3 py-2 ${
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
          {timeAgoShort(runSidebarActivityTimestamp(run))}
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
  const menuButtonRef = useRef<HTMLButtonElement | null>(null);
  const displayName = currentUserDisplayName(currentUser);

  useEffect(() => {
    if (!menuOpen) return;
    const handleClick = (event: MouseEvent) => {
      if (!menuRef.current) return;
      if (!menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMenuOpen(false);
        requestAnimationFrame(() => menuButtonRef.current?.focus());
      }
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
      className="app-header-shell relative flex items-center justify-between px-6 py-4 themed-bg-blur backdrop-blur-sm z-40 flex-shrink-0"
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
      <h1 id="main-header" className="flex-1 text-xl font-semibold min-w-0 truncate">{title}</h1>
      <div className="flex items-center gap-3">
        <AppHelp />
        <div className="relative" ref={menuRef}>
          <button
            ref={menuButtonRef}
            type="button"
            onClick={() => setMenuOpen(open => !open)}
            className={`flex items-center gap-3 px-3 h-11 rounded-full border bg-[var(--bg-secondary)] text-[var(--text-primary)] shadow-sm transition-all ${
              menuOpen ? 'border-[var(--border-accent)]' : 'border-[var(--border-primary)] hover:border-[var(--border-accent)]'
            } focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]`}
            aria-label={`Open user menu for ${displayName}`}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            aria-controls={menuOpen ? 'user-menu' : undefined}
          >
            <span className="text-sm font-semibold text-[var(--text-primary)] max-w-[160px] truncate">{displayName}</span>
          </button>
          {menuOpen && (
            <div
              id="user-menu"
              className="absolute right-0 mt-2 w-72 rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-2xl overflow-hidden z-[500]"
              role="menu"
              aria-label="User menu"
            >
              <div className="p-4 border-b border-[var(--border-primary)] bg-[var(--bg-tertiary)]/70 backdrop-blur-sm">
                <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] mb-1">Signed in as</p>
                <p className="text-sm font-semibold text-[var(--text-primary)] truncate">{userLoading ? 'Loading…' : displayName}</p>
                <p className="text-xs text-[var(--text-secondary)] mt-2">Global access model</p>
              </div>
              <div className="p-2 space-y-1">
                <button
                  role="menuitem"
                  className="w-full text-left px-3 py-2 rounded-lg hover:bg-[var(--bg-tertiary)] text-[var(--text-primary)] text-sm"
                  onClick={() => {
                    closeMenu();
                    onOpenProfile?.();
                  }}
                >
                  View profile
                </button>
                <button
                  role="menuitem"
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
                    role="menuitem"
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
