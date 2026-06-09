import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { NavLink, useParams, useSearchParams } from 'react-router-dom';
import { Grid2X2, List, Plus, Search, X } from 'lucide-react';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import { fetchGroupConfigRepository, requestPipelineRunsJson } from '../features/pipeline-runs/api';
import type {
  PipelineDefinition,
  RunListItem,
  StepDetail,
} from '../features/pipeline-runs/contracts';
import {
  buildGroupPath,
  extractLatestRunSummary,
  formatBranch,
  runMatchesSearch,
  summarizeStatus,
  type Group,
  type ParentRunInfo,
  type RepoSummary,
} from '../features/pipeline-runs/runPresentation';
import { RunLogsModal as LogsModal } from '../features/pipeline-runs/RunLogsModal';
import { PipelineRunsDashboard } from '../features/pipeline-runs/PipelineRunsDashboard';
import { RunDetailView } from '../features/pipeline-runs/RunDetailPanel';
import { PipelineDefinitionModal, StepDetailModal } from '../features/pipeline-runs/RunGraphModals';
import {
  FolderConfigRepositoryModal,
  NewFolderModal,
} from '../features/pipeline-runs/PipelineRunsModals';
import {
  buildConfigRepositoryWriteFiles,
  type ConfigRepositoryCommitResponse,
  type ConfigRepositoryDriftResponse,
} from '../lib/configRepositoryDrift';
import {
  createEmptyNotificationRouteForm,
  defaultNotificationRouteDefinition,
  normalizeNotificationRouteRecord,
  notificationRouteFormFromDefinition,
  notificationRoutePayloadFromForm,
} from '../features/pipeline-runs/notificationRoutes';
import type {
  NotificationRouteFormState,
  NotificationRouteRecord,
} from '../features/pipeline-runs/notificationRoutes';
type TabKey = 'main' | 'recent' | 'events';

type RunDetail = {
  run_info: RunListItem;
  steps: StepDetail[];
  pipeline_definition?: PipelineDefinition;
  pipeline_definition_yaml?: string;
  child_runs: RunListItem[];
  parent_run_info?: ParentRunInfo | null;
  approvals?: PipelineApproval[];
};

type PipelineApproval = {
  id: string;
  run_id: string;
  step_name: string;
  task_name: string;
  approval_type: string;
  assigned_groups: string[];
  allow_self_approval: boolean;
  status: string;
  requested_at: string;
  requested_by_type?: string;
  requested_by_id?: string;
  decided_by_email?: string;
  decided_at?: string;
  decision_comment?: string;
};

type TriggerGroup = {
  id: string;
  runs: RunListItem[];
  status: string;
  latestRun?: RunListItem;
};

type ConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
  last_sync_status: string;
  last_sync_message?: string;
  last_sync_started_at?: string;
  last_sync_completed_at?: string;
  last_sync_commit_sha?: string;
};

type ConfigRepositoryFormState = {
  repo_url: string;
  branch: string;
  base_path: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
};

type NewFolderPayload = {
  kind: 'group' | 'app';
  name: string;
  description: string;
  repoURL: string;
};

const tabs = [
  { id: 'main', label: 'Main' },
  { id: 'recent', label: 'Recent' },
  { id: 'events', label: 'Events' },
];

function isReservedRootGroupName(name: string) {
  const normalized = name.trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  return normalized === 'root' || normalized === '__general__';
}

const RECENT_FETCH_SIZE = 60;
const RECENT_INITIAL_BATCH = 30;
const RECENT_BATCH_SIZE = 20;

const emptyConfigRepositoryForm: ConfigRepositoryFormState = {
  repo_url: '',
  branch: 'main',
  base_path: '',
  enabled: true,
  write_enabled: false,
  write_branch: 'nopsai/ui-changes',
};

