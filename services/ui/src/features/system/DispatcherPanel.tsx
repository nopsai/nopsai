import { Link } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Activity, Boxes, Clock3, Copy, GitBranch, PauseCircle, PlayCircle, Plus, RefreshCw, Route, Server } from 'lucide-react';
import { buildApiUrl } from '../../lib/api';
import { asRecord, normalizeStringArray, normalizeStringMap, readOptionalString, readString } from './data';

type ConfigFormState = {
  runner_id: string;
  runner_scopes: string;
  runner_capacity: string;
};

const STALE_THRESHOLD_MS = 30_000;
const MAX_VISIBLE_ACTIVE_RUNS = 3;
const RUNNER_DEPLOYMENT_GUIDE_QUERY = 'guide';
const RUNNER_DEPLOYMENT_GUIDE_VALUE = 'runner';
const RUNNER_DEPLOYMENT_GUIDE_ID = 'dispatcher-runner-deployment-guide';

function scrollRunnerDeploymentGuide() {
  window.requestAnimationFrame(() => {
    document.getElementById(RUNNER_DEPLOYMENT_GUIDE_ID)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  });
}

export type Runner = {
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
  runtime: string;
  namespace: string;
  node: string;
  serviceAccount: string;
  activeRuns: RunnerActiveRun[];
};

type RunnerComposeTemplate = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  dispatcherAddress: string;
  networkMode: string;
  runnerImage: string;
  compose: string;
  command: string;
  bootstrapCommand: string;
  expiresAt: string;
  warnings: string[];
};

type KubernetesRunnerManifestTemplate = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  namespace: string;
  serviceAccount: string;
  dispatcherAddress: string;
  runnerImage: string;
  manifest: string;
  command: string;
  bootstrapCommand: string;
  expiresAt: string;
  warnings: string[];
};

type RunnerInstallRuntime = 'docker' | 'kubernetes';

export type DispatcherStatusState = {
  queuedJobs: number;
  runners: Runner[];
  routing: Record<string, string[]>;
  dispatcherError?: string;
  fetchedAt: number;
};


function DispatcherPanel({
  loading,
  error,
  status,
  pendingActions,
  onRefresh,
  onToggleRunnerDispatch,
  canManageDispatcher,
  runnerDefaults,
}: {
  loading: boolean;
  error: string | null;
  status: DispatcherStatusState | null;
  pendingActions: Set<string>;
  onRefresh: () => void;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
  canManageDispatcher: boolean;
  runnerDefaults: ConfigFormState;
}) {
  const runners = status?.runners ?? [];
  const runnerCount = runners.length;
  const queuedJobs = status?.queuedJobs ?? 0;
  const activeSum = runners.reduce((sum, r) => sum + (r.activeJobs || 0), 0);
  const kubernetesRunnerCount = runners.filter(runner => getRunnerMeta(runner).runtime === 'kubernetes').length;
  const dockerRunnerCount = Math.max(0, runnerCount - kubernetesRunnerCount);
  const pausedRunnerCount = runners.filter(runner => !runner.allowDispatch).length;
  const updatedLabel = status?.fetchedAt ? `Updated ${new Date(status.fetchedAt).toLocaleTimeString()}` : 'Not loaded yet';
  const nowMs = status?.fetchedAt ?? 0;

  return (
    <div id="system-dispatcher-section" className="dispatcher-workspace">
      <div className="dispatcher-header">
        <div>
          <p className="dispatcher-eyebrow">System Dispatcher</p>
          <h3 className="dispatcher-title">Runner Control Plane</h3>
          <p className="dispatcher-subtitle">Queue, capacity, routing, and runner installs in one view.</p>
        </div>
        <div className="dispatcher-header__actions">
          <span id="dispatcher-updated" className="dispatcher-updated">
            <Clock3 className="h-4 w-4" />
            {updatedLabel}
          </span>
          <button className="glass-button-ghost" type="button" onClick={onRefresh} disabled={loading}>
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
          <Link
            to={`/system/dispatcher?${RUNNER_DEPLOYMENT_GUIDE_QUERY}=${RUNNER_DEPLOYMENT_GUIDE_VALUE}`}
            className="glass-button-primary whitespace-nowrap"
            onClick={scrollRunnerDeploymentGuide}
          >
            <Plus className="h-4 w-4" />
            Add runner
          </Link>
        </div>
      </div>

      <div className="dispatcher-summary-grid">
        <StatCard label="Queued" value={queuedJobs} id="dispatcher-queue-count" icon={<GitBranch className="h-4 w-4" />} />
        <StatCard label="Runners" value={runnerCount} id="dispatcher-runner-count" icon={<Server className="h-4 w-4" />} hint={pausedRunnerCount > 0 ? `${pausedRunnerCount} paused` : 'dispatch ready'} />
        <StatCard label="Kubernetes" value={kubernetesRunnerCount} id="dispatcher-kubernetes-runner-count" icon={<Boxes className="h-4 w-4" />} hint={dockerRunnerCount > 0 ? `${dockerRunnerCount} docker` : 'cluster only'} />
        <StatCard label="Active" value={activeSum} id="dispatcher-active-count" icon={<Activity className="h-4 w-4" />} />
      </div>

      {error && <p className="dispatcher-error">Failed to load dispatcher status: {error}</p>}
      {status?.dispatcherError && <p className="dispatcher-error">Runner data is unavailable while dispatcher status cannot be loaded.</p>}

      <section className="dispatcher-section-card">
        <div className="dispatcher-section-header">
          <div>
            <h3>Runners</h3>
            <p>{runnerCount ? `${runnerCount} connected runner${runnerCount === 1 ? '' : 's'}` : 'No connected runners'}</p>
          </div>
          {loading && <span className="dispatcher-loading">Loading…</span>}
        </div>
        <div id="dispatcher-runner-list" className="dispatcher-runner-grid">
          {runners.map(runner => (
            <RunnerCard
              key={runnerActionKey(runner.runnerId, getRunnerMeta(runner).connectionId) || runner.runnerId}
              nowMs={nowMs}
              runner={runner}
              pendingActions={pendingActions}
              onToggleDispatch={onToggleRunnerDispatch}
              canManageDispatcher={canManageDispatcher}
            />
          ))}
        </div>
        {runners.length === 0 && (
          <div id="dispatcher-empty" className="dispatcher-empty">
            No runners registered.
          </div>
        )}
      </section>

      <section className="dispatcher-section-card">
        <div className="dispatcher-section-header">
          <div>
            <h3>Routing</h3>
            <p>Scope to runner mapping</p>
          </div>
          <Route className="h-4 w-4 text-[var(--text-secondary)]" />
        </div>
        <RoutingMap routing={status?.routing ?? {}} runners={runners} />
      </section>

      <RunnerDeploymentGuide canManageDispatcher={canManageDispatcher} runnerDefaults={runnerDefaults} />
    </div>
  );
}

