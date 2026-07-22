export const RUN_LOG_LEVELS = ['info', 'warn', 'error', 'debug'] as const;

export type RunLogLevel = (typeof RUN_LOG_LEVELS)[number];

export type RunLogLine = {
  id: number;
  timestamp: string;
  line: string;
  source?: string;
  stream?: string;
  level?: string;
  step_name?: string;
  task_name?: string;
  runner_id?: string;
  request_id?: string;
  traceparent?: string;
  metadata?: Record<string, unknown>;
};

export type EnrichedRunLogLine = RunLogLine & {
  level?: string;
  step?: string;
  task?: string;
};

export type RunLogsHashState = {
  steps: string[];
  tasks: string[];
  levels: Set<string>;
  wrap: boolean;
  structured: boolean;
  agentOnly: boolean;
  shortView: boolean;
  search: string;
};

export type RunLogFilter = {
  selectedSteps: Set<string>;
  selectedTasks?: Set<string>;
  selectedLevels: Set<string>;
  agentOnly: boolean;
  searchText: string;
};

export function normalizeRunLogLevel(level: string | undefined): string {
  const normalized = (level || 'info').trim().toLowerCase();
  if (normalized === 'warning') return 'warn';
  if (normalized === 'fatal' || normalized === 'panic') return 'error';
  if (normalized === 'trace') return 'debug';
  return normalized || 'info';
}

export function parseRunLogLine(line: string): Pick<EnrichedRunLogLine, 'level' | 'step' | 'task'> {
  if (!line) return {};
  try {
    const jsonStart = line.indexOf('{');
    if (jsonStart !== -1) {
      const parsed = JSON.parse(line.slice(jsonStart)) as {
        level?: unknown;
        output_level?: unknown;
        severity?: unknown;
        step?: unknown;
        step_name?: unknown;
        task?: unknown;
        task_name?: unknown;
        meta?: {
          level?: unknown;
          output_level?: unknown;
          severity?: unknown;
          step?: unknown;
          step_name?: unknown;
          task?: unknown;
          task_name?: unknown;
        };
      };
      const rawLevel = String(
        parsed.output_level || parsed.severity || parsed.level || parsed.meta?.output_level || parsed.meta?.severity || parsed.meta?.level || ''
      ).trim();
      const step = parsed.step || parsed.step_name || parsed.meta?.step || parsed.meta?.step_name;
      const task = parsed.task || parsed.task_name || parsed.meta?.task || parsed.meta?.task_name;
      return {
        level: rawLevel ? normalizeRunLogLevel(rawLevel) : undefined,
        step: typeof step === 'string' && step ? step : undefined,
        task: typeof task === 'string' && task ? task : undefined,
      };
    }
  } catch {
    // Plain-text logs are handled by the fallback matcher.
  }
  const levelMatch = line.match(/\b(info|warn|warning|error|debug)\b/i);
  return { level: levelMatch ? normalizeRunLogLevel(levelMatch[1]) : undefined };
}

export function enrichRunLogLines(lines: RunLogLine[]): EnrichedRunLogLine[] {
  return lines.map(line => {
    const parsed = parseRunLogLine(line.line || '');
    return {
      ...line,
      level: parsed.level || (line.level ? normalizeRunLogLevel(line.level) : undefined),
      step: parsed.step || line.step_name || undefined,
      task: parsed.task || line.task_name || undefined,
    };
  });
}

export function isAgentRunLogLine(line: EnrichedRunLogLine): boolean {
  const content = (line.line || '').toLowerCase();
  return content.includes('agent') || (line.step || '').toLowerCase().includes('agent');
}

export function getPresentRunLogLevels(lines: EnrichedRunLogLine[]): Set<string> {
  const levels = new Set<string>();
  lines.forEach(line => {
    levels.add(normalizeRunLogLevel(line.level));
    if (isAgentRunLogLine(line)) levels.add('agent');
  });
  return levels;
}

export function filterRunLogLines(lines: EnrichedRunLogLine[], filter: RunLogFilter): EnrichedRunLogLine[] {
  const stepFilterActive = filter.selectedSteps.size > 0;
  const taskFilterActive = Boolean(filter.selectedTasks?.size);
  const searchTerm = filter.searchText.trim().toLowerCase();
  return lines.filter(line => {
    if (stepFilterActive && (!line.step || !filter.selectedSteps.has(line.step))) return false;
    if (taskFilterActive && (!line.task || !filter.selectedTasks?.has(line.task))) return false;
    if (filter.agentOnly && !isAgentRunLogLine(line)) return false;
    if (filter.selectedLevels.size > 0 && !filter.selectedLevels.has(normalizeRunLogLevel(line.level))) return false;
    if (searchTerm && !(line.line || '').toLowerCase().includes(searchTerm)) return false;
    return true;
  });
}

