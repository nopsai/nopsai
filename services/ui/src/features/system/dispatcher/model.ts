import {
  asRecord,
  normalizeListPayload,
  normalizeNumber,
  normalizeStringArray,
  normalizeStringMap,
  readOptionalString,
  readString,
} from '../data.js';

export type Runner = {
  runnerId: string;
  scopes: string[];
  capacity: number;
  activeJobs: number;
  inflightJobs: number;
  lastHeartbeatUnix: number;
  allowDispatch: boolean;
  reachable: boolean;
  connectionStatus: string;
  metadata: Record<string, string>;
};

export type RunnerActiveRun = {
  runId: string;
  pipeline: string;
  parentStep?: string;
  triggerId?: string;
};

export type RunnerMeta = {
  connectionId: string;
  hostname: string;
  network: string;
  runtime: string;
  namespace: string;
  node: string;
  serviceAccount: string;
  disconnectedAt?: string;
  reachable: boolean;
  connectionStatus: string;
  activeRuns: RunnerActiveRun[];
};

export type DispatcherStatusState = {
  queuedJobs: number;
  runners: Runner[];
  routing: Record<string, string[]>;
  effectiveRouting: Record<string, string[]>;
  dispatcherError?: string;
  fetchedAt: number;
};

export type RunnerRoutingRow = {
  scope: string;
  runners: string[];
};

export type RunnerRouteAssignment = {
  runner: Runner;
  scopes: string[];
};

export type RunnerComposeTemplate = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  dispatcherAddress: string;
  networkMode: string;
  runnerImage: string;
  registryCredentialRefs: string[];
  registryHosts: string[];
  compose: string;
  command: string;
  bootstrapCommand: string;
  expiresAt: string;
  warnings: string[];
};

export type KubernetesRunnerManifestTemplate = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  namespace: string;
  serviceAccount: string;
  dispatcherAddress: string;
  runnerImage: string;
  registryCredentialRefs: string[];
  registryHosts: string[];
  manifest: string;
  command: string;
  bootstrapCommand: string;
  expiresAt: string;
  warnings: string[];
};

export type RunnerInstallRuntime = 'docker' | 'kubernetes';

export const DOCKER_RUNNER_IMAGE_REPOSITORY = 'ghcr.io/nopsai/nopsai-docker-runner';
export const KUBERNETES_RUNNER_IMAGE_REPOSITORY = 'ghcr.io/nopsai/nopsai-k8s-runner';
export const DEFAULT_DOCKER_RUNNER_IMAGE = runnerImageForVersion(DOCKER_RUNNER_IMAGE_REPOSITORY, 'dev');
export const DEFAULT_KUBERNETES_RUNNER_IMAGE = runnerImageForVersion(KUBERNETES_RUNNER_IMAGE_REPOSITORY, 'dev');

export function runnerImageForVersion(repository: string, version: string): string {
  return `${repository.trim().replace(/:+$/, '')}:${nopsaiImageTag(version)}`;
}

export function nopsaiImageTag(version: string): string {
  const tag = String(version || '').trim();
  if (!tag || tag.toLowerCase() === 'unknown' || tag.toLowerCase() === 'latest') return 'dev';
  return tag;
}

export type DispatcherRoutingDraftRow = {
  scope: string;
  runners: string;
};

export function normalizeDispatcherStatus(value: unknown): Omit<DispatcherStatusState, 'fetchedAt'> {
  const record = asRecord(value);
  const runnersRaw = record && Array.isArray(record.runners) ? record.runners : [];
  const routingRaw = record ? (record.routing ?? record.routing_map) : null;
  const effectiveRoutingRaw = record ? (record.effective_routing ?? record.effectiveRouting) : null;
  const runners = runnersRaw.map(normalizeRunner).filter(runner => runner.runnerId);
  const routing = normalizeRouting(routingRaw);
  const effectiveRouting = normalizeRouting(effectiveRoutingRaw);

  return {
    queuedJobs: record ? normalizeNumber(record.queued_jobs ?? record.queuedJobs) : 0,
    runners,
    routing,
    effectiveRouting: Object.keys(effectiveRouting).length > 0 ? effectiveRouting : buildEffectiveRunnerRouting(routing, runners),
    dispatcherError: record ? readOptionalString(record.dispatcher_error ?? record.dispatcherError) : undefined,
  };
}

