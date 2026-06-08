import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  CalendarClock,
  CheckCircle2,
  Clock3,
  Edit3,
  ExternalLink,
  GitBranch,
  PauseCircle,
  Play,
  Plus,
  RefreshCw,
  Search,
  Shield,
  Trash2,
  X,
} from 'lucide-react';

import {
  deleteSchedule as deleteScheduleRequest,
  fetchScheduleMetadata,
  fetchSchedules,
  runSchedule,
  saveSchedule,
  setScheduleEnabled as setScheduleEnabledRequest,
} from '../features/schedules/api';
import {
  MONTHDAY_VALUES,
  MONTH_OPTIONS,
  WEEKDAY_OPTIONS,
  WEEKDAY_VALUES,
  buildCronExpression,
  createEmptyForm,
  defaultRunGroupForPipeline,
  effectiveScheduleRunGroupPath,
  formFromSchedule,
  friendlyCronLabel,
  getTimezoneOptions,
  normalizeCronList,
  normalizeIdentifier,
  normalizeScheduleKind,
  normalizeScopeOption,
  toggleCronListValue,
  uniqueRunGroupOptions,
  type CronFormFields,
  type CronMode,
  type PipelineSchedule,
  type ScheduleFormState,
  type ScheduleModalState,
} from '../features/schedules/model';
import { useDialogFocus } from '../components/useDialogFocus';

type SchedulesPageProps = {
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
};

const TIMEZONE_OPTIONS = getTimezoneOptions();

function sourceLabel(source?: string) {
  const normalized = (source || '').trim().toLowerCase();
  if (normalized.includes('git')) return 'GitOps';
  if (normalized.includes('db') || normalized.includes('database')) return 'Database';
  return normalized || 'Database';
}

