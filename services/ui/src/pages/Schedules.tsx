import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { X } from 'lucide-react';

import { ResourceCollectionToolbar } from '../features/editor/ResourceCollectionToolbar';
import { ScheduleCard } from '../features/schedules/ScheduleCards';
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

  const [schedules, setSchedules] = useState<PipelineSchedule[]>([]);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [groups, setGroups] = useState<string[]>([]);
  const [scopes, setScopes] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
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
    const timeout = window.setTimeout(() => void loadSchedules(), 0);
    return () => window.clearTimeout(timeout);
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

  const runGroupOptions = useMemo(() => uniqueRunGroupOptions(groups), [groups]);

  const scopeOptions = useMemo(() => {
    const values = new Set(scopes.map(normalizeScopeOption));
    schedules.forEach(schedule => values.add(normalizeScopeOption(schedule.scope)));
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [schedules, scopes]);

  const filteredSchedules = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    return schedules.filter(schedule => {
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
  }, [schedules, searchTerm]);

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
    <div data-page="schedules" className="active h-full flex flex-col">
      <ResourceCollectionToolbar
        resourceLabel="schedule"
        searchTerm={searchTerm}
        canCreate={canWriteSchedules}
        createLabel="New schedule"
        createDisabledReason="You have read-only access to schedules"
        showCreateWhenDisabled
        onSearchTermChange={setSearchTerm}
        onCreate={openCreate}
        onRefresh={() => void loadSchedules()}
        refreshDisabled={loading || saving}
        summary={pipelineFilter ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="runner-pill runner-pill--link">
              Pipeline {pipelineFilter}
              <button type="button" onClick={clearPipelineFilter} aria-label="Clear pipeline filter">
                <X className="h-3.5 w-3.5" />
              </button>
            </span>
          </div>
        ) : undefined}
      />
      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {loading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading schedules...</div>
        ) : error ? (
          <div className="glass-card p-5 text-sm text-red-500">{error}</div>
        ) : filteredSchedules.length ? (
          <div className="compact-resource-grid" data-testid="schedule-card-list">
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
          <div className="pipelines-empty">
            <h2 className="text-base font-semibold text-[var(--text-primary)]">No schedules found</h2>
            <p className="text-sm text-[var(--text-secondary)]">
              Create a schedule or adjust the current filters.
            </p>
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
