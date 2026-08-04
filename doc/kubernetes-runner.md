# Kubernetes Runner

NopsAI supports a Kubernetes runner runtime for running the same dispatcher,
agent, pipeline, step, secret, variable, LLM, MCP, child pipeline, cancellation,
and log flows on a Kubernetes cluster.

## Runtime Shape

The Kubernetes runner is a separate in-cluster service:

1. `nopsai-k8s-runner` runs as a Deployment in one namespace.
2. It registers with the dispatcher like any other runner and advertises
   `runtime=kubernetes`.
3. For each assigned run it starts one agent pod.
4. The agent pod owns the run workspace volume and mounts it at `/workspace`.
5. The agent runs in `NOPSAI_RUNTIME=kubernetes` mode and creates one step pod
   per pipeline step image.
6. The agent mounts the same workspace PVC into each step pod at the pipeline
   `working_directory`, binds pipeline-declared PVCs by name, and execs
   task actions through the Kubernetes API.

This preserves the current pipeline behavior while moving the execution
substrate from Docker containers to Kubernetes pods.

## Log Delivery

The Kubernetes runner follows the agent pod's `pods/log` stream and forwards
logs to the dispatcher in batches. If Kubernetes closes the follow stream before
the agent pod reaches `Succeeded` or `Failed`, the runner reattaches using the
last observed Kubernetes log timestamp. Once the pod is terminal, the runner also
performs a final non-follow log read to capture any lines that arrived during
stream shutdown. Duplicate terminal lines are suppressed before large log entries
are split into transport-safe chunks and sent to NopsAI.

The generated Role includes `get`, `list`, and `watch` on `pods/log`. Keep those
permissions in custom manifests; without them the run may complete but the UI
will not receive complete Kubernetes runner logs.

Runner Deployment logs are exposed separately through System Logs when the
NopsAI API has Kubernetes log access to the runner namespace. Generated runner
manifests carry `nopsai.io/runner-id` and `nopsai.io/platform-id`, and advertise
`runner:<runner-id>` in dispatcher metadata with `kubernetes_namespace`,
`kubernetes_label_selector`, and `nopsai_platform_id`, so the Dispatcher runner
detail can open the matching source. The source is marked unavailable until the
configured System Logs provider can see an owned pod in that namespace.

## Runner Per Namespace

Run one Kubernetes runner per namespace. Give every runner a unique
`runner_id`, scope list, and capacity. Scope routing still happens in the
dispatcher, so a namespace runner can be dedicated to production, a region, a
team, or a workload class. Routing updates from the UI or the system GitOps repo
are applied to the live dispatcher for new scheduling decisions.

Runner registration also contributes to the dispatcher's effective routing
view. The configured `dispatcher_routing` map remains GitOps-owned, while a
newly connected runner is immediately eligible for the scopes it advertises.
Dispatcher status exposes both configured and effective routes, and the Scope
and Team pages show the registered runners that can receive work for the
selected scope or team subtree.

Removing a runner from **System > Dispatcher** clears its dispatcher
registration and disconnects any live stream. It does not revoke the runner ID,
so the same name can be reused after the old Deployment is deleted or scaled
down. Add the ID to `ejected_runner_ids` only when it must stay revoked.
Existing revocations can be cleared from **System > Config > Revoked runner
IDs** before reinstalling with the same name. Generating a replacement runner
install command also clears a stale revocation for that requested runner ID.

Generated Kubernetes runner resource names include a unique runner identity plus
a stable platform ownership ID derived from the NopsAI installation. The
submitted `runner_id` is kept as `RUNNER_NAME`; the emitted `RUNNER_ID` appends
a random suffix unless `runner_uid` is supplied for GitOps reproducibility.
Reusing the same runner name from a different NopsAI platform creates a
different Deployment, Secret, and ConfigMap instead of patching the existing
runner owned by another platform in the same namespace.

Agent pods must also receive a `NOPSAI_API_URL` that is reachable from inside
the Kubernetes cluster. Docker Compose names such as `http://nopsai:8080` work
for Docker runners, but Kubernetes runners usually need an externally
resolvable service DNS name or ingress URL.

