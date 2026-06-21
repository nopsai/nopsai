import * as yaml from 'js-yaml';
import { apiClient } from '../../lib/api.js';
import { encodeId, normalizePipelineSource, splitIdentifier, type PipelineListItem } from './model.js';

export type PipelineRun = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  status?: string;
  trigger_event_id?: string;
  git_repo_owner?: string;
  git_repo_name?: string;
  git_ref?: string;
  duration?: string;
  started_at?: string;
};

export type PipelineTrigger = {
  repoSlug: string;
  source: string;
  trigger: Record<string, unknown>;
};

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export function normalizePipelineListPayload(payload: unknown): PipelineListItem[] {
  const normalized: PipelineListItem[] = Array.isArray(payload)
    ? payload
        .map((item: unknown) => {
          if (typeof item === 'string') return { id: item };
          if (item && typeof item === 'object') {
            const record = item as Record<string, unknown>;
            const id = typeof record.id === 'string' ? record.id : typeof record.identifier === 'string' ? record.identifier : '';
            return id ? { id, source: typeof record.source === 'string' ? record.source : undefined } : null;
          }
          return null;
        })
        .filter(Boolean) as PipelineListItem[]
    : [];
  return normalized.sort((a, b) => a.id.localeCompare(b.id));
}

export async function fetchPipelineList(): Promise<PipelineListItem[]> {
  const response = await apiClient.fetch('/v1/pipelines?include_source=true');
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to load pipelines (${response.status})`));
  }
  return normalizePipelineListPayload(await response.json());
}

export async function fetchPipelineYaml(pipelineID: string): Promise<string> {
  const response = await apiClient.fetch(`/v1/pipelines/${encodeId(pipelineID)}`);
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to fetch pipeline (${response.status})`));
  }
  return response.text();
}

export async function savePipelineYaml(pipelineID: string, rawYaml: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/pipelines/${encodeId(pipelineID)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/x-yaml' },
    body: rawYaml,
  });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to save pipeline (${response.status})`));
  }
}

export async function deletePipeline(pipelineID: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/pipelines/${encodeId(pipelineID)}`, { method: 'DELETE' });
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to delete pipeline (${response.status})`));
  }
}

export async function checkPipelinePermission(action: string, resourceID: string): Promise<boolean> {
  const params = new URLSearchParams({
    action,
    resource_type: 'pipeline',
    resource_id: resourceID,
  });
  const response = await apiClient.fetch(`/v1/access/effective-permissions?${params.toString()}`);
  if (!response.ok) return false;
  const payload = await response.json();
  return Boolean(payload?.allowed);
}

export async function fetchRecentPipelineRuns(pipelineID: string, limit: number): Promise<PipelineRun[]> {
  const response = await apiClient.fetch('/v1/runs');
  if (!response.ok) {
    throw new Error(await readError(response, `Failed to load runs (${response.status})`));
  }
  const payload = await response.json();
  const runsPayload: PipelineRun[] = Array.isArray(payload) ? payload : [];
  const { path, name } = splitIdentifier(pipelineID);
  const normalizedName = name.toLowerCase();
  const normalizedPath = (path || '').toLowerCase();
  return runsPayload
    .filter(run => (run?.pipeline_name || '').toLowerCase() === normalizedName && (run?.pipeline_path || '').toLowerCase() === normalizedPath)
    .sort((a, b) => {
      const aTime = new Date(a.started_at || '').getTime() || 0;
      const bTime = new Date(b.started_at || '').getTime() || 0;
      return bTime - aTime;
    })
    .slice(0, limit);
}

function normalizePipelineIdentifier(value: unknown) {
  if (!value) return '';
  return String(value)
    .trim()
    .replace(/^\.nopsai\//, '')
    .replace(/\.ya?ml$/i, '')
    .replace(/\/+/g, '/')
    .replace(/^\//, '');
}

export async function fetchPipelineTriggers(pipelineID: string): Promise<PipelineTrigger[]> {
  const normalizedTarget = normalizePipelineIdentifier(pipelineID);
  if (!normalizedTarget) return [];

  const listResponse = await apiClient.fetch('/v1/overrides?include_source=true');
  if (!listResponse.ok) {
    throw new Error(await readError(listResponse, `Failed to load overrides (${listResponse.status})`));
  }
  const overridesPayload = await listResponse.json();
  const overrides: unknown[] = Array.isArray(overridesPayload) ? overridesPayload : [];
  const results: PipelineTrigger[] = [];

  await Promise.all(
    overrides.map(async entry => {
      try {
        const entryRecord = entry && typeof entry === 'object' ? (entry as Record<string, unknown>) : null;
        const slugRaw =
          typeof entry === 'string'
            ? entry
            : entryRecord?.name || entryRecord?.repository_name || entryRecord?.slug || entryRecord?.repo || '';
        const repoSlug = String(slugRaw || '').trim();
        if (!repoSlug || !repoSlug.includes('/')) return;
        const [owner, repo] = repoSlug.split('/');
        const overrideResponse = await apiClient.fetch(
          `/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`
        );
        if (!overrideResponse.ok) return;
        const yamlText = await overrideResponse.text();
        const manifest = yaml.load(yamlText) as Record<string, unknown>;
        const triggerList = Array.isArray(manifest?.triggers) ? manifest?.triggers : [];
        triggerList.forEach(item => {
          const trigger = (item || {}) as Record<string, unknown>;
          const pipelines = Array.isArray(trigger.pipelines) ? trigger.pipelines : [];
          const matches = pipelines.some((value: unknown) => {
            const valueRecord = value && typeof value === 'object' ? (value as Record<string, unknown>) : null;
            const candidate = typeof value === 'string' ? value : valueRecord?.path;
            return normalizePipelineIdentifier(candidate) === normalizedTarget;
          });
          if (matches) {
            results.push({
              repoSlug,
              source: normalizePipelineSource(typeof entryRecord?.source === 'string' ? entryRecord.source : 'database'),
              trigger,
            });
          }
        });
      } catch (innerError) {
        console.warn('Failed to parse overrides entry', innerError);
      }
    })
  );

  return results.sort((a, b) => a.repoSlug.localeCompare(b.repoSlug));
}
