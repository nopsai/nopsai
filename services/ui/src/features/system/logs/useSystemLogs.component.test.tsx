import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { useSystemLogs } from './useSystemLogs';

const apiMocks = vi.hoisted(() => ({
  fetchSystemLogSources: vi.fn(),
  streamSystemLogs: vi.fn(),
}));

vi.mock('./api', () => ({
  fetchSystemLogSources: apiMocks.fetchSystemLogSources,
  streamSystemLogs: apiMocks.streamSystemLogs,
}));

afterEach(() => {
  apiMocks.fetchSystemLogSources.mockReset();
  apiMocks.streamSystemLogs.mockReset();
});

test('selects the runner source requested by the system logs route query', async () => {
  apiMocks.fetchSystemLogSources.mockResolvedValue({
    redaction_warning: 'Secret redaction is best effort.',
    sources: [
      { id: 'dispatcher', display_name: 'Dispatcher', container_name: 'dispatcher', available: true },
      { id: 'runner:runner-k8s', display_name: 'Runner runner-k8s', container_name: 'runner', available: true },
    ],
  });
  apiMocks.streamSystemLogs.mockImplementation(async ({ signal }: { signal: AbortSignal }) => {
    await new Promise<void>(resolve => {
      if (signal.aborted) {
        resolve();
        return;
      }
      signal.addEventListener('abort', () => resolve(), { once: true });
    });
  });

  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={['/system/logs?source=runner%3Arunner-k8s']}>{children}</MemoryRouter>
  );

  const { result } = renderHook(() => useSystemLogs(), { wrapper });

  await waitFor(() => expect(result.current.selectedSourceID).toBe('runner:runner-k8s'));
});
