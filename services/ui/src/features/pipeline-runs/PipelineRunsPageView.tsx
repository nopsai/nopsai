import type { Dispatch, RefObject, SetStateAction } from 'react';
import { NavLink } from 'react-router-dom';
import { Grid2X2, List, Plus, Search, X } from 'lucide-react';
import { ConfigRepositoryDriftModal } from '../../components/ConfigRepositoryDriftModal';
import type { RunListItem } from './contracts';
import type { Group, RepoSummary } from './runPresentation';
import { PipelineRunsDashboard } from './PipelineRunsDashboard';
import { RunDetailView } from './RunDetailPanel';
import { PipelineDefinitionModal, StepDetailModal } from './RunGraphModals';
import { RunLogsModal as LogsModal } from './RunLogsModal';
import { FolderConfigRepositoryModal, NewFolderModal } from './PipelineRunsModals';
import type {
  PipelineApproval,
  PipelineRunDetail,
  PipelineRunsNewFolderPayload,
  PipelineRunsTabKey,
  PipelineRunsTriggerGroup,
} from './pageTypes';
import type { FolderConfigRepositoryController } from './useFolderConfigRepositoryController';

type SearchUpdateValue = string | number | null | undefined;

type PipelineRunsPageViewProps = {
  activeTab: PipelineRunsTabKey;
  activeGroupId: number | null;
  activeGroupPath: Group[];
  activeGroupLabel: string;
  activeRunId: string | null;
  searchTerm: string;
  searchOpen: boolean;
  searchInputRef: RefObject<HTMLInputElement | null>;
  setSearchTerm: Dispatch<SetStateAction<string>>;
  setSearchOpen: Dispatch<SetStateAction<boolean>>;
  updateSearchParams: (updates: Record<string, SearchUpdateValue>) => void;
  viewMode: 'grid' | 'list';
  setViewMode: Dispatch<SetStateAction<'grid' | 'list'>>;
  mainContentRef: RefObject<HTMLDivElement | null>;
  isViewingDetail: boolean;
  showSelectionBar: boolean;
  trimmedSearch: string;
  selectedRunIds: Set<string>;
  clearSelection: () => void;
  handleBulkDelete: () => Promise<void>;
  handleNewFolder: () => void;
  groups: Group[];
  groupsLoading: boolean;
  groupsError: string | null;
  runsByBranch: Record<string, RunListItem[]>;
  filteredRecentRuns: RunListItem[];
  groupedEvents: PipelineRunsTriggerGroup[];
  runsLoading: boolean;
  runsError: string | null;
  repoSummaries: Map<number, RepoSummary>;
  fetchRepoSummary: (groupId: number) => Promise<void>;
  onSelectGroup: (groupId: number | null) => void;
  handleDeleteFolder: (groupId: number) => Promise<void>;
  handleOpenRun: (runId: string) => void;
  handleRunSelect: (runId: string) => void;
  collapsedEvents: Set<string>;
  toggleEventGroup: (id: string) => void;
  collapseAllEvents: () => void;
  expandAllEvents: () => void;
  collapsedBranches: Set<string>;
  toggleBranchCollapse: (branch: string, scrollIntoView?: boolean) => void;
  handleDeleteBranch: (branch: string) => Promise<void>;
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
  newFolderOpen: boolean;
  newFolderError: string | null;
  newFolderPending: boolean;
  setNewFolderOpen: Dispatch<SetStateAction<boolean>>;
  setNewFolderError: Dispatch<SetStateAction<string | null>>;
  setNewFolderPending: Dispatch<SetStateAction<boolean>>;
  submitNewFolder: (payload: PipelineRunsNewFolderPayload) => Promise<void>;
  folderConfig: FolderConfigRepositoryController;
};

const tabs: Array<{ id: PipelineRunsTabKey; label: string }> = [
  { id: 'main', label: 'Main' },
  { id: 'recent', label: 'Recent' },
  { id: 'events', label: 'Events' },
];

export function PipelineRunsPageView({
  activeTab,
  activeGroupId,
  activeGroupPath,
  activeGroupLabel,
  activeRunId,
  searchTerm,
  searchOpen,
  searchInputRef,
  setSearchTerm,
  setSearchOpen,
  updateSearchParams,
  viewMode,
  setViewMode,
  mainContentRef,
  isViewingDetail,
  showSelectionBar,
  trimmedSearch,
  selectedRunIds,
  clearSelection,
  handleBulkDelete,
  handleNewFolder,
  groups,
  groupsLoading,
  groupsError,
  runsByBranch,
  filteredRecentRuns,
  groupedEvents,
  runsLoading,
  runsError,
  repoSummaries,
  fetchRepoSummary,
  onSelectGroup,
  handleDeleteFolder,
  handleOpenRun,
  handleRunSelect,
  collapsedEvents,
  toggleEventGroup,
  collapseAllEvents,
  expandAllEvents,
  collapsedBranches,
  toggleBranchCollapse,
  handleDeleteBranch,
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
  newFolderOpen,
  newFolderError,
  newFolderPending,
  setNewFolderOpen,
  setNewFolderError,
  setNewFolderPending,
  submitNewFolder,
  folderConfig,
}: PipelineRunsPageViewProps) {
  const {
    configRepoFolder,
    configRepo,
    configRepoForm,
    configRepoLoading,
    configRepoSaving,
    configRepoSyncing,
    configRepoError,
    configRepoDriftOpen,
    configRepoDrift,
    configRepoDriftLoading,
    configRepoDriftError,
    configRepoPushing,
    configRepoPushResult,
    configRepoManageAllowed,
    configRepoSyncAllowed,
    notificationRoute,
    notificationRouteForm,
    notificationRouteLoading,
    notificationRouteSaving,
    notificationRouteError,
    setConfigRepoForm,
    setNotificationRouteForm,
    setConfigRepoDriftOpen,
    openFolderConfigRepository,
    closeFolderConfigRepository,
    saveFolderConfigRepository,
    deleteFolderConfigRepository,
    syncFolderConfigRepository,
    checkFolderConfigRepositoryDrift,
    pushFolderConfigRepositoryDrift,
    saveFolderNotificationRoute,
    deleteFolderNotificationRoute,
  } = folderConfig;

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
                    clearSelection();
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
                    onClick={() => void handleBulkDelete()}
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
