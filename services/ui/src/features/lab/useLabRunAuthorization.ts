import { useEffect, useMemo, useState } from 'react';
import yaml from 'js-yaml';
import { apiClient } from '../../lib/api';
import { DEFAULT_PIPELINE_NAME } from '../../lib/lab';

export type ResourceUseCheckResult = {
  allowed: boolean;
  reason?: string;
  action?: string;
  resource_type?: string;
  resource_id?: string;
};

type LabRunValidationState = {
  loading: boolean;
  checks: ResourceUseCheckResult[];
  error: string | null;
};

function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value).trim().replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

function parsePipelineName(yamlText: string): string {
  const match = yamlText.match(/^\s*name:\s*([^\s]+)/m);
  return match ? match[1] : '';
}

export function collectLabResourceUseChecks(selectedPipelineID: string, yamlText: string, scopeValue: string) {
  const checks: Array<{ action: string; resource_type: string; resource_id: string }> = [];
  const pipelineID = selectedPipelineID.trim() || parsePipelineName(yamlText).trim() || DEFAULT_PIPELINE_NAME;
  if (pipelineID) {
    checks.push({
      action: 'pipeline.use',
      resource_type: 'pipeline',
      resource_id: pipelineID.replace(/^\/+|\/+$/g, ''),
    });
  }
  const scopeID = normalizeScopeLabel(scopeValue);
  if (scopeID) {
    checks.push({ action: 'scope.use', resource_type: 'scope', resource_id: scopeID });
  }

  try {
    const parsed = yaml.load(yamlText) as Record<string, unknown> | null;
    const steps = Array.isArray(parsed?.steps) ? parsed.steps : [];
    const seen = new Set(checks.map(check => `${check.action}:${check.resource_type}:${check.resource_id}`));
    steps.forEach(step => {
      if (!step || typeof step !== 'object') return;
      const include = (step as Record<string, unknown>).include;
      if (typeof include !== 'string') return;
      const trimmed = include.trim();
      const lower = trimmed.toLowerCase();
      let next: { action: string; resource_type: string; resource_id: string } | null = null;
      if (lower.startsWith('step:')) {
        const resourceID = trimmed.slice(5).trim().replace(/^\/+|\/+$/g, '');
        if (resourceID) next = { action: 'step.use', resource_type: 'step', resource_id: resourceID };
      } else if (lower.startsWith('pipeline:')) {
        const resourceID = trimmed.slice(9).trim().replace(/^\/+|\/+$/g, '');
        if (resourceID) {
          next = { action: 'pipeline.use', resource_type: 'pipeline', resource_id: resourceID };
        }
      }
      if (!next) return;
      const key = `${next.action}:${next.resource_type}:${next.resource_id}`;
      if (seen.has(key)) return;
      seen.add(key);
      checks.push(next);
    });
  } catch {
    return checks;
  }

  return checks;
}

export function useLabRunAuthorization(
  selectedPipelineID: string,
  yamlText: string,
  scopeValue: string,
  validationErrorCount: number
) {
  const [validation, setValidation] = useState<LabRunValidationState>({
    loading: false,
    checks: [],
    error: null,
  });
  const resourceUseChecks = useMemo(
    () => collectLabResourceUseChecks(selectedPipelineID, yamlText, scopeValue),
    [scopeValue, selectedPipelineID, yamlText]
  );

  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(async () => {
      if (validationErrorCount > 0 || resourceUseChecks.length === 0) {
        setValidation({ loading: false, checks: [], error: null });
        return;
      }
      setValidation(current => ({ ...current, loading: true, error: null }));
      try {
        const response = await apiClient.fetch('/v1/authz/resource-use/batch-check', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ checks: resourceUseChecks }),
        });
        const payload = await response.json().catch(() => null);
        if (!response.ok) {
          throw new Error(
            typeof payload === 'string' ? payload : `Unable to validate access (${response.status})`
          );
        }
        if (!cancelled) {
          setValidation({
            loading: false,
            checks: Array.isArray(payload?.results) ? payload.results : [],
            error: null,
          });
        }
      } catch (error) {
        if (!cancelled) {
          setValidation({
            loading: false,
            checks: [],
            error: error instanceof Error ? error.message : 'Unable to validate access',
          });
        }
      }
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [resourceUseChecks, validationErrorCount]);

  const deniedChecks = useMemo(
    () => validation.checks.filter(check => !check.allowed),
    [validation.checks]
  );

  return {
    ...validation,
    blocked: deniedChecks.length > 0,
    deniedChecks,
  };
}
