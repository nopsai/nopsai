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
  activeRuns: RunnerActiveRun[];
};

export type DispatcherStatusState = {
  queuedJobs: number;
  runners: Runner[];
  routing: Record<string, string[]>;
  dispatcherError?: string;
  fetchedAt: number;
};

export type RunnerComposeTemplate = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  dispatcherAddress: string;
  networkMode: string;
  runnerImage: string;
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
  manifest: string;
  command: string;
  bootstrapCommand: string;
  expiresAt: string;
  warnings: string[];
};

export type RunnerInstallRuntime = 'docker' | 'kubernetes';

export function normalizeDispatcherStatus(value: unknown): Omit<DispatcherStatusState, 'fetchedAt'> {
  const record = asRecord(value);
  const runnersRaw = record && Array.isArray(record.runners) ? record.runners : [];
  const routingRaw = record ? (record.routing ?? record.routing_map) : null;

  return {
    queuedJobs: record ? normalizeNumber(record.queued_jobs ?? record.queuedJobs) : 0,
    runners: runnersRaw.map(normalizeRunner).filter(runner => runner.runnerId),
    routing: normalizeRouting(routingRaw),
    dispatcherError: record ? readOptionalString(record.dispatcher_error ?? record.dispatcherError) : undefined,
  };
}

export function getRunnerMeta(runner: Runner): RunnerMeta {
  const meta = runner.metadata || {};
  const runtime = readString(meta.runtime || meta.runner_runtime).toLowerCase();
  return {
    connectionId: readString(meta.connection_id || meta.instance_id),
    hostname: readString(meta.hostname || meta.host || meta.runner_host),
    network: readString(meta.docker_network || meta.docker_network_name || meta.docker_networkname),
    runtime: runtime === 'k8s' ? 'kubernetes' : runtime || 'docker',
    namespace: readString(meta.kubernetes_namespace || meta.namespace),
    node: readString(meta.kubernetes_node || meta.node),
    serviceAccount: readString(meta.kubernetes_service_account || meta.service_account),
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
    dispatcherAddress: readString(record.dispatcher_address ?? record.dispatcherAddress),
    networkMode: readString(record.network_mode ?? record.networkMode),
    runnerImage: readString(record.runner_image ?? record.runnerImage),
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
    dispatcherAddress: readString(record.dispatcher_address ?? record.dispatcherAddress),
    runnerImage: readString(record.runner_image ?? record.runnerImage),
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

function normalizeRunner(value: unknown): Runner {
  const record = asRecord(value) || {};
  return {
    runnerId: readString(record.runner_id ?? record.runnerId),
    scopes: normalizeStringArray(record.scopes),
    capacity: normalizeNumber(record.capacity),
    activeJobs: normalizeNumber(record.active_jobs ?? record.activeJobs),
    inflightJobs: normalizeNumber(record.inflight_jobs ?? record.inflightJobs),
    lastHeartbeatUnix: normalizeNumber(record.last_heartbeat_unix ?? record.lastHeartbeatUnix),
    metadata: normalizeStringMap(record.metadata),
    allowDispatch: Boolean(record.allow_dispatch ?? record.allowDispatch),
  };
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
