import { Shield } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
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
    : 'root';
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
      size="wide"
      bodyClassName="space-y-4"
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
      <div className="grid gap-4 md:grid-cols-2">
            <label className="space-y-1">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Name</span>
              <input
                className="pipelines-input w-full"
                value={form.name}
                onChange={event => update({ name: event.target.value })}
                disabled={disabled}
                data-dialog-initial-focus
              />
            </label>
            <label className="space-y-1 md:col-span-2">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Pipeline</span>
              <select
                className="pipelines-input w-full"
                value={form.pipeline}
                onChange={event => {
                  const pipeline = normalizeIdentifier(event.target.value);
                  update({
                    pipeline,
                    runTeamPath:
                      selectedRunTeamPath && selectedRunTeamPath !== 'root'
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
            </label>
            <label className="space-y-1">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Run team</span>
              <select
                className="pipelines-input w-full"
                value={selectedRunTeamPath}
                onChange={event => update({ runTeamPath: event.target.value })}
                disabled={disabled}
              >
                {teamOptions.map(team => (
                  <option key={team} value={team}>
                    {team === 'root' ? 'Root' : team}
                  </option>
                ))}
              </select>
            </label>
            <label className="space-y-1">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Frequency</span>
              <select
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
            </label>
            {form.cronMode === 'once' ? (
              <>
                <label className="space-y-1">
                  <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Date</span>
                  <input
                    className="pipelines-input w-full"
                    type="date"
                    value={form.runAtDate}
                    onChange={event => updateCron({ runAtDate: event.target.value })}
                    disabled={disabled}
                  />
                </label>
                <label className="space-y-1">
                  <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Time</span>
                  <input
                    className="pipelines-input w-full"
                    type="time"
                    value={form.runAtTime}
                    onChange={event => updateCron({ runAtTime: event.target.value })}
                    disabled={disabled}
                  />
                </label>
              </>
            ) : null}
            {form.cronMode === 'minutes' ? (
              <label className="space-y-1">
                <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Every</span>
                <div className="flex items-center gap-2">
                  <input
                    className="pipelines-input w-full"
                    type="number"
                    min="1"
                    max="59"
                    value={form.intervalValue}
                    onChange={event => updateCron({ intervalValue: event.target.value })}
                    disabled={disabled}
                  />
                  <span className="text-sm text-[var(--text-secondary)]">minutes</span>
                </div>
              </label>
            ) : null}
            {form.cronMode === 'weekly' ? (
              <fieldset className="space-y-2 md:col-span-2">
                <legend className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Days</legend>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  {WEEKDAY_OPTIONS.map(option => (
                    <label
                      key={option.value}
                      className="flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                    >
                      <input
                        type="checkbox"
                        checked={selectedWeekdays.has(option.value)}
                        onChange={() =>
                          updateCron({
                            cronWeekday: toggleCronListValue(form.cronWeekday, option.value, WEEKDAY_VALUES, '1'),
                          })
                        }
                        disabled={disabled}
                        className="h-4 w-4 rounded border-[var(--border-primary)]"
                      />
                      <span>{option.short}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
            ) : null}
            {form.cronMode === 'monthly' ? (
              <fieldset className="space-y-2 md:col-span-2">
                <legend className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Days of month</legend>
                <div className="grid grid-cols-4 gap-2 sm:grid-cols-8">
                  {MONTHDAY_VALUES.map(day => (
                    <label
                      key={day}
                      className="flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2 py-2 text-sm text-[var(--text-primary)]"
                    >
                      <input
                        type="checkbox"
                        checked={selectedMonthdays.has(day)}
                        onChange={() =>
                          updateCron({
                            cronMonthday: toggleCronListValue(form.cronMonthday, day, MONTHDAY_VALUES, '1'),
                          })
                        }
                        disabled={disabled}
                        className="h-4 w-4 rounded border-[var(--border-primary)]"
                      />
                      <span>{day}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
            ) : null}
            {form.cronMode === 'yearly' ? (
              <>
                <label className="space-y-1">
                  <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Month</span>
                  <select
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
                </label>
                <label className="space-y-1">
                  <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Day</span>
                  <select
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
                </label>
              </>
            ) : null}
            {form.cronMode === 'hourly' ? (
              <>
                <label className="space-y-1">
                  <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Every</span>
                  <div className="flex items-center gap-2">
                    <input
                      className="pipelines-input w-full"
                      type="number"
                      min="1"
                      max="23"
                      value={form.intervalValue}
                      onChange={event => updateCron({ intervalValue: event.target.value })}
                      disabled={disabled}
                    />
                    <span className="text-sm text-[var(--text-secondary)]">hours</span>
                  </div>
                </label>
                <label className="space-y-1">
                  <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Minute</span>
                  <input
                    className="pipelines-input w-full"
                    type="number"
                    min="0"
                    max="59"
                    value={form.cronMinute}
                    onChange={event => updateCron({ cronMinute: event.target.value })}
                    disabled={disabled}
                  />
                </label>
              </>
            ) : null}
            {form.cronMode !== 'once' && form.cronMode !== 'minutes' && form.cronMode !== 'hourly' && form.cronMode !== 'custom' ? (
              <label className="space-y-1">
                <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Time</span>
                <input
                  className="pipelines-input w-full"
                  type="time"
                  value={form.cronTime}
                  onChange={event => updateCron({ cronTime: event.target.value })}
                  disabled={disabled}
                />
              </label>
            ) : null}
            {form.cronMode === 'custom' ? (
              <label className="space-y-1">
                <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Expression</span>
                <input
                  className="pipelines-input w-full font-mono"
                  value={form.cron_expression}
                  onChange={event => updateCron({ cron_expression: event.target.value })}
                  disabled={disabled}
                />
              </label>
            ) : null}
            <label className="space-y-1">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Timezone</span>
              <input
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
            </label>
            <label className="space-y-1">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Scope</span>
              <select
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
            </label>
            <label className="flex items-center gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={event => update({ enabled: event.target.checked })}
                disabled={disabled}
                className="h-4 w-4 rounded border-[var(--border-primary)]"
              />
              <span className="text-sm font-semibold text-[var(--text-primary)]">Enabled</span>
            </label>
            <label className="space-y-1 md:col-span-2">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Description</span>
              <input
                className="pipelines-input w-full"
                value={form.description}
                onChange={event => update({ description: event.target.value })}
                disabled={disabled}
              />
            </label>
            <label className="space-y-1 md:col-span-2">
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Variables</span>
              <textarea
                className="pipelines-input w-full min-h-32 font-mono"
                value={form.variablesText}
                onChange={event => update({ variablesText: event.target.value })}
                disabled={disabled}
              />
            </label>
      </div>

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
