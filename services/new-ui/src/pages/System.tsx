import { NavLink, useParams } from 'react-router-dom';
import { useState, type ChangeEvent, type FormEvent } from 'react';

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

type Runner = {
  runnerId: string;
  scopes: string[];
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  allowDispatch: boolean;
  metadata: {
    connectionId?: string;
    hostname?: string;
    network?: string;
    activeRuns?: { pipeline: string; runId: string; triggerId?: string }[];
  };
};

const initialConfig: ConfigFormState = {
  config_repo_url: '',
  agent_image: '',
  docker_network_name: '',
  default_pipeline_timeout: '300',
  llm_agent_timeout: '120',
  auto_removal_agent_container: true,
  agent_nopsai_api_url: '',
  git_bot_nopsai_api_url: '',
  nopsai_git_bot_api_url: '',
};

const sampleRunners: Runner[] = [
  {
    runnerId: 'runner-general-1',
    scopes: ['prod'],
    capacity: 2,
    activeJobs: 1,
    inflightJobs: 0,
    allowDispatch: true,
    metadata: {
      connectionId: 'conn-1a2b3c',
      hostname: 'runner-1.local',
      network: 'nopsai-prod',
      activeRuns: [
        { pipeline: 'build-and-test', runId: 'run-12345', triggerId: 'manual' },
      ],
    },
  },
  {
    runnerId: 'runner-general-2',
    scopes: ['staging', 'qa'],
    capacity: 3,
    activeJobs: 0,
    inflightJobs: 0,
    allowDispatch: false,
    metadata: {
      connectionId: 'conn-9f8e7d',
      hostname: 'runner-2.local',
      network: 'nopsai-staging',
      activeRuns: [],
    },
  },
];

const routing = [
  { scope: 'prod', runners: ['runner-general-1'] },
  { scope: 'staging', runners: ['runner-general-2'] },
  { scope: '*', runners: ['runner-general-1', 'runner-general-2'] },
];

function SystemPage() {
  const params = useParams<{ tab?: string }>();
  const activeTab = params.tab === 'dispatcher' ? 'dispatcher' : 'config';

  return (
    <div data-page="system" className="p-6 space-y-6">
      <div className="flex items-center gap-3 border-b border-[var(--border-primary)] pb-2">
        <NavLink
          to="/system/config"
          className={({ isActive }) => `system-tab-btn ${isActive ? 'system-tab-btn--active' : ''}`}
        >
          Config
        </NavLink>
        <NavLink
          to="/system/dispatcher"
          className={({ isActive }) => `system-tab-btn ${isActive ? 'system-tab-btn--active' : ''}`}
        >
          Dispatcher
        </NavLink>
      </div>
      {activeTab === 'config' ? <SystemConfig /> : <DispatcherPanel />}
    </div>
  );
}

function SystemConfig() {
  const [config, setConfig] = useState<ConfigFormState>(initialConfig);

  const handleChange = (key: keyof ConfigFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;
    setConfig(prev => ({ ...prev, [key]: value }));
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    console.log('Config submitted', config);
  };

  return (
    <div id="system-config-section" className="space-y-6">
      <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl">
        <div className="flex items-center justify-between gap-3 mb-4">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">Configuration repository</p>
            <h3 className="text-lg font-semibold">Definitions sync</h3>
          </div>
          <div className="flex gap-2">
            <button id="system-sync-btn" className="glass-button-subtle" type="button">Sync now</button>
            <button id="system-reload-btn" className="glass-button-ghost" type="button">Reload</button>
          </div>
        </div>
        <form id="system-config-form" className="grid grid-cols-1 md:grid-cols-2 gap-4" onSubmit={onSubmit}>
          <label className="flex flex-col gap-1 text-sm">
            <span>Config repo URL</span>
            <input
              id="system-config-repo"
              type="text"
              className="pipelines-input"
              value={config.config_repo_url}
              onChange={handleChange('config_repo_url')}
              placeholder="https://github.com/org/repo"
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>Agent image</span>
            <input
              id="system-agent-image"
              type="text"
              className="pipelines-input"
              value={config.agent_image}
              onChange={handleChange('agent_image')}
              placeholder="hoseindocker/nopsai-agent:latest"
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
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>Default pipeline timeout (s)</span>
            <input
              id="system-default-timeout"
              type="number"
              className="pipelines-input"
              value={config.default_pipeline_timeout}
              onChange={handleChange('default_pipeline_timeout')}
              min={0}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>LLM agent timeout (s)</span>
            <input
              id="system-llm-timeout"
              type="number"
              className="pipelines-input"
              value={config.llm_agent_timeout}
              onChange={handleChange('llm_agent_timeout')}
              min={0}
            />
          </label>
          <label className="flex items-center gap-2 text-sm mt-6">
            <input
              id="system-auto-remove"
              type="checkbox"
              checked={config.auto_removal_agent_container}
              onChange={handleChange('auto_removal_agent_container')}
            />
            <span>Auto-remove agent container</span>
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>Agent API URL</span>
            <input
              id="system-agent-api"
              type="text"
              className="pipelines-input"
              value={config.agent_nopsai_api_url}
              onChange={handleChange('agent_nopsai_api_url')}
              placeholder="http://agent:8080"
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>GitBot API URL</span>
            <input
              id="system-gitbot-api"
              type="text"
              className="pipelines-input"
              value={config.git_bot_nopsai_api_url}
              onChange={handleChange('git_bot_nopsai_api_url')}
              placeholder="http://gitbot:8080"
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>NopsAI GitBot API URL</span>
            <input
              id="system-nopsai-gitbot-api"
              type="text"
              className="pipelines-input"
              value={config.nopsai_git_bot_api_url}
              onChange={handleChange('nopsai_git_bot_api_url')}
              placeholder="http://nopsai-gitbot:8080"
            />
          </label>
          <div className="md:col-span-2 flex items-center justify-between mt-2">
            <div>
              <p id="system-config-status" className="text-xs text-[var(--text-secondary)]">Configure sync and runners below.</p>
              <div className="text-sm text-[var(--text-secondary)] mt-1">
                <span className="font-medium text-[var(--text-primary)]" id="system-repo-display">Not configured</span>
                <span className="ml-2" id="system-repo-helper">Set the Git URL to enable sync from source control.</span>
              </div>
            </div>
            <div className="flex gap-2">
              <button id="system-sync-refresh-btn" type="button" className="glass-button-ghost">Refresh status</button>
              <button id="system-save-btn" className="glass-button-primary" type="submit">Save settings</button>
            </div>
          </div>
        </form>
      </div>

      <div className="grid gap-4 lg:grid-cols-2" id="system-sync-report">
        <div className="pipeline-sync-card success">
          <div className="sync-icon">
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <div className="flex-1 min-w-0">
            <h3>Synced</h3>
            <p>Configuration synchronization completed successfully.</p>
            <p className="text-xs text-[var(--text-secondary)] mt-1">Finished just now</p>
          </div>
        </div>
        <div className="pipeline-sync-card info">
          <div className="sync-icon">
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12 18.5a6.5 6.5 0 1 1 6.32-8.13" />
            </svg>
          </div>
          <div className="flex-1 min-w-0">
            <h3>Ready</h3>
            <p>Awaiting the next sync from your repo.</p>
            <p className="text-xs text-[var(--text-secondary)] mt-1">Updated a few seconds ago</p>
          </div>
        </div>
      </div>
    </div>
  );
}