export function parseRunLogsHash(
  hash: string,
  runID?: string,
  levelOrder: readonly string[] = RUN_LOG_LEVELS
): RunLogsHashState | null {
  if (!hash || !hash.includes('/logs/')) return null;
  const trimmed = hash.replace(/^#/, '');
  const [pathPart, queryPart] = trimmed.split('?');
  const parts = pathPart.split('/').filter(Boolean).map(decodeURIComponent);
  const logsIndex = parts.indexOf('logs');
  if (logsIndex === -1) return null;
  const hashRunID = parts[logsIndex - 1];
  if (runID && hashRunID && hashRunID !== runID) return null;
  const segments = parts.slice(logsIndex + 1);
  if (segments.length < 6) return null;

  const [stepsSegment, levelsSegment, wrapSegment, structuredSegment, agentSegment, shortSegment] = segments;
  const steps = stepsSegment && stepsSegment !== 'all' ? stepsSegment.split(',').filter(Boolean) : [];
  const levels = levelsSegment && levelsSegment !== 'all'
    ? levelsSegment.split(',').filter(Boolean).map(normalizeRunLogLevel)
    : [];
  const order = new Map(levelOrder.map((level, index) => [level, index]));
  levels.sort((left, right) => (order.get(left) ?? levelOrder.length) - (order.get(right) ?? levelOrder.length));

  const query = queryPart ? new URLSearchParams(queryPart) : new URLSearchParams();
  const tasksSegment = query.get('task') || query.get('tasks') || '';
  const tasks = tasksSegment ? tasksSegment.split(',').filter(Boolean) : [];

  return {
    steps,
    tasks,
    levels: new Set(levels),
    wrap: wrapSegment !== 'unwrap',
    structured: structuredSegment !== 'unstructured',
    agentOnly: agentSegment === 'agent',
    shortView: shortSegment !== 'full',
    search: query.get('search') || '',
  };
}

export function buildRunLogsHash({
  currentHash,
  runID,
  selectedSteps,
  selectedTasks = new Set(),
  selectedLevels,
  wrap,
  structured,
  agentOnly,
  shortView,
  searchText,
  levelOrder = RUN_LOG_LEVELS,
}: {
  currentHash: string;
  runID: string;
  selectedSteps: Set<string>;
  selectedTasks?: Set<string>;
  selectedLevels: Set<string>;
  wrap: boolean;
  structured: boolean;
  agentOnly: boolean;
  shortView: boolean;
  searchText: string;
  levelOrder?: readonly string[];
}): string | null {
  if (!runID) return null;
  const trimmed = (currentHash || '#').replace(/^#/, '');
  const [pathPart] = trimmed.split('?');
  const parts = pathPart.split('/').filter(Boolean).map(decodeURIComponent);
  const logsIndex = parts.indexOf('logs');
  const prefix = logsIndex !== -1 ? parts.slice(0, logsIndex) : ['pipelineruns', 'events', runID];
  if (!prefix.includes(runID)) prefix.push(runID);

  const stepsSegment = selectedSteps.size ? encodeURIComponent(Array.from(selectedSteps).join(',')) : 'all';
  const orderedLevels = selectedLevels.size === 0 ? [] : levelOrder.filter(level => selectedLevels.has(level));
  const levelsSegment = orderedLevels.length ? encodeURIComponent(orderedLevels.join(',')) : 'all';
  const hashPath = `#/${[
    ...prefix.map(encodeURIComponent),
    'logs',
    stepsSegment,
    levelsSegment,
    wrap ? 'wrap' : 'unwrap',
    structured ? 'structured' : 'unstructured',
    agentOnly ? 'agent' : 'all',
    shortView ? 'short' : 'full',
  ].join('/')}`;
  const queryParts = [];
  if (searchText) queryParts.push(`search=${encodeURIComponent(searchText)}`);
  if (selectedTasks.size) queryParts.push(`task=${encodeURIComponent(Array.from(selectedTasks).join(','))}`);
  return `${hashPath}${queryParts.length ? `?${queryParts.join('&')}` : ''}`;
}

export function formatRunLogDownload(lines: EnrichedRunLogLine[]): string {
  return lines
    .map(line => {
      const timestamp = line.timestamp ? new Date(line.timestamp) : null;
      const parts = [timestamp && !Number.isNaN(timestamp.getTime()) ? timestamp.toISOString() : ''];
      if (line.step) parts.push(`[${line.step}]`);
      if (line.task) parts.push(`[${line.task}]`);
      if (line.level) parts.push(normalizeRunLogLevel(line.level).toUpperCase());
      parts.push('-', line.line || '');
      return parts.join(' ');
    })
    .join('\n');
}
