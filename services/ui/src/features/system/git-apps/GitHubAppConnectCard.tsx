import type { ChangeEvent, FormEvent } from 'react';
import { CheckCircle2, ExternalLink, Github, Loader2, Plus } from 'lucide-react';
import type { GitHubAppConnectFormState, GitHubAppConnectTarget, GitHubAppResource } from './model';

/**
 * The guided path: NopsAI generates the App manifest, GitHub asks the operator
 * to approve it, and the credentials come back automatically. Manual App ID and
 * credential-reference entry stays available on the Git Apps panel for GitHub
 * Enterprise Server and air-gapped installs.
 */
export default function GitHubAppConnectCard({
  app,
  form,
  connecting,
  canManage,
  onChange,
  onConnect,
  onInstall,
}: {
  app: GitHubAppResource;
  form: GitHubAppConnectFormState;
  connecting: boolean;
  canManage: boolean;
  onChange: (next: GitHubAppConnectFormState) => void;
  onConnect: () => void;
  onInstall: () => void;
}) {
  const connected = Boolean(app.app_id);
  const disabled = !canManage || connecting || !app.connect_supported;

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (disabled) return;
    onConnect();
  };

  const handleTarget = (event: ChangeEvent<HTMLSelectElement>) => {
    onChange({ ...form, target: event.target.value as GitHubAppConnectTarget });
  };

  return (
    <section
      data-panel="git-apps-connect"
      className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5"
    >
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)]">
            <Github className="h-5 w-5 text-[var(--text-primary)]" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <p className="text-xs text-[var(--text-secondary)]">Connect</p>
            <h3 className="text-lg font-semibold text-[var(--text-primary)]">
              {connected ? 'GitHub App connected' : 'Connect GitHub'}
            </h3>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">
              {connected
                ? 'Install the App on an account to give NopsAI access to its repositories.'
                : 'NopsAI creates the App on GitHub and stores its App ID, private key, and webhook secret for you.'}
            </p>
          </div>
        </div>
        {connected ? (
          <span className="runner-pill runner-pill--ok inline-flex items-center gap-1">
            <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
            App {app.app_id}
          </span>
        ) : null}
      </div>

      {!app.connect_supported ? (
        <div
          className="mb-4 rounded-lg border border-amber-500/40 bg-[var(--bg-primary)] px-4 py-3 text-sm text-amber-700 dark:text-amber-300"
          role="status"
        >
          {app.connect_blocked_by || 'Set a public URL GitHub can reach before connecting a GitHub App.'}
        </div>
      ) : null}

      <form className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto]" onSubmit={handleSubmit}>
        <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
          <span>Account type</span>
          <select
            className="pipelines-input"
            value={form.target}
            onChange={handleTarget}
            disabled={disabled}
            aria-label="GitHub account type"
          >
            <option value="organization">Organization</option>
            <option value="personal">Personal account</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
          <span>Organization</span>
          <input
            className="pipelines-input"
            value={form.organization}
            onChange={event => onChange({ ...form, organization: event.target.value })}
            placeholder="acme"
            disabled={disabled || form.target !== 'organization'}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
          <span>App name</span>
          <input
            className="pipelines-input"
            value={form.appName}
            onChange={event => onChange({ ...form, appName: event.target.value })}
            placeholder="NopsAI"
            disabled={disabled}
          />
        </label>
        <div className="flex flex-wrap items-end gap-2">
          <button type="submit" className="glass-button inline-flex items-center gap-2" disabled={disabled}>
            {connecting ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <Plus className="h-4 w-4" aria-hidden="true" />
            )}
            {connected ? 'Replace App' : 'Create App on GitHub'}
          </button>
          {connected ? (
            <button
              type="button"
              className="glass-button-subtle inline-flex items-center gap-2"
              onClick={onInstall}
              disabled={!canManage || connecting || !app.app_slug}
              title={app.app_slug ? undefined : 'The App slug is unknown; add the installation manually'}
            >
              <ExternalLink className="h-4 w-4" aria-hidden="true" />
              Install on an account
            </button>
          ) : null}
        </div>
      </form>

      <p className="mt-3 text-xs text-[var(--text-secondary)]">
        GitHub asks you to approve the App, then returns here. Webhook deliveries go to{' '}
        <span className="font-mono">{app.webhook_endpoint}</span>.
      </p>
    </section>
  );
}
