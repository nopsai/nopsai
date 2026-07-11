import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react';
import type {
  GraphLayout,
  GraphLayoutNode,
  GraphStatus,
  GraphStep,
  GraphTask,
  PipelineDefinition,
  RunListItem,
  StepConfiguration,
  StepDetail,
} from './contracts';
import {
  calculateGraphLayout,
  deriveTaskGraphStatus,
  getGraphStatusColor,
  getGraphStatusLabel,
  normalizeGraphStatus,
} from './graphLayout';
import { formatStepDuration, formatTaskDuration } from './runGraphModel';
import { getStatusMeta } from './statusPresentation';

const STEP_WIDTH_CLOSED = 190;
const STEP_HEIGHT_CLOSED = 56;
export const TASK_MIN_WIDTH = 160;
export const TASK_MAX_WIDTH = 280;
export const TASK_HEIGHT = 48;
const H_GAP = 76;
const V_GAP = 26;
const STEP_HEADER_HEIGHT = 44;
const INNER_PADDING = 12;
const MIN_GRAPH_SCALE = 0.4;
const MAX_GRAPH_SCALE = 1.4;

function activateWithKeyboard(event: KeyboardEvent<SVGElement>, action: () => void) {
  if (event.key !== 'Enter' && event.key !== ' ') return;
  event.preventDefault();
  event.stopPropagation();
  action();
}

