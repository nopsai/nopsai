import { useState } from 'react';
import { CheckCircle2, Github, Loader2 } from 'lucide-react';
import { gitHubWebhookEndpoint, gitHubWebhookURLWarning, installationDisplayName } from '../git-apps/model';
import { useGitHubApp } from '../git-apps/useGitHubApp';
import { StepIntro } from './SetupWizardPrimitives';

/**
 * First install asks for the one thing NopsAI cannot work out on its own — which
 * GitHub account owns the App — and nothing else. App ID, private key, and
 * webhook secret are issued by GitHub and stored automatically, so they are not
 * shown here; System > Git Apps keeps them for the rare hand edit. The button
 * leads straight to GitHub, where repository access is granted.
 */
export default function SetupGitHubStep({ canManage }: { canManage: boolean }) {
  const controller = useGitHubApp({ enabled: true, canManage });
  const [organization, setOrganization] = useState('');
  const [webhookURL, setWebhookURL] = useState('');
  const app = controller.app;
  const installations = app.installations;
  const registered = Boolean(app.app_slug);
  // GitHub's servers fetch the webhook address, so it is the only value that has
  // to be reachable from the internet and the only one worth asking for — and
  // only when the deployment has not configured it already.
  const deliveryAddress = app.webhook_endpoint || webhookURL.trim();
  // A bare tunnel address is accepted: the API appends git-bot's /webhook path
  // when the value carries none. Resolving it here shows what GitHub will be
  // registered with before the operator leaves for GitHub.
  const resolvedEndpoint = gitHubWebhookEndpoint(deliveryAddress);
  const webhookWarning = gitHubWebhookURLWarning(deliveryAddress);
  const canStart = canManage && !controller.loading && !controller.connecting && Boolean(deliveryAddress);

  return (
    <div className="space-y-4">
      <StepIntro title="Connect GitHub" icon={<Github className="h-4 w-4" />}>
        One button takes you to GitHub, where you approve the App and choose the repositories NopsAI
        may read. NopsAI stores the credentials GitHub issues. You can skip this step and connect
        GitHub later from System &gt; Git Apps.
      </StepIntro>

      {controller.error ? (
        <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500" role="alert">
          {controller.error}
        </div>
      ) : null}

      <div className="space-y-4 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
        {registered ? null : (
          <div className="flex max-w-md flex-col gap-1">
            <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
              <span>GitHub organization</span>
              <input
                className="pipelines-input"
                value={organization}
                onChange={event => setOrganization(event.target.value)}
                placeholder="acme"
                disabled={!canManage || controller.connecting}
              />
            </label>
            <p className="text-xs text-[var(--text-secondary)]">
              Leave this empty to create the App on your personal GitHub account. Only a real
              organization name belongs here — your own username is not one, and GitHub answers
              a 404.
            </p>
          </div>
        )}

        {app.webhook_endpoint ? null : (
          <div className="flex max-w-md flex-col gap-1">
            <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
              <span>Webhook URL</span>
              <input
                className="pipelines-input"
                value={webhookURL}
                onChange={event => setWebhookURL(event.target.value)}
                placeholder="https://your-tunnel.example.com/webhook"
                disabled={!canManage || controller.connecting}
              />
            </label>
            <p className="text-xs text-[var(--text-secondary)]">
              The public address of the tunnel or proxy in front of git-bot — the only address
              GitHub itself calls, and the reason NopsAI does not have to be reachable. The
              <code className="px-1 font-mono">/webhook</code> path is added for you if you leave
              it off.
            </p>
          </div>
        )}

        {resolvedEndpoint ? (
          <p className="text-xs text-[var(--text-secondary)]">
            GitHub will deliver events to{' '}
            <span className="font-mono text-[var(--text-primary)]">{resolvedEndpoint}</span>
          </p>
        ) : null}

        {webhookWarning ? (
          <p className="text-xs text-amber-700 dark:text-amber-300" role="status">
            {webhookWarning}
          </p>
        ) : null}

        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            className="glass-button inline-flex items-center gap-2"
            onClick={() => void controller.setUpGitHubApp({ organization, webhookURL: deliveryAddress })}
            disabled={!canStart}
          >
            {controller.connecting ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <Github className="h-4 w-4" aria-hidden="true" />
            )}
            {registered ? 'Install on another GitHub account' : 'Install GitHub App on GitHub'}
          </button>
          <p className="text-xs text-[var(--text-secondary)]">
            GitHub brings you back here when it is done.
          </p>
        </div>
      </div>

      <div className="rounded-lg border border-[var(--border-primary)] p-4 text-sm">
        <p className="text-xs text-[var(--text-secondary)]">Registered installations</p>
        {installations.length ? (
          <ul className="mt-2 space-y-1">
            {installations.map(installation => (
              <li key={installation.installation_id} className="flex items-center gap-2 text-[var(--text-primary)]">
                <CheckCircle2 className="h-4 w-4 text-emerald-600 dark:text-emerald-300" aria-hidden="true" />
                <span>{installationDisplayName(installation)}</span>
                <span className="font-mono text-xs text-[var(--text-secondary)]">{installation.installation_id}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-2 text-[var(--text-secondary)]">
            No installation yet. Use the button above and pick the repositories on GitHub.
          </p>
        )}
      </div>
    </div>
  );
}
