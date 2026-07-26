import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';
import type { SystemPagePermissions } from '../auth/capabilities';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import DataManagementPanel from '../features/system/DataManagementPanel';
import SetupWizard from '../features/system/SetupWizard';
import DispatcherPanel from '../features/system/DispatcherPanel';
import { useSystemDispatcher } from '../features/system/dispatcher/useSystemDispatcher';
import GitHubAppPanel from '../features/system/git-apps/GitHubAppPanel';
import { useGitHubApp } from '../features/system/git-apps/useGitHubApp';
import SystemConfig from '../features/system/SystemConfig';
import AccessPanel from '../features/system/AccessPanel';
import { useSystemAccess } from '../features/system/access/useSystemAccess';
import { useSystemConfig } from '../features/system/config/useSystemConfig';
import SystemLogsPanel from '../features/system/logs/SystemLogsPanel';
import type { SetupStatus } from '../features/system/setup/model';

type SystemTab = 'config' | 'git-apps' | 'setup' | 'data-management' | 'dispatcher' | 'logs' | 'access';

function resolveSystemTab(tab?: string): SystemTab {
  if (
    tab === 'setup' ||
    tab === 'git-apps' ||
    tab === 'dispatcher' ||
    tab === 'logs' ||
    tab === 'access' ||
    tab === 'data-management'
  ) {
    return tab;
  }
  return 'config';
}

function SystemPage({
  permissions,
  onSetupStatusChange,
}: {
  permissions: SystemPagePermissions;
  onSetupStatusChange?: (status: SetupStatus) => void;
}) {
  const params = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = resolveSystemTab(params.tab);
  const allowedTabs = useMemo(() => {
    const tabs: SystemTab[] = [];
    if (permissions.canViewConfig) tabs.push('config');
    if (permissions.canViewGitApps) tabs.push('git-apps');
    if (permissions.canViewSetup) tabs.push('setup');
    if (permissions.canViewDataManagement) tabs.push('data-management');
    if (permissions.canViewDispatcher) tabs.push('dispatcher');
    if (permissions.canViewLogs) tabs.push('logs');
    if (permissions.canViewAccess) tabs.push('access');
    return tabs;
  }, [
    permissions.canViewAccess,
    permissions.canViewConfig,
    permissions.canViewDataManagement,
    permissions.canViewDispatcher,
    permissions.canViewGitApps,
    permissions.canViewLogs,
    permissions.canViewSetup,
  ]);
  const visibleTab = allowedTabs.includes(activeTab) ? activeTab : allowedTabs[0] ?? activeTab;
  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

  useEffect(() => {
    if (allowedTabs.includes(activeTab)) return;
    const nextTab = allowedTabs[0];
    if (!nextTab) return;
    navigate(`/system/${nextTab}`, { replace: true });
  }, [activeTab, allowedTabs, navigate]);

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const systemConfig = useSystemConfig({
    runtimeConfigEnabled: permissions.canViewRuntimeConfig && (visibleTab === 'config' || visibleTab === 'dispatcher'),
    mailSettingsEnabled: permissions.canViewRuntimeConfig && visibleTab === 'config',
    globalConfigRepoEnabled: permissions.canViewGlobalConfigRepo && visibleTab === 'config',
    canManageRuntimeConfig: permissions.canManageRuntimeConfig,
    canViewGlobalConfigRepo: permissions.canViewGlobalConfigRepo,
    canManageGlobalConfigRepo: permissions.canManageGlobalConfigRepo,
    addToast,
  });

  const accessPanel = useSystemAccess({
    enabled: permissions.canViewAccess && visibleTab === 'access',
    addToast,
  });

  const dispatcherPanel = useSystemDispatcher({
    enabled: permissions.canViewDispatcher && visibleTab === 'dispatcher',
    locationSearch: location.search,
    addToast,
  });

  const gitHubApp = useGitHubApp({
    enabled: permissions.canViewGitApps && visibleTab === 'git-apps',
    canManage: permissions.canManageGitApps,
    addToast,
  });

  return (
    <div data-page="system" className="active p-6 space-y-6">
      {visibleTab === 'config' && (
        <SystemConfig
          {...systemConfig.panelProps}
          canViewRuntimeConfig={permissions.canViewRuntimeConfig}
          canManageRuntimeConfig={permissions.canManageRuntimeConfig}
          canViewGlobalConfigRepo={permissions.canViewGlobalConfigRepo}
          canManageGlobalConfigRepo={permissions.canManageGlobalConfigRepo}
        />
      )}
      {visibleTab === 'setup' && (
        <SetupWizard canManage={permissions.canManageSetup} onStatusChange={onSetupStatusChange} />
      )}
      {visibleTab === 'git-apps' && (
        <GitHubAppPanel controller={gitHubApp} canManage={permissions.canManageGitApps} />
      )}
      {visibleTab === 'data-management' && (
        <DataManagementPanel canManage={permissions.canManageDataManagement} />
      )}
      {visibleTab === 'dispatcher' && (
        <DispatcherPanel
          {...dispatcherPanel}
          canManageDispatcher={permissions.canManageDispatcher}
          canViewRuntimeConfig={permissions.canViewRuntimeConfig}
          canManageRuntimeConfig={permissions.canManageRuntimeConfig}
          runnerDefaults={systemConfig.config}
          config={systemConfig.config}
          fieldMetadata={systemConfig.panelProps.fieldMetadata}
          configLoading={systemConfig.panelProps.configLoading}
          saving={systemConfig.panelProps.saving}
          onConfigChange={systemConfig.panelProps.onChange}
          onSaveConfig={systemConfig.panelProps.onSave}
        />
      )}
      {visibleTab === 'logs' && <SystemLogsPanel />}
      {visibleTab === 'access' && (
        <AccessPanel {...accessPanel} />
      )}

      <WorkflowToastRegion toasts={toasts} />

      {systemConfig.globalConfigRepoDriftOpen && (
        <ConfigRepositoryDriftModal
          {...systemConfig.driftModalProps}
          canPush={systemConfig.globalConfigRepoDriftCanPush}
        />
      )}
    </div>
  );
}

export default SystemPage;
