import { Link } from 'react-router-dom';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Activity, Boxes, Check, Clock3, Copy, GitBranch, KeyRound, PauseCircle, PlayCircle, Plus, RefreshCw, Route, Server, Trash2 } from 'lucide-react';
import { copyTextToClipboard } from '../../lib/clipboard';
import type { ConfigFieldMetadata, ConfigFormState } from './config/model';
import { ApplyBadge } from './config/ConfigApplyBadge';
import {
  buildLiveRunnerRoutingRows,
  dispatcherRoutingConfigSignature,
  dispatcherRoutingRowsToConfig,
  formatDispatcherRouteScope,
  getRunnerMeta,
  normalizeDispatcherRoutingScope,
  runnerActionKey,
  type DispatcherRoutingDraftRow,
  type DispatcherStatusState,
  type Runner,
  type RunnerInstallRuntime,
} from './dispatcher/model';
import { useRunnerDeploymentGuide } from './dispatcher/useRunnerDeploymentGuide';

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

function DispatcherPanel({
  loading,
  error,
  status,
  pendingActions,
  pendingEjections,
  onToggleRunnerDispatch,
  onEjectRunner,
  canManageDispatcher,
  canViewRuntimeConfig,
  canManageRuntimeConfig,
  runnerDefaults,
  config,
  fieldMetadata,
  configLoading,
  saving,
  onConfigChange,
  onSaveConfig,
}: {
  loading: boolean;
  error: string | null;
  status: DispatcherStatusState | null;
  pendingActions: Set<string>;
  pendingEjections: Set<string>;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
  onEjectRunner: (runner: Runner) => Promise<void>;
  canManageDispatcher: boolean;
  canViewRuntimeConfig: boolean;
  canManageRuntimeConfig: boolean;
  runnerDefaults: ConfigFormState;
  config: ConfigFormState;
  fieldMetadata: Record<string, ConfigFieldMetadata>;
  configLoading: boolean;
  saving: boolean;
  onConfigChange: (next: ConfigFormState) => void;
  onSaveConfig: () => Promise<void>;
}) {
  const runners = status?.runners ?? [];
  const runnerCount = runners.length;
  const queuedJobs = status?.queuedJobs ?? 0;
  const activeSum = runners.reduce((sum, r) => sum + (r.activeJobs || 0), 0);
  const kubernetesRunnerCount = runners.filter(runner => getRunnerMeta(runner).runtime === 'kubernetes').length;
  const dockerRunnerCount = Math.max(0, runnerCount - kubernetesRunnerCount);
  const unreachableRunnerCount = runners.filter(runner => !getRunnerMeta(runner).reachable).length;
  const pausedRunnerCount = runners.filter(runner => getRunnerMeta(runner).reachable && !runner.allowDispatch).length;
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
        <StatCard label="Runners" value={runnerCount} id="dispatcher-runner-count" icon={<Server className="h-4 w-4" />} hint={unreachableRunnerCount > 0 ? `${unreachableRunnerCount} unreachable` : pausedRunnerCount > 0 ? `${pausedRunnerCount} paused` : 'dispatch ready'} />
        <StatCard label="Kubernetes" value={kubernetesRunnerCount} id="dispatcher-kubernetes-runner-count" icon={<Boxes className="h-4 w-4" />} hint={dockerRunnerCount > 0 ? `${dockerRunnerCount} docker` : 'cluster only'} />
        <StatCard label="Active" value={activeSum} id="dispatcher-active-count" icon={<Activity className="h-4 w-4" />} />
      </div>

      {error && <p className="dispatcher-error">Failed to load dispatcher status: {error}</p>}
      {status?.dispatcherError && <p className="dispatcher-error">Runner data is unavailable while dispatcher status cannot be loaded.</p>}

      <section className="dispatcher-section-card">
        <div className="dispatcher-section-header">
          <div>
            <h3>Runners</h3>
            <p>{runnerCount ? `${runnerCount} registered runner${runnerCount === 1 ? '' : 's'}` : 'No registered runners'}</p>
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
              pendingEjections={pendingEjections}
              onToggleDispatch={onToggleRunnerDispatch}
              onEjectRunner={onEjectRunner}
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
        {canViewRuntimeConfig ? (
          <DispatcherRoutingEditor
            config={config}
            fieldMetadata={fieldMetadata}
            configLoading={configLoading}
            saving={saving}
            canManageRuntimeConfig={canManageRuntimeConfig}
            onConfigChange={onConfigChange}
            onSaveConfig={onSaveConfig}
          />
        ) : (
          <div className="mb-4 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
            Runtime config access is required to edit routing.
          </div>
        )}
        <RoutingMap routing={status?.routing ?? {}} effectiveRouting={status?.effectiveRouting ?? {}} runners={runners} />
      </section>

      <RunnerDeploymentGuide canManageDispatcher={canManageDispatcher} runnerDefaults={runnerDefaults} />
    </div>
  );
}

