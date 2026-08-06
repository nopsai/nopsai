import { apiClient } from '../../lib/api';

export type BackendValidationIssue = {
  message: string;
  path?: string;
  line?: number;
  code?: string;
  file?: string;
};

export type BackendValidationResponse = {
  valid: boolean;
  errors: BackendValidationIssue[];
  warnings: BackendValidationIssue[];
};

export type BackendYamlValidationResource = 'pipeline' | 'step' | 'trigger';

export type BackendYamlValidationRequest = {
  yaml: string;
  content?: string;
  resource_id?: string;
  path?: string;
  name?: string;
  repository?: string;
  repo_owner?: string;
  repo_name?: string;
  team_path?: string;
};

export type ConfigRepositoryValidationFile = {
  path: string;
  content: string;
  delete?: boolean;
};

export type ConfigRepositoryValidationRequest = {
  base_path?: string;
  files: ConfigRepositoryValidationFile[];
};

const yamlValidationEndpoints: Record<BackendYamlValidationResource, string> = {
  pipeline: '/v1/pipelines/validate',
  step: '/v1/steps/validate',
  trigger: '/v1/overrides/validate',
};

async function parseValidationResponse(response: Response, fallback: string): Promise<BackendValidationResponse> {
  const contentType = response.headers.get('content-type') || '';
  const text = await response.text();
  if (contentType.includes('application/json') && text) {
    try {
      const payload = JSON.parse(text);
      if (payload && typeof payload === 'object' && 'valid' in payload) {
        const result = payload as Partial<BackendValidationResponse>;
        return {
          valid: Boolean(result.valid),
          errors: Array.isArray(result.errors) ? result.errors : [],
          warnings: Array.isArray(result.warnings) ? result.warnings : [],
        };
      }
    } catch {
      // Fall through to the shared validation error path below.
    }
  }
  throw new Error(text || fallback);
}

export async function validateBackendYamlResource(
  resource: BackendYamlValidationResource,
  request: BackendYamlValidationRequest,
  signal?: AbortSignal
): Promise<BackendValidationResponse> {
  const endpoint = yamlValidationEndpoints[resource];
  const response = await apiClient.fetch(endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal,
  });
  return parseValidationResponse(response, `Validation failed (${response.status})`);
}

export async function validateGlobalConfigRepositoryDraft(
  request: ConfigRepositoryValidationRequest,
  signal?: AbortSignal
): Promise<BackendValidationResponse> {
  const response = await apiClient.fetch('/v1/system/config-repo/validate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal,
  });
  return parseValidationResponse(response, `Config repository validation failed (${response.status})`);
}

export async function validateTeamConfigRepositoryDraft(
  teamID: string,
  request: ConfigRepositoryValidationRequest,
  signal?: AbortSignal
): Promise<BackendValidationResponse> {
  const encodedTeam = encodeURIComponent(teamID);
  const response = await apiClient.fetch(`/v1/teams/${encodedTeam}/config-repository/validate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal,
  });
  return parseValidationResponse(response, `Config repository validation failed (${response.status})`);
}
