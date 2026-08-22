import { useEffect, useRef, useState } from 'react';
import type { Dispatch, FormEvent, SetStateAction } from 'react';
import { Plus, Trash2, X, XCircle } from 'lucide-react';
import {
  formatConfigRepoTimestamp,
  isAppTeam,
  teamDisplayName,
  teamRepositoryURL,
  type Team,
} from '../../lib/teamModels';
import { CONFIG_REPOSITORY_PROVIDER_OPTIONS, type ConfigRepositoryProvider } from '../../lib/configRepositoryProviders.js';
import type { TeamParentOption } from './model';
import {
  WorkflowDialogCloseButton,
  workflowDialogOverlayClass,
} from '../../components/WorkflowPrimitives';
import {
  NOTIFICATION_EVENTS,
  teamNotificationGitOpsTarget,
  notificationRouteFormAddRoute,
  notificationRouteFormRemoveSelectedRoute,
  notificationRouteFormSelectRoute,
  type NotificationRouteFormState,
  type NotificationRouteRecord,
} from './notificationRoutes';
import { CredentialReferenceLink } from '../system/credentials/CredentialReferenceLink';

type ConfigRepository = {
  id: number;
  scope_type: string;
  scope_id: string;
  provider: ConfigRepositoryProvider;
  repo_url: string;
  branch: string;
  base_path: string;
  credential_ref: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
  last_sync_status: string;
  last_sync_message?: string;
  last_sync_started_at?: string;
  last_sync_completed_at?: string;
  last_sync_commit_sha?: string;
};

type ConfigRepositoryFormState = {
  provider: ConfigRepositoryProvider;
  repo_url: string;
  branch: string;
  base_path: string;
  credential_ref: string;
  enabled: boolean;
  write_enabled: boolean;
  write_branch: string;
};

export type NewTeamItemKind = 'team' | 'app';

type NewTeamItemPayload = {
  kind: NewTeamItemKind;
  name: string;
  description: string;
  repoURL: string;
  parentID: number | null;
};

export type TeamItemEditPayload = {
  name: string;
  description: string;
  repoURL: string;
  parentID: number | null;
};

export type TeamSettingsTab = 'sync' | 'notifications';

