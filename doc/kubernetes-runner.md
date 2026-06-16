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
   `working_directory`, creates any pipeline-declared PVCs, and execs task
   actions through the Kubernetes API.

This preserves the current pipeline behavior while moving the execution
substrate from Docker containers to Kubernetes pods.

## Log Delivery

The Kubernetes runner follows the agent pod's `pods/log` stream and forwards
logs to the dispatcher in batches. After the agent pod reaches `Succeeded` or
`Failed`, the runner waits briefly for the pod log stream to drain before
cleaning up the pod, so the final task and agent lines are persisted with the
run. Very large log entries are split into transport-safe chunks before they are
sent to NopsAI.

The generated Role includes `get`, `list`, and `watch` on `pods/log`. Keep those
permissions in custom manifests; without them the run may complete but the UI
will not receive complete Kubernetes runner logs.

## Runner Per Namespace

Run one Kubernetes runner per namespace. Give every runner a unique
`runner_id`, scope list, and capacity. Scope routing still happens in the
dispatcher, so a namespace runner can be dedicated to production, a region, a
team, or a workload class. Routing updates from the UI or the system GitOps repo
are applied to the live dispatcher for new scheduling decisions.

Agent pods must also receive an `agent_nopsai_api_url` that is reachable from
inside the Kubernetes cluster. Docker Compose names such as `http://nopsai:8080`
work for Docker runners, but Kubernetes runners usually need an externally
resolvable service DNS name or ingress URL.

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
still handled by the agent: when a step declares `volumes`, the agent looks for
the named PVC in the runner namespace and creates it if it does not exist.

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
connecting to the dispatcher on restart. The generated `config.yml` and `.env`
mirrors are compatibility outputs for writable local installs, not the durable
source of truth for Kubernetes.

```yaml
runner_id: k8s-runner-ams-1
runner_scopes: production,eu-west
runner_capacity: 30

runtime: kubernetes

kubernetes:
  namespace: nopsai-runs
  service_account: nopsai-runner
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

## Installing From The UI

Go to **System > Dispatcher > Runner Deployment Guide** and generate a
Kubernetes one-time install command. The command downloads a single-use
manifest token and applies it with `kubectl`, so the long-lived service auth and
TLS secrets are not printed in the UI. The token expires after 10 minutes and is
consumed by the first successful download.

Run the generated command from a workstation or automation host where `kubectl`
already targets the destination cluster.

The generated manifest includes:

- Namespace
- ServiceAccount
- Role and RoleBinding scoped to that namespace
- Secret for dispatcher service authentication
- ConfigMap for runner runtime settings
- Deployment for `nopsai-k8s-runner`

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
