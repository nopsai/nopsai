import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import yaml from 'js-yaml';
import type {
  GraphSize,
  GraphTask,
  PipelineDefinition,
  StepDetail,
  TaskDefinition,
  TaskGraphLayout,
} from './contracts';
import { calculateGraphLayout, deriveTaskGraphStatus, getGraphStatusColor } from './graphLayout';
import { TASK_HEIGHT, TASK_MAX_WIDTH, TASK_MIN_WIDTH, TaskNodeRenderer } from './RunGraph';
import { formatStepDuration, formatTaskDuration } from './runGraphModel';
import { getStatusMeta } from './statusPresentation';

const EMPTY_TASK_DEFINITIONS: TaskDefinition[] = [];

export function StepDetailModal({
  step,
  onClose,
  onViewLogs,
  pipelineDefinition,
}: {
  step: StepDetail | null;
  onClose: () => void;
  onViewLogs: () => void;
  pipelineDefinition?: PipelineDefinition;
}) {
  const config = step?.configuration;
  const taskDefs = useMemo(() => config?.tasks ?? [], [config?.tasks]);
  const [activeTaskName, setActiveTaskName] = useState<string | null>(null);

  const taskLayout = useMemo<TaskGraphLayout | null>(() => {
    if (!step) return null;
    const layoutTasks: GraphTask[] = (step.tasks || []).map(task => {
      const def = taskDefs.find(t => t.name === task.task_name);
      const deps = def?.depends_on || [];
      const taskId = task.task_name || task.task_id || `task-${task.task_index}`;
      const status = deriveTaskGraphStatus(task, step.status);
      return {
        id: taskId,
        name: task.task_name,
        status,
        duration: formatTaskDuration(task, status),
        dependsOn: deps,
      };
    });
    const dependencyCount = layoutTasks.reduce((sum, t) => sum + (t.dependsOn?.length || 0), 0);
    const hasAnyDeps = dependencyCount > 0;
    const chained = layoutTasks.map((t, idx) => (idx === 0 ? t : { ...t, dependsOn: [layoutTasks[idx - 1].id] }));
    const tasksForLayout = !hasAnyDeps && layoutTasks.length > 1 ? chained : layoutTasks;
    const sizeFor = (task: GraphTask): GraphSize => {
      const label = `${task.name} - ${task.duration || '0s'}`;
      const width = Math.max(TASK_MIN_WIDTH + 40, Math.min(TASK_MAX_WIDTH + 60, 38 + label.length * 8));
      return { width, height: Math.max(TASK_HEIGHT + 28, 64) };
    };

    const baseLayout = calculateGraphLayout(tasksForLayout, sizeFor, 44, 32, 'horizontal');
    const layoutNeedsChain = !hasAnyDeps && baseLayout.nodes.length > 1 && baseLayout.edges.length === 0;
    const finalLayout = layoutNeedsChain ? calculateGraphLayout(chained, sizeFor, 44, 32, 'horizontal') : baseLayout;

    return {
      ...finalLayout,
      orientation: 'horizontal',
      taskCount: layoutTasks.length,
      dependencyCount,
    };
  }, [step, taskDefs]);

  const taskGraphView = useMemo(() => {
    if (!taskLayout || !taskLayout.nodes.length) return null;
    const density = taskLayout.taskCount ? taskLayout.dependencyCount / taskLayout.taskCount : 0;
    const padScale = 1 + Math.min(0.6, taskLayout.taskCount * 0.04 + density * 0.06);
    const padX = Math.max(48, Math.min(170, taskLayout.width * 0.18 * padScale));
    const padY = Math.max(60, Math.min(200, taskLayout.height * 0.2 * padScale));
    const viewWidth = Math.max(taskLayout.width + padX * 2, 360 + taskLayout.taskCount * 6);
    const viewHeight = Math.max(taskLayout.height + padY * 2, 380 + taskLayout.taskCount * 8);
    return {
      viewWidth,
      viewHeight,
      taskCount: taskLayout.taskCount,
      dependencyCount: taskLayout.dependencyCount,
      density,
      orientation: taskLayout.orientation,
    };
  }, [taskLayout]);

  const graphContainerRef = useRef<HTMLDivElement | null>(null);
  const [baseGraphScale, setBaseGraphScale] = useState(1);
  const [userGraphScale, setUserGraphScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const draggingRef = useRef(false);
  const dragStartRef = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null);
  const userAdjustedRef = useRef(false);
  const [isTaskGraphDragging, setIsTaskGraphDragging] = useState(false);
  const clampUserScale = useCallback((value: number) => Math.min(3, Math.max(0.6, value)), []);
  const graphScale = baseGraphScale * userGraphScale;
  const nearlyEqual = useCallback((a: number, b: number, eps = 0.5) => Math.abs(a - b) < eps, []);
  const markUserAdjusted = () => {
    userAdjustedRef.current = true;
  };

  const centerGraph = useCallback(() => {
    if (!graphContainerRef.current || !taskGraphView) return;
    const rect = graphContainerRef.current.getBoundingClientRect();
    const scaledWidth = taskGraphView.viewWidth * graphScale;
    const scaledHeight = taskGraphView.viewHeight * graphScale;
    const nextPan = {
      x: (rect.width - scaledWidth) / 2,
      y: (rect.height - scaledHeight) / 2,
    };
    setPan(prev => {
      if (nearlyEqual(prev.x, nextPan.x, 0.3) && nearlyEqual(prev.y, nextPan.y, 0.3)) return prev;
      return nextPan;
    });
  }, [graphScale, nearlyEqual, taskGraphView]);

  const recomputeBaseScale = useCallback(() => {
    if (!graphContainerRef.current || !taskGraphView) return;
    const rect = graphContainerRef.current.getBoundingClientRect();
    const padding = 32;
    const availableWidth = Math.max(160, rect.width - padding * 2);
    const availableHeight = Math.max(260, rect.height - padding * 2);
    if (!taskGraphView.viewWidth || !taskGraphView.viewHeight) return;
    const fitScale = Math.min(availableWidth / taskGraphView.viewWidth, availableHeight / taskGraphView.viewHeight);
    const density = taskGraphView.taskCount ? taskGraphView.dependencyCount / taskGraphView.taskCount : 0;
    const sizeFactor =
      taskGraphView.taskCount <= 3 ? 1.18 : taskGraphView.taskCount <= 6 ? 1.05 : taskGraphView.taskCount <= 10 ? 0.95 : 0.82;
    const dependencyFactor = density > 1.3 ? 0.86 : density > 0.8 ? 0.92 : density > 0.4 ? 0.98 : 1.06;
    const orientationFactor = taskGraphView.orientation === 'horizontal' && taskGraphView.taskCount <= 4 ? 1.05 : 1;
    const target = Math.min(2.2, Math.max(0.7, fitScale * sizeFactor * dependencyFactor * orientationFactor));
    setBaseGraphScale(target);
    if (!userAdjustedRef.current) {
      setUserGraphScale(1);
    }
  }, [taskGraphView]);

  useEffect(() => {
    userAdjustedRef.current = false;
    recomputeBaseScale();
  }, [recomputeBaseScale]);

  useLayoutEffect(() => {
    const onResize = () => recomputeBaseScale();
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [recomputeBaseScale]);

  useLayoutEffect(() => {
    if (!taskGraphView || userAdjustedRef.current) return;
    const rect = graphContainerRef.current?.getBoundingClientRect();
    if (!rect) return;
    centerGraph();
  }, [baseGraphScale, centerGraph, graphScale, taskGraphView, userGraphScale]);

  useEffect(() => {
    const el = graphContainerRef.current;
    if (!el) return undefined;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const factor = e.deltaY > 0 ? 1 / 1.06 : 1.06;
      markUserAdjusted();
      setUserGraphScale(prev => clampUserScale(prev * factor));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [clampUserScale]);

  useEffect(() => {
    if (userAdjustedRef.current) return;
    centerGraph();
  }, [centerGraph, taskGraphView]);

  const taskContentOffset = useMemo(() => {
    if (!taskLayout || !taskLayout.nodes.length || !taskGraphView) return { x: 0, y: 0 };
    const minX = Math.min(...taskLayout.nodes.map(n => n.x));
    const minY = Math.min(...taskLayout.nodes.map(n => n.y));
    const maxX = Math.max(...taskLayout.nodes.map(n => n.x + n.width));
    const maxY = Math.max(...taskLayout.nodes.map(n => n.y + n.height));
    const contentWidth = maxX - minX;
    const contentHeight = maxY - minY;
    const extraX = Math.max(0, taskGraphView.viewWidth - contentWidth);
    const extraY = Math.max(0, taskGraphView.viewHeight - contentHeight);
    return {
      x: -minX + extraX / 2,
      y: -minY + extraY / 2,
    };
  }, [taskGraphView, taskLayout]);

  const hasTasks = (step?.tasks?.length || 0) > 0;
  const configTasks = config?.tasks ?? EMPTY_TASK_DEFINITIONS;
  const stepDefinition = pipelineDefinition?.steps?.find(s => s.name === step?.name) || null;
  const stepGoal = config?.goal || stepDefinition?.goal || '';
  const stepScript = config?.script || stepDefinition?.script || '';
  const taskDefsFromDefinition = stepDefinition?.tasks ?? EMPTY_TASK_DEFINITIONS;
  const allTaskDefinitions = useMemo(() => {
    if (configTasks.length && taskDefsFromDefinition.length) {
      const map = new Map<string, TaskDefinition>();
      taskDefsFromDefinition.forEach(def => map.set(def.name, def));
      configTasks.forEach(def => map.set(def.name, def));
      return Array.from(map.values());
    }
    return configTasks.length ? configTasks : taskDefsFromDefinition;
  }, [configTasks, taskDefsFromDefinition]);
  const hasTaskDefinitions = allTaskDefinitions.length > 0;
  const isSingleSyntheticTask = hasTasks && !hasTaskDefinitions && (step?.tasks?.length || 0) === 1;
  const showStepLevelInfo = !hasTasks || isSingleSyntheticTask;
  const effectiveActiveTaskName = (() => {
    const tasks = step?.tasks || [];
    if (!tasks.length || isSingleSyntheticTask) return null;
    if (activeTaskName && tasks.some(task => task.task_name === activeTaskName)) return activeTaskName;
    return tasks.length === 1 ? tasks[0].task_name : null;
  })();

  const selectedTask = useMemo(() => {
    if (!effectiveActiveTaskName) return null;
    return (step?.tasks || []).find(task => task.task_name === effectiveActiveTaskName) || null;
  }, [effectiveActiveTaskName, step?.tasks]);

  const selectedTaskDefinition = useMemo(() => {
    if (!effectiveActiveTaskName) return null;
    return allTaskDefinitions.find(def => def.name === effectiveActiveTaskName) || null;
  }, [effectiveActiveTaskName, allTaskDefinitions]);

  const onMouseDownGraph = (e: React.MouseEvent) => {
    markUserAdjusted();
    draggingRef.current = true;
    setIsTaskGraphDragging(true);
    dragStartRef.current = { x: e.clientX, y: e.clientY, panX: pan.x, panY: pan.y };
  };
  const onMouseMoveGraph = (e: React.MouseEvent) => {
    if (!draggingRef.current || !dragStartRef.current) return;
    const dx = e.clientX - dragStartRef.current.x;
    const dy = e.clientY - dragStartRef.current.y;
    setPan({ x: dragStartRef.current.panX + dx, y: dragStartRef.current.panY + dy });
  };
  const endDrag = () => {
    draggingRef.current = false;
    setIsTaskGraphDragging(false);
    dragStartRef.current = null;
  };

  const statusMeta = step ? getStatusMeta(step.status, true) : null;

  if (!step) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] px-3 py-6">
      <div className="w-full max-w-6xl bg-white dark:bg-slate-900 rounded-xl shadow-xl border border-[var(--border-primary)] flex flex-col max-h-[90vh] overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-primary)] bg-[var(--bg-primary)]">
          <div className="flex flex-wrap items-center gap-3">
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step: {step.name}</h3>
            {statusMeta && <span className={`runner-pill border ${statusMeta.pillClass} text-xs`}>{statusMeta.text}</span>}
            {step.duration && <span className="runner-pill runner-pill--muted text-xs">Duration: {step.duration}</span>}
          </div>
          <div className="flex items-center gap-2">
            <button className="runner-pill runner-pill--ghost" type="button" onClick={onViewLogs}>
              View Logs
            </button>
            <button className="runner-pill runner-pill--ghost" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-auto p-5 space-y-4 bg-[var(--bg-secondary)]">
          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
            <div className="flex items-center justify-between mb-2">
              <p className="text-sm font-semibold text-[var(--text-primary)]">Execution Flow</p>
              <span className="text-xs text-[var(--text-secondary)]">
                {step.tasks.length} task{step.tasks.length === 1 ? '' : 's'}
              </span>
            </div>
            <div
              className="relative h-[420px] lg:h-[480px] w-full overflow-hidden rounded border border-[var(--border-primary)] bg-white dark:bg-slate-950 flex items-center justify-center"
              ref={graphContainerRef}
              onMouseDown={onMouseDownGraph}
              onMouseMove={onMouseMoveGraph}
              onMouseUp={endDrag}
              onMouseLeave={endDrag}
            >
              {taskLayout && taskLayout.nodes.length && taskGraphView ? (
                <>
                  <div className="absolute right-3 top-3 z-20 flex gap-2">
                    <button
                      type="button"
                      className="h-9 w-9 rounded-full bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] border border-[var(--border-primary)]"
                      aria-label="Zoom out"
                      onClick={() => {
                        markUserAdjusted();
                        setUserGraphScale(prev => clampUserScale(prev / 1.15));
                      }}
                    >
                      −
                    </button>
                    <button
                      type="button"
                      className="h-9 w-9 rounded-full bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] border border-[var(--border-primary)]"
                      aria-label="Zoom in"
                      onClick={() => {
                        markUserAdjusted();
                        setUserGraphScale(prev => clampUserScale(prev * 1.15));
                      }}
                    >
                      +
                    </button>
                  </div>
                  <svg
                    width="100%"
                    height="100%"
                    viewBox={`0 0 ${taskGraphView.viewWidth} ${taskGraphView.viewHeight}`}
                    preserveAspectRatio="xMidYMid meet"
                    className="p-6"
                    style={{
                      transform: `translate(${pan.x}px, ${pan.y}px) scale(${graphScale})`,
                      transformOrigin: 'center center',
                      margin: '0 auto',
                      display: 'block',
                      cursor: isTaskGraphDragging ? 'grabbing' : 'grab',
                    }}
                  >
                    <g transform={`translate(${taskContentOffset.x}, ${taskContentOffset.y})`}>
                      {taskLayout.edges.map(edge => {
                        const [start, c1, c2, end] = edge.points;
                        return (
                          <path
                            key={edge.id}
                            d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
                            fill="none"
                            stroke={getGraphStatusColor(edge.status)}
                            strokeWidth={2.2}
                            strokeOpacity={0.75}
                            strokeLinecap="round"
                          />
                        );
                      })}
                      {taskLayout.nodes.map(node => (
                        <TaskNodeRenderer
                          key={node.data.id}
                          task={node}
                          stepName={step.name}
                          fontSize={13}
                          glyphSize={18}
                          onTaskClick={(_, taskName) => setActiveTaskName(taskName)}
                        />
                      ))}
                    </g>
                  </svg>
                </>
              ) : (
                <div className="text-sm text-[var(--text-secondary)]">No task graph available.</div>
              )}
            </div>
          </div>

          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 space-y-4">
            <p className="text-sm font-semibold text-[var(--text-primary)]">Configuration</p>
            <div className="space-y-3 text-sm text-[var(--text-primary)]">
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Image</span>
                  <span className="font-mono break-words">{config?.image || '—'}</span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Secrets</span>
                  <div className="flex-1 flex flex-wrap gap-1">
                    {(config?.secrets || []).map(secret => (
                      <span key={secret} className="px-2 py-1 rounded border border-[var(--border-primary)] text-[11px]">
                        {secret}
                      </span>
                    ))}
                    {!config?.secrets?.length && <span className="text-[var(--text-secondary)]">—</span>}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Volumes</span>
                  <div className="flex-1 space-y-1">
                    {(config?.volumes || []).map(volume => (
                      <div key={volume} className="font-mono text-xs bg-white dark:bg-slate-900 border border-[var(--border-primary)] rounded px-2 py-1">
                        {volume}
                      </div>
                    ))}
                    {!config?.volumes?.length && <span className="text-[var(--text-secondary)]">—</span>}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Include</span>
                  <span className="font-mono break-words">{config?.include || '—'}</span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">LLM profile</span>
                  <span className="font-mono break-words">{config?.llm_profile || 'Inherited'}</span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Runtime pool</span>
                  <span className="font-mono break-words">{config?.runtime_pool || 'Inherited'}</span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Variables</span>
                  <div className="flex-1 space-y-1">
                    {config?.variables &&
                      Object.entries(config.variables).map(([key, value]) => (
                        <div key={key} className="font-mono text-xs bg-white dark:bg-slate-900 border border-[var(--border-primary)] rounded px-2 py-1">
                          {key}: {value}
                        </div>
                      ))}
                    {(!config?.variables || Object.keys(config.variables || {}).length === 0) && (
                      <span className="text-[var(--text-secondary)]">—</span>
                    )}
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-secondary)]">
                <div className="flex items-center gap-2">
                  <span>Ignore failure</span>
                  <span className="text-[var(--text-primary)] font-semibold">{config?.ignore_failure ? 'true' : 'false'}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span>Sync</span>
                  <span className="text-[var(--text-primary)] font-semibold">{config?.sync ? 'true' : 'false'}</span>
                </div>
                {config?.llm_output_sharing !== undefined && (
                  <div className="flex items-center gap-2 col-span-2">
                    <span>LLM Output Sharing</span>
                    <span className="text-[var(--text-primary)] font-semibold">{config.llm_output_sharing ? 'true' : 'false'}</span>
                  </div>
                )}
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 space-y-3">
            <div className="flex items-center justify-between">
              <p className="text-sm font-semibold text-[var(--text-primary)]">Task details</p>
            {selectedTask && !isSingleSyntheticTask && (
              <span className={`runner-pill border ${getStatusMeta(selectedTask.status, true).pillClass} text-xs`}>
                {getStatusMeta(selectedTask.status, true).text}
              </span>
            )}
            {(isSingleSyntheticTask || !hasTasks) && (
              <span className={`runner-pill border ${getStatusMeta(step?.status, true).pillClass} text-xs`}>
                {getStatusMeta(step?.status, true).text}
              </span>
            )}
          </div>
          {!selectedTask && hasTasks && !isSingleSyntheticTask && (
            <p className="text-sm text-[var(--text-secondary)]">Click a task in the graph to see its details here.</p>
          )}
          {showStepLevelInfo && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="text-base font-semibold text-[var(--text-primary)]">{step?.name}</div>
                <div className="text-xs text-[var(--text-secondary)] font-mono">
                    Duration: {step ? formatStepDuration(step) : '—'}
                  </div>
                </div>
                {(stepGoal || stepScript) && (
                  <div className="space-y-2">
                    {stepGoal && <p className="text-sm text-[var(--text-primary)]">Goal: <span className="text-[var(--text-secondary)]">{stepGoal}</span></p>}
                    {stepScript && (
                      <div>
                        <p className="text-xs text-[var(--text-secondary)] mb-1">Script</p>
                        <pre className="text-xs font-mono whitespace-pre-wrap bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-2 text-[var(--text-primary)]">
                          {stepScript}
                        </pre>
                      </div>
                    )}
                  </div>
                )}
                <div className="grid gap-3 sm:grid-cols-2 text-sm text-[var(--text-primary)]">
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Depends on</span>
                    <span className="font-mono">{(step?.depends_on || []).join(', ') || 'None'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Started</span>
                    <span className="font-mono">{step?.started_at || '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Finished</span>
                    <span className="font-mono">{step?.finished_at || '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Ignore failure</span>
                    <span className="font-mono">{step?.configuration?.ignore_failure ? 'true' : 'false'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Sync</span>
                    <span className="font-mono">{step?.configuration?.sync ? 'true' : 'false'}</span>
                  </div>
                  {step?.configuration?.include && (
                    <div className="flex items-center gap-2 sm:col-span-2">
                      <span className="text-[var(--text-secondary)]">Include</span>
                      <span className="font-mono">{step.configuration.include}</span>
                    </div>
                  )}
                  {step?.configuration?.variables && Object.keys(step.configuration.variables).length > 0 && (
                    <div className="sm:col-span-2 space-y-1">
                      <p className="text-xs text-[var(--text-secondary)]">Variables</p>
                      {Object.entries(step.configuration.variables).map(([key, value]) => (
                        <div key={key} className="font-mono bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-1 text-xs">
                          {key}: {value}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                {configTasks.length > 0 && (
                  <div className="space-y-2">
                    <p className="text-xs text-[var(--text-secondary)] uppercase tracking-wide">Step directives</p>
                    <div className="space-y-2">
                      {configTasks.map(def => (
                        <div key={def.name} className="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm font-semibold text-[var(--text-primary)]">{def.name || 'Unnamed task'}</span>
                            <span className="text-xs text-[var(--text-secondary)]">
                              {def.depends_on?.length ? `Depends on: ${def.depends_on.join(', ')}` : 'No dependencies'}
                            </span>
                          </div>
                          {def.goal && <p className="text-xs text-[var(--text-secondary)]">Goal: {def.goal}</p>}
                          {def.script && (
                            <pre className="text-xs font-mono whitespace-pre-wrap bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded px-2 py-2 text-[var(--text-primary)]">
                              {def.script}
                            </pre>
                          )}
                          {def.variables && Object.keys(def.variables).length > 0 && (
                            <div className="space-y-1 text-xs">
                              <p className="text-[var(--text-secondary)]">Variables</p>
                              {Object.entries(def.variables).map(([key, value]) => (
                                <div key={key} className="font-mono bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded px-2 py-1">
                                  {key}: {value}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          {selectedTask && !isSingleSyntheticTask && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="text-base font-semibold text-[var(--text-primary)]">{selectedTask.task_name}</div>
                <div className="text-xs text-[var(--text-secondary)] font-mono">
                  Duration: {formatTaskDuration(selectedTask, deriveTaskGraphStatus(selectedTask, step?.status)) || '—'}
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2 text-sm text-[var(--text-primary)]">
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Dependencies</span>
                    <span className="font-mono">{(selectedTaskDefinition?.depends_on || []).join(', ') || 'None'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Exit code</span>
                    <span className="font-mono">{selectedTask.exit_code ?? '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Started</span>
                    <span className="font-mono">{selectedTask.started_at || '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Finished</span>
                    <span className="font-mono">{selectedTask.finished_at || '—'}</span>
                  </div>
                </div>
                {selectedTaskDefinition?.goal && (
                  <div className="text-sm text-[var(--text-secondary)]">
                    Goal: <span className="text-[var(--text-primary)]">{selectedTaskDefinition.goal}</span>
                  </div>
                )}
                {selectedTaskDefinition?.script && (
                  <div>
                    <p className="text-xs text-[var(--text-secondary)] mb-1">Script</p>
                    <pre className="text-xs font-mono whitespace-pre-wrap bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-2 text-[var(--text-primary)]">
                      {selectedTaskDefinition.script}
                    </pre>
                  </div>
                )}
                {selectedTaskDefinition?.variables && Object.keys(selectedTaskDefinition.variables).length > 0 && (
                  <div className="space-y-1">
                    <p className="text-xs text-[var(--text-secondary)]">Variables</p>
                    {Object.entries(selectedTaskDefinition.variables).map(([key, value]) => (
                      <div key={key} className="font-mono bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-1 text-xs">
                        {key}: {value}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export function PipelineDefinitionModal({
  open,
  pipelineName,
  yamlText,
  definition,
  onClose,
}: {
  open: boolean;
  pipelineName: string;
  yamlText?: string | null;
  definition?: PipelineDefinition;
  onClose: () => void;
}) {
  const content = useMemo(() => {
    if (yamlText && yamlText.trim().length > 0) return yamlText;
    if (definition) {
      try {
        return yaml.dump(definition);
      } catch {
        return JSON.stringify(definition, null, 2);
      }
    }
    return 'Pipeline definition is not available for this run.';
  }, [definition, yamlText]);

  if (!open) return null;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      return true;
    } catch {
      return false;
    }
  };

  const handleDownload = () => {
    const blob = new Blob([content], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${pipelineName || 'pipeline'}.yaml`;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)]">
      <div className="bg-[var(--bg-primary)] rounded-xl shadow-xl w-full max-w-5xl max-h-[85vh] overflow-hidden border border-[var(--border-primary)]">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border-primary)]">
          <div>
            <p className="text-sm font-semibold text-[var(--text-primary)]">Pipeline definition</p>
            <p className="text-xs text-[var(--text-secondary)]">{pipelineName}</p>
          </div>
          <div className="flex items-center gap-2">
            <button className="glass-button-subtle" type="button" onClick={handleCopy}>
              Copy
            </button>
            <button className="glass-button-subtle" type="button" onClick={handleDownload}>
              Download
            </button>
            <button className="glass-button-primary" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>
        <div className="p-4 bg-[var(--bg-secondary)] h-[70vh] overflow-auto">
          <pre className="text-xs text-[var(--text-primary)] whitespace-pre-wrap leading-5">{content}</pre>
        </div>
      </div>
    </div>
  );
}
