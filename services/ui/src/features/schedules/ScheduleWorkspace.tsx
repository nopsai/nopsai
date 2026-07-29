import { useEffect, useMemo, useRef, useState, type MouseEvent, type ReactNode } from 'react';
import {
  CalendarClock,
  CheckCircle2,
  Edit3,
  ExternalLink,
  GitBranch,
  PauseCircle,
  Play,
  RefreshCw,
  Trash2,
  Workflow,
  X,
} from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
import {
  AI_RESOURCE_TEAM_FILTER_ALL,
} from '../system/aiResourceTeams';
import {
  AIResourceEmptyState,
  AIResourceIconAction,
  AIResourceTableHeader,
} from '../system/AIResourcePanel';
import { AIResourceMetricGrid, AIResourceWorkspace, type AIResourceWorkspaceItem } from '../system/AIResourceWorkspace';
import { formatFilteredCount } from '../system/aiResourcePresentation';
import type { PipelineSchedule } from './model';
import {
  formatDateTime,
  formatScope,
  formatTeamPath,
  friendlyScheduleLabel,
  scheduleRunTeamLabel,
} from './presentation';
import {
  filterSchedules,
  formatScheduleRatio,
  isGitOpsSchedule,
  latestScheduleRunID,
  scheduleDisplayName,
  scheduleKindLabel,
  schedulePathOptions,
  scheduleResourceID,
  scheduleSourceDetail,
  scheduleStatusHealthClass,
  scheduleStatusText,
  summarizeSchedules,
  SCHEDULE_STATE_FILTERS,
  type ScheduleStateFilter,
} from './workspaceModel';
import './scheduleWorkspace.css';

export type ScheduleWorkspaceProps = {
  schedules: PipelineSchedule[];
  teams: string[];
  loading: boolean;
  error: string | null;
  saving: boolean;
  busyScheduleID: string | null;
  searchTerm: string;
  pipelineFilter: string;
  selectedScheduleID: string;
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
  onSearchTermChange: (value: string) => void;
  onClearPipelineFilter: () => void;
  onSelectedScheduleIDChange: (scheduleID: string) => void;
  onCreate: () => void;
  onRefresh: () => void;
  onEdit: (schedule: PipelineSchedule) => void;
  onEnable: (schedule: PipelineSchedule, enabled: boolean) => void;
  onRun: (schedule: PipelineSchedule) => void;
  onDelete: (schedule: PipelineSchedule) => void;
  onOpenRun: (runID: string) => void;
};