The runner uses two Kubernetes identities. `kubernetes.service_account` is the
runner orchestration identity and owns namespace-scoped RBAC for pods, pod
logs, pod exec, PVCs, and events. The runner Deployment and dynamically created
agent pods use this identity because agents create step pods and PVCs.
`kubernetes.workload_service_account` is used by step pods. New Helm and
generated manifests create this workload ServiceAccount without NopsAI RBAC and set
`kubernetes.workload_automount_service_account_token: false` by default. Private
registry access is preserved through explicit `kubernetes.image_pull_secrets`,
which are passed to both agent pods and step pods.

The generated runner install commands never expose long-lived secrets directly.
They download a one-time bootstrap script through the NopsAI HTTP API. When
the configured `dispatcher_grpc_address` is an internal stack name such as
`dispatcher:9090`, NopsAI derives an external dispatcher endpoint from the
request host and dispatcher port and emits `DISPATCHER_GRPC_ADDRESS`. For
Docker/OrbStack-style service hosts such as `nopsai-ui.<env>`, it uses the sibling dispatcher Service host
`dispatcher.<env>:9090` instead of the UI host. If the generated runner lives
in a different Kubernetes namespace from the control plane, prefer the
fully-qualified Service DNS name, for example
`dispatcher.<platform-namespace>.svc.cluster.local:9090`. The Dispatcher runner
install panel also lets operators override the dispatcher address for a single
generated command without changing the persisted runtime config. An explicit
empty `runner_scopes` value means the runner accepts all scopes.

The visible Kubernetes install command downloads and executes that one-time
script. The script writes the generated manifest to a temporary file, applies
it, waits for the runner Deployment rollout, and prints recent runner logs. If
the Deployment does not become ready, it prints pod, deployment, and runner log
diagnostics so network, image, RBAC, or dispatcher-address problems are visible
from the same terminal session.

## Workspace PVCs And Node Locality

By default the runner does not create a standalone PVC itself. It starts the
agent pod with a Kubernetes generic ephemeral PVC template using:

- `kubernetes.default_workspace_size`
- `kubernetes.default_workspace_access_mode`
- `kubernetes.storage_class`

Kubernetes creates the PVC for the agent pod, and the agent passes that claim to
the step pods for the run. The agent pod always mounts the workspace at
`/workspace`; step pods mount the same PVC at the normalized pipeline
`working_directory`, matching Docker runner behavior. Absolute working
directories such as `/tmp/test` are supported. Pipeline-declared volumes are
still handled by the agent: when a step declares `volumes`, the agent reuses an
existing PVC with that name in the runner namespace, or creates it with NopsAI
labels when it is missing. Steps and runs that declare the same PVC name share
the same writable storage, subject to the PVC access mode.

Runner pods, agent pods, and step pods use `RuntimeDefault` seccomp and disable
privilege escalation by default. Step pods do not drop workload container
capabilities or force `runAsNonRoot`, because many enterprise base images still
declare root as their default user and would fail before the task starts.

You can also configure `kubernetes.existing_workspace_pvc` with
`workspace_volume_mode: existing` when you want the agent and step pods to mount
a pre-created workspace PVC instead.

`kubernetes.affinity_enabled` defaults to `true` on the runner. The runner can
be scheduled anywhere. The agent pod is scheduled by Kubernetes using the
default runtime pool selector, and then the agent pins all step pods to the
agent pod's actual node. This is intentional for `ReadWriteOnce` PVCs.

A pipeline can override the runner default with the pipeline-level
`affinity_enabled` directive:

```yaml
affinity_enabled: false
```

Docker runners ignore this directive. Set it to `false` only when your storage
class supports safe multi-node mounting or when the cluster scheduler should be
free to place step pods anywhere.

`emptyDir` is not emitted by the install manifest because NopsAI uses separate
agent and step pods. Use PVC mode for full pipeline compatibility.

## GitOps Settings

Store runtime settings in the system config repository at
`setting/system/runner.yaml`:

NopsAI persists synced runtime settings in the database and reloads them before
connecting to the dispatcher on restart. `config.yml`, `.env`, Docker Compose,
and Kubernetes manifests are bootstrap inputs, not the durable source of truth.
Services that support reloads can consume the versioned runtime snapshot API at
`/internal/v1/runtime-config/{service}`.

