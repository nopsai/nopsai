import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { AlertTriangle, Route, Server } from 'lucide-react';
import {
  buildRunnerAssignmentsForScope,
  formatDispatcherRouteScope,
  getRunnerMeta,
  type DispatcherStatusState,
  type Runner,
} from './model';

type RunnerAssignmentsPanelProps = {
  title?: string;
  description?: string;
  targetScope: string;
  includeDescendantScopes?: boolean;
  status?: DispatcherStatusState | null;
  loading?: boolean;
  error?: string | null;
  className?: string;
};

export function RunnerAssignmentsPanel({
  title = 'Runner Assignments',
  description,
  targetScope,
  includeDescendantScopes = false,
  status = null,
  loading = false,
  error = null,
  className = '',
}: RunnerAssignmentsPanelProps) {
  const assignments = useMemo(
    () => buildRunnerAssignmentsForScope(status, targetScope, includeDescendantScopes),
    [includeDescendantScopes, status, targetScope]
  );

  return (
    <article className={`glass-card p-5 ${className}`.trim()}>
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h3 className="text-base font-semibold text-[var(--text-primary)]">{title}</h3>
          {description ? <p className="mt-1 text-sm text-[var(--text-secondary)]">{description}</p> : null}
        </div>
        <Server className="h-4 w-4 flex-none text-[var(--text-secondary)]" aria-hidden="true" />
      </div>

      <div className="mt-4 space-y-3">
        {loading && assignments.length === 0 ? (
          <div className="text-sm text-[var(--text-secondary)]">Loading runner assignments...</div>
        ) : error ? (
          <div className="flex items-center gap-2 text-sm text-amber-600">
            <AlertTriangle className="h-4 w-4" aria-hidden="true" />
            <span>Runner assignments unavailable.</span>
          </div>
        ) : assignments.length === 0 ? (
          <div className="text-sm text-[var(--text-secondary)]">No runner assignments found.</div>
        ) : (
          assignments.map(assignment => (
            <RunnerAssignmentRow
              key={assignment.runner.runnerId}
              runner={assignment.runner}
              scopes={assignment.scopes}
            />
          ))
        )}
      </div>

      <div className="mt-4 flex justify-end">
        <Link to="/system/dispatcher" className="glass-button-ghost text-xs">
          <Route className="h-3.5 w-3.5" aria-hidden="true" />
          Dispatcher
        </Link>
      </div>
    </article>
  );
}

function RunnerAssignmentRow({ runner, scopes }: { runner: Runner; scopes: string[] }) {
  const meta = getRunnerMeta(runner);
  const status = runnerStatusLabel(runner);
  const tone = runnerStatusTone(runner);
  const load = Math.max(runner.activeJobs || 0, runner.inflightJobs || 0);

  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-mono text-sm font-semibold text-[var(--text-primary)]">{runner.runnerId}</span>
        <span className={`runner-pill runner-pill--${tone}`}>{status}</span>
      </div>
      <div className="mt-2 flex flex-wrap gap-2 text-xs text-[var(--text-secondary)]">
        <span className="runner-pill runner-pill--muted">{load}/{runner.capacity || 1} active</span>
        <span className="runner-pill runner-pill--muted">{meta.runtime}</span>
        {scopes.map(scope => (
          <span key={`${runner.runnerId}-${scope}`} className="runner-pill runner-pill--ok">
            {formatDispatcherRouteScope(scope)}
          </span>
        ))}
      </div>
    </div>
  );
}

function runnerStatusLabel(runner: Runner) {
  const meta = getRunnerMeta(runner);
  if (!meta.reachable) return 'Unreachable';
  return runner.allowDispatch ? 'Online' : 'Paused';
}

function runnerStatusTone(runner: Runner): 'ok' | 'warning' | 'muted' {
  const meta = getRunnerMeta(runner);
  if (!meta.reachable) return 'warning';
  return runner.allowDispatch ? 'ok' : 'muted';
}
