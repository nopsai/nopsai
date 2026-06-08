import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import {
  fetchRunSidebarDetail,
  fetchRunSidebarGroups,
  fetchRunSidebarRecentRuns,
  fetchRunSidebarRepositoryRuns,
} from './runSidebarApi';
import { usePipelineRunsSidebar } from './usePipelineRunsSidebar';
import type { RunListItem } from './types';

vi.mock('./runSidebarApi', () => ({
  fetchRunSidebarDetail: vi.fn(),
  fetchRunSidebarGroups: vi.fn(),
  fetchRunSidebarRecentRuns: vi.fn(),
  fetchRunSidebarRepositoryRuns: vi.fn(),
}));

const fetchDetailMock = vi.mocked(fetchRunSidebarDetail);
const fetchGroupsMock = vi.mocked(fetchRunSidebarGroups);
const fetchRecentRunsMock = vi.mocked(fetchRunSidebarRecentRuns);
const fetchRepositoryRunsMock = vi.mocked(fetchRunSidebarRepositoryRuns);

const runItem = (runID: string): RunListItem => ({
  run_id: runID,
  pipeline_name: `Pipeline ${runID}`,
  status: 'success',
  is_complete: true,
});

beforeEach(() => {
  fetchGroupsMock.mockResolvedValue([]);
  fetchRecentRunsMock.mockResolvedValue([]);
  fetchRepositoryRunsMock.mockResolvedValue({});
  fetchDetailMock.mockResolvedValue(null);
});

test('loads and deduplicates paginated recent runs', async () => {
  const firstPage = Array.from({ length: 200 }, (_, index) => runItem(`run-${index}`));
  fetchRecentRunsMock.mockImplementation(async offset => (
    offset === 0 ? firstPage : [firstPage[0], runItem('run-200')]
  ));

  const { result } = renderHook(() => usePipelineRunsSidebar({
    activeGroupId: null,
    activeRunId: null,
    tab: 'recent',
  }));

  await waitFor(() => expect(result.current.runsLoading).toBe(false));
  expect(result.current.recentRuns).toHaveLength(200);
  expect(result.current.recentHasMore).toBe(true);

  await act(async () => {
    await result.current.loadMoreRecentRuns();
  });

  expect(fetchRecentRunsMock).toHaveBeenLastCalledWith(200, 200);
  expect(result.current.recentRuns).toHaveLength(201);
  expect(result.current.recentRuns.at(-1)?.run_id).toBe('run-200');
  expect(result.current.recentHasMore).toBe(false);
});

test('loads repository runs and expands the active run branch', async () => {
  fetchGroupsMock.mockResolvedValue([
    { id: 1, name: 'Engineering', kind: 'group' },
    {
      id: 2,
      name: 'platform/api',
      kind: 'app',
      parent_id: 1,
      repository_full_name: 'platform/api',
    },
  ]);
  fetchRepositoryRunsMock.mockResolvedValue({
    main: [
      {
        ...runItem('active-run'),
        git_repo_owner: 'platform',
        git_repo_name: 'api',
        git_ref: 'refs/heads/main',
      },
    ],
  });
  fetchDetailMock.mockResolvedValue({
    run_info: {
      ...runItem('active-run'),
      git_repo_owner: 'platform',
      git_repo_name: 'api',
      git_ref: 'refs/heads/main',
    },
  });

  const { result } = renderHook(() => usePipelineRunsSidebar({
    activeGroupId: 2,
    activeRunId: 'active-run',
    tab: 'main',
  }));

  await waitFor(() => {
    expect(result.current.groupsLoading).toBe(false);
    expect(result.current.repoRunsCache.get(2)?.main).toHaveLength(1);
    expect(result.current.expandedGroups).toEqual(new Set([1, 2]));
    expect(result.current.expandedBranches.has('2:main')).toBe(true);
  });

  act(() => result.current.toggleBranch(2, 'main'));
  expect(result.current.expandedBranches.has('2:main')).toBe(false);

  act(() => result.current.toggleGroup(result.current.groups[1]));
  expect(result.current.expandedGroups.has(2)).toBe(false);
  act(() => result.current.toggleGroup(result.current.groups[1]));
  expect(result.current.expandedGroups.has(2)).toBe(true);
  await waitFor(() => expect(fetchRepositoryRunsMock.mock.calls.length).toBeGreaterThan(1));
});