```yaml
runner_id: k8s-runner-ams-1
runner_scopes: production,eu-west
runner_capacity: 30

runtime: kubernetes

kubernetes:
  namespace: nopsai-runs
  service_account: nopsai-runner
  workload_service_account: nopsai-runner-workload
  workload_automount_service_account_token: false
  image_pull_secrets:
    - regcred
  default_image_pull_policy: IfNotPresent
  default_workspace_size: 5Gi
  default_workspace_access_mode: ReadWriteOnce
  default_task_timeout: 30m
  default_run_timeout: 2h
  storage_class: fast-rwo
  affinity_enabled: true

limits:
  max_concurrent_runs: 30
  max_concurrent_tasks: 200
  max_concurrent_tasks_per_run: 20
  max_pending_tasks: 1000

runtime_pools:
  default:
    node_selector:
      workload: nopsai
  high-memory:
    node_selector:
      workload: nopsai
      node-class: memory
    resources:
      requests:
        memory: 4Gi
      limits:
        memory: 16Gi
```

Runtime pools are passed to the agent. A step can select a pool by providing
`runtime_pool` in the pipeline or step definition:

```yaml
runtime_pool: default
affinity_enabled: true

steps:
  - name: heavy-build
    runtime_pool: high-memory
    image: golang:1.24
    tasks:
      - name: test
        script: go test ./...
```

The pipeline-level value is the default for Kubernetes step pods. A step-level
value overrides it. Docker runners ignore `runtime_pool`.

## Configuring Pools From The UI

Go to **System > Config > Runtime pools** to add or edit Kubernetes scheduling
pools. Each pool can define:

- `node_selector`: labels used to place step pods on matching nodes.
- `resources.requests`: CPU, memory, or other Kubernetes resource requests.
- `resources.limits`: CPU, memory, or other Kubernetes resource limits.

Saving the form persists the same `runtime_pools` shape used by GitOps in
`setting/system/runner.yaml`. The runner install command and Kubernetes
manifest include the configured pools as `KUBERNETES_RUNTIME_POOLS`.

Pipeline, Lab, and reusable-step editors suggest configured pool names when
editing a `runtime_pool:` value. Use Ctrl+Space in the YAML editor to open
suggestions, or type the pool name directly.

## Installing From The UI

Go to **System > Dispatcher > Install runner** and generate a Kubernetes
one-time install command. The command downloads a single-use install script and
executes it locally, so the long-lived service auth and TLS secrets are not
printed in the UI. The token expires after 10 minutes and is consumed by the
first successful download.

Run the generated command from a workstation or automation host where `kubectl`
already targets the destination cluster.

The generated manifest includes:

- Namespace
- ServiceAccount
- Role and RoleBinding scoped to that namespace
- Secret for dispatcher service authentication
- Optional Docker registry Secret from selected `docker_config_json`
  credentials
- ConfigMap for runner runtime settings
- Deployment for `nopsai-k8s-runner`

The API response also returns `runner_name`, `platform_id`, and `resource_name`.
Commit the generated YAML as-is for GitOps so the runner identity, platform
ownership label, and `KUBERNETES_RUNNER_LABEL_SELECTOR` stay aligned with the
Deployment name. Helm installs pass `global.platformID` to the API as
`NOPSAI_PLATFORM_ID` so bundled runners and generated runner manifests use the
same ownership boundary.

When registry credentials are selected in the install UI, the one-time
bootstrap command resolves only those credentials, creates a
`kubernetes.io/dockerconfigjson` Secret, and references it as an
`imagePullSecret` for the runner Deployment plus the workload ServiceAccount.
This covers the runner image plus agent and step images that Kubernetes pulls
in that namespace. The raw manifest endpoint below does not include credential
material; for GitOps cluster manifests, create the imagePullSecret through
infrastructure or secret-manager automation and list it in
`kubernetes.image_pull_secrets`.

Refresh **System > Dispatcher** to confirm the runner is registered. Kubernetes
runners show their runtime, namespace, service account, scope, capacity, and
active runs in Dispatcher and Monitoring.

For GitOps, use the authenticated manifest endpoint and commit the generated
YAML to your cluster repository:

```bash
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "https://nopsai.example.com/v1/system/dispatcher/kubernetes-runner-manifest?runner_id=k8s-runner-ams-1&runner_scopes=production,eu-west&runner_capacity=30&namespace=nopsai-runs" \
  -o nopsai-k8s-runner.yaml
```

## Building The Runner Image

`docker-compose.yaml` includes `k8s-runner` as a build/push-only service under
the `images` profile. It is not started by the local Compose stack because the
runner should execute inside the Kubernetes cluster.

Build and push the image with:

```bash
docker compose build base k8s-runner
docker compose --profile images push k8s-runner
```
