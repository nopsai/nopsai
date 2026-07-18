import { useMemo, useRef, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  CalendarClock,
  CheckCircle2,
  Clock3,
  Info,
  MoreHorizontal,
  PauseCircle,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  X,
} from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import { useOutsideDismiss } from '../../components/useOutsideDismiss';
import { WorkflowDialogFrame } from '../../components/WorkflowPrimitives';
import { friendlyCronLabel } from '../schedules/model';
import { DashboardBlocks, dashboardSpecNeedsWideLayout } from './blocks/DashboardBlocks';
import { dashboardAttentionSignals, type DashboardAttentionSignal, type DashboardAttentionTone } from './dashboardAttention';
import { DASHBOARD_CARD_SIZE_OPTIONS, dashboardCardLayoutItemKey, type DashboardCardSize } from './dashboardCardLayout';
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
import { useDashboardCardLayout } from './useDashboardCardLayout';

type DashboardWorkspaceProps = {
  dashboards: DashboardSummary[];
  teams: string[];
  selectedID: string;
  activeSectionKey: string;
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
  onSelectSection: (sectionKey: string) => void;
  sectionTabHref: (sectionKey: string) => string;
  onReloadDashboards: () => void;
  onCreateDashboard: () => void;
  onEditDashboard: (dashboard: DashboardSummary) => void;
  onDeleteDashboard: (dashboard: DashboardSummary) => void;
  onRefreshDashboard: () => void;
  onScheduleDashboard: () => void;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
  onDeletePublication: (publication: DashboardPublication) => void;
  onCancelRefresh: (refresh: DashboardRefresh) => void;
  onRetryRefresh: (refresh: DashboardRefresh) => void;
  onCreateSchedule: (scope: RefreshScheduleScope) => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
};

type RefreshScheduleScope = {
  scopeType?: 'dashboard' | 'section';
  sectionKey?: string;
};

type DashboardRefreshSource = NonNullable<DashboardRefresh['sources']>[number];
type DashboardDetailTabID = 'sources' | 'schedules' | 'refreshes' | 'latest-runs';

const EMPTY_DASHBOARD_SECTIONS: DashboardSection[] = [];
const EMPTY_DASHBOARD_SOURCES: DashboardSource[] = [];
const EMPTY_DASHBOARD_PUBLICATIONS: DashboardPublication[] = [];

