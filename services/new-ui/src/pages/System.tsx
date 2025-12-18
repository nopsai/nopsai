import { Link, NavLink, useParams } from 'react-router-dom';
import { useCallback, useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { buildApiUrl } from '../lib/api';

type ConfigFormState = {
  config_repo_url: string;
  agent_image: string;
  docker_network_name: string;
  default_pipeline_timeout: string;
  llm_agent_timeout: string;
  auto_removal_agent_container: boolean;
  agent_nopsai_api_url: string;
  git_bot_nopsai_api_url: string;
  nopsai_git_bot_api_url: string;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

type ConfigSyncStatus = {
  status: string;
  message?: string;
  details?: Record<string, number>;
  started_at?: string;
  completed_at?: string;
};

type Runner = {
  runnerId: string;
  scopes: string[];
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  lastHeartbeatUnix: number;
  allowDispatch: boolean;
  metadata: Record<string, string>;
};

type RunnerActiveRun = {
  runId: string;
  pipeline: string;
  parentStep?: string;
  triggerId?: string;
};

type RunnerMeta = {
  connectionId: string;
  hostname: string;
  network: string;
  activeRuns: RunnerActiveRun[];
};

const initialConfig: ConfigFormState = {
  config_repo_url: '',
  agent_image: '',
  docker_network_name: '',
  default_pipeline_timeout: '',
  llm_agent_timeout: '',
  auto_removal_agent_container: true,
  agent_nopsai_api_url: '',
  git_bot_nopsai_api_url: '',
  nopsai_git_bot_api_url: '',
};

const POLL_INTERVAL_MS = 5000;
const STALE_THRESHOLD_MS = 30_000;
const MAX_VISIBLE_ACTIVE_RUNS = 3;

function SystemPage() {
  const params = useParams<{ tab?: string }>();
  const activeTab = params.tab === 'dispatcher' ? 'dispatcher' : 'config';

  const isMountedRef = useRef(true);

  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const [config, setConfig] = useState<ConfigFormState>(initialConfig);
  const [envFilePath, setEnvFilePath] = useState('');
  const [configLoading, setConfigLoading] = useState(true);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [syncStatus, setSyncStatus] = useState<ConfigSyncStatus | null>(null);
  const [syncBusy, setSyncBusy] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);

  const [dispatcherLoading, setDispatcherLoading] = useState(false);
  const [dispatcherError, setDispatcherError] = useState<string | null>(null);
  const [dispatcherStatus, setDispatcherStatus] = useState<{
    queuedJobs: number;
    runners: Runner[];
    routing: Record<string, string[]>;
    fetchedAt: number;
  } | null>(null);

  const [runnerActionPending, setRunnerActionPending] = useState<Set<string>>(new Set());

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const addToast = useCallback((message: string, tone: ToastMessage['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const fetchJson = useCallback(async (path: string, init?: RequestInit): Promise<unknown> => {
    const response = await fetch(buildApiUrl(path), init);
    if (response.status === 204) return null;
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed (${response.status})`);
    }
    const contentType = response.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
      return response.json();
    }
    return response.text();
  }, []);

  const applySystemConfigResponse = useCallback((payload: unknown) => {
    const record = asRecord(payload);
    if (!record) throw new Error('Unexpected system config response');

    const nextConfig: ConfigFormState = {
      config_repo_url: readString(record.config_repo_url),
      agent_image: readString(record.agent_image),
      docker_network_name: readString(record.docker_network_name),
      default_pipeline_timeout: readString(record.default_pipeline_timeout),
      llm_agent_timeout: readString(record.llm_agent_timeout),
      auto_removal_agent_container: Boolean(record.auto_removal_agent_container),
      agent_nopsai_api_url: readString(record.agent_nopsai_api_url),
      git_bot_nopsai_api_url: readString(record.git_bot_nopsai_api_url),
      nopsai_git_bot_api_url: readString(record.nopsai_git_bot_api_url),
    };
    setConfig(nextConfig);
    setEnvFilePath(readString(record.env_file_path));

    const nextSyncStatus = normalizeConfigSyncStatus(record.config_sync_status);
    setSyncStatus(nextSyncStatus);
  }, []);

  const loadSystemConfig = useCallback(async () => {
    setConfigError(null);
    setConfigLoading(true);
    try {
      const payload = await fetchJson('/v1/system/config');
      applySystemConfigResponse(payload);
    } catch (error) {
      console.error('Failed to load system config', error);
      if (!isMountedRef.current) return;
      setConfigError(error instanceof Error ? error.message : 'Unable to load system config');
    } finally {
      if (isMountedRef.current) {
        setConfigLoading(false);
      }
    }
  }, [applySystemConfigResponse, fetchJson]);

  const loadSyncStatus = useCallback(
    async (opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setSyncError(null);
      }
      try {
        const payload = await fetchJson('/v1/system/config/sync');
        const nextStatus = normalizeConfigSyncStatus(payload);
        if (isMountedRef.current) {
          setSyncStatus(nextStatus);
          setSyncError(null);
        }
      } catch (error) {
        console.error('Failed to load sync status', error);
        if (!isMountedRef.current) return;
        setSyncError(error instanceof Error ? error.message : 'Unable to load sync status');
      }
    },
    [fetchJson]
  );

  const loadDispatcherStatus = useCallback(
    async (opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setDispatcherError(null);
        setDispatcherLoading(true);
      }
      try {
        const payload = await fetchJson('/v1/system/dispatcher');
        const normalized = normalizeDispatcherStatus(payload);
        if (isMountedRef.current) {
          setDispatcherStatus({ ...normalized, fetchedAt: Date.now() });
          setDispatcherError(null);
        }
      } catch (error) {
        console.error('Failed to load dispatcher status', error);
        if (!isMountedRef.current) return;
        setDispatcherError(error instanceof Error ? error.message : 'Unable to load dispatcher status');
      } finally {
        if (isMountedRef.current && !opts?.quiet) {
          setDispatcherLoading(false);
        }
      }
    },
    [fetchJson]
  );

  const saveConfig = useCallback(async () => {
    if (saving) return;
    setSaving(true);
    setConfigError(null);
    try {
      const payload = await fetchJson('/v1/system/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          config_repo_url: config.config_repo_url.trim(),
          agent_image: config.agent_image.trim(),
          docker_network_name: config.docker_network_name.trim(),
          default_pipeline_timeout: config.default_pipeline_timeout.trim(),
          llm_agent_timeout: config.llm_agent_timeout.trim(),
          auto_removal_agent_container: Boolean(config.auto_removal_agent_container),
          agent_nopsai_api_url: config.agent_nopsai_api_url.trim(),
          git_bot_nopsai_api_url: config.git_bot_nopsai_api_url.trim(),
          nopsai_git_bot_api_url: config.nopsai_git_bot_api_url.trim(),
        }),
      });
      applySystemConfigResponse(payload);
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
  }, [addToast, applySystemConfigResponse, config, fetchJson, saving]);

  const triggerSync = useCallback(async () => {
    if (syncBusy) return;
    setSyncBusy(true);
    setSyncError(null);
    try {
      const payload = await fetchJson('/v1/system/config/sync', { method: 'POST' });
      const status = normalizeConfigSyncStatus(payload);
      if (isMountedRef.current) {
        setSyncStatus(status);
      }
      addToast('Config sync requested.', 'info');
    } catch (error) {
      console.error('Failed to trigger config sync', error);
      addToast('Unable to start config sync.', 'error');
      if (isMountedRef.current) {
        setSyncError(error instanceof Error ? error.message : 'Unable to start config sync');
      }
    } finally {
      if (isMountedRef.current) {
        setSyncBusy(false);
      }
    }
  }, [addToast, fetchJson, syncBusy]);

  const setRunnerPending = useCallback((runnerId: string, connectionId: string, pending: boolean) => {
    const key = runnerActionKey(runnerId, connectionId);
    if (!key) return;
    setRunnerActionPending(prev => {
      const next = new Set(prev);
      if (pending) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);

  const toggleRunnerDispatch = useCallback(
    async (runner: Runner) => {
      const meta = getRunnerMeta(runner);
      const key = runnerActionKey(runner.runnerId, meta.connectionId);
      if (!key || runnerActionPending.has(key)) return;

      const nextAllow = !runner.allowDispatch;
      setRunnerPending(runner.runnerId, meta.connectionId, true);
      try {
        await fetchJson(`/v1/system/dispatcher/runners/${encodeURIComponent(runner.runnerId)}/dispatch`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            allow_dispatch: nextAllow,
            ...(meta.connectionId ? { connection_id: meta.connectionId } : {}),
          }),
        });
        await loadDispatcherStatus({ quiet: true });
      } catch (error) {
        console.error('Failed to toggle runner dispatch', error);
        addToast('Failed to update runner dispatch.', 'error');
      } finally {
        setRunnerPending(runner.runnerId, meta.connectionId, false);
      }
    },
    [addToast, fetchJson, loadDispatcherStatus, runnerActionPending, setRunnerPending]
  );

  useEffect(() => {
    void loadSystemConfig();
  }, [loadSystemConfig]);

  useEffect(() => {
    if (activeTab === 'dispatcher') {
      void loadDispatcherStatus();
    }
  }, [activeTab, loadDispatcherStatus]);

  useEffect(() => {
    const handle = window.setInterval(() => {
      void loadSyncStatus({ quiet: true });
      if (activeTab === 'dispatcher') {
        void loadDispatcherStatus({ quiet: true });
      }
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [activeTab, loadDispatcherStatus, loadSyncStatus]);

  return (
    <div data-page="system" className="active p-6 space-y-6">
      <div className="border-b border-[var(--border-primary)] pb-4 space-y-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <h2 className="text-2xl font-semibold text-[var(--text-primary)]">Control Center</h2>
            <p className="text-sm text-[var(--text-secondary)]">
              Configure GitOps sync, runner runtime, and monitor dispatcher health.
            </p>
          </div>
          <div className="flex items-center gap-2 flex-shrink-0">
            <button className="glass-button-subtle" type="button" onClick={triggerSync} disabled={syncBusy}>
              {syncBusy ? 'Syncing…' : 'Sync Config'}
            </button>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <NavLink to="/system/config" className={({ isActive }) => `system-tab-btn ${isActive ? 'system-tab-btn--active' : ''}`}>
            Config
          </NavLink>
          <NavLink
            to="/system/dispatcher"
            className={({ isActive }) => `system-tab-btn ${isActive ? 'system-tab-btn--active' : ''}`}
          >
            Dispatcher
          </NavLink>
        </div>
      </div>

      {activeTab === 'config' ? (
        <SystemConfig
          config={config}
          envFilePath={envFilePath}
          syncStatus={syncStatus}
          syncError={syncError}
          configError={configError}
          configLoading={configLoading}
          saving={saving}
          onChange={setConfig}
          onReload={loadSystemConfig}
          onRefreshSyncStatus={loadSyncStatus}
          onSave={saveConfig}
        />
      ) : (
        <DispatcherPanel
          loading={dispatcherLoading}
          error={dispatcherError}
          status={dispatcherStatus}
          pendingActions={runnerActionPending}
          onRefresh={() => loadDispatcherStatus()}
          onToggleRunnerDispatch={toggleRunnerDispatch}
        />
      )}

      {toasts.length > 0 && (
        <div className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3">
          {toasts.map(toast => (
            <div key={toast.id} className={`pipelines-toast pipelines-toast--${toast.tone} show`}>
              <div className="pipelines-toast__content">{toast.message}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function SystemConfig({
  config,
  syncStatus,
  syncError,
  configError,
  configLoading,
  saving,
  onChange,
  onReload,
  onRefreshSyncStatus,
  onSave,
}: {
  config: ConfigFormState;
  envFilePath: string;
  syncStatus: ConfigSyncStatus | null;
  syncError: string | null;
  configError: string | null;
  configLoading: boolean;
  saving: boolean;
  onChange: (next: ConfigFormState) => void;
  onReload: () => Promise<void>;
  onRefreshSyncStatus: (opts?: { quiet?: boolean }) => Promise<void>;
  onSave: () => Promise<void>;
}) {
  const repoUrl = config.config_repo_url.trim();
  const statusKey = normalizeStatus(syncStatus?.status, repoUrl);

  const handleChange = (key: keyof ConfigFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    onChange({ ...config, [key]: value } as ConfigFormState);
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    void onSave();
  };

  return (
    <div id="system-config-section" className="grid gap-6 lg:grid-cols-2 pb-24">
      <div className="space-y-4">
        <form id="system-config-form" className="space-y-4" onSubmit={onSubmit}>
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-xs text-[var(--text-secondary)]">Configuration source</p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Git sync setup</h3>
              </div>
            </div>
            <label className="flex flex-col gap-1 text-sm">
              <span>Config repo URL</span>
              <input
                id="system-config-repo"
                type="text"
                className="pipelines-input"
                value={config.config_repo_url}
                onChange={handleChange('config_repo_url')}
                placeholder="https://github.com/org/repo"
                disabled={configLoading || saving}
              />
            </label>
            <p className="text-xs text-[var(--text-secondary)]">
              This repository is the source of truth for pipelines, triggers, and steps.
            </p>
          </div>

          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">Runtime tuning</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Runners & timeouts</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <label className="flex flex-col gap-1 text-sm">
                <span>Agent image</span>
                <input
                  id="system-agent-image"
                  type="text"
                  className="pipelines-input"
                  value={config.agent_image}
                  onChange={handleChange('agent_image')}
                  placeholder="nopsai-agent:latest"
                  disabled={configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>Docker network name</span>
                <input
                  id="system-docker-network"
                  type="text"
                  className="pipelines-input"
                  value={config.docker_network_name}
                  onChange={handleChange('docker_network_name')}
                  placeholder="nopsai-net"
                  disabled={configLoading || saving}
                />
	              </label>
	              <label className="flex flex-col gap-1 text-sm">
	                <span>Default pipeline timeout</span>
	                <input
	                  id="system-default-timeout"
	                  type="text"
	                  className="pipelines-input"
	                  value={config.default_pipeline_timeout}
	                  onChange={handleChange('default_pipeline_timeout')}
	                  placeholder="30m"
	                  disabled={configLoading || saving}
	                />
	              </label>
	              <label className="flex flex-col gap-1 text-sm">
	                <span>LLM agent timeout</span>
	                <input
	                  id="system-llm-timeout"
	                  type="text"
	                  className="pipelines-input"
	                  value={config.llm_agent_timeout}
	                  onChange={handleChange('llm_agent_timeout')}
	                  placeholder="2m"
	                  disabled={configLoading || saving}
	                />
	              </label>
              <label className="flex items-center gap-2 text-sm md:col-span-2">
                <input
                  id="system-auto-remove"
                  type="checkbox"
                  checked={config.auto_removal_agent_container}
                  onChange={handleChange('auto_removal_agent_container')}
                  disabled={configLoading || saving}
                />
                <span>Auto-remove agent containers</span>
              </label>
            </div>
          </div>

          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">Networking</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Service discovery</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <label className="flex flex-col gap-1 text-sm">
                <span>Agent ↔ Server API URL</span>
                <input
                  id="system-agent-api"
                  type="text"
                  className="pipelines-input"
                  value={config.agent_nopsai_api_url}
                  onChange={handleChange('agent_nopsai_api_url')}
                  placeholder="http://agent:8080"
                  disabled={configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>GitBot ↔ Server API URL</span>
                <input
                  id="system-gitbot-api"
                  type="text"
                  className="pipelines-input"
                  value={config.git_bot_nopsai_api_url}
                  onChange={handleChange('git_bot_nopsai_api_url')}
                  placeholder="http://gitbot:8080"
                  disabled={configLoading || saving}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm md:col-span-2">
                <span>NopsAI ↔ GitBot API URL</span>
                <input
                  id="system-nopsai-gitbot-api"
                  type="text"
                  className="pipelines-input"
                  value={config.nopsai_git_bot_api_url}
                  onChange={handleChange('nopsai_git_bot_api_url')}
                  placeholder="http://nopsai-gitbot:8080"
                  disabled={configLoading || saving}
                />
              </label>
            </div>
          </div>

          {configError && (
            <div className="glass-card p-4 border border-red-500/30 rounded-xl text-sm text-red-500">
              Failed to load or save config: {configError}
            </div>
          )}
          {configLoading && <p className="text-sm text-[var(--text-secondary)]">Loading settings…</p>}
        </form>
      </div>

      <div className="space-y-4">
        <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-3">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">Repo status</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Sync overview</h3>
            </div>
            <div className="flex gap-2">
              <button className="glass-button-ghost" type="button" onClick={() => void onRefreshSyncStatus()} disabled={false}>
                Refresh status
              </button>
            </div>
          </div>

          <div className="space-y-1">
            <div className="flex items-center gap-2 flex-wrap">
              <span className="font-medium text-[var(--text-primary)]">{repoUrl || 'Not configured'}</span>
              <span className={`system-chip ${repoUrl ? (statusKey === 'success' ? 'system-chip--success' : statusKey === 'error' ? 'system-chip--error' : statusKey === 'running' ? 'system-chip--warning' : 'system-chip--muted') : 'system-chip--muted'}`}>
                {repoUrl ? (statusKey === 'success' ? 'Synced' : statusKey === 'error' ? 'Sync failed' : statusKey === 'running' ? 'Syncing' : 'Ready') : 'Not configured'}
              </span>
            </div>
            <p className="text-xs text-[var(--text-secondary)]">
              {syncStatus?.completed_at
                ? `Last sync completed ${formatTimestamp(syncStatus.completed_at)}`
                : syncStatus?.started_at
                  ? `Sync started ${formatTimestamp(syncStatus.started_at)}`
                  : repoUrl
                    ? 'Awaiting the first sync.'
                    : 'Set the Git URL to enable sync from source control.'}
            </p>
            {syncError && <p className="text-xs text-red-500">Sync status error: {syncError}</p>}
          </div>
        </div>

        <SyncStatusCard status={syncStatus} repoUrl={repoUrl} />

      </div>

      <div className="fixed bottom-6 right-6 z-40 flex items-center gap-2">
        <button className="glass-button-ghost" type="button" onClick={() => void onReload()} disabled={configLoading || saving}>
          Reload
        </button>
        <button className="glass-button-primary" type="button" onClick={() => void onSave()} disabled={configLoading || saving}>
          {saving ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </div>
  );
}

function DispatcherPanel({
  loading,
  error,
  status,
  pendingActions,
  onRefresh,
  onToggleRunnerDispatch,
}: {
  loading: boolean;
  error: string | null;
  status: { queuedJobs: number; runners: Runner[]; routing: Record<string, string[]>; fetchedAt: number } | null;
  pendingActions: Set<string>;
  onRefresh: () => void;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
}) {
  const runners = status?.runners ?? [];
  const runnerCount = runners.length;
  const queuedJobs = status?.queuedJobs ?? 0;
  const activeSum = runners.reduce((sum, r) => sum + (r.activeJobs || 0), 0);
  const updatedLabel = status?.fetchedAt ? `Updated ${new Date(status.fetchedAt).toLocaleTimeString()}` : 'Not loaded yet';
  const nowMs = status?.fetchedAt ?? 0;

  return (
    <div id="system-dispatcher-section" className="space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard label="Queued" value={queuedJobs} id="dispatcher-queue-count" />
        <StatCard label="Runners" value={runnerCount} id="dispatcher-runner-count" />
        <StatCard label="Active" value={activeSum} id="dispatcher-active-count" />
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Runners</h3>
          <div className="flex items-center gap-2">
            <span id="dispatcher-updated" className="text-sm text-[var(--text-secondary)]">
              {updatedLabel}
            </span>
            <button className="glass-button-ghost" type="button" onClick={onRefresh} disabled={loading}>
              Refresh
            </button>
          </div>
        </div>
        {error && <p className="text-sm text-red-500">Failed to load dispatcher status: {error}</p>}
        {loading && <p className="text-sm text-[var(--text-secondary)]">Loading runner status…</p>}
        <div id="dispatcher-runner-list" className="grid gap-4 md:grid-cols-2">
          {runners.map(runner => (
            <RunnerCard
              key={runnerActionKey(runner.runnerId, getRunnerMeta(runner).connectionId) || runner.runnerId}
              nowMs={nowMs}
              runner={runner}
              pendingActions={pendingActions}
              onToggleDispatch={onToggleRunnerDispatch}
            />
          ))}
        </div>
        {runners.length === 0 && (
          <div id="dispatcher-empty" className="text-sm text-[var(--text-secondary)]">
            No runners registered.
          </div>
        )}
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Routing</h3>
          <span className="text-sm text-[var(--text-secondary)]">Scope to runner mapping</span>
        </div>
        <RoutingMap routing={status?.routing ?? {}} />
      </div>
    </div>
  );
}

function RunnerCard({
  nowMs,
  runner,
  pendingActions,
  onToggleDispatch,
}: {
  nowMs: number;
  runner: Runner;
  pendingActions: Set<string>;
  onToggleDispatch: (runner: Runner) => Promise<void>;
}) {
  const stale = isStale(nowMs, runner.lastHeartbeatUnix);
  const paused = !runner.allowDispatch;
  const statusClass = stale ? 'runner-dot--error' : 'runner-dot--ok';
  const badgeLabel = stale ? 'Stale' : 'Healthy';
  const badgeClass = stale ? 'runner-pill--error' : 'runner-pill--ok';

  const meta = getRunnerMeta(runner);
  const connectionLabel = formatConnection(meta.connectionId);
  const pendingKey = runnerActionKey(runner.runnerId, meta.connectionId);
  const pending = Boolean(pendingKey && pendingActions.has(pendingKey));

  const toggleLabel = paused ? 'Resume' : 'Pause';
  const toggleTone = paused ? 'glass-button-primary' : 'glass-button-danger';
  const actionLabel = pending ? (paused ? 'Enabling…' : 'Pausing…') : toggleLabel;

  return (
    <div className={`runner-card glass-card p-5 space-y-4 ${paused ? 'runner-card--paused' : ''}`}>
      <div className="runner-card__header">
        <div className="runner-card__title">
          <span className={`runner-dot ${statusClass}`}></span>
          <div className="runner-card__title-stack">
            <div className={`runner-name ${paused ? 'runner-name--paused' : ''}`}>
              {runner.runnerId}
              {paused && <span className="runner-paused-label">Paused</span>}
            </div>
            <div className="runner-card__health-row">
              <span className={`runner-pill ${badgeClass}`}>{badgeLabel}</span>
            </div>
          </div>
        </div>
        <div className="runner-card__actions">
          <button
            type="button"
            className={`${toggleTone} text-xs ${pending ? 'opacity-60 cursor-wait' : ''}`}
            disabled={pending}
            onClick={() => void onToggleDispatch(runner)}
          >
            {actionLabel}
          </button>
        </div>
      </div>
      <div className="grid grid-cols-3 gap-2 runner-card__stat-grid text-xs">
        <div className="runner-stat">
          <span className="runner-stat__label">Active</span>
          <span className="runner-stat__value">{runner.activeJobs}</span>
        </div>
        <div className="runner-stat">
          <span className="runner-stat__label">Inflight</span>
          <span className="runner-stat__value">{runner.inflightJobs}</span>
        </div>
        <div className="runner-stat">
          <span className="runner-stat__label">Load</span>
          <span className="runner-stat__value">{runner.activeJobs}/{runner.capacity}</span>
        </div>
      </div>
      <div className="runner-card__meta-row text-xs text-[var(--text-secondary)]">
        <div className="flex flex-wrap gap-2">
          <span className="runner-pill runner-pill--muted">{runner.scopes.length ? runner.scopes.join(', ') : 'All scopes'}</span>
          {meta.hostname && <span className="runner-pill runner-pill--muted">{meta.hostname}</span>}
          {meta.network && <span className="runner-pill runner-pill--muted">{meta.network}</span>}
          <span className="runner-pill runner-pill--muted">Cap {runner.capacity}</span>
          {connectionLabel && <span className="runner-pill runner-pill--muted">{connectionLabel}</span>}
          <span className="runner-pill runner-pill--muted">Seen {formatSince(nowMs, runner.lastHeartbeatUnix)}</span>
        </div>
      </div>
      {meta.activeRuns.length > 0 ? (
        <div className="runner-run-list">
          {meta.activeRuns.slice(0, MAX_VISIBLE_ACTIVE_RUNS).map(run => {
            const runIdLabel = truncateId(run.runId, 6);
            const triggerLabel = truncateId(run.triggerId || 'manual', 6);
            const display = `${run.pipeline || 'Run'}-${triggerLabel}-${runIdLabel}`;
            const title = `${run.pipeline || 'Run'} • Trigger ${run.triggerId || 'manual'} • Run ${run.runId}`;
            const to = `/pipelineruns/recent?run_id=${encodeURIComponent(run.runId)}`;
            return (
              <Link key={run.runId} to={to} className="runner-pill runner-pill--link" title={title}>
                {display}
              </Link>
            );
          })}
          {meta.activeRuns.length > MAX_VISIBLE_ACTIVE_RUNS && (
            <span className="runner-pill runner-pill--muted">+{meta.activeRuns.length - MAX_VISIBLE_ACTIVE_RUNS}</span>
          )}
        </div>
      ) : (
        <p className="text-xs text-[var(--text-secondary)]">No active runs</p>
      )}
    </div>
  );
}

function StatCard({ label, value, id }: { label: string; value: number; id?: string }) {
  return (
    <div className="glass-card p-4 border border-[var(--border-primary)] rounded-xl" id={id}>
      <p className="text-xs text-[var(--text-secondary)]">{label}</p>
      <p className="text-2xl font-semibold">{value}</p>
    </div>
  );
}

function SyncStatusCard({ status, repoUrl }: { status: ConfigSyncStatus | null; repoUrl: string }) {
  const statusKey = normalizeStatus(status?.status, repoUrl);
  const message = (status && status.message) || defaultStatusMessage(statusKey);
  const tone = statusTone(statusKey);
  const title = syncTitle(statusKey, repoUrl);

  return (
    <div className={`pipeline-sync-card ${tone}`}>
      <div className="sync-icon">
        <svg
          className="h-4 w-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d={chooseIcon(statusKey)} />
        </svg>
      </div>
      <div className="flex-1 min-w-0">
        <h3>{title}</h3>
        <p>{message}</p>
        {status?.started_at && !status?.completed_at && (
          <p className="text-xs text-[var(--text-secondary)] mt-1">Started {formatTimestamp(status.started_at)}</p>
        )}
        {status?.completed_at && (
          <p className="text-xs text-[var(--text-secondary)] mt-1">Finished {formatTimestamp(status.completed_at)}</p>
        )}
        {status?.details && Object.keys(status.details).length > 0 && (
          <ul className="sync-detail-list">
            {Object.entries(status.details).map(([key, value]) => (
              <li key={key}>
                <strong>{key}:</strong> <span>{String(value)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function RoutingMap({ routing }: { routing: Record<string, string[]> }) {
  const rows = Object.entries(routing || {})
    .map(([scope, runners]) => ({
      scope: (scope || '*').trim() || '*',
      runners: Array.isArray(runners) && runners.length ? runners.map(r => (r || '*').trim() || '*') : ['*'],
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));

  if (rows.length === 0) {
    return (
      <div id="dispatcher-routing-empty" className="text-sm text-[var(--text-secondary)]">
        No routing rules configured.
      </div>
    );
  }

  return (
    <div id="dispatcher-routing" className="space-y-2">
      {rows.map(row => (
        <div
          key={row.scope}
          className="flex items-center justify-between gap-3 bg-[var(--bg-tertiary)] px-3 py-2 rounded-md border border-[var(--border-primary)]"
        >
          <span className="runner-pill runner-pill--ok">{row.scope}</span>
          <div className="flex flex-wrap gap-2 justify-end text-sm">
            {row.runners.map(r => (
              <span key={`${row.scope}-${r}`} className="runner-pill runner-pill--muted">
                {r === '*' ? 'Any' : r}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function formatConnection(connection: string) {
  const trimmed = connection.trim();
  if (!trimmed) return '';
  if (trimmed.length <= 14) return trimmed;
  return `${trimmed.slice(0, 6)}...${trimmed.slice(-4)}`;
}

function truncateId(value: string, length = 8) {
  const trimmed = (value || '').trim();
  if (!trimmed) return '';
  return trimmed.slice(0, length);
}

function formatTimestamp(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

function formatSince(nowMs: number, unixSeconds: number) {
  if (!unixSeconds) return 'never';
  const diff = nowMs - unixSeconds * 1000;
  if (diff < 0) return 'now';
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

function normalizeStatus(value: unknown, repoUrl = '') {
  const key = String(value ?? '').toLowerCase().trim();
  if (!key && !repoUrl.trim()) return 'missing';
  if (['running', 'loading', 'in_progress'].includes(key)) return 'running';
  if (['success', 'completed', 'complete', 'done'].includes(key)) return 'success';
  if (['error', 'failed', 'failure'].includes(key)) return 'error';
  return key || 'idle';
}

function syncTitle(statusKey: string, repoUrl: string) {
  if (!repoUrl.trim()) return 'Not configured';
  if (statusKey === 'success') return 'Synced';
  if (statusKey === 'error') return 'Sync failed';
  if (statusKey === 'running') return 'In progress';
  if (statusKey === 'idle') return 'Ready';
  return statusKey ? `${statusKey.charAt(0).toUpperCase()}${statusKey.slice(1)}` : 'Status';
}

function defaultStatusMessage(statusKey: string) {
  if (statusKey === 'running') return 'Synchronization is in progress.';
  if (statusKey === 'success') return 'Configuration synchronization completed successfully.';
  if (statusKey === 'error') return 'Configuration synchronization failed.';
  if (statusKey === 'missing') return 'Set a configuration repository to enable sync.';
  return 'Awaiting the next synchronization.';
}

function statusTone(statusKey: string) {
  if (statusKey === 'success') return 'success';
  if (statusKey === 'error') return 'error';
  if (statusKey === 'running') return 'loading';
  return '';
}

function chooseIcon(statusKey: string) {
  if (statusKey === 'success') return 'M5 13l4 4L19 7';
  if (statusKey === 'error') return 'M12 9v4m0 4h.01M5.455 5.455l13.09 13.09';
  if (statusKey === 'running')
    return 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99';
  return 'M12 18.5a6.5 6.5 0 1 1 6.32-8.13';
}

function normalizeConfigSyncStatus(value: unknown): ConfigSyncStatus | null {
  const record = asRecord(value);
  if (!record) return null;
  const status = readString(record.status);
  if (!status) return null;

  const detailsRaw = asRecord(record.details);
  const details: Record<string, number> = {};
  if (detailsRaw) {
    Object.entries(detailsRaw).forEach(([key, val]) => {
      const num = typeof val === 'number' ? val : Number(val);
      if (Number.isFinite(num)) {
        details[key] = num;
      }
    });
  }

  const normalized: ConfigSyncStatus = {
    status,
    message: readOptionalString(record.message),
    started_at: readOptionalString(record.started_at),
    completed_at: readOptionalString(record.completed_at),
  };
  if (Object.keys(details).length > 0) normalized.details = details;
  return normalized;
}

function normalizeDispatcherStatus(value: unknown): { queuedJobs: number; runners: Runner[]; routing: Record<string, string[]> } {
  const record = asRecord(value);
  const runnersRaw = record && Array.isArray(record.runners) ? record.runners : [];
  const routingRaw = record ? (record.routing ?? record.routing_map) : null;

  return {
    queuedJobs: record ? normalizeNumber(record.queued_jobs ?? record.queuedJobs) : 0,
    runners: runnersRaw.map(normalizeRunner).filter(runner => runner.runnerId),
    routing: normalizeRouting(routingRaw),
  };
}

function normalizeRunner(value: unknown): Runner {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    scopes: normalizeStringArray(record.scopes),
    capacity: normalizeNumber(record.capacity),
    activeJobs: normalizeNumber(record.active_jobs ?? record.activeJobs),
    inflightJobs: normalizeNumber(record.inflight_jobs ?? record.inflightJobs),
    lastHeartbeatUnix: normalizeNumber(record.last_heartbeat_unix ?? record.lastHeartbeatUnix),
    metadata: normalizeStringMap(record.metadata),
    allowDispatch: Boolean(record.allow_dispatch ?? record.allowDispatch),
  };
}

function getRunnerMeta(runner: Runner): RunnerMeta {
  const meta = runner.metadata || {};
  return {
    connectionId: readString(meta.connection_id || meta.instance_id),
    hostname: readString(meta.hostname || meta.host || meta.runner_host),
    network: readString(meta.docker_network || meta.docker_network_name || meta.docker_networkname),
    activeRuns: parseActiveRuns(meta),
  };
}

function parseActiveRuns(meta: Record<string, string>): RunnerActiveRun[] {
  const raw = (meta && meta.active_runs) || '';
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map(item => {
        const record = asRecord(item);
        if (!record) return null;
        const runId = readString(record.run_id);
        if (!runId) return null;
        return {
          runId,
          pipeline: readString(record.pipeline),
          parentStep: readOptionalString(record.parent_step),
          triggerId: readOptionalString(record.trigger_event_id),
        } satisfies RunnerActiveRun;
      })
      .filter(Boolean) as RunnerActiveRun[];
  } catch (error) {
    console.warn('Failed to parse active_runs metadata', error);
    return [];
  }
}

function runnerActionKey(runnerId: string, connectionId = '') {
  const rid = (runnerId || '').trim();
  const cid = (connectionId || '').trim();
  if (!rid) return '';
  return cid ? `${rid}::${cid}` : rid;
}

function isStale(nowMs: number, lastHeartbeatUnix: number) {
  if (!lastHeartbeatUnix) return true;
  return nowMs - lastHeartbeatUnix * 1000 > STALE_THRESHOLD_MS;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object') return null;
  return value as Record<string, unknown>;
}

function readString(value: unknown): string {
  if (typeof value !== 'string') return '';
  return value;
}

function readOptionalString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  return value;
}

function normalizeNumber(value: unknown): number {
  const num = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(num) ? num : 0;
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map(item => String(item || '').trim()).filter(Boolean);
}

function normalizeStringMap(value: unknown): Record<string, string> {
  const record = asRecord(value);
  if (!record) return {};
  const normalized: Record<string, string> = {};
  Object.entries(record).forEach(([key, val]) => {
    if (!key) return;
    normalized[key] = typeof val === 'string' ? val : String(val ?? '');
  });
  return normalized;
}

function normalizeRouting(value: unknown): Record<string, string[]> {
  const record = asRecord(value);
  if (!record) return {};
  const normalized: Record<string, string[]> = {};
  Object.entries(record).forEach(([scope, runners]) => {
    if (!scope) return;
    if (Array.isArray(runners)) {
      normalized[scope] = runners.map(item => String(item || '').trim()).filter(Boolean);
    } else if (typeof runners === 'string') {
      normalized[scope] = [runners.trim()].filter(Boolean);
    }
  });
  return normalized;
}

export default SystemPage;
