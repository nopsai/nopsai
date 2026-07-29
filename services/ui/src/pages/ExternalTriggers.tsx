import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { EventAutomationToolbar } from '../features/event-automation/EventAutomationToolbar';
import { ExternalTriggerFormModal } from '../features/external-triggers/ExternalTriggerFormModal';
import { ExternalTriggerMetricGrid, ExternalTriggerWorkspace } from '../features/external-triggers/ExternalTriggerWorkspace';
import {
  buildExternalTriggerCollectionMetrics,
  buildExternalTriggerTreeItems,
  externalTriggerBelongsToTeam,
  externalTriggerTeamPath,
  filterExternalTriggers,
  type AllowedCaller,
  type ExternalTrigger,
  type ExternalTriggerForm,
  type ExternalTriggerInvocation,
  type ExternalTriggerModalState,
  type SelectOption,
} from '../features/external-triggers/model';
import { apiClient, buildApiUrl } from '../lib/api';
import { fetchPipelineRunTeamPaths } from '../lib/resourceTeams';

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

type TeamListItem = {
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
  runTeamPath: 'root',
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
  const [runTeams, setRunTeams] = useState<string[]>([]);
  const [users, setUsers] = useState<UserListItem[]>([]);
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccountListItem[]>([]);
  const [teams, setTeams] = useState<TeamListItem[]>([]);
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
  const [activeTeamPath, setActiveTeamPath] = useState('');
  const [callerDraft, setCallerDraft] = useState<AllowedCaller>({ type: 'service_account', id: '' });

  const selectedTrigger = useMemo(
    () => selected || triggers.find(trigger => trigger.id === selectedID) || null,
    [selected, selectedID, triggers]
  );

  const filteredTriggers = useMemo(() => {
    return filterExternalTriggers(triggers, searchTerm);
  }, [searchTerm, triggers]);

  const workspaceTeamPath = selectedTrigger
    ? externalTriggerTeamPath(selectedTrigger.run_team_path)
    : activeTeamPath;

  const visibleTriggers = useMemo(() => {
    if (searchTerm.trim()) return filteredTriggers;
    return filteredTriggers.filter(trigger => externalTriggerBelongsToTeam(trigger, workspaceTeamPath));
  }, [filteredTriggers, searchTerm, workspaceTeamPath]);

  const treeItems = useMemo(() => buildExternalTriggerTreeItems(triggers), [triggers]);
  const metrics = useMemo(() => buildExternalTriggerCollectionMetrics(triggers), [triggers]);

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

  const runTeamOptions = useMemo(
    () => uniqueRunTeamOptions(runTeams),
    [runTeams]
  );
  const selectedRunTeamPath = useMemo(() => {
    const normalized = normalizeIdentifier(form.runTeamPath);
    return runTeamOptions.includes(normalized) ? normalized : 'root';
  }, [form.runTeamPath, runTeamOptions]);

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
      auth_team: teams
        .map(team => ({ value: team.id || team.name || '', label: team.name || team.id || '' }))
        .filter(option => Boolean(option.value)),
    }),
    [teams, serviceAccounts, users]
  );

  const activeCallerOptions = callerOptions[callerDraft.type] || [];

  useEffect(() => {
    if (!modal || form.runTeamPath === selectedRunTeamPath) return;
    setForm(current => ({ ...current, runTeamPath: selectedRunTeamPath }));
  }, [form.runTeamPath, modal, selectedRunTeamPath]);

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
    const [pipelineData, runtimeScopeData, secretScopeData, variableScopeData, runTeamData, userData, serviceAccountData, teamData] = await Promise.all([
      fetchJson<Array<string | PipelineListItem>>('/v1/pipelines?include_source=true').catch(() => []),
      fetchJson<Array<string | ScopeListItem>>('/v1/system/dispatcher/scopes').catch(() => []),
      fetchJson<Array<string | ScopeListItem>>('/v1/secrets/scopes').catch(() => []),
      fetchJson<Array<string | ScopeListItem>>('/v1/variables/scopes').catch(() => []),
      fetchPipelineRunTeamPaths().catch(() => []),
      fetchJson<UserListItem[]>('/v1/admin/users').catch(() => []),
      fetchJson<ServiceAccountListItem[]>('/v1/admin/service-accounts').catch(() => []),
      fetchJson<TeamListItem[]>('/v1/access/auth-teams').catch(() => []),
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
    setRunTeams(uniqueRunTeamOptions((Array.isArray(runTeamData) ? runTeamData : []).map(normalizeIdentifier)));
    setUsers(Array.isArray(userData) ? userData : []);
    setServiceAccounts(Array.isArray(serviceAccountData) ? serviceAccountData : []);
    setTeams(Array.isArray(teamData) ? teamData : []);
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
    const selectedTriggerFromList = triggers.find(trigger => trigger.id === id);
    if (selectedTriggerFromList) {
      setActiveTeamPath(externalTriggerTeamPath(selectedTriggerFromList.run_team_path));
    }
    navigate(id ? `/external-triggers/${encodeRouteIdentifier(id)}` : '/external-triggers');
  }, [navigate, triggers]);

  const openTeam = useCallback((path: string) => {
    setActiveTeamPath(externalTriggerTeamPath(path));
    setSelectedID('');
    setSelected(null);
    setInvocations([]);
    navigate('/external-triggers');
  }, [navigate]);

  const openCreate = () => {
    if (!canWriteExternalTriggers) return;
    const pipeline = pipelines[0] || '';
    const pipelineParent = parentPathFromIdentifier(pipeline);
    const defaultRunTeam = pipelineParent && runTeams.includes(pipelineParent) ? pipelineParent : 'root';
    setForm({ ...emptyForm, pipeline, runTeamPath: defaultRunTeam });
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
      runTeamPath: normalizeIdentifier(trigger.run_team_path) || 'root',
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
      run_team_path: normalizeIdentifier(form.runTeamPath) || 'root',
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
    if (trigger.managed_by_config_repo && !window.confirm(
      'This external trigger is managed by GitOps. Saving here creates a database override that the next GitOps sync can replace unless it is pushed to GitOps. Continue?'
    )) {
      return;
    }
    const payload = {
      id: trigger.id,
      name: trigger.name,
      description: trigger.description || '',
      pipeline: trigger.pipeline,
      scope: normalizeScopeOption(trigger.scope),
      run_team_path: normalizeIdentifier(trigger.run_team_path) || 'root',
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
    const message = trigger.managed_by_config_repo
      ? `Delete external trigger ${trigger.id}? This removes the database row; the next GitOps sync can recreate it from the repository.`
      : `Delete external trigger ${trigger.id}?`;
    if (!window.confirm(message)) return;
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
      <EventAutomationToolbar
        active="external-triggers"
        searchLabel="Search external triggers"
        searchTerm={searchTerm}
        canCreate={canWriteExternalTriggers}
        createLabel="New trigger"
        createDisabledReason="You have read-only access to external triggers"
        showCreateWhenDisabled
        onSearchTermChange={setSearchTerm}
        onCreate={openCreate}
        filters={!canWriteExternalTriggers ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
        summary={<ExternalTriggerMetricGrid metrics={metrics} />}
      />
      <div className="flex-1 overflow-auto px-4 pb-6 triggers-content">
        {error && <div className="dispatcher-error mb-4">{error}</div>}
        <ExternalTriggerWorkspace
          visibleTriggers={visibleTriggers}
          treeItems={treeItems}
          activeTeamPath={workspaceTeamPath}
          selectedID={selectedID}
          selectedTrigger={selectedTrigger}
          invocations={invocations}
          loading={loading}
          detailLoading={detailLoading}
          invocationsLoading={invocationsLoading}
          canWrite={canWriteExternalTriggers}
          canDelete={canDeleteExternalTriggers}
          deletePending={deletePending}
          copyState={copyState}
          invokeURL={invokeURL}
          exampleCurl={exampleCurl}
          searchTerm={searchTerm}
          onOpenTeam={openTeam}
          onSelect={selectTrigger}
          onEdit={openEdit}
          onToggle={trigger => void updateTrigger(trigger, { enabled: !trigger.enabled })}
          onCopyURL={() => void copyText('url', invokeURL)}
          onCopyCurl={() => void copyText('curl', exampleCurl)}
          onDelete={trigger => void deleteTrigger(trigger)}
          onRefreshInvocations={id => void loadInvocations(id)}
        />
      </div>

      {modal ? (
        <ExternalTriggerFormModal
          modal={modal}
          form={form}
          formError={formError}
          saving={saving}
          gitOpsManaged={Boolean(modal.mode === 'edit' && modal.trigger?.managed_by_config_repo)}
          pipelineOptions={pipelineOptions}
          scopeOptions={scopeOptions}
          runTeamOptions={runTeamOptions}
          callerDraft={callerDraft}
          activeCallerOptions={activeCallerOptions}
          onClose={closeModal}
          onSubmit={saveTrigger}
          onFormChange={patch => setForm(current => ({ ...current, ...patch }))}
          onPipelineChange={value => {
            const pipeline = normalizeIdentifier(value);
            const pipelineParent = parentPathFromIdentifier(pipeline);
            const defaultRunTeam = pipelineParent && runTeams.includes(pipelineParent) ? pipelineParent : 'root';
            setForm(current => ({
              ...current,
              pipeline,
              runTeamPath: current.runTeamPath && current.runTeamPath !== 'root'
                ? current.runTeamPath
                : defaultRunTeam,
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

function uniqueRunTeamOptions(values: string[]) {
  return uniqueSortedStrings(['root', ...values.map(normalizeIdentifier).filter(Boolean)]);
}

function parentPathFromIdentifier(identifier?: string) {
  const parts = normalizeIdentifier(identifier).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
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
