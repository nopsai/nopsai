import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { AlertTriangle, Clock3, Pencil, Plus, RefreshCw, RotateCcw, Save, Trash2, X } from 'lucide-react';

import {
  cancelDashboardRefresh,
  deleteDashboard,
  deleteDashboardSource,
  fetchDashboardHistory,
  fetchDashboardMetadata,
  fetchDashboardRefreshes,
  fetchDashboardRefreshSchedules,
  fetchDashboards,
  fetchDashboardView,
  retryDashboardRefreshFailed,
  runDashboardRefreshSchedule,
  saveDashboard,
  saveDashboardSource,
  startDashboardRefresh,
} from '../features/dashboards/api';
import { DashboardBlocks } from '../features/dashboards/blocks/DashboardBlocks';
import {
  createDashboardForm,
  createRefreshForm,
  createSourceForm,
  formFromDashboard,
  formatDateTime,
  groupPublicationsBySection,
  refreshProgress,
  refreshStatusLabel,
  sourceFormFromSource,
  staleLabel,
  type DashboardEvent,
  type DashboardFormState,
  type DashboardRefresh,
  type DashboardRefreshSchedule,
  type DashboardRefreshFormState,
  type DashboardSource,
  type DashboardSourceFormState,
  type DashboardSummary,
  type DashboardView,
} from '../features/dashboards/model';

type DashboardsPageProps = {
  canWriteDashboards: boolean;
  canDeleteDashboards: boolean;
};

type DashboardModalState =
  | { mode: 'create' }
  | { mode: 'edit'; dashboard: DashboardSummary };

type SourceModalState =
  | { mode: 'create'; sectionKey: string }
  | { mode: 'edit'; source: DashboardSource };

type RefreshModalState = {
  label: string;
};

