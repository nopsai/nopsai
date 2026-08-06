import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { fetchRunLogs } from './api';
import { useRunLogs } from './useRunLogs';

vi.mock('./api', async importOriginal => {
  const actual = await importOriginal<typeof import('./api')>();
  return {
    ...actual,
    fetchRunLogs: vi.fn(),
  };
});

const fetchRunLogsMock = vi.mocked(fetchRunLogs);

beforeEach(() => {
  fetchRunLogsMock.mockReset();
  fetchRunLogsMock.mockResolvedValue([]);
  window.history.replaceState(null, '', '/#/pipelineruns/events/run-1');
});

afterEach(() => {
  vi.useRealTimers();
});

test('hydrates filters from the legacy route hash and writes deterministic updates back', async () => {
  window.location.hash =
    '#/pipelineruns/events/run-1/logs/build/error/wrap/unstructured/agent/full?search=failed';

  const { result } = renderHook(() => useRunLogs({ runID: 'run-1' }));

  await waitFor(() => {
    expect(Array.from(result.current.selectedSteps)).toEqual(['build']);
    expect(Array.from(result.current.selectedLevels)).toEqual(['error']);
    expect(result.current.searchText).toBe('failed');
  });
  expect(result.current.wrap).toBe(true);
  expect(result.current.structured).toBe(false);
  expect(result.current.agentOnly).toBe(true);
  expect(result.current.shortView).toBe(false);

  act(() => {
    result.current.toggleStep('deploy');
    result.current.setAgentOnly(false);
    result.current.setSearchText('timeout');
  });

  await waitFor(() => {
    expect(window.location.hash).toBe(
      '#/pipelineruns/events/run-1/logs/build%2Cdeploy/error/wrap/unstructured/all/full?search=timeout'
    );
  });
});

test('cleans stale filters and lines when the selected run changes', async () => {
  fetchRunLogsMock
    .mockResolvedValueOnce([
      { id: 4, timestamp: '2026-06-11T10:00:00Z', line: '{"level":"info","step":"build","message":"first"}' },
    ])
    .mockResolvedValue([]);

  const { result, rerender } = renderHook(
    ({ runID, initialStep }) => useRunLogs({ runID, initialStep }),
    { initialProps: { runID: 'run-1', initialStep: 'build' as string | null } }
  );

  await waitFor(() => expect(result.current.lines.map(line => line.id)).toEqual([4]));
  expect(Array.from(result.current.selectedSteps)).toEqual(['build']);

  rerender({ runID: 'run-2', initialStep: null });

  await waitFor(() => {
    expect(result.current.lines).toEqual([]);
    expect(Array.from(result.current.selectedSteps)).toEqual([]);
    expect(result.current.error).toBeNull();
  });
  expect(fetchRunLogsMock).toHaveBeenLastCalledWith('run-2', 0);
});

test('explicit task log opens override stale hash filters and filter by task metadata', async () => {
  window.location.hash =
    '#/pipelineruns/events/run-1/logs/deploy/error/wrap/unstructured/all/full?search=stale&task=publish';
  fetchRunLogsMock.mockResolvedValueOnce([
    { id: 1, timestamp: '2026-06-11T10:00:00Z', line: '{"level":"info","step":"build","task":"compile","message":"compile"}' },
    { id: 2, timestamp: '2026-06-11T10:00:01Z', line: '{"level":"info","step":"build","task":"test","message":"test"}' },
    { id: 3, timestamp: '2026-06-11T10:00:02Z', line: '{"level":"info","step":"deploy","task":"compile","message":"deploy"}' },
  ]);

  const { result } = renderHook(() =>
    useRunLogs({
      runID: 'run-1',
      initialStep: 'build',
      initialTask: 'compile',
      initialSearch: null,
    })
  );

  await waitFor(() => {
    expect(Array.from(result.current.selectedSteps)).toEqual(['build']);
    expect(Array.from(result.current.selectedTasks)).toEqual(['compile']);
    expect(result.current.searchText).toBe('');
    expect(result.current.visibleLines.map(line => line.id)).toEqual([1]);
  });
  expect(window.location.hash).toBe(
    '#/pipelineruns/events/run-1/logs/build/all/unwrap/unstructured/all/short?task=compile'
  );
});

test('requests included pipeline logs when child log visibility is enabled', async () => {
  fetchRunLogsMock.mockResolvedValueOnce([
    {
      id: 5,
      timestamp: '2026-06-11T10:00:03Z',
      run_id: 'child-run',
      pipeline_name: 'child',
      parent_step_name: 'included',
      line: '{"level":"info","step":"child-build","message":"included output"}',
    },
  ]);

  const { result } = renderHook(() => useRunLogs({ runID: 'parent-run', includeChildren: true, initialStep: 'included' }));

  await waitFor(() => {
    expect(fetchRunLogsMock).toHaveBeenCalledWith('parent-run', 0, { includeChildren: true });
    expect(result.current.visibleLines.map(line => line.id)).toEqual([5]);
  });
});
