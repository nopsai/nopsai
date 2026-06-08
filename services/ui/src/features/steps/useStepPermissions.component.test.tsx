import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { checkStepPermission } from './api';
import { useStepPermissions } from './useStepPermissions';

vi.mock('./api', () => ({
  checkStepPermission: vi.fn(),
}));

const checkPermissionMock = vi.mocked(checkStepPermission);

beforeEach(() => {
  checkPermissionMock.mockReset();
});

test('coordinates step create and update permissions', async () => {
  checkPermissionMock.mockImplementation(async action => action === 'step.create');
  const { result, rerender } = renderHook(
    ({ selectedID, folder }) => useStepPermissions(selectedID, folder),
    { initialProps: { selectedID: 'platform/build', folder: '' } }
  );

  await waitFor(() => {
    expect(result.current).toEqual({
      permissionFolder: 'platform',
      canCreateStepHere: true,
      canUpdateSelectedStep: false,
    });
  });

  rerender({ selectedID: null, folder: 'shared' });
  await waitFor(() => expect(result.current.permissionFolder).toBe('shared'));
  expect(checkPermissionMock).toHaveBeenCalledWith(
    'step.create',
    'shared/__nopsai_permission_probe__'
  );
  expect(result.current.canUpdateSelectedStep).toBe(false);
});

test('fails closed when step permission checks reject', async () => {
  checkPermissionMock.mockRejectedValue(new Error('offline'));
  const { result } = renderHook(() => useStepPermissions('build', ''));

  await waitFor(() => expect(checkPermissionMock).toHaveBeenCalledTimes(2));
  expect(result.current.canCreateStepHere).toBe(false);
  expect(result.current.canUpdateSelectedStep).toBe(false);
});