export default function DashboardsPage({ canWriteDashboards, canDeleteDashboards }: DashboardsPageProps) {
  const [dashboards, setDashboards] = useState<DashboardSummary[]>([]);
  const [teams, setTeams] = useState<string[]>([]);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [selectedID, setSelectedID] = useState('');
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
  const [sourceModal, setSourceModal] = useState<SourceModalState | null>(null);
  const [sourceForm, setSourceForm] = useState<DashboardSourceFormState>(() => createSourceForm());
  const [refreshModal, setRefreshModal] = useState<RefreshModalState | null>(null);
  const [refreshForm, setRefreshForm] = useState<DashboardRefreshFormState>(() => createRefreshForm(''));
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
    void fetchDashboardMetadata().then(metadata => {
      if (cancelled) return;
      setTeams(metadata.teams);
      setPipelines(metadata.pipelines);
    }).catch(() => {
      if (!cancelled) {
        setTeams([]);
        setPipelines([]);
      }
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

  const publicationsBySection = useMemo(
    () => groupPublicationsBySection(view?.publications || []),
    [view?.publications]
  );

  const latestRefresh = refreshes[0] || null;
  const activeRefresh = refreshes.find(refresh => refresh.status === 'running') || null;
  const latestRefreshSourceByID = useMemo(() => {
    const map = new Map<string, NonNullable<DashboardRefresh['sources']>[number]>();
    for (const source of latestRefresh?.sources || []) {
      if (source.source_binding_id) map.set(source.source_binding_id, source);
    }
    return map;
  }, [latestRefresh]);

  const openCreateDashboard = useCallback(() => {
    setDashboardForm(createDashboardForm(teamFilter || teams[0] || ''));
    setFormError(null);
    setDashboardModal({ mode: 'create' });
  }, [teamFilter, teams]);

  const openEditDashboard = useCallback((dashboard: DashboardSummary) => {
    setDashboardForm(formFromDashboard(dashboard));
    setFormError(null);
    setDashboardModal({ mode: 'edit', dashboard });
  }, []);

  const submitDashboard = useCallback(async () => {
    if (!dashboardModal || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      const updated = await saveDashboard(dashboardForm, dashboardModal.mode === 'edit' ? dashboardModal.dashboard : undefined);
      await loadDashboards();
      setSelectedID(updated.id);
      setDashboardModal(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save dashboard');
    } finally {
      setSaving(false);
    }
  }, [dashboardForm, dashboardModal, loadDashboards, saving]);

  const removeDashboard = useCallback(async (dashboard: DashboardSummary) => {
    if (!window.confirm(`Delete dashboard ${dashboard.ref || dashboard.title}?`)) return;
    setSaving(true);
    try {
      await deleteDashboard(dashboard.id);
      await loadDashboards();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unable to delete dashboard');
    } finally {
      setSaving(false);
    }
  }, [loadDashboards]);

  const openCreateSource = useCallback((sectionKey: string) => {
    setSourceForm(createSourceForm(sectionKey));
    setFormError(null);
    setSourceModal({ mode: 'create', sectionKey });
  }, []);

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

  const removeSource = useCallback(async (source: DashboardSource) => {
    if (!selectedID || !window.confirm(`Delete source ${source.pipeline_id}/${source.output_name}?`)) return;
    setSaving(true);
    try {
      await deleteDashboardSource(selectedID, source.id);
      await loadSelected();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Unable to delete source');
    } finally {
      setSaving(false);
    }
  }, [loadSelected, selectedID]);

  const openRefreshDashboard = useCallback(() => {
    setRefreshForm(createRefreshForm(''));
    setFormError(null);
    setRefreshModal({ label: 'Refresh dashboard' });
  }, []);

  const openRefreshSection = useCallback((sectionKey: string) => {
    setRefreshForm(createRefreshForm(sectionKey));
    setFormError(null);
    setRefreshModal({ label: `Refresh ${sectionKey}` });
  }, []);

  const openRefreshSource = useCallback((source: DashboardSource) => {
    setRefreshForm(createRefreshForm(source.section_key, source.id));
    setFormError(null);
    setRefreshModal({ label: `Refresh ${source.pipeline_id}` });
  }, []);

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
    <div data-page="dashboards" className="active flex h-full flex-col">
      <div className="border-b border-[var(--border-subtle)] px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <input
            className="min-h-10 w-64 rounded border border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 text-sm"
            value={searchTerm}
            onChange={event => setSearchTerm(event.target.value)}
            placeholder="Search dashboards"
          />
          <select
            className="min-h-10 rounded border border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 text-sm"
            value={teamFilter}
            onChange={event => setTeamFilter(event.target.value)}
          >
            <option value="">All teams</option>
            {teams.map(team => <option key={team} value={team}>{team}</option>)}
          </select>
          <button className="inline-flex min-h-10 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={() => void loadDashboards()} disabled={loading}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            Refresh
          </button>
          {canWriteDashboards ? (
            <button className="inline-flex min-h-10 items-center gap-2 rounded bg-[var(--accent)] px-3 text-sm font-medium text-white" onClick={openCreateDashboard}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New dashboard
            </button>
          ) : null}
        </div>
        {error ? <div className="mt-2 text-sm text-rose-600">{error}</div> : null}
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="min-h-0 overflow-auto border-r border-[var(--border-subtle)]">
          {loading ? <div className="p-4 text-sm text-[var(--text-secondary)]">Loading...</div> : null}
          {!loading && dashboards.length === 0 ? <div className="p-4 text-sm text-[var(--text-secondary)]">No dashboards found.</div> : null}
          <div className="divide-y divide-[var(--border-subtle)]">
            {dashboards.map(dashboard => (
              <button
                key={dashboard.id}
                className={`w-full px-4 py-3 text-left hover:bg-[var(--bg-secondary)] ${selectedID === dashboard.id ? 'bg-[var(--bg-secondary)]' : ''}`}
                onClick={() => setSelectedID(dashboard.id)}
              >
                <div className="truncate text-sm font-semibold text-[var(--text-primary)]">{dashboard.title}</div>
                <div className="mt-1 truncate text-xs text-[var(--text-muted)]">{dashboard.ref}</div>
                <div className="mt-2 flex items-center justify-between text-xs text-[var(--text-secondary)]">
                  <span>{dashboard.current_publication_count || 0} publications</span>
                  <span>{dashboard.managed_by_config_repo ? 'GitOps' : (dashboard.last_published_at ? formatDateTime(dashboard.last_published_at) : 'No publish')}</span>
                </div>
              </button>
            ))}
          </div>
        </aside>

        <main className="min-h-0 overflow-auto">
          {!selectedDashboard ? (
            <div className="p-6 text-sm text-[var(--text-secondary)]">Select a dashboard.</div>
          ) : (
            <div className="space-y-6 p-4 lg:p-6">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="text-xs uppercase text-[var(--text-muted)]">{selectedDashboard.ref}</div>
                  <div className="mt-1 flex flex-wrap items-center gap-2">
                    <h2 className="text-2xl font-semibold">{selectedDashboard.title}</h2>
                    {selectedDashboard.managed_by_config_repo ? (
                      <span className="rounded bg-[var(--bg-secondary)] px-2 py-1 text-xs text-[var(--text-secondary)]">GitOps</span>
                    ) : null}
                  </div>
                  {selectedDashboard.description ? <p className="mt-1 max-w-3xl text-sm text-[var(--text-secondary)]">{selectedDashboard.description}</p> : null}
                </div>
                <div className="flex flex-wrap gap-2">
                  {canWriteDashboards ? (
                    <button className="inline-flex min-h-9 items-center gap-2 rounded bg-[var(--accent)] px-3 text-sm font-medium text-white" onClick={openRefreshDashboard} disabled={Boolean(activeRefresh)}>
                      <RefreshCw className="h-4 w-4" aria-hidden="true" />
                      Refresh
                    </button>
                  ) : null}
                  {canWriteDashboards ? (
                    <button className="inline-flex min-h-9 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={() => openEditDashboard(selectedDashboard)}>
                      <Pencil className="h-4 w-4" aria-hidden="true" />
                      Edit
                    </button>
                  ) : null}
                  {canDeleteDashboards ? (
                    <button className="inline-flex min-h-9 items-center gap-2 rounded border border-rose-200 px-3 text-sm text-rose-600" onClick={() => void removeDashboard(selectedDashboard)}>
                      <Trash2 className="h-4 w-4" aria-hidden="true" />
                      Delete
                    </button>
                  ) : null}
                </div>
              </div>

              {detailLoading ? <div className="text-sm text-[var(--text-secondary)]">Loading dashboard...</div> : null}

              {latestRefresh ? (
                <RefreshPanel
                  refresh={latestRefresh}
                  saving={saving}
                  onCancel={activeRefresh ? () => void cancelRefresh(activeRefresh) : undefined}
                  onRetry={latestRefresh.failed_sources > 0 || latestRefresh.skipped_sources > 0 ? () => void retryFailed(latestRefresh) : undefined}
                />
              ) : null}

              {(view?.sections || []).map(section => {
                const sectionPublications = publicationsBySection[section.section_key] || [];
                const sectionSources = (view?.sources || []).filter(source => source.section_key === section.section_key && source.enabled);
                const requiredSources = sectionSources.filter(source => source.required_for_refresh);
                const completeness = requiredSources.length === 0
                  ? 'No required sources'
                  : `${Math.min(sectionPublications.length, requiredSources.length)}/${requiredSources.length} required`;
                return (
                  <section key={section.id || section.section_key} className="space-y-3">
                    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-subtle)] pb-2">
                      <div>
                        <h3 className="text-lg font-semibold">{section.title}</h3>
                        <div className="mt-1 text-xs text-[var(--text-muted)]">{completeness}</div>
                        {section.description ? <p className="text-sm text-[var(--text-secondary)]">{section.description}</p> : null}
                      </div>
                      {canWriteDashboards ? (
                        <div className="flex flex-wrap gap-2">
                          <button className="inline-flex min-h-9 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={() => openRefreshSection(section.section_key)} disabled={Boolean(activeRefresh)}>
                            <RefreshCw className="h-4 w-4" aria-hidden="true" />
                            Refresh
                          </button>
                          <button className="inline-flex min-h-9 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={() => openCreateSource(section.section_key)}>
                            <Plus className="h-4 w-4" aria-hidden="true" />
                            Source
                          </button>
                        </div>
                      ) : null}
                    </div>
                    {sectionPublications.length === 0 ? (
                      <div className="rounded border border-dashed border-[var(--border-subtle)] p-4 text-sm text-[var(--text-secondary)]">No publications.</div>
                    ) : (
                      <div className="grid gap-3 xl:grid-cols-2">
                        {sectionPublications.map(publication => (
                          <article key={publication.id} className="rounded border border-[var(--border-subtle)] bg-[var(--bg-primary)] p-4">
                            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                              <div>
                                <div className="text-sm font-semibold">{publication.content.title || publication.entry_key}</div>
                                <div className="text-xs text-[var(--text-muted)]">{publication.pipeline_id} / {publication.output_name}</div>
                              </div>
                              <span className={`rounded px-2 py-1 text-xs ${publication.stale ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/30 dark:text-amber-100' : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100'}`}>
                                {staleLabel(publication)}
                              </span>
                            </div>
                            <DashboardBlocks spec={publication.content} />
                            <div className="mt-3 flex flex-wrap items-center gap-3 border-t border-[var(--border-subtle)] pt-3 text-xs text-[var(--text-muted)]">
                              <span>Revision {publication.revision}</span>
                              <span>{formatDateTime(publication.published_at)}</span>
                              {publication.run_id ? <span>Run {publication.run_id.slice(0, 8)}</span> : null}
                            </div>
                          </article>
                        ))}
                      </div>
                    )}
                  </section>
                );
              })}

              <section className="grid gap-4 xl:grid-cols-2">
                <div>
                  <div className="mb-2 flex items-center gap-2 text-sm font-semibold">
                    <Clock3 className="h-4 w-4" aria-hidden="true" />
                    Sources
                  </div>
                  <div className="overflow-hidden rounded border border-[var(--border-subtle)]">
                    {(view?.sources || []).length === 0 ? <div className="p-3 text-sm text-[var(--text-secondary)]">No sources.</div> : null}
                    {(view?.sources || []).map(source => (
                      <div key={source.id} className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-subtle)] p-3 last:border-b-0">
                        <div>
                          <div className="text-sm font-medium">{source.pipeline_id}</div>
                          <div className="text-xs text-[var(--text-muted)]">{source.section_key} / {source.output_name}{source.entry_key ? ` / ${source.entry_key}` : ''}</div>
                          <div className="mt-1 flex flex-wrap gap-1 text-xs">
                            <span className={`rounded px-2 py-0.5 ${source.enabled ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100' : 'bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200'}`}>
                              {source.enabled ? 'Enabled' : 'Disabled'}
                            </span>
                            <span className="rounded bg-[var(--bg-secondary)] px-2 py-0.5 text-[var(--text-secondary)]">{source.required_for_refresh ? 'Required' : 'Optional'}</span>
                            {latestRefreshSourceByID.get(source.id) ? (
                              <span className="rounded bg-[var(--bg-secondary)] px-2 py-0.5 text-[var(--text-secondary)]">{refreshStatusLabel(latestRefreshSourceByID.get(source.id)?.status || '')}</span>
                            ) : null}
                          </div>
                        </div>
                        {canWriteDashboards ? (
                          <div className="flex gap-2">
                            <button className="rounded border border-[var(--border-subtle)] p-2" onClick={() => openRefreshSource(source)} aria-label="Refresh source" disabled={Boolean(activeRefresh) || !source.enabled}>
                              <RefreshCw className="h-4 w-4" aria-hidden="true" />
                            </button>
                            <button className="rounded border border-[var(--border-subtle)] p-2" onClick={() => openEditSource(source)} aria-label="Edit source">
                              <Pencil className="h-4 w-4" aria-hidden="true" />
                            </button>
                            <button className="rounded border border-rose-200 p-2 text-rose-600" onClick={() => void removeSource(source)} aria-label="Delete source">
                              <Trash2 className="h-4 w-4" aria-hidden="true" />
                            </button>
                          </div>
                        ) : null}
                      </div>
                    ))}
                  </div>
                </div>

                <div>
                  <div className="mb-2 flex items-center gap-2 text-sm font-semibold">
                    <Clock3 className="h-4 w-4" aria-hidden="true" />
                    Schedules
                  </div>
                  <div className="mb-4 overflow-hidden rounded border border-[var(--border-subtle)]">
                    {refreshSchedules.length === 0 ? <div className="p-3 text-sm text-[var(--text-secondary)]">No schedules.</div> : null}
                    {refreshSchedules.map(schedule => (
                      <div key={schedule.id} className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-subtle)] p-3 last:border-b-0">
                        <div>
                          <div className="text-sm font-medium">{schedule.name}</div>
                          <div className="text-xs text-[var(--text-muted)]">{schedule.cron_expression} / {schedule.timezone} / {schedule.mode}</div>
                          <div className="mt-1 text-xs text-[var(--text-secondary)]">
                            {schedule.enabled ? `Next ${formatDateTime(schedule.next_run_at) || 'pending'}` : 'Disabled'}
                            {schedule.managed_by_config_repo ? ' / GitOps' : ''}
                          </div>
                        </div>
                        {canWriteDashboards ? (
                          <button className="inline-flex min-h-9 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={() => void runSchedule(schedule)} disabled={saving || Boolean(activeRefresh)}>
                            <RefreshCw className="h-4 w-4" aria-hidden="true" />
                            Run
                          </button>
                        ) : null}
                      </div>
                    ))}
                  </div>
                  <div className="mb-2 flex items-center gap-2 text-sm font-semibold">
                    <AlertTriangle className="h-4 w-4" aria-hidden="true" />
                    History
                  </div>
                  <div className="overflow-hidden rounded border border-[var(--border-subtle)]">
                    {history.length === 0 ? <div className="p-3 text-sm text-[var(--text-secondary)]">No history.</div> : null}
                    {history.slice(0, 8).map(event => (
                      <div key={event.id} className="border-b border-[var(--border-subtle)] p-3 last:border-b-0">
                        <div className="text-sm font-medium">{event.event_type}</div>
                        <div className="text-xs text-[var(--text-muted)]">{event.section_key} / {event.entry_key} / revision {event.revision}</div>
                        <div className="mt-1 text-xs text-[var(--text-secondary)]">{formatDateTime(event.created_at)}</div>
                      </div>
                    ))}
                  </div>
                  <div className="mt-4 overflow-hidden rounded border border-[var(--border-subtle)]">
                    {refreshes.length === 0 ? <div className="p-3 text-sm text-[var(--text-secondary)]">No refreshes.</div> : null}
                    {refreshes.slice(0, 6).map(refresh => (
                      <div key={refresh.id} className="border-b border-[var(--border-subtle)] p-3 last:border-b-0">
                        <div className="flex items-center justify-between gap-2">
                          <div className="text-sm font-medium">{refreshStatusLabel(refresh.status)}</div>
                          <div className="text-xs text-[var(--text-muted)]">{refresh.scope_type} / {refresh.mode}</div>
                        </div>
                        <div className="mt-1 text-xs text-[var(--text-secondary)]">{refresh.successful_sources}/{refresh.total_sources} sources · {formatDateTime(refresh.created_at)}</div>
                      </div>
                    ))}
                  </div>
                </div>
              </section>
            </div>
          )}
        </main>
      </div>

      {dashboardModal ? (
        <DashboardModal
          modal={dashboardModal}
          form={dashboardForm}
          teams={teams}
          saving={saving}
          error={formError}
          onChange={setDashboardForm}
          onClose={() => setDashboardModal(null)}
          onSubmit={() => void submitDashboard()}
        />
      ) : null}

      {sourceModal ? (
        <SourceModal
          form={sourceForm}
          pipelines={pipelines}
          saving={saving}
          error={formError}
          onChange={setSourceForm}
          onClose={() => setSourceModal(null)}
          onSubmit={() => void submitSource()}
        />
      ) : null}

      {refreshModal ? (
        <RefreshModal
          title={refreshModal.label}
          form={refreshForm}
          saving={saving}
          error={formError}
          onChange={setRefreshForm}
          onClose={() => setRefreshModal(null)}
          onSubmit={() => void submitRefresh()}
        />
      ) : null}
    </div>
  );
}

