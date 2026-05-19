import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, GitBranch, Plus, RefreshCw, Trash2, Users, X } from 'lucide-react';

import { buildApiUrl } from '../lib/api';
import { buildResourceGroupPaths, type ResourceGroup } from '../lib/resourceGroups';

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
  visibility: 'group' | 'restricted' | 'workspace';
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

type ResourceAccessCardProps = {
  resourceType: 'pipeline' | 'scope' | 'step' | 'runner' | 'config_repo' | 'knowledge_context';
  resourceID: string;
  label: string;
  sensitive?: boolean;
  buttonClassName?: string;
  onAccessChange?: (access: ResourceAccess) => void;
};

type GroupOption = {
  id: string;
  name: string;
};

const visibilityOptions = [
  { value: 'group', label: 'Only this group', description: 'Use stays inside the resource group unless specific grants exist.' },
  { value: 'restricted', label: 'This group and selected groups or repositories', description: 'Keep same-group use and add explicit sharing.' },
  { value: 'workspace', label: 'Public', description: 'Any authorized user can use it.' },
] as const;

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
  return `${resourceType}.use`;
}

function subjectLabel(grant: AccessGrant) {
  const display = grant.subject_display || grant.subject_id;
  if (grant.subject_type === 'group') return `Group ${display}`;
  if (grant.subject_type === 'auth_group') return `Group ${display}`;
  if (grant.subject_type === 'repository') return `Repository ${display}`;
  if (grant.subject_type === 'service_account') return `Service account ${display}`;
  if (grant.subject_type === 'trigger') return `Trigger ${display}`;
  return display;
}

function grantSourceLabel(grant: AccessGrant) {
  const role = grant.role === 'use' ? 'Use access' : grant.role;
  const inherited = grant.inherited_from_resource ? `Inherited from ${grant.inherited_from_resource}` : '';
  const source = (grant.managed_by_config_repo || grant.source === 'gitops')
    ? grant.config_source_path
      ? `GitOps: ${grant.config_source_path}`
      : 'GitOps'
    : 'UI';
  return [role, inherited, source].filter(Boolean).join(' · ');
}

async function readResponseError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

