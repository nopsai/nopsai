import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import {
  updateYamlResourceName,
  useYamlResourceMutations,
} from './useYamlResourceMutations';

type TestDetail = {
  id: string;
  rawYaml: string;
  source?: string;
  name: string;
};

const checkCreatePermission = vi.fn();
const persistYaml = vi.fn();
const deleteResource = vi.fn();
const upsertDraft = vi.fn();
const removeDraft = vi.fn();
const reloadResources = vi.fn();
const addToast = vi.fn();
const onSelect = vi.fn();
const onSaved = vi.fn();
const onDeleted = vi.fn();

function buildOptions(overrides: Partial<Parameters<typeof useYamlResourceMutations<TestDetail>>[0]> = {}) {
  return {
    resourceLabel: 'pipeline' as const,
    resources: [{ id: 'team/release', source: 'database' }],
    detail: null,
    editorValue: '',
    validationErrorCount: 0,
    validationMessage: 'Fix validation errors before saving.',
    permissionFolder: 'team',
    draftScope: 'user-1',
    canCreate: true,
    canUpdate: true,
    canDelete: true,
    canUseDrafts: true,
    namePattern: /^[a-zA-Z0-9_.-]+$/,
    normalizePath: (path: string) => path.trim().replace(/^\/+|\/+$/g, ''),
    normalizeSource: (source?: string) => source || 'database',
    checkCreatePermission,
    persistYaml,
    deleteResource,
    upsertDraft,
    removeDraft,
    parseSaved: (rawYaml: string, id: string, source?: string): TestDetail => ({
      id,
      rawYaml,
      source,
      name: id.split('/').pop() || '',
    }),
    reloadResources,
    addToast,
    onSelect,
    onSaved,
    onDeleted,
    buildTemplate: (name: string) => `name: ${name}\nsteps: []\n`,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  checkCreatePermission.mockResolvedValue(true);
  persistYaml.mockResolvedValue(undefined);
  deleteResource.mockResolvedValue(undefined);
  reloadResources.mockResolvedValue(undefined);
});

test('creates and clones drafts only after an action-time permission check', async () => {
  const { result } = renderHook(() => useYamlResourceMutations(buildOptions()));

  act(() => {
    result.current.openCreateModal();
    result.current.updateFormModal({ name: 'build', path: '/team/' });
  });
  await act(async () => {
    expect(await result.current.submitFormModal()).toBe(true);
  });

  expect(checkCreatePermission).toHaveBeenCalledWith('pipeline.create', 'team/build');
  expect(upsertDraft).toHaveBeenCalledWith({
    id: 'team/build',
    yaml: 'name: build\nsteps: []\n',
  });
  expect(onSelect).toHaveBeenCalledWith('team/build');

  const clone = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        detail: {
          id: 'team/release',
          name: 'release',
          rawYaml: 'name: release\nsteps: []\n',
          source: 'git',
        },
      })
    )
  );
  act(() => {
    clone.result.current.openCloneModal();
  });
  expect(clone.result.current.formModal?.name).toBe('release-copy');
  await act(async () => {
    expect(await clone.result.current.submitFormModal()).toBe(true);
  });
  expect(upsertDraft).toHaveBeenLastCalledWith({
    id: 'team/release-copy',
    yaml: 'name: release-copy\nsteps: []\n',
  });
});

test('persists an editable draft and converts it to a database resource', async () => {
  const detail: TestDetail = {
    id: 'team/release',
    name: 'release',
    rawYaml: 'name: release\nsteps: []\n',
    source: 'draft',
  };
  const { result } = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        resources: [{ id: detail.id, source: 'draft' }],
        detail,
        editorValue: 'name: release\nsteps:\n  - name: build\n',
      })
    )
  );

  await act(async () => {
    expect(await result.current.save()).toBe(true);
  });

  expect(persistYaml).toHaveBeenCalledWith(
    detail.id,
    'name: release\nsteps:\n  - name: build\n'
  );
  expect(removeDraft).toHaveBeenCalledWith(detail.id);
  expect(onSaved).toHaveBeenCalledWith(
    expect.objectContaining({ id: detail.id, source: 'database' })
  );
  expect(reloadResources).toHaveBeenCalledWith({ quiet: true });
  expect(result.current.saving).toBe(false);
});

