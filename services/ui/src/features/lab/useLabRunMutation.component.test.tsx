import { useState } from 'react';
import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiClient } from '../../lib/api';
import type { LabFeedback } from './useLabSession';
import { extractLabRunID, useLabRunMutation } from './useLabRunMutation';

vi.mock('../../lib/api', () => ({
  apiClient: { fetch: vi.fn() },
}));

const fetchMock = vi.mocked(apiClient.fetch);

beforeEach(() => {
  fetchMock.mockReset();
});

test('extracts full, short, and object run identifiers', () => {
  expect(extractLabRunID({ run_id: 'run-123' })).toBe('run-123');
  expect(extractLabRunID('started 12345678-abcd-1234-abcd-1234567890ab')).toBe(
    '12345678-abcd-1234-abcd-1234567890ab'
  );
  expect(extractLabRunID('started 12345678-abcd')).toBe('12345678-abcd');
  expect(extractLabRunID(null)).toBe('');
});

test('validates authorization and override inputs before mutation', async () => {
  const { result, rerender } = renderHook(
    ({ accessBlocked, accessLoading, validationErrorCount, key }) => {
      const [feedback, setFeedback] = useState<LabFeedback>(null);
      const mutation = useLabRunMutation({
        accessBlocked,
        accessLoading,
        overrides: [{ id: 1, key, value: 'value' }],
        scopeValue: '',
        selectedPipelineId: 'release',
        setFeedback,
        validationErrorCount,
        yamlText: 'name: release\nsteps: []\n',
      });
      return { ...mutation, feedback };
    },
    {
      initialProps: {
        accessBlocked: false,
        accessLoading: false,
        validationErrorCount: 1,
        key: 'valid',
      },
    }
  );

  await act(async () => {
    expect(await result.current.run()).toBe(false);
  });
  expect(result.current.feedback?.message).toBe('Fix validation errors first.');

  rerender({
    accessBlocked: true,
    accessLoading: false,
    validationErrorCount: 0,
    key: 'valid',
  });
  await act(async () => {
    expect(await result.current.run()).toBe(false);
  });
  expect(result.current.feedback?.message).toBe('Run access is not ready yet.');

  rerender({
    accessBlocked: false,
    accessLoading: true,
    validationErrorCount: 0,
    key: 'valid',
  });
  await act(async () => {
    expect(await result.current.run()).toBe(false);
  });
  expect(result.current.feedback?.tone).toBe('info');

  rerender({
    accessBlocked: false,
    accessLoading: false,
    validationErrorCount: 0,
    key: 'invalid key',
  });
  await act(async () => {
    expect(await result.current.run()).toBe(false);
  });
  expect(result.current.feedback?.message).toContain("Invalid override key 'invalid key'");
  expect(fetchMock).not.toHaveBeenCalled();
});

test('submits the authorized scoped run with temporary variables', async () => {
  fetchMock.mockResolvedValue(
    new Response(JSON.stringify({ run_id: 'run-123' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  );
  const { result } = renderHook(() => {
    const [feedback, setFeedback] = useState<LabFeedback>(null);
    const mutation = useLabRunMutation({
      accessBlocked: false,
      accessLoading: false,
      overrides: [{ id: 1, key: ' region ', value: 'eu' }],
      scopeValue: 'production',
      selectedPipelineId: 'team/release',
      setFeedback,
      validationErrorCount: 0,
      yamlText: 'name: release\nsteps: []\n',
    });
    return { ...mutation, feedback };
  });

  await act(async () => {
    expect(await result.current.run()).toBe(true);
  });

  expect(fetchMock).toHaveBeenCalledWith('/v1/run', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Nopsai-Scope': 'production',
    },
    body: JSON.stringify({
      pipeline: 'team/release',
      definition: 'name: release\nsteps: []\n',
      variables: { region: 'eu' },
      scope: 'production',
    }),
  });
  expect(result.current.feedback).toEqual({
    tone: 'success',
    message: 'Run started!',
    runId: 'run-123',
  });
  expect(result.current.runPending).toBe(false);
});

test('surfaces plain-text run failures and uses the YAML name when no pipeline is selected', async () => {
  fetchMock.mockResolvedValue(new Response('runner unavailable', { status: 503 }));
  const { result } = renderHook(() => {
    const [feedback, setFeedback] = useState<LabFeedback>(null);
    const mutation = useLabRunMutation({
      accessBlocked: false,
      accessLoading: false,
      overrides: [],
      scopeValue: '',
      selectedPipelineId: '',
      setFeedback,
      validationErrorCount: 0,
      yamlText: 'name: release\nsteps: []\n',
    });
    return { ...mutation, feedback };
  });

  await act(async () => {
    expect(await result.current.run()).toBe(false);
  });
  expect(fetchMock).toHaveBeenCalledWith(
    '/v1/run',
    expect.objectContaining({
      body: JSON.stringify({
        pipeline: 'release',
        definition: 'name: release\nsteps: []\n',
      }),
    })
  );
  expect(result.current.feedback).toEqual({
    tone: 'error',
    message: 'Run failed: runner unavailable',
  });
});