function RefreshPanel({
  refresh,
  saving,
  onCancel,
  onRetry,
}: {
  refresh: DashboardRefresh;
  saving: boolean;
  onCancel?: () => void;
  onRetry?: () => void;
}) {
  const progress = refreshProgress(refresh);
  return (
    <section className="rounded border border-[var(--border-subtle)] bg-[var(--bg-primary)] p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold">Refresh {refreshStatusLabel(refresh.status)}</div>
          <div className="mt-1 text-xs text-[var(--text-muted)]">{refresh.scope_type} / {refresh.mode} / {formatDateTime(refresh.created_at)}</div>
        </div>
        <div className="flex flex-wrap gap-2">
          {onRetry ? (
            <button className="inline-flex min-h-9 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={onRetry} disabled={saving}>
              <RotateCcw className="h-4 w-4" aria-hidden="true" />
              Retry failed
            </button>
          ) : null}
          {onCancel ? (
            <button className="inline-flex min-h-9 items-center gap-2 rounded border border-rose-200 px-3 text-sm text-rose-600" onClick={onCancel} disabled={saving}>
              <X className="h-4 w-4" aria-hidden="true" />
              Cancel
            </button>
          ) : null}
        </div>
      </div>
      <div className="mt-3 h-2 overflow-hidden rounded bg-[var(--bg-secondary)]">
        <div className="h-full bg-[var(--accent)]" style={{ width: `${progress}%` }} />
      </div>
      <div className="mt-2 flex flex-wrap gap-3 text-xs text-[var(--text-secondary)]">
        <span>{refresh.successful_sources} success</span>
        <span>{refresh.failed_sources} failed</span>
        <span>{refresh.skipped_sources} skipped</span>
        <span>{refresh.running_sources + refresh.queued_sources} active</span>
      </div>
      {(refresh.sources || []).length > 0 ? (
        <div className="mt-3 grid gap-2 md:grid-cols-2">
          {(refresh.sources || []).map(source => (
            <div key={source.id} className="rounded border border-[var(--border-subtle)] p-2">
              <div className="truncate text-xs font-medium">{source.pipeline_id}</div>
              <div className="mt-1 flex flex-wrap gap-2 text-xs text-[var(--text-muted)]">
                <span>{source.section_key} / {source.output_name}</span>
                <span>{refreshStatusLabel(source.status)}</span>
                <span>{source.required ? 'required' : 'optional'}</span>
              </div>
              {source.error ? <div className="mt-1 text-xs text-rose-600">{source.error}</div> : null}
            </div>
          ))}
        </div>
      ) : null}
    </section>
  );
}

