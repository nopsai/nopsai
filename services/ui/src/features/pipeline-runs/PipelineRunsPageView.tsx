import type { Dispatch, RefObject, SetStateAction } from 'react';
import { NavLink } from 'react-router-dom';
import { Grid2X2, List, Search, X } from 'lucide-react';
import type { RunListItem } from './contracts';
import type { Team } from './runPresentation';
import type { PipelineRunSourceFilter, PipelineRunStatusFilter } from './overviewModel';
import { PipelineRunsDashboard } from './PipelineRunsDashboard';
import { RunDetailView } from './RunDetailPanel';
import { PipelineDefinitionModal, StepDetailModal } from './RunGraphModals';
import { RunLogsModal as LogsModal } from './RunLogsModal';
import { buildPipelineRunsRoute } from '../../lib/teamRoutes';
import type {
  PipelineApproval,
  PipelineRunDetail,
  PipelineRunsTabKey,
  PipelineRunsTriggerTeam,
} from './pageTypes';

type SearchUpdateValue = string | number | null | undefined;

type PipelineRunsPageViewProps = {
  activeTab: PipelineRunsTabKey;
  activeTeamId: number | null;
  activeTeamURLValue: string;
  activeRunId: string | null;
  searchTerm: string;
  searchOpen: boolean;
  searchInputRef: RefObject<HTMLInputElement | null>;
  setSearchTerm: Dispatch<SetStateAction<string>>;
  setSearchOpen: Dispatch<SetStateAction<boolean>>;
  updateSearchParams: (updates: Record<string, SearchUpdateValue>) => void;
  viewMode: 'grid' | 'list';
  setViewMode: Dispatch<SetStateAction<'grid' | 'list'>>;
  sourceFilter: PipelineRunSourceFilter;
  statusFilter: PipelineRunStatusFilter;
  onSourceFilterChange: (filter: PipelineRunSourceFilter) => void;
  onStatusFilterChange: (filter: PipelineRunStatusFilter) => void;
  mainContentRef: RefObject<HTMLDivElement | null>;
  isViewingDetail: boolean;
  showSelectionBar: boolean;
  selectedRunIds: Set<string>;
  clearSelection: () => void;
  handleBulkDelete: () => Promise<void>;
  teams: Team[];
  teamsLoading: boolean;
  teamsError: string | null;
  runsByBranch: Record<string, RunListItem[]>;
  filteredRecentRuns: RunListItem[];
  teamedEvents: PipelineRunsTriggerTeam[];
  runsLoading: boolean;
  runsError: string | null;
  onSelectTeam: (teamId: number | null) => void;
  handleOpenRun: (runId: string) => void;
  handleRunSelect: (runId: string) => void;
  collapsedEvents: Set<string>;
  toggleEventTeam: (id: string) => void;
  collapseAllEvents: () => void;
  expandAllEvents: () => void;
  runDetail: PipelineRunDetail | null;
  runDetailLoading: boolean;
  runDetailError: string | null;
  handleCloseDetail: () => void;
  handleCancelRun: (runId: string) => Promise<void>;
  handleRerun: (runId: string) => Promise<void>;
  handleDeleteRun: (runId: string) => Promise<void>;
  selectedStep: string | null;
  setSelectedStep: Dispatch<SetStateAction<string | null>>;
  setLogsOpen: Dispatch<SetStateAction<boolean>>;
  setLogsStepFilter: Dispatch<SetStateAction<string | null>>;
  setLogsSearchFilter: Dispatch<SetStateAction<string | null>>;
  setStepDetailName: Dispatch<SetStateAction<string | null>>;
  setDefinitionOpen: Dispatch<SetStateAction<boolean>>;
  handleApprovalDecision: (approval: PipelineApproval, decision: 'approve' | 'reject') => Promise<void>;
  approvalDecisionPending: string | null;
  definitionOpen: boolean;
  logsOpen: boolean;
  logsStepFilter: string | null;
  logsSearchFilter: string | null;
  stepDetailName: string | null;
};

const tabs: Array<{ id: PipelineRunsTabKey; label: string }> = [
  { id: 'main', label: 'Overview' },
  { id: 'recent', label: 'All runs' },
  { id: 'events', label: 'Events' },
];