function DispatcherPanel() {
  const runnerCount = sampleRunners.length;
  const queuedJobs = 2;
  const activeSum = sampleRunners.reduce((sum, r) => sum + r.activeJobs, 0);

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
          <span id="dispatcher-updated" className="text-sm text-[var(--text-secondary)]">Updated just now</span>
        </div>
        <div id="dispatcher-runner-list" className="grid gap-4 md:grid-cols-2">
          {sampleRunners.map(runner => (
            <RunnerCard key={runner.runnerId} runner={runner} />
          ))}
        </div>
        <div id="dispatcher-empty" className="hidden text-sm text-[var(--text-secondary)]">No runners registered.</div>
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Routing</h3>
          <span className="text-sm text-[var(--text-secondary)]">Scope to runner mapping</span>
        </div>
        <div id="dispatcher-routing" className="space-y-2">
          {routing.map(route => (
            <div
              key={route.scope}
              className="flex items-center justify-between gap-3 bg-[var(--bg-tertiary)] px-3 py-2 rounded-md border border-[var(--border-primary)]"
            >
              <span className="runner-pill runner-pill--ok">{route.scope}</span>
              <div className="flex flex-wrap gap-2 justify-end text-sm">
                {route.runners.map(r => (
                  <span key={r} className="runner-pill runner-pill--muted">{r}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
        <div id="dispatcher-routing-empty" className="hidden text-sm text-[var(--text-secondary)]">No routing rules configured.</div>
      </div>
    </div>
  );
}

function RunnerCard({ runner }: { runner: Runner }) {
  const statusClass = runner.allowDispatch ? 'runner-dot--ok' : 'runner-dot--error';
  const badgeLabel = runner.allowDispatch ? 'Healthy' : 'Paused';
  const toggleLabel = runner.allowDispatch ? 'Pause' : 'Resume';
  const connectionLabel = formatConnection(runner.metadata.connectionId || '');

  return (
    <div className={`runner-card glass-card p-5 space-y-4 ${runner.allowDispatch ? '' : 'runner-card--paused'}`}>
      <div className="runner-card__header">
        <div className="runner-card__title">
          <span className={`runner-dot ${statusClass}`}></span>
          <div className="runner-card__title-stack">
            <div className={`runner-name ${runner.allowDispatch ? '' : 'runner-name--paused'}`}>
              {runner.runnerId}
              {!runner.allowDispatch && <span className="runner-paused-label">Paused</span>}
            </div>
            <div className="runner-card__health-row">
              <span className={`runner-pill ${runner.allowDispatch ? 'runner-pill--ok' : 'runner-pill--error'}`}>{badgeLabel}</span>
            </div>
          </div>
        </div>
        <div className="runner-card__actions">
          <button
            type="button"
            className={`${runner.allowDispatch ? 'glass-button-danger' : 'glass-button-primary'} text-xs`}
          >
            {toggleLabel}
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
          {runner.metadata.network && <span className="runner-pill runner-pill--muted">{runner.metadata.network}</span>}
          <span className="runner-pill runner-pill--muted">Cap {runner.capacity}</span>
          {connectionLabel && <span className="runner-pill runner-pill--muted">{connectionLabel}</span>}
        </div>
      </div>
      {runner.metadata.activeRuns && runner.metadata.activeRuns.length > 0 ? (
        <div className="runner-run-list">
          {runner.metadata.activeRuns.map(run => (
            <span key={run.runId} className="runner-pill runner-pill--muted">
              {run.pipeline}-{run.triggerId || 'manual'}-{run.runId}
            </span>
          ))}
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

function formatConnection(connection: string) {
  const trimmed = connection.trim();
  if (!trimmed) return '';
  if (trimmed.length <= 14) return trimmed;
  return `${trimmed.slice(0, 6)}...${trimmed.slice(-4)}`;
}

export default SystemPage;
