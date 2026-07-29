import { type KeyboardEvent } from 'react';
import type {
  GraphLayout,
  GraphLayoutNode,
  GraphPoint,
  GraphStatus,
  GraphStep,
  GraphTask,
} from './contracts';
import { getGraphStatusColor, getGraphStatusLabel } from './graphLayout';
import {
  matchesRunGraphEntityFilter,
  type RunGraphStatusFilter,
} from './runGraphModel';
import { getStatusMeta } from './statusPresentation';

export const TASK_MIN_WIDTH = 160;
export const TASK_MAX_WIDTH = 280;
export const TASK_HEIGHT = 48;
export const TASK_NODE_HEIGHT = 68;

export type StatusGlyphVariant = 'default' | 'dot';

function activateWithKeyboard(event: KeyboardEvent, action: () => void) {
  if (event.key !== 'Enter' && event.key !== ' ') return;
  event.preventDefault();
  event.stopPropagation();
  action();
}

export function StepNode({
  node,
  selected,
  dimmed,
  onActivate,
  onOpenDetails,
  opensStepLogs,
  statusVariant,
  statusColorOverride,
}: {
  node: GraphLayoutNode<GraphStep>;
  selected: boolean;
  dimmed: boolean;
  onActivate: () => void;
  onOpenDetails: () => void;
  opensStepLogs: boolean;
  statusVariant: StatusGlyphVariant;
  statusColorOverride?: string;
}) {
  const taskCount = node.data.tasks.length;
  const labelLines = splitLabel(node.data.name, 22);
  const status = getGraphStatusLabel(node.data.status);
  const includeText = node.data.includeLabel || '';
  const includeChipWidth = includeText.toLowerCase().includes('pipeline') ? 94 : 78;
  const chipY = node.height - 25;
  return (
    <g
      transform={`translate(${node.x}, ${node.y})`}
      className={`run-graph-svg-node${selected ? ' selected' : ''}${dimmed ? ' dimmed' : ''}`}
      role="button"
      tabIndex={0}
      aria-label={
        opensStepLogs
          ? `Open logs for ${node.data.name} step, ${status}`
          : `${selected ? 'Collapse' : 'Reveal'} ${node.data.name} step, ${status}`
      }
      data-run-graph-node
      onClick={event => {
        event.stopPropagation();
        onActivate();
      }}
      onMouseDown={event => event.stopPropagation()}
      onKeyDown={event => activateWithKeyboard(event, onActivate)}
    >
      <rect className="run-graph-node-card" width={node.width} height={node.height} rx={8} />
      <NodeDetailsButton
        x={node.width - 25}
        y={10}
        label={`Open full details for ${node.data.name} step`}
        onOpenDetails={onOpenDetails}
      />
      <GraphStatusGlyph
        status={node.data.status}
        x={20}
        y={23}
        size={17}
        variant={statusVariant}
        colorOverride={statusColorOverride}
      />
      <text className="run-graph-node-title" x={36} y={24}>
        {labelLines.slice(0, 2).map((line, index) => (
          <tspan key={line} x={36} dy={index === 0 ? 0 : 14}>{line}</tspan>
        ))}
      </text>
      <text className="run-graph-node-meta" x={16} y={labelLines.length > 1 ? 58 : 50}>
        {node.data.duration || '0s'} - {status.toLowerCase()}
      </text>
      {includeText ? (
        <>
          <rect className="run-graph-include-chip" x={16} y={chipY} width={includeChipWidth} height={17} rx={8.5} />
          <text className="run-graph-include-chip-text" x={16 + includeChipWidth / 2} y={chipY + 12}>
            {includeText}
          </text>
        </>
      ) : null}
      {taskCount > 0 ? (
        <>
          <rect className="run-graph-task-pill" x={node.width - 68} y={chipY} width={54} height={17} rx={9} />
          <text className="run-graph-task-pill-text" x={node.width - 41} y={chipY + 12}>
            {taskCount} task{taskCount === 1 ? '' : 's'}
          </text>
        </>
      ) : null}
      {node.data.childRun && !includeText && !taskCount ? (
        <text className="run-graph-node-kicker" x={node.width - 16} y={chipY + 12}>
          Child run
        </text>
      ) : null}
    </g>
  );
}

