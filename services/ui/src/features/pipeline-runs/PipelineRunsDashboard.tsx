import { useMemo } from 'react';
import { ChevronRight } from 'lucide-react';
import type { RunListItem } from './contracts';
import {
  BranchIcon,
  CommitIcon,
  RunCard,
  RunCollection,
} from './PipelineRunCards';
import { PipelineRunsOverview } from './PipelineRunsOverview';
import {
  filterPipelineRuns,
  flattenRunsByBranch,
  type PipelineRunSourceFilter,
  type PipelineRunStatusFilter,
} from './overviewModel';
import {
  formatBranchDisplay,
  formatRepoLabel,
  formatTriggerId,
  runActivityTimestamp,
  runTimestamp,
  summarizeStatus,
  timeAgo,
  type Team,
} from './runPresentation';
import { getStatusMeta, normalizeStatus } from './statusPresentation';

type TabKey = 'main' | 'recent' | 'events';

type TriggerTeam = {
  id: string;
  runs: RunListItem[];
  status: string;
  latestRun?: RunListItem;
};

export function PipelineRunsDashboard({
  activeTab,
  teams,
  teamsLoading,
  teamsError,
  onSelectTeam,
  activeTeamId,
  activeTeamURLValue,
  runsByBranch,
  recentRuns,
  teamedEvents,
  viewMode,
  runsLoading,
  runsError,
  searchTerm,
  sourceFilter,
  statusFilter,
  onOpenRun,
  onSelectRun,
  selectedRunIds,
  collapsedEvents,
  onToggleEventTeam,
  onCollapseAllEvents,
  onExpandAllEvents,
}: {
  activeTab: TabKey;
  teams: Team[];
  teamsLoading: boolean;
  teamsError: string | null;
  onSelectTeam: (id: number | null) => void;
  activeTeamId: number | null;
  activeTeamURLValue: string;
  runsByBranch: Record<string, RunListItem[]>;
  recentRuns: RunListItem[];
  teamedEvents: TriggerTeam[];
  viewMode: 'grid' | 'list';
  runsLoading: boolean;
  runsError: string | null;
  searchTerm: string;
  sourceFilter: PipelineRunSourceFilter;
  statusFilter: PipelineRunStatusFilter;
  onOpenRun: (id: string) => void;
  onSelectRun: (id: string) => void;
  selectedRunIds: Set<string>;
  collapsedEvents: Set<string>;
  onToggleEventTeam: (id: string) => void;
  onCollapseAllEvents: () => void;
  onExpandAllEvents: () => void;
}) {
  const flattenedBranchRuns = useMemo(() => flattenRunsByBranch(runsByBranch), [runsByBranch]);
  const overviewBaseRuns = flattenedBranchRuns.length ? flattenedBranchRuns : recentRuns;
  const overviewRuns = useMemo(
    () => filterPipelineRuns(overviewBaseRuns, { searchTerm, sourceFilter, statusFilter }),
    [overviewBaseRuns, searchTerm, sourceFilter, statusFilter]
  );
  const filteredRecentRuns = useMemo(
    () => filterPipelineRuns(recentRuns, { sourceFilter, statusFilter }),
    [recentRuns, sourceFilter, statusFilter]
  );
  const filteredEvents = useMemo(
    () =>
      teamedEvents
        .map(team => {
          const runs = filterPipelineRuns(team.runs, { sourceFilter, statusFilter });
          return {
            ...team,
            runs,
            status: summarizeStatus(runs),
            latestRun: [...runs].sort((left, right) => runTimestamp(right) - runTimestamp(left))[0],
          };
        })
        .filter(team => team.runs.length > 0),
    [sourceFilter, statusFilter, teamedEvents]
  );

  if (runsError) {
    return <div className="text-red-500 text-sm">{runsError}</div>;
  }

  if (activeTab === 'main') {
    return (
      <PipelineRunsOverview
        teams={teams}
        teamsLoading={teamsLoading}
        teamsError={teamsError}
        activeTeamId={activeTeamId}
        activeTeamURLValue={activeTeamURLValue}
        runs={overviewRuns}
        runsLoading={runsLoading}
        searchTerm={searchTerm}
        sourceFilter={sourceFilter}
        statusFilter={statusFilter}
        selectedRunIds={selectedRunIds}
        onSelectTeam={onSelectTeam}
        onOpenRun={onOpenRun}
        onSelectRun={onSelectRun}
      />
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
        {filteredEvents.length === 0 && !runsLoading ? (
          <div className="text-sm text-[var(--text-secondary)]">No trigger events yet.</div>
        ) : (
          <div className="space-y-3">
            {filteredEvents.map(team => (
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
        runs={filteredRecentRuns}
        viewMode={viewMode}
        onOpenRun={onOpenRun}
        onSelectRun={onSelectRun}
        selectedRunIds={selectedRunIds}
      />
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
  const timestamp = runActivityTimestamp(latestRun);
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
