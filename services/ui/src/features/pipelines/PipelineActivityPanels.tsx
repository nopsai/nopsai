import type { PipelineRun, PipelineTrigger } from './api';
import { runFinalOutputStatusPresentation } from '../pipeline-runs/finalOutputs';
import {
  formatPipelineGitRef,
  formatPipelineRelativeTime,
  formatPipelineTriggerBranchField,
  formatPipelineTriggerEvent,
  formatPipelineTriggerScope,
  parsePipelineDependencyReference,
  pipelineRunStatusClass,
  pipelineRunStatusLabel,
} from './model';

const MAX_VISIBLE_CARDS = 5;

type PipelineActivityPanelsProps = {
  pipelineLabel: string;
  triggers: PipelineTrigger[];
  triggersLoading: boolean;
  triggersError: string | null;
  dependencies: string[];
  runs: PipelineRun[];
  runsLoading: boolean;
  runsError: string | null;
  onOpenTrigger: (repoSlug: string) => void;
  onOpenDependency: (identifier: string) => void;
  onCopyDependency: (identifier: string) => void | Promise<void>;
  onOpenRun: (runID: string) => void;
};

export function PipelineActivityPanels({
  pipelineLabel,
  triggers,
  triggersLoading,
  triggersError,
  dependencies,
  runs,
  runsLoading,
  runsError,
  onOpenTrigger,
  onOpenDependency,
  onCopyDependency,
  onOpenRun,
}: PipelineActivityPanelsProps) {
  const normalizedDependencies = Array.from(new Set(dependencies))
    .map(parsePipelineDependencyReference)
    .sort((a, b) => a.raw.localeCompare(b.raw));

  return (
    <aside className="min-w-0 space-y-4">
      <section className="glass-card overflow-hidden">
        <header className="p-4 border-b border-[var(--border-primary)]" style={{ marginTop: '9px' }}>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">Trigger Rules</h3>
        </header>
        <div className="p-4">
          {triggersLoading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading triggers…</p>
          ) : triggersError ? (
            <p className="text-sm text-red-500">Failed to load triggers: {triggersError}</p>
          ) : triggers.length ? (
            <ul className={`triggers-pipeline-list ${triggers.length > MAX_VISIBLE_CARDS ? 'triggers-list-scroll' : ''}`}>
              {triggers.map((item, index) => {
                const branchField = formatPipelineTriggerBranchField(item.trigger);
                return (
                  <li key={`${item.repoSlug}-${index}`} className="triggers-pipeline-item">
                    <button
                      type="button"
                      className="triggers-pipeline-link"
                      title={`Open trigger ${item.repoSlug}`}
                      onClick={() => onOpenTrigger(item.repoSlug)}
                    >
                      <span className="triggers-pipeline-name">{item.repoSlug}</span>
                      <dl className="triggers-detail-grid triggers-pipeline-details">
                        <dt className="triggers-detail-label">Event:</dt>
                        <dd className="triggers-detail-value">{formatPipelineTriggerEvent(item.trigger.on)}</dd>
                        <dt className="triggers-detail-label">{branchField.label}</dt>
                        <dd className="triggers-detail-value">{branchField.value}</dd>
                        <dt className="triggers-detail-label">Scope:</dt>
                        <dd className="triggers-detail-value">{formatPipelineTriggerScope(item.trigger)}</dd>
                        <dt className="triggers-detail-label">Source:</dt>
                        <dd className="triggers-detail-value">{(item.source || 'database').trim() || 'database'}</dd>
                      </dl>
                    </button>
                  </li>
                );
              })}
            </ul>
          ) : (
            <p className="text-sm text-[var(--text-secondary)]">No trigger manifests reference this pipeline.</p>
          )}
        </div>
      </section>

      <section className="glass-card overflow-hidden">
        <header className="p-4 border-b border-[var(--border-primary)]">
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">Included Dependencies</h3>
        </header>
        <div className="p-4">
          {normalizedDependencies.length ? (
            <ul className={`triggers-pipeline-list ${normalizedDependencies.length > MAX_VISIBLE_CARDS ? 'triggers-list-scroll' : ''}`}>
              {normalizedDependencies.map(dependency => (
                <li key={dependency.raw} className="triggers-pipeline-item">
                  <button
                    type="button"
                    className="triggers-pipeline-link"
                    title={`${dependency.actionLabel} ${dependency.identifier}`}
                    onClick={() =>
                      dependency.navigable
                        ? onOpenDependency(dependency.identifier)
                        : void onCopyDependency(dependency.identifier || dependency.raw)
                    }
                  >
                    <span className="triggers-pipeline-name">{dependency.identifier || dependency.raw}</span>
                    <dl className="triggers-detail-grid triggers-pipeline-details">
                      <dt className="triggers-detail-label">Type:</dt>
                      <dd className="triggers-detail-value">{dependency.typeLabel}</dd>
                      <dt className="triggers-detail-label">Action:</dt>
                      <dd className="triggers-detail-value">{dependency.actionLabel}</dd>
                    </dl>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-[var(--text-secondary)]">No includes detected for this pipeline.</p>
          )}
        </div>
      </section>

      <section className="glass-card overflow-hidden" id="pipeline-recent-runs">
        <header className="p-4 border-b border-[var(--border-primary)]">
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">Recent Pipeline Runs</h3>
        </header>
        <div className="p-4">
          {runsLoading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading recent runs…</p>
          ) : runsError ? (
            <p className="text-sm text-red-500">Failed to load runs: {runsError}</p>
          ) : runs.length ? (
            <ul className="triggers-pipeline-list">
              {runs.map(run => {
                const runID = run.run_id || '';
                const triggerID = typeof run.trigger_event_id === 'string' ? run.trigger_event_id : '';
                const outputStatus = runFinalOutputStatusPresentation(run.final_output_status);
                return (
                  <li key={runID || `${run.pipeline_name}-${run.started_at}`} className="triggers-pipeline-item">
                    <button
                      type="button"
                      className="triggers-pipeline-link"
                      title={runID ? `Open run ${runID}` : 'Open pipeline runs'}
                      onClick={() => onOpenRun(runID)}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <span className="triggers-pipeline-name">{pipelineLabel}</span>
                          <p className="text-xs text-[var(--text-secondary)] mt-0.5">{formatPipelineRelativeTime(run.started_at)}</p>
                        </div>
                        <span className={`runner-pill ${pipelineRunStatusClass(run.status)}`}>{pipelineRunStatusLabel(run.status)}</span>
                      </div>
                      <dl className="triggers-detail-grid triggers-pipeline-details">
                        <dt className="triggers-detail-label">Branch:</dt>
                        <dd className="triggers-detail-value">{formatPipelineGitRef(run.git_ref)}</dd>
                        <dt className="triggers-detail-label">Run ID:</dt>
                        <dd className="triggers-detail-value">{runID ? runID.slice(0, 8) : '—'}</dd>
                        <dt className="triggers-detail-label">Trigger:</dt>
                        <dd className="triggers-detail-value">{triggerID ? triggerID.slice(0, 8) : '—'}</dd>
                        {outputStatus ? (
                          <>
                            <dt className="triggers-detail-label">Outputs:</dt>
                            <dd className="triggers-detail-value">
                              <span className={`runner-pill ${outputStatus.className}`} title={outputStatus.title}>
                                {outputStatus.label}
                              </span>
                            </dd>
                          </>
                        ) : null}
                      </dl>
                    </button>
                  </li>
                );
              })}
            </ul>
          ) : (
            <p className="text-sm text-[var(--text-secondary)]">No recent runs for this pipeline.</p>
          )}
        </div>
      </section>
    </aside>
  );
}