const sourceFilterOptions: Array<{ value: PipelineRunSourceFilter; label: string }> = [
  { value: 'all', label: 'All sources' },
  { value: 'repository', label: 'Application' },
  { value: 'schedule', label: 'Schedule' },
  { value: 'external', label: 'External' },
  { value: 'manual', label: 'Manual' },
];

const statusFilterOptions: Array<{ value: PipelineRunStatusFilter; label: string }> = [
  { value: 'all', label: 'Any status' },
  { value: 'attention', label: 'Needs attention' },
  { value: 'running', label: 'Running' },
  { value: 'failure', label: 'Failed' },
  { value: 'waiting_approval', label: 'Waiting approval' },
  { value: 'success', label: 'Success' },
  { value: 'pending', label: 'Pending' },
];

export function PipelineRunsPageView({
  activeTab,
  activeTeamId,
  activeTeamURLValue,
  activeRunId,
  searchTerm,
  searchOpen,
  searchInputRef,
  setSearchTerm,
  setSearchOpen,
  updateSearchParams,
  viewMode,
  setViewMode,
  sourceFilter,
  statusFilter,
  onSourceFilterChange,
  onStatusFilterChange,
  mainContentRef,
  isViewingDetail,
  showSelectionBar,
  selectedRunIds,
  clearSelection,
  handleBulkDelete,
  teams,
  teamsLoading,
  teamsError,
  runsByBranch,
  filteredRecentRuns,
  teamedEvents,
  runsLoading,
  runsError,
  onSelectTeam,
  handleOpenRun,
  handleRunSelect,
  collapsedEvents,
  toggleEventTeam,
  collapseAllEvents,
  expandAllEvents,
  runDetail,
  runDetailLoading,
  runDetailError,
  handleCloseDetail,
  handleCancelRun,
  handleRerun,
  handleDeleteRun,
  selectedStep,
  setSelectedStep,
  setLogsOpen,
  setLogsStepFilter,
  setLogsSearchFilter,
  setStepDetailName,
  setDefinitionOpen,
  handleApprovalDecision,
  approvalDecisionPending,
  definitionOpen,
  logsOpen,
  logsStepFilter,
  logsSearchFilter,
  stepDetailName,
}: PipelineRunsPageViewProps) {
  const tabRoute = (tab: PipelineRunsTabKey) => {
    const params = new URLSearchParams();
    const query = searchTerm.trim();
    if (query) params.set('q', query);
    if (sourceFilter !== 'all') params.set('source', sourceFilter);
    if (statusFilter !== 'all') params.set('status', statusFilter);
    const search = params.toString();
    return `${buildPipelineRunsRoute(tab, activeTeamURLValue)}${search ? `?${search}` : ''}`;
  };

  return (
    <div data-page="pipelineruns" className="active h-full min-h-0 flex flex-col overflow-hidden">
      <div className="px-6 pt-6 flex-shrink-0 tabs-nav-wrapper">
        <div className="border-b border-[var(--border-primary)]">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <nav className="tabs-nav" aria-label="Pipeline run tabs" role="tablist">
              {tabs.map(tab => (
                <NavLink
                  key={tab.id}
                  to={tabRoute(tab.id)}
                  role="tab"
                  aria-selected={activeTab === tab.id}
                  className={({ isActive }) => `tabs-nav__link ${isActive ? 'tabs-nav__link--active' : ''}`}
                  onClick={() => {
                    clearSelection();
                  }}
                >
                  {tab.label}
                </NavLink>
              ))}
            </nav>
          </div>
          {!isViewingDetail && (
            <div className="pipeline-runs-filterbar">
              <label className={`pipeline-runs-search-field ${searchOpen ? 'pipeline-runs-search-field--active' : ''}`}>
                <Search className="h-4 w-4" aria-hidden="true" />
                <span className="sr-only">Search pipeline runs</span>
                <input
                  ref={searchInputRef}
                  id="pipeline-runs-search"
                  type="text"
                  placeholder="Search pipeline, branch, commit, or run ID"
                  value={searchTerm}
                  onFocus={() => setSearchOpen(true)}
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
                    className="pipeline-runs-search-clear"
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
              </label>
              <div className="pipeline-runs-segmented" role="group" aria-label="Filter by run source">
                {sourceFilterOptions.map(option => (
                  <button
                    key={option.value}
                    type="button"
                    className={`pipeline-runs-segment ${sourceFilter === option.value ? 'pipeline-runs-segment--active' : ''}`}
                    aria-pressed={sourceFilter === option.value}
                    onClick={() => onSourceFilterChange(option.value)}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
              <select
                className="pipeline-runs-select"
                aria-label="Filter by run status"
                value={statusFilter}
                onChange={event => onStatusFilterChange(event.target.value as PipelineRunStatusFilter)}
              >
                {statusFilterOptions.map(option => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
              {activeTab === 'recent' && <ViewToggle viewMode={viewMode} onChange={setViewMode} />}
            </div>
          )}
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
                    onClick={() => void handleBulkDelete()}
                    className="inline-flex items-center px-3 py-1.5 border border-transparent rounded-md text-[var(--text-button)] bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 text-xs disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Delete Selected
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="flex-1 min-h-0 overflow-hidden">
        <main id="main-content-runs" ref={mainContentRef} className="pipeline-runs-main-scroll h-full min-h-0 overflow-y-auto p-6 space-y-4">
          {activeRunId ? (
            runDetail ? (
              <RunDetailView
                detail={runDetail}
                loading={runDetailLoading}
                error={runDetailError}
                onClose={handleCloseDetail}
                onCancel={() => void handleCancelRun(runDetail.run_info.run_id)}
                onRerun={() => void handleRerun(runDetail.run_info.run_id)}
                onDelete={() => void handleDeleteRun(runDetail.run_info.run_id)}
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
              <RunDetailLoadingState
                runId={activeRunId}
                loading={runDetailLoading}
                error={runDetailError}
                onClose={handleCloseDetail}
              />
            )
          ) : (
            <PipelineRunsDashboard
              activeTab={activeTab}
              teams={teams}
              teamsLoading={teamsLoading}
              teamsError={teamsError}
              onSelectTeam={onSelectTeam}
              activeTeamId={activeTeamId}
              activeTeamURLValue={activeTeamURLValue}
              runsByBranch={runsByBranch}
              recentRuns={filteredRecentRuns}
              teamedEvents={teamedEvents}
              viewMode={viewMode}
              runsLoading={runsLoading}
              runsError={runsError}
              searchTerm={searchTerm}
              sourceFilter={sourceFilter}
              statusFilter={statusFilter}
              onOpenRun={handleOpenRun}
              onSelectRun={handleRunSelect}
              selectedRunIds={selectedRunIds}
              collapsedEvents={collapsedEvents}
              onToggleEventTeam={toggleEventTeam}
              onCollapseAllEvents={collapseAllEvents}
              onExpandAllEvents={expandAllEvents}
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

    </div>
  );

}

function RunDetailLoadingState({
  runId,
  loading,
  error,
  onClose,
}: {
  runId: string;
  loading: boolean;
  error: string | null;
  onClose: () => void;
}) {
  return (
    <section className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5 shadow-sm">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="text-xs font-semibold uppercase text-[var(--text-muted)]">Pipeline run</div>
          <h2 className="mt-1 truncate text-xl font-semibold text-[var(--text-primary)]">{runId}</h2>
          <p className="mt-2 text-sm text-[var(--text-secondary)]">
            {error ? 'The run detail could not be loaded.' : loading ? 'Loading the selected run detail.' : 'Preparing the selected run detail.'}
          </p>
          {error ? <p className="mt-3 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{error}</p> : null}
        </div>
        <button type="button" className="glass-button-ghost shrink-0" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
          Close
        </button>
      </div>
    </section>
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
        aria-label="Grid view"
        onClick={() => onChange('grid')}
        title="Grid view"
      >
        <Grid2X2 className="h-4 w-4" aria-hidden="true" />
      </button>
      <button
        type="button"
        className={`runs-view-toggle__btn ${!isGrid ? 'runs-view-toggle__btn--active' : ''}`}
        aria-pressed={!isGrid}
        aria-label="List view"
        onClick={() => onChange('list')}
        title="List view"
      >
        <List className="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
  );
}
