import { renderHook, waitFor } from '@testing-library/react';
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

test('coordinates folder, update, and execute permission checks', async () => {
  checkPermissionMock.mockImplementation(async action => action !== 'pipeline.update');
  const { result, rerender } = renderHook(
    ({ selectedID, folder }) => usePipelinePermissions(selectedID, folder),
    { initialProps: { selectedID: 'platform/release', folder: 'ignored' } }
  );

  await waitFor(() => {
    expect(result.current).toEqual({
      permissionFolder: 'platform',
      canCreatePipelineHere: true,
      canUpdateSelectedPipeline: false,
      canExecuteSelectedPipeline: true,
    });
  });
  expect(checkPermissionMock).toHaveBeenCalledWith(
    'pipeline.create',
    'platform/__nopsai_permission_probe__'
  );

  rerender({ selectedID: null, folder: '' });
  await waitFor(() => {
    expect(result.current.permissionFolder).toBe('');
    expect(result.current.canUpdateSelectedPipeline).toBe(false);
    expect(result.current.canExecuteSelectedPipeline).toBe(false);
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