export function TaskCardNode({
  task,
  stepName,
  selected,
  dimmed,
  onActivate,
  onOpenDetails,
  statusColorOverride,
}: {
  task: GraphLayoutNode<GraphTask>;
  stepName: string;
  selected: boolean;
  dimmed: boolean;
  onActivate: () => void;
  onOpenDetails: () => void;
  statusColorOverride?: string;
}) {
  const lines = splitLabel(task.data.name, 21);
  return (
    <g
      transform={`translate(${task.x}, ${task.y})`}
      className={`run-graph-svg-node run-graph-task-node${selected ? ' selected' : ''}${dimmed ? ' dimmed' : ''}`}
      role="button"
      tabIndex={0}
      data-run-graph-node
      aria-label={`Open logs for ${task.data.name} task in ${stepName}, ${getGraphStatusLabel(task.data.status)}`}
      onClick={event => {
        event.stopPropagation();
        onActivate();
      }}
      onMouseDown={event => event.stopPropagation()}
      onKeyDown={event => activateWithKeyboard(event, onActivate)}
    >
      <rect className="run-graph-node-card" width={task.width} height={task.height} rx={8} />
      <NodeDetailsButton
        x={task.width - 24}
        y={9}
        label={`Open full details for ${task.data.name} task in ${stepName}`}
        onOpenDetails={onOpenDetails}
      />
      <GraphStatusGlyph
        status={task.data.status}
        x={19}
        y={20}
        size={15}
        colorOverride={statusColorOverride}
      />
      <text className="run-graph-node-title" x={34} y={22}>
        {lines.slice(0, 2).map((line, index) => (
          <tspan key={line} x={34} dy={index === 0 ? 0 : 13}>{line}</tspan>
        ))}
      </text>
      <text className="run-graph-node-meta" x={16} y={task.height - 13}>
        {task.data.duration || '0s'} - {getGraphStatusLabel(task.data.status).toLowerCase()}
      </text>
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
  const centerX = task.width / 2;
  const contentCenterY = task.height / 2;
  const statusIconCenterY = Math.max(glyphSize / 2 + 8, contentCenterY - 8);
  const textY = Math.min(task.height - 8, contentCenterY + fontSize / 2 + 10);
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
        size={glyphSize + 2}
        colorOverride={statusColorOverride}
      />
      <text x={centerX} y={textY} textAnchor="middle" style={{ fontSize, fontWeight: 700 }}>
        <tspan style={{ fill: 'var(--text-primary)' }}>{task.data.name}</tspan>
        <tspan style={{ fill: 'var(--text-secondary)', fontWeight: 700 }}>{` - ${durationLabel}`}</tspan>
      </text>
    </g>
  );
}

function NodeDetailsButton({
  x,
  y,
  label,
  onOpenDetails,
}: {
  x: number;
  y: number;
  label: string;
  onOpenDetails: () => void;
}) {
  return (
    <g
      className="run-graph-node-detail-button"
      transform={`translate(${x}, ${y})`}
      role="button"
      tabIndex={0}
      aria-label={label}
      onClick={event => {
        event.stopPropagation();
        onOpenDetails();
      }}
      onMouseDown={event => event.stopPropagation()}
      onKeyDown={event => activateWithKeyboard(event, onOpenDetails)}
    >
      <circle cx={8} cy={8} r={8} />
      <text x={8} y={11}>i</text>
    </g>
  );
}

export function TaskRevealContext({
  context,
  layout,
  selectedStep,
  onSelectStep,
  filter,
  colorOverride,
}: {
  context: { upstream: GraphStep[]; downstream: GraphStep[] };
  layout: GraphLayout<GraphTask>;
  selectedStep: GraphStep;
  onSelectStep: (step: GraphStep) => void;
  filter: { searchQuery?: string; statusFilter?: RunGraphStatusFilter };
  colorOverride?: string;
}) {
  const firstTasks = firstTaskNodes(layout);
  const lastTasks = lastTaskNodes(layout);
  const upstream = context.upstream[0] || null;
  const downstream = context.downstream[0] || null;
  const upstreamBox = upstream ? { x: 18, y: 158, width: 154, height: 64 } : null;
  const downstreamBox = downstream ? { x: 1048, y: 158, width: 154, height: 64 } : null;

  return (
    <>
      {upstreamBox && upstream ? (
        <>
          {firstTasks.map(task => (
            <GraphEdgePath
              key={`upstream-${task.data.id}`}
              points={contextEdgePoints(
                { x: upstreamBox.x + upstreamBox.width, y: upstreamBox.y + upstreamBox.height / 2 },
                { x: task.x, y: task.y + task.height / 2 }
              )}
              status={selectedStep.status}
              context
              colorOverride={colorOverride}
            />
          ))}
          <ContextStepNode
            step={upstream}
            box={upstreamBox}
            dimmed={!matchesRunGraphEntityFilter(upstream, filter)}
            extraCount={context.upstream.length - 1}
            onActivate={() => onSelectStep(upstream)}
            colorOverride={colorOverride}
          />
        </>
      ) : null}
      {downstreamBox && downstream ? (
        <>
          {lastTasks.map(task => (
            <GraphEdgePath
              key={`downstream-${task.data.id}`}
              points={contextEdgePoints(
                { x: task.x + task.width, y: task.y + task.height / 2 },
                { x: downstreamBox.x, y: downstreamBox.y + downstreamBox.height / 2 }
              )}
              status={selectedStep.status}
              context
              colorOverride={colorOverride}
            />
          ))}
          <ContextStepNode
            step={downstream}
            box={downstreamBox}
            dimmed={!matchesRunGraphEntityFilter(downstream, filter)}
            extraCount={context.downstream.length - 1}
            onActivate={() => onSelectStep(downstream)}
            colorOverride={colorOverride}
          />
        </>
      ) : null}
    </>
  );
}