export function DashboardWorkspace({
  dashboards,
  teams,
  selectedID,
  activeSectionKey,
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
  onSelectSection,
  sectionTabHref,
  onReloadDashboards,
  onCreateDashboard,
  onEditDashboard,
  onDeleteDashboard,
  onRefreshDashboard,
  onScheduleDashboard,
  onEditSource,
  onDeleteSource,
  onDeletePublication,
  onCancelRefresh,
  onRetryRefresh,
  onCreateSchedule,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: DashboardWorkspaceProps) {
  const [dashboardDetailsOpen, setDashboardDetailsOpen] = useState(false);
  const sections = view?.sections || EMPTY_DASHBOARD_SECTIONS;
  const sources = view?.sources || EMPTY_DASHBOARD_SOURCES;
  const publications = view?.publications || EMPTY_DASHBOARD_PUBLICATIONS;
  const publicationsBySection = useMemo(
    () => groupPublicationsBySection(publications),
    [publications]
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
  const activeSection = sections.find(section => section.section_key === activeSectionKey) || sections[0] || null;
  const resolvedActiveSectionKey = activeSection?.section_key || '';
  const selectedSectionPublications = activeSection ? publicationsBySection[activeSection.section_key] || [] : [];
  const selectedSectionRunningSources = activeSection
    ? (activeRefresh?.sources || []).filter(source => source.section_key === activeSection.section_key && sourceRefreshRunning(source.status))
    : [];
  const attentionSignals = useMemo(
    () => dashboardAttentionSignals({ sections, sources, publications, latestRefresh, refreshSchedules }),
    [latestRefresh, publications, refreshSchedules, sections, sources]
  );

  return (
    <div data-page="dashboards" className="active flex h-full flex-col bg-[var(--bg-tertiary)]">
      <header className="border-b border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-3 shadow-sm">
        <div className="mx-auto flex w-full max-w-[1320px] flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div className="grid min-w-0 flex-1 gap-2 md:grid-cols-[minmax(220px,1fr)_minmax(220px,1.25fr)_minmax(160px,220px)]">
            <label className="min-w-0">
              <span className="sr-only">Dashboard</span>
              <select
                className="h-10 w-full rounded-md border border-[var(--border-strong)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-primary)] shadow-sm outline-none focus:border-[var(--accent)]"
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
            <label className="relative min-w-0">
              <span className="sr-only">Search dashboards</span>
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-tertiary)]" aria-hidden="true" />
              <input
                className="h-10 w-full rounded-md border border-[var(--border-strong)] bg-[var(--bg-secondary)] px-9 text-sm text-[var(--text-primary)] shadow-sm outline-none placeholder:text-[var(--text-tertiary)] focus:border-[var(--accent)]"
                value={searchTerm}
                onChange={event => onSearchTermChange(event.target.value)}
                placeholder="Search dashboards"
                aria-label="Search dashboards"
              />
            </label>
            <label className="min-w-0">
              <span className="sr-only">Filter by team</span>
              <select
                className="h-10 w-full rounded-md border border-[var(--border-strong)] bg-[var(--bg-secondary)] px-3 text-sm text-[var(--text-primary)] shadow-sm outline-none focus:border-[var(--accent)]"
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
        {error ? <div className="mx-auto mt-3 w-full max-w-[1320px] rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/30 dark:text-rose-100">{error}</div> : null}
      </header>

      <main className="min-h-0 flex-1 overflow-auto">
        <div className="mx-auto w-full max-w-[1320px] space-y-6 px-4 py-6 lg:px-8">
          {!selectedDashboard ? (
            <EmptyDashboardState loading={loading} dashboardCount={dashboards.length} />
          ) : (
            <>
              <DashboardHeader
                dashboard={selectedDashboard}
                activeRefresh={activeRefresh}
                attentionSignals={attentionSignals}
                dashboardDetailsOpen={dashboardDetailsOpen}
                canWriteDashboards={canWriteDashboards}
                canDeleteDashboards={canDeleteDashboards}
                saving={saving}
                onOpenDetails={() => setDashboardDetailsOpen(true)}
                onRefresh={onRefreshDashboard}
                onSchedule={onScheduleDashboard}
                onEdit={() => onEditDashboard(selectedDashboard)}
                onDelete={() => onDeleteDashboard(selectedDashboard)}
              />

              {detailLoading ? <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)] shadow-sm">Loading dashboard...</div> : null}

              <DashboardSectionTabs
                sections={sections}
                activeSectionKey={resolvedActiveSectionKey}
                onSelectSection={onSelectSection}
                sectionTabHref={sectionTabHref}
              />

              {sections.length === 0 ? (
                <div className="rounded-md border border-dashed border-[var(--border-subtle)] bg-[var(--bg-secondary)] px-4 py-8 text-center text-sm text-[var(--text-secondary)]">
                  This dashboard has no sections yet.
                </div>
              ) : null}

              <div id="dashboard-sections" className="space-y-6">
                {activeSection ? (
                  <DashboardSectionSurface
                    key={activeSection.id || activeSection.section_key}
                    dashboardID={selectedDashboard.id}
                    section={activeSection}
                    publications={selectedSectionPublications}
                    activeSources={selectedSectionRunningSources}
                    activeRefresh={activeRefresh}
                    saving={saving}
                    canWriteDashboards={canWriteDashboards}
                    onDeletePublication={onDeletePublication}
                    onCancelRefresh={onCancelRefresh}
                  />
                ) : null}
              </div>

              {dashboardDetailsOpen ? (
                <DashboardDetailsModal onClose={() => setDashboardDetailsOpen(false)}>
                  <DashboardDetails
                    titleID="dashboard-details-modal-title"
                    dashboard={selectedDashboard}
                    sources={sources}
                    latestRefresh={latestRefresh}
                    activeRefresh={activeRefresh}
                    refreshes={refreshes}
                    schedules={refreshSchedules}
                    history={history}
                    latestRefreshSourceByID={latestRefreshSourceByID}
                    saving={saving}
                    canWriteDashboards={canWriteDashboards}
                    onEditSource={onEditSource}
                    onDeleteSource={onDeleteSource}
                    onCancelRefresh={onCancelRefresh}
                    onRetryRefresh={onRetryRefresh}
                    onCreateSchedule={() => onCreateSchedule({ scopeType: 'dashboard' })}
                    onEditSchedule={onEditSchedule}
                    onDeleteSchedule={onDeleteSchedule}
                    onToggleSchedule={onToggleSchedule}
                    onRunSchedule={onRunSchedule}
                  />
                </DashboardDetailsModal>
              ) : null}
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
  attentionSignals,
  dashboardDetailsOpen,
  canWriteDashboards,
  canDeleteDashboards,
  saving,
  onOpenDetails,
  onRefresh,
  onSchedule,
  onEdit,
  onDelete,
}: {
  dashboard: DashboardSummary;
  activeRefresh: DashboardRefresh | null;
  attentionSignals: DashboardAttentionSignal[];
  dashboardDetailsOpen: boolean;
  canWriteDashboards: boolean;
  canDeleteDashboards: boolean;
  saving: boolean;
  onOpenDetails: () => void;
  onRefresh: () => void;
  onSchedule: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]">
          <ObjectIcon type="dashboard" className="h-4 w-4" />
          <span className="truncate">{dashboard.ref}</span>
          {dashboard.managed_by_config_repo ? <Badge>GitOps</Badge> : null}
          <Badge>{dashboard.visibility || 'team'}</Badge>
        </div>
        <h2 className="mt-1 flex min-w-0 items-center gap-2 text-2xl font-bold tracking-normal text-[var(--text-primary)]">
          <DashboardAttentionIndicator signals={attentionSignals} />
          <span className="truncate">{dashboard.title}</span>
        </h2>
        <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-[var(--text-secondary)]">
          <span>Team {dashboard.team_path || 'unassigned'}</span>
          {dashboard.last_published_at ? <span>Published {formatDateTime(dashboard.last_published_at)}</span> : null}
          {activeRefresh ? <StatusBadge status={activeRefresh.status} /> : null}
        </div>
        {dashboard.description ? <p className="mt-2 max-w-4xl text-sm leading-6 text-[var(--text-secondary)]">{dashboard.description}</p> : null}
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
                label="Dashboard details"
                icon={<Info className="h-4 w-4" />}
                onClick={onOpenDetails}
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
    </section>
  );
}

