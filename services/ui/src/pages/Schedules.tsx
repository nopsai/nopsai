import { useCallback, useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { ScheduleFormModal } from '../features/schedules/ScheduleFormModal';
import { ScheduleWorkspace } from '../features/schedules/ScheduleWorkspace';
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
  uniqueRunTeamOptions,
  type PipelineSchedule,
  type ScheduleFormState,
  type ScheduleModalState,
} from '../features/schedules/model';
import { isGitOpsSchedule, scheduleResourceID } from '../features/schedules/workspaceModel';

type SchedulesPageProps = {
  canWriteSchedules: boolean;
  canDeleteSchedules: boolean;
};

function decodeScheduleRouteSegment(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

function scheduleIDFromRoute(pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'schedules' || segments.length < 2) return '';
  return normalizeIdentifier(segments.slice(1).map(decodeScheduleRouteSegment).join('/'));
}

function encodeScheduleRouteID(scheduleID: string) {
  return normalizeIdentifier(scheduleID).split('/').filter(Boolean).map(encodeURIComponent).join('/');
}

function buildSchedulesRoute(scheduleID: string, searchParams?: URLSearchParams) {
  const params = new URLSearchParams(searchParams);
  params.delete('schedule');
  const encodedScheduleID = encodeScheduleRouteID(scheduleID);
  const route = encodedScheduleID ? `/schedules/${encodedScheduleID}` : '/schedules';
  const query = params.toString();
  return query ? `${route}?${query}` : route;
}

function pipelineRunRoute(runID: string) {
  return `/pipelineruns/recent/${encodeURIComponent(runID)}`;
}

export default function SchedulesPage({ canWriteSchedules, canDeleteSchedules }: SchedulesPageProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search]);
  const pipelineFilter = normalizeIdentifier(searchParams.get('pipeline') || '');
  const schedulePathParam = useMemo(() => scheduleIDFromRoute(location.pathname), [location.pathname]);
  const legacyScheduleParam = searchParams.get('schedule') || '';
  const scheduleRouteParam = schedulePathParam || legacyScheduleParam;

  const [schedules, setSchedules] = useState<PipelineSchedule[]>([]);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [teams, setTeams] = useState<string[]>([]);
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
    if (schedulePathParam || !legacyScheduleParam) return;
    const params = new URLSearchParams(location.search);
    params.delete('schedule');
    navigate(buildSchedulesRoute(legacyScheduleParam, params), { replace: true, preventScrollReset: true });
  }, [legacyScheduleParam, location.search, navigate, schedulePathParam]);

  useEffect(() => {
    let cancelled = false;
    const loadMeta = async () => {
      try {
        const metadata = await fetchScheduleMetadata();
        if (cancelled) return;
        setPipelines(metadata.pipelines);
        setTeams(metadata.teams);
        setScopes(metadata.scopes);
      } catch {
        if (!cancelled) {
          setPipelines([]);
          setTeams([]);
          setScopes(['']);
        }
      }
    };
    void loadMeta();
    return () => {
      cancelled = true;
    };
  }, []);

  const runTeamOptions = useMemo(() => uniqueRunTeamOptions(teams), [teams]);

  const scopeOptions = useMemo(() => {
    const values = new Set(scopes.map(normalizeScopeOption));
    schedules.forEach(schedule => values.add(normalizeScopeOption(schedule.scope)));
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [schedules, scopes]);

  const selectedScheduleID = useMemo(() => {
    const normalizedParam = normalizeIdentifier(scheduleRouteParam);
    if (!normalizedParam) return '';
    const matched = schedules.find(schedule => {
      const resourceID = scheduleResourceID(schedule);
      return (
        resourceID === normalizedParam ||
        normalizeIdentifier(schedule.identifier) === normalizedParam ||
        normalizeIdentifier(schedule.id) === normalizedParam ||
        normalizeIdentifier(schedule.name) === normalizedParam
      );
    });
    return matched ? scheduleResourceID(matched) : normalizedParam;
  }, [scheduleRouteParam, schedules]);

  const clearPipelineFilter = useCallback(() => {
    const params = new URLSearchParams(location.search);
    params.delete('pipeline');
    navigate(buildSchedulesRoute(selectedScheduleID, params), { replace: true, preventScrollReset: true });
  }, [location.search, navigate, selectedScheduleID]);

  const setSelectedScheduleID = useCallback((scheduleID: string) => {
    const params = new URLSearchParams(location.search);
    navigate(buildSchedulesRoute(scheduleID, params), { preventScrollReset: true });
  }, [location.search, navigate]);

  const openCreate = useCallback(() => {
    setForm(createEmptyForm(pipelineFilter, runTeamOptions));
    setFormError(null);
    setModal({ mode: 'create' });
  }, [pipelineFilter, runTeamOptions]);

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
      if (isGitOpsSchedule(schedule) && !window.confirm(
        `This schedule is managed by GitOps. ${enabled ? 'Enabling' : 'Disabling'} it saves a database override that the next GitOps sync can replace unless it is pushed to GitOps. Continue?`
      )) {
        return;
      }
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
          navigate(pipelineRunRoute(result.run_id));
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
      const message = isGitOpsSchedule(schedule)
        ? `Delete schedule ${schedule.identifier}? This removes the database row; the next GitOps sync can recreate it from the repository.`
        : `Delete schedule ${schedule.identifier}?`;
      if (!window.confirm(message)) return;
      setBusyScheduleID(schedule.id);
      try {
        await deleteScheduleRequest(schedule.id);
        setSchedules(prev => prev.filter(item => item.id !== schedule.id));
        if (selectedScheduleID === scheduleResourceID(schedule)) {
          setSelectedScheduleID('');
        }
      } catch (err) {
        alert(err instanceof Error ? err.message : 'Unable to delete schedule');
      } finally {
        setBusyScheduleID(null);
      }
    },
    [selectedScheduleID, setSelectedScheduleID]
  );

  return (
    <div data-page="schedules" className="active h-full flex flex-col">
      <ScheduleWorkspace
        schedules={schedules}
        teams={teams}
        loading={loading}
        error={error}
        saving={saving}
        busyScheduleID={busyScheduleID}
        searchTerm={searchTerm}
        pipelineFilter={pipelineFilter}
        selectedScheduleID={selectedScheduleID}
        canWriteSchedules={canWriteSchedules}
        canDeleteSchedules={canDeleteSchedules}
        onSearchTermChange={setSearchTerm}
        onClearPipelineFilter={clearPipelineFilter}
        onSelectedScheduleIDChange={setSelectedScheduleID}
        onCreate={openCreate}
        onEdit={openEdit}
        onEnable={setScheduleEnabled}
        onRun={runNow}
        onDelete={deleteSchedule}
        onOpenRun={runID => navigate(pipelineRunRoute(runID))}
      />

      {modal ? (
        <ScheduleFormModal
          modal={modal}
          form={form}
          formError={formError}
          saving={saving}
          pipelines={pipelines}
          runTeams={runTeamOptions}
          scopes={scopeOptions}
          canSubmit={canWriteSchedules}
          onChange={setForm}
          onClose={closeModal}
          onSubmit={() => void submitForm()}
        />
      ) : null}
    </div>
  );
}
