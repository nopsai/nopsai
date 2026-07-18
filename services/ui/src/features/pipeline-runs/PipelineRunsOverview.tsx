import { useCallback, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Boxes,
  Check,
  CheckCircle2,
  ChevronRight,
  Clock3,
  GitBranch,
  Search,
  UsersRound,
} from 'lucide-react';
import { TreeColumnResizeHandle } from '../../components/resizableTreeColumn';
import { useResizableTreeColumn } from '../../components/resizableTreeColumnState';
import type { RunListItem } from './contracts';
import {
  ALL_PIPELINE_RUN_BRANCHES,
  buildPipelineRunBranchOptions,
  buildPipelineRunNavigationItems,
  buildPipelineRunOverviewMetrics,
  buildPipelineRunTableRows,
  filterPipelineRunsByBranch,
  type PipelineRunBranchOption,
  type PipelineRunSourceFilter,
  type PipelineRunStatusFilter,
} from './overviewModel';
import { isAppTeam, type Team } from './runPresentation';
import { buildPipelineRunsRoute } from '../../lib/teamRoutes';

type PipelineRunsOverviewProps = {
  teams: Team[];
  teamsLoading: boolean;
  teamsError: string | null;
  activeTeamId: number | null;
  activeTeamURLValue: string;
  runs: RunListItem[];
  runsLoading: boolean;
  searchTerm: string;
  sourceFilter: PipelineRunSourceFilter;
  statusFilter: PipelineRunStatusFilter;
  selectedRunIds: Set<string>;
  onSelectTeam: (teamId: number | null) => void;
  onOpenRun: (runId: string) => void;
  onSelectRun: (runId: string) => void;
};

