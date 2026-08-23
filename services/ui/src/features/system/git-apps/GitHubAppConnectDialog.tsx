import type { ChangeEvent, FormEvent } from 'react';
import { Link2 } from 'lucide-react';
import { ObjectIcon } from '../../../components/ObjectIcon';
import { WorkflowFormDialog } from '../../../components/WorkflowFormDialog';
import { WorkflowPropertyRow } from '../../../components/WorkflowPrimitives';
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
      <div className="modal-form-body">
        <p className="modal-hero-note">
          GitHub asks you to approve a new App, then sends you back here. NopsAI stores the App ID,
          private key, and webhook secret, and{' '}
          {replacing ? 'replaces the credentials of the App in use.' : 'no manual copying is needed.'}
        </p>

        <div className="modal-property-grid">
          <WorkflowPropertyRow label="Account type" hint="Where the App is installed">
            <select
              className="pipelines-input w-full"
              value={form.target}
              onChange={handleTarget}
              disabled={connecting}
            >
              <option value="organization">Organization</option>
              <option value="personal">Personal account</option>
            </select>
          </WorkflowPropertyRow>
          <WorkflowPropertyRow label="Organization" hint="GitHub account that owns the App">
            <input
              className="pipelines-input w-full"
              value={form.organization}
              onChange={event => onChange({ ...form, organization: event.target.value })}
              placeholder="acme"
              disabled={connecting || form.target !== 'organization'}
              autoFocus={form.target === 'organization'}
            />
          </WorkflowPropertyRow>
          <WorkflowPropertyRow label="App name" hint="Optional; GitHub rejects a taken name" control="wide">
            <input
              className="pipelines-input w-full"
              value={form.appName}
              onChange={event => onChange({ ...form, appName: event.target.value })}
              placeholder="NopsAI"
              disabled={connecting}
            />
          </WorkflowPropertyRow>
          <WorkflowPropertyRow
            label="Webhook URL"
            hint="Required; GitHub delivers events here, so it must reach git-bot from the internet"
            span="full"
            layout="stacked"
          >
            <input
              className="pipelines-input w-full"
              value={webhookURL}
              onChange={event => onWebhookURLChange(event.target.value)}
              placeholder="https://your-tunnel.example.com/webhook"
              disabled={connecting}
            />
          </WorkflowPropertyRow>
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