export function getRunnerMeta(runner: Runner): RunnerMeta {
  const meta = runner.metadata || {};
  const runtime = readString(meta.runtime || meta.runner_runtime).toLowerCase();
  const connectionStatus = normalizeRunnerConnectionStatus(runner.connectionStatus || meta.connection_status, runner.reachable);
  return {
    connectionId: readString(meta.connection_id || meta.instance_id),
    hostname: readString(meta.hostname || meta.host || meta.runner_host),
    network: readString(meta.docker_network || meta.docker_network_name || meta.docker_networkname),
    runtime: runtime === 'k8s' ? 'kubernetes' : runtime || 'docker',
    namespace: readString(meta.kubernetes_namespace || meta.namespace),
    node: readString(meta.kubernetes_node || meta.node),
    serviceAccount: readString(meta.kubernetes_service_account || meta.service_account),
    disconnectedAt: readOptionalString(meta.last_disconnected_at),
    reachable: runner.reachable,
    connectionStatus,
    activeRuns: parseActiveRuns(meta),
  };
}

export function runnerActionKey(runnerId: string, connectionId = '') {
  const rid = (runnerId || '').trim();
  const cid = (connectionId || '').trim();
  if (!rid) return '';
  return cid ? `${rid}::${cid}` : rid;
}

export function normalizeRunnerComposeTemplate(value: unknown): RunnerComposeTemplate {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    runnerScopes: readString(record.runner_scopes ?? record.runnerScopes),
    runnerCapacity: normalizeNumber(record.runner_capacity ?? record.runnerCapacity),
    dispatcherAddress: readString(record.dispatcher_grpc_address ?? record.dispatcherAddress),
    networkMode: readString(record.network_mode ?? record.networkMode),
    runnerImage: readString(record.runner_image ?? record.runnerImage),
    registryCredentialRefs: normalizeStringArray(record.registry_credential_refs ?? record.registryCredentialRefs),
    registryHosts: normalizeStringArray(record.registry_hosts ?? record.registryHosts),
    compose: readString(record.compose),
    command: readString(record.command),
    bootstrapCommand: readString(record.bootstrap_command ?? record.bootstrapCommand),
    expiresAt: readString(record.expires_at ?? record.expiresAt),
    warnings: normalizeStringArray(record.warnings),
  };
}

export function normalizeKubernetesRunnerManifestTemplate(value: unknown): KubernetesRunnerManifestTemplate {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    runnerScopes: readString(record.runner_scopes ?? record.runnerScopes),
    runnerCapacity: normalizeNumber(record.runner_capacity ?? record.runnerCapacity),
    namespace: readString(record.namespace),
    serviceAccount: readString(record.service_account ?? record.serviceAccount),
    dispatcherAddress: readString(record.dispatcher_grpc_address ?? record.dispatcherAddress),
    runnerImage: readString(record.runner_image ?? record.runnerImage),
    registryCredentialRefs: normalizeStringArray(record.registry_credential_refs ?? record.registryCredentialRefs),
    registryHosts: normalizeStringArray(record.registry_hosts ?? record.registryHosts),
    manifest: readString(record.manifest),
    command: readString(record.command),
    bootstrapCommand: readString(record.bootstrap_command ?? record.bootstrapCommand),
    expiresAt: readString(record.expires_at ?? record.expiresAt),
    warnings: normalizeStringArray(record.warnings),
  };
}

