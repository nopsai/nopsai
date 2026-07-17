import { useMemo, useRef, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock3,
  History,
  Info,
  LayoutDashboard,
  MoreHorizontal,
  PauseCircle,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Trash2,
  X,
} from 'lucide-react';

import ResourceAccessCard from '../../components/ResourceAccessCard';
import { useOutsideDismiss } from '../../components/useOutsideDismiss';
import { friendlyCronLabel } from '../schedules/model';
import { DashboardBlocks } from './blocks/DashboardBlocks';
import {
  formatDateTime,
  groupPublicationsBySection,
  refreshProgress,
  refreshStatusLabel,
  runScopeLabel,
  staleLabel,
  type DashboardEvent,
  type DashboardPublication,
  type DashboardRefresh,
  type DashboardRefreshSchedule,
  type DashboardSection,
  type DashboardSource,
  type DashboardSummary,
  type DashboardView,
} from './model';

type DashboardWorkspaceProps = {
  dashboards: DashboardSummary[];
  teams: string[];
  selectedID: string;
  selectedDashboard: DashboardSummary | null;
  view: DashboardView | null;
  history: DashboardEvent[];
  refreshes: DashboardRefresh[];
  refreshSchedules: DashboardRefreshSchedule[];
  loading: boolean;
  detailLoading: boolean;
  error: string | null;
  searchTerm: string;
  teamFilter: string;
  saving: boolean;
  canWriteDashboards: boolean;
  canDeleteDashboards: boolean;
  onSearchTermChange: (value: string) => void;
  onTeamFilterChange: (value: string) => void;
  onSelectDashboard: (id: string) => void;
  onReloadDashboards: () => void;
  onCreateDashboard: () => void;
  onEditDashboard: (dashboard: DashboardSummary) => void;
  onDeleteDashboard: (dashboard: DashboardSummary) => void;
  onRefreshDashboard: () => void;
  onScheduleDashboard: () => void;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
  onRefreshSource: (source: DashboardSource) => void;
  onCancelRefresh: (refresh: DashboardRefresh) => void;
  onRetryRefresh: (refresh: DashboardRefresh) => void;
  onCreateSchedule: (scope: RefreshScheduleScope) => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
};

type RefreshScheduleScope = {
  scopeType?: 'dashboard' | 'section' | 'source';
  sectionKey?: string;
  sourceID?: string;
};

