import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

import {
  cancelDashboardRefresh,
  deleteDashboard,
  deleteDashboardPublication,
  deleteDashboardRefreshSchedule,
  deleteDashboardSection,
  deleteDashboardSource,
  fetchDashboardPipelineCatalog,
  fetchDashboardPipelineOutputs,
  fetchDashboardHistory,
  fetchDashboardMetadata,
  fetchDashboardRefreshes,
  fetchDashboardRefreshSchedules,
  fetchDashboards,
  fetchDashboardView,
  retryDashboardRefreshFailed,
  runDashboardRefreshSchedule,
  saveDashboard,
  saveDashboardRefreshSchedule,
  saveDashboardSection,
  saveDashboardSource,
  setDashboardRefreshScheduleEnabled,
  startDashboardRefresh,
} from '../features/dashboards/api';
import {
  DashboardDeleteModal,
  DashboardModal,
  RefreshModal,
  RefreshScheduleModal,
  SectionModal,
  SourceModal,
  type DashboardDeleteModalState,
  type DashboardModalState,
  type RefreshScheduleModalState,
  type SectionModalState,
  type SourceModalState,
} from '../features/dashboards/DashboardModals';
import { DashboardWorkspace } from '../features/dashboards/DashboardWorkspace';
import {
  createDashboardForm,
  createRefreshForm,
  createRefreshScheduleForm,
  createSectionForm,
  createSourceForm,
  formFromDashboard,
  refreshScheduleFormFromSchedule,
  sectionFormFromSection,
  sourceFormFromSource,
  type DashboardEvent,
  type DashboardFormState,
  type DashboardPublication,
  type DashboardRefresh,
  type DashboardRefreshSchedule,
  type DashboardRefreshScheduleFormState,
  type DashboardRefreshFormState,
  type DashboardSection,
  type DashboardSectionSeed,
  type DashboardSectionFormState,
  type DashboardSource,
  type DashboardSourceFormState,
  type DashboardSummary,
  type DashboardView,
} from '../features/dashboards/model';
import {
  dashboardOutputBindingsFromForm,
  sectionFormFromSeed,
  sectionSeedsFromBindings,
  sourceBindingExists,
  sourceFormFromBinding,
  unselectedDashboardOutputSources,
  type DashboardOutputBinding,
} from '../features/dashboards/pipelineAssignments';
import {
  DASHBOARD_ROUTE_DASHBOARD_PARAM,
  DASHBOARD_ROUTE_TAB_PARAM,
  dashboardTabHref,
  dashboardTabSearchParams,
  normalizeDashboardRouteValue,
} from '../features/dashboards/routes';
import type { DashboardPipelineCatalogItem } from '../features/dashboards/sourceOptions';

type DashboardsPageProps = {
  canWriteDashboards: boolean;
  canDeleteDashboards: boolean;
};

type RefreshModalState = {
  label: string;
};