function DashboardAttentionIndicator({ signals }: { signals: DashboardAttentionSignal[] }) {
  if (signals.length === 0) return null;
  const primarySignal = signals[0];
  const remainingCount = signals.length - 1;
  const tooltipText = [
    `Needs attention: ${primarySignal.title}${remainingCount > 0 ? ` and ${remainingCount} more` : ''}`,
    primarySignal.detail,
    `What to do: ${primarySignal.action}`,
  ].join('\n');
  return (
    <span
      className={`group relative inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md shadow-sm outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] ${attentionIndicatorClass(primarySignal.tone)}`}
      role="img"
      aria-label={tooltipText}
      title={tooltipText}
      tabIndex={0}
    >
      <AlertTriangle className="relative h-4 w-4" aria-hidden="true" />
      <span className="pointer-events-none absolute left-0 top-full z-50 mt-2 w-[min(22rem,calc(100vw-2rem))] rounded-md border border-[var(--border-strong)] bg-[var(--bg-secondary)] p-3 text-left text-xs font-normal leading-5 text-[var(--text-secondary)] opacity-0 shadow-xl transition-opacity group-hover:opacity-100 group-focus:opacity-100" aria-hidden="true">
        <span className="block font-bold text-[var(--text-primary)]">
          Needs attention: {primarySignal.title}
          {remainingCount > 0 ? ` and ${remainingCount} more` : ''}
        </span>
        <span className="mt-1 block">{primarySignal.detail}</span>
        <span className="mt-2 block font-semibold text-[var(--text-primary)]">What to do</span>
        <span className="mt-1 block">{primarySignal.action}</span>
      </span>
    </span>
  );
}

function attentionIndicatorClass(tone: DashboardAttentionTone): string {
  if (tone === 'danger') {
    return 'border border-rose-300 bg-rose-50 text-rose-700 dark:border-rose-900/70 dark:bg-rose-950/40 dark:text-rose-100';
  }
  return 'border border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-900/70 dark:bg-amber-950/40 dark:text-amber-100';
}

function DashboardDetailsModal({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  return (
    <WorkflowDialogFrame
      id="dashboard-details-modal"
      titleId="dashboard-details-modal-title"
      onClose={onClose}
      overlayClassName="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] px-4 py-6 show"
      className="relative max-h-[92vh] w-full max-w-6xl overflow-y-auto rounded-md outline-none"
    >
      <button
        type="button"
        className="absolute right-3 top-3 z-10 inline-flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-secondary)] shadow-sm transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
        aria-label="Close dashboard details"
        title="Close dashboard details"
        onClick={onClose}
        data-dialog-initial-focus
      >
        <X className="h-4 w-4" aria-hidden="true" />
      </button>
      {children}
    </WorkflowDialogFrame>
  );
}

function DashboardSectionTabs({
  sections,
  activeSectionKey,
  onSelectSection,
  sectionTabHref,
}: {
  sections: DashboardSection[];
  activeSectionKey: string;
  onSelectSection: (sectionKey: string) => void;
  sectionTabHref: (sectionKey: string) => string;
}) {
  if (sections.length === 0) return null;

  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2 py-2 shadow-sm">
      <div className="flex gap-1 overflow-x-auto" role="tablist" aria-label="Dashboard sections">
        {sections.map(section => {
          const selected = section.section_key === activeSectionKey;
          return (
            <Link
              key={section.id || section.section_key}
              id={sectionTabID(section.section_key)}
              to={sectionTabHref(section.section_key)}
              role="tab"
              aria-selected={selected}
              aria-controls={selected ? sectionAnchorID(section.section_key) : undefined}
              aria-label={section.title}
              className={`inline-flex min-h-10 max-w-[280px] shrink-0 items-center gap-2 rounded-md border px-3 text-sm font-semibold transition-colors ${
                selected
                  ? 'border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--text-primary)] shadow-sm'
                  : 'border-transparent text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
              }`}
              onClick={event => {
                if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) return;
                onSelectSection(section.section_key);
              }}
            >
              <span className="truncate">{section.title}</span>
            </Link>
          );
        })}
      </div>
    </div>
  );
}

