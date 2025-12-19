import type React from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, NavLink, useParams, useSearchParams } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';
import { calculateGraphLayout, type GraphItem, type GraphLayout } from '../lib/pipelineGraph';

type TabKey = 'main' | 'recent' | 'events';

type Group = {
  id: number;
  name: string;
  parent_id?: number | null;
  description?: string;
  last_run_at?: string;
};

type RunListItem = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
  pipeline_source?: string;
  status: string;
  git_commit_sha?: string;
  git_repo_name?: string;
  git_repo_owner?: string;
  git_ref?: string;
  git_target_ref?: string;
  git_pusher_name?: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
  is_complete?: boolean;
  parent_run_id?: string | null;
  trigger_event_id?: string;
  parent_step_name?: string;
  failure_reason?: string;
};

type ParentRunInfo = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
};

type TaskDefinition = {
  name: string;
  goal?: string;
  script?: string;
  depends_on?: string[];
  ignore_failure?: boolean;
  variables?: Record<string, string>;
};

type StepConfiguration = {
  include?: string;
  sync?: boolean;
  image?: string;
  secrets?: string[];
  volumes?: string[];
  variables?: Record<string, string>;
  ignore_failure?: boolean;
  llm_output_sharing?: boolean;
  tasks?: TaskDefinition[];
};

type TaskDetail = {
  task_id: string;
  step_name: string;
  task_name: string;
  status: string;
  exit_code?: number | null;
  started_at?: string;
  finished_at?: string;
  task_index: number;
};

type StepDetail = {
  name: string;
  status: string;
  depends_on: string[];
  tasks: TaskDetail[];
  duration?: string;
  configuration?: StepConfiguration;
};

type PipelineDefinition = {
  name?: string;
  description?: string;
  version?: string;
  steps?: { name: string; description?: string; depends_on?: string[]; tasks?: TaskDefinition[] }[];
};

type RunDetail = {
  run_info: RunListItem;
  steps: StepDetail[];
  pipeline_definition?: PipelineDefinition;
  pipeline_definition_yaml?: string;
  child_runs: RunListItem[];
  parent_run_info?: ParentRunInfo | null;
};

type TriggerGroup = {
  id: string;
  runs: RunListItem[];
  status: string;
  latestRun?: RunListItem;
};

type BranchEventGroup = {
  id: string;
  runs: RunListItem[];
  status: string;
  startedAt?: string;
  actor?: string;
  branchLabel?: string;
  commitLabel?: string;
};

type LogLine = { id: number; timestamp: string; line: string };
type EnrichedLogLine = LogLine & { level?: string; step?: string };
type RepoSummary = {
  status: string;
  branch: string;
  commit: string;
  pusher: string;
  started_at?: string;
};

const tabs = [
  { id: 'main', label: 'Main' },
  { id: 'recent', label: 'Recent' },
  { id: 'events', label: 'Events' },
];

const STATUS_PRIORITY = ['failure', 'failure (ignored)', 'cancelled', 'running', 'pending', 'skipped', 'success'];
const RECENT_FETCH_SIZE = 60;
const RECENT_INITIAL_BATCH = 30;
const RECENT_BATCH_SIZE = 20;

const STATUS_META: Record<
  string,
  { text: string; pillClass: string; icon: string; strokeClass: string; border: string; bg: string }
> = {
  success: {
    text: 'Success',
    pillClass: 'bg-green-100 text-green-700 border-green-200 dark:bg-green-900/30 dark:text-green-200 dark:border-green-700',
    icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
    strokeClass: 'text-green-700',
    border: 'border-green-500/50',
    bg: 'fill-green-100 dark:fill-green-900/50 stroke-green-500',
  },
  failure: {
    text: 'Failure',
    pillClass: 'bg-red-100 text-red-700 border-red-200 dark:bg-red-900/30 dark:text-red-200 dark:border-red-700',
    icon: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0',
    strokeClass: 'text-red-500',
    border: 'border-red-500/60',
    bg: 'fill-red-100 dark:fill-red-900/50 stroke-red-500',
  },
  'failure (ignored)': {
    text: 'Failure (ignored)',
    pillClass: 'bg-amber-100 text-amber-800 border-amber-200 dark:bg-amber-900/30 dark:text-amber-100 dark:border-amber-600',
    icon: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z',
    strokeClass: 'text-amber-600',
    border: 'border-amber-500/60',
    bg: 'fill-amber-100 dark:fill-amber-900/50 stroke-amber-500',
  },
  running: {
    text: 'Running',
    pillClass: 'bg-blue-100 text-blue-700 border-blue-200 dark:bg-blue-900/30 dark:text-blue-200 dark:border-blue-700',
    icon: 'M12 8v4l3 3',
    strokeClass: 'text-blue-500 animate-pulse',
    border: 'border-blue-500/60',
    bg: 'fill-blue-100 dark:fill-blue-900/50 stroke-blue-500',
  },
  pending: {
    text: 'Pending',
    pillClass: 'bg-gray-100 text-gray-700 border-gray-200 dark:bg-gray-800/40 dark:text-gray-200 dark:border-gray-700',
    icon: 'M12 8v4l3 3',
    strokeClass: 'text-gray-500',
    border: 'border-gray-500/60',
    bg: 'fill-gray-100 dark:fill-gray-800 stroke-gray-500',
  },
  skipped: {
    text: 'Skipped',
    pillClass: 'bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-800/60 dark:text-slate-200 dark:border-slate-700',
    icon: 'M15 12H9',
    strokeClass: 'text-slate-500',
    border: 'border-slate-500/60',
    bg: 'fill-slate-100 dark:fill-slate-800 stroke-slate-500',
  },
  cancelled: {
    text: 'Cancelled',
    pillClass: 'bg-orange-100 text-orange-700 border-orange-200 dark:bg-orange-900/30 dark:text-orange-200 dark:border-orange-700',
    icon: 'M6 18L18 6M6 6l12 12',
    strokeClass: 'text-orange-500',
    border: 'border-orange-500/60',
    bg: 'fill-orange-100 dark:fill-orange-900/50 stroke-orange-500',
  },
};

