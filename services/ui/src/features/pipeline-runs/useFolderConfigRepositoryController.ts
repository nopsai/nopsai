import { useCallback, useEffect, useState } from 'react';
import { fetchGroupConfigRepository } from './api';
import { buildGroupPath, type Group } from './runPresentation';
import {
  createEmptyNotificationRouteForm,
  defaultNotificationRouteDefinition,
  normalizeNotificationRouteRecord,
  notificationRouteFormFromDefinition,
  notificationRoutePayloadFromForm,
  type NotificationRouteFormState,
  type NotificationRouteRecord,
} from './notificationRoutes';
import {
  buildConfigRepositoryWriteFiles,
  type ConfigRepositoryCommitResponse,
  type ConfigRepositoryDriftResponse,
} from '../../lib/configRepositoryDrift';

export type PipelineRunsConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
  last_sync_status: string;
  last_sync_message?: string;
  last_sync_started_at?: string;
  last_sync_completed_at?: string;
  last_sync_commit_sha?: string;
};

export type PipelineRunsConfigRepositoryFormState = {
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
};

type FolderConfigRepositorySelection = {
  group: Group;
  folderPath: string;
};

type FetchJson = <T>(path: string, options?: RequestInit) => Promise<T>;

type UseFolderConfigRepositoryControllerOptions = {
  groups: Group[];
  fetchJson: FetchJson;
  checkAccessPermission: (action: string, resourceType: string, resourceID: string) => Promise<boolean>;
};

const emptyConfigRepositoryForm: PipelineRunsConfigRepositoryFormState = {
  repo_url: '',
  branch: 'main',
  base_path: '',
  enabled: true,
  write_enabled: false,
  write_branch: 'nopsai/ui-changes',
};

const emptyNotificationRouteForm = createEmptyNotificationRouteForm();

function normalizeConfigRepository(payload: unknown): PipelineRunsConfigRepository | null {
  if (!payload || typeof payload !== 'object') return null;
  const record = payload as Record<string, unknown>;
  const id = typeof record.id === 'number' ? record.id : Number(record.id);
  return {
    id: Number.isFinite(id) ? id : 0,
    scope_type: typeof record.scope_type === 'string' ? record.scope_type : '',
    scope_id: typeof record.scope_id === 'string' ? record.scope_id : '',
    repo_url: typeof record.repo_url === 'string' ? record.repo_url : '',
    branch: typeof record.branch === 'string' && record.branch.trim() ? record.branch : 'main',
    base_path: typeof record.base_path === 'string' ? record.base_path : '',
    enabled: Boolean(record.enabled),
    write_enabled: Boolean(record.write_enabled),
    write_branch: typeof record.write_branch === 'string' && record.write_branch.trim() ? record.write_branch : 'nopsai/ui-changes',
    last_sync_status: typeof record.last_sync_status === 'string' ? record.last_sync_status : '',
    last_sync_message: typeof record.last_sync_message === 'string' ? record.last_sync_message : undefined,
    last_sync_started_at: typeof record.last_sync_started_at === 'string' ? record.last_sync_started_at : undefined,
    last_sync_completed_at: typeof record.last_sync_completed_at === 'string' ? record.last_sync_completed_at : undefined,
    last_sync_commit_sha: typeof record.last_sync_commit_sha === 'string' ? record.last_sync_commit_sha : undefined,
  };
}

function normalizeNotificationRoute(payload: unknown): NotificationRouteRecord {
  return normalizeNotificationRouteRecord(payload);
}