function DashboardSectionSurface({
  dashboardID,
  section,
  publications,
  activeSources,
  activeRefresh,
  saving,
  canWriteDashboards,
  onDeletePublication,
  onCancelRefresh,
}: {
  dashboardID: string;
  section: DashboardSection;
  publications: DashboardPublication[];
  activeSources: NonNullable<DashboardRefresh['sources']>;
  activeRefresh: DashboardRefresh | null;
  saving: boolean;
  canWriteDashboards: boolean;
  onDeletePublication: (publication: DashboardPublication) => void;
  onCancelRefresh: (refresh: DashboardRefresh) => void;
}) {
  const {
    layout,
    orderedPublications,
    resizeCard,
    moveCard,
    resetLayout,
    hasSavedLayout,
  } = useDashboardCardLayout(dashboardID, section.section_key, publications);

  return (
    <section id={sectionAnchorID(section.section_key)} role="tabpanel" aria-labelledby={sectionTabID(section.section_key)} className="space-y-3 scroll-mt-24">
      {section.description ? <p className="min-w-0 text-sm leading-6 text-[var(--text-secondary)]">{section.description}</p> : null}

      {activeSources.length > 0 ? (
        <SectionRunningSources
          sources={activeSources}
          saving={saving}
          onCancelRefresh={canWriteDashboards && activeRefresh ? () => onCancelRefresh(activeRefresh) : undefined}
        />
      ) : null}

      {publications.length === 0 ? (
        <div className="rounded-md border border-dashed border-[var(--border-subtle)] px-4 py-6 text-sm text-[var(--text-secondary)]">
          No publications yet. Assign a dashboard-output pipeline or run a refresh to populate this section.
        </div>
      ) : (
        <>
          {hasSavedLayout ? (
            <div className="flex justify-end">
              <button
                type="button"
                className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-xs font-semibold text-[var(--text-secondary)] shadow-sm transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
                onClick={resetLayout}
              >
                <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />
                Reset card layout
              </button>
            </div>
          ) : null}
          <div className="grid gap-3 xl:grid-cols-4">
            {orderedPublications.map((publication, index) => {
              const cardKey = dashboardCardLayoutItemKey(publication);
              const currentSize = layout[cardKey]?.size || defaultPublicationCardSize(publication, publications.length);
              return (
                <PublicationCard
                  key={cardKey}
                  publication={publication}
                  cardSize={currentSize}
                  canMoveEarlier={index > 0}
                  canMoveLater={index < orderedPublications.length - 1}
                  canWriteDashboards={canWriteDashboards}
                  onMoveCard={direction => moveCard(cardKey, direction)}
                  onResizeCard={size => resizeCard(cardKey, size)}
                  onDeletePublication={onDeletePublication}
                />
              );
            })}
          </div>
        </>
      )}

    </section>
  );
}

function PublicationCard({
  publication,
  cardSize,
  canMoveEarlier,
  canMoveLater,
  canWriteDashboards,
  onMoveCard,
  onResizeCard,
  onDeletePublication,
}: {
  publication: DashboardPublication;
  cardSize: DashboardCardSize;
  canMoveEarlier: boolean;
  canMoveLater: boolean;
  canWriteDashboards: boolean;
  onMoveCard: (direction: 'earlier' | 'later') => void;
  onResizeCard: (size: DashboardCardSize) => void;
  onDeletePublication: (publication: DashboardPublication) => void;
}) {
  const cardTitle = publication.content.title || publication.entry_key;
  return (
    <article
      aria-label={`Dashboard card ${cardTitle}`}
      className={`min-w-0 overflow-hidden rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm ${publicationCardSizeClass(cardSize)}`}
    >
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-[var(--accent-soft)] text-[var(--accent)]">
            <ObjectIcon type="dashboard" className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-bold text-[var(--text-primary)]">{cardTitle}</div>
            <div className="mt-1 truncate text-xs text-[var(--text-muted)]">
              {publication.pipeline_id} / {publication.output_name} / {runScopeLabel(publication.run_scope)}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          <ToolbarButton
            label={`Move card ${cardTitle} earlier`}
            icon={<ArrowLeft className="h-4 w-4" />}
            onClick={() => onMoveCard('earlier')}
            disabled={!canMoveEarlier}
          />
          <ToolbarButton
            label={`Move card ${cardTitle} later`}
            icon={<ArrowRight className="h-4 w-4" />}
            onClick={() => onMoveCard('later')}
            disabled={!canMoveLater}
          />
          <label className="min-w-28">
            <span className="sr-only">Card size for {cardTitle}</span>
            <select
              className="h-9 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2 text-xs font-semibold text-[var(--text-secondary)] shadow-sm outline-none transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] focus:border-[var(--accent)]"
              aria-label={`Card size for ${cardTitle}`}
              value={cardSize}
              onChange={event => onResizeCard(event.target.value as DashboardCardSize)}
            >
              {DASHBOARD_CARD_SIZE_OPTIONS.map(option => (
                <option key={option.id} value={option.id}>{option.label}</option>
              ))}
            </select>
          </label>
          <span className="runner-pill runner-pill--muted">
            {publication.mode}
          </span>
          <span className={`runner-pill ${publication.stale ? 'runner-pill--warning' : 'runner-pill--ok'}`}>
            {staleLabel(publication)}
          </span>
          {canWriteDashboards ? (
            <ToolbarButton
              label={`Remove entry ${publication.entry_key}`}
              icon={<Trash2 className="h-4 w-4" />}
              onClick={() => onDeletePublication(publication)}
              danger
            />
          ) : null}
        </div>
      </header>
      <div className="p-4">
        <DashboardBlocks spec={publication.content} />
      </div>
      <footer className="flex flex-wrap items-center gap-3 border-t border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-4 py-3 text-xs text-[var(--text-secondary)]">
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
      </footer>
    </article>
  );
}

function defaultPublicationCardSize(publication: DashboardPublication, publicationCount: number): DashboardCardSize {
  if (publicationCount === 1 || dashboardSpecNeedsWideLayout(publication.content)) return 'wide';
  return 'standard';
}

