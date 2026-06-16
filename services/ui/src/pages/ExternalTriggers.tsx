import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  CheckCircle2,
  Clipboard,
  Copy,
  Edit3,
  History,
  PauseCircle,
  PlayCircle,
  RefreshCw,
  Shield,
  Trash2,
} from 'lucide-react';

import { ResourceCollectionToolbar } from '../features/editor/ResourceCollectionToolbar';
import { ExternalTriggerFormModal } from '../features/external-triggers/ExternalTriggerFormModal';
import { ExternalTriggerCards } from '../features/external-triggers/ExternalTriggerCards';
import {
  externalTriggerGroupLabel,
  externalTriggerRelativeLabel,
  externalTriggerScopeLabel,
  type AllowedCaller,
  type ExternalTrigger,
  type ExternalTriggerForm,
  type ExternalTriggerModalState,
  type SelectOption,
} from '../features/external-triggers/model';
import { apiClient, buildApiUrl } from '../lib/api';
import { fetchPipelineRunGroupPaths } from '../lib/resourceGroups';

type ExternalTriggerInvocation = {
  id: string;
  trigger_id: string;
  caller_type: string;
  caller_id: string;
  status: string;
  run_id?: string;
  idempotency_key?: string;
  event_type?: string;
  source_ip?: string;
  created_at?: string;
  error?: string;
};

type PipelineListItem = {
  id?: string;
  identifier?: string;
};

type ScopeListItem = {
  scope?: string;
  name?: string;
};

type UserListItem = {
  id?: string;
  sub?: string;
  email?: string;
  status?: string;
};

type ServiceAccountListItem = {
  id?: string;
  sub?: string;
  email?: string;
  status?: string;
};

type GroupListItem = {
  id?: string;
  name?: string;
};

type ExternalTriggersPageProps = {
  canWriteExternalTriggers: boolean;
  canDeleteExternalTriggers: boolean;
};

const emptyForm: ExternalTriggerForm = {
  id: '',
  name: '',
  description: '',
  pipeline: '',
  scope: '',
  runGroupPath: 'root',
  enabled: true,
  allowedCallers: [],
  variableMappingText: '{\n  "VERSION": "payload.version"\n}',
  payloadSchemaText: '{\n  "type": "object"\n}',
  rateLimitPerMinute: '',
};

