import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { requestPipelineRunsJson } from '../features/pipeline-runs/api';
import { fetchTeams } from '../features/teams/api';
import type { RunListItem } from '../features/pipeline-runs/contracts';
import {
  findTeamByURLValue,
  normalizeTeamURLValue,
  runMatchesSearch,
  runTimestamp,
  summarizeStatus,
  teamPathForURL,
  type Team,
} from '../features/pipeline-runs/runPresentation';
import {
  filterPipelineRuns,
  normalizeRunSourceFilter,
  normalizeRunStatusFilter,
  type PipelineRunSourceFilter,
  type PipelineRunStatusFilter,
} from '../features/pipeline-runs/overviewModel';
import { PipelineRunsPageView } from '../features/pipeline-runs/PipelineRunsPageView';
import { buildPipelineRunsRoute, extractTeamPathFromRoute } from '../lib/teamRoutes';
import type {
  PipelineApproval,
  PipelineRunDetail as RunDetail,
  PipelineRunsTabKey as TabKey,
  PipelineRunsTriggerTeam as TriggerTeam,
} from '../features/pipeline-runs/pageTypes';

const RECENT_FETCH_SIZE = 60;
const RECENT_INITIAL_BATCH = 30;
const RECENT_BATCH_SIZE = 20;

function PipelineRunsPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { tab: tabParam } = useParams<{ tab?: string }>();
  const [searchParams, setSearchParams] = useSearchParams();

  const activeTab: TabKey = useMemo(() => {
    if (tabParam === 'recent' || tabParam === 'events') return tabParam;
    return 'main';
  }, [tabParam]);

  const [searchTerm, setSearchTerm] = useState(() => (searchParams.get('q') || '').trim());
  const [searchOpen, setSearchOpen] = useState(() => Boolean((searchParams.get('q') || '').trim()));
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() => {
    const fromUrl = searchParams.get('view');
    if (fromUrl === 'list' || fromUrl === 'grid') return fromUrl;
    const stored = typeof window !== 'undefined' ? localStorage.getItem('pipelineruns:view') : null;
    return stored === 'list' ? 'list' : 'grid';
  });

  const routeTeamValue = useMemo(
    () => normalizeTeamURLValue(extractTeamPathFromRoute(location.pathname, 'pipelineruns')),
    [location.pathname]
  );
  const queryTeamValue = useMemo(() => normalizeTeamURLValue(searchParams.get('team')), [searchParams]);
  const activeTeamValue = routeTeamValue || queryTeamValue;

  const activeRunId = searchParams.get('run');
  const sourceFilter = useMemo<PipelineRunSourceFilter>(
    () => normalizeRunSourceFilter(searchParams.get('source')),
    [searchParams]
  );
  const statusFilter = useMemo<PipelineRunStatusFilter>(
    () => normalizeRunStatusFilter(searchParams.get('status')),
    [searchParams]
  );

  const [teams, setTeams] = useState<Team[]>([]);
  const [teamsLoaded, setTeamsLoaded] = useState(false);
  const [teamsLoading, setTeamsLoading] = useState(false);
  const [teamsError, setTeamsError] = useState<string | null>(null);

  const [runsByBranch, setRunsByBranch] = useState<Record<string, RunListItem[]>>({});
  const [recentRunsAll, setRecentRunsAll] = useState<RunListItem[]>([]);
  const [recentVisibleCount, setRecentVisibleCount] = useState(RECENT_INITIAL_BATCH);
  const [recentHasMore, setRecentHasMore] = useState(true);
  const [recentLoadingMore, setRecentLoadingMore] = useState(false);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string | null>(null);
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(new Set());

  const [runDetail, setRunDetail] = useState<RunDetail | null>(null);
  const [runDetailLoading, setRunDetailLoading] = useState(false);
  const [runDetailError, setRunDetailError] = useState<string | null>(null);
  const [approvalDecisionPending, setApprovalDecisionPending] = useState<string | null>(null);
  const [selectedStep, setSelectedStep] = useState<string | null>(null);
  const [definitionOpen, setDefinitionOpen] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const hoverTriggerRef = useRef<string | null>(null);
  const selectedTriggerRef = useRef<string | null>(null);
  const [logsStepFilter, setLogsStepFilter] = useState<string | null>(null);
  const [logsSearchFilter, setLogsSearchFilter] = useState<string | null>(null);
  const [stepDetailName, setStepDetailName] = useState<string | null>(null);
  const [collapsedEvents, setCollapsedEvents] = useState<Set<string>>(new Set());
  const eventCollapseTouchedRef = useRef(false);

  const pollingRef = useRef<number | null>(null);
  const detailPollRef = useRef<number | null>(null);
  const mainContentRef = useRef<HTMLDivElement | null>(null);
  const pageWrapperRef = useRef<HTMLElement | null>(null);
  const scrollMainToTop = useCallback(() => {
    const el = mainContentRef.current;
    if (el) el.scrollTo({ top: 0, behavior: 'smooth' });
  }, []);

  const applyTriggerClass = useCallback((triggerId: string | null, className: string, add: boolean) => {
    if (!triggerId) return;
    const selector =
      typeof CSS !== 'undefined' && typeof CSS.escape === 'function'
        ? `[data-trigger-id="${CSS.escape(triggerId)}"]`
        : `[data-trigger-id="${triggerId.replace(/"/g, '\\"')}"]`;
    document.querySelectorAll(selector).forEach(el => el.classList.toggle(className, add));
  }, []);

  const clearTriggerHighlights = useCallback(() => {
    if (hoverTriggerRef.current) {
      applyTriggerClass(hoverTriggerRef.current, 'trigger-hover', false);
      hoverTriggerRef.current = null;
    }
    if (selectedTriggerRef.current) {
      applyTriggerClass(selectedTriggerRef.current, 'trigger-selected', false);
      selectedTriggerRef.current = null;
    }
  }, [applyTriggerClass]);

  useEffect(() => {
    setSearchTerm((searchParams.get('q') || '').trim());
    setSearchOpen(Boolean((searchParams.get('q') || '').trim()));
  }, [searchParams]);

  useEffect(() => {
    scrollMainToTop();
  }, [scrollMainToTop, activeTab]);

  useEffect(() => {
    // Reset step selection when run detail changes to avoid showing task graphs by default.
    setSelectedStep(null);
  }, [runDetail?.run_info.run_id]);

  useEffect(() => {
    const handleMove = (event: MouseEvent) => {
      const target = (event.target as HTMLElement | null)?.closest('[data-trigger-id]') as HTMLElement | null;
      const triggerId = target?.getAttribute('data-trigger-id') || null;
      if (hoverTriggerRef.current && hoverTriggerRef.current !== triggerId) {
        applyTriggerClass(hoverTriggerRef.current, 'trigger-hover', false);
      }
      if (triggerId) {
        hoverTriggerRef.current = triggerId;
        applyTriggerClass(triggerId, 'trigger-hover', true);
      } else {
        hoverTriggerRef.current = null;
      }
    };

    const handleLeave = () => {
      if (hoverTriggerRef.current) {
        applyTriggerClass(hoverTriggerRef.current, 'trigger-hover', false);
        hoverTriggerRef.current = null;
      }
    };

    document.addEventListener('mousemove', handleMove);
    document.addEventListener('mouseleave', handleLeave);
    return () => {
      document.removeEventListener('mousemove', handleMove);
      document.removeEventListener('mouseleave', handleLeave);
    };
  }, [applyTriggerClass]);

  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    try {
      localStorage.setItem('pipelineruns:view', viewMode);
    } catch {
      // ignore
    }
    const params = new URLSearchParams(searchParams);
    params.set('view', viewMode);
    setSearchParams(params, { replace: true, preventScrollReset: true });
  }, [viewMode, searchParams, setSearchParams]);

  const updateSearchParams = useCallback(
    (updates: Record<string, string | number | null | undefined>) => {
      const params = new URLSearchParams(searchParams);
      Object.entries(updates).forEach(([key, value]) => {
        if (value === null || value === undefined || value === '') {
          params.delete(key);
        } else {
          params.set(key, String(value));
        }
      });
      params.delete('team');
      setSearchParams(params, { replace: true, preventScrollReset: true });
    },
    [searchParams, setSearchParams]
  );

  const handleSourceFilterChange = useCallback(
    (filter: PipelineRunSourceFilter) => {
      updateSearchParams({ source: filter === 'all' ? null : filter });
      setSelectedRunIds(new Set());
    },
    [updateSearchParams]
  );

  const handleStatusFilterChange = useCallback(
    (filter: PipelineRunStatusFilter) => {
      updateSearchParams({ status: filter === 'all' ? null : filter });
      setSelectedRunIds(new Set());
    },
    [updateSearchParams]
  );

  const fetchJson = useCallback(
    async <T,>(path: string, options?: RequestInit): Promise<T> => requestPipelineRunsJson<T>(path, options),
    []
  );

  useLayoutEffect(() => {
    // Capture the scrollable wrapper used by the app layout so we can attach listeners there too.
    pageWrapperRef.current = (mainContentRef.current?.closest('#page-content-wrapper') as HTMLElement | null) ?? null;
  }, []);

  const fetchRecentPage = useCallback(
    async (offset: number, { replace }: { replace: boolean }) => {
      if (replace) {
        setRunsLoading(true);
        setRunsError(null);
      } else {
        setRecentLoadingMore(true);
      }
      try {
        const data = await fetchJson<RunListItem[]>(`/v1/runs?offset=${offset}&limit=${RECENT_FETCH_SIZE}`);
        const list = Array.isArray(data) ? data : [];
        setRecentHasMore(list.length === RECENT_FETCH_SIZE);

        let nextLength = 0;
        setRecentRunsAll(prev => {
          if (replace) {
            const newIds = new Set(list.map(item => item.run_id));
            const merged = [...list, ...prev.filter(item => !newIds.has(item.run_id))];
            nextLength = merged.length;
            return merged;
          }
          const existingIds = new Set(prev.map(item => item.run_id));
          const appended = list.filter(item => !existingIds.has(item.run_id));
          const merged = [...prev, ...appended];
          nextLength = merged.length;
          return merged;
        });

        if (replace) {
          setRecentVisibleCount(prev => Math.min(Math.max(RECENT_INITIAL_BATCH, prev), nextLength || RECENT_INITIAL_BATCH));
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to load pipeline runs';
        setRunsError(message);
      } finally {
        if (replace) {
          setRunsLoading(false);
        } else {
          setRecentLoadingMore(false);
        }
      }
    },
    [fetchJson]
  );

  const loadTeams = useCallback(async () => {
    setTeamsLoaded(false);
    setTeamsLoading(true);
    setTeamsError(null);
    try {
      const payload = await fetchTeams();
      setTeams(Array.isArray(payload) ? payload : []);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load teams';
      setTeamsError(message);
    } finally {
      setTeamsLoaded(true);
      setTeamsLoading(false);
    }
  }, []);

  const activeTeam = useMemo(() => findTeamByURLValue(activeTeamValue, teams), [activeTeamValue, teams]);
  const activeTeamId = activeTeam?.id ?? null;
  const activeTeamURLValue = useMemo(
    () => (activeTeam ? teamPathForURL(activeTeam, teams) : ''),
    [activeTeam, teams]
  );

  useEffect(() => {
    if ((!routeTeamValue && !queryTeamValue) || !teamsLoaded) return;
    const params = new URLSearchParams(searchParams);
    params.delete('team');
    if (activeTeamURLValue) {
      if (!queryTeamValue && routeTeamValue === activeTeamURLValue) return;
      const search = params.toString();
      navigate(`${buildPipelineRunsRoute(activeTab, activeTeamURLValue)}${search ? `?${search}` : ''}`, { replace: true, preventScrollReset: true });
    } else {
      const search = params.toString();
      navigate(`/pipelineruns/${activeTab}${search ? `?${search}` : ''}`, { replace: true, preventScrollReset: true });
    }
  }, [activeTab, activeTeamURLValue, navigate, queryTeamValue, routeTeamValue, searchParams, teamsLoaded]);

  const loadRuns = useCallback(async () => {
    if (activeTab === 'main' && activeTeamValue && !teamsLoaded) return;
    setRunsLoading(true);
    setRunsError(null);
    try {
      if (activeTab === 'main' && activeTeamId) {
        const data = await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?teamId=${activeTeamId}`);
        setRunsByBranch(data || {});
      } else if (activeTab === 'main') {
        setRunsByBranch({});
        await fetchRecentPage(0, { replace: true });
      } else {
        await fetchRecentPage(0, { replace: true });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load pipeline runs';
      setRunsError(message);
    } finally {
      setRunsLoading(false);
    }
  }, [activeTeamId, activeTab, activeTeamValue, fetchJson, fetchRecentPage, teamsLoaded]);

  const loadRunDetail = useCallback(async () => {
    if (!activeRunId) {
      setRunDetail(null);
      setRunDetailError(null);
      return;
    }
    setRunDetailLoading(true);
    setRunDetailError(null);
    try {
      const encodedRunID = encodeURIComponent(activeRunId);
      const [detail, approvals] = await Promise.all([
        fetchJson<RunDetail>(`/v1/runs/${encodedRunID}`),
        fetchJson<PipelineApproval[]>(`/v1/runs/${encodedRunID}/approvals`),
      ]);
      setRunDetail({ ...detail, approvals: approvals || [] });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load run details';
      setRunDetailError(message);
    } finally {
      setRunDetailLoading(false);
    }
  }, [activeRunId, fetchJson]);


  useEffect(() => {
    void loadTeams();
  }, [loadTeams]);

  useEffect(() => {
    if (pollingRef.current) {
      window.clearTimeout(pollingRef.current);
      pollingRef.current = null;
    }
    let cancelled = false;
    const tick = async () => {
      if (cancelled) return;
      await loadRuns();
      const interval = document.hidden ? 12000 : 6000;
      pollingRef.current = window.setTimeout(tick, interval);
    };
    void tick();
    return () => {
      cancelled = true;
      if (pollingRef.current) {
        window.clearTimeout(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [activeTeamId, activeTab, loadRuns, searchTerm]);

  useEffect(() => {
    if (detailPollRef.current) {
      window.clearTimeout(detailPollRef.current);
      detailPollRef.current = null;
    }
    if (!activeRunId) return undefined;
    let cancelled = false;
    const tick = async () => {
      if (cancelled) return;
      await loadRunDetail();
      const interval = document.hidden ? 12000 : 7000;
      detailPollRef.current = window.setTimeout(tick, interval);
    };
    void tick();
    return () => {
      cancelled = true;
      if (detailPollRef.current) {
        window.clearTimeout(detailPollRef.current);
        detailPollRef.current = null;
      }
    };
  }, [activeRunId, loadRunDetail]);

  const teamedEvents = useMemo<TriggerTeam[]>(() => {
    if (activeTab !== 'events') return [];
    const term = searchTerm.trim().toLowerCase();
    const runs = !term ? recentRunsAll : recentRunsAll.filter(run => runMatchesSearch(run, term));
    const bucket = new Map<string, RunListItem[]>();
    runs.forEach(run => {
      const key = run.trigger_event_id || run.run_id || 'unknown';
      const list = bucket.get(key) || [];
      list.push(run);
      bucket.set(key, list);
    });
    return Array.from(bucket.entries()).map(([id, runs]) => ({
      id,
      runs,
      status: summarizeStatus(runs),
      latestRun: [...runs].sort((left, right) => runTimestamp(right) - runTimestamp(left))[0],
    }));
  }, [activeTab, recentRunsAll, searchTerm]);

  const expandAllEvents = useCallback(() => {
    eventCollapseTouchedRef.current = true;
    setCollapsedEvents(new Set());
  }, []);

  const collapseAllEvents = useCallback(() => {
    eventCollapseTouchedRef.current = true;
    const next = new Set<string>();
    teamedEvents.forEach(team => next.add(team.id));
    setCollapsedEvents(next);
  }, [teamedEvents]);

  useEffect(() => {
    if (activeTab !== 'events') {
      eventCollapseTouchedRef.current = false;
      setCollapsedEvents(prev => (prev.size ? new Set() : prev));
      return;
    }
    if (eventCollapseTouchedRef.current) return;
    const next = new Set(teamedEvents.map(team => team.id));
    setCollapsedEvents(prev => (setsEqual(prev, next) ? prev : next));
  }, [activeTab, teamedEvents]);

  const effectiveCollapsedEvents = useMemo(() => {
    if (activeTab === 'events' && !eventCollapseTouchedRef.current) {
      return new Set(teamedEvents.map(team => team.id));
    }
    return collapsedEvents;
  }, [activeTab, collapsedEvents, teamedEvents]);

  const recentFilteredTotal = useMemo(() => {
    if (activeTab !== 'recent') return 0;
    return filterPipelineRuns(recentRunsAll, { searchTerm, sourceFilter, statusFilter }).length;
  }, [activeTab, recentRunsAll, searchTerm, sourceFilter, statusFilter]);

  const handleRecentScroll = useCallback((source?: HTMLElement | null) => {
    if (activeTab !== 'recent') return;
    const node =
      source ||
      mainContentRef.current ||
      pageWrapperRef.current ||
      (document.getElementById('page-content-wrapper') as HTMLElement | null) ||
      document.scrollingElement ||
      document.documentElement;
    if (!node) return;
    const remaining = node.scrollHeight - node.scrollTop - node.clientHeight;
    if (remaining > 240) return;
    setRecentVisibleCount(prev => {
      const limit = recentFilteredTotal || recentRunsAll.length;
      if (prev >= limit) return prev;
      return Math.min(prev + RECENT_BATCH_SIZE, limit || prev + RECENT_BATCH_SIZE);
    });
    if (!searchTerm.trim() && recentHasMore && !recentLoadingMore && recentVisibleCount >= recentRunsAll.length - RECENT_BATCH_SIZE) {
      void fetchRecentPage(recentRunsAll.length, { replace: false });
    }
  }, [activeTab, fetchRecentPage, recentFilteredTotal, recentHasMore, recentLoadingMore, recentRunsAll.length, recentVisibleCount, searchTerm]);

  useEffect(() => {
    if (activeTab !== 'recent') return;
    const listeners: Array<{ node: HTMLElement; handler: () => void }> = [];

    const mainNode = mainContentRef.current;
    if (mainNode) {
      const handler = () => handleRecentScroll(mainNode);
      mainNode.addEventListener('scroll', handler, { passive: true });
      listeners.push({ node: mainNode, handler });
    }

    const wrapperNode = pageWrapperRef.current;
    if (wrapperNode && wrapperNode !== mainNode) {
      const handler = () => handleRecentScroll(wrapperNode);
      wrapperNode.addEventListener('scroll', handler, { passive: true });
      listeners.push({ node: wrapperNode, handler });
    }

    const windowHandler = () => handleRecentScroll(document.documentElement);
    window.addEventListener('scroll', windowHandler, { passive: true });

    return () => {
      listeners.forEach(({ node, handler }) => node.removeEventListener('scroll', handler));
      window.removeEventListener('scroll', windowHandler);
    };
  }, [activeTab, handleRecentScroll]);

  useEffect(() => {
    if (activeTab === 'recent') {
      setRecentVisibleCount(RECENT_INITIAL_BATCH);
    }
    scrollMainToTop();
  }, [activeTab, scrollMainToTop]);

  useEffect(() => {
    if (activeTab === 'recent') {
      setRecentVisibleCount(RECENT_INITIAL_BATCH);
    }
  }, [activeTab, searchTerm, sourceFilter, statusFilter]);

  const toggleEventTeam = useCallback((id: string) => {
    eventCollapseTouchedRef.current = true;
    setCollapsedEvents(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const filteredRecentRuns = useMemo(() => {
    if (activeTab === 'events') return [];
    const base = filterPipelineRuns(recentRunsAll, { searchTerm, sourceFilter, statusFilter });
    return base.slice(0, recentVisibleCount);
  }, [activeTab, recentRunsAll, recentVisibleCount, searchTerm, sourceFilter, statusFilter]);

  useEffect(() => {
    const triggerId = activeRunId ? runDetail?.run_info?.trigger_event_id || null : null;
    if (!triggerId) {
      clearTriggerHighlights();
      return;
    }
    if (selectedTriggerRef.current && selectedTriggerRef.current !== triggerId) {
      applyTriggerClass(selectedTriggerRef.current, 'trigger-selected', false);
    }
    selectedTriggerRef.current = triggerId;
    applyTriggerClass(triggerId, 'trigger-selected', true);
  }, [activeRunId, applyTriggerClass, clearTriggerHighlights, runDetail]);

  const handleRunSelect = useCallback(
    (runId: string) => {
      setSelectedRunIds(prev => {
        const next = new Set(prev);
        if (next.has(runId)) {
          next.delete(runId);
        } else {
          next.add(runId);
        }
        return next;
      });
    },
    []
  );

  const clearSelection = useCallback(() => setSelectedRunIds(new Set()), []);

  const handleOpenRun = useCallback(
    (runId: string) => {
      updateSearchParams({ run: runId });
      setSelectedStep(null);
      setDefinitionOpen(false);
      setLogsStepFilter(null);
      scrollMainToTop();
    },
    [scrollMainToTop, updateSearchParams]
  );

  const handleCloseDetail = useCallback(() => {
    updateSearchParams({ run: null });
    setRunDetail(null);
    setSelectedStep(null);
    setDefinitionOpen(false);
    setLogsStepFilter(null);
  }, [updateSearchParams]);

  const handleDeleteRun = useCallback(
    async (runId: string) => {
      if (!window.confirm('Delete this pipeline run?')) return;
      try {
        await fetchJson<void>(`/v1/runs/${encodeURIComponent(runId)}`, { method: 'DELETE' });
        if (activeRunId === runId) handleCloseDetail();
        clearSelection();
        await loadRuns();
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to delete run';
        alert(message);
      }
    },
    [activeRunId, clearSelection, fetchJson, handleCloseDetail, loadRuns]
  );

  const handleBulkDelete = useCallback(async () => {
    if (!selectedRunIds.size) return;
    if (!window.confirm(`Delete ${selectedRunIds.size} selected runs?`)) return;
    try {
      await Promise.all(
        Array.from(selectedRunIds).map(id => fetchJson<void>(`/v1/runs/${encodeURIComponent(id)}`, { method: 'DELETE' }).catch(() => null))
      );
      clearSelection();
      await loadRuns();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete runs';
      alert(message);
    }
  }, [clearSelection, fetchJson, loadRuns, selectedRunIds]);

  const handleCancelRun = useCallback(
    async (runId: string) => {
      try {
        await fetchJson(`/v1/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST' });
        await loadRunDetail();
        await loadRuns();
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to cancel run';
        alert(message);
      }
    },
    [fetchJson, loadRunDetail, loadRuns]
  );

  const handleApprovalDecision = useCallback(
    async (approval: PipelineApproval, decision: 'approve' | 'reject') => {
      const runId = approval.run_id || runDetail?.run_info.run_id;
      if (!runId || approvalDecisionPending) return;
      const key = `${approval.id}:${decision}`;
      setApprovalDecisionPending(key);
      try {
        await fetchJson(`/v1/runs/${encodeURIComponent(runId)}/approvals/${encodeURIComponent(approval.id)}/${decision}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({}),
        });
        await loadRunDetail();
        await loadRuns();
      } catch (error) {
        const message = error instanceof Error ? error.message : `Unable to ${decision} approval`;
        alert(message);
      } finally {
        setApprovalDecisionPending(null);
      }
    },
    [approvalDecisionPending, fetchJson, loadRunDetail, loadRuns, runDetail?.run_info.run_id]
  );

  const handleRerun = useCallback(
    async (runId: string) => {
      try {
        const result = await fetchJson<{ runId: string; triggerEventId?: string }>(
          `/v1/runs/${encodeURIComponent(runId)}/rerun`,
          { method: 'POST' }
        );
        if (result?.runId) {
          updateSearchParams({ run: result.runId });
        }
        await loadRuns();
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to rerun pipeline';
        alert(message);
      }
    },
    [fetchJson, loadRuns, updateSearchParams]
  );

  const onSelectTeam = useCallback(
    (teamId: number | null) => {
      const team = teamId == null ? null : teams.find(item => item.id === teamId) || null;
      const params = new URLSearchParams(searchParams);
      params.delete('team');
      params.delete('run');
      const teamRoute = buildPipelineRunsRoute(activeTab, team ? teamPathForURL(team, teams) : '');
      const search = params.toString();
      navigate(`${teamRoute}${search ? `?${search}` : ''}`, { replace: true, preventScrollReset: true });
      setSelectedRunIds(new Set());
      setRunDetail(null);
    },
    [activeTab, navigate, searchParams, teams]
  );

  const isViewingDetail = Boolean(runDetail && activeRunId);
  const showSelectionBar = selectedRunIds.size > 0;

  return (
    <PipelineRunsPageView
      activeTab={activeTab}
      activeTeamId={activeTeamId}
      activeTeamURLValue={activeTeamURLValue}
      activeRunId={activeRunId}
      searchTerm={searchTerm}
      searchOpen={searchOpen}
      searchInputRef={searchInputRef}
      setSearchTerm={setSearchTerm}
      setSearchOpen={setSearchOpen}
      updateSearchParams={updateSearchParams}
      viewMode={viewMode}
      setViewMode={setViewMode}
      sourceFilter={sourceFilter}
      statusFilter={statusFilter}
      onSourceFilterChange={handleSourceFilterChange}
      onStatusFilterChange={handleStatusFilterChange}
      mainContentRef={mainContentRef}
      isViewingDetail={isViewingDetail}
      showSelectionBar={showSelectionBar}
      selectedRunIds={selectedRunIds}
      clearSelection={clearSelection}
      handleBulkDelete={handleBulkDelete}
      teams={teams}
      teamsLoading={teamsLoading}
      teamsError={teamsError}
      runsByBranch={runsByBranch}
      filteredRecentRuns={filteredRecentRuns}
      teamedEvents={teamedEvents}
      runsLoading={runsLoading}
      runsError={runsError}
      onSelectTeam={onSelectTeam}
      handleOpenRun={handleOpenRun}
      handleRunSelect={handleRunSelect}
      collapsedEvents={effectiveCollapsedEvents}
      toggleEventTeam={toggleEventTeam}
      collapseAllEvents={collapseAllEvents}
      expandAllEvents={expandAllEvents}
      runDetail={runDetail}
      runDetailLoading={runDetailLoading}
      runDetailError={runDetailError}
      handleCloseDetail={handleCloseDetail}
      handleCancelRun={handleCancelRun}
      handleRerun={handleRerun}
      handleDeleteRun={handleDeleteRun}
      selectedStep={selectedStep}
      setSelectedStep={setSelectedStep}
      setLogsOpen={setLogsOpen}
      setLogsStepFilter={setLogsStepFilter}
      setLogsSearchFilter={setLogsSearchFilter}
      setStepDetailName={setStepDetailName}
      setDefinitionOpen={setDefinitionOpen}
      handleApprovalDecision={handleApprovalDecision}
      approvalDecisionPending={approvalDecisionPending}
      definitionOpen={definitionOpen}
      logsOpen={logsOpen}
      logsStepFilter={logsStepFilter}
      logsSearchFilter={logsSearchFilter}
      stepDetailName={stepDetailName}
    />
  );
}

export default PipelineRunsPage;

function setsEqual(left: Set<string>, right: Set<string>) {
  if (left.size !== right.size) return false;
  for (const value of left) {
    if (!right.has(value)) return false;
  }
  return true;
}
