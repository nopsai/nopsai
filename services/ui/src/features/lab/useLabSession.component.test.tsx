import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiClient } from '../../lib/api';
import { buildBlankLabYaml, useLabSession } from './useLabSession';

vi.mock('../../lib/api', () => ({
  apiClient: { fetch: vi.fn() },
}));

const fetchMock = vi.mocked(apiClient.fetch);

async function withMutedConsoleError(run: () => Promise<void>) {
  const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
  try {
    await run();
  } finally {
    errorSpy.mockRestore();
  }
}

beforeEach(() => {
  sessionStorage.clear();
  fetchMock.mockReset();
  vi.restoreAllMocks();
});

test('restores, edits, and persists the isolated Lab session', () => {
  sessionStorage.setItem(
    'nopsai.lab.session.v1',
    JSON.stringify({
      version: 1,
      selectedPipelineId: 'team/release',
      yaml: 'name: release\nsteps: []\n',
      originalYaml: 'name: release\nsteps: []\n',
      scope: 'production',
      overrides: [{ key: 'region', value: 'eu' }],
    })
  );
  const { result } = renderHook(() => useLabSession());

  expect(result.current.selectedPipelineId).toBe('team/release');
  expect(result.current.scopeValue).toBe('production');
  expect(result.current.overrides).toEqual([{ id: 1, key: 'region', value: 'eu' }]);

  act(() => {
    result.current.addOverride();
  });
  expect(result.current.overrides[1]).toEqual({ id: 2, key: '', value: '' });

  act(() => {
    result.current.updateOverride(2, 'key', 'environment');
    result.current.updateOverride(2, 'value', 'staging');
    result.current.setYamlText('name: release-v2\nsteps: []\n');
  });
  expect(result.current.hasUnsavedChanges).toBe(true);

  act(() => {
    expect(result.current.saveSession(0)).toBe(true);
  });
  expect(result.current.feedback?.tone).toBe('success');
  expect(JSON.parse(sessionStorage.getItem('nopsai.lab.session.v1') || '{}')).toMatchObject({
    selectedPipelineId: 'team/release',
    yaml: 'name: release-v2\nsteps: []\n',
    scope: 'production',
    overrides: [
      { key: 'region', value: 'eu' },
      { key: 'environment', value: 'staging' },
    ],
  });

  act(() => {
    result.current.removeOverride(1);
  });
  expect(result.current.overrides).toHaveLength(1);
});

test('blocks invalid saves and protects unsaved pipeline changes', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
  const { result } = renderHook(() => useLabSession());

  act(() => {
    expect(result.current.saveSession(1)).toBe(false);
    result.current.setYamlText('name: changed\nsteps: []\n');
  });
  expect(result.current.feedback?.message).toBe('Fix validation errors before saving.');

  await act(async () => {
    expect(await result.current.changePipeline('team/release')).toBe(false);
  });
  expect(confirm).toHaveBeenCalled();
  expect(fetchMock).not.toHaveBeenCalled();
});

test('loads selected pipeline YAML and can reset to a blank definition', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  fetchMock.mockResolvedValue(new Response('name: release\nsteps: []\n', { status: 200 }));
  const { result } = renderHook(() => useLabSession());

  await act(async () => {
    expect(await result.current.changePipeline('team/release')).toBe(true);
  });
  expect(fetchMock).toHaveBeenCalledWith('/v1/pipelines/team/release');
  expect(result.current.yamlText).toBe('name: release\nsteps: []\n');
  expect(result.current.yamlLoading).toBe(false);

  await act(async () => {
    expect(await result.current.changePipeline('')).toBe(true);
  });
  expect(result.current.yamlText).toBe(buildBlankLabYaml());
});

test('keeps pipeline load failures visible and clears loading state', async () => {
  fetchMock.mockResolvedValue(new Response('pipeline unavailable', { status: 503 }));
  const { result } = renderHook(() => useLabSession());

  await withMutedConsoleError(async () => {
    await act(async () => {
      expect(await result.current.changePipeline('team/release')).toBe(false);
    });
  });
  expect(result.current.feedback).toEqual({
    tone: 'error',
    message: 'pipeline unavailable',
  });
  expect(result.current.yamlLoading).toBe(false);
});