function RefreshModal({
  title,
  form,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  title: string;
  form: DashboardRefreshFormState;
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardRefreshFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  return (
    <ModalFrame title={title} onClose={onClose}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Scope">
          <select className="form-input" value={form.scopeType} onChange={event => onChange({ ...form, scopeType: event.target.value as DashboardRefreshFormState['scopeType'] })}>
            <option value="dashboard">Dashboard</option>
            <option value="section">Section</option>
            <option value="source">Source</option>
          </select>
        </Field>
        <Field label="Mode">
          <select className="form-input" value={form.mode} onChange={event => onChange({ ...form, mode: event.target.value as DashboardRefreshFormState['mode'] })}>
            <option value="strict">Strict</option>
            <option value="best_effort">Best effort</option>
          </select>
        </Field>
        {form.scopeType === 'section' ? (
          <Field label="Section">
            <input className="form-input" value={form.sectionKey} onChange={event => onChange({ ...form, sectionKey: event.target.value })} />
          </Field>
        ) : null}
        {form.scopeType === 'source' ? (
          <Field label="Source ID">
            <input className="form-input" value={form.sourceID} onChange={event => onChange({ ...form, sourceID: event.target.value })} />
          </Field>
        ) : null}
        <Field label="Timeout">
          <input className="form-input" value={form.timeout} onChange={event => onChange({ ...form, timeout: event.target.value })} />
        </Field>
        <Field label="Concurrency">
          <input className="form-input" type="number" min="1" max="16" value={form.maxConcurrency} onChange={event => onChange({ ...form, maxConcurrency: event.target.value })} />
        </Field>
      </div>
      <ModalActions saving={saving} error={error} onClose={onClose} onSubmit={onSubmit} submitLabel="Start" />
    </ModalFrame>
  );
}

function DashboardModal({
  modal,
  form,
  teams,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  modal: DashboardModalState;
  form: DashboardFormState;
  teams: string[];
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  return (
    <ModalFrame title={modal.mode === 'create' ? 'New Dashboard' : 'Edit Dashboard'} onClose={onClose}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Team">
          <input className="form-input" list="dashboard-team-options" value={form.teamPath} onChange={event => onChange({ ...form, teamPath: event.target.value })} />
          <datalist id="dashboard-team-options">
            {teams.map(team => <option key={team} value={team} />)}
          </datalist>
        </Field>
        <Field label="Slug">
          <input className="form-input" value={form.slug} onChange={event => onChange({ ...form, slug: event.target.value })} />
        </Field>
        <Field label="Title">
          <input className="form-input" value={form.title} onChange={event => onChange({ ...form, title: event.target.value })} />
        </Field>
        <Field label="Visibility">
          <select className="form-input" value={form.visibility} onChange={event => onChange({ ...form, visibility: event.target.value })}>
            <option value="team">Team</option>
            <option value="restricted">Restricted</option>
            <option value="workspace">Workspace</option>
          </select>
        </Field>
        <Field label="Section">
          <input className="form-input" value={form.sectionKey} onChange={event => onChange({ ...form, sectionKey: event.target.value })} />
        </Field>
        <Field label="Section Title">
          <input className="form-input" value={form.sectionTitle} onChange={event => onChange({ ...form, sectionTitle: event.target.value })} />
        </Field>
        <Field label="Description" wide>
          <textarea className="form-input min-h-24" value={form.description} onChange={event => onChange({ ...form, description: event.target.value })} />
        </Field>
      </div>
      <ModalActions saving={saving} error={error} onClose={onClose} onSubmit={onSubmit} />
    </ModalFrame>
  );
}

function SourceModal({
  form,
  pipelines,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  form: DashboardSourceFormState;
  pipelines: string[];
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardSourceFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  return (
    <ModalFrame title="Dashboard Source" onClose={onClose}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Section">
          <input className="form-input" value={form.sectionKey} onChange={event => onChange({ ...form, sectionKey: event.target.value })} />
        </Field>
        <Field label="Pipeline">
          <input className="form-input" list="dashboard-pipeline-options" value={form.pipelineID} onChange={event => onChange({ ...form, pipelineID: event.target.value })} />
          <datalist id="dashboard-pipeline-options">
            {pipelines.map(pipeline => <option key={pipeline} value={pipeline} />)}
          </datalist>
        </Field>
        <Field label="Output">
          <input className="form-input" value={form.outputName} onChange={event => onChange({ ...form, outputName: event.target.value })} />
        </Field>
        <Field label="Entry">
          <input className="form-input" value={form.entryKey} onChange={event => onChange({ ...form, entryKey: event.target.value })} />
        </Field>
        <Field label="Order">
          <input className="form-input" type="number" value={form.refreshOrder} onChange={event => onChange({ ...form, refreshOrder: event.target.value })} />
        </Field>
        <div className="flex items-end gap-4">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={form.enabled} onChange={event => onChange({ ...form, enabled: event.target.checked })} />
            Enabled
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={form.requiredForRefresh} onChange={event => onChange({ ...form, requiredForRefresh: event.target.checked })} />
            Required
          </label>
        </div>
      </div>
      <ModalActions saving={saving} error={error} onClose={onClose} onSubmit={onSubmit} />
    </ModalFrame>
  );
}

function ModalFrame({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4">
      <div className="max-h-[90vh] w-full max-w-2xl overflow-auto rounded bg-[var(--bg-primary)] shadow-xl">
        <div className="flex items-center justify-between border-b border-[var(--border-subtle)] px-4 py-3">
          <h3 className="text-base font-semibold">{title}</h3>
          <button className="rounded p-2 hover:bg-[var(--bg-secondary)]" onClick={onClose} aria-label="Close">
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
        <div className="p-4">{children}</div>
      </div>
    </div>
  );
}

function Field({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return (
    <label className={`block text-sm ${wide ? 'sm:col-span-2' : ''}`}>
      <span className="mb-1 block text-xs font-medium uppercase text-[var(--text-muted)]">{label}</span>
      {children}
    </label>
  );
}

function ModalActions({
  saving,
  error,
  onClose,
  onSubmit,
  submitLabel = 'Save',
}: {
  saving: boolean;
  error: string | null;
  onClose: () => void;
  onSubmit: () => void;
  submitLabel?: string;
}) {
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-[var(--border-subtle)] pt-4">
      <div className="text-sm text-rose-600">{error}</div>
      <div className="flex gap-2">
        <button className="inline-flex min-h-9 items-center gap-2 rounded border border-[var(--border-subtle)] px-3 text-sm" onClick={onClose} disabled={saving}>
          <X className="h-4 w-4" aria-hidden="true" />
          Cancel
        </button>
        <button className="inline-flex min-h-9 items-center gap-2 rounded bg-[var(--accent)] px-3 text-sm font-medium text-white" onClick={onSubmit} disabled={saving}>
          <Save className="h-4 w-4" aria-hidden="true" />
          {submitLabel}
        </button>
      </div>
    </div>
  );
}
