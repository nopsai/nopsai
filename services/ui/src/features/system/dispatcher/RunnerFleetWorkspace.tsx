import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { PauseCircle, PlayCircle, Trash2 } from 'lucide-react';
import {
  getRunnerMeta,
  runnerActionKey,
  type Runner,
} from './model';
import { clampPercent, formatConnection, formatSince, formatTimestamp, isRunnerHeartbeatStale, isRunnerRecentlyDisconnected, truncateId } from './presentation';

type RunnerFleetWorkspaceProps = {
  runners: Runner[];
  nowMs: number;
  pendingActions: Set<string>;
  pendingEjections: Set<string>;
  canManageDispatcher: boolean;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
  onEjectRunner: (runner: Runner) => Promise<void>;
};

type RunnerFleetStatus = 'online' | 'busy' | 'paused' | 'offline' | 'recovered';
type RunnerRuntimeFilter = 'docker' | 'kubernetes';
type RunnerDetailTab = 'workloads' | 'metadata' | 'logs';

type RunnerFleetRow = {
  key: string;
  runner: Runner;
  status: RunnerFleetStatus;
  statusLabel: string;
  statusTone: string;
  dotClass: string;
  runtime: 'docker' | 'kubernetes';
  hostLabel: string;
  heartbeatLabel: string;
  capacityPercent: number;
  pendingDispatch: boolean;
  pendingEject: boolean;
  pending: boolean;
};