export function PipelineRunsOverview({
  teams,
  teamsLoading,
  teamsError,
  activeTeamId,
  activeTeamURLValue,
  runs,
  runsLoading,
  searchTerm,
  sourceFilter,
  statusFilter,
  selectedRunIds,
  onSelectTeam,
  onOpenRun,
  onSelectRun,
}: PipelineRunsOverviewProps) {
  const [teamSearch, setTeamSearch] = useState('');
  const [expandedTeamIds, setExpandedTeamIds] = useState<Set<number>>(new Set());
  const [collapsedTeamIds, setCollapsedTeamIds] = useState<Set<number>>(new Set());
  const [branchSelection, setBranchSelection] = useState<{ teamId: number | null; key: string }>({
    teamId: null,
    key: ALL_PIPELINE_RUN_BRANCHES,
  });
  const activeTeam = useMemo(() => teams.find(team => team.id === activeTeamId) || null, [activeTeamId, teams]);
  const isActiveApplication = Boolean(activeTeam && isAppTeam(activeTeam));
  const branchOptions = useMemo(
    () => (isActiveApplication ? buildPipelineRunBranchOptions(runs) : []),
    [isActiveApplication, runs]
  );
  const selectedBranchKey = useMemo(() => {
    if (!isActiveApplication || branchSelection.teamId !== activeTeamId) return ALL_PIPELINE_RUN_BRANCHES;
    if (branchSelection.key === ALL_PIPELINE_RUN_BRANCHES) return ALL_PIPELINE_RUN_BRANCHES;
    return branchOptions.some(option => option.key === branchSelection.key) ? branchSelection.key : ALL_PIPELINE_RUN_BRANCHES;
  }, [activeTeamId, branchOptions, branchSelection, isActiveApplication]);
  const branchFilteredRuns = useMemo(
    () => (isActiveApplication ? filterPipelineRunsByBranch(runs, selectedBranchKey) : runs),
    [isActiveApplication, runs, selectedBranchKey]
  );
  const metrics = useMemo(() => buildPipelineRunOverviewMetrics(branchFilteredRuns), [branchFilteredRuns]);
  const rows = useMemo(() => buildPipelineRunTableRows(branchFilteredRuns, 30), [branchFilteredRuns]);
  const navigationItems = useMemo(
    () => buildPipelineRunNavigationItems(teams, activeTeamId, teamSearch, expandedTeamIds, collapsedTeamIds),
    [activeTeamId, collapsedTeamIds, expandedTeamIds, teamSearch, teams]
  );
  const recentHref = useMemo(
    () => buildRecentRunsHref(activeTeamURLValue, searchTerm, sourceFilter, statusFilter),
    [activeTeamURLValue, searchTerm, sourceFilter, statusFilter]
  );
  const attentionHref = useMemo(
    () => buildRecentRunsHref(activeTeamURLValue, searchTerm, sourceFilter, 'attention'),
    [activeTeamURLValue, searchTerm, sourceFilter]
  );
  const treeResize = useResizableTreeColumn({
    storageKey: 'pipeline-runs',
    defaultWidth: 248,
    minWidth: 216,
    maxWidth: 480,
  });
  const handleToggleTeam = useCallback((item: ReturnType<typeof buildPipelineRunNavigationItems>[number]) => {
    if (!item.childCount) return;
    setExpandedTeamIds(prev => {
      const next = new Set(prev);
      if (item.expanded) {
        next.delete(item.id);
      } else {
        next.add(item.id);
      }
      return next;
    });
    setCollapsedTeamIds(prev => {
      const next = new Set(prev);
      if (item.expanded) {
        next.add(item.id);
      } else {
        next.delete(item.id);
      }
      return next;
    });
  }, []);

  return (
    <div className="pipeline-runs-workspace" style={treeResize.gridStyle}>
      <PipelineRunTeamRail
        activeTeamId={activeTeamId}
        navigationItems={navigationItems}
        teamsLoading={teamsLoading}
        teamsError={teamsError}
        teamSearch={teamSearch}
        onTeamSearchChange={setTeamSearch}
        onSelectTeam={onSelectTeam}
        onToggleTeam={handleToggleTeam}
      />
      <TreeColumnResizeHandle {...treeResize} label="Resize pipeline run team tree" />

      <div className="pipeline-runs-overview-main">
        <div className="pipeline-runs-metrics" data-testid="pipeline-runs-metrics">
          {metrics.map(metric => (
            <PipelineRunMetricCard
              key={metric.id}
              metric={metric}
              href={metric.id === 'attention' ? attentionHref : undefined}
            />
          ))}
        </div>

        <div className="pipeline-runs-content-grid">
          <section className="pipeline-runs-panel pipeline-runs-feed-panel" aria-labelledby="pipeline-runs-feed-title">
            <header className="pipeline-runs-panel-head">
              <div className="pipeline-runs-panel-title">
                <h2 id="pipeline-runs-feed-title">Live and recent runs</h2>
                <span>{runsLoading ? 'Loading' : `${rows.length} visible`}</span>
              </div>
              <div className="pipeline-runs-panel-actions">
                {isActiveApplication && branchOptions.length > 1 && (
                  <BranchFilter
                    branchOptions={branchOptions}
                    selectedBranchKey={selectedBranchKey}
                    onBranchChange={branchKey => setBranchSelection({ teamId: activeTeamId, key: branchKey })}
                  />
                )}
                <Link className="pipeline-runs-panel-link" to={recentHref}>View all</Link>
              </div>
            </header>
            <PipelineRunTable
              rows={rows}
              loading={runsLoading}
              selectedRunIds={selectedRunIds}
              onOpenRun={onOpenRun}
              onSelectRun={onSelectRun}
            />
          </section>
        </div>
      </div>
    </div>
  );
}

function PipelineRunTeamRail({
  activeTeamId,
  navigationItems,
  teamsLoading,
  teamsError,
  teamSearch,
  onTeamSearchChange,
  onSelectTeam,
  onToggleTeam,
}: {
  activeTeamId: number | null;
  navigationItems: ReturnType<typeof buildPipelineRunNavigationItems>;
  teamsLoading: boolean;
  teamsError: string | null;
  teamSearch: string;
  onTeamSearchChange: (value: string) => void;
  onSelectTeam: (teamId: number | null) => void;
  onToggleTeam: (item: ReturnType<typeof buildPipelineRunNavigationItems>[number]) => void;
}) {
  return (
    <aside className="pipeline-runs-scope-rail" aria-label="Pipeline run teams and applications">
      <div className="pipeline-runs-scope-head">
        <h2>Teams and applications</h2>
      </div>
      <label className="pipeline-runs-scope-search">
        <Search className="h-4 w-4" aria-hidden="true" />
        <span className="sr-only">Search pipeline run teams and applications</span>
        <input
          value={teamSearch}
          onChange={event => onTeamSearchChange(event.target.value)}
          placeholder="Find team or app"
        />
      </label>
      <div className="pipeline-runs-scope-list">
        <button
          type="button"
          className={`pipeline-runs-scope-item ${activeTeamId == null ? 'pipeline-runs-scope-item--active' : ''}`}
          onClick={() => onSelectTeam(null)}
          aria-pressed={activeTeamId == null}
        >
          <Boxes className="h-4 w-4" aria-hidden="true" />
          <span className="pipeline-runs-scope-label">All teams</span>
        </button>
        {teamsError && <div className="pipeline-runs-scope-error">{teamsError}</div>}
        {teamsLoading && <div className="pipeline-runs-scope-empty">Loading teams...</div>}
        {!teamsLoading && navigationItems.length === 0 && (
          <div className="pipeline-runs-scope-empty">No teams or applications found.</div>
        )}
        {!teamsLoading && <PipelineRunNavigationList items={navigationItems} onSelectTeam={onSelectTeam} onToggleTeam={onToggleTeam} />}
      </div>
    </aside>
  );
}