export function ScheduleWorkspace({
  schedules,
  teams,
  loading,
  error,
  saving,
  busyScheduleID,
  searchTerm,
  pipelineFilter,
  selectedScheduleID,
  canWriteSchedules,
  canDeleteSchedules,
  onSearchTermChange,
  onClearPipelineFilter,
  onSelectedScheduleIDChange,
  onCreate,
  onRefresh,
  onEdit,
  onEnable,
  onRun,
  onDelete,
  onOpenRun,
}: ScheduleWorkspaceProps) {
  const detailRef = useRef<HTMLElement | null>(null);
  const [pathFilter, setPathFilter] = useState(AI_RESOURCE_TEAM_FILTER_ALL);
  const [stateFilter, setStateFilter] = useState<ScheduleStateFilter>('all');

  const pathOptions = useMemo(() => schedulePathOptions(schedules, teams), [schedules, teams]);
  const visibleSchedules = useMemo(
    () => filterSchedules({ schedules, searchTerm, pathFilter, stateFilter }),
    [pathFilter, schedules, searchTerm, stateFilter]
  );
  const summary = useMemo(() => summarizeSchedules(schedules, visibleSchedules), [schedules, visibleSchedules]);
  const workspaceResources = useMemo<AIResourceWorkspaceItem[]>(
    () => schedules.map(schedule => ({
      id: scheduleResourceID(schedule),
      label: scheduleDisplayName(schedule),
      description: schedule.pipeline,
    })),
    [schedules]
  );
  const schedulesByResourceID = useMemo(
    () => new Map(schedules.map(schedule => [scheduleResourceID(schedule), schedule])),
    [schedules]
  );
  const selectedSchedule = selectedScheduleID ? schedulesByResourceID.get(selectedScheduleID) ?? null : null;
  const detailOpen = Boolean(selectedSchedule);
  const filteredCountToken = [
    searchTerm,
    pipelineFilter,
    pathFilter !== AI_RESOURCE_TEAM_FILTER_ALL ? pathFilter : '',
    stateFilter !== 'all' ? stateFilter : '',
  ].filter(Boolean).join(' ');

  useEffect(() => {
    if (loading) return;
    if (selectedScheduleID && !schedulesByResourceID.has(selectedScheduleID)) {
      onSelectedScheduleIDChange('');
    }
  }, [loading, onSelectedScheduleIDChange, schedulesByResourceID, selectedScheduleID]);

  const openPathFilter = (value: string) => {
    setPathFilter(value);
    onSelectedScheduleIDChange('');
  };

  return (
    <div className={`ai-resource-panel ai-resource-page schedule-workspace ${detailOpen ? 'schedule-workspace--detail' : ''} space-y-5 pb-24`}>
      {!detailOpen ? (
        <div className="ai-resource-page-header ai-resource-page-header--toolbar ai-resource-overview-bar">
          <h2 className="sr-only">Schedules</h2>
          <div className="ai-resource-default-control schedule-workspace__scope-card">
            <span>{pipelineFilter ? 'Pipeline filter' : 'Schedule view'}</span>
            <strong>{pipelineFilter || 'All pipelines'}</strong>
            {pipelineFilter ? (
              <button type="button" onClick={onClearPipelineFilter} aria-label="Clear pipeline filter">
                <X className="h-3.5 w-3.5" aria-hidden="true" />
                Clear
              </button>
            ) : null}
          </div>
          <AIResourceMetricGrid
            metrics={[
              { label: 'Enabled', value: formatScheduleRatio(summary.enabled, summary.visible), icon: <CheckCircle2 className="h-4 w-4" />, tone: summary.visible === 0 || summary.enabled === summary.visible ? 'ok' : 'warning' },
              { label: 'Pipelines', value: summary.pipelines, icon: <Workflow className="h-4 w-4" />, tone: 'info' },
              { label: 'GitOps', value: summary.gitops, icon: <GitBranch className="h-4 w-4" />, tone: summary.gitops > 0 ? 'muted' : 'default' },
            ]}
          />
          <div className="ai-resource-page-actions">
            {!canWriteSchedules && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="ai-resource-icon-button" onClick={onRefresh} disabled={loading || saving} aria-label="Reload schedules">
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
            </button>
            {canWriteSchedules ? (
              <button type="button" className="ai-resource-primary-button" onClick={onCreate} disabled={saving}>
                <CalendarClock className="h-4 w-4" aria-hidden="true" />
                New schedule
              </button>
            ) : (
              <button type="button" className="ai-resource-primary-button" disabled title="You have read-only access to schedules">
                <CalendarClock className="h-4 w-4" aria-hidden="true" />
                New schedule
              </button>
            )}
          </div>
        </div>
      ) : null}

      {error ? <div className="ai-resource-alert ai-resource-alert--error">{error}</div> : null}

      <AIResourceWorkspace
        storageKey="pipeline-schedules"
        workspaceLabel="Pipeline schedule workspace"
        treeTitle="Schedule tree"
        resourceType="schedule"
        resourceLabel="schedule"
        resources={workspaceResources}
        teamPaths={pathOptions}
        teamFilter={pathFilter}
        selectedResourceID={selectedScheduleID}
        onTeamFilterChange={openPathFilter}
        onResourceSelect={onSelectedScheduleIDChange}
        onDetailClose={() => onSelectedScheduleIDChange('')}
        detailOpen={detailOpen}
        detailRef={detailRef}
        detailLabel="Schedule detail"
        listHeader={(
          <AIResourceTableHeader
            title="Schedules"
            count={formatFilteredCount(visibleSchedules.length, schedules.length, filteredCountToken)}
            loading={loading}
            searchLabel="Search schedules"
            searchPlaceholder="Search schedules..."
            searchValue={searchTerm}
            onSearchChange={onSearchTermChange}
            filters={(
              <div className="schedule-workspace__filters">
                <SchedulePathFilter
                  value={pathFilter}
                  pathOptions={pathOptions}
                  onChange={openPathFilter}
                />
                <ScheduleStateSegmentedFilter
                  value={stateFilter}
                  onChange={setStateFilter}
                />
              </div>
            )}
          />
        )}
        list={(
          <ScheduleTable
            schedules={visibleSchedules}
            selectedScheduleID={selectedScheduleID}
            loading={loading}
            error={error}
            onSelectSchedule={onSelectedScheduleIDChange}
            onOpenRun={onOpenRun}
          />
        )}
        detail={selectedSchedule ? (
          <ScheduleDetail
            schedule={selectedSchedule}
            busy={busyScheduleID === selectedSchedule.id}
            canWriteSchedules={canWriteSchedules}
            canDeleteSchedules={canDeleteSchedules}
            onEdit={onEdit}
            onEnable={onEnable}
            onRun={onRun}
            onDelete={onDelete}
            onOpenRun={onOpenRun}
          />
        ) : (
          <AIResourceEmptyState>Select a schedule to inspect its cadence, runtime ownership, source, and latest run.</AIResourceEmptyState>
        )}
      />
    </div>
  );
}

function SchedulePathFilter({
  value,
  pathOptions,
  onChange,
}: {
  value: string;
  pathOptions: string[];
  onChange: (value: string) => void;
}) {
  const safeValue = value === AI_RESOURCE_TEAM_FILTER_ALL || pathOptions.includes(value)
    ? value
    : AI_RESOURCE_TEAM_FILTER_ALL;

  return (
    <label className="schedule-workspace__path-filter">
      <span className="sr-only">Filter by schedule path</span>
      <select
        aria-label="Filter by schedule path"
        value={safeValue}
        onChange={event => onChange(event.target.value)}
      >
        <option value={AI_RESOURCE_TEAM_FILTER_ALL}>All paths</option>
        {pathOptions.map(path => (
          <option key={path} value={path}>
            {path === 'root' ? 'Root' : `/${path}`}
          </option>
        ))}
      </select>
    </label>
  );
}

function ScheduleStateSegmentedFilter({
  value,
  onChange,
}: {
  value: ScheduleStateFilter;
  onChange: (value: ScheduleStateFilter) => void;
}) {
  return (
    <div className="schedule-workspace__state-filter" role="tablist" aria-label="Filter by schedule state">
      {SCHEDULE_STATE_FILTERS.map(option => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={value === option.value}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function ScheduleTable({
  schedules,
  selectedScheduleID,
  loading,
  error,
  onSelectSchedule,
  onOpenRun,
}: {
  schedules: PipelineSchedule[];
  selectedScheduleID: string | null;
  loading: boolean;
  error: string | null;
  onSelectSchedule: (resourceID: string) => void;
  onOpenRun: (runID: string) => void;
}) {
  if (loading) return <AIResourceEmptyState>Loading schedules...</AIResourceEmptyState>;
  if (error) return <AIResourceEmptyState>Unable to load schedules.</AIResourceEmptyState>;
  if (!schedules.length) return <AIResourceEmptyState>No schedules match the current filters.</AIResourceEmptyState>;

  return (
    <div className="ai-resource-table-shell schedule-workspace__table-shell" data-testid="schedule-workspace-table">
      <table className="ai-resource-registry-table schedule-workspace__table" aria-label="Pipeline schedules">
        <colgroup>
          <col style={{ width: '24%' }} />
          <col style={{ width: '24%' }} />
          <col style={{ width: '22%' }} />
          <col style={{ width: '18%' }} />
          <col style={{ width: '12%' }} />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">Schedule</th>
            <th scope="col">Pipeline</th>
            <th scope="col">Cadence</th>
            <th scope="col">Next run</th>
            <th scope="col">Latest</th>
          </tr>
        </thead>
        <tbody>
          {schedules.map(schedule => {
            const resourceID = scheduleResourceID(schedule);
            const displayName = scheduleDisplayName(schedule);
            const selected = selectedScheduleID === resourceID;
            return (
              <tr key={schedule.id} className={selected ? 'selected' : ''} onClick={() => onSelectSchedule(resourceID)}>
                <td>
                  <button
                    type="button"
                    className="ai-resource-table-resource"
                    aria-label={`Select schedule ${displayName}`}
                    onClick={event => {
                      event.stopPropagation();
                      onSelectSchedule(resourceID);
                    }}
                  >
                    <span className="ai-resource-table-resource-icon" aria-hidden="true">
                      <ObjectIcon type="schedule" />
                    </span>
                    <span className="ai-resource-table-resource-name">
                      <strong>{displayName}</strong>
                    </span>
                  </button>
                </td>
                <td><span className="ai-resource-table-mono">{schedule.pipeline || '-'}</span></td>
                <td>{friendlyScheduleLabel(schedule) || '-'}</td>
                <td>{formatDateTime(schedule.next_run_at, schedule.timezone)}</td>
                <td>
                  <ScheduleLatestRunLink
                    schedule={schedule}
                    onOpenRun={onOpenRun}
                  />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function ScheduleLatestRunLink({
  schedule,
  onOpenRun,
}: {
  schedule: PipelineSchedule;
  onOpenRun: (runID: string) => void;
}) {
  const latestRunID = latestScheduleRunID(schedule);
  const status = (
    <span className={scheduleStatusHealthClass(schedule)}>
      <span aria-hidden="true" />
      {scheduleStatusText(schedule)}
    </span>
  );

  if (!latestRunID) return status;

  return (
    <button
      type="button"
      className="schedule-workspace__latest-run-link"
      aria-label={`Open latest run for ${scheduleDisplayName(schedule)}`}
      title={`Open latest run ${latestRunID}`}
      onClick={event => {
        event.stopPropagation();
        onOpenRun(latestRunID);
      }}
    >
      {status}
      <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
    </button>
  );
}

function ScheduleDetail({
  schedule,
  busy,
  canWriteSchedules,
  canDeleteSchedules,
  onEdit,
  onEnable,
  onRun,
  onDelete,
  onOpenRun,
}: {
  schedule: PipelineSchedule;
  busy: boolean;
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
  onEdit: (schedule: PipelineSchedule) => void;
  onEnable: (schedule: PipelineSchedule, enabled: boolean) => void;
  onRun: (schedule: PipelineSchedule) => void;
  onDelete: (schedule: PipelineSchedule) => void;
  onOpenRun: (runID: string) => void;
}) {
  const displayName = scheduleDisplayName(schedule);
  const managed = isGitOpsSchedule(schedule);
  const latestRunID = latestScheduleRunID(schedule);
  const variables = Object.entries(schedule.variables || {});

  return (
    <div className="ai-resource-detail schedule-detail">
      <div className="ai-resource-detail__header">
        <div>
          <div className="ai-resource-detail__title">
            <h3>{displayName}</h3>
            <span className={`runner-pill ${schedule.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
              {schedule.enabled ? 'Enabled' : 'Disabled'}
            </span>
            {managed ? (
              <span className="runner-pill runner-pill--link">
                <ObjectIcon type="gitops" className="h-3.5 w-3.5" />
                GitOps
              </span>
            ) : null}
            <span className={scheduleStatusHealthClass(schedule)}>
              <span aria-hidden="true" />
              {scheduleStatusText(schedule)}
            </span>
          </div>
          <div className="ai-resource-detail__provider schedule-detail__cadence">
            <span className="ai-resource-provider-glyph" aria-hidden="true">
              <ObjectIcon type="schedule" />
            </span>
            {friendlyScheduleLabel(schedule)}
          </div>
        </div>
        <div className="ai-resource-detail__actions">
          <ScheduleActionBar
            schedule={schedule}
            busy={busy}
            canWriteSchedules={canWriteSchedules}
            canDeleteSchedules={canDeleteSchedules}
            onEdit={onEdit}
            onEnable={onEnable}
            onRun={onRun}
            onDelete={onDelete}
            onOpenRun={onOpenRun}
          />
        </div>
      </div>

      <div className="schedule-detail__grid">
        <ScheduleDetailSection
          title="Execution"
          rows={[
            { label: 'Pipeline', value: schedule.pipeline || '-', mono: true },
            { label: 'Cadence', value: friendlyScheduleLabel(schedule) || '-' },
            { label: 'Next run', value: formatDateTime(schedule.next_run_at, schedule.timezone) },
            { label: 'Timezone', value: schedule.timezone || 'UTC' },
          ]}
        />
        <ScheduleDetailSection
          title="Ownership"
          rows={[
            { label: 'Run team', value: scheduleRunTeamLabel(schedule) },
            { label: 'Definition path', value: formatTeamPath(schedule.path) },
            { label: 'Scope', value: formatScope(schedule.scope) },
            { label: 'Source', value: scheduleSourceDetail(schedule) },
          ]}
        />
        <ScheduleDetailSection
          title="Definition"
          rows={[
            { label: 'Kind', value: scheduleKindLabel(schedule) },
            { label: 'Description', value: schedule.description || '-' },
            { label: 'Cron', value: schedule.cron_expression || schedule.cron || '-', mono: true },
            { label: 'Run at', value: formatDateTime(schedule.run_at, schedule.timezone) },
            { label: 'Identifier', value: schedule.identifier || scheduleResourceID(schedule), mono: true },
          ]}
        />
        <ScheduleDetailSection
          title="Latest run"
          rows={[
            { label: 'Status', value: scheduleStatusText(schedule) },
            {
              label: 'Run',
              value: latestRunID ? (
                <button type="button" className="schedule-detail__link-button" onClick={() => onOpenRun(latestRunID)}>
                  <span className="ai-resource-table-mono">{latestRunID}</span>
                  <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                </button>
              ) : '-',
            },
            { label: 'Last run', value: formatDateTime(schedule.last_run_at || schedule.latest_run?.started_at, schedule.timezone) },
            { label: 'Updated', value: formatDateTime(schedule.updated_at, schedule.timezone) },
          ]}
        />
      </div>

      <ScheduleDetailSection
        title="Variables"
        rows={[
          {
            label: '',
            value: variables.length ? (
              <div className="schedule-detail__variables">
                {variables.map(([key, value]) => (
                  <span key={key}>
                    <strong>{key}</strong>
                    <code>{value}</code>
                  </span>
                ))}
              </div>
            ) : 'No schedule variable overrides.',
          },
        ]}
      />

      {managed ? (
        <p className="ai-resource-detail-copy">
          This schedule is GitOps-managed. UI mutations update the runtime row; the next config sync can replace those changes unless they are committed back to the owning repository.
        </p>
      ) : null}
    </div>
  );
}

function ScheduleActionBar({
  schedule,
  busy,
  canWriteSchedules,
  canDeleteSchedules,
  onEdit,
  onEnable,
  onRun,
  onDelete,
  onOpenRun,
  stopPropagation,
}: {
  schedule: PipelineSchedule;
  busy: boolean;
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
  onEdit: (schedule: PipelineSchedule) => void;
  onEnable: (schedule: PipelineSchedule, enabled: boolean) => void;
  onRun: (schedule: PipelineSchedule) => void;
  onDelete: (schedule: PipelineSchedule) => void;
  onOpenRun: (runID: string) => void;
  stopPropagation?: boolean;
}) {
  const displayName = scheduleDisplayName(schedule);
  const latestRunID = latestScheduleRunID(schedule);
  const managed = isGitOpsSchedule(schedule);
  const handleClick = (event: MouseEvent<HTMLButtonElement>, action: () => void) => {
    if (stopPropagation) event.stopPropagation();
    action();
  };

  return (
    <div className="schedule-workspace__row-actions">
      {canWriteSchedules ? (
        <AIResourceIconAction
          label={`Run ${displayName} now`}
          tone="primary"
          disabled={busy}
          onClick={event => handleClick(event, () => onRun(schedule))}
        >
          <Play className="h-4 w-4" aria-hidden="true" />
        </AIResourceIconAction>
      ) : null}
      {latestRunID ? (
        <AIResourceIconAction
          label={`Open latest run for ${displayName}`}
          onClick={event => handleClick(event, () => onOpenRun(latestRunID))}
        >
          <ExternalLink className="h-4 w-4" aria-hidden="true" />
        </AIResourceIconAction>
      ) : null}
      {canWriteSchedules ? (
        <AIResourceIconAction
          label={`${schedule.enabled ? 'Disable' : 'Enable'} ${displayName}`}
          title={managed ? 'Save database override; GitOps can replace it on next sync' : schedule.enabled ? 'Disable schedule' : 'Enable schedule'}
          disabled={busy}
          onClick={event => handleClick(event, () => onEnable(schedule, !schedule.enabled))}
        >
          {schedule.enabled ? <PauseCircle className="h-4 w-4" aria-hidden="true" /> : <CheckCircle2 className="h-4 w-4" aria-hidden="true" />}
        </AIResourceIconAction>
      ) : null}
      {canWriteSchedules ? (
        <AIResourceIconAction
          label={`Edit ${displayName}`}
          tone="accent"
          title={managed ? 'Edit database override; GitOps can replace it on next sync' : 'Edit schedule'}
          disabled={busy}
          onClick={event => handleClick(event, () => onEdit(schedule))}
        >
          <Edit3 className="h-4 w-4" aria-hidden="true" />
        </AIResourceIconAction>
      ) : null}
      {canDeleteSchedules ? (
        <AIResourceIconAction
          label={`Delete ${displayName}`}
          tone="danger"
          title={managed ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete schedule'}
          disabled={busy}
          onClick={event => handleClick(event, () => onDelete(schedule))}
        >
          <Trash2 className="h-4 w-4" aria-hidden="true" />
        </AIResourceIconAction>
      ) : null}
    </div>
  );
}

function ScheduleDetailSection({ title, rows }: { title: string; rows: Array<{ label: string; value: ReactNode; mono?: boolean }> }) {
  return (
    <section className="ai-resource-detail-section">
      <h4>{title}</h4>
      <dl>
        {rows.map(row => (
          <div key={`${title}-${row.label || 'content'}`} className={`ai-resource-detail-row ${row.label ? '' : 'ai-resource-detail-row--full'}`}>
            {row.label ? <dt>{row.label}</dt> : null}
            <dd className={row.mono ? 'ai-resource-detail-row__mono' : undefined}>{row.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export default ScheduleWorkspace;
