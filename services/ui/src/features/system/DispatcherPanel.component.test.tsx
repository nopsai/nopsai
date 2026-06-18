import { MemoryRouter } from 'react-router-dom';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import DispatcherPanel from './DispatcherPanel';
import type { ConfigFormState } from './config/model';

const apiMocks = vi.hoisted(() => ({
  fetchDispatcherScopeOptions: vi.fn(async () => ['default', 'prod', 'staging']),
  fetchDockerRunnerTemplate: vi.fn(async () => ({
    runnerId: 'runner-test',
    runnerScopes: 'prod',
    runnerCapacity: 2,
    dispatcherAddress: 'dispatcher:9090',
    networkMode: 'host',
    runnerImage: 'hoseindocker/nopsai-runner:latest',
    compose: '',
    command: '',
    bootstrapCommand: 'docker run nopsai-runner',
    expiresAt: '2026-06-08T12:00:00Z',
    warnings: [],
  })),
  fetchKubernetesRunnerTemplate: vi.fn(),
}));

vi.mock('./dispatcher/api', () => ({
  ...apiMocks,
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
        onRefresh={() => undefined}
        onToggleRunnerDispatch={async () => undefined}
        canManageDispatcher
        canViewRuntimeConfig
        canManageRuntimeConfig
        runnerDefaults={{
          runner_id: 'runner-test',
          runner_scopes: 'prod',
          runner_capacity: '2',
          dispatcher_address: 'nopsai-dispatcher.pre-nopsai.orb.local:9090',
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
  await user.type(screen.getByLabelText('Dispatcher address override'), 'nopsai-dispatcher.pre-nopsai.orb.local:9090');
  await user.click(screen.getByRole('button', { name: 'Generate command' }));

  await waitFor(() => expect(apiMocks.fetchDockerRunnerTemplate).toHaveBeenCalled());
  expect(apiMocks.fetchDockerRunnerTemplate).toHaveBeenCalledWith(expect.objectContaining({
    dispatcherAddress: 'nopsai-dispatcher.pre-nopsai.orb.local:9090',
  }));
  expect(await screen.findByText('docker run nopsai-runner')).toBeVisible();
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
        status={{ queuedJobs: 0, runners: [], routing: {}, fetchedAt: Date.now() }}
        pendingActions={new Set()}
        onRefresh={() => undefined}
        onToggleRunnerDispatch={async () => undefined}
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
