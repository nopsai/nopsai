import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ConfigFormState } from '../config/model';
import { fetchDispatcherScopeOptions, fetchDockerRunnerTemplate, fetchKubernetesRunnerTemplate } from './api';
import {
  sortRuntimeScopeOptions,
  splitRuntimeScopes,
  type KubernetesRunnerManifestTemplate,
  type RunnerComposeTemplate,
  type RunnerInstallRuntime,
} from './model';

export function useRunnerDeploymentGuide(canManageDispatcher: boolean, runnerDefaults: ConfigFormState) {
  const [installRuntime, setInstallRuntime] = useState<RunnerInstallRuntime>('docker');
  const [runnerId, setRunnerId] = useState(runnerDefaults.runner_id || 'runner-prod-1');
  const [runnerScopes, setRunnerScopes] = useState(runnerDefaults.runner_scopes || 'prod');
  const [runnerCapacity, setRunnerCapacity] = useState(runnerDefaults.runner_capacity || '2');
  const [runnerNetworkMode, setRunnerNetworkMode] = useState('host');
  const [runnerImage, setRunnerImage] = useState('hoseindocker/nopsai-runner:latest');
  const [kubernetesNamespace, setKubernetesNamespace] = useState('nopsai-runs');
  const [kubernetesServiceAccount, setKubernetesServiceAccount] = useState('nopsai-runner');
  const [kubernetesRunnerImage, setKubernetesRunnerImage] = useState('hoseindocker/nopsai-k8s-runner:latest');
  const [kubernetesStorageClass, setKubernetesStorageClass] = useState('');
  const [kubernetesAffinityEnabled, setKubernetesAffinityEnabled] = useState(true);
  const [scopeOptions, setScopeOptions] = useState<string[]>([]);
  const [template, setTemplate] = useState<RunnerComposeTemplate | null>(null);
  const [kubernetesTemplate, setKubernetesTemplate] = useState<KubernetesRunnerManifestTemplate | null>(null);
  const [loadingTemplate, setLoadingTemplate] = useState(false);
  const [loadingKubernetesTemplate, setLoadingKubernetesTemplate] = useState(false);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [kubernetesTemplateError, setKubernetesTemplateError] = useState<string | null>(null);

  useEffect(() => {
    if (!canManageDispatcher) return;
    let cancelled = false;
    void fetchDispatcherScopeOptions()
      .then(options => {
        if (!cancelled) setScopeOptions(options);
      })
      .catch(error => {
        console.error('Failed to load dispatcher scope options', error);
        if (!cancelled) setScopeOptions([]);
      });
    return () => {
      cancelled = true;
    };
  }, [canManageDispatcher]);

  const selectedRunnerScopes = useMemo(() => splitRuntimeScopes(runnerScopes), [runnerScopes]);
  const selectedRunnerScopeSet = useMemo(() => new Set(selectedRunnerScopes), [selectedRunnerScopes]);
  const runnerScopeChoices = useMemo(
    () => sortRuntimeScopeOptions(Array.from(new Set([...scopeOptions, ...selectedRunnerScopes]))),
    [scopeOptions, selectedRunnerScopes]
  );

  const toggleRunnerScope = useCallback(
    (scope: string, checked: boolean) => {
      const next = new Set(selectedRunnerScopes);
      if (checked) next.add(scope);
      else next.delete(scope);
      setRunnerScopes(sortRuntimeScopeOptions(Array.from(next)).join(','));
    },
    [selectedRunnerScopes]
  );

  const loadTemplate = useCallback(async () => {
    if (!canManageDispatcher) return;
    const capacity = Number.parseInt(runnerCapacity, 10);
    if (!Number.isFinite(capacity) || capacity <= 0) {
      setTemplateError('Capacity must be a positive number.');
      return;
    }
    setLoadingTemplate(true);
    setTemplateError(null);
    try {
      setTemplate(
        await fetchDockerRunnerTemplate({
          runnerId,
          runnerScopes,
          runnerCapacity: capacity,
          networkMode: runnerNetworkMode,
          runnerImage,
        })
      );
    } catch (error) {
      setTemplate(null);
      setTemplateError(error instanceof Error ? error.message : 'Unable to generate runner install command.');
    } finally {
      setLoadingTemplate(false);
    }
  }, [canManageDispatcher, runnerCapacity, runnerId, runnerImage, runnerNetworkMode, runnerScopes]);

  const loadKubernetesTemplate = useCallback(async () => {
    if (!canManageDispatcher) return;
    const capacity = Number.parseInt(runnerCapacity, 10);
    if (!Number.isFinite(capacity) || capacity <= 0) {
      setKubernetesTemplateError('Capacity must be a positive number.');
      return;
    }
    setLoadingKubernetesTemplate(true);
    setKubernetesTemplateError(null);
    try {
      setKubernetesTemplate(
        await fetchKubernetesRunnerTemplate({
          runnerId,
          runnerScopes,
          runnerCapacity: capacity,
          namespace: kubernetesNamespace,
          serviceAccount: kubernetesServiceAccount,
          runnerImage: kubernetesRunnerImage,
          storageClass: kubernetesStorageClass,
          affinityEnabled: kubernetesAffinityEnabled,
        })
      );
    } catch (error) {
      setKubernetesTemplate(null);
      setKubernetesTemplateError(error instanceof Error ? error.message : 'Unable to generate Kubernetes install command.');
    } finally {
      setLoadingKubernetesTemplate(false);
    }
  }, [
    canManageDispatcher,
    kubernetesAffinityEnabled,
    kubernetesNamespace,
    kubernetesRunnerImage,
    kubernetesServiceAccount,
    kubernetesStorageClass,
    runnerCapacity,
    runnerId,
    runnerScopes,
  ]);

  return {
    installRuntime,
    setInstallRuntime,
    runnerId,
    setRunnerId,
    runnerScopes,
    setRunnerScopes,
    runnerCapacity,
    setRunnerCapacity,
    runnerNetworkMode,
    setRunnerNetworkMode,
    runnerImage,
    setRunnerImage,
    kubernetesNamespace,
    setKubernetesNamespace,
    kubernetesServiceAccount,
    setKubernetesServiceAccount,
    kubernetesRunnerImage,
    setKubernetesRunnerImage,
    kubernetesStorageClass,
    setKubernetesStorageClass,
    kubernetesAffinityEnabled,
    setKubernetesAffinityEnabled,
    template,
    kubernetesTemplate,
    loadingTemplate,
    loadingKubernetesTemplate,
    templateError,
    setTemplateError,
    kubernetesTemplateError,
    setKubernetesTemplateError,
    selectedRunnerScopes,
    selectedRunnerScopeSet,
    runnerScopeChoices,
    toggleRunnerScope,
    loadTemplate,
    loadKubernetesTemplate,
  };
}
