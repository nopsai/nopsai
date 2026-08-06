import { useEffect, useMemo, useRef, useState } from 'react';
import type { YamlValidationError } from '../editor/YamlValidationPanel';
import {
  validateBackendYamlResource,
  type BackendValidationIssue,
  type BackendValidationResponse,
  type BackendYamlValidationResource,
} from './api';

export type BackendYamlValidationStatus =
  | 'idle'
  | 'checking'
  | 'valid'
  | 'invalid'
  | 'unavailable';

type BackendYamlValidationOptions = {
  enabled: boolean;
  resource: BackendYamlValidationResource;
  yaml: string;
  resourceID?: string;
  path?: string;
  name?: string;
  repository?: string;
  repoOwner?: string;
  repoName?: string;
  teamPath?: string;
  debounceMs?: number;
};

type BackendYamlValidationState = {
  status: BackendYamlValidationStatus;
  result: BackendValidationResponse | null;
  error: string | null;
};

const idleState: BackendYamlValidationState = {
  status: 'idle',
  result: null,
  error: null,
};

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError';
}

export function backendIssuesToYamlValidationErrors(issues: BackendValidationIssue[]): YamlValidationError[] {
  return issues
    .filter(issue => issue && typeof issue.message === 'string' && issue.message.trim())
    .map(issue => ({
      message: issue.file ? `${issue.file}: ${issue.message}` : issue.message,
      line: typeof issue.line === 'number' && Number.isFinite(issue.line) ? issue.line : null,
    }));
}

export function useBackendYamlValidation({
  enabled,
  resource,
  yaml,
  resourceID = '',
  path = '',
  name = '',
  repository = '',
  repoOwner = '',
  repoName = '',
  teamPath = '',
  debounceMs = 450,
}: BackendYamlValidationOptions) {
  const [state, setState] = useState<BackendYamlValidationState>(idleState);
  const sequenceRef = useRef(0);

  useEffect(() => {
    sequenceRef.current += 1;
    const sequence = sequenceRef.current;
    const trimmedYaml = yaml.trim();

    if (!enabled || !trimmedYaml) {
      setState(idleState);
      return;
    }

    const controller = new AbortController();
    setState({
      status: 'checking',
      result: null,
      error: null,
    });

    const timer = window.setTimeout(() => {
      void validateBackendYamlResource(
        resource,
        {
          yaml,
          resource_id: resourceID,
          path,
          name,
          repository,
          repo_owner: repoOwner,
          repo_name: repoName,
          team_path: teamPath,
        },
        controller.signal
      )
        .then(result => {
          if (sequenceRef.current !== sequence) return;
          setState({
            status: result.valid ? 'valid' : 'invalid',
            result,
            error: null,
          });
        })
        .catch(error => {
          if (controller.signal.aborted || isAbortError(error) || sequenceRef.current !== sequence) return;
          setState({
            status: 'unavailable',
            result: null,
            error: error instanceof Error ? error.message : 'Backend validation is unavailable',
          });
        });
    }, debounceMs);

    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [
    debounceMs,
    enabled,
    name,
    path,
    repoName,
    repoOwner,
    repository,
    resource,
    resourceID,
    teamPath,
    yaml,
  ]);

  const errors = useMemo(() => (
    state.status === 'invalid' && state.result
      ? backendIssuesToYamlValidationErrors(state.result.errors)
      : []
  ), [state.result, state.status]);

  const warnings = useMemo(() => (
    state.result ? backendIssuesToYamlValidationErrors(state.result.warnings) : []
  ), [state.result]);

  return {
    status: state.status,
    result: state.result,
    error: state.error,
    errors,
    warnings,
    blockingErrorCount: state.status === 'invalid' ? errors.length : 0,
    isInvalid: state.status === 'invalid',
    isChecking: state.status === 'checking',
    isUnavailable: state.status === 'unavailable',
  };
}
