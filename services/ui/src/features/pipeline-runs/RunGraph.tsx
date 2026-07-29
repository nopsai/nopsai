import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent, type WheelEvent as ReactWheelEvent } from 'react';
import { Copy, Maximize2, Search, X, ZoomIn, ZoomOut } from 'lucide-react';
import type {
  GraphLayout,
  GraphStatus,
  GraphStep,
  GraphTask,
  PipelineDefinition,
  RunListItem,
  StepDetail,
} from './contracts';
import {
  calculateGraphLayout,
  fitGraphLayoutToRegion,
  getGraphLayoutBounds,
  getGraphStatusColor,
  getGraphStatusLabel,
} from './graphLayout';
import {
  buildRunGraphSteps,
  countRunGraphTasks,
  matchesRunGraphEntityFilter,
  summarizeGraphStatuses,
  type RunGraphStatusFilter,
} from './runGraphModel';
import {
  GraphEdgePath,
  StepNode,
  TaskCardNode,
  TaskRevealContext,
  TASK_MAX_WIDTH,
  TASK_MIN_WIDTH,
  TASK_NODE_HEIGHT,
  type StatusGlyphVariant,
} from './RunGraphPrimitives';

export { TASK_HEIGHT, TASK_MAX_WIDTH, TASK_MIN_WIDTH, TaskNodeRenderer } from './RunGraphPrimitives';

const STEP_NODE_WIDTH = 184;
const STEP_NODE_HEIGHT = 88;
const OVERVIEW_MIN_WIDTH = 1120;
const OVERVIEW_MIN_HEIGHT = 310;
const TASK_VIEW_WIDTH = 1220;
const TASK_VIEW_HEIGHT = 360;
const TASK_REGION = { x: 148, y: 92, width: 924, height: 224 };
const MIN_GRAPH_SCALE = 0.45;
const MAX_GRAPH_SCALE = 5;
const ZOOM_BUTTON_FACTOR = 1.32;
const WHEEL_ZOOM_SENSITIVITY = 0.0018;
const DEFAULT_GRAPH_VIEW_PADDING = 52;

type SelectedGraphEntity =
  | { type: 'step'; stepId: string }
  | { type: 'task'; stepId: string; taskId: string };

type GraphViewport = {
  scale: number;
  pan: { x: number; y: number };
};

