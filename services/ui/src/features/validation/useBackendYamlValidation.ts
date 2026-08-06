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
  key: string;
  status: Exclude<BackendYamlValidationStatus, 'idle' | 'checking'>;
  result: BackendValidationResponse | null;
  error: string | null;
};

type BackendYamlValidationSnapshot = {
  status: BackendYamlValidationStatus;
  result: BackendValidationResponse | null;
  error: string | null;
};

const emptyState: BackendYamlValidationState = {
  key: '',
  status: 'valid',
  result: null,
  error: null,
};

const idleState: BackendYamlValidationSnapshot = {
  status: 'idle',
  result: null,
  error: null,
};

const checkingState: BackendYamlValidationSnapshot = {
  status: 'checking',
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
  const [state, setState] = useState<BackendYamlValidationState>(emptyState);
  const sequenceRef = useRef(0);
  const validationActive = enabled && Boolean(yaml.trim());
  const request = useMemo(() => ({
    yaml,
    resource_id: resourceID,
    path,
    name,
    repository,
    repo_owner: repoOwner,
    repo_name: repoName,
    team_path: teamPath,
  }), [name, path, repoName, repoOwner, repository, resourceID, teamPath, yaml]);
  const requestKey = useMemo(() => (
    validationActive ? JSON.stringify([resource, request]) : ''
  ), [request, resource, validationActive]);

  useEffect(() => {
    sequenceRef.current += 1;
    const sequence = sequenceRef.current;

    if (!validationActive) {
      return;
    }

    const controller = new AbortController();

    const timer = window.setTimeout(() => {
      void validateBackendYamlResource(
        resource,
        request,
        controller.signal
      )
        .then(result => {
          if (sequenceRef.current !== sequence) return;
          setState({
            key: requestKey,
            status: result.valid ? 'valid' : 'invalid',
            result,
            error: null,
          });
        })
        .catch(error => {
          if (controller.signal.aborted || isAbortError(error) || sequenceRef.current !== sequence) return;
          setState({
            key: requestKey,
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
    request,
    requestKey,
    resource,
    validationActive,
  ]);

  const currentState: BackendYamlValidationSnapshot = !validationActive
    ? idleState
    : state.key === requestKey
      ? state
      : checkingState;

  const errors = useMemo(() => (
    currentState.status === 'invalid' && currentState.result
      ? backendIssuesToYamlValidationErrors(currentState.result.errors)
      : []
  ), [currentState.result, currentState.status]);

  const warnings = useMemo(() => (
    currentState.result ? backendIssuesToYamlValidationErrors(currentState.result.warnings) : []
  ), [currentState.result]);

  return {
    status: currentState.status,
    result: currentState.result,
    error: currentState.error,
    errors,
    warnings,
    blockingErrorCount: currentState.status === 'invalid' ? errors.length : 0,
    isInvalid: currentState.status === 'invalid',
    isChecking: currentState.status === 'checking',
    isUnavailable: currentState.status === 'unavailable',
  };
}