const emptyNotificationRouteForm = createEmptyNotificationRouteForm();

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
  const [configRepoFolder, setConfigRepoFolder] = useState<{ group: Group; folderPath: string } | null>(null);
  const [configRepo, setConfigRepo] = useState<ConfigRepository | null>(null);
  const [configRepoForm, setConfigRepoForm] = useState<ConfigRepositoryFormState>(emptyConfigRepositoryForm);
  const [configRepoLoading, setConfigRepoLoading] = useState(false);
  const [configRepoSaving, setConfigRepoSaving] = useState(false);
  const [configRepoSyncing, setConfigRepoSyncing] = useState(false);
  const [configRepoError, setConfigRepoError] = useState<string | null>(null);
  const [configRepoDriftOpen, setConfigRepoDriftOpen] = useState(false);
  const [configRepoDrift, setConfigRepoDrift] = useState<ConfigRepositoryDriftResponse | null>(null);
  const [configRepoDriftLoading, setConfigRepoDriftLoading] = useState(false);
  const [configRepoDriftError, setConfigRepoDriftError] = useState<string | null>(null);
  const [configRepoPushing, setConfigRepoPushing] = useState(false);
  const [configRepoPushResult, setConfigRepoPushResult] = useState<ConfigRepositoryCommitResponse | null>(null);
  const [configRepoManageAllowed, setConfigRepoManageAllowed] = useState(false);
  const [configRepoSyncAllowed, setConfigRepoSyncAllowed] = useState(false);
  const [notificationRoute, setNotificationRoute] = useState<NotificationRouteRecord | null>(null);
  const [notificationRouteForm, setNotificationRouteForm] = useState<NotificationRouteFormState>(emptyNotificationRouteForm);
  const [notificationRouteLoading, setNotificationRouteLoading] = useState(false);
  const [notificationRouteSaving, setNotificationRouteSaving] = useState(false);
  const [notificationRouteError, setNotificationRouteError] = useState<string | null>(null);
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

  const checkAccessPermission = useCallback(async (action: string, resourceType: string, resourceID: string) => {
    const params = new URLSearchParams({
      action,
      resource_type: resourceType,
      resource_id: resourceID,
    });
    try {
      const payload = await fetchJson<{ allowed?: boolean }>(`/v1/access/effective-permissions?${params.toString()}`);
      return Boolean(payload?.allowed);
    } catch {
      return false;
    }
  }, [fetchJson]);

  const normalizeConfigRepository = useCallback((payload: unknown): ConfigRepository | null => {
    if (!payload || typeof payload !== 'object') return null;
    const record = payload as Record<string, unknown>;
    const id = typeof record.id === 'number' ? record.id : Number(record.id);
    return {
      id: Number.isFinite(id) ? id : 0,
      scope_type: typeof record.scope_type === 'string' ? record.scope_type : '',
      scope_id: typeof record.scope_id === 'string' ? record.scope_id : '',
      repo_url: typeof record.repo_url === 'string' ? record.repo_url : '',
      branch: typeof record.branch === 'string' && record.branch.trim() ? record.branch : 'main',
      base_path: typeof record.base_path === 'string' ? record.base_path : '',
      enabled: Boolean(record.enabled),
      write_enabled: Boolean(record.write_enabled),
      write_branch: typeof record.write_branch === 'string' && record.write_branch.trim() ? record.write_branch : 'nopsai/ui-changes',
      last_sync_status: typeof record.last_sync_status === 'string' ? record.last_sync_status : '',
      last_sync_message: typeof record.last_sync_message === 'string' ? record.last_sync_message : undefined,
      last_sync_started_at: typeof record.last_sync_started_at === 'string' ? record.last_sync_started_at : undefined,
      last_sync_completed_at: typeof record.last_sync_completed_at === 'string' ? record.last_sync_completed_at : undefined,
      last_sync_commit_sha: typeof record.last_sync_commit_sha === 'string' ? record.last_sync_commit_sha : undefined,
    };
  }, []);

  const normalizeNotificationRoute = useCallback((payload: unknown): NotificationRouteRecord => {
    return normalizeNotificationRouteRecord(payload);
  }, []);

  const loadFolderConfigRepository = useCallback(
    async (folderPath: string, opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setConfigRepoLoading(true);
        setConfigRepoError(null);
      }
      try {
        const payload = await fetchGroupConfigRepository(folderPath);
        if (!payload) {
          setConfigRepo(null);
          setConfigRepoForm(emptyConfigRepositoryForm);
          return;
        }
        const repo = normalizeConfigRepository(payload);
        setConfigRepo(repo);
        setConfigRepoForm(repo ? {
          repo_url: repo.repo_url,
          branch: repo.branch || 'main',
          base_path: repo.base_path || '',
          enabled: repo.enabled,
          write_enabled: repo.write_enabled,
          write_branch: repo.write_branch || 'nopsai/ui-changes',
        } : emptyConfigRepositoryForm);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to load config repository';
        setConfigRepoError(message);
      } finally {
        if (!opts?.quiet) {
          setConfigRepoLoading(false);
        }
      }
    },
    [normalizeConfigRepository]
  );

  const loadFolderNotificationRoute = useCallback(
    async (folderPath: string, opts?: { quiet?: boolean }) => {
      if (!opts?.quiet) {
        setNotificationRouteLoading(true);
        setNotificationRouteError(null);
      }
      try {
        const route = normalizeNotificationRoute(
          await fetchJson<NotificationRouteRecord>(`/v1/groups/${encodeURIComponent(folderPath)}/notifications`)
        );
        setNotificationRoute(route);
        setNotificationRouteForm(notificationRouteFormFromDefinition(route.definition));
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to load notification policy';
        setNotificationRouteError(message);
      } finally {
        if (!opts?.quiet) {
          setNotificationRouteLoading(false);
        }
      }
    },
    [fetchJson, normalizeNotificationRoute]
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
    if (!configRepoFolder) return undefined;
    let cancelled = false;
    setConfigRepoManageAllowed(false);
    setConfigRepoSyncAllowed(false);

    void Promise.all([
      loadFolderConfigRepository(configRepoFolder.folderPath),
      loadFolderNotificationRoute(configRepoFolder.folderPath),
      checkAccessPermission('config_repo.manage', 'folder', configRepoFolder.folderPath),
      checkAccessPermission('config_repo.sync', 'folder', configRepoFolder.folderPath),
    ]).then(([, , manageAllowed, syncAllowed]) => {
      if (cancelled) return;
      setConfigRepoManageAllowed(manageAllowed);
      setConfigRepoSyncAllowed(syncAllowed);
    });

    return () => {
      cancelled = true;
    };
  }, [checkAccessPermission, configRepoFolder, loadFolderConfigRepository, loadFolderNotificationRoute]);

  useEffect(() => {
    if (!configRepoFolder || configRepo?.last_sync_status !== 'running') return undefined;
    const handle = window.setInterval(() => {
      void loadFolderConfigRepository(configRepoFolder.folderPath, { quiet: true });
    }, 3000);
    return () => window.clearInterval(handle);
  }, [configRepo?.last_sync_status, configRepoFolder, loadFolderConfigRepository]);

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
      if (!window.confirm('Delete this group? Runs will remain attached to the repository.')) return;
      try {
        await fetchJson(`/v1/groups/${groupId}`, { method: 'DELETE' });
        if (activeGroupId === groupId) updateSearchParams({ group: null });
        await Promise.all([loadGroups(), loadRuns()]);
        window.dispatchEvent(new Event('nopsai-resource-groups-changed'));
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to delete group';
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
    async ({ kind, name, description, repoURL }: NewFolderPayload) => {
      const trimmedName = name.trim();
      const trimmedDescription = description.trim();
      const trimmedRepoURL = repoURL.trim();
      if (!trimmedName) {
        setNewFolderError(kind === 'app' ? 'App name is required.' : 'Group name is required.');
        return;
      }
      if (kind === 'group' && isReservedRootGroupName(trimmedName)) {
        setNewFolderError('Root is reserved and cannot be used as a group name.');
        return;
      }
      if (kind === 'app' && !trimmedRepoURL) {
        setNewFolderError('Repository URL is required for apps.');
        return;
      }
      setNewFolderPending(true);
      setNewFolderError(null);
      try {
        await fetchJson('/v1/groups', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            kind,
            name: trimmedName,
            description: kind === 'group' ? trimmedDescription || undefined : undefined,
            repo_url: kind === 'app' ? trimmedRepoURL : undefined,
            parent_id: activeGroupId,
          }),
        });
        setNewFolderOpen(false);
        setNewFolderPending(false);
        await loadGroups();
        window.dispatchEvent(new Event('nopsai-resource-groups-changed'));
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to create group';
        setNewFolderError(message);
        setNewFolderPending(false);
      }
    },
    [activeGroupId, fetchJson, loadGroups]
  );

  const openFolderConfigRepository = useCallback(
    (group: Group) => {
      const folderPath = buildGroupPath(group.id, groups).map(item => item.name).join('/');
      if (!folderPath) return;
      setConfigRepoFolder({ group, folderPath });
      setConfigRepo(null);
      setConfigRepoForm(emptyConfigRepositoryForm);
      setConfigRepoError(null);
      setNotificationRoute(null);
      setNotificationRouteForm(emptyNotificationRouteForm);
      setNotificationRouteError(null);
      setConfigRepoDriftOpen(false);
      setConfigRepoDrift(null);
      setConfigRepoDriftError(null);
      setConfigRepoPushResult(null);
      setConfigRepoManageAllowed(false);
      setConfigRepoSyncAllowed(false);
    },
    [groups]
  );

  const closeFolderConfigRepository = useCallback(() => {
    setConfigRepoFolder(null);
    setConfigRepo(null);
    setConfigRepoForm(emptyConfigRepositoryForm);
    setConfigRepoError(null);
    setNotificationRoute(null);
    setNotificationRouteForm(emptyNotificationRouteForm);
    setNotificationRouteError(null);
    setConfigRepoDriftOpen(false);
    setConfigRepoDrift(null);
    setConfigRepoDriftError(null);
    setConfigRepoPushResult(null);
    setConfigRepoSaving(false);
    setConfigRepoSyncing(false);
    setConfigRepoDriftLoading(false);
    setConfigRepoPushing(false);
    setNotificationRouteSaving(false);
    setNotificationRouteLoading(false);
  }, []);

  const saveFolderConfigRepository = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || configRepoSaving) return;
    const repoURL = configRepoForm.repo_url.trim();
    if (!repoURL) {
      setConfigRepoError('Repository URL is required.');
      return;
    }
    setConfigRepoSaving(true);
    setConfigRepoError(null);
    try {
      const repo = await fetchJson<ConfigRepository>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          repo_url: repoURL,
          branch: configRepoForm.branch.trim() || 'main',
          base_path: configRepoForm.base_path.trim(),
          enabled: Boolean(configRepoForm.enabled),
          write_enabled: Boolean(configRepoForm.write_enabled),
          write_branch: configRepoForm.write_branch.trim(),
        }),
      });
      const normalized = normalizeConfigRepository(repo);
      setConfigRepo(normalized);
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
      if (normalized) {
        setConfigRepoForm({
          repo_url: normalized.repo_url,
          branch: normalized.branch || 'main',
          base_path: normalized.base_path || '',
          enabled: normalized.enabled,
          write_enabled: normalized.write_enabled,
          write_branch: normalized.write_branch || 'nopsai/ui-changes',
        });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save config repository';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSaving(false);
    }
  }, [configRepoFolder, configRepoForm, configRepoManageAllowed, configRepoSaving, fetchJson, normalizeConfigRepository]);

  const deleteFolderConfigRepository = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || configRepoSaving) return;
    if (!window.confirm('Remove the config repository from this group? Synced resources will remain available.')) return;
    setConfigRepoSaving(true);
    setConfigRepoError(null);
    try {
      await fetchJson<void>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo`, { method: 'DELETE' });
      setConfigRepo(null);
      setConfigRepoForm(emptyConfigRepositoryForm);
      setConfigRepoDriftOpen(false);
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to remove config repository';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSaving(false);
    }
  }, [configRepoFolder, configRepoManageAllowed, configRepoSaving, fetchJson]);

  const syncFolderConfigRepository = useCallback(async () => {
    if (!configRepoFolder || !configRepoSyncAllowed || configRepoSyncing || configRepo?.last_sync_status === 'running') return;
    setConfigRepoSyncing(true);
    setConfigRepoError(null);
    try {
      await fetchJson(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo/sync`, { method: 'POST' });
      setConfigRepo(prev => prev ? {
        ...prev,
        last_sync_status: 'running',
        last_sync_message: 'Configuration synchronization started.',
        last_sync_started_at: new Date().toISOString(),
        last_sync_completed_at: undefined,
      } : prev);
      window.setTimeout(() => {
        void loadFolderConfigRepository(configRepoFolder.folderPath, { quiet: true });
      }, 1000);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to start config repository sync';
      setConfigRepoError(message);
    } finally {
      setConfigRepoSyncing(false);
    }
  }, [configRepo?.last_sync_status, configRepoFolder, configRepoSyncAllowed, configRepoSyncing, fetchJson, loadFolderConfigRepository]);

  const checkFolderConfigRepositoryDrift = useCallback(async () => {
    if (!configRepoFolder || configRepoDriftLoading) return;
    setConfigRepoDriftOpen(true);
    setConfigRepoDriftLoading(true);
    setConfigRepoDriftError(null);
    setConfigRepoPushResult(null);
    try {
      const payload = await fetchJson<ConfigRepositoryDriftResponse>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo/drift`);
      setConfigRepoDrift(payload);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to check config repository drift';
      setConfigRepoDriftError(message);
    } finally {
      setConfigRepoDriftLoading(false);
    }
  }, [configRepoDriftLoading, configRepoFolder, fetchJson]);

  const pushFolderConfigRepositoryDrift = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || configRepoPushing) return;
    const files = buildConfigRepositoryWriteFiles(configRepoDrift);
    if (!configRepoDrift || files.length === 0) return;
    if (!configRepoDrift.can_push) {
      setConfigRepoDriftError('Enable Git push and set a push branch before committing changes.');
      return;
    }
    setConfigRepoPushing(true);
    setConfigRepoDriftError(null);
    try {
      const result = await fetchJson<ConfigRepositoryCommitResponse>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/config-repo/write`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: configRepoDrift.push_message || 'Update Nopsai config',
          files,
        }),
      });
      setConfigRepoPushResult(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to push config repository changes';
      setConfigRepoDriftError(message);
    } finally {
      setConfigRepoPushing(false);
    }
  }, [configRepoDrift, configRepoFolder, configRepoManageAllowed, configRepoPushing, fetchJson]);

  const saveFolderNotificationRoute = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || notificationRouteSaving || notificationRoute?.managed_by_config_repo) return;
    setNotificationRouteSaving(true);
    setNotificationRouteError(null);
    try {
      const route = normalizeNotificationRoute(
        await fetchJson<NotificationRouteRecord>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/notifications`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(notificationRoutePayloadFromForm(notificationRouteForm)),
        })
      );
      setNotificationRoute(route);
      setNotificationRouteForm(notificationRouteFormFromDefinition(route.definition));
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save notification policy';
      setNotificationRouteError(message);
    } finally {
      setNotificationRouteSaving(false);
    }
  }, [
    configRepoFolder,
    configRepoManageAllowed,
    fetchJson,
    normalizeNotificationRoute,
    notificationRoute?.managed_by_config_repo,
    notificationRouteForm,
    notificationRouteSaving,
  ]);

  const deleteFolderNotificationRoute = useCallback(async () => {
    if (!configRepoFolder || !configRepoManageAllowed || notificationRouteSaving || notificationRoute?.managed_by_config_repo) return;
    if (!notificationRoute?.id) return;
    if (!window.confirm('Remove the notification policy from this group?')) return;
    setNotificationRouteSaving(true);
    setNotificationRouteError(null);
    try {
      await fetchJson<void>(`/v1/groups/${encodeURIComponent(configRepoFolder.folderPath)}/notifications`, { method: 'DELETE' });
      const definition = defaultNotificationRouteDefinition();
      setNotificationRoute({
        group_path: configRepoFolder.folderPath,
        definition,
        source: 'database',
        managed_by_config_repo: false,
      });
      setNotificationRouteForm(notificationRouteFormFromDefinition(definition));
      setConfigRepoDrift(null);
      setConfigRepoPushResult(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to remove notification policy';
      setNotificationRouteError(message);
    } finally {
      setNotificationRouteSaving(false);
    }
  }, [configRepoFolder, configRepoManageAllowed, fetchJson, notificationRoute?.id, notificationRoute?.managed_by_config_repo, notificationRouteSaving]);

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
                  aria-selected={activeTab === tab.id}
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
                    <Search className="h-4 w-4" aria-hidden="true" />
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
                      <X className="h-4 w-4" aria-hidden="true" />
                    </button>
                  )}
                </div>
                {activeTab === 'main' && (
                  <button
                    type="button"
                    className="pipelines-icon-only"
                    onClick={handleNewFolder}
                    aria-label="New group or app"
                    disabled={Boolean(trimmedSearch)}
                    title={trimmedSearch ? 'Clear search to create an item' : 'New group or app'}
                  >
                    <Plus className="h-4 w-4" aria-hidden="true" />
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
              onApprovalDecision={handleApprovalDecision}
              approvalDecisionPending={approvalDecisionPending}
            />
          ) : (
            <PipelineRunsDashboard
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
              onOpenConfigRepository={openFolderConfigRepository}
              onOpenRun={handleOpenRun}
              onSelectRun={handleRunSelect}
              selectedRunIds={selectedRunIds}
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

      {configRepoFolder && (
        <FolderConfigRepositoryModal
          folderLabel={configRepoFolder.folderPath}
          repo={configRepo}
          form={configRepoForm}
          loading={configRepoLoading}
          saving={configRepoSaving}
          syncing={configRepoSyncing}
          error={configRepoError}
          driftLoading={configRepoDriftLoading}
          pushing={configRepoPushing}
          notificationRoute={notificationRoute}
          notificationForm={notificationRouteForm}
          notificationLoading={notificationRouteLoading}
          notificationSaving={notificationRouteSaving}
          notificationError={notificationRouteError}
          canManage={configRepoManageAllowed}
          canSync={configRepoSyncAllowed}
          onChange={setConfigRepoForm}
          onNotificationChange={setNotificationRouteForm}
          onSave={saveFolderConfigRepository}
          onDelete={deleteFolderConfigRepository}
          onSync={syncFolderConfigRepository}
          onCheckDrift={checkFolderConfigRepositoryDrift}
          onSaveNotification={saveFolderNotificationRoute}
          onDeleteNotification={deleteFolderNotificationRoute}
          onClose={closeFolderConfigRepository}
        />
      )}

      {configRepoFolder && configRepoDriftOpen && (
        <ConfigRepositoryDriftModal
          title={`${configRepoFolder.folderPath} config repository`}
          drift={configRepoDrift}
          loading={configRepoDriftLoading}
          error={configRepoDriftError}
          pushing={configRepoPushing}
          pushResult={configRepoPushResult}
          canPush={configRepoManageAllowed && Boolean(configRepoDrift?.can_push)}
          onClose={() => setConfigRepoDriftOpen(false)}
          onRefresh={checkFolderConfigRepositoryDrift}
          onPush={pushFolderConfigRepositoryDrift}
        />
      )}
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
        <Grid2X2 className="h-4 w-4" aria-hidden="true" />
      </button>
      <button
        type="button"
        className={`runs-view-toggle__btn ${!isGrid ? 'runs-view-toggle__btn--active' : ''}`}
        aria-pressed={!isGrid}
        onClick={() => onChange('list')}
        title="List view"
      >
        <List className="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
  );
}

export default PipelineRunsPage;
