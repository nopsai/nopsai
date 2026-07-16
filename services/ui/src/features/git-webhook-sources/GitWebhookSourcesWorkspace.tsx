import { useMemo, type ReactNode } from 'react';
import { Activity, ArrowLeft, Copy, Edit3, GitBranch, PauseCircle, PlayCircle, ShieldCheck, Trash2, Webhook } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { TreeColumnResizeHandle, useResizableTreeColumn } from '../../components/resizableTreeColumn';
import { buildApiUrl } from '../../lib/api';
import { AutomationResourceTree } from '../event-automation/AutomationResourceTree';
import {
  buildAutomationResourceTree,
  findAutomationResourceTreeNode,
} from '../event-automation/resourceTreeModel';
import { CredentialReferenceLink } from '../system/credentials/CredentialReferenceLink';
import {
  buildGitWebhookSourceMetrics,
  deliveryStatusClass,
  formatGitWebhookDate,
  gitWebhookSourceConnectedCount,
  gitWebhookSourceTeamLabel,
  gitWebhookSourceVisibilityLabel,
  sourceStatusLabel,
  type GitWebhookDelivery,
  type GitWebhookSource,
  type GitWebhookSourceTreeItem,
} from './model';

type GitWebhookSourcesWorkspaceProps = {
  sources: GitWebhookSource[];
  visibleSources: GitWebhookSource[];
  treeItems: GitWebhookSourceTreeItem[];
  activeTeamPath: string;
  selected: GitWebhookSource | null;
  deliveries: GitWebhookDelivery[];
  loading: boolean;
  detailLoading: boolean;
  saving: boolean;
  searchTerm: string;
  canWrite: boolean;
  canDelete: boolean;
  onOpenTeam: (path: string) => void;
  onSelect: (sourceID: string) => void;
  onEdit: (source: GitWebhookSource) => void;
  onToggle: (source: GitWebhookSource) => void;
  onDelete: (source: GitWebhookSource) => void;
};

