import { useCallback, useEffect, useState } from 'react';
import { teamPathForURL, type Team } from '../../../lib/teamModels';
import { normalizeConfigRepositoryProvider, type ConfigRepositoryProvider } from '../../../lib/configRepositoryProviders.js';
import { fetchTeamConfigRepository } from '../api';
import {
  createEmptyNotificationRouteForm,
  defaultNotificationRouteDefinition,
  normalizeNotificationRouteRecord,
  notificationRouteFormFromDefinition,
  notificationRoutePayloadFromForm,
  type NotificationRouteFormState,
  type NotificationRouteRecord,
} from '../notificationRoutes';
import {
  buildConfigRepositoryWriteFiles,
  type ConfigRepositoryCommitResponse,
  type ConfigRepositoryDriftResponse,
} from '../../../lib/configRepositoryDrift';
import { validateTeamConfigRepositoryDraft, type BackendValidationIssue } from '../../validation/api';

export type PipelineRunsConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  provider: ConfigRepositoryProvider;
  repo_url: string;
  branch: string;
  base_path: string;
  credential_ref: string;
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
  provider: ConfigRepositoryProvider;
  repo_url: string;
  branch: string;
  base_path: string;
  credential_ref: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
};

type TeamConfigRepositorySelection = {
  team: Team;
  teamPath: string;
};

type TeamConfigRepositoryInitialTab = 'sync' | 'notifications';

type FetchJson = <T>(path: string, options?: RequestInit) => Promise<T>;

type UseTeamConfigRepositoryControllerOptions = {
  teams: Team[];
  fetchJson: FetchJson;
  checkAccessPermission: (action: string, resourceType: string, resourceID: string) => Promise<boolean>;
};

function formatBackendValidationIssue(issue?: BackendValidationIssue) {
  if (!issue) return 'Config repository validation failed.';
  const file = issue.file ? `${issue.file}: ` : '';
  const line = issue.line ? ` (line ${issue.line})` : '';
  return `${file}${issue.message}${line}`;
}

