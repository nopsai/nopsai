import { useEffect, useMemo } from 'react';
import type { ReactNode } from 'react';
import {
  Activity,
  ArrowUpRight,
  Boxes,
  CalendarClock,
  ChevronRight,
  Folder,
  FolderTree,
  GitBranch,
  PlayCircle,
  Settings,
  Timer,
  Trash2,
  User,
  Webhook,
} from 'lucide-react';
import type { RunListItem } from './contracts';
import {
  buildRunSourceGroups,
  buildStatusTimeline,
  formatBranch,
  formatBranchDisplay,
  formatRepoLabel,
  formatTriggerId,
  getBranchStatusTone,
  getStatusDotClass,
  groupDisplayName,
  groupRepositoryLabel,
  groupRepositoryURL,
  isAppGroup,
  runMatchesSearch,
  runTimestamp,
  summarizeStatus,
  timeAgo,
  type Group,
  type RepoSummary,
  type RunSourceGroup,
  type RunSourceKind,
} from './runPresentation';
import { STATUS_META, getStatusMeta, normalizeStatus } from './statusPresentation';

type TabKey = 'main' | 'recent' | 'events';

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

function runSourceIcon(kind: RunSourceKind) {
  switch (kind) {
    case 'repository':
      return <GitBranch className="h-4 w-4" />;
    case 'schedule':
      return <Timer className="h-4 w-4" />;
    case 'external':
      return <Webhook className="h-4 w-4" />;
    default:
      return <PlayCircle className="h-4 w-4" />;
  }
}

