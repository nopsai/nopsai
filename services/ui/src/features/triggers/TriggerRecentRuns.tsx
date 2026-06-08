import type { Ref, UIEventHandler } from 'react';

export type TriggerRecentRun = {
  run_id: string;
  pipeline_name: string;
  status?: string;
  git_ref?: string;
  started_at?: string;
  trigger_event_id?: string;
};

type TriggerRecentRunsProps = {
  runs: TriggerRecentRun[];
  loading: boolean;
  error: string | null;
  scrollable: boolean;
  listRef: Ref<HTMLUListElement>;
  onScroll: UIEventHandler<HTMLUListElement>;
  onOpenRun: (runId: string) => void;
};

function formatRelativeTime(value?: string) {
  if (!value) return 'N/A';
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return value;
  const delta = (Date.now() - timestamp) / 1000;
  if (delta < 60) return 'Just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
  return `${Math.floor(delta / 86400)}d ago`;
}

function formatRef(ref?: string) {
  if (!ref) return '—';
  return ref.replace(/^refs\/heads\//i, '').replace(/^refs\/tags\//i, '');
}

function statusClass(status?: string) {
  const key = (status || '').toLowerCase();
  if (key === 'success' || key === 'succeeded') return 'runner-pill--ok';
  if (key === 'failure' || key === 'failed' || key === 'error' || key === 'cancelled') return 'runner-pill--error';
  return 'runner-pill--muted';
}

function statusLabel(status?: string) {
  const value = (status || '').replace(/_/g, ' ').trim();
  if (!value) return 'unknown';
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function TriggerRecentRuns({
  runs,
  loading,
  error,
  scrollable,
  listRef,
  onScroll,
  onOpenRun,
}: TriggerRecentRunsProps) {
  return (
    <div className="glass-card overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
        <h3 className="text-lg font-semibold text-[var(--text-primary)]">Recent PipelineRuns</h3>
      </div>
      <div className="p-4">
        {loading ? (
          <p className="text-sm text-[var(--text-secondary)]">Loading runs…</p>
        ) : error ? (
          <p className="text-sm text-red-500">Failed to load runs: {error}</p>
        ) : runs.length ? (
          <ul ref={listRef} onScroll={onScroll} className={`triggers-runs-list ${scrollable ? 'triggers-runs-scroll' : ''}`}>
            {runs.map(run => {
              const runId = run.run_id || '';
              const triggerId = run.trigger_event_id || '';
              const shortRunId = runId ? String(runId).slice(0, 8) : 'unknown';
              const shortTriggerId = triggerId ? String(triggerId).slice(0, 8) : 'unknown';
              return (
                <li key={`run-${runId}`} className="triggers-runs-item">
                  <button
                    type="button"
                    className="pipelines-run-row w-full text-left"
                    onClick={() => onOpenRun(runId)}
                    title={`Open run ${runId}`}
                  >
                    <div className="triggers-run-row w-full">
                      <div className="triggers-run-row__line triggers-run-row__line--primary">
                        <span className="triggers-run-row__pipeline">{run.pipeline_name || 'pipeline'}</span>
                        <span className="triggers-run-row__time">{formatRelativeTime(run.started_at)}</span>
                      </div>
                      <div className="triggers-run-row__line triggers-run-row__line--status">
                        <span className={`runner-pill ${statusClass(run.status)}`}>{statusLabel(run.status)}</span>
                        <span className="runner-pill runner-pill--muted">{formatRef(run.git_ref)}</span>
                      </div>
                      <dl className="triggers-detail-grid triggers-run-details">
                        <dt className="triggers-detail-label">Run ID:</dt>
                        <dd className="triggers-detail-value">{shortRunId}</dd>
                        <dt className="triggers-detail-label">Trigger:</dt>
                        <dd className="triggers-detail-value">{shortTriggerId}</dd>
                      </dl>
                    </div>
                  </button>
                </li>
              );
            })}
          </ul>
        ) : (
          <p className="text-sm text-[var(--text-secondary)]">No recent runs for this trigger.</p>
        )}
      </div>
    </div>
  );
}
