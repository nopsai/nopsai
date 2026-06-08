import { useCallback, useState, type Dispatch, type SetStateAction } from 'react';
import { apiClient } from '../../lib/api';
import { DEFAULT_PIPELINE_NAME, validateOverrideKey } from '../../lib/lab';
import type { LabFeedback, LabOverride } from './useLabSession';

type LabRunMutationOptions = {
  accessBlocked: boolean;
  accessLoading: boolean;
  overrides: LabOverride[];
  scopeValue: string;
  selectedPipelineId: string;
  setFeedback: Dispatch<SetStateAction<LabFeedback>>;
  validationErrorCount: number;
  yamlText: string;
};

function parsePipelineName(yamlText: string): string {
  const match = yamlText.match(/^\s*name:\s*([^\s]+)/m);
  return match ? match[1] : '';
}

export function extractLabRunID(payload: unknown): string {
  if (!payload) return '';
  if (typeof payload === 'string') {
    const fullMatch = payload.match(
      /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i
    );
    if (fullMatch) return fullMatch[0];
    const shortMatch = payload.match(/[0-9a-f]{8}-[0-9a-f]{4}/i);
    return shortMatch ? shortMatch[0] : '';
  }
  if (typeof payload === 'object') {
    const record = payload as Record<string, unknown>;
    const runId = record.run_id ?? record.id ?? '';
    return typeof runId === 'string' ? runId : '';
  }
  return '';
}

export function useLabRunMutation({
  accessBlocked,
  accessLoading,
  overrides,
  scopeValue,
  selectedPipelineId,
  setFeedback,
  validationErrorCount,
  yamlText,
}: LabRunMutationOptions) {
  const [runPending, setRunPending] = useState(false);

  const run = useCallback(async () => {
    if (validationErrorCount > 0) {
      setFeedback({ tone: 'error', message: 'Fix validation errors first.' });
      return false;
    }
    if (accessBlocked) {
      setFeedback({ tone: 'error', message: 'Run access is not ready yet.' });
      return false;
    }
    if (accessLoading) {
      setFeedback({ tone: 'info', message: 'Access check is still running.' });
      return false;
    }

    const variables: Record<string, string> = {};
    for (const row of overrides) {
      const key = row.key.trim();
      if (!key) continue;
      if (!validateOverrideKey(key)) {
        setFeedback({
          tone: 'error',
          message: `Invalid override key '${key}'. Use only letters, numbers, underscore, dot, hyphen.`,
        });
        return false;
      }
      variables[key] = row.value;
    }

    const payload: Record<string, unknown> = {
      pipeline:
        selectedPipelineId || parsePipelineName(yamlText) || DEFAULT_PIPELINE_NAME,
      definition: yamlText,
    };
    if (Object.keys(variables).length > 0) payload.variables = variables;
    if (scopeValue) payload.scope = scopeValue;

    setRunPending(true);
    setFeedback(null);
    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' };
      if (scopeValue) headers['X-Nopsai-Scope'] = scopeValue;
      const response = await apiClient.fetch('/v1/run', {
        method: 'POST',
        headers,
        body: JSON.stringify(payload),
      });
      const contentType = response.headers.get('content-type') || '';
      const body = contentType.includes('application/json')
        ? await response.json()
        : await response.text();
      if (!response.ok) {
        const message = typeof body === 'string' ? body : JSON.stringify(body);
        throw new Error(message || `Run failed (${response.status})`);
      }

      const runId = extractLabRunID(body);
      setFeedback({
        tone: 'success',
        message: 'Run started!',
        runId: runId || undefined,
      });
      return true;
    } catch (error) {
      setFeedback({
        tone: 'error',
        message: `Run failed: ${error instanceof Error ? error.message : 'Unknown error'}`,
      });
      return false;
    } finally {
      setRunPending(false);
    }
  }, [
    accessBlocked,
    accessLoading,
    overrides,
    scopeValue,
    selectedPipelineId,
    setFeedback,
    validationErrorCount,
    yamlText,
  ]);

  return { run, runPending };
}
