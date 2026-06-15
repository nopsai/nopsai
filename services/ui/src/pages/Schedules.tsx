import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  CalendarClock,
  CheckCircle2,
  Clock3,
  GitBranch,
  Plus,
  RefreshCw,
  Search,
  X,
} from 'lucide-react';

import { ScheduleCard, ScheduleStatCard } from '../features/schedules/ScheduleCards';
import { ScheduleFormModal } from '../features/schedules/ScheduleFormModal';
import {
  deleteSchedule as deleteScheduleRequest,
  fetchScheduleMetadata,
  fetchSchedules,
  runSchedule,
  saveSchedule,
  setScheduleEnabled as setScheduleEnabledRequest,
} from '../features/schedules/api';
import {
  createEmptyForm,
  formFromSchedule,
  normalizeIdentifier,
  normalizeScopeOption,
  uniqueRunGroupOptions,
  type PipelineSchedule,
  type ScheduleFormState,
  type ScheduleModalState,
} from '../features/schedules/model';
import { sourceLabel } from '../features/schedules/presentation';

type SchedulesPageProps = {
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
};

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
            <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-secondary)]">
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
