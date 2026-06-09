import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import {
  checkScopePermission,
  deleteScopedValue,
  encryptSecretValue,
  saveScopedValue,
} from './api';
import { SAMPLE_SCOPE_VARIABLE, useScopeModalMutations } from './useScopeModalMutations';

vi.mock('./api', () => ({
  checkScopePermission: vi.fn(),
  deleteScopedValue: vi.fn(),
  encryptSecretValue: vi.fn(),
  saveScopedValue: vi.fn(),
  scopedResourcePath: (kind: 'variable' | 'secret', scope: string, name: string, repositorySlug = '') => {
    const plural = kind === 'variable' ? 'variables' : 'secrets';
    return `${repositorySlug ? `/repo/${repositorySlug}` : ''}/${plural}/${name}${scope ? `?scope=${scope}` : ''}`;
  },
}));

const checkScopePermissionMock = vi.mocked(checkScopePermission);
const deleteScopedValueMock = vi.mocked(deleteScopedValue);
const encryptSecretValueMock = vi.mocked(encryptSecretValue);
const saveScopedValueMock = vi.mocked(saveScopedValue);

const addToast = vi.fn();
const loadScopes = vi.fn();
const ensureScopeVariables = vi.fn();
const ensureScopeSecrets = vi.fn();
const selectVariable = vi.fn();
const selectSecret = vi.fn();
const clearExpandedVariable = vi.fn();
const onScopeCreated = vi.fn();

function renderMutations(overrides: Partial<Parameters<typeof useScopeModalMutations>[0]> = {}) {
  return renderHook(() =>
    useScopeModalMutations({
      activeFolder: 'team',
      scopesByLabel: new Map([['team/existing', {}]]),
      scopeDataByScope: {
        team: {
          variables: ['owner/repo/API_URL', 'API_URL_copy'],
          secrets: ['owner/repo/API_TOKEN'],
        },
      },
      canCreateScopeHere: true,
      canWriteVariablesInSelectedScope: true,
      canWriteSecretsInSelectedScope: true,
      canDeleteScopes: true,
      selectedVariable: 'owner/repo/API_URL',
      selectedSecret: 'owner/repo/API_TOKEN',
      addToast,
      loadScopes,
      ensureScopeVariables,
      ensureScopeSecrets,
      selectVariable,
      selectSecret,
      clearExpandedVariable,
      onScopeCreated,
      ...overrides,
    })
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  checkScopePermissionMock.mockResolvedValue(true);
  deleteScopedValueMock.mockResolvedValue(undefined);
  encryptSecretValueMock.mockResolvedValue('ENC[value]');
  saveScopedValueMock.mockResolvedValue(undefined);
  loadScopes.mockResolvedValue(undefined);
  ensureScopeVariables.mockResolvedValue(undefined);
  ensureScopeSecrets.mockResolvedValue(undefined);
  Object.assign(navigator, {
    clipboard: {
      writeText: vi.fn().mockResolvedValue(undefined),
    },
  });
});

test('creates nested scopes only after scope and seeded-variable authorization', async () => {
  const { result } = renderMutations();

  act(() => {
    result.current.openNewScopeModal();
    result.current.updateScopeName('existing');
  });
  await act(async () => {
    expect(await result.current.submitScopeModal()).toBe(false);
  });
  expect(result.current.scopeModal?.error).toBe('Scope “/team/existing” already exists.');

  act(() => {
    result.current.updateScopeName('new/app');
  });
  await act(async () => {
    expect(await result.current.submitScopeModal()).toBe(true);
  });

  expect(checkScopePermissionMock).toHaveBeenCalledWith('scope.update', 'scope', 'team/new/app');
  expect(checkScopePermissionMock).toHaveBeenCalledWith(
    'variable.write_value',
    'variable',
    expect.stringContaining(`name=${SAMPLE_SCOPE_VARIABLE}`)
  );
  expect(saveScopedValueMock).toHaveBeenCalledTimes(3);
  expect(ensureScopeVariables).toHaveBeenCalledWith('team/new/app', true);
  expect(onScopeCreated).toHaveBeenCalledWith('team/new/app');
});

