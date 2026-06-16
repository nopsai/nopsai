import { useMemo, useState } from 'react';
import {
  Activity,
  Edit3,
  GitBranch,
  PauseCircle,
  PlayCircle,
  ShieldCheck,
  Trash2,
  Webhook,
} from 'lucide-react';
import { ResourceCollectionToolbar } from '../editor/ResourceCollectionToolbar';
import { buildApiUrl } from '../../lib/api';
import { GitWebhookSourceForm } from './GitWebhookSourceForm';
import { GitWebhookSourceCards } from './GitWebhookSourceCards';
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
  const [searchTerm, setSearchTerm] = useState('');
  const filteredSources = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    if (!term) return controller.sources;
    return controller.sources.filter(source => [
      source.id,
      source.name,
      source.description,
      source.provider,
      source.auth_mode,
      source.credential_ref,
      source.source,
      ...source.repository_allowlist,
    ].join(' ').toLowerCase().includes(term));
  }, [controller.sources, searchTerm]);

  return (
    <div data-page="git-webhook-sources" className="active h-full flex flex-col">
      <ResourceCollectionToolbar
        resourceLabel="webhook source"
        searchTerm={searchTerm}
        canCreate={canWrite}
        createLabel="New source"
        createDisabledReason="You have read-only access to Git webhook sources"
        showCreateWhenDisabled
        onSearchTermChange={setSearchTerm}
        onCreate={controller.startCreate}
        onRefresh={() => void controller.loadSources()}
        refreshDisabled={controller.loading || controller.saving}
        filters={!canWrite ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
      />
      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {controller.error && !controller.editorOpen ? (
          <div className="mb-4 rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
            {controller.error}
          </div>
        ) : null}
        <div className={`grid items-start gap-6 ${selected ? 'xl:grid-cols-[minmax(280px,0.7fr)_minmax(0,1.3fr)]' : ''}`}>
          <section className="min-w-0">
            {controller.loading ? (
              <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading sources...</div>
            ) : filteredSources.length ? (
              <GitWebhookSourceCards
                sources={filteredSources}
                selectedID={selected?.id}
                onSelect={controller.onSelect}
              />
            ) : (
              <div className="pipelines-empty">
                <h2 className="text-base font-semibold text-[var(--text-primary)]">No webhook sources found</h2>
                <p className="text-sm text-[var(--text-secondary)]">
                  {searchTerm.trim() ? 'Adjust your search.' : 'Create a source to receive repository webhook events.'}
                </p>
              </div>
            )}
          </section>

          {selected ? (
            <SourceDetail
              source={selected}
              controller={controller}
              canWrite={canWrite}
              canDelete={canDelete}
            />
          ) : null}
        </div>
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
          {canWrite ? (
            <>
              <button
                type="button"
                className="glass-button-ghost"
                disabled={controller.saving}
                onClick={() => void controller.setEnabled(source, !source.enabled)}
                title={managed ? 'Save database override; GitOps can replace it on next sync' : source.enabled ? 'Disable source' : 'Enable source'}
              >
                {source.enabled ? <PauseCircle className="h-4 w-4" /> : <PlayCircle className="h-4 w-4" />}
              </button>
              <button
                type="button"
                className="glass-button-ghost"
                disabled={controller.saving}
                onClick={() => controller.startEdit(source)}
                title={managed ? 'Save database override; GitOps can replace it on next sync' : 'Edit source'}
              >
                <Edit3 className="h-4 w-4" />
              </button>
            </>
          ) : null}
          {canDelete ? (
            <button
              type="button"
              className="glass-button-danger"
              disabled={controller.saving}
              onClick={() => void controller.remove(source)}
              title={managed ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete source'}
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
