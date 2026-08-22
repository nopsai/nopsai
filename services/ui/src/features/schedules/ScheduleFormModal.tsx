import { CalendarClock, Shield } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import { WorkflowPropertyRow } from '../../components/WorkflowPrimitives';
import { GLOBAL_RESOURCE_TEAM_LABEL, GLOBAL_RESOURCE_TEAM_PATH, isGlobalResourceTeamPath } from '../../lib/resourceTeams';
import {
  MONTHDAY_VALUES,
  MONTH_OPTIONS,
  WEEKDAY_OPTIONS,
  WEEKDAY_VALUES,
  buildCronExpression,
  defaultRunTeamForPipeline,
  getTimezoneOptions,
  normalizeCronList,
  normalizeIdentifier,
  normalizeScopeOption,
  toggleCronListValue,
  uniqueRunTeamOptions,
  type CronFormFields,
  type CronMode,
  type ScheduleFormState,
  type ScheduleModalState,
} from './model';

const TIMEZONE_OPTIONS = getTimezoneOptions();

type ScheduleFormModalProps = {
  modal: ScheduleModalState;
  form: ScheduleFormState;
  formError: string | null;
  saving: boolean;
  pipelines: string[];
  runTeams: string[];
  scopes: string[];
  canSubmit: boolean;
  onChange: (form: ScheduleFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
};

export function ScheduleFormModal({
  modal,
  form,
  formError,
  saving,
  pipelines,
  runTeams,
  scopes,
  canSubmit,
  onChange,
  onClose,
  onSubmit,
}: ScheduleFormModalProps) {
  const disabled = saving || !canSubmit;
  const update = useCallback((patch: Partial<ScheduleFormState>) => onChange({ ...form, ...patch }), [form, onChange]);
  const pipelineOptions = Array.from(new Set([...pipelines, form.pipeline].map(normalizeIdentifier).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b)
  );
  const teamOptions = uniqueRunTeamOptions(runTeams);
  const selectedRunTeamPath = teamOptions.includes(normalizeIdentifier(form.runTeamPath))
    ? normalizeIdentifier(form.runTeamPath)
    : GLOBAL_RESOURCE_TEAM_PATH;
  const scopeOptions = Array.from(new Set(['', ...scopes, form.scope].map(normalizeScopeOption))).sort((a, b) => a.localeCompare(b));
  const updateCron = (patch: Partial<CronFormFields>) => {
    const next = { ...form, ...patch };
    onChange({
      ...next,
      cron_expression: next.cronMode === 'custom' ? next.cron_expression : buildCronExpression(next),
    });
  };
  const selectedWeekdays = new Set(normalizeCronList(form.cronWeekday, WEEKDAY_VALUES, '1').split(','));
  const selectedMonthdays = new Set(normalizeCronList(form.cronMonthday, MONTHDAY_VALUES, '1').split(','));
  const titleId = 'schedule-form-title';
  const errorId = 'schedule-form-error';

  useEffect(() => {
    if (form.runTeamPath === selectedRunTeamPath) return;
    update({ runTeamPath: selectedRunTeamPath });
  }, [form.runTeamPath, selectedRunTeamPath, update]);

  return (
    <WorkflowFormDialog
      id="schedule-form-modal"
      titleId={titleId}
      descriptionId={formError ? errorId : undefined}
      kicker="Schedule"
      title={modal.mode === 'edit' ? 'Edit schedule' : 'New schedule'}
      subtitle={
        modal.schedule?.managed_by_config_repo
          ? `GitOps source: ${modal.schedule.config_source_path || 'repository'}`
          : undefined
      }
      onClose={onClose}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="modal-form-body"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          {canSubmit ? (
            <button type="button" className="glass-button-primary" onClick={onSubmit} disabled={saving}>
              {saving ? 'Saving…' : 'Save'}
            </button>
          ) : null}
        </>
      )}
    >
      <div className="modal-hero">
        <input
          className="modal-hero-input"
          aria-label="Name"
          placeholder="nightly-deploy"
          value={form.name}
          onChange={event => update({ name: event.target.value })}
          disabled={disabled}
          data-dialog-initial-focus
        />
        <input
          className="modal-hero-summary"
          aria-label="Description"
          placeholder="Add an optional summary for this schedule."
          value={form.description}
          onChange={event => update({ description: event.target.value })}
          disabled={disabled}
        />
      </div>
      <hr className="modal-divider" />
      <section className="modal-property-section">
        <div className="modal-section-heading">
          <h4 className="modal-section-heading__title">
            <CalendarClock aria-hidden="true" />
            Schedule properties
          </h4>
          <span className="modal-section-heading__badge">{form.cron_expression || 'no cron yet'}</span>
        </div>
        <div className="modal-property-grid">
          <WorkflowPropertyRow label="Pipeline" hint="Pipeline to run" htmlFor="schedule-pipeline" control="wide">
            <select
              id="schedule-pipeline"
              className="pipelines-input w-full"
              value={form.pipeline}
              onChange={event => {
                const pipeline = normalizeIdentifier(event.target.value);
                update({
                  pipeline,
                  runTeamPath:
                    selectedRunTeamPath && !isGlobalResourceTeamPath(selectedRunTeamPath)
                      ? selectedRunTeamPath
                      : defaultRunTeamForPipeline(pipeline, runTeams),
                });
              }}
              disabled={disabled}
            >
              <option value="" disabled>
                Select pipeline
              </option>
              {pipelineOptions.map(pipeline => (
                <option key={pipeline} value={pipeline}>
                  {pipeline}
                </option>
              ))}
            </select>
          </WorkflowPropertyRow>
          <WorkflowPropertyRow label="Run team" hint="Owning team" htmlFor="schedule-run-team">
            <select
              id="schedule-run-team"
              className="pipelines-input w-full"
              value={selectedRunTeamPath}
              onChange={event => update({ runTeamPath: event.target.value })}
              disabled={disabled}
            >
              {teamOptions.map(team => (
                <option key={team} value={team}>
                  {isGlobalResourceTeamPath(team) ? GLOBAL_RESOURCE_TEAM_LABEL : team}
                </option>
              ))}
            </select>
          </WorkflowPropertyRow>
          <WorkflowPropertyRow label="Frequency" hint="Repeat cadence" htmlFor="schedule-frequency">
            <select
              id="schedule-frequency"
              className="pipelines-input w-full"
              value={form.cronMode}
              onChange={event => updateCron({ cronMode: event.target.value as CronMode })}
              disabled={disabled}
            >
              <option value="once">Specific date</option>
              <option value="minutes">Every N minutes</option>
              <option value="hourly">Hourly</option>
              <option value="daily">Daily</option>
              <option value="weekdays">Weekdays</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly</option>
              <option value="custom">Custom cron</option>
            </select>
          </WorkflowPropertyRow>
          {form.cronMode === 'once' ? (
            <>
              <WorkflowPropertyRow label="Date" hint="Run date" htmlFor="schedule-date">
                <input
                  id="schedule-date"
                  className="pipelines-input w-full"
                  type="date"
                  value={form.runAtDate}
                  onChange={event => updateCron({ runAtDate: event.target.value })}
                  disabled={disabled}
                />
              </WorkflowPropertyRow>
              <WorkflowPropertyRow label="Time" hint="Local start time" htmlFor="schedule-run-at-time">
                <input
                  id="schedule-run-at-time"
                  className="pipelines-input w-full"
                  type="time"
                  value={form.runAtTime}
                  onChange={event => updateCron({ runAtTime: event.target.value })}
                  disabled={disabled}
                />
              </WorkflowPropertyRow>
            </>
          ) : null}
          {form.cronMode === 'minutes' ? (
            <WorkflowPropertyRow label="Every" hint="Interval in minutes" htmlFor="schedule-interval-minutes">
              <input
                id="schedule-interval-minutes"
                className="pipelines-input w-full"
                type="number"
                min="1"
                max="59"
                value={form.intervalValue}
                onChange={event => updateCron({ intervalValue: event.target.value })}
                disabled={disabled}
              />
            </WorkflowPropertyRow>
          ) : null}
          {form.cronMode === 'hourly' ? (
            <>
              <WorkflowPropertyRow label="Every" hint="Interval in hours" htmlFor="schedule-interval-hours">
                <input
                  id="schedule-interval-hours"
                  className="pipelines-input w-full"
                  type="number"
                  min="1"
                  max="23"
                  value={form.intervalValue}
                  onChange={event => updateCron({ intervalValue: event.target.value })}
                  disabled={disabled}
                />
              </WorkflowPropertyRow>
              <WorkflowPropertyRow label="Minute" hint="Minute of hour" htmlFor="schedule-minute">
                <input
                  id="schedule-minute"
                  className="pipelines-input w-full"
                  type="number"
                  min="0"
                  max="59"
                  value={form.cronMinute}
                  onChange={event => updateCron({ cronMinute: event.target.value })}
                  disabled={disabled}
                />
              </WorkflowPropertyRow>
            </>
          ) : null}
          {form.cronMode === 'yearly' ? (
            <>
              <WorkflowPropertyRow label="Month" hint="Month of run" htmlFor="schedule-month">
                <select
                  id="schedule-month"
                  className="pipelines-input w-full"
                  value={form.cronMonth}
                  onChange={event => updateCron({ cronMonth: event.target.value })}
                  disabled={disabled}
                >
                  {MONTH_OPTIONS.map(option => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </WorkflowPropertyRow>
              <WorkflowPropertyRow label="Day" hint="Day of month" htmlFor="schedule-yearly-day">
                <select
                  id="schedule-yearly-day"
                  className="pipelines-input w-full"
                  value={form.cronMonthday}
                  onChange={event => updateCron({ cronMonthday: event.target.value })}
                  disabled={disabled}
                >
                  {Array.from({ length: 31 }, (_, index) => String(index + 1)).map(day => (
                    <option key={day} value={day}>
                      {day}
                    </option>
                  ))}
                </select>
              </WorkflowPropertyRow>
            </>
          ) : null}
          {form.cronMode !== 'once' && form.cronMode !== 'minutes' && form.cronMode !== 'hourly' && form.cronMode !== 'custom' ? (
            <WorkflowPropertyRow label="Time" hint="Local start time" htmlFor="schedule-time">
              <input
                id="schedule-time"
                className="pipelines-input w-full"
                type="time"
                value={form.cronTime}
                onChange={event => updateCron({ cronTime: event.target.value })}
                disabled={disabled}
              />
            </WorkflowPropertyRow>
          ) : null}
          {form.cronMode === 'custom' ? (
            <WorkflowPropertyRow label="Expression" hint="Raw cron" htmlFor="schedule-expression" control="wide">
              <input
                id="schedule-expression"
                className="pipelines-input w-full font-mono"
                value={form.cron_expression}
                onChange={event => updateCron({ cron_expression: event.target.value })}
                disabled={disabled}
              />
            </WorkflowPropertyRow>
          ) : null}
          <WorkflowPropertyRow label="Timezone" hint="Clock the time uses" htmlFor="schedule-timezone" control="wide">
            <input
              id="schedule-timezone"
              list="schedule-timezone-options"
              className="pipelines-input w-full"
              value={form.timezone}
              onChange={event => update({ timezone: event.target.value })}
              disabled={disabled}
            />
            <datalist id="schedule-timezone-options">
              {TIMEZONE_OPTIONS.map(zone => (
                <option key={zone} value={zone} />
              ))}
            </datalist>
          </WorkflowPropertyRow>
          <WorkflowPropertyRow label="Scope" hint="Variables and secrets" htmlFor="schedule-scope">
            <select
              id="schedule-scope"
              className="pipelines-input w-full"
              value={form.scope}
              onChange={event => update({ scope: event.target.value })}
              disabled={disabled}
            >
              {scopeOptions.map(scope => (
                <option key={scope || 'default'} value={scope}>
                  {scope || 'default'}
                </option>
              ))}
            </select>
          </WorkflowPropertyRow>
          <WorkflowPropertyRow label="Enabled" hint="Pause without deleting" htmlFor="schedule-enabled">
            <label className="modal-toggle">
              <input
                id="schedule-enabled"
                type="checkbox"
                checked={form.enabled}
                onChange={event => update({ enabled: event.target.checked })}
                disabled={disabled}
              />
              <span />
            </label>
          </WorkflowPropertyRow>
          {form.cronMode === 'weekly' ? (
            <WorkflowPropertyRow label="Days" hint="Weekdays to repeat on" span="full" layout="stacked">
              <div className="modal-chip-list">
                {WEEKDAY_OPTIONS.map(option => (
                  <label key={option.value} className="modal-chip">
                    <input
                      type="checkbox"
                      checked={selectedWeekdays.has(option.value)}
                      onChange={() =>
                        updateCron({
                          cronWeekday: toggleCronListValue(form.cronWeekday, option.value, WEEKDAY_VALUES, '1'),
                        })
                      }
                      disabled={disabled}
                    />
                    <span>{option.short}</span>
                  </label>
                ))}
              </div>
            </WorkflowPropertyRow>
          ) : null}
          {form.cronMode === 'monthly' ? (
            <WorkflowPropertyRow label="Days of month" hint="Days to repeat on" span="full" layout="stacked">
              <div className="modal-chip-list">
                {MONTHDAY_VALUES.map(day => (
                  <label key={day} className="modal-chip">
                    <input
                      type="checkbox"
                      checked={selectedMonthdays.has(day)}
                      onChange={() =>
                        updateCron({
                          cronMonthday: toggleCronListValue(form.cronMonthday, day, MONTHDAY_VALUES, '1'),
                        })
                      }
                      disabled={disabled}
                    />
                    <span>{day}</span>
                  </label>
                ))}
              </div>
            </WorkflowPropertyRow>
          ) : null}
          <WorkflowPropertyRow
            label="Variables"
            hint="One KEY=value per line"
            htmlFor="schedule-variables"
            span="full"
            layout="stacked"
          >
            <textarea
              id="schedule-variables"
              className="pipelines-input w-full min-h-32 font-mono"
              placeholder="ENVIRONMENT=staging"
              value={form.variablesText}
              onChange={event => update({ variablesText: event.target.value })}
              disabled={disabled}
            />
          </WorkflowPropertyRow>
        </div>
      </section>

      {formError ? <p id={errorId} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{formError}</p> : null}
      {modal.schedule?.managed_by_config_repo ? (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
          Saving here creates a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
        </div>
      ) : null}
      <div className="flex items-center gap-2 text-xs text-[var(--text-secondary)]">
        <Shield className="h-4 w-4" />
        <span>Runtime access is checked before saving.</span>
      </div>
    </WorkflowFormDialog>
  );
}