function PipelineRunsPage() {
  const { tab: tabParam } = useParams<{ tab?: string }>();
  const [searchParams, setSearchParams] = useSearchParams();

  const activeTab: TabKey = useMemo(() => {
    if (tabParam === 'recent' || tabParam === 'events') return tabParam;
    return 'main';
  }, [tabParam]);

  const [searchTerm, setSearchTerm] = useState(() => (searchParams.get('q') || '').trim());
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
  const [selectedStep, setSelectedStep] = useState<string | null>(null);
  const [stepDetail, setStepDetail] = useState<StepDetail | null>(null);
  const [definitionOpen, setDefinitionOpen] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logsStepFilter, setLogsStepFilter] = useState<string | null>(null);
  const [collapsedEvents, setCollapsedEvents] = useState<Set<string>>(new Set());
  const [collapsedBranches, setCollapsedBranches] = useState<Set<string>>(new Set());
  const collapsedInitRef = useRef(false);

  const pollingRef = useRef<number | null>(null);
  const detailPollRef = useRef<number | null>(null);
  const mainContentRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    setSearchTerm((searchParams.get('q') || '').trim());
  }, [searchParams]);

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

  const fetchJson = useCallback(async <T,>(path: string, options?: RequestInit): Promise<T> => {
    const response = await fetch(buildApiUrl(path), options);
    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || `Request failed: ${response.status}`);
    }
    return (await response.json()) as T;
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
      const message = error instanceof Error ? error.message : 'Unable to load folders';
      setGroupsError(message);
    } finally {
      setGroupsLoading(false);
    }
  }, [fetchJson]);

  const loadRuns = useCallback(async () => {
    setRunsLoading(true);
    setRunsError(null);
    try {
      if (activeTab === 'main' && activeGroupId) {
        const data = await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${activeGroupId}`);
        setRunsByBranch(data || {});
      } else if (activeTab === 'main') {
        setRunsByBranch({});
      } else {
        await fetchRecentPage(0, { replace: true });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load pipeline runs';
      setRunsError(message);
    } finally {
      setRunsLoading(false);
    }
  }, [activeGroupId, activeTab, fetchJson]);

  const loadRunDetail = useCallback(async () => {
    if (!activeRunId) {
      setRunDetail(null);
      setRunDetailError(null);
      return;
    }
    setRunDetailLoading(true);
    setRunDetailError(null);
    try {
      const detail = await fetchJson<RunDetail>(`/v1/runs/${encodeURIComponent(activeRunId)}`);
      setRunDetail(detail);
      if (!selectedStep && detail.steps.length) {
        setSelectedStep(detail.steps[0].name);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load run details';
      setRunDetailError(message);
    } finally {
      setRunDetailLoading(false);
    }
  }, [activeRunId, fetchJson, selectedStep]);

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
  }, [activeGroupId, activeTab, loadRuns]);

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
    const bucket = new Map<string, RunListItem[]>();
    recentRunsAll.forEach(run => {
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
  }, [activeTab, recentRunsAll]);

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

  const handleRecentScroll = useCallback(() => {
    if (activeTab !== 'recent') return;
    const node = mainContentRef.current;
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
    const node = mainContentRef.current;
    if (!node) return;
    const listener = () => handleRecentScroll();
    node.addEventListener('scroll', listener);
    return () => node.removeEventListener('scroll', listener);
  }, [activeTab, handleRecentScroll]);

  useEffect(() => {
    if (activeTab === 'recent') {
      setRecentVisibleCount(RECENT_INITIAL_BATCH);
    }
  }, [activeTab, searchTerm]);

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
      setStepDetail(null);
      setDefinitionOpen(false);
      setLogsStepFilter(null);
    },
    [updateSearchParams]
  );

  const handleCloseDetail = useCallback(() => {
    updateSearchParams({ run: null });
    setRunDetail(null);
    setSelectedStep(null);
    setStepDetail(null);
    setDefinitionOpen(false);
    setLogsStepFilter(null);
  }, [updateSearchParams]);

  const handleDeleteFolder = useCallback(
    async (groupId: number) => {
      if (!window.confirm('Delete this folder? Runs will remain attached to the repository.')) return;
      try {
        await fetchJson(`/v1/groups/${groupId}`, { method: 'DELETE' });
        if (activeGroupId === groupId) updateSearchParams({ group: null });
        await Promise.all([loadGroups(), loadRuns()]);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to delete folder';
        alert(message);
      }
    },
    [activeGroupId, fetchJson, loadGroups, loadRuns, updateSearchParams]
  );

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

  const handleNewFolder = useCallback(async () => {
    const name = window.prompt('Folder name');
    if (!name) return;
    try {
      await fetchJson('/v1/groups', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), parent_id: activeGroupId }),
      });
      await loadGroups();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create folder';
      alert(message);
    }
  }, [activeGroupId, fetchJson, loadGroups]);

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
    },
    [updateSearchParams]
  );

  const mainRunsEmpty = activeTab === 'main' && Boolean(activeGroupId) && Object.keys(runsByBranch).length === 0;
  const showSelectionBar = selectedRunIds.size > 0;
  const trimmedSearch = searchTerm.trim();

  return (
    <div data-page="pipelineruns" className="active min-h-screen flex flex-col overflow-x-hidden overflow-y-auto">
      <div className="px-6 pt-6 flex-shrink-0 tabs-nav-wrapper">
        <div className="border-b border-[var(--border-primary)]">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <nav className="tabs-nav" aria-label="Pipeline run tabs" role="tablist">
              {tabs.map(tab => (
                <NavLink
                  key={tab.id}
                  to={`/pipelineruns/${tab.id}`}
                  role="tab"
                  className={({ isActive }) => `tabs-nav__link ${isActive ? 'tabs-nav__link--active' : ''}`}
                  onClick={() => {
                    updateSearchParams({ run: null, group: activeGroupId, q: searchTerm || null });
                    setSelectedRunIds(new Set());
                  }}
                >
                  {tab.label}
                </NavLink>
              ))}
            </nav>
            <div className="flex items-center gap-2 flex-shrink-0 order-1 sm:order-2">
              {activeTab !== 'main' && <ViewToggle viewMode={viewMode} onChange={setViewMode} />}
              {activeTab === 'main' && (
                <button
                  type="button"
                  className={`flex items-center gap-2 px-4 py-2 rounded-full border border-[var(--border-primary)] bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)] text-[var(--text-primary)] transition shadow-sm disabled:opacity-60 disabled:cursor-not-allowed`}
                  onClick={handleNewFolder}
                  aria-label="New Folder"
                  disabled={Boolean(trimmedSearch)}
                  title={trimmedSearch ? 'Clear search to create a folder' : 'New Folder'}
                >
                  <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 5v14M5 12h14" />
                  </svg>
                  <span>New Folder</span>
                </button>
              )}
            </div>
            <div className="relative flex-1 min-w-[260px] max-w-3xl order-2 sm:order-1">
              <input
                type="search"
                value={searchTerm}
                placeholder="Search runs"
                aria-label="Search pipeline runs"
                id="pipeline-runs-search"
                name="pipeline-runs-search"
                onChange={event => {
                  setSearchTerm(event.target.value);
                  updateSearchParams({ q: event.target.value || null });
                }}
                className="w-full h-11 rounded-full border border-[var(--border-primary)] bg-[var(--bg-primary)] focus:border-[var(--border-accent)] focus:ring-2 focus:ring-[var(--border-accent)]/60 pl-11 pr-10 text-sm transition text-[var(--text-secondary)] shadow-[0_8px_24px_rgba(0,0,0,0.1)]"
              />
              <svg
                className="w-4 h-4 text-[var(--text-secondary)] absolute left-4 top-1/2 -translate-y-1/2"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 21l-4.35-4.35M10 18a8 8 0 110-16 8 8 0 010 16z" />
              </svg>
            </div>
          </div>
          {showSelectionBar && (
            <div className="mt-3">
              <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded-lg px-4 py-3 text-sm">
                <span className="text-[var(--text-primary)] font-medium">{selectedRunIds.size} runs selected</span>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={clearSelection}
                    className="inline-flex items-center px-3 py-1.5 border border-[var(--border-primary)] rounded-md text-[var(--text-primary)] hover:bg-[var(--border-primary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)] text-xs"
                  >
                    Clear
                  </button>
                  <button
                    type="button"
                    onClick={handleBulkDelete}
                    className="inline-flex items-center px-3 py-1.5 border border-transparent rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 text-xs disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Delete Selected
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="flex-1 min-h-0">
        <main id="main-content-runs" ref={mainContentRef} className="h-full min-h-0 overflow-y-auto p-6 space-y-4">
          {runDetail && activeRunId ? (
            <RunDetailView
              detail={runDetail}
              loading={runDetailLoading}
              error={runDetailError}
              onClose={handleCloseDetail}
              onCancel={() => handleCancelRun(runDetail.run_info.run_id)}
              onRerun={() => handleRerun(runDetail.run_info.run_id)}
              onDelete={() => handleDeleteRun(runDetail.run_info.run_id)}
              selectedStep={selectedStep}
              onSelectStep={setSelectedStep}
              onOpenLogs={() => {
                setLogsStepFilter(null);
                setLogsOpen(true);
              }}
              onOpenRun={handleOpenRun}
              onShowDefinition={() => setDefinitionOpen(true)}
              onOpenStepDetail={setStepDetail}
              onOpenLogsForStep={stepName => {
                setLogsStepFilter(stepName);
                setLogsOpen(true);
              }}
            />
          ) : (
            <Dashboard
              activeTab={activeTab}
              groups={groups}
              groupsLoading={groupsLoading}
              groupsError={groupsError}
              onSelectGroup={onSelectGroup}
              activeGroupId={activeGroupId}
              activeGroupPath={activeGroupPath}
              runsByBranch={runsByBranch}
              recentRuns={filteredRecentRuns}
              groupedEvents={groupedEvents}
              viewMode={viewMode}
              runsLoading={runsLoading}
              runsError={runsError}
              searchTerm={searchTerm}
              repoSummaries={repoSummaries}
              fetchRepoSummary={fetchRepoSummary}
              onDeleteFolder={handleDeleteFolder}
              onOpenRun={handleOpenRun}
              onSelectRun={handleRunSelect}
              selectedRunIds={selectedRunIds}
              mainRunsEmpty={mainRunsEmpty}
              collapsedEvents={collapsedEvents}
              onToggleEventGroup={toggleEventGroup}
              onCollapseAllEvents={collapseAllEvents}
              onExpandAllEvents={expandAllEvents}
              collapsedBranches={collapsedBranches}
              onToggleBranch={toggleBranchCollapse}
            />
          )}
        </main>
      </div>

      {definitionOpen && runDetail && (
        <PipelineDefinitionModal
          open={definitionOpen}
          pipelineName={runDetail.run_info.pipeline_name}
          yamlText={runDetail.pipeline_definition_yaml}
          definition={runDetail.pipeline_definition}
          onClose={() => setDefinitionOpen(false)}
        />
      )}

      {stepDetail && (
        <StepDetailModal
          step={stepDetail}
          pipelineDefinition={runDetail?.pipeline_definition}
          onClose={() => setStepDetail(null)}
          onOpenLogs={() => {
            if (stepDetail) {
              setLogsStepFilter(stepDetail.name);
              setLogsOpen(true);
            }
          }}
        />
      )}

      {logsOpen && activeRunId && (
        <LogsModal
          runId={activeRunId}
          onClose={() => {
            setLogsOpen(false);
            setLogsStepFilter(null);
          }}
          stepNames={runDetail?.steps.map(step => step.name)}
          initialStep={logsStepFilter}
        />
      )}
    </div>
  );
}

function Dashboard({
  activeTab,
  groups,
  groupsLoading,
  groupsError,
  onSelectGroup,
  activeGroupId,
  activeGroupPath,
  runsByBranch,
  recentRuns,
  groupedEvents,
  viewMode,
  runsLoading,
  runsError,
  searchTerm,
  repoSummaries,
  fetchRepoSummary,
  onDeleteFolder,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  mainRunsEmpty,
  collapsedEvents,
  onToggleEventGroup,
  onCollapseAllEvents,
  onExpandAllEvents,
  collapsedBranches,
  onToggleBranch,
}: {
  activeTab: TabKey;
  groups: Group[];
  groupsLoading: boolean;
  groupsError: string | null;
  onSelectGroup: (id: number | null) => void;
  activeGroupId: number | null;
  activeGroupPath: Group[];
  runsByBranch: Record<string, RunListItem[]>;
  recentRuns: RunListItem[];
  groupedEvents: TriggerGroup[];
  viewMode: 'grid' | 'list';
  runsLoading: boolean;
  runsError: string | null;
  searchTerm: string;
  repoSummaries: Map<number, RepoSummary>;
  fetchRepoSummary: (groupId: number) => Promise<void>;
  onDeleteFolder: (id: number) => void;
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  mainRunsEmpty: boolean;
  collapsedEvents: Set<string>;
  onToggleEventGroup: (id: string) => void;
  onCollapseAllEvents: () => void;
  onExpandAllEvents: () => void;
  collapsedBranches: Set<string>;
  onToggleBranch: (branch: string) => void;
}) {
  const term = searchTerm.trim().toLowerCase();
  const effectiveViewMode = activeTab === 'main' ? 'grid' : viewMode;

  const childGroups = useMemo(() => {
    if (activeTab !== 'main') return [] as Group[];
    return groups.filter(g => (g.parent_id ?? null) === (activeGroupId ?? null));
  }, [activeGroupId, activeTab, groups]);

  const visibleGroups = useMemo(() => {
    if (activeTab !== 'main') return [] as Group[];
    if (!term) return childGroups;
    return childGroups.filter(group => group.name.toLowerCase().includes(term));
  }, [activeTab, childGroups, term]);

  const filteredRunsByBranch = useMemo(() => {
    if (activeTab !== 'main') return runsByBranch;
    if (!term) return runsByBranch;
    return Object.entries(runsByBranch).reduce<Record<string, RunListItem[]>>((acc, [branch, runs]) => {
      const filtered = runs.filter(run => runMatchesSearch(run, term));
      if (filtered.length) acc[branch] = filtered;
      return acc;
    }, {});
  }, [activeTab, runsByBranch, term]);

  useEffect(() => {
    if (activeTab !== 'main') return;
    visibleGroups.forEach(group => {
      if (group.name.includes('/') && !repoSummaries.has(group.id)) {
        void fetchRepoSummary(group.id);
      }
    });
  }, [activeTab, fetchRepoSummary, repoSummaries, visibleGroups]);

  if (runsError) {
    return <div className="text-red-500 text-sm">{runsError}</div>;
  }

  if (activeTab === 'main') {
    return (
      <div className="space-y-4">
        {groupsError && <div className="text-red-500 text-sm">{groupsError}</div>}

        {activeGroupPath.length > 0 && (
          <div className="flex items-center flex-wrap gap-2 text-sm text-[var(--text-secondary)]">
            <button
              type="button"
              className="runner-pill runner-pill--muted"
              onClick={() => onSelectGroup(null)}
              aria-label="Back to root folders"
            >
              All folders
            </button>
            {activeGroupPath.map((group: Group) => (
              <div key={group.id} className="flex items-center gap-2">
                <span className="text-[var(--border-primary)]">/</span>
                <button
                  type="button"
                  className={`runner-pill ${group.id === activeGroupId ? 'runner-pill--muted' : 'runner-pill--ghost'}`}
                  onClick={() => onSelectGroup(group.id)}
                >
                  {group.name}
                </button>
              </div>
            ))}
          </div>
        )}

        {groupsLoading && !groups.length ? (
          <div className="text-sm text-[var(--text-secondary)]">Loading folders…</div>
        ) : (
          <GroupGrid
            groups={visibleGroups}
            allGroups={groups}
            activeGroupId={activeGroupId}
            repoSummaries={repoSummaries}
            onSelect={onSelectGroup}
            onDelete={onDeleteFolder}
          />
        )}

        {mainRunsEmpty && (
          <div className="p-6 border border-dashed border-[var(--border-primary)] rounded-lg text-center text-sm text-[var(--text-secondary)]">
            No runs found for this folder yet.
          </div>
        )}

        {Object.keys(filteredRunsByBranch).length > 0 && (
          <div className="space-y-5">
            {Object.entries(filteredRunsByBranch)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([branch, runs]) => (
                <BranchRunsSection
                  key={branch}
                  branch={branch}
                  runs={runs}
                  onOpenRun={onOpenRun}
                  onSelectRun={onSelectRun}
              selectedRunIds={selectedRunIds}
              collapsed={collapsedBranches.has(branch)}
              onToggleBranch={() => onToggleBranch(branch, collapsedBranches.has(branch))}
              onDeleteBranch={() => handleDeleteBranch(branch)}
            />
          ))}
        </div>
      )}
    </div>
    );
  }

  if (activeTab === 'events') {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-end gap-3">
          <button className="text-xs font-semibold text-[var(--text-secondary)] hover:text-[var(--text-primary)]" type="button" onClick={onExpandAllEvents}>
            Expand all
          </button>
          <span className="text-[var(--border-primary)]">|</span>
          <button className="text-xs font-semibold text-[var(--text-secondary)] hover:text-[var(--text-primary)]" type="button" onClick={onCollapseAllEvents}>
            Collapse all
          </button>
        </div>
        {runsLoading && <div className="text-sm text-[var(--text-secondary)]">Loading runs…</div>}
        {groupedEvents.length === 0 && !runsLoading ? (
          <div className="text-sm text-[var(--text-secondary)]">No trigger events yet.</div>
        ) : (
          <div className="space-y-3">
            {groupedEvents.map(group => (
              <EventCard
                key={group.id}
                group={group}
                collapsed={collapsedEvents.has(group.id)}
                onToggle={() => onToggleEventGroup(group.id)}
                onOpenRun={onOpenRun}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {runsLoading && <div className="text-sm text-[var(--text-secondary)]">Loading runs…</div>}
      <RunCollection
        runs={recentRuns}
        viewMode={effectiveViewMode}
        onOpenRun={onOpenRun}
        onSelectRun={onSelectRun}
        selectedRunIds={selectedRunIds}
      />
    </div>
  );
}

function GroupGrid({
  groups,
  allGroups,
  activeGroupId,
  repoSummaries,
  onSelect,
  onDelete,
}: {
  groups: Group[];
  allGroups: Group[];
  activeGroupId: number | null;
  repoSummaries: Map<number, RepoSummary>;
  onSelect: (id: number) => void;
  onDelete: (id: number) => void;
}) {
  if (!groups.length) return null;
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
      {groups.map(group => {
        const isRepo = group.name.includes('/');
        const description = (group.description || '').trim();
        const isActive = activeGroupId === group.id;
        const summary = repoSummaries.get(group.id);
        const applications = allGroups.filter(child => (child.parent_id ?? null) === group.id && child.name.includes('/')).length;
        const subfolders = allGroups.filter(child => (child.parent_id ?? null) === group.id && !child.name.includes('/')).length;
        const displayName = isRepo ? group.name.split('/')[1] : group.name;
        if (isRepo) {
          return (
            <div
              key={group.id}
              role="button"
              tabIndex={0}
              onClick={() => onSelect(group.id)}
              onKeyDown={event => {
                if (event.key === 'Enter') onSelect(group.id);
              }}
              className={`relative group bg-[var(--bg-secondary)] p-4 rounded-md hover:bg-[var(--bg-tertiary)] transition-colors duration-200 border border-[var(--border-primary)] hover:border-[var(--border-accent)] shadow-sm hover:shadow-lg flex flex-col justify-between min-h-[220px] ${isActive ? 'run-link-highlight' : ''}`}
            >
              <button
                type="button"
                className="delete-group-btn absolute top-2 right-2 text-[var(--text-secondary)] hover:text-red-500 opacity-0 group-hover:opacity-100 transition-opacity z-10"
                aria-label={`Delete ${displayName}`}
                onClick={event => {
                  event.stopPropagation();
                  onDelete(group.id);
                }}
              >
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </button>
              <div className="flex items-center">
                <svg className="h-8 w-8 text-[var(--text-accent)] mr-4" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <circle cx="8" cy="7" r="2.2" fill="currentColor" />
                  <circle cx="8" cy="17" r="2.2" fill="currentColor" />
                  <circle cx="16" cy="7" r="2.2" fill="currentColor" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2.2" d="M10.2 7h3.8M8 9v6a4 4 0 004 4h4" />
                </svg>
                <span className="text-lg font-medium text-[var(--text-primary)] truncate" title={displayName}>
                  {displayName}
                </span>
              </div>
              {summary ? (
                <div className="mt-4 pt-3 border-t border-[var(--border-primary)] text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center">
                      <RunStatusIcon status={summary.status} complete />
                      <span className="font-semibold text-sm text-[var(--text-primary)] truncate ml-2">{formatBranch(summary.branch)}</span>
                    </div>
                    <span className="text-xs text-[var(--text-secondary)] flex-shrink-0 ml-2">{timeAgo(summary.started_at)}</span>
                  </div>
                  <div className="flex items-center">
                    <CommitIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
                    <span className="truncate">{summary.commit || '—'}</span>
                  </div>
                  <div className="flex items-center">
                    <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                    </svg>
                    <span className="truncate">{summary.pusher || 'N/A'}</span>
                  </div>
                </div>
              ) : (
                <p className="mt-3 text-sm text-[var(--text-secondary)]">No runs yet.</p>
              )}
            </div>
          );
        }

        return (
          <div
            key={group.id}
            role="button"
            tabIndex={0}
            onClick={() => onSelect(group.id)}
            onKeyDown={event => {
              if (event.key === 'Enter') onSelect(group.id);
            }}
            className={`pipeline-folder-card border border-[var(--border-primary)] ${isActive ? 'run-link-highlight' : ''}`}
          >
            <div className="pipeline-folder-card-header">
              <span className="pipeline-folder-icon">
                <svg className="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z" />
                </svg>
              </span>
              <h3 className="pipeline-folder-title" title={displayName}>
                {displayName}
              </h3>
              <div className="pipeline-folder-actions">
                <span className="pipeline-folder-chevron" aria-hidden="true">
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M9 5l7 7-7 7" />
                  </svg>
                </span>
                <button
                  className="pipelines-delete-button pipeline-folder-delete-btn delete-group-btn"
                  type="button"
                  title="Delete folder"
                  aria-label={`Delete ${displayName}`}
                  onClick={event => {
                    event.stopPropagation();
                    onDelete(group.id);
                  }}
                >
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                    <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                    <path d="M4 7h16" />
                  </svg>
                </button>
              </div>
            </div>
            {description && <p className="pipeline-folder-description" title={description}>{description}</p>}
            <div className="pipeline-folder-meta">
              <div className="pipeline-folder-meta-row">
                <span className="pipeline-folder-meta-label">Applications:</span>
                <span className="pipeline-folder-meta-value">{applications}</span>
              </div>
              <div className="pipeline-folder-meta-row">
                <span className="pipeline-folder-meta-label">Sub folders:</span>
                <span className="pipeline-folder-meta-value">{subfolders}</span>
              </div>
            </div>
            {group.last_run_at && (
              <p className="mt-2 text-[11px] text-[var(--text-secondary)]">Last run {timeAgo(group.last_run_at)}</p>
            )}
          </div>
        );
      })}
    </div>
  );
}

function BranchIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="6" y1="3" x2="6" y2="15" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 01-9 9" />
    </svg>
  );
}

function CommitIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="3" />
      <path d="M3 12h6" />
      <path d="M15 12h6" />
    </svg>
  );
}

function BranchRunsSection({
  branch,
  runs,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsed,
  onToggleBranch,
  onDeleteBranch,
}: {
  branch: string;
  runs: RunListItem[];
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsed: boolean;
  onToggleBranch: () => void;
  onDeleteBranch: () => void;
}) {
  const branchLabel = formatBranch(branch);
  const sortedRuns = useMemo(() => [...runs].sort((a, b) => runTimestamp(b) - runTimestamp(a)), [runs]);
  const latestRun = sortedRuns[0];
  const latestStatus = normalizeStatus(latestRun?.status, latestRun?.is_complete);
  const latestTime = latestRun ? timeAgo(latestRun.started_at || latestRun.finished_at) : '—';

  const events = useMemo<BranchEventGroup[]>(() => {
    const bucket = new Map<string, RunListItem[]>();
    sortedRuns.forEach(run => {
      const key = run.trigger_event_id || run.run_id || 'unknown';
      const list = bucket.get(key) || [];
      list.push(run);
      bucket.set(key, list);
    });
    return Array.from(bucket.entries())
      .map(([id, items]) => {
        const ordered = [...items].sort((a, b) => runTimestamp(b) - runTimestamp(a));
        const newest = ordered[0];
        return {
          id,
          runs: ordered,
          status: summarizeStatus(ordered),
          startedAt: newest?.started_at || newest?.finished_at,
          actor: newest?.git_pusher_name,
          branchLabel: formatBranchDisplay(newest?.git_ref, newest?.git_target_ref),
          commitLabel: newest?.git_commit_sha ? newest.git_commit_sha.slice(0, 7) : undefined,
        };
      })
      .sort((a, b) => runTimestamp(b.runs[0]) - runTimestamp(a.runs[0]));
  }, [sortedRuns]);

  const timeline = useMemo(() => buildStatusTimeline(sortedRuns, 40), [sortedRuns]);

  return (
    <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-[0_10px_24px_rgba(0,0,0,0.12)] overflow-hidden" data-branch-row={branch}>
      <button
        type="button"
        className="w-full flex flex-col gap-3 px-4 sm:px-5 py-3 text-left hover:bg-[var(--bg-tertiary)] transition-colors sm:flex-row sm:items-center sm:gap-4 sm:flex-nowrap sm:justify-between"
        onClick={onToggleBranch}
        aria-expanded={!collapsed}
        aria-label={`${collapsed ? 'Expand' : 'Collapse'} branch ${branchLabel || branch}`}
      >
        <div className="flex items-center gap-3 min-w-[180px] sm:min-w-[240px] flex-1">
          <svg
            className={`h-5 w-5 text-[var(--text-secondary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="M9 5l7 7-7 7" />
          </svg>
          <span className="h-5 w-5 flex items-center justify-center text-[var(--text-link)]">
            <BranchIcon className="h-4 w-4" />
          </span>
          <span className="text-base font-semibold text-[var(--text-primary)] break-words" title={branchLabel || branch}>
            {branchLabel || branch}
          </span>
        </div>
        <div className="flex items-center gap-3 sm:gap-4 text-xs text-[var(--text-secondary)] sm:flex-1 sm:flex-nowrap flex-wrap justify-end">
          <div className="flex items-center gap-2 flex-nowrap overflow-hidden pr-1 sm:pr-0 sm:ml-auto">
            <StatusTimeline items={timeline} />
          </div>
          <span className="whitespace-nowrap">({sortedRuns.length} {sortedRuns.length === 1 ? 'run' : 'runs'})</span>
          <span className="hidden sm:inline-block h-4 border-l border-[var(--border-primary)]" aria-hidden="true" />
          <span className="whitespace-nowrap">Latest: {latestTime}</span>
          <span className="hidden sm:inline-block h-4 border-l border-[var(--border-primary)]" aria-hidden="true" />
          <BranchStatusIcon status={latestStatus} />
          <button
            type="button"
            className="ml-2 h-8 w-8 flex items-center justify-center rounded-full text-[var(--text-secondary)] hover:text-red-400 hover:bg-[var(--bg-tertiary)] border border-transparent hover:border-[var(--border-primary)]"
            aria-label={`Delete branch ${branchLabel || branch}`}
            onClick={event => {
              event.stopPropagation();
              onDeleteBranch();
            }}
          >
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7" />
              <path d="M10 11v6" />
              <path d="M14 11v6" />
              <path d="M9 7h6" />
              <path d="M12 3v1" />
            </svg>
          </button>
        </div>
      </button>
      {!collapsed && (
        <div className="border-t border-[var(--border-primary)] p-4 sm:p-5 space-y-4 bg-[var(--bg-primary)]">
          {events.map(event => (
            <BranchEventCard
              key={event.id}
              event={event}
              onOpenRun={onOpenRun}
              onSelectRun={onSelectRun}
              selectedRunIds={selectedRunIds}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function RunCollection({
  runs,
  viewMode,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
}: {
  runs: RunListItem[];
  viewMode: 'grid' | 'list';
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
}) {
  if (!runs.length) {
    return <div className="text-sm text-[var(--text-secondary)]">No runs to display.</div>;
  }

  if (viewMode === 'list') {
    return (
      <div className="flex flex-col gap-3">
        {runs.map(run => (
          <ListRunRow
            key={run.run_id}
            run={run}
            selected={selectedRunIds.has(run.run_id)}
            onSelect={() => onSelectRun(run.run_id)}
            onOpen={() => onOpenRun(run.run_id)}
          />
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
      {runs.map(run => (
        <RunCard
          key={run.run_id}
          run={run}
          selected={selectedRunIds.has(run.run_id)}
          onSelect={() => onSelectRun(run.run_id)}
          onOpen={() => onOpenRun(run.run_id)}
        />
      ))}
    </div>
  );
}

function StatusTimeline({ items }: { items: { key: string; status: string }[] }) {
  if (!items.length) {
    return <span className="text-xs text-[var(--text-secondary)]">No runs yet</span>;
  }
  return (
    <div className="flex items-center gap-1.5 flex-nowrap overflow-hidden" aria-hidden="true">
      {items.map(item => (
        <span key={item.key} className={`h-2 w-2 rounded-full ${getStatusDotClass(item.status)}`} title={item.status} />
      ))}
    </div>
  );
}

function BranchEventCard({
  event,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
}: {
  event: BranchEventGroup;
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
}) {
  if (!event.runs.length) return null;
  const meta = getStatusMeta(event.status, event.status === 'success');
  const triggerLabel = formatTriggerId(event.id);
  return (
    <div className="border border-[var(--border-primary)] rounded-xl bg-[var(--bg-secondary)] shadow-[0_10px_28px_rgba(0,0,0,0.12)]">
      <div className="flex items-center justify-between gap-3 px-4 py-3 border-b border-[var(--border-primary)] text-xs text-[var(--text-secondary)]">
        <div className="flex items-center gap-3 min-w-0 flex-1 flex-nowrap overflow-hidden">
          <span className={`runner-pill ${meta.pillClass} flex-shrink-0`}>{meta.text}</span>
          <div className="flex items-center gap-2 min-w-0 flex-nowrap overflow-hidden text-xs text-[var(--text-secondary)]">
            <span className="text-sm font-semibold text-[var(--text-primary)] truncate" title={triggerLabel.full}>
              Event: {triggerLabel.display}
            </span>
            {event.startedAt && (
              <span className="inline-flex items-center gap-1 whitespace-nowrap">
                <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {timeAgo(event.startedAt)}
              </span>
            )}
            {event.actor && (
              <span className="inline-flex items-center gap-1 whitespace-nowrap">
                <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                </svg>
                {event.actor}
              </span>
            )}
            {event.commitLabel && (
              <span className="inline-flex items-center gap-1 font-mono whitespace-nowrap">
                <CommitIcon className="h-3.5 w-3.5" />
                {event.commitLabel}
              </span>
            )}
          </div>
        </div>
        <span className="px-3 py-1 rounded-full text-[11px] bg-[var(--bg-primary)] border border-[var(--border-primary)] text-[var(--text-secondary)] whitespace-nowrap">
          {event.runs.length} {event.runs.length === 1 ? 'run' : 'runs'}
        </span>
      </div>
      <div className="p-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {event.runs.map(run => (
          <RunCard
            key={run.run_id}
            run={run}
            selected={selectedRunIds.has(run.run_id)}
            onSelect={() => onSelectRun(run.run_id)}
            onOpen={() => onOpenRun(run.run_id)}
            variant="event"
          />
        ))}
      </div>
    </div>
  );
}

function RunCard({
  run,
  selected,
  onSelect,
  onOpen,
  variant = 'default',
  showSelect = true,
}: {
  run: RunListItem;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
  variant?: 'default' | 'event';
  showSelect?: boolean;
}) {
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
  const repoLabel = run.pipeline_path || run.git_repo_name || 'N/A';
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const cardTone =
    variant === 'event'
      ? 'border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-[0_6px_18px_rgba(0,0,0,0.12)]'
      : 'border-[var(--border-primary)] bg-transparent shadow-sm';
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={event => {
        if (event.key === 'Enter') onOpen();
      }}
      className={`run-card run-card--grid p-4 flex flex-col justify-between ${cardTone} hover:border-[var(--border-accent)] rounded-2xl ${selected ? 'run-link-highlight' : ''}`}
    >
      <div className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div className="flex-1 min-w-0 pr-2">
            <div className="flex items-center gap-2 min-w-0">
              <RunStatusIcon status={run.status} complete={run.is_complete} />
              <div className="min-w-0">
                <p className="text-sm font-semibold text-[var(--text-primary)] truncate">{run.pipeline_name}</p>
                <p className="text-xs text-[var(--text-secondary)] truncate">{repoLabel}</p>
              </div>
            </div>
            <p className="text-sm text-[var(--text-link)] font-mono mt-2 truncate flex items-center gap-1">
              <BranchIcon className="h-4 w-4" />
              {branchLabel || 'N/A'}
            </p>
          </div>
          <PipelineBadges run={run} />
        </div>
        <div className="text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
            <span className="truncate">{run.git_pusher_name || 'N/A'}</span>
          </div>
          <div className="flex items-center">
            <CommitIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
            <span className="truncate" title="Commit Hash">{(run.git_commit_sha || '...').slice(0, 8)}</span>
          </div>
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" />
            </svg>
            <span className="truncate" title="Run ID">{(run.run_id || '...').slice(0, 8)}</span>
          </div>
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M7 7a1 1 0 011-1h3.586a1 1 0 01.707.293l6.414 6.414a1 1 0 010 1.414l-4.586 4.586a1 1 0 01-1.414 0L7.293 13.707A1 1 0 017 13V9a1 1 0 011-1z" />
            </svg>
            <span className="truncate" title="Trigger Event ID">{triggerLabel.display}</span>
          </div>
        </div>
      </div>
      <div className="mt-4 pt-3 border-t border-[var(--border-primary)] flex items-center justify-between text-xs text-[var(--text-secondary)]">
        <div className="flex items-center gap-2">
          <svg className="h-3.5 w-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="truncate">{timeAgo(timeToDisplay)}</span>
        </div>
        {showSelect && <RunSelectToggle selected={selected} onToggle={onSelect} />}
      </div>
    </div>
  );
}

function ListRunRow({ run, selected, onSelect, onOpen }: { run: RunListItem; selected: boolean; onSelect: () => void; onOpen: () => void }) {
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
  const repoLabel = run.pipeline_path || run.git_repo_name || 'N/A';
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  return (
    <div
      className={`run-card run-card--list border border-[var(--border-primary)] bg-transparent shadow-sm rounded-2xl hover:border-[var(--border-accent)] ${selected ? 'run-link-highlight' : ''}`}
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={event => {
        if (event.key === 'Enter') onOpen();
      }}
    >
      <div className="run-list-info">
        <span className="run-list-icon">
          <RunStatusIcon status={run.status} complete={run.is_complete} />
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-start justify-between gap-2">
            <div className="run-list-titles flex-1 min-w-0">
              <div className="run-list-title">{run.pipeline_name}</div>
              <div className="run-list-sub flex items-center gap-1">
                <BranchIcon className="h-4 w-4" />
                {branchLabel || 'N/A'}
              </div>
            </div>
            <PipelineBadges run={run} />
          </div>
          <div className="text-xs text-[var(--text-secondary)] mt-1 truncate">{repoLabel}</div>
        </div>
      </div>
      <div className="run-list-meta">
        <span className="run-list-meta-item">
          <span className="run-list-meta-label">Commit</span>
          <span className="run-list-meta-value font-mono">{(run.git_commit_sha || '...').slice(0, 8)}</span>
        </span>
        <span className="run-list-meta-item">
          <span className="run-list-meta-label">Repo</span>
          <span className="run-list-meta-value">{repoLabel}</span>
        </span>
        <span className="run-list-meta-item">
          <span className="run-list-meta-label">Run ID</span>
          <span className="run-list-meta-value font-mono">{(run.run_id || '...').slice(0, 8)}</span>
        </span>
        <span className="run-list-meta-item">
          <span className="run-list-meta-label">Trigger</span>
          <span className="run-list-meta-value">{triggerLabel.display}</span>
        </span>
        <span className="run-list-meta-item">
          <span className="run-list-meta-label">Updated</span>
          <span className="run-list-meta-value">{timeAgo(timeToDisplay)}</span>
        </span>
      </div>
      <div className="run-list-actions">
        <RunSelectToggle selected={selected} onToggle={onSelect} />
      </div>
    </div>
  );
}

function RunStatusIcon({ status, complete }: { status: string; complete?: boolean }) {
  return <BranchStatusIcon status={status} complete={complete} className="run-status-icon" />;
}

function BranchStatusIcon({ status, complete, className }: { status: string; complete?: boolean; className?: string }) {
  const rawStatus = (status || '').toLowerCase();
  const normalized = normalizeStatus(rawStatus, complete ?? Boolean(STATUS_META[rawStatus]));
  const tone = getBranchStatusTone(normalized);
  const isFailure = normalized === 'failure' || normalized === 'failure (ignored)';
  const isCancelled = normalized === 'cancelled';
  const isRunning = normalized === 'running';
  const isSkipped = normalized === 'skipped';
  const isPending = normalized === 'pending';
  return (
    <span
      className={`inline-flex h-7 w-7 items-center justify-center rounded-full border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${tone} ${className || ''}`}
      aria-label={normalized}
    >
      {isRunning ? (
        <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12a9 9 0 11-6.219-8.56" />
        </svg>
      ) : isFailure || isCancelled ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M18 6L6 18" />
          <path d="M6 6l12 12" />
        </svg>
      ) : isSkipped ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M6 12h12" />
          <path d="M6 16h12" />
        </svg>
      ) : isPending ? (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 8v4l3 3" />
          <path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      ) : (
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 13l4 4L19 7" />
        </svg>
      )}
    </span>
  );
}

function RunSelectToggle({ selected, onToggle }: { selected: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      className={`run-select-toggle inline-flex items-center justify-center h-8 w-8 rounded-full border border-[var(--border-primary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--border-accent)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)] transition-colors duration-150 ${selected ? 'bg-[var(--bg-tertiary)]' : ''}`}
      aria-pressed={selected}
      onClick={event => {
        event.stopPropagation();
        onToggle();
      }}
      title={selected ? 'Deselect run' : 'Select run'}
    >
      <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M5 13l4 4L19 7" />
      </svg>
    </button>
  );
}

function PipelineBadges({ run }: { run: RunListItem }) {
  const badges: React.ReactNode[] = [];
  if (run.pipeline_source === 'database override') {
    badges.push(
      <span key="override" className="text-xs font-semibold text-[var(--text-link)] inline-flex items-center gap-1">
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M13 16h-1v-4h-1m1-4h.01" />
          <path d="M12 2a10 10 0 100 20 10 10 0 000-20z" />
        </svg>
        Override
      </span>
    );
  }
  if (run.parent_run_id) {
    badges.push(
      <span key="included" className="text-xs font-semibold text-[var(--text-link)]">
        Included
      </span>
    );
  }
  if (!badges.length) return null;
  return <div className="flex flex-col items-end gap-1 text-right">{badges}</div>;
}

function StatusBadge({ status, complete }: { status: string; complete?: boolean }) {
  const meta = getStatusMeta(status, complete);
  return (
    <span className={`runner-pill border ${meta.pillClass}`} title={meta.text}>
      <svg className={`h-4 w-4 ${meta.strokeClass}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d={meta.icon} />
      </svg>
      {meta.text}
    </span>
  );
}

function RunDetailView({
  detail,
  loading,
  error,
  onClose,
  onCancel,
  onRerun,
  onDelete,
  selectedStep,
  onSelectStep,
  onOpenLogs,
  onOpenRun,
  onShowDefinition,
  onOpenStepDetail,
  onOpenLogsForStep,
}: {
  detail: RunDetail;
  loading: boolean;
  error: string | null;
  onClose: () => void;
  onCancel: () => void;
  onRerun: () => void;
  onDelete: () => void;
  selectedStep: string | null;
  onSelectStep: (step: string) => void;
  onOpenLogs: () => void;
  onOpenRun: (id: string) => void;
  onShowDefinition: () => void;
  onOpenStepDetail: (step: StepDetail) => void;
  onOpenLogsForStep: (stepName: string) => void;
}) {
  const run = detail.run_info;
  const isRunning = normalizeStatus(run.status, run.is_complete) === 'running';
  const pipelineLink = buildPipelineLink(run);
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const parentRun = detail.parent_run_info;
  const selectedStepData = selectedStep ? detail.steps.find(step => step.name === selectedStep) || null : null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start gap-3">
        <button className="runner-pill runner-pill--muted" type="button" onClick={onClose}>
          ← Back
        </button>
        <div className="flex-1 min-w-[320px] space-y-2">
          {parentRun && (
            <button className="runner-pill runner-pill--ghost" type="button" onClick={() => onOpenRun(parentRun.run_id)}>
              ← Parent: {parentRun.pipeline_name}
            </button>
          )}
          <div className="flex flex-wrap items-center gap-2 text-sm text-[var(--text-secondary)]">
            <span className="font-semibold text-[var(--text-primary)]">{formatRepo(run)}</span>
            <span>•</span>
            <span className="font-mono">{run.run_id}</span>
          </div>
          <div className="flex flex-wrap items-center gap-2 mt-1">
            <StatusBadge status={run.status} complete={run.is_complete} />
            <span className="text-xl font-semibold text-[var(--text-primary)]">{run.pipeline_name}</span>
            {run.pipeline_source && <span className="runner-pill runner-pill--muted">{run.pipeline_source}</span>}
            <span className="runner-pill runner-pill--ghost" title={triggerLabel.full}>
              Trigger {triggerLabel.display}
            </span>
          </div>
          <div className="text-sm text-[var(--text-secondary)] flex flex-wrap gap-2 mt-2">
            <span className="runner-pill runner-pill--muted">Branch {formatBranchDisplay(run.git_ref, run.git_target_ref)}</span>
            <span className="runner-pill runner-pill--muted">Commit {(run.git_commit_sha || '—').slice(0, 7)}</span>
            <span className="runner-pill runner-pill--muted">{run.git_pusher_name || 'Unknown'}</span>
            <span className="runner-pill runner-pill--muted">{run.duration || '—'}</span>
            <span className="runner-pill runner-pill--muted">Started {timeAgo(run.started_at)}</span>
          </div>
        </div>
        <div className="flex items-center gap-2 ml-auto flex-wrap">
          {pipelineLink && (
            <Link className="glass-button-subtle" to={pipelineLink}>
              View pipeline
            </Link>
          )}
          <button className="glass-button-subtle" type="button" onClick={onShowDefinition}>
            Definition
          </button>
          <button className="glass-button-subtle" type="button" onClick={onOpenLogs}>
            View logs
          </button>
          <button className="glass-button-subtle" type="button" onClick={onDelete}>
            Delete
          </button>
          {isRunning ? (
            <button className="glass-button-primary" type="button" onClick={onCancel}>
              Cancel run
            </button>
          ) : (
            <button className="glass-button-primary" type="button" onClick={onRerun} disabled={loading}>
              Rerun
            </button>
          )}
        </div>
      </div>

      {error && <div className="text-red-500 text-sm">{error}</div>}

      {run.failure_reason && (
        <div className="bg-red-50 dark:bg-red-900/40 border border-red-200 dark:border-red-700 text-red-700 dark:text-red-200 px-4 py-3 rounded-lg text-sm">
          Failed to start: {run.failure_reason}
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-[2fr,1fr]">
        <div className="border border-[var(--border-primary)] rounded-xl bg-[var(--bg-secondary)] p-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="font-semibold text-[var(--text-primary)]">Steps graph</h3>
            <span className="text-xs text-[var(--text-secondary)]">{detail.steps.length} steps</span>
          </div>
          <StepsGraph steps={detail.steps} selectedStep={selectedStep} onSelectStep={onSelectStep} />
        </div>
        <div className="border border-[var(--border-primary)] rounded-xl bg-[var(--bg-secondary)] p-4 space-y-3">
          <div className="flex items-center justify-between gap-2 flex-wrap">
            <h3 className="font-semibold text-[var(--text-primary)]">Tasks</h3>
            {selectedStepData && (
              <div className="flex items-center gap-2">
                <button className="glass-button-subtle text-xs" type="button" onClick={() => onOpenLogsForStep(selectedStepData.name)}>
                  Logs for step
                </button>
                <button className="glass-button-subtle text-xs" type="button" onClick={() => onOpenStepDetail(selectedStepData)}>
                  View step details
                </button>
              </div>
            )}
          </div>
          {selectedStepData ? (
            <TaskPanel step={selectedStepData} />
          ) : (
            <div className="text-sm text-[var(--text-secondary)]">Select a step to view its tasks.</div>
          )}
        </div>
      </div>

      {detail.child_runs?.length > 0 && (
        <div className="border border-[var(--border-primary)] rounded-xl bg-[var(--bg-secondary)] p-4 space-y-2">
          <h3 className="font-semibold text-[var(--text-primary)]">Child runs</h3>
          <div className="space-y-2">
            {detail.child_runs.map(child => (
              <div key={child.run_id} className="flex items-center justify-between text-sm">
                <div className="flex items-center gap-2">
                  <StatusBadge status={child.status} complete={child.is_complete} />
                  <span className="font-medium text-[var(--text-primary)]">{child.pipeline_name}</span>
                  {child.parent_step_name && <span className="runner-pill runner-pill--muted">Step {child.parent_step_name}</span>}
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-[var(--text-secondary)]">{timeAgo(child.started_at)}</span>
                  <button className="glass-button-subtle text-xs" type="button" onClick={() => onOpenRun(child.run_id)}>
                    Open
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function StepsGraph({ steps, selectedStep, onSelectStep }: { steps: StepDetail[]; selectedStep: string | null; onSelectStep: (name: string) => void }) {
  const items: GraphItem[] = steps.map(step => ({ name: step.name, depends_on: step.depends_on || [] }));
  const layout = useMemo<GraphLayout>(() => calculateGraphLayout(items, { nodeWidth: 180, nodeHeight: 100, horizontalGap: 120, verticalGap: 32 }), [items]);
  const [scale, setScale] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const draggingRef = useRef(false);
  const lastRef = useRef({ x: 0, y: 0 });

  const handleWheel = (event: React.WheelEvent) => {
    event.preventDefault();
    const delta = event.deltaY < 0 ? 0.1 : -0.1;
    setScale(prev => Math.min(2, Math.max(0.5, prev + delta)));
  };

  const onMouseDown = (event: React.MouseEvent) => {
    draggingRef.current = true;
    setIsDragging(true);
    lastRef.current = { x: event.clientX, y: event.clientY };
  };

  const onMouseMove = (event: React.MouseEvent) => {
    if (!draggingRef.current) return;
    const dx = event.clientX - lastRef.current.x;
    const dy = event.clientY - lastRef.current.y;
    lastRef.current = { x: event.clientX, y: event.clientY };
    setOffset(prev => ({ x: prev.x + dx, y: prev.y + dy }));
  };

  const onMouseUp = () => {
    draggingRef.current = false;
    setIsDragging(false);
  };

  return (
    <div className="relative border border-[var(--border-primary)] rounded-lg bg-[var(--bg-primary)] overflow-hidden" onWheel={handleWheel} onMouseMove={onMouseMove} onMouseUp={onMouseUp} onMouseLeave={onMouseUp}>
      <div className="absolute right-3 top-3 flex items-center gap-2 bg-[var(--bg-secondary)]/80 backdrop-blur px-2 py-1 rounded-full text-xs">
        <button className="runner-pill runner-pill--muted" type="button" onClick={() => setScale(prev => Math.max(0.5, prev - 0.1))}>
          -
        </button>
        <span className="text-[var(--text-secondary)] w-12 text-center">{Math.round(scale * 100)}%</span>
        <button className="runner-pill runner-pill--muted" type="button" onClick={() => setScale(prev => Math.min(2, prev + 0.1))}>
          +
        </button>
        <button className="runner-pill runner-pill--muted" type="button" onClick={() => { setScale(1); setOffset({ x: 0, y: 0 }); }}>
          Reset
        </button>
      </div>
      <div className="w-full h-[480px]" onMouseDown={onMouseDown}>
        <svg
          width="100%"
          height="100%"
          viewBox={`0 0 ${layout.width} ${layout.height}`}
          style={{ transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})`, transformOrigin: 'center center', cursor: isDragging ? 'grabbing' : 'grab' }}
        >
          {layout.edges.map((edge, index) => (
            <path
              key={`${edge.from.id}-${edge.to.id}-${index}`}
              d={buildEdgePath(edge.from.x + edge.from.width, edge.from.y + edge.from.height / 2, edge.to.x, edge.to.y + edge.to.height / 2)}
              fill="none"
              stroke="currentColor"
              strokeWidth={1.6}
              className="text-[var(--border-secondary)]"
            />
          ))}
          {layout.nodes.map(node => {
            const step = steps.find(stepItem => stepItem.name === node.id);
            const meta = getStatusMeta(step?.status || 'pending', step?.status === 'success');
            const isActive = selectedStep === node.id;
            const statusY = node.y + 56;
            return (
              <g key={node.id} className="graph-node" onClick={() => onSelectStep(node.id)}>
                <rect
                  x={node.x}
                  y={node.y}
                  width={node.width}
                  height={node.height}
                  rx={14}
                  className={`stroke-2 ${meta.bg} ${isActive ? 'ring-4 ring-[var(--border-accent)]' : ''}`}
                  fillOpacity={0.7}
                />
                <text x={node.x + 16} y={node.y + 28} className="text-sm font-semibold" fill="currentColor">
                  {node.name}
                </text>
                <circle cx={node.x + 16} cy={statusY} r={6} className={meta.strokeClass} fill="currentColor" />
                <text x={node.x + 30} y={statusY + 4} className={`text-[11px] ${meta.strokeClass}`} fill="currentColor">
                  {meta.text}
                </text>
                <text x={node.x + 16} y={node.y + 88} className="text-[11px] text-[var(--text-secondary)]" fill="currentColor">
                  {step?.duration || '—'}
                </text>
              </g>
            );
          })}
        </svg>
      </div>
    </div>
  );
}

function TaskPanel({ step }: { step: StepDetail | null }) {
  if (!step) return <div className="text-sm text-[var(--text-secondary)]">Select a step to view tasks.</div>;
  const taskDefinitions = step.configuration?.tasks || [];
  if (!taskDefinitions.length) return <div className="text-sm text-[var(--text-secondary)]">No tasks defined for this step.</div>;

  const layout = calculateGraphLayout(
    taskDefinitions.map(task => ({ name: task.name, depends_on: task.depends_on || [] })),
    { nodeWidth: 150, nodeHeight: 90, vertical: true, horizontalGap: 80, verticalGap: 28 }
  );

  const statusByName = new Map<string, string>();
  step.tasks.forEach(task => statusByName.set(task.task_name, task.status));

  return (
    <div className="border border-[var(--border-primary)] rounded-lg bg-[var(--bg-primary)] overflow-hidden">
      <div className="w-full h-64 overflow-auto">
        <svg width={layout.width} height={layout.height} className="bg-[var(--bg-primary)]">
          {layout.edges.map((edge, index) => (
            <path
              key={`${edge.from.id}-${edge.to.id}-${index}`}
              d={buildEdgePath(edge.from.x + edge.from.width / 2, edge.from.y + edge.from.height, edge.to.x + edge.to.width / 2, edge.to.y)}
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
              className="text-[var(--border-secondary)]"
            />
          ))}
          {layout.nodes.map(node => {
            const status = statusByName.get(node.id) || 'pending';
            const meta = getStatusMeta(status, status === 'success');
            const def = taskDefinitions.find(t => t.name === node.id);
            const statusY = node.y + node.height - 18;
            return (
              <g key={node.id} className="graph-node">
                <rect x={node.x} y={node.y} width={node.width} height={node.height} rx={12} className={`${meta.bg}`} />
                <text x={node.x + 12} y={node.y + 26} className="text-sm font-semibold" fill="currentColor">
                  {node.name}
                </text>
                <text x={node.x + 12} y={node.y + 48} className="text-[11px] text-[var(--text-secondary)]" fill="currentColor">
                  {(def?.goal || def?.script || '').slice(0, 40) || 'Task'}
                </text>
                <circle cx={node.x + 12} cy={statusY} r={6} className={meta.strokeClass} fill="currentColor" />
                <text x={node.x + 24} y={statusY + 4} className={`text-[11px] ${meta.strokeClass}`} fill="currentColor">
                  {meta.text}
                </text>
              </g>
            );
          })}
        </svg>
      </div>
    </div>
  );
}

function ViewToggle({ viewMode, onChange }: { viewMode: 'grid' | 'list'; onChange: (mode: 'grid' | 'list') => void }) {
  const isGrid = viewMode !== 'list';
  return (
    <div className="runs-view-toggle" role="group" aria-label="Pipeline run layout">
      <button
        type="button"
        className={`runs-view-toggle__btn ${isGrid ? 'runs-view-toggle__btn--active' : ''}`}
        aria-pressed={isGrid}
        onClick={() => onChange('grid')}
        title="Grid view"
      >
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <rect x="4" y="4" width="7" height="7"></rect>
          <rect x="13" y="4" width="7" height="7"></rect>
          <rect x="4" y="13" width="7" height="7"></rect>
          <rect x="13" y="13" width="7" height="7"></rect>
        </svg>
      </button>
      <button
        type="button"
        className={`runs-view-toggle__btn ${!isGrid ? 'runs-view-toggle__btn--active' : ''}`}
        aria-pressed={!isGrid}
        onClick={() => onChange('list')}
        title="List view"
      >
        <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M4 7h16" />
          <path d="M4 12h16" />
          <path d="M4 17h16" />
        </svg>
      </button>
    </div>
  );
}

function EventCard({
  group,
  collapsed,
  onToggle,
  onOpenRun,
}: {
  group: TriggerGroup;
  collapsed: boolean;
  onToggle: () => void;
  onOpenRun: (id: string) => void;
}) {
  const meta = getStatusMeta(group.status, group.status === 'success');
  const latestRun = group.latestRun || group.runs[0];
  const triggerLabel = formatTriggerId(group.id);
  const branchLabel = latestRun ? formatBranchDisplay(latestRun.git_ref, latestRun.git_target_ref) : '—';
  const commitLabel = latestRun?.git_commit_sha ? latestRun.git_commit_sha.slice(0, 7) : '—';
  const pusher = latestRun?.git_pusher_name || 'System';
  const timestamp = latestRun?.started_at;

  const statusDotClass = (status: string, complete?: boolean) => {
    const normalized = normalizeStatus(status, complete);
    if (normalized === 'success') return 'bg-green-500';
    if (normalized === 'failure') return 'bg-red-500';
    if (normalized === 'failure (ignored)') return 'bg-amber-500';
    if (normalized === 'running') return 'bg-blue-500 animate-pulse';
    if (normalized === 'cancelled') return 'bg-orange-500';
    if (normalized === 'skipped') return 'bg-amber-400';
    return 'bg-gray-500';
  };

  return (
    <div className="border border-[var(--border-primary)] rounded-xl bg-[var(--bg-secondary)] overflow-hidden shadow-[0_10px_28px_rgba(0,0,0,0.12)]">
      <button
        type="button"
        className="w-full flex items-center justify-between gap-3 p-4 text-left hover:bg-[var(--bg-tertiary)] transition-colors"
        onClick={onToggle}
        aria-expanded={!collapsed}
      >
        <div className="flex items-center gap-3 min-w-0 flex-1 flex-nowrap overflow-hidden">
          <svg
            className={`h-4 w-4 text-[var(--text-secondary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="M9 5l7 7-7 7" />
          </svg>
          <span className={`runner-pill ${meta.pillClass} flex-shrink-0`}>{meta.text}</span>
          <div className="flex items-center gap-2 min-w-0 flex-nowrap overflow-hidden text-xs text-[var(--text-secondary)]">
            <span className="text-sm font-semibold text-[var(--text-primary)] truncate" title={triggerLabel.full}>
              Event: {triggerLabel.display}
            </span>
            <span className="inline-flex items-center gap-1 whitespace-nowrap">
              <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              {timeAgo(timestamp)}
            </span>
            <span className="inline-flex items-center gap-1 whitespace-nowrap">
              <BranchIcon className="h-3.5 w-3.5" />
              {branchLabel}
            </span>
            <span className="inline-flex items-center gap-1 font-mono whitespace-nowrap">
              <CommitIcon className="h-3.5 w-3.5" />
              {commitLabel}
            </span>
            <span className="inline-flex items-center gap-1 whitespace-nowrap">
              <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
              </svg>
              {pusher}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-3 flex-shrink-0">
          <div className="flex items-center gap-1">
            {group.runs.slice(0, 6).map(run => (
              <span key={run.run_id} className={`h-2.5 w-2.5 rounded-full ${statusDotClass(run.status, run.is_complete)}`} />
            ))}
          </div>
          <span className="px-3 py-1 text-[11px] rounded-full bg-[var(--bg-primary)] border border-[var(--border-primary)] text-[var(--text-secondary)]">
            {group.runs.length} {group.runs.length === 1 ? 'Pipeline' : 'Pipelines'}
          </span>
        </div>
      </button>
      {!collapsed && (
        <div className="p-4 border-t border-[var(--border-primary)] bg-[var(--bg-primary)]">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {group.runs.map(run => (
              <EventRunRow key={run.run_id} run={run} onOpenRun={onOpenRun} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function EventRunRow({ run, onOpenRun }: { run: RunListItem; onOpenRun: (id: string) => void }) {
  return <RunCard run={run} selected={false} onSelect={() => {}} onOpen={() => onOpenRun(run.run_id)} variant="event" showSelect={false} />;
}

function LogsModal({
  runId,
  onClose,
  stepNames,
  initialStep,
}: {
  runId: string;
  onClose: () => void;
  stepNames?: string[];
  initialStep?: string | null;
}) {
  const [lines, setLines] = useState<EnrichedLogLine[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedStep, setSelectedStep] = useState<string>(initialStep || 'all');
  const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
  const lastIdRef = useRef(0);

  useEffect(() => {
    setSelectedStep(initialStep || 'all');
    setSelectedLevels(new Set());
    setLines([]);
    lastIdRef.current = 0;
  }, [initialStep, runId]);

  useEffect(() => {
    let cancelled = false;
    const fetchLogs = async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(buildApiUrl(`/v1/runs/${encodeURIComponent(runId)}/logs?since_line=${lastIdRef.current}`));
        if (!response.ok) throw new Error(await response.text());
        const payload = (await response.json()) as LogLine[];
        if (cancelled) return;
        if (payload.length) {
          lastIdRef.current = payload[payload.length - 1].id;
          const enriched = payload.map(line => ({ ...line, ...parseLogLine(line.line) }));
          setLines(prev => [...prev, ...enriched]);
        }
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : 'Failed to load logs');
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void fetchLogs();
    const timer = window.setInterval(fetchLogs, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [initialStep, runId]);

  const stepOptions = useMemo(() => {
    const provided = stepNames || [];
    const derived = Array.from(new Set(lines.map(line => line.step).filter(Boolean) as string[]));
    const combined = Array.from(new Set(['all', ...provided, ...derived]));
    return combined;
  }, [lines, stepNames]);

  const visibleLines = useMemo(() => {
    return lines.filter(line => {
      const level = line.level || 'info';
      if (selectedStep !== 'all') {
        if (!line.step || line.step !== selectedStep) return false;
      }
      if (selectedLevels.size > 0 && !selectedLevels.has(level)) return false;
      return true;
    });
  }, [lines, selectedLevels, selectedStep]);

  const toggleLevel = (level: string) => {
    setSelectedLevels(prev => {
      const next = new Set(prev);
      if (next.has(level)) {
        next.delete(level);
      } else {
        next.add(level);
      }
      return next;
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)]">
      <div className="bg-[var(--bg-primary)] rounded-xl shadow-xl w-full max-w-4xl max-h-[80vh] overflow-hidden border border-[var(--border-primary)]">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border-primary)]">
          <div>
            <p className="text-sm font-semibold text-[var(--text-primary)]">Logs for {runId}</p>
            <p className="text-xs text-[var(--text-secondary)]">Streaming newest at the bottom</p>
          </div>
          <button className="glass-button-subtle" type="button" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="px-4 py-3 border-b border-[var(--border-primary)] flex flex-wrap items-center gap-3 bg-[var(--bg-secondary)]">
          <label className="flex items-center gap-2 text-xs text-[var(--text-secondary)]">
            Step
            <select
              className="pipelines-input text-sm"
              value={selectedStep}
              onChange={event => setSelectedStep(event.target.value)}
            >
              {stepOptions.map(option => (
                <option key={option} value={option}>
                  {option === 'all' ? 'All steps' : option}
                </option>
              ))}
            </select>
          </label>
          <div className="flex items-center gap-1">
            {['info', 'warn', 'error', 'debug'].map(level => {
              const active = selectedLevels.has(level);
              return (
                <button
                  key={level}
                  type="button"
                  className={`runner-pill ${active ? 'runner-pill--muted' : 'runner-pill--ghost'}`}
                  onClick={() => toggleLevel(level)}
                >
                  {level}
                </button>
              );
            })}
          </div>
          <button
            type="button"
            className="runner-pill runner-pill--ghost ml-auto"
            onClick={() => {
              setSelectedStep('all');
              setSelectedLevels(new Set());
            }}
          >
            Reset filters
          </button>
        </div>
        <div className="p-4 bg-[var(--bg-secondary)] h-[60vh] overflow-auto font-mono text-xs space-y-1">
          {error && <div className="text-red-500">{error}</div>}
          {loading && !lines.length && <div className="text-[var(--text-secondary)]">Loading…</div>}
          {!loading && visibleLines.length === 0 && <div className="text-[var(--text-secondary)]">No log lines match the current filters.</div>}
          {visibleLines.map(line => (
            <div key={line.id} className="text-[var(--text-primary)] whitespace-pre-wrap">
              <span className="text-[var(--text-secondary)] mr-2">{new Date(line.timestamp).toLocaleTimeString()}</span>
              {line.step && <span className="runner-pill runner-pill--muted mr-2">{line.step}</span>}
              {line.level && <span className="runner-pill runner-pill--ghost mr-2 uppercase">{line.level}</span>}
              {line.line}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function PipelineDefinitionModal({
  open,
  pipelineName,
  yamlText,
  definition,
  onClose,
}: {
  open: boolean;
  pipelineName: string;
  yamlText?: string | null;
  definition?: PipelineDefinition;
  onClose: () => void;
}) {
  const content = useMemo(() => {
    if (yamlText && yamlText.trim().length > 0) return yamlText;
    if (definition) {
      try {
        return yaml.dump(definition);
      } catch {
        return JSON.stringify(definition, null, 2);
      }
    }
    return 'Pipeline definition is not available for this run.';
  }, [definition, yamlText]);

  if (!open) return null;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      return true;
    } catch {
      return false;
    }
  };

  const handleDownload = () => {
    const blob = new Blob([content], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${pipelineName || 'pipeline'}.yaml`;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)]">
      <div className="bg-[var(--bg-primary)] rounded-xl shadow-xl w-full max-w-5xl max-h-[85vh] overflow-hidden border border-[var(--border-primary)]">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border-primary)]">
          <div>
            <p className="text-sm font-semibold text-[var(--text-primary)]">Pipeline definition</p>
            <p className="text-xs text-[var(--text-secondary)]">{pipelineName}</p>
          </div>
          <div className="flex items-center gap-2">
            <button className="glass-button-subtle" type="button" onClick={handleCopy}>
              Copy
            </button>
            <button className="glass-button-subtle" type="button" onClick={handleDownload}>
              Download
            </button>
            <button className="glass-button-primary" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>
        <div className="p-4 bg-[var(--bg-secondary)] h-[70vh] overflow-auto">
          <pre className="text-xs text-[var(--text-primary)] whitespace-pre-wrap leading-5">{content}</pre>
        </div>
      </div>
    </div>
  );
}

function StepDetailModal({
  step,
  pipelineDefinition,
  onClose,
  onOpenLogs,
}: {
  step: StepDetail;
  pipelineDefinition?: PipelineDefinition;
  onClose: () => void;
  onOpenLogs: () => void;
}) {
  if (!step) return null;
  const config = step.configuration || {};
  const definitionSteps = pipelineDefinition?.steps || [];
  const matchingDefinition = definitionSteps.find(item => item.name === step.name);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)]">
      <div className="bg-[var(--bg-primary)] rounded-xl shadow-xl w-full max-w-5xl max-h-[85vh] overflow-hidden border border-[var(--border-primary)]">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border-primary)]">
          <div className="space-y-1">
            <div className="flex items-center gap-2 flex-wrap">
              <StatusBadge status={step.status} complete={step.status === 'success'} />
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step: {step.name}</h3>
              {matchingDefinition?.description && <span className="runner-pill runner-pill--muted">{matchingDefinition.description}</span>}
            </div>
            <p className="text-xs text-[var(--text-secondary)]">
              Duration {step.duration || '—'} - Depends on {step.depends_on.length ? step.depends_on.join(', ') : 'No dependencies'}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button className="glass-button-subtle" type="button" onClick={onOpenLogs}>
              Logs
            </button>
            <button className="glass-button-primary" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>
        <div className="grid gap-4 md:grid-cols-2 p-4 bg-[var(--bg-secondary)] max-h-[75vh] overflow-auto">
          <div className="space-y-3">
            <h4 className="text-sm font-semibold text-[var(--text-primary)]">Configuration</h4>
            <div className="space-y-2 text-sm text-[var(--text-secondary)]">
              <div className="runner-pill runner-pill--muted">Image: {config.image || 'Inherited'}</div>
              {config.include && <div className="runner-pill runner-pill--muted">Include: {config.include}</div>}
              {config.sync !== undefined && <div className="runner-pill runner-pill--muted">Sync: {config.sync ? 'true' : 'false'}</div>}
              {config.ignore_failure !== undefined && <div className="runner-pill runner-pill--muted">Ignore failure: {config.ignore_failure ? 'true' : 'false'}</div>}
              {config.variables && (
                <div>
                  <p className="text-xs uppercase tracking-wide mb-1">Variables</p>
                  <div className="flex flex-wrap gap-1">
                    {Object.entries(config.variables).map(([key, value]) => (
                      <span key={key} className="runner-pill runner-pill--ghost">{`${key}=${value}`}</span>
                    ))}
                  </div>
                </div>
              )}
              {config.secrets && config.secrets.length > 0 && (
                <div>
                  <p className="text-xs uppercase tracking-wide mb-1">Secrets</p>
                  <div className="flex flex-wrap gap-1">
                    {config.secrets.map(secret => (
                      <span key={secret} className="runner-pill runner-pill--ghost">{secret}</span>
                    ))}
                  </div>
                </div>
              )}
              {config.volumes && config.volumes.length > 0 && (
                <div>
                  <p className="text-xs uppercase tracking-wide mb-1">Volumes</p>
                  <div className="flex flex-wrap gap-1">
                    {config.volumes.map(volume => (
                      <span key={volume} className="runner-pill runner-pill--ghost">{volume}</span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <h4 className="text-sm font-semibold text-[var(--text-primary)]">Tasks</h4>
              <span className="text-xs text-[var(--text-secondary)]">{step.tasks.length} tasks</span>
            </div>
            {step.tasks.length === 0 ? (
              <p className="text-xs text-[var(--text-secondary)]">No tasks recorded for this step.</p>
            ) : (
              <div className="space-y-2">
                {step.tasks.map(task => {
                  const taskMeta = getStatusMeta(task.status, task.status === 'success');
                  return (
                    <div key={task.task_id} className="flex items-start justify-between gap-3 border border-[var(--border-primary)] rounded-lg px-3 py-2 bg-[var(--bg-primary)]">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className={`runner-pill ${taskMeta.pillClass}`}>{taskMeta.text}</span>
                          <span className="font-semibold text-[var(--text-primary)]">{task.task_name}</span>
                        </div>
                        <p className="text-[11px] text-[var(--text-secondary)]">
                          Index {task.task_index} - Exit {task.exit_code ?? '—'}
                        </p>
                      </div>
                      <span className="text-[11px] text-[var(--text-secondary)]">{timeAgo(task.started_at)}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function buildGroupPath(groupId: number | null, groups: Group[]): Group[] {
  if (!groupId) return [];
  const map = new Map<number, Group>();
  groups.forEach(group => map.set(group.id, group));
  const path: Group[] = [];
  let current = map.get(groupId) || null;
  const visited = new Set<number>();
  while (current && !visited.has(current.id)) {
    visited.add(current.id);
    path.unshift(current);
    const parentId = current.parent_id ?? null;
    current = parentId ? map.get(parentId) || null : null;
  }
  return path;
}

function getStatusDotClass(status: string | undefined, complete?: boolean) {
  const normalized = normalizeStatus(status, complete);
  if (normalized === 'success') return 'bg-emerald-400';
  if (normalized === 'failure') return 'bg-red-500';
  if (normalized === 'failure (ignored)') return 'bg-amber-500';
  if (normalized === 'running') return 'bg-blue-400';
  if (normalized === 'cancelled') return 'bg-orange-400';
  if (normalized === 'skipped') return 'bg-slate-400';
  return 'bg-gray-500';
}

function runTimestamp(run?: RunListItem) {
  if (!run) return 0;
  const value = run.started_at || run.finished_at;
  if (!value) return 0;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? 0 : date.getTime();
}

function buildStatusTimeline(runs: RunListItem[], limit = 36) {
  const sorted = [...runs].sort((a, b) => runTimestamp(b) - runTimestamp(a));
  return sorted.slice(0, limit).map((run, index) => ({
    key: run.run_id || `${run.trigger_event_id || 'run'}-${index}`,
    status: normalizeStatus(run.status, run.is_complete),
  }));
}

function getBranchStatusTone(status: string) {
  const normalized = normalizeStatus(status, true);
  if (normalized === 'success') return 'text-green-400';
  if (normalized === 'failure' || normalized === 'failure (ignored)') return 'text-red-400';
  if (normalized === 'running') return 'text-blue-400';
  return 'text-slate-300';
}

function normalizeStatus(status: string | undefined, complete?: boolean): string {
  const raw = (status || '').toLowerCase();
  if (!complete && raw !== 'success' && raw !== 'failure' && raw !== 'cancelled' && raw !== 'skipped') return 'running';
  if (STATUS_META[raw]) return raw;
  return 'pending';
}

function getStatusMeta(status: string | undefined, complete?: boolean) {
  const normalized = normalizeStatus(status, complete);
  return STATUS_META[normalized] || STATUS_META.pending;
}

function runMatchesSearch(run: RunListItem, term: string): boolean {
  if (!term) return true;
  const haystack = [
    run.pipeline_name,
    run.pipeline_path,
    run.git_repo_name,
    run.git_repo_owner,
    run.git_ref,
    run.git_target_ref,
    run.git_commit_sha,
    run.git_pusher_name,
    run.status,
    run.trigger_event_id,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  return haystack.includes(term);
}

function formatBranch(ref?: string) {
  if (!ref) return '—';
  return ref.replace(/^refs\/heads\//, '');
}

function formatBranchDisplay(source?: string, target?: string) {
  const sourceBranch = formatBranch(source);
  const targetBranch = formatBranch(target);
  if (targetBranch && targetBranch !== '—') {
    return `${sourceBranch} -> ${targetBranch}`;
  }
  return sourceBranch;
}

function formatRepo(run: RunListItem) {
  const owner = run.git_repo_owner || '';
  const name = run.git_repo_name || '';
  if (owner && name) return `${owner}/${name}`;
  return name || owner || 'Repository';
}

function getPipelineIdentifier(run?: Pick<RunListItem, 'pipeline_name' | 'pipeline_path'> | ParentRunInfo | null) {
  if (!run) return '';
  const name = (run.pipeline_name || '').trim();
  const path = (run.pipeline_path || '').trim().replace(/^\/+|\/+$/g, '');
  if (!name) return '';
  return path ? `${path}/${name}` : name;
}

function buildPipelineLink(run?: Pick<RunListItem, 'pipeline_name' | 'pipeline_path'> | ParentRunInfo | null) {
  const identifier = getPipelineIdentifier(run);
  if (!identifier) return '';
  const encoded = identifier
    .split('/')
    .map(segment => encodeURIComponent(segment))
    .join('/');
  return `/pipelines/${encoded}`;
}

function timeAgo(dateInput?: string) {
  if (!dateInput) return '—';
  const date = new Date(dateInput);
  if (Number.isNaN(date.getTime())) return '—';
  const diff = Date.now() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function formatTriggerId(id?: string) {
  if (!id) return { display: 'N/A', full: 'N/A' };
  const full = String(id);
  const display = full.length > 12 ? `${full.slice(0, 8)}...` : full;
  return { display, full };
}

function summarizeStatus(runs: RunListItem[]): string {
  if (!runs.length) return 'pending';
  const ranked = runs
    .map(run => normalizeStatus(run.status, run.is_complete))
    .sort((a, b) => STATUS_PRIORITY.indexOf(a) - STATUS_PRIORITY.indexOf(b));
  return ranked[0] || 'pending';
}

function parseLogLine(line: string): { level?: string; step?: string } {
  if (!line) return {};
  try {
    const jsonStart = line.indexOf('{');
    if (jsonStart !== -1) {
      const parsed = JSON.parse(line.slice(jsonStart));
      const level = (parsed.level || parsed.meta?.level || '').toString().toLowerCase();
      const step = parsed.step || parsed.step_name || parsed.meta?.step || parsed.meta?.step_name || undefined;
      return { level: level || undefined, step: step || undefined };
    }
  } catch {
    // ignore parse errors
  }
  const levelMatch = line.match(/\b(info|warn|error|debug)\b/i);
  return { level: levelMatch ? levelMatch[1].toLowerCase() : undefined };
}

function buildEdgePath(x1: number, y1: number, x2: number, y2: number) {
  const delta = Math.max(40, Math.abs(x2 - x1) * 0.5);
  return `M ${x1} ${y1} C ${x1 + delta} ${y1}, ${x2 - delta} ${y2}, ${x2} ${y2}`;
}

function extractLatestRunSummary(runsByBranch: Record<string, RunListItem[]> | null): RepoSummary | null {
  if (!runsByBranch) return null;
  let latest: RunListItem | null = null;
  let branchName = '';
  Object.entries(runsByBranch).forEach(([branch, runs]) => {
    runs.forEach((run: RunListItem) => {
      if (!latest) {
        latest = run;
        branchName = branch;
        return;
      }
      const currentTime = latest.started_at ? new Date(latest.started_at).getTime() : 0;
      const candidateTime = run.started_at ? new Date(run.started_at).getTime() : 0;
      if (candidateTime > currentTime) {
        latest = run;
        branchName = branch;
      }
    });
  });
  if (!latest) return null;
  const resolved = latest as RunListItem;
  return {
    status: normalizeStatus(resolved.status, resolved.is_complete),
    branch: branchName,
    commit: (resolved.git_commit_sha || '').slice(0, 7),
    pusher: resolved.git_pusher_name || '',
    started_at: resolved.started_at,
  };
}

export default PipelineRunsPage;
