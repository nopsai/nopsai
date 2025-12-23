import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { Link, NavLink, useParams, useSearchParams } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';

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
  goal?: string;
  script?: string;
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
  started_at?: string;
  finished_at?: string;
  configuration?: StepConfiguration;
};

type PipelineDefinition = {
  name?: string;
  description?: string;
  version?: string;
  steps?: {
    name: string;
    description?: string;
    depends_on?: string[];
    tasks?: TaskDefinition[];
    goal?: string;
    script?: string;
  }[];
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
    icon: 'M21 12a9 9 0 11-6.219-8.56',
    strokeClass: 'text-blue-500 animate-pulse',
    border: 'border-blue-500/60',
    bg: 'fill-blue-100 dark:fill-blue-900/50 stroke-blue-500',
  },
  pending: {
    text: 'Pending',
    pillClass: 'bg-gray-100 text-gray-700 border-gray-200 dark:bg-gray-800/40 dark:text-gray-200 dark:border-gray-700',
    icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
    strokeClass: 'text-gray-500',
    border: 'border-gray-500/60',
    bg: 'fill-gray-100 dark:fill-gray-800 stroke-gray-500',
  },
  skipped: {
    text: 'Skipped',
    pillClass: 'bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-800/60 dark:text-slate-200 dark:border-slate-700',
    icon: 'M6 12h12M12 3a9 9 0 110 18 9 9 0 010-18z',
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
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [newFolderError, setNewFolderError] = useState<string | null>(null);
  const [newFolderPending, setNewFolderPending] = useState(false);
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(new Set());
  const [repoSummaries, setRepoSummaries] = useState<Map<number, RepoSummary>>(new Map());

  const [runDetail, setRunDetail] = useState<RunDetail | null>(null);
  const [runDetailLoading, setRunDetailLoading] = useState(false);
  const [runDetailError, setRunDetailError] = useState<string | null>(null);
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
  }, []);

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
    const text = await response.text();
    if (!text) return undefined as T;
    try {
      return JSON.parse(text) as T;
    } catch {
      return text as unknown as T;
    }
  }, []);

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
      const hasSearch = Boolean(searchTerm.trim());
      if (activeTab === 'main' && activeGroupId) {
        const data = await fetchJson<Record<string, RunListItem[]>>(`/v1/runs?groupId=${activeGroupId}`);
        setRunsByBranch(data || {});
      } else if (activeTab === 'main' && hasSearch) {
        setRunsByBranch({});
        await fetchRecentPage(0, { replace: true });
      } else if (activeTab === 'main') {
        setRunsByBranch({});
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
      const detail = await fetchJson<RunDetail>(`/v1/runs/${encodeURIComponent(activeRunId)}`);
      setRunDetail(detail);
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
  const activeGroupLabel = activeGroupPath.length ? activeGroupPath[activeGroupPath.length - 1].name : 'Root';

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

  const handleNewFolder = useCallback(() => {
    setNewFolderError(null);
    setNewFolderOpen(true);
  }, []);

  const submitNewFolder = useCallback(
    async (name: string, description: string) => {
      const trimmedName = name.trim();
      const trimmedDescription = description.trim();
      if (!trimmedName) {
        setNewFolderError('Folder name is required.');
        return;
      }
      setNewFolderPending(true);
      setNewFolderError(null);
      try {
        await fetchJson('/v1/groups', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: trimmedName, description: trimmedDescription || undefined, parent_id: activeGroupId }),
        });
        setNewFolderOpen(false);
        setNewFolderPending(false);
        await loadGroups();
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to create folder';
        setNewFolderError(message);
        setNewFolderPending(false);
      }
    },
    [activeGroupId, fetchJson, loadGroups]
  );

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
      scrollMainToTop();
    },
    [scrollMainToTop, updateSearchParams]
  );

  const isViewingDetail = Boolean(runDetail && activeRunId);
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
            {!isViewingDetail && (
              <div className="flex items-center gap-2 flex-shrink-0 order-1 sm:order-2">
                {activeTab === 'recent' && <ViewToggle viewMode={viewMode} onChange={setViewMode} />}
                <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
                  <button
                    type="button"
                    className="pipelines-search-toggle"
                    aria-label="Search pipeline runs"
                    onClick={() => {
                      setSearchOpen(true);
                      requestAnimationFrame(() => searchInputRef.current?.focus());
                    }}
                  >
                    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 21l-4.35-4.35M10 18a8 8 0 110-16 8 8 0 010 16z" />
                    </svg>
                  </button>
                  <input
                    ref={searchInputRef}
                    id="pipeline-runs-search"
                    type="text"
                    placeholder="Search runs"
                    className="pipelines-search-input"
                    value={searchTerm}
                    onChange={event => {
                      setSearchTerm(event.target.value);
                      if (event.target.value && !searchOpen) setSearchOpen(true);
                      updateSearchParams({ q: event.target.value || null });
                    }}
                    onBlur={() => {
                      if (!searchTerm.trim()) setSearchOpen(false);
                    }}
                  />
                  {(searchTerm || searchOpen) && (
                    <button
                      type="button"
                      className="pipelines-search-clear"
                      onClick={() => {
                        setSearchTerm('');
                        setSearchOpen(false);
                        updateSearchParams({ q: null });
                        searchInputRef.current?.blur();
                      }}
                      aria-label="Clear search"
                    >
                      ✕
                    </button>
                  )}
                </div>
                {activeTab === 'main' && (
                  <button
                    type="button"
                    className="pipelines-icon-only"
                    onClick={handleNewFolder}
                    aria-label="New Folder"
                    disabled={Boolean(trimmedSearch)}
                    title={trimmedSearch ? 'Clear search to create a folder' : 'New Folder'}
                  >
                    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 5v14M5 12h14" />
                    </svg>
                  </button>
                )}
              </div>
            )}
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
                setLogsSearchFilter(null);
                setLogsOpen(true);
              }}
              onOpenTaskLogs={(stepName, taskName) => {
                setSelectedStep(stepName);
                setLogsStepFilter(stepName);
                setLogsSearchFilter(taskName);
                setLogsOpen(true);
              }}
              onOpenStepDetail={stepName => {
                setStepDetailName(stepName);
              }}
              onOpenRun={handleOpenRun}
              onShowDefinition={() => setDefinitionOpen(true)}
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
              onDeleteBranch={handleDeleteBranch}
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

      {logsOpen && activeRunId && (
        <LogsModal
          runId={activeRunId}
          runName={runDetail?.run_info.pipeline_name}
          onClose={() => {
            setLogsOpen(false);
            setLogsStepFilter(null);
            setLogsSearchFilter(null);
          }}
          steps={runDetail?.steps}
          stepNames={runDetail?.steps.map(step => step.name)}
          initialStep={logsStepFilter}
          initialSearch={logsSearchFilter}
        />
      )}

      {stepDetailName && runDetail && (
      <StepDetailModal
        step={runDetail.steps.find(step => step.name === stepDetailName) || null}
        onClose={() => setStepDetailName(null)}
        onViewLogs={() => {
          setLogsStepFilter(stepDetailName);
          setLogsSearchFilter(null);
          setLogsOpen(true);
        }}
        pipelineDefinition={runDetail.pipeline_definition}
      />
    )}

      {newFolderOpen && (
        <NewFolderModal
          open={newFolderOpen}
          parentLabel={activeGroupLabel}
          error={newFolderError}
          pending={newFolderPending}
          onClose={() => {
            setNewFolderOpen(false);
            setNewFolderError(null);
            setNewFolderPending(false);
          }}
          onSubmit={submitNewFolder}
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
  onDeleteBranch,
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
  onToggleBranch: (branch: string, scrollIntoView?: boolean) => void;
  onDeleteBranch: (branch: string) => void;
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

  const repoGroups = useMemo(() => visibleGroups.filter(group => group.name.includes('/')), [visibleGroups]);
  const folderGroups = useMemo(() => visibleGroups.filter(group => !group.name.includes('/')), [visibleGroups]);

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
    const hasSearch = Boolean(term);
    const mainSearchRuns = hasSearch ? recentRuns : [];

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

        {hasSearch ? (
          <RunCollection runs={mainSearchRuns} viewMode={viewMode} onOpenRun={onOpenRun} onSelectRun={onSelectRun} selectedRunIds={selectedRunIds} />
        ) : groupsLoading && !groups.length ? (
          <div className="text-sm text-[var(--text-secondary)]">Loading folders…</div>
        ) : (
          <div className="space-y-4">
            <GroupGrid
              groups={repoGroups}
              allGroups={groups}
              activeGroupId={activeGroupId}
              repoSummaries={repoSummaries}
              onSelect={onSelectGroup}
              onDelete={onDeleteFolder}
            />
            <GroupGrid
              groups={folderGroups}
              allGroups={groups}
              activeGroupId={activeGroupId}
              repoSummaries={repoSummaries}
              onSelect={onSelectGroup}
              onDelete={onDeleteFolder}
            />
          </div>
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
                  onDeleteBranch={() => onDeleteBranch(branch)}
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
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
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

function ZapIcon({ className }: { className?: string }) {
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
      <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
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
          commitLabel: newest?.git_commit_sha ? newest.git_commit_sha.slice(0, 8) : undefined,
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
    <div className="grid grid-cols-1 sm:grid-cols-4 lg:grid-cols-4 gap-4">
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
      <div className="p-4 grid gap-3 sm:grid-cols-4 xl:grid-cols-4">
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
  const repoLabel = formatRepoLabel(run);
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
      data-trigger-id={run.trigger_event_id || ''}
      data-run-id={run.run_id}
    >
      <div className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div className="flex-1 min-w-0 pr-2">
            <div className="flex items-center gap-2 min-w-0">
              <RunStatusIcon status={run.status} complete={run.is_complete} />
              <div className="min-w-0">
                <p className="text-sm font-semibold text-[var(--text-primary)] truncate">{run.pipeline_name}</p>
                <p className="text-[11px] font-mono text-[var(--text-secondary)] truncate flex items-center gap-1">
                  <RunIdIcon className="h-3.5 w-3.5 flex-shrink-0" />
                  <span>{(run.run_id || 'N/A').slice(0, 8)}</span>
                </p>
                <div className="flex items-center gap-3 text-xs text-[var(--text-secondary)] mt-1 flex-wrap">
                </div>
              </div>
            </div>
          </div>
          <PipelineBadges run={run} />
        </div>
        <div className="text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="8" cy="7" r="2" />
              <circle cx="8" cy="17" r="2" />
              <circle cx="16" cy="7" r="2" />
              <path d="M10 7h4" />
              <path d="M8 9v6a4 4 0 004 4h4" />
            </svg>
            <span className="truncate" title="Repository">{repoLabel}</span>
          </div>
          <div className="flex items-center">
            <BranchIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
            <span className="truncate" title="Branch">{branchLabel || 'N/A'}</span>
          </div>
          <div className="flex items-center">
            <svg className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
            <span className="truncate">{run.git_pusher_name || 'N/A'}</span>
          </div>          
          <div className="flex items-center">
            <CommitIcon className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" />
            <span className="truncate" title="Commit Hash">{(run.git_commit_sha || 'N/A').slice(0, 8)}</span>
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
  const repoLabel = formatRepoLabel(run);
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const commitLabel = (run.git_commit_sha || 'N/A').slice(0, 8);
  const runIdLabel = (run.run_id || 'N/A').slice(0, 8);
  return (
    <div
      className={`run-card run-card--list border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm rounded-2xl hover:border-[var(--border-accent)] ${selected ? 'run-link-highlight' : ''}`}
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={event => {
        if (event.key === 'Enter') onOpen();
      }}
      data-trigger-id={run.trigger_event_id || ''}
      data-run-id={run.run_id}
    >
      <div className="run-list-cell run-list-cell--main">
        <span className="run-list-icon">
          <RunStatusIcon status={run.status} complete={run.is_complete} />
        </span>
        <div className="run-list-main">
          <div className="run-list-title-row">
            <div className="run-list-title truncate" title={run.pipeline_name}>
              {run.pipeline_name}
            </div>
            <PipelineBadges run={run} />
          </div>
          <div className="run-list-chips">
            <span className="run-list-chip" title={repoLabel}>
              <svg className="h-3.5 w-3.5 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="8" cy="7" r="2" />
                <circle cx="8" cy="17" r="2" />
                <circle cx="16" cy="7" r="2" />
                <path d="M10 7h4" />
                <path d="M8 9v6a4 4 0 004 4h4" />
              </svg>
              <span className="truncate">{repoLabel}</span>
            </span>
            <span className="run-list-chip" title={branchLabel || 'N/A'}>
              <BranchIcon className="h-3.5 w-3.5 flex-shrink-0" />
              <span className="truncate">{branchLabel || 'N/A'}</span>
            </span>
            <span className="run-list-chip font-mono" title={`Run ${run.run_id || 'N/A'}`}>
              <RunIdIcon className="h-3.5 w-3.5 flex-shrink-0" />
              {runIdLabel}
            </span>
          </div>
        </div>
      </div>
      <div className="run-list-cell">
        <span className="run-list-meta-label">Commit</span>
        <span className="run-list-meta-value font-mono">{commitLabel}</span>
      </div>
      <div className="run-list-cell">
        <span className="run-list-meta-label">Trigger</span>
        <span className="run-list-meta-value truncate" title={triggerLabel.full}>
          {triggerLabel.display}
        </span>
      </div>
      <div className="run-list-cell">
        <span className="run-list-meta-label">Updated</span>
        <span className="run-list-meta-value">{timeAgo(timeToDisplay)}</span>
      </div>
      <div className="run-list-cell run-list-cell--actions">
        <RunSelectToggle selected={selected} onToggle={onSelect} />
      </div>
    </div>
  );
}

function RunStatusIcon({ status, complete }: { status: string; complete?: boolean }) {
  return <BranchStatusIcon status={status} complete={complete} className="run-status-icon" />;
}

function RunIdIcon({ className }: { className?: string }) {
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
      <path d="M4 7h4v10H4z" />
      <path d="M12 7h8" />
      <path d="M12 12h8" />
      <path d="M12 17h8" />
    </svg>
  );
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
          <circle cx="12" cy="12" r="10" />
          <path d="M6 12h12" />
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
        Overridden
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
  onOpenTaskLogs,
  onOpenStepDetail,
  onOpenRun,
  onShowDefinition,
}: {
  detail: RunDetail;
  loading: boolean;
  error: string | null;
  onClose: () => void;
  onCancel: () => void;
  onRerun: () => void;
  onDelete: () => void;
  selectedStep: string | null;
  onSelectStep: (step: string | null) => void;
  onOpenLogs: () => void;
  onOpenTaskLogs: (stepName: string, taskName: string) => void;
  onOpenStepDetail: (stepName: string) => void;
  onOpenRun: (id: string) => void;
  onShowDefinition: () => void;
}) {
  const run = detail.run_info;
  const isRunning = normalizeStatus(run.status, run.is_complete) === 'running';
  const normalizedStatus = normalizeStatus(run.status, run.is_complete);
  const pipelineLink = buildPipelineLink(run);
  const triggerLabel = formatTriggerId(run.trigger_event_id);
  const parentRun = detail.parent_run_info;

  const actionBase =
    'inline-flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-semibold transition duration-150 focus:outline-none';
  const ghostAction = `${actionBase} border border-[var(--border-primary)]/80 bg-[var(--bg-secondary)] text-[var(--text-primary)] shadow-[0_10px_30px_rgba(0,0,0,0.08)] hover:border-indigo-300/60 hover:text-indigo-600 dark:border-white/10 dark:bg-white/5 dark:text-white dark:shadow-[0_10px_30px_rgba(0,0,0,0.25)] dark:hover:border-indigo-300/50 dark:hover:bg-white/10`;
  const primaryAction = `${actionBase} bg-gradient-to-r from-indigo-500 to-purple-500 text-white shadow-[0_14px_34px_rgba(79,70,229,0.25)] hover:shadow-[0_18px_44px_rgba(79,70,229,0.32)] focus:ring-2 focus:ring-offset-2 focus:ring-indigo-400`;
  const dangerAction = `${actionBase} border border-red-500/40 text-red-600 bg-red-50 hover:bg-red-100 dark:text-red-100 dark:bg-red-500/10 dark:hover:bg-red-500/20`;
  const iconDanger = 'inline-flex items-center justify-center h-11 w-11 rounded-xl p-0 text-red-600 hover:text-red-700 dark:text-red-200 dark:hover:text-red-100 bg-transparent border-none shadow-none';

  const startedLabel = run.started_at ? timeAgo(run.started_at) : '—';
  const branchLabel = formatBranchDisplay(run.git_ref, run.git_target_ref);
  const repoLabel = formatRepoLabel(run);

  const detailLines = [
    {
      label: 'Run ID',
      value: run.run_id || '—',
      subtext: run.duration ? `${run.duration} elapsed` : 'Elapsed: —',
      icon: <RunIdIcon className="h-4 w-4 text-slate-500" />,
    },
    {
      label: 'Commit',
      value: run.git_commit_sha || '—',
      subtext: run.git_pusher_name ? `Committer: ${run.git_pusher_name}` : 'Committer: —',
      icon: <CommitIcon className="h-4 w-4 text-slate-500" />,
    },
    {
      label: 'Trigger Event ID',
      value: triggerLabel.full || '—',
      subtext: run.started_at ? `Started ${startedLabel}` : 'Started: —',
      icon: <ZapIcon className="h-4 w-4 text-slate-500" />,
    },
  ];


  const renderHeroStatus = () => {
    const pulseClasses = 'relative flex h-2.5 w-2.5';
    const baseCircle = 'absolute inline-flex h-full w-full rounded-full opacity-60';
    if (normalizedStatus === 'success') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-green-500/10 border border-green-500/30 text-green-700 dark:text-green-200 text-xs font-semibold">
          <span className={pulseClasses}>
            <span className={`${baseCircle} animate-ping bg-green-400`} />
            <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-green-500" />
          </span>
          Success
        </span>
      );
    }
    if (normalizedStatus === 'running') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-blue-500/10 border border-blue-500/30 text-blue-700 dark:text-blue-200 text-xs font-semibold">
          <svg className="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="10" className="opacity-30" />
            <path d="M12 2a10 10 0 0110 10" />
          </svg>
          Running
        </span>
      );
    }
    if (normalizedStatus === 'failure') {
      return (
        <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-red-500/10 border border-red-500/30 text-red-700 dark:text-red-200 text-xs font-semibold">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M6 18L18 6M6 6l12 12" />
          </svg>
          Failed
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-slate-500/10 border border-slate-300 text-slate-700 dark:text-slate-200 text-xs font-semibold">
        <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 8v4l3 3" />
          <circle cx="12" cy="12" r="10" />
        </svg>
        {getStatusMeta(normalizedStatus, run.is_complete).text}
      </span>
    );
  };

  return (
    <div className="space-y-6">
      <div className="rounded-3xl border border-[var(--border-primary)] bg-white text-[var(--text-primary)] shadow-[0_22px_60px_rgba(8,10,24,0.12)] dark:border-white/10 dark:bg-gradient-to-br from-[#0b0c15] via-[#0c0f1f] to-[#0b0c15] dark:text-white dark:shadow-[0_22px_60px_rgba(8,10,24,0.5)] overflow-hidden">
        <div className="p-6 flex flex-col gap-6">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-3 flex-wrap">
                <span className="text-3xl font-black tracking-tight text-[var(--text-primary)] dark:text-white">{run.pipeline_name}</span>
                {parentRun && (
                  <button type="button" className={`${ghostAction} px-3 py-1.5 text-xs`} onClick={() => onOpenRun(parentRun.run_id)}>
                    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M5 12h14" />
                      <path d="M12 5l7 7-7 7" />
                    </svg>
                    Parent: {parentRun.pipeline_name}
                  </button>
                )}
                {renderHeroStatus()}
                {run.pipeline_source && (
                  <span className="runner-pill runner-pill--muted capitalize bg-[var(--bg-secondary)] text-[var(--text-primary)] border-[var(--border-primary)] dark:bg-white/10 dark:text-white dark:border-white/20">
                    {run.pipeline_source}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-3 text-sm text-[var(--text-secondary)]">
                <span className="inline-flex items-center gap-2 min-w-0">
                  <svg className="h-4 w-4 text-[var(--text-secondary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <circle cx="8" cy="7" r="2" />
                    <circle cx="8" cy="17" r="2" />
                    <circle cx="16" cy="7" r="2" />
                    <path d="M10 7h4" />
                    <path d="M8 9v6a4 4 0 004 4h4" />
                  </svg>
                  <span className="font-medium text-[var(--text-primary)] dark:text-white truncate max-w-xs" title={repoLabel}>
                    {repoLabel}
                  </span>
                </span>
                <span className="text-[var(--border-primary)]">/</span>
                <span className="inline-flex items-center gap-2 min-w-0">
                  <BranchIcon className="h-4 w-4 text-[var(--text-secondary)]" />
                  <span className="font-mono text-[var(--text-primary)] dark:text-white break-words" title={branchLabel || undefined}>
                    {branchLabel || '—'}
                  </span>
                </span>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-2">
                <button className={ghostAction} type="button" onClick={onOpenLogs}>
                  <svg className="h-4 w-4 text-current" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M9 12h6" />
                    <path d="M9 16h6" />
                    <path d="M7 8h10" />
                    <rect x="4" y="4" width="16" height="16" rx="2" ry="2" />
                  </svg>
                  Logs
                </button>
                {pipelineLink ? (
                  <Link className={ghostAction} to={pipelineLink}>
                    <svg className="h-4 w-4 text-current" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <circle cx="12" cy="12" r="2.5" />
                      <path d="M4 12h3m10 0h3M12 4v3m0 10v3" />
                    </svg>
                    Pipeline
                  </Link>
                ) : (
                  <button className={ghostAction} type="button" onClick={onShowDefinition}>
                    <svg className="h-4 w-4 text-current" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <circle cx="12" cy="12" r="2.5" />
                      <path d="M4 12h3m10 0h3M12 4v3m0 10v3" />
                    </svg>
                    Pipeline
                  </button>
                )}
              </div>
              <div className="h-6 w-px bg-[var(--border-primary)] dark:bg-white/10" />
              <button className={isRunning ? dangerAction : primaryAction} type="button" onClick={isRunning ? onCancel : onRerun} disabled={loading}>
                <svg className="h-4 w-4 text-current" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="1 4 1 10 7 10" />
                  <polyline points="23 20 23 14 17 14" />
                  <path d="M3.51 9a9 9 0 0114.13-3.36L23 10M1 14l5.36 4.36A9 9 0 0020.49 15" />
                </svg>
                {isRunning ? 'Cancel' : 'Re-run'}
              </button>
              <button className={iconDanger} type="button" onClick={onDelete} aria-label="Delete run">
                <svg className="h-4 w-4 text-current" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="3 6 5 6 21 6" />
                  <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6" />
                  <path d="M10 11v6" />
                  <path d="M14 11v6" />
                  <path d="M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2" />
                </svg>
              </button>
              <button
                className="pipelines-icon-only"
                type="button"
                onClick={onClose}
                aria-label="Close details"
                title="Close"
              >
                <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M18 6L6 18" />
                  <path d="M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <div className="flex items-start gap-6 flex-wrap justify-between">
            <div className="flex-1 min-w-[320px] space-y-6">
              <div className="grid gap-3 md:grid-cols-3 text-sm text-[var(--text-primary)] mt-4">
                {detailLines.map(item => (
                  <div
                    key={item.label}
                    className="flex flex-col gap-2 rounded-2xl border border-[var(--border-primary)] bg-white text-[var(--text-primary)] px-4 py-3 shadow-[0_12px_32px_rgba(0,0,0,0.08)] dark:bg-white/5 dark:border-white/10 dark:text-white dark:shadow-[0_12px_32px_rgba(0,0,0,0.35)] h-full"
                  >
                    <div className="flex items-center justify-between text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
                      <span className="inline-flex items-center gap-2 font-semibold">
                        {item.icon}
                        {item.label}
                      </span>
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="font-mono text-sm text-[var(--text-primary)] dark:text-white break-words whitespace-pre-wrap">{item.value}</div>
                      {item.subtext && (
                        <div className="text-xs text-[var(--text-secondary)] dark:text-slate-400 break-words whitespace-pre-wrap">{item.subtext}</div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {error && <div className="text-red-500 text-sm">{error}</div>}

      {run.failure_reason && (
        <div className="bg-red-50 dark:bg-red-900/40 border border-red-200 dark:border-red-700 text-red-700 dark:text-red-200 px-4 py-3 rounded-lg text-sm">
          Failed to start: {run.failure_reason}
        </div>
      )}

      <div className="space-y-4">
        <div className="rounded-2xl border border-[var(--border-primary)] bg-white dark:bg-slate-950 shadow-[0_16px_44px_rgba(15,23,42,0.07)] p-2">
          <StepsGraph
            steps={detail.steps}
            selectedStep={selectedStep}
            onSelectStep={onSelectStep}
            onOpenTaskLogs={onOpenTaskLogs}
            onOpenStepDetail={onOpenStepDetail}
            childRuns={detail.child_runs}
            pipelineDefinition={detail.pipeline_definition}
          />
        </div>
      </div>

      {detail.child_runs?.length > 0 && (
        <div className="border border-[var(--border-primary)] rounded-2xl bg-white dark:bg-slate-950 p-4 space-y-2 shadow-sm">
          <h3 className="font-semibold text-[var(--text-primary)]">Child runs</h3>
          <div className="space-y-3">
            {detail.child_runs.map(child => (
              <div key={child.run_id} className="flex items-center justify-between text-sm p-3 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
                <div className="flex items-center gap-2">
                  <StatusBadge status={child.status} complete={child.is_complete} />
                  <span className="font-medium text-[var(--text-primary)]">{child.pipeline_name}</span>
                  {child.parent_step_name && <span className="runner-pill runner-pill--muted">Step {child.parent_step_name}</span>}
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-[var(--text-secondary)]">{timeAgo(child.started_at)}</span>
                  <button className={ghostAction} type="button" onClick={() => onOpenRun(child.run_id)}>
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

type GraphStatus = 'success' | 'failed' | 'running' | 'pending' | 'skipped';

type GraphPoint = { x: number; y: number };
type GraphSize = { width: number; height: number };
type GraphLayoutNode<T> = GraphPoint & GraphSize & { data: T; level: number };
type GraphLayoutEdge = { id: string; from: string; to: string; points: GraphPoint[]; status: GraphStatus };
type GraphLayout<T> = { nodes: GraphLayoutNode<T>[]; edges: GraphLayoutEdge[]; width: number; height: number };

type GraphTask = {
  id: string;
  name: string;
  status: GraphStatus;
  duration?: string;
  dependsOn?: string[];
};

type GraphStep = {
  id: string;
  name: string;
  status: GraphStatus;
  duration?: string;
  dependsOn?: string[];
  tasks: GraphTask[];
  includeLabel?: string;
  childRun?: RunListItem | null;
};

type TaskGraphLayout = GraphLayout<GraphTask> & {
  orientation: 'horizontal' | 'vertical';
  taskCount: number;
  dependencyCount: number;
};

const STEP_WIDTH_CLOSED = 190;
const STEP_HEIGHT_CLOSED = 56;
const TASK_MIN_WIDTH = 160;
const TASK_MAX_WIDTH = 280;
const TASK_HEIGHT = 24;
const H_GAP = 76;
const V_GAP = 26;
const PADDING = 32;
const STEP_HEADER_HEIGHT = 44;
const INNER_PADDING = 12;
const MIN_GRAPH_SCALE = 0.4;
const MAX_GRAPH_SCALE = 1.4;

function StepsGraph({
  steps,
  selectedStep,
  onSelectStep,
  onOpenTaskLogs,
  onOpenStepDetail,
  childRuns,
  pipelineDefinition,
  statusVariant = 'default',
  hideStatusLegend = false,
  statusColorOverride,
  stepStatusColorOverride,
  taskStatusColorOverride = '#60a5fa',
}: {
  steps: StepDetail[];
  selectedStep: string | null;
  onSelectStep: (name: string | null) => void;
  onOpenTaskLogs?: (stepName: string, taskName: string) => void;
  onOpenStepDetail?: (stepName: string) => void;
  childRuns: RunListItem[];
  pipelineDefinition?: PipelineDefinition;
  statusVariant?: StatusGlyphVariant;
  hideStatusLegend?: boolean;
  statusColorOverride?: string;
  stepStatusColorOverride?: string;
  taskStatusColorOverride?: string;
}) {
  const [expandedSteps, setExpandedSteps] = useState<Set<string>>(new Set());
  const [transform, setTransform] = useState({ x: 0, y: 0, k: 1 });
  const [preview, setPreview] = useState<{ step: GraphStep; x: number; y: number } | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [startPan, setStartPan] = useState({ x: 0, y: 0 });
  const containerRef = useRef<HTMLDivElement | null>(null);
  const interactedRef = useRef(false);
  const prevStepStatusesRef = useRef<Map<string, GraphStatus>>(new Map());

  useEffect(() => {
    if (!selectedStep) return undefined;
    const frame = requestAnimationFrame(() =>
      setExpandedSteps(prev => {
        if (prev.has(selectedStep)) return prev;
        const next = new Set(prev);
        next.add(selectedStep);
        return next;
      })
    );
    return () => cancelAnimationFrame(frame);
  }, [selectedStep]);

  const stepDefMap = useMemo(() => {
    const map = new Map<string, StepConfiguration>();
    (pipelineDefinition?.steps || []).forEach(step => map.set(step.name, step));
    return map;
  }, [pipelineDefinition]);

  const childRunMap = useMemo(() => {
    const map = new Map<string, RunListItem>();
    childRuns.forEach(run => {
      if (run.parent_step_name) map.set(run.parent_step_name, run);
    });
    return map;
  }, [childRuns]);

  const getStepBaseWidth = useCallback((step: GraphStep) => Math.max(STEP_WIDTH_CLOSED, Math.min(360, step.name.length * 8 + 120)), []);

  const graphSteps = useMemo<GraphStep[]>(() => {
    return steps.map(step => {
      const stepDef = stepDefMap.get(step.name);
      const includeLabel = step.configuration?.include
        ? `Included ${step.configuration.include.toLowerCase().includes('pipeline') ? 'Pipeline' : 'Step'}`
        : '';
      const tasks: GraphTask[] = (step.tasks || []).map(task => {
        const def = stepDef?.tasks?.find(t => t.name === task.task_name);
        return {
          id: task.task_name,
          name: task.task_name,
          status: normalizeGraphStatus(task.status, task.status === 'success'),
          duration: formatElapsedLabel(task.started_at, task.finished_at),
          dependsOn: def?.depends_on || [],
        };
      });
      return {
        id: step.name,
        name: step.name,
        status: normalizeGraphStatus(step.status, step.status === 'success'),
        duration: formatStepDuration(step),
        dependsOn: step.depends_on || [],
        tasks,
        includeLabel,
        childRun: childRunMap.get(step.name) || null,
      };
    });
  }, [childRunMap, stepDefMap, steps]);

  const expandedLayouts = useMemo(() => {
    const map = new Map<string, GraphLayout<GraphTask>>();
    const taskSize = (task: GraphTask) => {
      const label = `${task.name} - ${task.duration || '0s'}`;
      const width = Math.max(TASK_MIN_WIDTH, Math.min(TASK_MAX_WIDTH, 32 + label.length * 7));
      return { width, height: TASK_HEIGHT };
    };
    graphSteps.forEach(step => {
      if (!expandedSteps.has(step.id)) return;
      if (!step.tasks.length) return;
      const innerLayout = calculateGraphLayout<GraphTask>(step.tasks, taskSize, 30, 16);
      map.set(step.id, innerLayout);
    });
    return map;
  }, [expandedSteps, graphSteps]);

  const mainLayout = useMemo(
    () =>
      calculateGraphLayout<GraphStep>(
        graphSteps,
        step => {
          const inner = expandedLayouts.get(step.id);
          const baseWidth = getStepBaseWidth(step);
          if (inner) {
            return {
              width: Math.max(baseWidth, inner.width + INNER_PADDING * 2),
              height: Math.max(STEP_HEIGHT_CLOSED, inner.height + STEP_HEADER_HEIGHT + INNER_PADDING * 2),
            };
          }
          return { width: baseWidth, height: STEP_HEIGHT_CLOSED };
        },
        H_GAP,
        V_GAP
      ),
    [expandedLayouts, getStepBaseWidth, graphSteps]
  );

  const fitGraphToViewport = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const { clientWidth, clientHeight } = container;
    if (!clientWidth || !clientHeight || !mainLayout.width || !mainLayout.height) return;
    const padding = 32;
    const scaleX = (clientWidth - padding * 2) / mainLayout.width;
    const scaleY = (clientHeight - padding * 2) / mainLayout.height;
    const nextScale = Math.min(MAX_GRAPH_SCALE, Math.max(MIN_GRAPH_SCALE, Math.min(scaleX, scaleY)));
    const nextX = (clientWidth - mainLayout.width * nextScale) / 2;
    const nextY = (clientHeight - mainLayout.height * nextScale) / 2;
    setTransform({ x: nextX, y: nextY, k: nextScale });
  }, [mainLayout.height, mainLayout.width]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    if (interactedRef.current) return;
    const nextX = (container.clientWidth - mainLayout.width) / 2;
    const nextY = (container.clientHeight - mainLayout.height) / 2;
    if (Number.isFinite(nextX) && Number.isFinite(nextY)) {
      setTransform(prev => ({ ...prev, x: nextX, y: nextY }));
    }
  }, [mainLayout.height, mainLayout.width]);

  const pendingFitRef = useRef(false);

  useEffect(() => {
    const nextStatusMap = new Map<string, GraphStatus>();
    graphSteps.forEach(step => {
      nextStatusMap.set(step.id, step.status);
    });
    prevStepStatusesRef.current = nextStatusMap;
  }, [graphSteps]);

  useEffect(() => {
    if (pendingFitRef.current) {
      pendingFitRef.current = false;
      fitGraphToViewport();
    }
  }, [expandedLayouts, expandedSteps, fitGraphToViewport, mainLayout.height, mainLayout.width]);

  const toggleStep = useCallback(
    (id: string) => {
      pendingFitRef.current = true;
      setExpandedSteps(prev => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
      onSelectStep(id);
    },
    [onSelectStep]
  );
  const expandAll = useCallback(() => {
    pendingFitRef.current = true;
    setExpandedSteps(new Set(steps.map(step => step.name)));
  }, [steps]);
  const collapseAll = useCallback(() => {
    pendingFitRef.current = true;
    setExpandedSteps(new Set());
  }, []);

  const handleWheel = useCallback((event: React.WheelEvent | WheelEvent) => {
    interactedRef.current = true;
    event.stopPropagation();
    event.preventDefault();
    const deltaY = 'deltaY' in event ? event.deltaY : 0;
    const scaleSens = 0.001;
    setTransform(prev => {
      const nextScale = Math.min(MAX_GRAPH_SCALE, Math.max(MIN_GRAPH_SCALE, prev.k - deltaY * scaleSens));
      return { ...prev, k: nextScale };
    });
  }, []);

  const handleMouseDown = (event: React.MouseEvent) => {
    if (event.button !== 0) return;
    const target = event.target as HTMLElement;
    if (target.closest('[data-graph-node]')) return;
    interactedRef.current = true;
    setIsDragging(true);
    setStartPan({ x: event.clientX - transform.x, y: event.clientY - transform.y });
  };

  const handleMouseMove = (event: React.MouseEvent) => {
    if (!isDragging) return;
    setTransform(prev => ({ ...prev, x: event.clientX - startPan.x, y: event.clientY - startPan.y }));
  };

  const handleMouseUp = () => setIsDragging(false);

  const zoomIn = () => {
    interactedRef.current = true;
    setTransform(prev => ({ ...prev, k: Math.min(prev.k + 0.2, 3) }));
  };
  const zoomOut = () => {
    interactedRef.current = true;
    setTransform(prev => ({ ...prev, k: Math.max(prev.k - 0.2, 0.4) }));
  };

  const handleShowPreview = useCallback(
    (step: GraphStep, evt: React.MouseEvent) => {
      const rect = containerRef.current?.getBoundingClientRect();
      if (!rect) return;
      setPreview({
        step,
        x: evt.clientX - rect.left + 8,
        y: evt.clientY - rect.top - 12,
      });
    },
    []
  );

  const handleHidePreview = useCallback(() => {
    setPreview(null);
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return undefined;
    const listener = (evt: WheelEvent) => handleWheel(evt);
    el.addEventListener('wheel', listener, { passive: false });
    return () => el.removeEventListener('wheel', listener);
  }, [handleWheel]);

  const totalTasks = useMemo(() => steps.reduce((sum, step) => sum + (step.tasks?.length || 0), 0), [steps]);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 px-2 text-sm text-[var(--text-secondary)]">
        <span className="px-2.5 py-1 text-[11px] uppercase tracking-[0.08em] rounded-full bg-[var(--bg-secondary)] text-[var(--text-primary)]">
          {steps.length} step{steps.length === 1 ? '' : 's'}
        </span>
        <span className="px-2.5 py-1 text-[11px] uppercase tracking-[0.08em] rounded-full bg-[var(--bg-secondary)] text-[var(--text-primary)]">
          {totalTasks} task{totalTasks === 1 ? '' : 's'}
        </span>
      </div>

      <div
        className="relative h-[720px] w-full overflow-hidden rounded-2xl border border-[var(--border-primary)] bg-white dark:bg-slate-950 shadow-[0_16px_44px_rgba(15,23,42,0.07)]"
        ref={containerRef}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseUp}
        style={{ overscrollBehavior: 'contain' }}
      >
        {!hideStatusLegend && (
          <div className="absolute top-3 left-3 z-20 flex flex-wrap items-center gap-3 text-[11px] text-[var(--text-secondary)]">
            {(['success', 'running', 'failed', 'pending', 'skipped'] as GraphStatus[]).map(status => (
              <span key={status} className="flex items-center gap-1.5">
                <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
                  <GraphStatusGlyph status={status} x={8} y={8} size={12} />
                </svg>
                <span className="capitalize opacity-80">{getGraphStatusLabel(status)}</span>
              </span>
            ))}
          </div>
        )}

        <div className="absolute top-3 right-3 z-20 flex flex-col gap-1">
          <button
            onClick={() => {
              const allExpanded = expandedSteps.size === steps.length;
              if (allExpanded) {
                collapseAll();
              } else {
                expandAll();
                fitGraphToViewport();
              }
            }}
            className="h-9 w-9 rounded-full bg-[var(--bg-secondary)]/80 hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] shadow-sm border border-[var(--border-primary)]"
            title={expandedSteps.size === steps.length ? 'Collapse all' : 'Expand all'}
          >
            {expandedSteps.size === steps.length ? '⇱' : '⇲'}
          </button>
          <button onClick={zoomIn} className="h-9 w-9 rounded-full bg-[var(--bg-secondary)]/80 hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] shadow-sm border border-[var(--border-primary)]" title="Zoom in">
            +
          </button>
          <button onClick={zoomOut} className="h-9 w-9 rounded-full bg-[var(--bg-secondary)]/80 hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] shadow-sm border border-[var(--border-primary)]" title="Zoom out">
            −
          </button>
        </div>

        <svg width="100%" height="100%" className="cursor-grab active:cursor-grabbing">
          <g transform={`translate(${transform.x}, ${transform.y}) scale(${transform.k})`}>
            {mainLayout.edges.map(edge => {
              const [start, c1, c2, end] = edge.points;
              const color = getGraphStatusColor(edge.status);
              return (
                <g key={edge.id} className="transition-colors">
                  <path
                    d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
                    fill="none"
                    stroke={color}
                    strokeWidth={2.2}
                    strokeOpacity={0.75}
                    strokeLinecap="round"
                  />
                  {edge.status === 'running' && (
                    <path
                      d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
                      fill="none"
                      stroke="white"
                      strokeWidth={2}
                      strokeDasharray="4 8"
                      strokeOpacity={0.4}
                    >
                      <animate attributeName="stroke-dashoffset" from="12" to="0" dur="1s" repeatCount="indefinite" />
                    </path>
                  )}
                </g>
              );
            })}

      {mainLayout.nodes.map(node => (
      <StepNodeRenderer
        key={node.data.id}
        node={node}
        expanded={expandedSteps.has(node.data.id)}
        selected={selectedStep === node.data.id}
        onToggle={() => toggleStep(node.data.id)}
        onTaskClick={onOpenTaskLogs}
        onOpenDetail={onOpenStepDetail}
        onPreview={handleShowPreview}
        onPreviewEnd={handleHidePreview}
        innerLayout={expandedLayouts.get(node.data.id)}
        statusVariant={statusVariant}
        statusColorOverride={stepStatusColorOverride || statusColorOverride}
        taskStatusColorOverride={taskStatusColorOverride}
      />
      ))}
          </g>
        </svg>

        {preview && (
          <div
            className="absolute z-30 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-lg px-3 py-2 text-xs text-[var(--text-primary)] backdrop-blur-sm"
            style={{ left: preview.x, top: preview.y, pointerEvents: 'none' }}
          >
            <div className="flex items-center justify-between gap-3">
              <span className="font-semibold">{preview.step.name}</span>
              <span className="inline-flex items-center gap-1 text-[11px] text-[var(--text-secondary)]">
                <span
                  className="inline-flex h-2 w-2 rounded-full"
                  style={{ backgroundColor: getGraphStatusColor(preview.step.status) }}
                />
                {getGraphStatusLabel(preview.step.status)}
              </span>
            </div>
            <div className="mt-1 text-[11px] text-[var(--text-secondary)]">
              Duration: {preview.step.duration || '—'}
            </div>
            {preview.step.tasks?.length ? (
              <div className="mt-1 text-[11px] text-[var(--text-secondary)]">
                Tasks: {preview.step.tasks.length}
              </div>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}

function StepNodeRenderer({
  node,
  expanded,
  selected,
  onToggle,
  onTaskClick,
  onOpenDetail,
  onPreview,
  onPreviewEnd,
  innerLayout,
  statusVariant = 'default',
  statusColorOverride,
  taskStatusColorOverride,
}: {
  node: GraphLayoutNode<GraphStep>;
  expanded: boolean;
  selected: boolean;
  onToggle: () => void;
  onTaskClick?: (stepName: string, taskName: string) => void;
  onOpenDetail?: (stepName: string) => void;
  onPreview?: (step: GraphStep, evt: React.MouseEvent) => void;
  onPreviewEnd?: () => void;
  innerLayout?: GraphLayout<GraphTask>;
  statusVariant?: StatusGlyphVariant;
  statusColorOverride?: string;
  taskStatusColorOverride?: string;
}) {
  const statusColor = getGraphStatusColor(node.data.status);
  const titleColor = selected ? statusColor : 'var(--text-primary)';
  const durationLabel = node.data.duration || '0s';
  const showDuration = Boolean(durationLabel && durationLabel !== '0s');
  const nameWidthEstimate = node.data.name.length * 6.6;
  const infoX = Math.min(node.width - 22, 28 + nameWidthEstimate);
  const durationX = infoX + 4 + 6;
  const isDarkMode = typeof document !== 'undefined' && document.documentElement.classList.contains('dark');
  const infoColor = isDarkMode ? '#22c55e' : '#0284c7';
  const innerOffset = (() => {
    if (!innerLayout) return { x: 0, y: 0 };
    const availableWidth = Math.max(0, node.width - INNER_PADDING * 2);
    const availableHeight = Math.max(0, node.height - (STEP_HEADER_HEIGHT - 6) - INNER_PADDING);
    const offsetX = Math.max(0, (availableWidth - innerLayout.width) / 2);
    const offsetY = Math.max(0, (availableHeight - innerLayout.height) / 2);
    return { x: offsetX, y: offsetY };
  })();
  return (
    <g
      transform={`translate(${node.x}, ${node.y})`}
      className="cursor-pointer"
      onClick={event => {
        event.stopPropagation();
        onToggle();
      }}
      onMouseDown={event => event.stopPropagation()}
      data-graph-node
    >
      <rect width={node.width} height={node.height} fill="transparent" />

      <g transform={`translate(${INNER_PADDING}, 10)`}>
        <GraphStatusGlyph status={node.data.status} x={12} y={12} size={expanded ? 16 : 18} opacity={expanded ? 0.3 : 1} variant={statusVariant} colorOverride={statusColorOverride} />
        <text x={30} y={14} className="text-[13px] font-semibold">
          <tspan style={{ fill: expanded ? 'var(--text-secondary)' : titleColor, opacity: expanded ? 0.5 : 1 }}>{node.data.name}</tspan>
        </text>
        {onOpenDetail && (
          <g
            transform={`translate(${infoX}, -8)`}
            onClick={event => {
              event.stopPropagation();
              onPreviewEnd?.();
              onOpenDetail(node.data.name);
            }}
            className="cursor-pointer"
            aria-label="Step details"
            style={{ pointerEvents: 'auto' }}
            onMouseEnter={event => onPreview?.(node.data, event)}
            onMouseLeave={() => onPreviewEnd?.()}
          >
            <rect width={12} height={12} rx={6} fill="transparent" stroke={infoColor} strokeWidth={1} opacity={selected ? 0.9 : 0.7} />
            <text x={6} y={9} textAnchor="middle" style={{ fill: infoColor, fontSize: '8px', fontWeight: 800, opacity: selected ? 1 : 0.95 }}>
              i
            </text>
          </g>
        )}
        <text x={durationX} y={14} className="text-[13px] font-semibold">
          {showDuration && (
            <tspan style={{ fill: 'var(--text-secondary)', fontWeight: expanded ? 500 : 600, opacity: expanded ? 0.5 : 1 }}>
              {`-  ${durationLabel}`}
            </tspan>
          )}
        </text>
        {(node.data.includeLabel || node.data.childRun) && (
          <text
            x={30}
            y={32}
            className="text-[11px]"
            style={{ fill: 'var(--text-secondary)', opacity: expanded ? 0.65 : 1 }}
          >
            {node.data.includeLabel && (
              <tspan style={{ fill: statusColor, fontWeight: 600, opacity: expanded ? 0.7 : 1 }}>
                {node.data.includeLabel}
              </tspan>
            )}
            {node.data.childRun && (
              <tspan dx={node.data.includeLabel ? 10 : 0}>{node.data.includeLabel ? '• Child run' : 'Child run'}</tspan>
            )}
          </text>
        )}
      </g>

      {expanded && innerLayout && (
        <g transform={`translate(${INNER_PADDING + innerOffset.x}, ${STEP_HEADER_HEIGHT - 6 + innerOffset.y})`}>
          {innerLayout.edges.map(edge => (
            <path
              key={edge.id}
              d={`M ${edge.points[0].x} ${edge.points[0].y} C ${edge.points[1].x} ${edge.points[1].y}, ${edge.points[2].x} ${edge.points[2].y}, ${edge.points[3].x} ${edge.points[3].y}`}
              fill="none"
              stroke={taskStatusColorOverride || getGraphStatusColor(edge.status)}
              strokeWidth={1.2}
              strokeOpacity={0.35}
              strokeLinecap="round"
            />
          ))}
          {innerLayout.nodes.map(task => (
          <TaskNodeRenderer
            key={task.data.id}
            task={task}
            stepName={node.data.name}
            onTaskClick={onTaskClick}
            statusColorOverride={taskStatusColorOverride}
          />
          ))}
        </g>
      )}
    </g>
  );
}

function TaskNodeRenderer({
  task,
  stepName,
  onTaskClick,
  fontSize = 11,
  glyphSize = 14,
  statusColorOverride,
}: {
  task: GraphLayoutNode<GraphTask>;
  stepName: string;
  onTaskClick?: (stepName: string, taskName: string) => void;
  fontSize?: number;
  glyphSize?: number;
  statusColorOverride?: string;
}) {
  const durationLabel = task.data.duration || '0s';
  const showDuration = Boolean(durationLabel && durationLabel !== '0s');
  const centerX = task.width / 2;
  const statusIconSize = glyphSize + 2;
  const statusIconY = 8;
  const lineHeight = fontSize + 4;
  const textY = statusIconY + statusIconSize + lineHeight;
  const statusColor = statusColorOverride || getGraphStatusColor(task.data.status);
  return (
    <g
      transform={`translate(${task.x}, ${task.y})`}
      onClick={event => {
        event.stopPropagation();
        onTaskClick?.(stepName, task.data.name);
      }}
      className="cursor-pointer"
    >
      <rect width={task.width} height={task.height} fill="transparent" />
      <svg x={centerX - statusIconSize / 2} y={statusIconY} width={statusIconSize} height={statusIconSize} viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="6" fill={statusColor} />
      </svg>
      <text x={centerX} y={textY} textAnchor="middle" style={{ fontSize, fontWeight: 700 }}>
        <tspan style={{ fill: 'var(--text-primary)' }}>{task.data.name}</tspan>
        {showDuration && <tspan style={{ fill: 'var(--text-secondary)', fontWeight: 700 }}>{`  -  ${durationLabel}`}</tspan>}
      </text>
    </g>
  );
}

function getGraphStatusColor(status: GraphStatus) {
  if (status === 'success') return '#10b981';
  if (status === 'failed') return '#ef4444';
  if (status === 'running') return '#3b82f6';
  return '#94a3b8';
}

function getGraphStatusLabel(status: GraphStatus) {
  if (status === 'success') return 'Success';
  if (status === 'failed') return 'Failed';
  if (status === 'running') return 'Running';
  if (status === 'pending') return 'Pending';
  if (status === 'skipped') return 'Skipped';
  return status;
}

function getGraphStatusIconPath(status: GraphStatus) {
  if (status === 'success') return STATUS_META.success.icon;
  if (status === 'failed') return STATUS_META.failure.icon;
  if (status === 'running') return STATUS_META.running.icon;
  if (status === 'pending') return STATUS_META.pending.icon;
  if (status === 'skipped') return STATUS_META.skipped.icon;
  return STATUS_META.pending.icon;
}

const MAX_ELAPSED_MS = 1000 * 60 * 60 * 24 * 30; // cap at 30 days to avoid runaway durations

function parseTimestamp(value?: string | null): number | null {
  if (!value) return null;
  const t = Date.parse(value);
  return Number.isNaN(t) ? null : t;
}

function humanizeDurationMs(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const seconds = totalSeconds % 60;
  const minutes = Math.floor(totalSeconds / 60) % 60;
  const hours = Math.floor(totalSeconds / 3600);
  if (hours) return `${hours}h ${minutes}m`;
  if (minutes) return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
  return `${seconds}s`;
}

function formatElapsedLabel(start?: string | null, end?: string | null, fallback = '0s') {
  const startTs = parseTimestamp(start);
  if (!startTs) return fallback;
  const endTs = parseTimestamp(end) ?? Date.now();
  const duration = endTs - startTs;
  if (duration <= 0 || duration > MAX_ELAPSED_MS) return fallback;
  return humanizeDurationMs(duration);
}

function formatStepDuration(step: StepDetail): string {
  const provided = (step.duration || '').trim();
  const range = calculateStepDurationFromTasks(step.tasks);
  if (range) return range;
  if (provided && /[a-zA-Z]/.test(provided)) return provided;
  const label = formatElapsedLabel(step.started_at, step.finished_at, '');
  if (label) return label;
  return '0s';
}

function calculateStepDurationFromTasks(tasks: TaskDetail[]): string | null {
  if (!tasks?.length) return null;
  let startMin: number | null = null;
  let endMax: number | null = null;
  tasks.forEach(task => {
    const start = parseTimestamp(task.started_at);
    if (start === null) return;
    startMin = startMin === null ? start : Math.min(startMin, start);
    const end = parseTimestamp(task.finished_at) ?? Date.now();
    endMax = endMax === null ? end : Math.max(endMax, end);
  });
  if (startMin === null || endMax === null) return null;
  const duration = endMax - startMin;
  if (duration <= 0 || duration > MAX_ELAPSED_MS) return null;
  return humanizeDurationMs(duration);
}

function normalizeGraphStatus(status: string | undefined, complete?: boolean): GraphStatus {
  const normalized = normalizeStatus(status, complete);
  if (normalized === 'success') return 'success';
  if (normalized === 'running') return 'running';
  if (normalized === 'skipped') return 'skipped';
  if (normalized === 'pending') return 'pending';
  return 'failed';
}

type StatusGlyphVariant = 'default' | 'dot';

function GraphStatusGlyph({
  status,
  x,
  y,
  size = 14,
  opacity = 1,
  variant = 'default',
  colorOverride,
}: {
  status: GraphStatus;
  x: number;
  y: number;
  size?: number;
  opacity?: number;
  variant?: StatusGlyphVariant;
  colorOverride?: string;
}) {
  const color = colorOverride || getGraphStatusColor(status);
  const strokeWidth = Math.max(1.6, Math.min(2.4, size / 6.5));
  if (variant === 'dot') {
    const r = size / 2;
    return <circle cx={x} cy={y} r={r} fill={color} opacity={opacity} />;
  }
  if (status === 'running') {
    const r = size / 2 - strokeWidth;
    return (
      <g transform={`translate(${x}, ${y})`} opacity={opacity}>
        <circle
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray="6 6"
          strokeLinecap="round"
        >
          <animate attributeName="stroke-dashoffset" from="12" to="0" dur="0.9s" repeatCount="indefinite" />
        </circle>
        <svg
          x={-size / 2}
          y={-size / 2}
          width={size}
          height={size}
          viewBox="0 0 24 24"
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d={getGraphStatusIconPath(status)} />
        </svg>
      </g>
    );
  }
  const path = getGraphStatusIconPath(status);
  return (
    <g transform={`translate(${x}, ${y})`} opacity={opacity}>
      <svg
        x={-size / 2}
        y={-size / 2}
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d={path} />
      </svg>
    </g>
  );
}

function getRanks(items: { id: string; dependsOn?: string[] }[]) {
  const ranks: Record<string, number> = {};
  const visited = new Set<string>();

  items.forEach(item => {
    if (!item.dependsOn || item.dependsOn.length === 0) {
      ranks[item.id] = 0;
    }
  });

  const getRank = (id: string): number => {
    if (ranks[id] !== undefined) return ranks[id];
    if (visited.has(id)) return 0;
    visited.add(id);

    const item = items.find(i => i.id === id);
    if (!item || !item.dependsOn?.length) {
      ranks[id] = 0;
      return 0;
    }

    let maxParentRank = -1;
    item.dependsOn.forEach(parentId => {
      maxParentRank = Math.max(maxParentRank, getRank(parentId));
    });

    ranks[id] = maxParentRank + 1;
    return maxParentRank + 1;
  };

  items.forEach(item => getRank(item.id));
  return ranks;
}

function calculateGraphLayout<T extends { id: string; dependsOn?: string[]; status: GraphStatus }>(
  items: T[],
  getSize: (item: T) => GraphSize,
  hGap: number,
  vGap: number,
  orientation: 'horizontal' | 'vertical' = 'horizontal'
): GraphLayout<T> {
  if (!items.length) {
    return { nodes: [], edges: [], width: PADDING * 2, height: PADDING * 2 };
  }

  const ranks = getRanks(items);
  const levels: T[][] = [];
  items.forEach(item => {
    const r = ranks[item.id] || 0;
    if (!levels[r]) levels[r] = [];
    levels[r].push(item);
  });
  levels.forEach(levelItems => {
    if (!levelItems) return;
    levelItems.sort((a, b) => a.id.localeCompare(b.id));
  });

  const nodes: GraphLayoutNode<T>[] = [];
  const edges: GraphLayoutEdge[] = [];

  let totalWidth = PADDING * 2;
  let totalHeight = PADDING * 2;

  if (orientation === 'horizontal') {
    let currentX = PADDING;
    const levelXs: number[] = [];

    levels.forEach((levelItems, lvlIdx) => {
      levelXs[lvlIdx] = currentX;
      const sizes = levelItems.map(getSize);
      const maxWidth = Math.max(...sizes.map(s => s.width), 0);
      currentX += maxWidth + hGap;
    });

    totalWidth = Math.max(PADDING * 2, currentX - hGap + PADDING);
    const levelHeights = levels.map(levelItems => levelItems.reduce((acc, item) => acc + getSize(item).height + vGap, 0) - vGap);
    const maxLevelHeight = Math.max(...levelHeights, 0);
    totalHeight = Math.max(PADDING * 2, maxLevelHeight + PADDING * 2);

    levels.forEach((levelItems, lvlIdx) => {
      const x = levelXs[lvlIdx];
      const levelH = levelHeights[lvlIdx];
      let currentY = PADDING + (maxLevelHeight - levelH) / 2;

      levelItems.forEach(item => {
        const size = getSize(item);
        nodes.push({
          data: item,
          level: lvlIdx,
          x,
          y: currentY,
          width: size.width,
          height: size.height,
        });
        currentY += size.height + vGap;
      });
    });
  } else {
    let currentY = PADDING;
    const levelYs: number[] = [];

    levels.forEach((levelItems, lvlIdx) => {
      levelYs[lvlIdx] = currentY;
      const sizes = levelItems.map(getSize);
      const maxHeight = Math.max(...sizes.map(s => s.height), 0);
      currentY += maxHeight + vGap;
    });

    totalHeight = Math.max(PADDING * 2, currentY - vGap + PADDING);
    const levelWidths = levels.map(levelItems => levelItems.reduce((acc, item) => acc + getSize(item).width + hGap, 0) - hGap);
    const maxLevelWidth = Math.max(...levelWidths, 0);
    totalWidth = Math.max(PADDING * 2, maxLevelWidth + PADDING * 2);

    levels.forEach((levelItems, lvlIdx) => {
      const y = levelYs[lvlIdx];
      const levelW = levelWidths[lvlIdx];
      let currentX = PADDING + (maxLevelWidth - levelW) / 2;

      levelItems.forEach(item => {
        const size = getSize(item);
        nodes.push({
          data: item,
          level: lvlIdx,
          x: currentX,
          y,
          width: size.width,
          height: size.height,
        });
        currentX += size.width + hGap;
      });
    });
  }

  items.forEach(item => {
    if (!item.dependsOn) return;
    const targetNode = nodes.find(n => n.data.id === item.id);
    if (!targetNode) return;

    item.dependsOn.forEach(parentId => {
      const sourceNode = nodes.find(n => n.data.id === parentId);
      if (!sourceNode) return;
      const start =
        orientation === 'horizontal'
          ? { x: sourceNode.x + sourceNode.width - 35, y: sourceNode.y + sourceNode.height / 2 }
          : { x: sourceNode.x + sourceNode.width / 2, y: sourceNode.y + sourceNode.height - 2 };
      const end =
        orientation === 'horizontal'
          ? { x: targetNode.x - 2, y: targetNode.y + targetNode.height / 2 }
          : { x: targetNode.x + targetNode.width / 2, y: targetNode.y + 2 };
      const controlDist =
        orientation === 'horizontal'
          ? Math.max(20, (end.x - start.x) * 0.38)
          : Math.max(18, (end.y - start.y) * 0.45);
      const points =
        orientation === 'horizontal'
          ? [
              start,
              { x: start.x + controlDist, y: start.y },
              { x: end.x - controlDist, y: end.y },
              end,
            ]
          : [
              start,
              { x: start.x, y: start.y + controlDist },
              { x: end.x, y: end.y - controlDist },
              end,
            ];

      edges.push({
        id: `${parentId}-${item.id}`,
        from: parentId,
        to: item.id,
        status: sourceNode.data.status,
        points,
      });
    });
  });

  return { nodes, edges, width: totalWidth, height: totalHeight };
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
  const eventDisplay = (triggerLabel.full || triggerLabel.display).slice(0, 8);
  const branchLabel = latestRun ? formatBranchDisplay(latestRun.git_ref, latestRun.git_target_ref) : '—';
  const commitLabel = latestRun?.git_commit_sha ? latestRun.git_commit_sha.slice(0, 8) : '—';
  const pusher = latestRun?.git_pusher_name || 'System';
  const timestamp = latestRun?.started_at;
  const repoLabel = latestRun ? formatRepoLabel(latestRun) : '—';
  const timeLabel = timeAgo(timestamp);

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
        className="w-full p-4 text-left hover:bg-[var(--bg-tertiary)] transition-colors"
        onClick={onToggle}
        aria-expanded={!collapsed}
      >
        <div
          className="grid items-center gap-2 min-w-0 overflow-hidden text-xs text-[var(--text-secondary)]"
          style={{
            gridTemplateColumns:
              'auto minmax(88px,105px) minmax(160px,1.2fr) minmax(82px,0.55fr) minmax(220px,1.4fr) minmax(120px,0.75fr) minmax(110px,0.65fr) minmax(130px,0.9fr) minmax(130px,0.9fr)',
          }}
        >
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
          <span className={`runner-pill ${meta.pillClass} flex-shrink-0 min-w-[96px] justify-center text-center`}>
            {meta.text}
          </span>
          <span className="text-sm font-semibold text-[var(--text-primary)] truncate" title={triggerLabel.full}>
            Event: {eventDisplay}
          </span>
          <span className="inline-flex items-center gap-1 min-w-0 whitespace-nowrap">
            <svg className="h-3.5 w-3.5 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span className="truncate" title={timestamp || undefined}>{timeLabel}</span>
          </span>
          <span className="inline-flex items-center gap-1 min-w-0 whitespace-nowrap">
            <svg className="h-3.5 w-3.5 flex-shrink-0 text-gray-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <circle cx="8" cy="7" r="2" />
              <circle cx="8" cy="17" r="2" />
              <circle cx="16" cy="7" r="2" />
              <path d="M10 7h4" />
              <path d="M8 9v6a4 4 0 004 4h4" />
            </svg>
            <span className="truncate" title={repoLabel}>{repoLabel}</span>
          </span>
          <span className="inline-flex items-center gap-1 min-w-0 whitespace-nowrap font-mono">
            <CommitIcon className="h-3.5 w-3.5 flex-shrink-0" />
            <span className="truncate" title={latestRun?.git_commit_sha || commitLabel}>{commitLabel}</span>
          </span>
          <span className="inline-flex items-center gap-1 min-w-0 whitespace-nowrap">
            <BranchIcon className="h-3.5 w-3.5 flex-shrink-0" />
            <span className="truncate" title={branchLabel}>{branchLabel}</span>
          </span>
          <span className="inline-flex items-center gap-1 min-w-0 whitespace-nowrap">
            <svg className="h-3.5 w-3.5 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
            <span className="truncate" title={pusher}>{pusher}</span>
          </span>
          <div className="flex items-center justify-end gap-2 whitespace-nowrap">
            <span className="px-2 py-[3px] text-[11px] rounded-full bg-[var(--bg-primary)] border border-[var(--border-primary)] text-[var(--text-secondary)] text-center">
              {group.runs.length} {group.runs.length === 1 ? 'Pipeline' : 'Pipelines'}
            </span>
            <div className="flex items-center gap-1">
              {group.runs.slice(0, 6).map(run => (
                <span key={run.run_id} className={`h-2.5 w-2.5 rounded-full ${statusDotClass(run.status, run.is_complete)}`} />
              ))}
            </div>
          </div>
        </div>
      </button>
      {!collapsed && (
        <div className="p-4 border-t border-[var(--border-primary)] bg-[var(--bg-primary)]">
          <div className="grid gap-4 md:grid-cols-4 xl:grid-cols-4">
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

function parseLegacyLogsHash(hash: string, runId?: string, levelOrder: string[] = []) {
  if (!hash || !hash.includes('/logs/')) return null;
  const trimmed = hash.replace(/^#/, '');
  const [pathPart, queryPart] = trimmed.split('?');
  const parts = pathPart.split('/').filter(Boolean).map(decodeURIComponent);
  const logsIdx = parts.indexOf('logs');
  if (logsIdx === -1) return null;
  const hashRunId = parts[logsIdx - 1];
  if (runId && hashRunId && hashRunId !== runId) return null;
  const segments = parts.slice(logsIdx + 1);
  if (segments.length < 6) return null;
  const [stepsSeg, levelsSeg, wrapSeg, structuredSeg, agentSeg, shortSeg] = segments;
  const steps = stepsSeg && stepsSeg !== 'all' ? stepsSeg.split(',').filter(Boolean) : [];
  const levelList = levelsSeg && levelsSeg !== 'all' ? levelsSeg.split(',').filter(Boolean) : [];
  const normalizedLevels = levelList.map(level => (level.toLowerCase() === 'warning' ? 'warn' : level.toLowerCase()));
  const orderedLevels = levelOrder.length
    ? normalizedLevels.sort((a, b) => levelOrder.indexOf(a) - levelOrder.indexOf(b))
    : normalizedLevels;
  const params = queryPart ? new URLSearchParams(queryPart) : null;
  return {
    steps,
    levels: new Set(orderedLevels),
    wrap: wrapSeg !== 'unwrap',
    structured: structuredSeg !== 'unstructured',
    agentOnly: agentSeg === 'agent',
    shortView: shortSeg !== 'full',
    search: params?.get('search') || '',
  };
}

function buildLegacyLogsHash(
  currentHash: string,
  runId: string,
  selectedSteps: Set<string>,
  selectedLevels: Set<string>,
  wrap: boolean,
  structured: boolean,
  agentOnly: boolean,
  shortView: boolean,
  searchText: string,
  levelOrder: string[]
) {
  if (!runId) return null;
  const trimmed = (currentHash || '#').replace(/^#/, '');
  const [pathPart] = trimmed.split('?');
  const parts = pathPart.split('/').filter(Boolean).map(decodeURIComponent);
  const logsIdx = parts.indexOf('logs');
  let prefix: string[];
  if (logsIdx !== -1) {
    prefix = parts.slice(0, logsIdx);
  } else {
    prefix = ['pipelineruns', 'events', runId];
  }
  if (!prefix.includes(runId)) {
    prefix.push(runId);
  }

  const stepsSeg = selectedSteps.size ? encodeURIComponent(Array.from(selectedSteps).join(',')) : 'all';
  const orderedLevels =
    selectedLevels.size === 0
      ? []
      : levelOrder.filter(level => selectedLevels.has(level));
  const levelsSeg = orderedLevels.length ? encodeURIComponent(orderedLevels.join(',')) : 'all';
  const wrapSeg = wrap ? 'wrap' : 'unwrap';
  const structuredSeg = structured ? 'structured' : 'unstructured';
  const agentSeg = agentOnly ? 'agent' : 'all';
  const shortSeg = shortView ? 'short' : 'full';

  const hashPath = `#/${[...prefix.map(encodeURIComponent), 'logs', stepsSeg, levelsSeg, wrapSeg, structuredSeg, agentSeg, shortSeg].join('/')}`;
  const query = searchText ? `?search=${encodeURIComponent(searchText)}` : '';
  return `${hashPath}${query}`;
}

function LogsModal({
  runId,
  runName,
  onClose,
  steps,
  stepNames,
  initialStep,
  initialSearch,
}: {
  runId: string;
  runName?: string | null;
  onClose: () => void;
  steps?: StepDetail[];
  stepNames?: string[];
  initialStep?: string | null;
  initialSearch?: string | null;
}) {
  const [lines, setLines] = useState<EnrichedLogLine[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedSteps, setSelectedSteps] = useState<Set<string>>(
    () => (initialStep && initialStep !== 'all' ? new Set([initialStep]) : new Set())
  );
  const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
  const [searchText, setSearchText] = useState('');
  const [stepSearch, setStepSearch] = useState('');
  const [follow, setFollow] = useState(true);
  const [wrap, setWrap] = useState(false);
  const [structured, setStructured] = useState(false);
  const [shortView, setShortView] = useState(true);
  const [agentOnly, setAgentOnly] = useState(false);
  const [hasUnseen, setHasUnseen] = useState(false);
  const lastIdRef = useRef(0);
  const logContainerRef = useRef<HTMLDivElement | null>(null);

  const levelOptions = useMemo(() => ['info', 'warn', 'error', 'debug'], []);

  useEffect(() => {
    const parsed = parseLegacyLogsHash(window.location.hash, runId, levelOptions);
    setSelectedSteps(
      parsed?.steps && parsed.steps.length
        ? new Set(parsed.steps)
        : initialStep && initialStep !== 'all'
        ? new Set([initialStep])
        : new Set()
    );
    setSelectedLevels(parsed?.levels ?? new Set());
    setSearchText(initialSearch ?? parsed?.search ?? '');
    setStepSearch('');
    setFollow(true);
    setWrap(parsed?.wrap ?? false);
    setStructured(parsed?.structured ?? false);
    setShortView(parsed?.shortView ?? true);
    setAgentOnly(parsed?.agentOnly ?? false);
    setLines([]);
    lastIdRef.current = 0;
    setHasUnseen(false);
  }, [initialSearch, initialStep, levelOptions, runId]);

  useEffect(() => {
    const nextHash = buildLegacyLogsHash(
      window.location.hash,
      runId,
      selectedSteps,
      selectedLevels,
      wrap,
      structured,
      agentOnly,
      shortView,
      searchText,
      levelOptions
    );
    if (!nextHash) return;
    const current = window.location.hash || '';
    if (current === nextHash) return;
    try {
      const url = new URL(window.location.href);
      url.hash = nextHash.slice(1);
      history.replaceState(null, '', url.toString());
    } catch {
      window.location.hash = nextHash;
    }
  }, [agentOnly, levelOptions, runId, searchText, selectedLevels, selectedSteps, shortView, structured, wrap]);

  useEffect(() => {
    if (shortView) {
      setWrap(false);
      setStructured(false);
    }
  }, [shortView]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | null = null;

    const fetchLogs = async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(buildApiUrl(`/v1/runs/${encodeURIComponent(runId)}/logs?since_line=${lastIdRef.current}`));
        if (!response.ok) throw new Error(await response.text());
        const payload = (await response.json()) as LogLine[] | null;
        if (cancelled) return;
        const list = Array.isArray(payload) ? payload : [];
        if (list.length) {
          lastIdRef.current = list[list.length - 1].id;
          const enriched = list.map(line => ({ ...line, ...parseLogLine(line.line || '') }));
          setLines(prev => [...prev, ...enriched]);
          if (!follow) setHasUnseen(true);
        }
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : 'Failed to load logs');
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    const tick = async () => {
      await fetchLogs();
      if (cancelled) return;
      const interval = document.hidden ? 30000 : 5000;
      timer = window.setTimeout(tick, interval);
    };

    void tick();

    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [follow, initialStep, runId]);

  const stepItems = useMemo(() => {
    const fromSteps = (steps || []).map(step => ({
      name: step.name,
      status: step.status,
    }));
    const provided = (stepNames || []).map(name => ({ name, status: undefined }));
    const derived = Array.from(new Set(lines.map(line => line.step).filter(Boolean) as string[])).map(name => ({
      name,
      status: undefined,
    }));
    const merged = [...fromSteps, ...provided, ...derived];
    const seen = new Set<string>();
    return merged.filter(item => {
      if (!item.name || seen.has(item.name)) return false;
      seen.add(item.name);
      return true;
    });
  }, [lines, stepNames, steps]);

  const filteredStepItems = useMemo(() => {
    const term = stepSearch.trim().toLowerCase();
    if (!term) return stepItems;
    return stepItems.filter(item => item.name.toLowerCase().includes(term));
  }, [stepItems, stepSearch]);

  const isAgentLine = (line: EnrichedLogLine) => {
    const lower = (line.line || '').toLowerCase();
    return lower.includes('agent') || (line.step || '').toLowerCase().includes('agent');
  };

  const stepColorMap = useMemo(() => {
    const palette = [
      {
        pillClass: 'bg-sky-500 text-white border-sky-600 dark:bg-sky-400 dark:text-slate-900 dark:border-sky-500',
        dotClass: 'bg-sky-500',
        lineClass: 'border-sky-500 dark:border-sky-400',
      },
      {
        pillClass: 'bg-emerald-500 text-white border-emerald-600 dark:bg-emerald-400 dark:text-slate-900 dark:border-emerald-500',
        dotClass: 'bg-emerald-500',
        lineClass: 'border-emerald-500 dark:border-emerald-400',
      },
      {
        pillClass: 'bg-indigo-500 text-white border-indigo-600 dark:bg-indigo-400 dark:text-slate-900 dark:border-indigo-500',
        dotClass: 'bg-indigo-500',
        lineClass: 'border-indigo-500 dark:border-indigo-400',
      },
      {
        pillClass: 'bg-amber-500 text-white border-amber-600 dark:bg-amber-400 dark:text-slate-900 dark:border-amber-500',
        dotClass: 'bg-amber-500',
        lineClass: 'border-amber-500 dark:border-amber-400',
      },
      {
        pillClass: 'bg-rose-500 text-white border-rose-600 dark:bg-rose-400 dark:text-slate-900 dark:border-rose-500',
        dotClass: 'bg-rose-500',
        lineClass: 'border-rose-500 dark:border-rose-400',
      },
      {
        pillClass: 'bg-teal-500 text-white border-teal-600 dark:bg-teal-400 dark:text-slate-900 dark:border-teal-500',
        dotClass: 'bg-teal-500',
        lineClass: 'border-teal-500 dark:border-teal-400',
      },
      {
        pillClass: 'bg-purple-500 text-white border-purple-600 dark:bg-purple-400 dark:text-slate-900 dark:border-purple-500',
        dotClass: 'bg-purple-500',
        lineClass: 'border-purple-500 dark:border-purple-400',
      },
      {
        pillClass: 'bg-lime-500 text-white border-lime-600 dark:bg-lime-400 dark:text-slate-900 dark:border-lime-500',
        dotClass: 'bg-lime-500',
        lineClass: 'border-lime-500 dark:border-lime-400',
      },
    ];
    const map = new Map<string, (typeof palette)[number]>();
    Array.from(selectedSteps).forEach((step, index) => {
      map.set(step, palette[index % palette.length]);
    });
    return map;
  }, [selectedSteps]);

  const presentLevels = useMemo(() => {
    const set = new Set<string>();
    lines.forEach(line => {
      const lvl = (line.level || 'info').toLowerCase();
      const normalized = lvl === 'warning' ? 'warn' : lvl || 'info';
      set.add(normalized);
      if (isAgentLine(line)) set.add('agent');
    });
    return set;
  }, [lines]);

  const visibleLines = useMemo(() => {
    const stepFilterActive = selectedSteps.size > 0;
    const term = searchText.trim().toLowerCase();
    return lines.filter(line => {
      const level = (line.level || 'info').toLowerCase();
      const normalizedLevel = level === 'warning' ? 'warn' : level;
      const content = (line.line || '').toLowerCase();
      if (stepFilterActive && (!line.step || !selectedSteps.has(line.step))) return false;
      if (agentOnly && !isAgentLine(line)) return false;
      if (selectedLevels.size > 0 && !selectedLevels.has(normalizedLevel)) return false;
      if (term && !content.includes(term)) return false;
      return true;
    });
  }, [agentOnly, lines, searchText, selectedLevels, selectedSteps]);

  const toggleLevel = (level: string) => {
    setSelectedLevels(prev => {
      const isAll = prev.size === 0 || prev.size === levelOptions.length;
      if (isAll) return new Set([level]);
      if (prev.size === 1 && prev.has(level)) return new Set();
      return new Set([level]);
    });
  };

  const toggleStep = (step: string) => {
    setSelectedSteps(prev => {
      const next = new Set(prev);
      if (next.has(step)) {
        next.delete(step);
      } else {
        next.add(step);
      }
      return next;
    });
  };

  const resetFilters = () => {
    setSelectedSteps(new Set());
    setSelectedLevels(new Set());
    setAgentOnly(false);
    setSearchText('');
    setStepSearch('');
    setShortView(true);
    setWrap(false);
    setStructured(false);
    setHasUnseen(false);
  };

  const handleDownload = () => {
    const source = (visibleLines.length ? visibleLines : lines) || [];
    if (!source.length) {
      alert('No logs available to download yet.');
      return;
    }
    const content = source
      .map(line => {
        const ts = line.timestamp ? new Date(line.timestamp).toISOString() : '';
        const parts = [ts || ''];
        if (line.step) parts.push(`[${line.step}]`);
        if (line.level) parts.push(line.level.toUpperCase());
        parts.push('-', line.line || '');
        return parts.join(' ');
      })
      .join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `logs-${(runId || 'run').slice(0, 8)}.txt`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const formatTime = (iso: string) => {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    return date.toLocaleTimeString(undefined, { hour12: true });
  };

  const levelTone = (level: string) => {
    const normalized = level.toLowerCase();
    if (normalized === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-100';
    if (normalized === 'warn' || normalized === 'warning') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-100';
    if (normalized === 'debug') return 'bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-slate-200';
    if (normalized === 'agent') return 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-100';
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-100';
  };

  useEffect(() => {
    const container = logContainerRef.current;
    if (!container) return;
    const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
    if (follow && nearBottom) {
      container.scrollTop = container.scrollHeight;
      setHasUnseen(false);
    }
  }, [follow, visibleLines.length]);

  const handleScroll = () => {
    const container = logContainerRef.current;
    if (!container) return;
    const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
    if (!nearBottom) {
      setFollow(false);
    } else {
      setFollow(true);
      setHasUnseen(false);
    }
  };

  const logCountLabel = `${visibleLines.length} line${visibleLines.length === 1 ? '' : 's'} + ${lines.length} total`;

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-[var(--bg-overlay)] px-4 py-6">
      <div className="w-full max-w-6xl bg-[var(--bg-primary)] rounded-2xl shadow-2xl border border-[var(--border-primary)] flex flex-col max-h-[90vh] overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-primary)]">
          <div>
            <p className="text-base font-semibold text-[var(--text-primary)]">Agent Logs for {runName || runId}</p>
            <p className="text-xs text-[var(--text-secondary)]">Run ID: {runId}</p>
          </div>
          <div className="flex items-center gap-2">
            <button className="runner-pill runner-pill--ghost" type="button" onClick={handleDownload}>
              Download
            </button>
            <button className="glass-button-subtle" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>

        <div className="flex flex-col gap-3 border-b border-[var(--border-primary)] bg-[var(--bg-secondary)] px-5 py-3 md:flex-row md:items-start md:gap-6">
          <div className="flex-1 min-w-[280px]">
            <div className="relative">
              <input
                type="search"
                className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
                placeholder="Search logs..."
                value={searchText}
                onChange={event => setSearchText(event.target.value)}
              />
              {searchText && (
                <button
                  type="button"
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-[var(--text-secondary)] text-xs"
                  onClick={() => setSearchText('')}
                >
                  Clear
                </button>
              )}
            </div>
            <p className="text-[11px] text-[var(--text-secondary)] mt-1">{logCountLabel}</p>
          </div>
          <div className="flex flex-col gap-2 flex-1 min-w-[240px] w-full items-end">
            <div className="flex items-center gap-2 flex-wrap justify-end">
              {levelOptions.map(level => {
                const isDefault = selectedLevels.size === 0;
                const active = !isDefault && selectedLevels.has(level);
                const available = presentLevels.has(level);
                return (
                  <button
                    key={level}
                    type="button"
                    disabled={!available && lines.length > 0}
                    className={`px-2.5 py-1 rounded-full text-xs font-semibold border border-[var(--border-primary)] ${active ? 'bg-[var(--bg-primary)] text-[var(--text-primary)] ring-1 ring-[var(--border-accent)]' : 'text-[var(--text-secondary)]'} ${!available && lines.length > 0 ? 'opacity-40 cursor-not-allowed' : ''}`}
                    onClick={() => toggleLevel(level)}
                    title={`Toggle ${level} logs`}
                  >
                    {level.toUpperCase()}
                  </button>
                );
              })}
              <button
                type="button"
                disabled={!presentLevels.has('agent') && lines.length > 0}
                className={`px-2.5 py-1 rounded-full text-xs font-semibold border border-[var(--border-primary)] ${agentOnly ? 'bg-[var(--bg-primary)] text-[var(--text-primary)] ring-1 ring-[var(--border-accent)]' : 'text-[var(--text-secondary)]'} ${!presentLevels.has('agent') && lines.length > 0 ? 'opacity-40 cursor-not-allowed' : ''}`}
                onClick={() => {
                  setAgentOnly(prev => !prev);
                  setFollow(true);
                  setHasUnseen(false);
                }}
                title="Show only agent logs"
              >
                AGENT
              </button>
            </div>
            <div className="flex items-center gap-2 flex-wrap justify-end w-full">
              {[
                { label: 'Follow', value: follow, setter: setFollow },
                { label: 'Wrap', value: wrap, setter: setWrap },
                { label: 'Structured', value: structured, setter: setStructured },
                { label: 'Short', value: shortView, setter: setShortView },
              ].map(toggle => (
                <button
                  key={toggle.label}
                  type="button"
                  onClick={() => {
                    const next = !toggle.value;
                    toggle.setter(next);
                    if (toggle.label === 'Follow' && next) {
                      const container = logContainerRef.current;
                      if (container) container.scrollTop = container.scrollHeight;
                      setHasUnseen(false);
                    }
                  }}
                  disabled={shortView && (toggle.label === 'Wrap' || toggle.label === 'Structured')}
                  className={`px-3 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-2 ${toggle.value ? 'bg-[var(--bg-primary)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'} ${shortView && (toggle.label === 'Wrap' || toggle.label === 'Structured') ? 'opacity-50 cursor-not-allowed' : ''}`}
                  title={`Toggle ${toggle.label.toLowerCase()}`}
                >
                  <span
                    className={`h-3.5 w-3.5 rounded-sm flex items-center justify-center ${toggle.value ? 'bg-[var(--text-primary)] text-[var(--bg-primary)]' : 'bg-[var(--bg-primary)] text-[var(--text-secondary)]'}`}
                    aria-hidden="true"
                  >
                    {toggle.value && (
                      <svg className="h-2.5 w-2.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M5 12l4 4L19 7" />
                      </svg>
                    )}
                  </span>
                  {toggle.label}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="flex flex-1 min-h-0">
          <aside className="w-64 border-r border-[var(--border-primary)] bg-[var(--bg-primary)] flex flex-col">
            <div className="p-3">
              <input
                type="search"
                className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
                placeholder="Filter steps..."
                value={stepSearch}
                onChange={event => setStepSearch(event.target.value)}
              />
            </div>
            <div className="flex-1 overflow-auto px-2 pb-2 space-y-2">
              {filteredStepItems.map(item => {
                const active = selectedSteps.has(item.name);
                const meta = getStatusMeta(item.status, true);
                const color = stepColorMap.get(item.name);
                return (
                  <button
                    key={item.name}
                    type="button"
                    className={`w-full text-left px-3 py-2 rounded-lg border border-[var(--border-primary)] flex items-center justify-between gap-2 ${active ? 'bg-[var(--bg-secondary)]' : 'bg-[var(--bg-primary)] hover:bg-[var(--bg-secondary)]'}`}
                    onClick={() => toggleStep(item.name)}
                    title={item.name}
                  >
                    <span className="text-sm text-[var(--text-primary)] truncate flex items-center gap-2">
                      {active && color && <span className={`h-2.5 w-2.5 rounded-full ${color.dotClass}`} aria-hidden="true" />}
                      <span className="truncate">{item.name}</span>
                    </span>
                    {item.status && <span className={`text-[10px] px-2 py-1 rounded-full border ${meta.pillClass}`}>{meta.text}</span>}
                  </button>
                );
              })}
              {!filteredStepItems.length && (
                <div className="text-xs text-[var(--text-secondary)] px-3 py-2">No steps found.</div>
              )}
            </div>
            <div className="border-t border-[var(--border-primary)] p-3 flex items-center gap-2 justify-between">
              <button
                type="button"
                className="runner-pill runner-pill--ghost text-xs"
                onClick={() => setSelectedSteps(new Set(stepItems.map(item => item.name)))}
                disabled={!stepItems.length}
              >
                All
              </button>
              <button type="button" className="runner-pill runner-pill--ghost text-xs" onClick={() => setSelectedSteps(new Set())}>
                Clear
              </button>
            </div>
          </aside>

          <section className="flex-1 flex flex-col bg-[var(--bg-secondary)] min-h-0 min-w-0">
            <div className="flex items-center gap-3 px-5 py-3 border-b border-[var(--border-primary)] bg-[var(--bg-primary)]">
              {error && <div className="text-red-500 text-sm">{error}</div>}
              {hasUnseen && !follow && (
                <button
                  type="button"
                  className="runner-pill runner-pill--ghost text-xs"
                  onClick={() => {
                    setFollow(true);
                    const container = logContainerRef.current;
                    if (container) container.scrollTop = container.scrollHeight;
                    setHasUnseen(false);
                  }}
                >
                  Jump to latest
                </button>
              )}
              <button className="runner-pill runner-pill--ghost text-xs" type="button" onClick={resetFilters}>
                Reset filters
              </button>
              {loading && <span className="text-[var(--text-secondary)] text-xs">Fetching new logs…</span>}
            </div>
            <div
              ref={logContainerRef}
              onScroll={handleScroll}
              className={`flex-1 overflow-y-auto overflow-x-auto px-5 py-4 font-mono text-sm space-y-1 ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre'} bg-[var(--bg-secondary)] min-w-0`}
            >
              {loading && !lines.length && <div className="text-[var(--text-secondary)]">Loading…</div>}
              {!loading && visibleLines.length === 0 && <div className="text-[var(--text-secondary)]">No log lines match the current filters.</div>}
              {visibleLines.map(line => {
                const level = (line.level || 'info').toLowerCase();
                const isAgent = isAgentLine(line);
                const levelLabel = isAgent ? 'AGENT' : level.toUpperCase();
                const rawLine = line.line || '';
                const stepColor = line.step ? stepColorMap.get(line.step) : undefined;
                const content = structured
                  ? (() => {
                      const jsonStart = rawLine.indexOf('{');
                      if (jsonStart !== -1) {
                        try {
                          const parsed = JSON.parse(rawLine.slice(jsonStart));
                          return JSON.stringify(parsed, null, 2);
                        } catch {
                          return rawLine;
                        }
                      }
                      return rawLine;
                    })()
                  : rawLine;
                if (shortView) {
                  const messageOnly = (() => {
                    try {
                      const jsonStart = rawLine.indexOf('{');
                      if (jsonStart !== -1) {
                        const parsed = JSON.parse(rawLine.slice(jsonStart));
                        const msg = parsed.message ?? parsed.msg ?? parsed.output ?? '';
                        if (msg) {
                          return typeof msg === 'string' ? msg : JSON.stringify(msg);
                        }
                      }
                    } catch {
                      // ignore
                    }
                    return content || '';
                  })();
                  return (
                    <div
                      key={line.id}
                      className={`flex items-start gap-3 rounded-lg px-2 py-1 hover:bg-[var(--bg-primary)] ${stepColor ? `border-l-4 ${stepColor.lineClass}` : ''}`}
                    >
                      <span className="text-[var(--text-secondary)] text-xs w-20 flex-shrink-0">{formatTime(line.timestamp)}</span>
                      <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold ${levelTone(levelLabel)}`}>
                        {levelLabel}
                      </span>
                      <pre
                        className={`flex-1 text-[var(--text-primary)] leading-6 ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre min-w-max'}`}
                      >
                        {messageOnly || '—'}
                      </pre>
                    </div>
                  );
                }
                return (
                  <div key={line.id} className="flex items-start gap-3 rounded-lg px-2 py-1 hover:bg-[var(--bg-primary)]">
                    <span className="text-[var(--text-secondary)] text-xs w-20 flex-shrink-0">{formatTime(line.timestamp)}</span>
                    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold ${levelTone(levelLabel)}`}>
                      {levelLabel}
                    </span>
                    {line.step && (
                      <span
                        className={`inline-flex items-center px-2 py-0.5 rounded-full border text-[11px] font-semibold ${
                          stepColor ? stepColor.pillClass : 'bg-[var(--bg-primary)] border-[var(--border-primary)] text-[var(--text-primary)]'
                        }`}
                      >
                        {line.step}
                      </span>
                    )}
                    <pre
                      className={`flex-1 text-[var(--text-primary)] leading-6 ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre min-w-max'}`}
                    >
                      {content}
                    </pre>
                  </div>
                );
              })}
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}

function StepDetailModal({
  step,
  onClose,
  onViewLogs,
  pipelineDefinition,
}: {
  step: StepDetail | null;
  onClose: () => void;
  onViewLogs: () => void;
  pipelineDefinition?: PipelineDefinition;
}) {
  const config = step?.configuration;
  const taskDefs = config?.tasks || [];
  const [activeTaskName, setActiveTaskName] = useState<string | null>(null);

  useEffect(() => {
    setActiveTaskName(null);
  }, [step?.name]);

  const taskLayout = useMemo<TaskGraphLayout | null>(() => {
    if (!step) return null;
    const layoutTasks: GraphTask[] = (step.tasks || []).map(task => {
      const def = taskDefs.find(t => t.name === task.task_name);
      const deps = def?.depends_on || [];
      const taskId = task.task_name || task.task_id || `task-${task.task_index}`;
      return {
        id: taskId,
        name: task.task_name,
        status: normalizeGraphStatus(task.status, task.status === 'success'),
        duration: formatElapsedLabel(task.started_at, task.finished_at, ''),
        dependsOn: deps,
      };
    });
    const dependencyCount = layoutTasks.reduce((sum, t) => sum + (t.dependsOn?.length || 0), 0);
    const hasAnyDeps = dependencyCount > 0;
    const chained = layoutTasks.map((t, idx) => (idx === 0 ? t : { ...t, dependsOn: [layoutTasks[idx - 1].id] }));
    const tasksForLayout = !hasAnyDeps && layoutTasks.length > 1 ? chained : layoutTasks;
    const sizeFor = (task: GraphTask): GraphSize => {
      const label = `${task.name} - ${task.duration || '0s'}`;
      const width = Math.max(TASK_MIN_WIDTH + 40, Math.min(TASK_MAX_WIDTH + 60, 38 + label.length * 8));
      return { width, height: Math.max(TASK_HEIGHT + 28, 64) };
    };

    const baseLayout = calculateGraphLayout(tasksForLayout, sizeFor, 44, 32, 'horizontal');
    const layoutNeedsChain = !hasAnyDeps && baseLayout.nodes.length > 1 && baseLayout.edges.length === 0;
    const finalLayout = layoutNeedsChain ? calculateGraphLayout(chained, sizeFor, 44, 32, 'horizontal') : baseLayout;

    return {
      ...finalLayout,
      orientation: 'horizontal',
      taskCount: layoutTasks.length,
      dependencyCount,
    };
  }, [step, taskDefs]);

  const taskGraphView = useMemo(() => {
    if (!taskLayout || !taskLayout.nodes.length) return null;
    const density = taskLayout.taskCount ? taskLayout.dependencyCount / taskLayout.taskCount : 0;
    const padScale = 1 + Math.min(0.6, taskLayout.taskCount * 0.04 + density * 0.06);
    const padX = Math.max(48, Math.min(170, taskLayout.width * 0.18 * padScale));
    const padY = Math.max(60, Math.min(200, taskLayout.height * 0.2 * padScale));
    const viewWidth = Math.max(taskLayout.width + padX * 2, 360 + taskLayout.taskCount * 6);
    const viewHeight = Math.max(taskLayout.height + padY * 2, 380 + taskLayout.taskCount * 8);
    return {
      viewWidth,
      viewHeight,
      taskCount: taskLayout.taskCount,
      dependencyCount: taskLayout.dependencyCount,
      density,
      orientation: taskLayout.orientation,
    };
  }, [taskLayout]);

  const graphContainerRef = useRef<HTMLDivElement | null>(null);
  const [baseGraphScale, setBaseGraphScale] = useState(1);
  const [userGraphScale, setUserGraphScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const draggingRef = useRef(false);
  const dragStartRef = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null);
  const userAdjustedRef = useRef(false);
  const clampUserScale = useCallback((value: number) => Math.min(3, Math.max(0.6, value)), []);
  const graphScale = baseGraphScale * userGraphScale;
  const nearlyEqual = (a: number, b: number, eps = 0.5) => Math.abs(a - b) < eps;
  const markUserAdjusted = () => {
    userAdjustedRef.current = true;
  };

  const centerGraph = useCallback(() => {
    if (!graphContainerRef.current || !taskGraphView) return;
    const rect = graphContainerRef.current.getBoundingClientRect();
    const scaledWidth = taskGraphView.viewWidth * graphScale;
    const scaledHeight = taskGraphView.viewHeight * graphScale;
    const nextPan = {
      x: (rect.width - scaledWidth) / 2,
      y: (rect.height - scaledHeight) / 2,
    };
    setPan(prev => {
      if (nearlyEqual(prev.x, nextPan.x, 0.3) && nearlyEqual(prev.y, nextPan.y, 0.3)) return prev;
      return nextPan;
    });
  }, [graphScale, nearlyEqual, taskGraphView]);

  const recomputeBaseScale = useCallback(() => {
    if (!graphContainerRef.current || !taskGraphView) return;
    const rect = graphContainerRef.current.getBoundingClientRect();
    const padding = 32;
    const availableWidth = Math.max(160, rect.width - padding * 2);
    const availableHeight = Math.max(260, rect.height - padding * 2);
    if (!taskGraphView.viewWidth || !taskGraphView.viewHeight) return;
    const fitScale = Math.min(availableWidth / taskGraphView.viewWidth, availableHeight / taskGraphView.viewHeight);
    const density = taskGraphView.taskCount ? taskGraphView.dependencyCount / taskGraphView.taskCount : 0;
    const sizeFactor =
      taskGraphView.taskCount <= 3 ? 1.18 : taskGraphView.taskCount <= 6 ? 1.05 : taskGraphView.taskCount <= 10 ? 0.95 : 0.82;
    const dependencyFactor = density > 1.3 ? 0.86 : density > 0.8 ? 0.92 : density > 0.4 ? 0.98 : 1.06;
    const orientationFactor = taskGraphView.orientation === 'horizontal' && taskGraphView.taskCount <= 4 ? 1.05 : 1;
    const target = Math.min(2.2, Math.max(0.7, fitScale * sizeFactor * dependencyFactor * orientationFactor));
    setBaseGraphScale(target);
    if (!userAdjustedRef.current) {
      setUserGraphScale(1);
    }
  }, [taskGraphView]);

  useEffect(() => {
    userAdjustedRef.current = false;
    recomputeBaseScale();
  }, [recomputeBaseScale]);

  useLayoutEffect(() => {
    const onResize = () => recomputeBaseScale();
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [recomputeBaseScale]);

  useLayoutEffect(() => {
    if (!taskGraphView || userAdjustedRef.current) return;
    const rect = graphContainerRef.current?.getBoundingClientRect();
    if (!rect) return;
    centerGraph();
  }, [baseGraphScale, centerGraph, graphScale, nearlyEqual, taskGraphView, userGraphScale]);

  useEffect(() => {
    const el = graphContainerRef.current;
    if (!el) return undefined;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const factor = e.deltaY > 0 ? 1 / 1.06 : 1.06;
      markUserAdjusted();
      setUserGraphScale(prev => clampUserScale(prev * factor));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [clampUserScale]);

  useEffect(() => {
    if (userAdjustedRef.current) return;
    centerGraph();
  }, [centerGraph, taskGraphView]);

  const taskContentOffset = useMemo(() => {
    if (!taskLayout || !taskLayout.nodes.length || !taskGraphView) return { x: 0, y: 0 };
    const minX = Math.min(...taskLayout.nodes.map(n => n.x));
    const minY = Math.min(...taskLayout.nodes.map(n => n.y));
    const maxX = Math.max(...taskLayout.nodes.map(n => n.x + n.width));
    const maxY = Math.max(...taskLayout.nodes.map(n => n.y + n.height));
    const contentWidth = maxX - minX;
    const contentHeight = maxY - minY;
    const extraX = Math.max(0, taskGraphView.viewWidth - contentWidth);
    const extraY = Math.max(0, taskGraphView.viewHeight - contentHeight);
    return {
      x: -minX + extraX / 2,
      y: -minY + extraY / 2,
    };
  }, [taskGraphView, taskLayout]);

  const hasTasks = (step?.tasks?.length || 0) > 0;
  const configTasks = config?.tasks || [];
  const stepDefinition = useMemo(() => {
    if (!pipelineDefinition?.steps || !step?.name) return null;
    return pipelineDefinition.steps.find(s => s.name === step.name) || null;
  }, [pipelineDefinition, step?.name]);
  const stepGoal = config?.goal || (stepDefinition as any)?.goal || '';
  const stepScript = config?.script || (stepDefinition as any)?.script || '';
  const taskDefsFromDefinition = stepDefinition?.tasks || [];
  const allTaskDefinitions = useMemo(() => {
    if (configTasks.length && taskDefsFromDefinition.length) {
      const map = new Map<string, TaskDefinition>();
      taskDefsFromDefinition.forEach(def => map.set(def.name, def));
      configTasks.forEach(def => map.set(def.name, def));
      return Array.from(map.values());
    }
    return configTasks.length ? configTasks : taskDefsFromDefinition;
  }, [configTasks, taskDefsFromDefinition]);
  const hasTaskDefinitions = allTaskDefinitions.length > 0;
  const isSingleSyntheticTask = hasTasks && !hasTaskDefinitions && (step?.tasks?.length || 0) === 1;
  const showStepLevelInfo = !hasTasks || isSingleSyntheticTask;

  const selectedTask = useMemo(() => {
    if (!activeTaskName) return null;
    return (step?.tasks || []).find(task => task.task_name === activeTaskName) || null;
  }, [activeTaskName, step?.tasks]);

  const selectedTaskDefinition = useMemo(() => {
    if (!activeTaskName) return null;
    return allTaskDefinitions.find(def => def.name === activeTaskName) || null;
  }, [activeTaskName, allTaskDefinitions]);

  useEffect(() => {
    const tasks = step?.tasks || [];
    if (!tasks.length) {
      setActiveTaskName(null);
      return;
    }
    if (isSingleSyntheticTask) {
      setActiveTaskName(null);
      return;
    }
    const hasSelection = tasks.some(task => task.task_name === activeTaskName);
    if (!hasSelection) {
      if (tasks.length === 1) {
        setActiveTaskName(tasks[0].task_name);
      } else {
        setActiveTaskName(null);
      }
    }
  }, [activeTaskName, isSingleSyntheticTask, step?.tasks]);

  const onMouseDownGraph = (e: React.MouseEvent) => {
    markUserAdjusted();
    draggingRef.current = true;
    dragStartRef.current = { x: e.clientX, y: e.clientY, panX: pan.x, panY: pan.y };
  };
  const onMouseMoveGraph = (e: React.MouseEvent) => {
    if (!draggingRef.current || !dragStartRef.current) return;
    const dx = e.clientX - dragStartRef.current.x;
    const dy = e.clientY - dragStartRef.current.y;
    setPan({ x: dragStartRef.current.panX + dx, y: dragStartRef.current.panY + dy });
  };
  const endDrag = () => {
    draggingRef.current = false;
    dragStartRef.current = null;
  };

  const statusMeta = step ? getStatusMeta(step.status, true) : null;

  if (!step) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] px-3 py-6">
      <div className="w-full max-w-6xl bg-white dark:bg-slate-900 rounded-xl shadow-xl border border-[var(--border-primary)] flex flex-col max-h-[90vh] overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-primary)] bg-[var(--bg-primary)]">
          <div className="flex flex-wrap items-center gap-3">
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Step: {step.name}</h3>
            {statusMeta && <span className={`runner-pill border ${statusMeta.pillClass} text-xs`}>{statusMeta.text}</span>}
            {step.duration && <span className="runner-pill runner-pill--muted text-xs">Duration: {step.duration}</span>}
          </div>
          <div className="flex items-center gap-2">
            <button className="runner-pill runner-pill--ghost" type="button" onClick={onViewLogs}>
              View Logs
            </button>
            <button className="runner-pill runner-pill--ghost" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-auto p-5 space-y-4 bg-[var(--bg-secondary)]">
          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
            <div className="flex items-center justify-between mb-2">
              <p className="text-sm font-semibold text-[var(--text-primary)]">Execution Flow</p>
              <span className="text-xs text-[var(--text-secondary)]">
                {step.tasks.length} task{step.tasks.length === 1 ? '' : 's'}
              </span>
            </div>
            <div
              className="relative h-[420px] lg:h-[480px] w-full overflow-hidden rounded border border-[var(--border-primary)] bg-white dark:bg-slate-950 flex items-center justify-center"
              ref={graphContainerRef}
              onMouseDown={onMouseDownGraph}
              onMouseMove={onMouseMoveGraph}
              onMouseUp={endDrag}
              onMouseLeave={endDrag}
            >
              {taskLayout && taskLayout.nodes.length && taskGraphView ? (
                <>
                  <div className="absolute right-3 top-3 z-20 flex gap-2">
                    <button
                      type="button"
                      className="h-9 w-9 rounded-full bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] border border-[var(--border-primary)]"
                      aria-label="Zoom out"
                      onClick={() => {
                        markUserAdjusted();
                        setUserGraphScale(prev => clampUserScale(prev / 1.15));
                      }}
                    >
                      −
                    </button>
                    <button
                      type="button"
                      className="h-9 w-9 rounded-full bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)] border border-[var(--border-primary)]"
                      aria-label="Zoom in"
                      onClick={() => {
                        markUserAdjusted();
                        setUserGraphScale(prev => clampUserScale(prev * 1.15));
                      }}
                    >
                      +
                    </button>
                  </div>
                  <svg
                    width="100%"
                    height="100%"
                    viewBox={`0 0 ${taskGraphView.viewWidth} ${taskGraphView.viewHeight}`}
                    preserveAspectRatio="xMidYMid meet"
                    className="p-6"
                    style={{
                      transform: `translate(${pan.x}px, ${pan.y}px) scale(${graphScale})`,
                      transformOrigin: 'center center',
                      margin: '0 auto',
                      display: 'block',
                      cursor: draggingRef.current ? 'grabbing' : 'grab',
                    }}
                  >
                    <g transform={`translate(${taskContentOffset.x}, ${taskContentOffset.y})`}>
                      {taskLayout.edges.map(edge => {
                        const [start, c1, c2, end] = edge.points;
                        return (
                          <path
                            key={edge.id}
                            d={`M ${start.x} ${start.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${end.x} ${end.y}`}
                            fill="none"
                            stroke={getGraphStatusColor(edge.status)}
                            strokeWidth={2.2}
                            strokeOpacity={0.75}
                            strokeLinecap="round"
                          />
                        );
                      })}
                      {taskLayout.nodes.map(node => (
                        <TaskNodeRenderer
                          key={node.data.id}
                          task={node}
                          stepName={step.name}
                          fontSize={13}
                          glyphSize={18}
                          onTaskClick={(_, taskName) => setActiveTaskName(taskName)}
                        />
                      ))}
                    </g>
                  </svg>
                </>
              ) : (
                <div className="text-sm text-[var(--text-secondary)]">No task graph available.</div>
              )}
            </div>
          </div>

          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 space-y-4">
            <p className="text-sm font-semibold text-[var(--text-primary)]">Configuration</p>
            <div className="space-y-3 text-sm text-[var(--text-primary)]">
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Image</span>
                  <span className="font-mono break-words">{config?.image || '—'}</span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Secrets</span>
                  <div className="flex-1 flex flex-wrap gap-1">
                    {(config?.secrets || []).map(secret => (
                      <span key={secret} className="px-2 py-1 rounded border border-[var(--border-primary)] text-[11px]">
                        {secret}
                      </span>
                    ))}
                    {!config?.secrets?.length && <span className="text-[var(--text-secondary)]">—</span>}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Volumes</span>
                  <div className="flex-1 space-y-1">
                    {(config?.volumes || []).map(volume => (
                      <div key={volume} className="font-mono text-xs bg-white dark:bg-slate-900 border border-[var(--border-primary)] rounded px-2 py-1">
                        {volume}
                      </div>
                    ))}
                    {!config?.volumes?.length && <span className="text-[var(--text-secondary)]">—</span>}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Include</span>
                  <span className="font-mono break-words">{config?.include || '—'}</span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-[var(--text-secondary)] w-28">Variables</span>
                  <div className="flex-1 space-y-1">
                    {config?.variables &&
                      Object.entries(config.variables).map(([key, value]) => (
                        <div key={key} className="font-mono text-xs bg-white dark:bg-slate-900 border border-[var(--border-primary)] rounded px-2 py-1">
                          {key}: {value}
                        </div>
                      ))}
                    {(!config?.variables || Object.keys(config.variables || {}).length === 0) && (
                      <span className="text-[var(--text-secondary)]">—</span>
                    )}
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-secondary)]">
                <div className="flex items-center gap-2">
                  <span>Ignore failure</span>
                  <span className="text-[var(--text-primary)] font-semibold">{config?.ignore_failure ? 'true' : 'false'}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span>Sync</span>
                  <span className="text-[var(--text-primary)] font-semibold">{config?.sync ? 'true' : 'false'}</span>
                </div>
                {config?.llm_output_sharing !== undefined && (
                  <div className="flex items-center gap-2 col-span-2">
                    <span>LLM Output Sharing</span>
                    <span className="text-[var(--text-primary)] font-semibold">{config.llm_output_sharing ? 'true' : 'false'}</span>
                  </div>
                )}
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 space-y-3">
            <div className="flex items-center justify-between">
              <p className="text-sm font-semibold text-[var(--text-primary)]">Task details</p>
            {selectedTask && !isSingleSyntheticTask && (
              <span className={`runner-pill border ${getStatusMeta(selectedTask.status, true).pillClass} text-xs`}>
                {getStatusMeta(selectedTask.status, true).text}
              </span>
            )}
            {(isSingleSyntheticTask || !hasTasks) && (
              <span className={`runner-pill border ${getStatusMeta(step?.status, true).pillClass} text-xs`}>
                {getStatusMeta(step?.status, true).text}
              </span>
            )}
          </div>
          {!selectedTask && hasTasks && !isSingleSyntheticTask && (
            <p className="text-sm text-[var(--text-secondary)]">Click a task in the graph to see its details here.</p>
          )}
          {showStepLevelInfo && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="text-base font-semibold text-[var(--text-primary)]">{step?.name}</div>
                <div className="text-xs text-[var(--text-secondary)] font-mono">
                    Duration: {step ? formatStepDuration(step) : '—'}
                  </div>
                </div>
                {(stepGoal || stepScript) && (
                  <div className="space-y-2">
                    {stepGoal && <p className="text-sm text-[var(--text-primary)]">Goal: <span className="text-[var(--text-secondary)]">{stepGoal}</span></p>}
                    {stepScript && (
                      <div>
                        <p className="text-xs text-[var(--text-secondary)] mb-1">Script</p>
                        <pre className="text-xs font-mono whitespace-pre-wrap bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-2 text-[var(--text-primary)]">
                          {stepScript}
                        </pre>
                      </div>
                    )}
                  </div>
                )}
                <div className="grid gap-3 sm:grid-cols-2 text-sm text-[var(--text-primary)]">
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Depends on</span>
                    <span className="font-mono">{(step?.depends_on || []).join(', ') || 'None'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Started</span>
                    <span className="font-mono">{step?.started_at || '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Finished</span>
                    <span className="font-mono">{step?.finished_at || '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Ignore failure</span>
                    <span className="font-mono">{step?.configuration?.ignore_failure ? 'true' : 'false'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Sync</span>
                    <span className="font-mono">{step?.configuration?.sync ? 'true' : 'false'}</span>
                  </div>
                  {step?.configuration?.include && (
                    <div className="flex items-center gap-2 sm:col-span-2">
                      <span className="text-[var(--text-secondary)]">Include</span>
                      <span className="font-mono">{step.configuration.include}</span>
                    </div>
                  )}
                  {step?.configuration?.variables && Object.keys(step.configuration.variables).length > 0 && (
                    <div className="sm:col-span-2 space-y-1">
                      <p className="text-xs text-[var(--text-secondary)]">Variables</p>
                      {Object.entries(step.configuration.variables).map(([key, value]) => (
                        <div key={key} className="font-mono bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-1 text-xs">
                          {key}: {value}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                {configTasks.length > 0 && (
                  <div className="space-y-2">
                    <p className="text-xs text-[var(--text-secondary)] uppercase tracking-wide">Step directives</p>
                    <div className="space-y-2">
                      {configTasks.map(def => (
                        <div key={def.name} className="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm font-semibold text-[var(--text-primary)]">{def.name || 'Unnamed task'}</span>
                            <span className="text-xs text-[var(--text-secondary)]">
                              {def.depends_on?.length ? `Depends on: ${def.depends_on.join(', ')}` : 'No dependencies'}
                            </span>
                          </div>
                          {def.goal && <p className="text-xs text-[var(--text-secondary)]">Goal: {def.goal}</p>}
                          {def.script && (
                            <pre className="text-xs font-mono whitespace-pre-wrap bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded px-2 py-2 text-[var(--text-primary)]">
                              {def.script}
                            </pre>
                          )}
                          {def.variables && Object.keys(def.variables).length > 0 && (
                            <div className="space-y-1 text-xs">
                              <p className="text-[var(--text-secondary)]">Variables</p>
                              {Object.entries(def.variables).map(([key, value]) => (
                                <div key={key} className="font-mono bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded px-2 py-1">
                                  {key}: {value}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          {selectedTask && !isSingleSyntheticTask && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="text-base font-semibold text-[var(--text-primary)]">{selectedTask.task_name}</div>
                <div className="text-xs text-[var(--text-secondary)] font-mono">
                  Duration: {formatElapsedLabel(selectedTask.started_at, selectedTask.finished_at, '—')}
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2 text-sm text-[var(--text-primary)]">
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Dependencies</span>
                    <span className="font-mono">{(selectedTaskDefinition?.depends_on || []).join(', ') || 'None'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Exit code</span>
                    <span className="font-mono">{selectedTask.exit_code ?? '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Started</span>
                    <span className="font-mono">{selectedTask.started_at || '—'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[var(--text-secondary)]">Finished</span>
                    <span className="font-mono">{selectedTask.finished_at || '—'}</span>
                  </div>
                </div>
                {selectedTaskDefinition?.goal && (
                  <div className="text-sm text-[var(--text-secondary)]">
                    Goal: <span className="text-[var(--text-primary)]">{selectedTaskDefinition.goal}</span>
                  </div>
                )}
                {selectedTaskDefinition?.script && (
                  <div>
                    <p className="text-xs text-[var(--text-secondary)] mb-1">Script</p>
                    <pre className="text-xs font-mono whitespace-pre-wrap bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-2 text-[var(--text-primary)]">
                      {selectedTaskDefinition.script}
                    </pre>
                  </div>
                )}
                {selectedTaskDefinition?.variables && Object.keys(selectedTaskDefinition.variables).length > 0 && (
                  <div className="space-y-1">
                    <p className="text-xs text-[var(--text-secondary)]">Variables</p>
                    {Object.entries(selectedTaskDefinition.variables).map(([key, value]) => (
                      <div key={key} className="font-mono bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded px-2 py-1 text-xs">
                        {key}: {value}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
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

function NewFolderModal({
  open,
  parentLabel,
  error,
  pending,
  onClose,
  onSubmit,
}: {
  open: boolean;
  parentLabel: string;
  error: string | null;
  pending: boolean;
  onClose: () => void;
  onSubmit: (name: string, description: string) => Promise<void>;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const nameInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (open) {
      setName('');
      setDescription('');
      requestAnimationFrame(() => nameInputRef.current?.focus());
    }
  }, [open]);

  if (!open) return null;

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    await onSubmit(name, description);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] px-4">
      <div className="w-full max-w-md bg-white dark:bg-slate-900 rounded-2xl shadow-2xl border border-[var(--border-primary)] overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-primary)]">
          <div>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">Create New Folder</h3>
            <p className="text-xs text-[var(--text-secondary)]">Parent: {parentLabel || 'Root'}</p>
          </div>
          <button type="button" className="pipelines-icon-only" aria-label="Close" onClick={onClose}>
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 6L6 18" />
              <path d="M6 6l12 12" />
            </svg>
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          <div className="space-y-2">
            <label htmlFor="new-folder-name" className="text-sm font-medium text-[var(--text-primary)]">
              Folder Name
            </label>
            <input
              ref={nameInputRef}
              id="new-folder-name"
              name="new-folder-name"
              type="text"
              required
              value={name}
              onChange={event => setName(event.target.value)}
              className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)] focus:border-[var(--border-accent)]"
              placeholder="Enter folder name"
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="new-folder-description" className="text-sm font-medium text-[var(--text-primary)]">
              Description <span className="text-[var(--text-secondary)]">(optional)</span>
            </label>
            <textarea
              id="new-folder-description"
              name="new-folder-description"
              value={description}
              onChange={event => setDescription(event.target.value)}
              rows={3}
              className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)] focus:border-[var(--border-accent)]"
              placeholder="Add a short summary for this folder"
            />
          </div>
          {error && <div className="text-sm text-red-600">{error}</div>}
          <div className="flex items-center justify-end gap-3 pt-2">
            <button type="button" className="glass-button-subtle" onClick={onClose} disabled={pending}>
              Cancel
            </button>
            <button type="submit" className="glass-button-primary" disabled={pending}>
              {pending ? 'Creating…' : 'Create'}
            </button>
          </div>
        </form>
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
  if (STATUS_META[raw]) return raw;
  if (!complete) return raw || 'pending';
  return 'pending';
}

function getStatusMeta(status: string | undefined, complete?: boolean) {
  const normalized = normalizeStatus(status, complete);
  return STATUS_META[normalized] || STATUS_META.pending;
}

function runMatchesSearch(run: RunListItem, term: string): boolean {
  if (!term) return true;
  const haystack = [
    run.run_id,
    run.parent_run_id,
    run.pipeline_name,
    run.pipeline_path,
    run.pipeline_version,
    run.pipeline_source,
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

function formatRepoLabel(run: RunListItem) {
  const owner = (run.git_repo_owner || '').trim();
  const name = (run.git_repo_name || '').trim();
  if (owner && name) return `${owner}/${name}`;
  if (name) return name;
  if (owner) return owner;
  const path = (run.pipeline_path || '').trim().replace(/^\/+|\/+$/g, '');
  return path || 'Repository';
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
  const display = full.length > 12 ? `${full.slice(0, 8)}` : full;
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
    commit: (resolved.git_commit_sha || '').slice(0, 8),
    pusher: resolved.git_pusher_name || '',
    started_at: resolved.started_at,
  };
}

export { StepsGraph };
export type { StepDetail, RunListItem, PipelineDefinition, StepConfiguration, TaskDefinition, TaskDetail };
export default PipelineRunsPage;
