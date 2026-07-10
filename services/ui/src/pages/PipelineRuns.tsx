import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { requestPipelineRunsJson } from '../features/pipeline-runs/api';
import type { RunListItem } from '../features/pipeline-runs/contracts';
import {
  buildGroupPath,
  extractLatestRunSummary,
  formatBranch,
  runMatchesSearch,
  summarizeStatus,
  type Group,
  type RepoSummary,
} from '../features/pipeline-runs/runPresentation';
import { PipelineRunsPageView } from '../features/pipeline-runs/PipelineRunsPageView';
import type {
  PipelineApproval,
  PipelineRunDetail as RunDetail,
  PipelineRunsTabKey as TabKey,
  PipelineRunsTriggerGroup as TriggerGroup,
} from '../features/pipeline-runs/pageTypes';

const RECENT_FETCH_SIZE = 60;
const RECENT_INITIAL_BATCH = 30;
const RECENT_BATCH_SIZE = 20;

function PipelineRunsPage() {
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

  const activeGroupId = useMemo(() => {
    const raw = searchParams.get('group');
    if (!raw) return null;
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? parsed : null;
  }, [searchParams]);

  const activeRunId = searchParams.get('run');

  const [groups, setGroups] = useState<Group[]>([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);

  const [runsByBranch, setRunsByBranch] = useState<Record<string, RunListItem[]>>({});
  const [recentRunsAll, setRecentRunsAll] = useState<RunListItem[]>([]);
  const [recentVisibleCount, setRecentVisibleCount] = useState(RECENT_INITIAL_BATCH);
  const [recentHasMore, setRecentHasMore] = useState(true);
  const [recentLoadingMore, setRecentLoadingMore] = useState(false);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState<string | null>(null);
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(new Set());
  const [repoSummaries, setRepoSummaries] = useState<Map<number, RepoSummary>>(new Map());

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
  const [collapsedBranches, setCollapsedBranches] = useState<Set<string>>(new Set());
  const collapsedInitRef = useRef(false);

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
    setSearchParams(params, { replace: true });
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
      setSearchParams(params, { replace: true });
    },
    [searchParams, setSearchParams]
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

  const loadGroups = useCallback(async () => {
    setGroupsLoading(true);
    setGroupsError(null);
    try {
      const payload = await fetchJson<Group[]>('/v1/groups');
      setGroups(Array.isArray(payload) ? payload : []);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load groups';
      setGroupsError(message);
    } finally {
      setGroupsLoading(false);
    }
  }, [fetchJson]);

  const loadRuns = useCallback(async () => {
    setRunsLoading(true);
    setRunsError(null);
    try {
      const hasSearch = Boolean(searchTerm.trim());
      if (activeTab === 'main' && activeGroupId) {
        const data = await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${activeGroupId}`);
        setRunsByBranch(data || {});
      } else if (activeTab === 'main' && hasSearch) {
        setRunsByBranch({});
        await fetchRecentPage(0, { replace: true });
      } else if (activeTab === 'main') {
        const data = await fetchJson<Record<string, RunListItem[]>>('/v1/runs?groupId=root');
        setRunsByBranch(data || {});
        setRecentRunsAll([]);
      } else {
        await fetchRecentPage(0, { replace: true });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load pipeline runs';
      setRunsError(message);
    } finally {
      setRunsLoading(false);
    }
  }, [activeGroupId, activeTab, fetchJson, fetchRecentPage, searchTerm]);

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
    void loadGroups();
  }, [loadGroups]);

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
  }, [activeGroupId, activeTab, loadRuns, searchTerm]);

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

  const groupedEvents = useMemo<TriggerGroup[]>(() => {
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
      latestRun: runs.find(r => r.started_at) || runs[0],
    }));
  }, [activeTab, recentRunsAll, searchTerm]);

  const expandAllEvents = useCallback(() => setCollapsedEvents(new Set()), []);

  const collapseAllEvents = useCallback(() => {
    const next = new Set<string>();
    groupedEvents.forEach(group => next.add(group.id));
    setCollapsedEvents(next);
  }, [groupedEvents]);

  const toggleBranchCollapse = useCallback((branch: string, scrollIntoView = false) => {
    setCollapsedBranches(prev => {
      const next = new Set(prev);
      if (next.has(branch)) {
        next.delete(branch);
      } else {
        next.add(branch);
      }
      return next;
    });
    if (scrollIntoView) {
      requestAnimationFrame(() => {
        const selector = `[data-branch-row="${(window.CSS && CSS.escape ? CSS.escape(branch) : branch).replace(/"/g, '')}"]`;
        const el = document.querySelector(selector);
        if (el && 'scrollIntoView' in el) {
          (el as HTMLElement).scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
      });
    }
  }, []);

  const recentFilteredTotal = useMemo(() => {
    if (activeTab !== 'recent') return 0;
    const term = searchTerm.trim().toLowerCase();
    return (!term ? recentRunsAll : recentRunsAll.filter(run => runMatchesSearch(run, term))).length;
  }, [activeTab, recentRunsAll, searchTerm]);

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
  }, [activeTab, scrollMainToTop, searchTerm]);

  const toggleEventGroup = useCallback((id: string) => {
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
    const term = searchTerm.trim().toLowerCase();
    const base = !term ? recentRunsAll : recentRunsAll.filter(run => runMatchesSearch(run, term));
    return base.slice(0, recentVisibleCount);
  }, [activeTab, recentRunsAll, recentVisibleCount, searchTerm]);

  const activeGroupPath = useMemo(() => buildGroupPath(activeGroupId, groups), [activeGroupId, groups]);

  useEffect(() => {
    // reset collapse state when switching tabs/groups
    collapsedInitRef.current = false;
    setCollapsedBranches(new Set());
  }, [activeTab, activeGroupId]);

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

  useEffect(() => {
    if (activeTab !== 'main') return;
    if (collapsedInitRef.current) return;
    const branches = Object.keys(runsByBranch);
    if (!branches.length) return;
    collapsedInitRef.current = true;
    setCollapsedBranches(new Set(branches));
  }, [activeTab, runsByBranch]);

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

  const handleDeleteBranch = useCallback(
    async (branch: string) => {
      const runs = runsByBranch[branch] || [];
      const label = formatBranch(branch);
      if (!runs.length) return;
      if (!window.confirm(`Delete all runs for branch "${label || branch}"?`)) return;
      try {
        await Promise.all(
          runs.map(run => fetchJson<void>(`/v1/runs/${encodeURIComponent(run.run_id)}`, { method: 'DELETE' }).catch(() => null))
        );
        setSelectedRunIds(new Set());
        await loadRuns();
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to delete branch runs';
        alert(message);
      }
    },
    [fetchJson, loadRuns, runsByBranch]
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

  const fetchRepoSummary = useCallback(
    async (groupId: number) => {
      const runsForRepo = await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${groupId}`);
      const summary = extractLatestRunSummary(runsForRepo);
      if (!summary) return;
      setRepoSummaries(prev => {
        if (prev.has(groupId)) return prev;
        const next = new Map(prev);
        next.set(groupId, summary);
        return next;
      });
    },
    [fetchJson]
  );

  const onSelectGroup = useCallback(
    (groupId: number | null) => {
      updateSearchParams({ group: groupId, run: null });
      setSelectedRunIds(new Set());
      setRunDetail(null);
      scrollMainToTop();
    },
    [scrollMainToTop, updateSearchParams]
  );

  const isViewingDetail = Boolean(runDetail && activeRunId);
  const showSelectionBar = selectedRunIds.size > 0;

  return (
    <PipelineRunsPageView
      activeTab={activeTab}
      activeGroupId={activeGroupId}
      activeGroupPath={activeGroupPath}
      activeRunId={activeRunId}
      searchTerm={searchTerm}
      searchOpen={searchOpen}
      searchInputRef={searchInputRef}
      setSearchTerm={setSearchTerm}
      setSearchOpen={setSearchOpen}
      updateSearchParams={updateSearchParams}
      viewMode={viewMode}
      setViewMode={setViewMode}
      mainContentRef={mainContentRef}
      isViewingDetail={isViewingDetail}
      showSelectionBar={showSelectionBar}
      selectedRunIds={selectedRunIds}
      clearSelection={clearSelection}
      handleBulkDelete={handleBulkDelete}
      groups={groups}
      groupsLoading={groupsLoading}
      groupsError={groupsError}
      runsByBranch={runsByBranch}
      filteredRecentRuns={filteredRecentRuns}
      groupedEvents={groupedEvents}
      runsLoading={runsLoading}
      runsError={runsError}
      repoSummaries={repoSummaries}
      fetchRepoSummary={fetchRepoSummary}
      onSelectGroup={onSelectGroup}
      handleOpenRun={handleOpenRun}
      handleRunSelect={handleRunSelect}
      collapsedEvents={collapsedEvents}
      toggleEventGroup={toggleEventGroup}
      collapseAllEvents={collapseAllEvents}
      expandAllEvents={expandAllEvents}
      collapsedBranches={collapsedBranches}
      toggleBranchCollapse={toggleBranchCollapse}
      handleDeleteBranch={handleDeleteBranch}
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