type RoutingDraftRow = DispatcherRoutingDraftRow & {
  localId: string;
};

function DispatcherRoutingEditor({
  config,
  fieldMetadata,
  configLoading,
  saving,
  canManageRuntimeConfig,
  onConfigChange,
  onSaveConfig,
}: {
  config: ConfigFormState;
  fieldMetadata: Record<string, ConfigFieldMetadata>;
  configLoading: boolean;
  saving: boolean;
  canManageRuntimeConfig: boolean;
  onConfigChange: (next: ConfigFormState) => void;
  onSaveConfig: () => Promise<void>;
}) {
  const routingRowSeq = useRef(0);
  const [routingRows, setRoutingRows] = useState<RoutingDraftRow[]>([]);
  const disabled = !canManageRuntimeConfig || configLoading || saving;

  useEffect(() => {
    setRoutingRows(prev => {
      const currentRouting = dispatcherRoutingRowsToConfig(prev);
      if (dispatcherRoutingConfigSignature(currentRouting) === dispatcherRoutingConfigSignature(config.dispatcher_routing || {})) {
        return prev;
      }
      return Object.entries(config.dispatcher_routing || {})
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([scope, runners], index) => ({
          localId: prev[index]?.localId || `routing-${routingRowSeq.current++}`,
          scope,
          runners: (runners || []).join(', '),
        }));
    });
  }, [config.dispatcher_routing]);

  const commitRoutingRows = (nextRows: RoutingDraftRow[]) => {
    setRoutingRows(nextRows);
    onConfigChange({ ...config, dispatcher_routing: dispatcherRoutingRowsToConfig(nextRows) });
  };

  const updateRoutingScope = (localId: string, rawScope: string) => {
    commitRoutingRows(routingRows.map(row => (row.localId === localId ? { ...row, scope: rawScope } : row)));
  };

  const updateRoutingRunners = (localId: string, rawRunners: string) => {
    commitRoutingRows(routingRows.map(row => (row.localId === localId ? { ...row, runners: rawRunners } : row)));
  };

  const addRoutingRow = () => {
    const existingScopes = new Set(routingRows.map(row => normalizeDispatcherRoutingScope(row.scope)));
    let scope = '*';
    let suffix = 1;
    while (existingScopes.has(scope)) {
      scope = `scope-${suffix}`;
      suffix += 1;
    }
    commitRoutingRows([...routingRows, { localId: `routing-${routingRowSeq.current++}`, scope, runners: '' }]);
  };

  const removeRoutingRow = (localId: string) => {
    commitRoutingRows(routingRows.filter(row => row.localId !== localId));
  };

  return (
    <div className="mb-5 space-y-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm font-semibold text-[var(--text-primary)]">Configured routes</p>
            <ApplyBadge metadata={fieldMetadata.dispatcher_routing} />
          </div>
          <p className="text-xs text-[var(--text-secondary)]">Saved through runtime config and applied by the live dispatcher sync.</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canManageRuntimeConfig ? (
            <>
              <button className="glass-button-subtle" type="button" onClick={addRoutingRow} disabled={disabled}>
                <Plus className="h-4 w-4" />
                Add route
              </button>
              <button className="glass-button-primary" type="button" onClick={() => void onSaveConfig()} disabled={disabled}>
                {saving ? 'Saving...' : 'Save routes'}
              </button>
            </>
          ) : (
            <span className="runner-pill runner-pill--muted">Read-only</span>
          )}
        </div>
      </div>

      {routingRows.length > 0 ? (
        <div className="space-y-3">
          {routingRows.map(row => (
            <div key={row.localId} className="grid grid-cols-1 items-end gap-3 md:grid-cols-[minmax(0,180px)_1fr_auto]">
              <label className="flex flex-col gap-1 text-sm">
                <span>Scope</span>
                <input
                  type="text"
                  className="pipelines-input"
                  value={row.scope}
                  onChange={event => updateRoutingScope(row.localId, event.target.value)}
                  placeholder="prod"
                  disabled={disabled}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span>Runner IDs</span>
                <input
                  type="text"
                  className="pipelines-input"
                  value={row.runners}
                  onChange={event => updateRoutingRunners(row.localId, event.target.value)}
                  placeholder="runner-prod-1, runner-prod-2"
                  disabled={disabled}
                />
              </label>
              <button
                type="button"
                className="glass-button-danger md:mb-0"
                onClick={() => removeRoutingRow(row.localId)}
                disabled={disabled}
                aria-label={`Remove route ${normalizeDispatcherRoutingScope(row.scope)}`}
                title={`Remove route ${normalizeDispatcherRoutingScope(row.scope)}`}
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          ))}
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
          No dispatcher routing configured.
        </div>
      )}
    </div>
  );
}

