import { useState, type ReactNode } from 'react';
import { ArrowLeft, Copy, Download, Edit3, ExternalLink, GitBranch, Lock, MoreHorizontal, Plus, RotateCw, ShieldCheck, Trash2 } from 'lucide-react';

import ResourceAccessCard from '../../components/ResourceAccessCard';
import { ObjectIcon } from '../../components/ObjectIcon';
import type { ObjectIconType } from '../../components/objectIconRegistry';
import {
  isGitManagedDocument,
  isExternalKnowledgeDocument,
  knowledgeConnectionDisplayName,
  knowledgeConnectionMatchesIdentifier,
  knowledgeFailureModeOptions,
  knowledgeSyncModeOptions,
  knowledgeSyncStatusLabel,
  normalizeTeamPath,
  normalizeKnowledgeSource,
  type KnowledgeConnectionListItem,
  type KnowledgeContextDetail,
  type KnowledgeContextListItem,
  type KnowledgeFailureMode,
  type KnowledgeSyncMode,
} from './model';
import { formatKnowledgeDate, kindIconType } from './presentation';

type KnowledgeDetailTab = 'overview' | 'content' | 'usage' | 'gitops';

const detailTabs: Array<{ id: KnowledgeDetailTab; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'content', label: 'Content' },
  { id: 'usage', label: 'Usage' },
  { id: 'gitops', label: 'GitOps' },
];

export type KnowledgeContentMetrics = {
  lines: number;
  words: number;
  chars: number;
};

export type KnowledgeContextDetailViewProps = {
  detail: KnowledgeContextDetail | null;
  editorValue: string;
  previewContent: string;
  contentMetrics: KnowledgeContentMetrics;
  detailError: string | null;
  draftID: string | null;
  isEditing: boolean;
  canEditSelected: boolean;
  selectedCanEdit: boolean;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  saving: boolean;
  syncing: boolean;
  connections: KnowledgeConnectionListItem[];
  onBackToList: () => void;
  onCopy: () => void;
  onDownload: () => void;
  onStartEditing: () => void;
  onClone: () => void;
  onDiscardEditing: () => void;
  onSave: () => void;
  onSyncNow: () => void;
  onDelete: (doc: KnowledgeContextListItem) => void;
  onDescriptionChange: (value: string) => void;
  onDetailPatch: (patch: Partial<KnowledgeContextDetail>) => void;
  onContentChange: (value: string) => void;
  onAccessChange: (access: { resource_id?: string; visibility?: string }) => void;
  onOpenPipeline: (pipelineID: string) => void;
  onCreateDocument?: () => void;
};

