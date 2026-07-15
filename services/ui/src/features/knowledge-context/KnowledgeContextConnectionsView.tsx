import { useState } from 'react';
import { ExternalLink, KeyRound, Link2, PencilLine, Plug, Power, PowerOff, RefreshCw, Trash2 } from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
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
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
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
  canWriteKnowledge,
  canDeleteKnowledge,
  onSelectDocument,
  onTestConnection,
  onEditConnection,
  onToggleConnection,
  onDeleteConnection,
}: KnowledgeContextConnectionsViewProps) {
  const [selectedConnectionID, setSelectedConnectionID] = useState('');
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
  const connectedCount = activeConnections.filter(connection => connection.status === 'connected' && !connection.disabled).length;
  const authRequiredCount = activeConnections.filter(connection => connection.status === 'authentication_required').length;
  const disabledCount = activeConnections.filter(connection => connection.disabled).length;
  const selectedRow = activeRows.find(row => row.connection.id === selectedConnectionID) || activeRows[0] || null;

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
      <div className="kc-demo-card kc-demo-detail-head kc-connection-head">
        <div className="kc-demo-resource-head">
          <div className="kc-demo-resource-title">
            <span className="kc-demo-resource-icon kc-demo-resource-icon--green" aria-hidden="true">
              <Plug className="h-5 w-5" />
            </span>
            <div>
              <div className="kc-demo-resource-heading-line">
                <h2>Connections</h2>
                <span className="kc-demo-status">{activeConnections.length} configured</span>
              </div>
              <div className="kc-demo-resource-sub">
                <span>{visibleTeams.length} team {visibleTeams.length === 1 ? 'scope' : 'scopes'}</span>
                <span>{connectedCount} connected</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="kc-demo-top-grid kc-connection-summary-grid">
        <ConnectionSummaryCard label="Configured" value={String(activeConnections.length)} tone="green" icon="credential" />
        <ConnectionSummaryCard label="Connected" value={String(connectedCount)} tone="blue" icon="team" />
        <ConnectionSummaryCard label="Auth required" value={String(authRequiredCount)} tone="purple" icon="knowledge-context" />
        <ConnectionSummaryCard label="Disabled" value={String(disabledCount)} tone="cyan" icon="team" />
      </div>

      <div className="kc-connection-browser-grid">
        <div className="kc-demo-card kc-demo-usage kc-connection-table-card">
          <div className="kc-demo-usage-head">
            <div>
              <h3>Configured Connections</h3>
              <p>Select a connection to inspect linked Knowledge Contexts and management actions.</p>
            </div>
          </div>
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
                          onClick={() => setSelectedConnectionID(connection.id)}
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
  onSelectDocument: (id: string) => void;
  onTestConnection: (connection: KnowledgeConnectionListItem) => void;
  onEditConnection: (connection: KnowledgeConnectionListItem) => void;
  onToggleConnection: (connection: KnowledgeConnectionListItem) => void;
  onDeleteConnection: (connection: KnowledgeConnectionListItem) => void;
}) {
  const displayName = knowledgeConnectionDisplayName(connection);
  const linkedKnowledgeContexts = connectionKnowledgeContextIDs(connection);
  return (
    <aside className="kc-demo-card kc-connection-detail-panel" aria-label={`${displayName} connection details`}>
      <div className="kc-connection-detail-head">
        <span className="kc-demo-resource-icon kc-demo-resource-icon--green" aria-hidden="true">
          <Plug className="h-5 w-5" />
        </span>
        <div>
          <h3>{displayName}</h3>
          <p>{connection.id}</p>
        </div>
      </div>

      <div className="kc-connection-detail-actions" role="toolbar" aria-label={`${displayName} connection actions`}>
        {connection.base_url ? (
          <a
            className="kc-demo-kebab-btn kc-demo-connection-action"
            aria-label={`Open ${displayName} base URL`}
            title="Open provider"
            href={connection.base_url}
            target="_blank"
            rel="noreferrer"
          >
            <ExternalLink className="h-4 w-4" aria-hidden="true" />
          </a>
        ) : null}
        <button
          type="button"
          className="kc-demo-kebab-btn kc-demo-connection-action"
          aria-label={`Test ${displayName}`}
          title="Test connection"
          onClick={() => onTestConnection(connection)}
          disabled={!canWriteKnowledge}
        >
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          className="kc-demo-kebab-btn kc-demo-connection-action"
          aria-label={`Edit ${displayName}`}
          title="Edit"
          onClick={() => onEditConnection(connection)}
          disabled={!canWriteKnowledge}
        >
          <PencilLine className="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          className="kc-demo-kebab-btn kc-demo-connection-action"
          aria-label={`${connection.disabled ? 'Enable' : 'Disable'} ${displayName}`}
          title={connection.disabled ? 'Enable' : 'Disable'}
          onClick={() => onToggleConnection(connection)}
          disabled={!canWriteKnowledge}
        >
          {connection.disabled ? <Power className="h-4 w-4" aria-hidden="true" /> : <PowerOff className="h-4 w-4" aria-hidden="true" />}
        </button>
        {canDeleteKnowledge ? (
          <button
            type="button"
            className="kc-demo-kebab-btn kc-demo-connection-action danger"
            aria-label={`Delete ${displayName}`}
            title="Delete"
            onClick={() => onDeleteConnection(connection)}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : null}
      </div>

      <dl className="kc-connection-meta">
        <ConnectionMetaRow label="Team" value={teamPath} />
        <ConnectionMetaRow label="Provider" value={knowledgeConnectionProviderLabel(connection.provider)} />
        <ConnectionMetaRow label="Status" value={knowledgeConnectionStatusLabel(connection.status, connection.disabled)} />
        <ConnectionMetaRow label="Credential" value={connection.credential_visibility === 'configured' ? 'Configured' : 'Not configured'} />
        <ConnectionMetaRow label="Last checked" value={formatKnowledgeDate(connection.last_checked_at)} />
        {connection.last_error ? <ConnectionMetaRow label="Last error" value={connection.last_error} /> : null}
      </dl>

      <div className="kc-connection-linked-contexts">
        <div className="kc-demo-usage-head">
          <div>
            <h3>Linked Knowledge Contexts</h3>
            <p>{linkedKnowledgeContexts.length ? `${linkedKnowledgeContexts.length} document${linkedKnowledgeContexts.length === 1 ? '' : 's'} use this connection.` : 'No documents currently use this connection.'}</p>
          </div>
        </div>
        {linkedKnowledgeContexts.length ? (
          <ul>
            {linkedKnowledgeContexts.map(id => (
              <li key={id}>
                <button type="button" className="kc-connection-context-link" aria-label={`Open ${id}`} onClick={() => onSelectDocument(id)}>
                  <ObjectIcon type="knowledge-context" />
                  <span>
                    <strong>{knowledgeContextShortName(id)}</strong>
                    <small>{id}</small>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <div className="kc-connection-empty-contexts">Create an external page document with this connection to see it here.</div>
        )}
      </div>
    </aside>
  );
}

function ConnectionMetaRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value || '-'}</dd>
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

function ConnectionSummaryCard({ label, value, tone, icon }: { label: string; value: string; tone: string; icon: 'credential' | 'team' | 'knowledge-context' }) {
  return (
    <article className="kc-demo-stat">
      <span className={`kc-demo-stat-icon kc-demo-stat-icon--${tone}`} aria-hidden="true">
        <ObjectIcon type={icon} />
      </span>
      <span className="kc-demo-stat-label">{label}</span>
      <strong>{value}</strong>
    </article>
  );
}
