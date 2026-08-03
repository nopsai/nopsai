import { Copy, ExternalLink, KeyRound, Link2, PencilLine, Power, PowerOff, RefreshCw, Trash2, X } from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
import { copyTextToClipboard } from '../../lib/clipboard';
import {
  knowledgeConnectionDisplayName,
  knowledgeConnectionProviderLabel,
  knowledgeConnectionStatusLabel,
  type KnowledgeConnectionListItem,
  type KnowledgeConnectionTeamSummary,
} from './model';
import { formatKnowledgeDate } from './presentation';

type KnowledgeContextConnectionsViewProps = {
  listLoading: boolean;
  listError: string | null;
  search: string;
  teams: KnowledgeConnectionTeamSummary[];
  selectedConnectionID: string;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  onSelectConnection: (connectionID: string, teamPath: string) => void;
  onCloseConnectionDetails: () => void;
  onSelectDocument: (id: string) => void;
  onTestConnection: (connection: KnowledgeConnectionListItem) => void;
  onEditConnection: (connection: KnowledgeConnectionListItem) => void;
  onToggleConnection: (connection: KnowledgeConnectionListItem) => void;
  onDeleteConnection: (connection: KnowledgeConnectionListItem) => void;
};

export function KnowledgeContextConnectionsView({
  listLoading,
  listError,
  search,
  teams,
  selectedConnectionID,
  canWriteKnowledge,
  canDeleteKnowledge,
  onSelectConnection,
  onCloseConnectionDetails,
  onSelectDocument,
  onTestConnection,
  onEditConnection,
  onToggleConnection,
  onDeleteConnection,
}: KnowledgeContextConnectionsViewProps) {
  const term = search.trim().toLowerCase();
  const connectionTeams = teams.filter(team => team.connections.length > 0);
  const visibleTeams = term
    ? connectionTeams.filter(team =>
        [
          team.teamPath,
          ...team.providers,
          ...team.connections.flatMap(connection => [connection.name, connection.display_name, connection.provider, connection.status, ...(connection.used_by || [])]),
        ].some(value => (value || '').toLowerCase().includes(term))
      )
    : connectionTeams;
  const activeRows = visibleTeams.flatMap(team => team.connections.map(connection => ({ connection, teamPath: team.teamPath })));
  const activeConnections = activeRows.map(row => row.connection);
  const selectedRow = activeRows.find(row => row.connection.id === selectedConnectionID) || null;

  if (listLoading) {
    return <div className="kc-demo-detail-empty">Loading knowledge connections...</div>;
  }

  if (listError) {
    return <div className="kc-demo-detail-empty kc-demo-detail-empty--error">Failed to load knowledge connections: {listError}</div>;
  }

  if (!activeConnections.length) {
    return (
      <section id="knowledge-context-connections-view" className="kc-demo-detail" aria-label="Knowledge Context connections">
        <div className="kc-demo-detail-empty">
          <KeyRound className="h-6 w-6" aria-hidden="true" />
          <h2>{term ? 'No matching connections' : 'No knowledge connections yet'}</h2>
          <span>
            {term
              ? 'Adjust the search filter to find a configured provider connection.'
              : 'Use New connection in the toolbar to add a team-owned Notion, Confluence, or wiki connection.'}
          </span>
        </div>
      </section>
    );
  }

  return (
    <section id="knowledge-context-connections-view" className="kc-demo-detail" aria-label="Knowledge Context connections">
      <div className="kc-connection-browser-grid">
        <div className="kc-demo-card kc-demo-usage kc-connection-table-card">
          <div className="kc-demo-table-wrap">
            <table className="kc-demo-resource-table kc-connection-table">
              <thead>
                <tr>
                  <th>Connection</th>
                  <th>Team</th>
                  <th>Provider</th>
                  <th>Status</th>
                  <th>Knowledge Contexts</th>
                  <th>Last Checked</th>
                </tr>
              </thead>
              <tbody>
                {activeRows.map(({ connection, teamPath }) => {
                  const contextCount = connectionKnowledgeContextCount(connection);
                  const selected = selectedRow?.connection.id === connection.id;
                  return (
                    <tr key={connection.id} className={selected ? 'kc-connection-row-active' : undefined}>
                      <td>
                        <button
                          type="button"
                          className="kc-demo-resource-cell kc-demo-resource-cell--button"
                          aria-label={`View ${knowledgeConnectionDisplayName(connection)} details`}
                          onClick={() => onSelectConnection(connection.id, teamPath)}
                        >
                          <span className="kc-demo-resource-icon" aria-hidden="true">
                            <Link2 className="h-4 w-4" />
                          </span>
                          <span className="kc-demo-resource-name">
                            <strong>{knowledgeConnectionDisplayName(connection)}</strong>
                            <span>{connection.id}</span>
                            <em>{connection.credential_visibility === 'configured' ? 'Credential configured' : 'Credential not configured'}</em>
                          </span>
                        </button>
                      </td>
                      <td><span className="kc-demo-mono">{teamPath}</span></td>
                      <td><span className="kc-demo-badge blue"><span className="dot" />{knowledgeConnectionProviderLabel(connection.provider)}</span></td>
                      <td>
                        <span className={`kc-demo-badge ${connection.disabled || connection.status !== 'connected' ? 'amber' : 'green'}`}>
                          <span className="dot" />
                          {knowledgeConnectionStatusLabel(connection.status, connection.disabled)}
                        </span>
                      </td>
                      <td><span className="kc-demo-badge neutral">{contextCount} linked</span></td>
                      <td><span className="kc-demo-mono">{formatKnowledgeDate(connection.last_checked_at)}</span></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
        {selectedRow ? (
          <ConnectionDetailPanel
            connection={selectedRow.connection}
            teamPath={selectedRow.teamPath}
            canWriteKnowledge={canWriteKnowledge}
            canDeleteKnowledge={canDeleteKnowledge}
            onClose={onCloseConnectionDetails}
            onSelectDocument={onSelectDocument}
            onTestConnection={onTestConnection}
            onEditConnection={onEditConnection}
            onToggleConnection={onToggleConnection}
            onDeleteConnection={onDeleteConnection}
          />
        ) : null}
      </div>
    </section>
  );
}

function ConnectionDetailPanel({
  connection,
  teamPath,
  canWriteKnowledge,
  canDeleteKnowledge,
  onClose,
  onSelectDocument,
  onTestConnection,
  onEditConnection,
  onToggleConnection,
  onDeleteConnection,
}: {
  connection: KnowledgeConnectionListItem;
  teamPath: string;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  onClose: () => void;
  onSelectDocument: (id: string) => void;
  onTestConnection: (connection: KnowledgeConnectionListItem) => void;
  onEditConnection: (connection: KnowledgeConnectionListItem) => void;
  onToggleConnection: (connection: KnowledgeConnectionListItem) => void;
  onDeleteConnection: (connection: KnowledgeConnectionListItem) => void;
}) {
  const displayName = knowledgeConnectionDisplayName(connection);
  const linkedKnowledgeContexts = connectionKnowledgeContextIDs(connection);
  const statusLabel = knowledgeConnectionStatusLabel(connection.status, connection.disabled);
  const credentialLabel = connection.credential_visibility === 'configured' ? 'Configured' : 'Not configured';
  const linkedCount = linkedKnowledgeContexts.length || connection.external_document_count || connection.document_count || 0;
  const reference = `knowledge_connection:${connection.id}`;
  const detailFields = [
    { label: 'Team', value: teamPath },
    { label: 'Provider', value: knowledgeConnectionProviderLabel(connection.provider) },
    { label: 'Status', value: statusLabel },
    { label: 'Credential', value: credentialLabel },
    { label: 'Last checked', value: formatKnowledgeDate(connection.last_checked_at) },
    { label: 'Linked documents', value: String(linkedCount) },
    { label: 'Base URL', value: connection.base_url || '-' },
    { label: 'Connection ID', value: connection.id },
  ];
  const copyReference = () => {
    void copyTextToClipboard(reference).catch(() => undefined);
  };
  return (
    <div
      className="kc-connection-detail__overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="kc-connection-detail-heading"
      onMouseDown={event => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside className="kc-connection-detail__drawer">
        <header className="kc-connection-detail__header">
          <div className="kc-connection-detail__headline">
            <div className="min-w-0">
              <div className="kc-connection-detail__crumbs">
                <span className="kc-demo-badge blue"><span className="dot" />{knowledgeConnectionProviderLabel(connection.provider)}</span>
                <span className={`kc-demo-badge ${connection.disabled || connection.status !== 'connected' ? 'amber' : 'green'}`}><span className="dot" />{statusLabel}</span>
                <span className="kc-demo-badge neutral">{teamPath}</span>
                <span className="kc-demo-badge neutral">{credentialLabel}</span>
              </div>
              <h3 id="kc-connection-detail-heading" className="kc-connection-detail__title">{displayName}</h3>
              <p className="kc-connection-detail__subtitle">{connection.id}</p>
            </div>
            <button type="button" className="kc-connection-detail__close" aria-label="Close connection details" onClick={onClose}>
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>

        <div className="kc-connection-detail__body">
          <section>
            <p className="kc-connection-detail__label">Reference</p>
            <div className="kc-connection-detail__copybox">
              <code title={reference}>{reference}</code>
              <button type="button" className="kc-connection-detail__copy" aria-label="Copy connection reference" onClick={copyReference}>
                <Copy className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </section>

          <dl className="kc-connection-detail__meta-grid">
            {detailFields.map(field => (
              <div key={field.label} className="kc-connection-detail__meta">
                <dt>{field.label}</dt>
                <dd title={field.value}>
                  {field.label === 'Base URL' && connection.base_url ? (
                    <a href={connection.base_url} target="_blank" rel="noreferrer">{connection.base_url}</a>
                  ) : field.value}
                </dd>
              </div>
            ))}
          </dl>

          {connection.last_error ? <div className="kc-demo-alert kc-demo-alert--warning">{connection.last_error}</div> : null}

          <section>
            <div className="kc-connection-detail__section-title">
              <h4>Linked Knowledge Contexts</h4>
              <span className="kc-demo-badge neutral">{linkedCount} linked</span>
            </div>
            {linkedKnowledgeContexts.length ? (
              <div>
                {linkedKnowledgeContexts.map(id => (
                  <button key={id} type="button" className="kc-connection-detail__linked-item" aria-label={`Open ${id}`} onClick={() => onSelectDocument(id)}>
                    <ObjectIcon type="knowledge-context" />
                    <span>
                      <strong>{knowledgeContextShortName(id)}</strong>
                      <small>{id}</small>
                    </span>
                  </button>
                ))}
              </div>
            ) : (
              <p className="kc-connection-detail__empty">Create an external page document with this connection to see it here.</p>
            )}
          </section>
        </div>

        <footer className="kc-connection-detail__footer">
          {connection.base_url ? (
            <a className="kc-doc-menu-item" aria-label={`Open ${displayName} base URL`} href={connection.base_url} target="_blank" rel="noreferrer">
              <ExternalLink className="h-4 w-4" aria-hidden="true" />
              Open provider
            </a>
          ) : null}
          <button type="button" className="kc-doc-menu-item" aria-label={`Test ${displayName}`} onClick={() => onTestConnection(connection)} disabled={!canWriteKnowledge}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            Test
          </button>
          <button type="button" className="kc-doc-menu-item" aria-label={`Edit ${displayName}`} onClick={() => onEditConnection(connection)} disabled={!canWriteKnowledge}>
            <PencilLine className="h-4 w-4" aria-hidden="true" />
            Edit
          </button>
          <button
            type="button"
            className="kc-doc-menu-item"
            aria-label={`${connection.disabled ? 'Enable' : 'Disable'} ${displayName}`}
            onClick={() => onToggleConnection(connection)}
            disabled={!canWriteKnowledge}
          >
            {connection.disabled ? <Power className="h-4 w-4" aria-hidden="true" /> : <PowerOff className="h-4 w-4" aria-hidden="true" />}
            {connection.disabled ? 'Enable' : 'Disable'}
          </button>
          {canDeleteKnowledge ? (
            <button type="button" className="kc-doc-menu-item kc-doc-menu-item--danger" aria-label={`Delete ${displayName}`} onClick={() => onDeleteConnection(connection)}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
              Delete
            </button>
          ) : null}
        </footer>
      </aside>
    </div>
  );
}

function connectionKnowledgeContextIDs(connection: KnowledgeConnectionListItem) {
  return connection.used_by || [];
}

function connectionKnowledgeContextCount(connection: KnowledgeConnectionListItem) {
  return connectionKnowledgeContextIDs(connection).length || connection.external_document_count || connection.document_count || 0;
}

function knowledgeContextShortName(id: string) {
  return id.split('/').filter(Boolean).pop() || id;
}