function statusLabel(status?: string) {
  const normalized = (status || '').trim();
  if (!normalized) return 'No runs';
  return normalized
    .split(/[\s_-]+/)
    .filter(Boolean)
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

function statusClass(status?: string) {
  const normalized = (status || '').toLowerCase();
  if (normalized.includes('success')) return 'runner-pill--ok';
  if (normalized.includes('fail') || normalized.includes('cancel')) return 'runner-pill--error';
  if (normalized.includes('running') || normalized.includes('pending')) return 'runner-pill--warning';
  return 'runner-pill--muted';
}

function formatDateTime(value?: string, timeZone?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  const options: Intl.DateTimeFormatOptions = {
    weekday: 'short',
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'short',
  };
  const normalizedZone = (timeZone || '').trim();
  try {
    return new Intl.DateTimeFormat(undefined, normalizedZone ? { ...options, timeZone: normalizedZone } : options).format(date);
  } catch {
    return new Intl.DateTimeFormat(undefined, options).format(date);
  }
}

function formatScope(scope?: string) {
  const normalized = normalizeScopeOption(scope);
  return normalized || 'default';
}

function formatGroupPath(path?: string) {
  const normalized = normalizeIdentifier(path);
  return normalized === 'root' || !normalized ? 'Root' : normalized;
}

function friendlyScheduleLabel(schedule: PipelineSchedule) {
  if (normalizeScheduleKind(schedule.schedule_kind) === 'once') {
    return `Once at ${formatDateTime(schedule.run_at || schedule.next_run_at, schedule.timezone)}`;
  }
  return friendlyCronLabel(schedule.cron_expression || schedule.cron);
}

export default function SchedulesPage({ canWriteSchedules, canDeleteSchedules }: SchedulesPageProps) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const pipelineFilter = normalizeIdentifier(searchParams.get('pipeline') || '');
  const activeGroup = normalizeIdentifier(searchParams.get('folder') || '') || 'all';

  const [schedules, setSchedules] = useState<PipelineSchedule[]>([]);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [groups, setGroups] = useState<string[]>([]);
  const [scopes, setScopes] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [showDisabled, setShowDisabled] = useState(true);
  const [modal, setModal] = useState<ScheduleModalState | null>(null);
  const [form, setForm] = useState<ScheduleFormState>(() => createEmptyForm(''));
  const [formError, setFormError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [busyScheduleID, setBusyScheduleID] = useState<string | null>(null);

  const loadSchedules = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setSchedules(await fetchSchedules(pipelineFilter));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load schedules');
      setSchedules([]);
    } finally {
      setLoading(false);
    }
  }, [pipelineFilter]);

  useEffect(() => {
    void loadSchedules();
  }, [loadSchedules]);

  useEffect(() => {
    let cancelled = false;
    const loadMeta = async () => {
      try {
        const metadata = await fetchScheduleMetadata();
        if (cancelled) return;
        setPipelines(metadata.pipelines);
        setGroups(metadata.groups);
        setScopes(metadata.scopes);
      } catch {
        if (!cancelled) {
          setPipelines([]);
          setGroups([]);
          setScopes(['']);
        }
      }
    };
    void loadMeta();
    return () => {
      cancelled = true;
    };
  }, []);

  const scheduleGroupOptions = useMemo(() => {
    const values = new Set<string>(groups);
    schedules.forEach(schedule => {
      const path = normalizeIdentifier(schedule.path);
      if (path) values.add(path);
    });
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [groups, schedules]);

  const runGroupOptions = useMemo(() => uniqueRunGroupOptions(groups), [groups]);

  const scopeOptions = useMemo(() => {
    const values = new Set(scopes.map(normalizeScopeOption));
    schedules.forEach(schedule => values.add(normalizeScopeOption(schedule.scope)));
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [schedules, scopes]);

  const filteredSchedules = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    return schedules.filter(schedule => {
      if (!showDisabled && !schedule.enabled) return false;
      const path = normalizeIdentifier(schedule.path);
      if (activeGroup !== 'all') {
        if (activeGroup === 'root') {
          if (path) return false;
        } else if (path !== activeGroup && !path.startsWith(`${activeGroup}/`)) {
          return false;
        }
      }
      if (!term) return true;
      const haystack = [
        schedule.identifier,
        schedule.name,
        schedule.path,
        schedule.pipeline,
        schedule.cron_expression,
        schedule.schedule_kind,
        schedule.run_at,
        schedule.timezone,
        schedule.scope,
        schedule.last_status,
        sourceLabel(schedule.source),
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(term);
    });
  }, [activeGroup, schedules, searchTerm, showDisabled]);

  const stats = useMemo(() => {
    const enabled = schedules.filter(schedule => schedule.enabled).length;
    const due = schedules.filter(schedule => schedule.enabled && schedule.next_run_at).length;
    const gitops = schedules.filter(schedule => schedule.managed_by_config_repo || sourceLabel(schedule.source) === 'GitOps').length;
    return { total: schedules.length, enabled, due, gitops };
  }, [schedules]);

  const setFolderFilter = useCallback(
    (folder: string) => {
      const params = new URLSearchParams(searchParams);
      const normalized = normalizeIdentifier(folder);
      if (!normalized || normalized === 'all') params.delete('folder');
      else params.set('folder', normalized);
      setSearchParams(params, { replace: true });
    },
    [searchParams, setSearchParams]
  );

  const clearPipelineFilter = useCallback(() => {
    const params = new URLSearchParams(searchParams);
    params.delete('pipeline');
    setSearchParams(params, { replace: true });
  }, [searchParams, setSearchParams]);

  const openCreate = useCallback(() => {
    setForm(createEmptyForm(pipelineFilter, runGroupOptions));
    setFormError(null);
    setModal({ mode: 'create' });
  }, [pipelineFilter, runGroupOptions]);

  const openEdit = useCallback((schedule: PipelineSchedule) => {
    setForm(formFromSchedule(schedule));
    setFormError(null);
    setModal({ mode: 'edit', schedule });
  }, []);

  const closeModal = useCallback(() => {
    if (saving) return;
    setModal(null);
    setFormError(null);
  }, [saving]);

  const submitForm = useCallback(async () => {
    if (!modal || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      const editingSchedule = modal.mode === 'edit' ? modal.schedule : undefined;
      const updated = await saveSchedule(form, editingSchedule);
      setSchedules(prev => {
        if (!editingSchedule) return [...prev, updated].sort((a, b) => a.identifier.localeCompare(b.identifier));
        return prev.map(item => (item.id === updated.id ? updated : item));
      });
      setModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save schedule');
    } finally {
      setSaving(false);
    }
  }, [form, modal, saving]);

  const setScheduleEnabled = useCallback(
    async (schedule: PipelineSchedule, enabled: boolean) => {
      if (busyScheduleID) return;
      setBusyScheduleID(schedule.id);
      try {
        const updated = await setScheduleEnabledRequest(schedule.id, enabled);
        setSchedules(prev => prev.map(item => (item.id === updated.id ? updated : item)));
      } catch (err) {
        alert(err instanceof Error ? err.message : 'Unable to update schedule');
      } finally {
        setBusyScheduleID(null);
      }
    },
    [busyScheduleID]
  );

  const runNow = useCallback(
    async (schedule: PipelineSchedule) => {
      if (busyScheduleID) return;
      setBusyScheduleID(schedule.id);
      try {
        const result = await runSchedule(schedule.id);
        await loadSchedules();
        if (result.run_id) {
          navigate(`/pipelineruns/recent?run=${encodeURIComponent(result.run_id)}`);
        }
      } catch (err) {
        alert(err instanceof Error ? err.message : 'Unable to run schedule');
      } finally {
        setBusyScheduleID(null);
      }
    },
    [busyScheduleID, loadSchedules, navigate]
  );

  const deleteSchedule = useCallback(
    async (schedule: PipelineSchedule) => {
      if (!window.confirm(`Delete schedule ${schedule.identifier}?`)) return;
      setBusyScheduleID(schedule.id);
      try {
        await deleteScheduleRequest(schedule.id);
        setSchedules(prev => prev.filter(item => item.id !== schedule.id));
      } catch (err) {
        alert(err instanceof Error ? err.message : 'Unable to delete schedule');
      } finally {
        setBusyScheduleID(null);
      }
    },
    []
  );

  return (
    <div data-page="schedules" className="active min-h-screen flex flex-col overflow-x-hidden overflow-y-auto">
      <div className="px-6 py-6 space-y-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-2xl font-bold text-[var(--text-primary)]">Schedules</h1>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--text-secondary)]">
              {pipelineFilter ? (
                <span className="runner-pill runner-pill--link">
                  Pipeline {pipelineFilter}
                  <button type="button" onClick={clearPipelineFilter} aria-label="Clear pipeline filter">
                    <X className="h-3.5 w-3.5" />
                  </button>
                </span>
              ) : null}
              <span>{stats.total} total</span>
              <span>{stats.enabled} enabled</span>
              <span>{stats.gitops} GitOps</span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" className="glass-button-ghost" onClick={() => void loadSchedules()} title="Refresh schedules">
              <RefreshCw className="h-4 w-4" />
            </button>
            {canWriteSchedules ? (
              <button type="button" className="glass-button-primary" onClick={openCreate}>
                <Plus className="h-4 w-4" />
                <span>New schedule</span>
              </button>
            ) : null}
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-4">
          <ScheduleStatCard icon={<CalendarClock className="h-5 w-5" />} label="Schedules" value={stats.total} />
          <ScheduleStatCard icon={<CheckCircle2 className="h-5 w-5" />} label="Enabled" value={stats.enabled} />
          <ScheduleStatCard icon={<Clock3 className="h-5 w-5" />} label="Scheduled" value={stats.due} />
          <ScheduleStatCard icon={<GitBranch className="h-5 w-5" />} label="GitOps" value={stats.gitops} />
        </div>

        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
          <div className="pipelines-search-shell open min-w-[16rem] flex-1">
            <Search className="h-4 w-4 text-[var(--text-secondary)] ml-2" />
            <input
              value={searchTerm}
              onChange={event => setSearchTerm(event.target.value)}
              className="pipelines-search-input"
              placeholder="Search schedules"
            />
            {searchTerm ? (
              <button type="button" className="pipelines-search-clear" onClick={() => setSearchTerm('')} aria-label="Clear search">
                <X className="h-4 w-4" />
              </button>
            ) : null}
          </div>
          <select
            className="pipelines-input min-w-[12rem]"
            value={activeGroup}
            onChange={event => setFolderFilter(event.target.value)}
            aria-label="Filter by group"
          >
            <option value="all">All groups</option>
            {scheduleGroupOptions.map(group => (
              <option key={group} value={group}>
                {group === 'root' ? 'Root' : `/${group}`}
              </option>
            ))}
          </select>
          <label className="inline-flex items-center gap-2 text-sm text-[var(--text-secondary)]">
            <input
              type="checkbox"
              checked={showDisabled}
              onChange={event => setShowDisabled(event.target.checked)}
              className="h-4 w-4 rounded border-[var(--border-primary)]"
            />
            <span>Show disabled</span>
          </label>
        </div>

        {loading ? (
          <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5 text-sm text-[var(--text-secondary)]">
            Loading schedules…
          </div>
        ) : error ? (
          <div className="rounded-xl border border-red-300 bg-red-50 p-5 text-sm text-red-700 dark:border-red-800 dark:bg-red-950/40 dark:text-red-200">
            {error}
          </div>
        ) : filteredSchedules.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {filteredSchedules.map(schedule => (
              <ScheduleCard
                key={schedule.id}
                schedule={schedule}
                canWriteSchedules={canWriteSchedules}
                canDeleteSchedules={canDeleteSchedules}
                busy={busyScheduleID === schedule.id}
                onEdit={openEdit}
                onEnable={setScheduleEnabled}
                onRun={runNow}
                onDelete={deleteSchedule}
                onOpenRun={runID => navigate(`/pipelineruns/recent?run=${encodeURIComponent(runID)}`)}
              />
            ))}
          </div>
        ) : (
          <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-6 text-sm text-[var(--text-secondary)]">
            No schedules match the current filters.
          </div>
        )}
      </div>

      {modal ? (
        <ScheduleFormModal
          modal={modal}
          form={form}
          formError={formError}
          saving={saving}
          pipelines={pipelines}
          runGroups={runGroupOptions}
          scopes={scopeOptions}
          canSubmit={canWriteSchedules && !modal.schedule?.managed_by_config_repo}
          onChange={setForm}
          onClose={closeModal}
          onSubmit={() => void submitForm()}
        />
      ) : null}
    </div>
  );
}

