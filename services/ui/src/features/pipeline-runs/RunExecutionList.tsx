import { useMemo } from 'react';
import { CheckCircle2, Circle, CircleDashed, CircleSlash, ListTree, XCircle } from 'lucide-react';
import type { GraphStatus, PipelineDefinition, RunListItem, StepDetail } from './contracts';
import { getGraphStatusColor, getGraphStatusLabel } from './graphLayout';
import { buildRunGraphSteps } from './runGraphModel';
import {
  buildRunExecutionLines,
  buildStepExecutionGroups,
  type RunExecutionLine,
} from './runExecutionListModel';

export function RunExecutionList({
  steps,
  onSelectStep,
  onOpenStepLogs,
  onOpenTaskLogs,
  childRuns,
  pipelineDefinition,
}: {
  runID: string;
  steps: StepDetail[];
  selectedStep: string | null;
  onSelectStep: (stepName: string | null) => void;
  onOpenStepLogs: (stepName: string) => void;
  onOpenTaskLogs: (stepName: string, taskName: string) => void;
  onOpenStepDetail: (stepName: string, taskName?: string) => void;
  childRuns: RunListItem[];
  pipelineDefinition?: PipelineDefinition;
}) {
  const graphSteps = useMemo(
    () => buildRunGraphSteps({ steps, pipelineDefinition, childRuns }),
    [childRuns, pipelineDefinition, steps]
  );
  const groups = useMemo(() => buildStepExecutionGroups(graphSteps), [graphSteps]);
  const lines = useMemo(() => buildRunExecutionLines(groups), [groups]);
  const totalTasks = graphSteps.reduce((sum, step) => sum + step.tasks.length, 0);

  const openLineLogs = (line: RunExecutionLine) => {
    if (line.taskName) {
      onSelectStep(line.stepName);
      onOpenTaskLogs(line.stepName, line.taskName);
      return;
    }
    onSelectStep(null);
    onOpenStepLogs(line.stepName);
  };

  return (
    <section className="run-execution-list" aria-label="Pipeline execution list">
      <header className="run-execution-list-head">
        <div className="run-execution-list-title">
          <ListTree className="h-4 w-4" aria-hidden="true" />
          <div>
            <h3>Execution</h3>
            <span>dependency ordered</span>
          </div>
        </div>
        <div className="run-execution-list-summary" aria-label="Execution totals">
          <span>{graphSteps.length} step{graphSteps.length === 1 ? '' : 's'}</span>
          <span>{totalTasks} task{totalTasks === 1 ? '' : 's'}</span>
        </div>
      </header>

      {lines.length ? (
        <ul className="run-execution-lines">
          {lines.map(line => (
            <ExecutionLineRow key={line.id} line={line} onOpenLogs={openLineLogs} />
          ))}
        </ul>
      ) : (
        <div className="run-detail-empty-state">
          <ListTree className="h-5 w-5" aria-hidden="true" />
          <span>No execution steps recorded</span>
        </div>
      )}
    </section>
  );
}

function ExecutionLineRow({ line, onOpenLogs }: { line: RunExecutionLine; onOpenLogs: (line: RunExecutionLine) => void }) {
  const statusLabel = getRunExecutionStatusLabel(line.status);
  const duration = line.duration?.trim();
  return (
    <li className="run-execution-line-shell">
      <button
        type="button"
        className="run-execution-line"
        onClick={() => onOpenLogs(line)}
        aria-label={`Open logs for ${line.stepName}${line.taskName ? ` ${line.taskName}` : ''}, ${statusLabel}`}
      >
        <StatusMark status={line.status} />
        <span className="run-execution-line-content">
          <strong className="run-execution-step-name">{line.stepName}</strong>
          <span className="run-execution-punctuation">:</span>
          <code className="run-execution-unit-chip">{line.unitName}</code>
          <span className="run-execution-punctuation">-</span>
          <span className={`run-execution-status-text run-execution-status-text--${line.status}`}>{statusLabel}</span>
          {duration && duration !== '0s' ? <span className="run-execution-line-duration">(took {duration})</span> : null}
        </span>
      </button>
    </li>
  );
}

function StatusMark({ status }: { status: GraphStatus }) {
  const label = getRunExecutionStatusLabel(status);
  const color = getGraphStatusColor(status);
  const className = `run-execution-status-mark${status === 'running' ? ' run-execution-status-mark--running' : ''}`;
  const props = { className, style: { color }, role: 'img', 'aria-label': label };
  if (status === 'success') return <CheckCircle2 {...props} />;
  if (status === 'failed') return <XCircle {...props} />;
  if (status === 'cancelled') return <CircleSlash {...props} />;
  if (status === 'running') return <CircleDashed {...props} />;
  return (
    <Circle {...props} />
  );
}

function getRunExecutionStatusLabel(status: GraphStatus): string {
  if (status === 'failed') return 'failure';
  return getGraphStatusLabel(status).toLowerCase();
}
