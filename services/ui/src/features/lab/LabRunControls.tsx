import { NavLink } from 'react-router-dom';

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
  accessChecks,
  feedback,
  onPipelineChange,
  onScopeChange,
  onRun,
}: LabRunControlsProps) {
  const runDisabled = runPending || yamlLoading || validationErrorCount > 0 || accessLoading || accessBlocked;
  const readinessLabel = validationErrorCount
    ? 'Cannot run yet'
    : accessLoading
      ? 'Checking access'
      : accessError || accessBlocked
        ? 'Cannot run yet'
        : 'Ready to run';

  return (
    <div className="glass-card p-4 space-y-4 rounded-lg shadow-sm ring-1 ring-[var(--border-primary)]/70 bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-tertiary)]">
      <div className="grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-4 items-end">
        <div>
          <label htmlFor="lab-pipeline-select" className="block text-sm font-medium text-[var(--text-secondary)]">
            Pipeline
          </label>
          <select
            id="lab-pipeline-select"
            className="mt-1 block w-full pipelines-input py-2 px-3 text-sm"
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

        <div>
          <label htmlFor="lab-scope-input" className="block text-sm font-medium text-[var(--text-secondary)]">
            Target scope
          </label>
          <select
            id="lab-scope-input"
            className="mt-1 block w-full pipelines-input py-2 px-3 text-sm"
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

        <div className="flex flex-wrap items-center gap-2 justify-start md:justify-end">
          <button id="lab-run-btn" type="button" className="glass-button-primary" onClick={() => void onRun()} disabled={runDisabled}>
            <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 12h4l1-5 4 10 1-5h4" />
            </svg>
            <span>{runPending ? 'Running…' : 'Run'}</span>
          </button>
        </div>
      </div>

      <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm font-semibold text-[var(--text-primary)]">{readinessLabel}</p>
          {accessLoading ? <span className="text-xs text-[var(--text-secondary)]">Checking…</span> : null}
        </div>
        {accessError ? <p className="mt-2 text-sm text-red-500">{accessError}</p> : null}
        {!accessError && validationErrorCount === 0 ? (
          <ul className="mt-2 space-y-1 text-sm">
            {accessChecks.length ? (
              accessChecks.map((check, index) => (
                <li
                  key={`${check.action}-${check.resource_type}-${check.resource_id}-${index}`}
                  className={check.allowed ? 'text-green-600 dark:text-green-400' : 'text-red-500'}
                >
                  <span aria-hidden="true">{check.allowed ? '✓' : '✕'}</span> {formatRunCheck(check)}{' '}
                  {check.allowed ? 'is available' : 'is not available'}
                </li>
              ))
            ) : (
              <li className="text-[var(--text-secondary)]">Select a valid pipeline and scope.</li>
            )}
          </ul>
        ) : null}
      </div>

      <div
        id="lab-run-feedback"
        className={`text-sm ${feedback ? '' : 'hidden'} ${
          feedback?.tone === 'error'
            ? 'text-red-500'
            : feedback?.tone === 'success'
              ? 'text-green-500'
              : 'text-[var(--text-secondary)]'
        }`}
      >
        {feedback ? (
          <>
            <span>{feedback.message}</span>
            {feedback.runId ? (
              <>
                {' '}
                <NavLink className="underline" to="/pipelineruns/main">
                  View
                </NavLink>
              </>
            ) : null}
          </>
        ) : null}
      </div>
    </div>
  );
}