function ScheduleStatCard({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
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

function ScheduleCard({
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
        <ScheduleFact label="Run group" value={formatGroupPath(effectiveScheduleRunGroupPath(schedule))} mono />
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

function ScheduleFormModal({
  modal,
  form,
  formError,
  saving,
  pipelines,
  runGroups,
  scopes,
  canSubmit,
  onChange,
  onClose,
  onSubmit,
}: {
  modal: ScheduleModalState;
  form: ScheduleFormState;
  formError: string | null;
  saving: boolean;
  pipelines: string[];
  runGroups: string[];
  scopes: string[];
  canSubmit: boolean;
  onChange: (form: ScheduleFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const disabled = saving || !canSubmit;
  const update = (patch: Partial<ScheduleFormState>) => onChange({ ...form, ...patch });
  const pipelineOptions = Array.from(new Set([...pipelines, form.pipeline].map(normalizeIdentifier).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b)
  );
  const groupOptions = uniqueRunGroupOptions([...runGroups, form.runGroupPath]);
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
  const dialogRef = useDialogFocus(onClose);
  const titleId = 'schedule-form-title';
  const errorId = 'schedule-form-error';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] p-4">
      <div
        ref={dialogRef}
        className="w-full max-w-3xl rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-2xl"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={formError ? errorId : undefined}
        tabIndex={-1}
      >
        <div className="flex items-center justify-between gap-3 border-b border-[var(--border-primary)] p-4">
          <div className="min-w-0">
            <h2 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">
              {modal.mode === 'edit' ? 'Edit schedule' : 'New schedule'}
            </h2>
            {modal.schedule?.managed_by_config_repo ? (
              <p className="mt-1 text-xs text-[var(--text-secondary)]">Managed by {modal.schedule.config_source_path || 'GitOps'}</p>
            ) : null}
          </div>
          <button type="button" className="glass-button-ghost" onClick={onClose} title="Close">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="max-h-[75vh] overflow-y-auto p-4">
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
	                    runGroupPath:
	                      form.runGroupPath && form.runGroupPath !== 'root'
	                        ? form.runGroupPath
	                        : defaultRunGroupForPipeline(pipeline, runGroups),
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
              <span className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Run group</span>
              <select
                className="pipelines-input w-full"
                value={form.runGroupPath}
                onChange={event => update({ runGroupPath: event.target.value })}
                disabled={disabled}
              >
                {groupOptions.map(group => (
                  <option key={group} value={group}>
                    {group === 'root' ? 'Root' : group}
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

          {formError ? <p id={errorId} className="mt-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{formError}</p> : null}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-primary)] p-4">
          <div className="flex items-center gap-2 text-xs text-[var(--text-secondary)]">
            <Shield className="h-4 w-4" />
            <span>{modal.schedule?.managed_by_config_repo ? 'Change this schedule in GitOps.' : 'Runtime access is checked before saving.'}</span>
          </div>
          <div className="flex items-center gap-2">
            <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
              Cancel
            </button>
            {canSubmit ? (
              <button type="button" className="glass-button-primary" onClick={onSubmit} disabled={saving}>
                {saving ? 'Saving…' : 'Save'}
              </button>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}