function ContextStepNode({
  step,
  box,
  dimmed,
  extraCount,
  onActivate,
  colorOverride,
}: {
  step: GraphStep;
  box: { x: number; y: number; width: number; height: number };
  dimmed: boolean;
  extraCount: number;
  onActivate: () => void;
  colorOverride?: string;
}) {
  const lines = splitLabel(step.name, 17);
  return (
    <g
      transform={`translate(${box.x}, ${box.y})`}
      className={`run-graph-svg-node run-graph-context-node${dimmed ? ' dimmed' : ''}`}
      role="button"
      tabIndex={0}
      data-run-graph-node
      aria-label={`Reveal ${step.name} step, ${getGraphStatusLabel(step.status)}`}
      onClick={event => {
        event.stopPropagation();
        onActivate();
      }}
      onMouseDown={event => event.stopPropagation()}
      onKeyDown={event => activateWithKeyboard(event, onActivate)}
    >
      <rect className="run-graph-node-card" width={box.width} height={box.height} rx={8} />
      <GraphStatusGlyph status={step.status} x={18} y={20} size={14} colorOverride={colorOverride} />
      <text className="run-graph-node-title" x={33} y={21}>
        {lines.slice(0, 2).map((line, index) => (
          <tspan key={line} x={33} dy={index === 0 ? 0 : 13}>{line}</tspan>
        ))}
      </text>
      <text className="run-graph-node-meta" x={16} y={box.height - 12}>
        {extraCount > 0 ? `+${extraCount} more` : 'context'}
      </text>
    </g>
  );
}

export function GraphEdgePath({
  points,
  status,
  active = false,
  dimmed = false,
  context = false,
  colorOverride,
}: {
  points: GraphPoint[];
  status: GraphStatus;
  active?: boolean;
  dimmed?: boolean;
  context?: boolean;
  colorOverride?: string;
}) {
  const [start, c1, c2, end] = points;
  if (!start || !c1 || !c2 || !end) return null;
  return (
    <path
      className={`run-graph-edge${active ? ' active' : ''}${dimmed ? ' dimmed' : ''}${context ? ' context' : ''}`}
      d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
      fill="none"
      stroke={statusColor(status, colorOverride)}
      strokeWidth={active ? 2.8 : context ? 1.7 : 2.1}
      strokeLinecap="round"
    />
  );
}

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
  const color = statusColor(status, colorOverride);
  const strokeWidth = Math.max(1.6, Math.min(2.4, size / 6.5));
  if (variant === 'dot') {
    return <circle cx={x} cy={y} r={size / 2} fill={color} opacity={opacity} />;
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
        <GraphStatusPath status={status} size={size} strokeWidth={strokeWidth} color={color} />
      </g>
    );
  }
  return (
    <g transform={`translate(${x}, ${y})`} opacity={opacity}>
      <GraphStatusPath status={status} size={size} strokeWidth={strokeWidth} color={color} />
    </g>
  );
}

function GraphStatusPath({ status, size, strokeWidth, color }: { status: GraphStatus; size: number; strokeWidth: number; color: string }) {
  return (
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
      <path d={getStatusMeta(status === 'failed' ? 'failure' : status, true).icon} />
    </svg>
  );
}

function firstTaskNodes(layout: GraphLayout<GraphTask>) {
  const incoming = new Set(layout.edges.map(edge => edge.to));
  const starts = layout.nodes.filter(node => !incoming.has(node.data.id));
  return starts.length ? starts : layout.nodes.slice(0, 1);
}

function lastTaskNodes(layout: GraphLayout<GraphTask>) {
  const outgoing = new Set(layout.edges.map(edge => edge.from));
  const ends = layout.nodes.filter(node => !outgoing.has(node.data.id));
  return ends.length ? ends : layout.nodes.slice(-1);
}

function contextEdgePoints(from: GraphPoint, to: GraphPoint): GraphPoint[] {
  const control = Math.max(42, Math.abs(to.x - from.x) * 0.42);
  return [
    from,
    { x: from.x + control, y: from.y },
    { x: to.x - control, y: to.y },
    to,
  ];
}

function splitLabel(label: string, max: number): string[] {
  if (label.length <= max) return [label];
  const dash = label.lastIndexOf('-', max);
  if (dash > 6) return [label.slice(0, dash), label.slice(dash + 1)];
  return [`${label.slice(0, Math.max(1, max - 1))}...`];
}

function statusColor(status: GraphStatus, override?: string) {
  return override || getGraphStatusColor(status);
}
