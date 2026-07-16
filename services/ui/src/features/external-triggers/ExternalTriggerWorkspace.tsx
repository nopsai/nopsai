import { useMemo, type ReactNode } from 'react';
import { ArrowLeft, CheckCircle2, Clipboard, Copy, Edit3, History, PauseCircle, PlayCircle, RefreshCw, Shield, Trash2, Zap } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { TreeColumnResizeHandle, useResizableTreeColumn } from '../../components/resizableTreeColumn';
import { AutomationResourceTree } from '../event-automation/AutomationResourceTree';
import {
  buildAutomationResourceTree,
  findAutomationResourceTreeNode,
} from '../event-automation/resourceTreeModel';
import {
  buildExternalTriggerCollectionMetrics,
  externalTriggerRelativeLabel,
  externalTriggerScopeLabel,
  externalTriggerSourceLabel,
  externalTriggerTeamLabel,
  type ExternalTrigger,
  type ExternalTriggerInvocation,
  type ExternalTriggerTreeItem,
} from './model';

type ExternalTriggerWorkspaceProps = {
  triggers: ExternalTrigger[];
  visibleTriggers: ExternalTrigger[];
  treeItems: ExternalTriggerTreeItem[];
  activeTeamPath: string;
  selectedID: string;
  selectedTrigger: ExternalTrigger | null;
  invocations: ExternalTriggerInvocation[];
  loading: boolean;
  detailLoading: boolean;
  invocationsLoading: boolean;
  canWrite: boolean;
  canDelete: boolean;
  deletePending: boolean;
  copyState: string;
  invokeURL: string;
  exampleCurl: string;
  searchTerm: string;
  onOpenTeam: (path: string) => void;
  onSelect: (id: string) => void;
  onEdit: (trigger: ExternalTrigger) => void;
  onToggle: (trigger: ExternalTrigger) => void;
  onCopyURL: () => void;
  onCopyCurl: () => void;
  onDelete: (trigger: ExternalTrigger) => void;
  onRefreshInvocations: (id: string) => void;
};

