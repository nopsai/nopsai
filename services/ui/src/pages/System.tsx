import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';
import type { SystemPagePermissions } from '../auth/capabilities';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import AgentProfilesPanel from '../features/system/AgentProfilesPanel';
import LLMProfilesPanel from '../features/system/LLMProfilesPanel';
import MCPPanel from '../features/system/MCPPanel';
import CredentialsPanel from '../features/system/CredentialsPanel';
import DataManagementPanel from '../features/system/DataManagementPanel';
import SetupWizard from '../features/system/SetupWizard';
import DispatcherPanel from '../features/system/DispatcherPanel';
import { useSystemDispatcher } from '../features/system/dispatcher/useSystemDispatcher';
import SystemConfig from '../features/system/SystemConfig';
import AccessPanel from '../features/system/AccessPanel';
import { useSystemAccess } from '../features/system/access/useSystemAccess';
import { useSystemConfig } from '../features/system/config/useSystemConfig';

type SystemTab = 'config' | 'setup' | 'llm-profiles' | 'agent-profiles' | 'mcp' | 'credentials' | 'data-management' | 'dispatcher' | 'access';

function resolveSystemTab(tab?: string): SystemTab {
  if (
    tab === 'setup' ||
    tab === 'dispatcher' ||
    tab === 'access' ||
    tab === 'llm-profiles' ||
    tab === 'agent-profiles' ||
    tab === 'mcp' ||
    tab === 'credentials' ||
    tab === 'data-management'
  ) {
    return tab;
  }
  return 'config';
}

function SystemPage({ permissions }: { permissions: SystemPagePermissions }) {
  const params = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = resolveSystemTab(params.tab);
  const allowedTabs = useMemo(() => {
    const tabs: SystemTab[] = [];
    if (permissions.canViewConfig) tabs.push('config');
    if (permissions.canViewSetup) tabs.push('setup');
    if (permissions.canViewLLMProfiles) tabs.push('llm-profiles');
    if (permissions.canViewAgentProfiles) tabs.push('agent-profiles');
    if (permissions.canViewMCP) tabs.push('mcp');
    if (permissions.canViewCredentials) tabs.push('credentials');
    if (permissions.canViewDataManagement) tabs.push('data-management');
    if (permissions.canViewDispatcher) tabs.push('dispatcher');
    if (permissions.canViewAccess) tabs.push('access');
    return tabs;
  }, [
    permissions.canViewAccess,
    permissions.canViewConfig,
    permissions.canViewDataManagement,
    permissions.canViewDispatcher,
    permissions.canViewAgentProfiles,
    permissions.canViewLLMProfiles,
    permissions.canViewMCP,
    permissions.canViewCredentials,
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
      {visibleTab === 'llm-profiles' && (
        <LLMProfilesPanel canManage={permissions.canManageLLMProfiles} />
      )}
      {visibleTab === 'agent-profiles' && (
        <AgentProfilesPanel canManage={permissions.canManageAgentProfiles} />
      )}
      {visibleTab === 'setup' && (
        <SetupWizard canManage={permissions.canManageSetup} />
      )}
      {visibleTab === 'mcp' && (
        <MCPPanel canManage={permissions.canManageMCP} />
      )}
      {visibleTab === 'credentials' && (
        <CredentialsPanel canManage={permissions.canManageCredentials} />
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
