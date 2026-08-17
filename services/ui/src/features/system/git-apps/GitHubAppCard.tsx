import type { ChangeEvent, FormEvent } from 'react';
import { CheckCircle2, Copy, ExternalLink, Link2, Loader2, RefreshCw, Save } from 'lucide-react';
import { ObjectIcon } from '../../../components/ObjectIcon';
import { copyTextToClipboard } from '../../../lib/clipboard';
import { gitHubWebhookURLWarning, type GitHubAppFormState, type GitHubAppResource } from './model';

/**
 * There is exactly one GitHub App per installation, so it gets exactly one card:
 * its identity, the address GitHub delivers to, and the credentials it was
 * stored under. Choosing an account to register on is a one-off decision and
 * lives in the connect dialog instead of taking up room here.
 */
export default function GitHubAppCard({
  app,
  form,
  loading,
  saving,
  connecting,
  canManage,
  onChange,
  onSubmit,
  onRefresh,
  onConnect,
  onInstall,
}: {
  app: GitHubAppResource;
  form: GitHubAppFormState;
  loading: boolean;
  saving: boolean;
  connecting: boolean;
  canManage: boolean;
  onChange: (next: GitHubAppFormState) => void;
  onSubmit: (event: FormEvent) => void;
  onRefresh: () => void;
  onConnect: () => void;
  onInstall: () => void;
}) {
  const connected = Boolean(app.app_id);
  const readonly = !canManage;
  const fieldsDisabled = readonly || loading || saving;
  const webhookWarning = gitHubWebhookURLWarning(form.webhookURL);

  const handleChange = (key: keyof GitHubAppFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...form, [key]: event.target.value });
  };

  const copyWebhookURL = () => {
    const value = form.webhookURL.trim();
    if (value) void copyTextToClipboard(value).catch(() => undefined);
  };

  return (
    <section
      data-panel="git-app"
      className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5"
    >
      <form className="space-y-4" onSubmit={onSubmit}>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)]">
              <ObjectIcon type="git-app" className="h-5 w-5" />
            </span>
            <div className="min-w-0">
              <p className="text-xs text-[var(--text-secondary)]">Git Apps</p>
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">GitHub App</h3>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">
                {connected
                  ? 'One App serves every account below. Install it on an account to reach that account’s repositories.'
                  : 'NopsAI creates the App on GitHub and stores its App ID, private key, and webhook secret for you.'}
              </p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {connected ? (
              <span className="runner-pill runner-pill--ok inline-flex items-center gap-1">
                <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                App {app.app_id}
              </span>
            ) : (
              <span className="runner-pill runner-pill--muted">Not connected</span>
            )}
            <span className="runner-pill runner-pill--muted">setting/git-apps/github.yaml</span>
            {readonly ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
            <button
              type="button"
              className="glass-button-subtle inline-flex items-center gap-2"
              onClick={onRefresh}
              disabled={loading || saving}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              Refresh
            </button>
            <button type="submit" className="glass-button inline-flex items-center gap-2" disabled={fieldsDisabled}>
              <Save className="h-4 w-4" aria-hidden="true" />
              Save
            </button>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-3">
          <button
            type="button"
            className="glass-button inline-flex items-center gap-2"
            onClick={onConnect}
            disabled={readonly || connecting}
          >
            {connecting ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <Link2 className="h-4 w-4" aria-hidden="true" />
            )}
            {connected ? 'Replace App' : 'Connect GitHub'}
          </button>
          {connected ? (
            <button
              type="button"
              className="glass-button-subtle inline-flex items-center gap-2"
              onClick={onInstall}
              disabled={readonly || connecting || !app.app_slug}
              title={app.app_slug ? undefined : 'The App slug is unknown; add the installation manually'}
            >
              <ExternalLink className="h-4 w-4" aria-hidden="true" />
              Install on an account
            </button>
          ) : null}
          <p className="text-xs text-[var(--text-secondary)]">
            GitHub asks you to approve the App, then returns you to this NopsAI address. Only the
            webhook URL is fetched by GitHub, so NopsAI does not need to be reachable from the internet.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>App ID</span>
            <input
              className="pipelines-input"
              value={form.appID}
              onChange={handleChange('appID')}
              inputMode="numeric"
              placeholder="123456"
              disabled={fieldsDisabled}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>Private key credential ref</span>
            <input
              className="pipelines-input"
              value={form.privateKeyCredentialRef}
              onChange={handleChange('privateKeyCredentialRef')}
              placeholder="credential://system/github/app-private-key"
              disabled={fieldsDisabled}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>Webhook credential ref</span>
            <input
              className="pipelines-input"
              value={form.webhookCredentialRef}
              onChange={handleChange('webhookCredentialRef')}
              placeholder="credential://system/github/webhook-secret"
              disabled={fieldsDisabled}
            />
          </label>
        </div>

        <div className="flex flex-col gap-1">
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>Webhook URL</span>
            <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
              <input
                className="pipelines-input"
                value={form.webhookURL}
                onChange={handleChange('webhookURL')}
                placeholder="https://your-tunnel.example.com/webhook"
                disabled={fieldsDisabled}
              />
              <button
                type="button"
                className="glass-button-subtle inline-flex items-center justify-center px-3"
                aria-label="Copy GitHub webhook URL"
                title="Copy GitHub webhook URL"
                onClick={copyWebhookURL}
                disabled={!form.webhookURL.trim()}
              >
                <Copy className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </label>
          <p className="text-xs text-[var(--text-secondary)]">
            The address GitHub delivers events to. It has to reach git-bot from the internet, for
            example through a tunnel or reverse proxy.
          </p>
          {webhookWarning ? (
            <p className="text-xs text-amber-700 dark:text-amber-300" role="status">
              {webhookWarning}
            </p>
          ) : null}
        </div>
      </form>
    </section>
  );
}