export function RunnerFleetWorkspace({
  runners,
  nowMs,
  pendingActions,
  pendingEjections,
  canManageDispatcher,
  onToggleRunnerDispatch,
  onEjectRunner,
}: RunnerFleetWorkspaceProps) {
  const [runtimeFilter, setRuntimeFilter] = useState<RunnerRuntimeFilter>('docker');
  const [selectedRunnerKey, setSelectedRunnerKey] = useState('');
  const [detailTab, setDetailTab] = useState<RunnerDetailTab>('workloads');

  const rows = useMemo(
    () => runners.map(runner => buildRunnerFleetRow(runner, nowMs, pendingActions, pendingEjections)),
    [nowMs, pendingActions, pendingEjections, runners]
  );
  const filteredRows = useMemo(() => {
    return rows.filter(row => row.runtime === runtimeFilter);
  }, [rows, runtimeFilter]);
  const selectedRow = filteredRows.find(row => row.key === selectedRunnerKey) || null;

  const handleSelectRow = (row: RunnerFleetRow) => {
    setSelectedRunnerKey(row.key);
    setDetailTab('workloads');
  };

  return (
    <div className="dispatcher-runner-layout">
      <section className="dispatcher-section-card dispatcher-section-card--flush">
        <div className="dispatcher-toolbar dispatcher-toolbar--runner">
          <div className="dispatcher-runtime-filter" role="group" aria-label="Runner runtime">
            {(['docker', 'kubernetes'] as const).map(runtime => (
              <button
                key={runtime}
                type="button"
                className={runtimeFilter === runtime ? 'is-active' : ''}
                aria-pressed={runtimeFilter === runtime}
                onClick={() => {
                  setRuntimeFilter(runtime);
                  setSelectedRunnerKey('');
                }}
              >
                {runtime === 'docker' ? 'Docker' : 'Kubernetes'}
              </button>
            ))}
          </div>
          <span className="dispatcher-result-count"><b>{filteredRows.length}</b> visible</span>
        </div>

        <div className="dispatcher-table-wrap">
          <table className="dispatcher-fleet-table">
            <thead>
              <tr>
                <th>Runner</th>
                <th>Status</th>
                <th>Runtime</th>
                <th>Scopes</th>
                <th>Capacity</th>
                <th>Heartbeat</th>
                <th><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {filteredRows.map(row => (
                <tr
                  key={row.key}
                  className={selectedRow?.key === row.key ? 'is-selected' : ''}
                  tabIndex={0}
                  onClick={() => handleSelectRow(row)}
                  onKeyDown={event => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      handleSelectRow(row);
                    }
                  }}
                >
                  <td>
                    <div className="dispatcher-runner-cell">
                      <span className={`runner-dot ${row.dotClass}`} aria-hidden="true"></span>
                      <div>
                        <div className="dispatcher-runner-cell__name">{row.runner.runnerId}</div>
                        <div className="dispatcher-runner-cell__host">{row.hostLabel || 'Host metadata pending'}</div>
                      </div>
                    </div>
                  </td>
                  <td><span className={`runner-pill ${row.statusTone}`}>{row.statusLabel}</span></td>
                  <td>{row.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker'}</td>
                  <td>
                    <div className="dispatcher-scope-tags">
                      {(row.runner.scopes.length ? row.runner.scopes : ['All scopes']).map(scope => (
                        <span key={`${row.key}-${scope}`}>{scope}</span>
                      ))}
                    </div>
                  </td>
                  <td>
                    <div className="dispatcher-capacity-cell">
                      <div>
                        <span>{row.runner.activeJobs} / {row.runner.capacity}</span>
                        <span>{row.capacityPercent}%</span>
                      </div>
                      <div className="dispatcher-capacity-bar">
                        <span style={{ width: `${row.capacityPercent}%` }} />
                      </div>
                    </div>
                  </td>
                  <td>{row.heartbeatLabel}</td>
                  <td>
                    <div className="dispatcher-row-actions">
                      <button
                        className={row.runner.allowDispatch ? 'compact-resource-card__action compact-resource-card__action--danger' : 'compact-resource-card__action'}
                        type="button"
                        title={row.runner.allowDispatch ? 'Pause dispatch' : 'Resume dispatch'}
                        aria-label={`${row.runner.allowDispatch ? 'Pause dispatch for' : 'Resume dispatch for'} ${row.runner.runnerId}`}
                        disabled={!canManageDispatcher || row.pending}
                        onClick={event => {
                          event.stopPropagation();
                          void onToggleRunnerDispatch(row.runner);
                        }}
                      >
                        {row.runner.allowDispatch ? <PauseCircle className="h-4 w-4" /> : <PlayCircle className="h-4 w-4" />}
                      </button>
                      <button
                        className="compact-resource-card__action compact-resource-card__action--danger"
                        type="button"
                        title="Remove runner registration"
                        aria-label={`Remove ${row.runner.runnerId}`}
                        disabled={!canManageDispatcher || row.pending}
                        onClick={event => {
                          event.stopPropagation();
                          void onEjectRunner(row.runner);
                        }}
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {filteredRows.length === 0 && (
            <div className="dispatcher-empty dispatcher-empty--center">
              <strong>No {runtimeFilter === 'docker' ? 'Docker' : 'Kubernetes'} runners registered</strong>
            </div>
          )}
        </div>
      </section>

      {selectedRow && (
        <RunnerDetailPanel
          row={selectedRow}
          detailTab={detailTab}
          setDetailTab={setDetailTab}
        />
      )}
    </div>
  );
}

function RunnerDetailPanel({
  row,
  detailTab,
  setDetailTab,
}: {
  row: RunnerFleetRow | null;
  detailTab: RunnerDetailTab;
  setDetailTab: (tab: RunnerDetailTab) => void;
}) {
  if (!row) {
    return null;
  }

  const meta = getRunnerMeta(row.runner);
  const connectionLabel = formatConnection(meta.connectionId);
  const activeRuns = meta.activeRuns;

	  return (
	    <aside className="dispatcher-section-card dispatcher-runner-detail">
	      <div className="dispatcher-detail-headline">
	        <div>
	          <div className="dispatcher-detail-name">{meta.runnerName || row.runner.runnerId}</div>
	          {meta.runnerName && meta.runnerName !== row.runner.runnerId && <div className="dispatcher-detail-host font-mono">{row.runner.runnerId}</div>}
	          <div className="dispatcher-detail-host">{row.hostLabel || 'Host metadata pending'} / {row.runtime === 'kubernetes' ? 'Kubernetes' : 'Docker'}</div>
        </div>
        <span className={`runner-pill ${row.statusTone}`}>
          <span className={`runner-dot ${row.dotClass}`} aria-hidden="true"></span>
          {row.statusLabel}
        </span>
      </div>

      <div className="dispatcher-detail-stats">
        <DetailStat label="Active runs" value={row.runner.activeJobs} />
        <DetailStat label="Capacity" value={row.runner.capacity} />
        <DetailStat label="Heartbeat" value={row.heartbeatLabel} />
      </div>

      <div className="dispatcher-subtabs" role="tablist" aria-label="Runner detail sections">
        {(['workloads', 'metadata', 'logs'] as const).map(tab => (
          <button
            key={tab}
            type="button"
            role="tab"
            aria-selected={detailTab === tab}
            className={detailTab === tab ? 'is-active' : ''}
            onClick={() => setDetailTab(tab)}
          >
            {tab[0].toUpperCase() + tab.slice(1)}
          </button>
        ))}
      </div>

      {detailTab === 'workloads' && (
        <div className="dispatcher-detail-list">
          {activeRuns.length > 0 ? (
            activeRuns.map(run => (
              <div className="dispatcher-work-item" key={run.runId}>
                <div>
                  <strong>{run.pipeline || 'Pipeline run'}</strong>
                  <small>{run.parentStep || 'agent-execution'} / {run.triggerId || 'manual'}</small>
                </div>
                <Link className="runner-pill runner-pill--link" to={`/pipelineruns/recent?run=${encodeURIComponent(run.runId)}`}>
                  {truncateId(run.runId)}
                </Link>
              </div>
            ))
          ) : (
            <div className="dispatcher-work-item">
              <div>
                <strong>No active workloads</strong>
                <small>{row.status === 'online' ? `This runner can accept ${Math.max(0, row.runner.capacity - row.runner.activeJobs)} more runs.` : 'No workload metadata is currently advertised.'}</small>
              </div>
              <span className={`runner-pill ${row.statusTone}`}>{row.statusLabel}</span>
            </div>
          )}
        </div>
      )}

      {detailTab === 'metadata' && (
        <div className="dispatcher-detail-list">
          <DetailFact label="Connection" value={connectionLabel || 'Not advertised'} mono />
          <DetailFact label="Scopes" value={row.runner.scopes.length ? row.runner.scopes.join(', ') : 'All scopes'} />
          {row.runtime === 'kubernetes' ? (
            <>
              <DetailFact label="Namespace" value={meta.namespace || 'Not advertised'} mono />
              <DetailFact label="Node" value={meta.node || 'Not advertised'} mono />
              <DetailFact label="Service account" value={meta.serviceAccount || 'Not advertised'} mono />
            </>
          ) : (
            <>
              <DetailFact label="Host" value={meta.hostname || 'Not advertised'} mono />
              <DetailFact label="Docker network" value={meta.network || 'Not advertised'} mono />
            </>
          )}
        </div>
      )}

      {detailTab === 'logs' && (
        <div className="dispatcher-detail-list">
          <div className="dispatcher-work-item">
            <div>
              <strong>Live system logs</strong>
              <small>{meta.logSourceId || 'Runner log source is not advertised yet'}</small>
            </div>
            {meta.logSourceId ? (
              <Link className="runner-pill runner-pill--link" to={`/system/logs?source=${encodeURIComponent(meta.logSourceId)}`}>
                Open
              </Link>
            ) : (
              <span className="runner-pill runner-pill--muted">Pending</span>
            )}
          </div>
          <div className="dispatcher-work-item">
            <div>
              <strong>Heartbeat {row.status === 'offline' ? 'missing' : 'accepted'}</strong>
              <small>Last observed {row.heartbeatLabel}</small>
            </div>
            <span className={`runner-pill ${row.statusTone}`}>{row.statusLabel}</span>
          </div>
          {meta.disconnectedAt && (
            <div className="dispatcher-work-item">
              <div>
                <strong>Disconnected</strong>
                <small>{formatTimestamp(meta.disconnectedAt)}</small>
              </div>
              <span className="runner-pill runner-pill--warning">Recorded</span>
            </div>
          )}
        </div>
      )}
    </aside>
  );
}

function DetailStat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="dispatcher-detail-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function DetailFact({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="dispatcher-work-item">
      <div>
        <strong>{label}</strong>
        <small className={mono ? 'font-mono' : ''}>{value}</small>
      </div>
    </div>
  );
}

