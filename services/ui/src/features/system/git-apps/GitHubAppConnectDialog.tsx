import type { ChangeEvent, FormEvent } from 'react';
import { Link2 } from 'lucide-react';
import { ObjectIcon } from '../../../components/ObjectIcon';
import { WorkflowFormDialog } from '../../../components/WorkflowFormDialog';
import {
  gitHubWebhookURLWarning,
  type GitHubAppConnectFormState,
  type GitHubAppConnectTarget,
} from './model';

/**
 * Choosing which GitHub account to register the App on is a one-off decision, so
 * it lives in a dialog rather than on the Git App card. The webhook URL is the
 * same field the card edits, so the two can never drift apart.
 */
export default function GitHubAppConnectDialog({
  form,
  webhookURL,
  replacing,
  connecting,
  onChange,
  onWebhookURLChange,
  onClose,
  onSubmit,
}: {
  form: GitHubAppConnectFormState;
  webhookURL: string;
  replacing: boolean;
  connecting: boolean;
  onChange: (next: GitHubAppConnectFormState) => void;
  onWebhookURLChange: (next: string) => void;
  onClose: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const webhookWarning = gitHubWebhookURLWarning(webhookURL);
  const canSubmit = !connecting && Boolean(webhookURL.trim());

  const handleTarget = (event: ChangeEvent<HTMLSelectElement>) => {
    onChange({ ...form, target: event.target.value as GitHubAppConnectTarget });
  };

  return (
    <WorkflowFormDialog
      id="github-app-connect-dialog"
      titleId="github-app-connect-title"
      kicker="Git Apps"
      title={replacing ? 'Replace GitHub App' : 'Connect GitHub App'}
      headerLeading={<ObjectIcon type="git-app" className="h-5 w-5" />}
      onClose={onClose}
      onSubmit={onSubmit}
      closeDisabled={connecting}
      actions={(
        <>
          <button type="button" className="glass-button-subtle" onClick={onClose} disabled={connecting}>
            Cancel
          </button>
          <button type="submit" className="glass-button inline-flex items-center gap-2" disabled={!canSubmit}>
            <Link2 className="h-4 w-4" aria-hidden="true" />
            Continue on GitHub
          </button>
        </>
      )}
    >
      <div className="space-y-4">
        <p className="text-sm text-[var(--text-secondary)]">
          GitHub asks you to approve a new App, then sends you back here. NopsAI stores the App ID,
          private key, and webhook secret, and{' '}
          {replacing ? 'replaces the credentials of the App in use.' : 'no manual copying is needed.'}
        </p>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>Account type</span>
            <select
              className="pipelines-input"
              value={form.target}
              onChange={handleTarget}
              disabled={connecting}
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
              disabled={connecting || form.target !== 'organization'}
              autoFocus={form.target === 'organization'}
            />
          </label>
          <div className="flex flex-col gap-1 sm:col-span-2">
            <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
              <span>App name</span>
              <input
                className="pipelines-input"
                value={form.appName}
                onChange={event => onChange({ ...form, appName: event.target.value })}
                placeholder="NopsAI"
                disabled={connecting}
              />
            </label>
            <p className="text-xs text-[var(--text-secondary)]">
              Optional. GitHub rejects a name that is already taken.
            </p>
          </div>
        </div>

        <div className="flex flex-col gap-1">
          <label className="flex flex-col gap-1 text-sm text-[var(--text-primary)]">
            <span>Webhook URL</span>
            <input
              className="pipelines-input"
              value={webhookURL}
              onChange={event => onWebhookURLChange(event.target.value)}
              placeholder="https://your-tunnel.example.com/webhook"
              disabled={connecting}
            />
          </label>
          <p className="text-xs text-[var(--text-secondary)]">
            Required. GitHub delivers events here, so it has to reach git-bot from the internet.
          </p>
        </div>

        {webhookWarning ? (
          <div
            className="rounded-lg border border-amber-500/40 px-4 py-3 text-sm text-amber-700 dark:text-amber-300"
            role="status"
          >
            {webhookWarning}
          </div>
        ) : null}
      </div>
    </WorkflowFormDialog>
  );
}
