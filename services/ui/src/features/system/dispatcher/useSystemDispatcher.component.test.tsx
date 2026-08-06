import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { useSystemDispatcher } from './useSystemDispatcher';
import type { Runner } from './model';

const apiMocks = vi.hoisted(() => ({
  fetchDispatcherStatus: vi.fn(async () => ({
    queuedJobs: 0,
    runners: [],
    routing: {},
    effectiveRouting: {},
    fetchedAt: Date.now(),
  })),
  fetchSystemJson: vi.fn(async () => null),
}));

vi.mock('./api', () => ({
  fetchDispatcherStatus: apiMocks.fetchDispatcherStatus,
}));

vi.mock('../api', () => ({
  fetchSystemJson: apiMocks.fetchSystemJson,
}));

afterEach(() => {
  apiMocks.fetchDispatcherStatus.mockClear();
  apiMocks.fetchSystemJson.mockClear();
  vi.restoreAllMocks();
});

test('ejects a runner through the dispatcher delete API after confirmation', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
  const addToast = vi.fn();
  const runner: Runner = {
    runnerId: 'runner-prod-5',
    scopes: ['prod'],
    capacity: 2,
    activeJobs: 0,
    inflightJobs: 0,
    lastHeartbeatUnix: Date.now() / 1000,
    allowDispatch: true,
    reachable: true,
    connectionStatus: 'connected',
    metadata: { connection_id: 'conn-runner-prod-5' },
  };
  const { result } = renderHook(() =>
    useSystemDispatcher({
      enabled: false,
      locationSearch: '',
      addToast,
    })
  );

  await act(async () => {
    await result.current.onEjectRunner(runner);
  });

  expect(confirm).toHaveBeenCalledWith(expect.stringContaining('runner-prod-5'));
  expect(apiMocks.fetchSystemJson).toHaveBeenCalledWith('/v1/system/dispatcher/runners/runner-prod-5', {
    method: 'DELETE',
  });
  expect(apiMocks.fetchDispatcherStatus).toHaveBeenCalledTimes(1);
  expect(addToast).toHaveBeenCalledWith('Runner registration removed and ID revoked.', 'success');
});
