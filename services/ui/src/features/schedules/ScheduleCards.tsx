import type { ReactNode } from 'react';
import {
  CheckCircle2,
  Edit3,
  ExternalLink,
  GitBranch,
  PauseCircle,
  Play,
  Trash2,
} from 'lucide-react';
import type { PipelineSchedule } from './model';
import {
  formatDateTime,
  formatScope,
  friendlyScheduleLabel,
  scheduleRunGroupLabel,
  sourceLabel,
  statusClass,
  statusLabel,
} from './presentation';

export function ScheduleStatCard({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
  return (
    <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="text-[var(--text-secondary)]">{icon}</span>
        <span className="text-2xl font-semibold text-[var(--text-primary)]">{value}</span>
      </div>
      <p className="mt-2 text-xs font-semibold uppercase text-[var(--text-secondary)]">{label}</p>
    </div>
  );
}

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
  const lastRunAt = schedule.latest_run?.started_at || schedule.last_run_at || '';
  const canMutate = canWriteSchedules && !managed;

  return (
    <article className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-base font-semibold text-[var(--text-primary)] truncate">{schedule.name || schedule.identifier}</h2>
            <span className={`runner-pill ${schedule.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
              {schedule.enabled ? 'Enabled' : 'Disabled'}
            </span>
            {managed ? (
              <span className="runner-pill runner-pill--link">
                <GitBranch className="h-3.5 w-3.5" />
                GitOps
              </span>
            ) : null}
          </div>
          <p className="mt-1 text-xs font-mono text-[var(--text-secondary)] truncate">/{schedule.path || 'root'}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canWriteSchedules ? (
            <button
              type="button"
              className="glass-button-ghost"
              title="Run now"
              disabled={busy}
              onClick={() => onRun(schedule)}
            >
              <Play className="h-4 w-4" />
            </button>
          ) : null}
          {canMutate ? (
            <button
              type="button"
              className="glass-button-ghost"
              title={schedule.enabled ? 'Disable schedule' : 'Enable schedule'}
              disabled={busy}
              onClick={() => onEnable(schedule, !schedule.enabled)}
            >
              {schedule.enabled ? <PauseCircle className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
            </button>
          ) : null}
          {canMutate ? (
            <button type="button" className="glass-button-ghost" title="Edit schedule" disabled={busy} onClick={() => onEdit(schedule)}>
              <Edit3 className="h-4 w-4" />
            </button>
          ) : null}
          {canDeleteSchedules && !managed ? (
            <button type="button" className="glass-button-danger" title="Delete schedule" disabled={busy} onClick={() => onDelete(schedule)}>
              <Trash2 className="h-4 w-4" />
            </button>
          ) : null}
        </div>
      </div>

      {schedule.description ? <p className="mt-3 text-sm text-[var(--text-secondary)]">{schedule.description}</p> : null}

      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <ScheduleFact label="Pipeline" value={schedule.pipeline} mono />
        <ScheduleFact label="Scope" value={formatScope(schedule.scope)} mono />
        <ScheduleFact label="Run group" value={scheduleRunGroupLabel(schedule)} mono />
        <ScheduleFact label="Schedule" value={friendlyScheduleLabel(schedule)} />
        <ScheduleFact label="Timezone" value={schedule.timezone || 'UTC'} />
        <ScheduleFact label="Next run" value={formatDateTime(schedule.next_run_at, schedule.timezone)} />
        <ScheduleFact label="Last run" value={formatDateTime(lastRunAt, schedule.timezone)} />
      </div>

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-primary)] pt-4">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`runner-pill ${statusClass(latestStatus)}`}>{statusLabel(latestStatus)}</span>
          <span className="text-xs text-[var(--text-secondary)] truncate">{sourceLabel(schedule.source)}</span>
        </div>
        {latestRunID ? (
          <button type="button" className="glass-button-ghost" onClick={() => onOpenRun(latestRunID)}>
            <ExternalLink className="h-4 w-4" />
            <span>Latest run</span>
          </button>
        ) : null}
      </div>
    </article>
  );
}

function ScheduleFact({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3">
      <p className="text-[0.68rem] font-semibold uppercase text-[var(--text-secondary)]">{label}</p>
      <p className={`mt-1 truncate text-sm text-[var(--text-primary)] ${mono ? 'font-mono' : ''}`} title={value}>
        {value}
      </p>
    </div>
  );
}