function publicationCardSizeClass(size: DashboardCardSize): string {
  if (size === 'compact') return 'xl:col-span-1';
  if (size === 'wide') return 'xl:col-span-4';
  return 'xl:col-span-2';
}

function SectionRunningSources({
  sources,
  saving,
  onCancelRefresh,
}: {
  sources: NonNullable<DashboardRefresh['sources']>;
  saving: boolean;
  onCancelRefresh?: () => void;
}) {
  return (
    <div className="rounded-md border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-900 shadow-sm dark:border-sky-800/60 dark:bg-sky-950/30 dark:text-sky-100">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="flex min-w-0 items-start gap-2">
          <RefreshCw className="mt-0.5 h-4 w-4 shrink-0 animate-spin" aria-hidden="true" />
          <div className="min-w-0">
            <div className="font-semibold">{sources.length === 1 ? 'Dashboard output generating' : `${sources.length} dashboard outputs generating`}</div>
            <div className="mt-1 text-xs opacity-80">This section is waiting for dashboard output publication. One pipeline run can generate multiple section outputs.</div>
            {onCancelRefresh ? (
              <button
                type="button"
                className="mt-3 inline-flex h-8 items-center gap-2 rounded-md border border-rose-200 bg-white/80 px-3 text-xs font-semibold text-rose-700 shadow-sm transition hover:border-rose-300 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-rose-800/70 dark:bg-rose-950/20 dark:text-rose-100 dark:hover:bg-rose-950/40"
                onClick={onCancelRefresh}
                disabled={saving}
              >
                <X className="h-3.5 w-3.5" aria-hidden="true" />
                Cancel refresh
              </button>
            ) : null}
          </div>
        </div>
        <div className="grid min-w-0 gap-2 md:min-w-[360px]">
          {sources.map(source => (
            <div key={source.id} className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-md bg-white/70 px-3 py-2 dark:bg-slate-900/40">
              <div className="min-w-0">
                <div className="truncate text-xs font-semibold">{source.output_name || source.entry_key || 'Dashboard output'}</div>
                <div className="truncate text-xs opacity-80">{source.pipeline_id} / {source.entry_key || 'output name'} / {runScopeLabel(source.run_scope)}</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  <RefreshSourceStatusBadges source={source} />
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
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
  titleID,
  dashboard,
  sources,
  latestRefresh,
  activeRefresh,
  refreshes,
  schedules,
  history,
  latestRefreshSourceByID,
  saving,
  canWriteDashboards,
  onEditSource,
  onDeleteSource,
  onCancelRefresh,
  onRetryRefresh,
  onCreateSchedule,
  onEditSchedule,
  onDeleteSchedule,
  onToggleSchedule,
  onRunSchedule,
}: {
  titleID?: string;
  dashboard: DashboardSummary;
  sources: DashboardSource[];
  latestRefresh: DashboardRefresh | null;
  activeRefresh: DashboardRefresh | null;
  refreshes: DashboardRefresh[];
  schedules: DashboardRefreshSchedule[];
  history: DashboardEvent[];
  latestRefreshSourceByID: Map<string, DashboardRefreshSource>;
  saving: boolean;
  canWriteDashboards: boolean;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
  onCancelRefresh: (refresh: DashboardRefresh) => void;
  onRetryRefresh: (refresh: DashboardRefresh) => void;
  onCreateSchedule: () => void;
  onEditSchedule: (schedule: DashboardRefreshSchedule) => void;
  onDeleteSchedule: (schedule: DashboardRefreshSchedule) => void;
  onToggleSchedule: (schedule: DashboardRefreshSchedule, enabled: boolean) => void;
  onRunSchedule: (schedule: DashboardRefreshSchedule) => void;
}) {
  const [activeTab, setActiveTab] = useState<DashboardDetailTabID>('sources');
  const latestRuns = useMemo(() => latestRunHistoryItems(refreshes, history), [history, refreshes]);
  const tabs: Array<{ id: DashboardDetailTabID; label: string; count: number; icon: ReactNode }> = [
    { id: 'sources', label: 'Sources', count: sources.length, icon: <Clock3 className="h-4 w-4" /> },
    { id: 'schedules', label: 'Schedules', count: schedules.length, icon: <CalendarClock className="h-4 w-4" /> },
    { id: 'refreshes', label: 'Refreshes', count: refreshes.length, icon: <RefreshCw className="h-4 w-4" /> },
    { id: 'latest-runs', label: 'Latest runs', count: latestRuns.length, icon: <AlertTriangle className="h-4 w-4" /> },
  ];

  return (
    <section className="overflow-hidden rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm">
      <header className="border-b border-[var(--border-primary)] px-3 py-3">
        <div id={titleID} className="flex items-center gap-2 pr-10 text-sm font-semibold text-[var(--text-primary)]">
          <Info className="h-4 w-4" aria-hidden="true" />
          Dashboard details
        </div>
        <div className="mt-3 grid gap-2 text-sm md:grid-cols-2 xl:grid-cols-3">
          <DetailValue label="Team" value={dashboard.team_path || '-'} />
          <DetailValue label="Slug" value={dashboard.slug || '-'} />
          <DetailValue label="Visibility" value={dashboard.visibility || 'team'} />
          <DetailValue label="Updated" value={formatDateTime(dashboard.updated_at) || '-'} />
          <DetailValue label="Latest refresh" value={latestRefresh ? refreshStatusLabel(latestRefresh.status) : 'No refreshes'} />
          <DetailValue label="Source" value={dashboard.managed_by_config_repo ? 'GitOps' : 'Database'} />
          <DetailValue label="Config path" value={dashboard.config_source_path || '-'} />
        </div>
      </header>
      <div className="border-b border-[var(--border-primary)] px-2 py-2">
        <div className="flex gap-1 overflow-x-auto" role="tablist" aria-label="Dashboard detail views">
          {tabs.map(tab => (
            <button
              key={tab.id}
              id={dashboardDetailTabID(tab.id)}
              type="button"
              role="tab"
              aria-selected={activeTab === tab.id}
              aria-controls={dashboardDetailPanelID(tab.id)}
              className={`inline-flex min-h-9 shrink-0 items-center gap-2 rounded-md border px-3 text-sm font-semibold transition-colors ${
                activeTab === tab.id
                  ? 'border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--text-primary)] shadow-sm'
                  : 'border-transparent text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
              }`}
              onClick={() => setActiveTab(tab.id)}
            >
              {tab.icon}
              <span>{tab.label}</span>
              <span className={`rounded-md px-2 py-0.5 text-[11px] font-bold ${activeTab === tab.id ? 'bg-[var(--bg-primary)] text-[var(--text-primary)]' : 'bg-[var(--bg-tertiary)] text-[var(--text-muted)]'}`}>
                {tab.count}
              </span>
            </button>
          ))}
        </div>
      </div>
      <div id={dashboardDetailPanelID(activeTab)} role="tabpanel" aria-labelledby={dashboardDetailTabID(activeTab)} className="p-3">
        {activeTab === 'sources' ? (
          <SourceList
            sources={sources}
            latestRefreshSourceByID={latestRefreshSourceByID}
            canWriteDashboards={canWriteDashboards}
            onEditSource={onEditSource}
            onDeleteSource={onDeleteSource}
          />
        ) : null}
        {activeTab === 'schedules' ? (
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
        ) : null}
        {activeTab === 'refreshes' ? (
          <RefreshList
            refreshes={refreshes}
            activeRefresh={activeRefresh}
            saving={saving}
            canWriteDashboards={canWriteDashboards}
            onCancelRefresh={onCancelRefresh}
            onRetryRefresh={onRetryRefresh}
          />
        ) : null}
        {activeTab === 'latest-runs' ? <LatestRunHistoryList items={latestRuns} /> : null}
      </div>
    </section>
  );
}

function SourceList({
  sources,
  latestRefreshSourceByID,
  canWriteDashboards,
  onEditSource,
  onDeleteSource,
}: {
  sources: DashboardSource[];
  latestRefreshSourceByID: Map<string, DashboardRefreshSource>;
  canWriteDashboards: boolean;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
}) {
  if (sources.length === 0) return <EmptyDetail label="No sources attached." />;
  return (
    <div className="space-y-2">
      {sources.map(source => (
        <SourceRow
          key={source.id}
          source={source}
          refreshSource={latestRefreshSourceByID.get(source.id)}
          canWriteDashboards={canWriteDashboards}
          onEditSource={onEditSource}
          onDeleteSource={onDeleteSource}
        />
      ))}
    </div>
  );
}

function SourceRow({
  source,
  refreshSource,
  canWriteDashboards,
  onEditSource,
  onDeleteSource,
}: {
  source: DashboardSource;
  refreshSource?: DashboardRefreshSource;
  canWriteDashboards: boolean;
  onEditSource: (source: DashboardSource) => void;
  onDeleteSource: (source: DashboardSource) => void;
}) {
  return (
    <div className="flex flex-col gap-2 rounded-md bg-[var(--bg-primary)] px-3 py-2 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="truncate text-sm font-medium text-[var(--text-primary)]">{source.pipeline_id}</div>
          <Badge>{source.section_key}</Badge>
        </div>
        <div className="mt-1 truncate text-xs text-[var(--text-muted)]">
          {source.output_name}{source.entry_key ? ` / ${source.entry_key}` : ' / output name'} / {runScopeLabel(source.run_scope)} / order {source.refresh_order}
        </div>
        <div className="mt-2 flex flex-wrap gap-1 text-xs">
          <Badge>{source.enabled ? 'Enabled' : 'Disabled'}</Badge>
          <Badge>{source.required_for_refresh ? 'Required' : 'Optional'}</Badge>
          {refreshSource ? <RefreshSourceStatusBadges source={refreshSource} /> : null}
        </div>
        {refreshSource ? <RefreshSourceTiming source={refreshSource} /> : null}
        {refreshSource?.error ? <div className="mt-1 text-xs text-rose-600">{refreshSource.error}</div> : null}
      </div>
      {canWriteDashboards ? (
        <div className="flex shrink-0 items-center gap-2">
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

type LatestRunHistoryItem = {
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

function LatestRunHistoryList({ items }: { items: LatestRunHistoryItem[] }) {
  const visibleItems = items.slice(0, 8);
  if (visibleItems.length === 0) return <EmptyDetail label="No run history yet." />;
  return (
    <div className="space-y-2">
      {visibleItems.map(item => (
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
            <span>{item.type === 'run' ? 'Output attempt' : 'Publication event'}</span>
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

function RefreshList({
  refreshes,
  activeRefresh,
  saving,
  canWriteDashboards,
  onCancelRefresh,
  onRetryRefresh,
}: {
  refreshes: DashboardRefresh[];
  activeRefresh: DashboardRefresh | null;
  saving: boolean;
  canWriteDashboards: boolean;
  onCancelRefresh: (refresh: DashboardRefresh) => void;
  onRetryRefresh: (refresh: DashboardRefresh) => void;
}) {
  const [expandedRefreshID, setExpandedRefreshID] = useState<string | null>(null);
  if (refreshes.length === 0) return <EmptyDetail label="No refreshes." />;
  return (
    <div className="space-y-2">
      {refreshes.map(refresh => {
        const progress = refreshProgress(refresh);
        const expanded = expandedRefreshID === refresh.id;
        const canCancel = canWriteDashboards && activeRefresh?.id === refresh.id;
        const canRetry = canWriteDashboards && (refresh.failed_sources > 0 || refresh.skipped_sources > 0);
        return (
          <div key={refresh.id} className="rounded-md bg-[var(--bg-primary)] px-3 py-3">
            <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
              <div className="min-w-0">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <StatusBadge status={refresh.status} />
                  <span className="text-xs font-medium text-[var(--text-muted)]">{refresh.scope_type} / {refresh.mode}</span>
                  <span className="text-xs text-[var(--text-muted)]">{formatDateTime(refresh.created_at)}</span>
                </div>
                <div className="mt-2 flex flex-wrap gap-3 text-xs text-[var(--text-secondary)]">
                  <span>{refresh.successful_sources}/{refresh.total_sources} sources complete</span>
                  <span>{refresh.failed_sources} failed</span>
                  <span>{refresh.skipped_sources} skipped</span>
                  <span>{refresh.running_sources + refresh.queued_sources} active</span>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <ToolbarButton
                  label={expanded ? 'Hide refresh details' : 'Show refresh details'}
                  icon={<Info className="h-4 w-4" />}
                  onClick={() => setExpandedRefreshID(expanded ? null : refresh.id)}
                  pressed={expanded}
                />
                {canRetry ? <ToolbarButton label="Retry failed sources" icon={<RotateCcw className="h-4 w-4" />} onClick={() => onRetryRefresh(refresh)} disabled={saving} /> : null}
                {canCancel ? <ToolbarButton label="Cancel refresh" icon={<X className="h-4 w-4" />} onClick={() => onCancelRefresh(refresh)} disabled={saving} danger /> : null}
              </div>
            </div>
            <div className="mt-3 h-2 overflow-hidden rounded-md bg-[var(--bg-secondary)]">
              <div className="h-full rounded-md bg-[var(--accent)]" style={{ width: `${progress}%` }} />
            </div>
            {expanded ? <RefreshSourceAttemptList refresh={refresh} /> : null}
          </div>
        );
      })}
    </div>
  );
}

function RefreshSourceAttemptList({ refresh }: { refresh: DashboardRefresh }) {
  const sources = refresh.sources || [];
  if (sources.length === 0) {
    return refresh.error ? <div className="mt-3 text-xs text-rose-600">{refresh.error}</div> : null;
  }
  return (
    <div className="mt-3 grid gap-2 md:grid-cols-2">
      {sources.map(source => (
        <div key={source.id} className="rounded-md bg-[var(--bg-secondary)] px-3 py-2">
          <div className="truncate text-xs font-semibold text-[var(--text-primary)]">{source.pipeline_id}</div>
          <div className="mt-1 flex flex-wrap gap-2 text-xs text-[var(--text-muted)]">
            <span>{source.section_key} / {source.output_name} / {runScopeLabel(source.run_scope)}</span>
            <span>{source.required ? 'required' : 'optional'}</span>
          </div>
          <div className="mt-2 flex flex-wrap gap-1 text-xs">
            <RefreshSourceStatusBadges source={source} />
          </div>
          <RefreshSourceTiming source={source} />
          {source.error ? <div className="mt-1 text-xs text-rose-600">{source.error}</div> : null}
          {source.run_id ? (
            <Link className="mt-2 inline-flex text-xs font-medium text-[var(--accent)] hover:underline" to={runDetailHref(source.run_id)}>
              Run {source.run_id.slice(0, 8)}
            </Link>
          ) : null}
        </div>
      ))}
    </div>
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
    <div className="rounded-md border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-10 text-center text-sm text-[var(--text-secondary)]">
      {label}
    </div>
  );
}

function Badge({ children }: { children: ReactNode }) {
  return <span className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2 py-1 text-xs font-semibold text-[var(--text-secondary)]">{children}</span>;
}

function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase();
  const tone = normalized === 'complete' || normalized === 'success' || normalized === 'published'
    ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100'
    : normalized === 'running' || normalized === 'generating'
      ? 'bg-sky-100 text-sky-800 dark:bg-sky-950/30 dark:text-sky-100'
      : normalized === 'queued' || normalized === 'pending'
        ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/30 dark:text-amber-100'
      : normalized === 'failed' || normalized === 'failure' || normalized === 'timed_out'
        ? 'bg-rose-100 text-rose-800 dark:bg-rose-950/30 dark:text-rose-100'
        : normalized === 'cancelled'
          ? 'bg-orange-100 text-orange-800 dark:bg-orange-950/30 dark:text-orange-100'
          : 'bg-[var(--bg-primary)] text-[var(--text-secondary)]';
  return <span className={`rounded-md px-2 py-1 text-xs ${tone}`}>{refreshStatusLabel(status)}</span>;
}

function RefreshSourceStatusBadges({ source }: { source: DashboardRefreshSource }) {
  const pipelineStatus = source.pipeline_status || source.status;
  const outputStatus = source.output_status || legacyOutputStatus(source.status);
  return (
    <>
      <LabeledStatusBadge label="Refresh" status={source.status} />
      {pipelineStatus ? <LabeledStatusBadge label="Pipeline" status={pipelineStatus} /> : null}
      {outputStatus ? <LabeledStatusBadge label="Output" status={outputStatus} /> : null}
    </>
  );
}

function LabeledStatusBadge({ label, status }: { label: string; status: string }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-[var(--bg-secondary)] p-1 dark:bg-black/20">
      <span className="px-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--text-muted)]">{label}</span>
      <StatusBadge status={status} />
    </span>
  );
}

function RefreshSourceTiming({ source }: { source: DashboardRefreshSource }) {
  const outputDuration = source.output_duration || (source.output_duration_seconds ? formatDurationSeconds(source.output_duration_seconds) : '');
  const parts = [
    source.pipeline_finished_at
      ? `Pipeline finished ${formatDateTime(source.pipeline_finished_at)}`
      : source.pipeline_started_at
        ? `Pipeline started ${formatDateTime(source.pipeline_started_at)}`
        : '',
    source.output_updated_at
      ? `Output updated ${formatDateTime(source.output_updated_at)}`
      : source.output_created_at
        ? `Output created ${formatDateTime(source.output_created_at)}`
        : '',
    outputDuration ? `Output duration ${outputDuration}` : '',
  ].filter(Boolean);
  if (parts.length === 0) return null;
  return <div className="mt-1 text-xs text-[var(--text-muted)]">{parts.join(' / ')}</div>;
}

function legacyOutputStatus(status: string) {
  const normalized = status.toLowerCase();
  if (normalized === 'success') return 'success';
  if (normalized === 'failed') return 'failure';
  if (normalized === 'cancelled' || normalized === 'timed_out') return normalized;
  return '';
}

function formatDurationSeconds(rawSeconds: number) {
  const seconds = Math.max(0, Math.round(rawSeconds));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return remainingSeconds ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
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
        className="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-secondary)] shadow-sm transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] disabled:cursor-not-allowed disabled:opacity-50"
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
  const base = 'inline-flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border-primary)] text-sm shadow-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50';
  if (options.danger) return `${base} bg-rose-50 text-rose-600 hover:bg-rose-100 dark:bg-rose-950/30 dark:text-rose-100`;
  if (options.accent) return `${base} bg-[var(--accent)] text-white hover:opacity-90`;
  if (options.pressed) return `${base} bg-[var(--bg-active)] text-[var(--text-primary)]`;
  return `${base} bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]`;
}

function latestRunHistoryItems(refreshes: DashboardRefresh[], history: DashboardEvent[]): LatestRunHistoryItem[] {
  const items: LatestRunHistoryItem[] = [];
  for (const refresh of refreshes) {
    for (const source of refresh.sources || []) {
      items.push({
        id: `run-${refresh.id}-${source.id}`,
        type: 'run',
        status: source.output_status || source.pipeline_status || source.status || refresh.status,
        title: source.output_name || source.entry_key || 'Dashboard output',
        subtitle: `${source.pipeline_id}${source.entry_key ? ` / ${source.entry_key}` : ' / output name'} / ${runScopeLabel(source.run_scope)} / ${refreshSourceProgressLabel(source)} / ${source.required ? 'required' : 'optional'} / ${refresh.trigger_type}`,
        timestamp: source.output_updated_at || source.pipeline_finished_at || source.finished_at || source.started_at || source.updated_at || source.created_at || refresh.updated_at || refresh.created_at,
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

function refreshSourceProgressLabel(source: DashboardRefreshSource) {
  const pipeline = source.pipeline_status || source.status || 'queued';
  const output = source.output_status || legacyOutputStatus(source.status) || 'pending';
  return `pipeline ${refreshStatusLabel(pipeline)} / output ${refreshStatusLabel(output)}`;
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

function sectionAnchorID(sectionKey: string): string {
  return `dashboard-section-${safeSectionIDPart(sectionKey)}`;
}

function sectionTabID(sectionKey: string): string {
  return `dashboard-tab-${safeSectionIDPart(sectionKey)}`;
}

function dashboardDetailTabID(tabID: DashboardDetailTabID): string {
  return `dashboard-details-tab-${tabID}`;
}

function dashboardDetailPanelID(tabID: DashboardDetailTabID): string {
  return `dashboard-details-panel-${tabID}`;
}

function safeSectionIDPart(sectionKey: string): string {
  return sectionKey.replace(/[^a-zA-Z0-9_-]+/g, '-');
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