export default function ResourceAccessCard({ resourceType, resourceID, label, sensitive = false, buttonClassName = 'glass-button-ghost', onAccessChange }: ResourceAccessCardProps) {
  const [open, setOpen] = useState(false);
  const [access, setAccess] = useState<ResourceAccess | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [subjectType, setSubjectType] = useState<'repository' | 'group'>('repository');
  const [subjectID, setSubjectID] = useState('');
  const [groups, setGroups] = useState<GroupOption[]>([]);
  const [groupsLoading, setGroupsLoading] = useState(false);

  const endpoint = useMemo(() => encodeResourcePath(resourceType, resourceID), [resourceType, resourceID]);
  const grants = access?.use_access?.grants || [];
  const showGrantControls = access?.visibility === 'restricted' || grants.length > 0;

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
      const response = await fetch(buildApiUrl(`${endpoint}/access`), { cache: 'no-store' });
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

  const loadGroups = useCallback(async () => {
    setGroupsLoading(true);
    try {
      const response = await fetch(buildApiUrl('/v1/groups'), { cache: 'no-store' });
      if (!response.ok) {
        setGroups([]);
        return;
      }
      const payload = await response.json();
      const paths = Array.isArray(payload) ? buildResourceGroupPaths(payload as ResourceGroup[]) : [];
      setGroups(paths.map(path => ({ id: path, name: `/${path}` })));
    } finally {
      setGroupsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    void loadAccess();
    void loadGroups();
  }, [loadAccess, loadGroups, open]);

  useEffect(() => {
    if (subjectType !== 'group') return;
    if (subjectID) return;
    if (groups.length) setSubjectID(groups[0].id);
  }, [groups, subjectID, subjectType]);

  const updateVisibility = useCallback(
    async (visibility: ResourceAccess['visibility']) => {
      if (!access || saving) return;
      setSaving(true);
      setError(null);
      try {
        const response = await fetch(buildApiUrl(`${endpoint}/access`), {
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
      const response = await fetch(buildApiUrl(`${endpoint}/grants`), {
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
        const response = await fetch(buildApiUrl(`${endpoint}/grants/${encodeURIComponent(grantID)}`), { method: 'DELETE' });
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

  return (
    <>
      <button className={buttonClassName} type="button" onClick={() => setOpen(true)} title="Access">
        <Users className="h-4 w-4" />
        <span>{access?.access_overridden ? 'Access overridden' : 'Access'}</span>
      </button>

      {open ? (
        <div id="resource-access-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show px-4 py-6">
          <div className="pipelines-modal-card max-w-2xl w-full overflow-hidden">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Access</p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Who can use this {label}?</h3>
              </div>
              <div className="flex items-center gap-2">
                <button className="glass-button-ghost" type="button" onClick={() => void loadAccess()} disabled={loading || saving} title="Refresh access">
                  <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
                </button>
                <button className="glass-button-ghost" type="button" onClick={() => setOpen(false)} disabled={saving}>
                  <X className="h-4 w-4" />
                  <span>Close</span>
                </button>
              </div>
            </header>

            <div className="pipelines-modal-body space-y-5 max-h-[calc(100vh-180px)] overflow-auto">
              {error ? <p className="text-sm text-red-500 whitespace-pre-wrap">{error}</p> : null}
              {!access && !error ? <p className="text-sm text-[var(--text-secondary)]">Loading access…</p> : null}

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
                          <span className="block text-xs text-[var(--text-secondary)]">{option.description}</span>
                        </span>
                      </label>
                    );
                  })}
                </div>

                {showGrantControls ? (
                  <div className="space-y-3">
                    <div>
                      <p className="text-sm font-semibold text-[var(--text-primary)]">Groups and repositories</p>
                      <p className="text-xs text-[var(--text-secondary)] mt-1">Use access only. Manage access stays with owners.</p>
                    </div>

                    {grants.length ? (
                      <ul className="space-y-2">
                        {grants.map(grant => (
                          <li key={grant.id} className="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2">
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium text-[var(--text-primary)]">{subjectLabel(grant)}</p>
                              <p className="flex items-center gap-1 text-xs text-[var(--text-secondary)]">
                                {(grant.managed_by_config_repo || grant.source === 'gitops') ? <GitBranch className="h-3.5 w-3.5 shrink-0" /> : null}
                                <span className="truncate">{grantSourceLabel(grant)}</span>
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
                        No specific groups or repositories yet.
                      </div>
                    )}

                    <div className="grid gap-2 sm:grid-cols-[150px_1fr_auto]">
                      <select
                        className="pipelines-input px-3 py-2 text-sm"
                        value={subjectType}
                        onChange={event => {
                          const nextType = event.target.value as 'repository' | 'group';
                          setSubjectType(nextType);
                          setSubjectID(nextType === 'group' ? groups[0]?.id || '' : '');
                        }}
                        disabled={saving}
                      >
                        <option value="repository">Repository</option>
                        <option value="group">Group</option>
                      </select>
                      {subjectType === 'group' ? (
                        <select
                          className="pipelines-input px-3 py-2 text-sm"
                          value={subjectID}
                          onChange={event => setSubjectID(event.target.value)}
                          disabled={saving || groupsLoading || groups.length === 0}
                        >
                          {groups.length ? (
                            groups.map(group => (
                              <option key={group.id} value={group.id}>
                                {group.name}
                              </option>
                            ))
                          ) : (
                            <option value="">{groupsLoading ? 'Loading groups...' : 'No groups available'}</option>
                          )}
                        </select>
                      ) : (
                        <input
                          className="pipelines-input px-3 py-2 text-sm"
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
        </div>
      ) : null}
    </>
  );
}