export default function DashboardsPage({ canWriteDashboards, canDeleteDashboards }: DashboardsPageProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const routeDashboardID = normalizeDashboardRouteValue(searchParams.get(DASHBOARD_ROUTE_DASHBOARD_PARAM));
  const routeSectionKey = normalizeDashboardRouteValue(searchParams.get(DASHBOARD_ROUTE_TAB_PARAM));

  const [dashboards, setDashboards] = useState<DashboardSummary[]>([]);
  const [teams, setTeams] = useState<string[]>([]);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [scopes, setScopes] = useState<string[]>(['']);
  const [dashboardPipelineCatalog, setDashboardPipelineCatalog] = useState<DashboardPipelineCatalogItem[]>([]);
  const [pipelineLoading, setPipelineLoading] = useState(true);
  const [selectedID, setSelectedID] = useState(routeDashboardID);
  const [view, setView] = useState<DashboardView | null>(null);
  const [history, setHistory] = useState<DashboardEvent[]>([]);
  const [refreshes, setRefreshes] = useState<DashboardRefresh[]>([]);
  const [refreshSchedules, setRefreshSchedules] = useState<DashboardRefreshSchedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [teamFilter, setTeamFilter] = useState('');
  const [dashboardModal, setDashboardModal] = useState<DashboardModalState | null>(null);
  const [dashboardForm, setDashboardForm] = useState<DashboardFormState>(() => createDashboardForm());
  const [sectionModal, setSectionModal] = useState<SectionModalState | null>(null);
  const [sectionForm, setSectionForm] = useState<DashboardSectionFormState>(() => createSectionForm());
  const [sourceModal, setSourceModal] = useState<SourceModalState | null>(null);
  const [sourceForm, setSourceForm] = useState<DashboardSourceFormState>(() => createSourceForm());
  const [deleteModal, setDeleteModal] = useState<DashboardDeleteModalState | null>(null);
  const [refreshModal, setRefreshModal] = useState<RefreshModalState | null>(null);
  const [refreshForm, setRefreshForm] = useState<DashboardRefreshFormState>(() => createRefreshForm(''));
  const [refreshScheduleModal, setRefreshScheduleModal] = useState<RefreshScheduleModalState | null>(null);
  const [refreshScheduleForm, setRefreshScheduleForm] = useState<DashboardRefreshScheduleFormState>(() => createRefreshScheduleForm());
  const [formError, setFormError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const loadDashboards = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const payload = await fetchDashboards({ team: teamFilter, query: searchTerm });
      setDashboards(payload);
      setSelectedID(current => {
        if (current && payload.some(item => item.id === current)) return current;
        return payload[0]?.id || '';
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load dashboards');
      setDashboards([]);
      setSelectedID('');
    } finally {
      setLoading(false);
    }
  }, [searchTerm, teamFilter]);

  useEffect(() => {
    if (!routeDashboardID || routeDashboardID === selectedID) return;
    setSelectedID(routeDashboardID);
  }, [routeDashboardID, selectedID]);

  const loadSelected = useCallback(async () => {
    if (!selectedID) {
      setView(null);
      setHistory([]);
      setRefreshes([]);
      setRefreshSchedules([]);
      return;
    }
    setDetailLoading(true);
    try {
      const [nextView, nextHistory, nextRefreshes, nextSchedules] = await Promise.all([
        fetchDashboardView(selectedID),
        fetchDashboardHistory(selectedID).catch(() => []),
        fetchDashboardRefreshes(selectedID).catch(() => []),
        fetchDashboardRefreshSchedules(selectedID).catch(() => []),
      ]);
      setView(nextView);
      setHistory(nextHistory);
      setRefreshes(nextRefreshes);
      setRefreshSchedules(nextSchedules);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load dashboard');
      setView(null);
    } finally {
      setDetailLoading(false);
    }
  }, [selectedID]);

  useEffect(() => {
    const timeout = window.setTimeout(() => void loadDashboards(), 180);
    return () => window.clearTimeout(timeout);
  }, [loadDashboards]);

  useEffect(() => {
    void loadSelected();
  }, [loadSelected]);

  useEffect(() => {
    let cancelled = false;
    setPipelineLoading(true);
    void fetchDashboardMetadata()
      .then(async metadata => {
        const catalog = await fetchDashboardPipelineCatalog(metadata.pipelines);
        if (cancelled) return;
        setTeams(metadata.teams);
        setScopes(metadata.scopes);
        setDashboardPipelineCatalog(catalog);
        setPipelines(catalog.map(pipeline => pipeline.id));
      })
      .catch(() => {
        if (!cancelled) {
          setTeams([]);
          setScopes(['']);
          setDashboardPipelineCatalog([]);
          setPipelines([]);
        }
      })
      .finally(() => {
        if (!cancelled) setPipelineLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!selectedID || !refreshes.some(refresh => refresh.status === 'running')) return undefined;
    const interval = window.setInterval(() => {
      void fetchDashboardRefreshes(selectedID).then(setRefreshes).catch(() => undefined);
    }, 3500);
    return () => window.clearInterval(interval);
  }, [refreshes, selectedID]);

  const selectedDashboard = useMemo(
    () => dashboards.find(item => item.id === selectedID) || view?.dashboard || null,
    [dashboards, selectedID, view]
  );
  const activeSectionKey = useMemo(() => {
    const sections = view?.sections || [];
    if (routeSectionKey && sections.some(section => section.section_key === routeSectionKey)) return routeSectionKey;
    return sections[0]?.section_key || '';
  }, [routeSectionKey, view?.sections]);

  useEffect(() => {
    if (!selectedID) return;
    const nextParams = dashboardTabSearchParams(searchParams, selectedID, activeSectionKey);
    if (nextParams.toString() === searchParams.toString()) return;
    setSearchParams(nextParams, { replace: true });
  }, [activeSectionKey, searchParams, selectedID, setSearchParams]);

  const selectDashboard = useCallback((dashboardID: string) => {
    const normalizedDashboardID = normalizeDashboardRouteValue(dashboardID);
    setSelectedID(normalizedDashboardID);
    setSearchParams(dashboardTabSearchParams(searchParams, normalizedDashboardID, ''));
  }, [searchParams, setSearchParams]);

  const selectDashboardSection = useCallback((sectionKey: string) => {
    setSearchParams(dashboardTabSearchParams(searchParams, selectedID, sectionKey));
  }, [searchParams, selectedID, setSearchParams]);

  const dashboardSectionHref = useCallback(
    (sectionKey: string) => dashboardTabHref(searchParams, selectedID, sectionKey),
    [searchParams, selectedID]
  );

  const openCreateDashboard = useCallback(() => {
    setDashboardForm(createDashboardForm(teamFilter || teams[0] || ''));
    setFormError(null);
    setDashboardModal({ mode: 'create' });
  }, [teamFilter, teams]);

  const openEditDashboard = useCallback((dashboard: DashboardSummary) => {
    const dashboardPipelineIDs = new Set(dashboardPipelineCatalog.map(pipeline => pipeline.id));
    const attachedPipelineIDs = Array.from(new Set((view?.sources || []).map(source => source.pipeline_id)))
      .filter(id => dashboardPipelineIDs.has(id))
      .sort((a, b) => a.localeCompare(b));
    const pipelineScopes = Object.fromEntries(
      attachedPipelineIDs.map(pipelineID => [
        pipelineID,
        (view?.sources || []).find(source => source.pipeline_id === pipelineID)?.run_scope || '',
      ])
    );
    setDashboardForm({ ...formFromDashboard(dashboard), pipelineIDs: attachedPipelineIDs, pipelineScopes });
    setFormError(null);
    setDashboardModal({ mode: 'edit', dashboard });
  }, [dashboardPipelineCatalog, view?.sources]);

  const submitDashboard = useCallback(async () => {
    if (!dashboardModal || saving) return;
    const outputBindings = dashboardOutputBindingsFromForm(dashboardForm, dashboardPipelineCatalog);
    if (dashboardModal.mode === 'create' && outputBindings.length === 0) {
      setFormError('Select at least one pipeline with dashboard outputs targeting this dashboard.');
      return;
    }
    const sectionSeeds = sectionSeedsFromBindings(outputBindings);
    setSaving(true);
    setFormError(null);
    try {
      const updated = await saveDashboard(
        dashboardForm,
        dashboardModal.mode === 'edit' ? dashboardModal.dashboard : undefined,
        dashboardModal.mode === 'create' ? { sections: sectionSeeds } : {}
      );
      if (dashboardModal.mode === 'edit') {
        await createMissingSections(updated.id, sectionSeeds, view?.sections || []);
      }
      const existingSources = dashboardModal.mode === 'edit' ? view?.sources || [] : [];
      const removedSources = dashboardModal.mode === 'edit'
        ? unselectedDashboardOutputSources(dashboardForm, dashboardPipelineCatalog, existingSources)
        : [];
      await deleteDashboardOutputSources(updated.id, removedSources);
      const removedSourceIDs = new Set(removedSources.map(source => source.id));
      const remainingSources = existingSources.filter(source => !removedSourceIDs.has(source.id));
      await saveDashboardOutputBindings(updated.id, outputBindings.filter(binding => !sourceBindingExists(remainingSources, binding)));
      await loadDashboards();
      if (selectedID === updated.id) await loadSelected();
      selectDashboard(updated.id);
      setDashboardModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save dashboard');
    } finally {
      setSaving(false);
    }
  }, [dashboardForm, dashboardModal, dashboardPipelineCatalog, loadDashboards, loadSelected, saving, selectDashboard, selectedID, view?.sections, view?.sources]);

  const openDeleteDashboard = useCallback((dashboard: DashboardSummary) => {
    setFormError(null);
    setDeleteModal({ kind: 'dashboard', dashboard });
  }, []);

  const openDeleteSection = useCallback((section: DashboardSection) => {
    setFormError(null);
    setDeleteModal({ kind: 'section', section });
  }, []);

  const openDeleteSource = useCallback((source: DashboardSource) => {
    setFormError(null);
    setDeleteModal({ kind: 'source', source });
  }, []);

  const openDeletePublication = useCallback((publication: DashboardPublication) => {
    setFormError(null);
    setDeleteModal({ kind: 'publication', publication });
  }, []);

  const openDeleteRefreshSchedule = useCallback((schedule: DashboardRefreshSchedule) => {
    setFormError(null);
    setDeleteModal({ kind: 'schedule', schedule });
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!deleteModal || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      if (deleteModal.kind === 'dashboard') {
        await deleteDashboard(deleteModal.dashboard.id);
        setDashboardModal(null);
        setSectionModal(null);
        setSourceModal(null);
        setRefreshScheduleModal(null);
        await loadDashboards();
      } else if (deleteModal.kind === 'section') {
        if (!selectedID) throw new Error('Select a dashboard before deleting a section');
        await deleteDashboardSection(selectedID, deleteModal.section.id);
        await loadSelected();
      } else if (deleteModal.kind === 'source') {
        if (!selectedID) throw new Error('Select a dashboard before deleting a source');
        await deleteDashboardSource(selectedID, deleteModal.source.id);
        await loadSelected();
      } else if (deleteModal.kind === 'publication') {
        if (!selectedID) throw new Error('Select a dashboard before deleting an entry');
        await deleteDashboardPublication(selectedID, deleteModal.publication.id);
        await loadSelected();
      } else {
        if (!selectedID) throw new Error('Select a dashboard before deleting a schedule');
        await deleteDashboardRefreshSchedule(selectedID, deleteModal.schedule.id);
        await loadSelected();
      }
      setDeleteModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to delete');
    } finally {
      setSaving(false);
    }
  }, [deleteModal, loadDashboards, loadSelected, saving, selectedID]);

  const openEditSection = useCallback((section: DashboardSection) => {
    setSectionForm(sectionFormFromSection(section));
    setFormError(null);
    setSectionModal({ mode: 'edit', section });
  }, []);

  const submitSection = useCallback(async () => {
    if (!sectionModal || !selectedID || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      await saveDashboardSection(selectedID, sectionForm, sectionModal.mode === 'edit' ? sectionModal.section : undefined);
      await loadSelected();
      setSectionModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save section');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, saving, sectionForm, sectionModal, selectedID]);

  const openEditSource = useCallback((source: DashboardSource) => {
    setSourceForm(sourceFormFromSource(source));
    setFormError(null);
    setSourceModal({ mode: 'edit', source });
  }, []);

  const submitSource = useCallback(async () => {
    if (!sourceModal || !selectedID || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      await saveDashboardSource(selectedID, sourceForm, sourceModal.mode === 'edit' ? sourceModal.source : undefined);
      await loadSelected();
      setSourceModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save source');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, saving, selectedID, sourceForm, sourceModal]);

  const openRefreshDashboard = useCallback(() => {
    setRefreshForm(createRefreshForm(''));
    setFormError(null);
    setRefreshModal({ label: 'Refresh dashboard' });
  }, []);

  const openCreateRefreshSchedule = useCallback((
    scope: { scopeType?: DashboardRefreshScheduleFormState['scopeType']; sectionKey?: string } = {}
  ) => {
    setRefreshScheduleForm(createRefreshScheduleForm(scope));
    setFormError(null);
    setRefreshScheduleModal({ mode: 'create' });
  }, []);

  const openEditRefreshSchedule = useCallback((schedule: DashboardRefreshSchedule) => {
    setRefreshScheduleForm(refreshScheduleFormFromSchedule(schedule));
    setFormError(null);
    setRefreshScheduleModal({ mode: 'edit', schedule });
  }, []);

  const submitRefreshSchedule = useCallback(async () => {
    if (!selectedID || !refreshScheduleModal || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      await saveDashboardRefreshSchedule(
        selectedID,
        refreshScheduleForm,
        refreshScheduleModal.mode === 'edit' ? refreshScheduleModal.schedule : undefined
      );
      await loadSelected();
      setRefreshScheduleModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save refresh schedule');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, refreshScheduleForm, refreshScheduleModal, saving, selectedID]);

  const toggleRefreshSchedule = useCallback(async (schedule: DashboardRefreshSchedule, enabled: boolean) => {
    if (!selectedID || saving) return;
    setSaving(true);
    try {
      const updated = await setDashboardRefreshScheduleEnabled(selectedID, schedule.id, enabled);
      setRefreshSchedules(current => current.map(item => item.id === updated.id ? updated : item));
      await loadSelected();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unable to update refresh schedule');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, saving, selectedID]);

  const submitRefresh = useCallback(async () => {
    if (!selectedID || !refreshModal || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      const refresh = await startDashboardRefresh(selectedID, refreshForm);
      setRefreshes(current => [refresh, ...current.filter(item => item.id !== refresh.id)]);
      setRefreshModal(null);
      await loadSelected();
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to start refresh');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, refreshForm, refreshModal, saving, selectedID]);

  const cancelRefresh = useCallback(async (refresh: DashboardRefresh) => {
    if (!selectedID) return;
    setSaving(true);
    try {
      const updated = await cancelDashboardRefresh(selectedID, refresh.id);
      setRefreshes(current => [updated, ...current.filter(item => item.id !== updated.id)]);
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unable to cancel refresh');
    } finally {
      setSaving(false);
    }
  }, [selectedID]);

  const retryFailed = useCallback(async (refresh: DashboardRefresh) => {
    if (!selectedID) return;
    setSaving(true);
    try {
      const updated = await retryDashboardRefreshFailed(selectedID, refresh.id);
      setRefreshes(current => [updated, ...current.filter(item => item.id !== updated.id)]);
      await loadSelected();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unable to retry failed sources');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, selectedID]);

  const runSchedule = useCallback(async (schedule: DashboardRefreshSchedule) => {
    if (!selectedID || !window.confirm(`Run scheduled refresh ${schedule.name}?`)) return;
    setSaving(true);
    try {
      const refresh = await runDashboardRefreshSchedule(selectedID, schedule.id);
      setRefreshes(current => [refresh, ...current.filter(item => item.id !== refresh.id)]);
      await loadSelected();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unable to run schedule');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, selectedID]);

  return (
    <>
      <DashboardWorkspace
        dashboards={dashboards}
        teams={teams}
        selectedID={selectedID}
        activeSectionKey={activeSectionKey}
        selectedDashboard={selectedDashboard}
        view={view}
        history={history}
        refreshes={refreshes}
        refreshSchedules={refreshSchedules}
        loading={loading}
        detailLoading={detailLoading}
        error={error}
        searchTerm={searchTerm}
        teamFilter={teamFilter}
        saving={saving}
        canWriteDashboards={canWriteDashboards}
        canDeleteDashboards={canDeleteDashboards}
        onSearchTermChange={setSearchTerm}
        onTeamFilterChange={setTeamFilter}
        onSelectDashboard={selectDashboard}
        onSelectSection={selectDashboardSection}
        sectionTabHref={dashboardSectionHref}
        onReloadDashboards={() => void loadDashboards()}
        onCreateDashboard={openCreateDashboard}
        onEditDashboard={openEditDashboard}
        onDeleteDashboard={openDeleteDashboard}
        onRefreshDashboard={openRefreshDashboard}
        onScheduleDashboard={() => openCreateRefreshSchedule()}
        onEditSource={openEditSource}
        onDeleteSource={openDeleteSource}
        onDeletePublication={openDeletePublication}
        onCancelRefresh={refresh => void cancelRefresh(refresh)}
        onRetryRefresh={refresh => void retryFailed(refresh)}
        onCreateSchedule={scope => openCreateRefreshSchedule(scope)}
        onEditSchedule={openEditRefreshSchedule}
        onDeleteSchedule={openDeleteRefreshSchedule}
        onToggleSchedule={(schedule, enabled) => void toggleRefreshSchedule(schedule, enabled)}
        onRunSchedule={schedule => void runSchedule(schedule)}
      />

      {dashboardModal ? (
        <DashboardModal
          modal={dashboardModal}
          form={dashboardForm}
          teams={teams}
          sections={view?.sections || []}
          pipelineOptions={dashboardPipelineCatalog}
          scopeOptions={scopes}
          pipelineLoading={pipelineLoading}
          saving={saving}
          error={formError}
          onChange={setDashboardForm}
          onEditSection={openEditSection}
          onDeleteSection={openDeleteSection}
          onClose={() => setDashboardModal(null)}
          onSubmit={() => void submitDashboard()}
        />
      ) : null}

      {deleteModal ? (
        <DashboardDeleteModal
          modal={deleteModal}
          saving={saving}
          error={formError}
          onClose={() => {
            setDeleteModal(null);
            setFormError(null);
          }}
          onConfirm={() => void confirmDelete()}
        />
      ) : null}

      {sectionModal ? (
        <SectionModal
          modal={sectionModal}
          form={sectionForm}
          saving={saving}
          error={formError}
          onChange={setSectionForm}
          onClose={() => setSectionModal(null)}
          onSubmit={() => void submitSection()}
        />
      ) : null}

      {sourceModal ? (
        <SourceModal
          modal={sourceModal}
          form={sourceForm}
          dashboardRef={selectedDashboard?.ref || view?.dashboard.ref || ''}
          sections={view?.sections || []}
          sources={view?.sources || []}
          publications={view?.publications || []}
          pipelines={pipelines}
          scopeOptions={scopes}
          saving={saving}
          error={formError}
          loadPipelineOutputs={fetchDashboardPipelineOutputs}
          onChange={setSourceForm}
          onClose={() => setSourceModal(null)}
          onSubmit={() => void submitSource()}
        />
      ) : null}

      {refreshModal ? (
        <RefreshModal
          title={refreshModal.label}
          form={refreshForm}
          sections={view?.sections || []}
          saving={saving}
          error={formError}
          onChange={setRefreshForm}
          onClose={() => setRefreshModal(null)}
          onSubmit={() => void submitRefresh()}
        />
      ) : null}

      {refreshScheduleModal ? (
        <RefreshScheduleModal
          modal={refreshScheduleModal}
          form={refreshScheduleForm}
          sections={view?.sections || []}
          saving={saving}
          error={formError}
          onChange={setRefreshScheduleForm}
          onClose={() => setRefreshScheduleModal(null)}
          onSubmit={() => void submitRefreshSchedule()}
        />
      ) : null}
    </>
  );
}

async function createMissingSections(
  dashboardID: string,
  sectionSeeds: DashboardSectionSeed[],
  existingSections: DashboardSection[]
) {
  const existingKeys = new Set(existingSections.map(section => section.section_key));
  for (const section of sectionSeeds) {
    if (existingKeys.has(section.sectionKey)) continue;
    await saveDashboardSection(dashboardID, sectionFormFromSeed(section));
    existingKeys.add(section.sectionKey);
  }
}

async function saveDashboardOutputBindings(dashboardID: string, bindings: DashboardOutputBinding[]) {
  for (const [index, binding] of bindings.entries()) {
    await saveDashboardSource(dashboardID, sourceFormFromBinding(binding, index));
  }
}

async function deleteDashboardOutputSources(dashboardID: string, sources: DashboardSource[]) {
  for (const source of sources) {
    await deleteDashboardSource(dashboardID, source.id);
  }
}
