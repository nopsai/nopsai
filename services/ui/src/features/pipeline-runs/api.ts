import { apiClient } from '../../lib/api.js';

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function requestPipelineRunsJson<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await apiClient.fetch(path, { cache: 'no-store', ...options });
  if (!response.ok) {
    throw new Error(await responseError(response, `Request failed: ${response.status}`));
  }
  const text = await response.text();
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    return text as T;
  }
}

export async function fetchRunLogs<T>(
  runID: string,
  sinceLine: number,
  options: { includeChildren?: boolean } = {}
): Promise<T[]> {
  const query = new URLSearchParams({ since_line: String(sinceLine) });
  if (options.includeChildren) query.set('include_children', 'true');
  const response = await apiClient.fetch(`/v1/runs/${encodeURIComponent(runID)}/logs?${query.toString()}`, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to load run logs (${response.status})`));
  }
  const payload = (await response.json()) as T[] | null;
  return Array.isArray(payload) ? payload : [];
}
