import { useState, type ChangeEvent, type FormEvent } from 'react';
import { CheckCircle2, ChevronRight, Copy, Link2, Loader2, RefreshCw, Save } from 'lucide-react';
import { ObjectIcon } from '../../../components/ObjectIcon';
import { copyTextToClipboard } from '../../../lib/clipboard';
import {
  gitHubWebhookEndpoint,
  gitHubWebhookURLWarning,
  type GitHubAppFormState,
  type GitHubAppResource,
} from './model';

/**
 * There is exactly one GitHub App per installation, so it gets exactly one card.
 * It leads with the two things an operator checks — is an App connected, and
 * where does GitHub deliver — because the App ID, private key, and webhook
 * secret are issued by GitHub and stored automatically. Those live behind the
 * advanced disclosure for the rare hand edit or a migrated App.
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
}) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const connected = Boolean(app.app_id);
  const readonly = !canManage;
  const fieldsDisabled = readonly || loading || saving;
  const deliveryAddress = gitHubWebhookEndpoint(app.webhook_endpoint || form.webhookURL);
  const webhookWarning = gitHubWebhookURLWarning(app.webhook_endpoint || form.webhookURL);

  const handleChange = (key: keyof GitHubAppFormState) => (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...form, [key]: event.target.value });
  };

  const copyDeliveryAddress = () => {
    if (deliveryAddress) void copyTextToClipboard(deliveryAddress).catch(() => undefined);
  };

  return (
    <section data-panel="git-app" className="git-app-card">
      <div className="git-app-card__head">
        <span className="git-app-card__mark">
          <ObjectIcon type="git-app" className="h-5 w-5" />
        </span>
        <div className="min-w-0">
          <p className="git-app-card__kicker">Git Apps</p>
          <h3 className="git-app-card__title">GitHub App</h3>
          <p className="git-app-card__lede">
            {connected
              ? 'One App serves every account below. Install it on an account to reach that account’s repositories.'
              : 'NopsAI creates the App on GitHub and stores its credentials for you.'}
          </p>
        </div>
        <div className="git-app-card__state">
          {connected ? (
            <span className="runner-pill runner-pill--ok inline-flex items-center gap-1">
              <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
              App {app.app_id}
            </span>
          ) : (
            <span className="runner-pill runner-pill--muted">Not connected</span>
          )}
          {readonly ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
          <button
            type="button"
            className="glass-button-subtle inline-flex items-center gap-2"
            onClick={onRefresh}
            disabled={loading || saving}
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} aria-hidden="true" />
            Refresh
          </button>
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
        </div>
      </div>

      <dl className="git-app-facts">
        <div className="git-app-fact">
          <dt>Webhook delivery</dt>
          <dd>
            {deliveryAddress ? (
              <>
                <span className="git-app-fact__mono">{deliveryAddress}</span>
                <button
                  type="button"
                  className="github-app-action"
                  aria-label="Copy GitHub webhook URL"
                  title="Copy GitHub webhook URL"
                  onClick={copyDeliveryAddress}
                >
                  <Copy className="h-4 w-4" aria-hidden="true" />
                </button>
              </>
            ) : (
              <span className="git-app-fact__empty">Not configured</span>
            )}
          </dd>
        </div>
        <div className="git-app-fact">
          <dt>Defined in</dt>
          <dd><span className="git-app-fact__mono">setting/git-apps/github.yaml</span></dd>
        </div>
      </dl>

      {webhookWarning ? (
        <p className="git-app-card__warning" role="status">{webhookWarning}</p>
      ) : null}

      <form onSubmit={onSubmit}>
        <button
          type="button"
          className="git-app-disclosure"
          aria-expanded={advancedOpen}
          onClick={() => setAdvancedOpen(open => !open)}
        >
          <ChevronRight className={`h-4 w-4 transition-transform ${advancedOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
          Advanced
          <span className="git-app-disclosure__hint">
            App ID, credential references, and delivery address
          </span>
        </button>

        {advancedOpen ? (
          <div className="git-app-advanced">
            <p className="git-app-advanced__note">
              GitHub issues these when the App is created and NopsAI stores them. Edit them only to
              adopt an App that already exists, or after rotating a credential by hand.
            </p>
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
                <input
                  className="pipelines-input"
                  value={form.webhookURL}
                  onChange={handleChange('webhookURL')}
                  placeholder="https://your-tunnel.example.com/webhook"
                  disabled={fieldsDisabled}
                />
              </label>
              <p className="git-app-advanced__note">
                Where GitHub delivers events. It has to reach git-bot from the internet, for example
                through a tunnel or reverse proxy. A value with no path gets <code>/webhook</code>
                {' '}appended.
              </p>
            </div>

            <div className="flex justify-end">
              <button type="submit" className="glass-button inline-flex items-center gap-2" disabled={fieldsDisabled}>
                <Save className="h-4 w-4" aria-hidden="true" />
                Save
              </button>
            </div>
          </div>
        ) : null}
      </form>
    </section>
  );
}
