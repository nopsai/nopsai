import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { checkTriggerPermission } from './api';
import { useTriggerPermissions } from './useTriggerPermissions';

vi.mock('./api', () => ({
  checkTriggerPermission: vi.fn(),
}));

const checkPermissionMock = vi.mocked(checkTriggerPermission);

beforeEach(() => {
  checkPermissionMock.mockReset();
});

test('coordinates folder and selected trigger permissions', async () => {
  checkPermissionMock.mockImplementation(async (_action, resourceID) => resourceID.includes('acme/'));
  const { result, rerender } = renderHook(
    ({ folder, slug }) => useTriggerPermissions(folder, slug),
    { initialProps: { folder: 'acme', slug: 'acme/service' as string | null } }
  );

  await waitFor(() => {
    expect(result.current.canCreateTriggerHere).toBe(true);
    expect(result.current.canUpdateSelectedTrigger).toBe(true);
  });
  expect(checkPermissionMock).toHaveBeenCalledWith(
    'trigger.update',
    'acme/__nopsai_permission_probe__'
  );

  rerender({ folder: '', slug: null });
  await waitFor(() => expect(result.current.canCreateTriggerHere).toBe(false));
  expect(result.current.canUpdateSelectedTrigger).toBe(false);
  expect(checkPermissionMock).toHaveBeenCalledWith('trigger.update', '__nopsai_permission_probe__');
});

test('fails closed when trigger permission checks reject', async () => {
  checkPermissionMock.mockRejectedValue(new Error('offline'));
  const { result } = renderHook(() => useTriggerPermissions('', 'acme/service'));

  await waitFor(() => expect(checkPermissionMock).toHaveBeenCalledTimes(2));
  expect(result.current.canCreateTriggerHere).toBe(false);
  expect(result.current.canUpdateSelectedTrigger).toBe(false);
});
