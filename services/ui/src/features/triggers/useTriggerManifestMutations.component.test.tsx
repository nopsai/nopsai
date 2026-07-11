import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import {
  checkTriggerPermission,
  deleteTrigger,
  saveTrigger,
} from './api';
import type { TriggerDetail } from './model';
import { useTriggerManifestMutations } from './useTriggerManifestMutations';

vi.mock('./api', () => ({
  checkTriggerPermission: vi.fn(),
  deleteTrigger: vi.fn(),
  saveTrigger: vi.fn(),
}));

const checkTriggerPermissionMock = vi.mocked(checkTriggerPermission);
const deleteTriggerMock = vi.mocked(deleteTrigger);
const saveTriggerMock = vi.mocked(saveTrigger);

const addToast = vi.fn();
const loadTriggers = vi.fn();
const loadRecentRuns = vi.fn();
const onSelectSlug = vi.fn();
const onSaved = vi.fn();
const onEditingFinished = vi.fn();
const onDeleted = vi.fn();

const detail: TriggerDetail = {
  slug: 'owner/repo',
  source: 'database',
  rawYaml: 'triggers:\n  - on: push\n    pipelines:\n      - pipelines/release.yaml\n',
  summary: {
    triggerCount: 1,
    pipelines: [{ identifier: 'release', display: 'release', pathLabel: 'root' }],
    events: ['push'],
    branches: [],
    skipBranches: [],
    tags: [],
    scopes: [''],
  },
};

function renderMutation(overrides: Partial<Parameters<typeof useTriggerManifestMutations>[0]> = {}) {
  return renderHook(() =>
    useTriggerManifestMutations({
      canCreateTriggerHere: true,
      canUpdateSelectedTrigger: true,
      canDeleteTriggers: true,
      permissionTeam: 'team',
      detail,
      editorValue: detail.rawYaml,
      validationErrorCount: 0,
      serverTriggers: [{ slug: detail.slug, source: 'database' }],
      addToast,
      loadTriggers,
      loadRecentRuns,
      onSelectSlug,
      onSaved,
      onEditingFinished,
      onDeleted,
      ...overrides,
    })
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  checkTriggerPermissionMock.mockResolvedValue(true);
  deleteTriggerMock.mockResolvedValue(undefined);
  saveTriggerMock.mockResolvedValue(undefined);
  loadTriggers.mockResolvedValue(undefined);
  loadRecentRuns.mockResolvedValue(undefined);
});

test('creates trigger manifests with repository templates and action-time authorization', async () => {
  const { result } = renderMutation();

  act(() => {
    result.current.openCreateModal();
  });
  expect(result.current.createModal?.repository).toBe('team/new-repository');
  expect(result.current.createModal?.yamlPreview).toContain('pipelines/new-repository.yaml');

  act(() => {
    result.current.updateCreateRepository('owner/new-repo');
  });
  expect(result.current.createModal?.yamlPreview).toContain('pipelines/new-repo.yaml');

  await act(async () => {
    expect(await result.current.submitCreateModal()).toBe(true);
  });

  expect(checkTriggerPermissionMock).toHaveBeenCalledWith('trigger.update', 'owner/new-repo');
  expect(saveTriggerMock).toHaveBeenCalledWith(
    'owner/new-repo',
    expect.stringContaining('pipelines/new-repo.yaml')
  );
  expect(loadTriggers).toHaveBeenCalled();
  expect(onSelectSlug).toHaveBeenCalledWith('owner/new-repo');
});

test('keeps create and clone modal failures local to the modal state', async () => {
  const { result } = renderMutation({ permissionTeam: '' });

  act(() => {
    result.current.openCreateModal();
  });
  await act(async () => {
    expect(await result.current.submitCreateModal()).toBe(false);
  });
  expect(result.current.createModal?.error).toBe('Repository is required.');

  act(() => {
    result.current.updateCreateRepository('invalid');
  });
  await act(async () => {
    expect(await result.current.submitCreateModal()).toBe(false);
  });
  expect(result.current.createModal?.error).toBe('Repository must be in owner/name format.');

  checkTriggerPermissionMock.mockResolvedValue(false);
  act(() => {
    result.current.updateCreateRepository('owner/denied');
  });
  await act(async () => {
    expect(await result.current.submitCreateModal()).toBe(false);
  });
  expect(result.current.createModal?.error).toBe(
    'You do not have permission to create triggers for this repository.'
  );

  act(() => {
    result.current.openCloneModal();
    result.current.updateCloneRepository('');
  });
  await act(async () => {
    expect(await result.current.submitCloneModal()).toBe(false);
  });
  expect(result.current.cloneModal?.error).toBe('Repository is required.');
});

test('saves editable trigger manifests and refreshes dependent runs', async () => {
  const editorValue = 'triggers:\n  - on: pull_request\n    pipelines:\n      - pipelines/release.yaml\n';
  const { result } = renderMutation({ editorValue });

  await act(async () => {
    expect(await result.current.save()).toBe(true);
  });

  expect(saveTriggerMock).toHaveBeenCalledWith(detail.slug, editorValue);
  expect(onSaved).toHaveBeenCalledWith(
    expect.objectContaining({
      slug: detail.slug,
      rawYaml: editorValue,
      summary: expect.objectContaining({ events: ['pull_request'] }),
    })
  );
  expect(onEditingFinished).toHaveBeenCalled();
  expect(loadRecentRuns).toHaveBeenCalledWith(
    detail.slug,
    expect.arrayContaining([expect.objectContaining({ identifier: 'release' })])
  );
});

test('saves and deletes GitOps triggers as database overrides and surfaces delete failures', async () => {
  const editorValue = 'triggers:\n  - on: pull_request\n    pipelines:\n      - pipelines/release.yaml\n';
  const git = renderMutation({
    detail: { ...detail, source: 'git' },
    editorValue,
    serverTriggers: [{ slug: detail.slug, source: 'git' }],
  });

  await act(async () => {
    expect(await git.result.current.save()).toBe(true);
  });
  expect(saveTriggerMock).toHaveBeenCalledWith(detail.slug, editorValue);
  expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ source: 'database' }));
  expect(addToast).toHaveBeenCalledWith(
    'Trigger saved as a database override. The next GitOps sync can replace it unless it is pushed to GitOps.',
    'success'
  );

  act(() => {
    git.result.current.openDeleteModal(detail.slug);
  });
  expect(git.result.current.deleteModal).toMatchObject({ slug: detail.slug, gitOpsManaged: true });

  await act(async () => {
    expect(await git.result.current.confirmDelete()).toBe(true);
  });
  expect(deleteTriggerMock).toHaveBeenCalledWith(detail.slug);
  expect(addToast).toHaveBeenCalledWith(
    'Trigger database row deleted. The next GitOps sync can recreate it from the repository.',
    'success'
  );

  deleteTriggerMock.mockRejectedValueOnce(new Error('delete failed'));
  const stored = renderMutation();
  act(() => {
    stored.result.current.openDeleteModal(detail.slug);
  });
  await act(async () => {
    expect(await stored.result.current.confirmDelete()).toBe(false);
  });
  expect(stored.result.current.deleteModal).toMatchObject({
    slug: detail.slug,
    pending: false,
    error: 'delete failed',
  });
});
