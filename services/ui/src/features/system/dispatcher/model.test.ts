import assert from 'node:assert/strict';
import test from 'node:test';
import {
  normalizeKubernetesRunnerManifestTemplate,
  normalizeRunnerComposeTemplate,
  normalizeRuntimeScopeOptions,
  sortRuntimeScopeOptions,
  splitRuntimeScopes,
} from './model.js';

test('normalizes dispatcher install templates', () => {
  assert.deepEqual(
    normalizeRunnerComposeTemplate({
      runner_id: 'runner-1',
      runner_capacity: 3,
      dispatcher_address: 'dispatcher:9090',
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

test('normalizes and sorts dispatcher runtime scopes', () => {
  assert.deepEqual(normalizeRuntimeScopeOptions({ scopes: [{ scope: '/prod/' }, { name: 'default' }, 'prod'] }), ['default', 'prod']);
  assert.deepEqual(splitRuntimeScopes(' prod, staging,,'), ['prod', 'staging']);
  assert.deepEqual(sortRuntimeScopeOptions(['zeta', 'default', ' alpha ']), ['default', 'alpha', 'zeta']);
});
