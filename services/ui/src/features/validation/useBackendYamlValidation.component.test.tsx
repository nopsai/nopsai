import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { validateBackendYamlResource, type BackendValidationResponse } from './api';
import { useBackendYamlValidation } from './useBackendYamlValidation';

vi.mock('./api', () => ({
  validateBackendYamlResource: vi.fn(),
}));

const validateMock = vi.mocked(validateBackendYamlResource);

const validResponse: BackendValidationResponse = {
  valid: true,
  errors: [],
  warnings: [],
};

beforeEach(() => {
  vi.useFakeTimers();
  validateMock.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

test('debounces backend validation requests', async () => {
  validateMock.mockResolvedValue(validResponse);

  const { result } = renderHook(() =>
    useBackendYamlValidation({
      enabled: true,
      resource: 'pipeline',
      yaml: 'name: release\nsteps: []\n',
      resourceID: 'release',
      debounceMs: 100,
    })
  );

  expect(result.current.status).toBe('checking');

  await act(async () => {
    await vi.advanceTimersByTimeAsync(99);
  });
  expect(validateMock).not.toHaveBeenCalled();

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });

  expect(validateMock).toHaveBeenCalledWith(
    'pipeline',
    expect.objectContaining({
      yaml: 'name: release\nsteps: []\n',
      resource_id: 'release',
    }),
    expect.any(Object)
  );
  expect(result.current.status).toBe('valid');
});

test('ignores stale backend responses after YAML changes', async () => {
  const resolvers: Array<(value: BackendValidationResponse) => void> = [];
  validateMock.mockImplementation(
    () => new Promise<BackendValidationResponse>(resolve => resolvers.push(resolve))
  );

  const { result, rerender } = renderHook(
    ({ yaml }) =>
      useBackendYamlValidation({
        enabled: true,
        resource: 'pipeline',
        yaml,
        debounceMs: 1,
      }),
    { initialProps: { yaml: 'name: first\nsteps: []\n' } }
  );

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(validateMock).toHaveBeenCalledTimes(1);

  rerender({ yaml: 'name: second\nsteps: []\n' });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(validateMock).toHaveBeenCalledTimes(2);

  await act(async () => {
    resolvers[0]({
      valid: false,
      errors: [{ message: 'first failed', line: 1, code: 'required' }],
      warnings: [],
    });
    await Promise.resolve();
  });
  expect(result.current.status).toBe('checking');
  expect(result.current.errors).toEqual([]);

  await act(async () => {
    resolvers[1](validResponse);
    await Promise.resolve();
  });
  expect(result.current.status).toBe('valid');
});

test('marks validation unavailable without producing blocking errors', async () => {
  validateMock.mockRejectedValue(new Error('network down'));

  const { result } = renderHook(() =>
    useBackendYamlValidation({
      enabled: true,
      resource: 'step',
      yaml: 'name: reusable\nscript: echo ok\n',
      debounceMs: 1,
    })
  );

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });

  expect(result.current.status).toBe('unavailable');
  expect(result.current.error).toBe('network down');
  expect(result.current.blockingErrorCount).toBe(0);
  expect(result.current.errors).toEqual([]);
});
