import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import type { AppAccess } from '../auth/capabilities';
import { PermissionGuard } from '../auth/permissionGuards';
import type { CurrentUser } from './types';

const PipelineRunsPage = lazy(() => import('../pages/PipelineRuns'));
const PipelinesPage = lazy(() => import('../pages/Pipelines'));
const SchedulesPage = lazy(() => import('../pages/Schedules'));
const TriggersPage = lazy(() => import('../pages/Triggers'));
const ExternalTriggersPage = lazy(() => import('../pages/ExternalTriggers'));
const ScopesPage = lazy(() => import('../pages/Scopes'));
const LabPage = lazy(() => import('../pages/Lab'));
const StepsPage = lazy(() => import('../pages/Steps'));
const KnowledgeContextPage = lazy(() => import('../pages/KnowledgeContext'));
const ProductDocsPage = lazy(() => import('../pages/ProductDocs'));
const MonitoringPage = lazy(() => import('../pages/Monitoring'));
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
  onLogout,
  onPasswordChanged,
  onUserUpdated,
}: {
  access: AppAccess;
  currentUser: CurrentUser | null;
  currentUserLoading: boolean;
  mustChangePassword: boolean;
  onLogout: () => void;
  onPasswordChanged: () => void;
  onUserUpdated: (updates: Partial<CurrentUser>) => void;
}) {
  return (
    <Suspense fallback={<PageLoading />}>
      <Routes>
        <Route path="/" element={<Navigate to="/pipelineruns/main" replace />} />
        <Route path="/pipelineruns/:tab?" element={<PipelineRunsPage />} />
        <Route path="/monitoring" element={<MonitoringPage />} />
        <Route path="/docs" element={<ProductDocsPage />} />
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
              />
            </PermissionGuard>
          }
        />
        <Route
          path="/system/:tab?"
          element={
            <PermissionGuard allowed={access.canViewAnySystem} loading={currentUserLoading}>
              <SystemPage permissions={access.systemPermissions} />
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