function buildRunnerFleetRow(
  runner: Runner,
  nowMs: number,
  pendingActions: Set<string>,
  pendingEjections: Set<string>
): RunnerFleetRow {
  const meta = getRunnerMeta(runner);
  const key = runnerActionKey(runner.runnerId, meta.connectionId) || runner.runnerId;
  const stale = meta.reachable && runner.allowDispatch && isRunnerHeartbeatStale(nowMs, runner.lastHeartbeatUnix);
  const recentlyDisconnected = meta.reachable && isRunnerRecentlyDisconnected(nowMs, meta.disconnectedAt);
  const status = !meta.reachable || stale ? 'offline' : !runner.allowDispatch ? 'paused' : recentlyDisconnected ? 'recovered' : runner.activeJobs > 0 ? 'busy' : 'online';
  const runtime = meta.runtime === 'kubernetes' ? 'kubernetes' : 'docker';
  const pendingDispatch = Boolean(key && pendingActions.has(key));
  const pendingEject = Boolean(key && pendingEjections.has(key));
  const hostLabel = runtime === 'kubernetes'
    ? meta.node || meta.namespace || meta.hostname
    : meta.hostname || meta.network;
  const capacityPercent = runner.capacity > 0 ? clampPercent((runner.activeJobs / runner.capacity) * 100) : 0;

  return {
    key,
    runner,
    status,
    ...statusPresentation(status, stale),
    runtime,
    hostLabel,
    heartbeatLabel: formatSince(nowMs, runner.lastHeartbeatUnix),
    capacityPercent,
    pendingDispatch,
    pendingEject,
    pending: pendingDispatch || pendingEject,
  };
}

function statusPresentation(status: RunnerFleetStatus, stale: boolean) {
  if (status === 'online') {
    return { statusLabel: 'Online', statusTone: 'runner-pill--ok', dotClass: 'runner-dot--ok' };
  }
  if (status === 'busy') {
    return { statusLabel: 'Busy', statusTone: 'runner-pill--warning', dotClass: 'runner-dot--warning' };
  }
  if (status === 'paused') {
    return { statusLabel: 'Paused', statusTone: 'runner-pill--warning', dotClass: 'runner-dot--warning' };
  }
  if (status === 'recovered') {
    return { statusLabel: 'Recovered', statusTone: 'runner-pill--warning', dotClass: 'runner-dot--warning' };
  }
  return {
    statusLabel: stale ? 'Heartbeat stale' : 'Offline',
    statusTone: 'runner-pill--error',
    dotClass: 'runner-dot--error',
  };
}
