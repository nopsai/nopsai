import { NavLink } from 'react-router-dom';
import { Activity } from 'lucide-react';

export type LabRunPipelineOption = {
  id: string;
};

export type LabRunResourceCheck = {
  allowed: boolean;
  action?: string;
  resource_type?: string;
  resource_id?: string;
};

export type LabRunFeedback = {
  tone: 'success' | 'error' | 'info';
  message: string;
  runId?: string;
} | null;

type LabRunControlsProps = {
  pipelines: LabRunPipelineOption[];
  pipelinesLoading: boolean;
  yamlLoading: boolean;
  selectedPipelineId: string;
  scopeOptions: string[];
  scopeValue: string;
  runPending: boolean;
  validationErrorCount: number;
  accessLoading: boolean;
  accessError: string | null;
  accessBlocked: boolean;
  accessChecks: LabRunResourceCheck[];
  feedback: LabRunFeedback;
  onPipelineChange: (pipelineId: string) => unknown | Promise<unknown>;
  onScopeChange: (scope: string) => void;
  onRun: () => void | Promise<void>;
};

function formatRunCheck(check: LabRunResourceCheck) {
  const type = String(check.resource_type || '').replace(/_/g, ' ');
  const id = String(check.resource_id || '');
  if (!type && !id) return 'Resource';
  return `${type} ${id}`.trim();
}

function pipelineRunDetailRoute(runId: string) {
  return `/pipelineruns/recent/${encodeURIComponent(runId)}`;
}

function getReadinessLabel({
  accessBlocked,
  accessError,
  accessLoading,
  validationErrorCount,
}: {
  accessBlocked: boolean;
  accessError: string | null;
  accessLoading: boolean;
  validationErrorCount: number;
}) {
  if (validationErrorCount) return 'Cannot run yet';
  if (accessLoading) return 'Checking access';
  if (accessError || accessBlocked) return 'Cannot run yet';
  return 'Ready to run';
}

export function LabRunControls({
  pipelines,
  pipelinesLoading,
  yamlLoading,
  selectedPipelineId,
  scopeOptions,
  scopeValue,
  runPending,
  validationErrorCount,
  accessLoading,
  accessError,
  accessBlocked,
  feedback,
  onPipelineChange,
  onScopeChange,
  onRun,
}: LabRunControlsProps) {
  const runDisabled = runPending || yamlLoading || validationErrorCount > 0 || accessLoading || accessBlocked || Boolean(accessError);
  const feedbackRunHref = feedback?.runId ? pipelineRunDetailRoute(feedback.runId) : '';

  return (
    <section className="glass-card lab-run-controls" aria-label="Lab run setup">
      <div className="lab-run-controls__form">
        <div className="lab-run-controls__field">
          <label htmlFor="lab-pipeline-select">
            Pipeline
          </label>
          <select
            id="lab-pipeline-select"
            className="pipelines-input"
            aria-label="Pipeline selection"
            value={selectedPipelineId}
            disabled={pipelinesLoading || yamlLoading}
            onChange={event => void onPipelineChange(event.target.value)}
          >
            <option value="">Select a pipeline</option>
            {pipelines.map(item => (
              <option key={item.id} value={item.id}>
                {item.id}
              </option>
            ))}
          </select>
        </div>

        <div className="lab-run-controls__field">
          <label htmlFor="lab-scope-input">
            Target scope
          </label>
          <select
            id="lab-scope-input"
            className="pipelines-input"
            aria-label="Target scope selection"
            value={scopeValue}
            onChange={event => onScopeChange(event.target.value)}
          >
            <option value="">Default scope</option>
            {scopeOptions.map(scope => (
              <option key={scope} value={scope}>
                {`/${scope}`}
              </option>
            ))}
          </select>
        </div>

        <div className="lab-run-controls__action">
          <button id="lab-run-btn" type="button" className="glass-button-primary" onClick={() => void onRun()} disabled={runDisabled}>
            <Activity className="h-4 w-4" aria-hidden="true" />
            <span>{runPending ? 'Running…' : 'Run'}</span>
          </button>
        </div>
      </div>

      <div
        id="lab-run-feedback"
        className={`lab-run-controls__feedback ${feedback ? '' : 'hidden'} ${
          feedback?.tone === 'error'
            ? 'lab-run-controls__feedback--error'
            : feedback?.tone === 'success'
              ? 'lab-run-controls__feedback--success'
              : 'lab-run-controls__feedback--info'
        }`}
      >
        {feedback ? (
          <>
            <span>{feedback.message}</span>
            {feedback.runId ? (
              <>
                {' '}
                <NavLink className="underline" to={feedbackRunHref}>
                  View
                </NavLink>
              </>
            ) : null}
          </>
        ) : null}
      </div>
    </section>
  );
}

export function LabRunReadinessPanel({
  validationErrorCount,
  accessLoading,
  accessError,
  accessBlocked,
  accessChecks,
}: Pick<
  LabRunControlsProps,
  'validationErrorCount' | 'accessLoading' | 'accessError' | 'accessBlocked' | 'accessChecks'
>) {
  const readinessLabel = getReadinessLabel({ accessBlocked, accessError, accessLoading, validationErrorCount });

  return (
    <section className="glass-card lab-run-readiness-panel" aria-label="Run readiness">
      <div className="lab-run-readiness-panel__head">
        <div>
          <h3>{readinessLabel}</h3>
          <p>{validationErrorCount ? `${validationErrorCount} validation issue${validationErrorCount === 1 ? '' : 's'}` : 'Access and references for this run.'}</p>
        </div>
        {accessLoading ? <span>Checking...</span> : null}
      </div>
      {accessError ? <p className="lab-run-readiness-panel__error">{accessError}</p> : null}
      {!accessError && validationErrorCount === 0 ? (
        <ul className="lab-run-readiness-panel__checks">
          {accessChecks.length ? (
            accessChecks.map((check, index) => (
              <li
                key={`${check.action}-${check.resource_type}-${check.resource_id}-${index}`}
                className={check.allowed ? 'lab-run-readiness-panel__check--ok' : 'lab-run-readiness-panel__check--blocked'}
              >
                <span className="lab-run-readiness-panel__dot" aria-hidden="true" />
                <span>{formatRunCheck(check)}</span>
                <strong>{check.allowed ? 'Available' : 'Blocked'}</strong>
              </li>
            ))
          ) : (
            <li className="lab-run-readiness-panel__empty">Select a valid pipeline and scope.</li>
          )}
        </ul>
      ) : null}
    </section>
  );
}
