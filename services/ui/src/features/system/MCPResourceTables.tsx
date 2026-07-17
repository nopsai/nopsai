import { ObjectIcon } from '../../components/ObjectIcon';
import { AIResourceEmptyState, AIResourceTeamBadge } from './AIResourcePanel';
import { countMCPProfileTools, formatMCPScopes, type MCPProfileRecord, type MCPServerRecord } from './mcp/model';

export function MCPServerTable({
  servers,
  selectedServerName,
  loading,
  emptyMessage,
  onSelectServer,
}: {
  servers: MCPServerRecord[];
  selectedServerName: string | null;
  loading: boolean;
  emptyMessage: string;
  onSelectServer: (name: string) => void;
}) {
  if (!loading && servers.length === 0) {
    return <AIResourceEmptyState>{emptyMessage}</AIResourceEmptyState>;
  }

  return (
    <div className="ai-resource-table-shell">
      <table className="ai-resource-registry-table" aria-label="MCP servers">
        <colgroup>
          <col style={{ width: '30%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '16%' }} />
          <col style={{ width: '16%' }} />
          <col style={{ width: '8%' }} />
          <col style={{ width: '16%' }} />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">Server</th>
            <th scope="col">Team</th>
            <th scope="col">Provider</th>
            <th scope="col">Scopes</th>
            <th scope="col">Tools</th>
            <th scope="col">Status</th>
          </tr>
        </thead>
        <tbody>
          {servers.map(server => {
            const selected = selectedServerName === server.name;
            return (
              <tr key={server.name} className={selected ? 'selected' : ''} onClick={() => onSelectServer(server.name)}>
                <td>
                  <button
                    type="button"
                    className="ai-resource-table-resource"
                    aria-label={`Select MCP server ${server.display_name || server.name}`}
                    onClick={event => {
                      event.stopPropagation();
                      onSelectServer(server.name);
                    }}
                  >
                    <span className="ai-resource-table-resource-icon" aria-hidden="true">
                      <ObjectIcon type="mcp-server" />
                    </span>
                    <span className="ai-resource-table-resource-name">
                      <strong>{server.display_name || server.name}</strong>
                    </span>
                  </button>
                </td>
                <td><AIResourceTeamBadge resourceID={server.name} /></td>
                <td>{server.provider || '-'}</td>
                <td>{formatMCPScopes(server.allowed_scopes)}</td>
                <td><span className="ai-resource-table-mono ai-resource-table-number">{server.tools.length}</span></td>
                <td>
                  <span className={`ai-resource-health ${server.enabled ? 'ai-resource-health--ok' : 'ai-resource-health--muted'}`}>
                    <span aria-hidden="true" />
                    {server.last_test_status || (server.enabled ? 'Enabled' : 'Disabled')}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function MCPProfileTable({
  profiles,
  selectedProfileName,
  loading,
  emptyMessage,
  onSelectProfile,
}: {
  profiles: MCPProfileRecord[];
  selectedProfileName: string | null;
  loading: boolean;
  emptyMessage: string;
  onSelectProfile: (name: string) => void;
}) {
  if (!loading && profiles.length === 0) {
    return <AIResourceEmptyState>{emptyMessage}</AIResourceEmptyState>;
  }

  return (
    <div className="ai-resource-table-shell">
      <table className="ai-resource-registry-table" aria-label="MCP profiles">
        <colgroup>
          <col style={{ width: '30%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '10%' }} />
          <col style={{ width: '10%' }} />
          <col style={{ width: '20%' }} />
          <col style={{ width: '16%' }} />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">Profile</th>
            <th scope="col">Team</th>
            <th scope="col">Servers</th>
            <th scope="col">Tools</th>
            <th scope="col">Scopes</th>
            <th scope="col">Status</th>
          </tr>
        </thead>
        <tbody>
          {profiles.map(profile => {
            const toolCount = countMCPProfileTools(profile);
            const selected = selectedProfileName === profile.name;
            return (
              <tr key={profile.name} className={selected ? 'selected' : ''} onClick={() => onSelectProfile(profile.name)}>
                <td>
                  <button
                    type="button"
                    className="ai-resource-table-resource"
                    aria-label={`Select MCP profile table row ${profile.name.split('/').filter(Boolean).pop() || profile.name}`}
                    onClick={event => {
                      event.stopPropagation();
                      onSelectProfile(profile.name);
                    }}
                  >
                    <span className="ai-resource-table-resource-icon" aria-hidden="true">
                      <ObjectIcon type="mcp-profile" />
                    </span>
                    <span className="ai-resource-table-resource-name">
                      <strong>{profile.name}</strong>
                    </span>
                  </button>
                </td>
                <td><AIResourceTeamBadge resourceID={profile.name} /></td>
                <td><span className="ai-resource-table-mono ai-resource-table-number">{profile.servers.length}</span></td>
                <td><span className="ai-resource-table-mono ai-resource-table-number">{toolCount}</span></td>
                <td>{formatMCPScopes(profile.allowed_scopes)}</td>
                <td>
                  <span className={`ai-resource-health ${profile.enabled ? 'ai-resource-health--ok' : 'ai-resource-health--muted'}`}>
                    <span aria-hidden="true" />
                    {profile.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