export function splitRuntimeScopes(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

export function normalizeRuntimeScopeOptions(value: unknown): string[] {
  const items = normalizeListPayload(value, ['scopes']);
  if (!items) return [];
  const scopes = new Set<string>();
  items.forEach(item => {
    const record = asRecord(item);
    const raw = record ? record.scope ?? record.name ?? record.value : item;
    const scope = readString(raw).trim().replace(/^\/+|\/+$/g, '');
    if (scope) scopes.add(scope);
  });
  return sortRuntimeScopeOptions(Array.from(scopes));
}

export function sortRuntimeScopeOptions(scopes: string[]): string[] {
  return scopes.map(scope => scope.trim()).filter(Boolean).sort((a, b) => {
    if (a === 'default' && b !== 'default') return -1;
    if (b === 'default' && a !== 'default') return 1;
    return a.localeCompare(b);
  });
}

export function normalizeDispatcherRoutingScope(value: string) {
  const trimmed = value.trim();
  return trimmed || '*';
}

export function dispatcherRoutingRowsToConfig(rows: DispatcherRoutingDraftRow[]): Record<string, string[]> {
  const routing: Record<string, string[]> = {};
  rows.forEach(row => {
    const runners = row.runners
      .split(/[\n,]/)
      .map(item => item.trim())
      .filter(Boolean);
    routing[normalizeDispatcherRoutingScope(row.scope)] = runners;
  });
  return routing;
}

export function dispatcherRoutingConfigSignature(routing: Record<string, string[]>): string {
  return JSON.stringify(
    Object.entries(routing || {})
      .map(([scope, runners]) => [
        normalizeDispatcherRoutingScope(scope),
        Array.isArray(runners) ? runners.map(item => String(item || '').trim()).filter(Boolean) : [],
      ])
      .sort(([left], [right]) => String(left).localeCompare(String(right)))
  );
}

export function buildLiveRunnerRoutingRows(
  runners: Runner[],
  { includeUnreachable = false }: { includeUnreachable?: boolean } = {}
): RunnerRoutingRow[] {
  const scopeMap = new Map<string, Set<string>>();
  runners.forEach(runner => {
    if (!runner.runnerId || (!includeUnreachable && !runner.reachable)) return;
    const scopes = runner.scopes.length ? runner.scopes : ['*'];
    scopes.forEach(scopeValue => {
      const scope = normalizeRunnerRouteScope(scopeValue);
      const existing = scopeMap.get(scope) || new Set<string>();
      existing.add(runner.runnerId);
      scopeMap.set(scope, existing);
    });
  });
  return Array.from(scopeMap.entries())
    .map(([scope, runnerSet]) => ({
      scope,
      runners: Array.from(runnerSet).sort((a, b) => a.localeCompare(b)),
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));
}

export function buildEffectiveRunnerRouting(configured: Record<string, string[]>, runners: Runner[]): Record<string, string[]> {
  const routing = cloneRouting(configured);
  buildLiveRunnerRoutingRows(runners, { includeUnreachable: true }).forEach(row => {
    const existing = routing[row.scope] || [];
    row.runners.forEach(runnerId => {
      if (!existing.includes(runnerId)) existing.push(runnerId);
    });
    routing[row.scope] = existing;
  });
  return routing;
}

export function buildRunnerAssignmentsForScope(
  status: Pick<DispatcherStatusState, 'runners' | 'routing' | 'effectiveRouting'> | null | undefined,
  targetScope: string,
  includeDescendantScopes = false
): RunnerRouteAssignment[] {
  if (!status) return [];
  const routing = Object.keys(status.effectiveRouting || {}).length > 0
    ? status.effectiveRouting
    : buildEffectiveRunnerRouting(status.routing, status.runners);
  const runnerById = new Map(status.runners.map(runner => [runner.runnerId, runner]));
  const matchedByRunner = new Map<string, Set<string>>();

  Object.entries(routing).forEach(([scope, runnerIds]) => {
    if (!runnerRouteScopeMatchesTarget(scope, targetScope, includeDescendantScopes)) return;
    (runnerIds || []).forEach(rawRunnerId => {
      const runnerId = String(rawRunnerId || '').trim();
      if (!runnerId || !runnerById.has(runnerId)) return;
      const scopes = matchedByRunner.get(runnerId) || new Set<string>();
      scopes.add(normalizeRunnerRouteScope(scope));
      matchedByRunner.set(runnerId, scopes);
    });
  });

  return Array.from(matchedByRunner.entries())
    .map(([runnerId, scopes]) => ({
      runner: runnerById.get(runnerId)!,
      scopes: Array.from(scopes).sort((a, b) => a.localeCompare(b)),
    }))
    .sort((left, right) => left.runner.runnerId.localeCompare(right.runner.runnerId));
}

export function runnerRouteScopeMatchesTarget(routeScope: string, targetScope: string, includeDescendantScopes = false): boolean {
  const route = normalizeRunnerRouteScope(routeScope);
  const target = normalizeRunnerRouteScope(targetScope);
  if (target === '*') return includeDescendantScopes || route === '*' || route === 'default';
  if (route === '*') return true;
  if (route === target) return true;
  return includeDescendantScopes && route.startsWith(`${target}/`);
}

export function formatDispatcherRouteScope(scope: string) {
  const normalized = normalizeRunnerRouteScope(scope);
  return normalized === '*' || normalized === 'default' ? 'Default' : normalized;
}

function normalizeRunnerRouteScope(value: string) {
  const trimmed = String(value || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
  if (!trimmed) return '*';
  const lower = trimmed.toLowerCase();
  if (lower === 'root') return '*';
  if (lower === 'default') return 'default';
  return trimmed;
}

function cloneRouting(routing: Record<string, string[]>): Record<string, string[]> {
  const clone: Record<string, string[]> = {};
  Object.entries(routing || {}).forEach(([scope, runners]) => {
    const key = normalizeRunnerRouteScope(scope);
    clone[key] = Array.isArray(runners) ? runners.map(runner => String(runner || '').trim()).filter(Boolean) : [];
  });
  return clone;
}

function normalizeRunner(value: unknown): Runner {
  const record = asRecord(value) || {};
  const metadata = normalizeStringMap(record.metadata);
  const connectionStatus = normalizeRunnerConnectionStatus(record.connection_status ?? record.connectionStatus ?? metadata.connection_status);
  const reachable = normalizeRunnerReachability(record.reachable ?? metadata.reachable, connectionStatus);
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    scopes: normalizeStringArray(record.scopes),
    capacity: normalizeNumber(record.capacity),
    activeJobs: normalizeNumber(record.active_jobs ?? record.activeJobs),
    inflightJobs: normalizeNumber(record.inflight_jobs ?? record.inflightJobs),
    lastHeartbeatUnix: normalizeNumber(record.last_heartbeat_unix ?? record.lastHeartbeatUnix),
    metadata,
    allowDispatch: Boolean(record.allow_dispatch ?? record.allowDispatch),
    reachable,
    connectionStatus: normalizeRunnerConnectionStatus(connectionStatus, reachable),
  };
}

function normalizeRunnerConnectionStatus(value: unknown, reachable?: boolean): string {
  const normalized = readString(value).trim().toLowerCase();
  if (reachable === false) return 'unreachable';
  if (['unreachable', 'disconnected', 'offline'].includes(normalized)) return 'unreachable';
  if (['online', 'connected', 'reachable'].includes(normalized)) return 'online';
  return normalized || 'online';
}

function normalizeRunnerReachability(value: unknown, connectionStatus = ''): boolean {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  const normalized = readString(value).trim().toLowerCase();
  if (['false', '0', 'no', 'unreachable', 'offline', 'disconnected'].includes(normalized)) return false;
  if (['true', '1', 'yes', 'reachable', 'online', 'connected'].includes(normalized)) return true;
  return normalizeRunnerConnectionStatus(connectionStatus) !== 'unreachable';
}

function parseActiveRuns(meta: Record<string, string>): RunnerActiveRun[] {
  const raw = (meta && meta.active_runs) || '';
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map(item => {
        const record = asRecord(item);
        if (!record) return null;
        const runId = readString(record.run_id);
        if (!runId) return null;
        return {
          runId,
          pipeline: readString(record.pipeline),
          parentStep: readOptionalString(record.parent_step),
          triggerId: readOptionalString(record.trigger_event_id),
        } satisfies RunnerActiveRun;
      })
      .filter(Boolean) as RunnerActiveRun[];
  } catch (error) {
    console.warn('Failed to parse active_runs metadata', error);
    return [];
  }
}

function normalizeRouting(value: unknown): Record<string, string[]> {
  const record = asRecord(value);
  if (!record) return {};
  const normalized: Record<string, string[]> = {};
  Object.entries(record).forEach(([scope, runners]) => {
    if (!scope) return;
    if (Array.isArray(runners)) {
      normalized[scope] = runners.map(item => String(item || '').trim()).filter(Boolean);
    } else if (typeof runners === 'string') {
      normalized[scope] = [runners.trim()].filter(Boolean);
    }
  });
  return normalized;
}
