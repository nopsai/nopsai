import { useEffect, useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as ReactPointerEvent, ReactNode } from 'react';
import { FileText, GripHorizontal, ListTree, Maximize2, Workflow, X } from 'lucide-react';
import type { PipelineDefinition, PipelineRunFinalOutput, RunListItem, StepDetail } from './contracts';
import { RunExecutionList } from './RunExecutionList';
import { RunFinalOutputs } from './RunFinalOutputs';
import { StepsGraph } from './RunGraph';

type WorkspaceTab = 'graph' | 'list' | 'outputs';

const DEFAULT_GRAPH_FRAME_HEIGHT = 390;
const MIN_GRAPH_FRAME_HEIGHT = 300;
const MAX_GRAPH_FRAME_HEIGHT = 860;

export function RunDetailWorkspaceTabs({
  runID,
  steps,
  selectedStep,
  onSelectStep,
  onOpenStepLogs,
  onOpenTaskLogs,
  onOpenStepDetail,
  childRuns,
  pipelineDefinition,
  outputs,
  onCancelOutput,
  onRetryOutput,
}: {
  runID: string;
  steps: StepDetail[];
  selectedStep: string | null;
  onSelectStep: (step: string | null) => void;
  onOpenStepLogs: (stepName: string) => void;
  onOpenTaskLogs: (stepName: string, taskName: string) => void;
  onOpenStepDetail: (stepName: string, taskName?: string) => void;
  childRuns: RunListItem[];
  pipelineDefinition?: PipelineDefinition;
  outputs?: PipelineRunFinalOutput[];
  onCancelOutput: (outputId: string) => void;
  onRetryOutput: (outputId: string) => void;
}) {
  const [tabState, setTabState] = useState<{ runID: string; activeTab: WorkspaceTab }>({
    runID,
    activeTab: 'graph',
  });
  const activeTab = tabState.runID === runID ? tabState.activeTab : 'graph';
  const [expandedGraphOpen, setExpandedGraphOpen] = useState(false);
  const [graphFrameHeight, setGraphFrameHeight] = useState(DEFAULT_GRAPH_FRAME_HEIGHT);
  const graphResizeCleanupRef = useRef<(() => void) | null>(null);
  const outputCount = outputs?.length || 0;
  const setActiveTab = (tab: WorkspaceTab) => setTabState({ runID, activeTab: tab });
  const graphFrameStyle = {
    '--run-graph-frame-height': `${graphFrameHeight}px`,
  } as CSSProperties;

  useEffect(() => {
    if (!expandedGraphOpen) return undefined;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setExpandedGraphOpen(false);
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [expandedGraphOpen]);
  useEffect(() => () => graphResizeCleanupRef.current?.(), []);

  const beginGraphResize = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    const startY = event.clientY;
    const startHeight = graphFrameHeight;
    graphResizeCleanupRef.current?.();
    const handleMove = (moveEvent: PointerEvent) => {
      moveEvent.preventDefault();
      setGraphFrameHeight(clampGraphFrameHeight(startHeight + moveEvent.clientY - startY));
    };
    const stopResize = () => {
      document.removeEventListener('pointermove', handleMove);
      document.removeEventListener('pointerup', stopResize);
      document.body.classList.remove('run-detail-graph-resizing');
      graphResizeCleanupRef.current = null;
    };
    document.body.classList.add('run-detail-graph-resizing');
    document.addEventListener('pointermove', handleMove);
    document.addEventListener('pointerup', stopResize);
    graphResizeCleanupRef.current = stopResize;
  };

  return (
    <section className="run-detail-workspace" aria-label="Run graph and outputs">
      <div className="run-detail-workspace-head">
        <div className="run-detail-workspace-tabs" role="tablist" aria-label="Run detail workspace">
          <WorkspaceTabButton
            id="graph"
            label="Graph"
            active={activeTab === 'graph'}
            onSelect={setActiveTab}
            panelId="run-detail-graph-panel"
            icon={<Workflow className="h-4 w-4" aria-hidden="true" />}
          />
          <WorkspaceTabButton
            id="list"
            label="List"
            active={activeTab === 'list'}
            onSelect={setActiveTab}
            panelId="run-detail-list-panel"
            icon={<ListTree className="h-4 w-4" aria-hidden="true" />}
          />
          <WorkspaceTabButton
            id="outputs"
            label="Outputs"
            count={outputCount}
            active={activeTab === 'outputs'}
            onSelect={setActiveTab}
            panelId="run-detail-outputs-panel"
            icon={<FileText className="h-4 w-4" aria-hidden="true" />}
          />
        </div>
        {activeTab === 'graph' ? (
          <button
            type="button"
            className="run-detail-workspace-action"
            onClick={() => {
              setActiveTab('graph');
              setExpandedGraphOpen(true);
            }}
          >
            <Maximize2 className="h-4 w-4" aria-hidden="true" />
            <span>Expand graph</span>
          </button>
        ) : null}
      </div>

      {activeTab === 'graph' ? (
        <div
          id="run-detail-graph-panel"
          role="tabpanel"
          aria-labelledby="run-detail-graph-tab"
          className="run-detail-workspace-panel run-detail-workspace-panel--graph"
          style={graphFrameStyle}
        >
          <StepsGraph
            graphKey={runID}
            steps={steps}
            selectedStep={selectedStep}
            onSelectStep={onSelectStep}
            onOpenStepLogs={onOpenStepLogs}
            onOpenTaskLogs={onOpenTaskLogs}
            onOpenStepDetail={onOpenStepDetail}
            childRuns={childRuns}
            pipelineDefinition={pipelineDefinition}
          />
          <button
            type="button"
            className="run-detail-graph-resize"
            aria-label="Resize graph height"
            title="Drag to resize graph height"
            onPointerDown={beginGraphResize}
          >
            <GripHorizontal className="h-4 w-4" aria-hidden="true" />
            <span>{graphFrameHeight}px</span>
          </button>
        </div>
      ) : activeTab === 'list' ? (
        <div
          id="run-detail-list-panel"
          role="tabpanel"
          aria-labelledby="run-detail-list-tab"
          className="run-detail-workspace-panel"
        >
          <RunExecutionList
            runID={runID}
            steps={steps}
            selectedStep={selectedStep}
            onSelectStep={onSelectStep}
            onOpenStepLogs={onOpenStepLogs}
            onOpenTaskLogs={onOpenTaskLogs}
            onOpenStepDetail={onOpenStepDetail}
            childRuns={childRuns}
            pipelineDefinition={pipelineDefinition}
          />
        </div>
      ) : (
        <div
          id="run-detail-outputs-panel"
          role="tabpanel"
          aria-labelledby="run-detail-outputs-tab"
          className="run-detail-workspace-panel"
        >
          {outputCount > 0 ? (
            <RunFinalOutputs
              runID={runID}
              outputs={outputs}
              pipelineDefinition={pipelineDefinition}
              onCancelOutput={onCancelOutput}
              onRetryOutput={onRetryOutput}
            />
          ) : (
            <div className="run-detail-empty-state">
              <FileText className="h-5 w-5" aria-hidden="true" />
              <span>No final outputs recorded</span>
            </div>
          )}
        </div>
      )}
      {expandedGraphOpen ? (
        <div
          className="run-detail-graph-modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="run-detail-graph-modal-title"
          onMouseDown={event => {
            if (event.target === event.currentTarget) setExpandedGraphOpen(false);
          }}
        >
          <section className="run-detail-graph-modal__surface">
            <header className="run-detail-graph-modal__head">
              <div className="run-detail-graph-modal__title">
                <Workflow className="h-4 w-4" aria-hidden="true" />
                <h2 id="run-detail-graph-modal-title">Pipeline graph</h2>
                <span>{steps.length} step{steps.length === 1 ? '' : 's'}</span>
              </div>
              <button
                type="button"
                className="run-detail-graph-modal__close"
                aria-label="Close expanded graph"
                onClick={() => setExpandedGraphOpen(false)}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </header>
            <div className="run-detail-graph-modal__body">
              <StepsGraph
                graphKey={`${runID}:expanded`}
                steps={steps}
                selectedStep={selectedStep}
                onSelectStep={onSelectStep}
                onOpenStepLogs={onOpenStepLogs}
                onOpenTaskLogs={onOpenTaskLogs}
                onOpenStepDetail={onOpenStepDetail}
                childRuns={childRuns}
                pipelineDefinition={pipelineDefinition}
                presentation="embedded"
                ariaLabel="Expanded pipeline graph"
                allowScrollZoom
                expandedFrame
              />
            </div>
          </section>
        </div>
      ) : null}
    </section>
  );
}

function clampGraphFrameHeight(value: number): number {
  return Math.max(MIN_GRAPH_FRAME_HEIGHT, Math.min(MAX_GRAPH_FRAME_HEIGHT, Math.round(value)));
}

function WorkspaceTabButton({
  id,
  label,
  count,
  active,
  onSelect,
  panelId,
  icon,
}: {
  id: WorkspaceTab;
  label: string;
  count?: number;
  active: boolean;
  onSelect: (tab: WorkspaceTab) => void;
  panelId: string;
  icon: ReactNode;
}) {
  return (
    <button
      id={`run-detail-${id}-tab`}
      type="button"
      role="tab"
      aria-selected={active}
      aria-controls={panelId}
      className={`run-detail-workspace-tab${active ? ' is-active' : ''}`}
      onClick={() => onSelect(id)}
    >
      {icon}
      <span>{label}</span>
      {typeof count === 'number' ? <b>{count}</b> : null}
    </button>
  );
}
