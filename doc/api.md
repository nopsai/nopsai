docker-compose down -v ; docker container prune -f -a && docker volume prune -f -a && docker image prune -f -a

curl -X PUT -d '{"value": "General level secret prod env"}' \
  'http://localhost:8080/v1/secrets/TEST_SECRET?env=prod'

curl -X PUT -d '{"value": "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"}' \
  'http://localhost:8080/v1/secrets/GITHUB_PIPELINE_TOKEN?env=prod'

curl -X PUT -d '{"value": "General level secret env"}'   'http://localhost:8080/v1/secrets/TEST_SECRET'

# Secrets

## General
curl http://localhost:8080/v1/secrets?env=prod

curl -X DELETE http://localhost:8080/v1/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "General level secret"}' \
  http://localhost:8080/v1/secrets/TEST_SECRET


## General with env

curl -X PUT -d '{"value": "General level secret prod env"}' \
  'http://localhost:8080/v1/secrets/TEST_SECRET?env=prod'



## Repository
curl http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets

curl -X DELETE http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "repo level secret"}' \
  http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET

## Repository with env

curl -X PUT -d '{"value": "repo level secret prod env"}' \
  'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET?env=prod'



_______________________________________-

# environments

## General
curl -X PUT -d '{"value": "general"}' \
  'http://localhost:8080/v1/environments/TEST_ENV'

curl 'http://localhost:8080/v1/environments/'

curl -X 'DELETE http://localhost:8080/v1/environments/TEST_ENV'

## Repository
curl -X PUT -d '{"value": "repo"}' \
  'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV'

curl 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments'

curl -X DELETE 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV'

## General with env
curl -X PUT -d '{"value": "general prod"}' \
  'http://localhost:8080/v1/environments/TEST_ENV?env=prod'

curl 'http://localhost:8080/v1/environments?env=prod'

curl -X DELETE 'http://localhost:8080/v1/environments/TEST_ENV?env=prod'

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "repo-specific setting"}' \
  'http://localhost:8080/v1/environments/DOCKER_HOST?env=team-1%2Fprod'


## Repository with env
curl -X PUT -d '{"value": "repo prod"}' \
  'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV?env=prod'

curl 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments?env=prod'

curl -X DELETE 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV?env=prod'

_______________________________________-

# Pipelines

curl http://localhost:8080/v1/pipelines

curl http://localhost:8080/v1/pipelines/main-pipeline

curl http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline

curl -X DELETE http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/main-pipeline

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/team-1/dev/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline



# Trigger

curl http://localhost:8080/v1/overrides

curl http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X DELETE http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/hosein-yousefii/test-app

_______________________________________-

# Steps

curl http://localhost:8080/v1/steps

curl http://localhost:8080/v1/steps/simple-step

curl http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X DELETE http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/shared/utilities/archive-step.yaml" \
  http://localhost:8080/v1/steps/shared/utilities/archive-step

# Config Repository Environments

> Define environment variables in the config repository under the `environments/` folder. Each directory represents an environment scope (for example `prod`, `team-1/prod`, or `team-2/group-a/dev`) and must contain a single `env.yaml` (or `env.yml`) file.

```
environments/
  env.yaml                     # Global defaults (applies when no environment is specified)
  prod/env.yaml                # prod
  team-1/prod/env.yaml         # team-1/prod
  team-1/dev/env.yaml          # team-1/dev
  team-2/group-a/dev/env.yaml  # team-2/group-a/dev
```

- Keys without slashes (e.g. `API_URL: "https://api.example.com"`) create general variables for that environment scope.
- Keys in the form `owner/repo/NAME` (e.g. `acme/widgets/API_URL`) create repository-specific variables for the same environment scope.
- All values must be strings; duplicate keys within a scope are rejected.
- During sync the service upserts these definitions into the `environments` table and removes any that were deleted from the config repo.

Trigger a manual sync to force a refresh of pipelines, steps, environments, and triggers:

```
curl -X POST http://localhost:8080/v1/internal/config/sync
```

This endpoint now propagates environment changes in addition to pipelines and steps.
