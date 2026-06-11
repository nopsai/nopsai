import { apiClient } from '../../lib/api';

export async function fetchLabAgentProfilesMetadata(): Promise<unknown> {
  const response = await apiClient.fetch('/v1/system/agent-profiles');
  return response.ok ? response.json() : null;
}