function PipelineRunNavigationList({
  items,
  onSelectTeam,
  onToggleTeam,
}: {
  items: ReturnType<typeof buildPipelineRunNavigationItems>;
  onSelectTeam: (teamId: number | null) => void;
  onToggleTeam: (item: ReturnType<typeof buildPipelineRunNavigationItems>[number]) => void;
}) {
  if (!items.length) return null;
  return (
    <>
      {items.map(item => (
        <div
          key={item.id}
          className="pipeline-runs-scope-row"
          style={{ paddingLeft: `${0.55 + item.level * 0.9}rem` }}
        >
          {item.childCount > 0 ? (
            <button
              type="button"
              className="pipeline-runs-scope-toggle"
              onClick={() => onToggleTeam(item)}
              aria-label={`${item.expanded ? 'Collapse' : 'Expand'} team ${item.label}`}
              aria-expanded={item.expanded}
            >
              <ChevronRight className={`h-3.5 w-3.5 ${item.expanded ? 'rotate-90' : ''}`} aria-hidden="true" />
            </button>
          ) : (
            <span className="pipeline-runs-scope-toggle-spacer" aria-hidden="true" />
          )}
          <button
            type="button"
            className={`pipeline-runs-scope-select ${item.active ? 'pipeline-runs-scope-select--active' : ''}`}
            onClick={() => onSelectTeam(item.id)}
            aria-label={`Open ${item.kind === 'application' ? 'application' : 'team'} ${item.label}`}
            aria-pressed={item.active}
          >
            {item.kind === 'application' ? <GitBranch className="h-4 w-4" aria-hidden="true" /> : <UsersRound className="h-4 w-4" aria-hidden="true" />}
            <span className="pipeline-runs-scope-label" title={item.path || item.label}>{item.label}</span>
          </button>
        </div>
      ))}
    </>
  );
}

function PipelineRunMetricCard({ metric, href }: { metric: ReturnType<typeof buildPipelineRunOverviewMetrics>[number]; href?: string }) {
  const content = (
    <>
      <div className="pipeline-runs-metric-top">
        <span>{metric.label}</span>
        <span className={`pipeline-runs-metric-icon pipeline-runs-tone-${metric.tone}`}>
          {metric.id === 'running' && <Activity className="h-4 w-4" aria-hidden="true" />}
          {metric.id === 'attention' && <AlertTriangle className="h-4 w-4" aria-hidden="true" />}
          {metric.id === 'success-rate' && <CheckCircle2 className="h-4 w-4" aria-hidden="true" />}
          {metric.id === 'median-duration' && <Clock3 className="h-4 w-4" aria-hidden="true" />}
        </span>
      </div>
      <strong>{metric.value}</strong>
      <span>{metric.note}</span>
    </>
  );

  if (href) {
    return (
      <Link className="pipeline-runs-metric pipeline-runs-metric--action" to={href} aria-label="Show pipelines that need attention">
        {content}
      </Link>
    );
  }

  return (
    <section className="pipeline-runs-metric">
      {content}
    </section>
  );
}