function RunnerDeploymentGuide({ canManageDispatcher, runnerDefaults }: { canManageDispatcher: boolean; runnerDefaults: ConfigFormState }) {
  const {
    installRuntime,
    setInstallRuntime,
    runnerId,
    setRunnerId,
    setRunnerScopes,
    runnerCapacity,
    setRunnerCapacity,
    dispatcherAddress,
    setDispatcherAddress,
    runnerNetworkMode,
    setRunnerNetworkMode,
    runnerImage,
    setRunnerImage,
    kubernetesNamespace,
    setKubernetesNamespace,
    kubernetesServiceAccount,
    setKubernetesServiceAccount,
    kubernetesRunnerImage,
    setKubernetesRunnerImage,
    kubernetesStorageClass,
    setKubernetesStorageClass,
    kubernetesAffinityEnabled,
    setKubernetesAffinityEnabled,
    template,
    kubernetesTemplate,
    loadingTemplate,
    loadingKubernetesTemplate,
    templateError,
    setTemplateError,
    kubernetesTemplateError,
    setKubernetesTemplateError,
    selectedRunnerScopes,
    selectedRunnerScopeSet,
    registryCredentials,
    registryCredentialsLoading,
    registryCredentialsError,
    selectedRegistryCredentialRefs,
    selectedRegistryCredentialRefSet,
    runnerScopeChoices,
    toggleRunnerScope,
    toggleRegistryCredentialRef,
    loadTemplate,
    loadKubernetesTemplate,
  } = useRunnerDeploymentGuide(canManageDispatcher, runnerDefaults);
  const [copied, setCopied] = useState(false);
  const [copiedKubernetes, setCopiedKubernetes] = useState(false);

  const handleCopyTemplate = async () => {
    if (!template?.bootstrapCommand) return;
    try {
      await copyTextToClipboard(template.bootstrapCommand);
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
      await copyTextToClipboard(kubernetesTemplate.bootstrapCommand);
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
  const activeRegistryHosts = activeTemplate?.registryHosts || [];

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
              <label className="space-y-1.5 text-sm dispatcher-install-grid__wide">
                <span className="dispatcher-field-label">Dispatcher address override</span>
                <input
                  className="pipelines-input w-full font-mono text-xs sm:text-sm"
                  value={dispatcherAddress}
                  onChange={event => setDispatcherAddress(event.target.value)}
                  placeholder={runnerDefaults.dispatcher_grpc_address ? `auto from ${runnerDefaults.dispatcher_grpc_address}` : 'auto from system config or current host'}
                />
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
                    <div className="dispatcher-choice-team">
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

            <RegistryCredentialPicker
              credentials={registryCredentials}
              loading={registryCredentialsLoading}
              error={registryCredentialsError}
              runtime={installRuntime}
              selectedCount={selectedRegistryCredentialRefs.length}
              selectedRefs={selectedRegistryCredentialRefSet}
              onToggle={toggleRegistryCredentialRef}
            />

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
                {activeRegistryHosts.length > 0 && <RunnerFact label="Registries" value={activeRegistryHosts.join(', ')} mono />}
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

function RegistryCredentialPicker({
  credentials,
  loading,
  error,
  runtime,
  selectedCount,
  selectedRefs,
  onToggle,
}: {
  credentials: Array<{ reference: string; description?: string; metadata?: Record<string, unknown> }>;
  loading: boolean;
  error: string | null;
  runtime: RunnerInstallRuntime;
  selectedCount: number;
  selectedRefs: Set<string>;
  onToggle: (ref: string, checked: boolean) => void;
}) {
  const modeLabel = runtime === 'kubernetes' ? 'imagePullSecrets' : 'RegistryAuth';
  return (
    <div className="dispatcher-registry-auth" aria-label="Registry authentication">
      <div className="dispatcher-registry-auth__head">
        <div className="dispatcher-registry-auth__title">
          <KeyRound className="h-4 w-4" />
          <span>Registry auth</span>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <span className="runner-pill runner-pill--muted">{modeLabel}</span>
          <span className={`runner-pill ${selectedCount > 0 ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
            {selectedCount > 0 ? `${selectedCount} selected` : '0 selected'}
          </span>
        </div>
      </div>
      {loading ? (
        <div className="dispatcher-registry-auth__state">Loading registry credentials...</div>
      ) : error ? (
        <div className="dispatcher-registry-auth__state dispatcher-registry-auth__state--warning">{error}</div>
      ) : credentials.length > 0 ? (
        <div className="dispatcher-registry-options">
          {credentials.map(credential => {
            const hosts = credentialRegistryHosts(credential);
            const selected = selectedRefs.has(credential.reference);
            const label = credentialReferenceLabel(credential.reference);
            return (
              <button
                key={credential.reference}
                type="button"
                role="checkbox"
                aria-checked={selected}
                aria-label={`${selected ? 'Deselect' : 'Select'} registry credential ${label}`}
                className={`dispatcher-registry-option ${selected ? 'is-selected' : ''}`}
                onClick={() => onToggle(credential.reference, !selected)}
              >
                <span className="dispatcher-registry-option__check" aria-hidden="true">
                  {selected && <Check className="h-3.5 w-3.5" />}
                </span>
                <span className="dispatcher-registry-option__main">
                  <span className="dispatcher-registry-option__name">{label}</span>
                  <span className="dispatcher-registry-option__ref">{credential.reference}</span>
                </span>
                <span className="dispatcher-registry-option__hosts">
                  {hosts.length > 0 ? hosts.join(', ') : 'Registry host metadata pending'}
                </span>
              </button>
            );
          })}
        </div>
      ) : (
        <div className="dispatcher-registry-empty">
          <span>No active docker_config_json credentials.</span>
          <Link to="/credentials" className="runner-pill runner-pill--link">Credentials</Link>
        </div>
      )}
    </div>
  );
}

function credentialRegistryHosts(credential: { metadata?: Record<string, unknown> }): string[] {
  const hosts = credential.metadata?.registry_hosts;
  if (!Array.isArray(hosts)) return [];
  return hosts.map(host => String(host || '').trim()).filter(Boolean);
}

function credentialReferenceLabel(reference: string): string {
  const body = reference.trim().replace(/^credential:\/\//i, '');
  const parts = body.split('/').filter(Boolean);
  return parts.at(-1) || body || reference;
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
  pendingEjections,
  onToggleDispatch,
  onEjectRunner,
  canManageDispatcher,
}: {
  nowMs: number;
  runner: Runner;
  pendingActions: Set<string>;
  pendingEjections: Set<string>;
  onToggleDispatch: (runner: Runner) => Promise<void>;
  onEjectRunner: (runner: Runner) => Promise<void>;
  canManageDispatcher: boolean;
}) {
  const meta = getRunnerMeta(runner);
  const unreachable = !meta.reachable;
  const stale = !unreachable && isStale(nowMs, runner.lastHeartbeatUnix);
  const paused = !runner.allowDispatch;
  const statusClass = unreachable ? 'runner-dot--warning' : paused ? 'runner-dot--warning' : stale ? 'runner-dot--error' : 'runner-dot--ok';
  const statusLabel = unreachable ? 'Unreachable' : paused ? 'Dispatch paused' : stale ? 'Heartbeat stale' : 'Online';
  const statusTone = unreachable ? 'runner-pill--warning' : paused ? 'runner-pill--warning' : stale ? 'runner-pill--error' : 'runner-pill--ok';

  const runtimeLabel = meta.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker';
  const runtimeTone = meta.runtime === 'kubernetes' ? 'runner-pill--ok' : 'runner-pill--muted';
  const connectionLabel = formatConnection(meta.connectionId);
  const pendingKey = runnerActionKey(runner.runnerId, meta.connectionId);
  const pendingDispatch = Boolean(pendingKey && pendingActions.has(pendingKey));
  const pendingEject = Boolean(pendingKey && pendingEjections.has(pendingKey));
  const pending = pendingDispatch || pendingEject;

  const toggleLabel = paused ? 'Resume dispatch' : 'Pause dispatch';
  const toggleTone = paused ? 'glass-button-primary' : 'glass-button-danger';
  const actionLabel = pendingDispatch ? (paused ? 'Resuming…' : 'Pausing…') : toggleLabel;
  const ejectLabel = pendingEject ? 'Ejecting…' : 'Eject';
  const scopesLabel = runner.scopes.length ? runner.scopes.join(', ') : 'All scopes';
  const activeRunCount = meta.activeRuns.length;

  return (
    <div className={`runner-card p-5 ${unreachable ? 'runner-card--unreachable' : paused ? 'runner-card--paused' : stale ? 'runner-card--stale' : ''}`}>
      <div className="runner-card__header">
        <div className="runner-card__title">
          <span className={`runner-dot ${statusClass}`} aria-hidden="true"></span>
          <div className="runner-card__title-stack">
            <div className={`runner-name ${paused && !unreachable ? 'runner-name--paused' : ''}`}>{runner.runnerId}</div>
            <div className="runner-card__badges">
              <span className={`runner-pill ${statusTone}`}>{statusLabel}</span>
              {unreachable && paused && <span className="runner-pill runner-pill--muted">Dispatch paused</span>}
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
          <button
            type="button"
            className={`glass-button-danger text-xs ${pending ? 'opacity-60 cursor-wait' : ''}`}
            disabled={!canManageDispatcher || pending}
            onClick={() => void onEjectRunner(runner)}
          >
            <Trash2 className="h-4 w-4" />
            {ejectLabel}
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
        {unreachable && meta.disconnectedAt && <RunnerFact label="Disconnected" value={formatTimestamp(meta.disconnectedAt)} />}
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
              const to = `/pipelineruns/recent?run=${encodeURIComponent(run.runId)}`;
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

function RoutingMap({
  routing,
  effectiveRouting,
  runners,
}: {
  routing: Record<string, string[]>;
  effectiveRouting: Record<string, string[]>;
  runners: Runner[];
}) {
  const routeMap = Object.keys(effectiveRouting || {}).length > 0 ? effectiveRouting : routing;
  const rows = Object.entries(routeMap || {})
    .map(([scope, runners]) => ({
      scope: (scope || '*').trim() || '*',
      runners: Array.isArray(runners) && runners.length ? runners.map(r => (r || '*').trim() || '*') : ['*'],
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));
  const liveRows = buildLiveRunnerRoutingRows(runners);
  const reachableRunnerIds = new Set(runners.filter(runner => getRunnerMeta(runner).reachable).map(runner => runner.runnerId).filter(Boolean));

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
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Effective routing</p>
          {rows.map(row => (
            <div
              key={row.scope}
              className="flex items-center justify-between gap-3 bg-[var(--bg-tertiary)] px-3 py-2 rounded-md border border-[var(--border-primary)]"
            >
              <span className="runner-pill runner-pill--ok">{formatDispatcherRouteScope(row.scope)}</span>
              <div className="flex flex-wrap gap-2 justify-end text-sm">
                {row.runners.map(runnerId => {
                  const connected = reachableRunnerIds.has(runnerId);
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
              <span className="runner-pill runner-pill--ok">{formatDispatcherRouteScope(row.scope)}</span>
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
      ) : (
        <div id="dispatcher-routing-live-empty" className="text-sm text-[var(--text-secondary)]">
          No live runner scopes.
        </div>
      )}
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


function isStale(nowMs: number, lastHeartbeatUnix: number) {
  if (!lastHeartbeatUnix) return true;
  return nowMs - lastHeartbeatUnix * 1000 > STALE_THRESHOLD_MS;
}


export default DispatcherPanel;