export function DashboardWorkspace({
  dashboards,
  teams,
  selectedID,
  selectedDashboard,
  view,
  history,
  refreshes,
  refreshSchedules,
  loading,
  detailLoading,
  error,
  searchTerm,
  teamFilter,
  saving,
  canWriteDashboards,
  canDeleteDashboards,
  onSearchTermChange,
  onTeamFilterChange,
  onSelectDashboard,
  onReloadDashboards,
  onCreateDashboard,
  onEditDashboard,
  onDeleteDashboard,
  onRefreshDashboard,
  onScheduleDashboard,
  onEditSource,
  onDeleteSource,
  onRefreshSource,
  onCancelRefresh,
  onRetryRefresh,
  onCreateSchedule,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: DashboardWorkspaceProps) {
  const [dashboardDetailsOpen, setDashboardDetailsOpen] = useState(false);
  const [openSectionDetails, setOpenSectionDetails] = useState<Set<string>>(new Set());
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(new Set());
  const sections = view?.sections || [];
  const sources = view?.sources || [];
  const publicationsBySection = useMemo(
    () => groupPublicationsBySection(view?.publications || []),
    [view?.publications]
  );
  const latestRefresh = refreshes[0] || null;
  const activeRefresh = refreshes.find(refresh => refresh.status === 'running') || null;
  const latestRefreshSourceByID = useMemo(() => {
    const map = new Map<string, NonNullable<DashboardRefresh['sources']>[number]>();
    for (const source of latestRefresh?.sources || []) {
      if (source.source_binding_id) map.set(source.source_binding_id, source);
    }
    return map;
  }, [latestRefresh]);

  const toggleSectionDetails = (sectionKey: string) => {
    setOpenSectionDetails(current => {
      const next = new Set(current);
      if (next.has(sectionKey)) next.delete(sectionKey);
      else next.add(sectionKey);
      return next;
    });
  };

  const toggleSectionCollapsed = (sectionKey: string) => {
    setCollapsedSections(current => {
      const next = new Set(current);
      if (next.has(sectionKey)) next.delete(sectionKey);
      else next.add(sectionKey);
      return next;
    });
  };

  return (
    <div data-page="dashboards" className="active flex h-full flex-col bg-[var(--bg-primary)]">
      <header className="border-b border-[var(--border-subtle)] bg-[var(--bg-primary)] px-4 py-3">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div className="grid min-w-0 flex-1 gap-2 md:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_minmax(160px,220px)]">
            <label className="min-w-0">
              <span className="sr-only">Dashboard</span>
              <select
                className="h-10 w-full rounded-md border border-transparent bg-[var(--bg-secondary)] px-3 text-sm font-medium text-[var(--text-primary)] shadow-sm outline-none focus:border-[var(--accent)]"
                value={selectedID}
                onChange={event => onSelectDashboard(event.target.value)}
                disabled={loading || dashboards.length === 0}
                aria-label="Dashboard"
              >
                <option value="">{loading ? 'Loading dashboards' : 'Select dashboard'}</option>
                {dashboards.map(dashboard => (
                  <option key={dashboard.id} value={dashboard.id}>
                    {dashboard.title} - {dashboard.ref}
                  </option>
                ))}
              </select>
            </label>
            <label className="min-w-0">
              <span className="sr-only">Search dashboards</span>
              <input
                className="h-10 w-full rounded-md border border-transparent bg-[var(--bg-secondary)] px-3 text-sm text-[var(--text-primary)] shadow-sm outline-none placeholder:text-[var(--text-muted)] focus:border-[var(--accent)]"
                value={searchTerm}
                onChange={event => onSearchTermChange(event.target.value)}
                placeholder="Search dashboards"
                aria-label="Search dashboards"
              />
            </label>
            <label className="min-w-0">
              <span className="sr-only">Filter by team</span>
              <select
                className="h-10 w-full rounded-md border border-transparent bg-[var(--bg-secondary)] px-3 text-sm text-[var(--text-primary)] shadow-sm outline-none focus:border-[var(--accent)]"
                value={teamFilter}
                onChange={event => onTeamFilterChange(event.target.value)}
                aria-label="Filter by team"
              >
                <option value="">All teams</option>
                {teams.map(team => <option key={team} value={team}>{team}</option>)}
              </select>
            </label>
          </div>

          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <ToolbarButton label="Reload dashboards" icon={<RefreshCw className="h-4 w-4" />} onClick={onReloadDashboards} disabled={loading} />
            {canWriteDashboards ? (
              <ToolbarButton label="New dashboard" icon={<Plus className="h-4 w-4" />} onClick={onCreateDashboard} accent />
            ) : null}
          </div>
        </div>
        {error ? <div className="mt-3 rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/30 dark:text-rose-100">{error}</div> : null}
      </header>

      <main className="min-h-0 flex-1 overflow-auto">
        <div className="mx-auto w-full max-w-[1500px] space-y-5 px-4 py-5 lg:px-6">
          {!selectedDashboard ? (
            <EmptyDashboardState loading={loading} dashboardCount={dashboards.length} />
          ) : (
            <>
              <DashboardHeader
                dashboard={selectedDashboard}
                activeRefresh={activeRefresh}
                dashboardDetailsOpen={dashboardDetailsOpen}
                canWriteDashboards={canWriteDashboards}
                canDeleteDashboards={canDeleteDashboards}
                saving={saving}
                onToggleDetails={() => setDashboardDetailsOpen(open => !open)}
                onRefresh={onRefreshDashboard}
                onSchedule={onScheduleDashboard}
                onEdit={() => onEditDashboard(selectedDashboard)}
                onDelete={() => onDeleteDashboard(selectedDashboard)}
              />

              {dashboardDetailsOpen ? (
                <DashboardDetails
                  dashboard={selectedDashboard}
                  latestRefresh={latestRefresh}
                  activeRefresh={activeRefresh}
                  refreshes={refreshes}
                  schedules={refreshSchedules}
                  history={history.slice(0, 8)}
                  saving={saving}
                  canWriteDashboards={canWriteDashboards}
                  onCancelRefresh={onCancelRefresh}
                  onRetryRefresh={onRetryRefresh}
                  onCreateSchedule={() => onCreateSchedule({ scopeType: 'dashboard' })}
                  onEditSchedule={onEditSchedule}
                  onDeleteSchedule={onDeleteSchedule}
                  onToggleSchedule={onToggleSchedule}
                  onRunSchedule={onRunSchedule}
                />
              ) : null}

              {detailLoading ? <div className="rounded-md bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">Loading dashboard...</div> : null}

              {sections.length === 0 ? (
                <div className="rounded-md border border-dashed border-[var(--border-subtle)] px-4 py-8 text-center text-sm text-[var(--text-secondary)]">
                  This dashboard has no sections yet.
                </div>
              ) : null}

              {sections.map(section => {
                const sectionPublications = publicationsBySection[section.section_key] || [];
                const sectionSources = sources.filter(source => source.section_key === section.section_key);
                const enabledSources = sectionSources.filter(source => source.enabled);
                const requiredSources = enabledSources.filter(source => source.required_for_refresh);
                const sectionSchedules = refreshSchedules.filter(schedule => scheduleMatchesSection(schedule, section.section_key, sources));
                const sectionHistory = history.filter(event => event.section_key === section.section_key).slice(0, 6);
                const sectionRefreshes = refreshes.filter(refresh => refreshMatchesSection(refresh, section.section_key)).slice(0, 4);
                const activeSectionSources = (activeRefresh?.sources || [])
                  .filter(source => source.section_key === section.section_key && sourceRefreshRunning(source.status));
                const detailsOpen = openSectionDetails.has(section.section_key);
                const collapsed = collapsedSections.has(section.section_key);
                const completeness = requiredSources.length === 0
                  ? 'No required sources'
                  : `${Math.min(sectionPublications.length, requiredSources.length)}/${requiredSources.length} required`;
                return (
                  <DashboardSectionSurface
                    key={section.id || section.section_key}
                    section={section}
                    publications={sectionPublications}
                    sources={sectionSources}
                    schedules={sectionSchedules}
                    history={sectionHistory}
                    refreshes={sectionRefreshes}
                    activeSources={activeSectionSources}
                    latestRefreshSourceByID={latestRefreshSourceByID}
                    completeness={completeness}
                    detailsOpen={detailsOpen}
                    collapsed={collapsed}
                    activeRefresh={activeRefresh}
                    saving={saving}
                    canWriteDashboards={canWriteDashboards}
                    onToggleDetails={() => toggleSectionDetails(section.section_key)}
                    onToggleCollapsed={() => toggleSectionCollapsed(section.section_key)}
                    onEditSource={onEditSource}
                    onDeleteSource={onDeleteSource}
                    onRefreshSource={onRefreshSource}
                    onEditSchedule={onEditSchedule}
                    onDeleteSchedule={onDeleteSchedule}
                    onToggleSchedule={onToggleSchedule}
                    onRunSchedule={onRunSchedule}
                  />
                );
              })}
            </>
          )}
        </div>
      </main>
    </div>
  );
}

function DashboardHeader({
  dashboard,
  activeRefresh,
  dashboardDetailsOpen,
  canWriteDashboards,
  canDeleteDashboards,
  saving,
  onToggleDetails,
  onRefresh,
  onSchedule,
  onEdit,
  onDelete,
}: {
  dashboard: DashboardSummary;
  activeRefresh: DashboardRefresh | null;
  dashboardDetailsOpen: boolean;
  canWriteDashboards: boolean;
  canDeleteDashboards: boolean;
  saving: boolean;
  onToggleDetails: () => void;
  onRefresh: () => void;
  onSchedule: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="space-y-3">
      <div className="flex flex-col gap-3 rounded-md bg-[var(--bg-secondary)] px-4 py-3 shadow-sm lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-muted)]">
            <LayoutDashboard className="h-4 w-4" aria-hidden="true" />
            <span className="truncate">{dashboard.ref}</span>
            {dashboard.managed_by_config_repo ? <Badge>GitOps</Badge> : null}
            <Badge>{dashboard.visibility || 'team'}</Badge>
          </div>
          <p className="mt-1 truncate text-2xl font-semibold text-[var(--text-primary)]">{dashboard.title}</p>
          {dashboard.description ? <p className="mt-1 max-w-4xl text-sm text-[var(--text-secondary)]">{dashboard.description}</p> : null}
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <ActionMenu label="Dashboard actions" buttonLabel="Actions">
            {({ close }) => (
              <>
                {canWriteDashboards ? (
                  <ActionMenuItem
                    label="Refresh dashboard"
                    icon={<RefreshCw className="h-4 w-4" />}
                    onClick={onRefresh}
                    close={close}
                    disabled={Boolean(activeRefresh)}
                    primary
                  />
                ) : null}
                {canWriteDashboards ? (
                  <ActionMenuItem
                    label="Schedule refresh"
                    icon={<CalendarClock className="h-4 w-4" />}
                    onClick={onSchedule}
                    close={close}
                  />
                ) : null}
                {canWriteDashboards ? (
                  <ActionMenuItem label="Edit dashboard" icon={<Pencil className="h-4 w-4" />} onClick={onEdit} close={close} />
                ) : null}
                <ResourceAccessCard
                  resourceType="dashboard"
                  resourceID={dashboard.ref || dashboard.id}
                  label="dashboard"
                  buttonClassName={actionMenuItemClass()}
                  onDialogClose={close}
                />
                <ActionMenuItem
                  label={dashboardDetailsOpen ? 'Hide dashboard details' : 'Show dashboard details'}
                  icon={<Info className="h-4 w-4" />}
                  onClick={onToggleDetails}
                  close={close}
                  active={dashboardDetailsOpen}
                />
                {canDeleteDashboards ? (
                  <ActionMenuItem
                    label="Delete dashboard"
                    icon={<Trash2 className="h-4 w-4" />}
                    onClick={onDelete}
                    close={close}
                    disabled={saving}
                    danger
                  />
                ) : null}
              </>
            )}
          </ActionMenu>
        </div>
      </div>
    </section>
  );
}

function DashboardSectionSurface({
  section,
  publications,
  sources,
  schedules,
  history,
  refreshes,
  activeSources,
  latestRefreshSourceByID,
  completeness,
  detailsOpen,
  collapsed,
  activeRefresh,
  saving,
  canWriteDashboards,
  onToggleDetails,
  onToggleCollapsed,
  onEditSource,
  onDeleteSource,
  onRefreshSource,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: {
  section: DashboardSection;
  publications: DashboardPublication[];
  sources: DashboardSource[];
  schedules: DashboardRefreshSchedule[];
  history: DashboardEvent[];
  refreshes: DashboardRefresh[];
  activeSources: NonNullable<DashboardRefresh['sources']>;
  latestRefreshSourceByID: Map<string, NonNullable<DashboardRefresh['sources']>[number]>;
  completeness: string;
  detailsOpen: boolean;
  collapsed: boolean;
  activeRefresh: DashboardRefresh | null;
  saving: boolean;
  canWriteDashboards: boolean;
  onToggleDetails: () => void;
  onToggleCollapsed: () => void;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
  onRefreshSource: (source: DashboardSource) => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
}) {
  return (
    <section className="space-y-3">
      <div className="flex flex-col gap-3 rounded-md bg-[var(--bg-secondary)] px-4 py-3 shadow-sm lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-lg font-semibold text-[var(--text-primary)]">{section.title}</h2>
            <Badge>{completeness}</Badge>
            {refreshes[0] ? <StatusBadge status={refreshes[0].status} /> : null}
          </div>
          <div className="mt-1 flex flex-wrap gap-2 text-xs text-[var(--text-muted)]">
            <span>{section.section_key}</span>
            <span>{publications.length} entries</span>
            <span>{sources.length} sources</span>
          </div>
          {section.description ? <p className="mt-1 text-sm text-[var(--text-secondary)]">{section.description}</p> : null}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <ToolbarButton
            label={detailsOpen ? 'Hide details' : 'Show details'}
            icon={<Info className="h-4 w-4" />}
            onClick={onToggleDetails}
            pressed={detailsOpen}
          />
          <ToolbarButton
            label={collapsed ? 'Expand section' : 'Collapse section'}
            icon={collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
            onClick={onToggleCollapsed}
            pressed={collapsed}
          />
        </div>
      </div>

      {activeSources.length > 0 ? (
        <SectionRunningSources sources={activeSources} />
      ) : null}

      {collapsed ? null : publications.length === 0 ? (
        <div className="rounded-md border border-dashed border-[var(--border-subtle)] px-4 py-6 text-sm text-[var(--text-secondary)]">
          No publications yet. Assign a dashboard-output pipeline or run a refresh to populate this section.
        </div>
      ) : (
        <div className="grid gap-3 xl:grid-cols-2">
          {publications.map(publication => (
            <PublicationCard key={publication.id} publication={publication} />
          ))}
        </div>
      )}

      {detailsOpen && !collapsed ? (
        <SectionDetails
          sources={sources}
          schedules={schedules}
          history={history}
          refreshes={refreshes}
          latestRefreshSourceByID={latestRefreshSourceByID}
          activeRefresh={activeRefresh}
          saving={saving}
          canWriteDashboards={canWriteDashboards}
          onEditSource={onEditSource}
          onDeleteSource={onDeleteSource}
          onRefreshSource={onRefreshSource}
          onEditSchedule={onEditSchedule}
          onDeleteSchedule={onDeleteSchedule}
          onToggleSchedule={onToggleSchedule}
          onRunSchedule={onRunSchedule}
        />
      ) : null}
    </section>
  );
}

function PublicationCard({ publication }: { publication: DashboardPublication }) {
  return (
    <article className="min-w-0 rounded-md bg-[var(--bg-secondary)] p-4 shadow-sm">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold text-[var(--text-primary)]">{publication.content.title || publication.entry_key}</div>
          <div className="mt-1 truncate text-xs text-[var(--text-muted)]">
            {publication.pipeline_id} / {publication.output_name} / {runScopeLabel(publication.run_scope)}
          </div>
        </div>
        <span className={`rounded-md px-2 py-1 text-xs ${publication.stale ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/30 dark:text-amber-100' : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100'}`}>
          {staleLabel(publication)}
        </span>
      </div>
      <DashboardBlocks spec={publication.content} />
      <div className="mt-3 flex flex-wrap items-center gap-3 text-xs text-[var(--text-muted)]">
        <span>Revision {publication.revision}</span>
        <span>{formatDateTime(publication.published_at)}</span>
        {publication.run_id ? (
          <Link
            className="font-medium text-[var(--accent)] hover:underline"
            to={runDetailHref(publication.run_id)}
          >
            Run {publication.run_id.slice(0, 8)}
          </Link>
        ) : null}
      </div>
    </article>
  );
}

function SectionRunningSources({ sources }: { sources: NonNullable<DashboardRefresh['sources']> }) {
  return (
    <div className="rounded-md border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-900 shadow-sm dark:border-sky-800/60 dark:bg-sky-950/30 dark:text-sky-100">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="flex min-w-0 items-start gap-2">
          <RefreshCw className="mt-0.5 h-4 w-4 shrink-0 animate-spin" aria-hidden="true" />
          <div className="min-w-0">
            <div className="font-semibold">{sources.length === 1 ? 'Pipeline source running' : `${sources.length} pipeline sources running`}</div>
            <div className="mt-1 text-xs opacity-80">This section is being refreshed. New publications will appear here when source runs finish.</div>
          </div>
        </div>
        <div className="grid min-w-0 gap-2 md:min-w-[360px]">
          {sources.map(source => (
            <div key={source.id} className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-md bg-white/70 px-3 py-2 dark:bg-slate-900/40">
              <div className="min-w-0">
                <div className="truncate text-xs font-semibold">{source.pipeline_id}</div>
                <div className="truncate text-xs opacity-80">{source.output_name} / {runScopeLabel(source.run_scope)}</div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <StatusBadge status={source.status} />
                {source.run_id ? (
                  <Link className="text-xs font-semibold text-[var(--accent)] hover:underline" to={runDetailHref(source.run_id)}>
                    Run {source.run_id.slice(0, 8)}
                  </Link>
                ) : null}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function DashboardDetails({
  dashboard,
  latestRefresh,
  activeRefresh,
  refreshes,
  schedules,
  history,
  saving,
  canWriteDashboards,
  onCancelRefresh,
  onRetryRefresh,
  onCreateSchedule,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: {
  dashboard: DashboardSummary;
  latestRefresh: DashboardRefresh | null;
  activeRefresh: DashboardRefresh | null;
  refreshes: DashboardRefresh[];
  schedules: DashboardRefreshSchedule[];
  history: DashboardEvent[];
  saving: boolean;
  canWriteDashboards: boolean;
  onCancelRefresh: (refresh: DashboardRefresh) => void;
  onRetryRefresh: (refresh: DashboardRefresh) => void;
  onCreateSchedule: () => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
}) {
  return (
    <div className="grid gap-3 xl:grid-cols-[minmax(0,1.1fr)_minmax(320px,0.9fr)]">
      <DetailPanel title="Dashboard details" icon={<Info className="h-4 w-4" />}>
        <div className="grid gap-2 text-sm md:grid-cols-2">
          <DetailValue label="Team" value={dashboard.team_path || '-'} />
          <DetailValue label="Slug" value={dashboard.slug || '-'} />
          <DetailValue label="Visibility" value={dashboard.visibility || 'team'} />
          <DetailValue label="Updated" value={formatDateTime(dashboard.updated_at) || '-'} />
          <DetailValue label="Source" value={dashboard.managed_by_config_repo ? 'GitOps' : 'Database'} />
          <DetailValue label="Config path" value={dashboard.config_source_path || '-'} />
        </div>
      </DetailPanel>
      <DetailPanel title="Refresh status" icon={<RefreshCw className="h-4 w-4" />}>
        {latestRefresh ? (
          <RefreshPanel
            refresh={latestRefresh}
            saving={saving}
            compact
            onCancel={activeRefresh ? () => onCancelRefresh(activeRefresh) : undefined}
            onRetry={canWriteDashboards && (latestRefresh.failed_sources > 0 || latestRefresh.skipped_sources > 0) ? () => onRetryRefresh(latestRefresh) : undefined}
          />
        ) : (
          <EmptyDetail label="No refreshes yet." />
        )}
      </DetailPanel>
      <DetailPanel title="Schedules" icon={<CalendarClock className="h-4 w-4" />}>
        <ScheduleList
          schedules={schedules}
          activeRefresh={activeRefresh}
          saving={saving}
          canWriteDashboards={canWriteDashboards}
          onCreateSchedule={onCreateSchedule}
          onEditSchedule={onEditSchedule}
          onDeleteSchedule={onDeleteSchedule}
          onToggleSchedule={onToggleSchedule}
          onRunSchedule={onRunSchedule}
        />
      </DetailPanel>
      <DetailPanel title="Activity" icon={<History className="h-4 w-4" />}>
        <HistoryList history={history} />
        <RefreshList refreshes={refreshes.slice(0, 5)} />
      </DetailPanel>
    </div>
  );
}

function SectionDetails({
  sources,
  schedules,
  history,
  refreshes,
  latestRefreshSourceByID,
  activeRefresh,
  saving,
  canWriteDashboards,
  onEditSource,
  onDeleteSource,
  onRefreshSource,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: {
  sources: DashboardSource[];
  schedules: DashboardRefreshSchedule[];
  history: DashboardEvent[];
  refreshes: DashboardRefresh[];
  latestRefreshSourceByID: Map<string, NonNullable<DashboardRefresh['sources']>[number]>;
  activeRefresh: DashboardRefresh | null;
  saving: boolean;
  canWriteDashboards: boolean;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
  onRefreshSource: (source: DashboardSource) => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
}) {
  return (
    <div className="grid gap-3 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
      <DetailPanel title="Sources" icon={<Clock3 className="h-4 w-4" />}>
        {sources.length === 0 ? <EmptyDetail label="No sources attached." /> : null}
        <div className="space-y-2">
          {sources.map(source => (
            <SourceRow
              key={source.id}
              source={source}
              refreshSource={latestRefreshSourceByID.get(source.id)}
              activeRefresh={activeRefresh}
              canWriteDashboards={canWriteDashboards}
              onEditSource={onEditSource}
              onDeleteSource={onDeleteSource}
              onRefreshSource={onRefreshSource}
            />
          ))}
        </div>
      </DetailPanel>
      <DetailPanel title="Refreshes" icon={<RefreshCw className="h-4 w-4" />}>
        <RefreshList refreshes={refreshes} />
      </DetailPanel>
      <DetailPanel title="Schedules" icon={<CalendarClock className="h-4 w-4" />}>
        <ScheduleList
          schedules={schedules}
          activeRefresh={activeRefresh}
          saving={saving}
          canWriteDashboards={canWriteDashboards}
          onEditSchedule={onEditSchedule}
          onDeleteSchedule={onDeleteSchedule}
          onToggleSchedule={onToggleSchedule}
          onRunSchedule={onRunSchedule}
        />
      </DetailPanel>
      <DetailPanel title="Latest runs" icon={<AlertTriangle className="h-4 w-4" />}>
        <SectionRunHistoryList refreshes={refreshes} history={history} />
      </DetailPanel>
    </div>
  );
}

function SourceRow({
  source,
  refreshSource,
  activeRefresh,
  canWriteDashboards,
  onEditSource,
  onDeleteSource,
  onRefreshSource,
}: {
  source: DashboardSource;
  refreshSource?: NonNullable<DashboardRefresh['sources']>[number];
  activeRefresh: DashboardRefresh | null;
  canWriteDashboards: boolean;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
  onRefreshSource: (source: DashboardSource) => void;
}) {
  return (
    <div className="flex flex-col gap-2 rounded-md bg-[var(--bg-primary)] px-3 py-2 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-[var(--text-primary)]">{source.pipeline_id}</div>
        <div className="mt-1 truncate text-xs text-[var(--text-muted)]">
          {source.output_name}{source.entry_key ? ` / ${source.entry_key}` : ''} / {runScopeLabel(source.run_scope)} / order {source.refresh_order}
        </div>
        <div className="mt-2 flex flex-wrap gap-1 text-xs">
          <Badge>{source.enabled ? 'Enabled' : 'Disabled'}</Badge>
          <Badge>{source.required_for_refresh ? 'Required' : 'Optional'}</Badge>
          {refreshSource ? <StatusBadge status={refreshSource.status} /> : null}
        </div>
        {refreshSource?.error ? <div className="mt-1 text-xs text-rose-600">{refreshSource.error}</div> : null}
      </div>
      {canWriteDashboards ? (
        <div className="flex shrink-0 items-center gap-2">
          <ToolbarButton label="Refresh source" icon={<RefreshCw className="h-4 w-4" />} onClick={() => onRefreshSource(source)} disabled={Boolean(activeRefresh) || !source.enabled} />
          <ToolbarButton label="Edit source" icon={<Pencil className="h-4 w-4" />} onClick={() => onEditSource(source)} />
          <ToolbarButton label="Delete source" icon={<Trash2 className="h-4 w-4" />} onClick={() => onDeleteSource(source)} danger />
        </div>
      ) : null}
    </div>
  );
}

function ScheduleList({
  schedules,
  activeRefresh,
  saving,
  canWriteDashboards,
  onCreateSchedule,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: {
  schedules: DashboardRefreshSchedule[];
  activeRefresh: DashboardRefresh | null;
  saving: boolean;
  canWriteDashboards: boolean;
  onCreateSchedule?: () => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
}) {
  return (
    <div className="space-y-2">
      {canWriteDashboards && onCreateSchedule ? (
        <button
          type="button"
          className="inline-flex min-h-9 items-center gap-2 rounded-md bg-[var(--accent)] px-3 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          onClick={onCreateSchedule}
          disabled={saving}
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          New schedule
        </button>
      ) : null}
      {schedules.length === 0 ? <EmptyDetail label="No schedules." /> : null}
      {schedules.map(schedule => (
        <div key={schedule.id} className="flex flex-col gap-3 rounded-md bg-[var(--bg-primary)] px-3 py-3 md:flex-row md:items-center md:justify-between">
          <div className="min-w-0">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <div className="truncate text-sm font-semibold text-[var(--text-primary)]">{schedule.name}</div>
              <span className={`runner-pill ${schedule.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                {schedule.enabled ? 'Enabled' : 'Disabled'}
              </span>
              {schedule.managed_by_config_repo ? <span className="runner-pill runner-pill--link">GitOps</span> : null}
              <span className={`runner-pill ${scheduleStatusTone(schedule.last_status)}`}>{scheduleStatusLabel(schedule.last_status)}</span>
            </div>
            {schedule.description ? <div className="mt-1 text-sm text-[var(--text-secondary)]">{schedule.description}</div> : null}
            <div className="mt-2 grid gap-2 text-xs text-[var(--text-secondary)] sm:grid-cols-4">
              <ScheduleFact label="Schedule" value={friendlyCronLabel(schedule.cron_expression || schedule.cron)} />
              <ScheduleFact label="Next" value={schedule.enabled ? formatDateTime(schedule.next_run_at) || 'pending' : 'Disabled'} />
              <ScheduleFact label="Target" value={`${scheduleScopeLabel(schedule)} / ${schedule.mode}`} />
              <ScheduleFact label="Run scope" value={schedule.run_scope ? runScopeLabel(schedule.run_scope) : 'Source scopes'} />
            </div>
            <div className="mt-2 truncate font-mono text-xs text-[var(--text-muted)]" title={`${schedule.cron_expression || schedule.cron} / ${schedule.timezone}`}>
              {schedule.cron_expression || schedule.cron} / {schedule.timezone}
            </div>
          </div>
          {canWriteDashboards ? (
            <div className="flex shrink-0 items-center gap-2">
              <ToolbarButton label={`Run ${schedule.name}`} icon={<Play className="h-4 w-4" />} onClick={() => onRunSchedule(schedule)} disabled={saving || Boolean(activeRefresh)} />
              <ToolbarButton label={`Edit ${schedule.name}`} icon={<Pencil className="h-4 w-4" />} onClick={() => onEditSchedule(schedule)} disabled={saving} />
              <ToolbarButton
                label={`${schedule.enabled ? 'Disable' : 'Enable'} ${schedule.name}`}
                icon={schedule.enabled ? <PauseCircle className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
                onClick={() => onToggleSchedule(schedule, !schedule.enabled)}
                disabled={saving}
                pressed={schedule.enabled}
              />
              <ToolbarButton label={`Delete ${schedule.name}`} icon={<Trash2 className="h-4 w-4" />} onClick={() => onDeleteSchedule(schedule)} disabled={saving} danger />
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function ScheduleFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md bg-[var(--bg-secondary)] px-2 py-1.5">
      <div className="text-[10px] font-semibold uppercase text-[var(--text-muted)]">{label}</div>
      <div className="mt-0.5 truncate text-xs font-medium text-[var(--text-primary)]" title={value}>{value}</div>
    </div>
  );
}

type SectionRunHistoryItem = {
  id: string;
  type: 'run' | 'publication';
  status: string;
  title: string;
  subtitle: string;
  timestamp: string;
  runID?: string;
  refreshID?: string;
  error?: string;
};

function SectionRunHistoryList({ refreshes, history }: { refreshes: DashboardRefresh[]; history: DashboardEvent[] }) {
  const items = sectionRunHistoryItems(refreshes, history).slice(0, 8);
  if (items.length === 0) return <EmptyDetail label="No run history yet." />;
  return (
    <div className="space-y-2">
      {items.map(item => (
        <div key={item.id} className="rounded-md bg-[var(--bg-primary)] px-3 py-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex min-w-0 items-center gap-2">
              <StatusBadge status={item.status} />
              <span className="truncate text-sm font-medium text-[var(--text-primary)]">{item.title}</span>
            </div>
            <span className="text-xs text-[var(--text-muted)]">{formatDateTime(item.timestamp)}</span>
          </div>
          <div className="mt-1 truncate text-xs text-[var(--text-muted)]" title={item.subtitle}>{item.subtitle}</div>
          {item.error ? <div className="mt-1 text-xs text-rose-600">{item.error}</div> : null}
          <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--text-secondary)]">
            <span>{item.type === 'run' ? 'Refresh source' : 'Publication event'}</span>
            {item.refreshID ? <span>Refresh {item.refreshID.slice(0, 8)}</span> : null}
            {item.runID ? (
              <Link className="font-medium text-[var(--accent)] hover:underline" to={runDetailHref(item.runID)}>
                Run {item.runID.slice(0, 8)}
              </Link>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  );
}

function HistoryList({ history }: { history: DashboardEvent[] }) {
  if (history.length === 0) return <EmptyDetail label="No history." />;
  return (
    <div className="space-y-2">
      {history.map(event => (
        <div key={event.id} className="rounded-md bg-[var(--bg-primary)] px-3 py-2">
          <div className="text-sm font-medium text-[var(--text-primary)]">{event.event_type}</div>
          <div className="mt-1 text-xs text-[var(--text-muted)]">{event.section_key} / {event.entry_key} / revision {event.revision}</div>
          <div className="mt-1 text-xs text-[var(--text-secondary)]">{formatDateTime(event.created_at)}</div>
        </div>
      ))}
    </div>
  );
}

function RefreshList({ refreshes }: { refreshes: DashboardRefresh[] }) {
  if (refreshes.length === 0) return <EmptyDetail label="No refreshes." />;
  return (
    <div className="mt-3 space-y-2">
      {refreshes.map(refresh => (
        <div key={refresh.id} className="rounded-md bg-[var(--bg-primary)] px-3 py-2">
          <div className="flex items-center justify-between gap-2">
            <StatusBadge status={refresh.status} />
            <span className="text-xs text-[var(--text-muted)]">{refresh.scope_type} / {refresh.mode}</span>
          </div>
          <div className="mt-1 text-xs text-[var(--text-secondary)]">{refresh.successful_sources}/{refresh.total_sources} sources / {formatDateTime(refresh.created_at)}</div>
        </div>
      ))}
    </div>
  );
}

function RefreshPanel({
  refresh,
  saving,
  compact = false,
  onCancel,
  onRetry,
}: {
  refresh: DashboardRefresh;
  saving: boolean;
  compact?: boolean;
  onCancel?: () => void;
  onRetry?: () => void;
}) {
  const progress = refreshProgress(refresh);
  return (
    <div>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <StatusBadge status={refresh.status} />
          <div className="mt-1 text-xs text-[var(--text-muted)]">{refresh.scope_type} / {refresh.mode} / {formatDateTime(refresh.created_at)}</div>
        </div>
        <div className="flex flex-wrap gap-2">
          {onRetry ? <ToolbarButton label="Retry failed sources" icon={<RotateCcw className="h-4 w-4" />} onClick={onRetry} disabled={saving} /> : null}
          {onCancel ? <ToolbarButton label="Cancel refresh" icon={<X className="h-4 w-4" />} onClick={onCancel} disabled={saving} danger /> : null}
        </div>
      </div>
      <div className="mt-3 h-2 overflow-hidden rounded-md bg-[var(--bg-primary)]">
        <div className="h-full rounded-md bg-[var(--accent)]" style={{ width: `${progress}%` }} />
      </div>
      <div className="mt-2 flex flex-wrap gap-3 text-xs text-[var(--text-secondary)]">
        <span>{refresh.successful_sources} success</span>
        <span>{refresh.failed_sources} failed</span>
        <span>{refresh.skipped_sources} skipped</span>
        <span>{refresh.running_sources + refresh.queued_sources} active</span>
      </div>
      {!compact && (refresh.sources || []).length > 0 ? (
        <div className="mt-3 grid gap-2 md:grid-cols-2">
          {(refresh.sources || []).map(source => (
            <div key={source.id} className="rounded-md bg-[var(--bg-primary)] px-3 py-2">
              <div className="truncate text-xs font-medium text-[var(--text-primary)]">{source.pipeline_id}</div>
              <div className="mt-1 flex flex-wrap gap-2 text-xs text-[var(--text-muted)]">
                <span>{source.section_key} / {source.output_name} / {runScopeLabel(source.run_scope)}</span>
                <span>{refreshStatusLabel(source.status)}</span>
                <span>{source.required ? 'required' : 'optional'}</span>
              </div>
              {source.error ? <div className="mt-1 text-xs text-rose-600">{source.error}</div> : null}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function DetailPanel({ title, icon, children }: { title: string; icon: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-md bg-[var(--bg-secondary)] p-3 shadow-sm">
      <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]">
        {icon}
        {title}
      </div>
      {children}
    </section>
  );
}

function DetailValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md bg-[var(--bg-primary)] px-3 py-2">
      <div className="text-[11px] uppercase text-[var(--text-muted)]">{label}</div>
      <div className="mt-1 truncate text-sm font-medium text-[var(--text-primary)]" title={value}>{value}</div>
    </div>
  );
}

function EmptyDetail({ label }: { label: string }) {
  return <div className="rounded-md bg-[var(--bg-primary)] px-3 py-3 text-sm text-[var(--text-secondary)]">{label}</div>;
}

function EmptyDashboardState({ loading, dashboardCount }: { loading: boolean; dashboardCount: number }) {
  const label = loading ? 'Loading dashboards...' : dashboardCount === 0 ? 'No dashboards found.' : 'Select a dashboard.';
  return (
    <div className="rounded-md bg-[var(--bg-secondary)] px-4 py-10 text-center text-sm text-[var(--text-secondary)]">
      {label}
    </div>
  );
}

function Badge({ children }: { children: ReactNode }) {
  return <span className="rounded-md bg-[var(--bg-primary)] px-2 py-1 text-xs text-[var(--text-secondary)]">{children}</span>;
}

function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase();
  const tone = normalized === 'complete' || normalized === 'success' || normalized === 'published'
    ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100'
    : normalized === 'running'
      ? 'bg-sky-100 text-sky-800 dark:bg-sky-950/30 dark:text-sky-100'
      : normalized === 'failed' || normalized === 'timed_out'
        ? 'bg-rose-100 text-rose-800 dark:bg-rose-950/30 dark:text-rose-100'
        : 'bg-[var(--bg-primary)] text-[var(--text-secondary)]';
  return <span className={`rounded-md px-2 py-1 text-xs ${tone}`}>{refreshStatusLabel(status)}</span>;
}

function ActionMenu({
  label,
  buttonLabel,
  children,
}: {
  label: string;
  buttonLabel: string;
  children: (controls: { close: () => void }) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const close = () => setOpen(false);
  useOutsideDismiss(menuRef, open, close, { ignore: ['#resource-access-modal'] });

  return (
    <div className="relative" ref={menuRef}>
      <button
        type="button"
        className="inline-flex h-9 items-center gap-2 rounded-md bg-[var(--bg-secondary)] px-3 text-sm font-medium text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] disabled:cursor-not-allowed disabled:opacity-50"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(current => !current)}
      >
        <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        {buttonLabel}
      </button>
      {open ? (
        <div
          role="menu"
          aria-label={label}
          className="absolute right-0 z-40 mt-2 grid min-w-56 gap-1 rounded-md border border-[var(--border-subtle)] bg-[var(--bg-secondary)] p-1.5 shadow-xl"
        >
          {children({ close })}
        </div>
      ) : null}
    </div>
  );
}

function ActionMenuItem({
  label,
  icon,
  onClick,
  close,
  disabled,
  active,
  primary,
  danger,
}: {
  label: string;
  icon: ReactNode;
  onClick: () => void;
  close: () => void;
  disabled?: boolean;
  active?: boolean;
  primary?: boolean;
  danger?: boolean;
}) {
  const handleClick = () => {
    if (disabled) return;
    close();
    onClick();
  };
  return (
    <button
      type="button"
      role="menuitem"
      className={actionMenuItemClass({ active, primary, danger })}
      onClick={handleClick}
      disabled={disabled}
    >
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}

function ToolbarButton({
  label,
  icon,
  onClick,
  disabled,
  accent,
  danger,
  pressed,
}: {
  label: string;
  icon: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  accent?: boolean;
  danger?: boolean;
  pressed?: boolean;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={pressed}
      className={iconButtonClass({ accent, danger, pressed })}
      onClick={onClick}
      disabled={disabled}
    >
      {icon}
    </button>
  );
}

function actionMenuItemClass(options: { active?: boolean; primary?: boolean; danger?: boolean } = {}) {
  const base = 'flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50';
  if (options.danger) return `${base} text-rose-600 hover:bg-rose-50 dark:text-rose-100 dark:hover:bg-rose-950/30`;
  if (options.primary) return `${base} text-[var(--accent)] hover:bg-[var(--bg-active)]`;
  if (options.active) return `${base} bg-[var(--bg-active)] text-[var(--text-primary)]`;
  return `${base} text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]`;
}

function iconButtonClass(options: { accent?: boolean; danger?: boolean; pressed?: boolean } = {}) {
  const base = 'inline-flex h-9 w-9 items-center justify-center rounded-md text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50';
  if (options.danger) return `${base} bg-rose-50 text-rose-600 hover:bg-rose-100 dark:bg-rose-950/30 dark:text-rose-100`;
  if (options.accent) return `${base} bg-[var(--accent)] text-white hover:opacity-90`;
  if (options.pressed) return `${base} bg-[var(--bg-active)] text-[var(--text-primary)]`;
  return `${base} bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]`;
}

function scheduleMatchesSection(schedule: DashboardRefreshSchedule, sectionKey: string, sources: DashboardSource[]): boolean {
  if (schedule.scope_type === 'section') return scopeSectionKeys(schedule.scope).includes(sectionKey);
  if (schedule.scope_type !== 'source') return false;
  const sourceID = recordString(schedule.scope, 'source_id');
  return sources.some(source => source.id === sourceID && source.section_key === sectionKey);
}

function refreshMatchesSection(refresh: DashboardRefresh, sectionKey: string): boolean {
  if (refresh.scope_type === 'section') return scopeSectionKeys(refresh.scope).includes(sectionKey);
  return (refresh.sources || []).some(source => source.section_key === sectionKey);
}

function sectionRunHistoryItems(refreshes: DashboardRefresh[], history: DashboardEvent[]): SectionRunHistoryItem[] {
  const items: SectionRunHistoryItem[] = [];
  for (const refresh of refreshes) {
    for (const source of refresh.sources || []) {
      items.push({
        id: `run-${refresh.id}-${source.id}`,
        type: 'run',
        status: source.status || refresh.status,
        title: source.pipeline_id,
        subtitle: `${source.output_name}${source.entry_key ? ` / ${source.entry_key}` : ''} / ${runScopeLabel(source.run_scope)} / ${source.required ? 'required' : 'optional'} / ${refresh.trigger_type}`,
        timestamp: source.finished_at || source.started_at || source.updated_at || source.created_at || refresh.updated_at || refresh.created_at,
        runID: source.run_id,
        refreshID: refresh.id,
        error: source.error || refresh.error,
      });
    }
    if ((refresh.sources || []).length === 0) {
      items.push({
        id: `refresh-${refresh.id}`,
        type: 'run',
        status: refresh.status,
        title: `${refresh.scope_type} refresh`,
        subtitle: `${refresh.successful_sources}/${refresh.total_sources} sources / ${refresh.mode}`,
        timestamp: refresh.finished_at || refresh.started_at || refresh.updated_at || refresh.created_at,
        refreshID: refresh.id,
        error: refresh.error,
      });
    }
  }
  for (const event of history) {
    items.push({
      id: `event-${event.id}`,
      type: 'publication',
      status: event.event_type,
      title: event.event_type,
      subtitle: `${event.section_key} / ${event.entry_key} / revision ${event.revision}`,
      timestamp: event.created_at,
      runID: event.run_id,
      refreshID: event.refresh_id,
    });
  }
  return items.sort((left, right) => new Date(right.timestamp).getTime() - new Date(left.timestamp).getTime());
}

function scheduleScopeLabel(schedule: DashboardRefreshSchedule): string {
  if (schedule.scope_type === 'section') {
    const sections = scopeSectionKeys(schedule.scope);
    return sections.length > 0 ? `section ${sections.join(', ')}` : 'section';
  }
  if (schedule.scope_type === 'source') {
    const sourceIDs = scopeSourceIDs(schedule.scope);
    return sourceIDs.length > 0 ? `source ${sourceIDs.join(', ')}` : 'source';
  }
  return 'dashboard';
}

function scheduleStatusLabel(status?: string): string {
  const normalized = (status || '').trim();
  if (!normalized) return 'No runs';
  return normalized
    .split(/[\s_-]+/)
    .filter(Boolean)
    .map(part => `${part.slice(0, 1).toUpperCase()}${part.slice(1)}`)
    .join(' ');
}

function scheduleStatusTone(status?: string): string {
  const normalized = (status || '').toLowerCase();
  if (normalized.includes('success') || normalized.includes('complete')) return 'runner-pill--ok';
  if (normalized.includes('fail') || normalized.includes('cancel') || normalized.includes('timeout')) return 'runner-pill--error';
  if (normalized.includes('running') || normalized.includes('pending')) return 'runner-pill--warning';
  return 'runner-pill--muted';
}

function sourceRefreshRunning(status: string): boolean {
  const normalized = status.toLowerCase();
  return normalized === 'queued' || normalized === 'running';
}

function runDetailHref(runID: string): string {
  return `/pipelineruns/recent/${encodeURIComponent(runID)}`;
}

function scopeSectionKeys(scope: Record<string, unknown> | undefined): string[] {
  if (!scope) return [];
  const direct = typeof scope.section_key === 'string' ? [scope.section_key] : [];
  const many = Array.isArray(scope.section_keys) ? scope.section_keys.filter((value): value is string => typeof value === 'string') : [];
  return [...direct, ...many].map(value => value.trim()).filter(Boolean);
}

function scopeSourceIDs(scope: Record<string, unknown> | undefined): string[] {
  if (!scope) return [];
  const direct = typeof scope.source_id === 'string' ? [scope.source_id] : [];
  const many = Array.isArray(scope.source_ids) ? scope.source_ids.filter((value): value is string => typeof value === 'string') : [];
  return [...direct, ...many].map(value => value.trim()).filter(Boolean);
}

function recordString(record: Record<string, unknown> | undefined, key: string): string {
  const value = record?.[key];
  return typeof value === 'string' ? value : '';
}
