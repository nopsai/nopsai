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
        runnerDefaults={{
          runner_id: 'runner-test',
          runner_scopes: 'prod',
          runner_capacity: '2',
        } as ConfigFormState}
      />
    </MemoryRouter>
  );

  expect(await screen.findByText('staging')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Generate command' }));

  await waitFor(() => expect(apiMocks.fetchDockerRunnerTemplate).toHaveBeenCalled());
  expect(await screen.findByText('docker run nopsai-runner')).toBeVisible();
});
