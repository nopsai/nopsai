import { ExternalLink, KeyRound, Link2, Plug, Power, PowerOff, RefreshCw, Trash2 } from 'lucide-react';

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
          ...team.connections.flatMap(connection => [connection.name, connection.display_name, connection.provider, connection.status]),
        ].some(value => (value || '').toLowerCase().includes(term))
      )
    : connectionTeams;
  const activeRows = visibleTeams.flatMap(team => team.connections.map(connection => ({ connection, teamPath: team.teamPath })));
  const activeConnections = activeRows.map(row => row.connection);
  const connectedCount = activeConnections.filter(connection => connection.status === 'connected' && !connection.disabled).length;
  const authRequiredCount = activeConnections.filter(connection => connection.status === 'authentication_required').length;
  const disabledCount = activeConnections.filter(connection => connection.disabled).length;

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

      <div className="kc-demo-card kc-demo-usage">
        <div className="kc-demo-usage-head">
          <div>
            <h3>Configured Connections</h3>
            <p>Team-owned provider links available for external Knowledge Context documents.</p>
          </div>
        </div>
        <div className="kc-demo-table-wrap">
          <table className="kc-demo-resource-table">
            <thead>
              <tr>
                <th>Connection</th>
                <th>Team</th>
                <th>Provider</th>
                <th>Status</th>
                <th>Knowledge Contexts</th>
                <th>Last Checked</th>
                <th aria-label="Actions" />
              </tr>
            </thead>
            <tbody>
              {activeRows.map(({ connection, teamPath }) => {
                const contextCount = connection.external_document_count ?? connection.document_count ?? connection.used_by?.length ?? 0;
                return (
                  <tr key={connection.id}>
                    <td>
                      <span className="kc-demo-resource-cell kc-demo-resource-cell--static">
                        <span className="kc-demo-resource-icon" aria-hidden="true">
                          <Link2 className="h-4 w-4" />
                        </span>
                        <span className="kc-demo-resource-name">
                          <strong>{knowledgeConnectionDisplayName(connection)}</strong>
                          <span>{connection.id}</span>
                          <em>{connection.credential_visibility === 'configured' ? 'Credential configured' : 'Credential not configured'}</em>
                        </span>
                      </span>
                    </td>
                    <td><span className="kc-demo-mono">{teamPath}</span></td>
                    <td><span className="kc-demo-badge blue"><span className="dot" />{knowledgeConnectionProviderLabel(connection.provider)}</span></td>
                    <td>
                      <span className={`kc-demo-badge ${connection.disabled || connection.status !== 'connected' ? 'amber' : 'green'}`}>
                        <span className="dot" />
                        {knowledgeConnectionStatusLabel(connection.status, connection.disabled)}
                      </span>
                    </td>
                    <td><span className="kc-demo-mono">{contextCount}</span></td>
                    <td><span className="kc-demo-mono">{formatKnowledgeDate(connection.last_checked_at)}</span></td>
                    <td>
                      <div className="kc-demo-row-actions kc-demo-row-actions--wide">
                        {connection.base_url ? (
                          <a
                            className="kc-demo-kebab-btn kc-demo-connection-action"
                            aria-label={`Open ${knowledgeConnectionDisplayName(connection)} base URL`}
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
                          aria-label={`Test ${knowledgeConnectionDisplayName(connection)}`}
                          title="Test connection"
                          onClick={() => onTestConnection(connection)}
                          disabled={!canWriteKnowledge}
                        >
                          <RefreshCw className="h-4 w-4" aria-hidden="true" />
                        </button>
                        <button
                          type="button"
                          className="kc-demo-kebab-btn kc-demo-connection-action"
                          aria-label={`Reconnect ${knowledgeConnectionDisplayName(connection)}`}
                          title="Reconnect"
                          onClick={() => onEditConnection(connection)}
                          disabled={!canWriteKnowledge}
                        >
                          <KeyRound className="h-4 w-4" aria-hidden="true" />
                        </button>
                        <button
                          type="button"
                          className="kc-demo-kebab-btn kc-demo-connection-action"
                          aria-label={`${connection.disabled ? 'Enable' : 'Disable'} ${knowledgeConnectionDisplayName(connection)}`}
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
                            aria-label={`Delete ${knowledgeConnectionDisplayName(connection)}`}
                            title="Delete"
                            onClick={() => onDeleteConnection(connection)}
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </button>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
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
