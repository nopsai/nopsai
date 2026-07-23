import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import type { AppAccess } from '../auth/capabilities';
import { PermissionGuard } from '../auth/permissionGuards';
import type { CurrentUser } from './types';
import { isInitialSetupAllowedRoute, type InitialSetupGate } from './useInitialSetupRedirect';

const PipelineRunsPage = lazy(() => import('../pages/PipelineRuns'));
const TeamsPage = lazy(() => import('../pages/Teams'));
const PipelinesPage = lazy(() => import('../pages/Pipelines'));
const SchedulesPage = lazy(() => import('../pages/Schedules'));
const DashboardsPage = lazy(() => import('../pages/Dashboards'));
const TriggersPage = lazy(() => import('../pages/Triggers'));
const ExternalTriggersPage = lazy(() => import('../pages/ExternalTriggers'));
const GitWebhookSourcesPage = lazy(() => import('../pages/GitWebhookSources'));
const ScopesPage = lazy(() => import('../pages/Scopes'));
const LabPage = lazy(() => import('../pages/Lab'));
const StepsPage = lazy(() => import('../pages/Steps'));
const KnowledgeContextPage = lazy(() => import('../pages/KnowledgeContext'));
const ProductDocsPage = lazy(() => import('../pages/ProductDocs'));
const MonitoringPage = lazy(() => import('../pages/Monitoring'));
const AssistantPage = lazy(() => import('../pages/Assistant'));
const LLMProfilesPage = lazy(() => import('../pages/LLMProfiles'));
const AgentProfilesPage = lazy(() => import('../pages/AgentProfiles'));
const MCPPage = lazy(() => import('../pages/MCP'));
const CredentialsPage = lazy(() => import('../pages/Credentials'));
const SystemPage = lazy(() => import('../pages/System'));
const ProfilePage = lazy(() => import('../pages/Profile'));

function PageLoading() {
  return <div className="p-6 text-sm text-[var(--text-secondary)]">Loading...</div>;
}

