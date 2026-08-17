import { CheckCircle2, Github } from 'lucide-react';
import GitHubAppCard from '../git-apps/GitHubAppCard';
import GitHubAppConnectDialog from '../git-apps/GitHubAppConnectDialog';
import { installationDisplayName } from '../git-apps/model';
import { useGitHubApp } from '../git-apps/useGitHubApp';
import { StepIntro } from './SetupWizardPrimitives';

/**
 * The wizard's GitHub step runs the same connect flow as System > Git Apps, so a
 * new installation never has to paste an App ID, private key, or webhook secret.
 * It stays optional: an installation without GitHub is still a working workspace.
 */
export default function SetupGitHubStep({ canManage }: { canManage: boolean }) {
  const controller = useGitHubApp({ enabled: true, canManage });
  const installations = controller.app.installations;

  return (
    <div className="space-y-4">
      <StepIntro title="Connect GitHub" icon={<Github className="h-4 w-4" />}>
        NopsAI creates a GitHub App for this installation, stores its credentials, and registers each
        account you install it on. You can skip this step and connect GitHub later from System &gt; Git Apps.
      </StepIntro>

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
        onChange={controller.setForm}
        onSubmit={controller.submitApp}
        onRefresh={() => void controller.loadApp()}
        onConnect={controller.openConnectDialog}
        onInstall={() => void controller.installGitHubApp()}
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
            No installation yet. Create the App, then install it on the account whose repositories NopsAI should read.
          </p>
        )}
      </div>
    </div>
  );
}