function BranchFilter({
  branchOptions,
  selectedBranchKey,
  onBranchChange,
}: {
  branchOptions: PipelineRunBranchOption[];
  selectedBranchKey: string;
  onBranchChange: (branchKey: string) => void;
}) {
  return (
    <label className="pipeline-runs-branch-filter">
      <GitBranch className="h-4 w-4" aria-hidden="true" />
      <span className="sr-only">Filter application runs by branch</span>
      <select
        value={selectedBranchKey}
        onChange={event => onBranchChange(event.target.value)}
      >
        <option value={ALL_PIPELINE_RUN_BRANCHES}>All branches</option>
        {branchOptions.map(option => (
          <option key={option.key} value={option.key}>
            {option.label} - {option.runCount} {option.runCount === 1 ? 'run' : 'runs'}
          </option>
        ))}
      </select>
    </label>
  );
}

function PipelineRunTable({
  rows,
  loading,
  selectedRunIds,
  onOpenRun,
  onSelectRun,
}: {
  rows: ReturnType<typeof buildPipelineRunTableRows>;
  loading: boolean;
  selectedRunIds: Set<string>;
  onOpenRun: (runId: string) => void;
  onSelectRun: (runId: string) => void;
}) {
  if (loading && !rows.length) {
    return <div className="pipeline-runs-empty-state">Loading pipeline runs...</div>;
  }
  if (!rows.length) {
    return <div className="pipeline-runs-empty-state">No runs match these filters.</div>;
  }
  return (
    <div className="pipeline-runs-table-wrap">
      <table className="pipeline-runs-table">
        <thead>
          <tr>
            <th>Status</th>
            <th>Pipeline run</th>
            <th>Repository</th>
            <th>Run ID</th>
            <th>Branch</th>
            <th>Started</th>
            <th>Duration</th>
            <th>
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map(row => {
            const selected = selectedRunIds.has(row.run.run_id);
            return (
              <tr key={row.run.run_id} data-trigger-id={row.run.trigger_event_id || undefined}>
                <td>
                  <StatusPill status={row.status} label={row.statusLabel} />
                </td>
                <td>
                  <button type="button" className="pipeline-runs-table-title" onClick={() => onOpenRun(row.run.run_id)}>
                    <span>{row.pipelineName}</span>
                  </button>
                </td>
                <td>
                  <span className="pipeline-runs-mono">{row.repoName}</span>
                </td>
                <td className="pipeline-runs-mono">{row.runID}</td>
                <td>
                  <span className="pipeline-runs-mono">{row.branchLabel}</span>
                </td>
                <td>{row.startedLabel}</td>
                <td className="pipeline-runs-mono">{row.durationLabel}</td>
                <td>
                  <div className="pipeline-runs-row-actions">
                    <button
                      type="button"
                      className={`pipeline-runs-icon-button ${selected ? 'pipeline-runs-icon-button--selected' : ''}`}
                      onClick={() => onSelectRun(row.run.run_id)}
                      aria-label={`${selected ? 'Deselect' : 'Select'} run ${row.run.run_id}`}
                      aria-pressed={selected}
                    >
                      <Check className="h-4 w-4" aria-hidden="true" />
                    </button>
                    <button
                      type="button"
                      className="pipeline-runs-icon-button"
                      onClick={() => onOpenRun(row.run.run_id)}
                      aria-label={`Open run ${row.run.run_id}`}
                    >
                      <ArrowRight className="h-4 w-4" aria-hidden="true" />
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function StatusPill({ status, label }: { status: string; label: string }) {
  return (
    <span className={`pipeline-runs-status pipeline-runs-status-${statusClass(status)}`}>
      <span aria-hidden="true" />
      {label}
    </span>
  );
}

function statusClass(status: string): string {
  if (status === 'success') return 'success';
  if (status === 'running') return 'running';
  if (status === 'waiting_approval') return 'waiting';
  if (status === 'failure' || status === 'failure (ignored)' || status === 'rejected') return 'failed';
  return 'pending';
}

function buildRecentRunsHref(
  activeTeamURLValue: string,
  searchTerm: string,
  sourceFilter: PipelineRunSourceFilter,
  statusFilter: PipelineRunStatusFilter
) {
  const params = new URLSearchParams();
  const query = searchTerm.trim();
  if (query) params.set('q', query);
  if (sourceFilter !== 'all') params.set('source', sourceFilter);
  if (statusFilter !== 'all') params.set('status', statusFilter);
  const search = params.toString();
  return `${buildPipelineRunsRoute('recent', activeTeamURLValue)}${search ? `?${search}` : ''}`;
}
