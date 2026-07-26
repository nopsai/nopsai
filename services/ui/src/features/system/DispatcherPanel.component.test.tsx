import { MemoryRouter } from 'react-router-dom';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import DispatcherPanel from './DispatcherPanel';
import type { ConfigFormState } from './config/model';

const apiMocks = vi.hoisted(() => ({
  fetchPlatformVersionTag: vi.fn(async () => '2.10.648'),
  fetchDispatcherScopeOptions: vi.fn(async () => ['default', 'prod', 'staging']),
  fetchDockerRunnerTemplate: vi.fn(async () => ({
    runnerId: 'runner-test',
    runnerScopes: 'prod',
    runnerCapacity: 2,
    dispatcherAddress: 'dispatcher:9090',
    networkMode: 'host',
    runnerImage: 'ghcr.io/nopsai/nopsai-docker-runner:2.10.648',
    registryCredentialRefs: ['credential://system/registry/production-ghcr'],
    registryHosts: ['ghcr.io'],
    compose: '',
    command: '',
    bootstrapCommand: 'docker run nopsai-docker-runner',
    expiresAt: '2026-06-08T12:00:00Z',
    warnings: [],
  })),
  fetchKubernetesRunnerTemplate: vi.fn(),
  fetchCredentials: vi.fn(async () => [
    {
      id: 'credential-1',
      reference: 'credential://system/registry/production-ghcr',
      kind: 'docker_config_json',
      description: 'Production GHCR pull config',
      metadata: { registry_hosts: ['ghcr.io'] },
      status: 'active',
      has_value: true,
      active_version: 1,
      managed_by_config_repo: false,
      created_at: '2026-06-08T12:00:00Z',
      updated_at: '2026-06-08T12:00:00Z',
      versions: [],
    },
  ]),
}));

vi.mock('./dispatcher/api', () => ({
  ...apiMocks,
}));

vi.mock('./credentials/api', () => ({
  fetchCredentials: apiMocks.fetchCredentials,
}));

test('loads runner scopes and generates an install command through the dispatcher feature API', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <DispatcherPanel
        loading={false}
        error={null}
        status={null}
        pendingActions={new Set()}
        pendingEjections={new Set()}
        onRefresh={() => undefined}
        onToggleRunnerDispatch={async () => undefined}
        onEjectRunner={async () => undefined}
        canManageDispatcher
        canViewRuntimeConfig
        canManageRuntimeConfig
        runnerDefaults={{
          runner_id: 'runner-test',
          runner_scopes: 'prod',
          runner_capacity: '2',
          dispatcher_grpc_address: 'nopsai-dispatcher.nopsai.orb.local:9090',
        } as ConfigFormState}
        config={{ dispatcher_routing: {} } as ConfigFormState}
        fieldMetadata={{}}
        configLoading={false}
        saving={false}
        onConfigChange={() => undefined}
        onSaveConfig={async () => undefined}
      />
    </MemoryRouter>
  );

  expect(await screen.findByText('staging')).toBeVisible();
  expect(await screen.findByText('production-ghcr')).toBeVisible();
  await user.click(screen.getByRole('checkbox', { name: /production-ghcr/i }));
  await user.type(screen.getByLabelText('Dispatcher address override'), 'nopsai-dispatcher.nopsai.orb.local:9090');
  await user.click(screen.getByRole('button', { name: 'Generate command' }));

  await waitFor(() => expect(apiMocks.fetchDockerRunnerTemplate).toHaveBeenCalled());
  expect(apiMocks.fetchDockerRunnerTemplate).toHaveBeenCalledWith(expect.objectContaining({
    dispatcherAddress: 'nopsai-dispatcher.nopsai.orb.local:9090',
    registryCredentialRefs: ['credential://system/registry/production-ghcr'],
  }));
  expect(await screen.findByText('1 selected')).toBeVisible();
  expect((await screen.findAllByText('ghcr.io')).length).toBeGreaterThan(0);
  expect(await screen.findByText('docker run nopsai-docker-runner')).toBeVisible();
});

