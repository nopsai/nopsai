import { useCallback, useEffect, useRef, useState } from 'react';
import { SIDEBAR_RECENT_PAGE_SIZE } from './constants';
import {
  fetchRunSidebarDetail,
  fetchRunSidebarTeams,
  fetchRunSidebarRecentRuns,
  fetchRunSidebarRepositoryRuns,
} from './runSidebarApi';
import { buildTeamPath, formatBranch, isRunAppTeam, runTeamMatchesRepository } from './runSidebarUtils';
import type { RunTeam, RunListItem, RunTabKey } from './types';

type PipelineRunsSidebarOptions = {
  activeTeamId: number | null;
  activeRunId: string | null;
  tab: RunTabKey;
};

export function usePipelineRunsSidebar({ activeTeamId, activeRunId, tab }: PipelineRunsSidebarOptions) {
  const [teams, setTeams] = useState<RunTeam[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(false);
  const [recentRuns, setRecentRuns] = useState<RunListItem[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [recentHasMore, setRecentHasMore] = useState(true);
  const [recentLoadingMore, setRecentLoadingMore] = useState(false);
  const [expandedTeams, setExpandedTeams] = useState<Set<number>>(new Set());
  const [expandedBranches, setExpandedBranches] = useState<Set<string>>(new Set());
  const [repoRunsCache, setRepoRunsCache] = useState<Map<number, Record<string, RunListItem[]>>>(new Map());
  const [loadingRepos, setLoadingRepos] = useState<Set<number>>(new Set());
  const teamsRef = useRef(teams);
  const recentRunsRef = useRef(recentRuns);
  const expandedTeamsRef = useRef(expandedTeams);
  const activeTeamIdRef = useRef(activeTeamId);
  const repoRunsCacheRef = useRef(repoRunsCache);
  const loadingReposRef = useRef(loadingRepos);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    teamsRef.current = teams;
    recentRunsRef.current = recentRuns;
    expandedTeamsRef.current = expandedTeams;
    activeTeamIdRef.current = activeTeamId;
  }, [activeTeamId, expandedTeams, teams, recentRuns]);

  const ensureRepoRuns = useCallback(async (teamId: number, options?: { force?: boolean }) => {
    const force = options?.force ?? false;
    if ((!force && repoRunsCacheRef.current.has(teamId)) || loadingReposRef.current.has(teamId)) return;
    setLoadingRepos(previous => {
      const next = new Set(previous);
      next.add(teamId);
      loadingReposRef.current = next;
      return next;
    });
    const data = await fetchRunSidebarRepositoryRuns(teamId);
    setRepoRunsCache(previous => {
      const next = new Map(previous);
      next.set(teamId, data || {});
      repoRunsCacheRef.current = next;
      return next;
    });
    setLoadingRepos(previous => {
      const next = new Set(previous);
      next.delete(teamId);
      loadingReposRef.current = next;
      return next;
    });
  }, []);

  useEffect(() => {
    let cancelled = false;
    const loadTeams = async () => {
      setTeamsLoading(true);
      const data = await fetchRunSidebarTeams();
      if (cancelled) return;
      setTeams(data);
      setTeamsLoading(false);
    };
    void loadTeams();
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
    const teamsById = new Map(teamsRef.current.map(team => [team.id, team]));
    const targetTeamIds = new Set<number>();
    const activeTeam = activeTeamIdRef.current !== null ? teamsById.get(activeTeamIdRef.current) : null;
    if (activeTeam && isRunAppTeam(activeTeam)) targetTeamIds.add(activeTeam.id);
    expandedTeamsRef.current.forEach(teamId => {
      const team = teamsById.get(teamId);
      if (team && isRunAppTeam(team)) targetTeamIds.add(teamId);
    });

    const idsToRefresh = Array.from(targetTeamIds).filter(teamId => !loadingReposRef.current.has(teamId));
    const responses = await Promise.all(
      idsToRefresh.map(async teamId => ({
        teamId,
        data: await fetchRunSidebarRepositoryRuns(teamId),
      }))
    );
    setRepoRunsCache(previous => {
      let next: Map<number, Record<string, RunListItem[]>> | null = null;
      responses.forEach(({ teamId, data }) => {
        if (!data) return;
        if (!next) next = new Map(previous);
        next.set(teamId, data);
      });
      if (!next) return previous;
      repoRunsCacheRef.current = next;
      return next;
    });
  }, []);

  useEffect(() => {
    if (!activeTeamId || !teams.length) return;
    const path = buildTeamPath(activeTeamId, teams);
    if (!path.length) return;
    const handle = window.setTimeout(() => {
      setExpandedTeams(previous => {
        const next = new Set(previous);
        path.forEach(team => next.add(team.id));
        return next;
      });
    }, 0);
    return () => window.clearTimeout(handle);
  }, [activeTeamId, teams]);

  useEffect(() => {
    if (tab !== 'main' || !activeTeamId) return;
    const team = teams.find(candidate => candidate.id === activeTeamId);
    if (team && isRunAppTeam(team)) void ensureRepoRuns(team.id, { force: true });
  }, [activeTeamId, ensureRepoRuns, teams, tab]);

  useEffect(() => {
    if (!activeRunId) return;
    let cancelled = false;
    void fetchRunSidebarDetail(activeRunId).then(async detail => {
      if (cancelled) return;
      const info = detail?.run_info;
      if (!info) return;
      const repoName = info.git_repo_owner && info.git_repo_name ? `${info.git_repo_owner}/${info.git_repo_name}` : '';
      const repoTeam = repoName ? teams.find(team => runTeamMatchesRepository(team, repoName)) : null;
      if (!repoTeam) return;
      const path = buildTeamPath(repoTeam.id, teams);
      if (cancelled) return;
      setExpandedTeams(previous => {
        const next = new Set(previous);
        path.forEach(team => next.add(team.id));
        return next;
      });
      await ensureRepoRuns(repoTeam.id, { force: true });
      if (cancelled) return;
      const branchName = formatBranch(info.git_ref);
      if (branchName) {
        setExpandedBranches(previous => new Set(previous).add(`${repoTeam.id}:${branchName}`));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [activeRunId, ensureRepoRuns, teams]);

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

  const toggleTeam = useCallback(
    (team: RunTeam) => {
      const isRepository = isRunAppTeam(team);
      setExpandedTeams(previous => {
        const next = new Set(previous);
        if (next.has(team.id)) next.delete(team.id);
        else {
          next.add(team.id);
          if (isRepository) void ensureRepoRuns(team.id, { force: true });
        }
        return next;
      });
    },
    [ensureRepoRuns]
  );

  const toggleBranch = useCallback((teamId: number, branch: string) => {
    const key = `${teamId}:${branch}`;
    setExpandedBranches(previous => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  return {
    teams,
    teamsLoading,
    recentRuns,
    runsLoading,
    recentHasMore,
    recentLoadingMore,
    expandedTeams,
    expandedBranches,
    repoRunsCache,
    loadingRepos,
    loadMoreRecentRuns,
    toggleTeam,
    toggleBranch,
  };
}