export function AppRoutes({
  access,
  currentUser,
  currentUserLoading,
  mustChangePassword,
  setupGate,
  onLogout,
  onPasswordChanged,
  onSetupStatusChange,
  onUserUpdated,
}: {
  access: AppAccess;
  currentUser: CurrentUser | null;
  currentUserLoading: boolean;
  mustChangePassword: boolean;
  setupGate?: InitialSetupGate;
  onLogout: () => void;
  onPasswordChanged: () => void;
  onSetupStatusChange?: InitialSetupGate['recordStatus'];
  onUserUpdated: (updates: Partial<CurrentUser>) => void;
}) {
  const location = useLocation();
  const setupRouteAllowed = isInitialSetupAllowedRoute(location.pathname, mustChangePassword);

  if (setupGate?.checking && !setupRouteAllowed) {
    return <PageLoading />;
  }

  if (setupGate?.required && !setupRouteAllowed) {
    return <Navigate to="/system/setup" replace />;
  }

  return (
    <Suspense fallback={<PageLoading />}>
      <Routes>
        <Route path="/" element={<Navigate to="/pipelineruns/main" replace />} />
        <Route path="/pipelineruns/:tab/team/*" element={<PipelineRunsPage />} />
        <Route path="/pipelineruns/:tab/:runID" element={<PipelineRunsPage />} />
        <Route path="/pipelineruns/:tab?" element={<PipelineRunsPage />} />
        <Route path="/monitoring" element={<MonitoringPage />} />
        <Route path="/teams/*" element={<TeamsPage />} />
        <Route path="/assistant" element={<AssistantPage />} />
        <Route
          path="/llm-profiles"
          element={
            <PermissionGuard allowed={access.canViewSystemLLMProfiles} loading={currentUserLoading}>
              <LLMProfilesPage canManage={access.canManageSystemLLMProfiles} />
            </PermissionGuard>
          }
        />
        <Route
          path="/agent-profiles"
          element={
            <PermissionGuard allowed={access.canViewSystemAgentProfiles} loading={currentUserLoading}>
              <AgentProfilesPage canManage={access.canManageSystemAgentProfiles} />
            </PermissionGuard>
          }
        />
        <Route
          path="/mcp"
          element={
            <PermissionGuard allowed={access.canViewSystemMCP} loading={currentUserLoading}>
              <MCPPage canManage={access.canManageSystemMCP} />
            </PermissionGuard>
          }
        />
        <Route
          path="/credentials"
          element={
            <PermissionGuard allowed={access.canViewSystemCredentials} loading={currentUserLoading}>
              <CredentialsPage
                canManage={access.canManageSystemCredentials}
                isNopsAIAdmin={access.isNopsAIAdmin}
              />
            </PermissionGuard>
          }
        />
        <Route path="/docs/*" element={<ProductDocsPage />} />
        <Route
          path="/pipelines/*"
          element={
            <PipelinesPage
              draftScope={access.draftScope}
              canDeletePipelines={access.canDeletePipelines}
            />
          }
        />
        <Route
          path="/schedules/*"
          element={
            <PermissionGuard allowed={access.canViewSchedules} loading={currentUserLoading}>
              <SchedulesPage
                canWriteSchedules={access.canWriteSchedules}
                canDeleteSchedules={access.canDeleteSchedules}
              />
            </PermissionGuard>
          }
        />
        <Route
          path="/dashboards/*"
          element={
            <PermissionGuard allowed={access.canViewDashboards} loading={currentUserLoading}>
              <DashboardsPage
                canWriteDashboards={access.canWriteDashboards}
                canDeleteDashboards={access.canDeleteDashboards}
              />
            </PermissionGuard>
          }
        />
        <Route
          path="/triggers/*"
          element={
            <PermissionGuard allowed={access.canViewTriggers} loading={currentUserLoading}>
              <TriggersPage canDeleteTriggers={access.canDeleteTriggers} />
            </PermissionGuard>
          }
        />
        <Route
          path="/external-triggers/*"
          element={
            <PermissionGuard allowed={access.canViewExternalTriggers} loading={currentUserLoading}>
              <ExternalTriggersPage
                canWriteExternalTriggers={access.canWriteExternalTriggers}
                canDeleteExternalTriggers={access.canDeleteExternalTriggers}
              />
            </PermissionGuard>
          }
        />
        <Route
          path="/git-webhook-sources/*"
          element={
            <PermissionGuard allowed={access.canViewGitWebhookSources} loading={currentUserLoading}>
              <GitWebhookSourcesPage
                canWriteGitWebhookSources={access.canWriteGitWebhookSources}
                canDeleteGitWebhookSources={access.canDeleteGitWebhookSources}
              />
            </PermissionGuard>
          }
        />
        <Route
          path="/scopes/*"
          element={
            <PermissionGuard allowed={access.canViewScopes} loading={currentUserLoading}>
              <ScopesPage canDeleteScopes={access.canDeleteScopes} />
            </PermissionGuard>
          }
        />
        <Route path="/lab/*" element={<LabPage />} />
        <Route
          path="/steps/*"
          element={
            <StepsPage
              draftScope={access.draftScope}
              canDeleteSteps={access.canDeleteSteps}
            />
          }
        />
        <Route
          path="/knowledge-context/*"
          element={
            <PermissionGuard allowed={access.canViewKnowledge} loading={currentUserLoading}>
              <KnowledgeContextPage
                canWriteKnowledge={access.canWriteKnowledge}
                canDeleteKnowledge={access.canDeleteKnowledge}
                canWriteKnowledgeConnections={access.canWriteKnowledgeConnections}
                canDeleteKnowledgeConnections={access.canDeleteKnowledgeConnections}
              />
            </PermissionGuard>
          }
        />
        <Route path="/system/llm-profiles" element={<Navigate to="/llm-profiles" replace />} />
        <Route path="/system/agent-profiles" element={<Navigate to="/agent-profiles" replace />} />
        <Route path="/system/mcp" element={<Navigate to="/mcp" replace />} />
        <Route path="/system/credentials" element={<Navigate to={{ pathname: '/credentials', search: location.search }} replace />} />
        <Route
          path="/system/:tab?"
          element={
            <PermissionGuard allowed={access.canViewAnySystem} loading={currentUserLoading}>
              <SystemPage permissions={access.systemPermissions} onSetupStatusChange={onSetupStatusChange} />
            </PermissionGuard>
          }
        />
        <Route
          path="/profile"
          element={
            <ProfilePage
              user={currentUser}
              loading={currentUserLoading}
              onLogout={onLogout}
              onUserUpdated={onUserUpdated}
              mustChangePassword={mustChangePassword}
              onPasswordChanged={onPasswordChanged}
              canAccessSystem={access.canViewAnySystem}
              systemPath={access.preferredSystemPath}
            />
          }
        />
        <Route path="*" element={<Navigate to="/pipelineruns/main" replace />} />
      </Routes>
    </Suspense>
  );
}

export { PageLoading };
