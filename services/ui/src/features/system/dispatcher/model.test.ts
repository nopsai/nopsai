import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildEffectiveRunnerRouting,
  buildRunnerAssignmentsForScope,
  dispatcherRoutingConfigSignature,
  dispatcherRoutingRowsToConfig,
  formatDispatcherRouteScope,
  getRunnerMeta,
  nopsaiImageTag,
  normalizeKubernetesRunnerManifestTemplate,
  normalizeDispatcherStatus,
  normalizeRunnerComposeTemplate,
  normalizeRuntimeScopeOptions,
  runnerImageForVersion,
  sortRuntimeScopeOptions,
  splitRuntimeScopes,
} from './model.js';

test('normalizes dispatcher install templates', () => {
  assert.deepEqual(
    normalizeRunnerComposeTemplate({
      runner_id: 'runner-1',
      runner_capacity: 3,
      dispatcher_grpc_address: 'dispatcher:9090',
      bootstrap_command: 'docker compose up',
      warnings: ['rotate token'],
    }),
    {
      runnerId: 'runner-1',
      runnerScopes: '',
      runnerCapacity: 3,
      dispatcherAddress: 'dispatcher:9090',
      networkMode: '',
      runnerImage: '',
      registryCredentialRefs: [],
      registryHosts: [],
      compose: '',
      command: '',
      bootstrapCommand: 'docker compose up',
      expiresAt: '',
      warnings: ['rotate token'],
    }
  );

  const kubernetes = normalizeKubernetesRunnerManifestTemplate({
    runnerId: 'k8s-1',
    runnerCapacity: '2',
    service_account: 'runner-sa',
    bootstrapCommand: 'kubectl apply -f -',
  });
  assert.equal(kubernetes.runnerCapacity, 2);
  assert.equal(kubernetes.serviceAccount, 'runner-sa');
  assert.equal(kubernetes.bootstrapCommand, 'kubectl apply -f -');
});

test('builds runner image defaults from NopsAI version tags', () => {
  assert.equal(
    runnerImageForVersion('ghcr.io/nopsai/nopsai-docker-runner', '1.2.3'),
    'ghcr.io/nopsai/nopsai-docker-runner:1.2.3'
  );
  assert.equal(nopsaiImageTag('latest'), 'dev');
  assert.equal(nopsaiImageTag(' unknown '), 'dev');
});

test('normalizes registered runner reachability metadata', () => {
  const status = normalizeDispatcherStatus({
    runners: [
      {
        runner_id: 'runner-offline',
        capacity: 2,
        allow_dispatch: false,
        metadata: {
          connection_status: 'unreachable',
          reachable: 'false',
          last_disconnected_at: '2026-07-14T10:00:00Z',
        },
      },
    ],
  });

  assert.equal(status.runners[0].reachable, false);
  assert.equal(status.runners[0].connectionStatus, 'unreachable');
  assert.deepEqual(getRunnerMeta(status.runners[0]), {
    connectionId: '',
    hostname: '',
    network: '',
    runtime: 'docker',
    namespace: '',
    node: '',
    serviceAccount: '',
    disconnectedAt: '2026-07-14T10:00:00Z',
    reachable: false,
    connectionStatus: 'unreachable',
    activeRuns: [],
  });
});

test('builds effective routing and runner assignments from registered scopes', () => {
  const status = normalizeDispatcherStatus({
    routing: { prod: ['runner-static'] },
    runners: [
      { runner_id: 'runner-prod', scopes: ['prod'], capacity: 2, allow_dispatch: true },
      { runner_id: 'runner-team', scopes: ['team-1/subgroup'], capacity: 1, allow_dispatch: true },
      { runner_id: 'runner-default', scopes: [], capacity: 1, allow_dispatch: true },
      { runner_id: 'runner-literal-default', scopes: ['default'], capacity: 1, allow_dispatch: true },
    ],
  });

  assert.deepEqual(buildEffectiveRunnerRouting(status.routing, status.runners), {
    prod: ['runner-static', 'runner-prod'],
    'team-1/subgroup': ['runner-team'],
    '*': ['runner-default'],
    default: ['runner-literal-default'],
  });
  assert.deepEqual(
    buildRunnerAssignmentsForScope(status, 'prod').map(item => `${item.runner.runnerId}:${item.scopes.join(',')}`),
    ['runner-default:*', 'runner-prod:prod']
  );
  assert.deepEqual(
    buildRunnerAssignmentsForScope(status, 'team-1', true).map(item => `${item.runner.runnerId}:${item.scopes.join(',')}`),
    ['runner-default:*', 'runner-team:team-1/subgroup']
  );
  assert.deepEqual(
    buildRunnerAssignmentsForScope(status, '').map(item => `${item.runner.runnerId}:${item.scopes.join(',')}`),
    ['runner-default:*', 'runner-literal-default:default']
  );
  assert.equal(formatDispatcherRouteScope('*'), 'Default');
});

test('normalizes and sorts dispatcher runtime scopes', () => {
  assert.deepEqual(normalizeRuntimeScopeOptions({ scopes: [{ scope: '/prod/' }, { name: 'default' }, 'prod'] }), ['default', 'prod']);
  assert.deepEqual(splitRuntimeScopes(' prod, staging,,'), ['prod', 'staging']);
  assert.deepEqual(sortRuntimeScopeOptions(['zeta', 'default', ' alpha ']), ['default', 'alpha', 'zeta']);
});

test('converts dispatcher routing editor rows into runtime config', () => {
  assert.deepEqual(
    dispatcherRoutingRowsToConfig([
      { scope: ' prod ', runners: ' runner-prod-1, runner-prod-2\nrunner-prod-3 ' },
      { scope: '', runners: 'runner-default' },
    ]),
    {
      prod: ['runner-prod-1', 'runner-prod-2', 'runner-prod-3'],
      '*': ['runner-default'],
    }
  );
});

test('signs dispatcher routing config independent of row order and whitespace', () => {
  assert.equal(
    dispatcherRoutingConfigSignature({ prod: ['runner-prod'], '*': ['runner-default'] }),
    dispatcherRoutingConfigSignature({ ' * ': [' runner-default '], prod: ['runner-prod'] })
  );
});
