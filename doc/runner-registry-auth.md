# Runner Private Registry Auth

NopsAI can deliver read-only Docker registry authentication for runner installs
and Docker-runner image pulls without making every runner an administrator of
the full credential store.

## Ownership Boundary

Infrastructure administrators still own the platform deployment credentials:

- Helm `global.imagePullSecrets` or ServiceAccount `imagePullSecrets` used to
  pull the NopsAI control-plane and in-cluster runner release images.
- Docker host, Kubernetes cluster, namespace, service account, and secret
  manager provisioning.
- Network egress to private registries.

NopsAI owns only the selected runner registry assignment:

- An administrator creates active credentials of kind `docker_config_json`.
- During runner install generation, the administrator selects one or more
  credential references.
- NopsAI validates the references through AAA, records the runner assignment,
  and delivers auth only for registries present in those selected Docker config
  files.

This matters for Docker runners because they pull images through the Docker
Engine API. A host-level `docker login` can help an operator manually test the
host, but the runner must pass `ImagePullOptions.RegistryAuth` when it asks the
Docker API to pull agent or step images.

## Credential Format

Use the same JSON shape that Docker stores in `~/.docker/config.json`:

```json
{
  "auths": {
    "ghcr.io": {
      "auth": "<base64-user-pass>"
    },
    "registry.company.example": {
      "username": "robot",
      "password": "<token>"
    }
  }
}
```

The credential value is encrypted like other NopsAI credentials. NopsAI parses
the registry hostnames and stores only non-secret metadata such as:

```json
{
  "registry_hosts": ["ghcr.io", "registry.company.example"]
}
```

## Docker Runner Flow

1. The administrator opens **System > Dispatcher > Runner Installs**.
2. They choose a Docker runtime and select one or more active
   `docker_config_json` credentials.
3. The generated command contains only a short-lived one-time bearer token.
4. When the script is downloaded, NopsAI resolves the selected configs, merges
   them, and embeds the merged Docker config in the downloaded shell script as
   base64.
5. The bootstrap script writes a temporary Docker CLI config only for the
   initial `docker pull` of the runner image.
6. The runner starts with its normal service identity and
   `NOPSAI_REGISTRY_DOCKER_CONFIG_B64` in its container environment. This works
   for local and remote Docker daemons because the value is part of the
   container create request rather than a host bind mount.
7. For later agent image pulls, the Docker runner decodes that local env-carried
   config and passes the matching per-image `RegistryAuth` value to the Docker
   API.
8. The Docker runner passes the same env value into agent containers, so step
   image pulls use the same local matching logic.

Registry values are not printed in the UI, stored in the generated command, or
sent back to NopsAI during Docker image pulls. The base64 env value is still
registry secret material and can be seen by administrators with Docker inspect
or equivalent host/container access, so treat the runner host as a secret
boundary. Rotating the selected registry credential requires regenerating the
runner install or updating the runner container environment through the same
infrastructure process.

## Kubernetes Runner Flow

Kubernetes can pull private images through ServiceAccount `imagePullSecrets`.
When selected registry credentials are present, the one-time Kubernetes
bootstrap command creates a `kubernetes.io/dockerconfigjson` Secret and attaches
it to the runner ServiceAccount before applying the runner Deployment.

The raw manifest preview endpoint is GitOps-friendly and does not include this
secret material. If you commit raw manifests to a cluster repository, create the
registry Secret out-of-band through the cluster's normal secret-management
process, then attach it to the ServiceAccount.

## GitOps

Runner registry assignments can be declared in the system/global runtime file:

```yaml
runner_registry_credentials:
  runner-prod-1:
    - credential://system/registry/production-ghcr
    - credential://system/registry/internal-harbor
  k8s-runner-prod:
    - credential://system/registry/production-ghcr
```

Only references are stored in `setting/system/runner.yaml`. The encrypted
credential values belong in `setting/system/credentials.yaml` or in the
database-managed **Credentials** page.

## Security And Audit

- Bootstrap install tokens are short-lived, single-use, and sent as bearer
  headers.
- Registry auth is resolved again at bootstrap download time so disabled or
  rotated credentials are honored.
- Docker runners do not call NopsAI for every image pull. They match the image
  registry host against the local env-carried Docker config.
- Agents filter `NOPSAI_REGISTRY_DOCKER_CONFIG_*` out of pipeline environment
  inheritance so registry auth is used for image pulls, not exposed as a normal
  step variable.
- Credential access logs record registry-auth delivery purpose without storing
  the secret payload.
- `GET /metrics` includes `nopsai_registry_auth_resolutions_total` for
  bootstrap-time registry-auth delivery activity and compatibility broker use.
