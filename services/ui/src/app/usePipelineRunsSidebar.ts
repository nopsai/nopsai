import { useCallback, useEffect, useRef, useState } from 'react';
import { SIDEBAR_RECENT_PAGE_SIZE } from './constants';
import {
  fetchRunSidebarDetail,
  fetchRunSidebarGroups,
  fetchRunSidebarRecentRuns,
  fetchRunSidebarRepositoryRuns,
} from './runSidebarApi';
import { buildGroupPath, formatBranch, isRunAppGroup, runGroupMatchesRepository } from './runSidebarUtils';
import type { RunGroup, RunListItem, RunTabKey } from './types';

type PipelineRunsSidebarOptions = {
  activeGroupId: number | null;
  activeRunId: string | null;
  tab: RunTabKey;
};

export function usePipelineRunsSidebar({ activeGroupId, activeRunId, tab }: PipelineRunsSidebarOptions) {
  const [groups, setGroups] = useState<RunGroup[]>([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [recentRuns, setRecentRuns] = useState<RunListItem[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [recentHasMore, setRecentHasMore] = useState(true);
  const [recentLoadingMore, setRecentLoadingMore] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Set<number>>(new Set());
  const [expandedBranches, setExpandedBranches] = useState<Set<string>>(new Set());
  const [repoRunsCache, setRepoRunsCache] = useState<Map<number, Record<string, RunListItem[]>>>(new Map());
  const [loadingRepos, setLoadingRepos] = useState<Set<number>>(new Set());
  const groupsRef = useRef(groups);
  const recentRunsRef = useRef(recentRuns);
  const expandedGroupsRef = useRef(expandedGroups);
  const activeGroupIdRef = useRef(activeGroupId);
  const repoRunsCacheRef = useRef(repoRunsCache);
  const loadingReposRef = useRef(loadingRepos);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    groupsRef.current = groups;
    recentRunsRef.current = recentRuns;
    expandedGroupsRef.current = expandedGroups;
    activeGroupIdRef.current = activeGroupId;
  }, [activeGroupId, expandedGroups, groups, recentRuns]);

  const ensureRepoRuns = useCallback(async (groupId: number, options?: { force?: boolean }) => {
    const force = options?.force ?? false;
    if ((!force && repoRunsCacheRef.current.has(groupId)) || loadingReposRef.current.has(groupId)) return;
    setLoadingRepos(previous => {
      const next = new Set(previous);
      next.add(groupId);
      loadingReposRef.current = next;
      return next;
    });
    const data = await fetchRunSidebarRepositoryRuns(groupId);
    setRepoRunsCache(previous => {
      const next = new Map(previous);
      next.set(groupId, data || {});
      repoRunsCacheRef.current = next;
      return next;
    });
    setLoadingRepos(previous => {
      const next = new Set(previous);
      next.delete(groupId);
      loadingReposRef.current = next;
      return next;
    });
  }, []);

  useEffect(() => {
    let cancelled = false;
    const loadGroups = async () => {
      setGroupsLoading(true);
      const data = await fetchRunSidebarGroups();
      if (cancelled) return;
      setGroups(data);
      setGroupsLoading(false);
    };
    void loadGroups();
    return () => {
      cancelled = true;
    };
  }, [tab]);

  useEffect(() => {
    if (tab !== 'recent') return;
    let cancelled = false;
    const loadRecentRuns = async () => {
      setRunsLoading(true);
      setRecentHasMore(true);
      setRecentLoadingMore(false);
      const list = await fetchRunSidebarRecentRuns(0, SIDEBAR_RECENT_PAGE_SIZE);
      if (cancelled) return;
      setRecentRuns(list);
      setRecentHasMore(list.length === SIDEBAR_RECENT_PAGE_SIZE);
      setRunsLoading(false);
    };
    void loadRecentRuns();
    return () => {
      cancelled = true;
    };
  }, [tab]);

  const loadMoreRecentRuns = useCallback(async () => {
    if (tab !== 'recent' || !recentHasMore || recentLoadingMore || runsLoading) return;
    setRecentLoadingMore(true);
    const list = await fetchRunSidebarRecentRuns(recentRuns.length, SIDEBAR_RECENT_PAGE_SIZE);
    setRecentHasMore(list.length === SIDEBAR_RECENT_PAGE_SIZE);
    setRecentRuns(previous => {
      const existing = new Set(previous.map(run => run.run_id));
      return [...previous, ...list.filter(run => !existing.has(run.run_id))];
    });
    setRecentLoadingMore(false);
  }, [recentHasMore, recentLoadingMore, recentRuns.length, runsLoading, tab]);

  const refreshRecentRuns = useCallback(async () => {
    const limit = Math.max(SIDEBAR_RECENT_PAGE_SIZE, recentRunsRef.current.length || 0);
    const data = await fetchRunSidebarRecentRuns(0, limit);
    setRecentRuns(data);
    setRecentHasMore(data.length === limit);
  }, []);

  const refreshVisibleRepoRuns = useCallback(async () => {
    const groupsById = new Map(groupsRef.current.map(group => [group.id, group]));
    const targetGroupIds = new Set<number>();
    const activeGroup = activeGroupIdRef.current !== null ? groupsById.get(activeGroupIdRef.current) : null;
    if (activeGroup && isRunAppGroup(activeGroup)) targetGroupIds.add(activeGroup.id);
    expandedGroupsRef.current.forEach(groupId => {
      const group = groupsById.get(groupId);
      if (group && isRunAppGroup(group)) targetGroupIds.add(groupId);
    });

    const idsToRefresh = Array.from(targetGroupIds).filter(groupId => !loadingReposRef.current.has(groupId));
    const responses = await Promise.all(
      idsToRefresh.map(async groupId => ({
        groupId,
        data: await fetchRunSidebarRepositoryRuns(groupId),
      }))
    );
    setRepoRunsCache(previous => {
      let next: Map<number, Record<string, RunListItem[]>> | null = null;
      responses.forEach(({ groupId, data }) => {
        if (!data) return;
        if (!next) next = new Map(previous);
        next.set(groupId, data);
      });
      if (!next) return previous;
      repoRunsCacheRef.current = next;
      return next;
    });
  }, []);

  useEffect(() => {
    if (!activeGroupId || !groups.length) return;
    const path = buildGroupPath(activeGroupId, groups);
    if (!path.length) return;
    const handle = window.setTimeout(() => {
      setExpandedGroups(previous => {
        const next = new Set(previous);
        path.forEach(group => next.add(group.id));
        return next;
      });
    }, 0);
    return () => window.clearTimeout(handle);
  }, [activeGroupId, groups]);

  useEffect(() => {
    if (tab !== 'main' || !activeGroupId) return;
    const group = groups.find(candidate => candidate.id === activeGroupId);
    if (group && isRunAppGroup(group)) void ensureRepoRuns(group.id, { force: true });
  }, [activeGroupId, ensureRepoRuns, groups, tab]);

  useEffect(() => {
    if (!activeRunId) return;
    let cancelled = false;
    void fetchRunSidebarDetail(activeRunId).then(async detail => {
      if (cancelled) return;
      const info = detail?.run_info;
      if (!info) return;
      const repoName = info.git_repo_owner && info.git_repo_name ? `${info.git_repo_owner}/${info.git_repo_name}` : '';
      const repoGroup = repoName ? groups.find(group => runGroupMatchesRepository(group, repoName)) : null;
      if (!repoGroup) return;
      const path = buildGroupPath(repoGroup.id, groups);
      if (cancelled) return;
      setExpandedGroups(previous => {
        const next = new Set(previous);
        path.forEach(group => next.add(group.id));
        return next;
      });
      await ensureRepoRuns(repoGroup.id, { force: true });
      if (cancelled) return;
      const branchName = formatBranch(info.git_ref);
      if (branchName) {
        setExpandedBranches(previous => new Set(previous).add(`${repoGroup.id}:${branchName}`));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [activeRunId, ensureRepoRuns, groups]);

  useEffect(() => {
    if (pollRef.current) window.clearTimeout(pollRef.current);
    let cancelled = false;
    const tick = async () => {
      if (cancelled) return;
      if (tab === 'recent') await refreshRecentRuns();
      else await refreshVisibleRepoRuns();
      if (!cancelled) pollRef.current = window.setTimeout(tick, document.hidden ? 12000 : 6000);
    };
    pollRef.current = window.setTimeout(tick, document.hidden ? 12000 : 6000);
    return () => {
      cancelled = true;
      if (pollRef.current) window.clearTimeout(pollRef.current);
      pollRef.current = null;
    };
  }, [refreshRecentRuns, refreshVisibleRepoRuns, tab]);

  const toggleGroup = useCallback(
    (group: RunGroup) => {
      const isRepository = isRunAppGroup(group);
      setExpandedGroups(previous => {
        const next = new Set(previous);
        if (next.has(group.id)) next.delete(group.id);
        else {
          next.add(group.id);
          if (isRepository) void ensureRepoRuns(group.id, { force: true });
        }
        return next;
      });
    },
    [ensureRepoRuns]
  );

  const toggleBranch = useCallback((groupId: number, branch: string) => {
    const key = `${groupId}:${branch}`;
    setExpandedBranches(previous => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  return {
    groups,
    groupsLoading,
    recentRuns,
    runsLoading,
    recentHasMore,
    recentLoadingMore,
    expandedGroups,
    expandedBranches,
    repoRunsCache,
    loadingRepos,
    loadMoreRecentRuns,
    toggleGroup,
    toggleBranch,
  };
}
