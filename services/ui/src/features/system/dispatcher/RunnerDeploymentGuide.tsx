import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Boxes, Check, Copy, Download, Info, KeyRound, RefreshCw, Server, Terminal } from 'lucide-react';
import { copyTextToClipboard } from '../../../lib/clipboard';
import type { ConfigFormState } from '../config/model';
import type { RunnerInstallRuntime } from './model';
import { formatTimestamp } from './presentation';
import { useRunnerDeploymentGuide } from './useRunnerDeploymentGuide';

export const RUNNER_DEPLOYMENT_GUIDE_ID = 'dispatcher-runner-deployment-guide';

export function RunnerDeploymentGuide({ canManageDispatcher, runnerDefaults }: { canManageDispatcher: boolean; runnerDefaults: ConfigFormState }) {
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
  const runtimeLabel = isKubernetesInstall ? 'Kubernetes' : 'Docker';
  const scopeSummary = selectedRunnerScopes.length > 0 ? selectedRunnerScopes.join(', ') : 'All scopes';
  const commandStatusClass = activeCommand
    ? 'runner-pill runner-pill--ok'
    : activeLoading
      ? 'runner-pill runner-pill--warning'
      : 'runner-pill runner-pill--muted';
  const commandStatusLabel = activeCommand ? 'Command ready' : activeLoading ? 'Generating' : 'Not generated';
  const copiedActiveCommand = isKubernetesInstall ? copiedKubernetes : copied;
  const generateCommand = () => void (isKubernetesInstall ? loadKubernetesTemplate() : loadTemplate());
  const copyActiveCommand = () => void (isKubernetesInstall ? handleCopyKubernetesInstallCommand() : handleCopyTemplate());

  return (
    <section id={RUNNER_DEPLOYMENT_GUIDE_ID} className="dispatcher-install scroll-mt-6">
      <div className="dispatcher-install-shell">
        <div className="dispatcher-install-config">
          <section className="dispatcher-section-card dispatcher-install-panel">
            <div className="dispatcher-section-header">
              <div>
                <h3>Install a runner</h3>
                <p>Choose a runtime, configure placement, then generate a one-time install command.</p>
              </div>
              <span className="runner-pill runner-pill--muted">New runner</span>
            </div>
            <InstallSectionHeading index={1} title="Runtime" note="The runtime controls which deployment parameters and install command are generated." />
            <div className="dispatcher-runtime-picker" role="tablist" aria-label="Runner install runtime">
              {(['docker', 'kubernetes'] as const).map(runtime => {
                const selected = installRuntime === runtime;
                return (
                  <button
                    key={runtime}
                    type="button"
                    className={`dispatcher-runtime-tab ${selected ? 'is-active' : ''}`}
                    aria-pressed={selected}
                    onClick={() => setInstallRuntime(runtime)}
                  >
                    <span className="dispatcher-runtime-tab__icon">
                      {runtime === 'docker' ? <Server className="h-5 w-5" /> : <Boxes className="h-5 w-5" />}
                    </span>
                    <span className="dispatcher-runtime-tab__copy">
                      <strong>{runtime === 'docker' ? 'Docker' : 'Kubernetes'}</strong>
                      <small>{runtime === 'docker' ? 'Install on a host using a single docker run command.' : 'Install into a namespace using Helm and cluster RBAC.'}</small>
                    </span>
                  </button>
                );
              })}
            </div>
          </section>

          {canManageDispatcher ? (
            <section className="dispatcher-section-card dispatcher-install-panel">
              <div className="dispatcher-config-section">
                <InstallSectionHeading index={2} title="Identity and capacity" note="Use a stable name so this runner is easy to recognize in the fleet." />
                <div className="dispatcher-install-grid">
                  <label className="dispatcher-field">
                    <span className="dispatcher-field-label">Runner name</span>
                    <input aria-label="Runner name" className="pipelines-input w-full" value={runnerId} onChange={event => setRunnerId(event.target.value)} />
                    <span className="dispatcher-field-hint">Unique within this dispatcher.</span>
                  </label>
                  <label className="dispatcher-field">
                    <span className="dispatcher-field-label">Capacity</span>
                    <input aria-label="Capacity" className="pipelines-input w-full" type="number" min="1" value={runnerCapacity} onChange={event => setRunnerCapacity(event.target.value)} />
                    <span className="dispatcher-field-hint">Maximum runs accepted at once.</span>
                  </label>
                </div>
              </div>

              <div className="dispatcher-config-section">
                <InstallSectionHeading index={3} title="Dispatch placement" note="Select where this runner can receive work. The dispatcher address is normally discovered automatically." />
                <div className="dispatcher-install-grid">
                  <div className="dispatcher-field dispatcher-install-grid__wide">
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
                  <label className="dispatcher-field dispatcher-install-grid__wide">
                    <span className="dispatcher-field-label">Dispatcher address override</span>
                    <input
                      aria-label="Dispatcher address override"
                      className="pipelines-input w-full font-mono text-xs sm:text-sm"
                      value={dispatcherAddress}
                      onChange={event => setDispatcherAddress(event.target.value)}
                      placeholder={runnerDefaults.dispatcher_grpc_address ? `auto from ${runnerDefaults.dispatcher_grpc_address}` : 'auto from system config or current host'}
                    />
                    <span className="dispatcher-field-hint">Leave empty unless the runner reaches the dispatcher through a different hostname or port.</span>
                  </label>
                </div>
              </div>

              <div className="dispatcher-config-section">
                <InstallSectionHeading index={4} title="Runtime configuration" note="Only parameters relevant to the selected runtime are shown." />
                <div className="dispatcher-install-grid">
                  {isKubernetesInstall ? (
                    <>
                      <label className="dispatcher-field">
                        <span className="dispatcher-field-label">Namespace</span>
                        <input aria-label="Namespace" className="pipelines-input w-full" value={kubernetesNamespace} onChange={event => setKubernetesNamespace(event.target.value)} />
                      </label>
                      <label className="dispatcher-field">
                        <span className="dispatcher-field-label">Service account</span>
                        <input aria-label="Service account" className="pipelines-input w-full" value={kubernetesServiceAccount} onChange={event => setKubernetesServiceAccount(event.target.value)} />
                      </label>
                      <label className="dispatcher-field">
                        <span className="dispatcher-field-label">Storage class</span>
                        <input aria-label="Storage class" className="pipelines-input w-full" value={kubernetesStorageClass} onChange={event => setKubernetesStorageClass(event.target.value)} placeholder="cluster default" />
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
                      <label className="dispatcher-field dispatcher-install-grid__wide">
                        <span className="dispatcher-field-label">Runner image</span>
                        <input aria-label="Runner image" className="pipelines-input w-full font-mono text-xs sm:text-sm" value={kubernetesRunnerImage} onChange={event => setKubernetesRunnerImage(event.target.value)} />
                      </label>
                    </>
                  ) : (
                    <>
                      <div className="dispatcher-field">
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
                        <span className="dispatcher-field-hint">Host is simplest when dispatcher and runner share the machine.</span>
                      </div>
                      <label className="dispatcher-field dispatcher-install-grid__wide">
                        <span className="dispatcher-field-label">Runner image</span>
                        <input aria-label="Runner image" className="pipelines-input w-full font-mono text-xs sm:text-sm" value={runnerImage} onChange={event => setRunnerImage(event.target.value)} />
                      </label>
                    </>
                  )}
                </div>
              </div>

              <div className="dispatcher-config-section">
                <InstallSectionHeading index={5} title="Private registry access" note="Attach pull credentials only when the selected runner image requires them." />
                <RegistryCredentialPicker
                  credentials={registryCredentials}
                  loading={registryCredentialsLoading}
                  error={registryCredentialsError}
                  runtime={installRuntime}
                  selectedCount={selectedRegistryCredentialRefs.length}
                  selectedRefs={selectedRegistryCredentialRefSet}
                  onToggle={toggleRegistryCredentialRef}
                />
              </div>
            </section>
          ) : (
            <section className="dispatcher-section-card">
              <div className="dispatcher-empty">
                Dispatcher management access is required to generate runner install commands.
              </div>
            </section>
          )}
        </div>

        <aside className="dispatcher-section-card dispatcher-install-review">
          <div className="dispatcher-section-header">
            <div>
              <h3>Review and generate</h3>
              <p>The one-time credential is embedded directly in the generated command.</p>
            </div>
            <span className={commandStatusClass}>{commandStatusLabel}</span>
          </div>
          <div className="dispatcher-review-summary">
            <InstallSummaryRow label="Runtime" value={runtimeLabel} />
            <InstallSummaryRow label="Runner" value={runnerId || 'Name required'} />
            <InstallSummaryRow label="Scopes" value={scopeSummary} />
            <InstallSummaryRow label="Capacity" value={`${runnerCapacity || '1'} slot${String(runnerCapacity) === '1' ? '' : 's'}`} />
            <InstallSummaryRow label="Dispatcher" value={dispatcherAddress || activeDispatcherAddress || 'Auto-detected'} />
          </div>

          <div className="dispatcher-install-note">
            <Info className="h-4 w-4" />
            <span>The command is shown only after generation. Its embedded credential can be used once and expires after 30 minutes.</span>
          </div>

          <button
            type="button"
            className="glass-button-primary dispatcher-generate-button"
            onClick={generateCommand}
            disabled={!canManageDispatcher || activeLoading}
          >
            {activeLoading ? <RefreshCw className="h-4 w-4 animate-spin" /> : activeCommand ? <RefreshCw className="h-4 w-4" /> : <Download className="h-4 w-4" />}
            {activeLoading ? 'Generating...' : activeCommand ? 'Generate a new command' : 'Generate command'}
          </button>

          {activeError && <p className="dispatcher-install-error">{activeError}</p>}

          <div className="dispatcher-command-state">
            {activeCommand ? (
              <div className="dispatcher-command-box">
                <div className="dispatcher-command-head">
                  <div className="dispatcher-command-title">
                    <span className="runner-pill runner-pill--ok">Ready</span>
                    <span>{runtimeLabel} command</span>
                  </div>
                  <button
                    type="button"
                    className="glass-button-subtle"
                    onClick={copyActiveCommand}
                    disabled={activeLoading}
                  >
                    <Copy className="h-4 w-4" />
                    {copiedActiveCommand ? 'Copied' : 'Copy'}
                  </button>
                </div>
                <pre className="dispatcher-install-command">
                  <code>{activeCommand}</code>
                </pre>
                <div className="dispatcher-command-meta">
                  Generated command uses a single-use credential.
                  {activeExpiresAt ? ` Expires ${formatTimestamp(activeExpiresAt)}.` : ''}
                </div>
              </div>
            ) : (
              <div className="dispatcher-command-empty">
                <div>
                  <span className="dispatcher-command-empty__icon">
                    <Terminal className="h-5 w-5" />
                  </span>
                  <strong>No command generated</strong>
                  <p>Review the parameters above, then generate the one-time command when you are ready to install the runner.</p>
                </div>
              </div>
            )}
          </div>

          {(activeCommand && (activeDispatcherAddress || activeRunnerImage || (!isKubernetesInstall && template?.networkMode) || (isKubernetesInstall && kubernetesTemplate?.namespace) || activeRegistryHosts.length > 0)) && (
            <div className="dispatcher-install-meta">
              {activeDispatcherAddress && <RunnerFact label="Dispatcher" value={activeDispatcherAddress} mono />}
              {!isKubernetesInstall && template?.networkMode && <RunnerFact label="Network mode" value={template.networkMode} />}
              {isKubernetesInstall && kubernetesTemplate?.namespace && <RunnerFact label="Namespace" value={kubernetesTemplate.namespace} mono />}
              {activeRunnerImage && <RunnerFact label="Image" value={activeRunnerImage} mono />}
              {activeRegistryHosts.length > 0 && <RunnerFact label="Registries" value={activeRegistryHosts.join(', ')} mono />}
            </div>
          )}

          {activeWarnings.map(warning => (
            <div key={warning} className="dispatcher-install-warning">
              {warning}
            </div>
          ))}
        </aside>
      </div>
    </section>
  );
}

function InstallSectionHeading({ index, title, note }: { index: number; title: string; note: string }) {
  return (
    <div className="dispatcher-install-section-head">
      <span>{index}</span>
      <div>
        <strong>{title}</strong>
        <p>{note}</p>
      </div>
    </div>
  );
}

function InstallSummaryRow({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="dispatcher-review-row">
      <span>{label}</span>
      <b>{value}</b>
    </div>
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
