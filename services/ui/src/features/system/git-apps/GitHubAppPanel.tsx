import { useMemo, useState, type ChangeEvent, type FormEvent, type ReactNode } from 'react';
import {
  BadgeCheck,
  CheckCircle2,
  ExternalLink,
  FolderSync,
  GitBranch,
  Github,
  PencilLine,
  Plus,
  RefreshCw,
  Save,
  Search,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import { ObjectIcon } from '../../../components/ObjectIcon';
import GitHubAppCard from './GitHubAppCard';
import GitHubAppConnectDialog from './GitHubAppConnectDialog';
import { WorkflowFormDialog } from '../../../components/WorkflowFormDialog';
import {
  buildGitHubAppMetrics,
  filterGitHubAppInstallations,
  formatGitHubAppDate,
  gitHubInstallationStatusLabel,
  gitHubInstallationStatusTone,
  installationDisplayName,
  type GitHubAppInstallation,
  type GitHubAppInstallationFormState,
  type GitHubAppInstallationRepository,
} from './model';
import type { GitHubAppController } from './useGitHubApp';

export default function GitHubAppPanel({
  controller,
  canManage,
}: {
  controller: GitHubAppController;
  canManage: boolean;
}) {
  const [searchTerm, setSearchTerm] = useState('');
  const metrics = useMemo(() => buildGitHubAppMetrics(controller.app), [controller.app]);
  const installations = useMemo(
    () => filterGitHubAppInstallations(controller.app.installations, searchTerm),
    [controller.app.installations, searchTerm]
  );
  const selected = controller.selectedInstallation;
  const readonly = !canManage;

  return (
    <div data-panel="git-apps" className="space-y-5 pb-24">
      {controller.error ? (
        <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500" role="alert">
          {controller.error}
        </div>
      ) : null}

      <GitHubAppCard
        app={controller.app}
        form={controller.form}
        loading={controller.loading}
        saving={controller.saving}
        connecting={controller.connecting}
        canManage={canManage}
        onChange={next => controller.setForm(next)}
        onSubmit={controller.submitApp}
        onRefresh={() => void controller.loadApp()}
        onConnect={controller.openConnectDialog}
        onInstall={() => void controller.installGitHubApp()}
      />

      <section className="grid gap-3 md:grid-cols-5" aria-label="GitHub App summary">
        <Metric icon={<Github className="h-4 w-4" aria-hidden="true" />} label="Installations" value={metrics.installations} />
        <Metric icon={<CheckCircle2 className="h-4 w-4" aria-hidden="true" />} label="Enabled" value={metrics.enabled} tone="ok" />
        <Metric icon={<ShieldCheck className="h-4 w-4" aria-hidden="true" />} label="Disabled" value={metrics.disabled} tone="muted" />
        <Metric icon={<GitBranch className="h-4 w-4" aria-hidden="true" />} label="Repositories" value={metrics.repositories} tone="blue" />
        <Metric icon={<ObjectIcon type="trigger" />} label="Triggers" value={metrics.connectedTriggers} tone="blue" />
      </section>

      <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5">
        <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <p className="text-xs text-[var(--text-secondary)]">Installations</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">GitHub accounts</h3>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="relative block min-w-[240px] text-sm">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-secondary)]" aria-hidden="true" />
              <input
                className="pipelines-input w-full pl-9"
                value={searchTerm}
                onChange={event => setSearchTerm(event.target.value)}
                placeholder="Search installations"
                aria-label="Search GitHub App installations"
              />
            </label>
            <button
              type="button"
              className="glass-button inline-flex items-center gap-2"
              onClick={() => void controller.installGitHubApp()}
              disabled={readonly || controller.connecting || !controller.app.app_slug}
              title={controller.app.app_slug ? undefined : 'Connect a GitHub App first, or add the installation manually'}
            >
              <ExternalLink className="h-4 w-4" aria-hidden="true" />
              Install on GitHub
            </button>
            <button
              type="button"
              className="glass-button-subtle inline-flex items-center gap-2"
              onClick={controller.startCreateInstallation}
              disabled={readonly || controller.saving}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
              Add manually
            </button>
          </div>
        </div>

        {controller.loading ? (
          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-6 text-sm text-[var(--text-secondary)]">
            Loading GitHub App...
          </div>
        ) : installations.length ? (
          <div className="overflow-x-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)]">
            <table className="w-full min-w-[920px] text-left text-sm">
              <thead className="border-b border-[var(--border-primary)] text-xs uppercase text-[var(--text-secondary)]">
                <tr>
                  <th scope="col" className="px-4 py-3">Account</th>
                  <th scope="col" className="px-4 py-3">Installation ID</th>
                  <th scope="col" className="px-4 py-3">Type</th>
                  <th scope="col" className="px-4 py-3">Repositories</th>
                  <th scope="col" className="px-4 py-3">Triggers</th>
                  <th scope="col" className="px-4 py-3">Status</th>
                  <th scope="col" className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {installations.map(installation => (
                  <InstallationRow
                    key={installation.installation_id}
                    installation={installation}
                    selected={installation.installation_id === controller.selectedInstallationID}
                    saving={controller.saving}
                    canManage={canManage}
                    onSelect={() => controller.setSelectedInstallationID(installation.installation_id)}
                    onEdit={() => controller.startEditInstallation(installation)}
                    onVerify={() => void controller.verifyInstallation(installation)}
                    onRefresh={() => void controller.refreshInstallation(installation)}
                    onDelete={() => void controller.removeInstallation(installation)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-6 text-sm text-[var(--text-secondary)]">
            No GitHub App installations found.
          </div>
        )}
      </section>

      <RepositoryPanel
        installation={selected}
        saving={controller.saving}
        onRefresh={installation => void controller.refreshInstallation(installation)}
        onLoad={installation => void controller.loadRepositories(installation)}
      />

      {controller.connectDialogOpen ? (
        <GitHubAppConnectDialog
          form={controller.connectForm}
          webhookURL={controller.form.webhookURL}
          replacing={Boolean(controller.app.app_id)}
          connecting={controller.connecting}
          onChange={controller.setConnectForm}
          onWebhookURLChange={value => controller.setForm(current => ({ ...current, webhookURL: value }))}
          onClose={controller.closeConnectDialog}
          onSubmit={event => {
            event.preventDefault();
            void controller.connectGitHubApp();
          }}
        />
      ) : null}

      {controller.installationEditorOpen ? (
        <InstallationDialog
          editing={controller.editingInstallation || null}
          form={controller.installationForm}
          saving={controller.saving}
          onChange={controller.setInstallationForm}
          onClose={controller.closeInstallationEditor}
          onSubmit={controller.submitInstallation}
        />
      ) : null}
    </div>
  );
}

function Metric({
  icon,
  label,
  value,
  tone,
}: {
  icon: ReactNode;
  label: string;
  value: number;
  tone?: 'ok' | 'muted' | 'blue';
}) {
  const toneClass = tone === 'ok'
    ? 'text-emerald-600 dark:text-emerald-300'
    : tone === 'muted'
      ? 'text-[var(--text-secondary)]'
      : 'text-blue-600 dark:text-blue-300';
  return (
    <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
      <div className={`flex h-7 w-7 items-center justify-center rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] ${toneClass}`}>
        {icon}
      </div>
      <div className="min-w-0 truncate text-xs text-[var(--text-secondary)]">{label}</div>
      <div className="justify-self-end text-xl font-semibold leading-none text-[var(--text-primary)]">{value}</div>
    </div>
  );
}

function InstallationRow({
  installation,
  selected,
  saving,
  canManage,
  onSelect,
  onEdit,
  onVerify,
  onRefresh,
  onDelete,
}: {
  installation: GitHubAppInstallation;
  selected: boolean;
  saving: boolean;
  canManage: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onVerify: () => void;
  onRefresh: () => void;
  onDelete: () => void;
}) {
  const statusTone = gitHubInstallationStatusTone(installation);
  return (
    <tr
      className={`border-b border-[var(--border-primary)] last:border-0 ${selected ? 'bg-[var(--bg-tertiary)]' : ''}`}
      onClick={onSelect}
    >
      <td className="px-4 py-3">
        <button
          type="button"
          className="max-w-[220px] truncate text-left font-medium text-[var(--text-primary)] hover:underline"
          onClick={event => {
            event.stopPropagation();
            onSelect();
          }}
        >
          {installationDisplayName(installation)}
        </button>
      </td>
      <td className="px-4 py-3 font-mono text-xs text-[var(--text-secondary)]">{installation.installation_id}</td>
      <td className="px-4 py-3 capitalize text-[var(--text-secondary)]">{installation.account_type || 'Unknown'}</td>
      <td className="px-4 py-3 text-[var(--text-primary)]">{installation.accessible_repositories}</td>
      <td className="px-4 py-3 text-[var(--text-primary)]">{installation.connected_triggers}</td>
      <td className="px-4 py-3">
        <span className={`runner-pill runner-pill--${statusTone}`}>{gitHubInstallationStatusLabel(installation)}</span>
      </td>
      <td className="px-4 py-3">
        <div className="flex justify-end gap-1">
          <IconAction label={`Verify ${installationDisplayName(installation)}`} tone="verify" disabled={!canManage || saving} onClick={onVerify}>
            <BadgeCheck className="h-4 w-4" aria-hidden="true" />
          </IconAction>
          <IconAction label={`Refresh repositories for ${installationDisplayName(installation)}`} tone="sync" disabled={saving} onClick={onRefresh}>
            <FolderSync className="h-4 w-4" aria-hidden="true" />
          </IconAction>
          <IconAction label={`Edit ${installationDisplayName(installation)}`} tone="edit" disabled={!canManage || saving} onClick={onEdit}>
            <PencilLine className="h-4 w-4" aria-hidden="true" />
          </IconAction>
          <IconAction label={`Delete ${installationDisplayName(installation)}`} tone="danger" disabled={!canManage || saving} onClick={onDelete}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </IconAction>
        </div>
      </td>
    </tr>
  );
}

function IconAction({
  label,
  tone = 'default',
  disabled,
  onClick,
  children,
}: {
  label: string;
  tone?: 'default' | 'verify' | 'sync' | 'edit' | 'danger';
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  const toneClass = tone === 'default' ? '' : `github-app-action--${tone}`;

  return (
    <button
      type="button"
      className={`github-app-action ${toneClass}`}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={event => {
        event.stopPropagation();
        onClick();
      }}
    >
      {children}
    </button>
  );
}

function RepositoryPanel({
  installation,
  saving,
  onRefresh,
  onLoad,
}: {
  installation: GitHubAppInstallation | null;
  saving: boolean;
  onRefresh: (installation: GitHubAppInstallation) => void;
  onLoad: (installation: GitHubAppInstallation) => void;
}) {
  const repositories = installation?.repositories || [];
  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">Repositories</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">
            {installation ? installationDisplayName(installation) : 'GitHub repositories'}
          </h3>
          {installation?.last_repository_refresh_at ? (
            <p className="mt-1 text-xs text-[var(--text-secondary)]">
              Refreshed {formatGitHubAppDate(installation.last_repository_refresh_at)}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            className="glass-button-subtle inline-flex items-center gap-2"
            disabled={!installation || saving}
            onClick={() => installation && onLoad(installation)}
          >
            <Search className="h-4 w-4" aria-hidden="true" />
            Load
          </button>
          <button
            type="button"
            className="glass-button-subtle inline-flex items-center gap-2"
            disabled={!installation || saving}
            onClick={() => installation && onRefresh(installation)}
          >
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            Refresh
          </button>
        </div>
      </div>

      {installation?.last_error ? (
        <div className="mb-4 rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
          {installation.last_error}
        </div>
      ) : null}

      {!installation ? (
        <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-6 text-sm text-[var(--text-secondary)]">
          Select an installation.
        </div>
      ) : repositories.length ? (
        <div className="overflow-x-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)]">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b border-[var(--border-primary)] text-xs uppercase text-[var(--text-secondary)]">
              <tr>
                <th scope="col" className="px-4 py-3">Repository</th>
                <th scope="col" className="px-4 py-3">Default branch</th>
                <th scope="col" className="px-4 py-3">Visibility</th>
                <th scope="col" className="px-4 py-3">Access</th>
                <th scope="col" className="px-4 py-3">NopsAI</th>
              </tr>
            </thead>
            <tbody>
              {repositories.map(repository => (
                <RepositoryRow key={repository.id || repository.full_name} repository={repository} />
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-6 text-sm text-[var(--text-secondary)]">
          No repositories loaded.
        </div>
      )}
    </section>
  );
}

function RepositoryRow({ repository }: { repository: GitHubAppInstallationRepository }) {
  return (
    <tr className="border-b border-[var(--border-primary)] last:border-0">
      <td className="px-4 py-3 font-medium text-[var(--text-primary)]">{repository.full_name}</td>
      <td className="px-4 py-3 text-[var(--text-secondary)]">{repository.default_branch || 'Unknown'}</td>
      <td className="px-4 py-3 text-[var(--text-secondary)]">{repository.private ? 'Private' : 'Public'}</td>
      <td className="px-4 py-3 text-[var(--text-secondary)]">{repository.access || 'Unknown'}</td>
      <td className="px-4 py-3">
        <span className={`runner-pill runner-pill--${repository.used_by_nopsai ? 'ok' : 'muted'}`}>
          {repository.used_by_nopsai ? 'Used' : 'Unused'}
        </span>
      </td>
    </tr>
  );
}

function InstallationDialog({
  editing,
  form,
  saving,
  onChange,
  onClose,
  onSubmit,
}: {
  editing: GitHubAppInstallation | null;
  form: GitHubAppInstallationFormState;
  saving: boolean;
  onChange: (next: GitHubAppInstallationFormState) => void;
  onClose: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const handleChange = (key: keyof GitHubAppInstallationFormState) => (
    event: ChangeEvent<HTMLInputElement | HTMLSelectElement>
  ) => {
    const value = event.target instanceof HTMLInputElement && event.target.type === 'checkbox'
      ? event.target.checked
      : event.target.value;
    onChange({ ...form, [key]: value } as GitHubAppInstallationFormState);
  };

  return (
    <WorkflowFormDialog
      id="github-app-installation-dialog"
      titleId="github-app-installation-title"
      kicker="Git Apps"
      title={editing ? 'Edit GitHub installation' : 'Add GitHub installation'}
      headerLeading={<ObjectIcon type="git-app" className="h-5 w-5" />}
      onClose={onClose}
      onSubmit={onSubmit}
      closeDisabled={saving}
      actions={(
        <>
          <button type="button" className="glass-button-subtle" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="submit" className="glass-button inline-flex items-center gap-2" disabled={saving}>
            <Save className="h-4 w-4" aria-hidden="true" />
            Save installation
          </button>
        </>
      )}
    >
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
          <span>Installation ID</span>
          <input
            className="pipelines-input"
            value={form.installationID}
            onChange={handleChange('installationID')}
            inputMode="numeric"
            placeholder="987654"
            disabled={saving || Boolean(editing)}
            autoFocus={!editing}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
          <span>Account login</span>
          <input
            className="pipelines-input"
            value={form.accountLogin}
            onChange={handleChange('accountLogin')}
            placeholder="nopsai"
            disabled={saving}
            autoFocus={Boolean(editing)}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
          <span>Account type</span>
          <select
            className="pipelines-input"
            value={form.accountType || 'organization'}
            onChange={handleChange('accountType')}
            disabled={saving}
          >
            <option value="organization">Organization</option>
            <option value="user">User</option>
          </select>
        </label>
        <label className="flex min-h-[46px] items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={handleChange('enabled')}
            disabled={saving}
          />
          <span>Enabled</span>
        </label>
      </div>
    </WorkflowFormDialog>
  );
}