const emptyConfigRepositoryForm: PipelineRunsConfigRepositoryFormState = {
  provider: 'github',
  repo_url: '',
  branch: 'main',
  base_path: '',
  credential_ref: '',
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
    provider: normalizeConfigRepositoryProvider(record.provider),
    repo_url: typeof record.repo_url === 'string' ? record.repo_url : '',
    branch: typeof record.branch === 'string' && record.branch.trim() ? record.branch : 'main',
    base_path: typeof record.base_path === 'string' ? record.base_path : '',
    credential_ref: typeof record.credential_ref === 'string' ? record.credential_ref : '',
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

export function useTeamConfigRepositoryController({
  teams,
  fetchJson,
  checkAccessPermission,
}: UseTeamConfigRepositoryControllerOptions) {
  const [configRepoTeam, setConfigRepoTeam] = useState<TeamConfigRepositorySelection | null>(null);
  const [configRepoInitialTab, setConfigRepoInitialTab] = useState<TeamConfigRepositoryInitialTab>('sync');
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

  const loadTeamConfigRepository = useCallback(
    async (teamPath: string, opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setConfigRepoLoading(true);
        setConfigRepoError(null);
      }
      try {
        const payload = await fetchTeamConfigRepository(teamPath);
        if (!payload) {
          setConfigRepo(null);
          setConfigRepoForm(emptyConfigRepositoryForm);
          return;
        }
        const repo = normalizeConfigRepository(payload);
        setConfigRepo(repo);
        setConfigRepoForm(repo ? {
          repo_url: repo.repo_url,
          provider: repo.provider,
          branch: repo.branch || 'main',
          base_path: repo.base_path || '',
          credential_ref: repo.credential_ref || '',
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

  const loadTeamNotificationRoute = useCallback(
    async (teamPath: string, opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setNotificationRouteLoading(true);
        setNotificationRouteError(null);
      }
      try {
        const route = normalizeNotificationRoute(
          await fetchJson<NotificationRouteRecord>(`/v1/teams/${encodeURIComponent(teamPath)}/notifications`)
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
    if (!configRepoTeam) return undefined;
    let cancelled = false;
    setConfigRepoManageAllowed(false);
    setConfigRepoSyncAllowed(false);

    void Promise.all([
      loadTeamConfigRepository(configRepoTeam.teamPath),
      loadTeamNotificationRoute(configRepoTeam.teamPath),
      checkAccessPermission('config_repo.manage', 'team', configRepoTeam.teamPath),
      checkAccessPermission('config_repo.sync', 'team', configRepoTeam.teamPath),
    ]).then(([, , manageAllowed, syncAllowed]) => {
      if (cancelled) return;
      setConfigRepoManageAllowed(manageAllowed);
      setConfigRepoSyncAllowed(syncAllowed);
    });

    return () => {
      cancelled = true;
    };
  }, [checkAccessPermission, configRepoTeam, loadTeamConfigRepository, loadTeamNotificationRoute]);

  useEffect(() => {
    if (!configRepoTeam || configRepo?.last_sync_status !== 'running') return undefined;
    const handle = window.setInterval(() => {
      void loadTeamConfigRepository(configRepoTeam.teamPath, { quiet: true });
    }, 3000);
    return () => window.clearInterval(handle);
  }, [configRepo?.last_sync_status, configRepoTeam, loadTeamConfigRepository]);

  const openTeamConfigRepository = useCallback(
    (team: Team, initialTab: TeamConfigRepositoryInitialTab = 'sync') => {
      const teamPath = teamPathForURL(team, teams);
      if (!teamPath) return;
      setConfigRepoTeam({ team, teamPath });
      setConfigRepoInitialTab(initialTab);
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
    [teams]
  );

  const closeTeamConfigRepository = useCallback(() => {
    setConfigRepoTeam(null);
    setConfigRepoInitialTab('sync');
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

  const saveTeamConfigRepository = useCallback(async () => {
    if (!configRepoTeam || !configRepoManageAllowed || configRepoSaving) return;
    const repoURL = configRepoForm.repo_url.trim();
    if (!repoURL) {
      setConfigRepoError('Repository URL is required.');
      return;
    }
    if (configRepoForm.provider !== 'github' && !configRepoForm.credential_ref.trim()) {
      setConfigRepoError('Credential reference is required for this Git provider.');
      return;
    }
    setConfigRepoSaving(true);
    setConfigRepoError(null);
    try {
      const repo = await fetchJson<PipelineRunsConfigRepository>(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/config-repository`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          repo_url: repoURL,
          provider: configRepoForm.provider,
          branch: configRepoForm.branch.trim() || 'main',
          base_path: configRepoForm.base_path.trim(),
          credential_ref: configRepoForm.credential_ref.trim(),
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
          provider: normalized.provider,
          branch: normalized.branch || 'main',
          base_path: normalized.base_path || '',
          credential_ref: normalized.credential_ref || '',
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
  }, [configRepoTeam, configRepoForm, configRepoManageAllowed, configRepoSaving, fetchJson]);

  const deleteTeamConfigRepository = useCallback(async () => {
    if (!configRepoTeam || !configRepoManageAllowed || configRepoSaving) return;
    if (!window.confirm('Remove the config repository from this team? Synced resources will remain available.')) return;
    setConfigRepoSaving(true);
    setConfigRepoError(null);
    try {
      await fetchJson<void>(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/config-repository`, { method: 'DELETE' });
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
  }, [configRepoTeam, configRepoManageAllowed, configRepoSaving, fetchJson]);

  const syncTeamConfigRepository = useCallback(async () => {
    if (!configRepoTeam || !configRepoSyncAllowed || configRepoSyncing || configRepo?.last_sync_status === 'running') return;
    setConfigRepoSyncing(true);
    setConfigRepoError(null);
    try {
      await fetchJson(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/config-repository/sync`, { method: 'POST' });
      setConfigRepo(prev => prev ? {
        ...prev,
        last_sync_status: 'running',
        last_sync_message: 'Configuration synchronization started.',
        last_sync_started_at: new Date().toISOString(),
        last_sync_completed_at: undefined,
      } : prev);
      window.setTimeout(() => {
        void loadTeamConfigRepository(configRepoTeam.teamPath, { quiet: true });
      }, 1000);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to start config repository sync';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSyncing(false);
    }
  }, [configRepo?.last_sync_status, configRepoTeam, configRepoSyncAllowed, configRepoSyncing, fetchJson, loadTeamConfigRepository]);

  const checkTeamConfigRepositoryDrift = useCallback(async () => {
    if (!configRepoTeam || configRepoDriftLoading) return;
    setConfigRepoDriftOpen(true);
    setConfigRepoDriftLoading(true);
    setConfigRepoDriftError(null);
    setConfigRepoPushResult(null);
    try {
      const payload = await fetchJson<ConfigRepositoryDriftResponse>(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/config-repository/drift`);
      setConfigRepoDrift(payload);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to check config repository drift';
      setConfigRepoDriftError(message);
    } finally {
      setConfigRepoDriftLoading(false);
    }
  }, [configRepoDriftLoading, configRepoTeam, fetchJson]);

  const pushTeamConfigRepositoryDrift = useCallback(async () => {
    if (!configRepoTeam || !configRepoManageAllowed || configRepoPushing) return;
    const files = buildConfigRepositoryWriteFiles(configRepoDrift);
    if (!configRepoDrift || files.length === 0) return;
    if (!configRepoDrift.can_push) {
      setConfigRepoDriftError('Enable Git push and set a push branch before committing changes.');
      return;
    }
    setConfigRepoPushing(true);
    setConfigRepoDriftError(null);
    try {
      const validation = await validateTeamConfigRepositoryDraft(configRepoTeam.teamPath, {
        base_path: configRepo?.base_path || configRepoForm.base_path || '',
        files: files.map(file => ({
          path: file.path,
          content: file.content ?? '',
          delete: Boolean(file.delete),
        })),
      });
      if (!validation.valid) {
        setConfigRepoDriftError(formatBackendValidationIssue(validation.errors[0]));
        return;
      }
      const result = await fetchJson<ConfigRepositoryCommitResponse>(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/config-repository/write`, {
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
  }, [configRepo?.base_path, configRepoDrift, configRepoForm.base_path, configRepoTeam, configRepoManageAllowed, configRepoPushing, fetchJson]);

  const saveTeamNotificationRoute = useCallback(async () => {
    if (!configRepoTeam || !configRepoManageAllowed || notificationRouteSaving || notificationRoute?.managed_by_config_repo) return;
    setNotificationRouteSaving(true);
    setNotificationRouteError(null);
    try {
      const route = normalizeNotificationRoute(
        await fetchJson<NotificationRouteRecord>(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/notifications`, {
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
    configRepoTeam,
    configRepoManageAllowed,
    fetchJson,
    notificationRoute?.managed_by_config_repo,
    notificationRouteForm,
    notificationRouteSaving,
  ]);

  const deleteTeamNotificationRoute = useCallback(async () => {
    if (!configRepoTeam || !configRepoManageAllowed || notificationRouteSaving || notificationRoute?.managed_by_config_repo) return;
    if (!notificationRoute?.id) return;
    if (!window.confirm('Remove the notification policy from this team?')) return;
    setNotificationRouteSaving(true);
    setNotificationRouteError(null);
    try {
      await fetchJson<void>(`/v1/teams/${encodeURIComponent(configRepoTeam.teamPath)}/notifications`, { method: 'DELETE' });
      const definition = defaultNotificationRouteDefinition();
      setNotificationRoute({
        team_path: configRepoTeam.teamPath,
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
  }, [configRepoTeam, configRepoManageAllowed, fetchJson, notificationRoute?.id, notificationRoute?.managed_by_config_repo, notificationRouteSaving]);

  return {
    configRepoTeam,
    configRepoInitialTab,
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
    openTeamConfigRepository,
    closeTeamConfigRepository,
    saveTeamConfigRepository,
    deleteTeamConfigRepository,
    syncTeamConfigRepository,
    checkTeamConfigRepositoryDrift,
    pushTeamConfigRepositoryDrift,
    saveTeamNotificationRoute,
    deleteTeamNotificationRoute,
  };
}

export type TeamConfigRepositoryController = ReturnType<typeof useTeamConfigRepositoryController>;
