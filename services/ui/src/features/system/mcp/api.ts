import { apiClient } from '../../../lib/api';
import {
  normalizeMCPProfileTestMessage,
  normalizeMCPProfilesPayload,
  normalizeMCPServersPayload,
  type MCPProfilePayload,
  type MCPProfileRecord,
  type MCPServerPayload,
  type MCPServerRecord,
} from './model';

export type MCPRegistry = {
  servers: MCPServerRecord[];
  profiles: MCPProfileRecord[];
};

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function fetchMCPRegistry(): Promise<MCPRegistry> {
  const [serversResp, profilesResp] = await Promise.all([
    apiClient.fetch('/v1/system/mcp/servers', { cache: 'no-store' }),
    apiClient.fetch('/v1/system/mcp/profiles', { cache: 'no-store' }),
  ]);
  if (!serversResp.ok) throw new Error(await readError(serversResp, `Failed to load MCP servers (${serversResp.status})`));
  if (!profilesResp.ok) throw new Error(await readError(profilesResp, `Failed to load MCP profiles (${profilesResp.status})`));

  return {
    servers: normalizeMCPServersPayload(await serversResp.json()),
    profiles: normalizeMCPProfilesPayload(await profilesResp.json()),
  };
}

export async function saveMCPServer(payload: MCPServerPayload, editingServer: string | null): Promise<MCPServerRecord[]> {
  const path = editingServer ? `/v1/system/mcp/servers/${encodeURIComponent(payload.name)}` : '/v1/system/mcp/servers';
  const response = await apiClient.fetch(path, {
    method: editingServer ? 'PUT' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw new Error(await readError(response, `Failed to save MCP server (${response.status})`));
  return normalizeMCPServersPayload(await response.json());
}

export async function deleteMCPServer(name: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/system/mcp/servers/${encodeURIComponent(name)}`, { method: 'DELETE' });
  if (!response.ok && response.status !== 204) {
    throw new Error(await readError(response, `Failed to delete MCP server (${response.status})`));
  }
}

export async function discoverMCPServer(name: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/system/mcp/servers/${encodeURIComponent(name)}/discover-tools`, { method: 'POST' });
  if (!response.ok) throw new Error(await readError(response, `MCP discovery failed (${response.status})`));
}

export async function saveMCPProfile(payload: MCPProfilePayload, editingProfile: string | null): Promise<MCPProfileRecord[]> {
  const path = editingProfile ? `/v1/system/mcp/profiles/${encodeURIComponent(payload.name)}` : '/v1/system/mcp/profiles';
  const response = await apiClient.fetch(path, {
    method: editingProfile ? 'PUT' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw new Error(await readError(response, `Failed to save MCP profile (${response.status})`));
  return normalizeMCPProfilesPayload(await response.json());
}

export async function deleteMCPProfile(name: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/system/mcp/profiles/${encodeURIComponent(name)}`, { method: 'DELETE' });
  if (!response.ok && response.status !== 204) {
    throw new Error(await readError(response, `Failed to delete MCP profile (${response.status})`));
  }
}

export async function testMCPProfile(name: string): Promise<string> {
  const response = await apiClient.fetch(`/v1/system/mcp/profiles/${encodeURIComponent(name)}/test`, { method: 'POST' });
  if (!response.ok) throw new Error(await readError(response, `MCP profile test failed (${response.status})`));
  return normalizeMCPProfileTestMessage(await response.json());
}
