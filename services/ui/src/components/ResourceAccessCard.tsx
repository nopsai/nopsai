import { useCallback, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { AlertTriangle, GitBranch, Plus, RefreshCw, Trash2, Users } from 'lucide-react';

import { apiClient } from '../lib/api';
import { GLOBAL_RESOURCE_TEAM_LABEL, fetchResourceTeamPaths, isGlobalResourceTeamPath } from '../lib/resourceTeams';
import { useDialogFocus } from './useDialogFocus';
import { WorkflowDialogCloseButton, workflowDialogOverlayClass } from './WorkflowPrimitives';

type AccessGrant = {
  id: string;
  subject_type: string;
  subject_id: string;
  subject_display?: string;
  role: string;
  source?: string;
  managed_by_config_repo?: boolean;
  config_source_path?: string;
  inherited_from_resource?: string;
};

type ResourceAccess = {
  resource: string;
  resource_type: string;
  resource_id: string;
  visibility: 'team' | 'restricted' | 'workspace';
  use_access?: {
    mode?: string;
    grants?: AccessGrant[];
  };
  manage_access?: {
    mode?: string;
  };
  access_overridden?: boolean;
  overridden_by?: string;
  overridden_at?: string;
};

export type ResourceAccessResourceType =
  | 'pipeline'
  | 'dashboard'
  | 'scope'
  | 'step'
  | 'runner'
  | 'config_repo'
  | 'knowledge_context'
  | 'model'
  | 'agent_role'
  | 'mcp_server'
  | 'mcp_profile';

type ResourceAccessCardProps = {
  resourceType: ResourceAccessResourceType;
  resourceID: string;
  label: string;
  sensitive?: boolean;
  buttonClassName?: string;
  iconOnly?: boolean;
  onAccessChange?: (access: ResourceAccess) => void;
  onDialogClose?: () => void;
};

type TeamOption = {
  id: string;
  name: string;
};

type ServiceAccountOption = {
  id: string;
  sub: string;
  email?: string;
  status?: string;
};

type GrantSubjectType = 'repository' | 'team' | 'service_account';

const visibilityOptions = [
  { value: 'team', label: 'Only this team', description: 'Use stays inside the resource team unless specific grants exist.' },
  { value: 'restricted', label: 'This team and selected subjects', description: 'Keep same-team use and add explicit sharing.' },
  { value: 'workspace', label: 'Public', description: 'Any authorized user can use it.' },
] as const;

function visibilityDescription(value: (typeof visibilityOptions)[number]['value'], resourceType: ResourceAccessResourceType) {
  if (resourceType !== 'dashboard') {
    return visibilityOptions.find(option => option.value === value)?.description || '';
  }
  switch (value) {
    case 'team':
      return 'Viewing stays inside the dashboard team unless specific grants exist.';
    case 'restricted':
      return 'Keep same-team viewing and add explicit sharing.';
    case 'workspace':
      return 'Any authorized user can view it.';
    default:
      return '';
  }
}

function encodeResourcePath(resourceType: string, resourceID: string) {
  const idPath = resourceID
    .split('/')
    .map(part => encodeURIComponent(part))
    .join('/');
  return `/v1/resources/${encodeURIComponent(resourceType)}/${idPath}`;
}

function defaultUseAction(resourceType: ResourceAccessCardProps['resourceType']) {
  if (resourceType === 'config_repo') return 'config_repo.use';
  if (resourceType === 'knowledge_context') return 'knowledge_context.use';
  if (resourceType === 'dashboard') return 'dashboard.read';
  return `${resourceType}.use`;
}

function subjectLabel(grant: AccessGrant) {
  const display = grant.subject_display || grant.subject_id;
  if (grant.subject_type === 'team' && isGlobalResourceTeamPath(display)) return GLOBAL_RESOURCE_TEAM_LABEL;
  if (grant.subject_type === 'team') return `Team ${display}`;
  if (grant.subject_type === 'auth_team') return `Team ${display}`;
  if (grant.subject_type === 'repository') return `Repository ${display}`;
  if (grant.subject_type === 'service_account') return `Service account ${display}`;
  if (grant.subject_type === 'trigger') return `Trigger ${display}`;
  return display;
}

function grantSourceLabel(grant: AccessGrant, resourceType: ResourceAccessResourceType) {
  const role = grant.role === 'use' ? (resourceType === 'dashboard' ? 'View access' : 'Use access') : grant.role;
  const inherited = grant.inherited_from_resource ? `Inherited from ${grant.inherited_from_resource}` : '';
  const source = (grant.managed_by_config_repo || grant.source === 'gitops')
    ? grant.config_source_path
      ? `GitOps: ${grant.config_source_path}`
      : 'GitOps'
    : 'UI';
  return [role, inherited, source].filter(Boolean).join(' · ');
}

function normalizeServiceAccountOptions(payload: unknown): ServiceAccountOption[] {
  const records = Array.isArray(payload) ? payload : [];
  return records
    .map(record => {
      if (!record || typeof record !== 'object') return null;
      const entry = record as Record<string, unknown>;
      const sub = typeof entry.sub === 'string' ? entry.sub.trim() : '';
      if (!sub) return null;
      return {
        id: typeof entry.id === 'string' ? entry.id : sub,
        sub,
        email: typeof entry.email === 'string' ? entry.email : '',
        status: typeof entry.status === 'string' ? entry.status : '',
      };
    })
    .filter(Boolean) as ServiceAccountOption[];
}

async function readResponseError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

export default function ResourceAccessCard({
  resourceType,
  resourceID,
  label,
  sensitive = false,
  buttonClassName = 'glass-button-ghost',
  iconOnly = false,
  onAccessChange,
  onDialogClose,
}: ResourceAccessCardProps) {
  const [open, setOpen] = useState(false);
  const [access, setAccess] = useState<ResourceAccess | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [subjectType, setSubjectType] = useState<GrantSubjectType>('repository');
  const [subjectID, setSubjectID] = useState('');
  const [teams, setTeams] = useState<TeamOption[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(false);
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccountOption[]>([]);
  const [serviceAccountsLoading, setServiceAccountsLoading] = useState(false);

  const endpoint = useMemo(() => encodeResourcePath(resourceType, resourceID), [resourceType, resourceID]);
  const grants = access?.use_access?.grants || [];
  const showGrantControls = access?.visibility === 'restricted' || grants.length > 0;
  const portalHost = typeof document === 'undefined' ? null : document.body;
  const accessVerb = resourceType === 'dashboard' ? 'view' : 'use';
  const accessNoun = resourceType === 'dashboard' ? 'View' : 'Use';
  const closeDialog = useCallback(() => {
    setOpen(false);
    onDialogClose?.();
  }, [onDialogClose]);
  const dialogRef = useDialogFocus(closeDialog, open);

  const loadAccess = useCallback(async () => {
    if (!resourceID.trim()) {
      setAccess(null);
      setError(`Access settings are not available for this ${label}.`);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const response = await apiClient.fetch(`${endpoint}/access`, { cache: 'no-store' });
      if (!response.ok) {
        throw new Error(await readResponseError(response, `Access settings unavailable (${response.status})`));
      }
      const payload = await response.json();
      setAccess(payload);
      onAccessChange?.(payload);
    } catch (err) {
      setAccess(null);
      setError(err instanceof Error ? err.message : 'Access settings unavailable');
    } finally {
      setLoading(false);
    }
  }, [endpoint, label, onAccessChange, resourceID]);

  const loadTeams = useCallback(async () => {
    setTeamsLoading(true);
    try {
      const paths = await fetchResourceTeamPaths();
      setTeams(paths.map(path => ({
        id: path,
        name: isGlobalResourceTeamPath(path) ? GLOBAL_RESOURCE_TEAM_LABEL : `/${path}`,
      })));
    } finally {
      setTeamsLoading(false);
    }
  }, []);

  const loadServiceAccounts = useCallback(async () => {
    setServiceAccountsLoading(true);
    try {
      const response = await apiClient.fetch('/v1/admin/service-accounts', { cache: 'no-store' });
      if (!response.ok) {
        setServiceAccounts([]);
        return;
      }
      const payload = await response.json();
      setServiceAccounts(normalizeServiceAccountOptions(payload));
    } finally {
      setServiceAccountsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    void loadAccess();
    void loadTeams();
    void loadServiceAccounts();
  }, [loadAccess, loadTeams, loadServiceAccounts, open]);

  useEffect(() => {
    if (subjectType !== 'team') return;
    if (subjectID) return;
    if (teams.length) setSubjectID(teams[0].id);
  }, [teams, subjectID, subjectType]);

  const updateVisibility = useCallback(
    async (visibility: ResourceAccess['visibility']) => {
      if (!access || saving) return;
      setSaving(true);
      setError(null);
      try {
        const response = await apiClient.fetch(`${endpoint}/access`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ visibility }),
        });
        if (!response.ok) {
          throw new Error(await readResponseError(response, `Unable to update access (${response.status})`));
        }
        const payload = await response.json();
        setAccess(payload);
        onAccessChange?.(payload);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to update access');
      } finally {
        setSaving(false);
      }
    },
    [access, endpoint, onAccessChange, saving]
  );

  const addGrant = useCallback(async () => {
    const trimmedSubject = subjectID.trim();
    if (!trimmedSubject || saving) return;
    setSaving(true);
    setError(null);
    try {
      const response = await apiClient.fetch(`${endpoint}/grants`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          subject_type: subjectType,
          subject_id: trimmedSubject,
          actions: [defaultUseAction(resourceType)],
          conditions: { branches: [], events: [] },
        }),
      });
      if (!response.ok) {
        throw new Error(await readResponseError(response, `Unable to grant access (${response.status})`));
      }
      setSubjectID('');
      await loadAccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to grant access');
    } finally {
      setSaving(false);
    }
  }, [endpoint, loadAccess, resourceType, saving, subjectID, subjectType]);

  const deleteGrant = useCallback(
    async (grantID: string) => {
      if (saving) return;
      setSaving(true);
      setError(null);
      try {
        const response = await apiClient.fetch(`${endpoint}/grants/${encodeURIComponent(grantID)}`, { method: 'DELETE' });
        if (!response.ok) {
          throw new Error(await readResponseError(response, `Unable to remove access (${response.status})`));
        }
        await loadAccess();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unable to remove access');
      } finally {
        setSaving(false);
      }
    },
    [endpoint, loadAccess, saving]
  );

  const openerLabel = access?.access_overridden ? 'Access overridden' : 'Access';

  return (
    <>
      <button
        className={buttonClassName}
        type="button"
        onClick={() => setOpen(true)}
        title={openerLabel}
        aria-label={iconOnly ? openerLabel : undefined}
      >
        <Users className="h-4 w-4" aria-hidden="true" />
        <span className={iconOnly ? 'sr-only' : undefined}>{openerLabel}</span>
      </button>

      {open && portalHost ? createPortal(
        <div
          id="resource-access-modal"
          className={workflowDialogOverlayClass}
          onPointerDown={event => {
            if (!saving && event.target === event.currentTarget) closeDialog();
          }}
        >
          <div
            ref={dialogRef}
            className="pipelines-modal-card workflow-dialog--wide w-full"
            role="dialog"
            aria-modal="true"
            aria-labelledby="resource-access-title"
            tabIndex={-1}
          >
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Access</p>
                <h3 id="resource-access-title" className="text-lg font-semibold text-[var(--text-primary)]">Who can {accessVerb} this {label}?</h3>
              </div>
              <div className="flex items-center gap-1">
                <button
                  className="workflow-dialog-close"
                  type="button"
                  onClick={() => void loadAccess()}
                  disabled={loading || saving}
                  aria-label="Refresh access"
                  title="Refresh access"
                >
                  <RefreshCw className={loading ? 'animate-spin' : ''} />
                </button>
                <WorkflowDialogCloseButton onClose={closeDialog} disabled={saving} initialFocus />
              </div>
            </header>

            <div className="pipelines-modal-body space-y-5">
              {error ? <p className="text-sm text-red-500 whitespace-pre-wrap" role="alert">{error}</p> : null}
              {!access && !error ? <p className="text-sm text-[var(--text-secondary)]" role="status">Loading access…</p> : null}

              {access ? (
                <>
                {access.access_overridden ? (
                  <div className="flex items-start gap-2 rounded-md border border-amber-300/70 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                    <span>
                      Access was changed in the UI
                      {access.overridden_by ? ` by ${access.overridden_by}` : ''}. The next GitOps sync will restore configured access.
                    </span>
                  </div>
                ) : null}

                <div className="space-y-2">
                  {visibilityOptions.map(option => {
                    const disabled = saving || (option.value === 'workspace' && sensitive);
                    return (
                      <label
                        key={option.value}
                        className={`flex items-start gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-sm ${
                          disabled ? 'opacity-60' : ''
                        }`}
                      >
                        <input
                          type="radio"
                          className="mt-0.5 h-4 w-4 accent-[var(--accent-primary)]"
                          checked={access.visibility === option.value}
                          disabled={disabled}
                          onChange={() => void updateVisibility(option.value)}
                        />
                        <span className="min-w-0">
                          <span className="block text-[var(--text-primary)]">{option.label}</span>
                          <span className="block text-xs text-[var(--text-secondary)]">{visibilityDescription(option.value, resourceType)}</span>
                        </span>
                      </label>
                    );
                  })}
                </div>

                {showGrantControls ? (
                  <div className="space-y-3">
                    <div>
                      <p className="text-sm font-semibold text-[var(--text-primary)]">Teams, repositories, and service accounts</p>
                      <p className="text-xs text-[var(--text-secondary)] mt-1">{accessNoun} access only. Manage access stays with owners.</p>
                    </div>

                    {grants.length ? (
                      <ul className="space-y-2">
                        {grants.map(grant => (
                          <li key={grant.id} className="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2">
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium text-[var(--text-primary)]">{subjectLabel(grant)}</p>
                              <p className="flex items-center gap-1 text-xs text-[var(--text-secondary)]">
                                {(grant.managed_by_config_repo || grant.source === 'gitops') ? <GitBranch className="h-3.5 w-3.5 shrink-0" /> : null}
                                <span className="truncate">{grantSourceLabel(grant, resourceType)}</span>
                              </p>
                            </div>
                            <button
                              className="glass-button-ghost"
                              type="button"
                              onClick={() => void deleteGrant(grant.id)}
                              disabled={saving || Boolean(grant.inherited_from_resource)}
                              title={grant.inherited_from_resource ? 'Inherited access must be changed on its source resource' : 'Remove access'}
                            >
                              <Trash2 className="h-4 w-4" />
                            </button>
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div className="rounded-md border border-dashed border-[var(--border-primary)] p-3 text-sm text-[var(--text-secondary)]">
                        No specific teams, repositories, or service accounts yet.
                      </div>
                    )}

                    <div className="grid gap-2 sm:grid-cols-[150px_1fr_auto]">
                      <select
                        className="pipelines-input px-3 py-2 text-sm"
                        aria-label="Grant subject type"
                        value={subjectType}
                        onChange={event => {
                          const nextType = event.target.value as GrantSubjectType;
                          setSubjectType(nextType);
                          setSubjectID(nextType === 'team' ? teams[0]?.id || '' : '');
                        }}
                        disabled={saving}
                      >
                        <option value="repository">Repository</option>
                        <option value="team">Team</option>
                        <option value="service_account">Service account</option>
                      </select>
                      {subjectType === 'team' ? (
                        <select
                          className="pipelines-input px-3 py-2 text-sm"
                          aria-label="Grant team"
                          value={subjectID}
                          onChange={event => setSubjectID(event.target.value)}
                          disabled={saving || teamsLoading || teams.length === 0}
                        >
                          {teams.length ? (
                            teams.map(team => (
                              <option key={team.id} value={team.id}>
                                {team.name}
                              </option>
                            ))
                          ) : (
                            <option value="">{teamsLoading ? 'Loading teams...' : 'No teams available'}</option>
                          )}
                        </select>
                      ) : subjectType === 'service_account' ? (
                        <>
                          <input
                            className="pipelines-input px-3 py-2 text-sm"
                            aria-label="Grant service account"
                            value={subjectID}
                            onChange={event => setSubjectID(event.target.value)}
                            placeholder={serviceAccountsLoading ? 'Loading service accounts...' : 'servicenow-prod'}
                            list="resource-access-service-accounts"
                            disabled={saving}
                          />
                          <datalist id="resource-access-service-accounts">
                            {serviceAccounts.map(account => (
                              <option key={account.id || account.sub} value={account.sub}>
                                {account.email ? `${account.sub} (${account.email})` : account.sub}
                              </option>
                            ))}
                          </datalist>
                        </>
                      ) : (
                        <input
                          className="pipelines-input px-3 py-2 text-sm"
                          aria-label="Grant repository"
                          value={subjectID}
                          onChange={event => setSubjectID(event.target.value)}
                          placeholder="owner/repo"
                          disabled={saving}
                        />
                      )}
                      <button className="glass-button-primary" type="button" onClick={() => void addGrant()} disabled={saving || !subjectID.trim()}>
                        <Plus className="h-4 w-4" />
                        <span>Add</span>
                      </button>
                    </div>
                  </div>
                ) : null}

                <div className="flex items-center gap-2 rounded-md bg-[var(--bg-tertiary)] px-3 py-2 text-xs text-[var(--text-secondary)]">
                  <Users className="h-4 w-4 shrink-0" />
                  <span>Manage access: Owners</span>
                </div>
                </>
              ) : null}
            </div>
          </div>
        </div>,
        portalHost
      ) : null}
    </>
  );
}
