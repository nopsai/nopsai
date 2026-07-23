import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ConfigFormState } from '../config/model';
import { fetchCredentials } from '../credentials/api';
import type { CredentialRecord } from '../credentials/model';
import { fetchDispatcherScopeOptions, fetchDockerRunnerTemplate, fetchKubernetesRunnerTemplate, fetchPlatformVersionTag } from './api';
import {
  DEFAULT_DOCKER_RUNNER_IMAGE,
  DEFAULT_KUBERNETES_RUNNER_IMAGE,
  DOCKER_RUNNER_IMAGE_REPOSITORY,
  KUBERNETES_RUNNER_IMAGE_REPOSITORY,
  runnerImageForVersion,
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
  const [dispatcherAddress, setDispatcherAddress] = useState('');
  const [runnerNetworkMode, setRunnerNetworkMode] = useState('host');
  const [runnerImage, setRunnerImage] = useState(DEFAULT_DOCKER_RUNNER_IMAGE);
  const [kubernetesNamespace, setKubernetesNamespace] = useState('nopsai-runs');
  const [kubernetesServiceAccount, setKubernetesServiceAccount] = useState('nopsai-runner');
  const [kubernetesRunnerImage, setKubernetesRunnerImage] = useState(DEFAULT_KUBERNETES_RUNNER_IMAGE);
  const [kubernetesStorageClass, setKubernetesStorageClass] = useState('');
  const [kubernetesAffinityEnabled, setKubernetesAffinityEnabled] = useState(true);
  const [scopeOptions, setScopeOptions] = useState<string[]>([]);
  const [registryCredentials, setRegistryCredentials] = useState<CredentialRecord[]>([]);
  const [registryCredentialsLoading, setRegistryCredentialsLoading] = useState(false);
  const [registryCredentialsError, setRegistryCredentialsError] = useState<string | null>(null);
  const [selectedRegistryCredentialRefs, setSelectedRegistryCredentialRefs] = useState<string[]>([]);
  const [template, setTemplate] = useState<RunnerComposeTemplate | null>(null);
  const [kubernetesTemplate, setKubernetesTemplate] = useState<KubernetesRunnerManifestTemplate | null>(null);
  const [loadingTemplate, setLoadingTemplate] = useState(false);
  const [loadingKubernetesTemplate, setLoadingKubernetesTemplate] = useState(false);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [kubernetesTemplateError, setKubernetesTemplateError] = useState<string | null>(null);

  useEffect(() => {
    if (!canManageDispatcher) return;
    let cancelled = false;
    void fetchPlatformVersionTag()
      .then(versionTag => {
        if (cancelled) return;
        const dockerImage = runnerImageForVersion(DOCKER_RUNNER_IMAGE_REPOSITORY, versionTag);
        const kubernetesImage = runnerImageForVersion(KUBERNETES_RUNNER_IMAGE_REPOSITORY, versionTag);
        setRunnerImage(current => (current === DEFAULT_DOCKER_RUNNER_IMAGE ? dockerImage : current));
        setKubernetesRunnerImage(current => (current === DEFAULT_KUBERNETES_RUNNER_IMAGE ? kubernetesImage : current));
      })
      .catch(error => {
        console.error('Failed to load platform version for runner image defaults', error);
      });
    setRegistryCredentialsLoading(true);
    setRegistryCredentialsError(null);
    void fetchDispatcherScopeOptions()
      .then(options => {
        if (!cancelled) setScopeOptions(options);
      })
      .catch(error => {
        console.error('Failed to load dispatcher scope options', error);
        if (!cancelled) setScopeOptions([]);
      });
    void fetchCredentials()
      .then(credentials => {
        if (!cancelled) {
          setRegistryCredentials(
            credentials.filter(credential => credential.kind === 'docker_config_json' && credential.status === 'active' && credential.has_value)
          );
        }
      })
      .catch(error => {
        console.error('Failed to load registry credentials', error);
        if (!cancelled) {
          setRegistryCredentials([]);
          setRegistryCredentialsError(error instanceof Error ? error.message : 'Unable to load registry credentials.');
        }
      })
      .finally(() => {
        if (!cancelled) setRegistryCredentialsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [canManageDispatcher]);

  const selectedRunnerScopes = useMemo(() => splitRuntimeScopes(runnerScopes), [runnerScopes]);
  const selectedRunnerScopeSet = useMemo(() => new Set(selectedRunnerScopes), [selectedRunnerScopes]);
  const selectedRegistryCredentialRefSet = useMemo(() => new Set(selectedRegistryCredentialRefs), [selectedRegistryCredentialRefs]);
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

  const toggleRegistryCredentialRef = useCallback((ref: string, checked: boolean) => {
    setSelectedRegistryCredentialRefs(current => {
      const next = new Set(current);
      if (checked) next.add(ref);
      else next.delete(ref);
      return Array.from(next).sort((left, right) => left.localeCompare(right));
    });
  }, []);

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
          dispatcherAddress,
          networkMode: runnerNetworkMode,
          runnerImage,
          registryCredentialRefs: selectedRegistryCredentialRefs,
        })
      );
    } catch (error) {
      setTemplate(null);
      setTemplateError(error instanceof Error ? error.message : 'Unable to generate runner install command.');
    } finally {
      setLoadingTemplate(false);
    }
  }, [
    canManageDispatcher,
    dispatcherAddress,
    runnerCapacity,
    runnerId,
    runnerImage,
    runnerNetworkMode,
    runnerScopes,
    selectedRegistryCredentialRefs,
  ]);

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
          dispatcherAddress,
          namespace: kubernetesNamespace,
          serviceAccount: kubernetesServiceAccount,
          runnerImage: kubernetesRunnerImage,
          storageClass: kubernetesStorageClass,
          affinityEnabled: kubernetesAffinityEnabled,
          registryCredentialRefs: selectedRegistryCredentialRefs,
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
    dispatcherAddress,
    kubernetesAffinityEnabled,
    kubernetesNamespace,
    kubernetesRunnerImage,
    kubernetesServiceAccount,
    kubernetesStorageClass,
    runnerCapacity,
    runnerId,
    runnerScopes,
    selectedRegistryCredentialRefs,
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
    dispatcherAddress,
    setDispatcherAddress,
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
    registryCredentials,
    registryCredentialsLoading,
    registryCredentialsError,
    selectedRegistryCredentialRefs,
    selectedRegistryCredentialRefSet,
    runnerScopeChoices,
    toggleRunnerScope,
    toggleRegistryCredentialRef,
    loadTemplate,
    loadKubernetesTemplate,
  };
}