export function ExternalTriggerWorkspace({
  triggers,
  visibleTriggers,
  treeItems,
  activeTeamPath,
  selectedID,
  selectedTrigger,
  invocations,
  loading,
  detailLoading,
  invocationsLoading,
  canWrite,
  canDelete,
  deletePending,
  copyState,
  invokeURL,
  exampleCurl,
  searchTerm,
  onOpenTeam,
  onSelect,
  onEdit,
  onToggle,
  onCopyURL,
  onCopyCurl,
  onDelete,
  onRefreshInvocations,
}: ExternalTriggerWorkspaceProps) {
  const metrics = buildExternalTriggerCollectionMetrics(triggers);
  const treeRoot = useMemo(() => buildAutomationResourceTree(treeItems), [treeItems]);
  const activeTeamNode = useMemo(
    () => findAutomationResourceTreeNode(treeRoot, activeTeamPath),
    [activeTeamPath, treeRoot]
  );
  const activeTeamLabel = activeTeamPath ? activeTeamPath : 'All teams';
  const emptyTitle = searchTerm.trim()
    ? 'No matching external triggers'
    : activeTeamPath
      ? 'No external triggers for this team'
      : 'No external triggers found';
  const treeResize = useResizableTreeColumn({
    storageKey: 'event-automation',
    defaultWidth: 280,
    minWidth: 240,
    maxWidth: 520,
  });

  if (selectedTrigger) {
    return (
      <section className="triggers-detail-fullscreen triggers-detail-fullscreen--with-tree" style={treeResize.gridStyle} aria-label="External trigger detail">
        <AutomationResourceTree
          title="Team tree"
          rootLabel="All teams"
          rootNode={treeRoot}
          items={treeItems}
          activePath={activeTeamPath}
          selectedID={selectedID}
          leafIconType="external-trigger"
          leafAriaLabel="Select external trigger"
          onOpenPath={onOpenTeam}
          onSelectItem={onSelect}
        />
        <TreeColumnResizeHandle {...treeResize} label="Resize team tree" />
        <div className="triggers-detail-fullscreen-main">
          <ExternalTriggerDetail
            trigger={selectedTrigger}
            invocations={invocations}
            detailLoading={detailLoading}
            invocationsLoading={invocationsLoading}
            canWrite={canWrite}
            canDelete={canDelete}
            deletePending={deletePending}
            copyState={copyState}
            invokeURL={invokeURL}
            exampleCurl={exampleCurl}
            onBack={() => onOpenTeam(activeTeamPath)}
            onEdit={onEdit}
            onToggle={onToggle}
            onCopyURL={onCopyURL}
            onCopyCurl={onCopyCurl}
            onDelete={onDelete}
            onRefreshInvocations={onRefreshInvocations}
          />
        </div>
      </section>
    );
  }

  return (
    <div className="triggers-workspace-panel triggers-workspace-panel--trigger-browser triggers-workspace-panel--summary">
      <div className="triggers-workspace-list triggers-browser" style={treeResize.gridStyle} aria-label="External trigger list">
        <AutomationResourceTree
          title="Team tree"
          rootLabel="All teams"
          rootNode={treeRoot}
          items={treeItems}
          activePath={activeTeamPath}
          selectedID={null}
          leafIconType="external-trigger"
          leafAriaLabel="Select external trigger"
          onOpenPath={onOpenTeam}
          onSelectItem={onSelect}
        />
        <TreeColumnResizeHandle {...treeResize} label="Resize team tree" />
        <section className="triggers-browser-main" aria-label="External trigger collection">
          <div className="triggers-metrics-grid" aria-label="External trigger summary">
            <Metric icon={<Zap className="h-4 w-4" aria-hidden="true" />} label="API endpoints" value={metrics.total} />
            <Metric icon={<CheckCircle2 className="h-4 w-4" aria-hidden="true" />} label="Enabled" value={metrics.enabled} tone="green" />
            <Metric icon={<ObjectIcon type="gitops" />} label="GitOps managed" value={metrics.gitManaged} tone="blue" />
            <Metric icon={<Shield className="h-4 w-4" aria-hidden="true" />} label="Caller policies" value={metrics.callerPolicies} tone="amber" />
          </div>

          <div className="triggers-list-container">
            {loading ? (
              <div className="triggers-workspace-empty">Loading external triggers...</div>
            ) : (
              <>
                <div className="triggers-collection-head">
                  <div>
                    <h3>{searchTerm.trim() ? 'Search results' : activeTeamLabel}</h3>
                    <p>
                      {visibleTriggers.length} endpoint{visibleTriggers.length === 1 ? '' : 's'}
                      {searchTerm.trim() ? ` matching "${searchTerm.trim()}"` : ''}
                    </p>
                  </div>
                  {!searchTerm.trim() && activeTeamNode.children.length ? (
                    <span className="triggers-badge triggers-badge--neutral">
                      {activeTeamNode.children.length} nested team{activeTeamNode.children.length === 1 ? '' : 's'}
                    </span>
                  ) : null}
                </div>

                {visibleTriggers.length ? (
                  <div className="triggers-resource-table-shell">
                    <table className="triggers-resource-table">
                      <thead>
                        <tr>
                          <th scope="col">Name</th>
                          <th scope="col">Pipeline</th>
                          <th scope="col">Team</th>
                          <th scope="col">Scope</th>
                          <th scope="col">Last used</th>
                          <th scope="col">Source</th>
                        </tr>
                      </thead>
                      <tbody>
                        {visibleTriggers.map(trigger => (
                          <ExternalTriggerRow
                            key={trigger.id}
                            trigger={trigger}
                            selected={trigger.id === selectedID}
                            onSelect={onSelect}
                          />
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <div className="triggers-workspace-empty">
                    <span className="triggers-empty-icon" aria-hidden="true">
                      <ObjectIcon type="external-trigger" />
                    </span>
                    <strong>{emptyTitle}</strong>
                    <span>{searchTerm.trim() ? 'Adjust your search.' : 'Create an authenticated endpoint or browse another team.'}</span>
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

function ExternalTriggerRow({
  trigger,
  selected,
  onSelect,
}: {
  trigger: ExternalTrigger;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  const sourceLabel = externalTriggerSourceLabel(trigger);
  return (
    <tr className={selected ? 'selected' : ''} onClick={() => onSelect(trigger.id)}>
      <td>
        <button type="button" className="triggers-resource-cell" onClick={event => {
          event.stopPropagation();
          onSelect(trigger.id);
        }}>
          <span className="triggers-resource-icon triggers-resource-icon--external" aria-hidden="true">
            <ObjectIcon type="external-trigger" />
          </span>
          <span className="triggers-resource-name">
            <strong>{trigger.name || trigger.id}</strong>
          </span>
        </button>
      </td>
      <td>
        <span className="triggers-mono">{trigger.pipeline || '-'}</span>
      </td>
      <td>
        <span className="triggers-mono">{externalTriggerTeamLabel(trigger.run_team_path)}</span>
      </td>
      <td>
        <span className="triggers-mono">{externalTriggerScopeLabel(trigger.scope)}</span>
      </td>
      <td>
        <span className="triggers-mono">{formatDate(trigger.last_used_at)}</span>
      </td>
      <td>
        <span className={`triggers-badge triggers-badge--${trigger.managed_by_config_repo ? 'blue' : 'neutral'}`}>
          <span className="triggers-badge-dot" aria-hidden="true"></span>
          {sourceLabel}
        </span>
      </td>
    </tr>
  );
}

function ExternalTriggerDetail({
  trigger,
  invocations,
  detailLoading,
  invocationsLoading,
  canWrite,
  canDelete,
  deletePending,
  copyState,
  invokeURL,
  exampleCurl,
  onEdit,
  onToggle,
  onCopyURL,
  onCopyCurl,
  onDelete,
  onRefreshInvocations,
  onBack,
}: {
  trigger: ExternalTrigger;
  invocations: ExternalTriggerInvocation[];
  detailLoading: boolean;
  invocationsLoading: boolean;
  canWrite: boolean;
  canDelete: boolean;
  deletePending: boolean;
  copyState: string;
  invokeURL: string;
  exampleCurl: string;
  onEdit: (trigger: ExternalTrigger) => void;
  onToggle: (trigger: ExternalTrigger) => void;
  onCopyURL: () => void;
  onCopyCurl: () => void;
  onDelete: (trigger: ExternalTrigger) => void;
  onRefreshInvocations: (id: string) => void;
  onBack: () => void;
}) {
  const managed = Boolean(trigger.managed_by_config_repo);
  return (
    <div className="triggers-detail-pane">
      <div className="triggers-detail-scroll">
        <div className="triggers-detail-head">
          <span className="triggers-detail-icon triggers-detail-icon--external" aria-hidden="true">
            <ObjectIcon type="external-trigger" className="h-5 w-5" />
          </span>
          <div className="triggers-detail-title">
            <h2>{trigger.name || trigger.id}</h2>
            <p>{trigger.id}</p>
          </div>
          <span className={`triggers-badge triggers-badge--${trigger.enabled ? 'green' : 'neutral'}`}>
            <span className="triggers-badge-dot" aria-hidden="true"></span>
            {trigger.enabled ? 'Enabled' : 'Disabled'}
          </span>
          <div className="triggers-detail-actions">
            <button type="button" className="triggers-mini-button" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>List</span>
            </button>
            {canWrite ? (
              <>
                <button type="button" className="triggers-mini-button" onClick={() => onEdit(trigger)} title={managed ? 'Save database override; GitOps can replace it on next sync' : 'Edit'}>
                  <Edit3 className="h-4 w-4" aria-hidden="true" />
                  <span>Edit</span>
                </button>
                <button type="button" className="triggers-mini-button" onClick={() => onToggle(trigger)} title={managed ? 'Save database override; GitOps can replace it on next sync' : trigger.enabled ? 'Disable' : 'Enable'}>
                  {trigger.enabled ? <PauseCircle className="h-4 w-4" aria-hidden="true" /> : <PlayCircle className="h-4 w-4" aria-hidden="true" />}
                  <span>{trigger.enabled ? 'Disable' : 'Enable'}</span>
                </button>
              </>
            ) : null}
            <button type="button" className="triggers-mini-button" onClick={onCopyURL}>
              <Copy className="h-4 w-4" aria-hidden="true" />
              <span>{copyState === 'url' ? 'Copied' : 'URL'}</span>
            </button>
            {canDelete ? (
              <button type="button" className="triggers-mini-button triggers-mini-button--danger" onClick={() => onDelete(trigger)} disabled={deletePending} title={managed ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete'}>
                <Trash2 className="h-4 w-4" aria-hidden="true" />
                <span>Delete</span>
              </button>
            ) : null}
          </div>
        </div>

        <div className="triggers-detail-page-grid">
          <div className="triggers-detail-column">
            <section className="triggers-detail-panel" aria-labelledby="external-trigger-overview-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="external-trigger-overview-heading">Overview</h3>
              </div>
              <div className="triggers-detail-panel-body">
                {trigger.description ? <p className="description">{trigger.description}</p> : null}
                <div className="triggers-facts-grid">
                  <Fact label="Target pipeline" value={trigger.pipeline} />
                  <Fact label="Scope" value={externalTriggerScopeLabel(trigger.scope)} />
                  <Fact label="Run team" value={externalTriggerTeamLabel(trigger.run_team_path)} />
                  <Fact label="Source" value={managed ? `GitOps ${trigger.config_source_path || ''}`.trim() : trigger.source || 'database'} />
                  <Fact label="Created by" value={trigger.created_by || '-'} />
                  <Fact label="Last used" value={formatDate(trigger.last_used_at)} />
                </div>
              </div>
            </section>

            <section className="triggers-detail-panel" aria-labelledby="external-trigger-callers-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="external-trigger-callers-heading">Allowed callers</h3>
                <Shield className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
              </div>
              <div className="triggers-detail-panel-body">
                <div className="triggers-chip-row">
                  {(trigger.allowed_callers || []).length ? (
                    (trigger.allowed_callers || []).map(caller => (
                      <span key={`${caller.type}:${caller.id}`} className="triggers-chip triggers-mono">{caller.type}:{caller.id}</span>
                    ))
                  ) : (
                    <span className="triggers-chip">No callers configured</span>
                  )}
                </div>
              </div>
            </section>

            <section className="triggers-detail-panel" aria-labelledby="external-trigger-invocations-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="external-trigger-invocations-heading">
                  <History className="h-4 w-4" aria-hidden="true" />
                  Recent invocations
                </h3>
                <button type="button" className="triggers-icon-button" title="Refresh invocations" aria-label="Refresh invocations" onClick={() => onRefreshInvocations(trigger.id)}>
                  <RefreshCw className="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              <div className="triggers-detail-panel-body">
                {detailLoading || invocationsLoading ? (
                  <p className="description">Loading details...</p>
                ) : invocations.length ? (
                  <div className="triggers-timeline">
                    {invocations.map(invocation => (
                      <div key={invocation.id} className="triggers-event">
                        <span className={`triggers-event-dot ${invocation.status === 'failed' ? 'red' : invocation.status === 'queued' ? 'green' : ''}`} aria-hidden="true"></span>
                        <div>
                          <strong>{invocation.status}</strong>
                          <p>{invocation.caller_type}:{invocation.caller_id}{invocation.event_type ? ` · ${invocation.event_type}` : ''}</p>
                          {invocation.error ? <p className="text-red-500">{invocation.error}</p> : null}
                        </div>
                        <time>{externalTriggerRelativeLabel(invocation.created_at)}</time>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="description">No invocations yet.</p>
                )}
              </div>
            </section>
          </div>

          <div className="triggers-detail-column triggers-detail-column--definition">
            <section className="triggers-detail-panel" aria-labelledby="external-trigger-endpoint-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="external-trigger-endpoint-heading">Invocation endpoint</h3>
                <span className="triggers-badge triggers-badge--blue">POST</span>
              </div>
              <div className="triggers-detail-panel-body">
                <div className="triggers-endpoint">
                  <code>{invokeURL}</code>
                  <button type="button" className="triggers-icon-button" aria-label="Copy invocation URL" onClick={onCopyURL}>
                    <Copy className="h-4 w-4" aria-hidden="true" />
                  </button>
                </div>
              </div>
            </section>

            <section className="triggers-detail-panel" aria-labelledby="external-trigger-curl-heading">
              <div className="triggers-detail-panel-head">
                <h3 id="external-trigger-curl-heading">Example curl</h3>
                <button type="button" className="triggers-icon-button" title="Copy curl" aria-label="Copy curl" onClick={onCopyCurl}>
                  {copyState === 'curl' ? <CheckCircle2 className="h-4 w-4" aria-hidden="true" /> : <Clipboard className="h-4 w-4" aria-hidden="true" />}
                </button>
              </div>
              <div className="triggers-detail-panel-body">
                <pre className="triggers-code">{exampleCurl}</pre>
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
      <strong>{value || '-'}</strong>
    </div>
  );
}

function formatDate(value?: string) {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Never';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