export function GitWebhookSourcesWorkspace({
  sources,
  visibleSources,
  treeItems,
  activeTeamPath,
  selected,
  deliveries,
  loading,
  detailLoading,
  saving,
  searchTerm,
  canWrite,
  canDelete,
  onOpenTeam,
  onSelect,
  onEdit,
  onToggle,
  onDelete,
}: GitWebhookSourcesWorkspaceProps) {
  const metrics = buildGitWebhookSourceMetrics(sources);
  const treeRoot = useMemo(() => buildAutomationResourceTree(treeItems), [treeItems]);
  const activeTeamNode = useMemo(
    () => findAutomationResourceTreeNode(treeRoot, activeTeamPath),
    [activeTeamPath, treeRoot]
  );
  const activeTeamLabel = activeTeamPath ? activeTeamPath : 'All teams';
  const emptyTitle = searchTerm.trim()
    ? 'No matching webhook sources'
    : activeTeamPath
      ? 'No webhook sources for this team'
      : 'No webhook sources found';
  const treeResize = useResizableTreeColumn({
    storageKey: 'event-automation',
    defaultWidth: 280,
    minWidth: 240,
    maxWidth: 520,
  });

  if (selected) {
    return (
      <section className="triggers-detail-fullscreen triggers-detail-fullscreen--with-tree" style={treeResize.gridStyle} aria-label="Git webhook source detail">
        <AutomationResourceTree
          title="Team tree"
          rootLabel="All teams"
          rootNode={treeRoot}
          items={treeItems}
          activePath={activeTeamPath}
          selectedID={selected.id}
          leafIconType="git-webhook-source"
          leafAriaLabel="Select webhook source"
          onOpenPath={onOpenTeam}
          onSelectItem={onSelect}
        />
        <TreeColumnResizeHandle {...treeResize} label="Resize team tree" />
        <div className="triggers-detail-fullscreen-main">
          <GitWebhookSourceDetail
            source={selected}
            deliveries={deliveries}
            detailLoading={detailLoading}
            saving={saving}
            canWrite={canWrite}
            canDelete={canDelete}
            onBack={() => onOpenTeam(activeTeamPath)}
            onEdit={onEdit}
            onToggle={onToggle}
            onDelete={onDelete}
          />
        </div>
      </section>
    );
  }

  return (
    <div className="triggers-workspace-panel triggers-workspace-panel--trigger-browser triggers-workspace-panel--summary">
      <div className="triggers-workspace-list triggers-browser" style={treeResize.gridStyle} aria-label="Git webhook source list">
        <AutomationResourceTree
          title="Team tree"
          rootLabel="All teams"
          rootNode={treeRoot}
          items={treeItems}
          activePath={activeTeamPath}
          selectedID={null}
          leafIconType="git-webhook-source"
          leafAriaLabel="Select webhook source"
          onOpenPath={onOpenTeam}
          onSelectItem={onSelect}
        />
        <TreeColumnResizeHandle {...treeResize} label="Resize team tree" />
        <section className="triggers-browser-main" aria-label="Git webhook source collection">
          <div className="triggers-metrics-grid" aria-label="Git webhook source summary">
            <Metric icon={<Webhook className="h-4 w-4" aria-hidden="true" />} label="Webhook sources" value={metrics.total} />
            <Metric icon={<Activity className="h-4 w-4" aria-hidden="true" />} label="Receiving" value={metrics.enabled} tone="green" />
            <Metric icon={<ObjectIcon type="gitops" />} label="GitOps managed" value={metrics.gitManaged} tone="blue" />
            <Metric icon={<ShieldCheck className="h-4 w-4" aria-hidden="true" />} label="Secured" value={metrics.secured} tone="amber" />
            <Metric icon={<Webhook className="h-4 w-4" aria-hidden="true" />} label="Workspace-shared" value={metrics.workspaceShared} tone="blue" />
          </div>

          <div className="triggers-list-container">
            {loading ? (
              <div className="triggers-workspace-empty">Loading sources...</div>
            ) : (
              <>
                <div className="triggers-collection-head">
                  <div>
                    <h3>{searchTerm.trim() ? 'Search results' : activeTeamLabel}</h3>
                    <p>
                      {visibleSources.length} source{visibleSources.length === 1 ? '' : 's'}
                      {searchTerm.trim() ? ` matching "${searchTerm.trim()}"` : ''}
                    </p>
                  </div>
                  {!searchTerm.trim() && activeTeamNode.children.length ? (
                    <span className="triggers-badge triggers-badge--neutral">
                      {activeTeamNode.children.length} nested team{activeTeamNode.children.length === 1 ? '' : 's'}
                    </span>
                  ) : null}
                </div>

                {visibleSources.length ? (
                  <div className="triggers-resource-table-shell">
                    <table className="triggers-resource-table">
                      <thead>
                        <tr>
                          <th scope="col">Source</th>
                          <th scope="col">Provider</th>
                          <th scope="col">Owner</th>
                          <th scope="col">Visibility</th>
                          <th scope="col">Repositories allowed</th>
                          <th scope="col">Triggers connected</th>
                          <th scope="col">Last used</th>
                          <th scope="col">Status</th>
                        </tr>
                      </thead>
                      <tbody>
                        {visibleSources.map(source => (
                          <GitWebhookSourceRow
                            key={source.id}
                            source={source}
                            selected={false}
                            onSelect={onSelect}
                          />
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <div className="triggers-workspace-empty">
                    <span className="triggers-empty-icon" aria-hidden="true">
                      <ObjectIcon type="git-webhook-source" />
                    </span>
                    <strong>{emptyTitle}</strong>
                    <span>{searchTerm.trim() ? 'Adjust your search.' : 'Create a source or browse another team.'}</span>
                  </div>
                )}
              </>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

function Metric({
  icon,
  label,
  value,
  tone,
}: {
  icon: ReactNode;
  label: string;
  value: number;
  tone?: 'green' | 'blue' | 'amber';
}) {
  return (
    <div className="triggers-metric">
      <span className={`triggers-metric-icon ${tone ? `triggers-metric-icon--${tone}` : ''}`}>{icon}</span>
      <span className="triggers-metric-label">{label}</span>
      <strong className="triggers-metric-value">{value}</strong>
    </div>
  );
}

function GitWebhookSourceRow({
  source,
  selected,
  onSelect,
}: {
  source: GitWebhookSource;
  selected: boolean;
  onSelect: (sourceID: string) => void;
}) {
  const label = sourceStatusLabel(source);
  return (
    <tr className={selected ? 'selected' : ''} onClick={() => onSelect(source.id)}>
      <td>
        <button type="button" className="triggers-resource-cell" onClick={event => {
          event.stopPropagation();
          onSelect(source.id);
        }}>
          <span className="triggers-resource-icon triggers-resource-icon--webhook" aria-hidden="true">
            <ObjectIcon type="git-webhook-source" />
          </span>
          <span className="triggers-resource-name">
            <strong>{source.name || source.id}</strong>
          </span>
        </button>
      </td>
      <td>
        <span className="triggers-mono">{source.provider}</span>
      </td>
      <td>
        <span className="triggers-mono">{gitWebhookSourceTeamLabel(source)}</span>
      </td>
      <td>
        <span className="triggers-mono">{gitWebhookSourceVisibilityLabel(source.visibility)}</span>
      </td>
      <td>
        <span className="triggers-mono">{source.repository_allowlist.length}</span>
      </td>
      <td>
        <span className="triggers-mono">{gitWebhookSourceConnectedCount(source)}</span>
      </td>
      <td>
        <span className="triggers-mono">{formatGitWebhookDate(source.last_used_at)}</span>
      </td>
      <td>
        <span className={`triggers-badge triggers-badge--${source.enabled ? 'green' : 'neutral'}`}>
          <span className="triggers-badge-dot" aria-hidden="true"></span>
          {label}
        </span>
      </td>
    </tr>
  );
}

function GitWebhookSourceDetail({
  source,
  deliveries,
  detailLoading,
  saving,
  canWrite,
  canDelete,
  onEdit,
  onToggle,
  onDelete,
  onBack,
}: {
  source: GitWebhookSource;
  deliveries: GitWebhookDelivery[];
  detailLoading: boolean;
  saving: boolean;
  canWrite: boolean;
  canDelete: boolean;
  onEdit: (source: GitWebhookSource) => void;
  onToggle: (source: GitWebhookSource) => void;
  onDelete: (source: GitWebhookSource) => void;
  onBack: () => void;
}) {
  const managed = Boolean(source.managed_by_config_repo);
  const endpoint = buildApiUrl(`/v1/git/webhooks/${encodeURIComponent(source.id)}`);

  return (
    <div className="triggers-detail-pane">
      <div className="triggers-detail-scroll">
        <div className="triggers-detail-head">
          <span className="triggers-detail-icon triggers-detail-icon--webhook" aria-hidden="true">
            <ObjectIcon type="git-webhook-source" className="h-5 w-5" />
          </span>
          <div className="triggers-detail-title">
            <h2>{source.name || source.id}</h2>
            <p>{source.id}</p>
          </div>
          <span className={`triggers-badge triggers-badge--${source.enabled ? 'green' : 'neutral'}`}>
            <span className="triggers-badge-dot" aria-hidden="true"></span>
            {sourceStatusLabel(source)}
          </span>
          <div className="triggers-detail-actions">
            <button type="button" className="triggers-mini-button" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>List</span>
            </button>
            {canWrite ? (
              <>
                <button type="button" className="triggers-mini-button" disabled={saving} onClick={() => onToggle(source)} title={managed ? 'Save database override; GitOps can replace it on next sync' : source.enabled ? 'Disable source' : 'Enable source'}>
                  {source.enabled ? <PauseCircle className="h-4 w-4" aria-hidden="true" /> : <PlayCircle className="h-4 w-4" aria-hidden="true" />}
                  <span>{source.enabled ? 'Disable' : 'Enable'}</span>
                </button>
                <button type="button" className="triggers-mini-button" disabled={saving} onClick={() => onEdit(source)} title={managed ? 'Save database override; GitOps can replace it on next sync' : 'Edit source'}>
                  <Edit3 className="h-4 w-4" aria-hidden="true" />
                  <span>Edit</span>
                </button>
              </>
            ) : null}
            {canDelete ? (
              <button type="button" className="triggers-mini-button triggers-mini-button--danger" disabled={saving} onClick={() => onDelete(source)} title={managed ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete source'}>
                <Trash2 className="h-4 w-4" aria-hidden="true" />
                <span>Delete</span>
              </button>
            ) : null}
          </div>
        </div>

        <div className="triggers-detail-page-grid">
          <div className="triggers-detail-column">
            <section className="triggers-detail-panel" aria-labelledby="git-webhook-overview-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="git-webhook-overview-heading">Overview</h3>
              </div>
              <div className="triggers-detail-panel-body">
                {source.description ? <p className="description">{source.description}</p> : null}
                <div className="triggers-facts-grid">
                  <Fact label="Provider" value={source.provider} />
                  <Fact label="Authentication" value={source.auth_mode} />
                  <Fact label="Owner" value={gitWebhookSourceTeamLabel(source)} />
                  <Fact label="Visibility" value={gitWebhookSourceVisibilityLabel(source.visibility)} />
                  <Fact label="Repositories allowed" value={String(source.repository_allowlist.length)} />
                  <Fact label="Triggers connected" value={String(gitWebhookSourceConnectedCount(source))} />
                  <Fact label="Last delivery" value={formatGitWebhookDate(source.last_used_at)} />
                  <Fact label="Rate limit" value={formatRateLimit(source.rate_limit)} />
                  <Fact label="Source" value={managed ? `GitOps ${source.config_source_path || ''}`.trim() : source.source || 'database'} />
                </div>
                {source.allowlist_unconfigured_repositories?.length ? (
                  <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-[var(--text-secondary)]">
                    Repository allowed, but no NopsAI trigger is configured. Webhook events will not start pipelines.
                  </div>
                ) : null}
              </div>
            </section>

            <section className="triggers-detail-panel" aria-labelledby="git-webhook-allowlist-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="git-webhook-allowlist-heading">Repository allowlist</h3>
                <GitBranch className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
              </div>
              <div className="triggers-detail-panel-body">
                <div className="triggers-chip-row">
                  {source.repository_allowlist.map(pattern => (
                    <span key={pattern} className="triggers-chip triggers-mono">{pattern}</span>
                  ))}
                </div>
              </div>
            </section>

            {source.generated_credential ? (
              <section className="triggers-detail-panel" aria-labelledby="git-webhook-generated-credential-heading">
                <div className="triggers-detail-panel-head">
                  <h3 id="git-webhook-generated-credential-heading">Generated webhook secret</h3>
                  <ShieldCheck className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
                </div>
                <div className="triggers-detail-panel-body">
                  <p className="description">Shown once. Copy this value into the Git provider before leaving this source.</p>
                  <div className="triggers-endpoint triggers-endpoint--wrap">
                    <code>{source.generated_credential.value}</code>
                    <button
                      type="button"
                      className="triggers-mini-button"
                      aria-label="Copy generated webhook secret"
                      onClick={() => copyText(source.generated_credential?.value || '')}
                    >
                      <Copy className="h-4 w-4" aria-hidden="true" />
                      <span>Copy</span>
                    </button>
                  </div>
                  <p className="triggers-mono break-all text-[var(--text-primary)]">
                    <CredentialReferenceLink reference={source.generated_credential.reference} />
                  </p>
                </div>
              </section>
            ) : null}

            {source.credential_ref ? (
              <section className="triggers-detail-panel" aria-labelledby="git-webhook-credential-heading">
                <div className="triggers-detail-panel-head">
                  <h3 id="git-webhook-credential-heading">Credential reference</h3>
                  <ShieldCheck className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
                </div>
                <div className="triggers-detail-panel-body">
                  <p className="triggers-mono break-all text-[var(--text-primary)]">
                    <CredentialReferenceLink reference={source.credential_ref} />
                  </p>
                </div>
              </section>
            ) : null}

            <section className="triggers-detail-panel" aria-labelledby="git-webhook-connected-triggers-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="git-webhook-connected-triggers-heading">Connected triggers</h3>
                <span className="triggers-badge triggers-badge--neutral">{gitWebhookSourceConnectedCount(source)}</span>
              </div>
              <div className="triggers-detail-panel-body">
                {source.connected_triggers?.length ? (
                  <ul className="space-y-2">
                    {source.connected_triggers.map(trigger => (
                      <li key={trigger.repository_name} className="rounded-md border border-[var(--border-primary)] px-3 py-2">
                        <div className="font-mono text-sm text-[var(--text-primary)]">{trigger.team_path || 'root'} / {trigger.repository_for_webhook || trigger.repository_name}</div>
                        <div className="mt-1 text-xs text-[var(--text-secondary)]">{trigger.management || 'nopsai'} / {trigger.provider}</div>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="description">No repository triggers are assigned to this source.</p>
                )}
              </div>
            </section>
          </div>

          <div className="triggers-detail-column triggers-detail-column--definition">
            <section className="triggers-detail-panel" aria-labelledby="git-webhook-endpoint-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="git-webhook-endpoint-heading">Webhook endpoint</h3>
                <span className="triggers-badge triggers-badge--blue">{source.provider}</span>
              </div>
              <div className="triggers-detail-panel-body">
                <div className="triggers-endpoint">
                  <code>{endpoint}</code>
                </div>
              </div>
            </section>

            <section className="triggers-detail-panel" aria-labelledby="git-webhook-deliveries-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="git-webhook-deliveries-heading">Recent deliveries</h3>
              </div>
              <div className="triggers-detail-panel-body">
                {detailLoading ? (
                  <p className="description">Loading deliveries...</p>
                ) : deliveries.length ? (
                  <div className="triggers-resource-table-shell">
                    <table className="triggers-resource-table triggers-resource-table--compact">
                      <thead>
                        <tr>
                          <th scope="col">Received</th>
                          <th scope="col">Event</th>
                          <th scope="col">Repository</th>
                          <th scope="col">Status</th>
                        </tr>
                      </thead>
                      <tbody>
                        {deliveries.map(delivery => (
                          <tr key={delivery.id}>
                            <td>{formatGitWebhookDate(delivery.received_at)}</td>
                            <td className="triggers-mono">{delivery.event_type || '-'}</td>
                            <td className="triggers-mono">{delivery.repository_full_name || '-'}</td>
                            <td>
                              <span className={`runner-pill ${deliveryStatusClass(delivery.status)}`} title={delivery.error}>
                                {delivery.status}
                              </span>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p className="description">No deliveries recorded yet.</p>
                )}
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="triggers-fact">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function formatRateLimit(rateLimit: Record<string, unknown>): string {
  const perMinute = rateLimit.per_minute;
  if (typeof perMinute === 'number' && Number.isFinite(perMinute) && perMinute > 0) return `${perMinute}/minute`;
  if (typeof perMinute === 'string' && perMinute.trim()) return `${perMinute}/minute`;
  return 'Unlimited';
}

function copyText(value: string) {
  if (!value || typeof navigator === 'undefined' || !navigator.clipboard) return;
  void navigator.clipboard.writeText(value);
}
