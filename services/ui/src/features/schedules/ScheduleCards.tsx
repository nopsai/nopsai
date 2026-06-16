import {
  CheckCircle2,
  Edit3,
  ExternalLink,
  PauseCircle,
  Play,
  Trash2,
} from 'lucide-react';
import { CompactResourceCard } from '../../components/CompactResourceCard';
import { ObjectIcon } from '../../components/ObjectIcon';
import type { PipelineSchedule } from './model';
import {
  formatDateTime,
  friendlyScheduleLabel,
  sourceLabel,
  statusClass,
  statusLabel,
} from './presentation';

export function ScheduleCard({
  schedule,
  canWriteSchedules,
  canDeleteSchedules,
  busy,
  onEdit,
  onEnable,
  onRun,
  onDelete,
  onOpenRun,
}: {
  schedule: PipelineSchedule;
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
  busy: boolean;
  onEdit: (schedule: PipelineSchedule) => void;
  onEnable: (schedule: PipelineSchedule, enabled: boolean) => void;
  onRun: (schedule: PipelineSchedule) => void;
  onDelete: (schedule: PipelineSchedule) => void;
  onOpenRun: (runID: string) => void;
}) {
  const managed = Boolean(schedule.managed_by_config_repo || sourceLabel(schedule.source) === 'GitOps');
  const latestRunID = schedule.latest_run?.run_id || schedule.last_run_id || '';
  const latestStatus = schedule.latest_run?.status || schedule.last_status || '';
  const canMutate = canWriteSchedules;
  const canDeleteSchedule = canDeleteSchedules;
  const hasHeadingActions = canWriteSchedules || canDeleteSchedule;

  return (
    <CompactResourceCard
      className="compact-resource-card--bordered schedule-card"
      icon={<ObjectIcon type="schedule" />}
      tone="violet"
      title={schedule.name || schedule.identifier}
      subtitle={<span className="font-mono">/{schedule.path || 'root'}</span>}
      description={schedule.description}
      badges={(
        <>
          <span className={`runner-pill ${schedule.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
            {schedule.enabled ? 'Enabled' : 'Disabled'}
          </span>
          {managed ? (
            <span className="runner-pill runner-pill--link">
              <ObjectIcon type="gitops" className="h-3.5 w-3.5" />
              GitOps
            </span>
          ) : null}
          <span className={`runner-pill ${statusClass(latestStatus)}`}>{statusLabel(latestStatus)}</span>
        </>
      )}
      facts={[
        { label: 'Pipeline', value: schedule.pipeline, mono: true, title: schedule.pipeline },
        { label: 'Schedule', value: friendlyScheduleLabel(schedule), title: friendlyScheduleLabel(schedule) },
        { label: 'Next', value: formatDateTime(schedule.next_run_at, schedule.timezone) },
      ]}
      headingActions={hasHeadingActions ? (
        <>
          {canWriteSchedules ? (
            <button
              type="button"
              className="schedule-card__icon-button"
              title="Run now"
              aria-label={`Run ${schedule.name || schedule.identifier} now`}
              disabled={busy}
              onClick={() => onRun(schedule)}
            >
              <Play className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          {canDeleteSchedule ? (
            <button
              type="button"
              className="pipelines-delete-button schedule-card__delete-button"
              title={managed ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete schedule'}
              aria-label={`Delete ${schedule.name || schedule.identifier}`}
              disabled={busy}
              onClick={() => onDelete(schedule)}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </>
      ) : undefined}
      footerActions={latestRunID ? (
        <button
          type="button"
          className="schedule-card__icon-button schedule-card__latest-run-button"
          title="Open latest run"
          aria-label={`Open latest run for ${schedule.name || schedule.identifier}`}
          onClick={() => onOpenRun(latestRunID)}
        >
          <ExternalLink className="h-4 w-4" aria-hidden="true" />
        </button>
      ) : null}
      actions={canMutate ? (
        <>
          <button
            type="button"
            className="compact-resource-card__action"
            title={managed ? 'Save database override; GitOps can replace it on next sync' : schedule.enabled ? 'Disable schedule' : 'Enable schedule'}
            aria-label={`${schedule.enabled ? 'Disable' : 'Enable'} ${schedule.name || schedule.identifier}`}
            disabled={busy}
            onClick={() => onEnable(schedule, !schedule.enabled)}
          >
            {schedule.enabled ? <PauseCircle className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
          </button>
          <button
            type="button"
            className="compact-resource-card__action"
            title={managed ? 'Edit database override; GitOps can replace it on next sync' : 'Edit schedule'}
            aria-label={`Edit ${schedule.name || schedule.identifier}`}
            disabled={busy}
            onClick={() => onEdit(schedule)}
          >
            <Edit3 className="h-4 w-4" />
          </button>
        </>
      ) : undefined}
    />
  );
}
