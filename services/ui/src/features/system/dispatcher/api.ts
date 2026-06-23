import { fetchSystemJson } from '../api';
import {
  normalizeKubernetesRunnerManifestTemplate,
  normalizeRunnerComposeTemplate,
  normalizeRuntimeScopeOptions,
  type KubernetesRunnerManifestTemplate,
  type RunnerComposeTemplate,
} from './model';

export type DockerRunnerTemplateInput = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  dispatcherAddress: string;
  networkMode: string;
  runnerImage: string;
};

export type KubernetesRunnerTemplateInput = {
  runnerId: string;
  runnerScopes: string;
  runnerCapacity: number;
  dispatcherAddress: string;
  namespace: string;
  serviceAccount: string;
  runnerImage: string;
  storageClass: string;
  affinityEnabled: boolean;
};

export async function fetchDispatcherScopeOptions(): Promise<string[]> {
  const payload = await fetchSystemJson('/v1/system/dispatcher/scopes', { cache: 'no-store' });
  return normalizeRuntimeScopeOptions(payload);
}

export async function fetchDockerRunnerTemplate(input: DockerRunnerTemplateInput): Promise<RunnerComposeTemplate> {
  const params = new URLSearchParams({
    runner_id: input.runnerId.trim() || 'runner-prod-1',
    runner_scopes: input.runnerScopes.trim(),
    runner_capacity: String(input.runnerCapacity),
    runner_network_mode: input.networkMode,
    runner_image: input.runnerImage.trim() || 'hoseindocker/nopsai-runner:latest',
  });
  if (input.dispatcherAddress.trim()) params.set('dispatcher_grpc_address', input.dispatcherAddress.trim());
  const payload = await fetchSystemJson(`/v1/system/dispatcher/runner-bootstrap-command?${params.toString()}`, {
    cache: 'no-store',
  });
  return normalizeRunnerComposeTemplate(payload);
}

export async function fetchKubernetesRunnerTemplate(
  input: KubernetesRunnerTemplateInput
): Promise<KubernetesRunnerManifestTemplate> {
  const params = new URLSearchParams({
    runner_id: input.runnerId.trim() || 'k8s-runner-prod-1',
    runner_scopes: input.runnerScopes.trim(),
    runner_capacity: String(input.runnerCapacity),
    namespace: input.namespace.trim() || 'nopsai-runs',
    service_account: input.serviceAccount.trim() || 'nopsai-runner',
    runner_image: input.runnerImage.trim() || 'hoseindocker/nopsai-k8s-runner:latest',
    affinity_enabled: String(input.affinityEnabled),
  });
  if (input.dispatcherAddress.trim()) params.set('dispatcher_grpc_address', input.dispatcherAddress.trim());
  if (input.storageClass.trim()) params.set('storage_class', input.storageClass.trim());
  const payload = await fetchSystemJson(
    `/v1/system/dispatcher/kubernetes-runner-bootstrap-command?${params.toString()}`,
    { cache: 'no-store' }
  );
  return normalizeKubernetesRunnerManifestTemplate(payload);
}
