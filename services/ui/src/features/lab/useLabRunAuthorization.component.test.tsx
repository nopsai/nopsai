import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { apiClient } from '../../lib/api';
import {
  collectLabResourceUseChecks,
  useLabRunAuthorization,
} from './useLabRunAuthorization';

vi.mock('../../lib/api', () => ({
  apiClient: { fetch: vi.fn() },
}));

const fetchMock = vi.mocked(apiClient.fetch);

beforeEach(() => {
  vi.useFakeTimers();
  fetchMock.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

test('collects and deduplicates pipeline, scope, and include checks', () => {
  expect(
    collectLabResourceUseChecks(
      '',
      'name: release\nsteps:\n  - include: step:shared/build\n  - include: pipeline:release\n  - include: step:shared/build\n',
      '/production/'
    )
  ).toEqual([
    { action: 'pipeline.use', resource_type: 'pipeline', resource_id: 'release' },
    { action: 'scope.use', resource_type: 'scope', resource_id: 'production' },
    { action: 'step.use', resource_type: 'step', resource_id: 'shared/build' },
  ]);
  expect(collectLabResourceUseChecks('selected', 'not: [valid', 'default')).toEqual([
    { action: 'pipeline.use', resource_type: 'pipeline', resource_id: 'selected' },
  ]);
});

test('debounces authorization and reports denied checks', async () => {
  fetchMock.mockResolvedValue(
    new Response(
      JSON.stringify({
        results: [
          {
            allowed: false,
            action: 'pipeline.use',
            resource_type: 'pipeline',
            resource_id: 'release',
          },
        ],
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } }
    )
  );
  const { result } = renderHook(() =>
    useLabRunAuthorization('release', 'name: release\nsteps: []\n', '', 0)
  );

  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });

  expect(fetchMock).toHaveBeenCalledWith('/v1/authz/resource-use/batch-check', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      checks: [{ action: 'pipeline.use', resource_type: 'pipeline', resource_id: 'release' }],
    }),
  });
  expect(result.current.loading).toBe(false);
  expect(result.current.blocked).toBe(true);
  expect(result.current.deniedChecks).toHaveLength(1);
});

test('skips invalid definitions and fails closed with a visible validation error', async () => {
  const invalid = renderHook(() =>
    useLabRunAuthorization('release', 'name: release\n', '', 1)
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  expect(fetchMock).not.toHaveBeenCalled();
  expect(invalid.result.current.checks).toEqual([]);

  fetchMock.mockResolvedValue(
    new Response(JSON.stringify({ message: 'denied' }), {
      status: 503,
      headers: { 'Content-Type': 'application/json' },
    })
  );
  const failed = renderHook(() =>
    useLabRunAuthorization('release', 'name: release\nsteps: []\n', '', 0)
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  expect(failed.result.current.error).toBe('Unable to validate access (503)');
  expect(failed.result.current.blocked).toBe(false);
});
