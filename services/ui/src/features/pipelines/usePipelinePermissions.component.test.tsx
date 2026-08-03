import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { checkPipelinePermission } from './api';
import { usePipelinePermissions } from './usePipelinePermissions';

vi.mock('./api', () => ({
  checkPipelinePermission: vi.fn(),
}));

const checkPermissionMock = vi.mocked(checkPipelinePermission);

beforeEach(() => {
  checkPermissionMock.mockReset();
});

test('coordinates team, update, and execute permission checks', async () => {
  checkPermissionMock.mockImplementation(async action => action !== 'pipeline.update');
  const { result, rerender } = renderHook(
    ({ selectedID, team }) => usePipelinePermissions(selectedID, team),
    { initialProps: { selectedID: 'platform/release', team: 'ignored' } }
  );

  await waitFor(() => {
    expect(result.current).toEqual({
      permissionTeam: 'platform',
      canCreatePipelineHere: true,
      canUpdateSelectedPipeline: false,
      canExecuteSelectedPipeline: true,
    });
  });
  expect(checkPermissionMock).toHaveBeenCalledWith(
    'pipeline.create',
    'platform/__nopsai_permission_probe__'
  );

  rerender({ selectedID: null, team: '' });
  await waitFor(() => {
    expect(result.current.permissionTeam).toBe('');
    expect(result.current.canUpdateSelectedPipeline).toBe(false);
    expect(result.current.canExecuteSelectedPipeline).toBe(false);
  });
  expect(checkPermissionMock).toHaveBeenCalledWith('pipeline.create', '__nopsai_permission_probe__');

  rerender({ selectedID: null, team: 'global' });
  await waitFor(() => {
    expect(result.current.permissionTeam).toBe('');
  });
  expect(checkPermissionMock).toHaveBeenCalledWith('pipeline.create', '__nopsai_permission_probe__');
});

test('fails closed when permission checks reject', async () => {
  checkPermissionMock.mockRejectedValue(new Error('offline'));
  const { result } = renderHook(() => usePipelinePermissions('release', ''));

  await waitFor(() => expect(checkPermissionMock).toHaveBeenCalledTimes(3));
  expect(result.current.canCreatePipelineHere).toBe(false);
  expect(result.current.canUpdateSelectedPipeline).toBe(false);
  expect(result.current.canExecuteSelectedPipeline).toBe(false);
});

test('ignores stale selected-resource permission results after navigation changes', async () => {
  const deferred = new Map<string, { resolve: (value: boolean) => void; promise: Promise<boolean> }>();
  checkPermissionMock.mockImplementation((action, resourceID) => {
    let resolve!: (value: boolean) => void;
    const promise = new Promise<boolean>(innerResolve => {
      resolve = innerResolve;
    });
    deferred.set(`${action}:${resourceID}`, { resolve, promise });
    return promise;
  });

  const { result, rerender } = renderHook(
    ({ selectedID }) => usePipelinePermissions(selectedID, ''),
    { initialProps: { selectedID: 'platform/release' as string | null } }
  );

  await waitFor(() => {
    expect(deferred.has('pipeline.update:platform/release')).toBe(true);
    expect(deferred.has('pipeline.execute:platform/release')).toBe(true);
  });

  rerender({ selectedID: 'platform/hotfix' });

  await waitFor(() => {
    expect(deferred.has('pipeline.update:platform/hotfix')).toBe(true);
    expect(deferred.has('pipeline.execute:platform/hotfix')).toBe(true);
  });

  await act(async () => {
    deferred.get('pipeline.create:platform/__nopsai_permission_probe__')?.resolve(true);
    deferred.get('pipeline.update:platform/hotfix')?.resolve(false);
    deferred.get('pipeline.execute:platform/hotfix')?.resolve(false);
  });

  await waitFor(() => {
    expect(result.current.canUpdateSelectedPipeline).toBe(false);
    expect(result.current.canExecuteSelectedPipeline).toBe(false);
  });

  await act(async () => {
    deferred.get('pipeline.update:platform/release')?.resolve(true);
    deferred.get('pipeline.execute:platform/release')?.resolve(true);
  });

  expect(result.current.canUpdateSelectedPipeline).toBe(false);
  expect(result.current.canExecuteSelectedPipeline).toBe(false);
});