function ExternalTriggersPage({ canWriteExternalTriggers, canDeleteExternalTriggers }: ExternalTriggersPageProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const routeSelectedID = useMemo(() => externalTriggerIDFromPath(location.pathname), [location.pathname]);
  const [triggers, setTriggers] = useState<ExternalTrigger[]>([]);
  const [selectedID, setSelectedID] = useState(() => routeSelectedID);
  const [selected, setSelected] = useState<ExternalTrigger | null>(null);
  const [invocations, setInvocations] = useState<ExternalTriggerInvocation[]>([]);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [scopes, setScopes] = useState<string[]>([]);
  const [runGroups, setRunGroups] = useState<string[]>([]);
  const [users, setUsers] = useState<UserListItem[]>([]);
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccountListItem[]>([]);
  const [groups, setGroups] = useState<GroupListItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [invocationsLoading, setInvocationsLoading] = useState(false);
  const [error, setError] = useState('');
  const [modal, setModal] = useState<ExternalTriggerModalState | null>(null);
  const [form, setForm] = useState<ExternalTriggerForm>(emptyForm);
  const [formError, setFormError] = useState('');
  const [saving, setSaving] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
  const [copyState, setCopyState] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [callerDraft, setCallerDraft] = useState<AllowedCaller>({ type: 'service_account', id: '' });

  const selectedTrigger = useMemo(
    () => selected || triggers.find(trigger => trigger.id === selectedID) || null,
    [selected, selectedID, triggers]
  );

  const filteredTriggers = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    if (!term) return triggers;
    return triggers.filter(trigger => [
      trigger.id,
      trigger.name,
      trigger.description,
      trigger.pipeline,
      trigger.scope,
      trigger.run_group_path,
      trigger.source,
      ...(trigger.allowed_callers || []).flatMap(caller => [caller.type, caller.id]),
    ].join(' ').toLowerCase().includes(term));
  }, [searchTerm, triggers]);

  const invokeURL = useMemo(() => {
    if (!selectedTrigger) return '';
    return buildApiUrl(`/v1/external-triggers/${encodeURIComponent(selectedTrigger.id)}/invoke`);
  }, [selectedTrigger]);

  const exampleCurl = useMemo(() => {
    if (!selectedTrigger) return '';
    return [
      `curl -X POST ${invokeURL} \\`,
      '  -H "Authorization: Bearer <SERVICE_ACCOUNT_TOKEN>" \\',
      '  -H "Content-Type: application/json" \\',
      `  -d '{"event_type":"servicenow.change.approved","idempotency_key":"servicenow.change.approved:<SOURCE_EVENT_ID>","variables":{"VERSION":"1.2.3"},"payload":{}}'`,
    ].join('\n');
  }, [invokeURL, selectedTrigger]);

  const pipelineOptions = useMemo(
    () => uniqueSortedStrings([...pipelines, form.pipeline].map(normalizeIdentifier).filter((id): id is string => Boolean(id))),
    [form.pipeline, pipelines]
  );

  const scopeOptions = useMemo(
    () => uniqueSortedStrings(['', ...scopes, form.scope].map(normalizeScopeOption)),
    [form.scope, scopes]
  );

  const runGroupOptions = useMemo(
    () => uniqueRunGroupOptions([...runGroups, form.runGroupPath]),
    [form.runGroupPath, runGroups]
  );

  const callerOptions = useMemo<Record<AllowedCaller['type'], SelectOption[]>>(
    () => ({
      service_account: serviceAccounts
        .filter(account => account.status !== 'disabled')
        .map(account => ({ value: account.sub || account.id || '', label: identityLabel(account.sub, account.email, account.id) }))
        .filter(option => Boolean(option.value)),
      user: users
        .filter(user => user.status !== 'disabled')
        .map(user => ({ value: user.sub || user.id || user.email || '', label: identityLabel(user.sub, user.email, user.id) }))
        .filter(option => Boolean(option.value)),
      auth_group: groups
        .map(group => ({ value: group.id || group.name || '', label: group.name || group.id || '' }))
        .filter(option => Boolean(option.value)),
    }),
    [groups, serviceAccounts, users]
  );

  const activeCallerOptions = callerOptions[callerDraft.type] || [];
  const selectedManagedByGitOps = Boolean(selectedTrigger?.managed_by_config_repo);

  const fetchJson = useCallback(async <T,>(path: string, options?: RequestInit): Promise<T> => {
    const response = await apiClient.fetch(path, { cache: 'no-store', ...options });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed (${response.status})`);
    }
    const text = await response.text();
    if (!text) return undefined as T;
    return JSON.parse(text) as T;
  }, []);

  const loadTriggers = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await fetchJson<ExternalTrigger[]>('/v1/external-triggers');
      const list = Array.isArray(data) ? data : [];
      setTriggers(list);
      setSelectedID(prev => prev || routeSelectedID || '');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load external triggers');
      setTriggers([]);
      setSelectedID('');
    } finally {
      setLoading(false);
    }
  }, [fetchJson, routeSelectedID]);

  const loadReferenceData = useCallback(async () => {
    const [pipelineData, runtimeScopeData, secretScopeData, variableScopeData, runGroupData, userData, serviceAccountData, groupData] = await Promise.all([
      fetchJson<Array<string | PipelineListItem>>('/v1/pipelines?include_source=true').catch(() => []),
      fetchJson<Array<string | ScopeListItem>>('/v1/system/dispatcher/scopes').catch(() => []),
      fetchJson<Array<string | ScopeListItem>>('/v1/secrets/scopes').catch(() => []),
      fetchJson<Array<string | ScopeListItem>>('/v1/variables/scopes').catch(() => []),
      fetchPipelineRunGroupPaths().catch(() => []),
      fetchJson<UserListItem[]>('/v1/admin/users').catch(() => []),
      fetchJson<ServiceAccountListItem[]>('/v1/admin/service-accounts').catch(() => []),
      fetchJson<GroupListItem[]>('/v1/access/auth-groups').catch(() => []),
    ]);

    const pipelineIDs = (Array.isArray(pipelineData) ? pipelineData : [])
      .map(item => (typeof item === 'string' ? item : item.identifier || item.id || ''))
      .map(normalizeIdentifier)
      .filter((id): id is string => Boolean(id));
    setPipelines(uniqueSortedStrings(pipelineIDs));

    const scopeIDs = [...runtimeScopeData, ...secretScopeData, ...variableScopeData]
      .map(item => (typeof item === 'string' ? item : item.scope || item.name || ''))
      .map(normalizeScopeOption);
    setScopes(uniqueSortedStrings(['', ...scopeIDs]));
    setRunGroups(uniqueRunGroupOptions((Array.isArray(runGroupData) ? runGroupData : []).map(normalizeIdentifier)));
    setUsers(Array.isArray(userData) ? userData : []);
    setServiceAccounts(Array.isArray(serviceAccountData) ? serviceAccountData : []);
    setGroups(Array.isArray(groupData) ? groupData : []);
  }, [fetchJson]);

  const loadSelected = useCallback(async (id: string) => {
    if (!id) {
      setSelected(null);
      setInvocations([]);
      return;
    }
    setDetailLoading(true);
    try {
      const detail = await fetchJson<ExternalTrigger>(`/v1/external-triggers/${encodeURIComponent(id)}`);
      setSelected(detail);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load external trigger');
      setSelected(null);
    } finally {
      setDetailLoading(false);
    }
  }, [fetchJson]);

  const loadInvocations = useCallback(async (id: string) => {
    if (!id) {
      setInvocations([]);
      return;
    }
    setInvocationsLoading(true);
    try {
      const data = await fetchJson<ExternalTriggerInvocation[]>(`/v1/external-triggers/${encodeURIComponent(id)}/invocations?limit=20`);
      setInvocations(Array.isArray(data) ? data : []);
    } catch {
      setInvocations([]);
    } finally {
      setInvocationsLoading(false);
    }
  }, [fetchJson]);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void loadTriggers();
      void loadReferenceData();
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadReferenceData, loadTriggers]);

  useEffect(() => {
    if (!routeSelectedID || routeSelectedID === selectedID) return;
    const timeout = window.setTimeout(() => setSelectedID(routeSelectedID), 0);
    return () => window.clearTimeout(timeout);
  }, [routeSelectedID, selectedID]);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void loadSelected(selectedID);
      void loadInvocations(selectedID);
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadInvocations, loadSelected, selectedID]);

  const selectTrigger = useCallback((id: string) => {
    setSelectedID(id);
    navigate(id ? `/external-triggers/${encodeRouteIdentifier(id)}` : '/external-triggers');
  }, [navigate]);

  const openCreate = () => {
    if (!canWriteExternalTriggers) return;
    const pipeline = pipelines[0] || '';
    const pipelineParent = parentPathFromIdentifier(pipeline);
    const defaultRunGroup = pipelineParent && runGroups.includes(pipelineParent) ? pipelineParent : 'root';
    setForm({ ...emptyForm, pipeline, runGroupPath: defaultRunGroup });
    setCallerDraft({ type: 'service_account', id: callerOptions.service_account[0]?.value || '' });
    setFormError('');
    setModal({ mode: 'create' });
  };

  const openEdit = (trigger: ExternalTrigger) => {
    setForm({
      id: trigger.id,
      name: trigger.name || trigger.id,
      description: trigger.description || '',
      pipeline: trigger.pipeline || '',
      scope: normalizeScopeOption(trigger.scope),
      runGroupPath: normalizeIdentifier(trigger.run_group_path) || 'root',
      enabled: Boolean(trigger.enabled),
      allowedCallers: Array.isArray(trigger.allowed_callers) ? trigger.allowed_callers : [],
      variableMappingText: JSON.stringify(trigger.variable_mapping || {}, null, 2),
      payloadSchemaText: JSON.stringify(trigger.payload_schema || {}, null, 2),
      rateLimitPerMinute: readPerMinute(trigger.rate_limit),
    });
    setCallerDraft({ type: 'service_account', id: callerOptions.service_account[0]?.value || '' });
    setFormError('');
    setModal({ mode: 'edit', trigger });
  };

  const closeModal = () => {
    if (saving) return;
    setModal(null);
    setFormError('');
  };

  const addAllowedCaller = () => {
    const id = callerDraft.id.trim();
    if (!id) return;
    setForm(prev => {
      const exists = prev.allowedCallers.some(caller => caller.type === callerDraft.type && caller.id === id);
      if (exists) return prev;
      return { ...prev, allowedCallers: [...prev.allowedCallers, { type: callerDraft.type, id }] };
    });
    setCallerDraft(prev => ({ ...prev, id: '' }));
  };

  const removeAllowedCaller = (idx: number) => {
    setForm(prev => ({ ...prev, allowedCallers: prev.allowedCallers.filter((_, index) => index !== idx) }));
  };

  const buildPayload = () => {
    let variableMapping: Record<string, string>;
    let payloadSchema: Record<string, unknown>;
    try {
      variableMapping = JSON.parse(form.variableMappingText || '{}') as Record<string, string>;
    } catch {
      throw new Error('Variable mapping must be valid JSON.');
    }
    try {
      payloadSchema = JSON.parse(form.payloadSchemaText || '{}') as Record<string, unknown>;
    } catch {
      throw new Error('Payload schema must be valid JSON.');
    }
    const rateLimit: Record<string, unknown> = {};
    const perMinute = Number(form.rateLimitPerMinute);
    if (Number.isFinite(perMinute) && perMinute > 0) {
      rateLimit.per_minute = perMinute;
    }
    return {
      id: form.id.trim(),
      name: form.name.trim(),
      description: form.description.trim(),
      pipeline: normalizeIdentifier(form.pipeline),
      scope: normalizeScopeOption(form.scope),
      run_group_path: normalizeIdentifier(form.runGroupPath) || 'root',
      enabled: form.enabled,
      allowed_callers: form.allowedCallers,
      variable_mapping: variableMapping,
      payload_schema: payloadSchema,
      rate_limit: rateLimit,
    };
  };

  const saveTrigger = async (event: FormEvent) => {
    event.preventDefault();
    if (!modal) return;
    setSaving(true);
    setFormError('');
    try {
      const payload = buildPayload();
      const path =
        modal.mode === 'edit'
          ? `/v1/external-triggers/${encodeURIComponent(modal.trigger?.id || payload.id)}`
          : '/v1/external-triggers';
      const method = modal.mode === 'edit' ? 'PUT' : 'POST';
      const saved = await fetchJson<ExternalTrigger>(path, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      setModal(null);
      await loadTriggers();
      selectTrigger(saved?.id || payload.id);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Unable to save external trigger');
    } finally {
      setSaving(false);
    }
  };

  const updateTrigger = async (trigger: ExternalTrigger, updates: Partial<ExternalTrigger>) => {
    const payload = {
      id: trigger.id,
      name: trigger.name,
      description: trigger.description || '',
      pipeline: trigger.pipeline,
      scope: normalizeScopeOption(trigger.scope),
      run_group_path: normalizeIdentifier(trigger.run_group_path) || 'root',
      enabled: trigger.enabled,
      allowed_callers: trigger.allowed_callers || [],
      variable_mapping: trigger.variable_mapping || {},
      payload_schema: trigger.payload_schema || {},
      rate_limit: trigger.rate_limit || {},
      ...updates,
    };
    await fetchJson<ExternalTrigger>(`/v1/external-triggers/${encodeURIComponent(trigger.id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    await loadTriggers();
    await loadSelected(trigger.id);
  };

  const deleteTrigger = async (trigger: ExternalTrigger) => {
    if (!canDeleteExternalTriggers || deletePending) return;
    setDeletePending(true);
    try {
      await fetchJson<void>(`/v1/external-triggers/${encodeURIComponent(trigger.id)}`, { method: 'DELETE' });
      selectTrigger('');
      setSelected(null);
      await loadTriggers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete external trigger');
    } finally {
      setDeletePending(false);
    }
  };

  const copyText = async (label: string, value: string) => {
    if (!value) return;
    await navigator.clipboard.writeText(value);
    setCopyState(label);
    window.setTimeout(() => setCopyState(''), 1600);
  };

  return (
    <div data-page="external-triggers" className="active h-full flex flex-col">
      <ResourceCollectionToolbar
        resourceLabel="external trigger"
        searchTerm={searchTerm}
        canCreate={canWriteExternalTriggers}
        createLabel="New trigger"
        createDisabledReason="You have read-only access to external triggers"
        showCreateWhenDisabled
        onSearchTermChange={setSearchTerm}
        onCreate={openCreate}
        onRefresh={() => void loadTriggers()}
        refreshDisabled={loading || saving}
        filters={!canWriteExternalTriggers ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
      />
      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {error && <div className="dispatcher-error mb-4">{error}</div>}
        <div className={`grid grid-cols-1 gap-5 ${selectedTrigger ? 'xl:grid-cols-[minmax(0,1fr)_440px]' : ''}`}>
          <section className="min-w-0">
            {loading ? <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading external triggers...</div> : null}
            {!loading && !filteredTriggers.length ? (
              <div className="pipelines-empty">
                <h2 className="text-base font-semibold text-[var(--text-primary)]">No external triggers found</h2>
                <p className="text-sm text-[var(--text-secondary)]">
                  {searchTerm.trim() ? 'Adjust your search.' : 'Create an external trigger to expose an authenticated endpoint.'}
                </p>
              </div>
            ) : null}
            {!loading && filteredTriggers.length ? (
              <ExternalTriggerCards triggers={filteredTriggers} selectedID={selectedID} onSelect={selectTrigger} />
            ) : null}
          </section>

          {selectedTrigger ? (
            <aside className="glass-card rounded-xl border border-[var(--border-primary)] p-4 min-h-[520px]">
              <div className="space-y-5">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h2 className="text-lg font-semibold text-[var(--text-primary)] truncate">{selectedTrigger.name}</h2>
                    <p className="text-xs font-mono text-[var(--text-secondary)] truncate">{selectedTrigger.id}</p>
                  </div>
                  <span className={selectedTrigger.enabled ? 'runner-pill runner-pill--ok' : 'runner-pill runner-pill--muted'}>
                    {selectedTrigger.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                </div>

                <div className="flex flex-wrap gap-2">
                  {canWriteExternalTriggers && (
                    <>
                      <button type="button" className="pipelines-secondary-button" onClick={() => openEdit(selectedTrigger)} disabled={selectedManagedByGitOps} title={selectedManagedByGitOps ? 'Change GitOps-managed external triggers in the config repository' : 'Edit'}>
                        <Edit3 className="h-4 w-4" />
                        Edit
                      </button>
                      <button
                        type="button"
                        className="pipelines-secondary-button"
                        onClick={() => void updateTrigger(selectedTrigger, { enabled: !selectedTrigger.enabled })}
                        disabled={selectedManagedByGitOps}
                        title={selectedManagedByGitOps ? 'Change GitOps-managed external triggers in the config repository' : selectedTrigger.enabled ? 'Disable' : 'Enable'}
                      >
                        {selectedTrigger.enabled ? <PauseCircle className="h-4 w-4" /> : <PlayCircle className="h-4 w-4" />}
                        {selectedTrigger.enabled ? 'Disable' : 'Enable'}
                      </button>
                    </>
                  )}
                  <button type="button" className="pipelines-secondary-button" onClick={() => void copyText('url', invokeURL)}>
                    <Copy className="h-4 w-4" />
                    {copyState === 'url' ? 'Copied' : 'URL'}
                  </button>
                  {canDeleteExternalTriggers && (
                    <button type="button" className="pipelines-danger-button" onClick={() => void deleteTrigger(selectedTrigger)} disabled={deletePending || selectedManagedByGitOps} title={selectedManagedByGitOps ? 'Change GitOps-managed external triggers in the config repository' : 'Delete'}>
                      <Trash2 className="h-4 w-4" />
                      Delete
                    </button>
                  )}
                </div>

                <dl className="grid grid-cols-2 gap-3 text-sm">
                  <Meta label="Pipeline" value={selectedTrigger.pipeline} />
                  <Meta label="Scope" value={externalTriggerScopeLabel(selectedTrigger.scope)} />
                  <Meta label="Run group" value={externalTriggerGroupLabel(selectedTrigger.run_group_path)} />
                  <Meta label="Created by" value={selectedTrigger.created_by || '-'} />
                  <Meta label="Last used" value={formatDate(selectedTrigger.last_used_at)} />
                  <Meta label="Source" value={selectedTrigger.managed_by_config_repo ? `GitOps ${selectedTrigger.config_source_path || ''}`.trim() : selectedTrigger.source || 'database'} />
                </dl>

                <section className="space-y-2">
                  <div className="flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]">
                    <Shield className="h-4 w-4" />
                    Allowed Callers
                  </div>
                  <div className="space-y-2">
                    {(selectedTrigger.allowed_callers || []).map(caller => (
                      <div key={`${caller.type}:${caller.id}`} className="flex items-center justify-between rounded-lg border border-[var(--border-primary)] px-3 py-2">
                        <span className="text-sm font-mono text-[var(--text-primary)]">{caller.type}:{caller.id}</span>
                      </div>
                    ))}
                    {!(selectedTrigger.allowed_callers || []).length && <p className="text-sm text-[var(--text-secondary)]">No callers configured.</p>}
                  </div>
                </section>

                <section className="space-y-2">
                  <div className="flex items-center justify-between">
                    <h3 className="text-sm font-semibold text-[var(--text-primary)]">Example curl</h3>
                    <button type="button" className="pipelines-icon-only" title="Copy curl" aria-label="Copy curl" onClick={() => void copyText('curl', exampleCurl)}>
                      {copyState === 'curl' ? <CheckCircle2 className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}
                    </button>
                  </div>
                  <pre className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-xs overflow-auto text-[var(--text-primary)]">
                    {exampleCurl}
                  </pre>
                </section>

                <section className="space-y-2">
                  <div className="flex items-center justify-between">
                    <h3 className="inline-flex items-center gap-2 text-sm font-semibold text-[var(--text-primary)]">
                      <History className="h-4 w-4" />
                      Recent invocations
                    </h3>
                    <button type="button" className="pipelines-icon-only" title="Refresh invocations" aria-label="Refresh invocations" onClick={() => void loadInvocations(selectedTrigger.id)}>
                      <RefreshCw className="h-4 w-4" />
                    </button>
                  </div>
                  {detailLoading || invocationsLoading ? (
                    <p className="text-sm text-[var(--text-secondary)]">Loading details...</p>
                  ) : invocations.length ? (
                    <div className="space-y-2">
                      {invocations.map(invocation => (
                        <div key={invocation.id} className="rounded-lg border border-[var(--border-primary)] p-3">
                          <div className="flex items-center justify-between gap-2">
                            <span className={invocation.status === 'queued' ? 'runner-pill runner-pill--ok' : invocation.status === 'failed' ? 'runner-pill runner-pill--error' : 'runner-pill runner-pill--muted'}>
                              {invocation.status}
                            </span>
                            <span className="text-xs text-[var(--text-secondary)]">{externalTriggerRelativeLabel(invocation.created_at)}</span>
                          </div>
                          <div className="mt-2 text-xs text-[var(--text-secondary)] space-y-1">
                            <p className="font-mono truncate">{invocation.caller_type}:{invocation.caller_id}</p>
                            {invocation.event_type && <p className="truncate">Event: {invocation.event_type}</p>}
                            {invocation.idempotency_key && <p className="truncate">Idempotency: {invocation.idempotency_key}</p>}
                            {invocation.error && <p className="text-red-500 truncate">Error: {invocation.error}</p>}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No invocations yet.</p>
                  )}
                </section>
              </div>
            </aside>
          ) : null}
        </div>
      </div>

      {modal ? (
        <ExternalTriggerFormModal
          modal={modal}
          form={form}
          formError={formError}
          saving={saving}
          pipelineOptions={pipelineOptions}
          scopeOptions={scopeOptions}
          runGroupOptions={runGroupOptions}
          callerDraft={callerDraft}
          activeCallerOptions={activeCallerOptions}
          onClose={closeModal}
          onSubmit={saveTrigger}
          onFormChange={patch => setForm(current => ({ ...current, ...patch }))}
          onPipelineChange={value => {
            const pipeline = normalizeIdentifier(value);
            const pipelineParent = parentPathFromIdentifier(pipeline);
            const defaultRunGroup = pipelineParent && runGroups.includes(pipelineParent) ? pipelineParent : 'root';
            setForm(current => ({
              ...current,
              pipeline,
              runGroupPath: current.runGroupPath && current.runGroupPath !== 'root'
                ? current.runGroupPath
                : defaultRunGroup,
            }));
          }}
          onCallerTypeChange={type => {
            const first = callerOptions[type]?.[0]?.value || '';
            setCallerDraft({ type, id: first });
          }}
          onCallerIDChange={id => setCallerDraft(current => ({ ...current, id }))}
          onAddCaller={addAllowedCaller}
          onRemoveCaller={removeAllowedCaller}
        />
      ) : null}
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-[var(--border-primary)] px-3 py-2">
      <dt className="text-xs text-[var(--text-secondary)]">{label}</dt>
      <dd className="text-sm text-[var(--text-primary)] truncate" title={value}>{value || '-'}</dd>
    </div>
  );
}

function normalizeIdentifier(value?: string) {
  return String(value || '')
    .trim()
    .replace(/^\.nopsai\//i, '')
    .replace(/^(pipelines|external-triggers)\//i, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\/+|\/+$/g, '');
}

function externalTriggerIDFromPath(pathname: string) {
  const parts = pathname.split('/').filter(Boolean);
  if (parts[0] !== 'external-triggers' || parts.length < 2) return '';
  return parts.slice(1).map(segment => {
    try {
      return decodeURIComponent(segment);
    } catch {
      return segment;
    }
  }).join('/');
}

function encodeRouteIdentifier(identifier: string) {
  return identifier.split('/').filter(Boolean).map(segment => encodeURIComponent(segment)).join('/');
}

function normalizeScopeOption(value?: string) {
  const normalized = normalizeIdentifier(value);
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

function uniqueRunGroupOptions(values: string[]) {
  return uniqueSortedStrings(['root', ...values.map(normalizeIdentifier).filter(Boolean)]);
}

function parentPathFromIdentifier(identifier?: string) {
  const parts = normalizeIdentifier(identifier).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function readPerMinute(rateLimit?: Record<string, unknown>) {
  const value = rateLimit?.per_minute ?? rateLimit?.requests_per_minute ?? rateLimit?.invocations_per_minute;
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return String(value);
  if (typeof value === 'string') return value;
  return '';
}

function uniqueSortedStrings(values: string[]) {
  return Array.from(new Set(values.map(value => String(value || '').trim()).filter(value => value.length > 0 || value === '')))
    .sort((a, b) => {
      if (a === 'root') return -1;
      if (b === 'root') return 1;
      if (a === '') return -1;
      if (b === '') return 1;
      return a.localeCompare(b);
    });
}

function identityLabel(primary?: string, secondary?: string, fallback?: string) {
  const main = String(primary || fallback || '').trim();
  const detail = String(secondary || '').trim();
  if (main && detail && main !== detail) return `${main} (${detail})`;
  return main || detail || String(fallback || '').trim();
}

export default ExternalTriggersPage;
