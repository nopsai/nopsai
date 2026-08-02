import { useCallback, useEffect, useId, useMemo, useRef, useState, type MouseEvent, type WheelEvent as ReactWheelEvent } from 'react';
import { Copy, Maximize2, Search, X, ZoomIn, ZoomOut } from 'lucide-react';
import type {
  GraphLayout,
  GraphPoint,
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
const TASK_VIEW_HEIGHT = 250;
const TASK_REGION = { x: 134, y: 72, width: 952, height: 146 };
const EXPANDED_TASK_VIEW_WIDTH = 1320;
const EXPANDED_TASK_VIEW_HEIGHT = 340;
const EXPANDED_TASK_REGION = { x: 126, y: 82, width: 1068, height: 210 };
const MIN_GRAPH_SCALE = 0.45;
const MAX_GRAPH_SCALE = 5;
const ZOOM_BUTTON_FACTOR = 1.32;
const WHEEL_ZOOM_SENSITIVITY = 0.0018;
const DEFAULT_GRAPH_VIEW_PADDING = 52;
const DRAG_CLOSE_THRESHOLD = 5;
const MINIMAP_MIN_WINDOW_SIZE = 24;
const RUN_GRAPH_FLOATING_LAYER_SELECTOR = '[data-run-graph-floating-layer]';

type GraphDragStart = {
  clientX: number;
  clientY: number;
  panX: number;
  panY: number;
};

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
  ariaLabel,
  presentation = 'panel',
  allowScrollZoom = false,
  expandedFrame = false,
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
  ariaLabel?: string;
  presentation?: 'panel' | 'embedded';
  allowScrollZoom?: boolean;
  expandedFrame?: boolean;
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
  const [isMinimapDragging, setIsMinimapDragging] = useState(false);
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'unavailable'>('idle');
  const workspaceRef = useRef<HTMLDivElement | null>(null);
  const instructionsID = useId();
  const overviewRef = useRef<SVGSVGElement | null>(null);
  const minimapRef = useRef<SVGSVGElement | null>(null);
  const dragStartRef = useRef<GraphDragStart | null>(null);
  const didDragRef = useRef(false);
  const hydratedSelectionRef = useRef<string | null>(null);
  const copyResetRef = useRef<number | null>(null);
  const dragFrameRef = useRef<number | null>(null);
  const pendingDragViewportRef = useRef<GraphViewport | null>(null);

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
  const embedded = presentation === 'embedded';
  const graphRegionLabel = ariaLabel || (embedded ? 'Pipeline graph' : 'Pipeline run graph');

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
      if (dragFrameRef.current !== null) window.cancelAnimationFrame(dragFrameRef.current);
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
  const taskView = expandedFrame
    ? { width: EXPANDED_TASK_VIEW_WIDTH, height: EXPANDED_TASK_VIEW_HEIGHT, region: EXPANDED_TASK_REGION, maxScale: 1.32 }
    : { width: TASK_VIEW_WIDTH, height: TASK_VIEW_HEIGHT, region: TASK_REGION, maxScale: 1.08 };
  const fittedTaskLayout = useMemo(
    () => (selectedTaskLayout ? fitGraphLayoutToRegion(selectedTaskLayout, taskView.region, taskView.maxScale) : null),
    [selectedTaskLayout, taskView.maxScale, taskView.region]
  );
  const stepContext = useMemo(
    () => getStepContext(graphSteps, overviewLayout, selectedStepId),
    [graphSteps, overviewLayout, selectedStepId]
  );
  const minimapViewport = useMemo(
    () => getMinimapViewportRect(activeViewport, overviewView),
    [activeViewport, overviewView]
  );
  const minimapVisible = useMemo(() => {
    if (overviewLayout.nodes.length <= 1) return false;
    const zoomChanged = Math.abs(scale - defaultViewport.scale) > 0.04;
    const panChanged = Math.hypot(pan.x - defaultViewport.pan.x, pan.y - defaultViewport.pan.y) > 12;
    return zoomChanged || panChanged;
  }, [defaultViewport.pan.x, defaultViewport.pan.y, defaultViewport.scale, overviewLayout.nodes.length, pan.x, pan.y, scale]);
  const clearSelection = useCallback(() => {
    setSelectedEntity(null);
    onSelectStep(null);
  }, [onSelectStep, setSelectedEntity]);

  useEffect(() => {
    if (!revealOpen || typeof document === 'undefined') return undefined;
    const handleDocumentPointerDown = (event: globalThis.PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) return;
      if (workspaceRef.current?.contains(target)) return;
      if (target.closest(RUN_GRAPH_FLOATING_LAYER_SELECTOR)) return;
      clearSelection();
    };
    document.addEventListener('pointerdown', handleDocumentPointerDown);
    return () => document.removeEventListener('pointerdown', handleDocumentPointerDown);
  }, [clearSelection, revealOpen]);

  const graphFocusPoint = useCallback(
    (clientX?: number, clientY?: number) => {
      const rect = overviewRef.current?.getBoundingClientRect() || workspaceRef.current?.getBoundingClientRect();
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
  const scheduleDragViewport = useCallback(
    (viewport: GraphViewport) => {
      pendingDragViewportRef.current = viewport;
      if (typeof window === 'undefined') {
        setViewport(viewport);
        return;
      }
      if (dragFrameRef.current !== null) return;
      dragFrameRef.current = window.requestAnimationFrame(() => {
        const next = pendingDragViewportRef.current;
        pendingDragViewportRef.current = null;
        dragFrameRef.current = null;
        if (next) setViewport(next);
      });
    },
    [setViewport]
  );
  const minimapPointFromClient = useCallback(
    (clientX: number, clientY: number) => {
      const rect = minimapRef.current?.getBoundingClientRect();
      if (!rect?.width || !rect.height) return null;
      return {
        x: clampNumber(((clientX - rect.left) / rect.width) * overviewView.width, 0, overviewView.width),
        y: clampNumber(((clientY - rect.top) / rect.height) * overviewView.height, 0, overviewView.height),
      };
    },
    [overviewView.height, overviewView.width]
  );
  const panToMinimapPoint = useCallback(
    (point: { x: number; y: number } | null) => {
      if (!point) return;
      setViewport({
        scale,
        pan: {
          x: overviewView.width / 2 - point.x * scale,
          y: overviewView.height / 2 - point.y * scale,
        },
      });
    },
    [overviewView.height, overviewView.width, scale, setViewport]
  );
  const handleMinimapMouseDown = useCallback(
    (event: MouseEvent<SVGSVGElement>) => {
      if (event.button !== 0) return;
      event.preventDefault();
      event.stopPropagation();
      setIsMinimapDragging(true);
      panToMinimapPoint(minimapPointFromClient(event.clientX, event.clientY));
    },
    [minimapPointFromClient, panToMinimapPoint]
  );

  useEffect(() => {
    if (!isMinimapDragging || typeof document === 'undefined') return undefined;
    const handleMove = (event: globalThis.MouseEvent) => {
      event.preventDefault();
      panToMinimapPoint(minimapPointFromClient(event.clientX, event.clientY));
    };
    const handleUp = () => setIsMinimapDragging(false);
    document.addEventListener('mousemove', handleMove);
    document.addEventListener('mouseup', handleUp);
    return () => {
      document.removeEventListener('mousemove', handleMove);
      document.removeEventListener('mouseup', handleUp);
    };
  }, [isMinimapDragging, minimapPointFromClient, panToMinimapPoint]);

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
      if (!allowScrollZoom && !event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      event.stopPropagation();
      const deltaY = 'deltaY' in event ? event.deltaY : 0;
      const zoomFactor = Math.exp(-deltaY * WHEEL_ZOOM_SENSITIVITY);
      setGraphZoom(scale * zoomFactor, graphFocusPoint(event.clientX, event.clientY));
    },
    [allowScrollZoom, graphFocusPoint, scale, setGraphZoom]
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
    if (isInteractiveGraphTarget(event.target)) return;
    event.preventDefault();
    setIsDragging(true);
    didDragRef.current = false;
    dragStartRef.current = {
      clientX: event.clientX,
      clientY: event.clientY,
      panX: pan.x,
      panY: pan.y,
    };
    clearWindowSelection();
  };

  const handleMouseMove = (event: MouseEvent<HTMLDivElement>) => {
    const dragStart = dragStartRef.current;
    if (!isDragging || !dragStart) return;
    event.preventDefault();
    const clientDeltaX = event.clientX - dragStart.clientX;
    const clientDeltaY = event.clientY - dragStart.clientY;
    const moved = Math.hypot(clientDeltaX, clientDeltaY);
    if (moved > DRAG_CLOSE_THRESHOLD) didDragRef.current = true;
    const graphDelta = clientDeltaToGraphDelta(clientDeltaX, clientDeltaY, overviewRef.current, overviewView);
    scheduleDragViewport({
      scale,
      pan: {
        x: dragStart.panX + graphDelta.x,
        y: dragStart.panY + graphDelta.y,
      },
    });
    clearWindowSelection();
  };

  const endDragging = (event?: MouseEvent<HTMLDivElement>) => {
    if (isDragging && event && !didDragRef.current && !isInteractiveGraphTarget(event.target)) {
      clearSelection();
    }
    setIsDragging(false);
    dragStartRef.current = null;
    didDragRef.current = false;
  };

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
  const graphSearchControls = (
    <>
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
    </>
  );

  return (
    <section
      className={`run-graph-redesign${embedded ? ' run-graph-redesign--embedded' : ''}${expandedFrame ? ' run-graph-redesign--expanded-frame' : ''}`}
      data-presentation={presentation}
      role="region"
      aria-label={graphRegionLabel}
      aria-describedby={instructionsID}
    >
      <p id={instructionsID} className="sr-only">
        Use Tab to reach graph controls and nodes. Press Enter or Space to select graph nodes or open logs. {allowScrollZoom ? 'Use wheel scrolling to zoom.' : 'Use Control or Command with wheel scrolling to zoom.'}
      </p>
      <div className="run-graph-panel">
        {!embedded ? (
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
            {graphSearchControls}
          </div>
        ) : null}

        <div
          className={`run-graph-workspace${revealOpen ? ' has-task-reveal' : ''}`}
          ref={workspaceRef}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={endDragging}
          onMouseLeave={() => endDragging()}
        >
          <div className="run-graph-grid" />
          {embedded ? <div className="run-graph-embedded-controls">{graphSearchControls}</div> : null}
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
          {minimapVisible ? (
            <div
              className="run-graph-minimap"
              role="group"
              aria-label="Graph navigator"
              onMouseDown={event => event.stopPropagation()}
            >
              <div className="run-graph-minimap-label">Navigator</div>
              <svg
                className="run-graph-minimap-svg"
                ref={minimapRef}
                viewBox={`0 0 ${overviewView.width} ${overviewView.height}`}
                aria-label="Full graph position navigator"
                role="img"
                onMouseDown={handleMinimapMouseDown}
              >
                {overviewLayout.edges.map(edge => (
                  <path
                    key={edge.id}
                    className="run-graph-minimap-edge"
                    d={edgePathD(edge.points)}
                    vectorEffect="non-scaling-stroke"
                  />
                ))}
                {overviewLayout.nodes.map(node => (
                  <rect
                    key={node.data.id}
                    className={`run-graph-minimap-node${selectedStepId === node.data.id ? ' selected' : ''}`}
                    x={node.x}
                    y={node.y}
                    width={node.width}
                    height={node.height}
                    rx={8}
                    vectorEffect="non-scaling-stroke"
                  />
                ))}
                <rect
                  className="run-graph-minimap-window"
                  x={minimapViewport.x}
                  y={minimapViewport.y}
                  width={minimapViewport.width}
                  height={minimapViewport.height}
                  rx={10}
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
            </div>
          ) : null}

          <svg
            className="run-graph-overview"
            ref={overviewRef}
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
                viewBox={`0 0 ${taskView.width} ${taskView.height}`}
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

function isInteractiveGraphTarget(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest('[data-run-graph-node], button, input, select, textarea, a'));
}

function clientDeltaToGraphDelta(
  clientDeltaX: number,
  clientDeltaY: number,
  graphElement: Element | null,
  view: { width: number; height: number }
) {
  const rect = graphElement?.getBoundingClientRect();
  if (!rect?.width || !rect.height) return { x: clientDeltaX, y: clientDeltaY };
  return {
    x: clientDeltaX * (view.width / rect.width),
    y: clientDeltaY * (view.height / rect.height),
  };
}

function clearWindowSelection() {
  if (typeof window === 'undefined') return;
  window.getSelection()?.removeAllRanges();
}

function getMinimapViewportRect(viewport: GraphViewport, view: { width: number; height: number }) {
  const raw = {
    x: -viewport.pan.x / viewport.scale,
    y: -viewport.pan.y / viewport.scale,
    width: view.width / viewport.scale,
    height: view.height / viewport.scale,
  };
  const left = clampNumber(raw.x, 0, view.width);
  const top = clampNumber(raw.y, 0, view.height);
  const right = clampNumber(raw.x + raw.width, 0, view.width);
  const bottom = clampNumber(raw.y + raw.height, 0, view.height);
  const width = Math.min(view.width, Math.max(MINIMAP_MIN_WINDOW_SIZE, right - left));
  const height = Math.min(view.height, Math.max(MINIMAP_MIN_WINDOW_SIZE, bottom - top));
  return {
    x: clampNumber(left, 0, view.width - width),
    y: clampNumber(top, 0, view.height - height),
    width,
    height,
  };
}

function edgePathD(points: GraphPoint[]) {
  const [start, c1, c2, end] = points;
  if (!start || !c1 || !c2 || !end) return '';
  return `M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`;
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

function clampNumber(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}
