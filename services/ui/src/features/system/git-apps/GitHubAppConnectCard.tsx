import type { ChangeEvent, FormEvent } from 'react';
import { CheckCircle2, Copy, ExternalLink, Github, Loader2, Plus } from 'lucide-react';
import { copyTextToClipboard } from '../../../lib/clipboard';
import { gitHubWebhookURLWarning, type GitHubAppConnectFormState, type GitHubAppConnectTarget, type GitHubAppResource } from './model';

/**
 * The guided path: NopsAI generates the App manifest, GitHub asks the operator
 * to approve it, and the credentials come back automatically. Manual App ID and
 * credential-reference entry stays available on the Git Apps panel for GitHub
 * Enterprise Server and air-gapped installs.
 */
export default function GitHubAppConnectCard({
  app,
  form,
  webhookURL,
  connecting,
  canManage,
  onChange,
  onWebhookURLChange,
  onConnect,
  onInstall,
}: {
  app: GitHubAppResource;
  form: GitHubAppConnectFormState;
  webhookURL: string;
  connecting: boolean;
  canManage: boolean;
  onChange: (next: GitHubAppConnectFormState) => void;
  onWebhookURLChange: (next: string) => void;
  onConnect: () => void;
  onInstall: () => void;
}) {
  const connected = Boolean(app.app_id);
  const disabled = !canManage || connecting;
  const webhookWarning = gitHubWebhookURLWarning(webhookURL);
  const canConnect = !disabled && Boolean(webhookURL.trim());

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!canConnect) return;
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

      {webhookWarning ? (
        <div
          className="mb-4 rounded-lg border border-amber-500/40 bg-[var(--bg-primary)] px-4 py-3 text-sm text-amber-700 dark:text-amber-300"
          role="status"
        >
          {webhookWarning}
        </div>
      ) : null}

      <form className="grid gap-4 lg:grid-cols-3" onSubmit={handleSubmit}>
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
        <div className="flex flex-col gap-1 lg:col-span-3">
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>Webhook URL</span>
            <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
              <input
                className="pipelines-input"
                value={webhookURL}
                onChange={event => onWebhookURLChange(event.target.value)}
                placeholder="https://your-tunnel.example.com/webhook"
                disabled={disabled}
              />
              <button
                type="button"
                className="glass-button-subtle inline-flex items-center justify-center px-3"
                aria-label="Copy GitHub webhook URL"
                title="Copy GitHub webhook URL"
                onClick={() => {
                  const value = webhookURL.trim();
                  if (value) void copyTextToClipboard(value).catch(() => undefined);
                }}
                disabled={!webhookURL.trim()}
              >
                <Copy className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </label>
          <p className="text-xs text-[var(--text-secondary)]">
            The address GitHub delivers events to. It has to reach git-bot from the internet, for
            example through a tunnel or reverse proxy, and NopsAI itself can stay private.
          </p>
        </div>
        <div className="flex flex-wrap items-end gap-2 lg:col-span-3">
          <button type="submit" className="glass-button inline-flex items-center gap-2" disabled={!canConnect}>
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
              disabled={disabled || !app.app_slug}
              title={app.app_slug ? undefined : 'The App slug is unknown; add the installation manually'}
            >
              <ExternalLink className="h-4 w-4" aria-hidden="true" />
              Install on an account
            </button>
          ) : null}
        </div>
      </form>

      <p className="mt-3 text-xs text-[var(--text-secondary)]">
        GitHub asks you to approve the App, then returns you to this NopsAI address. Only the webhook
        URL is fetched by GitHub, so NopsAI does not need to be reachable from the internet.
      </p>
    </section>
  );
}
