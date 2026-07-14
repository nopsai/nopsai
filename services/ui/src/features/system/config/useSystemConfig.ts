import { useCallback, useEffect, useRef, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import {
  buildConfigRepositoryWriteFiles,
  type ConfigRepositoryCommitResponse,
  type ConfigRepositoryDriftResponse,
} from '../../../lib/configRepositoryDrift';
import {
  deleteGlobalConfigRepository,
  fetchGlobalConfigRepository,
  fetchGlobalConfigRepositoryDrift,
  fetchMailSettings,
  fetchRuntimeConfig,
  pushGlobalConfigRepositoryDrift,
  saveGlobalConfigRepository,
  saveMailSettings,
  saveRuntimeConfig,
  sendMailSettingsTest,
  syncGlobalConfigRepository,
} from './api';
import {
  configRepositoryFormFromRecord,
  emptyConfigRepositoryForm,
  emptyNotificationMailSettingsForm,
  initialConfig,
  mailSettingsFormFromRecord,
  type ConfigFieldMetadata,
  type ConfigFormState,
  type ConfigRepository,
  type ConfigRepositoryFormState,
  type NotificationMailSettingsFormState,
  type NotificationMailSettingsRecord,
} from './model';

type ToastTone = 'success' | 'error' | 'info';

type UseSystemConfigOptions = {
  runtimeConfigEnabled: boolean;
  mailSettingsEnabled: boolean;
  globalConfigRepoEnabled: boolean;
  canManageRuntimeConfig: boolean;
  canViewGlobalConfigRepo: boolean;
  canManageGlobalConfigRepo: boolean;
  addToast: (message: string, tone?: ToastTone) => void;
};

type SystemConfigPanelProps = {
  config: ConfigFormState;
  envFilePath: string;
  fieldMetadata: Record<string, ConfigFieldMetadata>;
  configError: string | null;
  configLoading: boolean;
  saving: boolean;
  globalConfigRepo: ConfigRepository | null;
  globalConfigRepoForm: ConfigRepositoryFormState;
  globalConfigRepoLoading: boolean;
  globalConfigRepoSaving: boolean;
  globalConfigRepoSyncing: boolean;
  globalConfigRepoError: string | null;
  mailSettings: NotificationMailSettingsRecord | null;
  mailSettingsForm: NotificationMailSettingsFormState;
  mailSettingsLoading: boolean;
  mailSettingsSaving: boolean;
  mailSettingsTesting: boolean;
  mailSettingsError: string | null;
  onChange: Dispatch<SetStateAction<ConfigFormState>>;
  onReload: () => Promise<void>;
  onSave: () => Promise<void>;
  onMailSettingsChange: Dispatch<SetStateAction<NotificationMailSettingsFormState>>;
  onSaveMailSettings: () => Promise<void>;
  onTestMailSettings: () => Promise<void>;
  onGlobalConfigRepoChange: Dispatch<SetStateAction<ConfigRepositoryFormState>>;
  onSaveGlobalConfigRepo: () => Promise<void>;
  onDeleteGlobalConfigRepo: () => Promise<void>;
  onSyncGlobalConfigRepo: () => Promise<void>;
  onCheckGlobalConfigRepoDrift: () => Promise<void>;
  globalConfigRepoDriftLoading: boolean;
  globalConfigRepoPushing: boolean;
};

type GlobalConfigRepositoryDriftModalProps = {
  title: string;
  drift: ConfigRepositoryDriftResponse | null;
  loading: boolean;
  error: string | null;
  pushing: boolean;
  pushResult: ConfigRepositoryCommitResponse | null;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onPush: () => Promise<void>;
};

export function useSystemConfig({
  runtimeConfigEnabled,
  mailSettingsEnabled,
  globalConfigRepoEnabled,
  canManageRuntimeConfig,
  canViewGlobalConfigRepo,
  canManageGlobalConfigRepo,
  addToast,
}: UseSystemConfigOptions): {
  config: ConfigFormState;
  globalConfigRepoDriftOpen: boolean;
  globalConfigRepoDriftCanPush: boolean;
  panelProps: SystemConfigPanelProps;
  driftModalProps: GlobalConfigRepositoryDriftModalProps;
} {
  const isMountedRef = useRef(true);
  const [config, setConfig] = useState<ConfigFormState>(initialConfig);
  const [envFilePath, setEnvFilePath] = useState('');
  const [fieldMetadata, setFieldMetadata] = useState<Record<string, ConfigFieldMetadata>>({});
  const [configLoading, setConfigLoading] = useState(runtimeConfigEnabled);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [globalConfigRepo, setGlobalConfigRepo] = useState<ConfigRepository | null>(null);
  const [globalConfigRepoForm, setGlobalConfigRepoForm] = useState<ConfigRepositoryFormState>(emptyConfigRepositoryForm);
  const [globalConfigRepoLoading, setGlobalConfigRepoLoading] = useState(false);
  const [globalConfigRepoSaving, setGlobalConfigRepoSaving] = useState(false);
  const [globalConfigRepoSyncing, setGlobalConfigRepoSyncing] = useState(false);
  const [globalConfigRepoError, setGlobalConfigRepoError] = useState<string | null>(null);
  const [globalConfigRepoDriftOpen, setGlobalConfigRepoDriftOpen] = useState(false);
  const [globalConfigRepoDrift, setGlobalConfigRepoDrift] = useState<ConfigRepositoryDriftResponse | null>(null);
  const [globalConfigRepoDriftLoading, setGlobalConfigRepoDriftLoading] = useState(false);
  const [globalConfigRepoDriftError, setGlobalConfigRepoDriftError] = useState<string | null>(null);
  const [globalConfigRepoPushing, setGlobalConfigRepoPushing] = useState(false);
  const [globalConfigRepoPushResult, setGlobalConfigRepoPushResult] = useState<ConfigRepositoryCommitResponse | null>(null);
  const [mailSettings, setMailSettings] = useState<NotificationMailSettingsRecord | null>(null);
  const [mailSettingsForm, setMailSettingsForm] = useState<NotificationMailSettingsFormState>(emptyNotificationMailSettingsForm);
  const [mailSettingsLoading, setMailSettingsLoading] = useState(false);
  const [mailSettingsSaving, setMailSettingsSaving] = useState(false);
  const [mailSettingsTesting, setMailSettingsTesting] = useState(false);
  const [mailSettingsError, setMailSettingsError] = useState<string | null>(null);

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const loadSystemConfig = useCallback(async () => {
    setConfigError(null);
    setConfigLoading(true);
    try {
      const payload = await fetchRuntimeConfig();
      if (!isMountedRef.current) return;
      setConfig(payload.config);
      setEnvFilePath(payload.envFilePath);
      setFieldMetadata(payload.fieldMetadata);
    } catch (error) {
      console.error('Failed to load system config', error);
      if (!isMountedRef.current) return;
      setConfigError(error instanceof Error ? error.message : 'Unable to load system config');
    } finally {
      if (isMountedRef.current) {
        setConfigLoading(false);
      }
    }
  }, []);

  const loadMailSettings = useCallback(async () => {
    setMailSettingsLoading(true);
    setMailSettingsError(null);
    try {
      const settings = await fetchMailSettings();
      if (!isMountedRef.current) return;
      setMailSettings(settings);
      setMailSettingsForm(mailSettingsFormFromRecord(settings));
    } catch (error) {
      console.error('Failed to load mail notification settings', error);
      if (!isMountedRef.current) return;
      setMailSettingsError(error instanceof Error ? error.message : 'Unable to load mail notification settings');
    } finally {
      if (isMountedRef.current) {
        setMailSettingsLoading(false);
      }
    }
  }, []);

  const loadGlobalConfigRepository = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) {
      setGlobalConfigRepoLoading(true);
      setGlobalConfigRepoError(null);
    }
    try {
      const repo = await fetchGlobalConfigRepository();
      if (!isMountedRef.current) return;
      setGlobalConfigRepo(repo);
      setGlobalConfigRepoForm(configRepositoryFormFromRecord(repo));
    } catch (error) {
      console.error('Failed to load global config repository', error);
      if (!isMountedRef.current) return;
      setGlobalConfigRepoError(error instanceof Error ? error.message : 'Unable to load global config repository');
    } finally {
      if (isMountedRef.current && !opts?.quiet) {
        setGlobalConfigRepoLoading(false);
      }
    }
  }, []);

  const saveConfig = useCallback(async () => {
    if (saving) return;
    setSaving(true);
    setConfigError(null);
    try {
      const payload = await saveRuntimeConfig(config);
      if (!isMountedRef.current) return;
      setConfig(payload.config);
      setEnvFilePath(payload.envFilePath);
      setFieldMetadata(payload.fieldMetadata);
      addToast('System settings saved.', 'success');
    } catch (error) {
      console.error('Failed to save system config', error);
      addToast('Failed to save settings.', 'error');
      if (isMountedRef.current) {
        setConfigError(error instanceof Error ? error.message : 'Unable to save system config');
      }
    } finally {
      if (isMountedRef.current) {
        setSaving(false);
      }
    }
  }, [addToast, config, saving]);

  const saveMail = useCallback(async () => {
    if (mailSettingsSaving || !canManageRuntimeConfig || mailSettings?.managed_by_config_repo) return;
    setMailSettingsSaving(true);
    setMailSettingsError(null);
    try {
      const settings = await saveMailSettings(mailSettingsForm);
      if (!isMountedRef.current) return;
      setMailSettings(settings);
      setMailSettingsForm(mailSettingsFormFromRecord(settings, mailSettingsForm.test_to));
      setGlobalConfigRepoDrift(null);
      setGlobalConfigRepoPushResult(null);
      addToast('Mail notification settings saved.', 'success');
    } catch (error) {
      console.error('Failed to save mail notification settings', error);
      const message = error instanceof Error ? error.message : 'Unable to save mail notification settings';
      setMailSettingsError(message);
      addToast('Failed to save mail notification settings.', 'error');
    } finally {
      if (isMountedRef.current) {
        setMailSettingsSaving(false);
      }
    }
  }, [addToast, canManageRuntimeConfig, mailSettings?.managed_by_config_repo, mailSettingsForm, mailSettingsSaving]);

  const testMail = useCallback(async () => {
    if (mailSettingsTesting || !canManageRuntimeConfig) return;
    const to = mailSettingsForm.test_to.trim();
    if (!to) {
      setMailSettingsError('Test recipient is required.');
      return;
    }
    setMailSettingsTesting(true);
    setMailSettingsError(null);
    try {
      await sendMailSettingsTest(to);
      addToast('Mail test sent.', 'success');
    } catch (error) {
      console.error('Failed to send mail notification test', error);
      const message = error instanceof Error ? error.message : 'Unable to send mail test';
      setMailSettingsError(message);
      addToast('Failed to send mail test.', 'error');
    } finally {
      if (isMountedRef.current) {
        setMailSettingsTesting(false);
      }
    }
  }, [addToast, canManageRuntimeConfig, mailSettingsForm.test_to, mailSettingsTesting]);

  const saveGlobalRepo = useCallback(async () => {
    if (globalConfigRepoSaving || !canManageGlobalConfigRepo) return;
    const repoURL = globalConfigRepoForm.repo_url.trim();
    if (!repoURL) {
      setGlobalConfigRepoError('Repository URL is required.');
      return;
    }
    if (globalConfigRepoForm.provider !== 'github' && !globalConfigRepoForm.credential_ref.trim()) {
      setGlobalConfigRepoError('Credential reference is required for this Git provider.');
      return;
    }
    setGlobalConfigRepoSaving(true);
    setGlobalConfigRepoError(null);
    try {
      const repo = await saveGlobalConfigRepository(globalConfigRepoForm);
      if (!isMountedRef.current) return;
      setGlobalConfigRepo(repo);
      setGlobalConfigRepoForm(configRepositoryFormFromRecord(repo));
      addToast('Global config repository saved.', 'success');
    } catch (error) {
      console.error('Failed to save global config repository', error);
      const message = error instanceof Error ? error.message : 'Unable to save global config repository';
      setGlobalConfigRepoError(message);
      addToast('Failed to save global config repository.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoSaving(false);
      }
    }
  }, [addToast, canManageGlobalConfigRepo, globalConfigRepoForm, globalConfigRepoSaving]);

  const deleteGlobalRepo = useCallback(async () => {
    if (globalConfigRepoSaving || !canManageGlobalConfigRepo || !globalConfigRepo) return;
    if (!window.confirm('Remove the global config repository? Synced resources will remain available.')) return;
    setGlobalConfigRepoSaving(true);
    setGlobalConfigRepoError(null);
    try {
      await deleteGlobalConfigRepository();
      if (!isMountedRef.current) return;
      setGlobalConfigRepo(null);
      setGlobalConfigRepoForm(emptyConfigRepositoryForm);
      addToast('Global config repository removed.', 'success');
    } catch (error) {
      console.error('Failed to remove global config repository', error);
      const message = error instanceof Error ? error.message : 'Unable to remove global config repository';
      setGlobalConfigRepoError(message);
      addToast('Failed to remove global config repository.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoSaving(false);
      }
    }
  }, [addToast, canManageGlobalConfigRepo, globalConfigRepo, globalConfigRepoSaving]);

  const syncGlobalRepo = useCallback(async () => {
    if (!canManageGlobalConfigRepo || globalConfigRepoSyncing || globalConfigRepo?.last_sync_status === 'running') return;
    setGlobalConfigRepoSyncing(true);
    setGlobalConfigRepoError(null);
    try {
      await syncGlobalConfigRepository();
      if (!isMountedRef.current) return;
      setGlobalConfigRepo(prev =>
        prev
          ? {
              ...prev,
              last_sync_status: 'running',
              last_sync_message: 'Configuration synchronization started.',
              last_sync_started_at: new Date().toISOString(),
              last_sync_completed_at: undefined,
            }
          : prev
      );
      window.setTimeout(() => {
        void loadGlobalConfigRepository({ quiet: true });
      }, 1000);
      addToast('Global config repository sync started.', 'success');
    } catch (error) {
      console.error('Failed to start global config repository sync', error);
      const message = error instanceof Error ? error.message : 'Unable to start global config repository sync';
      setGlobalConfigRepoError(message);
      addToast('Failed to start global config repository sync.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoSyncing(false);
      }
    }
  }, [addToast, canManageGlobalConfigRepo, globalConfigRepo?.last_sync_status, globalConfigRepoSyncing, loadGlobalConfigRepository]);

  const checkGlobalRepoDrift = useCallback(async () => {
    if (!canViewGlobalConfigRepo || globalConfigRepoDriftLoading) return;
    setGlobalConfigRepoDriftOpen(true);
    setGlobalConfigRepoDriftLoading(true);
    setGlobalConfigRepoDriftError(null);
    setGlobalConfigRepoPushResult(null);
    try {
      const payload = await fetchGlobalConfigRepositoryDrift();
      if (!isMountedRef.current) return;
      setGlobalConfigRepoDrift(payload);
    } catch (error) {
      console.error('Failed to check global config repository drift', error);
      const message = error instanceof Error ? error.message : 'Unable to check config repository drift';
      setGlobalConfigRepoDriftError(message);
      addToast('Failed to check config repository drift.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoDriftLoading(false);
      }
    }
  }, [addToast, canViewGlobalConfigRepo, globalConfigRepoDriftLoading]);

  const pushGlobalRepoDrift = useCallback(async () => {
    if (!canManageGlobalConfigRepo || globalConfigRepoPushing) return;
    const files = buildConfigRepositoryWriteFiles(globalConfigRepoDrift);
    if (!globalConfigRepoDrift || files.length === 0) return;
    if (!globalConfigRepoDrift.can_push) {
      setGlobalConfigRepoDriftError('Enable Git push and set a push branch before committing changes.');
      return;
    }
    setGlobalConfigRepoPushing(true);
    setGlobalConfigRepoDriftError(null);
    try {
      const result = await pushGlobalConfigRepositoryDrift(
        globalConfigRepoDrift.push_message || 'Update Nopsai config',
        files
      );
      if (!isMountedRef.current) return;
      setGlobalConfigRepoPushResult(result);
      const branch = result.branch || globalConfigRepoDrift.push_branch || globalConfigRepo?.write_branch || 'the push branch';
      addToast(`Pushed ${files.length} file${files.length === 1 ? '' : 's'} to ${branch}.`, 'success');
    } catch (error) {
      console.error('Failed to push global config repository drift', error);
      const message = error instanceof Error ? error.message : 'Unable to push config repository changes';
      setGlobalConfigRepoDriftError(message);
      addToast('Failed to push config repository changes.', 'error');
    } finally {
      if (isMountedRef.current) {
        setGlobalConfigRepoPushing(false);
      }
    }
  }, [addToast, canManageGlobalConfigRepo, globalConfigRepo?.write_branch, globalConfigRepoDrift, globalConfigRepoPushing]);

  useEffect(() => {
    if (!runtimeConfigEnabled) {
      setConfigLoading(false);
      return;
    }
    void loadSystemConfig();
  }, [loadSystemConfig, runtimeConfigEnabled]);

  useEffect(() => {
    if (!mailSettingsEnabled) return;
    void loadMailSettings();
  }, [loadMailSettings, mailSettingsEnabled]);

  useEffect(() => {
    if (!globalConfigRepoEnabled) return;
    void loadGlobalConfigRepository();
  }, [globalConfigRepoEnabled, loadGlobalConfigRepository]);

  useEffect(() => {
    if (!globalConfigRepoEnabled || globalConfigRepo?.last_sync_status !== 'running') return undefined;
    const handle = window.setInterval(() => {
      void loadGlobalConfigRepository({ quiet: true });
    }, 3000);
    return () => window.clearInterval(handle);
  }, [globalConfigRepo?.last_sync_status, globalConfigRepoEnabled, loadGlobalConfigRepository]);

  const reloadConfigTab = useCallback(async () => {
    if (!runtimeConfigEnabled) return;
    await Promise.all([loadSystemConfig(), loadMailSettings()]);
  }, [loadMailSettings, loadSystemConfig, runtimeConfigEnabled]);

  return {
    config,
    globalConfigRepoDriftOpen,
    globalConfigRepoDriftCanPush: canManageGlobalConfigRepo && Boolean(globalConfigRepoDrift?.can_push),
    panelProps: {
      config,
      envFilePath,
      fieldMetadata,
      configError,
      configLoading,
      saving,
      globalConfigRepo,
      globalConfigRepoForm,
      globalConfigRepoLoading,
      globalConfigRepoSaving,
      globalConfigRepoSyncing,
      globalConfigRepoError,
      mailSettings,
      mailSettingsForm,
      mailSettingsLoading,
      mailSettingsSaving,
      mailSettingsTesting,
      mailSettingsError,
      onChange: setConfig,
      onReload: reloadConfigTab,
      onSave: saveConfig,
      onMailSettingsChange: setMailSettingsForm,
      onSaveMailSettings: saveMail,
      onTestMailSettings: testMail,
      onGlobalConfigRepoChange: setGlobalConfigRepoForm,
      onSaveGlobalConfigRepo: saveGlobalRepo,
      onDeleteGlobalConfigRepo: deleteGlobalRepo,
      onSyncGlobalConfigRepo: syncGlobalRepo,
      onCheckGlobalConfigRepoDrift: checkGlobalRepoDrift,
      globalConfigRepoDriftLoading,
      globalConfigRepoPushing,
    },
    driftModalProps: {
      title: 'Global config repository',
      drift: globalConfigRepoDrift,
      loading: globalConfigRepoDriftLoading,
      error: globalConfigRepoDriftError,
      pushing: globalConfigRepoPushing,
      pushResult: globalConfigRepoPushResult,
      onClose: () => setGlobalConfigRepoDriftOpen(false),
      onRefresh: checkGlobalRepoDrift,
      onPush: pushGlobalRepoDrift,
    },
  };
}