export function useFolderConfigRepositoryController({
  groups,
  fetchJson,
  checkAccessPermission,
}: UseFolderConfigRepositoryControllerOptions) {
  const [configRepoFolder, setConfigRepoFolder] = useState<FolderConfigRepositorySelection | null>(null);
  const [configRepo, setConfigRepo] = useState<PipelineRunsConfigRepository | null>(null);
  const [configRepoForm, setConfigRepoForm] = useState<PipelineRunsConfigRepositoryFormState>(emptyConfigRepositoryForm);
  const [configRepoLoading, setConfigRepoLoading] = useState(false);
  const [configRepoSaving, setConfigRepoSaving] = useState(false);
  const [configRepoSyncing, setConfigRepoSyncing] = useState(false);
  const [configRepoError, setConfigRepoError] = useState<string | null>(null);
  const [configRepoDriftOpen, setConfigRepoDriftOpen] = useState(false);
  const [configRepoDrift, setConfigRepoDrift] = useState<ConfigRepositoryDriftResponse | null>(null);
  const [configRepoDriftLoading, setConfigRepoDriftLoading] = useState(false);
  const [configRepoDriftError, setConfigRepoDriftError] = useState<string | null>(null);
  const [configRepoPushing, setConfigRepoPushing] = useState(false);
  const [configRepoPushResult, setConfigRepoPushResult] = useState<ConfigRepositoryCommitResponse | null>(null);
  const [configRepoManageAllowed, setConfigRepoManageAllowed] = useState(false);
  const [configRepoSyncAllowed, setConfigRepoSyncAllowed] = useState(false);
  const [notificationRoute, setNotificationRoute] = useState<NotificationRouteRecord | null>(null);
  const [notificationRouteForm, setNotificationRouteForm] = useState<NotificationRouteFormState>(emptyNotificationRouteForm);
  const [notificationRouteLoading, setNotificationRouteLoading] = useState(false);
  const [notificationRouteSaving, setNotificationRouteSaving] = useState(false);
  const [notificationRouteError, setNotificationRouteError] = useState<string | null>(null);

  const loadFolderConfigRepository = useCallback(
    async (folderPath: string, opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setConfigRepoLoading(true);
        setConfigRepoError(null);
      }
      try {
        const payload = await fetchGroupConfigRepository(folderPath);
        if (!payload) {
          setConfigRepo(null);
          setConfigRepoForm(emptyConfigRepositoryForm);
          return;
        }
        const repo = normalizeConfigRepository(payload);
        setConfigRepo(repo);
        setConfigRepoForm(repo ? {
          repo_url: repo.repo_url,
          branch: repo.branch || 'main',
          base_path: repo.base_path || '',
          enabled: repo.enabled,
          write_enabled: repo.write_enabled,
          write_branch: repo.write_branch || 'nopsai/ui-changes',
        } : emptyConfigRepositoryForm);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to load config repository';
        setConfigRepoError(message);
      } finally {
        if (!opts?.quiet) {
          setConfigRepoLoading(false);
        }
      }
    },
    []
  );

  const loadFolderNotificationRoute = useCallback(
    async (folderPath: string, opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setNotificationRouteLoading(true);
        setNotificationRouteError(null);
      }
      try {
        const route = normalizeNotificationRoute(
          await fetchJson<NotificationRouteRecord>(`/v1/groups/${encodeURIComponent(folderPath)}/notifications`)
        );
        setNotificationRoute(route);
        setNotificationRouteForm(notificationRouteFormFromDefinition(route.definition));
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to load notification policy';
        setNotificationRouteError(message);
      } finally {
        if (!opts?.quiet) {
          setNotificationRouteLoading(false);
        }
      }
    },
    [fetchJson]
  );

  useEffect(() => {
    if (!configRepoFolder) return undefined;
    let cancelled = false;
    setConfigRepoManageAllowed(false);
    setConfigRepoSyncAllowed(false);

    void Promise.all([
      loadFolderConfigRepository(configRepoFolder.folderPath),
      loadFolderNotificationRoute(configRepoFolder.folderPath),
      checkAccessPermission('config_repo.manage', 'folder', configRepoFolder.folderPath),
      checkAccessPermission('config_repo.sync', 'folder', configRepoFolder.folderPath),
    ]).then(([, , manageAllowed, syncAllowed]) => {
      if (cancelled) return;
      setConfigRepoManageAllowed(manageAllowed);
      setConfigRepoSyncAllowed(syncAllowed);
    });

    return () => {
      cancelled = true;
    };
  }, [checkAccessPermission, configRepoFolder, loadFolderConfigRepository, loadFolderNotificationRoute]);

  useEffect(() => {
    if (!configRepoFolder || configRepo?.last_sync_status !== 'running') return undefined;
    const handle = window.setInterval(() => {
      void loadFolderConfigRepository(configRepoFolder.folderPath, { quiet: true });
    }, 3000);
    return () => window.clearInterval(handle);
  }, [configRepo?.last_sync_status, configRepoFolder, loadFolderConfigRepository]);

  const openFolderConfigRepository = useCallback(
    (group: Group) => {
      const folderPath = buildGroupPath(group.id, groups).map(item => item.name).join('/');
      if (!folderPath) return;
      setConfigRepoFolder({ group, folderPath });
      setConfigRepo(null);
      setConfigRepoForm(emptyConfigRepositoryForm);
      setConfigRepoError(null);
      setNotificationRoute(null);
      setNotificationRouteForm(emptyNotificationRouteForm);
      setNotificationRouteError(null);
      setConfigRepoDriftOpen(false);
      setConfigRepoDrift(null);
      setConfigRepoDriftError(null);
      setConfigRepoPushResult(null);
      setConfigRepoManageAllowed(false);
      setConfigRepoSyncAllowed(false);
    },
    [groups]
  );

  const closeFolderConfigRepository = useCallback(() => {
    setConfigRepoFolder(null);
    setConfigRepo(null);
    setConfigRepoForm(emptyConfigRepositoryForm);
    setConfigRepoError(null);
    setNotificationRoute(null);
    setNotificationRouteForm(emptyNotificationRouteForm);
    setNotificationRouteError(null);
    setConfigRepoDriftOpen(false);
    setConfigRepoDrift(null);
    setConfigRepoDriftError(null);
    setConfigRepoPushResult(null);
    setConfigRepoSaving(false);
    setConfigRepoSyncing(false);
    setConfigRepoDriftLoading(false);
    setConfigRepoPushing(false);
    setNotificationRouteSaving(false);
    setNotificationRouteLoading(false);
  }, []);

  const saveFolderConfigRepository = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || configRepoSaving) return;
    const repoURL = configRepoForm.repo_url.trim();
    if (!repoURL) {
      setConfigRepoError('Repository URL is required.');
      return;
    }
    setConfigRepoSaving(true);
    setConfigRepoError(null);
    try {
      const repo = await fetchJson<PipelineRunsConfigRepository>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          repo_url: repoURL,
          branch: configRepoForm.branch.trim() || 'main',
          base_path: configRepoForm.base_path.trim(),
          enabled: Boolean(configRepoForm.enabled),
          write_enabled: Boolean(configRepoForm.write_enabled),
          write_branch: configRepoForm.write_branch.trim(),
        }),
      });
      const normalized = normalizeConfigRepository(repo);
      setConfigRepo(normalized);
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
      if (normalized) {
        setConfigRepoForm({
          repo_url: normalized.repo_url,
          branch: normalized.branch || 'main',
          base_path: normalized.base_path || '',
          enabled: normalized.enabled,
          write_enabled: normalized.write_enabled,
          write_branch: normalized.write_branch || 'nopsai/ui-changes',
        });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save config repository';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSaving(false);
    }
  }, [configRepoFolder, configRepoForm, configRepoManageAllowed, configRepoSaving, fetchJson]);

  const deleteFolderConfigRepository = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || configRepoSaving) return;
    if (!window.confirm('Remove the config repository from this group? Synced resources will remain available.')) return;
    setConfigRepoSaving(true);
    setConfigRepoError(null);
    try {
      await fetchJson<void>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo`, { method: 'DELETE' });
      setConfigRepo(null);
      setConfigRepoForm(emptyConfigRepositoryForm);
      setConfigRepoDriftOpen(false);
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to remove config repository';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSaving(false);
    }
  }, [configRepoFolder, configRepoManageAllowed, configRepoSaving, fetchJson]);

  const syncFolderConfigRepository = useCallback(async () => {
    if (!configRepoFolder || !configRepoSyncAllowed || configRepoSyncing || configRepo?.last_sync_status === 'running') return;
    setConfigRepoSyncing(true);
    setConfigRepoError(null);
    try {
      await fetchJson(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo/sync`, { method: 'POST' });
      setConfigRepo(prev => prev ? {
        ...prev,
        last_sync_status: 'running',
        last_sync_message: 'Configuration synchronization started.',
        last_sync_started_at: new Date().toISOString(),
        last_sync_completed_at: undefined,
      } : prev);
      window.setTimeout(() => {
        void loadFolderConfigRepository(configRepoFolder.folderPath, { quiet: true });
      }, 1000);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to start config repository sync';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSyncing(false);
    }
  }, [configRepo?.last_sync_status, configRepoFolder, configRepoSyncAllowed, configRepoSyncing, fetchJson, loadFolderConfigRepository]);

  const checkFolderConfigRepositoryDrift = useCallback(async () => {
    if (!configRepoFolder || configRepoDriftLoading) return;
    setConfigRepoDriftOpen(true);
    setConfigRepoDriftLoading(true);
    setConfigRepoDriftError(null);
    setConfigRepoPushResult(null);
    try {
      const payload = await fetchJson<ConfigRepositoryDriftResponse>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo/drift`);
      setConfigRepoDrift(payload);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to check config repository drift';
      setConfigRepoDriftError(message);
    } finally {
      setConfigRepoDriftLoading(false);
    }
  }, [configRepoDriftLoading, configRepoFolder, fetchJson]);

  const pushFolderConfigRepositoryDrift = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || configRepoPushing) return;
    const files = buildConfigRepositoryWriteFiles(configRepoDrift);
    if (!configRepoDrift || files.length === 0) return;
    if (!configRepoDrift.can_push) {
      setConfigRepoDriftError('Enable Git push and set a push branch before committing changes.');
      return;
    }
    setConfigRepoPushing(true);
    setConfigRepoDriftError(null);
    try {
      const result = await fetchJson<ConfigRepositoryCommitResponse>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo/write`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: configRepoDrift.push_message || 'Update Nopsai config',
          files,
        }),
      });
      setConfigRepoPushResult(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to push config repository changes';
      setConfigRepoDriftError(message);
    } finally {
      setConfigRepoPushing(false);
    }
  }, [configRepoDrift, configRepoFolder, configRepoManageAllowed, configRepoPushing, fetchJson]);

  const saveFolderNotificationRoute = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || notificationRouteSaving || notificationRoute?.managed_by_config_repo) return;
    setNotificationRouteSaving(true);
    setNotificationRouteError(null);
    try {
      const route = normalizeNotificationRoute(
        await fetchJson<NotificationRouteRecord>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/notifications`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(notificationRoutePayloadFromForm(notificationRouteForm)),
        })
      );
      setNotificationRoute(route);
      setNotificationRouteForm(notificationRouteFormFromDefinition(route.definition));
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save notification policy';
      setNotificationRouteError(message);
    } finally {
      setNotificationRouteSaving(false);
    }
  }, [
    configRepoFolder,
    configRepoManageAllowed,
    fetchJson,
    notificationRoute?.managed_by_config_repo,
    notificationRouteForm,
    notificationRouteSaving,
  ]);

  const deleteFolderNotificationRoute = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || notificationRouteSaving || notificationRoute?.managed_by_config_repo) return;
    if (!notificationRoute?.id) return;
    if (!window.confirm('Remove the notification policy from this group?')) return;
    setNotificationRouteSaving(true);
    setNotificationRouteError(null);
    try {
      await fetchJson<void>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/notifications`, { method: 'DELETE' });
      const definition = defaultNotificationRouteDefinition();
      setNotificationRoute({
        group_path: configRepoFolder.folderPath,
        definition,
        source: 'database',
        managed_by_config_repo: false,
      });
      setNotificationRouteForm(notificationRouteFormFromDefinition(definition));
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to remove notification policy';
      setNotificationRouteError(message);
    } finally {
      setNotificationRouteSaving(false);
    }
  }, [configRepoFolder, configRepoManageAllowed, fetchJson, notificationRoute?.id, notificationRoute?.managed_by_config_repo, notificationRouteSaving]);

  return {
    configRepoFolder,
    configRepo,
    configRepoForm,
    configRepoLoading,
    configRepoSaving,
    configRepoSyncing,
    configRepoError,
    configRepoDriftOpen,
    configRepoDrift,
    configRepoDriftLoading,
    configRepoDriftError,
    configRepoPushing,
    configRepoPushResult,
    configRepoManageAllowed,
    configRepoSyncAllowed,
    notificationRoute,
    notificationRouteForm,
    notificationRouteLoading,
    notificationRouteSaving,
    notificationRouteError,
    setConfigRepoForm,
    setNotificationRouteForm,
    setConfigRepoDriftOpen,
    openFolderConfigRepository,
    closeFolderConfigRepository,
    saveFolderConfigRepository,
    deleteFolderConfigRepository,
    syncFolderConfigRepository,
    checkFolderConfigRepositoryDrift,
    pushFolderConfigRepositoryDrift,
    saveFolderNotificationRoute,
    deleteFolderNotificationRoute,
  };
}

export type FolderConfigRepositoryController = ReturnType<typeof useFolderConfigRepositoryController>;