function RunnerDeploymentGuide({ canManageDispatcher, runnerDefaults }: { canManageDispatcher: boolean; runnerDefaults: ConfigFormState }) {
  const [installRuntime, setInstallRuntime] = useState<RunnerInstallRuntime>('docker');
  const [runnerId, setRunnerId] = useState(runnerDefaults.runner_id || 'runner-prod-1');
  const [runnerScopes, setRunnerScopes] = useState(runnerDefaults.runner_scopes || 'prod');
  const [runnerCapacity, setRunnerCapacity] = useState(runnerDefaults.runner_capacity || '2');
  const [runnerNetworkMode, setRunnerNetworkMode] = useState('host');
  const [runnerImage, setRunnerImage] = useState('hoseindocker/nopsai-runner:latest');
  const [kubernetesNamespace, setKubernetesNamespace] = useState('nopsai-runs');
  const [kubernetesServiceAccount, setKubernetesServiceAccount] = useState('nopsai-runner');
  const [kubernetesRunnerImage, setKubernetesRunnerImage] = useState('hoseindocker/nopsai-k8s-runner:latest');
  const [kubernetesStorageClass, setKubernetesStorageClass] = useState('');
  const [kubernetesAffinityEnabled, setKubernetesAffinityEnabled] = useState(true);
  const [scopeOptions, setScopeOptions] = useState<string[]>([]);
  const [template, setTemplate] = useState<RunnerComposeTemplate | null>(null);
  const [kubernetesTemplate, setKubernetesTemplate] = useState<KubernetesRunnerManifestTemplate | null>(null);
  const [loadingTemplate, setLoadingTemplate] = useState(false);
  const [loadingKubernetesTemplate, setLoadingKubernetesTemplate] = useState(false);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [kubernetesTemplateError, setKubernetesTemplateError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [copiedKubernetes, setCopiedKubernetes] = useState(false);

  useEffect(() => {
    if (!canManageDispatcher) return;
    let cancelled = false;
    fetch(buildApiUrl('/v1/system/dispatcher/scopes'), { cache: 'no-store' })
      .then(response => (response.ok ? response.json() : []))
      .then(payload => {
        if (cancelled) return;
        setScopeOptions(normalizeRuntimeScopeOptions(payload));
      })
      .catch(() => {
        if (!cancelled) setScopeOptions([]);
      });
    return () => {
      cancelled = true;
    };
  }, [canManageDispatcher]);

  const selectedRunnerScopes = useMemo(() => splitCSV(runnerScopes), [runnerScopes]);
  const selectedRunnerScopeSet = useMemo(() => new Set(selectedRunnerScopes), [selectedRunnerScopes]);
  const runnerScopeChoices = useMemo(() => {
    return sortRuntimeScopeOptions(Array.from(new Set([...scopeOptions, ...selectedRunnerScopes])));
  }, [scopeOptions, selectedRunnerScopes]);

  const toggleRunnerScope = (scope: string, checked: boolean) => {
    const next = new Set(selectedRunnerScopes);
    if (checked) {
      next.add(scope);
    } else {
      next.delete(scope);
    }
    setRunnerScopes(sortRuntimeScopeOptions(Array.from(next)).join(','));
  };

  const loadTemplate = useCallback(async () => {
    if (!canManageDispatcher) return;
    const capacity = Number.parseInt(runnerCapacity, 10);
    if (!Number.isFinite(capacity) || capacity <= 0) {
      setTemplateError('Capacity must be a positive number.');
      return;
    }
    const params = new URLSearchParams({
      runner_id: runnerId.trim() || 'runner-prod-1',
      runner_scopes: runnerScopes.trim(),
      runner_capacity: String(capacity),
      runner_network_mode: runnerNetworkMode,
      runner_image: runnerImage.trim() || 'hoseindocker/nopsai-runner:latest',
    });
    setLoadingTemplate(true);
    setTemplateError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/dispatcher/runner-bootstrap-command?${params.toString()}`), { cache: 'no-store' });
      if (!response.ok) throw new Error(await response.text() || `Unable to generate runner install command (${response.status})`);
      setTemplate(normalizeRunnerComposeTemplate(await response.json()));
    } catch (error) {
      setTemplate(null);
      setTemplateError(error instanceof Error ? error.message : 'Unable to generate runner install command.');
    } finally {
      setLoadingTemplate(false);
    }
  }, [canManageDispatcher, runnerCapacity, runnerId, runnerImage, runnerNetworkMode, runnerScopes]);

  const loadKubernetesTemplate = useCallback(async () => {
    if (!canManageDispatcher) return;
    const capacity = Number.parseInt(runnerCapacity, 10);
    if (!Number.isFinite(capacity) || capacity <= 0) {
      setKubernetesTemplateError('Capacity must be a positive number.');
      return;
    }
    const params = new URLSearchParams({
      runner_id: runnerId.trim() || 'k8s-runner-prod-1',
      runner_scopes: runnerScopes.trim(),
      runner_capacity: String(capacity),
      namespace: kubernetesNamespace.trim() || 'nopsai-runs',
      service_account: kubernetesServiceAccount.trim() || 'nopsai-runner',
      runner_image: kubernetesRunnerImage.trim() || 'hoseindocker/nopsai-k8s-runner:latest',
      affinity_enabled: String(kubernetesAffinityEnabled),
    });
    if (kubernetesStorageClass.trim()) params.set('storage_class', kubernetesStorageClass.trim());
    setLoadingKubernetesTemplate(true);
    setKubernetesTemplateError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/system/dispatcher/kubernetes-runner-bootstrap-command?${params.toString()}`), { cache: 'no-store' });
      if (!response.ok) throw new Error(await response.text() || `Unable to generate Kubernetes install command (${response.status})`);
      setKubernetesTemplate(normalizeKubernetesRunnerManifestTemplate(await response.json()));
    } catch (error) {
      setKubernetesTemplate(null);
      setKubernetesTemplateError(error instanceof Error ? error.message : 'Unable to generate Kubernetes install command.');
    } finally {
      setLoadingKubernetesTemplate(false);
    }
  }, [canManageDispatcher, kubernetesAffinityEnabled, kubernetesNamespace, kubernetesRunnerImage, kubernetesServiceAccount, kubernetesStorageClass, runnerCapacity, runnerId, runnerScopes]);

  const handleCopyTemplate = async () => {
    if (!template?.bootstrapCommand) return;
    try {
      await navigator.clipboard.writeText(template.bootstrapCommand);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch (error) {
      console.error('Failed to copy runner install command', error);
      setTemplateError('Unable to copy runner install command.');
    }
  };

  const handleCopyKubernetesInstallCommand = async () => {
    if (!kubernetesTemplate?.bootstrapCommand) return;
    try {
      await navigator.clipboard.writeText(kubernetesTemplate.bootstrapCommand);
      setCopiedKubernetes(true);
      window.setTimeout(() => setCopiedKubernetes(false), 1600);
    } catch (error) {
      console.error('Failed to copy Kubernetes runner install command', error);
      setKubernetesTemplateError('Unable to copy Kubernetes install command.');
    }
  };

  const isKubernetesInstall = installRuntime === 'kubernetes';
  const activeTemplate = isKubernetesInstall ? kubernetesTemplate : template;
  const activeLoading = isKubernetesInstall ? loadingKubernetesTemplate : loadingTemplate;
  const activeError = isKubernetesInstall ? kubernetesTemplateError : templateError;
  const activeWarnings = activeTemplate?.warnings || [];
  const activeCommand = activeTemplate?.bootstrapCommand || '';
  const activeExpiresAt = activeTemplate?.expiresAt || '';
  const activeDispatcherAddress = activeTemplate?.dispatcherAddress || '';
  const activeRunnerImage = activeTemplate?.runnerImage || '';

  return (
    <section id={RUNNER_DEPLOYMENT_GUIDE_ID} className="dispatcher-install scroll-mt-6">
      <div className="dispatcher-section-header">
        <div>
          <h3>Runner Installs</h3>
          <p>Generate one runtime at a time with the same runner identity, scope, and capacity controls.</p>
        </div>
        <div className="dispatcher-runtime-switch" role="tablist" aria-label="Runner install runtime">
          {(['docker', 'kubernetes'] as const).map(runtime => (
            <button
              key={runtime}
              type="button"
              className={installRuntime === runtime ? 'is-active' : ''}
              aria-pressed={installRuntime === runtime}
              onClick={() => setInstallRuntime(runtime)}
            >
              {runtime === 'docker' ? <Server className="h-4 w-4" /> : <Boxes className="h-4 w-4" />}
              <span>{runtime === 'docker' ? 'Docker' : 'Kubernetes'}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="dispatcher-install-card dispatcher-install-card--focused">
        <div className="dispatcher-install-card__head">
          <div>
            <h4>{isKubernetesInstall ? 'Kubernetes runner' : 'Docker runner'}</h4>
            <p>
              {isKubernetesInstall
                ? 'Install an in-cluster namespace runner with RBAC, service auth, and runtime defaults.'
                : 'Install a host runner with dispatcher, TLS, service auth, image, scope, and capacity prefilled.'}
            </p>
          </div>
          <span className="runner-pill runner-pill--muted">{isKubernetesInstall ? 'Namespace runtime' : 'Host runtime'}</span>
        </div>

        {canManageDispatcher ? (
          <div className="dispatcher-install-card__body">
            <div className="dispatcher-install-grid">
              <label className="space-y-1.5 text-sm">
                <span className="dispatcher-field-label">Runner name</span>
                <input className="pipelines-input w-full" value={runnerId} onChange={event => setRunnerId(event.target.value)} />
              </label>
              <label className="space-y-1.5 text-sm">
                <span className="dispatcher-field-label">Capacity</span>
                <input className="pipelines-input w-full" type="number" min="1" value={runnerCapacity} onChange={event => setRunnerCapacity(event.target.value)} />
              </label>
              <div className="space-y-1.5 text-sm dispatcher-install-grid__wide">
                <span className="dispatcher-field-label">Scopes</span>
                <div className="dispatcher-scope-picker">
                  <label className={`runner-pill cursor-pointer ${selectedRunnerScopes.length === 0 ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                    <input
                      type="checkbox"
                      className="sr-only"
                      checked={selectedRunnerScopes.length === 0}
                      onChange={() => setRunnerScopes('')}
                    />
                    All scopes
                  </label>
                  {runnerScopeChoices.map(scope => (
                    <label key={scope} className={`runner-pill cursor-pointer ${selectedRunnerScopeSet.has(scope) ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                      <input
                        type="checkbox"
                        className="sr-only"
                        checked={selectedRunnerScopeSet.has(scope)}
                        onChange={event => toggleRunnerScope(scope, event.target.checked)}
                      />
                      {scope}
                    </label>
                  ))}
                </div>
              </div>

              {isKubernetesInstall ? (
                <>
                  <label className="space-y-1.5 text-sm">
                    <span className="dispatcher-field-label">Namespace</span>
                    <input className="pipelines-input w-full" value={kubernetesNamespace} onChange={event => setKubernetesNamespace(event.target.value)} />
                  </label>
                  <label className="space-y-1.5 text-sm">
                    <span className="dispatcher-field-label">Service account</span>
                    <input className="pipelines-input w-full" value={kubernetesServiceAccount} onChange={event => setKubernetesServiceAccount(event.target.value)} />
                  </label>
                  <label className="space-y-1.5 text-sm">
                    <span className="dispatcher-field-label">Storage class</span>
                    <input className="pipelines-input w-full" value={kubernetesStorageClass} onChange={event => setKubernetesStorageClass(event.target.value)} placeholder="cluster default" />
                  </label>
                  <label className="dispatcher-toggle">
                    <input type="checkbox" checked={kubernetesAffinityEnabled} onChange={event => setKubernetesAffinityEnabled(event.target.checked)} />
                    <span className="dispatcher-toggle__control" aria-hidden="true">
                      <span />
                    </span>
                    <span className="min-w-0">
                      <span className="dispatcher-toggle__label">Default step affinity</span>
                      <span className="dispatcher-toggle__hint">Pipeline affinity_enabled can override it.</span>
                    </span>
                  </label>
                  <label className="space-y-1.5 text-sm dispatcher-install-grid__wide">
                    <span className="dispatcher-field-label">Runner image</span>
                    <input className="pipelines-input w-full font-mono text-xs sm:text-sm" value={kubernetesRunnerImage} onChange={event => setKubernetesRunnerImage(event.target.value)} />
                  </label>
                </>
              ) : (
                <>
                  <div className="space-y-1.5 text-sm">
                    <span className="dispatcher-field-label">Network mode</span>
                    <div className="dispatcher-choice-group">
                      {(['host', 'bridge'] as const).map(mode => (
                        <button
                          key={mode}
                          type="button"
                          className={runnerNetworkMode === mode ? 'is-active' : ''}
                          aria-pressed={runnerNetworkMode === mode}
                          onClick={() => setRunnerNetworkMode(mode)}
                        >
                          {mode === 'host' ? 'Host' : 'Bridge'}
                        </button>
                      ))}
                    </div>
                  </div>
                  <label className="space-y-1.5 text-sm dispatcher-install-grid__wide">
                    <span className="dispatcher-field-label">Runner image</span>
                    <input className="pipelines-input w-full font-mono text-xs sm:text-sm" value={runnerImage} onChange={event => setRunnerImage(event.target.value)} />
                  </label>
                </>
              )}
            </div>

            <div className="dispatcher-install-actions">
              <button
                type="button"
                className="glass-button-subtle"
                onClick={() => void (isKubernetesInstall ? loadKubernetesTemplate() : loadTemplate())}
                disabled={activeLoading}
              >
                <RefreshCw className={`h-4 w-4 ${activeLoading ? 'animate-spin' : ''}`} />
                {activeLoading ? 'Generating…' : activeCommand ? 'Regenerate command' : 'Generate command'}
              </button>
              <button
                type="button"
                className="glass-button-primary"
                onClick={() => void (isKubernetesInstall ? handleCopyKubernetesInstallCommand() : handleCopyTemplate())}
                disabled={!activeCommand || activeLoading}
              >
                <Copy className="h-4 w-4" />
                {isKubernetesInstall ? (copiedKubernetes ? 'Copied' : 'Copy command') : (copied ? 'Copied' : 'Copy command')}
              </button>
            </div>

            {activeError && <p className="text-sm text-red-500">{activeError}</p>}

            {(activeDispatcherAddress || activeExpiresAt || activeRunnerImage || (!isKubernetesInstall && template?.networkMode) || (isKubernetesInstall && kubernetesTemplate?.namespace)) && (
              <div className="dispatcher-install-meta">
                {activeDispatcherAddress && <RunnerFact label="Dispatcher" value={activeDispatcherAddress} mono />}
                {!isKubernetesInstall && template?.networkMode && <RunnerFact label="Network mode" value={template.networkMode} />}
                {isKubernetesInstall && kubernetesTemplate?.namespace && <RunnerFact label="Namespace" value={kubernetesTemplate.namespace} mono />}
                {activeExpiresAt && <RunnerFact label="Token expires" value={formatTimestamp(activeExpiresAt)} />}
                {activeRunnerImage && <RunnerFact label="Image" value={activeRunnerImage} mono />}
              </div>
            )}

            <pre className="dispatcher-install-command">
              <code>{activeCommand || `Generate a one-time ${isKubernetesInstall ? 'Kubernetes' : 'Docker'} install command first.`}</code>
            </pre>

            {activeWarnings.map(warning => (
              <div key={warning} className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs leading-5 text-amber-700 dark:text-amber-300">
                {warning}
              </div>
            ))}
          </div>
        ) : (
          <div className="mt-4 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-sm leading-6 text-[var(--text-secondary)]">
            Dispatcher management access is required to generate runner install commands.
          </div>
        )}
      </div>
    </section>
  );
}

function RunnerFact({ label, value, mono }: { label: string; value: string | number; mono?: boolean }) {
  return (
    <span className="runner-fact">
      <span className="runner-fact__label">{label}</span>
      <span className={`runner-fact__value ${mono ? 'runner-fact__value--mono' : ''}`}>{value}</span>
    </span>
  );
}

function RunnerCard({
  nowMs,
  runner,
  pendingActions,
  onToggleDispatch,
  canManageDispatcher,
}: {
  nowMs: number;
  runner: Runner;
  pendingActions: Set<string>;
  onToggleDispatch: (runner: Runner) => Promise<void>;
  canManageDispatcher: boolean;
}) {
  const stale = isStale(nowMs, runner.lastHeartbeatUnix);
  const paused = !runner.allowDispatch;
  const statusClass = paused ? 'runner-dot--warning' : stale ? 'runner-dot--error' : 'runner-dot--ok';
  const statusLabel = paused ? 'Dispatch paused' : stale ? 'Heartbeat stale' : 'Online';
  const statusTone = paused ? 'runner-pill--warning' : stale ? 'runner-pill--error' : 'runner-pill--ok';

  const meta = getRunnerMeta(runner);
  const runtimeLabel = meta.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker';
  const runtimeTone = meta.runtime === 'kubernetes' ? 'runner-pill--ok' : 'runner-pill--muted';
  const connectionLabel = formatConnection(meta.connectionId);
  const pendingKey = runnerActionKey(runner.runnerId, meta.connectionId);
  const pending = Boolean(pendingKey && pendingActions.has(pendingKey));

  const toggleLabel = paused ? 'Resume dispatch' : 'Pause dispatch';
  const toggleTone = paused ? 'glass-button-primary' : 'glass-button-danger';
  const actionLabel = pending ? (paused ? 'Resuming…' : 'Pausing…') : toggleLabel;
  const scopesLabel = runner.scopes.length ? runner.scopes.join(', ') : 'All scopes';
  const activeRunCount = meta.activeRuns.length;

  return (
    <div className={`runner-card p-5 ${paused ? 'runner-card--paused' : stale ? 'runner-card--stale' : ''}`}>
      <div className="runner-card__header">
        <div className="runner-card__title">
          <span className={`runner-dot ${statusClass}`} aria-hidden="true"></span>
          <div className="runner-card__title-stack">
            <div className={`runner-name ${paused ? 'runner-name--paused' : ''}`}>{runner.runnerId}</div>
            <div className="runner-card__badges">
              <span className={`runner-pill ${statusTone}`}>{statusLabel}</span>
              <span className={`runner-pill ${runtimeTone}`}>{runtimeLabel}</span>
            </div>
          </div>
        </div>
        <div className="runner-card__actions">
          <button
            type="button"
            className={`${toggleTone} text-xs ${pending ? 'opacity-60 cursor-wait' : ''}`}
            disabled={!canManageDispatcher || pending}
            onClick={() => void onToggleDispatch(runner)}
          >
            {paused ? <PlayCircle className="h-4 w-4" /> : <PauseCircle className="h-4 w-4" />}
            {actionLabel}
          </button>
        </div>
      </div>

      <div className="runner-card__metrics">
        <RunnerMetric label="Running" value={runner.activeJobs} />
        <RunnerMetric label="Assigned" value={runner.inflightJobs} />
        <RunnerMetric label="Capacity" value={`${runner.activeJobs}/${runner.capacity}`} />
      </div>

      <div className="runner-card__facts">
        <RunnerFact label="Scopes" value={scopesLabel} />
        <RunnerFact label="Last heartbeat" value={formatSince(nowMs, runner.lastHeartbeatUnix)} />
        {connectionLabel && <RunnerFact label="Connection" value={connectionLabel} mono />}
        {meta.runtime === 'kubernetes' ? (
          <>
            {meta.namespace && <RunnerFact label="Namespace" value={meta.namespace} mono />}
            {meta.node && <RunnerFact label="Node" value={meta.node} mono />}
            {meta.serviceAccount && <RunnerFact label="Service account" value={meta.serviceAccount} mono />}
          </>
        ) : (
          <>
            {meta.hostname && <RunnerFact label="Host" value={meta.hostname} mono />}
            {meta.network && <RunnerFact label="Docker network" value={meta.network} mono />}
          </>
        )}
      </div>

      <div className="runner-card__runs">
        <div className="runner-card__runs-header">
          <span>Active runs</span>
          {activeRunCount > 0 && <span>{activeRunCount}</span>}
        </div>
        {activeRunCount > 0 ? (
          <div className="runner-run-list">
            {meta.activeRuns.slice(0, MAX_VISIBLE_ACTIVE_RUNS).map(run => {
              const runIdLabel = truncateId(run.runId, 8);
              const display = `${run.pipeline || 'Run'} run ${runIdLabel}`;
              const title = `${run.pipeline || 'Run'} | Trigger ${run.triggerId || 'manual'} | Run ${run.runId}`;
              const to = `/pipelineruns/recent?run_id=${encodeURIComponent(run.runId)}`;
              return (
                <Link key={run.runId} to={to} className="runner-pill runner-pill--link" title={title}>
                  {display}
                </Link>
              );
            })}
            {activeRunCount > MAX_VISIBLE_ACTIVE_RUNS && (
              <span className="runner-pill runner-pill--muted">+{activeRunCount - MAX_VISIBLE_ACTIVE_RUNS} more</span>
            )}
          </div>
        ) : (
          <p>No active runs</p>
        )}
      </div>
    </div>
  );
}

function RunnerMetric({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="runner-metric">
      <span className="runner-metric__label">{label}</span>
      <span className="runner-metric__value">{value}</span>
    </div>
  );
}

function StatCard({ label, value, id, icon, hint }: { label: string; value: number; id?: string; icon?: ReactNode; hint?: string }) {
  return (
    <div className="dispatcher-stat-card" id={id}>
      <div className="dispatcher-stat-card__top">
        <p>{label}</p>
        {icon && <span>{icon}</span>}
      </div>
      <div className="dispatcher-stat-card__value">{value}</div>
      {hint && <div className="dispatcher-stat-card__hint">{hint}</div>}
    </div>
  );
}

function RoutingMap({ routing, runners }: { routing: Record<string, string[]>; runners: Runner[] }) {
  const rows = Object.entries(routing || {})
    .map(([scope, runners]) => ({
      scope: (scope || '*').trim() || '*',
      runners: Array.isArray(runners) && runners.length ? runners.map(r => (r || '*').trim() || '*') : ['*'],
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));
  const liveRows = buildLiveRoutingRows(runners);
  const connectedRunnerIds = new Set(runners.map(runner => runner.runnerId).filter(Boolean));

  if (rows.length === 0 && liveRows.length === 0) {
    return (
      <div id="dispatcher-routing-empty" className="text-sm text-[var(--text-secondary)]">
        No routing rules configured.
      </div>
    );
  }

  return (
    <div id="dispatcher-routing" className="space-y-2">
      {rows.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Configured routing</p>
          {rows.map(row => (
            <div
              key={row.scope}
              className="flex items-center justify-between gap-3 bg-[var(--bg-tertiary)] px-3 py-2 rounded-md border border-[var(--border-primary)]"
            >
              <span className="runner-pill runner-pill--ok">{formatRoutingScope(row.scope)}</span>
              <div className="flex flex-wrap gap-2 justify-end text-sm">
                {row.runners.map(runnerId => {
                  const connected = connectedRunnerIds.has(runnerId);
                  return (
                    <span key={`${row.scope}-${runnerId}`} className={`runner-pill ${connected ? 'runner-pill--ok' : 'runner-pill--warning'}`}>
                      {runnerId}
                    </span>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      ) : null}
      {liveRows.length > 0 ? (
        <div className="space-y-2">
          {rows.length > 0 && <p className="pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Live runner scopes</p>}
          {liveRows.map(row => (
            <div
              key={`live-${row.scope}`}
              className="flex items-center justify-between gap-3 bg-[var(--bg-tertiary)] px-3 py-2 rounded-md border border-[var(--border-primary)]"
            >
              <span className="runner-pill runner-pill--ok">{formatRoutingScope(row.scope)}</span>
              <div className="flex flex-wrap gap-2 justify-end text-sm">
                {row.runners.map(runnerId => (
                  <span key={`live-${row.scope}-${runnerId}`} className="runner-pill runner-pill--muted">
                    {runnerId}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : rows.length === 0 ? (
        <div id="dispatcher-routing-empty" className="text-sm text-[var(--text-secondary)]">
          No live runner scopes.
        </div>
      ) : null}
    </div>
  );
}

function buildLiveRoutingRows(runners: Runner[]): Array<{ scope: string; runners: string[] }> {
  const scopeMap = new Map<string, Set<string>>();
  runners.forEach(runner => {
    if (!runner.runnerId) return;
    const scopes = runner.scopes.length ? runner.scopes : ['*'];
    scopes.forEach(scopeValue => {
      const scope = (scopeValue || '*').trim() || '*';
      const existing = scopeMap.get(scope) || new Set<string>();
      existing.add(runner.runnerId);
      scopeMap.set(scope, existing);
    });
  });
  return Array.from(scopeMap.entries())
    .map(([scope, runnerSet]) => ({
      scope,
      runners: Array.from(runnerSet).sort((a, b) => a.localeCompare(b)),
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));
}

function formatRoutingScope(scope: string) {
  return scope === '*' ? 'Default' : scope;
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


export function normalizeDispatcherStatus(value: unknown): Omit<DispatcherStatusState, 'fetchedAt'> {
  const record = asRecord(value);
  const runnersRaw = record && Array.isArray(record.runners) ? record.runners : [];
  const routingRaw = record ? (record.routing ?? record.routing_map) : null;

  return {
    queuedJobs: record ? normalizeNumber(record.queued_jobs ?? record.queuedJobs) : 0,
    runners: runnersRaw.map(normalizeRunner).filter(runner => runner.runnerId),
    routing: normalizeRouting(routingRaw),
    dispatcherError: record ? readOptionalString(record.dispatcher_error ?? record.dispatcherError) : undefined,
  };
}

function normalizeRunnerComposeTemplate(value: unknown): RunnerComposeTemplate {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    runnerScopes: readString(record.runner_scopes ?? record.runnerScopes),
    runnerCapacity: normalizeNumber(record.runner_capacity ?? record.runnerCapacity),
    dispatcherAddress: readString(record.dispatcher_address ?? record.dispatcherAddress),
    networkMode: readString(record.network_mode ?? record.networkMode),
    runnerImage: readString(record.runner_image ?? record.runnerImage),
    compose: readString(record.compose),
    command: readString(record.command),
    bootstrapCommand: readString(record.bootstrap_command ?? record.bootstrapCommand),
    expiresAt: readString(record.expires_at ?? record.expiresAt),
    warnings: normalizeStringArray(record.warnings),
  };
}

function normalizeKubernetesRunnerManifestTemplate(value: unknown): KubernetesRunnerManifestTemplate {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    runnerScopes: readString(record.runner_scopes ?? record.runnerScopes),
    runnerCapacity: normalizeNumber(record.runner_capacity ?? record.runnerCapacity),
    namespace: readString(record.namespace),
    serviceAccount: readString(record.service_account ?? record.serviceAccount),
    dispatcherAddress: readString(record.dispatcher_address ?? record.dispatcherAddress),
    runnerImage: readString(record.runner_image ?? record.runnerImage),
    manifest: readString(record.manifest),
    command: readString(record.command),
    bootstrapCommand: readString(record.bootstrap_command ?? record.bootstrapCommand),
    expiresAt: readString(record.expires_at ?? record.expiresAt),
    warnings: normalizeStringArray(record.warnings),
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

export function getRunnerMeta(runner: Runner): RunnerMeta {
  const meta = runner.metadata || {};
  const runtime = readString(meta.runtime || meta.runner_runtime).toLowerCase();
  return {
    connectionId: readString(meta.connection_id || meta.instance_id),
    hostname: readString(meta.hostname || meta.host || meta.runner_host),
    network: readString(meta.docker_network || meta.docker_network_name || meta.docker_networkname),
    runtime: runtime === 'k8s' ? 'kubernetes' : runtime || 'docker',
    namespace: readString(meta.kubernetes_namespace || meta.namespace),
    node: readString(meta.kubernetes_node || meta.node),
    serviceAccount: readString(meta.kubernetes_service_account || meta.service_account),
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

export function runnerActionKey(runnerId: string, connectionId = '') {
  const rid = (runnerId || '').trim();
  const cid = (connectionId || '').trim();
  if (!rid) return '';
  return cid ? `${rid}::${cid}` : rid;
}

function isStale(nowMs: number, lastHeartbeatUnix: number) {
  if (!lastHeartbeatUnix) return true;
  return nowMs - lastHeartbeatUnix * 1000 > STALE_THRESHOLD_MS;
}


function normalizeListPayload(payload: unknown, keys: string[] = []): unknown[] | null {
  let value = payload;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed || trimmed === 'null') return [];
    if (trimmed.startsWith('[') || trimmed.startsWith('{')) {
      try {
        value = JSON.parse(trimmed);
      } catch {
        return null;
      }
    }
  }
  if (value == null) return [];
  if (Array.isArray(value)) return value;

  const record = asRecord(value);
  if (!record) return null;
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) continue;
    const candidate = record[key];
    if (candidate == null) return [];
    if (Array.isArray(candidate)) return candidate;
  }
  return null;
}


function normalizeNumber(value: unknown): number {
  const num = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(num) ? num : 0;
}


function splitCSV(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

function normalizeRuntimeScopeOptions(value: unknown): string[] {
  const items = normalizeListPayload(value, ['scopes']);
  if (!items) return [];
  const scopes = new Set<string>();
  items.forEach(item => {
    const record = asRecord(item);
    const raw = record ? record.scope ?? record.name ?? record.value : item;
    const scope = readString(raw).trim().replace(/^\/+|\/+$/g, '');
    if (scope) scopes.add(scope);
  });
  return sortRuntimeScopeOptions(Array.from(scopes));
}

function sortRuntimeScopeOptions(scopes: string[]): string[] {
  return scopes.map(scope => scope.trim()).filter(Boolean).sort((a, b) => {
    if (a === 'default' && b !== 'default') return -1;
    if (b === 'default' && a !== 'default') return 1;
    return a.localeCompare(b);
  });
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


export default DispatcherPanel;