export function PipelineRunsDashboard({
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
  onOpenConfigRepository,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
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
  onOpenConfigRepository: (group: Group) => void;
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
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
    return childGroups.filter(group =>
      [group.name, group.repository_full_name, group.repo_url]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(term)
    );
  }, [activeTab, childGroups, term]);

  const repoGroups = useMemo(() => visibleGroups.filter(group => isAppGroup(group)), [visibleGroups]);
  const folderGroups = useMemo(() => visibleGroups.filter(group => !isAppGroup(group)), [visibleGroups]);

  const filteredRunsByBranch = useMemo(() => {
    if (activeTab !== 'main') return runsByBranch;
    if (!term) return runsByBranch;
    return Object.entries(runsByBranch).reduce<Record<string, RunListItem[]>>((acc, [branch, runs]) => {
      const filtered = runs.filter(run => runMatchesSearch(run, term));
      if (filtered.length) acc[branch] = filtered;
      return acc;
    }, {});
  }, [activeTab, runsByBranch, term]);

  const selectedRuns = useMemo(() => {
    if (activeTab !== 'main') return [] as RunListItem[];
    return Object.values(filteredRunsByBranch)
      .flat()
      .sort((a, b) => runTimestamp(b) - runTimestamp(a));
  }, [activeTab, filteredRunsByBranch]);

  const runSourceGroups = useMemo(
    () => buildRunSourceGroups(filteredRunsByBranch),
    [filteredRunsByBranch]
  );

  useEffect(() => {
    if (activeTab !== 'main') return;
    visibleGroups.forEach(group => {
      if (isAppGroup(group) && !repoSummaries.has(group.id)) {
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

    const activeFolder = activeGroupPath.length ? activeGroupPath[activeGroupPath.length - 1] : null;

    return (
      <div className="space-y-4">
        {groupsError && <div className="text-red-500 text-sm">{groupsError}</div>}

        {activeGroupPath.length > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center flex-wrap gap-2 text-sm text-[var(--text-secondary)]">
              <button
                type="button"
                className="runner-pill runner-pill--muted"
                onClick={() => onSelectGroup(null)}
                aria-label="Back to root groups"
              >
                All groups
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
            {activeFolder && !isAppGroup(activeFolder) && (
              <button
                type="button"
                className="glass-button-subtle"
                onClick={() => onOpenConfigRepository(activeFolder)}
              >
                Config Repository
              </button>
            )}
          </div>
        )}

        {hasSearch ? (
          <RunCollection runs={mainSearchRuns} viewMode={viewMode} onOpenRun={onOpenRun} onSelectRun={onSelectRun} selectedRunIds={selectedRunIds} />
        ) : groupsLoading && !groups.length ? (
          <div className="text-sm text-[var(--text-secondary)]">Loading groups…</div>
        ) : (
          <div className="space-y-7">
            <DashboardPanel
              title="Subgroups"
              count={folderGroups.length}
              icon={<FolderTree className="h-4 w-4" />}
              emptyLabel="No subgroups."
            >
              <GroupGrid
                groups={folderGroups}
                allGroups={groups}
                activeGroupId={activeGroupId}
                repoSummaries={repoSummaries}
                onSelect={onSelectGroup}
                onDelete={onDeleteFolder}
                onOpenConfigRepository={onOpenConfigRepository}
              />
            </DashboardPanel>

            <DashboardPanel
              title="Applications"
              count={repoGroups.length}
              icon={<Boxes className="h-4 w-4" />}
              emptyLabel="No applications."
            >
              <GroupGrid
                groups={repoGroups}
                allGroups={groups}
                activeGroupId={activeGroupId}
                repoSummaries={repoSummaries}
                onSelect={onSelectGroup}
                onDelete={onDeleteFolder}
                onOpenConfigRepository={onOpenConfigRepository}
              />
            </DashboardPanel>

            <DashboardPanel
              title="Runs"
              count={selectedRuns.length}
              icon={<Activity className="h-4 w-4" />}
              emptyLabel="No runs."
            >
              <RunSourcesView
                groups={runSourceGroups}
                viewMode={viewMode}
                onOpenRun={onOpenRun}
                onSelectRun={onSelectRun}
                selectedRunIds={selectedRunIds}
                collapsedBranches={collapsedBranches}
                onToggleBranch={onToggleBranch}
                onDeleteBranch={onDeleteBranch}
              />
            </DashboardPanel>
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

function DashboardPanel({
  title,
  count,
  icon,
  emptyLabel,
  children,
}: {
  title: string;
  count: number;
  icon: ReactNode;
  emptyLabel: string;
  children: ReactNode;
}) {
  return (
    <section className="pipeline-dashboard-section">
      <header className="pipeline-dashboard-section-header">
        <div className="pipeline-dashboard-section-title">
          <span className="pipeline-dashboard-section-icon" aria-hidden="true">{icon}</span>
          <h2>{title}</h2>
        </div>
        <span className="runner-pill runner-pill--muted">{count}</span>
      </header>
      {count > 0 ? children : <div className="pipeline-dashboard-empty-state">{emptyLabel}</div>}
    </section>
  );
}

function RunSourcesView({
  groups,
  viewMode,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsedBranches,
  onToggleBranch,
  onDeleteBranch,
}: {
  groups: RunSourceGroup[];
  viewMode: 'grid' | 'list';
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsedBranches: Set<string>;
  onToggleBranch: (branch: string, scrollIntoView?: boolean) => void;
  onDeleteBranch: (branch: string) => void;
}) {
  if (!groups.length) return null;
  return (
    <div className="space-y-5">
      {groups.map(group => (
        <RunSourceSection
          key={group.kind}
          group={group}
          viewMode={viewMode}
          onOpenRun={onOpenRun}
          onSelectRun={onSelectRun}
          selectedRunIds={selectedRunIds}
          collapsedBranches={collapsedBranches}
          onToggleBranch={onToggleBranch}
          onDeleteBranch={onDeleteBranch}
        />
      ))}
    </div>
  );
}

function RunSourceSection({
  group,
  viewMode,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsedBranches,
  onToggleBranch,
  onDeleteBranch,
}: {
  group: RunSourceGroup;
  viewMode: 'grid' | 'list';
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsedBranches: Set<string>;
  onToggleBranch: (branch: string, scrollIntoView?: boolean) => void;
  onDeleteBranch: (branch: string) => void;
}) {
  const icon = runSourceIcon(group.kind);
  const branches = group.branches || {};
  const branchEntries = Object.entries(branches).sort(([a], [b]) => a.localeCompare(b));
  const sortedRuns = useMemo(() => [...group.runs].sort((a, b) => runTimestamp(b) - runTimestamp(a)), [group.runs]);

  return (
    <section className="pipeline-run-source-section">
      <header className="pipeline-run-source-header">
        <div className="pipeline-run-source-title">
          <span className="pipeline-run-source-icon" aria-hidden="true">{icon}</span>
          <h3>{group.label}</h3>
        </div>
        <span className="runner-pill runner-pill--muted">{group.runs.length}</span>
      </header>

      {branchEntries.length > 0 ? (
        <div className="space-y-4">
          {branchEntries.map(([branch, runs]) => (
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
      ) : (
        <RunCollection
          runs={sortedRuns}
          viewMode={viewMode}
          onOpenRun={onOpenRun}
          onSelectRun={onSelectRun}
          selectedRunIds={selectedRunIds}
        />
      )}
    </section>
  );
}

function GroupGrid({
  groups,
  allGroups,
  activeGroupId,
  repoSummaries,
  onSelect,
  onDelete,
  onOpenConfigRepository,
}: {
  groups: Group[];
  allGroups: Group[];
  activeGroupId: number | null;
  repoSummaries: Map<number, RepoSummary>;
  onSelect: (id: number) => void;
  onDelete: (id: number) => void;
  onOpenConfigRepository: (group: Group) => void;
}) {
  if (!groups.length) return null;
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {groups.map(group => {
        const isRepo = isAppGroup(group);
        const description = (group.description || '').trim();
        const isActive = activeGroupId === group.id;
        const summary = repoSummaries.get(group.id);
        const applications = allGroups.filter(child => (child.parent_id ?? null) === group.id && isAppGroup(child)).length;
        const subfolders = allGroups.filter(child => (child.parent_id ?? null) === group.id && !isAppGroup(child)).length;
        const displayName = groupDisplayName(group);
        const repoURL = groupRepositoryURL(group);
        const repoLabel = groupRepositoryLabel(group);
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
                <Trash2 className="h-5 w-5" aria-hidden="true" />
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
              {repoURL && (
                <a
                  href={repoURL}
                  target="_blank"
                  rel="noreferrer"
                  className="mt-3 inline-flex min-w-0 items-center gap-1.5 text-xs font-medium text-[var(--text-accent)] hover:underline"
                  title={repoURL}
                  onClick={event => event.stopPropagation()}
                >
                  <ArrowUpRight className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                  <span className="truncate">{repoLabel || repoURL}</span>
                </a>
              )}
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
                    <User className="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" aria-hidden="true" />
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
                <Folder className="h-6 w-6" aria-hidden="true" />
              </span>
              <h3 className="pipeline-folder-title" title={displayName}>
                {displayName}
              </h3>
              <div className="pipeline-folder-actions">
                <span className="pipeline-folder-chevron" aria-hidden="true">
                  <ChevronRight className="h-4 w-4" />
                </span>
                <button
                  className="pipelines-delete-button pipeline-folder-delete-btn"
                  type="button"
                  title="Config repository"
                  aria-label={`Config repository for ${displayName}`}
                  onClick={event => {
                    event.stopPropagation();
                    onOpenConfigRepository(group);
                  }}
                >
                  <Settings className="h-4 w-4" aria-hidden="true" />
                </button>
                <button
                  className="pipelines-delete-button pipeline-folder-delete-btn delete-group-btn"
                  type="button"
                  title="Delete group"
                  aria-label={`Delete ${displayName}`}
                  onClick={event => {
                    event.stopPropagation();
                    onDelete(group.id);
                  }}
                >
                  <Trash2 className="h-4 w-4" aria-hidden="true" />
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
                <span className="pipeline-folder-meta-label">Sub groups:</span>
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

export function BranchIcon({ className }: { className?: string }) {
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

export function CommitIcon({ className }: { className?: string }) {
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

export function ZapIcon({ className }: { className?: string }) {
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
          <ChevronRight
            className={`h-5 w-5 text-[var(--text-secondary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}
            aria-hidden="true"
          />
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
            <Trash2 className="h-4 w-4" aria-hidden="true" />
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

function getFailurePreview(reason?: string): { title: string; detail?: string } | null {
  const lines = (reason || '')
    .split('\n')
    .map(line => line.trim())
    .filter(Boolean);
  if (!lines.length) return null;
  const whyLine = lines.find(line => line.startsWith('Why: '));
  const decisionLine = lines.find(line => line.startsWith('Decision reason: '));
  return {
    title: lines[0],
    detail: whyLine || decisionLine,
  };
}

export function RunCard({
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
  const failurePreview = getFailurePreview(run.failure_reason);
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
            <span className="truncate" title="Source">{repoLabel}</span>
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
        {failurePreview && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-700/70 dark:bg-red-950/40 dark:text-red-200">
            <div className="font-semibold truncate" title={failurePreview.title}>{failurePreview.title}</div>
            {failurePreview.detail && (
              <div className="mt-1 truncate opacity-90" title={failurePreview.detail}>{failurePreview.detail}</div>
            )}
          </div>
        )}
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
  const failurePreview = getFailurePreview(run.failure_reason);
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
          {failurePreview && (
            <div className="mt-2 max-w-full rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-700/70 dark:bg-red-950/40 dark:text-red-200">
              <div className="font-semibold truncate" title={failurePreview.title}>{failurePreview.title}</div>
              {failurePreview.detail && (
                <div className="mt-1 truncate opacity-90" title={failurePreview.detail}>{failurePreview.detail}</div>
              )}
            </div>
          )}
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

export function RunIdIcon({ className }: { className?: string }) {
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
  const isFailure = normalized === 'failure' || normalized === 'failure (ignored)' || normalized === 'rejected';
  const isCancelled = normalized === 'cancelled';
  const isRunning = normalized === 'running' || normalized === 'waiting_approval';
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
  const badges: ReactNode[] = [];
  const external = run.trigger_source === 'external_trigger' || Boolean(run.external_trigger_id);
  if (external) {
    const label = run.external_trigger_name ? `External trigger: ${run.external_trigger_name}` : 'External trigger';
    badges.push(
      <span key="external" className="text-xs font-semibold text-[var(--text-link)] inline-flex items-center gap-1" title={label}>
        <Webhook className="h-4 w-4" />
        External
      </span>
    );
  }
  const scheduled = run.trigger_source === 'schedule' || Boolean(run.schedule_id);
  if (scheduled) {
    const label = run.schedule_name ? `Scheduled: ${run.schedule_name}` : 'Scheduled';
    badges.push(
      <span key="scheduled" className="text-xs font-semibold text-[var(--text-link)] inline-flex items-center gap-1" title={label}>
        <CalendarClock className="h-4 w-4" />
        Scheduled
      </span>
    );
  }
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

export function StatusBadge({ status, complete }: { status: string; complete?: boolean }) {
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
          <ChevronRight
            className={`h-4 w-4 text-[var(--text-secondary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}
            aria-hidden="true"
          />
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
