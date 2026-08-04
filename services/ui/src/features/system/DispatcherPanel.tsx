import { useLocation } from 'react-router-dom';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { LayoutDashboard, Plus, Route, Server, Trash2 } from 'lucide-react';
import type { ConfigFieldMetadata, ConfigFormState } from './config/model';
import { ApplyBadge } from './config/ConfigApplyBadge';
import { DispatcherOverview } from './dispatcher/DispatcherOverview';
import { RunnerDeploymentGuide } from './dispatcher/RunnerDeploymentGuide';
import { RunnerFleetWorkspace } from './dispatcher/RunnerFleetWorkspace';
import {
  buildLiveRunnerRoutingRows,
  dispatcherRoutingConfigSignature,
  dispatcherRoutingRowsToConfig,
  formatDispatcherRouteScope,
  getRunnerMeta,
  normalizeDispatcherRoutingScope,
  type DispatcherRoutingDraftRow,
  type DispatcherStatusState,
  type Runner,
} from './dispatcher/model';
import { isRunnerRecentlyDisconnected } from './dispatcher/presentation';

const RUNNER_DEPLOYMENT_GUIDE_QUERY = 'guide';
const RUNNER_DEPLOYMENT_GUIDE_VALUE = 'runner';

type DispatcherPageTab = 'overview' | 'runners' | 'routing' | 'install';

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
  onRefresh: () => void;
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
  const location = useLocation();
  const initialPage = new URLSearchParams(location.search).get(RUNNER_DEPLOYMENT_GUIDE_QUERY) === RUNNER_DEPLOYMENT_GUIDE_VALUE
    ? 'install'
    : 'overview';
  const [activePage, setActivePage] = useState<DispatcherPageTab>(initialPage);
  const runners = status?.runners ?? [];
  const runnerCount = runners.length;
  const routeSource = status?.effectiveRouting && Object.keys(status.effectiveRouting).length > 0
    ? status.effectiveRouting
    : status?.routing ?? config.dispatcher_routing ?? {};
  const routeCount = Object.keys(routeSource).length;
  const nowMs = status?.fetchedAt ?? 0;
  const tabs: Array<{ id: DispatcherPageTab; label: string; count?: number; icon: ReactNode }> = [
    { id: 'overview', label: 'Overview', icon: <LayoutDashboard className="h-4 w-4" /> },
    { id: 'runners', label: 'Runners', count: runnerCount, icon: <Server className="h-4 w-4" /> },
    { id: 'routing', label: 'Routing', count: routeCount, icon: <Route className="h-4 w-4" /> },
    { id: 'install', label: 'Install runner', icon: <Plus className="h-4 w-4" /> },
  ];

  return (
    <div id="system-dispatcher-section" className="dispatcher-workspace">
      {error && <p className="dispatcher-error">Failed to load dispatcher status: {error}</p>}
      {status?.dispatcherError && <p className="dispatcher-error">Runner data is unavailable while dispatcher status cannot be loaded.</p>}

      <div className="dispatcher-page-tabs" role="tablist" aria-label="Dispatcher sections">
        {tabs.map(tab => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={activePage === tab.id}
            aria-controls={`dispatcher-${tab.id}-panel`}
            className={activePage === tab.id ? 'is-active' : ''}
            onClick={() => setActivePage(tab.id)}
          >
            {tab.icon}
            <span>{tab.label}</span>
            {typeof tab.count === 'number' && <span className="dispatcher-tab-count">{tab.count}</span>}
          </button>
        ))}
      </div>

      {activePage === 'overview' && (
        <div id="dispatcher-overview-panel" role="tabpanel" aria-label="Dispatcher overview">
          <DispatcherOverview
            loading={loading}
            status={status}
            nowMs={nowMs}
            onOpenRunners={() => setActivePage('runners')}
            onOpenRouting={() => setActivePage('routing')}
          />
        </div>
      )}

      {activePage === 'runners' && (
        <div id="dispatcher-runners-panel" role="tabpanel" aria-label="Dispatcher runners">
          <RunnerFleetWorkspace
            runners={runners}
            nowMs={nowMs}
            pendingActions={pendingActions}
            pendingEjections={pendingEjections}
            onToggleRunnerDispatch={onToggleRunnerDispatch}
            onEjectRunner={onEjectRunner}
            canManageDispatcher={canManageDispatcher}
          />
        </div>
      )}

      {activePage === 'routing' && (
        <div id="dispatcher-routing-panel" className="dispatcher-routing-layout" role="tabpanel" aria-label="Dispatcher routing">
          <section className="dispatcher-section-card">
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
            <RoutingMap routing={status?.routing ?? {}} effectiveRouting={status?.effectiveRouting ?? {}} runners={runners} nowMs={nowMs} />
          </section>
        </div>
      )}

      {activePage === 'install' && (
        <div id="dispatcher-install-panel" role="tabpanel" aria-label="Install dispatcher runner">
          <RunnerDeploymentGuide canManageDispatcher={canManageDispatcher} runnerDefaults={runnerDefaults} />
        </div>
      )}
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
    <div className="dispatcher-routing-editor">
      <div className="dispatcher-routing-editor__head">
        <div>
          <div className="dispatcher-routing-editor__title">
            <strong>Configured routes</strong>
            <ApplyBadge metadata={fieldMetadata.dispatcher_routing} />
          </div>
        </div>
        <div className="dispatcher-routing-actions">
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
        <div className="dispatcher-table-wrap">
          <table className="dispatcher-route-edit-table">
            <thead>
              <tr>
                <th>Scope</th>
                <th>Runner IDs</th>
                <th>Fallback</th>
                <th><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {routingRows.map(row => {
                const normalizedScope = normalizeDispatcherRoutingScope(row.scope);
                return (
                  <tr key={row.localId}>
                    <td>
                      <label>
                        <span className="sr-only">Scope</span>
                        <input
                          type="text"
                          className="pipelines-input"
                          value={row.scope}
                          onChange={event => updateRoutingScope(row.localId, event.target.value)}
                          placeholder="prod"
                          disabled={disabled}
                        />
                      </label>
                    </td>
                    <td>
                      <label>
                        <span className="sr-only">Runner IDs</span>
                        <input
                          type="text"
                          className="pipelines-input"
                          value={row.runners}
                          onChange={event => updateRoutingRunners(row.localId, event.target.value)}
                          placeholder="runner-prod-1, runner-prod-2"
                          disabled={disabled}
                        />
                      </label>
                    </td>
                    <td>
                      <span className="runner-pill runner-pill--muted">
                        {normalizedScope === '*' || normalizedScope === 'default' ? 'Queue' : 'Default'}
                      </span>
                    </td>
                    <td>
                      <div className="dispatcher-row-actions">
                        <button
                          type="button"
                          className="compact-resource-card__action compact-resource-card__action--danger"
                          onClick={() => removeRoutingRow(row.localId)}
                          disabled={disabled}
                          aria-label={`Remove route ${normalizedScope}`}
                          title={`Remove route ${normalizedScope}`}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="dispatcher-empty dispatcher-empty--center">
          No dispatcher routing configured.
        </div>
      )}
    </div>
  );
}