export function StepsGraph({
  graphKey,
  steps,
  selectedStep,
  onSelectStep,
  onOpenStepLogs,
  onOpenTaskLogs,
  onOpenStepDetail,
  childRuns,
  pipelineDefinition,
  statusVariant = 'default',
  hideStatusLegend = false,
  statusColorOverride,
  stepStatusColorOverride,
  taskStatusColorOverride,
}: {
  graphKey?: string;
  steps: StepDetail[];
  selectedStep: string | null;
  onSelectStep: (name: string | null) => void;
  onOpenStepLogs?: (stepName: string) => void;
  onOpenTaskLogs?: (stepName: string, taskName: string) => void;
  onOpenStepDetail?: (stepName: string, taskName?: string) => void;
  childRuns: RunListItem[];
  pipelineDefinition?: PipelineDefinition;
  statusVariant?: StatusGlyphVariant;
  hideStatusLegend?: boolean;
  statusColorOverride?: string;
  stepStatusColorOverride?: string;
  taskStatusColorOverride?: string;
}) {
  const [selectedEntityState, setSelectedEntityState] = useState<{
    graphKey: string | null;
    entity: SelectedGraphEntity | null;
  }>({ graphKey: null, entity: null });
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<RunGraphStatusFilter>('all');
  const [viewportState, setViewportState] = useState<(GraphViewport & { graphKey: string | null })>({
    graphKey: null,
    scale: 1,
    pan: { x: 0, y: 0 },
  });
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'unavailable'>('idle');
  const workspaceRef = useRef<HTMLDivElement | null>(null);
  const hydratedSelectionRef = useRef<string | null>(null);
  const copyResetRef = useRef<number | null>(null);

  const graphSteps = useMemo(
    () => buildRunGraphSteps({ steps, pipelineDefinition, childRuns }),
    [childRuns, pipelineDefinition, steps]
  );
  const totalTasks = useMemo(() => countRunGraphTasks(graphSteps), [graphSteps]);
  const statusSummary = useMemo(() => summarizeGraphStatuses(graphSteps), [graphSteps]);
  const filter = useMemo(
    () => ({ searchQuery, statusFilter }),
    [searchQuery, statusFilter]
  );
  const graphIdentity = useMemo(
    () => graphKey || buildGraphIdentity(graphSteps),
    [graphKey, graphSteps]
  );

  const taskLayouts = useMemo(() => {
    const map = new Map<string, GraphLayout<GraphTask>>();
    const taskSize = (task: GraphTask) => {
      const label = `${task.name} ${task.duration || '0s'}`;
      return {
        width: Math.max(TASK_MIN_WIDTH, Math.min(TASK_MAX_WIDTH, 36 + label.length * 7)),
        height: TASK_NODE_HEIGHT,
      };
    };
    graphSteps.forEach(step => {
      if (!step.tasks.length) return;
      map.set(step.id, calculateGraphLayout(step.tasks, taskSize, 44, 26));
    });
    return map;
  }, [graphSteps]);

  const overviewLayout = useMemo(
    () =>
      calculateGraphLayout<GraphStep>(
        graphSteps,
        step => ({
          width: Math.max(STEP_NODE_WIDTH, Math.min(260, 96 + step.name.length * 7)),
          height: STEP_NODE_HEIGHT,
        }),
        86,
        42
      ),
    [graphSteps]
  );

  const overviewView = useMemo(
    () => ({
      width: Math.max(OVERVIEW_MIN_WIDTH, overviewLayout.width + 40),
      height: Math.max(OVERVIEW_MIN_HEIGHT, overviewLayout.height + 30),
    }),
    [overviewLayout.height, overviewLayout.width]
  );
  const defaultViewport = useMemo(
    () => centeredGraphViewport(overviewLayout, overviewView),
    [overviewLayout, overviewView]
  );
  const activeViewport = viewportState.graphKey === graphIdentity ? viewportState : defaultViewport;
  const scale = activeViewport.scale;
  const pan = activeViewport.pan;
  const selectedEntity = selectedEntityState.graphKey === graphIdentity ? selectedEntityState.entity : null;
  const setSelectedEntity = useCallback(
    (entity: SelectedGraphEntity | null) => setSelectedEntityState({ graphKey: graphIdentity, entity }),
    [graphIdentity]
  );
  const setViewport = useCallback(
    (viewport: GraphViewport) => setViewportState({ graphKey: graphIdentity, ...viewport }),
    [graphIdentity]
  );
  const updateViewport = useCallback(
    (patch: Partial<GraphViewport>) => {
      setViewportState(prev => {
        const current = prev.graphKey === graphIdentity ? prev : defaultViewport;
        return {
          graphKey: graphIdentity,
          scale: patch.scale ?? current.scale,
          pan: patch.pan ?? current.pan,
        };
      });
    },
    [defaultViewport, graphIdentity]
  );

  useEffect(() => {
    if (hydratedSelectionRef.current === graphIdentity || !graphSteps.length) return;
    if (selectedStep) {
      hydratedSelectionRef.current = graphIdentity;
      return;
    }
    const selection = readSelectionFromLocation(graphSteps);
    hydratedSelectionRef.current = graphIdentity;
    if (!selection) return;
    const frame = window.requestAnimationFrame(() => {
      setSelectedEntity(selection);
      onSelectStep(selection.stepId);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [graphIdentity, graphSteps, onSelectStep, selectedStep, setSelectedEntity]);

  useEffect(() => {
    return () => {
      if (copyResetRef.current !== null) window.clearTimeout(copyResetRef.current);
    };
  }, []);

  const effectiveSelection = useMemo<SelectedGraphEntity | null>(() => {
    const selectedEntityExists = selectedEntity
      ? graphSteps.some(step => step.id === selectedEntity.stepId)
      : false;
    if (selectedEntity && selectedEntityExists && (!selectedStep || selectedEntity.stepId === selectedStep)) {
      return selectedEntity;
    }
    if (selectedStep && graphSteps.some(step => step.id === selectedStep)) {
      return { type: 'step', stepId: selectedStep };
    }
    return null;
  }, [graphSteps, selectedEntity, selectedStep]);
  const selectedStepId = effectiveSelection?.stepId || null;
  const selectedStepData = useMemo(
    () => graphSteps.find(step => step.id === selectedStepId) || null,
    [graphSteps, selectedStepId]
  );
  const selectedTaskData = useMemo(() => {
    if (effectiveSelection?.type !== 'task') return null;
    return selectedStepData?.tasks.find(task => task.id === effectiveSelection.taskId) || null;
  }, [effectiveSelection, selectedStepData]);
  const selectedTaskLayout = selectedStepData ? taskLayouts.get(selectedStepData.id) || null : null;
  const revealOpen = Boolean(selectedStepData && selectedTaskLayout && selectedStepData.tasks.length);
  const fittedTaskLayout = useMemo(
    () => (selectedTaskLayout ? fitGraphLayoutToRegion(selectedTaskLayout, TASK_REGION, 1.08) : null),
    [selectedTaskLayout]
  );
  const stepContext = useMemo(
    () => getStepContext(graphSteps, overviewLayout, selectedStepId),
    [graphSteps, overviewLayout, selectedStepId]
  );
  const clearSelection = useCallback(() => {
    setSelectedEntity(null);
    onSelectStep(null);
  }, [onSelectStep, setSelectedEntity]);

  const graphFocusPoint = useCallback(
    (clientX?: number, clientY?: number) => {
      const workspace = workspaceRef.current;
      const rect = workspace?.getBoundingClientRect();
      if (!rect?.width || !rect.height || clientX === undefined || clientY === undefined) {
        return { x: overviewView.width / 2, y: overviewView.height / 2 };
      }
      return {
        x: Math.max(0, Math.min(overviewView.width, ((clientX - rect.left) / rect.width) * overviewView.width)),
        y: Math.max(0, Math.min(overviewView.height, ((clientY - rect.top) / rect.height) * overviewView.height)),
      };
    },
    [overviewView.height, overviewView.width]
  );

  const setGraphZoom = useCallback(
    (nextScale: number, focus = graphFocusPoint()) => {
      const clampedScale = clampGraphScale(nextScale);
      if (clampedScale === scale) return;
      const graphX = (focus.x - pan.x) / scale;
      const graphY = (focus.y - pan.y) / scale;
      setViewport({
        scale: clampedScale,
        pan: {
          x: focus.x - graphX * clampedScale,
          y: focus.y - graphY * clampedScale,
        },
      });
    },
    [graphFocusPoint, pan.x, pan.y, scale, setViewport]
  );

  const handleStepActivate = useCallback(
    (step: GraphStep) => {
      if (!step.tasks.length) {
        clearSelection();
        onOpenStepLogs?.(step.name);
        return;
      }
      const clickingOpenStep = effectiveSelection?.type === 'step' && effectiveSelection.stepId === step.id;
      if (clickingOpenStep) {
        clearSelection();
        return;
      }
      setSelectedEntity({ type: 'step', stepId: step.id });
      onSelectStep(step.id);
    },
    [clearSelection, effectiveSelection, onOpenStepLogs, onSelectStep, setSelectedEntity]
  );

  const handleTaskActivate = useCallback(
    (step: GraphStep, task: GraphTask) => {
      setSelectedEntity({ type: 'task', stepId: step.id, taskId: task.id });
      onSelectStep(step.id);
      onOpenTaskLogs?.(step.name, task.name);
    },
    [onOpenTaskLogs, onSelectStep, setSelectedEntity]
  );

  const handleWheel = useCallback(
    (event: ReactWheelEvent<HTMLDivElement> | WheelEvent) => {
      event.preventDefault();
      event.stopPropagation();
      const deltaY = 'deltaY' in event ? event.deltaY : 0;
      const zoomFactor = Math.exp(-deltaY * WHEEL_ZOOM_SENSITIVITY);
      setGraphZoom(scale * zoomFactor, graphFocusPoint(event.clientX, event.clientY));
    },
    [graphFocusPoint, scale, setGraphZoom]
  );

  useEffect(() => {
    const workspace = workspaceRef.current;
    if (!workspace) return undefined;
    const listener = (event: WheelEvent) => handleWheel(event);
    workspace.addEventListener('wheel', listener, { passive: false });
    return () => workspace.removeEventListener('wheel', listener);
  }, [handleWheel]);

  const handleMouseDown = (event: MouseEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    const target = event.target;
    if (target instanceof Element && target.closest('[data-run-graph-node], button, input, select')) return;
    setIsDragging(true);
    setDragStart({ x: event.clientX - pan.x, y: event.clientY - pan.y });
  };

  const handleMouseMove = (event: MouseEvent<HTMLDivElement>) => {
    if (!isDragging) return;
    updateViewport({ pan: { x: event.clientX - dragStart.x, y: event.clientY - dragStart.y } });
  };

  const endDragging = () => setIsDragging(false);

  const resetView = () => {
    setViewport(defaultViewport);
  };

  const zoomIn = () => {
    setGraphZoom(scale * ZOOM_BUTTON_FACTOR);
  };

  const zoomOut = () => {
    setGraphZoom(scale / ZOOM_BUTTON_FACTOR);
  };

  const copySelectionLink = useCallback(async () => {
    if (!effectiveSelection) return;
    const href = buildSelectionHref(effectiveSelection);
    try {
      if (typeof navigator === 'undefined' || !navigator.clipboard) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(href);
      setCopyState('copied');
    } catch {
      setCopyState('unavailable');
    }
    if (copyResetRef.current !== null) window.clearTimeout(copyResetRef.current);
    copyResetRef.current = window.setTimeout(() => setCopyState('idle'), 2200);
  }, [effectiveSelection]);

  return (
    <section
      className="run-graph-redesign"
      role="region"
      aria-label="Pipeline run graph"
      aria-describedby="pipeline-run-graph-instructions"
    >
      <p id="pipeline-run-graph-instructions" className="sr-only">
        Use Tab to reach graph controls and nodes. Press Enter or Space to select graph nodes or open logs.
      </p>
      <div className="run-graph-panel">
        <div className="run-graph-head">
          <div className="run-graph-title">
            <span>Execution Graph</span>
            <small>Step dependency map</small>
          </div>
          <span className="run-graph-count">{steps.length} step{steps.length === 1 ? '' : 's'}</span>
          <span className="run-graph-count">{totalTasks} task{totalTasks === 1 ? '' : 's'}</span>
          {!hideStatusLegend ? (
            <div className="run-graph-legend" aria-label="Graph status summary">
              {(['success', 'running', 'failed'] as GraphStatus[]).map(status => (
                <span key={status}>
                  <i style={{ backgroundColor: statusColor(status, statusColorOverride) }} />
                  {statusSummary[status]} {getGraphStatusLabel(status)}
                </span>
              ))}
            </div>
          ) : null}
          <div className="run-graph-head-spacer" />
          <label className="run-graph-search">
            <Search className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">Search graph nodes</span>
            <input
              value={searchQuery}
              onChange={event => setSearchQuery(event.target.value)}
              placeholder="Find step or task"
            />
          </label>
          <select
            className="run-graph-select"
            aria-label="Filter graph by status"
            value={statusFilter}
            onChange={event => setStatusFilter(event.target.value as RunGraphStatusFilter)}
          >
            <option value="all">All statuses</option>
            <option value="success">Succeeded</option>
            <option value="failed">Failed</option>
            <option value="running">Running</option>
            <option value="pending">Pending</option>
            <option value="skipped">Skipped</option>
            <option value="cancelled">Cancelled</option>
          </select>
        </div>

        <div
          className={`run-graph-workspace${revealOpen ? ' has-task-reveal' : ''}`}
          ref={workspaceRef}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={endDragging}
          onMouseLeave={endDragging}
        >
          <div className="run-graph-grid" />
          <div className="run-graph-tools">
            <button type="button" onClick={resetView} aria-label="Fit graph" title="Fit graph">
              <Maximize2 className="h-4 w-4" aria-hidden="true" />
            </button>
            <button type="button" onClick={zoomIn} aria-label="Zoom in" title="Zoom in">
              <ZoomIn className="h-4 w-4" aria-hidden="true" />
            </button>
            <button type="button" onClick={zoomOut} aria-label="Zoom out" title="Zoom out">
              <ZoomOut className="h-4 w-4" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={copySelectionLink}
              disabled={!effectiveSelection}
              aria-label="Copy graph selection link"
              title="Copy selection link"
            >
              <Copy className="h-4 w-4" aria-hidden="true" />
            </button>
            {copyState !== 'idle' ? (
              <span className="run-graph-copy-state run-graph-copy-state-floating">
                {copyState === 'copied' ? 'Copied' : 'Clipboard unavailable'}
              </span>
            ) : null}
          </div>

          <svg
            className="run-graph-overview"
            viewBox={`0 0 ${overviewView.width} ${overviewView.height}`}
            role="img"
            aria-label={`${steps.length}-step pipeline dependency graph`}
          >
            <g transform={`translate(${pan.x}, ${pan.y}) scale(${scale})`}>
              {overviewLayout.edges.map(edge => {
                const from = graphSteps.find(step => step.id === edge.from);
                const to = graphSteps.find(step => step.id === edge.to);
                const dimmed = Boolean(from && to && !matchesRunGraphEntityFilter(from, filter) && !matchesRunGraphEntityFilter(to, filter));
                return (
                  <GraphEdgePath
                    key={edge.id}
                    points={edge.points}
                    status={edge.status}
                    active={Boolean(selectedStepId && (edge.from === selectedStepId || edge.to === selectedStepId))}
                    dimmed={dimmed}
                    colorOverride={statusColorOverride}
                  />
                );
              })}
              {overviewLayout.nodes.map(node => (
                <StepNode
                  key={node.data.id}
                  node={node}
                  selected={selectedStepId === node.data.id}
                  dimmed={!matchesRunGraphEntityFilter(node.data, filter)}
                  onActivate={() => handleStepActivate(node.data)}
                  onOpenDetails={() => onOpenStepDetail?.(node.data.name)}
                  opensStepLogs={Boolean(onOpenStepLogs && !node.data.tasks.length)}
                  statusVariant={statusVariant}
                  statusColorOverride={stepStatusColorOverride || statusColorOverride}
                />
              ))}
            </g>
          </svg>

          {revealOpen && selectedStepData && fittedTaskLayout ? (
            <div className="run-graph-task-reveal" aria-live="polite">
              <div className="run-graph-task-chip">
                {selectedStepData.name} - {selectedStepData.tasks.length} task{selectedStepData.tasks.length === 1 ? '' : 's'} - {selectedStepData.duration || '0s'}
              </div>
              <button
                className="run-graph-reveal-close"
                type="button"
                aria-label="Clear graph selection"
                title="Clear selection"
                onMouseDown={event => event.stopPropagation()}
                onClick={clearSelection}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
              <svg
                className="run-graph-task-svg"
                viewBox={`0 0 ${TASK_VIEW_WIDTH} ${TASK_VIEW_HEIGHT}`}
                role="img"
                aria-label={`Task graph for ${selectedStepData.name}`}
              >
                <TaskRevealContext
                  context={stepContext}
                  layout={fittedTaskLayout}
                  selectedStep={selectedStepData}
                  onSelectStep={handleStepActivate}
                  filter={filter}
                  colorOverride={statusColorOverride}
                />
                {fittedTaskLayout.edges.map(edge => {
                  const from = selectedStepData.tasks.find(task => task.id === edge.from);
                  const to = selectedStepData.tasks.find(task => task.id === edge.to);
                  const dimmed = Boolean(from && to && !matchesRunGraphEntityFilter(from, filter) && !matchesRunGraphEntityFilter(to, filter));
                  return (
                    <GraphEdgePath
                      key={edge.id}
                      points={edge.points}
                      status={edge.status}
                      active={effectiveSelection?.type === 'task' && (edge.from === effectiveSelection.taskId || edge.to === effectiveSelection.taskId)}
                      dimmed={dimmed}
                      colorOverride={taskStatusColorOverride || statusColorOverride}
                    />
                  );
                })}
                {fittedTaskLayout.nodes.map(task => (
                  <TaskCardNode
                    key={task.data.id}
                    task={task}
                    stepName={selectedStepData.name}
                    selected={selectedTaskData?.id === task.data.id}
                    dimmed={!matchesRunGraphEntityFilter(task.data, filter)}
                    onActivate={() => handleTaskActivate(selectedStepData, task.data)}
                    onOpenDetails={() => onOpenStepDetail?.(selectedStepData.name, task.data.name)}
                    statusColorOverride={taskStatusColorOverride || statusColorOverride}
                  />
                ))}
              </svg>
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );
}

function getStepContext(steps: GraphStep[], layout: GraphLayout<GraphStep>, selectedStepId: string | null) {
  if (!selectedStepId) return { upstream: [], downstream: [] };
  const stepMap = new Map(steps.map(step => [step.id, step]));
  const upstream = layout.edges
    .filter(edge => edge.to === selectedStepId)
    .map(edge => stepMap.get(edge.from))
    .filter((step): step is GraphStep => Boolean(step));
  const downstream = layout.edges
    .filter(edge => edge.from === selectedStepId)
    .map(edge => stepMap.get(edge.to))
    .filter((step): step is GraphStep => Boolean(step));
  return { upstream, downstream };
}

function readSelectionFromLocation(steps: GraphStep[]): SelectedGraphEntity | null {
  if (typeof window === 'undefined') return null;
  const params = readHashRouteSearchParams();
  const stepId = params.get('step');
  if (!stepId) return null;
  const step = steps.find(candidate => candidate.id === stepId);
  if (!step) return null;
  const taskId = params.get('task');
  if (taskId && step.tasks.some(task => task.id === taskId)) {
    return { type: 'task', stepId, taskId };
  }
  return { type: 'step', stepId };
}

function buildSelectionHref(selection: SelectedGraphEntity): string {
  if (typeof window === 'undefined') return '';
  const url = new URL(window.location.href);
  const hash = url.hash.startsWith('#') ? url.hash.slice(1) : url.hash;
  const [routePart, fragmentPart = ''] = hash.split('#');
  const [pathPart, queryPart = ''] = routePart.split('?');
  const params = new URLSearchParams(queryPart);
  params.set('step', selection.stepId);
  if (selection.type === 'task') params.set('task', selection.taskId);
  else params.delete('task');
  url.hash = `${pathPart}?${params.toString()}${fragmentPart ? `#${fragmentPart}` : ''}`;
  return url.toString();
}

function readHashRouteSearchParams() {
  const hash = window.location.hash.startsWith('#') ? window.location.hash.slice(1) : window.location.hash;
  const routePart = hash.split('#')[0] || '';
  const queryPart = routePart.includes('?') ? routePart.slice(routePart.indexOf('?') + 1) : window.location.search.slice(1);
  return new URLSearchParams(queryPart);
}

function buildGraphIdentity(steps: GraphStep[]): string {
  if (!steps.length) return 'empty';
  return steps
    .map(step => [
      step.id,
      step.dependsOn?.join(',') || '',
      step.tasks.map(task => `${task.id}:${task.dependsOn?.join(',') || ''}`).join(','),
    ].join(':'))
    .join('|');
}

function centeredGraphViewport<T>(layout: GraphLayout<T>, view: { width: number; height: number }): GraphViewport {
  if (!layout.nodes.length) return { scale: 1, pan: { x: 0, y: 0 } };
  const bounds = getGraphLayoutBounds(layout);
  const availableWidth = Math.max(1, view.width - DEFAULT_GRAPH_VIEW_PADDING * 2);
  const availableHeight = Math.max(1, view.height - DEFAULT_GRAPH_VIEW_PADDING * 2);
  const fitScale = Math.min(availableWidth / bounds.width, availableHeight / bounds.height);
  const scale = clampGraphScale(Math.min(1, fitScale));
  const graphCenter = {
    x: bounds.x + bounds.width / 2,
    y: bounds.y + bounds.height / 2,
  };
  return {
    scale,
    pan: {
      x: view.width / 2 - graphCenter.x * scale,
      y: view.height / 2 - graphCenter.y * scale,
    },
  };
}

function statusColor(status: GraphStatus, override?: string) {
  return override || getGraphStatusColor(status);
}

function clampGraphScale(value: number) {
  return Math.max(MIN_GRAPH_SCALE, Math.min(MAX_GRAPH_SCALE, value));
}