test('edits dispatcher routing from the dispatcher panel and saves runtime config', async () => {
  const user = userEvent.setup();
  const onConfigChange = vi.fn();
  const onSaveConfig = vi.fn(async () => undefined);

  render(
    <MemoryRouter>
      <DispatcherPanel
        loading={false}
        error={null}
        status={{ queuedJobs: 0, runners: [], routing: {}, effectiveRouting: {}, fetchedAt: Date.now() }}
        pendingActions={new Set()}
        pendingEjections={new Set()}
        onRefresh={() => undefined}
        onToggleRunnerDispatch={async () => undefined}
        onEjectRunner={async () => undefined}
        canManageDispatcher
        canViewRuntimeConfig
        canManageRuntimeConfig
        runnerDefaults={{ runner_id: 'runner-test', runner_scopes: 'prod', runner_capacity: '2' } as ConfigFormState}
        config={{ dispatcher_routing: { prod: ['runner-prod'] } } as ConfigFormState}
        fieldMetadata={{ dispatcher_routing: { scope: 'runtime', label: 'Dispatcher routing', section: 'Dispatcher', apply: 'live' } }}
        configLoading={false}
        saving={false}
        onConfigChange={onConfigChange}
        onSaveConfig={onSaveConfig}
      />
    </MemoryRouter>
  );

  expect(screen.getByDisplayValue('prod')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Add route' }));
  expect(onConfigChange).toHaveBeenLastCalledWith(expect.objectContaining({
    dispatcher_routing: {
      prod: ['runner-prod'],
      '*': [],
    },
  }));

  await user.click(screen.getByRole('button', { name: 'Save routes' }));
  expect(onSaveConfig).toHaveBeenCalled();
});

test('shows previously registered unreachable runners with a warning', () => {
  render(
    <MemoryRouter>
      <DispatcherPanel
        loading={false}
        error={null}
        status={{
          queuedJobs: 0,
          runners: [
            {
              runnerId: 'runner-offline',
              scopes: ['prod'],
              capacity: 2,
              activeJobs: 0,
              inflightJobs: 0,
              lastHeartbeatUnix: 1_783_000_000,
              allowDispatch: true,
              reachable: false,
              connectionStatus: 'unreachable',
              metadata: {
                runtime: 'docker',
                connection_status: 'unreachable',
                reachable: 'false',
                last_disconnected_at: '2026-07-14T10:00:00Z',
              },
            },
          ],
          routing: { prod: ['runner-offline'] },
          effectiveRouting: { prod: ['runner-offline'] },
          fetchedAt: Date.parse('2026-07-14T10:01:00Z'),
        }}
        pendingActions={new Set()}
        pendingEjections={new Set()}
        onRefresh={() => undefined}
        onToggleRunnerDispatch={async () => undefined}
        onEjectRunner={async () => undefined}
        canManageDispatcher
        canViewRuntimeConfig
        canManageRuntimeConfig
        runnerDefaults={{ runner_id: 'runner-test', runner_scopes: 'prod', runner_capacity: '2' } as ConfigFormState}
        config={{ dispatcher_routing: { prod: ['runner-offline'] } } as ConfigFormState}
        fieldMetadata={{}}
        configLoading={false}
        saving={false}
        onConfigChange={() => undefined}
        onSaveConfig={async () => undefined}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('1 unreachable')).toBeVisible();
  expect(screen.getByText('Unreachable')).toBeVisible();
  expect(screen.getByText('No live runner scopes.')).toBeVisible();
});

test('offers a permanent runner eject action from runner cards', async () => {
  const user = userEvent.setup();
  const onEjectRunner = vi.fn(async () => undefined);
  render(
    <MemoryRouter>
      <DispatcherPanel
        loading={false}
        error={null}
        status={{
          queuedJobs: 0,
          runners: [
            {
              runnerId: 'runner-prod-5',
              scopes: ['prod'],
              capacity: 2,
              activeJobs: 0,
              inflightJobs: 0,
              lastHeartbeatUnix: Date.now() / 1000,
              allowDispatch: true,
              reachable: true,
              connectionStatus: 'connected',
              metadata: {
                runtime: 'docker',
                connection_id: 'conn-runner-prod-5',
                connection_status: 'connected',
                reachable: 'true',
              },
            },
          ],
          routing: { prod: ['runner-prod-5'] },
          effectiveRouting: { prod: ['runner-prod-5'] },
          fetchedAt: Date.now(),
        }}
        pendingActions={new Set()}
        pendingEjections={new Set()}
        onRefresh={() => undefined}
        onToggleRunnerDispatch={async () => undefined}
        onEjectRunner={onEjectRunner}
        canManageDispatcher
        canViewRuntimeConfig
        canManageRuntimeConfig
        runnerDefaults={{ runner_id: 'runner-test', runner_scopes: 'prod', runner_capacity: '2' } as ConfigFormState}
        config={{ dispatcher_routing: { prod: ['runner-prod-5'] } } as ConfigFormState}
        fieldMetadata={{}}
        configLoading={false}
        saving={false}
        onConfigChange={() => undefined}
        onSaveConfig={async () => undefined}
      />
    </MemoryRouter>
  );

  await user.click(screen.getByRole('button', { name: 'Eject' }));
  expect(onEjectRunner).toHaveBeenCalledWith(expect.objectContaining({ runnerId: 'runner-prod-5' }));
});
