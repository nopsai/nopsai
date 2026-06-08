import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { checkScopePermission } from './api';
import { buildNamedResourceID, useScopePermissions } from './useScopePermissions';

vi.mock('./api', () => ({
  checkScopePermission: vi.fn(),
}));

const checkPermissionMock = vi.mocked(checkScopePermission);

beforeEach(() => {
  checkPermissionMock.mockReset();
});

test('builds stable named resource identifiers', () => {
  expect(buildNamedResourceID('acme/api', '/production/', 'token')).toBe(
    'repo=acme%2Fapi&scope=production&name=token'
  );
  expect(buildNamedResourceID('', 'default', 'token')).toBe('name=token');
});

test('coordinates scope and value-write permissions', async () => {
  checkPermissionMock.mockImplementation(async action => action !== 'secret.write_value');
  const { result, rerender } = renderHook(
    ({ folder, scope }) => useScopePermissions(folder, scope),
    { initialProps: { folder: 'platform', scope: 'production' as string | null } }
  );

  await waitFor(() => {
    expect(result.current).toEqual({
      canCreateScopeHere: true,
      canWriteVariablesInSelectedScope: true,
      canWriteSecretsInSelectedScope: false,
    });
  });
  expect(checkPermissionMock).toHaveBeenCalledWith(
    'scope.update',
    'scope',
    'platform/__nopsai_permission_probe__'
  );

  rerender({ folder: '', scope: null });
  await waitFor(() => {
    expect(result.current.canWriteVariablesInSelectedScope).toBe(false);
    expect(result.current.canWriteSecretsInSelectedScope).toBe(false);
  });
});

test('fails closed when scope permission checks reject', async () => {
  checkPermissionMock.mockRejectedValue(new Error('offline'));
  const { result } = renderHook(() => useScopePermissions('', 'production'));

  await waitFor(() => expect(checkPermissionMock).toHaveBeenCalledTimes(3));
  expect(result.current.canCreateScopeHere).toBe(false);
  expect(result.current.canWriteVariablesInSelectedScope).toBe(false);
  expect(result.current.canWriteSecretsInSelectedScope).toBe(false);
});