function RoutingMap({
  routing,
  effectiveRouting,
  runners,
  nowMs,
}: {
  routing: Record<string, string[]>;
  effectiveRouting: Record<string, string[]>;
  runners: Runner[];
  nowMs: number;
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
  const degradedRunnerIds = new Set(runners.filter(runner => {
    const meta = getRunnerMeta(runner);
    return meta.reachable && isRunnerRecentlyDisconnected(nowMs, meta.disconnectedAt);
  }).map(runner => runner.runnerId).filter(Boolean));

  if (rows.length === 0 && liveRows.length === 0) {
    return (
      <div id="dispatcher-routing-empty" className="text-sm text-[var(--text-secondary)]">
        No routing rules configured.
      </div>
    );
  }

  return (
    <div id="dispatcher-routing" className="dispatcher-routing-map">
      {rows.length > 0 ? (
        <RoutingMapTable
          title="Effective routing"
          rows={rows}
          reachableRunnerIds={reachableRunnerIds}
          degradedRunnerIds={degradedRunnerIds}
          targetTone="availability"
        />
      ) : null}
      {liveRows.length > 0 ? (
        <RoutingMapTable
          title="Live runner scopes"
          rows={liveRows}
          reachableRunnerIds={reachableRunnerIds}
          degradedRunnerIds={degradedRunnerIds}
          targetTone="muted"
        />
      ) : (
        <div id="dispatcher-routing-live-empty" className="text-sm text-[var(--text-secondary)]">
          No live runner scopes.
        </div>
      )}
    </div>
  );
}

function RoutingMapTable({
  title,
  rows,
  reachableRunnerIds,
  degradedRunnerIds,
  targetTone,
}: {
  title: string;
  rows: Array<{ scope: string; runners: string[] }>;
  reachableRunnerIds: Set<string>;
  degradedRunnerIds: Set<string>;
  targetTone: 'availability' | 'muted';
}) {
  return (
    <div className="dispatcher-route-map-block">
      <p className="dispatcher-route-map-title">{title}</p>
      <div className="dispatcher-table-wrap">
        <table className="dispatcher-route-map-table">
          <thead>
            <tr>
              <th>Scope</th>
              <th>Runner pool</th>
              <th>Health</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(row => {
              const health = routeHealth(row.runners, reachableRunnerIds, degradedRunnerIds);
              return (
                <tr key={`${title}-${row.scope}`}>
                  <td>
                    <div className="dispatcher-route-scope">
                      <span>{routeScopeInitial(row.scope)}</span>
                      <strong>{formatDispatcherRouteScope(row.scope)}</strong>
                    </div>
                  </td>
                  <td>
                    <div className="dispatcher-route-targets">
                      {row.runners.map(runnerId => {
                        const reachable = runnerId === '*' || reachableRunnerIds.has(runnerId);
                        const degraded = runnerId !== '*' && degradedRunnerIds.has(runnerId);
                        return (
                          <span
                            key={`${title}-${row.scope}-${runnerId}`}
                            className={`dispatcher-route-target ${targetTone === 'muted' ? 'dispatcher-route-target--muted' : reachable && !degraded ? 'dispatcher-route-target--ok' : 'dispatcher-route-target--warning'}`}
                          >
                            <span aria-hidden="true"></span>
                            {runnerId}
                          </span>
                        );
                      })}
                    </div>
                  </td>
                  <td>
                    <span className={`runner-pill ${health.tone}`}>{targetTone === 'muted' ? 'Registered' : health.label}</span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function routeHealth(runnerIds: string[], reachableRunnerIds: Set<string>, degradedRunnerIds: Set<string>) {
  const reachableCount = runnerIds.filter(runnerId => runnerId === '*' || reachableRunnerIds.has(runnerId)).length;
  const degradedCount = runnerIds.filter(runnerId => runnerId !== '*' && degradedRunnerIds.has(runnerId)).length;
  if (reachableCount === runnerIds.length && degradedCount === 0) return { label: 'Healthy', tone: 'runner-pill--ok' };
  if (reachableCount > 0) return { label: 'Degraded', tone: 'runner-pill--warning' };
  return { label: 'Unavailable', tone: 'runner-pill--error' };
}

function routeScopeInitial(scope: string) {
  const formatted = formatDispatcherRouteScope(scope);
  return formatted === 'Default' ? '*' : formatted.slice(0, 1).toUpperCase();
}

export default DispatcherPanel;
