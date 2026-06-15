import {
  Activity,
  Edit3,
  GitBranch,
  PauseCircle,
  PlayCircle,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Webhook,
} from 'lucide-react';
import { buildApiUrl } from '../../lib/api';
import { GitWebhookSourceForm } from './GitWebhookSourceForm';
import {
  deliveryStatusClass,
  formatGitWebhookDate,
  sourceStatusLabel,
  type GitWebhookSource,
} from './model';
import type { GitWebhookSourcesController } from './useGitWebhookSources';

export function GitWebhookSourcesView({
  controller,
  canWrite,
  canDelete,
}: {
  controller: GitWebhookSourcesController;
  canWrite: boolean;
  canDelete: boolean;
}) {
  const { selected } = controller;
  return (
    <div className="space-y-6 pb-24">
      <section className="glass-card rounded-xl border border-[var(--border-primary)] p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p className="max-w-3xl text-sm text-[var(--text-secondary)]">
              Receive GitLab, Bitbucket, Gitea, or normalized generic repository events and apply the same trigger manifest rules used by GitHub automation.
            </p>
          </div>
          <div className="flex items-center gap-2">
            {!canWrite ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
            <button
              type="button"
              className="glass-button-ghost"
              onClick={() => void controller.loadSources()}
              disabled={controller.loading || controller.saving}
            >
              <RefreshCw className="h-4 w-4" />
              Reload
            </button>
            {canWrite ? (
              <button type="button" className="glass-button-primary" onClick={controller.startCreate}>
                <Plus className="h-4 w-4" />
                New source
              </button>
            ) : null}
          </div>
        </div>
        {controller.error && !controller.editorOpen ? (
          <div className="mt-4 rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
            {controller.error}
          </div>
        ) : null}
      </section>

      <div className="grid items-start gap-6 xl:grid-cols-[minmax(280px,0.7fr)_minmax(0,1.3fr)]">
        <section className="glass-card rounded-xl border border-[var(--border-primary)] p-4">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="font-semibold text-[var(--text-primary)]">Sources</h2>
            <span className="runner-pill runner-pill--muted">{controller.sources.length}</span>
          </div>
          {controller.loading ? (
            <p className="py-8 text-center text-sm text-[var(--text-secondary)]">Loading sources...</p>
          ) : controller.sources.length ? (
            <div className="space-y-2">
              {controller.sources.map(source => (
                <SourceListItem
                  key={source.id}
                  source={source}
                  selected={selected?.id === source.id}
                  onSelect={() => controller.onSelect(source.id)}
                />
              ))}
            </div>
          ) : (
            <p className="py-8 text-center text-sm text-[var(--text-secondary)]">No Git webhook sources configured.</p>
          )}
        </section>

        {selected ? (
          <SourceDetail
            source={selected}
            controller={controller}
            canWrite={canWrite}
            canDelete={canDelete}
          />
        ) : (
          <section className="glass-card rounded-xl border border-dashed border-[var(--border-primary)] p-10 text-center">
            <Webhook className="mx-auto h-8 w-8 text-[var(--text-secondary)]" />
            <h2 className="mt-3 font-semibold text-[var(--text-primary)]">Select a webhook source</h2>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">
              Review its endpoint, repository allowlist, authentication, and recent deliveries.
            </p>
          </section>
        )}
      </div>

      {controller.editorOpen ? (
        <GitWebhookSourceForm
          source={controller.editing || null}
          form={controller.form}
          saving={controller.saving}
          error={controller.error}
          onChange={controller.setForm}
          onClose={controller.closeEditor}
          onSubmit={controller.submit}
        />
      ) : null}
    </div>
  );
}

function SourceListItem({
  source,
  selected,
  onSelect,
}: {
  source: GitWebhookSource;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      className={`w-full rounded-lg border p-3 text-left transition-colors ${
        selected
          ? 'border-[var(--accent-primary)] bg-[var(--bg-tertiary)]'
          : 'border-[var(--border-primary)] hover:bg-[var(--bg-tertiary)]'
      }`}
      onClick={onSelect}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="truncate font-medium text-[var(--text-primary)]">{source.name || source.id}</span>
        <span className={`runner-pill ${source.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
          {sourceStatusLabel(source)}
        </span>
      </div>
      <div className="mt-2 flex items-center gap-2 text-xs text-[var(--text-secondary)]">
        <span className="font-mono">{source.provider}</span>
        <span aria-hidden="true">/</span>
        <span className="truncate font-mono">{source.id}</span>
      </div>
    </button>
  );
}

function SourceDetail({
  source,
  controller,
  canWrite,
  canDelete,
}: {
  source: GitWebhookSource;
  controller: GitWebhookSourcesController;
  canWrite: boolean;
  canDelete: boolean;
}) {
  const managed = Boolean(source.managed_by_config_repo);
  const endpoint = buildApiUrl(`/v1/git/webhooks/${encodeURIComponent(source.id)}`);
  return (
    <section className="glass-card rounded-xl border border-[var(--border-primary)] p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-lg font-semibold text-[var(--text-primary)]">{source.name || source.id}</h2>
            <span className={`runner-pill ${source.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
              {sourceStatusLabel(source)}
            </span>
            {managed ? (
              <span className="runner-pill runner-pill--link">
                <GitBranch className="h-3.5 w-3.5" />
                GitOps
              </span>
            ) : null}
          </div>
          <p className="mt-1 font-mono text-xs text-[var(--text-secondary)]">{source.id}</p>
        </div>
        <div className="flex items-center gap-2">
          {canWrite && !managed ? (
            <>
              <button
                type="button"
                className="glass-button-ghost"
                disabled={controller.saving}
                onClick={() => void controller.setEnabled(source, !source.enabled)}
                title={source.enabled ? 'Disable source' : 'Enable source'}
              >
                {source.enabled ? <PauseCircle className="h-4 w-4" /> : <PlayCircle className="h-4 w-4" />}
              </button>
              <button
                type="button"
                className="glass-button-ghost"
                disabled={controller.saving}
                onClick={() => controller.startEdit(source)}
                title="Edit source"
              >
                <Edit3 className="h-4 w-4" />
              </button>
            </>
          ) : null}
          {canDelete && !managed ? (
            <button
              type="button"
              className="glass-button-danger"
              disabled={controller.saving}
              onClick={() => void controller.remove(source)}
              title="Delete source"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          ) : null}
        </div>
      </div>

      {source.description ? <p className="mt-4 text-sm text-[var(--text-secondary)]">{source.description}</p> : null}

      <div className="mt-5 grid gap-3 md:grid-cols-2">
        <Fact icon={<Webhook className="h-4 w-4" />} label="Provider" value={source.provider} />
        <Fact icon={<ShieldCheck className="h-4 w-4" />} label="Authentication" value={source.auth_mode} />
        <Fact icon={<Activity className="h-4 w-4" />} label="Last delivery" value={formatGitWebhookDate(source.last_used_at)} />
        <Fact
          icon={<GitBranch className="h-4 w-4" />}
          label="Rate limit"
          value={source.rate_limit.per_minute ? `${source.rate_limit.per_minute}/minute` : 'Unlimited'}
        />
      </div>

      <div className="mt-5">
        <p className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Webhook endpoint</p>
        <input className="pipelines-input mt-1 w-full font-mono text-xs" value={endpoint} readOnly />
      </div>

      <div className="mt-5">
        <p className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Repository allowlist</p>
        <div className="mt-2 flex flex-wrap gap-2">
          {source.repository_allowlist.map(pattern => (
            <span key={pattern} className="runner-pill runner-pill--muted font-mono">{pattern}</span>
          ))}
        </div>
      </div>

      {source.credential_ref ? (
        <div className="mt-5">
          <p className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Credential reference</p>
          <p className="mt-1 break-all font-mono text-sm text-[var(--text-primary)]">{source.credential_ref}</p>
        </div>
      ) : null}

      <div className="mt-6 border-t border-[var(--border-primary)] pt-5">
        <h3 className="font-semibold text-[var(--text-primary)]">Recent deliveries</h3>
        {controller.detailLoading ? (
          <p className="py-6 text-sm text-[var(--text-secondary)]">Loading deliveries...</p>
        ) : controller.deliveries.length ? (
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-[var(--text-secondary)]">
                <tr>
                  <th className="px-2 py-2">Received</th>
                  <th className="px-2 py-2">Event</th>
                  <th className="px-2 py-2">Repository</th>
                  <th className="px-2 py-2">Status</th>
                  <th className="px-2 py-2">Runs</th>
                </tr>
              </thead>
              <tbody>
                {controller.deliveries.map(delivery => (
                  <tr key={delivery.id} className="border-t border-[var(--border-primary)]">
                    <td className="px-2 py-3 whitespace-nowrap">{formatGitWebhookDate(delivery.received_at)}</td>
                    <td className="px-2 py-3 font-mono">{delivery.event_type || '-'}</td>
                    <td className="px-2 py-3 font-mono">{delivery.repository_full_name || '-'}</td>
                    <td className="px-2 py-3">
                      <span className={`runner-pill ${deliveryStatusClass(delivery.status)}`} title={delivery.error}>
                        {delivery.status}
                      </span>
                    </td>
                    <td className="px-2 py-3">{delivery.run_ids.length}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="py-6 text-sm text-[var(--text-secondary)]">No deliveries recorded yet.</p>
        )}
      </div>
    </section>
  );
}

function Fact({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3">
      <div className="flex items-center gap-2 text-xs font-semibold uppercase text-[var(--text-secondary)]">
        {icon}
        {label}
      </div>
      <div className="mt-1 text-sm capitalize text-[var(--text-primary)]">{value}</div>
    </div>
  );
}