export function StepsGraph({
  steps,
  selectedStep,
  onSelectStep,
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
  steps: StepDetail[];
  selectedStep: string | null;
  onSelectStep: (name: string | null) => void;
  onOpenTaskLogs?: (stepName: string, taskName: string) => void;
  onOpenStepDetail?: (stepName: string) => void;
  childRuns: RunListItem[];
  pipelineDefinition?: PipelineDefinition;
  statusVariant?: StatusGlyphVariant;
  hideStatusLegend?: boolean;
  statusColorOverride?: string;
  stepStatusColorOverride?: string;
  taskStatusColorOverride?: string;
}) {
  const [expandedSteps, setExpandedSteps] = useState<Set<string>>(new Set());
  const [transform, setTransform] = useState({ x: 0, y: 0, k: 1 });
  const [preview, setPreview] = useState<{ step: GraphStep; x: number; y: number } | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [startPan, setStartPan] = useState({ x: 0, y: 0 });
  const containerRef = useRef<HTMLDivElement | null>(null);
  const interactedRef = useRef(false);
  const prevStepStatusesRef = useRef<Map<string, GraphStatus>>(new Map());

  useEffect(() => {
    if (!selectedStep) return undefined;
    const frame = requestAnimationFrame(() =>
      setExpandedSteps(prev => {
        if (prev.has(selectedStep)) return prev;
        const next = new Set(prev);
        next.add(selectedStep);
        return next;
      })
    );
    return () => cancelAnimationFrame(frame);
  }, [selectedStep]);

  const stepDefMap = useMemo(() => {
    const map = new Map<string, StepConfiguration>();
    (pipelineDefinition?.steps || []).forEach(step => map.set(step.name, step));
    return map;
  }, [pipelineDefinition]);

  const childRunMap = useMemo(() => {
    const map = new Map<string, RunListItem>();
    childRuns.forEach(run => {
      if (run.parent_step_name) map.set(run.parent_step_name, run);
    });
    return map;
  }, [childRuns]);

  const getStepBaseWidth = useCallback((step: GraphStep) => Math.max(STEP_WIDTH_CLOSED, Math.min(360, step.name.length * 8 + 120)), []);

  const graphSteps = useMemo<GraphStep[]>(() => {
    return steps.map(step => {
      const stepDef = stepDefMap.get(step.name);
      const includeLabel = step.configuration?.include
        ? `Included ${step.configuration.include.toLowerCase().includes('pipeline') ? 'Pipeline' : 'Step'}`
        : '';
      const tasks: GraphTask[] = (step.tasks || []).map(task => {
        const def = stepDef?.tasks?.find(t => t.name === task.task_name);
        const status = deriveTaskGraphStatus(task, step.status);
        return {
          id: task.task_name,
          name: task.task_name,
          status,
          duration: formatTaskDuration(task, status),
          dependsOn: def?.depends_on || [],
        };
      });
      return {
        id: step.name,
        name: step.name,
        status: normalizeGraphStatus(step.status, step.status === 'success'),
        duration: formatStepDuration(step),
        dependsOn: step.depends_on || [],
        tasks,
        includeLabel,
        childRun: childRunMap.get(step.name) || null,
      };
    });
  }, [childRunMap, stepDefMap, steps]);

  const expandedLayouts = useMemo(() => {
    const map = new Map<string, GraphLayout<GraphTask>>();
    const taskSize = (task: GraphTask) => {
      const label = `${task.name} - ${task.duration || '0s'}`;
      const width = Math.max(TASK_MIN_WIDTH, Math.min(TASK_MAX_WIDTH, 32 + label.length * 7));
      return { width, height: TASK_HEIGHT };
    };
    graphSteps.forEach(step => {
      if (!expandedSteps.has(step.id)) return;
      if (!step.tasks.length) return;
      const innerLayout = calculateGraphLayout<GraphTask>(step.tasks, taskSize, 30, 16);
      map.set(step.id, innerLayout);
    });
    return map;
  }, [expandedSteps, graphSteps]);

  const mainLayout = useMemo(
    () =>
      calculateGraphLayout<GraphStep>(
        graphSteps,
        step => {
          const inner = expandedLayouts.get(step.id);
          const baseWidth = getStepBaseWidth(step);
          if (inner) {
            return {
              width: Math.max(baseWidth, inner.width + INNER_PADDING * 2),
              height: Math.max(STEP_HEIGHT_CLOSED, inner.height + STEP_HEADER_HEIGHT + INNER_PADDING * 2),
            };
          }
          return { width: baseWidth, height: STEP_HEIGHT_CLOSED };
        },
        H_GAP,
        V_GAP
      ),
    [expandedLayouts, getStepBaseWidth, graphSteps]
  );

  const fitGraphToViewport = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const { clientWidth, clientHeight } = container;
    if (!clientWidth || !clientHeight || !mainLayout.width || !mainLayout.height) return;
    const padding = 32;
    const scaleX = (clientWidth - padding * 2) / mainLayout.width;
    const scaleY = (clientHeight - padding * 2) / mainLayout.height;
    const nextScale = Math.min(MAX_GRAPH_SCALE, Math.max(MIN_GRAPH_SCALE, Math.min(scaleX, scaleY)));
    const nextX = (clientWidth - mainLayout.width * nextScale) / 2;
    const nextY = (clientHeight - mainLayout.height * nextScale) / 2;
    setTransform({ x: nextX, y: nextY, k: nextScale });
  }, [mainLayout.height, mainLayout.width]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    if (interactedRef.current) return;
    const nextX = (container.clientWidth - mainLayout.width) / 2;
    const nextY = (container.clientHeight - mainLayout.height) / 2;
    if (Number.isFinite(nextX) && Number.isFinite(nextY)) {
      setTransform(prev => ({ ...prev, x: nextX, y: nextY }));
    }
  }, [mainLayout.height, mainLayout.width]);

  const pendingFitRef = useRef(false);

  useEffect(() => {
    const nextStatusMap = new Map<string, GraphStatus>();
    graphSteps.forEach(step => {
      nextStatusMap.set(step.id, step.status);
    });
    prevStepStatusesRef.current = nextStatusMap;
  }, [graphSteps]);

  useEffect(() => {
    if (pendingFitRef.current) {
      pendingFitRef.current = false;
      fitGraphToViewport();
    }
  }, [expandedLayouts, expandedSteps, fitGraphToViewport, mainLayout.height, mainLayout.width]);

  const toggleStep = useCallback(
    (id: string) => {
      pendingFitRef.current = true;
      setExpandedSteps(prev => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
      onSelectStep(id);
    },
    [onSelectStep]
  );
  const expandAll = useCallback(() => {
    pendingFitRef.current = true;
    setExpandedSteps(new Set(steps.map(step => step.name)));
  }, [steps]);
  const collapseAll = useCallback(() => {
    pendingFitRef.current = true;
    setExpandedSteps(new Set());
  }, []);

  const handleWheel = useCallback((event: React.WheelEvent | WheelEvent) => {
    interactedRef.current = true;
    event.stopPropagation();
    event.preventDefault();
    const deltaY = 'deltaY' in event ? event.deltaY : 0;
    const scaleSens = 0.001;
    setTransform(prev => {
      const nextScale = Math.min(MAX_GRAPH_SCALE, Math.max(MIN_GRAPH_SCALE, prev.k - deltaY * scaleSens));
      return { ...prev, k: nextScale };
    });
  }, []);

  const handleMouseDown = (event: MouseEvent) => {
    if (event.button !== 0) return;
    const target = event.target as HTMLElement;
    if (target.closest('[data-graph-node]')) return;
    interactedRef.current = true;
    setIsDragging(true);
    setStartPan({ x: event.clientX - transform.x, y: event.clientY - transform.y });
  };

  const handleMouseMove = (event: MouseEvent) => {
    if (!isDragging) return;
    setTransform(prev => ({ ...prev, x: event.clientX - startPan.x, y: event.clientY - startPan.y }));
  };

  const handleMouseUp = () => setIsDragging(false);

  const zoomIn = () => {
    interactedRef.current = true;
    setTransform(prev => ({ ...prev, k: Math.min(prev.k + 0.2, 3) }));
  };
  const zoomOut = () => {
    interactedRef.current = true;
    setTransform(prev => ({ ...prev, k: Math.max(prev.k - 0.2, 0.4) }));
  };

  const handleShowPreview = useCallback(
    (step: GraphStep, evt: MouseEvent) => {
      const rect = containerRef.current?.getBoundingClientRect();
      if (!rect) return;
      setPreview({
        step,
        x: evt.clientX - rect.left + 8,
        y: evt.clientY - rect.top - 12,
      });
    },
    []
  );

  const handleHidePreview = useCallback(() => {
    setPreview(null);
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return undefined;
    const listener = (evt: WheelEvent) => handleWheel(evt);
    el.addEventListener('wheel', listener, { passive: false });
    return () => el.removeEventListener('wheel', listener);
  }, [handleWheel]);

  const totalTasks = useMemo(() => steps.reduce((sum, step) => sum + (step.tasks?.length || 0), 0), [steps]);

  return (
    <div
      className="space-y-3"
      role="region"
      aria-label="Pipeline run graph"
      aria-describedby="pipeline-run-graph-instructions"
    >
      <p id="pipeline-run-graph-instructions" className="sr-only">
        Use Tab to reach graph controls and step nodes. Press Enter or Space to expand a step or open task logs.
      </p>
      <div className="flex flex-wrap items-center gap-2 px-2 text-sm text-[var(--text-secondary)]">
        <span className="px-2.5 py-1 text-[11px] uppercase tracking-[0.08em] rounded-full bg-[var(--bg-secondary)] text-[var(--text-primary)]">
          {steps.length} step{steps.length === 1 ? '' : 's'}
        </span>
        <span className="px-2.5 py-1 text-[11px] uppercase tracking-[0.08em] rounded-full bg-[var(--bg-secondary)] text-[var(--text-primary)]">
          {totalTasks} task{totalTasks === 1 ? '' : 's'}
        </span>
      </div>

      <div
        className="relative h-[720px] w-full overflow-hidden rounded-2xl border border-[var(--border-primary)] bg-white dark:bg-slate-950 shadow-[0_16px_44px_rgba(15,23,42,0.07)]"
        ref={containerRef}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseUp}
        style={{ overscrollBehavior: 'contain' }}
      >
        {!hideStatusLegend && (
          <div className="absolute top-3 left-3 z-20 flex flex-wrap items-center gap-3 text-[11px] text-[var(--text-secondary)]">
            {(['success', 'running', 'failed', 'cancelled', 'pending', 'skipped'] as GraphStatus[]).map(status => (
              <span key={status} className="flex items-center gap-1.5">
                <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
                  <GraphStatusGlyph status={status} x={8} y={8} size={12} />
                </svg>
                <span className="capitalize opacity-80">{getGraphStatusLabel(status)}</span>
              </span>
            ))}
          </div>
        )}

        <div className="absolute top-3 right-3 z-20 flex flex-col gap-1">
          <button
            type="button"
            onClick={() => {
              const allExpanded = expandedSteps.size === steps.length;
              if (allExpanded) {
                collapseAll();
              } else {
                expandAll();
                fitGraphToViewport();
              }
            }}
            className="h-9 w-9 rounded-full bg-[var(--bg-secondary)]/80 hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] shadow-sm border border-[var(--border-primary)]"
            title={expandedSteps.size === steps.length ? 'Collapse all' : 'Expand all'}
            aria-label={expandedSteps.size === steps.length ? 'Collapse all steps' : 'Expand all steps'}
          >
            {expandedSteps.size === steps.length ? '⇱' : '⇲'}
          </button>
          <button type="button" onClick={zoomIn} className="h-9 w-9 rounded-full bg-[var(--bg-secondary)]/80 hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] shadow-sm border border-[var(--border-primary)]" title="Zoom in" aria-label="Zoom in">
            +
          </button>
          <button type="button" onClick={zoomOut} className="h-9 w-9 rounded-full bg-[var(--bg-secondary)]/80 hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] shadow-sm border border-[var(--border-primary)]" title="Zoom out" aria-label="Zoom out">
            −
          </button>
        </div>

        <svg
          width="100%"
          height="100%"
          className="cursor-grab active:cursor-grabbing"
          role="team"
          aria-label={`${steps.length}-step pipeline run dependency graph`}
        >
          <g transform={`translate(${transform.x}, ${transform.y}) scale(${transform.k})`}>
            {mainLayout.edges.map(edge => {
              const [start, c1, c2, end] = edge.points;
              const color = getGraphStatusColor(edge.status);
              return (
                <g key={edge.id} className="transition-colors">
                  <path
                    d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
                    fill="none"
                    stroke={color}
                    strokeWidth={2.2}
                    strokeOpacity={0.75}
                    strokeLinecap="round"
                  />
                  {edge.status === 'running' && (
                    <path
                      d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
                      fill="none"
                      stroke="white"
                      strokeWidth={2}
                      strokeDasharray="4 8"
                      strokeOpacity={0.4}
                    >
                      <animate attributeName="stroke-dashoffset" from="12" to="0" dur="1s" repeatCount="indefinite" />
                    </path>
                  )}
                </g>
              );
            })}

      {mainLayout.nodes.map(node => (
      <StepNodeRenderer
        key={node.data.id}
        node={node}
        expanded={expandedSteps.has(node.data.id)}
        selected={selectedStep === node.data.id}
        onToggle={() => toggleStep(node.data.id)}
        onTaskClick={onOpenTaskLogs}
        onOpenDetail={onOpenStepDetail}
        onPreview={handleShowPreview}
        onPreviewEnd={handleHidePreview}
        innerLayout={expandedLayouts.get(node.data.id)}
        statusVariant={statusVariant}
        statusColorOverride={stepStatusColorOverride || statusColorOverride}
        taskStatusColorOverride={taskStatusColorOverride}
      />
      ))}
          </g>
        </svg>

        {preview && (
          <div
            className="absolute z-30 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-lg px-3 py-2 text-xs text-[var(--text-primary)] backdrop-blur-sm"
            style={{ left: preview.x, top: preview.y, pointerEvents: 'none' }}
          >
            <div className="flex items-center justify-between gap-3">
              <span className="font-semibold">{preview.step.name}</span>
              <span className="inline-flex items-center gap-1 text-[11px] text-[var(--text-secondary)]">
                <span
                  className="inline-flex h-2 w-2 rounded-full"
                  style={{ backgroundColor: getGraphStatusColor(preview.step.status) }}
                />
                {getGraphStatusLabel(preview.step.status)}
              </span>
            </div>
            <div className="mt-1 text-[11px] text-[var(--text-secondary)]">
              Duration: {preview.step.duration || '—'}
            </div>
            {preview.step.tasks?.length ? (
              <div className="mt-1 text-[11px] text-[var(--text-secondary)]">
                Tasks: {preview.step.tasks.length}
              </div>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}

function StepNodeRenderer({
  node,
  expanded,
  selected,
  onToggle,
  onTaskClick,
  onOpenDetail,
  onPreview,
  onPreviewEnd,
  innerLayout,
  statusVariant = 'default',
  statusColorOverride,
  taskStatusColorOverride,
}: {
  node: GraphLayoutNode<GraphStep>;
  expanded: boolean;
  selected: boolean;
  onToggle: () => void;
  onTaskClick?: (stepName: string, taskName: string) => void;
  onOpenDetail?: (stepName: string) => void;
  onPreview?: (step: GraphStep, evt: MouseEvent) => void;
  onPreviewEnd?: () => void;
  innerLayout?: GraphLayout<GraphTask>;
  statusVariant?: StatusGlyphVariant;
  statusColorOverride?: string;
  taskStatusColorOverride?: string;
}) {
  const statusColor = getGraphStatusColor(node.data.status);
  const titleColor = selected ? statusColor : 'var(--text-primary)';
  const durationLabel = node.data.duration || '0s';
  const showDuration = Boolean(durationLabel && durationLabel !== '0s');
  const nameWidthEstimate = node.data.name.length * 6.6;
  const infoX = Math.min(node.width - 22, 28 + nameWidthEstimate);
  const durationX = infoX + 4 + 6;
  const isDarkMode = typeof document !== 'undefined' && document.documentElement.classList.contains('dark');
  const infoColor = isDarkMode ? '#22c55e' : '#0284c7';
  const innerOffset = (() => {
    if (!innerLayout) return { x: 0, y: 0 };
    const availableWidth = Math.max(0, node.width - INNER_PADDING * 2);
    const availableHeight = Math.max(0, node.height - (STEP_HEADER_HEIGHT - 6) - INNER_PADDING);
    const offsetX = Math.max(0, (availableWidth - innerLayout.width) / 2);
    const offsetY = Math.max(0, (availableHeight - innerLayout.height) / 2);
    return { x: offsetX, y: offsetY };
  })();
  return (
    <g
      transform={`translate(${node.x}, ${node.y})`}
      className="cursor-pointer"
      onClick={event => {
        event.stopPropagation();
        onToggle();
      }}
      onMouseDown={event => event.stopPropagation()}
      data-graph-node
      role="team"
      aria-label={`${node.data.name} step`}
    >
      <rect
        width={node.width}
        height={node.height}
        fill="transparent"
        role="button"
        tabIndex={0}
        aria-label={`${expanded ? 'Collapse' : 'Expand'} ${node.data.name} step, ${getGraphStatusLabel(node.data.status)}`}
        aria-expanded={expanded}
        onKeyDown={event => activateWithKeyboard(event, onToggle)}
      />

      <g transform={`translate(${INNER_PADDING}, 10)`}>
        <GraphStatusGlyph status={node.data.status} x={12} y={12} size={expanded ? 16 : 18} opacity={expanded ? 0.3 : 1} variant={statusVariant} colorOverride={statusColorOverride} />
        <text x={30} y={14} className="text-[13px] font-semibold">
          <tspan style={{ fill: expanded ? 'var(--text-secondary)' : titleColor, opacity: expanded ? 0.5 : 1 }}>{node.data.name}</tspan>
        </text>
        {onOpenDetail && (
          <g
            transform={`translate(${infoX}, -8)`}
            onClick={event => {
              event.stopPropagation();
              onPreviewEnd?.();
              onOpenDetail(node.data.name);
            }}
            className="cursor-pointer"
            role="button"
            tabIndex={0}
            aria-label={`Open details for ${node.data.name} step`}
            style={{ pointerEvents: 'auto' }}
            onMouseEnter={event => onPreview?.(node.data, event)}
            onMouseLeave={() => onPreviewEnd?.()}
            onKeyDown={event =>
              activateWithKeyboard(event, () => {
                onPreviewEnd?.();
                onOpenDetail(node.data.name);
              })
            }
          >
            <rect width={12} height={12} rx={6} fill="transparent" stroke={infoColor} strokeWidth={1} opacity={selected ? 0.9 : 0.7} />
            <text x={6} y={9} textAnchor="middle" style={{ fill: infoColor, fontSize: '8px', fontWeight: 800, opacity: selected ? 1 : 0.95 }}>
              i
            </text>
          </g>
        )}
        <text x={durationX} y={14} className="text-[13px] font-semibold">
          {showDuration && (
            <tspan style={{ fill: 'var(--text-secondary)', fontWeight: expanded ? 500 : 600, opacity: expanded ? 0.5 : 1 }}>
              {`-  ${durationLabel}`}
            </tspan>
          )}
        </text>
        {(node.data.includeLabel || node.data.childRun) && (
          <text
            x={30}
            y={32}
            className="text-[11px]"
            style={{ fill: 'var(--text-secondary)', opacity: expanded ? 0.65 : 1 }}
          >
            {node.data.includeLabel && (
              <tspan style={{ fill: statusColor, fontWeight: 600, opacity: expanded ? 0.7 : 1 }}>
                {node.data.includeLabel}
              </tspan>
            )}
            {node.data.childRun && (
              <tspan dx={node.data.includeLabel ? 10 : 0}>{node.data.includeLabel ? '• Child run' : 'Child run'}</tspan>
            )}
          </text>
        )}
      </g>

      {expanded && innerLayout && (
        <g transform={`translate(${INNER_PADDING + innerOffset.x}, ${STEP_HEADER_HEIGHT - 6 + innerOffset.y})`}>
          {innerLayout.edges.map(edge => (
            <path
              key={edge.id}
              d={`M ${edge.points[0].x} ${edge.points[0].y} C ${edge.points[1].x} ${edge.points[1].y}, ${edge.points[2].x} ${edge.points[2].y}, ${edge.points[3].x} ${edge.points[3].y}`}
              fill="none"
              stroke={taskStatusColorOverride || getGraphStatusColor(edge.status)}
              strokeWidth={1.2}
              strokeOpacity={0.35}
              strokeLinecap="round"
            />
          ))}
          {innerLayout.nodes.map(task => (
          <TaskNodeRenderer
            key={task.data.id}
            task={task}
            stepName={node.data.name}
            onTaskClick={onTaskClick}
            statusColorOverride={taskStatusColorOverride}
          />
          ))}
        </g>
      )}
    </g>
  );
}

export function TaskNodeRenderer({
  task,
  stepName,
  onTaskClick,
  fontSize = 11,
  glyphSize = 14,
  statusColorOverride,
}: {
  task: GraphLayoutNode<GraphTask>;
  stepName: string;
  onTaskClick?: (stepName: string, taskName: string) => void;
  fontSize?: number;
  glyphSize?: number;
  statusColorOverride?: string;
}) {
  const durationLabel = task.data.duration || '0s';
  const showDuration = true;
  const centerX = task.width / 2;
  const statusIconSize = glyphSize + 2;
  const contentCenterY = task.height / 2;
  const statusIconCenterY = Math.max(statusIconSize / 2 + 6, contentCenterY - 8);
  const textY = Math.min(task.height - 8, contentCenterY + fontSize / 2 + 10);
  const statusColor = statusColorOverride || getGraphStatusColor(task.data.status);
  return (
    <g
      transform={`translate(${task.x}, ${task.y})`}
      onClick={event => {
        event.stopPropagation();
        onTaskClick?.(stepName, task.data.name);
      }}
      className="cursor-pointer"
      role={onTaskClick ? 'button' : undefined}
      tabIndex={onTaskClick ? 0 : undefined}
      aria-label={
        onTaskClick
          ? `Open logs for ${task.data.name} task in ${stepName}, ${getGraphStatusLabel(task.data.status)}`
          : undefined
      }
      onKeyDown={
        onTaskClick
          ? event => activateWithKeyboard(event, () => onTaskClick(stepName, task.data.name))
          : undefined
      }
    >
      <rect width={task.width} height={task.height} fill="transparent" />
      <GraphStatusGlyph
        status={task.data.status}
        x={centerX}
        y={statusIconCenterY}
        size={statusIconSize}
        colorOverride={statusColor}
      />
      <text x={centerX} y={textY} textAnchor="middle" style={{ fontSize, fontWeight: 700 }}>
        <tspan style={{ fill: 'var(--text-primary)' }}>{task.data.name}</tspan>
        {showDuration && <tspan style={{ fill: 'var(--text-secondary)', fontWeight: 700 }}>{`  -  ${durationLabel}`}</tspan>}
      </text>
    </g>
  );
}

function getGraphStatusIconPath(status: GraphStatus) {
  return getStatusMeta(status === 'failed' ? 'failure' : status, true).icon;
}

type StatusGlyphVariant = 'default' | 'dot';

function GraphStatusGlyph({
  status,
  x,
  y,
  size = 14,
  opacity = 1,
  variant = 'default',
  colorOverride,
}: {
  status: GraphStatus;
  x: number;
  y: number;
  size?: number;
  opacity?: number;
  variant?: StatusGlyphVariant;
  colorOverride?: string;
}) {
  const color = colorOverride || getGraphStatusColor(status);
  const strokeWidth = Math.max(1.6, Math.min(2.4, size / 6.5));
  if (variant === 'dot') {
    const r = size / 2;
    return <circle cx={x} cy={y} r={r} fill={color} opacity={opacity} />;
  }
  if (status === 'running') {
    const r = size / 2 - strokeWidth;
    return (
      <g transform={`translate(${x}, ${y})`} opacity={opacity}>
        <circle
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray="6 6"
          strokeLinecap="round"
        >
          <animate attributeName="stroke-dashoffset" from="12" to="0" dur="0.9s" repeatCount="indefinite" />
        </circle>
        <svg
          x={-size / 2}
          y={-size / 2}
          width={size}
          height={size}
          viewBox="0 0 24 24"
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d={getGraphStatusIconPath(status)} />
        </svg>
      </g>
    );
  }
  const path = getGraphStatusIconPath(status);
  return (
    <g transform={`translate(${x}, ${y})`} opacity={opacity}>
      <svg
        x={-size / 2}
        y={-size / 2}
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d={path} />
      </svg>
    </g>
  );
}
