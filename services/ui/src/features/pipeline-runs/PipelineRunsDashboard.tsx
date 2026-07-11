import { useEffect, useMemo } from 'react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  Activity,
  ArrowUpRight,
  Boxes,
  ChevronRight,
  GitBranch,
  PlayCircle,
  Timer,
  User,
  UsersRound,
  Webhook,
} from 'lucide-react';
import type { RunListItem } from './contracts';
import {
  BranchIcon,
  BranchRunsSection,
  CommitIcon,
  RunCard,
  RunCollection,
  RunStatusIcon,
} from './PipelineRunCards';
import {
  buildRunSourceTeams,
  formatBranch,
  formatBranchDisplay,
  formatRepoLabel,
  formatTriggerId,
  teamDisplayName,
  teamRepositoryLabel,
  teamRepositoryURL,
  isAppTeam,
  runMatchesSearch,
  runTimestamp,
  timeAgo,
  type Team,
  type RepoSummary,
  type RunSourceTeam,
  type RunSourceKind,
} from './runPresentation';
import { getStatusMeta, normalizeStatus } from './statusPresentation';

type TabKey = 'main' | 'recent' | 'events';

type TriggerTeam = {
  id: string;
  runs: RunListItem[];
  status: string;
  latestRun?: RunListItem;
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
  teams,
  teamsLoading,
  teamsError,
  onSelectTeam,
  activeTeamId,
  activeTeamPath,
  runsByBranch,
  recentRuns,
  teamedEvents,
  viewMode,
  runsLoading,
  runsError,
  searchTerm,
  repoSummaries,
  fetchRepoSummary,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsedEvents,
  onToggleEventTeam,
  onCollapseAllEvents,
  onExpandAllEvents,
  collapsedBranches,
  onToggleBranch,
  onDeleteBranch,
}: {
  activeTab: TabKey;
  teams: Team[];
  teamsLoading: boolean;
  teamsError: string | null;
  onSelectTeam: (id: number | null) => void;
  activeTeamId: number | null;
  activeTeamPath: Team[];
  runsByBranch: Record<string, RunListItem[]>;
  recentRuns: RunListItem[];
  teamedEvents: TriggerTeam[];
  viewMode: 'grid' | 'list';
  runsLoading: boolean;
  runsError: string | null;
  searchTerm: string;
  repoSummaries: Map<number, RepoSummary>;
  fetchRepoSummary: (teamId: number) => Promise<void>;
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsedEvents: Set<string>;
  onToggleEventTeam: (id: string) => void;
  onCollapseAllEvents: () => void;
  onExpandAllEvents: () => void;
  collapsedBranches: Set<string>;
  onToggleBranch: (branch: string, scrollIntoView?: boolean) => void;
  onDeleteBranch: (branch: string) => void;
}) {
  const term = searchTerm.trim().toLowerCase();
  const effectiveViewMode = activeTab === 'main' ? 'grid' : viewMode;

  const childTeams = useMemo(() => {
    if (activeTab !== 'main') return [] as Team[];
    return teams.filter(g => (g.parent_id ?? null) === (activeTeamId ?? null));
  }, [activeTeamId, activeTab, teams]);

  const visibleTeams = useMemo(() => {
    if (activeTab !== 'main') return [] as Team[];
    if (!term) return childTeams;
    return childTeams.filter(team =>
      [team.name, team.repository_full_name, team.repo_url]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(term)
    );
  }, [activeTab, childTeams, term]);

  const repoTeams = useMemo(() => visibleTeams.filter(team => isAppTeam(team)), [visibleTeams]);
  const teamTeams = useMemo(() => visibleTeams.filter(team => !isAppTeam(team)), [visibleTeams]);

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

  const runSourceTeams = useMemo(
    () => buildRunSourceTeams(filteredRunsByBranch),
    [filteredRunsByBranch]
  );

  useEffect(() => {
    if (activeTab !== 'main') return;
    visibleTeams.forEach(team => {
      if (isAppTeam(team) && !repoSummaries.has(team.id)) {
        void fetchRepoSummary(team.id);
      }
    });
  }, [activeTab, fetchRepoSummary, repoSummaries, visibleTeams]);

  if (runsError) {
    return <div className="text-red-500 text-sm">{runsError}</div>;
  }

  if (activeTab === 'main') {
    const hasSearch = Boolean(term);
    const mainSearchRuns = hasSearch ? recentRuns : [];

    const activeTeam = activeTeamPath.length ? activeTeamPath[activeTeamPath.length - 1] : null;

    return (
      <div className="space-y-4">
        {teamsError && <div className="text-red-500 text-sm">{teamsError}</div>}

        {activeTeamPath.length > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center flex-wrap gap-2 text-sm text-[var(--text-secondary)]">
              <button
                type="button"
                className="runner-pill runner-pill--muted"
                onClick={() => onSelectTeam(null)}
                aria-label="Back to root teams"
              >
                All teams
              </button>
              {activeTeamPath.map((team: Team) => (
                <div key={team.id} className="flex items-center gap-2">
                  <span className="text-[var(--border-primary)]">/</span>
                  <button
                    type="button"
                    className={`runner-pill ${team.id === activeTeamId ? 'runner-pill--muted' : 'runner-pill--ghost'}`}
                    onClick={() => onSelectTeam(team.id)}
                  >
                    {team.name}
                  </button>
                </div>
              ))}
            </div>
            {activeTeam && (
              <Link className="glass-button-subtle" to={`/teams?team=${encodeURIComponent(String(activeTeam.id))}`}>
                Manage team
              </Link>
            )}
          </div>
        )}

        {hasSearch ? (
          <RunCollection runs={mainSearchRuns} viewMode={viewMode} onOpenRun={onOpenRun} onSelectRun={onSelectRun} selectedRunIds={selectedRunIds} />
        ) : teamsLoading && !teams.length ? (
          <div className="text-sm text-[var(--text-secondary)]">Loading teams...</div>
        ) : (
          <div className="space-y-7">
            <DashboardPanel
              title="Teams"
              count={teamTeams.length}
              icon={<UsersRound className="h-4 w-4" />}
              emptyLabel="No teams."
            >
              <TeamGrid
                teams={teamTeams}
                allTeams={teams}
                activeTeamId={activeTeamId}
                repoSummaries={repoSummaries}
                onSelect={onSelectTeam}
              />
            </DashboardPanel>

            <DashboardPanel
              title="Applications"
              count={repoTeams.length}
              icon={<Boxes className="h-4 w-4" />}
              emptyLabel="No applications."
            >
              <TeamGrid
                teams={repoTeams}
                allTeams={teams}
                activeTeamId={activeTeamId}
                repoSummaries={repoSummaries}
                onSelect={onSelectTeam}
              />
            </DashboardPanel>

            <DashboardPanel
              title="Runs"
              count={selectedRuns.length}
              icon={<Activity className="h-4 w-4" />}
              emptyLabel="No runs."
            >
              <RunSourcesView
                teams={runSourceTeams}
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
        {teamedEvents.length === 0 && !runsLoading ? (
          <div className="text-sm text-[var(--text-secondary)]">No trigger events yet.</div>
        ) : (
          <div className="space-y-3">
            {teamedEvents.map(team => (
              <EventCard
                key={team.id}
                team={team}
                collapsed={collapsedEvents.has(team.id)}
                onToggle={() => onToggleEventTeam(team.id)}
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
  teams,
  viewMode,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsedBranches,
  onToggleBranch,
  onDeleteBranch,
}: {
  teams: RunSourceTeam[];
  viewMode: 'grid' | 'list';
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsedBranches: Set<string>;
  onToggleBranch: (branch: string, scrollIntoView?: boolean) => void;
  onDeleteBranch: (branch: string) => void;
}) {
  if (!teams.length) return null;
  return (
    <div className="space-y-5">
      {teams.map(team => (
        <RunSourceSection
          key={team.kind}
          team={team}
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
  team,
  viewMode,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsedBranches,
  onToggleBranch,
  onDeleteBranch,
}: {
  team: RunSourceTeam;
  viewMode: 'grid' | 'list';
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsedBranches: Set<string>;
  onToggleBranch: (branch: string, scrollIntoView?: boolean) => void;
  onDeleteBranch: (branch: string) => void;
}) {
  const icon = runSourceIcon(team.kind);
  const branches = team.branches || {};
  const branchEntries = Object.entries(branches).sort(([a], [b]) => a.localeCompare(b));
  const sortedRuns = useMemo(() => [...team.runs].sort((a, b) => runTimestamp(b) - runTimestamp(a)), [team.runs]);

  return (
    <section className="pipeline-run-source-section">
      <header className="pipeline-run-source-header">
        <div className="pipeline-run-source-title">
          <span className="pipeline-run-source-icon" aria-hidden="true">{icon}</span>
          <h3>{team.label}</h3>
        </div>
        <span className="runner-pill runner-pill--muted">{team.runs.length}</span>
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

function TeamGrid({
  teams,
  allTeams,
  activeTeamId,
  repoSummaries,
  onSelect,
}: {
  teams: Team[];
  allTeams: Team[];
  activeTeamId: number | null;
  repoSummaries: Map<number, RepoSummary>;
  onSelect: (id: number) => void;
}) {
  if (!teams.length) return null;
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {teams.map(team => {
        const isRepo = isAppTeam(team);
        const description = (team.description || '').trim();
        const isActive = activeTeamId === team.id;
        const summary = repoSummaries.get(team.id);
        const applications = allTeams.filter(child => (child.parent_id ?? null) === team.id && isAppTeam(child)).length;
        const subteams = allTeams.filter(child => (child.parent_id ?? null) === team.id && !isAppTeam(child)).length;
        const displayName = teamDisplayName(team);
        const repoURL = teamRepositoryURL(team);
        const repoLabel = teamRepositoryLabel(team);
        if (isRepo) {
          return (
            <div
              key={team.id}
              role="button"
              tabIndex={0}
              onClick={() => onSelect(team.id)}
              onKeyDown={event => {
                if (event.key === 'Enter') onSelect(team.id);
              }}
              className={`relative team bg-[var(--bg-secondary)] p-4 rounded-md hover:bg-[var(--bg-tertiary)] transition-colors duration-200 border border-[var(--border-primary)] hover:border-[var(--border-accent)] shadow-sm hover:shadow-lg flex flex-col justify-between min-h-[220px] ${isActive ? 'run-link-highlight' : ''}`}
            >
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
            key={team.id}
            role="button"
            tabIndex={0}
            onClick={() => onSelect(team.id)}
            onKeyDown={event => {
              if (event.key === 'Enter') onSelect(team.id);
            }}
            className={`pipeline-team-card border border-[var(--border-primary)] ${isActive ? 'run-link-highlight' : ''}`}
          >
            <div className="pipeline-team-card-header">
              <span className="pipeline-team-icon">
                <UsersRound className="h-6 w-6" aria-hidden="true" />
              </span>
              <h3 className="pipeline-team-title" title={displayName}>
                {displayName}
              </h3>
              <div className="pipeline-team-actions">
                <span className="pipeline-team-chevron" aria-hidden="true">
                  <ChevronRight className="h-4 w-4" />
                </span>
              </div>
            </div>
            {description && <p className="pipeline-team-description" title={description}>{description}</p>}
            <div className="pipeline-team-meta">
              <div className="pipeline-team-meta-row">
                <span className="pipeline-team-meta-label">Applications:</span>
                <span className="pipeline-team-meta-value">{applications}</span>
              </div>
              <div className="pipeline-team-meta-row">
                <span className="pipeline-team-meta-label">Teams:</span>
                <span className="pipeline-team-meta-value">{subteams}</span>
              </div>
            </div>
            {team.last_run_at && (
              <p className="mt-2 text-[11px] text-[var(--text-secondary)]">Last run {timeAgo(team.last_run_at)}</p>
            )}
          </div>
        );
      })}
    </div>
  );
}

function EventCard({
  team,
  collapsed,
  onToggle,
  onOpenRun,
}: {
  team: TriggerTeam;
  collapsed: boolean;
  onToggle: () => void;
  onOpenRun: (id: string) => void;
}) {
  const meta = getStatusMeta(team.status, team.status === 'success');
  const latestRun = team.latestRun || team.runs[0];
  const triggerLabel = formatTriggerId(team.id);
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
              {team.runs.length} {team.runs.length === 1 ? 'Pipeline' : 'Pipelines'}
            </span>
            <div className="flex items-center gap-1">
              {team.runs.slice(0, 6).map(run => (
                <span key={run.run_id} className={`h-2.5 w-2.5 rounded-full ${statusDotClass(run.status, run.is_complete)}`} />
              ))}
            </div>
          </div>
        </div>
      </button>
      {!collapsed && (
        <div className="p-4 border-t border-[var(--border-primary)] bg-[var(--bg-primary)]">
          <div className="grid gap-4 md:grid-cols-4 xl:grid-cols-4">
            {team.runs.map(run => (
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