export function KnowledgeContextDetailView({
  detail,
  editorValue,
  previewContent,
  contentMetrics,
  detailError,
  draftID,
  isEditing,
  canEditSelected,
  selectedCanEdit,
  canWriteKnowledge,
  canDeleteKnowledge,
  saving,
  syncing,
  connections,
  onBackToList,
  onCopy,
  onDownload,
  onStartEditing,
  onClone,
  onDiscardEditing,
  onSave,
  onSyncNow,
  onDelete,
  onDescriptionChange,
  onDetailPatch,
  onContentChange,
  onAccessChange,
  onOpenPipeline,
  onCreateDocument,
}: KnowledgeContextDetailViewProps) {
  const [activeTab, setActiveTab] = useState<KnowledgeDetailTab>(() => (isEditing ? 'content' : 'overview'));
  const [actionsOpen, setActionsOpen] = useState(false);

  if (!detail) {
    return (
      <section className="kc-demo-detail" aria-label="Knowledge Context detail">
        <div className="kc-demo-detail-empty">
          <ObjectIcon type="knowledge-context" className="h-6 w-6" />
          <strong>{detailError ? 'Unable to load knowledge context' : 'Select a knowledge context'}</strong>
          <span>{detailError || 'Choose a document from the browser to inspect content, access, GitOps state, and usage.'}</span>
          {canWriteKnowledge && onCreateDocument ? (
            <button type="button" className="kc-demo-primary-btn" onClick={onCreateDocument}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New context
            </button>
          ) : null}
        </div>
      </section>
    );
  }

  const nameLabel = detail.name || detail.id;
  const source = normalizeKnowledgeSource(detail.source);
  const isExternal = isExternalKnowledgeDocument(detail);
  const syncBadge = knowledgeSyncStatusLabel(detail.sync_status, isExternal);
  const isDraftDocument = Boolean(draftID && !detail.uuid);
  const updatedLabel = isDraftDocument ? 'Unsaved' : formatKnowledgeDate(detail.updated_at);
  const rawConnectionID = detail.connection_ref || detail.connection_id || '';
  const selectedConnection = connections.find(connection => knowledgeConnectionMatchesIdentifier(connection, rawConnectionID));
  const selectedConnectionID = selectedConnection?.id || rawConnectionID;
  const handleStartEditing = () => {
    setActionsOpen(false);
    setActiveTab('content');
    onStartEditing();
  };

  return (
    <section className="kc-demo-detail" aria-label="Knowledge Context detail">
      <div className="kc-demo-detail-head">
        <div className="kc-demo-resource-head">
          <div className="kc-demo-resource-title">
            <span className="kc-demo-resource-icon kc-demo-resource-icon--purple" aria-hidden="true">
              <ObjectIcon type={kindIconType(detail.kind || '')} className="h-5 w-5" />
            </span>
            <div>
              <h2>
                {nameLabel}
                <span className="kc-demo-status">{isDraftDocument ? 'Draft' : isExternal ? syncBadge.label : 'Active'}</span>
              </h2>
              <div className="kc-demo-resource-sub">
                <span>ID: {detail.id}</span>
                <span>Owner: {detail.team || 'Root'}</span>
              </div>
            </div>
          </div>
          <div className="kc-demo-detail-actions" role="toolbar" aria-label="Document actions">
            <button type="button" className="kc-doc-action-btn kc-doc-action-btn--back" onClick={onBackToList}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              Back
            </button>
            {!isEditing ? (
              <div className="kc-doc-actions-menu">
                <button
                  type="button"
                  className="kc-doc-action-btn kc-doc-action-btn--primary"
                  aria-haspopup="true"
                  aria-expanded={actionsOpen}
                  onClick={() => setActionsOpen(open => !open)}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
                  Actions
                </button>
                {actionsOpen ? (
                  <div className="kc-doc-actions-popover" aria-label="Document action menu">
                    {!draftID ? (
                      <ResourceAccessCard
                        resourceType="knowledge_context"
                        resourceID={detail.id}
                        label="knowledge context"
                        buttonClassName="kc-doc-menu-item"
                        onAccessChange={onAccessChange}
                      />
                    ) : null}
                    <button type="button" className="kc-doc-menu-item" onClick={onDownload}>
                      <Download className="h-4 w-4" aria-hidden="true" />
                      Export
                    </button>
                    {isExternal && detail.external_page_url ? (
                      <a className="kc-doc-menu-item" href={detail.external_page_url} target="_blank" rel="noreferrer">
                        <ExternalLink className="h-4 w-4" aria-hidden="true" />
                        Open page
                      </a>
                    ) : null}
                    {isExternal ? (
                      <button type="button" className="kc-doc-menu-item" onClick={onSyncNow} disabled={syncing}>
                        <RotateCw className="h-4 w-4" aria-hidden="true" />
                        {syncing ? 'Syncing...' : 'Sync now'}
                      </button>
                    ) : null}
                    {canEditSelected ? (
                      <button type="button" className="kc-doc-menu-item kc-doc-menu-item--primary" onClick={handleStartEditing}>
                        <Edit3 className="h-4 w-4" aria-hidden="true" />
                        Edit
                      </button>
                    ) : null}
                    {canWriteKnowledge ? (
                      <button type="button" className="kc-doc-menu-item" onClick={onClone}>
                        <Plus className="h-4 w-4" aria-hidden="true" />
                        Clone
                      </button>
                    ) : null}
                    {canDeleteKnowledge ? (
                      <button
                        type="button"
                        className="kc-doc-menu-item kc-doc-menu-item--danger"
                        aria-label="Delete"
                        onClick={() => onDelete(detail)}
                        disabled={saving}
                      >
                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                        Delete
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </div>
            ) : (
              <>
                <button type="button" className="kc-doc-action-btn" onClick={onDiscardEditing}>
                  Discard
                </button>
                <button type="button" className="kc-doc-action-btn kc-doc-action-btn--primary" onClick={onSave} disabled={saving}>
                  {saving ? 'Saving...' : 'Save changes'}
                </button>
              </>
            )}
          </div>
        </div>
        <div className="kc-demo-tabs" role="tablist" aria-label="Knowledge context detail sections">
          {detailTabs.map(tab => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={activeTab === tab.id}
              className={activeTab === tab.id ? 'active' : ''}
              onClick={() => setActiveTab(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {detailError ? <div className="kc-demo-alert">{detailError}</div> : null}
      {isGitManagedDocument(detail) ? (
        <div className="kc-demo-alert kc-demo-alert--warning">
          Editing here saves a database override. The next GitOps sync can replace it unless the change is pushed to GitOps.
        </div>
      ) : null}

      {activeTab === 'overview' ? (
        <div className="kc-overview-stack">
          {isEditing ? (
            <div className="kc-demo-card kc-demo-panel kc-overview-edit-card">
              <div className="kc-demo-panel-title">
                <div>
                  <h3>Edit Overview</h3>
                  <p>Update the document summary and source settings.</p>
                </div>
              </div>
              <div className="kc-demo-edit-stack">
                <label>
                  <span>Description</span>
                  <textarea
                    value={detail.description || ''}
                    disabled={!selectedCanEdit}
                    onChange={event => onDescriptionChange(event.target.value)}
                    placeholder="Optional description"
                  />
                </label>
                {isExternal ? (
                  <ExternalSettingsEditor
                    detail={detail}
                    connections={connections}
                    selectedConnectionID={selectedConnectionID}
                    onDetailPatch={onDetailPatch}
                  />
                ) : null}
              </div>
            </div>
          ) : (
            <>
              <div className="kc-overview-detail-grid">
                <OverviewPanel title="Document Details" iconType={kindIconType(detail.kind || '')} tone="purple">
                  <InfoRow label="Document ID" value={detail.id} copyable />
                  <InfoRow label="Kind" value={detail.kind} />
                  <InfoRow label="Owner" value={detail.team || 'Root'} />
                  <InfoRow label="Visibility" value={detail.visibility || 'team'} />
                  <div className="kc-overview-divider" aria-hidden="true" />
                  <InfoRow label="Content source" value={isExternal ? 'External page' : 'Inline content'} />
                  <InfoRow label="Provider" value={source} badge={isGitManagedDocument(detail) ? 'Synced' : undefined} />
                  {isExternal ? (
                    <>
                      <InfoRow label="Connection" value={selectedConnection ? knowledgeConnectionDisplayName(selectedConnection) : detail.connection_ref || detail.connection_id || '-'} />
                      <InfoRow label="Page" value={detail.external_page_title || '-'} />
                      <InfoRow label="Page ID" value={detail.external_page_id || '-'} />
                      <InfoRow label="Page URL" value={detail.external_page_url || '-'} href={detail.external_page_url || undefined} />
                    </>
                  ) : null}
                </OverviewPanel>
                <OverviewPanel title="System Health" iconType={isExternal ? 'monitoring' : 'access'} tone="green">
                  {isExternal ? (
                    <>
                      <InfoRow label="Sync mode" value={(detail.sync_mode || 'manual').replace(/_/g, ' ')} />
                      <InfoRow label="Sync status" value={syncBadge.label} />
                      <InfoRow label="Failure behavior" value={(detail.failure_mode || 'fail').replace(/_/g, ' ')} />
                      <div className="kc-overview-divider" aria-hidden="true" />
                      <InfoRow label="Last synced" value={formatKnowledgeDate(detail.last_synced_at)} />
                      <InfoRow label="Provider modified" value={formatKnowledgeDate(detail.source_modified_at)} />
                      <InfoRow label="Content hash" value={detail.content_hash ? detail.content_hash.slice(0, 12) : '-'} />
                    </>
                  ) : (
                    <>
                      <InfoRow label="Source state" value={isGitManagedDocument(detail) ? 'GitOps synced' : 'Database'} />
                      <InfoRow label="Access posture" value={detail.visibility || 'team'} />
                      <div className="kc-overview-divider" aria-hidden="true" />
                    </>
                  )}
                  <InfoRow label="Updated" value={updatedLabel} />
                  <InfoRow label="Content size" value={`${contentMetrics.words} words / ${contentMetrics.lines} lines`} />
                </OverviewPanel>
              </div>
            </>
          )}
        </div>
      ) : null}

      {activeTab === 'content' ? (
        <div className="kc-demo-card kc-demo-content-preview">
          <div className="kc-demo-usage-head">
            <h3>{isEditing ? 'Edit Content' : 'Content Preview'}</h3>
            {!isEditing ? (
              <button type="button" className="kc-demo-outline-btn" onClick={onCopy}>
                <Copy className="h-4 w-4" aria-hidden="true" />
                Copy
              </button>
            ) : null}
          </div>
          {isEditing && !isExternal ? (
            <div className="kc-demo-edit-stack">
              <label>
                <span>Content</span>
                <textarea
                  value={editorValue}
                  disabled={!selectedCanEdit}
                  onChange={event => onContentChange(event.target.value)}
                  spellCheck={false}
                />
              </label>
            </div>
          ) : isEditing && isExternal ? (
            <>
              <div className="kc-demo-alert">External page content is read-only here. Update the source page, then use Sync now when provider fetch is available.</div>
              <pre><code>{previewContent || 'No cached content yet'}</code></pre>
            </>
          ) : (
            <pre><code>{previewContent || 'No content'}</code></pre>
          )}
        </div>
      ) : null}

      {activeTab === 'usage' ? (
        <UsagePanel
          detail={detail}
          nameLabel={nameLabel}
          source={source}
          updatedLabel={updatedLabel}
          onOpenPipeline={onOpenPipeline}
        />
      ) : null}

      {activeTab === 'gitops' ? (
        <div className="kc-demo-card kc-demo-panel">
          <div className="kc-demo-panel-title">
            <div>
              <h3>GitOps</h3>
              <p>{isGitManagedDocument(detail) ? 'Repository-managed document with database override protection.' : 'Database-managed document.'}</p>
            </div>
            <span className={`kc-demo-resource-icon ${isGitManagedDocument(detail) ? 'kc-demo-resource-icon--green' : 'kc-demo-resource-icon--purple'}`} aria-hidden="true">
              {isGitManagedDocument(detail) ? <GitBranch className="h-5 w-5" /> : <ShieldCheck className="h-5 w-5" />}
            </span>
          </div>
          <dl className="kc-demo-kv">
            <InfoRow label="State" value={isGitManagedDocument(detail) ? 'Synced from GitOps' : 'Database'} />
            <InfoRow label="Source path" value={detail.config_source_path || '-'} />
            <InfoRow label="Commit" value={detail.config_source_commit_sha || '-'} />
            <InfoRow label="Updated" value={updatedLabel} />
          </dl>
        </div>
      ) : null}
    </section>
  );
}

function ExternalSettingsEditor({
  detail,
  connections,
  selectedConnectionID,
  onDetailPatch,
}: {
  detail: KnowledgeContextDetail;
  connections: KnowledgeConnectionListItem[];
  selectedConnectionID: string;
  onDetailPatch: (patch: Partial<KnowledgeContextDetail>) => void;
}) {
  const teamConnections = connections.filter(connection => normalizeTeamPath(connection.team) === normalizeTeamPath(detail.team));
  return (
    <div className="kc-external-settings">
      <label>
        <span>Connection</span>
        <select
          value={selectedConnectionID}
          onChange={event => {
            const connection = connections.find(item => knowledgeConnectionMatchesIdentifier(item, event.target.value));
            onDetailPatch({
              connection_id: connection?.uuid || event.target.value,
              connection_ref: connection?.id || event.target.value,
              external_provider: connection?.provider || detail.external_provider,
              source: connection?.provider || detail.source,
            });
          }}
        >
          <option value="">Choose a connection</option>
          {teamConnections.map(connection => (
            <option key={connection.id} value={connection.id}>
              {knowledgeConnectionDisplayName(connection)} ({connection.provider})
            </option>
          ))}
        </select>
      </label>
      <label>
        <span>Page URL</span>
        <input
          value={detail.external_page_url || ''}
          onChange={event => onDetailPatch({ external_page_url: event.target.value })}
          placeholder="https://workspace.example/wiki/page"
        />
      </label>
      <label>
        <span>Page ID</span>
        <input
          value={detail.external_page_id || ''}
          onChange={event => onDetailPatch({ external_page_id: event.target.value })}
          placeholder="Provider page ID"
        />
      </label>
      <div className="kc-external-settings-grid">
        <label>
          <span>Sync mode</span>
          <select
            value={detail.sync_mode || 'manual'}
            onChange={event => onDetailPatch({ sync_mode: event.target.value as KnowledgeSyncMode })}
          >
            {knowledgeSyncModeOptions.map(option => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </label>
        <label>
          <span>Failure behavior</span>
          <select
            value={detail.failure_mode || 'fail'}
            onChange={event => onDetailPatch({ failure_mode: event.target.value as KnowledgeFailureMode })}
          >
            {knowledgeFailureModeOptions.map(option => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </label>
      </div>
    </div>
  );
}

function UsagePanel({
  detail,
  nameLabel,
  source,
  updatedLabel,
  onOpenPipeline,
}: {
  detail: KnowledgeContextDetail;
  nameLabel: string;
  source: string;
  updatedLabel: string;
  onOpenPipeline: (pipelineID: string) => void;
}) {
  if (!detail.used_by?.length) {
    return (
      <div className="kc-demo-card kc-demo-usage">
        <div className="kc-demo-usage-head">
          <h3>Recent Usage</h3>
        </div>
        <div className="kc-demo-table-empty">
          <Lock className="h-5 w-5" aria-hidden="true" />
          <strong>No pipeline usage yet</strong>
          <span>Pipelines can reference this context after `knowledge_context.use` is granted.</span>
        </div>
      </div>
    );
  }

  return (
    <div className="kc-demo-card kc-demo-usage">
      <div className="kc-demo-usage-head">
        <h3>Recent Usage</h3>
      </div>
      <table>
        <thead>
          <tr>
            <th>Pipeline</th>
            <th>Status</th>
            <th>Knowledge</th>
            <th>Source</th>
            <th>Updated</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {detail.used_by.map(pipelineID => (
            <tr key={pipelineID}>
              <td>
                <button type="button" className="kc-demo-table-link" onClick={() => onOpenPipeline(pipelineID)}>
                  {pipelineID}
                </button>
              </td>
              <td><span className="kc-demo-pill ok">Using</span></td>
              <td>{nameLabel}</td>
              <td>{source}</td>
              <td>{updatedLabel}</td>
              <td className="kc-demo-kebab">...</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function InfoRow({
  label,
  value,
  badge,
  href,
  copyable = false,
}: {
  label: string;
  value: string;
  badge?: string;
  href?: string;
  copyable?: boolean;
}) {
  const displayValue = value || '-';
  const copyLabel = label === 'Document ID' ? 'Copy document ID' : `Copy ${label.toLowerCase()}`;
  return (
    <div className="kc-info-row">
      <dt>{label}</dt>
      <dd>
        {href && displayValue !== '-' ? (
          <a className="kc-info-value kc-info-link" href={href} target="_blank" rel="noreferrer" title={displayValue}>
            {displayValue}
          </a>
        ) : (
          <span className="kc-info-value" title={displayValue}>{displayValue}</span>
        )}
        {copyable && displayValue !== '-' ? (
          <button
            type="button"
            className="kc-info-copy-btn"
            aria-label={copyLabel}
            onClick={() => void navigator.clipboard?.writeText(displayValue)}
          >
            <Copy className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        ) : null}
        {badge ? <span className="kc-demo-sync">{badge}</span> : null}
      </dd>
    </div>
  );
}

function OverviewPanel({
  title,
  iconType,
  tone,
  children,
}: {
  title: string;
  iconType: ObjectIconType;
  tone: 'green' | 'purple';
  children: ReactNode;
}) {
  return (
    <section className="kc-overview-panel">
      <div className="kc-overview-panel-head">
        <span className={`kc-demo-act-icon kc-demo-act-icon--${tone}`} aria-hidden="true">
          <ObjectIcon type={iconType} />
        </span>
        <h4>{title}</h4>
      </div>
      <dl className="kc-demo-kv kc-demo-kv--overview">{children}</dl>
    </section>
  );
}