export function NewTeamItemModal({
  open,
  parentLabel,
  parentOptions,
  defaultParentID,
  initialKind = 'team',
  error,
  pending,
  onClose,
  onSubmit,
}: {
  open: boolean;
  parentLabel: string;
  parentOptions: TeamParentOption[];
  defaultParentID: number | null;
  initialKind?: NewTeamItemKind;
  error: string | null;
  pending: boolean;
  onClose: () => void;
  onSubmit: (payload: NewTeamItemPayload) => Promise<void>;
}) {
  const [kind, setKind] = useState<NewTeamItemKind>(initialKind);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [repoURL, setRepoURL] = useState('');
  const [parentID, setParentID] = useState<number | null>(null);
  const nameInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (open) {
      const handle = window.setTimeout(() => {
        const defaultOptionID = parentOptions.some(option => option.id === defaultParentID) ? defaultParentID : null;
        setKind(initialKind);
        setName('');
        setDescription('');
        setRepoURL('');
        setParentID(defaultOptionID);
        requestAnimationFrame(() => nameInputRef.current?.focus());
      }, 0);
      return () => window.clearTimeout(handle);
    }
    return undefined;
  }, [defaultParentID, initialKind, open, parentOptions]);

  if (!open) return null;

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    await onSubmit({ kind, name, description, repoURL, parentID });
  };

  const selectKind = (nextKind: NewTeamItemKind) => {
    setKind(nextKind);
    if (nextKind === 'team') {
      setRepoURL('');
    } else {
      setDescription('');
    }
  };

  const selectedParentLabel = parentOptions.find(option => option.id === parentID)?.label || parentLabel || 'Global';
  const title = kind === 'app' ? 'Create Application' : 'Create Team';

  return (
    <div
      className={workflowDialogOverlayClass}
      onPointerDown={event => {
        if (!pending && event.target === event.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="team-item-modal-title"
        className="pipelines-modal-card workflow-dialog--compact w-full"
      >
        <form onSubmit={handleSubmit}>
          <header className="pipelines-modal-header">
            <div className="min-w-0">
              <p className="pipelines-modal-kicker">Parent: {selectedParentLabel}</p>
              <h3 id="team-item-modal-title" className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
            </div>
            <WorkflowDialogCloseButton onClose={onClose} disabled={pending} />
          </header>
          <div className="pipelines-modal-body space-y-4">
            <div className="modal-segmented" role="group" aria-label="Create item type">
              {(['team', 'app'] as const).map(option => (
                <button
                  key={option}
                  type="button"
                  aria-pressed={kind === option}
                  onClick={() => selectKind(option)}
                >
                  {option === 'team' ? 'Team' : 'Application'}
                </button>
              ))}
            </div>
            <div className="space-y-2">
              <label htmlFor="new-team-parent" className="text-sm font-medium text-[var(--text-primary)]">
                Parent team
              </label>
              <select
                id="new-team-parent"
                name="new-team-parent"
                value={parentID == null ? 'root' : String(parentID)}
                onChange={event => setParentID(event.target.value === 'root' ? null : Number(event.target.value))}
                className="pipelines-input w-full"
              >
                {parentOptions.map(option => (
                  <option key={option.id ?? 'root'} value={option.id == null ? 'root' : String(option.id)}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2">
              <label htmlFor="new-team-name" className="text-sm font-medium text-[var(--text-primary)]">
                {kind === 'app' ? 'Application Name' : 'Team Name'}
              </label>
              <input
                ref={nameInputRef}
                id="new-team-name"
                name="new-team-name"
                type="text"
                required
                value={name}
                onChange={event => setName(event.target.value)}
                className="pipelines-input w-full"
                placeholder={kind === 'app' ? 'service-api' : 'platform'}
              />
            </div>
            {kind === 'app' ? (
              <div className="space-y-2">
                <label htmlFor="new-team-repo-url" className="text-sm font-medium text-[var(--text-primary)]">
                  Repository URL
                </label>
                <input
                  id="new-team-repo-url"
                  name="new-team-repo-url"
                  type="text"
                  required
                  value={repoURL}
                  onChange={event => setRepoURL(event.target.value)}
                  className="pipelines-input w-full"
                  placeholder="https://github.com/acme/service-api"
                />
              </div>
            ) : (
              <div className="space-y-2">
                <label htmlFor="new-team-description" className="text-sm font-medium text-[var(--text-primary)]">
                  Description <span className="text-[var(--text-secondary)]">(optional)</span>
                </label>
                <textarea
                  id="new-team-description"
                  name="new-team-description"
                  value={description}
                  onChange={event => setDescription(event.target.value)}
                  rows={3}
                  className="pipelines-input w-full"
                  placeholder="Add a short summary for this team"
                />
              </div>
            )}
            {error && <div className="text-sm text-red-600">{error}</div>}
          </div>
          <footer className="pipelines-modal-footer">
            <div className="pipelines-modal-actions">
              <button type="button" className="glass-button-subtle" onClick={onClose} disabled={pending}>
                Cancel
              </button>
              <button type="submit" className="glass-button-primary" disabled={pending}>
                {pending ? 'Creating...' : title}
              </button>
            </div>
          </footer>
        </form>
      </div>
    </div>
  );
}

export function EditTeamItemModal({
  open,
  team,
  parentOptions,
  error,
  pending,
  onClose,
  onSubmit,
}: {
  open: boolean;
  team: Team | null;
  parentOptions: TeamParentOption[];
  error: string | null;
  pending: boolean;
  onClose: () => void;
  onSubmit: (payload: TeamItemEditPayload) => Promise<void>;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [repoURL, setRepoURL] = useState('');
  const [parentID, setParentID] = useState<number | null>(null);
  const nameInputRef = useRef<HTMLInputElement | null>(null);
  const app = team ? isAppTeam(team) : false;
  const label = team ? teamDisplayName(team) : '';
  const title = app ? 'Edit Application' : 'Edit Team';

  useEffect(() => {
    if (!open || !team) return undefined;
    const handle = window.setTimeout(() => {
      setName(teamDisplayName(team));
      setDescription(team.description || '');
      setRepoURL(team.repo_url || teamRepositoryURL(team));
      setParentID(team.parent_id ?? null);
      requestAnimationFrame(() => nameInputRef.current?.focus());
    }, 0);
    return () => window.clearTimeout(handle);
  }, [open, team]);

  if (!open || !team) return null;

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    await onSubmit({ name, description, repoURL, parentID });
  };

  return (
    <div
      className={workflowDialogOverlayClass}
      onPointerDown={event => {
        if (!pending && event.target === event.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="team-edit-modal-title"
        className="pipelines-modal-card w-full"
      >
        <form onSubmit={handleSubmit}>
          <header className="pipelines-modal-header">
            <div className="min-w-0">
              <p className="pipelines-modal-kicker">{label}</p>
              <h3 id="team-edit-modal-title" className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
            </div>
            <WorkflowDialogCloseButton onClose={onClose} disabled={pending} />
          </header>
          <div className="pipelines-modal-body space-y-4">
            <div className="space-y-2">
              <label htmlFor="edit-team-name" className="text-sm font-medium text-[var(--text-primary)]">
                {app ? 'Application Name' : 'Team Name'}
              </label>
              <input
                ref={nameInputRef}
                id="edit-team-name"
                name="edit-team-name"
                type="text"
                required
                value={name}
                onChange={event => setName(event.target.value)}
                className="pipelines-input w-full"
              />
            </div>

            <div className="space-y-2">
              <label htmlFor="edit-team-parent" className="text-sm font-medium text-[var(--text-primary)]">
                Parent team
              </label>
              <select
                id="edit-team-parent"
                name="edit-team-parent"
                value={parentID == null ? 'root' : String(parentID)}
                onChange={event => setParentID(event.target.value === 'root' ? null : Number(event.target.value))}
                className="pipelines-input w-full"
              >
                {parentOptions.map(option => (
                  <option key={option.id ?? 'root'} value={option.id == null ? 'root' : String(option.id)}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>

            {app ? (
              <div className="space-y-2">
                <label htmlFor="edit-team-repo-url" className="text-sm font-medium text-[var(--text-primary)]">
                  Repository URL
                </label>
                <input
                  id="edit-team-repo-url"
                  name="edit-team-repo-url"
                  type="text"
                  required
                  value={repoURL}
                  onChange={event => setRepoURL(event.target.value)}
                  className="pipelines-input w-full"
                  placeholder="https://github.com/acme/service-api"
                />
              </div>
            ) : (
              <div className="space-y-2">
                <label htmlFor="edit-team-description" className="text-sm font-medium text-[var(--text-primary)]">
                  Description <span className="text-[var(--text-secondary)]">(optional)</span>
                </label>
                <textarea
                  id="edit-team-description"
                  name="edit-team-description"
                  value={description}
                  onChange={event => setDescription(event.target.value)}
                  rows={3}
                  className="pipelines-input w-full"
                />
              </div>
            )}

            {error && <div className="text-sm text-red-600">{error}</div>}
          </div>
          <footer className="pipelines-modal-footer">
            <div className="pipelines-modal-actions">
              <button type="button" className="glass-button-subtle" onClick={onClose} disabled={pending}>
                Cancel
              </button>
              <button type="submit" className="glass-button-primary" disabled={pending}>
                {pending ? 'Saving...' : 'Save Changes'}
              </button>
            </div>
          </footer>
        </form>
      </div>
    </div>
  );
}

export function TeamConfigRepositoryModal({
  teamLabel,
  repo,
  form,
  loading,
  saving,
  syncing,
  error,
  driftLoading,
  notificationRoute,
  notificationForm,
  notificationLoading,
  notificationSaving,
  notificationError,
  initialTab = 'sync',
  canManage,
  canSync,
  onChange,
  onNotificationChange,
  onSave,
  onDelete,
  onSync,
  onCancelSync,
  onCheckDrift,
  onSaveNotification,
  onDeleteNotification,
  onClose,
}: {
  teamLabel: string;
  repo: ConfigRepository | null;
  form: ConfigRepositoryFormState;
  loading: boolean;
  saving: boolean;
  syncing: boolean;
  error: string | null;
  driftLoading: boolean;
  notificationRoute: NotificationRouteRecord | null;
  notificationForm: NotificationRouteFormState;
  notificationLoading: boolean;
  notificationSaving: boolean;
  notificationError: string | null;
  initialTab?: TeamSettingsTab;
  canManage: boolean;
  canSync: boolean;
  onChange: Dispatch<SetStateAction<ConfigRepositoryFormState>>;
  onNotificationChange: Dispatch<SetStateAction<NotificationRouteFormState>>;
  onSave: () => Promise<void>;
  onDelete: () => Promise<void>;
  onSync: () => Promise<void>;
  onCancelSync: () => Promise<void>;
  onCheckDrift: () => Promise<void>;
  onSaveNotification: () => Promise<void>;
  onDeleteNotification: () => Promise<void>;
  onClose: () => void;
}) {
  const inputClass = 'pipelines-input w-full text-sm disabled:cursor-not-allowed disabled:opacity-70';
  const textareaClass = `${inputClass} min-h-[104px] resize-y`;
  const checkboxClass = 'h-4 w-4 rounded border-[var(--border-primary)] text-indigo-600 focus:ring-indigo-500';
  const sectionClass = 'rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4';
  const sectionTitleClass = 'text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]';
  const fieldClass = 'flex flex-col gap-1 text-sm text-[var(--text-primary)]';
  const toggleClass = 'flex min-h-[46px] items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]';
  const isRunning = repo?.last_sync_status === 'running';
  const canEdit = canManage && !loading && !saving;
  const syncDisabled = !repo || !canSync || syncing || saving || isRunning;
  const cancelSyncDisabled = !repo || !canSync || syncing || saving || !isRunning;
  const driftDisabled = !repo || driftLoading || saving || syncing || isRunning;
  const notificationManaged = Boolean(notificationRoute?.managed_by_config_repo);
  const notificationCanEdit = canManage && !notificationLoading && !notificationSaving && !notificationManaged;
  const notificationSourceLabel = notificationManaged ? 'GitOps' : notificationRoute?.id ? 'Database' : 'Default';
  const notificationSaveDisabled = !notificationCanEdit;
  const notificationDeleteDisabled = !notificationCanEdit || !notificationRoute?.id;
  const notificationGitOpsTarget = repo ? teamNotificationGitOpsTarget(repo.base_path) : '';
  const settingsTabKey = `${teamLabel}:${initialTab}`;
  const [settingsTabState, setSettingsTabState] = useState<{ key: string; tab: TeamSettingsTab }>(() => ({
    key: settingsTabKey,
    tab: initialTab,
  }));
  const activeSettingsTab = settingsTabState.key === settingsTabKey ? settingsTabState.tab : initialTab;
  const selectSettingsTab = (tab: TeamSettingsTab) => setSettingsTabState({ key: settingsTabKey, tab });
  const settingsTabClass = (tab: TeamSettingsTab) =>
    `inline-flex min-h-[38px] items-center justify-center rounded-md px-4 py-2 text-sm font-semibold transition ${
      activeSettingsTab === tab
        ? 'bg-[var(--bg-primary)] text-[var(--text-primary)] shadow-sm border border-[var(--border-primary)]'
        : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
    }`;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] px-4"
      onPointerDown={event => {
        if (!saving && !syncing && event.target === event.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="team-settings-modal-title"
        className="w-full max-w-5xl max-h-[90vh] bg-white dark:bg-slate-900 rounded-xl shadow-xl border border-[var(--border-primary)] overflow-y-auto"
      >
        <div className="flex items-start justify-between gap-4 px-5 py-4 border-b border-[var(--border-primary)] bg-[var(--bg-secondary)]/70">
          <div>
            <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Team Settings</p>
            <h3 id="team-settings-modal-title" className="text-lg font-semibold text-[var(--text-primary)]">Config & Notifications</h3>
            <p className="text-xs text-[var(--text-secondary)] break-all">{teamLabel}</p>
          </div>
          <div className="flex items-center gap-2">
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button type="button" className="pipelines-icon-only" aria-label="Close" onClick={onClose}>
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </div>

        <div className="p-5 space-y-5">
          <div className="inline-flex rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-1" role="tablist" aria-label="Team settings sections">
            <button
              type="button"
              role="tab"
              aria-selected={activeSettingsTab === 'sync'}
              className={settingsTabClass('sync')}
              onClick={() => selectSettingsTab('sync')}
            >
              Sync config
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={activeSettingsTab === 'notifications'}
              className={settingsTabClass('notifications')}
              onClick={() => selectSettingsTab('notifications')}
            >
              Notifications
            </button>
          </div>

          {activeSettingsTab === 'sync' && (
            <div className="space-y-5" role="tabpanel" aria-label="Sync config">
              {loading ? (
                <div className="text-sm text-[var(--text-secondary)]">Loading config repository...</div>
              ) : (
                <>
              {!repo && (
                <div className="rounded-lg border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
                  No config repository connected.
                </div>
              )}

              {repo && (
                <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3">
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Provider</p>
                      <p className="font-semibold text-[var(--text-primary)]">{repo.provider}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Status</p>
                      <p className="font-semibold text-[var(--text-primary)]">{repo.last_sync_status || 'Not synced'}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Completed</p>
                      <p className="font-semibold text-[var(--text-primary)]">{formatConfigRepoTimestamp(repo.last_sync_completed_at)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Started</p>
                      <p className="font-semibold text-[var(--text-primary)]">{formatConfigRepoTimestamp(repo.last_sync_started_at)}</p>
                    </div>
                    <div>
                      <p className="text-xs text-[var(--text-secondary)]">Commit</p>
                      <p className="font-semibold text-[var(--text-primary)] truncate" title={repo.last_sync_commit_sha || ''}>
                        {repo.last_sync_commit_sha || '-'}
                      </p>
                    </div>
                  </div>
                  {repo.last_sync_message && (
                    <p className="mt-3 text-xs text-[var(--text-secondary)] break-words">{repo.last_sync_message}</p>
                  )}
                </div>
              )}

              <div className={`${sectionClass} space-y-4`}>
                <div className="grid grid-cols-1 lg:grid-cols-[minmax(180px,220px)_minmax(0,1fr)] gap-4 items-end">
                  <label htmlFor="team-config-repo-provider" className={fieldClass}>
                    <span>Provider</span>
                    <select
                      id="team-config-repo-provider"
                      value={form.provider}
                      onChange={event => onChange(prev => ({ ...prev, provider: event.target.value as ConfigRepositoryProvider }))}
                      disabled={!canEdit}
                      className={inputClass}
                    >
                      {CONFIG_REPOSITORY_PROVIDER_OPTIONS.map(option => (
                        <option key={option.value} value={option.value}>{option.label}</option>
                      ))}
                    </select>
                  </label>
                  <label htmlFor="team-config-repo-url" className={fieldClass}>
                    <span>Repository URL</span>
                    <input
                      id="team-config-repo-url"
                      type="url"
                      required={canManage}
                      value={form.repo_url}
                      onChange={event => onChange(prev => ({ ...prev, repo_url: event.target.value }))}
                      disabled={!canEdit}
                      className={inputClass}
                      placeholder="https://github.com/org/config-repo"
                    />
                  </label>
                  <label htmlFor="team-config-repo-credential-ref" className={`${fieldClass} lg:col-span-2`}>
                    <span>Credential reference</span>
                    <input
                      id="team-config-repo-credential-ref"
                      value={form.credential_ref}
                      onChange={event => onChange(prev => ({ ...prev, credential_ref: event.target.value }))}
                      disabled={!canEdit}
                      className={inputClass}
                      placeholder="credential://system/gitops/gitlab-token"
                      required={canManage && form.provider !== 'github'}
                    />
                    <CredentialReferenceLink reference={form.credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
                      Open credential
                    </CredentialReferenceLink>
                    <span className="text-xs text-[var(--text-secondary)]">Expected type: bearer_token</span>
                  </label>

                  <label htmlFor="team-config-repo-branch" className={fieldClass}>
                    <span>Branch</span>
                    <input
                      id="team-config-repo-branch"
                      value={form.branch}
                      onChange={event => onChange(prev => ({ ...prev, branch: event.target.value }))}
                      disabled={!canEdit}
                      className={inputClass}
                      placeholder="main"
                    />
                  </label>
                  <label htmlFor="team-config-repo-base-path" className={fieldClass}>
                    <span>Base path</span>
                    <input
                      id="team-config-repo-base-path"
                      value={form.base_path}
                      onChange={event => onChange(prev => ({ ...prev, base_path: event.target.value }))}
                      disabled={!canEdit}
                      className={inputClass}
                      placeholder="configs/team-1"
                    />
                  </label>
                </div>

                <label className={toggleClass}>
                  <input
                    type="checkbox"
                    className={checkboxClass}
                    checked={form.enabled}
                    onChange={event => onChange(prev => ({ ...prev, enabled: event.target.checked }))}
                    disabled={!canEdit}
                  />
                  Enabled
                </label>

                <div className="border-t border-[var(--border-primary)] pt-4">
                  <div className="grid grid-cols-1 sm:grid-cols-[minmax(0,220px)_1fr] gap-4 items-end">
                    <label className={toggleClass}>
                      <input
                        id="team-config-repo-write-enabled"
                        type="checkbox"
                        className={checkboxClass}
                        checked={form.write_enabled}
                        onChange={event => onChange(prev => ({ ...prev, write_enabled: event.target.checked }))}
                        disabled={!canEdit}
                      />
                      Enable Git push
                    </label>
                    <label htmlFor="team-config-repo-write-branch" className={fieldClass}>
                      <span>Push branch</span>
                      <input
                        id="team-config-repo-write-branch"
                        value={form.write_branch}
                        onChange={event => onChange(prev => ({ ...prev, write_branch: event.target.value }))}
                        disabled={!canEdit || !form.write_enabled}
                        className={inputClass}
                        placeholder="nopsai/ui-changes"
                      />
                    </label>
                  </div>
                </div>
              </div>
                </>
              )}

              {error && <div className="text-sm text-red-600 break-words">{error}</div>}

              <div className="flex flex-wrap items-center justify-end gap-3 pt-2">
                {repo && canManage && (
                  <button type="button" className="glass-button-danger mr-auto" onClick={onDelete} disabled={saving || syncing || isRunning}>
                    {saving ? 'Removing...' : 'Remove'}
                  </button>
                )}
                {repo && canSync && (
                  <button type="button" className="glass-button-subtle" onClick={onSync} disabled={syncDisabled}>
                    {isRunning || syncing ? 'Syncing...' : 'Sync Now'}
                  </button>
                )}
                {repo && canSync && isRunning && (
                  <button type="button" className="glass-button-danger" onClick={() => void onCancelSync()} disabled={cancelSyncDisabled}>
                    <XCircle className="h-4 w-4" />
                    {syncing ? 'Canceling...' : 'Cancel sync'}
                  </button>
                )}
                {repo && (
                  <button type="button" className="glass-button-subtle" onClick={() => void onCheckDrift()} disabled={driftDisabled}>
                    {driftLoading ? 'Checking...' : 'Check drift'}
                  </button>
                )}
                <button type="button" className="glass-button-subtle" onClick={onClose} disabled={saving || syncing}>
                  Close
                </button>
                {canManage && (
                  <button type="button" className="glass-button-primary" onClick={() => void onSave()} disabled={!canEdit}>
                    {saving ? 'Saving...' : repo ? 'Save Repository' : 'Connect Repository'}
                  </button>
                )}
              </div>
            </div>
          )}

          {activeSettingsTab === 'notifications' && (
            <div className="space-y-5" role="tabpanel" aria-label="Notifications">
              <div className="space-y-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <h4 className="text-sm font-semibold text-[var(--text-primary)]">Pipeline notifications</h4>
                    <p className="text-xs text-[var(--text-secondary)]">{notificationRoute?.updated_at ? `Updated ${formatConfigRepoTimestamp(notificationRoute.updated_at)}` : 'No saved policy'}</p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="runner-pill runner-pill--muted">{notificationSourceLabel}</span>
                    {notificationManaged && notificationRoute?.config_source_path && (
                      <span className="runner-pill runner-pill--link" title={notificationRoute.config_source_path}>
                        {notificationRoute.config_source_path}
                      </span>
                    )}
                  </div>
                </div>

                {notificationLoading ? (
                  <div className="text-sm text-[var(--text-secondary)]">Loading notification policy...</div>
                ) : (
                  <>
                    {notificationManaged && (
                      <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
                        Managed by GitOps.
                      </div>
                    )}
                    {repo && (
                      <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
                        GitOps target: <span className="font-mono text-[var(--text-primary)]">{notificationGitOpsTarget}</span>
                      </div>
                    )}

                    <div className={`${sectionClass} space-y-4`}>
                      <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_minmax(260px,320px)] gap-4">
                        <div className="space-y-2">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <p className={sectionTitleClass}>Routes</p>
                            <div className="flex flex-wrap items-center gap-2">
                              {notificationCanEdit && (
                                <button type="button" className="glass-button-subtle" onClick={() => onNotificationChange(prev => notificationRouteFormAddRoute(prev))}>
                                  <Plus className="h-4 w-4" />
                                  Add route
                                </button>
                              )}
                              {notificationCanEdit && notificationForm.routes.length > 1 && (
                                <button type="button" className="glass-button-danger" onClick={() => onNotificationChange(prev => notificationRouteFormRemoveSelectedRoute(prev))}>
                                  <Trash2 className="h-4 w-4" />
                                  Remove selected
                                </button>
                              )}
                            </div>
                          </div>
                          <div className="flex min-h-[44px] flex-wrap items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2">
                            {notificationForm.routes.map((route, index) => (
                              <button
                                key={`${route.name}-${index}`}
                                type="button"
                                className={`runner-pill ${notificationForm.selectedRouteIndex === index ? 'runner-pill--ok' : 'runner-pill--muted'}`}
                                onClick={() => onNotificationChange(prev => notificationRouteFormSelectRoute(prev, index))}
                                disabled={notificationSaving}
                              >
                                {(notificationForm.selectedRouteIndex === index ? notificationForm.routeName : route.name) || `route-${index + 1}`}
                              </button>
                            ))}
                          </div>
                        </div>
                        <label htmlFor="notification-route-name" className={fieldClass}>
                          <span>Route name</span>
                          <input
                            id="notification-route-name"
                            value={notificationForm.routeName}
                            onChange={event => onNotificationChange(prev => ({ ...prev, routeName: event.target.value }))}
                            disabled={!notificationCanEdit}
                            className={inputClass}
                            placeholder="release failures"
                          />
                        </label>
                      </div>

                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                        <label className={toggleClass}>
                          <input
                            type="checkbox"
                            className={checkboxClass}
                            checked={notificationForm.enabled}
                            onChange={event => onNotificationChange(prev => ({ ...prev, enabled: event.target.checked }))}
                            disabled={!notificationCanEdit}
                          />
                          Route enabled
                        </label>
                        <label className={toggleClass}>
                          <input
                            type="checkbox"
                            className={checkboxClass}
                            checked
                            disabled
                          />
                          Mail
                        </label>
                      </div>
                    </div>

                    <div className={`${sectionClass} space-y-4`}>
                      <p className={sectionTitleClass}>Recipients</p>
                      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                        <div className="space-y-3">
                          <label className={toggleClass}>
                            <input
                              type="checkbox"
                              className={checkboxClass}
                              checked={notificationForm.includeSameTeam}
                              onChange={event => onNotificationChange(prev => ({ ...prev, includeSameTeam: event.target.checked }))}
                              disabled={!notificationCanEdit}
                            />
                            Same team
                          </label>
                          <label htmlFor="notification-include-users" className={fieldClass}>
                            <span>People</span>
                            <textarea
                              id="notification-include-users"
                              value={notificationForm.includeUsers}
                              onChange={event => onNotificationChange(prev => ({ ...prev, includeUsers: event.target.value }))}
                              disabled={!notificationCanEdit}
                              className={textareaClass}
                              placeholder="release@example.com"
                            />
                          </label>
                          <label htmlFor="notification-include-teams" className={fieldClass}>
                            <span>Teams</span>
                            <textarea
                              id="notification-include-teams"
                              value={notificationForm.includeTeams}
                              onChange={event => onNotificationChange(prev => ({ ...prev, includeTeams: event.target.value }))}
                              disabled={!notificationCanEdit}
                              className={textareaClass}
                              placeholder="team-1/platform"
                            />
                          </label>
                        </div>

                        <div className="space-y-3">
                          <div className="min-h-[46px] rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-secondary)]">
                            Exclusions apply after all included recipients are resolved.
                          </div>
                          <label htmlFor="notification-exclude-users" className={fieldClass}>
                            <span>Excluded people</span>
                            <textarea
                              id="notification-exclude-users"
                              value={notificationForm.excludeUsers}
                              onChange={event => onNotificationChange(prev => ({ ...prev, excludeUsers: event.target.value }))}
                              disabled={!notificationCanEdit}
                              className={textareaClass}
                              placeholder="quiet@example.com"
                            />
                          </label>
                          <label htmlFor="notification-exclude-teams" className={fieldClass}>
                            <span>Excluded teams</span>
                            <textarea
                              id="notification-exclude-teams"
                              value={notificationForm.excludeTeams}
                              onChange={event => onNotificationChange(prev => ({ ...prev, excludeTeams: event.target.value }))}
                              disabled={!notificationCanEdit}
                              className={textareaClass}
                              placeholder="team-1/noisy-workloads"
                            />
                          </label>
                        </div>
                      </div>
                    </div>

                    <div className={`${sectionClass} space-y-3`}>
                      <p className={sectionTitleClass}>Events</p>
                      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-2">
                        {NOTIFICATION_EVENTS.map(option => (
                          <label key={option.key} className={toggleClass}>
                            <input
                              type="checkbox"
                              className={checkboxClass}
                              checked={notificationForm.events[option.key]}
                              onChange={event =>
                                onNotificationChange(prev => ({
                                  ...prev,
                                  events: { ...prev.events, [option.key]: event.target.checked },
                                }))
                              }
                              disabled={!notificationCanEdit}
                            />
                            {option.label}
                          </label>
                        ))}
                      </div>
                    </div>

                    <div className={`${sectionClass} space-y-4`}>
                      <p className={sectionTitleClass}>Filters</p>
                      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                        <NotificationPatternInputs
                          label="Pipelines"
                          includeID="notification-pipeline-include"
                          excludeID="notification-pipeline-exclude"
                          includeValue={notificationForm.pipelineInclude}
                          excludeValue={notificationForm.pipelineExclude}
                          disabled={!notificationCanEdit}
                          inputClass={inputClass}
                          fieldClass={fieldClass}
                          onIncludeChange={value => onNotificationChange(prev => ({ ...prev, pipelineInclude: value }))}
                          onExcludeChange={value => onNotificationChange(prev => ({ ...prev, pipelineExclude: value }))}
                        />
                        <NotificationPatternInputs
                          label="Repositories"
                          includeID="notification-repo-include"
                          excludeID="notification-repo-exclude"
                          includeValue={notificationForm.repoInclude}
                          excludeValue={notificationForm.repoExclude}
                          disabled={!notificationCanEdit}
                          inputClass={inputClass}
                          fieldClass={fieldClass}
                          onIncludeChange={value => onNotificationChange(prev => ({ ...prev, repoInclude: value }))}
                          onExcludeChange={value => onNotificationChange(prev => ({ ...prev, repoExclude: value }))}
                        />
                        <NotificationPatternInputs
                          label="Branches"
                          includeID="notification-branch-include"
                          excludeID="notification-branch-exclude"
                          includeValue={notificationForm.branchInclude}
                          excludeValue={notificationForm.branchExclude}
                          disabled={!notificationCanEdit}
                          inputClass={inputClass}
                          fieldClass={fieldClass}
                          onIncludeChange={value => onNotificationChange(prev => ({ ...prev, branchInclude: value }))}
                          onExcludeChange={value => onNotificationChange(prev => ({ ...prev, branchExclude: value }))}
                        />
                      </div>
                    </div>

                    <div className={`${sectionClass} space-y-4`}>
                      <p className={sectionTitleClass}>Delivery limits</p>
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                        <label htmlFor="notification-dedupe-window" className={fieldClass}>
                          <span>Dedupe window</span>
                          <input
                            id="notification-dedupe-window"
                            value={notificationForm.dedupeWindow}
                            onChange={event => onNotificationChange(prev => ({ ...prev, dedupeWindow: event.target.value }))}
                            disabled={!notificationCanEdit}
                            className={inputClass}
                            placeholder="10m"
                          />
                        </label>
                        <label htmlFor="notification-max-per-run" className={fieldClass}>
                          <span>Max per run</span>
                          <input
                            id="notification-max-per-run"
                            type="number"
                            min="1"
                            value={notificationForm.maxPerRun}
                            onChange={event => onNotificationChange(prev => ({ ...prev, maxPerRun: event.target.value }))}
                            disabled={!notificationCanEdit}
                            className={inputClass}
                            placeholder="5"
                          />
                        </label>
                      </div>
                    </div>

                    {notificationError && <div className="text-sm text-red-600 break-words">{notificationError}</div>}

                    <div className="flex flex-wrap justify-end gap-3">
                      {notificationRoute?.id && (
                        <button type="button" className="glass-button-danger mr-auto" onClick={() => void onDeleteNotification()} disabled={notificationDeleteDisabled}>
                          {notificationSaving ? 'Removing...' : 'Remove Policy'}
                        </button>
                      )}
                      {repo && (
                        <button type="button" className="glass-button-subtle" onClick={() => void onCheckDrift()} disabled={driftDisabled}>
                          {driftLoading ? 'Checking...' : 'Review GitOps drift'}
                        </button>
                      )}
                      {canManage && (
                        <button type="button" className="glass-button-primary" onClick={() => void onSaveNotification()} disabled={notificationSaveDisabled}>
                          {notificationSaving ? 'Saving...' : 'Save Notifications'}
                        </button>
                      )}
                    </div>
                  </>
                )}
              </div>
            </div>
          )}

        </div>
      </div>
    </div>
  );
}

function NotificationPatternInputs({
  label,
  includeID,
  excludeID,
  includeValue,
  excludeValue,
  disabled,
  inputClass,
  fieldClass,
  onIncludeChange,
  onExcludeChange,
}: {
  label: string;
  includeID: string;
  excludeID: string;
  includeValue: string;
  excludeValue: string;
  disabled: boolean;
  inputClass: string;
  fieldClass: string;
  onIncludeChange: (value: string) => void;
  onExcludeChange: (value: string) => void;
}) {
  return (
    <div className="h-full space-y-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3">
      <p className="text-sm font-semibold text-[var(--text-primary)]">{label}</p>
      <label htmlFor={includeID} className={fieldClass}>
        <span>Include</span>
        <input
          id={includeID}
          value={includeValue}
          onChange={event => onIncludeChange(event.target.value)}
          disabled={disabled}
          className={inputClass}
          placeholder="*"
        />
      </label>
      <label htmlFor={excludeID} className={fieldClass}>
        <span>Exclude</span>
        <input
          id={excludeID}
          value={excludeValue}
          onChange={event => onExcludeChange(event.target.value)}
          disabled={disabled}
          className={inputClass}
          placeholder="dependabot/*"
        />
      </label>
    </div>
  );
}
