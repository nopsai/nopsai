import type { AuthSession, CurrentUser, ReadCapabilities, ResourceCapabilities, SystemCapabilities } from '../app/types.js';

export type Permission =
  | 'pipelines.write'
  | 'pipelines.delete'
  | 'steps.write'
  | 'steps.delete'
  | 'schedules.read'
  | 'schedules.write'
  | 'schedules.delete'
  | 'triggers.read'
  | 'triggers.write'
  | 'triggers.delete'
  | 'external_triggers.read'
  | 'external_triggers.write'
  | 'external_triggers.delete'
  | 'scopes.read'
  | 'scopes.write'
  | 'scopes.delete'
  | 'knowledge_contexts.read'
  | 'knowledge_contexts.write'
  | 'knowledge_contexts.delete'
  | 'system.config.read'
  | 'system.config.write'
  | 'system.llm_profiles.read'
  | 'system.llm_profiles.write'
  | 'system.agent_profiles.read'
  | 'system.agent_profiles.write'
  | 'system.mcp.read'
  | 'system.mcp.write'
  | 'system.config_repos.read'
  | 'system.config_repos.write'
  | 'system.dispatcher.read'
  | 'system.dispatcher.write'
  | 'system.access';

export type SystemPagePermissions = {
  canViewConfig: boolean;
  canViewSetup: boolean;
  canManageSetup: boolean;
  canViewRuntimeConfig: boolean;
  canManageRuntimeConfig: boolean;
  canViewLLMProfiles: boolean;
  canManageLLMProfiles: boolean;
  canViewAgentProfiles: boolean;
  canManageAgentProfiles: boolean;
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

export type AppAccess = {
  draftScope: string;
  canWritePipelines: boolean;
  canDeletePipelines: boolean;
  canViewSchedules: boolean;
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
  canWriteSteps: boolean;
  canDeleteSteps: boolean;
  canViewTriggers: boolean;
  canDeleteTriggers: boolean;
  canViewExternalTriggers: boolean;
  canWriteExternalTriggers: boolean;
  canDeleteExternalTriggers: boolean;
  canViewScopes: boolean;
  canDeleteScopes: boolean;
  canViewKnowledge: boolean;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  canViewSystemRuntimeConfig: boolean;
  canManageSystemRuntimeConfig: boolean;
  canViewSystemConfigRepo: boolean;
  canManageSystemConfigRepo: boolean;
  canViewSystemConfig: boolean;
  canViewSystemSetup: boolean;
  canManageSystemSetup: boolean;
  canViewSystemLLMProfiles: boolean;
  canManageSystemLLMProfiles: boolean;
  canViewSystemAgentProfiles: boolean;
  canManageSystemAgentProfiles: boolean;
  canViewSystemMCP: boolean;
  canManageSystemMCP: boolean;
  canViewSystemDispatcher: boolean;
  canManageSystemDispatcher: boolean;
  canViewSystemAccess: boolean;
  canViewAnySystem: boolean;
  preferredSystemPath: string;
  isInitialAdminUser: boolean;
  systemPermissions: SystemPagePermissions;
};

type CapabilityPayload = Record<string, unknown>;

const asObject = (value: unknown): CapabilityPayload | null => {
  if (!value || typeof value !== 'object') return null;
  return value as CapabilityPayload;
};

const normalizeResourceCapabilities = (value: unknown): ResourceCapabilities | undefined => {
  const record = asObject(value);
  if (!record) return undefined;
  return {
    write: Boolean(record.write),
    delete: Boolean(record.delete),
  };
};

const normalizeReadCapabilities = (value: unknown): ReadCapabilities | undefined => {
  const record = asObject(value);
  if (!record) return undefined;
  return {
    read: Boolean(record.read),
    write: Boolean(record.write),
    delete: Boolean(record.delete),
  };
};

const normalizeSystemCapabilities = (value: unknown): SystemCapabilities | undefined => {
  const record = asObject(value);
  if (!record) return undefined;
  return {
    configRead: Boolean(record.config_read),
    configWrite: Boolean(record.config_write),
    llmProfilesRead: Boolean(record.llm_profiles_read),
    llmProfilesWrite: Boolean(record.llm_profiles_write),
    agentProfilesRead: Boolean(record.agent_profiles_read),
    agentProfilesWrite: Boolean(record.agent_profiles_write),
    mcpRead: Boolean(record.mcp_read),
    mcpWrite: Boolean(record.mcp_write),
    configReposRead: Boolean(record.config_repos_read),
    configReposWrite: Boolean(record.config_repos_write),
    dispatcherRead: Boolean(record.dispatcher_read),
    dispatcherWrite: Boolean(record.dispatcher_write),
    access: Boolean(record.access),
  };
};

export function normalizeCurrentUser(data: unknown): CurrentUser {
  const record = asObject(data) || {};
  const capabilitiesRecord = asObject(record.capabilities);
  const capabilities = capabilitiesRecord
    ? {
        pipelines: normalizeResourceCapabilities(capabilitiesRecord.pipelines),
        steps: normalizeResourceCapabilities(capabilitiesRecord.steps),
        schedules: normalizeReadCapabilities(capabilitiesRecord.schedules),
        triggers: normalizeReadCapabilities(capabilitiesRecord.triggers),
        external_triggers: normalizeReadCapabilities(capabilitiesRecord.external_triggers),
        scopes: normalizeReadCapabilities(capabilitiesRecord.scopes),
        knowledge_contexts: normalizeReadCapabilities(capabilitiesRecord.knowledge_contexts),
        system: normalizeSystemCapabilities(capabilitiesRecord.system),
      }
    : undefined;

  return {
    sub: typeof record.sub === 'string' ? record.sub : '',
    email: typeof record.email === 'string' ? record.email : '',
    roles: Array.isArray(record.roles) ? record.roles.filter((role): role is string => typeof role === 'string') : undefined,
    capabilities,
  };
}

export function can(user: CurrentUser | null | undefined, permission: Permission): boolean {
  const capabilities = user?.capabilities;
  switch (permission) {
    case 'pipelines.write':
      return Boolean(capabilities?.pipelines?.write);
    case 'pipelines.delete':
      return Boolean(capabilities?.pipelines?.delete);
    case 'steps.write':
      return Boolean(capabilities?.steps?.write);
    case 'steps.delete':
      return Boolean(capabilities?.steps?.delete);
    case 'schedules.read':
      return Boolean(capabilities?.schedules?.read);
    case 'schedules.write':
      return Boolean(capabilities?.schedules?.write);
    case 'schedules.delete':
      return Boolean(capabilities?.schedules?.delete);
    case 'triggers.read':
      return Boolean(capabilities?.triggers?.read);
    case 'triggers.write':
      return Boolean(capabilities?.triggers?.write);
    case 'triggers.delete':
      return Boolean(capabilities?.triggers?.delete);
    case 'external_triggers.read':
      return Boolean(capabilities?.external_triggers?.read);
    case 'external_triggers.write':
      return Boolean(capabilities?.external_triggers?.write);
    case 'external_triggers.delete':
      return Boolean(capabilities?.external_triggers?.delete);
    case 'scopes.read':
      return Boolean(capabilities?.scopes?.read);
    case 'scopes.write':
      return Boolean(capabilities?.scopes?.write);
    case 'scopes.delete':
      return Boolean(capabilities?.scopes?.delete);
    case 'knowledge_contexts.read':
      return Boolean(capabilities?.knowledge_contexts?.read);
    case 'knowledge_contexts.write':
      return Boolean(capabilities?.knowledge_contexts?.write);
    case 'knowledge_contexts.delete':
      return Boolean(capabilities?.knowledge_contexts?.delete);
    case 'system.config.read':
      return Boolean(capabilities?.system?.configRead);
    case 'system.config.write':
      return Boolean(capabilities?.system?.configWrite);
    case 'system.llm_profiles.read':
      return Boolean(capabilities?.system?.llmProfilesRead);
    case 'system.llm_profiles.write':
      return Boolean(capabilities?.system?.llmProfilesWrite);
    case 'system.agent_profiles.read':
      return Boolean(capabilities?.system?.agentProfilesRead);
    case 'system.agent_profiles.write':
      return Boolean(capabilities?.system?.agentProfilesWrite);
    case 'system.mcp.read':
      return Boolean(capabilities?.system?.mcpRead);
    case 'system.mcp.write':
      return Boolean(capabilities?.system?.mcpWrite);
    case 'system.config_repos.read':
      return Boolean(capabilities?.system?.configReposRead);
    case 'system.config_repos.write':
      return Boolean(capabilities?.system?.configReposWrite);
    case 'system.dispatcher.read':
      return Boolean(capabilities?.system?.dispatcherRead);
    case 'system.dispatcher.write':
      return Boolean(capabilities?.system?.dispatcherWrite);
    case 'system.access':
      return Boolean(capabilities?.system?.access);
    default:
      return false;
  }
}

export function getPreferredSystemPath(permissions: SystemPagePermissions): string {
  if (permissions.canViewConfig) return '/system/config';
  if (permissions.canViewSetup) return '/system/setup';
  if (permissions.canViewLLMProfiles) return '/system/llm-profiles';
  if (permissions.canViewAgentProfiles) return '/system/agent-profiles';
  if (permissions.canViewMCP) return '/system/mcp';
  if (permissions.canViewDispatcher) return '/system/dispatcher';
  if (permissions.canViewAccess) return '/system/access';
  return '/system/config';
}

export function getSystemPagePermissions(user: CurrentUser | null | undefined): SystemPagePermissions {
  const canViewRuntimeConfig = can(user, 'system.config.read');
  const canManageRuntimeConfig = can(user, 'system.config.write');
  const canViewGlobalConfigRepo = can(user, 'system.config_repos.read');
  const canManageGlobalConfigRepo = can(user, 'system.config_repos.write');

  return {
    canViewConfig: canViewRuntimeConfig || canViewGlobalConfigRepo,
    canViewSetup: canViewRuntimeConfig,
    canManageSetup: canManageRuntimeConfig,
    canViewRuntimeConfig,
    canManageRuntimeConfig,
    canViewLLMProfiles: can(user, 'system.llm_profiles.read') || canViewRuntimeConfig,
    canManageLLMProfiles: can(user, 'system.llm_profiles.write') || canManageRuntimeConfig,
    canViewAgentProfiles: can(user, 'system.agent_profiles.read') || canViewRuntimeConfig,
    canManageAgentProfiles: can(user, 'system.agent_profiles.write') || canManageRuntimeConfig,
    canViewMCP: can(user, 'system.mcp.read') || canViewRuntimeConfig,
    canManageMCP: can(user, 'system.mcp.write') || canManageRuntimeConfig,
    canViewDataManagement: canViewRuntimeConfig,
    canManageDataManagement: canManageRuntimeConfig,
    canViewGlobalConfigRepo,
    canManageGlobalConfigRepo,
    canViewDispatcher: can(user, 'system.dispatcher.read'),
    canManageDispatcher: can(user, 'system.dispatcher.write'),
    canViewAccess: can(user, 'system.access'),
  };
}

export function isInitialAdminUser(user: CurrentUser | null | undefined, session: AuthSession): boolean {
  const sub = (user?.sub || session.sub || '').trim().toLowerCase();
  const roles = user?.roles || session.roles || [];
  return sub === 'admin' || roles.some(role => role === 'nopsai-admin');
}

export function getAppAccess(user: CurrentUser | null | undefined, session: AuthSession): AppAccess {
  const systemPermissions = getSystemPagePermissions(user);
  const canViewAnySystem =
    systemPermissions.canViewConfig ||
    systemPermissions.canViewSetup ||
    systemPermissions.canViewLLMProfiles ||
    systemPermissions.canViewAgentProfiles ||
    systemPermissions.canViewMCP ||
    systemPermissions.canViewDispatcher ||
    systemPermissions.canViewAccess;

  return {
    draftScope: (session.sub || user?.sub || '').trim(),
    canWritePipelines: can(user, 'pipelines.write'),
    canDeletePipelines: can(user, 'pipelines.delete'),
    canViewSchedules: can(user, 'schedules.read'),
    canWriteSchedules: can(user, 'schedules.write'),
    canDeleteSchedules: can(user, 'schedules.delete'),
    canWriteSteps: can(user, 'steps.write'),
    canDeleteSteps: can(user, 'steps.delete'),
    canViewTriggers: can(user, 'triggers.read'),
    canDeleteTriggers: can(user, 'triggers.delete'),
    canViewExternalTriggers: can(user, 'external_triggers.read'),
    canWriteExternalTriggers: can(user, 'external_triggers.write'),
    canDeleteExternalTriggers: can(user, 'external_triggers.delete'),
    canViewScopes: can(user, 'scopes.read'),
    canDeleteScopes: can(user, 'scopes.delete'),
    canViewKnowledge: can(user, 'knowledge_contexts.read'),
    canWriteKnowledge: can(user, 'knowledge_contexts.write'),
    canDeleteKnowledge: can(user, 'knowledge_contexts.delete'),
    canViewSystemRuntimeConfig: systemPermissions.canViewRuntimeConfig,
    canManageSystemRuntimeConfig: systemPermissions.canManageRuntimeConfig,
    canViewSystemConfigRepo: systemPermissions.canViewGlobalConfigRepo,
    canManageSystemConfigRepo: systemPermissions.canManageGlobalConfigRepo,
    canViewSystemConfig: systemPermissions.canViewConfig,
    canViewSystemSetup: systemPermissions.canViewSetup,
    canManageSystemSetup: systemPermissions.canManageSetup,
    canViewSystemLLMProfiles: systemPermissions.canViewLLMProfiles,
    canManageSystemLLMProfiles: systemPermissions.canManageLLMProfiles,
    canViewSystemAgentProfiles: systemPermissions.canViewAgentProfiles,
    canManageSystemAgentProfiles: systemPermissions.canManageAgentProfiles,
    canViewSystemMCP: systemPermissions.canViewMCP,
    canManageSystemMCP: systemPermissions.canManageMCP,
    canViewSystemDispatcher: systemPermissions.canViewDispatcher,
    canManageSystemDispatcher: systemPermissions.canManageDispatcher,
    canViewSystemAccess: systemPermissions.canViewAccess,
    canViewAnySystem,
    preferredSystemPath: getPreferredSystemPath(systemPermissions),
    isInitialAdminUser: isInitialAdminUser(user, session),
    systemPermissions,
  };
}
