import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import { WorkflowPropertyRow } from '../../components/WorkflowPrimitives';
import {
  GLOBAL_RESOURCE_TEAM_PATH,
  compareResourceTeamPathsWithGlobalFirst,
  isGlobalResourceTeamPath,
} from '../../lib/resourceTeams';
import {
  GIT_WEBHOOK_AUTH_MODES,
  GIT_WEBHOOK_PROVIDERS,
  GIT_WEBHOOK_VISIBILITIES,
  type GitWebhookSource,
  type GitWebhookSourceFormState,
} from './model';

export function GitWebhookSourceForm({
  source,
  form,
  saving,
  error,
  teamPaths,
  teamPathsLoading = false,
  onChange,
  onClose,
  onSubmit,
}: {
  source: GitWebhookSource | null;
  form: GitWebhookSourceFormState;
  saving: boolean;
  error: string | null;
  teamPaths: string[];
  teamPathsLoading?: boolean;
  onChange: (form: GitWebhookSourceFormState) => void;
  onClose: () => void;
  onSubmit: React.FormEventHandler<HTMLFormElement>;
}) {
  const update = <K extends keyof GitWebhookSourceFormState>(
    key: K,
    value: GitWebhookSourceFormState[K]
  ) => onChange({ ...form, [key]: value });
  const titleId = 'git-webhook-source-form-title';
  const errorId = 'git-webhook-source-form-error';
  const teamOptions = uniqueTeamOptions(teamPaths);

  return (
    <WorkflowFormDialog
      id="git-webhook-source-form-modal"
      titleId={titleId}
      descriptionId={error ? errorId : undefined}
      onClose={onClose}
      onSubmit={onSubmit}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="modal-form-body"
      kicker={source ? 'Edit source' : 'New source'}
      title={source ? 'Edit Git webhook source' : 'New Git webhook source'}
      subtitle="Provider credentials use encrypted NopsAI credential references."
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={saving}>
            {saving ? 'Saving...' : source ? 'Save source' : 'Create source'}
          </button>
        </>
      )}
    >
      {source?.managed_by_config_repo ? (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
          Saving here creates a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
        </div>
      ) : null}

      <div className="modal-property-grid">
        <Field label="Source ID" hint="Used in the delivery URL" htmlFor="git-webhook-source-id" control="wide">
          <input
            className="pipelines-input w-full font-mono"
            id="git-webhook-source-id"
            value={form.id}
            onChange={event => update('id', event.target.value)}
            disabled={Boolean(source)}
            placeholder="gitlab-platform"
            required
            data-dialog-initial-focus
          />
        </Field>
        <Field label="Display name" hint="Shown in lists and pickers" htmlFor="git-webhook-source-name">
          <input
            className="pipelines-input w-full"
            id="git-webhook-source-name"
            value={form.name}
            onChange={event => update('name', event.target.value)}
            placeholder="GitLab Platform"
          />
        </Field>
        <Field label="Provider" hint="Git host" htmlFor="git-webhook-source-provider">
          <select
            className="pipelines-input w-full"
            id="git-webhook-source-provider"
            value={form.provider}
            onChange={event => update('provider', event.target.value as GitWebhookSourceFormState['provider'])}
          >
            {GIT_WEBHOOK_PROVIDERS.map(provider => (
              <option key={provider} value={provider}>{provider}</option>
            ))}
          </select>
        </Field>
        <Field label="Team" hint={teamPathsLoading ? 'Loading teams…' : 'Owning team'} htmlFor="git-webhook-source-team">
          <select
            className="pipelines-input w-full"
            id="git-webhook-source-team"
            value={form.teamPath}
            onChange={event => update('teamPath', event.target.value)}
          >
            {teamOptions.map(path => (
              <option key={path} value={path}>{isGlobalResourceTeamPath(path) ? 'Global' : `/${path}`}</option>
            ))}
          </select>
        </Field>
        <Field label="Visibility" hint="Who may assign this source" htmlFor="git-webhook-source-visibility">
          <select
            className="pipelines-input w-full"
            id="git-webhook-source-visibility"
            value={form.visibility}
            onChange={event => update('visibility', event.target.value as GitWebhookSourceFormState['visibility'])}
          >
            {GIT_WEBHOOK_VISIBILITIES.map(visibility => (
              <option key={visibility} value={visibility}>{visibility === 'workspace' ? 'workspace-shared' : 'team'}</option>
            ))}
          </select>
        </Field>
        <Field label="Authentication" hint="How deliveries are verified" htmlFor="git-webhook-source-auth">
          <select
            className="pipelines-input w-full"
            id="git-webhook-source-auth"
            value={form.authMode}
            onChange={event => update('authMode', event.target.value as GitWebhookSourceFormState['authMode'])}
          >
            {GIT_WEBHOOK_AUTH_MODES.map(mode => (
              <option key={mode} value={mode}>{mode === 'none' ? 'none (internal only)' : mode}</option>
            ))}
          </select>
        </Field>
        <Field label="Description" hint="What this source receives" htmlFor="git-webhook-source-description" span="full" layout="stacked">
          <textarea
            className="pipelines-input min-h-20 w-full"
            id="git-webhook-source-description"
            value={form.description}
            onChange={event => update('description', event.target.value)}
            placeholder="Receives repository events from the primary GitLab instance."
          />
        </Field>
        {form.authMode !== 'none' ? (
        <Field label="Credential reference" hint="Expected type: webhook_secret. Leave blank on create to generate a one-time value." htmlFor="git-webhook-source-credential" span="full" layout="stacked">
          <input
            className="pipelines-input w-full font-mono"
            id="git-webhook-source-credential"
            value={form.credentialRef}
            onChange={event => update('credentialRef', event.target.value)}
            placeholder="credential://system/webhooks/gitlab-platform"
          />
        </Field>
        ) : (
          <p className="modal-property-row--full rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm text-[var(--text-secondary)]">
            Unauthenticated sources must only be exposed through a trusted, network-isolated ingress.
          </p>
        )}
        <Field
          label="Repository allowlist"
          hint="One owner/repository pattern per line; recursive wildcards are supported"
          htmlFor="git-webhook-source-allowlist"
          span="full"
          layout="stacked"
        >
          <textarea
            className="pipelines-input min-h-28 w-full font-mono"
            id="git-webhook-source-allowlist"
            value={form.repositoryAllowlistText}
            onChange={event => update('repositoryAllowlistText', event.target.value)}
            placeholder={'platform/api\nplatform/*'}
            required
          />
        </Field>

        <Field label="Rate limit per minute" hint="Empty means no source-level limit" htmlFor="git-webhook-source-rate-limit">
          <input
            className="pipelines-input w-full"
            type="number"
            min="1"
            step="1"
            id="git-webhook-source-rate-limit"
            value={form.rateLimitPerMinute}
            onChange={event => update('rateLimitPerMinute', event.target.value)}
            placeholder="120"
          />
        </Field>
        <div className="modal-property-row self-end">
          <div className="min-w-0">
            <label className="modal-property-label" htmlFor="git-webhook-source-enabled">
              Accept webhook deliveries
            </label>
            <span className="modal-property-hint">Pause the source without deleting it.</span>
          </div>
          <label className="modal-toggle">
            <input
              id="git-webhook-source-enabled"
              type="checkbox"
              checked={form.enabled}
              onChange={event => update('enabled', event.target.checked)}
            />
            <span />
          </label>
        </div>
      </div>

      {error ? (
        <div
          id={errorId}
          className="mt-4 rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500"
          role="alert"
        >
          {error}
        </div>
      ) : null}
    </WorkflowFormDialog>
  );
}

function uniqueTeamOptions(paths: string[]): string[] {
  const normalized = paths
    .map(path => String(path || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/'))
    .map(path => isGlobalResourceTeamPath(path) ? GLOBAL_RESOURCE_TEAM_PATH : path)
    .filter(Boolean);
  return Array.from(new Set([GLOBAL_RESOURCE_TEAM_PATH, ...normalized]))
    .sort(compareResourceTeamPathsWithGlobalFirst);
}

function Field({
  label,
  hint,
  htmlFor,
  span,
  layout,
  control,
  children,
}: {
  label: string;
  hint?: string;
  htmlFor: string;
  span?: 'half' | 'full';
  layout?: 'inline' | 'stacked';
  control?: 'default' | 'wide';
  children: React.ReactNode;
}) {
  return (
    <WorkflowPropertyRow label={label} hint={hint} htmlFor={htmlFor} span={span} layout={layout} control={control}>
      {children}
    </WorkflowPropertyRow>
  );
}