test('validates and saves repository-scoped variables with action-time permission', async () => {
  const { result } = renderMutations();

  act(() => {
    result.current.openVariableCreateModal('team');
  });
  await act(async () => {
    expect(await result.current.submitVariableModal()).toBe(false);
  });
  expect(result.current.variableModal?.error).toBe('Variable name is required.');

  checkScopePermissionMock.mockResolvedValue(false);
  act(() => {
    result.current.updateVariableModal({
      name: 'API_URL',
      repository: 'owner/repo',
      value: 'https://example.test',
    });
  });
  await act(async () => {
    expect(await result.current.submitVariableModal()).toBe(false);
  });
  expect(result.current.variableModal?.error).toBe('You do not have permission to save variables in this scope.');

  checkScopePermissionMock.mockResolvedValue(true);
  await act(async () => {
    expect(await result.current.submitVariableModal()).toBe(true);
  });
  expect(saveScopedValueMock).toHaveBeenCalledWith(
    '/repo/owner/repo/variables/API_URL?scope=team',
    'https://example.test',
    'variable'
  );
  expect(ensureScopeVariables).toHaveBeenCalledWith('team', true);
  expect(selectVariable).toHaveBeenCalledWith('owner/repo/API_URL');
  expect(clearExpandedVariable).toHaveBeenCalled();
});

test('clones, updates, and no-ops secrets without exposing old values', async () => {
  const { result } = renderMutations();

  act(() => {
    result.current.openSecretCloneModal('team', 'owner/repo/API_TOKEN');
  });
  expect(result.current.secretModal).toMatchObject({
    mode: 'create',
    repository: 'owner/repo',
    name: 'API_TOKEN_copy',
    value: '',
  });

  act(() => {
    result.current.updateSecretModal({ value: 'secret-value' });
  });
  await act(async () => {
    expect(await result.current.submitSecretModal()).toBe(true);
  });
  expect(saveScopedValueMock).toHaveBeenCalledWith(
    '/repo/owner/repo/secrets/API_TOKEN_copy?scope=team',
    'secret-value',
    'secret'
  );
  expect(selectSecret).toHaveBeenCalledWith('owner/repo/API_TOKEN_copy');

  act(() => {
    result.current.openSecretUpdateModal('team', 'owner/repo/API_TOKEN');
  });
  await act(async () => {
    expect(await result.current.submitSecretModal()).toBe(true);
  });
  expect(addToast).toHaveBeenCalledWith('Secret value updated (unchanged).', 'info');
});

test('encrypts and copies GitOps secret values', async () => {
  const { result } = renderMutations();

  act(() => {
    result.current.openGitOpsEncryptModal();
  });
  await act(async () => {
    expect(await result.current.encryptGitOpsSecretValue()).toBe(false);
  });
  expect(result.current.gitOpsEncryptModal?.error).toBe('Provide a value to encrypt.');

  act(() => {
    result.current.updateGitOpsEncryptValue('plain');
  });
  await act(async () => {
    expect(await result.current.encryptGitOpsSecretValue()).toBe(true);
  });
  expect(encryptSecretValueMock).toHaveBeenCalledWith('plain');
  expect(result.current.gitOpsEncryptModal?.encryptedValue).toBe('ENC[value]');

  await act(async () => {
    expect(await result.current.copyGitOpsEncryptedValue()).toBe(true);
  });
  expect(navigator.clipboard.writeText).toHaveBeenCalledWith('ENC[value]');
  expect(addToast).toHaveBeenCalledWith('Encrypted value copied.', 'success');
});

test('deletes selected scoped values and keeps failures in the delete modal', async () => {
  deleteScopedValueMock.mockRejectedValueOnce(new Error('delete failed'));
  const { result } = renderMutations();

  act(() => {
    result.current.openDeleteModal('variable', 'team', 'owner/repo/API_URL');
  });
  await act(async () => {
    expect(await result.current.confirmDelete()).toBe(false);
  });
  expect(result.current.deleteModal).toMatchObject({
    pending: false,
    error: 'delete failed',
  });

  deleteScopedValueMock.mockResolvedValue(undefined);
  await act(async () => {
    expect(await result.current.confirmDelete()).toBe(true);
  });
  expect(deleteScopedValueMock).toHaveBeenLastCalledWith('/repo/owner/repo/variables/API_URL?scope=team');
  expect(ensureScopeVariables).toHaveBeenCalledWith('team', true);
  expect(selectVariable).toHaveBeenCalledWith(null);
  expect(loadScopes).toHaveBeenCalled();
});