test('saves GitOps resources as database overrides and handles draft and persisted deletion separately', async () => {
  const git = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        resources: [{ id: 'team/release', source: 'git' }],
        detail: {
          id: 'team/release',
          name: 'release',
          rawYaml: 'name: release\nsteps: []\n',
          source: 'git',
        },
        editorValue: 'name: release\nsteps: []\n',
      })
    )
  );
  await act(async () => {
    expect(await git.result.current.save()).toBe(true);
  });
  expect(persistYaml).toHaveBeenCalledWith('team/release', 'name: release\nsteps: []\n');
  expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ source: 'database' }));
  expect(addToast).toHaveBeenCalledWith(
    'Pipeline saved as a database override. The next GitOps sync can replace it unless it is pushed to GitOps.',
    'success'
  );
  vi.clearAllMocks();

  const draft = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        resources: [{ id: 'draft-one', source: 'draft' }],
      })
    )
  );
  act(() => {
    draft.result.current.openDeleteModal('draft-one', 'draft-one');
  });
  await act(async () => {
    expect(await draft.result.current.confirmDelete()).toBe(true);
  });
  expect(removeDraft).toHaveBeenCalledWith('draft-one');
  expect(deleteResource).not.toHaveBeenCalled();
  expect(onDeleted).toHaveBeenCalled();

  const persisted = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        resources: [{ id: 'stored-one', source: 'git' }],
      })
    )
  );
  act(() => {
    persisted.result.current.openDeleteModal('stored-one', 'stored-one');
  });
  expect(persisted.result.current.deleteModal).toMatchObject({ gitOpsManaged: true });
  await act(async () => {
    expect(await persisted.result.current.confirmDelete()).toBe(true);
  });
  expect(deleteResource).toHaveBeenCalledWith('stored-one');
  expect(addToast).toHaveBeenCalledWith(
    'Pipeline database row deleted. The next GitOps sync can recreate it from the repository.',
    'success'
  );
});

test('updates a YAML name even when the source is malformed', () => {
  expect(updateYamlResourceName('name: old\nsteps: []\n', 'new')).toContain('name: new');
  expect(updateYamlResourceName('not: [valid', 'new')).toBe('name: new\nnot: [valid');
});

test('reports create validation and action-time authorization failures in the modal', async () => {
  const readOnly = renderHook(() =>
    useYamlResourceMutations(buildOptions({ canCreate: false }))
  );
  act(() => {
    readOnly.result.current.openCreateModal();
    readOnly.result.current.openCloneModal();
  });
  expect(readOnly.result.current.formModal).toBeNull();
  expect(addToast).toHaveBeenCalledWith(
    'You have read-only access to pipelines.',
    'info'
  );

  const missing = renderHook(() => useYamlResourceMutations(buildOptions()));
  act(() => {
    missing.result.current.openCreateModal();
  });
  await act(async () => {
    expect(await missing.result.current.submitFormModal()).toBe(false);
  });
  expect(missing.result.current.formModal?.error).toBe('Pipeline name is required.');

  act(() => {
    missing.result.current.updateFormModal({ name: 'invalid name' });
  });
  await act(async () => {
    expect(await missing.result.current.submitFormModal()).toBe(false);
  });
  expect(missing.result.current.formModal?.error).toContain(
    'Pipeline name can only contain'
  );

  checkCreatePermission.mockResolvedValue(false);
  act(() => {
    missing.result.current.updateFormModal({ name: 'denied' });
  });
  await act(async () => {
    expect(await missing.result.current.submitFormModal()).toBe(false);
  });
  expect(missing.result.current.formModal?.error).toBe(
    'You do not have permission to create pipelines in this path.'
  );
});

test('surfaces persistence and deletion failures without leaving pending state behind', async () => {
  persistYaml.mockRejectedValueOnce(new Error('write failed'));
  const saveFailure = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        detail: {
          id: 'team/release',
          name: 'release',
          rawYaml: 'name: release\nsteps: []\n',
          source: 'database',
        },
        editorValue: 'name: release\nsteps: []\n',
      })
    )
  );
  await act(async () => {
    expect(await saveFailure.result.current.save()).toBe(false);
  });
  expect(addToast).toHaveBeenCalledWith('write failed', 'error');
  expect(saveFailure.result.current.saving).toBe(false);

  const deleteFailure = renderHook(() =>
    useYamlResourceMutations(
      buildOptions({
        canDelete: false,
        resources: [{ id: 'stored-one', source: 'database' }],
      })
    )
  );
  act(() => {
    deleteFailure.result.current.openDeleteModal('stored-one', 'stored-one');
  });
  await act(async () => {
    expect(await deleteFailure.result.current.confirmDelete()).toBe(false);
  });
  expect(deleteFailure.result.current.deleteModal).toMatchObject({
    pending: false,
    error: 'You do not have permission to delete pipelines.',
  });
});
